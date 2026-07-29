package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/realm"
)

func TestResolveRuntimeHomesPrefersXDGWithoutResolvingUserHome(t *testing.T) {
	t.Parallel()
	configHome := filepath.Join(string(filepath.Separator), "xdg", "config")
	stateHome := filepath.Join(string(filepath.Separator), "xdg", "state")
	gotConfig, gotState, err := resolveRuntimeHomes(configHome, stateHome, func() (string, error) {
		return "", errors.New("user home must not be resolved")
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotConfig != configHome || gotState != stateHome {
		t.Fatalf("resolveRuntimeHomes() = (%q, %q), want (%q, %q)", gotConfig, gotState, configHome, stateHome)
	}
}

func TestResolveRuntimeHomesUsesXDGFallbacksOnEveryPlatform(t *testing.T) {
	t.Parallel()
	home := filepath.Join(string(filepath.Separator), "home", "example")
	configHome, stateHome, err := resolveRuntimeHomes("", "", func() (string, error) {
		return home, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config"); configHome != want {
		t.Fatalf("config home = %q, want %q", configHome, want)
	}
	if want := filepath.Join(home, ".local", "state"); stateHome != want {
		t.Fatalf("state home = %q, want %q", stateHome, want)
	}
}

func TestResolveRuntimeHomesRequiresUserHomeForFallback(t *testing.T) {
	t.Parallel()
	_, _, err := resolveRuntimeHomes("", "/xdg/state", func() (string, error) {
		return "", errors.New("missing home")
	})
	if err == nil || !strings.Contains(err.Error(), "resolve user home directory") {
		t.Fatalf("resolveRuntimeHomes() error = %v, want user home resolution error", err)
	}
}

type runnerCall struct {
	args []string
}

type recordingRunner struct {
	runs    []runnerCall
	outputs []runnerCall
}

func (r *recordingRunner) Run(
	_ context.Context,
	args, _ []string,
	_ io.Reader,
	_, _ io.Writer,
) error {
	r.runs = append(r.runs, runnerCall{args: append([]string(nil), args...)})
	return nil
}

func (r *recordingRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string(nil), args...)})
	return []byte(""), nil
}

func runtimeState(root string) realm.State {
	return realm.State{
		SchemaVersion:    1,
		Root:             root,
		RuntimeDirectory: filepath.Join(root, "runtime"),
		PolicyDirectory:  filepath.Join(root, "policy"),
		CredentialConfig: filepath.Join(root, "credentials.json"),
		CredentialDir:    filepath.Join(root, "credentials"),
		AssetVersion:     "asset",
		ProxyEndpoint:    "http://gateway:8080",
	}
}

func TestLoadStateRejectsTrailingDocument(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(runtime.statePath()), 0o700); err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("\n{}\n")...)
	if err := os.WriteFile(runtime.statePath(), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtime.LoadState(context.Background()); err == nil {
		t.Fatal("state with trailing JSON was accepted")
	}
}

func TestExecMapsCWDAndPreservesExactArgv(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	subdirectory := filepath.Join(root, "repository")
	if err := os.Mkdir(subdirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	code, err := runtime.Exec(
		context.Background(),
		state,
		realm.ExecRequest{
			HostCWD: subdirectory, CWDExplicit: true,
			Command: []string{"printf", "%s", "a value"}, Interactive: true,
		},
		bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err != nil || code != 0 {
		t.Fatalf("Exec() code = %d, error = %v", code, err)
	}
	want := []string{
		"exec", "-i", "--user", "tobari", "--workdir", "/workspace/repository",
		"tobari-realm", "printf", "%s", "a value",
	}
	if len(runner.runs) != 1 || !slices.Equal(runner.runs[0].args, want) {
		t.Fatalf("docker argv = %v, want %v", runner.runs, want)
	}
}

func TestExecFallsBackForImplicitOutsideCWDButRejectsExplicitOne(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	parent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "root")
	outside := filepath.Join(parent, "outside")
	for _, path := range []string{root, outside} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(parent, "config"), filepath.Join(parent, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	_, err = runtime.Exec(
		context.Background(), state,
		realm.ExecRequest{HostCWD: outside, Command: []string{"pwd"}},
		bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(runner.runs[0].args, "/workspace") {
		t.Fatalf("implicit outside cwd did not fall back: %v", runner.runs[0].args)
	}
	_, err = runtime.Exec(
		context.Background(), state,
		realm.ExecRequest{HostCWD: outside, CWDExplicit: true, Command: []string{"pwd"}},
		bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err == nil {
		t.Fatal("explicit outside cwd was accepted")
	}
	if len(runner.runs) != 1 {
		t.Fatalf("Docker was called after explicit cwd rejection: %d", len(runner.runs))
	}
}

func TestComposeEnvironmentUsesPinnedEmbeddedImages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := runtime.composeEnvironment(runtimeState(root))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(environment, "\n")
	for _, key := range []string{"TOBARI_MITMPROXY_IMAGE=", "TOBARI_OPA_IMAGE=", "TOBARI_DEBIAN_IMAGE="} {
		index := strings.LastIndex(joined, key)
		if index < 0 || !strings.Contains(joined[index:], "@sha256:") {
			t.Fatalf("%s is not digest pinned", key)
		}
	}
}

func TestPolicyValidationRunsAsPolicyOwner(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.testPolicy(context.Background(), runtimeState(root)); err != nil {
		t.Fatal(err)
	}
	uid, gid := currentIDs()
	wantUser := strconv.Itoa(uid) + ":" + strconv.Itoa(gid)
	if len(runner.outputs) != 1 {
		t.Fatalf("policy validation calls = %v, want one call", runner.outputs)
	}
	userIndex := slices.Index(runner.outputs[0].args, "--user")
	if userIndex < 0 || userIndex+1 >= len(runner.outputs[0].args) ||
		runner.outputs[0].args[userIndex+1] != wantUser {
		t.Fatalf("policy validation argv = %v, want --user %s", runner.outputs, wantUser)
	}
}

func TestCredentialConfigValidationRejectsPathEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	config := filepath.Join(root, "config")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{
		"version":"v1",
		"profiles":{
			"escaped":{
				"type":"bearer",
				"hosts":["api.example.com"],
				"secret_file":"/run/tobari/credentials/../credentials.json"
			}
		}
	}`)
	if err := os.WriteFile(filepath.Join(config, "credentials.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	_, status := runtime.checkCredentialConfig()
	if status != doctor.CheckStatusFail {
		t.Fatalf("credential config status = %q, want fail", status)
	}
}

func TestPrepareStateKeepsPolicyAndCredentialsPrivate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	state, err := runtime.prepareState(root)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{
		state.PolicyDirectory:                               0o700,
		filepath.Join(state.PolicyDirectory, "tobari.rego"): 0o600,
		state.CredentialDir:                                 0o700,
		state.CredentialConfig:                              0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s mode = %o, want %o", path, got, want)
		}
	}
}
