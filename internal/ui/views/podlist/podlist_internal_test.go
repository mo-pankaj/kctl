package podlist

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
)

func loaded(t *testing.T, pods []core.Pod, cursor int) Model {
	t.Helper()

	m := New(zap.NewNop(), theme.New(), keys.Default(), nil, core.Selector{Namespace: "ns1"})
	m.pods = pods
	// Go through refresh so visible/table stay in step, exactly as the
	// loadedMsg path does in production.
	(&m).refresh()
	m.table.SetCursor(cursor)

	return m
}

func TestEmitSelectedReturnsTheHighlightedPod(t *testing.T) {
	pods := []core.Pod{
		{Namespace: "ns1", Name: "api-1"},
		{Namespace: "ns1", Name: "worker-2"},
		{Namespace: "ns1", Name: "cron-3"},
	}

	// refresh() name-sorts, so the visible order is api-1, cron-3, worker-2.
	// Cursor on the second row must therefore select cron-3; the test fails if
	// emitSelected ignores the cursor and always returns the first pod, and it
	// fails differently if it indexes the unsorted slice.
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

	if selected.Pod.Name != "cron-3" {
		t.Fatalf("SelectedMsg.Pod.Name = %q, want %q", selected.Pod.Name, "cron-3")
	}
}

func TestEmitSelectedOnAnEmptyListReturnsNoCommand(t *testing.T) {
	m := loaded(t, nil, 0)

	cmd := m.emitSelected()
	if cmd != nil {
		t.Fatal("emitSelected() returned a command with no pods loaded; Enter on an empty list must do nothing")
	}
}

// Filter lifecycle is asserted at the model level rather than through teatest:
// the "row disappears, then comes back" shape needs the LATEST frame, but
// teatest.WaitFor drains the output stream, so a second or third wait in one
// test sees nothing. This is deterministic and tests the same behaviour.
func TestEscapeClearsTheFilterAndRestoresEveryRow(t *testing.T) {
	pods := []core.Pod{
		{Namespace: "ns1", Name: "api-1"},
		{Namespace: "ns1", Name: "worker-2"},
		{Namespace: "ns1", Name: "worker-3"},
	}

	m := loaded(t, pods, 0)
	if len(m.visible) != 3 {
		t.Fatalf("visible = %d before filtering, want 3", len(m.visible))
	}

	m.filtering = true
	m.filter.Focus()
	m.filter.SetValue("worker")
	(&m).refresh()

	if len(m.visible) != 2 {
		t.Fatalf("visible = %d with filter %q, want 2", len(m.visible), "worker")
	}

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	after, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}

	if after.filtering {
		t.Fatal("still filtering after esc; esc must dismiss the filter")
	}

	if after.filter.Value() != "" {
		t.Fatalf("filter value = %q after esc, want it cleared", after.filter.Value())
	}

	if len(after.visible) != 3 {
		t.Fatalf("visible = %d after esc, want all 3 rows restored", len(after.visible))
	}
}

func TestRetryClearsTheErrorAndReloads(t *testing.T) {
	m := New(zap.NewNop(), theme.New(), keys.Default(), nil, core.Selector{Namespace: "ns1"})
	m.err = errors.New("error-listing-pods :forbidden")

	// r must clear the error and issue a fresh read.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

	after, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}

	if after.err != nil {
		t.Fatalf("err = %v after retry, want it cleared", after.err)
	}

	if cmd == nil {
		t.Fatal("retry produced no reload command")
	}
}

func TestRetryIsInertWithoutAnError(t *testing.T) {
	m := loaded(t, []core.Pod{{Namespace: "ns1", Name: "api-1"}}, 0)

	// Without the m.err guard this would fire a redundant cache read on every
	// press during normal browsing.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})

	if cmd != nil {
		t.Fatal("retry issued a reload with no error present; it must be inert")
	}
}

func TestColonTypesIntoTheFilterRatherThanEscapingIt(t *testing.T) {
	pods := []core.Pod{
		{Namespace: "ns1", Name: "api-1", Status: "Running"},
		{Namespace: "ns1", Name: "api-2", Status: "CrashLoopBackOff"},
	}

	m := loaded(t, pods, 0)
	m.filtering = true
	m.filter.Focus()

	// A column filter is mostly colons. If the view does not accept them the
	// feature is unusable, and the root must not treat ":" as its command key
	// while this input has focus.
	for _, r := range "status:crash" {
		updated, _ := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = updated.(Model)
	}

	if m.filter.Value() != "status:crash" {
		t.Fatalf("filter value = %q, want the whole term including the colon", m.filter.Value())
	}

	if len(m.visible) != 1 || m.visible[0].Name != "api-2" {
		t.Fatalf("visible = %v, want just the crashlooping pod", names(m.visible))
	}
}

func names(pods []core.Pod) (out []string) {
	for _, p := range pods {
		out = append(out, p.Name)
	}

	return out
}
