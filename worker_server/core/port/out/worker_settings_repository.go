package out

import (
	"context"

	"github.com/google/uuid"
)

// SettingsRepository defines the outbound port for user settings persistence.
type SettingsRepository interface {
	// GetClassificationRules retrieves user's classification rules.
	GetClassificationRules(ctx context.Context, userID uuid.UUID) (*ClassificationRulesEntity, error)

	// UpsertClassificationRules creates or updates classification rules.
	UpsertClassificationRules(ctx context.Context, rules *ClassificationRulesEntity) error
}

// ClassificationRulesEntity represents classification rules for persistence.
type ClassificationRulesEntity struct {
	UserID string `json:"user_id" db:"user_id"`

	// 중요 발신자 도메인
	ImportantDomains []string `json:"important_domains" db:"important_domains"`

	// 중요 키워드
	ImportantKeywords []string `json:"important_keywords" db:"important_keywords"`

	// 무시할 발신자
	IgnoreSenders []string `json:"ignore_senders" db:"ignore_senders"`

	// 무시할 키워드
	IgnoreKeywords []string `json:"ignore_keywords" db:"ignore_keywords"`

	// 자유 형식 규칙 (LLM 프롬프트에 직접 포함)
	HighPriorityRules string `json:"high_priority_rules" db:"high_priority_rules"`
	LowPriorityRules  string `json:"low_priority_rules" db:"low_priority_rules"`
	CategoryRules     string `json:"category_rules" db:"category_rules"`
}

// HasCustomRules checks if user has any custom rules.
func (r *ClassificationRulesEntity) HasCustomRules() bool {
	if r == nil {
		return false
	}
	return len(r.ImportantDomains) > 0 ||
		len(r.ImportantKeywords) > 0 ||
		len(r.IgnoreSenders) > 0 ||
		len(r.IgnoreKeywords) > 0 ||
		r.HighPriorityRules != "" ||
		r.LowPriorityRules != "" ||
		r.CategoryRules != ""
}

// ToPromptContext converts rules to LLM prompt context.
func (r *ClassificationRulesEntity) ToPromptContext() string {
	if r == nil || !r.HasCustomRules() {
		return ""
	}

	var parts []string

	if len(r.ImportantDomains) > 0 {
		parts = append(parts, "IMPORTANT sender domains (treat as HIGH priority): "+joinSlice(r.ImportantDomains))
	}
	if len(r.ImportantKeywords) > 0 {
		parts = append(parts, "IMPORTANT keywords (boost priority when found): "+joinSlice(r.ImportantKeywords))
	}
	if len(r.IgnoreSenders) > 0 {
		parts = append(parts, "LOW priority senders (treat as low priority): "+joinSlice(r.IgnoreSenders))
	}
	if len(r.IgnoreKeywords) > 0 {
		parts = append(parts, "LOW priority keywords: "+joinSlice(r.IgnoreKeywords))
	}
	if r.HighPriorityRules != "" {
		parts = append(parts, "User's HIGH priority rules: "+r.HighPriorityRules)
	}
	if r.LowPriorityRules != "" {
		parts = append(parts, "User's LOW priority rules: "+r.LowPriorityRules)
	}
	if r.CategoryRules != "" {
		parts = append(parts, "User's category rules: "+r.CategoryRules)
	}

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n"
		}
		result += "- " + p
	}
	return result
}

func joinSlice(s []string) string {
	result := ""
	for i, v := range s {
		if i > 0 {
			result += ", "
		}
		result += v
	}
	return result
}
