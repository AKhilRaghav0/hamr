# Getting Started

This page gets you from zero to a working MCP server connected to Claude Desktop. It should take about 5 minutes.

## Prerequisites

You need Go 1.22 or later. Check with:

```bash
go version
```

You also need an MCP client. Claude Desktop is the most common one, but Cursor, Windsurf, and other tools also support MCP. This guide uses Claude Desktop for the examples.

## Option 1: Scaffold a new project (recommended)

Install the mcpx CLI:

```bash
go install github.com/AKhilRaghav0/hamr/cmd/hamr@latest
```

Create a new project:

```bash
hamr init my-server
cd my-server
```

This creates a complete, compilable project with an example tool, a Makefile, and a Claude Desktop config you can copy-paste. Take a look at the generated `main.go` and `tools/example.go` to see how it's structured.

Build and run it:

```bash
go build -o my-server .
```

Now skip to the "Connect to Claude Desktop" section below.

## Option 2: Add to an existing Go project

Add the dependency:

```bash
go get github.com/AKhilRaghav0/hamr
```

Create a minimal server:

```go
package main

import (
    "context"
    "log"

    "github.com/AKhilRaghav0/hamr"
)

type GreetInput struct {
    Name string `json:"name" desc:"person to greet"`
}

func Greet(ctx context.Context, input GreetInput) (string, error) {
    return "Hello, " + input.Name + "!", nil
}

func main() {
    s := mcpx.New("my-server", "1.0.0")
    s.Tool("greet", "Greet someone by name", Greet)
    log.Fatal(s.Run())
}
```

Build it:

```bash
go build -o my-server .
```

## Connect to Claude Desktop

Open your Claude Desktop config file:

- **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

Add your server to the `mcpServers` section:

```json
{
  "mcpServers": {
    "my-server": {
      "command": "/absolute/path/to/my-server"
    }
  }
}
```

Use the absolute path to your built binary. Relative paths won't work.

Restart Claude Desktop. You should see your server listed in the available tools. Try asking Claude something like "Can you greet Alice?" and it will call your tool.

## What just happened?

When Claude Desktop starts, it launches your binary as a subprocess. Your server communicates with Claude over stdin/stdout using the MCP protocol (JSON-RPC 2.0). When Claude decides it needs to call one of your tools, it sends a `tools/call` request with the arguments, your handler runs, and the result goes back to Claude.

mcpx handles all the protocol details. You just write the struct (which becomes the schema Claude sees) and the function (which runs when Claude calls the tool).

## Next steps

- Read [Tools](Tools) to understand how tool definitions work in depth
- Read [Middleware](Middleware) to add logging, error recovery, and rate limiting
- Look at the [examples](https://github.com/AKhilRaghav0/hamr/tree/main/examples) directory for real-world servers

## Quick test without Claude Desktop

If you want to test your server without setting up Claude Desktop, you can send JSON-RPC messages directly:

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}' | ./my-server
```

You should get back a JSON response with your server info. You can also use the [MCP Inspector](https://github.com/anthropics/mcp-inspector) for a visual testing UI:

```bash
npx @anthropic-ai/mcp-inspector ./my-server
```
