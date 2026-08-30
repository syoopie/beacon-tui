// Package ui is beacon's Bubble Tea front end: a server list with derived
// status, a live log view for the selected server, and start/stop/force-kill.
// It holds no authority. The registry on disk is the source of truth, and every
// mutating key routes through internal/lifecycle and its host lock.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"os"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/importdetect"
	"github.com/syoopie/beacon-tui/internal/lifecycle"
	"github.com/syoopie/beacon-tui/internal/procstat"
	"github.com/syoopie/beacon-tui/internal/rcon"
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
	refreshEvery  = time.Second
	pollEvery     = 3 * time.Second
	maxLogLines   = 5000
	initialStatus = "loading…"
	busyStatus    = "an operation is already running"
)

type model struct {
	app      App
	specs    []server.Spec
	reports  map[server.ID]reconcile.Report
	timedOut map[server.ID]bool

	list    list.Model
	help    help.Model
	keys    keymap
	vp      viewport.Model
	tail    *logFollower
	selID   server.ID
	console *textinput.Model
	pick    *filepicker.Model
	pat     *patchPrompt
	launch  *launchPrompt
	update  *updateNotice

	screen     screen
	menuCursor int
	listW      int

	logTab    consoleTab
	logFull   bool
	logQuery  string
	logSearch *textinput.Model
	railW     int

	rconSnap     rcon.Snapshot
	rconErr      string
	rconAt       time.Time
	rconInFlight bool

	proc         procstat.Stat
	procErr      string
	procAt       time.Time
	procInFlight bool

	ready         bool
	loaded        bool
	width, height int
	bodyW, bodyH  int

	busy   bool
	status string
}

type updateNotice struct {
	latest  string
	command string
}

func newModel(app App) *model {
	l := list.New(nil, serverDelegate{}, 0, 0)
	l.Title = "Servers"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetStatusBarItemName("server", "servers")
	l.KeyMap.Quit.SetEnabled(false)
	l.KeyMap.ForceQuit.SetEnabled(false)
	// One key per action: the list's vim-style aliases (j/k, h/l, g/G) are
	// dropped in favour of the arrow and page keys.
	l.KeyMap.CursorUp = key.NewBinding(key.WithKeys("up"))
	l.KeyMap.CursorDown = key.NewBinding(key.WithKeys("down"))
	l.KeyMap.NextPage = key.NewBinding(key.WithKeys("pgdown"))
	l.KeyMap.PrevPage = key.NewBinding(key.WithKeys("pgup"))
	l.KeyMap.GoToStart = key.NewBinding(key.WithKeys("home"))
	l.KeyMap.GoToEnd = key.NewBinding(key.WithKeys("end"))
	l.Styles.Title = lipgloss.NewStyle().Bold(true)
	l.Styles.TitleBar = lipgloss.NewStyle().Padding(0, 0, 1, 0)
	l.Styles.NoItems = mutedStyle

	return &model{
		app:      app,
		reports:  map[server.ID]reconcile.Report{},
		timedOut: map[server.ID]bool{},
		status:   initialStatus,
		list:     l,
		help:     help.New(),
		keys:     newKeymap(),
	}
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.reloadCmd(), tick(), m.updateCheckCmd())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if !m.ready {
			m.vp = viewport.New(1, 1)
			m.ready = true
		}
		m.relayout()
		return m, nil

	case tickMsg:
		cmds := []tea.Cmd{m.reloadCmd(), m.tailCmd(), tick()}
		if c := m.rconPollCmd(); c != nil {
			cmds = append(cmds, c)
		}
		if c := m.procPollCmd(); c != nil {
			cmds = append(cmds, c)
		}
		return m, tea.Batch(cmds...)

	case rconMsg:
		m.rconInFlight = false
		if msg.id != m.selID {
			return m, nil
		}
		if msg.err != nil {
			m.rconErr = "can't reach RCON"
		} else {
			m.rconSnap, m.rconErr = msg.snap, ""
		}
		return m, nil

	case procMsg:
		m.procInFlight = false
		if msg.id != m.selID {
			return m, nil
		}
		if msg.err != nil {
			m.procErr = "unavailable"
		} else {
			m.proc, m.procErr = msg.stat, ""
		}
		return m, nil

	case reloadedMsg:
		return m, m.applyReload(msg)

	case reconciledMsg:
		if msg.err != nil {
			m.status = "reconcile: " + msg.err.Error()
			return m, nil
		}
		m.reports = msg.reports
		m.refreshItems()
		m.relayout() // a new warning adds the notice banner, which resizes the body
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
		} else if msg.id != "" {
			delete(m.timedOut, msg.id)
		}
		return m, m.reloadCmd()

	case patchPlannedMsg:
		if msg.err != nil {
			m.status = "patch: " + msg.err.Error()
			return m, nil
		}
		if !msg.needed {
			m.status = string(msg.id) + ": start script already runs with exec"
			return m, nil
		}
		m.pat = &patchPrompt{id: msg.id, patch: msg.patch}
		m.relayout()
		return m, nil

	case rootAddedMsg:
		m.busy = false
		if msg.err != nil {
			m.status = "add folder: " + msg.err.Error()
			return m, nil
		}
		m.app.Cfg = msg.cfg
		m.busy = true
		m.status = "scanning the folder you added…"
		return m, m.importCmd()

	case updateMsg:
		if msg.err == nil && msg.res.Available {
			m.update = &updateNotice{latest: msg.res.Latest, command: selfupdate.UpdateCommand(m.app.Repo)}
			if !m.busy {
				m.status = "Beacon " + msg.res.Latest + " is out — run: " + m.update.command
			}
			m.relayout()
		}
		return m, nil

	case consoleSentMsg:
		m.status = msg.label
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Components' own async messages: filepicker directory reads, filter
	// recomputes, cursor blink. Route to whichever owns the screen.
	if m.pick != nil {
		fp, cmd := m.pick.Update(msg)
		m.pick = &fp
		return m, cmd
	}
	if m.console != nil {
		ti, cmd := m.console.Update(msg)
		m.console = &ti
		return m, cmd
	}
	if m.launch != nil {
		ti, cmd := m.launch.args.Update(msg)
		m.launch.args = ti
		return m, cmd
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" { // the interrupt, always available, never in help
		return m, tea.Quit
	}
	switch {
	case m.console != nil:
		return m.updateConsole(msg)
	case m.logSearch != nil:
		return m.updateLogSearch(msg)
	case m.pick != nil:
		return m.updatePicker(msg)
	case m.pat != nil:
		return m.updatePatch(msg)
	case m.launch != nil:
		return m.updateLaunch(msg)
	}

	// Screen-independent keys.
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.relayout()
		return m, nil
	}

	switch m.screen {
	case screenMenu:
		return m.handleMenuKey(msg)
	case screenConsole:
		return m.handleConsoleKey(msg)
	default:
		return m.handleListKey(msg)
	}
}

func (m *model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		m.syncSelection()
		return m, cmd
	}
	switch {
	case key.Matches(msg, m.keys.Act):
		if _, ok := m.selected(); ok {
			m.screen = screenMenu
			m.menuCursor = 0
			m.relayout()
		}
		return m, nil
	case key.Matches(msg, m.keys.Add):
		return m, m.openPicker()
	case key.Matches(msg, m.keys.Rescan):
		if m.busy {
			m.status = busyStatus
			return m, nil
		}
		m.busy = true
		m.status = "scanning your folders…"
		return m, m.importCmd()
	case key.Matches(msg, m.keys.Refresh):
		m.status = "refreshing"
		return m, m.reloadCmd()
	case key.Matches(msg, m.keys.Update):
		if m.update != nil {
			m.status = "update: " + m.update.command
			m.update = nil
			m.relayout()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m.syncSelection()
	return m, cmd
}

// --- modal input: console ---

// openConsole focuses a one-line input that sends commands to the selected
// server's console. It stays open after a send so the operator can type again.
func (m *model) openConsole(spec server.Spec) tea.Cmd {
	ti := textinput.New()
	ti.Prompt = string(spec.ID) + " › "
	ti.Placeholder = "server command, e.g. list"
	ti.CharLimit = 512
	ti.Focus()
	m.console = &ti
	m.status = "console open"
	m.relayout()
	return textinput.Blink
}

func (m *model) closeConsole(reason string) (tea.Model, tea.Cmd) {
	m.console = nil
	m.status = reason
	m.relayout()
	return m, nil
}

func (m *model) updateConsole(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
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

// --- modal input: folder picker ---

// openPicker starts the folder browser at the operator's home directory.
func (m *model) openPicker() tea.Cmd {
	fp := filepicker.New()
	fp.DirAllowed = true
	fp.FileAllowed = false
	if home, err := os.UserHomeDir(); err == nil {
		fp.CurrentDirectory = home
	}
	fp.AutoHeight = false
	m.pick = &fp
	m.status = "pick the folder your server lives in"
	m.relayout()
	return fp.Init()
}

func (m *model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "esc" {
		m.pick = nil
		m.status = "add cancelled"
		m.relayout()
		return m, nil
	}
	fp, cmd := m.pick.Update(msg)
	m.pick = &fp
	if ok, path := fp.DidSelectFile(msg); ok {
		m.pick = nil
		m.busy = true
		m.status = "adding " + path + "…"
		m.relayout()
		return m, m.addRootCmd(path)
	}
	if ok, path := fp.DidSelectDisabledFile(msg); ok {
		m.status = path + " can't be added (not a folder)"
		return m, nil
	}
	return m, cmd
}

// --- modal input: patch confirm ---

func (m *model) updatePatch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y":
		p := m.pat.patch
		m.pat = nil
		m.busy = true
		m.status = "patching…"
		m.relayout()
		return m, m.applyPatchCmd(p)
	case "esc":
		m.pat = nil
		m.status = "script left unchanged"
		m.relayout()
		return m, nil
	}
	return m, nil
}

// --- selection and layout ---

func (m *model) selected() (server.Spec, bool) {
	for _, s := range m.specs {
		if s.ID == m.selID {
			return s, true
		}
	}
	return server.Spec{}, false
}

// syncSelection follows the list cursor: it points the log follower at the
// newly selected server and clears the viewport.
func (m *model) syncSelection() {
	it, ok := m.list.SelectedItem().(serverItem)
	if !ok {
		m.selID = ""
		m.tail = nil
		return
	}
	if it.spec.ID == m.selID && m.tail != nil {
		return
	}
	m.selID = it.spec.ID
	m.tail = newFollower(it.spec.LogFile)
	m.logQuery = ""
	m.logSearch = nil
	m.rconSnap = rcon.Snapshot{}
	m.rconErr = ""
	m.rconAt = time.Time{}
	m.proc = procstat.Stat{}
	m.procErr = ""
	m.procAt = time.Time{}
	m.vp.SetContent("")
	m.vp.GotoTop()
	m.relayout() // the notice banner depends on which server is selected
}

func (m *model) refreshItems() {
	if m.list.FilterState() == list.Filtering {
		return
	}
	items := make([]list.Item, len(m.specs))
	for i, s := range m.specs {
		r := m.reports[s.ID]
		items[i] = serverItem{spec: s, status: r.Derived, warn: r.Warning}
	}
	m.list.SetItems(items)
	m.syncSelection()
}

func (m *model) appendLogs(lines []string) {
	m.tail.append(lines, maxLogLines)
	m.renderLog()
}

// renderLog word-wraps the buffered log to the current pane width so no line is
// cut off. It re-runs on resize, on a tab or filter change, and as lines arrive.
func (m *model) renderLog() {
	if m.tail == nil {
		m.vp.SetContent("")
		return
	}
	w := max(m.vp.Width, 1)
	m.vp.SetContent(lipgloss.NewStyle().Width(w).Render(m.logBody()))
}

// relayout sizes every pane from the current terminal size and mode. It runs on
// resize and whenever a mode changes the chrome (console bar, "?" help grid).
func (m *model) relayout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	// Leave the last terminal column empty. A frame that fills the width exactly
	// makes every rendered line full-width, and Bubble Tea then skips its
	// erase-to-end-of-line on repaint (a full line "can't" have stale trailing
	// cells). Some terminals, VS Code's integrated terminal among them, do leave
	// stale cells there when a coloured log line is repainted shorter during a
	// scroll, which showed up as the console's side rail drifting sideways.
	// One spare column brings the erase back and pins the rail.
	innerW := max(m.width-2*framePadX-1, 20)
	innerH := max(m.height-2*framePadY, 8)
	m.help.Width = innerW
	m.bodyW = innerW

	helpH := lipgloss.Height(m.help.View(m.commandBar()))
	noticeH := 0
	if t := m.noticeText(); t != "" {
		noticeH = lipgloss.Height(lipgloss.NewStyle().Width(innerW).Render(t)) + 1 // banner + spacer
	}
	// rows: header(1) + help(helpH) + spacer(1) + [notice + spacer] + body + spacer(1) + status(1)
	bodyH := innerH - helpH - noticeH - 4
	if m.console != nil || m.logSearch != nil {
		bodyH -= 2 // spacer + input bar
	}
	bodyH = max(bodyH, 3)
	m.bodyH = bodyH

	// The list and detail screens share the body as two columns; the console
	// screen takes the whole width. The list is held to a slim column so the
	// detail panel beside it gets the room.
	m.listW = clampInt(innerW*2/5, 24, 34)
	m.list.SetSize(m.listW, bodyH)

	// The console screen carries a player and resource rail on the right, but
	// only when the terminal is wide enough to spare the columns.
	m.railW = 0
	logW := innerW
	if m.screen == screenConsole && innerW >= 64 {
		m.railW = 24
		logW = innerW - m.railW - 3
	}
	m.vp.Width = max(logW, 20)
	m.vp.Height = max(bodyH-3, 1) // log header + tab bar + rule
	m.renderLog()

	if m.pick != nil {
		m.pick.SetHeight(max(bodyH-2, 3))
	}
	if m.console != nil {
		m.console.Width = max(innerW-lipgloss.Width(m.console.Prompt)-2, 20)
	}
	if m.logSearch != nil {
		m.logSearch.Width = max(innerW-lipgloss.Width(m.logSearch.Prompt)-2, 20)
	}
}

func (m *model) applyReload(msg reloadedMsg) tea.Cmd {
	if msg.err != nil {
		m.status = "registry: " + msg.err.Error()
		return nil
	}
	m.loaded = true
	prev := m.selID
	m.specs = msg.specs
	m.refreshItems()
	if prev != "" {
		for i, s := range m.specs {
			if s.ID == prev {
				m.list.Select(i)
				break
			}
		}
	}
	m.syncSelection()
	// A server that vanished from under a drilled-in screen sends us home.
	if _, ok := m.selected(); !ok && m.screen != screenList {
		m.screen = screenList
	}
	m.clampMenuCursor()
	m.relayout()

	switch {
	case len(m.specs) == 0:
		m.screen = screenList
		m.tail = nil
		if m.transientStatus() {
			m.status = "no servers yet"
		}
		return nil
	default:
		if m.transientStatus() {
			m.status = "ready"
		}
		return m.reconcileCmd()
	}
}

// transientStatus reports whether the status line still holds a boot or scan
// placeholder that a fresh reload should replace.
func (m *model) transientStatus() bool {
	switch m.status {
	case initialStatus, "scanning your folders…", "scanning the folder you added…", "refreshing":
		return true
	}
	return false
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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

type rconMsg struct {
	id   server.ID
	snap rcon.Snapshot
	err  error
}

// rconPollCmd asks the selected server who is online, but only while its console
// is open, it is running, RCON is configured, and the last poll has aged out.
func (m *model) rconPollCmd() tea.Cmd {
	if m.screen != screenConsole || m.rconInFlight || time.Since(m.rconAt) < pollEvery {
		return nil
	}
	spec, ok := m.selected()
	if !ok || !spec.RCON.Enabled || spec.RCON.Port == 0 {
		return nil
	}
	if m.reports[spec.ID].Derived != server.StatusRunning {
		return nil
	}
	m.rconInFlight = true
	m.rconAt = time.Now()
	addr := fmt.Sprintf("127.0.0.1:%d", spec.RCON.Port)
	pw, id := spec.RCON.Password, spec.ID
	return func() tea.Msg {
		snap, err := rcon.Poll(addr, pw)
		return rconMsg{id: id, snap: snap, err: err}
	}
}

type procMsg struct {
	id   server.ID
	stat procstat.Stat
	err  error
}

// procPollCmd samples the selected server's process for memory and CPU, on the
// same terms as the player poll: console open, server running, poll aged out.
func (m *model) procPollCmd() tea.Cmd {
	if m.screen != screenConsole || m.procInFlight || time.Since(m.procAt) < pollEvery {
		return nil
	}
	spec, ok := m.selected()
	if !ok || m.reports[spec.ID].Derived != server.StatusRunning {
		return nil
	}
	m.procInFlight = true
	m.procAt = time.Now()
	sup, sess, id := m.app.Sup, spec.Session, spec.ID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), pollEvery)
		defer cancel()
		pid, err := sup.PID(ctx, sess)
		if err != nil || pid == 0 {
			return procMsg{id: id, err: fmt.Errorf("%s: no process id", id)}
		}
		stat, err := procstat.Sample(ctx, pid)
		return procMsg{id: id, stat: stat, err: err}
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
