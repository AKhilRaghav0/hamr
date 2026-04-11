# Comparison

This page shows what mcpx actually saves you compared to building an MCP server by hand, and covers the cases where you might not need hamr.

## The same tool, two ways

Here is a single tool — `get_pods`, which wraps `kubectl get pods` — written both ways. This is a real example from the mcpx repo.

### With mcpx (15 lines)

```go
type GetPodsInput struct {
    Namespace string `json:"namespace" desc:"kubernetes namespace" default:"default"`
    Selector  string `json:"selector" desc:"label selector like app=nginx" optional:"true"`
}

func getPods(_ context.Context, in GetPodsInput) (string, error) {
    args := []string{"get", "pods", "-n", in.Namespace, "-o", "wide"}
    if in.Selector != "" {
        args = append(args, "-l", in.Selector)
    }
    return kubectl(args...)
}

// Registration:
s.Tool("get_pods", "List pods in a namespace (with optional label selector)", getPods)
```

That is it. No schema writing. No argument parsing. No required-field checking. No default injection. You add this tool to a server that already has `s.Run()` at the bottom of main.

### Without mcpx (80+ lines)

```go
// Step 1: Write the JSON Schema by hand.
var getPodsSchema = map[string]any{
    "type": "object",
    "properties": map[string]any{
        "namespace": map[string]any{
            "type":        "string",
            "description": "kubernetes namespace",
            "default":     "default",
        },
        "selector": map[string]any{
            "type":        "string",
            "description": "label selector like app=nginx",
        },
    },
    "required": []string{"namespace"},
}

// Step 2: Add it to the tools list.
var tools = []ToolDefinition{
    {
        Name:        "get_pods",
        Description: "List pods in a namespace (with optional label selector)",
        InputSchema: getPodsSchema,
    },
}

// Step 3: Add a case to the dispatch switch.
case "get_pods":
    var args struct {
        Namespace string `json:"namespace"`
        Selector  string `json:"selector"`
    }
    if err := json.Unmarshal(params.Arguments, &args); err != nil {
        return errorResponse(req.ID, "invalid arguments: "+err.Error())
    }
    // Apply defaults manually — the schema "default" doesn't do anything on its own.
    if args.Namespace == "" {
        args.Namespace = "default"
    }
    // Validate required fields manually.
    if args.Namespace == "" {
        return errorResponse(req.ID, "namespace is required")
    }
    // Your actual logic.
    cmdArgs := []string{"get", "pods", "-n", args.Namespace, "-o", "wide"}
    if args.Selector != "" {
        cmdArgs = append(cmdArgs, "-l", args.Selector)
    }
    result, callErr = runKubectl(cmdArgs...)

// Plus you also wrote the JSON-RPC request/response types,
// the stdio loop, the initialize handler, the tools/list handler,
// error helpers...
```

The raw version of this same 6-tool k8s server comes to ~480 lines and only implements 3 of the 6 tools. The hamr version is 157 lines with all 6 tools.

## Lines of code comparison

The raw k8s-mcp server (with 3 tools, no middleware, no testing) vs the hamr version (6 tools, middleware, full feature set):

| | Raw SDK | mcpx |
|--|---------|------|
| Tool count | 3 | 6 |
| Total lines | ~480 | ~157 |
| Lines per tool | ~80 | ~15 |
| Schema writing | Manual | Automatic |
| Validation | Manual per-tool | Automatic |
| Default injection | Manual per-tool | Automatic |
| Middleware | None | Built-in |
| Testing | None | hamrtest |
| Panic recovery | None | middleware.Recovery() |
| SSE transport | Not present | s.RunSSE() |
| Dashboard | Not present | s.RunSSEWithDashboard() |

## Feature matrix

| Feature | Raw SDK | mcpx |
|---------|---------|------|
| stdio transport | Write it yourself | `s.Run()` |
| SSE transport | Write it yourself | `s.RunSSE(":8080")` |
| JSON Schema generation | Write by hand per tool | Automatic from struct tags |
| Input validation | Write by hand per tool | Automatic |
| Default values | Write by hand per tool | `default:"value"` tag |
| Required field checks | Write by hand per tool | All fields required unless `optional:"true"` |
| Enum validation | Write by hand per tool | `enum:"a,b,c"` tag |
| Numeric bounds | Write by hand per tool | `min:"1" max:"100"` tags |
| Middleware system | Not present | `s.Use()` |
| Logger middleware | Not present | `middleware.Logger()` |
| Panic recovery | Not present | `middleware.Recovery()` |
| Rate limiting | Not present | `middleware.RateLimit(10)` |
| Timeout middleware | Not present | `middleware.Timeout(30*time.Second)` |
| Cache middleware | Not present | `middleware.Cache(5*time.Minute)` |
| Test client | Not present | `hamrtest.NewClient()` |
| TUI dashboard | Not present | `s.RunSSEWithDashboard()` |
| Pre-built tools | Not present | `toolbox.*` |
| Project scaffolding | Not present | `hamr init` |
| Live reload | Not present | `hamr dev` |
| Validation CLI | Not present | `hamr validate` |

## What mcpx actually eliminates

**Schema writing.** Every tool needs a JSON Schema so the AI knows what arguments to pass. Without mcpx, you write this by hand as a `map[string]any`. It is tedious and it can drift from your actual code — the schema says `required: ["namespace"]` but your code silently accepts an empty namespace and uses a default. With mcpx, the schema is generated from your struct. If the struct changes, the schema changes automatically.

**Validation.** After parsing arguments, you have to check that required fields are present, types match, enums are valid, numbers are in range. Without mcpx this is a manual `if` block in every case of your dispatch switch. With mcpx it is automatic from the schema.

**Default injection.** JSON Schema has a `default` keyword but it is purely descriptive — it does not cause defaults to be applied. Without mcpx you write `if args.Count == 0 { args.Count = 10 }` everywhere. With mcpx you add `default:"10"` to the struct tag.

**Transport.** The stdio read loop, response writing, concurrency-safe output, message parsing — about 100 lines of careful code. The SSE server with session management is another 150 lines. You get both with `s.Run()` and `s.RunSSE()`.

**Error handling.** Panics in tool handlers crash the entire server process without recovery middleware. Every error path needs to be wrapped in the right JSON-RPC error shape. mcpx handles all of this.

**Testing infrastructure.** Without mcpx you test your server by either spawning it as a subprocess or manually constructing JSON-RPC messages and parsing the responses. Neither is pleasant. `hamrtest` gives you a real test client that calls through your actual handler logic in-memory.

## When you might not need mcpx

**A single, very simple tool.** If your server does exactly one thing and has one or two arguments, writing it by hand might be shorter than adding a dependency. A 50-line server that echoes input probably does not benefit from a framework.

**Non-Go projects.** mcpx is Go-specific. If your team works in TypeScript, Python, or another language, use the appropriate SDK for that language. Anthropic has official TypeScript and Python SDKs.

**When you need to own the transport layer.** If you have unusual transport requirements — custom authentication at the HTTP layer, non-standard session management, integration with an existing server framework — you might find it easier to build on the raw transport types directly and only use mcpx for schema generation and validation.

**When you want to learn the protocol.** Building one MCP server by hand is educational. You will understand what mcpx does for you much better after you have done it the hard way once.

For anything beyond a throwaway prototype or a single-tool server, mcpx pays for itself immediately.
