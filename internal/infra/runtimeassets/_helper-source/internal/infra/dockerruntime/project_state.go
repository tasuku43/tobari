package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const maxProjectStateBytes = 128 * 1024

const (
	projectJournalSchema = 1
	projectOpCreate      = "create"
	projectOpDelete      = "delete"
	projectPhaseStarted  = "started"
	projectPhaseHome     = "home_created"
	projectPhaseState    = "state_created"
	projectPhaseIndex    = "root_index_created"
	projectPhaseRuntime  = "runtime_removed"
	projectPhaseInstance = "instance_removed"
)

type projectJournal struct {
	SchemaVersion       int    `json:"schema_version"`
	Operation           string `json:"operation"`
	ProjectID           string `json:"workspace_id"`
	Root                string `json:"root"`
	WorkspaceManifestID string `json:"workspace_manifest_id"`
	Phase               string `json:"phase"`
}

func (j projectJournal) Validate() error {
	if j.SchemaVersion != projectJournalSchema {
		return fmt.Errorf("project journal schema version must be %d", projectJournalSchema)
	}
	if j.Operation != projectOpCreate && j.Operation != projectOpDelete {
		return fmt.Errorf("project journal operation is invalid")
	}
	if err := tobari.ValidateWorkspaceID(j.ProjectID); err != nil {
		return err
	}
	if err := tobari.ValidateCanonicalRoot(j.Root); err != nil {
		return err
	}
	if err := tobari.ValidateWorkspaceManifestID(j.WorkspaceManifestID); err != nil {
		return err
	}
	if j.Phase == "" {
		return fmt.Errorf("project journal phase is missing")
	}
	return nil
}

// ResolveProject resolves cwd, then returns the nearest logical Tobari. A
// pending journal is reconciled under the project lock before selection so a
// process interrupted at a multi-file boundary cannot make the next command
// select stale state.
func (r *Runtime) ResolveProject(ctx context.Context, cwd string) (tobari.Workspace, bool, error) {
	return r.ResolveProjectInContext(ctx, cwd, "")
}

func (r *Runtime) ResolveProjectInContext(ctx context.Context, cwd, contextName string) (tobari.Workspace, bool, error) {
	manifest, _, err := r.resolveContext(contextName)
	if err != nil {
		return tobari.Workspace{}, false, err
	}
	return r.ResolveBoundProject(ctx, cwd, manifest)
}

// ResolveBoundProject consumes the already resolved stable Context binding;
// lifecycle selection must not rediscover a display-name selector.
func (r *Runtime) ResolveBoundProject(ctx context.Context, cwd string, manifest tobari.WorkspaceManifest) (tobari.Workspace, bool, error) {
	if err := manifest.Validate(); err != nil {
		return tobari.Workspace{}, false, err
	}
	resolved, err := r.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return tobari.Workspace{}, false, err
	}
	var (
		instance tobari.Workspace
		found    bool
	)
	err = r.withProjectLock(ctx, func() error {
		if err := r.reconcileProjectJournal(); err != nil {
			return err
		}
		var resolveErr error
		instance, found, resolveErr = r.resolveProjectUnlocked(resolved, manifest.ID)
		return resolveErr
	})
	if err != nil {
		return tobari.Workspace{}, false, err
	}
	return instance, found, nil
}

// ObserveBoundProject selects logical state without creating a lock file. A
// pre-existing journal is the sole read-side exception: it is reconciled under
// the project lock so interrupted multi-file state cannot remain authoritative.
func (r *Runtime) ObserveBoundProject(ctx context.Context, cwd string, manifest tobari.WorkspaceManifest) (tobari.Workspace, bool, error) {
	if err := manifest.Validate(); err != nil {
		return tobari.Workspace{}, false, err
	}
	resolved, err := r.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return tobari.Workspace{}, false, err
	}
	var instance tobari.Workspace
	var found bool
	err = r.withProjectObservation(ctx, func() error {
		var resolveErr error
		instance, found, resolveErr = r.resolveProjectUnlocked(resolved, manifest.ID)
		return resolveErr
	})
	return instance, found, err
}

func (r *Runtime) resolveProjectUnlocked(cwd, contextID string) (tobari.Workspace, bool, error) {
	indexes, err := r.listRootIndexes()
	if err != nil {
		return tobari.Workspace{}, false, err
	}
	indexes, err = tobari.RootIndexesForContext(indexes, contextID)
	if err != nil {
		return tobari.Workspace{}, false, err
	}
	index, found, err := tobari.NearestRoot(cwd, indexes)
	if err != nil || !found {
		if err != nil {
			return tobari.Workspace{}, false, err
		}
		return r.resolveOrphanInstance(cwd, contextID)
	}
	instance, err := r.readProjectInstance(index.InstanceID)
	if err == nil {
		if instance.Root != index.Root {
			return tobari.Workspace{}, false, fmt.Errorf("root index and instance root disagree")
		}
		return instance, true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return tobari.Workspace{}, false, err
	}
	// A root index contains enough immutable identity to make deletion
	// recoverable even when the instance file was lost after the index write.
	// The caller must still use the normal ownership checks before removing
	// Docker resources.
	return cleanupOnlyProjectInstance(index), true, nil
}

func cleanupOnlyProjectInstance(index tobari.RootIndex) tobari.Workspace {
	return tobari.Workspace{
		SchemaVersion:         tobari.WorkspaceStateSchemaVersion,
		ID:                    index.InstanceID,
		Root:                  index.Root,
		WorkspaceManifestID:   index.WorkspaceManifestID,
		WorkspaceManifestName: index.WorkspaceManifestName,
		Profile:               tobari.DefaultProfile,
		Image:                 tobari.BuiltinImageSelector,
		Incomplete:            true,
	}
}

func (r *Runtime) resolveOrphanInstance(cwd, contextID string) (tobari.Workspace, bool, error) {
	entries, err := os.ReadDir(r.instancesDirectory())
	if errors.Is(err, os.ErrNotExist) {
		return tobari.Workspace{}, false, nil
	}
	if err != nil {
		return tobari.Workspace{}, false, fmt.Errorf("read project instances: %w", err)
	}
	instances := make([]tobari.Workspace, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return tobari.Workspace{}, false, fmt.Errorf("project instance directory contains an unsafe entry")
		}
		instance, readErr := r.readProjectInstance(entry.Name())
		if readErr != nil {
			return tobari.Workspace{}, false, readErr
		}
		instances = append(instances, instance)
	}
	if len(instances) == 0 {
		return tobari.Workspace{}, false, nil
	}
	roots := make([]tobari.RootIndex, 0, len(instances))
	byRoot := make(map[string]tobari.Workspace, len(instances))
	for _, instance := range instances {
		if instance.WorkspaceManifestID != contextID {
			continue
		}
		roots = append(roots, tobari.RootIndex{
			SchemaVersion:         tobari.WorkspaceStateSchemaVersion,
			Root:                  instance.Root,
			InstanceID:            instance.ID,
			WorkspaceManifestID:   instance.WorkspaceManifestID,
			WorkspaceManifestName: instance.WorkspaceManifestName,
		})
		byRoot[instance.Root] = instance
	}
	index, found, err := tobari.NearestRoot(cwd, roots)
	if err != nil || !found {
		return tobari.Workspace{}, found, err
	}
	return byRoot[index.Root], true, nil
}

// ResolveOrCreateProject returns the nearest logical Tobari. When none covers
// cwd, it creates exactly one durable root index, instance record, and home
// before any Docker resource is created.
func (r *Runtime) ResolveOrCreateProject(
	ctx context.Context, cwd string,
) (tobari.Workspace, bool, error) {
	return r.ResolveOrCreateProjectInContext(ctx, cwd, "")
}

func (r *Runtime) ResolveOrCreateProjectInContext(
	ctx context.Context, cwd, contextName string,
) (tobari.Workspace, bool, error) {
	resolved, err := r.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return tobari.Workspace{}, false, err
	}
	var (
		instance tobari.Workspace
		created  bool
	)
	manifest, _, err := r.resolveContext(contextName)
	if err != nil {
		return tobari.Workspace{}, false, err
	}
	err = r.withProjectLock(ctx, func() error {
		if err := r.reconcileProjectJournal(); err != nil {
			return err
		}
		indexes, loadErr := r.listRootIndexes()
		if loadErr != nil {
			return loadErr
		}
		indexes, filterErr := tobari.RootIndexesForContext(indexes, manifest.ID)
		if filterErr != nil {
			return filterErr
		}
		index, found, nearestErr := tobari.NearestRoot(resolved, indexes)
		if nearestErr != nil {
			return nearestErr
		}
		if found {
			loaded, readErr := r.readProjectInstance(index.InstanceID)
			if errors.Is(readErr, os.ErrNotExist) {
				instance = cleanupOnlyProjectInstance(index)
				return nil
			}
			if readErr != nil {
				return readErr
			}
			if loaded.Root != index.Root {
				return fmt.Errorf("root index and instance root disagree")
			}
			instance = loaded
			return nil
		}
		createdInstance, createErr := r.createProjectUnlocked(ctx, resolved, manifest)
		if createErr != nil {
			return createErr
		}
		instance, created = createdInstance, true
		return nil
	})
	if err != nil {
		return tobari.Workspace{}, false, err
	}
	return instance, created, nil
}

// CreateProject always creates a logical Workspace at the canonical cwd. It
// intentionally permits containing ancestor roots, but rejects an exact root
// that appeared after the caller's selection snapshot.
func (r *Runtime) CreateProject(ctx context.Context, cwd string) (tobari.Workspace, error) {
	return r.CreateProjectInContext(ctx, cwd, "")
}

func (r *Runtime) CreateProjectInContext(ctx context.Context, cwd, contextName string) (tobari.Workspace, error) {
	manifest, _, err := r.resolveContext(contextName)
	if err != nil {
		return tobari.Workspace{}, err
	}
	return r.CreateBoundProject(ctx, cwd, manifest)
}

// CreateBoundProject creates only for the stable Context binding resolved by
// the application before lifecycle state selection.
func (r *Runtime) CreateBoundProject(ctx context.Context, cwd string, manifest tobari.WorkspaceManifest) (tobari.Workspace, error) {
	if err := manifest.Validate(); err != nil {
		return tobari.Workspace{}, err
	}
	resolved, err := r.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return tobari.Workspace{}, err
	}
	var instance tobari.Workspace
	err = r.withProjectLock(ctx, func() error {
		if err := r.reconcileProjectJournal(); err != nil {
			return err
		}
		indexes, err := r.listRootIndexes()
		if err != nil {
			return err
		}
		for _, index := range indexes {
			if index.Root == resolved && index.WorkspaceManifestID == manifest.ID {
				return tobari.ErrProjectExists
			}
		}
		created, err := r.createProjectUnlocked(ctx, resolved, manifest)
		if err != nil {
			return err
		}
		instance = created
		return nil
	})
	if err != nil {
		return tobari.Workspace{}, err
	}
	return instance, nil
}

func (r *Runtime) createProjectUnlocked(ctx context.Context, resolved string, manifest tobari.WorkspaceManifest) (tobari.Workspace, error) {
	image, imageErr := r.resolveContextImageFor(ctx, manifest)
	if imageErr != nil {
		return tobari.Workspace{}, imageErr
	}
	createdInstance, createErr := r.identities.newProjectInstance(tobari.ProjectInstanceRequest{
		Root:                     resolved,
		WorkspaceManifestID:      manifest.ID,
		WorkspaceManifestName:    manifest.Name,
		Image:                    image,
		CreationDefaultsRevision: manifest.Desired.CreationDefaultsRevision,
		BootstrapRevision: func() string {
			if manifest.Bootstrap == nil {
				return ""
			}
			return manifest.Bootstrap.Revision
		}(),
	})
	if createErr != nil {
		return tobari.Workspace{}, createErr
	}
	journal := projectJournal{
		SchemaVersion: projectJournalSchema, Operation: projectOpCreate,
		ProjectID: createdInstance.ID, Root: createdInstance.Root, Phase: projectPhaseStarted,
		WorkspaceManifestID: createdInstance.WorkspaceManifestID,
	}
	if err := r.writeProjectJournal(journal); err != nil {
		return tobari.Workspace{}, err
	}
	if err := r.ensurePrivateDirectory(r.projectHomePath(createdInstance.ID)); err != nil {
		return tobari.Workspace{}, r.discardUnindexedProject(createdInstance, fmt.Errorf("create project home: %w", err))
	}
	if err := applyProjectBootstrap(r.projectHomePath(createdInstance.ID), manifest.Bootstrap); err != nil {
		return tobari.Workspace{}, r.discardUnindexedProject(createdInstance, fmt.Errorf("apply Workspace bootstrap: %w", err))
	}
	journal.Phase = projectPhaseHome
	if err := r.writeProjectJournal(journal); err != nil {
		return tobari.Workspace{}, err
	}
	if err := r.writeProjectInstance(createdInstance); err != nil {
		return tobari.Workspace{}, r.discardUnindexedProject(createdInstance, err)
	}
	journal.Phase = projectPhaseState
	if err := r.writeProjectJournal(journal); err != nil {
		return tobari.Workspace{}, err
	}
	if err := r.writeRootIndex(tobari.RootIndex{
		SchemaVersion:         tobari.WorkspaceStateSchemaVersion,
		Root:                  createdInstance.Root,
		InstanceID:            createdInstance.ID,
		WorkspaceManifestID:   createdInstance.WorkspaceManifestID,
		WorkspaceManifestName: createdInstance.WorkspaceManifestName,
	}); err != nil {
		return tobari.Workspace{}, r.discardUnindexedProject(createdInstance, err)
	}
	journal.Phase = projectPhaseIndex
	if err := r.clearProjectJournal(); err != nil {
		return tobari.Workspace{}, err
	}
	return createdInstance, nil
}

func (r *Runtime) discardUnindexedProject(instance tobari.Workspace, cause error) error {
	directory, err := r.projectDirectory(instance.ID)
	if err != nil {
		return fmt.Errorf("%w; discard unindexed project: %v", cause, err)
	}
	if err := os.RemoveAll(directory); err != nil {
		return fmt.Errorf("%w; discard unindexed project: %v", cause, err)
	}
	if err := syncDirectory(filepath.Dir(directory)); err != nil {
		return fmt.Errorf("%w; sync discarded project state: %v", cause, err)
	}
	if err := r.clearProjectJournal(); err != nil {
		return fmt.Errorf("%w; clear project journal: %v", cause, err)
	}
	return cause
}

func (r *Runtime) projectJournalPath() string {
	return filepath.Join(r.stateDirectory, "project-journal.json")
}

func (r *Runtime) writeProjectJournal(journal projectJournal) error {
	if err := journal.Validate(); err != nil {
		return err
	}
	return writeAtomicJSON(r.projectJournalPath(), journal)
}

func (r *Runtime) readProjectJournal() (projectJournal, bool, error) {
	var journal projectJournal
	if err := readStrictJSON(r.projectJournalPath(), &journal); errors.Is(err, os.ErrNotExist) {
		return projectJournal{}, false, nil
	} else if err != nil {
		return projectJournal{}, false, err
	}
	if err := journal.Validate(); err != nil {
		return projectJournal{}, false, err
	}
	return journal, true, nil
}

func (r *Runtime) clearProjectJournal() error {
	if err := os.Remove(r.projectJournalPath()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return syncDirectory(r.stateDirectory)
}

func (r *Runtime) reconcileProjectJournal() error {
	journal, exists, err := r.readProjectJournal()
	if err != nil || !exists {
		return err
	}
	switch journal.Operation {
	case projectOpCreate:
		indexPath, pathErr := r.rootIndexPath(journal.Root, journal.WorkspaceManifestID)
		if pathErr != nil {
			return pathErr
		}
		state, stateErr := r.readProjectInstance(journal.ProjectID)
		var index tobari.RootIndex
		indexErr := readStrictJSON(indexPath, &index)
		if stateErr == nil && indexErr == nil && state.Root == journal.Root && index.Root == journal.Root && index.InstanceID == journal.ProjectID {
			return r.clearProjectJournal()
		}
		return r.removeIncompleteProject(journal, indexPath)
	case projectOpDelete:
		if journal.Phase != projectPhaseRuntime && journal.Phase != projectPhaseInstance {
			return nil
		}
		indexPath, pathErr := r.rootIndexPath(journal.Root, journal.WorkspaceManifestID)
		if pathErr != nil {
			return pathErr
		}
		return r.removeIncompleteProject(journal, indexPath)
	default:
		return fmt.Errorf("unsupported project journal operation")
	}
}

func (r *Runtime) removeIncompleteProject(journal projectJournal, indexPath string) error {
	directory, err := r.projectDirectory(journal.ProjectID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(directory); err != nil {
		return fmt.Errorf("remove incomplete project directory: %w", err)
	}
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove incomplete project index: %w", err)
	}
	if err := syncDirectoryIfPresent(filepath.Dir(directory)); err != nil {
		return err
	}
	if err := syncDirectoryIfPresent(filepath.Dir(indexPath)); err != nil {
		return err
	}
	return r.clearProjectJournal()
}

func (r *Runtime) resolveContextImage(ctx context.Context) (string, error) {
	manifest, _, err := r.activeContext()
	if err != nil {
		return "", err
	}
	return r.resolveContextImageFor(ctx, manifest)
}

func (r *Runtime) resolveContextImageFor(ctx context.Context, manifest tobari.WorkspaceManifest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := manifest.Validate(); err != nil {
		return "", err
	}
	return r.resolveBuiltinImageSelector(manifest.Image), nil
}

// ListProjects returns every valid logical Tobari record ordered by root.
func (r *Runtime) ListProjects(ctx context.Context) ([]tobari.Workspace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var instances []tobari.Workspace
	err := r.withProjectObservation(ctx, func() error {
		indexes, err := r.listRootIndexes()
		if err != nil {
			return err
		}
		instances = make([]tobari.Workspace, 0, len(indexes))
		indexedIDs := make(map[string]bool, len(indexes))
		for _, index := range indexes {
			indexedIDs[index.InstanceID] = true
			instance, readErr := r.readProjectInstance(index.InstanceID)
			if errors.Is(readErr, os.ErrNotExist) {
				instances = append(instances, cleanupOnlyProjectInstance(index))
				continue
			}
			if readErr != nil {
				return readErr
			}
			if instance.Root != index.Root {
				return fmt.Errorf("root index and instance root disagree")
			}
			instances = append(instances, instance)
		}
		entries, err := os.ReadDir(r.instancesDirectory())
		if errors.Is(err, os.ErrNotExist) {
			entries = nil
		} else if err != nil {
			return fmt.Errorf("read project instances for orphan diagnosis: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("project instance directory contains an unsafe entry")
			}
			if indexedIDs[entry.Name()] {
				continue
			}
			instance, readErr := r.readProjectInstance(entry.Name())
			if readErr != nil {
				return fmt.Errorf("diagnose orphan project instance: %w", readErr)
			}
			return fmt.Errorf("project instance %s has no root index", instance.ID)
		}
		sort.Slice(instances, func(left, right int) bool { return instances[left].Root < instances[right].Root })
		return nil
	})
	if err != nil {
		return nil, err
	}
	return instances, nil
}

// listProjectsForRuntimeProtection reads the complete Workspace collection
// without reconciling an interrupted project journal or creating a lock. A
// pending journal makes Runtime protection incomplete and therefore blocks
// retirement until the explicit Workspace recovery path completes.
func (r *Runtime) listProjectsForRuntimeProtection(ctx context.Context) ([]tobari.Workspace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(r.projectJournalPath()); err == nil {
		return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect project journal: %w", err)
	}
	var instances []tobari.Workspace
	err := r.withExistingProjectLock(ctx, func() error {
		var err error
		instances, err = r.listProjectsUnlocked()
		return err
	})
	return instances, err
}

func (r *Runtime) listProjectsUnlocked() ([]tobari.Workspace, error) {
	indexes, err := r.listRootIndexes()
	if err != nil {
		return nil, err
	}
	instances := make([]tobari.Workspace, 0, len(indexes))
	indexedIDs := make(map[string]bool, len(indexes))
	for _, index := range indexes {
		indexedIDs[index.InstanceID] = true
		directory, pathErr := r.projectDirectory(index.InstanceID)
		if pathErr != nil || requirePrivateDirectory(directory) != nil {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
		}
		instance, readErr := r.readProjectInstance(index.InstanceID)
		if errors.Is(readErr, os.ErrNotExist) {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
		}
		if readErr != nil {
			return nil, readErr
		}
		if instance.Root != index.Root {
			return nil, fmt.Errorf("root index and instance root disagree")
		}
		instances = append(instances, instance)
	}
	entries, _, err := readPrivateDirectoryIfPresent(r.instancesDirectory())
	if err != nil {
		return nil, fmt.Errorf("read project instances for orphan diagnosis: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
		}
		if err := requirePrivateDirectory(filepath.Join(r.instancesDirectory(), entry.Name())); err != nil {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
		}
		if indexedIDs[entry.Name()] {
			continue
		}
		return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
	}
	sort.Slice(instances, func(left, right int) bool { return instances[left].Root < instances[right].Root })
	return instances, nil
}

func (r *Runtime) withExistingProjectLock(ctx context.Context, action func() error) (resultErr error) {
	path := filepath.Join(r.stateDirectory, "project.lock")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return action()
	}
	if err != nil {
		return fmt.Errorf("inspect project lock: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
	}
	file, err := os.OpenFile(path, os.O_RDONLY, 0) // #nosec G304 -- validated existing fixed state child; observation never creates it.
	if err != nil {
		return fmt.Errorf("open project lock for Runtime protection: %w", err)
	}
	acquired := false
	defer func() {
		if acquired {
			unlockProjectFile(file)
		}
		if closeErr := file.Close(); resultErr == nil && closeErr != nil {
			resultErr = fmt.Errorf("close project observation lock: %w", closeErr)
		}
	}()
	for {
		locked, lockErr := tryLockProjectFile(file)
		if lockErr != nil {
			return fmt.Errorf("lock project state for Runtime protection: %w", lockErr)
		}
		if locked {
			acquired = true
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return action()
}

// UpdateProjectRuntime persists diagnostic resource identifiers while retaining
// the immutable logical identity and root binding.
func (r *Runtime) UpdateProjectRuntime(ctx context.Context, instance tobari.Workspace) error {
	if err := instance.Validate(); err != nil {
		return err
	}
	return r.withProjectLock(ctx, func() error {
		stored, err := r.readProjectInstance(instance.ID)
		if err != nil {
			return err
		}
		if stored.ID != instance.ID || stored.Root != instance.Root || stored.WorkspaceManifestID != instance.WorkspaceManifestID ||
			stored.WorkspaceManifestName != instance.WorkspaceManifestName || stored.Profile != instance.Profile || stored.Image != instance.Image ||
			stored.CreationApplied != instance.CreationApplied || stored.LastSuccessfulEntry != instance.LastSuccessfulEntry ||
			stored.LastFailure != instance.LastFailure {
			return fmt.Errorf("runtime update changes immutable logical Tobari state")
		}
		return r.writeProjectInstance(instance)
	})
}

func (r *Runtime) rootsDirectory() string { return filepath.Join(r.stateDirectory, "roots") }

func (r *Runtime) instancesDirectory() string { return filepath.Join(r.stateDirectory, "instances") }

func (r *Runtime) projectDirectory(id string) (string, error) {
	if err := tobari.ValidateWorkspaceID(id); err != nil {
		return "", err
	}
	return filepath.Join(r.instancesDirectory(), id), nil
}

func (r *Runtime) projectStatePath(id string) (string, error) {
	directory, err := r.projectDirectory(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "state.json"), nil
}

func (r *Runtime) projectHomePath(id string) string {
	return filepath.Join(r.instancesDirectory(), id, "home")
}

func (r *Runtime) rootIndexPath(root, contextID string) (string, error) {
	if err := tobari.ValidateCanonicalRoot(root); err != nil {
		return "", err
	}
	if err := tobari.ValidateWorkspaceManifestID(contextID); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(root + "\x00" + contextID))
	return filepath.Join(r.rootsDirectory(), hex.EncodeToString(digest[:])+".json"), nil
}

func (r *Runtime) listRootIndexes() ([]tobari.RootIndex, error) {
	directory := r.rootsDirectory()
	entries, present, err := readPrivateDirectoryIfPresent(directory)
	if !present && err == nil {
		return []tobari.RootIndex{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read root indexes: %w", err)
	}
	indexes := make([]tobari.RootIndex, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tobari-state-") {
			continue
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			return nil, fmt.Errorf("root index directory contains an unsafe entry")
		}
		var index tobari.RootIndex
		if err := readStrictJSON(filepath.Join(directory, entry.Name()), &index); err != nil {
			return nil, fmt.Errorf("read root index: %w", err)
		}
		if err := index.Validate(); err != nil {
			return nil, fmt.Errorf("validate root index: %w", err)
		}
		expectedPath, err := r.rootIndexPath(index.Root, index.WorkspaceManifestID)
		if err != nil || filepath.Base(expectedPath) != entry.Name() {
			return nil, fmt.Errorf("root index file name does not match canonical root")
		}
		indexes = append(indexes, index)
	}
	if err := tobari.ValidateRootIndexes(indexes); err != nil {
		return nil, fmt.Errorf("validate root index set: %w", err)
	}
	return indexes, nil
}

func (r *Runtime) readProjectInstance(id string) (tobari.Workspace, error) {
	path, err := r.projectStatePath(id)
	if err != nil {
		return tobari.Workspace{}, err
	}
	var instance tobari.Workspace
	if err := readStrictJSON(path, &instance); err != nil {
		return tobari.Workspace{}, fmt.Errorf("read instance state: %w", err)
	}
	if err := instance.Validate(); err != nil {
		return tobari.Workspace{}, fmt.Errorf("validate instance state: %w", err)
	}
	if instance.ID != id {
		return tobari.Workspace{}, fmt.Errorf("instance state ID does not match its directory")
	}
	return instance, nil
}

func (r *Runtime) writeRootIndex(index tobari.RootIndex) error {
	if err := index.Validate(); err != nil {
		return err
	}
	path, err := r.rootIndexPath(index.Root, index.WorkspaceManifestID)
	if err != nil {
		return err
	}
	return writeAtomicJSON(path, index)
}

func (r *Runtime) writeProjectInstance(instance tobari.Workspace) error {
	if err := instance.Validate(); err != nil {
		return err
	}
	if r.projectStateWriter != nil {
		return r.projectStateWriter(instance)
	}
	path, err := r.projectStatePath(instance.ID)
	if err != nil {
		return err
	}
	return writeAtomicJSON(path, instance)
}

func (r *Runtime) withProjectLock(ctx context.Context, action func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.ensurePrivateDirectory(r.stateDirectory); err != nil {
		return fmt.Errorf("prepare project state directory: %w", err)
	}
	path := filepath.Join(r.stateDirectory, "project.lock")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("project lock is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect project lock: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- fixed state child after lstat.
	if err != nil {
		return fmt.Errorf("open project lock: %w", err)
	}
	defer file.Close()
	for {
		acquired, lockErr := tryLockProjectFile(file)
		if lockErr != nil {
			return fmt.Errorf("lock project state: %w", lockErr)
		}
		if acquired {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	defer unlockProjectFile(file)
	return action()
}

func (r *Runtime) withProjectObservation(ctx context.Context, action func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(r.projectJournalPath()); err == nil {
		return r.withProjectLock(ctx, func() error {
			if err := r.reconcileProjectJournal(); err != nil {
				return err
			}
			return action()
		})
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect project journal: %w", err)
	}
	path := filepath.Join(r.stateDirectory, "project.lock")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return action()
	}
	if err != nil {
		return fmt.Errorf("inspect project lock: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("project lock is not a regular file")
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0) // #nosec G304 -- validated existing fixed state child; observation never creates it.
	if err != nil {
		return fmt.Errorf("open project lock: %w", err)
	}
	defer file.Close()
	for {
		acquired, lockErr := tryLockProjectFile(file)
		if lockErr != nil {
			return fmt.Errorf("lock project state: %w", lockErr)
		}
		if acquired {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	defer unlockProjectFile(file)
	if _, err := os.Lstat(r.projectJournalPath()); err == nil {
		if err := r.reconcileProjectJournal(); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect project journal: %w", err)
	}
	return action()
}

func (r *Runtime) ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path is not a regular directory")
	}
	if err := os.Chmod(path, 0o700); err != nil { // #nosec G302 -- runtime-owned directories are owner-only.
		return err
	}
	return requirePrivateDirectory(path)
}

func requirePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path is not a regular directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("path is not owner-only")
	}
	return nil
}

func readStrictJSON(path string, value any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxProjectStateBytes {
		return fmt.Errorf("state file is unsafe")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- caller derives a validated state child.
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("state contains trailing data")
	}
	return nil
}

func writeAtomicJSON(path string, value any) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(directory, ".tobari-state-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	directoryFile, err := os.Open(directory) // #nosec G304 -- directory is the parent of a runtime-owned state path.
	if err != nil {
		return err
	}
	defer directoryFile.Close()
	return directoryFile.Sync()
}
