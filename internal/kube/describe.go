package kube

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	"github.com/mo-pankaj/kctl/internal/core"
)

// Describe returns the full detail behind one pod, read from the informer cache.
func (s *PodSource) Describe(ctx context.Context, ns, name string) (detail *core.PodDetail, err error) {
	logger := s.logger.With(zap.String("method", "Describe"), zap.String("namespace", ns), zap.String("pod", name))

	found, err := s.lister.Pods(ns).Get(name)
	if err != nil {
		err = fmt.Errorf("error-getting-pod :%w", err)
		logger.Error("error-getting-pod", zap.Error(err))

		return detail, err
	}

	detail = toDomainDetail(found)

	return detail, err
}

// Events returns the events concerning one pod, newest first.
//
// This is a live API call rather than a cached read. Events are only requested
// while a describe view is open, and the reason a pod will not start is exactly
// the thing that must not be stale.
func (s *PodSource) Events(ctx context.Context, ns, name string) (events []core.Event, err error) {
	logger := s.logger.With(zap.String("method", "Events"), zap.String("namespace", ns), zap.String("pod", name))

	selector := fields.Set{"involvedObject.name": name, "involvedObject.namespace": ns}.AsSelector().String()

	list, err := s.client.CoreV1().Events(ns).List(ctx, metav1.ListOptions{FieldSelector: selector})
	if err != nil {
		err = fmt.Errorf("error-listing-events :%w", err)
		logger.Error("error-listing-events", zap.Error(err))

		return events, err
	}

	for _, e := range list.Items {
		events = append(events, core.Event{
			Type:    e.Type,
			Reason:  e.Reason,
			Message: e.Message,
			Count:   e.Count,
			First:   e.FirstTimestamp.Time,
			Last:    lastSeen(e),
		})
	}

	// Newest first: when a pod is failing, the most recent event is the one
	// that explains why.
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].Last.After(events[j].Last)
	})

	return events, err
}

// lastSeen prefers the event series time, falling back through LastTimestamp to
// the creation time — any of the three may be zero depending on how the event
// was recorded.
func lastSeen(e corev1.Event) (t time.Time) {
	switch {
	case e.Series != nil && !e.Series.LastObservedTime.IsZero():
		return e.Series.LastObservedTime.Time

	case !e.LastTimestamp.IsZero():
		return e.LastTimestamp.Time

	case !e.EventTime.IsZero():
		return e.EventTime.Time

	default:
		return e.CreationTimestamp.Time
	}
}

func toDomainDetail(p *corev1.Pod) (d *core.PodDetail) {
	d = &core.PodDetail{
		Pod:            ToDomainPod(p),
		QOSClass:       string(p.Status.QOSClass),
		ServiceAccount: p.Spec.ServiceAccountName,
		PodIP:          p.Status.PodIP,
		HostIP:         p.Status.HostIP,
		Labels:         p.Labels,
	}

	if len(p.OwnerReferences) > 0 {
		d.OwnerKind = p.OwnerReferences[0].Kind
		d.OwnerName = p.OwnerReferences[0].Name
	}

	for _, c := range p.Status.Conditions {
		d.Conditions = append(d.Conditions, core.PodCondition{
			Type:           string(c.Type),
			Status:         string(c.Status),
			Reason:         c.Reason,
			Message:        c.Message,
			LastTransition: c.LastTransitionTime.Time,
		})
	}

	images := map[string]string{}
	for _, c := range p.Spec.Containers {
		images[c.Name] = c.Image
	}

	for _, c := range p.Spec.InitContainers {
		images[c.Name] = c.Image
	}

	for _, cs := range p.Status.ContainerStatuses {
		d.Containers = append(d.Containers, toContainerState(cs, images[cs.Name]))
	}

	for _, cs := range p.Status.InitContainerStatuses {
		d.InitContainers = append(d.InitContainers, toContainerState(cs, images[cs.Name]))
	}

	for _, v := range p.Spec.Volumes {
		d.Volumes = append(d.Volumes, v.Name)
	}

	return d
}

// toContainerState flattens a container status into the one state that is
// currently true, carrying the reason and exit code that explain it.
func toContainerState(cs corev1.ContainerStatus, image string) (state core.ContainerState) {
	state = core.ContainerState{
		Name:         cs.Name,
		Image:        image,
		Ready:        cs.Ready,
		RestartCount: cs.RestartCount,
	}

	if state.Image == "" {
		state.Image = cs.Image
	}

	switch {
	case cs.State.Running != nil:
		state.State = "Running"
		state.Started = cs.State.Running.StartedAt.Time

	case cs.State.Waiting != nil:
		state.State = "Waiting"
		state.Reason = cs.State.Waiting.Reason
		state.Message = cs.State.Waiting.Message

	case cs.State.Terminated != nil:
		code := cs.State.Terminated.ExitCode
		state.State = "Terminated"
		state.Reason = cs.State.Terminated.Reason
		state.Message = cs.State.Terminated.Message
		state.ExitCode = &code
		state.Started = cs.State.Terminated.StartedAt.Time

	default:
		state.State = "Unknown"
	}

	return state
}
