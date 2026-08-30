package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/syoopie/beacon-tui/internal/reconcile"
	"github.com/syoopie/beacon-tui/internal/server"
)

// railColumns returns the distinct display columns the rail's "Players" header
// starts at across a rendered frame. A stable rail is exactly one column.
func railColumns(view string) map[int]bool {
	cols := map[int]bool{}
	for _, line := range strings.Split(view, "\n") {
		if i := strings.Index(line, "Players"); i >= 0 {
			cols[lipgloss.Width(line[:i])] = true
		}
	}
	return cols
}

func TestConsoleRailStaysPutWhileScrolling(t *testing.T) {
	for _, width := range []int{72, 80, 90, 100, 110, 132} {
		m, tm, sup, dirs, _ := bootModel(t)
		tm, _ = drive(t, tm, tea.WindowSizeMsg{Width: width, Height: 26})
		spec := writeSpec(t, dirs, "survival")
		tm = loadRegistry(t, m, tm)
		sup.present[spec.Session] = true
		m.reports[spec.ID] = reconcile.Report{ID: spec.ID, Derived: server.StatusRunning}
		m.refreshItems()
		for i := range m.specs {
			m.specs[i].RCON = server.RCON{Enabled: true, Port: 25575, Password: "x"}
		}
		tm = openMenu(t, m, tm)
		tm, _ = chooseMenu(t, m, tm, "Open console")

		// Real logs mix short lines with unbreakable tokens far wider than the
		// column: stack-trace class names, long paths. Word wrap cannot split
		// these, so without a hard clamp they widen the whole column.
		var lines []string
		for i := 0; i < 120; i++ {
			if i%5 == 0 {
				lines = append(lines, "[12:00:00] [Server/ERROR]: net.minecraftforge.fml.common.EnhancedRuntimeException.veryLongUnbreakableIdentifierThatCannotWrap"+strings.Repeat("x", i%40))
			} else {
				lines = append(lines, "[12:00:00] [Server/INFO]: line "+strings.Repeat("y ", i%12))
			}
		}
		m.appendLogs(lines)
		m.renderLog()
		m.vp.GotoBottom()

		all := map[int]bool{}
		for step := 0; step < 25; step++ {
			for c := range railColumns(tm.View()) {
				all[c] = true
			}
			// Every row of the composed console body must be the same width, or
			// a row is ragged and the rail is misaligned on it.
			seen := map[int]bool{}
			for _, row := range strings.Split(m.consoleView(), "\n") {
				seen[lipgloss.Width(row)] = true
			}
			if len(seen) != 1 {
				t.Fatalf("width %d, step %d: console rows have uneven widths %v", width, step, seen)
			}
			tm, _ = drive(t, tm, tea.KeyMsg{Type: tea.KeyUp})
		}
		if len(all) != 1 {
			t.Fatalf("width %d: the rail shifted across columns %v while scrolling", width, all)
		}
		// The frame must stop one column short of the terminal, so Bubble Tea
		// keeps erasing each line's tail on repaint. A line that fills the width
		// exactly is the redraw bug that made the rail drift in some terminals.
		for _, line := range strings.Split(tm.View(), "\n") {
			if w := lipgloss.Width(line); w >= width {
				t.Fatalf("width %d: a line filled %d cols; the frame must leave the last column empty: %q", width, w, line)
			}
		}
	}
}

func TestConsoleLogScrollsSeveralLinesPerKey(t *testing.T) {
	m, tm, _, dirs, _ := bootModel(t)
	tm, _ = drive(t, tm, tea.WindowSizeMsg{Width: 100, Height: 24})
	writeSpec(t, dirs, "survival")
	tm = loadRegistry(t, m, tm)
	tm = openConsole(t, m, tm)

	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, "[12:00:00] [Server/INFO]: line")
	}
	m.appendLogs(lines)
	m.renderLog()
	m.vp.GotoBottom()

	before := m.vp.YOffset
	drive(t, tm, tea.KeyMsg{Type: tea.KeyUp})
	if moved := before - m.vp.YOffset; moved != logScrollStep {
		t.Fatalf("one up-press moved %d lines, want %d", moved, logScrollStep)
	}
}
