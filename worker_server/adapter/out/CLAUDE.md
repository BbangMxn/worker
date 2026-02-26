# Outbound Adapter

> **핵심**: Core(Service)가 외부 시스템과 통신할 때 사용하는 Port Out 인터페이스 구현

---

## 디렉토리 구조

```
adapter/out/
├── CLAUDE.md
│
├── persistence/             # PostgreSQL (Supabase)
│   ├── mail_adapter.go      # 이메일 CRUD, 검색
│   ├── mail_adapter_thread.go # 스레드 관리
│   ├── mail_domain_wrapper.go # 도메인 래퍼
│   ├── modifier_adapter.go  # 메일 수정 배치
│   ├── label_adapter.go     # 라벨 CRUD
│   ├── folder_adapter.go    # 폴더 관리
│   ├── smart_folder_adapter.go # 스마트 폴더
│   ├── calendar_adapter.go  # 캘린더 CRUD
│   ├── calendar_sync_adapter.go # 캘린더 동기화
│   ├── contact_adapter.go   # 연락처 CRUD
│   ├── contact_cache.go     # 연락처 캐시
│   ├── oauth.go             # OAuth 토큰 저장
│   ├── oauth_state_store.go # OAuth State 관리
│   ├── settings_adapter.go  # 사용자 설정
│   ├── shortcut_adapter.go  # 키보드 단축키
│   ├── template_adapter.go  # 이메일 템플릿
│   ├── sender_profile_adapter.go # 발신자 프로필
│   ├── notification_adapter.go # 알림 설정
│   ├── webhook_adapter.go   # Webhook 구독
│   ├── sync_state_adapter.go # 동기화 상태
│   ├── attachment_adapter.go # 첨부파일
│   └── errors.go            # 공용 에러
│
├── mongodb/                 # MongoDB
│   ├── client.go            # MongoDB 클라이언트
│   ├── mail_body_adapter.go # 이메일 본문 (30일 TTL, gzip 압축)
│   └── report_adapter.go    # 리포트 저장
│
├── graph/                   # Neo4j
│   ├── driver.go            # Neo4j 드라이버
│   ├── personalization_adapter.go # 개인화 데이터
│   ├── classification_adapter.go  # 분류 학습 데이터
│   └── vector_adapter.go    # pgvector 연동 (이전 버전)
│
├── messaging/               # Redis Stream
│   ├── producer.go          # 작업 발행 (Pub)
│   └── consumer.go          # 작업 소비 (Sub)
│
├── provider/                # 외부 API (Gmail, Outlook)
│   ├── factory.go           # Provider 팩토리
│   ├── gmail_adapter.go     # Gmail API
│   ├── gmail/gmail.go       # Gmail 유틸리티
│   ├── outlook_adapter.go   # Outlook API
│   ├── outlook/outlook.go   # Outlook 유틸리티
│   └── google_calendar_adapter.go # Google Calendar API
│
├── cache/                   # Redis 캐시 (L2)
│   └── (common/cache.go에서 관리)
│
└── realtime/                # 실시간 (SSE/WebSocket)
    └── sse_adapter.go       # SSE 브로드캐스트
```

---

## Persistence (PostgreSQL/Supabase)

### MailAdapter (`mail_adapter.go`)

```go
// 주요 메서드
Create(ctx, entity) error
GetByID(ctx, id) (*MailEntity, error)
GetByExternalID(ctx, connectionID, externalID) (*MailEntity, error)
List(ctx, filter) ([]*MailEntity, int, error)
Update(ctx, entity) error
Delete(ctx, id) error

// 배치 작업
BatchUpdateReadStatus(ctx, ids, isRead) error
BatchUpdateFolder(ctx, ids, folder) error
BatchUpdateTags(ctx, ids, addTags, removeTags) error
BatchUpdateWorkflowStatus(ctx, ids, status, snoozedUntil) error
BatchDelete(ctx, ids) error

// 검색
Search(ctx, query, userID, limit, offset) ([]*MailEntity, int, error)
GetByThreadID(ctx, threadID) ([]*MailEntity, error)
```

### Row Mapping

```go
// embedding 컬럼 제외 (pgvector 별도 처리)
const mailSelectColumns = `
    e.id, e.external_id, e.external_thread_id, ...
    -- embedding 컬럼은 RAG에서 별도 처리
`

// COUNT(*) OVER() 윈도우 함수로 총 개수 조회 최적화
type mailRowWithCount struct {
    mailRow
    TotalCount int `db:"total_count"`
}
```

### OAuthAdapter (`oauth.go`)

```go
Create(ctx, entity) error
GetByID(ctx, id) (*OAuthConnectionEntity, error)
GetByEmail(ctx, userID, provider, email) (*OAuthConnectionEntity, error)
GetByEmailOnly(ctx, email, provider) (*OAuthConnectionEntity, error)
GetByWebhookID(ctx, subscriptionID, provider) (*OAuthConnectionEntity, error)
ListByUser(ctx, userID) ([]*OAuthConnectionEntity, error)
ListAllActive(ctx) ([]*OAuthConnectionEntity, error)
Update(ctx, entity) error
Disconnect(ctx, id) error
```

### SettingsAdapter (`settings_adapter.go`)

```go
Get(ctx, userID) (*SettingsEntity, error)
Upsert(ctx, userID, settings) error
GetClassificationRules(ctx, userID) (*ClassificationRules, error)
UpdateClassificationRules(ctx, userID, rules) error
```

---

## MongoDB

### MailBodyAdapter (`mail_body_adapter.go`)

**30일 TTL + gzip 압축**으로 이메일 본문 저장:

```go
// 인덱스 (TTL 포함)
{Key: "email_id", Unique: true}
{Key: "connection_id"}
{Key: "expires_at", ExpireAfterSeconds: 0}  // TTL 인덱스

// 압축 (1KB 이상)
const compressionThreshold = 1024

// 주요 메서드
SaveBody(ctx, body) error
GetBody(ctx, emailID) (*MailBodyEntity, error)
DeleteBody(ctx, emailID) error
ExistsBody(ctx, emailID) (bool, error)

// 배치
BulkSaveBody(ctx, bodies) error
BulkGetBody(ctx, emailIDs) (map[int64]*MailBodyEntity, error)
BulkDeleteBody(ctx, emailIDs) error

// 정리
DeleteExpired(ctx) (int64, error)
DeleteOlderThan(ctx, before) (int64, error)
DeleteByConnectionID(ctx, connectionID) (int64, error)

// 통계
GetStorageStats(ctx) (*BodyStorageStats, error)
GetCompressionStats(ctx) (*CompressionStats, error)
```

### Document 구조

```go
type mailBodyDocument struct {
    EmailID      int64  `bson:"email_id"`
    ConnectionID int64  `bson:"connection_id"`
    ExternalID   string `bson:"external_id"`
    
    HTML         []byte `bson:"html"`      // gzip 압축
    Text         []byte `bson:"text"`      // gzip 압축
    IsCompressed bool   `bson:"is_compressed"`
    
    OriginalSize   int64 `bson:"original_size"`
    CompressedSize int64 `bson:"compressed_size"`
    
    CachedAt  time.Time `bson:"cached_at"`
    ExpiresAt time.Time `bson:"expires_at"`  // TTL
    TTLDays   int       `bson:"ttl_days"`
}
```

---

## Neo4j (Graph)

### PersonalizationAdapter (`personalization_adapter.go`)

**사용자 개인화 데이터** 저장:

```go
// 프로필
GetUserProfile(ctx, userID) (*UserProfile, error)
UpdateUserProfile(ctx, userID, profile) error

// 특성/성향
GetUserTraits(ctx, userID) ([]*UserTrait, error)
UpdateUserTrait(ctx, userID, trait) error

// 작문 스타일
GetWritingStyle(ctx, userID) (*WritingStyle, error)
UpdateWritingStyle(ctx, userID, style) error

// 자주 쓰는 문구
GetPhrases(ctx, userID) ([]*FrequentPhrase, error)
AddPhrase(ctx, userID, phrase) error

// 톤 선호도
GetTonePreference(ctx, userID, context) (*TonePreference, error)
UpdateTonePreference(ctx, userID, pref) error

// 연락처 관계
GetContactRelationship(ctx, userID, email) (*ContactRelationship, error)
UpsertContactRelationship(ctx, userID, rel) error

// 커뮤니케이션 패턴
UpsertCommunicationPattern(ctx, userID, pattern) error
```

### 그래프 모델

```cypher
(User)-[:HAS_TRAIT]->(Trait)
(User)-[:HAS_STYLE]->(WritingStyle)
(User)-[:USES_PHRASE]->(Phrase)
(User)-[:PREFERS_TONE]->(TonePreference)
(User)-[:COMMUNICATES_WITH]->(Contact)
```

---

## Redis Stream (Messaging)

### Producer (`producer.go`)

```go
// 스트림 이름
const (
    StreamMailSend      = "mail:send"
    StreamMailSync      = "mail:sync"
    StreamMailBatch     = "mail:batch"
    StreamMailModify    = "mail:modify"
    StreamCalendarSync  = "calendar:sync"
    StreamAIClassify    = "ai:classify"
    StreamAISummarize   = "ai:summarize"
    StreamRAGIndex      = "rag:index"
    StreamRAGBatchIndex = "rag:batch"
    StreamProfile       = "profile:analyze"
)

// 발행 메서드
PublishMailSync(ctx, job) error
PublishMailModify(ctx, job) error
PublishAIClassify(ctx, job) error
PublishRAGIndex(ctx, job) error
PublishProfileAnalyze(ctx, job) error

// 동기화 상태 (Redis Hash)
SetSyncStatus(ctx, connectionID, status) error
GetSyncStatus(ctx, connectionID) (*SyncStatus, error)
IncrementSyncProgress(ctx, connectionID, emailCount) error
```

### Consumer (`consumer.go`)

```go
// Consumer Group 기반 소비
type RedisConsumer struct {
    client    *redis.Client
    group     string
    consumer  string
    handlers  map[string]Handler
}

// 핸들러 등록
RegisterHandler(stream string, handler Handler)

// 소비 시작
Start(ctx) error

// Pending 메시지 처리 (서버 복구 시)
ClaimPendingMessages(ctx, stream, minIdleTime) error
```

---

## Provider (외부 API)

### GmailAdapter (`gmail_adapter.go`)

```go
// 인증
GetAuthURL(state) string
ExchangeToken(ctx, code) (*oauth2.Token, error)
RefreshToken(ctx, token) (*oauth2.Token, error)
ValidateToken(ctx, token) (bool, error)

// 동기화
InitialSync(ctx, token, opts) (*ProviderSyncResult, error)
DeltaSync(ctx, token, historyID) (*ProviderSyncResult, error)

// 메시지 조회
ListMessages(ctx, token, opts) (*ProviderListResult, error)
GetMessage(ctx, token, messageID) (*ProviderMailMessage, error)
GetMessageBatch(ctx, token, messageIDs) ([]*ProviderMailMessage, error)

// 메시지 수정
Send(ctx, token, msg) (*ProviderSendResult, error)
Reply(ctx, token, originalID, msg) (*ProviderSendResult, error)
MarkAsRead(ctx, token, messageID) error
MarkAsUnread(ctx, token, messageID) error
Star(ctx, token, messageID) error
Unstar(ctx, token, messageID) error
Trash(ctx, token, messageID) error
Delete(ctx, token, messageID) error
Archive(ctx, token, messageID) error
ModifyLabels(ctx, token, messageID, add, remove) error

// 배치 수정
BatchModifyLabels(ctx, token, messageIDs, add, remove) error

// Webhook
WatchMailbox(ctx, token, topicName) (*WatchResult, error)
StopWatch(ctx, token) error
GetHistory(ctx, token, historyID, types) (*HistoryResult, error)
```

### Circuit Breaker

```go
// 연속 5회 실패 시 회로 차단
cbSettings := gobreaker.Settings{
    Name:        "gmail-api",
    MaxRequests: 5,
    Interval:    60 * time.Second,
    Timeout:     30 * time.Second,
    ReadyToTrip: func(counts gobreaker.Counts) bool {
        return counts.ConsecutiveFailures > 5
    },
}
```

### Google Calendar Adapter (`google_calendar_adapter.go`)

```go
ListCalendars(ctx, token) ([]*ProviderCalendar, error)
ListEvents(ctx, token, calendarID, opts) ([]*ProviderCalendarEvent, error)
GetEvent(ctx, token, calendarID, eventID) (*ProviderCalendarEvent, error)
CreateEvent(ctx, token, calendarID, event) (*ProviderCalendarEvent, error)
UpdateEvent(ctx, token, calendarID, eventID, event) (*ProviderCalendarEvent, error)
DeleteEvent(ctx, token, calendarID, eventID) error
```

---

## 구현 상태

### 완료

**Provider**:
- [x] GmailAdapter (OAuth, CRUD, InitialSync, BatchModify)
- [x] GoogleCalendarAdapter (CRUD)
- [x] OutlookAdapter (기본 구조)

**Persistence**:
- [x] MailAdapter (CRUD, 배치, 검색)
- [x] OAuthAdapter (토큰 저장/조회, Webhook ID)
- [x] SettingsAdapter (사용자 설정, 분류 규칙)
- [x] LabelAdapter, FolderAdapter
- [x] CalendarAdapter, ContactAdapter
- [x] SenderProfileAdapter
- [x] WebhookAdapter

**MongoDB**:
- [x] MailBodyAdapter (본문 저장, 30일 TTL, gzip 압축)
- [x] ReportAdapter

**Graph (Neo4j)**:
- [x] PersonalizationAdapter (프로필, 스타일, 관계)
- [x] ExtendedPersonalizationStore (관계 변화 추적)

**Messaging**:
- [x] RedisProducer (작업 발행)
- [x] RedisConsumer (작업 소비, Pending 처리)

### 개선 필요

- [ ] GmailAdapter.GetHistory() 최적화 (증분 동기화)
- [ ] PubSubAdapter - Gmail Watch 통합
- [ ] OutlookAdapter 완전 구현
- [ ] SSEAdapter - 실시간 이벤트 브로드캐스트

---

## 환경 변수

```env
# PostgreSQL (Supabase)
DATABASE_URL=postgres://...

# MongoDB
MONGODB_URI=mongodb://...

# Neo4j
NEO4J_URI=bolt://...
NEO4J_USERNAME=neo4j
NEO4J_PASSWORD=...

# Redis
REDIS_URL=redis://...

# Google OAuth
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_PROJECT_ID=...

# Microsoft OAuth
MS_CLIENT_ID=...
MS_CLIENT_SECRET=...
```
