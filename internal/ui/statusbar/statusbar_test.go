package statusbar_test

import (
	"strings"
	"testing"

	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/statusbar"
)

func newBar() statusbar.Model {
	m := statusbar.New(theme.New())
	m.Width = 100
	return m
}

func TestViewShowsContextAndNamespace(t *testing.T) {
	m := newBar()
	m.Context = "dev-01"
	m.Namespace = "trading-service"

	got := m.View()

	if !strings.Contains(got, "dev-01") {
		t.Fatalf("View() missing context name, got: %q", got)
	}

	if !strings.Contains(got, "trading-service") {
		t.Fatalf("View() missing namespace, got: %q", got)
	}
}

func TestViewShowsAllNamespacesMarker(t *testing.T) {
	m := newBar()
	m.Context = "dev-01"
	m.Namespace = ""

	got := m.View()

	if !strings.Contains(got, "all namespaces") {
		t.Fatalf("View() with empty namespace should say 'all namespaces', got: %q", got)
	}
}

func TestViewShowsConnectingState(t *testing.T) {
	m := newBar()
	m.Context = "cnc-prod"
	m.Connecting = true

	got := m.View()

	if !strings.Contains(got, "connecting") {
		t.Fatalf("View() while connecting should say so, got: %q", got)
	}
}

func TestViewShowsMessage(t *testing.T) {
	m := newBar()
	m.Context = "dev-01"
	m.Namespace = "default"
	m.Message = "error-listing-pods: forbidden"

	got := m.View()

	if !strings.Contains(got, "forbidden") {
		t.Fatalf("View() missing message, got: %q", got)
	}
}

func TestLongMessageIsTruncatedToOneLine(t *testing.T) {
	m := newBar()
	m.Width = 80
	m.Context = "kind-dev"
	m.Namespace = "default"
	m.Message = strings.Repeat("error-building-sources :error-starting-pod-source ", 6)

	got := m.View()

	// A wrapped status bar pushes the view under it off screen, which reads as
	// the list emptying rather than a message appearing.
	for _, line := range strings.Split(got, "\n") {
		if len([]rune(stripANSI(line))) > m.Width+1 {
			t.Fatalf("status bar line is %d columns wide, want <= %d:\n%q", len([]rune(stripANSI(line))), m.Width, line)
		}
	}

	if !strings.Contains(got, "…") {
		t.Fatalf("a too-long message should be elided; got %q", got)
	}
}

func stripANSI(s string) string {
	var b strings.Builder

	inEscape := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEscape = true
		case inEscape && (r == 'm' || r == 'K'):
			inEscape = false
		case !inEscape:
			b.WriteRune(r)
		}
	}

	return b.String()
}
