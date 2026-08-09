package authproviders

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

func TestKubernetesBearerExampleIsOneExactNonExecutableBinding(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "auth-providers", "kubernetes-bearer", "provider.json"))
	if err != nil {
		t.Fatalf("read Kubernetes provider example: %v", err)
	}
	provider, err := authbroker.ParseProvider(data)
	if err != nil {
		t.Fatalf("parse Kubernetes provider example: %v", err)
	}

	if provider.SchemaVersion != authbroker.LegacyProviderSchemaVersion ||
		provider.Acquisition.Mode != authbroker.AcquisitionStdinImport ||
		provider.Credential.Kind != authbroker.CredentialPrimarySecret {
		t.Fatalf("Kubernetes example must remain an owner schema-v1 stdin-import provider: %#v", provider)
	}
	if len(provider.WorkspaceProjections) != 1 {
		t.Fatalf("Kubernetes example projections = %d, want 1", len(provider.WorkspaceProjections))
	}
	projection := provider.WorkspaceProjections[0]
	if projection.Kind != authbroker.WorkspaceProjectionCompleteFile || projection.Path != ".kube/config" {
		t.Fatalf("Kubernetes example projection = %#v, want complete .kube/config", projection)
	}
	for _, required := range []string{
		"server: https://kubernetes-api.example.com:443",
		"certificate-authority-data: REPLACE_WITH_BASE64_PEM_CA_DATA",
		"token: \"${HANDLE}\"",
	} {
		if !strings.Contains(projection.Template, required) {
			t.Errorf("Kubernetes example kubeconfig is missing %q", required)
		}
	}
	for _, prohibited := range []string{
		"exec:", "auth-provider:", "client-certificate", "client-key",
		"insecure-skip-tls-verify", "proxy-url",
	} {
		if strings.Contains(projection.Template, prohibited) {
			t.Errorf("Kubernetes example kubeconfig contains prohibited field %q", prohibited)
		}
	}

	if len(provider.HeaderBindings) != 1 {
		t.Fatalf("Kubernetes example bindings = %d, want 1", len(provider.HeaderBindings))
	}
	binding := provider.HeaderBindings[0]
	if binding.Target != (authbroker.BindingTarget{
		Scheme: "https", Host: "kubernetes-api.example.com", Port: 443,
	}) {
		t.Fatalf("Kubernetes example target = %#v, want one exact HTTPS authority", binding.Target)
	}
	if binding.Source.Header != "authorization" ||
		len(binding.Source.Formats) != 1 || binding.Source.Formats[0] != authbroker.SourceFormatBearer ||
		binding.Destination.Header != "authorization" ||
		binding.Destination.Format != authbroker.DestinationFormatBearer {
		t.Fatalf("Kubernetes example must remain an exact bearer Authorization binding: %#v", binding)
	}
}
