package tui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// programRef holds a reference to the running tea.Program in a pointer so
// that DashboardModel (a value type) can be copied freely by bubbletea without
// triggering the "copies lock value" vet warning.
type programRef struct {
	mu  sync.Mutex
	ptr *tea.Program
}

func (r *programRef) set(p *tea.Program) {
	r.mu.Lock()
	r.ptr = p
	r.mu.Unlock()
}

func (r *programRef) send(msg tea.Msg) {
	r.mu.Lock()
	p := r.ptr
	r.mu.Unlock()
	if p != nil {
		p.Send(msg)
	}
}

// ---------------------------------------------------------------------------
// Message types
// ---------------------------------------------------------------------------

// CallMsg is a bubbletea message sent when a tool call completes.
// Cast a CallEntry to CallMsg and deliver it via Program.Send.
type CallMsg CallEntry

// tickMsg is sent on every clock tick to refresh the uptime counter.
type tickMsg time.Time

// ---------------------------------------------------------------------------
// Domain types
// ---------------------------------------------------------------------------

// ToolStat holds aggregated statistics for a single registered tool.
type ToolStat struct {
	// Name is the tool's registered identifier.
	Name string

	// Description is the human-readable summary of what the tool does.
	Description string

	// Calls is the total number of invocations recorded since startup.
	Calls int

	// AvgLatency is the running average call duration.
	AvgLatency time.Duration

	// Errors is the number of invocations that returned an error.
	Errors int
}

// ---------------------------------------------------------------------------
// Pane / focus management
// ---------------------------------------------------------------------------

// pane identifies which section of the dashboard currently has keyboard focus.
type pane int

const (
	paneCalls pane = iota // Recent Calls pane
	paneTools             // Tools pane
)

// ---------------------------------------------------------------------------
// DashboardModel
// ---------------------------------------------------------------------------

// DashboardModel is the top-level bubbletea model for the mcpx live dashboard.
// It implements tea.Model so it can be run directly with tea.NewProgram.
//
// Use NewDashboard to construct an instance and RecordCall to stream call data
// into the dashboard from other goroutines.
type DashboardModel struct {
	// ---- server metadata ---------------------------------------------------
	serverName    string
	serverVersion string
	transport     string
	startTime     time.Time

	// ---- live data ---------------------------------------------------------
	tools         []ToolStat
	totalRequests int
	totalErrors   int

	// ---- sub-components ----------------------------------------------------
	log callLog

	// ---- focus & navigation ------------------------------------------------
	activePane   pane
	toolSelected int // index into tools slice

	// ---- rendering helpers -------------------------------------------------
	width, height int
	styles        Styles
	ready         bool // true once we have received the first WindowSizeMsg

	// ---- thread-safety -----------------------------------------------------
	// progRef is a pointer so that DashboardModel can be copied by value
	// (as bubbletea requires) without copying the embedded mutex.
	progRef *programRef
}

// NewDashboard constructs a DashboardModel ready to be handed to tea.NewProgram.
//
//	name      — server name shown in the title bar
//	version   — version string shown in the title bar (without "v" prefix)
//	transport — transport label ("stdio", "sse", …)
//	tools     — initial tool list (may be nil; stats are updated via CallMsg)
func NewDashboard(name, version, transport string, tools []ToolStat) DashboardModel {
	s := newStyles()
	return DashboardModel{
		serverName:    name,
		serverVersion: version,
		transport:     transport,
		startTime:     time.Now(),
		tools:         tools,
		styles:        s,
		log:           newCallLog(s),
		width:         120,
		height:        40,
		progRef:       &programRef{},
	}
}

// SetProgram wires the tea.Program back into the model so that RecordCall can
// use Program.Send to deliver CallMsg values from arbitrary goroutines.
// Call this immediately after tea.NewProgram, before p.Run().
//
//	p := tea.NewProgram(dash, tea.WithAltScreen())
//	dash.SetProgram(p)
//	p.Run()
func (m *DashboardModel) SetProgram(p *tea.Program) {
	m.progRef.set(p)
}

// RecordCall is the thread-safe public API for pushing a completed tool call
// into the dashboard. It may be called from any goroutine concurrently with
// the bubbletea event loop.
func (m *DashboardModel) RecordCall(entry CallEntry) {
	m.progRef.send(CallMsg(entry))
}

// ---------------------------------------------------------------------------
// tea.Model interface
// ---------------------------------------------------------------------------

// Init returns the initial command: a 1-second tick for the uptime counter.
func (m DashboardModel) Init() tea.Cmd {
	return doTick()
}

// Update handles all incoming messages and returns the updated model plus an
// optional follow-up command.
func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// ---- terminal resized --------------------------------------------------
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

	// ---- clock tick (uptime refresh) ---------------------------------------
	case tickMsg:
		return m, doTick()

	// ---- keyboard ----------------------------------------------------------
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "Q", "ctrl+c":
			return m, tea.Quit

		case "tab", "shift+tab":
			if m.activePane == paneCalls {
				m.activePane = paneTools
			} else {
				m.activePane = paneCalls
			}

		case "up", "k":
			if m.activePane == paneCalls {
				m.log.scrollUp()
			} else {
				if m.toolSelected > 0 {
					m.toolSelected--
				}
			}

		case "down", "j":
			if m.activePane == paneCalls {
				m.log.scrollDown()
			} else {
				if m.toolSelected < len(m.tools)-1 {
					m.toolSelected++
				}
			}
		}

	// ---- tool call completed -----------------------------------------------
	case CallMsg:
		entry := CallEntry(msg)
		m.log.push(entry)
		m.totalRequests++
		if !entry.Success {
			m.totalErrors++
		}
		m = m.updateToolStat(entry)
	}

	return m, nil
}

// View renders the full dashboard as a string.
func (m DashboardModel) View() string {
	if !m.ready {
		return m.styles.Subtle.Render("  initialising…")
	}

	s := m.styles

	// Account for the outer rounded border (1 char each side).
	innerWidth := m.width - 2
	if innerWidth < 20 {
		innerWidth = 20
	}

	sections := []string{
		m.renderTitle(innerWidth),
		m.renderStatusBar(innerWidth),
		m.renderSectionDivider("Recent Calls", innerWidth),
		m.renderCallLog(innerWidth),
		m.renderSectionDivider("Tools", innerWidth),
		m.renderToolsPane(innerWidth),
		m.renderHelpBar(innerWidth),
	}

	body := strings.Join(sections, "\n")

	return s.AppBorder.
		Width(innerWidth).
		Render(body)
}

// ---------------------------------------------------------------------------
// Section render helpers
// ---------------------------------------------------------------------------

// renderTitle renders the header line: "mcpx v1.0.0 · my-server   stdio".
func (m DashboardModel) renderTitle(width int) string {
	s := m.styles

	left := s.TitleAccent.Render("mcpx") +
		s.TitleMeta.Render(" v"+m.serverVersion+" · "+m.serverName)
	right := s.TitleMeta.Render(m.transport)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderStatusBar renders the status / stats row beneath the title.
func (m DashboardModel) renderStatusBar(width int) string {
	s := m.styles

	statusStr := s.StatusRunning.Render("● Running")

	uptime := time.Since(m.startTime).Round(time.Second)
	uptimeStr := s.StatLabel.Render("Uptime: ") + s.StatValue.Render(formatDuration(uptime))

	toolCountStr := s.StatLabel.Render("Tools: ") +
		s.StatValue.Render(fmt.Sprintf("%d", len(m.tools)))

	reqStr := s.StatLabel.Render("Requests: ") +
		s.StatValue.Render(fmt.Sprintf("%d", m.totalRequests))

	var errPart string
	if m.totalErrors > 0 {
		errPart = s.StatusError.Render(fmt.Sprintf("%d", m.totalErrors))
	} else {
		errPart = s.StatValue.Render("0")
	}
	errStr := s.StatLabel.Render("Errors: ") + errPart

	sep := s.StatSeparator.Render("  │  ")
	line := statusStr + sep + uptimeStr + sep + toolCountStr + sep + reqStr + sep + errStr

	// Pad to full width for consistent layout.
	lineW := lipgloss.Width(line)
	if lineW < width {
		line += strings.Repeat(" ", width-lineW)
	}
	return line
}

// renderSectionDivider renders a horizontal rule with an embedded label:
//
//	"── Recent Calls ─────────────────────────────────────"
func (m DashboardModel) renderSectionDivider(label string, width int) string {
	s := m.styles

	title := " " + s.SectionTitle.Render(label) + " "
	titleW := lipgloss.Width(title)

	leftRule := s.SectionHeader.Render("──")
	rightLen := width - titleW - lipgloss.Width(leftRule)
	if rightLen < 0 {
		rightLen = 0
	}
	rightRule := s.SectionHeader.Render(strings.Repeat("─", rightLen))

	return leftRule + title + rightRule
}

// renderCallLog renders the Recent Calls pane.
func (m DashboardModel) renderCallLog(width int) string {
	s := m.styles

	// Allocate roughly 40 % of the inner height; clamp to a sane range.
	inner := m.height - 2
	callPaneRows := inner * 2 / 5
	if callPaneRows < 4 {
		callPaneRows = 4
	}
	if callPaneRows > 20 {
		callPaneRows = 20
	}

	// The render method draws its own header row, so subtract 1 from dataRows.
	dataRows := callPaneRows - 1
	if dataRows < 1 {
		dataRows = 1
	}

	active := m.activePane == paneCalls
	content := m.log.render(width-2, dataRows, active) // -2 for left+right padding

	return s.PanePadding.Render(content)
}

// renderToolsPane renders the Tools statistics pane.
func (m DashboardModel) renderToolsPane(width int) string {
	s := m.styles

	if len(m.tools) == 0 {
		return s.PanePadding.Render(s.Subtle.Render("  no tools registered"))
	}

	// Fixed column widths; description gets the remaining space.
	nameW := 14
	callsW := 8
	avgW := 9
	errorsW := 8
	// 5 separators of width 2 each, plus 1 leading space.
	fixedTotal := 1 + nameW + 2 + callsW + 2 + avgW + 2 + errorsW + 2
	descW := width - 2 - fixedTotal // -2 for PanePadding
	if descW < 10 {
		descW = 10
	}

	// Column header row.
	headerStr := " " +
		s.ColumnHeader.Render(padRight("TOOL", nameW)) + "  " +
		s.ColumnHeader.Render(padRight("DESCRIPTION", descW)) + "  " +
		s.ColumnHeader.Render(padLeft("CALLS", callsW)) + "  " +
		s.ColumnHeader.Render(padLeft("AVG", avgW)) + "  " +
		s.ColumnHeader.Render(padLeft("ERRORS", errorsW))

	rows := []string{headerStr}

	active := m.activePane == paneTools

	for i, t := range m.tools {
		selected := i == m.toolSelected && active

		nameStr := s.ToolName.Render(padRight(truncate(t.Name, nameW), nameW))
		descStr := s.ToolDesc.Render(padRight(truncate(t.Description, descW), descW))
		callsStr := s.ToolCalls.Render(padLeft(fmt.Sprintf("%d", t.Calls), callsW))
		latStr := s.ToolLatency.Render(padLeft(formatDuration(t.AvgLatency), avgW))

		var errStr string
		if t.Errors > 0 {
			errStr = s.ToolErrors.Render(padLeft(fmt.Sprintf("%d", t.Errors), errorsW))
		} else {
			errStr = s.ToolCalls.Render(padLeft("0", errorsW))
		}

		row := " " + nameStr + "  " + descStr + "  " + callsStr + "  " + latStr + "  " + errStr

		if selected {
			row = s.ToolRowSelected.Width(width - 2).Render(row)
		}
		rows = append(rows, row)
	}

	return s.PanePadding.Render(strings.Join(rows, "\n"))
}

// renderHelpBar renders the one-line keyboard-shortcut hint at the bottom.
func (m DashboardModel) renderHelpBar(width int) string {
	s := m.styles

	sep := s.StatSeparator.Render("  │  ")
	hints := []string{
		s.HelpKey.Render("q") + s.HelpDesc.Render(" quit"),
		s.HelpKey.Render("tab") + s.HelpDesc.Render(" switch pane"),
		s.HelpKey.Render("↑↓") + s.HelpDesc.Render(" / ") + s.HelpKey.Render("jk") + s.HelpDesc.Render(" scroll"),
	}
	return s.HelpBar.Width(width).Render(strings.Join(hints, sep))
}

// ---------------------------------------------------------------------------
// Business logic helpers
// ---------------------------------------------------------------------------

// updateToolStat merges a completed call entry into the tools statistics slice.
// If no ToolStat exists for entry.Tool, a minimal entry is appended.
func (m DashboardModel) updateToolStat(entry CallEntry) DashboardModel {
	for i := range m.tools {
		if m.tools[i].Name == entry.Tool {
			t := &m.tools[i]
			// Running average: avg = (avg*n + new) / (n+1)
			n := time.Duration(t.Calls)
			t.AvgLatency = (t.AvgLatency*n + entry.Duration) / (n + 1)
			t.Calls++
			if !entry.Success {
				t.Errors++
			}
			return m
		}
	}

	// Unknown tool — create a minimal stat entry so it appears in the table.
	ts := ToolStat{
		Name:       entry.Tool,
		Calls:      1,
		AvgLatency: entry.Duration,
	}
	if !entry.Success {
		ts.Errors = 1
	}
	m.tools = append(m.tools, ts)
	return m
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

// doTick returns a Cmd that fires a tickMsg after one second.
func doTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}
