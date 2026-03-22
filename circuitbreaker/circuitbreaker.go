// Package circuitbreaker implements a simple circuit breaker that tracks
// consecutive failures and temporarily stops attempts when a threshold is
// exceeded.
package circuitbreaker

import (
	"sync"
	"time"
)

const (
	// StateClosed indicates normal operation where all requests pass through.
	StateClosed = "closed"
	// StateOpen indicates the circuit has tripped and requests are rejected.
	StateOpen = "open"
	// StateHalfOpen indicates the cooldown has elapsed and one probe request
	// is allowed through.
	StateHalfOpen = "half-open"
)

// CircuitBreaker tracks consecutive failures and opens when a threshold
// is exceeded, preventing further attempts until a cooldown period elapses.
type CircuitBreaker struct {
	mu        sync.Mutex
	failures  int
	threshold int
	cooldown  time.Duration
	openedAt  time.Time
	state     string
}

// New creates a CircuitBreaker that opens after threshold consecutive
// failures and stays open for cooldown duration before allowing a probe.
func New(threshold int, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold: threshold,
		cooldown:  cooldown,
		state:     StateClosed,
	}
}

// Allow reports whether a request should proceed. Returns false when the
// circuit is open and the cooldown has not elapsed.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.openedAt) >= cb.cooldown {
			cb.state = StateHalfOpen
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return true
	}
}

// RecordSuccess resets the failure counter and closes the circuit.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.state = StateClosed
}

// RecordFailure increments the failure counter. If the threshold is reached,
// the circuit opens.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	if cb.failures >= cb.threshold {
		cb.state = StateOpen
		cb.openedAt = time.Now()
	}
}

// State returns the current state: "closed", "open", or "half-open".
func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	return cb.state
}
