package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/syoopie/beacon-tui/internal/importdetect"
	"github.com/syoopie/beacon-tui/internal/javadetect"
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

	// javaChoices is the runtime picker on its own row: "System Java (PATH)"
	// first, then the host's JDKs, and the spec's current setting if it is not
	// among them. javaPick indexes it. javaKnown is false until detection lands.
	javaChoices []javaChoice
	javaPick    int
	javaKnown   bool
}

type javaChoice struct {
	label string
	path  string // "" means the java on PATH
}

func (lp *launchPrompt) argsRow() int    { return len(lp.opts) }
func (lp *launchPrompt) versionRow() int { return len(lp.opts) + 1 }
func (lp *launchPrompt) javaRow() int    { return len(lp.opts) + 2 }

// setJavas rebuilds the runtime choices once detection returns, keeping whatever
// path is selected now.
func (lp *launchPrompt) setJavas(jdks []javadetect.JDK) {
	current := lp.javaChoices[lp.javaPick].path
	lp.javaChoices = javaChoicesFor(current, jdks)
	lp.javaKnown = true
	lp.javaPick = 0
	for i, c := range lp.javaChoices {
		if c.path == current {
			lp.javaPick = i
			break
		}
	}
}

func (lp *launchPrompt) cycleJava(delta int) {
	n := len(lp.javaChoices)
	lp.javaPick = (lp.javaPick + delta%n + n) % n
}

func (lp *launchPrompt) javaLabel() string {
	return lp.javaChoices[lp.javaPick].label
}

func (lp *launchPrompt) javaNote() string {
	if !lp.javaKnown {
		return "looking for installed JDKs on this host..."
	}
	if c := lp.javaChoices[lp.javaPick]; c.path != "" {
		return c.path
	}
	return "the java found on PATH when Beacon starts a server"
}

// javaChoicesFor builds the picker list: PATH, then the current setting if it is
// not a discovered JDK, then the discovered JDKs.
func javaChoicesFor(current string, jdks []javadetect.JDK) []javaChoice {
	choices := []javaChoice{{label: "System Java (PATH)", path: ""}}
	seen := map[string]bool{"": true}
	known := false
	for _, j := range jdks {
		if j.Path == current {
			known = true
		}
	}
	if current != "" && !known {
		choices = append(choices, javaChoice{label: "Current setting", path: current})
		seen[current] = true
	}
	for _, j := range jdks {
		if seen[j.Path] {
			continue
		}
		choices = append(choices, javaChoice{label: j.Label, path: j.Path})
		seen[j.Path] = true
	}
	return choices
}

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
	lp.cursor = clampInt(lp.cursor+delta, 0, lp.javaRow())
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
	lp.javaChoices = javaChoicesFor(spec.Java, m.javas)
	lp.javaKnown = m.javasDone
	for i, c := range lp.javaChoices {
		if c.path == spec.Java {
			lp.javaPick = i
			break
		}
	}
	lp.refocus()
	m.launch = lp
	m.status = "launch settings for " + string(spec.ID)
	m.relayout()
	return tea.Batch(textinput.Blink, m.detectJavaCmd())
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
	case "left":
		if lp.cursor == lp.javaRow() {
			lp.cycleJava(-1)
		}
		return m, nil
	case "right":
		if lp.cursor == lp.javaRow() {
			lp.cycleJava(1)
		}
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
		java := lp.javaChoices[lp.javaPick].path
		id := lp.id
		m.launch = nil
		m.actions = nil
		m.busy = true
		m.status = "saving launch settings…"
		m.relayout()
		return m, m.applyLaunchCmd(id, opt, args, version, java)
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
// exec state, Java runtime and detected Minecraft version, then saves the spec
// through the manager's host lock.
func (m *model) applyLaunchCmd(id server.ID, opt importdetect.LaunchOption, args, version, java string) tea.Cmd {
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
		target.Java = java
		target.Commands.MCVersion = version
		if _, err := mgr.SaveSpec(target); err != nil {
			return opDoneMsg{id: id, label: "launch settings", err: err}
		}
		return opDoneMsg{id: id, label: string(id) + " now starts with  " + target.Start}
	}
}
