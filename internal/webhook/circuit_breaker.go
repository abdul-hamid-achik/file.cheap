package webhook

import (
	"sync"
	"time"
)

type circuitState int

const (
	stateClosed circuitState = iota
	stateOpen
	stateHalfOpen
)

type endpointHealth struct {
	failures    int
	lastFailure time.Time
	state       circuitState
}

// CircuitBreaker implements a circuit breaker pattern for webhook endpoints
type CircuitBreaker struct {
	mu               sync.RWMutex
	endpoints        map[string]*endpointHealth
	failureThreshold int
	recoveryTime     time.Duration
}

// NewCircuitBreaker creates a new circuit breaker with default settings
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		endpoints:        make(map[string]*endpointHealth),
		failureThreshold: 5,
		recoveryTime:     30 * time.Minute,
	}
}

// NewCircuitBreakerWithConfig creates a circuit breaker with custom settings
func NewCircuitBreakerWithConfig(failureThreshold int, recoveryTime time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		endpoints:        make(map[string]*endpointHealth),
		failureThreshold: failureThreshold,
		recoveryTime:     recoveryTime,
	}
}

// Allow checks if a request to the endpoint should be allowed
func (cb *CircuitBreaker) Allow(endpoint string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	health, exists := cb.endpoints[endpoint]
	if !exists {
		return true
	}

	switch health.state {
	case stateClosed:
		return true
	case stateOpen:
		if time.Since(health.lastFailure) > cb.recoveryTime {
			health.state = stateHalfOpen
			return true
		}
		return false
	case stateHalfOpen:
		return true
	}
	return true
}

// RecordSuccess records a successful request and resets the circuit
func (cb *CircuitBreaker) RecordSuccess(endpoint string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	health, exists := cb.endpoints[endpoint]
	if !exists {
		return
	}

	health.failures = 0
	health.state = stateClosed
}

// RecordFailure records a failed request and potentially opens the circuit
func (cb *CircuitBreaker) RecordFailure(endpoint string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	health, exists := cb.endpoints[endpoint]
	if !exists {
		health = &endpointHealth{}
		cb.endpoints[endpoint] = health
	}

	health.failures++
	health.lastFailure = time.Now()

	if health.state == stateHalfOpen {
		health.state = stateOpen
	} else if health.failures >= cb.failureThreshold {
		health.state = stateOpen
	}
}

// IsOpen returns whether the circuit is open for the given endpoint
func (cb *CircuitBreaker) IsOpen(endpoint string) bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	health, exists := cb.endpoints[endpoint]
	if !exists {
		return false
	}
	return health.state == stateOpen
}

// GetState returns the current state for the given endpoint
func (cb *CircuitBreaker) GetState(endpoint string) string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	health, exists := cb.endpoints[endpoint]
	if !exists {
		return "closed"
	}

	switch health.state {
	case stateClosed:
		return "closed"
	case stateOpen:
		return "open"
	case stateHalfOpen:
		return "half_open"
	}
	return "unknown"
}

// Reset resets the circuit breaker state for an endpoint
func (cb *CircuitBreaker) Reset(endpoint string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	delete(cb.endpoints, endpoint)
}
