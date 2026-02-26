package out

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ProfileRepository defines the outbound port for profile persistence.
type ProfileRepository interface {
	// GetByUserID retrieves profile by user ID.
	GetByUserID(ctx context.Context, userID uuid.UUID) (*ProfileEntity, error)

	// Upsert creates or updates a profile.
	Upsert(ctx context.Context, profile *ProfileEntity) error

	// UpdateAnalysisResult updates profile with analysis results.
	UpdateAnalysisResult(ctx context.Context, userID uuid.UUID, result *ProfileAnalysisResult) error

	// GetStaleProfiles returns profiles that need re-analysis.
	GetStaleProfiles(ctx context.Context, before time.Time, limit int) ([]*ProfileEntity, error)

	// UpdateNextAnalysis sets the next analysis time.
	UpdateNextAnalysis(ctx context.Context, userID uuid.UUID, nextAt time.Time) error
}

// ProfileEntity represents profile data for persistence.
type ProfileEntity struct {
	ID     int64
	UserID uuid.UUID

	// Job/Role
	JobRole   string
	Industry  string
	Seniority string

	// Personality
	Personality []string

	// Writing style
	Formality      string
	Tone           string
	AvgEmailLength string

	// Common phrases
	CommonPhrases map[string]interface{}

	// Language
	PrimaryLanguage    string
	SecondaryLanguages []string

	// Analysis metadata
	SampleCount     int
	LastAnalyzedAt  *time.Time
	NextAnalysisAt  *time.Time
	AnalysisVersion int
	RawAnalysis     map[string]interface{}

	CreatedAt time.Time
	UpdatedAt time.Time
}

// ProfileAnalysisResult represents LLM analysis output.
type ProfileAnalysisResult struct {
	JobRole            string                 `json:"job_role"`
	Industry           string                 `json:"industry"`
	Seniority          string                 `json:"seniority"`
	Personality        []string               `json:"personality"`
	Formality          string                 `json:"formality"`
	Tone               string                 `json:"tone"`
	AvgEmailLength     string                 `json:"avg_email_length"`
	CommonPhrases      map[string]interface{} `json:"common_phrases"`
	PrimaryLanguage    string                 `json:"primary_language"`
	SecondaryLanguages []string               `json:"secondary_languages"`
	SampleCount        int                    `json:"sample_count"`
	RawAnalysis        map[string]interface{} `json:"raw_analysis,omitempty"`
}
