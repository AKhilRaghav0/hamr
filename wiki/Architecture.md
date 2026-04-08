# Architecture

This page is for people who want to understand how mcpx works internally — either to contribute, or because knowing how the machine works helps you use it better.

## High-level overview

```
MCP Client (Claude Desktop, Cursor, web app, ...)
      |
      | JSON-RPC 2.0
      |
 ┌────▼──────────────────────────────────────────┐
 │  transport/                                    │
 │  ┌──────────────┐    ┌──────────────────────┐  │
 │  │ StdioTransport│    │     SSETransport      │  │
 │  │ reads stdin  │    │ /sse  + /message HTTP │  │
 │  └──────┬───────┘    └──────────┬───────────┘  │
 └─────────┼────────────────────────┼──────────────┘
           │                        │
           └───────────┬────────────┘
                       │ transport.Handler interface
                       ▼
              ┌────────────────┐
              │  handler.go    │
              │  mcpHandler    │
              │  ─────────     │
              │  initialize    │
              │  tools/list    │
              │  tools/call ───┼──────────────────────┐
              │  prompts/...   │                      │
              │  resources/... │                      ▼
              └────────────────┘           ┌──────────────────┐
                                           │  mcpx.go         │
                                           │  invokeTool()    │
                                           │  ─────────────   │
                                           │  1. parse JSON   │
                                           │  2. apply defaults│
                                           │  3. validate      │
                                           │  4. build chain   │
                                           │  5. call handler  │
                                           └──────────────────┘
                                                    │
                                          ┌─────────┼──────────┐
                                          │         │          │
                                          ▼         ▼          ▼
                                       global   per-tool   your func
                                      middleware middleware
```

## How a request flows

Take a `tools/call` request. Here is the path from wire to your function and back:

**1. Transport receives a line**

`StdioTransport.Run()` reads lines from stdin with a buffered scanner. Each line is expected to be a complete JSON-RPC message. The SSE transport does the same thing but receives messages via HTTP POST.

**2. JSON-RPC dispatch**

The raw JSON is decoded into a `transport.JSONRPCRequest`. If there is an `id` field it is a request (expects a response). If not, it is a notification (no response needed). Requests are passed to `handler.HandleRequest()`.

**3. Method routing in handler.go**

`mcpHandler.HandleRequest()` is a switch on `req.Method`:
- `initialize` — returns protocol version and capabilities
- `tools/list` — iterates `s.tools` and formats the response
- `tools/call` — extracts tool name and arguments, calls `s.invokeTool()`
- Anything else — returns a JSON-RPC method-not-found error

**4. invokeTool in mcpx.go**

This is where the real work happens:

```
rawArgs (json.RawMessage)
    → json.Unmarshal to map[string]any
    → validate.ApplyDefaults()       ← fills in default values
    → validate.Validate()            ← checks types, required, enum, min/max, pattern
    → build middleware chain
    → call through chain
    → callToolHandler()              ← reflection invocation
    → fire onCall callback (for dashboard)
    → convert result to Result type
```

**5. Middleware chain**

Global middleware wraps per-tool middleware wraps the actual handler. They are applied in reverse registration order so the first-registered middleware is outermost:

```
global[0] → global[1] → ... → tool[0] → tool[1] → ... → callToolHandler
```

Each middleware is `func(next HandlerFunc) HandlerFunc`, the standard onion pattern.

**6. callToolHandler — reflection invocation**

This is where your Go function is called. The process:

1. Marshal the `map[string]any` args back to JSON
2. `reflect.New(td.inputType)` — allocate a new value of your input struct type
3. `json.Unmarshal` into that value — this uses Go's standard JSON decoder with your struct field tags
4. `td.handlerVal.Call([]reflect.Value{ctx, input})` — invoke your function via reflection
5. Extract the return values — if the error return is non-nil, wrap it in `ToolError`; otherwise return the first value

The double marshal/unmarshal (JSON → map → JSON → struct) is deliberate. The validator works on `map[string]any` because it needs to check fields before they are decoded into Go types. The struct deserialization happens after validation passes.

**7. Response**

The result travels back up: `callToolHandler` returns to `invokeTool`, which converts it to a `Result`, which `handleCallTool` formats into the MCP content response shape, which the transport serializes to JSON and writes to the client.

## Package responsibilities

| Package | What it does |
|---------|--------------|
| `mcpx` (root) | Server struct, tool registration, `invokeTool`, public API |
| `transport/` | StdioTransport, SSETransport, JSON-RPC types |
| `handler.go` | `mcpHandler` — bridges transport to server |
| `schema/` | JSON Schema generation via reflection |
| `validate/` | Validation against JSON Schema, default injection |
| `middleware/` | Middleware type, built-in middleware implementations |
| `tui/` | Bubbletea TUI dashboard model |
| `toolbox/` | Pre-built tool collections |
| `mcpxtest/` | Test client and assertion helpers |
| `cmd/mcpx/` | CLI (init, validate, dev, version) |

## The reflect-based handler invocation system

Tool handlers are stored as `reflect.Value` in `toolDef.handlerVal`. At registration time (`buildToolDef`), mcpx uses reflection to validate the handler signature:

- Exactly 2 input parameters
- First parameter implements `context.Context`
- Second parameter is a struct (or pointer to struct)
- Exactly 2 return values
- Second return implements `error`
- First return is `string`, `[]Content`, or `Result`

If any of these checks fail, `Tool()` panics immediately with a descriptive message. This is intentional — see the design decisions section below.

At call time, `callToolHandler` uses `reflect.New` to allocate the input struct type and `td.handlerVal.Call()` to invoke the function. The input type is stored as `td.inputType` so the struct can be re-allocated on every call without re-inspecting the function signature.

## How schema generation works

`schema.GenerateFromType()` takes a `reflect.Type` and walks it recursively:

- Primitive types (`string`, `int`, `bool`, etc.) map directly to JSON Schema scalar types
- Slices become `{"type": "array", "items": <element schema>}`
- `map[string]V` becomes `{"type": "object", "additionalProperties": <V schema>}`
- Pointers are unwrapped transparently
- `time.Time` is special-cased to `{"type": "string", "format": "date-time"}`
- Structs are the main case: each exported field becomes a property, struct tags set description/default/enum/min/max/pattern

For structs, the function iterates exported fields. It reads the `json` tag for the field name (or falls back to the Go field name). It reads `optional:"true"` to decide whether to include the field in the `required` array. Everything else goes into the property schema via struct tags.

Circular type references (e.g. a struct with a field of the same type) are detected with a `seen map[reflect.Type]bool` that is threaded through the recursive calls. A repeated type emits `{"type": "object"}` and stops recursing.

## Design decisions

**Why panics at registration time?**

When `s.Tool()` gets a handler with the wrong signature, it panics immediately. The alternative would be to return an error or fail later when the tool is called. The panic is deliberate for two reasons:

1. A wrong handler signature is always a programming mistake, not a runtime condition. It should be caught during development, not in production when Claude tries to call the tool.
2. Panics at server startup are easy to catch and fix. An error buried in a call trace at 3am is not.

The panic message is specific: it tells you exactly which tool has the problem and what is wrong with the signature.

**Why `map[string]any` for schemas?**

JSON Schema could be represented as a typed struct hierarchy. mcpx uses `map[string]any` instead. The reasons:

- The MCP protocol sends schemas as JSON objects, so they need to be JSON-serializable anyway
- The validator (`validate.Validate`) receives the same `map[string]any` that the schema generator produces, and JSON parsing also produces `map[string]any`, so validation is straightforward structural traversal
- Adding new schema keywords (new tags) does not require changing a struct definition

The trade-off is less type safety in the schema layer, but since schemas are generated (not hand-written), the inconsistencies that type safety would catch mostly don't arise.

**Why double marshal/unmarshal?**

The invocation flow goes: JSON string → `map[string]any` → validate → JSON string → Go struct → handler. The extra round-trip through JSON exists because the validator needs to see the data as a `map[string]any` (the natural shape from JSON parsing), but your handler needs a typed struct. There is no clean way to do both in one pass without a custom decoder. The overhead is negligible in practice.

**Why no goroutine per call?**

The stdio transport processes requests sequentially (one at a time). This is fine for most uses — Claude Desktop sends one tool call, waits for the response, then sends the next one. The SSE transport handles HTTP requests concurrently (each request in its own goroutine, handled by `net/http`). The `Server.tools` map is protected by a `sync.RWMutex` to allow concurrent reads.
