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
