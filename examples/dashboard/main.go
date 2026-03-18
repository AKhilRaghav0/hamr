// Example dashboard demonstrates the mcpx TUI dashboard.
// Run this to see the real-time monitoring interface.
package main

import (
	"fmt"
	"math/rand"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/AKhilRaghav0/hamr/tui"
)

func main() {
	tools := []tui.ToolStat{
		{Name: "search", Description: "Search the web", Calls: 0, Errors: 0},
		{Name: "fetch_url", Description: "Fetch URL contents", Calls: 0, Errors: 0},
		{Name: "summarize", Description: "Summarize text", Calls: 0, Errors: 0},
		{Name: "translate", Description: "Translate text", Calls: 0, Errors: 0},
	}

	m := tui.NewDashboard("demo-server", "1.0.0", "stdio", tools)

	p := tea.NewProgram(m, tea.WithAltScreen())

	// Simulate tool calls in the background
	go func() {
		time.Sleep(1 * time.Second)
		toolNames := []string{"search", "fetch_url", "summarize", "translate"}
		queries := []string{
			"golang tutorials",
			"https://go.dev",
			"mcpx framework",
			"how to build MCP servers",
			"Go generics guide",
			"REST API design",
			"kubernetes deployment",
			"docker compose",
		}

		for i := 0; i < 50; i++ {
			time.Sleep(time.Duration(500+rand.Intn(2000)) * time.Millisecond)
			tool := toolNames[rand.Intn(len(toolNames))]
			query := queries[rand.Intn(len(queries))]
			duration := time.Duration(20+rand.Intn(500)) * time.Millisecond
			success := rand.Float64() > 0.1

			entry := tui.CallEntry{
				Time:     time.Now(),
				Tool:     tool,
				Args:     fmt.Sprintf("%q", query),
				Duration: duration,
				Success:  success,
			}
			if !success {
				entry.Error = "context deadline exceeded"
			}

			p.Send(tui.CallMsg(entry))
		}
	}()

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
