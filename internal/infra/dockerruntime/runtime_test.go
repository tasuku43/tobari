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
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runnerCall struct{ args []string }
type recordingRunner struct {
	runs       []runnerCall
	outputs    []runnerCall
	outputData []byte
	outputErr  error
}

func (r *recordingRunner) Run(_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	return nil
}
func (r *recordingRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	return append([]byte{}, r.outputData...), r.outputErr
}

type gatewayNetworkRunner struct {
	outputs  []runnerCall
	networks string
}

func (r *gatewayNetworkRunner) Run(
	context.Context, []string, []string, io.Reader, io.Writer, io.Writer,
) error {
	return nil
}

func (r *gatewayNetworkRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) > 0 && args[0] == "inspect" {
		return []byte(r.networks), nil
	}
	return []byte{}, nil
}

func runtimeState(root string) tobari.State {
	return tobari.State{
		SchemaVersion: 2, RuntimeDirectory: filepath.Join(root, "runtime"),
		PolicyDirectory:  filepath.Join(root, "policy"),
		CredentialConfig: filepath.Join(root, "credentials.json"),
		CredentialDir:    filepath.Join(root, "credentials"), AssetVersion: "asset",
		ProxyEndpoint: "http://gateway:8080", Tobari: []tobari.Instance{},
	}
}

func runtimeInstance(root string) tobari.Instance {
	return tobari.Instance{
		ID: "tbr_0123456789abcdef0123456789abcdef", Name: "work", Root: root,
		Container: "tobari-work", Network: "tobari-work-net", HomeVolume: "tobari-work-home",
	}
}

func TestResolveRuntimeHomesUsesXDGAndPortableFallbacks(t *testing.T) {
	t.Parallel()
	config := filepath.Join(string(filepath.Separator), "xdg", "config")
	state := filepath.Join(string(filepath.Separator), "xdg", "state")
	gotConfig, gotState, err := resolveRuntimeHomes(config, state, func() (string, error) {
		return "", errors.New("must not resolve home")
	})
	if err != nil || gotConfig != config || gotState != state {
		t.Fatalf("resolved (%q,%q,%v)", gotConfig, gotState, err)
	}
	home := filepath.Join(string(filepath.Separator), "home", "example")
	gotConfig, gotState, err = resolveRuntimeHomes("", "", func() (string, error) { return home, nil })
	if err != nil || gotConfig != filepath.Join(home, ".config") || gotState != filepath.Join(home, ".local", "state") {
		t.Fatalf("fallback (%q,%q,%v)", gotConfig, gotState, err)
	}
}

func TestLoadStateRejectsLegacyAndTrailingDocuments(t *testing.T) {
	t.Parallel()
	for name, data := range map[string][]byte{
		"legacy":   []byte(`{"schema_version":1}`),
		"trailing": append(mustJSON(t, runtimeState(t.TempDir())), []byte("\n{}\n")...),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
			if err := os.MkdirAll(filepath.Dir(runtime.statePath()), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(runtime.statePath(), data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := runtime.LoadState(context.Background()); err == nil {
				t.Fatal("invalid state was accepted")
			}
		})
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExecMapsCWDAndPreservesExactArgv(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	subdirectory := filepath.Join(root, "repository")
	if err := os.Mkdir(subdirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	instance := runtimeInstance(root)
	code, err := runtime.Exec(
		context.Background(), instance,
		tobari.ExecRequest{HostCWD: subdirectory, CWDExplicit: true, Command: []string{"printf", "%s", "a value"}},
		bytes.NewReader(nil), io.Discard, io.Discard,
	)
	uid, gid := currentIDs()
	want := []string{
		"exec", "-i", "--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid), "--workdir", "/workspace/repository",
		"tobari-work", "printf", "%s", "a value",
	}
	if err != nil || code != 0 || len(runner.runs) != 1 || !slices.Equal(runner.runs[0].args, want) {
		t.Fatalf("Exec() code=%d err=%v calls=%v want=%v", code, err, runner.runs, want)
	}
}

func TestPolicyValidationUsesReadOnlyMountAndPolicyOwner(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err := runtime.testPolicy(context.Background(), runtimeState(root)); err != nil {
		t.Fatal(err)
	}
	args := runner.outputs[0].args
	uid, gid := currentIDs()
	wantUser := strconv.Itoa(uid) + ":" + strconv.Itoa(gid)
	if !slices.Contains(args, wantUser) || !slices.Contains(args, "type=bind,src="+filepath.Join(root, "policy")+",dst=/policy,readonly") {
		t.Fatalf("policy argv = %v", args)
	}
}

func TestPrepareStateUsesXDGPolicyAndEmptySchemaTwoCollection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	state, err := runtime.prepareState()
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 2 || state.Tobari == nil || len(state.Tobari) != 0 {
		t.Fatalf("state = %+v", state)
	}
	for path, want := range map[string]os.FileMode{
		state.PolicyDirectory: 0o700, filepath.Join(state.PolicyDirectory, "tobari.rego"): 0o600,
		state.CredentialDir: 0o700, state.CredentialConfig: 0o600,
		filepath.Join(root, "config", "config.json"): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s mode=%v err=%v want=%o", path, info.Mode().Perm(), err, want)
		}
	}
}

func TestResolveImageSelectorUsesExplicitThenXDGDefaultThenBuiltin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	config := filepath.Join(root, "config")
	runtime, _ := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
	got, err := runtime.ResolveImageSelector(context.Background(), "")
	if err != nil || got != tobari.BuiltinImageSelector {
		t.Fatalf("missing config resolved %q, %v", got, err)
	}
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(config, "config.json"),
		[]byte("{\"version\":\"v1\",\"default_image\":\"workbench:dev\"}\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	got, err = runtime.ResolveImageSelector(context.Background(), "")
	if err != nil || got != "workbench:dev" {
		t.Fatalf("configured default resolved %q, %v", got, err)
	}
	got, err = runtime.ResolveImageSelector(context.Background(), "explicit:dev")
	if err != nil || got != "explicit:dev" {
		t.Fatalf("explicit selector resolved %q, %v", got, err)
	}
}

func TestResolveImageSelectorRejectsUnsafeOrMalformedConfig(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		data []byte
		mode os.FileMode
	}{
		"unknown field": {data: []byte(`{"version":"v1","default_image":"builtin","extra":true}`), mode: 0o600},
		"invalid image": {data: []byte(`{"version":"v1","default_image":"--pull=always"}`), mode: 0o600},
		"unsafe mode":   {data: []byte(`{"version":"v1","default_image":"builtin"}`), mode: 0o644},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			config := filepath.Join(root, "config")
			if err := os.MkdirAll(config, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config, "config.json"), test.data, test.mode); err != nil {
				t.Fatal(err)
			}
			runtime, _ := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
			if _, err := runtime.ResolveImageSelector(context.Background(), ""); err == nil {
				t.Fatal("invalid config was accepted")
			}
		})
	}
}

func TestCredentialConfigValidationRejectsPathEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	config := filepath.Join(root, "config")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"version":"v1","profiles":{"escaped":{"type":"bearer","hosts":["api.example.com"],"secret_file":"/run/tobari/credentials/../credentials.json"}}}`)
	if err := os.WriteFile(filepath.Join(config, "credentials.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, _ := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
	_, status := runtime.checkCredentialConfig()
	if status != doctor.CheckStatusFail {
		t.Fatalf("status = %q", status)
	}
}

func TestComposeEnvironmentUsesPinnedImages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
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

func TestBuildTobariImageTagsVersionAndStableExtensionBase(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	state := runtimeState(root)
	if err := runtime.buildTobariImage(context.Background(), state, os.Environ()); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 1 {
		t.Fatalf("build calls = %v", runner.runs)
	}
	args := runner.runs[0].args
	for _, tag := range []string{tobariImage(state), "tobari-runtime:local"} {
		found := false
		for index := 0; index+1 < len(args); index++ {
			if args[index] == "--tag" && args[index+1] == tag {
				found = true
			}
		}
		if !found {
			t.Errorf("build args %v lack tag %q", args, tag)
		}
	}
}

func TestAttachRejectsMissingImageBeforeCreatingResources(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	runner := &recordingRunner{outputErr: errors.New("No such image")}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	_, err := runtime.Attach(context.Background(), runtimeState(root), "work", root, "workbench:dev")
	if err == nil {
		t.Fatal("missing image was accepted")
	}
	if len(runner.outputs) != 1 || len(runner.outputs[0].args) < 2 ||
		runner.outputs[0].args[0] != "image" || runner.outputs[0].args[1] != "inspect" {
		t.Fatalf("Docker calls = %v", runner.outputs)
	}
}

func TestValidateCompatibleImageRequiresRuntimeAPILabel(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		configuration string
		wantErr       bool
	}{
		"compatible": {
			configuration: `{"api":"1","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
		},
		"unlabeled": {
			configuration: `{"api":"","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
			wantErr:       true,
		},
		"overridden entrypoint": {
			configuration: `{"api":"1","user":"tobari","entrypoint":["/bin/sh"]}`,
			wantErr:       true,
		},
		"overridden user": {
			configuration: `{"api":"1","user":"root","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
			wantErr:       true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runner := &recordingRunner{outputData: []byte(test.configuration)}
			runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			err := runtime.validateCompatibleImage(context.Background(), "workbench:dev")
			if (err != nil) != test.wantErr {
				t.Fatalf("validateCompatibleImage() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestEnsureGatewayNetworkReconnectsAfterComposeReplacement(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		networks  string
		wantCalls int
	}{
		"already connected": {networks: `{"tobari-work-net":{}}`, wantCalls: 1},
		"reconnect":         {networks: `{"tobari-control":{}}`, wantCalls: 2},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runner := &gatewayNetworkRunner{networks: test.networks}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.ensureGatewayNetwork(context.Background(), "tobari-work-net"); err != nil {
				t.Fatal(err)
			}
			if len(runner.outputs) != test.wantCalls {
				t.Fatalf("Docker calls = %v, want %d", runner.outputs, test.wantCalls)
			}
			if test.wantCalls == 2 {
				want := []string{"network", "connect", "--alias", "gateway", "tobari-work-net", gatewayContainer}
				if !slices.Equal(runner.outputs[1].args, want) {
					t.Fatalf("reconnect argv = %v, want %v", runner.outputs[1].args, want)
				}
			}
		})
	}
}
