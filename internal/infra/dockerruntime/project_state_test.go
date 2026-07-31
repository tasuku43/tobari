package dockerruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func newProjectStateRuntime(t *testing.T) *Runtime {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &recordingRunner{},
	)
	if err != nil {
		t.Fatalf("newRuntimeWithData() error = %v", err)
	}
	return runtime
}

func TestResolveOrCreateProjectCreatesDurableCWDState(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	child := filepath.Join(root, "internal", "gateway")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}

	instance, created, err := runtime.ResolveOrCreateProject(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveOrCreateProject() error = %v", err)
	}
	if !created {
		t.Fatal("ResolveOrCreateProject() did not report first creation")
	}
	if err := instance.Validate(); err != nil {
		t.Fatalf("instance.Validate() error = %v", err)
	}
	if _, err := os.Stat(runtime.projectHomePath(instance.ID)); err != nil {
		t.Fatalf("project home was not created: %v", err)
	}

	again, created, err := runtime.ResolveOrCreateProject(context.Background(), child)
	if err != nil {
		t.Fatalf("ResolveOrCreateProject(child) error = %v", err)
	}
	if created || again.ID != instance.ID || again.Root != instance.Root {
		t.Fatalf("ResolveOrCreateProject(child) = (%+v, created=%t), want existing parent project", again, created)
	}
}

func TestResolveProjectFollowsCanonicalSymlink(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	base := t.TempDir()
	root := filepath.Join(base, "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtime.ResolveOrCreateProject(context.Background(), root); err != nil {
		t.Fatalf("ResolveOrCreateProject() error = %v", err)
	}
	canonicalRoot, err := runtime.ResolveRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	link := filepath.Join(base, "project-link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	instance, found, err := runtime.ResolveProject(context.Background(), link)
	if err != nil {
		t.Fatalf("ResolveProject() error = %v", err)
	}
	if !found || instance.Root != canonicalRoot {
		t.Fatalf("ResolveProject() = (%+v, %t), want canonical root %q", instance, found, canonicalRoot)
	}
}

func TestResolveOrCreateProjectUsesProjectLocalDevContainerImage(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(root, ".devcontainer"), 0o700); err != nil {
		t.Fatal(err)
	}
	definition := []byte(`{
  // Only image metadata is interpreted by Tobari.
  "image": "workbench:local",
}`)
	if err := os.WriteFile(filepath.Join(root, ".devcontainer", "devcontainer.json"), definition, 0o600); err != nil {
		t.Fatal(err)
	}
	instance, created, err := runtime.ResolveOrCreateProject(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveOrCreateProject() error = %v", err)
	}
	if !created || instance.Image != "workbench:local" {
		t.Fatalf("instance = %+v, created=%t", instance, created)
	}
}

func TestProjectStateRejectsUnknownFields(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveOrCreateProject() error = %v", err)
	}
	path, err := runtime.projectStatePath(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"id":"`+instance.ID+`","root":"`+root+`","profile":"default","image":"builtin","runtime":{"container_id":"","network_id":""},"unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtime.ResolveProject(context.Background(), root); err == nil {
		t.Fatal("ResolveProject() accepted an unknown state field")
	}
}

func TestResolveOrCreateProjectSerializesConcurrentCreation(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	const callers = 12
	ids := make(chan string, callers)
	errs := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			instance, _, err := runtime.ResolveOrCreateProject(context.Background(), root)
			if err != nil {
				errs <- err
				return
			}
			ids <- instance.ID
		}()
	}
	group.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("ResolveOrCreateProject() error = %v", err)
	}
	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatalf("concurrent creation produced IDs %q and %q", first, id)
		}
	}
	instances, err := runtime.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(instances) != 1 || instances[0].ID != first {
		t.Fatalf("ListProjects() = %+v, want one project %q", instances, first)
	}
}

func TestProjectStateDoesNotTreatTemporaryAtomicFileAsState(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.rootsDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.rootsDirectory(), ".tobari-state-interrupted"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	instance, created, err := runtime.ResolveOrCreateProject(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveOrCreateProject() error = %v", err)
	}
	if !created || instance.ID == "" {
		t.Fatalf("ResolveOrCreateProject() = (%+v, created=%t)", instance, created)
	}
}

func TestResolveOrCreateProjectCleansUpAfterLogicalCreationBoundaryFailures(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*testing.T, *Runtime, string){
		"instance state": func(t *testing.T, runtime *Runtime, _ string) {
			runtime.projectStateWriter = func(tobari.ProjectInstance) error {
				return errors.New("injected instance state failure")
			}
		},
		"root index": func(t *testing.T, runtime *Runtime, root string) {
			path, err := runtime.rootIndexPath(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"home": func(t *testing.T, runtime *Runtime, _ string) {
			if err := os.MkdirAll(filepath.Dir(runtime.instancesDirectory()), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(runtime.instancesDirectory(), []byte("not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, inject := range tests {
		name, inject := name, inject
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runtime := newProjectStateRuntime(t)
			root := filepath.Join(t.TempDir(), "project")
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			inject(t, runtime, root)
			if _, _, err := runtime.ResolveOrCreateProject(context.Background(), root); err == nil {
				t.Fatal("ResolveOrCreateProject() unexpectedly succeeded")
			}
			if entries, err := os.ReadDir(runtime.instancesDirectory()); err == nil && len(entries) != 0 {
				t.Fatalf("unindexed instance state remains after %s failure: %v", name, entries)
			}
		})
	}
}

func TestProjectJournalReconcilesInterruptedCreateAndDelete(t *testing.T) {
	t.Parallel()
	for name, operation := range map[string]string{"create": projectOpCreate, "delete": projectOpDelete} {
		name, operation := name, operation
		t.Run(name, func(t *testing.T) {
			runtime := newProjectStateRuntime(t)
			root := filepath.Join(t.TempDir(), "project")
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			instance, _, err := runtime.ResolveOrCreateProject(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.removeProjectRootIndex(instance.Root); err != nil {
				t.Fatal(err)
			}
			if operation == projectOpDelete {
				if err := runtime.removeProjectInstanceDirectory(instance.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err := runtime.writeProjectJournal(projectJournal{
				SchemaVersion: projectJournalSchema, Operation: operation,
				ProjectID: instance.ID, Root: instance.Root, Phase: projectPhaseRuntime,
			}); err != nil {
				t.Fatal(err)
			}
			recreated, created, err := runtime.ResolveOrCreateProject(context.Background(), root)
			if err != nil || !created || recreated.ID == instance.ID {
				t.Fatalf("ResolveOrCreateProject() = (%+v, %t, %v), want fresh project after %s recovery", recreated, created, err, operation)
			}
			if _, err := os.Stat(runtime.projectJournalPath()); !os.IsNotExist(err) {
				t.Fatalf("journal remains after %s recovery: %v", operation, err)
			}
		})
	}
}

func TestResolveProjectFindsOneSidedLogicalRecordsForRecovery(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*testing.T, *Runtime, tobari.ProjectInstance){
		"root index only": func(t *testing.T, runtime *Runtime, instance tobari.ProjectInstance) {
			path, err := runtime.projectStatePath(instance.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		},
		"instance only": func(t *testing.T, runtime *Runtime, instance tobari.ProjectInstance) {
			if err := runtime.removeProjectRootIndex(instance.Root); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, breakRecord := range tests {
		name, breakRecord := name, breakRecord
		t.Run(name, func(t *testing.T) {
			runtime := newProjectStateRuntime(t)
			root := filepath.Join(t.TempDir(), "project")
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			instance, _, err := runtime.ResolveOrCreateProject(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			breakRecord(t, runtime, instance)
			resolved, found, err := runtime.ResolveProject(context.Background(), root)
			if err != nil || !found || resolved.ID != instance.ID || resolved.Root != instance.Root {
				t.Fatalf("ResolveProject() = (%+v, %t, %v), want recoverable %s record", resolved, found, err, name)
			}
		})
	}
}

func TestListProjectsDiagnosesUnindexedInstance(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.removeProjectRootIndex(instance.Root); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListProjects(context.Background()); err == nil || !strings.Contains(err.Error(), "has no root index") {
		t.Fatalf("ListProjects() error = %v, want unindexed-instance diagnostic", err)
	}
}

func TestUpdateProjectRuntimeDoesNotChangeLogicalIdentity(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	instance.Runtime = tobari.ProjectRuntime{ContainerID: "container", NetworkID: "network"}
	if err := runtime.UpdateProjectRuntime(context.Background(), instance); err != nil {
		t.Fatalf("UpdateProjectRuntime() error = %v", err)
	}
	stored, found, err := runtime.ResolveProject(context.Background(), root)
	if err != nil || !found || stored.Runtime != instance.Runtime {
		t.Fatalf("ResolveProject() = (%+v, %t, %v)", stored, found, err)
	}
}
