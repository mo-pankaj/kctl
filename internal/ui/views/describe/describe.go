// Package describe implements kctl's pod detail view.
package describe

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

// ErrorMsg reports a failed load.
type ErrorMsg struct {
	Err error
}

// loadedMsg carries a completed describe read.
type loadedMsg struct {
	Detail *core.PodDetail
	Events []core.Event
}

// Model is the pod describe view.
type Model struct {
	logger    *zap.Logger
	styles    theme.Styles
	keys      keys.Map
	describer core.PodDescriber
	namespace string
	name      string

	viewport viewport.Model
	detail   *core.PodDetail
	events   []core.Event
	err      error
}

// New builds the view.
func New(logger *zap.Logger, styles theme.Styles, k keys.Map, describer core.PodDescriber, ns, name string) (m Model) {
	m = Model{
		logger:    logger.With(zap.String("view", "describe"), zap.String("pod", name)),
		styles:    styles,
		keys:      k,
		describer: describer,
		namespace: ns,
		name:      name,
		viewport:  viewport.New(),
	}

	return m
}

// Init loads the detail and its events.
func (m Model) Init() (cmd tea.Cmd) {
	cmd = m.load()

	return cmd
}

// load fetches the detail and events together. Events are the reason a pod will
// not start, so a describe without them is half a view.
func (m Model) load() (cmd tea.Cmd) {
	logger := m.logger.With(zap.String("method", "load"))

	cmd = func() tea.Msg {
		ctx := context.Background()

		detail, err := m.describer.Describe(ctx, m.namespace, m.name)
		if err != nil {
			logger.Error("error-describing-pod", zap.Error(err))

			return ErrorMsg{Err: err}
		}

		// An events failure must not hide the detail we already have: RBAC
		// often allows reading pods while forbidding events.
		events, err := m.describer.Events(ctx, m.namespace, m.name)
		if err != nil {
			logger.Warn("error-listing-events", zap.Error(err))
		}

		return loadedMsg{Detail: detail, Events: events}
	}

	return cmd
}

// Update satisfies tea.Model.
func (m Model) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		m.detail = msg.Detail
		m.events = msg.Events
		m.err = nil
		m.viewport.SetContent(m.body())

		return m, cmd

	case ErrorMsg:
		m.err = msg.Err

		return m, cmd

	case tea.WindowSizeMsg:
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(maxInt(3, msg.Height-6))

		if m.detail != nil {
			m.viewport.SetContent(m.body())
		}

		return m, cmd

	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Retry) && m.err != nil {
			m.err = nil

			return m, m.load()
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)

	return m, cmd
}

// View satisfies tea.Model.
func (m Model) View() (v tea.View) {
	if m.err != nil {
		body := "\n" + m.styles.StatusError.Render("  "+m.err.Error()) + "\n\n" +
			m.styles.Help.Render("  "+keys.HelpLine(m.keys.Retry, m.keys.Back)) + "\n"

		v = tea.NewView(body)

		return v
	}

	if m.detail == nil {
		v = tea.NewView("\n  loading…\n")

		return v
	}

	body := "\n" + m.styles.TableHeader.Render(fmt.Sprintf("  %s/%s", m.detail.Namespace, m.detail.Name)) + "\n" +
		m.viewport.View() + "\n" +
		m.styles.Help.Render("  "+keys.HelpLine(m.keys.Back)+"  ↑/↓:scroll") + "\n"

	v = tea.NewView(body)

	return v
}

// body renders the scrollable detail.
func (m Model) body() (s string) {
	d := m.detail
	now := time.Now()

	var b strings.Builder

	row := func(k, val string) {
		if val == "" {
			return
		}

		b.WriteString("  " + m.styles.StatusKey.Render(fmt.Sprintf("%-14s", k)) + val + "\n")
	}

	row("Status", d.Status)
	row("Node", d.Node)
	row("Pod IP", d.PodIP)
	row("QoS", d.QOSClass)
	row("Service acct", d.ServiceAccount)

	if d.OwnerKind != "" {
		row("Controlled by", d.OwnerKind+"/"+d.OwnerName)
	}

	row("Age", formatAge(d.Age(now)))

	writeContainers := func(title string, list []core.ContainerState) {
		if len(list) == 0 {
			return
		}

		b.WriteString("\n" + m.styles.TableHeader.Render("  "+title) + "\n")

		for _, c := range list {
			state := c.State
			if c.Reason != "" {
				state += " (" + c.Reason + ")"
			}

			if c.ExitCode != nil {
				state += fmt.Sprintf(" exit=%d", *c.ExitCode)
			}

			style := m.styles.StatusValue
			if !c.Ready {
				style = m.styles.StatusError
			}

			fmt.Fprintf(&b, "  %-20s %s\n", c.Name, style.Render(state))
			fmt.Fprintf(&b, "  %-20s %s\n", "", m.styles.Dimmed.Render(c.Image))

			if c.RestartCount > 0 {
				fmt.Fprintf(&b, "  %-20s restarts: %d\n", "", c.RestartCount)
			}

			if c.Message != "" {
				fmt.Fprintf(&b, "  %-20s %s\n", "", m.styles.Dimmed.Render(truncate(c.Message, 100)))
			}
		}
	}

	writeContainers("INIT CONTAINERS", d.InitContainers)
	writeContainers("CONTAINERS", d.Containers)

	if len(d.Conditions) > 0 {
		b.WriteString("\n" + m.styles.TableHeader.Render("  CONDITIONS") + "\n")

		for _, c := range d.Conditions {
			line := fmt.Sprintf("  %-28s %-6s %s", c.Type, c.Status, c.Reason)
			if c.Status != "True" {
				line = m.styles.StatusWarn.Render(line)
			}

			b.WriteString(line + "\n")
		}
	}

	b.WriteString("\n" + m.styles.TableHeader.Render(fmt.Sprintf("  EVENTS (%d)", len(m.events))) + "\n")

	if len(m.events) == 0 {
		b.WriteString(m.styles.Dimmed.Render("  none\n"))

		return b.String()
	}

	for _, e := range m.events {
		stamp := formatAge(now.Sub(e.Last))
		head := fmt.Sprintf("  %-6s %-22s %-5s x%-3d", stamp, e.Reason, e.Type, e.Count)

		if e.Warning() {
			head = m.styles.StatusError.Render(head)
		}

		b.WriteString(head + truncate(e.Message, 90) + "\n")
	}

	s = b.String()

	return s
}

func truncate(s string, n int) (out string) {
	out = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(out) > n {
		out = out[:n-1] + "…"
	}

	return out
}

func formatAge(d time.Duration) (s string) {
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
