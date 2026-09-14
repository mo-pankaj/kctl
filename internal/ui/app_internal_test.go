package ui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/core/mocks"
)

// TestViewRendersHelpFromKeymap rebinds the quit key to a distinctive value
// and checks the rendered help line follows it. This is an internal test
// (package ui, not ui_test) because it needs to reach the unexported keys
// field directly; New does not expose a way to override the keymap, and
// adding one just to serve this test isn't warranted.
func TestViewRendersHelpFromKeymap(t *testing.T) {
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

	m := New(zap.NewNop(), contexts, pods, core.Selector{Namespace: "trading-service"})
	m.keys.Quit = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "exit"))

	view := m.View().Content

	if !strings.Contains(view, "x:exit") {
		t.Fatalf("View() = %q, want it to contain the rebound help %q", view, "x:exit")
	}

	if strings.Contains(view, "q:quit") {
		t.Fatalf("View() = %q, want it not to contain the stale default help %q", view, "q:quit")
	}
}
