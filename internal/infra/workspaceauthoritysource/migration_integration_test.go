package workspaceauthoritysource

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

type realCombinedMigrationStage struct {
	sources  workspaceauthoritystore.InstallationMigrationSourceStage
	runtimes workspaceauthoritystore.InstallationMigrationSourceStage
}

func (s *realCombinedMigrationStage) ExpectedIdentity() (tobari.SemanticDigest, error) {
	source, err := s.sources.ExpectedIdentity()
	if err != nil {
		return "", err
	}
	runtime, err := s.runtimes.ExpectedIdentity()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte("source\x00" + string(source) + "\x00runtime\x00" + string(runtime)))
	return tobari.SemanticDigest(fmt.Sprintf("sha256:%x", digest)), nil
}

func (s *realCombinedMigrationStage) Commit(ctx context.Context) error {
	if err := s.sources.Commit(ctx); err != nil {
		return err
	}
	if err := s.runtimes.Commit(ctx); err != nil {
		return fmt.Errorf("commit Runtime migration: %w", err)
	}
	return nil
}

func (s *realCombinedMigrationStage) Verify(ctx context.Context) error {
	if err := s.sources.Verify(ctx); err != nil {
		return fmt.Errorf("verify real source stage: %w", err)
	}
	return s.runtimes.Verify(ctx)
}

func (s *realCombinedMigrationStage) Rollback(ctx context.Context) error {
	if err := s.runtimes.Rollback(ctx); err != nil {
		return err
	}
	if err := s.sources.Rollback(ctx); err != nil {
		return fmt.Errorf("rollback real source stage: %w", err)
	}
	return nil
}

func (s *realCombinedMigrationStage) Complete(ctx context.Context) error {
	if err := s.sources.Complete(ctx); err != nil {
		return fmt.Errorf("complete real source stage: %w", err)
	}
	return s.runtimes.Complete(ctx)
}

func (s *realCombinedMigrationStage) Abort(ctx context.Context) error {
	if err := s.runtimes.Abort(ctx); err != nil {
		return err
	}
	if err := s.sources.Abort(ctx); err != nil {
		return fmt.Errorf("abort real source stage: %w", err)
	}
	return nil
}

func realMigrationComposition(t *testing.T, root string, sourceBoundary func(string) error) (*dockerruntime.Runtime, *workspaceauthoritystore.Mutator, *Store, tobari.WorkspaceAuthorityCollection) {
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
	authorityRoot, err := runtime.FinalWorkspaceAuthorityRoot()
	if err != nil {
		t.Fatal(err)
	}
	collection := sourceMigrationCollectionFixture(t)
	if _, err := os.Lstat(authorityRoot); os.IsNotExist(err) {
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
	}
	store, err := workspaceauthoritystore.NewFinalOnly(authorityRoot, runtime)
	if err != nil {
		t.Fatal(err)
	}
	mutator, err := workspaceauthoritystore.NewMutator(context.Background(), store, runtime, runtime, runtime, runtime, runtime)
	if err != nil {
		t.Fatal(err)
	}
	configRoot, err := runtime.ResourceSourceRoot()
	if err != nil {
		t.Fatal(err)
	}
	sources, err := New(configRoot)
	if err != nil {
		t.Fatal(err)
	}
	if sourceBoundary != nil {
		sources.phase = sourceBoundary
	}
	return runtime, mutator, sources, collection
}

func realMigrationPreparer(runtime *dockerruntime.Runtime, sources *Store) workspaceauthoritystore.InstallationMigrationSourcePreparer {
	return func(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, recovery bool) (workspaceauthoritystore.InstallationMigrationSourceStage, error) {
		sourceStage, err := sources.PrepareInstallationMigrationSources(ctx, collection, recovery)
		if err != nil {
			return nil, err
		}
		runtimeStage, err := runtime.PrepareInstallationRuntimeMigration(ctx, collection, recovery)
		if err != nil {
			_ = sourceStage.Abort(ctx)
			return nil, err
		}
		return &realCombinedMigrationStage{sources: sourceStage, runtimes: runtimeStage}, nil
	}
}

func TestComposedInstallationMigrationRecoversEverySourceCrashAndRetriesSamePlan(t *testing.T) {
	for _, boundary := range []string{
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
	} {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			command := exec.Command(os.Args[0], "-test.run=^TestComposedInstallationMigrationCrashProcess$") // #nosec G204 -- exact current test binary and fixed test selector.
			command.Env = append(os.Environ(), "TOBARI_MIGRATION_CRASH_ROOT="+root, "TOBARI_MIGRATION_CRASH_BOUNDARY="+boundary)
			err := command.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 91 {
				t.Fatalf("composed crash process at %s = %v", boundary, err)
			}
			restartedRuntime, restartedMutator, restartedSources, _ := realMigrationComposition(t, root, nil)
			plan, err := restartedMutator.PlanInstallationMigration(context.Background(), restartedRuntime.ObserveInstallationRuntimeMigration)
			if err != nil {
				t.Fatalf("same-plan recomputation after %s: %v", boundary, err)
			}
			result, err := restartedMutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, restartedRuntime.ObserveInstallationRuntimeMigration, realMigrationPreparer(restartedRuntime, restartedSources))
			if err != nil || !result.Changed || result.PlanRef != plan.PlanRef {
				t.Fatalf("same-plan composed recovery after %s = %+v/%v", boundary, result, err)
			}
			authorityRoot, _ := restartedRuntime.FinalWorkspaceAuthorityRoot()
			if _, err := os.Lstat(filepath.Join(authorityRoot, "authority.json")); !os.IsNotExist(err) {
				t.Fatalf("same-plan composed recovery after %s retained predecessor: %v", boundary, err)
			}
			for _, concept := range []string{"templates", "contexts"} {
				if info, err := os.Lstat(filepath.Join(root, "config-home", "tobari", concept)); err != nil || !info.IsDir() {
					t.Fatalf("same-plan composed recovery after %s missing %s: %v", boundary, concept, err)
				}
			}
		})
	}
}

func TestComposedInstallationMigrationSettlesAcceptedSourceCleanupCrash(t *testing.T) {
	for _, boundary := range []string{
		"migration_source_phase_temp_written:cleanup_started",
		"migration_source_phase_temp_synced:cleanup_started",
		"migration_source_phase_renamed:cleanup_started",
		"migration_source_phase_parent_synced:cleanup_started",
		"migration_source_cleanup_prepared",
		"migration_source_cleanup_renamed",
		"migration_source_cleanup_rename_synced",
		"migration_source_cleanup_removed",
		"migration_source_cleanup_remove_synced",
	} {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			command := exec.Command(os.Args[0], "-test.run=^TestComposedInstallationMigrationCrashProcess$") // #nosec G204 -- exact current test binary and fixed test selector.
			command.Env = append(os.Environ(), "TOBARI_MIGRATION_CRASH_ROOT="+root, "TOBARI_MIGRATION_CRASH_BOUNDARY="+boundary)
			err := command.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 91 {
				t.Fatalf("accepted composed crash process at %s = %v", boundary, err)
			}
			planData, err := os.ReadFile(filepath.Join(root, "migration-plan-ref"))
			if err != nil {
				t.Fatal(err)
			}
			planRef := string(planData)
			restartedRuntime, restartedMutator, restartedSources, _ := realMigrationComposition(t, root, nil)
			result, err := restartedMutator.ApplyInstallationMigration(context.Background(), planRef, restartedRuntime.ObserveInstallationRuntimeMigration, realMigrationPreparer(restartedRuntime, restartedSources))
			if err != nil || !result.Changed || result.PlanRef != planRef {
				t.Fatalf("accepted composed settlement after %s = %+v/%v", boundary, result, err)
			}
			if _, err := os.Lstat(filepath.Join(root, "config-home", "tobari", migrationSourceStageName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("accepted composed settlement after %s retained source stage: %v", boundary, err)
			}
			if _, err := os.Lstat(filepath.Join(root, "config-home", "tobari", migrationSourceCleanupName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("accepted composed settlement after %s retained source cleanup tombstone: %v", boundary, err)
			}
		})
	}
}

func TestComposedInstallationMigrationCrashProcess(t *testing.T) {
	root := os.Getenv("TOBARI_MIGRATION_CRASH_ROOT")
	boundary := os.Getenv("TOBARI_MIGRATION_CRASH_BOUNDARY")
	if root == "" || boundary == "" {
		return
	}
	runtime, mutator, sources, _ := realMigrationComposition(t, root, func(observed string) error {
		if observed == boundary {
			os.Exit(91)
		}
		return nil
	})
	plan, err := mutator.PlanInstallationMigration(context.Background(), runtime.ObserveInstallationRuntimeMigration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "migration-plan-ref"), []byte(plan.PlanRef), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, runtime.ObserveInstallationRuntimeMigration, realMigrationPreparer(runtime, sources)); err != nil {
		t.Fatal(err)
	}
	os.Exit(92)
}
