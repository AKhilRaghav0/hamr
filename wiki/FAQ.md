# FAQ

Common questions about mcpx and MCP in general.

---

**What is MCP?**

MCP (Model Context Protocol) is an open protocol for connecting AI models to external tools and data sources. When you build an MCP server, you are giving an AI model structured access to your systems — files, databases, APIs, shell commands, whatever you need. The AI client (like Claude Desktop) discovers what tools your server offers, and can call them during a conversation when it decides they are useful.

The protocol is based on JSON-RPC 2.0 and is transport-agnostic. mcpx handles all the protocol details so you just write Go functions.

---

**What clients support MCP?**

As of early 2026, the main ones are:

- **Claude Desktop** — Anthropic's desktop app, the most widely used MCP client
- **Cursor** — the AI code editor
- **Windsurf** — another AI code editor
- **Continue** — an open-source VS Code/JetBrains extension
- **MCP Inspector** — a debugging and testing tool from Anthropic

The MCP spec is open and the ecosystem is growing. Any client that implements the protocol can connect to any compliant server, including one built with mcpx.

---

**Can I use mcpx without Claude Desktop?**

Yes. Claude Desktop is just the most common client. You can connect mcpx servers to any MCP-compatible client. You can also test them with the [MCP Inspector](https://github.com/anthropics/mcp-inspector):

```bash
npx @anthropic-ai/mcp-inspector ./my-server
```

Or send raw JSON-RPC messages yourself:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./my-server
```

The `mcpxtest` package is designed for testing without any client at all — see the [Testing](Testing) page.

---

**How do I add environment variables to my server?**

For stdio transport, environment variables are passed by the client launcher. In Claude Desktop's config:

```json
{
  "mcpServers": {
    "my-server": {
      "command": "/path/to/my-server",
      "env": {
        "DATABASE_URL": "postgres://localhost/mydb",
        "API_KEY": "secret123"
      }
    }
  }
}
```

In your server code, read them normally:

```go
func main() {
    dbURL := os.Getenv("DATABASE_URL")
    if dbURL == "" {
        log.Fatal("DATABASE_URL is required")
    }

    db, err := sql.Open("postgres", dbURL)
    // ...

    s := mcpx.New("my-server", "1.0.0")
    s.AddTools(toolbox.Database(db))
    log.Fatal(s.Run())
}
```

For SSE transport, your server is a long-running process, so you set environment variables the normal way — in a `.env` file loaded at startup, via systemd unit `Environment=` directives, via your container's environment, etc.

---

**Can I register tools dynamically at runtime?**

Not after the server has started serving requests. `s.Tool()` acquires a write lock on the tools map, so technically you could call it from a goroutine, but this is not a supported pattern and the tool list sent to clients during the `initialize` handshake would be stale.

The right approach is to register all your tools before calling `s.Run()`. If you need dynamic behavior, build it into your tool logic:

```go
// Instead of dynamic tool registration, use a dispatcher tool
type PluginCallInput struct {
    Plugin string `json:"plugin" desc:"plugin name to invoke"`
    Args   string `json:"args" desc:"arguments for the plugin"`
}

func callPlugin(ctx context.Context, in PluginCallInput) (string, error) {
    p, ok := loadedPlugins[in.Plugin]
    if !ok {
        return "", fmt.Errorf("unknown plugin: %s", in.Plugin)
    }
    return p.Run(ctx, in.Args)
}
```

---

**Does mcpx support resources and prompts?**

Yes, though these features are less commonly used than tools. Register them with `s.Resource()` and `s.Prompt()`:

```go
// Register a resource (a piece of data the AI can read)
s.Resource(
    "file://config.yaml",
    "config",
    "The server's configuration",
    func(ctx context.Context) (string, error) {
        data, err := os.ReadFile("config.yaml")
        return string(data), err
    },
)

// Register a prompt (a reusable message template)
s.Prompt(
    "analyze_code",
    "Analyze a code snippet for issues",
    func(ctx context.Context, args map[string]string) (string, error) {
        code := args["code"]
        return fmt.Sprintf("Please analyze this code:\n\n%s", code), nil
    },
)
```

Resources and prompts are exposed through the standard MCP protocol methods (`resources/list`, `resources/read`, `prompts/list`, `prompts/get`). The `mcpxtest.Client` has `ListResources()` and `ListPrompts()` for testing them.

---

**How do I deploy an MCP server to production?**

It depends on the transport.

For **stdio transport**, there is nothing to deploy — the client launches your binary directly. You just need the binary to be available at the configured path on the machine where the client runs.

For **SSE transport**, you deploy a normal HTTP server:

```bash
# Build a binary
go build -o my-server .

# Run it (substitute your actual process manager)
./my-server   # or systemd, Docker, etc.
```

A minimal systemd unit:

```ini
[Unit]
Description=My MCP Server

[Service]
ExecStart=/usr/local/bin/my-server
Environment=DATABASE_URL=postgres://localhost/mydb
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

Or with Docker:

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o my-server .

FROM alpine:latest
COPY --from=builder /app/my-server /usr/local/bin/
EXPOSE 8080
CMD ["my-server"]
```

For production SSE servers you will want TLS. Put your server behind a reverse proxy (nginx, Caddy, Traefik) that handles TLS termination.

---

**What's the performance overhead of mcpx vs raw SDK?**

The overhead is the validation step plus two JSON marshal/unmarshal round-trips (map to struct). In practice this is microseconds for typical tool inputs.

Tool call latency is almost entirely dominated by whatever your handler actually does — a database query, an HTTP request, a shell command. The mcpx overhead is not measurable in real workloads.

If you have a tool that is called thousands of times per second with very large payloads, you might eventually see the validation overhead. At that point you are probably not building a typical MCP server and you should benchmark your specific case.

---

**Can I use mcpx with TypeScript/Python MCP clients?**

Yes. The MCP protocol is language-agnostic. An mcpx server speaks standard MCP over stdio or HTTP/SSE, and any client that speaks the same protocol can connect to it — regardless of what language the client is written in.

---

**How do I debug my MCP server?**

A few approaches, depending on the problem:

**Use mcpxtest for unit testing.** Write tests that call your tools directly. This is the fastest feedback loop and catches most issues before you ever connect a client. See the [Testing](Testing) page.

**Use the MCP Inspector.** It is a visual debugging UI that lets you call tools manually and see the raw JSON:

```bash
npx @anthropic-ai/mcp-inspector ./my-server
```

**Use the dashboard.** If you switch to SSE transport, `RunSSEWithDashboard` gives you a live view of every call, including errors and latency. Useful for understanding what Claude is actually calling.

**Add the logger middleware.** It writes structured logs to stderr for every tool call:

```go
s.Use(middleware.Logger())
```

**Check stderr.** For stdio transport, your server's stderr goes to Claude Desktop's logs. On macOS, they are at `~/Library/Logs/Claude/`. On Windows, check `%APPDATA%\Claude\logs\`.

**Send JSON directly.** For a quick smoke test without Claude:

```bash
# Initialize
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./my-server

# List tools
printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}\n{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}\n' | ./my-server
```

---

**Why Go and not TypeScript?**

A few reasons mcpx is Go-specific:

- Go compiles to a single static binary. Distributing an MCP server means giving someone a file, not a `node_modules` directory or a Python virtual environment.
- Go's type system and reflection let mcpx generate schemas and validate inputs from ordinary struct definitions, with no code generation step.
- Go starts fast. Claude Desktop launches your server as a subprocess on demand; a Go binary is up in milliseconds.
- The people who built mcpx write Go.

The TypeScript MCP SDK from Anthropic is excellent and there are good Python frameworks too. mcpx is for Go developers who want to stay in Go.
