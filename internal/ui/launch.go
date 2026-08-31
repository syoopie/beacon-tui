package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/syoopie/beacon-tui/internal/importdetect"
	"github.com/syoopie/beacon-tui/internal/server"
)

// launchPrompt is the modal for choosing which script or jar starts a server,
// what arguments it is given, and which Minecraft version it runs. The version
// feeds console command completion; Beacon detects it at import and this is
// where the operator corrects a guess it got wrong or could not make.
//
// The cursor runs over one flat list of rows: the launch options first, then
// the arguments field, then the version field. chosen is the committed launch
// method; the cursor moves freely without disturbing it (see pick).
type launchPrompt struct {
	id      server.ID
	opts    []importdetect.LaunchOption
	chosen  int
	cursor  int
	args    textinput.Model
	version textinput.Model
}

func (lp *launchPrompt) argsRow() int    { return len(lp.opts) }
func (lp *launchPrompt) versionRow() int { return len(lp.opts) + 1 }

// refocus points the keyboard at whatever the cursor is on: the arguments
// field, the version field, or nothing while it sits on an option row.
func (lp *launchPrompt) refocus() {
	lp.args.Blur()
	lp.version.Blur()
	switch lp.cursor {
	case lp.argsRow():
		lp.args.Focus()
	case lp.versionRow():
		lp.version.Focus()
	}
}

// move walks the cursor over the flat row list and clamps at both ends. The
// selection does not follow it: scrolling past an option to reach the version
// field must not change which launch method is saved.
func (lp *launchPrompt) move(delta int) {
	lp.cursor = clampInt(lp.cursor+delta, 0, lp.versionRow())
	lp.refocus()
}

// pick reports the launch option the save should use: the one under the cursor
// while it sits on an option row, otherwise the last one explicitly chosen.
func (lp *launchPrompt) pick() int {
	if lp.cursor < len(lp.opts) {
		return lp.cursor
	}
	return lp.chosen
}

// openLaunch reads the launch methods present in the server's directory and
// opens the picker on the one the spec currently uses.
func (m *model) openLaunch(spec server.Spec) tea.Cmd {
	opts := importdetect.LaunchOptions(spec.Dir)
	if len(opts) == 0 {
		m.status = "no run.sh, start.sh or server jar found in " + spec.Dir
		return nil
	}

	cursor := 0
	for i, o := range opts {
		if strings.HasPrefix(spec.Start, o.Base) {
			cursor = i
			break
		}
	}
	args := strings.TrimSpace(strings.TrimPrefix(spec.Start, opts[cursor].Base))
	if args == "" {
		args = "nogui" // Beacon is the console; the server should not open its own window
	}

	ti := textinput.New()
	ti.Prompt = "arguments  "
	ti.Placeholder = "e.g. nogui"
	ti.CharLimit = 256
	ti.SetValue(args)

	vi := textinput.New()
	vi.Prompt = "MC version  "
	vi.Placeholder = "e.g. 1.20.1"
	vi.CharLimit = 16
	vi.SetValue(spec.Commands.MCVersion)

	lp := &launchPrompt{id: spec.ID, opts: opts, chosen: cursor, cursor: cursor, args: ti, version: vi}
	lp.refocus()
	m.launch = lp
	m.status = "launch settings for " + string(spec.ID)
	m.relayout()
	return textinput.Blink
}

func (m *model) updateLaunch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	lp := m.launch
	switch msg.String() {
	case "esc":
		m.launch = nil
		m.status = "launch settings unchanged"
		m.relayout()
		return m, nil
	case "up":
		lp.move(-1)
		return m, nil
	case "down":
		lp.move(1)
		return m, nil
	case " ":
		// Space fixes the selection without leaving, so the operator can pick a
		// method and then go edit the arguments before saving.
		if lp.cursor < len(lp.opts) {
			lp.chosen = lp.cursor
			return m, nil
		}
	case "enter":
		version := strings.TrimSpace(lp.version.Value())
		if version != "" && !server.ValidMCVersion(version) {
			m.status = "launch settings: MC version should look like 1.20.1"
			return m, nil
		}
		opt := lp.opts[lp.pick()]
		args := strings.TrimSpace(lp.args.Value())
		id := lp.id
		m.launch = nil
		m.actions = nil
		m.busy = true
		m.status = "saving launch settings…"
		m.relayout()
		return m, m.applyLaunchCmd(id, opt, args, version)
	}

	switch lp.cursor {
	case lp.argsRow():
		ti, cmd := lp.args.Update(msg)
		lp.args = ti
		return m, cmd
	case lp.versionRow():
		vi, cmd := lp.version.Update(msg)
		lp.version = vi
		return m, cmd
	}
	return m, nil
}

// applyLaunchCmd rewrites the selected server's launch command, start script,
// exec state and detected Minecraft version, then saves the spec through the
// manager's host lock.
func (m *model) applyLaunchCmd(id server.ID, opt importdetect.LaunchOption, args, version string) tea.Cmd {
	mgr, specs := m.app.Mgr, m.specs
	return func() tea.Msg {
		var target server.Spec
		found := false
		for _, s := range specs {
			if s.ID == id {
				target, found = s, true
				break
			}
		}
		if !found {
			return opDoneMsg{id: id, label: "launch settings", err: fmt.Errorf("%s is no longer in the registry", id)}
		}

		target.Script = opt.Script
		target.Start = opt.Command(args)
		target.Exec = opt.Exec
		target.Commands.MCVersion = version
		if _, err := mgr.SaveSpec(target); err != nil {
			return opDoneMsg{id: id, label: "launch settings", err: err}
		}
		return opDoneMsg{id: id, label: string(id) + " now starts with  " + target.Start}
	}
}
