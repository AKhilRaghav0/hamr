package otelhamr_test

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/AKhilRaghav0/hamr/middleware"
	"github.com/AKhilRaghav0/hamr/otelhamr"
)

// makeHandler returns a HandlerFunc that returns the given value and error.
func makeHandler(val any, err error) middleware.HandlerFunc {
	return func(_ context.Context, _ string, _ map[string]any) (any, error) {
		return val, err
	}
}

// TestTracing_NoopTracerDoesNotCrash ensures that the middleware initialises
// and invokes the inner handler without panicking when backed by a noop tracer.
func TestTracing_NoopTracerDoesNotCrash(t *testing.T) {
	t.Parallel()

	tracer := noop.NewTracerProvider().Tracer("test")
	mw := otelhamr.Tracing(tracer)
	handler := mw(makeHandler("ok", nil))

	_, err := handler(context.Background(), "myTool", map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestTracing_SuccessfulCallCompletesWithoutError verifies that a successful
// handler invocation is passed through without modification.
func TestTracing_SuccessfulCallCompletesWithoutError(t *testing.T) {
	t.Parallel()

	tracer := noop.NewTracerProvider().Tracer("test")
	mw := otelhamr.Tracing(tracer)
	handler := mw(makeHandler("result", nil))

	val, err := handler(context.Background(), "tool", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if val != "result" {
		t.Errorf("expected result %q, got %v", "result", val)
	}
}

// TestTracing_ErrorCallReturnsOriginalError verifies that an error from the
// inner handler is returned unchanged to the caller.
func TestTracing_ErrorCallReturnsOriginalError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("handler error")
	tracer := noop.NewTracerProvider().Tracer("test")
	mw := otelhamr.Tracing(tracer)
	handler := mw(makeHandler(nil, sentinel))

	_, err := handler(context.Background(), "failTool", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got: %v", err)
	}
}

// TestTracing_ResultPassedThroughUnchanged verifies that an arbitrary result
// value from the inner handler is returned unchanged to the caller.
func TestTracing_ResultPassedThroughUnchanged(t *testing.T) {
	t.Parallel()

	type myResult struct{ Value int }
	expected := myResult{Value: 42}

	tracer := noop.NewTracerProvider().Tracer("test")
	mw := otelhamr.Tracing(tracer)
	handler := mw(makeHandler(expected, nil))

	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := val.(myResult)
	if !ok {
		t.Fatalf("expected myResult, got %T", val)
	}
	if got.Value != expected.Value {
		t.Errorf("expected Value=%d, got Value=%d", expected.Value, got.Value)
	}
}

// TestTracing_NilArgsDoesNotPanic verifies that the middleware handles a nil
// args map without panicking.
func TestTracing_NilArgsDoesNotPanic(t *testing.T) {
	t.Parallel()

	tracer := noop.NewTracerProvider().Tracer("test")
	mw := otelhamr.Tracing(tracer)
	handler := mw(makeHandler("ok", nil))

	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "ok" {
		t.Errorf("expected %q, got %v", "ok", val)
	}
}
