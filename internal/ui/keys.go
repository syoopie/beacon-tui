package ui

import "github.com/charmbracelet/bubbles/key"

// keymap is beacon's full set of bindings. The command bar shows only the
// subset that applies to the current screen and the selected server, but every
// binding here stays live so a key press always gets an answer, even if that
// answer is "not now".
type keymap struct {
	Up          key.Binding
	Down        key.Binding
	Start       key.Binding
	Stop        key.Binding
	Console     key.Binding
	Kill        key.Binding
	MarkStopped key.Binding
	Patch       key.Binding
	Add         key.Binding
	Rescan      key.Binding
	Update      key.Binding
	Refresh     key.Binding
	Help        key.Binding
	Quit        key.Binding
}

func newKeymap() keymap {
	return keymap{
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Start:       key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start")),
		Stop:        key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
		Console:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "console")),
		Kill:        key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "force-kill")),
		MarkStopped: key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mark stopped")),
		Patch:       key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "fix script")),
		Add:         key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add server")),
		Rescan:      key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "rescan folders")),
		Update:      key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "update")),
		Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Help:        key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more keys")),
		Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// helpSet adapts a slice of bindings to bubbles/help.KeyMap. ShortHelp is the
// one-line command bar; FullHelp is the grid shown after "?".
type helpSet struct {
	short []key.Binding
	full  [][]key.Binding
}

func (h helpSet) ShortHelp() []key.Binding { return h.short }

func (h helpSet) FullHelp() [][]key.Binding {
	if h.full != nil {
		return h.full
	}
	return [][]key.Binding{h.short}
}

func hint(keys, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, desc))
}
