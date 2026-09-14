// Package theme holds kctl's lipgloss styles.
//
// Styles live here so a colour change is one edit. Tests assert on rendered
// structure and text, never on ANSI escape codes, so retheming never breaks
// the suite.
package theme

import "charm.land/lipgloss/v2"

// Styles is the full palette.
type Styles struct {
	StatusBar     lipgloss.Style
	StatusKey     lipgloss.Style
	StatusValue   lipgloss.Style
	StatusWarn    lipgloss.Style
	StatusError   lipgloss.Style
	Help          lipgloss.Style
	TableHeader   lipgloss.Style
	TableSelected lipgloss.Style
	Dimmed        lipgloss.Style
}

// New returns the default styles.
func New() (s Styles) {
	s = Styles{
		StatusBar:     lipgloss.NewStyle().Padding(0, 1),
		StatusKey:     lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		StatusValue:   lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true),
		StatusWarn:    lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		StatusError:   lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		Help:          lipgloss.NewStyle().Foreground(lipgloss.Color("241")),
		TableHeader:   lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Bold(true),
		TableSelected: lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Background(lipgloss.Color("62")),
		Dimmed:        lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}
	return s
}
