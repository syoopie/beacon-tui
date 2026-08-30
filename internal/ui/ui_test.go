package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/lifecycle"
	"github.com/syoopie/beacon-tui/internal/reconcile"
	"github.com/syoopie/beacon-tui/internal/selfupdate"
	"github.com/syoopie/beacon-tui/internal/server"
	"github.com/syoopie/beacon-tui/internal/supervisor"
)

type stubSupervisor struct {
	mu      sync.Mutex
	present map[server.Session]bool
	sent    []string
}

func newStub() *stubSupervisor {
	return &stubSupervisor{present: map[server.Session]bool{}}
}

func (s *stubSupervisor) Start(_ context.Context, l supervisor.Launch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.present[l.Session] = true
	return nil
}

func (s *stubSupervisor) Exists(_ context.Context, sess server.Session) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.present[sess], nil
}

func (s *stubSupervisor) SendKeys(_ context.Context, _ server.Session, line string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, line)
	return nil
}

func (s *stubSupervisor) sentLines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sent...)
}

func (s *stubSupervisor) Kill(_ context.Context, sess server.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.present, sess)
	return nil
}

func (s *stubSupervisor) isUp(sess server.Session) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.present[sess]
}

func runCmd(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, runCmd(t, c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

func drive(t *testing.T, m tea.Model, msg tea.Msg) (tea.Model, []tea.Msg) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next, runCmd(t, cmd)
}

func bootModel(t *testing.T) (*model, tea.Model, *stubSupervisor, config.Dirs, string) {
	t.Helper()
	root := t.TempDir()
	dirs := config.Dirs{Config: filepath.Join(root, "c"), State: filepath.Join(root, "s")}
	sup := newStub()
	app := App{
		Dirs: dirs,
		Cfg:  config.Config{ScanRoots: []string{root}, StopTimeout: config.Duration(time.Minute)},
		Sup:  sup,
		Mgr:  lifecycle.NewManager(sup, dirs, time.Minute),
	}
	m := newModel(app)
	var tm tea.Model = m
	tm, _ = drive(t, tm, tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, tm, sup, dirs, root
}

func loadRegistry(t *testing.T, m *model, tm tea.Model) tea.Model {
	t.Helper()
	for _, msg := range runCmd(t, m.reloadCmd()) {
		tm, _ = drive(t, tm, msg)
	}
	return tm
}

func writeSpec(t *testing.T, dirs config.Dirs, id string) server.Spec {
	t.Helper()
	pid, err := server.ParseID(id)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	s := server.Spec{
		ID:      pid,
		Dir:     dir,
		Start:   "./run.sh",
		Script:  "run.sh",
		Port:    40000,
		Session: server.SessionFor(pid),
		LogFile: filepath.Join(dir, "server.log"),
		Exec:    server.ExecOK,
		State:   server.State{LastKnown: server.StatusStopped},
	}
	if err := config.SaveSpec(dirs, s); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}
	return s
}

func pressRune(t *testing.T, m *model, tm tea.Model, r string) (tea.Model, []tea.Msg) {
	return drive(t, tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)})
}

// openMenu presses enter on the list to drill into the selected server's menu.
func openMenu(t *testing.T, m *model, tm tea.Model) tea.Model {
	t.Helper()
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyEnter})
	if m.screen != screenMenu {
		t.Fatalf("expected the server menu, still on screen %d", m.screen)
	}
	return tm
}

// chooseMenu moves the menu cursor to the row with the given label and presses enter.
func chooseMenu(t *testing.T, m *model, tm tea.Model, label string) (tea.Model, []tea.Msg) {
	t.Helper()
	idx := -1
	for i, r := range m.menuRows() {
		if r.label == label {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("menu has no row %q; rows: %v", label, menuLabels(m))
	}
	for m.menuCursor != idx {
		key := tea.KeyDown
		if m.menuCursor > idx {
			key = tea.KeyUp
		}
		tm, _ = drive(t, tm, tea.KeyMsg{Type: key})
	}
	return drive(t, tm, tea.KeyMsg{Type: tea.KeyEnter})
}

func menuLabels(m *model) []string {
	var out []string
	for _, r := range m.menuRows() {
		out = append(out, r.label)
	}
	return out
}

func hasMenuRow(m *model, label string) bool {
	for _, l := range menuLabels(m) {
		if l == label {
			return true
		}
	}
	return false
}

func TestModelLoadsRegistryAndRendersList(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	writeSpec(t, dirs, "survival")
	writeSpec(t, dirs, "creative")
	tm = loadRegistry(t, m, tm)

	view := tm.View()
	if !strings.Contains(view, "survival") || !strings.Contains(view, "creative") {
		t.Fatalf("list view missing servers:\n%s", view)
	}
}

func TestStartFromMenuDrivesLifecycleAndClearsBusy(t *testing.T) {
	m, tm, sup, dirs, _ := bootModel(t)
	spec := writeSpec(t, dirs, "survival")
	tm = loadRegistry(t, m, tm)

	tm = openMenu(t, m, tm)
	tm, msgs := chooseMenu(t, m, tm, "Start")
	if !m.busy {
		t.Fatal("model should be busy right after choosing Start")
	}
	for _, msg := range msgs {
		tm, _ = drive(t, tm, msg)
	}
	if m.busy {
		t.Fatal("busy flag still set after the start op finished")
	}
	if !sup.isUp(spec.Session) {
		t.Fatal("supervisor was never asked to start the session")
	}
	reloaded, err := config.LoadSpec(dirs.ServerFile(spec.ID))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.State.LastKnown != server.StatusRunning {
		t.Fatalf("persisted state = %v, want running", reloaded.State.LastKnown)
	}
}

func TestListStaysVisibleBesideTheActionPanel(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	writeSpec(t, dirs, "survival")
	writeSpec(t, dirs, "creative")
	tm = loadRegistry(t, m, tm)

	if !strings.Contains(tm.View(), "creative") || !strings.Contains(tm.View(), "→  act on this server") {
		t.Fatalf("list screen should show both the list and the passive action panel:\n%s", tm.View())
	}

	tm = openMenu(t, m, tm)
	view := tm.View()
	if !strings.Contains(view, "survival") || !strings.Contains(view, "creative") {
		t.Fatalf("the list must remain on screen while the panel is focused:\n%s", view)
	}
	if !strings.Contains(view, "▸ Open console") {
		t.Fatalf("focused panel should show the row cursor:\n%s", view)
	}

	drive(t, tm, tea.KeyMsg{Type: tea.KeyLeft})
	if m.screen != screenList {
		t.Fatalf("left arrow should return focus to the list, screen = %d", m.screen)
	}
}

func TestForceKillHiddenFromMenuUntilTimeout(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	writeSpec(t, dirs, "survival")
	tm = loadRegistry(t, m, tm)

	openMenu(t, m, tm)
	if hasMenuRow(m, "Force-kill") {
		t.Fatalf("force-kill offered without a prior stop timeout; rows: %v", menuLabels(m))
	}
}

func TestImportKeyWritesSpecsForScannedDirs(t *testing.T) {
	m, tm, _, dirs, root := bootModel(t)
	tm = loadRegistry(t, m, tm)

	srv := filepath.Join(root, "mc", "vanilla")
	if err := writeRunScript(t, srv); err != nil {
		t.Fatal(err)
	}
	m.app.Cfg.ScanRoots = []string{filepath.Join(root, "mc")}

	_, msgs := pressRune(t, m, tm, "i")
	for _, msg := range msgs {
		tm, _ = drive(t, tm, msg)
	}
	specs, err := config.LoadSpecs(dirs)
	if err != nil {
		t.Fatalf("LoadSpecs: %v", err)
	}
	if len(specs) != 1 || specs[0].ID != "vanilla" {
		t.Fatalf("import wrote %+v, want one spec 'vanilla'", specs)
	}
}

func TestAddKeyOpensAndEscapesTheFolderPicker(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	writeSpec(t, dirs, "survival")
	tm = loadRegistry(t, m, tm)

	drive(t, tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if m.pick == nil {
		t.Fatal("pressing a did not open the folder picker")
	}
	if !strings.Contains(tm.View(), "Add a server") {
		t.Fatalf("picker view not shown:\n%s", tm.View())
	}

	drive(t, tm, tea.KeyMsg{Type: tea.KeyEsc})
	if m.pick != nil {
		t.Fatal("esc did not close the picker")
	}
}

func TestPickedFolderIsAddedAndScanned(t *testing.T) {
	m, tm, _, dirs, root := bootModel(t)
	m.app.Cfg = config.Config{StopTimeout: config.Duration(time.Minute)}
	tm = loadRegistry(t, m, tm)

	server := filepath.Join(root, "survival")
	if err := writeRunScript(t, server); err != nil {
		t.Fatal(err)
	}

	// The filepicker's own selection handling is its concern; drive the message
	// it would emit and assert beacon's glue writes config and re-scans.
	_, msgs := drive(t, tm, runCmd(t, m.addRootCmd(server))[0])
	for _, msg := range msgs {
		tm, _ = drive(t, tm, msg)
	}

	cfg, err := config.Load(dirs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.ScanRoots) != 1 || cfg.ScanRoots[0] != server {
		t.Fatalf("config scan roots = %v, want [%s]", cfg.ScanRoots, server)
	}
	specs, err := config.LoadSpecs(dirs)
	if err != nil {
		t.Fatalf("LoadSpecs: %v", err)
	}
	if len(specs) != 1 || specs[0].ID != "survival" {
		t.Fatalf("import after add wrote %+v, want one spec 'survival'", specs)
	}
}

func TestLandingPageShownWhenNoServers(t *testing.T) {
	m, tm, _, _, _ := bootModel(t)
	tm = loadRegistry(t, m, tm)

	view := tm.View()
	for _, want := range []string{"no servers yet", "add your first server", "a add server"} {
		if !strings.Contains(view, want) {
			t.Fatalf("landing view missing %q:\n%s", want, view)
		}
	}
}

func TestFrameIsPadded(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	writeSpec(t, dirs, "survival")
	tm = loadRegistry(t, m, tm)

	for _, line := range strings.Split(tm.View(), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			t.Fatalf("line is flush with the edge, no left padding: %q", line)
		}
	}
}

func TestUnknownServerShowsNoticeAndNeverOverflows(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t) // 100 x 30
	spec := writeSpec(t, dirs, "bmc4_serverpack_v61")
	tm = loadRegistry(t, m, tm)

	m.reports[spec.ID] = reconcile.Report{
		ID:      spec.ID,
		Derived: server.StatusUnknown,
		Warning: "Beacon lost track of this server: its tmux session vanished while it was running. Check whether it is really down before starting it again.",
	}
	m.refreshItems()
	tm = openMenu(t, m, tm)
	tm, _ = chooseMenu(t, m, tm, "Open console")
	m.appendLogs([]string{strings.Repeat("a very long unbroken log line that must not spill past the pane ", 4)})

	view := tm.View()
	if !strings.Contains(view, "lost track of this server") {
		t.Fatalf("Unknown server should raise the notice banner:\n%s", view)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > 100 {
			t.Fatalf("line %d is %d cols wide, past the 100-col terminal: %q", i, w, line)
		}
	}
}

func TestServerMenuTracksStatus(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	spec := writeSpec(t, dirs, "survival")
	loadRegistry(t, m, tm)

	m.reports[spec.ID] = reconcile.Report{ID: spec.ID, Derived: server.StatusStopped}
	if !hasMenuRow(m, "Start") || hasMenuRow(m, "Stop") {
		t.Fatalf("stopped server menu should offer Start, not Stop: %v", menuLabels(m))
	}

	m.reports[spec.ID] = reconcile.Report{ID: spec.ID, Derived: server.StatusRunning}
	if !hasMenuRow(m, "Stop") || hasMenuRow(m, "Start") {
		t.Fatalf("running server menu should offer Stop, not Start: %v", menuLabels(m))
	}
	if !hasMenuRow(m, "Open console") {
		t.Fatalf("every server menu should offer the console: %v", menuLabels(m))
	}
}

func TestConsoleSendsTypedLineToRunningServer(t *testing.T) {
	m, tm, sup, dirs, _ := bootModel(t)
	spec := writeSpec(t, dirs, "survival")
	tm = loadRegistry(t, m, tm)

	sup.present[spec.Session] = true
	m.reports[spec.ID] = reconcile.Report{ID: spec.ID, Derived: server.StatusRunning}

	tm = openMenu(t, m, tm)
	tm, _ = chooseMenu(t, m, tm, "Open console")
	tm, _ = pressRune(t, m, tm, "c")
	if m.console == nil {
		t.Fatal("pressing c in the console view did not open the input")
	}

	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("say hi")})
	_, msgs := drive(t, tm, tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range msgs {
		tm, _ = drive(t, tm, msg)
	}

	if got := sup.sentLines(); len(got) != 1 || got[0] != "say hi" {
		t.Fatalf("sent lines = %v, want [say hi]", got)
	}
	if m.console == nil {
		t.Fatal("console closed itself after a send; it should stay open")
	}
	if m.console.Value() != "" {
		t.Fatalf("console still holds %q after send, want it cleared", m.console.Value())
	}

	drive(t, tm, tea.KeyMsg{Type: tea.KeyEsc})
	if m.console != nil {
		t.Fatal("esc did not close the console")
	}
}

func TestConsoleRefusedUnlessServerIsRunning(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	writeSpec(t, dirs, "survival")
	tm = loadRegistry(t, m, tm)

	tm = openMenu(t, m, tm)
	tm, _ = chooseMenu(t, m, tm, "Open console")
	pressRune(t, m, tm, "c")
	if m.console != nil {
		t.Fatal("console input opened for a server that is not running")
	}
	if !strings.Contains(m.status, "running") {
		t.Fatalf("status = %q, want the running-only explanation", m.status)
	}
}

func TestUpdateNoticeShowsCommandAndDismisses(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	m.app.Repo = "syoopie/beacon-tui"
	writeSpec(t, dirs, "survival")
	tm = loadRegistry(t, m, tm)

	tm, _ = drive(t, tm, updateMsg{res: selfupdate.Result{Current: "v0.1.0", Latest: "v0.2.0", Available: true}})
	view := tm.View()
	if !strings.Contains(view, "v0.2.0 available") {
		t.Fatalf("title missing the update banner:\n%s", view)
	}
	if !strings.Contains(m.status, "install.sh | bash") {
		t.Fatalf("status = %q, want the update command", m.status)
	}

	pressRune(t, m, tm, "u")
	if m.update != nil {
		t.Fatal("pressing u did not dismiss the banner")
	}
}

func TestUpdateNoticeSilentWhenCurrent(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	writeSpec(t, dirs, "survival")
	tm = loadRegistry(t, m, tm)

	tm, _ = drive(t, tm, updateMsg{res: selfupdate.Result{Current: "v0.2.0", Latest: "v0.2.0", Available: false}})
	if m.update != nil || strings.Contains(tm.View(), "available") {
		t.Fatal("showed an update notice when running the latest release")
	}
}

func writeRunScript(t *testing.T, dir string) error {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\nexec java -jar server.jar nogui\n"), 0o755)
}

func TestLaunchSettingsSwitchesTheStartScript(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	spec := writeSpec(t, dirs, "survival")
	for name, body := range map[string]string{
		"run.sh":   "#!/bin/sh\nexec java -jar server.jar \"$@\"\n",
		"start.sh": "#!/bin/sh\njava -jar server.jar nogui\n",
	} {
		if err := os.WriteFile(filepath.Join(spec.Dir, name), []byte(body), 0o755); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	tm = loadRegistry(t, m, tm)

	tm = openMenu(t, m, tm)
	tm, _ = chooseMenu(t, m, tm, "Launch settings")
	if m.launch == nil {
		t.Fatal("choosing Launch settings did not open the dialog")
	}
	if !strings.Contains(tm.View(), "How should Beacon start survival") {
		t.Fatalf("launch dialog not shown:\n%s", tm.View())
	}

	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyDown}) // run.sh -> start.sh
	_, msgs := drive(t, tm, tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range msgs {
		tm, _ = drive(t, tm, msg)
	}

	reloaded, err := config.LoadSpec(dirs.ServerFile(spec.ID))
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Script != "start.sh" {
		t.Fatalf("Script = %q, want start.sh", reloaded.Script)
	}
	if reloaded.Start != "./start.sh nogui" {
		t.Fatalf("Start = %q, want ./start.sh nogui", reloaded.Start)
	}
	if reloaded.Exec != server.ExecMissing {
		t.Fatalf("Exec = %v, want missing: start.sh has a bare java line", reloaded.Exec)
	}

	tm = loadRegistry(t, m, tm)
	if !strings.Contains(tm.View(), "via start.sh") {
		t.Fatalf("detail pane should show the new launch script:\n%s", tm.View())
	}
}

func TestLaunchSettingsEditsTheArguments(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	spec := writeSpec(t, dirs, "survival")
	if err := os.WriteFile(filepath.Join(spec.Dir, "run.sh"),
		[]byte("#!/bin/sh\nexec java -jar server.jar \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tm = loadRegistry(t, m, tm)

	tm = openMenu(t, m, tm)
	tm, _ = chooseMenu(t, m, tm, "Launch settings")
	// The args field defaults to "nogui"; clear it and type a custom set.
	for range "nogui" {
		tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("nogui --world lobby")})
	_, msgs := drive(t, tm, tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range msgs {
		tm, _ = drive(t, tm, msg)
	}

	reloaded, err := config.LoadSpec(dirs.ServerFile(spec.ID))
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Start != "./run.sh nogui --world lobby" {
		t.Fatalf("Start = %q, want ./run.sh nogui --world lobby", reloaded.Start)
	}
}
