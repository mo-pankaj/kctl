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
			name: "by age puts the oldest first",
			key:  podlist.SortAge,
			want: []string{"API-Gateway-1", "worker-4k2", "api-7d9f-2xk9"},
		},
		{
			name: "by restarts puts the most-restarted first",
			key:  podlist.SortRestarts,
			want: []string{"worker-4k2", "api-7d9f-2xk9", "API-Gateway-1"},
		},
		{
			name: "by status groups alphabetically, stably",
			key:  podlist.SortStatus,
			// CrashLoopBackOff sorts before Running. The two Running pods keep
			// their fixture order because the sort is stable — that is what stops
			// the table reshuffling on every poke.
			want: []string{"worker-4k2", "API-Gateway-1", "api-7d9f-2xk9"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(podlist.Sort(queryFixture(), tt.key))

			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("Sort(%v) = %v, want %v", tt.key, got, tt.want)
				}
			}
		})
	}
}

func TestSortIsStableWithinEqualKeys(t *testing.T) {
	// Two pods with identical status must keep their relative order, so the
	// table does not reshuffle on every poke.
	pods := []core.Pod{
		{Name: "b", Status: "Running"},
		{Name: "a", Status: "Running"},
	}

	got := names(podlist.Sort(pods, podlist.SortStatus))

	if got[0] != "b" || got[1] != "a" {
		t.Fatalf("Sort was not stable: got %v, want [b a]", got)
	}
}
