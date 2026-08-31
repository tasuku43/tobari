package dockerruntime

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// ObserveFinalRuntimeMaterials proves that every distinct Runtime binding in
// the selected final Template history still resolves to the exact lifecycle
// authority and compatible Docker material required by ordinary entry. The
// final collection is selection authority; this observer never reconstructs a
// target from a predecessor name, selector, source path, or retained file.
func (r *Runtime) ObserveFinalRuntimeMaterials(ctx context.Context, collection tobari.WorkspaceAuthorityCollection) ([]tobari.RuntimeBinding, error) {
	if r == nil {
		return nil, fmt.Errorf("Docker runtime is unavailable")
	}
	if ctx == nil {
		return nil, fmt.Errorf("final Runtime material observation context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bindings, err := finalTemplateRuntimeBindings(collection)
	if err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return []tobari.RuntimeBinding{}, nil
	}
	if r.finalRuntimeProtectionSource == nil {
		return nil, fmt.Errorf("final Runtime material authority is unavailable")
	}
	expectedAuthority, err := tobari.NewFinalRuntimeProtectionAuthority(collection, true)
	if err != nil {
		return nil, err
	}

	var result []tobari.RuntimeBinding
	err = r.withLifecycleObservation(ctx, func(lockContext context.Context) error {
		observationContext, cancel := context.WithTimeout(lockContext, runtimeLifecycleWallBudget)
		defer cancel()
		if err := r.confirmFinalRuntimeMaterialAuthority(observationContext, expectedAuthority); err != nil {
			return err
		}
		// One lifecycle snapshot supplies the complete catalog and Docker
		// availability join. Re-reading by name or selector would create a
		// second, weaker selection path for this diagnostic.
		snapshot, _, err := r.readRuntimeLifecycleSnapshotLocked(observationContext)
		if err != nil {
			return err
		}
		if err := r.confirmFinalRuntimeMaterialAuthority(observationContext, expectedAuthority); err != nil {
			return err
		}
		result, err = r.validateFinalRuntimeMaterials(observationContext, snapshot, bindings)
		if err != nil {
			return err
		}
		return observationContext.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func finalTemplateRuntimeBindings(collection tobari.WorkspaceAuthorityCollection) ([]tobari.RuntimeBinding, error) {
	if err := collection.Validate(); err != nil {
		return nil, err
	}
	byReference := make(map[string]tobari.RuntimeBinding)
	for _, template := range collection.Templates {
		for _, revision := range template.Retained {
			binding := revision.Body.EntryDefaults.Runtime
			key := binding.RuntimeID + "\x00" + binding.Revision
			if previous, exists := byReference[key]; exists && !reflect.DeepEqual(previous, binding) {
				return nil, fmt.Errorf("final Templates bind one Runtime revision inconsistently")
			}
			byReference[key] = binding
		}
	}
	result := make([]tobari.RuntimeBinding, 0, len(byReference))
	for _, binding := range byReference {
		result = append(result, binding)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].RuntimeID+"\x00"+result[i].Revision < result[j].RuntimeID+"\x00"+result[j].Revision
	})
	return result, nil
}

func (r *Runtime) confirmFinalRuntimeMaterialAuthority(ctx context.Context, expected tobari.FinalRuntimeProtectionAuthority) error {
	observed, err := r.finalRuntimeProtectionSource.ReadFinalRuntimeProtectionAuthority(ctx)
	if err != nil {
		return err
	}
	if err := observed.Validate(); err != nil || !reflect.DeepEqual(observed, expected) {
		return fmt.Errorf("final Runtime material observation is not bound to the selected complete authority")
	}
	return nil
}

func (r *Runtime) validateFinalRuntimeMaterials(ctx context.Context, snapshot tobari.RuntimeLifecycleSnapshot, bindings []tobari.RuntimeBinding) ([]tobari.RuntimeBinding, error) {
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	runtimes := make(map[string]tobari.RuntimeManifest, len(snapshot.Runtimes))
	materials := make(map[string]tobari.RuntimeMaterialObservation, len(snapshot.Materials))
	for _, manifest := range snapshot.Runtimes {
		runtimes[manifest.ID] = manifest
	}
	for _, material := range snapshot.Materials {
		materials[material.RuntimeID+"\x00"+material.Revision] = material
	}
	for _, binding := range bindings {
		for _, activity := range snapshot.Journals.Active {
			if activity.RuntimeID == binding.RuntimeID {
				return nil, fmt.Errorf("final Template Runtime lifecycle is active")
			}
		}
		if binding.RuntimeID == tobari.StandardRuntimeID {
			if err := r.validateExactStandardRuntimeBinding(binding); err != nil {
				return nil, fmt.Errorf("final Template standard Runtime binding is invalid: %w", err)
			}
			if _, err := r.inspectFinalStandardRuntimeImage(ctx, binding.Image); err != nil {
				return nil, fmt.Errorf("final Template standard Runtime material is incompatible or unavailable: %w", err)
			}
			continue
		}
		manifest, exists := runtimes[binding.RuntimeID]
		if !exists {
			return nil, fmt.Errorf("final Template Runtime authority is unavailable")
		}
		var revision tobari.RuntimeRevision
		found := false
		for _, candidate := range manifest.Revisions {
			if candidate.Revision == binding.Revision {
				revision, found = candidate, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("final Template Runtime revision is unavailable")
		}
		observedBinding, err := manifest.Binding(revision.Ordinal)
		if err != nil || !reflect.DeepEqual(observedBinding, binding) {
			return nil, fmt.Errorf("final Template Runtime binding differs from exact lifecycle authority")
		}
		if manifest.Kind == tobari.RuntimeKindManaged {
			material, exists := materials[binding.RuntimeID+"\x00"+binding.Revision]
			if !exists || material.Availability != tobari.RuntimeAvailabilityAvailable ||
				!material.TagPresent || !material.ContentPresent || !material.OwnershipVerified || !material.ObservationComplete {
				return nil, fmt.Errorf("final Template managed Runtime material is not exactly available")
			}
			if err := r.validateManagedRuntimeBuildCompatibility(ctx, revision.ImageDigest); err != nil {
				return nil, fmt.Errorf("final Template managed Runtime material is incompatible: %w", err)
			}
			continue
		}
		return nil, fmt.Errorf("final Template Runtime kind is unsupported")
	}
	return append([]tobari.RuntimeBinding{}, bindings...), nil
}
