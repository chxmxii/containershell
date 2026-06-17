package dashboard

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keyboard shortcuts for the dashboard.
type KeyMap struct {
	Quit    key.Binding
	Tab     key.Binding
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	Shell   key.Binding
	Logs    key.Binding
	Inspect key.Binding
	Top     key.Binding
	Env     key.Binding
	Netstat key.Binding
	Refresh key.Binding
	Sort    key.Binding
	Filter  key.Binding
	Help    key.Binding
	Escape  key.Binding
}

// DefaultKeyMap returns the default key bindings for the dashboard.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q/ctrl+c", "quit"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch panel"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "shell into container"),
		),
		Shell: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "shell into container"),
		),
		Logs: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "view logs"),
		),
		Inspect: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "inspect container"),
		),
		Top: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "process list"),
		),
		Env: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "environment variables"),
		),
		Netstat: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "network connections"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh now"),
		),
		Sort: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "cycle sort field"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "close/cancel"),
		),
	}
}
