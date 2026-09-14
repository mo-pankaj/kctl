// Package kube implements kctl's core ports against client-go.
package kube

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/mo-pankaj/kctl/internal/core"
)

// ContextStore reads kubeconfig and tracks which context kctl is pointed at.
//
// Switching contexts is process-local: the kubeconfig file on disk is never
// modified, so other terminals keep whatever context they had.
type ContextStore struct {
	logger *zap.Logger
	rules  *clientcmd.ClientConfigLoadingRules

	mu      sync.RWMutex
	raw     clientcmdapi.Config
	current string
}

// NewContextStore loads kubeconfig. An empty explicitPath uses the standard
// discovery rules, honouring KUBECONFIG.
func NewContextStore(logger *zap.Logger, explicitPath string) (store *ContextStore, err error) {
	logger = logger.With(zap.String("component", "context-store"))

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if explicitPath != "" {
		rules = &clientcmd.ClientConfigLoadingRules{ExplicitPath: explicitPath}
	}

	raw, err := rules.Load()
	if err != nil {
		err = fmt.Errorf("error-loading-kubeconfig :%w", err)
		logger.Error("error-loading-kubeconfig", zap.Error(err))
		return store, err
	}

	if len(raw.Contexts) == 0 {
		err = fmt.Errorf("error-no-contexts-in-kubeconfig")
		logger.Error("error-no-contexts-in-kubeconfig")
		return store, err
	}

	current := raw.CurrentContext
	_, ok := raw.Contexts[current]
	if !ok {
		// A kubeconfig whose current-context is dangling is valid on disk but
		// unusable; fall back to the first context by name rather than failing.
		names := contextNames(raw)
		current = names[0]
		logger.Warn("kubeconfig-current-context-missing", zap.String("fallback", current))
	}

	store = &ContextStore{
		logger:  logger,
		rules:   rules,
		raw:     *raw,
		current: current,
	}

	return store, err
}

func contextNames(raw *clientcmdapi.Config) (names []string) {
	for name := range raw.Contexts {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

// Contexts returns every context, sorted by name.
func (s *ContextStore) Contexts() (infos []core.ContextInfo) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	raw := s.raw
	for _, name := range contextNames(&raw) {
		infos = append(infos, s.infoLocked(name))
	}

	return infos
}

// Current returns the active context.
func (s *ContextStore) Current() (info core.ContextInfo) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info = s.infoLocked(s.current)
	return info
}

// infoLocked builds a ContextInfo. Callers must hold at least a read lock.
func (s *ContextStore) infoLocked(name string) (info core.ContextInfo) {
	kctx, ok := s.raw.Contexts[name]
	if !ok {
		info = core.ContextInfo{Name: name}
		return info
	}

	namespace := kctx.Namespace
	if namespace == "" {
		namespace = "default"
	}

	info = core.ContextInfo{
		Name:      name,
		Cluster:   kctx.Cluster,
		Namespace: namespace,
		Current:   name == s.current,
	}

	cluster, ok := s.raw.Clusters[kctx.Cluster]
	if ok {
		info.Server = cluster.Server
	}

	return info
}

// Use switches the active context without touching the kubeconfig file.
func (s *ContextStore) Use(ctx context.Context, name string) (err error) {
	logger := s.logger.With(zap.String("method", "Use"), zap.String("context", name))

	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.raw.Contexts[name]
	if !ok {
		err = fmt.Errorf("error-unknown-context :%s", name)
		logger.Error("error-unknown-context")
		return err
	}

	s.current = name
	logger.Info("context-switched")

	return err
}

// RESTConfig builds a REST config for the named context, which need not be the
// active one.
func (s *ContextStore) RESTConfig(name string) (cfg *rest.Config, err error) {
	logger := s.logger.With(zap.String("method", "RESTConfig"), zap.String("context", name))

	overrides := &clientcmd.ConfigOverrides{CurrentContext: name}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(s.rules, overrides)

	cfg, err = clientConfig.ClientConfig()
	if err != nil {
		err = fmt.Errorf("error-building-rest-config :%w", err)
		logger.Error("error-building-rest-config", zap.Error(err))
		return cfg, err
	}

	return cfg, err
}
