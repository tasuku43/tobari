package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func legacyRuntimeInstallationFixture(t *testing.T) (*Runtime, *managedRuntimeBuildRunner, tobari.WorkspaceAuthorityCollection, tobari.RuntimeManifest, []byte) {
	t.Helper()
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.CreateRuntime(context.Background(), "tools", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	built, err := runtime.BuildManagedRuntime(context.Background(), "tools", nil)
	if err != nil {
		t.Fatal(err)
	}
	manifest := built.Runtime
	legacyStage := filepath.Join(runtime.configDirectory, "legacy-runtimes-stage")
	legacyRoot := filepath.Join(legacyStage, manifest.Name)
	if err := os.MkdirAll(filepath.Join(legacyRoot, "revisions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := copyRuntimeSource(context.Background(), manifest.SourcePath, filepath.Join(legacyRoot, "source")); err != nil {
		t.Fatal(err)
	}
	legacy := manifest
	legacy.SourcePath = filepath.Join(runtime.runtimesDirectory(), manifest.Name, "source")
	for index := range legacy.Revisions {
		digest := strings.TrimPrefix(legacy.Revisions[index].Revision, "sha256:")
		destination := filepath.Join(legacyRoot, "revisions", digest, "source")
		if err := os.Mkdir(filepath.Dir(destination), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := copyRuntimeSource(context.Background(), manifest.Revisions[index].SnapshotPath, destination); err != nil {
			t.Fatal(err)
		}
		legacy.Revisions[index].SnapshotPath = filepath.Join(runtime.runtimesDirectory(), manifest.Name, "revisions", digest, "source")
	}
	if err := writeAtomicJSON(filepath.Join(legacyRoot, "runtime.json"), legacy); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(runtime.runtimesDirectory()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(runtime.runtimeStatesDirectory()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(legacyStage, runtime.runtimesDirectory()); err != nil {
		t.Fatal(err)
	}
	originalManifest, err := os.ReadFile(filepath.Join(runtime.runtimesDirectory(), manifest.Name, "runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	base := finalProjectionCollectionFixture(t, "")
	template := base.Templates[0]
	body := template.Current.Body.Clone()
	binding, err := manifest.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	body.EntryDefaults.Runtime = binding
	revision, err := tobari.NewWorkspaceTemplateRevision(template.ID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template.Current = revision
	template.Retained = []tobari.WorkspaceTemplateRevision{revision}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection([]tobari.WorkspaceTemplate{template}, []tobari.WorkspaceAuthorityContextRecord{}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return runtime, runner, collection, manifest, originalManifest
}

func TestInstallationRuntimeMigrationBindsBytesAndPublishesExactSplit(t *testing.T) {
	runtime, _, collection, manifest, original := legacyRuntimeInstallationFixture(t)
	digest, err := runtime.ObserveInstallationRuntimeMigration(context.Background(), collection)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := runtime.PrepareInstallationRuntimeMigration(context.Background(), collection)
	if err != nil {
		t.Fatal(err)
	}
	legacyManifest := filepath.Join(runtime.runtimesDirectory(), manifest.Name, "runtime.json")
	if err := os.WriteFile(legacyManifest, append(append([]byte{}, original...), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := runtime.ObserveInstallationRuntimeMigration(context.Background(), collection)
	if err != nil || changed == digest {
		t.Fatalf("predecessor Runtime byte drift was not rejected: %s/%v", changed, err)
	}
	if err := os.WriteFile(legacyManifest, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := stage.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	stage, err = runtime.PrepareInstallationRuntimeMigration(context.Background(), collection)
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := stage.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := stage.Complete(context.Background()); err != nil {
		t.Fatal(err)
	}
	resolved, err := runtime.ResolveRuntimeReference(context.Background(), tobari.RuntimeRef(manifest.ID))
	if err != nil || resolved.ID != manifest.ID || resolved.SourcePath != runtime.runtimeSourceDirectory(manifest.ID) || resolved.Revisions[0].SnapshotPath != filepath.Join(runtime.runtimeRevisionsDirectory(manifest.ID), strings.TrimPrefix(manifest.Revisions[0].Revision, "sha256:"), "source") {
		t.Fatalf("split Runtime publication = %+v/%v", resolved, err)
	}
	if _, err := os.Lstat(runtime.runtimeMigrationLegacyQuarantine()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("accepted predecessor quarantine remained: %v", err)
	}
}

func TestInstallationRuntimeMigrationCrashBoundariesRestoreByteIdenticalLegacy(t *testing.T) {
	for _, boundary := range []string{"journal_written", "legacy_quarantined", "config_published", "state_published"} {
		t.Run(boundary, func(t *testing.T) {
			runtime, runner, collection, manifest, original := legacyRuntimeInstallationFixture(t)
			stage, err := runtime.PrepareInstallationRuntimeMigration(context.Background(), collection)
			if err != nil {
				t.Fatal(err)
			}
			interrupted := errors.New("synthetic Runtime migration crash")
			runtime.runtimeInstallationMigrationBoundary = func(observed string) error {
				if observed == boundary {
					return interrupted
				}
				return nil
			}
			if err := stage.Commit(context.Background()); !errors.Is(err, interrupted) {
				t.Fatalf("migration interruption at %s = %v", boundary, err)
			}
			if _, err := runtime.ListRuntimes(context.Background()); err == nil || !strings.Contains(err.Error(), "requires recovery") {
				t.Fatalf("ordinary Runtime read did not fail closed at %s: %v", boundary, err)
			}
			restarted, err := newRuntime(runtime.configDirectory, runtime.stateDirectory, runner)
			if err != nil {
				t.Fatal(err)
			}
			if err := restarted.rollbackInterruptedRuntimeInstallationMigration(); err != nil {
				t.Fatalf("restart rollback at %s: %v", boundary, err)
			}
			got, err := os.ReadFile(filepath.Join(restarted.runtimesDirectory(), manifest.Name, "runtime.json"))
			if err != nil || !bytes.Equal(got, original) {
				t.Fatalf("legacy manifest changed at %s: equal=%t err=%v", boundary, bytes.Equal(got, original), err)
			}
			if _, err := os.Lstat(restarted.runtimeStatesDirectory()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("split state remained after rollback at %s: %v", boundary, err)
			}
			if _, err := restarted.ObserveInstallationRuntimeMigration(context.Background(), collection); err != nil {
				t.Fatalf("restored predecessor invalid at %s: %v", boundary, err)
			}
		})
	}
}

func TestInstallationRuntimeMigrationVerifyBindsEntirePublishedConfigAndStateTrees(t *testing.T) {
	for _, target := range []string{"config source", "state manifest"} {
		t.Run(target, func(t *testing.T) {
			runtime, _, collection, manifest, _ := legacyRuntimeInstallationFixture(t)
			stage, err := runtime.PrepareInstallationRuntimeMigration(context.Background(), collection)
			if err != nil {
				t.Fatal(err)
			}
			if err := stage.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(runtime.runtimeSourceDirectory(manifest.ID), "Dockerfile")
			if target == "state manifest" {
				path = runtime.runtimeManifestPath(manifest.ID)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := stage.Verify(context.Background()); err == nil || !strings.Contains(err.Error(), "tree differs") {
				t.Fatalf("tampered %s Verify = %v", target, err)
			}
		})
	}
}

func TestInstallationRuntimeMigrationJournalRecoveryRejectsMalformedAndConflictingSuccessors(t *testing.T) {
	for _, kind := range []string{"malformed", "conflicting"} {
		t.Run(kind, func(t *testing.T) {
			runtime, _, collection, _, _ := legacyRuntimeInstallationFixture(t)
			stage, err := runtime.PrepareInstallationRuntimeMigration(context.Background(), collection)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := stage.ExpectedIdentity()
			if err != nil {
				t.Fatal(err)
			}
			if kind == "conflicting" {
				journal := runtimeInstallationMigrationJournal{SchemaVersion: runtimeInstallationMigrationJournalSchema, RuntimeIDs: installationRuntimeIDs(collection), ExpectedTree: expected}
				if err := writeAtomicJSON(runtime.runtimeMigrationJournalPath(), journal); err != nil {
					t.Fatal(err)
				}
				journal.ExpectedTree = tobari.SemanticDigest("sha256:" + strings.Repeat("9", 64))
				if err := writeAtomicJSON(runtime.runtimeMigrationJournalTempPath(), journal); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(runtime.runtimeMigrationJournalTempPath(), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.PrepareInstallationRuntimeMigration(context.Background(), collection, true); err == nil || !strings.Contains(err.Error(), "journal") {
				t.Fatalf("%s Runtime journal successor = %v", kind, err)
			}
		})
	}
}
