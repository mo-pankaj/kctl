// Package stack implements kctl's view navigation.
//
// Exactly one view is visible at a time. Drilling in pushes; Esc pops. The root
// view can never be popped, so the stack is never empty once initialised and
// the root model never has to render "nothing".
package stack

import tea "charm.land/bubbletea/v2"

// Stack is a LIFO of views. The zero value is ready to use.
type Stack struct {
	views []tea.Model
}

// Push makes m the visible view.
func (s *Stack) Push(m tea.Model) {
	s.views = append(s.views, m)
}

// Pop removes the visible view and reveals the one beneath. It reports whether
// anything was popped; the last remaining view is never removed.
func (s *Stack) Pop() (popped bool) {
	if len(s.views) <= 1 {
		return popped
	}

	s.views = s.views[:len(s.views)-1]
	popped = true

	return popped
}

// Replace swaps the visible view without changing the stack depth.
func (s *Stack) Replace(m tea.Model) {
	if len(s.views) == 0 {
		s.Push(m)
		return
	}

	s.views[len(s.views)-1] = m
}

// Top returns the visible view, or nil when the stack is empty.
func (s *Stack) Top() (m tea.Model) {
	if len(s.views) == 0 {
		return m
	}

	m = s.views[len(s.views)-1]
	return m
}

// Len returns the stack depth.
func (s *Stack) Len() (n int) {
	n = len(s.views)
	return n
}

// Empty reports whether no view is stacked.
func (s *Stack) Empty() (empty bool) {
	empty = len(s.views) == 0
	return empty
}
