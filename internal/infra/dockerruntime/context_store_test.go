package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

type contextSwitchRunner struct {
	running     bool
	failCompose bool
	runs        []runnerCall
}

func (r *contextSwitchRunner) Run(_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	if r.failCompose && len(args) > 0 && args[0] == "compose" {
		return errors.New("synthetic compose failure")
	}
	return nil
}

func (r *contextSwitchRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) > 0 && args[0] == "image" {
		if strings.Contains(strings.Join(args, " "), tobari.RuntimeImageAPILabel) {
			return compatibleImageInspection(), nil
		}
		versions, err := runtimeassets.Versions()
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf(`{"RepoDigests":[%q],"Architecture":"arm64","Os":"linux","Config":{"User":"1000:1000","Labels":{"io.tobari.gateway-api":"1","io.tobari.gateway-role":"enforcement"},"Entrypoint":["/opt/tobari/entrypoint.sh"]}}`, versions["GATEWAY_IMAGE"])), nil
	}
	if len(args) > 0 && args[0] == "version" {
		return []byte(`{"Os":"linux","Arch":"arm64"}`), nil
	}
	if len(args) >= 3 && args[0] == "inspect" {
		if strings.Contains(args[2], "NetworkSettings.Networks") {
			return []byte(`{}`), nil
		}
		if r.running {
			return []byte(`{"state":"running","health":"healthy"}`), nil
		}
		return []byte(`{"state":"exited","health":"none"}`), nil
	}
	return []byte{}, nil
}

func newContextSwitchRuntime(t *testing.T, runner *contextSwitchRunner) *Runtime {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(context.Background(), "project-tools", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeAdvanced, tobari.ContextSourceAccessReadWrite); err != nil {
		t.Fatal(err)
	}
	return runtime
}

func TestFreshObservationsCreateNoTobariOwnedFilesOrDockerCalls(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &contextSwitchRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := runtime.ListContexts(context.Background()); err != nil ||
		result.ContextState != tobari.ContextObservationSyntheticDefault || len(result.Items) != 0 {
		t.Fatalf("ListContexts() = %+v, %v", result, err)
	}
	if result, err := runtime.ShowContext(context.Background(), ""); err != nil ||
		result.ContextState != tobari.ContextObservationSyntheticDefault || result.ID != "" {
		t.Fatalf("ShowContext() = %+v, %v", result, err)
	}
	if result, err := runtime.AuthStatus(context.Background(), ""); err != nil ||
		result.ContextState != tobari.ContextObservationSyntheticDefault || result.ContextID != "" {
		t.Fatalf("AuthStatus() = %+v, %v", result, err)
	}
	if projects, err := runtime.ListProjects(context.Background()); err != nil || len(projects) != 0 {
		t.Fatalf("ListProjects() = %+v, %v", projects, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("fresh observation created durable paths: %+v", entries)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("fresh observation made Docker calls: %+v", runner.runs)
	}
}

func snapshotOwnedTree(t *testing.T, root string) []string {
	t.Helper()
	var snapshot []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry := relative + "|" + info.Mode().String()
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry += "|" + string(data)
		}
		snapshot = append(snapshot, entry)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(snapshot)
	return snapshot
}

func TestFreshObservationsAreConcurrentAndReadOnlyXDGCompatible(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	config, state, data := filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data")
	for _, directory := range []string{config, state, data} {
		if err := os.Mkdir(directory, 0o500); err != nil {
			t.Fatal(err)
		}
	}
	runtime, err := newRuntimeWithData(config, state, data, &contextSwitchRunner{})
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotOwnedTree(t, root)
	const readers = 24
	errorsSeen := make(chan error, readers*4)
	var wait sync.WaitGroup
	for index := 0; index < readers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if _, err := runtime.ListContexts(context.Background()); err != nil {
				errorsSeen <- err
			}
			if _, err := runtime.ShowContext(context.Background(), ""); err != nil {
				errorsSeen <- err
			}
			if _, err := runtime.AuthStatus(context.Background(), ""); err != nil {
				errorsSeen <- err
			}
			if _, err := runtime.ListProjects(context.Background()); err != nil {
				errorsSeen <- err
			}
		}()
	}
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		t.Errorf("concurrent read failed: %v", err)
	}
	if after := snapshotOwnedTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("read-only concurrent observations changed XDG state\nbefore=%v\nafter=%v", before, after)
	}
}

func TestExplicitFreshDefaultIsNotSyntheticAuthority(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &contextSwitchRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ShowContext(context.Background(), tobari.DefaultContextName); !errors.Is(err, tobari.ErrContextNotFound) {
		t.Fatalf("explicit fresh default = %v, want context not found", err)
	}
	if _, err := runtime.AuthStatus(context.Background(), tobari.DefaultContextName); err == nil {
		t.Fatal("explicit fresh default auth status unexpectedly succeeded")
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("explicit missing observation changed root: entries=%v err=%v", entries, err)
	}
}

func TestFreshExplicitDefaultCreateSucceedsOnceAndPreservesManifestOnDuplicate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &contextSwitchRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}

	created, err := runtime.CreateContext(
		context.Background(), tobari.DefaultContextName,
		tobari.OfficialRuntimeBase, tobari.ContextPolicyModeAdvanced, tobari.ContextSourceAccessReadOnly,
	)
	if err != nil {
		t.Fatalf("first CreateContext(default) error = %v", err)
	}
	if err := created.Validate(); err != nil {
		t.Fatalf("created report is invalid: %v", err)
	}
	if created.Task != tobari.TaskContextCreate ||
		created.ContextState != tobari.ContextObservationPersisted ||
		created.Name != tobari.DefaultContextName || !created.Active ||
		created.PolicyMode != tobari.ContextPolicyModeAdvanced ||
		created.SourceAccess != tobari.ContextSourceAccessReadOnly ||
		created.Cluster != tobari.ContextClusterStatusNotApplicable {
		t.Fatalf("created default Context = %+v", created)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("Context create made Docker calls: %+v", runner.runs)
	}

	manifestPath := runtime.contextManifestPath(tobari.DefaultContextName)
	manifestBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := runtime.readContextManifest(tobari.DefaultContextName)
	if err != nil || manifest.SourceAccess != tobari.ContextSourceAccessReadOnly {
		t.Fatalf("persisted source access = %q, error = %v", manifest.SourceAccess, err)
	}
	treeBefore := snapshotOwnedTree(t, root)
	if _, err := runtime.CreateContext(
		context.Background(), tobari.DefaultContextName,
		tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite,
	); !errors.Is(err, tobari.ErrContextExists) {
		t.Fatalf("second CreateContext(default) error = %v, want ErrContextExists", err)
	}
	manifestAfter, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manifestAfter, manifestBefore) {
		t.Fatalf("duplicate create changed manifest\nbefore=%s\nafter=%s", manifestBefore, manifestAfter)
	}
	if treeAfter := snapshotOwnedTree(t, root); !reflect.DeepEqual(treeAfter, treeBefore) {
		t.Fatalf("duplicate create changed Context state\nbefore=%v\nafter=%v", treeBefore, treeAfter)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("duplicate Context create made Docker calls: %+v", runner.runs)
	}
}

func TestCorruptStoredContextFailsClosedWithoutWrites(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &contextSwitchRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.contextDirectory(tobari.DefaultContextName), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime.contextManifestPath(tobari.DefaultContextName), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotOwnedTree(t, root)
	if _, err := runtime.ListContexts(context.Background()); err == nil {
		t.Fatal("corrupt Context list unexpectedly succeeded")
	}
	if _, err := runtime.ShowContext(context.Background(), ""); err == nil {
		t.Fatal("corrupt Context show unexpectedly succeeded")
	}
	if after := snapshotOwnedTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("corrupt Context observation changed state\nbefore=%v\nafter=%v", before, after)
	}
}

func TestStoredContextMissingSourceAccessFailsClosed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &contextSwitchRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	directory := runtime.contextDirectory(tobari.DefaultContextName)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{
		"schema_version": tobari.ContextSchemaVersion,
		"id":             "018bcfe5-687b-7000-8000-000000000099",
		"name":           tobari.DefaultContextName,
		"agent_profile":  tobari.DefaultProfile,
		"image":          tobari.OfficialRuntimeBase,
		"policy_mode":    tobari.ContextPolicyModeGuided,
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime.contextManifestPath(tobari.DefaultContextName), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotOwnedTree(t, root)
	if _, err := runtime.ShowContext(context.Background(), tobari.DefaultContextName); err == nil ||
		!strings.Contains(err.Error(), "source access") {
		t.Fatalf("missing source access error = %v", err)
	}
	if after := snapshotOwnedTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("missing source access changed state\nbefore=%v\nafter=%v", before, after)
	}
}

func TestListContextsRejectsExtraSymbolicLinkWithoutWrites(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &contextSwitchRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(
		context.Background(), "fixture", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite,
	); err != nil {
		t.Fatalf("initialize valid default Context fixture: %v", err)
	}
	linkTarget := t.TempDir()
	linkPath := filepath.Join(runtime.contextsDirectory(), "extra-link")
	if err := os.Symlink(linkTarget, linkPath); err != nil {
		t.Fatal(err)
	}
	before := snapshotOwnedTree(t, root)
	if _, err := runtime.ListContexts(context.Background()); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("ListContexts() error = %v, want symbolic-link rejection", err)
	}
	if after := snapshotOwnedTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("unsafe Context list changed state\nbefore=%v\nafter=%v", before, after)
	}
}

func contextSwitchState(runtime *Runtime, name string, root string) tobari.State {
	_ = runtime
	_ = name
	state := runtimeState(root)
	return state
}

func TestUseContextReportsUnconfiguredAndStoppedStatesWithoutStartingDocker(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		runner *contextSwitchRunner
		want   tobari.ContextClusterStatus
	}{
		{name: "unconfigured", runner: &contextSwitchRunner{}, want: tobari.ContextClusterStatusDefaultUpdated},
		{name: "stopped", runner: &contextSwitchRunner{}, want: tobari.ContextClusterStatusDefaultUpdated},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := newContextSwitchRuntime(t, test.runner)
			if test.name == "stopped" {
				state := contextSwitchState(runtime, tobari.DefaultContextName, t.TempDir())
				if err := runtime.writeState(state); err != nil {
					t.Fatal(err)
				}
			}
			result, err := runtime.UseContext(context.Background(), "project-tools")
			if err != nil {
				t.Fatalf("UseContext() error = %v", err)
			}
			if result.Cluster != test.want || !result.Active || result.Name != "project-tools" {
				t.Fatalf("UseContext() result = %+v, want cluster %q", result, test.want)
			}
			if len(test.runner.runs) != 0 {
				t.Fatalf("Docker Run calls = %+v, want none", test.runner.runs)
			}
		})
	}
}

func TestUseContextReportsAlreadyReadyWithoutReconcile(t *testing.T) {
	t.Parallel()
	runner := &contextSwitchRunner{running: true}
	runtime := newContextSwitchRuntime(t, runner)
	if err := runtime.writeState(contextSwitchState(runtime, tobari.DefaultContextName, t.TempDir())); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.UseContext(context.Background(), tobari.DefaultContextName)
	if err != nil {
		t.Fatalf("UseContext() error = %v", err)
	}
	if result.Cluster != tobari.ContextClusterStatusAlreadyReady {
		t.Fatalf("cluster status = %q, want already_ready", result.Cluster)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("Docker Run calls = %+v, want none", runner.runs)
	}
}

func TestUseContextChangesOnlyDefaultWhileClusterIsRunning(t *testing.T) {
	t.Parallel()
	runner := &contextSwitchRunner{running: true}
	runtime := newContextSwitchRuntime(t, runner)
	if err := runtime.writeState(contextSwitchState(runtime, tobari.DefaultContextName, t.TempDir())); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.UseContext(context.Background(), "project-tools")
	if err != nil {
		t.Fatalf("UseContext() error = %v", err)
	}
	if result.Cluster != tobari.ContextClusterStatusDefaultUpdated {
		t.Fatalf("cluster status = %q, want default_updated", result.Cluster)
	}
	state, configured, err := runtime.LoadState(context.Background())
	if err != nil || !configured {
		t.Fatalf("LoadState() = %+v, %t, %v", state, configured, err)
	}
	if state.ContextName != "" || state.PolicyDirectory == runtime.contextPaths("project-tools").PolicyDirectory {
		t.Fatalf("shared state acquired Context authority: %+v", state)
	}
	active, err := runtime.readActiveContext()
	if err != nil || active != "project-tools" {
		t.Fatalf("active Context = %q, error = %v", active, err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
		t.Fatalf("reconcile journal = exists:%t error:%v", exists, err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("Context default change touched Docker: %+v", runner.runs)
	}
}

func TestUseContextDoesNotConsultOrMutateClusterReconcileState(t *testing.T) {
	t.Parallel()
	runner := &contextSwitchRunner{running: true, failCompose: true}
	runtime := newContextSwitchRuntime(t, runner)
	previous := contextSwitchState(runtime, tobari.DefaultContextName, t.TempDir())
	if err := runtime.writeState(previous); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.UseContext(context.Background(), "project-tools"); err != nil {
		t.Fatalf("UseContext() error = %v", err)
	}
	active, err := runtime.readActiveContext()
	if err != nil || active != "project-tools" {
		t.Fatalf("current Context = %q, error = %v", active, err)
	}
	state, configured, err := runtime.LoadState(context.Background())
	if err != nil || !configured || state.ContextName != "" {
		t.Fatalf("shared state changed = %+v, configured=%t, error=%v", state, configured, err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("Context default change touched Docker: %+v", runner.runs)
	}
}

func TestConfigureContextShellPersistsOnlyAllowlistedContextSetting(t *testing.T) {
	t.Parallel()
	runner := &contextSwitchRunner{}
	runtime := newContextSwitchRuntime(t, runner)
	prompt := `\[\e[33m\]\$\[\e[0m\] `
	result, err := runtime.ConfigureContextShell(context.Background(), "project-tools", []tobari.ContextShellEnvironmentSetting{{
		Variable: "PS1", Source: tobari.ContextShellEnvironmentLiteral, Value: &prompt,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task != tobari.TaskConfigShell || result.Name != "project-tools" {
		t.Fatalf("configure result = %+v", result)
	}
	manifest, err := runtime.readContextManifest("project-tools")
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.ShellEnvironment) != 1 || manifest.ShellEnvironment[0].Value == nil ||
		*manifest.ShellEnvironment[0].Value != prompt {
		t.Fatalf("persisted shell environment = %+v", manifest.ShellEnvironment)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("shell configuration touched Docker: %+v", runner.runs)
	}

	if _, err := runtime.ConfigureContextShell(context.Background(), "project-tools", []tobari.ContextShellEnvironmentSetting{{
		Variable: "PS1", Source: tobari.ContextShellEnvironmentDefault,
	}}); err != nil {
		t.Fatal(err)
	}
	manifest, err = runtime.readContextManifest("project-tools")
	if err != nil || len(manifest.ShellEnvironment) != 0 {
		t.Fatalf("default shell setting = %+v, error = %v", manifest.ShellEnvironment, err)
	}
}

func TestConfigureContextShellCommitsOneValidatedBatch(t *testing.T) {
	t.Parallel()
	runtime := newContextSwitchRuntime(t, &contextSwitchRunner{})
	literal := "truecolor"
	changes := []tobari.ContextShellEnvironmentSetting{
		{Variable: "COLORTERM", Source: tobari.ContextShellEnvironmentLiteral, Value: &literal},
		{Variable: "TERM", Source: tobari.ContextShellEnvironmentInherit},
	}
	result, err := runtime.ConfigureContextShell(context.Background(), "project-tools", changes)
	if err != nil {
		t.Fatalf("ConfigureContextShell() error = %v", err)
	}
	if result.Task != tobari.TaskConfigShell || result.Name != "project-tools" {
		t.Fatalf("configure result = %+v", result)
	}
	manifest, err := runtime.readContextManifest("project-tools")
	if err != nil {
		t.Fatal(err)
	}
	complete, err := tobari.CompleteContextShellEnvironment(manifest.ShellEnvironment)
	if err != nil || complete[0].Source != tobari.ContextShellEnvironmentLiteral || complete[0].Value == nil ||
		*complete[0].Value != literal || complete[3].Source != tobari.ContextShellEnvironmentInherit {
		t.Fatalf("persisted shell batch = %+v", manifest.ShellEnvironment)
	}
	duplicate := append(append([]tobari.ContextShellEnvironmentSetting(nil), changes...), changes[0])
	if _, err := runtime.ConfigureContextShell(context.Background(), "project-tools", duplicate); err == nil {
		t.Fatal("duplicate shell batch was accepted")
	}
	after, err := runtime.readContextManifest("project-tools")
	if err != nil || !reflect.DeepEqual(after.ShellEnvironment, manifest.ShellEnvironment) {
		t.Fatalf("invalid batch changed manifest = %+v, error = %v", after.ShellEnvironment, err)
	}
}

func TestConfigureContextGitPersistsAtomicPairAndDefaultRemovesOverride(t *testing.T) {
	t.Parallel()
	runner := &contextSwitchRunner{}
	runtime := newContextSwitchRuntime(t, runner)
	name, email := "Tobari User", "tobari@example.com"
	result, err := runtime.ConfigureContextGit(context.Background(), "project-tools", tobari.ContextGitIdentitySetting{
		Source: tobari.ContextGitIdentityLiteral, Name: &name, Email: &email,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task != tobari.TaskConfigGit || result.GitIdentity.Source != tobari.ContextGitIdentityLiteral ||
		result.GitIdentity.Name == nil || *result.GitIdentity.Name != name ||
		result.GitIdentity.Email == nil || *result.GitIdentity.Email != email {
		t.Fatalf("literal Git configuration result = %+v", result)
	}
	manifest, err := runtime.readContextManifest("project-tools")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.GitIdentity == nil || manifest.GitIdentity.Source != tobari.ContextGitIdentityLiteral ||
		manifest.GitIdentity.Name == nil || *manifest.GitIdentity.Name != name ||
		manifest.GitIdentity.Email == nil || *manifest.GitIdentity.Email != email {
		t.Fatalf("persisted Git identity = %+v", manifest.GitIdentity)
	}

	result, err = runtime.ConfigureContextGit(context.Background(), "project-tools", tobari.ContextGitIdentitySetting{
		Source: tobari.ContextGitIdentityDefault,
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err = runtime.readContextManifest("project-tools")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.GitIdentity != nil || result.GitIdentity.Source != tobari.ContextGitIdentityDefault ||
		result.GitIdentity.Name != nil || result.GitIdentity.Email != nil {
		t.Fatalf("default Git configuration = manifest:%+v report:%+v", manifest.GitIdentity, result.GitIdentity)
	}
	data, err := os.ReadFile(runtime.contextManifestPath("project-tools"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "git_identity") {
		t.Fatalf("default Git identity remained persisted: %s", data)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("Git configuration touched Docker: %+v", runner.runs)
	}
}

func TestContextManifestRoundTripsEveryMaximumSchemaFiveProjectionValue(t *testing.T) {
	t.Parallel()
	runtime := newContextSwitchRuntime(t, &contextSwitchRunner{})
	shellValue := strings.Repeat("\x01", tobari.MaxContextShellValueBytes)
	for _, variable := range tobari.ContextShellEnvironmentVariables() {
		if _, err := runtime.ConfigureContextShell(
			context.Background(), "project-tools", []tobari.ContextShellEnvironmentSetting{{
				Variable: variable, Source: tobari.ContextShellEnvironmentLiteral, Value: &shellValue,
			}},
		); err != nil {
			t.Fatalf("configure maximum %s literal: %v", variable, err)
		}
	}
	// encoding/json escapes '<' as six bytes, exercising the same maximum
	// expansion reserved by maxContextManifestBytes for both Git fields.
	gitValue := strings.Repeat("<", tobari.MaxContextGitIdentityValueBytes)
	if _, err := runtime.ConfigureContextGit(
		context.Background(), "project-tools", tobari.ContextGitIdentitySetting{
			Source: tobari.ContextGitIdentityLiteral, Name: &gitValue, Email: &gitValue,
		},
	); err != nil {
		t.Fatalf("configure maximum Git identity pair: %v", err)
	}

	info, err := os.Stat(runtime.contextManifestPath("project-tools"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= maxContextManifestFixedJSONBytes || info.Size() > maxContextManifestBytes {
		t.Fatalf("maximum schema-1 manifest size = %d, bound = %d", info.Size(), maxContextManifestBytes)
	}
	manifest, err := runtime.readContextManifest("project-tools")
	if err != nil {
		t.Fatalf("read maximum schema-1 manifest: %v", err)
	}
	if len(manifest.ShellEnvironment) != len(tobari.ContextShellEnvironmentVariables()) ||
		manifest.GitIdentity == nil || manifest.GitIdentity.Name == nil || manifest.GitIdentity.Email == nil ||
		len(*manifest.GitIdentity.Name) != tobari.MaxContextGitIdentityValueBytes ||
		len(*manifest.GitIdentity.Email) != tobari.MaxContextGitIdentityValueBytes {
		t.Fatalf(
			"maximum schema-1 manifest did not round-trip: shell=%d git-present=%t",
			len(manifest.ShellEnvironment), manifest.GitIdentity != nil,
		)
	}
}

func TestActiveContextDocumentRetainsIndependentSizeBound(t *testing.T) {
	t.Parallel()
	runtime := newContextSwitchRuntime(t, &contextSwitchRunner{})
	if maxActiveContextDocumentBytes >= maxContextManifestBytes {
		t.Fatalf(
			"active Context bound %d is not independent from manifest bound %d",
			maxActiveContextDocumentBytes, maxContextManifestBytes,
		)
	}
	oversized := strings.Repeat("x", maxActiveContextDocumentBytes+1)
	if err := os.WriteFile(runtime.activeContextPath(), []byte(oversized), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.readActiveContext(); err == nil {
		t.Fatal("oversized active Context document was accepted")
	}
}
