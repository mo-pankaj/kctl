// Package ui holds the Bubble Tea root model and the views it hosts.
package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
	"github.com/mo-pankaj/kctl/internal/ui/stack"
	"github.com/mo-pankaj/kctl/internal/ui/statusbar"
	"github.com/mo-pankaj/kctl/internal/ui/views/ctxpicker"
	"github.com/mo-pankaj/kctl/internal/ui/views/podlist"
)

// inputFocuser is implemented by views that own a text input. While one reports
// true, global single-letter bindings are suppressed so typing works — otherwise
// typing "q" into a filter would quit the program.
type inputFocuser interface {
	InputFocused() bool
}

// Model is the kctl root model.
type Model struct {
	logger   *zap.Logger
	keys     keys.Map
	styles   theme.Styles
	contexts core.ContextManager
	pods     core.PodReader
	sel      core.Selector
	status   statusbar.Model
	stack    stack.Stack
	width    int
	height   int
}

// New builds the root model.
func New(logger *zap.Logger, contexts core.ContextManager, pods core.PodReader, sel core.Selector) (m Model) {
	styles := theme.New()

	m = Model{
		logger:   logger.With(zap.String("component", "root")),
		keys:     keys.Default(),
		styles:   styles,
		contexts: contexts,
		pods:     pods,
		sel:      sel,
		status:   statusbar.New(styles),
	}

	current := contexts.Current()
	m.status.Context = current.Name
	m.status.Namespace = current.Namespace

	m.stack.Push(podlist.New(m.logger, styles, m.keys, pods, sel))

	return m
}

// Init satisfies tea.Model.
func (m Model) Init() (cmd tea.Cmd) {
	top := m.stack.Top()
	if top != nil {
		cmd = top.Init()
	}

	return cmd
}

// Update satisfies tea.Model.
func (m Model) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.status.Width = msg.Width
		// Fall through so the visible view also learns the new size.

	case ctxpicker.SwitchedMsg:
		m.status.Context = msg.Context.Name
		m.status.Namespace = msg.Context.Namespace
		m.status.Message = ""

		return m, cmd

	case ctxpicker.ErrorMsg:
		m.status.Message = msg.Err.Error()
		// Fall through so the picker renders its own error state too.

	case tea.KeyPressMsg:
		focuser, ok := m.stack.Top().(inputFocuser)
		typing := ok && focuser.InputFocused()

		if !typing {
			if key.Matches(msg, m.keys.Quit) {
				return m, tea.Quit
			}

			if msg.String() == "c" {
				m.stack.Push(ctxpicker.New(m.logger, m.styles, m.keys, m.contexts))
				return m, m.stack.Top().Init()
			}

			if key.Matches(msg, m.keys.Back) {
				popped := m.stack.Pop()
				if popped {
					return m, cmd
				}
			}
		}
	}

	top := m.stack.Top()
	if top == nil {
		return m, cmd
	}

	updated, cmd := top.Update(msg)
	m.stack.Replace(updated)

	return m, cmd
}

// View satisfies tea.Model.
//
// Child views are tea.Models, so their View() returns a tea.View; the root
// embeds their rendered Content and owns AltScreen for the whole program.
func (m Model) View() (v tea.View) {
	body := "\n  no view loaded\n"

	top := m.stack.Top()
	if top != nil {
		body = top.View().Content
	}

	help := m.styles.Help.Render("  " + keys.HelpLine(m.keys.Quit))

	v = tea.NewView(m.status.View() + "\n" + body + "\n" + help)
	v.AltScreen = true

	return v
}
