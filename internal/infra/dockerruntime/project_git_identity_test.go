package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type staticHostGitIdentityResolver struct {
	identity *projectGitIdentity
	err      error
	roots    []string
}

func (r *staticHostGitIdentityResolver) Resolve(_ context.Context, root string) (*projectGitIdentity, error) {
	r.roots = append(r.roots, root)
	return r.identity, r.err
}

func projectGitTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	base := canonicalTestDirectory(t)
	runtime, err := newRuntimeWithData(
		filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"),
		&projectGitContainerRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func projectGitTestInstance(t *testing.T, runtime *Runtime) tobari.ProjectInstance {
	t.Helper()
	root := canonicalTestDirectory(t)
	instance := tobari.ProjectInstance{
		SchemaVersion: tobari.ProjectStateSchemaVersion,
		ID:            "01900000-0000-7000-8000-0000000000a1",
		Root:          root,
		ContextID:     "01912345-6789-7abc-8def-0123456789ad",
		ContextName:   "default",
		Profile:       tobari.DefaultProfile,
		Image:         tobari.BuiltinImageSelector,
	}
	if err := instance.Validate(); err != nil {
		t.Fatal(err)
	}
	directory, err := runtime.projectDirectory(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return instance
}

func projectGitTestManifest(t *testing.T, setting *tobari.ContextGitIdentitySetting) tobari.ContextManifest {
	t.Helper()
	manifest := tobari.ContextManifest{
		SchemaVersion:  tobari.ContextSchemaVersion,
		ID:             "01912345-6789-7abc-8def-0123456789ad",
		Name:           "default",
		AgentProfile:   tobari.DefaultProfile,
		Image:          tobari.BuiltinImageSelector,
		PolicyMode:     tobari.ContextPolicyModeGuided,
		SourceAccess:   tobari.ContextSourceAccessReadWrite,
		PolicyRevision: tobari.DefaultContextPolicyRevision(),
		GitIdentity:    setting,
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func literalProjectGitSetting(name, email string) *tobari.ContextGitIdentitySetting {
	return &tobari.ContextGitIdentitySetting{
		Source: tobari.ContextGitIdentityLiteral,
		Name:   &name,
		Email:  &email,
	}
}

func TestEncodeProjectGitConfigQuotesValuesAndOnlyEmitsFixedKeys(t *testing.T) {
	t.Parallel()
	identity := &projectGitIdentity{
		Name:  `Tobari "User" \\ [include] path=/tmp/escape`,
		Email: `tobari+"quoted"\\path@example.com`,
	}
	encoded, err := encodeProjectGitConfig(identity)
	if err != nil {
		t.Fatal(err)
	}
	config := string(encoded)
	if strings.Count(config, "[include]\n") != 1 || strings.Count(config, "[user]\n") != 1 ||
		strings.Count(config, "\n\tname = ") != 1 || strings.Count(config, "\n\temail = ") != 1 {
		t.Fatalf("generated config has an unexpected directive shape:\n%s", config)
	}
	if !strings.HasPrefix(config, "[include]\n\tpath = \"/etc/gitconfig\"\n[user]\n") {
		t.Fatalf("generated config did not preserve system config first:\n%s", config)
	}
	if !strings.Contains(config, `name = "Tobari \"User\" \\\\ [include] path=/tmp/escape"`) ||
		!strings.Contains(config, `email = "tobari+\"quoted\"\\\\path@example.com"`) {
		t.Fatalf("generated config did not safely quote identity values:\n%s", config)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return
	}
	base := canonicalTestDirectory(t)
	path := filepath.Join(base, "projection.gitconfig")
	global := filepath.Join(base, "empty-global.gitconfig")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(global, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	environment := []string{"GIT_CONFIG_SYSTEM=" + path, "GIT_CONFIG_GLOBAL=" + global, "HOME=" + base}
	if got := runSyntheticGit(t, git, environment, "config", "--system", "--get", "user.name"); got != identity.Name {
		t.Fatalf("Git decoded generated name as %q, want %q", got, identity.Name)
	}
	if got := runSyntheticGit(t, git, environment, "config", "--system", "--get", "user.email"); got != identity.Email {
		t.Fatalf("Git decoded generated email as %q, want %q", got, identity.Email)
	}
}

func TestEncodeProjectGitConfigWithoutIdentityContainsNoUserFallback(t *testing.T) {
	t.Parallel()
	encoded, err := encodeProjectGitConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "[user]") || strings.Contains(string(encoded), "name") || strings.Contains(string(encoded), "email") {
		t.Fatalf("empty fallback contains user identity: %q", encoded)
	}
	if string(encoded) != "[include]\n\tpath = \"/etc/gitconfig\"\n" {
		t.Fatalf("empty fallback = %q", encoded)
	}
}

func TestWriteProjectGitConfigIsAtomicPrivateAndRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()
	t.Run("private regular file", func(t *testing.T) {
		runtime := projectGitTestRuntime(t)
		instance := projectGitTestInstance(t, runtime)
		content, err := encodeProjectGitConfig(&projectGitIdentity{Name: "Tobari User", Email: "tobari@example.com"})
		if err != nil {
			t.Fatal(err)
		}
		if err := runtime.writeProjectGitConfig(instance.ID, content); err != nil {
			t.Fatal(err)
		}
		directory, _ := runtime.projectGitDirectory(instance.ID)
		path, _ := runtime.projectGitConfigPath(instance.ID)
		directoryInfo, err := os.Lstat(directory)
		if err != nil || !directoryInfo.IsDir() || directoryInfo.Mode().Perm() != 0o700 {
			t.Fatalf("projection directory mode = %v, err=%v", directoryInfo, err)
		}
		fileInfo, err := os.Lstat(path)
		if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode().Perm() != 0o600 {
			t.Fatalf("projection file mode = %v, err=%v", fileInfo, err)
		}
		if observed, err := os.ReadFile(path); err != nil || !slices.Equal(observed, content) {
			t.Fatalf("projection content = %q, err=%v", observed, err)
		}
	})

	t.Run("reject oversized existing file before read or replacement", func(t *testing.T) {
		runtime := projectGitTestRuntime(t)
		instance := projectGitTestInstance(t, runtime)
		directory, _ := runtime.projectGitDirectory(instance.ID)
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		path, _ := runtime.projectGitConfigPath(instance.ID)
		oversized := []byte(strings.Repeat("x", maxProjectGitConfigBytes+1))
		if err := os.WriteFile(path, oversized, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := runtime.writeProjectGitConfig(instance.ID, []byte("replacement\n")); err == nil {
			t.Fatal("oversized managed file was accepted")
		}
		info, err := os.Stat(path)
		if err != nil || info.Size() != int64(len(oversized)) {
			t.Fatalf("oversized file changed: info=%v err=%v", info, err)
		}
	})

	for _, target := range []string{"directory", "file"} {
		target := target
		t.Run("reject "+target+" symlink", func(t *testing.T) {
			runtime := projectGitTestRuntime(t)
			instance := projectGitTestInstance(t, runtime)
			outside := filepath.Join(canonicalTestDirectory(t), "outside")
			if err := os.WriteFile(outside, []byte("preserve"), 0o600); err != nil {
				t.Fatal(err)
			}
			directory, _ := runtime.projectGitDirectory(instance.ID)
			if target == "directory" {
				if err := os.Symlink(filepath.Dir(outside), directory); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(directory, 0o700); err != nil {
					t.Fatal(err)
				}
				path, _ := runtime.projectGitConfigPath(instance.ID)
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			}
			if err := runtime.writeProjectGitConfig(instance.ID, []byte("fixed\n")); err == nil {
				t.Fatal("unsafe managed target was accepted")
			}
			if observed, err := os.ReadFile(outside); err != nil || string(observed) != "preserve" {
				t.Fatalf("outside file = %q, err=%v", observed, err)
			}
		})
	}
}

func TestReconcileProjectGitIdentityIncompleteInheritanceRemovesOnlyFallback(t *testing.T) {
	t.Parallel()
	runtime := projectGitTestRuntime(t)
	instance := projectGitTestInstance(t, runtime)
	if err := runtime.writeProjectGitConfig(instance.ID, []byte("[user]\n\tname = \"old\"\n")); err != nil {
		t.Fatal(err)
	}
	resolver := &staticHostGitIdentityResolver{}
	runtime.gitIdentity = resolver
	setting := &tobari.ContextGitIdentitySetting{Source: tobari.ContextGitIdentityInherit}
	if err := runtime.reconcileProjectGitIdentity(context.Background(), projectGitTestManifest(t, setting), instance); err != nil {
		t.Fatal(err)
	}
	path, _ := runtime.projectGitConfigPath(instance.ID)
	projected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(projected), "[user]") {
		t.Fatalf("incomplete inherited pair retained a fallback: %s", projected)
	}
	if !slices.Equal(resolver.roots, []string{instance.Root}) {
		t.Fatalf("resolver roots = %v, want stable Workspace root", resolver.roots)
	}
}

func TestReconcileProjectGitIdentityFailurePreservesPreviousProjection(t *testing.T) {
	t.Parallel()
	runtime := projectGitTestRuntime(t)
	instance := projectGitTestInstance(t, runtime)
	previous, err := encodeProjectGitConfig(&projectGitIdentity{Name: "Previous User", Email: "previous@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.writeProjectGitConfig(instance.ID, previous); err != nil {
		t.Fatal(err)
	}
	runtime.gitIdentity = &staticHostGitIdentityResolver{err: errors.New("private diagnostic")}
	setting := &tobari.ContextGitIdentitySetting{Source: tobari.ContextGitIdentityInherit}
	err = runtime.reconcileProjectGitIdentity(context.Background(), projectGitTestManifest(t, setting), instance)
	assertGitIdentityResolutionFault(t, err)
	path, _ := runtime.projectGitConfigPath(instance.ID)
	observed, readErr := os.ReadFile(path)
	if readErr != nil || !slices.Equal(observed, previous) {
		t.Fatalf("projection after failed resolution = %q, err=%v", observed, readErr)
	}
}

func TestReconcileProjectGitIdentityPreservesCancellation(t *testing.T) {
	t.Parallel()
	for name, canceled := range map[string]error{
		"canceled": context.Canceled,
		"deadline": context.DeadlineExceeded,
	} {
		name, canceled := name, canceled
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runtime := projectGitTestRuntime(t)
			instance := projectGitTestInstance(t, runtime)
			runtime.gitIdentity = &staticHostGitIdentityResolver{err: canceled}
			setting := &tobari.ContextGitIdentitySetting{Source: tobari.ContextGitIdentityInherit}
			err := runtime.reconcileProjectGitIdentity(context.Background(), projectGitTestManifest(t, setting), instance)
			if !errors.Is(err, canceled) {
				t.Fatalf("error = %v, want %v", err, canceled)
			}
			path, _ := runtime.projectGitConfigPath(instance.ID)
			if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("canceled resolution wrote a projection: %v", statErr)
			}
		})
	}
}

func TestEnsureProjectRuntimeStopsBeforeDockerWhenInheritedGitResolutionFails(t *testing.T) {
	t.Parallel()
	base := canonicalTestDirectory(t)
	runner := &projectGitContainerRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	previous, err := encodeProjectGitConfig(&projectGitIdentity{Name: "Previous User", Email: "previous@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.writeProjectGitConfig(instance.ID, previous); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := runtime.contextByID(instance.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	manifest.GitIdentity = &tobari.ContextGitIdentitySetting{Source: tobari.ContextGitIdentityInherit}
	if err := writeAtomicJSON(runtime.contextManifestPath(manifest.Name), manifest); err != nil {
		t.Fatal(err)
	}
	runtime.gitIdentity = &staticHostGitIdentityResolver{err: errors.New("private diagnostic")}
	runner.calls = nil
	_, err = runtime.EnsureProjectRuntime(context.Background(), runtimeState(base), instance)
	assertGitIdentityResolutionFault(t, err)
	if len(runner.calls) != 0 {
		t.Fatalf("failed Git resolution reached Docker: %v", runner.calls)
	}
	path, _ := runtime.projectGitConfigPath(instance.ID)
	observed, readErr := os.ReadFile(path)
	if readErr != nil || !slices.Equal(observed, previous) {
		t.Fatalf("projection after failed Ensure = %q, err=%v", observed, readErr)
	}
}

func TestProjectGitFallbackPrecedenceWithGit(t *testing.T) {
	t.Parallel()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is not installed")
	}
	base := canonicalTestDirectory(t)
	repository := filepath.Join(base, "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	runSyntheticGit(t, git, nil, "init", "--quiet", repository)
	projection := filepath.Join(base, "system.gitconfig")
	encoded, err := encodeProjectGitConfig(&projectGitIdentity{Name: "Context User", Email: "context@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(projection, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	global := filepath.Join(base, "workspace.gitconfig")
	environment := []string{"GIT_CONFIG_SYSTEM=" + projection, "GIT_CONFIG_GLOBAL=" + global, "HOME=" + base}
	runSyntheticGit(t, git, environment, "config", "--file", global, "user.name", "Workspace User")
	if got := runSyntheticGit(t, git, environment, "-C", repository, "config", "user.name"); got != "Workspace User" {
		t.Fatalf("Workspace global precedence = %q", got)
	}
	runSyntheticGit(t, git, environment, "-C", repository, "config", "user.name", "Repository User")
	if got := runSyntheticGit(t, git, environment, "-C", repository, "config", "user.name"); got != "Repository User" {
		t.Fatalf("repository-local precedence = %q", got)
	}
	runSyntheticGit(t, git, environment, "-C", repository, "config", "--unset", "user.name")
	if err := os.Remove(global); err != nil {
		t.Fatal(err)
	}
	if got := runSyntheticGit(t, git, environment, "-C", repository, "config", "user.name"); got != "Context User" {
		t.Fatalf("Context fallback = %q", got)
	}
}

type projectGitContainerRunner struct {
	calls       [][]string
	created     bool
	mountOutput []byte
}

func (*projectGitContainerRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *projectGitContainerRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	if len(args) == 0 {
		return nil, errors.New("empty Docker argv")
	}
	if args[0] == "inspect" && len(args) > 2 && args[2] == "{{.Id}}" && !r.created {
		return nil, errors.New("no such container")
	}
	if args[0] == "create" {
		r.created = true
		return nil, nil
	}
	if args[0] == "inspect" && strings.Contains(strings.Join(args, " "), ".State.Health") {
		return []byte(`{"state":"running","health":"healthy"}`), nil
	}
	if args[0] == "inspect" && strings.Contains(strings.Join(args, " "), ".Mounts") {
		return append([]byte(nil), r.mountOutput...), nil
	}
	return nil, nil
}

func TestProjectContainerInspectsExactSourceBindAccess(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		rw     bool
		access tobari.ContextSourceAccess
	}{
		{name: "read write", rw: true, access: tobari.ContextSourceAccessReadWrite},
		{name: "read only", rw: false, access: tobari.ContextSourceAccessReadOnly},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			base := canonicalTestDirectory(t)
			root := filepath.Join(base, "project")
			workspaceRoot := "/workspace/project"
			mounts := fmt.Sprintf(
				`[{"Type":"bind","Source":%q,"Destination":%q,"RW":%t},{"Type":"bind","Source":%q,"Destination":"/var/lib/tobari","RW":true}]`,
				root, workspaceRoot, test.rw, filepath.Join(base, "home"),
			)
			runner := &projectGitContainerRunner{mountOutput: []byte(mounts)}
			runtime, err := newRuntimeWithData(
				filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner,
			)
			if err != nil {
				t.Fatal(err)
			}
			access, err := runtime.projectContainerSourceAccess(context.Background(), "project", root, workspaceRoot)
			if err != nil || access != test.access {
				t.Fatalf("source access = %q, error = %v, want %q", access, err, test.access)
			}
		})
	}

	base := canonicalTestDirectory(t)
	root := filepath.Join(base, "project")
	runner := &projectGitContainerRunner{mountOutput: []byte(fmt.Sprintf(
		`[{"Type":"bind","Source":%q,"Destination":"/workspace/project","RW":false},{"Type":"bind","Source":%q,"Destination":"/workspace/alias","RW":true}]`,
		root, root,
	))}
	runtime, err := newRuntimeWithData(
		filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.projectContainerSourceAccess(context.Background(), "project", root, "/workspace/project"); err == nil ||
		!strings.Contains(err.Error(), "writable alias") {
		t.Fatalf("writable source alias error = %v", err)
	}
}

func TestProjectContainerMountsGitFallbackAtSystemScope(t *testing.T) {
	t.Parallel()
	base := canonicalTestDirectory(t)
	runner := &projectGitContainerRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectGitTestInstance(t, runtime)
	if err := runtime.ensureProjectContainer(
		context.Background(), runtimeState(base), instance, "/profile", "project", "network", "172.29.0.2", "image", "sha256:git",
	); err != nil {
		t.Fatal(err)
	}
	var create []string
	for _, call := range runner.calls {
		if len(call) != 0 && call[0] == "create" {
			create = call
			break
		}
	}
	if len(create) == 0 {
		t.Fatalf("Docker create call missing: %v", runner.calls)
	}
	directory, _ := runtime.projectGitDirectory(instance.ID)
	if !containsConsecutiveArgs(create, "--env", "GIT_CONFIG_SYSTEM="+projectGitContainerConfig) {
		t.Fatalf("create args are missing system Git config environment: %v", create)
	}
	wantMount := "type=bind,src=" + directory + ",dst=" + projectGitContainerDirectory + ",readonly"
	if !containsConsecutiveArgs(create, "--mount", wantMount) {
		t.Fatalf("create args are missing read-only Git projection mount: %v", create)
	}
}

func TestProjectContainerSelectsOnlyDirectSourceBindAccess(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		access     tobari.ContextSourceAccess
		wantSuffix string
	}{
		{name: "read write", access: tobari.ContextSourceAccessReadWrite},
		{name: "read only", access: tobari.ContextSourceAccessReadOnly, wantSuffix: ",readonly"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			base := canonicalTestDirectory(t)
			runner := &projectGitContainerRunner{}
			runtime, err := newRuntimeWithData(
				filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner,
			)
			if err != nil {
				t.Fatal(err)
			}
			instance := projectGitTestInstance(t, runtime)
			state := runtimeState(base)
			if err := runtime.ensureProjectContainerWithAuth(
				context.Background(), state, instance, "/profile", "project", "network",
				"172.29.0.2", "image", "sha256:source", projectAuthProjection{Environment: []string{}, Files: []projectAuthFile{}}, test.access,
			); err != nil {
				t.Fatal(err)
			}
			var create []string
			for _, call := range runner.calls {
				if len(call) > 0 && call[0] == "create" {
					create = call
					break
				}
			}
			workspaceRoot, _ := runtime.projectContainerRoot(instance.Root)
			wantSource := "type=bind,src=" + instance.Root + ",dst=" + workspaceRoot + test.wantSuffix
			if !containsConsecutiveArgs(create, "--mount", wantSource) {
				t.Fatalf("source mount = %v, want %q", create, wantSource)
			}
			if !containsConsecutiveArgs(create, "--mount", "type=bind,src="+runtime.projectHomePath(instance.ID)+",dst=/var/lib/tobari") {
				t.Fatalf("Workspace home is not a separate writable bind: %v", create)
			}
			if !containsConsecutiveArgs(create, "--tmpfs", "/tmp:size=512m,mode=1777") {
				t.Fatalf("Workspace tmpfs is missing: %v", create)
			}
			opener := filepath.Join(state.RuntimeDirectory, "browser", "tobari-open")
			for _, mount := range []string{
				"type=bind,src=" + opener + ",dst=/run/tobari-open,readonly",
				"type=bind,src=" + opener + ",dst=/usr/local/bin/xdg-open,readonly",
			} {
				if !containsConsecutiveArgs(create, "--mount", mount) {
					t.Fatalf("Workspace browser mount %q is missing: %v", mount, create)
				}
			}
		})
	}
}

func TestProjectRuntimeSpecIncludesGitFallbackEnvironmentAndMount(t *testing.T) {
	t.Parallel()
	runtime := projectGitTestRuntime(t)
	instance := projectGitTestInstance(t, runtime)
	profile, err := runtime.ensureSharedProfile(tobari.DefaultProfile)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(filepath.Dir(instance.Root))
	spec, err := runtime.projectRuntimeSpecWithAuthAndCommand(
		state, instance, profile, "network", "image", "sha256:image",
		projectAuthProjection{Environment: []string{}, Files: []projectAuthFile{}}, projectLifetimeCommand(),
		tobari.ContextSourceAccessReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(spec.Environment, "GIT_CONFIG_SYSTEM="+projectGitContainerConfig) {
		t.Fatalf("project spec environment = %v", spec.Environment)
	}
	directory, _ := runtime.projectGitDirectory(instance.ID)
	wantMount := "bind:" + directory + "->" + projectGitContainerDirectory + ":ro"
	if !slices.Contains(spec.Mounts, wantMount) {
		t.Fatalf("project spec mounts = %v, want %q", spec.Mounts, wantMount)
	}
	opener := filepath.Join(state.RuntimeDirectory, "browser", "tobari-open")
	for _, mount := range []string{
		"bind:" + opener + "->/run/tobari-open:ro",
		"bind:" + opener + "->/usr/local/bin/xdg-open:ro",
	} {
		if !slices.Contains(spec.Mounts, mount) {
			t.Fatalf("project spec mounts = %v, want %q", spec.Mounts, mount)
		}
	}
}

func TestProjectRuntimeSpecHashBindsSourceAccess(t *testing.T) {
	t.Parallel()
	runtime := projectGitTestRuntime(t)
	instance := projectGitTestInstance(t, runtime)
	profile, err := runtime.ensureSharedProfile(tobari.DefaultProfile)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(filepath.Dir(instance.Root))
	auth := projectAuthProjection{Environment: []string{}, Files: []projectAuthFile{}}
	readWrite, err := runtime.projectSpecHashWithAuthAndSourceAccess(
		state, instance, profile, "network", "image", "sha256:image", auth, tobari.ContextSourceAccessReadWrite,
	)
	if err != nil {
		t.Fatal(err)
	}
	readOnly, err := runtime.projectSpecHashWithAuthAndSourceAccess(
		state, instance, profile, "network", "image", "sha256:image", auth, tobari.ContextSourceAccessReadOnly,
	)
	if err != nil {
		t.Fatal(err)
	}
	if readOnly == readWrite {
		t.Fatalf("source access did not change project spec hash: %q", readOnly)
	}
}

func runSyntheticGit(t *testing.T, executable string, extraEnvironment []string, args ...string) string {
	t.Helper()
	command := exec.Command(executable, args...) // #nosec G204 -- test invokes a resolved Git executable with synthetic fixed arguments.
	environment := slices.DeleteFunc(os.Environ(), func(value string) bool { return strings.HasPrefix(value, "GIT_") })
	command.Env = append(environment, extraEnvironment...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}
