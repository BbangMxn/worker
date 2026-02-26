// Package out defines outbound ports (driven ports) for the application.
package out

import (
	"context"
	"time"
)

// =============================================================================
// VectorStore (Neo4j / pgvector)
// =============================================================================

// VectorStore defines the outbound port for vector storage and similarity search.
// 구현체: Neo4j (RAG/개인화) 또는 pgvector (PostgreSQL)
type VectorStore interface {
	// Search operations
	Search(ctx context.Context, embedding []float32, topK int) ([]VectorSearchResult, error)
	SearchWithFilter(ctx context.Context, embedding []float32, topK int, opts *VectorSearchOptions) ([]VectorSearchResult, error)
	SearchByRecipient(ctx context.Context, userID, recipientEmail string, topK int) ([]VectorSearchResult, error)

	// CRUD operations
	Store(ctx context.Context, id string, embedding []float32, metadata map[string]interface{}) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*VectorItem, error)

	// Batch operations
	BatchStore(ctx context.Context, items []VectorItem) error
	BatchDelete(ctx context.Context, ids []string) error
}

// VectorSearchResult represents a search result with similarity score.
type VectorSearchResult struct {
	ID             string                 `json:"id"`
	Score          float64                `json:"score"`
	Subject        string                 `json:"subject,omitempty"`
	Snippet        string                 `json:"snippet,omitempty"`
	RecipientEmail string                 `json:"recipient_email,omitempty"`
	SentAt         time.Time              `json:"sent_at,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// VectorSearchOptions represents search filter options.
type VectorSearchOptions struct {
	UserID         string
	MinScore       float64
	SentOnly       bool // outbound만
	RecipientEmail string
	RecipientType  string // superior, peer, junior, external
	Folder         string
	Category       string
	DateFrom       *time.Time
	DateTo         *time.Time
}

// VectorItem represents a vector item for storage.
type VectorItem struct {
	ID        string
	Embedding []float32
	Metadata  map[string]interface{}
}

// =============================================================================
// PersonalizationStore (Neo4j)
// =============================================================================

// PersonalizationStore defines the port for user personalization data.
// Neo4j 그래프 DB에 저장되는 사용자 개인화 정보.
type PersonalizationStore interface {
	// User profile
	GetUserProfile(ctx context.Context, userID string) (*UserProfile, error)
	UpdateUserProfile(ctx context.Context, userID string, profile *UserProfile) error

	// Traits (personality)
	GetUserTraits(ctx context.Context, userID string) ([]*UserTrait, error)
	UpdateUserTrait(ctx context.Context, userID string, trait *UserTrait) error

	// Writing style
	GetWritingStyle(ctx context.Context, userID string) (*WritingStyle, error)
	UpdateWritingStyle(ctx context.Context, userID string, style *WritingStyle) error

	// Tone preferences by context
	GetTonePreference(ctx context.Context, userID, context string) (*TonePreference, error)
	UpdateTonePreference(ctx context.Context, userID string, pref *TonePreference) error

	// Frequently used phrases
	GetFrequentPhrases(ctx context.Context, userID string, limit int) ([]*FrequentPhrase, error)
	AddPhrase(ctx context.Context, userID string, phrase *FrequentPhrase) error
	IncrementPhraseCount(ctx context.Context, userID, phraseText string) error

	// Signatures
	GetSignatures(ctx context.Context, userID string) ([]*Signature, error)
	SetDefaultSignature(ctx context.Context, userID string, signatureID string) error
}

// UserProfile represents user profile for personalization.
type UserProfile struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	JobTitle string `json:"job_title,omitempty"`
	Company  string `json:"company,omitempty"`
	Industry string `json:"industry,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// UserTrait represents a personality trait with score.
type UserTrait struct {
	Name  string  `json:"name"`  // professional, casual, friendly, formal, direct, diplomatic
	Score float64 `json:"score"` // 0.0 ~ 1.0
}

// WritingStyle represents user's writing style.
type WritingStyle struct {
	Embedding         []float32 `json:"embedding,omitempty"` // 768-dim vector
	AvgSentenceLength int       `json:"avg_sentence_length"`
	FormalityScore    float64   `json:"formality_score"` // 0.0 ~ 1.0
	EmojiFrequency    float64   `json:"emoji_frequency"` // 0.0 ~ 1.0
	SampleCount       int       `json:"sample_count"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TonePreference represents preferred tone for a context.
type TonePreference struct {
	Context   string  `json:"context"`   // client, team, boss, external, vendor, friend
	Style     string  `json:"style"`     // formal, casual, friendly, professional
	Formality float64 `json:"formality"` // 0.0 ~ 1.0
}

// FrequentPhrase represents a frequently used phrase.
type FrequentPhrase struct {
	Text     string    `json:"text"`
	Count    int       `json:"count"`
	Category string    `json:"category,omitempty"` // greeting, closing, transition
	LastUsed time.Time `json:"last_used"`
}

// Signature represents an email signature.
type Signature struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	IsDefault bool   `json:"is_default"`
}

// =============================================================================
// Extended User Profile (Graph-based personalization)
// =============================================================================

// ExtendedUserProfile represents comprehensive user profile for AI personalization.
type ExtendedUserProfile struct {
	UserID string `json:"user_id"`

	// Basic info
	Email    string `json:"email"`
	Name     string `json:"name"`
	Nickname string `json:"nickname,omitempty"`

	// Demographics
	AgeRange  string   `json:"age_range,omitempty"` // 20s, 30s, 40s, etc.
	Gender    string   `json:"gender,omitempty"`    // optional
	Location  string   `json:"location,omitempty"`  // city/country
	Timezone  string   `json:"timezone,omitempty"`
	Language  string   `json:"language,omitempty"`  // primary language
	Languages []string `json:"languages,omitempty"` // all languages

	// Professional info
	JobTitle   string   `json:"job_title,omitempty"`
	Department string   `json:"department,omitempty"`
	Company    string   `json:"company,omitempty"`
	Industry   string   `json:"industry,omitempty"`
	Seniority  string   `json:"seniority,omitempty"` // junior, mid, senior, executive
	Skills     []string `json:"skills,omitempty"`

	// Communication preferences
	PreferredTone    string  `json:"preferred_tone,omitempty"`    // formal, casual, friendly
	ResponseSpeed    string  `json:"response_speed,omitempty"`    // quick, detailed, balanced
	PreferredLength  string  `json:"preferred_length,omitempty"`  // short, medium, long
	EmojiUsage       float64 `json:"emoji_usage,omitempty"`       // 0.0 ~ 1.0
	FormalityDefault float64 `json:"formality_default,omitempty"` // 0.0 ~ 1.0

	// Metadata
	ProfileCompleteness float64   `json:"profile_completeness"` // 0.0 ~ 1.0
	LastUpdated         time.Time `json:"last_updated"`
	SourceCount         int       `json:"source_count"` // number of emails analyzed
}

// ContactRelationship represents relationship between user and a contact.
type ContactRelationship struct {
	ContactEmail string `json:"contact_email"`
	ContactName  string `json:"contact_name,omitempty"`

	// Relationship type (can change over time)
	RelationType string `json:"relation_type"` // colleague, client, vendor, friend, family, boss, subordinate

	// Relationship history (tracks changes over time)
	RelationHistory []RelationChange `json:"relation_history,omitempty"`

	// Interaction stats
	EmailsSent     int       `json:"emails_sent"`
	EmailsReceived int       `json:"emails_received"`
	LastContact    time.Time `json:"last_contact"`
	FirstContact   time.Time `json:"first_contact"`

	// Communication style for this contact (evolves over time)
	ToneUsed       string  `json:"tone_used,omitempty"`       // formal, casual, friendly
	FormalityLevel float64 `json:"formality_level,omitempty"` // 0.0 ~ 1.0
	AvgReplyTime   int     `json:"avg_reply_time_hours,omitempty"`

	// Style trend (tracks formality changes)
	FormalityTrend string  `json:"formality_trend,omitempty"`  // increasing, decreasing, stable
	ToneChangeRate float64 `json:"tone_change_rate,omitempty"` // how fast tone is changing

	// Importance (recalculated periodically)
	ImportanceScore float64 `json:"importance_score"` // 0.0 ~ 1.0 based on frequency/recency
	IsFrequent      bool    `json:"is_frequent"`
	IsImportant     bool    `json:"is_important"`

	// Activity status
	IsActive         bool      `json:"is_active"` // had contact in last 90 days
	LastActivityDate time.Time `json:"last_activity_date"`
	InactivityDays   int       `json:"inactivity_days,omitempty"`
}

// RelationChange tracks a change in relationship type or status.
type RelationChange struct {
	FromType   string    `json:"from_type"`
	ToType     string    `json:"to_type"`
	ChangedAt  time.Time `json:"changed_at"`
	Confidence float64   `json:"confidence"`       // 0.0 ~ 1.0
	Reason     string    `json:"reason,omitempty"` // inferred, manual, email_signature
}

// CommunicationPattern represents learned communication pattern.
type CommunicationPattern struct {
	PatternID   string `json:"pattern_id"`
	UserID      string `json:"user_id"`
	PatternType string `json:"pattern_type"` // greeting, closing, transition, response

	// Pattern content
	Text      string   `json:"text"`
	Variants  []string `json:"variants,omitempty"`  // similar phrases
	Context   string   `json:"context,omitempty"`   // when to use: formal, casual, urgent
	Recipient string   `json:"recipient,omitempty"` // specific recipient type

	// Usage stats
	UsageCount int       `json:"usage_count"`
	LastUsed   time.Time `json:"last_used"`
	Confidence float64   `json:"confidence"` // 0.0 ~ 1.0
}

// TopicExpertise represents user's expertise in topics.
type TopicExpertise struct {
	Topic           string    `json:"topic"`
	ExpertiseLevel  float64   `json:"expertise_level"` // 0.0 ~ 1.0
	MentionCount    int       `json:"mention_count"`
	LastMentioned   time.Time `json:"last_mentioned"`
	RelatedKeywords []string  `json:"related_keywords,omitempty"`
}

// ExtendedPersonalizationStore extends PersonalizationStore with graph relationships.
type ExtendedPersonalizationStore interface {
	PersonalizationStore

	// Extended profile
	GetExtendedProfile(ctx context.Context, userID string) (*ExtendedUserProfile, error)
	UpdateExtendedProfile(ctx context.Context, userID string, profile *ExtendedUserProfile) error

	// Contact relationships (Graph edges)
	GetContactRelationships(ctx context.Context, userID string, limit int) ([]*ContactRelationship, error)
	GetContactRelationship(ctx context.Context, userID, contactEmail string) (*ContactRelationship, error)
	UpsertContactRelationship(ctx context.Context, userID string, rel *ContactRelationship) error
	GetFrequentContacts(ctx context.Context, userID string, limit int) ([]*ContactRelationship, error)
	GetImportantContacts(ctx context.Context, userID string, limit int) ([]*ContactRelationship, error)

	// Communication patterns
	GetCommunicationPatterns(ctx context.Context, userID string, patternType string, limit int) ([]*CommunicationPattern, error)
	UpsertCommunicationPattern(ctx context.Context, userID string, pattern *CommunicationPattern) error
	GetPatternsByContext(ctx context.Context, userID, context string, limit int) ([]*CommunicationPattern, error)
	IncrementPatternUsage(ctx context.Context, userID, patternID string) error

	// Topic expertise
	GetTopicExpertise(ctx context.Context, userID string, limit int) ([]*TopicExpertise, error)
	UpsertTopicExpertise(ctx context.Context, userID string, topic *TopicExpertise) error

	// Autocomplete support
	GetAutocompleteContext(ctx context.Context, userID string, recipientEmail string, inputPrefix string) (*AutocompleteContext, error)
}

// AutocompleteContext provides context for AI autocomplete.
type AutocompleteContext struct {
	UserProfile     *ExtendedUserProfile    `json:"user_profile"`
	ContactInfo     *ContactRelationship    `json:"contact_info,omitempty"`
	RelevantPhrases []*FrequentPhrase       `json:"relevant_phrases,omitempty"`
	Patterns        []*CommunicationPattern `json:"patterns,omitempty"`
	WritingStyle    *WritingStyle           `json:"writing_style,omitempty"`
	TonePreference  *TonePreference         `json:"tone_preference,omitempty"`
}

// =============================================================================
// ClassificationPatternStore (RAG for classification)
// =============================================================================

// ClassificationPatternStore defines the port for classification pattern storage.
// 사용자의 이메일 분류 패턴을 학습하여 유사한 이메일에 적용.
type ClassificationPatternStore interface {
	// Store classification pattern
	Store(ctx context.Context, pattern *ClassificationPattern, embedding []float32) error

	// Search similar patterns
	Search(ctx context.Context, userID string, embedding []float32, topK int) ([]*ClassificationPattern, error)

	// Get patterns by category
	GetByCategory(ctx context.Context, userID, category string, limit int) ([]*ClassificationPattern, error)

	// Delete pattern
	Delete(ctx context.Context, userID string, emailID int64) error
}

// ClassificationPattern represents a learned classification pattern.
type ClassificationPattern struct {
	UserID    string    `json:"user_id"`
	EmailID   int64     `json:"email_id"`
	From      string    `json:"from"`
	Subject   string    `json:"subject"`
	Snippet   string    `json:"snippet"`
	Category  string    `json:"category"`
	Priority  float64   `json:"priority"`
	Tags      []string  `json:"tags"`
	Intent    string    `json:"intent"`    // action_required, fyi, etc.
	IsManual  bool      `json:"is_manual"` // 사용자가 수동으로 분류했는지
	CreatedAt time.Time `json:"created_at"`
}
