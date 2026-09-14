// Package ui holds the Bubble Tea root model and the views it hosts.
package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
	"github.com/mo-pankaj/kctl/internal/ui/statusbar"
)

// Model is the kctl root model.
type Model struct {
	logger   *zap.Logger
	keys     keys.Map
	styles   theme.Styles
	contexts core.ContextManager
	status   statusbar.Model
	width    int
	height   int
}

// New builds the root model.
func New(logger *zap.Logger, contexts core.ContextManager) (m Model) {
	styles := theme.New()

	m = Model{
		logger:   logger.With(zap.String("component", "root")),
		keys:     keys.Default(),
		styles:   styles,
		contexts: contexts,
		status:   statusbar.New(styles),
	}

	current := contexts.Current()
	m.status.Context = current.Name
	m.status.Namespace = current.Namespace

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
		m.status.Width = msg.Width
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
// The root model is the only one that sets AltScreen: in v2 the alternate
// screen is a property of the rendered view, not a program option.
func (m Model) View() (v tea.View) {
	m.status.Width = m.width

	body := "\n  no view loaded\n"
	help := m.styles.Help.Render("  " + keys.HelpLine(m.keys.Quit))

	v = tea.NewView(m.status.View() + "\n" + body + "\n" + help)
	v.AltScreen = true

	return v
}
