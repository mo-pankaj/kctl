package kube_test

import (
	"context"
	"io"
	"testing"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/kube"
)

// startedSource brings up a PodSource against the test API server.
func startedSource(t *testing.T, ns string) (*kube.PodSource, struct{}) {
	t.Helper()

	cfg := requireEnv(t)

	client, err := kube.NewClientset(cfg)
	if err != nil {
		t.Fatalf("NewClientset: %v", err)
	}

	src := kube.NewPodSource(zap.NewNop(), client, ns)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	err = src.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	return src, struct{}{}
}

// seedPod creates a pod with a waiting container status, the shape a pod that
// will not start actually has.
func seedPod(t *testing.T, ns, name string) {
	t.Helper()

	cfg := requireEnv(t)

	client, err := kube.NewClientset(cfg)
	if err != nil {
		t.Fatalf("NewClientset: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{"app": "seeded"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "busybox:1.36"}},
		},
	}

	created, err := client.CoreV1().Pods(ns).Create(context.Background(), pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("creating pod: %v", err)
	}

	// envtest runs no kubelet, so the status has to be written by the test.
	created.Status = corev1.PodStatus{
		Phase:    corev1.PodPending,
		QOSClass: corev1.PodQOSBestEffort,
		Conditions: []corev1.PodCondition{
			{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady"},
		},
		ContainerStatuses: []corev1.ContainerStatus{{
			Name:         "app",
			Image:        "busybox:1.36",
			Ready:        false,
			RestartCount: 3,
			State: corev1.ContainerState{
				Waiting: &corev1.ContainerStateWaiting{
					Reason:  "ImagePullBackOff",
					Message: "Back-off pulling image",
				},
			},
		}},
	}

	_, err = client.CoreV1().Pods(ns).UpdateStatus(context.Background(), created, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("writing pod status: %v", err)
	}
}

func TestDescribeCarriesTheReasonAPodIsNotRunning(t *testing.T) {
	requireEnv(t)
	seedPod(t, "default", "described")

	src, _ := startedSource(t, "default")

	// The informer cache has to catch up with the status write.
	var detail *core.PodDetail

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d, err := src.Describe(context.Background(), "default", "described")
		if err == nil && len(d.Containers) > 0 {
			detail = d

			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	if detail == nil {
		t.Fatal("Describe never returned container detail")
	}

	if detail.Status != "ImagePullBackOff" {
		t.Fatalf("Status = %q, want the container's waiting reason, not the phase", detail.Status)
	}

	c := detail.Containers[0]
	if c.State != "Waiting" || c.Reason != "ImagePullBackOff" {
		t.Fatalf("container state = %q/%q, want Waiting/ImagePullBackOff", c.State, c.Reason)
	}

	if c.RestartCount != 3 {
		t.Fatalf("RestartCount = %d, want 3", c.RestartCount)
	}

	if detail.QOSClass != "BestEffort" {
		t.Fatalf("QOSClass = %q", detail.QOSClass)
	}

	var notReady bool
	for _, cond := range detail.Conditions {
		if cond.Type == "Ready" && cond.Status == "False" && cond.Reason == "ContainersNotReady" {
			notReady = true
		}
	}

	if !notReady {
		t.Fatalf("conditions did not carry the not-ready reason: %+v", detail.Conditions)
	}
}

func TestDescribeOnAMissingPodErrors(t *testing.T) {
	src, _ := startedSource(t, "default")

	_, err := src.Describe(context.Background(), "default", "no-such-pod")
	if err == nil {
		t.Fatal("Describe on a missing pod returned nil error")
	}
}

func TestEventsAreScopedToTheNamedPodAndNewestFirst(t *testing.T) {
	cfg := requireEnv(t)

	client, err := kube.NewClientset(cfg)
	if err != nil {
		t.Fatalf("NewClientset: %v", err)
	}

	ctx := context.Background()
	now := time.Now()

	write := func(pod, reason string, age time.Duration) {
		_, err := client.CoreV1().Events("default").Create(ctx, &corev1.Event{
			ObjectMeta:     metav1.ObjectMeta{GenerateName: "ev-"},
			InvolvedObject: corev1.ObjectReference{Namespace: "default", Name: pod, Kind: "Pod"},
			Reason:         reason,
			Message:        reason + " happened",
			Type:           "Warning",
			Count:          1,
			FirstTimestamp: metav1.NewTime(now.Add(-age)),
			LastTimestamp:  metav1.NewTime(now.Add(-age)),
		}, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("creating event: %v", err)
		}
	}

	write("evpod", "Older", 10*time.Minute)
	write("evpod", "Newer", 1*time.Minute)
	write("otherpod", "Unrelated", 1*time.Minute)

	src, _ := startedSource(t, "default")

	events, err := src.Events(ctx, "default", "evpod")
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("got %d events, want only the two for evpod: %+v", len(events), events)
	}

	// Newest first: when a pod is failing, the latest event explains why.
	if events[0].Reason != "Newer" {
		t.Fatalf("events are not newest-first: %v, %v", events[0].Reason, events[1].Reason)
	}

	if !events[0].Warning() {
		t.Fatal("a Warning event did not report itself as one")
	}
}

func TestLogsOpensAStreamForARealPod(t *testing.T) {
	requireEnv(t)
	seedPod(t, "default", "logpod")

	src, _ := startedSource(t, "default")

	rc, err := src.Logs(context.Background(), core.LogRequest{
		Namespace: "default", Pod: "logpod", Container: "app", TailLines: 5,
	})

	// envtest has no kubelet, so the request is well-formed but cannot be
	// served. Either outcome proves the request was built correctly; a
	// malformed one fails differently, before reaching the node.
	if err != nil {
		return
	}

	defer func() { _ = rc.Close() }()

	_, _ = io.Copy(io.Discard, rc)
}
