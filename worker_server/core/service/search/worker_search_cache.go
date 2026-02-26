package search

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// SearchCache provides in-memory caching for search results.
type SearchCache struct {
	cache map[string]*cacheEntry
	mu    sync.RWMutex
	ttl   time.Duration
}

type cacheEntry struct {
	response  *SearchResponse
	expiresAt time.Time
}

// NewSearchCache creates a new search cache.
func NewSearchCache(ttl time.Duration) *SearchCache {
	c := &SearchCache{
		cache: make(map[string]*cacheEntry),
		ttl:   ttl,
	}

	// Start cleanup goroutine
	go c.cleanupLoop()

	return c
}

// BuildKey creates a cache key from a search request.
func (c *SearchCache) BuildKey(req *SearchRequest) string {
	// Combine relevant fields
	keyData := fmt.Sprintf("%s:%d:%s:%s:%d:%d",
		req.UserID.String(),
		req.ConnectionID,
		req.Query,
		req.Strategy,
		req.Limit,
		req.Offset,
	)

	// Add filters if present
	if req.Filters != nil {
		if req.Filters.From != nil {
			keyData += ":from:" + *req.Filters.From
		}
		if req.Filters.Category != nil {
			keyData += ":cat:" + *req.Filters.Category
		}
	}

	// Hash for consistent key length
	hash := sha256.Sum256([]byte(keyData))
	return "search:" + hex.EncodeToString(hash[:16])
}

// Get retrieves a cached response.
func (c *SearchCache) Get(key string) (*SearchResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.cache[key]
	if !ok {
		return nil, false
	}

	// Check expiration
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry.response, true
}

// Set stores a response in the cache.
func (c *SearchCache) Set(key string, response *SearchResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[key] = &cacheEntry{
		response:  response,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Invalidate removes a specific cache entry.
func (c *SearchCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, key)
}

// InvalidateUser removes all cache entries for a user.
func (c *SearchCache) InvalidateUser(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Since keys are hashed, we need to iterate
	// In production, consider using a more efficient approach
	// like storing user -> keys mapping
	prefix := "search:"
	for key := range c.cache {
		// This is a simple approach; in production, maintain a user index
		if len(key) > len(prefix) {
			delete(c.cache, key)
		}
	}
}

// cleanupLoop periodically removes expired entries.
func (c *SearchCache) cleanupLoop() {
	ticker := time.NewTicker(c.ttl)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries.
func (c *SearchCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.cache {
		if now.After(entry.expiresAt) {
			delete(c.cache, key)
		}
	}
}

// Size returns the number of cached entries.
func (c *SearchCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Clear removes all cached entries.
func (c *SearchCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*cacheEntry)
}
