// Package ui is beacon's Bubble Tea front end: a server list with derived
// status, a live log view for the selected server, and start/stop/force-kill.
// It holds no authority. The registry on disk is the source of truth, and every
// mutating key routes through internal/lifecycle and its host lock.
package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sunyupei/beacon-tui/internal/config"
	"github.com/sunyupei/beacon-tui/internal/importdetect"
	"github.com/sunyupei/beacon-tui/internal/lifecycle"
	"github.com/sunyupei/beacon-tui/internal/reconcile"
	"github.com/sunyupei/beacon-tui/internal/server"
	"github.com/sunyupei/beacon-tui/internal/supervisor"
)

// App is the wiring the UI needs, assembled by the caller.
type App struct {
	Dirs config.Dirs
	Cfg  config.Config
	Sup  supervisor.Supervisor
	Mgr  *lifecycle.Manager
}

// Run starts the TUI and blocks until the operator quits.
func Run(app App) error {
	_, err := tea.NewProgram(newModel(app), tea.WithAltScreen()).Run()
	return err
}

const (
	refreshEvery = time.Second
	maxLogLines  = 5000
	listWidth    = 26
)

type model struct {
	app     App
	specs   []server.Spec
	reports map[server.ID]reconcile.Report
	// timedOut holds servers whose last Stop hit the timeout, so the UI can
	// offer force-kill without the status machine gaining a state for it.
	timedOut map[server.ID]bool

	selID server.ID
	tail  *logFollower

	vp            viewport.Model
	ready         bool
	width, height int

	busy   bool
	status string
	pat    *patchPrompt
}

func newModel(app App) *model {
	return &model{
		app:      app,
		reports:  map[server.ID]reconcile.Report{},
		timedOut: map[server.ID]bool{},
		status:   "loading…",
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.reloadCmd(), tick())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		bodyHeight := msg.Height - 2
		if !m.ready {
			m.vp = viewport.New(msg.Width-listWidth-1, bodyHeight)
			m.ready = true
		} else {
			m.vp.Width = msg.Width - listWidth - 1
			m.vp.Height = bodyHeight
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.reloadCmd(), m.tailCmd(), tick())

	case reloadedMsg:
		return m, m.applyReload(msg)

	case reconciledMsg:
		if msg.err != nil {
			m.status = "reconcile: " + msg.err.Error()
			return m, nil
		}
		m.reports = msg.reports
		return m, nil

	case logMsg:
		if msg.id == m.selID && len(msg.lines) > 0 {
			atBottom := m.vp.AtBottom()
			m.appendLogs(msg.lines)
			if atBottom {
				m.vp.GotoBottom()
			}
		}
		return m, nil

	case opDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.status = msg.label + ": " + msg.err.Error()
		} else {
			m.status = msg.label
		}
		if msg.timedOut {
			m.timedOut[msg.id] = true
		} else {
			delete(m.timedOut, msg.id)
		}
		return m, m.reloadCmd()

	case patchPlannedMsg:
		if msg.err != nil {
			m.status = "patch: " + msg.err.Error()
			return m, nil
		}
		if !msg.needed {
			m.status = string(msg.id) + ": start script already execs"
			return m, nil
		}
		m.pat = &patchPrompt{id: msg.id, patch: msg.patch}
		return m, nil

	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m *model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pat != nil {
		switch msg.String() {
		case "y":
			p := m.pat.patch
			m.pat = nil
			m.busy = true
			m.status = "patching…"
			return m, m.applyPatchCmd(p)
		case "n", "esc", "q":
			m.pat = nil
			m.status = "patch cancelled"
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "r":
		return m, m.reloadCmd()
	case "i":
		m.busy = true
		m.status = "scanning…"
		return m, m.importCmd()
	}

	if m.busy {
		m.status = "an operation is already running"
		return m, nil
	}
	spec, ok := m.selected()
	if !ok {
		return m, nil
	}
	switch msg.String() {
	case "s":
		m.busy = true
		m.status = "starting " + string(spec.ID) + "…"
		return m, m.startCmd(spec)
	case "x":
		m.busy = true
		m.status = "stopping " + string(spec.ID) + "… (up to " + m.app.Cfg.StopTimeout.Std().String() + ")"
		return m, m.stopCmd(spec)
	case "K":
		if !m.timedOut[spec.ID] {
			m.status = "force-kill is offered only after a stop times out"
			return m, nil
		}
		m.busy = true
		m.status = "force-killing " + string(spec.ID) + "…"
		return m, m.forceKillCmd(spec)
	case "m":
		if m.reports[spec.ID].Derived != server.StatusUnknown {
			m.status = "mark-stopped applies only to a server in Unknown"
			return m, nil
		}
		m.busy = true
		m.status = "marking " + string(spec.ID) + " stopped…"
		return m, m.markStoppedCmd(spec)
	case "p":
		return m, m.planPatchCmd(spec)
	}
	return m, nil
}

func (m *model) move(delta int) {
	if len(m.specs) == 0 {
		return
	}
	i := m.indexOf(m.selID) + delta
	if i < 0 {
		i = 0
	}
	if i >= len(m.specs) {
		i = len(m.specs) - 1
	}
	m.point(m.specs[i].ID)
}

func (m *model) indexOf(id server.ID) int {
	for i, s := range m.specs {
		if s.ID == id {
			return i
		}
	}
	return 0
}

func (m *model) selected() (server.Spec, bool) {
	for _, s := range m.specs {
		if s.ID == m.selID {
			return s, true
		}
	}
	return server.Spec{}, false
}

// point moves the log view to a different server, resetting the follower and
// the buffered lines.
func (m *model) point(id server.ID) {
	if id == m.selID && m.tail != nil {
		return
	}
	m.selID = id
	spec, ok := m.selected()
	if !ok {
		m.tail = nil
		return
	}
	m.tail = newFollower(spec.LogFile)
	m.vp.SetContent("")
	m.vp.GotoTop()
}

func (m *model) appendLogs(lines []string) {
	cur := m.tail.append(lines, maxLogLines)
	m.vp.SetContent(joinLines(cur))
}

func (m *model) applyReload(msg reloadedMsg) tea.Cmd {
	if msg.err != nil {
		m.status = "registry: " + msg.err.Error()
		return nil
	}
	m.specs = msg.specs
	if len(m.specs) == 0 {
		m.selID = ""
		m.tail = nil
		m.status = "no servers imported yet — press i to scan " + fmt.Sprint(m.app.Cfg.ScanRoots)
		return nil
	}
	if _, ok := m.selected(); !ok {
		m.point(m.specs[0].ID)
	}
	if m.status == "loading…" {
		m.status = "j/k move · s start · x stop · K force-kill · m mark-stopped · i import · p patch · q quit"
	}
	return m.reconcileCmd()
}

// --- messages and commands ---

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type reloadedMsg struct {
	specs []server.Spec
	err   error
}

type reconciledMsg struct {
	reports map[server.ID]reconcile.Report
	err     error
}

type logMsg struct {
	id    server.ID
	lines []string
}

type opDoneMsg struct {
	id       server.ID
	label    string
	timedOut bool
	err      error
}

type patchPlannedMsg struct {
	id     server.ID
	patch  importdetect.Patch
	needed bool
	err    error
}

func (m *model) reloadCmd() tea.Cmd {
	dirs := m.app.Dirs
	return func() tea.Msg {
		specs, err := config.LoadSpecs(dirs)
		return reloadedMsg{specs: specs, err: err}
	}
}

func (m *model) reconcileCmd() tea.Cmd {
	sup, specs := m.app.Sup, m.specs
	return func() tea.Msg {
		reports, err := reconcile.Run(context.Background(), sup, specs)
		if err != nil {
			return reconciledMsg{err: err}
		}
		byID := make(map[server.ID]reconcile.Report, len(reports))
		for _, r := range reports {
			byID[r.ID] = r
		}
		return reconciledMsg{reports: byID}
	}
}

func (m *model) tailCmd() tea.Cmd {
	if m.tail == nil {
		return nil
	}
	f, id := m.tail, m.selID
	return func() tea.Msg {
		lines, err := f.read()
		if err != nil {
			return logMsg{id: id, lines: []string{"[log error] " + err.Error()}}
		}
		return logMsg{id: id, lines: lines}
	}
}

func (m *model) startCmd(spec server.Spec) tea.Cmd {
	mgr, all := m.app.Mgr, m.specs
	return func() tea.Msg {
		_, err := mgr.Start(context.Background(), spec, all)
		label := string(spec.ID) + " started"
		if err != nil {
			label = "start " + string(spec.ID)
		}
		return opDoneMsg{id: spec.ID, label: label, err: err}
	}
}

func (m *model) stopCmd(spec server.Spec) tea.Cmd {
	mgr := m.app.Mgr
	return func() tea.Msg {
		_, outcome, err := mgr.Stop(context.Background(), spec)
		switch {
		case err != nil:
			return opDoneMsg{id: spec.ID, label: "stop " + string(spec.ID), err: err}
		case outcome.TimedOut:
			return opDoneMsg{id: spec.ID, label: string(spec.ID) + " did not stop in time — press K to force-kill", timedOut: true}
		default:
			return opDoneMsg{id: spec.ID, label: string(spec.ID) + " stopped"}
		}
	}
}

func (m *model) forceKillCmd(spec server.Spec) tea.Cmd {
	mgr := m.app.Mgr
	return func() tea.Msg {
		_, err := mgr.ForceKill(context.Background(), spec)
		label := string(spec.ID) + " force-killed"
		if err != nil {
			label = "force-kill " + string(spec.ID)
		}
		return opDoneMsg{id: spec.ID, label: label, err: err}
	}
}

func (m *model) markStoppedCmd(spec server.Spec) tea.Cmd {
	mgr := m.app.Mgr
	return func() tea.Msg {
		_, err := mgr.MarkStopped(spec)
		label := string(spec.ID) + " marked stopped"
		if err != nil {
			label = "mark-stopped " + string(spec.ID)
		}
		return opDoneMsg{id: spec.ID, label: label, err: err}
	}
}

// --- view ---

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	listStyle   = lipgloss.NewStyle().Width(listWidth).Border(lipgloss.NormalBorder(), false, true, false, false)
	selStyle    = lipgloss.NewStyle().Bold(true).Reverse(true)
	statusStyle = lipgloss.NewStyle().Padding(0, 1)
	warnStyle   = lipgloss.NewStyle().Bold(true)
	dialogStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

func (m *model) View() string {
	if !m.ready {
		return "starting beacon…"
	}
	if m.pat != nil {
		return m.dialogView()
	}

	title := titleStyle.Render("beacon")
	body := lipgloss.JoinHorizontal(lipgloss.Top, listStyle.Render(m.listView()), m.vp.View())
	status := statusStyle.Render(m.statusLine())
	return lipgloss.JoinVertical(lipgloss.Left, title, body, status)
}

func (m *model) listView() string {
	if len(m.specs) == 0 {
		return "no servers"
	}
	lines := make([]string, 0, len(m.specs))
	for _, s := range m.specs {
		st := m.reports[s.ID].Derived
		row := fmt.Sprintf("%s %-*s %s", statusGlyph(st), listWidth-12, truncate(string(s.ID), listWidth-12), short(st))
		if s.ID == m.selID {
			row = selStyle.Render(row)
		}
		lines = append(lines, row)
	}
	return joinLines(lines)
}

func (m *model) statusLine() string {
	spec, ok := m.selected()
	if !ok {
		return m.status
	}
	r := m.reports[spec.ID]
	line := fmt.Sprintf("%s  [%s]  port %d", spec.ID, r.Derived, spec.Port)
	if !spec.Exec.Launchable() {
		line += "  script needs `exec` (press p)"
	}
	if r.Warning != "" {
		line += "  " + warnStyle.Render("⚠ "+r.Warning)
	}
	return line + "\n" + m.status
}

func (m *model) dialogView() string {
	p := m.pat
	content := fmt.Sprintf("Patch %s so its last line execs?\n\n%s\n\ny apply · n cancel", p.id, p.patch.Diff())
	return dialogStyle.Render(content)
}

func statusGlyph(s server.Status) string {
	switch s {
	case server.StatusRunning:
		return "▶"
	case server.StatusStarting, server.StatusStopping:
		return "…"
	case server.StatusUnknown:
		return "?"
	default:
		return "○"
	}
}

func short(s server.Status) string { return s.String() }

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	return s[:n-1] + "…"
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
