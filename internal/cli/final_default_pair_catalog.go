package cli

import (
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
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

func statusHomeReadErrors() []CommandError {
	return append(finalAuthorityReadErrors("status", "doctor"),
		declaredCommandError(fault.KindUnavailable, "status_observation_failed", true, "status", "Retry one bounded read-only status snapshot."),
		declaredCommandError(fault.KindContract, "invalid_status_snapshot", false, "doctor", "Repair contradictory status authority or observation evidence."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the status snapshot composition root."))
}

func finalDefaultPairEnterErrors() []CommandError {
	return append(finalNoOutputMutationErrors(WorkspaceEntryCommandPath, "status"),
		classifiedCommandError(fault.KindRejected, "first_use_interactive_required", false, fault.PhasePrecondition, fault.ChangeNone, "help tobari", "Open an interactive terminal in the Project, then run tobari."),
		classifiedCommandError(fault.KindContract, "invalid_first_use_draft", false, fault.PhasePrecondition, fault.ChangeNone, "help tobari", "Inspect the root first-use contract."),
		classifiedCommandError(fault.KindInternal, "first_use_review_failed", false, fault.PhasePrecondition, fault.ChangeNone, "help tobari", "Open an interactive terminal and retry the reviewed first-use flow."),
		classifiedCommandError(fault.KindContract, "invalid_first_use_action", false, fault.PhasePrecondition, fault.ChangeNone, "help tobari", "Inspect the root first-use contract."),
		classifiedCommandError(fault.KindUnavailable, "docker_cli_unavailable", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Inspect generic Docker readiness."),
		classifiedCommandError(fault.KindUnavailable, "docker_engine_unavailable", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Start the selected engine externally, then retry root entry."),
		classifiedCommandError(fault.KindUnavailable, "docker_context_unavailable", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Inspect the selected Docker context."),
		classifiedCommandError(fault.KindUnavailable, "docker_compose_unavailable", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Inspect Docker Compose v2 readiness."),
		classifiedCommandError(fault.KindUnsupported, "docker_engine_incompatible", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Use Docker Engine 24 or newer."),
		classifiedCommandError(fault.KindContract, "invalid_readiness_profile", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Repair the closed Workspace readiness profile."),
		classifiedCommandError(fault.KindContract, "invalid_readiness_observation", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Repair the generic Docker readiness observation."),
		classifiedCommandError(fault.KindUnavailable, "default_pair_read_failed", false, fault.PhaseObservation, fault.ChangeNotApplicable, "status", "Observe the current default Template and Context pair."),
		classifiedCommandError(fault.KindRejected, "default_pair_changed", false, fault.PhasePrecondition, fault.ChangeNone, "status", "Observe the current default pair before retrying."),
		classifiedCommandError(fault.KindRejected, "default_template_required", false, fault.PhasePrecondition, fault.ChangeNone, "template list", "Discover a Template reference, then select it with template default set."),
		classifiedCommandError(fault.KindInvalidInput, "invalid_template_body", false, fault.PhasePrecondition, fault.ChangeNone, "template create", "Review a valid Workspace Template body."),
		classifiedCommandError(fault.KindContract, "invalid_default_pair_initialization", false, fault.PhaseVerification, fault.ChangeUnknown, "status", "Reconcile the default-pair publication before retrying."),
		classifiedCommandError(fault.KindContract, "invalid_default_pair", false, fault.PhaseVerification, fault.ChangeUnknown, "status", "Inspect the current default pair before retrying."),
		classifiedCommandError(fault.KindUnavailable, "default_pair_initialized", false, fault.PhaseMutation, fault.ChangePartial, "status", "Observe the confirmed default pair before entering again."),
		classifiedCommandError(fault.KindContract, "invalid_catalog", false, fault.PhasePrecondition, fault.ChangeNone, "help cluster up", "Repair the Catalog-owned cluster activation contract."),
		declaredCommandError(fault.KindUnavailable, "cluster_reconcile_interrupted", false, "cluster status", "Inspect the retained final activation decision."),
		classifiedCommandError(fault.KindContract, "invalid_cluster_reconciliation_result", false, fault.PhaseVerification, fault.ChangeUnknown, "cluster status", "Inspect final authority and component state."),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_attachment_unavailable", false, fault.PhaseAttachment, fault.ChangeConfirmed, "status", "Read the confirmed desired, applied, and active authority before another explicit entry."),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_interrupted", false, fault.PhaseMutation, fault.ChangePartial, "status", "Read the preserved last-successful entry and active recovery authority."),
		classifiedCommandError(fault.KindRejected, "workspace_entry_template_policy_inactive", false, fault.PhasePrecondition, fault.ChangeNone, "status", "Read the current Template policy activation before explicit cluster reconciliation."),
		classifiedCommandError(fault.KindRejected, "workspace_entry_policy_memory_inactive", false, fault.PhasePrecondition, fault.ChangeNone, "status", "Read current and active Policy Memory authority before explicit policy reconciliation."),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_observation_unavailable", false, fault.PhaseObservation, fault.ChangeNotApplicable, "status", "Read desired, applied, and active authority without reconciling it."),
		classifiedCommandError(fault.KindCanceled, "workspace_entry_canceled", false, fault.PhasePrecondition, fault.ChangeNone, "status", "Read current authority before deciding whether to enter again."),
		classifiedCommandError(fault.KindRejected, "runtime_not_ready", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Choose one exact ready Runtime revision."),
		classifiedCommandError(fault.KindRejected, "runtime_retirement_observation_unknown", false, fault.PhaseObservation, fault.ChangeNotApplicable, "doctor", "Inspect the host Runtime lifecycle state."),
		classifiedCommandError(fault.KindContract, "invalid_runtime_list", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Review the current Runtime authority catalog."),
		classifiedCommandError(fault.KindInvalidInput, "invalid_runtime_ref", false, fault.PhasePrecondition, fault.ChangeNone, "review runtimes", "Choose one exact ready Runtime reference."),
		classifiedCommandError(fault.KindInternal, "missing_runtime", false, fault.PhasePrecondition, fault.ChangeNone, "doctor", "Configure the root Workspace-entry composition."),
	)
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
			Errors:        finalDefaultPairEnterErrors(),
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
		{Name: "task", Type: OutputFieldTypeString, Description: "Exact CWD status task.", Enum: []string{tobari.TaskStatusHome}},
		{Name: "authority_state", Type: OutputFieldTypeString, Description: "Whether final authority is exact empty or initialized.", Enum: []string{"empty", "initialized"}},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Nearest canonical Project root resolved before selection."},
		{Name: "default_template_state", Type: OutputFieldTypeString, Description: "Whether the installation default Template is selected.", Enum: []string{"absent", "selected"}},
		{Name: "template", Type: OutputFieldTypeObject, Description: "Current desired default Template authority.", Nullable: true, Fields: statusHomeTemplateFields()},
		{Name: "context", Type: OutputFieldTypeObject, Description: "Selected Context's independent active axes.", Nullable: true, Fields: statusHomeContextFields()},
		{Name: "workspace", Type: OutputFieldTypeObject, Description: "Selected Workspace applied and observed facts.", Fields: statusHomeWorkspaceFields()},
		{Name: "runtime", Type: OutputFieldTypeObject, Description: "Exact desired Runtime revision and local material facts.", Fields: statusHomeRuntimeFields()},
		{Name: "cluster", Type: OutputFieldTypeObject, Description: "Bounded shared-cluster observation.", Fields: []OutputField{
			{Name: "observation", Type: OutputFieldTypeString, Description: "Whether the cluster was observed.", Enum: []string{"not_observed", "observed", "unknown"}},
			{Name: "runtime", Type: OutputFieldTypeString, Description: "Shared component closure state.", Enum: []string{"absent", "stopped", "running", "unhealthy", "drifted", "unknown"}},
			{Name: "receipt", Type: OutputFieldTypeString, Description: "Shared activation receipt state.", Enum: []string{"absent", "active", "stopped", "drifted", "unknown"}},
		}},
		{Name: "permissions", Type: OutputFieldTypeObject, Description: "Selected Context pending permission summary.", Fields: []OutputField{{Name: "observation", Type: OutputFieldTypeString, Description: "Bounded permission observation.", Enum: []string{"not_observed", "observed", "unknown"}}, {Name: "pending_count", Type: OutputFieldTypeInteger, Description: "Pending remembered-permission count."}}},
		{Name: "services", Type: OutputFieldTypeObject, Description: "Selected Workspace Service owner summary without refs, URLs, or ports.", Fields: []OutputField{{Name: "observation", Type: OutputFieldTypeString, Description: "Bounded owner observation.", Enum: []string{"not_observed", "complete", "partial", "unavailable"}}, {Name: "pending_count", Type: OutputFieldTypeInteger, Description: "Pending Service requests."}, {Name: "active_count", Type: OutputFieldTypeInteger, Description: "Active Service exposures."}, {Name: "unavailable_owner_count", Type: OutputFieldTypeInteger, Description: "Unavailable selected owners."}}},
		{Name: "login_validity", Type: OutputFieldTypeString, Description: "Standard native login validity; status never inspects credentials.", Enum: []string{"not_observed"}},
		{Name: "siblings", Type: OutputFieldTypeArray, Description: "Exhaustive same-root nonselected Contexts.", SemanticScope: "Every same-root Context bound to another Template in the coherent final collection.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One logical sibling.", Fields: []OutputField{{Name: "context_id", Type: OutputFieldTypeString, Description: "Sibling Context identity."}, {Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Sibling Template identity."}, {Name: "template_name", Type: OutputFieldTypeString, Description: "Sibling Template display name."}, {Name: "workspace_present", Type: OutputFieldTypeBoolean, Description: "Whether the sibling has a Workspace binding."}}}},
		{Name: "next", Type: OutputFieldTypeObject, Description: "One domain-derived primary continuation.", Fields: statusHomeNextFields()},
		{Name: "attention", Type: OutputFieldTypeArray, Description: "Separately ordered pending facts.", SemanticScope: "Every selected-scope permission or Service attention fact in the snapshot.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One attention fact.", Fields: []OutputField{{Name: "kind", Type: OutputFieldTypeString, Description: "Attention kind.", Enum: []string{"permissions", "services"}}, {Name: "count", Type: OutputFieldTypeInteger, Description: "Relevant pending count."}, {Name: "observation", Type: OutputFieldTypeString, Description: "Typed observation state."}, {Name: "path", Type: OutputFieldTypeString, Description: "Exact Catalog drill-down path."}, {Name: "inputs", Type: OutputFieldTypeArray, Description: "Typed drill-down inputs.", SemanticScope: "Every input required by the drill-down command.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact input.", Fields: []OutputField{{Name: "name", Type: OutputFieldTypeString, Description: "Catalog input name."}, {Name: "value", Type: OutputFieldTypeString, Description: "Validated value."}}}}}}},
	}
	return CommandSpec{
		Path: "status", Summary: "Understand the canonical current Project and its next action",
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID: "workspace.authority", Outcome: "Return one CWD-first snapshot of desired, active, applied, observed, attention, and next-action facts for the exact default Template and canonical Project Context",
			Inputs:        []CommandInput{formatInput()},
			Output:        CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: fields, Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable, JSONEnvelope: "status", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 3},
			Prerequisites: []string{}, Errors: statusHomeReadErrors(),
		},
		handler: runFinalDefaultPairStatus,
	}
}

func statusHomeTemplateFields() []OutputField {
	return []OutputField{
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Selected Template identity."}, {Name: "name", Type: OutputFieldTypeString, Description: "Template display name."},
		{Name: "generation", Type: OutputFieldTypeInteger, Description: "Current Template generation."}, {Name: "revision", Type: OutputFieldTypeString, Description: "Current semantic Template revision."},
		{Name: "policy_slice_digest", Type: OutputFieldTypeString, Description: "Desired Template-policy slice."}, {Name: "entry_slice_digest", Type: OutputFieldTypeString, Description: "Desired entry slice."},
		{Name: "source_access", Type: OutputFieldTypeString, Description: "Direct source access.", Enum: []string{"read-only", "read-write"}}, {Name: "native_readiness", Type: OutputFieldTypeString, Description: "Template native readiness selection.", Enum: []string{"enabled", "disabled"}},
		{Name: "runtime", Type: OutputFieldTypeObject, Description: "Exact desired Runtime binding without Docker selector.", Fields: []OutputField{{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Runtime identity."}, {Name: "name", Type: OutputFieldTypeString, Description: "Runtime display name."}, {Name: "revision", Type: OutputFieldTypeString, Description: "Runtime revision."}, {Name: "ordinal", Type: OutputFieldTypeInteger, Description: "Runtime revision ordinal."}}},
	}
}

func statusHomeContextFields() []OutputField {
	return []OutputField{
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Exact selected Context identity."}, {Name: "active_template_policy_slice_digest", Type: OutputFieldTypeString, Description: "Independently active Template-policy slice.", Nullable: true},
		{Name: "current_policy_memory_revision", Type: OutputFieldTypeString, Description: "Current Policy Memory revision."}, {Name: "active_policy_memory_revision", Type: OutputFieldTypeString, Description: "Independently active Policy Memory revision.", Nullable: true},
		{Name: "template_policy_activation", Type: OutputFieldTypeString, Description: "Desired versus active Template policy.", Enum: []string{"absent", "current", "pending"}}, {Name: "policy_memory_activation", Type: OutputFieldTypeString, Description: "Current versus active Policy Memory.", Enum: []string{"absent", "current", "pending"}},
	}
}

func statusHomeWorkspaceFields() []OutputField {
	return []OutputField{
		{Name: "presence", Type: OutputFieldTypeString, Description: "Workspace binding presence.", Enum: []string{"absent", "present"}}, {Name: "workspace_id", Type: OutputFieldTypeString, Description: "Workspace identity.", Nullable: true},
		{Name: "workspace_ref", Type: OutputFieldTypeString, Description: "Opaque Workspace drill-down reference.", ReferenceKind: tobari.WorkspaceReferenceKind, Optional: true}, {Name: "applied_entry", Type: OutputFieldTypeObject, Description: "Independent last-successful AppliedEntry.", Nullable: true, Fields: finalAppliedEntryFields()},
		{Name: "entry_state", Type: OutputFieldTypeString, Description: "Desired versus AppliedEntry state.", Enum: []string{"not_observed", "absent", "current", "pending", "blocked_attached", "unknown"}},
		{Name: "observed_runtime_state", Type: OutputFieldTypeString, Description: "Exact selected container observation.", Enum: []string{"not_observed", "absent", "stopped", "running", "drifted", "unknown"}},
		{Name: "attachment_state", Type: OutputFieldTypeString, Description: "Canonical interactive attachment state.", Enum: []string{"not_observed", "detached", "attached", "unknown"}},
	}
}

func statusHomeRuntimeFields() []OutputField {
	return []OutputField{
		{Name: "revision_authority", Type: OutputFieldTypeString, Description: "Exact Runtime revision authority.", Enum: []string{"not_observed", "ready", "not_ready", "unknown"}},
		{Name: "execution_material_availability", Type: OutputFieldTypeString, Description: "Exact local execution material.", Enum: []string{"available", "missing", "mismatched", "pruned", "unknown"}},
		{Name: "native_compatibility", Type: OutputFieldTypeString, Description: "Runtime executable compatibility, never authentication state.", Enum: []string{"not_observed", "compatible", "incompatible", "unknown"}},
	}
}

func statusHomeNextFields() []OutputField {
	return []OutputField{
		{Name: "kind", Type: OutputFieldTypeString, Description: "Command or non-command guidance.", Enum: []string{"command", "guidance"}}, {Name: "path", Type: OutputFieldTypeString, Description: "Exact Catalog path.", Nullable: true},
		{Name: "inputs", Type: OutputFieldTypeArray, Description: "Typed command inputs.", SemanticScope: "Every input required by the selected command.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact input.", Fields: []OutputField{{Name: "name", Type: OutputFieldTypeString, Description: "Catalog input name."}, {Name: "value", Type: OutputFieldTypeString, Description: "Validated value."}}}},
		{Name: "guidance", Type: OutputFieldTypeString, Description: "Typed non-command guidance.", Nullable: true, Enum: []string{"wait_for_detach", "continue_attached"}}, {Name: "reason", Type: OutputFieldTypeString, Description: "Human reason for the primary continuation."},
	}
}
