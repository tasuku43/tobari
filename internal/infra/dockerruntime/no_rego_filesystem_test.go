package dockerruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// assertNoHostEvaluatorSources is deliberately a recursive filesystem
// contract. A production source guard cannot prove that a complete lifecycle
// kept evaluator bytes out of the user's XDG roots; this check covers config,
// state, data, Context/Template authority, and the populated user Workspace
// fixture together.
func assertNoHostEvaluatorSources(t *testing.T, roots ...string) {
	t.Helper()
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path != root && strings.HasSuffix(strings.ToLower(entry.Name()), ".rego") {
				return errors.New("host XDG evaluator source found at " + path)
			}
			return nil
		})
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPolicyLifecycleKeepsEvaluatorSourceOutOfAllUserXDGRoots(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config-home")
	stateHome := filepath.Join(root, "state-home")
	dataHome := filepath.Join(root, "data-home")
	workspaceRoot := filepath.Join(root, "workspace")
	runner := &recordingRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(configHome, "tobari"), filepath.Join(stateHome, "tobari"), filepath.Join(dataHome, "tobari"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "workspace-fixture.txt"), []byte("user Workspace fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	initializeTestWorkspaceManifest(t, runtime)
	if _, created, err := runtime.ResolveOrCreateProject(context.Background(), workspaceRoot); err != nil || !created {
		t.Fatalf("Workspace fixture project = created:%t error:%v", created, err)
	}
	roots := []string{configHome, stateHome, dataHome, workspaceRoot}

	state, err := runtime.prepareState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertNoHostEvaluatorSources(t, roots...)

	manifest, paths, err := runtime.resolveContext("default")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := readPolicyData(paths.PolicyDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.testContextPolicyCandidate(context.Background(), manifest, paths.PolicyDirectory, policy); err != nil {
		t.Fatalf("policy mutation preflight: %v", err)
	}
	assertNoHostEvaluatorSources(t, roots...)

	t.Run("mutation preflight failure", func(t *testing.T) {
		previousRunner := runtime.runner
		failureRunner := &recordingRunner{outputData: []byte("FAIL\n"), outputErr: errors.New("injected mutation preflight failure")}
		runtime.runner = failureRunner
		_, err := runtime.testContextPolicyCandidate(context.Background(), manifest, paths.PolicyDirectory, policy)
		runtime.runner = previousRunner
		if err == nil {
			t.Fatal("failed mutation preflight unexpectedly succeeded")
		}
		if len(failureRunner.outputs) == 0 {
			t.Fatal("failed mutation preflight did not reach its Docker test boundary")
		}
		assertNoHostEvaluatorSources(t, roots...)
	})

	// ApplyPolicy is the reducing-transition path: its deny-all fence and the
	// complete candidate bundle are both assembled in memory and staged only in
	// the owned Docker volume.
	if err := runtime.ApplyPolicy(context.Background(), state); err != nil {
		t.Fatalf("reducing policy transition: %v", err)
	}
	assertNoHostEvaluatorSources(t, roots...)

	t.Run("reducing transition Docker failure", func(t *testing.T) {
		previousRunner := runtime.runner
		failureRunner := &recordingRunner{outputData: []byte("FAIL\n"), outputErr: errors.New("injected reducing transition Docker failure")}
		runtime.runner = failureRunner
		err := runtime.ApplyPolicy(context.Background(), state)
		runtime.runner = previousRunner
		if err == nil {
			t.Fatal("failed reducing transition unexpectedly succeeded")
		}
		if len(failureRunner.outputs) == 0 {
			t.Fatal("failed reducing transition did not reach Docker")
		}
		assertNoHostEvaluatorSources(t, roots...)
	})

	// The dormant/final projection has its own lifecycle and failure recovery.
	finalRuntime, _, collection := finalPolicyActivationFixture(t)
	finalRoots := []string{finalRuntime.configDirectory, finalRuntime.stateDirectory, finalRuntime.dataDirectory}
	if _, _, _, err := finalRuntime.PrepareFinalClusterPolicyReconciliation(context.Background(), collection); err != nil {
		t.Fatalf("dormant final projection: %v", err)
	}
	assertNoHostEvaluatorSources(t, finalRoots...)
	finalRuntime.finalPolicyAfterApply = func() error { return errors.New("injected final projection failure") }
	if _, err := finalRuntime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err == nil {
		t.Fatal("failed final projection unexpectedly completed")
	}
	finalRuntime.finalPolicyAfterApply = nil
	if _, err := finalRuntime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err != nil {
		t.Fatalf("recover failed final projection: %v", err)
	}
	assertNoHostEvaluatorSources(t, finalRoots...)

	// Both root and nested legacy executable sources are detected before the
	// Context can be copied into an aggregate. The test removes only its own
	// hostile fixture before the final recursive contract check.
	for name, path := range map[string]string{
		"root":   filepath.Join(paths.PolicyDirectory, "legacy.rego"),
		"nested": filepath.Join(paths.PolicyDirectory, "nested", "legacy.rego"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package legacy\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.buildAggregateProjection(context.Background()); err == nil || !errors.Is(err, tobari.ErrLegacyExecutablePolicy) {
			t.Fatalf("%s legacy source error = %v", name, err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if name == "nested" {
			if err := os.Remove(filepath.Dir(path)); err != nil {
				t.Fatal(err)
			}
		}
	}
	assertNoHostEvaluatorSources(t, roots...)
}
