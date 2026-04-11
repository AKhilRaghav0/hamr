// Package tui provides a terminal user interface dashboard for mcpx servers.
// It is built on top of the Charmbracelet suite (bubbletea, lipgloss, bubbles)
// and renders a live-updating view of server health, recent tool calls, and
// per-tool statistics.
package tui

import "github.com/charmbracelet/lipgloss"

// Color palette — a purple/indigo primary accent on a dark background.
// All colors are specified as hex strings so they work on 24-bit terminals.
// We intentionally avoid ANSI numbers to keep the palette consistent.
const (
	// Background tones
	colorBase    = lipgloss.Color("#0d0f14") // near-black
	colorSurface = lipgloss.Color("#1a1d27") // card / pane bg
	colorOverlay = lipgloss.Color("#252837") // highlighted row bg

	// Purple accent family
	colorAccent    = lipgloss.Color("#7c6af7") // primary — title, borders
	colorAccentDim = lipgloss.Color("#4a4480") // dimmed accent for section dividers

	// Text hierarchy
	colorText       = lipgloss.Color("#e2e4f0") // default body text
	colorTextDim    = lipgloss.Color("#7a7d99") // secondary / metadata
	colorTextSubtle = lipgloss.Color("#4a4d63") // very dim, column headers

	// Semantic status
	colorGreen  = lipgloss.Color("#4caf91") // success / running
	colorRed    = lipgloss.Color("#e06c75") // error / stopped
	colorYellow = lipgloss.Color("#e5c07b") // warning / degraded
	colorBlue   = lipgloss.Color("#61afef") // informational
	colorCyan   = lipgloss.Color("#56b6c2") // duration / timing values
)

// Styles groups every lipgloss.Style used by the dashboard.
// The struct is initialised once via newStyles() and then threaded through
// the model so that components never rebuild styles on every render.
type Styles struct {
	// ---- Outer chrome -------------------------------------------------------

	// AppBorder is the outermost rounded frame that wraps the entire dashboard.
	AppBorder lipgloss.Style

	// TitleBar renders the top line of the outer frame (server name + version).
	TitleBar lipgloss.Style

	// TitleAccent is the primary accent text inside TitleBar (e.g. "mcpx").
	TitleAccent lipgloss.Style

	// TitleMeta is the dimmer server-name / version segment in the title bar.
	TitleMeta lipgloss.Style

	// ---- Section headers ----------------------------------------------------

	// SectionHeader is the separator row that introduces each pane
	// (e.g. "── Recent Calls ──────────").
	SectionHeader lipgloss.Style

	// SectionTitle is the bold label inside a section header.
	SectionTitle lipgloss.Style

	// ---- Status bar ---------------------------------------------------------

	// StatusRunning renders the green "● Running" badge.
	StatusRunning lipgloss.Style

	// StatusError renders the red "● Error" badge.
	StatusError lipgloss.Style

	// StatusWarning renders the yellow "● Degraded" badge.
	StatusWarning lipgloss.Style

	// StatLabel is the dimmed key in a "Key: Value" pair.
	StatLabel lipgloss.Style

	// StatValue is the bright value in a "Key: Value" pair.
	StatValue lipgloss.Style

	// StatSeparator is the vertical bar between stat pairs.
	StatSeparator lipgloss.Style

	// ---- Call log -----------------------------------------------------------

	// CallTime renders the HH:MM:SS timestamp column.
	CallTime lipgloss.Style

	// CallTool renders the tool-name column.
	CallTool lipgloss.Style

	// CallArgs renders the (truncated) args column.
	CallArgs lipgloss.Style

	// CallDuration renders the duration column (right-aligned, cyan).
	CallDuration lipgloss.Style

	// CallSuccess renders the green ✓ icon.
	CallSuccess lipgloss.Style

	// CallError renders the red ✗ icon.
	CallError lipgloss.Style

	// CallRowSelected highlights the currently focused call row.
	CallRowSelected lipgloss.Style

	// CallRowNormal is the default unselected call row style.
	CallRowNormal lipgloss.Style

	// ---- Tools pane ---------------------------------------------------------

	// ToolName renders the tool name in the tools list.
	ToolName lipgloss.Style

	// ToolDesc renders the tool description (dimmed).
	ToolDesc lipgloss.Style

	// ToolCalls renders the call count for a tool (right-aligned).
	ToolCalls lipgloss.Style

	// ToolLatency renders the average latency for a tool.
	ToolLatency lipgloss.Style

	// ToolErrors renders the error count (red when > 0).
	ToolErrors lipgloss.Style

	// ToolRowSelected highlights the focused tool row.
	ToolRowSelected lipgloss.Style

	// ToolRowNormal is the default unselected tool row style.
	ToolRowNormal lipgloss.Style

	// ColumnHeader renders "TIME", "TOOL", etc. header labels.
	ColumnHeader lipgloss.Style

	// ---- Help bar -----------------------------------------------------------

	// HelpKey renders a keyboard shortcut key.
	HelpKey lipgloss.Style

	// HelpDesc renders the description next to a help key.
	HelpDesc lipgloss.Style

	// HelpBar wraps the entire bottom help line.
	HelpBar lipgloss.Style

	// ---- Generic utilities --------------------------------------------------

	// InfoText is used for generic body copy (uptime, generic labels).
	InfoText lipgloss.Style

	// Subtle is used for very dim annotations.
	Subtle lipgloss.Style

	// PanePadding is applied around pane content to add breathing room.
	PanePadding lipgloss.Style
}

// newStyles constructs and returns all dashboard styles.
// Call once at startup; styles are value types (no pointer needed for reads).
func newStyles() Styles {
	s := Styles{}

	// ---- Outer chrome -------------------------------------------------------

	s.AppBorder = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent)

	s.TitleBar = lipgloss.NewStyle().
		Foreground(colorText).
		Bold(true)

	s.TitleAccent = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true)

	s.TitleMeta = lipgloss.NewStyle().
		Foreground(colorTextDim)

	// ---- Section headers ----------------------------------------------------

	s.SectionHeader = lipgloss.NewStyle().
		Foreground(colorAccentDim)

	s.SectionTitle = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true)

	// ---- Status bar ---------------------------------------------------------

	s.StatusRunning = lipgloss.NewStyle().
		Foreground(colorGreen).
		Bold(true)

	s.StatusError = lipgloss.NewStyle().
		Foreground(colorRed).
		Bold(true)

	s.StatusWarning = lipgloss.NewStyle().
		Foreground(colorYellow).
		Bold(true)

	s.StatLabel = lipgloss.NewStyle().
		Foreground(colorTextDim)

	s.StatValue = lipgloss.NewStyle().
		Foreground(colorText).
		Bold(true)

	s.StatSeparator = lipgloss.NewStyle().
		Foreground(colorTextSubtle)

	// ---- Call log -----------------------------------------------------------

	s.CallTime = lipgloss.NewStyle().
		Foreground(colorTextDim)

	s.CallTool = lipgloss.NewStyle().
		Foreground(colorBlue).
		Bold(true)

	s.CallArgs = lipgloss.NewStyle().
		Foreground(colorTextDim)

	s.CallDuration = lipgloss.NewStyle().
		Foreground(colorCyan)

	s.CallSuccess = lipgloss.NewStyle().
		Foreground(colorGreen).
		Bold(true)

	s.CallError = lipgloss.NewStyle().
		Foreground(colorRed).
		Bold(true)

	s.CallRowSelected = lipgloss.NewStyle().
		Background(colorOverlay).
		Foreground(colorText)

	s.CallRowNormal = lipgloss.NewStyle().
		Foreground(colorText)

	// ---- Tools pane ---------------------------------------------------------

	s.ToolName = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true)

	s.ToolDesc = lipgloss.NewStyle().
		Foreground(colorTextDim)

	s.ToolCalls = lipgloss.NewStyle().
		Foreground(colorText)

	s.ToolLatency = lipgloss.NewStyle().
		Foreground(colorCyan)

	s.ToolErrors = lipgloss.NewStyle().
		Foreground(colorRed)

	s.ToolRowSelected = lipgloss.NewStyle().
		Background(colorOverlay).
		Foreground(colorText)

	s.ToolRowNormal = lipgloss.NewStyle().
		Foreground(colorText)

	// ---- Columns ------------------------------------------------------------

	s.ColumnHeader = lipgloss.NewStyle().
		Foreground(colorTextSubtle).
		Bold(true)

	// ---- Help bar -----------------------------------------------------------

	s.HelpKey = lipgloss.NewStyle().
		Foreground(colorAccent).
		Bold(true)

	s.HelpDesc = lipgloss.NewStyle().
		Foreground(colorTextDim)

	s.HelpBar = lipgloss.NewStyle().
		Foreground(colorTextSubtle).
		BorderTop(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderTopForeground(colorAccentDim).
		PaddingLeft(1)

	// ---- Generic ------------------------------------------------------------

	s.InfoText = lipgloss.NewStyle().
		Foreground(colorText)

	s.Subtle = lipgloss.NewStyle().
		Foreground(colorTextSubtle)

	s.PanePadding = lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingRight(1)

	return s
}
