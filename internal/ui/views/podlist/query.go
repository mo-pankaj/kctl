package podlist

import (
	"sort"
	"strings"

	"github.com/mo-pankaj/kctl/internal/core"
)

// SortKey names a column to order by.
type SortKey int

// The available sort orders, in the cycle order the `s` key walks.
const (
	SortName SortKey = iota
	SortStatus
	SortRestarts
	SortAge
)

// String renders the key for the help line.
func (k SortKey) String() (s string) {
	switch k {
	case SortStatus:
		s = "status"

	case SortRestarts:
		s = "restarts"

	case SortAge:
		s = "age"

	default:
		s = "name"
	}

	return s
}

// Next returns the following key in the cycle.
func (k SortKey) Next() (next SortKey) {
	next = (k + 1) % 4
	return next
}

// Filter returns the pods whose name contains query, case-insensitively. An
// empty or whitespace-only query returns every pod. The input is never mutated.
func Filter(pods []core.Pod, query string) (out []core.Pod) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		out = make([]core.Pod, len(pods))
		copy(out, pods)
		return out
	}

	needle := strings.ToLower(trimmed)

	for _, p := range pods {
		if strings.Contains(strings.ToLower(p.Name), needle) {
			out = append(out, p)
		}
	}

	return out
}

// Sort orders a copy of pods by key. The sort is stable, so pods with equal keys
// keep their relative order and the table does not reshuffle on every poke.
func Sort(pods []core.Pod, key SortKey) (out []core.Pod) {
	out = make([]core.Pod, len(pods))
	copy(out, pods)

	switch key {
	case SortStatus:
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].Status) < strings.ToLower(out[j].Status)
		})

	case SortRestarts:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Restarts > out[j].Restarts
		})

	case SortAge:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		})

	default:
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		})
	}

	return out
}
