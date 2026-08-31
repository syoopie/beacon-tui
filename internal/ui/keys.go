package ui

import "github.com/charmbracelet/bubbles/key"

// keymap is Beacon's full set of bindings. The command bar shows only the
// subset that applies to the current screen, but every binding here stays live
// so a key press always gets an answer, even if that answer is "not now".
type keymap struct {
	Up        key.Binding
	Down      key.Binding
	Enter     key.Binding
	Act       key.Binding
	Back      key.Binding
	Power     key.Binding
	Kill      key.Binding
	Actions   key.Binding
	Console   key.Binding
	LogTab    key.Binding
	LogFilter key.Binding
	LogSearch key.Binding
	Add       key.Binding
	Rescan    key.Binding
	Help      key.Binding
	Quit      key.Binding
}

func newKeymap() keymap {
	return keymap{
		Up:        key.NewBinding(key.WithKeys("up"), key.WithHelp("↑", "up")),
		Down:      key.NewBinding(key.WithKeys("down"), key.WithHelp("↓", "down")),
		Enter:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Act:       key.NewBinding(key.WithKeys("right", "enter"), key.WithHelp("→", "console")),
		Back:      key.NewBinding(key.WithKeys("esc", "left"), key.WithHelp("←", "back")),
		Power:     key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "start/stop")),
		Kill:      key.NewBinding(key.WithKeys("K"), key.WithHelp("K", "force-kill")),
		Actions:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "actions")),
		Console:   key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "type a command")),
		LogTab:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch tab")),
		LogFilter: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "important only")),
		LogSearch: key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Add:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add server")),
		Rescan:    key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "scan folders")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more keys")),
		Quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
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
