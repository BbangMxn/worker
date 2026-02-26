package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"time"
)

const numShards = 32

type PromptCache struct {
	shards      [numShards]*cacheShard
	ttl         time.Duration
	maxItems    int
	totalHits   int64
	totalMisses int64
}

type cacheShard struct {
	mu    sync.RWMutex
	items map[string]*cacheItem
}

type cacheItem struct {
	value     string
	expiresAt time.Time
}

func NewPromptCache(ttl time.Duration, maxItems int) *PromptCache {
	c := &PromptCache{
		ttl:      ttl,
		maxItems: maxItems,
	}
	for i := 0; i < numShards; i++ {
		c.shards[i] = &cacheShard{
			items: make(map[string]*cacheItem),
		}
	}
	return c
}

func (c *PromptCache) getShard(key string) *cacheShard {
	h := uint32(2166136261) // FNV-1a offset basis
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619 // FNV prime
	}
	return c.shards[h%numShards]
}

func (c *PromptCache) Get(prompt string) (string, bool) {
	key := hashPrompt(prompt)
	shard := c.getShard(key)

	shard.mu.RLock()
	item, ok := shard.items[key]
	shard.mu.RUnlock()

	if !ok || time.Now().After(item.expiresAt) {
		atomic.AddInt64(&c.totalMisses, 1)
		return "", false
	}

	atomic.AddInt64(&c.totalHits, 1)
	return item.value, true
}

func (c *PromptCache) Set(prompt, response string) {
	key := hashPrompt(prompt)
	shard := c.getShard(key)

	shard.mu.Lock()
	// Simple eviction: clear if over max
	if len(shard.items) >= c.maxItems/numShards {
		now := time.Now()
		for k, v := range shard.items {
			if now.After(v.expiresAt) {
				delete(shard.items, k)
			}
		}
	}

	shard.items[key] = &cacheItem{
		value:     response,
		expiresAt: time.Now().Add(c.ttl),
	}
	shard.mu.Unlock()
}

func (c *PromptCache) Stats() (hits, misses int64, hitRate float64) {
	hits = atomic.LoadInt64(&c.totalHits)
	misses = atomic.LoadInt64(&c.totalMisses)
	total := hits + misses
	if total > 0 {
		hitRate = float64(hits) / float64(total)
	}
	return
}

func hashPrompt(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(h[:16])
}

// TokenOptimizer reduces token usage by compressing prompts
type TokenOptimizer struct {
	maxTokens int
}

func NewTokenOptimizer(maxTokens int) *TokenOptimizer {
	return &TokenOptimizer{maxTokens: maxTokens}
}

func (o *TokenOptimizer) OptimizePrompt(prompt string) string {
	// Simple optimization: truncate if too long
	// Rough estimate: 1 token ≈ 4 characters
	maxChars := o.maxTokens * 4
	if len(prompt) > maxChars {
		return prompt[:maxChars]
	}
	return prompt
}

func (o *TokenOptimizer) EstimateTokens(text string) int {
	// Rough estimate
	return len(text) / 4
}
