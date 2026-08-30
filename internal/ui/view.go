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
	case m.pick != nil:
		return helpSet{short: []key.Binding{
			hint("→", "open folder"), hint("←", "up a level"),
			hint("enter", "choose this folder"), hint("esc", "cancel"),
		}}
	case m.pat != nil:
		return helpSet{short: []key.Binding{hint("y", "apply fix"), hint("esc", "leave unchanged")}}
	case m.list.FilterState() == list.Filtering:
		return helpSet{short: []key.Binding{hint("enter", "apply filter"), hint("esc", "clear")}}
	}

	full := [][]key.Binding{
		{m.keys.Up, m.keys.Down, hint("/", "filter")},
		{m.keys.Start, m.keys.Stop, m.keys.Console, m.keys.Kill, m.keys.MarkStopped, m.keys.Patch},
		{m.keys.Add, m.keys.Rescan, m.keys.Refresh, m.keys.Update},
		{m.keys.Help, m.keys.Quit},
	}
	return helpSet{short: m.contextBindings(), full: full}
}

func (m *model) contextBindings() []key.Binding {
	if len(m.specs) == 0 {
		return []key.Binding{m.keys.Add, m.keys.Refresh, m.keys.Quit}
	}
	b := []key.Binding{m.keys.Up, m.keys.Down}
	if spec, ok := m.selected(); ok {
		switch m.reports[spec.ID].Derived {
		case server.StatusStopped:
			b = append(b, m.keys.Start)
		case server.StatusRunning:
			b = append(b, m.keys.Stop, m.keys.Console)
		case server.StatusStarting, server.StatusStopping:
			b = append(b, m.keys.Stop)
		case server.StatusUnknown:
			b = append(b, m.keys.MarkStopped)
		}
		if m.timedOut[spec.ID] {
			b = append(b, m.keys.Kill)
		}
		if !spec.Exec.Launchable() {
			b = append(b, m.keys.Patch)
		}
	}
	b = append(b, m.keys.Add)
	if m.update != nil {
		b = append(b, m.keys.Update)
	}
	b = append(b, m.keys.Help, m.keys.Quit)
	return b
}

// --- view ---

func (m *model) View() string {
	if !m.ready {
		return "starting beacon…"
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
	if m.console != nil {
		rows = append(rows, "", consoleBarStyle.Render(m.console.View()))
	}
	rows = append(rows, "", m.statusView())
	return frameStyle.Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m *model) headerView() string {
	left := brandStyle.Render("beacon")
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
	if m.pat != nil || m.pick != nil {
		return ""
	}
	spec, ok := m.selected()
	if !ok {
		return ""
	}
	r := m.reports[spec.ID]
	switch {
	case r.Derived == server.StatusUnknown && r.Warning != "":
		return "⚠  " + r.Warning + "  Once you have checked, press m to mark it stopped."
	case !spec.Exec.Launchable():
		return "⚠  " + string(spec.ID) + " has a start script beacon can't stop cleanly yet. Press p to review and apply the fix."
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
	case m.pick != nil:
		content = m.pickerView()
	case m.loaded && len(m.specs) == 0:
		content = m.landingView()
	default:
		content = m.paneView()
	}
	return lipgloss.NewStyle().MaxWidth(m.bodyW).
		Height(m.bodyH).MaxHeight(m.bodyH).Render(content)
}

func (m *model) paneView() string {
	rightW := max(m.vp.Width, 1)
	right := lipgloss.NewStyle().MaxWidth(rightW).Render(lipgloss.JoinVertical(lipgloss.Left,
		m.logHeaderView(),
		mutedStyle.Render(strings.Repeat("─", rightW)),
		m.vp.View(),
	))
	divider := mutedStyle.Render(strings.TrimSuffix(strings.Repeat("│\n", max(m.bodyH, 1)), "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, m.list.View(), " ", divider, "  ", right)
}

// logHeaderView is one line: name, status, port. Anything that needs more room
// (warnings, script problems) goes to the notice banner instead.
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
	}, mutedStyle.Render("   ·   "))
}

func (m *model) landingView() string {
	panel := lipgloss.JoinVertical(lipgloss.Center,
		brandStyle.Render("beacon"),
		mutedStyle.Render("Run your Minecraft servers from one screen."),
		"",
		lipgloss.NewStyle().Foreground(mutedColor).Align(lipgloss.Center).Render(
			"You have no servers yet.\n\n"+
				"beacon needs the folder your server lives in —\n"+
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
		sectionStyle.Render("Let beacon fix "+string(p.id)+"'s start script?"),
		"",
		para.Render("beacon runs each server inside tmux and needs the Java process to be "+
			"the script itself, not a program the script starts and waits on. As a "+
			"child process, a \"stop\" typed to the server may not reach it, and beacon "+
			"cannot tell whether the server is really up."),
		"",
		para.Render("The fix adds exec in front of the java line in "+scriptName+", so the "+
			"shell hands off to Java. Your original is copied to "+scriptName+".bak first."),
		"",
		sectionStyle.Render("The change  ")+mutedStyle.Render(fmt.Sprintf("%s line %d", scriptName, p.patch.Line)),
		code.Foreground(warnColor).Render("- "+p.patch.Old),
		code.Foreground(runColor).Render("+ "+p.patch.New),
		"",
		mutedStyle.Render("y  apply the fix        esc  leave it unchanged"),
	)
	return lipgloss.Place(m.bodyW, m.bodyH, lipgloss.Center, lipgloss.Center, dialogStyle.Render(inner))
}
