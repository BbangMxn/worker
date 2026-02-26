// Package ratelimit provides rate limiting and protection for API calls.
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// =============================================================================
// API Protection Layer
// 구조: Semaphore → Cache → DB → Debounce → Rate Limiter → API
// =============================================================================

// Config holds rate limiter configuration.
type Config struct {
	// Semaphore: 동시 요청 제한
	MaxConcurrent int // 최대 동시 요청 수 (기본: 100)

	// Rate Limiter: API 호출 속도 제한
	RequestsPerSecond int // 초당 요청 수 (기본: 10)
	BurstSize         int // 버스트 허용량 (기본: 20)

	// Debounce: 중복 요청 방지
	DebounceDuration time.Duration // 중복 방지 시간 (기본: 1분)

	// Payload: 응답 크기 제한
	MaxPayloadSize int // 최대 응답 개수 (기본: 50)
}

// DefaultConfig returns default configuration.
func DefaultConfig() *Config {
	return &Config{
		MaxConcurrent:     100,
		RequestsPerSecond: 10,
		BurstSize:         20,
		DebounceDuration:  1 * time.Minute,
		MaxPayloadSize:    50,
	}
}

// =============================================================================
// APIProtector - 통합 보호 레이어
// =============================================================================

// APIProtector provides comprehensive API protection.
type APIProtector struct {
	config      *Config
	semaphore   chan struct{}
	rateLimiter *SlidingWindowLimiter
	debouncer   *Debouncer
	redis       *redis.Client
	mu          sync.RWMutex
}

// NewAPIProtector creates a new API protector.
func NewAPIProtector(redisClient *redis.Client, config *Config) *APIProtector {
	if config == nil {
		config = DefaultConfig()
	}

	return &APIProtector{
		config:      config,
		semaphore:   make(chan struct{}, config.MaxConcurrent),
		rateLimiter: NewSlidingWindowLimiter(redisClient, config.RequestsPerSecond, config.BurstSize),
		debouncer:   NewDebouncer(redisClient, config.DebounceDuration),
		redis:       redisClient,
	}
}

// ProtectionResult contains the result of protection check.
type ProtectionResult struct {
	Allowed      bool
	Reason       string
	ShouldWait   bool
	WaitDuration time.Duration
	FromDebounce bool
}

// Acquire tries to acquire permission for API call.
// Returns a release function that must be called after API call completes.
func (p *APIProtector) Acquire(ctx context.Context, key string) (*ProtectionResult, func()) {
	// 1. Semaphore 체크 (동시 요청 제한)
	select {
	case p.semaphore <- struct{}{}:
		// 획득 성공
	default:
		return &ProtectionResult{
			Allowed: false,
			Reason:  "too many concurrent requests",
		}, nil
	}

	releaseFunc := func() {
		<-p.semaphore
	}

	// 2. Debounce 체크 (중복 요청 방지)
	if p.debouncer.IsDuplicate(ctx, key) {
		releaseFunc()
		return &ProtectionResult{
			Allowed:      false,
			Reason:       "duplicate request (debounced)",
			FromDebounce: true,
		}, nil
	}

	// 3. Rate Limiter 체크 (API 호출 속도 제한)
	allowed, waitDuration := p.rateLimiter.Allow(ctx, key)
	if !allowed {
		releaseFunc()
		return &ProtectionResult{
			Allowed:      false,
			Reason:       "rate limit exceeded",
			ShouldWait:   waitDuration > 0,
			WaitDuration: waitDuration,
		}, nil
	}

	// 4. Debounce 마킹 (이 요청 기록)
	p.debouncer.Mark(ctx, key)

	return &ProtectionResult{Allowed: true}, releaseFunc
}

// AcquireWithWait tries to acquire with waiting if rate limited.
func (p *APIProtector) AcquireWithWait(ctx context.Context, key string, maxWait time.Duration) (*ProtectionResult, func()) {
	result, release := p.Acquire(ctx, key)

	// Rate limit으로 거부되고 대기 가능하면 대기
	if !result.Allowed && result.ShouldWait && result.WaitDuration <= maxWait {
		select {
		case <-time.After(result.WaitDuration):
			// 대기 후 재시도
			return p.Acquire(ctx, key)
		case <-ctx.Done():
			return &ProtectionResult{
				Allowed: false,
				Reason:  "context cancelled",
			}, nil
		}
	}

	return result, release
}

// MaxPayloadSize returns the configured max payload size.
func (p *APIProtector) MaxPayloadSize() int {
	return p.config.MaxPayloadSize
}

// =============================================================================
// SlidingWindowLimiter - Redis 기반 Sliding Window Rate Limiter
// =============================================================================

// SlidingWindowLimiter implements sliding window rate limiting using Redis.
type SlidingWindowLimiter struct {
	redis     *redis.Client
	rate      int           // requests per window
	window    time.Duration // window size
	burstSize int           // allowed burst
}

// NewSlidingWindowLimiter creates a new sliding window rate limiter.
func NewSlidingWindowLimiter(redisClient *redis.Client, requestsPerSecond, burstSize int) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		redis:     redisClient,
		rate:      requestsPerSecond,
		window:    time.Second,
		burstSize: burstSize,
	}
}

// Allow checks if request is allowed and returns wait duration if not.
func (l *SlidingWindowLimiter) Allow(ctx context.Context, key string) (bool, time.Duration) {
	if l.redis == nil {
		// Redis 없으면 허용 (fallback)
		return true, 0
	}

	now := time.Now()
	windowStart := now.Add(-l.window)
	redisKey := fmt.Sprintf("ratelimit:%s", key)

	// Lua script for atomic sliding window check
	script := redis.NewScript(`
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window_start = tonumber(ARGV[2])
		local max_requests = tonumber(ARGV[3])
		local window_ms = tonumber(ARGV[4])

		-- Remove old entries
		redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)

		-- Count current requests
		local count = redis.call('ZCARD', key)

		if count < max_requests then
			-- Add new request
			redis.call('ZADD', key, now, now .. '-' .. math.random())
			redis.call('PEXPIRE', key, window_ms * 2)
			return 1
		else
			-- Get oldest entry to calculate wait time
			local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
			if #oldest > 0 then
				return -(oldest[2] + window_ms - now)
			end
			return 0
		end
	`)

	result, err := script.Run(ctx, l.redis, []string{redisKey},
		now.UnixMilli(),
		windowStart.UnixMilli(),
		l.rate+l.burstSize,
		l.window.Milliseconds(),
	).Int64()

	if err != nil {
		// Redis 에러 시 허용 (fallback)
		return true, 0
	}

	if result == 1 {
		return true, 0
	}

	// result is negative wait time in milliseconds
	if result < 0 {
		return false, time.Duration(-result) * time.Millisecond
	}

	return false, l.window
}

// =============================================================================
// Debouncer - 중복 요청 방지
// =============================================================================

// Debouncer prevents duplicate requests within a time window.
type Debouncer struct {
	redis    *redis.Client
	duration time.Duration
	local    map[string]time.Time // fallback for no redis
	mu       sync.RWMutex
}

// NewDebouncer creates a new debouncer.
func NewDebouncer(redisClient *redis.Client, duration time.Duration) *Debouncer {
	return &Debouncer{
		redis:    redisClient,
		duration: duration,
		local:    make(map[string]time.Time),
	}
}

// IsDuplicate checks if this is a duplicate request.
func (d *Debouncer) IsDuplicate(ctx context.Context, key string) bool {
	redisKey := fmt.Sprintf("debounce:%s", key)

	if d.redis != nil {
		exists, err := d.redis.Exists(ctx, redisKey).Result()
		if err == nil {
			return exists > 0
		}
	}

	// Fallback to local map
	d.mu.RLock()
	lastTime, exists := d.local[key]
	d.mu.RUnlock()

	if exists && time.Since(lastTime) < d.duration {
		return true
	}

	return false
}

// Mark marks this request as processed.
func (d *Debouncer) Mark(ctx context.Context, key string) {
	redisKey := fmt.Sprintf("debounce:%s", key)

	if d.redis != nil {
		d.redis.Set(ctx, redisKey, "1", d.duration)
	}

	// Also update local map
	d.mu.Lock()
	d.local[key] = time.Now()
	d.mu.Unlock()

	// Cleanup old entries periodically
	go d.cleanup()
}

func (d *Debouncer) cleanup() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	for k, v := range d.local {
		if now.Sub(v) > d.duration*2 {
			delete(d.local, k)
		}
	}
}

// =============================================================================
// MemoryGuard - 메모리 보호
// =============================================================================

// MemoryGuard provides memory protection utilities.
type MemoryGuard struct {
	MaxPayloadSize int
}

// NewMemoryGuard creates a new memory guard.
func NewMemoryGuard(maxPayloadSize int) *MemoryGuard {
	return &MemoryGuard{MaxPayloadSize: maxPayloadSize}
}

// LimitInt limits integer value to max.
func (g *MemoryGuard) LimitInt(value, max int) int {
	if value > max {
		return max
	}
	return value
}

// LimitPayloadSize limits value to MaxPayloadSize.
func (g *MemoryGuard) LimitPayloadSize(value int) int {
	if value > g.MaxPayloadSize {
		return g.MaxPayloadSize
	}
	return value
}

// LimitSliceLen returns min(len, maxPayloadSize) for slice limiting.
func (g *MemoryGuard) LimitSliceLen(sliceLen int) int {
	if sliceLen > g.MaxPayloadSize {
		return g.MaxPayloadSize
	}
	return sliceLen
}
