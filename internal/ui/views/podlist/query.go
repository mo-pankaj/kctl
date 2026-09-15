package podlist

import (
	"sort"
	"strings"

	"github.com/mo-pankaj/kctl/internal/core"
)

// SortKey names a column to order by.
type SortKey int

// The available sort orders, in the cycle order the sort key walks.
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

// SortDir is the direction a sort runs in.
type SortDir int

// Sort directions.
const (
	Asc SortDir = iota
	Desc
)

// String renders an arrow for the help line, so the direction is always visible
// rather than something the user has to infer from the rows.
func (d SortDir) String() (s string) {
	s = "↑"
	if d == Desc {
		s = "↓"
	}

	return s
}

// Toggle flips the direction.
func (d SortDir) Toggle() (next SortDir) {
	next = Desc
	if d == Desc {
		next = Asc
	}

	return next
}

// DefaultDir is the direction each column starts in: whichever end of that
// column a person is actually asking about.
//
//   - name      ascending, which reads like a directory listing
//   - status    ascending, and since statuses sort alphabetically that puts
//     CrashLoopBackOff and Error above Running
//   - restarts  descending, because the most-restarted pod is the question
//   - age       ascending. The arrow describes AGE, so ascending is the
//     smallest age: the newest pods, which is what "what just changed?" means.
//     Descending gives oldest-first for the rare case you want that.
func DefaultDir(k SortKey) (d SortDir) {
	switch k {
	case SortRestarts:
		d = Desc

	default:
		d = Asc
	}

	return d
}

// Sort orders a copy of pods by key and direction.
//
// Name is always the final tiebreak, so pods that compare equal on the chosen
// column keep a stable, predictable order instead of shuffling on every poke.
func Sort(pods []core.Pod, key SortKey, dir SortDir) (out []core.Pod) {
	out = make([]core.Pod, len(pods))
	copy(out, pods)

	byName := func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	}

	less := byName

	switch key {
	case SortStatus:
		less = func(i, j int) bool {
			a, b := strings.ToLower(out[i].Status), strings.ToLower(out[j].Status)
			if a == b {
				return byName(i, j)
			}

			return a < b
		}

	case SortRestarts:
		less = func(i, j int) bool {
			if out[i].Restarts == out[j].Restarts {
				return byName(i, j)
			}

			return out[i].Restarts < out[j].Restarts
		}

	case SortAge:
		less = func(i, j int) bool {
			if out[i].CreatedAt.Equal(out[j].CreatedAt) {
				return byName(i, j)
			}

			// Ascending age means youngest first, so a later creation time is
			// "less". Reading the arrow as age, not as timestamp, is what a
			// user expects from a column labelled AGE.
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if dir == Desc {
			return less(j, i)
		}

		return less(i, j)
	})

	return out
}

// Filter narrows pods by a query.
//
// A bare word matches the name. A "column:value" term matches that column, so
// "status:crash" finds the crashlooping pods without their names having to
// contain anything in particular. Terms are ANDed, so "status:running api"
// means both.
//
// Unknown columns match nothing rather than silently matching everything: a
// typo should show an empty list, not the whole list.
func Filter(pods []core.Pod, query string) (out []core.Pod) {
	terms := strings.Fields(strings.TrimSpace(query))
	if len(terms) == 0 {
		out = make([]core.Pod, len(pods))
		copy(out, pods)

		return out
	}

	for _, p := range pods {
		if matchesAll(p, terms) {
			out = append(out, p)
		}
	}

	return out
}

func matchesAll(p core.Pod, terms []string) (ok bool) {
	for _, term := range terms {
		if !matchesTerm(p, term) {
			return ok
		}
	}

	ok = true

	return ok
}

func matchesTerm(p core.Pod, term string) (ok bool) {
	column, value, found := strings.Cut(term, ":")
	if !found || value == "" {
		ok = contains(p.Name, term)

		return ok
	}

	switch strings.ToLower(column) {
	case "name":
		ok = contains(p.Name, value)

	case "status", "s":
		ok = contains(p.Status, value)

	case "ns", "namespace":
		ok = contains(p.Namespace, value)

	case "node":
		ok = contains(p.Node, value)

	case "ready":
		ok = contains(p.Ready(), value)

	case "restarts":
		// "restarts:1" means at least one, which is the question being asked.
		ok = atLeast(p.Restarts, value)
	}

	return ok
}

// atLeast reports whether n meets the numeric threshold in value.
func atLeast(n int32, value string) (ok bool) {
	var threshold int32

	for _, r := range value {
		if r < '0' || r > '9' {
			return ok
		}

		threshold = threshold*10 + (r - '0')
	}

	ok = n >= threshold

	return ok
}

func contains(haystack, needle string) (ok bool) {
	ok = strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))

	return ok
}

// Columns lists the filterable column names, for the hint shown under the
// filter input.
func Columns() (names []string) {
	names = []string{"name", "status", "ns", "node", "ready", "restarts"}

	return names
}
