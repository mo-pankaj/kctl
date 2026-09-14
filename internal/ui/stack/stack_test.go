package stack_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/mo-pankaj/kctl/internal/ui/stack"
)

// fake is a minimal tea.Model used to identify stack entries.
type fake struct{ id string }

func (f fake) Init() tea.Cmd                       { return nil }
func (f fake) Update(tea.Msg) (tea.Model, tea.Cmd) { return f, nil }
func (f fake) View() tea.View                      { return tea.NewView(f.id) }

func TestEmptyStack(t *testing.T) {
	var s stack.Stack

	if !s.Empty() {
		t.Fatal("a zero-value Stack should be empty")
	}

	if s.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", s.Len())
	}

	if s.Top() != nil {
		t.Fatal("Top() on an empty stack should be nil")
	}
}

func TestPushAndTop(t *testing.T) {
	var s stack.Stack

	s.Push(fake{id: "a"})
	s.Push(fake{id: "b"})

	if s.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", s.Len())
	}

	top, ok := s.Top().(fake)
	if !ok || top.id != "b" {
		t.Fatalf("Top() = %v, want fake{b}", s.Top())
	}
}

func TestPopReturnsToPrevious(t *testing.T) {
	var s stack.Stack

	s.Push(fake{id: "a"})
	s.Push(fake{id: "b"})

	popped := s.Pop()
	if !popped {
		t.Fatal("Pop() = false, want true when more than one view is stacked")
	}

	top, ok := s.Top().(fake)
	if !ok || top.id != "a" {
		t.Fatalf("after Pop, Top() = %v, want fake{a}", s.Top())
	}
}

func TestPopRefusesToEmptyTheStack(t *testing.T) {
	var s stack.Stack

	s.Push(fake{id: "root"})

	popped := s.Pop()
	if popped {
		t.Fatal("Pop() = true on a single-entry stack; the root view must never be popped")
	}

	if s.Len() != 1 {
		t.Fatalf("Len() = %d after refused Pop, want 1", s.Len())
	}
}

func TestReplaceSwapsTopWithoutGrowing(t *testing.T) {
	var s stack.Stack

	s.Push(fake{id: "a"})
	s.Push(fake{id: "b"})
	s.Replace(fake{id: "c"})

	if s.Len() != 2 {
		t.Fatalf("Len() = %d after Replace, want 2", s.Len())
	}

	top, ok := s.Top().(fake)
	if !ok || top.id != "c" {
		t.Fatalf("after Replace, Top() = %v, want fake{c}", s.Top())
	}
}
