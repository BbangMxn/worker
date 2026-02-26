# Architecture Design Document

## 1. System Overview

Bridgify는 **Hexagonal Architecture (Ports & Adapters)** 패턴을 기반으로 설계된 AI-powered 업무 자동화 플랫폼입니다.

시스템은 크게 두 프로세스로 분리 가능합니다:

| Mode | Role | Entry Point |
|------|------|-------------|
| `api` | HTTP 요청 처리, SSE 실시간 이벤트, Webhook 수신 | `bootstrap.NewAPI()` |
| `worker` | Redis Stream 소비, 비동기 작업 처리, 스케줄링 | `bootstrap.NewWorker()` |
| `all` | API + Worker를 단일 프로세스에서 동시 실행 | `main.go --mode=all` |

```
main.go
  │
  ├─ --mode=api    → bootstrap.NewAPI()    → Fiber HTTP Server
  ├─ --mode=worker → bootstrap.NewWorker() → Redis Stream Consumer + Schedulers
  └─ --mode=all    → Worker(goroutine) + API(main)
```

---

## 2. Hexagonal Architecture

### 2.1 Layer 구조

```
┌─────────────────────────────────────────────┐
│              Adapter (In)                    │
│  HTTP Handlers  │  Worker Processors         │
├─────────────────┴───────────────────────────┤
│              Port (In)                       │
│  UseCase Interfaces                          │
├─────────────────────────────────────────────┤
│              Core                            │
│  Domain Entities  │  Services  │  AI Agent   │
├─────────────────────────────────────────────┤
│              Port (Out)                      │
│  Repository / Provider Interfaces            │
├─────────────────────────────────────────────┤
│              Adapter (Out)                   │
│  PostgreSQL │ MongoDB │ Neo4j │ Redis │ API  │
└─────────────────────────────────────────────┘
```

### 2.2 의존성 규칙

- **Core**는 어떤 외부 패키지에도 의존하지 않음 (stdlib + domain 내부만)
- **Core → Port**: Core는 Port 인터페이스만 참조
- **Adapter → Port**: Adapter는 Port 인터페이스를 구현
- **외부 → Core**: Adapter(In)이 Core의 Service를 호출
- **Core → 외부**: Core의 Service가 Port(Out)을 통해 Adapter(Out)을 호출

### 2.3 Port 인터페이스

**Port In (UseCase)** — 애플리케이션이 제공하는 기능:

| Interface | Methods |
|-----------|---------|
| `EmailUseCase` | ListEmails, GetEmail, SendEmail, ReplyEmail, SearchEmails, ModifyEmail |
| `CalendarUseCase` | ListEvents, CreateEvent, UpdateEvent, DeleteEvent |
| `ContactUseCase` | ListContacts, GetContact, CreateContact, UpdateContact |
| `AIUseCase` | Classify, Summarize, GenerateReply, Chat |
| `AuthUseCase` | ConnectOAuth, DisconnectOAuth, GetConnections |
| `SettingsUseCase` | GetSettings, UpdateSettings, GetClassificationRules |

**Port Out (Repository/Provider)** — 애플리케이션이 필요로 하는 외부 기능:

| Interface | Implementation |
|-----------|---------------|
| `EmailRepositoryPort` | PostgreSQL (emails 테이블) |
| `EmailBodyRepositoryPort` | MongoDB (email_bodies 컬렉션) |
| `EmailProviderPort` | Gmail API / Outlook API |
| `CalendarProviderPort` | Google Calendar API |
| `LabelRepositoryPort` | PostgreSQL (labels 테이블) |
| `OAuthRepositoryPort` | PostgreSQL (oauth_connections) |
| `SettingsRepositoryPort` | PostgreSQL (settings) |
| `ClassificationRuleRepo` | PostgreSQL (classification_rules) |
| `SenderProfileRepo` | PostgreSQL (sender_profiles) |
| `KnownDomainRepo` | PostgreSQL (known_domains) |
| `EmbeddingRepositoryPort` | pgvector (email_embeddings) |
| `GraphRepositoryPort` | Neo4j (writing style, contacts) |
| `MessageQueuePort` | Redis Stream |
| `CachePort` | Redis / In-memory L1 |
| `SSEPort` | SSE Hub (realtime broadcast) |

---

## 3. AI Agent Architecture

### 3.1 Orchestrator

`Orchestrator`는 AI Agent의 중앙 브레인으로, 사용자의 자연어 요청을 처리합니다.

```
User Message
    │
    ▼
┌─────────────────┐
│ detectIntent()  │  LLM에게 사용자 의도 파악 요청
│                 │  → IntentType: query | action | analysis | chat
│                 │  → ToolCalls: [{name, args}, ...]
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ gatherContext() │  RAG를 통한 컨텍스트 수집 (필요시)
│                 │  → pgvector: 유사 이메일 검색
│                 │  → Neo4j: 문체, 관계 패턴
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ toolRegistry    │  Function Calling 도구 실행
│ .Execute()      │  → 읽기 작업: 즉시 결과 반환
│                 │  → 쓰기 작업: ActionProposal 생성
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ generateResponse│  LLM으로 최종 응답 생성
│ ()              │  → tool results + context + 대화 히스토리 종합
└────────┬────────┘
         │
         ▼
    AgentResponse
    ├── message: string
    ├── proposals: []ActionProposal  (확인 대기 중인 작업)
    └── suggestions: []string        (후속 질문 제안)
```

### 3.2 Proposal Safety Pattern

파괴적(mutating) 작업은 절대 즉시 실행하지 않습니다:

```go
type ActionProposal struct {
    ID          string         // UUID
    Type        string         // "email.send", "calendar.create" 등
    Description string         // 사람이 읽을 수 있는 설명
    Data        map[string]any // 실행에 필요한 파라미터
    ExpiresAt   time.Time      // 10분 후 만료
}
```

**Flow:**
1. Tool이 쓰기 작업 감지 → `ActionProposal` 생성
2. Proposal을 `ProposalStore`에 저장 (in-memory, 10분 TTL)
3. 클라이언트에 Proposal 반환
4. 사용자가 `POST /ai/proposals/:id/confirm` → `executeProposal()` 실행
5. 사용자가 `POST /ai/proposals/:id/reject` → Proposal 삭제

### 3.3 Tool Registry

OpenAI Function Calling 형식의 도구 정의:

| Tool | Type | Actions |
|------|------|---------|
| `email_list` | query | 이메일 목록 조회 |
| `email_read` | query | 이메일 상세 읽기 |
| `email_search` | query | 이메일 검색 |
| `email_send` | action (proposal) | 이메일 전송 |
| `email_reply` | action (proposal) | 이메일 답장 |
| `email_delete` | action (proposal) | 이메일 삭제 |
| `email_archive` | action (proposal) | 이메일 보관 |
| `email_mark_read` | action | 읽음 처리 |
| `email_star` | action | 별표 |
| `email_translate` | query | 이메일 번역 |
| `email_summarize` | query | 이메일 요약 |
| `calendar_list` | query | 일정 조회 |
| `calendar_create` | action (proposal) | 일정 생성 |
| `calendar_update` | action (proposal) | 일정 수정 |
| `calendar_delete` | action (proposal) | 일정 삭제 |
| `calendar_find_free` | query | 빈 시간 검색 |

### 3.4 Session Management

대화 세션은 in-memory로 관리됩니다:

```go
type Session struct {
    ID        string
    UserID    uuid.UUID
    Messages  []Message      // 대화 히스토리
    Context   map[string]any // 세션 컨텍스트
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

---

## 4. Classification Pipeline Architecture

### 4.1 Score-Based Classification (v3.12.0)

각 Stage의 분류기는 `ScoreClassifierResult`를 반환합니다:

```go
type ScoreClassifierResult struct {
    Category    EmailCategory
    SubCategory EmailSubCategory
    Priority    Priority       // 0.0 ~ 1.0
    Confidence  float64        // 분류 확신도
    Source      string         // "rfc", "domain", "subject", "user", "llm"
    Tags        []string
}
```

### 4.2 RFC Score Classifier

40+ 개별 분류기가 RFC 이메일 헤더를 분석합니다:

```
Headers 입력
    │
    ├─ List-Unsubscribe 존재? → newsletter/marketing (0.95 confidence)
    ├─ Precedence: bulk?     → marketing (0.90)
    ├─ Auto-Submitted: yes?  → notification (0.85)
    ├─ X-Mailer: Mailchimp?  → marketing (0.95)
    ├─ From: *@github.com?   → developer/notification
    ├─ X-GitHub-Reason?      → developer (pr_review, issue, ci_cd)
    ├─ From: *@jira.*.com?   → developer/issue
    ├─ From: *@slack.com?    → notification/social
    └─ ... (40+ more rules)
```

### 4.3 Pipeline Fallthrough

각 Stage는 독립적으로 실행되며, 결과가 `nil`이면 다음 Stage로 넘어갑니다:

```go
func (p *Pipeline) Classify(ctx context.Context, input *ClassifyInput) (*Result, error) {
    // Stage 0: RFC → 결과 있으면 즉시 반환
    if result, err := p.rfcScoreClassifier.Classify(ctx, input); result != nil {
        return result, nil
    }
    // Stage 1: Domain → 결과 있으면 반환
    if result, err := p.domainScoreClassifier.Classify(ctx, input); result != nil {
        return result, nil
    }
    // ... Stage 2-5 ...
    // Stage 6: LLM (마지막 수단)
    return p.classifyByLLMWithUserRules(ctx, input)
}
```

---

## 5. Real-time Sync Architecture

### 5.1 Gmail Push Notification Flow

```
1. OAuth 연결 시:
   POST /api/v1/oauth/connect/google
     → OAuth2 토큰 획득
     → Redis Job: email.watch_setup 발행

2. Worker가 Watch 설정:
   gmail.users.watch() API 호출
     → Gmail이 Pub/Sub Topic에 등록
     → historyId 저장 (sync_states 테이블)

3. 이메일 수신 시:
   Gmail → Google Pub/Sub → POST /webhook/gmail
     → historyId 비교
     → Redis Job: email.delta_sync 발행

4. Delta Sync 실행:
   Worker가 gmail.users.history.list() 호출
     → 변경된 메시지만 가져옴
     → PostgreSQL 업데이트
     → MongoDB에 본문 저장
     → Redis Job: ai.classify 발행
     → SSE Hub: email.new 이벤트 브로드캐스트
```

### 5.2 Schedulers

| Scheduler | Interval | Description |
|-----------|----------|-------------|
| Watch Renew | 매 6시간 | Gmail Watch 7일 만료 전 자동 갱신 |
| Sync Retry | 매 1분 | 실패한 동기화 작업 재시도 |
| Gap Sync | 서버 시작 시 1회 | 서버 다운 동안 놓친 이메일 보정 |

---

## 6. Multi-Tier Caching

```
요청 → L1 Cache (in-memory, 30s TTL, 1000 entries)
         │ miss
         ▼
       L2 Cache (Redis, 1min TTL)
         │ miss
         ▼
       Database (PostgreSQL / MongoDB)
         │
         ▼
       결과를 L1 + L2에 캐싱
```

| Tier | Storage | TTL | Max Entries | Use Case |
|------|---------|-----|-------------|----------|
| L1 | In-process memory | 30초 | 1,000 | 이메일 목록, 핫 데이터 |
| L2 | Redis | 1분 ~ 24시간 | 무제한 | 이메일 목록, 세션 |
| Embedding Cache | In-process | 영구 (LRU eviction) | - | OpenAI 임베딩 중복 호출 방지 |

---

## 7. Dependency Injection

Go에는 DI 프레임워크가 없으므로 `internal/bootstrap/worker_deps.go`에서 수동으로 모든 의존성을 조립합니다:

```
1. Config 로드
2. Database 연결 (PostgreSQL, MongoDB, Redis, Neo4j)
3. Repository 생성 (Port Out 구현체)
4. Provider 생성 (Gmail, Outlook API 어댑터)
5. LLM Client 생성 (OpenAI)
6. RAG Components 생성 (VectorStore, Retriever, StyleAnalyzer)
7. Tool Registry 생성 (Function Calling 도구)
8. Service 생성 (Port In 구현체)
9. Orchestrator 생성 (AI Agent 브레인)
10. Handler 생성 (HTTP 핸들러)
11. Worker 생성 (프로세서, 스케줄러)
```

이 순서는 의존성 그래프의 위상 정렬(topological sort)입니다. 각 단계는 이전 단계에서 생성된 객체만 참조합니다.

---

## 8. Error Handling & Resilience

### Circuit Breaker

외부 API 호출(Gmail, OpenAI)에 `sony/gobreaker` Circuit Breaker를 적용:

```
Closed (정상) → 오류 5회 연속 → Open (차단) → 30초 후 → Half-Open (시험)
                                                             │
                                                    성공 → Closed
                                                    실패 → Open
```

### Graceful Shutdown

```go
// 30초 타임아웃으로 graceful shutdown
const shutdownTimeout = 30 * time.Second

// SIGINT, SIGTERM 수신 시:
// 1. 새 요청 거부
// 2. 진행 중인 요청 완료 대기
// 3. Worker Pool 드레인
// 4. DB 연결 정리
// 5. 타임아웃 시 강제 종료
```

### Rate Limiting

- API 전체: per-IP rate limiting
- LLM 호출: per-user rate limiting
- Gmail API: OAuth 토큰 기반 quota 관리

---

## 9. Frontend Architecture (FSD Pattern)

Feature Sliced Design에서 영감을 받은 레이어 구조:

```
app/          → Next.js App Router (라우팅, 레이아웃)
  │
widgets/      → 복합 UI 위젯 (비즈니스 로직 포함)
  │               EmailList, EmailDetail, ComposeModal,
  │               CalendarView, Sidebar, CommandPalette
  │
entities/     → 도메인 엔티티 (타입 정의, 목업 데이터, 개별 카드)
  │               email/, calendar/, contact/, document/
  │
shared/       → 재사용 가능한 공통 레이어
                  ui/     → Avatar, Button, IconButton, Input, Skeleton
                  hooks/  → useGlobalShortcuts
                  lib/    → utils
                  types/  → api_types
                  config/ → worker_theme
```

**의존성 규칙:** `app → widgets → entities → shared` (상위에서 하위만 참조)
