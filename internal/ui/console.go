package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/dustin/go-humanize"

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
	m.ensureConsoleData()
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
	case msg.String() == "esc":
		// esc alone backs out of the console; the left arrow stays a no-op here
		// so it cannot yank the operator off a log they are reading.
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
	case key.Matches(msg, m.keys.LogBottom):
		m.vp.GotoBottom()
		return m, nil
	case msg.String() == "home" || msg.String() == "g":
		m.vp.GotoTop()
		return m, nil
	case key.Matches(msg, m.keys.Up):
		m.vp.ScrollUp(logScrollStep)
		return m, nil
	case key.Matches(msg, m.keys.Down):
		m.vp.ScrollDown(logScrollStep)
		return m, nil
	case key.Matches(msg, m.keys.Chat):
		return m.enterConsole("")
	case key.Matches(msg, m.keys.Console):
		return m.enterConsole("/")
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// enterConsole opens the console input for the selected server, seeded with
// prefill ("/" to land straight in command mode, "" for a free-text line). The
// input only opens while the server is running, since it has nowhere to send to
// otherwise.
func (m *model) enterConsole(prefill string) (tea.Model, tea.Cmd) {
	spec, ok := m.selected()
	if !ok {
		return m, nil
	}
	if m.reports[spec.ID].Derived != server.StatusRunning {
		m.status = "the console only opens while the server is running"
		return m, nil
	}
	return m, m.openConsole(spec, prefill)
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
		for _, seg := range strings.Split(ansi.Wrap(e.display, w, ""), "\n") {
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
		// tier. It matches the compact text the operator actually sees.
		return strings.Contains(strings.ToLower(e.display), lowerQuery)
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
	head := []string{m.logHeaderView(w), m.tabBarView(w)}
	if m.railW == 0 {
		// No room for the rail, so its facts collapse to one dimmed line.
		if s := m.railStrip(w); s != "" {
			head = append(head, s)
		}
	}
	head = append(head, m.logKeysView(w), m.vp.View(), m.newLinesRow(w))
	logBlock := lipgloss.NewStyle().Width(w).MaxWidth(w).Height(m.bodyH).
		Render(lipgloss.JoinVertical(lipgloss.Left, head...))
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

// powerHint is the s-key binding, labelled for what it does in the server's
// current state. It comes back disabled (and so is skipped by the hint bar)
// when there is no primary action, e.g. a server still starting.
func (m *model) powerHint(s server.Status) key.Binding {
	act, ok := m.primaryAction(s)
	if !ok {
		return key.Binding{}
	}
	switch act {
	case actStart:
		return hint("s", "start")
	case actStop:
		return hint("s", "stop")
	case actMarkStopped:
		return hint("s", "mark stopped")
	default:
		return key.Binding{}
	}
}

// logKeysView is the hint row sitting on top of the log viewport: the filter
// toggle first (named for the view it switches to, right under the word for the
// current one), then the keys that move and search the log. It stands in for a
// plain rule, so it costs no height. While the input is open it falls back to
// the rule, since the arrows drive the input then, not the log.
func (m *model) logKeysView(w int) string {
	if m.console != nil {
		return mutedStyle.Render(strings.Repeat("─", max(w, 1)))
	}
	var b []key.Binding
	if m.logTab == tabServer {
		if m.logImportantOnly {
			b = append(b, hint("f", "full log"))
		} else {
			b = append(b, hint("f", "important only"))
		}
	}
	b = append(b, hint("↑↓", "scroll"), hint("end", "latest"), hint("ctrl+f", "find"))
	return ansi.Truncate(m.hintBar(b...), max(w, 1), "…")
}

// newLinesRow is the centred nudge on its own line under the log: the tail has
// been scrolled out of view while new lines keep arriving below the fold, and
// end jumps back down to them. It stays a blank row when the view is already at
// the bottom, so the log height never shifts.
func (m *model) newLinesRow(w int) string {
	if m.vp.AtBottom() {
		return ""
	}
	pill := lipgloss.NewStyle().Foreground(accentColor).Render("↓ new lines below") +
		"   " + m.hintBar(hint("end", "jump down"))
	return lipgloss.PlaceHorizontal(max(w, 1), lipgloss.Center, pill)
}

// consoleInputReady reports whether the console input can open: a server is
// selected and running, so there is somewhere for a typed line to go.
func (m *model) consoleInputReady() bool {
	spec, ok := m.selected()
	return ok && m.reports[spec.ID].Derived == server.StatusRunning
}

// rconRailLabel is the one-word state of a server's RCON: off, or on with its
// port.
func rconRailLabel(spec server.Spec) string {
	if spec.RCON.Enabled && spec.RCON.Port != 0 {
		return fmt.Sprintf("on · %d", spec.RCON.Port)
	}
	return "off"
}

func eulaRailLabel(accepted bool) string {
	if accepted {
		return "accepted"
	}
	return "not accepted"
}

// railStrip is the one-liner the console shows in place of the rail when the
// terminal is too narrow for it: the facts the header does not already carry.
func (m *model) railStrip(w int) string {
	spec, ok := m.selected()
	if !ok {
		return ""
	}
	parts := []string{"rcon " + rconRailLabel(spec), "eula " + eulaRailLabel(m.eula[spec.ID])}
	if p, ok := m.procByID[spec.ID]; ok {
		parts = append(parts,
			"up "+humanShortDuration(p.Uptime),
			"mem "+humanize.IBytes(uint64(p.RSS)),
			fmt.Sprintf("cpu %.0f%%", p.CPUPercent),
		)
	}
	return lipgloss.NewStyle().MaxWidth(max(w, 1)).Render(mutedStyle.Render(strings.Join(parts, "  ·  ")))
}

// railView is the console's right column: the server's fixed details always, and
// its players and live resource use while it is running.
func (m *model) railView() string {
	spec, ok := m.selected()
	if !ok {
		return ""
	}
	r := m.reports[spec.ID]
	running := r.Derived == server.StatusRunning

	rows := []string{sectionStyle.Render("Details")}
	port := mutedStyle.Render(fmt.Sprintf("port  %d", spec.Port))
	if word, color := portHealthLabel(r.PortHealth); word != "" {
		port += " " + lipgloss.NewStyle().Foreground(color).Render(word)
	}
	rows = append(rows,
		port,
		mutedStyle.Render("rcon  "+rconRailLabel(spec)),
		mutedStyle.Render("eula  "+eulaRailLabel(m.eula[spec.ID])),
		"",
		mutedStyle.Render(spec.Start),
		mutedStyle.Render(filepath.Base(spec.Dir)),
	)

	rows = append(rows, "", sectionStyle.Render("Players"))
	switch {
	case !spec.RCON.Enabled || spec.RCON.Port == 0:
		rows = append(rows, mutedStyle.Render("RCON is off"))
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

	if running {
		rows = append(rows, "", sectionStyle.Render("Resources"))
		if e := m.procErrByID[spec.ID]; e != "" {
			rows = append(rows, mutedStyle.Render(e))
		} else if p, ok := m.procByID[spec.ID]; ok {
			rows = append(rows,
				mutedStyle.Render("up   "+humanShortDuration(p.Uptime)),
				mutedStyle.Render("mem  "+humanize.IBytes(uint64(p.RSS))),
				mutedStyle.Render(fmt.Sprintf("cpu  %.0f%%", p.CPUPercent)),
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
	tabs := tab("Server log", tabServer) + " " + tab("Chat", tabChat)

	// The right side names the current view. The key that changes it, f, leads
	// the hint row just below, next to this word.
	var right string
	switch m.logTab {
	case tabChat:
		right = mutedStyle.Render("player activity")
	case tabServer:
		if m.logImportantOnly {
			right = mutedStyle.Render("important only")
		} else {
			right = mutedStyle.Render("full log")
		}
	}
	if q := strings.TrimSpace(m.logQuery); q != "" {
		right = mutedStyle.Render("search: "+q) + mutedStyle.Render("   ·   ") + right
	}

	// Add the "tab" hint by the tabs only when it and the view word both still
	// fit; on a very narrow log pane the word wins.
	left := tabs
	if lipgloss.Width(tabs)+6+lipgloss.Width(right) <= w {
		left = tabs + "  " + m.help.Styles.ShortKey.Render("tab")
	}

	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return ansi.Truncate(left+strings.Repeat(" ", gap)+right, max(w, 1), "")
}
