// Package nspicker implements the namespace selection view.
package nspicker

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

// allNamespaces is the synthetic first row, so widening the scope is a visible
// choice rather than something you have to know to type.
const allNamespaces = "· all namespaces ·"

// SelectedMsg reports the chosen namespace. An empty Namespace means all.
type SelectedMsg struct {
	Namespace string
}

// ErrorMsg reports a failed load.
type ErrorMsg struct {
	Err error
}

type loadedMsg struct {
	names []string
}

// Model is the namespace picker.
type Model struct {
	logger  *zap.Logger
	styles  theme.Styles
	keys    keys.Map
	lister  core.NamespaceLister
	current string

	table  table.Model
	names  []string
	err    error
	loaded bool
}

// New builds the picker.
func New(logger *zap.Logger, styles theme.Styles, k keys.Map, lister core.NamespaceLister, current string) (m Model) {
	m = Model{
		logger:  logger.With(zap.String("view", "nspicker")),
		styles:  styles,
		keys:    k,
		lister:  lister,
		current: current,
		table:   buildTable(nil, current),
	}

	return m
}

// Init loads the namespace list.
func (m Model) Init() (cmd tea.Cmd) {
	logger := m.logger.With(zap.String("method", "Init"))

	cmd = func() tea.Msg {
		names, err := m.lister.Namespaces(context.Background())
		if err != nil {
			logger.Error("error-listing-namespaces", zap.Error(err))

			return ErrorMsg{Err: err}
		}

		return loadedMsg{names: names}
	}

	return cmd
}

// Update satisfies tea.Model.
func (m Model) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		m.names = msg.names
		m.loaded = true
		m.err = nil
		m.table = buildTable(msg.names, m.current)

		return m, cmd

	case ErrorMsg:
		m.err = msg.Err

		return m, cmd

	case tea.WindowSizeMsg:
		m.table.SetWidth(msg.Width)
		m.table.SetHeight(maxInt(3, msg.Height-6))

		return m, cmd

	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Retry) && m.err != nil {
			m.err = nil

			return m, m.Init()
		}

		if key.Matches(msg, m.keys.Select) {
			return m, m.choose()
		}
	}

	m.table, cmd = m.table.Update(msg)

	return m, cmd
}

// choose reports the highlighted namespace.
func (m Model) choose() (cmd tea.Cmd) {
	index := m.table.Cursor()
	if index < 0 || index > len(m.names) {
		return cmd
	}

	// Row zero is the all-namespaces row, so the real names are offset by one.
	namespace := ""
	if index > 0 {
		namespace = m.names[index-1]
	}

	cmd = func() tea.Msg {
		return SelectedMsg{Namespace: namespace}
	}

	return cmd
}

// View satisfies tea.Model.
func (m Model) View() (v tea.View) {
	if m.err != nil {
		body := "\n" + m.styles.StatusError.Render("  "+m.err.Error()) + "\n\n" +
			m.styles.Dimmed.Render("  listing namespaces needs cluster-scoped rights;\n"+
				"  :ns <name> still works without them") + "\n\n" +
			m.styles.Help.Render("  "+keys.HelpLine(m.keys.Retry, m.keys.Back)) + "\n"

		v = tea.NewView(body)

		return v
	}

	if !m.loaded {
		v = tea.NewView("\n  loading namespaces…\n")

		return v
	}

	body := "\n" + m.table.View() + "\n" +
		m.styles.Help.Render(fmt.Sprintf("  %d namespaces  ·  %s",
			len(m.names), keys.HelpLine(m.keys.Select, m.keys.Back))) + "\n"

	v = tea.NewView(body)

	return v
}

func buildTable(names []string, current string) (t table.Model) {
	rows := []table.Row{{" ", allNamespaces}}

	for _, n := range names {
		marker := " "
		if n == current {
			marker = "*"
		}

		rows = append(rows, table.Row{marker, n})
	}

	t = table.New(
		table.WithColumns([]table.Column{{Title: "", Width: 2}, {Title: "NAMESPACE", Width: 40}}),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(20),
		table.WithWidth(80),
	)

	return t
}

func maxInt(a, b int) (n int) {
	n = a
	if b > a {
		n = b
	}

	return n
}
