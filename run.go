package hamr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AKhilRaghav0/hamr/transport"
	"github.com/AKhilRaghav0/hamr/tui"
)

// runStdio starts the server with stdio transport.
func (s *Server) runStdio() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	handler := &mcpHandler{server: s}
	t := transport.NewStdio(handler)

	s.logger.Info("starting mcpx server", "name", s.name, "version", s.version, "transport", "stdio")
	return t.Run(ctx)
}

// runSSE starts the server with SSE transport.
func (s *Server) runSSE(addr string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	handler := &mcpHandler{server: s}
	t := transport.NewSSE(handler, addr, s.logger)

	s.logger.Info("starting mcpx server", "name", s.name, "version", s.version, "transport", "sse", "addr", addr)
	return t.Run(ctx)
}

// runSSEWithDashboard starts the SSE server and the TUI dashboard in the same process.
// The dashboard shows real-time tool call monitoring while SSE handles MCP traffic over HTTP.
func (s *Server) runSSEWithDashboard(addr string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Build tool stats for the dashboard
	s.mu.RLock()
	toolStats := make([]tui.ToolStat, 0, len(s.tools))
	for _, td := range s.tools {
		toolStats = append(toolStats, tui.ToolStat{
			Name:        td.Name,
			Description: td.Description,
		})
	}
	s.mu.RUnlock()

	// Create the dashboard
	dash := tui.NewDashboard(s.name, s.version, fmt.Sprintf("sse %s", addr), toolStats)

	// Wire up the onCall callback to feed real calls into the dashboard
	s.OnCall(func(tool string, args map[string]any, duration time.Duration, err error) {
		argsSummary := summarizeArgs(args)
		entry := tui.CallEntry{
			Time:     time.Now(),
			Tool:     tool,
			Args:     argsSummary,
			Duration: duration,
			Success:  err == nil,
		}
		if err != nil {
			entry.Error = err.Error()
		}
		dash.RecordCall(entry)
	})

	// Start SSE server in background
	handler := &mcpHandler{server: s}
	t := transport.NewSSE(handler, addr, s.logger)
	go func() {
		if err := t.Run(ctx); err != nil {
			s.logger.Error("SSE server error", "error", err)
		}
	}()

	// Run the TUI (blocks until quit)
	p := tea.NewProgram(dash, tea.WithAltScreen())
	dash.SetProgram(p)

	_, err := p.Run()
	cancel() // stop SSE server when TUI exits
	return err
}

// summarizeArgs creates a short string representation of tool arguments for the dashboard.
func summarizeArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	// Try to find a primary string arg to display
	for _, key := range []string{"query", "path", "url", "command", "pattern", "text", "message", "name", "sql", "dir"} {
		if v, ok := args[key]; ok {
			s := fmt.Sprintf("%v", v)
			if len(s) > 50 {
				s = s[:47] + "..."
			}
			return fmt.Sprintf("%q", s)
		}
	}
	// Fallback: JSON
	b, err := json.Marshal(args)
	if err != nil {
		return "<args>"
	}
	s := string(b)
	if len(s) > 60 {
		s = s[:57] + "..."
	}
	return s
}
