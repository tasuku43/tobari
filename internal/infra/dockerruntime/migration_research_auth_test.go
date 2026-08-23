package dockerruntime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func installResearchAuthMigrationFixture(t *testing.T, runtimeStore *Runtime, manifestID string) {
	t.Helper()
	stateAuth := filepath.Join(runtimeStore.stateDirectory, "auth")
	vaultDirectory := filepath.Join(stateAuth, "contexts", manifestID)
	for _, directory := range []string{
		vaultDirectory,
		filepath.Join(stateAuth, "runtime"),
		filepath.Join(runtimeStore.configDirectory, "auth", "providers"),
	} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	envelope := map[string]any{
		"schema_version": 1,
		"context_id":     manifestID,
		"algorithm":      "AES-256-GCM",
		"nonce":          base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 12)),
		"ciphertext":     base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32)),
	}
	if err := writeAtomicJSON(filepath.Join(vaultDirectory, "vault.enc"), envelope); err != nil {
		t.Fatal(err)
	}
	if goruntime.GOOS == "linux" {
		keyDirectory := filepath.Join(stateAuth, "keys")
		if err := os.MkdirAll(keyDirectory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(keyDirectory, "root.key"), bytes.Repeat([]byte{0x33}, 32), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	workspaceID := "01912345-6789-7abc-8def-0123456789ab"
	projections := map[string][]byte{
		".tobari/research-handle":       []byte("opaque-research-handle\n"),
		".tobari/research-handle-index": []byte("opaque-research-index\n"),
	}
	registryFiles := make([]projectAuthRegistryEntry, 0, len(projections))
	for relative, projection := range projections {
		projectionPath := filepath.Join(runtimeStore.projectHomePath(workspaceID), filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(projectionPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(projectionPath, projection, 0o600); err != nil {
			t.Fatal(err)
		}
		registryFiles = append(registryFiles, projectAuthRegistryEntry{Path: relative, Digest: digestBytes(projection)})
	}
	sort.Slice(registryFiles, func(i, j int) bool { return registryFiles[i].Path < registryFiles[j].Path })
	projects := filepath.Join(stateAuth, "projects")
	if err := os.MkdirAll(projects, 0o700); err != nil {
		t.Fatal(err)
	}
	registry := projectAuthRegistry{
		SchemaVersion: projectAuthRegistrySchema,
		ProjectID:     workspaceID,
		Providers:     []projectAuthProviderBinding{},
		Files:         registryFiles,
		JSONMerges:    []projectAuthJSONMergeRegistryEntry{},
	}
	if err := writeAtomicJSON(filepath.Join(projects, workspaceID+".json"), registry); err != nil {
		t.Fatal(err)
	}
}

func newResearchAuthMigrationRuntime(t *testing.T) (*Runtime, legacyContextManifest) {
	t.Helper()
	root := t.TempDir()
	runtimeStore, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	legacy, _ := installLegacyMigrationContext(t, runtimeStore, tobari.DefaultManifestName, false)
	installResearchAuthMigrationFixture(t, runtimeStore, legacy.ID)
	return runtimeStore, legacy
}

func TestResearchAuthMigrationQuarantinesAuthorityWithoutReadingRootKey(t *testing.T) {
	runtimeStore, _ := newResearchAuthMigrationRuntime(t)
	keyCalls := 0
	runtimeStore.rootKeyLoader = func(context.Context) ([]byte, error) {
		keyCalls++
		return nil, errors.New("root key must remain untouched")
	}
	workspaceID := "01912345-6789-7abc-8def-0123456789ab"
	standardHome := filepath.Join(runtimeStore.stateDirectory, "instances", workspaceID, "home")
	standardAuth := filepath.Join(standardHome, ".config", "gh", "hosts.yml")
	if err := os.MkdirAll(filepath.Dir(standardAuth), 0o700); err != nil {
		t.Fatal(err)
	}
	standardBytes := []byte("github.example.com:\n  user: example\n")
	if err := os.WriteFile(standardAuth, standardBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	report, err := runtimeStore.MigrateInstallation(context.Background(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed || report.ResearchAuthDisposition != tobari.ResearchAuthReauthenticationRequired || keyCalls != 0 {
		t.Fatalf("migration report/key access = %+v calls=%d", report, keyCalls)
	}
	if _, err := os.Lstat(filepath.Join(runtimeStore.stateDirectory, "auth")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical research state remains reachable: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(runtimeStore.configDirectory, "auth")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical research config remains reachable: %v", err)
	}
	if got, err := os.ReadFile(standardAuth); err != nil || !bytes.Equal(got, standardBytes) {
		t.Fatalf("standard Workspace-owned native auth changed: %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(standardHome, ".tobari", "research-handle")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old research handle remains discoverable: %v", err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"vault.enc", "root.key", runtimeStore.stateDirectory, runtimeStore.configDirectory} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public migration report exposed research secret/path/owner material: %s", encoded)
		}
	}
	second, err := runtimeStore.MigrateInstallation(context.Background(), io.Discard)
	if err != nil || second.Changed || second.RecoveryID != nil || keyCalls != 0 {
		t.Fatalf("idempotent migration = %+v err=%v keyCalls=%d", second, err, keyCalls)
	}
}

func TestResearchAuthMigrationRollbackIsExactAndRefusesFreshCanonicalState(t *testing.T) {
	runtimeStore, _ := newResearchAuthMigrationRuntime(t)
	stateBefore := snapshotOwnedTree(t, filepath.Join(runtimeStore.stateDirectory, "auth"))
	configBefore := snapshotOwnedTree(t, filepath.Join(runtimeStore.configDirectory, "auth"))
	workspaceHome := runtimeStore.projectHomePath("01912345-6789-7abc-8def-0123456789ab")
	homeBefore := snapshotOwnedTree(t, workspaceHome)
	if _, err := runtimeStore.MigrateInstallation(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(runtimeStore.stateDirectory, "auth"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.rollbackResearchAuthQuarantine(); err == nil || !strings.Contains(err.Error(), "fresh canonical") {
		t.Fatalf("rollback merged fresh canonical authority: %v", err)
	}
	if err := os.Remove(filepath.Join(runtimeStore.stateDirectory, "auth")); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(workspaceHome, ".tobari", "research-handle")
	if err := os.WriteFile(artifact, []byte("fresh-state\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.rollbackResearchAuthQuarantine(); err == nil || !strings.Contains(err.Error(), "fresh Workspace") {
		t.Fatalf("rollback overwrote fresh Workspace auth state: %v", err)
	}
	if err := os.Remove(artifact); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.rollbackResearchAuthQuarantine(); err != nil {
		t.Fatal(err)
	}
	if got := snapshotOwnedTree(t, filepath.Join(runtimeStore.stateDirectory, "auth")); !slicesEqualStrings(got, stateBefore) {
		t.Fatalf("state rollback is not byte/mode identical\nwant=%v\ngot=%v", stateBefore, got)
	}
	if got := snapshotOwnedTree(t, filepath.Join(runtimeStore.configDirectory, "auth")); !slicesEqualStrings(got, configBefore) {
		t.Fatalf("config rollback is not byte/mode identical\nwant=%v\ngot=%v", configBefore, got)
	}
	if got := snapshotOwnedTree(t, workspaceHome); !slicesEqualStrings(got, homeBefore) {
		t.Fatalf("Workspace research projection rollback is not byte/mode identical\nwant=%v\ngot=%v", homeBefore, got)
	}
}

func TestResearchAuthMigrationFailsClosedBeforeMutation(t *testing.T) {
	tests := map[string]func(*testing.T, *Runtime, legacyContextManifest){
		"unknown mixed state": func(t *testing.T, runtimeStore *Runtime, _ legacyContextManifest) {
			if err := os.WriteFile(filepath.Join(runtimeStore.stateDirectory, "auth", "unknown"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"corrupt vault": func(t *testing.T, runtimeStore *Runtime, legacy legacyContextManifest) {
			if err := os.WriteFile(filepath.Join(runtimeStore.stateDirectory, "auth", "contexts", legacy.ID, "vault.enc"), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"symlinked authority": func(t *testing.T, runtimeStore *Runtime, legacy legacyContextManifest) {
			path := filepath.Join(runtimeStore.stateDirectory, "auth", "contexts", legacy.ID, "vault.enc")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(runtimeStore.stateDirectory, "outside"), path); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			runtimeStore, legacy := newResearchAuthMigrationRuntime(t)
			mutate(t, runtimeStore, legacy)
			before := snapshotOwnedTree(t, filepath.Join(runtimeStore.stateDirectory, "auth"))
			if _, err := runtimeStore.MigrateInstallation(context.Background(), io.Discard); !errors.Is(err, tobari.ErrMigrationSourceUnsafe) {
				t.Fatalf("unsafe research state error = %v", err)
			}
			after := snapshotOwnedTree(t, filepath.Join(runtimeStore.stateDirectory, "auth"))
			if !slicesEqualStrings(after, before) {
				t.Fatalf("failed preflight mutated authority\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
}

func TestResearchAuthQuarantineRejectsDigestDrift(t *testing.T) {
	runtimeStore, _ := newResearchAuthMigrationRuntime(t)
	plans, err := runtimeStore.planInstallationMigration(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := runtimeStore.planResearchAuthMigration(plans)
	if err != nil {
		t.Fatal(err)
	}
	vaults, err := filepath.Glob(filepath.Join(runtimeStore.stateDirectory, "auth", "contexts", "*", "vault.enc"))
	if err != nil || len(vaults) != 1 {
		t.Fatalf("vault fixture = %v, %v", vaults, err)
	}
	if err := os.WriteFile(vaults[0], append(bytes.Repeat([]byte{' '}, 1), mustReadFile(t, vaults[0])...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.quarantineResearchAuth(plan, tobari.DefaultManifestName); !errors.Is(err, tobari.ErrMigrationSourceChanged) {
		t.Fatalf("digest drift error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(runtimeStore.stateDirectory, "auth")); err != nil {
		t.Fatalf("digest drift moved canonical authority: %v", err)
	}
}

func TestResearchAuthQuarantineCrashPhasesExposeFullOrZeroAuthority(t *testing.T) {
	tests := []struct {
		name           string
		stateMoved     bool
		configMoved    bool
		artifactsMoved int
		wantResolvable bool
	}{
		{name: "before state move exposes full predecessor set", wantResolvable: true},
		{name: "after state move before config move exposes zero authority", stateMoved: true},
		{name: "after config move before Workspace artifacts exposes zero authority", stateMoved: true, configMoved: true},
		{name: "during Workspace artifact moves exposes zero authority", stateMoved: true, configMoved: true, artifactsMoved: 1},
		{name: "after Workspace artifact moves exposes zero authority", stateMoved: true, configMoved: true, artifactsMoved: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtimeStore, legacy := newResearchAuthMigrationRuntime(t)
			plans, err := runtimeStore.planInstallationMigration(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			plan, err := runtimeStore.planResearchAuthMigration(plans)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Artifacts) != 2 {
				t.Fatalf("crash fixture artifacts = %d, want 2", len(plan.Artifacts))
			}
			journal := researchAuthJournal{
				SchemaVersion: researchAuthJournalSchema,
				Digest:        plan.Digest, StateDigest: plan.StateDigest, ConfigDigest: plan.ConfigDigest,
				StatePresent: plan.StatePresent, ConfigPresent: plan.ConfigPresent,
				Artifacts: append([]researchAuthArtifact{}, plan.Artifacts...), DefaultManifest: tobari.DefaultManifestName,
			}
			if err := runtimeStore.ensurePrivateDirectory(filepath.Dir(runtimeStore.researchAuthJournalPath())); err != nil {
				t.Fatal(err)
			}
			if err := writeAtomicJSON(runtimeStore.researchAuthJournalPath(), journal); err != nil {
				t.Fatal(err)
			}
			advanceResearchAuthCrashFixture(t, runtimeStore, &journal, test.stateMoved, test.configMoved, test.artifactsMoved)

			if got := predecessorResearchHandleResolvable(runtimeStore, legacy.ID); got != test.wantResolvable {
				t.Fatalf("old-reader handle resolution at crash point = %t, want %t", got, test.wantResolvable)
			}
			if test.stateMoved && predecessorResearchAuthorityReachable(runtimeStore, legacy.ID) {
				t.Fatal("state moved but predecessor ciphertext/lookup authority remains reachable")
			}

			if err := runtimeStore.resumeResearchAuthQuarantine(journal); err != nil {
				t.Fatal(err)
			}
			if predecessorResearchHandleResolvable(runtimeStore, legacy.ID) {
				t.Fatal("resume re-exposed the predecessor handle to the old reader")
			}
			recovered, exists, err := runtimeStore.readResearchAuthJournal()
			if err != nil || !exists || !recovered.StateMoved || !recovered.ConfigMoved || recovered.ArtifactsMoved != len(plan.Artifacts) {
				t.Fatalf("resumed journal = %+v exists=%t err=%v", recovered, exists, err)
			}
			if err := runtimeStore.rollbackResearchAuthQuarantine(); err != nil {
				t.Fatal(err)
			}
			if !predecessorResearchHandleResolvable(runtimeStore, legacy.ID) {
				t.Fatal("rollback did not restore the complete predecessor authority set")
			}
		})
	}
}

func advanceResearchAuthCrashFixture(t *testing.T, runtimeStore *Runtime, journal *researchAuthJournal, stateMoved, configMoved bool, artifactsMoved int) {
	t.Helper()
	if stateMoved {
		stateQuarantine := runtimeStore.researchAuthStateQuarantine(journal.Digest)
		if err := runtimeStore.ensurePrivateDirectory(stateQuarantine); err != nil {
			t.Fatal(err)
		}
		if err := moveExactPrivateTree(filepath.Join(runtimeStore.stateDirectory, "auth"), filepath.Join(stateQuarantine, "state-auth"), journal.StateDigest); err != nil {
			t.Fatal(err)
		}
		journal.StateMoved = true
	}
	if configMoved {
		configQuarantine := runtimeStore.researchAuthConfigQuarantine(journal.Digest)
		if err := runtimeStore.ensurePrivateDirectory(configQuarantine); err != nil {
			t.Fatal(err)
		}
		if err := moveExactPrivateTree(filepath.Join(runtimeStore.configDirectory, "auth"), filepath.Join(configQuarantine, "config-auth"), journal.ConfigDigest); err != nil {
			t.Fatal(err)
		}
		journal.ConfigMoved = true
	}
	for index := 0; index < artifactsMoved; index++ {
		artifact := journal.Artifacts[index]
		source := filepath.Join(runtimeStore.projectHomePath(artifact.WorkspaceID), filepath.FromSlash(artifact.Relative))
		target := filepath.Join(runtimeStore.stateDirectory, "migrations", "domain-model-v1", strings.TrimPrefix(journal.Digest, "sha256:"), "research-auth", "workspace-homes", artifact.WorkspaceID, filepath.FromSlash(artifact.Relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(source, target); err != nil {
			t.Fatal(err)
		}
		journal.ArtifactsMoved = index + 1
	}
	if err := writeAtomicJSON(runtimeStore.researchAuthJournalPath(), *journal); err != nil {
		t.Fatal(err)
	}
}

// predecessorResearchHandleResolvable models the predecessor's complete
// lookup: a registry binds a projected handle to ciphertext owned by the same
// preserved Context UUID. A leftover projected file alone is not authority.
func predecessorResearchHandleResolvable(runtimeStore *Runtime, manifestID string) bool {
	if !predecessorResearchAuthorityReachable(runtimeStore, manifestID) {
		return false
	}
	workspaceID := "01912345-6789-7abc-8def-0123456789ab"
	registry, err := runtimeStore.readProjectAuthRegistry(workspaceID)
	if err != nil || len(registry.Files) == 0 {
		return false
	}
	for _, entry := range registry.Files {
		content, err := os.ReadFile(filepath.Join(runtimeStore.projectHomePath(workspaceID), filepath.FromSlash(entry.Path)))
		if err != nil || digestBytes(content) != entry.Digest {
			return false
		}
	}
	return true
}

func predecessorResearchAuthorityReachable(runtimeStore *Runtime, manifestID string) bool {
	_, vaultErr := os.Lstat(filepath.Join(runtimeStore.stateDirectory, "auth", "contexts", manifestID, "vault.enc"))
	_, registryErr := os.Lstat(runtimeStore.projectAuthRegistryPath("01912345-6789-7abc-8def-0123456789ab"))
	return vaultErr == nil && registryErr == nil
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func slicesEqualStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
