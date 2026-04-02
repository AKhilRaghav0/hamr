# mcpx — Project Plan

> **One-liner:** Cobra for MCP servers. Turn 200 lines of boilerplate into 20.
> **License:** MIT
> **Go version:** 1.22+ (generics required)

## Architecture

```
mcpx (public API)
├── core/         — Server, Tool, Resource, Prompt registration
├── schema/       — Auto JSON Schema generation from Go types via generics
├── middleware/    — Auth, rate limiting, logging, recovery
├── transport/    — stdio, SSE (wraps official SDK)
├── validate/     — Input validation engine
├── tui/          — Bubbletea-based dashboard
├── cli/          — mcpx init scaffolder + mcpx validate
├── toolbox/      — Pre-built tool collections
└── testing/      — Mock MCP client for unit tests
```

## Priority Order

- Week 1: schema/ → validate/ → mcpx.go (core API) → transport/stdio
- Week 2: middleware/ → transport/sse → cmd/mcpx init → cmd/mcpx validate
- Week 3: tui/ → toolbox/ → mcpxtest/
- Week 4: examples/ → README → docs → LAUNCH
