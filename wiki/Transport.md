# Transport

An MCP server needs a way to send and receive messages. mcpx gives you two transport options: stdio and SSE. Which one you use depends on who is connecting to your server.

## stdio transport

stdio is the default. When you call `s.Run()`, your server reads JSON-RPC messages from stdin and writes responses to stdout. Line by line, one message per line.

```go
func main() {
    s := hamr.New("my-server", "1.0.0")
    s.Tool("greet", "Greet someone", Greet)
    log.Fatal(s.Run())
}
```

This is the right choice when your client is Claude Desktop, Cursor, Windsurf, or any other desktop MCP client. These tools launch your binary as a subprocess and communicate with it over pipes. The MCP protocol was designed for this model.

There is one important constraint with stdio: your server must not write anything to stdout except JSON-RPC responses. No `fmt.Println`, no log output to stdout. If you do, you will corrupt the message stream and confuse the client. Use `os.Stderr` for logging, or let mcpx's built-in logger handle it (it writes to stderr by default).

## SSE transport

SSE (Server-Sent Events) runs your server as an HTTP server. Clients connect via HTTP rather than pipes. Use `s.RunSSE(":8080")` to start it:

```go
func main() {
    s := hamr.New("my-server", "1.0.0")
    s.Tool("greet", "Greet someone", Greet)
    log.Fatal(s.RunSSE(":8080"))
}
```

The server listens on two endpoints:

- `GET /sse` — the client connects here and holds the connection open to receive server responses as Server-Sent Events
- `POST /message?sessionId=<id>` — the client sends requests here; the `sessionId` is given to the client in the initial endpoint event

This is the right choice when:

- Your client is a web app connecting over HTTP
- You need remote access (the server is on a different machine)
- You want multiple clients to connect to the same server
- You want to run the server as a persistent process instead of a subprocess

For Claude Desktop, stdio is usually simpler. But if you are building something like a shared team MCP server deployed to a VM, SSE is the way to go.

## `s.Run()` vs `s.RunSSE()` vs `s.RunSSEWithDashboard()`

There are three ways to start a server:

```go
// stdio — for Claude Desktop, Cursor, etc.
log.Fatal(s.Run())

// SSE — for HTTP clients, remote access, web apps
log.Fatal(s.RunSSE(":8080"))

// SSE + TUI dashboard — same as above but with a live monitoring UI in the terminal
log.Fatal(s.RunSSEWithDashboard(":8080"))
```

All three block until the server exits. All three handle graceful shutdown on SIGINT and SIGTERM.

## The TUI dashboard

`RunSSEWithDashboard` starts the SSE server and opens a terminal UI in the same process. The dashboard updates live as tool calls come in.

```go
func main() {
    s := hamr.New("my-server", "1.0.0")
    s.Tool("search", "Search for things", Search)
    s.Tool("fetch", "Fetch a URL", Fetch)

    log.Fatal(s.RunSSEWithDashboard(":8080"))
}
```

What the dashboard shows:

- Server name, version, transport, and uptime
- Total requests and error count since startup
- A scrollable log of recent tool calls — tool name, argument summary, duration, success or error
- A tools table — each registered tool with call count, average latency, and error count

Keyboard controls:

- `tab` or `shift+tab` — switch focus between the calls pane and the tools pane
- `up`/`down` or `j`/`k` — scroll whichever pane is focused
- `q` or `ctrl+c` — quit (also stops the SSE server)

The dashboard only works with SSE mode. The reason is simple: stdio uses stdin and stdout for the MCP protocol. There is no way to also drive a terminal UI from the same process without corrupting those streams. SSE uses HTTP, so stdin and stdout are free for the TUI.

Here is what the dashboard looks like in your terminal:

```
╭─────────────────────────────────────────────────────────────╮
│ mcpx v1.0.0 · my-server                               sse   │
│ ● Running  Uptime: 4m32s  Tools: 3  Requests: 47  Errors: 1 │
│── Recent Calls ──────────────────────────────────────────────│
│  TIME      TOOL      ARGS              DURATION  STATUS      │
│  14:22:01  search    "kubernetes"        143ms   ok          │
│  14:22:08  fetch     "https://..."        89ms   ok          │
│  14:22:15  search    "golang error"      201ms   error       │
│── Tools ─────────────────────────────────────────────────────│
│  TOOL        DESCRIPTION          CALLS  AVG    ERRORS       │
│  search      Search for things       31  167ms       1       │
│  fetch       Fetch a URL             12   94ms       0       │
│  calculate   Evaluate an expr         4  312ms       0       │
│ q quit  tab switch pane  ↑↓/jk scroll                       │
╰─────────────────────────────────────────────────────────────╯
```

## Graceful shutdown

All three run methods set up signal handling for SIGINT (ctrl-c) and SIGTERM. When a signal arrives:

- For stdio, the transport stops reading and `Run()` returns
- For SSE, the HTTP server is given a 5-second grace period to finish in-flight requests before shutting down
- For the dashboard, the TUI exits first and then stops the SSE server

This means your server can be included in a `systemd` unit or a Docker container and will shut down cleanly when asked.

If you need to do your own cleanup on shutdown, use a context that gets cancelled when the server exits:

```go
func main() {
    s := hamr.New("my-server", "1.0.0")
    s.Tool("query", "Query the database", Query)

    // s.Run() returns when the process receives SIGINT or SIGTERM.
    // Do any cleanup after it returns.
    if err := s.Run(); err != nil {
        log.Printf("server stopped: %v", err)
    }

    // Clean up resources here.
    db.Close()
}
```
