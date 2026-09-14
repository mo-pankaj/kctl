package logview

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

const (
	defaultCapacity  = 10000
	streamBufferSize = 512
	maxDrainPerTick  = 200
)

// ErrorMsg reports a stream failure.
type ErrorMsg struct {
	Err error
}

// openedMsg carries the live stream back into Update.
type openedMsg struct {
	lines  <-chan string
	closer io.Closer
}

// linesMsg carries a batch of lines.
type linesMsg struct {
	lines []string
}

// streamEndedMsg means the log stream finished.
type streamEndedMsg struct{}

// Model is the log viewer.
type Model struct {
	logger *zap.Logger
	styles theme.Styles
	keys   keys.Map
	reader core.PodReader
	req    core.LogRequest

	viewport viewport.Model
	ring     *Ring
	lines    <-chan string
	closer   io.Closer
	follow   bool
	pending  int
	err      error
	ready    bool
}

// New builds the viewer.
func New(logger *zap.Logger, styles theme.Styles, k keys.Map, reader core.PodReader, req core.LogRequest) (m Model) {
	m = Model{
		logger:   logger.With(zap.String("view", "logview"), zap.String("pod", req.Pod)),
		styles:   styles,
		keys:     k,
		reader:   reader,
		req:      req,
		ring:     NewRing(defaultCapacity),
		viewport: viewport.New(),
		follow:   true,
	}
	return m
}

// Init opens the stream.
func (m Model) Init() (cmd tea.Cmd) {
	cmd = m.open()
	return cmd
}

// open starts the log stream and the goroutine that feeds lines into a channel.
//
// The view owns this goroutine and ends it by closing the stream on Close, so
// no separate lifecycle registry is needed for a single log view.
func (m Model) open() (cmd tea.Cmd) {
	logger := m.logger.With(zap.String("method", "open"))

	cmd = func() tea.Msg {
		rc, err := m.reader.Logs(context.Background(), m.req)
		if err != nil {
			logger.Error("error-opening-logs", zap.Error(err))
			return ErrorMsg{Err: err}
		}

		lines := make(chan string, streamBufferSize)

		go func() {
			defer close(lines)

			scanner := bufio.NewScanner(rc)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

			for scanner.Scan() {
				// A full channel blocks here, which is correct: backpressure on
				// a log tail just means reading the socket more slowly.
				lines <- scanner.Text()
			}
		}()

		return openedMsg{lines: lines, closer: rc}
	}

	return cmd
}

// awaitLines drains up to maxDrainPerTick lines per message. One message per
// line would turn a chatty pod into thousands of renders per second.
func awaitLines(lines <-chan string) (cmd tea.Cmd) {
	cmd = func() tea.Msg {
		first, ok := <-lines
		if !ok {
			return streamEndedMsg{}
		}

		batch := []string{first}

		for len(batch) < maxDrainPerTick {
			select {
			case next, ok := <-lines:
				if !ok {
					return linesMsg{lines: batch}
				}

				batch = append(batch, next)

			default:
				return linesMsg{lines: batch}
			}
		}

		return linesMsg{lines: batch}
	}

	return cmd
}

// Close ends the stream. The root model calls this when the view is popped.
func (m Model) Close() {
	if m.closer != nil {
		_ = m.closer.Close()
	}
}

// Update satisfies tea.Model.
func (m Model) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	switch msg := msg.(type) {
	case openedMsg:
		m.lines = msg.lines
		m.closer = msg.closer
		m.ready = true
		return m, awaitLines(m.lines)

	case linesMsg:
		for _, line := range msg.lines {
			m.ring.Add(line)
		}

		// While paused the screen must not move at all. Re-rendering would
		// slide the visible window as the ring grows and starts dropping its
		// oldest lines, which is exactly what "paused" promises not to do.
		if !m.follow {
			m.pending += len(msg.lines)

			return m, awaitLines(m.lines)
		}

		m.viewport.SetContent(strings.Join(m.ring.Lines(), "\n"))
		m.viewport.GotoBottom()

		return m, awaitLines(m.lines)

	case streamEndedMsg:
		m.lines = nil
		return m, cmd

	case ErrorMsg:
		m.err = msg.Err
		return m, cmd

	case tea.WindowSizeMsg:
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(maxInt(3, msg.Height-6))
		return m, cmd

	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.Follow) {
			m.follow = !m.follow

			if m.follow {
				// Catch up on everything buffered while paused.
				m.viewport.SetContent(strings.Join(m.ring.Lines(), "\n"))
				m.viewport.GotoBottom()
				m.pending = 0
			}

			return m, cmd
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View satisfies tea.Model.
func (m Model) View() (v tea.View) {
	var s string
	if m.err != nil {
		s = "\n" + m.styles.StatusError.Render("  "+m.err.Error()) + "\n\n" +
			m.styles.Help.Render("  "+keys.HelpLine(m.keys.Back)) + "\n"
		v = tea.NewView(s)
		return v
	}

	mode := m.styles.StatusValue.Render("follow")
	if !m.follow {
		mode = m.styles.StatusWarn.Render("paused")

		if m.pending > 0 {
			// Say how far behind the screen is, so a frozen view is obviously
			// frozen rather than looking like a dead stream.
			mode += m.styles.Dimmed.Render(fmt.Sprintf(" (+%d buffered)", m.pending))
		}
	}

	header := fmt.Sprintf("  logs %s/%s [%s]  ·  %s  ·  %d lines",
		m.req.Namespace, m.req.Pod, m.req.Container, mode, m.ring.Len())

	if m.ring.Truncated() {
		header += m.styles.StatusWarn.Render("  · truncated")
	}

	s = "\n" + m.styles.TableHeader.Render(header) + "\n" +
		m.viewport.View() + "\n" +
		m.styles.Help.Render("  "+keys.HelpLine(m.keys.Follow, m.keys.Back)+"  ↑/↓:scroll") + "\n"

	v = tea.NewView(s)
	return v
}

func maxInt(a, b int) (n int) {
	n = a
	if b > a {
		n = b
	}

	return n
}
