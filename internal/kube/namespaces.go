package kube

import (
	"context"
	"fmt"
	"sort"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// NamespaceStore lists namespaces on demand.
//
// Namespaces are not cached: the list is read when the user asks to switch,
// which is rare, and a stale namespace list is more confusing than a one-off
// API call is expensive.
type NamespaceStore struct {
	logger *zap.Logger
	client kubernetes.Interface
}

// NewNamespaceStore builds the store.
func NewNamespaceStore(logger *zap.Logger, client kubernetes.Interface) (s *NamespaceStore) {
	s = &NamespaceStore{
		logger: logger.With(zap.String("component", "namespace-store")),
		client: client,
	}
	return s
}

// Namespaces returns every namespace name, sorted.
func (s *NamespaceStore) Namespaces(ctx context.Context) (names []string, err error) {
	logger := s.logger.With(zap.String("method", "Namespaces"))

	list, err := s.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		err = fmt.Errorf("error-listing-namespaces :%w", err)
		logger.Error("error-listing-namespaces", zap.Error(err))
		return names, err
	}

	for _, ns := range list.Items {
		names = append(names, ns.Name)
	}

	sort.Strings(names)
	return names, err
}
