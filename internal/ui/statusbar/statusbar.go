// Package statusbar renders kctl's persistent context/namespace bar.
package statusbar

import (
	"strings"

	"github.com/mo-pankaj/kctl/internal/theme"
)

// Model is the status bar state. It is a pure renderer: the root model sets the
// fields, and View draws them.
type Model struct {
	Context    string
	Namespace  string
	Message    string
	Connecting bool
	Width      int

	styles theme.Styles
}

// New builds a status bar.
func New(styles theme.Styles) (m Model) {
	m = Model{styles: styles}
	return m
}

// remaining reports how many columns are left for the message.
func (m Model) remaining(used int) (n int) {
	// Rendered styles inflate Len with escape codes, so measure against the
	// visible text rather than the buffer: context + namespace + padding.
	visible := len("ctx ") + len(m.Context) + len("  ns ") + len(namespaceLabel(m.Namespace))
	if m.Connecting {
		visible += len("  ⟳ connecting")
	}

	n = m.Width - visible - 4
	if n < 12 {
		n = 12
	}

	return n
}

// namespaceLabel renders the namespace, naming the all-namespaces case.
func namespaceLabel(ns string) (s string) {
	s = ns
	if s == "" {
		s = "all namespaces"
	}

	return s
}

// truncate shortens s to at most n columns.
func truncate(s string, n int) (out string) {
	out = s
	if len(out) > n {
		out = out[:n-1] + "…"
	}

	return out
}

// View renders the bar.
func (m Model) View() (s string) {
	var b strings.Builder

	b.WriteString(m.styles.StatusKey.Render("ctx "))
	b.WriteString(m.styles.StatusValue.Render(m.Context))

	b.WriteString(m.styles.StatusKey.Render("  ns "))
	b.WriteString(m.styles.StatusValue.Render(namespaceLabel(m.Namespace)))

	if m.Connecting {
		b.WriteString(m.styles.StatusWarn.Render("  ⟳ connecting"))
	}

	if m.Message != "" {
		// Truncated to what is left on the line. An untruncated cluster error
		// wraps and pushes the view below it off the screen, which looks like
		// the list emptying rather than a message appearing.
		b.WriteString(m.styles.StatusError.Render("  " + truncate(m.Message, m.remaining(b.Len()))))
	}

	s = m.styles.StatusBar.Width(m.Width).Render(b.String())
	return s
}
