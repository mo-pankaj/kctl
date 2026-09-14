package kube_test

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/kube"
)

func newStore(t *testing.T) *kube.ContextStore {
	t.Helper()

	store, err := kube.NewContextStore(zap.NewNop(), "testdata/kubeconfig.yaml")
	if err != nil {
		t.Fatalf("NewContextStore returned error: %v", err)
	}

	return store
}

func TestContextStoreImplementsPort(t *testing.T) {
	var _ core.ContextManager = newStore(t)
}

func TestContextsAreSortedAndCarryServerURL(t *testing.T) {
	store := newStore(t)

	got := store.Contexts()

	if len(got) != 3 {
		t.Fatalf("Contexts() returned %d contexts, want 3", len(got))
	}

	wantOrder := []string{"cnc-prod", "dev-01", "dev-trading"}
	for i, name := range wantOrder {
		if got[i].Name != name {
			t.Fatalf("Contexts()[%d].Name = %q, want %q (must be sorted by name)", i, got[i].Name, name)
		}
	}

	// The server URL is what the v0.3 risk guard keys on, so it must be carried
	// through from the cluster entry, not inferred from the context name.
	if got[0].Server != "https://api.zulu.example.com" {
		t.Fatalf("cnc-prod Server = %q, want the zulu cluster URL", got[0].Server)
	}

	if got[1].Namespace != "dev-01" {
		t.Fatalf("dev-01 Namespace = %q, want %q", got[1].Namespace, "dev-01")
	}
}

func TestContextWithoutNamespaceDefaultsToDefault(t *testing.T) {
	store := newStore(t)

	for _, c := range store.Contexts() {
		if c.Name != "cnc-prod" {
			continue
		}

		if c.Namespace != "default" {
			t.Fatalf("cnc-prod Namespace = %q, want %q", c.Namespace, "default")
		}

		return
	}

	t.Fatal("cnc-prod context not found")
}

func TestCurrentReflectsKubeconfigCurrentContext(t *testing.T) {
	store := newStore(t)

	got := store.Current()

	if got.Name != "dev-01" {
		t.Fatalf("Current().Name = %q, want %q", got.Name, "dev-01")
	}

	if !got.Current {
		t.Fatal("Current().Current = false, want true")
	}
}

func TestUseSwitchesActiveContext(t *testing.T) {
	store := newStore(t)

	err := store.Use(context.Background(), "cnc-prod")
	if err != nil {
		t.Fatalf("Use returned error: %v", err)
	}

	if store.Current().Name != "cnc-prod" {
		t.Fatalf("after Use, Current().Name = %q, want %q", store.Current().Name, "cnc-prod")
	}

	// Exactly one context may be marked Current.
	marked := 0
	for _, c := range store.Contexts() {
		if c.Current {
			marked++
		}
	}

	if marked != 1 {
		t.Fatalf("%d contexts marked Current, want exactly 1", marked)
	}
}

func TestUseRejectsUnknownContext(t *testing.T) {
	store := newStore(t)

	err := store.Use(context.Background(), "does-not-exist")
	if err == nil {
		t.Fatal("Use with an unknown context returned nil error, want an error")
	}

	if store.Current().Name != "dev-01" {
		t.Fatalf("a failed Use changed the active context to %q; it must be left alone", store.Current().Name)
	}
}

func TestRESTConfigTargetsTheNamedContextNotTheCurrentOne(t *testing.T) {
	store := newStore(t)

	cfg, err := store.RESTConfig("cnc-prod")
	if err != nil {
		t.Fatalf("RESTConfig returned error: %v", err)
	}

	if cfg.Host != "https://api.zulu.example.com" {
		t.Fatalf("RESTConfig(%q).Host = %q, want the zulu cluster URL", "cnc-prod", cfg.Host)
	}
}
