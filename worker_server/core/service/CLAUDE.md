# Service Layer

> **핵심**: UseCase(Port In) 구현, 비즈니스 로직 오케스트레이션, 트랜잭션 경계 관리

---

## 디렉토리 구조

```
core/service/
├── CLAUDE.md
├── template.go              # 이메일 템플릿 렌더링
│
├── ai/                      # AI 서비스
│   ├── service.go           # 분류, 요약, 답장 생성, 채팅
│   ├── optimized.go         # 비용 최적화 (캐싱, 배치)
│   └── personalization.go   # 개인화 서비스 (Neo4j)
│
├── auth/                    # 인증/OAuth
│   ├── oauth.go             # OAuth 토큰 관리 (Google, Outlook)
│   └── settings.go          # 사용자 설정 관리
│
├── calendar/                # 캘린더
│   ├── service.go           # 일정 CRUD
│   └── sync.go              # Provider 동기화
│
├── classification/          # 4단계 분류 파이프라인
│   └── pipeline.go          # Stage 0~3 분류
│
├── common/                  # 공용 유틸리티
│   ├── cache.go             # Redis 캐시 서비스
│   ├── cache_l1.go          # L1 인메모리 캐시
│   └── errors.go            # 공용 에러 정의
│
├── contact/                 # 연락처
│   └── service.go           # 연락처 CRUD
│
├── mail/                    # 메일
│   ├── service.go           # 메일 CRUD, 배치 작업
│   ├── sync.go              # Provider 동기화
│   └── modifier.go          # Gmail/Outlook 라벨 수정
│
├── notification/            # 알림
│   ├── service.go           # 알림 서비스
│   └── webhook.go           # Gmail Push Webhook
│
└── report/                  # 리포트
    └── service.go           # 일간/주간 리포트 생성
```

---

## Mail Service (`mail/service.go`)

### 주요 기능

```go
// 조회
GetEmail(ctx, userID, emailID) (*Email, error)
ListEmails(ctx, filter) ([]*Email, int, error)
ListEmailsHybrid(ctx, filter) ([]*Email, int, hasMore, error)  // DB + API 하이브리드
GetEmailBody(ctx, emailID) (*EmailBody, error)

// 상태 변경 (배치 지원)
MarkAsRead(ctx, userID, emailIDs)
MarkAsUnread(ctx, userID, emailIDs)
Star(ctx, userID, emailIDs)
Unstar(ctx, userID, emailIDs)
Archive(ctx, userID, emailIDs)
Trash(ctx, userID, emailIDs)
Delete(ctx, userID, emailIDs)
MoveToFolder(ctx, userID, emailIDs, folder)
Snooze(ctx, userID, emailIDs, until)
Unsnooze(ctx, userID, emailIDs)

// 라벨 (배치 지원)
BatchAddLabels(ctx, userID, emailIDs, labels)
BatchRemoveLabels(ctx, userID, emailIDs, labels)
AddLabels(ctx, userID, emailID, labelIDs)
RemoveLabels(ctx, userID, emailID, labelIDs)

// 발송
SendEmail(ctx, userID, req) (*Email, error)
ReplyEmail(ctx, userID, emailID, req) (*Email, error)
ForwardEmail(ctx, userID, emailID, req) (*Email, error)
```

### 하이브리드 로딩

DB에 부족하면 Gmail API에서 추가 로딩:

```go
// 1. DB에서 먼저 조회
emails, total, _ := s.emailRepo.List(filter)

// 2. 요청한 개수보다 적으면 API에서 추가 로딩
if len(emails) < filter.Limit {
    moreEmails, hasMore, _ := s.fetchFromProvider(...)
    emails = append(emails, moreEmails...)
}
```

### 배치 처리

모든 수정 작업은 배치 쿼리 + 동시성 제어:

```go
// 1. 배치 쿼리 시도 (최적화 경로)
if err := s.mailRepo.BatchUpdateReadStatus(ctx, emailIDs, true); err == nil {
    s.invalidateEmailCache(ctx, userID, emailIDs)
    s.publishMailModifyJob(ctx, userID, emailIDs, "read")
    return nil
}

// 2. 폴백: 개별 업데이트 (동시성 10)
s.batchUpdateEmails(ctx, userID, emailIDs, func(email *Email) {
    email.IsRead = true
})
```

### 비동기 Provider 동기화

DB 업데이트 후 Redis Stream으로 Provider 동기화 Job 발행:

```go
// DB 업데이트 성공 후
s.messageProducer.PublishMailModify(ctx, &MailModifyJob{
    UserID:       userID,
    ConnectionID: connectionID,
    Action:       "read",
    ExternalIDs:  providerIDs,
    AddLabels:    []string{},
    RemoveLabels: []string{"UNREAD"},
})
```

---

## Classification Pipeline (`classification/pipeline.go`)

### 4단계 분류 (~75% LLM 비용 절감)

```
Stage 0: User Rules    (10%) - ImportantDomains, Keywords, IgnoreSenders
Stage 1: Headers       (35%) - List-Unsubscribe, Precedence, X-Campaign
Stage 2: Known Domain  (30%) - SenderProfile, KnownDomain DB
Stage 3: LLM           (25%) - 나머지만 LLM 호출
```

### Stage 0: User Rules

```go
// 사용자 정의 규칙 (최우선)
rules.ImportantDomains  → Category: work, Priority: high
rules.ImportantKeywords → Category: work, Priority: high
rules.IgnoreSenders     → Category: other, Priority: low
rules.IgnoreKeywords    → Category: other, Priority: low
```

### Stage 1: Header Rules

```go
// 헤더 기반 분류
List-Unsubscribe → newsletter
Precedence: bulk → marketing
X-Mailchimp-ID   → marketing
X-Campaign       → marketing
```

### Stage 2: Domain Matching

```go
// 1. SenderProfile (사용자별 학습)
profile, _ := senderProfileRepo.GetByEmail(userID, fromEmail)
if profile.LearnedCategory != nil {
    return profile.LearnedCategory
}

// 2. KnownDomain (글로벌)
knownDomain, _ := knownDomainRepo.GetByDomain(domain)
return knownDomain.Category
```

### Stage 3: LLM (Fallback)

```go
// 위 3단계로 분류 안 된 경우만 LLM 호출
result, _ := llmClient.ClassifyEmailEnhanced(ctx, email, body)
```

---

## AI Service (`ai/service.go`)

### 주요 기능

| 메서드 | 설명 | 최적화 |
|--------|------|--------|
| `ClassifyEmail` | 4단계 파이프라인 분류 | 75% LLM 비용 절감 |
| `ClassifyEmailBatch` | 배치 분류 (동시성 5) | 세마포어 제한 |
| `SummarizeEmail` | 이메일 요약 | 200자 미만 스킵 |
| `SummarizeThread` | 스레드 요약 | - |
| `GenerateReply` | RAG 기반 답장 생성 | 스타일 학습 |
| `ExtractMeetingInfo` | 미팅 정보 추출 | 키워드 없으면 스킵 |
| `Chat` | RAG 컨텍스트 채팅 | 도구 실행 포함 |
| `ChatStream` | 스트리밍 채팅 | - |

### 비용 최적화

```go
// 1. 요약: 200자 미만은 API 호출 생략
if !force && contentLength < 200 {
    return body, nil  // 본문 그대로 반환
}

// 2. 미팅 추출: 키워드 없으면 스킵
if !containsMeetingKeyword(subject, body) {
    return &MeetingInfo{HasMeeting: false}, nil
}

// 3. 분류: 4단계 파이프라인으로 75% 절감
```

### RAG 연동

```go
// 답장 생성 시 발송 이메일에서 스타일 학습
styleContext := ""
results, _ := ragRetriever.RetrieveForStyle(ctx, userID, query, 3)
for _, r := range results {
    styleContext += r.Content + "\n"
}

reply, _ := llmClient.GenerateReply(ctx, subject, body, from, styleContext, options)
```

---

## OAuth Service (`auth/oauth.go`)

### 주요 기능

```go
// 인증 플로우
GetAuthURL(ctx, provider, state) (string, error)
HandleCallback(ctx, provider, code, userID) (*OAuthConnection, error)

// 연결 관리
GetConnection(ctx, connectionID) (*OAuthConnection, error)
GetConnectionsByUser(ctx, userID) ([]*OAuthConnection, error)
GetConnectionByUserID(ctx, userID, provider) (*OAuthConnection, error)
Disconnect(ctx, connectionID) error

// 토큰 관리
RefreshToken(ctx, connectionID) error
GetValidToken(ctx, connectionID) (string, error)
GetOAuth2Token(ctx, connectionID) (*oauth2.Token, error)

// Webhook
GetConnectionByWebhookID(ctx, subscriptionID, provider) (*OAuthConnection, error)
ListAllActiveConnections(ctx) ([]*OAuthConnection, error)
```

### OAuth 콜백 처리

```go
// 1. 토큰 교환
token, _ := googleConfig.Exchange(ctx, code)

// 2. 사용자 이메일 조회
email, _ := s.getGoogleEmail(ctx, token)

// 3. DB 저장/업데이트
s.oauthRepo.Create(ctx, entity)

// 4. 초기 동기화 Job 발행
s.messageProducer.PublishMailSync(ctx, &MailSyncJob{
    ConnectionID: conn.ID,
    FullSync:     true,
})

// 5. Webhook 설정 (비동기)
go s.webhookSetup(ctx, conn.ID)
```

### 토큰 자동 갱신

```go
// 만료 5분 전에 자동 갱신
if time.Until(conn.ExpiresAt) < 5*time.Minute {
    s.RefreshToken(ctx, connectionID)
}
```

---

## Common (`common/`)

### CacheService

```go
// L1 (인메모리) + L2 (Redis) 2계층 캐시
cache := NewCacheService(redisClient, "workspace")

// 이메일 목록 캐시
cache.GetList(ctx, userID, folder) ([]*Email, error)
cache.SetList(ctx, userID, folder, emails, ttl)
cache.InvalidateList(ctx, userID, folder)

// 단일 이메일 캐시
cache.GetEmail(ctx, emailID) (*Email, error)
cache.SetEmail(ctx, emailID, email, ttl)
cache.InvalidateEmail(ctx, emailID)
```

### 공용 에러

```go
var (
    ErrNotFound     = errors.New("not found")
    ErrForbidden    = errors.New("forbidden")
    ErrUnauthorized = errors.New("unauthorized")
    ErrBadRequest   = errors.New("bad request")
)
```

---

## Notification Service (`notification/`)

### Webhook 처리

```go
// Gmail Push Notification 처리
func (s *WebhookService) HandleGmailPush(ctx context.Context, notification *GmailPushNotification) error {
    // 1. historyId로 변경사항 조회
    // 2. 변경된 메일 동기화
    // 3. SSE 브로드캐스트
}
```

---

## 의존성 주입

### MailService

```go
NewServiceFull(
    emailRepo,       // domain.EmailRepository
    labelRepo,       // domain.LabelRepository
    mailRepo,        // out.MailRepository (배치 작업)
    cacheService,    // *common.CacheService
    provider,        // out.MailProviderPort (Gmail/Outlook API)
    oauthService,    // *auth.OAuthService
    messageProducer, // out.MessageProducer (Redis Stream)
)
```

### AIService

```go
NewService(
    emailRepo,     // domain.EmailRepository
    settingsRepo,  // domain.SettingsRepository
    llmClient,     // *llm.Client
    ragRetriever,  // *rag.Retriever
    ragIndexer,    // *rag.IndexerService
    toolRegistry,  // *tools.Registry
)
// + SetClassificationPipeline(pipeline)
```

---

## 구현 상태

### 완료

- [x] MailService - CRUD, 배치 작업, Provider 동기화
- [x] AIService - 분류, 요약, 답장, 채팅
- [x] OAuthService - Google OAuth, 토큰 갱신
- [x] ClassificationPipeline - 4단계 분류
- [x] CacheService - L1/L2 캐시
- [x] WebhookService - Gmail Push

### 개선 필요

- [ ] Outlook OAuth 완전 지원
- [ ] Mail Sync Delta (History API)
- [ ] Real-time SSE 브로드캐스트
- [ ] Report 스케줄러

---

## 환경 변수

```env
# Google OAuth
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...
GOOGLE_REDIRECT_URL=...

# Microsoft OAuth (TODO)
MS_CLIENT_ID=...
MS_CLIENT_SECRET=...
MS_REDIRECT_URL=...

# Redis (캐시)
REDIS_URL=redis://...

# OpenAI (AI 서비스)
OPENAI_API_KEY=...
```
