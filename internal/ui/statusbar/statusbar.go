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

// View renders the bar.
func (m Model) View() (s string) {
	var b strings.Builder

	b.WriteString(m.styles.StatusKey.Render("ctx "))
	b.WriteString(m.styles.StatusValue.Render(m.Context))

	b.WriteString(m.styles.StatusKey.Render("  ns "))
	namespace := m.Namespace
	if namespace == "" {
		namespace = "all namespaces"
	}
	b.WriteString(m.styles.StatusValue.Render(namespace))

	if m.Connecting {
		b.WriteString(m.styles.StatusWarn.Render("  ⟳ connecting"))
	}

	if m.Message != "" {
		b.WriteString(m.styles.StatusError.Render("  " + m.Message))
	}

	s = m.styles.StatusBar.Width(m.Width).Render(b.String())
	return s
}
