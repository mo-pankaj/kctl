// Package ctxpicker implements the context selection view.
package ctxpicker

import (
	"context"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

// SwitchedMsg reports a successful context switch.
type SwitchedMsg struct {
	Context core.ContextInfo
}

// ErrorMsg reports a failed context switch.
type ErrorMsg struct {
	Err error
}

// Model is the context picker.
type Model struct {
	logger   *zap.Logger
	styles   theme.Styles
	keys     keys.Map
	contexts core.ContextManager

	table table.Model
	rows  []core.ContextInfo
	err   error
}

// New builds the picker.
func New(logger *zap.Logger, styles theme.Styles, k keys.Map, contexts core.ContextManager) (m Model) {
	rows := contexts.Contexts()

	m = Model{
		logger:   logger.With(zap.String("view", "ctxpicker")),
		styles:   styles,
		keys:     k,
		contexts: contexts,
		rows:     rows,
		table:    buildTable(rows, contexts.Current().Name),
	}

	return m
}

func buildTable(infos []core.ContextInfo, current string) (t table.Model) {
	columns := []table.Column{
		{Title: "", Width: 2},
		{Title: "CONTEXT", Width: 24},
		{Title: "CLUSTER", Width: 14},
		{Title: "NAMESPACE", Width: 20},
		{Title: "SERVER", Width: 44},
	}

	rows := make([]table.Row, 0, len(infos))
	for _, c := range infos {
		marker := " "
		if c.Name == current {
			marker = "*"
		}

		rows = append(rows, table.Row{marker, c.Name, c.Cluster, c.Namespace, c.Server})
	}

	t = table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(20),
		table.WithWidth(120),
	)

	return t
}

// Init satisfies tea.Model.
func (m Model) Init() (cmd tea.Cmd) {
	return cmd
}

// Update satisfies tea.Model.
func (m Model) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case ErrorMsg:
		m.err = msg.Err
		return m, cmd

	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Select) {
			return m, m.selectCurrent()
		}
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// selectCurrent switches to the highlighted context.
func (m Model) selectCurrent() (cmd tea.Cmd) {
	index := m.table.Cursor()
	if index < 0 || index >= len(m.rows) {
		return cmd
	}

	target := m.rows[index]
	logger := m.logger.With(zap.String("method", "selectCurrent"), zap.String("context", target.Name))

	cmd = func() tea.Msg {
		err := m.contexts.Use(context.Background(), target.Name)
		if err != nil {
			logger.Error("error-switching-context", zap.Error(err))
			return ErrorMsg{Err: err}
		}

		logger.Info("context-switched")
		return SwitchedMsg{Context: target}
	}

	return cmd
}

// View satisfies tea.Model.
func (m Model) View() (v tea.View) {
	var s string
	s = "\n" + m.table.View() + "\n"

	if m.err != nil {
		s += m.styles.StatusError.Render("  "+m.err.Error()) + "\n"
	}

	s += m.styles.Help.Render("  "+keys.HelpLine(m.keys.Select, m.keys.Back)) + "\n"
	v = tea.NewView(s)
	return v
}
