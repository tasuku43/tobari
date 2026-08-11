package projectconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testProject() Project {
	return Project{
		Name:             "Example Tool",
		BinaryName:       "example-tool",
		GoModule:         "example.com/example/tool",
		GitHubOwner:      "example",
		GitHubRepository: "example-tool",
		Description:      "Example tool.",
		FormulaClass:     "ExampleTool",
		LicenseSPDX:      "MIT",
		SecurityContact:  "security@example.com",
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"schema_version":3,"project":{},"public_guard":{},"unknown":true}`
	if err := os.WriteFile(filepath.Join(root, ".harness", "project.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("error = %v", err)
	}
}

func TestLoadRejectsUnsupportedSchemaWithoutInferringState(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"schema_version":2,"project":{"name":"Example Tool","binary_name":"example-tool","go_module":"example.com/example/tool","github_owner":"example","github_repository":"example-tool","description":"Example tool.","formula_class":"ExampleTool","license_spdx":"MIT","security_contact":"security@example.com"},"public_guard":{"documentation_locale":"en","denylist_file":".harness/denylist.txt","required_paths":[]}}`
	if err := os.WriteFile(filepath.Join(root, ".harness", "project.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(root)
	if err == nil || !strings.Contains(err.Error(), "schema_version = 2, want 1") {
		t.Fatalf("unsupported schema error = %v", err)
	}
}

func TestWriteAndLoadPreserveExplicitNonEnglishLocale(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".harness"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := Config{
		SchemaVersion: SchemaVersion,
		Project:       testProject(),
		PublicGuard: PublicGuard{
			DocumentationLocale: "ja",
			DenylistFile:        ".harness/denylist.txt",
			Required:            []string{},
		},
	}
	if err := Write(root, config); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PublicGuard.DocumentationLocale != "ja" {
		t.Fatalf("documentation locale = %q", loaded.PublicGuard.DocumentationLocale)
	}
}

func TestConfigRejectsWindowsReservedBinaryNames(t *testing.T) {
	config := Config{
		SchemaVersion: SchemaVersion,
		Project:       testProject(),
		PublicGuard:   PublicGuard{DocumentationLocale: "en", DenylistFile: ".harness/denylist.txt"},
	}
	for _, name := range []string{"con", "aux", "prn", "nul", "com1", "com9", "lpt1", "lpt9", "Con", "cOm1", "LpT9"} {
		config.Project.BinaryName = name
		if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "reserved Windows device") {
			t.Fatalf("Validate() accepted binary_name %q: %v", name, err)
		}
	}
	for _, name := range []string{"console", "auxiliary", "null", "com0", "com10", "lpt0", "lpt10"} {
		config.Project.BinaryName = name
		if err := config.Validate(); err != nil {
			t.Fatalf("Validate() rejected binary_name %q: %v", name, err)
		}
	}
	config.Project.BinaryName = "license"
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "release archive LICENSE") {
		t.Fatalf("Validate() accepted release-support collision: %v", err)
	}
	config.Project.BinaryName = strings.Repeat("a", maximumBinaryNameBytes)
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate() rejected maximum portable release basename: %v", err)
	}
	config.Project.BinaryName = strings.Repeat("a", maximumBinaryNameBytes+1)
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "at most 96 bytes") {
		t.Fatalf("Validate() accepted overlong release basename: %v", err)
	}
	for _, name := range []string{"Con", "cOm1", "LpT9"} {
		if !isWindowsReservedBaseName(name) {
			t.Fatalf("isWindowsReservedBaseName(%q) = false", name)
		}
	}
}

func TestConfigRequiresExplicitDocumentationLocale(t *testing.T) {
	config := Config{
		SchemaVersion: SchemaVersion,
		Project:       testProject(),
		PublicGuard: PublicGuard{
			DocumentationLocale: "en",
			DenylistFile:        ".harness/denylist.txt",
		},
	}
	for _, locale := range []string{"en", "ja", "pt-BR"} {
		config.PublicGuard.DocumentationLocale = locale
		if err := config.Validate(); err != nil {
			t.Fatalf("Validate() rejected locale %q: %v", locale, err)
		}
	}
	for _, locale := range []string{"", "English", "en_US", "-ja", "ja-"} {
		config.PublicGuard.DocumentationLocale = locale
		if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "documentation_locale") {
			t.Fatalf("Validate() accepted locale %q: %v", locale, err)
		}
	}
}
