# Mail Service - 이메일 시스템 완전 문서

> **핵심 목표**: Superhuman급 이메일 클라이언트 - 빠른 로딩, 실시간 동기화, AI 분류, 오프라인 지원

---

## 아키텍처 개요

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Mail System Architecture                       │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐              │
│  │   Frontend   │◄──►│   HTTP API   │◄──►│    Service   │              │
│  │   (React)    │    │   (Fiber)    │    │    Layer     │              │
│  └──────────────┘    └──────────────┘    └──────────────┘              │
│         │                   │                   │                        │
│         │ SSE               │                   │                        │
│         ▼                   │                   ▼                        │
│  ┌──────────────┐          │           ┌──────────────┐                │
│  │   Realtime   │◄─────────┼───────────│   Sync       │                │
│  │   Events     │          │           │   Service    │                │
│  └──────────────┘          │           └──────────────┘                │
│                            │                   │                        │
│                            │                   ▼                        │
│  ┌──────────────┐    ┌──────────────┐   ┌──────────────┐              │
│  │   Worker     │◄───│ Redis Stream │◄──│   Provider   │              │
│  │ (Background) │    │   (Jobs)     │   │ (Gmail/OL)   │              │
│  └──────────────┘    └──────────────┘   └──────────────┘              │
│         │                                      │                        │
│         ▼                                      ▼                        │
│  ┌──────────────┐    ┌──────────────┐   ┌──────────────┐              │
│  │  PostgreSQL  │    │   MongoDB    │   │   pgvector   │              │
│  │  (Metadata)  │    │   (Body)     │   │ (Embedding)  │              │
│  └──────────────┘    └──────────────┘   └──────────────┘              │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 1. 구현된 기능

### 1.1 Core Operations (service.go)

| 기능 | 메서드 | 상태 | 설명 |
|------|--------|------|------|
| 이메일 조회 | `GetEmail()` | ✅ | 단일 이메일 상세 조회 |
| 목록 조회 | `ListEmails()` | ✅ | 필터링/페이징 지원 |
| 하이브리드 조회 | `ListEmailsHybrid()` | ✅ | DB + Provider 병합 |
| 본문 조회 | `GetEmailBody()` | ✅ | MongoDB 캐시 활용 |
| 읽음 처리 | `MarkAsRead()` | ✅ | 배치 + Provider 비동기 동기화 |
| 안읽음 처리 | `MarkAsUnread()` | ✅ | 배치 + Provider 비동기 동기화 |
| 별표 | `Star()` / `Unstar()` | ✅ | 배치 + Provider 비동기 동기화 |
| 보관 | `Archive()` | ✅ | 배치 + Provider 비동기 동기화 |
| 휴지통 | `Trash()` | ✅ | 배치 + Provider 비동기 동기화 |
| 영구 삭제 | `Delete()` | ✅ | 배치 + Provider 비동기 동기화 |
| 폴더 이동 | `MoveToFolder()` | ✅ | 배치 + Provider 비동기 동기화 |
| 스누즈 | `Snooze()` | ✅ | 배치 지원, 지정 시간까지 숨김 |
| 스누즈 해제 | `Unsnooze()` | ✅ | 배치 지원 |
| 라벨 일괄 추가 | `BatchAddLabels()` | ✅ | 배치 + Provider 비동기 동기화 |
| 라벨 일괄 제거 | `BatchRemoveLabels()` | ✅ | 배치 + Provider 비동기 동기화 |
| 이메일 전송 | `SendEmail()` | ✅ | Gmail/Outlook API, 다중 수신자 (To/Cc/Bcc) |
| 답장 | `ReplyEmail()` | ✅ | In-Reply-To 헤더, 전체 답장 지원 |
| 전달 | `ForwardEmail()` | ✅ | 원본 인용 포함, 다중 수신자 |

### 1.2 Sync System (sync.go)

| 기능 | 메서드 | 상태 | 설명 |
|------|--------|------|------|
| 초기 동기화 | `InitialSync()` | ✅ | Progressive Loading (50개 즉시 + 나머지 백그라운드) |
| 증분 동기화 | `DeltaSync()` | ✅ | Gmail History API 활용 |
| 갭 동기화 | `GapSync()` | ✅ | 오프라인 복구 |
| 전체 재동기화 | `FullResync()` | ✅ | historyID 만료 시 |
| 체크포인트 복구 | `resumeFromCheckpoint()` | ✅ | 중단 후 재시작 |
| 본문 캐싱 | `fetchAndCacheBody()` | ✅ | MongoDB 30일 TTL |
| 첨부파일 저장 | `saveAttachments()` | ✅ | 메타데이터만 저장 |

### 1.3 Offline-First (modifier.go)

| 기능 | 메서드 | 상태 | 설명 |
|------|--------|------|------|
| 오프라인 큐 | `EnqueueModifier()` | ✅ | 로컬 수정 대기열 |
| 큐 처리 | `ProcessPendingModifiers()` | ✅ | 온라인 복귀 시 적용 |
| 버전 충돌 감지 | `checkVersionConflict()` | ✅ | 클라이언트 vs 서버 |
| 충돌 해결 | `resolveConflict()` | ✅ | 자동/수동 해결 |

### 1.4 첨부파일 시스템

| 기능 | API | 상태 | 설명 |
|------|-----|------|------|
| 이메일별 첨부파일 | `GET /mail/:id/attachments` | ✅ | 특정 이메일의 첨부파일 |
| 첨부파일 상세 | `GET /mail/:id/attachments/:attachmentId` | ✅ | 메타데이터 조회 |
| 첨부파일 다운로드 | `GET /mail/:id/attachments/:attachmentId/download` | ✅ | Provider API에서 직접 다운로드 |
| 전체 첨부파일 목록 | `GET /mail/attachments` | ✅ | 모아보기 (필터/정렬/페이징) |
| 첨부파일 통계 | `GET /mail/attachments/stats` | ✅ | 타입별 개수/용량 |
| 첨부파일 검색 | `GET /mail/attachments/search` | ✅ | 파일명 검색 |

---

## 2. API 엔드포인트

### 2.1 기본 API

```
GET    /mail                              # 이메일 목록 (DB + Provider 하이브리드)
GET    /mail/unified                      # 통합 목록 (커서 기반 페이징)
GET    /mail/search                       # 검색 (DB + Provider)
GET    /mail/fetch                        # Provider 직접 조회 (pageToken 지원)
GET    /mail/fetch/body                   # Provider 본문 직접 조회
POST   /mail/sync                         # 동기화 트리거

GET    /mail/:id                          # 이메일 상세
GET    /mail/:id/body                     # 이메일 본문

POST   /mail                              # 이메일 전송
POST   /mail/:id/reply                    # 답장
POST   /mail/:id/forward                  # 전달

POST   /mail/read                         # 읽음 처리 (배치)
POST   /mail/unread                       # 안읽음 처리 (배치)
POST   /mail/star                         # 별표 (배치)
POST   /mail/unstar                       # 별표 해제 (배치)
POST   /mail/archive                      # 보관 (배치)
POST   /mail/trash                        # 휴지통 (배치)
POST   /mail/delete                       # 영구 삭제 (배치)
POST   /mail/move                         # 폴더 이동 (배치)
POST   /mail/snooze                       # 스누즈 (배치)
POST   /mail/unsnooze                     # 스누즈 해제 (배치)
POST   /mail/labels/add                   # 라벨 일괄 추가 (배치)
POST   /mail/labels/remove                # 라벨 일괄 제거 (배치)
```

### 2.2 첨부파일 API

```
GET    /mail/attachments                  # 전체 첨부파일 목록 (모아보기)
       ?connection_id=1                   # 특정 계정 필터
       &type=image|video|pdf|document|spreadsheet|presentation|archive|text
       &mime_types=image/*,application/pdf
       &min_size=1024&max_size=10485760
       &start_date=2024-01-01T00:00:00Z
       &end_date=2024-12-31T23:59:59Z
       &sort_by=created_at|size|filename
       &sort_order=asc|desc
       &limit=50&offset=0

GET    /mail/attachments/stats            # 첨부파일 통계
       Response: {
         total_count: 1234,
         total_size: 5368709120,
         total_size_display: "5.00 GB",
         count_by_type: { image: 500, pdf: 200, ... },
         size_by_type: { image: 2147483648, pdf: 1073741824, ... }
       }

GET    /mail/attachments/search?q=report  # 첨부파일 검색
       &limit=50&offset=0

GET    /mail/:id/attachments              # 특정 이메일의 첨부파일
GET    /mail/:id/attachments/:attachmentId
GET    /mail/:id/attachments/:attachmentId/download
```

---

## 3. 데이터 모델

### 3.1 Email Entity (PostgreSQL)

```go
type MailEntity struct {
    // Identity
    ID           int64
    ExternalID   string     // Provider ID (Gmail/Outlook)
    ThreadID     *int64
    ConnectionID int64
    UserID       uuid.UUID

    // Provider
    Provider     string     // "google" | "outlook"
    AccountEmail string

    // Threading
    MessageID  string
    InReplyTo  string
    References []string

    // Participants
    FromEmail string
    FromName  string
    ToEmails  []string
    CcEmails  []string
    BccEmails []string

    // Content
    Subject string
    Snippet string         // 본문 미리보기 (200자)

    // Status Flags
    IsRead        bool
    IsDraft       bool
    HasAttachment bool
    IsReplied     bool
    IsForwarded   bool

    // Organization
    Folder string          // inbox, sent, drafts, trash, spam, archive
    Labels []string        // Gmail labels
    Tags   []string        // User custom tags

    // Workflow
    WorkflowStatus string  // todo, done, snoozed
    SnoozedUntil   *time.Time

    // AI Classification
    AIStatus   string      // pending, processing, completed, failed
    Category   string      // primary, social, promotions, updates, forums
    Priority   int         // 1(highest) ~ 5(lowest)
    Sentiment  float64     // -1.0 ~ 1.0
    Summary    string      // AI 요약
    ActionItem string      // 필요 액션
    Intent     string      // action_required, fyi, urgent, follow_up, scheduling
    IsUrgent   bool
    DueDate    *string     // 감지된 마감일

    // Timestamps
    ReceivedAt time.Time
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

### 3.2 Email Body (MongoDB)

```go
type MailBodyEntity struct {
    ID             primitive.ObjectID
    EmailID        int64
    ConnectionID   int64
    
    // Content
    HTML           string
    Text           string
    Attachments    []AttachmentEntity
    
    // Compression
    OriginalSize   int64
    CompressedSize int64
    IsCompressed   bool    // GZIP (> 1KB)
    
    // TTL
    CachedAt       time.Time
    ExpiresAt      time.Time
    TTLDays        int     // Default: 30
}
```

### 3.3 Attachment (PostgreSQL)

```go
type EmailAttachmentEntity struct {
    ID         int64
    EmailID    int64
    ExternalID string     // Provider attachment ID
    Filename   string
    MimeType   string
    Size       int64
    ContentID  *string    // For inline (CID)
    IsInline   bool
    CreatedAt  time.Time
}
```

### 3.4 Sync State

```go
type SyncState struct {
    ID           int64
    UserID       uuid.UUID
    ConnectionID int64
    Provider     string

    // Status
    Status       SyncStatus  // none, pending, syncing, idle, error, retry_scheduled
    Phase        SyncPhase   // initial_first_batch, initial_remaining, delta, gap, full_resync
    LastError    string

    // Gmail History
    HistoryID    int64

    // Watch (Push Notifications)
    WatchExpiry     *time.Time
    WatchResourceID string

    // Retry
    RetryCount   int
    MaxRetries   int         // Default: 5
    NextRetryAt  *time.Time

    // Checkpoint (Resume)
    CheckpointPageToken    string
    CheckpointSyncedCount  int
    CheckpointTotalCount   int

    // Stats
    TotalSynced           int64
    LastSyncCount         int
    LastSyncAt            *time.Time
    FirstSyncCompletedAt  *time.Time
    AvgSyncDurationMs     int64
    LastSyncDurationMs    int64
}
```

---

## 4. 동기화 전략

### 4.1 Progressive Loading (초기 동기화)

```
Phase 1: First Batch (< 2초 목표)
┌─────────────────────────────────────────┐
│ 1. Gmail API: 최근 50개 메일 조회        │
│ 2. SSE: 즉시 프론트엔드로 전송            │
│ 3. DB: 메타데이터 저장                    │
│ 4. MongoDB: 본문 캐싱 (비동기)            │
└─────────────────────────────────────────┘
         │
         ▼
Phase 2: Background Sync (백그라운드)
┌─────────────────────────────────────────┐
│ 1. 나머지 메일 페이지별 동기화             │
│ 2. 체크포인트 저장 (중단 복구용)          │
│ 3. AI 분류 작업 발행                      │
│ 4. RAG 인덱싱 작업 발행                   │
│ 5. Watch 설정 (Push Notification)         │
└─────────────────────────────────────────┘
```

### 4.2 Delta Sync (증분 동기화)

```
Gmail Pub/Sub Webhook 수신
         │
         ▼
┌─────────────────────────────────────────┐
│ 1. History API: historyId 이후 변경 조회  │
│ 2. 새 메일: DB 저장 + SSE 알림            │
│ 3. 삭제된 메일: DB에서 삭제               │
│ 4. 라벨 변경: DB 업데이트                 │
│ 5. historyId 갱신                         │
└─────────────────────────────────────────┘
```

### 4.3 Gap Sync (오프라인 복구)

```
사용자 온라인 복귀
         │
         ▼
┌─────────────────────────────────────────┐
│ 1. 마지막 historyId로 History API 조회   │
│ 2. 404 에러 → Full Resync 필요           │
│ 3. 정상 → 누락된 메일 동기화              │
│ 4. 오프라인 Modifier 큐 처리              │
└─────────────────────────────────────────┘
```

### 4.4 Retry Strategy

```go
RetryDelays = [30s, 1m, 5m, 15m, 30m]  // Exponential backoff
MaxRetries  = 5
```

---

## 5. Worker Jobs (Redis Stream)

### 5.1 Mail Jobs

| Job | Stream | Description |
|-----|--------|-------------|
| `mail.sync` | stream:mail:sync | 전체/증분 동기화 |
| `mail.sync.init` | stream:mail:sync | 페이지 디스커버리 |
| `mail.sync.page` | stream:mail:sync | 단일 페이지 동기화 |
| `mail.save` | stream:mail:save | 메타데이터 비동기 저장 |
| `mail.modify` | stream:mail:modify | Provider 상태 동기화 |
| `mail.send` | stream:mail:send | 이메일 전송 |

### 5.2 AI Jobs

| Job | Stream | Description |
|-----|--------|-------------|
| `ai.classify` | stream:ai:classify | 이메일 분류 |
| `ai.batch_classify` | stream:ai:classify | 배치 분류 |
| `ai.summarize` | stream:ai:summarize | 이메일 요약 |
| `ai.translate` | stream:ai:translate | 번역 |
| `ai.reply` | stream:ai:reply | 답장 생성 |

### 5.3 RAG Jobs

| Job | Stream | Description |
|-----|--------|-------------|
| `rag.index` | stream:rag:index | 단일 이메일 임베딩 |
| `rag.batch_index` | stream:rag:index | 배치 임베딩 |

---

## 6. 실시간 이벤트 (SSE)

### 6.1 Event Types

```go
const (
    EventSyncStarted    = "sync:started"
    EventSyncFirstBatch = "sync:first_batch"
    EventSyncProgress   = "sync:progress"
    EventSyncCompleted  = "sync:completed"
    EventSyncError      = "sync:error"
    EventNewEmail       = "email:new"
    EventEmailUpdated   = "email:updated"
    EventEmailDeleted   = "email:deleted"
)
```

### 6.2 Event Payloads

```go
// Sync Progress
type SyncProgressData struct {
    ConnectionID int64  `json:"connection_id"`
    Current      int    `json:"current"`
    Total        int    `json:"total,omitempty"`
    Status       string `json:"status"`
    Phase        string `json:"phase"`
}

// New Email
type NewEmailData struct {
    EmailID   int64     `json:"email_id"`
    Subject   string    `json:"subject"`
    From      string    `json:"from"`
    FromName  string    `json:"from_name"`
    Snippet   string    `json:"snippet"`
    Folder    string    `json:"folder"`
    IsRead    bool      `json:"is_read"`
    HasAttach bool      `json:"has_attachments"`
}
```

---

## 7. 성능 최적화

### 7.1 API 보호 레이어

```go
APIProtector := &Config{
    MaxConcurrent:     100,  // 최대 동시 요청
    RequestsPerSecond: 10,   // Gmail API 제한 고려
    BurstSize:         20,   // 버스트 허용
    DebounceDuration:  30 * time.Second,
    MaxPayloadSize:    50,   // 응답 최대 개수
}
```

### 7.2 캐싱 전략

```go
EmailListCache := &CacheConfig{
    L1MaxSize:          1000,          // 메모리 캐시
    L1TTL:              30 * time.Second,
    L2TTL:              1 * time.Minute, // Redis
    MaxCacheableOffset: 100,           // offset 100 이상은 캐시 안 함
}
```

### 7.3 데이터베이스 최적화

```sql
-- Partial Indexes (자주 쓰는 쿼리 최적화)
CREATE INDEX idx_emails_unread ON emails(user_id, is_read) WHERE is_read = FALSE;
CREATE INDEX idx_emails_starred ON emails(user_id, is_starred) WHERE is_starred = TRUE;

-- Covering Index (추가 조회 없이 인덱스만으로 결과)
CREATE INDEX idx_emails_list ON emails(user_id, folder, received_at DESC) 
    INCLUDE (id, subject, from_email, snippet, is_read);
```

### 7.4 배치 처리

```go
// N+1 문제 해결: 배치 조회
existingMap, _ := h.mailRepo.GetByExternalIDs(ctx, connectionID, externalIDs)

// 배치 업데이트
func BatchUpdateReadStatus(ctx, ids []int64, isRead bool)

// BulkUpsert (N개 메일을 1개 쿼리로)
func BulkUpsert(ctx, userID, connectionID, mails []*MailEntity)
```

---

## 8. 검색 최적화 (완료)

### 8.1 Full-Text Search 구현

PostgreSQL의 GIN 인덱스를 활용한 Full-Text Search 구현:

```sql
-- Full-Text Search Index (migration에 포함)
CREATE INDEX idx_emails_fts ON emails 
    USING GIN (to_tsvector('english', subject || ' ' || snippet));

-- 검색 쿼리 (mail_adapter.go)
SELECT *, ts_rank(to_tsvector('english', subject || ' ' || snippet), query) as rank
FROM emails, to_tsquery('english', 'hello:* & world:*') query
WHERE to_tsvector('english', subject || ' ' || snippet) @@ query
ORDER BY rank DESC, email_date DESC;
```

### 8.2 검색 쿼리 변환

```go
// buildTsQuery: 사용자 입력 → tsquery 형식 변환
// "hello world" → "hello:* & world:*"
// "report 2024" → "report:* & 2024:*"

func buildTsQuery(query string) string {
    words := strings.Fields(strings.TrimSpace(query))
    if len(words) == 0 {
        return ""
    }
    
    // 각 단어에 :* 붙여서 prefix 검색 지원
    var parts []string
    for _, word := range words {
        cleaned := strings.Map(func(r rune) rune {
            if unicode.IsLetter(r) || unicode.IsNumber(r) {
                return r
            }
            return -1
        }, word)
        if cleaned != "" {
            parts = append(parts, cleaned+":*")
        }
    }
    return strings.Join(parts, " & ")  // AND 조건
}
```

### 8.3 검색 기능 요약

| 기능 | 설명 | 상태 |
|------|------|------|
| Full-Text Search | GIN 인덱스 활용 | ✅ |
| Prefix 검색 | `hello:*` 형식 지원 | ✅ |
| AND 검색 | 여러 단어 모두 포함 | ✅ |
| Relevance 정렬 | `ts_rank` 활용 | ✅ |
| 시맨틱 검색 | pgvector 활용 (별도) | ✅ |

---

## 9. TODO / 개선 예정

### 9.1 알림 시스템 (완료)

```go
// Notification Domain (domain/notification.go)
type Notification struct {
    ID         int64
    UserID     uuid.UUID
    Type       NotificationType     // email, calendar, system, sync, ai
    Title      string
    Body       string
    Data       map[string]any
    EntityType string               // email, calendar_event
    EntityID   int64
    IsRead     bool
    ReadAt     *time.Time
    Priority   NotificationPriority // low, normal, high, urgent
    CreatedAt  time.Time
    ExpiresAt  *time.Time
}

// Realtime Events (SSE)
const (
    EventNewEmail        = "email.new"
    EventEmailClassified = "email.classified"
    EventEmailSummarized = "email.summarized"
    EventSyncStarted     = "sync.started"
    EventSyncFirstBatch  = "sync.first_batch"
    EventSyncProgress    = "sync.progress"
    EventSyncCompleted   = "sync.completed"
)
```

### 9.2 알림 API 엔드포인트

```
GET    /notifications                  # 알림 목록
       ?unread_only=true               # 안읽은 것만
       ?type=email|calendar|system     # 타입 필터
       &limit=50&offset=0

GET    /notifications/unread-count     # 안읽은 알림 수
POST   /notifications/mark-read        # 읽음 처리 (배치)
       { "notification_ids": [1, 2, 3] }
POST   /notifications/mark-all-read    # 전체 읽음 처리
DELETE /notifications/:id              # 개별 삭제
DELETE /notifications                  # 전체 삭제
```

---

## 10. 배치 처리 최적화 (완료)

### 10.1 구현된 배치 메서드

모든 일괄 처리 작업은 **단일 SQL 쿼리**로 최적화되어 있음:

| 메서드 | SQL 패턴 | 설명 |
|--------|----------|------|
| `BatchUpdateReadStatus` | `UPDATE ... WHERE id = ANY($1)` | 읽음/안읽음 일괄 처리 |
| `BatchUpdateFolder` | `UPDATE ... WHERE id = ANY($1)` | 폴더 이동 일괄 처리 |
| `BatchUpdateTags` | `UPDATE ... array_cat/array_remove` | 라벨 추가/제거 일괄 처리 |
| `BatchUpdateWorkflowStatus` | `UPDATE ... WHERE id = ANY($1)` | 스누즈/워크플로우 일괄 처리 |
| `BatchDelete` | `DELETE ... WHERE id = ANY($1)` | 영구 삭제 일괄 처리 |
| `BulkUpsert` | `INSERT ... ON CONFLICT DO UPDATE` | 동기화 시 대량 삽입/업데이트 |

### 10.2 서비스 계층 최적화

```go
// service.go - 모든 배치 작업이 단일 쿼리로 실행됨
func (s *Service) MarkAsRead(ctx, userID, emailIDs) error {
    // 1. DB 배치 업데이트 (단일 쿼리)
    s.mailRepo.BatchUpdateReadStatus(ctx, emailIDs, true)
    
    // 2. 캐시 무효화
    s.invalidateEmailCache(ctx, userID, emailIDs)
    
    // 3. Provider 동기화 (비동기 - Redis Stream)
    s.publishMailModifyJob(ctx, userID, emailIDs, "read")
}
```

### 10.3 Provider 동기화 최적화

```go
// MailModifyJob - Provider별 배치 처리
type MailModifyJob struct {
    UserID       string
    ConnectionID int64
    Provider     string   // google, outlook
    Action       string   // read, unread, star, archive, trash, delete, labels
    ExternalIDs  []string // Provider 메시지 ID 목록 (배치)
    AddLabels    []string
    RemoveLabels []string
}

// Gmail API 배치 수정 (Worker에서 처리)
// POST https://gmail.googleapis.com/gmail/v1/users/me/messages/batchModify
// { "ids": [...], "addLabelIds": [...], "removeLabelIds": [...] }
```

### 10.4 성능 비교

| 작업 | 최적화 전 | 최적화 후 | 개선율 |
|------|-----------|-----------|--------|
| 100개 읽음 처리 | 100 쿼리 | 1 쿼리 | **100x** |
| 50개 스누즈 | 50 쿼리 | 1 쿼리 | **50x** |
| 200개 라벨 추가 | 200 쿼리 | 1 쿼리 | **200x** |

---

## 11. TODO / 개선 예정

### 11.1 스마트 분류 규칙 (Pending)

```go
// 사용자 정의 분류 규칙
type ClassificationRule struct {
    ID          int64
    UserID      uuid.UUID
    Name        string
    Conditions  []RuleCondition  // AND 조건들
    Actions     []RuleAction     // 실행할 액션들
    Priority    int              // 규칙 우선순위
    IsEnabled   bool
}

type RuleCondition struct {
    Field    string  // from, to, subject, body, has_attachment
    Operator string  // contains, equals, matches, starts_with
    Value    string
}

type RuleAction struct {
    Type   string  // move_to_folder, add_label, mark_read, archive, star
    Value  string
}
```

---

## 12. 파일 구조

```
core/service/mail/
├── service.go      # Core Service (CRUD, 전송, 상태변경)
├── sync.go         # Sync Service (Progressive, Delta, Gap)
├── modifier.go     # Offline-First Modifier Queue
└── CLAUDE.md       # 이 문서

core/port/
├── in/mail.go              # MailService Interface
└── out/
    ├── mail_repository.go      # PostgreSQL Repository
    ├── mail_body_repository.go # MongoDB Repository
    ├── mail_provider.go        # Gmail/Outlook Provider
    ├── sync_repository.go      # Sync State Repository
    ├── modifier_repository.go  # Modifier Queue Repository
    └── messaging.go            # Redis Stream Jobs

adapter/
├── in/
│   ├── http/mail.go            # HTTP Handler
│   └── worker/mail_processor.go # Worker Processor
└── out/
    ├── persistence/
    │   ├── mail_adapter.go         # PostgreSQL
    │   └── attachment_adapter.go   # Attachments
    ├── mongodb/
    │   └── mail_body_adapter.go    # MongoDB
    └── provider/
        ├── gmail_adapter.go        # Gmail API
        └── outlook_adapter.go      # Outlook API

migrations/
├── 002_emails.sql          # emails 테이블
├── 013_sync_states.sql     # sync_states 테이블
├── 017_email_templates.sql # 이메일 템플릿
└── 019_attachments.sql     # email_attachments 테이블
```

---

## 13. 테스트 시나리오

### 13.1 초기 동기화

```bash
# 1. 새 사용자 Gmail 연결
POST /oauth/connect?provider=google

# 2. 동기화 시작 (자동)
# SSE로 progress 이벤트 수신

# 3. 첫 50개 메일 즉시 표시
# 나머지 백그라운드 동기화
```

### 13.2 오프라인 작업

```bash
# 1. 오프라인 상태에서 읽음 처리
POST /mail/read { ids: [1, 2, 3] }

# 2. 로컬 DB 즉시 업데이트
# 3. Modifier 큐에 저장

# 4. 온라인 복귀 시 자동 동기화
# 5. 충돌 발생 시 해결 로직 실행
```

### 10.3 실시간 알림

```bash
# 1. Gmail Pub/Sub Webhook 수신
POST /webhook/gmail

# 2. Delta Sync 실행
# 3. 새 메일 발견 시 SSE 이벤트 전송
# 4. 프론트엔드 목록 자동 업데이트
```
