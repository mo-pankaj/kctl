package mocks_test

import (
	"context"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/core/mocks"
)

func TestMocksSatisfyPorts(t *testing.T) {
	ctrl := gomock.NewController(t)

	var (
		_ core.ContextManager  = mocks.NewMockContextManager(ctrl)
		_ core.NamespaceLister = mocks.NewMockNamespaceLister(ctrl)
		_ core.PodReader       = mocks.NewMockPodReader(ctrl)
	)
}

func TestMockPodReaderReturnsConfiguredPods(t *testing.T) {
	ctrl := gomock.NewController(t)
	reader := mocks.NewMockPodReader(ctrl)

	want := []core.Pod{{Namespace: "ns1", Name: "api-1", Status: "Running"}}

	reader.EXPECT().
		Pods(gomock.Any(), core.Selector{Namespace: "ns1"}).
		Return(want, nil)

	got, err := reader.Pods(context.Background(), core.Selector{Namespace: "ns1"})
	if err != nil {
		t.Fatalf("Pods returned error: %v", err)
	}

	if len(got) != 1 || got[0].Name != "api-1" {
		t.Fatalf("Pods() = %v, want %v", got, want)
	}
}
