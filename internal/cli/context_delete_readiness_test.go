package cli

import (
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

func TestContextDeleteCatalogDeclaresFreshDockerReadinessFaults(t *testing.T) {
	spec, ok := DefaultCatalog().Lookup("context delete")
	if !ok {
		t.Fatal("context delete is missing from Catalog")
	}
	for _, code := range []string{
		"docker_cli_unavailable",
		"docker_engine_unavailable",
		"docker_engine_incompatible",
		"docker_context_unavailable",
		"docker_compose_unavailable",
		"invalid_readiness_profile",
		"invalid_readiness_observation",
	} {
		declared := commandErrorByCode(t, spec.Agent.Errors, code)
		if declared.Phase != fault.PhasePrecondition || declared.ChangeState != fault.ChangeNone ||
			len(declared.NextActions) != 1 || declared.NextActions[0].Command != "doctor" {
			t.Fatalf("context delete %s = %#v", code, declared)
		}
	}
}

func TestTemplatePlanningRecoveryCommandsRemainReachableForInvalidDrafts(t *testing.T) {
	for _, path := range []string{"template plan", "template migration plan"} {
		spec, ok := DefaultCatalog().Lookup(path)
		if !ok {
			t.Fatalf("%s is missing from Catalog", path)
		}
		for _, code := range []string{"resource_source_missing", "resource_source_invalid"} {
			declared := commandErrorByCode(t, spec.Agent.Errors, code)
			if len(declared.NextActions) != 1 || declared.NextActions[0].Command != "template list" {
				t.Fatalf("%s %s recovery = %#v", path, code, declared.NextActions)
			}
		}
	}
	migration, _ := DefaultCatalog().Lookup("template migration plan")
	missing := commandErrorByCode(t, migration.Agent.Errors, "template_not_found")
	if missing.Phase != fault.PhaseObservation || missing.ChangeState != fault.ChangeNotApplicable ||
		len(missing.NextActions) != 1 || missing.NextActions[0].Command != "template list" {
		t.Fatalf("template migration plan missing authority fault = %#v", missing)
	}
}
