package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestProjectPrincipalRegistryRejectsAmbiguousBindings(t *testing.T) {
	registry := projectPrincipalRegistry{
		SchemaVersion: projectPrincipalRegistrySchema,
		Bindings: []projectPrincipalBinding{
			{ProjectID: "01912345-6789-7abc-8def-0123456789ab", GatewayIP: "172.29.0.2", Network: "tobari-a-net"},
			{ProjectID: "01912345-6789-7abc-8def-0123456789ac", GatewayIP: "172.29.0.2", Network: "tobari-b-net"},
		},
	}
	if err := registry.Validate(); err == nil {
		t.Fatal("project principal registry accepted duplicate Gateway address")
	}
}

func TestProjectPrincipalRegistryUpdateIsAtomicAndProjectBound(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	projectID := "01912345-6789-7abc-8def-0123456789ab"
	if err := runtime.updateProjectPrincipal(context.Background(), projectID, "tobari-a-net", "172.29.0.2"); err != nil {
		t.Fatalf("updateProjectPrincipal() error = %v", err)
	}
	data, err := os.ReadFile(runtime.principalRegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	var registry projectPrincipalRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	if len(registry.Bindings) != 1 || registry.Bindings[0].ProjectID != projectID {
		t.Fatalf("registry = %+v", registry)
	}
	if err := runtime.removeProjectPrincipal(context.Background(), projectID); err != nil {
		t.Fatalf("removeProjectPrincipal() error = %v", err)
	}
	registry = projectPrincipalRegistry{}
	data, err = os.ReadFile(runtime.principalRegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		t.Fatal(err)
	}
	if len(registry.Bindings) != 0 {
		t.Fatalf("removed project remains in registry: %+v", registry)
	}
}

func TestProjectPrincipalRegistryUsesDedicatedDirectoryAndMigratesLegacyFile(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	runtime, err := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.principalRegistryPath() == filepath.Join(config, "principals.json") {
		t.Fatal("principal registry still uses the single-file mount path")
	}
	legacy := projectPrincipalRegistry{
		SchemaVersion: projectPrincipalRegistrySchema,
		Bindings: []projectPrincipalBinding{{
			ProjectID: "01912345-6789-7abc-8def-0123456789ab",
			GatewayIP: "172.29.0.2", Network: "tobari-a-net",
		}},
	}
	legacyData, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(config, "principals.json")
	if err := os.WriteFile(legacyPath, append(legacyData, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := runtime.ensureProjectPrincipalRegistry(); err != nil {
		t.Fatalf("ensureProjectPrincipalRegistry() error = %v", err)
	}
	data, err := os.ReadFile(runtime.principalRegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	var migrated projectPrincipalRegistry
	if err := json.Unmarshal(data, &migrated); err != nil {
		t.Fatal(err)
	}
	if len(migrated.Bindings) != 1 || migrated.Bindings[0] != legacy.Bindings[0] {
		t.Fatalf("migrated registry = %+v, want %+v", migrated, legacy)
	}
}

func TestProjectPrincipalRegistryRejectsStaleOrMalformedState(t *testing.T) {
	tests := map[string]projectPrincipalRegistry{
		"wrong schema": {SchemaVersion: projectPrincipalRegistrySchema + 1},
		"invalid project": {
			SchemaVersion: projectPrincipalRegistrySchema,
			Bindings:      []projectPrincipalBinding{{ProjectID: "not-a-project", GatewayIP: "172.29.0.2", Network: "tobari-net"}},
		},
		"loopback": {
			SchemaVersion: projectPrincipalRegistrySchema,
			Bindings:      []projectPrincipalBinding{{ProjectID: "01912345-6789-7abc-8def-0123456789ab", GatewayIP: "127.0.0.1", Network: "tobari-net"}},
		},
	}
	for name, registry := range tests {
		t.Run(name, func(t *testing.T) {
			if err := registry.Validate(); err == nil {
				t.Fatal("registry unexpectedly validated")
			}
		})
	}
}

func TestProjectPrincipalRegistryMissingFileFailsClosed(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.readProjectPrincipalRegistry(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("readProjectPrincipalRegistry() error = %v, want missing file", err)
	}
}

func TestProjectPrincipalRegistryUsesValidatedProjectIDs(t *testing.T) {
	if err := tobari.ValidateProjectID("01912345-6789-7abc-8def-0123456789ab"); err != nil {
		t.Fatal(err)
	}
}
