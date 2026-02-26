# AI Classification - 코드 가이드

## 개요

새 메일 도착 시 AI가 자동으로 분류 (카테고리, 우선순위, 요약).

---

## 1. 분류 항목

| 항목 | 값 | 설명 |
|------|-----|------|
| **Category** | primary, social, promotion, updates, forums | Gmail 카테고리 |
| **Priority** | urgent, high, normal, low | 긴급도 |
| **Intent** | info, action_required, fyi, meeting, spam | 메일 의도 |
| **Summary** | string | 긴 메일 요약 (선택) |

---

## 2. 도메인 모델

### core/domain/classification.go

```go
package domain

type Classification struct {
    EmailID      int64     `json:"email_id"`
    Category     Category  `json:"category"`
    Priority     Priority  `json:"priority"`
    Intent       Intent    `json:"intent"`
    Summary      string    `json:"summary,omitempty"`
    Tags         []string  `json:"tags,omitempty"`
    Confidence   float64   `json:"confidence"`
    MatchedRules []int64   `json:"matched_rules,omitempty"`
    ProcessedAt  time.Time `json:"processed_at"`
    ModelUsed    string    `json:"model_used"`
    TokensUsed   int       `json:"tokens_used"`
}

type Category string
const (
    CategoryPrimary   Category = "primary"
    CategorySocial    Category = "social"
    CategoryPromotion Category = "promotion"
    CategoryUpdates   Category = "updates"
    CategoryForums    Category = "forums"
)

type Priority string
const (
    PriorityUrgent Priority = "urgent"
    PriorityHigh   Priority = "high"
    PriorityNormal Priority = "normal"
    PriorityLow    Priority = "low"
)

type Intent string
const (
    IntentInfo           Intent = "info"
    IntentActionRequired Intent = "action_required"
    IntentFYI            Intent = "fyi"
    IntentMeeting        Intent = "meeting"
    IntentSpam           Intent = "spam"
)
```

---

## 3. Port 인터페이스

### core/port/out/ai_classifier.go

```go
package out

type AIClassifierPort interface {
    // 단일 이메일 분류
    Classify(ctx context.Context, email *domain.Email, rules *domain.ClassificationRules) (*domain.Classification, error)
    
    // 배치 분류 (비용 최적화)
    ClassifyBatch(ctx context.Context, emails []*domain.Email, rules *domain.ClassificationRules) ([]*domain.Classification, error)
    
    // 요약 생성
    Summarize(ctx context.Context, email *domain.Email, maxLength int) (string, error)
}
```

---

## 4. LLM 구현

### core/agent/llm/classify.go

```go
package llm

type Classifier struct {
    client *Client
}

func (c *Classifier) Classify(ctx context.Context, email *domain.Email, rules *domain.ClassificationRules) (*domain.Classification, error) {
    prompt := c.buildClassifyPrompt(email, rules)
    
    response, err := c.client.Complete(ctx, &CompletionRequest{
        Model:       "gpt-4o-mini",
        Messages:    prompt,
        Temperature: 0.1, // 일관성 위해 낮게
        MaxTokens:   500,
        ResponseFormat: &ResponseFormat{
            Type: "json_object",
        },
    })
    if err != nil {
        return nil, err
    }
    
    var result classificationResult
    json.Unmarshal([]byte(response.Content), &result)
    
    return &domain.Classification{
        EmailID:    email.ID,
        Category:   domain.Category(result.Category),
        Priority:   domain.Priority(result.Priority),
        Intent:     domain.Intent(result.Intent),
        Summary:    result.Summary,
        Confidence: result.Confidence,
        ModelUsed:  "gpt-4o-mini",
        TokensUsed: response.Usage.TotalTokens,
    }, nil
}

func (c *Classifier) buildClassifyPrompt(email *domain.Email, rules *domain.ClassificationRules) []Message {
    systemPrompt := `You are an email classifier. Analyze the email and return JSON:
{
    "category": "primary|social|promotion|updates|forums",
    "priority": "urgent|high|normal|low",
    "intent": "info|action_required|fyi|meeting|spam",
    "summary": "1-2 sentence summary if email is long",
    "confidence": 0.0-1.0
}

Classification rules:
- urgent: 마감 임박, 긴급 표시, 중요 발신자
- high: 업무 관련, 답장 필요
- normal: 일반 메일
- low: 뉴스레터, 광고, 자동 발송`

    // 사용자 규칙 추가
    if rules != nil {
        if len(rules.ImportantDomains) > 0 {
            systemPrompt += fmt.Sprintf("\n\nImportant domains (mark as high priority): %v", rules.ImportantDomains)
        }
        if len(rules.ImportantKeywords) > 0 {
            systemPrompt += fmt.Sprintf("\nImportant keywords: %v", rules.ImportantKeywords)
        }
        if rules.CustomRules != "" {
            systemPrompt += fmt.Sprintf("\nCustom rules: %s", rules.CustomRules)
        }
    }

    emailContent := fmt.Sprintf(`From: %s <%s>
Subject: %s
Date: %s

%s`, email.FromName, email.FromEmail, email.Subject, email.Date, truncate(email.Body, 2000))

    return []Message{
        {Role: "system", Content: systemPrompt},
        {Role: "user", Content: emailContent},
    }
}
```

---

## 5. Worker Processor

### adapter/in/worker/ai_processor.go

```go
package worker

type AIClassifyProcessor struct {
    classifier   out.AIClassifierPort
    mailRepo     out.MailRepository
    settingsRepo out.SettingsRepository
    realtime     out.RealtimePort
}

func (p *AIClassifyProcessor) Process(ctx context.Context, job *domain.SyncJob) error {
    var payload struct {
        EmailID int64 `json:"email_id"`
    }
    json.Unmarshal(job.Payload, &payload)
    
    // 1. 이메일 조회
    email, err := p.mailRepo.GetByID(ctx, payload.EmailID)
    if err != nil {
        return err
    }
    
    // 2. 사용자 분류 규칙 조회
    rules, _ := p.settingsRepo.GetClassificationRules(ctx, job.UserID)
    
    // 3. AI 분류
    classification, err := p.classifier.Classify(ctx, email, rules)
    if err != nil {
        return err
    }
    
    // 4. DB 업데이트
    err = p.mailRepo.UpdateClassification(ctx, email.ID, classification)
    if err != nil {
        return err
    }
    
    // 5. 실시간 이벤트 발행
    p.realtime.Push(ctx, job.UserID, &domain.RealtimeEvent{
        Type: "email.classified",
        Data: classification,
    })
    
    return nil
}

func (p *AIClassifyProcessor) JobType() domain.JobType {
    return domain.JobAIClassify
}
```

---

## 6. 배치 분류 (비용 최적화)

다수의 메일을 한 번에 분류하여 API 호출 횟수 감소.

```go
// 배치 크기: 10개씩
func (p *AIClassifyProcessor) ProcessBatch(ctx context.Context, jobs []*domain.SyncJob) error {
    emails := make([]*domain.Email, 0, len(jobs))
    for _, job := range jobs {
        email, _ := p.mailRepo.GetByID(ctx, job.EmailID)
        emails = append(emails, email)
    }
    
    // 배치 분류 (단일 API 호출)
    classifications, err := p.classifier.ClassifyBatch(ctx, emails, rules)
    if err != nil {
        return err
    }
    
    // 결과 저장
    for i, classification := range classifications {
        p.mailRepo.UpdateClassification(ctx, emails[i].ID, classification)
        p.realtime.Push(ctx, jobs[i].UserID, &domain.RealtimeEvent{
            Type: "email.classified",
            Data: classification,
        })
    }
    
    return nil
}
```

---

## 7. 분류 결과 저장

### PostgreSQL 스키마

```sql
-- emails 테이블에 분류 필드 추가
ALTER TABLE emails ADD COLUMN IF NOT EXISTS ai_category VARCHAR(20);
ALTER TABLE emails ADD COLUMN IF NOT EXISTS ai_priority VARCHAR(20);
ALTER TABLE emails ADD COLUMN IF NOT EXISTS ai_intent VARCHAR(30);
ALTER TABLE emails ADD COLUMN IF NOT EXISTS ai_summary TEXT;
ALTER TABLE emails ADD COLUMN IF NOT EXISTS ai_confidence DECIMAL(3,2);
ALTER TABLE emails ADD COLUMN IF NOT EXISTS ai_processed_at TIMESTAMP;
ALTER TABLE emails ADD COLUMN IF NOT EXISTS ai_status VARCHAR(20) DEFAULT 'pending';

-- 인덱스
CREATE INDEX idx_emails_ai_category ON emails(ai_category);
CREATE INDEX idx_emails_ai_priority ON emails(ai_priority);
CREATE INDEX idx_emails_ai_status ON emails(ai_status);
```

### Repository

```go
func (r *MailAdapter) UpdateClassification(ctx context.Context, emailID int64, c *domain.Classification) error {
    query := `
        UPDATE emails SET
            ai_category = $1,
            ai_priority = $2,
            ai_intent = $3,
            ai_summary = $4,
            ai_confidence = $5,
            ai_processed_at = $6,
            ai_status = 'completed'
        WHERE id = $7
    `
    _, err := r.db.ExecContext(ctx, query,
        c.Category, c.Priority, c.Intent, c.Summary, c.Confidence, c.ProcessedAt, emailID)
    return err
}
```

---

## 8. 파이프라인 흐름

```
1. 새 메일 저장 (MailSyncService)
   │
   ▼
2. AI 분류 작업 발행
   └─→ messageQueue.Publish("ai:classify", {email_id: 123})
   │
   ▼
3. AIClassifyProcessor 처리
   ├─→ 이메일 조회
   ├─→ 사용자 규칙 조회
   ├─→ LLM API 호출
   ├─→ 결과 DB 저장
   └─→ 실시간 이벤트 발행
   │
   ▼
4. 프론트엔드 업데이트
   └─→ SSE로 분류 결과 수신 → UI 업데이트
```
