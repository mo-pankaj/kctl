package kube_test

import (
	"context"
	"testing"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/kube"
)

func TestNamespaceStoreListsSorted(t *testing.T) {
	client := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "trading-service"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
	)

	store := kube.NewNamespaceStore(zap.NewNop(), client)

	var _ core.NamespaceLister = store

	got, err := store.Namespaces(context.Background())
	if err != nil {
		t.Fatalf("Namespaces returned error: %v", err)
	}

	want := []string{"default", "kube-system", "trading-service"}
	if len(got) != len(want) {
		t.Fatalf("Namespaces() = %v, want %v", got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Namespaces() = %v, want %v (sorted)", got, want)
		}
	}
}
