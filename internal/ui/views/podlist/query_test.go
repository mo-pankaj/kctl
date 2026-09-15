package podlist_test

import (
	"testing"
	"time"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/ui/views/podlist"
)

func queryFixture() []core.Pod {
	base := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	return []core.Pod{
		{Name: "worker-4k2", Status: "CrashLoopBackOff", Restarts: 7, CreatedAt: base.Add(-2 * time.Hour)},
		{Name: "API-Gateway-1", Status: "Running", Restarts: 0, CreatedAt: base.Add(-5 * time.Hour)},
		{Name: "api-7d9f-2xk9", Status: "Running", Restarts: 2, CreatedAt: base.Add(-1 * time.Hour)},
	}
}

func names(pods []core.Pod) (out []string) {
	for _, p := range pods {
		out = append(out, p.Name)
	}
	return out
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{
			name:  "empty query returns everything unchanged",
			query: "",
			want:  []string{"worker-4k2", "API-Gateway-1", "api-7d9f-2xk9"},
		},
		{
			name:  "substring match",
			query: "api",
			want:  []string{"API-Gateway-1", "api-7d9f-2xk9"},
		},
		{
			name:  "match is case insensitive in both directions",
			query: "GATEWAY",
			want:  []string{"API-Gateway-1"},
		},
		{
			name:  "no match returns empty, not everything",
			query: "zzz",
			want:  nil,
		},
		{
			name:  "whitespace-only query is treated as empty",
			query: "   ",
			want:  []string{"worker-4k2", "API-Gateway-1", "api-7d9f-2xk9"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(podlist.Filter(queryFixture(), tt.query))

			if len(got) != len(tt.want) {
				t.Fatalf("Filter(%q) = %v, want %v", tt.query, got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("Filter(%q) = %v, want %v", tt.query, got, tt.want)
				}
			}
		})
	}
}

func TestFilterDoesNotMutateInput(t *testing.T) {
	pods := queryFixture()
	original := names(pods)

	podlist.Filter(pods, "api")

	after := names(pods)
	for i := range original {
		if original[i] != after[i] {
			t.Fatalf("Filter mutated its input: was %v, now %v", original, after)
		}
	}
}

func TestSort(t *testing.T) {
	tests := []struct {
		name string
		key  podlist.SortKey
		want []string
	}{
		{
			name: "by name is case insensitive",
			key:  podlist.SortName,
			want: []string{"api-7d9f-2xk9", "API-Gateway-1", "worker-4k2"},
		},
		{
			name: "ascending age is youngest first",
			key:  podlist.SortAge,
			// The arrow describes AGE, not the timestamp: ascending means the
			// smallest age, which is the most recently created pod.
			want: []string{"api-7d9f-2xk9", "worker-4k2", "API-Gateway-1"},
		},
		{
			name: "ascending restarts is fewest first",
			key:  podlist.SortRestarts,
			want: []string{"API-Gateway-1", "api-7d9f-2xk9", "worker-4k2"},
		},
		{
			name: "by status, then by name",
			key:  podlist.SortStatus,
			// CrashLoopBackOff sorts before Running. The two Running pods are
			// then ordered by name, so the result does not depend on the order
			// the cache happened to return them in.
			want: []string{"worker-4k2", "api-7d9f-2xk9", "API-Gateway-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(podlist.Sort(queryFixture(), tt.key, podlist.Asc))

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("Sort(%v) = %v, want %v", tt.key, got, tt.want)
				}
			}
		})
	}
}

func TestSortDoesNotDependOnInputOrder(t *testing.T) {
	// Equal on the sorted column, so only the name tiebreak can order them.
	// Feeding the same pods in either order must give the same result, because
	// the informer cache makes no promise about the order it returns.
	forward := []core.Pod{
		{Name: "a", Status: "Running"},
		{Name: "b", Status: "Running"},
	}
	reversed := []core.Pod{
		{Name: "b", Status: "Running"},
		{Name: "a", Status: "Running"},
	}

	one := names(podlist.Sort(forward, podlist.SortStatus, podlist.Asc))
	two := names(podlist.Sort(reversed, podlist.SortStatus, podlist.Asc))

	if one[0] != "a" || one[1] != "b" {
		t.Fatalf("Sort = %v, want [a b]", one)
	}

	if two[0] != one[0] || two[1] != one[1] {
		t.Fatalf("result depends on input order: %v vs %v", one, two)
	}
}

func TestSortDirectionReversesTheOrder(t *testing.T) {
	asc := names(podlist.Sort(queryFixture(), podlist.SortName, podlist.Asc))
	desc := names(podlist.Sort(queryFixture(), podlist.SortName, podlist.Desc))

	if len(asc) != len(desc) {
		t.Fatalf("different lengths: %v vs %v", asc, desc)
	}

	for i := range asc {
		if asc[i] != desc[len(desc)-1-i] {
			t.Fatalf("descending is not the reverse of ascending:\nasc  %v\ndesc %v", asc, desc)
		}
	}
}

func TestNameIsAlwaysTheFinalTiebreak(t *testing.T) {
	// Same status, same restarts: only the name can order these, and it must do
	// so deterministically or the table reshuffles on every poke.
	pods := []core.Pod{
		{Name: "zebra", Status: "Running"},
		{Name: "alpha", Status: "Running"},
		{Name: "mango", Status: "Running"},
	}

	got := names(podlist.Sort(pods, podlist.SortStatus, podlist.Asc))

	want := []string{"alpha", "mango", "zebra"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Sort = %v, want %v (name as tiebreak)", got, want)
		}
	}
}

func TestDefaultDirectionSuitsTheColumn(t *testing.T) {
	tests := []struct {
		key  podlist.SortKey
		want podlist.SortDir
	}{
		{key: podlist.SortName, want: podlist.Asc},
		{key: podlist.SortStatus, want: podlist.Asc},
		// The most-restarted pod is the question being asked.
		{key: podlist.SortRestarts, want: podlist.Desc},
		// Ascending AGE is the smallest age, i.e. newest first — which is what
		// "what just changed?" means. Descending would be oldest first.
		{key: podlist.SortAge, want: podlist.Asc},
	}

	for _, tt := range tests {
		if got := podlist.DefaultDir(tt.key); got != tt.want {
			t.Fatalf("DefaultDir(%v) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestSortDirRendersAnArrow(t *testing.T) {
	if podlist.Asc.String() != "↑" || podlist.Desc.String() != "↓" {
		t.Fatalf("directions render as %q/%q", podlist.Asc, podlist.Desc)
	}

	if podlist.Asc.Toggle() != podlist.Desc || podlist.Desc.Toggle() != podlist.Asc {
		t.Fatal("Toggle does not flip")
	}
}

func columnFixture() []core.Pod {
	return []core.Pod{
		{Name: "api-1", Namespace: "prod", Status: "Running", Node: "node-a", Restarts: 0, ReadyCount: 1, TotalCount: 1},
		{Name: "api-2", Namespace: "staging", Status: "CrashLoopBackOff", Node: "node-b", Restarts: 7, ReadyCount: 0, TotalCount: 1},
		{Name: "worker-1", Namespace: "prod", Status: "Pending", Node: "node-a", Restarts: 2, ReadyCount: 0, TotalCount: 2},
	}
}

func TestFilterByColumn(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{name: "bare word still matches the name", query: "api", want: []string{"api-1", "api-2"}},
		{name: "status", query: "status:crash", want: []string{"api-2"}},
		{name: "status shorthand", query: "s:pending", want: []string{"worker-1"}},
		{name: "namespace", query: "ns:prod", want: []string{"api-1", "worker-1"}},
		{name: "node", query: "node:node-b", want: []string{"api-2"}},
		{name: "ready", query: "ready:0/2", want: []string{"worker-1"}},
		{name: "restarts is a threshold", query: "restarts:2", want: []string{"api-2", "worker-1"}},
		{name: "terms are ANDed", query: "ns:prod status:running", want: []string{"api-1"}},
		{name: "column and bare word combine", query: "ns:prod worker", want: []string{"worker-1"}},
		{name: "matching is case insensitive", query: "STATUS:RUNNING", want: []string{"api-1"}},
		{name: "an unknown column matches nothing", query: "colour:blue", want: nil},
		{name: "a typo shows an empty list, not everything", query: "stats:running", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(podlist.Filter(columnFixture(), tt.query))

			if len(got) != len(tt.want) {
				t.Fatalf("Filter(%q) = %v, want %v", tt.query, got, tt.want)
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("Filter(%q) = %v, want %v", tt.query, got, tt.want)
				}
			}
		})
	}
}
