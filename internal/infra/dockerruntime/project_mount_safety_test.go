package dockerruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type projectMountSafetyRunner struct {
	calls       [][]string
	exists      bool
	starts      int
	removes     int
	afterCreate func()
	guardID     string
	guardJSON   string
}

type stoppedProjectMountSafetyRunner struct {
	projectID     string
	spec          string
	root          string
	workspaceRoot string
	network       string
	dns           string
	starts        int
	afterStatus   func()
	statusSeen    bool
}

func (*projectMountSafetyRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *projectMountSafetyRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	if len(args) == 0 {
		return nil, errors.New("missing Docker argv")
	}
	switch args[0] {
	case "ps":
		if r.guardID == "" {
			return nil, nil
		}
		return []byte(r.guardID + "\n"), nil
	case "inspect":
		if r.guardID != "" && args[len(args)-1] == r.guardID {
			return []byte(r.guardJSON), nil
		}
		if !r.exists {
			return []byte("no such container"), errors.New("not found")
		}
		return []byte(strings.Repeat("c", 64)), nil
	case "create":
		r.exists = true
		if r.afterCreate != nil {
			r.afterCreate()
		}
		return []byte("created"), nil
	case "start":
		r.starts++
		return nil, nil
	case "rm":
		r.removes++
		r.exists = false
		return nil, nil
	default:
		return nil, errors.New("unexpected Docker argv")
	}
}

func (*stoppedProjectMountSafetyRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *stoppedProjectMountSafetyRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("missing Docker argv")
	}
	if args[0] == "ps" {
		return nil, nil
	}
	if args[0] == "start" {
		r.starts++
		return nil, nil
	}
	if args[0] != "inspect" || len(args) < 3 {
		return nil, errors.New("unexpected Docker argv")
	}
	format := args[2]
	switch {
	case format == "{{.Id}}":
		return []byte(strings.Repeat("c", 64)), nil
	case strings.Contains(format, ownerLabel):
		return []byte(ownerValue), nil
	case strings.Contains(format, projectIDLabel):
		return []byte(r.projectID), nil
	case strings.Contains(format, projectRoleLabel):
		return []byte(projectWorkRole), nil
	case strings.Contains(format, projectSpecLabel):
		return []byte(r.spec), nil
	case strings.Contains(format, ".HostConfig.Dns"):
		return []byte(`["` + r.dns + `"]`), nil
	case strings.Contains(format, ".State.Health"):
		if !r.statusSeen {
			r.statusSeen = true
			if r.afterStatus != nil {
				r.afterStatus()
			}
		}
		return []byte(`{"state":"exited","health":"none"}`), nil
	case strings.Contains(format, ".Mounts"):
		return []byte(`[{"Type":"bind","Source":"` + r.root + `","Destination":"` + r.workspaceRoot + `","RW":true}]`), nil
	case strings.Contains(format, ".NetworkSettings.Networks"):
		return []byte(`{"` + r.network + `":{}}`), nil
	default:
		return nil, errors.New("unexpected Docker inspect format")
	}
}

func finalProjectMountSafetySpec(t *testing.T, runtime *Runtime, snapshot tobari.ContextAuthoritySnapshot, plan tobari.WorkspaceEntryReconciliationPlan) finalWorkspaceRuntimeSpec {
	t.Helper()
	gitConfig, err := runtime.finalWorkspaceGitConfig(context.Background(), plan.Authority.SessionDefaults.GitIdentity, plan.Workspace.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := runtime.finalWorkspaceSpec(
		plan.Authority, plan.CreationDefaults, plan.Network, snapshot.Context,
		plan.Workspace.ProjectRoot, plan.Workspace.ID, plan.Authority.Runtime.Image,
		"sha256:"+strings.Repeat("a", 64), gitConfig,
	)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func replaceProjectDirectoryWithSymlink(t *testing.T, root, target string) {
	t.Helper()
	if err := os.Rename(root, root+".before-swap"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, root); err != nil {
		t.Fatal(err)
	}
}

func TestResolveProjectRootRejectsDockerMountDelimiter(t *testing.T) {
	base := canonicalTestDirectory(t)
	runtime, err := newRuntimeWithData(
		filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), &projectMountSafetyRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, suffix := range map[string]string{
		"comma":  ",bind-propagation=rshared",
		"tab":    "\toption",
		"escape": "\x1boption",
		"c1":     "\u0085option",
	} {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(base, "project"+suffix)
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.ResolveProjectRoot(context.Background(), root); err == nil || !strings.Contains(err.Error(), "exact Docker bind source") {
				t.Fatalf("delimiter project root error = %v", err)
			}
		})
	}
}

func TestLegacyProjectContainerRejectsMountDelimiterBeforeDocker(t *testing.T) {
	base := canonicalTestDirectory(t)
	runner := &projectMountSafetyRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectGitTestInstance(t, runtime)
	instance.Root = filepath.Join(filepath.Dir(instance.Root), "project,bind-propagation=rshared")
	if err := os.Mkdir(instance.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	err = runtime.ensureProjectContainerWithAuth(
		context.Background(), runtimeState(base), instance, "/profile", "project", "network",
		"172.29.0.2", "image", "sha256:source", projectAuthProjection{}, tobari.ManifestSourceAccessReadWrite,
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported Docker mount syntax") {
		t.Fatalf("legacy comma-delimited bind error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("Docker was called for rejected legacy source: %v", runner.calls)
	}
}

func TestFinalWorkspaceContainerRejectsMountDelimiterBeforeDocker(t *testing.T) {
	runtime, snapshot, plan := finalWorkspaceRuntimeFixture(t)
	runner := &projectMountSafetyRunner{}
	runtime.runner = runner
	spec := finalProjectMountSafetySpec(t, runtime, snapshot, plan)
	spec.ProjectRoot = filepath.Join(filepath.Dir(spec.ProjectRoot), "project,bind-propagation=rshared")
	if err := os.Mkdir(spec.ProjectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	spec.WorkspaceRoot, _ = runtime.projectContainerRoot(spec.ProjectRoot)
	container, network, _ := tobari.ProjectResourceNames(string(plan.Workspace.ID))
	err := runtime.ensureFinalWorkspaceContainer(context.Background(), plan, spec, container, network, plan.Network.WorkspaceIP, plan.Network.GatewayIP)
	if err == nil || !strings.Contains(err.Error(), "unsupported Docker mount syntax") {
		t.Fatalf("final comma-delimited bind error = %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("Docker was called for rejected final source: %v", runner.calls)
	}
}

func TestExistingStoppedProjectContainerDoesNotStartAfterBindSourceDrift(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		base := canonicalTestDirectory(t)
		runner := &projectMountSafetyRunner{exists: true}
		runtime, err := newRuntimeWithData(
			filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner,
		)
		if err != nil {
			t.Fatal(err)
		}
		instance := projectGitTestInstance(t, runtime)
		replaceProjectDirectoryWithSymlink(t, instance.Root, canonicalTestDirectory(t))
		err = runtime.ensureProjectContainerWithAuth(
			context.Background(), runtimeState(base), instance, "/profile", "project", "network",
			"172.29.0.2", "image", "sha256:source", projectAuthProjection{}, tobari.ManifestSourceAccessReadWrite,
		)
		if err == nil || !strings.Contains(err.Error(), "bind source changed") {
			t.Fatalf("legacy stopped-container drift error = %v", err)
		}
		if runner.starts != 0 || len(runner.calls) != 0 {
			t.Fatalf("Docker calls after legacy bind drift: starts=%d calls=%v", runner.starts, runner.calls)
		}
	})

	t.Run("final", func(t *testing.T) {
		runtime, snapshot, plan := finalWorkspaceRuntimeFixture(t)
		runner := &projectMountSafetyRunner{exists: true}
		runtime.runner = runner
		spec := finalProjectMountSafetySpec(t, runtime, snapshot, plan)
		replaceProjectDirectoryWithSymlink(t, spec.ProjectRoot, canonicalTestDirectory(t))
		container, network, _ := tobari.ProjectResourceNames(string(plan.Workspace.ID))
		err := runtime.ensureFinalWorkspaceContainer(context.Background(), plan, spec, container, network, plan.Network.WorkspaceIP, plan.Network.GatewayIP)
		if err == nil || !strings.Contains(err.Error(), "bind source changed") {
			t.Fatalf("final stopped-container drift error = %v", err)
		}
		if runner.starts != 0 || len(runner.calls) != 0 {
			t.Fatalf("Docker calls after final bind drift: starts=%d calls=%v", runner.starts, runner.calls)
		}
	})
}

func TestExistingStoppedProjectContainerRechecksBindSourceImmediatelyBeforeStart(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		base := canonicalTestDirectory(t)
		runtime, err := newRuntimeWithData(
			filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), &projectMountSafetyRunner{},
		)
		if err != nil {
			t.Fatal(err)
		}
		instance := projectGitTestInstance(t, runtime)
		workspaceRoot, err := runtime.projectContainerRoot(instance.Root)
		if err != nil {
			t.Fatal(err)
		}
		runner := &stoppedProjectMountSafetyRunner{
			projectID: instance.ID, spec: "sha256:source", root: instance.Root, workspaceRoot: workspaceRoot,
			network: "network", dns: "172.29.0.2",
		}
		runtime.runner = runner
		target := canonicalTestDirectory(t)
		runner.afterStatus = func() { replaceProjectDirectoryWithSymlink(t, instance.Root, target) }
		err = runtime.ensureProjectContainerWithAuth(
			context.Background(), runtimeState(base), instance, "/profile", "project", runner.network,
			runner.dns, "image", runner.spec, projectAuthProjection{}, tobari.ManifestSourceAccessReadWrite,
		)
		if err == nil || !strings.Contains(err.Error(), "before starting existing container") {
			t.Fatalf("legacy start-boundary drift error = %v", err)
		}
		if runner.starts != 0 {
			t.Fatalf("legacy drifted stopped container starts = %d", runner.starts)
		}
	})

	t.Run("final", func(t *testing.T) {
		runtime, snapshot, plan := finalWorkspaceRuntimeFixture(t)
		spec := finalProjectMountSafetySpec(t, runtime, snapshot, plan)
		runner := &stoppedProjectMountSafetyRunner{
			projectID: string(plan.Workspace.ID), spec: string(plan.Applied.ResolvedSpec), root: spec.ProjectRoot,
			workspaceRoot: spec.WorkspaceRoot, network: plan.Network.Network, dns: plan.Network.GatewayIP,
		}
		runtime.runner = runner
		target := canonicalTestDirectory(t)
		runner.afterStatus = func() { replaceProjectDirectoryWithSymlink(t, spec.ProjectRoot, target) }
		container, network, _ := tobari.ProjectResourceNames(string(plan.Workspace.ID))
		err := runtime.ensureFinalWorkspaceContainer(context.Background(), plan, spec, container, network, plan.Network.WorkspaceIP, plan.Network.GatewayIP)
		if err == nil || !strings.Contains(err.Error(), "before starting existing container") {
			t.Fatalf("final start-boundary drift error = %v", err)
		}
		if runner.starts != 0 {
			t.Fatalf("final drifted stopped container starts = %d", runner.starts)
		}
	})
}

func TestCreatedProjectContainerIsRemovedWhenBindSourceDriftsBeforeStart(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		base := canonicalTestDirectory(t)
		runner := &projectMountSafetyRunner{}
		runtime, err := newRuntimeWithData(
			filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner,
		)
		if err != nil {
			t.Fatal(err)
		}
		instance := projectGitTestInstance(t, runtime)
		target := canonicalTestDirectory(t)
		runner.afterCreate = func() { replaceProjectDirectoryWithSymlink(t, instance.Root, target) }
		err = runtime.ensureProjectContainerWithAuth(
			context.Background(), runtimeState(base), instance, "/profile", "project", "network",
			"172.29.0.2", "image", "sha256:source", projectAuthProjection{}, tobari.ManifestSourceAccessReadWrite,
		)
		if err == nil || !strings.Contains(err.Error(), "changed after container creation") {
			t.Fatalf("legacy create-boundary drift error = %v", err)
		}
		if runner.starts != 0 || runner.removes != 1 {
			t.Fatalf("legacy drift settlement: starts=%d removes=%d calls=%v", runner.starts, runner.removes, runner.calls)
		}
	})

	t.Run("final", func(t *testing.T) {
		runtime, snapshot, plan := finalWorkspaceRuntimeFixture(t)
		runner := &projectMountSafetyRunner{}
		runtime.runner = runner
		spec := finalProjectMountSafetySpec(t, runtime, snapshot, plan)
		target := canonicalTestDirectory(t)
		runner.afterCreate = func() { replaceProjectDirectoryWithSymlink(t, spec.ProjectRoot, target) }
		container, network, _ := tobari.ProjectResourceNames(string(plan.Workspace.ID))
		err := runtime.ensureFinalWorkspaceContainer(context.Background(), plan, spec, container, network, plan.Network.WorkspaceIP, plan.Network.GatewayIP)
		if err == nil || !strings.Contains(err.Error(), "changed after container creation") {
			t.Fatalf("final create-boundary drift error = %v", err)
		}
		if runner.starts != 0 || runner.removes != 1 {
			t.Fatalf("final drift settlement: starts=%d removes=%d calls=%v", runner.starts, runner.removes, runner.calls)
		}
	})
}
