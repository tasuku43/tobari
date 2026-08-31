package dockerruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type workspaceMountGuardRunner struct {
	listing     string
	observation map[string]string
	inspectErr  map[string]error
	inspectArgs []string
}

type workspaceMountCleanupRunner struct {
	fail        bool
	canceled    bool
	hasDeadline bool
}

func (*workspaceMountCleanupRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *workspaceMountCleanupRunner) Output(ctx context.Context, args, _ []string) ([]byte, error) {
	if len(args) != 3 || args[0] != "rm" || args[1] != "-f" {
		return nil, errors.New("unexpected cleanup argv")
	}
	r.canceled = ctx.Err() != nil
	_, r.hasDeadline = ctx.Deadline()
	if r.fail {
		return []byte("synthetic cleanup failure"), errors.New("cleanup failed")
	}
	return nil, nil
}

func (*workspaceMountGuardRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *workspaceMountGuardRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) == 0 {
		return nil, errors.New("missing Docker argv")
	}
	switch args[0] {
	case "ps":
		return []byte(r.listing), nil
	case "inspect":
		r.inspectArgs = append([]string(nil), args...)
		id := args[len(args)-1]
		if err := r.inspectErr[id]; err != nil {
			return nil, err
		}
		value, ok := r.observation[id]
		if !ok {
			return nil, errors.New("missing observation")
		}
		return []byte(value), nil
	default:
		return nil, errors.New("unexpected Docker argv")
	}
}

func TestWorkspaceMountGuardProjectsOnlyOwnedDockerEvidence(t *testing.T) {
	base := canonicalTestDirectory(t)
	target := filepath.Join(base, "project")
	id := strings.Repeat("9", 64)
	runner := &workspaceMountGuardRunner{
		listing:     id + "\n",
		observation: map[string]string{id: workspaceMountGuardJSON(id, target, true, false)},
	}
	runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.requireNoLiveWritableWorkspaceAncestor(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if len(runner.inspectArgs) < 3 {
		t.Fatalf("inspect argv = %v", runner.inspectArgs)
	}
	format := runner.inspectArgs[2]
	if strings.Contains(format, `{{json .Mounts}}`) || !strings.Contains(format, `{{range $index, $mount := .Mounts}}`) ||
		!strings.Contains(format, `"destination":{{json $mount.Destination}}`) {
		t.Fatalf("mount evidence format is not a narrow projection: %s", format)
	}
}

func workspaceMountGuardJSON(id, source string, writable, running bool) string {
	return `{"id":"` + id + `","owner":"` + ownerValue + `","component":"tobari","workspace":"workspace",` +
		`"role":"` + projectWorkRole + `","running":` + boolJSON(running) + `,"env":["TOBARI_ROOT=/workspace/project"],` +
		`"mounts":[{"type":"bind","source":"` + source + `","destination":"/workspace/project","rw":` + boolJSON(writable) + `}]}`
}

func boolJSON(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func TestWorkspaceMountGuardAllowsOnlyNonMutatingOverlapStates(t *testing.T) {
	base := canonicalTestDirectory(t)
	target := filepath.Join(base, "parent", "child")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 64)
	tests := map[string]struct {
		source   string
		writable bool
		running  bool
	}{
		"same root":           {source: target, writable: true, running: true},
		"descendant":          {source: filepath.Join(target, "nested"), writable: true, running: true},
		"read-only ancestor":  {source: filepath.Dir(target), writable: false, running: true},
		"stopped rw ancestor": {source: filepath.Dir(target), writable: true, running: false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runner := &workspaceMountGuardRunner{listing: id + "\n", observation: map[string]string{id: workspaceMountGuardJSON(id, test.source, test.writable, test.running)}}
			runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.requireNoLiveWritableWorkspaceAncestor(context.Background(), target); err != nil {
				t.Fatalf("mount guard rejected allowed %s: %v", name, err)
			}
		})
	}
}

func TestWorkspaceMountGuardRejectsLiveWritableStrictAncestor(t *testing.T) {
	base := canonicalTestDirectory(t)
	target := filepath.Join(base, "parent", "child")
	id := strings.Repeat("b", 64)
	runner := &workspaceMountGuardRunner{listing: id + "\n", observation: map[string]string{id: workspaceMountGuardJSON(id, filepath.Dir(target), true, true)}}
	runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.requireNoLiveWritableWorkspaceAncestor(context.Background(), target)
	var public *fault.Error
	if !errors.As(err, &public) || public.Code != "workspace_entry_overlap_unsafe" || public.Kind != fault.KindRejected ||
		public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
		t.Fatalf("live writable ancestor fault = %#v", err)
	}
}

func TestWorkspaceMountGuardFailsClosedOnUntrustedDockerEvidence(t *testing.T) {
	base := canonicalTestDirectory(t)
	target := filepath.Join(base, "parent", "child")
	id := strings.Repeat("c", 64)
	valid := workspaceMountGuardJSON(id, filepath.Dir(target), false, false)
	tests := map[string]*workspaceMountGuardRunner{
		"invalid id":         {listing: "short\n"},
		"duplicate id":       {listing: id + "\n" + id + "\n"},
		"inspect failure":    {listing: id + "\n", inspectErr: map[string]error{id: errors.New("inspect failed")}},
		"foreign owner":      {listing: id + "\n", observation: map[string]string{id: strings.Replace(valid, `"owner":"`+ownerValue+`"`, `"owner":"foreign"`, 1)}},
		"missing root mount": {listing: id + "\n", observation: map[string]string{id: strings.Replace(valid, `"destination":"/workspace/project"`, `"destination":"/other"`, 1)}},
	}
	for name, runner := range tests {
		t.Run(name, func(t *testing.T) {
			if runner.observation == nil {
				runner.observation = map[string]string{}
			}
			if runner.inspectErr == nil {
				runner.inspectErr = map[string]error{}
			}
			runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner)
			if err != nil {
				t.Fatal(err)
			}
			err = runtime.requireNoLiveWritableWorkspaceAncestor(context.Background(), target)
			var public *fault.Error
			if !errors.As(err, &public) || public.Code != "workspace_entry_overlap_unverified" || public.Kind != fault.KindContract {
				t.Fatalf("unsafe evidence fault = %#v", err)
			}
		})
	}
}

func TestWorkspaceMountCleanupUsesBoundedUncanceledContextAndClassifiesFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(boolJSON(fail), func(t *testing.T) {
			runner := &workspaceMountCleanupRunner{fail: fail}
			base := canonicalTestDirectory(t)
			runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner)
			if err != nil {
				t.Fatal(err)
			}
			runtime.lifetimeContext = context.Background()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err = runtime.removeUnstartedProjectContainer(ctx, "workspace", workspaceMountGuardBlocked())
			if runner.canceled || !runner.hasDeadline {
				t.Fatalf("cleanup context canceled=%t deadline=%t", runner.canceled, runner.hasDeadline)
			}
			public, ok := fault.PublicCopy(err)
			if !ok {
				t.Fatalf("cleanup result is unclassified: %v", err)
			}
			if fail {
				if public.Code != "workspace_entry_cleanup_failed" || public.Kind != fault.KindUnavailable || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangePartial {
					t.Fatalf("cleanup failure = %#v", public)
				}
			} else if public.Code != "workspace_entry_overlap_unsafe" || public.ChangeState != fault.ChangeNone {
				t.Fatalf("successful cleanup changed guard classification = %#v", public)
			}
		})
	}
}

func TestLiveWritableAncestorBlocksBeforeProjectContainerCreation(t *testing.T) {
	t.Run("legacy", func(t *testing.T) {
		base := canonicalTestDirectory(t)
		runner := &projectMountSafetyRunner{}
		runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner)
		if err != nil {
			t.Fatal(err)
		}
		instance := projectGitTestInstance(t, runtime)
		runner.guardID = strings.Repeat("d", 64)
		runner.guardJSON = workspaceMountGuardJSON(runner.guardID, filepath.Dir(instance.Root), true, true)
		err = runtime.ensureProjectContainerWithAuth(context.Background(), runtimeState(base), instance, "/profile", "project", "network", "172.29.0.2", "image", "sha256:source", projectAuthProjection{}, tobari.ManifestSourceAccessReadWrite)
		if err == nil || runner.exists || runner.starts != 0 || runner.removes != 0 {
			t.Fatalf("legacy overlap settlement: err=%v starts=%d removes=%d", err, runner.starts, runner.removes)
		}
	})

	t.Run("final", func(t *testing.T) {
		runtime, snapshot, plan := finalWorkspaceRuntimeFixture(t)
		runner := &projectMountSafetyRunner{guardID: strings.Repeat("e", 64)}
		runtime.runner = runner
		spec := finalProjectMountSafetySpec(t, runtime, snapshot, plan)
		runner.guardJSON = workspaceMountGuardJSON(runner.guardID, filepath.Dir(spec.ProjectRoot), true, true)
		container, network, _ := tobari.ProjectResourceNames(string(plan.Workspace.ID))
		err := runtime.ensureFinalWorkspaceContainer(context.Background(), plan, spec, container, network, plan.Network.WorkspaceIP, plan.Network.GatewayIP)
		if err == nil || runner.exists || runner.starts != 0 || runner.removes != 0 {
			t.Fatalf("final overlap settlement: err=%v starts=%d removes=%d", err, runner.starts, runner.removes)
		}
	})
}

func TestFinalWorkspaceMountGuardFailurePrecedesNetworkMutation(t *testing.T) {
	runtime, snapshot, plan := finalWorkspaceRuntimeFixture(t)
	runner := &projectMountSafetyRunner{guardID: strings.Repeat("f", 64)}
	runtime.runner = runner
	spec := finalProjectMountSafetySpec(t, runtime, snapshot, plan)
	runner.guardJSON = workspaceMountGuardJSON(runner.guardID, filepath.Dir(spec.ProjectRoot), true, true)
	if _, err := runtime.reconcileFinalWorkspaceDocker(context.Background(), plan, spec); err == nil {
		t.Fatal("live writable ancestor passed final Docker reconciliation")
	}
	for _, call := range runner.calls {
		if len(call) > 0 && (call[0] == "network" || call[0] == "create" || call[0] == "rm" || call[0] == "start") {
			t.Fatalf("mount guard failure crossed Docker mutation: %v", runner.calls)
		}
	}
}
