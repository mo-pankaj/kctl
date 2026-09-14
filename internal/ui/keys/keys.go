// Package keys holds every key binding kctl uses.
package keys

import (
	"strings"

	"charm.land/bubbles/v2/key"
)

// Map is the complete set of kctl key bindings.
type Map struct {
	Quit   key.Binding
	Back   key.Binding
	Select key.Binding
}

// Default returns the standard binding set.
func Default() (m Map) {
	m = Map{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
	}
	return m
}

// HelpLine renders bindings as "key:desc", joined by two spaces.
//
// Every view's help text goes through here, so the displayed keys always follow
// the keymap and cannot drift from what the bindings actually do. Bindings with
// no help key are skipped, which lets callers pass a conditional binding without
// guarding at the call site.
func HelpLine(bindings ...key.Binding) (s string) {
	parts := make([]string, 0, len(bindings))

	for _, b := range bindings {
		h := b.Help()
		if h.Key == "" {
			continue
		}

		parts = append(parts, h.Key+":"+h.Desc)
	}

	s = strings.Join(parts, "  ")
	return s
}
