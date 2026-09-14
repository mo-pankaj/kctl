// Package keys holds every key binding kctl uses.
package keys

import (
	"strings"

	"charm.land/bubbles/v2/key"
)

// Map is the complete set of kctl key bindings.
type Map struct {
	Quit      key.Binding
	Filter    key.Binding
	SortCycle key.Binding
	Command   key.Binding
	Follow    key.Binding
	Retry     key.Binding
	Describe  key.Binding
	Apply     key.Binding
	Back      key.Binding
	Select    key.Binding
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
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		SortCycle: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "sort"),
		),
		Command: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "command"),
		),
		Follow: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "follow"),
		),
		Retry: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "retry"),
		),
		Describe: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "describe"),
		),
		Apply: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "apply"),
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
