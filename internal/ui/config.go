package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
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
	key      string
	label    string
	kind     fieldKind
	choices  []string           // fieldEnum only
	def      string             // value shown when the key is absent from the file
	section  string             // section header this field sits under
	validate func(string) error // nil accepts anything; run only for changed fields
}

// portValue accepts a TCP port number.
func portValue(label string) func(string) error {
	return func(v string) error {
		if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("%s must be a port number between 1 and 65535", label)
		}
		return nil
	}
}

func intAtLeast(label string, low int) func(string) error {
	return func(v string) error {
		if n, err := strconv.Atoi(v); err != nil || n < low {
			return fmt.Errorf("%s must be a whole number of at least %d", label, low)
		}
		return nil
	}
}

func intRange(label string, low, high int) func(string) error {
	return func(v string) error {
		if n, err := strconv.Atoi(v); err != nil || n < low || n > high {
			return fmt.Errorf("%s must be a whole number between %d and %d", label, low, high)
		}
		return nil
	}
}

// configFields is the curated set of server.properties keys Beacon surfaces,
// grouped into the sections the editor draws as headers. Every other key in the
// file is left untouched by an edit.
var configFields = []propField{
	{section: "General", key: "server-port", label: "Port", kind: fieldText, def: "25565", validate: portValue("Port")},
	{section: "General", key: "motd", label: "MOTD", kind: fieldText, def: "A Minecraft Server"},
	{section: "General", key: "max-players", label: "Max players", kind: fieldText, def: "20", validate: intAtLeast("Max players", 1)},

	{section: "Gameplay", key: "difficulty", label: "Difficulty", kind: fieldEnum, choices: []string{"peaceful", "easy", "normal", "hard"}, def: "easy"},
	{section: "Gameplay", key: "gamemode", label: "Game mode", kind: fieldEnum, choices: []string{"survival", "creative", "adventure", "spectator"}, def: "survival"},
	{section: "Gameplay", key: "hardcore", label: "Hardcore", kind: fieldBool, def: "false"},
	{section: "Gameplay", key: "pvp", label: "PvP", kind: fieldBool, def: "true"},
	{section: "Gameplay", key: "spawn-protection", label: "Spawn protect", kind: fieldText, def: "16", validate: intAtLeast("Spawn protect", 0)},
	{section: "Gameplay", key: "allow-flight", label: "Allow flight", kind: fieldBool, def: "false"},
	{section: "Gameplay", key: "force-gamemode", label: "Force gamemode", kind: fieldBool, def: "false"},
	{section: "Gameplay", key: "enable-command-block", label: "Command blocks", kind: fieldBool, def: "false"},

	{section: "Access", key: "online-mode", label: "Online mode", kind: fieldBool, def: "true"},
	{section: "Access", key: "white-list", label: "Whitelist", kind: fieldBool, def: "false"},
	{section: "Access", key: "enforce-whitelist", label: "Enforce list", kind: fieldBool, def: "false"},

	{section: "World", key: "level-seed", label: "Level seed", kind: fieldText, def: ""},
	{section: "World", key: "level-type", label: "Level type", kind: fieldText, def: "minecraft:normal"},
	{section: "World", key: "view-distance", label: "View distance", kind: fieldText, def: "10", validate: intRange("View distance", 3, 32)},
	{section: "World", key: "simulation-distance", label: "Sim distance", kind: fieldText, def: "10", validate: intRange("Sim distance", 3, 32)},
	{section: "World", key: "allow-nether", label: "Nether", kind: fieldBool, def: "true"},
	{section: "World", key: "spawn-monsters", label: "Spawn monsters", kind: fieldBool, def: "true"},

	{section: "RCON", key: "enable-rcon", label: "Enable RCON", kind: fieldBool, def: "false"},
	{section: "RCON", key: "rcon.port", label: "RCON port", kind: fieldText, def: "25575", validate: portValue("RCON port")},
	{section: "RCON", key: "rcon.password", label: "RCON password", kind: fieldText, def: ""},
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
	vp     viewport.Model
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
		vp:     viewport.New(1, 1),
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

// edits returns the key=value pairs to write, after running each changed field's
// validator and checking that RCON has a password when on.
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
		if f.validate != nil {
			if err := f.validate(v); err != nil {
				return nil, err
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
		m.actions = nil
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

// renderFields draws every field row, with a bold header wherever the section
// changes, and reports which rendered line the cursor's row landed on. Section
// headers exist only here: the cursor still steps field by field, so the line
// index is the only thing that has to bridge the two.
//
// Each row is clipped rather than wrapped, so one field is always one line and
// the reported index stays true.
func (cf *configForm) renderFields(width int) (string, int) {
	labelStyle := lipgloss.NewStyle().Width(15)
	clip := lipgloss.NewStyle().MaxWidth(max(width, 1))

	var rows []string
	cursorLine := 0
	section := ""
	for i, f := range configFields {
		if f.section != section {
			if section != "" {
				rows = append(rows, "")
			}
			rows = append(rows, clip.Render(sectionStyle.Render(f.section)))
			section = f.section
		}

		marker := "  "
		display := cf.values[i]
		if f.key == "rcon.password" && display != "" {
			display = strings.Repeat("•", len(display))
		}
		label := labelStyle.Render(f.label)
		if i == cf.cursor {
			cursorLine = len(rows)
			marker = "▸ "
			label = selectedRow.Render(label)
			switch f.kind {
			case fieldText:
				display = cf.input.View()
			default:
				display = "‹ " + cf.values[i] + " ›"
			}
		}
		rows = append(rows, clip.Render(marker+label+display))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...), cursorLine
}

// scrollToCursor refills the field viewport and scrolls it so the cursor row
// stays on screen. Every arrow key and every resize reaches the editor through a
// redraw, so this is the one place that owns the scroll offset.
func (cf *configForm) scrollToCursor(width, height int, content string, cursorLine int) {
	cf.vp.Width = width
	cf.vp.Height = height
	cf.vp.SetContent(content)
	switch {
	case cursorLine < cf.vp.YOffset:
		cf.vp.SetYOffset(cursorLine)
	case cursorLine >= cf.vp.YOffset+height:
		cf.vp.SetYOffset(cursorLine - height + 1)
	default:
		cf.vp.SetYOffset(cf.vp.YOffset) // re-clamp a stale offset after a resize
	}
}

// configDialogView renders the editor as a centred modal, in the same shape as
// the launch-settings dialog. The field list is too tall for a short terminal,
// so it scrolls inside a viewport while the title and hint bar stay put. The
// viewport takes whatever height is left once the fixed chrome is measured, so
// the modal always fits inside m.bodyH.
func (m *model) configDialogView() string {
	cf := m.config
	width := clampInt(m.bodyW-12, 44, 72)

	title := sectionStyle.Render("Edit " + string(cf.id) + "'s server.properties")
	subtitle := lipgloss.NewStyle().Width(width).Render(
		mutedStyle.Render("Writes only the keys you change; the rest of the file is left alone."))
	hints := m.hintBar(hint("↑↓", "field"), hint("←→", "change"), hint("enter", "save"), hint("esc", "cancel"))

	head := []string{title, subtitle}
	if n := m.restartNoticeRow(cf.id, width); n != "" {
		head = append(head, "", n)
	}
	head = append(head, "")

	content, cursorLine := cf.renderFields(width)
	// The dialog box adds a rounded border and one padding row top and bottom;
	// the head block, a blank spacer, and the hint bar sit around the viewport.
	chrome := 4 + lipgloss.Height(strings.Join(head, "\n")) + lipgloss.Height(hints) + 1
	vpHeight := clampInt(m.bodyH-chrome, 3, lipgloss.Height(content))
	cf.scrollToCursor(width, vpHeight, content, cursorLine)

	inner := lipgloss.NewStyle().Width(width).Render(lipgloss.JoinVertical(lipgloss.Left,
		append(head, cf.vp.View(), "", hints)...))
	return lipgloss.Place(m.bodyW, m.bodyH, lipgloss.Center, lipgloss.Center, dialogStyle.Render(inner))
}
