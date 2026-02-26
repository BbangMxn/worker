# Inbound Adapter

> **핵심**: 외부 요청(HTTP, Webhook, Redis Stream)을 받아 Core(Service)를 호출

---

## 디렉토리 구조

```
adapter/in/
├── CLAUDE.md
│
├── http/                        # REST API 핸들러 (Fiber)
│   ├── mail.go                  # 메일 CRUD, 배치, 검색
│   ├── mail_optimized.go        # 최적화된 메일 조회
│   ├── calendar.go              # 캘린더 CRUD
│   ├── contact.go               # 연락처 CRUD
│   ├── ai.go                    # AI 분류, 요약, 답장, 채팅
│   ├── oauth.go                 # OAuth 인증
│   ├── settings.go              # 사용자 설정
│   ├── shortcut.go              # 키보드 단축키
│   ├── label.go                 # 라벨 관리
│   ├── folder.go                # 폴더 관리
│   ├── template_handler.go      # 이메일 템플릿
│   ├── sender_profile.go        # 발신자 프로필
│   ├── report.go                # 리포트
│   ├── notification.go          # 알림 설정
│   ├── webhook.go               # Webhook 수신 (Gmail Push)
│   ├── sse.go                   # SSE 실시간 이벤트
│   ├── health.go                # 헬스 체크
│   └── helpers.go               # 공용 헬퍼 (응답, 에러)
│
├── worker/                      # Redis Stream Worker
│   ├── pool.go                  # go-pkgz/pool 기반 워커 풀
│   ├── handler.go               # Job 라우터
│   ├── message.go               # 메시지 파싱
│   ├── mail_processor.go        # 메일 동기화, 발송
│   ├── ai_processor.go          # AI 분류, 요약 (배치 최적화)
│   ├── rag_processor.go         # RAG 인덱싱 (DB에서 데이터 조회)
│   ├── calendar_processor.go    # 캘린더 동기화
│   ├── webhook_processor.go     # Webhook 처리
│   ├── background_sync_scheduler.go  # 백그라운드 동기화
│   ├── gap_sync_scheduler.go    # 갭 동기화
│   ├── sync_retry_scheduler.go  # 실패 재시도
│   └── watch_renew_scheduler.go # Gmail Watch 갱신
│
├── scheduler/                   # 스케줄러 (cron)
│   └── (cron jobs)
│
├── webhook/                     # Webhook 전용 처리
│   └── (webhook handlers)
│
└── websocket/                   # WebSocket (실시간)
    └── (websocket handlers)
```

---

## HTTP Handlers

### MailHandler (`mail.go`)

```go
// 라우트 등록
mail := app.Group("/mail")

// 조회
mail.Get("/", h.ListEmails)              // DB 우선 + Provider 보충
mail.Get("/unified", h.ListEmailsUnified) // 커서 기반 페이징
mail.Get("/search", h.SearchEmails)       // Gmail API 직접 검색
mail.Get("/fetch", h.FetchFromProvider)   // Provider에서 직접 가져오기
mail.Get("/fetch/body", h.FetchBodyFromProvider)
mail.Get("/:id", h.GetEmail)
mail.Get("/:id/body", h.GetEmailBody)

// 첨부파일
mail.Get("/attachments", h.ListAllAttachments)     // 전체 모아보기
mail.Get("/attachments/stats", h.GetAttachmentStats)
mail.Get("/attachments/search", h.SearchAttachments)
mail.Get("/:id/attachments", h.GetAttachments)
mail.Get("/:id/attachments/:attachmentId", h.GetAttachment)
mail.Get("/:id/attachments/:attachmentId/download", h.DownloadAttachment)

// 발송
mail.Post("/", h.SendEmail)
mail.Post("/:id/reply", h.ReplyEmail)
mail.Post("/:id/forward", h.ForwardEmail)

// 배치 작업
mail.Post("/read", h.MarkAsRead)
mail.Post("/unread", h.MarkAsUnread)
mail.Post("/star", h.Star)
mail.Post("/unstar", h.Unstar)
mail.Post("/archive", h.Archive)
mail.Post("/trash", h.Trash)
mail.Post("/delete", h.DeleteEmails)
mail.Post("/move", h.MoveToFolder)
mail.Post("/snooze", h.Snooze)
mail.Post("/unsnooze", h.Unsnooze)
mail.Post("/labels/add", h.BatchAddLabels)
mail.Post("/labels/remove", h.BatchRemoveLabels)

// 동기화
mail.Post("/sync", h.TriggerSync)
```

### API 보호 레이어

```go
// Rate Limiting + 캐시
apiProtector := ratelimit.NewAPIProtector(redisClient, &ratelimit.Config{
    MaxConcurrent:     100,
    RequestsPerSecond: 10,  // Gmail API 제한 고려
    BurstSize:         20,
    DebounceDuration:  30 * time.Second,
    MaxPayloadSize:    50,
})

// L1/L2 이메일 캐시
emailCache := ratelimit.NewEmailListCache(redisClient, &ratelimit.CacheConfig{
    L1MaxSize:          1000,
    L1TTL:              30 * time.Second,
    L2TTL:              1 * time.Minute,
    MaxCacheableOffset: 100,
})
```

### AIHandler (`ai.go`)

```go
ai := app.Group("/ai")

// 분류/요약
ai.Post("/classify/:id", h.ClassifyEmail)
ai.Post("/classify/batch", h.ClassifyBatch)
ai.Post("/summarize/:id", h.SummarizeEmail)
ai.Post("/reply/:id", h.GenerateReply)
ai.Post("/extract-meeting/:id", h.ExtractMeeting)

// 채팅
ai.Post("/chat", h.Chat)
ai.Get("/chat/stream", h.ChatStream)

// Proposal 관리
ai.Post("/proposals/:id/confirm", h.ConfirmProposal)
ai.Post("/proposals/:id/reject", h.RejectProposal)
ai.Get("/proposals", h.ListProposals)

// 개인화
ai.Post("/autocomplete", h.GetAutocomplete)
ai.Get("/autocomplete/context", h.GetAutocompleteContext)
ai.Get("/profile", h.GetUserProfile)
ai.Put("/profile", h.UpdateUserProfile)
ai.Get("/contacts/frequent", h.GetFrequentContacts)
ai.Get("/contacts/important", h.GetImportantContacts)
ai.Get("/patterns", h.GetCommunicationPatterns)
ai.Get("/phrases", h.GetFrequentPhrases)
```

### SSE Handler (`sse.go`)

```go
// Server-Sent Events 실시간 알림
sse := app.Group("/sse")
sse.Get("/events", h.StreamEvents)

// 이벤트 타입
// - email.new: 새 메일 도착
// - email.updated: 메일 상태 변경
// - sync.progress: 동기화 진행 상황
// - sync.complete: 동기화 완료
```

---

## Worker (Redis Stream)

### Worker Pool (`pool.go`)

**go-pkgz/pool 기반 고성능 워커 풀** (41% 성능 향상):

```go
type PoolConfig struct {
    MinWorkers         int           // 최소 워커 수 (기본 2)
    MaxWorkers         int           // 최대 워커 수 (기본 20)
    QueueSize          int           // 작업 큐 크기 (기본 1000)
    ScaleUpThreshold   float64       // 스케일업 임계값 (0.8)
    ScaleDownThreshold float64       // 스케일다운 임계값 (0.2)
    ScaleInterval      time.Duration // 스케일링 체크 간격
    JobTimeout         time.Duration // 기본 작업 타임아웃 (60초)
    JobTimeoutByType   map[JobType]time.Duration // 작업별 타임아웃
}

// 작업 유형별 타임아웃
JobMailSync:       3 * time.Minute   // 초기 동기화
JobMailDeltaSync:  2 * time.Minute   // 증분 동기화
JobMailBatch:      5 * time.Minute   // 배치 처리
JobMailSend:       30 * time.Second  // 메일 발송
JobAIClassify:     30 * time.Second  // AI 분류
JobRAGIndex:       1 * time.Minute   // RAG 인덱싱
JobRAGBatchIndex:  5 * time.Minute   // RAG 배치 인덱싱
```

### Job Types

```go
const (
    // 메일
    JobMailSync      = "mail.sync"
    JobMailDeltaSync = "mail.delta_sync"
    JobMailBatch     = "mail.batch"
    JobMailSend      = "mail.send"
    JobMailReply     = "mail.reply"
    JobMailModify    = "mail.modify"
    
    // 캘린더
    JobCalendarSync  = "calendar.sync"
    
    // AI
    JobAIClassify    = "ai.classify"
    JobAISummarize   = "ai.summarize"
    JobAIReply       = "ai.reply"
    
    // RAG
    JobRAGIndex      = "rag.index"
    JobRAGBatchIndex = "rag.batch"
    
    // 프로필
    JobProfileAnalyze = "profile.analyze"
    
    // 리포트
    JobReportGenerate = "report.generate"
)
```

### MailProcessor (`mail_processor.go`)

```go
// Push 기반 실시간 동기화 (Superhuman 스타일)
ProcessSync(ctx, msg)       // 초기 동기화 + Gmail Watch 설정
ProcessDeltaSync(ctx, msg)  // Pub/Sub 트리거 증분 동기화
ProcessSend(ctx, msg)       // 메일 발송
ProcessModify(ctx, msg)     // Provider 상태 동기화 + SSE 브로드캐스트
```

### AIProcessor (`ai_processor.go`)

**배치 최적화** - 개별 요청을 모아서 배치 처리:

```go
type AIProcessor struct {
    // 배치 누적
    classifyBatch  []int64
    summarizeBatch []int64
    batchSize      int           // 10개씩 배치
    batchTimeout   time.Duration // 최대 3초 대기
}

// 개별 요청 → 배치 누적 → 일괄 처리
ProcessClassify(ctx, msg)      // 배치에 추가
ProcessClassifyBatch(ctx, msg) // 배치 처리 실행
ProcessSummarize(ctx, msg)     // 배치에 추가
```

### RAGProcessor (`rag_processor.go`)

**Sync에서 최소 Payload만 전달, Processor에서 DB 조회**:

```go
// Sync에서 발행하는 Payload (최소)
type RAGIndexMinimalPayload struct {
    UserID  string `json:"user_id"`
    EmailID int64  `json:"email_id"`
}

// Processor에서 실제 데이터 조회
func (p *RAGProcessor) ProcessIndex(ctx, msg) error {
    // 1. PostgreSQL에서 이메일 메타데이터 조회
    email, _ := p.emailRepo.GetByID(ctx, payload.EmailID)
    
    // 2. MongoDB에서 본문 조회
    body, _ := p.bodyRepo.GetBody(ctx, payload.EmailID)
    
    // 3. 임베딩 생성 및 인덱싱
    p.indexer.IndexEmail(ctx, req)
}
```

---

## Webhook Handler (`webhook.go`)

### Gmail Push Notification

```go
// Gmail Pub/Sub 웹훅 수신
webhook := app.Group("/webhook")
webhook.Post("/gmail", h.HandleGmailPush)
webhook.Post("/outlook", h.HandleOutlookPush)

// Gmail Push 처리 플로우
// 1. Pub/Sub 메시지 수신
// 2. historyId 추출
// 3. mail.delta_sync Job 발행
// 4. Worker에서 DeltaSync 실행
```

---

## 스케줄러

### BackgroundSyncScheduler

```go
// 주기적 백그라운드 동기화 (안전망)
// Gmail Watch가 실패할 경우를 대비
type BackgroundSyncScheduler struct {
    interval time.Duration  // 기본 5분
}
```

### WatchRenewScheduler

```go
// Gmail Watch 자동 갱신 (7일마다 만료)
type WatchRenewScheduler struct {
    renewBefore time.Duration  // 만료 1일 전 갱신
}
```

### GapSyncScheduler

```go
// 동기화 갭 탐지 및 복구
type GapSyncScheduler struct {
    checkInterval time.Duration
}
```

### SyncRetryScheduler

```go
// 실패한 동기화 재시도
type SyncRetryScheduler struct {
    maxRetries int
    backoff    time.Duration
}
```

---

## 구현 상태

### HTTP (완료)

- [x] MailHandler (CRUD, 배치, 검색, 첨부파일)
- [x] CalendarHandler
- [x] ContactHandler
- [x] AIHandler (분류, 요약, 답장, 채팅, Proposal)
- [x] OAuthHandler (Google, Outlook)
- [x] SettingsHandler (설정, 분류 규칙)
- [x] LabelHandler, FolderHandler
- [x] TemplateHandler
- [x] WebhookHandler (Gmail Push)
- [x] SSE Handler (기본)

### Worker (완료)

- [x] Worker Pool (go-pkgz/pool 기반)
- [x] MailProcessor (동기화, 발송, 상태 동기화)
- [x] AIProcessor (배치 최적화)
- [x] RAGProcessor (DB 조회 후 인덱싱)
- [x] CalendarProcessor
- [x] WebhookProcessor

### 스케줄러 (완료)

- [x] BackgroundSyncScheduler
- [x] WatchRenewScheduler
- [x] GapSyncScheduler
- [x] SyncRetryScheduler

### 개선 필요

- [ ] SSE Handler 고도화 (채널별 구독 관리)
- [ ] Auto Scaler (동적 워커 수 조절)
- [ ] Priority Queue 처리 최적화
- [ ] Dead Letter Queue 처리

---

## 인증 미들웨어

```go
// JWT 인증
func AuthMiddleware(jwtSecret string) fiber.Handler {
    return func(c *fiber.Ctx) error {
        token := c.Get("Authorization")
        // Bearer 토큰 검증
        // userID를 Context에 저장
        c.Locals("userID", userID)
        return c.Next()
    }
}

// 핸들러에서 사용
userID, err := GetUserID(c)
```

---

## 응답 형식

```go
// 성공 응답
c.JSON(fiber.Map{
    "data": result,
})

// 에러 응답
func ErrorResponse(c *fiber.Ctx, status int, message string) error {
    return c.Status(status).JSON(fiber.Map{
        "error": message,
    })
}
```
