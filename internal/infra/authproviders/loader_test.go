package authproviders

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

func TestBuiltinsPublishesExactToolContracts(t *testing.T) {
	projection, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	if projection.SchemaVersion != authbroker.ProviderSchemaVersion || len(projection.Providers) != 4 {
		t.Fatalf("built-in projection = %#v", projection)
	}
	providers := make(map[string]authbroker.Provider, len(projection.Providers))
	for _, provider := range projection.Providers {
		providers[provider.ID] = provider
	}
	if BuiltinAWSProviderID != "aws" || BuiltinChatworkProviderID != "chatwork" || BuiltinDatadogProviderID != "datadog" {
		t.Fatalf("tool provider IDs = %q, %q, %q", BuiltinAWSProviderID, BuiltinChatworkProviderID, BuiltinDatadogProviderID)
	}
	if BuiltinGitHubProviderID != "github" {
		t.Fatalf("BuiltinGitHubProviderID = %q", BuiltinGitHubProviderID)
	}
	provider := providers[BuiltinGitHubProviderID]
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

	aws := providers[BuiltinAWSProviderID]
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
	assertImportedHeaderProvider(t, projection, importedHeaderExpectation{
		providerID:  BuiltinDatadogProviderID,
		displayName: "Datadog access token for pup",
		environment: map[string]string{
			"DD_ACCESS_TOKEN": "${HANDLE}",
			"DD_SITE":         "datadoghq.com",
		},
		host:              "api.datadoghq.com",
		sourceHeader:      "authorization",
		sourceFormat:      authbroker.SourceFormatBearer,
		destinationHeader: "authorization",
		destinationFormat: authbroker.DestinationFormatBearer,
	})
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
	if len(projection.Providers) != 5 || projection.Providers[0].ID != "aws" ||
		projection.Providers[1].ID != "chatwork" || projection.Providers[2].ID != "datadog" ||
		projection.Providers[3].ID != "example-token" || projection.Providers[4].ID != "github" {
		t.Fatalf("provider ordering = %#v", projection.Providers)
	}
	if len(projection.CompleteFiles) != 1 || projection.CompleteFiles[0].Path != ".config/example/auth.toml" {
		t.Fatalf("complete-file projection = %#v", projection.CompleteFiles)
	}
	if strings.Join(projection.SecretHeaders, ",") != "authorization,x-amz-security-token,x-api-key,x-chatworktoken" {
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
	if len(projection.Providers) != 4 || projection.Providers[0].ID != BuiltinAWSProviderID ||
		projection.Providers[1].ID != BuiltinChatworkProviderID ||
		projection.Providers[2].ID != BuiltinDatadogProviderID || projection.Providers[3].ID != BuiltinGitHubProviderID {
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
		"aws override":          bytes.Replace(fixture, []byte(`"id": "example-token"`), []byte(`"id": "aws"`), 1),
		"chatwork override":     bytes.Replace(fixture, []byte(`"id": "example-token"`), []byte(`"id": "chatwork"`), 1),
		"datadog override":      bytes.Replace(fixture, []byte(`"id": "example-token"`), []byte(`"id": "datadog"`), 1),
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
