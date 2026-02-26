package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// UserSettings contains user preferences for the application.
// Note: Notification-specific settings are in NotificationSettings (notification.go)
type UserSettings struct {
	ID     int64     `json:"id"`
	UserID uuid.UUID `json:"user_id"`

	// Email settings
	DefaultSignature *string `json:"default_signature,omitempty"`
	AutoReplyEnabled bool    `json:"auto_reply_enabled"`
	AutoReplyMessage *string `json:"auto_reply_message,omitempty"`

	// AI settings
	AIEnabled      bool   `json:"ai_enabled"`
	AIAutoClassify bool   `json:"ai_auto_classify"`
	AITone         string `json:"ai_tone"` // professional, casual, friendly

	// UI preferences
	Theme    string `json:"theme"` // light, dark, system
	Language string `json:"language"`
	Timezone string `json:"timezone"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DefaultUserSettings returns default settings for new users.
func DefaultUserSettings(userID uuid.UUID) *UserSettings {
	return &UserSettings{
		UserID: userID,

		// AI
		AIEnabled:      true,
		AIAutoClassify: true,
		AITone:         "professional",

		// UI
		Theme:    "system",
		Language: "ko",
		Timezone: "Asia/Seoul",
	}
}

// ClassificationRules defines user-specific email classification rules.
// Rules are divided into two types:
// 1. Simple Rules (Stage 1): Domain/Sender/Keyword matching - no LLM required
// 2. LLM Rules (Stage 3): Natural language rules interpreted by LLM
type ClassificationRules struct {
	ID     int64     `json:"id"`
	UserID uuid.UUID `json:"user_id"`

	// ==========================================================================
	// Simple Rules (Stage 1) - No LLM required
	// ==========================================================================

	// Important patterns → Work category, High priority
	ImportantDomains  []string `json:"important_domains"`  // "@company.com" → high priority
	ImportantSenders  []string `json:"important_senders"`  // "ceo@company.com" → high priority
	ImportantKeywords []string `json:"important_keywords"` // "긴급", "마감" → high priority

	// Ignore patterns → Other category, Low priority
	IgnoreSenders  []string `json:"ignore_senders"`  // "newsletter@" → low priority
	IgnoreKeywords []string `json:"ignore_keywords"` // "광고" → low priority

	// ==========================================================================
	// LLM Rules (Stage 3) - Natural language for LLM interpretation
	// ==========================================================================

	// Priority rules (natural language)
	HighPriorityRules string `json:"high_priority_rules"` // "CEO나 임원진 메일은 항상 긴급"
	LowPriorityRules  string `json:"low_priority_rules"`  // "내부 공지는 낮은 우선순위"

	// Category rules (natural language)
	CategoryRules string `json:"category_rules"` // "HR팀 메일은 admin 카테고리"

	// Custom instructions (free-form)
	CustomInstructions string `json:"custom_instructions"` // "클라이언트 피드백은 최우선으로 처리"

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ToRuleList converts ClassificationRules to a list of ClassificationRule for LLM prompt
func (r *ClassificationRules) ToRuleList() []ClassificationRule {
	var rules []ClassificationRule

	// Add domain rules
	for _, domain := range r.ImportantDomains {
		desc := "Emails from " + domain + " are high priority"
		rules = append(rules, ClassificationRule{
			Name:        "important_domain",
			Description: &desc,
		})
	}

	// Add keyword rules
	for _, keyword := range r.ImportantKeywords {
		desc := "Emails containing '" + keyword + "' are high priority"
		rules = append(rules, ClassificationRule{
			Name:        "important_keyword",
			Description: &desc,
		})
	}

	// Add custom rules
	if r.HighPriorityRules != "" {
		rules = append(rules, ClassificationRule{
			Name:        "high_priority",
			Description: &r.HighPriorityRules,
		})
	}
	if r.LowPriorityRules != "" {
		rules = append(rules, ClassificationRule{
			Name:        "low_priority",
			Description: &r.LowPriorityRules,
		})
	}
	if r.CategoryRules != "" {
		rules = append(rules, ClassificationRule{
			Name:        "category",
			Description: &r.CategoryRules,
		})
	}

	return rules
}

// SettingsRepository defines the repository interface for user settings.
type SettingsRepository interface {
	GetByUserID(userID uuid.UUID) (*UserSettings, error)
	Create(settings *UserSettings) error
	Update(settings *UserSettings) error

	// Classification rules
	GetClassificationRules(ctx context.Context, userID uuid.UUID) (*ClassificationRules, error)
	SaveClassificationRules(ctx context.Context, rules *ClassificationRules) error
}
