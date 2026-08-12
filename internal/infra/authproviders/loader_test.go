package authproviders

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

func TestBuiltinsContainOnlyGitHubStaticPlan(t *testing.T) {
	projection, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}
	if len(projection.Providers) != 1 {
		t.Fatalf("providers = %#v", projection.Providers)
	}
	provider := projection.Providers[0]
	if provider.ID != BuiltinGitHubProviderID || provider.Credential.Kind != authbroker.CredentialPrimarySecret ||
		provider.Acquisition != (authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "github-gh"}) {
		t.Fatalf("GitHub provider = %#v", provider)
	}
}

func TestOwnerProviderMustBeStaticStdinImport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner mode test is Unix-specific")
	}
	directory := filepath.Join(t.TempDir(), "providers")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"schema_version":1,"id":"owner","display_name":"Owner static provider","acquisition":{"mode":"stdin_import"},"credential":{"kind":"primary_secret"},"workspace_projections":[{"kind":"env","name":"OWNER_TOKEN","template":"${HANDLE}"}],"header_bindings":[{"target":{"scheme":"https","host":"owner.example.com","port":443},"source":{"header":"authorization","formats":["bearer"]},"destination":{"header":"authorization","format":"bearer","secret_field":"primary_secret"},"secret_headers":["authorization"]}]}`)
	if err := os.WriteFile(filepath.Join(directory, "owner.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	loader, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(projection.Providers) != 2 {
		t.Fatalf("providers = %#v", projection.Providers)
	}
}

func writeProviderDirectory(t *testing.T, directory, name string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(directory, name), mode); err != nil {
		t.Fatal(err)
	}
}

func requireUserProviderSupport(t *testing.T) {
	t.Helper()
	if !currentUserOwnershipSupported {
		t.Skip("current-user ownership checks are unsupported on this platform")
	}
}
