package middleware_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AKhilRaghav0/hamr/middleware"
)

// ---- helpers ----------------------------------------------------------------

// makeHandler returns a HandlerFunc that returns the given value and error.
func makeHandler(val any, err error) middleware.HandlerFunc {
	return func(_ context.Context, _ string, _ map[string]any) (any, error) {
		return val, err
	}
}

// ---- Chain ------------------------------------------------------------------

func TestChain_OrderIsPreserved(t *testing.T) {
	t.Parallel()

	var order []string

	makeMiddleware := func(name string) middleware.Middleware {
		return func(next middleware.HandlerFunc) middleware.HandlerFunc {
			return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
				order = append(order, name+":before")
				result, err := next(ctx, toolName, args)
				order = append(order, name+":after")
				return result, err
			}
		}
	}

	chain := middleware.Chain(
		makeMiddleware("A"),
		makeMiddleware("B"),
		makeMiddleware("C"),
	)

	handler := chain(makeHandler("ok", nil))
	_, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"A:before", "B:before", "C:before",
		"C:after", "B:after", "A:after",
	}
	if len(order) != len(want) {
		t.Fatalf("got order %v, want %v", order, want)
	}
	for i, step := range want {
		if order[i] != step {
			t.Errorf("step %d: got %q, want %q", i, order[i], step)
		}
	}
}

func TestChain_EmptyPassesThrough(t *testing.T) {
	t.Parallel()

	chain := middleware.Chain()
	handler := chain(makeHandler(42, nil))
	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Errorf("got %v, want 42", val)
	}
}

// ---- Logger -----------------------------------------------------------------

func TestLogger_SuccessDoesNotReturnError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mw := middleware.Logger(middleware.WithCustomLogger(logger))
	handler := mw(makeHandler("result", nil))

	val, err := handler(context.Background(), "myTool", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "result" {
		t.Errorf("got %v, want \"result\"", val)
	}

	log := buf.String()
	if !strings.Contains(log, "myTool") {
		t.Errorf("log output missing tool name; got: %s", log)
	}
	if !strings.Contains(log, "succeeded") {
		t.Errorf("log output missing success message; got: %s", log)
	}
}

func TestLogger_ErrorIsLogged(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mw := middleware.Logger(middleware.WithCustomLogger(logger))
	handler := mw(makeHandler(nil, errors.New("boom")))

	_, err := handler(context.Background(), "failTool", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	log := buf.String()
	if !strings.Contains(log, "failTool") {
		t.Errorf("log output missing tool name; got: %s", log)
	}
	if !strings.Contains(log, "boom") {
		t.Errorf("log output missing error message; got: %s", log)
	}
	if !strings.Contains(log, "ERROR") {
		t.Errorf("expected ERROR level in log; got: %s", log)
	}
}

func TestLogger_WithLogLevel(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mw := middleware.Logger(
		middleware.WithCustomLogger(logger),
		middleware.WithLogLevel(slog.LevelDebug),
	)
	handler := mw(makeHandler("ok", nil))

	_, _ = handler(context.Background(), "tool", nil)

	if !strings.Contains(buf.String(), "DEBUG") {
		t.Errorf("expected DEBUG level in log; got: %s", buf.String())
	}
}

// ---- Recovery ---------------------------------------------------------------

func TestRecovery_CatchesPanic(t *testing.T) {
	t.Parallel()

	panicHandler := middleware.HandlerFunc(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		panic("something went wrong")
	})

	mw := middleware.Recovery()
	handler := mw(panicHandler)

	val, err := handler(context.Background(), "panicTool", nil)
	if err == nil {
		t.Fatal("expected error from panic recovery, got nil")
	}
	if val != nil {
		t.Errorf("expected nil result after panic, got %v", val)
	}
	if !strings.Contains(err.Error(), "panicTool") {
		t.Errorf("error should mention tool name; got: %v", err)
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Errorf("error should include panic value; got: %v", err)
	}
}

func TestRecovery_NoPanicPassesThrough(t *testing.T) {
	t.Parallel()

	mw := middleware.Recovery()
	handler := mw(makeHandler("safe", nil))

	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "safe" {
		t.Errorf("got %v, want \"safe\"", val)
	}
}

func TestRecovery_NonStringPanicValue(t *testing.T) {
	t.Parallel()

	panicHandler := middleware.HandlerFunc(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		panic(42)
	})

	mw := middleware.Recovery()
	handler := mw(panicHandler)

	_, err := handler(context.Background(), "tool", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("error should include panic value 42; got: %v", err)
	}
}

// ---- RateLimit --------------------------------------------------------------

func TestRateLimit_AllowsWithinLimit(t *testing.T) {
	t.Parallel()

	mw := middleware.RateLimit(10)
	handler := mw(makeHandler("ok", nil))

	// A single request must always succeed.
	_, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("first request should succeed, got: %v", err)
	}
}

func TestRateLimit_BlocksWhenExceeded(t *testing.T) {
	t.Parallel()

	// Set a limit of 1 req/s. Fire enough requests that at least one is
	// rejected. The bucket starts full (1 token) so the first request
	// consumes it; all subsequent ones within the same second must fail.
	mw := middleware.RateLimit(1)
	handler := mw(makeHandler("ok", nil))

	const attempts = 10
	var rejected int
	for i := 0; i < attempts; i++ {
		_, err := handler(context.Background(), "tool", nil)
		if err != nil {
			rejected++
		}
	}

	if rejected == 0 {
		t.Errorf("expected at least one request to be rate-limited out of %d attempts", attempts)
	}
}

func TestRateLimit_IndependentPerTool(t *testing.T) {
	t.Parallel()

	// Rate limit of 1 req/s. Exhaust the bucket for "toolA", then verify
	// "toolB" can still proceed.
	mw := middleware.RateLimit(1)
	handler := mw(makeHandler("ok", nil))

	// Exhaust toolA.
	for i := 0; i < 5; i++ {
		handler(context.Background(), "toolA", nil) //nolint:errcheck
	}

	// toolB should have its own full bucket.
	_, err := handler(context.Background(), "toolB", nil)
	if err != nil {
		t.Errorf("toolB should not be rate-limited, got: %v", err)
	}
}

func TestRateLimit_RefillsOverTime(t *testing.T) {
	t.Parallel()

	// 2 req/s → 1 token refills every 500 ms.
	mw := middleware.RateLimit(2)
	handler := mw(makeHandler("ok", nil))

	// Drain the bucket.
	for i := 0; i < 10; i++ {
		handler(context.Background(), "tool", nil) //nolint:errcheck
	}

	// After 600 ms at least one token should have refilled.
	time.Sleep(600 * time.Millisecond)

	_, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Errorf("expected request to succeed after refill window, got: %v", err)
	}
}

// ---- Timeout ----------------------------------------------------------------

func TestTimeout_FastHandlerSucceeds(t *testing.T) {
	t.Parallel()

	mw := middleware.Timeout(500 * time.Millisecond)
	handler := mw(makeHandler("fast", nil))

	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "fast" {
		t.Errorf("got %v, want \"fast\"", val)
	}
}

func TestTimeout_SlowHandlerIsInterrupted(t *testing.T) {
	t.Parallel()

	slowHandler := middleware.HandlerFunc(func(ctx context.Context, _ string, _ map[string]any) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Second):
			return "slow", nil
		}
	})

	mw := middleware.Timeout(50 * time.Millisecond)
	handler := mw(slowHandler)

	start := time.Now()
	_, err := handler(context.Background(), "slowTool", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "slowTool") {
		t.Errorf("error should mention tool name; got: %v", err)
	}
	// Sanity check: we should not have waited anywhere near 10 seconds.
	if elapsed > 2*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestTimeout_RespectsAlreadyCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	mw := middleware.Timeout(5 * time.Second)
	handler := mw(makeHandler("ok", nil))

	// The handler itself ignores context, so the result depends on scheduling.
	// What matters is that no goroutine leaks and the call returns.
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler(ctx, "tool", nil) //nolint:errcheck
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("handler did not return after context was cancelled")
	}
}

// ---- Cache ------------------------------------------------------------------

func TestCache_ReturnsCachedResponse(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int64

	countingHandler := middleware.HandlerFunc(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		callCount.Add(1)
		return "response", nil
	})

	mw := middleware.Cache(5 * time.Second)
	handler := mw(countingHandler)

	args := map[string]any{"key": "value"}

	val1, err := handler(context.Background(), "tool", args)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}

	val2, err := handler(context.Background(), "tool", args)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}

	if val1 != val2 {
		t.Errorf("cached value mismatch: %v != %v", val1, val2)
	}
	if callCount.Load() != 1 {
		t.Errorf("expected underlying handler called once, got %d", callCount.Load())
	}
}

func TestCache_DifferentArgsMissCacheAndDelegate(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int64

	countingHandler := middleware.HandlerFunc(func(_ context.Context, _ string, args map[string]any) (any, error) {
		callCount.Add(1)
		return args["id"], nil
	})

	mw := middleware.Cache(5 * time.Second)
	handler := mw(countingHandler)

	handler(context.Background(), "tool", map[string]any{"id": 1})   //nolint:errcheck
	handler(context.Background(), "tool", map[string]any{"id": 2})   //nolint:errcheck
	handler(context.Background(), "tool", map[string]any{"id": 1})   //nolint:errcheck // cache hit

	if callCount.Load() != 2 {
		t.Errorf("expected 2 underlying calls (distinct args), got %d", callCount.Load())
	}
}

func TestCache_ArgsOrderIndependent(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int64

	countingHandler := middleware.HandlerFunc(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		callCount.Add(1)
		return "ok", nil
	})

	mw := middleware.Cache(5 * time.Second)
	handler := mw(countingHandler)

	handler(context.Background(), "tool", map[string]any{"a": 1, "b": 2}) //nolint:errcheck
	handler(context.Background(), "tool", map[string]any{"b": 2, "a": 1}) //nolint:errcheck // same args, different insertion order

	if callCount.Load() != 1 {
		t.Errorf("arg-order-independent caching failed: expected 1 call, got %d", callCount.Load())
	}
}

func TestCache_DoesNotCacheErrors(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int64

	failingHandler := middleware.HandlerFunc(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		callCount.Add(1)
		return nil, errors.New("service unavailable")
	})

	mw := middleware.Cache(5 * time.Second)
	handler := mw(failingHandler)

	for i := 0; i < 3; i++ {
		_, err := handler(context.Background(), "tool", nil)
		if err == nil {
			t.Fatalf("call %d: expected error, got nil", i)
		}
	}

	if callCount.Load() != 3 {
		t.Errorf("errors should not be cached; expected 3 calls, got %d", callCount.Load())
	}
}

func TestCache_ExpiresAfterTTL(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int64

	countingHandler := middleware.HandlerFunc(func(_ context.Context, _ string, _ map[string]any) (any, error) {
		callCount.Add(1)
		return "value", nil
	})

	ttl := 100 * time.Millisecond
	mw := middleware.Cache(ttl)
	handler := mw(countingHandler)

	handler(context.Background(), "tool", nil) //nolint:errcheck // populates cache

	// Wait for the TTL to expire.
	time.Sleep(ttl + 50*time.Millisecond)

	handler(context.Background(), "tool", nil) //nolint:errcheck // should miss and call through

	if callCount.Load() != 2 {
		t.Errorf("expected 2 calls after TTL expiry, got %d", callCount.Load())
	}
}

func TestCache_ConcurrentAccessIsSafe(t *testing.T) {
	t.Parallel()

	mw := middleware.Cache(5 * time.Second)
	handler := mw(makeHandler("shared", nil))

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = handler(context.Background(), "tool", map[string]any{"k": "v"})
		}()
	}

	wg.Wait() // race detector will catch any data races
}

// ---- Auth -------------------------------------------------------------------

func TestAuth_ValidTokenEnrichesContext(t *testing.T) {
	t.Parallel()

	type userKey struct{}

	validator := func(ctx context.Context, token string) (context.Context, error) {
		if token != "valid-token" {
			return ctx, errors.New("invalid token")
		}
		return context.WithValue(ctx, userKey{}, "alice"), nil
	}

	var capturedCtx context.Context
	inner := middleware.HandlerFunc(func(ctx context.Context, _ string, _ map[string]any) (any, error) {
		capturedCtx = ctx
		return "ok", nil
	})

	mw := middleware.Auth(validator)
	handler := mw(inner)

	ctx := context.WithValue(context.Background(), middleware.AuthTokenKey, "valid-token")
	_, err := handler(ctx, "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedCtx.Value(userKey{}) != "alice" {
		t.Error("enriched context not forwarded to inner handler")
	}
}

func TestAuth_MissingTokenReturnsError(t *testing.T) {
	t.Parallel()

	mw := middleware.Auth(func(ctx context.Context, _ string) (context.Context, error) {
		return ctx, nil
	})
	handler := mw(makeHandler("ok", nil))

	_, err := handler(context.Background(), "tool", nil)
	if err == nil {
		t.Fatal("expected error for missing token, got nil")
	}
	if !strings.Contains(err.Error(), "no token") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestAuth_InvalidTokenReturnsError(t *testing.T) {
	t.Parallel()

	validator := func(ctx context.Context, token string) (context.Context, error) {
		return ctx, errors.New("forbidden")
	}

	mw := middleware.Auth(validator)
	handler := mw(makeHandler("ok", nil))

	ctx := context.WithValue(context.Background(), middleware.AuthTokenKey, "bad-token")
	_, err := handler(ctx, "tool", nil)
	if err == nil {
		t.Fatal("expected error for invalid token, got nil")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("unexpected error message: %v", err)
	}
}
