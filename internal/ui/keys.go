package ui

import (
	"charm.land/bubbles/v2/key"

	"github.com/ivuorinen/nitpicker-tui/internal/findings"
)

// keyMap holds this model's own bindings; the list and the viewport bring
// their own for moving around.
type keyMap struct {
	Search    key.Binding
	Accept    key.Binding
	Cancel    key.Binding
	Up        key.Binding
	Down      key.Binding
	State     key.Binding
	Lens      key.Binding
	Severity  key.Binding
	Category  key.Binding
	Open      key.Binding
	Warnings  key.Binding
	Rescan    key.Binding
	Back      key.Binding
	Quit      key.Binding
	ForceQuit key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Search:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Accept:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply")),
		Cancel:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		Up:       key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("↓", "down")),
		State:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "state")),
		Lens:     key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "lens")),
		Severity: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "severity")),
		Category: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "category")),
		Open:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Warnings: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "warnings")),
		Rescan:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rescan")),
		Back:     key.NewBinding(key.WithKeys("esc", "q", "backspace"), key.WithHelp("esc", "back")),
		Quit:     key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		// ForceQuit works everywhere, even while typing.
		ForceQuit: key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	}
}

// listHelp is what the list's help line shows besides its own movement keys.
func (k keyMap) listHelp() []key.Binding {
	return []key.Binding{k.Search, k.State, k.Lens, k.Severity, k.Category, k.Open, k.Rescan, k.Warnings, k.Quit}
}

// facets pairs each facet with the key that opens its picker.
func (k keyMap) facets() map[findings.Facet]key.Binding {
	return map[findings.Facet]key.Binding{
		findings.State:    k.State,
		findings.Lens:     k.Lens,
		findings.Severity: k.Severity,
		findings.Category: k.Category,
	}
}
