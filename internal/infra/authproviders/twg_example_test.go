package authproviders

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

func TestTWGDelegatedOAuthExampleIsOneExactStaticBinding(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "auth-providers", "twg-delegated-oauth", "provider.json"))
	if err != nil {
		t.Fatalf("read TWG provider example: %v", err)
	}
	provider, err := authbroker.ParseProvider(data)
	if err != nil {
		t.Fatalf("parse TWG provider example: %v", err)
	}

	if provider.SchemaVersion != authbroker.LegacyProviderSchemaVersion ||
		provider.Acquisition.Mode != authbroker.AcquisitionStdinImport ||
		provider.Credential.Kind != authbroker.CredentialPrimarySecret {
		t.Fatalf("TWG example must remain an owner schema-v1 stdin-import provider: %#v", provider)
	}
	if len(provider.WorkspaceProjections) != 1 {
		t.Fatalf("TWG example projections = %d, want 1", len(provider.WorkspaceProjections))
	}
	projection := provider.WorkspaceProjections[0]
	if projection.Kind != authbroker.WorkspaceProjectionEnvironment ||
		projection.Name != "TWG_OAUTH_ACCESS_TOKEN" || projection.Template != "${HANDLE}" {
		t.Fatalf("TWG example projection = %#v, want delegated-OAuth handle environment", projection)
	}
	if len(provider.HeaderBindings) != 1 {
		t.Fatalf("TWG example bindings = %d, want 1", len(provider.HeaderBindings))
	}
	binding := provider.HeaderBindings[0]
	if binding.Target != (authbroker.BindingTarget{
		Scheme: "https", Host: "api.atlassian.com", Port: 443,
	}) {
		t.Fatalf("TWG example target = %#v, want one exact Atlassian OAuth authority", binding.Target)
	}
	if binding.Source.Header != "authorization" ||
		len(binding.Source.Formats) != 1 || binding.Source.Formats[0] != authbroker.SourceFormatBearer ||
		binding.Destination.Header != "authorization" ||
		binding.Destination.Format != authbroker.DestinationFormatBearer ||
		binding.Destination.SecretField != authbroker.CredentialPrimarySecret ||
		len(binding.SecretHeaders) != 1 || binding.SecretHeaders[0] != "authorization" {
		t.Fatalf("TWG example must remain an exact bearer Authorization binding: %#v", binding)
	}
}
