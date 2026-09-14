package podlist

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

func loaded(t *testing.T, pods []core.Pod, cursor int) Model {
	t.Helper()

	m := New(zap.NewNop(), theme.New(), keys.Default(), nil, core.Selector{Namespace: "ns1"})
	m.pods = pods
	m.table.SetRows(rowsFor(pods, false, time.Now()))
	m.table.SetCursor(cursor)

	return m
}

func TestEmitSelectedReturnsTheHighlightedPod(t *testing.T) {
	pods := []core.Pod{
		{Namespace: "ns1", Name: "api-1"},
		{Namespace: "ns1", Name: "worker-2"},
		{Namespace: "ns1", Name: "cron-3"},
	}

	// Cursor on the second row: the test fails if emitSelected ignores the
	// cursor and always returns the first pod.
	m := loaded(t, pods, 1)

	cmd := m.emitSelected()
	if cmd == nil {
		t.Fatal("emitSelected() returned a nil command for a valid cursor")
	}

	msg := cmd()

	selected, ok := msg.(SelectedMsg)
	if !ok {
		t.Fatalf("emitSelected() produced %T, want SelectedMsg", msg)
	}

	if selected.Pod.Name != "worker-2" {
		t.Fatalf("SelectedMsg.Pod.Name = %q, want %q", selected.Pod.Name, "worker-2")
	}
}

func TestEmitSelectedOnAnEmptyListReturnsNoCommand(t *testing.T) {
	m := loaded(t, nil, 0)

	cmd := m.emitSelected()
	if cmd != nil {
		t.Fatal("emitSelected() returned a command with no pods loaded; Enter on an empty list must do nothing")
	}
}
