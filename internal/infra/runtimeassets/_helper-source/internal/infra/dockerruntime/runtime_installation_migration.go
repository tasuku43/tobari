package dockerruntime

import (
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

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const runtimeInstallationMigrationJournalSchema = 1

func emptyRuntimeMigrationIdentity() tobari.SemanticDigest {
	digest := sha256.Sum256([]byte("tobari-installation-runtime-output\x00empty"))
	return tobari.SemanticDigest("sha256:" + hex.EncodeToString(digest[:]))
}

type runtimeInstallationMigrationJournal struct {
	SchemaVersion int                   `json:"schema_version"`
	RuntimeIDs    []string              `json:"runtime_ids"`
	ExpectedTree  tobari.SemanticDigest `json:"expected_tree_digest"`
}

type RuntimeInstallationMigrationStage struct {
	runtime   *Runtime
	ids       []string
	noOp      bool
	committed bool
	complete  bool
	expected  tobari.SemanticDigest
}

func (s *RuntimeInstallationMigrationStage) ExpectedIdentity() (tobari.SemanticDigest, error) {
	if s == nil || s.expected.Validate() != nil {
		return "", fmt.Errorf("Runtime migration expected identity is unavailable")
	}
	return s.expected, nil
}

// ObserveInstallationRuntimeMigration returns a byte identity for the entire
// predecessor Runtime catalog after strict structural and Template-binding
// validation. It creates no directory, lock, stage, or journal.
func (r *Runtime) ObserveInstallationRuntimeMigration(ctx context.Context, collection tobari.WorkspaceAuthorityCollection) (tobari.SemanticDigest, error) {
	if err := collection.Validate(); err != nil {
		return "", err
	}
	root := r.runtimesDirectory()
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		digest := sha256.Sum256([]byte("tobari-installation-runtime-migration\x00empty"))
		return tobari.SemanticDigest("sha256:" + hex.EncodeToString(digest[:])), nil
	}
	if err != nil {
		return "", err
	}
	byID := make(map[string]tobari.RuntimeManifest, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		manifest, err := r.readLegacyRuntimeForInstallationMigration(entry.Name())
		if err != nil {
			return "", err
		}
		if _, exists := byID[manifest.ID]; exists {
			return "", fmt.Errorf("predecessor Runtime IDs are not unique")
		}
		byID[manifest.ID] = manifest
	}
	if err := validateInstallationRuntimeBindings(collection, byID); err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("tobari-installation-runtime-migration\x00v1\x00"))
	rooted, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer rooted.Close()
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("predecessor Runtime tree is unsafe: %w", err)
		}
		_, _ = hash.Write([]byte(filepath.ToSlash(relative)))
		_, _ = fmt.Fprintf(hash, "\x00%04o\x00", info.Mode().Perm())
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxRuntimeSourceFile {
			return fmt.Errorf("predecessor Runtime file is unsafe")
		}
		file, err := rooted.Open(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		opened, statErr := file.Stat()
		data, readErr := io.ReadAll(io.LimitReader(file, maxRuntimeSourceFile+1))
		closeErr := file.Close()
		after, afterErr := rooted.Lstat(filepath.ToSlash(relative))
		if statErr != nil || readErr != nil || closeErr != nil || afterErr != nil {
			return errors.Join(statErr, readErr, closeErr, afterErr)
		}
		if int64(len(data)) != info.Size() || !os.SameFile(info, opened) || !os.SameFile(info, after) || after.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("predecessor Runtime file changed during observation")
		}
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
		return nil
	})
	if err != nil {
		return "", err
	}
	return tobari.SemanticDigest("sha256:" + hex.EncodeToString(hash.Sum(nil))), nil
}

func validateInstallationRuntimeBindings(collection tobari.WorkspaceAuthorityCollection, byID map[string]tobari.RuntimeManifest) error {
	for _, template := range collection.Templates {
		binding := template.Current.Body.EntryDefaults.Runtime
		if binding.RuntimeID == tobari.StandardRuntimeID {
			continue
		}
		manifest, exists := byID[binding.RuntimeID]
		if !exists {
			return fmt.Errorf("Template Runtime binding is absent from predecessor Runtime catalog")
		}
		found := false
		for _, revision := range manifest.Revisions {
			found = found || revision.Revision == binding.Revision
		}
		if !found {
			return fmt.Errorf("Template Runtime revision is absent from predecessor Runtime history")
		}
	}
	return nil
}

func (r *Runtime) runtimeMigrationConfigStage() string {
	return filepath.Join(r.configDirectory, ".installation-runtime-config-stage")
}

func (r *Runtime) runtimeMigrationStateStage() string {
	return filepath.Join(r.stateDirectory, ".installation-runtime-state-stage")
}

func (r *Runtime) runtimeMigrationLegacyQuarantine() string {
	return filepath.Join(r.configDirectory, ".installation-runtime-legacy")
}

func (r *Runtime) runtimeMigrationJournalPath() string {
	return filepath.Join(r.stateDirectory, "installation-runtime-migration.json")
}

func (r *Runtime) runtimeMigrationJournalTempPath() string {
	return r.runtimeMigrationJournalPath() + ".next"
}

func (r *Runtime) requireNoInstallationRuntimeMigration() error {
	for _, path := range []string{r.runtimeMigrationJournalPath(), r.runtimeMigrationJournalTempPath()} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("Runtime installation migration journal is unsafe")
		}
		return fmt.Errorf("Runtime installation migration requires recovery")
	}
	return nil
}

// PrepareInstallationRuntimeMigration snapshots the exact predecessor custom
// Runtime layout into the concept-separated config/state shape. Preparation is
// read-only with respect to every canonical Runtime path.
func (r *Runtime) PrepareInstallationRuntimeMigration(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, recovery ...bool) (*RuntimeInstallationMigrationStage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := collection.Validate(); err != nil {
		return nil, err
	}
	if err := r.recoverRuntimeInstallationMigrationJournal(); err != nil {
		return nil, err
	}
	if len(recovery) == 1 && recovery[0] {
		if journal, err := r.readRuntimeInstallationMigrationJournal(); err != nil {
			return nil, err
		} else if journal != nil {
			return &RuntimeInstallationMigrationStage{runtime: r, ids: append([]string{}, journal.RuntimeIDs...), committed: true, expected: journal.ExpectedTree}, nil
		}
		ids := installationRuntimeIDs(collection)
		configStagePresent := false
		if _, err := os.Lstat(r.runtimeMigrationConfigStage()); err == nil {
			configStagePresent = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		stateStagePresent := false
		if _, err := os.Lstat(r.runtimeMigrationStateStage()); err == nil {
			stateStagePresent = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if configStagePresent != stateStagePresent {
			return nil, fmt.Errorf("Runtime installation migration prepared stage is incomplete")
		}
		if configStagePresent {
			if _, err := os.Lstat(r.runtimeMigrationLegacyQuarantine()); !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("Runtime installation migration prepared and committed locations conflict: %w", err)
			}
			expected, err := runtimeMigrationTreeDigest(ctx, r.runtimeMigrationConfigStage(), r.runtimeMigrationStateStage())
			if err != nil {
				return nil, err
			}
			return &RuntimeInstallationMigrationStage{runtime: r, ids: ids, expected: expected}, nil
		}
		expected := emptyRuntimeMigrationIdentity()
		if len(ids) != 0 {
			var err error
			expected, err = runtimeMigrationTreeDigest(ctx, r.runtimesDirectory(), r.runtimeStatesDirectory())
			if err != nil {
				return nil, err
			}
		}
		stage := &RuntimeInstallationMigrationStage{runtime: r, ids: ids, noOp: len(ids) == 0, committed: true, complete: true, expected: expected}
		if err := stage.Verify(ctx); err != nil {
			return nil, err
		}
		return stage, nil
	} else if err := r.rollbackInterruptedRuntimeInstallationMigration(); err != nil {
		return nil, err
	}
	// A process may die after both complete output trees were durably prepared
	// but before the outer cross-component journal became selectable. With no
	// Runtime journal or quarantine, those paired hidden trees cannot be active;
	// validate them as closed Runtime trees and retire them before recomputing the
	// same stale-bound plan. A lone or malformed stage remains a fail-closed
	// recovery fault rather than being guessed away.
	configStagePresent := false
	if _, err := os.Lstat(r.runtimeMigrationConfigStage()); err == nil {
		configStagePresent = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	stateStagePresent := false
	if _, err := os.Lstat(r.runtimeMigrationStateStage()); err == nil {
		stateStagePresent = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if configStagePresent != stateStagePresent {
		return nil, fmt.Errorf("Runtime installation migration prepared stage is incomplete")
	}
	if configStagePresent {
		if _, err := runtimeMigrationTreeDigest(ctx, r.runtimeMigrationConfigStage(), r.runtimeMigrationStateStage()); err != nil {
			return nil, fmt.Errorf("Runtime installation migration prepared stage is unsafe: %w", err)
		}
		if err := os.RemoveAll(r.runtimeMigrationConfigStage()); err != nil { // #nosec G301 -- exact verified migration-owned stage.
			return nil, err
		}
		if err := os.RemoveAll(r.runtimeMigrationStateStage()); err != nil { // #nosec G301 -- exact verified migration-owned stage.
			return nil, err
		}
		if err := errors.Join(syncDirectoryIfPresent(r.configDirectory), syncDirectoryIfPresent(r.stateDirectory)); err != nil {
			return nil, err
		}
	}
	entries, err := os.ReadDir(r.runtimesDirectory())
	if errors.Is(err, os.ErrNotExist) || err == nil && len(entries) == 0 {
		return &RuntimeInstallationMigrationStage{runtime: r, noOp: true, ids: []string{}, expected: emptyRuntimeMigrationIdentity()}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := requirePrivateDirectory(r.runtimesDirectory()); err != nil {
		return nil, err
	}
	legacy := make([]tobari.RuntimeManifest, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("predecessor Runtime catalog contains an unsafe child")
		}
		manifest, err := r.readLegacyRuntimeForInstallationMigration(entry.Name())
		if err != nil {
			return nil, err
		}
		legacy = append(legacy, manifest)
	}
	sort.Slice(legacy, func(i, j int) bool { return legacy[i].ID < legacy[j].ID })
	byID := make(map[string]tobari.RuntimeManifest, len(legacy))
	for _, manifest := range legacy {
		if _, exists := byID[manifest.ID]; exists {
			return nil, fmt.Errorf("predecessor Runtime IDs are not unique")
		}
		byID[manifest.ID] = manifest
	}
	if err := validateInstallationRuntimeBindings(collection, byID); err != nil {
		return nil, err
	}
	configStage := r.runtimeMigrationConfigStage()
	stateStage := r.runtimeMigrationStateStage()
	for _, path := range []string{configStage, stateStage, r.runtimeMigrationLegacyQuarantine()} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("Runtime migration reserved path is occupied: %w", err)
		}
	}
	if err := os.Mkdir(configStage, 0o700); err != nil {
		return nil, err
	}
	if err := os.Mkdir(stateStage, 0o700); err != nil {
		_ = os.Remove(configStage)
		return nil, err
	}
	abort := true
	defer func() {
		if abort {
			_ = os.RemoveAll(configStage) // #nosec G301 -- exact migration-owned staging path.
			_ = os.RemoveAll(stateStage)  // #nosec G301 -- exact migration-owned staging path.
		}
	}()
	ids := make([]string, 0, len(legacy))
	for _, old := range legacy {
		ids = append(ids, old.ID)
		configRuntime := filepath.Join(configStage, old.ID)
		stateRuntime := filepath.Join(stateStage, old.ID)
		if err := os.Mkdir(configRuntime, 0o700); err != nil {
			return nil, err
		}
		if err := os.Mkdir(stateRuntime, 0o700); err != nil {
			return nil, err
		}
		oldRoot := filepath.Join(r.runtimesDirectory(), old.Name)
		if _, err := copyRuntimeSource(ctx, filepath.Join(oldRoot, "source"), filepath.Join(configRuntime, "source")); err != nil {
			return nil, err
		}
		if err := writeRuntimeSourceMetadata(filepath.Join(configRuntime, "runtime.yaml"), runtimeSourceMetadata{SchemaVersion: 1, RuntimeID: old.ID, Name: old.Name}); err != nil {
			return nil, err
		}
		if err := os.Mkdir(filepath.Join(stateRuntime, "revisions"), 0o700); err != nil {
			return nil, err
		}
		next := old
		next.SourcePath = r.runtimeSourceDirectory(old.ID)
		for index := range next.Revisions {
			digest := strings.TrimPrefix(next.Revisions[index].Revision, "sha256:")
			oldSnapshot := filepath.Join(oldRoot, "revisions", digest, "source")
			newSnapshot := filepath.Join(stateRuntime, "revisions", digest, "source")
			if err := os.Mkdir(filepath.Dir(newSnapshot), 0o700); err != nil {
				return nil, err
			}
			if _, err := copyRuntimeSource(ctx, oldSnapshot, newSnapshot); err != nil {
				return nil, err
			}
			next.Revisions[index].SnapshotPath = filepath.Join(r.runtimeRevisionsDirectory(old.ID), digest, "source")
		}
		if err := next.Validate(); err != nil {
			return nil, err
		}
		if err := writeAtomicJSON(filepath.Join(stateRuntime, "runtime.json"), next); err != nil {
			return nil, err
		}
	}
	if err := syncRuntimeMigrationTree(configStage); err != nil {
		return nil, err
	}
	if err := syncRuntimeMigrationTree(stateStage); err != nil {
		return nil, err
	}
	expected, err := runtimeMigrationTreeDigest(ctx, configStage, stateStage)
	if err != nil {
		return nil, err
	}
	abort = false
	return &RuntimeInstallationMigrationStage{runtime: r, ids: ids, expected: expected}, nil
}

func installationRuntimeIDs(collection tobari.WorkspaceAuthorityCollection) []string {
	set := map[string]struct{}{}
	for _, template := range collection.Templates {
		id := template.Current.Body.EntryDefaults.Runtime.RuntimeID
		if id != tobari.StandardRuntimeID {
			set[id] = struct{}{}
		}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func validateRuntimeInstallationMigrationJournal(journal runtimeInstallationMigrationJournal) error {
	if journal.SchemaVersion != runtimeInstallationMigrationJournalSchema || journal.RuntimeIDs == nil || journal.ExpectedTree.Validate() != nil {
		return fmt.Errorf("Runtime installation migration journal is invalid")
	}
	return nil
}

func (r *Runtime) writeRuntimeInstallationMigrationJournal(journal runtimeInstallationMigrationJournal) error {
	if err := validateRuntimeInstallationMigrationJournal(journal); err != nil {
		return err
	}
	path := r.runtimeMigrationJournalPath()
	temp := r.runtimeMigrationJournalTempPath()
	for _, reserved := range []string{path, temp} {
		if _, err := os.Lstat(reserved); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Runtime installation migration journal path is occupied: %w", err)
		}
	}
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- exact deterministic Runtime migration journal successor.
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = r.runtimeInstallationMigrationBoundaryCall("runtime_journal_temp_written")
	}
	if err == nil {
		err = file.Sync()
	}
	if err == nil {
		err = r.runtimeInstallationMigrationBoundaryCall("runtime_journal_temp_synced")
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("runtime_journal_renamed"); err != nil {
		return err
	}
	if err := syncDirectoryIfPresent(r.stateDirectory); err != nil {
		return err
	}
	return r.runtimeInstallationMigrationBoundaryCall("runtime_journal_parent_synced")
}

func (r *Runtime) recoverRuntimeInstallationMigrationJournal() error {
	path := r.runtimeMigrationJournalPath()
	temp := r.runtimeMigrationJournalTempPath()
	var staged runtimeInstallationMigrationJournal
	if err := readStrictJSON(temp, &staged); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("Runtime installation migration journal successor is invalid: %w", err)
	}
	if err := validateRuntimeInstallationMigrationJournal(staged); err != nil {
		return fmt.Errorf("Runtime installation migration journal successor is invalid: %w", err)
	}
	var current runtimeInstallationMigrationJournal
	if err := readStrictJSON(path, &current); err == nil {
		if validateRuntimeInstallationMigrationJournal(current) != nil || current.SchemaVersion != staged.SchemaVersion || current.ExpectedTree != staged.ExpectedTree || strings.Join(current.RuntimeIDs, "\x00") != strings.Join(staged.RuntimeIDs, "\x00") {
			return fmt.Errorf("Runtime installation migration journal and successor conflict")
		}
		if err := os.Remove(temp); err != nil {
			return err
		}
		return syncDirectoryIfPresent(r.stateDirectory)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		return err
	}
	return syncDirectoryIfPresent(r.stateDirectory)
}

func (r *Runtime) readRuntimeInstallationMigrationJournal() (*runtimeInstallationMigrationJournal, error) {
	if err := r.recoverRuntimeInstallationMigrationJournal(); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(r.runtimeMigrationJournalPath()); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var journal runtimeInstallationMigrationJournal
	if err := readStrictJSON(r.runtimeMigrationJournalPath(), &journal); err != nil || validateRuntimeInstallationMigrationJournal(journal) != nil {
		return nil, fmt.Errorf("Runtime installation migration journal is invalid: %w", err)
	}
	return &journal, nil
}

func (r *Runtime) readLegacyRuntimeForInstallationMigration(name string) (tobari.RuntimeManifest, error) {
	if err := tobari.ValidateName(name); err != nil {
		return tobari.RuntimeManifest{}, err
	}
	root := filepath.Join(r.runtimesDirectory(), name)
	if err := requirePrivateDirectory(root); err != nil {
		return tobari.RuntimeManifest{}, err
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 3 {
		return tobari.RuntimeManifest{}, fmt.Errorf("predecessor Runtime inventory is incomplete: %w", err)
	}
	want := map[string]bool{"runtime.json": true, "revisions": true, "source": true}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !want[entry.Name()] {
			return tobari.RuntimeManifest{}, fmt.Errorf("predecessor Runtime inventory is unsafe")
		}
		delete(want, entry.Name())
	}
	var manifest tobari.RuntimeManifest
	if err := readStrictJSON(filepath.Join(root, "runtime.json"), &manifest); err != nil {
		return manifest, err
	}
	if err := manifest.Validate(); err != nil || manifest.Kind != tobari.RuntimeKindManaged || manifest.Name != name || manifest.SourcePath != filepath.Join(root, "source") {
		return manifest, fmt.Errorf("predecessor Runtime manifest does not match its path: %w", err)
	}
	for _, revision := range manifest.Revisions {
		want := filepath.Join(root, "revisions", strings.TrimPrefix(revision.Revision, "sha256:"), "source")
		if revision.SnapshotPath != want {
			return manifest, fmt.Errorf("predecessor Runtime snapshot path is not canonical")
		}
	}
	return manifest, nil
}

func (s *RuntimeInstallationMigrationStage) Commit(ctx context.Context) error {
	if s == nil || s.runtime == nil || s.committed {
		return fmt.Errorf("Runtime migration stage is not committable")
	}
	if s.noOp {
		s.committed = true
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	r := s.runtime
	if s.expected.Validate() != nil {
		return fmt.Errorf("Runtime migration expected tree identity is invalid")
	}
	journal := runtimeInstallationMigrationJournal{SchemaVersion: runtimeInstallationMigrationJournalSchema, RuntimeIDs: append([]string{}, s.ids...), ExpectedTree: s.expected}
	if err := r.runtimeInstallationMigrationBoundaryCall("journal_write_prepared"); err != nil {
		return err
	}
	if err := r.writeRuntimeInstallationMigrationJournal(journal); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("journal_written"); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("legacy_rename_prepared"); err != nil {
		return err
	}
	if err := os.Rename(r.runtimesDirectory(), r.runtimeMigrationLegacyQuarantine()); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("legacy_quarantined"); err != nil {
		return err
	}
	if err := syncDirectoryIfPresent(r.configDirectory); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("legacy_quarantine_synced"); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("config_rename_prepared"); err != nil {
		return err
	}
	if err := os.Rename(r.runtimeMigrationConfigStage(), r.runtimesDirectory()); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("config_published"); err != nil {
		return err
	}
	if err := syncDirectoryIfPresent(r.configDirectory); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("config_publish_synced"); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("state_rename_prepared"); err != nil {
		return err
	}
	if err := os.Rename(r.runtimeMigrationStateStage(), r.runtimeStatesDirectory()); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("state_published"); err != nil {
		return err
	}
	if err := syncDirectoryIfPresent(r.stateDirectory); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("state_publish_synced"); err != nil {
		return err
	}
	s.committed = true
	return nil
}

func (r *Runtime) runtimeInstallationMigrationBoundaryCall(phase string) error {
	if r.runtimeInstallationMigrationBoundary != nil {
		return r.runtimeInstallationMigrationBoundary(phase)
	}
	return nil
}

func (s *RuntimeInstallationMigrationStage) Verify(ctx context.Context) error {
	if s == nil || !s.committed {
		return fmt.Errorf("Runtime migration stage is not committed")
	}
	if s.noOp {
		return ctx.Err()
	}
	for _, id := range s.ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := s.runtime.readRuntimeManifestByID(id); err != nil {
			return err
		}
	}
	if s.expected != "" {
		observed, err := runtimeMigrationTreeDigest(ctx, s.runtime.runtimesDirectory(), s.runtime.runtimeStatesDirectory())
		if err != nil || observed != s.expected {
			return errors.Join(fmt.Errorf("published Runtime migration tree differs from the planned tree"), err)
		}
	}
	return s.runtime.runtimeInstallationMigrationBoundaryCall("runtime_readback_verified")
}

func (s *RuntimeInstallationMigrationStage) Rollback(context.Context) error {
	if s == nil || s.runtime == nil || s.noOp {
		return nil
	}
	err := s.runtime.rollbackInterruptedRuntimeInstallationMigration()
	s.committed = false
	return err
}

func (s *RuntimeInstallationMigrationStage) Abort(ctx context.Context) error {
	if s == nil || s.runtime == nil || s.noOp {
		return nil
	}
	if _, err := os.Lstat(s.runtime.runtimeMigrationJournalPath()); err == nil {
		return s.Rollback(ctx)
	}
	err := os.RemoveAll(s.runtime.runtimeMigrationConfigStage())                 // #nosec G301 -- exact migration-owned staging path.
	err = errors.Join(err, os.RemoveAll(s.runtime.runtimeMigrationStateStage())) // #nosec G301 -- exact migration-owned staging path.
	return err
}

func (s *RuntimeInstallationMigrationStage) Complete(context.Context) error {
	if s == nil || !s.committed {
		return fmt.Errorf("Runtime migration stage is not committed")
	}
	if s.noOp {
		return nil
	}
	if s.complete {
		return nil
	}
	r := s.runtime
	if err := r.runtimeInstallationMigrationBoundaryCall("runtime_cleanup_prepared"); err != nil {
		return err
	}
	if err := os.RemoveAll(r.runtimeMigrationLegacyQuarantine()); err != nil { // #nosec G301 -- validated exact predecessor quarantine.
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("runtime_legacy_cleanup_removed"); err != nil {
		return err
	}
	if err := os.Remove(r.runtimeMigrationJournalPath()); err != nil {
		return err
	}
	if err := r.runtimeInstallationMigrationBoundaryCall("runtime_journal_removed"); err != nil {
		return err
	}
	if err := errors.Join(syncDirectoryIfPresent(r.configDirectory), syncDirectoryIfPresent(r.stateDirectory)); err != nil {
		return err
	}
	return r.runtimeInstallationMigrationBoundaryCall("runtime_cleanup_synced")
}

func (r *Runtime) rollbackInterruptedRuntimeInstallationMigration() error {
	if _, err := os.Lstat(r.runtimeMigrationJournalPath()); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	var journal runtimeInstallationMigrationJournal
	if err := readStrictJSON(r.runtimeMigrationJournalPath(), &journal); err != nil || journal.SchemaVersion != runtimeInstallationMigrationJournalSchema || journal.RuntimeIDs == nil {
		return fmt.Errorf("Runtime installation migration journal is invalid: %w", err)
	}
	if _, err := os.Lstat(r.runtimeStatesDirectory()); err == nil {
		if _, stageErr := os.Lstat(r.runtimeMigrationStateStage()); !errors.Is(stageErr, os.ErrNotExist) {
			return fmt.Errorf("Runtime migration state target and stage both exist")
		}
		if err := os.Rename(r.runtimeStatesDirectory(), r.runtimeMigrationStateStage()); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Lstat(r.runtimeMigrationLegacyQuarantine()); err == nil {
		if _, currentErr := os.Lstat(r.runtimesDirectory()); currentErr == nil {
			if _, stageErr := os.Lstat(r.runtimeMigrationConfigStage()); !errors.Is(stageErr, os.ErrNotExist) {
				return fmt.Errorf("Runtime migration config target and stage both exist")
			}
			if err := os.Rename(r.runtimesDirectory(), r.runtimeMigrationConfigStage()); err != nil {
				return err
			}
		} else if !errors.Is(currentErr, os.ErrNotExist) {
			return currentErr
		}
		if err := os.Rename(r.runtimeMigrationLegacyQuarantine(), r.runtimesDirectory()); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(r.runtimeMigrationConfigStage()); err != nil { // #nosec G301 -- exact migration-owned stage.
		return err
	}
	if err := os.RemoveAll(r.runtimeMigrationStateStage()); err != nil { // #nosec G301 -- exact migration-owned stage.
		return err
	}
	if err := os.Remove(r.runtimeMigrationJournalPath()); err != nil {
		return err
	}
	return errors.Join(syncDirectoryIfPresent(r.configDirectory), syncDirectoryIfPresent(r.stateDirectory))
}

func syncRuntimeMigrationTree(root string) error {
	directories := make([]string, 0)
	rooted, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rooted.Close()
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			directories = append(directories, path)
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		before, err := rooted.Lstat(filepath.ToSlash(relative))
		if err != nil || before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() {
			return fmt.Errorf("Runtime migration staged file is unsafe: %w", err)
		}
		file, err := rooted.Open(filepath.ToSlash(relative))
		if err != nil {
			return err
		}
		opened, statErr := file.Stat()
		syncErr := file.Sync()
		closeErr := file.Close()
		after, afterErr := rooted.Lstat(filepath.ToSlash(relative))
		if statErr != nil || syncErr != nil || closeErr != nil || afterErr != nil {
			return errors.Join(statErr, syncErr, closeErr, afterErr)
		}
		if !os.SameFile(before, opened) || !os.SameFile(before, after) || after.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Runtime migration staged file changed during sync")
		}
		return nil
	}); err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := syncDirectoryIfPresent(directories[index]); err != nil {
			return err
		}
	}
	return nil
}

func runtimeMigrationTreeDigest(ctx context.Context, roots ...string) (tobari.SemanticDigest, error) {
	hash := sha256.New()
	_, _ = hash.Write([]byte("tobari-runtime-migration-output\x00v1\x00"))
	for rootIndex, root := range roots {
		rooted, err := os.OpenRoot(root)
		if err != nil {
			return "", err
		}
		_, _ = fmt.Fprintf(hash, "root:%d\x00", rootIndex)
		err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil || info.Mode()&os.ModeSymlink != 0 {
				return errors.Join(err, fmt.Errorf("Runtime migration output tree is unsafe"))
			}
			_, _ = hash.Write([]byte(filepath.ToSlash(relative)))
			_, _ = fmt.Fprintf(hash, "\x00%04o\x00", info.Mode().Perm())
			if entry.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxRuntimeSourceFile {
				return fmt.Errorf("Runtime migration output file is unsafe")
			}
			file, err := rooted.Open(filepath.ToSlash(relative))
			if err != nil {
				return err
			}
			opened, statErr := file.Stat()
			data, readErr := io.ReadAll(io.LimitReader(file, maxRuntimeSourceFile+1))
			closeErr := file.Close()
			after, afterErr := rooted.Lstat(filepath.ToSlash(relative))
			if statErr != nil || readErr != nil || closeErr != nil || afterErr != nil || !os.SameFile(info, opened) || !os.SameFile(info, after) || int64(len(data)) != info.Size() {
				return errors.Join(statErr, readErr, closeErr, afterErr, fmt.Errorf("Runtime migration output changed during verification"))
			}
			_, _ = hash.Write(data)
			_, _ = hash.Write([]byte{0})
			return nil
		})
		closeErr := rooted.Close()
		if err != nil || closeErr != nil {
			return "", errors.Join(err, closeErr)
		}
	}
	return tobari.SemanticDigest("sha256:" + hex.EncodeToString(hash.Sum(nil))), nil
}
