# 완벽한 메일 분류 시스템 설계

> **목표**: 점수 기반(Score-Based) 분류 + Embedding 재활용 + Auto Labeling으로 LLM 비용 85-95% 절감

---

## 1. 핵심 설계 원칙

### 1.1 점수 기반 분류 (Score Fusion)

모든 분류 규칙이 **점수(0.0-1.0)**를 반환하고, 가장 높은 점수가 최종 결정:

```
최종 점수 = max(Stage0, Stage1, Stage2, Stage3, Stage4)
```

**장점**:
- 규칙 간 우선순위 문제 자연스럽게 해결
- 새로운 규칙 추가 시 기존 로직 변경 불필요
- 신뢰도 기반 LLM 호출 결정 (낮은 점수만 LLM)

### 1.2 Embedding 재활용

이미 RAG 인덱싱에서 생성하는 embedding을 분류/라벨에 재활용:

```
메일 동기화 → Embedding 생성 → emails.embedding 저장
                                    ↓
                            분류 (유사도 검색)
                            Auto Label (유사 이메일 탐색)
                            시맨틱 캐시 (LLM 호출 회피)
```

**비용**: text-embedding-3-small = $0.02/1M tokens (이미 지출하는 비용)

### 1.3 5-Stage 파이프라인

```
Stage 0: RFC Headers      (~50-60%)  → List-Unsubscribe, Precedence
Stage 1: Sender Profile   (~15-20%)  → 행동 기반 점수 (read/reply/delete)
Stage 2: User Rules       (~10-15%)  → 도메인/키워드/VIP
Stage 3: Semantic Cache   (~10-15%)  → Embedding 유사도 검색
Stage 4: LLM Fallback     (~5-10%)   → 최후 수단
```

---

## 2. Stage별 상세 설계

### Stage 0: RFC Headers (0.80-0.95 점수)

기존 RFCClassifier를 점수 기반으로 변환:

| Signal | Category | Score |
|--------|----------|-------|
| `List-Unsubscribe` 헤더 존재 | Newsletter/Marketing | 0.95 |
| `Precedence: bulk` | Newsletter | 0.90 |
| `Auto-Submitted: auto-generated` | Notification | 0.85 |
| ESP 도메인 (Mailchimp, SendGrid) | Marketing | 0.90 |
| `X-Campaign` 헤더 존재 | Marketing | 0.85 |
| `Feedback-ID` (Gmail) | Newsletter | 0.80 |

```go
type RFCScoreResult struct {
    Category    EmailCategory
    SubCategory *EmailSubCategory
    Score       float64         // 0.0 - 1.0
    Signals     []string        // 탐지된 시그널들
}
```

### Stage 1: Sender Profile Score (0.60-0.95 점수)

**행동 기반 중요도 점수 공식**:

```go
func (p *SenderProfile) ImportanceScore() float64 {
    // 기본 Engagement 점수 (0-60점)
    engagementScore := (p.ReadRate * 20) +      // 읽기: 최대 20점
                       (p.ReplyRate * 40) +      // 답장: 최대 40점 (가장 중요)
                       ((1 - p.DeleteRate) * 15) // 삭제 안함: 최대 15점

    // 최근성 보너스 (0-15점)
    recencyBonus := 0.0
    daysSinceLastEmail := time.Since(p.LastSeenAt).Hours() / 24
    if daysSinceLastEmail < 7 {
        recencyBonus = 15.0
    } else if daysSinceLastEmail < 30 {
        recencyBonus = 10.0
    } else if daysSinceLastEmail < 90 {
        recencyBonus = 5.0
    }

    // 연락처 보너스 (0-10점)
    contactBonus := 0.0
    if p.IsContact {
        contactBonus = 10.0
    }

    // VIP 보너스
    if p.IsVIP {
        return 0.98  // VIP는 최고 점수
    }

    // Muted는 최저 점수
    if p.IsMuted {
        return 0.10
    }

    // 총점: 최대 100점 → 0.0-1.0 변환
    totalScore := engagementScore + recencyBonus + contactBonus
    return min(totalScore / 100.0, 0.95)
}
```

**확장된 SenderProfile 모델**:

```go
type SenderProfile struct {
    // 기존 필드
    ID, UserID, Email, Domain
    LearnedCategory, LearnedSubCategory
    IsVIP, IsMuted
    EmailCount, ReadRate, ReplyRate

    // 새로운 필드
    DeleteRate       float64   // 삭제 비율 (0.0-1.0)
    AvgResponseTime  *int64    // 평균 응답 시간 (분)
    IsContact        bool      // 연락처에 있는지
    InteractionCount int       // 총 상호작용 수 (읽기+답장+클릭)
    LastInteractedAt *time.Time // 마지막 상호작용 시간
    
    // 분류 학습
    ConfirmedLabels  []int64   // 사용자가 확정한 라벨들
    ImportanceScore  float64   // 캐시된 중요도 점수
}
```

### Stage 2: User Rules (0.85-0.99 점수)

**우선순위 기반 규칙 시스템**:

```go
type ClassificationRule struct {
    ID       int64
    UserID   uuid.UUID
    Type     RuleType      // exact_sender, sender_domain, subject_keyword, body_keyword, ai_prompt
    Pattern  string        // 매칭 패턴
    Action   RuleAction    // assign_category, assign_priority, assign_label, mark_important
    Value    string        // 액션 값
    Score    float64       // 기본 점수 (0.90-0.99)
    Position int           // 우선순위 (낮을수록 먼저)
    IsActive bool
}

type RuleType string
const (
    RuleTypeExactSender   RuleType = "exact_sender"    // 0.99 점수
    RuleTypeSenderDomain  RuleType = "sender_domain"   // 0.95 점수
    RuleTypeSubjectKeyword RuleType = "subject_keyword" // 0.90 점수
    RuleTypeBodyKeyword   RuleType = "body_keyword"    // 0.85 점수
    RuleTypeAIPrompt      RuleType = "ai_prompt"       // LLM 필요
)
```

**규칙 매칭 순서** (Position 무관하게 Type 기반):

1. `exact_sender` → 정확한 이메일 주소 매칭 (0.99)
2. `sender_domain` → 도메인 매칭 (0.95)
3. `subject_keyword` → 제목 키워드 (0.90)
4. `body_keyword` → 본문 키워드 (0.85)
5. `ai_prompt` → LLM 자연어 규칙 (Stage 4로 이동)

### Stage 3: Semantic Cache (0.80-0.95 점수)

**Embedding 유사도 기반 분류 캐시**:

```go
type SemanticCache struct {
    vectorStore *VectorStore
    threshold   float64  // 기본: 0.92
}

type CachedClassification struct {
    ID           int64
    UserID       uuid.UUID
    Embedding    []float32     // 1536 dimensions
    Category     EmailCategory
    Priority     Priority
    Labels       []int64
    Score        float64       // LLM 분류 시 신뢰도
    UsageCount   int           // 캐시 히트 횟수
    LastUsedAt   time.Time
    CreatedAt    time.Time
}

func (c *SemanticCache) Find(ctx context.Context, userID uuid.UUID, embedding []float32) (*CachedClassification, error) {
    // 유사한 이메일의 분류 결과 검색
    results, err := c.vectorStore.SearchClassificationCache(ctx, embedding, &CacheSearchOptions{
        UserID:    userID,
        Limit:     5,
        MinScore:  c.threshold,  // 0.92 이상만
    })
    
    if len(results) == 0 {
        return nil, ErrCacheMiss
    }
    
    // 가장 유사한 결과 반환 (점수 × 사용 빈도 가중치)
    best := results[0]
    for _, r := range results[1:] {
        if r.Score * math.Log2(float64(r.UsageCount+1)) > 
           best.Score * math.Log2(float64(best.UsageCount+1)) {
            best = r
        }
    }
    
    return best, nil
}
```

**Threshold 최적화**:

| Threshold | Recall | Precision | 사용 시나리오 |
|-----------|--------|-----------|--------------|
| 0.95+ | 낮음 | 매우 높음 | 중요 이메일 |
| 0.90-0.95 | 중간 | 높음 | 일반 분류 (권장) |
| 0.85-0.90 | 높음 | 중간 | 대량 처리 |

### Stage 4: LLM Fallback (0.70-0.95 점수)

**호출 조건**:
- 이전 Stage 최고 점수 < 0.80
- 또는 `ai_prompt` 규칙 존재
- 또는 새로운 발신자 (SenderProfile 없음)

**비용 최적화**:
- 프롬프트 캐싱 (30분 TTL)
- 배치 처리 (10개씩)
- 저비용 모델 우선 (gpt-4o-mini)
- 토큰 제한 (최대 1000 tokens)

---

## 3. Auto Labeling 시스템

### 3.1 라벨 규칙 구조

```go
type LabelRule struct {
    ID            int64
    UserID        uuid.UUID
    LabelID       int64
    Type          LabelRuleType
    Pattern       string         // 매칭 패턴 or AI 프롬프트
    Score         float64        // 적용 점수 (0.0-1.0)
    IsAutoCreated bool           // 자동 학습으로 생성됨
    IsActive      bool
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

type LabelRuleType string
const (
    LabelRuleExactSender     LabelRuleType = "exact_sender"     // 0.99
    LabelRuleSenderDomain    LabelRuleType = "sender_domain"    // 0.95
    LabelRuleSubjectKeyword  LabelRuleType = "subject_keyword"  // 0.90
    LabelRuleEmbedding       LabelRuleType = "embedding"        // 유사도 기반
    LabelRuleAIPrompt        LabelRuleType = "ai_prompt"        // LLM 필요
)
```

### 3.2 사용자 라벨 추가 시 학습

```go
func (s *AutoLabelService) OnUserAddLabel(ctx context.Context, userID uuid.UUID, emailID, labelID int64) error {
    email, _ := s.emailRepo.GetByID(emailID)
    
    // 1. 정확한 발신자 규칙 체크 (이미 있으면 스킵)
    existingRule, _ := s.ruleRepo.FindByPattern(userID, labelID, LabelRuleExactSender, email.FromEmail)
    if existingRule != nil {
        return nil
    }
    
    // 2. 같은 라벨이 적용된 이메일들 분석
    sameLabeled, _ := s.emailRepo.GetByLabel(userID, labelID, 100)
    
    // 3. 패턴 추출
    patterns := s.extractPatterns(email, sameLabeled)
    
    // 4. 규칙 자동 생성
    for _, pattern := range patterns {
        if pattern.Confidence >= 0.85 {
            s.ruleRepo.Create(&LabelRule{
                UserID:        userID,
                LabelID:       labelID,
                Type:          pattern.Type,
                Pattern:       pattern.Value,
                Score:         pattern.Confidence,
                IsAutoCreated: true,
            })
        }
    }
    
    // 5. Embedding 기반 규칙 (유사 이메일 탐지용)
    if email.Embedding != nil {
        s.ruleRepo.Create(&LabelRule{
            UserID:        userID,
            LabelID:       labelID,
            Type:          LabelRuleEmbedding,
            Pattern:       fmt.Sprintf("ref:%d", emailID),  // 참조 이메일 ID
            Score:         0.90,
            IsAutoCreated: true,
        })
    }
    
    return nil
}

func (s *AutoLabelService) extractPatterns(email *Email, sameLabeled []*Email) []Pattern {
    patterns := []Pattern{}
    
    // 발신자 분석
    senderCount := countSender(email.FromEmail, sameLabeled)
    if senderCount >= 3 || float64(senderCount)/float64(len(sameLabeled)) > 0.5 {
        patterns = append(patterns, Pattern{
            Type:       LabelRuleExactSender,
            Value:      email.FromEmail,
            Confidence: min(0.99, 0.80 + float64(senderCount)*0.05),
        })
    }
    
    // 도메인 분석
    domain := extractDomain(email.FromEmail)
    domainCount := countDomain(domain, sameLabeled)
    if domainCount >= 5 || float64(domainCount)/float64(len(sameLabeled)) > 0.6 {
        patterns = append(patterns, Pattern{
            Type:       LabelRuleSenderDomain,
            Value:      domain,
            Confidence: min(0.95, 0.75 + float64(domainCount)*0.03),
        })
    }
    
    // 키워드 분석 (TF-IDF 기반)
    keywords := extractKeywords(email.Subject, sameLabeled)
    for _, kw := range keywords {
        if kw.Score >= 0.7 {
            patterns = append(patterns, Pattern{
                Type:       LabelRuleSubjectKeyword,
                Value:      kw.Word,
                Confidence: kw.Score,
            })
        }
    }
    
    return patterns
}
```

### 3.3 새 이메일 도착 시 Auto Label 적용

```go
func (s *AutoLabelService) ApplyLabels(ctx context.Context, email *Email) ([]int64, error) {
    appliedLabels := []int64{}
    
    // 1. 규칙 기반 라벨 (LLM 0%)
    rules, _ := s.ruleRepo.ListByUser(email.UserID)
    
    for _, rule := range rules {
        if !rule.IsActive {
            continue
        }
        
        matched, score := s.matchRule(email, rule)
        if matched && score >= 0.85 {
            appliedLabels = append(appliedLabels, rule.LabelID)
        }
    }
    
    // 2. Embedding 유사도 기반 라벨 (LLM 0%)
    if email.Embedding != nil {
        embeddingRules, _ := s.ruleRepo.ListByType(email.UserID, LabelRuleEmbedding)
        for _, rule := range embeddingRules {
            refEmailID := parseRefEmailID(rule.Pattern)
            refEmail, _ := s.emailRepo.GetByID(refEmailID)
            
            if refEmail != nil && refEmail.Embedding != nil {
                similarity := cosineSimilarity(email.Embedding, refEmail.Embedding)
                if similarity >= 0.90 {
                    appliedLabels = append(appliedLabels, rule.LabelID)
                }
            }
        }
    }
    
    // 3. AI 프롬프트 라벨 (LLM 필요 - 최후 수단)
    // 사용자가 명시적으로 AI 프롬프트 규칙을 만든 경우만
    
    return unique(appliedLabels), nil
}
```

---

## 4. 통합 파이프라인 아키텍처

### 4.1 전체 플로우

```
┌─────────────────────────────────────────────────────────────────┐
│                    Email Classification Pipeline                 │
└─────────────────────────────────────────────────────────────────┘

새 이메일 도착
     │
     ▼
┌─────────────────┐
│ Stage 0: RFC    │ ──→ Score: 0.95 (Newsletter) ──┐
│ Headers         │                                 │
└─────────────────┘                                 │
     │ (Score < 0.80)                              │
     ▼                                             │
┌─────────────────┐                                │
│ Stage 1: Sender │ ──→ Score: 0.85 (Important) ──┤
│ Profile Score   │                                │
└─────────────────┘                                │
     │ (Score < 0.80)                              │
     ▼                                             │
┌─────────────────┐                                │
│ Stage 2: User   │ ──→ Score: 0.99 (VIP Rule) ───┤
│ Rules           │                                │
└─────────────────┘                                │
     │ (Score < 0.80)                              │
     ▼                                             │
┌─────────────────┐                                │
│ Stage 3: Sem.   │ ──→ Score: 0.92 (Cache Hit) ──┤
│ Cache           │                                │
└─────────────────┘                                │
     │ (Score < 0.80)                              │
     ▼                                             │
┌─────────────────┐                                │
│ Stage 4: LLM    │ ──→ Score: 0.87 (AI)          │
│ Fallback        │                                │
└─────────────────┘                                │
                                                   │
                                                   ▼
                                         ┌─────────────────┐
                                         │ Score Fusion    │
                                         │ max(all stages) │
                                         └─────────────────┘
                                                   │
                                                   ▼
                                         ┌─────────────────┐
                                         │ Auto Labeling   │
                                         │ (Rules + Embed) │
                                         └─────────────────┘
                                                   │
                                                   ▼
                                         ┌─────────────────┐
                                         │ Store & Index   │
                                         │ (DB + Cache)    │
                                         └─────────────────┘
```

### 4.2 점수 기반 분류기 인터페이스

```go
type ScoreResult struct {
    Category    EmailCategory
    SubCategory *EmailSubCategory
    Priority    Priority
    Labels      []int64
    Score       float64       // 0.0 - 1.0
    Source      string        // "rfc", "sender", "rule", "cache", "llm"
    Signals     []string      // 탐지된 시그널들
    LLMUsed     bool
}

type ScoreClassifier interface {
    Classify(ctx context.Context, input *ClassifyInput) (*ScoreResult, error)
    Name() string
}

// Pipeline
type ScorePipeline struct {
    classifiers []ScoreClassifier  // Stage 순서대로
    labelService *AutoLabelService
    cacheService *SemanticCache
}

func (p *ScorePipeline) Classify(ctx context.Context, input *ClassifyInput) (*ClassificationResult, error) {
    var bestResult *ScoreResult
    var allResults []*ScoreResult
    
    for _, classifier := range p.classifiers {
        result, err := classifier.Classify(ctx, input)
        if err != nil {
            continue
        }
        
        allResults = append(allResults, result)
        
        if bestResult == nil || result.Score > bestResult.Score {
            bestResult = result
        }
        
        // 충분히 높은 점수면 Early Exit
        if result.Score >= 0.95 {
            break
        }
    }
    
    // LLM Fallback (최고 점수가 낮을 때만)
    if bestResult == nil || bestResult.Score < 0.80 {
        llmResult, _ := p.llmFallback(ctx, input)
        if llmResult != nil {
            allResults = append(allResults, llmResult)
            if bestResult == nil || llmResult.Score > bestResult.Score {
                bestResult = llmResult
            }
        }
    }
    
    // Auto Labeling
    labels, _ := p.labelService.ApplyLabels(ctx, input.Email)
    bestResult.Labels = append(bestResult.Labels, labels...)
    
    // 캐시 저장 (LLM 결과인 경우)
    if bestResult.LLMUsed && input.Email.Embedding != nil {
        p.cacheService.Store(ctx, input.Email.Embedding, bestResult)
    }
    
    return &ClassificationResult{
        Category:    bestResult.Category,
        SubCategory: bestResult.SubCategory,
        Priority:    bestResult.Priority,
        Labels:      bestResult.Labels,
        Score:       bestResult.Score,
        Source:      bestResult.Source,
        AllResults:  allResults,  // 디버깅용
        LLMUsed:     bestResult.LLMUsed,
    }, nil
}
```

---

## 5. 데이터 모델 변경

### 5.1 SenderProfile 확장

```sql
ALTER TABLE sender_profiles ADD COLUMN delete_rate FLOAT DEFAULT 0;
ALTER TABLE sender_profiles ADD COLUMN avg_response_time INTEGER;
ALTER TABLE sender_profiles ADD COLUMN is_contact BOOLEAN DEFAULT FALSE;
ALTER TABLE sender_profiles ADD COLUMN interaction_count INTEGER DEFAULT 0;
ALTER TABLE sender_profiles ADD COLUMN last_interacted_at TIMESTAMPTZ;
ALTER TABLE sender_profiles ADD COLUMN importance_score FLOAT DEFAULT 0;
ALTER TABLE sender_profiles ADD COLUMN confirmed_labels INTEGER[] DEFAULT '{}';

-- 인덱스
CREATE INDEX idx_sender_profiles_importance ON sender_profiles(user_id, importance_score DESC);
```

### 5.2 LabelRule 테이블 생성

```sql
CREATE TABLE label_rules (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    label_id BIGINT NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,  -- exact_sender, sender_domain, subject_keyword, embedding, ai_prompt
    pattern TEXT NOT NULL,
    score FLOAT DEFAULT 0.90,
    is_auto_created BOOLEAN DEFAULT FALSE,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    UNIQUE(user_id, label_id, type, pattern)
);

CREATE INDEX idx_label_rules_user ON label_rules(user_id, is_active);
CREATE INDEX idx_label_rules_type ON label_rules(user_id, type);
```

### 5.3 Classification Cache 테이블

```sql
CREATE TABLE classification_cache (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    embedding vector(1536),       -- pgvector
    category VARCHAR(50) NOT NULL,
    sub_category VARCHAR(50),
    priority VARCHAR(20) NOT NULL,
    labels INTEGER[] DEFAULT '{}',
    score FLOAT NOT NULL,
    usage_count INTEGER DEFAULT 1,
    last_used_at TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    
    -- 30일 후 자동 삭제
    expires_at TIMESTAMPTZ DEFAULT NOW() + INTERVAL '30 days'
);

-- HNSW 인덱스 (cosine similarity)
CREATE INDEX idx_classification_cache_embedding 
ON classification_cache USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

CREATE INDEX idx_classification_cache_user ON classification_cache(user_id);
CREATE INDEX idx_classification_cache_expires ON classification_cache(expires_at);
```

### 5.4 Classification Rule 테이블 (기존 대체)

```sql
CREATE TABLE classification_rules (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,    -- exact_sender, sender_domain, subject_keyword, body_keyword, ai_prompt
    pattern TEXT NOT NULL,
    action VARCHAR(50) NOT NULL,  -- assign_category, assign_priority, assign_label, mark_important
    value TEXT NOT NULL,          -- category 값 or priority 값 or label_id
    score FLOAT DEFAULT 0.90,
    position INTEGER DEFAULT 0,   -- 같은 type 내 우선순위
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    UNIQUE(user_id, type, pattern, action)
);

CREATE INDEX idx_classification_rules_user ON classification_rules(user_id, is_active, type);
```

---

## 6. 성능 최적화

### 6.1 pgvector HNSW 인덱스 튜닝

```sql
-- emails 테이블 (이미 있음, 튜닝)
CREATE INDEX CONCURRENTLY idx_emails_embedding_hnsw
ON emails USING hnsw (embedding vector_cosine_ops)
WITH (m = 24, ef_construction = 100);  -- m 증가로 recall 향상

-- 검색 시 ef_search 조정
SET hnsw.ef_search = 100;  -- 기본 40, recall 향상
```

### 6.2 Sender Profile 캐싱

```go
type SenderProfileCache struct {
    redis      *redis.Client
    ttl        time.Duration  // 30분
}

func (c *SenderProfileCache) Key(userID uuid.UUID, email string) string {
    return fmt.Sprintf("sender:%s:%s", userID, email)
}

func (c *SenderProfileCache) Get(ctx context.Context, userID uuid.UUID, email string) (*SenderProfile, error) {
    key := c.Key(userID, email)
    data, err := c.redis.Get(ctx, key).Bytes()
    if err == redis.Nil {
        return nil, ErrCacheMiss
    }
    // JSON unmarshal
}

func (c *SenderProfileCache) Set(ctx context.Context, profile *SenderProfile) error {
    key := c.Key(profile.UserID, profile.Email)
    // JSON marshal + SET with TTL
}
```

### 6.3 규칙 매칭 최적화

```go
// Trie 기반 키워드 매칭 (O(m) vs O(n*m))
type RuleMatcher struct {
    exactSenders   map[string]*ClassificationRule  // O(1) lookup
    domainTrie     *Trie                           // 도메인 suffix 매칭
    keywordMatcher *AhoCorasick                    // 다중 키워드 동시 매칭
}
```

---

## 7. 비용 분석

### 7.1 예상 LLM 비용 절감

| Stage | 처리 비율 | LLM 비용 |
|-------|----------|----------|
| Stage 0: RFC | ~55% | $0 |
| Stage 1: Sender | ~15% | $0 |
| Stage 2: Rules | ~12% | $0 |
| Stage 3: Cache | ~10% | $0 |
| Stage 4: LLM | ~8% | ~$0.001/email |

**월 10,000 이메일 기준**:
- 기존 (100% LLM): 10,000 × $0.001 = **$10/월**
- 최적화 후 (8% LLM): 800 × $0.001 = **$0.80/월**
- **절감률: 92%**

### 7.2 Embedding 비용

text-embedding-3-small: $0.02 / 1M tokens

- 월 10,000 이메일
- 평균 500 tokens/email = 5M tokens
- 비용: $0.10/월 (이미 RAG에서 지출)

---

## 8. 구현 순서

### Phase 1: 기반 인프라 (1주)
1. [ ] 데이터 모델 마이그레이션
2. [ ] SenderProfile 확장 및 ImportanceScore 계산
3. [ ] ScoreClassifier 인터페이스 정의
4. [ ] 기존 RFCClassifier를 ScoreClassifier로 변환

### Phase 2: Stage 구현 (2주)
5. [ ] SenderProfileClassifier (Stage 1)
6. [ ] UserRuleClassifier (Stage 2)
7. [ ] SemanticCacheClassifier (Stage 3)
8. [ ] LLMFallbackClassifier (Stage 4)

### Phase 3: Auto Labeling (1주)
9. [ ] LabelRule 모델 및 Repository
10. [ ] AutoLabelService (학습 + 적용)
11. [ ] 라벨 추가/삭제 이벤트 연동

### Phase 4: 최적화 (1주)
12. [ ] pgvector HNSW 인덱스 튜닝
13. [ ] SenderProfile 캐싱
14. [ ] 규칙 매칭 최적화 (Trie, Aho-Corasick)
15. [ ] 모니터링 및 메트릭 추가

---

## 참고 자료

### Embedding 최적화
- [text-embedding-3-small Guide](https://blog.promptlayer.com/text-embedding-3-small-high-quality-embeddings-at-scale/)
- [Embedding Models in 2025](https://medium.com/@alex-azimbaev/embedding-models-in-2025-technology-pricing-practical-advice-2ed273fead7f)

### pgvector 최적화
- [pgvector GitHub](https://github.com/pgvector/pgvector)
- [Supabase pgvector Docs](https://supabase.com/docs/guides/database/extensions/pgvector)
- [Cosine Distance Optimization](https://0xhagen.medium.com/enhancing-performance-in-postgresql-the-journey-to-optimized-cosine-distance-searches-7b22e52c2efb)

### Email Classification
- [AI-Powered Mail Classification](https://thescimus.com/blog/ai-powered-mail-classification-models/)
- [Graph-Based Semantic Classification](https://ijai.iaescore.com/index.php/IJAI/article/download/27385/14895)

### Microservices Architecture
- [Event-Driven Architecture for Microservices](https://www.confluent.io/blog/do-microservices-need-event-driven-architectures/)
- [Microservices for AI Applications 2025](https://medium.com/@meeran03/microservices-architecture-for-ai-applications-scalable-patterns-and-2025-trends-5ac273eac232)
