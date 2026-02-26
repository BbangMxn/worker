package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/goccy/go-json"

	"worker_server/core/domain"
)

// ClassificationResponse is the legacy response format
type ClassificationResponse struct {
	Category string   `json:"category"`
	Priority float64  `json:"priority"`
	Summary  string   `json:"summary"`
	Tags     []string `json:"tags"`
	Score    float64  `json:"score"`
}

// EnhancedClassificationResponse is the new response format with sub-category
type EnhancedClassificationResponse struct {
	Category    string   `json:"category"`
	SubCategory string   `json:"sub_category,omitempty"`
	Priority    float64  `json:"priority"`
	Summary     string   `json:"summary"`
	Tags        []string `json:"tags"`
	Score       float64  `json:"score"`
}

// ClassifyEmail performs legacy email classification (kept for backward compatibility)
func (c *Client) ClassifyEmail(ctx context.Context, subject, body, from string, userRules []domain.ClassificationRule) (*ClassificationResponse, error) {
	// Build rules context
	rulesContext := ""
	if len(userRules) > 0 {
		rulesContext = "\n\nUser-defined classification rules:\n"
		for _, rule := range userRules {
			rulesContext += fmt.Sprintf("- %s: %s\n", rule.Name, *rule.Description)
		}
	}

	systemPrompt := `You are an email classification AI. Analyze the email and respond with JSON only.

Categories: primary, social, promotion, updates, forums
Priority: 0.0 (lowest) to 1.0 (urgent)
  - 0.80-1.00: Urgent (needs immediate attention)
  - 0.60-0.79: High (should read soon)
  - 0.40-0.59: Normal (standard priority)
  - 0.20-0.39: Low (not urgent)
  - 0.00-0.19: Lowest (can read later)

Respond with this exact JSON format:
{
  "category": "primary|social|promotion|updates|forums",
  "priority": 0.0-1.0,
  "summary": "brief 1-2 sentence summary",
  "tags": ["tag1", "tag2"],
  "score": 0.0-1.0
}` + rulesContext

	userPrompt := fmt.Sprintf("From: %s\nSubject: %s\n\nBody:\n%s", from, subject, truncateBody(body, 2000))

	resp, err := c.CompleteWithSystem(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	var result ClassificationResponse
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return nil, fmt.Errorf("failed to parse classification response: %w", err)
	}

	return &result, nil
}

// ClassifyEmailEnhanced performs enhanced email classification with new category system
func (c *Client) ClassifyEmailEnhanced(ctx context.Context, email *domain.Email, body string) (*EnhancedClassificationResponse, error) {
	systemPrompt := `You are an email classification AI. Analyze the email and respond with JSON only.

Categories (pick ONE):
- primary: Important personal or work emails requiring attention
- work: Work-related emails (meetings, projects, colleagues)
- personal: Personal emails from friends/family
- newsletter: Subscribed newsletters and digests
- notification: System notifications, alerts, updates
- marketing: Promotional content, advertisements
- social: Social media notifications
- finance: Banking, payments, invoices, receipts
- travel: Flight, hotel, travel bookings
- shopping: Order confirmations, shipping, delivery
- spam: Unwanted or suspicious emails
- other: Doesn't fit other categories

Sub-categories (optional, pick if applicable):
- receipt: Purchase receipts
- invoice: Bills and invoices
- shipping: Delivery tracking, shipping updates
- order: Order confirmations
- travel: Flight/hotel confirmations
- calendar: Meeting invites, calendar events
- account: Account notifications, password resets
- security: Security alerts, 2FA codes
- sns: Social network notifications
- comment: Comments, replies on platforms
- newsletter: Newsletter content
- marketing: Marketing campaigns
- deal: Deals, coupons, promotions

Priority: 0.0 (lowest) to 1.0 (urgent)
  - 0.80-1.00: Urgent (needs immediate attention)
  - 0.60-0.79: High (should read soon)
  - 0.40-0.59: Normal (standard priority)
  - 0.20-0.39: Low (not urgent)
  - 0.00-0.19: Lowest (can read later)

Respond with this exact JSON format:
{
  "category": "category_name",
  "sub_category": "sub_category_name or empty",
  "priority": 0.0-1.0,
  "summary": "brief 1-2 sentence summary",
  "tags": ["tag1", "tag2"],
  "score": 0.0-1.0
}`

	userPrompt := fmt.Sprintf("From: %s\nSubject: %s\n\nBody:\n%s", email.FromEmail, email.Subject, truncateBody(body, 2000))

	resp, err := c.CompleteWithSystem(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	var result EnhancedClassificationResponse
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return nil, fmt.Errorf("failed to parse classification response: %w", err)
	}

	return &result, nil
}

// UserLLMRules contains user's natural language classification rules for LLM.
// These rules are interpreted by LLM to customize classification behavior.
type UserLLMRules struct {
	HighPriorityRules  string // "CEO나 임원진 메일은 항상 긴급"
	LowPriorityRules   string // "내부 공지는 낮은 우선순위"
	CategoryRules      string // "HR팀 메일은 admin 카테고리"
	CustomInstructions string // 추가 지시사항
}

// ClassifyEmailWithUserRules performs email classification with user's natural language rules.
// This is used in Stage 3 of the classification pipeline.
func (c *Client) ClassifyEmailWithUserRules(ctx context.Context, email *domain.Email, body string, userRules *UserLLMRules) (*EnhancedClassificationResponse, error) {
	// Build user rules context
	userRulesContext := ""
	if userRules != nil {
		var rulesParts []string

		if userRules.HighPriorityRules != "" {
			rulesParts = append(rulesParts, fmt.Sprintf("High Priority Rules: %s", userRules.HighPriorityRules))
		}
		if userRules.LowPriorityRules != "" {
			rulesParts = append(rulesParts, fmt.Sprintf("Low Priority Rules: %s", userRules.LowPriorityRules))
		}
		if userRules.CategoryRules != "" {
			rulesParts = append(rulesParts, fmt.Sprintf("Category Rules: %s", userRules.CategoryRules))
		}
		if userRules.CustomInstructions != "" {
			rulesParts = append(rulesParts, fmt.Sprintf("Custom Instructions: %s", userRules.CustomInstructions))
		}

		if len(rulesParts) > 0 {
			userRulesContext = "\n\n## User-Defined Rules (MUST follow):\n" + strings.Join(rulesParts, "\n")
		}
	}

	systemPrompt := `You are an email classification AI. Analyze the email and respond with JSON only.

## Categories (pick ONE):
- work: Work-related emails (meetings, projects, colleagues, clients)
- personal: Personal emails from friends/family
- finance: Banking, payments, invoices, receipts
- travel: Flight, hotel, travel bookings
- shopping: Order confirmations, shipping, delivery
- other: Doesn't fit other categories

Note: Newsletters, marketing, notifications, and spam are already filtered before reaching you.
Focus on distinguishing work vs personal emails and their importance.

## Sub-categories (optional):
- receipt, invoice, shipping, order, travel, calendar, account, security

## Priority (0.0 to 1.0):
- 0.80-1.00: Urgent (needs immediate attention)
- 0.60-0.79: High (should read soon)
- 0.40-0.59: Normal (standard priority)
- 0.20-0.39: Low (not urgent)
- 0.00-0.19: Lowest (can read later)` + userRulesContext + `

Respond with this exact JSON format:
{
  "category": "category_name",
  "sub_category": "sub_category_name or empty",
  "priority": 0.0-1.0,
  "summary": "brief 1-2 sentence summary in the same language as the email",
  "tags": ["tag1", "tag2"],
  "score": 0.0-1.0
}`

	fromName := ""
	if email.FromName != nil {
		fromName = *email.FromName
	}
	userPrompt := fmt.Sprintf("From: %s <%s>\nSubject: %s\nDate: %s\n\nBody:\n%s",
		fromName, email.FromEmail, email.Subject,
		email.ReceivedAt.Format("2006-01-02 15:04"),
		truncateBody(body, 2000))

	resp, err := c.CompleteWithSystem(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	// Parse JSON response
	var result EnhancedClassificationResponse
	resp = strings.TrimPrefix(resp, "```json")
	resp = strings.TrimSuffix(resp, "```")
	resp = strings.TrimSpace(resp)

	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return nil, fmt.Errorf("failed to parse classification response: %w", err)
	}

	return &result, nil
}

func truncateBody(body string, maxLen int) string {
	if len(body) <= maxLen {
		return body
	}
	return body[:maxLen] + "..."
}
