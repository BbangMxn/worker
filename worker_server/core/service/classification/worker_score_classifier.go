// Package classification implements the score-based email classification pipeline.
//
// 5-Stage Pipeline (optimized for LLM cost reduction ~85-95%):
//
//	Stage 0: RFC Headers      (~55%)  → List-Unsubscribe, Precedence, ESP detection
//	Stage 1: Sender Profile   (~15%)  → Importance score based on engagement
//	Stage 2: User Rules       (~12%)  → Domain/Keyword matching
//	Stage 3: Semantic Cache   (~10%)  → Embedding similarity search
//	Stage 4: LLM Fallback     (~8%)   → Last resort
//
// Each stage returns a score (0.0-1.0), and the highest score wins.
package classification

import (
	"context"

	"worker_server/core/domain"
	"worker_server/core/port/out"

	"github.com/google/uuid"
)

// =============================================================================
// Score Classifier Interface
// =============================================================================

// ScoreClassifierInput contains all inputs needed for score-based classification
type ScoreClassifierInput struct {
	UserID    uuid.UUID
	Email     *domain.Email
	Headers   *out.ProviderClassificationHeaders
	Body      string
	Embedding []float32 // Pre-computed embedding (from RAG indexing)
}

// ScoreClassifierResult contains the result from a score classifier
type ScoreClassifierResult struct {
	Category    domain.EmailCategory
	SubCategory *domain.EmailSubCategory
	Priority    domain.Priority
	Labels      []int64
	Score       float64  // 0.0 - 1.0
	Source      string   // classifier name
	Signals     []string // detected signals (for debugging)
	LLMUsed     bool
}

// ScoreClassifier is the interface for all score-based classifiers
type ScoreClassifier interface {
	// Name returns the classifier name (for logging)
	Name() string

	// Stage returns the pipeline stage number (0-4)
	Stage() int

	// Classify performs classification and returns a scored result
	// Returns nil if the classifier cannot classify the input (skip to next stage)
	Classify(ctx context.Context, input *ScoreClassifierInput) (*ScoreClassifierResult, error)
}

// =============================================================================
// Score Pipeline Configuration
// =============================================================================

// ScorePipelineConfig holds configuration for the score pipeline
type ScorePipelineConfig struct {
	// EarlyExitThreshold: stop pipeline if score >= this value
	EarlyExitThreshold float64 // Default: 0.95

	// LLMFallbackThreshold: call LLM if best score < this value
	LLMFallbackThreshold float64 // Default: 0.80

	// SemanticCacheThreshold: minimum similarity for cache hit
	SemanticCacheThreshold float64 // Default: 0.92

	// EnableSemanticCache: whether to use semantic cache
	EnableSemanticCache bool // Default: true

	// EnableAutoLabeling: whether to apply auto-labeling rules
	EnableAutoLabeling bool // Default: true
}

// DefaultScorePipelineConfig returns the default configuration
func DefaultScorePipelineConfig() *ScorePipelineConfig {
	return &ScorePipelineConfig{
		EarlyExitThreshold:     0.95,
		LLMFallbackThreshold:   0.80,
		SemanticCacheThreshold: 0.92,
		EnableSemanticCache:    true,
		EnableAutoLabeling:     true,
	}
}

// =============================================================================
// Classification Pipeline Result
// =============================================================================

// ClassificationPipelineResultV2 is the final result from the score pipeline
type ClassificationPipelineResultV2 struct {
	// Final classification
	Category    domain.EmailCategory
	SubCategory *domain.EmailSubCategory
	Priority    domain.Priority
	Labels      []int64

	// Score info
	Score  float64 // Best score from all stages
	Stage  string  // Stage that produced the winning score
	Source string  // Detailed source (e.g., "rfc:list-unsubscribe", "sender:vip")

	// Debug info
	AllResults []*ScoreClassifierResult // Results from all stages
	Signals    []string                 // All detected signals
	LLMUsed    bool

	// Stats
	ProcessingTimeMs int64
}

// =============================================================================
// Signal Constants
// =============================================================================

// RFC Header Signals
const (
	SignalListUnsubscribe  = "list-unsubscribe"
	SignalPrecedenceBulk   = "precedence-bulk"
	SignalAutoSubmitted    = "auto-submitted"
	SignalMailchimp        = "esp-mailchimp"
	SignalSendGrid         = "esp-sendgrid"
	SignalCampaign         = "x-campaign"
	SignalFeedbackID       = "feedback-id"
	SignalListID           = "list-id"
	SignalBulkMail         = "bulk-mail"
	SignalMarketingESP     = "marketing-esp"
	SignalNoReply          = "noreply-sender"
	SignalUnsubscribeInURL = "unsubscribe-url"
	SignalDeveloperService = "developer-service" // GitHub, GitLab, Jira, etc.
)

// Sender Profile Signals
const (
	SignalVIP            = "vip"
	SignalMuted          = "muted"
	SignalContact        = "contact"
	SignalHighReplyRate  = "high-reply-rate"
	SignalHighReadRate   = "high-read-rate"
	SignalLowReadRate    = "low-read-rate"
	SignalHighDeleteRate = "high-delete-rate"
	SignalRecentSender   = "recent-sender"
	SignalFrequentSender = "frequent-sender"
)

// User Rule Signals
const (
	SignalExactSender    = "exact-sender"
	SignalSenderDomain   = "sender-domain"
	SignalSubjectKeyword = "subject-keyword"
	SignalBodyKeyword    = "body-keyword"
	SignalAIPrompt       = "ai-prompt"
)

// Cache Signals
const (
	SignalSemanticCacheHit = "semantic-cache-hit"
)

// LLM Signals
const (
	SignalLLMClassified = "llm-classified"
)
