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
		declaredCommandError(fault.KindRejected, "legacy_state_present", false, "doctor", "Reset or recreate this pre-release installation before initializing final authority."),
		declaredCommandError(fault.KindNotFound, "authority_not_found", false, recovery, "Discover current final authority references."),
		declaredCommandError(fault.KindContract, "invalid_authority", false, recovery, "Repair the final authority envelope."),
		declaredCommandError(fault.KindInternal, "missing_port", false, "doctor", "Configure the final authority adapter."))
}

func finalAuthorityMutationErrors(path, recovery string) []CommandError {
	return mutationCommandErrors(path, recovery,
		declaredCommandError(fault.KindRejected, "legacy_state_present", false, "doctor", "Reset or recreate this pre-release installation before initializing final authority."),
		declaredCommandError(fault.KindNotFound, "authority_not_found", false, recovery, "Discover current final authority references."),
		declaredCommandError(fault.KindRejected, "authority_in_use", false, recovery, "Remove the exact dependent final authority first."),
		declaredCommandError(fault.KindInternal, "missing_port", false, "doctor", "Configure the final authority mutation adapter."))
}

func finalContextEnterErrors(path, recovery string) []CommandError {
	return append(finalAuthorityMutationErrors(path, recovery),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_attachment_unavailable", false, fault.PhaseAttachment, fault.ChangeConfirmed, "context list", "Discover the confirmed Context authority before another explicit entry."),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_interrupted", false, fault.PhaseMutation, fault.ChangePartial, "context list", "Discover the preserved Context authority before another explicit entry."),
		classifiedCommandError(fault.KindRejected, "workspace_entry_template_policy_inactive", false, fault.PhasePrecondition, fault.ChangeNone, "cluster status", "Read the current Template policy activation before explicit cluster reconciliation."),
		classifiedCommandError(fault.KindRejected, "workspace_entry_policy_memory_inactive", false, fault.PhasePrecondition, fault.ChangeNone, "context list", "Discover current Context authority before explicit policy reconciliation."),
		classifiedCommandError(fault.KindUnavailable, "workspace_entry_observation_unavailable", false, fault.PhaseObservation, fault.ChangeNotApplicable, "context list", "Discover desired, applied, and active Context authority without reconciling it."),
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
		{Name: "template_ref", Type: OutputFieldTypeString, Description: "Opaque Workspace Template reference.", ReferenceKind: tobari.WorkspaceTemplateReferenceKind},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact final Template identity."},
		{Name: "name", Type: OutputFieldTypeString, Description: "Template display name."},
		{Name: "generation", Type: OutputFieldTypeInteger, Description: "Current immutable revision generation."},
		{Name: "revision", Type: OutputFieldTypeString, Description: "Current complete-body digest."},
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Current Runtime identity."},
		{Name: "runtime_revision", Type: OutputFieldTypeString, Description: "Current Runtime revision digest."},
		{Name: "source_access", Type: OutputFieldTypeString, Description: "Immutable direct Project source access: read-only or read-write."},
		finalTemplateGraphQLEndpointOutputField(),
	}}}}
}

func finalTemplateShowFields(includeRevisionRef bool) []OutputField {
	fields := []OutputField{
		{Name: "template_ref", Type: OutputFieldTypeString, Description: "Opaque Workspace Template reference.", ReferenceKind: tobari.WorkspaceTemplateReferenceKind},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact final Template identity."},
		{Name: "name", Type: OutputFieldTypeString, Description: "Template display name."},
		{Name: "generation", Type: OutputFieldTypeInteger, Description: "Current immutable revision generation."},
		{Name: "revision", Type: OutputFieldTypeString, Description: "Current complete-body digest."},
		{Name: "runtime_id", Type: OutputFieldTypeString, Description: "Current Runtime identity."},
		{Name: "runtime_revision", Type: OutputFieldTypeString, Description: "Current Runtime revision digest."},
		{Name: "source_access", Type: OutputFieldTypeString, Description: "Immutable direct Project source access: read-only or read-write."},
		finalTemplateGraphQLEndpointOutputField(),
		{Name: "policy_slice_digest", Type: OutputFieldTypeString, Description: "Current independent Template-policy slice digest."},
		{Name: "entry_slice_digest", Type: OutputFieldTypeString, Description: "Current entry-authority slice digest."},
	}
	if includeRevisionRef {
		fields = append(fields[:1], append([]OutputField{{Name: "current_revision_ref", Type: OutputFieldTypeString, Description: "Opaque exact current Template revision reference.", ReferenceKind: tobari.WorkspaceTemplateRevisionReferenceKind}}, fields[1:]...)...)
	}
	return fields
}

func finalTemplateMutationFields() []OutputField {
	fields := finalTemplateShowFields(false)
	return append([]OutputField{}, fields[1:]...)
}

func finalContextFields(includeContextRef bool) []OutputField {
	fields := []OutputField{
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Exact final Context identity."},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact bound Template identity."},
		{Name: "template_name", Type: OutputFieldTypeString, Description: "Bound Template display name."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Canonical Project root."},
		{Name: "desired_template_generation", Type: OutputFieldTypeInteger, Description: "Current desired immutable Template generation."},
		{Name: "desired_template_revision", Type: OutputFieldTypeString, Description: "Current desired complete Template revision digest."},
		{Name: "desired_template_policy_slice_digest", Type: OutputFieldTypeString, Description: "Current desired Template-policy slice digest."},
		{Name: "active_template_policy_slice_digest", Type: OutputFieldTypeString, Description: "Independently active Template-policy slice digest, or null when inactive.", Nullable: true},
		{Name: "current_policy_memory_revision", Type: OutputFieldTypeString, Description: "Current Context-owned Policy Memory revision."},
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
	return CommandSpec{Path: "template list", Summary: "List final Workspace Templates", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return the exhaustive final Workspace Template collection", Inputs: []CommandInput{formatInput()}, Output: finalJSONOutput("templates", finalTemplateListFields(), CollectionCoverageExhaustive), Prerequisites: []string{}, Errors: finalAuthorityReadErrors("template list", "doctor")}, handler: runFinalTemplateList}
}

func finalTemplateShowSpec() CommandSpec {
	return CommandSpec{Path: "template show", Summary: "Inspect one final Workspace Template", Args: "[--name <name>] [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return one final Template and its exact current immutable revision", Inputs: []CommandInput{{Name: "--name", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Read-only Template display-name selector; omission selects the exact default.", AllowedValues: []string{}}, formatInput()}, Output: finalJSONOutput("template", finalTemplateShowFields(true), CollectionCoverageNotApplicable), Prerequisites: []string{}, Errors: finalAuthorityReadErrors("template show", "template list")}, handler: runFinalTemplateShow}
}

func finalTemplateCreateSpec() CommandSpec {
	return CommandSpec{Path: "template create", Summary: "Create one direct final Workspace Template", Args: "--name <name> [--source-access read-only|read-write] [--graphql-endpoint <https-url>] [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Create one fresh Template from the reviewed standard body with bounded source access and optional exact GraphQL endpoint", Inputs: []CommandInput{
		{Name: "--name", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Unique Template display name.", AllowedValues: []string{}},
		{Name: "--source-access", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Immutable direct Project source access for this Template.", AllowedValues: []string{"read-only", "read-write"}, DefaultValue: stringPointer("read-write")},
		{Name: "--graphql-endpoint", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, MinimumLength: int64Pointer(1), Description: "One exact HTTPS GraphQL endpoint with an explicit port and path; it is bounded by the standard destination and POST method ceilings.", AllowedValues: []string{}},
		formatInput()}, Output: finalJSONOutput("template", finalTemplateMutationFields(), CollectionCoverageNotApplicable), Prerequisites: []string{"The built-in standard Runtime revision is available exactly."}, FixedTarget: &FixedTarget{Kind: tobari.WorkspaceTemplateCatalogTargetKind, ID: tobari.WorkspaceTemplateCatalogTargetID, Description: "This installation's final Workspace Template collection.", Scope: FixedTargetScopeToolLocal}, Errors: finalAuthorityMutationErrors("template create", "template list"), Mutation: &MutationContract{TargetKind: tobari.WorkspaceTemplateCatalogTargetKind, TargetInputs: []string{}, Impact: workspaceauthoritycmd.TemplateCreateImpact()}}, handler: runFinalTemplateCreate}
}

func finalTemplateCopySpec() CommandSpec {
	return CommandSpec{Path: "template copy", Summary: "Copy one immutable Template revision", Args: "--from <template-revision-ref> --name <name> [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Create one independent Template from one exact retained revision", Inputs: []CommandInput{finalReferenceInput("--from", "Opaque exact Template revision reference.", tobari.WorkspaceTemplateRevisionReferenceKind), {Name: "--name", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Unique new Template display name.", AllowedValues: []string{}}, formatInput()}, Output: finalJSONOutput("template", finalTemplateMutationFields(), CollectionCoverageNotApplicable), Prerequisites: []string{}, Errors: finalAuthorityMutationErrors("template copy", "template list"), Mutation: &MutationContract{TargetKind: tobari.WorkspaceTemplateReferenceKind, TargetInputs: []string{"--from"}, ParentInput: "--from", Impact: workspaceauthoritycmd.TemplateCreateImpact()}}, handler: runFinalTemplateCopy}
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
	return CommandSpec{Path: path, Summary: summary, Args: args + " [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: summary, Inputs: inputs, Output: finalJSONOutput("result", []OutputField{{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact affected Template identity."}, stateField}, CollectionCoverageNotApplicable), Prerequisites: []string{}, Errors: finalAuthorityMutationErrors(path, "template list"), Mutation: &MutationContract{TargetKind: tobari.WorkspaceTemplateReferenceKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: impact}}, handler: handler}
}

func finalContextListSpec() CommandSpec {
	return CommandSpec{Path: "context list", Summary: "List final Context bindings", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return every final Context with exact Project and Template scope", Inputs: []CommandInput{formatInput()}, Output: finalJSONOutput("contexts", finalContextListFields(), CollectionCoverageExhaustive), Prerequisites: []string{}, Errors: finalAuthorityReadErrors("context list", "doctor")}, handler: runFinalContextList}
}
func finalContextShowSpec() CommandSpec {
	return CommandSpec{Path: "context show", Summary: "Inspect one final Context", Args: "--id <context-ref> [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return one exact Context with desired and independently active authority", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque Context reference.", tobari.ContextReferenceKind), formatInput()}, Output: finalJSONOutput("context", finalContextFields(true), CollectionCoverageNotApplicable), Prerequisites: []string{}, Errors: finalAuthorityReadErrors("context show", "context list")}, handler: runFinalContextShow}
}
func finalContextCreateSpec() CommandSpec {
	return CommandSpec{Path: "context create", Summary: "Create a final Context for the canonical current Project", Args: "--template <template-ref> [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Create one empty Context from one unchanged Template reference and canonical CWD", Inputs: []CommandInput{finalReferenceInput("--template", "Opaque parent Workspace Template reference.", tobari.WorkspaceTemplateReferenceKind), formatInput()}, Output: finalJSONOutput("context", finalContextFields(true), CollectionCoverageNotApplicable), Prerequisites: []string{"The canonical current Project root can be resolved without mutation."}, Errors: finalAuthorityMutationErrors("context create", "context list"), Mutation: &MutationContract{TargetKind: tobari.ContextReferenceKind, TargetInputs: []string{"--template"}, ParentInput: "--template", Impact: workspaceauthoritycmd.ContextCreateImpact()}}, handler: runFinalContextCreate}
}
func finalContextEnterSpec() CommandSpec {
	output := CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields:   []OutputField{{Name: "workspace_ref", Type: OutputFieldTypeString, Description: "Opaque resulting Workspace reference.", ReferenceKind: tobari.WorkspaceReferenceKind}, {Name: "workspace_id", Type: OutputFieldTypeString, Description: "Exact resulting Workspace identity."}, {Name: "context_id", Type: OutputFieldTypeString, Description: "Exact owning Context identity."}, {Name: "exit_code", Type: OutputFieldTypeInteger, Description: "Authoritative child exit code."}},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable, JSONEnvelope: "entry", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1}
	return CommandSpec{Path: "context enter", Summary: "Enter one explicit final Context", Args: "--id <context-ref> [--format text|json] [-- <command>...]", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Reconcile and enter one exact Context Workspace", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque Context parent reference.", tobari.ContextReferenceKind), formatInput(), {Name: "command", Source: InputSourceArgument, ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable, Description: "Exact child argv after --.", AllowedValues: []string{}, PositionalOnly: true}}, Output: output, Prerequisites: []string{"The exact final Context, Runtime, and cluster settlement authorities are available without a conflicting lifecycle or session owner."}, Errors: finalContextEnterErrors("context enter", "context list"), Mutation: &MutationContract{TargetKind: tobari.WorkspaceReferenceKind, TargetInputs: []string{"--id"}, ParentInput: "--id", Impact: workspaceauthoritycmd.ContextEnterImpact()}}, handler: runFinalContextEnter}
}
func finalContextDeleteSpec() CommandSpec {
	return CommandSpec{Path: "context delete", Summary: "Delete one empty final Context", Args: "--id <context-ref> --confirm=delete [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Delete one exact Context, its Policy Memory, and unresolved candidates", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque Context reference.", tobari.ContextReferenceKind), {Name: "--confirm", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Literal destructive confirmation.", AllowedValues: []string{"delete"}}, formatInput()}, Output: finalJSONOutput("result", []OutputField{{Name: "context_id", Type: OutputFieldTypeString, Description: "Deleted Context identity."}, {Name: "deleted", Type: OutputFieldTypeBoolean, Description: "Always true after confirmed deletion."}}, CollectionCoverageNotApplicable), Prerequisites: []string{"No Workspace, live attachment, or research credential remains."}, Errors: finalAuthorityMutationErrors("context delete", "context list"), Mutation: &MutationContract{TargetKind: tobari.ContextReferenceKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: workspaceauthoritycmd.ContextDeleteImpact()}}, handler: runFinalContextDelete}
}

func finalWorkspaceListSpec() CommandSpec {
	return CommandSpec{Path: "workspace list", Summary: "List final Workspaces", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return every final Workspace and its exact owner binding", Inputs: []CommandInput{formatInput()}, Output: finalJSONOutput("workspaces", finalWorkspaceListFields(), CollectionCoverageExhaustive), Prerequisites: []string{}, Errors: finalAuthorityReadErrors("workspace list", "doctor")}, handler: runFinalWorkspaceList}
}
func finalWorkspaceStatusSpec() CommandSpec {
	return CommandSpec{Path: "workspace status", Summary: "Inspect one final Workspace", Args: "--id <workspace-ref> [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Return one exact Workspace and its applied authority", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque Workspace reference.", tobari.WorkspaceReferenceKind), formatInput()}, Output: finalJSONOutput("workspace", finalWorkspaceFields(true), CollectionCoverageNotApplicable), Prerequisites: []string{}, Errors: finalAuthorityReadErrors("workspace status", "workspace list")}, handler: runFinalWorkspaceStatus}
}
func finalWorkspaceDeleteSpec() CommandSpec {
	return CommandSpec{Path: "workspace delete", Summary: "Delete one exact final Workspace", Args: "--id <workspace-ref> --confirm=delete [--force] [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: "Retire one exact Workspace, home, native auth, and owned runtime resources while preserving Context Policy Memory", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque Workspace reference.", tobari.WorkspaceReferenceKind), {Name: "--confirm", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Literal destructive confirmation.", AllowedValues: []string{"delete"}}, {Name: "--force", Source: InputSourceFlag, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Retire the exact live target session and owned container; missing, foreign, ambiguous, or unrelated live owners remain blocking.", AllowedValues: []string{}, DefaultValue: stringPointer("false")}, formatInput()}, Output: finalJSONOutput("result", []OutputField{{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Deleted Workspace identity."}, {Name: "deleted", Type: OutputFieldTypeBoolean, Description: "Always true after confirmed deletion."}}, CollectionCoverageNotApplicable), Prerequisites: []string{"The exact target and canonical attachment authority can be observed without ambiguity."}, Errors: finalAuthorityMutationErrors("workspace delete", "workspace list"), Mutation: &MutationContract{TargetKind: tobari.WorkspaceReferenceKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: workspaceauthoritycmd.WorkspaceDeleteImpact()}}, handler: runFinalWorkspaceDelete}
}

func finalConfigShellSpec() CommandSpec {
	return finalTemplateConfigSpec("config shell", "Update exact Template shell defaults", tobari.WorkspaceTemplateChangeShell, runFinalConfigShell, false)
}
func finalConfigGitSpec() CommandSpec {
	return finalTemplateConfigSpec("config git", "Update exact Template Git defaults", tobari.WorkspaceTemplateChangeGit, runFinalConfigGit, false)
}
func finalConfigBootstrapAWSSpec() CommandSpec {
	spec := finalTemplateConfigSpec("config bootstrap aws", "Update exact Template AWS creation defaults", tobari.WorkspaceTemplateChangeBootstrapAWS, runFinalConfigBootstrapAWS, false)
	spec.Args = "--id <template-ref> [--profile <name>] [--refresh] [--remove] [--format text|json]"
	actions := []CommandInput{
		{Name: "--profile", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Exact host AWS profile to review and normalize.", AllowedValues: []string{}, ConflictsWith: []string{"--refresh", "--remove"}},
		{Name: "--refresh", Source: InputSourceFlag, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Refresh the exact retained profile authority.", AllowedValues: []string{}, ConflictsWith: []string{"--profile", "--remove"}},
		{Name: "--remove", Source: InputSourceFlag, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Remove future-Workspace AWS creation defaults.", AllowedValues: []string{}, ConflictsWith: []string{"--profile", "--refresh"}},
	}
	spec.Agent.Inputs = append(spec.Agent.Inputs[:1], append(actions, spec.Agent.Inputs[1:]...)...)
	return spec
}
func finalConfigBootstrapEKSSpec() CommandSpec {
	spec := finalTemplateConfigSpec("config bootstrap kubernetes eks", "Update exact Template EKS creation defaults", tobari.WorkspaceTemplateChangeBootstrapEKS, runFinalConfigBootstrapEKS, false)
	spec.Args = "--id <template-ref> [--kube-context <name>] [--refresh] [--remove] [--format text|json]"
	actions := []CommandInput{
		{Name: "--kube-context", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Exact host Kubernetes EKS context to review and normalize.", AllowedValues: []string{}, ConflictsWith: []string{"--refresh", "--remove"}},
		{Name: "--refresh", Source: InputSourceFlag, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Refresh the exact retained EKS source authority.", AllowedValues: []string{}, ConflictsWith: []string{"--kube-context", "--remove"}},
		{Name: "--remove", Source: InputSourceFlag, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Remove future-Workspace EKS creation defaults.", AllowedValues: []string{}, ConflictsWith: []string{"--kube-context", "--refresh"}},
	}
	spec.Agent.Inputs = append(spec.Agent.Inputs[:1], append(actions, spec.Agent.Inputs[1:]...)...)
	return spec
}
func finalTemplateRuntimeSetSpec() CommandSpec {
	return finalTemplateConfigSpec("template runtime set", "Replace exact Template Runtime binding", tobari.WorkspaceTemplateChangeRuntime, runFinalTemplateRuntimeSet, true)
}

func finalTemplateConfigSpec(path, summary string, kind tobari.WorkspaceTemplateChangeKind, handler commandHandler, runtimeParent bool) CommandSpec {
	inputs := []CommandInput{finalReferenceInput("--id", "Opaque Workspace Template target reference.", tobari.WorkspaceTemplateReferenceKind)}
	args := "--id <template-ref>"
	if runtimeParent {
		inputs = append(inputs, finalReferenceInput("--runtime", "Opaque exact Runtime revision parent reference.", tobari.RuntimeRevisionReferenceKind))
		args += " --runtime <runtime-revision-ref>"
	}
	if kind == tobari.WorkspaceTemplateChangeShell {
		inputs = append(inputs, CommandInput{Name: "--variable", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Allowlisted shell variable.", AllowedValues: []string{"COLORTERM", "NO_COLOR", "PS1", "TERM"}}, CommandInput{Name: "--source", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Shell value source.", AllowedValues: []string{"default", "inherit", "literal"}}, CommandInput{Name: "--value", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Literal value when source is literal.", AllowedValues: []string{}})
		args += " --variable COLORTERM|NO_COLOR|PS1|TERM --source default|inherit|literal [--value <value>]"
	}
	if kind == tobari.WorkspaceTemplateChangeGit {
		inputs = append(inputs, CommandInput{Name: "--source", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Git identity source; literal requires both name and email, while default and inherit accept neither.", AllowedValues: []string{"default", "inherit", "literal"}}, CommandInput{Name: "--name", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Literal Git author name.", AllowedValues: []string{}, Requires: []string{"--source", "--email"}}, CommandInput{Name: "--email", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Literal Git author email.", AllowedValues: []string{}, Requires: []string{"--source", "--name"}})
		args += " --source default|inherit|literal [--name <name> --email <email>]"
	}
	inputs = append(inputs, formatInput())
	targetInputs := []string{"--id"}
	parent := ""
	if runtimeParent {
		targetInputs = append(targetInputs, "--runtime")
		parent = "--runtime"
	}
	impact, _ := workspaceauthoritycmd.TemplateConfigurationImpact(path)
	return CommandSpec{Path: path, Summary: summary, Args: args + " [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.authority", Outcome: summary + " from the current body under the lifecycle lock", Inputs: inputs, Output: finalJSONOutput("template", finalTemplateMutationFields(), CollectionCoverageNotApplicable), Prerequisites: []string{}, Errors: finalAuthorityMutationErrors(path, "template show"), Mutation: &MutationContract{TargetKind: tobari.WorkspaceTemplateReferenceKind, TargetInputs: targetInputs, TargetIDInput: "--id", ParentInput: parent, Impact: impact}}, handler: handler}
}
