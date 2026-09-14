package podlist_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest/v2"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/core/mocks"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
	"github.com/mo-pankaj/kctl/internal/ui/views/podlist"
)

func fixturePods() []core.Pod {
	now := time.Now()
	return []core.Pod{
		{Namespace: "ns1", Name: "api-7d9f-2xk9", Status: "Running", ReadyCount: 1, TotalCount: 1, CreatedAt: now.Add(-4 * 24 * time.Hour)},
		{Namespace: "ns1", Name: "worker-4k2", Status: "CrashLoopBackOff", ReadyCount: 0, TotalCount: 1, Restarts: 7, CreatedAt: now.Add(-time.Hour)},
	}
}

func newList(t *testing.T, pods []core.Pod, dirty chan struct{}) podlist.Model {
	t.Helper()

	ctrl := gomock.NewController(t)
	reader := mocks.NewMockPodReader(ctrl)

	reader.EXPECT().Pods(gomock.Any(), gomock.Any()).Return(pods, nil).AnyTimes()
	reader.EXPECT().
		Subscribe(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, core.Selector) (<-chan struct{}, error) {
			return dirty, nil
		}).
		AnyTimes()

	return podlist.New(zap.NewNop(), theme.New(), keys.Default(), reader, core.Selector{Namespace: "ns1"})
}

func TestListRendersPodsWithStatusAndRestarts(t *testing.T) {
	dirty := make(chan struct{}, 1)
	dirty <- struct{}{}

	tm := teatest.NewTestModel(t, newList(t, fixturePods(), dirty), teatest.WithInitialTermSize(140, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("api-7d9f-2xk9")) &&
			bytes.Contains(b, []byte("CrashLoopBackOff")) &&
			bytes.Contains(b, []byte("worker-4k2"))
	}, teatest.WithDuration(5*time.Second))

	tm.Quit()
}

func TestPokeTriggersReRead(t *testing.T) {
	dirty := make(chan struct{}, 1)

	ctrl := gomock.NewController(t)
	reader := mocks.NewMockPodReader(ctrl)

	calls := make(chan struct{}, 8)
	reader.EXPECT().
		Pods(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, core.Selector) ([]core.Pod, error) {
			select {
			case calls <- struct{}{}:
			default:
			}
			return fixturePods(), nil
		}).
		AnyTimes()

	reader.EXPECT().
		Subscribe(gomock.Any(), gomock.Any()).
		Return((<-chan struct{})(dirty), nil).
		AnyTimes()

	m := podlist.New(zap.NewNop(), theme.New(), keys.Default(), reader, core.Selector{Namespace: "ns1"})
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(140, 30))

	dirty <- struct{}{}

	select {
	case <-calls:
	case <-time.After(5 * time.Second):
		t.Fatal("a poke did not cause a cache re-read")
	}

	tm.Quit()
}

// NOTE: Enter's effect is verified in podlist_internal_test.go, not here.
// A teatest-level assertion cannot see SelectedMsg — podlist emits it for the
// ROOT model to consume, so nothing changes on podlist's own screen. Asserting
// on this view's output after Enter would pass whether or not the message was
// ever produced.

func TestFormatAge(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{name: "seconds", d: 45 * time.Second, want: "45s"},
		{name: "minutes", d: 5 * time.Minute, want: "5m"},
		{name: "hours", d: 3 * time.Hour, want: "3h"},
		{name: "days", d: 4 * 24 * time.Hour, want: "4d"},
		{name: "zero", d: 0, want: "0s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := podlist.FormatAge(tt.d)

			if got != tt.want {
				t.Fatalf("FormatAge(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}
