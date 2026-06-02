package middleware

import (
	"context"
	"errors"
	"testing"
)

func TestCompression_StringResponse(t *testing.T) {
	t.Parallel()

	// Create a simple handler that returns a string.
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "hello world", nil
	}

	// Create the compression middleware with enabled set to true.
	mw := Compression(WithCompressionEnabled(true))
	chained := mw(handler)

	// Invoke the chained handler.
	result, err := chained(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	str, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
	expected := "COMPRESSED:hello world"
	if str != expected {
		t.Errorf("Expected compressed result %q, got %q", expected, str)
	}
}

func TestCompression_NoCompressionWhenDisabled(t *testing.T) {
	t.Parallel()

	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "hello world", nil
	}

	mw := Compression(WithCompressionEnabled(false))
	chained := mw(handler)

	result, err := chained(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	str, ok := result.(string)
	if !ok {
		t.Fatalf("Expected string result, got %T", result)
	}
	if str != "hello world" {
		t.Errorf("Expected unchanged result %q, got %q", "hello world", str)
	}
}

func TestCompression_PassesThroughError(t *testing.T) {
	t.Parallel()

	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return nil, errors.New("something went wrong")
	}

	mw := Compression(WithCompressionEnabled(true))
	chained := mw(handler)

	result, err := chained(context.Background(), "test", nil)
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	if result != nil {
		t.Errorf("Expected nil result, got %v", result)
	}
}

// We'll also test that non-string types are passed through when compression is enabled.
func TestCompression_NonStringResponse(t *testing.T) {
	t.Parallel()

	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return 42, nil
	}

	mw := Compression(WithCompressionEnabled(true))
	chained := mw(handler)

	result, err := chained(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	num, ok := result.(int)
	if !ok {
		t.Fatalf("Expected int result, got %T", result)
	}
	if num != 42 {
		t.Errorf("Expected 42, got %v", num)
	}
}