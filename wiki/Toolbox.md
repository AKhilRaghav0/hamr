# Toolbox

The toolbox package ships pre-built tool collections so you don't have to write common tools from scratch. Each collection is a group of related tools that you register all at once using `s.AddTools()`.

```go
import "github.com/AKhilRaghav0/hamr/toolbox"

s.AddTools(toolbox.FileSystem("/projects/myapp"))
s.AddTools(toolbox.HTTP())
s.AddTools(toolbox.Shell("/projects/myapp", toolbox.WithAllowedCommands("go", "make")))
s.AddTools(toolbox.Git("/projects/myapp"))
s.AddTools(toolbox.Database(db))
```

You can mix toolbox collections with your own custom tools:

```go
s := hamr.New("dev-server", "1.0.0")
s.AddTools(toolbox.FileSystem("/workspace"))
s.AddTools(toolbox.Git("/workspace"))
s.Tool("deploy", "Deploy the app", Deploy)  // your own tool
```

## How AddTools works

`AddTools` takes anything that implements the `ToolCollection` interface:

```go
type ToolCollection interface {
    Tools() []ToolInfo
}
```

Each `ToolInfo` has a name, description, and handler. `AddTools` iterates the list and calls `s.Tool()` for each one. If you register the same tool name twice, it panics at startup — same as manually registering duplicates.

## FileSystem tools

Gives the AI sandboxed access to a directory. All paths are validated to stay inside the root you provide.

```go
s.AddTools(toolbox.FileSystem("/safe/workspace"))
```

Registers four tools:

| Tool | What it does |
|------|-------------|
| `read_file` | Read the contents of a file |
| `write_file` | Write text content to a file (creates directories if needed) |
| `list_dir` | List directory contents with type and size |
| `search_files` | Find files matching a glob pattern |

All paths are relative to the sandbox root. If the AI tries to pass `../../etc/passwd`, it gets an error. The sandbox check is not just string manipulation — it resolves symlinks and repeats the containment check on the real path, so a symlink inside the sandbox cannot point outside it.

```go
// The AI can call read_file with path "src/main.go"
// which resolves to /safe/workspace/src/main.go.
// Trying path "../../etc/passwd" returns:
// error: path "../../etc/passwd" escapes the sandbox root
```

Security measures applied in order:
1. Null-byte rejection — null bytes in paths are a classic injection vector
2. Lexical containment check — rejects `..` traversal without any I/O
3. Symlink resolution — follows all symlinks and re-checks containment

The root path itself is also resolved through symlinks when `FileSystem()` is called. On macOS where `/tmp` is a symlink to `/private/tmp`, this ensures the comparison works correctly.

## HTTP tools

Lets the AI make outbound HTTP requests.

```go
s.AddTools(toolbox.HTTP())

// With custom options:
s.AddTools(toolbox.HTTP(
    toolbox.WithTimeout(10 * time.Second),
    toolbox.WithMaxBodySize(512 * 1024), // 512 KiB
))
```

Registers three tools:

| Tool | What it does |
|------|-------------|
| `http_get` | GET a URL, optional custom headers |
| `http_post` | POST to a URL with a body and content type |
| `fetch_url` | GET a URL and return status code plus body |

Defaults: 30-second timeout, 1 MiB body size limit. Response bodies larger than the limit are silently truncated. This prevents the AI from accidentally pulling multi-megabyte responses into the context window.

```go
// Set a shorter timeout for fast endpoints
s.AddTools(toolbox.HTTP(
    toolbox.WithTimeout(5 * time.Second),
))

// Allow larger bodies for APIs that return bulk data
s.AddTools(toolbox.HTTP(
    toolbox.WithMaxBodySize(5 * 1024 * 1024), // 5 MiB
))
```

There is no domain allowlist built in. The AI can request any URL. If you need to restrict which hosts are reachable, add middleware or wrap the HTTP tools in a custom collection.

## Shell tools

Lets the AI run shell commands. This is powerful and needs to be configured carefully.

```go
// Unrestricted — any command can be run (use with caution)
s.AddTools(toolbox.Shell("/workspace"))

// Restricted to specific commands only
s.AddTools(toolbox.Shell("/workspace",
    toolbox.WithAllowedCommands("go", "make", "git", "grep"),
))

// Custom output limit
s.AddTools(toolbox.Shell("/workspace",
    toolbox.WithAllowedCommands("go", "make"),
    toolbox.WithMaxOutput(50 * 1024), // 50 KiB
))
```

Registers one tool:

| Tool | What it does |
|------|-------------|
| `run_command` | Run a command with arguments in the configured working directory |

The AI calls `run_command` with a `command` (the executable name) and optional `args` (a list of arguments). Commands always run in the working directory you specified. Output is combined stdout and stderr.

Several security checks are applied before execution:

- **Null-byte rejection** — null bytes in command names could bypass allowlist checks at the kernel level
- **Path separator rejection** — paths like `/bin/rm` cannot be used to bypass a name-based allowlist (e.g. when only `rm` is allowed)
- **Allowlist enforcement** — when `WithAllowedCommands` is set, any command not in the list is rejected with a clear error

Output is truncated at `maxOutput` bytes (default 10 KiB). The command has a 30-second execution timeout. Exit codes other than 0 are reported in the output rather than returned as errors, so the AI can see the failure message.

```go
// This is a reasonable setup for a Go development server
s.AddTools(toolbox.Shell("/myapp",
    toolbox.WithAllowedCommands("go", "make", "cat", "ls", "grep"),
    toolbox.WithMaxOutput(20 * 1024),
))
```

If you omit `WithAllowedCommands`, any command can be run. Only do this if you fully trust the AI and the environment it is running in.

## Git tools

Read-only access to a git repository. All operations shell out to the `git` binary.

```go
s.AddTools(toolbox.Git("/path/to/repo"))
```

Registers four tools:

| Tool | What it does |
|------|-------------|
| `git_status` | Working tree status (`git status`) |
| `git_diff` | Unstaged or staged changes (`git diff` or `git diff --staged`) |
| `git_log` | Commit history, configurable count |
| `git_blame` | Line-by-line authorship for a file |

All operations are read-only. There are no tools for commit, push, checkout, reset, or any write operation. This is intentional.

```go
// git_log shows the last 10 commits by default.
// The AI can pass a different count:
// {"count": 25}

// git_diff can show staged changes:
// {"staged": true}

// git_blame takes a path relative to the repo root:
// {"path": "src/handler.go"}
```

Each command has a 30-second timeout. If `git` is not installed or not in PATH, the tool returns an error.

## Database tools

Read-only SQL access through Go's `database/sql` interface. Works with any driver that implements it — PostgreSQL, MySQL, SQLite, etc.

```go
import (
    "database/sql"
    _ "github.com/lib/pq"  // or any other driver
)

db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
if err != nil {
    log.Fatal(err)
}
s.AddTools(toolbox.Database(db))
```

Registers three tools:

| Tool | What it does |
|------|-------------|
| `query` | Execute a SELECT query and return results as a table |
| `list_tables` | List all user tables in the database |
| `describe_table` | Show column names and types for a table |

The `query` tool only accepts SELECT statements (or CTEs starting with WITH). Any other statement type is rejected with an error. This is enforced by a prefix check on the SQL string. Note that this is not a full SQL parser — for production deployments, you should also ensure the database user only has read permissions (SELECT) at the database level. Defense in depth.

```go
// The AI can run:
// {"sql": "SELECT * FROM users WHERE created_at > NOW() - INTERVAL '7 days'"}
// {"sql": "WITH recent AS (SELECT ...) SELECT ..."}

// But not:
// {"sql": "DELETE FROM users WHERE ..."} → error: only SELECT statements permitted
// {"sql": "UPDATE users SET ..."} → error: only SELECT statements permitted
```

Results are formatted as a plain text table with a column header row and a row count at the bottom. The `list_tables` query works for PostgreSQL and SQLite automatically, falling back between the two if the primary query fails. MySQL uses a slightly different schema, so you may need to adjust — or just register a custom `list_tables` tool instead of using the toolbox one.

## Security considerations summary

| Collection | Key risk | Mitigation |
|------------|----------|------------|
| FileSystem | Path traversal, symlink escape | Sandbox with symlink resolution |
| HTTP | Unbounded response sizes, SSRF | Body size limit, timeout |
| Shell | Arbitrary code execution | Allowlist commands, working directory |
| Git | Low risk (read-only) | Read-only operations only |
| Database | Data exfiltration via SELECT | Read-only user at the DB level |

The toolbox provides reasonable defaults but it cannot make every decision for you. Think about what access is actually needed and configure accordingly.
