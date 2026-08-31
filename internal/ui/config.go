package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/syoopie/beacon-tui/internal/mcprops"
	"github.com/syoopie/beacon-tui/internal/server"
)

// fieldKind is how a server.properties value is edited: free text, one of a
// fixed set, or a boolean.
type fieldKind int

const (
	fieldText fieldKind = iota
	fieldEnum
	fieldBool
)

type propField struct {
	key     string
	label   string
	kind    fieldKind
	choices []string // fieldEnum only
	def     string   // value shown when the key is absent from the file
}

// configFields is the curated set of server.properties keys Beacon surfaces.
// Every other key in the file is left untouched by an edit.
var configFields = []propField{
	{key: "server-port", label: "Port", kind: fieldText, def: "25565"},
	{key: "motd", label: "MOTD", kind: fieldText, def: "A Minecraft Server"},
	{key: "difficulty", label: "Difficulty", kind: fieldEnum, choices: []string{"peaceful", "easy", "normal", "hard"}, def: "easy"},
	{key: "max-players", label: "Max players", kind: fieldText, def: "20"},
	{key: "enable-rcon", label: "Enable RCON", kind: fieldBool, def: "false"},
	{key: "rcon.port", label: "RCON port", kind: fieldText, def: "25575"},
	{key: "rcon.password", label: "RCON password", kind: fieldText, def: ""},
}

func boolChoices() []string { return []string{"false", "true"} }

// configForm is the modal for editing a server's server.properties. It holds
// the value being edited for every field, plus the value each started at so a
// save writes only what actually changed.
type configForm struct {
	id     server.ID
	values []string
	orig   []string
	cursor int
	input  textinput.Model
}

// openConfig loads the server's server.properties and opens the editor on it.
func (m *model) openConfig(spec server.Spec) tea.Cmd {
	props, err := mcprops.LoadProperties(spec.Dir)
	if err != nil {
		m.status = "config: " + err.Error()
		return nil
	}
	values := make([]string, len(configFields))
	for i, f := range configFields {
		values[i] = props.GetOr(f.key, f.def)
	}
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 256

	cf := &configForm{
		id:     spec.ID,
		values: values,
		orig:   append([]string(nil), values...),
		input:  ti,
	}
	cf.focusField()
	m.config = cf
	m.status = "editing " + string(spec.ID) + "'s server.properties"
	m.relayout()
	return textinput.Blink
}

// focusField points the shared text input at the current field, or blurs it for
// a field that changes with the arrow keys instead.
func (cf *configForm) focusField() {
	f := configFields[cf.cursor]
	if f.kind != fieldText {
		cf.input.Blur()
		return
	}
	if f.key == "rcon.password" {
		cf.input.EchoMode = textinput.EchoPassword
	} else {
		cf.input.EchoMode = textinput.EchoNormal
	}
	cf.input.SetValue(cf.values[cf.cursor])
	cf.input.CursorEnd()
	cf.input.Focus()
}

func (cf *configForm) syncText() {
	if configFields[cf.cursor].kind == fieldText {
		cf.values[cf.cursor] = cf.input.Value()
	}
}

func (cf *configForm) valueOf(key string) string {
	for i, f := range configFields {
		if f.key == key {
			return cf.values[i]
		}
	}
	return ""
}

func (cf *configForm) changed(i int) bool {
	return strings.TrimSpace(cf.values[i]) != strings.TrimSpace(cf.orig[i])
}

// rconField reports whether a field is part of the RCON group, which is written
// as a unit: turning enable-rcon on has to persist the port and password with
// it, even when they still hold the values the form showed.
func rconField(key string) bool {
	return key == "enable-rcon" || strings.HasPrefix(key, "rcon.")
}

// edits returns the key=value pairs to write, after checking that numeric fields
// are numbers and RCON has a password when on.
func (cf *configForm) edits() (map[string]string, error) {
	rconTouched := false
	for i, f := range configFields {
		if rconField(f.key) && cf.changed(i) {
			rconTouched = true
		}
	}

	out := map[string]string{}
	for i, f := range configFields {
		v := strings.TrimSpace(cf.values[i])
		if !cf.changed(i) && (!rconTouched || !rconField(f.key)) {
			continue
		}
		switch f.key {
		case "server-port", "rcon.port":
			if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 65535 {
				return nil, fmt.Errorf("%s must be a port number between 1 and 65535", f.label)
			}
		case "max-players":
			if n, err := strconv.Atoi(v); err != nil || n < 1 {
				return nil, fmt.Errorf("%s must be a positive whole number", f.label)
			}
		}
		out[f.key] = v
	}
	if strings.EqualFold(cf.valueOf("enable-rcon"), "true") && strings.TrimSpace(cf.valueOf("rcon.password")) == "" {
		return nil, fmt.Errorf("RCON needs a password; Minecraft will not open it without one")
	}
	return out, nil
}

func (m *model) updateConfig(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	cf := m.config
	f := configFields[cf.cursor]
	switch msg.String() {
	case "esc":
		m.config = nil
		m.status = "config left unchanged"
		m.relayout()
		return m, nil
	case "up", "down":
		cf.syncText()
		if msg.String() == "up" && cf.cursor > 0 {
			cf.cursor--
		} else if msg.String() == "down" && cf.cursor < len(configFields)-1 {
			cf.cursor++
		}
		cf.focusField()
		return m, nil
	case "left", "right":
		if f.kind != fieldText {
			choices := f.choices
			if f.kind == fieldBool {
				choices = boolChoices()
			}
			cf.values[cf.cursor] = cycleChoice(choices, cf.values[cf.cursor], msg.String() == "right")
		}
		return m, nil
	case "enter":
		cf.syncText()
		edits, err := cf.edits()
		if err != nil {
			m.status = "config: " + err.Error()
			return m, nil
		}
		id := cf.id
		m.config = nil
		m.relayout()
		if len(edits) == 0 {
			m.status = string(id) + ": no changes"
			return m, nil
		}
		m.busy = true
		m.status = "saving " + string(id) + "'s config…"
		return m, m.applyConfigCmd(id, edits)
	}
	if f.kind == fieldText {
		ti, cmd := cf.input.Update(msg)
		cf.input = ti
		cf.values[cf.cursor] = ti.Value()
		return m, cmd
	}
	return m, nil
}

func cycleChoice(choices []string, cur string, forward bool) string {
	idx := 0
	for i, c := range choices {
		if strings.EqualFold(c, cur) {
			idx = i
			break
		}
	}
	if forward {
		idx = (idx + 1) % len(choices)
	} else {
		idx = (idx - 1 + len(choices)) % len(choices)
	}
	return choices[idx]
}

// applyConfigCmd writes the edits through the lifecycle manager, which holds the
// host lock, rewrites server.properties, and mirrors the port and RCON block
// into the spec.
func (m *model) applyConfigCmd(id server.ID, edits map[string]string) tea.Cmd {
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
			return opDoneMsg{id: id, label: "config", err: fmt.Errorf("%s is no longer in the registry", id)}
		}
		if _, err := mgr.EditProperties(target, edits); err != nil {
			return opDoneMsg{id: id, label: "config", err: err}
		}
		return opDoneMsg{id: id, label: string(id) + ": config saved"}
	}
}

func (m *model) acceptEULACmd(spec server.Spec) tea.Cmd {
	mgr := m.app.Mgr
	return func() tea.Msg {
		if err := mgr.AcceptEULA(spec); err != nil {
			return opDoneMsg{id: spec.ID, label: "accept EULA", err: err}
		}
		return opDoneMsg{id: spec.ID, label: string(spec.ID) + ": EULA accepted"}
	}
}

// configDialogView renders the editor as a centred modal, in the same shape as
// the launch-settings dialog.
func (m *model) configDialogView() string {
	cf := m.config
	width := clampInt(m.bodyW-12, 44, 72)

	rows := []string{
		sectionStyle.Render("Edit " + string(cf.id) + "'s server.properties"),
		mutedStyle.Render("Writes only the keys you change; the rest of the file is left alone."),
		"",
	}
	labelStyle := lipgloss.NewStyle().Width(15)
	for i, f := range configFields {
		marker := "  "
		display := cf.values[i]
		if f.key == "rcon.password" && display != "" {
			display = strings.Repeat("•", len(display))
		}
		if i == cf.cursor {
			marker = "▸ "
			switch f.kind {
			case fieldText:
				display = cf.input.View()
			default:
				display = "‹ " + cf.values[i] + " ›"
			}
		}
		label := labelStyle.Render(f.label)
		if i == cf.cursor {
			label = selectedRow.Render(labelStyle.Render(f.label))
		}
		rows = append(rows, marker+label+display)
	}
	rows = append(rows,
		"",
		m.hintBar(hint("↑↓", "field"), hint("←→", "change"), hint("enter", "save"), hint("esc", "cancel")),
	)
	inner := lipgloss.NewStyle().Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	return lipgloss.Place(m.bodyW, m.bodyH, lipgloss.Center, lipgloss.Center, dialogStyle.Render(inner))
}
