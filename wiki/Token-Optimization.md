# Token Optimization

Every time an AI interacts with your MCP server, it reads all your tool schemas. With 20 tools, that's 20 schemas on every conversation turn — real tokens, real cost. These features exist to reduce that overhead.

## Tool Groups

Instead of exposing all tools upfront, organize them into groups. The AI sees group names first and only loads the full schemas when it picks a group.

```go
s.ToolGroup("kubernetes", "Kubernetes cluster operations", func(g *hamr.Group) {
    g.Tool("get_pods", "List pods", getPods)
    g.Tool("get_logs", "Get pod logs", getLogs)
    g.Tool("describe", "Describe a resource", describe)
})

s.ToolGroup("database", "Database operations", func(g *hamr.Group) {
    g.Tool("query", "Run a SQL query", query)
    g.Tool("list_tables", "List tables", listTables)
})
```

The AI sees 2 entries in tools/list instead of 5. When it calls the "kubernetes" group, it gets back a text listing of the 3 k8s tools with their parameters. Then it calls the specific tool it needs by its plain name.

Tool names must be unique across all groups and standalone tools. You call grouped tools by their plain name — `get_pods`, not `kubernetes__get_pods`.

If you have a server with 20 tools and the AI only needs 3 of them in a given conversation, this can cut schema token overhead by 80%.

## Minimal Schemas

Full JSON Schema includes descriptions, defaults, enums, min/max, patterns — useful for validation, verbose for the wire. `WithMinimalSchemas` strips all that from the tools/list response and keeps only type information.

```go
s := hamr.New("server", "1.0.0", hamr.WithMinimalSchemas())
```

What the AI sees without this option:
```json
{"type":"object","properties":{"query":{"type":"string","description":"the search query to execute","default":"","enum":["web","images","news"]}}}
```

What the AI sees with this option:
```json
{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}
```

Validation still uses the full schema internally. The AI just gets a leaner version. Saves roughly 15-25% of schema tokens depending on how many tags you use.

## Response Truncation

Large tool responses eat tokens. A `git log` on a big repo or a database query with thousands of rows can blow up the context window.

```go
s.Use(middleware.MaxResponseTokens(4000))
```

If a text response would exceed ~4000 tokens (estimated at 4 chars per token), it gets truncated at the last newline boundary with a notice:

```
... [response truncated at ~4000 tokens. Use pagination or offset parameters to see more]
```

Set to 0 to disable truncation entirely. Only affects string responses — non-text content (images, binary) passes through unchanged.

## Cost Tracking

Know what each tool call costs in tokens:

```go
s.Use(middleware.CostTracker(func(stats middleware.CostStats) {
    fmt.Printf("%s: ~%d tokens (req: %d, resp: %d)\n",
        stats.ToolName, stats.TotalTokens,
        stats.RequestTokens, stats.ResponseTokens)
}))
```

`CostStats` fields:
- `ToolName` — which tool was called
- `RequestTokens` — estimated from the size of the arguments JSON
- `ResponseTokens` — estimated from the response size
- `TotalTokens` — request + response
- `Duration` — how long the call took

These are rough estimates (4 chars = 1 token), not exact BPE counts. Good for finding expensive tools and comparing before/after optimization.

## EstimateSchemaTokens

Check the token cost of your schemas at startup:

```go
for name, tokens := range s.EstimateSchemaTokens() {
    fmt.Printf("  %s: ~%d tokens\n", name, tokens)
}
```

Returns a map of tool name to estimated token count. Includes group selectors (as `group__name`) but not individual grouped tools (since those aren't sent in tools/list).

Run this during development to catch bloated schemas before deploying.

## Combining Features

These features stack. A typical production setup:

```go
s := hamr.New("server", "1.0.0",
    hamr.WithMinimalSchemas(),          // leaner schemas
)

s.Use(
    middleware.CostTracker(reportCost), // track everything
    middleware.MaxResponseTokens(4000), // cap responses
    middleware.Logger(),                // log calls
    middleware.Recovery(),              // catch panics
)

// Group related tools
s.ToolGroup("k8s", "Kubernetes ops", func(g *hamr.Group) {
    g.Tool("get_pods", "List pods", getPods)
    g.Tool("get_logs", "Get logs", getLogs)
})

// Standalone tools that are always visible
s.Tool("search", "Search docs", search)
```

The AI sees 2 entries (the k8s group + search) instead of 3. Schemas are minimal. Responses are capped. Every call is tracked. And the whole thing is 15 lines.
