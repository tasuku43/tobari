package dockerruntime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	maxRuntimeLifecycleRuntimes   = 256
	maxRuntimeLifecycleMaterials  = 1024
	maxRuntimeContainersPerImage  = 256
	maxRuntimeLifecycleContainers = 1024
	maxRuntimeLifecycleListBytes  = 64 * 1024
	maxRuntimeLifecycleTags       = 256
	maxRuntimeLifecycleInspect    = 64 * 1024
	runtimeLifecycleCallBudget    = 6000
	runtimeLifecycleWallBudget    = 30 * time.Second
)

var runtimeLifecycleContainerID = regexp.MustCompile(`^[0-9a-f]{64}$`)

type runtimeLifecycleLocalObservation struct {
	Runtimes        []tobari.RuntimeManifest
	Protection      runtimeProtectionObservation
	Build           *runtimeBuildJournal
	Prune           *runtimePruneJournal
	Delete          *runtimeDeleteJournal
	DeleteProjected bool
	Receipts        runtimePruneReceiptStore
	Storage         []tobari.RuntimeStorageObservation
}

type runtimeImageObservation struct {
	ID        string   `json:"id"`
	Size      int64    `json:"size"`
	RepoTags  []string `json:"repo_tags"`
	Owner     string   `json:"owner"`
	Component string   `json:"component"`
	RuntimeID string   `json:"runtime_id"`
	Revision  string   `json:"revision"`
}

type runtimeContainerObservation struct {
	ID        string `json:"id"`
	Image     string `json:"image"`
	Owner     string `json:"owner"`
	Component string `json:"component"`
	Workspace string `json:"workspace"`
	Role      string `json:"role"`
	Spec      string `json:"spec"`
}

type runtimeContentUse struct{ workspace, external bool }

type runtimeLifecycleBudget struct {
	remaining int
}

func (b *runtimeLifecycleBudget) run(ctx context.Context, runner commandRunner, args, environment []string, limit int) ([]byte, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if b == nil || b.remaining <= 0 {
		return nil, nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	b.remaining--
	stdout := &boundedBuffer{limit: limit}
	stderr := &boundedBuffer{limit: limit}
	err := runner.Run(ctx, args, environment, nil, stdout, stderr)
	if stdout.overflow || stderr.overflow {
		return nil, nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	return append([]byte(nil), stdout.buffer.Bytes()...), append([]byte(nil), stderr.buffer.Bytes()...), err
}

type runtimeMaterialTarget struct {
	RuntimeID      string
	Revision       string
	TagRole        tobari.RuntimeMaterialTagRole
	Selector       string
	RecordedDigest string
	Name           string
}

// ReadRuntimeLifecycleSnapshot proves a complete local catalog/protection/
// journal join under the non-creating lifecycle read lock. Docker cannot be
// locked, so its bounded evidence and the local authority are both observed
// before and after and any drift rejects the snapshot.
func (r *Runtime) ReadRuntimeLifecycleSnapshot(ctx context.Context) (tobari.RuntimeLifecycleSnapshot, time.Time, error) {
	var result tobari.RuntimeLifecycleSnapshot
	var observedAt time.Time
	err := r.withLifecycleObservation(ctx, func(lockContext context.Context) error {
		var err error
		result, observedAt, err = r.readRuntimeLifecycleSnapshotLocked(lockContext)
		return err
	})
	if err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, err
	}
	if err := result.Validate(); err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, fmt.Errorf("validate Runtime lifecycle snapshot (runtimes=%d materials=%d): %w", len(result.Runtimes), len(result.Materials), err)
	}
	return result, observedAt, nil
}

// readRuntimeLifecycleSnapshotLocked requires the installation lifecycle lock
// and performs the same coherent, bounded observation used by dry-run. A
// mutation caller may therefore revalidate the plan without reacquiring the
// non-reentrant lock.
func (r *Runtime) readRuntimeLifecycleSnapshotLocked(lockContext context.Context) (tobari.RuntimeLifecycleSnapshot, time.Time, error) {
	budget := runtimeLifecycleBudget{remaining: runtimeLifecycleCallBudget}
	return r.readRuntimeLifecycleSnapshotLockedWithBudget(lockContext, &budget)
}

func (r *Runtime) readRuntimeLifecycleSnapshotLockedWithBudget(lockContext context.Context, budget *runtimeLifecycleBudget) (tobari.RuntimeLifecycleSnapshot, time.Time, error) {
	observationContext, cancel := context.WithTimeout(lockContext, runtimeLifecycleWallBudget)
	defer cancel()
	before, err := r.readRuntimeLifecycleLocalObserved(observationContext, budget)
	if err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, fmt.Errorf("read first Runtime lifecycle authority: %w", err)
	}
	beforeSnapshot, err := r.observeRuntimeLifecycleDocker(observationContext, before, budget)
	if err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, fmt.Errorf("observe first Runtime lifecycle Docker state: %w", err)
	}
	after, err := r.readRuntimeLifecycleLocalObserved(observationContext, budget)
	if err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, fmt.Errorf("read second Runtime lifecycle authority: %w", err)
	}
	afterSnapshot, err := r.observeRuntimeLifecycleDocker(observationContext, after, budget)
	if err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, fmt.Errorf("observe second Runtime lifecycle Docker state: %w", err)
	}
	beforeToken, err := runtimeLifecycleToken(before, beforeSnapshot)
	if err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, err
	}
	afterToken, err := runtimeLifecycleToken(after, afterSnapshot)
	if err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, err
	}
	if beforeToken != afterToken {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	if err := beforeSnapshot.Validate(); err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, err
	}
	return beforeSnapshot, time.Now().UTC(), nil
}

// ReadRuntimeBuildRecovery observes one active build/restore journal without
// creating state. Failed attempts use bounded Docker observation before
// becoming actionable. Restore recovery additionally returns the exact
// revision reference accepted by the separate review-confirmed mutation.
func (r *Runtime) ReadRuntimeBuildRecovery(ctx context.Context) (tobari.RuntimeBuildRecovery, bool, error) {
	observationContext, cancel := r.runtimeBuildRecoveryContext(ctx)
	defer cancel()
	var result tobari.RuntimeBuildRecovery
	found := false
	err := r.withLifecycleObservation(observationContext, func(lockContext context.Context) error {
		journal, err := r.readStrictRuntimeBuildJournalInventory()
		if err != nil || journal == nil {
			return err
		}
		runtimes, err := r.readStrictRuntimeCatalogObserved(journal)
		if err != nil {
			return err
		}
		managed := false
		for _, manifest := range runtimes {
			if manifest.ID == journal.RuntimeID && manifest.Name == journal.RuntimeName && manifest.Kind == tobari.RuntimeKindManaged {
				managed = true
				break
			}
		}
		if !managed {
			return fmt.Errorf("Runtime build recovery lacks current managed Runtime authority")
		}
		kind := tobari.RuntimeBuildRecoveryFailed
		switch journal.Phase {
		case runtimeBuildPhaseSnapshotting, runtimeBuildPhasePrepared:
			kind = tobari.RuntimeBuildRecoveryPreDocker
		case runtimeBuildPhaseBuilding:
			kind = tobari.RuntimeBuildRecoveryBuilding
		case runtimeBuildPhaseBuilt, runtimeBuildPhaseFinalTagged, runtimeBuildPhaseStagingReleased, runtimeBuildPhaseSnapshotPublished, runtimeBuildPhaseManifestCommitted:
			kind = tobari.RuntimeBuildRecoveryPublication
		case runtimeBuildPhaseCompleting:
			kind = tobari.RuntimeBuildRecoveryCleanup
		case runtimeBuildPhaseOrphanStaging:
			kind = tobari.RuntimeBuildRecoveryOrphan
		case runtimeBuildPhaseFailed:
			if _, err := r.observeRuntimeFailedAttempt(observationContext, *journal); err != nil {
				return err
			}
			kind = tobari.RuntimeBuildRecoveryFailed
		default:
			return fmt.Errorf("Runtime build recovery phase is invalid")
		}
		result = tobari.RuntimeBuildRecovery{RuntimeID: journal.RuntimeID, RuntimeRef: tobari.RuntimeRef(journal.RuntimeID), Name: journal.RuntimeName, Kind: kind}
		if journal.Restore {
			result.RevisionRef = tobari.RuntimeRevisionRef(journal.RuntimeID, journal.Revision)
			result.RestoreFailed = journal.Phase == runtimeBuildPhaseFailed ||
				(journal.Phase == runtimeBuildPhaseCompleting && journal.CleanupFrom == runtimeBuildPhaseFailed)
		}
		if err := result.Validate(); err != nil {
			return err
		}
		found = true
		return lockContext.Err()
	})
	if err != nil {
		return tobari.RuntimeBuildRecovery{}, false, err
	}
	return result, found, nil
}

func runtimeLifecycleToken(local runtimeLifecycleLocalObservation, snapshot tobari.RuntimeLifecycleSnapshot) ([sha256.Size]byte, error) {
	encoded, err := json.Marshal(struct {
		Local    runtimeLifecycleLocalObservation `json:"local"`
		Snapshot tobari.RuntimeLifecycleSnapshot  `json:"snapshot"`
	}{Local: local, Snapshot: snapshot})
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}

func (r *Runtime) readRuntimeLifecycleLocalObserved(ctx context.Context, budget *runtimeLifecycleBudget) (runtimeLifecycleLocalObservation, error) {
	if err := ctx.Err(); err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	journal, err := r.readStrictRuntimeBuildJournalInventory()
	if err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	prune, err := r.readRuntimePruneJournalObserved()
	if err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	deletion, err := r.readRuntimeDeleteJournalObserved()
	if err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	receipts, err := r.readRuntimePruneReceiptStoreObserved()
	if err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	runtimes, err := r.readStrictRuntimeCatalogObserved(journal)
	if err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	deleteProjected := false
	if deletion != nil {
		found := false
		for _, manifest := range runtimes {
			if manifest.ID == deletion.Target.Runtime.ID {
				found = true
				break
			}
		}
		if !found {
			// After the atomic catalog-to-quarantine rename, the exact journal
			// target remains task-owned retiring authority. Project it only while
			// the delete journal is active; this is observation and never
			// reactivates the Runtime catalog entry.
			runtimes = append(runtimes, deletion.Target.Runtime)
			deleteProjected = true
		}
	}
	protection, err := r.readRuntimeProtectionInventoryObserved(ctx, budget)
	if err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	storageRuntimes := runtimes
	if deleteProjected {
		storageRuntimes = make([]tobari.RuntimeManifest, 0, len(runtimes)-1)
		for _, manifest := range runtimes {
			if manifest.ID != deletion.Target.Runtime.ID {
				storageRuntimes = append(storageRuntimes, manifest)
			}
		}
	}
	storage, err := r.observeRuntimeStorage(ctx, storageRuntimes, journal)
	if err != nil {
		return runtimeLifecycleLocalObservation{}, err
	}
	if deleteProjected {
		storage = append(storage, deletion.Target.Storage)
		sort.Slice(storage, func(i, j int) bool { return storage[i].RuntimeID < storage[j].RuntimeID })
	}
	return runtimeLifecycleLocalObservation{Runtimes: runtimes, Protection: protection, Build: journal, Prune: prune, Delete: deletion, DeleteProjected: deleteProjected, Receipts: receipts, Storage: storage}, nil
}

type runtimeLogicalFile struct {
	path string
	mode os.FileMode
	size int64
	info os.FileInfo
}

func (r *Runtime) observeRuntimeStorage(ctx context.Context, runtimes []tobari.RuntimeManifest, journal *runtimeBuildJournal) ([]tobari.RuntimeStorageObservation, error) {
	result := make([]tobari.RuntimeStorageObservation, 0, len(runtimes))
	for _, manifest := range runtimes {
		if manifest.Kind != tobari.RuntimeKindManaged {
			continue
		}
		sourceBytes, err := observeRuntimeTreeLogicalBytes(ctx, manifest.SourcePath)
		if err != nil {
			return nil, fmt.Errorf("observe managed Runtime source storage: %w", err)
		}
		storage := tobari.RuntimeStorageObservation{RuntimeID: manifest.ID, Name: manifest.Name, SourceLogicalBytes: sourceBytes, Snapshots: []tobari.RuntimeSnapshotStorage{}}
		for _, revision := range manifest.Revisions {
			fingerprint, logicalBytes, err := observeImmutableRuntimeSnapshot(ctx, revision.SnapshotPath, revision.Revision)
			if err != nil {
				return nil, fmt.Errorf("observe immutable Runtime snapshot storage: %w", err)
			}
			storage.Snapshots = append(storage.Snapshots, tobari.RuntimeSnapshotStorage{Kind: tobari.RuntimePruneCandidateRevision, Revision: revision.Revision, SemanticFingerprint: fingerprint, LogicalBytes: logicalBytes})
		}
		if journal != nil && !journal.Restore && journal.RuntimeID == manifest.ID && journal.Phase == runtimeBuildPhaseFailed && journal.StagingArtifact == runtimeBuildStagingOwned && journal.AttemptSettlement == runtimeBuildAttemptSettled {
			fingerprint, logicalBytes, err := observeImmutableRuntimeSnapshot(ctx, journal.SnapshotPath, journal.Revision)
			if err != nil {
				return nil, fmt.Errorf("observe failed Runtime build snapshot storage: %w", err)
			}
			storage.Snapshots = append(storage.Snapshots, tobari.RuntimeSnapshotStorage{Kind: tobari.RuntimePruneCandidateFailedBuild, Revision: journal.Revision, SemanticFingerprint: fingerprint, LogicalBytes: logicalBytes})
		}
		sort.Slice(storage.Snapshots, func(i, j int) bool {
			return string(storage.Snapshots[i].Kind)+"\x00"+storage.Snapshots[i].Revision < string(storage.Snapshots[j].Kind)+"\x00"+storage.Snapshots[j].Revision
		})
		if err := storage.Validate(); err != nil {
			return nil, err
		}
		result = append(result, storage)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].RuntimeID < result[j].RuntimeID })
	return result, nil
}

func observeImmutableRuntimeSnapshot(ctx context.Context, root, expected string) (string, int64, error) {
	fingerprint, err := digestRuntimeSnapshot(ctx, root)
	if err != nil || fingerprint != expected {
		return "", 0, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
	}
	logicalBytes, err := observeRuntimeTreeLogicalBytes(ctx, root)
	if err != nil {
		return "", 0, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
	}
	return fingerprint, logicalBytes, nil
}

func observeRuntimeTreeLogicalBytes(ctx context.Context, root string) (int64, error) {
	first, firstBytes, err := readRuntimeLogicalTree(ctx, root)
	if err != nil {
		return 0, err
	}
	second, secondBytes, err := readRuntimeLogicalTree(ctx, root)
	if err != nil {
		return 0, err
	}
	if firstBytes != secondBytes || len(first) != len(second) {
		return 0, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	for index := range first {
		if first[index].path != second[index].path || first[index].mode != second[index].mode || first[index].size != second[index].size || !os.SameFile(first[index].info, second[index].info) {
			return 0, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
	}
	return firstBytes, nil
}

func readRuntimeLogicalTree(ctx context.Context, root string) ([]runtimeLogicalFile, int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if err := requirePrivateDirectory(root); err != nil {
		return nil, 0, err
	}
	entries := make([]runtimeLogicalFile, 0)
	directories := 0
	total := int64(0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
				return fmt.Errorf("Runtime storage root is unsafe")
			}
			entries = append(entries, runtimeLogicalFile{path: ".", mode: info.Mode(), info: info})
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == "." || filepath.Clean(relative) != relative || filepath.IsAbs(relative) {
			return fmt.Errorf("Runtime storage contains a non-canonical child")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("Runtime storage contains unsafe ownership or link evidence")
		}
		if entry.IsDir() {
			directories++
			if directories > maxRuntimeSourceDirs {
				return fmt.Errorf("Runtime storage directory inventory exceeds the bound")
			}
			entries = append(entries, runtimeLogicalFile{path: filepath.ToSlash(relative), mode: info.Mode(), info: info})
			return nil
		}
		if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxRuntimeSourceFile || len(entries)-directories-1 >= maxRuntimeSourceFiles {
			return fmt.Errorf("Runtime storage file inventory is invalid")
		}
		total += info.Size()
		if total > maxRuntimeSourceTotal {
			return fmt.Errorf("Runtime storage exceeds the source bound")
		}
		entries = append(entries, runtimeLogicalFile{path: filepath.ToSlash(relative), mode: info.Mode(), size: info.Size(), info: info})
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	if len(entries) == directories+1 {
		return nil, 0, fmt.Errorf("Runtime storage tree is empty")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	return entries, total, nil
}

func (r *Runtime) readStrictRuntimeBuildJournalInventory() (*runtimeBuildJournal, error) {
	if err := r.requireNoInstallationRuntimeMigration(); err != nil {
		return nil, err
	}
	directory := r.runtimeLifecycleDirectory()
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := requirePrivateDirectory(directory); err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || (entry.Name() != runtimeBuildJournalFile && entry.Name() != runtimeBuildSnapshotDir && entry.Name() != runtimePruneJournalFile && entry.Name() != runtimePruneReceiptsFile && entry.Name() != runtimeDeleteJournalFile && entry.Name() != runtimeDeleteReceiptsDir && entry.Name() != runtimeCreateJournalFile) {
			return nil, fmt.Errorf("Runtime lifecycle journal inventory contains an unknown entry")
		}
	}
	if create, err := r.readRuntimeCreateJournal(); err != nil {
		return nil, err
	} else if create != nil {
		return nil, fmt.Errorf("Runtime create transaction requires recovery")
	}
	journal, err := r.readRuntimeBuildJournalObserved()
	if err != nil {
		return nil, err
	}
	if journal == nil {
		for _, entry := range entries {
			if entry.Name() == runtimeBuildSnapshotDir {
				return nil, fmt.Errorf("Runtime lifecycle staging state lacks journal authority")
			}
		}
		return nil, nil
	}
	snapshotRoot := filepath.Dir(journal.SnapshotPath)
	if info, err := os.Lstat(snapshotRoot); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("Runtime build staging snapshot is unsafe")
		}
		children, err := os.ReadDir(snapshotRoot)
		if err != nil {
			return nil, fmt.Errorf("Runtime build staging snapshot inventory is incomplete")
		}
		partialSnapshot := journal.Phase == runtimeBuildPhaseSnapshotting || (journal.Phase == runtimeBuildPhaseCompleting && journal.CleanupFrom == runtimeBuildPhaseSnapshotting)
		if partialSnapshot && len(children) == 0 {
			return journal, nil
		}
		if len(children) != 1 || children[0].Name() != "source" || !children[0].IsDir() || children[0].Type()&os.ModeSymlink != 0 || requirePrivateDirectory(journal.SnapshotPath) != nil {
			return nil, fmt.Errorf("Runtime build staging snapshot inventory is incomplete")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else if journal.Phase != runtimeBuildPhaseSnapshotting && journal.Phase != runtimeBuildPhaseSnapshotPublished && journal.Phase != runtimeBuildPhaseManifestCommitted && journal.Phase != runtimeBuildPhaseCompleting {
		return nil, fmt.Errorf("Runtime build journal lacks its staging snapshot")
	}
	return journal, nil
}

func (r *Runtime) readStrictRuntimeCatalogObserved(journal *runtimeBuildJournal) ([]tobari.RuntimeManifest, error) {
	standard, err := r.standardRuntimeManifest()
	if err != nil {
		return nil, err
	}
	result := []tobari.RuntimeManifest{standard}
	entries, err := os.ReadDir(r.runtimesDirectory())
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if len(entries) > maxRuntimeLifecycleRuntimes {
		return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	materialCount := 0
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return nil, fmt.Errorf("Runtime catalog contains an unknown child")
		}
		manifest, err := r.readRuntimeManifestByID(entry.Name())
		if err != nil {
			return nil, err
		}
		if err := r.validateStrictRuntimeDirectory(manifest, journal); err != nil {
			return nil, err
		}
		materialCount += len(manifest.Revisions)
		if materialCount > maxRuntimeLifecycleMaterials {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		result = append(result, manifest)
	}
	sort.Slice(result[1:], func(i, j int) bool { return result[i+1].Name < result[j+1].Name })
	return result, nil
}

func (r *Runtime) validateStrictRuntimeDirectory(manifest tobari.RuntimeManifest, journal *runtimeBuildJournal) error {
	entries, err := os.ReadDir(r.runtimeDirectory(manifest.ID))
	if err != nil {
		return err
	}
	want := map[string]bool{"runtime.yaml": true, "source": true}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !want[entry.Name()] {
			return fmt.Errorf("Runtime directory contains an unknown child")
		}
		delete(want, entry.Name())
	}
	if len(want) != 0 {
		return fmt.Errorf("Runtime directory inventory is incomplete")
	}
	stateEntries, err := os.ReadDir(r.runtimeStateDirectory(manifest.ID))
	if err != nil {
		return err
	}
	wantState := map[string]bool{"runtime.json": true, "revisions": true}
	for _, entry := range stateEntries {
		if entry.Type()&os.ModeSymlink != 0 || !wantState[entry.Name()] {
			return fmt.Errorf("Runtime state contains an unknown child")
		}
		delete(wantState, entry.Name())
	}
	if len(wantState) != 0 {
		return fmt.Errorf("Runtime state inventory is incomplete")
	}
	revisionEntries, err := os.ReadDir(r.runtimeRevisionsDirectory(manifest.ID))
	if err != nil {
		return err
	}
	wantRevisions := make(map[string]bool, len(manifest.Revisions)+1)
	for _, revision := range manifest.Revisions {
		if revision.Image != managedLibraryRuntimeImage(manifest.Name, manifest.ID, revision.Revision) {
			return fmt.Errorf("Runtime revision image selector is not canonical")
		}
		wantRevisions[strings.TrimPrefix(revision.Revision, "sha256:")] = true
	}
	if journal != nil && journal.Phase == runtimeBuildPhaseSnapshotPublished && journal.RuntimeID == manifest.ID && journal.RuntimeName == manifest.Name && journal.Revision != "" {
		wantRevisions[strings.TrimPrefix(journal.Revision, "sha256:")] = true
	}
	for _, entry := range revisionEntries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() || !wantRevisions[entry.Name()] {
			return fmt.Errorf("Runtime revision store contains an unknown child")
		}
		children, err := os.ReadDir(filepath.Join(r.runtimeRevisionsDirectory(manifest.ID), entry.Name()))
		if err != nil || len(children) != 1 || children[0].Name() != "source" || !children[0].IsDir() || children[0].Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Runtime revision snapshot inventory is incomplete")
		}
		delete(wantRevisions, entry.Name())
	}
	if len(wantRevisions) != 0 {
		return fmt.Errorf("Runtime revision snapshot inventory is incomplete")
	}
	return nil
}

func (r *Runtime) observeRuntimeLifecycleDocker(ctx context.Context, local runtimeLifecycleLocalObservation, budget *runtimeLifecycleBudget) (tobari.RuntimeLifecycleSnapshot, error) {
	targets := make([]runtimeMaterialTarget, 0)
	for _, runtime := range local.Runtimes {
		if runtime.Kind != tobari.RuntimeKindManaged {
			continue
		}
		for _, revision := range runtime.Revisions {
			targets = append(targets, runtimeMaterialTarget{RuntimeID: runtime.ID, Revision: revision.Revision, TagRole: tobari.RuntimeMaterialTagPublishedRevision, Selector: managedLibraryRuntimeImage(runtime.Name, runtime.ID, revision.Revision), RecordedDigest: revision.ImageDigest, Name: runtime.Name})
		}
	}
	journalInventory := tobari.RuntimeLifecycleJournals{Complete: true, Active: []tobari.RuntimeLifecycleActivity{}, FailedBuilds: []tobari.RuntimeFailedBuildArtifact{}}
	if local.Build != nil {
		if !local.Build.Restore && local.Build.Phase == runtimeBuildPhaseFailed && local.Build.StagingArtifact == runtimeBuildStagingOwned && local.Build.AttemptSettlement == runtimeBuildAttemptSettled {
			targets = append(targets, runtimeMaterialTarget{RuntimeID: local.Build.RuntimeID, Revision: local.Build.Revision, TagRole: tobari.RuntimeMaterialTagJournaledStaging, Selector: local.Build.StagingImage, RecordedDigest: local.Build.ImageDigest, Name: local.Build.RuntimeName})
		} else {
			journalInventory.Active = append(journalInventory.Active, runtimeLifecycleActivityFromBuild(*local.Build))
		}
	}
	if local.DeleteProjected {
		for _, target := range local.Delete.Target.Materials {
			if target.Candidate.Kind != tobari.RuntimePruneCandidateFailedBuild || (local.Build != nil && local.Build.RuntimeID == target.Candidate.RuntimeID && local.Build.Revision == target.Candidate.Revision) {
				continue
			}
			targets = append(targets, runtimeMaterialTarget{RuntimeID: target.Candidate.RuntimeID, Revision: target.Candidate.Revision, TagRole: tobari.RuntimeMaterialTagJournaledStaging, Selector: managedRuntimeStagingImage(target.Candidate.RuntimeID, target.Candidate.Revision), Name: target.Candidate.Name})
		}
	}
	if local.Prune != nil {
		journalInventory.Active = append(journalInventory.Active, local.Prune.activities()...)
	}
	if local.Delete != nil {
		journalInventory.Active = append(journalInventory.Active, local.Delete.activity())
	}
	if len(targets) > maxRuntimeLifecycleMaterials {
		return tobari.RuntimeLifecycleSnapshot{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	materials, err := r.observeRuntimeMaterials(ctx, targets, local.Protection.Containers, budget)
	if err != nil {
		return tobari.RuntimeLifecycleSnapshot{}, err
	}
	for index := range materials {
		if materials[index].Availability == tobari.RuntimeAvailabilityMissing && runtimePruneReceiptRetired(local.Receipts, targets[index]) && !runtimeRestoreCommittedButNotSuperseded(local.Build, targets[index]) {
			materials[index].Availability = tobari.RuntimeAvailabilityPruned
			if err := materials[index].Validate(); err != nil {
				return tobari.RuntimeLifecycleSnapshot{}, err
			}
		}
	}
	successful := make([]tobari.RuntimeMaterialObservation, 0, len(materials))
	for index, material := range materials {
		if targets[index].TagRole == tobari.RuntimeMaterialTagJournaledStaging {
			journalInventory.FailedBuilds = append(journalInventory.FailedBuilds, tobari.RuntimeFailedBuildArtifact{RuntimeID: targets[index].RuntimeID, Revision: targets[index].Revision, RuntimeRef: tobari.RuntimeRef(targets[index].RuntimeID), Name: targets[index].Name, Material: material})
		} else {
			successful = append(successful, material)
		}
	}
	sortRuntimeLifecycleJournals(&journalInventory)
	generations := runtimeRetirementGenerations(local.Receipts, targets)
	return tobari.RuntimeLifecycleSnapshot{CatalogComplete: true, Runtimes: local.Runtimes, Protection: local.Protection.Inventory, Materials: successful, Storage: local.Storage, Journals: journalInventory, RetirementGenerations: generations}, nil
}

func runtimePruneReceiptRetired(store runtimePruneReceiptStore, target runtimeMaterialTarget) bool {
	if target.TagRole != tobari.RuntimeMaterialTagPublishedRevision {
		return false
	}
	latest := runtimePruneLatestReceiptRevision(store, target.RuntimeID, target.Revision)
	return latest > runtimePruneSupersededThrough(store, target.RuntimeID, target.Revision)
}

func runtimeRestoreCommittedButNotSuperseded(journal *runtimeBuildJournal, target runtimeMaterialTarget) bool {
	if journal == nil || !journal.Restore || journal.RuntimeID != target.RuntimeID || journal.Revision != target.Revision {
		return false
	}
	return journal.Phase == runtimeBuildPhaseManifestCommitted ||
		(journal.Phase == runtimeBuildPhaseCompleting && journal.CleanupFrom == runtimeBuildPhaseManifestCommitted)
}

func runtimeRetirementGenerations(store runtimePruneReceiptStore, targets []runtimeMaterialTarget) []tobari.RuntimeRetirementGeneration {
	current := make(map[string]bool, len(targets))
	for _, target := range targets {
		if target.TagRole == tobari.RuntimeMaterialTagPublishedRevision {
			current[target.RuntimeID+"\x00"+target.Revision] = true
		}
	}
	result := make([]tobari.RuntimeRetirementGeneration, 0, len(store.AvailabilitySupersessions))
	for _, supersession := range store.AvailabilitySupersessions {
		if current[supersession.RuntimeID+"\x00"+supersession.Revision] {
			result = append(result, tobari.RuntimeRetirementGeneration{RuntimeID: supersession.RuntimeID, Revision: supersession.Revision, Generation: supersession.ThroughReceiptRevision})
		}
	}
	return result
}

func (r *Runtime) observeRuntimeMaterials(ctx context.Context, targets []runtimeMaterialTarget, workspaces map[string]runtimeWorkspaceContainerAuthority, budget *runtimeLifecycleBudget) ([]tobari.RuntimeMaterialObservation, error) {
	digests := make(map[string]string, len(targets))
	for _, target := range targets {
		if target.RecordedDigest != "" {
			digests[target.RuntimeID+"\x00"+target.Revision] = target.RecordedDigest
		}
	}
	containerUse, err := r.observeRuntimeContainerUse(ctx, workspaces, digests, budget)
	if err != nil {
		return nil, err
	}
	result := make([]tobari.RuntimeMaterialObservation, 0, len(targets))
	for _, target := range targets {
		material, err := r.observeRuntimeMaterial(ctx, target, containerUse, budget)
		if err != nil {
			return nil, err
		}
		result = append(result, material)
	}
	return result, nil
}

func (r *Runtime) observeRuntimeMaterial(ctx context.Context, target runtimeMaterialTarget, containerUse map[string]runtimeContentUse, budget *runtimeLifecycleBudget) (tobari.RuntimeMaterialObservation, error) {
	expectedSelector := managedLibraryRuntimeImage(target.Name, target.RuntimeID, target.Revision)
	if target.TagRole == tobari.RuntimeMaterialTagJournaledStaging {
		expectedSelector = managedRuntimeStagingImage(target.RuntimeID, target.Revision)
	}
	if target.Selector != expectedSelector {
		return tobari.RuntimeMaterialObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	material := tobari.RuntimeMaterialObservation{RuntimeID: target.RuntimeID, Revision: target.Revision, TagRole: target.TagRole, ObservationComplete: true}
	tagImage, tagMissing, err := r.inspectRuntimeLifecycleImage(ctx, target.Selector, budget)
	if err != nil {
		return tobari.RuntimeMaterialObservation{}, err
	}
	material.TagPresent = !tagMissing
	if !tagMissing {
		member := false
		for _, tag := range tagImage.RepoTags {
			if tag == target.Selector {
				member = true
				break
			}
		}
		if !member {
			return tobari.RuntimeMaterialObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
	}
	contentDigest := target.RecordedDigest
	if contentDigest == "" && !tagMissing {
		contentDigest = tagImage.ID
	}
	var content runtimeImageObservation
	contentMissing := contentDigest == ""
	if !contentMissing && !tagMissing && tagImage.ID == contentDigest {
		content = tagImage
	} else if !contentMissing {
		content, contentMissing, err = r.inspectRuntimeLifecycleImage(ctx, contentDigest, budget)
		if err != nil {
			return tobari.RuntimeMaterialObservation{}, err
		}
	}
	if !contentMissing {
		material.ContentPresent = true
		material.ImageVirtualBytes = &content.Size
		material.OwnershipVerified = content.Owner == ownerValue && content.Component == managedRuntimeComponentLabel && content.RuntimeID == target.RuntimeID && content.Revision == target.Revision
		for _, repoTag := range content.RepoTags {
			if repoTag != target.Selector {
				material.SharedContent = true
				break
			}
		}
		if use, ok := containerUse[contentDigest]; ok {
			material.WorkspaceInUse, material.ExternalInUse = use.workspace, use.external
		}
	}
	switch {
	case !tagMissing && target.RecordedDigest != "" && tagImage.ID != target.RecordedDigest:
		material.Availability = tobari.RuntimeAvailabilityMismatched
	case !tagMissing && (!material.ContentPresent || !material.OwnershipVerified):
		material.Availability = tobari.RuntimeAvailabilityUnknown
		material.MigrationUnverified = true
	case !tagMissing:
		material.Availability = tobari.RuntimeAvailabilityAvailable
	case material.ContentPresent && !material.OwnershipVerified:
		material.Availability = tobari.RuntimeAvailabilityUnknown
		material.MigrationUnverified = true
	default:
		material.Availability = tobari.RuntimeAvailabilityMissing
	}
	if err := material.Validate(); err != nil {
		return tobari.RuntimeMaterialObservation{}, err
	}
	return material, nil
}

func (r *Runtime) inspectRuntimeLifecycleImage(ctx context.Context, selector string, budget *runtimeLifecycleBudget) (runtimeImageObservation, bool, error) {
	format := `{"id":{{json .Id}},"size":{{json .Size}},"repo_tags":{{json .RepoTags}},` +
		`"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"runtime_id":{{json (index .Config.Labels "` + managedRuntimeIDLabel + `")}},` +
		`"revision":{{json (index .Config.Labels "` + managedRuntimeRevisionLabel + `")}}}`
	output, diagnostic, err := budget.run(ctx, r.runner, []string{"image", "inspect", "--format", format, selector}, os.Environ(), maxRuntimeLifecycleInspect)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return runtimeImageObservation{}, false, err
		}
		if isMissingRuntimeImageInspect(err, diagnostic, selector) {
			return runtimeImageObservation{}, true, nil
		}
		return runtimeImageObservation{}, false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	var image runtimeImageObservation
	if decodeStrictJSON(output, &image) != nil || tobari.ValidateDigest(image.ID) != nil || image.Size < 0 || len(image.RepoTags) > maxRuntimeLifecycleTags {
		return runtimeImageObservation{}, false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	if tobari.ValidateDigest(selector) == nil && image.ID != selector {
		return runtimeImageObservation{}, false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	seen := make(map[string]bool, len(image.RepoTags))
	for _, tag := range image.RepoTags {
		if tag == "" || len(tag) > 512 || strings.ContainsAny(tag, "\x00\r\n") || seen[tag] {
			return runtimeImageObservation{}, false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		seen[tag] = true
	}
	return image, false, nil
}

func (r *Runtime) observeRuntimeContainerUse(ctx context.Context, workspaces map[string]runtimeWorkspaceContainerAuthority, revisionDigests map[string]string, budget *runtimeLifecycleBudget) (map[string]runtimeContentUse, error) {
	use := make(map[string]runtimeContentUse)
	digests := make(map[string]bool, len(revisionDigests))
	for _, digest := range revisionDigests {
		if tobari.ValidateDigest(digest) != nil {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		digests[digest] = true
	}
	orderedDigests := make([]string, 0, len(digests))
	for digest := range digests {
		orderedDigests = append(orderedDigests, digest)
	}
	sort.Strings(orderedDigests)
	containerDigests := make(map[string]string, len(workspaces))
	for id := range workspaces {
		if !runtimeLifecycleContainerID.MatchString(id) {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		containerDigests[id] = ""
	}
	if len(containerDigests) > maxRuntimeLifecycleContainers {
		return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	for _, digest := range orderedDigests {
		output, _, err := budget.run(ctx, r.runner, []string{"container", "ls", "--all", "--no-trunc", "--filter", "ancestor=" + digest, "--format", "{{.ID}}"}, os.Environ(), maxRuntimeLifecycleListBytes)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if err != nil {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		lines, err := parseRuntimeContainerIDs(output)
		if err != nil || len(lines) > maxRuntimeContainersPerImage {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		for _, id := range lines {
			if previous, exists := containerDigests[id]; exists && previous != "" && previous != digest {
				return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
			}
			containerDigests[id] = digest
		}
		if len(containerDigests) > maxRuntimeLifecycleContainers {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
	}
	ids := make([]string, 0, len(containerDigests))
	for id := range containerDigests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	format := `{"id":{{json .Id}},"image":{{json .Image}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
		`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},` +
		`"spec":{{json (index .Config.Labels "` + projectSpecLabel + `")}}}`
	for _, id := range ids {
		observedBytes, _, inspectErr := budget.run(ctx, r.runner, []string{"container", "inspect", "--format", format, id}, os.Environ(), 4096)
		if errors.Is(inspectErr, context.Canceled) || errors.Is(inspectErr, context.DeadlineExceeded) {
			return nil, inspectErr
		}
		if inspectErr != nil {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		var observed runtimeContainerObservation
		if decodeStrictJSON(observedBytes, &observed) != nil || observed.ID != id || tobari.ValidateDigest(observed.Image) != nil {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		if filteredDigest := containerDigests[id]; filteredDigest != "" && observed.Image != filteredDigest {
			return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
		}
		current := use[observed.Image]
		if authority, trusted := workspaces[id]; trusted {
			expectedDigest, knownRevision := revisionDigests[authority.RuntimeID+"\x00"+authority.Revision]
			if !knownRevision || observed.Owner != ownerValue || observed.Component != "tobari" || observed.Workspace != authority.WorkspaceID || observed.Role != projectWorkRole || observed.Spec != authority.ResolvedSpec || observed.Image != expectedDigest {
				return nil, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
			}
			current.workspace = true
		} else {
			current.external = true
		}
		use[observed.Image] = current
	}
	return use, nil
}

func parseRuntimeContainerIDs(output []byte) ([]string, error) {
	trimmed := strings.TrimSuffix(string(output), "\n")
	if trimmed == "" {
		return []string{}, nil
	}
	lines := strings.Split(trimmed, "\n")
	seen := make(map[string]bool, len(lines))
	for _, id := range lines {
		if !runtimeLifecycleContainerID.MatchString(id) || seen[id] {
			return nil, fmt.Errorf("Runtime container discovery returned invalid authority")
		}
		seen[id] = true
	}
	return lines, nil
}
