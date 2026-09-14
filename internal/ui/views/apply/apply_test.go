package apply_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"go.uber.org/zap"

	"github.com/mo-pankaj/kctl/internal/core"
	"github.com/mo-pankaj/kctl/internal/theme"
	"github.com/mo-pankaj/kctl/internal/ui/keys"
	"github.com/mo-pankaj/kctl/internal/ui/views/apply"
)

type runner struct {
	diffs      []core.DiffResult
	applyCalls int
	lastForce  bool
}

func (r *runner) DryRun(context.Context, []core.Manifest) ([]core.DiffResult, error) {
	return r.diffs, nil
}

func (r *runner) Apply(_ context.Context, docs []core.Manifest, force bool) ([]core.ApplyResult, error) {
	r.applyCalls++
	r.lastForce = force

	out := make([]core.ApplyResult, 0, len(docs))
	for _, d := range docs {
		out = append(out, core.ApplyResult{Manifest: d, Applied: true})
	}

	return out, nil
}

func manifestFile(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "m.yaml")
	os.WriteFile(path, []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: cfg\n"), 0o600)

	return path
}

func newModel(t *testing.T, r *runner, target apply.Target) (tea.Model, string) {
	t.Helper()

	path := manifestFile(t)

	p := func(p, ns string) ([]core.Manifest, error) {
		return []core.Manifest{{Index: 0, Kind: "ConfigMap", Name: "cfg", Namespace: ns}}, nil
	}

	m := apply.New(zap.NewNop(), theme.New(), keys.Default(), r, p, target)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	return model, path
}

func typePath(t *testing.T, model tea.Model, path string) tea.Model {
	t.Helper()

	// The field is pre-seeded with the working directory, so clear it first
	// (ctrl+u is textinput's DeleteBeforeCursor).
	model, _ = model.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})

	for _, r := range path {
		model, _ = model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		model, _ = model.Update(cmd())
	}

	return model
}

func press(model tea.Model, r rune) (tea.Model, tea.Cmd) {
	return model.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
}

func TestNormalClusterAppliesOnY(t *testing.T) {
	r := &runner{diffs: []core.DiffResult{{
		Manifest: core.Manifest{Kind: "ConfigMap", Name: "cfg"}, Added: 2,
	}}}

	model, path := newModel(t, r, apply.Target{Context: "kind-dev", Namespace: "default"})
	model = typePath(t, model, path)

	model, _ = press(model, 'y') // diff -> confirm
	_, cmd := press(model, 'y')  // confirm -> apply

	if cmd == nil {
		t.Fatal("confirming on a normal cluster produced no apply command")
	}

	cmd()

	if r.applyCalls != 1 {
		t.Fatalf("Apply called %d times, want 1", r.applyCalls)
	}
}

func TestProtectedClusterRefusesTheWrongName(t *testing.T) {
	r := &runner{diffs: []core.DiffResult{{
		Manifest: core.Manifest{Kind: "ConfigMap", Name: "cfg"}, Added: 2,
	}}}

	model, path := newModel(t, r, apply.Target{Context: "cnc-prod", Namespace: "default", Protected: true})
	model = typePath(t, model, path)

	model, _ = press(model, 'y') // diff -> confirm

	// Type a plausible but wrong name.
	for _, ch := range "cnc-dev" {
		model, _ = press(model, ch)
	}

	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		cmd()
	}

	if r.applyCalls != 0 {
		t.Fatal("a protected cluster applied despite a mismatched context name")
	}

	if !strings.Contains(model.View().Content, "mismatch") {
		t.Fatalf("expected a mismatch error to be shown; view = %q", model.View().Content)
	}
}

func TestProtectedClusterAppliesOnExactName(t *testing.T) {
	r := &runner{diffs: []core.DiffResult{{
		Manifest: core.Manifest{Kind: "ConfigMap", Name: "cfg"}, Added: 2,
	}}}

	model, path := newModel(t, r, apply.Target{Context: "cnc-prod", Namespace: "default", Protected: true})
	model = typePath(t, model, path)
	model, _ = press(model, 'y')

	for _, ch := range "cnc-prod" {
		model, _ = press(model, ch)
	}

	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("typing the exact context name produced no apply command")
	}

	cmd()

	if r.applyCalls != 1 {
		t.Fatalf("Apply called %d times, want 1", r.applyCalls)
	}
}

func TestConflictBlocksApplyUntilForceIsArmed(t *testing.T) {
	r := &runner{diffs: []core.DiffResult{{
		Manifest: core.Manifest{Kind: "ConfigMap", Name: "cfg"},
		Conflict: true,
		Managers: []string{"flux"},
		Fields:   []string{".data.tier"},
	}}}

	model, path := newModel(t, r, apply.Target{Context: "kind-dev", Namespace: "default"})
	model = typePath(t, model, path)

	// y must be inert: another manager owns these fields.
	model, _ = press(model, 'y')
	model, cmd := press(model, 'y')
	if cmd != nil {
		cmd()
	}

	if r.applyCalls != 0 {
		t.Fatal("applied over a field-ownership conflict without force armed")
	}

	if !strings.Contains(model.View().Content, "flux") {
		t.Fatal("the conflicting manager should be named on screen")
	}

	// Arm force, then it may proceed.
	model, _ = press(model, 'F')
	model, _ = press(model, 'y')
	_, cmd = press(model, 'y')

	if cmd == nil {
		t.Fatal("force armed but apply still refused")
	}

	cmd()

	if r.applyCalls != 1 || !r.lastForce {
		t.Fatalf("Apply calls=%d force=%v, want 1 and true", r.applyCalls, r.lastForce)
	}
}

func TestDryRunErrorBlocksApply(t *testing.T) {
	r := &runner{diffs: []core.DiffResult{{
		Manifest: core.Manifest{Kind: "ConfigMap", Name: "cfg"},
		Err:      context.DeadlineExceeded,
	}}}

	model, path := newModel(t, r, apply.Target{Context: "kind-dev"})
	model = typePath(t, model, path)

	model, _ = press(model, 'y')
	_, cmd := press(model, 'y')
	if cmd != nil {
		cmd()
	}

	if r.applyCalls != 0 {
		t.Fatal("applied despite a document failing its dry run")
	}
}

func TestEscapeLeavesTheApplyViewFromEveryStage(t *testing.T) {
	r := &runner{diffs: []core.DiffResult{{
		Manifest: core.Manifest{Kind: "ConfigMap", Name: "cfg"}, Added: 1,
	}}}

	// Stage 1: the path input would otherwise swallow esc entirely, leaving the
	// user stranded with no way back and no way to quit.
	model, path := newModel(t, r, apply.Target{Context: "kind-dev"})

	for _, ch := range "some/typed/path" {
		model, _ = press(model, ch)
	}

	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc at the path stage produced no command")
	}

	if _, ok := cmd().(apply.CancelMsg); !ok {
		t.Fatalf("esc produced %T, want apply.CancelMsg", cmd())
	}

	// Stage 2: the diff.
	model, path = newModel(t, r, apply.Target{Context: "kind-dev"})
	model = typePath(t, model, path)

	_, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc at the diff stage produced no command")
	}

	if _, ok := cmd().(apply.CancelMsg); !ok {
		t.Fatalf("esc at diff produced %T, want apply.CancelMsg", cmd())
	}

	// Stage 3: the protected-cluster confirm input.
	model, path = newModel(t, r, apply.Target{Context: "cnc-prod", Protected: true})
	model = typePath(t, model, path)
	model, _ = press(model, 'y')

	_, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc at the protected confirm stage produced no command")
	}

	if _, ok := cmd().(apply.CancelMsg); !ok {
		t.Fatalf("esc at confirm produced %T, want apply.CancelMsg", cmd())
	}
}
