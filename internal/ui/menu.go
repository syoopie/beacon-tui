package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/syoopie/beacon-tui/internal/server"
)

// screen is the top-level view. The list is the home screen; picking a server
// opens its menu, and the console opens from there as its own full screen.
type screen int

const (
	screenList screen = iota
	screenMenu
	screenConsole
)

// menuAction is what a server-menu row does when chosen.
type menuAction int

const (
	actConsole menuAction = iota
	actStart
	actStop
	actForceKill
	actMarkStopped
	actLaunch
	actPatch
)

type menuRow struct {
	label string
	act   menuAction
}

// menuRows is the selected server's menu: the console, then the actions its
// status allows, then launch settings.
func (m *model) menuRows() []menuRow {
	spec, ok := m.selected()
	if !ok {
		return nil
	}
	rows := []menuRow{{"Open console", actConsole}}
	switch m.reports[spec.ID].Derived {
	case server.StatusStopped:
		rows = append(rows, menuRow{"Start", actStart})
	case server.StatusRunning, server.StatusStarting, server.StatusStopping:
		rows = append(rows, menuRow{"Stop", actStop})
	case server.StatusUnknown:
		rows = append(rows, menuRow{"Mark stopped", actMarkStopped})
	}
	if m.timedOut[spec.ID] {
		rows = append(rows, menuRow{"Force-kill", actForceKill})
	}
	if !spec.Exec.Launchable() {
		rows = append(rows, menuRow{"Fix start script", actPatch})
	}
	rows = append(rows, menuRow{"Launch settings", actLaunch})
	return rows
}

func (m *model) clampMenuCursor() {
	if n := len(m.menuRows()); m.menuCursor >= n {
		m.menuCursor = max(n-1, 0)
	}
}

func (m *model) handleMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.clampMenuCursor()
	rows := m.menuRows()
	switch {
	case key.Matches(msg, m.keys.Back):
		m.screen = screenList
		m.relayout()
		return m, nil
	case key.Matches(msg, m.keys.Up):
		if m.menuCursor > 0 {
			m.menuCursor--
		}
		return m, nil
	case key.Matches(msg, m.keys.Down):
		if m.menuCursor < len(rows)-1 {
			m.menuCursor++
		}
		return m, nil
	case key.Matches(msg, m.keys.Enter):
		if m.menuCursor < len(rows) {
			return m, m.runMenuAction(rows[m.menuCursor].act)
		}
	}
	return m, nil
}

// runMenuAction fires the chosen menu row. Actions that mutate a server take the
// busy path; opening the console or a modal does not.
func (m *model) runMenuAction(act menuAction) tea.Cmd {
	spec, ok := m.selected()
	if !ok {
		return nil
	}
	switch act {
	case actConsole:
		m.openConsoleScreen()
		return nil
	case actLaunch:
		return m.openLaunch(spec)
	case actPatch:
		return m.planPatchCmd(spec)
	}

	if m.busy {
		m.status = busyStatus
		return nil
	}
	m.busy = true
	switch act {
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

// detailView is the right column: the selected server's status and the actions
// that apply to it. It is always on screen beside the list; screenMenu focus
// only adds the row cursor and lets enter fire a row.
func (m *model) detailView() string {
	spec, ok := m.selected()
	if !ok {
		return mutedStyle.Render("Select a server on the left.")
	}
	r := m.reports[spec.ID]
	focused := m.screen == screenMenu

	head := lipgloss.JoinVertical(lipgloss.Left,
		sectionStyle.Render(string(spec.ID)),
		lipgloss.NewStyle().Foreground(statusColor(r.Derived)).Render(statusGlyph(r.Derived)+" "+r.Derived.String())+
			mutedStyle.Render(fmt.Sprintf("   port %d", spec.Port)),
		mutedStyle.Render("via "+launchSummary(spec)),
	)

	rows := make([]string, 0, len(m.menuRows()))
	for i, row := range m.menuRows() {
		marker := "  "
		label := row.label
		if focused && i == m.menuCursor {
			marker = "▸ "
			label = selectedRow.Render(label)
		}
		rows = append(rows, marker+label)
	}
	actions := lipgloss.JoinVertical(lipgloss.Left, rows...)
	if !focused {
		actions = mutedStyle.Render(actions) + "\n\n" + mutedStyle.Render("→  act on this server")
	}
	return lipgloss.JoinVertical(lipgloss.Left, head, "", actions)
}
