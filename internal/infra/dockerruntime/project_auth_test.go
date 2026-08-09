package dockerruntime

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type authProjectionRunner struct {
	handle string
	calls  [][]string
}

func (r *authProjectionRunner) Run(_ context.Context, args []string, _ []string, _ io.Reader, out, _ io.Writer) error {
	r.calls = append(r.calls, append([]string(nil), args...))
	if !slices.Contains(args, "authbroker.control") {
		return nil
	}
	response := map[string]any{"schema_version": 1, "ok": true}
	provider := ""
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "--provider" {
			provider = args[index+1]
			break
		}
	}
	switch {
	case slices.Contains(args, "health"):
		response["state"] = "unlocked"
	case slices.Contains(args, "status"):
		response["provider"] = provider
		if provider == "github" {
			response["state"] = "ready"
			response["revision"] = "revision_synthetic"
		} else {
			response["state"] = "not_configured"
		}
	case slices.Contains(args, "issue_handle"):
		response["provider"] = provider
		response["revision"] = "revision_synthetic"
		response["handle"] = r.handle
	}
	return json.NewEncoder(out).Encode(response)
}

func (r *authProjectionRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) != 0 && args[0] == "inspect" && slices.Contains(args, authBrokerContainer) {
		return []byte(`{"state":"running","health":"healthy"}`), nil
	}
	return nil, nil
}

func syntheticProjectHandle(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	return "tobari-h1_" + base64.RawURLEncoding.EncodeToString(digest[:])
}

func TestReconcileProjectAuthProjectsOnlyHandleAndProviderMetadata(t *testing.T) {
	runner := &authProjectionRunner{handle: syntheticProjectHandle("project-a")}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	if err := os.MkdirAll(filepath.Join(runtime.stateDirectory, "auth", "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	projection, err := runtime.reconcileProjectAuth(context.Background(), instance)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GH_HOST=github.com", "GH_TOKEN=" + runner.handle}
	if !slices.Equal(projection.Environment, want) {
		t.Fatalf("Workspace auth environment = %v, want %v", projection.Environment, want)
	}
	if len(projection.Files) != 0 {
		t.Fatalf("GitHub projection unexpectedly wrote files: %+v", projection.Files)
	}
	joined := strings.Join(projection.Environment, "\n")
	if strings.Contains(joined, "synthetic-real-token") || !strings.Contains(joined, "tobari-h1_") {
		t.Fatalf("Workspace projection contains the wrong credential material: %q", joined)
	}
	foundIssue := false
	for _, call := range runner.calls {
		if slices.Contains(call, "issue_handle") {
			foundIssue = slices.Contains(call, instance.ContextID) && slices.Contains(call, instance.ID)
			for index, argument := range call {
				if argument != "--bindings" || index+1 >= len(call) {
					continue
				}
				var bindings []map[string]any
				if err := json.Unmarshal([]byte(call[index+1]), &bindings); err != nil || len(bindings) != 2 ||
					bindings[0]["provider_id"] != "github" || bindings[1]["provider_id"] != "github" {
					t.Fatalf("Broker handle bindings = %s, want generic provider identity on every binding", call[index+1])
				}
			}
		}
	}
	if !foundIssue {
		t.Fatalf("handle issue did not bind Context and project: %v", runner.calls)
	}
}

func TestBrokerBindingsForAWSIncludesCanonicalSigningPlanInDigest(t *testing.T) {
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	projection, err := runtime.loadAuthProviders()
	if err != nil {
		t.Fatal(err)
	}
	bindings, encoded, digest, err := brokerBindingsForProvider(projection, "aws")
	if err != nil {
		t.Fatal(err)
	}
	const want = `[{"provider_id":"aws","kind":"aws_sigv4","aws_sigv4":{"target":{"scheme":"https","port":443,"dns_suffixes":["amazonaws.com"]},"source":{"authorization_header":"authorization","security_token_header":"x-amz-security-token"},"secret_headers":["authorization","x-amz-security-token"]}}]`
	if string(encoded) != want {
		t.Fatalf("AWS broker bindings = %s, want %s", encoded, want)
	}
	if len(bindings) != 1 || bindings[0].ProviderID != "aws" ||
		bindings[0].Kind != "aws_sigv4" || bindings[0].AWSSigV4 == nil ||
		bindings[0].Target != nil || bindings[0].Source != nil || bindings[0].Destination != nil ||
		len(bindings[0].SecretHeaders) != 0 {
		t.Fatalf("AWS broker binding union = %+v", bindings)
	}
	if digest != digestBytes(encoded) {
		t.Fatalf("AWS binding digest = %q, want digest of full union %q", digest, digestBytes(encoded))
	}
}

func TestReconcileProjectAuthFilesOwnsCompleteFilesWithoutEscapingHome(t *testing.T) {
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	if err := os.MkdirAll(filepath.Join(runtime.stateDirectory, "auth", "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("handle=tobari-h1_synthetic\n")
	file := projectAuthFile{Path: ".config/synthetic/auth", Content: content, Digest: digestBytes(content)}
	if err := runtime.reconcileProjectAuthFiles(instance, []projectAuthFile{file}, []projectAuthProviderBinding{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runtime.projectHomePath(instance.ID), filepath.FromSlash(file.Path))
	observed, err := os.ReadFile(path)
	if err != nil || string(observed) != string(content) {
		t.Fatalf("projected file = %q, err=%v", observed, err)
	}
	if info, err := os.Lstat(path); err != nil || info.Mode().Perm() != 0o600 || !info.Mode().IsRegular() {
		t.Fatalf("projected file mode/type = %v, err=%v", info, err)
	}
	if err := runtime.reconcileProjectAuthFiles(instance, []projectAuthFile{{Path: "../escape", Content: content, Digest: digestBytes(content)}}, []projectAuthProviderBinding{}); err == nil {
		t.Fatal("relative escape projection succeeded")
	}
}

func TestReconcileProjectAuthFilesRefusesUnownedAndSymlinkTargets(t *testing.T) {
	for name, prepare := range map[string]func(*testing.T, *Runtime, string){
		"unowned file": func(t *testing.T, runtime *Runtime, projectID string) {
			path := filepath.Join(runtime.projectHomePath(projectID), ".config", "synthetic", "auth")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("user-owned"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"symlink parent": func(t *testing.T, runtime *Runtime, projectID string) {
			outside := t.TempDir()
			if err := os.Symlink(outside, filepath.Join(runtime.projectHomePath(projectID), ".config")); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), &recordingRunner{})
			if err != nil {
				t.Fatal(err)
			}
			instance := projectRuntimeInstance(t, runtime)
			if err := os.MkdirAll(filepath.Join(runtime.stateDirectory, "auth", "projects"), 0o700); err != nil {
				t.Fatal(err)
			}
			prepare(t, runtime, instance.ID)
			content := []byte("replacement")
			err = runtime.reconcileProjectAuthFiles(instance, []projectAuthFile{{
				Path: ".config/synthetic/auth", Content: content, Digest: digestBytes(content),
			}}, []projectAuthProviderBinding{})
			if err == nil {
				t.Fatal("unsafe Workspace auth file target was overwritten")
			}
		})
	}
}
