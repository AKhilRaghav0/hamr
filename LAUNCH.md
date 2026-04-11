# hamr Launch

Repository: https://github.com/AKhilRaghav0/hamr

---

## Launch Checklist

- [ ] Tag a release on GitHub (v0.1.0 or similar)
- [ ] Publish Dev.to article (draft below)
- [ ] Submit to Hacker News (Show HN text below)
- [ ] r/golang post — already posted: https://www.reddit.com/r/golang/comments/1sib0rh/i_got_tired_of_writing_300_lines_of_jsonrpc/
- [ ] Post on Twitter/X (options below)
- [ ] Post on LinkedIn (draft below)
- [ ] Reply to any HN/Reddit comments within the first two hours
- [ ] Update pkg.go.dev description if needed
- [ ] Pin the Reddit thread in the repo Community section

---

## Dev.to Article

**Title:** I built a Go framework that makes MCP servers 60% cheaper to run

**Tags:** go, mcp, ai, opensource

---

If you've been building anything with AI assistants lately — Claude, Cursor, Windsurf — you've probably run into MCP. Model Context Protocol is the standard that lets AI assistants call external tools: query your database, read your filesystem, hit an API, whatever you need.

The developer experience for building an MCP server in Go is rough. The official SDK is a reference implementation. It's correct and complete, but it's low-level. You hand-write JSON schemas. You deal with JSON-RPC framing. You wire up transport yourself. By the time you've written the boilerplate for four basic tools, you're looking at 350+ lines before your actual logic starts.

But there's a second problem that doesn't get talked about as much: token cost.

### The problem with how MCP works today

Every time the AI starts a conversation turn, it reads the full schema for every tool you've registered. All of them, every turn, regardless of whether they're relevant. If you have a 20-tool server with detailed parameter descriptions, you're spending hundreds of tokens on schema overhead before the AI does anything useful. On a busy server — or a long conversation — that adds up fast.

I ran the numbers on a moderate server with 15 tools. The schema overhead alone was burning about 800 tokens per turn. At Claude Sonnet pricing, that's roughly $0.0024 per turn just to tell the AI what tools exist. Across thousands of conversations, it's not a rounding error anymore.

### What hamr does differently

hamr is a Go framework for MCP servers that tackles both problems. The boilerplate problem and the token cost problem.

For boilerplate: you write a Go struct for your input and a function for your logic. hamr generates the JSON schema from your struct tags, handles validation, manages the JSON-RPC framing, and wires up transport. Your struct tags are your schema.

Here's the same 4-tool notes server, both ways.

The hamr version (74 lines, including comments and blank lines):

```go
type SearchInput struct {
    Query      string `json:"query" desc:"the search query"`
    MaxResults int    `json:"max_results" desc:"max results" default:"10"`
    Format     string `json:"format" desc:"output format" enum:"json,text,markdown" default:"text"`
}

func Search(ctx context.Context, input SearchInput) (string, error) {
    // your logic here
    return "results for: " + input.Query, nil
}

func main() {
    s := hamr.New("demo-server", "1.0.0")
    s.Tool("search", "Search for information", Search)
    s.Tool("greet", "Greet a person by name", Greet)
    s.Tool("reverse", "Reverse a string", Reverse)
    s.Tool("word_count", "Count words in text", WordCount)
    log.Fatal(s.Run())
}
```

The equivalent with the official go-sdk requires you to define JSON schemas by hand, write separate handler functions with a more verbose signature, manually register each tool with its schema, and handle validation yourself. The same four tools runs to about 350 lines.

That's the boilerplate gap. But the token cost features are where hamr goes further than just ergonomics.

### Tool groups

Instead of exposing all tools upfront, you can organize them into named groups. The AI sees group names and descriptions first, then loads full schemas only for the groups it needs.

```go
s.Group("filesystem", "Read and write local files",
    hamr.GroupTool("read_file", "Read a file", ReadFile),
    hamr.GroupTool("write_file", "Write a file", WriteFile),
    hamr.GroupTool("list_dir", "List directory", ListDir),
)

s.Group("database", "Query the database",
    hamr.GroupTool("query", "Run a SELECT", QueryDB),
    hamr.GroupTool("list_tables", "List tables", ListTables),
)
```

The AI sees two tools instead of five. If the task doesn't touch the database, those schemas never load. On a 20-tool server, this can cut schema overhead by 80% or more depending on actual usage patterns.

### Minimal schemas

Full JSON Schema includes `$schema` declarations, `additionalProperties`, format annotations, and other fields that are useful for machine validation but redundant when you're briefing an AI. `WithMinimalSchemas()` strips those fields before anything is sent. One option flag, no code changes to your tools, typically saves 15-25% of schema tokens.

### Response truncation

Tool responses can be arbitrarily large. A `git log` over a big repo or a database query with thousands of rows can return tens of thousands of tokens. `MaxResponseTokens` caps any single tool response before it reaches the AI, with a note appended so the AI knows the output was truncated.

```go
s.Tool("git_log", "Show commit history", GitLog,
    hamr.MaxResponseTokens(500),
)
```

### Cost tracking

`CostTracker` records estimated token usage per tool call — schema tokens sent, response tokens returned, running totals. These are estimates based on character counts, not exact BPE counts. They're useful for relative comparisons: figuring out which tools are expensive relative to each other, not for billing.

```go
tracker := hamr.NewCostTracker()
s := hamr.New("my-server", "1.0.0", hamr.WithCostTracker(tracker))

// after some calls...
report := tracker.Report()
fmt.Printf("Total estimated tokens: %d\n", report.TotalTokens)
```

### The honest take

hamr is not a replacement for the official go-sdk in all cases. The official SDK will always be first to get new protocol features — output schemas, progress tracking, StreamableHTTP as it rolls out. It has first-party backing. If you need any of those features, or if you want to stay as close to the spec as possible, go-sdk is the right call.

hamr is for the case where you're building something real, you care about what the token overhead looks like at scale, and you want to write 70 lines instead of 350. The middleware, the built-in tool collections (filesystem, git, database, HTTP, shell), the TUI dashboard, the test client — all of that is there for building production-grade servers, not just demos.

The Kubernetes example in the repo is probably the clearest illustration. You point it at a cluster and Claude can answer "what pods are crashing right now?" with real data. That server is 157 lines.

If you're building MCP servers in Go and you want to try it:

```bash
go install github.com/AKhilRaghav0/hamr/cmd/hamr@latest
hamr init my-server
cd my-server && go run .
```

Repo: https://github.com/AKhilRaghav0/hamr

Happy to answer questions. The codebase is straightforward — no code generation, no magic, just reflection and struct tags.

---

## Hacker News Submission

**Title:** Show HN: hamr – Go framework for MCP servers, optimized for token cost

**URL:** https://github.com/AKhilRaghav0/hamr

**Body:**

Every MCP tool schema gets sent to the AI on every conversation turn — all of them, every time, whether they're relevant or not. On a server with 20+ tools this becomes meaningful token overhead fast.

hamr addresses this with tool groups (lazy schema loading), minimal schema mode, and per-tool response truncation. It also handles the usual boilerplate — schema generation from struct tags, middleware, transport — so the same 4-tool server that takes ~350 lines with the official go-sdk takes about 74 lines here.

---

## r/golang Post

Already posted: https://www.reddit.com/r/golang/comments/1sib0rh/i_got_tired_of_writing_300_lines_of_jsonrpc/

Reference this thread in other posts where relevant. It has organic engagement and anchors the community discussion.

---

## Twitter / X Posts

### Option 1 — Single post (under 280 chars)

Built a Go framework for MCP servers. Schema generation from struct tags, middleware, token cost tracking. Same 4-tool server: 74 lines instead of 350. github.com/AKhilRaghav0/hamr

---

### Option 2 — Thread

**Starter post:**

I built hamr, a Go framework for MCP servers. The pitch: the same server takes 74 lines instead of 350, and it won't burn hundreds of tokens on schema overhead every conversation turn.

Here's what that looks like in practice.

**Reply 1:**

With the official go-sdk you hand-write JSON schemas, deal with JSON-RPC framing, wire up transport, validate inputs. It's all there but it's low-level. hamr generates schemas from struct tags and gets out of your way.

```go
s := hamr.New("my-server", "1.0.0")
s.Tool("search", "Search for information", Search)
log.Fatal(s.Run())
```

That's a complete, working MCP server.

**Reply 2:**

The token cost angle is the part I haven't seen other frameworks tackle. Every tool schema gets sent to the AI every turn. On a 20-tool server that's hundreds of wasted tokens before the AI does anything.

Tool groups fix this: the AI sees group names, loads full schemas only when it needs them. Can cut schema overhead 80%+.

**Reply 3:**

Also ships with: middleware (logger, recovery, rate limit, auth, cache), pre-built tool collections (filesystem, git, database, shell), a live TUI dashboard, and a test client.

Repo: github.com/AKhilRaghav0/hamr — MIT, feedback welcome.

---

### Option 3 — Quote-tweet friendly (standalone, designed to be retweeted with commentary)

Hot take: the token cost of your MCP server's tool schemas matters more than people realize.

Every tool schema is sent to the AI every single turn. 20 tools x detailed schemas = hundreds of tokens of overhead, every time, before the AI does anything.

Built hamr to fix this: github.com/AKhilRaghav0/hamr

---

## LinkedIn Post

I shipped an open source Go framework for building MCP servers. MCP is the protocol that lets AI assistants like Claude and Cursor call external tools — your database, your Kubernetes cluster, your codebase.

The framework is called hamr. The short version: it cuts the boilerplate from ~350 lines to ~74 for a typical server, and it introduces features specifically for controlling token cost in production.

That second part is what I found missing from existing tools. Every MCP tool schema gets sent to the AI on every conversation turn, whether those tools are relevant or not. On a server with 20 or more tools, the schema overhead alone can run into hundreds of tokens per turn. Across thousands of user conversations, that is not a negligible cost.

hamr addresses this with tool groups (lazy schema loading — the AI sees group names first, loads full schemas only when needed), a minimal schema mode that strips fields redundant in an AI context, per-tool response truncation for tools that can return large outputs, and a cost tracker for understanding where your context budget actually goes.

On the production side: it ships with middleware (logger, recovery, rate limiting, auth, timeout, cache), OpenTelemetry-compatible hooks, pre-built tool collections for filesystem, git, database, and shell access, and a live terminal dashboard for monitoring tool calls in real time.

The codebase is straightforward Go — reflection and struct tags, no code generation. MIT license.

Repo: https://github.com/AKhilRaghav0/hamr

If you are building anything in the MCP ecosystem or thinking about it, I would be glad to hear what you are working on.
