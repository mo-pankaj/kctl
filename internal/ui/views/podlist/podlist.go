// Package podlist implements kctl's main pod table.
package podlist

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

// SelectedMsg reports that the user chose a pod.
type SelectedMsg struct {
	Pod core.Pod
}

// ErrorMsg reports a read failure.
type ErrorMsg struct {
	Err error
}

// dirtyMsg means the cache changed and the view must re-read.
type dirtyMsg struct{}

// sourceClosedMsg means the subscription ended.
type sourceClosedMsg struct{}

// loadedMsg carries a completed cache read.
type loadedMsg struct {
	Pods []core.Pod
}

// Model is the pod list view.
type Model struct {
	logger *zap.Logger
	styles theme.Styles
	keys   keys.Map
	reader core.PodReader
	sel    core.Selector

	table     table.Model
	pods      []core.Pod
	visible   []core.Pod
	filter    textinput.Model
	filtering bool
	sortKey   SortKey
	dirty     <-chan struct{}
	err       error
}

// New builds the view.
func New(logger *zap.Logger, styles theme.Styles, k keys.Map, reader core.PodReader, sel core.Selector) (m Model) {
	m = Model{
		logger: logger.With(zap.String("view", "podlist")),
		styles: styles,
		keys:   k,
		reader: reader,
		sel:    sel,
		table:  buildTable(nil, sel.AllNamespaces()),
	}

	input := textinput.New()
	input.Prompt = "/"
	input.CharLimit = 64
	m.filter = input
	return m
}

// Init subscribes to cache changes.
func (m Model) Init() (cmd tea.Cmd) {
	cmd = m.subscribe()
	return cmd
}

// subscribe opens the notification channel and arms the first wait.
func (m Model) subscribe() (cmd tea.Cmd) {
	logger := m.logger.With(zap.String("method", "subscribe"))

	cmd = func() tea.Msg {
		dirty, err := m.reader.Subscribe(context.Background(), m.sel)
		if err != nil {
			logger.Error("error-subscribing", zap.Error(err))
			return ErrorMsg{Err: err}
		}

		return subscribedMsg{Dirty: dirty}
	}

	return cmd
}

// subscribedMsg carries the notification channel back into Update.
type subscribedMsg struct {
	Dirty <-chan struct{}
}

// awaitPoke blocks in a command until the cache changes. Re-armed after every
// dirtyMsg, this is the bridge between the informer's goroutine and Bubble Tea's
// single-threaded Update.
func awaitPoke(dirty <-chan struct{}) (cmd tea.Cmd) {
	cmd = func() tea.Msg {
		_, ok := <-dirty
		if !ok {
			return sourceClosedMsg{}
		}

		return dirtyMsg{}
	}

	return cmd
}

// load reads the cache.
func (m Model) load() (cmd tea.Cmd) {
	logger := m.logger.With(zap.String("method", "load"))

	cmd = func() tea.Msg {
		pods, err := m.reader.Pods(context.Background(), m.sel)
		if err != nil {
			logger.Error("error-loading-pods", zap.Error(err))
			return ErrorMsg{Err: err}
		}

		return loadedMsg{Pods: pods}
	}

	return cmd
}

// Update satisfies tea.Model.
func (m Model) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case subscribedMsg:
		m.dirty = msg.Dirty
		return m, tea.Batch(awaitPoke(m.dirty), m.load())

	case dirtyMsg:
		return m, tea.Batch(awaitPoke(m.dirty), m.load())

	case sourceClosedMsg:
		m.dirty = nil
		return m, cmd

	case loadedMsg:
		m.pods = msg.Pods
		m.err = nil
		(&m).refresh()

		return m, cmd

	case ErrorMsg:
		m.err = msg.Err
		return m, cmd

	case tea.WindowSizeMsg:
		// Leave room for the status bar, header, and help line.
		m.table.SetHeight(maxInt(3, msg.Height-6))
		return m, cmd

	case tea.KeyPressMsg:
		if m.filtering {
			switch msg.Code {
			case tea.KeyEscape:
				m.filtering = false
				m.filter.SetValue("")
				m.filter.Blur()
				(&m).refresh()

				return m, cmd

			case tea.KeyEnter:
				m.filtering = false
				m.filter.Blur()

				return m, cmd
			}

			m.filter, cmd = m.filter.Update(msg)
			(&m).refresh()

			return m, cmd
		}

		// Guarded on m.err: without it, r would fire a redundant read on every
		// press during normal browsing.
		if key.Matches(msg, m.keys.Retry) && m.err != nil {
			m.err = nil

			return m, m.load()
		}

		if key.Matches(msg, m.keys.Filter) {
			m.filtering = true
			m.filter.Focus()

			return m, cmd
		}

		if key.Matches(msg, m.keys.SortCycle) {
			m.sortKey = m.sortKey.Next()
			(&m).refresh()

			return m, cmd
		}

		if key.Matches(msg, m.keys.Select) {
			return m, m.emitSelected()
		}
	}

	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// emitSelected reports the highlighted pod.
func (m Model) emitSelected() (cmd tea.Cmd) {
	index := m.table.Cursor()
	if index < 0 || index >= len(m.visible) {
		return cmd
	}

	pod := m.visible[index]
	cmd = func() tea.Msg {
		return SelectedMsg{Pod: pod}
	}

	return cmd
}

// View satisfies tea.Model.
func (m Model) View() (v tea.View) {
	var s string
	if m.err != nil {
		s = "\n" + m.styles.StatusError.Render("  "+m.err.Error()) + "\n\n" +
			m.styles.Help.Render("  "+keys.HelpLine(m.keys.Retry, m.keys.Back)) + "\n"
		v = tea.NewView(s)
		return v
	}

	header := ""
	if m.filtering || m.filter.Value() != "" {
		header = "  " + m.filter.View() + "\n"
	}

	s = "\n" + header + m.table.View() + "\n" +
		m.styles.Help.Render(fmt.Sprintf("  %d/%d pods  ·  sort:%s  ·  %s",
			len(m.visible), len(m.pods), m.sortKey,
			keys.HelpLine(m.keys.Filter, m.keys.SortCycle, m.keys.Select, m.keys.Back))) + "\n"

	v = tea.NewView(s)
	return v
}

// refresh recomputes the visible rows from the last cache read.
func (m *Model) refresh() {
	m.visible = Sort(Filter(m.pods, m.filter.Value()), m.sortKey)
	m.table.SetRows(rowsFor(m.visible, m.sel.AllNamespaces(), time.Now()))
}

// InputFocused reports whether the filter input is taking keystrokes.
func (m Model) InputFocused() (focused bool) {
	focused = m.filtering
	return focused
}

func buildTable(pods []core.Pod, allNamespaces bool) (t table.Model) {
	columns := []table.Column{}
	if allNamespaces {
		columns = append(columns, table.Column{Title: "NAMESPACE", Width: 22})
	}

	columns = append(columns,
		table.Column{Title: "NAME", Width: 42},
		table.Column{Title: "READY", Width: 7},
		table.Column{Title: "STATUS", Width: 20},
		table.Column{Title: "RESTARTS", Width: 9},
		table.Column{Title: "AGE", Width: 6},
	)

	t = table.New(
		table.WithColumns(columns),
		table.WithRows(rowsFor(pods, allNamespaces, time.Now())),
		table.WithFocused(true),
		table.WithHeight(20),
		table.WithWidth(120),
	)

	return t
}

func rowsFor(pods []core.Pod, allNamespaces bool, now time.Time) (rows []table.Row) {
	rows = make([]table.Row, 0, len(pods))

	for _, p := range pods {
		row := table.Row{}
		if allNamespaces {
			row = append(row, p.Namespace)
		}

		row = append(row,
			p.Name,
			p.Ready(),
			p.Status,
			fmt.Sprintf("%d", p.Restarts),
			FormatAge(p.Age(now)),
		)

		rows = append(rows, row)
	}

	return rows
}

// FormatAge renders a duration the way kubectl does: one unit, no decimals.
func FormatAge(d time.Duration) (s string) {
	switch {
	case d >= 24*time.Hour:
		s = fmt.Sprintf("%dd", int(d.Hours())/24)

	case d >= time.Hour:
		s = fmt.Sprintf("%dh", int(d.Hours()))

	case d >= time.Minute:
		s = fmt.Sprintf("%dm", int(d.Minutes()))

	default:
		s = fmt.Sprintf("%ds", int(d.Seconds()))
	}

	return s
}

func maxInt(a, b int) (n int) {
	n = a
	if b > a {
		n = b
	}

	return n
}
