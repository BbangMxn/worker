// Package rag provides RAG (Retrieval Augmented Generation) functionality.
package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// =============================================================================
// Embedding Cache
// =============================================================================

// EmbeddingCache caches embeddings to reduce API calls.
type EmbeddingCache struct {
	cache   map[string]*cachedEmbedding
	mu      sync.RWMutex
	maxSize int
	ttl     time.Duration

	// Metrics
	hits   int64
	misses int64
}

type cachedEmbedding struct {
	embedding []float32
	createdAt time.Time
}

// EmbeddingCacheConfig configures the embedding cache.
type EmbeddingCacheConfig struct {
	MaxSize int           // Maximum number of cached embeddings
	TTL     time.Duration // Time to live for cache entries
}

// DefaultEmbeddingCacheConfig returns sensible defaults.
func DefaultEmbeddingCacheConfig() *EmbeddingCacheConfig {
	return &EmbeddingCacheConfig{
		MaxSize: 10000,
		TTL:     24 * time.Hour, // Embeddings don't change, cache for a day
	}
}

// NewEmbeddingCache creates a new embedding cache.
func NewEmbeddingCache(config *EmbeddingCacheConfig) *EmbeddingCache {
	if config == nil {
		config = DefaultEmbeddingCacheConfig()
	}

	cache := &EmbeddingCache{
		cache:   make(map[string]*cachedEmbedding),
		maxSize: config.MaxSize,
		ttl:     config.TTL,
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves an embedding from cache.
func (c *EmbeddingCache) Get(text string) ([]float32, bool) {
	key := c.hashText(text)

	c.mu.RLock()
	entry, ok := c.cache[key]
	c.mu.RUnlock()

	if !ok {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	// Check TTL
	if time.Since(entry.createdAt) > c.ttl {
		c.mu.Lock()
		delete(c.cache, key)
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	c.mu.Lock()
	c.hits++
	c.mu.Unlock()

	return entry.embedding, true
}

// Set stores an embedding in cache.
func (c *EmbeddingCache) Set(text string, embedding []float32) {
	key := c.hashText(text)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.cache) >= c.maxSize {
		c.evictOldest()
	}

	c.cache[key] = &cachedEmbedding{
		embedding: embedding,
		createdAt: time.Now(),
	}
}

// Stats returns cache statistics.
func (c *EmbeddingCache) Stats() (hits, misses int64, hitRate float64) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	hits = c.hits
	misses = c.misses
	total := hits + misses
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}
	return
}

func (c *EmbeddingCache) hashText(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:16])
}

func (c *EmbeddingCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.cache {
		if oldestKey == "" || entry.createdAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.createdAt
		}
	}

	if oldestKey != "" {
		delete(c.cache, oldestKey)
	}
}

func (c *EmbeddingCache) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

func (c *EmbeddingCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.cache {
		if now.Sub(entry.createdAt) > c.ttl {
			delete(c.cache, key)
		}
	}
}

// =============================================================================
// Optimized Embedder with Caching
// =============================================================================

// OptimizedEmbedder wraps Embedder with caching.
type OptimizedEmbedder struct {
	*Embedder
	cache *EmbeddingCache
}

// NewOptimizedEmbedder creates an optimized embedder with caching.
func NewOptimizedEmbedder(embedder *Embedder, cache *EmbeddingCache) *OptimizedEmbedder {
	if cache == nil {
		cache = NewEmbeddingCache(nil)
	}
	return &OptimizedEmbedder{
		Embedder: embedder,
		cache:    cache,
	}
}

// Embed returns embedding, using cache if available.
func (e *OptimizedEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	// Check cache first
	if embedding, ok := e.cache.Get(text); ok {
		return embedding, nil
	}

	// Generate embedding
	embedding, err := e.Embedder.Embed(ctx, text)
	if err != nil {
		return nil, err
	}

	// Cache result
	e.cache.Set(text, embedding)

	return embedding, nil
}

// EmbedBatch embeds multiple texts, using cache where possible.
func (e *OptimizedEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	uncachedIndices := make([]int, 0)
	uncachedTexts := make([]string, 0)

	// Check cache for each text
	for i, text := range texts {
		if embedding, ok := e.cache.Get(text); ok {
			results[i] = embedding
		} else {
			uncachedIndices = append(uncachedIndices, i)
			uncachedTexts = append(uncachedTexts, text)
		}
	}

	// Batch embed uncached texts
	if len(uncachedTexts) > 0 {
		embeddings, err := e.Embedder.EmbedBatch(ctx, uncachedTexts)
		if err != nil {
			return nil, err
		}

		// Store in cache and results
		for i, embedding := range embeddings {
			originalIdx := uncachedIndices[i]
			results[originalIdx] = embedding
			e.cache.Set(uncachedTexts[i], embedding)
		}
	}

	return results, nil
}

// GetCacheStats returns cache statistics.
func (e *OptimizedEmbedder) GetCacheStats() (hits, misses int64, hitRate float64) {
	return e.cache.Stats()
}

// =============================================================================
// Search Result Cache
// =============================================================================

// SearchCache caches search results.
type SearchCache struct {
	cache map[string]*cachedSearchResult
	mu    sync.RWMutex
	ttl   time.Duration
}

type cachedSearchResult struct {
	results   []*SearchResult
	createdAt time.Time
}

// NewSearchCache creates a new search cache.
func NewSearchCache(ttl time.Duration) *SearchCache {
	if ttl == 0 {
		ttl = 5 * time.Minute // Short TTL for search results
	}
	return &SearchCache{
		cache: make(map[string]*cachedSearchResult),
		ttl:   ttl,
	}
}

// Get retrieves cached search results.
func (c *SearchCache) Get(key string) ([]*SearchResult, bool) {
	c.mu.RLock()
	entry, ok := c.cache[key]
	c.mu.RUnlock()

	if !ok || time.Since(entry.createdAt) > c.ttl {
		return nil, false
	}

	return entry.results, true
}

// Set stores search results in cache.
func (c *SearchCache) Set(key string, results []*SearchResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[key] = &cachedSearchResult{
		results:   results,
		createdAt: time.Now(),
	}
}

// Invalidate removes a key from cache.
func (c *SearchCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, key)
}

// InvalidateUser removes all cache entries for a user.
func (c *SearchCache) InvalidateUser(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple prefix matching (keys should start with userID)
	for key := range c.cache {
		if len(key) >= len(userID) && key[:len(userID)] == userID {
			delete(c.cache, key)
		}
	}
}
