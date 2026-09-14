package describe_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/core/mocks"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
	"github.com/mo-pankaj/kctl/internal/ui/views/describe"
)

func brokenDetail() *core.PodDetail {
	exit := int32(137)

	return &core.PodDetail{
		Pod: core.Pod{
			Namespace: "ns1", Name: "brokenpod", Status: "ImagePullBackOff",
			Node: "node-a", ReadyCount: 0, TotalCount: 1,
			CreatedAt: time.Now().Add(-3 * time.Minute),
		},
		QOSClass: "BestEffort",
		Containers: []core.ContainerState{{
			Name: "app", Image: "nginx:nope", Ready: false,
			State: "Waiting", Reason: "ErrImagePull", Message: "failed to pull",
		}},
		InitContainers: []core.ContainerState{{
			Name: "migrate", State: "Terminated", Reason: "Error", ExitCode: &exit,
		}},
		Conditions: []core.PodCondition{
			{Type: "Ready", Status: "False", Reason: "ContainersNotReady"},
		},
	}
}

func render(t *testing.T, setup func(*mocks.MockPodDescriber)) string {
	t.Helper()

	ctrl := gomock.NewController(t)
	d := mocks.NewMockPodDescriber(ctrl)
	setup(d)

	m := describe.New(zap.NewNop(), theme.New(), keys.Default(), d, "ns1", "brokenpod")

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model, _ = model.Update(model.Init()())

	return model.View().Content
}

func TestDescribeShowsWhyTheContainerIsNotRunning(t *testing.T) {
	out := render(t, func(d *mocks.MockPodDescriber) {
		d.EXPECT().Describe(gomock.Any(), "ns1", "brokenpod").Return(brokenDetail(), nil)
		d.EXPECT().Events(gomock.Any(), "ns1", "brokenpod").Return([]core.Event{{
			Type: "Warning", Reason: "Failed", Message: "Error: ErrImagePull", Count: 4,
			Last: time.Now().Add(-10 * time.Second),
		}}, nil)
	})

	for _, want := range []string{"ImagePullBackOff", "ErrImagePull", "nginx:nope", "ContainersNotReady", "Failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("describe output missing %q\n%s", want, out)
		}
	}
}

func TestDescribeShowsInitContainerExitCode(t *testing.T) {
	out := render(t, func(d *mocks.MockPodDescriber) {
		d.EXPECT().Describe(gomock.Any(), gomock.Any(), gomock.Any()).Return(brokenDetail(), nil)
		d.EXPECT().Events(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
	})

	// A failed init container's exit code is often the only clue; it must survive
	// into the rendered output rather than being flattened to "Terminated".
	if !strings.Contains(out, "exit=137") {
		t.Fatalf("init container exit code missing from output\n%s", out)
	}
}

func TestEventsFailureStillShowsTheDetail(t *testing.T) {
	out := render(t, func(d *mocks.MockPodDescriber) {
		d.EXPECT().Describe(gomock.Any(), gomock.Any(), gomock.Any()).Return(brokenDetail(), nil)
		// RBAC commonly allows reading pods but forbids events.
		d.EXPECT().Events(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("error-listing-events :forbidden"))
	})

	if !strings.Contains(out, "ImagePullBackOff") {
		t.Fatalf("an events failure hid the pod detail entirely\n%s", out)
	}

	if !strings.Contains(out, "EVENTS (0)") {
		t.Fatalf("expected an empty events section, got\n%s", out)
	}
}

func TestDescribeFailureRendersRetryableError(t *testing.T) {
	out := render(t, func(d *mocks.MockPodDescriber) {
		d.EXPECT().Describe(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("error-getting-pod :not found"))
	})

	if !strings.Contains(out, "not found") {
		t.Fatalf("error not surfaced\n%s", out)
	}

	if !strings.Contains(out, "r:retry") {
		t.Fatalf("error state should offer retry\n%s", out)
	}
}

var _ = context.Background
