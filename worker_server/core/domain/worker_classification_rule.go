package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Classification Rule Types
// =============================================================================

// RuleType represents the type of classification rule
type RuleType string

const (
	// RuleTypeExactSender matches exact email address (highest priority, 0.99 score)
	RuleTypeExactSender RuleType = "exact_sender"

	// RuleTypeSenderDomain matches sender domain (0.95 score)
	RuleTypeSenderDomain RuleType = "sender_domain"

	// RuleTypeSubjectKeyword matches keywords in subject (0.90 score)
	RuleTypeSubjectKeyword RuleType = "subject_keyword"

	// RuleTypeBodyKeyword matches keywords in body (0.85 score)
	RuleTypeBodyKeyword RuleType = "body_keyword"

	// RuleTypeAIPrompt uses LLM to evaluate natural language rules
	RuleTypeAIPrompt RuleType = "ai_prompt"
)

// DefaultScoreForRuleType returns the default score for a rule type
func DefaultScoreForRuleType(rt RuleType) float64 {
	switch rt {
	case RuleTypeExactSender:
		return 0.99
	case RuleTypeSenderDomain:
		return 0.95
	case RuleTypeSubjectKeyword:
		return 0.90
	case RuleTypeBodyKeyword:
		return 0.85
	case RuleTypeAIPrompt:
		return 0.85 // LLM will override this
	default:
		return 0.80
	}
}

// ScoreRuleAction represents the action to take when a score rule matches
type ScoreRuleAction string

const (
	ScoreRuleActionAssignCategory ScoreRuleAction = "assign_category"
	ScoreRuleActionAssignPriority ScoreRuleAction = "assign_priority"
	ScoreRuleActionAssignLabel    ScoreRuleAction = "assign_label"
	ScoreRuleActionMarkImportant  ScoreRuleAction = "mark_important"
	ScoreRuleActionMarkSpam       ScoreRuleAction = "mark_spam"
)

// =============================================================================
// Classification Rule
// =============================================================================

// ScoreClassificationRule represents a user-defined classification rule (v2, score-based)
type ScoreClassificationRule struct {
	ID       int64           `json:"id"`
	UserID   uuid.UUID       `json:"user_id"`
	Type     RuleType        `json:"type"`
	Pattern  string          `json:"pattern"`
	Action   ScoreRuleAction `json:"action"`
	Value    string          `json:"value"` // category name, priority name, label_id, etc.
	Score    float64         `json:"score"` // 0.0 - 1.0
	Position int             `json:"position"`
	IsActive bool            `json:"is_active"`

	// Stats
	HitCount  int        `json:"hit_count"`
	LastHitAt *time.Time `json:"last_hit_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ScoreClassificationRuleRepository interface for score classification rule operations
type ScoreClassificationRuleRepository interface {
	// CRUD
	GetByID(ctx context.Context, id int64) (*ScoreClassificationRule, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*ScoreClassificationRule, error)
	ListByUserAndType(ctx context.Context, userID uuid.UUID, ruleType RuleType) ([]*ScoreClassificationRule, error)
	ListActiveByUser(ctx context.Context, userID uuid.UUID) ([]*ScoreClassificationRule, error)
	Create(ctx context.Context, rule *ScoreClassificationRule) error
	Update(ctx context.Context, rule *ScoreClassificationRule) error
	Delete(ctx context.Context, id int64) error

	// Bulk operations
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
	UpdatePositions(ctx context.Context, userID uuid.UUID, ruleIDs []int64) error

	// Stats
	IncrementHitCount(ctx context.Context, id int64) error
}

// =============================================================================
// Label Rule (Auto Labeling)
// =============================================================================

// LabelRuleType represents the type of label rule
type LabelRuleType string

const (
	// LabelRuleExactSender matches exact email address (0.99 score)
	LabelRuleExactSender LabelRuleType = "exact_sender"

	// LabelRuleSenderDomain matches sender domain (0.95 score)
	LabelRuleSenderDomain LabelRuleType = "sender_domain"

	// LabelRuleSubjectKeyword matches keywords in subject (0.90 score)
	LabelRuleSubjectKeyword LabelRuleType = "subject_keyword"

	// LabelRuleBodyKeyword matches keywords in body (0.85 score)
	LabelRuleBodyKeyword LabelRuleType = "body_keyword"

	// LabelRuleEmbedding uses embedding similarity (pattern = "ref:{email_id}")
	LabelRuleEmbedding LabelRuleType = "embedding"

	// LabelRuleAIPrompt uses LLM to evaluate natural language rules
	LabelRuleAIPrompt LabelRuleType = "ai_prompt"
)

// DefaultScoreForLabelRuleType returns the default score for a label rule type
func DefaultScoreForLabelRuleType(rt LabelRuleType) float64 {
	switch rt {
	case LabelRuleExactSender:
		return 0.99
	case LabelRuleSenderDomain:
		return 0.95
	case LabelRuleSubjectKeyword:
		return 0.90
	case LabelRuleBodyKeyword:
		return 0.85
	case LabelRuleEmbedding:
		return 0.90
	case LabelRuleAIPrompt:
		return 0.85
	default:
		return 0.80
	}
}

// LabelRule represents a rule for auto-labeling emails
type LabelRule struct {
	ID            int64         `json:"id"`
	UserID        uuid.UUID     `json:"user_id"`
	LabelID       int64         `json:"label_id"`
	Type          LabelRuleType `json:"type"`
	Pattern       string        `json:"pattern"` // For embedding: "ref:{email_id}"
	Score         float64       `json:"score"`
	IsAutoCreated bool          `json:"is_auto_created"` // Created by auto-learning
	IsActive      bool          `json:"is_active"`

	// Stats
	HitCount  int        `json:"hit_count"`
	LastHitAt *time.Time `json:"last_hit_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LabelRuleRepository interface for label rule operations
type LabelRuleRepository interface {
	// CRUD
	GetByID(ctx context.Context, id int64) (*LabelRule, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]*LabelRule, error)
	ListByLabel(ctx context.Context, labelID int64) ([]*LabelRule, error)
	ListByUserAndType(ctx context.Context, userID uuid.UUID, ruleType LabelRuleType) ([]*LabelRule, error)
	ListActiveByUser(ctx context.Context, userID uuid.UUID) ([]*LabelRule, error)
	FindByPattern(ctx context.Context, userID uuid.UUID, labelID int64, ruleType LabelRuleType, pattern string) (*LabelRule, error)
	Create(ctx context.Context, rule *LabelRule) error
	Update(ctx context.Context, rule *LabelRule) error
	Delete(ctx context.Context, id int64) error

	// Bulk operations
	DeleteByLabel(ctx context.Context, labelID int64) error
	DeleteAutoCreatedByLabel(ctx context.Context, labelID int64) error

	// Stats
	IncrementHitCount(ctx context.Context, id int64) error
}

// =============================================================================
// Classification Cache (Semantic Cache)
// =============================================================================

// ClassificationCache represents a cached classification result based on embedding similarity
type ClassificationCache struct {
	ID          int64     `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Embedding   []float32 `json:"embedding"` // 1536 dimensions
	Category    string    `json:"category"`
	SubCategory *string   `json:"sub_category,omitempty"`
	Priority    string    `json:"priority"`
	Labels      []int64   `json:"labels"`
	Score       float64   `json:"score"` // LLM confidence

	// Stats
	UsageCount int       `json:"usage_count"`
	LastUsedAt time.Time `json:"last_used_at"`
	ExpiresAt  time.Time `json:"expires_at"` // 30 days TTL

	CreatedAt time.Time `json:"created_at"`
}

// ClassificationCacheRepository interface for classification cache operations
type ClassificationCacheRepository interface {
	// Search
	FindSimilar(ctx context.Context, userID uuid.UUID, embedding []float32, minScore float64, limit int) ([]*ClassificationCache, error)

	// CRUD
	GetByID(ctx context.Context, id int64) (*ClassificationCache, error)
	Create(ctx context.Context, cache *ClassificationCache) error
	Delete(ctx context.Context, id int64) error

	// Bulk operations
	DeleteExpired(ctx context.Context) (int, error)
	DeleteByUser(ctx context.Context, userID uuid.UUID) error

	// Stats
	IncrementUsageCount(ctx context.Context, id int64) error
}

// =============================================================================
// Score Result (Pipeline Stage Output)
// =============================================================================

// ScoreResult represents the result from a classification stage
type ScoreResult struct {
	Category    EmailCategory     `json:"category"`
	SubCategory *EmailSubCategory `json:"sub_category,omitempty"`
	Priority    Priority          `json:"priority"`
	Labels      []int64           `json:"labels,omitempty"`
	Score       float64           `json:"score"` // 0.0 - 1.0
	Source      string            `json:"source"`
	Signals     []string          `json:"signals,omitempty"`
	LLMUsed     bool              `json:"llm_used"`
}

// ClassificationStage represents the stage that performed the classification
type ClassificationStage string

const (
	ClassificationStageRFC    ClassificationStage = "rfc"
	ClassificationStageSender ClassificationStage = "sender"
	ClassificationStageRule   ClassificationStage = "rule"
	ClassificationStageCache  ClassificationStage = "cache"
	ClassificationStageLLM    ClassificationStage = "llm"
)
