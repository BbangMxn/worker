// Package resilience provides fault tolerance patterns for external service calls.
package resilience

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// CircuitState represents the state of the circuit breaker.
type CircuitState int32

const (
	StateClosed   CircuitState = iota // Normal operation, requests pass through
	StateOpen                         // Circuit open, requests fail immediately
	StateHalfOpen                     // Testing if service recovered
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Errors returned by the circuit breaker.
var (
	ErrCircuitOpen    = errors.New("circuit breaker is open")
	ErrTooManyRequest = errors.New("too many requests in half-open state")
)

// CircuitBreakerConfig holds configuration for a circuit breaker.
type CircuitBreakerConfig struct {
	Name               string        // Name for logging/metrics
	FailureThreshold   int           // Number of failures before opening (default: 5)
	SuccessThreshold   int           // Number of successes to close from half-open (default: 2)
	Timeout            time.Duration // Time to wait before half-open (default: 30s)
	MaxHalfOpenRequest int           // Max concurrent requests in half-open (default: 1)
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig(name string) *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		Name:               name,
		FailureThreshold:   5,
		SuccessThreshold:   2,
		Timeout:            30 * time.Second,
		MaxHalfOpenRequest: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	name string

	state            int32 // atomic: CircuitState
	failureCount     int32 // atomic
	successCount     int32 // atomic
	halfOpenRequests int32 // atomic

	failureThreshold   int
	successThreshold   int
	timeout            time.Duration
	maxHalfOpenRequest int

	lastFailureTime time.Time
	mu              sync.RWMutex

	// Callbacks for monitoring
	onStateChange func(name string, from, to CircuitState)
}

// NewCircuitBreaker creates a new circuit breaker with the given config.
func NewCircuitBreaker(cfg *CircuitBreakerConfig) *CircuitBreaker {
	if cfg == nil {
		cfg = DefaultCircuitBreakerConfig("default")
	}

	return &CircuitBreaker{
		name:               cfg.Name,
		state:              int32(StateClosed),
		failureThreshold:   cfg.FailureThreshold,
		successThreshold:   cfg.SuccessThreshold,
		timeout:            cfg.Timeout,
		maxHalfOpenRequest: cfg.MaxHalfOpenRequest,
	}
}

// OnStateChange sets a callback for state changes.
func (cb *CircuitBreaker) OnStateChange(fn func(name string, from, to CircuitState)) {
	cb.mu.Lock()
	cb.onStateChange = fn
	cb.mu.Unlock()
}

// State returns the current state.
func (cb *CircuitBreaker) State() CircuitState {
	return CircuitState(atomic.LoadInt32(&cb.state))
}

// Name returns the circuit breaker name.
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// Execute runs the given function with circuit breaker protection.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	err := fn()
	cb.afterRequest(err)
	return err
}

// beforeRequest checks if the request should be allowed.
func (cb *CircuitBreaker) beforeRequest() error {
	state := cb.State()

	switch state {
	case StateClosed:
		return nil

	case StateOpen:
		// Check if timeout has passed
		cb.mu.RLock()
		lastFailure := cb.lastFailureTime
		cb.mu.RUnlock()

		if time.Since(lastFailure) > cb.timeout {
			// Transition to half-open
			cb.setState(StateHalfOpen)
			atomic.StoreInt32(&cb.halfOpenRequests, 0)
			atomic.StoreInt32(&cb.successCount, 0)
			return nil
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		// Limit concurrent requests in half-open state
		current := atomic.AddInt32(&cb.halfOpenRequests, 1)
		if int(current) > cb.maxHalfOpenRequest {
			atomic.AddInt32(&cb.halfOpenRequests, -1)
			return ErrTooManyRequest
		}
		return nil
	}

	return nil
}

// afterRequest updates state based on result.
func (cb *CircuitBreaker) afterRequest(err error) {
	state := cb.State()

	if err != nil {
		cb.recordFailure()

		switch state {
		case StateClosed:
			failures := atomic.LoadInt32(&cb.failureCount)
			if int(failures) >= cb.failureThreshold {
				cb.setState(StateOpen)
			}

		case StateHalfOpen:
			// Any failure in half-open goes back to open
			cb.setState(StateOpen)
			atomic.AddInt32(&cb.halfOpenRequests, -1)
		}
	} else {
		cb.recordSuccess()

		switch state {
		case StateHalfOpen:
			atomic.AddInt32(&cb.halfOpenRequests, -1)
			successes := atomic.LoadInt32(&cb.successCount)
			if int(successes) >= cb.successThreshold {
				cb.setState(StateClosed)
			}
		}
	}
}

// recordFailure records a failure.
func (cb *CircuitBreaker) recordFailure() {
	atomic.AddInt32(&cb.failureCount, 1)
	atomic.StoreInt32(&cb.successCount, 0)

	cb.mu.Lock()
	cb.lastFailureTime = time.Now()
	cb.mu.Unlock()
}

// recordSuccess records a success.
func (cb *CircuitBreaker) recordSuccess() {
	atomic.AddInt32(&cb.successCount, 1)

	// Reset failure count on success (in closed state)
	if cb.State() == StateClosed {
		atomic.StoreInt32(&cb.failureCount, 0)
	}
}

// setState atomically sets the state and triggers callback.
func (cb *CircuitBreaker) setState(newState CircuitState) {
	oldState := CircuitState(atomic.SwapInt32(&cb.state, int32(newState)))

	if oldState != newState {
		// Reset counters on state change
		atomic.StoreInt32(&cb.failureCount, 0)
		atomic.StoreInt32(&cb.successCount, 0)

		// Trigger callback
		cb.mu.RLock()
		callback := cb.onStateChange
		cb.mu.RUnlock()

		if callback != nil {
			callback(cb.name, oldState, newState)
		}
	}
}

// Reset forces the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.setState(StateClosed)
	atomic.StoreInt32(&cb.failureCount, 0)
	atomic.StoreInt32(&cb.successCount, 0)
	atomic.StoreInt32(&cb.halfOpenRequests, 0)
}

// Stats returns current circuit breaker statistics.
type CircuitBreakerStats struct {
	Name         string
	State        string
	Failures     int
	Successes    int
	LastFailure  time.Time
	HalfOpenReqs int
}

// Stats returns current statistics.
func (cb *CircuitBreaker) Stats() CircuitBreakerStats {
	cb.mu.RLock()
	lastFailure := cb.lastFailureTime
	cb.mu.RUnlock()

	return CircuitBreakerStats{
		Name:         cb.name,
		State:        cb.State().String(),
		Failures:     int(atomic.LoadInt32(&cb.failureCount)),
		Successes:    int(atomic.LoadInt32(&cb.successCount)),
		LastFailure:  lastFailure,
		HalfOpenReqs: int(atomic.LoadInt32(&cb.halfOpenRequests)),
	}
}
