// Package keys holds every key binding kctl uses.
package keys

import "charm.land/bubbles/v2/key"

// Map is the complete set of kctl key bindings.
type Map struct {
	Quit key.Binding
}

// Default returns the standard binding set.
func Default() (m Map) {
	m = Map{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
	return m
}
