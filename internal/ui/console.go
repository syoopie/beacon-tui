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

// openConsoleScreen switches to the full-screen console, clears the player rail
// so the first poll after arriving is fresh, and drops to the newest log line.
func (m *model) openConsoleScreen() {
	m.screen = screenConsole
	m.rconSnap = rcon.Snapshot{}
	m.rconErr = ""
	m.rconAt = time.Time{}
	m.proc = procstat.Stat{}
	m.procErr = ""
	m.procAt = time.Time{}
	m.relayout()
	m.vp.GotoBottom()
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
// active tab and filter, hard-wrapped to the column width with a two-column
// gutter. The gutter, not colour, is how a notable line stands out: the log
// column is kept plain text so no styled cell can linger under it when a line
// repaints shorter during a scroll.
func (m *model) logBody() string {
	if m.tail == nil {
		return ""
	}
	w := max(m.vp.Width-2, 1) // two columns reserved for the gutter
	q := strings.ToLower(strings.TrimSpace(m.logQuery))
	var b strings.Builder
	first := true
	for _, e := range m.tail.entries {
		if !m.lineVisible(e, q) {
			continue
		}
		gutter := "  "
		if m.logTab == tabServer && e.kind == kindNotable {
			gutter = "▏ "
		}
		for _, seg := range strings.Split(ansi.Wrap(e.raw, w, ""), "\n") {
			if !first {
				b.WriteByte('\n')
			}
			first = false
			b.WriteString(gutter + seg)
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

// railGap is the plain-space gutter between the log column and the rail. It is
// deliberately whitespace: no border character and no colour anywhere on the
// boundary, so nothing that a partial repaint can smear as the log scrolls.
const railGap = "    "

func (m *model) consoleView() string {
	w := max(m.vp.Width, 1)
	logBlock := lipgloss.JoinVertical(lipgloss.Left,
		m.logHeaderView(w),
		m.tabBarView(w),
		mutedStyle.Render(strings.Repeat("─", w)),
		m.vp.View(),
	)
	logRows := padBlock(logBlock, w, m.bodyH)
	if m.railW == 0 {
		return strings.Join(logRows, "\n")
	}
	railRows := padBlock(m.railView(), m.railW, m.bodyH)
	rows := make([]string, m.bodyH)
	for i := range rows {
		rows[i] = logRows[i] + railGap + railRows[i]
	}
	return strings.Join(rows, "\n")
}

// padBlock splits s into exactly h rows, each exactly w display columns: short
// rows are space-padded, long rows truncated, missing rows blank. Composing the
// two console columns row by row from these keeps every boundary at a fixed
// column no matter what any inner view does.
func padBlock(s string, w, h int) []string {
	src := strings.Split(s, "\n")
	out := make([]string, h)
	for i := range out {
		line := ""
		if i < len(src) {
			line = src[i]
		}
		if lw := lipgloss.Width(line); lw > w {
			line = ansi.Truncate(line, w, "")
		} else {
			line += strings.Repeat(" ", w-lw)
		}
		out[i] = line
	}
	return out
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
