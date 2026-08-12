package authbroker

import (
	"encoding/json"
	"strings"
	"testing"
)

func staticProvider(id string) Provider {
	return Provider{
		SchemaVersion:        ProviderSchemaVersion,
		ID:                   id,
		DisplayName:          "Synthetic static provider " + id,
		Acquisition:          Acquisition{Mode: AcquisitionStdinImport},
		Credential:           Credential{Kind: CredentialPrimarySecret},
		WorkspaceProjections: []WorkspaceProjection{{Kind: WorkspaceProjectionEnvironment, Name: "SYNTHETIC_TOKEN", Template: "${HANDLE}"}},
		HeaderBindings: []HeaderBinding{{
			Target:        BindingTarget{Scheme: "https", Host: id + ".example.com", Port: 443},
			Source:        BindingSource{Header: "authorization", Formats: []SourceFormat{SourceFormatBearer}},
			Destination:   BindingDestination{Header: "authorization", Format: DestinationFormatBearer, SecretField: CredentialPrimarySecret},
			SecretHeaders: []string{"authorization"},
		}},
	}
}

func TestStaticProviderRoundTripsAndNormalizes(t *testing.T) {
	provider := staticProvider("synthetic")
	data, err := json.Marshal(provider)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProvider(data)
	if err != nil {
		t.Fatalf("ParseProvider: %v", err)
	}
	projection, err := NormalizeProviders([]Provider{parsed})
	if err != nil {
		t.Fatalf("NormalizeProviders: %v", err)
	}
	if len(projection.Providers) != 1 || len(projection.HeaderBindings) != 1 || len(projection.Environment) != 1 {
		t.Fatalf("projection = %#v", projection)
	}
}

func TestProviderRejectsRetiredCredentialAndSigningPlans(t *testing.T) {
	base, err := json.Marshal(staticProvider("synthetic"))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"aws kind":     []byte(strings.Replace(string(base), `"primary_secret"`, `"aws_sso_session"`, 1)),
		"datadog kind": []byte(strings.Replace(string(base), `"primary_secret"`, `"datadog_oauth_session"`, 1)),
		"openai kind":  []byte(strings.Replace(string(base), `"primary_secret"`, `"openai_codex_oauth_session"`, 1)),
		"signing":      []byte(strings.TrimSuffix(string(base), "}") + `,"signing_bindings":[]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseProvider(data); err == nil {
				t.Fatal("retired provider plan was accepted")
			}
		})
	}
}

func TestProviderRejectsExecutableAcquisition(t *testing.T) {
	provider := staticProvider("synthetic")
	provider.Acquisition = Acquisition{Mode: "shell", Helper: "run"}
	if err := provider.Validate(); err == nil {
		t.Fatal("executable acquisition was accepted")
	}
}

func TestNormalizeProvidersRejectsDuplicateIdentityAndHTTPAuthority(t *testing.T) {
	left := staticProvider("left")
	right := staticProvider("right")
	right.HeaderBindings[0].Target = left.HeaderBindings[0].Target
	if _, err := NormalizeProviders([]Provider{left, right}); err == nil {
		t.Fatal("overlapping binding was accepted")
	}
	if _, err := NormalizeProviders([]Provider{left, left}); err == nil {
		t.Fatal("duplicate provider was accepted")
	}
}
