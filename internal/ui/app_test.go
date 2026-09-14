package ui_test

import (
	"bytes"
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

	return ui.New(zap.NewNop(), contexts)
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
