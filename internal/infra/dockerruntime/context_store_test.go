package dockerruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestContextStoreMigratesLegacyStoresAndPersistsRuntimeImage(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config")
	if err := os.MkdirAll(filepath.Join(config, "policy"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config, "credentials"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "config.json"), []byte(`{"version":"v1","default_image":"legacy-runtime:dev"}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyPolicy := []byte("package tobari\n\nallow := false\n")
	if err := os.WriteFile(filepath.Join(config, "policy", "tobari.rego"), legacyPolicy, 0o600); err != nil {
		t.Fatal(err)
	}
	legacyCredentials := []byte(`{"version":"v1","profiles":{}}`)
	if err := os.WriteFile(filepath.Join(config, "credentials.json"), legacyCredentials, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "credentials", "legacy-token"), []byte("synthetic-secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	runtime, err := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	contexts, err := runtime.ListContexts(context.Background())
	if err != nil {
		t.Fatalf("ListContexts() error = %v", err)
	}
	if contexts.Active != tobari.DefaultContextName || len(contexts.Items) != 1 || contexts.Items[0].Image != "legacy-runtime:dev" {
		t.Fatalf("initial Contexts = %+v", contexts)
	}

	defaultPolicy := filepath.Join(config, "contexts", "default", "policy", "tobari.rego")
	data, err := os.ReadFile(defaultPolicy)
	if err != nil || string(data) != string(legacyPolicy) {
		t.Fatalf("migrated policy = %q, error = %v", data, err)
	}
	migratedCredentials, err := os.ReadFile(filepath.Join(config, "contexts", "default", "credentials", "legacy-token"))
	if err != nil || string(migratedCredentials) != "synthetic-secret" {
		t.Fatalf("migrated credential = %q, error = %v", migratedCredentials, err)
	}
	if _, err := os.Stat(filepath.Join(config, "policy", "tobari.rego")); err != nil {
		t.Fatalf("legacy policy was removed: %v", err)
	}

	created, err := runtime.CreateContext(context.Background(), "project-tools", "tobari-runtime:local", tobari.ContextPolicyModeAdvanced)
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if created.Image != "tobari-runtime:local" || created.PolicyMode != tobari.ContextPolicyModeAdvanced {
		t.Fatalf("created Context = %+v", created)
	}
	if _, err := runtime.UseContext(context.Background(), "project-tools"); err != nil {
		t.Fatalf("UseContext() error = %v", err)
	}
	shown, err := runtime.ShowContext(context.Background(), "")
	if err != nil {
		t.Fatalf("ShowContext() error = %v", err)
	}
	if !shown.Active || shown.Name != "project-tools" || shown.Image != "tobari-runtime:local" {
		t.Fatalf("active Context = %+v", shown)
	}
	manifestData, err := os.ReadFile(filepath.Join(config, "contexts", "project-tools", "context.json"))
	if err != nil || strings.Contains(string(manifestData), "synthetic-secret") {
		t.Fatalf("manifest contains credential material or could not be read: %q, %v", manifestData, err)
	}
	for _, path := range []string{
		filepath.Join(config, "contexts", "project-tools"),
		filepath.Join(config, "contexts", "project-tools", "policy"),
		filepath.Join(config, "contexts", "project-tools", "credentials"),
		filepath.Join(config, "contexts", "project-tools", "context.json"),
		filepath.Join(config, "contexts", "active.json"),
	} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("Context path %s is not owner-only: %o", path, info.Mode().Perm())
		}
	}
}

func TestContextImageBecomesProjectDefaultAndOutlivesLegacyConfig(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "config.json"), []byte(`{"version":"v1","default_image":"initial:dev"}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.UseContext(context.Background(), tobari.DefaultContextName); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "config.json"), []byte(`{"version":"v1","default_image":"--invalid"}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	shown, err := runtime.ShowContext(context.Background(), "")
	if err != nil || shown.Image != "initial:dev" {
		t.Fatalf("Context after legacy config change = %+v, error = %v", shown, err)
	}

	image, err := runtime.resolveContextImage(context.Background())
	if err != nil || image != "initial:dev" {
		t.Fatalf("resolveContextImage() = %q, error = %v", image, err)
	}
}
