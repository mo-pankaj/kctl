package ui_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/ui"
)

type stubReader struct{ core.PodReader }
type stubLister struct{ core.NamespaceLister }

func TestSessionSwitchCancelsThePreviousSource(t *testing.T) {
	cancelled := make(chan string, 4)

	factory := func(ctx context.Context, contextName, namespace string) (core.PodReader, core.NamespaceLister, error) {
		go func() {
			<-ctx.Done()
			cancelled <- contextName
		}()

		return stubReader{}, stubLister{}, nil
	}

	session := ui.NewSession(zap.NewNop(), factory)

	_, err := session.Switch(context.Background(), "dev-01", "ns1")
	if err != nil {
		t.Fatalf("first Switch returned error: %v", err)
	}

	_, err = session.Switch(context.Background(), "cnc-prod", "ns2")
	if err != nil {
		t.Fatalf("second Switch returned error: %v", err)
	}

	select {
	case name := <-cancelled:
		if name != "dev-01" {
			t.Fatalf("cancelled context %q, want the previous one (dev-01)", name)
		}

	case <-context.Background().Done():
		t.Fatal("unreachable")
	}
}

func TestSessionGenerationIncrementsPerSwitch(t *testing.T) {
	factory := func(context.Context, string, string) (core.PodReader, core.NamespaceLister, error) {
		return stubReader{}, stubLister{}, nil
	}

	session := ui.NewSession(zap.NewNop(), factory)

	first, err := session.Switch(context.Background(), "dev-01", "ns1")
	if err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}

	second, err := session.Switch(context.Background(), "dev-02", "ns1")
	if err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}

	if second.Generation <= first.Generation {
		t.Fatalf("generation did not advance: first=%d second=%d", first.Generation, second.Generation)
	}

	if !session.IsCurrent(second.Generation) {
		t.Fatal("IsCurrent(second) = false, want true")
	}

	// This is the property that stops a late poke from the old cluster
	// rendering into the new one.
	if session.IsCurrent(first.Generation) {
		t.Fatal("IsCurrent(first) = true after a switch; stale generations must be rejected")
	}
}

func TestFailedSwitchLeavesThePreviousSourceActive(t *testing.T) {
	calls := 0

	factory := func(context.Context, string, string) (core.PodReader, core.NamespaceLister, error) {
		calls++
		if calls == 2 {
			return nil, nil, errors.New("error-unreachable-cluster :boom")
		}

		return stubReader{}, stubLister{}, nil
	}

	session := ui.NewSession(zap.NewNop(), factory)

	good, err := session.Switch(context.Background(), "dev-01", "ns1")
	if err != nil {
		t.Fatalf("first Switch returned error: %v", err)
	}

	_, err = session.Switch(context.Background(), "cnc-prod", "ns2")
	if err == nil {
		t.Fatal("Switch to an unreachable cluster returned nil error, want an error")
	}

	if !session.IsCurrent(good.Generation) {
		t.Fatal("a failed switch invalidated the working source; the user must be left on the previous context")
	}
}
