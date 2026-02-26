package llm

import (
	"sync"
	"time"
)

// CostTracker tracks LLM API costs
type CostTracker struct {
	mu           sync.RWMutex
	totalCost    float64
	totalTokens  int64
	requestCount int64
	dailyCost    map[string]float64
	modelUsage   map[string]int64
}

func NewCostTracker() *CostTracker {
	return &CostTracker{
		dailyCost:  make(map[string]float64),
		modelUsage: make(map[string]int64),
	}
}

func (t *CostTracker) Track(model string, inputTokens, outputTokens int) float64 {
	// Use CalculateCost from batch.go
	cost := CalculateCost(model, inputTokens, outputTokens)

	t.mu.Lock()
	t.totalCost += cost
	t.totalTokens += int64(inputTokens + outputTokens)
	t.requestCount++

	today := time.Now().Format("2006-01-02")
	t.dailyCost[today] += cost
	t.modelUsage[model] += int64(inputTokens + outputTokens)
	t.mu.Unlock()

	return cost
}

func (t *CostTracker) GetStats() CostStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return CostStats{
		TotalCost:    t.totalCost,
		TotalTokens:  t.totalTokens,
		RequestCount: t.requestCount,
		AvgCostPerRequest: func() float64 {
			if t.requestCount == 0 {
				return 0
			}
			return t.totalCost / float64(t.requestCount)
		}(),
	}
}

type CostStats struct {
	TotalCost         float64 `json:"total_cost"`
	TotalTokens       int64   `json:"total_tokens"`
	RequestCount      int64   `json:"request_count"`
	AvgCostPerRequest float64 `json:"avg_cost_per_request"`
}

// ModelSelector selects optimal model based on task complexity
type ModelSelector struct {
	defaultModel string
	fastModel    string
}

func NewModelSelector() *ModelSelector {
	return &ModelSelector{
		defaultModel: "gpt-4-turbo-preview",
		fastModel:    "gpt-3.5-turbo",
	}
}

// SelectModel chooses model based on task type and input length
func (s *ModelSelector) SelectModel(taskType string, inputLength int) string {
	// Use fast model for simple tasks
	switch taskType {
	case "classification", "extraction":
		if inputLength < 1000 {
			return s.fastModel
		}
	case "summary":
		if inputLength < 2000 {
			return s.fastModel
		}
	}

	// Use default for complex tasks
	return s.defaultModel
}

// BatchProcessor handles batch LLM requests with rate limiting
type BatchProcessor struct {
	maxConcurrent int
	sem           chan struct{}
	rateLimit     *RateLimiter
}

func NewBatchProcessor(maxConcurrent int, requestsPerMinute int) *BatchProcessor {
	return &BatchProcessor{
		maxConcurrent: maxConcurrent,
		sem:           make(chan struct{}, maxConcurrent),
		rateLimit:     NewRateLimiter(requestsPerMinute),
	}
}

type RateLimiter struct {
	tokens     int64
	maxTokens  int64
	refillRate int64
	lastRefill int64
	mu         sync.Mutex
}

func NewRateLimiter(requestsPerMinute int) *RateLimiter {
	return &RateLimiter{
		tokens:     int64(requestsPerMinute),
		maxTokens:  int64(requestsPerMinute),
		refillRate: int64(requestsPerMinute),
		lastRefill: time.Now().Unix(),
	}
}

func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().Unix()
	elapsed := now - r.lastRefill
	if elapsed > 0 {
		refill := elapsed * r.refillRate / 60
		r.tokens = minInt64(r.maxTokens, r.tokens+refill)
		r.lastRefill = now
	}

	if r.tokens > 0 {
		r.tokens--
		return true
	}
	return false
}

func (r *RateLimiter) Wait() {
	for !r.Allow() {
		time.Sleep(100 * time.Millisecond)
	}
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
