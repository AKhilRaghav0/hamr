package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// tokenBucket tracks the state of a single per-tool token bucket.
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64    // tokens refilled per second (equal to maxTokens for a standard token bucket)
	lastRefill time.Time
}

// allow reports whether a token is available and, if so, consumes one.
func (b *tokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill)
	b.lastRefill = now

	// Refill proportionally to elapsed time: add refillRate tokens per second.
	b.tokens += elapsed.Seconds() * b.refillRate
	if b.tokens > b.maxTokens {
		b.tokens = b.maxTokens
	}

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// bucketStore manages per-tool token buckets.
type bucketStore struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rps     float64
}

func newBucketStore(requestsPerSecond int) *bucketStore {
	return &bucketStore{
		buckets: make(map[string]*tokenBucket),
		rps:     float64(requestsPerSecond),
	}
}

func (s *bucketStore) get(toolName string) *tokenBucket {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.buckets[toolName]
	if !ok {
		b = &tokenBucket{
			tokens:     s.rps, // start full
			maxTokens:  s.rps,
			refillRate: s.rps, // tokens per second; elapsed.Seconds() * refillRate = tokens added
			lastRefill: time.Now(),
		}
		s.buckets[toolName] = b
	}
	return b
}

// RateLimit returns a Middleware that enforces a token-bucket rate limit of
// requestsPerSecond per tool name. Each distinct tool has its own independent
// bucket, so a burst on one tool does not affect another.
//
// When a request arrives and no token is available the handler is not called
// and an error is returned immediately to avoid blocking the caller.
//
// requestsPerSecond must be >= 1; values < 1 are treated as 1.
func RateLimit(requestsPerSecond int) Middleware {
	if requestsPerSecond < 1 {
		requestsPerSecond = 1
	}
	store := newBucketStore(requestsPerSecond)

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			bucket := store.get(toolName)
			if !bucket.allow() {
				return nil, fmt.Errorf("rate limit exceeded for tool %q: limit is %d req/s", toolName, requestsPerSecond)
			}
			return next(ctx, toolName, args)
		}
	}
}
