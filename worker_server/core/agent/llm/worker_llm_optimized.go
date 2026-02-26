// Package llm provides optimized LLM client with caching and cost tracking.
package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goccy/go-json"

	openai "github.com/sashabaranov/go-openai"
)

// =============================================================================
// Optimized Client with Prompt Caching
// =============================================================================

// OptimizedClient wraps the base client with caching and metrics
type OptimizedClient struct {
	*Client

	// Prompt cache (in-memory, could be Redis)
	promptCache    map[string]*cachedResponse
	promptCacheMu  sync.RWMutex
	promptCacheTTL time.Duration

	// Cost tracking
	metrics *ClientMetrics

	// Rate limiting
	rateLimiter *rateLimiter

	// Configuration
	config *OptimizedConfig
}

// OptimizedConfig holds optimization settings
type OptimizedConfig struct {
	// Caching
	EnablePromptCache bool
	PromptCacheTTL    time.Duration

	// Rate limiting
	MaxRequestsPerMinute int

	// Retry
	MaxRetries     int
	RetryBaseDelay time.Duration

	// Cost control
	MaxCostPerRequest float64 // USD
	MaxDailySpend     float64 // USD
}

// DefaultOptimizedConfig returns sensible defaults
func DefaultOptimizedConfig() *OptimizedConfig {
	return &OptimizedConfig{
		EnablePromptCache:    true,
		PromptCacheTTL:       30 * time.Minute,
		MaxRequestsPerMinute: 60,
		MaxRetries:           3,
		RetryBaseDelay:       time.Second,
		MaxCostPerRequest:    0.10, // $0.10
		MaxDailySpend:        100,  // $100/day
	}
}

// cachedResponse stores a cached LLM response
type cachedResponse struct {
	Response  string
	CreatedAt time.Time
	HitCount  int64
}

// ClientMetrics tracks usage and costs
type ClientMetrics struct {
	TotalRequests     int64
	CacheHits         int64
	CacheMisses       int64
	TotalTokensInput  int64
	TotalTokensOutput int64
	TotalCostUSD      float64
	RequestsByModel   map[string]int64
	ErrorCount        int64
	RetryCount        int64
	mu                sync.Mutex
}

// rateLimiter implements token bucket rate limiting
type rateLimiter struct {
	tokens     int64
	maxTokens  int64
	refillRate int64 // tokens per second
	lastRefill time.Time
	mu         sync.Mutex
}

// NewOptimizedClient creates an optimized LLM client
func NewOptimizedClient(apiKey string, config *OptimizedConfig) *OptimizedClient {
	if config == nil {
		config = DefaultOptimizedConfig()
	}

	return &OptimizedClient{
		Client:         NewClient(apiKey),
		promptCache:    make(map[string]*cachedResponse),
		promptCacheTTL: config.PromptCacheTTL,
		metrics: &ClientMetrics{
			RequestsByModel: make(map[string]int64),
		},
		rateLimiter: &rateLimiter{
			tokens:     int64(config.MaxRequestsPerMinute),
			maxTokens:  int64(config.MaxRequestsPerMinute),
			refillRate: int64(config.MaxRequestsPerMinute) / 60,
			lastRefill: time.Now(),
		},
		config: config,
	}
}

// =============================================================================
// Cached Completion
// =============================================================================

// CompleteWithCache performs completion with caching
func (c *OptimizedClient) CompleteWithCache(ctx context.Context, model ModelType, systemPrompt, userPrompt string) (string, error) {
	atomic.AddInt64(&c.metrics.TotalRequests, 1)

	// Check cache
	if c.config.EnablePromptCache {
		cacheKey := c.generateCacheKey(string(model), systemPrompt, userPrompt)
		if cached := c.getFromCache(cacheKey); cached != "" {
			atomic.AddInt64(&c.metrics.CacheHits, 1)
			return cached, nil
		}
		atomic.AddInt64(&c.metrics.CacheMisses, 1)
	}

	// Rate limit check
	if err := c.rateLimiter.wait(ctx); err != nil {
		return "", fmt.Errorf("rate limit exceeded: %w", err)
	}

	// Make request with retry
	var resp string
	var err error
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			atomic.AddInt64(&c.metrics.RetryCount, 1)
			delay := c.config.RetryBaseDelay * time.Duration(1<<attempt)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err = c.doRequest(ctx, string(model), systemPrompt, userPrompt)
		if err == nil {
			break
		}

		// Check if error is retryable
		if !isRetryableError(err) {
			atomic.AddInt64(&c.metrics.ErrorCount, 1)
			return "", err
		}
	}

	if err != nil {
		atomic.AddInt64(&c.metrics.ErrorCount, 1)
		return "", err
	}

	// Cache result
	if c.config.EnablePromptCache {
		cacheKey := c.generateCacheKey(string(model), systemPrompt, userPrompt)
		c.setCache(cacheKey, resp)
	}

	return resp, nil
}

func (c *OptimizedClient) doRequest(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	messages := []openai.ChatCompletionMessage{}

	if systemPrompt != "" {
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		})
	}

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userPrompt,
	})

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
	})
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from LLM")
	}

	// Track metrics
	c.trackUsage(model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

	return resp.Choices[0].Message.Content, nil
}

// =============================================================================
// Cached JSON Completion
// =============================================================================

// CompleteJSONWithCache performs JSON completion with caching
func (c *OptimizedClient) CompleteJSONWithCache(ctx context.Context, model ModelType, prompt string, result interface{}) error {
	atomic.AddInt64(&c.metrics.TotalRequests, 1)

	// Check cache
	if c.config.EnablePromptCache {
		cacheKey := c.generateCacheKey(string(model), "json", prompt)
		if cached := c.getFromCache(cacheKey); cached != "" {
			atomic.AddInt64(&c.metrics.CacheHits, 1)
			return json.Unmarshal([]byte(cached), result)
		}
		atomic.AddInt64(&c.metrics.CacheMisses, 1)
	}

	// Rate limit
	if err := c.rateLimiter.wait(ctx); err != nil {
		return err
	}

	// Make request
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: string(model),
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
		Temperature: 0.3,
	})
	if err != nil {
		atomic.AddInt64(&c.metrics.ErrorCount, 1)
		return err
	}

	if len(resp.Choices) == 0 {
		return fmt.Errorf("no response")
	}

	content := resp.Choices[0].Message.Content

	// Track metrics
	c.trackUsage(string(model), resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

	// Cache
	if c.config.EnablePromptCache {
		cacheKey := c.generateCacheKey(string(model), "json", prompt)
		c.setCache(cacheKey, content)
	}

	return json.Unmarshal([]byte(content), result)
}

// =============================================================================
// Optimized Batch Classification
// =============================================================================

// ClassifyBatchOptimized classifies emails with optimal batching
func (c *OptimizedClient) ClassifyBatchOptimized(ctx context.Context, emails []BatchClassifyInput, userRules []string) ([]BatchClassifyResult, error) {
	if len(emails) == 0 {
		return nil, nil
	}

	// Split into optimal batch sizes (max 10 per request for token efficiency)
	const maxBatchSize = 10
	var allResults []BatchClassifyResult

	for i := 0; i < len(emails); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(emails) {
			end = len(emails)
		}
		batch := emails[i:end]

		results, err := c.classifyBatch(ctx, batch, userRules)
		if err != nil {
			return nil, fmt.Errorf("batch %d failed: %w", i/maxBatchSize, err)
		}

		allResults = append(allResults, results...)
	}

	return allResults, nil
}

func (c *OptimizedClient) classifyBatch(ctx context.Context, emails []BatchClassifyInput, userRules []string) ([]BatchClassifyResult, error) {
	prompt := c.buildOptimizedClassifyPrompt(emails, userRules)

	var response struct {
		Results []BatchClassifyResult `json:"results"`
	}

	if err := c.CompleteJSONWithCache(ctx, ModelMini, prompt, &response); err != nil {
		return nil, err
	}

	return response.Results, nil
}

func (c *OptimizedClient) buildOptimizedClassifyPrompt(emails []BatchClassifyInput, userRules []string) string {
	// Optimized prompt - shorter, clearer, more structured
	prompt := `Classify emails. Return JSON with "results" array.

Categories: primary, social, promotions, updates, forums
Priority: 0.0-1.0 (0.0=lowest, 1.0=urgent)
Intent: action_required, fyi, urgent, meeting, newsletter, receipt, spam

`

	if len(userRules) > 0 {
		prompt += "Rules:\n"
		for _, rule := range userRules {
			prompt += "- " + rule + "\n"
		}
		prompt += "\n"
	}

	prompt += "Emails:\n"
	for _, e := range emails {
		// Keep snippets very short for token efficiency
		snippet := e.Snippet
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		prompt += fmt.Sprintf("[%d] %s | %s | %s\n", e.ID, e.From, e.Subject, snippet)
	}

	prompt += `
Output: {"results":[{"id":N,"category":"...","priority":N,"intent":"...","tags":["..."]}]}`

	return prompt
}

// =============================================================================
// Optimized Summarization
// =============================================================================

// SummarizeBatchOptimized summarizes multiple emails efficiently
func (c *OptimizedClient) SummarizeBatchOptimized(ctx context.Context, emails []BatchSummarizeInput) ([]BatchSummarizeResult, error) {
	if len(emails) == 0 {
		return nil, nil
	}

	// Smaller batches for summarization (more tokens per item)
	const maxBatchSize = 5
	var allResults []BatchSummarizeResult

	for i := 0; i < len(emails); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(emails) {
			end = len(emails)
		}
		batch := emails[i:end]

		results, err := c.summarizeBatch(ctx, batch)
		if err != nil {
			return nil, err
		}

		allResults = append(allResults, results...)
	}

	return allResults, nil
}

func (c *OptimizedClient) summarizeBatch(ctx context.Context, emails []BatchSummarizeInput) ([]BatchSummarizeResult, error) {
	prompt := `Summarize each email in 2-3 sentences. Return JSON.

`
	for _, e := range emails {
		body := e.Body
		if len(body) > 800 {
			body = body[:800] + "..."
		}
		prompt += fmt.Sprintf("[%d] %s\n%s\n\n", e.ID, e.Subject, body)
	}

	prompt += `Output: {"results":[{"id":N,"summary":"..."}]}`

	var response struct {
		Results []BatchSummarizeResult `json:"results"`
	}

	if err := c.CompleteJSONWithCache(ctx, ModelMini, prompt, &response); err != nil {
		return nil, err
	}

	return response.Results, nil
}

// =============================================================================
// Cache Management
// =============================================================================

func (c *OptimizedClient) generateCacheKey(model, prefix, prompt string) string {
	hash := sha256.Sum256([]byte(model + prefix + prompt))
	return hex.EncodeToString(hash[:16])
}

func (c *OptimizedClient) getFromCache(key string) string {
	c.promptCacheMu.RLock()
	defer c.promptCacheMu.RUnlock()

	cached, ok := c.promptCache[key]
	if !ok {
		return ""
	}

	// Check TTL
	if time.Since(cached.CreatedAt) > c.promptCacheTTL {
		return ""
	}

	atomic.AddInt64(&cached.HitCount, 1)
	return cached.Response
}

func (c *OptimizedClient) setCache(key, response string) {
	c.promptCacheMu.Lock()
	defer c.promptCacheMu.Unlock()

	c.promptCache[key] = &cachedResponse{
		Response:  response,
		CreatedAt: time.Now(),
	}

	// Simple cache eviction - remove expired entries
	if len(c.promptCache) > 1000 {
		c.evictExpired()
	}
}

func (c *OptimizedClient) evictExpired() {
	now := time.Now()
	for key, cached := range c.promptCache {
		if now.Sub(cached.CreatedAt) > c.promptCacheTTL {
			delete(c.promptCache, key)
		}
	}
}

// ClearCache clears the prompt cache
func (c *OptimizedClient) ClearCache() {
	c.promptCacheMu.Lock()
	defer c.promptCacheMu.Unlock()
	c.promptCache = make(map[string]*cachedResponse)
}

// =============================================================================
// Metrics
// =============================================================================

func (c *OptimizedClient) trackUsage(model string, promptTokens, completionTokens int) {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	c.metrics.TotalTokensInput += int64(promptTokens)
	c.metrics.TotalTokensOutput += int64(completionTokens)
	c.metrics.RequestsByModel[model]++

	cost := CalculateCost(model, promptTokens, completionTokens)
	c.metrics.TotalCostUSD += cost
}

// GetMetrics returns current metrics
func (c *OptimizedClient) GetMetrics() *ClientMetrics {
	c.metrics.mu.Lock()
	defer c.metrics.mu.Unlock()

	// Return a copy
	copy := &ClientMetrics{
		TotalRequests:     c.metrics.TotalRequests,
		CacheHits:         c.metrics.CacheHits,
		CacheMisses:       c.metrics.CacheMisses,
		TotalTokensInput:  c.metrics.TotalTokensInput,
		TotalTokensOutput: c.metrics.TotalTokensOutput,
		TotalCostUSD:      c.metrics.TotalCostUSD,
		RequestsByModel:   make(map[string]int64),
		ErrorCount:        c.metrics.ErrorCount,
		RetryCount:        c.metrics.RetryCount,
	}
	for k, v := range c.metrics.RequestsByModel {
		copy.RequestsByModel[k] = v
	}
	return copy
}

// GetCacheHitRate returns the cache hit rate
func (c *OptimizedClient) GetCacheHitRate() float64 {
	hits := atomic.LoadInt64(&c.metrics.CacheHits)
	misses := atomic.LoadInt64(&c.metrics.CacheMisses)
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

// =============================================================================
// Rate Limiting
// =============================================================================

func (r *rateLimiter) wait(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Refill tokens
	now := time.Now()
	elapsed := now.Sub(r.lastRefill)
	tokensToAdd := int64(elapsed.Seconds()) * r.refillRate
	r.tokens = min(r.tokens+tokensToAdd, r.maxTokens)
	r.lastRefill = now

	if r.tokens <= 0 {
		// Wait for next token
		waitTime := time.Second / time.Duration(r.refillRate)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			r.tokens = 1
		}
	}

	r.tokens--
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

func isRetryableError(err error) bool {
	// Check for rate limit or temporary errors
	errStr := err.Error()
	return contains(errStr, "rate limit") ||
		contains(errStr, "timeout") ||
		contains(errStr, "503") ||
		contains(errStr, "502")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
