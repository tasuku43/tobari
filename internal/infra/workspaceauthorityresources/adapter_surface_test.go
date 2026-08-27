package workspaceauthorityresources

import (
	"reflect"
	"testing"
)

// The public resources adapter is the composition boundary consumed by the
// CLI. Keeping the Mutator as a named private field is intentional: embedding
// it would silently promote every granular/direct-active method back into the
// public capability surface even when the Catalog no longer registers one.
func TestAdapterExposesNoDirectTemplateOrUnplannedContextWriter(t *testing.T) {
	typeOf := reflect.TypeOf((*Adapter)(nil))
	for _, forbidden := range []string{
		"CreateWorkspaceTemplate",
		"CopyWorkspaceTemplateByRevisionReference",
		"UpdateWorkspaceTemplateByReference",
		"UpdateWorkspaceTemplateBootstrapByReference",
		"CreateContextByTemplateReference",
		"ApplyContextSourceByReference",
	} {
		if method, exists := typeOf.MethodByName(forbidden); exists {
			t.Errorf("direct semantic writer %s is publicly promoted: %+v", forbidden, method)
		}
	}
	for _, required := range []string{
		"CreateWorkspaceTemplateDraft",
		"CopyWorkspaceTemplateDraftByRevisionReference",
		"PlanWorkspaceTemplateSourceByReference",
		"ApplyWorkspaceTemplateSourceByReference",
		"PlanWorkspaceTemplatePolicyMigrationByReference",
		"ApplyWorkspaceTemplatePolicyMigrationByReference",
		"CreateContextDraftByTemplateReference",
		"PlanContextSourceByReference",
		"ApplyContextSourceByPlan",
	} {
		if _, exists := typeOf.MethodByName(required); !exists {
			t.Errorf("required file-backed writer boundary %s is absent", required)
		}
	}
}
