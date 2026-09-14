package ui

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
)

// SourceFactory builds the data sources for one context and namespace. The
// returned sources stop when ctx is cancelled.
type SourceFactory func(ctx context.Context, contextName, namespace string) (core.PodReader, core.NamespaceLister, error)

// Active describes the live data sources.
type Active struct {
	Pods       core.PodReader
	Namespaces core.NamespaceLister
	Context    string
	Namespace  string
	Generation uint64
}

// Session owns the active cluster connection and the generation counter that
// makes stale notifications identifiable.
//
// Every switch increments the generation. Messages carrying an older generation
// are dropped by Update, which removes the race where a poke from the previous
// cluster arrives after the user has already moved on.
type Session struct {
	logger  *zap.Logger
	factory SourceFactory

	mu         sync.Mutex
	cancel     context.CancelFunc
	generation uint64
	active     Active
}

// NewSession builds a session.
func NewSession(logger *zap.Logger, factory SourceFactory) (s *Session) {
	s = &Session{
		logger:  logger.With(zap.String("component", "session")),
		factory: factory,
	}
	return s
}

// Switch tears down the current sources and builds new ones. On failure the
// previous sources are left running and the error is returned.
func (s *Session) Switch(parent context.Context, contextName, namespace string) (active Active, err error) {
	logger := s.logger.With(
		zap.String("method", "Switch"),
		zap.String("context", contextName),
		zap.String("namespace", namespace),
	)

	ctx, cancel := context.WithCancel(parent)

	pods, namespaces, err := s.factory(ctx, contextName, namespace)
	if err != nil {
		cancel()
		err = fmt.Errorf("error-building-sources :%w", err)
		logger.Error("error-building-sources", zap.Error(err))
		return active, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Only tear the old one down once the new one is known good.
	if s.cancel != nil {
		s.cancel()
	}

	s.cancel = cancel
	s.generation++

	s.active = Active{
		Pods:       pods,
		Namespaces: namespaces,
		Context:    contextName,
		Namespace:  namespace,
		Generation: s.generation,
	}

	active = s.active
	logger.Info("session-switched", zap.Uint64("generation", active.Generation))

	return active, err
}

// Active returns the live sources.
func (s *Session) Active() (active Active) {
	s.mu.Lock()
	defer s.mu.Unlock()

	active = s.active
	return active
}

// IsCurrent reports whether generation is the live one.
func (s *Session) IsCurrent(generation uint64) (current bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current = generation == s.generation
	return current
}

// Close ends the active sources.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}
