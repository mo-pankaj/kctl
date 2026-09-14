package kube

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/mo-pankaj/kctl/internal/core"
)

// PodStatus derives the single status word kctl shows for a pod.
//
// This mirrors the precedence kubectl uses: deletion beats everything, then
// init-container trouble, then any container's waiting or terminated reason,
// then a pod-level reason, and finally the raw phase. Showing the raw phase
// alone would render every broken pod as "Pending" or "Running", which is
// exactly the case this tool exists to diagnose.
func PodStatus(p *corev1.Pod) (status string) {
	if p.DeletionTimestamp != nil {
		status = "Terminating"
		return status
	}

	status, initialised := initStatus(p)
	if !initialised {
		return status
	}

	status = string(p.Status.Phase)
	if p.Status.Reason != "" {
		status = p.Status.Reason
	}

	for _, cs := range p.Status.ContainerStatuses {
		switch {
		case cs.State.Waiting != nil && cs.State.Waiting.Reason != "":
			status = cs.State.Waiting.Reason
			return status

		case cs.State.Terminated != nil && cs.State.Terminated.Reason != "":
			status = cs.State.Terminated.Reason
			return status
		}
	}

	return status
}

// initStatus reports the init-container status, and whether initialisation has
// finished. When it has not, the returned status describes why.
func initStatus(p *corev1.Pod) (status string, done bool) {
	total := len(p.Spec.InitContainers)

	for i, ics := range p.Status.InitContainerStatuses {
		switch {
		case ics.State.Terminated != nil && ics.State.Terminated.ExitCode == 0:
			continue

		case ics.State.Terminated != nil && ics.State.Terminated.Reason != "":
			status = "Init:" + ics.State.Terminated.Reason
			return status, done

		case ics.State.Terminated != nil:
			status = fmt.Sprintf("Init:ExitCode:%d", ics.State.Terminated.ExitCode)
			return status, done

		case ics.State.Waiting != nil && ics.State.Waiting.Reason != "" && ics.State.Waiting.Reason != "PodInitializing":
			status = "Init:" + ics.State.Waiting.Reason
			return status, done

		default:
			status = fmt.Sprintf("Init:%d/%d", i, total)
			return status, done
		}
	}

	done = true
	return status, done
}

// ToDomainPod projects a Kubernetes pod into the shape kctl renders.
func ToDomainPod(p *corev1.Pod) (pod core.Pod) {
	pod = core.Pod{
		Namespace:  p.Namespace,
		Name:       p.Name,
		Status:     PodStatus(p),
		Node:       p.Spec.NodeName,
		TotalCount: len(p.Status.ContainerStatuses),
		CreatedAt:  p.CreationTimestamp.Time,
	}

	for _, cs := range p.Status.ContainerStatuses {
		if cs.Ready {
			pod.ReadyCount++
		}

		pod.Restarts += cs.RestartCount
	}

	for _, c := range p.Spec.Containers {
		pod.Containers = append(pod.Containers, c.Name)
	}

	return pod
}

// ToDomainPods projects a slice of pods.
func ToDomainPods(items []*corev1.Pod) (pods []core.Pod) {
	pods = make([]core.Pod, 0, len(items))

	for _, p := range items {
		pods = append(pods, ToDomainPod(p))
	}

	return pods
}
