package tobari

import (
	"strings"
	"testing"
	"time"
)

func TestRuntimeManifestRequiresContiguousUniqueImmutableRevisions(t *testing.T) {
	revision := RuntimeRevision{Ordinal: 1, Revision: "sha256:" + strings.Repeat("a", 64), Image: "tobari-runtime-tools:aaaaaaaaaaaa", ImageDigest: "sha256:" + strings.Repeat("b", 64), CreatedAt: time.Unix(1, 0).UTC(), SnapshotPath: "/tmp/runtimes/tools/revisions/a/source"}
	manifest := RuntimeManifest{SchemaVersion: RuntimeSchemaVersion, ID: "018bcfe5-687b-7000-8000-000000000077", Name: "tools", Kind: RuntimeKindManaged, SourcePath: "/tmp/runtimes/tools/source", Revisions: []RuntimeRevision{revision}}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}

	invalid := manifest
	invalid.Revisions = append([]RuntimeRevision(nil), manifest.Revisions...)
	invalid.Revisions[0].Ordinal = 2
	if err := invalid.Validate(); err == nil {
		t.Fatal("non-contiguous ordinal accepted")
	}

	invalid = manifest
	invalid.Revisions = append(invalid.Revisions, revision)
	invalid.Revisions[1].Ordinal = 2
	if err := invalid.Validate(); err == nil {
		t.Fatal("duplicate semantic revision accepted")
	}
}

func TestRuntimeSelectionIsHumanSyntaxNotAuthority(t *testing.T) {
	name, ordinal, err := ParseRuntimeSelection("frontend@4")
	if err != nil || name != "frontend" || ordinal != 4 {
		t.Fatalf("selection = %q/%d/%v", name, ordinal, err)
	}
	for _, invalid := range []string{"frontend", "@4", "frontend@0", "frontend@latest", "../frontend@1"} {
		if _, _, err := ParseRuntimeSelection(invalid); err == nil {
			t.Errorf("invalid selection %q accepted", invalid)
		}
	}
}

func TestRuntimeSourceBaseAcceptsOnlyStandardOrManagedNameSyntax(t *testing.T) {
	for _, value := range []string{StandardRuntimeName, "frontend", "project-tools"} {
		base, err := ParseRuntimeCopySource(value)
		if err != nil || string(base) != value {
			t.Fatalf("ParseRuntimeCopySource(%q) = %q/%v", value, base, err)
		}
	}
	for _, value := range []string{"", "frontend@1", "../frontend", "standard@1"} {
		if _, err := ParseRuntimeCopySource(value); err == nil {
			t.Errorf("invalid Runtime source Base %q accepted", value)
		}
	}
}

func TestRuntimeBindingRequiresStableIDAndSemanticRevision(t *testing.T) {
	binding := RuntimeBinding{RuntimeID: "018bcfe5-687b-7000-8000-000000000077", Name: "frontend", Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 4, Image: "tobari-runtime-frontend:aaaaaaaaaaaa"}
	if err := binding.Validate(); err != nil {
		t.Fatal(err)
	}
	binding.Revision = "frontend@4"
	if err := binding.Validate(); err == nil {
		t.Fatal("presentation revision accepted as authority")
	}
}
