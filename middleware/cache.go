package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// cacheEntry holds a cached result along with its expiry time.
type cacheEntry struct {
	value     any
	expiresAt time.Time
}

// cache is a mutex-protected map of cache entries with lazy expiry.
type cache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

func newCache(ttl time.Duration) *cache {
	return &cache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

// get returns the cached value for key if it exists and has not expired.
// Expired entries are deleted lazily on access.
func (c *cache) get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.entries, key)
		return nil, false
	}
	return entry.value, true
}

// set stores value under key with the configured TTL.
func (c *cache) set(key string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// cacheKey builds a deterministic cache key from a tool name and its
// arguments. The arguments map is serialised as JSON with keys sorted so
// that {"a":1,"b":2} and {"b":2,"a":1} produce the same key.
func cacheKey(toolName string, args map[string]any) (string, error) {
	// Sort the keys for a stable JSON representation.
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ordered := make([]any, 0, len(keys)*2)
	for _, k := range keys {
		ordered = append(ordered, k, args[k])
	}

	b, err := json.Marshal(ordered)
	if err != nil {
		return "", fmt.Errorf("cache: failed to marshal args: %w", err)
	}
	return toolName + ":" + string(b), nil
}

// Cache returns a Middleware that memoises successful tool responses in an
// in-memory cache for the given TTL. Responses are keyed by tool name and a
// stable JSON serialisation of the arguments map.
//
// Only successful (non-error) responses are cached. Error responses are
// always forwarded to the underlying handler on the next call.
//
// Expired entries are removed lazily when the key is next accessed; no
// background goroutine is started.
func Cache(ttl time.Duration) Middleware {
	c := newCache(ttl)

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
			key, err := cacheKey(toolName, args)
			if err != nil {
				// If we cannot derive a key, skip caching and call through.
				return next(ctx, toolName, args)
			}

			if cached, ok := c.get(key); ok {
				return cached, nil
			}

			result, err := next(ctx, toolName, args)
			if err != nil {
				return nil, err
			}

			c.set(key, result)
			return result, nil
		}
	}
}
