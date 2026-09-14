// Package apply implements kctl's guarded apply flow: pick a file, see the
// server-side dry-run diff, confirm against the target cluster, then write.
package apply

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

// Target describes the cluster an apply would write to.
type Target struct {
	Context   string
	Server    string
	Namespace string
	Protected bool
}

// Runner performs the dry run and the apply.
type Runner interface {
	DryRun(ctx context.Context, docs []core.Manifest) ([]core.DiffResult, error)
	Apply(ctx context.Context, docs []core.Manifest, force bool) ([]core.ApplyResult, error)
}

// Parser turns a file's contents into manifests.
type Parser func(path, defaultNamespace string) ([]core.Manifest, error)

type stage int

const (
	stagePath stage = iota
	stageDiff
	stageConfirm
	stageDone
)

type diffedMsg struct {
	docs    []core.Manifest
	results []core.DiffResult
}

type appliedMsg struct {
	results []core.ApplyResult
}

// CancelMsg asks the root to dismiss this view.
//
// The view cannot pop itself, and its text inputs consume esc before the root
// ever sees it — so esc has to be turned into a message rather than left to the
// global back binding.
type CancelMsg struct{}

// ErrorMsg reports a failure.
type ErrorMsg struct{ Err error }

// Model is the apply view.
type Model struct {
	logger *zap.Logger
	styles theme.Styles
	keys   keys.Map
	runner Runner
	parse  Parser
	target Target

	stage    stage
	path     textinput.Model
	confirm  textinput.Model
	viewport viewport.Model
	docs     []core.Manifest
	diffs    []core.DiffResult
	applied  []core.ApplyResult
	force    bool
	err      error
	width    int
}

// New builds the apply view.
func New(logger *zap.Logger, styles theme.Styles, k keys.Map, runner Runner, parse Parser, target Target) (m Model) {
	path := textinput.New()
	path.Prompt = "path: "
	path.CharLimit = 512
	path.SetValue("")
	path.Focus()

	confirm := textinput.New()
	confirm.Prompt = "type the context name: "
	confirm.CharLimit = 128

	m = Model{
		logger:   logger.With(zap.String("view", "apply")),
		styles:   styles,
		keys:     k,
		runner:   runner,
		parse:    parse,
		target:   target,
		stage:    stagePath,
		path:     path,
		confirm:  confirm,
		viewport: viewport.New(),
	}

	return m
}

// InputFocused keeps the root's global letter bindings out of the way while the
// user is typing a path or a context name.
func (m Model) InputFocused() (focused bool) {
	focused = m.stage == stagePath || (m.stage == stageConfirm && m.target.Protected)

	return focused
}

// Init satisfies tea.Model.
func (m Model) Init() (cmd tea.Cmd) { return cmd }

// dryRun parses the file and asks the server what would change.
func (m Model) dryRun(path string) (cmd tea.Cmd) {
	logger := m.logger.With(zap.String("method", "dryRun"), zap.String("path", path))

	cmd = func() tea.Msg {
		expanded := expandHome(path)

		info, err := os.Stat(expanded)
		if err != nil {
			logger.Error("error-reading-manifest", zap.Error(err))

			return ErrorMsg{Err: fmt.Errorf("error-reading-manifest :%w", err)}
		}

		if info.IsDir() {
			return ErrorMsg{Err: fmt.Errorf("error-path-is-a-directory :%s", expanded)}
		}

		docs, err := m.parse(expanded, m.target.Namespace)
		if err != nil {
			logger.Error("error-parsing-manifest", zap.Error(err))

			return ErrorMsg{Err: err}
		}

		if len(docs) == 0 {
			return ErrorMsg{Err: fmt.Errorf("error-no-documents :%s", expanded)}
		}

		results, err := m.runner.DryRun(context.Background(), docs)
		if err != nil {
			logger.Error("error-dry-run", zap.Error(err))

			return ErrorMsg{Err: err}
		}

		return diffedMsg{docs: docs, results: results}
	}

	return cmd
}

// apply writes for real.
func (m Model) apply() (cmd tea.Cmd) {
	logger := m.logger.With(zap.String("method", "apply"), zap.Bool("force", m.force))
	docs := m.docs
	force := m.force

	cmd = func() tea.Msg {
		results, err := m.runner.Apply(context.Background(), docs, force)
		if err != nil {
			logger.Error("error-applying", zap.Error(err))

			return ErrorMsg{Err: err}
		}

		return appliedMsg{results: results}
	}

	return cmd
}

// Update satisfies tea.Model.
func (m Model) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case diffedMsg:
		m.docs = msg.docs
		m.diffs = msg.results
		m.stage = stageDiff
		m.err = nil
		m.viewport.SetContent(m.diffBody())

		return m, cmd

	case appliedMsg:
		m.applied = msg.results
		m.stage = stageDone

		return m, cmd

	case ErrorMsg:
		m.err = msg.Err
		m.stage = stagePath
		m.path.Focus()

		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(maxInt(3, msg.Height-12))

		return m, cmd

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (model tea.Model, cmd tea.Cmd) {
	// esc always leaves, at every stage. The path and confirm inputs would
	// otherwise swallow it and strand the user in this view.
	if msg.Code == tea.KeyEscape {
		cmd = func() tea.Msg { return CancelMsg{} }

		return m, cmd
	}

	switch m.stage {
	case stagePath:
		if msg.Code == tea.KeyEnter {
			value := strings.TrimSpace(m.path.Value())
			if value == "" {
				return m, cmd
			}

			return m, m.dryRun(value)
		}

		m.path, cmd = m.path.Update(msg)

		return m, cmd

	case stageDiff:
		switch msg.String() {
		case "y":
			if !m.applicable() {
				return m, cmd
			}

			m.stage = stageConfirm
			if m.target.Protected {
				m.confirm.SetValue("")
				m.confirm.Focus()
			}

			return m, cmd

		case "F":
			// Uppercase on purpose: forcing overrides another controller's
			// ownership, so it should not share a key with anything casual.
			m.force = !m.force
			m.viewport.SetContent(m.diffBody())

			return m, cmd
		}

		m.viewport, cmd = m.viewport.Update(msg)

		return m, cmd

	case stageConfirm:
		if !m.target.Protected {
			if msg.String() == "y" {
				return m, m.apply()
			}

			m.stage = stageDiff

			return m, cmd
		}

		if msg.Code == tea.KeyEnter {
			if m.confirm.Value() == m.target.Context {
				return m, m.apply()
			}

			m.err = fmt.Errorf("error-context-name-mismatch :typed %q", m.confirm.Value())
			m.stage = stageDiff

			return m, cmd
		}

		m.confirm, cmd = m.confirm.Update(msg)

		return m, cmd
	}

	return m, cmd
}

// applicable reports whether anything can be applied: a conflict without force
// armed, or a hard error, blocks the write.
func (m Model) applicable() (ok bool) {
	for _, d := range m.diffs {
		switch {
		case d.Err != nil:
			return ok

		case d.Conflict && !m.force:
			return ok
		}
	}

	ok = true

	return ok
}

// View satisfies tea.Model.
func (m Model) View() (v tea.View) {
	var b strings.Builder

	b.WriteString("\n" + m.banner() + "\n")

	if m.err != nil {
		b.WriteString(m.styles.StatusError.Render("  "+m.err.Error()) + "\n")
	}

	switch m.stage {
	case stagePath:
		b.WriteString("\n  " + m.path.View() + "\n\n")
		b.WriteString(m.styles.Help.Render("  enter:dry-run  "+keys.HelpLine(m.keys.Back)) + "\n")

	case stageDiff:
		b.WriteString(m.viewport.View() + "\n")
		b.WriteString(m.summary() + "\n")
		b.WriteString(m.styles.Help.Render("  y:continue  F:toggle force  ↑/↓:scroll  "+keys.HelpLine(m.keys.Back)) + "\n")

	case stageConfirm:
		b.WriteString("\n" + m.confirmPrompt() + "\n")

	case stageDone:
		b.WriteString("\n" + m.results() + "\n")
		b.WriteString(m.styles.Help.Render("  "+keys.HelpLine(m.keys.Back)) + "\n")
	}

	v = tea.NewView(b.String())

	return v
}

// banner names the cluster being written to. It is always visible, because the
// most expensive mistake this view can permit is applying to the wrong one.
func (m Model) banner() (s string) {
	label := m.styles.StatusValue.Render(m.target.Context)
	if m.target.Protected {
		label = m.styles.StatusError.Render(m.target.Context + "  ⚠ PROTECTED")
	}

	s = "  " + m.styles.StatusKey.Render("target ") + label + "\n" +
		"  " + m.styles.StatusKey.Render("server ") + m.styles.Dimmed.Render(m.target.Server) + "\n" +
		"  " + m.styles.StatusKey.Render("ns     ") + m.styles.StatusValue.Render(namespaceLabel(m.target.Namespace))

	return s
}

func (m Model) summary() (s string) {
	var parts []string

	for _, d := range m.diffs {
		switch {
		case d.Err != nil:
			parts = append(parts, m.styles.StatusError.Render(d.Manifest.Describe()+" error"))

		case d.Conflict:
			owners := strings.Join(d.Managers, ",")
			parts = append(parts, m.styles.StatusError.Render(fmt.Sprintf("%s conflict(%s)", d.Manifest.Describe(), owners)))

		case d.Creation:
			parts = append(parts, fmt.Sprintf("%s create", d.Manifest.Describe()))

		case !d.Changed():
			parts = append(parts, m.styles.Dimmed.Render(d.Manifest.Describe()+" unchanged"))

		default:
			parts = append(parts, fmt.Sprintf("%s +%d/-%d", d.Manifest.Describe(), d.Added, d.Removed))
		}
	}

	force := ""
	if m.force {
		force = m.styles.StatusWarn.Render("   FORCE ARMED")
	}

	s = "  " + strings.Join(parts, "  ·  ") + force

	return s
}

func (m Model) confirmPrompt() (s string) {
	if !m.target.Protected {
		s = "  " + m.styles.StatusWarn.Render(fmt.Sprintf("Apply %d document(s) to %s? ", len(m.docs), m.target.Context)) +
			"\n\n" + m.styles.Help.Render("  y:apply  any other key:cancel")

		return s
	}

	s = "  " + m.styles.StatusError.Render("This cluster is protected.") + "\n" +
		"  Type " + m.styles.StatusValue.Render(m.target.Context) + " exactly to apply.\n\n" +
		"  " + m.confirm.View() + "\n\n" +
		m.styles.Help.Render("  enter:confirm  esc:cancel")

	return s
}

func (m Model) results() (s string) {
	var b strings.Builder

	for _, r := range m.applied {
		switch {
		case r.Applied:
			b.WriteString("  " + m.styles.StatusValue.Render("applied  ") + r.Manifest.Describe() + "\n")

		case r.Conflict:
			b.WriteString("  " + m.styles.StatusError.Render("conflict ") + r.Manifest.Describe() +
				"  owned by " + strings.Join(r.Managers, ",") + "\n")

		default:
			b.WriteString("  " + m.styles.StatusError.Render("failed   ") + r.Manifest.Describe() + ": " + errText(r.Err) + "\n")
		}
	}

	s = b.String()

	return s
}

func (m Model) diffBody() (s string) {
	var b strings.Builder

	for _, d := range m.diffs {
		b.WriteString(m.styles.TableHeader.Render("  "+d.Manifest.Describe()+"  ("+d.Manifest.Namespace+")") + "\n")

		switch {
		case d.Err != nil:
			b.WriteString(m.styles.StatusError.Render("    "+d.Err.Error()) + "\n\n")

			continue

		case d.Conflict:
			b.WriteString(m.styles.StatusError.Render(fmt.Sprintf("    field ownership conflict with %s over %s",
				strings.Join(d.Managers, ", "), strings.Join(d.Fields, ", "))) + "\n")
			b.WriteString(m.styles.Dimmed.Render("    press F to arm force, which overrides that owner") + "\n\n")

			continue

		case !d.Changed():
			b.WriteString(m.styles.Dimmed.Render("    no change") + "\n\n")

			continue
		}

		for _, line := range strings.Split(d.Unified, "\n") {
			switch {
			case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
				continue

			case strings.HasPrefix(line, "+"):
				b.WriteString("    " + m.styles.StatusValue.Render(line) + "\n")

			case strings.HasPrefix(line, "-"):
				b.WriteString("    " + m.styles.StatusError.Render(line) + "\n")

			default:
				b.WriteString("    " + m.styles.Dimmed.Render(line) + "\n")
			}
		}

		b.WriteString("\n")
	}

	s = b.String()

	return s
}

func namespaceLabel(ns string) (s string) {
	s = ns
	if s == "" {
		s = "default"
	}

	return s
}

func errText(err error) (s string) {
	if err == nil {
		s = "unknown"

		return s
	}

	s = err.Error()

	return s
}

func expandHome(path string) (out string) {
	out = path
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			out = home + path[1:]
		}
	}

	return out
}

func maxInt(a, b int) (n int) {
	n = a
	if b > a {
		n = b
	}

	return n
}
