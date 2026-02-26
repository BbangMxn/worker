// Package metrics provides latency tracking with percentile calculations.
package metrics

import (
	"sort"
	"sync"
	"time"
)

// =============================================================================
// Latency Tracker with P50/P95/P99 Percentiles
// =============================================================================

// LatencyTracker tracks request latencies and calculates percentiles.
// Uses a sliding window to track recent latencies efficiently.
type LatencyTracker struct {
	mu         sync.RWMutex
	samples    []int64 // Latency samples in microseconds
	maxSamples int     // Maximum samples to keep (sliding window)
	sorted     bool    // Whether samples are currently sorted
}

// NewLatencyTracker creates a new latency tracker.
// windowSize determines how many samples to keep for percentile calculation.
func NewLatencyTracker(windowSize int) *LatencyTracker {
	if windowSize <= 0 {
		windowSize = 1000 // Default: keep last 1000 samples
	}
	return &LatencyTracker{
		samples:    make([]int64, 0, windowSize),
		maxSamples: windowSize,
	}
}

// Record records a latency measurement.
func (lt *LatencyTracker) Record(d time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	// Convert to microseconds for precision
	micros := d.Microseconds()

	// Sliding window: remove oldest if at capacity
	if len(lt.samples) >= lt.maxSamples {
		// Remove first 10% to avoid frequent shifts
		removeCount := lt.maxSamples / 10
		if removeCount < 1 {
			removeCount = 1
		}
		lt.samples = lt.samples[removeCount:]
	}

	lt.samples = append(lt.samples, micros)
	lt.sorted = false
}

// Stats returns latency statistics including percentiles.
func (lt *LatencyTracker) Stats() LatencyStats {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if len(lt.samples) == 0 {
		return LatencyStats{}
	}

	// Sort samples if needed for percentile calculation
	if !lt.sorted {
		sort.Slice(lt.samples, func(i, j int) bool {
			return lt.samples[i] < lt.samples[j]
		})
		lt.sorted = true
	}

	n := len(lt.samples)

	// Calculate statistics
	var sum int64
	for _, v := range lt.samples {
		sum += v
	}

	return LatencyStats{
		Count:   int64(n),
		Min:     time.Duration(lt.samples[0]) * time.Microsecond,
		Max:     time.Duration(lt.samples[n-1]) * time.Microsecond,
		Avg:     time.Duration(sum/int64(n)) * time.Microsecond,
		P50:     time.Duration(lt.percentile(0.50)) * time.Microsecond,
		P90:     time.Duration(lt.percentile(0.90)) * time.Microsecond,
		P95:     time.Duration(lt.percentile(0.95)) * time.Microsecond,
		P99:     time.Duration(lt.percentile(0.99)) * time.Microsecond,
		Samples: n,
	}
}

// percentile calculates the percentile value (must be called with lock held and sorted data)
func (lt *LatencyTracker) percentile(p float64) int64 {
	if len(lt.samples) == 0 {
		return 0
	}

	idx := int(float64(len(lt.samples)-1) * p)
	return lt.samples[idx]
}

// Reset clears all samples.
func (lt *LatencyTracker) Reset() {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	lt.samples = lt.samples[:0]
	lt.sorted = false
}

// LatencyStats holds latency statistics.
type LatencyStats struct {
	Count   int64         `json:"count"`
	Min     time.Duration `json:"min"`
	Max     time.Duration `json:"max"`
	Avg     time.Duration `json:"avg"`
	P50     time.Duration `json:"p50"`
	P90     time.Duration `json:"p90"`
	P95     time.Duration `json:"p95"`
	P99     time.Duration `json:"p99"`
	Samples int           `json:"samples"`
}

// MarshalJSON provides custom JSON marshaling for LatencyStats.
func (s LatencyStats) ToMap() map[string]any {
	return map[string]any{
		"count":       s.Count,
		"min_ms":      float64(s.Min.Microseconds()) / 1000,
		"max_ms":      float64(s.Max.Microseconds()) / 1000,
		"avg_ms":      float64(s.Avg.Microseconds()) / 1000,
		"p50_ms":      float64(s.P50.Microseconds()) / 1000,
		"p90_ms":      float64(s.P90.Microseconds()) / 1000,
		"p95_ms":      float64(s.P95.Microseconds()) / 1000,
		"p99_ms":      float64(s.P99.Microseconds()) / 1000,
		"sample_size": s.Samples,
	}
}

// =============================================================================
// Multi-Endpoint Latency Registry
// =============================================================================

// LatencyRegistry manages latency trackers for multiple endpoints.
type LatencyRegistry struct {
	mu       sync.RWMutex
	trackers map[string]*LatencyTracker
	window   int
}

// NewLatencyRegistry creates a new latency registry.
func NewLatencyRegistry(windowSize int) *LatencyRegistry {
	return &LatencyRegistry{
		trackers: make(map[string]*LatencyTracker),
		window:   windowSize,
	}
}

// Record records a latency for the given endpoint.
func (r *LatencyRegistry) Record(endpoint string, d time.Duration) {
	r.mu.RLock()
	tracker, ok := r.trackers[endpoint]
	r.mu.RUnlock()

	if !ok {
		r.mu.Lock()
		// Double-check after acquiring write lock
		if tracker, ok = r.trackers[endpoint]; !ok {
			tracker = NewLatencyTracker(r.window)
			r.trackers[endpoint] = tracker
		}
		r.mu.Unlock()
	}

	tracker.Record(d)
}

// Stats returns latency statistics for a specific endpoint.
func (r *LatencyRegistry) Stats(endpoint string) LatencyStats {
	r.mu.RLock()
	tracker, ok := r.trackers[endpoint]
	r.mu.RUnlock()

	if !ok {
		return LatencyStats{}
	}
	return tracker.Stats()
}

// AllStats returns latency statistics for all endpoints.
func (r *LatencyRegistry) AllStats() map[string]LatencyStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]LatencyStats, len(r.trackers))
	for name, tracker := range r.trackers {
		result[name] = tracker.Stats()
	}
	return result
}

// Reset clears all trackers.
func (r *LatencyRegistry) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, tracker := range r.trackers {
		tracker.Reset()
	}
}

// =============================================================================
// Global Registry (Singleton)
// =============================================================================

var (
	globalRegistry     *LatencyRegistry
	globalRegistryOnce sync.Once
)

// GlobalRegistry returns the global latency registry.
func GlobalRegistry() *LatencyRegistry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewLatencyRegistry(1000)
	})
	return globalRegistry
}

// RecordLatency is a convenience function to record latency to the global registry.
func RecordLatency(endpoint string, d time.Duration) {
	GlobalRegistry().Record(endpoint, d)
}

// GetLatencyStats is a convenience function to get stats from the global registry.
func GetLatencyStats(endpoint string) LatencyStats {
	return GlobalRegistry().Stats(endpoint)
}

// GetAllLatencyStats returns all stats from the global registry.
func GetAllLatencyStats() map[string]LatencyStats {
	return GlobalRegistry().AllStats()
}
