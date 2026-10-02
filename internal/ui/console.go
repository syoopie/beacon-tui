package ui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/syoopie/beacon-tui/internal/importdetect"
	"github.com/syoopie/beacon-tui/internal/procstat"
	"github.com/syoopie/beacon-tui/internal/rcon"
	"github.com/syoopie/beacon-tui/internal/reconcile"
	"github.com/syoopie/beacon-tui/internal/server"
)

// openConsoleScreen switches to the full-screen console, clears the player rail
// so the first poll after arriving is fresh, and drops to the newest log line.
func (m *model) openConsoleScreen() {
	m.screen = screenConsole
	m.closeRconClient()
	m.rconSnap = rcon.Snapshot{}
	m.tickHist = nil
	m.rconErr = ""
	m.rconAt = time.Time{}
	m.ensureConsoleData()
	m.relayout()
	m.vp.GotoBottom()
}

// closeRconClient releases the held RCON connection, if any, so it does not
// keep sitting open once nothing is going to poll over it.
func (m *model) closeRconClient() {
	if m.rconClient != nil {
		_ = m.rconClient.Close()
		m.rconClient = nil
	}
	m.rconClientAddr = ""
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
		m.closeRconClient()
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
			m.stop = &stopPrompt{id: spec.ID}
			m.relayout()
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
		for _, seg := range wrapLogLine(e.display, w) {
			rows = append(rows, style.Render(seg))
		}
	}
	if len(rows) == 0 && m.logTab == tabServer && m.logImportantOnly && q == "" {
		msg := fmt.Sprintf("no warnings or errors in the last %d lines", maxLogLines)
		return lipgloss.PlaceHorizontal(w, lipgloss.Center, mutedStyle.Render(msg))
	}
	return strings.Join(rows, "\n")
}

// clockPrefix matches the "HH:MM:SS  " formatConsoleLine leads a line with.
var clockPrefix = regexp.MustCompile(`^\d\d:\d\d:\d\d  `)

// wrapLogLine wraps one display line to w columns. A line that leads with a
// clock wraps its message in a column of its own, so continuation rows start
// under the message rather than under the time and the times stay scannable.
func wrapLogLine(display string, w int) []string {
	if loc := clockPrefix.FindStringIndex(display); loc != nil && w-loc[1] >= 20 {
		indent := loc[1]
		segs := strings.Split(ansi.Wrap(display[indent:], w-indent, ""), "\n")
		pad := strings.Repeat(" ", indent)
		segs[0] = display[:indent] + segs[0]
		for i := 1; i < len(segs); i++ {
			segs[i] = pad + segs[i]
		}
		return segs
	}
	return strings.Split(ansi.Wrap(display, w, ""), "\n")
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

// railTextW is the width the rail's text is laid out in.
func (m *model) railTextW() int { return max(m.railW-railChrome, 8) }

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
	// lipgloss counts padding inside Width and the margin and border outside it,
	// and MaxWidth clips the whole block, margin included.
	rail := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).BorderLeft(true).BorderForeground(mutedColor).
		MarginLeft(2).PaddingLeft(2).
		Width(m.railTextW() + 2).MaxWidth(m.railW).Height(m.bodyH).
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
	if word, color := portHealthLabel(r.PortHealth, r.Derived); word != "" {
		port += mutedStyle.Render(" ") + lipgloss.NewStyle().Foreground(color).Render(word)
	}
	parts := []string{
		sectionStyle.Render(string(spec.ID)),
		lipgloss.NewStyle().Foreground(statusColor(r.Derived)).Render(r.Derived.String()),
		port,
		mutedStyle.Render("via " + launchSummary(spec)),
	}
	// Drop whole facts from the right rather than cut one mid-word; the name
	// and status always stay, with an ellipsis if even they do not fit.
	sep := mutedStyle.Render("   ·   ")
	line := strings.Join(parts, sep)
	for len(parts) > 2 && lipgloss.Width(line) > w {
		parts = parts[:len(parts)-1]
		line = strings.Join(parts, sep)
	}
	return ansi.Truncate(line, max(w, 1), "…")
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
// been scrolled out of view and new lines have arrived below the fold since,
// and end jumps back down to them. It stays a blank row otherwise, so the log
// height never shifts.
func (m *model) newLinesRow(w int) string {
	if !m.newBelow || m.vp.AtBottom() {
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
			"mem "+shortIBytes(p.RSS),
			fmt.Sprintf("cpu %.0f%%", p.CPUPercent),
		)
	}
	if n := len(m.tickHist); n > 0 {
		parts = append(parts, fmt.Sprintf("tps %.1f", m.tickHist[n-1].TPS))
	}
	return lipgloss.NewStyle().MaxWidth(max(w, 1)).Render(mutedStyle.Render(strings.Join(parts, "  ·  ")))
}

// railView is the console's right column: the server's fixed details always, and
// its players and live resource use while it is running.
func (m *model) railView() string {
	// Graphs shrink, then go, before they push the live sections off the
	// bottom. The details below those may still be clipped.
	var v string
	for graphH := 2; graphH >= 0; graphH-- {
		var liveH int
		if v, liveH = m.railContent(graphH); liveH <= m.bodyH {
			break
		}
	}
	// Clip what still overflows here rather than in the style, so a section
	// heading is not left at the bottom with none of its rows.
	lines := strings.Split(v, "\n")
	if len(lines) > m.bodyH {
		lines = lines[:max(m.bodyH, 0)]
		for len(lines) > 0 {
			last := strings.TrimSpace(ansi.Strip(lines[len(lines)-1]))
			if last != "" && last != "Details" {
				break
			}
			lines = lines[:len(lines)-1]
		}
	}
	return strings.Join(lines, "\n")
}

// railContent is the rail with each graph graphH rows tall, or none at 0, and
// the height of its live sections: Resources and Players on a running server,
// the whole rail otherwise.
func (m *model) railContent(graphH int) (string, int) {
	spec, ok := m.selected()
	if !ok {
		return "", 0
	}
	r := m.reports[spec.ID]
	running := r.Derived == server.StatusRunning

	details := []string{sectionStyle.Render("Details")}
	port := mutedStyle.Render(fmt.Sprintf("port  %d", spec.Port))
	if word, color := portHealthLabel(r.PortHealth, r.Derived); word != "" {
		port += " " + lipgloss.NewStyle().Foreground(color).Render(word)
	}
	details = append(details,
		port,
		mutedStyle.Render("rcon  "+rconRailLabel(spec)),
		mutedStyle.Render("eula  "+eulaRailLabel(m.eula[spec.ID])),
		"",
		mutedStyle.Render(startLine(spec)),
		mutedStyle.Render(filepath.Base(spec.Dir)),
	)

	players := []string{sectionStyle.Render("Players")}
	switch {
	case !spec.RCON.Enabled || spec.RCON.Port == 0:
		players = append(players, mutedStyle.Render("RCON is off"))
	case !running:
		players = append(players, mutedStyle.Render("server not running"))
	case r.PortHealth != reconcile.PortOpen:
		// RCON opens only once the world has loaded; until then a failed poll
		// is expected, not an error.
		players = append(players, mutedStyle.Render("starting up…"))
	case m.rconErr != "":
		players = append(players, mutedStyle.Render(m.rconErr))
	default:
		players = append(players, mutedStyle.Render(fmt.Sprintf("%d / %d online", m.rconSnap.Online, m.rconSnap.Max)))
		if len(m.rconSnap.Players) == 0 {
			players = append(players, mutedStyle.Render("nobody yet"))
		}
		for _, p := range m.rconSnap.Players {
			players = append(players, "• "+p)
		}
	}

	// A running server leads with what changes, so a short terminal clips the
	// fixed details at the bottom rather than the live numbers.
	sections := [][]string{details, players}
	if running {
		resources := []string{sectionStyle.Render("Resources")}
		if e := m.procErrByID[spec.ID]; e != "" {
			resources = append(resources, mutedStyle.Render(e))
		} else if p, ok := m.procByID[spec.ID]; ok {
			resources = append(resources, m.resourceRows(p, m.procHist[spec.ID], m.tickHist, m.railTextW(), graphH)...)
		}
		sections = [][]string{resources, players, details}
	}
	var rows []string
	liveH := 0
	for i, sec := range sections {
		if i > 0 {
			rows = append(rows, "")
		}
		rows = append(rows, sec...)
		if i < 2 {
			liveH = lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, rows...))
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, rows...), liveH
}

// resourceRows are the rail's live numbers for a running server: uptime, then
// graphs of tick time, CPU and memory over the recent samples, each under a line
// with the current value and the scale the graph is drawn against. Tick speed
// comes over RCON and is left out when the server does not report it.
func (m *model) resourceRows(p procstat.Stat, hist []procstat.Stat, ticks []rcon.Tick, w, graphH int) []string {
	if len(hist) > w {
		hist = hist[len(hist)-w:]
	}
	if len(ticks) > w {
		ticks = ticks[len(ticks)-w:]
	}
	cpu := make([]float64, len(hist))
	mem := make([]float64, len(hist))
	var cpuPeak float64
	var memPeak int64
	for i, h := range hist {
		cpu[i], mem[i] = h.CPUPercent, float64(h.RSS)
		cpuPeak, memPeak = max(cpuPeak, h.CPUPercent), max(memPeak, h.RSS)
	}

	// CPU is drawn against one full core until it goes past it. Memory is drawn
	// against the heap limit, or the peak when the limit is unknown; resident
	// memory can pass the heap limit, so the peak still bounds the scale.
	memScale, memRight := memPeak, "peak "+shortIBytes(memPeak)
	if p.MaxHeap > 0 {
		memScale, memRight = max(p.MaxHeap, memPeak), "heap "+shortIBytes(p.MaxHeap)
	}
	rows := []string{mutedStyle.Render("up   " + humanShortDuration(p.Uptime))}
	rows = append(rows, tickRows(ticks, w, graphH)...)
	rows = append(rows, railStatLine(fmt.Sprintf("cpu  %.0f%%", p.CPUPercent), fmt.Sprintf("peak %.0f%%", cpuPeak), w))
	rows = append(rows, railGraph(cpu, max(100, cpuPeak), w, graphH, accentColor)...)
	rows = append(rows, railStatLine("mem  "+shortIBytes(p.RSS), memRight, w))
	rows = append(rows, railGraph(mem, float64(max(memScale, 1)), w, graphH, runColor)...)
	return append(rows, mutedStyle.Render(fmt.Sprintf("host %.0f%% of RAM", p.MemPercent)))
}

// tickBudgetMS is the time one tick may take at 20 TPS.
const tickBudgetMS = 50

// tickRows are the TPS line and, when the server reports tick time, a graph of
// it against the 50 ms a tick may take before the server falls behind. The
// graph's colour is the current health: green at full speed, amber when it has
// slipped, red when players will notice.
func tickRows(ticks []rcon.Tick, w, graphH int) []string {
	if len(ticks) == 0 {
		return nil
	}
	last := ticks[len(ticks)-1]
	tps := fmt.Sprintf("tps  %.1f", last.TPS)
	if last.MSPT == 0 {
		return []string{mutedStyle.Render(tps)}
	}
	mspt := make([]float64, len(ticks))
	peak := float64(tickBudgetMS)
	for i, t := range ticks {
		mspt[i], peak = t.MSPT, max(peak, t.MSPT)
	}
	color := runColor
	switch {
	case last.TPS < 15:
		color = errColor
	case last.TPS < 19.5:
		color = warnColor
	}
	return append([]string{railStatLine(tps, fmt.Sprintf("%.1f ms/tick", last.MSPT), w)},
		railGraph(mspt, peak, w, graphH, color)...)
}

// railStatLine puts a value on the left of the rail and its scale on the right.
func railStatLine(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return mutedStyle.Render(left)
	}
	return mutedStyle.Render(left + strings.Repeat(" ", gap) + right)
}

// railGraph draws samples as columns rising from the bottom, newest at the
// right, against a fixed top value.
func railGraph(data []float64, top float64, w, h int, color lipgloss.TerminalColor) []string {
	if h == 0 {
		return nil
	}
	sl := sparkline.New(w, h,
		sparkline.WithNoAutoMaxValue(),
		sparkline.WithMaxValue(top),
		sparkline.WithStyle(lipgloss.NewStyle().Foreground(color)),
	)
	sl.PushAll(data)
	sl.Draw()
	return []string{sl.View()}
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

// startLine is the launch command for the rail, or the loader name when Beacon
// manages the install, whose shell line means nothing to an operator.
func startLine(spec server.Spec) string {
	if l := importdetect.InstallerLabel(spec.Start); l != "" {
		return l + ", installed by Beacon"
	}
	return spec.Start
}
