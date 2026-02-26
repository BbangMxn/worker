package domain

import (
	"time"

	"github.com/google/uuid"
)

type UserProfile struct {
	ID     int64     `json:"id"`
	UserID uuid.UUID `json:"user_id"`

	// Writing style analysis
	ToneProfile     *ToneProfile     `json:"tone_profile,omitempty"`
	WritingPatterns *WritingPatterns `json:"writing_patterns,omitempty"`

	// Preferences learned from behavior
	PreferredTopics []string `json:"preferred_topics,omitempty"`
	CommonPhrases   []string `json:"common_phrases,omitempty"`
	SignatureStyle  *string  `json:"signature_style,omitempty"`

	// Statistics
	TotalEmailsAnalyzed int        `json:"total_emails_analyzed"`
	LastAnalyzedAt      *time.Time `json:"last_analyzed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ToneProfile struct {
	Formality    float64 `json:"formality"`    // 0-1: casual to formal
	Friendliness float64 `json:"friendliness"` // 0-1: neutral to friendly
	Directness   float64 `json:"directness"`   // 0-1: indirect to direct
	Enthusiasm   float64 `json:"enthusiasm"`   // 0-1: neutral to enthusiastic
}

type WritingPatterns struct {
	AvgSentenceLength  float64  `json:"avg_sentence_length"`
	AvgParagraphLength float64  `json:"avg_paragraph_length"`
	UsesEmoji          bool     `json:"uses_emoji"`
	UsesExclamation    bool     `json:"uses_exclamation"`
	CommonGreetings    []string `json:"common_greetings"`
	CommonClosings     []string `json:"common_closings"`
}

type ProfileRepository interface {
	GetByUserID(userID uuid.UUID) (*UserProfile, error)
	Create(profile *UserProfile) error
	Update(profile *UserProfile) error
}
