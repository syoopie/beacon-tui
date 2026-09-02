package ui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/syoopie/beacon-tui/internal/server"
)

// screen is the top-level view. The list is the home screen and view only;
// picking a server opens its console, which is where every action lives.
type screen int

const (
	screenList screen = iota
	screenConsole
)

// menuAction is what an action does when run, whether it came from the s / K
// keys or the console's actions overlay.
type menuAction int

const (
	actStart menuAction = iota
	actStop
	actForceKill
	actMarkStopped
	actEditConfig
	actAcceptEULA
	actLaunch
	actPatch
)

type menuRow struct {
	label string
	act   menuAction
}

// primaryAction is what the s key does for a server in the given state: start a
// stopped one, stop a live one, mark a vanished one stopped.
func (m *model) primaryAction(s server.Status) (menuAction, bool) {
	switch s {
	case server.StatusStopped:
		return actStart, true
	case server.StatusRunning, server.StatusStarting, server.StatusStopping:
		return actStop, true
	case server.StatusUnknown:
		return actMarkStopped, true
	default:
		return 0, false
	}
}

// consoleActions is the console's actions overlay: the pre-launch chores that do
// not warrant a dedicated key. Start, stop and force-kill are keys, not rows.
func (m *model) consoleActions() []menuRow {
	spec, ok := m.selected()
	if !ok {
		return nil
	}
	var rows []menuRow
	if !m.eula[spec.ID] {
		rows = append(rows, menuRow{"Accept the Minecraft EULA", actAcceptEULA})
	}
	if !spec.Exec.Launchable() {
		rows = append(rows, menuRow{"Fix start script", actPatch})
	}
	rows = append(rows, menuRow{"Edit config", actEditConfig})
	rows = append(rows, menuRow{"Launch settings", actLaunch})
	return rows
}

// actionsPrompt is the state of the console's actions overlay.
type actionsPrompt struct {
	cursor int
}

// stopPrompt is the confirm modal Beacon shows before it stops a running
// server. y goes through, esc or n backs out, and a stray key is ignored so a
// fumbled press does not send a live server down.
type stopPrompt struct {
	id server.ID
}

func (m *model) updateStopConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		m.stop = nil
		m.relayout()
		return m, m.runAction(actStop)
	case "esc", "n":
		m.stop = nil
		m.status = "stop cancelled"
		m.relayout()
		return m, nil
	}
	return m, nil
}

func (m *model) openActions() {
	if _, ok := m.selected(); !ok {
		return
	}
	m.actions = &actionsPrompt{}
	m.relayout()
}

func (m *model) clampActionsCursor() {
	if m.actions == nil {
		return
	}
	if n := len(m.consoleActions()); m.actions.cursor >= n {
		m.actions.cursor = max(n-1, 0)
	}
}

func (m *model) updateActions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.clampActionsCursor()
	rows := m.consoleActions()
	switch {
	case key.Matches(msg, m.keys.Back):
		m.actions = nil
		m.relayout()
		return m, nil
	case key.Matches(msg, m.keys.Up):
		if m.actions.cursor > 0 {
			m.actions.cursor--
		}
		return m, nil
	case key.Matches(msg, m.keys.Down):
		if m.actions.cursor < len(rows)-1 {
			m.actions.cursor++
		}
		return m, nil
	case key.Matches(msg, m.keys.Enter):
		if m.actions.cursor < len(rows) {
			return m, m.runAction(rows[m.actions.cursor].act)
		}
	}
	return m, nil
}

// runAction fires an action. Opening a modal keeps the actions overlay behind
// it, so its breadcrumb nests one level deeper and esc in the modal steps back
// to the overlay rather than the console. Every other action leaves the overlay
// and takes the busy path.
func (m *model) runAction(act menuAction) tea.Cmd {
	spec, ok := m.selected()
	if !ok {
		return nil
	}
	switch act {
	case actLaunch:
		return m.openLaunch(spec)
	case actEditConfig:
		return m.openConfig(spec)
	case actPatch:
		return m.planPatchCmd(spec)
	}

	m.actions = nil
	m.relayout()

	if m.busy {
		m.status = busyStatus
		return nil
	}
	m.busy = true
	switch act {
	case actAcceptEULA:
		m.status = "accepting the Minecraft EULA for " + string(spec.ID) + "…"
		return m.acceptEULACmd(spec)
	case actStart:
		m.status = "starting " + string(spec.ID) + "…"
		return m.startCmd(spec)
	case actStop:
		m.status = "stopping " + string(spec.ID) + "… (up to " + m.app.Cfg.StopTimeout.Std().String() + ")"
		return m.stopCmd(spec)
	case actForceKill:
		m.status = "force-killing " + string(spec.ID) + "…"
		return m.forceKillCmd(spec)
	case actMarkStopped:
		m.status = "marking " + string(spec.ID) + " stopped…"
		return m.markStoppedCmd(spec)
	}
	m.busy = false
	return nil
}

// actionsDialogView renders the console's actions overlay as a centred modal.
func (m *model) actionsDialogView() string {
	spec, ok := m.selected()
	if !ok {
		return ""
	}
	m.clampActionsCursor()
	rows := []string{sectionStyle.Render(string(spec.ID) + "  ·  settings"), ""}
	for i, row := range m.consoleActions() {
		marker := "  "
		label := row.label
		if i == m.actions.cursor {
			marker = "▸ "
			label = selectedRow.Render(label)
		}
		rows = append(rows, marker+label)
	}
	rows = append(rows, "", m.hintBar(hint("↑↓", "move"), hint("enter", "run"), hint("esc", "close")))
	inner := lipgloss.NewStyle().Width(clampInt(m.bodyW-12, 30, 52)).
		Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	return lipgloss.Place(m.bodyW, m.bodyH, lipgloss.Center, lipgloss.Center, dialogStyle.Render(inner))
}

// stopDialogView renders the stop confirm as a centred modal, in place of the
// old faint status-line prompt.
func (m *model) stopDialogView() string {
	width := clampInt(m.bodyW-12, 30, 52)
	title := lipgloss.NewStyle().Width(width).Bold(true)
	prose := lipgloss.NewStyle().Width(width).Foreground(mutedColor)
	inner := lipgloss.JoinVertical(lipgloss.Left,
		title.Render("Stop "+string(m.stop.id)+"?"),
		"",
		prose.Render("Beacon asks the server to save and shut down, then waits up to "+
			humanShortDuration(m.app.Cfg.StopTimeout.Std())+". Anyone playing is disconnected."),
		"",
		m.hintBar(hint("y", "stop it"), hint("esc", "keep it running")),
	)
	return lipgloss.Place(m.bodyW, m.bodyH, lipgloss.Center, lipgloss.Center, dialogStyle.Render(inner))
}
