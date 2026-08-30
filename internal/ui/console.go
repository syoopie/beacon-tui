package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/syoopie/beacon-tui/internal/server"
)

// consoleTab is the console screen's top-level split: the raw server log, or
// just player activity. The log is the default because that is what the console
// is for; chat is the narrower view.
type consoleTab int

const (
	tabServer consoleTab = iota
	tabChat
)

var (
	tabActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	tabInactiveStyle = mutedStyle
	noiseLineStyle   = mutedStyle
	notableLineStyle = lipgloss.NewStyle().Foreground(accentColor)
)

// handleConsoleKey drives the full-screen console: tab switch, the noise-filter
// toggle, log search, and the command input.
func (m *model) handleConsoleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		if m.logQuery != "" {
			m.logQuery = ""
			m.renderLog()
			return m, nil
		}
		m.screen = screenMenu
		m.relayout()
		return m, nil
	case key.Matches(msg, m.keys.LogTab):
		if m.logTab == tabChat {
			m.logTab = tabServer
		} else {
			m.logTab = tabChat
		}
		m.renderLog()
		m.vp.GotoBottom()
		return m, nil
	case key.Matches(msg, m.keys.LogFilter):
		m.logFull = !m.logFull
		m.renderLog()
		m.vp.GotoBottom()
		return m, nil
	case key.Matches(msg, m.keys.LogSearch):
		return m, m.openLogSearch()
	case key.Matches(msg, m.keys.Console):
		spec, ok := m.selected()
		if !ok {
			return m, nil
		}
		if m.reports[spec.ID].Derived != server.StatusRunning {
			m.status = "typing a command works only while the server is running"
			return m, nil
		}
		return m, m.openConsole(spec)
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// openLogSearch focuses a one-line input that narrows the visible log to lines
// containing the query. It filters live as the operator types.
func (m *model) openLogSearch() tea.Cmd {
	ti := textinput.New()
	ti.Prompt = "search  "
	ti.Placeholder = "text to find in the log"
	ti.CharLimit = 128
	ti.SetValue(m.logQuery)
	ti.CursorEnd()
	ti.Focus()
	m.logSearch = &ti
	m.relayout()
	return textinput.Blink
}

func (m *model) updateLogSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.logQuery = ""
		m.logSearch = nil
		m.renderLog()
		m.relayout()
		return m, nil
	case "enter":
		m.logSearch = nil
		m.relayout()
		return m, nil
	}
	ti, cmd := m.logSearch.Update(msg)
	m.logSearch = &ti
	m.logQuery = ti.Value()
	m.renderLog()
	return m, cmd
}

// logBody is the console viewport's content: the buffered lines kept by the
// active tab and filter, styled by tier, joined for the viewport to wrap.
func (m *model) logBody() string {
	if m.tail == nil {
		return ""
	}
	q := strings.ToLower(strings.TrimSpace(m.logQuery))
	var b strings.Builder
	first := true
	for _, e := range m.tail.entries {
		if !m.lineVisible(e, q) {
			continue
		}
		if !first {
			b.WriteByte('\n')
		}
		first = false
		b.WriteString(styleLogLine(e, m.logTab))
	}
	return b.String()
}

func (m *model) lineVisible(e logEntry, lowerQuery string) bool {
	switch m.logTab {
	case tabChat:
		if e.kind != kindChat {
			return false
		}
	case tabServer:
		if !m.logFull && e.kind == kindNoise {
			return false
		}
	}
	if lowerQuery != "" && !strings.Contains(strings.ToLower(e.raw), lowerQuery) {
		return false
	}
	return true
}

func styleLogLine(e logEntry, tab consoleTab) string {
	if tab != tabServer {
		return e.raw
	}
	switch e.kind {
	case kindNoise:
		return noiseLineStyle.Render(e.raw)
	case kindNotable:
		return notableLineStyle.Render(e.raw)
	default:
		return e.raw
	}
}

func (m *model) consoleView() string {
	w := max(m.bodyW, 1)
	return lipgloss.JoinVertical(lipgloss.Left,
		m.logHeaderView(),
		m.tabBarView(w),
		mutedStyle.Render(strings.Repeat("─", w)),
		m.vp.View(),
	)
}

// logHeaderView is one line: name, status, port, launch method. Anything that
// needs more room goes to the notice banner instead.
func (m *model) logHeaderView() string {
	spec, ok := m.selected()
	if !ok {
		return sectionStyle.Render("Log")
	}
	r := m.reports[spec.ID]
	return strings.Join([]string{
		sectionStyle.Render(string(spec.ID)),
		lipgloss.NewStyle().Foreground(statusColor(r.Derived)).Render(r.Derived.String()),
		mutedStyle.Render(fmt.Sprintf("port %d", spec.Port)),
		mutedStyle.Render("via " + launchSummary(spec)),
	}, mutedStyle.Render("   ·   "))
}

func (m *model) tabBarView(w int) string {
	tab := func(label string, t consoleTab) string {
		if m.logTab == t {
			return tabActiveStyle.Render("[ " + label + " ]")
		}
		return tabInactiveStyle.Render("  " + label + "  ")
	}
	left := tab("Server log", tabServer) + " " + tab("Chat", tabChat)

	var right string
	switch {
	case m.logTab == tabChat:
		right = mutedStyle.Render("player activity")
	case m.logFull:
		right = mutedStyle.Render("full log · f to filter noise")
	default:
		right = mutedStyle.Render("noise filtered · f for full log")
	}
	if q := strings.TrimSpace(m.logQuery); q != "" {
		right = mutedStyle.Render("search: "+q) + mutedStyle.Render("   ·   ") + right
	}

	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}
