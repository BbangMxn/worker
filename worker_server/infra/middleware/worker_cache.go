// Package middleware provides HTTP middleware for caching and optimization.
package middleware

import (
	"crypto/md5"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// =============================================================================
// Cache Control Middleware
// =============================================================================

// CacheConfig holds cache middleware configuration.
type CacheConfig struct {
	// 정적 리소스 캐시 (CSS, JS, 이미지)
	StaticMaxAge time.Duration

	// API 응답 캐시
	APIMaxAge time.Duration

	// Private 캐시 (사용자별 데이터)
	PrivateMaxAge time.Duration

	// ETag 사용 여부
	EnableETag bool

	// Vary 헤더
	VaryHeaders []string
}

// DefaultCacheConfig returns default cache configuration.
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		StaticMaxAge:  24 * time.Hour,
		APIMaxAge:     0, // API는 기본적으로 캐시 안 함
		PrivateMaxAge: 30 * time.Second,
		EnableETag:    true,
		VaryHeaders:   []string{"Authorization", "Accept-Encoding"},
	}
}

// CacheControl adds cache control headers based on route type.
func CacheControl(config *CacheConfig) fiber.Handler {
	if config == nil {
		config = DefaultCacheConfig()
	}

	return func(c *fiber.Ctx) error {
		// 요청 처리
		err := c.Next()

		// 응답이 성공이 아니면 캐시하지 않음
		if c.Response().StatusCode() >= 400 {
			c.Set("Cache-Control", "no-store")
			return err
		}

		// POST, PUT, DELETE 등은 캐시하지 않음
		method := c.Method()
		if method != "GET" && method != "HEAD" {
			c.Set("Cache-Control", "no-store")
			return err
		}

		// Vary 헤더 설정
		if len(config.VaryHeaders) > 0 {
			for _, h := range config.VaryHeaders {
				c.Vary(h)
			}
		}

		return err
	}
}
// =============================================================================
// ETag Middleware
// =============================================================================

// ETagConfig holds ETag middleware configuration.
type ETagConfig struct {
	// SkipPaths - 이 경로들은 ETag 처리를 건너뜀
	// 프론트엔드가 자체 캐시(IndexedDB 등)를 사용하는 엔드포인트에 유용
	SkipPaths []string
}

// DefaultETagConfig returns default ETag configuration.
func DefaultETagConfig() *ETagConfig {
	return &ETagConfig{
		SkipPaths: []string{
			"/body", // 이메일 본문 - 프론트엔드 IndexedDB 캐시 사용
		},
	}
}

// ETag generates and validates ETag for responses.
func ETag() fiber.Handler {
	return ETagWithConfig(DefaultETagConfig())
}

// ETagWithConfig generates and validates ETag for responses with custom config.
func ETagWithConfig(config *ETagConfig) fiber.Handler {
	if config == nil {
		config = DefaultETagConfig()
	}

	return func(c *fiber.Ctx) error {
		// POST, PUT, DELETE 등은 ETag 처리 안 함
		method := c.Method()
		if method != "GET" && method != "HEAD" {
			return c.Next()
		}

		// Skip 경로 체크 - /body 경로는 프론트엔드 캐시 사용
		path := c.Path()
		for _, skip := range config.SkipPaths {
			if strings.Contains(path, skip) {
				return c.Next()
			}
		}

		// 응답 처리
		if err := c.Next(); err != nil {
			return err
		}

		// 응답이 성공이 아니면 ETag 생성 안 함
		if c.Response().StatusCode() >= 400 {
			return nil
		}

		// 응답 본문으로 ETag 생성
		body := c.Response().Body()
		if len(body) == 0 {
			return nil
		}

		// MD5 해시로 ETag 생성 (빠름)
		hash := md5.Sum(body)
		etag := fmt.Sprintf(`"%x"`, hash)
		c.Set("ETag", etag)

		// If-None-Match 체크
		clientETag := c.Get("If-None-Match")
		if clientETag == etag {
			c.Status(304)
			c.Response().SetBody(nil)
		}

		return nil
	}
}

// =============================================================================
// API-specific Cache Headers
// =============================================================================

// NoCache sets no-cache headers for dynamic API responses.
func NoCache() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")
		return c.Next()
	}
}

// PrivateCache sets private cache headers for user-specific data.
func PrivateCache(maxAge time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := c.Next(); err != nil {
			return err
		}

		// 성공 응답만 캐시
		if c.Response().StatusCode() < 400 {
			c.Set("Cache-Control", fmt.Sprintf("private, max-age=%d", int(maxAge.Seconds())))
		}

		return nil
	}
}

// PublicCache sets public cache headers for shared data.
func PublicCache(maxAge time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := c.Next(); err != nil {
			return err
		}

		// 성공 응답만 캐시
		if c.Response().StatusCode() < 400 {
			c.Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(maxAge.Seconds())))
		}

		return nil
	}
}

// =============================================================================
// Conditional Request Helper
// =============================================================================

// LastModified sets Last-Modified header and checks If-Modified-Since.
func LastModified(modTime time.Time) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Last-Modified 헤더 설정
		c.Set("Last-Modified", modTime.UTC().Format(time.RFC1123))

		// If-Modified-Since 체크
		ifModifiedSince := c.Get("If-Modified-Since")
		if ifModifiedSince != "" {
			clientTime, err := time.Parse(time.RFC1123, ifModifiedSince)
			if err == nil && !modTime.After(clientTime) {
				return c.SendStatus(304)
			}
		}

		return c.Next()
	}
}

// =============================================================================
// Response Size Limiter
// =============================================================================

// MaxResponseSize limits response body size.
func MaxResponseSize(maxSize int) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := c.Next(); err != nil {
			return err
		}

		body := c.Response().Body()
		if len(body) > maxSize {
			c.Set("X-Truncated", "true")
			c.Set("X-Original-Size", strconv.Itoa(len(body)))
			// 실제로 자르지는 않고 경고만 (필요시 구현)
		}

		return nil
	}
}
