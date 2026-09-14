// Package ui holds the Bubble Tea root model and the views it hosts.
package ui

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
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

	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
	}

	return m, cmd
}

// View satisfies tea.Model.
//
// The root model is the only view that sets AltScreen: in Bubble Tea v2 the
// alternate screen is a property of the rendered view rather than a program
// option, and nested views must not fight the root over it.
func (m Model) View() (v tea.View) {
	help := m.keys.Quit.Help()
	body := fmt.Sprintf("kctl\n\npress %s to %s\n", help.Key, help.Desc)

	v = tea.NewView(body)
	v.AltScreen = true

	return v
}
