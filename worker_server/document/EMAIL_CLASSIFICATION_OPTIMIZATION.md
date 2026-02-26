# Email Classification Optimization System

> **목표**: RFC 헤더 기반으로 LLM 호출 없이 ~70-80%의 이메일을 자동 분류하여 비용 절감
>
> **상태**: ✅ 구현 완료 (2024-01)

---

## 1. 새로운 파이프라인 구조

```
Stage 0: RFC Headers (~50-60%)     ← NEW: 업무 메일 아닌 것 필터링
   ├─ List-Unsubscribe → Newsletter
   ├─ Precedence: bulk/list → Marketing  
   ├─ Auto-Submitted → Notification
   ├─ X-Mailer (SendGrid, Mailchimp 등) → Marketing
   └─ noreply@ + 트랜잭션 패턴 → Transaction

Stage 1: User Rules (~10%)         ← 기존 Stage 0
   └─ ImportantDomains, Keywords

Stage 2: Known Domain (~15%)       ← 기존 Stage 2
   └─ SenderProfile, KnownDomain DB

Stage 3: LLM (~15-25%)             ← 실제 사람이 보낸 메일만
   └─ Work/Personal 구분
```

---

## 2. RFC 헤더 분류 규칙

### 2.1 Newsletter (뉴스레터)

| 헤더 | 값 | 신뢰도 | 설명 |
|------|-----|--------|------|
| `List-Unsubscribe` | 존재 | 95% | RFC 2369 - 메일링 리스트 표준 |
| `List-Unsubscribe-Post` | 존재 | 95% | RFC 8058 - One-Click Unsubscribe |
| `List-Id` | 존재 | 90% | RFC 2919 - 메일링 리스트 식별자 |
| `Precedence` | list | 85% | 메일링 리스트 표시 |

### 2.2 Marketing (마케팅)

| 헤더 | 값 | 신뢰도 | 설명 |
|------|-----|--------|------|
| `Precedence` | bulk | 90% | 대량 발송 표시 |
| `X-Mailer` | Mailchimp, SendGrid, etc. | 85% | 이메일 마케팅 툴 |
| `X-MC-User` | 존재 | 90% | Mailchimp |
| `X-SG-EID` | 존재 | 90% | SendGrid |
| `X-SES-*` | 존재 | 85% | Amazon SES |
| `X-Mailgun-*` | 존재 | 85% | Mailgun |
| `X-PM-*` | 존재 | 85% | Postmark |
| `Feedback-ID` | 존재 | 80% | Gmail 대량 발송 추적 |
| `X-Campaign-*` | 존재 | 85% | 캠페인 이메일 |

### 2.3 Notification (자동 알림)

| 헤더 | 값 | 신뢰도 | 설명 |
|------|-----|--------|------|
| `Auto-Submitted` | auto-generated, auto-replied | 95% | RFC 3834 |
| `X-Auto-Response-Suppress` | 존재 | 90% | Microsoft 자동 응답 |
| `Precedence` | junk | 85% | 자동 발송 |

### 2.4 Transaction (트랜잭션)

발신자 패턴 + 제목/본문 키워드 조합:

| 발신자 패턴 | 키워드 | 분류 |
|-------------|--------|------|
| noreply@, no-reply@ | 결제, 주문, 배송, 영수증 | Transaction/Receipt |
| notification@, alert@ | 로그인, 비밀번호, 보안 | Transaction/Security |
| support@, help@ | 티켓, 문의 | Transaction/Support |

---

## 3. Gmail API 헤더 추출

### 3.1 현재 상태

```go
// 현재: 기본 헤더만 추출
MetadataHeaders("From", "To", "Cc", "Bcc", "Subject", "Date", 
                "Message-ID", "In-Reply-To", "References", "Content-Type")
```

### 3.2 추가 필요 헤더

```go
// RFC 분류용 헤더 추가
var ClassificationHeaders = []string{
    // 기존 헤더
    "From", "To", "Cc", "Bcc", "Subject", "Date",
    "Message-ID", "In-Reply-To", "References", "Content-Type",
    
    // RFC 분류 헤더 (NEW)
    "List-Unsubscribe",       // RFC 2369 - Newsletter
    "List-Unsubscribe-Post",  // RFC 8058 - One-Click
    "List-Id",                // RFC 2919 - Mailing List ID
    "Precedence",             // bulk, list, junk
    "Auto-Submitted",         // RFC 3834 - Auto-generated
    "X-Auto-Response-Suppress", // Microsoft auto-reply
    
    // ESP (Email Service Provider) 헤더
    "X-Mailer",               // 발송 클라이언트
    "X-MC-User",              // Mailchimp
    "X-SG-EID",               // SendGrid
    "X-SES-Outgoing",         // Amazon SES
    "X-Mailgun-Variables",    // Mailgun
    "X-PM-Message-Id",        // Postmark
    "Feedback-ID",            // Gmail bulk tracking
    "X-Campaign-ID",          // Campaign emails
}
```

---

## 4. 데이터 구조

### 4.1 Provider 레벨 - 헤더 추출

```go
// ProviderMailMessage에 RFC 헤더 추가
type ProviderMailMessage struct {
    // 기존 필드...
    
    // RFC Classification Headers
    ClassificationHeaders *ClassificationHeaders `json:"classification_headers,omitempty"`
}

type ClassificationHeaders struct {
    // Mailing List (RFC 2369, 2919)
    ListUnsubscribe     string `json:"list_unsubscribe,omitempty"`
    ListUnsubscribePost string `json:"list_unsubscribe_post,omitempty"`
    ListId              string `json:"list_id,omitempty"`
    
    // Auto/Bulk (RFC 3834)
    Precedence          string `json:"precedence,omitempty"`
    AutoSubmitted       string `json:"auto_submitted,omitempty"`
    AutoResponseSuppress string `json:"auto_response_suppress,omitempty"`
    
    // ESP Headers
    XMailer             string `json:"x_mailer,omitempty"`
    FeedbackID          string `json:"feedback_id,omitempty"`
    
    // ESP Specific (boolean flags)
    IsMailchimp         bool   `json:"is_mailchimp,omitempty"`
    IsSendGrid          bool   `json:"is_sendgrid,omitempty"`
    IsAmazonSES         bool   `json:"is_amazon_ses,omitempty"`
    IsMailgun           bool   `json:"is_mailgun,omitempty"`
    IsPostmark          bool   `json:"is_postmark,omitempty"`
}
```

### 4.2 Domain 레벨 - 분류 결과

```go
// Email 도메인 모델에 헤더 기반 분류 힌트 추가
type EmailClassificationHints struct {
    // RFC Header Signals
    HasListUnsubscribe  bool   `json:"has_list_unsubscribe"`
    Precedence          string `json:"precedence,omitempty"` // bulk, list, junk
    IsAutoGenerated     bool   `json:"is_auto_generated"`
    
    // ESP Detection
    DetectedESP         string `json:"detected_esp,omitempty"` // mailchimp, sendgrid, etc.
    
    // Sender Pattern
    IsNoReply           bool   `json:"is_no_reply"`
    IsNotification      bool   `json:"is_notification"`
}
```

---

## 5. 분류 파이프라인 구현

### 5.1 Stage 0: RFC Header Classifier

```go
// ClassifyByRFCHeaders performs Stage 0: RFC header-based classification
func (p *Pipeline) ClassifyByRFCHeaders(headers *ClassificationHeaders, fromEmail string) *ClassificationResult {
    if headers == nil {
        return nil
    }
    
    // 1. Newsletter Detection (최고 우선순위)
    if headers.ListUnsubscribe != "" || headers.ListUnsubscribePost != "" {
        return &ClassificationResult{
            Category:   CategoryNewsletter,
            SubCategory: SubCategoryNewsletter,
            Priority:   PriorityLow,
            Source:     ClassificationSourceRFCHeader,
            Confidence: 0.95,
        }
    }
    
    // 2. List/Bulk Detection
    if headers.ListId != "" {
        return &ClassificationResult{
            Category:   CategoryNewsletter,
            Priority:   PriorityLow,
            Source:     ClassificationSourceRFCHeader,
            Confidence: 0.90,
        }
    }
    
    // 3. Precedence Header
    precedence := strings.ToLower(headers.Precedence)
    switch precedence {
    case "bulk":
        return &ClassificationResult{
            Category:   CategoryMarketing,
            Priority:   PriorityLowest,
            Source:     ClassificationSourceRFCHeader,
            Confidence: 0.90,
        }
    case "list":
        return &ClassificationResult{
            Category:   CategoryNewsletter,
            Priority:   PriorityLow,
            Source:     ClassificationSourceRFCHeader,
            Confidence: 0.85,
        }
    case "junk":
        return &ClassificationResult{
            Category:   CategorySpam,
            Priority:   PriorityLowest,
            Source:     ClassificationSourceRFCHeader,
            Confidence: 0.85,
        }
    }
    
    // 4. Auto-Submitted (RFC 3834)
    if headers.AutoSubmitted != "" && headers.AutoSubmitted != "no" {
        return &ClassificationResult{
            Category:   CategoryNotification,
            SubCategory: SubCategoryNotification,
            Priority:   PriorityLow,
            Source:     ClassificationSourceRFCHeader,
            Confidence: 0.95,
        }
    }
    
    // 5. ESP Detection
    if headers.IsMailchimp || headers.IsSendGrid || headers.IsAmazonSES || 
       headers.IsMailgun || headers.IsPostmark {
        return &ClassificationResult{
            Category:   CategoryMarketing,
            SubCategory: SubCategoryMarketing,
            Priority:   PriorityLow,
            Source:     ClassificationSourceRFCHeader,
            Confidence: 0.88,
        }
    }
    
    // 6. X-Mailer 기반 마케팅 툴 감지
    if isMarketingMailer(headers.XMailer) {
        return &ClassificationResult{
            Category:   CategoryMarketing,
            Priority:   PriorityLow,
            Source:     ClassificationSourceRFCHeader,
            Confidence: 0.85,
        }
    }
    
    // 7. Transaction Pattern (noreply + keywords)
    if isTransactionEmail(fromEmail, headers) {
        return &ClassificationResult{
            Category:   CategoryTransaction,
            Priority:   PriorityNormal,
            Source:     ClassificationSourceRFCHeader,
            Confidence: 0.80,
        }
    }
    
    return nil // RFC 헤더로 분류 불가 → 다음 Stage로
}

// isMarketingMailer checks if X-Mailer indicates marketing tool
func isMarketingMailer(mailer string) bool {
    mailerLower := strings.ToLower(mailer)
    marketingMailers := []string{
        "mailchimp", "sendgrid", "mailgun", "postmark", "sendinblue",
        "constant contact", "campaign monitor", "hubspot", "marketo",
        "klaviyo", "drip", "convertkit", "aweber", "activecampaign",
    }
    for _, m := range marketingMailers {
        if strings.Contains(mailerLower, m) {
            return true
        }
    }
    return false
}

// isTransactionEmail checks for transactional email patterns
func isTransactionEmail(fromEmail string, headers *ClassificationHeaders) bool {
    fromLower := strings.ToLower(fromEmail)
    
    // noreply 패턴 체크
    noReplyPatterns := []string{
        "noreply@", "no-reply@", "donotreply@", "do-not-reply@",
        "notification@", "notifications@", "alert@", "alerts@",
        "info@", "support@", "billing@", "orders@", "receipts@",
    }
    
    isNoReply := false
    for _, pattern := range noReplyPatterns {
        if strings.Contains(fromLower, pattern) {
            isNoReply = true
            break
        }
    }
    
    // noreply + Auto-Submitted 조합
    if isNoReply && headers.AutoSubmitted != "" {
        return true
    }
    
    // noreply만으로는 불충분 (추가 신호 필요)
    return false
}
```

### 5.2 전체 파이프라인 수정

```go
// Classify runs the email through the 4-stage classification pipeline.
func (p *Pipeline) Classify(ctx context.Context, userID uuid.UUID, email *domain.Email, 
    headers *ClassificationHeaders, body string) (*ClassificationResult, error) {
    
    // Stage 0: RFC Header Classification (NEW)
    if result := p.ClassifyByRFCHeaders(headers, email.FromEmail); result != nil {
        return result, nil
    }
    
    // Stage 1: User-defined rules (기존 Stage 0)
    if result, err := p.classifyByUserRules(ctx, userID, email); err == nil && result != nil {
        return result, nil
    }
    
    // Stage 2: Known domain matching (기존)
    if result, err := p.classifyByDomain(ctx, userID, email.FromEmail); err == nil && result != nil {
        return result, nil
    }
    
    // Stage 3: LLM-based classification (기존)
    if p.llmClient != nil {
        return p.classifyByLLM(ctx, email, body)
    }
    
    // Default
    return &ClassificationResult{
        Category:   CategoryOther,
        Priority:   PriorityNormal,
        Source:     ClassificationSourceDefault,
        Confidence: 0.5,
    }, nil
}
```

---

## 6. Gmail Adapter 수정

### 6.1 헤더 추출 함수 추가

```go
// extractClassificationHeaders extracts RFC headers for classification
func (a *GmailAdapter) extractClassificationHeaders(gmailHeaders []*gmail.MessagePartHeader) *ClassificationHeaders {
    headers := &ClassificationHeaders{}
    
    for _, h := range gmailHeaders {
        switch h.Name {
        // Mailing List Headers
        case "List-Unsubscribe":
            headers.ListUnsubscribe = h.Value
        case "List-Unsubscribe-Post":
            headers.ListUnsubscribePost = h.Value
        case "List-Id":
            headers.ListId = h.Value
            
        // Auto/Bulk Headers
        case "Precedence":
            headers.Precedence = h.Value
        case "Auto-Submitted":
            headers.AutoSubmitted = h.Value
        case "X-Auto-Response-Suppress":
            headers.AutoResponseSuppress = h.Value
            
        // ESP Headers
        case "X-Mailer":
            headers.XMailer = h.Value
        case "Feedback-ID":
            headers.FeedbackID = h.Value
            
        // ESP Specific
        case "X-MC-User":
            headers.IsMailchimp = true
        case "X-SG-EID":
            headers.IsSendGrid = true
        case "X-SES-Outgoing":
            headers.IsAmazonSES = true
        case "X-Mailgun-Variables":
            headers.IsMailgun = true
        case "X-PM-Message-Id":
            headers.IsPostmark = true
        }
    }
    
    return headers
}
```

### 6.2 MetadataHeaders 업데이트

```go
// Gmail API metadata 요청에 추가할 헤더 목록
var GmailClassificationHeaders = []string{
    // 기존
    "From", "To", "Cc", "Bcc", "Subject", "Date",
    "Message-ID", "In-Reply-To", "References", "Content-Type",
    
    // RFC Classification (NEW)
    "List-Unsubscribe", "List-Unsubscribe-Post", "List-Id",
    "Precedence", "Auto-Submitted", "X-Auto-Response-Suppress",
    "X-Mailer", "Feedback-ID",
    "X-MC-User", "X-SG-EID", "X-SES-Outgoing", 
    "X-Mailgun-Variables", "X-PM-Message-Id",
}

// fetchMessagesParallel 수정
metaMsg, err := svc.Users.Messages.Get("me", id).
    Format("metadata").
    MetadataHeaders(GmailClassificationHeaders...).
    Context(msgCtx).Do()
```

---

## 7. 예상 효과

### 7.1 LLM 호출 감소

| Stage | 처리 비율 | LLM 필요 |
|-------|----------|----------|
| Stage 0: RFC Headers | ~50-60% | No |
| Stage 1: User Rules | ~10% | No |
| Stage 2: Known Domain | ~15% | No |
| Stage 3: LLM | ~15-25% | **Yes** |

**결과**: LLM 호출 ~75% 감소 → 비용 75% 절감

### 7.2 분류 정확도

- RFC 헤더 기반: 90-95% 정확도 (표준 기반)
- User Rules: 95%+ 정확도 (사용자 정의)
- Known Domain: 85%+ 정확도 (학습 기반)
- LLM: 80-90% 정확도

### 7.3 처리 속도

- RFC 헤더 분류: < 1ms (메모리 내 처리)
- LLM 분류: 500ms-2s (API 호출)

**개선**: 평균 응답 시간 70%+ 감소

---

## 8. 구현 현황

| Phase | 작업 | 상태 |
|-------|------|------|
| Phase 1 | `ClassificationHeaders` 구조체 정의 | ✅ 완료 |
| Phase 2 | Gmail Adapter 헤더 추출 수정 | ✅ 완료 |
| Phase 3 | RFC Header Classifier 구현 | ✅ 완료 |
| Phase 4 | Pipeline Stage 순서 변경 | ✅ 완료 |
| Phase 5 | User Rules 분리 (Simple + LLM) | ✅ 완료 |
| Phase 6 | LLM 프롬프트에 User Rules 통합 | ✅ 완료 |
| Phase 7 | Outlook Adapter 동일 적용 | 🔲 예정 |
| Phase 8 | 테스트 및 모니터링 | 🔲 예정 |

### 구현된 파일

```
core/domain/classification_headers.go    # RFC 헤더 구조체
core/service/classification/
├── pipeline.go                          # 4-Stage 파이프라인
└── rfc_classifier.go                    # Stage 0: RFC 분류기
core/agent/llm/classify.go               # Stage 3: LLM + User Rules
adapter/out/provider/gmail_adapter.go    # Gmail RFC 헤더 추출
```

---

## 9. 참고 RFC 문서

- **RFC 2369**: The Use of URLs as Meta-Syntax for Core Mail List Commands
- **RFC 2919**: List-Id: A Structured Field and Namespace for the Identification of Mailing Lists
- **RFC 3834**: Recommendations for Automatic Responses to Electronic Mail
- **RFC 8058**: Signaling One-Click Functionality for List Email Headers
