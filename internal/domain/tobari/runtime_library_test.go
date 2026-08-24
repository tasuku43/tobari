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

func TestRuntimeProtectionRejectsBuiltinAndRequiresExactOwnerIdentity(t *testing.T) {
	protection := RuntimeProtection{
		RuntimeID: "018bcfe5-687b-7000-8000-000000000077", RuntimeRevision: "sha256:" + strings.Repeat("b", 64),
		Reason: RuntimeProtectedByTemplateCurrent, WorkspaceTemplateID: "01912345-6789-7abc-8def-0123456789ad",
		TemplateRevision: SemanticDigest("sha256:" + strings.Repeat("a", 64)),
	}
	if err := protection.Validate(); err != nil {
		t.Fatalf("managed Runtime protection rejected: %v", err)
	}
	tests := map[string]func(*RuntimeProtection){
		"builtin Runtime":      func(value *RuntimeProtection) { value.RuntimeID = StandardRuntimeID },
		"malformed Runtime":    func(value *RuntimeProtection) { value.RuntimeID = "standard" },
		"missing Template":     func(value *RuntimeProtection) { value.WorkspaceTemplateID = "" },
		"missing revision":     func(value *RuntimeProtection) { value.TemplateRevision = "" },
		"unexpected Workspace": func(value *RuntimeProtection) { value.WorkspaceID = "01912345-6789-7abc-8def-0123456789ab" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := protection
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid Runtime protection was accepted")
			}
		})
	}
}

func TestRuntimeProtectionInventoryFaultReasonsRemainDistinct(t *testing.T) {
	for _, reason := range []RuntimeProtectionInventoryFaultReason{
		RuntimeProtectionInventoryIncomplete,
		RuntimeProtectionInventoryMigrationUnverified,
		RuntimeProtectionInventoryObservationUnknown,
	} {
		err := RuntimeProtectionInventoryError{Reason: reason}
		if err.Error() == "" || err.Reason != reason {
			t.Fatalf("RuntimeProtectionInventoryError(%q) = %+v", reason, err)
		}
	}
}

func TestRuntimeProtectionInventoryRejectsExactDuplicateButKeepsTemplateRevisionsDistinct(t *testing.T) {
	item := RuntimeProtection{
		RuntimeID: "018bcfe5-687b-7000-8000-000000000077", RuntimeRevision: "sha256:" + strings.Repeat("b", 64),
		Reason: RuntimeProtectedByTemplateRetained, WorkspaceTemplateID: "01912345-6789-7abc-8def-0123456789ad",
		TemplateRevision: SemanticDigest("sha256:" + strings.Repeat("a", 64)),
	}
	duplicate := RuntimeProtectionInventory{Complete: true, Items: []RuntimeProtection{item, item}}
	if err := duplicate.Validate(); err == nil {
		t.Fatal("duplicate Runtime protection evidence was accepted")
	}
	distinct := item
	distinct.TemplateRevision = SemanticDigest("sha256:" + strings.Repeat("c", 64))
	if err := (RuntimeProtectionInventory{Complete: true, Items: []RuntimeProtection{item, distinct}}).Validate(); err != nil {
		t.Fatalf("distinct retained Template revisions rejected: %v", err)
	}
}
