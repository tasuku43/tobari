package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const configuratorRuntimeSourceMetadataSchema = 1

type configuratorRuntimeSourceMetadata struct {
	SchemaVersion   int                   `json:"schema_version"`
	DraftID         string                `json:"draft_id"`
	Binding         tobari.RuntimeBinding `json:"binding"`
	TargetRuntimeID string                `json:"target_runtime_id,omitempty"`
	BaseRevision    tobari.SemanticDigest `json:"base_revision"`
}

// ObserveManagedRuntimeSourceRevision returns one exact digest while holding
// the same Runtime lifecycle fences used by source publication.
func (r *Runtime) ObserveManagedRuntimeSourceRevision(ctx context.Context, reference string) (tobari.SemanticDigest, error) {
	var result tobari.SemanticDigest
	err := r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			manifest, err := r.resolveManagedRuntimeReferenceUnlocked(reference)
			if err != nil {
				return err
			}
			if manifest.Kind != tobari.RuntimeKindManaged {
				return fmt.Errorf("Runtime source is not managed")
			}
			revision, err := digestRuntimeSnapshot(lockContext, manifest.SourcePath)
			if err != nil {
				return err
			}
			result = tobari.SemanticDigest(revision)
			return result.Validate()
		})
	})
	return result, err
}

// ConfiguratorRuntimeSourcePublished verifies a confirmed task receipt against
// canonical editable-source authority while holding the same fences as Apply.
func (r *Runtime) ConfiguratorRuntimeSourcePublished(ctx context.Context, draft tobari.ConfiguratorDraft, source tobari.ConfiguratorRuntimeSource) (bool, error) {
	if draft.Task != tobari.ConfiguratorTaskRuntime || source.ValidateFor(draft) != nil {
		return false, fmt.Errorf("Runtime assist publication receipt is invalid")
	}
	var published bool
	err := r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			manifest, err := r.resolveManagedRuntimeReferenceUnlocked(draft.TargetRuntimeID)
			if err != nil || manifest.ID != draft.TargetRuntimeID || manifest.Kind != tobari.RuntimeKindManaged {
				return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			observed, err := digestRuntimeSnapshot(lockContext, manifest.SourcePath)
			if err != nil {
				return err
			}
			published = observed == string(source.FrozenRevision)
			if !published && observed != string(source.BaseRevision) {
				return tobari.ErrResourceSourceRecoveryRequired
			}
			return nil
		})
	})
	return published, err
}

// ApplyConfiguratorRuntimeSourceOnly promotes one task-scoped frozen working
// tree into the managed Runtime's editable source with an exact base CAS. It
// deliberately does not build or append immutable Runtime authority.
func (r *Runtime) ApplyConfiguratorRuntimeSourceOnly(ctx context.Context, draft tobari.ConfiguratorDraft, source tobari.ConfiguratorRuntimeSource) error {
	if draft.Task != tobari.ConfiguratorTaskRuntime || source.ValidateFor(draft) != nil {
		return fmt.Errorf("Runtime assist source publication is invalid")
	}
	if !source.Changed {
		return r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
			return r.withRuntimeStoreLock(lockContext, func() error {
				manifest, err := r.resolveManagedRuntimeReferenceUnlocked(draft.TargetRuntimeID)
				if err != nil || manifest.ID != draft.TargetRuntimeID {
					return errors.Join(tobari.ErrResourceSourceChanged, err)
				}
				observed, err := digestRuntimeSnapshot(lockContext, manifest.SourcePath)
				if err != nil {
					return err
				}
				if observed != string(source.BaseRevision) {
					return tobari.ErrResourceSourceChanged
				}
				return nil
			})
		})
	}
	_, _, frozenRoot, err := r.configuratorRuntimeSourcePaths(draft)
	if err != nil {
		return err
	}
	frozen := filepath.Join(frozenRoot, strings.TrimPrefix(string(source.FrozenRevision), "sha256:"))
	if observed, digestErr := digestRuntimeSnapshot(ctx, frozen); digestErr != nil || observed != string(source.FrozenRevision) {
		return fmt.Errorf("frozen Runtime assist source is unavailable or changed: %w", digestErr)
	}
	return r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			manifest, err := r.resolveManagedRuntimeReferenceUnlocked(draft.TargetRuntimeID)
			if err != nil || manifest.ID != draft.TargetRuntimeID {
				return errors.Join(tobari.ErrResourceSourceChanged, err)
			}
			current := manifest.SourcePath
			stage := filepath.Join(r.configDirectory, ".configurator-runtime-"+draft.ID+"-new")
			backup := filepath.Join(r.configDirectory, ".configurator-runtime-"+draft.ID+"-old")
			if err := recoverConfiguratorRuntimeSource(lockContext, current, stage, backup, string(source.FrozenRevision)); err != nil {
				return err
			}
			observed, err := digestRuntimeSnapshot(lockContext, current)
			if err != nil {
				return err
			}
			if observed == string(source.FrozenRevision) {
				return nil
			}
			if observed != string(source.BaseRevision) {
				return tobari.ErrResourceSourceChanged
			}
			if err := os.RemoveAll(stage); err != nil {
				return err
			}
			if copied, err := copyRuntimeSource(lockContext, frozen, stage); err != nil || copied != string(source.FrozenRevision) {
				return fmt.Errorf("stage frozen Runtime assist source: %w", err)
			}
			if err := syncRuntimeSourceTree(stage, syncRegularRuntimeFile); err != nil {
				return err
			}
			if err := os.Rename(current, backup); err != nil {
				return err
			}
			if err := os.Rename(stage, current); err != nil {
				return errors.Join(err, os.Rename(backup, current))
			}
			if err := syncDirectoryIfPresent(filepath.Dir(current)); err != nil {
				return err
			}
			if err := os.RemoveAll(backup); err != nil {
				return err
			}
			return syncDirectoryIfPresent(filepath.Dir(current))
		})
	})
}

// ApplyConfiguratorRuntimeSource promotes one frozen managed-Runtime source
// with an exact base CAS, then delegates the immutable revision build to the
// canonical Runtime lifecycle. Replay observes the promoted digest and builds
// the same revision rather than copying mutable Home again.
func (r *Runtime) ApplyConfiguratorRuntimeSource(ctx context.Context, draft tobari.ConfiguratorDraft, source tobari.ConfiguratorRuntimeSource, diagnostics io.Writer) (tobari.RuntimeBinding, error) {
	if err := source.ValidateFor(draft); err != nil {
		return tobari.RuntimeBinding{}, err
	}
	if draft.Task != tobari.ConfiguratorTaskAggregate {
		return tobari.RuntimeBinding{}, fmt.Errorf("task-scoped Runtime source requires source-only publication")
	}
	if !source.Changed {
		return draft.Runtime, nil
	}
	_, _, frozenRoot, err := r.configuratorRuntimeSourcePaths(draft)
	if err != nil {
		return tobari.RuntimeBinding{}, err
	}
	frozen := filepath.Join(frozenRoot, strings.TrimPrefix(string(source.FrozenRevision), "sha256:"))
	if observed, err := digestRuntimeSnapshot(ctx, frozen); err != nil || observed != string(source.FrozenRevision) {
		return tobari.RuntimeBinding{}, fmt.Errorf("frozen Configurator Runtime source is unavailable or changed: %w", err)
	}
	var report tobari.RuntimeReport
	var replayed *tobari.RuntimeBinding
	err = r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		if err := r.withRuntimeStoreLock(lockContext, func() error {
			manifest, err := r.resolveManagedRuntimeReferenceUnlocked(draft.Runtime.RuntimeID)
			if err != nil {
				return err
			}
			binding, err := manifest.Binding(draft.Runtime.Ordinal)
			if err != nil || binding != draft.Runtime {
				return fmt.Errorf("Configurator Runtime binding changed before source promotion: %w", err)
			}
			for _, revision := range manifest.Revisions {
				if revision.Revision != string(source.FrozenRevision) {
					continue
				}
				published, err := manifest.Binding(revision.Ordinal)
				if err != nil {
					return err
				}
				imageID, err := r.resolveFinalContextLoginRuntimeImage(lockContext, published)
				if err != nil || imageID != revision.ImageDigest {
					return fmt.Errorf("published Configurator Runtime revision is not exact immutable material: %w", err)
				}
				replayed = &published
				return nil
			}
			current := manifest.SourcePath
			stage := filepath.Join(r.configDirectory, ".configurator-runtime-"+draft.ID+"-new")
			backup := filepath.Join(r.configDirectory, ".configurator-runtime-"+draft.ID+"-old")
			if err := recoverConfiguratorRuntimeSource(ctx, current, stage, backup, string(source.FrozenRevision)); err != nil {
				return err
			}
			observed, err := digestRuntimeSnapshot(lockContext, current)
			if err != nil {
				return err
			}
			if observed == string(source.FrozenRevision) {
				return nil
			}
			if observed != string(source.BaseRevision) {
				return tobari.ErrResourceSourceChanged
			}
			if err := os.RemoveAll(stage); err != nil {
				return err
			}
			if copied, err := copyRuntimeSource(lockContext, frozen, stage); err != nil || copied != string(source.FrozenRevision) {
				return fmt.Errorf("stage frozen Configurator Runtime source: %w", err)
			}
			if err := syncRuntimeSourceTree(stage, syncRegularRuntimeFile); err != nil {
				return err
			}
			if err := os.Rename(current, backup); err != nil {
				return err
			}
			if err := os.Rename(stage, current); err != nil {
				return errors.Join(err, os.Rename(backup, current))
			}
			if err := syncDirectoryIfPresent(filepath.Dir(current)); err != nil {
				return err
			}
			if err := os.RemoveAll(backup); err != nil {
				return err
			}
			return syncDirectoryIfPresent(filepath.Dir(current))
		}); err != nil {
			return err
		}
		if replayed != nil {
			return nil
		}
		if r.configuratorRuntimeAfterPromotion != nil {
			r.configuratorRuntimeAfterPromotion()
		}
		var buildErr error
		report, buildErr = r.buildManagedRuntimeLifecycleLockedFromSource(lockContext, "", draft.Runtime.RuntimeID, diagnostics, frozen, string(source.FrozenRevision))
		return buildErr
	})
	if err != nil {
		return tobari.RuntimeBinding{}, err
	}
	if replayed != nil {
		return *replayed, nil
	}
	for _, revision := range report.Runtime.Revisions {
		if revision.Revision == string(source.FrozenRevision) {
			return report.Runtime.Binding(revision.Ordinal)
		}
	}
	return tobari.RuntimeBinding{}, fmt.Errorf("Runtime build did not publish the frozen Configurator source revision")
}

func recoverConfiguratorRuntimeSource(ctx context.Context, current, stage, backup, frozenRevision string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	_, currentErr := os.Lstat(current)
	_, stageErr := os.Lstat(stage)
	_, backupErr := os.Lstat(backup)
	currentPresent, stagePresent, backupPresent := currentErr == nil, stageErr == nil, backupErr == nil
	for _, err := range []error{currentErr, stageErr, backupErr} {
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	parent := filepath.Dir(current)
	switch {
	case currentPresent && backupPresent:
		observed, err := digestRuntimeSnapshot(ctx, current)
		if err != nil || observed != frozenRevision {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		if stagePresent {
			if err := os.RemoveAll(stage); err != nil {
				return err
			}
			if err := syncDirectoryIfPresent(parent); err != nil {
				return err
			}
		}
		if err := os.RemoveAll(backup); err != nil {
			return err
		}
		return syncDirectoryIfPresent(parent)
	case !currentPresent && backupPresent && stagePresent:
		observed, err := digestRuntimeSnapshot(ctx, stage)
		if err != nil || observed != frozenRevision {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		if err := os.Rename(stage, current); err != nil {
			return err
		}
		if err := syncDirectoryIfPresent(parent); err != nil {
			return err
		}
		if err := os.RemoveAll(backup); err != nil {
			return err
		}
		return syncDirectoryIfPresent(parent)
	case !currentPresent && backupPresent && !stagePresent:
		if err := os.Rename(backup, current); err != nil {
			return err
		}
		if err := syncDirectoryIfPresent(parent); err != nil {
			return err
		}
		return tobari.ErrResourceSourceRecoveryRequired
	case currentPresent && !backupPresent && stagePresent:
		if err := os.RemoveAll(stage); err != nil {
			return err
		}
		return syncDirectoryIfPresent(parent)
	case currentPresent && !backupPresent && !stagePresent:
		return nil
	default:
		return tobari.ErrResourceSourceRecoveryRequired
	}
}

func (m configuratorRuntimeSourceMetadata) validateFor(draft tobari.ConfiguratorDraft) error {
	targetRuntimeID := draft.Runtime.RuntimeID
	if draft.Task == tobari.ConfiguratorTaskRuntime {
		targetRuntimeID = draft.TargetRuntimeID
	}
	if m.SchemaVersion != configuratorRuntimeSourceMetadataSchema || m.DraftID != draft.ID || m.Binding != draft.Runtime || m.TargetRuntimeID != targetRuntimeID || targetRuntimeID == tobari.StandardRuntimeID || m.BaseRevision.Validate() != nil {
		return fmt.Errorf("Configurator Runtime source metadata is invalid")
	}
	if draft.Task == tobari.ConfiguratorTaskRuntime && m.BaseRevision != draft.TargetRuntimeRevision {
		return tobari.ErrResourceSourceChanged
	}
	return draft.Validate()
}

func (r *Runtime) configuratorRuntimeSourcePaths(draft tobari.ConfiguratorDraft) (working, metadata, frozenRoot string, err error) {
	if err = draft.Validate(); err != nil {
		return
	}
	home, err := r.configuratorHome(draft)
	if err != nil {
		return "", "", "", err
	}
	relative, err := tobari.ConfiguratorWorkingDirectory(draft)
	if err != nil {
		return "", "", "", err
	}
	working = filepath.Join(home, filepath.FromSlash(relative), "runtime", "source")
	root, err := r.ConfiguratorRoot()
	if err != nil {
		return "", "", "", err
	}
	metadata = filepath.Join(root, draft.ID, "runtime-source.json")
	frozenRoot = filepath.Join(root, draft.ID, "frozen-runtime")
	return
}

// PrepareConfiguratorRuntimeSource copies the current editable source for a
// managed Runtime into Home. The copy is non-authoritative and survives
// repeated Configurator sessions; standard Runtime source is immutable and is
// therefore not projected.
func (r *Runtime) PrepareConfiguratorRuntimeSource(ctx context.Context, draft tobari.ConfiguratorDraft) error {
	if err := draft.Validate(); err != nil {
		return err
	}
	targetRuntimeID := draft.Runtime.RuntimeID
	if draft.Task == tobari.ConfiguratorTaskPolicy {
		return nil
	}
	if draft.Task == tobari.ConfiguratorTaskRuntime {
		targetRuntimeID = draft.TargetRuntimeID
	}
	if targetRuntimeID == tobari.StandardRuntimeID {
		return nil
	}
	manifest, err := r.ResolveRuntimeReference(ctx, targetRuntimeID)
	if err != nil {
		return err
	}
	if manifest.Kind != tobari.RuntimeKindManaged || manifest.ID != targetRuntimeID {
		return fmt.Errorf("Configurator Runtime source target changed")
	}
	if draft.Task == tobari.ConfiguratorTaskAggregate {
		binding, bindingErr := manifest.Binding(draft.Runtime.Ordinal)
		if bindingErr != nil || binding != draft.Runtime {
			return fmt.Errorf("Configurator Runtime source binding changed: %w", bindingErr)
		}
	}
	working, metadataPath, _, err := r.configuratorRuntimeSourcePaths(draft)
	if err != nil {
		return err
	}
	if existing, err := readConfiguratorRuntimeSourceMetadata(metadataPath); err == nil {
		if err := existing.validateFor(draft); err != nil {
			return err
		}
		if _, digestErr := digestRuntimeSnapshot(ctx, working); digestErr == nil {
			// The working tree is deliberately agent-editable. Metadata binds its
			// exact draft and immutable base, while Freeze records the current
			// arbitrary safe digest on every resumed review.
			return nil
		} else if !errors.Is(digestErr, os.ErrNotExist) {
			return digestErr
		}
		// Metadata is committed only after the working-directory rename is
		// durable. Repair legacy or interrupted halves from the exact binding.
		return r.restoreConfiguratorRuntimeWorkingCopy(ctx, draft, manifest.SourcePath, working, metadataPath, existing)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if observed, digestErr := digestRuntimeSnapshot(ctx, working); digestErr == nil {
		value := configuratorRuntimeSourceMetadata{SchemaVersion: configuratorRuntimeSourceMetadataSchema, DraftID: draft.ID, Binding: draft.Runtime, TargetRuntimeID: targetRuntimeID, BaseRevision: tobari.SemanticDigest(observed)}
		if err := value.validateFor(draft); err != nil {
			return err
		}
		return writeAtomicJSON(metadataPath, value)
	} else if !errors.Is(digestErr, os.ErrNotExist) {
		return digestErr
	}
	if err := os.MkdirAll(filepath.Dir(working), 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(working), ".source-")
	if err != nil {
		return err
	}
	if err := os.Remove(stage); err != nil {
		return err
	}
	owned := false
	defer func() {
		if !owned {
			_ = os.RemoveAll(stage)
		}
	}()
	revision, err := copyRuntimeSource(ctx, manifest.SourcePath, stage)
	if err != nil {
		return err
	}
	if err := os.Rename(stage, working); err != nil {
		return err
	}
	owned = true
	if err := syncDirectoryIfPresent(filepath.Dir(working)); err != nil {
		return err
	}
	value := configuratorRuntimeSourceMetadata{SchemaVersion: configuratorRuntimeSourceMetadataSchema, DraftID: draft.ID, Binding: draft.Runtime, TargetRuntimeID: targetRuntimeID, BaseRevision: tobari.SemanticDigest(revision)}
	if err := value.validateFor(draft); err != nil {
		return err
	}
	return writeAtomicJSON(metadataPath, value)
}

func (r *Runtime) restoreConfiguratorRuntimeWorkingCopy(ctx context.Context, draft tobari.ConfiguratorDraft, source, working, metadataPath string, value configuratorRuntimeSourceMetadata) error {
	if err := os.MkdirAll(filepath.Dir(working), 0o700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(working), ".source-repair-")
	if err != nil {
		return err
	}
	if err := os.Remove(stage); err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	revision, err := copyRuntimeSource(ctx, source, stage)
	if err != nil || revision != string(value.BaseRevision) {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := os.Rename(stage, working); err != nil {
		return err
	}
	if err := syncDirectoryIfPresent(filepath.Dir(working)); err != nil {
		return err
	}
	return writeAtomicJSON(metadataPath, value)
}

// FreezeConfiguratorRuntimeSource snapshots the agent-edited managed Runtime
// source outside mutable Home and returns only its content-addressed receipt.
func (r *Runtime) FreezeConfiguratorRuntimeSource(ctx context.Context, draft tobari.ConfiguratorDraft) (*tobari.ConfiguratorRuntimeSource, error) {
	if err := draft.Validate(); err != nil {
		return nil, err
	}
	if draft.Task == tobari.ConfiguratorTaskPolicy {
		return nil, nil
	}
	targetRuntimeID := draft.Runtime.RuntimeID
	if draft.Task == tobari.ConfiguratorTaskRuntime {
		targetRuntimeID = draft.TargetRuntimeID
	}
	if targetRuntimeID == tobari.StandardRuntimeID {
		return nil, nil
	}
	working, metadataPath, frozenRoot, err := r.configuratorRuntimeSourcePaths(draft)
	if err != nil {
		return nil, err
	}
	metadata, err := readConfiguratorRuntimeSourceMetadata(metadataPath)
	if err != nil {
		return nil, err
	}
	if err := metadata.validateFor(draft); err != nil {
		return nil, err
	}
	if err := ensureDurableConfiguratorFrozenRoot(frozenRoot); err != nil {
		return nil, err
	}
	stage, err := os.MkdirTemp(frozenRoot, ".freeze-")
	if err != nil {
		return nil, err
	}
	if err := os.Remove(stage); err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	revision, err := copyRuntimeSource(ctx, working, stage)
	if err != nil {
		return nil, err
	}
	// A frozen submission is a durable authority receipt. Flush every copied
	// file and directory before publishing the snapshot name so the Store can
	// never persist a receipt for a merely cached tree after power loss.
	if err := syncRuntimeSourceTree(stage, syncRegularRuntimeFile); err != nil {
		return nil, err
	}
	result := &tobari.ConfiguratorRuntimeSource{SchemaVersion: tobari.ConfiguratorRuntimeSourceSchemaVersion, RuntimeID: targetRuntimeID, BaseRevision: metadata.BaseRevision, FrozenRevision: tobari.SemanticDigest(revision), Changed: revision != string(metadata.BaseRevision)}
	if err := result.ValidateFor(draft); err != nil {
		return nil, err
	}
	if !result.Changed {
		return result, nil
	}
	final := filepath.Join(frozenRoot, strings.TrimPrefix(revision, "sha256:"))
	if observed, digestErr := digestRuntimeSnapshot(ctx, final); digestErr == nil {
		if observed != revision {
			return nil, fmt.Errorf("frozen Configurator Runtime source digest changed")
		}
		if err := syncRuntimeSourceTree(final, syncRegularRuntimeFile); err != nil {
			return nil, err
		}
		if err := syncDirectoryIfPresent(frozenRoot); err != nil {
			return nil, err
		}
		return result, nil
	} else if !errors.Is(digestErr, os.ErrNotExist) {
		return nil, digestErr
	}
	if err := os.Rename(stage, final); err != nil {
		return nil, err
	}
	keep = true
	if err := syncDirectoryIfPresent(frozenRoot); err != nil {
		return nil, err
	}
	return result, nil
}

func ensureDurableConfiguratorFrozenRoot(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := requirePrivateDirectory(path); err != nil {
			return err
		}
		return syncDirectoryIfPresent(filepath.Dir(path))
	}
	if err := requirePrivateDirectory(path); err != nil {
		return err
	}
	// The draft directory already owns durable reservation metadata. Persist
	// its new frozen-runtime child before an immutable submission can refer to
	// any snapshot published beneath that child.
	return syncDirectoryIfPresent(filepath.Dir(path))
}

func readConfiguratorRuntimeSourceMetadata(path string) (configuratorRuntimeSourceMetadata, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return configuratorRuntimeSourceMetadata{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() <= 0 || info.Size() > maxRuntimeMetadataSize {
		return configuratorRuntimeSourceMetadata{}, fmt.Errorf("Configurator Runtime source metadata is unsafe")
	}
	var value configuratorRuntimeSourceMetadata
	if err := readStrictJSON(path, &value); err != nil {
		return value, err
	}
	return value, nil
}
