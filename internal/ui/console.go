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
// viewport's own default of one line at a time is too slow for a busy log;
// pgup and pgdn still jump a whole screen.
const logScrollStep = 6

var (
	tabActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	tabInactiveStyle = mutedStyle
)

// handleConsoleKey drives the full-screen console: start/stop, the actions
// overlay, tab switch, the important-only toggle, log search, and the command
// input.
func (m *model) handleConsoleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// A pending stop confirm eats the next key: y goes through, anything else
	// backs out.
	if m.pendingStop {
		m.pendingStop = false
		if msg.String() == "y" {
			return m, m.runAction(actStop)
		}
		m.status = "stop cancelled"
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Back):
		if m.logQuery != "" {
			m.logQuery = ""
			m.renderLog()
			return m, nil
		}
		m.screen = screenList
		m.relayout()
		return m, nil
	case key.Matches(msg, m.keys.Power):
		spec, ok := m.selected()
		if !ok {
			return m, nil
		}
		act, ok := m.primaryAction(m.reports[spec.ID].Derived)
		if !ok {
			return m, nil
		}
		if act == actStop {
			m.pendingStop = true
			m.status = "stop " + string(spec.ID) + "?  y = yes, any other key = no"
			return m, nil
		}
		return m, m.runAction(act)
	case key.Matches(msg, m.keys.Kill):
		spec, ok := m.selected()
		if !ok || !m.timedOut[spec.ID] {
			return m, nil
		}
		return m, m.runAction(actForceKill)
	case key.Matches(msg, m.keys.Actions):
		m.openActions()
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
		m.logImportantOnly = !m.logImportantOnly
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
// active tab and filter, hard-wrapped to the column width. Word wrap alone is
// not enough, because a stack frame's class name is one unbreakable token wider
// than the column.
func (m *model) logBody() string {
	if m.tail == nil {
		return ""
	}
	w := max(m.vp.Width, 1)
	q := strings.ToLower(strings.TrimSpace(m.logQuery))
	var rows []string
	for _, e := range m.tail.entries {
		if !m.lineVisible(e, q) {
			continue
		}
		style := m.logLineStyle(e.kind)
		for _, seg := range strings.Split(ansi.Wrap(e.raw, w, ""), "\n") {
			rows = append(rows, style.Render(seg))
		}
	}
	if len(rows) == 0 && m.logTab == tabServer && m.logImportantOnly && q == "" {
		msg := fmt.Sprintf("no warnings or errors in the last %d lines", maxLogLines)
		return lipgloss.PlaceHorizontal(w, lipgloss.Center, mutedStyle.Render(msg))
	}
	return strings.Join(rows, "\n")
}

// logLineStyle colours a server-log line by its tier: errors red, warnings
// orange, events blue. In the full log everything else is dimmed so those three
// stand out, and known noise is dimmed further. The chat tab is uniform, since
// every line there already matters.
func (m *model) logLineStyle(k logKind) lipgloss.Style {
	if m.logTab == tabChat {
		return lipgloss.NewStyle()
	}
	switch k {
	case kindError:
		return lipgloss.NewStyle().Bold(true).Foreground(errColor)
	case kindWarn:
		return lipgloss.NewStyle().Foreground(warnColor)
	case kindEvent:
		return lipgloss.NewStyle().Foreground(accentColor)
	case kindNoise:
		return mutedStyle.Faint(true)
	default:
		return mutedStyle
	}
}

func (m *model) lineVisible(e logEntry, lowerQuery string) bool {
	if m.logTab == tabChat {
		if !e.kind.onChatTab() {
			return false
		}
	} else if e.kind == kindChat {
		return false // raw chat lives on the Chat tab only
	}
	if lowerQuery != "" {
		// An active search looks through the whole log, not just the current
		// tier. The tab split above still holds.
		return strings.Contains(strings.ToLower(e.raw), lowerQuery)
	}
	if m.logTab == tabServer && m.logImportantOnly && !e.kind.important() {
		return false
	}
	return true
}

// railChrome is what the rail spends on its left border and the breathing room
// either side of it. relayout reserves it out of railW so the rail's text gets
// the remainder.
const railChrome = 5

func (m *model) consoleView() string {
	w := max(m.vp.Width, 1)
	logBlock := lipgloss.NewStyle().Width(w).MaxWidth(w).Height(m.bodyH).Render(
		lipgloss.JoinVertical(lipgloss.Left,
			m.logHeaderView(w),
			m.tabBarView(w),
			mutedStyle.Render(strings.Repeat("─", w)),
			m.vp.View(),
		))
	if m.railW == 0 {
		return logBlock
	}
	railW := max(m.railW-railChrome, 8)
	rail := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(mutedColor).
		MarginLeft(2).PaddingLeft(2).
		Width(railW).MaxWidth(railW).Height(m.bodyH).
		Render(m.railView())
	return lipgloss.JoinHorizontal(lipgloss.Top, logBlock, rail)
}

// logHeaderView is one line: name, status, port, launch method. Anything that
// needs more room goes to the notice banner instead.
func (m *model) logHeaderView(w int) string {
	spec, ok := m.selected()
	if !ok {
		return sectionStyle.Render("Log")
	}
	r := m.reports[spec.ID]
	port := mutedStyle.Render(fmt.Sprintf("port %d", spec.Port))
	if word, color := portHealthLabel(r.PortHealth); word != "" {
		port += mutedStyle.Render(" ") + lipgloss.NewStyle().Foreground(color).Render(word)
	}
	line := strings.Join([]string{
		sectionStyle.Render(string(spec.ID)),
		lipgloss.NewStyle().Foreground(statusColor(r.Derived)).Render(r.Derived.String()),
		port,
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
	r := m.reports[spec.ID]
	running := r.Derived == server.StatusRunning

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
	default:
		if word, color := portHealthLabel(r.PortHealth); word != "" {
			rows = append(rows, mutedStyle.Render("port ")+lipgloss.NewStyle().Foreground(color).Render(word))
		}
		if m.procErr != "" {
			rows = append(rows, mutedStyle.Render(m.procErr))
		} else {
			rows = append(rows,
				mutedStyle.Render("mem  "+humanize.IBytes(uint64(m.proc.RSS))),
				mutedStyle.Render(fmt.Sprintf("cpu  %.0f%%", m.proc.CPUPercent)),
			)
		}
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
	case m.logImportantOnly:
		right = mutedStyle.Render("important only") + mutedStyle.Render("  ·  ") + m.hintBar(hint("f", "show all"))
	default:
		right = mutedStyle.Render("full log") + mutedStyle.Render("  ·  ") + m.hintBar(hint("f", "important only"))
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
