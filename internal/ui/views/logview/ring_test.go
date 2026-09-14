package logview_test

import (
	"fmt"
	"testing"

	"github.com/mo-pankaj/kctl/internal/ui/views/logview"
)

func TestRingHoldsLinesInOrder(t *testing.T) {
	r := logview.NewRing(5)

	r.Add("one")
	r.Add("two")
	r.Add("three")

	got := r.Lines()

	if len(got) != 3 {
		t.Fatalf("Lines() returned %d lines, want 3", len(got))
	}

	want := []string{"one", "two", "three"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Lines() = %v, want %v", got, want)
		}
	}

	if r.Truncated() {
		t.Fatal("Truncated() = true before the buffer filled")
	}
}

func TestRingDropsOldestWhenFull(t *testing.T) {
	r := logview.NewRing(3)

	for i := 1; i <= 5; i++ {
		r.Add(fmt.Sprintf("line-%d", i))
	}

	got := r.Lines()

	if len(got) != 3 {
		t.Fatalf("Lines() returned %d lines, want 3 (the capacity)", len(got))
	}

	want := []string{"line-3", "line-4", "line-5"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Lines() = %v, want %v (oldest dropped)", got, want)
		}
	}
}

func TestRingReportsTruncationOnceItHasDropped(t *testing.T) {
	r := logview.NewRing(2)

	r.Add("a")
	r.Add("b")

	if r.Truncated() {
		t.Fatal("Truncated() = true when exactly at capacity with nothing dropped")
	}

	r.Add("c")

	if !r.Truncated() {
		t.Fatal("Truncated() = false after dropping a line; the header must say so")
	}
}

func TestRingCapacityFloorIsOne(t *testing.T) {
	// A zero or negative capacity would panic on modulo; clamp instead.
	r := logview.NewRing(0)

	r.Add("only")

	got := r.Lines()
	if len(got) != 1 || got[0] != "only" {
		t.Fatalf("Lines() = %v, want [only]", got)
	}
}

func TestRingLinesReturnsACopy(t *testing.T) {
	r := logview.NewRing(3)
	r.Add("a")

	lines := r.Lines()
	lines[0] = "mutated"

	if r.Lines()[0] != "a" {
		t.Fatal("Lines() exposed internal storage; callers must not be able to corrupt the buffer")
	}
}
