# CLI Reference

The `mcpx` CLI helps you scaffold projects, validate them, and run them in development mode with live reload.

Install it with:

```bash
go install github.com/AKhilRaghav0/hamr/cmd/hamr@latest
```

## hamr init

Scaffolds a new MCP server project.

```bash
hamr init <name>
hamr init <name> --description "A brief description of what this server does"
hamr init <name> --transport sse
```

**Flags:**

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--description` | `-d` | `"An MCP server built with mcpx"` | Short description of the server |
| `--transport` | `-t` | `"stdio"` | Transport type: `stdio` or `sse` |

**What it generates:**

```
my-server/
├── main.go           entry point
├── tools/
│   └── example.go    two example tools (echo and greet)
├── go.mod            module file with mcpx dependency
├── Makefile          build/run/dev targets
├── README.md         project documentation
├── claude.json       Claude Desktop config snippet
└── .gitignore        standard Go gitignore
```

The `main.go` calls `s.Run()` for stdio transport or `s.RunSSE(":8080")` for SSE, depending on the `--transport` flag.

The `claude.json` file contains a ready-to-paste snippet for Claude Desktop's `mcpServers` config:

```json
{
  "mcpServers": {
    "my-server": {
      "command": "my-server",
      "args": []
    }
  }
}
```

**After init:**

```bash
cd my-server
go mod tidy
go run .        # run it once
hamr dev        # run with live reload
```

`hamr init` refuses to overwrite an existing directory. If `my-server/` already exists, it exits with an error.

## hamr validate

Statically checks a project for mcpx compliance. Useful in CI pipelines.

```bash
hamr validate            # check current directory
hamr validate ./my-server  # check a specific directory
```

**What it checks:**

- `go.mod` exists and declares the `github.com/AKhilRaghav0/hamr` dependency
- At least one `.go` file is present
- At least one `hamr.New()` call is present
- At least one `.Tool()` call is present (warning if absent)
- `s.Run()` or `s.RunSSE()` is called somewhere
- Each `.Tool()` call that can be parsed statically has a non-empty name and description
- The `"context"` package is imported (warning if not)

**Output:**

```
Validating project at /path/to/my-server

  ✓  go.mod exists
  ✓  go.mod imports github.com/AKhilRaghav0/hamr
  ✓  found 3 .go source file(s)
  ✓  hamr.New() call found
  ✓  3 s.Tool() call(s) found
  ✓  Tool "greet" has valid name and description
  ✓  Tool "search" has valid name and description
  ✓  Tool "fetch" has valid name and description
  ✓  s.Run() or s.RunSSE() call found
  ✓  "context" package imported

Summary: 9 passed, 0 failed, 0 warnings

Project is compliant.
```

If any required checks fail, `hamr validate` exits with a non-zero exit code, which fails CI pipelines.

Warnings (like no tools registered) do not cause a non-zero exit but are shown distinctly.

The validator works by reading source files and running regex patterns — it is not a full Go parser. Tool calls that span multiple lines or use variable names for the name/description string will produce a warning rather than a pass or fail.

**In CI:**

```yaml
# GitHub Actions example
- name: Validate MCP server
  run: hamr validate ./my-server
```

## hamr dev

Builds and runs your server with live reload on file changes. This is the main development workflow.

```bash
hamr dev             # watch current directory
hamr dev ./my-server # watch a specific directory
```

**What it does:**

1. Runs `go build -o <tempdir>/mcpx-server .` in the project directory
2. Starts the compiled binary
3. Watches all `.go` files recursively for changes using `fsnotify`
4. When a change is detected, waits 300ms for the file system to settle (debounce), then kills the running process and repeats from step 1
5. Handles SIGINT (ctrl-c) gracefully — kills the server process and exits cleanly

The built binary goes in a temporary directory, not your project directory, so there is no `.gitignore` clutter.

**Output looks like:**

```
14:22:01 INF  starting dev mode — watching /path/to/my-server
14:22:01 INF  building...
14:22:02 INF  build succeeded in 1.2s
14:22:02 INF  server started (pid 12345)

# ... you edit a file ...

14:23:15 INF  change detected: handler.go
14:23:15 INF  stopping server (pid 12345)
14:23:15 INF  building...
14:23:16 INF  build succeeded in 0.9s
14:23:16 INF  server started (pid 12346)
```

If the build fails, the error is printed and the previous process stays down until the next successful build. This way you never accidentally connect Claude to a server that is half-way through a bad edit.

All output from `hamr dev` goes to stderr. The server's own stdout and stdin pass through directly — which is how the stdio transport communicates. This means you can connect Claude Desktop to a server running under `hamr dev`, and it works.

**Directories that are watched:** Everything under the project root, except `vendor/`, `.git/`, and `testdata/`.

When you are done, press ctrl-c. The `hamr dev` process sends SIGTERM to the server and gives it 2 seconds to exit. If it doesn't, SIGKILL is sent.

## hamr version

Prints the hamr version.

```bash
$ hamr version
mcpx v0.1.0
```
