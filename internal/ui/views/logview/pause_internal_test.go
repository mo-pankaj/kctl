package logview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

func newPaused(t *testing.T) Model {
	t.Helper()

	m := New(zap.NewNop(), theme.New(), keys.Default(), nil,
		core.LogRequest{Namespace: "ns1", Pod: "p", Container: "c", Follow: true})

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})

	return updated.(Model)
}

func feed(m Model, lines ...string) Model {
	updated, _ := m.Update(linesMsg{lines: lines})

	return updated.(Model)
}

func TestPauseFreezesTheVisibleContent(t *testing.T) {
	m := newPaused(t)
	m = feed(m, "alpha", "bravo")

	before := m.viewport.View()

	// Pause, then keep the stream running.
	updated, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)

	if m.follow {
		t.Fatal("f did not pause following")
	}

	m = feed(m, "charlie", "delta", "echo")

	if got := m.viewport.View(); got != before {
		t.Fatalf("paused view changed on screen.\nbefore:\n%s\nafter:\n%s", before, got)
	}

	// The lines are still being collected, just not shown.
	if m.ring.Len() != 5 {
		t.Fatalf("ring holds %d lines, want 5 — paused must keep buffering", m.ring.Len())
	}

	if m.pending != 3 {
		t.Fatalf("pending = %d, want 3", m.pending)
	}

	if !strings.Contains(m.View().Content, "+3 buffered") {
		t.Fatalf("header should say how far behind it is:\n%s", m.View().Content)
	}
}

func TestResumingCatchesUp(t *testing.T) {
	m := newPaused(t)
	m = feed(m, "alpha")

	updated, _ := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)
	m = feed(m, "bravo", "charlie")

	// Resume.
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = updated.(Model)

	if !m.follow {
		t.Fatal("f did not resume following")
	}

	if m.pending != 0 {
		t.Fatalf("pending = %d after resuming, want 0", m.pending)
	}

	if !strings.Contains(m.viewport.View(), "charlie") {
		t.Fatalf("resuming did not show the lines buffered while paused:\n%s", m.viewport.View())
	}
}

func TestFollowIsTheDefault(t *testing.T) {
	m := newPaused(t)

	if !m.follow {
		t.Fatal("a log view should start following; it is a tail")
	}
}
