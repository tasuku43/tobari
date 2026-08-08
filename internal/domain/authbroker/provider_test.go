package authbroker

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func syntheticProvider() Provider {
	return Provider{
		SchemaVersion: ProviderSchemaVersion,
		ID:            "example-token",
		DisplayName:   "Example API",
		Acquisition:   Acquisition{Mode: AcquisitionStdinImport},
		Credential:    Credential{Kind: CredentialPrimarySecret},
		WorkspaceProjections: []WorkspaceProjection{
			{Kind: WorkspaceProjectionEnvironment, Name: "EXAMPLE_TOKEN", Template: "${HANDLE}"},
			{Kind: WorkspaceProjectionCompleteFile, Path: ".config/example/auth.toml", Template: "provider=${PROVIDER_ID}\ntoken=${HANDLE}\n"},
		},
		HeaderBindings: []HeaderBinding{{
			Target: BindingTarget{Scheme: "https", Host: "api.example.com", Port: 443},
			Source: BindingSource{Header: "x-api-key", Formats: []SourceFormat{SourceFormatRaw}},
			Destination: BindingDestination{
				Header: "x-api-key", Format: DestinationFormatRaw, SecretField: CredentialPrimarySecret,
			},
			SecretHeaders: []string{"x-api-key"},
		}},
	}
}

func providerJSON(t *testing.T, provider Provider) []byte {
	t.Helper()
	data, err := json.Marshal(provider)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseProviderRejectsDuplicateUnknownAndTrailingJSON(t *testing.T) {
	valid := providerJSON(t, syntheticProvider())
	if _, err := ParseProvider(valid); err != nil {
		t.Fatalf("ParseProvider(valid): %v", err)
	}
	cases := map[string][]byte{
		"duplicate top-level": bytes.Replace(valid, []byte(`"id":"example-token"`), []byte(`"id":"example-token","id":"other"`), 1),
		"duplicate nested":    bytes.Replace(valid, []byte(`"mode":"stdin_import"`), []byte(`"mode":"stdin_import","mode":"builtin_helper"`), 1),
		"unknown top-level":   bytes.Replace(valid, []byte(`"schema_version":1`), []byte(`"schema_version":1,"command":"curl"`), 1),
		"case-folded field":   bytes.Replace(valid, []byte(`"display_name":"Example API"`), []byte(`"Display_Name":"Example API"`), 1),
		"unknown command":     bytes.Replace(valid, []byte(`"mode":"stdin_import"`), []byte(`"mode":"stdin_import","command":["sh","-c","env"]`), 1),
		"unknown shell":       bytes.Replace(valid, []byte(`"mode":"stdin_import"`), []byte(`"mode":"stdin_import","shell":"env"`), 1),
		"trailing":            append(append([]byte(nil), valid...), []byte(` {}`)...),
	}
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseProvider(document); err == nil {
				t.Fatal("ParseProvider accepted an ambiguous or expanded document")
			}
		})
	}
	if _, err := ParseProvider(bytes.Repeat([]byte("x"), MaxProviderDocumentBytes+1)); err == nil {
		t.Fatal("ParseProvider accepted an oversized document")
	}
	invalidUTF8 := append(append([]byte(nil), valid[:len(valid)-1]...), 0xff, '}')
	if _, err := ParseProvider(invalidUTF8); err == nil {
		t.Fatal("ParseProvider accepted invalid UTF-8")
	}
}

func TestProviderValidatesAcquisitionCredentialAndTemplates(t *testing.T) {
	cases := map[string]func(*Provider){
		"unknown acquisition": func(p *Provider) { p.Acquisition.Mode = "shell" },
		"helper missing":      func(p *Provider) { p.Acquisition = Acquisition{Mode: AcquisitionBuiltinHelper} },
		"helper unsafe":       func(p *Provider) { p.Acquisition = Acquisition{Mode: AcquisitionBuiltinHelper, Helper: "sh -c"} },
		"stdin helper":        func(p *Provider) { p.Acquisition.Helper = "github-gh" },
		"unknown credential":  func(p *Provider) { p.Credential.Kind = "password" },
		"unknown projection":  func(p *Provider) { p.WorkspaceProjections[0].Kind = "patch_file" },
		"reserved env":        func(p *Provider) { p.WorkspaceProjections[0].Name = "TOBARI_CONTEXT_ID" },
		"process env":         func(p *Provider) { p.WorkspaceProjections[0].Name = "HTTPS_PROXY" },
		"env with path":       func(p *Provider) { p.WorkspaceProjections[0].Path = ".config/x" },
		"file with name":      func(p *Provider) { p.WorkspaceProjections[1].Name = "TOKEN" },
		"unknown placeholder": func(p *Provider) { p.WorkspaceProjections[0].Template = "${SECRET}" },
		"repeated handle":     func(p *Provider) { p.WorkspaceProjections[0].Template = "${HANDLE}:${HANDLE}" },
		"env newline":         func(p *Provider) { p.WorkspaceProjections[0].Template = "${HANDLE}\n" },
		"no handle projection": func(p *Provider) {
			p.WorkspaceProjections[0].Template = "static"
			p.WorkspaceProjections[1].Template = "provider=${PROVIDER_ID}\n"
		},
		"oversize template": func(p *Provider) {
			p.WorkspaceProjections[1].Template = "${HANDLE}" + strings.Repeat("x", MaxTemplateBytes)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			provider := cloneProvider(syntheticProvider())
			mutate(&provider)
			if err := provider.Validate(); err == nil {
				t.Fatal("Provider.Validate accepted invalid provider")
			}
		})
	}

	builtin := syntheticProvider()
	builtin.Acquisition = Acquisition{Mode: AcquisitionBuiltinHelper, Helper: "reviewed-helper"}
	if err := builtin.Validate(); err != nil {
		t.Fatalf("Provider.Validate rejected bounded builtin helper: %v", err)
	}
	static := syntheticProvider()
	static.WorkspaceProjections = append(static.WorkspaceProjections, WorkspaceProjection{
		Kind: WorkspaceProjectionEnvironment, Name: "EXAMPLE_HOST", Template: "api.example.com",
	})
	if err := static.Validate(); err != nil {
		t.Fatalf("Provider.Validate rejected static bounded projection: %v", err)
	}
}

func TestValidateRelativeHomePathRejectsEscapesAndProjectPaths(t *testing.T) {
	for _, valid := range []string{".config/example/auth.json", ".local/share/tool/token", "tool.json"} {
		if err := ValidateRelativeHomePath(valid); err != nil {
			t.Errorf("ValidateRelativeHomePath(%q): %v", valid, err)
		}
	}
	for _, invalid := range []string{
		"", "/etc/passwd", "../secret", ".config/../secret", ".config//secret", ".config/", ".",
		`Z:\example\secret`, "Z:/example/secret", "~/secret", "${HANDLE}/secret", "project/root/../../secret",
	} {
		t.Run(invalid, func(t *testing.T) {
			if err := ValidateRelativeHomePath(invalid); err == nil {
				t.Fatalf("ValidateRelativeHomePath(%q) accepted unsafe path", invalid)
			}
		})
	}
}

func TestProviderValidatesExactHTTPSHeaderBindings(t *testing.T) {
	cases := map[string]func(*HeaderBinding){
		"http":                    func(b *HeaderBinding) { b.Target.Scheme = "http" },
		"wildcard host":           func(b *HeaderBinding) { b.Target.Host = "*.example.com" },
		"uppercase host":          func(b *HeaderBinding) { b.Target.Host = "API.example.com" },
		"host path":               func(b *HeaderBinding) { b.Target.Host = "api.example.com/v1" },
		"host port":               func(b *HeaderBinding) { b.Target.Host = "api.example.com:443" },
		"single label":            func(b *HeaderBinding) { b.Target.Host = "localhost" },
		"zero port":               func(b *HeaderBinding) { b.Target.Port = 0 },
		"large port":              func(b *HeaderBinding) { b.Target.Port = 65536 },
		"uppercase source header": func(b *HeaderBinding) { b.Source.Header = "Authorization" },
		"unknown source format":   func(b *HeaderBinding) { b.Source.Formats = []SourceFormat{"basic"} },
		"missing source formats":  func(b *HeaderBinding) { b.Source.Formats = nil },
		"duplicate source format": func(b *HeaderBinding) { b.Source.Formats = []SourceFormat{SourceFormatBearer, SourceFormatBearer} },
		"ambiguous raw source":    func(b *HeaderBinding) { b.Source.Formats = []SourceFormat{SourceFormatRaw, SourceFormatToken} },
		"unknown destination":     func(b *HeaderBinding) { b.Destination.Format = "template" },
		"unknown secret field":    func(b *HeaderBinding) { b.Destination.SecretField = "secondary_secret" },
		"missing secret headers":  func(b *HeaderBinding) { b.SecretHeaders = nil },
		"duplicate secret header": func(b *HeaderBinding) { b.SecretHeaders = []string{"x-api-key", "x-api-key"} },
		"bound header not secret": func(b *HeaderBinding) { b.SecretHeaders = []string{"authorization"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			provider := cloneProvider(syntheticProvider())
			mutate(&provider.HeaderBindings[0])
			if err := provider.Validate(); err == nil {
				t.Fatal("Provider.Validate accepted unsafe header binding")
			}
		})
	}

	for _, header := range []string{"host", "content-length", "proxy-authorization", "cookie", "set-cookie", "x-tobari-session", "x-tobari-credential-profile"} {
		t.Run("forbidden source "+header, func(t *testing.T) {
			provider := cloneProvider(syntheticProvider())
			provider.HeaderBindings[0].Source.Header = header
			provider.HeaderBindings[0].Destination.Header = header
			provider.HeaderBindings[0].SecretHeaders = []string{header}
			if err := provider.Validate(); err == nil {
				t.Fatalf("Provider.Validate accepted forbidden header %q", header)
			}
		})
		t.Run("forbidden destination "+header, func(t *testing.T) {
			provider := cloneProvider(syntheticProvider())
			provider.HeaderBindings[0].Destination.Header = header
			provider.HeaderBindings[0].SecretHeaders = []string{"x-api-key", header}
			if err := provider.Validate(); err == nil {
				t.Fatalf("Provider.Validate accepted forbidden destination header %q", header)
			}
		})
	}
}

func TestNormalizeProvidersExpandsFormatsAndRejectsCollisions(t *testing.T) {
	provider := syntheticProvider()
	provider.HeaderBindings[0].Source.Formats = []SourceFormat{SourceFormatToken, SourceFormatBearer}
	provider.HeaderBindings[0].Destination.Format = DestinationFormatPreserveScheme
	projection, err := NormalizeProviders([]Provider{provider})
	if err != nil {
		t.Fatal(err)
	}
	if projection.SchemaVersion != ProviderSchemaVersion || len(projection.HeaderBindings) != 2 {
		t.Fatalf("normalized projection = %#v", projection)
	}
	if projection.HeaderBindings[0].Source.Format != SourceFormatBearer || projection.HeaderBindings[1].Source.Format != SourceFormatToken {
		t.Fatalf("normalized source formats = %#v", projection.HeaderBindings)
	}
	bindings, err := provider.NormalizedBindings()
	if err != nil || len(bindings) != 2 {
		t.Fatalf("NormalizedBindings() = %#v, %v", bindings, err)
	}

	second := independentProvider()
	cases := map[string]func(*Provider){
		"provider ID":   func(p *Provider) { p.ID = provider.ID },
		"display name":  func(p *Provider) { p.DisplayName = provider.DisplayName },
		"environment":   func(p *Provider) { p.WorkspaceProjections[0].Name = provider.WorkspaceProjections[0].Name },
		"complete file": func(p *Provider) { p.WorkspaceProjections[1].Path = provider.WorkspaceProjections[1].Path },
		"header recognition": func(p *Provider) {
			p.HeaderBindings[0].Target = provider.HeaderBindings[0].Target
			p.HeaderBindings[0].Source.Header = provider.HeaderBindings[0].Source.Header
			p.HeaderBindings[0].Source.Formats = []SourceFormat{SourceFormatBearer}
			p.HeaderBindings[0].Destination = provider.HeaderBindings[0].Destination
			p.HeaderBindings[0].SecretHeaders = append([]string(nil), provider.HeaderBindings[0].SecretHeaders...)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := cloneProvider(second)
			mutate(&candidate)
			if _, err := NormalizeProviders([]Provider{provider, candidate}); err == nil {
				t.Fatal("NormalizeProviders accepted cross-provider collision")
			} else if name == "header recognition" && !errors.Is(err, ErrAmbiguousHTTPBinding) {
				t.Fatalf("header-recognition collision = %v, want ErrAmbiguousHTTPBinding", err)
			}
		})
	}

	disjoint := independentProvider()
	disjoint.HeaderBindings[0].Target = provider.HeaderBindings[0].Target
	disjoint.HeaderBindings[0].Source.Header = provider.HeaderBindings[0].Source.Header
	disjoint.HeaderBindings[0].Source.Formats = []SourceFormat{SourceFormatRaw}
	disjoint.HeaderBindings[0].Destination = provider.HeaderBindings[0].Destination
	disjoint.HeaderBindings[0].SecretHeaders = append([]string(nil), provider.HeaderBindings[0].SecretHeaders...)
	if _, err := NormalizeProviders([]Provider{provider, disjoint}); err == nil {
		t.Fatal("NormalizeProviders accepted raw recognition overlapping token/bearer")
	} else if !errors.Is(err, ErrAmbiguousHTTPBinding) {
		t.Fatalf("raw recognition collision = %v, want ErrAmbiguousHTTPBinding", err)
	}
}

func independentProvider() Provider {
	provider := syntheticProvider()
	provider.ID = "other-token"
	provider.DisplayName = "Other API"
	provider.WorkspaceProjections[0].Name = "OTHER_TOKEN"
	provider.WorkspaceProjections[1].Path = ".config/other/auth.toml"
	provider.HeaderBindings[0].Target.Host = "api.other.example"
	provider.HeaderBindings[0].Source.Header = "x-other-key"
	provider.HeaderBindings[0].Destination.Header = "x-other-key"
	provider.HeaderBindings[0].SecretHeaders = []string{"x-other-key"}
	return provider
}
