// Package metrics provides pool monitoring utilities.
package metrics

import (
	"database/sql"
	"sync"
	"time"
)

// =============================================================================
// Database Pool Monitor
// =============================================================================

// DBPoolStats holds database connection pool statistics.
type DBPoolStats struct {
	// Current state
	OpenConnections int `json:"open_connections"`
	InUse           int `json:"in_use"`
	Idle            int `json:"idle"`

	// Limits
	MaxOpenConnections int `json:"max_open_connections"`

	// Cumulative stats
	WaitCount         int64         `json:"wait_count"`
	WaitDuration      time.Duration `json:"wait_duration"`
	MaxIdleClosed     int64         `json:"max_idle_closed"`
	MaxIdleTimeClosed int64         `json:"max_idle_time_closed"`
	MaxLifetimeClosed int64         `json:"max_lifetime_closed"`
}

// ToMap converts stats to a map for JSON serialization.
func (s DBPoolStats) ToMap() map[string]any {
	return map[string]any{
		"open_connections":     s.OpenConnections,
		"in_use":               s.InUse,
		"idle":                 s.Idle,
		"max_open_connections": s.MaxOpenConnections,
		"wait_count":           s.WaitCount,
		"wait_duration_ms":     s.WaitDuration.Milliseconds(),
		"max_idle_closed":      s.MaxIdleClosed,
		"max_idle_time_closed": s.MaxIdleTimeClosed,
		"max_lifetime_closed":  s.MaxLifetimeClosed,
	}
}

// GetDBPoolStats retrieves pool statistics from a sql.DB instance.
func GetDBPoolStats(db *sql.DB) DBPoolStats {
	if db == nil {
		return DBPoolStats{}
	}

	stats := db.Stats()
	return DBPoolStats{
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
		MaxOpenConnections: stats.MaxOpenConnections,
		WaitCount:          stats.WaitCount,
		WaitDuration:       stats.WaitDuration,
		MaxIdleClosed:      stats.MaxIdleClosed,
		MaxIdleTimeClosed:  stats.MaxIdleTimeClosed,
		MaxLifetimeClosed:  stats.MaxLifetimeClosed,
	}
}

// =============================================================================
// Pool Health Monitor
// =============================================================================

// PoolHealthStatus indicates the health of a connection pool.
type PoolHealthStatus string

const (
	PoolHealthy   PoolHealthStatus = "healthy"
	PoolDegraded  PoolHealthStatus = "degraded"
	PoolUnhealthy PoolHealthStatus = "unhealthy"
)

// PoolHealth represents the health assessment of a pool.
type PoolHealth struct {
	Status      PoolHealthStatus `json:"status"`
	Utilization float64          `json:"utilization"` // 0.0 - 1.0
	WaitRatio   float64          `json:"wait_ratio"`  // Wait time / total time
	Message     string           `json:"message,omitempty"`
}

// AssessDBPoolHealth evaluates the health of a database pool.
func AssessDBPoolHealth(stats DBPoolStats) PoolHealth {
	if stats.MaxOpenConnections == 0 {
		return PoolHealth{Status: PoolHealthy, Message: "unlimited connections"}
	}

	utilization := float64(stats.InUse) / float64(stats.MaxOpenConnections)

	var status PoolHealthStatus
	var message string

	switch {
	case utilization >= 0.95:
		status = PoolUnhealthy
		message = "pool nearly exhausted"
	case utilization >= 0.80:
		status = PoolDegraded
		message = "high pool utilization"
	default:
		status = PoolHealthy
		message = "pool operating normally"
	}

	// Check for excessive waiting
	if stats.WaitCount > 0 && stats.WaitDuration > 5*time.Second {
		if status == PoolHealthy {
			status = PoolDegraded
		}
		message = "elevated connection wait times"
	}

	return PoolHealth{
		Status:      status,
		Utilization: utilization,
		Message:     message,
	}
}

// =============================================================================
// Multi-Pool Monitor Registry
// =============================================================================

// PoolMonitor tracks multiple connection pools.
type PoolMonitor struct {
	mu    sync.RWMutex
	pools map[string]*sql.DB
}

// NewPoolMonitor creates a new pool monitor.
func NewPoolMonitor() *PoolMonitor {
	return &PoolMonitor{
		pools: make(map[string]*sql.DB),
	}
}

// Register adds a database pool to be monitored.
func (m *PoolMonitor) Register(name string, db *sql.DB) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pools[name] = db
}

// Unregister removes a database pool from monitoring.
func (m *PoolMonitor) Unregister(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.pools, name)
}

// Stats returns statistics for a specific pool.
func (m *PoolMonitor) Stats(name string) (DBPoolStats, bool) {
	m.mu.RLock()
	db, ok := m.pools[name]
	m.mu.RUnlock()

	if !ok {
		return DBPoolStats{}, false
	}
	return GetDBPoolStats(db), true
}

// AllStats returns statistics for all registered pools.
func (m *PoolMonitor) AllStats() map[string]DBPoolStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]DBPoolStats, len(m.pools))
	for name, db := range m.pools {
		result[name] = GetDBPoolStats(db)
	}
	return result
}

// AllHealth returns health assessments for all registered pools.
func (m *PoolMonitor) AllHealth() map[string]PoolHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]PoolHealth, len(m.pools))
	for name, db := range m.pools {
		stats := GetDBPoolStats(db)
		result[name] = AssessDBPoolHealth(stats)
	}
	return result
}

// =============================================================================
// Global Pool Monitor (Singleton)
// =============================================================================

var (
	globalPoolMonitor     *PoolMonitor
	globalPoolMonitorOnce sync.Once
)

// GlobalPoolMonitor returns the global pool monitor.
func GlobalPoolMonitor() *PoolMonitor {
	globalPoolMonitorOnce.Do(func() {
		globalPoolMonitor = NewPoolMonitor()
	})
	return globalPoolMonitor
}

// RegisterPool registers a pool with the global monitor.
func RegisterPool(name string, db *sql.DB) {
	GlobalPoolMonitor().Register(name, db)
}

// GetPoolStats gets stats from the global pool monitor.
func GetPoolStats(name string) (DBPoolStats, bool) {
	return GlobalPoolMonitor().Stats(name)
}

// GetAllPoolStats gets all stats from the global pool monitor.
func GetAllPoolStats() map[string]DBPoolStats {
	return GlobalPoolMonitor().AllStats()
}

// GetAllPoolHealth gets health for all pools from the global monitor.
func GetAllPoolHealth() map[string]PoolHealth {
	return GlobalPoolMonitor().AllHealth()
}
