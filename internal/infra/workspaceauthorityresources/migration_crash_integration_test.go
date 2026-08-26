package workspaceauthorityresources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritysource"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

const composedRuntimeID = "01912345-6789-7abc-8def-0123456789c1"

func realResourcesMigrationAdapter(t *testing.T, root string, boundary string) (*Adapter, *dockerruntime.Runtime, tobari.WorkspaceAuthorityCollection) {
	t.Helper()
	configHome := filepath.Join(root, "config-home")
	stateHome := filepath.Join(root, "state-home")
	dataHome := filepath.Join(root, "data-home")
	for _, path := range []string{configHome, stateHome, dataHome} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	runtime, err := dockerruntime.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	configRoot, _ := runtime.ResourceSourceRoot()
	authorityRoot, _ := runtime.FinalWorkspaceAuthorityRoot()
	marker := filepath.Join(root, "fixture-created")
	collection := realResourcesMigrationCollection(t, configRoot)
	if _, err := os.Lstat(marker); errors.Is(err, os.ErrNotExist) {
		writeLegacyCustomRuntime(t, configRoot)
		if err := os.MkdirAll(authorityRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(collection)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(authorityRoot, "authority.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(marker, []byte("created\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store, err := workspaceauthoritystore.NewFinalOnly(authorityRoot, runtime)
	if err != nil {
		t.Fatal(err)
	}
	mutator, err := workspaceauthoritystore.NewMutator(context.Background(), store, runtime, runtime, runtime, runtime, runtime)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := workspaceauthoritysource.New(configRoot)
	if err != nil {
		t.Fatal(err)
	}
	crash := func(observed string) error {
		if observed == boundary {
			os.Exit(91)
		}
		return nil
	}
	if boundary != "" {
		sources.SetInstallationMigrationBoundaryForTest(crash)
		runtime.SetInstallationMigrationBoundaryForTest(crash)
		mutator.SetInstallationMigrationBoundaryForTest(crash)
	}
	prepareRuntime := func(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, recovery bool) (workspaceauthoritystore.InstallationMigrationSourceStage, error) {
		return runtime.PrepareInstallationRuntimeMigration(ctx, collection, recovery)
	}
	adapter, err := New(store, mutator, sources, runtime.ObserveInstallationRuntimeMigration, prepareRuntime)
	if err != nil {
		t.Fatal(err)
	}
	return adapter, runtime, collection
}

func writeLegacyCustomRuntime(t *testing.T, configRoot string) {
	t.Helper()
	revision := "sha256:" + strings.Repeat("c", 64)
	root := filepath.Join(configRoot, "runtimes", "tools")
	source := filepath.Join(root, "source")
	snapshot := filepath.Join(root, "revisions", strings.TrimPrefix(revision, "sha256:"), "source")
	for _, path := range []string{source, snapshot} {
		if err := os.MkdirAll(filepath.Join(path, "nested"), 0o700); err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string]string{"Dockerfile": "FROM example.invalid/base@sha256:" + strings.Repeat("d", 64) + "\n", "nested/tool.sh": "#!/bin/sh\nexit 0\n"} {
			if err := os.WriteFile(filepath.Join(path, name), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	manifest := tobari.RuntimeManifest{SchemaVersion: tobari.RuntimeSchemaVersion, ID: composedRuntimeID, Name: "tools", Kind: tobari.RuntimeKindManaged, SourcePath: source, Revisions: []tobari.RuntimeRevision{{Ordinal: 1, Revision: revision, Image: "tobari-runtime:tools-1", ImageDigest: "sha256:" + strings.Repeat("e", 64), CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), SnapshotPath: snapshot}}}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, "runtime.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func realResourcesMigrationCollection(t *testing.T, configRoot string) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	binding := tobari.RuntimeBinding{RuntimeID: composedRuntimeID, Name: "tools", Revision: "sha256:" + strings.Repeat("c", 64), Ordinal: 1, Image: "tobari-runtime:tools-1"}
	body := tobari.WorkspaceTemplateBody{Boundary: tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadOnly, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}}, Policy: tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}}, EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: binding}, SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{}}
	id := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789c2")
	revision, err := tobari.NewWorkspaceTemplateRevision(id, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: id, Name: "tools-template", Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision}}
	if err := tobari.InitializeWorkspaceTemplateMetadata(&template); err != nil {
		t.Fatal(err)
	}
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789c3")
	contextBinding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, ProjectRoot: "/workspace/custom-runtime", TemplateID: id}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection([]tobari.WorkspaceTemplate{template}, []tobari.WorkspaceAuthorityContextRecord{{Context: contextBinding, PolicyMemory: memory}}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return collection
}

func TestRealResourcesMigrationWithCustomRuntimeSurvivesProcessDeath(t *testing.T) {
	boundaries := []string{
		"migration_source_phase_temp_written:prepared",
		"migration_source_phase_temp_synced:prepared",
		"migration_source_phase_renamed:prepared",
		"migration_source_phase_parent_synced:prepared",
		"migration_source_phase_temp_written:templates_renaming",
		"migration_source_phase_temp_synced:templates_renaming",
		"migration_source_phase_renamed:templates_renaming",
		"migration_source_phase_parent_synced:templates_renaming",
		"migration_source_rename_prepared:templates",
		"migration_source_renamed:templates",
		"migration_source_synced:templates",
		"migration_source_committed:templates",
		"migration_source_phase_temp_written:contexts_renaming",
		"migration_source_phase_temp_synced:contexts_renaming",
		"migration_source_phase_renamed:contexts_renaming",
		"migration_source_phase_parent_synced:contexts_renaming",
		"migration_source_rename_prepared:contexts",
		"migration_source_renamed:contexts",
		"migration_source_synced:contexts",
		"migration_source_committed:contexts",
		"migration_source_phase_temp_written:cleanup_started",
		"migration_source_phase_temp_synced:cleanup_started",
		"migration_source_phase_renamed:cleanup_started",
		"migration_source_phase_parent_synced:cleanup_started",
		"migration_source_cleanup_prepared",
		"migration_source_cleanup_renamed",
		"migration_source_cleanup_rename_synced",
		"migration_source_cleanup_removed",
		"migration_source_cleanup_remove_synced",
		"journal_write_prepared", "runtime_journal_temp_written", "runtime_journal_temp_synced", "runtime_journal_renamed", "runtime_journal_parent_synced", "journal_written",
		"legacy_rename_prepared", "legacy_quarantined", "legacy_quarantine_synced",
		"config_rename_prepared", "config_published", "config_publish_synced",
		"state_rename_prepared", "state_published", "state_publish_synced",
		"runtime_readback_verified", "runtime_cleanup_prepared", "runtime_legacy_cleanup_removed", "runtime_journal_removed", "runtime_cleanup_synced",
		"authority_stage_materialized", "authority_stage_synced", "authority_stage_readback",
		"outer_phase_temp_synced:prepared", "outer_phase_renamed:prepared", "outer_phase_parent_synced:prepared",
		"outer_journal_written", "components_committed", "outer_sources_phase_written",
		"outer_phase_temp_synced:sources_committed", "outer_phase_renamed:sources_committed", "outer_phase_parent_synced:sources_committed",
		"authority_legacy_quarantined", "authority_new_published", "authority_parent_synced",
		"outer_phase_temp_synced:authority_published", "outer_phase_renamed:authority_published", "outer_phase_parent_synced:authority_published",
		"outer_authority_phase_written", "authority_readback", "components_readback",
		"outer_phase_temp_synced:verified", "outer_phase_renamed:verified", "outer_phase_parent_synced:verified",
		"outer_verified_phase_written", "authority_backup_removed", "authority_backup_remove_synced",
		"outer_phase_temp_synced:accepted", "outer_phase_renamed:accepted", "outer_phase_parent_synced:accepted",
		"outer_accepted_phase_written", "components_completed",
		"accepted_receipt_temp_written", "accepted_receipt_temp_synced", "accepted_receipt_renamed", "accepted_receipt_parent_synced",
		"outer_cleanup_prepared", "outer_transaction_removed", "outer_cleanup_synced", "outer_journal_removed",
	}
	for _, boundary := range boundaries {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			command := exec.Command(os.Args[0], "-test.run=^TestRealResourcesMigrationCrashProcess$") // #nosec G204 -- exact current test binary and fixed selector.
			command.Env = append(os.Environ(), "TOBARI_REAL_MIGRATION_ROOT="+root, "TOBARI_REAL_MIGRATION_BOUNDARY="+boundary)
			err := command.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 91 {
				t.Fatalf("real composed crash at %s = %v", boundary, err)
			}
			planRef, err := os.ReadFile(filepath.Join(root, "plan-ref"))
			if err != nil {
				t.Fatal(err)
			}
			adapter, runtime, _ := realResourcesMigrationAdapter(t, root, "")
			result, err := adapter.ApplyInstallationMigration(context.Background(), string(planRef))
			if err != nil || !result.Changed || result.PlanRef != string(planRef) {
				t.Fatalf("real composed retry after %s = %+v/%v", boundary, result, err)
			}
			configRoot, _ := runtime.ResourceSourceRoot()
			authorityRoot, _ := runtime.FinalWorkspaceAuthorityRoot()
			stateRoot := filepath.Dir(authorityRoot)
			for _, path := range []string{
				filepath.Join(configRoot, ".installation-migration-source-stage"),
				filepath.Join(configRoot, ".installation-migration-source-cleanup"),
				filepath.Join(configRoot, ".installation-runtime-config-stage"),
				filepath.Join(configRoot, ".installation-runtime-legacy"),
				filepath.Join(stateRoot, ".installation-runtime-state-stage"),
				filepath.Join(stateRoot, "installation-runtime-migration.json"),
				filepath.Join(stateRoot, "installation-runtime-migration.json.next"),
				filepath.Join(filepath.Dir(authorityRoot), "authority.migration-stage"),
				filepath.Join(filepath.Dir(authorityRoot), "authority.migration-old"),
				authorityRoot + ".migration-transaction.json",
				authorityRoot + ".migration-transaction.json.next",
				filepath.Join(authorityRoot, "journal", "installation-migration-accepted.json.next"),
			} {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("real composed retry after %s retained %s: %v", boundary, path, err)
				}
			}
			if _, err := os.Lstat(filepath.Join(configRoot, "runtimes", composedRuntimeID, "runtime.yaml")); err != nil {
				t.Fatalf("real composed retry after %s omitted Runtime config: %v", boundary, err)
			}
			if _, err := os.Lstat(filepath.Join(authorityRoot, "journal", "installation-migration-accepted.json")); err != nil {
				t.Fatalf("real composed retry after %s omitted durable accepted receipt: %v", boundary, err)
			}
			if err := filepath.WalkDir(stateRoot, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if strings.HasPrefix(entry.Name(), ".tobari-state-") {
					return errors.New("random Runtime state temporary remained: " + path)
				}
				return nil
			}); err != nil {
				t.Fatalf("real composed retry after %s retained random Runtime temporary: %v", boundary, err)
			}
		})
	}
}

func TestRealResourcesMigrationCrashProcess(t *testing.T) {
	root := os.Getenv("TOBARI_REAL_MIGRATION_ROOT")
	boundary := os.Getenv("TOBARI_REAL_MIGRATION_BOUNDARY")
	if root == "" || boundary == "" {
		return
	}
	adapter, _, _ := realResourcesMigrationAdapter(t, root, boundary)
	plan, err := adapter.PlanInstallationMigration(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "plan-ref"), []byte(plan.PlanRef), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ApplyInstallationMigration(context.Background(), plan.PlanRef); err != nil {
		t.Fatal(err)
	}
	os.Exit(92)
}
