// Package llm provides LLM client and utilities.
package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/goccy/go-json"

	"worker_server/core/domain"

	openai "github.com/sashabaranov/go-openai"
)

// =============================================================================
// Model Selection Strategy
// =============================================================================

// ModelType represents the type of model to use
type ModelType string

const (
	ModelMini     ModelType = "gpt-4o-mini" // 저비용, 단순 작업
	ModelStandard ModelType = "gpt-4o"      // 고품질, 복잡 작업
)

// TaskType represents the type of AI task
type TaskType string

const (
	TaskClassify   TaskType = "classify"    // 분류 → mini
	TaskPriority   TaskType = "priority"    // 우선순위 → mini
	TaskTag        TaskType = "tag"         // 태그 추출 → mini
	TaskSummarize  TaskType = "summarize"   // 요약 → mini (짧은) / standard (상세)
	TaskReply      TaskType = "reply"       // 답장 생성 → standard
	TaskIntent     TaskType = "intent"      // 의도 분석 → mini
	TaskToolSelect TaskType = "tool_select" // 도구 선택 → mini
)

// GetModelForTask returns the appropriate model for a task type
func GetModelForTask(task TaskType) ModelType {
	switch task {
	case TaskClassify, TaskPriority, TaskTag, TaskIntent, TaskToolSelect:
		return ModelMini
	case TaskSummarize:
		return ModelMini // 기본은 mini, 상세 요약은 별도 호출
	case TaskReply:
		return ModelStandard
	default:
		return ModelMini
	}
}

// =============================================================================
// Batch Classification
// =============================================================================

// BatchClassifyInput represents input for batch classification
type BatchClassifyInput struct {
	ID      int64  `json:"id"`
	Subject string `json:"subject"`
	From    string `json:"from"`
	Snippet string `json:"snippet"` // 본문 일부 (최대 500자)
}

// BatchClassifyResult represents result for a single email
type BatchClassifyResult struct {
	ID       int64    `json:"id"`
	Category string   `json:"category"`
	Priority float64  `json:"priority"`
	Tags     []string `json:"tags,omitempty"`
	Intent   string   `json:"intent,omitempty"`
}

// BatchClassifyResponse represents the full batch response
type BatchClassifyResponse struct {
	Results []BatchClassifyResult `json:"results"`
}

// ClassifyEmailBatch classifies multiple emails in a single API call
func (c *Client) ClassifyEmailBatch(ctx context.Context, emails []BatchClassifyInput, userRules []domain.ClassificationRule) ([]BatchClassifyResult, error) {
	if len(emails) == 0 {
		return nil, nil
	}

	// Build batch prompt
	prompt := buildBatchClassifyPrompt(emails, userRules)

	// Use mini model for classification
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: string(ModelMini),
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
		Temperature: 0.3, // 낮은 temperature로 일관성 확보
	})
	if err != nil {
		return nil, fmt.Errorf("batch classify failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from LLM")
	}

	// Parse response
	var batchResp BatchClassifyResponse
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &batchResp); err != nil {
		return nil, fmt.Errorf("failed to parse batch response: %w", err)
	}

	return batchResp.Results, nil
}

func buildBatchClassifyPrompt(emails []BatchClassifyInput, userRules []domain.ClassificationRule) string {
	var sb strings.Builder

	sb.WriteString(`다음 이메일들을 분류하세요.

카테고리 옵션:
- primary: 중요한 개인/업무 메일
- social: 소셜 네트워크, 데이팅
- promotions: 프로모션, 마케팅
- updates: 알림, 업데이트, 영수증
- forums: 메일링 리스트, 포럼

우선순위: 1(낮음) ~ 5(긴급)

의도 옵션:
- action_required: 조치 필요
- fyi: 참고용
- urgent: 긴급
- meeting: 미팅 관련
- newsletter: 뉴스레터
- receipt: 영수증/확인
- spam: 스팸

`)

	// Add user rules if any
	if len(userRules) > 0 {
		sb.WriteString("사용자 정의 규칙:\n")
		for _, rule := range userRules {
			// Build condition description from rule conditions and actions
			var condDesc string
			if len(rule.Conditions) > 0 {
				cond := rule.Conditions[0]
				condDesc = fmt.Sprintf("%s %s '%s'", cond.Field, cond.Operator, cond.Value)
			} else {
				condDesc = rule.Name
			}
			var actionDesc string
			for _, action := range rule.Actions {
				if action.Type == domain.ActionSetCategory {
					actionDesc = action.Value
					break
				}
			}
			if actionDesc == "" && len(rule.Actions) > 0 {
				actionDesc = string(rule.Actions[0].Type)
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n", condDesc, actionDesc))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("이메일 목록:\n\n")

	for _, email := range emails {
		snippet := truncateText(email.Snippet, 300)
		sb.WriteString(fmt.Sprintf("[%d]\nFrom: %s\nSubject: %s\nSnippet: %s\n\n",
			email.ID, email.From, email.Subject, snippet))
	}

	sb.WriteString(`JSON 형식으로 응답 (priority는 0.0~1.0 범위):
{
  "results": [
    {"id": 1, "category": "primary", "priority": 0.75, "tags": ["meeting"], "intent": "action_required"},
    ...
  ]
}`)

	return sb.String()
}

// =============================================================================
// Token Optimization
// =============================================================================

// PrepareEmailForLLM prepares email content for LLM processing
func PrepareEmailForLLM(subject, body, from string, maxBodyLength int) string {
	// Clean body
	cleanBody := CleanEmailBody(body)

	// Truncate if needed
	if len(cleanBody) > maxBodyLength {
		cleanBody = cleanBody[:maxBodyLength] + "..."
	}

	return fmt.Sprintf("From: %s\nSubject: %s\nBody:\n%s", from, subject, cleanBody)
}

// CleanEmailBody cleans email body for LLM processing
func CleanEmailBody(body string) string {
	// Remove HTML tags if present
	htmlPattern := regexp.MustCompile(`<[^>]*>`)
	body = htmlPattern.ReplaceAllString(body, "")

	// Remove excessive whitespace
	whitespacePattern := regexp.MustCompile(`\s+`)
	body = whitespacePattern.ReplaceAllString(body, " ")

	// Remove common signatures
	signaturePatterns := []string{
		`(?i)--\s*\n.*`,        // -- signature
		`(?i)sent from my.*`,   // Sent from my iPhone
		`(?i)regards,?\s*\n.*`, // Regards,
		`(?i)best,?\s*\n.*`,    // Best,
		`(?i)thanks,?\s*\n.*`,  // Thanks,
		`(?i)cheers,?\s*\n.*`,  // Cheers,
	}

	for _, pattern := range signaturePatterns {
		re := regexp.MustCompile(pattern)
		body = re.ReplaceAllString(body, "")
	}

	// Remove quoted text (lines starting with >)
	quotedPattern := regexp.MustCompile(`(?m)^>.*$`)
	body = quotedPattern.ReplaceAllString(body, "")

	// Remove "On ... wrote:" patterns
	onWrotePattern := regexp.MustCompile(`(?i)on .* wrote:.*`)
	body = onWrotePattern.ReplaceAllString(body, "")

	// Trim and clean up
	body = strings.TrimSpace(body)

	return body
}

// truncateText truncates text to maxLen characters
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// =============================================================================
// Batch Summarize
// =============================================================================

// BatchSummarizeInput represents input for batch summarization
type BatchSummarizeInput struct {
	ID      int64  `json:"id"`
	Subject string `json:"subject"`
	Body    string `json:"body"` // 이미 정제된 본문
}

// BatchSummarizeResult represents result for a single email
type BatchSummarizeResult struct {
	ID      int64  `json:"id"`
	Summary string `json:"summary"`
}

// SummarizeEmailBatch summarizes multiple emails in a single API call
func (c *Client) SummarizeEmailBatch(ctx context.Context, emails []BatchSummarizeInput) ([]BatchSummarizeResult, error) {
	if len(emails) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	sb.WriteString("다음 이메일들을 각각 2-3문장으로 요약하세요.\n\n")

	for _, email := range emails {
		body := truncateText(email.Body, 1000)
		sb.WriteString(fmt.Sprintf("[%d]\nSubject: %s\nBody: %s\n\n", email.ID, email.Subject, body))
	}

	sb.WriteString(`JSON 형식으로 응답:
{
  "results": [
    {"id": 1, "summary": "요약 내용..."},
    ...
  ]
}`)

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: string(ModelMini),
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: sb.String(),
			},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response")
	}

	var result struct {
		Results []BatchSummarizeResult `json:"results"`
	}
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		return nil, err
	}

	return result.Results, nil
}

// =============================================================================
// Cost Tracking
// =============================================================================

// TokenUsage tracks token usage for cost calculation
type TokenUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	TotalTokens      int     `json:"total_tokens"`
	Model            string  `json:"model"`
	EstimatedCost    float64 `json:"estimated_cost_usd"`
}

// Pricing per 1M tokens (as of 2024)
var modelPricing = map[string]struct {
	InputPer1M  float64
	OutputPer1M float64
}{
	"gpt-4o-mini": {InputPer1M: 0.15, OutputPer1M: 0.60},
	"gpt-4o":      {InputPer1M: 5.00, OutputPer1M: 15.00},
}

// CalculateCost calculates estimated cost for token usage
func CalculateCost(model string, promptTokens, completionTokens int) float64 {
	pricing, ok := modelPricing[model]
	if !ok {
		return 0
	}

	inputCost := float64(promptTokens) / 1_000_000 * pricing.InputPer1M
	outputCost := float64(completionTokens) / 1_000_000 * pricing.OutputPer1M

	return inputCost + outputCost
}
