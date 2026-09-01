package cli

import (
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func finalReferenceInput(name, description, kind string) CommandInput {
	return CommandInput{Name: name, Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: description, AllowedValues: []string{}, ReferenceKind: kind}
}

func finalAuthorityReadErrors(path, recovery string) []CommandError {
	return readCommandErrors(path, true,
		declaredCommandError(fault.KindRejected, "installation_migration_required", false, "installation migration plan", "Review the exact supported authority.json migration."),
		declaredCommandError(fault.KindRejected, "legacy_state_present", false, "doctor", "Reset or recreate this pre-release installation before initializing final authority."),
		declaredCommandError(fault.KindNotFound, "authority_not_found", false, recovery, "Discover current final authority references."),
		classifiedCommandError(fault.KindContract, "invalid_authority", false, fault.PhaseVerification, fault.ChangeUnknown, recovery, "Repair the final authority envelope."),
		classifiedCommandError(fault.KindInternal, "missing_runtime", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Configure the final authority adapter."))
}

func finalAuthorityMutationErrors(path, recovery string) []CommandError {
	authorityRecovery := "tobari"
	if path == WorkspaceEntryCommandPath {
		authorityRecovery = "status"
	}
	return mutationCommandErrors(path, recovery,
		declaredCommandError(fault.KindRejected, "installation_migration_required", false, "installation migration plan", "Review the exact supported authority.json migration."),
		declaredCommandError(fault.KindInvalidInput, "invalid_template_ref", false, "template list", "Use one exact opaque Workspace Template reference emitted by Template discovery."),
		declaredCommandError(fault.KindInvalidInput, "invalid_runtime_revision_ref", false, "review runtimes", "Use one exact opaque Runtime revision reference emitted by Runtime discovery."),
		classifiedCommandError(fault.KindUnavailable, "final_authority_mutation_recovery_required", false, fault.PhasePrecondition, fault.ChangeNone, "status", "Read and recover the preserved final-authority decision through the exact initiating command; do not remove authority files manually."),
		declaredCommandError(fault.KindRejected, "legacy_state_present", false, "doctor", "Reset or recreate this pre-release installation before initializing final authority."),
		classifiedCommandError(fault.KindNotFound, "authority_not_found", false, fault.PhasePrecondition, fault.ChangeNone, authorityRecovery, "Initialize final authority through the canonical first-use flow."),
		declaredCommandError(fault.KindRejected, "authority_in_use", false, recovery, "Remove the exact dependent final authority first."),
		classifiedCommandError(fault.KindInternal, "missing_runtime", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Configure the final authority mutation adapter."))
}

func finalReadVerificationError(code, recovery, reason string) CommandError {
	return classifiedCommandError(fault.KindContract, code, false, fault.PhaseVerification, fault.ChangeUnknown, recovery, reason)
}

func finalMutationVerificationError(code, recovery, reason string) CommandError {
	return classifiedCommandError(fault.KindContract, code, false, fault.PhaseVerification, fault.ChangeUnknown, recovery, reason)
}

func finalTemplateListErrors(path, recovery string) []CommandError {
	return append(finalAuthorityReadErrors(path, recovery),
		declaredCommandError(fault.KindUnavailable, "template_read_failed", false, recovery, "Read the final Template authority again."),
		declaredCommandError(fault.KindUnavailable, "template_source_read_failed", false, recovery, "Inspect the canonical Template source pair and draft files."),
		finalReadVerificationError("invalid_template_list", recovery, "Repair contradictory Template authority or source projections."))
}

func finalTemplateShowErrors(path, recovery string) []CommandError {
	return append(finalTemplateListErrors(path, recovery),
		classifiedCommandError(fault.KindInvalidInput, "invalid_template_name", false, fault.PhasePrecondition, fault.ChangeNone, recovery, "Use a valid Template display name."),
		declaredCommandError(fault.KindNotFound, "template_not_found", false, recovery, "Discover current Template authority."),
		finalReadVerificationError("invalid_template", recovery, "Repair the selected Template authority or source projection."))
}

func finalContextListErrors(path, recovery string) []CommandError {
	return append(finalAuthorityReadErrors(path, recovery),
		declaredCommandError(fault.KindUnavailable, "context_read_failed", false, recovery, "Read the final Context authority again."),
		declaredCommandError(fault.KindUnavailable, "context_source_read_failed", false, recovery, "Inspect canonical Context sources and drafts."),
		finalReadVerificationError("invalid_context_list", recovery, "Repair contradictory Context authority or source projections."))
}

func finalContextShowErrors(path, recovery string) []CommandError {
	return append(finalContextListErrors(path, recovery),
		classifiedCommandError(fault.KindInvalidInput, "invalid_context_ref", false, fault.PhasePrecondition, fault.ChangeNone, recovery, "Use one exact Context reference from Context discovery."),
		declaredCommandError(fault.KindNotFound, "context_not_found", false, recovery, "Discover current Context authority."),
		finalReadVerificationError("invalid_context", recovery, "Repair the selected Context authority or source projection."))
}

func finalWorkspaceListErrors(path, recovery string) []CommandError {
	return append(finalAuthorityReadErrors(path, recovery),
		declaredCommandError(fault.KindUnavailable, "workspace_read_failed", false, recovery, "Read the final Workspace authority again."),
		finalReadVerificationError("invalid_workspace_list", recovery, "Repair contradictory Workspace authority."))
}

func finalWorkspaceStatusErrors(path, recovery string) []CommandError {
	return append(finalWorkspaceListErrors(path, recovery),
		classifiedCommandError(fault.KindInvalidInput, "invalid_workspace_ref", false, fault.PhasePrecondition, fault.ChangeNone, recovery, "Use one exact Workspace reference from Workspace discovery."),
		declaredCommandError(fault.KindNotFound, "workspace_not_found", false, recovery, "Discover current Workspace authority."),
		finalReadVerificationError("invalid_workspace", recovery, "Repair the selected Workspace authority."))
}

func finalContextEnterErrors(path, recovery string) []CommandError {
	return append(finalAuthorityMutationErrors(path, recovery),
		classifiedCommandError(fault.KindInvalidInput, "invalid_context_ref", false, fault.PhasePrecondition, fault.ChangeNone, recovery, "Use one exact Context reference from Context discovery."),
		classifiedCommandError(fault.KindInvalidInput, "invalid_root", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Use a current directory eligible for one Workspace Project root."),
		classifiedCommandError(fault.KindContract, "invalid_project_root_resolution", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Repair the canonical Project-root resolver before entering a Workspace."),
		declaredCommandError(fault.KindNotFound, "context_not_found", false, recovery, "Discover current Context authority."),
		finalMutationVerificationError("invalid_context_entry_result", recovery, "Reconcile the confirmed Context and Workspace entry authority."),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_attachment_unavailable", false, fault.PhaseAttachment, fault.ChangeConfirmed, "context list", "Discover the confirmed Context authority before another explicit entry."),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_interrupted", false, fault.PhaseMutation, fault.ChangePartial, "help context enter", "Inspect the exact Context entry contract, then repeat it with the same Context reference."),
		classifiedCommandError(fault.KindRejected, "workspace_entry_template_policy_inactive", false, fault.PhasePrecondition, fault.ChangeNone, "cluster status", "Read the current Template policy activation before explicit cluster reconciliation."),
		classifiedCommandError(fault.KindRejected, "workspace_entry_policy_memory_inactive", false, fault.PhasePrecondition, fault.ChangeNone, "context list", "Discover current Context authority before explicit policy reconciliation."),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_repair_required", true, fault.PhasePrecondition, fault.ChangeNone, "tobari", "Use root entry so readiness and the staged recovery flow can reconcile the exact current Workspace."),
		classifiedCommandError(fault.KindUnavailable, "workspace_runtime_preparation_uncertain", false, fault.PhaseMutation, fault.ChangeUnknown, "status", "Read current authority and Runtime material before deciding whether to retry entry."),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_observation_unavailable", false, fault.PhaseObservation, fault.ChangeNotApplicable, "context list", "Discover desired, applied, and active Context authority without reconciling it."),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_busy", true, fault.PhasePrecondition, fault.ChangeNone, "context list", "Read current Context authority, then retry after the blocking Workspace session or exclusive Context Home operation finishes."),
		classifiedCommandError(fault.KindRejected, "workspace_entry_overlap_unsafe", false, fault.PhasePrecondition, fault.ChangeNone, "workspace list", "Inspect the live read-write ancestor Workspace before retrying descendant entry."),
		classifiedCommandError(fault.KindContract, "workspace_entry_overlap_unverified", false, fault.PhasePrecondition, fault.ChangeNone, "workspace list", "Repair contradictory owned work-container mount evidence before retrying entry."),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_cleanup_failed", false, fault.PhaseMutation, fault.ChangePartial, "status", "Inspect the partial Workspace runtime before another entry attempt."),
		classifiedCommandError(fault.KindCanceled, "workspace_entry_canceled", false, fault.PhasePrecondition, fault.ChangeNone, "context list", "Discover current Context authority before deciding whether to enter again."),
	)
}

func finalJSONOutput(envelope string, fields []OutputField, coverage CollectionCoverage) CommandOutput {
	return CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: fields, Delivery: OutputDeliveryComplete, CollectionCoverage: coverage, JSONEnvelope: envelope, JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1}
}

func finalTemplateGraphQLEndpointFields() []OutputField {
	return []OutputField{
		{Name: "scheme", Type: OutputFieldTypeString, Description: "Exact endpoint URL scheme."},
		{Name: "host", Type: OutputFieldTypeString, Description: "Exact endpoint host."},
		{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact endpoint port."},
		{Name: "method", Type: OutputFieldTypeString, Description: "Exact endpoint transport method."},
		{Name: "path", Type: OutputFieldTypeString, Description: "Exact endpoint path."},
	}
}

func finalTemplateGraphQLEndpointOutputField() OutputField {
	return OutputField{Name: "graphql_endpoints", Type: OutputFieldTypeArray, Description: "Bounded exact GraphQL transport endpoints in the immutable Template policy.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One bounded exact GraphQL endpoint.", Fields: finalTemplateGraphQLEndpointFields()}}
}

func finalTemplateListFields() []OutputField {
	return []OutputField{{Name: "items", Type: OutputFieldTypeArray, Description: "Complete final Workspace Template collection.", SemanticScope: "All final Workspace Templates at one coherent observation.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One Workspace Template.", Fields: []OutputField{
		{Name: "lifecycle", Type: OutputFieldTypeString, Description: "Template lifecycle.", Enum: []string{"draft", "active"}},
		{Name: "template_ref", Type: OutputFieldTypeString, Description: "Opaque Workspace Template reference.", ReferenceKind: tobari.WorkspaceTemplateReferenceKind},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact final Template identity."},
		{Name: "name", Type: OutputFieldTypeString, Description: "Template display name."},
		{Name: "generation", Type: OutputFieldTypeInteger, Description: "Current immutable revision generation.", Optional: true},
		{Name: "revision", Type: OutputFieldTypeString, Description: "Current complete-body digest.", Optional: true},
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Current Runtime identity.", Optional: true},
		{Name: "runtime_revision", Type: OutputFieldTypeString, Description: "Current Runtime revision digest.", Optional: true},
		{Name: "source_access", Type: OutputFieldTypeString, Description: "Desired or active direct Project source access: read-only or read-write.", Optional: true},
		{Name: "source_path", Type: OutputFieldTypeString, Description: "Canonical absolute Template source directory file path."},
		{Name: "source_state", Type: OutputFieldTypeString, Description: "Desired/active source state: in_sync, modified, invalid, or missing."},
		{Name: "source_revision", Type: OutputFieldTypeString, Description: "Parsed desired semantic revision, absent when missing or invalid.", Optional: true},
		{Name: "active_revision", Type: OutputFieldTypeString, Description: "Last successfully applied active Template revision.", Optional: true},
		func() OutputField {
			value := finalTemplateGraphQLEndpointOutputField()
			value.Optional = true
			return value
		}(),
	}}}}
}

func finalTemplateShowFields(includeRevisionRef bool) []OutputField {
	fields := []OutputField{
		{Name: "lifecycle", Type: OutputFieldTypeString, Description: "Template lifecycle.", Enum: []string{"draft", "active"}},
		{Name: "template_ref", Type: OutputFieldTypeString, Description: "Opaque Workspace Template reference.", ReferenceKind: tobari.WorkspaceTemplateReferenceKind},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact final Template identity."},
		{Name: "name", Type: OutputFieldTypeString, Description: "Template display name."},
		{Name: "generation", Type: OutputFieldTypeInteger, Description: "Current immutable revision generation.", Optional: true},
		{Name: "revision", Type: OutputFieldTypeString, Description: "Current complete-body digest.", Optional: true},
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Current Runtime identity.", Optional: true},
		{Name: "runtime_revision", Type: OutputFieldTypeString, Description: "Current Runtime revision digest.", Optional: true},
		{Name: "source_access", Type: OutputFieldTypeString, Description: "Desired or active direct Project source access: read-only or read-write.", Optional: true},
		{Name: "source_path", Type: OutputFieldTypeString, Description: "Canonical absolute path of template.yaml; policy.yaml is its closed sibling."},
		{Name: "source_state", Type: OutputFieldTypeString, Description: "Desired/active source state: in_sync, modified, invalid, or missing."},
		{Name: "source_revision", Type: OutputFieldTypeString, Description: "Parsed desired semantic revision, absent when missing or invalid.", Optional: true},
		{Name: "active_revision", Type: OutputFieldTypeString, Description: "Last successfully applied active Template revision.", Optional: true},
		func() OutputField {
			value := finalTemplateGraphQLEndpointOutputField()
			value.Optional = true
			return value
		}(),
		{Name: "policy_slice_digest", Type: OutputFieldTypeString, Description: "Current independent Template-policy slice digest.", Optional: true},
		{Name: "entry_slice_digest", Type: OutputFieldTypeString, Description: "Current entry-authority slice digest.", Optional: true},
	}
	if includeRevisionRef {
		fields = append(fields[:2], append([]OutputField{{Name: "current_revision_ref", Type: OutputFieldTypeString, Description: "Opaque exact current Template revision reference.", ReferenceKind: tobari.WorkspaceTemplateRevisionReferenceKind, Optional: true}}, fields[2:]...)...)
	}
	return fields
}

func finalTemplateMutationFields() []OutputField {
	fields := finalTemplateShowFields(false)
	result := make([]OutputField, 0, len(fields)-1)
	for _, field := range fields {
		if field.Name != "template_ref" {
			result = append(result, field)
		}
	}
	return result
}

func finalTemplateCreateFields() []OutputField {
	return []OutputField{
		{Name: "lifecycle", Type: OutputFieldTypeString, Description: "Resource lifecycle; create always returns draft.", Enum: []string{"draft"}},
		{Name: "template_ref", Type: OutputFieldTypeString, Description: "Opaque draft Template reference.", ReferenceKind: tobari.WorkspaceTemplateReferenceKind},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Stable installation-owned Template ID."},
		{Name: "name", Type: OutputFieldTypeString, Description: "Template display name."},
		{Name: "source_path", Type: OutputFieldTypeString, Description: "Canonical absolute template.yaml path; policy.yaml is its closed sibling."},
		{Name: "source_state", Type: OutputFieldTypeString, Description: "Desired source state; a new draft is modified until planned Apply."},
		{Name: "source_revision", Type: OutputFieldTypeString, Description: "Canonical desired semantic revision."},
		{Name: "source_access", Type: OutputFieldTypeString, Description: "Desired Project source access."},
		finalTemplateGraphQLEndpointOutputField(),
	}
}

func finalContextFields(includeContextRef bool) []OutputField {
	fields := []OutputField{
		{Name: "lifecycle", Type: OutputFieldTypeString, Description: "Context lifecycle.", Enum: []string{"draft", "active"}},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Exact final Context identity."},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact bound Template identity.", Optional: true},
		{Name: "template_name", Type: OutputFieldTypeString, Description: "Bound Template display name.", Optional: true},
		{Name: "source_path", Type: OutputFieldTypeString, Description: "Canonical absolute path of the immutable Context source document."},
		{Name: "source_state", Type: OutputFieldTypeString, Description: "Desired/active source state: in_sync, modified, invalid, or missing."},
		{Name: "source_revision", Type: OutputFieldTypeString, Description: "Parsed Context source identity, or null when missing or invalid.", Nullable: true},
		{Name: "active_revision", Type: OutputFieldTypeString, Description: "Active immutable Context source identity.", Optional: true},
		{Name: "desired_template_generation", Type: OutputFieldTypeInteger, Description: "Current desired immutable Template generation.", Optional: true},
		{Name: "desired_template_revision", Type: OutputFieldTypeString, Description: "Current desired complete Template revision digest.", Optional: true},
		{Name: "desired_template_policy_slice_digest", Type: OutputFieldTypeString, Description: "Current desired Template-policy slice digest.", Optional: true},
		{Name: "active_template_policy_slice_digest", Type: OutputFieldTypeString, Description: "Independently active Template-policy slice digest, or null when inactive.", Nullable: true},
		{Name: "current_policy_memory_revision", Type: OutputFieldTypeString, Description: "Current Context-owned Policy Memory revision.", Optional: true},
		{Name: "active_policy_memory_revision", Type: OutputFieldTypeString, Description: "Independently active Policy Memory revision, or null when inactive.", Nullable: true},
		{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Current Workspace identity when present.", Optional: true},
		{Name: "applied_entry", Type: OutputFieldTypeObject, Description: "Independent last-successful Workspace AppliedEntry, or null before entry.", Nullable: true, Fields: []OutputField{
			{Name: "context_id", Type: OutputFieldTypeString, Description: "Applied Context identity."},
			{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Applied Template identity."},
			{Name: "workspace_template_revision", Type: OutputFieldTypeString, Description: "Applied immutable Template revision."},
			{Name: "entry_slice_digest", Type: OutputFieldTypeString, Description: "Applied entry-slice digest."},
			{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Applied Runtime identity."},
			{Name: "runtime_revision", Type: OutputFieldTypeString, Description: "Applied Runtime revision."},
			{Name: "resolved_spec_revision", Type: OutputFieldTypeString, Description: "Applied resolved runtime specification digest."},
			{Name: "reconciled_at", Type: OutputFieldTypeString, Description: "Confirmed UTC reconciliation time."},
		}},
	}
	if includeContextRef {
		fields = append([]OutputField{{Name: "context_ref", Type: OutputFieldTypeString, Description: "Opaque Context reference.", ReferenceKind: tobari.ContextReferenceKind}}, fields...)
	}
	return fields
}

func finalContextListFields() []OutputField {
	return []OutputField{{Name: "items", Type: OutputFieldTypeArray, Description: "Complete final Context collection.", SemanticScope: "All final Contexts at one coherent observation.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One final Context.", Fields: finalContextFields(true)}}}
}

func finalWorkspaceFields(includeRef bool) []OutputField {
	fields := []OutputField{
		{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Exact final Workspace identity."},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Owning Context identity."},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Bound Template identity."},
		{Name: "template_name", Type: OutputFieldTypeString, Description: "Bound Template display name."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Canonical mounted Project root."},
		{Name: "workspace_home", Type: OutputFieldTypeString, Description: "Exact owner-only Workspace home."},
		{Name: "applied", Type: OutputFieldTypeBoolean, Description: "Whether exact last-successful entry authority exists."},
		{Name: "applied_entry_revision", Type: OutputFieldTypeString, Description: "Exact last-successful entry revision when present.", Optional: true},
	}
	if includeRef {
		fields = append([]OutputField{{Name: "workspace_ref", Type: OutputFieldTypeString, Description: "Opaque Workspace reference.", ReferenceKind: tobari.WorkspaceReferenceKind}}, fields...)
	}
	return fields
}

func finalWorkspaceListFields() []OutputField {
	return []OutputField{{Name: "items", Type: OutputFieldTypeArray, Description: "Complete final Workspace collection.", SemanticScope: "All final Workspaces at one coherent observation.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One final Workspace.", Fields: finalWorkspaceFields(true)}}}
}

func finalTemplateListSpec() CommandSpec {
	errors := finalTemplateListErrors("template list", "doctor")
	return CommandSpec{Path: "template list", Summary: "List final Workspace Templates", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return the exhaustive final Workspace Template collection", Inputs: []CommandInput{formatInput()}, Output: finalJSONOutput("templates", finalTemplateListFields(), CollectionCoverageExhaustive), Prerequisites: []string{}, Errors: errors}, handler: runFinalTemplateList}
}

func finalTemplateShowSpec() CommandSpec {
	errors := finalTemplateShowErrors("template show", "template list")
	return CommandSpec{Path: "template show", Summary: "Inspect one final Workspace Template", Args: "[--name <name>] [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return one final Template and its exact current immutable revision", Inputs: []CommandInput{{Name: "--name", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Read-only Template display-name selector; omission selects the exact default.", AllowedValues: []string{}}, formatInput()}, Output: finalJSONOutput("template", finalTemplateShowFields(true), CollectionCoverageNotApplicable), Prerequisites: []string{}, Errors: errors}, handler: runFinalTemplateShow}
}

func finalTemplateCreateSpec() CommandSpec {
	return CommandSpec{Path: "template create", Summary: "Create one unpublished Workspace Template draft", Args: "--name <name> [--source-access read-only|read-write] [--graphql-endpoint <https-url>] [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Write one fresh Template source draft from the reviewed standard body without active authority", Inputs: []CommandInput{
		{Name: "--name", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Unique Template display name.", AllowedValues: []string{}},
		{Name: "--source-access", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Immutable direct Project source access for this Template.", AllowedValues: []string{"read-only", "read-write"}, DefaultValue: stringPointer("read-write")},
		{Name: "--graphql-endpoint", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: int64Pointer(1), Description: "One exact HTTPS GraphQL endpoint with an explicit port and path; it is bounded by the standard destination and POST method ceilings.", AllowedValues: []string{}},
		formatInput()}, Output: finalJSONOutput("template", finalTemplateCreateFields(), CollectionCoverageNotApplicable), Prerequisites: []string{"The built-in standard Runtime revision is available exactly."}, FixedTarget: &FixedTarget{Kind: tobari.WorkspaceTemplateCatalogTargetKind, ID: tobari.WorkspaceTemplateCatalogTargetID, Description: "This installation's final Workspace Template collection.", Scope: FixedTargetScopeToolLocal}, Errors: append(finalAuthorityMutationErrors("template create", "template list"),
		declaredCommandError(fault.KindInvalidInput, "invalid_template_name", false, "template list", "Use a valid unique Template display name."),
		declaredCommandError(fault.KindInvalidInput, "invalid_template_body", false, "help template create", "Correct the reviewed Template body."),
		classifiedCommandError(fault.KindContract, "invalid_standard_template_body", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Repair the built-in standard Template body contract."),
		declaredCommandError(fault.KindRejected, "template_exists", false, "template list", "Choose another name or inspect the existing Template."),
		finalMutationVerificationError("invalid_template_create_result", "template list", "Reconcile the created Template draft.")), Mutation: &MutationContract{TargetKind: tobari.WorkspaceTemplateCatalogTargetKind, TargetInputs: []string{}, Impact: workspaceauthoritycmd.TemplateCreateImpact()}}, handler: runFinalTemplateCreate}
}

func finalTemplateCopySpec() CommandSpec {
	errors := append(finalAuthorityMutationErrors("template copy", "template list"),
		declaredCommandError(fault.KindInvalidInput, "invalid_template_revision_ref", false, "template show", "Use one exact retained Template revision reference."),
		declaredCommandError(fault.KindInvalidInput, "invalid_template_name", false, "template list", "Use a valid unique Template display name."),
		declaredCommandError(fault.KindNotFound, "template_not_found", false, "template list", "Discover current Template authority."),
		declaredCommandError(fault.KindRejected, "template_exists", false, "template list", "Choose another name or inspect the existing Template."),
		finalMutationVerificationError("invalid_template_copy_result", "template list", "Reconcile the copied Template draft."))
	return CommandSpec{Path: "template copy", Summary: "Copy one immutable Template revision into a draft", Args: "--from <template-revision-ref> --name <name> [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Write one independent unpublished Template source draft from one exact retained revision", Inputs: []CommandInput{finalReferenceInput("--from", "Opaque exact Template revision reference.", tobari.WorkspaceTemplateRevisionReferenceKind), {Name: "--name", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Unique new Template display name.", AllowedValues: []string{}}, formatInput()}, Output: finalJSONOutput("template", finalTemplateCreateFields(), CollectionCoverageNotApplicable), Prerequisites: []string{"The source Template pair is in sync."}, Errors: errors, Mutation: &MutationContract{TargetKind: tobari.WorkspaceTemplateReferenceKind, TargetInputs: []string{"--from"}, ParentInput: "--from", Impact: workspaceauthoritycmd.TemplateCreateImpact()}}, handler: runFinalTemplateCopy}
}

func finalTemplateApplySpec() CommandSpec {
	fields := append([]OutputField{}, finalTemplateShowFields(true)...)
	fields = append(fields, OutputField{Name: "changed", Type: OutputFieldTypeBoolean, Description: "Whether one new active Template authority was published."})
	errors := finalAuthorityMutationErrors("template apply", "template show")
	errors = append(errors,
		declaredCommandError(fault.KindInvalidInput, "invalid_template_change_plan_ref", false, "template list", "Use one exact opaque plan reference emitted by template plan."),
		finalMutationVerificationError("invalid_template_apply_result", "template show", "Reconcile the confirmed Template publication and canonical source state."),
		declaredCommandError(fault.KindRejected, "template_change_plan_stale", false, "template list", "Discover the Template and create a fresh plan after any source, active, Context, Memory, or Workspace drift."),
		declaredCommandError(fault.KindNotFound, "resource_source_missing", false, "template show", "Inspect the canonical source path; missing source never deletes active authority."),
		declaredCommandError(fault.KindInvalidInput, "resource_source_invalid", false, "template show", "Correct the strict source schema or closed file-set diagnostic."),
		declaredCommandError(fault.KindRejected, "resource_source_changed", true, "template show", "Re-read the exact source and active revisions before retrying."),
		declaredCommandError(fault.KindRejected, "resource_source_modified", false, "template show", "Re-read the exact source and active revisions before applying again."),
		classifiedCommandError(fault.KindUnavailable, "resource_source_recovery_required", false, fault.PhaseMutation, fault.ChangePartial, "template show", "Inspect source and active identities before exact recovery."),
	)
	return CommandSpec{Path: "template apply", Summary: "Apply one reviewed Template change plan", Args: "--plan <template-change-plan-ref> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{
		CapabilityID: "workspace.authority", Outcome: "Revalidate one exact Template change plan and publish at most one immutable moving-head Template revision",
		Inputs:        []CommandInput{finalReferenceInput("--plan", "Opaque Template change plan reference emitted by template plan and consumed unchanged.", tobari.WorkspaceTemplateChangePlanReferenceKind), formatInput()},
		Output:        finalJSONOutput("template", fields, CollectionCoverageNotApplicable),
		Prerequisites: []string{"The reviewed plan still exactly matches source bytes, active/base revision, bound Contexts, Policy Memory, running Workspaces, and Runtime authority."}, Errors: errors,
		Mutation: &MutationContract{TargetKind: tobari.WorkspaceTemplateChangePlanReferenceKind, TargetInputs: []string{"--plan"}, TargetIDInput: "--plan", Impact: workspaceauthoritycmd.TemplateApplyImpact()},
	}, handler: runFinalTemplateApply}
}

func finalTemplatePlanSpec() CommandSpec {
	fields := []OutputField{
		{Name: "plan_ref", Type: OutputFieldTypeString, Description: "Opaque exact Template change plan reference.", ReferenceKind: tobari.WorkspaceTemplateChangePlanReferenceKind},
		{Name: "template_ref", Type: OutputFieldTypeString, Description: "Opaque planned Workspace Template reference.", ReferenceKind: tobari.WorkspaceTemplateReferenceKind},
		{Name: "active_revision", Type: OutputFieldTypeString, Description: "Exact active Template revision bound by the plan; absent for a draft.", Optional: true},
		{Name: "active_metadata_revision", Type: OutputFieldTypeString, Description: "Exact active display-metadata revision bound by the plan; absent for a draft.", Optional: true},
		{Name: "base_revision", Type: OutputFieldTypeString, Description: "Exact desired-source base revision bound by the plan; absent for a draft.", Optional: true},
		{Name: "source_fingerprint", Type: OutputFieldTypeString, Description: "Exact template.yaml and policy.yaml byte-pair fingerprint."},
		{Name: "source_revision", Type: OutputFieldTypeString, Description: "Canonical desired Template semantic revision."},
		{Name: "impact", Type: OutputFieldTypeString, Description: "Classified authority impact.", Enum: []string{"widening", "reducing", "mixed", "no-op"}},
		{Name: "diff", Type: OutputFieldTypeObject, Description: "Complete classified Template dimensions.", Fields: []OutputField{
			{Name: "name", Type: OutputFieldTypeBoolean, Description: "Whether display metadata changes."},
			{Name: "boundary", Type: OutputFieldTypeBoolean, Description: "Whether the Method/source/network Boundary changes."},
			{Name: "semantic_policy", Type: OutputFieldTypeBoolean, Description: "Whether static semantic policy changes."},
			{Name: "runtime", Type: OutputFieldTypeBoolean, Description: "Whether exact Runtime binding changes."},
			{Name: "session_defaults", Type: OutputFieldTypeBoolean, Description: "Whether session defaults change."},
			{Name: "workspace_defaults", Type: OutputFieldTypeBoolean, Description: "Whether future-Workspace creation defaults change."},
		}},
		{Name: "contexts", Type: OutputFieldTypeArray, Description: "All bound Context and relevant Memory revision evidence.", SemanticScope: "Every Context bound to the planned Template at one coherent observation.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One bound Context.", Fields: []OutputField{
			{Name: "context_ref", Type: OutputFieldTypeString, Description: "Opaque affected Context reference.", ReferenceKind: tobari.ContextReferenceKind},
			{Name: "policy_memory_revision", Type: OutputFieldTypeString, Description: "Exact relevant Context Policy Memory revision."},
			{Name: "workspace_ref", Type: OutputFieldTypeString, Description: "Opaque running Workspace reference when present.", ReferenceKind: tobari.WorkspaceReferenceKind, Optional: true},
		}}},
		{Name: "affected_context_count", Type: OutputFieldTypeInteger, Description: "Exact bound Context count."},
		{Name: "running_workspace_count", Type: OutputFieldTypeInteger, Description: "Exact bound Contexts with running Workspace authority."},
	}
	errors := append(finalAuthorityReadErrors("template plan", "template list"),
		declaredCommandError(fault.KindUnavailable, "template_plan_read_failed", false, "template list", "Inspect the current Template source and authority before planning again."),
		classifiedCommandError(fault.KindInvalidInput, "invalid_template_ref", false, fault.PhasePrecondition, fault.ChangeNone, "template list", "Use one exact opaque Workspace Template reference emitted by Template discovery."),
		classifiedCommandError(fault.KindNotFound, "template_not_found", false, fault.PhaseObservation, fault.ChangeNotApplicable, "template list", "Discover an active or draft Template reference before planning."),
		classifiedCommandError(fault.KindNotFound, "resource_source_missing", false, fault.PhasePrecondition, fault.ChangeNone, "template show", "Restore the canonical source pair before planning."),
		classifiedCommandError(fault.KindInvalidInput, "resource_source_invalid", false, fault.PhasePrecondition, fault.ChangeNone, "template show", "Correct the strict source schema before planning."),
		classifiedCommandError(fault.KindRejected, "resource_source_modified", false, fault.PhasePrecondition, fault.ChangeNone, "template show", "Rebase the desired source on the exact active revision."),
	)
	return CommandSpec{Path: "template plan", Summary: "Review one exact desired Template change", Args: "--id <template-ref> [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{
		CapabilityID: "workspace.authority", Outcome: "Classify one exact Template source change and bind all Apply-relevant authority without mutation",
		Inputs: []CommandInput{finalReferenceInput("--id", "Opaque Workspace Template reference whose desired source is reviewed.", tobari.WorkspaceTemplateReferenceKind), formatInput()},
		Output: finalJSONOutput("template_change_plan", fields, CollectionCoverageNotApplicable), Prerequisites: []string{"The strict canonical source pair and exact immutable Runtime revision are readable."}, Errors: errors,
	}, handler: runFinalTemplatePlan}
}

func finalTemplateMigrationPlanFields() []OutputField {
	return []OutputField{
		{Name: "plan_ref", Type: OutputFieldTypeString, Description: "Opaque exact non-activating policy source migration plan.", ReferenceKind: tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind},
		{Name: "template_ref", Type: OutputFieldTypeString, Description: "Opaque active Workspace Template reference.", ReferenceKind: tobari.WorkspaceTemplateReferenceKind},
		{Name: "active_revision", Type: OutputFieldTypeString, Description: "Exact active Template revision that remains unchanged."},
		{Name: "source_fingerprint", Type: OutputFieldTypeString, Description: "Exact alpha template.yaml and policy.yaml byte-pair fingerprint."},
		{Name: "target_fingerprint", Type: OutputFieldTypeString, Description: "Exact generated V1 source-pair fingerprint."},
		{Name: "source_schema", Type: OutputFieldTypeString, Description: "Exact predecessor schema.", Enum: []string{tobari.WorkspaceTemplatePolicyAlphaSchemaVersion}},
		{Name: "target_schema", Type: OutputFieldTypeString, Description: "Exact final schema.", Enum: []string{tobari.WorkspaceTemplatePolicySchemaVersion}},
	}
}

func finalTemplateMigrationPlanSpec() CommandSpec {
	errors := append(finalAuthorityReadErrors("template migration plan", "template list"),
		classifiedCommandError(fault.KindInvalidInput, "invalid_template_ref", false, fault.PhasePrecondition, fault.ChangeNone, "template list", "Use an exact active Template reference."),
		classifiedCommandError(fault.KindInvalidInput, "resource_source_invalid", false, fault.PhasePrecondition, fault.ChangeNone, "template show", "The alpha source must be strict, in sync, and losslessly representable in V1."),
		classifiedCommandError(fault.KindNotFound, "resource_source_missing", false, fault.PhasePrecondition, fault.ChangeNone, "template show", "Restore the exact source pair before migration planning."),
	)
	return CommandSpec{Path: "template migration plan", Summary: "Review an alpha policy source migration to V1", Args: "--id <template-ref> [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{
		CapabilityID: "workspace.authority", Outcome: "Bind one in-sync alpha source and its exact non-activating V1 replacement without mutation",
		Inputs:        []CommandInput{finalReferenceInput("--id", "Opaque active Template reference whose source is migrated.", tobari.WorkspaceTemplateReferenceKind), formatInput()},
		Output:        finalJSONOutput("template_policy_migration_plan", finalTemplateMigrationPlanFields(), CollectionCoverageNotApplicable),
		Prerequisites: []string{"The active Template source is in sync and uses the exact supported alpha schema."}, Errors: errors,
	}, handler: runFinalTemplateMigrationPlan}
}

func finalTemplateMigrationApplySpec() CommandSpec {
	fields := []OutputField{
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact migrated Template identity."},
		{Name: "template_ref", Type: OutputFieldTypeString, Description: "Opaque active Workspace Template reference.", ReferenceKind: tobari.WorkspaceTemplateReferenceKind},
		{Name: "active_revision", Type: OutputFieldTypeString, Description: "Exact active revision left unchanged."},
		{Name: "source_fingerprint", Type: OutputFieldTypeString, Description: "Exact resulting V1 source-pair fingerprint."},
		{Name: "changed", Type: OutputFieldTypeBoolean, Description: "Whether the desired source pair was migrated during this reconciliation."},
	}
	errors := append(finalAuthorityMutationErrors("template migration apply", "template show"),
		declaredCommandError(fault.KindInvalidInput, "invalid_template_policy_migration_plan_ref", false, "template list", "Discover the active Template, then create and use one exact migration plan unchanged."),
		declaredCommandError(fault.KindRejected, "template_policy_migration_plan_stale", false, "template list", "Rediscover the active Template before creating a fresh plan after source or authority drift."),
		classifiedCommandError(fault.KindUnavailable, "resource_source_recovery_required", false, fault.PhaseMutation, fault.ChangePartial, "template show", "Inspect the source state, then retry the same exact migration plan to settle source-only publication."),
	)
	return CommandSpec{Path: "template migration apply", Summary: "Apply a reviewed non-activating policy source migration", Args: "--plan <template-policy-migration-plan-ref> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{
		CapabilityID: "workspace.authority", Outcome: "Atomically replace one exact alpha policy.yaml with lossless V1 desired source while leaving active authority unchanged",
		Inputs:        []CommandInput{finalReferenceInput("--plan", "Opaque migration plan emitted by template migration plan and consumed unchanged.", tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind), formatInput()},
		Output:        finalJSONOutput("template_policy_migration", fields, CollectionCoverageNotApplicable),
		Prerequisites: []string{"The reviewed source bytes and active revision are unchanged."}, Errors: errors,
		Mutation: &MutationContract{TargetKind: tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind, TargetInputs: []string{"--plan"}, TargetIDInput: "--plan", Impact: workspaceauthoritycmd.TemplatePolicyMigrationImpact()},
	}, handler: runFinalTemplateMigrationApply}
}

func finalTemplateDefaultSetSpec() CommandSpec {
	return finalTemplateWriteSpec("template default set", "Select the default Workspace Template", runFinalTemplateDefaultSet, workspaceauthoritycmd.TemplateDefaultImpact(), false)
}
func finalTemplateDeleteSpec() CommandSpec {
	return finalTemplateWriteSpec("template delete", "Delete one unused Workspace Template", runFinalTemplateDelete, workspaceauthoritycmd.TemplateDeleteImpact(), true)
}

func finalTemplateWriteSpec(path, summary string, handler commandHandler, impact operation.Impact, confirm bool) CommandSpec {
	inputs := []CommandInput{finalReferenceInput("--id", "Opaque Workspace Template reference.", tobari.WorkspaceTemplateReferenceKind)}
	args := "--id <template-ref>"
	if confirm {
		inputs = append(inputs, CommandInput{Name: "--confirm", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Literal destructive confirmation.", AllowedValues: []string{"delete"}})
		args += " --confirm=delete"
	}
	inputs = append(inputs, formatInput())
	stateField := OutputField{Name: "selected", Type: OutputFieldTypeBoolean, Description: "Whether this exact Template is the confirmed default selection."}
	if confirm {
		stateField = OutputField{Name: "deleted", Type: OutputFieldTypeBoolean, Description: "Always true after confirmed deletion."}
	}
	errors := append(finalAuthorityMutationErrors(path, "template list"),
		declaredCommandError(fault.KindNotFound, "template_not_found", false, "template list", "Discover current Template authority."))
	if confirm {
		errors = append(errors,
			declaredCommandError(fault.KindRejected, "template_in_use", false, "context list", "Remove dependent Contexts and the default selection first."),
			finalMutationVerificationError("invalid_template_delete_result", "template list", "Reconcile the deleted Template authority."))
	} else {
		errors = append(errors, finalMutationVerificationError("invalid_template_default_result", "template list", "Reconcile the default Template selection."))
	}
	return CommandSpec{Path: path, Summary: summary, Args: args + " [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: summary, Inputs: inputs, Output: finalJSONOutput("result", []OutputField{{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact affected Template identity."}, stateField}, CollectionCoverageNotApplicable), Prerequisites: []string{}, Errors: errors, Mutation: &MutationContract{TargetKind: tobari.WorkspaceTemplateReferenceKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: impact}}, handler: handler}
}

func finalContextListSpec() CommandSpec {
	return CommandSpec{Path: "context list", Summary: "List final Context bindings", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return every final Context with exact Template and Policy Memory authority", Inputs: []CommandInput{formatInput()}, Output: finalJSONOutput("contexts", finalContextListFields(), CollectionCoverageExhaustive), Prerequisites: []string{}, Errors: finalContextListErrors("context list", "doctor")}, handler: runFinalContextList}
}
func finalContextShowSpec() CommandSpec {
	return CommandSpec{Path: "context show", Summary: "Inspect one final Context", Args: "--id <context-ref> [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return one exact Context with desired and independently active authority", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque Context reference.", tobari.ContextReferenceKind), formatInput()}, Output: finalJSONOutput("context", finalContextFields(true), CollectionCoverageNotApplicable), Prerequisites: []string{}, Errors: finalContextShowErrors("context show", "context list")}, handler: runFinalContextShow}
}
func finalContextApplySpec() CommandSpec {
	fields := append([]OutputField{}, finalContextFields(true)...)
	fields = append(fields, OutputField{Name: "changed", Type: OutputFieldTypeBoolean, Description: "Whether the reviewed draft became active."})
	errors := finalAuthorityMutationErrors("context apply", "context list")
	errors = append(errors,
		declaredCommandError(fault.KindInvalidInput, "invalid_context_activation_plan_ref", false, "context list", "Use one exact opaque Context plan emitted by context plan."),
		declaredCommandError(fault.KindRejected, "context_activation_plan_stale", false, "context list", "Discover the Context and create a fresh activation plan."),
		declaredCommandError(fault.KindNotFound, "resource_source_missing", false, "context list", "Rediscover the retained Context, then inspect its canonical source path; missing source never deletes active authority."),
		declaredCommandError(fault.KindInvalidInput, "resource_source_invalid", false, "context list", "Rediscover the retained Context and correct the strict context.yaml diagnostic."),
		declaredCommandError(fault.KindRejected, "context_identity_immutable", false, "context list", "Rediscover the current binding; another root or Template requires a fresh Context."),
	)
	return CommandSpec{Path: "context apply", Summary: "Apply one reviewed Context activation plan", Args: "--plan <context-activation-plan-ref> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Revalidate and activate one immutable Context identity with new empty Policy Memory", Inputs: []CommandInput{finalReferenceInput("--plan", "Opaque Context activation plan emitted by context plan and consumed unchanged.", tobari.ContextActivationPlanReferenceKind), formatInput()}, Output: finalJSONOutput("context", fields, CollectionCoverageNotApplicable), Prerequisites: []string{"The reviewed context.yaml and exact Template revision remain unchanged."}, Errors: errors, Mutation: &MutationContract{TargetKind: tobari.ContextActivationPlanReferenceKind, TargetInputs: []string{"--plan"}, TargetIDInput: "--plan", Impact: workspaceauthoritycmd.ContextApplyImpact()}}, handler: runFinalContextApply}
}
func finalContextPlanSpec() CommandSpec {
	fields := []OutputField{
		{Name: "plan_ref", Type: OutputFieldTypeString, Description: "Opaque exact Context activation plan.", ReferenceKind: tobari.ContextActivationPlanReferenceKind},
		{Name: "context_ref", Type: OutputFieldTypeString, Description: "Opaque planned Context reference.", ReferenceKind: tobari.ContextReferenceKind},
		{Name: "source_fingerprint", Type: OutputFieldTypeString, Description: "Exact context.yaml byte fingerprint."},
		{Name: "template_ref", Type: OutputFieldTypeString, Description: "Opaque active Template reference.", ReferenceKind: tobari.WorkspaceTemplateReferenceKind},
		{Name: "template_revision", Type: OutputFieldTypeString, Description: "Exact reviewed Template revision."},
		{Name: "duplicate_binding", Type: OutputFieldTypeBoolean, Description: "Always false for an applicable plan."},
		{Name: "no_op", Type: OutputFieldTypeBoolean, Description: "Whether the same immutable Context is already active."},
		{Name: "source_access", Type: OutputFieldTypeString, Description: "Effective source access."},
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Exact effective Runtime ID."},
		{Name: "runtime_revision", Type: OutputFieldTypeString, Description: "Exact effective Runtime revision."},
		{Name: "boundary_fingerprint", Type: OutputFieldTypeString, Description: "Exact effective Method/source/network Boundary identity."},
		{Name: "policy_slice_digest", Type: OutputFieldTypeString, Description: "Exact effective static Semantic Policy identity."},
		{Name: "new_policy_memory_owner", Type: OutputFieldTypeString, Description: "New empty Policy Memory owner Context ID."},
	}
	errors := append(finalAuthorityReadErrors("context plan", "context list"),
		declaredCommandError(fault.KindUnavailable, "context_plan_read_failed", false, "context list", "Inspect the current Context source and authority before planning again."),
		classifiedCommandError(fault.KindInvalidInput, "invalid_context_ref", false, fault.PhasePrecondition, fault.ChangeNone, "context list", "Use one exact opaque Context reference emitted by Context discovery."),
		declaredCommandError(fault.KindContract, "invalid_context_activation_plan", false, "context list", "Repair the Context planning result before applying it."),
		declaredCommandError(fault.KindNotFound, "resource_source_missing", false, "context list", "Restore context.yaml before planning."),
		declaredCommandError(fault.KindInvalidInput, "resource_source_invalid", false, "context list", "Correct strict context.yaml before planning."),
		declaredCommandError(fault.KindRejected, "context_exists", false, "context list", "Use the existing Context identity."))
	return CommandSpec{Path: "context plan", Summary: "Review one Context activation", Args: "--id <context-ref> [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Bind one exact Context source, active Template revision, and duplicate observation without mutation", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque draft or active Context reference.", tobari.ContextReferenceKind), formatInput()}, Output: finalJSONOutput("context_activation_plan", fields, CollectionCoverageNotApplicable), Prerequisites: []string{"context.yaml and its exact active Template are readable."}, Errors: errors}, handler: runFinalContextPlan}
}
func finalContextCreateSpec() CommandSpec {
	fields := []OutputField{{Name: "lifecycle", Type: OutputFieldTypeString, Description: "Always draft.", Enum: []string{"draft"}}, {Name: "context_ref", Type: OutputFieldTypeString, Description: "Opaque draft Context reference.", ReferenceKind: tobari.ContextReferenceKind}, {Name: "context_id", Type: OutputFieldTypeString, Description: "Stable Context ID."}, {Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Bound active Template ID."}, {Name: "source_path", Type: OutputFieldTypeString, Description: "Canonical context.yaml path."}, {Name: "source_state", Type: OutputFieldTypeString, Description: "Modified until planned Apply."}, {Name: "source_revision", Type: OutputFieldTypeString, Description: "Desired Context identity digest."}}
	errors := append(finalAuthorityMutationErrors("context create", "context list"),
		declaredCommandError(fault.KindNotFound, "resource_source_missing", false, "context list", "Restore the exact resource source."),
		declaredCommandError(fault.KindInvalidInput, "resource_source_invalid", false, "context list", "Correct the exact resource source."),
		declaredCommandError(fault.KindRejected, "context_identity_immutable", false, "context list", "Create a fresh Context for another identity."),
		declaredCommandError(fault.KindNotFound, "template_not_found", false, "template list", "Discover current Template authority."),
		declaredCommandError(fault.KindRejected, "context_exists", false, "context list", "Use the existing Context reference."),
		finalMutationVerificationError("invalid_context_create_result", "context list", "Reconcile the created Context draft."))
	return CommandSpec{Path: "context create", Summary: "Create a location-free draft Context", Args: "--template <template-ref> [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Write one draft context.yaml bound only to an active Template", Inputs: []CommandInput{finalReferenceInput("--template", "Opaque parent active Workspace Template reference.", tobari.WorkspaceTemplateReferenceKind), formatInput()}, Output: finalJSONOutput("context", fields, CollectionCoverageNotApplicable), Prerequisites: []string{"The exact active Template is available."}, Errors: errors, Mutation: &MutationContract{TargetKind: tobari.ContextReferenceKind, TargetInputs: []string{"--template"}, ParentInput: "--template", Impact: workspaceauthoritycmd.ContextCreateImpact()}}, handler: runFinalContextCreate}
}
func finalContextEnterSpec() CommandSpec {
	output := CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields:   []OutputField{{Name: "workspace_ref", Type: OutputFieldTypeString, Description: "Opaque resulting Workspace reference.", ReferenceKind: tobari.WorkspaceReferenceKind}, {Name: "workspace_id", Type: OutputFieldTypeString, Description: "Exact resulting Workspace identity."}, {Name: "context_id", Type: OutputFieldTypeString, Description: "Exact owning Context identity."}, {Name: "exit_code", Type: OutputFieldTypeInteger, Description: "Authoritative child exit code."}},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable, JSONEnvelope: "entry", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1}
	return CommandSpec{Path: "context enter", Summary: "Enter one explicit final Context", Args: "--id <context-ref> [--format text|json] [-- <command>...]", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Reconcile and enter one exact Context Workspace", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque Context parent reference.", tobari.ContextReferenceKind), formatInput(), {Name: "command", Source: InputSourceArgument, ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable, Description: "Exact child argv after --.", AllowedValues: []string{}, PositionalOnly: true}}, Output: output, Prerequisites: []string{"The exact final Context, Runtime, and cluster settlement authorities are available without a conflicting lifecycle or session owner."}, Errors: finalContextEnterErrors("context enter", "context list"), Mutation: &MutationContract{TargetKind: tobari.WorkspaceReferenceKind, TargetInputs: []string{"--id"}, ParentInput: "--id", Impact: workspaceauthoritycmd.ContextEnterImpact()}}, handler: runFinalContextEnter}
}
func finalContextDeleteSpec() CommandSpec {
	errors := append(finalAuthorityMutationErrors("context delete", "context list"),
		declaredCommandError(fault.KindInvalidInput, "invalid_context_ref", false, "context list", "Use one exact Context reference from Context discovery."),
		declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Discover current Context authority."),
		declaredCommandError(fault.KindRejected, "context_in_use", false, "context list", "Remove the exact blocking Workspace or attachment first."),
		finalMutationVerificationError("invalid_context_delete_result", "context list", "Reconcile the deleted Context authority."))
	return CommandSpec{Path: "context delete", Summary: "Delete one empty final Context", Args: "--id <context-ref> --confirm=delete [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Delete one exact Context, its Policy Memory, and unresolved candidates", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque Context reference.", tobari.ContextReferenceKind), {Name: "--confirm", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Literal destructive confirmation.", AllowedValues: []string{"delete"}}, formatInput()}, Output: finalJSONOutput("result", []OutputField{{Name: "context_id", Type: OutputFieldTypeString, Description: "Deleted Context identity."}, {Name: "deleted", Type: OutputFieldTypeBoolean, Description: "Always true after confirmed deletion."}}, CollectionCoverageNotApplicable), Prerequisites: []string{"No Workspace, live attachment, or research credential remains."}, Errors: errors, Mutation: &MutationContract{TargetKind: tobari.ContextReferenceKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: workspaceauthoritycmd.ContextDeleteImpact()}}, handler: runFinalContextDelete}
}

func finalWorkspaceListSpec() CommandSpec {
	return CommandSpec{Path: "workspace list", Summary: "List final Workspaces", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return every final Workspace and its exact owner binding", Inputs: []CommandInput{formatInput()}, Output: finalJSONOutput("workspaces", finalWorkspaceListFields(), CollectionCoverageExhaustive), Prerequisites: []string{}, Errors: finalWorkspaceListErrors("workspace list", "doctor")}, handler: runFinalWorkspaceList}
}
func finalWorkspaceStatusSpec() CommandSpec {
	return CommandSpec{Path: "workspace status", Summary: "Inspect one final Workspace", Args: "--id <workspace-ref> [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return one exact Workspace and its applied authority", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque Workspace reference.", tobari.WorkspaceReferenceKind), formatInput()}, Output: finalJSONOutput("workspace", finalWorkspaceFields(true), CollectionCoverageNotApplicable), Prerequisites: []string{}, Errors: finalWorkspaceStatusErrors("workspace status", "workspace list")}, handler: runFinalWorkspaceStatus}
}
func finalWorkspaceDeleteSpec() CommandSpec {
	errors := append(finalAuthorityMutationErrors("workspace delete", "workspace list"),
		declaredCommandError(fault.KindInvalidInput, "invalid_workspace_ref", false, "workspace list", "Use one exact Workspace reference from Workspace discovery."),
		declaredCommandError(fault.KindNotFound, "workspace_not_found", false, "workspace list", "Discover current Workspace authority."),
		declaredCommandError(fault.KindRejected, "workspace_attached", false, "workspace list", "Leave the exact Workspace or explicitly confirm forced cleanup."),
		finalMutationVerificationError("invalid_workspace_delete_result", "workspace list", "Reconcile the deleted Workspace authority."))
	return CommandSpec{Path: "workspace delete", Summary: "Delete one exact final Workspace", Args: "--id <workspace-ref> --confirm=delete [--force] [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Retire one exact Workspace, home, native auth, and owned runtime resources while preserving Context Policy Memory", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque Workspace reference.", tobari.WorkspaceReferenceKind), {Name: "--confirm", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Literal destructive confirmation.", AllowedValues: []string{"delete"}}, {Name: "--force", Source: InputSourceFlag, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Retire the exact live target session and owned container; missing, foreign, ambiguous, or unrelated live owners remain blocking.", AllowedValues: []string{}, DefaultValue: stringPointer("false")}, formatInput()}, Output: finalJSONOutput("result", []OutputField{{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Deleted Workspace identity."}, {Name: "deleted", Type: OutputFieldTypeBoolean, Description: "Always true after confirmed deletion."}}, CollectionCoverageNotApplicable), Prerequisites: []string{"The exact target and canonical attachment authority can be observed without ambiguity."}, Errors: errors, Mutation: &MutationContract{TargetKind: tobari.WorkspaceReferenceKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: workspaceauthoritycmd.WorkspaceDeleteImpact()}}, handler: runFinalWorkspaceDelete}
}
