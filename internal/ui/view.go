package ui

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/dustin/go-humanize"

	"github.com/syoopie/beacon-tui/internal/procstat"
	"github.com/syoopie/beacon-tui/internal/reconcile"
	"github.com/syoopie/beacon-tui/internal/server"
)

// The whole screen sits inside this padding so nothing touches the terminal
// edge. relayout subtracts it before sizing the panes.
const (
	framePadX = 2
	framePadY = 1
)

var (
	accentColor = lipgloss.Color("39")
	runColor    = lipgloss.Color("42")
	warnColor   = lipgloss.Color("214")
	errColor    = lipgloss.Color("203")
	transColor  = lipgloss.Color("45")
	mutedColor  = lipgloss.AdaptiveColor{Light: "245", Dark: "241"}

	frameStyle      = lipgloss.NewStyle().Padding(framePadY, framePadX)
	brandStyle      = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	badgeStyle      = lipgloss.NewStyle().Bold(true).Foreground(warnColor)
	mutedStyle      = lipgloss.NewStyle().Foreground(mutedColor)
	sectionStyle    = lipgloss.NewStyle().Bold(true)
	markerStyle     = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	selectedRow     = lipgloss.NewStyle().Bold(true)
	consoleBarStyle = lipgloss.NewStyle().Foreground(accentColor)
	ctaStyle        = lipgloss.NewStyle().Bold(true).Padding(0, 3).
			Border(lipgloss.RoundedBorder()).BorderForeground(accentColor)
	dialogStyle = lipgloss.NewStyle().Padding(1, 3).
			Border(lipgloss.RoundedBorder()).BorderForeground(accentColor)
)

func statusColor(s server.Status) lipgloss.TerminalColor {
	switch s {
	case server.StatusRunning:
		return runColor
	case server.StatusStarting, server.StatusStopping:
		return transColor
	case server.StatusUnknown:
		return warnColor
	default:
		return mutedColor
	}
}

// portHealthLabel turns a live port probe into a word and its colour: "ready"
// once the server accepts connections, "starting" while its session is up but
// the port has not opened. An unprobed port returns "", so a stopped server's
// header stays just "port 25565".
func portHealthLabel(h reconcile.PortHealth) (string, lipgloss.TerminalColor) {
	switch h {
	case reconcile.PortOpen:
		return "ready", runColor
	case reconcile.PortClosed:
		return "starting", transColor
	default:
		return "", mutedColor
	}
}

func statusGlyph(s server.Status) string {
	switch s {
	case server.StatusRunning:
		return "●"
	case server.StatusStarting, server.StatusStopping:
		return "◐"
	case server.StatusUnknown:
		return "◆"
	default:
		return "○"
	}
}

// --- server list delegate ---

// serverItem is one card in the list. It carries a snapshot of everything the
// card draws, so the delegate stays a pure function of the item.
type serverItem struct {
	spec    server.Spec
	status  server.Status
	health  reconcile.PortHealth
	warn    string
	proc    procstat.Stat
	hasProc bool
}

func (i serverItem) FilterValue() string { return string(i.spec.ID) }

// statusGroup buckets a status for the list sort: live servers first, then
// stopped, then the ones Beacon has lost track of.
func statusGroup(s server.Status) int {
	switch s {
	case server.StatusRunning, server.StatusStarting, server.StatusStopping:
		return 0
	case server.StatusStopped:
		return 1
	default:
		return 2
	}
}

// listCols is the set of columns a given list width can carry. name == 0 means
// the width is too small for a table at all, and the row falls back to one
// loose line. Columns drop right to left as the terminal narrows.
type listCols struct {
	name   int // name column width, 0 for the compact fallback
	status bool
	port   bool
	dot    bool // the port-health dot
	detail bool
}

func columnsFor(width int) listCols {
	switch {
	case width < 55:
		return listCols{}
	case width < 66:
		return listCols{name: 16, status: true}
	case width < 82:
		return listCols{name: 16, status: true, port: true}
	case width < 120:
		return listCols{name: 20, status: true, port: true, dot: true, detail: true}
	default:
		return listCols{name: 28, status: true, port: true, dot: true, detail: true}
	}
}

// portHealthDot is the compact form of portHealthLabel for the list: a filled
// dot after the port, green once the port accepts connections, amber while the
// session is up but the port has not opened, nothing for a stopped server.
func portHealthDot(h reconcile.PortHealth) (string, lipgloss.TerminalColor) {
	switch h {
	case reconcile.PortOpen:
		return "●", runColor
	case reconcile.PortClosed:
		return "●", transColor
	default:
		return " ", mutedColor
	}
}

// serverDelegate draws each server as one aligned row. cols is the column set
// the current width can fit; blurred suppresses the cursor row while focus is
// on the add row above the list.
type serverDelegate struct {
	cols    listCols
	blurred bool
}

func (serverDelegate) Height() int                         { return 1 }
func (serverDelegate) Spacing() int                        { return 0 }
func (serverDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d serverDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(serverItem)
	if !ok {
		return
	}
	width := max(m.Width(), 12)
	selected := index == m.Index() && !d.blurred
	sc := lipgloss.NewStyle().Foreground(statusColor(si.status))

	bar := "  "
	if selected {
		bar = markerStyle.Render("▎ ")
	}
	name := string(si.spec.ID)

	if d.cols.name == 0 { // too narrow for a table
		if selected {
			name = selectedRow.Render(name)
		}
		line := bar + sc.Render(statusGlyph(si.status)) + " " + name +
			mutedStyle.Render("  ·  ") + sc.Render(si.status.String())
		_, _ = fmt.Fprint(w, lipgloss.NewStyle().MaxWidth(width).Render(line))
		return
	}

	nameCell := ansi.Truncate(name, d.cols.name, "…")
	nameCell = nameCell + strings.Repeat(" ", max(d.cols.name-ansi.StringWidth(nameCell), 0))
	if selected {
		nameCell = selectedRow.Render(nameCell)
	}
	line := bar + sc.Render(statusGlyph(si.status)) + " " + nameCell
	line += "  " + sc.Render(pad(si.status.String(), statusColW))
	if d.cols.port {
		cell := strings.Repeat(" ", portColW)
		if si.spec.Port > 0 {
			cell = fmt.Sprintf("%-5d", si.spec.Port)
			if d.cols.dot {
				g, gc := portHealthDot(si.health)
				cell += " " + lipgloss.NewStyle().Foreground(gc).Render(g)
			}
		}
		line += "  " + mutedStyle.Render(cell)
	}
	if d.cols.detail {
		avail := width - ansi.StringWidth(line) - 2
		if avail > 4 {
			ds := mutedStyle
			if selected {
				ds = lipgloss.NewStyle()
			}
			line += "  " + ds.Render(ansi.Truncate(rowDetail(si), avail, "…"))
		}
	}
	_, _ = fmt.Fprint(w, lipgloss.NewStyle().MaxWidth(width).Render(line))
}

const (
	statusColW = 8
	portColW   = 7
)

// pad right-pads s with spaces to display width n.
func pad(s string, n int) string {
	if gap := n - ansi.StringWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// rowDetail is the abbreviated last column: how a running server is doing, what
// starts a stopped one, or a short token for one Beacon has lost. The full text
// lives in the console and, for the lost case, the notice banner.
func rowDetail(si serverItem) string {
	switch statusGroup(si.status) {
	case 0:
		if si.hasProc {
			return fmt.Sprintf("%s · %s · %.0f%%",
				humanShortDuration(si.proc.Uptime),
				shortIBytes(si.proc.RSS),
				si.proc.CPUPercent)
		}
		return launchSummary(si.spec)
	case 2:
		return "session lost"
	default:
		return launchSummary(si.spec)
	}
}

// shortIBytes is humanize.IBytes with the unit clipped to one letter: "3.1G".
func shortIBytes(n int64) string {
	s := strings.TrimSuffix(humanize.IBytes(uint64(n)), "iB")
	return strings.ReplaceAll(s, " ", "")
}

// humanShortDuration is a compact "4h12m" style age, for a card that has one
// column to spare.
func humanShortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

// --- the command bar ---

// commandBar picks the bindings shown at the top of the screen. It is the
// user's answer to "what can I do right now", so it tracks the mode and the
// selected server's status rather than listing every key all the time.
func (m *model) commandBar() helpSet {
	switch {
	case m.console != nil:
		short := []key.Binding{hint("enter", "send")}
		if m.completer != nil {
			short = append(short, hint("tab", "complete"), hint("↑↓", "history"))
		}
		return helpSet{short: append(short, hint("esc", "close console"))}
	case m.logSearch != nil:
		return helpSet{short: []key.Binding{hint("enter", "keep filter"), hint("esc", "clear search")}}
	case m.pat != nil, m.launch != nil, m.config != nil:
		// These are centred dialogs that carry their own key hints in a footer,
		// right under the fields. Repeating them up here just adds noise. They
		// can sit on top of the actions overlay, so this case comes first.
		return helpSet{}
	case m.actions != nil:
		return helpSet{short: []key.Binding{hint("↑↓", "move"), hint("enter", "run"), hint("esc", "close")}}
	case m.pick != nil:
		return helpSet{short: []key.Binding{
			hint("→", "open folder"), hint("←", "up a level"),
			hint("enter", "choose this folder"), hint("esc", "cancel"),
		}}
	}

	if m.screen == screenConsole {
		spec, ok := m.selected()
		power := m.keys.Power
		if ok {
			if act, valid := m.primaryAction(m.reports[spec.ID].Derived); valid {
				switch act {
				case actStart:
					power = hint("s", "start")
				case actStop:
					power = hint("s", "stop")
				case actMarkStopped:
					power = hint("s", "mark stopped")
				}
			}
		}
		short := []key.Binding{
			power, m.keys.Actions, hint("c", "command"),
			hint("tab", "tabs"), hint("/", "search"), hint("esc", "back"), m.keys.Help,
		}
		full := [][]key.Binding{
			{power, m.keys.Actions, m.keys.Console},
			{hint("↑↓", "scroll"), m.keys.LogTab, m.keys.LogFilter, m.keys.LogSearch},
			{m.keys.Back, m.keys.Help, m.keys.Quit},
		}
		if ok && m.timedOut[spec.ID] {
			full[0] = append(full[0], m.keys.Kill)
		}
		return helpSet{short: short, full: full}
	}

	// screenList
	if len(m.specs) == 0 {
		return helpSet{short: []key.Binding{m.keys.Add, hint("esc", "quit")}}
	}
	back := hint("esc", "quit")
	if m.list.FilterInput.Value() != "" {
		back = hint("esc", "clear search")
	}
	act := m.keys.Act
	if m.onAddRow {
		act = hint("enter", "add server")
	}
	short := []key.Binding{
		m.keys.Up, m.keys.Down, act, m.keys.Rescan, back,
	}
	return helpSet{short: short}
}

// --- view ---

func (m *model) View() string {
	if !m.ready {
		return "starting Beacon…"
	}
	rows := []string{
		m.headerView(),
		m.help.View(m.commandBar()),
		"",
	}
	if n := m.noticeView(); n != "" {
		rows = append(rows, n, "")
	}
	rows = append(rows, m.bodyView())
	switch {
	case m.console != nil:
		rows = append(rows, "", m.completionPanelView(m.bodyW), consoleBarStyle.Render(m.console.View()))
	case m.logSearch != nil:
		rows = append(rows, "", consoleBarStyle.Render(m.logSearch.View()))
	}
	rows = append(rows, "", m.statusView())
	return frameStyle.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

// breadcrumb is the path shown at the top left: "Beacon" on the list, and a
// trail down to wherever the operator has stepped. "Beacon" stays in the brand
// colour, the separators are faint, and the last segment reads in normal weight.
func (m *model) breadcrumb() string {
	segs := []string{"Beacon"}
	if spec, ok := m.selected(); ok && m.screen == screenConsole {
		segs = append(segs, string(spec.ID))
		if m.actions != nil {
			segs = append(segs, "actions")
		}
		switch {
		case m.pat != nil:
			segs = append(segs, "fix start script")
		case m.launch != nil:
			segs = append(segs, "launch settings")
		case m.config != nil:
			segs = append(segs, "edit config")
		}
	}
	if m.pick != nil {
		segs = []string{"Beacon", "add server"}
	}

	sep := mutedStyle.Render("  ›  ")
	out := brandStyle.Render(segs[0])
	for i, s := range segs[1:] {
		style := mutedStyle
		if i == len(segs)-2 {
			style = lipgloss.NewStyle()
		}
		out += sep + style.Render(s)
	}
	return out
}

func (m *model) headerView() string {
	left := m.breadcrumb()
	var right string
	if m.update != nil {
		right = badgeStyle.Render("↑ " + m.update.latest + " available")
	}
	gap := m.bodyW - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *model) statusView() string {
	return lipgloss.NewStyle().Foreground(mutedColor).MaxWidth(m.bodyW).Render(m.status)
}

// noticeText is the banner for the selected server when something needs the
// operator's attention. Empty when all is well.
func (m *model) noticeText() string {
	if m.pat != nil || m.pick != nil || m.launch != nil || m.config != nil || m.actions != nil {
		return ""
	}
	// The notice belongs to whichever server is highlighted, so it rides along
	// with the selection on every screen.
	spec, ok := m.selected()
	if !ok {
		return ""
	}
	r := m.reports[spec.ID]
	switch {
	case r.Derived == server.StatusUnknown && r.Warning != "":
		return "⚠  " + r.Warning + "  Once you have checked, choose Mark stopped."
	case !spec.Exec.Launchable():
		return "⚠  " + string(spec.ID) + "'s start script does not hand off to Java with exec, so Beacon can't start it. Choose Fix start script, or Launch settings to point it at another one."
	case !m.eula[spec.ID]:
		return "⚠  " + string(spec.ID) + " has not accepted the Minecraft EULA, so Beacon can't start it. Choose Accept the Minecraft EULA once you agree to https://aka.ms/MinecraftEULA."
	}
	return ""
}

func (m *model) noticeView() string {
	t := m.noticeText()
	if t == "" {
		return ""
	}
	return lipgloss.NewStyle().Foreground(warnColor).Width(m.bodyW).Render(t)
}

func (m *model) bodyView() string {
	var content string
	switch {
	case m.pat != nil:
		content = m.patchDialogView()
	case m.launch != nil:
		content = m.launchDialogView()
	case m.config != nil:
		content = m.configDialogView()
	case m.actions != nil:
		content = m.actionsDialogView()
	case m.pick != nil:
		content = m.pickerView()
	case m.loaded && len(m.specs) == 0:
		content = m.landingView()
	case m.screen == screenConsole:
		content = m.consoleView()
	default:
		content = m.listView()
	}
	return lipgloss.NewStyle().MaxWidth(m.bodyW).
		Height(m.bodyH).MaxHeight(m.bodyH).Render(content)
}

// listView is the home screen: a bordered search box, the add row, a column
// header, then the aligned, view-only list of servers.
func (m *model) listView() string {
	parts := []string{m.searchBoxView(), ""}
	if m.addRowVisible() {
		parts = append(parts, m.addRowLine(), "")
	}
	if columnsFor(m.listW).name > 0 {
		parts = append(parts,
			m.columnHeaderView(),
			mutedStyle.Render(strings.Repeat("─", max(m.listW-1, 1))))
	}
	parts = append(parts, m.list.View())
	body := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return lipgloss.NewStyle().Width(m.bodyW).MaxWidth(m.bodyW).Render(body)
}

// addRowVisible reports whether the add row is on screen: there is a list to
// sit above, and no search is narrowing it.
func (m *model) addRowVisible() bool {
	return len(m.specs) > 0 && m.list.FilterInput.Value() == ""
}

func (m *model) matchCount() int {
	n := 0
	for _, it := range m.list.VisibleItems() {
		if _, ok := it.(serverItem); ok {
			n++
		}
	}
	return n
}

// searchBoxView is the persistent filter, a rounded border open on the right.
// The border is muted until the search has text, then accent, so an active
// filter reads at a glance.
func (m *model) searchBoxView() string {
	q := m.list.FilterInput.Value()
	line := "⌕ "
	if q == "" {
		line += mutedStyle.Render("Search…")
	} else {
		line += q
	}
	innerW := max(m.bodyW-2, 12) // border box content width
	textW := innerW - 1          // minus the left padding
	if q != "" {
		count := mutedStyle.Render(fmt.Sprintf("%d shown", m.matchCount()))
		line = ansi.Truncate(line, textW-ansi.StringWidth(count)-2, "…")
		gap := textW - ansi.StringWidth(line) - ansi.StringWidth(count)
		if gap < 2 {
			gap = 2
		}
		line += strings.Repeat(" ", gap) + count
	}
	var col lipgloss.TerminalColor = mutedColor
	if q != "" {
		col = accentColor
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true, false, true, true).
		BorderForeground(col).
		PaddingLeft(1).
		Width(innerW).MaxWidth(m.bodyW).
		Render(ansi.Truncate(line, textW, "…"))
}

func (m *model) addRowLine() string {
	bar, label := "  ", "+  Add a server"
	if m.onAddRow {
		bar = markerStyle.Render("▎ ")
		label = selectedRow.Render(label)
	} else {
		label = mutedStyle.Render(label)
	}
	return bar + label
}

// columnHeaderView labels the row columns, aligned to the delegate's layout:
// two for the cursor bar, two for the status glyph, then the columns.
func (m *model) columnHeaderView() string {
	c := columnsFor(m.listW)
	h := "    " + mutedStyle.Render(pad("NAME", c.name))
	if c.status {
		h += "  " + mutedStyle.Render(pad("STATUS", statusColW))
	}
	if c.port {
		h += "  " + mutedStyle.Render(pad("PORT", portColW))
	}
	if c.detail {
		h += "  " + mutedStyle.Render("DETAIL")
	}
	return h
}

// launchSummary names what starts the server: the start script, or the jar
// pulled from its launch command.
func launchSummary(s server.Spec) string {
	if s.Script != "" {
		return s.Script
	}
	for _, f := range strings.Fields(s.Start) {
		if strings.HasSuffix(f, ".jar") {
			return f
		}
	}
	return s.Start
}

func (m *model) landingView() string {
	panel := lipgloss.JoinVertical(lipgloss.Center,
		brandStyle.Render("Beacon"),
		mutedStyle.Render("Run your Minecraft servers from one screen."),
		"",
		lipgloss.NewStyle().Foreground(mutedColor).Align(lipgloss.Center).Render(
			"You have no servers yet.\n\n"+
				"Beacon needs the folder your server lives in —\n"+
				"the one with a run.sh or a server.jar inside."),
		"",
		ctaStyle.Render("Press  a  to add your first server"),
		"",
		mutedStyle.Render("Dropped one into a folder Beacon watches?  Press  ctrl+r  to scan."),
	)
	return lipgloss.Place(m.bodyW, m.bodyH, lipgloss.Center, lipgloss.Center, panel)
}

func (m *model) pickerView() string {
	head := sectionStyle.Render("Add a server") + mutedStyle.Render("   "+m.pick.CurrentDirectory)
	return lipgloss.JoinVertical(lipgloss.Left, head, "", m.pick.View())
}

func (m *model) patchDialogView() string {
	p := m.pat
	prose := clampInt(m.bodyW-12, 36, 76)
	para := lipgloss.NewStyle().Width(prose).Foreground(mutedColor)
	code := lipgloss.NewStyle().Width(prose)

	scriptName := filepath.Base(p.patch.Path)
	inner := lipgloss.JoinVertical(lipgloss.Left,
		sectionStyle.Render("Let Beacon fix "+string(p.id)+"'s start script?"),
		"",
		para.Render("Beacon runs each server inside tmux and needs the Java process to be "+
			"the script itself, not a program the script starts and waits on. As a "+
			"child process, a \"stop\" typed to the server may not reach it, and Beacon "+
			"cannot tell whether the server is really up."),
		"",
		para.Render("The fix adds exec in front of the java line in "+scriptName+", so the "+
			"shell hands off to Java. Your original is copied to "+scriptName+".bak first."),
		"",
		sectionStyle.Render("The change  ")+mutedStyle.Render(fmt.Sprintf("%s line %d", scriptName, p.patch.Line)),
		code.Foreground(warnColor).Render("- "+p.patch.Old),
		code.Foreground(runColor).Render("+ "+p.patch.New),
		"",
		m.hintBar(hint("y", "apply the fix"), hint("esc", "leave it unchanged")),
	)
	return lipgloss.Place(m.bodyW, m.bodyH, lipgloss.Center, lipgloss.Center, dialogStyle.Render(inner))
}

func (m *model) launchDialogView() string {
	lp := m.launch
	width := clampInt(m.bodyW-12, 40, 72)

	rows := []string{
		sectionStyle.Render("How should Beacon start " + string(lp.id) + "?"),
		mutedStyle.Render("Every launch method Beacon found in its folder:"),
		"",
	}
	for i, o := range lp.opts {
		marker := "  "
		label := o.Label
		if i == lp.cursor {
			marker = "▸ "
			label = selectedRow.Render(label)
		}
		note := o.Base
		if o.Script != "" && !o.Exec.Launchable() {
			note += "   (not exec java; press p after saving to try to fix it)"
		}
		rows = append(rows, marker+label, mutedStyle.Render("    "+note))
	}
	rows = append(rows,
		"",
		lp.args.View(),
		"",
		m.hintBar(hint("↑↓", "choose"), hint("enter", "save"), hint("esc", "cancel")),
	)
	inner := lipgloss.NewStyle().Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	return lipgloss.Place(m.bodyW, m.bodyH, lipgloss.Center, lipgloss.Center, dialogStyle.Render(inner))
}
