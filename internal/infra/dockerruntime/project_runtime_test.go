package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/capabilityprofile"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func projectRuntimeInstance(t *testing.T, runtime *Runtime) tobari.ProjectInstance {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	return instance
}

func projectRuntimeContext(t *testing.T, runtime *Runtime, instance tobari.ProjectInstance) tobari.ContextManifest {
	t.Helper()
	manifest, _, err := runtime.contextByID(instance.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ShellEnvironment = nil
	return manifest
}

func TestInspectProjectRuntimeClassifiesDockerUnreachable(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputErr: errors.New("Docker daemon unavailable")}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	diagnostic, err := runtime.InspectProjectRuntime(context.Background(), instance)
	if err != nil || diagnostic != tobari.RuntimeDiagnosticUnreachable {
		t.Fatalf("InspectProjectRuntime() = (%q, %v)", diagnostic, err)
	}
}

func TestInspectProjectRuntimeClassifiesIncompleteStateBeforeDocker(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	instance.Incomplete = true
	diagnostic, err := runtime.InspectProjectRuntime(context.Background(), instance)
	if err != nil || diagnostic != tobari.RuntimeDiagnosticIncomplete {
		t.Fatalf("InspectProjectRuntime() = (%q, %v), want incomplete", diagnostic, err)
	}
	if len(runner.outputs) != 0 || len(runner.runs) != 0 {
		t.Fatalf("InspectProjectRuntime() performed Docker calls: outputs=%v runs=%v", runner.outputs, runner.runs)
	}
}

func TestProjectSessionAttachedReadsActiveExecIDs(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputData: []byte(`["exec-id"]`)}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	attached, err := runtime.ProjectSessionAttached(context.Background(), instance)
	if err != nil || !attached {
		t.Fatalf("ProjectSessionAttached() = (%t, %v), want attached", attached, err)
	}
	container, _, err := tobari.ProjectResourceNames(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 1 || strings.Join(runner.outputs[0].args, " ") != strings.Join([]string{"inspect", "--format", "{{json .ExecIDs}}", container}, " ") {
		t.Fatalf("Docker inspect calls = %+v", runner.outputs)
	}
}

func TestProjectSessionAttachedAcceptsEmptyExecIDs(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputData: []byte(`[]`)}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	attached, err := runtime.ProjectSessionAttached(context.Background(), instance)
	if err != nil || attached {
		t.Fatalf("ProjectSessionAttached() = (%t, %v), want detached", attached, err)
	}
}

func TestProjectSessionAttachedTreatsMissingContainerAsDetached(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputErr: errors.New("No such object")}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	attached, err := runtime.ProjectSessionAttached(context.Background(), instance)
	if err != nil || attached {
		t.Fatalf("ProjectSessionAttached() = (%t, %v), want detached missing resource", attached, err)
	}
}

func TestProjectSessionAttachedRejectsMalformedExecIDs(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputData: []byte(`{"exec_id":"exec-id"}`)}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	if _, err := runtime.ProjectSessionAttached(context.Background(), instance); err == nil {
		t.Fatal("ProjectSessionAttached() accepted malformed ExecIDs output")
	}
}

func TestEnterProjectRuntimeMirrorsHostCWDPath(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	manifest := projectRuntimeContext(t, runtime, instance)
	nested := filepath.Join(instance.Root, "root")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.EnterProjectRuntime(context.Background(), instance, manifest, nested, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 1 {
		t.Fatalf("EnterProjectRuntime() run count = %d, want 1", len(runner.runs))
	}
	want := "/workspace" + nested
	container, _, err := tobari.ProjectResourceNames(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	uid, gid := currentIDs()
	capabilities, err := json.Marshal(tobari.NewHostLoopbackCapabilityProjection())
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{
		"exec", "-i", "-t", "--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		"--env", "BROWSER=" + workspaceBrowserOpenerPath,
		"--env", "GH_BROWSER=" + workspaceBrowserOpenerPath,
		"--env", workspaceBrowserSocketEnv + "=" + browserSocketEnvironment(t, runner.runs[0].args),
		"--env", "PS1=" + projectInteractivePrompt,
		"--env", "PROMPT_COMMAND=PS1=" + bashSingleQuoted(projectInteractivePrompt),
		"--env", "TOBARI_CAPABILITIES_JSON=" + string(capabilities),
		"--workdir", want, container, "/bin/bash",
	}
	if got := strings.Join(runner.runs[0].args, " "); got != strings.Join(wantArgs, " ") {
		t.Fatalf("EnterProjectRuntime() argv = %q, want %q", got, strings.Join(wantArgs, " "))
	}
	for index, arg := range runner.runs[0].args {
		if arg == "--workdir" {
			if index+1 >= len(runner.runs[0].args) || runner.runs[0].args[index+1] != want {
				t.Fatalf("EnterProjectRuntime() workdir = %q, want %q", runner.runs[0].args[index+1], want)
			}
			return
		}
	}
	t.Fatalf("EnterProjectRuntime() args = %v, missing --workdir", runner.runs[0].args)
}

func browserSocketEnvironment(t *testing.T, args []string) string {
	t.Helper()
	prefix := workspaceBrowserSocketEnv + "="
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			value := strings.TrimPrefix(arg, prefix)
			if !strings.HasPrefix(value, "/run/tobari-browser-") || !strings.HasSuffix(value, ".sock") {
				t.Fatalf("invalid Workspace browser socket environment %q", arg)
			}
			return value
		}
	}
	t.Fatal("Workspace browser socket environment is missing")
	return ""
}

func TestEnterProjectRuntimeSetsPromptWithoutUserName(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	manifest := projectRuntimeContext(t, runtime, instance)
	if _, err := runtime.EnterProjectRuntime(context.Background(), instance, manifest, instance.Root, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 1 {
		t.Fatalf("EnterProjectRuntime() run count = %d, want 1", len(runner.runs))
	}
	args := strings.Join(runner.runs[0].args, "\n")
	for _, want := range []string{
		"PS1=" + projectInteractivePrompt,
		"PROMPT_COMMAND=PS1=" + bashSingleQuoted(projectInteractivePrompt),
	} {
		if !strings.Contains(args, want) {
			t.Fatalf("EnterProjectRuntime() args missing %q in %v", want, runner.runs[0].args)
		}
	}
	if strings.Contains(projectInteractivePrompt, "\\u") {
		t.Fatalf("projectInteractivePrompt includes username escape: %q", projectInteractivePrompt)
	}
}

func TestEnterProjectRuntimeProjectsAmbientHostLoopbackCapabilityAndRevokesLease(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	manifest := projectRuntimeContext(t, runtime, instance)
	if _, err := runtime.EnterProjectRuntime(context.Background(), instance, manifest, instance.Root, nil, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 1 {
		t.Fatalf("run count = %d", len(runner.runs))
	}
	joined := strings.Join(runner.runs[0].args, "\n")
	for _, want := range []string{"TOBARI_CAPABILITIES_JSON=", `"url_template":"http://host.tobari.test:{port}"`, `"minimum_port":1024`, `"lifetime":"attachment"`, `"host_docker_control":"unavailable"`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("exec args lack %q: %s", want, joined)
		}
	}
	var routes tobari.HostLoopbackRegistry
	if err := readStrictJSON(runtime.hostLoopbackRegistryPath(), &routes); err != nil {
		t.Fatal(err)
	}
	var grants tobari.AttachmentGrantRegistry
	if err := readStrictJSON(runtime.attachmentGrantRegistryPath(), &grants); err != nil {
		t.Fatal(err)
	}
	if len(routes.Routes) != 0 || len(grants.Grants) != 0 {
		t.Fatalf("lease survived exit: routes=%+v grants=%+v", routes, grants)
	}
}

func TestProjectShellExecEnvironmentUsesOnlyDeclaredSourcesAndQuotesPrompt(t *testing.T) {
	literal := "truecolor"
	manifest := tobari.ContextManifest{
		SchemaVersion: tobari.ContextSchemaVersion,
		ID:            "018bcfe5-687b-7000-8000-000000000000", Name: "default",
		AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
		PolicyMode: tobari.ContextPolicyModeGuided, SourceAccess: tobari.ContextSourceAccessReadWrite,
		PolicyRevision: tobari.DefaultContextPolicyRevision(),
		ShellEnvironment: []tobari.ContextShellEnvironmentSetting{
			{Variable: "PS1", Source: tobari.ContextShellEnvironmentInherit},
			{Variable: "TERM", Source: tobari.ContextShellEnvironmentInherit},
			{Variable: "COLORTERM", Source: tobari.ContextShellEnvironmentLiteral, Value: &literal},
			{Variable: "NO_COLOR", Source: tobari.ContextShellEnvironmentInherit},
		},
	}
	host := map[string]string{
		"PS1":      "\\[\\e[33m\\]$'\\[\\e[0m\\] ",
		"TERM":     "xterm-256color",
		"GH_TOKEN": "must-not-cross",
	}
	environment, err := projectShellExecEnvironment(manifest, func(name string) (string, bool) {
		value, found := host[name]
		return value, found
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(environment, "\n")
	for _, want := range []string{
		"PS1=" + host["PS1"],
		"PROMPT_COMMAND=PS1=" + bashSingleQuoted(host["PS1"]),
		"COLORTERM=truecolor",
		"TERM=xterm-256color",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("shell environment %q lacks %q", joined, want)
		}
	}
	for _, absent := range []string{"GH_TOKEN", "NO_COLOR="} {
		if strings.Contains(joined, absent) {
			t.Fatalf("shell environment copied undeclared or absent value %q: %q", absent, joined)
		}
	}
}

func TestProjectShellExecEnvironmentFallsBackWhenInheritedPS1IsAbsent(t *testing.T) {
	manifest := tobari.ContextManifest{
		SchemaVersion: tobari.ContextSchemaVersion,
		ID:            "018bcfe5-687b-7000-8000-000000000000", Name: "default",
		AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
		PolicyMode:       tobari.ContextPolicyModeGuided,
		SourceAccess:     tobari.ContextSourceAccessReadWrite,
		PolicyRevision:   tobari.DefaultContextPolicyRevision(),
		ShellEnvironment: tobari.InitialContextShellEnvironment(),
	}
	environment, err := projectShellExecEnvironment(manifest, func(string) (string, bool) { return "", false })
	if err != nil {
		t.Fatal(err)
	}
	if got, want := environment[0], "PS1="+projectInteractivePrompt; got != want {
		t.Fatalf("fallback prompt = %q, want %q", got, want)
	}
}

func TestProjectShellExecEnvironmentRejectsOversizedInheritedValue(t *testing.T) {
	manifest := tobari.ContextManifest{
		SchemaVersion: tobari.ContextSchemaVersion,
		ID:            "018bcfe5-687b-7000-8000-000000000000", Name: "default",
		AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
		PolicyMode: tobari.ContextPolicyModeGuided, SourceAccess: tobari.ContextSourceAccessReadWrite,
		PolicyRevision: tobari.DefaultContextPolicyRevision(),
		ShellEnvironment: []tobari.ContextShellEnvironmentSetting{
			{Variable: "TERM", Source: tobari.ContextShellEnvironmentInherit},
		},
	}
	_, err := projectShellExecEnvironment(manifest, func(string) (string, bool) {
		return strings.Repeat("x", tobari.MaxContextShellValueBytes+1), true
	})
	if err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("oversized inherited value error = %v", err)
	}
}

func TestProjectContainerRootPreservesHostHomeRelativePath(t *testing.T) {
	t.Parallel()
	hostHome := t.TempDir()
	projectRoot := filepath.Join(hostHome, "path", "to")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	containerRoot, err := projectContainerRootForHostHome(projectRoot, hostHome)
	if err != nil {
		t.Fatalf("projectContainerRootForHostHome() error = %v", err)
	}
	if containerRoot != "/var/lib/tobari/path/to" {
		t.Fatalf("projectContainerRootForHostHome() = %q, want /var/lib/tobari/path/to", containerRoot)
	}

	nested := filepath.Join(projectRoot, "src")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	workdir, err := mapProjectCWDToContainer(projectRoot, nested, containerRoot)
	if err != nil {
		t.Fatalf("mapProjectCWDToContainer() error = %v", err)
	}
	if workdir != "/var/lib/tobari/path/to/src" {
		t.Fatalf("mapProjectCWDToContainer() = %q, want /var/lib/tobari/path/to/src", workdir)
	}
}

func TestProjectContainerRootKeepsHostHomeExternalPathMapping(t *testing.T) {
	t.Parallel()
	hostHome := t.TempDir()
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	containerRoot, err := projectContainerRootForHostHome(projectRoot, hostHome)
	if err != nil {
		t.Fatalf("projectContainerRootForHostHome() error = %v", err)
	}
	canonicalRoot, err := canonicalPathWithMissing(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	want := "/workspace" + filepath.ToSlash(canonicalRoot)
	if containerRoot != want {
		t.Fatalf("projectContainerRootForHostHome() = %q, want %q", containerRoot, want)
	}
}

func TestEnsureProjectHomeMountTargetCreatesOwnerOnlyScaffold(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := ensureProjectHomeMountTarget(home, "/var/lib/tobari/workspace/child-workspace"); err != nil {
		t.Fatalf("ensureProjectHomeMountTarget() error = %v", err)
	}
	for _, relative := range []string{"workspace", filepath.Join("workspace", "child-workspace")} {
		info, err := os.Lstat(filepath.Join(home, relative))
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("mount scaffold %q mode = %v, want owner-only directory", relative, info.Mode())
		}
	}
}

func TestEnsureProjectHomeMountTargetRejectsUnsafeExistingPath(t *testing.T) {
	t.Parallel()
	for _, setup := range []struct {
		name string
		make func(string) error
	}{
		{name: "symlink", make: func(path string) error { return os.Symlink("elsewhere", path) }},
		{name: "broad directory", make: func(path string) error {
			if err := os.Mkdir(path, 0o700); err != nil {
				return err
			}
			return os.Chmod(path, 0o755)
		}},
		{name: "regular file", make: func(path string) error { return os.WriteFile(path, []byte("fixture"), 0o600) }},
	} {
		setup := setup
		t.Run(setup.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			if err := setup.make(filepath.Join(home, "workspace")); err != nil {
				t.Fatal(err)
			}
			if err := ensureProjectHomeMountTarget(home, "/var/lib/tobari/workspace/project"); err == nil {
				t.Fatal("ensureProjectHomeMountTarget() accepted an unsafe existing path")
			}
		})
	}
}

func TestEnsureProjectHomeMountTargetIgnoresExternalMount(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := ensureProjectHomeMountTarget(home, "/workspace/tmp/project"); err != nil {
		t.Fatalf("ensureProjectHomeMountTarget() external target error = %v", err)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("external mount created Workspace-home entries: %v", entries)
	}
}

func TestProjectContainerRootRejectsHostHomeOrAncestor(t *testing.T) {
	t.Parallel()
	hostHome := t.TempDir()
	for _, root := range []string{hostHome, filepath.Dir(hostHome)} {
		if _, err := projectContainerRootForHostHome(root, hostHome); err == nil {
			t.Fatalf("projectContainerRootForHostHome(%q) accepted a protected root", root)
		}
	}
}

func TestMapProjectCWDToContainerRejectsSibling(t *testing.T) {
	t.Parallel()
	hostHome := t.TempDir()
	projectRoot := filepath.Join(hostHome, "project")
	sibling := filepath.Join(hostHome, "project-other")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := mapProjectCWDToContainer(projectRoot, sibling, "/var/lib/tobari/project"); err == nil {
		t.Fatal("mapProjectCWDToContainer() accepted a sibling path")
	}
}

func TestDeleteProjectRemovesLogicalStateWhenRuntimeResourcesAreMissing(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputErr: errors.New("No such object")}
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	home, err := runtime.ProjectHome(context.Background(), instance)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.DeleteProject(context.Background(), instance); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("project home stat error = %v, want removed", err)
	}
	if _, found, err := runtime.ResolveProject(context.Background(), instance.Root); err != nil || found {
		t.Fatalf("ResolveProject() = found=%t err=%v, want not found", found, err)
	}
}

func TestDeleteProjectRemovesRootIndexOnlyStateWithoutRebuildingRuntime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputErr: errors.New("No such object")}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	statePath, err := runtime.projectStatePath(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	cleanupOnly, found, err := runtime.ResolveProject(context.Background(), instance.Root)
	if err != nil || !found || !cleanupOnly.Incomplete {
		t.Fatalf("ResolveProject() = (%+v, %t, %v), want cleanup-only record", cleanupOnly, found, err)
	}
	if err := runtime.DeleteProject(context.Background(), cleanupOnly); err != nil {
		t.Fatalf("DeleteProject() = %v", err)
	}
	if _, found, err := runtime.ResolveProject(context.Background(), instance.Root); err != nil || found {
		t.Fatalf("ResolveProject() after cleanup = found=%t err=%v, want absent", found, err)
	}
}

func TestEnsureProjectRuntimeRejectsCleanupOnlyRecordBeforeDocker(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	instance.Incomplete = true
	if _, err := runtime.EnsureProjectRuntime(context.Background(), runtimeState(root), instance); err == nil {
		t.Fatal("EnsureProjectRuntime() rebuilt a cleanup-only record")
	}
	if len(runner.outputs) != 0 || len(runner.runs) != 0 {
		t.Fatalf("Docker calls for cleanup-only record: outputs=%v runs=%v", runner.outputs, runner.runs)
	}
}

func TestEnsureProjectAgentStateMergesSharedAndLocalSettings(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	profile, err := runtime.ensureSharedProfile(tobari.DefaultProfile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "common", "settings.json"), []byte(`{"shared":true,"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	if err := runtime.ensureProjectAgentState(instance.ID, profile); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(runtime.projectHomePath(instance.ID), ".claude", "settings.json")
	localSettingsPath := filepath.Join(runtime.projectHomePath(instance.ID), ".claude", projectLocalSettingsFile)
	if err := os.WriteFile(localSettingsPath, []byte(`{"theme":"light","local":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "common", "settings.json"), []byte(`{"shared":false,"theme":"blue","new":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureProjectAgentState(instance.ID, profile); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["shared"] != false || got["theme"] != "light" || got["local"] != true || got["new"] != true {
		t.Fatalf("merged settings = %v", got)
	}
}

type projectReconcileRunner struct {
	failOn          func([]string) bool
	imageData       []byte
	imageID         string
	networkExists   bool
	containerExists bool
	containerSpec   string
	instanceID      string
	gatewayNetworks map[string]string
	sourceRoot      string
	workspaceRoot   string
	calls           [][]string
}

type interruptedProjectReconcileRunner struct {
	inner       *projectReconcileRunner
	cancel      context.CancelFunc
	onInterrupt func()
	interrupted bool
}

func (r *interruptedProjectReconcileRunner) Run(
	ctx context.Context, args []string, environment []string, in io.Reader, out, errOut io.Writer,
) error {
	return r.inner.Run(ctx, args, environment, in, out, errOut)
}

func (r *interruptedProjectReconcileRunner) Output(
	ctx context.Context, args, environment []string,
) ([]byte, error) {
	if !r.interrupted && len(args) > 0 && args[0] == "run" && args[len(args)-1] == "gateway" {
		r.interrupted = true
		if r.onInterrupt != nil {
			r.onInterrupt()
		}
		r.cancel()
		return nil, ctx.Err()
	}
	return r.inner.Output(ctx, args, environment)
}

type projectSpecDriftRunner struct {
	instanceID string
	stale      bool
	calls      [][]string
}

type projectReadinessRunner struct {
	state  string
	health string
}

func (r *projectReadinessRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *projectReadinessRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return []byte(fmt.Sprintf(`{"state":%q,"health":%q}`, r.state, r.health)), nil
}

func (r *projectSpecDriftRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *projectSpecDriftRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{}, args...))
	if len(args) == 0 {
		return nil, errors.New("empty Docker argv")
	}
	switch args[0] {
	case "inspect":
		format := ""
		if len(args) > 2 {
			format = args[2]
		}
		switch {
		case strings.Contains(format, ".State.Health"):
			return []byte(`{"state":"running","health":"healthy"}`), nil
		case strings.Contains(format, projectSpecLabel):
			if r.stale {
				return []byte("sha256:stale\n"), nil
			}
			return []byte("sha256:desired\n"), nil
		case strings.Contains(format, projectIDLabel):
			return []byte(r.instanceID + "\n"), nil
		case strings.Contains(format, projectRoleLabel):
			return []byte(projectWorkRole + "\n"), nil
		case strings.Contains(format, ownerLabel):
			return []byte(ownerValue + "\n"), nil
		default:
			return []byte("container-id\n"), nil
		}
	case "rm":
		r.stale = false
		return nil, nil
	case "create", "start":
		return nil, nil
	default:
		return nil, nil
	}
}

type projectExitRunner struct{ code int }

func (r *projectExitRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return projectExitError{code: r.code}
}

func (r *projectExitRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, nil
}

type projectExitError struct{ code int }

func (e projectExitError) Error() string { return fmt.Sprintf("child exited with status %d", e.code) }
func (e projectExitError) ExitCode() int { return e.code }

func (r *projectReconcileRunner) Run(_ context.Context, args []string, _ []string, _ io.Reader, out, _ io.Writer) error {
	if slices.Contains(args, "authbroker.control") {
		state := "unlocked"
		if slices.Contains(args, "status") {
			state = "not_configured"
			provider := ""
			for index := 0; index+1 < len(args); index++ {
				if args[index] == "--provider" {
					provider = args[index+1]
					break
				}
			}
			_, _ = io.WriteString(out, `{"schema_version":1,"ok":true,"state":"`+state+`","provider":"`+provider+`"}`+"\n")
			return nil
		}
		_, _ = io.WriteString(out, `{"schema_version":1,"ok":true,"state":"`+state+`"}`+"\n")
	}
	return nil
}

func (r *projectReconcileRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{}, args...))
	if r.failOn != nil && r.failOn(args) {
		return []byte("injected failure"), errors.New("injected failure")
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("empty Docker argv")
	}
	switch args[0] {
	case "image":
		if len(args) > 3 && strings.Contains(args[3], ".Id") {
			if r.imageID != "" {
				return []byte(r.imageID + "\n"), nil
			}
			return []byte("sha256:compatible-image\n"), nil
		}
		if r.imageData != nil {
			return append([]byte{}, r.imageData...), nil
		}
		return compatibleImageInspection(), nil
	case "network":
		if len(args) > 1 && args[1] == "inspect" {
			if !r.networkExists {
				return nil, errors.New("No such network")
			}
			format := ""
			if len(args) > 3 {
				format = args[3]
			}
			switch {
			case strings.Contains(format, ".IPAM.Config"):
				return []byte(`[{"Subnet":"172.20.0.0/24"}]`), nil
			case strings.Contains(format, ownerLabel):
				return []byte(ownerValue + "\n"), nil
			case strings.Contains(format, projectIDLabel):
				return []byte(r.instanceID + "\n"), nil
			case strings.Contains(format, projectRoleLabel):
				return []byte(projectNetRole + "\n"), nil
			}
			return []byte("network-id\n"), nil
		}
		if len(args) > 1 && args[1] == "create" {
			r.networkExists = true
			return []byte("network-id\n"), nil
		}
		if len(args) > 1 && args[1] == "connect" {
			if r.gatewayNetworks == nil {
				r.gatewayNetworks = make(map[string]string)
			}
			if len(args) > 5 && args[2] == "--alias" && args[3] == "gateway" && args[5] == gatewayContainer {
				r.gatewayNetworks[args[4]] = "172.20.0.2"
			}
			return nil, nil
		}
		return nil, nil
	case "inspect":
		name := ""
		if len(args) > 0 {
			name = args[len(args)-1]
		}
		if len(args) > 2 && strings.Contains(args[2], ".State.Health") {
			return []byte(`{"state":"running","health":"healthy"}`), nil
		}
		if len(args) > 2 && args[2] == "{{.Image}}" && name == gatewayContainer {
			return []byte("sha256:" + strings.Repeat("b", 64) + "\n"), nil
		}
		if len(args) > 2 && strings.Contains(args[2], ".NetworkSettings.Networks") {
			if name == gatewayContainer {
				entries := make([]string, 0, len(r.gatewayNetworks))
				for network, ip := range r.gatewayNetworks {
					entries = append(entries, fmt.Sprintf("%q:{\"IPAddress\":%q}", network, ip))
				}
				return []byte("{" + strings.Join(entries, ",") + "}"), nil
			}
			for network := range r.gatewayNetworks {
				return []byte(fmt.Sprintf(`{%q:{"IPAddress":"172.20.0.3"}}`, network)), nil
			}
			return []byte(`{}`), nil
		}
		if len(args) > 2 && strings.Contains(args[2], ".HostConfig.Dns") {
			return []byte(`["172.20.0.2"]`), nil
		}
		if len(args) > 2 && strings.Contains(args[2], ".Mounts") && r.sourceRoot != "" && r.workspaceRoot != "" {
			return []byte(fmt.Sprintf(
				`[{"Type":"bind","Source":%q,"Destination":%q,"RW":true}]`,
				r.sourceRoot, r.workspaceRoot,
			)), nil
		}
		if len(args) > 2 && strings.Contains(args[2], projectSpecLabel) {
			if r.containerSpec != "" {
				return []byte(r.containerSpec + "\n"), nil
			}
			return []byte("sha256:current\n"), nil
		}
		if len(args) > 2 && strings.Contains(args[2], ownerLabel) {
			return []byte(ownerValue + "\n"), nil
		}
		if len(args) > 2 && strings.Contains(args[2], projectIDLabel) {
			return []byte(r.instanceID + "\n"), nil
		}
		if len(args) > 2 && strings.Contains(args[2], projectRoleLabel) {
			return []byte(projectWorkRole + "\n"), nil
		}
		if r.containerExists {
			return []byte("container-id\n"), nil
		}
		return nil, errors.New("No such object")
	case "rm":
		r.containerExists = false
		r.containerSpec = ""
		return nil, nil
	case "create":
		r.containerExists = true
		return []byte("container-id\n"), nil
	case "start":
		return nil, nil
	case "run":
		mode := args[len(args)-1]
		if len(args) >= 3 && args[len(args)-3] == "workspace" {
			mode = "workspace"
		}
		return []byte("tobari-network-guard v1 " + mode + "\n"), nil
	default:
		return nil, nil
	}
}

func setActiveContextImage(t *testing.T, runtime *Runtime, image string) {
	t.Helper()
	manifest, _, err := runtime.activeContext()
	if err != nil {
		t.Fatal(err)
	}
	manifest.Image = image
	if manifest.RuntimeBinding != nil {
		manifest.RuntimeBinding.Image = image
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicJSON(runtime.contextManifestPath(manifest.Name), manifest); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureProjectRuntimeReconcilesActiveContextImageForExistingWorkspace(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &projectReconcileRunner{
		imageID:         "sha256:new-runtime",
		networkExists:   true,
		containerExists: true,
		containerSpec:   "sha256:old-spec",
	}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{runtimeImage: "runtime-old:latest"}
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if instance.Image != "runtime-old:latest" {
		t.Fatalf("created image = %q, want runtime-old:latest", instance.Image)
	}
	runner.instanceID = instance.ID
	marker := filepath.Join(runtime.projectHomePath(instance.ID), "marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	setActiveContextImage(t, runtime, "runtime-new:latest")

	updated, err := runtime.EnsureProjectRuntime(context.Background(), runtimeState(root), instance)
	if err != nil {
		t.Fatalf("EnsureProjectRuntime() error = %v", err)
	}
	if updated.Image != "runtime-new:latest" {
		t.Fatalf("updated image = %q, want runtime-new:latest", updated.Image)
	}
	stored, found, err := runtime.ResolveProject(context.Background(), projectRoot)
	if err != nil || !found || stored.Image != "runtime-new:latest" {
		t.Fatalf("stored image after reconcile = (%+v, %t, %v)", stored, found, err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "preserve" {
		t.Fatalf("home marker after reconcile = %q, err=%v", data, err)
	}
	var removed, created bool
	for _, call := range runner.calls {
		if len(call) == 0 {
			continue
		}
		if !capabilityprofile.Compiled().IncludesExperimental() && slices.Contains(call, authBrokerContainer) {
			t.Fatalf("standard Workspace reconciliation inspected the Auth Broker: %v", call)
		}
		if call[0] == "rm" {
			removed = true
		}
		if call[0] == "create" {
			created = true
			if !containsArgs(call, "runtime-new:latest") {
				t.Fatalf("recreated container did not use active Context image: %v", call)
			}
			if containsArgs(call, "runtime-old:latest") {
				t.Fatalf("recreated container used old stored image: %v", call)
			}
		}
	}
	if !removed || !created {
		t.Fatalf("reconcile calls = %v, want drift removal and container creation", runner.calls)
	}
}

func TestEnsureProjectRuntimeCancellationBeforeDriftPreservesPrincipal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	inner := &projectReconcileRunner{
		networkExists:   true,
		containerExists: true,
		gatewayNetworks: map[string]string{},
	}
	runner := &interruptedProjectReconcileRunner{inner: inner, cancel: cancel}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	inner.instanceID = instance.ID
	_, network, err := tobari.ProjectResourceNames(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	inner.gatewayNetworks[network] = "172.20.0.2"
	inner.sourceRoot = instance.Root
	inner.workspaceRoot, err = runtime.projectContainerRoot(instance.Root)
	if err != nil {
		t.Fatal(err)
	}
	manifest := projectRuntimeContext(t, runtime, instance)
	image, err := runtime.resolveContextImageFor(context.Background(), manifest)
	if err != nil {
		t.Fatal(err)
	}
	image = runtime.resolveBuiltinImageSelector(image)
	profile, err := runtime.ensureSharedProfile(manifest.AgentProfile)
	if err != nil {
		t.Fatal(err)
	}
	desired := instance
	desired.Image = image
	authProjection, err := runtime.reconcileProjectAuth(context.Background(), desired)
	if err != nil {
		t.Fatal(err)
	}
	inner.containerSpec, err = runtime.projectSpecHashWithAuthAndSourceAccess(
		runtimeState(root), desired, profile, network, image, "sha256:compatible-image", authProjection, manifest.SourceAccess,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.updateProjectPrincipal(
		context.Background(), instance, network, "172.20.0.3", "172.20.0.2",
	); err != nil {
		t.Fatal(err)
	}
	sawPrincipal := false
	runner.onInterrupt = func() {
		registry, readErr := runtime.readProjectPrincipalRegistry()
		sawPrincipal = readErr == nil && len(registry.Bindings) == 1 && registry.Bindings[0].ProjectID == instance.ID
	}

	_, err = runtime.EnsureProjectRuntime(ctx, runtimeState(root), instance)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("EnsureProjectRuntime() error = %v, want cancellation", err)
	}
	if !sawPrincipal {
		t.Fatal("ordinary re-entry removed the existing principal before a Docker mutation was required")
	}
	registry, err := runtime.readProjectPrincipalRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Bindings) != 1 {
		t.Fatalf("principal registry after cancellation = %+v", registry)
	}
	got := registry.Bindings[0]
	if got.ProjectID != instance.ID || got.ContextID != instance.ContextID || got.ContextName != instance.ContextName ||
		got.ProjectRoot != instance.Root || got.Network != network || got.WorkspaceIP != "172.20.0.3" || got.GatewayIP != "172.20.0.2" {
		t.Fatalf("retained principal = %+v", got)
	}
	if !runner.interrupted {
		t.Fatal("cancellation fixture did not exercise the selected project runtime")
	}
	for _, call := range inner.calls {
		if len(call) > 0 && (call[0] == "rm" || call[0] == "create" || call[0] == "start") {
			t.Fatalf("ready re-entry mutated the project runtime before cancellation: %v", call)
		}
	}
}

func TestEnsureProjectRuntimeDriftClosesPrincipalBeforeDockerMutation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &projectReconcileRunner{
		networkExists:   true,
		containerExists: true,
		containerSpec:   "sha256:stale",
		gatewayNetworks: map[string]string{},
	}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	runner.instanceID = instance.ID
	_, network, err := tobari.ProjectResourceNames(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	runner.gatewayNetworks[network] = "172.20.0.2"
	if err := runtime.updateProjectPrincipal(
		context.Background(), instance, network, "172.20.0.3", "172.20.0.2",
	); err != nil {
		t.Fatal(err)
	}
	sawClosed := false
	runner.failOn = func(args []string) bool {
		if len(args) == 0 || args[0] != "rm" {
			return false
		}
		registry, readErr := runtime.readProjectPrincipalRegistry()
		sawClosed = readErr == nil && len(registry.Bindings) == 0
		return true
	}

	if _, err := runtime.EnsureProjectRuntime(context.Background(), runtimeState(root), instance); err == nil {
		t.Fatal("EnsureProjectRuntime() unexpectedly completed injected drift mutation")
	}
	if !sawClosed {
		t.Fatal("drifted runtime reached Docker mutation before its principal was removed")
	}
	registry, err := runtime.readProjectPrincipalRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Bindings) != 0 {
		t.Fatalf("failed drift reconciliation retained authority: %+v", registry.Bindings)
	}
}

func TestEnsureProjectRuntimeImageDriftFailurePreservesStoredImage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &projectReconcileRunner{
		failOn: func(args []string) bool {
			return len(args) > 0 && args[0] == "create"
		},
		imageID: "sha256:new-runtime",
	}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{runtimeImage: "runtime-old:latest"}
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	runner.instanceID = instance.ID
	setActiveContextImage(t, runtime, "runtime-new:latest")

	if _, err := runtime.EnsureProjectRuntime(context.Background(), runtimeState(root), instance); err == nil {
		t.Fatal("EnsureProjectRuntime() unexpectedly succeeded")
	}
	stored, found, err := runtime.ResolveProject(context.Background(), projectRoot)
	if err != nil || !found || stored.Image != instance.Image || stored.Runtime != (tobari.ProjectRuntime{}) {
		t.Fatalf("logical state after failed image reconcile = (%+v, %t, %v), want old image %q", stored, found, err, instance.Image)
	}
}

func TestEnsureProjectRuntimeRejectsIncompatibleImageBeforeProjectMutation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &projectReconcileRunner{
		imageData: []byte(`{"api":"0","user":"root","entrypoint":["/bin/sh"]}`),
	}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(runtime.projectHomePath(instance.ID), "marker")
	if err := os.WriteFile(marker, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.EnsureProjectRuntime(context.Background(), runtimeState(root), instance)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "incompatible_image" {
		t.Fatalf("EnsureProjectRuntime() error = %v, want incompatible_image", err)
	}
	for _, call := range runner.calls {
		if len(call) == 0 {
			continue
		}
		if call[0] == "network" || call[0] == "create" || call[0] == "start" {
			t.Fatalf("incompatible image reached project mutation: %v", runner.calls)
		}
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "preserve" {
		t.Fatalf("logical home marker after incompatible image = %q, err=%v", data, err)
	}
}

func TestEnsureProjectContainerRecreatesOnSpecDrift(t *testing.T) {
	t.Parallel()
	runner := &projectSpecDriftRunner{stale: true, instanceID: "01900000-0000-7000-8000-000000000001"}
	runtime, err := newRuntimeWithData(
		filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), filepath.Join(t.TempDir(), "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := tobari.ProjectInstance{
		SchemaVersion: tobari.ProjectStateSchemaVersion,
		ID:            runner.instanceID,
		Root:          filepath.Join(t.TempDir(), "project"),
		ContextID:     "01912345-6789-7abc-8def-0123456789ad",
		ContextName:   "default",
		Profile:       tobari.DefaultProfile,
		Image:         tobari.BuiltinImageSelector,
	}
	if err := os.MkdirAll(instance.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: filepath.Join(t.TempDir(), "runtime"),
		AggregateRevision: strings.Repeat("a", 64), ContextCount: 1,
		PolicyDirectory: filepath.Join(t.TempDir(), "policy"), GatewayConfig: filepath.Join(t.TempDir(), "gateway.json"),
		AssetVersion: "asset",
	}
	if err := runtime.ensureProjectContainer(context.Background(), state, instance, "/profile", "tobari-project", "tobari-network", "172.29.0.2", "tobari-image", "sha256:desired"); err != nil {
		t.Fatalf("ensureProjectContainer() error = %v", err)
	}
	var removed, created bool
	for _, call := range runner.calls {
		if len(call) > 0 && call[0] == "rm" {
			removed = true
		}
		if len(call) > 0 && call[0] == "create" {
			created = true
			if !strings.Contains(strings.Join(call, " "), projectSpecLabel+"=sha256:desired") {
				t.Fatalf("recreated container is missing desired spec hash: %v", call)
			}
		}
	}
	if !removed || !created {
		t.Fatalf("spec drift calls = %v, want rm followed by create", runner.calls)
	}
}

func TestEnsureProjectContainerAppliesSharedResourceBounds(t *testing.T) {
	t.Parallel()
	runner := &projectSpecDriftRunner{stale: true, instanceID: "01900000-0000-7000-8000-000000000002"}
	runtime, err := newRuntimeWithData(
		filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), filepath.Join(t.TempDir(), "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := tobari.ProjectInstance{
		SchemaVersion: tobari.ProjectStateSchemaVersion,
		ID:            runner.instanceID,
		Root:          filepath.Join(t.TempDir(), "project"),
		Profile:       tobari.DefaultProfile,
		Image:         tobari.BuiltinImageSelector,
	}
	if err := os.MkdirAll(instance.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureProjectContainer(
		context.Background(),
		runtimeState(filepath.Dir(instance.Root)),
		instance,
		"/profile",
		"tobari-project",
		"tobari-network",
		"172.29.0.2",
		"tobari-image",
		"sha256:desired",
	); err != nil {
		t.Fatalf("ensureProjectContainer() error = %v", err)
	}
	var create []string
	for _, call := range runner.calls {
		if len(call) > 0 && call[0] == "create" {
			create = call
			break
		}
	}
	if len(create) == 0 {
		t.Fatalf("project create call missing from %v", runner.calls)
	}
	for _, want := range [][]string{
		{"tobari-image", "sleep", "infinity"},
		{"--network", "tobari-network", "--dns", "172.29.0.2"},
		{"--cpus", "2.0"},
		{"--memory", "4g"},
		{"--memory-swap", "4g"},
		{"--pids-limit", "512"},
		{"--log-driver", "json-file"},
		{"--log-opt", "max-size=10m"},
		{"--log-opt", "max-file=3"},
	} {
		if !containsConsecutiveArgs(create, want...) {
			t.Errorf("project create args = %v, missing %v", create, want)
		}
	}
	joined := strings.Join(create, " ")
	for _, prohibited := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "NO_PROXY", "no_proxy", "gateway:8080"} {
		if strings.Contains(joined, prohibited) {
			t.Errorf("project create args contain prohibited proxy value %q: %v", prohibited, create)
		}
	}
}

func TestProjectSpecHashIncludesLifetimeCommand(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &recordingRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	profile, err := runtime.ensureSharedProfile(tobari.DefaultProfile)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	standard, err := runtime.projectSpecHashWithCommand(
		state, instance, profile, "tobari-network", "tobari-image", "sha256:image", []string{"sleep", "infinity"},
	)
	if err != nil {
		t.Fatal(err)
	}
	terminating, err := runtime.projectSpecHashWithCommand(
		state, instance, profile, "tobari-network", "tobari-image", "sha256:image", []string{"sh", "-c", "exit 23"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if standard == terminating {
		t.Fatal("project spec hash ignored the lifetime command")
	}
}

func containsConsecutiveArgs(args []string, expected ...string) bool {
	if len(expected) == 0 || len(expected) > len(args) {
		return false
	}
	for start := 0; start <= len(args)-len(expected); start++ {
		match := true
		for offset, value := range expected {
			if args[start+offset] != value {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestWaitProjectReadyDistinguishesTerminalFailures(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		state  string
		health string
		want   string
	}{
		"unhealthy": {state: "running", health: "unhealthy", want: "unhealthy"},
		"exited":    {state: "exited", health: "none", want: "exited"},
		"no health": {state: "running", health: "none", want: "no readiness healthcheck"},
	} {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			runtime, err := newRuntimeWithData(
				filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), filepath.Join(t.TempDir(), "data"),
				&projectReadinessRunner{state: test.state, health: test.health},
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.waitProjectReady(context.Background(), "tobari-project"); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("waitProjectReady() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestEnsureProjectRuntimeFaultsPreserveLogicalState(t *testing.T) {
	t.Parallel()
	stages := map[string]func([]string) bool{
		"network creation": func(args []string) bool {
			return len(args) > 1 && args[0] == "network" && args[1] == "create"
		},
		"Gateway connection": func(args []string) bool {
			return len(args) > 1 && args[0] == "network" && args[1] == "connect"
		},
		"container creation": func(args []string) bool {
			return len(args) > 0 && args[0] == "create"
		},
		"container start": func(args []string) bool {
			return len(args) > 0 && args[0] == "start"
		},
	}
	for name, failOn := range stages {
		name, failOn := name, failOn
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			runner := &projectReconcileRunner{failOn: failOn}
			runtime, err := newRuntimeWithData(
				filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
			)
			if err != nil {
				t.Fatal(err)
			}
			projectRoot := filepath.Join(root, "project")
			if err := os.MkdirAll(projectRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			instance, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.EnsureProjectRuntime(context.Background(), runtimeState(root), instance); err == nil {
				t.Fatal("EnsureProjectRuntime() unexpectedly succeeded")
			}
			stored, found, err := runtime.ResolveProject(context.Background(), projectRoot)
			if err != nil || !found || stored.ID != instance.ID || stored.Root != instance.Root {
				t.Fatalf("logical state after %s failure = (%+v, %t, %v)", name, stored, found, err)
			}
			if _, err := os.Stat(runtime.projectHomePath(instance.ID)); err != nil {
				t.Fatalf("project home after %s failure: %v", name, err)
			}
		})
	}
}

func TestEnsureProjectRuntimeStateWriteFailurePreservesLogicalState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &projectReconcileRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	runtime.projectStateWriter = func(tobari.ProjectInstance) error {
		return errors.New("injected state update failure")
	}
	if _, err := runtime.EnsureProjectRuntime(context.Background(), runtimeState(root), instance); err == nil {
		t.Fatal("EnsureProjectRuntime() unexpectedly succeeded")
	}
	stored, found, err := runtime.ResolveProject(context.Background(), projectRoot)
	if err != nil || !found || stored.ID != instance.ID || stored.Runtime != (tobari.ProjectRuntime{}) {
		t.Fatalf("logical state after state update failure = (%+v, %t, %v)", stored, found, err)
	}
}

func TestEnterProjectRuntimePreservesChildExitStatus(t *testing.T) {
	t.Parallel()
	runtimeRoot := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(runtimeRoot, "config"), filepath.Join(runtimeRoot, "state"), filepath.Join(runtimeRoot, "data"),
		&projectExitRunner{code: 37},
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	manifest := projectRuntimeContext(t, runtime, instance)
	code, err := runtime.EnterProjectRuntime(context.Background(), instance, manifest, instance.Root, nil, io.Discard, io.Discard)
	if err != nil || code != 37 {
		t.Fatalf("EnterProjectRuntime() = (%d, %v), want child status 37", code, err)
	}
}
