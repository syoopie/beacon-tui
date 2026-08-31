package ui

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

type serverItem struct {
	spec   server.Spec
	status server.Status
	warn   string
}

func (i serverItem) FilterValue() string { return string(i.spec.ID) }

type serverDelegate struct{}

func (serverDelegate) Height() int                         { return 1 }
func (serverDelegate) Spacing() int                        { return 0 }
func (serverDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (serverDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	si, ok := item.(serverItem)
	if !ok {
		return
	}
	width := m.Width()
	if width < 12 {
		width = 12
	}
	selected := index == m.Index()

	marker := "  "
	if selected {
		marker = "▸ "
	}
	glyph := statusGlyph(si.status) + " "
	status := si.status.String()

	// Give the name whatever is left after the marker, glyph, one gap, and the
	// status word, then ellipsize it rather than letting the row overflow.
	nameW := width - lipgloss.Width(marker) - lipgloss.Width(glyph) - lipgloss.Width(status) - 1
	name := truncateName(string(si.spec.ID), nameW)

	gap := width - lipgloss.Width(marker) - lipgloss.Width(glyph) - lipgloss.Width(name) - lipgloss.Width(status)
	if gap < 1 {
		gap = 1
	}
	color := lipgloss.NewStyle().Foreground(statusColor(si.status))
	rendered := markerStyle.Render(marker) + color.Render(glyph) + name +
		strings.Repeat(" ", gap) + color.Render(status)
	if selected {
		rendered = selectedRow.Render(rendered)
	}
	_, _ = fmt.Fprint(w, lipgloss.NewStyle().MaxWidth(width).Render(rendered))
}

func truncateName(s string, w int) string {
	if w < 1 {
		return ""
	}
	if len(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return s[:w-1] + "…"
}

// --- the command bar ---

// commandBar picks the bindings shown at the top of the screen. It is the
// user's answer to "what can I do right now", so it tracks the mode and the
// selected server's status rather than listing every key all the time.
func (m *model) commandBar() helpSet {
	switch {
	case m.console != nil:
		return helpSet{short: []key.Binding{hint("enter", "send"), hint("esc", "close console")}}
	case m.logSearch != nil:
		return helpSet{short: []key.Binding{hint("enter", "keep filter"), hint("esc", "clear search")}}
	case m.actions != nil:
		return helpSet{short: []key.Binding{hint("↑↓", "move"), hint("enter", "run"), hint("esc", "close")}}
	case m.pick != nil:
		return helpSet{short: []key.Binding{
			hint("→", "open folder"), hint("←", "up a level"),
			hint("enter", "choose this folder"), hint("esc", "cancel"),
		}}
	case m.pat != nil, m.launch != nil, m.config != nil:
		// These are centred dialogs that carry their own key hints in a footer,
		// right under the fields. Repeating them up here just adds noise.
		return helpSet{}
	case m.list.FilterState() == list.Filtering:
		return helpSet{short: []key.Binding{hint("enter", "apply filter"), hint("esc", "clear")}}
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
		return helpSet{short: []key.Binding{m.keys.Add, m.keys.Refresh, m.keys.Quit}}
	}
	short := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Act, hint("/", "filter"), m.keys.Add}
	if m.update != nil {
		short = append(short, m.keys.Update)
	}
	short = append(short, m.keys.Help, m.keys.Quit)
	full := [][]key.Binding{
		{m.keys.Up, m.keys.Down, m.keys.Act, hint("/", "filter")},
		{m.keys.Add, m.keys.Rescan, m.keys.Refresh, m.keys.Update},
		{m.keys.Help, m.keys.Quit},
	}
	return helpSet{short: short, full: full}
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
		rows = append(rows, "", consoleBarStyle.Render(m.console.View()))
	case m.logSearch != nil:
		rows = append(rows, "", consoleBarStyle.Render(m.logSearch.View()))
	}
	rows = append(rows, "", m.statusView())
	return frameStyle.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m *model) headerView() string {
	left := brandStyle.Render("Beacon")
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

// listView is the home screen: a full-width, view-only list of servers. Every
// action is a step in from here, on the console.
func (m *model) listView() string {
	return lipgloss.NewStyle().Width(m.bodyW).MaxWidth(m.bodyW).Render(m.list.View())
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
		mutedStyle.Render("Added one already?  Press  r  to refresh."),
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
