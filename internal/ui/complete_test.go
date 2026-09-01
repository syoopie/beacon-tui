package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/lifecycle"
	"github.com/syoopie/beacon-tui/internal/rcon"
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

// runningConsoleInput boots a running server and opens its console input in
// command mode: it presses "/", which opens the input already holding the
// slash. Command completion tests type the rest of the command after it.
func runningConsoleInput(t *testing.T, cmds server.Commands) (*model, tea.Model) {
	t.Helper()
	m, tm, sup, dirs, _ := bootModel(t)
	spec := writeSpec(t, dirs, "survival")
	setCommands(t, dirs, spec, cmds)
	tm = loadRegistry(t, m, tm)

	sup.present[spec.Session] = true
	m.reports[spec.ID] = reconcile.Report{ID: spec.ID, Derived: server.StatusRunning}

	tm = openConsole(t, m, tm)
	tm, _ = pressRune(t, m, tm, "/")
	if m.console == nil {
		t.Fatal("pressing / did not open the command input")
	}
	if m.console.Value() != "/" {
		t.Fatalf("command input opened holding %q, want a leading slash", m.console.Value())
	}
	return m, tm
}

func typeRunes(t *testing.T, tm tea.Model, s string) tea.Model {
	t.Helper()
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return tm
}

func TestConsoleChatModeVersusCommandMode(t *testing.T) {
	m, tm, sup, dirs, _ := bootModel(t)
	spec := writeSpec(t, dirs, "survival")
	setCommands(t, dirs, spec, server.Commands{MCVersion: "1.20.1"})
	tm = loadRegistry(t, m, tm)
	sup.present[spec.Session] = true
	m.reports[spec.ID] = reconcile.Report{ID: spec.ID, Derived: server.StatusRunning}
	tm = openConsole(t, m, tm)

	// t opens a free-text line: no slash, no completion panel, arrows are history.
	tm, _ = pressRune(t, m, tm, "t")
	if m.console == nil || m.console.Value() != "" {
		t.Fatalf("t should open an empty input, got %v / %q", m.console != nil, valueOr(m))
	}
	tm = typeRunes(t, tm, "hello there")
	if m.commandMode() {
		t.Fatal("a line without a slash is not command mode")
	}
	if len(m.cmp.Suggestions) != 0 {
		t.Fatalf("chat line offered completions: %+v", m.cmp.Suggestions)
	}
	if strings.Contains(tm.View(), "▸") {
		t.Fatalf("the completion panel showed for a chat line:\n%s", tm.View())
	}

	// Typing a leading slash flips into command mode: the panel and its
	// suggestions come back.
	m.console.SetValue("/ga")
	m.onConsoleEdit()
	if !m.commandMode() {
		t.Fatal("a line starting with / is command mode")
	}
	if len(m.cmp.Suggestions) < 2 {
		t.Fatalf("command mode lost its suggestions: %+v", m.cmp.Suggestions)
	}

	// esc closes; / reopens straight into command mode holding the slash.
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyEsc})
	pressRune(t, m, tm, "/")
	if !m.commandMode() || m.console.Value() != "/" {
		t.Fatalf("/ should reopen in command mode holding the slash, got mode=%v val=%q", m.commandMode(), valueOr(m))
	}
}

func valueOr(m *model) string {
	if m.console == nil {
		return "<no input>"
	}
	return m.console.Value()
}

func TestConsoleCompletionSuggestsAndTabCycles(t *testing.T) {
	m, tm := runningConsoleInput(t, server.Commands{MCVersion: "1.20.1"})

	tm = typeRunes(t, tm, "ga") // "/ga"
	got := make([]string, len(m.cmp.Suggestions))
	for i, s := range m.cmp.Suggestions {
		got[i] = s.Text
	}
	if len(got) < 2 || got[0] != "gamemode" || got[1] != "gamerule" {
		t.Fatalf("suggestions for %q = %v, want gamemode then gamerule", "/ga", got)
	}

	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyTab})
	if m.console.Value() != "/gamemode" {
		t.Fatalf("first tab put %q in the input, want /gamemode", m.console.Value())
	}
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyTab})
	if m.console.Value() != "/gamerule" {
		t.Fatalf("second tab put %q in the input, want /gamerule", m.console.Value())
	}
	// shift+tab walks back.
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.console.Value() != "/gamemode" {
		t.Fatalf("shift+tab put %q in the input, want /gamemode", m.console.Value())
	}

	// Typing ends the cycle; the token stays and suggestions refresh.
	typeRunes(t, tm, "x")
	if m.cmpCycle {
		t.Error("cycle did not end when the operator typed")
	}
	if m.console.Value() != "/gamemodex" {
		t.Fatalf("input = %q after typing past a cycle", m.console.Value())
	}
}

func TestConsoleCompletionArrowsCycleWhenTyping(t *testing.T) {
	m, tm := runningConsoleInput(t, server.Commands{MCVersion: "1.20.1"})

	tm = typeRunes(t, tm, "ga") // "/ga"
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyDown})
	if m.console.Value() != "/gamemode" {
		t.Fatalf("down put %q in the input, want /gamemode", m.console.Value())
	}
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyDown})
	if m.console.Value() != "/gamerule" {
		t.Fatalf("second down put %q in the input, want /gamerule", m.console.Value())
	}
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyUp})
	if m.console.Value() != "/gamemode" {
		t.Fatalf("up put %q in the input, want /gamemode", m.console.Value())
	}

	// Back on a plain line the same arrows walk history instead.
	m.console.SetValue("")
	m.onConsoleEdit()
	m.history.Add("seed")
	drive(t, tm, tea.KeyMsg{Type: tea.KeyUp})
	if m.console.Value() != "seed" {
		t.Fatalf("up on an empty line = %q, want the last command", m.console.Value())
	}
}

func TestConsoleCompletionUsageHintForArguments(t *testing.T) {
	m, tm := runningConsoleInput(t, server.Commands{MCVersion: "1.20.1"})

	typeRunes(t, tm, "give ") // "/give "
	if !strings.Contains(m.cmp.Hint, "<targets>") {
		t.Fatalf("hint for %q = %q, want it to mention <targets>", "/give ", m.cmp.Hint)
	}
}

func TestConsoleCompletionOffWithoutVersion(t *testing.T) {
	m, tm := runningConsoleInput(t, server.Commands{}) // no mc_version

	typeRunes(t, tm, "g")
	if len(m.cmp.Suggestions) != 0 {
		t.Fatalf("suggestions = %d, want none when no command tree is available", len(m.cmp.Suggestions))
	}
	if !strings.Contains(m.cmp.Degraded, "could not detect") {
		t.Fatalf("Degraded = %q, want it to explain the missing version", m.cmp.Degraded)
	}
	// The panel points the operator at the field that fixes it.
	if panel := m.completionPanelView(80); !strings.Contains(panel, "Launch settings") {
		t.Fatalf("completion panel = %q, want it to name Launch settings", panel)
	}
}

func TestConsoleHistoryRecall(t *testing.T) {
	m, tm := runningConsoleInput(t, server.Commands{MCVersion: "1.20.1"})
	tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyEsc}) // start from a closed input

	// Sending closes the input, so each line is: open with t, type, enter.
	send := func(line string) {
		t.Helper()
		tm, _ = pressRune(t, m, tm, "t")
		tm = typeRunes(t, tm, line)
		_, msgs := drive(t, tm, tea.KeyMsg{Type: tea.KeyEnter})
		for _, msg := range msgs {
			tm, _ = drive(t, tm, msg)
		}
		if m.console != nil {
			t.Fatalf("console stayed open after sending %q", line)
		}
	}
	send("list")
	send("time set day")

	if all := m.history.All(); len(all) != 2 || all[1] != "time set day" {
		t.Fatalf("history = %v, want [list, time set day]", m.history.All())
	}

	// Reopen a plain line; the arrows now walk the two sent commands.
	tm, _ = pressRune(t, m, tm, "t")
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
	tm, _ = pressRune(t, m, tm, "t")
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
	pressRune(t, m2, tm2, "t")

	if m2.history == nil || len(m2.history.All()) != 1 || m2.history.All()[0] != "seed" {
		t.Fatalf("reopened history = %v, want [seed]", historyLines(m2))
	}
}

func TestConsoleCompletionBackfillsMissingVersion(t *testing.T) {
	m, tm, sup, dirs, _ := bootModel(t)
	spec := writeSpec(t, dirs, "survival") // no [commands] mc_version

	// Give the server dir a forge layout so Identify has something to find.
	forge := filepath.Join(spec.Dir, "libraries", "net", "minecraftforge", "forge", "1.20.1-47.4.20")
	if err := os.MkdirAll(forge, 0o755); err != nil {
		t.Fatal(err)
	}
	// Drive the reload and every message it fans out, including the backfill
	// the reload schedules and the commandsDetectedMsg it produces.
	queue := runCmd(t, m.reloadCmd())
	for len(queue) > 0 {
		msg := queue[0]
		queue = queue[1:]
		var more []tea.Msg
		tm, more = drive(t, tm, msg)
		queue = append(queue, more...)
	}
	sup.present[spec.Session] = true
	m.reports[spec.ID] = reconcile.Report{ID: spec.ID, Derived: server.StatusRunning}

	got, _ := m.selected()
	if got.Commands.MCVersion != "1.20.1" || got.Commands.Loader != "forge" {
		t.Fatalf("spec not backfilled in memory: %+v", got.Commands)
	}

	// The completion engine now works without the operator touching a file.
	tm = openConsole(t, m, tm)
	tm, _ = pressRune(t, m, tm, "/")
	typeRunes(t, tm, "ga")
	if len(m.cmp.Suggestions) < 2 || m.cmp.Degraded != "" {
		t.Fatalf("completion still degraded after backfill: %d suggestions, %q", len(m.cmp.Suggestions), m.cmp.Degraded)
	}
}

func TestConsoleCompletionFoldsInRCONHelp(t *testing.T) {
	m, tm := runningConsoleInput(t, server.Commands{MCVersion: "1.20.1", Loader: "forge"})

	// A modded command is unknown until the /help text arrives.
	tm = typeRunes(t, tm, "ftbques") // "/ftbques"
	if len(m.cmp.Suggestions) != 0 {
		t.Fatalf("ftbquests suggested before /help was fetched: %+v", m.cmp.Suggestions)
	}

	spec, _ := m.selected()
	raw := "/give <targets> <item> [<count>]\n/ftbquests (reload|open_book|locked)\n"
	drive(t, tm, cmdHelpMsg{id: spec.ID, raw: raw})

	if m.cmdHelp[spec.ID] != raw {
		t.Fatal("help text not cached on the model")
	}
	m.console.SetValue("/ftbques")
	m.recomputeCompletion()
	if len(m.cmp.Suggestions) == 0 || m.cmp.Suggestions[0].Text != "ftbquests" {
		t.Fatalf("ftbquests not suggested after /help arrived: %+v", m.cmp.Suggestions)
	}

	// The vanilla grammar still wins for a shared command.
	m.console.SetValue("/give ")
	m.recomputeCompletion()
	if m.cmp.Hint == "" {
		t.Fatalf("give lost its bundled usage hint after folding in /help")
	}
}

func TestConsoleCompletionSuggestsOnlinePlayers(t *testing.T) {
	m, tm := runningConsoleInput(t, server.Commands{MCVersion: "1.20.1"})

	// An entity slot shows only its usage hint until a roster arrives.
	tm = typeRunes(t, tm, "kill ") // "/kill "
	if len(m.cmp.Suggestions) != 0 {
		t.Fatalf("kill suggested names before any RCON poll: %+v", m.cmp.Suggestions)
	}

	spec, _ := m.selected()
	drive(t, tm, rconMsg{id: spec.ID, snap: rcon.Snapshot{Players: []string{"Notch", "jeb_"}}})

	m.console.SetValue("/kill ")
	m.recomputeCompletion()
	got := make([]string, len(m.cmp.Suggestions))
	for i, s := range m.cmp.Suggestions {
		got[i] = s.Text
	}
	if len(got) != 2 || got[0] != "Notch" || got[1] != "jeb_" {
		t.Fatalf("kill suggestions = %v, want the online roster", got)
	}
	if m.cmp.Hint == "" {
		t.Error("kill lost its <targets> usage hint next to the names")
	}
}

func historyLines(m *model) []string {
	if m.history == nil {
		return nil
	}
	return m.history.All()
}
