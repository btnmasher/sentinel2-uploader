package keyboard

import "github.com/charmbracelet/bubbles/key"

type Map struct {
	NextFocus   key.Binding
	PrevFocus   key.Binding
	PrevTab     key.Binding
	NextTab     key.Binding
	Activate    key.Binding
	Quit        key.Binding
	ModalToggle key.Binding
}

func New() Map {
	return Map{
		NextFocus: key.NewBinding(
			key.WithKeys("tab", "down"),
			key.WithHelp("tab/down", "next"),
		),
		PrevFocus: key.NewBinding(
			key.WithKeys("shift+tab", "up"),
			key.WithHelp("shift+tab/up", "prev"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("ctrl+left"),
			key.WithHelp("ctrl+left", "overview"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("ctrl+right"),
			key.WithHelp("ctrl+right", "settings"),
		),
		Activate: key.NewBinding(
			key.WithKeys("enter", " "),
			key.WithHelp("enter/space", "activate"),
		),
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		ModalToggle: key.NewBinding(
			key.WithKeys("tab", "up", "down", "left", "right"),
			key.WithHelp("tab/arrows", "toggle"),
		),
	}
}

func (m Map) ShortHelp() []key.Binding {
	return []key.Binding{m.NextFocus, m.Activate, m.PrevTab, m.NextTab, m.Quit}
}

func (m Map) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{m.NextFocus, m.PrevFocus, m.Activate},
		{m.PrevTab, m.NextTab, m.Quit},
	}
}
