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

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/lifecycle"
	"github.com/syoopie/beacon-tui/internal/selfupdate"
	"github.com/syoopie/beacon-tui/internal/server"
	"github.com/syoopie/beacon-tui/internal/supervisor"
)

type stubSupervisor struct {
	mu      sync.Mutex
	present map[server.Session]bool
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

func (s *stubSupervisor) SendKeys(context.Context, server.Session, string) error { return nil }

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

func TestStartKeyDrivesLifecycleAndClearsBusy(t *testing.T) {
	m, tm, sup, dirs, _ := bootModel(t)
	spec := writeSpec(t, dirs, "survival")
	tm = loadRegistry(t, m, tm)

	tm, msgs := pressRune(t, m, tm, "s")
	if !m.busy {
		t.Fatal("model should be busy right after pressing s")
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

func TestForceKillKeyRefusedUntilTimeout(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	writeSpec(t, dirs, "survival")
	tm = loadRegistry(t, m, tm)

	pressRune(t, m, tm, "K")
	if m.busy {
		t.Fatal("force-kill ran without a prior stop timeout")
	}
	if !strings.Contains(m.status, "force-kill") {
		t.Fatalf("status = %q, want an explanation of when force-kill is offered", m.status)
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
