// Package classification implements the score-based email classification pipeline.
package classification

import (
	"context"
	"fmt"
	"math"
	"strconv"

	"worker_server/core/domain"
)

// =============================================================================
// Semantic Cache Score Classifier (Stage 3)
// =============================================================================

// SemanticCacheClassifier performs Stage 3 classification using embedding similarity.
// Finds similar emails that have already been classified and reuses their classification.
type SemanticCacheClassifier struct {
	cacheRepo domain.ClassificationCacheRepository
	threshold float64 // Minimum similarity score for cache hit (default: 0.92)
}

// NewSemanticCacheClassifier creates a new semantic cache classifier.
func NewSemanticCacheClassifier(cacheRepo domain.ClassificationCacheRepository, threshold float64) *SemanticCacheClassifier {
	if threshold <= 0 {
		threshold = 0.92 // Default threshold
	}
	return &SemanticCacheClassifier{
		cacheRepo: cacheRepo,
		threshold: threshold,
	}
}

// Name returns the classifier name.
func (c *SemanticCacheClassifier) Name() string {
	return "cache"
}

// Stage returns the pipeline stage number.
func (c *SemanticCacheClassifier) Stage() int {
	return 3
}

// Classify performs semantic cache-based classification.
func (c *SemanticCacheClassifier) Classify(ctx context.Context, input *ScoreClassifierInput) (*ScoreClassifierResult, error) {
	if c.cacheRepo == nil {
		return nil, nil
	}

	// Embedding is required for cache lookup
	if len(input.Embedding) == 0 {
		return nil, nil
	}

	// Search for similar cached classifications
	cached, err := c.cacheRepo.FindSimilar(ctx, input.UserID, input.Embedding, c.threshold, 5)
	if err != nil || len(cached) == 0 {
		return nil, nil
	}

	// Find the best match (highest similarity × usage frequency weight)
	best := c.selectBestMatch(cached)
	if best == nil {
		return nil, nil
	}

	// Update usage count asynchronously
	go func(cacheID int64) {
		_ = c.cacheRepo.IncrementUsageCount(ctx, cacheID)
	}(best.ID)

	// Convert cached classification to result
	result := &ScoreClassifierResult{
		Category: domain.EmailCategory(best.Category),
		Priority: parsePriority(best.Priority),
		Score:    best.Score, // Use the original LLM confidence
		Source:   "cache:semantic",
		Signals:  []string{SignalSemanticCacheHit},
		LLMUsed:  false, // Cache hit, no LLM used
		Labels:   best.Labels,
	}

	if best.SubCategory != nil {
		subCat := domain.EmailSubCategory(*best.SubCategory)
		result.SubCategory = &subCat
	}

	return result, nil
}

// selectBestMatch selects the best cached classification considering similarity and usage.
func (c *SemanticCacheClassifier) selectBestMatch(cached []*domain.ClassificationCache) *domain.ClassificationCache {
	if len(cached) == 0 {
		return nil
	}

	var best *domain.ClassificationCache
	var bestScore float64

	for _, cache := range cached {
		// Weight: similarity × log2(usage_count + 1)
		// This gives preference to frequently used cache entries
		usageWeight := math.Log2(float64(cache.UsageCount + 1))
		weightedScore := cache.Score * usageWeight

		if best == nil || weightedScore > bestScore {
			best = cache
			bestScore = weightedScore
		}
	}

	return best
}

// =============================================================================
// Semantic Cache Manager
// =============================================================================

// SemanticCacheManager manages the classification cache.
type SemanticCacheManager struct {
	repo      domain.ClassificationCacheRepository
	threshold float64
}

// NewSemanticCacheManager creates a new cache manager.
func NewSemanticCacheManager(repo domain.ClassificationCacheRepository, threshold float64) *SemanticCacheManager {
	if threshold <= 0 {
		threshold = 0.92
	}
	return &SemanticCacheManager{
		repo:      repo,
		threshold: threshold,
	}
}

// Store saves a classification result to the cache.
func (m *SemanticCacheManager) Store(ctx context.Context, input *ScoreClassifierInput, result *ScoreClassifierResult) error {
	if m.repo == nil || len(input.Embedding) == 0 {
		return nil
	}

	// Only cache high-confidence LLM results
	if !result.LLMUsed || result.Score < 0.75 {
		return nil
	}

	// Check if similar entry already exists
	existing, err := m.repo.FindSimilar(ctx, input.UserID, input.Embedding, 0.98, 1)
	if err == nil && len(existing) > 0 {
		// Very similar entry exists, skip
		return nil
	}

	// Create cache entry
	cache := &domain.ClassificationCache{
		UserID:    input.UserID,
		Embedding: input.Embedding,
		Category:  string(result.Category),
		Priority:  fmt.Sprintf("%.2f", result.Priority),
		Labels:    result.Labels,
		Score:     result.Score,
	}

	if result.SubCategory != nil {
		subCat := string(*result.SubCategory)
		cache.SubCategory = &subCat
	}

	return m.repo.Create(ctx, cache)
}

// Cleanup removes expired cache entries.
func (m *SemanticCacheManager) Cleanup(ctx context.Context) (int, error) {
	if m.repo == nil {
		return 0, nil
	}
	return m.repo.DeleteExpired(ctx)
}

// =============================================================================
// Embedding Similarity Utilities
// =============================================================================

// CosineSimilarity calculates the cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64

	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// CosineDistance calculates the cosine distance (1 - similarity).
func CosineDistance(a, b []float32) float64 {
	return 1 - CosineSimilarity(a, b)
}

// parsePriority converts a string to domain.Priority.
func parsePriority(s string) domain.Priority {
	// Try to parse as float64 first (new format)
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return domain.Priority(val)
	}
	// Fallback to default
	return domain.PriorityNormal
}
