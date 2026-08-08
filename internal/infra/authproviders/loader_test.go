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

func TestBuiltinsPublishesExactGitHubContract(t *testing.T) {
	projection, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	if projection.SchemaVersion != authbroker.ProviderSchemaVersion || len(projection.Providers) != 1 {
		t.Fatalf("built-in projection = %#v", projection)
	}
	provider := projection.Providers[0]
	if BuiltinGitHubProviderID != "github" {
		t.Fatalf("BuiltinGitHubProviderID = %q", BuiltinGitHubProviderID)
	}
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
	if len(projection.HeaderBindings) != 2 {
		t.Fatalf("GitHub normalized bindings = %#v", projection.HeaderBindings)
	}
	for _, binding := range projection.HeaderBindings {
		if binding.ProviderID != "github" || binding.Target.Scheme != "https" ||
			binding.Target.Host != "api.github.com" || binding.Target.Port != 443 ||
			binding.Source.Header != "authorization" ||
			binding.Destination.Header != "authorization" ||
			binding.Destination.Format != authbroker.DestinationFormatPreserveScheme ||
			binding.Destination.SecretField != authbroker.CredentialPrimarySecret {
			t.Fatalf("GitHub normalized binding = %#v", binding)
		}
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
	if len(projection.Providers) != 2 || projection.Providers[0].ID != "example-token" || projection.Providers[1].ID != "github" {
		t.Fatalf("provider ordering = %#v", projection.Providers)
	}
	if len(projection.CompleteFiles) != 1 || projection.CompleteFiles[0].Path != ".config/example/auth.toml" {
		t.Fatalf("complete-file projection = %#v", projection.CompleteFiles)
	}
	if strings.Join(projection.SecretHeaders, ",") != "authorization,x-api-key" {
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
	if len(projection.Providers) != 1 || projection.Providers[0].ID != BuiltinGitHubProviderID {
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
		"unreviewed helper":     bytes.Replace(fixture, []byte(`"mode": "stdin_import"`), []byte(`"mode": "builtin_helper", "helper": "github-gh"`), 1),
		"builtin env collision": bytes.Replace(fixture, []byte(`"name": "EXAMPLE_TOKEN"`), []byte(`"name": "GH_TOKEN"`), 1),
		"unknown shell field":   bytes.Replace(fixture, []byte(`"mode": "stdin_import"`), []byte(`"mode": "stdin_import", "shell": "env"`), 1),
	}
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
