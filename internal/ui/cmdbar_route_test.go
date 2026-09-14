package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/core/mocks"
	"github.com/mo-pankaj/kctl/internal/ui/cmdbar"
)

func rootWithMocks(t *testing.T) Model {
	t.Helper()

	ctrl := gomock.NewController(t)

	contexts := mocks.NewMockContextManager(ctrl)
	contexts.EXPECT().Current().
		Return(core.ContextInfo{Name: "kind-dev", Namespace: "default", Current: true}).AnyTimes()
	contexts.EXPECT().Contexts().
		Return([]core.ContextInfo{{Name: "kind-dev", Namespace: "default", Current: true}}).AnyTimes()

	pods := mocks.NewMockPodReader(ctrl)
	pods.EXPECT().Pods(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	pods.EXPECT().Subscribe(gomock.Any(), gomock.Any()).
		Return((<-chan struct{})(make(chan struct{})), nil).AnyTimes()

	return New(zap.NewNop(), contexts, pods, core.Selector{Namespace: "default"})
}

func typeInto(t *testing.T, m tea.Model, s string) tea.Model {
	t.Helper()

	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	return m
}

func TestColonOpensTheCommandBarAndQTypesIntoIt(t *testing.T) {
	var m tea.Model = rootWithMocks(t)

	m, _ = m.Update(tea.KeyPressMsg{Code: ':', Text: ":"})

	// "q" must reach the command bar, not quit. If routing were wrong the
	// model would have returned tea.Quit instead of accepting the rune.
	m = typeInto(t, m, "ns q")

	if !strings.Contains(m.View().Content, "ns q") {
		t.Fatalf("command bar did not receive the typed text; view = %q", m.View().Content)
	}
}

func TestColonNsRescopesTheSelectorAndStatusBar(t *testing.T) {
	var m tea.Model = rootWithMocks(t)

	m, _ = m.Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	m = typeInto(t, m, "ns kube-system")
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("submitting the command produced no command")
	}

	m, _ = m.Update(cmd())

	root, ok := m.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", m)
	}

	if root.sel.Namespace != "kube-system" {
		t.Fatalf("sel.Namespace = %q, want %q", root.sel.Namespace, "kube-system")
	}

	if !strings.Contains(root.View().Content, "kube-system") {
		t.Fatal("status bar does not show the new namespace")
	}
}

func TestColonNsAllMeansEveryNamespace(t *testing.T) {
	var m tea.Model = rootWithMocks(t)

	m, _ = m.Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	m = typeInto(t, m, "ns all")
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(cmd())

	root := m.(Model)

	if root.sel.Namespace != "" {
		t.Fatalf("sel.Namespace = %q, want empty (all namespaces)", root.sel.Namespace)
	}

	if !strings.Contains(root.View().Content, "all namespaces") {
		t.Fatal("status bar should read 'all namespaces'")
	}
}

func TestUnknownCommandShowsAnErrorAndDoesNotCrash(t *testing.T) {
	var m tea.Model = rootWithMocks(t)

	m, _ = m.Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	m = typeInto(t, m, "frobnicate")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !strings.Contains(m.View().Content, "error-unknown-command") {
		t.Fatalf("expected an inline error for an unknown verb; view = %q", m.View().Content)
	}

	_ = cmdbar.VerbNamespace
}

func TestStatusBarFollowsTheSelectorNotTheContextDefault(t *testing.T) {
	ctrl := gomock.NewController(t)

	contexts := mocks.NewMockContextManager(ctrl)
	contexts.EXPECT().Current().
		Return(core.ContextInfo{Name: "kind-dev", Namespace: "default", Current: true}).AnyTimes()
	contexts.EXPECT().Contexts().Return(nil).AnyTimes()

	pods := mocks.NewMockPodReader(ctrl)
	pods.EXPECT().Pods(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	pods.EXPECT().Subscribe(gomock.Any(), gomock.Any()).
		Return((<-chan struct{})(make(chan struct{})), nil).AnyTimes()

	// An all-namespaces selector, while the context still defaults to "default".
	m := New(zap.NewNop(), contexts, pods, core.Selector{})

	view := m.View().Content

	if !strings.Contains(view, "all namespaces") {
		t.Fatalf("status bar should read 'all namespaces' for an empty selector; view = %q", view)
	}

	if strings.Contains(view, "ns default") {
		t.Fatal("status bar reported the context's default namespace instead of the selector in use")
	}
}

func TestPushedViewsReceiveTheCurrentSize(t *testing.T) {
	var m tea.Model = rootWithMocks(t)

	// Startup size, as Bubble Tea delivers it once.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Open the context picker, which is pushed AFTER that size arrived.
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if cmd != nil {
		if msg := cmd(); msg != nil {
			m, _ = m.Update(msg)
		}
	}

	// A view pushed without being handed the size renders an empty body: its
	// viewport or table has zero height. That looked exactly like the key not
	// working at all.
	body := m.View().Content
	if !strings.Contains(body, "kind-dev") {
		t.Fatalf("pushed view rendered no content — it was never given the window size\n%s", body)
	}
}

func TestCtrlCQuitsEvenWhileTyping(t *testing.T) {
	var m tea.Model = rootWithMocks(t)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	// Open the command bar so an input owns the keyboard.
	m, _ = m.Update(tea.KeyPressMsg{Code: ':', Text: ":"})
	m = typeInto(t, m, "ns kube")

	// ctrl+c must still quit. Suppressing it along with the other global
	// bindings left no way out of a focused input at all.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c produced no command while an input was focused")
	}

	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("ctrl+c produced %T, want tea.QuitMsg", cmd())
	}
}
