// Package httputil provides optimized HTTP client utilities.
package httputil

import (
	"context"
	"net"
	"net/http"
	"time"
)

// =============================================================================
// Optimized HTTP Client Pool
// =============================================================================

// ClientConfig holds HTTP client configuration.
type ClientConfig struct {
	// Connection settings
	MaxIdleConns        int           // 최대 유휴 연결 수 (기본: 100)
	MaxIdleConnsPerHost int           // 호스트당 최대 유휴 연결 (기본: 20)
	MaxConnsPerHost     int           // 호스트당 최대 연결 (기본: 100)
	IdleConnTimeout     time.Duration // 유휴 연결 타임아웃 (기본: 90초)

	// Timeout settings
	DialTimeout         time.Duration // 연결 타임아웃 (기본: 10초)
	TLSHandshakeTimeout time.Duration // TLS 핸드셰이크 타임아웃 (기본: 10초)
	ResponseTimeout     time.Duration // 응답 타임아웃 (기본: 30초)

	// Keep-alive settings
	DisableKeepAlives bool          // Keep-alive 비활성화
	KeepAliveInterval time.Duration // Keep-alive 간격 (기본: 30초)
}

// DefaultClientConfig returns optimized default configuration.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     100,
		IdleConnTimeout:     90 * time.Second,
		DialTimeout:         10 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		ResponseTimeout:     30 * time.Second,
		DisableKeepAlives:   false,
		KeepAliveInterval:   30 * time.Second,
	}
}

// HighThroughputConfig returns configuration for high-throughput scenarios.
func HighThroughputConfig() *ClientConfig {
	return &ClientConfig{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		MaxConnsPerHost:     200,
		IdleConnTimeout:     120 * time.Second,
		DialTimeout:         5 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
		ResponseTimeout:     30 * time.Second,
		DisableKeepAlives:   false,
		KeepAliveInterval:   15 * time.Second,
	}
}

// NewOptimizedClient creates an optimized HTTP client with connection pooling.
func NewOptimizedClient(cfg *ClientConfig) *http.Client {
	if cfg == nil {
		cfg = DefaultClientConfig()
	}

	dialer := &net.Dialer{
		Timeout:   cfg.DialTimeout,
		KeepAlive: cfg.KeepAliveInterval,
	}

	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxConnsPerHost,
		IdleConnTimeout:       cfg.IdleConnTimeout,
		TLSHandshakeTimeout:   cfg.TLSHandshakeTimeout,
		DisableKeepAlives:     cfg.DisableKeepAlives,
		ForceAttemptHTTP2:     true, // HTTP/2 우선 시도
		DisableCompression:    false,
		ResponseHeaderTimeout: cfg.ResponseTimeout,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   cfg.ResponseTimeout,
	}
}

// =============================================================================
// API-Specific Client Configurations
// =============================================================================

// GmailClientConfig returns optimized configuration for Gmail API.
// Gmail allows high concurrency but needs longer timeouts for batch operations.
func GmailClientConfig() *ClientConfig {
	return &ClientConfig{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 50, // High concurrency for batch fetches
		MaxConnsPerHost:     100,
		IdleConnTimeout:     120 * time.Second,
		DialTimeout:         10 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		ResponseTimeout:     60 * time.Second, // Longer for batch operations
		DisableKeepAlives:   false,
		KeepAliveInterval:   30 * time.Second,
	}
}

// OutlookClientConfig returns optimized configuration for Microsoft Graph API.
// Outlook/Graph has stricter rate limits, so we use fewer connections.
func OutlookClientConfig() *ClientConfig {
	return &ClientConfig{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 20, // More conservative for rate limits
		MaxConnsPerHost:     50,
		IdleConnTimeout:     90 * time.Second,
		DialTimeout:         10 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		ResponseTimeout:     45 * time.Second,
		DisableKeepAlives:   false,
		KeepAliveInterval:   30 * time.Second,
	}
}

// OpenAIClientConfig returns optimized configuration for OpenAI API.
// OpenAI needs longer timeouts for LLM responses but moderate concurrency.
func OpenAIClientConfig() *ClientConfig {
	return &ClientConfig{
		MaxIdleConns:        30,
		MaxIdleConnsPerHost: 20,
		MaxConnsPerHost:     30,
		IdleConnTimeout:     120 * time.Second,
		DialTimeout:         10 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		ResponseTimeout:     120 * time.Second, // Long timeout for LLM completions
		DisableKeepAlives:   false,
		KeepAliveInterval:   30 * time.Second,
	}
}

// MongoDBClientConfig returns optimized configuration for MongoDB connections.
func MongoDBClientConfig() *ClientConfig {
	return &ClientConfig{
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     20,
		IdleConnTimeout:     60 * time.Second,
		DialTimeout:         5 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
		ResponseTimeout:     30 * time.Second,
		DisableKeepAlives:   false,
		KeepAliveInterval:   30 * time.Second,
	}
}

// =============================================================================
// Global Shared Client Pool (Singleton)
// =============================================================================

var (
	defaultClient        *http.Client
	highThroughputClient *http.Client
	gmailClient          *http.Client
	outlookClient        *http.Client
	openaiClient         *http.Client
)

func init() {
	defaultClient = NewOptimizedClient(DefaultClientConfig())
	highThroughputClient = NewOptimizedClient(HighThroughputConfig())
	gmailClient = NewOptimizedClient(GmailClientConfig())
	outlookClient = NewOptimizedClient(OutlookClientConfig())
	openaiClient = NewOptimizedClient(OpenAIClientConfig())
}

// DefaultClient returns the shared default HTTP client.
func DefaultClient() *http.Client {
	return defaultClient
}

// HighThroughputClient returns the shared high-throughput HTTP client.
func HighThroughputClient() *http.Client {
	return highThroughputClient
}

// GmailClient returns the optimized HTTP client for Gmail API.
func GmailClient() *http.Client {
	return gmailClient
}

// OutlookClient returns the optimized HTTP client for Microsoft Graph API.
func OutlookClient() *http.Client {
	return outlookClient
}

// OpenAIClient returns the optimized HTTP client for OpenAI API.
func OpenAIClient() *http.Client {
	return openaiClient
}

// =============================================================================
// Request Helper with Context
// =============================================================================

// DoWithContext executes HTTP request with context and timeout.
func DoWithContext(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = defaultClient
	}
	return client.Do(req.WithContext(ctx))
}

// =============================================================================
// Client Pool Statistics
// =============================================================================

// ClientPoolStats holds HTTP client pool statistics.
type ClientPoolStats struct {
	Name                string `json:"name"`
	MaxIdleConns        int    `json:"max_idle_conns"`
	MaxIdleConnsPerHost int    `json:"max_idle_conns_per_host"`
	MaxConnsPerHost     int    `json:"max_conns_per_host"`
	TimeoutSeconds      int    `json:"timeout_seconds"`
}

// GetAllPoolStats returns statistics for all HTTP client pools.
func GetAllPoolStats() []ClientPoolStats {
	return []ClientPoolStats{
		getPoolStats("default", defaultClient, DefaultClientConfig()),
		getPoolStats("high_throughput", highThroughputClient, HighThroughputConfig()),
		getPoolStats("gmail", gmailClient, GmailClientConfig()),
		getPoolStats("outlook", outlookClient, OutlookClientConfig()),
		getPoolStats("openai", openaiClient, OpenAIClientConfig()),
	}
}

func getPoolStats(name string, _ *http.Client, cfg *ClientConfig) ClientPoolStats {
	return ClientPoolStats{
		Name:                name,
		MaxIdleConns:        cfg.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:     cfg.MaxConnsPerHost,
		TimeoutSeconds:      int(cfg.ResponseTimeout.Seconds()),
	}
}
