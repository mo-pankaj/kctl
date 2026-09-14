// Package core defines kctl's domain types and the role interfaces the UI
// depends on.
//
// This package imports no Kubernetes, Bubble Tea, or logging libraries. That
// boundary is what lets every view be tested against a generated mock with no
// cluster and no API server binary.
package core

import (
	"fmt"
	"time"
)

// ContextInfo describes one kubeconfig context.
type ContextInfo struct {
	Name      string
	Cluster   string
	Server    string
	Namespace string
	Current   bool
}

// Selector narrows a resource query. An empty Namespace means all namespaces.
type Selector struct {
	Namespace     string
	LabelSelector string
}

// AllNamespaces reports whether the selector spans every namespace.
func (s Selector) AllNamespaces() (all bool) {
	all = s.Namespace == ""
	return all
}

// Pod is the projection of a Kubernetes pod that kctl renders.
type Pod struct {
	Namespace  string
	Name       string
	Status     string
	Node       string
	ReadyCount int
	TotalCount int
	Restarts   int32
	CreatedAt  time.Time
	Containers []string
}

// Ready renders the ready/total column, e.g. "1/1".
func (p Pod) Ready() (s string) {
	s = fmt.Sprintf("%d/%d", p.ReadyCount, p.TotalCount)
	return s
}

// Age returns how long the pod has existed as of now, clamped at zero so clock
// skew between the cluster and this machine cannot render a negative age.
func (p Pod) Age(now time.Time) (d time.Duration) {
	if p.CreatedAt.IsZero() {
		return d
	}

	d = now.Sub(p.CreatedAt)
	if d < 0 {
		d = 0
	}

	return d
}

// LogRequest describes a log stream to open.
type LogRequest struct {
	Namespace string
	Pod       string
	Container string
	Follow    bool
	TailLines int64
}
