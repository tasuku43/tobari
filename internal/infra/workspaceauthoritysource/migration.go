package workspaceauthoritysource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const migrationSourceStageName = ".installation-migration-source-stage"
const migrationSourceCleanupName = ".installation-migration-source-cleanup"
const migrationSourceJournalName = "transaction.phase"
const migrationSourceJournalTempName = "transaction.phase.next"

type InstallationMigrationStage struct {
	store                *Store
	root                 string
	committed            []string
	complete             bool
	templates            map[tobari.WorkspaceTemplateID]tobari.WorkspaceTemplateSource
	contexts             map[tobari.ContextID]tobari.ContextSource
	templateFingerprints map[tobari.WorkspaceTemplateID]string
	contextFingerprints  map[tobari.ContextID]string
}

// ExpectedIdentity binds the byte-exact canonical source files prepared for
// every concept resource. It is safe to persist in the outer transaction and
// lets restart recovery reject a different or partially rebuilt stage.
func (s *InstallationMigrationStage) ExpectedIdentity() (tobari.SemanticDigest, error) {
	if s == nil || s.templateFingerprints == nil || s.contextFingerprints == nil {
		return "", fmt.Errorf("migration source identity is unavailable")
	}
	templateIDs := make([]string, 0, len(s.templateFingerprints))
	for id := range s.templateFingerprints {
		templateIDs = append(templateIDs, string(id))
	}
	contextIDs := make([]string, 0, len(s.contextFingerprints))
	for id := range s.contextFingerprints {
		contextIDs = append(contextIDs, string(id))
	}
	sort.Strings(templateIDs)
	sort.Strings(contextIDs)
	hash := sha256.New()
	for _, id := range templateIDs {
		_, _ = fmt.Fprintf(hash, "template\x00%s\x00%s\x00", id, s.templateFingerprints[tobari.WorkspaceTemplateID(id)])
	}
	for _, id := range contextIDs {
		_, _ = fmt.Fprintf(hash, "context\x00%s\x00%s\x00", id, s.contextFingerprints[tobari.ContextID(id)])
	}
	return tobari.SemanticDigest("sha256:" + hex.EncodeToString(hash.Sum(nil))), nil
}

// PrepareInstallationMigrationSources materializes and validates the complete
// canonical Template/Context source tree below one hidden staging directory.
// It never touches the selectable concept roots.
func (s *Store) PrepareInstallationMigrationSources(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, recovery ...bool) (*InstallationMigrationStage, error) {
	if s == nil || s.phase == nil {
		return nil, fmt.Errorf("installation migration source store is unavailable")
	}
	if err := collection.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ensurePrivateDirectory(s.configRoot); err != nil {
		return nil, err
	}
	stageRoot := filepath.Join(s.configRoot, migrationSourceStageName)
	cleanupRoot := filepath.Join(s.configRoot, migrationSourceCleanupName)
	if _, stageErr := os.Lstat(stageRoot); stageErr == nil {
		if _, cleanupErr := os.Lstat(cleanupRoot); !errors.Is(cleanupErr, os.ErrNotExist) {
			return nil, errors.Join(tobari.ErrResourceSourceRecoveryRequired, cleanupErr)
		}
	} else if !errors.Is(stageErr, os.ErrNotExist) {
		return nil, stageErr
	} else if _, cleanupErr := os.Lstat(cleanupRoot); cleanupErr == nil {
		if len(recovery) != 1 || !recovery[0] {
			return nil, tobari.ErrResourceSourceRecoveryRequired
		}
		stage, err := s.reopenInstallationMigrationSources(ctx, collection, cleanupRoot)
		if err != nil {
			return nil, err
		}
		return stage, nil
	} else if !errors.Is(cleanupErr, os.ErrNotExist) {
		return nil, cleanupErr
	}
	if _, err := os.Lstat(stageRoot); err == nil {
		if len(recovery) == 1 && recovery[0] {
			stage, err := s.reopenInstallationMigrationSources(ctx, collection, stageRoot)
			if err != nil {
				return nil, err
			}
			return stage, nil
		} else {
			stage, err := s.reopenInstallationMigrationSources(ctx, collection, stageRoot)
			if errors.Is(err, os.ErrNotExist) {
				stage, err = s.reopenUnjournaledPreparedInstallationMigrationSources(ctx, collection, stageRoot)
			}
			if err != nil {
				return nil, err
			}
			if err := stage.Abort(ctx); err != nil {
				return nil, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else if len(recovery) == 1 && recovery[0] {
		stage, err := s.reopenCompletedInstallationMigrationSources(ctx, collection, stageRoot)
		if err != nil {
			return nil, err
		}
		return stage, nil
	}
	for _, concept := range []string{"templates", "contexts"} {
		if _, err := os.Lstat(filepath.Join(s.configRoot, concept)); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				return nil, errors.Join(tobari.ErrMigrationSourceUnsafe, fmt.Errorf("existing %s source must not be replaced", concept))
			}
			return nil, err
		}
	}
	if err := os.Mkdir(stageRoot, 0o700); err != nil {
		return nil, err
	}
	stage := &InstallationMigrationStage{store: s, root: stageRoot, committed: []string{}, templates: map[tobari.WorkspaceTemplateID]tobari.WorkspaceTemplateSource{}, contexts: map[tobari.ContextID]tobari.ContextSource{}, templateFingerprints: map[tobari.WorkspaceTemplateID]string{}, contextFingerprints: map[tobari.ContextID]string{}}
	abort := true
	defer func() {
		if abort {
			_ = stage.Abort(ctx)
		}
	}()
	stagedStore, err := New(stageRoot)
	if err != nil {
		return nil, err
	}
	stagedStore.phase = s.phase
	templates := append([]tobari.WorkspaceTemplate(nil), collection.Templates...)
	sort.Slice(templates, func(i, j int) bool { return templates[i].ID < templates[j].ID })
	for _, template := range templates {
		source, err := tobari.NewWorkspaceTemplateSource(template)
		if err != nil {
			return nil, err
		}
		if err := stagedStore.PublishTemplate(ctx, source); err != nil {
			return nil, err
		}
		stage.templates[template.ID] = source.Clone()
		_, fingerprint, present, err := stagedStore.ReadTemplateSnapshot(ctx, template.ID)
		if err != nil || !present {
			return nil, errors.Join(err, tobari.ErrResourceSourceInvalid)
		}
		stage.templateFingerprints[template.ID] = fingerprint
		if err := s.phase("migration_template_staged:" + string(template.ID)); err != nil {
			return nil, err
		}
	}
	contexts := append([]tobari.WorkspaceAuthorityContextRecord(nil), collection.Contexts...)
	sort.Slice(contexts, func(i, j int) bool { return contexts[i].Context.ID < contexts[j].Context.ID })
	for _, record := range contexts {
		source, err := tobari.NewContextSource(record.Context)
		if err != nil {
			return nil, err
		}
		if err := stagedStore.PublishContext(ctx, source); err != nil {
			return nil, err
		}
		stage.contexts[record.Context.ID] = source
		_, fingerprint, present, err := stagedStore.ReadContextSnapshot(ctx, record.Context.ID)
		if err != nil || !present {
			return nil, errors.Join(err, tobari.ErrResourceSourceInvalid)
		}
		stage.contextFingerprints[record.Context.ID] = fingerprint
		if err := s.phase("migration_context_staged:" + string(record.Context.ID)); err != nil {
			return nil, err
		}
	}
	if err := syncSourceTree(stageRoot); err != nil {
		return nil, err
	}
	if err := s.writeMigrationSourcePhase(stageRoot, "prepared"); err != nil {
		return nil, err
	}
	if err := stage.verifyAt(ctx, stagedStore); err != nil {
		return nil, err
	}
	if err := syncDirectory(s.configRoot); err != nil {
		return nil, err
	}
	abort = false
	return stage, nil
}

func (s *Store) reopenUnjournaledPreparedInstallationMigrationSources(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, stageRoot string) (*InstallationMigrationStage, error) {
	stage := &InstallationMigrationStage{store: s, root: stageRoot, templates: map[tobari.WorkspaceTemplateID]tobari.WorkspaceTemplateSource{}, contexts: map[tobari.ContextID]tobari.ContextSource{}, templateFingerprints: map[tobari.WorkspaceTemplateID]string{}, contextFingerprints: map[tobari.ContextID]string{}}
	for _, template := range collection.Templates {
		source, err := tobari.NewWorkspaceTemplateSource(template)
		if err != nil {
			return nil, err
		}
		stage.templates[template.ID] = source
	}
	for _, record := range collection.Contexts {
		source, err := tobari.NewContextSource(record.Context)
		if err != nil {
			return nil, err
		}
		stage.contexts[record.Context.ID] = source
	}
	if err := stage.populateCanonicalFingerprints(); err != nil {
		return nil, err
	}
	stagedStore, err := New(stageRoot)
	if err != nil {
		return nil, err
	}
	if err := stage.verifyAt(ctx, stagedStore); err != nil {
		return nil, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	return stage, nil
}

func (s *Store) reopenCompletedInstallationMigrationSources(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, stageRoot string) (*InstallationMigrationStage, error) {
	stage := &InstallationMigrationStage{store: s, root: stageRoot, committed: []string{"templates", "contexts"}, complete: true, templates: map[tobari.WorkspaceTemplateID]tobari.WorkspaceTemplateSource{}, contexts: map[tobari.ContextID]tobari.ContextSource{}, templateFingerprints: map[tobari.WorkspaceTemplateID]string{}, contextFingerprints: map[tobari.ContextID]string{}}
	for _, template := range collection.Templates {
		source, err := tobari.NewWorkspaceTemplateSource(template)
		if err != nil {
			return nil, err
		}
		stage.templates[template.ID] = source
	}
	for _, record := range collection.Contexts {
		source, err := tobari.NewContextSource(record.Context)
		if err != nil {
			return nil, err
		}
		stage.contexts[record.Context.ID] = source
	}
	if err := stage.populateCanonicalFingerprints(); err != nil {
		return nil, err
	}
	if err := stage.verifyAt(ctx, s); err != nil {
		return nil, err
	}
	return stage, nil
}

func (s *Store) reopenInstallationMigrationSources(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, stageRoot string) (*InstallationMigrationStage, error) {
	stage := &InstallationMigrationStage{store: s, root: stageRoot, templates: map[tobari.WorkspaceTemplateID]tobari.WorkspaceTemplateSource{}, contexts: map[tobari.ContextID]tobari.ContextSource{}, templateFingerprints: map[tobari.WorkspaceTemplateID]string{}, contextFingerprints: map[tobari.ContextID]string{}}
	for _, template := range collection.Templates {
		source, err := tobari.NewWorkspaceTemplateSource(template)
		if err != nil {
			return nil, err
		}
		stage.templates[template.ID] = source
	}
	for _, record := range collection.Contexts {
		source, err := tobari.NewContextSource(record.Context)
		if err != nil {
			return nil, err
		}
		stage.contexts[record.Context.ID] = source
	}
	phase, err := readMigrationSourcePhase(stageRoot)
	if err != nil {
		return nil, err
	}
	switch phase {
	case "prepared", "templates_renaming", "templates_committed", "contexts_renaming", "contexts_committed", "cleanup_started":
	default:
		return nil, tobari.ErrResourceSourceRecoveryRequired
	}
	if err := stage.populateCanonicalFingerprints(); err != nil {
		return nil, err
	}
	if err := stage.observeCommittedLocations(ctx); err != nil {
		return nil, err
	}
	return stage, nil
}

func (s *InstallationMigrationStage) observeCommittedLocations(ctx context.Context) error {
	committed := make([]string, 0, 2)
	seenStaged := false
	for _, concept := range []string{"templates", "contexts"} {
		canonical, err := s.conceptMatches(ctx, s.store.configRoot, concept)
		if err != nil {
			return err
		}
		staged, err := s.conceptMatches(ctx, s.root, concept)
		if err != nil {
			return err
		}
		if canonical == staged {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, fmt.Errorf("migration %s source exists in neither or both locations", concept))
		}
		if canonical {
			if seenStaged {
				return errors.Join(tobari.ErrResourceSourceRecoveryRequired, fmt.Errorf("migration source concept order is contradictory"))
			}
			committed = append(committed, concept)
		} else {
			seenStaged = true
		}
	}
	s.committed = committed
	return nil
}

func (s *InstallationMigrationStage) conceptMatches(ctx context.Context, root, concept string) (bool, error) {
	path := filepath.Join(root, concept)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || validatePrivateDirectory(info) != nil {
		return false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	store, err := New(root)
	if err != nil {
		return false, err
	}
	switch concept {
	case "templates":
		ids, err := store.ListTemplateIDs(ctx)
		if err != nil || len(ids) != len(s.templateFingerprints) {
			return false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		for _, id := range ids {
			_, fingerprint, present, err := store.ReadTemplateSnapshot(ctx, id)
			if err != nil || !present || fingerprint != s.templateFingerprints[id] {
				return false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
		}
	case "contexts":
		ids, err := store.ListContextIDs(ctx)
		if err != nil || len(ids) != len(s.contextFingerprints) {
			return false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		for _, id := range ids {
			_, fingerprint, present, err := store.ReadContextSnapshot(ctx, id)
			if err != nil || !present || fingerprint != s.contextFingerprints[id] {
				return false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
		}
	default:
		return false, tobari.ErrResourceSourceRecoveryRequired
	}
	return true, nil
}

func (s *InstallationMigrationStage) populateCanonicalFingerprints() error {
	for id, source := range s.templates {
		templateData, err := encodeCanonicalYAML(source.Template)
		if err != nil {
			return err
		}
		policyData, err := encodeCanonicalYAML(source.Policy)
		if err != nil {
			return err
		}
		s.templateFingerprints[id] = sourceFingerprint(templateData, policyData)
	}
	for id, source := range s.contexts {
		data, err := encodeCanonicalYAML(source)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		s.contextFingerprints[id] = hex.EncodeToString(digest[:])
	}
	return nil
}

func (s *InstallationMigrationStage) Commit(ctx context.Context) (resultErr error) {
	if s == nil || s.store == nil || s.complete || len(s.committed) != 0 {
		return fmt.Errorf("migration source stage is not committable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, concept := range []string{"templates", "contexts"} {
		from := filepath.Join(s.root, concept)
		if _, err := os.Lstat(from); errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(from, 0o700); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		to := filepath.Join(s.store.configRoot, concept)
		if _, err := os.Lstat(to); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(tobari.ErrMigrationSourceUnsafe, err)
		}
		if err := s.store.writeMigrationSourcePhase(s.root, concept+"_renaming"); err != nil {
			_ = s.Rollback(ctx)
			return err
		}
		if err := s.store.phase("migration_source_rename_prepared:" + concept); err != nil {
			_ = s.Rollback(ctx)
			return err
		}
		if err := os.Rename(from, to); err != nil {
			_ = s.Rollback(ctx)
			return err
		}
		s.committed = append(s.committed, concept)
		if err := s.store.phase("migration_source_renamed:" + concept); err != nil {
			_ = s.Rollback(ctx)
			return err
		}
		if err := syncDirectory(s.store.configRoot); err != nil {
			_ = s.Rollback(ctx)
			return err
		}
		if err := s.store.phase("migration_source_synced:" + concept); err != nil {
			_ = s.Rollback(ctx)
			return err
		}
		if err := s.store.writeMigrationSourcePhase(s.root, concept+"_committed"); err != nil {
			_ = s.Rollback(ctx)
			return err
		}
		if err := s.store.phase("migration_source_committed:" + concept); err != nil {
			_ = s.Rollback(ctx)
			return err
		}
	}
	return nil
}

func (s *InstallationMigrationStage) Verify(ctx context.Context) error {
	if s == nil || len(s.committed) != 2 {
		return fmt.Errorf("migration source stage is not committed")
	}
	return s.verifyAt(ctx, s.store)
}

func (s *InstallationMigrationStage) verifyAt(ctx context.Context, store *Store) error {
	for _, concept := range []string{"templates", "contexts"} {
		path := filepath.Join(store.configRoot, concept)
		if info, err := os.Lstat(path); err != nil || validatePrivateDirectory(info) != nil {
			return errors.Join(tobari.ErrResourceSourceInvalid, err)
		}
	}
	templateIDs, err := store.ListTemplateIDs(ctx)
	if err != nil {
		return err
	}
	if len(templateIDs) != len(s.templates) {
		return errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("staged Template set changed"))
	}
	for _, id := range templateIDs {
		_, fingerprint, present, err := store.ReadTemplateSnapshot(ctx, id)
		if err != nil || !present || fingerprint != s.templateFingerprints[id] {
			return errors.Join(tobari.ErrResourceSourceInvalid, err, fmt.Errorf("staged Template bytes changed"))
		}
	}
	contextIDs, err := store.ListContextIDs(ctx)
	if err != nil {
		return err
	}
	if len(contextIDs) != len(s.contexts) {
		return errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("staged Context set changed"))
	}
	for _, id := range contextIDs {
		_, fingerprint, present, err := store.ReadContextSnapshot(ctx, id)
		if err != nil || !present || fingerprint != s.contextFingerprints[id] {
			return errors.Join(tobari.ErrResourceSourceInvalid, err, fmt.Errorf("staged Context bytes changed"))
		}
	}
	return nil
}

func (s *InstallationMigrationStage) Rollback(ctx context.Context) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("migration source rollback is unavailable")
	}
	var result error
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	for index := len(s.committed) - 1; index >= 0; index-- {
		concept := s.committed[index]
		from := filepath.Join(s.store.configRoot, concept)
		to := filepath.Join(s.root, concept)
		if err := os.Rename(from, to); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}
	s.committed = nil
	result = errors.Join(result, syncDirectory(s.store.configRoot))
	return result
}

func (s *InstallationMigrationStage) Complete(ctx context.Context) error {
	if s == nil || len(s.committed) != 2 {
		return fmt.Errorf("migration source stage is not committed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.complete {
		return nil
	}
	cleanupRoot := filepath.Join(s.store.configRoot, migrationSourceCleanupName)
	if s.root != cleanupRoot {
		if s.root != filepath.Join(s.store.configRoot, migrationSourceStageName) {
			return tobari.ErrResourceSourceRecoveryRequired
		}
		if err := s.store.writeMigrationSourcePhase(s.root, "cleanup_started"); err != nil {
			return err
		}
		if err := s.store.phase("migration_source_cleanup_prepared"); err != nil {
			return err
		}
		if _, err := os.Lstat(cleanupRoot); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		if err := os.Rename(s.root, cleanupRoot); err != nil {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		s.root = cleanupRoot
		if err := s.store.phase("migration_source_cleanup_renamed"); err != nil {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		if err := syncDirectory(s.store.configRoot); err != nil {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		if err := s.store.phase("migration_source_cleanup_rename_synced"); err != nil {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
	}
	if err := os.RemoveAll(s.root); err != nil { // #nosec G301 -- exact private migration cleanup tombstone.
		return err
	}
	if err := s.store.phase("migration_source_cleanup_removed"); err != nil {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := syncDirectory(s.store.configRoot); err != nil {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := s.store.phase("migration_source_cleanup_remove_synced"); err != nil {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	s.complete = true
	return nil
}

func (s *InstallationMigrationStage) Abort(ctx context.Context) error {
	if s == nil || s.store == nil || s.complete {
		return nil
	}
	if _, err := readMigrationSourcePhase(s.root); errors.Is(err, os.ErrNotExist) {
		if removeErr := os.RemoveAll(s.root); removeErr != nil { // #nosec G301 -- exact incomplete migration staging root.
			return removeErr
		}
		return syncDirectory(s.store.configRoot)
	} else if err != nil {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := s.observeCommittedLocations(ctx); err != nil {
		return err
	}
	if len(s.committed) != 0 {
		if err := s.Rollback(ctx); err != nil {
			return err
		}
	}
	info, err := os.Lstat(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || validatePrivateDirectory(info) != nil || filepath.Dir(s.root) != s.store.configRoot {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := os.RemoveAll(s.root); err != nil {
		return err
	}
	return syncDirectory(s.store.configRoot)
}

func (s *Store) writeMigrationSourcePhase(root, phase string) error {
	path := filepath.Join(root, migrationSourceJournalName)
	temp := filepath.Join(root, migrationSourceJournalTempName)
	if info, err := os.Lstat(temp); err == nil {
		if validatePrivateFile(info) != nil {
			return tobari.ErrResourceSourceRecoveryRequired
		}
		if err := os.Remove(temp); err != nil {
			return err
		}
		if err := syncDirectory(root); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- exact private migration stage journal successor.
	if err != nil {
		return err
	}
	if _, err = file.WriteString(phase + "\n"); err == nil {
		err = s.phase("migration_source_phase_temp_written:" + phase)
	}
	if err == nil {
		err = file.Sync()
	}
	if err == nil {
		err = s.phase("migration_source_phase_temp_synced:" + phase)
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
	if err := s.phase("migration_source_phase_renamed:" + phase); err != nil {
		return err
	}
	if err := syncDirectory(root); err != nil {
		return err
	}
	return s.phase("migration_source_phase_parent_synced:" + phase)
}

func readMigrationSourcePhase(root string) (string, error) {
	temp := filepath.Join(root, migrationSourceJournalTempName)
	if info, err := os.Lstat(temp); err == nil {
		if validatePrivateFile(info) != nil {
			return "", tobari.ErrResourceSourceRecoveryRequired
		}
		if err := os.Remove(temp); err != nil {
			return "", err
		}
		if err := syncDirectory(root); err != nil {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(root, migrationSourceJournalName)) // #nosec G304 -- exact private migration stage journal.
	if err != nil {
		return "", err
	}
	switch string(data) {
	case "prepared\n":
		return "prepared", nil
	case "templates_renaming\n":
		return "templates_renaming", nil
	case "templates_committed\n":
		return "templates_committed", nil
	case "contexts_renaming\n":
		return "contexts_renaming", nil
	case "contexts_committed\n":
		return "contexts_committed", nil
	case "cleanup_started\n":
		return "cleanup_started", nil
	default:
		return "", fmt.Errorf("migration source journal phase is invalid")
	}
}

func syncSourceTree(root string) error {
	directories := []string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error { // #nosec G703 -- root is the exact private migration stage below the validated config root.
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("migration source stage contains a symlink")
		}
		if entry.IsDir() {
			directories = append(directories, path)
		}
		return nil
	}); err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool { return len(directories[i]) > len(directories[j]) })
	for _, directory := range directories {
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}
