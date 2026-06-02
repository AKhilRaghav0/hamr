package middleware

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedToOpenOnFailures(t *testing.T) {
	t.Parallel()

	// Handler that always returns an error.
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return nil, errors.New("always fails")
	}

	// Create circuit breaker with low failure threshold for testing.
	mw := CircuitBreaker(
		WithFailureThreshold(2),
		WithTimeout(10*time.Second), // long timeout so we don't wait
		WithSuccessThreshold(2),
	)
	chained := mw(handler)

	// First call: should fail and transition state but still be closed? Actually after first failure, still closed.
	// We expect an error and the circuit breaker to count the failure.
	result1, err1 := chained(context.Background(), "test", nil)
	if err1 == nil {
		t.Fatalf("Expected error on first call")
	}
	if result1 != nil {
		t.Fatalf("Expected nil result on error")
	}

	// Second call: should fail and now reach the threshold, so circuit opens.
	result2, err2 := chained(context.Background(), "test", nil)
	if err2 == nil {
		t.Fatalf("Expected error on second call")
	}
	if result2 != nil {
		t.Fatalf("Expected nil result on error")
	}

	// Third call: circuit should be open, so we get an error without calling the handler.
	result3, err3 := chained(context.Background(), "test", nil)
	if err3 == nil {
		t.Fatalf("Expected error on third call (circuit open)")
	}
	if result3 != nil {
		t.Fatalf("Expected nil result on error")
	}
	// Check that the error indicates the circuit is open.
	if err3.Error() != "circuit breaker is open" {
		t.Errorf("Expected circuit open error, got %v", err3)
	}
}

func TestCircuitBreaker_RecoversAfterTimeout(t *testing.T) {
	t.Parallel()

	// Handler that fails for the first two calls, then succeeds.
	failCount := 0
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		failCount++
		if failCount <= 2 {
			return nil, errors.New("fail")
		}
		return "success", nil
	}

	// Create circuit breaker with failure threshold 2 and short timeout.
	mw := CircuitBreaker(
		WithFailureThreshold(2),
		WithTimeout(10*time.Millisecond), // short timeout for test
		WithSuccessThreshold(1),
	)
	chained := mw(handler)

	// First two calls: should fail.
	for i := 0; i < 2; i++ {
		result, err := chained(context.Background(), "test", nil)
		if err == nil {
			t.Fatalf("Expected error on call %d", i+1)
		}
		if result != nil {
			t.Fatalf("Expected nil result on error")
		}
	}

	// Wait for timeout to pass so circuit goes to half-open.
	time.Sleep(20 * time.Millisecond)

	// Next call: should succeed (half-open state, and we set success threshold to 1).
	result, err := chained(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("Expected success after timeout, got error: %v", err)
	}
	if result != "success" {
		t.Errorf("Expected success result, got %v", result)
	}

	// Subsequent calls should succeed (circuit closed).
	result2, err2 := chained(context.Background(), "test", nil)
	if err2 != nil {
		t.Fatalf("Expected success after recovery, got error: %v", err2)
	}
	if result2 != "success" {
		t.Errorf("Expected success result, got %v", result2)
	}
}

func TestCircuitBreaker_PassesThroughSuccessWhenClosed(t *testing.T) {
	t.Parallel()

	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "ok", nil
	}

	mw := CircuitBreaker()
	chained := mw(handler)

	result, err := chained(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "ok" {
		t.Errorf("Expected 'ok', got %v", result)
	}
}