package middleware_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/AKhilRaghav0/hamr/middleware"
)

// ---- MaxResponseTokens ------------------------------------------------------

func TestMaxResponseTokens_ShortResponsePassesThrough(t *testing.T) {
	t.Parallel()

	mw := middleware.MaxResponseTokens(100)
	handler := mw(makeHandler("hello", nil))

	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "hello" {
		t.Errorf("got %v, want \"hello\"", val)
	}
}

func TestMaxResponseTokens_LongStringIsTruncated(t *testing.T) {
	t.Parallel()

	const maxTokens = 10 // maxChars = 40
	// Build a string that clearly exceeds 40 chars with no newlines.
	long := strings.Repeat("x", 200)

	mw := middleware.MaxResponseTokens(maxTokens)
	handler := mw(makeHandler(long, nil))

	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", val)
	}

	if len(s) >= len(long) {
		t.Errorf("response was not truncated: len=%d", len(s))
	}
	if !strings.Contains(s, "truncated") {
		t.Errorf("truncation message missing in: %s", s)
	}
}

func TestMaxResponseTokens_TruncationCutsAtLastNewline(t *testing.T) {
	t.Parallel()

	const maxTokens = 10 // maxChars = 40
	// Construct a string of 80 chars with a newline well past the midpoint of
	// the 40-char window (position 30).
	//   chars 0-29: 'a'
	//   char  30:   '\n'
	//   chars 31-79: 'b'
	body := strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 49)

	mw := middleware.MaxResponseTokens(maxTokens)
	handler := mw(makeHandler(body, nil))

	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", val)
	}

	// The result should start with the 'a' segment followed by the truncation
	// message, with no 'b' characters before the message.
	prefix := strings.SplitN(s, "\n\n...", 2)[0]
	if strings.Contains(prefix, "b") {
		t.Errorf("expected cut at newline, but 'b' chars present in prefix: %s", prefix)
	}
}

func TestMaxResponseTokens_NonStringPassesThrough(t *testing.T) {
	t.Parallel()

	type myResult struct{ Value int }
	res := myResult{Value: 42}

	mw := middleware.MaxResponseTokens(1) // extremely low limit
	handler := mw(makeHandler(res, nil))

	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, ok := val.(myResult)
	if !ok {
		t.Fatalf("expected myResult, got %T", val)
	}
	if got.Value != 42 {
		t.Errorf("got %v, want {Value:42}", got)
	}
}

func TestMaxResponseTokens_ErrorPassesThrough(t *testing.T) {
	t.Parallel()

	boom := errors.New("downstream error")
	long := strings.Repeat("x", 10000)

	mw := middleware.MaxResponseTokens(10)
	handler := mw(makeHandler(long, boom))

	val, err := handler(context.Background(), "tool", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, boom) {
		t.Errorf("got error %v, want %v", err, boom)
	}
	// The raw (untruncated) value should be forwarded alongside the error.
	if val != long {
		t.Errorf("expected original value forwarded with error")
	}
}

func TestMaxResponseTokens_TruncationMessageIncludesTokenCount(t *testing.T) {
	t.Parallel()

	const maxTokens = 25
	long := strings.Repeat("z", 500)

	mw := middleware.MaxResponseTokens(maxTokens)
	handler := mw(makeHandler(long, nil))

	val, err := handler(context.Background(), "tool", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s, ok := val.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", val)
	}

	if !strings.Contains(s, "25") {
		t.Errorf("truncation message should include token count 25; got: %s", s)
	}
}
