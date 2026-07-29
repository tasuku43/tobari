package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/realm"
)

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
		ProxyEndpoint:    "http://tobari-gateway:8080",
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
