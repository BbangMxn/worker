package domain

import (
	"time"

	"github.com/google/uuid"
)

// TemplateCategory defines template categories
type TemplateCategory string

const (
	TemplateCategorySignature TemplateCategory = "signature"
	TemplateCategoryReply     TemplateCategory = "reply"
	TemplateCategoryFollowUp  TemplateCategory = "follow_up"
	TemplateCategoryIntro     TemplateCategory = "intro"
	TemplateCategoryThankYou  TemplateCategory = "thank_you"
	TemplateCategoryMeeting   TemplateCategory = "meeting"
	TemplateCategoryCustom    TemplateCategory = "custom"
)

// EmailTemplate represents an email template
type EmailTemplate struct {
	ID         int64
	UserID     uuid.UUID
	Name       string
	Category   TemplateCategory
	Subject    *string
	Body       string
	HTMLBody   *string
	Variables  []TemplateVariable
	Tags       []string
	IsDefault  bool
	IsArchived bool
	UsageCount int
	LastUsedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TemplateVariable represents a variable placeholder in template
type TemplateVariable struct {
	Name        string `json:"name"`          // e.g., "recipientName"
	Placeholder string `json:"placeholder"`   // e.g., "${recipientName}"
	DefaultVal  string `json:"default_value"` // default value if not provided
	Description string `json:"description"`   // for UI tooltip
}

// TemplateFilter for querying templates
type TemplateFilter struct {
	UserID     uuid.UUID
	Category   *TemplateCategory
	Search     *string
	Tags       []string
	IsDefault  *bool
	IsArchived *bool
	Limit      int
	Offset     int
	OrderBy    string
	Order      string
}

// TemplateListItem is a lightweight version for list views
type TemplateListItem struct {
	ID         int64
	Name       string
	Category   TemplateCategory
	Subject    *string
	Preview    string // First 100 chars of body
	Tags       []string
	IsDefault  bool
	UsageCount int
	LastUsedAt *time.Time
	UpdatedAt  time.Time
}

// ToListItem converts EmailTemplate to TemplateListItem
func (t *EmailTemplate) ToListItem() *TemplateListItem {
	preview := t.Body
	if len(preview) > 100 {
		preview = preview[:100] + "..."
	}

	return &TemplateListItem{
		ID:         t.ID,
		Name:       t.Name,
		Category:   t.Category,
		Subject:    t.Subject,
		Preview:    preview,
		Tags:       t.Tags,
		IsDefault:  t.IsDefault,
		UsageCount: t.UsageCount,
		LastUsedAt: t.LastUsedAt,
		UpdatedAt:  t.UpdatedAt,
	}
}

// DefaultVariables returns common template variables
func DefaultVariables() []TemplateVariable {
	return []TemplateVariable{
		{Name: "recipientName", Placeholder: "${recipientName}", DefaultVal: "", Description: "Recipient's name"},
		{Name: "recipientEmail", Placeholder: "${recipientEmail}", DefaultVal: "", Description: "Recipient's email"},
		{Name: "senderName", Placeholder: "${senderName}", DefaultVal: "", Description: "Your name"},
		{Name: "senderEmail", Placeholder: "${senderEmail}", DefaultVal: "", Description: "Your email"},
		{Name: "date", Placeholder: "${date}", DefaultVal: "", Description: "Current date"},
		{Name: "time", Placeholder: "${time}", DefaultVal: "", Description: "Current time"},
		{Name: "companyName", Placeholder: "${companyName}", DefaultVal: "", Description: "Company name"},
		{Name: "signature", Placeholder: "${signature}", DefaultVal: "", Description: "Your email signature"},
	}
}

// ValidCategories returns all valid template categories
func ValidTemplateCategories() []TemplateCategory {
	return []TemplateCategory{
		TemplateCategorySignature,
		TemplateCategoryReply,
		TemplateCategoryFollowUp,
		TemplateCategoryIntro,
		TemplateCategoryThankYou,
		TemplateCategoryMeeting,
		TemplateCategoryCustom,
	}
}

// IsValidCategory checks if a category is valid
func IsValidTemplateCategory(category string) bool {
	for _, c := range ValidTemplateCategories() {
		if string(c) == category {
			return true
		}
	}
	return false
}
