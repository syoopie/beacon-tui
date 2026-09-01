package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/syoopie/beacon-tui/internal/mccmd"
	"github.com/syoopie/beacon-tui/internal/server"
)

// completionPanelH is the block reserved above the command input while it is
// open: one status line (the usage hint, or why completion is degraded) plus a
// fixed window of suggestions. Fixed so the log above it never reflows as
// suggestions come and go mid-keystroke.
const completionPanelH = 6

var suggestSelStyle = lipgloss.NewStyle().Bold(true).Foreground(accentColor)

// completerKey identifies a spec's completion inputs, so ensureConsoleData
// rebuilds the engine only when one of them changed, not on every registry tick.
// help is the server's cached "/help" text, which arrives once over RCON and
// then never changes for the session.
func completerKey(s server.Spec, help string) string {
	return strings.Join([]string{
		string(s.ID), s.Commands.MCVersion, s.Commands.Loader, s.Commands.Completion, help,
	}, "\x00")
}

// ensureConsoleData makes sure the completion engine and the recall history
// match the selected server. Cheap to call repeatedly: it is a no-op unless the
// server or its [commands] block changed.
func (m *model) ensureConsoleData() {
	spec, ok := m.selected()
	if !ok {
		return
	}

	help := m.cmdHelp[spec.ID]
	if key := completerKey(spec, help); key != m.cmpKey {
		m.cmpKey = key
		m.completer = nil
		m.cmp = mccmd.Result{}
		m.cmpSel = 0
		if spec.Commands.CompletionEnabled() {
			sources := []mccmd.VocabularySource{mccmd.Bundled()}
			if help != "" {
				sources = append(sources, mccmd.HelpSource(help))
			}
			eng, err := mccmd.New(mccmd.Options{
				Sources: sources,
				Context: mccmd.Context{
					MCVersion: spec.Commands.MCVersion,
					Loader:    spec.Commands.Loader,
					ServerID:  string(spec.ID),
				},
			})
			if err == nil {
				m.completer = eng
				// A rebuild drops the roster the running engine had; re-seed it
				// from the last poll so player-name completion survives.
				m.completer.SetPlayers(m.rconSnap.Players)
			}
		}
	}

	if spec.ID != m.histID {
		m.histID = spec.ID
		h, err := mccmd.LoadHistory(m.app.Dirs.HistoryFile(spec.ID), 0)
		if err != nil {
			h = mccmd.NewHistory(0)
		}
		m.history = h
	}
}

// recomputeCompletion refreshes m.cmp from the current input. The cursor is
// treated as end-of-line, which is where console typing almost always sits.
func (m *model) recomputeCompletion() {
	if m.completer == nil || m.console == nil || !m.commandMode() {
		m.cmp = mccmd.Result{}
		return
	}
	v := m.console.Value()
	m.cmp = m.completer.Complete(v, len(v))
	if m.cmpSel >= len(m.cmp.Suggestions) {
		m.cmpSel = 0
	}
}

// onConsoleEdit is the bookkeeping after the input text changed by any means
// other than tab-cycle: the cycle ends, history recall ends, the highlight
// drops, and the suggestions are recomputed.
func (m *model) onConsoleEdit() {
	m.endCycle()
	m.histIdx = -1
	m.cmpSel = 0
	m.recomputeCompletion()
}

// consoleUp is the up (delta -1) and down (delta +1) arrows in the console
// input. In command mode with something to suggest they step the suggestion
// list, the way the arrows move a selection in any completion menu (tab and
// shift+tab do the same). Every other time, including the whole of a plain
// chat line, they walk the command history, the shell convention.
func (m *model) consoleUp(delta int) {
	if m.console == nil {
		return
	}
	if m.cmpCycle { // mid-cycle: keep stepping the pinned list
		m.cycleSuggestion(delta)
		return
	}
	if m.commandMode() && m.histIdx < 0 && len(m.cmp.Suggestions) > 0 {
		m.cycleSuggestion(delta)
		return
	}
	m.recallHistory(delta)
}

// cycleSuggestion is tab (delta +1) and shift+tab (delta -1): it steps the
// highlight through the suggestion list and rewrites the token being completed
// to match, the way the vanilla client's tab key works. The first press starts
// a cycle from the current suggestions and pins the token's start; later
// presses just advance within that pinned list.
func (m *model) cycleSuggestion(delta int) {
	if m.console == nil {
		return
	}
	if !m.cmpCycle {
		if len(m.cmp.Suggestions) == 0 {
			return
		}
		m.cmpCycle = true
		m.cmpList = m.cmp.Suggestions
		m.cmpAnchor = m.cmp.Replace.Start
		m.cmpSel = 0
		if delta < 0 {
			m.cmpSel = len(m.cmpList) - 1
		}
	} else {
		n := len(m.cmpList)
		m.cmpSel = (m.cmpSel + delta + n) % n
	}

	line := m.console.Value()
	if m.cmpAnchor > len(line) {
		m.endCycle()
		return
	}
	seg := m.cmpList[m.cmpSel].Text
	m.console.SetValue(line[:m.cmpAnchor] + seg)
	m.console.CursorEnd()
	m.histIdx = -1

	// Keep the panel showing the pinned list; refresh only the hint against the
	// rewritten line.
	m.cmp.Suggestions = m.cmpList
	m.cmp.Replace = mccmd.Span{Start: m.cmpAnchor, End: m.cmpAnchor + len(seg)}
	if m.completer != nil {
		v := m.console.Value()
		m.cmp.Hint = m.completer.Complete(v, len(v)).Hint
	}
}

// endCycle leaves tab-cycle mode. The rewritten token stays in the input.
func (m *model) endCycle() {
	m.cmpCycle = false
	m.cmpList = nil
}

// recallHistory steps through the recall ring. delta -1 goes further back, +1
// returns toward the live line the operator was typing.
func (m *model) recallHistory(delta int) {
	if m.history == nil || m.console == nil {
		return
	}
	m.endCycle()
	lines := m.history.All()
	if len(lines) == 0 {
		return
	}

	switch {
	case m.histIdx < 0: // on the live line
		if delta > 0 {
			return
		}
		m.histStash = m.console.Value()
		m.histIdx = len(lines) - 1
	default:
		m.histIdx += delta
	}

	switch {
	case m.histIdx < 0:
		m.histIdx = 0
	case m.histIdx >= len(lines):
		m.histIdx = -1
		m.console.SetValue(m.histStash)
		m.console.CursorEnd()
		m.recomputeCompletion()
		return
	}

	m.console.SetValue(lines[m.histIdx])
	m.console.CursorEnd()
	m.recomputeCompletion()
}

// resetCompletionState returns the panel to its just-opened state: no
// highlight, not cycling, not recalling.
func (m *model) resetCompletionState() {
	m.cmp = mccmd.Result{}
	m.cmpSel = 0
	m.endCycle()
	m.histIdx = -1
	m.histStash = ""
}

// completionPanelView renders the fixed block above the command input:
// exactly completionPanelH lines, blank where there is nothing to show.
func (m *model) completionPanelView(w int) string {
	lines := make([]string, completionPanelH)

	// Usage hint when there is one; otherwise the Degraded note, which for a
	// switched-off or mismatched tree is the only thing worth saying. An empty
	// engine never emits a hint, so its fix-it note always shows.
	status := m.cmp.Hint
	cta := ""
	if status == "" {
		status = m.cmp.Degraded
		// When the note is "could not detect the version", point at the field
		// that fixes it rather than leaving the operator to guess. Its own line,
		// so a narrow terminal does not truncate it away.
		if spec, ok := m.selected(); ok && status != "" && spec.Commands.MCVersion == "" {
			cta = "set it with  a → Launch settings"
		}
	}
	if status != "" {
		lines[0] = mutedStyle.Render(ansi.Truncate(status, max(w, 1), "…"))
	}
	if cta != "" {
		lines[1] = mutedStyle.Render(ansi.Truncate(cta, max(w, 1), "…"))
	}

	base := 1 // first row below the status line
	if cta != "" {
		base = 2
	}
	rows := completionPanelH - base // suggestion rows below the status line
	sugs := m.cmp.Suggestions
	start := 0
	if len(sugs) > rows {
		// Keep the highlight roughly centred in the window.
		start = m.cmpSel - rows/2
		start = clampInt(start, 0, len(sugs)-rows)
	}
	for i := 0; i < rows && start+i < len(sugs); i++ {
		idx := start + i
		s := sugs[idx]
		text := s.Text
		if s.Detail != "" {
			text += "  " + mutedStyle.Render(s.Detail)
		}
		marker := "  "
		if idx == m.cmpSel {
			marker = suggestSelStyle.Render("▸ ")
			text = suggestSelStyle.Render(s.Text)
			if s.Detail != "" {
				text += "  " + mutedStyle.Render(s.Detail)
			}
		}
		lines[base+i] = ansi.Truncate(marker+text, max(w, 1), "…")
	}

	return strings.Join(lines, "\n")
}

// consoleHistoryCmd persists the recall ring for id after a command was sent.
// Runs off the render path; a write failure is not worth interrupting the
// operator over.
func (m *model) consoleHistoryCmd(id server.ID) tea.Cmd {
	h, dirs := m.history, m.app.Dirs
	if h == nil {
		return nil
	}
	return func() tea.Msg {
		_ = h.Save(dirs.HistoryFile(id))
		return nil
	}
}
