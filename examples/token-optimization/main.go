// Example showing hamr's token optimization features.
// These features reduce the token cost of running MCP servers.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/AKhilRaghav0/hamr"
	"github.com/AKhilRaghav0/hamr/middleware"
)

// --- Tool inputs ---

type GetPodsInput struct {
	Namespace string `json:"namespace" desc:"kubernetes namespace" default:"default"`
}

type GetLogsInput struct {
	Pod   string `json:"pod" desc:"pod name"`
	Lines int    `json:"lines" desc:"number of log lines" default:"50"`
}

type QueryInput struct {
	SQL string `json:"sql" desc:"SQL query to run"`
}

type ListTablesInput struct{}

type ReadFileInput struct {
	Path string `json:"path" desc:"file path to read"`
}

// --- Handlers ---

func getPods(_ context.Context, in GetPodsInput) (string, error) {
	return fmt.Sprintf("Pods in %s:\n  nginx-abc123  Running\n  redis-def456  Running", in.Namespace), nil
}

func getLogs(_ context.Context, in GetLogsInput) (string, error) {
	// Simulate large output
	lines := make([]string, in.Lines)
	for i := range lines {
		lines[i] = fmt.Sprintf("[2026-04-11 10:%02d:00] INFO: processing request %d", i%60, i)
	}
	return strings.Join(lines, "\n"), nil
}

func query(_ context.Context, in QueryInput) (string, error) {
	return "id | name | email\n1 | alice | alice@example.com\n2 | bob | bob@example.com", nil
}

func listTables(_ context.Context, _ ListTablesInput) (string, error) {
	return "users\norders\nproducts\nsessions", nil
}

func readFile(_ context.Context, in ReadFileInput) (string, error) {
	return fmt.Sprintf("Contents of %s:\n// ... imagine a very large file here ...", in.Path), nil
}

func main() {
	s := hamr.New("optimized-server", "1.0.0",
		// Minimal schemas: strip descriptions/defaults/enums from tools/list.
		// AI can still pick the right tool, but reads far fewer tokens.
		hamr.WithMinimalSchemas(),
	)

	// Cost tracking: see how many tokens each tool call uses.
	s.Use(middleware.CostTracker(func(stats middleware.CostStats) {
		fmt.Printf("[cost] %s: ~%d tokens (req: %d, resp: %d, %v)\n",
			stats.ToolName, stats.TotalTokens, stats.RequestTokens, stats.ResponseTokens, stats.Duration)
	}))

	// Response truncation: cap large outputs at ~2000 tokens.
	s.Use(middleware.MaxResponseTokens(2000))

	s.Use(middleware.Logger(), middleware.Recovery())

	// Tool groups: AI sees 2 group names instead of 5 individual schemas.
	// It calls the group to see what's inside, then calls the specific tool.
	s.ToolGroup("kubernetes", "Kubernetes cluster operations", func(g *hamr.Group) {
		g.Tool("get_pods", "List pods in a namespace", getPods)
		g.Tool("get_logs", "Get logs from a pod", getLogs)
	})

	s.ToolGroup("database", "Database query operations", func(g *hamr.Group) {
		g.Tool("query", "Run a SQL query", query)
		g.Tool("list_tables", "List all tables", listTables)
	})

	// Standalone tool (not in a group)
	s.Tool("read_file", "Read file contents", readFile)

	// Show token cost estimates at startup
	fmt.Println("Estimated schema tokens per tool:")
	for name, tokens := range s.EstimateSchemaTokens() {
		fmt.Printf("  %s: ~%d tokens\n", name, tokens)
	}
	fmt.Println()

	log.Fatal(s.Run())
}
