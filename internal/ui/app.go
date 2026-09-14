// Package ui holds the Bubble Tea root model and the views it hosts.
package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

// Model is the kctl root model.
type Model struct {
	logger *zap.Logger
	keys   keys.Map
	width  int
	height int
}

// New builds the root model.
func New(logger *zap.Logger) (m Model) {
	m = Model{
		logger: logger.With(zap.String("component", "root")),
		keys:   keys.Default(),
	}
	return m
}

// Init satisfies tea.Model.
func (m Model) Init() (cmd tea.Cmd) {
	return cmd
}

// Update satisfies tea.Model.
func (m Model) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, cmd

	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
	}

	return m, cmd
}

// View satisfies tea.Model.
func (m Model) View() (s string) {
	help := m.keys.Quit.Help()
	s = fmt.Sprintf("kctl\n\npress %s to %s\n", help.Key, help.Desc)
	return s
}
