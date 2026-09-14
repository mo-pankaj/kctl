package ui_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/core/mocks"
	"github.com/mo-pankaj/kctl/internal/ui"
)

func newApp(t *testing.T) ui.Model {
	t.Helper()

	ctrl := gomock.NewController(t)
	contexts := mocks.NewMockContextManager(ctrl)

	contexts.EXPECT().Current().
		Return(core.ContextInfo{Name: "dev-01", Namespace: "trading-service", Current: true}).
		AnyTimes()

	contexts.EXPECT().Contexts().
		Return([]core.ContextInfo{{Name: "dev-01", Cluster: "charlie", Namespace: "trading-service", Current: true}}).
		AnyTimes()

	pods := mocks.NewMockPodReader(ctrl)
	pods.EXPECT().Pods(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	pods.EXPECT().
		Subscribe(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, core.Selector) (<-chan struct{}, error) {
			return make(chan struct{}), nil
		}).
		AnyTimes()

	return ui.New(zap.NewNop(), contexts, pods, core.Selector{Namespace: "trading-service"})
}

func TestAppStatusBarShowsCurrentContext(t *testing.T) {
	tm := teatest.NewTestModel(t, newApp(t), teatest.WithInitialTermSize(120, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("dev-01")) && bytes.Contains(b, []byte("trading-service"))
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestAppQuitsOnQ(t *testing.T) {
	tm := teatest.NewTestModel(t, newApp(t), teatest.WithInitialTermSize(120, 30))

	tm.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})

	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}
