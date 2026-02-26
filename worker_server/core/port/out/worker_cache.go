package out

import (
	"context"
	"time"
)

// Cache defines the outbound port for caching.
type Cache interface {
	// Basic operations
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)

	// Typed operations
	GetString(ctx context.Context, key string) (string, error)
	SetString(ctx context.Context, key string, value string, ttl time.Duration) error
	GetInt(ctx context.Context, key string) (int64, error)
	SetInt(ctx context.Context, key string, value int64, ttl time.Duration) error

	// Counter operations
	Incr(ctx context.Context, key string) (int64, error)
	IncrBy(ctx context.Context, key string, value int64) (int64, error)
	Decr(ctx context.Context, key string) (int64, error)

	// Expiration
	Expire(ctx context.Context, key string, ttl time.Duration) error
	TTL(ctx context.Context, key string) (time.Duration, error)

	// Hash operations
	HGet(ctx context.Context, key, field string) ([]byte, error)
	HSet(ctx context.Context, key, field string, value []byte) error
	HGetAll(ctx context.Context, key string) (map[string][]byte, error)
	HDel(ctx context.Context, key string, fields ...string) error

	// List operations
	LPush(ctx context.Context, key string, values ...[]byte) error
	RPush(ctx context.Context, key string, values ...[]byte) error
	LPop(ctx context.Context, key string) ([]byte, error)
	RPop(ctx context.Context, key string) ([]byte, error)
	LRange(ctx context.Context, key string, start, stop int64) ([][]byte, error)
	LLen(ctx context.Context, key string) (int64, error)

	// Set operations
	SAdd(ctx context.Context, key string, members ...[]byte) error
	SRem(ctx context.Context, key string, members ...[]byte) error
	SMembers(ctx context.Context, key string) ([][]byte, error)
	SIsMember(ctx context.Context, key string, member []byte) (bool, error)

	// Sorted set operations
	ZAdd(ctx context.Context, key string, score float64, member []byte) error
	ZRem(ctx context.Context, key string, members ...[]byte) error
	ZRange(ctx context.Context, key string, start, stop int64) ([][]byte, error)
	ZRangeByScore(ctx context.Context, key string, min, max float64) ([][]byte, error)

	// Pub/Sub
	Publish(ctx context.Context, channel string, message []byte) error
	Subscribe(ctx context.Context, channel string) (<-chan []byte, error)

	// Lock
	Lock(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Unlock(ctx context.Context, key string) error
}

// RateLimiter defines the outbound port for rate limiting.
type RateLimiter interface {
	// Allow checks if a request is allowed.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)

	// AllowN checks if n requests are allowed.
	AllowN(ctx context.Context, key string, limit int, window time.Duration, n int) (bool, error)

	// Remaining returns the remaining requests.
	Remaining(ctx context.Context, key string, limit int, window time.Duration) (int, error)

	// Reset resets the rate limit for a key.
	Reset(ctx context.Context, key string) error
}
