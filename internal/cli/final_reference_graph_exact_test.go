package cli

import (
	"reflect"
	"sort"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalCatalogReferenceEdge struct {
	Program  string
	Command  string
	Kind     string
	Endpoint string
}

func TestADR0084WholeCatalogReferenceGraphIsExact(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}

	governedKinds := map[string]struct{}{
		tobari.WorkspaceTemplateReferenceKind:                    {},
		tobari.WorkspaceTemplateRevisionReferenceKind:            {},
		tobari.WorkspaceTemplateChangePlanReferenceKind:          {},
		tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind: {},
		tobari.ContextActivationPlanReferenceKind:                {},
		tobari.InstallationMigrationPlanReferenceKind:            {},
		tobari.ContextReferenceKind:                              {},
		tobari.WorkspaceReferenceKind:                            {},
		tobari.PolicyCandidateKind:                               {},
		tobari.PolicyRuleKind:                                    {},
	}
	var gotProduced, gotConsumed []finalCatalogReferenceEdge
	for _, command := range catalog.registeredCommands() {
		produced := command.ProducedRefs()
		consumed := command.ConsumedRefs()
		if command.Role == RoleUtility && (len(produced) != 0 || len(consumed) != 0) {
			t.Errorf("RoleUtility command %s %q has reference edges: produced=%+v consumed=%+v", command.programName(), command.Path, produced, consumed)
		}
		for _, reference := range produced {
			if _, governed := governedKinds[reference.Kind]; !governed {
				continue
			}
			gotProduced = append(gotProduced, finalCatalogReferenceEdge{Program: command.programName(), Command: command.Path, Kind: reference.Kind, Endpoint: reference.Field})
		}
		for _, reference := range consumed {
			if _, governed := governedKinds[reference.Kind]; !governed {
				continue
			}
			gotConsumed = append(gotConsumed, finalCatalogReferenceEdge{Program: command.programName(), Command: command.Path, Kind: reference.Kind, Endpoint: reference.Argument})
		}
	}
	sortFinalCatalogReferenceEdges(gotProduced)
	sortFinalCatalogReferenceEdges(gotConsumed)

	wantProduced := []finalCatalogReferenceEdge{
		{Program: ProgramName, Command: "installation migration apply", Kind: tobari.InstallationMigrationPlanReferenceKind, Endpoint: "plan_ref"},
		{Program: ProgramName, Command: "installation migration plan", Kind: tobari.InstallationMigrationPlanReferenceKind, Endpoint: "plan_ref"},
		{Program: ProgramName, Command: "template migration apply", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template migration plan", Kind: tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind, Endpoint: "plan_ref"},
		{Program: ProgramName, Command: "template migration plan", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template plan", Kind: tobari.WorkspaceTemplateChangePlanReferenceKind, Endpoint: "plan_ref"},
		{Program: ProgramName, Command: "template plan", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template plan", Kind: tobari.ContextReferenceKind, Endpoint: "contexts[].context_ref"},
		{Program: ProgramName, Command: "template plan", Kind: tobari.WorkspaceReferenceKind, Endpoint: "contexts[].workspace_ref"},
		{Program: ProgramName, Command: "template apply", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template apply", Kind: tobari.WorkspaceTemplateRevisionReferenceKind, Endpoint: "current_revision_ref"},
		{Program: ProgramName, Command: "template create", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template copy", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template list", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "items[].template_ref"},
		{Program: ProgramName, Command: "template show", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template show", Kind: tobari.WorkspaceTemplateRevisionReferenceKind, Endpoint: "current_revision_ref"},
		{Program: ProgramName, Command: "context create", Kind: tobari.ContextReferenceKind, Endpoint: "context_ref"},
		{Program: ProgramName, Command: "context plan", Kind: tobari.ContextActivationPlanReferenceKind, Endpoint: "plan_ref"},
		{Program: ProgramName, Command: "context plan", Kind: tobari.ContextReferenceKind, Endpoint: "context_ref"},
		{Program: ProgramName, Command: "context plan", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "context apply", Kind: tobari.ContextReferenceKind, Endpoint: "context_ref"},
		{Program: ProgramName, Command: "context list", Kind: tobari.ContextReferenceKind, Endpoint: "items[].context_ref"},
		{Program: ProgramName, Command: "context show", Kind: tobari.ContextReferenceKind, Endpoint: "context_ref"},
		{Program: ProgramName, Command: "context enter", Kind: tobari.WorkspaceReferenceKind, Endpoint: "workspace_ref"},
		{Program: ProgramName, Command: "status", Kind: tobari.WorkspaceReferenceKind, Endpoint: "workspace.workspace_ref"},
		{Program: ProgramName, Command: "workspace list", Kind: tobari.WorkspaceReferenceKind, Endpoint: "items[].workspace_ref"},
		{Program: ProgramName, Command: "workspace status", Kind: tobari.WorkspaceReferenceKind, Endpoint: "workspace_ref"},
		{Program: ProgramName, Command: "policy candidates", Kind: tobari.PolicyCandidateKind, Endpoint: "id"},
		{Program: ProgramName, Command: "review permissions", Kind: tobari.PolicyCandidateKind, Endpoint: "id"},
		{Program: ProgramName, Command: "policy rules", Kind: tobari.PolicyRuleKind, Endpoint: "id"},
		{Program: ProgramName, Command: "policy apply-reviewed", Kind: tobari.PolicyRuleKind, Endpoint: "decisions[].rule_id"},
	}
	if buildIdentityHasBroker() {
		wantProduced = append(wantProduced, finalCatalogReferenceEdge{Program: ProgramName, Command: "auth status", Kind: tobari.ContextReferenceKind, Endpoint: "context_ref"})
	}
	sortFinalCatalogReferenceEdges(wantProduced)

	wantConsumed := []finalCatalogReferenceEdge{
		{Program: ProgramName, Command: "installation migration apply", Kind: tobari.InstallationMigrationPlanReferenceKind, Endpoint: "--plan"},
		{Program: ProgramName, Command: "template migration apply", Kind: tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind, Endpoint: "--plan"},
		{Program: ProgramName, Command: "template migration plan", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "template plan", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "template apply", Kind: tobari.WorkspaceTemplateChangePlanReferenceKind, Endpoint: "--plan"},
		{Program: ProgramName, Command: "context plan", Kind: tobari.ContextReferenceKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "context apply", Kind: tobari.ContextActivationPlanReferenceKind, Endpoint: "--plan"},
		{Program: ProgramName, Command: "template default set", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "template delete", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "context create", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "--template"},
		{Program: ProgramName, Command: "template copy", Kind: tobari.WorkspaceTemplateRevisionReferenceKind, Endpoint: "--from"},
		{Program: ProgramName, Command: "context show", Kind: tobari.ContextReferenceKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "context enter", Kind: tobari.ContextReferenceKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "context delete", Kind: tobari.ContextReferenceKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "workspace status", Kind: tobari.WorkspaceReferenceKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "workspace delete", Kind: tobari.WorkspaceReferenceKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "policy allow", Kind: tobari.PolicyCandidateKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "policy assist", Kind: tobari.ContextReferenceKind, Endpoint: "--context"},
		{Program: ProgramName, Command: "policy deny", Kind: tobari.PolicyCandidateKind, Endpoint: "--id"},
		{Program: ProgramName, Command: "policy reset", Kind: tobari.PolicyRuleKind, Endpoint: "--id"},
	}
	if buildIdentityHasBroker() {
		for _, command := range []string{"auth login", "auth import", "auth status", "auth logout"} {
			wantConsumed = append(wantConsumed, finalCatalogReferenceEdge{Program: ProgramName, Command: command, Kind: tobari.ContextReferenceKind, Endpoint: "--context"})
		}
	}
	sortFinalCatalogReferenceEdges(wantConsumed)

	if !reflect.DeepEqual(gotProduced, wantProduced) {
		t.Errorf("ADR 0084 produced-reference graph =\n%+v\nwant\n%+v", gotProduced, wantProduced)
	}
	if !reflect.DeepEqual(gotConsumed, wantConsumed) {
		t.Errorf("ADR 0084 consumed-reference graph =\n%+v\nwant\n%+v", gotConsumed, wantConsumed)
	}

	var templateProducers []finalCatalogReferenceEdge
	for _, edge := range gotProduced {
		if edge.Kind == tobari.WorkspaceTemplateReferenceKind {
			templateProducers = append(templateProducers, edge)
		}
	}
	wantTemplateProducers := []finalCatalogReferenceEdge{
		{Program: ProgramName, Command: "context plan", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template apply", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template copy", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template create", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template list", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "items[].template_ref"},
		{Program: ProgramName, Command: "template migration apply", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template migration plan", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template plan", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
		{Program: ProgramName, Command: "template show", Kind: tobari.WorkspaceTemplateReferenceKind, Endpoint: "template_ref"},
	}
	sortFinalCatalogReferenceEdges(wantTemplateProducers)
	if !reflect.DeepEqual(templateProducers, wantTemplateProducers) {
		t.Errorf("workspace-template producers = %+v, want apply/create/list/show %+v", templateProducers, wantTemplateProducers)
	}
}

func sortFinalCatalogReferenceEdges(edges []finalCatalogReferenceEdge) {
	sort.Slice(edges, func(i, j int) bool {
		left, right := edges[i], edges[j]
		if left.Program != right.Program {
			return left.Program < right.Program
		}
		if left.Command != right.Command {
			return left.Command < right.Command
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.Endpoint < right.Endpoint
	})
}
