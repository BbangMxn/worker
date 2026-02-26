package middleware

import (
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// RateLimiter provides basic rate limiting
type RateLimiter struct {
	requests map[string]*requestInfo
	mu       sync.RWMutex
	limit    int
	window   time.Duration
}

type requestInfo struct {
	count     int
	expiresAt time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string]*requestInfo),
		limit:    limit,
		window:   window,
	}

	// Cleanup goroutine
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			rl.cleanup()
		}
	}()

	return rl
}

func (rl *RateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, info := range rl.requests {
		if now.After(info.expiresAt) {
			delete(rl.requests, key)
		}
	}
}

func (rl *RateLimiter) Handler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := c.IP()

		rl.mu.Lock()
		info, exists := rl.requests[key]
		now := time.Now()

		if !exists || now.After(info.expiresAt) {
			rl.requests[key] = &requestInfo{
				count:     1,
				expiresAt: now.Add(rl.window),
			}
			rl.mu.Unlock()
			setRateLimitHeaders(c, rl.limit, rl.limit-1, info)
			return c.Next()
		}

		remaining := rl.limit - info.count
		if info.count >= rl.limit {
			rl.mu.Unlock()
			setRateLimitHeaders(c, rl.limit, 0, info)
			return c.Status(429).JSON(fiber.Map{
				"error":       "rate limit exceeded",
				"code":        "RATE_LIMITED",
				"retry_after": int(info.expiresAt.Sub(now).Seconds()),
			})
		}

		info.count++
		rl.mu.Unlock()

		setRateLimitHeaders(c, rl.limit, remaining-1, info)
		return c.Next()
	}
}

func setRateLimitHeaders(c *fiber.Ctx, limit, remaining int, info *requestInfo) {
	c.Set("X-RateLimit-Limit", fmt.Sprintf("%d", limit))
	c.Set("X-RateLimit-Remaining", fmt.Sprintf("%d", remaining))
	if info != nil {
		c.Set("X-RateLimit-Reset", fmt.Sprintf("%d", info.expiresAt.Unix()))
	}
}

// AdvancedRateLimiter provides user-aware rate limiting with different tiers
type AdvancedRateLimiter struct {
	ipLimits   map[string]*requestInfo
	userLimits map[string]*requestInfo
	mu         sync.RWMutex

	// Configuration
	ipLimit   int           // Limit per IP (unauthenticated)
	userLimit int           // Limit per user (authenticated)
	window    time.Duration // Time window

	// Endpoint-specific limits
	endpointLimits map[string]*EndpointLimit
}

// EndpointLimit defines rate limits for specific endpoints
type EndpointLimit struct {
	Limit  int
	Window time.Duration
	// Per-user tracking
	userRequests map[string]*requestInfo
	mu           sync.RWMutex
}

// RateLimitConfig holds configuration for rate limiting
type RateLimitConfig struct {
	IPLimit      int           // Requests per IP (default: 100)
	UserLimit    int           // Requests per user (default: 1000)
	Window       time.Duration // Time window (default: 1 minute)
	BurstAllowed int           // Extra burst capacity (default: 10)
}

// DefaultRateLimitConfig returns default configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		IPLimit:      500,
		UserLimit:    2000,
		Window:       time.Minute,
		BurstAllowed: 50,
	}
}

// NewAdvancedRateLimiter creates a new advanced rate limiter
func NewAdvancedRateLimiter(config RateLimitConfig) *AdvancedRateLimiter {
	rl := &AdvancedRateLimiter{
		ipLimits:       make(map[string]*requestInfo),
		userLimits:     make(map[string]*requestInfo),
		ipLimit:        config.IPLimit,
		userLimit:      config.UserLimit,
		window:         config.Window,
		endpointLimits: make(map[string]*EndpointLimit),
	}

	// Register sensitive endpoint limits
	rl.RegisterEndpoint("/api/v1/oauth", 10, time.Minute)           // OAuth: 10/min
	rl.RegisterEndpoint("/api/v1/ai", 30, time.Minute)              // AI: 30/min
	rl.RegisterEndpoint("/api/v1/email/send", 20, time.Minute)       // Send mail: 20/min
	rl.RegisterEndpoint("/api/v1/email/sync", 5, time.Minute)        // Sync: 5/min
	rl.RegisterEndpoint("/api/v1/reports", 10, time.Minute)         // Reports: 10/min
	rl.RegisterEndpoint("/api/v1/calendar/events", 50, time.Minute) // Calendar: 50/min

	// Cleanup goroutine
	go func() {
		ticker := time.NewTicker(time.Minute)
		for range ticker.C {
			rl.cleanup()
		}
	}()

	return rl
}

// RegisterEndpoint adds a custom rate limit for a specific endpoint pattern
func (rl *AdvancedRateLimiter) RegisterEndpoint(pattern string, limit int, window time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.endpointLimits[pattern] = &EndpointLimit{
		Limit:        limit,
		Window:       window,
		userRequests: make(map[string]*requestInfo),
	}
}

func (rl *AdvancedRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Cleanup IP limits
	for key, info := range rl.ipLimits {
		if now.After(info.expiresAt) {
			delete(rl.ipLimits, key)
		}
	}

	// Cleanup user limits
	for key, info := range rl.userLimits {
		if now.After(info.expiresAt) {
			delete(rl.userLimits, key)
		}
	}

	// Cleanup endpoint limits
	for _, el := range rl.endpointLimits {
		el.mu.Lock()
		for key, info := range el.userRequests {
			if now.After(info.expiresAt) {
				delete(el.userRequests, key)
			}
		}
		el.mu.Unlock()
	}
}

// Handler returns the rate limiting middleware
func (rl *AdvancedRateLimiter) Handler() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Skip rate limiting for CORS preflight requests
		if c.Method() == "OPTIONS" {
			return c.Next()
		}

		path := c.Path()
		now := time.Now()

		// Check endpoint-specific limits first
		for pattern, el := range rl.endpointLimits {
			if matchesPattern(path, pattern) {
				// Use user ID if authenticated, otherwise IP
				key := c.IP()
				if userID, ok := c.Locals("user_id").(uuid.UUID); ok {
					key = userID.String()
				}

				el.mu.Lock()
				info, exists := el.userRequests[key]

				if !exists || now.After(info.expiresAt) {
					el.userRequests[key] = &requestInfo{
						count:     1,
						expiresAt: now.Add(el.Window),
					}
					el.mu.Unlock()
					setRateLimitHeaders(c, el.Limit, el.Limit-1, nil)
					return c.Next()
				}

				if info.count >= el.Limit {
					el.mu.Unlock()
					setRateLimitHeaders(c, el.Limit, 0, info)
					return c.Status(429).JSON(fiber.Map{
						"error":       "rate limit exceeded for this endpoint",
						"code":        "RATE_LIMITED",
						"endpoint":    pattern,
						"retry_after": int(info.expiresAt.Sub(now).Seconds()),
					})
				}

				info.count++
				remaining := el.Limit - info.count
				el.mu.Unlock()
				setRateLimitHeaders(c, el.Limit, remaining, info)
			}
		}

		// Apply general rate limits
		userID, hasUserID := c.Locals("user_id").(uuid.UUID)

		rl.mu.Lock()
		defer rl.mu.Unlock()

		if hasUserID {
			// User-based rate limiting (higher limit)
			key := userID.String()
			info, exists := rl.userLimits[key]

			if !exists || now.After(info.expiresAt) {
				rl.userLimits[key] = &requestInfo{
					count:     1,
					expiresAt: now.Add(rl.window),
				}
				return c.Next()
			}

			if info.count >= rl.userLimit {
				return c.Status(429).JSON(fiber.Map{
					"error":       "rate limit exceeded",
					"code":        "RATE_LIMITED",
					"retry_after": int(info.expiresAt.Sub(now).Seconds()),
				})
			}

			info.count++
		} else {
			// IP-based rate limiting (lower limit)
			key := c.IP()
			info, exists := rl.ipLimits[key]

			if !exists || now.After(info.expiresAt) {
				rl.ipLimits[key] = &requestInfo{
					count:     1,
					expiresAt: now.Add(rl.window),
				}
				return c.Next()
			}

			if info.count >= rl.ipLimit {
				return c.Status(429).JSON(fiber.Map{
					"error":       "rate limit exceeded",
					"code":        "RATE_LIMITED",
					"retry_after": int(info.expiresAt.Sub(now).Seconds()),
				})
			}

			info.count++
		}

		return c.Next()
	}
}

// matchesPattern checks if a path matches a pattern prefix
func matchesPattern(path, pattern string) bool {
	return len(path) >= len(pattern) && path[:len(pattern)] == pattern
}

// SensitiveEndpointLimiter creates strict rate limits for sensitive operations
func SensitiveEndpointLimiter(limit int, window time.Duration) fiber.Handler {
	limiter := NewRateLimiter(limit, window)
	return limiter.Handler()
}
