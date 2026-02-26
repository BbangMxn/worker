# AI Agent Layer

> **핵심**: AI 기반 업무 자동화 - LLM을 활용한 의도 분석, 이메일 분류/요약/답장, RAG 기반 스타일 학습

---

## 디렉토리 구조

```
core/agent/
├── orchestrator.go          # 중앙 AI Agent 오케스트레이터
├── orchestrator_test.go     
│
├── entity/
│   └── agent.go             # Agent 엔티티 정의
│
├── llm/                     # LLM 클라이언트 (OpenAI)
│   ├── client.go            # OpenAI API 클라이언트 (Complete, Stream, Embedding)
│   ├── optimized_client.go  # 최적화된 클라이언트 (캐싱, 배치)
│   ├── optimizer.go         # 프롬프트 캐싱, 토큰 최적화
│   ├── classify.go          # 이메일 분류 (Category, Priority, Tags)
│   ├── summarize.go         # 이메일/스레드 요약
│   ├── reply.go             # RAG 기반 답장 생성
│   ├── intent.go            # 의도 분석
│   ├── extractor.go         # 미팅/연락처 정보 추출
│   ├── batch.go             # 배치 처리
│   └── cost.go              # 비용 추적
│
├── rag/                     # RAG 시스템 (pgvector + Neo4j)
│   ├── embedder.go          # OpenAI Ada Embedding
│   ├── vectorstore.go       # pgvector 저장소
│   ├── indexer.go           # 이메일 인덱싱 (발송/수신)
│   ├── retriever.go         # 시맨틱 검색
│   ├── ranker.go            # 결과 랭킹
│   ├── style_analyzer.go    # 작문 스타일 분석 (Neo4j 저장)
│   └── cache.go             # 임베딩 캐시
│
├── tools/                   # 도구 시스템 (Function Calling)
│   ├── types.go             # Tool 인터페이스, ToolResult, ActionProposal
│   ├── registry.go          # 도구 레지스트리
│   ├── executor.go          # 도구 실행기
│   ├── mail.go              # 메일 도구 (list, read, search, send, reply)
│   ├── calendar.go          # 캘린더 도구 (list, create, find_free)
│   ├── contact.go           # 연락처 도구
│   └── search.go            # 검색 도구 (시맨틱)
│
├── session/
│   └── session.go           # 대화 세션 관리, Proposal 저장소
│
└── service/
    └── agent.go             # Agent 서비스 인터페이스
```

---

## Orchestrator (핵심)

### 처리 플로우

```
사용자 메시지 → Process()
    │
    ├─→ 1. detectIntent()         # 의도 분석 (LLM JSON 응답)
    │       ├─→ type: query/action/analysis/chat
    │       ├─→ category: mail/calendar/contact/search
    │       └─→ tool_calls: 필요한 도구 목록
    │
    ├─→ 2. gatherContext()        # RAG 컨텍스트 검색
    │       └─→ RetrieveForContext() / RetrieveForStyle()
    │
    ├─→ 3. Execute Tools          # 도구 실행
    │       ├─→ 조회 도구 → 즉시 실행
    │       └─→ 수정 도구 → Proposal 생성 (사용자 확인 필요)
    │
    └─→ 4. generateResponse()     # LLM 응답 생성
            └─→ 컨텍스트 + 도구 결과 → 자연어 응답
```

### Proposal 기반 액션

수정 작업(send, reply, create, delete)은 직접 실행하지 않고 **Proposal**을 생성:

```go
// Proposal 생성 → 사용자 확인 대기
result.Proposal = &ActionProposal{
    ID:          uuid.New().String(),
    Action:      "mail.send",
    Description: "Send email to john@example.com",
    Data:        map[string]any{...},
    ExpiresAt:   time.Now().Add(10 * time.Minute),
}

// 사용자 확인 후 실행
ConfirmProposal(ctx, userID, proposalID) → executeProposal()
```

### 지원 Proposal 액션

| 액션 | 설명 |
|------|------|
| `mail.send` | 이메일 전송 |
| `mail.reply` | 이메일 답장 |
| `mail.delete` | 이메일 삭제 (휴지통 또는 영구) |
| `mail.archive` | 이메일 보관 |
| `mail.mark_read` | 읽음/안읽음 표시 |
| `mail.star` | 별표 표시 |
| `calendar.create` | 일정 생성 |
| `calendar.update` | 일정 수정 |
| `calendar.delete` | 일정 삭제 |
| `label.add` | 라벨 추가 |
| `label.remove` | 라벨 제거 |
| `label.create` | 라벨 생성 |

---

## LLM 클라이언트

### 기본 설정

```go
// 기본 모델: gpt-4o-mini
// MaxTokens: 2048
// Temperature: 0.7

client := llm.NewClient(apiKey)
client := llm.NewClientWithConfig(ClientConfig{
    Model:       "gpt-4o",
    MaxTokens:   4096,
    Temperature: 0.3,
})
```

### 주요 메서드

| 메서드 | 설명 |
|--------|------|
| `Complete(prompt)` | 단순 텍스트 완성 |
| `CompleteWithSystem(system, user)` | 시스템 프롬프트 포함 |
| `CompleteJSON(prompt)` | JSON 응답 강제 |
| `CompleteWithTools(system, user, tools)` | Function Calling |
| `Stream(prompt, handler)` | 스트리밍 응답 |
| `Embedding(text)` | 단일 임베딩 |
| `EmbeddingBatch(texts)` | 배치 임베딩 |

### 이메일 분류 (classify.go)

**레거시 분류** (`ClassifyEmail`):
- Categories: primary, social, promotion, updates, forums

**향상된 분류** (`ClassifyEmailEnhanced`):
- Categories: primary, work, personal, newsletter, notification, marketing, social, finance, travel, shopping, spam, other
- Sub-categories: receipt, invoice, shipping, order, travel, calendar, account, security, sns, comment, newsletter, marketing, deal

```go
result, err := client.ClassifyEmailEnhanced(ctx, email, body)
// result.Category = "finance"
// result.SubCategory = "receipt"
// result.Priority = 2
// result.Summary = "Amazon 구매 영수증"
// result.Tags = ["amazon", "purchase", "receipt"]
```

---

## RAG 시스템

### 저장소 분리

| 저장소 | 용도 |
|--------|------|
| **pgvector** | 이메일 임베딩 벡터 |
| **Neo4j** | 사용자 개인화 (스타일, 관계) |

### Indexer (indexer.go)

```go
// 단일 이메일 인덱싱
indexer.IndexEmail(ctx, &EmailIndexRequest{
    EmailID:   email.ID,
    UserID:    email.UserID,
    Subject:   email.Subject,
    Body:      body,
    FromEmail: email.FromEmail,
    Direction: "inbound",  // 또는 "outbound"
    Folder:    email.Folder,
})

// 배치 인덱싱
indexer.IndexBatch(ctx, requests)
```

### StyleAnalyzer (style_analyzer.go)

발송 이메일을 분석하여 **Neo4j**에 저장:

```go
result, err := analyzer.AnalyzeSentEmail(ctx, &AnalysisInput{
    UserID:         userID,
    EmailID:        email.ID,
    Subject:        subject,
    Body:           body,
    RecipientEmail: "john@example.com",
    SentAt:         time.Now(),
})
```

**분석 항목**:
- 평균 문장 길이
- 격식 점수 (0~1)
- 이모지 빈도
- 인사말/맺음말 패턴
- 자주 사용하는 문구
- 연락처별 관계 유형 (colleague, boss, client, vendor)
- 관계 변화 추적 (승진, 이동 등)

### Retriever (retriever.go)

```go
// 컨텍스트 검색 (수신 이메일)
results, err := retriever.RetrieveForContext(ctx, userID, query, limit)

// 스타일 검색 (발송 이메일)
results, err := retriever.RetrieveForStyle(ctx, userID, query, limit)
```

---

## 도구 시스템

### Tool 인터페이스

```go
type Tool interface {
    Name() string                     // "mail.list"
    Description() string
    Category() ToolCategory           // mail, calendar, contact, search
    Parameters() []ParameterSpec
    Execute(ctx, userID, args) (*ToolResult, error)
}
```

### 등록된 도구

| 도구 | 설명 | Proposal |
|------|------|----------|
| `mail.list` | 이메일 목록 조회 | No |
| `mail.read` | 이메일 상세 조회 | No |
| `mail.search` | 이메일 검색 | No |
| `mail.send` | 이메일 전송 | **Yes** |
| `mail.reply` | 이메일 답장 | **Yes** |
| `calendar.list` | 일정 목록 조회 | No |
| `calendar.create` | 일정 생성 | **Yes** |
| `calendar.find_free` | 빈 시간 찾기 | No |
| `contact.list` | 연락처 목록 | No |
| `contact.get` | 연락처 상세 | No |
| `contact.search` | 연락처 검색 | No |
| `search.email` | 시맨틱 이메일 검색 | No |
| `search.calendar` | 일정 검색 | No |
| `search.contact` | 연락처 검색 | No |

### ToolResult 구조

```go
type ToolResult struct {
    Success  bool
    Data     any              // 조회 결과
    Message  string           // 성공 메시지
    Error    string           // 오류 메시지
    Proposal *ActionProposal  // 수정 작업일 경우
}
```

---

## 의존성

### 필수 의존성

```go
type Orchestrator struct {
    llmClient      *llm.Client           // OpenAI API
    ragRetriever   *rag.Retriever        // 시맨틱 검색
    toolRegistry   *tools.Registry       // 도구 실행
    proposalStore  *session.ProposalStore
    sessionManager *session.Manager
}
```

### 선택 의존성 (Proposal 실행용)

```go
mailProvider     out.MailProviderPort      // Gmail/Outlook API
calendarProvider out.CalendarProviderPort  // Calendar API
oauthProvider    OAuthTokenProvider        // OAuth 토큰 관리
labelRepo        domain.LabelRepository    // 라벨 CRUD
```

---

## 사용 예시

### 기본 처리

```go
orchestrator := agent.NewOrchestrator(llmClient, retriever, registry)
orchestrator.SetMailProvider(mailProvider)
orchestrator.SetOAuthProvider(oauthProvider)

response, err := orchestrator.Process(ctx, &AgentRequest{
    UserID:    userID,
    SessionID: "session-123",
    Message:   "지난주에 John이 보낸 이메일 보여줘",
})
// response.Message = "지난주 John으로부터 3개의 이메일이 있습니다..."
// response.Data = [...emails...]
```

### 답장 생성

```go
reply, err := orchestrator.GenerateReply(ctx, userID, originalEmail, body, "professional")
// RAG에서 사용자 스타일 학습 → 스타일에 맞는 답장 생성
```

### 이메일 분류

```go
result, err := orchestrator.ClassifyEmail(ctx, email, body, userRules)
// result.Category = "work"
// result.Priority = 3
```

---

## 구현 상태

### 완료

- [x] Orchestrator 기본 흐름 (Process, ConfirmProposal, RejectProposal)
- [x] LLM Client (OpenAI) - Complete, Stream, Embedding
- [x] Intent Detection (JSON 응답)
- [x] Tool Registry & Executor
- [x] Mail/Calendar/Contact/Search Tools
- [x] RAG Embedder, Indexer, Retriever
- [x] Proposal 시스템 (전체 액션 지원)
- [x] StyleAnalyzer (Neo4j 연동)
- [x] 이메일 분류 (레거시 + 향상된 버전)

### 개선 필요

- [ ] 스트리밍 응답 개선 (Token 단위, 중간 상태)
- [ ] RAG 인덱싱 속도 최적화 (배치 크기 조정)
- [ ] 프롬프트 캐싱 TTL 최적화
- [ ] 다국어 의도 분석 개선

---

## 환경 변수

```env
OPENAI_API_KEY=sk-...    # OpenAI API 키
```

---

## 참고

- 모든 수정 작업은 **Proposal 확인 후 실행** (안전)
- RAG 임베딩: `text-embedding-ada-002` (1536차원)
- Neo4j: 스타일/관계 데이터만 저장 (벡터는 pgvector)
- Proposal 만료: 10분
