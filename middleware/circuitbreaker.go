package middleware

import (
	"context"
	"errors"
	"sync"
	"time"
)

// CircuitBreakerConfig holds the configuration for the CircuitBreaker middleware.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening the circuit.
	FailureThreshold int
	// Timeout is the duration the circuit breaker stays open before attempting to recover.
	Timeout time.Duration
	// SuccessThreshold is the number of successful attempts required to close the circuit from half-open.
	SuccessThreshold int
}

// CircuitBreakerOption is a functional option for configuring the CircuitBreaker middleware.
type CircuitBreakerOption func(*CircuitBreakerConfig)

// WithFailureThreshold sets the failure threshold.
func WithFailureThreshold(threshold int) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.FailureThreshold = threshold
	}
}

// WithTimeout sets the timeout duration.
func WithTimeout(d time.Duration) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.Timeout = d
	}
}

// WithSuccessThreshold sets the success threshold for half-open state.
func WithSuccessThreshold(threshold int) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.SuccessThreshold = threshold
	}
}

// CircuitBreaker returns a Middleware that implements the circuit breaker pattern.
func CircuitBreaker(opts ...CircuitBreakerOption) Middleware {
	cfg := &CircuitBreakerConfig{
		FailureThreshold: 3,
		Timeout:          30 * time.Second,
		SuccessThreshold: 2,
	}
	for _, o := range opts {
		o(cfg)
	}

	// We'll store state per tool name.
	type state struct {
		mu          sync.Mutex
		state       string // "closed", "open", "half-open"
		failureCount int
		successCount int
		lastFailureTime time.Time
	}
	states := map[string]*state{}
	var statesMu sync.Mutex

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			// Get or create state for this tool.
			statesMu.Lock()
			s, exists := states[toolName]
			if !exists {
				s = &state{
					state:       "closed",
					failureCount: 0,
					successCount: 0,
				}
				states[toolName] = s
			}
			statesMu.Unlock()

			s.mu.Lock()
			defer s.mu.Unlock()

			// Check if we are in open state and if the timeout has elapsed.
			if s.state == "open" {
				if time.Since(s.lastFailureTime) > cfg.Timeout {
					// Move to half-open to try a test request.
					s.state = "half-open"
					// Reset success count for half-open state.
					s.successCount = 0
				} else {
					// Still open, reject the request.
					return nil, errors.New("circuit breaker is open")
				}
			}

			// Call the next handler.
			result, err := next(ctx, toolName, args)

			if err != nil {
				// Increment failure count.
				s.failureCount++
				s.lastFailureTime = time.Now()

				// If we have reached the failure threshold, open the circuit.
				if s.state == "closed" && s.failureCount >= cfg.FailureThreshold {
					s.state = "open"
				} else if s.state == "half-open" {
					// Failure in half-open state goes back to open.
					s.state = "open"
				}
				return result, err
			}

			// Success.
			if s.state == "half-open" {
				s.successCount++
				if s.successCount >= cfg.SuccessThreshold {
					// Enough successes, close the circuit.
					s.state = "closed"
					s.failureCount = 0
					s.successCount = 0
				}
			} else if s.state == "closed" {
				// Reset failure count on success in closed state.
				s.failureCount = 0
			}
			return result, err
		}
	}
}