package ui_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/ui"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

func TestAppRendersAndQuitsOnQ(t *testing.T) {
	tm := teatest.NewTestModel(t, ui.New(zap.NewNop()), teatest.WithInitialTermSize(100, 30))

	// Assert against streaming output, never FinalOutput: after tea.Quit the
	// terminal is restored and the rendered view is gone from FinalOutput.
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("kctl"))
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestAppViewRendersQuitHintFromKeymap(t *testing.T) {
	m := ui.New(zap.NewNop())

	view := m.View()

	help := keys.Default().Quit.Help()
	if !strings.Contains(view, help.Key) {
		t.Fatalf("View() = %q, want it to contain the keymap's quit key %q", view, help.Key)
	}
}
