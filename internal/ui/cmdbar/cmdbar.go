// Package cmdbar implements kctl's ":" command line.
package cmdbar

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/mo-pankaj/kctl/internal/theme"
)

// Verbs recognised in v0.1.
const (
	VerbNamespace = "ns"
	VerbContext   = "ctx"
)

// Command is a parsed command line.
type Command struct {
	Verb string
	Arg  string
}

// SubmitMsg reports a successfully parsed command.
type SubmitMsg struct {
	Command Command
}

// Model is the command bar.
type Model struct {
	input  textinput.Model
	styles theme.Styles
	err    error
	open   bool
}

// New builds the bar.
func New(styles theme.Styles) (m Model) {
	input := textinput.New()
	input.Prompt = ":"
	input.CharLimit = 128

	m = Model{input: input, styles: styles}
	return m
}

// Open focuses the bar for input.
func (m *Model) Open() {
	m.open = true
	m.err = nil
	m.input.SetValue("")
	m.input.Focus()
}

// Close dismisses the bar.
func (m *Model) Close() {
	m.open = false
	m.input.Blur()
	m.input.SetValue("")
}

// Focused reports whether the bar is taking keystrokes.
func (m Model) Focused() (focused bool) {
	focused = m.open
	return focused
}

// Update handles key input while the bar is open.
func (m Model) Update(msg tea.Msg) (model Model, cmd tea.Cmd) {
	pressed, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, cmd
	}

	switch pressed.Code {
	case tea.KeyEscape:
		m.Close()
		return m, cmd

	case tea.KeyEnter:
		parsed, err := Parse(m.input.Value())
		if err != nil {
			m.err = err
			return m, cmd
		}

		m.Close()
		cmd = func() tea.Msg { return SubmitMsg{Command: parsed} }

		return m, cmd
	}

	m.err = nil
	m.input, cmd = m.input.Update(msg)

	return m, cmd
}

// View renders the bar, or an empty string when closed and error-free.
//
// A parse error leaves the bar OPEN so the user can correct what they typed, so
// the input and the error must render together. Rendering only one of them meant
// a bad command produced no visible feedback at all.
func (m Model) View() (s string) {
	if m.open {
		s = "  " + m.input.View()
	}

	if m.err != nil {
		if s != "" {
			s += "  "
		}

		s += m.styles.StatusError.Render(m.err.Error())
	}

	return s
}

// Parse turns a command line into a Command.
func Parse(input string) (c Command, err error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), ":"))
	if trimmed == "" {
		err = fmt.Errorf("error-empty-command")
		return c, err
	}

	fields := strings.Fields(trimmed)

	c.Verb = strings.ToLower(fields[0])
	if len(fields) > 1 {
		c.Arg = strings.Join(fields[1:], " ")
	}

	switch c.Verb {
	case VerbNamespace, VerbContext:
		return c, err
	}

	err = fmt.Errorf("error-unknown-command :%s", c.Verb)
	c = Command{}

	return c, err
}
