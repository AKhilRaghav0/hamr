package middleware

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrCircuitBreakerOpen is returned when the circuit breaker is open and the request is not allowed.
var ErrCircuitBreakerOpen = errors.New("circuit breaker open")

// circuitBreakerState represents the state of the circuit breaker for a specific tool.
type circuitBreakerState struct {
	mu          sync.Mutex
	state       State // closed, open, halfOpen
	failures    int
	lastFailure time.Time
	timeout     time.Duration
	maxFailures uint32
}

// State of the circuit breaker.
type State uint8

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

// circuitBreakerConfig holds the resolved configuration for the CircuitBreaker middleware.
type circuitBreakerConfig struct {
	timeout  time.Duration
	maxFailures uint32
}

// CircuitBreakerOption is a functional option for configuring the CircuitBreaker middleware.
type CircuitBreakerOption func(*circuitBreakerConfig)

// WithTimeout sets the duration the circuit breaker stays open before transitioning to half-open.
// Default is 60 seconds.
func WithTimeout(d time.Duration) CircuitBreakerOption {
	return func(c *circuitBreakerConfig) {
		c.timeout = d
	}
}

// WithMaxFailures sets the number of consecutive failures before opening the circuit.
// Default is 5.
func WithMaxFailures(max uint32) CircuitBreakerOption {
	return func(c *circuitBreakerConfig) {
		c.maxFailures = max
	}
}

// CircuitBreaker returns a Middleware that implements the circuit breaker pattern to
// protect against repeated failures to external services. It tracks failures per tool name.
//
// When the circuit is open, it returns ErrCircuitBreakerOpen without calling the next handler.
// After the timeout, it allows a single request to test if the service has recovered (half-open).
// If that request succeeds, the circuit is closed; if it fails, it goes back to open.
func CircuitBreaker(opts ...CircuitBreakerOption) Middleware {
	cfg := &circuitBreakerConfig{
		timeout:     60 * time.Second,
		maxFailures: 5,
	}
	for _, o := range opts {
		o(cfg)
	}

	// Map of tool name to circuit breaker state.
	var states sync.Map

		return func(next HandlerFunc) HandlerFunc {
			return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
				// Get or create the state for this tool.
				if val, ok := states.Load(toolName); ok {
					cb := val.(*circuitBreakerState)
					// Process with existing cb
					return processCircuitBreaker(ctx, cb, next, toolName, args)
				}
				// Create new state
				newCB := &circuitBreakerState{
					timeout:   cfg.timeout,
					maxFailures: cfg.maxFailures,
				}
				// Store if not present, or use existing if raced
				if actual, loaded := states.LoadOrStore(toolName, newCB); loaded {
					cb := actual.(*circuitBreakerState)
					return processCircuitBreaker(ctx, cb, next, toolName, args)
				}
				cb := newCB
				return processCircuitBreaker(ctx, cb, next, toolName, args)
			}
		}
	}
	
	// processCircuitBreaker contains the core circuit breaker logic
	func processCircuitBreaker(ctx context.Context, cb *circuitBreakerState, next HandlerFunc, toolName string, args map[string]any) (any, error) {
		cb.mu.Lock()
		state := cb.state
		if state == StateOpen {
			// Check if timeout has elapsed to move to half-open.
			if time.Since(cb.lastFailure) > cb.timeout {
				cb.state = StateHalfOpen
			} else {
				cb.mu.Unlock()
				return nil, ErrCircuitBreakerOpen
			}
		}
		cb.mu.Unlock()

		// Call the next handler.
		result, err := next(ctx, toolName, args)

		cb.mu.Lock()
		defer cb.mu.Unlock()
		if err != nil {
			// Increment failure count and possibly open the circuit.
			cb.failures++
			if cb.failures >= int(cb.maxFailures) {
				cb.state = StateOpen
				cb.lastFailure = time.Now()
			}
		} else {
			// Success: reset failure count and close the circuit if in half-open.
			if cb.state == StateHalfOpen {
				cb.state = StateClosed
				cb.failures = 0
			} else if cb.state == StateClosed {
				cb.failures = 0
			}
		}

		return result, err
	}