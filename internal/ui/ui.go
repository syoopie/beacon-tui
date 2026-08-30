// Package ui is beacon's Bubble Tea front end: a server list with derived
// status, a live log view for the selected server, and start/stop/force-kill.
// It holds no authority. The registry on disk is the source of truth, and every
// mutating key routes through internal/lifecycle and its host lock.
package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/importdetect"
	"github.com/syoopie/beacon-tui/internal/lifecycle"
	"github.com/syoopie/beacon-tui/internal/reconcile"
	"github.com/syoopie/beacon-tui/internal/selfupdate"
	"github.com/syoopie/beacon-tui/internal/server"
	"github.com/syoopie/beacon-tui/internal/supervisor"
)

// App is the wiring the UI needs, assembled by the caller.
type App struct {
	Dirs    config.Dirs
	Cfg     config.Config
	Sup     supervisor.Supervisor
	Mgr     *lifecycle.Manager
	Version string // running build, for the update check
	Repo    string // "owner/name", for the update check and its install command
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
	update *updateNotice
	// pick is non-nil while the operator is browsing for a server folder.
	pick *filepicker.Model
	// console is non-nil while the operator is typing a command for the
	// selected server. Key input goes to it, not the key handler.
	console *textinput.Model
}

type updateNotice struct {
	latest  string
	command string
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
	return tea.Batch(m.reloadCmd(), tick(), m.updateCheckCmd())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.pick != nil {
		return m.updatePicker(msg)
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if !m.ready {
			m.vp = viewport.New(msg.Width-listWidth-1, msg.Height-2)
			m.ready = true
		}
		m.layout()
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

	case rootAddedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "add folder: " + msg.err.Error()
			return m, nil
		}
		m.app.Cfg = msg.cfg
		m.busy = true
		m.status = "scanning…"
		return m, m.importCmd()

	case updateMsg:
		if msg.err == nil && msg.res.Available {
			m.update = &updateNotice{latest: msg.res.Latest, command: selfupdate.UpdateCommand(m.app.Repo)}
			if !m.busy {
				m.status = "beacon " + msg.res.Latest + " is out — run: " + m.update.command
			}
		}
		return m, nil

	case consoleSentMsg:
		m.status = msg.label
		return m, nil

	case tea.KeyMsg:
		return m.onKey(msg)
	}
	// Cursor-blink and other component ticks reach the focused console here.
	if m.console != nil {
		ti, cmd := m.console.Update(msg)
		m.console = &ti
		return m, cmd
	}
	return m, nil
}

// layout sizes the log viewport for the current chrome. The console bar, when
// open, takes one row from the log.
func (m *model) layout() {
	if !m.ready {
		return
	}
	h := m.height - 2
	if m.console != nil {
		h--
	}
	m.vp.Width = max(m.width-listWidth-1, 1)
	m.vp.Height = max(h, 1)
}

// openPicker starts the folder browser at the operator's home directory.
func (m *model) openPicker() tea.Cmd {
	fp := filepicker.New()
	fp.DirAllowed = true
	fp.FileAllowed = false
	if home, err := os.UserHomeDir(); err == nil {
		fp.CurrentDirectory = home
	}
	fp.AutoHeight = false
	fp.SetHeight(max(m.height-4, 5))
	m.pick = &fp
	m.status = "→ open folder · ← up · enter choose this folder · esc cancel"
	return fp.Init()
}

// openConsole focuses a one-line input that sends commands to the selected
// server's console. It stays open after a send so the operator can type again.
func (m *model) openConsole(spec server.Spec) tea.Cmd {
	ti := textinput.New()
	ti.Prompt = string(spec.ID) + " › "
	ti.Placeholder = "server command, e.g. list"
	ti.CharLimit = 512
	ti.Width = max(m.width-listWidth-len(ti.Prompt)-4, 20)
	ti.Focus()
	m.console = &ti
	m.status = "console · enter send · esc close"
	m.layout()
	return textinput.Blink
}

func (m *model) closeConsole(reason string) (tea.Model, tea.Cmd) {
	m.console = nil
	m.status = reason
	m.layout()
	return m, nil
}

func (m *model) updateConsole(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		return m.closeConsole("console closed")
	case "enter":
		line := strings.TrimSpace(m.console.Value())
		if line == "" {
			return m, nil
		}
		spec, ok := m.selected()
		if !ok {
			return m.closeConsole("console closed (no server selected)")
		}
		m.console.SetValue("")
		return m, m.consoleCmd(spec, line)
	}
	ti, cmd := m.console.Update(msg)
	m.console = &ti
	return m, cmd
}

func (m *model) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && (k.String() == "esc" || k.String() == "ctrl+c") {
		m.pick = nil
		m.status = "add cancelled"
		return m, nil
	}
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.width, m.height = sz.Width, sz.Height
		m.pick.SetHeight(max(sz.Height-4, 5))
	}

	fp, cmd := m.pick.Update(msg)
	m.pick = &fp

	if ok, path := fp.DidSelectFile(msg); ok {
		m.pick = nil
		m.busy = true
		m.status = "adding " + path + "…"
		return m, m.addRootCmd(path)
	}
	if ok, path := fp.DidSelectDisabledFile(msg); ok {
		m.status = path + " can't be added (not a folder)"
		return m, nil
	}
	return m, cmd
}

func (m *model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.console != nil {
		return m.updateConsole(msg)
	}
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
	case "a":
		return m, m.openPicker()
	case "u":
		if m.update != nil {
			m.status = "update: " + m.update.command
			m.update = nil
			return m, nil
		}
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
	case "c":
		if m.reports[spec.ID].Derived != server.StatusRunning {
			m.status = "console works only while the server is running"
			return m, nil
		}
		return m, m.openConsole(spec)
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
	m.vp.SetContent(strings.Join(cur, "\n"))
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
		if m.status == "loading…" || m.status == "scanning…" {
			if len(m.app.Cfg.ScanRoots) == 0 {
				m.status = "no servers yet — press a to add the folder your servers live in"
			} else {
				m.status = "no servers found under " + fmt.Sprint(m.app.Cfg.ScanRoots) + " — press a to add another folder, i to re-scan"
			}
		}
		return nil
	}
	if _, ok := m.selected(); !ok {
		m.point(m.specs[0].ID)
	}
	if m.status == "loading…" || m.status == "scanning…" {
		m.status = "j/k move · s start · x stop · c console · K force-kill · m mark-stopped · a add-folder · i import · p patch · q quit"
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

type updateMsg struct {
	res selfupdate.Result
	err error
}

type rootAddedMsg struct {
	cfg config.Config
	err error
}

type consoleSentMsg struct{ label string }

func (m *model) consoleCmd(spec server.Spec, line string) tea.Cmd {
	mgr := m.app.Mgr
	return func() tea.Msg {
		if err := mgr.SendConsole(context.Background(), spec, line); err != nil {
			return consoleSentMsg{label: "console: " + err.Error()}
		}
		return consoleSentMsg{label: string(spec.ID) + " ‹ " + line}
	}
}

func (m *model) addRootCmd(dir string) tea.Cmd {
	dirs := m.app.Dirs
	return func() tea.Msg {
		cfg, err := config.AddScanRoot(dirs, dir)
		return rootAddedMsg{cfg: cfg, err: err}
	}
}

func (m *model) updateCheckCmd() tea.Cmd {
	repo, version := m.app.Repo, m.app.Version
	if repo == "" {
		return nil
	}
	return func() tea.Msg {
		res, err := selfupdate.Check(context.Background(), repo, version)
		return updateMsg{res: res, err: err}
	}
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
	titleStyle   = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	updateStyle  = lipgloss.NewStyle().Bold(true)
	listStyle    = lipgloss.NewStyle().Width(listWidth).Border(lipgloss.NormalBorder(), false, true, false, false)
	selStyle     = lipgloss.NewStyle().Bold(true).Reverse(true)
	statusStyle  = lipgloss.NewStyle().Padding(0, 1)
	consoleStyle = lipgloss.NewStyle().Padding(0, 1)
	warnStyle    = lipgloss.NewStyle().Bold(true)
	dialogStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

func (m *model) View() string {
	if !m.ready {
		return "starting beacon…"
	}
	if m.pat != nil {
		return m.dialogView()
	}

	name := "beacon"
	if m.update != nil {
		name += updateStyle.Render("   ⬆ " + m.update.latest + " available (press u for the command)")
	}
	title := titleStyle.Render(name)

	var body string
	if m.pick != nil {
		body = titleStyle.Render("add a server — "+m.pick.CurrentDirectory) + "\n" + m.pick.View()
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, listStyle.Render(m.listView()), m.vp.View())
	}

	rows := []string{title, body}
	if m.console != nil {
		rows = append(rows, consoleStyle.Render(m.console.View()))
	}
	rows = append(rows, statusStyle.Render(m.statusLine()))
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
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
	return strings.Join(lines, "\n")
}

func (m *model) statusLine() string {
	if m.pick != nil || m.console != nil {
		return m.status
	}
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
