package middleware

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTransformation_ArgsTransformer(t *testing.T) {
	t.Parallel()

	// Handler that returns the value of the "input" arg.
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return args["input"], nil
	}

	// Transformer that adds 10 to the input value (assuming it's an int).
	addTen := func(in any) (any, error) {
		if v, ok := in.(int); ok {
			return v + 10, nil
		}
		return nil, errors.New("expected int")
	}

	// We want to transform the args: the args map has a key "input" with an int.
	// We'll create a transformer that takes the args map and returns a new map with input+10.
	argsTransformer := func(in any) (any, error) {
		if m, ok := in.(map[string]any); ok {
			if v, ok := m["input"].(int); ok {
				newMap := map[string]any{"input": v + 10}
				return newMap, nil
			}
		}
		return nil, errors.New("expected map[string]any with int input")
	}

	mw := Transformation(WithArgsTransformer(argsTransformer))
	chained := mw(handler)

	// Call with input = 5.
	result, err := chained(context.Background(), "test", map[string]any{"input": 5})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != 15 {
		t.Errorf("Expected 15, got %v", result)
	}
}

func TestTransformation_ResultTransformer(t *testing.T) {
	t.Parallel()

	// Handler that returns a string.
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return "hello", nil
	}

	// Transformer that appends " world" to the string.
	appendWorld := func(in any) (any, error) {
		if s, ok := in.(string); ok {
			return s + " world", nil
		}
		return nil, errors.New("expected string")
	}

	mw := Transformation(WithResultTransformer(appendWorld))
	chained := mw(handler)

	result, err := chained(context.Background(), "test", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("Expected 'hello world', got %v", result)
	}
}

func TestTransformation_BothTransformers(t *testing.T) {
	t.Parallel()

	// Handler that returns the input string unchanged.
	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return args["input"], nil
	}

	// Args transformer: convert input string to uppercase.
	uppercaseArgs := func(in any) (any, error) {
		if m, ok := in.(map[string]any); ok {
			if s, ok := m["input"].(string); ok {
				newMap := map[string]any{"input": strings.ToUpper(s)}
				return newMap, nil
			}
		}
		return nil, errors.New("expected map with string input")
	}

	// Result transformer: append "!!" to the string.
	appendBangBang := func(in any) (any, error) {
		if s, ok := in.(string); ok {
			return s + "!!", nil
		}
		return nil, errors.New("expected string")
	}

	mw := Transformation(WithArgsTransformer(uppercaseArgs), WithResultTransformer(appendBangBang))
	chained := mw(handler)

	// Call with input = "hello".
	result, err := chained(context.Background(), "test", map[string]any{"input": "hello"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result != "HELLO!!" {
		t.Errorf("Expected 'HELLO!!', got %v", result)
	}
}

func TestTransformation_PassesThroughError(t *testing.T) {
	t.Parallel()

	handler := func(ctx context.Context, tool string, args map[string]any) (any, error) {
		return nil, errors.New("handler error")
	}

	mw := Transformation(WithResultTransformer(func(in any) (any, error) {
		return "transformed", nil
	}))
	chained := mw(handler)

	result, err := chained(context.Background(), "test", nil)
	if err == nil {
		t.Fatalf("Expected error")
	}
	if result != nil {
		t.Fatalf("Expected nil result on error")
	}
}