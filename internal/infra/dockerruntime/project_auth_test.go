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
	handle        string
	readyProvider string
	calls         [][]string
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
		if provider == r.readyProvider {
			response["state"] = "configured"
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
	runner := &authProjectionRunner{handle: syntheticProjectHandle("project-a"), readyProvider: "github"}
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

func TestReconcileProjectAuthCanonicalizesEmptyRegistryForRepeatedReconciliation(t *testing.T) {
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), &authProjectionRunner{})
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	for attempt := 0; attempt < 2; attempt++ {
		projection, err := runtime.reconcileProjectAuth(context.Background(), instance)
		if err != nil {
			t.Fatalf("empty authentication reconciliation %d: %v", attempt+1, err)
		}
		if len(projection.Environment) != 0 || len(projection.Files) != 0 || len(projection.Providers) != 0 {
			t.Fatalf("credential-free reconciliation %d projected %+v", attempt+1, projection)
		}
	}
	data, err := os.ReadFile(runtime.projectAuthRegistryPath(instance.ID))
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Providers json.RawMessage `json:"providers"`
		Files     json.RawMessage `json:"files"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw.Providers) != "[]" || string(raw.Files) != "[]" {
		t.Fatalf("empty registry collections = providers:%s files:%s, want explicit empty arrays", raw.Providers, raw.Files)
	}
}

func TestReadProjectAuthRegistryRecoversOnlyKnownEmptyProviderNullDocument(t *testing.T) {
	newRuntimeWithRegistry := func(t *testing.T, documentFor func(string) string) (*Runtime, string, string) {
		t.Helper()
		runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), &recordingRunner{})
		if err != nil {
			t.Fatal(err)
		}
		instance := projectRuntimeInstance(t, runtime)
		registryPath := runtime.projectAuthRegistryPath(instance.ID)
		if err := os.MkdirAll(filepath.Dir(registryPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(registryPath, []byte(documentFor(instance.ID)), 0o600); err != nil {
			t.Fatal(err)
		}
		return runtime, instance.ID, registryPath
	}

	t.Run("recovers and rewrites the exact historical document", func(t *testing.T) {
		runtime, projectID, registryPath := newRuntimeWithRegistry(t, func(projectID string) string {
			return `{
  "schema_version": 1,
  "project_id": "` + projectID + `",
  "providers": null,
  "files": []
}
`
		})
		registry, err := runtime.readProjectAuthRegistry(projectID)
		if err != nil {
			t.Fatal(err)
		}
		if registry.Providers == nil || registry.Files == nil || len(registry.Providers) != 0 || len(registry.Files) != 0 {
			t.Fatalf("recovered registry = %+v, want canonical empty collections", registry)
		}
		data, err := os.ReadFile(registryPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), `"providers": null`) || !strings.Contains(string(data), `"providers": []`) || !strings.Contains(string(data), `"files": []`) {
			t.Fatalf("recovered registry was not canonically rewritten: %s", data)
		}
	})

	for name, documentFor := range map[string]func(string) string{
		"missing providers": func(projectID string) string {
			return `{"schema_version":1,"project_id":"` + projectID + `","files":[]}`
		},
		"null files": func(projectID string) string {
			return `{"schema_version":1,"project_id":"` + projectID + `","providers":null,"files":null}`
		},
		"nonempty files": func(projectID string) string {
			return `{"schema_version":1,"project_id":"` + projectID + `","providers":null,"files":[{"path":".config/auth","digest":"sha256:` + strings.Repeat("a", 64) + `"}]}`
		},
		"wrong project": func(string) string {
			return `{"schema_version":1,"project_id":"project-other","providers":null,"files":[]}`
		},
		"duplicate providers": func(projectID string) string {
			return `{"schema_version":1,"project_id":"` + projectID + `","providers":[],"providers":null,"files":[]}`
		},
		"duplicate files": func(projectID string) string {
			return `{"schema_version":1,"project_id":"` + projectID + `","providers":null,"files":null,"files":[]}`
		},
		"duplicate project": func(projectID string) string {
			return `{"schema_version":1,"project_id":"project-other","project_id":"` + projectID + `","providers":null,"files":[]}`
		},
		"duplicate schema": func(projectID string) string {
			return `{"schema_version":1,"schema_version":1,"project_id":"` + projectID + `","providers":null,"files":[]}`
		},
	} {
		t.Run(name+" remains rejected", func(t *testing.T) {
			runtime, projectID, registryPath := newRuntimeWithRegistry(t, documentFor)
			document := documentFor(projectID)
			if _, err := runtime.readProjectAuthRegistry(projectID); err == nil {
				t.Fatal("invalid registry was accepted")
			}
			data, err := os.ReadFile(registryPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != document {
				t.Fatalf("invalid registry was rewritten: got %q, want %q", data, document)
			}
		})
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
