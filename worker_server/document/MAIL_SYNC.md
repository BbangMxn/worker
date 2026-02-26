# Mail Sync System - 코드 가이드

## 개요

Gmail/Outlook에서 실시간으로 메일을 동기화하는 시스템.

---

## 1. Gmail Push Notification 설정

### Google Cloud 설정

```bash
# 1. Pub/Sub 토픽 생성
gcloud pubsub topics create gmail-push-notifications

# 2. 구독 생성 (Push to webhook)
gcloud pubsub subscriptions create gmail-push-sub \
  --topic=gmail-push-notifications \
  --push-endpoint=https://your-api.com/webhook/gmail \
  --ack-deadline=60

# 3. Gmail API에 Pub/Sub 권한 부여
gcloud pubsub topics add-iam-policy-binding gmail-push-notifications \
  --member="serviceAccount:gmail-api-push@system.gserviceaccount.com" \
  --role="roles/pubsub.publisher"
```

### 환경변수

```env
GOOGLE_CLOUD_PROJECT=your-project-id
GOOGLE_PUBSUB_TOPIC=gmail-push-notifications
GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json
```

---

## 2. 도메인 모델

### core/domain/sync.go

```go
package domain

import "time"

type SyncState struct {
    ID           int64      `json:"id"`
    UserID       string     `json:"user_id"`
    ConnectionID int64      `json:"connection_id"`
    Provider     Provider   `json:"provider"`
    HistoryID    uint64     `json:"history_id"`      // Gmail history ID
    WatchExpiry  time.Time  `json:"watch_expiry"`    // Watch 만료 시간
    Status       SyncStatus `json:"status"`
    LastSyncAt   time.Time  `json:"last_sync_at"`
    LastError    string     `json:"last_error,omitempty"`
}

type SyncStatus string

const (
    SyncStatusIdle         SyncStatus = "idle"
    SyncStatusSyncing      SyncStatus = "syncing"
    SyncStatusError        SyncStatus = "error"
    SyncStatusWatchExpired SyncStatus = "watch_expired"
)

type SyncJob struct {
    ID           string          `json:"id"`
    Type         JobType         `json:"type"`
    UserID       string          `json:"user_id"`
    ConnectionID int64           `json:"connection_id"`
    HistoryID    uint64          `json:"history_id,omitempty"`
    Priority     int             `json:"priority"`
    RetryCount   int             `json:"retry_count"`
    CreatedAt    time.Time       `json:"created_at"`
}

type JobType string

const (
    JobMailSyncFull  JobType = "mail.sync.full"
    JobMailSyncDelta JobType = "mail.sync.delta"
    JobAIClassify    JobType = "ai.classify"
)
```

---

## 3. Port 인터페이스

### core/port/out/push_notification.go

```go
package out

import (
    "context"
    "golang.org/x/oauth2"
)

type PushNotificationPort interface {
    // Gmail watch 등록 (7일 유효)
    WatchMailbox(ctx context.Context, token *oauth2.Token, labelIDs []string) (*WatchResponse, error)
    
    // Watch 중지
    StopWatch(ctx context.Context, token *oauth2.Token) error
}

type WatchResponse struct {
    HistoryID  uint64
    Expiration time.Time
}
```

### core/port/out/sync_repository.go

```go
package out

import "context"

type SyncStateRepository interface {
    GetByConnectionID(ctx context.Context, connectionID int64) (*domain.SyncState, error)
    Create(ctx context.Context, state *domain.SyncState) error
    Update(ctx context.Context, state *domain.SyncState) error
    GetExpiredWatches(ctx context.Context) ([]*domain.SyncState, error)
}
```

---

## 4. Service 구현

### core/service/mail_sync.go

```go
package service

type MailSyncService struct {
    mailRepo      out.MailRepository
    syncRepo      out.SyncStateRepository
    mailProvider  out.MailProvider
    pushNotif     out.PushNotificationPort
    messageQueue  out.MessageQueuePort
    oauthRepo     out.OAuthRepository
}

// 초기 동기화 (OAuth 연결 직후)
func (s *MailSyncService) InitialSync(ctx context.Context, connectionID int64) error {
    // 1. OAuth 토큰 가져오기
    token, _ := s.oauthRepo.GetToken(ctx, connectionID)
    
    // 2. Gmail에서 전체 메일 가져오기 (최근 N개)
    result, _ := s.mailProvider.InitialSync(ctx, token, &SyncOptions{MaxResults: 500})
    
    // 3. DB에 저장
    for _, msg := range result.Messages {
        s.mailRepo.Create(ctx, convertToEntity(msg))
    }
    
    // 4. Watch 등록
    watch, _ := s.pushNotif.WatchMailbox(ctx, token, []string{"INBOX"})
    
    // 5. SyncState 저장
    s.syncRepo.Create(ctx, &domain.SyncState{
        ConnectionID: connectionID,
        HistoryID:    watch.HistoryID,
        WatchExpiry:  watch.Expiration,
        Status:       domain.SyncStatusIdle,
    })
    
    return nil
}

// 증분 동기화 (Pub/Sub 알림 수신 시)
func (s *MailSyncService) DeltaSync(ctx context.Context, connectionID int64, newHistoryID uint64) error {
    // 1. 현재 상태 조회
    state, _ := s.syncRepo.GetByConnectionID(ctx, connectionID)
    
    // 2. History API로 변경사항 조회
    token, _ := s.oauthRepo.GetToken(ctx, connectionID)
    changes, _ := s.mailProvider.GetHistory(ctx, token, state.HistoryID)
    
    // 3. 변경사항 처리
    for _, change := range changes {
        switch change.Type {
        case "messageAdded":
            // 새 메일 가져와서 저장
            msg, _ := s.mailProvider.GetMessage(ctx, token, change.MessageID)
            s.mailRepo.Create(ctx, convertToEntity(msg))
            
            // AI 분류 작업 발행
            s.messageQueue.Publish(ctx, "ai:classify", &SyncJob{
                Type:    JobAIClassify,
                Payload: msg.ID,
            })
            
        case "messageDeleted":
            s.mailRepo.Delete(ctx, change.MessageID)
        }
    }
    
    // 4. 상태 업데이트
    state.HistoryID = newHistoryID
    state.LastSyncAt = time.Now()
    s.syncRepo.Update(ctx, state)
    
    return nil
}
```

---

## 5. Webhook Handler

### adapter/in/http/webhook.go

```go
package http

type WebhookHandler struct {
    mailSyncService *service.MailSyncService
    messageQueue    out.MessageQueuePort
}

// POST /webhook/gmail
func (h *WebhookHandler) HandleGmailPush(c *fiber.Ctx) error {
    var notification struct {
        Message struct {
            Data string `json:"data"` // base64 encoded
        } `json:"message"`
    }
    c.BodyParser(&notification)
    
    // Base64 디코드
    data, _ := base64.StdEncoding.DecodeString(notification.Message.Data)
    
    var gmailNotif struct {
        EmailAddress string `json:"emailAddress"`
        HistoryID    uint64 `json:"historyId"`
    }
    json.Unmarshal(data, &gmailNotif)
    
    // Redis Stream에 동기화 작업 발행
    h.messageQueue.Publish(c.Context(), "mail:sync", &domain.SyncJob{
        Type:      domain.JobMailSyncDelta,
        UserID:    gmailNotif.EmailAddress,
        HistoryID: gmailNotif.HistoryID,
        Priority:  10, // High
    })
    
    return c.SendStatus(200)
}
```

---

## 6. Worker Processor

### adapter/in/worker/mail_sync_processor.go

```go
package worker

type MailSyncProcessor struct {
    mailSyncService *service.MailSyncService
}

func (p *MailSyncProcessor) Process(ctx context.Context, job *domain.SyncJob) error {
    switch job.Type {
    case domain.JobMailSyncFull:
        return p.mailSyncService.InitialSync(ctx, job.ConnectionID)
    case domain.JobMailSyncDelta:
        return p.mailSyncService.DeltaSync(ctx, job.ConnectionID, job.HistoryID)
    }
    return nil
}
```

---

## 7. Watch 갱신 스케줄러

Gmail Watch는 7일 후 만료됨. 주기적 갱신 필요.

```go
// 매일 실행 (cron 또는 goroutine)
func (s *MailSyncService) RenewExpiredWatches(ctx context.Context) error {
    // 만료 임박한 watch 조회 (1일 이내)
    states, _ := s.syncRepo.GetExpiredWatches(ctx)
    
    for _, state := range states {
        token, _ := s.oauthRepo.GetToken(ctx, state.ConnectionID)
        watch, _ := s.pushNotif.WatchMailbox(ctx, token, []string{"INBOX"})
        
        state.WatchExpiry = watch.Expiration
        s.syncRepo.Update(ctx, state)
    }
    
    return nil
}
```
