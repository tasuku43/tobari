package cli

import (
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func finalNoOutputMutationErrors(path, recovery string) []CommandError {
	declared := finalAuthorityMutationErrors(path, recovery)
	result := make([]CommandError, 0, len(declared))
	for _, item := range declared {
		if item.Code != "output_write_failed" && item.Code != "mutation_output_write_failed" {
			result = append(result, item)
		}
	}
	return result
}

func finalDefaultPairEnterSpec() CommandSpec {
	return CommandSpec{
		Path: WorkspaceEntryCommandPath, Summary: "Initialize or enter the canonical current Project's final default pair",
		Args: "[-- <command>...]", Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "workspace.authority", Outcome: "Atomically initialize a fresh default Template and Context when required, then reconcile and enter their exact Workspace",
			Inputs:        []CommandInput{{Name: "command", Source: InputSourceArgument, ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable, Description: "Exact child argv after --.", AllowedValues: []string{}, PositionalOnly: true}},
			Output:        noOutput(),
			Prerequisites: []string{"The canonical current Project root and final-only legacy absence guard are observable exactly."},
			FixedTarget:   fixedCurrentDirectoryTarget(),
			Errors:        finalNoOutputMutationErrors(WorkspaceEntryCommandPath, "status"),
			Mutation:      &MutationContract{TargetKind: tobari.CurrentDirectoryTargetKind, TargetInputs: []string{}, Impact: workspaceauthoritycmd.DefaultPairEnterImpact()},
		},
		handler: runFinalDefaultPairEnter,
	}
}

func finalAppliedEntryFields() []OutputField {
	return []OutputField{
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Applied Context identity."},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Applied Template identity."},
		{Name: "workspace_template_revision", Type: OutputFieldTypeString, Description: "Applied immutable Template revision."},
		{Name: "entry_slice_digest", Type: OutputFieldTypeString, Description: "Applied entry-slice digest."},
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Applied Runtime identity."},
		{Name: "runtime_revision", Type: OutputFieldTypeString, Description: "Applied Runtime revision."},
		{Name: "resolved_spec_revision", Type: OutputFieldTypeString, Description: "Applied resolved runtime specification digest."},
		{Name: "reconciled_at", Type: OutputFieldTypeString, Description: "Confirmed UTC reconciliation time."},
	}
}

func finalDefaultPairStatusSpec() CommandSpec {
	fields := []OutputField{
		{Name: "authority_state", Type: OutputFieldTypeString, Description: "Whether final authority is exact empty or initialized.", Enum: []string{"empty", "initialized"}},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Canonical current Project root."},
		{Name: "default_template_state", Type: OutputFieldTypeString, Description: "Whether an exact default Template is selected.", Enum: []string{"absent", "selected"}},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Selected final Template identity.", Nullable: true},
		{Name: "template_name", Type: OutputFieldTypeString, Description: "Selected Template display name.", Nullable: true},
		{Name: "desired_template_generation", Type: OutputFieldTypeInteger, Description: "Current desired Template generation.", Nullable: true},
		{Name: "desired_template_revision", Type: OutputFieldTypeString, Description: "Current desired Template revision.", Nullable: true},
		{Name: "desired_template_policy_slice_digest", Type: OutputFieldTypeString, Description: "Current desired Template-policy slice digest.", Nullable: true},
		{Name: "active_template_policy_slice_digest", Type: OutputFieldTypeString, Description: "Independently active Template-policy slice digest.", Nullable: true},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Current default-pair Context identity.", Nullable: true},
		{Name: "current_policy_memory_revision", Type: OutputFieldTypeString, Description: "Current Context-owned Policy Memory revision.", Nullable: true},
		{Name: "active_policy_memory_revision", Type: OutputFieldTypeString, Description: "Independently active Policy Memory revision.", Nullable: true},
		{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Current Workspace identity.", Nullable: true},
		{Name: "workspace_ref", Type: OutputFieldTypeString, Description: "Opaque current Workspace reference when a Workspace exists.", ReferenceKind: tobari.WorkspaceReferenceKind, Optional: true},
		{Name: "workspace_home", Type: OutputFieldTypeString, Description: "Owner-only Workspace home.", Nullable: true},
		{Name: "applied_entry", Type: OutputFieldTypeObject, Description: "Independent last-successful AppliedEntry.", Nullable: true, Fields: finalAppliedEntryFields()},
	}
	return CommandSpec{
		Path: "status", Summary: "Inspect the canonical current Project's final default pair",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "workspace.authority", Outcome: "Return desired, independently active, and applied authority for the exact final default Template and canonical Project Context",
			Inputs:        []CommandInput{formatInput()},
			Output:        CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: fields, Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable, JSONEnvelope: "status", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 3},
			Prerequisites: []string{}, Errors: finalAuthorityReadErrors("status", "doctor"),
		},
		handler: runFinalDefaultPairStatus,
	}
}
