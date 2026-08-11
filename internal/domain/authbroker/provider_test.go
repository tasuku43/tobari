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
		SchemaVersion: LegacyProviderSchemaVersion,
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

func syntheticAWSProvider() Provider {
	return Provider{
		SchemaVersion: ProviderSchemaVersion,
		ID:            "aws",
		DisplayName:   "AWS IAM Identity Center",
		Acquisition:   Acquisition{Mode: AcquisitionBuiltinHelper, Helper: "aws-sso"},
		Credential:    Credential{Kind: CredentialAWSSSOSession},
		WorkspaceProjections: []WorkspaceProjection{
			{Kind: WorkspaceProjectionEnvironment, Name: "AWS_ACCESS_KEY_ID", Template: "${HANDLE}"},
			{Kind: WorkspaceProjectionEnvironment, Name: "AWS_SECRET_ACCESS_KEY", Template: "${HANDLE}"},
			{Kind: WorkspaceProjectionEnvironment, Name: "AWS_SESSION_TOKEN", Template: "${HANDLE}"},
			{Kind: WorkspaceProjectionEnvironment, Name: "AWS_EC2_METADATA_DISABLED", Template: "true"},
		},
		HeaderBindings: []HeaderBinding{},
		SigningBindings: []SigningBinding{{
			Kind: SigningBindingAWSSigV4,
			AWSSigV4: &AWSSigV4Binding{
				Target: AWSSigV4Target{
					Scheme: "https", Port: 443,
					DNSSuffixes: []string{"amazonaws.com"},
				},
				Source: AWSSigV4Source{
					AuthorizationHeader: "authorization",
					SecurityTokenHeader: "x-amz-security-token",
				},
				SecretHeaders: []string{"x-amz-security-token", "authorization"},
			},
		}},
	}
}

func syntheticOpenAICodexProvider() Provider {
	return Provider{
		SchemaVersion: ProviderSchemaVersion,
		ID:            "openai",
		DisplayName:   "OpenAI account for Codex",
		Acquisition:   Acquisition{Mode: AcquisitionBuiltinHelper, Helper: "codex-chatgpt-oauth"},
		Credential:    Credential{Kind: CredentialOpenAICodexOAuthSession},
		WorkspaceProjections: []WorkspaceProjection{{
			Kind: WorkspaceProjectionCompleteFile, Path: ".codex/auth.json", Template: openAICodexWorkspaceAuthTemplate,
		}},
		HeaderBindings: []HeaderBinding{{
			Target: BindingTarget{Scheme: "https", Host: "chatgpt.com", Port: 443},
			Source: BindingSource{Header: "authorization", Formats: []SourceFormat{SourceFormatBearer}},
			Destination: BindingDestination{
				Header: "authorization", Format: DestinationFormatBearer, SecretField: CredentialOpenAICodexOAuthSession,
			},
			SecretHeaders: []string{"authorization", "chatgpt-account-id", "x-openai-fedramp"},
		}},
	}
}

func syntheticAnthropicClaudeProvider() Provider {
	return Provider{
		SchemaVersion: LegacyProviderSchemaVersion,
		ID:            "anthropic",
		DisplayName:   "Anthropic account for Claude Code",
		Acquisition:   Acquisition{Mode: AcquisitionBuiltinHelper, Helper: "claude-setup-token"},
		Credential:    Credential{Kind: CredentialPrimarySecret},
		WorkspaceProjections: []WorkspaceProjection{{
			Kind: WorkspaceProjectionEnvironment, Name: "CLAUDE_CODE_OAUTH_TOKEN", Template: "${HANDLE}",
		}},
		HeaderBindings: []HeaderBinding{{
			Target: BindingTarget{Scheme: "https", Host: "api.anthropic.com", Port: 443},
			Source: BindingSource{Header: "authorization", Formats: []SourceFormat{SourceFormatBearer}},
			Destination: BindingDestination{
				Header: "authorization", Format: DestinationFormatBearer, SecretField: CredentialPrimarySecret,
			},
			SecretHeaders: []string{"authorization"},
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

func TestParseProviderV2RejectsUnknownBehavioralPlanFields(t *testing.T) {
	valid := providerJSON(t, syntheticAWSProvider())
	if _, err := ParseProvider(valid); err != nil {
		t.Fatalf("ParseProvider(valid AWS): %v", err)
	}
	cases := map[string][]byte{
		"null header bindings": bytes.Replace(
			valid, []byte(`"header_bindings":[]`), []byte(`"header_bindings":null`), 1,
		),
		"unknown binding field": bytes.Replace(
			valid, []byte(`"kind":"aws_sigv4"`), []byte(`"kind":"aws_sigv4","command":["aws"]`), 1,
		),
		"unknown plan field": bytes.Replace(
			valid, []byte(`"target":{"scheme"`), []byte(`"executable":"aws","target":{"scheme"`), 1,
		),
		"unknown target field": bytes.Replace(
			valid, []byte(`"scheme":"https"`), []byte(`"scheme":"https","host":"*.amazonaws.com"`), 1,
		),
		"unknown source field": bytes.Replace(
			valid, []byte(`"authorization_header":"authorization"`),
			[]byte(`"authorization_header":"authorization","algorithm":"AWS4-HMAC-SHA256"`), 1,
		),
		"schema-v1 behavioral field": bytes.Replace(valid, []byte(`"schema_version":2`), []byte(`"schema_version":1`), 1),
	}
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseProvider(document); err == nil {
				t.Fatal("ParseProvider accepted an expanded behavioral manifest")
			}
		})
	}
}

func TestProviderV2ValidatesClosedCredentialPlans(t *testing.T) {
	cases := map[string]func(*Provider){
		"legacy kind in v2": func(p *Provider) { p.Credential.Kind = CredentialPrimarySecret },
		"AWS kind in v1":    func(p *Provider) { p.SchemaVersion = LegacyProviderSchemaVersion },
		"wrong provider ID": func(p *Provider) { p.ID = "aws-alt" },
		"wrong helper":      func(p *Provider) { p.Acquisition.Helper = "aws-cli" },
		"stdin acquisition": func(p *Provider) { p.Acquisition = Acquisition{Mode: AcquisitionStdinImport} },
		"header binding": func(p *Provider) {
			p.HeaderBindings = cloneProvider(syntheticProvider()).HeaderBindings
		},
		"missing signing":      func(p *Provider) { p.SigningBindings = nil },
		"unknown signing kind": func(p *Provider) { p.SigningBindings[0].Kind = "aws_sigv4a" },
		"missing plan":         func(p *Provider) { p.SigningBindings[0].AWSSigV4 = nil },
		"http":                 func(p *Provider) { p.SigningBindings[0].AWSSigV4.Target.Scheme = "http" },
		"custom port":          func(p *Provider) { p.SigningBindings[0].AWSSigV4.Target.Port = 8443 },
		"missing suffix": func(p *Provider) {
			p.SigningBindings[0].AWSSigV4.Target.DNSSuffixes = []string{}
		},
		"unreviewed suffix": func(p *Provider) {
			p.SigningBindings[0].AWSSigV4.Target.DNSSuffixes = []string{"example.com"}
		},
		"wildcard suffix": func(p *Provider) {
			p.SigningBindings[0].AWSSigV4.Target.DNSSuffixes = []string{"*.amazonaws.com"}
		},
		"wrong authorization header": func(p *Provider) {
			p.SigningBindings[0].AWSSigV4.Source.AuthorizationHeader = "x-authorization"
		},
		"wrong security header": func(p *Provider) {
			p.SigningBindings[0].AWSSigV4.Source.SecurityTokenHeader = "x-amz-token"
		},
		"missing secret header": func(p *Provider) {
			p.SigningBindings[0].AWSSigV4.SecretHeaders = []string{"authorization"}
		},
		"unexpected secret header": func(p *Provider) {
			p.SigningBindings[0].AWSSigV4.SecretHeaders = []string{"authorization", "x-api-key"}
		},
		"missing AWS env": func(p *Provider) { p.WorkspaceProjections = p.WorkspaceProjections[:3] },
		"raw access key": func(p *Provider) {
			p.WorkspaceProjections[0].Template = "AKIAEXAMPLE"
		},
		"AWS config file": func(p *Provider) {
			p.WorkspaceProjections[0] = WorkspaceProjection{
				Kind: WorkspaceProjectionCompleteFile, Path: ".aws/config", Template: "${HANDLE}",
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			provider := cloneProvider(syntheticAWSProvider())
			mutate(&provider)
			if err := provider.Validate(); err == nil {
				t.Fatal("Provider.Validate accepted an unreviewed schema-v2 credential plan")
			}
		})
	}
}

func TestOpenAICodexProviderPublishesOnlyTheReviewedOAuthShim(t *testing.T) {
	provider := syntheticOpenAICodexProvider()
	if err := provider.Validate(); err != nil {
		t.Fatalf("Provider.Validate rejected reviewed OpenAI Codex plan: %v", err)
	}
	projection, err := NormalizeProviders([]Provider{provider})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Environment) != 0 || len(projection.CompleteFiles) != 1 ||
		projection.CompleteFiles[0] != (CompleteFileProjection{
			ProviderID: "openai", Path: ".codex/auth.json", Template: openAICodexWorkspaceAuthTemplate,
		}) {
		t.Fatalf("OpenAI Workspace projection = %#v", projection)
	}
	if len(projection.HeaderBindings) != 1 {
		t.Fatalf("OpenAI normalized bindings = %#v", projection.HeaderBindings)
	}
	binding := projection.HeaderBindings[0]
	if binding.ProviderID != "openai" ||
		binding.Target != (BindingTarget{Scheme: "https", Host: "chatgpt.com", Port: 443}) ||
		binding.Source != (NormalizedBindingSource{Header: "authorization", Format: SourceFormatBearer}) ||
		binding.Destination != (BindingDestination{
			Header: "authorization", Format: DestinationFormatBearer, SecretField: CredentialOpenAICodexOAuthSession,
		}) || strings.Join(binding.SecretHeaders, ",") != "authorization,chatgpt-account-id,x-openai-fedramp" {
		t.Fatalf("OpenAI normalized binding = %#v", binding)
	}
	if strings.Join(projection.SecretHeaders, ",") != "authorization,chatgpt-account-id,x-openai-fedramp" {
		t.Fatalf("OpenAI secret headers = %#v", projection.SecretHeaders)
	}
}

func TestOpenAICodexProviderRejectsUnreviewedPlansAndCredentialProjection(t *testing.T) {
	cases := map[string]func(*Provider){
		"legacy schema":     func(p *Provider) { p.SchemaVersion = LegacyProviderSchemaVersion },
		"impostor provider": func(p *Provider) { p.ID = "openai-alt" },
		"different display name": func(p *Provider) {
			p.DisplayName = "OpenAI account"
		},
		"different helper":      func(p *Provider) { p.Acquisition.Helper = "codex-login" },
		"stdin acquisition":     func(p *Provider) { p.Acquisition = Acquisition{Mode: AcquisitionStdinImport} },
		"static credential":     func(p *Provider) { p.Credential.Kind = CredentialPrimarySecret },
		"additional projection": func(p *Provider) { p.WorkspaceProjections = append(p.WorkspaceProjections, p.WorkspaceProjections[0]) },
		"environment projection": func(p *Provider) {
			p.WorkspaceProjections[0] = WorkspaceProjection{Kind: WorkspaceProjectionEnvironment, Name: "CODEX_TOKEN", Template: "${HANDLE}"}
		},
		"different auth path": func(p *Provider) { p.WorkspaceProjections[0].Path = ".config/codex/auth.json" },
		"API key projection": func(p *Provider) {
			p.WorkspaceProjections[0].Template = strings.Replace(openAICodexWorkspaceAuthTemplate, `"OPENAI_API_KEY":null`, `"OPENAI_API_KEY":"${HANDLE}"`, 1)
		},
		"real-looking ID token": func(p *Provider) {
			p.WorkspaceProjections[0].Template = strings.Replace(openAICodexWorkspaceAuthTemplate, `"id_token":"e30.e30.x"`, `"id_token":"private-canary"`, 1)
		},
		"projected refresh token": func(p *Provider) {
			p.WorkspaceProjections[0].Template = strings.Replace(openAICodexWorkspaceAuthTemplate, `"refresh_token":""`, `"refresh_token":"${HANDLE}"`, 1)
		},
		"API authority":         func(p *Provider) { p.HeaderBindings[0].Target.Host = "api.openai.com" },
		"insecure authority":    func(p *Provider) { p.HeaderBindings[0].Target.Scheme = "http" },
		"custom port":           func(p *Provider) { p.HeaderBindings[0].Target.Port = 8443 },
		"alternate source":      func(p *Provider) { p.HeaderBindings[0].Source.Header = "x-api-key" },
		"raw source":            func(p *Provider) { p.HeaderBindings[0].Source.Formats = []SourceFormat{SourceFormatRaw} },
		"raw destination":       func(p *Provider) { p.HeaderBindings[0].Destination.Format = DestinationFormatRaw },
		"missing account strip": func(p *Provider) { p.HeaderBindings[0].SecretHeaders = []string{"authorization", "x-openai-fedramp"} },
		"missing FedRAMP strip": func(p *Provider) { p.HeaderBindings[0].SecretHeaders = []string{"authorization", "chatgpt-account-id"} },
		"reordered strip set": func(p *Provider) {
			p.HeaderBindings[0].SecretHeaders = []string{"chatgpt-account-id", "authorization", "x-openai-fedramp"}
		},
		"additional binding": func(p *Provider) { p.HeaderBindings = append(p.HeaderBindings, p.HeaderBindings[0]) },
		"signing plan": func(p *Provider) {
			p.SigningBindings = cloneProvider(syntheticAWSProvider()).SigningBindings
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			provider := cloneProvider(syntheticOpenAICodexProvider())
			mutate(&provider)
			if err := provider.Validate(); err == nil {
				t.Fatal("Provider.Validate accepted an unreviewed OpenAI Codex OAuth plan")
			}
		})
	}
}

func TestAnthropicClaudeProviderPublishesOnlyTheReviewedSetupTokenPlan(t *testing.T) {
	provider := syntheticAnthropicClaudeProvider()
	if err := provider.Validate(); err != nil {
		t.Fatalf("Provider.Validate rejected reviewed Anthropic Claude plan: %v", err)
	}
	projection, err := NormalizeProviders([]Provider{provider})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Environment) != 1 || projection.Environment[0] != (EnvironmentProjection{
		ProviderID: "anthropic", Name: "CLAUDE_CODE_OAUTH_TOKEN", Template: "${HANDLE}",
	}) || len(projection.CompleteFiles) != 0 {
		t.Fatalf("Anthropic Workspace projection = %#v", projection)
	}
	if len(projection.HeaderBindings) != 1 {
		t.Fatalf("Anthropic normalized bindings = %#v", projection.HeaderBindings)
	}
	binding := projection.HeaderBindings[0]
	if binding.ProviderID != "anthropic" ||
		binding.Target != (BindingTarget{Scheme: "https", Host: "api.anthropic.com", Port: 443}) ||
		binding.Source != (NormalizedBindingSource{Header: "authorization", Format: SourceFormatBearer}) ||
		binding.Destination != (BindingDestination{
			Header: "authorization", Format: DestinationFormatBearer, SecretField: CredentialPrimarySecret,
		}) || strings.Join(binding.SecretHeaders, ",") != "authorization" {
		t.Fatalf("Anthropic normalized binding = %#v", binding)
	}
}

func TestAnthropicClaudeProviderRejectsImpostorsAndRawSecretChannels(t *testing.T) {
	cases := map[string]func(*Provider){
		"behavioral schema":      func(p *Provider) { p.SchemaVersion = ProviderSchemaVersion },
		"impostor provider":      func(p *Provider) { p.ID = "anthropic-alt" },
		"different display name": func(p *Provider) { p.DisplayName = "Anthropic account" },
		"different helper":       func(p *Provider) { p.Acquisition.Helper = "claude-login" },
		"stdin acquisition":      func(p *Provider) { p.Acquisition = Acquisition{Mode: AcquisitionStdinImport} },
		"OAuth session kind":     func(p *Provider) { p.Credential.Kind = CredentialOpenAICodexOAuthSession },
		"additional projection":  func(p *Provider) { p.WorkspaceProjections = append(p.WorkspaceProjections, p.WorkspaceProjections[0]) },
		"API key environment":    func(p *Provider) { p.WorkspaceProjections[0].Name = "ANTHROPIC_API_KEY" },
		"auth token environment": func(p *Provider) { p.WorkspaceProjections[0].Name = "ANTHROPIC_AUTH_TOKEN" },
		"raw setup token":        func(p *Provider) { p.WorkspaceProjections[0].Template = "private-canary" },
		"credential file": func(p *Provider) {
			p.WorkspaceProjections[0] = WorkspaceProjection{Kind: WorkspaceProjectionCompleteFile, Path: ".claude/auth.json", Template: "${HANDLE}"}
		},
		"different authority": func(p *Provider) { p.HeaderBindings[0].Target.Host = "console.anthropic.com" },
		"insecure authority":  func(p *Provider) { p.HeaderBindings[0].Target.Scheme = "http" },
		"custom port":         func(p *Provider) { p.HeaderBindings[0].Target.Port = 8443 },
		"alternate source":    func(p *Provider) { p.HeaderBindings[0].Source.Header = "x-api-key" },
		"raw source":          func(p *Provider) { p.HeaderBindings[0].Source.Formats = []SourceFormat{SourceFormatRaw} },
		"raw destination":     func(p *Provider) { p.HeaderBindings[0].Destination.Format = DestinationFormatRaw },
		"additional secret":   func(p *Provider) { p.HeaderBindings[0].SecretHeaders = []string{"authorization", "x-api-key"} },
		"additional binding":  func(p *Provider) { p.HeaderBindings = append(p.HeaderBindings, p.HeaderBindings[0]) },
		"signing plan": func(p *Provider) {
			p.SigningBindings = cloneProvider(syntheticAWSProvider()).SigningBindings
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			provider := cloneProvider(syntheticAnthropicClaudeProvider())
			mutate(&provider)
			if err := provider.Validate(); err == nil {
				t.Fatal("Provider.Validate accepted an unreviewed Anthropic Claude setup-token plan")
			}
		})
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

func TestNormalizeProvidersPublishesDeterministicAWSSigningContract(t *testing.T) {
	provider := syntheticAWSProvider()
	projection, err := NormalizeProviders([]Provider{provider})
	if err != nil {
		t.Fatal(err)
	}
	if projection.SchemaVersion != ProviderSchemaVersion || len(projection.HeaderBindings) != 0 ||
		len(projection.SigningBindings) != 1 {
		t.Fatalf("AWS projection = %#v", projection)
	}
	binding := projection.SigningBindings[0]
	if binding.ProviderID != "aws" || binding.Kind != SigningBindingAWSSigV4 || binding.AWSSigV4 == nil ||
		strings.Join(binding.AWSSigV4.Target.DNSSuffixes, ",") != "amazonaws.com" ||
		strings.Join(binding.AWSSigV4.SecretHeaders, ",") != "authorization,x-amz-security-token" {
		t.Fatalf("normalized AWS signing binding = %#v", binding)
	}
	if strings.Join(projection.SecretHeaders, ",") != "authorization,x-amz-security-token" {
		t.Fatalf("AWS secret headers = %#v", projection.SecretHeaders)
	}
	bindings, err := provider.NormalizedSigningBindings()
	if err != nil || len(bindings) != 1 || bindings[0].ProviderID != "aws" {
		t.Fatalf("NormalizedSigningBindings() = %#v, %v", bindings, err)
	}

	collision := syntheticProvider()
	collision.HeaderBindings[0].Target.Host = "sts.amazonaws.com"
	collision.HeaderBindings[0].Source.Header = "authorization"
	collision.HeaderBindings[0].Destination.Header = "authorization"
	collision.HeaderBindings[0].SecretHeaders = []string{"authorization"}
	if _, err := NormalizeProviders([]Provider{provider, collision}); err == nil {
		t.Fatal("NormalizeProviders accepted a header binding overlapping AWS SigV4 recognition")
	} else if !errors.Is(err, ErrAmbiguousHTTPBinding) {
		t.Fatalf("AWS overlap error = %v, want ErrAmbiguousHTTPBinding", err)
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
