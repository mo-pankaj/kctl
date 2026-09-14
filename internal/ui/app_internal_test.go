package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"go.uber.org/zap"
)

// TestViewRendersQuitHintFromKeymap rebinds the quit key to a distinctive
// value and checks the rendered hint follows it. This is an internal test
// (package ui, not ui_test) because it needs to reach the unexported keys
// field directly; New does not expose a way to override the keymap, and
// adding one just to serve this test isn't warranted.
func TestViewRendersQuitHintFromKeymap(t *testing.T) {
	m := New(zap.NewNop())
	m.keys.Quit = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "exit"))

	view := m.View()

	if !strings.Contains(view, "press x to exit") {
		t.Fatalf("View() = %q, want it to contain the rebound hint %q", view, "press x to exit")
	}

	if strings.Contains(view, "press q to quit") {
		t.Fatalf("View() = %q, want it not to contain the stale default hint %q", view, "press q to quit")
	}
}
