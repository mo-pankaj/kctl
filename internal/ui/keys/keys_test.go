package keys_test

import (
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

func TestHelpLineRendersKeyAndDescription(t *testing.T) {
	got := keys.HelpLine(
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	)

	want := "enter:select  esc:back"
	if got != want {
		t.Fatalf("HelpLine() = %q, want %q", got, want)
	}
}

func TestHelpLineFollowsARebinding(t *testing.T) {
	// The point of the helper: rebinding changes the rendered text. A literal
	// help string would not, which is the regression this guards.
	got := keys.HelpLine(key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "exit")))

	if got != "x:exit" {
		t.Fatalf("HelpLine() = %q, want %q", got, "x:exit")
	}
}

func TestHelpLineSkipsBindingsWithoutHelp(t *testing.T) {
	got := keys.HelpLine(
		key.NewBinding(key.WithKeys("a")),
		key.NewBinding(key.WithKeys("b"), key.WithHelp("b", "back")),
	)

	if got != "b:back" {
		t.Fatalf("HelpLine() = %q, want %q", got, "b:back")
	}
}
