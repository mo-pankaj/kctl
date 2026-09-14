package kube_test

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/mo-pankaj/kctl/internal/kube"
)

func TestPodStatus(t *testing.T) {
	deleting := metav1.NewTime(time.Now())

	tests := []struct {
		name string
		pod  *corev1.Pod
		want string
	}{
		{
			name: "running",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
					},
				},
			},
			want: "Running",
		},
		{
			name: "deletion timestamp wins over phase",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deleting},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
			want: "Terminating",
		},
		{
			name: "waiting reason surfaces over phase",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}},
					},
				},
			},
			want: "ImagePullBackOff",
		},
		{
			name: "crash loop",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
					},
				},
			},
			want: "CrashLoopBackOff",
		},
		{
			name: "completed job pod",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
					ContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed", ExitCode: 0}}},
					},
				},
			},
			want: "Completed",
		},
		{
			name: "init container still waiting",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: []corev1.Container{{Name: "migrate"}, {Name: "seed"}}},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}},
					},
				},
			},
			want: "Init:0/2",
		},
		{
			name: "init container failed",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{InitContainers: []corev1.Container{{Name: "migrate"}}},
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{
						{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}}},
					},
				},
			},
			want: "Init:Error",
		},
		{
			name: "pod level reason wins over phase",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{Phase: corev1.PodFailed, Reason: "Evicted"},
			},
			want: "Evicted",
		},
		{
			name: "bare pending with no container statuses",
			pod:  &corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}},
			want: "Pending",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := kube.PodStatus(tt.pod)

			if got != tt.want {
				t.Fatalf("PodStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToDomainPodCountsReadyAndRestarts(t *testing.T) {
	created := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "api-7d9f-2xk9",
			Namespace:         "trading-service",
			CreationTimestamp: metav1.NewTime(created),
		},
		Spec: corev1.PodSpec{
			NodeName:   "ip-10-2-3-4",
			Containers: []corev1.Container{{Name: "api"}, {Name: "sidecar"}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{Name: "api", Ready: true, RestartCount: 2, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				{Name: "sidecar", Ready: false, RestartCount: 3, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
			},
		},
	}

	got := kube.ToDomainPod(pod)

	if got.Name != "api-7d9f-2xk9" || got.Namespace != "trading-service" {
		t.Fatalf("identity not carried: %+v", got)
	}

	if got.ReadyCount != 1 || got.TotalCount != 2 {
		t.Fatalf("Ready() = %q, want %q", got.Ready(), "1/2")
	}

	if got.Restarts != 5 {
		t.Fatalf("Restarts = %d, want 5 (sum across containers)", got.Restarts)
	}

	if got.Node != "ip-10-2-3-4" {
		t.Fatalf("Node = %q, want %q", got.Node, "ip-10-2-3-4")
	}

	if !got.CreatedAt.Equal(created) {
		t.Fatalf("CreatedAt = %v, want %v", got.CreatedAt, created)
	}

	if len(got.Containers) != 2 || got.Containers[0] != "api" {
		t.Fatalf("Containers = %v, want [api sidecar]", got.Containers)
	}
}
