package core_test

import (
	"testing"
	"time"

	"github.com/mo-pankaj/kctl/internal/core"
)

func TestPodReady(t *testing.T) {
	tests := []struct {
		name  string
		pod   core.Pod
		want  string
	}{
		{name: "all ready", pod: core.Pod{ReadyCount: 1, TotalCount: 1}, want: "1/1"},
		{name: "partially ready", pod: core.Pod{ReadyCount: 1, TotalCount: 3}, want: "1/3"},
		{name: "none ready", pod: core.Pod{ReadyCount: 0, TotalCount: 2}, want: "0/2"},
		{name: "no containers", pod: core.Pod{ReadyCount: 0, TotalCount: 0}, want: "0/0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pod.Ready()

			if got != tt.want {
				t.Fatalf("Ready() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPodAge(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		pod  core.Pod
		want time.Duration
	}{
		{
			name: "one hour old",
			pod:  core.Pod{CreatedAt: now.Add(-time.Hour)},
			want: time.Hour,
		},
		{
			name: "zero creation time yields zero age",
			pod:  core.Pod{},
			want: 0,
		},
		{
			name: "clock skew in the future clamps to zero",
			pod:  core.Pod{CreatedAt: now.Add(time.Minute)},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pod.Age(now)

			if got != tt.want {
				t.Fatalf("Age() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectorAllNamespaces(t *testing.T) {
	all := core.Selector{}
	if !all.AllNamespaces() {
		t.Fatal("empty Namespace should mean all namespaces")
	}

	scoped := core.Selector{Namespace: "trading-service"}
	if scoped.AllNamespaces() {
		t.Fatal("non-empty Namespace should not mean all namespaces")
	}
}
