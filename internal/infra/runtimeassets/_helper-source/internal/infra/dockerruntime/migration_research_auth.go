package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	researchAuthJournalSchema = 1
	maxResearchAuthFileBytes  = 4 * 1024 * 1024
)

type researchAuthArtifact struct {
	WorkspaceID string `json:"workspace_id"`
	Relative    string `json:"relative_path"`
	Digest      string `json:"digest"`
}

type researchAuthPlan struct {
	Digest        string
	StateDigest   string
	ConfigDigest  string
	StatePresent  bool
	ConfigPresent bool
	Artifacts     []researchAuthArtifact
}

type researchAuthJournal struct {
	SchemaVersion     int                    `json:"schema_version"`
	Digest            string                 `json:"digest"`
	StateDigest       string                 `json:"state_digest,omitempty"`
	ConfigDigest      string                 `json:"config_digest,omitempty"`
	StatePresent      bool                   `json:"state_present"`
	ConfigPresent     bool                   `json:"config_present"`
	Artifacts         []researchAuthArtifact `json:"workspace_artifacts"`
	StateMoved        bool                   `json:"state_moved"`
	ConfigMoved       bool                   `json:"config_moved"`
	ArtifactsMoved    int                    `json:"artifacts_moved"`
	DefaultManifest   string                 `json:"default_manifest"`
	RecoveryID        string                 `json:"recovery_id,omitempty"`
	ContextsCommitted bool                   `json:"contexts_committed"`
	SelectorCommitted bool                   `json:"selector_committed"`
	Committed         bool                   `json:"committed"`
}

func (r *Runtime) researchAuthJournalPath() string {
	return filepath.Join(r.stateDirectory, "migrations", "domain-model-v1-research-auth.json")
}

func (r *Runtime) researchAuthStateQuarantine(digest string) string {
	return filepath.Join(r.stateDirectory, "migrations", "domain-model-v1", strings.TrimPrefix(digest, "sha256:"), "research-auth")
}

func (r *Runtime) researchAuthConfigQuarantine(digest string) string {
	return filepath.Join(r.configDirectory, "migrations", "domain-model-v1", strings.TrimPrefix(digest, "sha256:"), "research-auth")
}

func (r *Runtime) planResearchAuthMigration(manifests []migrationContextPlan) (researchAuthPlan, error) {
	manifestIDs := make(map[string]struct{}, len(manifests))
	for _, manifest := range manifests {
		manifestIDs[manifest.manifest.ID] = struct{}{}
	}
	statePath := filepath.Join(r.stateDirectory, "auth")
	configPath := filepath.Join(r.configDirectory, "auth")
	statePresent, err := privateDirectoryPresent(statePath)
	if err != nil {
		return researchAuthPlan{}, fmt.Errorf("research auth state: %w", err)
	}
	configPresent, err := privateDirectoryPresent(configPath)
	if err != nil {
		return researchAuthPlan{}, fmt.Errorf("research auth config: %w", err)
	}
	if configPresent {
		if err := r.validateResearchAuthConfig(); err != nil {
			return researchAuthPlan{}, err
		}
	}
	artifacts := []researchAuthArtifact{}
	if statePresent {
		artifacts, err = r.validateResearchAuthState(manifestIDs)
		if err != nil {
			return researchAuthPlan{}, err
		}
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].WorkspaceID+"\x00"+artifacts[i].Relative < artifacts[j].WorkspaceID+"\x00"+artifacts[j].Relative
	})
	hash := sha256.New()
	var stateDigest, configDigest string
	for _, source := range []struct {
		label   string
		path    string
		present bool
	}{{"state", statePath, statePresent}, {"config", configPath, configPresent}} {
		_, _ = hash.Write([]byte(source.label))
		if source.present {
			digest, digestErr := digestPrivateTree(source.path)
			if digestErr != nil {
				return researchAuthPlan{}, digestErr
			}
			_, _ = hash.Write([]byte(digest))
			if source.label == "state" {
				stateDigest = digest
			} else {
				configDigest = digest
			}
		} else {
			_, _ = hash.Write([]byte("absent"))
		}
	}
	for _, artifact := range artifacts {
		_, _ = hash.Write([]byte(artifact.WorkspaceID + "\x00" + artifact.Relative + "\x00" + artifact.Digest))
	}
	return researchAuthPlan{
		Digest: "sha256:" + hex.EncodeToString(hash.Sum(nil)), StateDigest: stateDigest,
		ConfigDigest: configDigest, StatePresent: statePresent,
		ConfigPresent: configPresent, Artifacts: artifacts,
	}, nil
}

func (r *Runtime) validateMigrationRuntimeQuiescence(ctx context.Context) error {
	for _, name := range clusterComponentOrder {
		component, err := r.inspectContainer(ctx, name, clusterContainers[name])
		if err != nil {
			return fmt.Errorf("%w: cluster state cannot be inspected", tobari.ErrMigrationSourceUnsafe)
		}
		switch component.State {
		case "absent", "exited", "dead":
		default:
			return fmt.Errorf("%w: cluster must be stopped before migration", tobari.ErrMigrationSourceUnsafe)
		}
	}
	entries, err := os.ReadDir(r.instancesDirectory())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || tobari.ValidateWorkspaceID(entry.Name()) != nil {
			return fmt.Errorf("%w: Workspace instance collection is unsafe", tobari.ErrMigrationSourceUnsafe)
		}
		container, _, err := tobari.ProjectResourceNames(entry.Name())
		if err != nil {
			return err
		}
		output, inspectErr := r.runner.Output(ctx, []string{"inspect", "--format", "{{json .ExecIDs}}", container}, os.Environ())
		if inspectErr != nil {
			if isMissingDockerResource(inspectErr, output) {
				continue
			}
			return fmt.Errorf("%w: Workspace attachment state cannot be inspected", tobari.ErrMigrationSourceUnsafe)
		}
		var execIDs []string
		if json.Unmarshal(bytes.TrimSpace(output), &execIDs) != nil {
			return fmt.Errorf("%w: Workspace attachment state is invalid", tobari.ErrMigrationSourceUnsafe)
		}
		for _, execID := range execIDs {
			if strings.TrimSpace(execID) != "" {
				return fmt.Errorf("%w: live Workspace attachment blocks migration", tobari.ErrMigrationSourceUnsafe)
			}
		}
	}
	return nil
}

func privateDirectoryPresent(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return false, fmt.Errorf("path is not an owner-only real directory")
	}
	return true, nil
}

func (r *Runtime) validateResearchAuthConfig() error {
	root := filepath.Join(r.configDirectory, "auth")
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != "providers" || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: research auth config contains an unknown entry", tobari.ErrMigrationSourceUnsafe)
		}
	}
	providers := r.authProviderDirectory()
	if present, err := privateDirectoryPresent(providers); err != nil {
		return err
	} else if present {
		children, err := os.ReadDir(providers)
		if err != nil {
			return err
		}
		for _, child := range children {
			if child.IsDir() || child.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(child.Name(), ".json") {
				return fmt.Errorf("%w: research provider collection contains an unknown entry", tobari.ErrMigrationSourceUnsafe)
			}
		}
	}
	if _, err := r.loadAuthProviders(); err != nil {
		return fmt.Errorf("%w: research provider collection: %v", tobari.ErrMigrationSourceUnsafe, err)
	}
	return nil
}

func (r *Runtime) validateResearchAuthState(manifestIDs map[string]struct{}) ([]researchAuthArtifact, error) {
	root := filepath.Join(r.stateDirectory, "auth")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{"contexts": true, "runtime": true, "projection": true, "projects": true}
	if runtime.GOOS == "linux" {
		allowed["keys"] = true
	}
	present := map[string]bool{}
	for _, entry := range entries {
		if !allowed[entry.Name()] || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: research auth state contains an unknown or mixed entry", tobari.ErrMigrationSourceUnsafe)
		}
		if ok, dirErr := privateDirectoryPresent(filepath.Join(root, entry.Name())); dirErr != nil || !ok {
			return nil, fmt.Errorf("%w: research auth state directory is unsafe", tobari.ErrMigrationSourceUnsafe)
		}
		present[entry.Name()] = true
	}
	vaults, err := r.validateResearchVaults(manifestIDs, filepath.Join(root, "contexts"), present["contexts"])
	if err != nil {
		return nil, err
	}
	if err := validateEmptyResearchRuntime(filepath.Join(root, "runtime"), present["runtime"]); err != nil {
		return nil, err
	}
	if err := r.validateResearchProjection(filepath.Join(root, "projection"), present["projection"]); err != nil {
		return nil, err
	}
	artifacts, err := r.validateResearchProjectRegistries(filepath.Join(root, "projects"), present["projects"])
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "linux" {
		keyPresent, keyErr := validateLinuxResearchRootKey(filepath.Join(root, "keys"), present["keys"])
		if keyErr != nil {
			return nil, keyErr
		}
		if vaults && !keyPresent {
			return nil, fmt.Errorf("%w: encrypted research state has no Linux root key", tobari.ErrMigrationSourceUnsafe)
		}
	}
	return artifacts, nil
}

func (r *Runtime) validateResearchVaults(manifestIDs map[string]struct{}, path string, present bool) (bool, error) {
	if !present {
		return false, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	found := false
	for _, entry := range entries {
		if tobari.ValidateWorkspaceManifestID(entry.Name()) != nil || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("%w: research vault owner is invalid", tobari.ErrMigrationSourceUnsafe)
		}
		if _, ok := manifestIDs[entry.Name()]; !ok {
			return false, fmt.Errorf("%w: research vault has no Manifest owner", tobari.ErrMigrationSourceUnsafe)
		}
		directory := filepath.Join(path, entry.Name())
		if ok, dirErr := privateDirectoryPresent(directory); dirErr != nil || !ok {
			return false, fmt.Errorf("%w: research vault directory is unsafe", tobari.ErrMigrationSourceUnsafe)
		}
		children, err := os.ReadDir(directory)
		if err != nil || len(children) != 1 || children[0].Name() != "vault.enc" || children[0].IsDir() || children[0].Type()&os.ModeSymlink != 0 {
			return false, fmt.Errorf("%w: research vault collection is incomplete", tobari.ErrMigrationSourceUnsafe)
		}
		data, err := readPrivateMigrationFile(filepath.Join(directory, "vault.enc"), 1024*1024)
		if err != nil || validateResearchVaultEnvelope(data, entry.Name()) != nil {
			return false, fmt.Errorf("%w: research vault envelope is corrupt", tobari.ErrMigrationSourceUnsafe)
		}
		found = true
	}
	return found, nil
}

func validateResearchVaultEnvelope(data []byte, manifestID string) error {
	var envelope struct {
		SchemaVersion int    `json:"schema_version"`
		ContextID     string `json:"context_id"`
		Algorithm     string `json:"algorithm"`
		Nonce         string `json:"nonce"`
		Ciphertext    string `json:"ciphertext"`
	}
	if validateNoDuplicateJSONKeys(data) != nil || decodeStrictMigrationJSON(data, &envelope) != nil ||
		envelope.SchemaVersion != 1 || envelope.ContextID != manifestID || envelope.Algorithm != "AES-256-GCM" {
		return fmt.Errorf("invalid envelope")
	}
	nonce, err := base64.RawURLEncoding.Strict().DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != 12 {
		return fmt.Errorf("invalid nonce")
	}
	ciphertext, err := base64.RawURLEncoding.Strict().DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) < 16 {
		return fmt.Errorf("invalid ciphertext")
	}
	return nil
}

func validateEmptyResearchRuntime(path string, present bool) error {
	if !present {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("%w: research runtime directory is not stopped", tobari.ErrMigrationSourceUnsafe)
	}
	return nil
}

func (r *Runtime) validateResearchProjection(path string, present bool) error {
	if !present {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	if len(entries) != 1 || entries[0].Name() != "providers.json" || entries[0].IsDir() || entries[0].Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: research provider projection is ambiguous", tobari.ErrMigrationSourceUnsafe)
	}
	data, err := readPrivateMigrationFile(filepath.Join(path, "providers.json"), maxResearchAuthFileBytes)
	if err != nil {
		return err
	}
	var observed authbroker.Projection
	if validateNoDuplicateJSONKeys(data) != nil || decodeStrictMigrationJSON(data, &observed) != nil {
		return fmt.Errorf("%w: research provider projection is invalid", tobari.ErrMigrationSourceUnsafe)
	}
	expected, err := r.loadAuthProviders()
	if err != nil {
		return err
	}
	left, _ := json.Marshal(observed)
	right, _ := json.Marshal(expected)
	if !bytes.Equal(left, right) {
		return fmt.Errorf("%w: research provider projection drifted", tobari.ErrMigrationSourceUnsafe)
	}
	return nil
}

func (r *Runtime) validateResearchProjectRegistries(path string, present bool) ([]researchAuthArtifact, error) {
	if !present {
		return []researchAuthArtifact{}, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	artifacts := []researchAuthArtifact{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("%w: research Workspace registry contains an unknown entry", tobari.ErrMigrationSourceUnsafe)
		}
		workspaceID := strings.TrimSuffix(entry.Name(), ".json")
		if tobari.ValidateWorkspaceID(workspaceID) != nil {
			return nil, fmt.Errorf("%w: research Workspace registry owner is invalid", tobari.ErrMigrationSourceUnsafe)
		}
		data, err := readPrivateMigrationFile(filepath.Join(path, entry.Name()), 256*1024)
		if err != nil {
			return nil, err
		}
		var registry projectAuthRegistry
		if validateNoDuplicateJSONKeys(data) != nil || decodeStrictMigrationJSON(data, &registry) != nil || validateProjectAuthRegistry(registry, workspaceID) != nil {
			return nil, fmt.Errorf("%w: research Workspace registry is invalid", tobari.ErrMigrationSourceUnsafe)
		}
		home, err := os.OpenRoot(r.projectHomePath(workspaceID))
		if err != nil {
			return nil, fmt.Errorf("%w: research Workspace home is unavailable", tobari.ErrMigrationSourceUnsafe)
		}
		for _, item := range registry.Files {
			content, readErr := home.ReadFile(filepath.FromSlash(item.Path))
			if readErr != nil || digestBytes(content) != item.Digest {
				_ = home.Close()
				return nil, fmt.Errorf("%w: research Workspace handle projection drifted", tobari.ErrMigrationSourceUnsafe)
			}
			artifacts = append(artifacts, researchAuthArtifact{WorkspaceID: workspaceID, Relative: item.Path, Digest: item.Digest})
		}
		for _, item := range registry.JSONMerges {
			content, readErr := home.ReadFile(filepath.FromSlash(item.Path))
			fields, fieldErr := projectAuthJSONMergeFields(content)
			if readErr != nil || fieldErr != nil || strings.Join(fields, "\x00") != strings.Join(item.Fields, "\x00") {
				_ = home.Close()
				return nil, fmt.Errorf("%w: mixed standard and research Workspace auth state cannot be separated", tobari.ErrMigrationSourceUnsafe)
			}
			artifacts = append(artifacts, researchAuthArtifact{WorkspaceID: workspaceID, Relative: item.Path, Digest: digestBytes(content)})
		}
		if err := home.Close(); err != nil {
			return nil, err
		}
	}
	return artifacts, nil
}

func validateLinuxResearchRootKey(path string, present bool) (bool, error) {
	if !present {
		return false, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	if len(entries) == 0 {
		return false, nil
	}
	if len(entries) != 1 || entries[0].Name() != "root.key" || entries[0].IsDir() || entries[0].Type()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("%w: Linux research root-key state is invalid", tobari.ErrMigrationSourceUnsafe)
	}
	data, err := readPrivateMigrationFile(filepath.Join(path, "root.key"), 32)
	if err != nil || len(data) != 32 {
		return false, fmt.Errorf("%w: Linux research root-key state is invalid", tobari.ErrMigrationSourceUnsafe)
	}
	clear(data)
	return true, nil
}

func digestPrivateTree(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("private migration tree contains an unsafe entry")
		}
		_, _ = hash.Write([]byte(filepath.ToSlash(relative)))
		_, _ = hash.Write([]byte{0})
		if entry.IsDir() {
			if info.Mode().Perm()&0o500 != 0o500 {
				return fmt.Errorf("private migration directory is inaccessible")
			}
			return nil
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxResearchAuthFileBytes {
			return fmt.Errorf("private migration tree contains an unsupported file")
		}
		data, err := readPrivateMigrationFile(path, maxResearchAuthFileBytes)
		if err != nil {
			return err
		}
		_, _ = hash.Write(data)
		return nil
	})
	if err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func (r *Runtime) quarantineResearchAuth(plan researchAuthPlan, defaultManifest string) error {
	journal := researchAuthJournal{
		SchemaVersion: researchAuthJournalSchema, Digest: plan.Digest, StateDigest: plan.StateDigest,
		ConfigDigest: plan.ConfigDigest, StatePresent: plan.StatePresent,
		ConfigPresent: plan.ConfigPresent, Artifacts: append([]researchAuthArtifact{}, plan.Artifacts...),
		DefaultManifest: defaultManifest,
	}
	if err := r.ensurePrivateDirectory(filepath.Dir(r.researchAuthJournalPath())); err != nil {
		return err
	}
	if err := writeAtomicJSON(r.researchAuthJournalPath(), journal); err != nil {
		return err
	}
	return r.resumeResearchAuthQuarantine(journal)
}

func (r *Runtime) resumeResearchAuthQuarantine(journal researchAuthJournal) error {
	if journal.SchemaVersion != researchAuthJournalSchema || tobari.ValidateDigest(journal.Digest) != nil ||
		(journal.StatePresent && tobari.ValidateDigest(journal.StateDigest) != nil) ||
		(journal.ConfigPresent && tobari.ValidateDigest(journal.ConfigDigest) != nil) ||
		journal.ArtifactsMoved < 0 || journal.ArtifactsMoved > len(journal.Artifacts) {
		return fmt.Errorf("%w: research auth migration journal is invalid", tobari.ErrMigrationSourceUnsafe)
	}
	stateQuarantine := r.researchAuthStateQuarantine(journal.Digest)
	configQuarantine := r.researchAuthConfigQuarantine(journal.Digest)
	if err := r.ensurePrivateDirectory(stateQuarantine); err != nil {
		return err
	}
	if err := r.ensurePrivateDirectory(configQuarantine); err != nil {
		return err
	}
	if journal.StatePresent && !journal.StateMoved {
		if err := moveExactPrivateTree(filepath.Join(r.stateDirectory, "auth"), filepath.Join(stateQuarantine, "state-auth"), journal.StateDigest); err != nil {
			return err
		}
		journal.StateMoved = true
		if err := writeAtomicJSON(r.researchAuthJournalPath(), journal); err != nil {
			return err
		}
	}
	if journal.ConfigPresent && !journal.ConfigMoved {
		if err := moveExactPrivateTree(filepath.Join(r.configDirectory, "auth"), filepath.Join(configQuarantine, "config-auth"), journal.ConfigDigest); err != nil {
			return err
		}
		journal.ConfigMoved = true
		if err := writeAtomicJSON(r.researchAuthJournalPath(), journal); err != nil {
			return err
		}
	}
	stateRoot, err := os.OpenRoot(r.stateDirectory)
	if err != nil {
		return err
	}
	defer stateRoot.Close()
	for index := journal.ArtifactsMoved; index < len(journal.Artifacts); index++ {
		artifact := journal.Artifacts[index]
		source := filepath.Join("instances", artifact.WorkspaceID, "home", filepath.FromSlash(artifact.Relative))
		target := filepath.Join("migrations", "domain-model-v1", strings.TrimPrefix(journal.Digest, "sha256:"), "research-auth", "workspace-homes", artifact.WorkspaceID, filepath.FromSlash(artifact.Relative))
		content, sourceErr := stateRoot.ReadFile(source)
		if errors.Is(sourceErr, os.ErrNotExist) {
			retained, targetErr := stateRoot.ReadFile(target)
			if targetErr != nil || digestBytes(retained) != artifact.Digest {
				return fmt.Errorf("%w: research Workspace projection recovery is ambiguous", tobari.ErrMigrationSourceUnsafe)
			}
			journal.ArtifactsMoved = index + 1
			if err := writeAtomicJSON(r.researchAuthJournalPath(), journal); err != nil {
				return err
			}
			continue
		}
		if sourceErr != nil || digestBytes(content) != artifact.Digest {
			return fmt.Errorf("%w: research Workspace projection changed during migration", tobari.ErrMigrationSourceChanged)
		}
		if err := stateRoot.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if _, err := stateRoot.Lstat(target); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: research auth quarantine target already exists", tobari.ErrMigrationSourceUnsafe)
		}
		if err := stateRoot.Rename(source, target); err != nil {
			return err
		}
		journal.ArtifactsMoved = index + 1
		if err := writeAtomicJSON(r.researchAuthJournalPath(), journal); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) readResearchAuthJournal() (researchAuthJournal, bool, error) {
	var journal researchAuthJournal
	if err := readStrictJSON(r.researchAuthJournalPath(), &journal); errors.Is(err, os.ErrNotExist) {
		return researchAuthJournal{}, false, nil
	} else if err != nil {
		return researchAuthJournal{}, false, err
	}
	if journal.SchemaVersion != researchAuthJournalSchema || tobari.ValidateDigest(journal.Digest) != nil || journal.Artifacts == nil {
		return researchAuthJournal{}, false, fmt.Errorf("research auth migration journal is invalid")
	}
	return journal, true, nil
}

// rollbackResearchAuthQuarantine is an internal recovery boundary. It refuses
// to merge with or overwrite any fresh canonical authentication state.
func (r *Runtime) rollbackResearchAuthQuarantine() error {
	journal, exists, err := r.readResearchAuthJournal()
	if err != nil || !exists {
		return err
	}
	if journal.StatePresent {
		if _, err := os.Lstat(filepath.Join(r.stateDirectory, "auth")); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("fresh canonical research auth state blocks rollback")
		}
	}
	if journal.ConfigPresent {
		if _, err := os.Lstat(filepath.Join(r.configDirectory, "auth")); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("fresh canonical research auth config blocks rollback")
		}
	}
	stateRoot, err := os.OpenRoot(r.stateDirectory)
	if err != nil {
		return err
	}
	defer stateRoot.Close()
	for _, artifact := range journal.Artifacts {
		destination := filepath.Join("instances", artifact.WorkspaceID, "home", filepath.FromSlash(artifact.Relative))
		if _, err := stateRoot.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("fresh Workspace auth state blocks rollback")
		}
	}
	for index := len(journal.Artifacts) - 1; index >= 0; index-- {
		artifact := journal.Artifacts[index]
		source := filepath.Join("migrations", "domain-model-v1", strings.TrimPrefix(journal.Digest, "sha256:"), "research-auth", "workspace-homes", artifact.WorkspaceID, filepath.FromSlash(artifact.Relative))
		destination := filepath.Join("instances", artifact.WorkspaceID, "home", filepath.FromSlash(artifact.Relative))
		content, err := stateRoot.ReadFile(source)
		if err != nil || digestBytes(content) != artifact.Digest {
			return fmt.Errorf("quarantined Workspace auth state is invalid")
		}
		if err := stateRoot.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return err
		}
		if err := stateRoot.Rename(source, destination); err != nil {
			return err
		}
	}
	if journal.ConfigPresent {
		if err := restoreExactPrivateTree(filepath.Join(r.researchAuthConfigQuarantine(journal.Digest), "config-auth"), filepath.Join(r.configDirectory, "auth"), journal.ConfigDigest); err != nil {
			return err
		}
	}
	// State is restored last. Until this rename succeeds, no predecessor
	// ciphertext or lookup registry is reachable at an ordinary reader path.
	if journal.StatePresent {
		if err := restoreExactPrivateTree(filepath.Join(r.researchAuthStateQuarantine(journal.Digest), "state-auth"), filepath.Join(r.stateDirectory, "auth"), journal.StateDigest); err != nil {
			return err
		}
	}
	if err := os.Remove(r.researchAuthJournalPath()); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(r.researchAuthJournalPath()))
}

func restoreExactPrivateTree(source, target, expectedDigest string) error {
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("fresh canonical state blocks rollback")
	}
	digest, err := digestPrivateTree(source)
	if err != nil || digest != expectedDigest {
		return fmt.Errorf("quarantined state identity is invalid")
	}
	if err := os.Rename(source, target); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(source)); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(target))
}

func moveExactPrivateTree(source, target, expectedDigest string) error {
	sourceInfo, sourceErr := os.Lstat(source)
	targetInfo, targetErr := os.Lstat(target)
	if errors.Is(sourceErr, os.ErrNotExist) && targetErr == nil && targetInfo.IsDir() && targetInfo.Mode()&os.ModeSymlink == 0 {
		digest, err := digestPrivateTree(target)
		if err != nil || digest != expectedDigest {
			return fmt.Errorf("quarantine target identity is invalid")
		}
		return nil
	}
	if sourceErr != nil || sourceInfo == nil || !sourceInfo.IsDir() || sourceInfo.Mode()&os.ModeSymlink != 0 || !errors.Is(targetErr, os.ErrNotExist) {
		return fmt.Errorf("quarantine source or target state is invalid")
	}
	digest, err := digestPrivateTree(source)
	if err != nil || digest != expectedDigest {
		return fmt.Errorf("%w: research auth source changed during migration", tobari.ErrMigrationSourceChanged)
	}
	if err := os.Rename(source, target); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(source)); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(target))
}
