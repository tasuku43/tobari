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
	SchemaVersion int    `json:"schema_version"`
	Operation     string `json:"operation"`
	ProjectID     string `json:"project_id"`
	Root          string `json:"root"`
	Phase         string `json:"phase"`
}

func (j projectJournal) Validate() error {
	if j.SchemaVersion != projectJournalSchema {
		return fmt.Errorf("project journal schema version must be %d", projectJournalSchema)
	}
	if j.Operation != projectOpCreate && j.Operation != projectOpDelete {
		return fmt.Errorf("project journal operation is invalid")
	}
	if err := tobari.ValidateProjectID(j.ProjectID); err != nil {
		return err
	}
	if err := tobari.ValidateCanonicalRoot(j.Root); err != nil {
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
func (r *Runtime) ResolveProject(ctx context.Context, cwd string) (tobari.ProjectInstance, bool, error) {
	resolved, err := r.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return tobari.ProjectInstance{}, false, err
	}
	var (
		instance tobari.ProjectInstance
		found    bool
	)
	err = r.withProjectLock(ctx, func() error {
		if err := r.reconcileProjectJournal(); err != nil {
			return err
		}
		var resolveErr error
		instance, found, resolveErr = r.resolveProjectUnlocked(resolved)
		return resolveErr
	})
	if err != nil {
		return tobari.ProjectInstance{}, false, err
	}
	return instance, found, nil
}

func (r *Runtime) resolveProjectUnlocked(cwd string) (tobari.ProjectInstance, bool, error) {
	indexes, err := r.listRootIndexes()
	if err != nil {
		return tobari.ProjectInstance{}, false, err
	}
	index, found, err := tobari.NearestRoot(cwd, indexes)
	if err != nil || !found {
		if err != nil {
			return tobari.ProjectInstance{}, false, err
		}
		return r.resolveOrphanInstance(cwd)
	}
	instance, err := r.readProjectInstance(index.InstanceID)
	if err == nil {
		if instance.Root != index.Root {
			return tobari.ProjectInstance{}, false, fmt.Errorf("root index and instance root disagree")
		}
		return instance, true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return tobari.ProjectInstance{}, false, err
	}
	// A root index contains enough immutable identity to make deletion
	// recoverable even when the instance file was lost after the index write.
	// The caller must still use the normal ownership checks before removing
	// Docker resources.
	return tobari.ProjectInstance{
		SchemaVersion: tobari.ProjectStateSchemaVersion,
		ID:            index.InstanceID,
		Root:          index.Root,
		Profile:       tobari.DefaultProfile,
		Image:         tobari.BuiltinImageSelector,
	}, true, nil
}

func (r *Runtime) resolveOrphanInstance(cwd string) (tobari.ProjectInstance, bool, error) {
	entries, err := os.ReadDir(r.instancesDirectory())
	if errors.Is(err, os.ErrNotExist) {
		return tobari.ProjectInstance{}, false, nil
	}
	if err != nil {
		return tobari.ProjectInstance{}, false, fmt.Errorf("read project instances: %w", err)
	}
	instances := make([]tobari.ProjectInstance, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return tobari.ProjectInstance{}, false, fmt.Errorf("project instance directory contains an unsafe entry")
		}
		instance, readErr := r.readProjectInstance(entry.Name())
		if readErr != nil {
			return tobari.ProjectInstance{}, false, readErr
		}
		instances = append(instances, instance)
	}
	if len(instances) == 0 {
		return tobari.ProjectInstance{}, false, nil
	}
	roots := make([]tobari.RootIndex, 0, len(instances))
	byRoot := make(map[string]tobari.ProjectInstance, len(instances))
	for _, instance := range instances {
		roots = append(roots, tobari.RootIndex{
			SchemaVersion: tobari.ProjectStateSchemaVersion,
			Root:          instance.Root,
			InstanceID:    instance.ID,
		})
		byRoot[instance.Root] = instance
	}
	index, found, err := tobari.NearestRoot(cwd, roots)
	if err != nil || !found {
		return tobari.ProjectInstance{}, found, err
	}
	return byRoot[index.Root], true, nil
}

// ResolveOrCreateProject returns the nearest logical Tobari. When none covers
// cwd, it creates exactly one durable root index, instance record, and home
// before any Docker resource is created.
func (r *Runtime) ResolveOrCreateProject(
	ctx context.Context, cwd string,
) (tobari.ProjectInstance, bool, error) {
	resolved, err := r.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return tobari.ProjectInstance{}, false, err
	}
	var (
		instance tobari.ProjectInstance
		created  bool
	)
	err = r.withProjectLock(ctx, func() error {
		if err := r.reconcileProjectJournal(); err != nil {
			return err
		}
		indexes, loadErr := r.listRootIndexes()
		if loadErr != nil {
			return loadErr
		}
		index, found, nearestErr := tobari.NearestRoot(resolved, indexes)
		if nearestErr != nil {
			return nearestErr
		}
		if found {
			loaded, readErr := r.readProjectInstance(index.InstanceID)
			if readErr != nil {
				return readErr
			}
			if loaded.Root != index.Root {
				return fmt.Errorf("root index and instance root disagree")
			}
			instance = loaded
			return nil
		}
		image, imageErr := r.resolveProjectImage(ctx, resolved)
		if imageErr != nil {
			return imageErr
		}
		createdInstance, createErr := tobari.NewProductionProjectInstance(resolved, image)
		if createErr != nil {
			return createErr
		}
		journal := projectJournal{
			SchemaVersion: projectJournalSchema, Operation: projectOpCreate,
			ProjectID: createdInstance.ID, Root: createdInstance.Root, Phase: projectPhaseStarted,
		}
		if err := r.writeProjectJournal(journal); err != nil {
			return err
		}
		if err := r.ensurePrivateDirectory(r.projectHomePath(createdInstance.ID)); err != nil {
			return r.discardUnindexedProject(createdInstance, fmt.Errorf("create project home: %w", err))
		}
		journal.Phase = projectPhaseHome
		if err := r.writeProjectJournal(journal); err != nil {
			return err
		}
		if err := r.writeProjectInstance(createdInstance); err != nil {
			return r.discardUnindexedProject(createdInstance, err)
		}
		journal.Phase = projectPhaseState
		if err := r.writeProjectJournal(journal); err != nil {
			return err
		}
		if err := r.writeRootIndex(tobari.RootIndex{
			SchemaVersion: tobari.ProjectStateSchemaVersion,
			Root:          createdInstance.Root,
			InstanceID:    createdInstance.ID,
		}); err != nil {
			return r.discardUnindexedProject(createdInstance, err)
		}
		journal.Phase = projectPhaseIndex
		if err := r.clearProjectJournal(); err != nil {
			return err
		}
		instance, created = createdInstance, true
		return nil
	})
	if err != nil {
		return tobari.ProjectInstance{}, false, err
	}
	return instance, created, nil
}

func (r *Runtime) discardUnindexedProject(instance tobari.ProjectInstance, cause error) error {
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
		indexPath, pathErr := r.rootIndexPath(journal.Root)
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
		indexPath, pathErr := r.rootIndexPath(journal.Root)
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

func (r *Runtime) resolveProjectImage(ctx context.Context, root string) (string, error) {
	image, err := r.ResolveImageSelector(ctx, "")
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, ".devcontainer", "devcontainer.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return image, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect project Dev Container file: %w", err)
	}
	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("project Dev Container path is unsafe")
	}
	config, err := r.ReadDevContainer(ctx, root, path)
	if err != nil {
		return "", err
	}
	if unsupported := config.UnsupportedProperties(); len(unsupported) != 0 {
		return "", fmt.Errorf("project Dev Container contains unsupported properties: %s", strings.Join(unsupported, ", "))
	}
	if err := config.Validate(); err != nil {
		return "", err
	}
	return config.Image, nil
}

// ListProjects returns every valid logical Tobari record ordered by root.
func (r *Runtime) ListProjects(ctx context.Context) ([]tobari.ProjectInstance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	indexes, err := r.listRootIndexes()
	if err != nil {
		return nil, err
	}
	instances := make([]tobari.ProjectInstance, 0, len(indexes))
	indexedIDs := make(map[string]bool, len(indexes))
	for _, index := range indexes {
		indexedIDs[index.InstanceID] = true
		instance, readErr := r.readProjectInstance(index.InstanceID)
		if readErr != nil {
			return nil, readErr
		}
		if instance.Root != index.Root {
			return nil, fmt.Errorf("root index and instance root disagree")
		}
		instances = append(instances, instance)
	}
	entries, err := os.ReadDir(r.instancesDirectory())
	if errors.Is(err, os.ErrNotExist) {
		entries = nil
	} else if err != nil {
		return nil, fmt.Errorf("read project instances for orphan diagnosis: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("project instance directory contains an unsafe entry")
		}
		if indexedIDs[entry.Name()] {
			continue
		}
		instance, readErr := r.readProjectInstance(entry.Name())
		if readErr != nil {
			return nil, fmt.Errorf("diagnose orphan project instance: %w", readErr)
		}
		return nil, fmt.Errorf("project instance %s has no root index", instance.ID)
	}
	sort.Slice(instances, func(left, right int) bool { return instances[left].Root < instances[right].Root })
	return instances, nil
}

// UpdateProjectRuntime persists diagnostic resource identifiers while retaining
// the immutable logical identity and root binding.
func (r *Runtime) UpdateProjectRuntime(ctx context.Context, instance tobari.ProjectInstance) error {
	if err := instance.Validate(); err != nil {
		return err
	}
	return r.withProjectLock(ctx, func() error {
		stored, err := r.readProjectInstance(instance.ID)
		if err != nil {
			return err
		}
		if stored.ID != instance.ID || stored.Root != instance.Root || stored.Profile != instance.Profile || stored.Image != instance.Image {
			return fmt.Errorf("runtime update changes immutable logical Tobari state")
		}
		return r.writeProjectInstance(instance)
	})
}

func (r *Runtime) rootsDirectory() string { return filepath.Join(r.stateDirectory, "roots") }

func (r *Runtime) instancesDirectory() string { return filepath.Join(r.stateDirectory, "instances") }

func (r *Runtime) projectDirectory(id string) (string, error) {
	if err := tobari.ValidateProjectID(id); err != nil {
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

func (r *Runtime) rootIndexPath(root string) (string, error) {
	if err := tobari.ValidateCanonicalRoot(root); err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(root))
	return filepath.Join(r.rootsDirectory(), hex.EncodeToString(digest[:])+".json"), nil
}

func (r *Runtime) listRootIndexes() ([]tobari.RootIndex, error) {
	directory := r.rootsDirectory()
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
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
		expectedPath, err := r.rootIndexPath(index.Root)
		if err != nil || filepath.Base(expectedPath) != entry.Name() {
			return nil, fmt.Errorf("root index file name does not match canonical root")
		}
		indexes = append(indexes, index)
	}
	return indexes, nil
}

func (r *Runtime) readProjectInstance(id string) (tobari.ProjectInstance, error) {
	path, err := r.projectStatePath(id)
	if err != nil {
		return tobari.ProjectInstance{}, err
	}
	var instance tobari.ProjectInstance
	if err := readStrictJSON(path, &instance); err != nil {
		return tobari.ProjectInstance{}, fmt.Errorf("read instance state: %w", err)
	}
	if err := instance.Validate(); err != nil {
		return tobari.ProjectInstance{}, fmt.Errorf("validate instance state: %w", err)
	}
	if instance.ID != id {
		return tobari.ProjectInstance{}, fmt.Errorf("instance state ID does not match its directory")
	}
	return instance, nil
}

func (r *Runtime) writeRootIndex(index tobari.RootIndex) error {
	if err := index.Validate(); err != nil {
		return err
	}
	path, err := r.rootIndexPath(index.Root)
	if err != nil {
		return err
	}
	return writeAtomicJSON(path, index)
}

func (r *Runtime) writeProjectInstance(instance tobari.ProjectInstance) error {
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

func (r *Runtime) ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path is not a directory")
	}
	return os.Chmod(path, 0o700) // #nosec G302 -- project state is owner-only.
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
