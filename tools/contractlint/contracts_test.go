package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/agentic-cli-foundry/internal/cli"
)

func TestRepositoryContractsMatchDefaultCatalog(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	catalog := cli.DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatalf("DefaultCatalog.Validate() error = %v", err)
	}
	issues, err := inspectContracts(root, catalogCapabilityIDs(catalog))
	if err != nil {
		t.Fatalf("inspectContracts() error = %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("contract issues = %+v", issues)
	}
}

func TestInspectContractsAcceptsPublicCapabilitiesAndSchemaFixture(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, root, capabilitiesPath, []capability{
		{ID: "items.read", Status: "public"},
		{ID: "items.write", Status: "deferred", Reason: "The mutation policy is not selected."},
	})
	fixturePath := "internal/infra/example/testdata/schema.json"
	fixture := []byte("{\"version\":1}\n")
	writeFixture(t, root, fixturePath, fixture)
	writeJSON(t, root, schemasPath, []schemaFixture{{
		ID:         "example.v1",
		Path:       fixturePath,
		SHA256:     digest(fixture),
		Provenance: "A publishable synthetic provider fixture.",
		License:    "CC0-1.0",
	}})

	issues, err := inspectContracts(root, set("items.read"))
	if err != nil {
		t.Fatalf("inspectContracts() error = %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("contract issues = %+v", issues)
	}
}

func TestCapabilitiesRejectCoverageAndLedgerErrors(t *testing.T) {
	entries := []capability{
		{ID: "items.read", Status: "public"},
		{ID: "Items.Write", Status: "public"},
		{ID: "items.read", Status: "public"},
		{ID: "items.internal", Status: "internal"},
		{ID: "items.deferred", Status: "deferred", Reason: "Later milestone."},
		{ID: "items.excluded", Status: "excluded", Reason: "Outside the product thesis."},
		{ID: "items.unknown", Status: "planned", Reason: "Not a supported status."},
	}
	issues := validateCapabilities(entries, set("items.read", "items.internal", "items.missing"))
	assertIssuesContain(t, issues,
		"lowercase dot syntax",
		"duplicate capability id",
		"requires a non-empty reason",
		"non-public capability \"items.internal\" is exposed",
		"status must be public, internal, deferred, or excluded",
		"catalog capability \"items.missing\" is not declared public",
	)
}

func TestCapabilitiesRejectPublicEntryWithoutCatalogExposure(t *testing.T) {
	issues := validateCapabilities([]capability{{ID: "items.read", Status: "public"}}, map[string]struct{}{})
	assertIssuesContain(t, issues, "has no command catalog entry")
}

func TestStrictManifestParsingRejectsUnknownDuplicateAndTrailingJSON(t *testing.T) {
	tests := []struct {
		name string
		path string
		data string
	}{
		{name: "capability unknown field", path: capabilitiesPath, data: `[{"id":"items.read","status":"public","reason":"","command":"items read"}]`},
		{name: "capability duplicate key", path: capabilitiesPath, data: `[{"id":"items.read","id":"items.write","status":"public","reason":""}]`},
		{name: "schema unknown field", path: schemasPath, data: `[{"id":"provider.v1","path":"testdata/schema.json","sha256":"","provenance":"fixture","license":"CC0-1.0","url":"https://example.com"}]`},
		{name: "schema duplicate key", path: schemasPath, data: `[{"id":"provider.v1","path":"testdata/a.json","path":"testdata/b.json","sha256":"","provenance":"fixture","license":"CC0-1.0"}]`},
		{name: "null is not explicit empty", path: schemasPath, data: `null`},
		{name: "object is not array", path: schemasPath, data: `{}`},
		{name: "multiple values", path: schemasPath, data: `[] []`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, test.path, []byte(test.data))
			var err error
			if test.path == capabilitiesPath {
				_, err = loadStrictArray[capability](root, test.path)
			} else {
				_, err = loadStrictArray[schemaFixture](root, test.path)
			}
			if err == nil {
				t.Fatal("strict manifest parsing succeeded")
			}
		})
	}
}

func TestManifestsMustBeRegularFilesWithoutSymbolicLinks(t *testing.T) {
	t.Run("symbolic link", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, ".harness/target.json", []byte("[]\n"))
		link := filepath.Join(root, filepath.FromSlash(capabilitiesPath))
		if err := os.Symlink("target.json", link); err != nil {
			t.Skipf("symbolic links are unavailable: %v", err)
		}
		if _, err := loadStrictArray[capability](root, capabilitiesPath); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("error = %v, want symbolic-link rejection", err)
		}
	})

	t.Run("symbolic link parent", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, root, "real-harness/capabilities.json", []byte("[]\n"))
		if err := os.Symlink("real-harness", filepath.Join(root, ".harness")); err != nil {
			t.Skipf("symbolic links are unavailable: %v", err)
		}
		if _, err := loadStrictArray[capability](root, capabilitiesPath); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("error = %v, want symbolic-link rejection", err)
		}
	})

	t.Run("directory", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, filepath.FromSlash(schemasPath))
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := loadStrictArray[schemaFixture](root, schemasPath); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("error = %v, want regular-file rejection", err)
		}
	})
}

func TestSchemasAllowExplicitEmptyManifest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, schemasPath, []byte("[]\n"))
	entries, err := loadStrictArray[schemaFixture](root, schemasPath)
	if err != nil {
		t.Fatalf("loadStrictArray() error = %v", err)
	}
	if entries == nil || len(entries) != 0 {
		t.Fatalf("entries = %#v, want explicit empty slice", entries)
	}
	if issues := validateSchemas(root, entries); len(issues) != 0 {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestSchemasRejectInvalidMetadataAndDigest(t *testing.T) {
	root := t.TempDir()
	fixturePath := "testdata/provider/schema.json"
	fixture := []byte("{\"version\":1}\n")
	writeFixture(t, root, fixturePath, fixture)
	entries := []schemaFixture{
		{ID: "Provider.V1", Path: fixturePath, SHA256: digest(fixture), Provenance: "fixture", License: "CC0-1.0"},
		{ID: "provider.v1", Path: fixturePath, SHA256: strings.ToUpper(digest(fixture)), Provenance: "", License: ""},
		{ID: "provider.v1", Path: fixturePath, SHA256: strings.Repeat("0", 64), Provenance: "fixture", License: "CC0-1.0"},
	}
	issues := validateSchemas(root, entries)
	assertIssuesContain(t, issues,
		"lowercase dot syntax",
		"duplicate schema id",
		"duplicate schema path",
		"provenance is required",
		"license is required",
		"sha256 must be exactly 64 lowercase hexadecimal characters",
		"sha256 mismatch",
	)
}

func TestSchemaFixturePathsRejectTraversalHiddenNonFixtureAndNonRegularTargets(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "testdata/valid.json", []byte("{}\n"))
	if err := os.MkdirAll(filepath.Join(root, "testdata", "directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "traversal", path: "testdata/../outside.json", want: "canonical repository-relative"},
		{name: "absolute", path: filepath.Join(root, "testdata", "valid.json"), want: "canonical repository-relative"},
		{name: "hidden", path: "testdata/.schema.json", want: "hidden components"},
		{name: "not fixture", path: "schemas/provider.json", want: "below a testdata directory"},
		{name: "missing", path: "testdata/missing.json", want: "cannot be inspected"},
		{name: "directory", path: "testdata/directory", want: "not a regular file"},
		{name: "credential-like", path: "testdata/credentials.json", want: "credential-bearing"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, issues := readSchemaFixture(root, test.path)
			if !messagesContain(issues, test.want) {
				t.Fatalf("issues = %q, want substring %q", issues, test.want)
			}
		})
	}
}

func TestSchemaFixtureRejectsSymbolicLinks(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "testdata/target.json", []byte("{}\n"))
	link := filepath.Join(root, "testdata", "schema.json")
	if err := os.Symlink("target.json", link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	_, issues := readSchemaFixture(root, "testdata/schema.json")
	if !messagesContain(issues, "symbolic link") {
		t.Fatalf("issues = %q", issues)
	}
}

func TestSchemaFixtureDigestIsByteExact(t *testing.T) {
	root := t.TempDir()
	one := []byte("{\"version\":1}\n")
	two := []byte("{\"version\":1}\r\n")
	writeFixture(t, root, "testdata/schema.json", two)
	issues := validateSchemas(root, []schemaFixture{{
		ID:         "provider.v1",
		Path:       "testdata/schema.json",
		SHA256:     digest(one),
		Provenance: "Synthetic fixture.",
		License:    "CC0-1.0",
	}})
	assertIssuesContain(t, issues, "sha256 mismatch")
}

func writeJSON(t *testing.T, root, relative string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, relative, append(data, '\n'))
}

func writeFixture(t *testing.T, root, relative string, data []byte) {
	t.Helper()
	writeFile(t, root, relative, data)
}

func writeFile(t *testing.T, root, relative string, data []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func set(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func assertIssuesContain(t *testing.T, issues []issue, wanted ...string) {
	t.Helper()
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		messages = append(messages, issue.Message)
	}
	for _, substring := range wanted {
		if !messagesContain(messages, substring) {
			t.Errorf("issues = %q, want substring %q", messages, substring)
		}
	}
}

func messagesContain(messages []string, wanted string) bool {
	for _, message := range messages {
		if strings.Contains(message, wanted) {
			return true
		}
	}
	return false
}
