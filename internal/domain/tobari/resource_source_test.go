package tobari

import (
	"reflect"
	"testing"
)

func TestWorkspaceTemplateSourceOwnsOnlyCurrentTypedBody(t *testing.T) {
	body := templateBodyFixture("v1/items")
	body.Boundary.DestinationCeiling = ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []ManifestPolicyAuthority{}}
	body.Boundary.MethodPolicy = ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}}
	revision, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := WorkspaceTemplate{SchemaVersion: WorkspaceTemplateSchemaVersion, ID: testTemplateAuthorityID, Name: "tools", Current: revision, Retained: []WorkspaceTemplateRevision{revision}}
	source, err := NewWorkspaceTemplateSource(template)
	if err != nil {
		t.Fatal(err)
	}
	if source.Template.TemplateID != template.ID || source.Template.Name != template.Name || source.Policy.Semantic.Protocols.HTTP.Generic.Allow.Rules[0].Path != "/v1/items" {
		t.Fatalf("source = %+v", source)
	}
	if err := source.ValidateFor(template); err != nil {
		t.Fatal(err)
	}
	stale := authorityDigest("e")
	source.Template.BaseRevision = &stale
	if err := source.ValidateFor(template); err == nil {
		t.Fatal("stale Template base revision accepted")
	}
}

func TestWorkspaceTemplatePolicyV1CompilesClosedTaxonomyAndSemanticSets(t *testing.T) {
	body := templateBodyFixture("v1/items")
	body.Boundary.DestinationCeiling = ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []ManifestPolicyAuthority{}}
	body.Boundary.MethodPolicy = ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{{Method: "DELETE", Decision: ManifestMethodDeny}}}
	body.Policy.BaselineGrants = append(body.Policy.BaselineGrants,
		ManifestPolicyExactRule{Scheme: "https", Host: "other.example.dev", Port: 443, Method: "GET", Path: "/other"},
	)
	source, err := NewWorkspaceTemplateDraftSource(testTemplateAuthorityID, "tools", body)
	if err != nil {
		t.Fatal(err)
	}
	if source.Policy.SchemaVersion != WorkspaceTemplatePolicySchemaVersion || !reflect.DeepEqual(source.Policy.Boundary.Methods.Deny, []string{"DELETE"}) {
		t.Fatalf("V1 source envelope = %+v", source.Policy)
	}
	first, err := source.Body(body.EntryDefaults.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	reordered := source.Clone()
	rules := reordered.Policy.Semantic.Protocols.HTTP.Generic.Allow.Rules
	rules[0], rules[1] = rules[1], rules[0]
	second, err := reordered.Body(body.EntryDefaults.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	firstRevision, _ := semanticIdentity(first)
	secondRevision, _ := semanticIdentity(second)
	if firstRevision != secondRevision {
		t.Fatalf("semantic set reorder changed revision: %s != %s", firstRevision, secondRevision)
	}
	if first.Policy.SemanticModules == nil || first.Policy.SemanticModules.Providers.AWS.Allow.Rules == nil || first.Policy.SemanticModules.Protocols.HTTP.OCI.Deny.Rules == nil {
		t.Fatalf("closed semantic taxonomy was not compiled: %+v", first.Policy.SemanticModules)
	}
	if len(first.Policy.BaselineGrants) != 0 || len(first.Policy.BaselineTemplates) != 0 || len(first.Policy.MCPBaselineGrants) != 0 {
		t.Fatalf("predecessor static fields survived V1 compile: %+v", first.Policy)
	}
}

func TestCompileWorkspaceTemplateBodyV1MatchesFileBackedCompiler(t *testing.T) {
	body := templateBodyFixture("items")
	body.Boundary.DestinationCeiling = ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []ManifestPolicyAuthority{}}
	body.Boundary.MethodPolicy = ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{{Method: "POST", Decision: ManifestMethodDeny}}}
	original := body.Clone()

	compiled, err := CompileWorkspaceTemplateBodyV1(body)
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewWorkspaceTemplateDraftSource(testTemplateAuthorityID, "tools", body)
	if err != nil {
		t.Fatal(err)
	}
	fromSource, err := source.Body(body.EntryDefaults.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(compiled, fromSource) {
		t.Fatalf("built-in compile differs from file-backed compile:\n built-in=%#v\n source=%#v", compiled, fromSource)
	}
	if compiled.Policy.SemanticModules == nil || len(compiled.Policy.BaselineGrants) != 0 || compiled.Policy.BaselineGrants == nil {
		t.Fatalf("compiled V1 body did not close predecessor fields: %#v", compiled.Policy)
	}
	if !reflect.DeepEqual(body, original) || body.Policy.SemanticModules != nil {
		t.Fatal("V1 compile mutated its predecessor input")
	}
}

func TestWorkspaceTemplatePolicyV1RejectsMethodShadowAndUnrepresentableAlpha(t *testing.T) {
	body := templateBodyFixture("v1/items")
	if _, err := NewWorkspaceTemplateDraftSource(testTemplateAuthorityID, "tools", body); err == nil {
		t.Fatal("exact alpha destination ceiling was silently widened")
	}
	body.Boundary.DestinationCeiling = ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []ManifestPolicyAuthority{}}
	body.Boundary.MethodPolicy = ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{{Method: "GET", Decision: ManifestMethodDeny}}}
	if _, err := NewWorkspaceTemplateDraftSource(testTemplateAuthorityID, "tools", body); err == nil {
		t.Fatal("Method-Boundary-shadowed Allow was accepted")
	}
}

func TestWorkspaceTemplatePolicyMigrationRejectsTransportWideAlphaDeny(t *testing.T) {
	body := templateBodyFixture("/")
	body.Policy.BaselineDenies = []ManifestPolicyExactRule{{
		Scheme: "https", Host: "sts.us-east-1.amazonaws.com", Port: 443, Method: "POST", Path: "/",
	}}
	if _, err := migrateAlphaPolicyBody(body.Policy); err == nil {
		t.Fatal("transport-wide alpha Deny was narrowed to generic HTTP during migration")
	}
}

func TestWorkspaceTemplatePolicyMigrationRejectsFinalModulesInsideAlpha(t *testing.T) {
	body := templateBodyFixture("/items")
	modules := EmptyWorkspaceTemplateSemanticModules()
	body.Policy.SemanticModules = &modules
	if _, err := migrateAlphaPolicyBody(body.Policy); err == nil {
		t.Fatal("alpha policy accepted final semantic_modules")
	}
}

func TestContextSourceMatchesExactImmutableBinding(t *testing.T) {
	binding := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: testContextAuthorityID, TemplateID: testTemplateAuthorityID}
	source, err := NewContextSource(binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateFor(binding); err != nil {
		t.Fatal(err)
	}
	source.TemplateID = WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ff")
	if err := source.ValidateFor(binding); err == nil {
		t.Fatal("Context Template rebind accepted")
	}
}
