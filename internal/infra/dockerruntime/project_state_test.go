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

func TestSameCanonicalRootCanOwnIndependentTobariInDifferentContexts(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(context.Background(), "restricted", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite); err != nil {
		t.Fatal(err)
	}
	defaultProject, created, err := runtime.ResolveOrCreateProjectInContext(context.Background(), root, "default")
	if err != nil || !created {
		t.Fatalf("default project = %+v, created=%t, error=%v", defaultProject, created, err)
	}
	restrictedProject, created, err := runtime.ResolveOrCreateProjectInContext(context.Background(), root, "restricted")
	if err != nil || !created {
		t.Fatalf("restricted project = %+v, created=%t, error=%v", restrictedProject, created, err)
	}
	if defaultProject.ID == restrictedProject.ID || defaultProject.ContextID == restrictedProject.ContextID ||
		runtime.projectHomePath(defaultProject.ID) == runtime.projectHomePath(restrictedProject.ID) || defaultProject.Root != restrictedProject.Root {
		t.Fatalf("Context-bound projects are not independently identified: default=%+v restricted=%+v", defaultProject, restrictedProject)
	}
	projects, err := runtime.ListProjects(context.Background())
	if err != nil || len(projects) != 2 {
		t.Fatalf("ListProjects() = %+v, error=%v", projects, err)
	}
}

func TestBoundContextManifestSelectsSameRootWorkspaceWithoutNameRediscovery(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	defaultManifest, err := runtime.ResolveContext(context.Background(), tobari.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(context.Background(), "toolbox", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadOnly); err != nil {
		t.Fatal(err)
	}
	toolboxManifest, err := runtime.ResolveContext(context.Background(), "toolbox")
	if err != nil {
		t.Fatal(err)
	}
	if defaultManifest.SourceAccess != tobari.ContextSourceAccessReadWrite ||
		toolboxManifest.SourceAccess != tobari.ContextSourceAccessReadOnly {
		t.Fatalf("same-root Context access = %q/%q", defaultManifest.SourceAccess, toolboxManifest.SourceAccess)
	}
	defaultProject, err := runtime.CreateBoundProject(context.Background(), root, defaultManifest)
	if err != nil {
		t.Fatal(err)
	}
	toolboxProject, err := runtime.CreateBoundProject(context.Background(), root, toolboxManifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		manifest tobari.ContextManifest
		wantID   string
	}{
		{manifest: defaultManifest, wantID: defaultProject.ID},
		{manifest: toolboxManifest, wantID: toolboxProject.ID},
	} {
		got, found, err := runtime.ResolveBoundProject(context.Background(), root, test.manifest)
		if err != nil || !found || got.ID != test.wantID || got.ContextID != test.manifest.ID {
			t.Fatalf("ResolveBoundProject(%s) = (%+v, %t, %v), want %s", test.manifest.Name, got, found, err, test.wantID)
		}
	}
}

func TestCreateProjectAllowsExplicitNestedWorkspaceCreation(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	base := t.TempDir()
	parent := filepath.Join(base, "project")
	child := filepath.Join(parent, "internal")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	canonicalChild, err := runtime.ResolveProjectRoot(context.Background(), child)
	if err != nil {
		t.Fatalf("ResolveProjectRoot(child) error = %v", err)
	}
	parentInstance, _, err := runtime.ResolveOrCreateProject(context.Background(), parent)
	if err != nil {
		t.Fatalf("ResolveOrCreateProject(parent) error = %v", err)
	}
	childInstance, err := runtime.CreateProject(context.Background(), child)
	if err != nil {
		t.Fatalf("CreateProject(child) error = %v", err)
	}
	if childInstance.Root != canonicalChild || childInstance.ID == parentInstance.ID {
		t.Fatalf("CreateProject(child) = %+v, parent = %+v", childInstance, parentInstance)
	}
	resolved, found, err := runtime.ResolveProject(context.Background(), child)
	if err != nil || !found || resolved.ID != childInstance.ID || resolved.Root != canonicalChild {
		t.Fatalf("ResolveProject(child) = (%+v, %t, %v)", resolved, found, err)
	}
	if _, err := runtime.CreateProject(context.Background(), child); !errors.Is(err, tobari.ErrProjectExists) {
		t.Fatalf("CreateProject(existing child) error = %v, want ErrProjectExists", err)
	}
}

func TestCreateProjectSerializesConcurrentSameRootCreation(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := runtime.ResolveProjectRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveProjectRoot() error = %v", err)
	}
	const callers = 12
	ids := make(chan string, callers)
	errs := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			instance, err := runtime.CreateProject(context.Background(), root)
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

	var createdID string
	for id := range ids {
		if createdID != "" {
			t.Fatalf("concurrent explicit creation produced Workspace IDs %q and %q", createdID, id)
		}
		createdID = id
	}
	var duplicateErrors int
	for err := range errs {
		if !errors.Is(err, tobari.ErrProjectExists) {
			t.Fatalf("CreateProject() error = %v, want ErrProjectExists for the losing caller", err)
		}
		duplicateErrors++
	}
	if createdID == "" || duplicateErrors != callers-1 {
		t.Fatalf("concurrent explicit creation = created %q, duplicate errors %d; want one creation and %d duplicate errors", createdID, duplicateErrors, callers-1)
	}
	instances, err := runtime.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(instances) != 1 || instances[0].ID != createdID || instances[0].Root != canonicalRoot {
		t.Fatalf("ListProjects() = %+v, want one Workspace %q at %q", instances, createdID, canonicalRoot)
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
	canonicalRoot, err := runtime.ResolveProjectRoot(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveProjectRoot() error = %v", err)
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

func TestResolveOrCreateProjectIgnoresProjectLocalDevContainerImage(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(root, ".devcontainer"), 0o700); err != nil {
		t.Fatal(err)
	}
	definition := []byte(`{
	"image": "workbench:local",
}`)
	if err := os.WriteFile(filepath.Join(root, ".devcontainer", "devcontainer.json"), definition, 0o600); err != nil {
		t.Fatal(err)
	}
	instance, created, err := runtime.ResolveOrCreateProject(context.Background(), root)
	if err != nil {
		t.Fatalf("ResolveOrCreateProject() error = %v", err)
	}
	if !created || instance.Image != tobari.OfficialRuntimeBase {
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
			manifest, _, err := runtime.activeContext()
			if err != nil {
				t.Fatal(err)
			}
			path, err := runtime.rootIndexPath(root, manifest.ID)
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
				ProjectID: instance.ID, Root: instance.Root, ContextID: instance.ContextID, Phase: projectPhaseRuntime,
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

func TestListProjectsOnlyCleansPreExistingCompletedJournal(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, created, err := runtime.ResolveOrCreateProject(context.Background(), root)
	if err != nil || !created {
		t.Fatalf("create fixture project = %+v, created=%t, err=%v", instance, created, err)
	}
	journal := projectJournal{
		SchemaVersion: projectJournalSchema, Operation: projectOpCreate,
		ProjectID: instance.ID, Root: instance.Root, ContextID: instance.ContextID, Phase: projectPhaseIndex,
	}
	if err := runtime.writeProjectJournal(journal); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(runtime.stateDirectory, "project.lock")
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("remove fixture project lock: %v", err)
	}
	if _, err := os.Lstat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fixture project lock still exists before observation: %v", err)
	}
	statePath, err := runtime.projectStatePath(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	indexPath, err := runtime.rootIndexPath(instance.Root, instance.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := runtime.ListProjects(context.Background())
	if err != nil || len(projects) != 1 || projects[0].ID != instance.ID {
		t.Fatalf("ListProjects() = %+v, %v", projects, err)
	}
	if _, err := os.Lstat(runtime.projectJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("completed pre-existing journal remains: %v", err)
	}
	for _, path := range []string{statePath, indexPath, lockPath} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("journal cleanup removed durable state %s: %v", path, err)
		}
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
			wantIncomplete := name == "root index only"
			if err != nil || !found || resolved.ID != instance.ID || resolved.Root != instance.Root || resolved.Incomplete != wantIncomplete {
				t.Fatalf("ResolveProject() = (%+v, %t, %v), want recoverable %s record", resolved, found, err, name)
			}
		})
	}
}

func TestResolveOrCreateProjectReturnsCleanupOnlyRecordForMissingInstanceState(t *testing.T) {
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
	statePath, err := runtime.projectStatePath(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	recovered, created, err := runtime.ResolveOrCreateProject(context.Background(), root)
	if err != nil || created || !recovered.Incomplete || recovered.ID != instance.ID {
		t.Fatalf("ResolveOrCreateProject() = (%+v, %t, %v), want cleanup-only existing record", recovered, created, err)
	}
}

func TestListProjectsIncludesRootIndexOnlyAsCleanupOnlyLogicalState(t *testing.T) {
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
	statePath, err := runtime.projectStatePath(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	projects, err := runtime.ListProjects(context.Background())
	if err != nil || len(projects) != 1 || !projects[0].Incomplete || projects[0].ID != instance.ID {
		t.Fatalf("ListProjects() = (%+v, %v), want one cleanup-only project", projects, err)
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
