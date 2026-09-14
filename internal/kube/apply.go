package kube

import (
	"context"
	"fmt"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"sigs.k8s.io/yaml"

	"github.com/mo-pankaj/kctl/internal/core"
)

// fieldManager identifies kctl's ownership in server-side apply.
const fieldManager = "kctl"

// Applier performs server-side apply, and the dry run that precedes it.
type Applier struct {
	logger    *zap.Logger
	dynamic   dynamic.Interface
	discovery discovery.DiscoveryInterface
}

// NewApplier builds an applier from a REST config.
func NewApplier(logger *zap.Logger, cfg *rest.Config) (a *Applier, err error) {
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		err = fmt.Errorf("error-building-dynamic-client :%w", err)

		return a, err
	}

	disco, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		err = fmt.Errorf("error-building-discovery-client :%w", err)

		return a, err
	}

	a = &Applier{
		logger:    logger.With(zap.String("component", "applier")),
		dynamic:   dyn,
		discovery: disco,
	}

	return a, err
}

// resourceFor maps a manifest's kind to the REST resource that serves it.
func (a *Applier) resourceFor(doc core.Manifest) (ri dynamic.ResourceInterface, err error) {
	gv, err := schema.ParseGroupVersion(doc.APIVersion)
	if err != nil {
		err = fmt.Errorf("error-parsing-group-version :%w", err)

		return ri, err
	}

	groups, err := restmapper.GetAPIGroupResources(a.discovery)
	if err != nil {
		err = fmt.Errorf("error-discovering-api-groups :%w", err)

		return ri, err
	}

	mapper := restmapper.NewDiscoveryRESTMapper(groups)

	mapping, err := mapper.RESTMapping(schema.GroupKind{Group: gv.Group, Kind: doc.Kind}, gv.Version)
	if err != nil {
		err = fmt.Errorf("error-mapping-kind :%s: %w", doc.Kind, err)

		return ri, err
	}

	if mapping.Scope.Name() == "namespace" {
		ri = a.dynamic.Resource(mapping.Resource).Namespace(doc.Namespace)

		return ri, err
	}

	ri = a.dynamic.Resource(mapping.Resource)

	return ri, err
}

// DryRun applies every document with DryRun=All and diffs the result against
// what is live, without writing anything.
func (a *Applier) DryRun(ctx context.Context, docs []core.Manifest) (results []core.DiffResult, err error) {
	for _, doc := range docs {
		results = append(results, a.dryRunOne(ctx, doc))
	}

	return results, err
}

func (a *Applier) dryRunOne(ctx context.Context, doc core.Manifest) (result core.DiffResult) {
	logger := a.logger.With(
		zap.String("method", "dryRunOne"),
		zap.String("kind", doc.Kind),
		zap.String("name", doc.Name),
		zap.String("namespace", doc.Namespace),
	)

	result = core.DiffResult{Manifest: doc}

	ri, err := a.resourceFor(doc)
	if err != nil {
		logger.Error("error-resolving-resource", zap.Error(err))
		result.Err = err

		return result
	}

	live, err := ri.Get(ctx, doc.Name, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		result.Creation = true
		live = nil

	case err != nil:
		err = fmt.Errorf("error-reading-live-object :%w", err)
		logger.Error("error-reading-live-object", zap.Error(err))
		result.Err = err

		return result
	}

	proposed, err := ri.Patch(ctx, doc.Name, types.ApplyPatchType, doc.Raw, metav1.PatchOptions{
		FieldManager: fieldManager,
		DryRun:       []string{metav1.DryRunAll},
	})
	switch {
	case apierrors.IsConflict(err):
		// Surface the conflict as data rather than a bare error: the user needs
		// to see who owns the fields before choosing to force.
		result.Conflict = true
		result.Managers = conflictManagers(err)
		result.Fields = conflictFields(err)
		logger.Warn("dry-run-field-conflict",
			zap.Strings("managers", result.Managers), zap.Strings("fields", result.Fields))

		return result

	case err != nil:
		err = fmt.Errorf("error-dry-run-apply :%w", err)
		logger.Error("error-dry-run-apply", zap.Error(err))
		result.Err = err

		return result
	}

	before := ""
	if live != nil {
		before = renderForDiff(live)
	}

	after := renderForDiff(proposed)

	result.Unified, result.Added, result.Removed = unifiedDiff(doc.Describe(), before, after)

	return result
}

// Apply writes the documents for real, in order. force controls whether a
// field-ownership conflict is overridden.
func (a *Applier) Apply(ctx context.Context, docs []core.Manifest, force bool) (results []core.ApplyResult, err error) {
	for _, doc := range docs {
		results = append(results, a.applyOne(ctx, doc, force))
	}

	return results, err
}

func (a *Applier) applyOne(ctx context.Context, doc core.Manifest, force bool) (result core.ApplyResult) {
	logger := a.logger.With(
		zap.String("method", "applyOne"),
		zap.String("kind", doc.Kind),
		zap.String("name", doc.Name),
		zap.String("namespace", doc.Namespace),
		zap.Bool("force", force),
	)

	result = core.ApplyResult{Manifest: doc}

	ri, err := a.resourceFor(doc)
	if err != nil {
		result.Err = err

		return result
	}

	_, err = ri.Patch(ctx, doc.Name, types.ApplyPatchType, doc.Raw, metav1.PatchOptions{
		FieldManager: fieldManager,
		Force:        &force,
	})

	switch {
	case apierrors.IsConflict(err):
		// Another field manager owns fields this manifest sets. Surface who,
		// and require an explicit force rather than stomping them.
		result.Conflict = true
		result.Managers = conflictManagers(err)
		result.Err = fmt.Errorf("error-field-ownership-conflict :%w", err)
		logger.Warn("field-ownership-conflict", zap.Strings("managers", result.Managers))

		return result

	case err != nil:
		result.Err = fmt.Errorf("error-applying :%w", err)
		logger.Error("error-applying", zap.Error(err))

		return result
	}

	result.Applied = true
	logger.Info("applied")

	return result
}

// conflictManagers pulls the owning managers out of a 409 status.
func conflictManagers(err error) (managers []string) {
	status, ok := err.(apierrors.APIStatus)
	if !ok || status.Status().Details == nil {
		return managers
	}

	seen := map[string]bool{}

	for _, cause := range status.Status().Details.Causes {
		// Messages read: conflict with "flux" using apps/v1
		parts := strings.Split(cause.Message, `"`)
		if len(parts) < 2 || seen[parts[1]] {
			continue
		}

		seen[parts[1]] = true
		managers = append(managers, parts[1])
	}

	return managers
}

// conflictFields pulls the contested field paths out of a 409 status.
func conflictFields(err error) (fields []string) {
	status, ok := err.(apierrors.APIStatus)
	if !ok || status.Status().Details == nil {
		return fields
	}

	for _, cause := range status.Status().Details.Causes {
		if cause.Field != "" {
			fields = append(fields, cause.Field)
		}
	}

	return fields
}

// renderForDiff serialises an object to YAML with the fields that always differ
// stripped, so the diff shows intent rather than bookkeeping.
func renderForDiff(obj *unstructured.Unstructured) (s string) {
	clone := obj.DeepCopy()

	unstructured.RemoveNestedField(clone.Object, "metadata", "managedFields")
	unstructured.RemoveNestedField(clone.Object, "metadata", "resourceVersion")
	unstructured.RemoveNestedField(clone.Object, "metadata", "generation")
	unstructured.RemoveNestedField(clone.Object, "metadata", "creationTimestamp")
	unstructured.RemoveNestedField(clone.Object, "metadata", "uid")
	unstructured.RemoveNestedField(clone.Object, "metadata", "selfLink")
	unstructured.RemoveNestedField(clone.Object, "status")

	out, err := yaml.Marshal(clone.Object)
	if err != nil {
		return s
	}

	s = string(out)

	return s
}

// unifiedDiff renders a unified diff and counts the changed lines.
func unifiedDiff(name, before, after string) (out string, added, removed int) {
	edits := myers.ComputeEdits(span.URIFromPath(name), before, after)
	out = fmt.Sprint(gotextdiff.ToUnified(name+" (live)", name+" (applied)", before, edits))

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			continue

		case strings.HasPrefix(line, "+"):
			added++

		case strings.HasPrefix(line, "-"):
			removed++
		}
	}

	return out, added, removed
}
