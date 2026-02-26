package search

import (
	"context"
	"sync"
	"time"

	"worker_server/core/agent/rag"
	"worker_server/core/port/out"
	"worker_server/pkg/logger"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

// SearchExecutor executes searches across multiple sources.
type SearchExecutor struct {
	emailRepo    out.EmailRepository
	vectorStore *rag.VectorStore
	embedder    *rag.Embedder
	planner     *StrategyPlanner
}

// NewSearchExecutor creates a new search executor.
func NewSearchExecutor(
	emailRepo out.EmailRepository,
	vectorStore *rag.VectorStore,
	embedder *rag.Embedder,
	planner *StrategyPlanner,
) *SearchExecutor {
	return &SearchExecutor{
		emailRepo:    emailRepo,
		vectorStore: vectorStore,
		embedder:    embedder,
		planner:     planner,
	}
}

// ProviderSearchFunc is a function type for provider-specific search.
type ProviderSearchFunc func(ctx context.Context, token *oauth2.Token, query string, limit int) ([]*SearchResult, error)

// Execute runs the search plan and collects results from all sources.
func (e *SearchExecutor) Execute(
	ctx context.Context,
	plan *SearchPlan,
	req *SearchRequest,
	parsed *ParsedQuery,
	providerSearch ProviderSearchFunc,
	token *oauth2.Token,
) (*SearchResponse, error) {
	startTime := time.Now()

	response := &SearchResponse{
		Results:  make([]*SearchResult, 0),
		Sources:  make([]SearchSource, 0),
		Strategy: req.Strategy,
		Intent:   parsed.Intent,
	}

	// Phase 1: Fast searches (DB + Vector) in parallel
	phase1Results := e.executePhase1(ctx, plan, req, parsed)

	// Collect Phase 1 results
	for _, pr := range phase1Results {
		if pr.Error != nil {
			logger.WithError(pr.Error).WithField("source", pr.Source).Warn("[SearchExecutor] search failed")
			continue
		}
		response.Results = append(response.Results, pr.Results...)
		response.Sources = append(response.Sources, pr.Source)

		switch pr.Source {
		case SourceDB:
			response.DBCount = len(pr.Results)
		case SourceVector:
			response.VectorCount = len(pr.Results)
		}
	}

	// Check if fallback to Provider is needed
	if e.planner.ShouldFallbackToProvider(plan, response.DBCount, response.VectorCount, req.Limit) {
		e.planner.AdjustPlanForFallback(plan, parsed, req.Limit)
	}

	// Phase 2: Provider search if needed
	if plan.UseProvider && providerSearch != nil && token != nil {
		providerResults := e.executeProviderSearch(ctx, plan, providerSearch, token)
		if providerResults.Error != nil {
			logger.WithError(providerResults.Error).Warn("[SearchExecutor] Provider search failed")
		} else {
			response.Results = append(response.Results, providerResults.Results...)
			response.Sources = append(response.Sources, SourceProvider)
			response.ProviderCount = len(providerResults.Results)
		}
	}

	response.TimeTaken = time.Since(startTime).Milliseconds()
	response.Total = len(response.Results)
	response.HasMore = len(response.Results) >= req.Limit

	return response, nil
}

// executePhase1 runs DB and Vector searches in parallel.
func (e *SearchExecutor) executePhase1(
	ctx context.Context,
	plan *SearchPlan,
	req *SearchRequest,
	parsed *ParsedQuery,
) []*PartialResult {
	results := make([]*PartialResult, 0, 2)
	resultChan := make(chan *PartialResult, 2)

	var wg sync.WaitGroup

	// Timeout for Phase 1
	phase1Ctx, cancel := context.WithTimeout(ctx, time.Duration(plan.Phase1TimeoutMs)*time.Millisecond)
	defer cancel()

	// DB Search
	if plan.UseDB {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			dbResults, err := e.searchDB(phase1Ctx, plan.DBQuery, req.UserID, req.ConnectionID, parsed)
			resultChan <- &PartialResult{
				Source:  SourceDB,
				Results: dbResults,
				Error:   err,
				TimeMs:  time.Since(start).Milliseconds(),
			}
		}()
	}

	// Vector Search
	if plan.UseVector && e.vectorStore != nil && e.embedder != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			vectorResults, err := e.searchVector(phase1Ctx, plan.VectorQuery, req.UserID)
			resultChan <- &PartialResult{
				Source:  SourceVector,
				Results: vectorResults,
				Error:   err,
				TimeMs:  time.Since(start).Milliseconds(),
			}
		}()
	}

	// Wait for all searches to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	for pr := range resultChan {
		results = append(results, pr)
	}

	return results
}

// searchDB performs PostgreSQL full-text search.
func (e *SearchExecutor) searchDB(
	ctx context.Context,
	query *DBSearchQuery,
	userID uuid.UUID,
	connectionID int64,
	parsed *ParsedQuery,
) ([]*SearchResult, error) {
	if e.emailRepo == nil {
		return nil, nil
	}

	// Use existing repository search method
	emails, _, err := e.emailRepo.Search(ctx, userID, query.Query, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}

	// Convert to SearchResult
	results := make([]*SearchResult, 0, len(emails))
	for _, email := range emails {
		// BM25 스타일 점수 사용 (0이면 기본값 1.0)
		textScore := email.SearchScore
		if textScore == 0 {
			textScore = 1.0
		}

		results = append(results, &SearchResult{
			EmailID:    email.ID,
			ProviderID: email.ExternalID,
			Subject:    email.Subject,
			Snippet:    email.Snippet,
			From:       email.FromEmail,
			Date:       email.ReceivedAt,
			IsRead:     email.IsRead,
			HasAttach:  email.HasAttachment,
			Folder:     email.Folder,
			Source:     SourceDB,
			TextScore:  textScore,
			Score:      textScore, // 초기 점수로 설정
		})
	}

	return results, nil
}

// searchVector performs pgvector similarity search.
func (e *SearchExecutor) searchVector(
	ctx context.Context,
	query *VectorSearchQuery,
	userID uuid.UUID,
) ([]*SearchResult, error) {
	if e.embedder == nil || e.vectorStore == nil {
		return nil, nil
	}

	if query.Text == "" {
		return nil, nil
	}

	// Generate embedding for query
	embedding, err := e.embedder.Embed(ctx, query.Text)
	if err != nil {
		return nil, err
	}

	// Search in vector store
	vectorResults, err := e.vectorStore.Search(ctx, embedding, &rag.SearchOptions{
		UserID:   userID.String(),
		Limit:    query.Limit,
		MinScore: query.MinScore,
	})
	if err != nil {
		return nil, err
	}

	// Convert to SearchResult
	results := make([]*SearchResult, 0, len(vectorResults))
	for _, vr := range vectorResults {
		subject := ""
		snippet := ""
		if vr.Metadata != nil {
			if s, ok := vr.Metadata["subject"].(string); ok {
				subject = s
			}
			if s, ok := vr.Metadata["snippet"].(string); ok {
				snippet = s
			}
		}

		results = append(results, &SearchResult{
			EmailID:     vr.EmailID,
			Subject:     subject,
			Snippet:     snippet,
			Source:      SourceVector,
			VectorScore: vr.Score,
			Score:       vr.Score,
		})
	}

	return results, nil
}

// executeProviderSearch runs the provider-specific search.
func (e *SearchExecutor) executeProviderSearch(
	ctx context.Context,
	plan *SearchPlan,
	providerSearch ProviderSearchFunc,
	token *oauth2.Token,
) *PartialResult {
	start := time.Now()

	// Use Phase 2 timeout
	providerCtx, cancel := context.WithTimeout(ctx, time.Duration(plan.Phase2TimeoutMs)*time.Millisecond)
	defer cancel()

	results, err := providerSearch(providerCtx, token, plan.ProviderQuery.GmailQuery, plan.ProviderQuery.Limit)

	return &PartialResult{
		Source:  SourceProvider,
		Results: results,
		Error:   err,
		TimeMs:  time.Since(start).Milliseconds(),
	}
}
