# Middleware

Middleware wraps your tool handlers. It runs before and after each tool call, letting you add cross-cutting behavior like logging, error recovery, rate limiting, and caching without touching your tool logic.

If you've used middleware in Express, Gin, Echo, or any HTTP framework, this is the same concept.

## How it works

A middleware is a function that takes the next handler and returns a new handler:

```go
type Middleware func(next HandlerFunc) HandlerFunc
type HandlerFunc func(ctx context.Context, toolName string, args map[string]any) (any, error)
```

Middleware executes in the order you register it. If you register A, B, C, the execution flows like this:

```
A (before) -> B (before) -> C (before) -> your handler -> C (after) -> B (after) -> A (after)
```

This is the standard onion model.

## Using middleware

### Global middleware

Applied to every tool:

```go
s := mcpx.New("server", "1.0.0")
s.Use(
    middleware.Logger(),
    middleware.Recovery(),
    middleware.RateLimit(10),
)
```

### Per-tool middleware

Applied only to a specific tool, runs inside the global middleware:

```go
s.Tool("search", "Search", Search,
    middleware.Cache(5 * time.Minute),
)
```

## Built-in middleware

### Logger

Logs every tool call with structured output — tool name, duration, success or error.

```go
s.Use(middleware.Logger())
```

Options:

```go
middleware.Logger(
    middleware.WithLogLevel(slog.LevelDebug),    // change log level
    middleware.WithCustomLogger(myLogger),        // use your own slog.Logger
)
```

### Recovery

Catches panics in your handlers and converts them to errors instead of crashing the server. You should almost always use this.

```go
s.Use(middleware.Recovery())
```

Without recovery, if a tool handler panics, your entire MCP server process dies. With recovery, the AI gets an error message and the server keeps running.

### RateLimit

Limits how many times a tool can be called per second. Uses a token bucket algorithm, per-tool.

```go
s.Use(middleware.RateLimit(10))  // 10 calls per second per tool
```

This is useful when your tools call external APIs with rate limits.

### Timeout

Kills tool calls that take too long. The context passed to your handler is cancelled after the deadline.

```go
s.Use(middleware.Timeout(30 * time.Second))
```

Your handler should respect context cancellation:

```go
func SlowTool(ctx context.Context, in Input) (string, error) {
    select {
    case result := <-doWork():
        return result, nil
    case <-ctx.Done():
        return "", ctx.Err()
    }
}
```

### Cache

Caches successful tool responses for a given TTL. Same tool name + same arguments = cached response. Failed calls are never cached.

```go
s.Tool("lookup", "External API lookup", handler,
    middleware.Cache(5 * time.Minute),
)
```

### Auth

Validates an auth token from the context before allowing the tool call.

```go
s.Use(middleware.Auth(func(ctx context.Context, token string) (context.Context, error) {
    if token != "valid-token" {
        return ctx, fmt.Errorf("invalid token")
    }
    // Optionally enrich the context with user info
    return context.WithValue(ctx, "user", "alice"), nil
}))
```

## Writing your own middleware

A middleware is just a function. Here's a simple one that adds a request ID to the context:

```go
func RequestID() middleware.Middleware {
    return func(next middleware.HandlerFunc) middleware.HandlerFunc {
        return func(ctx context.Context, toolName string, args map[string]any) (any, error) {
            // Before
            id := uuid.New().String()
            ctx = context.WithValue(ctx, "request_id", id)

            // Call the next handler
            result, err := next(ctx, toolName, args)

            // After (if you need it)
            return result, err
        }
    }
}
```

Register it like any other middleware:

```go
s.Use(RequestID())
```
