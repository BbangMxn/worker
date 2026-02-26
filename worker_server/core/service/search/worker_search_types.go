// Package search provides unified email search across DB, Vector, and Provider APIs.
package search

import (
	"time"

	"github.com/google/uuid"
)

// SearchStrategy defines the search execution strategy.
type SearchStrategy string

const (
	// StrategyFast uses DB only for fastest response.
	StrategyFast SearchStrategy = "fast"

	// StrategyBalanced uses DB + Vector, with Provider fallback if needed.
	StrategyBalanced SearchStrategy = "balanced"

	// StrategyComplete uses all sources for complete results.
	StrategyComplete SearchStrategy = "complete"

	// StrategySemantic uses Vector search only for semantic queries.
	StrategySemantic SearchStrategy = "semantic"

	// StrategyProvider uses Provider API only.
	StrategyProvider SearchStrategy = "provider"
)

// SearchIntent represents the detected intent of a search query.
type SearchIntent string

const (
	// IntentKeyword for exact keyword/email matching.
	IntentKeyword SearchIntent = "keyword"

	// IntentSemantic for natural language meaning-based search.
	IntentSemantic SearchIntent = "semantic"

	// IntentHybrid for mixed queries requiring both approaches.
	IntentHybrid SearchIntent = "hybrid"

	// IntentStructured for explicit filter queries (from:, has:, etc).
	IntentStructured SearchIntent = "structured"
)

// SearchSource indicates where search results came from.
type SearchSource string

const (
	SourceDB       SearchSource = "db"
	SourceVector   SearchSource = "vector"
	SourceProvider SearchSource = "provider"
)

// SearchRequest represents a unified search request.
type SearchRequest struct {
	UserID       uuid.UUID
	ConnectionID int64
	Query        string
	Strategy     SearchStrategy
	Limit        int
	Offset       int
	Cursor       string

	// Optional filters
	Filters *SearchFilters
}

// SearchFilters contains structured filter options.
type SearchFilters struct {
	From          *string
	To            *string
	Subject       *string
	HasAttachment *bool
	IsRead        *bool
	IsStarred     *bool
	DateFrom      *time.Time
	DateTo        *time.Time
	Folder        *string
	Labels        []string
	Category      *string
	Priority      *float64
}

// ParsedQuery represents a parsed and analyzed search query.
type ParsedQuery struct {
	// Original query
	RawQuery string

	// Extracted components
	Keywords      []string
	From          *string
	To            *string
	Subject       *string
	HasAttachment *bool
	IsRead        *bool
	IsStarred     *bool
	DateFrom      *time.Time
	DateTo        *time.Time
	Folder        *string

	// Analysis results
	Intent     SearchIntent
	Complexity int // 1 (simple) to 5 (complex)

	// For semantic search
	SemanticQuery string // cleaned query for embedding
}

// SearchPlan defines what searches to execute.
type SearchPlan struct {
	UseDB       bool
	UseVector   bool
	UseProvider bool

	// Queries for each source
	DBQuery       *DBSearchQuery
	VectorQuery   *VectorSearchQuery
	ProviderQuery *ProviderSearchQuery

	// Timeouts
	Phase1TimeoutMs int // DB + Vector (fast)
	Phase2TimeoutMs int // Provider (slow)

	// Merge strategy
	MergeStrategy MergeStrategy
}

// DBSearchQuery for PostgreSQL full-text search.
type DBSearchQuery struct {
	Query   string
	Filters *SearchFilters
	Limit   int
	Offset  int
	UseRank bool
}

// VectorSearchQuery for pgvector similarity search.
type VectorSearchQuery struct {
	Text     string // text to embed
	Filters  *SearchFilters
	Limit    int
	MinScore float64
}

// ProviderSearchQuery for Gmail/Outlook API.
type ProviderSearchQuery struct {
	GmailQuery   string // Gmail q parameter
	OutlookQuery string // Outlook $search parameter
	Limit        int
}

// MergeStrategy defines how to combine results from multiple sources.
type MergeStrategy string

const (
	// MergeRRF uses Reciprocal Rank Fusion.
	MergeRRF MergeStrategy = "rrf"

	// MergeScore uses weighted score combination.
	MergeScore MergeStrategy = "score"

	// MergeDedup just deduplicates and preserves order.
	MergeDedup MergeStrategy = "dedup"
)

// SearchResult represents a single search result.
type SearchResult struct {
	EmailID    int64
	ProviderID string
	Subject    string
	Snippet    string
	From       string
	To         []string
	Date       time.Time
	IsRead     bool
	HasAttach  bool
	Folder     string

	// Scoring
	Score       float64
	Source      SearchSource
	VectorScore float64 // if from vector search
	TextScore   float64 // if from full-text search
}

// SearchResponse represents the unified search response.
type SearchResponse struct {
	Results    []*SearchResult
	Total      int
	HasMore    bool
	NextCursor string

	// Metadata
	Sources   []SearchSource
	TimeTaken int64 // milliseconds
	Strategy  SearchStrategy
	Intent    SearchIntent

	// Debug info
	DBCount       int
	VectorCount   int
	ProviderCount int
}

// PartialResult holds results from a single search source.
type PartialResult struct {
	Source  SearchSource
	Results []*SearchResult
	Error   error
	TimeMs  int64
}
