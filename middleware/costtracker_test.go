package middleware_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AKhilRaghav0/hamr/middleware"
)

// ---- CostTracker ------------------------------------------------------------

func TestCostTracker_CallbackReceivesToolName(t *testing.T) {
	t.Parallel()

	var got middleware.CostStats
	mw := middleware.CostTracker(func(s middleware.CostStats) { got = s })
	handler := mw(makeHandler("ok", nil))

	_, err := handler(context.Background(), "myTool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ToolName != "myTool" {
		t.Errorf("got ToolName %q, want %q", got.ToolName, "myTool")
	}
}

func TestCostTracker_RequestTokensEstimatedFromArgs(t *testing.T) {
	t.Parallel()

	var got middleware.CostStats
	mw := middleware.CostTracker(func(s middleware.CostStats) { got = s })
	handler := mw(makeHandler("ok", nil))

	// args JSON will be {"key":"value"} = 15 chars → 3 tokens
	args := map[string]any{"key": "value"}
	_, _ = handler(context.Background(), "tool", args)

	if got.RequestTokens <= 0 {
		t.Errorf("expected positive RequestTokens for non-empty args, got %d", got.RequestTokens)
	}
}

func TestCostTracker_ResponseTokensEstimatedFromStringResult(t *testing.T) {
	t.Parallel()

	// 40-char string → 10 tokens
	result := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 38 'a' → 9 tokens

	var got middleware.CostStats
	mw := middleware.CostTracker(func(s middleware.CostStats) { got = s })
	handler := mw(makeHandler(result, nil))

	_, _ = handler(context.Background(), "tool", nil)

	expected := len(result) / 4
	if got.ResponseTokens != expected {
		t.Errorf("got ResponseTokens %d, want %d", got.ResponseTokens, expected)
	}
}

func TestCostTracker_ResponseTokensEstimatedFromNonStringResult(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name  string
		Value int
	}
	result := payload{Name: "test", Value: 99}

	var got middleware.CostStats
	mw := middleware.CostTracker(func(s middleware.CostStats) { got = s })
	handler := mw(makeHandler(result, nil))

	_, _ = handler(context.Background(), "tool", nil)

	if got.ResponseTokens <= 0 {
		t.Errorf("expected positive ResponseTokens for non-string result, got %d", got.ResponseTokens)
	}
}

func TestCostTracker_DurationIsNonNegative(t *testing.T) {
	t.Parallel()

	var got middleware.CostStats
	mw := middleware.CostTracker(func(s middleware.CostStats) { got = s })
	// Use a handler that does a tiny amount of work so duration is measurable
	handler := mw(func(ctx context.Context, toolName string, args map[string]any) (any, error) {
		sum := 0
		for i := 0; i < 1000; i++ {
			sum += i
		}
		_ = sum
		return "ok", nil
	})

	_, _ = handler(context.Background(), "tool", nil)

	if got.Duration < 0 {
		t.Errorf("expected non-negative Duration, got %v", got.Duration)
	}
}

func TestCostTracker_TotalEqualsRequestPlusResponse(t *testing.T) {
	t.Parallel()

	var got middleware.CostStats
	mw := middleware.CostTracker(func(s middleware.CostStats) { got = s })
	handler := mw(makeHandler("hello world", nil))

	_, _ = handler(context.Background(), "tool", map[string]any{"x": 1})

	if got.TotalTokens != got.RequestTokens+got.ResponseTokens {
		t.Errorf("TotalTokens %d != RequestTokens %d + ResponseTokens %d",
			got.TotalTokens, got.RequestTokens, got.ResponseTokens)
	}
}

func TestCostTracker_CallbackFiresOnError(t *testing.T) {
	t.Parallel()

	boom := errors.New("something failed")
	fired := false

	mw := middleware.CostTracker(func(s middleware.CostStats) { fired = true })
	handler := mw(makeHandler(nil, boom))

	_, err := handler(context.Background(), "tool", nil)
	if !errors.Is(err, boom) {
		t.Fatalf("expected original error, got %v", err)
	}
	if !fired {
		t.Error("callback should fire even when handler returns an error")
	}
}
