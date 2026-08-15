package authproviders

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

type reviewedProviderCapabilityFixture struct {
	SchemaVersion      int                                  `json:"schema_version"`
	ReviewedLoginOrder []string                             `json:"reviewed_login_order"`
	Providers          []reviewedProviderCapabilityProvider `json:"providers"`
}

type reviewedProviderCapabilityProvider struct {
	ProviderID                string                                `json:"provider_id"`
	HostAcquisition           reviewedProviderCapabilityAcquisition `json:"host_acquisition"`
	ManifestCredentialKind    authbroker.CredentialKind             `json:"manifest_credential_kind"`
	BrokerControlLogin        string                                `json:"broker_control_login"`
	BrokerRecordKind          string                                `json:"broker_record_kind"`
	BrokerRuntimeCapabilities []string                              `json:"broker_runtime_capabilities"`
	GatewayReviewedProfile    bool                                  `json:"gateway_reviewed_profile"`
}

type reviewedProviderCapabilityAcquisition struct {
	Mode   authbroker.AcquisitionMode `json:"mode"`
	Helper string                     `json:"helper,omitempty"`
}

func TestBuiltinManifestCollectionMatchesClosedDomainRegistry(t *testing.T) {
	projection, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	providerIDs := make([]string, len(projection.Providers))
	for index, provider := range projection.Providers {
		providerIDs[index] = provider.ID
	}
	if got, want := strings.Join(providerIDs, ","), strings.Join(authbroker.ActiveBuiltinProviderIDs(), ","); got != want {
		t.Fatalf("embedded provider IDs = %q, want closed domain registry %q", got, want)
	}
	for _, provider := range projection.Providers {
		if provider.ID == authbroker.BuiltinAWSProviderID {
			if !authbroker.SupportsReviewedLoginProvider(provider.ID) {
				t.Fatal("inactive AWS manifest entered the standard projection")
			}
		}
	}
}

func TestReviewedProviderCapabilityFixtureMatchesGoAuthorities(t *testing.T) {
	fixture := readReviewedProviderCapabilityFixture(t)
	if fixture.SchemaVersion != 1 {
		t.Fatalf("fixture schema_version = %d, want 1", fixture.SchemaVersion)
	}
	if got, want := strings.Join(fixture.ReviewedLoginOrder, ","), strings.Join(authbroker.KnownReviewedLoginProviderIDs(), ","); got != want {
		t.Fatalf("fixture reviewed login order = %q, want %q", got, want)
	}

	providers, _, err := loadAllBuiltins()
	if err != nil {
		t.Fatal(err)
	}
	projection, err := authbroker.NormalizeProviders(providers)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture.Providers) != len(projection.Providers) {
		t.Fatalf("fixture providers = %d, built-ins = %d", len(fixture.Providers), len(projection.Providers))
	}
	for index, provider := range projection.Providers {
		entry := fixture.Providers[index]
		if entry.ProviderID != provider.ID {
			t.Fatalf("fixture provider[%d] = %q, built-in = %q", index, entry.ProviderID, provider.ID)
		}
		if entry.HostAcquisition != (reviewedProviderCapabilityAcquisition{
			Mode: provider.Acquisition.Mode, Helper: provider.Acquisition.Helper,
		}) {
			t.Fatalf("fixture acquisition for %q = %#v, built-in = %#v", provider.ID, entry.HostAcquisition, provider.Acquisition)
		}
		if entry.ManifestCredentialKind != provider.Credential.Kind {
			t.Fatalf("fixture credential kind for %q = %q, built-in = %q", provider.ID, entry.ManifestCredentialKind, provider.Credential.Kind)
		}
		_, supportsLogin := authbroker.KnownReviewedLoginProviderHelper(provider.ID)
		if got := entry.BrokerControlLogin != "none"; got != supportsLogin {
			t.Fatalf("fixture control-login membership for %q = %t, reviewed host login = %t", provider.ID, got, supportsLogin)
		}
	}
}

func readReviewedProviderCapabilityFixture(t *testing.T) reviewedProviderCapabilityFixture {
	t.Helper()
	path := filepath.Join("..", "..", "..", "authbroker", "tests", "fixtures", "reviewed_provider_capabilities_v1.json")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var fixture reviewedProviderCapabilityFixture
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("fixture has trailing JSON: %v", err)
	}
	return fixture
}

func TestBuiltinsPublishesExactToolContracts(t *testing.T) {
	providers, _, err := loadAllBuiltins()
	if err != nil {
		t.Fatal(err)
	}
	projection, err := authbroker.NormalizeProviders(providers)
	if err != nil {
		t.Fatal(err)
	}
	if projection.SchemaVersion != authbroker.ProviderSchemaVersion || len(projection.Providers) != 6 {
		t.Fatalf("built-in projection = %#v", projection)
	}
	providerByID := make(map[string]authbroker.Provider, len(projection.Providers))
	for _, provider := range projection.Providers {
		providerByID[provider.ID] = provider
	}
	if BuiltinAnthropicProviderID != "anthropic" || BuiltinAWSProviderID != "aws" ||
		BuiltinChatworkProviderID != "chatwork" || BuiltinDatadogProviderID != "datadog" ||
		BuiltinOpenAIProviderID != "openai" {
		t.Fatalf(
			"tool provider IDs = %q, %q, %q, %q, %q",
			BuiltinAnthropicProviderID, BuiltinAWSProviderID, BuiltinChatworkProviderID,
			BuiltinDatadogProviderID, BuiltinOpenAIProviderID,
		)
	}
	if BuiltinGitHubProviderID != "github" {
		t.Fatalf("BuiltinGitHubProviderID = %q", BuiltinGitHubProviderID)
	}
	provider := providerByID[BuiltinGitHubProviderID]
	if provider.ID != BuiltinGitHubProviderID ||
		provider.Acquisition.Mode != authbroker.AcquisitionBuiltinHelper || provider.Acquisition.Helper != "github-gh" {
		t.Fatalf("GitHub provider identity/acquisition = %#v", provider)
	}
	environment := map[string]string{}
	for _, item := range projection.Environment {
		environment[item.Name] = item.Template
	}
	if environment["GH_TOKEN"] != "${HANDLE}" || environment["GH_HOST"] != "github.com" {
		t.Fatalf("GitHub Workspace environment = %#v", environment)
	}
	githubBindings := 0
	for _, binding := range projection.HeaderBindings {
		if binding.ProviderID != BuiltinGitHubProviderID {
			continue
		}
		githubBindings++
		if binding.Target.Scheme != "https" ||
			binding.Target.Host != "api.github.com" || binding.Target.Port != 443 ||
			binding.Source.Header != "authorization" ||
			binding.Destination.Header != "authorization" ||
			binding.Destination.Format != authbroker.DestinationFormatPreserveScheme ||
			binding.Destination.SecretField != authbroker.CredentialPrimarySecret {
			t.Fatalf("GitHub normalized binding = %#v", binding)
		}
	}
	if githubBindings != 2 {
		t.Fatalf("GitHub normalized bindings = %#v", projection.HeaderBindings)
	}

	aws := providerByID[BuiltinAWSProviderID]
	if aws.ID != BuiltinAWSProviderID || aws.SchemaVersion != authbroker.ProviderSchemaVersion ||
		aws.Acquisition.Mode != authbroker.AcquisitionBuiltinHelper || aws.Acquisition.Helper != "aws-sso" ||
		aws.Credential.Kind != authbroker.CredentialAWSSSOSession || len(aws.HeaderBindings) != 0 {
		t.Fatalf("AWS provider identity/plan = %#v", aws)
	}
	awsEnvironment := map[string]string{}
	for _, item := range projection.Environment {
		if item.ProviderID == BuiltinAWSProviderID {
			awsEnvironment[item.Name] = item.Template
		}
	}
	wantAWSEnvironment := map[string]string{
		"AWS_ACCESS_KEY_ID":         "${HANDLE}",
		"AWS_SECRET_ACCESS_KEY":     "${HANDLE}",
		"AWS_SESSION_TOKEN":         "${HANDLE}",
		"AWS_EC2_METADATA_DISABLED": "true",
	}
	if len(awsEnvironment) != len(wantAWSEnvironment) {
		t.Fatalf("AWS Workspace environment = %#v", awsEnvironment)
	}
	for name, template := range wantAWSEnvironment {
		if awsEnvironment[name] != template {
			t.Fatalf("AWS Workspace environment = %#v", awsEnvironment)
		}
	}
	if len(projection.SigningBindings) != 1 {
		t.Fatalf("AWS signing bindings = %#v", projection.SigningBindings)
	}
	awsBinding := projection.SigningBindings[0]
	if awsBinding.ProviderID != BuiltinAWSProviderID || awsBinding.Kind != authbroker.SigningBindingAWSSigV4 ||
		awsBinding.AWSSigV4 == nil || awsBinding.AWSSigV4.Target.Scheme != "https" ||
		awsBinding.AWSSigV4.Target.Port != 443 ||
		strings.Join(awsBinding.AWSSigV4.Target.DNSSuffixes, ",") != "amazonaws.com" ||
		awsBinding.AWSSigV4.Source.AuthorizationHeader != "authorization" ||
		awsBinding.AWSSigV4.Source.SecurityTokenHeader != "x-amz-security-token" ||
		strings.Join(awsBinding.AWSSigV4.SecretHeaders, ",") != "authorization,x-amz-security-token" {
		t.Fatalf("AWS normalized signing binding = %#v", awsBinding)
	}

	assertImportedHeaderProvider(t, projection, importedHeaderExpectation{
		providerID:  BuiltinChatworkProviderID,
		displayName: "Chatwork API for cwk",
		environment: map[string]string{
			"CWK_API_TOKEN": "${HANDLE}",
		},
		host:              "api.chatwork.com",
		sourceHeader:      "x-chatworktoken",
		sourceFormat:      authbroker.SourceFormatRaw,
		destinationHeader: "x-chatworktoken",
		destinationFormat: authbroker.DestinationFormatRaw,
	})
	datadog := providerByID[BuiltinDatadogProviderID]
	if datadog.SchemaVersion != authbroker.ProviderSchemaVersion ||
		datadog.Acquisition != (authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "pup-oauth"}) ||
		datadog.Credential.Kind != authbroker.CredentialDatadogOAuthSession {
		t.Fatalf("Datadog provider plan = %#v", datadog)
	}
	datadogEnvironment := map[string]string{}
	for _, item := range projection.Environment {
		if item.ProviderID == BuiltinDatadogProviderID {
			datadogEnvironment[item.Name] = item.Template
		}
	}
	if datadogEnvironment["DD_ACCESS_TOKEN"] != "${HANDLE}" || datadogEnvironment["DD_SITE"] != "datadoghq.com" {
		t.Fatalf("Datadog Workspace environment = %#v", datadogEnvironment)
	}
	datadogBindings := 0
	for _, binding := range projection.HeaderBindings {
		if binding.ProviderID != BuiltinDatadogProviderID {
			continue
		}
		datadogBindings++
		if binding.Target != (authbroker.BindingTarget{Scheme: "https", Host: "api.datadoghq.com", Port: 443}) ||
			binding.Source.Header != "authorization" || binding.Source.Format != authbroker.SourceFormatBearer ||
			binding.Destination.Header != "authorization" || binding.Destination.Format != authbroker.DestinationFormatBearer ||
			binding.Destination.SecretField != authbroker.CredentialDatadogOAuthSession {
			t.Fatalf("Datadog normalized binding = %#v", binding)
		}
	}
	if datadogBindings != 1 {
		t.Fatalf("Datadog normalized bindings = %#v", projection.HeaderBindings)
	}

	anthropic := providerByID[BuiltinAnthropicProviderID]
	const anthropicAuthTemplate = `{"claudeAiOauth":{"accessToken":"${HANDLE}","refreshToken":"dummy-value","expiresAt":4102444800000,"scopes":${OAUTH_SCOPES_JSON},"subscriptionType":${CLAUDE_SUBSCRIPTION_TYPE_JSON},"rateLimitTier":${CLAUDE_RATE_LIMIT_TIER_JSON}}}`
	const anthropicOnboardingTemplate = `{"hasCompletedOnboarding":true}`
	if anthropic.SchemaVersion != authbroker.ProviderSchemaVersion ||
		anthropic.DisplayName != "Anthropic account for Claude Code" ||
		anthropic.Acquisition != (authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "claude-native-oauth"}) ||
		anthropic.Credential.Kind != authbroker.CredentialAnthropicClaudeOAuthSession || len(anthropic.WorkspaceProjections) != 2 ||
		anthropic.WorkspaceProjections[0] != (authbroker.WorkspaceProjection{
			Kind: authbroker.WorkspaceProjectionCompleteFile, Path: ".claude/.credentials.json", Template: anthropicAuthTemplate,
		}) || anthropic.WorkspaceProjections[1] != (authbroker.WorkspaceProjection{
		Kind: authbroker.WorkspaceProjectionMergeJSON, Path: ".claude.json", Template: anthropicOnboardingTemplate,
	}) {
		t.Fatalf("Anthropic provider plan = %#v", anthropic)
	}
	assertExactBearerBinding(t, projection, BuiltinAnthropicProviderID, "api.anthropic.com", authbroker.CredentialAnthropicClaudeOAuthSession,
		[]string{"authorization"})
	if len(projection.CompleteFiles) != 2 || projection.CompleteFiles[0].ProviderID != BuiltinAnthropicProviderID ||
		projection.CompleteFiles[0].Path != ".claude/.credentials.json" || projection.CompleteFiles[0].Template != anthropicAuthTemplate {
		t.Fatalf("Anthropic complete-file projection = %#v", projection.CompleteFiles)
	}
	if len(projection.JSONMerges) != 1 || projection.JSONMerges[0] != (authbroker.JSONMergeProjection{
		ProviderID: BuiltinAnthropicProviderID, Path: ".claude.json", Template: anthropicOnboardingTemplate,
	}) {
		t.Fatalf("Anthropic JSON-merge projection = %#v", projection.JSONMerges)
	}

	openai := providerByID[BuiltinOpenAIProviderID]
	const openAIAuthTemplate = `{"auth_mode":"chatgptAuthTokens","OPENAI_API_KEY":null,"tokens":{"id_token":"e30.e30.x","access_token":"${HANDLE}","refresh_token":"","account_id":null},"last_refresh":"1970-01-01T00:00:00Z"}`
	if openai.SchemaVersion != authbroker.ProviderSchemaVersion ||
		openai.DisplayName != "OpenAI account for Codex" ||
		openai.Acquisition != (authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "codex-chatgpt-oauth"}) ||
		openai.Credential.Kind != authbroker.CredentialOpenAICodexOAuthSession || len(openai.WorkspaceProjections) != 1 ||
		openai.WorkspaceProjections[0] != (authbroker.WorkspaceProjection{
			Kind: authbroker.WorkspaceProjectionCompleteFile, Path: ".codex/auth.json", Template: openAIAuthTemplate,
		}) {
		t.Fatalf("OpenAI provider plan = %#v", openai)
	}
	assertExactBearerBinding(t, projection, BuiltinOpenAIProviderID, "chatgpt.com", authbroker.CredentialOpenAICodexOAuthSession,
		[]string{"authorization", "chatgpt-account-id", "x-openai-fedramp"})
	if len(projection.CompleteFiles) != 2 || projection.CompleteFiles[1].ProviderID != BuiltinOpenAIProviderID ||
		projection.CompleteFiles[1].Path != ".codex/auth.json" || projection.CompleteFiles[1].Template != openAIAuthTemplate {
		t.Fatalf("OpenAI complete-file projection = %#v", projection.CompleteFiles)
	}
}

func assertExactBearerBinding(
	t *testing.T,
	projection authbroker.Projection,
	providerID, host string,
	credentialKind authbroker.CredentialKind,
	secretHeaders []string,
) {
	t.Helper()
	bindings := 0
	for _, binding := range projection.HeaderBindings {
		if binding.ProviderID != providerID {
			continue
		}
		bindings++
		if binding.Target != (authbroker.BindingTarget{Scheme: "https", Host: host, Port: 443}) ||
			binding.Source != (authbroker.NormalizedBindingSource{Header: "authorization", Format: authbroker.SourceFormatBearer}) ||
			binding.Destination != (authbroker.BindingDestination{
				Header: "authorization", Format: authbroker.DestinationFormatBearer, SecretField: credentialKind,
			}) || !slicesEqual(binding.SecretHeaders, secretHeaders) {
			t.Fatalf("provider %q normalized binding = %#v", providerID, binding)
		}
	}
	if bindings != 1 {
		t.Fatalf("provider %q normalized bindings = %#v", providerID, projection.HeaderBindings)
	}
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type importedHeaderExpectation struct {
	providerID        string
	displayName       string
	environment       map[string]string
	host              string
	sourceHeader      string
	sourceFormat      authbroker.SourceFormat
	destinationHeader string
	destinationFormat authbroker.DestinationFormat
}

func assertImportedHeaderProvider(t *testing.T, projection authbroker.Projection, want importedHeaderExpectation) {
	t.Helper()
	var provider authbroker.Provider
	for _, candidate := range projection.Providers {
		if candidate.ID == want.providerID {
			provider = candidate
			break
		}
	}
	if provider.ID == "" || provider.DisplayName != want.displayName ||
		provider.Acquisition.Mode != authbroker.AcquisitionStdinImport || provider.Acquisition.Helper != "" {
		t.Fatalf("provider %q identity/acquisition = %#v", want.providerID, provider)
	}
	environment := map[string]string{}
	for _, item := range projection.Environment {
		if item.ProviderID == want.providerID {
			environment[item.Name] = item.Template
		}
	}
	if len(environment) != len(want.environment) {
		t.Fatalf("provider %q Workspace environment = %#v", want.providerID, environment)
	}
	for name, template := range want.environment {
		if environment[name] != template {
			t.Fatalf("provider %q Workspace environment = %#v", want.providerID, environment)
		}
	}
	bindings := 0
	for _, binding := range projection.HeaderBindings {
		if binding.ProviderID != want.providerID {
			continue
		}
		bindings++
		if binding.Target.Scheme != "https" || binding.Target.Host != want.host || binding.Target.Port != 443 ||
			binding.Source.Header != want.sourceHeader || binding.Source.Format != want.sourceFormat ||
			binding.Destination.Header != want.destinationHeader || binding.Destination.Format != want.destinationFormat ||
			binding.Destination.SecretField != authbroker.CredentialPrimarySecret ||
			len(binding.SecretHeaders) != 1 || binding.SecretHeaders[0] != want.destinationHeader {
			t.Fatalf("provider %q normalized binding = %#v", want.providerID, binding)
		}
	}
	if bindings != 1 {
		t.Fatalf("provider %q normalized bindings = %#v", want.providerID, projection.HeaderBindings)
	}
}

func TestLoaderLoadsOwnerOnlyUserProvidersAndNormalizesThem(t *testing.T) {
	requireUserProviderSupport(t)
	directory := filepath.Join(t.TempDir(), "providers")
	writeProviderDirectory(t, directory, "synthetic.json", syntheticFixture(t), 0o600)
	loader, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	wantProviders := append(authbroker.ActiveBuiltinProviderIDs(), "example-token")
	slices.Sort(wantProviders)
	gotProviders := make([]string, len(projection.Providers))
	for index, provider := range projection.Providers {
		gotProviders[index] = provider.ID
	}
	if !slices.Equal(gotProviders, wantProviders) {
		t.Fatalf("provider ordering = %#v", projection.Providers)
	}
	if len(projection.CompleteFiles) != 3 || projection.CompleteFiles[0].Path != ".claude/.credentials.json" ||
		projection.CompleteFiles[1].Path != ".codex/auth.json" ||
		projection.CompleteFiles[2].Path != ".config/example/auth.toml" {
		t.Fatalf("complete-file projection = %#v", projection.CompleteFiles)
	}
	wantSecretHeaders := "authorization,chatgpt-account-id,x-api-key,x-chatworktoken,x-openai-fedramp"
	if authbroker.SupportsReviewedLoginProvider(authbroker.BuiltinAWSProviderID) {
		wantSecretHeaders = "authorization,chatgpt-account-id,x-amz-security-token,x-api-key,x-chatworktoken,x-openai-fedramp"
	}
	if strings.Join(projection.SecretHeaders, ",") != wantSecretHeaders {
		t.Fatalf("secret headers = %#v", projection.SecretHeaders)
	}
}

func TestLoaderUsesOnlyInjectedCanonicalDirectory(t *testing.T) {
	if _, err := New("relative/providers"); err == nil {
		t.Fatal("New accepted relative path")
	}
	unclean := t.TempDir() + string(filepath.Separator) + "x" + string(filepath.Separator) + ".." + string(filepath.Separator) + "providers"
	if _, err := New(unclean); err == nil {
		t.Fatal("New accepted non-canonical path")
	}
	directory := filepath.Join(t.TempDir(), "missing")
	loader, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := loader.Load()
	if err != nil {
		t.Fatal(err)
	}
	gotProviders := make([]string, len(projection.Providers))
	for index, provider := range projection.Providers {
		gotProviders[index] = provider.ID
	}
	if !slices.Equal(gotProviders, authbroker.ActiveBuiltinProviderIDs()) {
		t.Fatalf("missing user directory projection = %#v", projection.Providers)
	}
}

func TestLoaderRejectsUnsafeModesAndFileKinds(t *testing.T) {
	requireUserProviderSupport(t)
	cases := map[string]func(t *testing.T, directory string){
		"directory mode": func(t *testing.T, directory string) {
			writeProviderDirectory(t, directory, "synthetic.json", syntheticFixture(t), 0o600)
			if err := os.Chmod(directory, 0o750); err != nil {
				t.Fatal(err)
			}
		},
		"file mode": func(t *testing.T, directory string) {
			writeProviderDirectory(t, directory, "synthetic.json", syntheticFixture(t), 0o644)
		},
		"executable file": func(t *testing.T, directory string) {
			writeProviderDirectory(t, directory, "synthetic.json", syntheticFixture(t), 0o700)
		},
		"json directory": func(t *testing.T, directory string) {
			if err := os.MkdirAll(filepath.Join(directory, "bad.json"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(directory, 0o700); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "providers")
			setup(t, directory)
			loader, err := New(directory)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := loader.Load(); err == nil {
				t.Fatal("Loader.Load accepted unsafe provider storage")
			}
		})
	}
}

func TestLoaderRejectsSymlinkDirectoryAndProvider(t *testing.T) {
	requireUserProviderSupport(t)
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	t.Run("directory", func(t *testing.T) {
		root := t.TempDir()
		realDirectory := filepath.Join(root, "real")
		writeProviderDirectory(t, realDirectory, "synthetic.json", syntheticFixture(t), 0o600)
		link := filepath.Join(root, "providers")
		if err := os.Symlink(realDirectory, link); err != nil {
			t.Fatal(err)
		}
		loader, err := New(link)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := loader.Load(); err == nil {
			t.Fatal("Loader.Load accepted symlinked provider directory")
		}
	})
	t.Run("file", func(t *testing.T) {
		root := t.TempDir()
		directory := filepath.Join(root, "providers")
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, "synthetic.json")
		if err := os.WriteFile(target, syntheticFixture(t), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(directory, "synthetic.json")); err != nil {
			t.Fatal(err)
		}
		loader, err := New(directory)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := loader.Load(); err == nil {
			t.Fatal("Loader.Load accepted symlinked provider file")
		}
	})
}

func TestLoaderRejectsBuiltinOverrideUnreviewedHelperAndCollisions(t *testing.T) {
	requireUserProviderSupport(t)
	fixture := syntheticFixture(t)
	cases := map[string][]byte{
		"builtin override":      bytes.Replace(fixture, []byte(`"id": "example-token"`), []byte(`"id": "github"`), 1),
		"anthropic override":    bytes.Replace(fixture, []byte(`"id": "example-token"`), []byte(`"id": "anthropic"`), 1),
		"aws override":          bytes.Replace(fixture, []byte(`"id": "example-token"`), []byte(`"id": "aws"`), 1),
		"chatwork override":     bytes.Replace(fixture, []byte(`"id": "example-token"`), []byte(`"id": "chatwork"`), 1),
		"datadog override":      bytes.Replace(fixture, []byte(`"id": "example-token"`), []byte(`"id": "datadog"`), 1),
		"openai override":       bytes.Replace(fixture, []byte(`"id": "example-token"`), []byte(`"id": "openai"`), 1),
		"unreviewed helper":     bytes.Replace(fixture, []byte(`"mode": "stdin_import"`), []byte(`"mode": "builtin_helper", "helper": "github-gh"`), 1),
		"builtin env collision": bytes.Replace(fixture, []byte(`"name": "EXAMPLE_TOKEN"`), []byte(`"name": "GH_TOKEN"`), 1),
		"unknown shell field":   bytes.Replace(fixture, []byte(`"mode": "stdin_import"`), []byte(`"mode": "stdin_import", "shell": "env"`), 1),
	}
	schemaV2 := bytes.Replace(fixture, []byte(`"schema_version": 1`), []byte(`"schema_version": 2`), 1)
	schemaV2 = bytes.ReplaceAll(schemaV2, []byte(`"primary_secret"`), []byte(`"static_primary_secret"`))
	cases["owner schema-v2 plan"] = schemaV2
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "providers")
			writeProviderDirectory(t, directory, "candidate.json", document, 0o600)
			loader, err := New(directory)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := loader.Load(); err == nil {
				t.Fatal("Loader.Load accepted unsafe provider document")
			}
		})
	}
}

func TestLoaderIgnoresNonJSONFilesButBoundsJSON(t *testing.T) {
	requireUserProviderSupport(t)
	directory := filepath.Join(t.TempDir(), "providers")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "README.txt"), []byte("not a provider"), 0o644); err != nil {
		t.Fatal(err)
	}
	loader, err := New(directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(); err != nil {
		t.Fatalf("Loader.Load rejected unrelated non-JSON file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "oversized.json"), bytes.Repeat([]byte("x"), authbroker.MaxProviderDocumentBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(); err == nil {
		t.Fatal("Loader.Load accepted oversized provider")
	}
}

func syntheticFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "synthetic-provider-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
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
