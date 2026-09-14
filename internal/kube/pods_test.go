package kube_test

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/kube"
)

type runtimeObject = k8sruntime.Object

func podFixture(ns, name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ns,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "app", Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}
}

func startSource(t *testing.T, scope string, objects ...runtimeObject) (*kube.PodSource, *fake.Clientset, context.Context) {
	t.Helper()

	client := fake.NewClientset(objects...)
	source := kube.NewPodSource(zap.NewNop(), client, scope)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	err := source.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	return source, client, ctx
}

func TestPodSourceImplementsReaderSubset(t *testing.T) {
	source, _, _ := startSource(t, "ns1")

	// Logs is added in Task 11; until then the source satisfies the read half.
	var _ interface {
		Pods(context.Context, core.Selector) ([]core.Pod, error)
		Pod(context.Context, string, string) (*core.Pod, error)
		Subscribe(context.Context, core.Selector) (<-chan struct{}, error)
	} = source
}

func TestPodsReadsFromCache(t *testing.T) {
	source, _, ctx := startSource(t, "ns1", podFixture("ns1", "api-1"), podFixture("ns1", "api-2"))

	pods, err := source.Pods(ctx, core.Selector{Namespace: "ns1"})
	if err != nil {
		t.Fatalf("Pods returned error: %v", err)
	}

	if len(pods) != 2 {
		t.Fatalf("Pods() returned %d pods, want 2", len(pods))
	}
}

func TestCreateFiresPokeAndCacheReflectsIt(t *testing.T) {
	source, client, ctx := startSource(t, "ns1")

	dirty, err := source.Subscribe(ctx, core.Selector{Namespace: "ns1"})
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	// Subscribe primes the channel so a fresh subscriber renders immediately.
	// Drain that priming poke first, or the wait below is satisfied by it and
	// the cache read races the informer's handling of the Create.
	select {
	case <-dirty:
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe did not prime its channel")
	}

	_, err = client.CoreV1().Pods("ns1").Create(ctx, podFixture("ns1", "api-new"), metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("creating pod returned error: %v", err)
	}

	select {
	case <-dirty:
	case <-time.After(5 * time.Second):
		t.Fatal("no poke received after creating a pod")
	}

	pods, err := source.Pods(ctx, core.Selector{Namespace: "ns1"})
	if err != nil {
		t.Fatalf("Pods returned error: %v", err)
	}

	if len(pods) != 1 || pods[0].Name != "api-new" {
		t.Fatalf("cache does not reflect the created pod: %+v", pods)
	}
}

func TestPokesCoalesce(t *testing.T) {
	source, _, ctx := startSource(t, "ns1")

	dirty, err := source.Subscribe(ctx, core.Selector{Namespace: "ns1"})
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}

	// The channel has capacity 1 and sends are non-blocking, so a burst can
	// never queue more than one pending notification. This is the property that
	// removes backpressure handling from the whole design.
	if cap(dirty) != 1 {
		t.Fatalf("poke channel capacity = %d, want 1 so bursts coalesce", cap(dirty))
	}
}

func TestPodReturnsNotFoundForMissingPod(t *testing.T) {
	source, _, ctx := startSource(t, "ns1")

	_, err := source.Pod(ctx, "ns1", "nope")
	if err == nil {
		t.Fatal("Pod() for a missing pod returned nil error, want an error")
	}
}

func TestAllNamespacesScopeSeesEveryNamespace(t *testing.T) {
	source, _, ctx := startSource(t, "", podFixture("ns1", "a"), podFixture("ns2", "b"))

	pods, err := source.Pods(ctx, core.Selector{})
	if err != nil {
		t.Fatalf("Pods returned error: %v", err)
	}

	if len(pods) != 2 {
		t.Fatalf("all-namespaces scope returned %d pods, want 2", len(pods))
	}

	// Narrowing the selector filters the same cache without a new informer.
	scoped, err := source.Pods(ctx, core.Selector{Namespace: "ns2"})
	if err != nil {
		t.Fatalf("Pods returned error: %v", err)
	}

	if len(scoped) != 1 || scoped[0].Name != "b" {
		t.Fatalf("namespace-narrowed read = %+v, want just pod b", scoped)
	}
}

func TestHealthyIsTrueAfterCacheSync(t *testing.T) {
	source, _, _ := startSource(t, "ns1")

	if !source.Healthy() {
		t.Fatal("Healthy() = false immediately after a successful Start")
	}
}

func TestHealthyIsFalseBeforeStart(t *testing.T) {
	source := kube.NewPodSource(zap.NewNop(), fake.NewClientset(), "ns1")

	if source.Healthy() {
		t.Fatal("Healthy() = true before Start; an unstarted source is not connected")
	}
}

func TestHealthyGoesFalseWhenTheSourceIsCancelled(t *testing.T) {
	source := kube.NewPodSource(zap.NewNop(), fake.NewClientset(), "ns1")

	ctx, cancel := context.WithCancel(context.Background())

	err := source.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	cancel()

	// close() runs on a goroutine watching ctx.Done, so poll briefly.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !source.Healthy() {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("Healthy() stayed true after the source's context was cancelled")
}
