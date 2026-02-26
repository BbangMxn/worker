package search

import (
	"context"
	"time"

	"worker_server/core/agent/rag"
	"worker_server/core/port/out"
	"worker_server/pkg/logger"

	"golang.org/x/oauth2"
)

// Service provides unified email search functionality.
type Service struct {
	analyzer    *QueryAnalyzer
	transformer *QueryTransformer
	planner     *StrategyPlanner
	executor    *SearchExecutor
	merger      *ResultMerger
	cache       *SearchCache
}

// NewService creates a new search service.
func NewService(
	emailRepo out.EmailRepository,
	vectorStore *rag.VectorStore,
	embedder *rag.Embedder,
) *Service {
	analyzer := NewQueryAnalyzer()
	transformer := NewQueryTransformer()
	planner := NewStrategyPlanner(transformer)
	executor := NewSearchExecutor(emailRepo, vectorStore, embedder, planner)
	merger := NewResultMerger()
	cache := NewSearchCache(5 * time.Minute)

	return &Service{
		analyzer:    analyzer,
		transformer: transformer,
		planner:     planner,
		executor:    executor,
		merger:      merger,
		cache:       cache,
	}
}

// Search performs a unified search across all available sources.
func (s *Service) Search(
	ctx context.Context,
	req *SearchRequest,
	providerSearch ProviderSearchFunc,
	token *oauth2.Token,
) (*SearchResponse, error) {
	startTime := time.Now()

	// Set defaults
	if req.Strategy == "" {
		req.Strategy = StrategyBalanced
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}

	// Check cache first
	cacheKey := s.cache.BuildKey(req)
	if cached, ok := s.cache.Get(cacheKey); ok {
		logger.WithField("cache_key", cacheKey).Debug("[SearchService] Cache hit")
		cached.TimeTaken = time.Since(startTime).Milliseconds()
		return cached, nil
	}

	// 1. Analyze query
	parsed := s.analyzer.Analyze(req.Query)

	logger.WithFields(map[string]any{
		"query":      req.Query,
		"intent":     parsed.Intent,
		"complexity": parsed.Complexity,
		"keywords":   parsed.Keywords,
	}).Debug("[SearchService] Query analyzed")

	// 2. Create search plan
	plan := s.planner.Plan(parsed, req)

	logger.WithFields(map[string]any{
		"use_db":       plan.UseDB,
		"use_vector":   plan.UseVector,
		"use_provider": plan.UseProvider,
	}).Debug("[SearchService] Search plan created")

	// 3. Execute search
	response, err := s.executor.Execute(ctx, plan, req, parsed, providerSearch, token)
	if err != nil {
		return nil, err
	}

	// 4. Merge and rank results
	s.merger.Merge(response, plan.MergeStrategy, req.Limit)

	// 5. Apply recency boost for balanced/complete strategies
	if req.Strategy == StrategyBalanced || req.Strategy == StrategyComplete {
		s.merger.BoostRecentResults(response.Results, 0.2)
	}

	// 6. Cache results
	s.cache.Set(cacheKey, response)

	response.TimeTaken = time.Since(startTime).Milliseconds()

	logger.WithFields(map[string]any{
		"total":          response.Total,
		"db_count":       response.DBCount,
		"vector_count":   response.VectorCount,
		"provider_count": response.ProviderCount,
		"time_ms":        response.TimeTaken,
	}).Info("[SearchService] Search completed")

	return response, nil
}

// SearchFast performs a fast DB-only search.
func (s *Service) SearchFast(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	req.Strategy = StrategyFast
	return s.Search(ctx, req, nil, nil)
}

// SearchSemantic performs a semantic vector-based search.
func (s *Service) SearchSemantic(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	req.Strategy = StrategySemantic
	return s.Search(ctx, req, nil, nil)
}

// SearchComplete performs a complete search across all sources.
func (s *Service) SearchComplete(
	ctx context.Context,
	req *SearchRequest,
	providerSearch ProviderSearchFunc,
	token *oauth2.Token,
) (*SearchResponse, error) {
	req.Strategy = StrategyComplete
	return s.Search(ctx, req, providerSearch, token)
}

// AnalyzeQuery analyzes a query without executing search.
// Useful for UI to show query interpretation.
func (s *Service) AnalyzeQuery(query string) *ParsedQuery {
	return s.analyzer.Analyze(query)
}

// TransformToGmail converts a query to Gmail search format.
func (s *Service) TransformToGmail(query string) string {
	parsed := s.analyzer.Analyze(query)
	return s.transformer.ToGmailQuery(parsed)
}

// TransformToOutlook converts a query to Outlook search format.
func (s *Service) TransformToOutlook(query string) (search, filter string) {
	parsed := s.analyzer.Analyze(query)
	return s.transformer.ToOutlookQuery(parsed)
}

// InvalidateCache removes cached results for a user.
func (s *Service) InvalidateCache(userID string) {
	s.cache.InvalidateUser(userID)
}
