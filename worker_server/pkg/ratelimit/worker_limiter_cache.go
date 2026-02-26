package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/goccy/go-json"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// EmailListCache - 이메일 목록 캐시
// 전략: 최신 메일 (offset < 100)만 캐시, 오래된 메일은 캐시 X
// =============================================================================

// CacheConfig holds cache configuration.
type CacheConfig struct {
	// L1 (로컬 메모리)
	L1MaxSize int           // 최대 항목 수 (기본: 1000)
	L1TTL     time.Duration // TTL (기본: 30초)

	// L2 (Redis)
	L2TTL time.Duration // TTL (기본: 1분)

	// 캐시 대상 제한
	MaxCacheableOffset int // 이 offset 이상은 캐시 안 함 (기본: 100)
}

// DefaultCacheConfig returns default cache configuration.
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		L1MaxSize:          1000,
		L1TTL:              30 * time.Second,
		L2TTL:              1 * time.Minute,
		MaxCacheableOffset: 100, // offset 100 이상은 캐시 안 함
	}
}

// EmailListCache provides two-level caching for email lists.
type EmailListCache struct {
	config *CacheConfig
	l1     *L1Cache
	redis  *redis.Client
}

// NewEmailListCache creates a new email list cache.
func NewEmailListCache(redisClient *redis.Client, config *CacheConfig) *EmailListCache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	return &EmailListCache{
		config: config,
		l1:     NewL1Cache(config.L1MaxSize, config.L1TTL),
		redis:  redisClient,
	}
}

// CacheKey generates cache key for email list query.
type CacheKey struct {
	UserID       string
	ConnectionID int64
	Folder       string
	Offset       int
	Limit        int
}

func (k *CacheKey) String() string {
	return fmt.Sprintf("email:list:%s:%d:%s:%d:%d",
		k.UserID, k.ConnectionID, k.Folder, k.Offset, k.Limit)
}

// ShouldCache returns true if this query should be cached.
func (c *EmailListCache) ShouldCache(offset int) bool {
	return offset < c.config.MaxCacheableOffset
}

// Get retrieves cached email list.
func (c *EmailListCache) Get(ctx context.Context, key *CacheKey) ([]byte, bool) {
	// 오래된 메일은 캐시하지 않음
	if !c.ShouldCache(key.Offset) {
		return nil, false
	}

	keyStr := key.String()

	// 1. L1 캐시 확인
	if data, ok := c.l1.Get(keyStr); ok {
		return data, true
	}

	// 2. L2 (Redis) 캐시 확인
	if c.redis != nil {
		data, err := c.redis.Get(ctx, keyStr).Bytes()
		if err == nil {
			// L1에도 저장
			c.l1.Set(keyStr, data)
			return data, true
		}
	}

	return nil, false
}

// Set stores email list in cache.
func (c *EmailListCache) Set(ctx context.Context, key *CacheKey, data []byte) {
	// 오래된 메일은 캐시하지 않음
	if !c.ShouldCache(key.Offset) {
		return
	}

	keyStr := key.String()

	// 1. L1 캐시에 저장
	c.l1.Set(keyStr, data)

	// 2. L2 (Redis)에 저장
	if c.redis != nil {
		c.redis.Set(ctx, keyStr, data, c.config.L2TTL)
	}
}

// Invalidate removes cache entries for a user/connection.
func (c *EmailListCache) Invalidate(ctx context.Context, userID string, connectionID int64) {
	// L1 캐시 무효화 (패턴 매칭)
	c.l1.InvalidateByPrefix(fmt.Sprintf("email:list:%s:%d:", userID, connectionID))

	// L2 캐시 무효화
	if c.redis != nil {
		pattern := fmt.Sprintf("email:list:%s:%d:*", userID, connectionID)
		keys, _ := c.redis.Keys(ctx, pattern).Result()
		if len(keys) > 0 {
			c.redis.Del(ctx, keys...)
		}
	}
}

// InvalidateByUser removes all cache entries for a user (all connections).
func (c *EmailListCache) InvalidateByUser(ctx context.Context, userID string) {
	// L1 캐시 무효화 (유저의 모든 캐시)
	c.l1.InvalidateByPrefix(fmt.Sprintf("email:list:%s:", userID))

	// L2 캐시 무효화
	if c.redis != nil {
		pattern := fmt.Sprintf("email:list:%s:*", userID)
		keys, _ := c.redis.Keys(ctx, pattern).Result()
		if len(keys) > 0 {
			c.redis.Del(ctx, keys...)
		}
	}
}

// =============================================================================
// Cache Patch Methods (Optimistic Update)
// 전체 캐시 삭제 대신 해당 이메일만 패치하여 성능 최적화
// =============================================================================

// CachedEmail represents email structure in cache for patching.
// 이 구조체는 domain.Email과 동일한 JSON 필드를 가짐
type CachedEmail struct {
	ID           int64   `json:"id"`
	ConnectionID int64   `json:"connection_id"`
	ProviderID   string  `json:"provider_id"`
	Subject      string  `json:"subject"`
	FromEmail    string  `json:"from_email"`
	FromName     *string `json:"from_name,omitempty"`
	Snippet      string  `json:"snippet"`
	Folder       string  `json:"folder"`
	FolderID     *int64  `json:"folder_id,omitempty"`
	IsRead       bool    `json:"is_read"`
	IsStarred    bool    `json:"is_starred"`
	HasAttach    bool    `json:"has_attachments"`
	ReceivedAt   string  `json:"received_at"`
	// 나머지 필드는 json.RawMessage로 보존
}

// PatchReadStatus patches is_read field for specific emails in all cached lists.
func (c *EmailListCache) PatchReadStatus(ctx context.Context, userID string, emailIDs []int64, isRead bool) {
	idSet := make(map[int64]bool, len(emailIDs))
	for _, id := range emailIDs {
		idSet[id] = true
	}

	c.patchEmails(ctx, userID, func(email map[string]interface{}) bool {
		if id, ok := email["id"].(float64); ok && idSet[int64(id)] {
			email["is_read"] = isRead
			return true
		}
		return false
	})
}

// PatchStarStatus patches is_starred field for specific emails in all cached lists.
func (c *EmailListCache) PatchStarStatus(ctx context.Context, userID string, emailIDs []int64, isStarred bool) {
	idSet := make(map[int64]bool, len(emailIDs))
	for _, id := range emailIDs {
		idSet[id] = true
	}

	c.patchEmails(ctx, userID, func(email map[string]interface{}) bool {
		if id, ok := email["id"].(float64); ok && idSet[int64(id)] {
			email["is_starred"] = isStarred
			return true
		}
		return false
	})
}

// PatchFolder patches folder field for specific emails in all cached lists.
func (c *EmailListCache) PatchFolder(ctx context.Context, userID string, emailIDs []int64, folder string) {
	idSet := make(map[int64]bool, len(emailIDs))
	for _, id := range emailIDs {
		idSet[id] = true
	}

	c.patchEmails(ctx, userID, func(email map[string]interface{}) bool {
		if id, ok := email["id"].(float64); ok && idSet[int64(id)] {
			email["folder"] = folder
			return true
		}
		return false
	})
}

// RemoveFromCache removes specific emails from all cached lists (for delete/archive/trash).
func (c *EmailListCache) RemoveFromCache(ctx context.Context, userID string, emailIDs []int64) {
	idSet := make(map[int64]bool, len(emailIDs))
	for _, id := range emailIDs {
		idSet[id] = true
	}

	prefix := fmt.Sprintf("email:list:%s:", userID)

	// L1 캐시 패치
	c.l1.mu.Lock()
	for key, entry := range c.l1.items {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			var emails []map[string]interface{}
			if err := json.Unmarshal(entry.data, &emails); err != nil {
				continue
			}

			// 해당 이메일 제거
			filtered := make([]map[string]interface{}, 0, len(emails))
			for _, email := range emails {
				if id, ok := email["id"].(float64); ok && !idSet[int64(id)] {
					filtered = append(filtered, email)
				}
			}

			if len(filtered) != len(emails) {
				if newData, err := json.Marshal(filtered); err == nil {
					entry.data = newData
				}
			}
		}
	}
	c.l1.mu.Unlock()

	// L2 (Redis) 캐시 패치
	if c.redis != nil {
		pattern := fmt.Sprintf("email:list:%s:*", userID)
		keys, _ := c.redis.Keys(ctx, pattern).Result()
		for _, key := range keys {
			data, err := c.redis.Get(ctx, key).Bytes()
			if err != nil {
				continue
			}

			var emails []map[string]interface{}
			if err := json.Unmarshal(data, &emails); err != nil {
				continue
			}

			filtered := make([]map[string]interface{}, 0, len(emails))
			for _, email := range emails {
				if id, ok := email["id"].(float64); ok && !idSet[int64(id)] {
					filtered = append(filtered, email)
				}
			}

			if len(filtered) != len(emails) {
				if newData, err := json.Marshal(filtered); err == nil {
					c.redis.Set(ctx, key, newData, c.config.L2TTL)
				}
			}
		}
	}
}

// patchEmails is a helper that applies a patch function to all cached email lists for a user.
func (c *EmailListCache) patchEmails(ctx context.Context, userID string, patchFn func(email map[string]interface{}) bool) {
	prefix := fmt.Sprintf("email:list:%s:", userID)

	// L1 캐시 패치
	c.l1.mu.Lock()
	for key, entry := range c.l1.items {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			var emails []map[string]interface{}
			if err := json.Unmarshal(entry.data, &emails); err != nil {
				continue
			}

			modified := false
			for _, email := range emails {
				if patchFn(email) {
					modified = true
				}
			}

			if modified {
				if newData, err := json.Marshal(emails); err == nil {
					entry.data = newData
				}
			}
		}
	}
	c.l1.mu.Unlock()

	// L2 (Redis) 캐시 패치
	if c.redis != nil {
		pattern := fmt.Sprintf("email:list:%s:*", userID)
		keys, _ := c.redis.Keys(ctx, pattern).Result()
		for _, key := range keys {
			data, err := c.redis.Get(ctx, key).Bytes()
			if err != nil {
				continue
			}

			var emails []map[string]interface{}
			if err := json.Unmarshal(data, &emails); err != nil {
				continue
			}

			modified := false
			for _, email := range emails {
				if patchFn(email) {
					modified = true
				}
			}

			if modified {
				if newData, err := json.Marshal(emails); err == nil {
					c.redis.Set(ctx, key, newData, c.config.L2TTL)
				}
			}
		}
	}
}

// =============================================================================
// L1Cache - 로컬 메모리 캐시 (LRU + TTL)
// =============================================================================

type cacheEntry struct {
	data      []byte
	expiresAt time.Time
}

// L1Cache is a simple LRU cache with TTL.
type L1Cache struct {
	maxSize int
	ttl     time.Duration
	items   map[string]*cacheEntry
	order   []string // LRU order
	mu      sync.RWMutex
}

// NewL1Cache creates a new L1 cache.
func NewL1Cache(maxSize int, ttl time.Duration) *L1Cache {
	cache := &L1Cache{
		maxSize: maxSize,
		ttl:     ttl,
		items:   make(map[string]*cacheEntry),
		order:   make([]string, 0, maxSize),
	}

	// 주기적 정리
	go cache.cleanupLoop()

	return cache
}

// Get retrieves value from cache.
func (c *L1Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	entry, exists := c.items[key]
	c.mu.RUnlock()

	if !exists {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, false
	}

	return entry.data, true
}

// Set stores value in cache.
func (c *L1Cache) Set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// LRU eviction if at capacity
	if len(c.items) >= c.maxSize {
		if len(c.order) > 0 {
			oldest := c.order[0]
			delete(c.items, oldest)
			c.order = c.order[1:]
		}
	}

	c.items[key] = &cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(c.ttl),
	}
	c.order = append(c.order, key)
}

// InvalidateByPrefix removes all entries with matching prefix.
func (c *L1Cache) InvalidateByPrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.items {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.items, key)
		}
	}
}

func (c *L1Cache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

func (c *L1Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.items {
		if now.After(entry.expiresAt) {
			delete(c.items, key)
		}
	}
}

// =============================================================================
// String key based methods (for handler convenience)
// =============================================================================

// GetByString retrieves cached data using string key.
func (c *EmailListCache) GetByString(ctx context.Context, key string, offset int) ([]byte, bool) {
	// 오래된 메일은 캐시하지 않음
	if !c.ShouldCache(offset) {
		return nil, false
	}

	// 1. L1 캐시 확인
	if data, ok := c.l1.Get(key); ok {
		return data, true
	}

	// 2. L2 (Redis) 캐시 확인
	if c.redis != nil {
		data, err := c.redis.Get(ctx, key).Bytes()
		if err == nil {
			// L1에도 저장
			c.l1.Set(key, data)
			return data, true
		}
	}

	return nil, false
}

// SetByString stores data using string key.
func (c *EmailListCache) SetByString(ctx context.Context, key string, offset int, data []byte) {
	// 오래된 메일은 캐시하지 않음
	if !c.ShouldCache(offset) {
		return
	}

	// 1. L1 캐시에 저장
	c.l1.Set(key, data)

	// 2. L2 (Redis)에 저장
	if c.redis != nil {
		c.redis.Set(ctx, key, data, c.config.L2TTL)
	}
}

// =============================================================================
// Helper functions
// =============================================================================

// CacheableResponse wraps response for caching.
type CacheableResponse struct {
	Emails  json.RawMessage `json:"emails"`
	Total   int             `json:"total"`
	HasMore bool            `json:"has_more"`
}
