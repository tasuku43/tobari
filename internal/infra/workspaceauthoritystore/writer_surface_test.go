package workspaceauthoritystore

import (
	"reflect"
	"testing"
)

// The infrastructure Store is not an alternate semantic registry. Public
// mutation methods may publish planned source changes, selection, explicit
// deletion, Memory decisions, and migration only; predecessor fixture writers
// must remain package-private.
func TestMutatorDoesNotExposeDirectActiveTemplateOrContextWriters(t *testing.T) {
	typeOf := reflect.TypeOf((*Mutator)(nil))
	for _, forbidden := range []string{
		"CreateWorkspaceTemplate",
		"InitializeFinalDefaultPair",
		"CopyWorkspaceTemplateByRevisionReference",
		"UpdateWorkspaceTemplateByReference",
		"UpdateWorkspaceTemplateBootstrapByReference",
		"CreateContextByTemplateReference",
	} {
		if method, exists := typeOf.MethodByName(forbidden); exists {
			t.Errorf("direct-active writer %s is publicly reachable: %+v", forbidden, method)
		}
	}
	for _, required := range []string{
		"PlanWorkspaceTemplateSourceByReference",
		"ApplyWorkspaceTemplateSourceByReference",
		"PlanContextSourceByReference",
		"ApplyContextSourceByPlan",
		"PlanInstallationMigration",
		"ApplyInstallationMigration",
	} {
		if _, exists := typeOf.MethodByName(required); !exists {
			t.Errorf("planned publication method %s is unavailable", required)
		}
	}
}
