package kube_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applyconfigv1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/kube"
)

// applier builds an Applier and a clientset against the test API server.
func applier(t *testing.T) (*kube.Applier, kubernetes.Interface) {
	t.Helper()

	cfg := requireEnv(t)

	a, err := kube.NewApplier(zap.NewNop(), cfg)
	if err != nil {
		t.Fatalf("NewApplier: %v", err)
	}

	client, err := kube.NewClientset(cfg)
	if err != nil {
		t.Fatalf("NewClientset: %v", err)
	}

	return a, client
}

// cmYAML builds a ConfigMap manifest with the given data pairs.
func cmYAML(name string, pairs ...string) []byte {
	var b strings.Builder

	fmt.Fprintf(&b, "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: %s\ndata:\n", name)

	for i := 0; i+1 < len(pairs); i += 2 {
		fmt.Fprintf(&b, "  %s: %q\n", pairs[i], pairs[i+1])
	}

	return []byte(b.String())
}

func parse(t *testing.T, y []byte) []core.Manifest {
	t.Helper()

	docs, err := kube.ParseManifestBytes(y, "default")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	return docs
}

func TestDryRunOnANewObjectReportsCreationAndWritesNothing(t *testing.T) {
	a, client := applier(t)
	ctx := context.Background()

	docs := parse(t, cmYAML("dryrun-create", "colour", "blue"))

	results, err := a.DryRun(ctx, docs)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}

	r := results[0]
	if r.Err != nil {
		t.Fatalf("dry run errored: %v", r.Err)
	}

	if !r.Creation {
		t.Fatal("Creation = false for an object that does not exist")
	}

	if r.Added == 0 {
		t.Fatalf("Added = 0; a creation should show the object as added\n%s", r.Unified)
	}

	if !strings.Contains(r.Unified, "colour") {
		t.Fatalf("diff does not mention the new key:\n%s", r.Unified)
	}

	// The property that matters most: a dry run must not create anything.
	_, getErr := client.CoreV1().ConfigMaps("default").Get(ctx, "dryrun-create", metav1.GetOptions{})
	if getErr == nil {
		t.Fatal("the dry run CREATED the object; it must never write")
	}
}

func TestDryRunOnAnUnchangedObjectReportsNoChange(t *testing.T) {
	a, _ := applier(t)
	ctx := context.Background()

	docs := parse(t, cmYAML("unchanged", "k", "v"))

	_, err := a.Apply(ctx, docs, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	results, err := a.DryRun(ctx, docs)
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}

	if results[0].Changed() {
		t.Fatalf("re-applying an identical manifest reported a change:\n%s", results[0].Unified)
	}
}

func TestDryRunOnAChangedObjectShowsBothSides(t *testing.T) {
	a, _ := applier(t)
	ctx := context.Background()

	_, err := a.Apply(ctx, parse(t, cmYAML("changed", "stage", "one")), false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	results, err := a.DryRun(ctx, parse(t, cmYAML("changed", "stage", "two")))
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}

	r := results[0]
	if r.Creation {
		t.Fatal("Creation = true for an object that already exists")
	}

	if r.Added == 0 || r.Removed == 0 {
		t.Fatalf("a value change should add and remove a line, got +%d/-%d:\n%s", r.Added, r.Removed, r.Unified)
	}

	if !strings.Contains(r.Unified, "-  stage: one") || !strings.Contains(r.Unified, "+  stage: two") {
		t.Fatalf("diff does not show the value moving:\n%s", r.Unified)
	}
}

func TestDiffOmitsServerBookkeeping(t *testing.T) {
	a, _ := applier(t)
	ctx := context.Background()

	_, err := a.Apply(ctx, parse(t, cmYAML("noisy", "a", "1")), false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	results, _ := a.DryRun(ctx, parse(t, cmYAML("noisy", "a", "2")))

	// These change on every write and would drown the real change.
	for _, noise := range []string{"managedFields", "resourceVersion", "creationTimestamp", "uid"} {
		if strings.Contains(results[0].Unified, noise) {
			t.Fatalf("diff contains %q, which changes on every write:\n%s", noise, results[0].Unified)
		}
	}
}

func TestApplyWritesAndTakesOwnership(t *testing.T) {
	a, client := applier(t)
	ctx := context.Background()

	results, err := a.Apply(ctx, parse(t, cmYAML("owned", "k", "v")), false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if !results[0].Applied {
		t.Fatalf("Applied = false: %v", results[0].Err)
	}

	live, err := client.CoreV1().ConfigMaps("default").Get(ctx, "owned", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("object was not written: %v", err)
	}

	if live.Data["k"] != "v" {
		t.Fatalf("data = %v, want k=v", live.Data)
	}

	found := false
	for _, mf := range live.ManagedFields {
		if mf.Manager == "kctl" {
			found = true
		}
	}

	if !found {
		t.Fatalf("kctl did not take field ownership; managers = %v", live.ManagedFields)
	}
}

// applyAs writes the object under a different field manager, so the next apply
// meets a genuine ownership conflict rather than a simulated one.
func applyAs(t *testing.T, client kubernetes.Interface, manager, name string, data map[string]string) {
	t.Helper()

	_, err := client.CoreV1().ConfigMaps("default").Apply(
		context.Background(),
		applyconfigv1.ConfigMap(name, "default").WithData(data),
		metav1.ApplyOptions{FieldManager: manager},
	)
	if err != nil {
		t.Fatalf("seeding as %q: %v", manager, err)
	}
}

func TestDryRunSurfacesAConflictAsDataBeforeAnythingIsWritten(t *testing.T) {
	a, client := applier(t)
	ctx := context.Background()

	applyAs(t, client, "flux", "contested", map[string]string{"tier": "silver"})

	results, err := a.DryRun(ctx, parse(t, cmYAML("contested", "tier", "bronze")))
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}

	r := results[0]
	if !r.Conflict {
		t.Fatalf("Conflict = false when another manager owns the field; +%d/-%d err=%v", r.Added, r.Removed, r.Err)
	}

	// Naming the owner is the point: the user has to know who they would be
	// overriding before deciding to force.
	if len(r.Managers) == 0 || r.Managers[0] != "flux" {
		t.Fatalf("Managers = %v, want [flux]", r.Managers)
	}

	if len(r.Fields) == 0 || !strings.Contains(r.Fields[0], "tier") {
		t.Fatalf("Fields = %v, want the contested field named", r.Fields)
	}

	live, _ := client.CoreV1().ConfigMaps("default").Get(ctx, "contested", metav1.GetOptions{})
	if live.Data["tier"] != "silver" {
		t.Fatalf("the dry run MUTATED the object: tier = %q", live.Data["tier"])
	}
}

func TestApplyRefusesAConflictWithoutForce(t *testing.T) {
	a, client := applier(t)
	ctx := context.Background()

	applyAs(t, client, "flux", "refuse", map[string]string{"tier": "silver"})

	results, err := a.Apply(ctx, parse(t, cmYAML("refuse", "tier", "bronze")), false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	r := results[0]
	if r.Applied {
		t.Fatal("applied over another manager's field without force")
	}

	if !r.Conflict {
		t.Fatalf("Conflict = false; err = %v", r.Err)
	}

	live, _ := client.CoreV1().ConfigMaps("default").Get(ctx, "refuse", metav1.GetOptions{})
	if live.Data["tier"] != "silver" {
		t.Fatalf("the refused apply still changed the value to %q", live.Data["tier"])
	}
}

func TestForceAppliesAndTransfersOwnership(t *testing.T) {
	a, client := applier(t)
	ctx := context.Background()

	applyAs(t, client, "flux", "forced", map[string]string{"tier": "silver"})

	results, err := a.Apply(ctx, parse(t, cmYAML("forced", "tier", "bronze")), true)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if !results[0].Applied {
		t.Fatalf("force did not apply: conflict=%v err=%v", results[0].Conflict, results[0].Err)
	}

	live, _ := client.CoreV1().ConfigMaps("default").Get(ctx, "forced", metav1.GetOptions{})
	if live.Data["tier"] != "bronze" {
		t.Fatalf("tier = %q, want bronze", live.Data["tier"])
	}

	// Force does not merely change a value; it takes the field. That is why it
	// is a separate armed action in the UI.
	var kctlOwns bool
	for _, mf := range live.ManagedFields {
		if mf.Manager == "kctl" && mf.FieldsV1 != nil && strings.Contains(string(mf.FieldsV1.Raw), "tier") {
			kctlOwns = true
		}
	}

	if !kctlOwns {
		t.Fatalf("ownership of the contested field did not transfer; managers = %v", live.ManagedFields)
	}
}

func TestOneBadDocumentDoesNotStopTheOthers(t *testing.T) {
	a, client := applier(t)
	ctx := context.Background()

	// Second document targets a kind that does not exist.
	y := append(cmYAML("good-one", "a", "b"),
		[]byte("---\napiVersion: v1\nkind: NoSuchKind\nmetadata:\n  name: bad\n")...)

	docs := parse(t, y)

	results, err := a.Apply(ctx, docs, false)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("got %d results for 2 documents", len(results))
	}

	if !results[0].Applied {
		t.Fatalf("the valid document was not applied: %v", results[0].Err)
	}

	if results[1].Applied || results[1].Err == nil {
		t.Fatal("the invalid document should have failed with an error")
	}

	// Partial application must be observable, not silent.
	_, err = client.CoreV1().ConfigMaps("default").Get(ctx, "good-one", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("the valid document did not reach the cluster: %v", err)
	}
}
