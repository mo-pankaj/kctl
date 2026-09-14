package core

import (
	"context"
	"io"
)

//go:generate go tool mockgen -source=ports.go -destination=mocks/ports_mock.go -package=mocks

// ContextManager lists and switches kubeconfig contexts.
type ContextManager interface {
	// Contexts returns every context in the loaded kubeconfig, sorted by name.
	Contexts() []ContextInfo
	// Current returns the active context.
	Current() ContextInfo
	// Use switches the active context. It does not modify the kubeconfig file.
	Use(ctx context.Context, name string) error
}

// NamespaceLister lists namespaces in the active context.
type NamespaceLister interface {
	Namespaces(ctx context.Context) ([]string, error)
}

// PodReader reads pods and opens log streams for the active context.
type PodReader interface {
	// Pods returns the pods matching sel from the informer cache.
	Pods(ctx context.Context, sel Selector) ([]Pod, error)
	// Pod returns a single pod from the informer cache.
	Pod(ctx context.Context, ns, name string) (*Pod, error)
	// Subscribe returns a channel that receives a value whenever the cache for
	// sel has changed. The channel carries no data: the receiver re-reads via
	// Pods. Sends are coalescing and non-blocking, so a burst of cluster
	// activity yields one notification rather than a queue.
	Subscribe(ctx context.Context, sel Selector) (<-chan struct{}, error)
	// Logs opens a log stream. The caller must Close the reader.
	Logs(ctx context.Context, req LogRequest) (io.ReadCloser, error)
}

// PodDescriber returns the detail and events behind a single pod. Events are
// fetched on demand rather than watched: they are only ever read while a
// describe view is open, and a stale event list is worse than a live call.
type PodDescriber interface {
	Describe(ctx context.Context, ns, name string) (*PodDetail, error)
	Events(ctx context.Context, ns, name string) ([]Event, error)
}
