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
