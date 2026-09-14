package kube

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/mo-pankaj/kctl/internal/core"
)

const (
	cacheSyncTimeout = 20 * time.Second
)

// PodSource serves pods from a shared informer cache.
//
// Watch events are never delivered as data. They poke subscribers, who re-read
// the cache. The cache is already correct and deduplicated, so this removes the
// need for any backpressure, drop, or event-ordering policy: a burst of several
// hundred updates during a rollout collapses into a single re-render.
type PodSource struct {
	logger  *zap.Logger
	client  kubernetes.Interface
	scope   string // namespace, or "" for all namespaces
	factory informers.SharedInformerFactory
	lister  listersv1.PodLister

	mu      sync.Mutex
	subs    []chan struct{}
	started bool
	closed  bool
}

// NewPodSource builds a source scoped to a namespace, or to all namespaces when
// scope is empty.
func NewPodSource(logger *zap.Logger, client kubernetes.Interface, scope string) (s *PodSource) {
	s = &PodSource{
		logger: logger.With(zap.String("component", "pod-source"), zap.String("scope", scope)),
		client: client,
		scope:  scope,
	}
	return s
}

// Start builds the informer, registers the poke handler, and blocks until the
// cache has synced. The source stops when ctx is cancelled.
func (s *PodSource) Start(ctx context.Context) (err error) {
	logger := s.logger.With(zap.String("method", "Start"))

	options := []informers.SharedInformerOption{}
	if s.scope != "" {
		options = append(options, informers.WithNamespace(s.scope))
	}

	s.factory = informers.NewSharedInformerFactoryWithOptions(s.client, 0, options...)
	podInformer := s.factory.Core().V1().Pods()
	s.lister = podInformer.Lister()

	_, err = podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(interface{}) { s.poke() },
		UpdateFunc: func(interface{}, interface{}) { s.poke() },
		DeleteFunc: func(interface{}) { s.poke() },
	})
	if err != nil {
		err = fmt.Errorf("error-adding-informer-handler :%w", err)
		logger.Error("error-adding-informer-handler", zap.Error(err))
		return err
	}

	s.factory.Start(ctx.Done())

	syncCtx, cancel := context.WithTimeout(ctx, cacheSyncTimeout)
	defer cancel()

	synced := s.factory.WaitForCacheSync(syncCtx.Done())
	for typ, ok := range synced {
		if !ok {
			err = fmt.Errorf("error-cache-sync-timeout :%v", typ)
			logger.Error("error-cache-sync-timeout", zap.String("type", fmt.Sprintf("%v", typ)))
			return err
		}
	}

	s.mu.Lock()
	s.started = true
	s.mu.Unlock()

	// Closing subscriber channels is what lets Update learn the source is gone.
	go func() {
		<-ctx.Done()
		s.close()
	}()

	logger.Info("pod-source-started")
	return err
}

// poke notifies every subscriber that the cache changed. Sends are
// non-blocking: a notification already pending is as good as two.
func (s *PodSource) poke() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	for _, ch := range s.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// close ends every subscription.
func (s *PodSource) close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.closed = true
	for _, ch := range s.subs {
		close(ch)
	}

	s.subs = nil
}

// Subscribe returns a coalescing notification channel. The channel carries no
// data: receive from it, then re-read via Pods.
func (s *PodSource) Subscribe(ctx context.Context, sel core.Selector) (dirty <-chan struct{}, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		err = fmt.Errorf("error-source-closed")
		return dirty, err
	}

	ch := make(chan struct{}, 1)
	s.subs = append(s.subs, ch)

	// Prime it so a fresh subscriber renders immediately rather than waiting for
	// the next cluster change.
	select {
	case ch <- struct{}{}:
	default:
	}

	dirty = ch
	return dirty, err
}

// Pods returns pods from the cache, narrowed by sel. A selector may narrow an
// all-namespaces source; it can never widen a namespace-scoped one.
func (s *PodSource) Pods(ctx context.Context, sel core.Selector) (pods []core.Pod, err error) {
	logger := s.logger.With(zap.String("method", "Pods"), zap.String("namespace", sel.Namespace))

	selector := labels.Everything()
	if sel.LabelSelector != "" {
		selector, err = labels.Parse(sel.LabelSelector)
		if err != nil {
			err = fmt.Errorf("error-parsing-label-selector :%w", err)
			logger.Error("error-parsing-label-selector", zap.Error(err))
			return pods, err
		}
	}

	var items []*corev1.Pod
	switch {
	case sel.Namespace == "":
		items, err = s.lister.List(selector)

	default:
		items, err = s.lister.Pods(sel.Namespace).List(selector)
	}

	if err != nil {
		err = fmt.Errorf("error-listing-pods :%w", err)
		logger.Error("error-listing-pods", zap.Error(err))
		return pods, err
	}

	pods = ToDomainPods(items)
	return pods, err
}

// Pod returns a single pod from the cache.
func (s *PodSource) Pod(ctx context.Context, ns, name string) (pod *core.Pod, err error) {
	logger := s.logger.With(zap.String("method", "Pod"), zap.String("namespace", ns), zap.String("pod", name))

	found, err := s.lister.Pods(ns).Get(name)
	if err != nil {
		err = fmt.Errorf("error-getting-pod :%w", err)
		logger.Error("error-getting-pod", zap.Error(err))
		return pod, err
	}

	converted := ToDomainPod(found)
	pod = &converted

	return pod, err
}
