package dockerruntime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestAdvancedPolicyReceivesContextNamespaceAndCannotClaimSystemPackages(t *testing.T) {
	t.Parallel()
	item := aggregateContext{
		manifest: tobari.ContextManifest{
			SchemaVersion: tobari.ContextSchemaVersion,
			ID:            "01912345-6789-7abc-8def-0123456789ad",
			Name:          "restricted",
			AgentProfile:  tobari.DefaultProfile,
			PolicyMode:    tobari.ContextPolicyModeAdvanced,
			Image:         tobari.BuiltinImageSelector,
		},
		rego: []byte("package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { input.schema_version == 4; data.tobari.schema_version == 2 }\n"),
	}
	transformed, err := transformContextRego(item)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(transformed, []byte("package tobari.contexts.c0191234567897abc8def0123456789ad.http")) ||
		!bytes.Contains(transformed, []byte("input.schema_version == 5")) ||
		!bytes.Contains(transformed, []byte("data.tobari_contexts[input.principal.context_id]")) {
		t.Fatalf("advanced Context policy was not safely namespaced:\n%s", transformed)
	}
	item.rego = []byte("package tobari.system\n")
	if _, err := transformContextRego(item); err == nil {
		t.Fatal("user policy claimed the system package")
	}
}

func TestAdvancedPolicyMigratesPreviousSourceInputSchema(t *testing.T) {
	item := aggregateContext{
		manifest: tobari.ContextManifest{
			SchemaVersion: tobari.ContextSchemaVersion,
			ID:            "01912345-6789-7abc-8def-0123456789ad",
			Name:          "restricted",
			AgentProfile:  tobari.DefaultProfile,
			PolicyMode:    tobari.ContextPolicyModeAdvanced,
			Image:         tobari.BuiltinImageSelector,
		},
		rego: []byte("package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { input.schema_version == 3; data.tobari.schema_version == 2 }\n"),
	}
	transformed, err := transformContextRego(item)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(transformed, []byte("input.schema_version == 3")) ||
		!bytes.Contains(transformed, []byte("input.schema_version == 5")) {
		t.Fatalf("previous Context input schema was not migrated:\n%s", transformed)
	}
}

func TestAggregateRejectsUnsupportedOrAmbiguousSourceInputSchema(t *testing.T) {
	t.Parallel()
	manifest := tobari.ContextManifest{
		SchemaVersion: tobari.ContextSchemaVersion,
		ID:            "01912345-6789-7abc-8def-0123456789ad",
		Name:          "restricted",
		AgentProfile:  tobari.DefaultProfile,
		PolicyMode:    tobari.ContextPolicyModeAdvanced,
		Image:         tobari.BuiltinImageSelector,
	}
	for _, source := range []string{
		"package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { input.schema_version == 2 }\n",
		"package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { input.schema_version == 3; input.schema_version == 4 }\n",
	} {
		if _, err := transformContextRego(aggregateContext{manifest: manifest, rego: []byte(source)}); err == nil {
			t.Fatalf("unsupported policy source was accepted:\n%s", source)
		}
	}
}

func TestGuidedAggregateUsesCanonicalEvaluatorInsteadOfContextRego(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, paths, err := runtime.resolveContext(tobari.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	regoPath := filepath.Join(paths.PolicyDirectory, "tobari.rego")
	if _, err := os.Lstat(regoPath); !os.IsNotExist(err) {
		t.Fatalf("guided Context unexpectedly owns Rego: %v", err)
	}
	first, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Even if a former installation left a stale Context-local evaluator, it
	// has no authority in guided mode and cannot change the aggregate revision.
	if err := os.WriteFile(regoPath, []byte("package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { input.schema_version == 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != second.Revision || first.PolicyDirectory != second.PolicyDirectory {
		t.Fatalf("stale guided Rego changed aggregate: first=%q second=%q", first.Revision, second.Revision)
	}
	projected, err := os.ReadFile(filepath.Join(second.PolicyDirectory, "guided.rego"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(projected, []byte("input.schema_version == 2")) ||
		!bytes.Contains(projected, []byte("input.schema_version == 5")) ||
		!bytes.Contains(projected, []byte("broker_provider")) {
		t.Fatalf("guided aggregate did not use the current evaluator:\n%s", projected)
	}
}

func TestAggregateRevisionIncludesCredentialSecretContent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, paths, err := runtime.resolveContext(tobari.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(paths.CredentialDirectory, "shared-token")
	if err := os.WriteFile(secret, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision == second.Revision || first.PolicyDirectory == second.PolicyDirectory {
		t.Fatalf("credential secret change reused aggregate revision %q", first.Revision)
	}
	if !strings.HasPrefix(first.PolicyDirectory, runtime.aggregateRoot()) || !strings.HasPrefix(second.PolicyDirectory, runtime.aggregateRoot()) {
		t.Fatalf("aggregate directories escaped owned root: first=%q second=%q", first.PolicyDirectory, second.PolicyDirectory)
	}
}

func TestInvalidContextPolicyDoesNotReplaceKnownGoodAggregate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	knownGood, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(context.Background(), "broken", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeAdvanced); err != nil {
		t.Fatal(err)
	}
	_, paths, err := runtime.resolveContext("broken")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.PolicyDirectory, "tobari.rego"), []byte("package tobari.system\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.buildAggregateProjection(context.Background()); err == nil {
		t.Fatal("invalid Context policy was accepted")
	}
	if _, err := os.Stat(filepath.Dir(knownGood.PolicyDirectory)); err != nil {
		t.Fatalf("known-good aggregate was removed: %v", err)
	}
}
