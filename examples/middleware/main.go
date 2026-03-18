// Example middleware demonstrates all middleware features available in mcpx.
//
// It registers three tools and applies:
//   - Global middleware: Logger, Recovery, RateLimit, Timeout
//   - Per-tool middleware on one tool: Cache
//
// Run with: go run ./examples/middleware
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/AKhilRaghav0/hamr"
	"github.com/AKhilRaghav0/hamr/middleware"
)

// ---- input structs ----------------------------------------------------------

// LookupInput is the input for the lookup tool, which caches results.
type LookupInput struct {
	Key string `json:"key" desc:"cache key to look up" required:"true"`
}

// SlowInput is the input for the slow tool, which sleeps to demo timeout.
type SlowInput struct {
	Seconds int `json:"seconds" desc:"number of seconds to sleep" default:"15"`
}

// RiskyInput is the input for the risky tool, which panics to demo recovery.
type RiskyInput struct {
	Trigger bool `json:"trigger" desc:"set true to trigger a panic"`
}

// ---- handlers ---------------------------------------------------------------

// handleLookup simulates a cache-able data fetch. In a real server this would
// call a database or external API. The Cache middleware ensures that identical
// key lookups within the TTL window are served from memory.
func handleLookup(_ context.Context, in LookupInput) (string, error) {
	// Simulate latency of a real lookup.
	time.Sleep(50 * time.Millisecond)
	return fmt.Sprintf("value for key=%q fetched at %s", in.Key, time.Now().Format(time.RFC3339)), nil
}

// handleSlow sleeps for the requested duration. With a 10-second global Timeout
// it will be cancelled before finishing if seconds >= 10.
func handleSlow(ctx context.Context, in SlowInput) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(time.Duration(in.Seconds) * time.Second):
		return fmt.Sprintf("completed after %d second(s)", in.Seconds), nil
	}
}

// handleRisky panics when trigger=true. The global Recovery middleware catches
// the panic and converts it into an ordinary error, keeping the server alive.
func handleRisky(_ context.Context, in RiskyInput) (string, error) {
	if in.Trigger {
		panic("deliberate panic to demonstrate recovery middleware")
	}
	return "completed without panic", nil
}

// ---- main -------------------------------------------------------------------

func main() {
	s := mcpx.New("middleware-demo", "1.0.0")

	// ---- global middleware ----
	// Applied to every tool call, outermost-first.
	//   Logger   — structured log on every call
	//   Recovery — catches panics and converts them to errors
	//   RateLimit(5) — at most 5 calls per second per tool
	//   Timeout(10s) — cancels any tool that runs longer than 10 seconds
	s.Use(
		middleware.Logger(),
		middleware.Recovery(),
		middleware.RateLimit(5),
		middleware.Timeout(10*time.Second),
	)

	// lookup: cached for 1 minute using per-tool Cache middleware.
	// The Cache middleware runs inside the global chain so a cache hit avoids
	// the rate-limit token and any latency entirely.
	s.Tool(
		"lookup",
		"Look up a value by key. Results are cached for 1 minute.",
		handleLookup,
		middleware.Cache(1*time.Minute),
	)

	// slow: context-aware sleep that demonstrates the 10-second global Timeout.
	// Pass seconds >= 10 to observe the timeout error in the response.
	s.Tool(
		"slow",
		"Sleep for the specified number of seconds. The global 10-second timeout will interrupt long sleeps.",
		handleSlow,
	)

	// risky: deliberately panics when trigger=true, demonstrating Recovery.
	s.Tool(
		"risky",
		"Trigger a deliberate panic when trigger=true to demonstrate the Recovery middleware.",
		handleRisky,
	)

	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
