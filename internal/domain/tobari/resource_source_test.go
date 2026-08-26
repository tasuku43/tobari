package tobari

import "testing"

func TestWorkspaceTemplateSourceOwnsOnlyCurrentTypedBody(t *testing.T) {
	body := templateBodyFixture("v1/items")
	revision, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := WorkspaceTemplate{SchemaVersion: WorkspaceTemplateSchemaVersion, ID: testTemplateAuthorityID, Name: "tools", Current: revision, Retained: []WorkspaceTemplateRevision{revision}}
	source, err := NewWorkspaceTemplateSource(template)
	if err != nil {
		t.Fatal(err)
	}
	if source.Template.TemplateID != template.ID || source.Template.Name != template.Name || source.Policy.Semantic.BaselineGrants[0].Path != "/v1/items" {
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

func TestContextSourceMatchesExactImmutableBinding(t *testing.T) {
	binding := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: testContextAuthorityID, ProjectRoot: "/work/project", TemplateID: testTemplateAuthorityID}
	source, err := NewContextSource(binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := source.ValidateFor(binding); err != nil {
		t.Fatal(err)
	}
	source.ProjectRoot = "/work/other"
	if err := source.ValidateFor(binding); err == nil {
		t.Fatal("Context rebind accepted")
	}
}
