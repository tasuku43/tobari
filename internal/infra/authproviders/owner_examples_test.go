package authproviders

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

func TestOwnerExamplesLoadThroughOwnerManifestBoundary(t *testing.T) {
	requireUserProviderSupport(t)
	directory := filepath.Join(t.TempDir(), "providers")
	examples := map[string]string{
		"kubernetes-api-token.json": filepath.Join("kubernetes-bearer", "provider.json"),
		"twg-delegated-oauth.json":  filepath.Join("twg-delegated-oauth", "provider.json"),
	}
	for name, relative := range examples {
		data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "auth-providers", relative))
		if err != nil {
			t.Fatalf("read owner provider example %s: %v", name, err)
		}
		writeProviderDirectory(t, directory, name, data, 0o600)
	}

	loader, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := loader.Load()
	if err != nil {
		t.Fatalf("load owner provider examples: %v", err)
	}

	want := map[string]bool{
		"kubernetes-api-token": false,
		"twg-delegated-oauth":  false,
	}
	for _, provider := range projection.Providers {
		if _, tracked := want[provider.ID]; !tracked {
			continue
		}
		if provider.SchemaVersion != authbroker.LegacyProviderSchemaVersion ||
			provider.Acquisition.Mode != authbroker.AcquisitionStdinImport {
			t.Fatalf("loaded owner provider %q crossed the non-behavioral boundary: %#v", provider.ID, provider)
		}
		want[provider.ID] = true
	}
	for id, found := range want {
		if !found {
			t.Errorf("loaded projection is missing owner example %q", id)
		}
	}
}
