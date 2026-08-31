package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/lifecycle"
	"github.com/syoopie/beacon-tui/internal/reconcile"
	"github.com/syoopie/beacon-tui/internal/server"
)

// bootModelAt is bootModel against an existing Dirs, for tests that need a
// second model over the same on-disk state.
func bootModelAt(t *testing.T, dirs config.Dirs) (*model, tea.Model, *stubSupervisor) {
	t.Helper()
	sup := newStub()
	app := App{
		Dirs: dirs,
		Cfg:  config.Config{ScanRoots: []string{t.TempDir()}, StopTimeout: config.Duration(time.Minute)},
		Sup:  sup,
		Mgr:  lifecycle.NewManager(sup, dirs, time.Minute),
	}
	m := newModel(app)
	var tm tea.Model = m
	tm, _ = drive(t, tm, tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, tm, sup
}

func setCommands(t *testing.T, dirs config.Dirs, s server.Spec, c server.Commands) {
	t.Helper()
	s.Commands = c
	if err := config.SaveSpec(dirs, s); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}
}

// runningConsoleInput boots a running server, opens its console, and presses c
// to focus the command input.
func runningConsoleInput(t *testing.T, cmds server.Commands) (*model, tea.Model) {
	t.Helper()
	m, tm, sup, dirs, _ := bootModel(t)
	spec := writeSpec(t, dirs, "survival")
	setCommands(t, dirs, spec, cmds)
	tm = loadRegistry(t, m, tm)

	sup.present[spec.Session] = true
	m.reports[spec.ID] = reconcile.Report{ID: spec.ID, Derived: server.StatusRunning}

	tm = openConsole(t, m, tm)
	tm, _ = pressRune(t, m, tm, "c")
	if m.console == nil {
		t.Fatal("pressing c did not open the command input")
	}
	return m, tm
}

func typeRunes(t *testing.T, tm tea.Model, s string) tea.Model {
	t.Helper()
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return tm
}

func TestConsoleCompletionSuggestsAndTabCycles(t *testing.T) {
	m, tm := runningConsoleInput(t, server.Commands{MCVersion: "1.20.1"})

	tm = typeRunes(t, tm, "ga")
	got := make([]string, len(m.cmp.Suggestions))
	for i, s := range m.cmp.Suggestions {
		got[i] = s.Text
	}
	if len(got) < 2 || got[0] != "gamemode" || got[1] != "gamerule" {
		t.Fatalf("suggestions for %q = %v, want gamemode then gamerule", "ga", got)
	}

	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyTab})
	if m.console.Value() != "gamemode" {
		t.Fatalf("first tab put %q in the input, want gamemode", m.console.Value())
	}
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyTab})
	if m.console.Value() != "gamerule" {
		t.Fatalf("second tab put %q in the input, want gamerule", m.console.Value())
	}
	// shift+tab walks back.
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.console.Value() != "gamemode" {
		t.Fatalf("shift+tab put %q in the input, want gamemode", m.console.Value())
	}

	// Typing ends the cycle; the token stays and suggestions refresh.
	typeRunes(t, tm, "x")
	if m.cmpCycle {
		t.Error("cycle did not end when the operator typed")
	}
	if m.console.Value() != "gamemodex" {
		t.Fatalf("input = %q after typing past a cycle", m.console.Value())
	}
}

func TestConsoleCompletionUsageHintForArguments(t *testing.T) {
	m, tm := runningConsoleInput(t, server.Commands{MCVersion: "1.20.1"})

	typeRunes(t, tm, "give ")
	if !strings.Contains(m.cmp.Hint, "<targets>") {
		t.Fatalf("hint for %q = %q, want it to mention <targets>", "give ", m.cmp.Hint)
	}
}

func TestConsoleCompletionOffWithoutVersion(t *testing.T) {
	m, tm := runningConsoleInput(t, server.Commands{}) // no mc_version

	typeRunes(t, tm, "g")
	if len(m.cmp.Suggestions) != 0 {
		t.Fatalf("suggestions = %d, want none when no command tree is available", len(m.cmp.Suggestions))
	}
	if !strings.Contains(m.cmp.Degraded, "mc_version") {
		t.Fatalf("Degraded = %q, want it to point at mc_version", m.cmp.Degraded)
	}
}

func TestConsoleHistoryRecall(t *testing.T) {
	m, tm := runningConsoleInput(t, server.Commands{MCVersion: "1.20.1"})

	send := func(line string) {
		t.Helper()
		tm = typeRunes(t, tm, line)
		_, msgs := drive(t, tm, tea.KeyMsg{Type: tea.KeyEnter})
		for _, msg := range msgs {
			tm, _ = drive(t, tm, msg)
		}
	}
	send("list")
	send("time set day")

	if all := m.history.All(); len(all) != 2 || all[1] != "time set day" {
		t.Fatalf("history = %v, want [list, time set day]", m.history.All())
	}

	up := func() { tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyUp}) }
	down := func() { tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyDown}) }

	up()
	if m.console.Value() != "time set day" {
		t.Fatalf("first up = %q, want the newest command", m.console.Value())
	}
	up()
	if m.console.Value() != "list" {
		t.Fatalf("second up = %q, want the older command", m.console.Value())
	}
	up() // clamp at the oldest
	if m.console.Value() != "list" {
		t.Fatalf("up past the oldest = %q, want it to hold at list", m.console.Value())
	}
	down()
	if m.console.Value() != "time set day" {
		t.Fatalf("down = %q, want to walk back toward the live line", m.console.Value())
	}
	down() // back to the live (empty) line
	if m.console.Value() != "" {
		t.Fatalf("down past the newest = %q, want the live line", m.console.Value())
	}
}

func TestConsoleHistoryPersistsAcrossReopen(t *testing.T) {
	m, tm, sup, dirs, _ := bootModel(t)
	spec := writeSpec(t, dirs, "survival")
	setCommands(t, dirs, spec, server.Commands{MCVersion: "1.20.1"})
	tm = loadRegistry(t, m, tm)
	sup.present[spec.Session] = true
	m.reports[spec.ID] = reconcile.Report{ID: spec.ID, Derived: server.StatusRunning}

	tm = openConsole(t, m, tm)
	tm, _ = pressRune(t, m, tm, "c")
	tm = typeRunes(t, tm, "seed")
	_, msgs := drive(t, tm, tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range msgs {
		tm, _ = drive(t, tm, msg) // the history-save command runs here
	}

	// A fresh model over the same dirs sees the persisted line.
	m2, tm2, sup2 := bootModelAt(t, dirs)
	sup2.present[spec.Session] = true
	tm2 = loadRegistry(t, m2, tm2)
	m2.reports[spec.ID] = reconcile.Report{ID: spec.ID, Derived: server.StatusRunning}
	tm2 = openConsole(t, m2, tm2)
	pressRune(t, m2, tm2, "c")

	if m2.history == nil || len(m2.history.All()) != 1 || m2.history.All()[0] != "seed" {
		t.Fatalf("reopened history = %v, want [seed]", historyLines(m2))
	}
}

func historyLines(m *model) []string {
	if m.history == nil {
		return nil
	}
	return m.history.All()
}
