package ctxpicker_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/core/mocks"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
	"github.com/mo-pankaj/kctl/internal/ui/views/ctxpicker"
)

func fixtureContexts() []core.ContextInfo {
	return []core.ContextInfo{
		{Name: "cnc-prod", Cluster: "zulu", Server: "https://api.zulu.example.com", Namespace: "cnc-prod"},
		{Name: "dev-01", Cluster: "charlie", Server: "https://api.charlie.example.com", Namespace: "dev-01", Current: true},
		{Name: "dev-trading", Cluster: "charlie", Server: "https://api.charlie.example.com", Namespace: "trading-service"},
	}
}

func newPicker(t *testing.T, setup func(*mocks.MockContextManager)) ctxpicker.Model {
	t.Helper()

	ctrl := gomock.NewController(t)
	cm := mocks.NewMockContextManager(ctrl)
	cm.EXPECT().Contexts().Return(fixtureContexts()).AnyTimes()
	cm.EXPECT().Current().Return(fixtureContexts()[1]).AnyTimes()

	if setup != nil {
		setup(cm)
	}

	return ctxpicker.New(zap.NewNop(), theme.New(), keys.Default(), cm)
}

func TestPickerListsEveryContextWithItsCluster(t *testing.T) {
	tm := teatest.NewTestModel(t, newPicker(t, nil), teatest.WithInitialTermSize(140, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("cnc-prod")) &&
			bytes.Contains(b, []byte("dev-trading")) &&
			bytes.Contains(b, []byte("zulu"))
	}, teatest.WithDuration(3*time.Second))

	tm.Quit()
}

func TestEnterSwitchesToSelectedContext(t *testing.T) {
	switched := make(chan string, 1)

	m := newPicker(t, func(cm *mocks.MockContextManager) {
		cm.EXPECT().
			Use(gomock.Any(), "cnc-prod").
			DoAndReturn(func(_ context.Context, name string) error {
				switched <- name
				return nil
			})
	})

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(140, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("cnc-prod"))
	}, teatest.WithDuration(3*time.Second))

	// The table starts on the first row, which is cnc-prod (sorted).
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	select {
	case name := <-switched:
		if name != "cnc-prod" {
			t.Fatalf("Use called with %q, want %q", name, "cnc-prod")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Use was never called after pressing enter")
	}

	tm.Quit()
}

func TestFailedSwitchSurfacesErrorAndDoesNotCrash(t *testing.T) {
	m := newPicker(t, func(cm *mocks.MockContextManager) {
		cm.EXPECT().
			Use(gomock.Any(), gomock.Any()).
			Return(errors.New("error-unknown-context :boom"))
	})

	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(140, 30))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("cnc-prod"))
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("error-unknown-context"))
	}, teatest.WithDuration(3*time.Second))

	tm.Quit()
}
