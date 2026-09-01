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
	Chat      key.Binding
	Console   key.Binding
	LogTab    key.Binding
	LogFilter key.Binding
	LogSearch key.Binding
	LogBottom key.Binding
	Add       key.Binding
	Rescan    key.Binding
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
		Actions:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "settings")),
		Chat:      key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "type")),
		Console:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "command")),
		LogTab:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch tab")),
		LogFilter: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter")),
		LogSearch: key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("ctrl+f", "search")),
		LogBottom: key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("end", "latest")),
		Add:       key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add server")),
		Rescan:    key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "scan folders")),
	}
}

// helpSet is the set of bindings the command bar shows for the current mode.
// Every one is displayed; a bar too wide for the terminal wraps onto more rows
// rather than hiding keys behind a "more" toggle.
type helpSet struct {
	short []key.Binding
}

func hint(keys, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, desc))
}
