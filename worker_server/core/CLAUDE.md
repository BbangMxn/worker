# Core Layer

## 목적

비즈니스 로직의 중심. 외부 의존성 없이 순수한 도메인 로직만 포함.

## 설계 방향

- **의존성 역전**: 외부(DB, API)에 의존하지 않음
- **Port 인터페이스**: in(UseCase), out(Repository)으로 경계 정의
- **Service**: Port를 조합하여 UseCase 구현
- **Domain**: 순수 비즈니스 모델

```
core/
├── domain/      # 도메인 모델 (Entity, Value Object)
├── port/
│   ├── in/      # UseCase 인터페이스
│   └── out/     # Repository 인터페이스
├── service/     # UseCase 구현
└── agent/       # AI Agent 시스템
```

---

## AI Agent 아키텍처

### 개요

AI Agent는 사용자의 자연어 요청을 처리하여 메일, 캘린더, 연락처 작업을 자동화합니다.
Neo4j 그래프 DB를 활용하여 사용자의 말투, 커뮤니케이션 패턴, 연락처 관계를 학습합니다.

### 디렉토리 구조

```
core/agent/
├── orchestrator.go          # 중앙 AI 오케스트레이터
├── orchestrator_test.go
│
├── llm/                     # LLM 클라이언트
│   ├── client.go            # OpenAI API (Complete, Stream, Embedding)
│   ├── classify.go          # 이메일 분류
│   ├── summarize.go         # 요약 생성
│   ├── reply.go             # 답장 생성
│   ├── extractor.go         # 미팅/연락처 정보 추출
│   ├── intent.go            # 의도 분석
│   ├── optimizer.go         # 프롬프트 캐싱
│   └── cost.go              # 비용 추적
│
├── rag/                     # RAG 시스템
│   ├── embedder.go          # 임베딩 생성 (OpenAI text-embedding)
│   ├── indexer.go           # 이메일 인덱싱
│   ├── retriever.go         # 컨텍스트 검색
│   ├── vectorstore.go       # pgvector 저장소
│   ├── style_analyzer.go    # 말투/스타일 분석기 (NEW)
│   ├── ranker.go            # 결과 랭킹
│   └── cache.go             # 임베딩 캐시
│
├── session/                 # 대화 세션 관리
│   └── session.go
│
├── service/                 # Agent 서비스
│   └── agent.go
│
├── entity/                  # Agent 도메인 엔티티
│   └── agent.go
│
└── tools/                   # Tool System
    ├── types.go             # Tool 인터페이스, ActionProposal
    ├── registry.go          # 도구 레지스트리
    ├── executor.go          # 도구 실행기
    ├── mail.go              # 메일 도구
    ├── calendar.go          # 캘린더 도구
    ├── contact.go           # 연락처 도구
    └── search.go            # 검색 도구
```

### 핵심 컴포넌트

#### 1. Orchestrator (orchestrator.go)

모든 AI 요청을 처리하는 중앙 컨트롤러.

```go
type Orchestrator struct {
    llmClient     *llm.Client
    ragRetriever  *rag.Retriever
    toolRegistry  *tools.Registry
    proposalStore *ProposalStore    // 대기 중인 제안 관리
    sessionMgr    *session.Manager  // 대화 세션 관리
}

// 처리 흐름
func (o *Orchestrator) Process(ctx, userID, message) (*AgentResponse, error)
func (o *Orchestrator) ConfirmProposal(ctx, userID, proposalID) (*ExecutionResult, error)
func (o *Orchestrator) RejectProposal(ctx, userID, proposalID) error
```

**처리 흐름:**
```
사용자 메시지 → Intent Detection → Tool Selection → Execution/Proposal → Response
```

#### 2. RAG System (rag/)

##### Retriever
- `RetrieveForStyle()`: 발송 메일에서 말투/스타일 학습
- `RetrieveForContext()`: 모든 메일에서 컨텍스트 검색

##### StyleAnalyzer (NEW)
발송 이메일을 분석하여 사용자의 커뮤니케이션 패턴 학습.

```go
type StyleAnalyzer struct {
    embedder    *Embedder
    personStore out.ExtendedPersonalizationStore  // Neo4j
    vectorStore *VectorStore
}

// 분석 항목
type StyleAnalysisResult struct {
    AvgSentenceLength int      // 평균 문장 길이
    FormalityScore    float64  // 격식 점수 (0~1)
    EmojiFrequency    float64  // 이모지 사용 빈도
    Greetings         []string // 인사말 패턴
    Closings          []string // 맺음말 패턴
    FrequentPhrases   []string // 자주 쓰는 문구
    ToneEstimate      string   // formal, casual, friendly, professional
}
```

#### 3. Tool System (tools/)

| 도구 | 설명 | Proposal 필요 |
|------|------|--------------|
| `mail.list` | 이메일 목록 조회 | No |
| `mail.read` | 이메일 상세 조회 | No |
| `mail.search` | 이메일 검색 | No |
| `mail.send` | 이메일 전송 | **Yes** |
| `mail.reply` | 이메일 답장 | **Yes** |
| `mail.delete` | 이메일 삭제 | **Yes** |
| `mail.archive` | 이메일 보관 | **Yes** |
| `mail.translate` | 이메일 번역 | No |
| `calendar.list` | 일정 목록 | No |
| `calendar.create` | 일정 생성 | **Yes** |
| `calendar.delete` | 일정 삭제 | **Yes** |
| `contact.list` | 연락처 목록 | No |
| `contact.search` | 연락처 검색 | No |

#### 4. Proposal System

수정 작업은 직접 실행하지 않고 사용자 확인 후 실행.

```go
type ActionProposal struct {
    ID          string
    Type        string           // mail.send, calendar.create, etc.
    Description string
    Data        map[string]any   // 실행에 필요한 데이터
    ExpiresAt   time.Time        // 10분 만료
}
```

---

## Neo4j 기반 개인화 시스템

### 그래프 스키마

```
(:User)
  ├──[:HAS_TRAIT]──>(:Trait)                  # 성격 특성
  ├──[:HAS_WRITING_STYLE]──>(:WritingStyle)   # 작문 스타일
  ├──[:HAS_TONE_PREF]──>(:TonePreference)     # 톤 선호도
  ├──[:HAS_PATTERN]──>(:CommunicationPattern) # 커뮤니케이션 패턴
  ├──[:USES_PHRASE]──>(:Phrase)               # 자주 쓰는 문구
  ├──[:HAS_SIGNATURE]──>(:Signature)          # 서명
  ├──[:HAS_EXPERTISE]──>(:TopicExpertise)     # 전문 분야
  └──[:COMMUNICATES_WITH]──>(:Contact)        # 연락처 관계
```

### ExtendedPersonalizationStore 인터페이스

```go
type ExtendedPersonalizationStore interface {
    PersonalizationStore
    
    // 확장된 프로필
    GetExtendedProfile(ctx, userID) (*ExtendedUserProfile, error)
    UpdateExtendedProfile(ctx, userID, profile) error
    
    // 연락처 관계 (Graph Edge)
    GetContactRelationships(ctx, userID, limit) ([]*ContactRelationship, error)
    GetContactRelationship(ctx, userID, contactEmail) (*ContactRelationship, error)
    UpsertContactRelationship(ctx, userID, rel) error
    GetFrequentContacts(ctx, userID, limit) ([]*ContactRelationship, error)
    GetImportantContacts(ctx, userID, limit) ([]*ContactRelationship, error)
    
    // 커뮤니케이션 패턴
    GetCommunicationPatterns(ctx, userID, patternType, limit) ([]*CommunicationPattern, error)
    UpsertCommunicationPattern(ctx, userID, pattern) error
    GetPatternsByContext(ctx, userID, context, limit) ([]*CommunicationPattern, error)
    
    // 전문 분야
    GetTopicExpertise(ctx, userID, limit) ([]*TopicExpertise, error)
    UpsertTopicExpertise(ctx, userID, topic) error
    
    // 자동완성 컨텍스트
    GetAutocompleteContext(ctx, userID, recipientEmail, inputPrefix) (*AutocompleteContext, error)
}
```

### 주요 데이터 모델

#### ExtendedUserProfile
```go
type ExtendedUserProfile struct {
    UserID    string
    Email     string
    Name      string
    Nickname  string
    
    // 인구통계
    AgeRange  string   // 20s, 30s, 40s...
    Location  string
    Timezone  string
    Language  string
    Languages []string
    
    // 직업 정보
    JobTitle   string
    Department string
    Company    string
    Industry   string
    Seniority  string   // junior, mid, senior, executive
    Skills     []string
    
    // 커뮤니케이션 선호
    PreferredTone    string   // formal, casual, friendly
    ResponseSpeed    string   // quick, detailed, balanced
    PreferredLength  string   // short, medium, long
    EmojiUsage       float64  // 0.0 ~ 1.0
    FormalityDefault float64  // 0.0 ~ 1.0
    
    // 메타데이터
    ProfileCompleteness float64
    SourceCount         int      // 분석된 이메일 수
}
```

#### ContactRelationship
```go
type ContactRelationship struct {
    ContactEmail string
    ContactName  string
    RelationType string   // colleague, client, vendor, boss, friend
    
    // 관계 변화 이력 (시간에 따른 변화 추적)
    RelationHistory []RelationChange
    
    // 상호작용 통계
    EmailsSent     int
    EmailsReceived int
    LastContact    time.Time
    FirstContact   time.Time
    
    // 이 연락처에 사용하는 스타일 (시간에 따라 진화)
    ToneUsed       string
    FormalityLevel float64
    AvgReplyTime   int
    
    // 스타일 트렌드 (격식 변화 추적)
    FormalityTrend string   // increasing, decreasing, stable
    ToneChangeRate float64  // 톤 변화 속도
    
    // 중요도 (주기적 재계산)
    ImportanceScore float64  // frequency + recency 기반
    IsFrequent      bool
    IsImportant     bool
    
    // 활동 상태
    IsActive         bool      // 최근 90일 내 연락 여부
    LastActivityDate time.Time
    InactivityDays   int
}

// RelationChange - 관계 변화 기록
type RelationChange struct {
    FromType   string    // 이전 관계
    ToType     string    // 새 관계
    ChangedAt  time.Time // 변경 시점
    Confidence float64   // 신뢰도 (0~1)
    Reason     string    // inferred, manual, email_signature
}
```

#### CommunicationPattern
```go
type CommunicationPattern struct {
    PatternID   string
    PatternType string   // greeting, closing, transition, response
    Text        string
    Variants    []string
    Context     string   // formal, casual, urgent
    UsageCount  int
    Confidence  float64
}
```

---

## 자동완성 API

### 엔드포인트

| Method | Path | 설명 |
|--------|------|------|
| POST | `/ai/autocomplete` | 자동완성 제안 |
| GET | `/ai/autocomplete/context` | 자동완성 컨텍스트 조회 |
| GET | `/ai/profile` | 사용자 프로필 조회 |
| PUT | `/ai/profile` | 사용자 프로필 수정 |
| GET | `/ai/contacts/frequent` | 자주 연락하는 연락처 |
| GET | `/ai/contacts/important` | 중요 연락처 |
| GET | `/ai/patterns` | 커뮤니케이션 패턴 |
| GET | `/ai/phrases` | 자주 쓰는 문구 |

### 자동완성 요청/응답

```json
// POST /ai/autocomplete
{
  "input_prefix": "안녕하",
  "recipient_email": "boss@company.com",
  "context": "greeting",
  "max_suggestions": 5
}

// Response
{
  "suggestions": [
    {
      "text": "안녕하세요, 팀장님",
      "type": "pattern",
      "confidence": 0.95,
      "source": "learned"
    },
    {
      "text": "안녕하십니까",
      "type": "phrase",
      "confidence": 0.8,
      "source": "pattern"
    }
  ],
  "context": {
    "user_profile": {...},
    "contact_info": {...},
    "tone_preference": {...}
  }
}
```

---

## 데이터 흐름

### 발송 메일 분석 파이프라인

```
메일 발송
    │
    ▼
Worker: ProcessProfileAnalysis
    │
    ├──> StyleAnalyzer.AnalyzeSentEmail()
    │       │
    │       ├──> 문장 길이, 격식 점수, 이모지 빈도 계산
    │       ├──> 인사말/맺음말 패턴 추출
    │       └──> 자주 쓰는 문구 추출
    │
    ├──> Neo4j: UpdateWritingStyle()
    │
    ├──> Neo4j: UpsertContactRelationship()
    │
    ├──> Neo4j: UpsertCommunicationPattern()
    │
    └──> Neo4j: AddPhrase()
```

### 자동완성 컨텍스트 조회

```
자동완성 요청 (recipient_email)
    │
    ▼
Neo4j: GetAutocompleteContext()
    │
    ├──> GetExtendedProfile()       # 사용자 프로필
    ├──> GetContactRelationship()   # 수신자와의 관계
    ├──> GetTonePreference()        # 해당 관계의 톤 선호
    ├──> GetWritingStyle()          # 작문 스타일
    ├──> GetFrequentPhrases()       # 자주 쓰는 문구
    └──> GetPatternsByContext()     # 컨텍스트별 패턴
    │
    ▼
AutocompleteContext 반환
```

---

## 구현 완료

- [x] 도메인 모델 (Email, Calendar, Contact, User)
- [x] Port 인터페이스 정의
- [x] 기본 Service 구현
- [x] AI Agent Orchestrator
- [x] Tool System (mail, calendar, contact, search)
- [x] Proposal 기반 액션
- [x] RAG System (embedder, indexer, retriever, vectorstore)
- [x] Neo4j ExtendedPersonalizationStore
- [x] StyleAnalyzer (말투/스타일 분석)
- [x] 자동완성 API

---

## 관계 변화 추적 시스템

### 개요

연락처 관계는 시간에 따라 변할 수 있습니다:
- 동료 → 상사 (승진)
- 클라이언트 → 동료 (이직)
- 격식체 → 친근체 (친해짐)
- 활발 → 비활성 (프로젝트 종료)

### 변화 감지 로직

```go
// StyleAnalyzer.updateContactRelationship() 에서 처리

// 1. 관계 유형 변화 감지
if inferredRelation != rel.RelationType {
    changeConfidence := calculateRelationChangeConfidence(oldType, newType, body)
    if changeConfidence > 0.7 {
        // 변화 기록 및 업데이트
        rel.RelationHistory = append(rel.RelationHistory, change)
        rel.RelationType = inferredRelation
    }
}

// 2. 격식 트렌드 추적
formalityDiff := currentFormality - rel.FormalityLevel
if formalityDiff > 0.1 {
    rel.FormalityTrend = "increasing"  // 더 격식체로
} else if formalityDiff < -0.1 {
    rel.FormalityTrend = "decreasing"  // 더 친근체로
}

// 3. Exponential Moving Average로 노이즈 제거
alpha := 0.3
rel.FormalityLevel = alpha*current + (1-alpha)*existing
```

### 관계 변화 신뢰도 계산

| 전환 | 키워드 | 신뢰도 증가 |
|------|--------|------------|
| colleague → boss | "promoted", "팀장님", "승진" | +0.2 |
| client → colleague | "joined", "입사", "합류" | +0.2 |
| boss → colleague | "stepping down", "전배" | +0.2 |
| * → boss | "manager", "director", "부장" | +0.1 |
| * → subordinate | "intern", "junior", "인턴" | +0.1 |

### 활동 상태 관리

```go
// 연락 시 활성화
rel.IsActive = true
rel.LastActivityDate = now
rel.InactivityDays = 0

// 비활성 감지 (스케줄러에서 처리)
if time.Since(rel.LastActivityDate) > 90*24*time.Hour {
    rel.IsActive = false
    rel.InactivityDays = daysSince(rel.LastActivityDate)
}
```

### 사용 예시

```json
// GET /ai/contacts/frequent 응답
{
  "contacts": [
    {
      "contact_email": "kim@company.com",
      "contact_name": "김팀장",
      "relation_type": "boss",
      "relation_history": [
        {
          "from_type": "colleague",
          "to_type": "boss",
          "changed_at": "2024-03-15T10:00:00Z",
          "confidence": 0.85,
          "reason": "inferred"
        }
      ],
      "formality_trend": "stable",
      "formality_level": 0.8,
      "is_active": true,
      "importance_score": 0.92
    }
  ]
}
```

---

## 향후 구현

- [ ] AI 기반 자동완성 (LLM 활용)
- [ ] 프로필 자동 추론 (발송 메일 기반)
- [ ] 다국어 스타일 분석
- [ ] 관계 유형 자동 분류 (LLM 기반)
- [ ] 비활성 연락처 자동 아카이브
- [ ] 관계 변화 알림 기능
