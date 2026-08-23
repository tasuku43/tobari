package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type stoppedMigrationRunner struct{ commandRunner }

func (r stoppedMigrationRunner) Output(ctx context.Context, args, environment []string) ([]byte, error) {
	if len(args) > 0 && args[0] == "inspect" {
		return nil, errors.New("No such object")
	}
	return r.commandRunner.Output(ctx, args, environment)
}

func TestInstallationMigrationPreservesAuthorityAndPromotesLegacyRuntime(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{outputQueue: [][]byte{compatibleImageInspection(), imageDigestInspection()}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(context.Background(), "plain", tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeAdvanced, tobari.ManifestSourceAccessReadOnly); err != nil {
		t.Fatal(err)
	}
	plainManifest, err := runtime.readContextManifest("plain")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.writeDefaultManifest(plainManifest); err != nil {
		t.Fatal(err)
	}
	defaultLegacy, defaultPolicy := installLegacyMigrationContext(t, runtime, "default", true)
	plainLegacy, plainPolicy := installLegacyMigrationContext(t, runtime, "plain", false)

	learnedPath := filepath.Join(runtime.contextPolicyDirectory("default"), "domains", "example.com", "decision.json")
	learned := []byte("learned-rule\n")
	if err := writeAtomicBytes(learnedPath, learned); err != nil {
		t.Fatal(err)
	}
	workspacePath := filepath.Join(runtime.stateDirectory, "instances", "01912345-6789-7abc-8def-0123456789ab", "home", "marker")
	workspace := []byte("workspace-home\n")
	if err := writeAtomicBytes(workspacePath, workspace); err != nil {
		t.Fatal(err)
	}
	protectedMarkers := map[string][]byte{
		filepath.Join(runtime.stateDirectory, "roots", "fixture.json"): []byte("root-binding\n"),
	}
	for path, content := range protectedMarkers {
		if err := writeAtomicBytes(path, content); err != nil {
			t.Fatal(err)
		}
	}

	observation := runtime.observeDoctorContext(context.Background())
	if observation.Cause != doctor.ObservationCauseMigrationRequired {
		_, planErr := runtime.planInstallationMigration(context.Background())
		t.Fatalf("doctor observation = %+v; plan error = %v", observation, planErr)
	}
	report, err := runtime.MigrateInstallation(context.Background(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Changed || report.RecoveryID == nil || len(report.Contexts) != 2 {
		t.Fatalf("migration report = %+v", report)
	}
	backup := migrationBackupRoot(t, runtime)
	if active, err := runtime.readDefaultManifestName(); err != nil || active != "plain" {
		t.Fatalf("active Context = %q, error = %v", active, err)
	}
	for name, wantID := range map[string]string{"default": defaultLegacy.ID, "plain": plainLegacy.ID} {
		manifest, readErr := runtime.readContextManifestRaw(name)
		if readErr != nil {
			t.Fatalf("read migrated %s Context: %v", name, readErr)
		}
		if manifest.ID != wantID || manifest.Runtime != nil || manifest.RuntimeBinding == nil {
			t.Fatalf("migrated %s manifest = %+v", name, manifest)
		}
		if manifest.PolicyRevision != tobari.DefaultContextPolicyRevision() {
			t.Fatalf("migrated %s policy revision = %q", name, manifest.PolicyRevision)
		}
		if _, statErr := os.Stat(filepath.Join(runtime.contextPolicyDirectory(name), "preset.json")); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("migrated %s residual policy exists or is unsafe: %v", name, statErr)
		}
	}
	defaultManifest, _ := runtime.readContextManifestRaw("default")
	if defaultManifest.RuntimeBinding.Name != "legacy-default" || defaultManifest.RuntimeBinding.Ordinal != 1 {
		t.Fatalf("promoted Runtime binding = %+v", defaultManifest.RuntimeBinding)
	}
	plainManifest, _ = runtime.readContextManifestRaw("plain")
	if plainManifest.RuntimeBinding.RuntimeID != tobari.StandardRuntimeID {
		t.Fatalf("standard Runtime binding = %+v", plainManifest.RuntimeBinding)
	}
	assertMigrationBytes(t, learnedPath, learned)
	assertMigrationBytes(t, workspacePath, workspace)
	for path, content := range protectedMarkers {
		assertMigrationBytes(t, path, content)
	}
	assertMigrationBytes(t, filepath.Join(backup, "contexts", "default", "context.json"), mustMigrationJSON(t, defaultLegacy))
	assertMigrationBytes(t, filepath.Join(backup, "contexts", "default", "policy", "preset.json"), defaultPolicy)
	assertMigrationBytes(t, filepath.Join(backup, "contexts", "plain", "policy", "preset.json"), plainPolicy)

	second, err := runtime.MigrateInstallation(context.Background(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed || second.RecoveryID != nil {
		t.Fatalf("second migration = %+v", second)
	}
	if len(runner.runs) != 1 {
		t.Fatalf("Docker builds = %d, want 1", len(runner.runs))
	}
	if len(runner.runs[0].args) == 0 || runner.runs[0].args[0] != "buildx" {
		t.Fatalf("migration Docker mutation = %+v", runner.runs)
	}
}

func TestInstallationMigrationRejectsUnknownLegacyShapeWithoutMutation(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	installLegacyMigrationContext(t, runtime, "default", false)
	path := runtime.contextManifestPath("default")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(before, &document); err != nil {
		t.Fatal(err)
	}
	document["unknown_future_field"] = true
	unknown := mustMigrationJSON(t, document)
	if err := writeAtomicBytes(path, unknown); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.MigrateInstallation(context.Background(), io.Discard)
	if !errors.Is(err, tobari.ErrMigrationNotSupported) {
		t.Fatalf("migration error = %v", err)
	}
	assertMigrationBytes(t, path, unknown)
	if _, err := os.Stat(runtime.contextPolicyPath("default")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("current policy was created for rejected source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(runtime.configDirectory, "migrations")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup was created for rejected source: %v", err)
	}
}

func TestInstallationMigrationRejectsPredecessorStateRecreatedAfterCommit(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	_, legacyPolicy := installLegacyMigrationContext(t, runtime, "default", false)
	if _, err := runtime.MigrateInstallation(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	residualPath := filepath.Join(runtime.contextPolicyDirectory("default"), "preset.json")
	if err := writeAtomicBytes(residualPath, legacyPolicy); err != nil {
		t.Fatal(err)
	}
	if observation := runtime.observeDoctorPolicyData(context.Background()); observation.Cause != doctor.ObservationCauseMigrationRequired {
		t.Fatalf("residual policy doctor observation = %+v", observation)
	}

	if _, err := runtime.MigrateInstallation(context.Background(), io.Discard); !errors.Is(err, tobari.ErrMigrationSourceUnsafe) {
		t.Fatalf("recreated predecessor state error = %v", err)
	}
	if got, err := os.ReadFile(residualPath); err != nil || !bytes.Equal(got, legacyPolicy) {
		t.Fatalf("fail-closed migration changed recreated predecessor state: %q, %v", got, err)
	}
}

func TestInstallationMigrationRejectsDriftAndUnsafeSourcesWithoutContextWrites(t *testing.T) {
	tests := map[string]func(*testing.T, *Runtime){
		"duplicate manifest key": func(t *testing.T, runtime *Runtime) {
			path := runtime.contextManifestPath("default")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			raw = bytes.Replace(raw, []byte("{\n"), []byte("{\n  \"name\": \"default\",\n"), 1)
			if err := writeAtomicBytes(path, raw); err != nil {
				t.Fatal(err)
			}
		},
		"policy digest drift": func(t *testing.T, runtime *Runtime) {
			path := filepath.Join(runtime.contextPolicyDirectory("default"), "preset.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := writeAtomicBytes(path, append(raw, ' ')); err != nil {
				t.Fatal(err)
			}
		},
		"unsafe policy mode": func(t *testing.T, runtime *Runtime) {
			if err := os.Chmod(filepath.Join(runtime.contextPolicyDirectory("default"), "preset.json"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"symlink policy": func(t *testing.T, runtime *Runtime) {
			path := filepath.Join(runtime.contextPolicyDirectory("default"), "preset.json")
			target := filepath.Join(t.TempDir(), "outside.json")
			if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		},
		"unsupported standard image": func(t *testing.T, runtime *Runtime) {
			path := runtime.contextManifestPath("default")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := decodeLegacyContextManifest(raw, "default")
			if err != nil {
				t.Fatal(err)
			}
			manifest.Image = "example.invalid/runtime:old"
			if err := writeAtomicJSON(path, manifest); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.ensureContextStore(); err != nil {
				t.Fatal(err)
			}
			installLegacyMigrationContext(t, runtime, "default", false)
			mutate(t, runtime)
			manifestPath := runtime.contextManifestPath("default")
			before, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			_, err = runtime.MigrateInstallation(context.Background(), io.Discard)
			if !errors.Is(err, tobari.ErrMigrationNotSupported) && !errors.Is(err, tobari.ErrMigrationSourceUnsafe) {
				t.Fatalf("migration error = %v", err)
			}
			assertMigrationBytes(t, manifestPath, before)
			if _, err := os.Stat(runtime.contextPolicyPath("default")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("current policy was created for rejected source: %v", err)
			}
		})
	}
}

func TestInstallationMigrationRejectsRuntimeConflictBeforeContextWrites(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	installLegacyMigrationContext(t, runtime, "default", true)
	if _, err := runtime.CreateRuntime(context.Background(), "legacy-default", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicBytes(filepath.Join(runtime.runtimeSourceDirectory("legacy-default"), "Dockerfile"), []byte("FROM example.invalid/other:1\n")); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(runtime.contextManifestPath("default"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.MigrateInstallation(context.Background(), io.Discard)
	if !errors.Is(err, tobari.ErrMigrationRuntimeConflict) {
		t.Fatalf("migration error = %v", err)
	}
	assertMigrationBytes(t, runtime.contextManifestPath("default"), before)
}

func TestInstallationMigrationHonorsPreCanceledContextWithoutMutation(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	installLegacyMigrationContext(t, runtime, "default", false)
	before, err := os.ReadFile(runtime.contextManifestPath("default"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runtime.MigrateInstallation(ctx, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("migration error = %v", err)
	}
	assertMigrationBytes(t, runtime.contextManifestPath("default"), before)
}

func TestInstallationMigrationBackupFailurePreventsContextWrites(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	installLegacyMigrationContext(t, runtime, "default", false)
	before, err := os.ReadFile(runtime.contextManifestPath("default"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.configDirectory, "migrations"), []byte("blocked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.MigrateInstallation(context.Background(), io.Discard)
	if !errors.Is(err, tobari.ErrMigrationBackupFailed) {
		t.Fatalf("migration error = %v", err)
	}
	assertMigrationBytes(t, runtime.contextManifestPath("default"), before)
}

func TestInstallationMigrationWriteFailureRetainsRestartableSource(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("owner permission failure cannot be exercised as root")
	}
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	installLegacyMigrationContext(t, runtime, "default", false)
	manifestPath := runtime.contextManifestPath("default")
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	policyDirectory := runtime.contextPolicyDirectory("default")
	if err := os.Chmod(policyDirectory, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(policyDirectory, 0o700) })
	_, err = runtime.MigrateInstallation(context.Background(), io.Discard)
	if !errors.Is(err, tobari.ErrMigrationWriteFailed) {
		t.Fatalf("migration error = %v", err)
	}
	assertMigrationBytes(t, manifestPath, before)
}

func installLegacyMigrationContext(t *testing.T, runtime *Runtime, name string, customRuntime bool) (legacyContextManifest, []byte) {
	t.Helper()
	if _, wrapped := runtime.runner.(stoppedMigrationRunner); !wrapped {
		runtime.runner = stoppedMigrationRunner{commandRunner: runtime.runner}
	}
	if selected, err := runtime.readDefaultManifestName(); err == nil {
		if err := writeAtomicJSON(runtime.activeContextPath(), legacyActiveContextDocument{Name: selected}); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(runtime.defaultManifestPath()); err != nil {
			t.Fatal(err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.RemoveAll(runtime.manifestRevisionsDirectory(name)); err != nil {
		t.Fatal(err)
	}
	current, err := runtime.readContextManifestRaw(name)
	if err != nil {
		t.Fatal(err)
	}
	policy, ok := tobari.DefaultContextPolicySnapshot()
	if !ok {
		t.Fatal("default Context policy unavailable")
	}
	policy, err = tobari.ApplyNativeToolAuthReadiness(true, false, policy)
	if err != nil {
		t.Fatal(err)
	}
	legacyPolicy := legacyContextPolicy{
		SchemaVersion: policy.SchemaVersion, Name: "agent-ready",
		DestinationCeiling: policy.DestinationCeiling, MethodPolicy: policy.MethodPolicy,
		BaselineGrants: policy.BaselineGrants, BaselineTemplates: policy.BaselineTemplates,
		MCPBaselineGrants: policy.MCPBaselineGrants, BaselineDenies: policy.BaselineDenies,
		GraphQLEndpoints: policy.GraphQLEndpoints, MCPEndpoints: policy.MCPEndpoints,
		Guardrail: migrationPolicyGuardrail,
	}
	policyBytes := mustMigrationJSON(t, legacyPolicy)
	policyDigest := sha256.Sum256(policyBytes)
	legacy := legacyContextManifest{
		SchemaVersion: current.SchemaVersion, ID: current.ID, Name: current.Name,
		AgentProfile: current.AgentProfile, Image: legacyStandardRuntimeImage, PolicyMode: current.PolicyMode,
		SourceAccess: current.SourceAccess, NativeReadiness: current.NativeReadiness,
		PolicyPresetOrigin: "builtin/agent-ready", PolicyPresetRevision: "sha256:" + strings.ToLower(hexDigest(policyDigest[:])),
		ShellEnvironment: current.ShellEnvironment, GitIdentity: current.GitIdentity, Bootstrap: current.Bootstrap,
	}
	if customRuntime {
		dockerfile := []byte("FROM tobari-runtime:dev\nRUN true\n")
		sourceDigest := sha256.Sum256(dockerfile)
		digest := "sha256:" + hexDigest(sourceDigest[:])
		legacy.Image = "tobari-context-default:fixture"
		legacy.Runtime = &tobari.ManifestRuntimeRecipe{
			Kind: tobari.ManifestRuntimeKindDockerfile, File: tobari.ManifestRuntimeRecipeFile,
			BaseReference: "tobari-runtime:dev", SourceDigest: digest,
			LastBuild: &tobari.ManifestRuntimeBuild{Image: legacy.Image, ImageDigest: "sha256:" + strings.Repeat("d", 64), SourceDigest: digest},
		}
		if err := writeAtomicBytes(filepath.Join(runtime.contextDirectory(name), tobari.ManifestRuntimeRecipeFile), dockerfile); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeAtomicBytes(filepath.Join(runtime.contextPolicyDirectory(name), "preset.json"), policyBytes); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(runtime.contextPolicyPath(name)); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicJSON(runtime.contextManifestPath(name), legacy); err != nil {
		t.Fatal(err)
	}
	return legacy, policyBytes
}

func mustMigrationJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}

func hexDigest(data []byte) string {
	const digits = "0123456789abcdef"
	encoded := make([]byte, len(data)*2)
	for index, value := range data {
		encoded[index*2] = digits[value>>4]
		encoded[index*2+1] = digits[value&0x0f]
	}
	return string(encoded)
}

func assertMigrationBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s changed: got %q want %q", path, got, want)
	}
}

func migrationBackupRoot(t *testing.T, runtime *Runtime) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(runtime.configDirectory, "migrations", "pre-v1-*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("migration backup roots = %v, error = %v", matches, err)
	}
	return matches[0]
}
