// Package common provides L1 (in-memory) cache layer.
package common

import (
	"worker_server/core/domain"
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/goccy/go-json"
)

// =============================================================================
// L1 Cache - In-Memory with O(1) LRU Eviction (Doubly Linked List)
// =============================================================================

// lruNode represents a node in the doubly linked list for O(1) LRU operations
type lruNode struct {
	key  string
	prev *lruNode
	next *lruNode
}

// L1Cache provides fast in-memory caching with TTL and O(1) LRU eviction.
// Uses a doubly linked list + hashmap for constant-time access and eviction.
// This sits in front of Redis (L2) to reduce network calls.
type L1Cache struct {
	data       map[string]*l1Entry
	mu         sync.RWMutex
	maxItems   int
	defaultTTL time.Duration
	config     *L1Config // Full config for adaptive TTL

	// O(1) LRU tracking with doubly linked list
	lruHead *lruNode            // Most recently used (dummy head)
	lruTail *lruNode            // Least recently used (dummy tail)
	nodeMap map[string]*lruNode // key -> node for O(1) lookup

	// Metrics
	hits   int64
	misses int64
}

type l1Entry struct {
	value       []byte
	expiresAt   time.Time
	size        int
	accessCount int64     // Adaptive TTL: access frequency tracking
	createdAt   time.Time // Original creation time
}

// L1Config configures the L1 cache
type L1Config struct {
	MaxItems   int           // Maximum number of items (default 10000)
	DefaultTTL time.Duration // Default TTL (default 2 minutes)

	// Adaptive TTL settings
	EnableAdaptiveTTL bool          // Enable adaptive TTL based on access patterns
	MinTTL            time.Duration // Minimum TTL (default 30 seconds)
	MaxTTL            time.Duration // Maximum TTL (default 10 minutes)
	TTLBoostThreshold int64         // Access count to start boosting TTL (default 3)
	TTLBoostFactor    float64       // TTL multiplier per access tier (default 1.5)
}

// DefaultL1Config returns sensible defaults for L1 cache
func DefaultL1Config() *L1Config {
	return &L1Config{
		MaxItems:          10000,
		DefaultTTL:        2 * time.Minute, // Short TTL for L1
		EnableAdaptiveTTL: true,
		MinTTL:            30 * time.Second,
		MaxTTL:            10 * time.Minute,
		TTLBoostThreshold: 3,
		TTLBoostFactor:    1.5,
	}
}

// NewL1Cache creates a new L1 cache with O(1) LRU eviction
func NewL1Cache(config *L1Config) *L1Cache {
	if config == nil {
		config = DefaultL1Config()
	}

	// Initialize dummy head and tail for doubly linked list
	head := &lruNode{}
	tail := &lruNode{}
	head.next = tail
	tail.prev = head

	cache := &L1Cache{
		data:       make(map[string]*l1Entry),
		maxItems:   config.MaxItems,
		defaultTTL: config.DefaultTTL,
		config:     config,
		lruHead:    head,
		lruTail:    tail,
		nodeMap:    make(map[string]*lruNode),
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves a value from the cache
func (c *L1Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	entry, ok := c.data[key]
	c.mu.RUnlock()

	if !ok {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	// Check expiration
	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.data, key)
		c.removeFromAccessOrder(key)
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	c.mu.Lock()
	c.hits++
	entry.accessCount++
	c.updateAccessOrder(key)

	// Adaptive TTL: extend TTL for frequently accessed items
	if c.config.EnableAdaptiveTTL && entry.accessCount >= c.config.TTLBoostThreshold {
		c.extendTTL(entry)
	}
	c.mu.Unlock()

	return entry.value, true
}

// extendTTL extends the TTL for frequently accessed items
func (c *L1Cache) extendTTL(entry *l1Entry) {
	// Calculate tier based on access count
	// Tier 0: 3-5 accesses, Tier 1: 6-8 accesses, etc.
	tier := (entry.accessCount - c.config.TTLBoostThreshold) / 3

	// Calculate new TTL with exponential boost
	newTTL := c.defaultTTL
	for i := int64(0); i <= tier && i < 5; i++ { // Cap at 5 tiers
		newTTL = time.Duration(float64(newTTL) * c.config.TTLBoostFactor)
	}

	// Clamp to MaxTTL
	if newTTL > c.config.MaxTTL {
		newTTL = c.config.MaxTTL
	}

	// Only extend if new expiration is later
	newExpiry := time.Now().Add(newTTL)
	if newExpiry.After(entry.expiresAt) {
		entry.expiresAt = newExpiry
	}
}

// Set stores a value in the cache with default TTL
func (c *L1Cache) Set(key string, value []byte) {
	c.SetWithTTL(key, value, c.defaultTTL)
}

// SetWithTTL stores a value with a specific TTL
func (c *L1Cache) SetWithTTL(key string, value []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Apply minimum TTL if adaptive TTL is enabled
	if c.config.EnableAdaptiveTTL && ttl < c.config.MinTTL {
		ttl = c.config.MinTTL
	}

	// Evict if at capacity
	if len(c.data) >= c.maxItems {
		c.evictLRU()
	}

	now := time.Now()
	c.data[key] = &l1Entry{
		value:       value,
		expiresAt:   now.Add(ttl),
		size:        len(value),
		accessCount: 1,
		createdAt:   now,
	}

	c.updateAccessOrder(key)
}

// Delete removes a key from the cache
func (c *L1Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
	c.removeFromAccessOrder(key)
}

// DeletePrefix removes all keys with the given prefix
func (c *L1Cache) DeletePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.data {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.data, key)
			c.removeFromAccessOrder(key)
		}
	}
}

// Clear removes all entries
func (c *L1Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]*l1Entry)

	// Reset doubly linked list
	c.lruHead.next = c.lruTail
	c.lruTail.prev = c.lruHead
	c.nodeMap = make(map[string]*lruNode)
}

// Stats returns cache statistics
func (c *L1Cache) Stats() L1Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalSize int
	var hotItems int64 // Items with boosted TTL (access >= threshold)
	var totalAccessCount int64

	for _, entry := range c.data {
		totalSize += entry.size
		totalAccessCount += entry.accessCount
		if entry.accessCount >= c.config.TTLBoostThreshold {
			hotItems++
		}
	}

	hitRate := float64(0)
	total := c.hits + c.misses
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	avgAccessCount := float64(0)
	if len(c.data) > 0 {
		avgAccessCount = float64(totalAccessCount) / float64(len(c.data))
	}

	return L1Stats{
		Items:          len(c.data),
		TotalSize:      totalSize,
		Hits:           c.hits,
		Misses:         c.misses,
		HitRate:        hitRate,
		MaxItems:       c.maxItems,
		DefaultTTL:     c.defaultTTL,
		AdaptiveTTL:    c.config.EnableAdaptiveTTL,
		HotItems:       hotItems,
		AvgAccessCount: avgAccessCount,
	}
}

// L1Stats contains cache statistics
type L1Stats struct {
	Items          int           `json:"items"`
	TotalSize      int           `json:"total_size_bytes"`
	Hits           int64         `json:"hits"`
	Misses         int64         `json:"misses"`
	HitRate        float64       `json:"hit_rate"`
	MaxItems       int           `json:"max_items"`
	DefaultTTL     time.Duration `json:"default_ttl"`
	AdaptiveTTL    bool          `json:"adaptive_ttl_enabled"`
	HotItems       int64         `json:"hot_items"`        // Items with boosted TTL
	AvgAccessCount float64       `json:"avg_access_count"` // Average accesses per item
}

// =============================================================================
// Internal Methods - O(1) LRU with Doubly Linked List
// =============================================================================

// moveToFront moves a node to the front of the list (most recently used)
// O(1) operation - just pointer updates
func (c *L1Cache) moveToFront(node *lruNode) {
	// Remove from current position
	node.prev.next = node.next
	node.next.prev = node.prev

	// Insert after head (most recently used position)
	node.next = c.lruHead.next
	node.prev = c.lruHead
	c.lruHead.next.prev = node
	c.lruHead.next = node
}

// addToFront adds a new node to the front of the list
// O(1) operation
func (c *L1Cache) addToFront(key string) {
	node := &lruNode{key: key}

	// Insert after head
	node.next = c.lruHead.next
	node.prev = c.lruHead
	c.lruHead.next.prev = node
	c.lruHead.next = node

	c.nodeMap[key] = node
}

// removeNode removes a node from the list
// O(1) operation
func (c *L1Cache) removeNode(node *lruNode) {
	node.prev.next = node.next
	node.next.prev = node.prev
}

func (c *L1Cache) updateAccessOrder(key string) {
	if node, ok := c.nodeMap[key]; ok {
		// Key exists - move to front (O(1))
		c.moveToFront(node)
	} else {
		// New key - add to front (O(1))
		c.addToFront(key)
	}
}

func (c *L1Cache) removeFromAccessOrder(key string) {
	if node, ok := c.nodeMap[key]; ok {
		c.removeNode(node)
		delete(c.nodeMap, key)
	}
}

func (c *L1Cache) evictLRU() {
	// Evict 10% of items or at least 1
	evictCount := c.maxItems / 10
	if evictCount < 1 {
		evictCount = 1
	}

	// Remove from tail (least recently used) - O(1) per removal
	for i := 0; i < evictCount && c.lruTail.prev != c.lruHead; i++ {
		// Get LRU node (just before tail)
		lruNode := c.lruTail.prev
		lruKey := lruNode.key

		// Remove from data map
		delete(c.data, lruKey)

		// Remove from linked list
		c.removeNode(lruNode)
		delete(c.nodeMap, lruKey)
	}
}

func (c *L1Cache) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanupExpired()
	}
}

func (c *L1Cache) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.data {
		if now.After(entry.expiresAt) {
			delete(c.data, key)
			c.removeFromAccessOrder(key)
		}
	}
}

// =============================================================================
// L1+L2 Hybrid Cache
// =============================================================================

// HybridCache combines L1 (memory) and L2 (Redis) caching
type HybridCache struct {
	l1     *L1Cache
	l2     *CacheService
	config *HybridCacheConfig
}

// HybridCacheConfig configures the hybrid cache
type HybridCacheConfig struct {
	L1Config   *L1Config
	L1Enabled  bool
	L2Enabled  bool
	WriteToL1  bool    // Write L2 results back to L1
	L1TTLRatio float64 // L1 TTL as ratio of L2 TTL (default 0.2 = 20%)
}

// DefaultHybridCacheConfig returns sensible defaults
func DefaultHybridCacheConfig() *HybridCacheConfig {
	return &HybridCacheConfig{
		L1Config:   DefaultL1Config(),
		L1Enabled:  true,
		L2Enabled:  true,
		WriteToL1:  true,
		L1TTLRatio: 0.2,
	}
}

// NewHybridCache creates a new hybrid cache
func NewHybridCache(l2 *CacheService, config *HybridCacheConfig) *HybridCache {
	if config == nil {
		config = DefaultHybridCacheConfig()
	}

	var l1 *L1Cache
	if config.L1Enabled {
		l1 = NewL1Cache(config.L1Config)
	}

	return &HybridCache{
		l1:     l1,
		l2:     l2,
		config: config,
	}
}

// GetBody gets email body with L1 -> L2 -> Provider fallback
func (c *HybridCache) GetBody(ctx context.Context, emailID int64, connectionID int64) (*domain.EmailBody, error) {
	key := bodyKey(emailID)

	// L1 check
	if c.l1 != nil {
		if data, ok := c.l1.Get(key); ok {
			var body domain.EmailBody
			if err := json.Unmarshal(data, &body); err == nil {
				// Check if body has content (including empty body marker)
				if body.TextBody != "" || body.HTMLBody != "" {
					log.Printf("[HybridCache] GetBody: L1 HIT for email %d", emailID)
					// Clean empty body marker before returning
					if body.TextBody == EmptyBodyMarker {
						body.TextBody = ""
					}
					if body.HTMLBody == EmptyBodyMarker {
						body.HTMLBody = ""
					}
					return &body, nil
				}
				log.Printf("[HybridCache] GetBody: L1 has EMPTY body for email %d, checking L2", emailID)
			}
		}
	}

	// L2 check (Redis -> MongoDB -> Provider)
	if c.l2 != nil {
		body, err := c.l2.GetBody(ctx, emailID, connectionID)
		if err != nil {
			return nil, err
		}

		// Write back to L1 only if body has content (including empty body marker)
		if c.l1 != nil && c.config.WriteToL1 && body != nil && (body.TextBody != "" || body.HTMLBody != "") {
			if data, err := json.Marshal(body); err == nil {
				l1TTL := time.Duration(float64(c.l2.config.BodyTTL) * c.config.L1TTLRatio)
				c.l1.SetWithTTL(key, data, l1TTL)
			}
		}

		return body, nil
	}

	return nil, fmt.Errorf("no cache layer available")
}

// InvalidateBody invalidates body cache in all layers
func (c *HybridCache) InvalidateBody(ctx context.Context, emailID int64) {
	key := bodyKey(emailID)

	if c.l1 != nil {
		c.l1.Delete(key)
	}

	// L2 invalidation would need Redis DEL
	// For now, just let it expire naturally
}

// GetL1Stats returns L1 cache statistics
func (c *HybridCache) GetL1Stats() *L1Stats {
	if c.l1 == nil {
		return nil
	}
	stats := c.l1.Stats()
	return &stats
}

// GetL2Metrics returns L2 cache metrics
func (c *HybridCache) GetL2Metrics() *CacheMetrics {
	if c.l2 == nil {
		return nil
	}
	return c.l2.GetMetrics()
}
