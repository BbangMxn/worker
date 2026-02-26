// Package ai provides optimized AI service with batch processing.
package ai

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"worker_server/core/agent/llm"
	"worker_server/core/domain"
	"worker_server/core/port/out"
	"worker_server/core/service/common"
)

// =============================================================================
// Optimized AI Service
// =============================================================================

// OptimizedService provides cost-optimized AI operations
type OptimizedService struct {
	llmClient       *llm.Client
	optimizedClient *llm.OptimizedClient // Optimized client with caching
	domainRepo      domain.EmailRepository
	emailRepo        out.EmailRepository
	cacheService    *common.CacheService
	settingsRepo    domain.SettingsRepository

	// Batch processing
	batchSize     int
	batchTimeout  time.Duration
	classifyQueue chan *classifyRequest
	batchWg       sync.WaitGroup

	// Metrics
	metrics *AIMetrics
}

// AIMetrics tracks AI service metrics
type AIMetrics struct {
	TotalClassified  int64
	TotalSummarized  int64
	TotalReplies     int64
	BatchesProcessed int64
	TokensUsed       int64
	EstimatedCostUSD float64
	mu               sync.Mutex
}

type classifyRequest struct {
	EmailID  int64
	ResultCh chan *domain.ClassificationResult
	ErrorCh  chan error
}

// NewOptimizedService creates a new optimized AI service
func NewOptimizedService(
	llmClient *llm.Client,
	domainRepo domain.EmailRepository,
	emailRepo out.EmailRepository,
	cacheService *common.CacheService,
	settingsRepo domain.SettingsRepository,
) *OptimizedService {
	s := &OptimizedService{
		llmClient:     llmClient,
		domainRepo:    domainRepo,
		emailRepo:      emailRepo,
		cacheService:  cacheService,
		settingsRepo:  settingsRepo,
		batchSize:     10,              // 10개씩 배치 처리
		batchTimeout:  2 * time.Second, // 최대 2초 대기
		classifyQueue: make(chan *classifyRequest, 100),
		metrics:       &AIMetrics{},
	}

	// Start batch processor
	go s.batchClassifyProcessor()

	return s
}

// NewOptimizedServiceFull creates optimized service with all optimizations enabled
func NewOptimizedServiceFull(
	llmClient *llm.Client,
	optimizedClient *llm.OptimizedClient,
	domainRepo domain.EmailRepository,
	emailRepo out.EmailRepository,
	cacheService *common.CacheService,
	settingsRepo domain.SettingsRepository,
) *OptimizedService {
	s := &OptimizedService{
		llmClient:       llmClient,
		optimizedClient: optimizedClient,
		domainRepo:      domainRepo,
		emailRepo:        emailRepo,
		cacheService:    cacheService,
		settingsRepo:    settingsRepo,
		batchSize:       10,
		batchTimeout:    2 * time.Second,
		classifyQueue:   make(chan *classifyRequest, 100),
		metrics:         &AIMetrics{},
	}

	go s.batchClassifyProcessor()
	return s
}

// =============================================================================
// Batch Classification
// =============================================================================

// ClassifyEmail classifies a single email (uses batch internally)
func (s *OptimizedService) ClassifyEmail(ctx context.Context, emailID int64) (*domain.ClassificationResult, error) {
	// Check cache first
	if s.cacheService != nil {
		if cached, _ := s.cacheService.GetAIResult(ctx, emailID); cached != nil {
			category := domain.EmailCategory(cached.Category)
			priority := domain.Priority(cached.Priority)
			return &domain.ClassificationResult{
				EmailID:  emailID,
				Category: &category,
				Priority: &priority,
				Summary:  &cached.Summary,
				Tags:     cached.Tags,
				Score:    cached.Score,
			}, nil
		}
	}

	// Check if already classified in DB
	if s.domainRepo != nil {
		email, err := s.domainRepo.GetByID(emailID)
		if err == nil && email != nil && email.AICategory != nil {
			return &domain.ClassificationResult{
				EmailID:  emailID,
				Category: email.AICategory,
				Priority: email.AIPriority,
				Summary:  email.AISummary,
				Tags:     email.AITags,
			}, nil
		}
	}

	// Queue for batch processing
	req := &classifyRequest{
		EmailID:  emailID,
		ResultCh: make(chan *domain.ClassificationResult, 1),
		ErrorCh:  make(chan error, 1),
	}

	select {
	case s.classifyQueue <- req:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Wait for result
	select {
	case result := <-req.ResultCh:
		return result, nil
	case err := <-req.ErrorCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ClassifyEmailBatch classifies multiple emails directly
func (s *OptimizedService) ClassifyEmailBatch(ctx context.Context, emailIDs []int64) ([]*domain.ClassificationResult, error) {
	if len(emailIDs) == 0 {
		return nil, nil
	}

	// Get email data
	var inputs []llm.BatchClassifyInput
	emailMap := make(map[int64]*domain.Email)

	for _, id := range emailIDs {
		email, err := s.domainRepo.GetByID(id)
		if err != nil {
			log.Printf("[OptimizedService] Failed to get email %d: %v", id, err)
			continue
		}

		// Skip already classified
		if email.AICategory != nil {
			continue
		}

		emailMap[id] = email

		// Get snippet (first 500 chars of body)
		snippet := ""
		if s.domainRepo != nil {
			if body, err := s.domainRepo.GetBody(id); err == nil && body != nil {
				snippet = llm.CleanEmailBody(body.TextBody)
				if len(snippet) > 500 {
					snippet = snippet[:500]
				}
			}
		}

		inputs = append(inputs, llm.BatchClassifyInput{
			ID:      id,
			Subject: email.Subject,
			From:    email.FromEmail,
			Snippet: snippet,
		})
	}

	if len(inputs) == 0 {
		return nil, nil
	}

	// Get user rules (from first email's user)
	var userRules []domain.ClassificationRule
	var userRulesStr []string
	if len(emailMap) > 0 && s.settingsRepo != nil {
		for _, email := range emailMap {
			rules, _ := s.settingsRepo.GetClassificationRules(ctx, email.UserID)
			if rules != nil {
				userRules = rules.ToRuleList()
				// Convert to string format for optimized client
				for _, r := range userRules {
					if r.Description != nil {
						userRulesStr = append(userRulesStr, fmt.Sprintf("%s: %s", r.Name, *r.Description))
					}
				}
			}
			break
		}
	}

	// Use optimized client if available (with caching and rate limiting)
	var batchResults []llm.BatchClassifyResult
	var err error

	if s.optimizedClient != nil {
		batchResults, err = s.optimizedClient.ClassifyBatchOptimized(ctx, inputs, userRulesStr)
	} else {
		batchResults, err = s.llmClient.ClassifyEmailBatch(ctx, inputs, userRules)
	}

	if err != nil {
		return nil, fmt.Errorf("batch classify failed: %w", err)
	}

	// Convert results
	var results []*domain.ClassificationResult
	for _, br := range batchResults {
		category := domain.EmailCategory(br.Category)
		priority := domain.Priority(br.Priority)

		result := &domain.ClassificationResult{
			EmailID:  br.ID,
			Category: &category,
			Priority: &priority,
			Tags:     br.Tags,
		}
		results = append(results, result)

		// Update email in DB
		if email, ok := emailMap[br.ID]; ok {
			email.AICategory = &category
			email.AIPriority = &priority
			email.AITags = br.Tags
			if err := s.domainRepo.Update(email); err != nil {
				log.Printf("[OptimizedService] Failed to update email %d: %v", br.ID, err)
			}
		}

		// Cache result
		if s.cacheService != nil {
			s.cacheService.CacheAIResult(ctx, br.ID, &common.CachedAIResult{
				Category: br.Category,
				Priority: br.Priority,
				Tags:     br.Tags,
				Intent:   br.Intent,
			})
		}
	}

	// Update metrics
	s.metrics.mu.Lock()
	s.metrics.TotalClassified += int64(len(results))
	s.metrics.BatchesProcessed++
	s.metrics.mu.Unlock()

	return results, nil
}

// batchClassifyProcessor processes classification requests in batches
func (s *OptimizedService) batchClassifyProcessor() {
	var batch []*classifyRequest
	timer := time.NewTimer(s.batchTimeout)

	for {
		select {
		case req := <-s.classifyQueue:
			batch = append(batch, req)

			if len(batch) >= s.batchSize {
				s.processBatch(batch)
				batch = nil
				timer.Reset(s.batchTimeout)
			}

		case <-timer.C:
			if len(batch) > 0 {
				s.processBatch(batch)
				batch = nil
			}
			timer.Reset(s.batchTimeout)
		}
	}
}

func (s *OptimizedService) processBatch(batch []*classifyRequest) {
	ctx := context.Background()

	// Collect email IDs
	emailIDs := make([]int64, len(batch))
	for i, req := range batch {
		emailIDs[i] = req.EmailID
	}

	// Process batch
	results, err := s.ClassifyEmailBatch(ctx, emailIDs)
	if err != nil {
		// Send error to all requests
		for _, req := range batch {
			req.ErrorCh <- err
		}
		return
	}

	// Map results by email ID
	resultMap := make(map[int64]*domain.ClassificationResult)
	for _, r := range results {
		resultMap[r.EmailID] = r
	}

	// Send results
	for _, req := range batch {
		if result, ok := resultMap[req.EmailID]; ok {
			req.ResultCh <- result
		} else {
			req.ErrorCh <- fmt.Errorf("no result for email %d", req.EmailID)
		}
	}
}

// =============================================================================
// Batch Summarization
// =============================================================================

// MinSummarizeLength is the minimum content length to trigger summarization.
// Emails shorter than this are too brief to need summarization.
const MinSummarizeLength = 200

// SummarizeEmailBatch summarizes multiple emails
func (s *OptimizedService) SummarizeEmailBatch(ctx context.Context, emailIDs []int64) (map[int64]string, error) {
	if len(emailIDs) == 0 {
		return nil, nil
	}

	var inputs []llm.BatchSummarizeInput
	summaries := make(map[int64]string)

	for _, id := range emailIDs {
		email, err := s.domainRepo.GetByID(id)
		if err != nil {
			continue
		}

		// Skip if already summarized
		if email.AISummary != nil && *email.AISummary != "" {
			continue
		}

		body := ""
		if emailBody, err := s.domainRepo.GetBody(id); err == nil && emailBody != nil {
			body = llm.CleanEmailBody(emailBody.TextBody)
		}

		// Skip short emails - no need to summarize brief content
		contentLength := len(email.Subject) + len(body)
		if contentLength < MinSummarizeLength {
			// For short emails, use the body itself as "summary"
			shortSummary := body
			if shortSummary == "" {
				shortSummary = email.Subject
			}
			summaries[id] = shortSummary

			// Update in DB directly (no API call needed)
			email.AISummary = &shortSummary
			s.domainRepo.Update(email)
			continue
		}

		inputs = append(inputs, llm.BatchSummarizeInput{
			ID:      id,
			Subject: email.Subject,
			Body:    body,
		})
	}

	// If all emails were short, return early with existing summaries
	if len(inputs) == 0 {
		if len(summaries) > 0 {
			return summaries, nil
		}
		return nil, nil
	}

	// Use optimized client if available
	var results []llm.BatchSummarizeResult
	var err error

	if s.optimizedClient != nil {
		results, err = s.optimizedClient.SummarizeBatchOptimized(ctx, inputs)
	} else {
		results, err = s.llmClient.SummarizeEmailBatch(ctx, inputs)
	}

	if err != nil {
		return nil, err
	}

	// Add API results to summaries map
	for _, r := range results {
		summaries[r.ID] = r.Summary

		// Update in DB
		if email, err := s.domainRepo.GetByID(r.ID); err == nil && email != nil {
			email.AISummary = &r.Summary
			s.domainRepo.Update(email)
		}
	}

	s.metrics.mu.Lock()
	s.metrics.TotalSummarized += int64(len(results))
	s.metrics.mu.Unlock()

	return summaries, nil
}

// =============================================================================
// Reply Generation (Uses Standard Model)
// =============================================================================

// GenerateReply generates a reply using the standard model for quality
func (s *OptimizedService) GenerateReply(ctx context.Context, emailID int64, tone string, styleContext string) (string, error) {
	email, err := s.domainRepo.GetByID(emailID)
	if err != nil {
		return "", err
	}

	body := ""
	if emailBody, err := s.domainRepo.GetBody(emailID); err == nil && emailBody != nil {
		body = llm.CleanEmailBody(emailBody.TextBody)
	}

	// Use standard model for quality
	reply, err := s.llmClient.GenerateReplySimple(ctx, email.Subject, body, email.FromEmail, styleContext, tone)
	if err != nil {
		return "", err
	}

	s.metrics.mu.Lock()
	s.metrics.TotalReplies++
	s.metrics.mu.Unlock()

	return reply, nil
}

// =============================================================================
// Skip Logic
// =============================================================================

// ShouldSkipClassification checks if email should skip classification
func (s *OptimizedService) ShouldSkipClassification(ctx context.Context, emailID int64) (bool, error) {
	email, err := s.domainRepo.GetByID(emailID)
	if err != nil {
		return false, err
	}

	// Skip if already classified
	if email.AICategory != nil {
		return true, nil
	}

	return false, nil
}

// =============================================================================
// Metrics
// =============================================================================

// GetMetrics returns AI service metrics
func (s *OptimizedService) GetMetrics() *AIMetrics {
	s.metrics.mu.Lock()
	defer s.metrics.mu.Unlock()

	metrics := &AIMetrics{
		TotalClassified:  s.metrics.TotalClassified,
		TotalSummarized:  s.metrics.TotalSummarized,
		TotalReplies:     s.metrics.TotalReplies,
		BatchesProcessed: s.metrics.BatchesProcessed,
		TokensUsed:       s.metrics.TokensUsed,
		EstimatedCostUSD: s.metrics.EstimatedCostUSD,
	}

	// Include optimized client metrics if available
	if s.optimizedClient != nil {
		clientMetrics := s.optimizedClient.GetMetrics()
		metrics.TokensUsed = clientMetrics.TotalTokensInput + clientMetrics.TotalTokensOutput
		metrics.EstimatedCostUSD = clientMetrics.TotalCostUSD
	}

	return metrics
}

// GetLLMMetrics returns detailed LLM client metrics
func (s *OptimizedService) GetLLMMetrics() *llm.ClientMetrics {
	if s.optimizedClient != nil {
		return s.optimizedClient.GetMetrics()
	}
	return nil
}

// GetCacheHitRate returns the prompt cache hit rate
func (s *OptimizedService) GetCacheHitRate() float64 {
	if s.optimizedClient != nil {
		return s.optimizedClient.GetCacheHitRate()
	}
	return 0
}
