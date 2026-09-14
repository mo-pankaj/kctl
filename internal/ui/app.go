// Package ui holds the Bubble Tea root model and the views it hosts.
package ui

import (
	"context"
	"time"

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

// healthReporter is implemented by data sources that can report connectivity.
type healthReporter interface {
	Healthy() bool
}

// healthTickMsg drives the periodic connectivity check.
type healthTickMsg struct{}

// healthTick schedules the next connectivity check.
func healthTick() (cmd tea.Cmd) {
	cmd = tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return healthTickMsg{}
	})

	return cmd
}

// Model is the kctl root model.
type Model struct {
	logger    *zap.Logger
	keys      keys.Map
	styles    theme.Styles
	contexts  core.ContextManager
	pods      core.PodReader
	sel       core.Selector
	session   *Session
	switching bool
	status    statusbar.Model
	cmdBar    cmdbar.Model
	stack     stack.Stack
	width     int
	height    int
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

	// The status bar must describe the SELECTOR the list is actually using, not
	// the context's default namespace. Reading it from the context made an
	// all-namespaces list ("" selector) still claim it was scoped to "default".
	m.status.Namespace = sel.Namespace

	m.stack.Push(podlist.New(m.logger, styles, m.keys, pods, sel))

	return m
}

// WithSession attaches the session that rebuilds data sources on a context or
// namespace switch. Without one the model still runs against a single fixed
// source, which is what the view tests use.
func (m Model) WithSession(session *Session) (out Model) {
	m.session = session
	out = m

	return out
}

// Init satisfies tea.Model.
func (m Model) Init() (cmd tea.Cmd) {
	top := m.stack.Top()
	if top != nil {
		cmd = tea.Batch(top.Init(), healthTick())

		return cmd
	}

	cmd = healthTick()

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
		if m.session == nil {
			// No factory wired (tests, or a single-source build): just relabel.
			m.status.Context = msg.Context.Name
			m.status.Namespace = msg.Context.Namespace
			m.status.Message = ""

			return m, cmd
		}

		m.status.Connecting = true
		m.status.Message = ""
		m.switching = true
		m.stack.Pop()

		return m, m.switchTo(msg.Context.Name, msg.Context.Namespace)

	case healthTickMsg:
		reporter, ok := m.pods.(healthReporter)
		switch {
		case ok && !reporter.Healthy():
			m.status.Connecting = true

		case !m.switching:
			// Only clear it when no switch is in flight, so the two do not
			// fight over the same indicator.
			m.status.Connecting = false
		}

		return m, healthTick()

	case switchedMsg:
		m.status.Connecting = false
		m.switching = false

		if msg.Err != nil {
			// Stay put. Leaving the user on a dead context with an empty list
			// is worse than staying where they were with an error.
			m.status.Message = msg.Err.Error()

			return m, cmd
		}

		m.pods = msg.Active.Pods
		m.sel = core.Selector{Namespace: msg.Active.Namespace}
		m.status.Context = msg.Active.Context
		m.status.Namespace = msg.Active.Namespace
		m.status.Message = ""
		m.stack.Replace(podlist.New(m.logger, m.styles, m.keys, m.pods, m.sel))

		return m, m.stack.Top().Init()

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

// switchedMsg reports the outcome of a context or namespace switch.
type switchedMsg struct {
	Active Active
	Err    error
}

// switchTo rebuilds the data sources for a context and namespace, off the
// update loop so the UI stays responsive while the cache syncs.
func (m Model) switchTo(contextName, namespace string) (cmd tea.Cmd) {
	session := m.session
	if session == nil {
		return cmd
	}

	cmd = func() tea.Msg {
		active, err := session.Switch(context.Background(), contextName, namespace)

		return switchedMsg{Active: active, Err: err}
	}

	return cmd
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
