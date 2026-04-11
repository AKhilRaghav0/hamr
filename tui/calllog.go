// Package tui provides the terminal user interface for mcpx's development
// dashboard.
//
// The dashboard is a Bubble Tea application that renders a live, scrollable
// table of every tool call made to the running MCP server. Each row shows the
// call time, tool name, truncated arguments, duration, and a success/error
// indicator.
//
// Use Server.RunSSEWithDashboard to start the server with the dashboard
// enabled, or construct a DashboardModel directly and feed it CallEntry values
// via the AddCall method when embedding the TUI in a custom program.
package tui

import (
	"fmt"
	"strings"
	"time"
)

const (
	// maxCallHistory is the maximum number of call entries retained in memory.
	maxCallHistory = 100

	// Column widths (in characters) for the call log table.
	callTimeWidth = 8  // "HH:MM:SS"
	callToolWidth = 14 // tool name, padded / truncated
	callDurWidth  = 8  // "9999ms  "
	callIconWidth = 1  // "✓" / "✗"
)

// CallEntry records a single tool invocation.
type CallEntry struct {
	// Time is when the call was initiated.
	Time time.Time

	// Tool is the registered tool name.
	Tool string

	// Args is a human-readable, truncated summary of the call arguments.
	Args string

	// Duration is the end-to-end wall time for the call.
	Duration time.Duration

	// Success reports whether the tool returned without error.
	Success bool

	// Error holds the error message when Success is false.
	Error string
}

// callLog is the scrollable call-history component.
// It owns the ordered slice of entries and the current scroll offset.
type callLog struct {
	entries  []CallEntry // most-recent first
	offset   int         // topmost visible row index
	selected int         // focused row index (0 = newest)
	styles   Styles
}

// newCallLog constructs an empty callLog with the given styles.
func newCallLog(s Styles) callLog {
	return callLog{styles: s}
}

// push prepends entry to the log and caps the slice at maxCallHistory.
func (cl *callLog) push(entry CallEntry) {
	cl.entries = append([]CallEntry{entry}, cl.entries...)
	if len(cl.entries) > maxCallHistory {
		cl.entries = cl.entries[:maxCallHistory]
	}
}

// scrollUp moves the selection toward newer entries.
func (cl *callLog) scrollUp() {
	if cl.selected > 0 {
		cl.selected--
	}
}

// scrollDown moves the selection toward older entries.
func (cl *callLog) scrollDown() {
	if cl.selected < len(cl.entries)-1 {
		cl.selected++
	}
}

// adjustOffset updates the scroll offset so that selected is always visible
// within the given viewport height (measured in rows).
func (cl *callLog) adjustOffset(viewportRows int) {
	if viewportRows <= 0 {
		return
	}
	if cl.selected < cl.offset {
		cl.offset = cl.selected
	}
	if cl.selected >= cl.offset+viewportRows {
		cl.offset = cl.selected - viewportRows + 1
	}
}

// render returns the rendered call-log section as a string.
// width is the inner content width (border / padding already subtracted).
// maxRows caps how many entries are shown.
func (cl *callLog) render(width, maxRows int, active bool) string {
	if width <= 0 || maxRows <= 0 {
		return ""
	}

	cl.adjustOffset(maxRows)

	var b strings.Builder

	// ---- Column header row --------------------------------------------------
	header := cl.renderHeader(width)
	b.WriteString(header)
	b.WriteRune('\n')

	// ---- Entries ------------------------------------------------------------
	end := cl.offset + maxRows
	if end > len(cl.entries) {
		end = len(cl.entries)
	}

	for i := cl.offset; i < end; i++ {
		row := cl.renderRow(cl.entries[i], width, i == cl.selected && active)
		b.WriteString(row)
		if i < end-1 {
			b.WriteRune('\n')
		}
	}

	// ---- Empty-state placeholder --------------------------------------------
	if len(cl.entries) == 0 {
		placeholder := cl.styles.Subtle.Render("  no calls recorded yet")
		b.WriteString(placeholder)
	}

	return b.String()
}

// renderHeader returns the column-header line for the call log.
func (cl *callLog) renderHeader(width int) string {
	s := cl.styles

	timeH := padRight("TIME", callTimeWidth)
	toolH := padRight("TOOL", callToolWidth)
	iconH := "  " // spacer for the status icon column

	// The args column is whatever is left after the fixed columns.
	fixedCols := callTimeWidth + 2 + callToolWidth + 2 + callDurWidth + 2 + callIconWidth
	argsWidth := width - fixedCols
	if argsWidth < 6 {
		argsWidth = 6
	}
	argsH := padRight("ARGS", argsWidth)
	durH := padLeft("DUR", callDurWidth)

	return s.ColumnHeader.Render(
		" " + timeH + "  " + toolH + "  " + argsH + "  " + durH + "  " + iconH,
	)
}

// renderRow renders a single call entry as a fixed-layout table row.
func (cl *callLog) renderRow(e CallEntry, width int, selected bool) string {
	s := cl.styles

	// Compute available width for the args column.
	fixedCols := callTimeWidth + 2 + callToolWidth + 2 + callDurWidth + 2 + callIconWidth
	argsWidth := width - fixedCols
	if argsWidth < 6 {
		argsWidth = 6
	}

	timeStr := s.CallTime.Render(padRight(e.Time.Format("15:04:05"), callTimeWidth))
	toolStr := s.CallTool.Render(padRight(truncate(e.Tool, callToolWidth), callToolWidth))
	argsStr := s.CallArgs.Render(padRight(truncate(e.Args, argsWidth), argsWidth))
	durStr := s.CallDuration.Render(padLeft(formatDuration(e.Duration), callDurWidth))

	var icon string
	if e.Success {
		icon = s.CallSuccess.Render("✓")
	} else {
		icon = s.CallError.Render("✗")
	}

	row := " " + timeStr + "  " + toolStr + "  " + argsStr + "  " + durStr + "  " + icon

	if selected {
		// Apply a background highlight across the full row width.
		row = s.CallRowSelected.Width(width).Render(row)
	}
	return row
}

// ---- helpers ----------------------------------------------------------------

// truncate shortens s to at most n visible characters, appending "…" if needed.
func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(runes[:n-1]) + "…"
}

// padRight pads or truncates s to exactly n characters (left-aligned).
func padRight(s string, n int) string {
	runes := []rune(s)
	if len(runes) >= n {
		return string(runes[:n])
	}
	return s + strings.Repeat(" ", n-len(runes))
}

// padLeft pads or truncates s to exactly n characters (right-aligned).
func padLeft(s string, n int) string {
	runes := []rune(s)
	if len(runes) >= n {
		return string(runes[:n])
	}
	return strings.Repeat(" ", n-len(runes)) + s
}

// formatDuration renders a duration in a compact, human-readable form.
//
//	< 1 ms  → "0ms"
//	< 1 s   → "123ms"
//	< 1 min → "12.3s"
//	>= 1 min → "4m32s"
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return "0ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%ds", m, s)
}
