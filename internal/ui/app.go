// Package ui holds the Bubble Tea root model and the views it hosts.
package ui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/cmdbar"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
	"github.com/mo-pankaj/kctl/internal/ui/stack"
	"github.com/mo-pankaj/kctl/internal/ui/statusbar"
	"github.com/mo-pankaj/kctl/internal/ui/views/ctxpicker"
	"github.com/mo-pankaj/kctl/internal/ui/views/logview"
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
	cmdBar   cmdbar.Model
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
		cmdBar:   cmdbar.New(styles),
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

	case podlist.SelectedMsg:
		container := ""
		if len(msg.Pod.Containers) > 0 {
			container = msg.Pod.Containers[0]
		}

		req := core.LogRequest{
			Namespace: msg.Pod.Namespace,
			Pod:       msg.Pod.Name,
			Container: container,
			Follow:    true,
		}

		m.stack.Push(logview.New(m.logger, m.styles, m.keys, m.pods, req))

		return m, m.stack.Top().Init()

	case cmdbar.SubmitMsg:
		return m.runCommand(msg.Command)

	case tea.KeyPressMsg:
		if m.cmdBar.Focused() {
			m.cmdBar, cmd = m.cmdBar.Update(msg)

			return m, cmd
		}

		if key.Matches(msg, m.keys.Command) {
			m.cmdBar.Open()

			return m, cmd
		}

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
				// Close before popping: a log view holds a live stream, and
				// dropping it without closing leaks a connection per pod visited.
				closer, ok := m.stack.Top().(interface{ Close() })
				if ok {
					closer.Close()
				}

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

// runCommand applies a parsed command bar command.
func (m Model) runCommand(c cmdbar.Command) (model tea.Model, cmd tea.Cmd) {
	switch c.Verb {
	case cmdbar.VerbContext:
		m.stack.Push(ctxpicker.New(m.logger, m.styles, m.keys, m.contexts))

		return m, m.stack.Top().Init()

	case cmdbar.VerbNamespace:
		namespace := c.Arg
		if namespace == "all" {
			namespace = ""
		}

		return m.switchNamespace(namespace)
	}

	return m, cmd
}

// switchNamespace re-scopes the pod list. Task 12 replaces this with the full
// teardown/resync sequence once sources are rebuilt per context.
func (m Model) switchNamespace(namespace string) (model tea.Model, cmd tea.Cmd) {
	m.sel.Namespace = namespace
	m.status.Namespace = namespace
	m.stack.Replace(podlist.New(m.logger, m.styles, m.keys, m.pods, m.sel))

	cmd = m.stack.Top().Init()

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

	help := m.styles.Help.Render("  " + keys.HelpLine(m.keys.Command, m.keys.Quit))

	bar := m.cmdBar.View()
	if bar != "" {
		bar += "\n"
	}

	v = tea.NewView(m.status.View() + "\n" + bar + body + "\n" + help)
	v.AltScreen = true

	return v
}
