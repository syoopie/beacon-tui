package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/syoopie/beacon-tui/internal/config"
	"github.com/syoopie/beacon-tui/internal/importdetect"
	"github.com/syoopie/beacon-tui/internal/server"
)

// launchPrompt is the modal for choosing which script or jar starts a server
// and what arguments it is given.
type launchPrompt struct {
	id     server.ID
	opts   []importdetect.LaunchOption
	cursor int
	args   textinput.Model
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
	ti.Focus()

	m.launch = &launchPrompt{id: spec.ID, opts: opts, cursor: cursor, args: ti}
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
		if lp.cursor > 0 {
			lp.cursor--
		}
		return m, nil
	case "down":
		if lp.cursor < len(lp.opts)-1 {
			lp.cursor++
		}
		return m, nil
	case "enter":
		opt := lp.opts[lp.cursor]
		args := strings.TrimSpace(lp.args.Value())
		m.launch = nil
		m.busy = true
		m.status = "saving launch settings…"
		m.relayout()
		return m, m.applyLaunchCmd(lp.id, opt, args)
	}
	ti, cmd := lp.args.Update(msg)
	lp.args = ti
	return m, cmd
}

// applyLaunchCmd rewrites the selected server's launch command, start script and
// exec state, then saves the spec.
func (m *model) applyLaunchCmd(id server.ID, opt importdetect.LaunchOption, args string) tea.Cmd {
	dirs, specs := m.app.Dirs, m.specs
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
		if err := config.SaveSpec(dirs, target); err != nil {
			return opDoneMsg{id: id, label: "launch settings", err: err}
		}
		return opDoneMsg{id: id, label: string(id) + " now starts with  " + target.Start}
	}
}
