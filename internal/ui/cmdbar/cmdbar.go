// Package cmdbar implements kctl's ":" command line.
package cmdbar

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/mo-pankaj/kctl/internal/theme"
)

// Recognised verbs.
const (
	VerbNamespace = "ns"
	VerbContext   = "ctx"
	VerbSort      = "sort"
	VerbFilter    = "filter"
	VerbQuit      = "q"
)

// verbs describes every command, and is the single source for both validation
// and the hint shown to the user. A command that exists but is undocumented
// here would be undiscoverable, so they are deliberately the same list.
var verbs = []struct {
	Name    string
	Aliases []string
	Arg     string
	Help    string
}{
	{Name: VerbNamespace, Aliases: []string{"namespace"}, Arg: "<name>|all", Help: "scope the list to a namespace"},
	{Name: VerbContext, Aliases: []string{"context"}, Arg: "", Help: "open the context picker"},
	{Name: VerbSort, Aliases: nil, Arg: "name|status|restarts|age", Help: "sort by a column"},
	{Name: VerbFilter, Aliases: nil, Arg: "<expr>", Help: "filter, e.g. status:crash"},
	{Name: VerbQuit, Aliases: []string{"quit"}, Arg: "", Help: "quit"},
}

// Verbs renders the command list for the hint line.
func Verbs() (lines []string) {
	for _, v := range verbs {
		name := v.Name
		if v.Arg != "" {
			name += " " + v.Arg
		}

		lines = append(lines, name)
	}

	return lines
}

// canonical resolves a typed verb, following aliases. ok is false when the verb
// is not recognised.
func canonical(typed string) (name string, ok bool) {
	for _, v := range verbs {
		if typed == v.Name {
			return v.Name, true
		}

		for _, a := range v.Aliases {
			if typed == a {
				return v.Name, true
			}
		}
	}

	return name, ok
}

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

// SetWidth sizes the input, so long commands scroll within the line instead of
// wrapping onto the line above.
func (m *Model) SetWidth(w int) {
	if w > 8 {
		m.input.SetWidth(w - 6)
	}
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

		// An empty bar is the moment the user does not know what to type.
		if m.input.Value() == "" {
			s += "\n" + m.styles.Dimmed.Render("  "+strings.Join(Verbs(), "   "))
		}
	}

	if m.err != nil {
		// Its own line, not appended: the input is width-padded, so anything
		// after it lands past the right edge of the terminal and is invisible.
		if s != "" {
			s += "\n"
		}

		s += m.styles.StatusError.Render("  " + m.err.Error())
	}

	return s
}

// Parse turns a command line into a Command.
func Parse(input string) (c Command, err error) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), ":"))
	if trimmed == "" {
		err = fmt.Errorf("type a command: %s", strings.Join(Verbs(), ", "))

		return c, err
	}

	fields := strings.Fields(trimmed)
	typed := strings.ToLower(fields[0])

	// A user reaching for the command bar to type a filter is reaching for the
	// wrong key, not making a mistake. Say so rather than calling it unknown.
	if strings.Contains(typed, ":") {
		err = fmt.Errorf("%q looks like a filter — press / to filter, : is for commands", typed)

		return c, err
	}

	name, ok := canonical(typed)
	if !ok {
		// User-facing, so no error- prefix: this is read on screen, not grepped
		// in a log.
		err = fmt.Errorf("no command %q. try: %s", typed, strings.Join(Verbs(), ", "))

		return c, err
	}

	c.Verb = name
	if len(fields) > 1 {
		c.Arg = strings.Join(fields[1:], " ")
	}

	return c, err
}
