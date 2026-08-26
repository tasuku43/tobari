package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/capabilitysurface"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalContextRuntimeRunner struct {
	observations []string
	calls        [][]string
	err          error
}

func (r *finalContextRuntimeRunner) Run(_ context.Context, args, _ []string, _ io.Reader, stdout, _ io.Writer) error {
	r.calls = append(r.calls, append([]string{}, args...))
	if r.err != nil {
		return r.err
	}
	if len(args) < 5 || args[0] != "image" || args[1] != "inspect" || len(r.observations) == 0 {
		return fmt.Errorf("unexpected final Context Runtime observation: %v", args)
	}
	observation := r.observations[0]
	r.observations = r.observations[1:]
	_, err := io.WriteString(stdout, observation)
	return err
}

func (*finalContextRuntimeRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, errors.New("unexpected Output observation")
}

func finalStandardRuntimeObservation(id, api, lifetime, user string, entrypoint string) string {
	return fmt.Sprintf(`{"id":%q,"api":%q,"lifetime":%q,"user":%q,"entrypoint":%s}`,
		id, api, lifetime, user, entrypoint)
}

func TestFinalContextLoginRuntimeUsesExactImmutableStandardMaterial(t *testing.T) {
	imageA := "sha256:" + strings.Repeat("a", 64)
	imageB := "sha256:" + strings.Repeat("b", 64)
	exact := finalStandardRuntimeObservation(imageA, tobari.RuntimeImageAPI, tobari.RuntimeImageLifetimeCommand, "tobari", `["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]`)
	for _, test := range []struct {
		name         string
		mutate       func(tobari.RuntimeBinding) tobari.RuntimeBinding
		observations []string
		runnerErr    error
		wantCalls    int
	}{
		{name: "exact immutable image id", observations: []string{exact, exact}, wantCalls: 2},
		{name: "persisted selector tamper", mutate: func(binding tobari.RuntimeBinding) tobari.RuntimeBinding {
			binding.Image = "example.com/runtime:mutable"
			return binding
		}},
		{name: "persisted revision tamper", mutate: func(binding tobari.RuntimeBinding) tobari.RuntimeBinding {
			binding.Revision = "sha256:" + strings.Repeat("f", 64)
			return binding
		}},
		{name: "canonical tag reassignment", observations: []string{exact, finalStandardRuntimeObservation(imageB, tobari.RuntimeImageAPI, tobari.RuntimeImageLifetimeCommand, "tobari", `["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]`)}, wantCalls: 2},
		{name: "missing image", runnerErr: errors.New("No such image"), wantCalls: 1},
		{name: "wrong API label", observations: []string{finalStandardRuntimeObservation(imageA, "wrong", tobari.RuntimeImageLifetimeCommand, "tobari", `["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]`)}, wantCalls: 1},
		{name: "wrong lifetime label", observations: []string{finalStandardRuntimeObservation(imageA, tobari.RuntimeImageAPI, "wrong", "tobari", `["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]`)}, wantCalls: 1},
		{name: "wrong user", observations: []string{finalStandardRuntimeObservation(imageA, tobari.RuntimeImageAPI, tobari.RuntimeImageLifetimeCommand, "root", `["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]`)}, wantCalls: 1},
		{name: "wrong entrypoint", observations: []string{finalStandardRuntimeObservation(imageA, tobari.RuntimeImageAPI, tobari.RuntimeImageLifetimeCommand, "tobari", `["/bin/sh"]`)}, wantCalls: 1},
		{name: "non digest image id", observations: []string{finalStandardRuntimeObservation("runtime:latest", tobari.RuntimeImageAPI, tobari.RuntimeImageLifetimeCommand, "tobari", `["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]`)}, wantCalls: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &finalContextRuntimeRunner{observations: append([]string{}, test.observations...), err: test.runnerErr}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			standard, err := runtime.standardRuntimeManifest()
			if err != nil {
				t.Fatal(err)
			}
			binding, err := standard.Binding(1)
			if err != nil {
				t.Fatal(err)
			}
			if test.mutate != nil {
				binding = test.mutate(binding)
			}
			got, resolveErr := runtime.resolveFinalContextLoginRuntimeImage(context.Background(), binding)
			if test.name == "exact immutable image id" {
				if resolveErr != nil || got != imageA {
					t.Fatalf("resolve image = %q, %v", got, resolveErr)
				}
			} else if resolveErr == nil {
				t.Fatalf("unsafe Runtime material resolved as %q", got)
			}
			if len(runner.calls) != test.wantCalls {
				t.Fatalf("Docker observations = %d, want %d: %v", len(runner.calls), test.wantCalls, runner.calls)
			}
			for _, call := range runner.calls {
				if call[len(call)-1] != binding.Image {
					t.Fatalf("Docker observation selected ambient image: %v", call)
				}
			}
		})
	}
}

func TestFinalContextLoginRuntimeRejectsManagedAuthorityDrift(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "credential-runtime", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	built, err := runtime.BuildManagedRuntime(context.Background(), created.Runtime.Name, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := built.Runtime.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	imageID, err := runtime.resolveFinalContextLoginRuntimeImage(context.Background(), binding)
	if err != nil || imageID != built.Runtime.Revisions[0].ImageDigest {
		t.Fatalf("exact managed image = %q, %v", imageID, err)
	}

	for _, corrupt := range []string{"digest", "owner", "component", "runtime", "revision"} {
		t.Run(corrupt, func(t *testing.T) {
			runner.corruptEvidence = corrupt
			if _, err := runtime.resolveFinalContextLoginRuntimeImage(context.Background(), binding); err == nil {
				t.Fatalf("managed %s drift passed", corrupt)
			}
			runner.corruptEvidence = ""
		})
	}
	delete(runner.images, binding.Image)
	if _, err := runtime.resolveFinalContextLoginRuntimeImage(context.Background(), binding); err == nil {
		t.Fatal("pruned managed Runtime image passed")
	}
}

type finalContextHostLoginRunner struct {
	runs    [][]string
	outputs [][]string
}

func (r *finalContextHostLoginRunner) Run(_ context.Context, args, _ []string, _ io.Reader, stdout, _ io.Writer) error {
	r.runs = append(r.runs, append([]string{}, args...))
	switch {
	case containsString(args, "health"):
		_, _ = io.WriteString(stdout, `{"schema_version":1,"ok":true,"state":"unlocked"}`)
	case containsString(args, "login"):
		_, _ = io.WriteString(stdout, `{"schema_version":1,"ok":true,"provider":"github","revision":"`+strings.Repeat("a", 64)+`","account_label":"octo-user"}`)
	default:
		return fmt.Errorf("unexpected final Context host login mutation: %v", args)
	}
	return nil
}

func (r *finalContextHostLoginRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, append([]string{}, args...))
	if len(args) > 0 && args[0] == "inspect" && args[len(args)-1] == authBrokerContainer {
		return []byte(`{"state":"running","health":"healthy"}`), nil
	}
	return nil, fmt.Errorf("unexpected final Context host login observation: %v", args)
}

func TestFinalContextStandardProviderLoginDoesNotResolveRuntimeImage(t *testing.T) {
	if !capabilitysurface.Compiled().IncludesResearch() {
		t.Skip("final Context Broker adapter is compiled only in the research surface")
	}
	collection := finalProjectionCollectionFixture(t, "")
	snapshots, err := collection.ContextSnapshots()
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("Context snapshots = %d, %v", len(snapshots), err)
	}
	contextRef, err := tobari.ContextRef(snapshots[0].Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := authbroker.NewContextAuthenticationAuthority(snapshots[0], contextRef)
	if err != nil {
		t.Fatal(err)
	}
	runner := &finalContextHostLoginRunner{}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.hostCLIs = &fakeHostCLIResolver{path: "/usr/local/bin/gh"}
	runtime.credentialHost = &fakeHostCredentialAcquirer{githubPayload: hostCredentialPayload{secret: []byte("ghp_synthetic_host_token_canary_123456"), accountLabel: "octo-user"}}
	runtime.browser = &recordingBrowser{}
	target, err := runtime.ResolveFinalContextProvider(context.Background(), authority, authbroker.BuiltinGitHubProviderID)
	if err != nil {
		t.Fatal(err)
	}
	status, _, _, err := runtime.LoginFinalContextProvider(context.Background(), target, "", strings.NewReader(""), io.Discard)
	if err != nil || status.Provider != authbroker.BuiltinGitHubProviderID || status.State != authbroker.ProviderCredentialConfigured {
		t.Fatalf("standard provider login = %#v, %v", status, err)
	}
	for _, call := range append(append([][]string{}, runner.runs...), runner.outputs...) {
		if len(call) >= 2 && call[0] == "image" && call[1] == "inspect" {
			t.Fatalf("standard provider resolved a Runtime image: %v", call)
		}
	}
}

func TestFinalContextAuthAdapterIsUnavailableInReleaseSurface(t *testing.T) {
	if capabilitysurface.Compiled().IncludesResearch() {
		t.Skip("release-surface absence assertion")
	}
	runner := &finalContextRuntimeRunner{}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ResolveFinalContextProvider(context.Background(), authbroker.ContextAuthenticationAuthority{}, authbroker.BuiltinGitHubProviderID); err == nil {
		t.Fatal("release surface resolved final research credential authority")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("release surface crossed Docker: %v", runner.calls)
	}
}

func TestFinalContextAuthObservationDoesNotCreateFreshLifecycleState(t *testing.T) {
	if !capabilitysurface.Compiled().IncludesResearch() {
		t.Skip("final Context Broker adapter is compiled only in the research surface")
	}
	root := t.TempDir()
	state := filepath.Join(root, "state", "tobari")
	runtime, err := newRuntime(filepath.Join(root, "config", "tobari"), state, &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err := runtime.WithFinalContextAuthObservation(context.Background(), func(context.Context) error {
		calls++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(before) != 0 || len(after) != 0 {
		t.Fatalf("fresh read mutated full tree: calls=%d before=%v after=%v", calls, before, after)
	}
	if _, err := os.Lstat(filepath.Join(state, "lifecycle.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only observation created lifecycle lock: %v", err)
	}

	if err := runtime.WithFinalContextAuthObservation(context.Background(), func(context.Context) error {
		return os.MkdirAll(state, 0o700)
	}); err == nil {
		t.Fatal("state appearing during fresh observation was accepted")
	}
	if _, err := os.Lstat(filepath.Join(state, "lifecycle.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("drift rejection created lifecycle lock: %v", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
