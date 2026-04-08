# Testing

mcpx has a built-in test package that lets you write proper Go tests for your MCP server. No mocking frameworks, no subprocess management, no network. Tests talk to your server in-memory using the same request/response path that a real client uses.

## The two pieces

You need two things to write tests:

1. `s.NewTestHandler()` — gets a `transport.Handler` from your server
2. `mcpxtest.NewClient()` — wraps that handler in a test client

```go
import (
    "testing"

    "github.com/AKhilRaghav0/hamr"
    "github.com/AKhilRaghav0/hamr/mcpxtest"
)

func TestMyTool(t *testing.T) {
    s := mcpx.New("test-server", "1.0.0")
    s.Tool("greet", "Greet someone", Greet)

    client := mcpxtest.NewClient(t, s.NewTestHandler())

    result, err := client.CallTool("greet", map[string]any{
        "name": "Alice",
    })
    if err != nil {
        t.Fatal(err)
    }
    if result.Text() != "Hello, Alice!" {
        t.Errorf("got %q", result.Text())
    }
}
```

The client calls go directly through `mcpHandler.HandleRequest()` — the same path used by the real transports. Validation runs, middleware runs, defaults are applied. The only thing that doesn't run is the network.

## client.CallTool

`CallTool` sends a `tools/call` request and returns the result:

```go
result, err := client.CallTool("my_tool", map[string]any{
    "field": "value",
})
```

`err` is only non-nil if there was a protocol-level error (unknown tool, malformed arguments). Validation failures and tool errors come back as `result.IsError == true` with the error text in `result.Text()`.

```go
// Check if the call succeeded
if result.IsError {
    t.Errorf("tool returned error: %s", result.Text())
}

// Get the text content
text := result.Text()

// Access individual content blocks
for _, block := range result.Content {
    if block.Type == "text" {
        // block.Text has the content
    }
}
```

## client.ListTools

`ListTools` sends a `tools/list` request and returns the registered tools:

```go
tools := client.ListTools()
for _, tool := range tools {
    // tool.Name, tool.Description, tool.InputSchema
}
```

This is useful for verifying that your server exposes the tools it should.

## Helper assertions

The `mcpxtest` package provides two assertion helpers:

```go
// Assert a tool with this name exists
mcpxtest.AssertToolExists(t, client, "search")

// Assert exactly n tools are registered
mcpxtest.AssertToolCount(t, client, 3)
```

Both call `t.Error` rather than `t.Fatal`, so you get all failures in one test run instead of stopping at the first.

## Testing validation errors

Validation failures come back as tool results with `IsError == true`. They do not come back as `err` from `CallTool`. This is correct: validation failing means the MCP server responded, not that the protocol broke.

```go
func TestMissingRequiredField(t *testing.T) {
    client := mcpxtest.NewClient(t, newServer().NewTestHandler())

    // Omit a required field
    result, err := client.CallTool("search", map[string]any{})
    if err != nil {
        // This would mean a protocol error, not a validation error.
        t.Fatalf("unexpected protocol error: %v", err)
    }

    if !result.IsError {
        t.Error("expected validation error, got success")
    }

    if result.Text() == "" {
        t.Error("error message should not be empty")
    }
}

func TestEnumViolation(t *testing.T) {
    client := mcpxtest.NewClient(t, newServer().NewTestHandler())

    result, err := client.CallTool("search", map[string]any{
        "query":  "test",
        "format": "xml",  // not in enum: json, text, markdown
    })
    if err != nil {
        t.Fatalf("unexpected protocol error: %v", err)
    }
    if !result.IsError {
        t.Error("expected enum validation error")
    }
}

func TestWrongType(t *testing.T) {
    client := mcpxtest.NewClient(t, newServer().NewTestHandler())

    result, err := client.CallTool("search", map[string]any{
        "query":       42,    // should be string
        "max_results": "ten", // should be integer
    })
    if err != nil {
        t.Fatalf("unexpected protocol error: %v", err)
    }
    if !result.IsError {
        t.Error("expected type validation error")
    }
}
```

## Testing defaults

mcpx applies default values before calling your handler. You can test this by omitting optional fields with defaults and checking the output reflects the default:

```go
func TestDefaultsApplied(t *testing.T) {
    client := mcpxtest.NewClient(t, newServer().NewTestHandler())

    // Only pass required field; max_results should default to 10
    result, err := client.CallTool("search", map[string]any{
        "query": "test",
    })
    if err != nil || result.IsError {
        t.Fatalf("unexpected error: %v / %s", err, result.Text())
    }

    // Your handler should have seen max_results = 10
    if !strings.Contains(result.Text(), "max=10") {
        t.Errorf("default not applied, got: %s", result.Text())
    }
}
```

## Testing with middleware

Middleware runs normally in tests. If you have global middleware on your server, it runs on every test call. This is usually what you want.

```go
func newServer() *mcpx.Server {
    s := mcpx.New("test-server", "1.0.0")
    s.Use(middleware.Recovery())  // runs in tests too
    s.Tool("greet", "Greet someone", Greet)
    return s
}
```

If you want to test your server without specific middleware, create a separate server instance without it:

```go
func newServerNoMiddleware() *mcpx.Server {
    s := mcpx.New("test-server", "1.0.0")
    s.Tool("greet", "Greet someone", Greet)
    return s
}
```

## Structuring your tests

The cleanest approach is a server factory function that each test (or test file) calls to get a fresh, isolated server:

```go
// server_test.go

package main

import (
    "context"
    "strings"
    "testing"

    "github.com/AKhilRaghav0/hamr"
    "github.com/AKhilRaghav0/hamr/mcpxtest"
)

type SearchInput struct {
    Query      string `json:"query" desc:"search query"`
    MaxResults int    `json:"max_results" default:"10"`
}

func handleSearch(_ context.Context, in SearchInput) (string, error) {
    return fmt.Sprintf("results for %q (max=%d)", in.Query, in.MaxResults), nil
}

func newServer() *mcpx.Server {
    s := mcpx.New("my-server", "1.0.0")
    s.Tool("search", "Search for things", handleSearch)
    return s
}

func TestSearch(t *testing.T) {
    client := mcpxtest.NewClient(t, newServer().NewTestHandler())

    result, err := client.CallTool("search", map[string]any{
        "query": "golang",
    })
    if err != nil || result.IsError {
        t.Fatalf("unexpected error: %v / %s", err, result.Text())
    }
    if !strings.Contains(result.Text(), "golang") {
        t.Errorf("query not reflected in result: %s", result.Text())
    }
}

func TestSearchDefault(t *testing.T) {
    client := mcpxtest.NewClient(t, newServer().NewTestHandler())

    result, err := client.CallTool("search", map[string]any{
        "query": "golang",
    })
    if err != nil || result.IsError {
        t.Fatalf("unexpected error: %v / %s", err, result.Text())
    }
    // Default max_results should be 10
    if !strings.Contains(result.Text(), "max=10") {
        t.Errorf("default not applied: %s", result.Text())
    }
}

func TestSearchMissingQuery(t *testing.T) {
    client := mcpxtest.NewClient(t, newServer().NewTestHandler())

    result, err := client.CallTool("search", map[string]any{})
    if err != nil {
        t.Fatalf("protocol error: %v", err)
    }
    if !result.IsError {
        t.Error("expected validation error for missing query")
    }
}

func TestToolsRegistered(t *testing.T) {
    client := mcpxtest.NewClient(t, newServer().NewTestHandler())
    mcpxtest.AssertToolExists(t, client, "search")
    mcpxtest.AssertToolCount(t, client, 1)
}

func TestInitialize(t *testing.T) {
    client := mcpxtest.NewClient(t, newServer().NewTestHandler())
    info := client.Initialize()

    serverInfo, ok := info["serverInfo"].(map[string]any)
    if !ok {
        t.Fatal("serverInfo missing")
    }
    if serverInfo["name"] != "my-server" {
        t.Errorf("server name = %v", serverInfo["name"])
    }
}
```

Run tests normally:

```bash
go test ./...
go test -v ./...
go test -run TestSearch ./...
```

Because everything runs in-memory, the tests are fast. No subprocess startup time, no port binding, no race conditions from parallel test processes.
