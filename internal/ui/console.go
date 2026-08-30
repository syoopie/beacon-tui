package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/dustin/go-humanize"

	"github.com/syoopie/beacon-tui/internal/procstat"
	"github.com/syoopie/beacon-tui/internal/rcon"
	"github.com/syoopie/beacon-tui/internal/server"
)

// openConsoleScreen switches to the full-screen console and clears the player
// rail so the first poll after arriving is a fresh one.
func (m *model) openConsoleScreen() {
	m.screen = screenConsole
	m.rconSnap = rcon.Snapshot{}
	m.rconErr = ""
	m.rconAt = time.Time{}
	m.proc = procstat.Stat{}
	m.procErr = ""
	m.procAt = time.Time{}
	m.relayout()
}

// consoleTab is the console screen's top-level split: the raw server log, or
// just player activity. The log is the default because that is what the console
// is for; chat is the narrower view.
type consoleTab int

const (
	tabServer consoleTab = iota
	tabChat
)

// logScrollStep is how many lines one press of up or down moves the log. The
// viewport's own default of one line at a time is too slow for a busy log.
const logScrollStep = 3

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
	case key.Matches(msg, m.keys.Up):
		m.vp.ScrollUp(logScrollStep)
		return m, nil
	case key.Matches(msg, m.keys.Down):
		m.vp.ScrollDown(logScrollStep)
		return m, nil
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
// active tab and filter, hard-wrapped to the column width and styled by tier.
// Wrapping here rather than leaving it to the viewport is what keeps an
// over-long unbreakable token (a stack-trace class name, a path) from widening
// the column and shoving the rail sideways.
func (m *model) logBody() string {
	if m.tail == nil {
		return ""
	}
	w := max(m.vp.Width, 1)
	q := strings.ToLower(strings.TrimSpace(m.logQuery))
	var b strings.Builder
	first := true
	for _, e := range m.tail.entries {
		if !m.lineVisible(e, q) {
			continue
		}
		for _, seg := range strings.Split(ansi.Wrap(e.raw, w, ""), "\n") {
			if !first {
				b.WriteByte('\n')
			}
			first = false
			b.WriteString(styleLogSegment(seg, e.kind, m.logTab))
		}
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

func styleLogSegment(seg string, kind logKind, tab consoleTab) string {
	if tab != tabServer {
		return seg
	}
	switch kind {
	case kindNoise:
		return noiseLineStyle.Render(seg)
	case kindNotable:
		return notableLineStyle.Render(seg)
	default:
		return seg
	}
}

func (m *model) consoleView() string {
	w := max(m.vp.Width, 1)
	// Pin the log column to exactly w and bodyH. Without this a single visible
	// line wider than w (a long unbroken token, an over-long header) would widen
	// the whole column and shove the rail sideways as it scrolled in and out.
	logCol := lipgloss.NewStyle().Width(w).MaxWidth(w).Height(m.bodyH).MaxHeight(m.bodyH).
		Render(lipgloss.JoinVertical(lipgloss.Left,
			m.logHeaderView(w),
			m.tabBarView(w),
			mutedStyle.Render(strings.Repeat("─", w)),
			m.vp.View(),
		))
	if m.railW == 0 {
		return logCol
	}
	rail := lipgloss.NewStyle().
		Width(m.railW).Height(m.bodyH).MaxHeight(m.bodyH).
		BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(mutedColor).
		MarginLeft(1).PaddingLeft(1).
		Render(m.railView())
	return lipgloss.JoinHorizontal(lipgloss.Top, logCol, rail)
}

// logHeaderView is one line: name, status, port, launch method. Anything that
// needs more room goes to the notice banner instead.
func (m *model) logHeaderView(w int) string {
	spec, ok := m.selected()
	if !ok {
		return sectionStyle.Render("Log")
	}
	r := m.reports[spec.ID]
	line := strings.Join([]string{
		sectionStyle.Render(string(spec.ID)),
		lipgloss.NewStyle().Foreground(statusColor(r.Derived)).Render(r.Derived.String()),
		mutedStyle.Render(fmt.Sprintf("port %d", spec.Port)),
		mutedStyle.Render("via " + launchSummary(spec)),
	}, mutedStyle.Render("   ·   "))
	return lipgloss.NewStyle().MaxWidth(max(w, 1)).Render(line)
}

// railView is the console's right column: who is online, and (phase 2c) the
// server process's memory and CPU.
func (m *model) railView() string {
	spec, ok := m.selected()
	if !ok {
		return ""
	}
	running := m.reports[spec.ID].Derived == server.StatusRunning

	rows := []string{sectionStyle.Render("Players")}
	switch {
	case !spec.RCON.Enabled || spec.RCON.Port == 0:
		rows = append(rows, mutedStyle.Render("RCON is off. Turn on enable-rcon in server.properties."))
	case !running:
		rows = append(rows, mutedStyle.Render("server not running"))
	case m.rconErr != "":
		rows = append(rows, mutedStyle.Render(m.rconErr))
	default:
		rows = append(rows, mutedStyle.Render(fmt.Sprintf("%d / %d online", m.rconSnap.Online, m.rconSnap.Max)))
		if len(m.rconSnap.Players) == 0 {
			rows = append(rows, mutedStyle.Render("nobody yet"))
		}
		for _, p := range m.rconSnap.Players {
			rows = append(rows, "• "+p)
		}
	}

	rows = append(rows, "", sectionStyle.Render("Resources"))
	switch {
	case !running:
		rows = append(rows, mutedStyle.Render("—"))
	case m.procErr != "":
		rows = append(rows, mutedStyle.Render(m.procErr))
	default:
		rows = append(rows,
			mutedStyle.Render("mem  "+humanize.IBytes(uint64(m.proc.RSS))),
			mutedStyle.Render(fmt.Sprintf("cpu  %.0f%%", m.proc.CPUPercent)),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...)
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
