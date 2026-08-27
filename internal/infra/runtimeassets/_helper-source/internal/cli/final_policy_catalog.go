package cli

import (
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func finalPolicyProtocolFields() []OutputField {
	return []OutputField{
		{Name: "scheme", Type: OutputFieldTypeString, Description: "Exact HTTP transport scheme."},
		{Name: "protocol", Type: OutputFieldTypeString, Description: "Exact reviewed application protocol.", Enum: tobari.PolicyProtocolValues()},
		{Name: "graphql_operation_type", Type: OutputFieldTypeString, Description: "GraphQL operation type when applicable.", Optional: true},
		{Name: "graphql_root_field", Type: OutputFieldTypeString, Description: "GraphQL root field when applicable.", Optional: true},
		{Name: "mcp_method", Type: OutputFieldTypeString, Description: "MCP method when applicable.", Optional: true},
		{Name: "mcp_tool_name", Type: OutputFieldTypeString, Description: "MCP tool name when applicable.", Optional: true},
		{Name: "aws_wire_protocol", Type: OutputFieldTypeString, Description: "AWS wire protocol when applicable.", Optional: true},
		{Name: "aws_service", Type: OutputFieldTypeString, Description: "AWS SigV4 service when applicable.", Optional: true},
		{Name: "aws_protocol_version", Type: OutputFieldTypeString, Description: "AWS Query protocol version when applicable.", Optional: true},
		{Name: "aws_target_namespace", Type: OutputFieldTypeString, Description: "AWS JSON target namespace when applicable.", Optional: true},
		{Name: "aws_operation", Type: OutputFieldTypeString, Description: "AWS wire operation when applicable.", Optional: true},
		{Name: "kubernetes_kind", Type: OutputFieldTypeString, Description: "Kubernetes resource or non-resource identity kind when applicable.", Optional: true},
		{Name: "kubernetes_verb", Type: OutputFieldTypeString, Description: "Kubernetes verb when applicable.", Optional: true},
		{Name: "kubernetes_group", Type: OutputFieldTypeString, Description: "Kubernetes API group when applicable; empty denotes core.", Optional: true},
		{Name: "kubernetes_version", Type: OutputFieldTypeString, Description: "Kubernetes API version when applicable.", Optional: true},
		{Name: "kubernetes_resource", Type: OutputFieldTypeString, Description: "Kubernetes resource when applicable.", Optional: true},
		{Name: "kubernetes_namespace", Type: OutputFieldTypeString, Description: "Kubernetes namespace when applicable.", Optional: true},
		{Name: "kubernetes_name", Type: OutputFieldTypeString, Description: "Kubernetes resource name when applicable.", Optional: true},
		{Name: "kubernetes_subresource", Type: OutputFieldTypeString, Description: "Kubernetes subresource when applicable.", Optional: true},
		{Name: "kubernetes_dry_run", Type: OutputFieldTypeString, Description: "Kubernetes dry-run dimension when applicable.", Optional: true},
		{Name: "kubernetes_non_resource_path", Type: OutputFieldTypeString, Description: "Exact Kubernetes non-resource path when applicable.", Optional: true},
		{Name: "git_service", Type: OutputFieldTypeString, Description: "Git smart-HTTP service when applicable.", Optional: true},
		{Name: "git_repository", Type: OutputFieldTypeString, Description: "Git repository path when applicable.", Optional: true},
		{Name: "oci_action", Type: OutputFieldTypeString, Description: "OCI distribution action when applicable.", Optional: true},
		{Name: "oci_repository", Type: OutputFieldTypeString, Description: "OCI repository when applicable.", Optional: true},
		{Name: "oci_object", Type: OutputFieldTypeString, Description: "OCI object coordinate when applicable.", Optional: true},
	}
}

func finalPolicyRuleBodyFields() []OutputField {
	fields := finalPolicyEffectFields()
	fields = append(fields,
		OutputField{Name: "source_candidates", Type: OutputFieldTypeArray, Description: "Historical candidate evidence; not an actionable reference producer.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One consumed candidate identity."}},
	)
	return fields
}

func finalPolicyEffectFields() []OutputField {
	fields := finalPolicyProtocolFields()
	fields = append(fields,
		OutputField{Name: "match", Type: OutputFieldTypeString, Description: "Exact or reviewed path-template match.", Enum: tobari.PolicyMatchValues()},
		OutputField{Name: "host", Type: OutputFieldTypeString, Description: "Exact normalized destination host."},
		OutputField{Name: "port", Type: OutputFieldTypeInteger, Description: "Exact destination port."},
		OutputField{Name: "method", Type: OutputFieldTypeString, Description: "Exact HTTP method."},
		OutputField{Name: "path", Type: OutputFieldTypeString, Description: "Exact path or reviewed path template."},
		OutputField{Name: "segments", Type: OutputFieldTypeArray, Description: "Canonical path-template segments; empty for exact rules.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One canonical path segment."}},
		OutputField{Name: "examples", Type: OutputFieldTypeArray, Description: "Complete sorted source paths.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One exact source path."}},
	)
	return fields
}

func finalPolicyCandidateFields() []OutputField {
	return []OutputField{
		{Name: "id", Type: OutputFieldTypeString, Description: "Opaque actionable Policy candidate reference.", ReferenceKind: tobari.PolicyCandidateKind},
		{Name: "context", Type: OutputFieldTypeString, Description: "Owning Template display name for the final Context."},
		{Name: "template", Type: OutputFieldTypeString, Description: "Owning Template display name."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Canonical owning Project root."},
		{Name: "observing_workspace", Type: OutputFieldTypeString, Description: "Canonical Project root of the observing Workspace."},
		{Name: "effect", Type: OutputFieldTypeObject, Description: "Complete exact proposed Policy Memory effect.", Fields: finalPolicyEffectFields()},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Exact final Context identity."},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact final Workspace Template identity."},
		{Name: "observing_workspace_id", Type: OutputFieldTypeString, Description: "Exact observing Workspace identity."},
	}
}

func finalPolicyRuleFields() []OutputField {
	return []OutputField{
		{Name: "id", Type: OutputFieldTypeString, Description: "Opaque actionable Policy Memory rule reference.", ReferenceKind: tobari.PolicyRuleKind},
		{Name: "decision", Type: OutputFieldTypeString, Description: "Remembered Allow or Deny decision.", Enum: []string{string(tobari.PolicyMemoryAllow), string(tobari.PolicyMemoryDeny)}},
		{Name: "match", Type: OutputFieldTypeString, Description: "Exact or reviewed path-template match.", Enum: tobari.PolicyMatchValues()},
		{Name: "context", Type: OutputFieldTypeString, Description: "Owning Template display name for the final Context."},
		{Name: "template", Type: OutputFieldTypeString, Description: "Owning Template display name."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Canonical owning Project root."},
		{Name: "body", Type: OutputFieldTypeObject, Description: "Complete remembered rule body.", Fields: finalPolicyRuleBodyFields()},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Exact final Context identity."},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact final Workspace Template identity."},
	}
}

func finalPolicyReadErrors(path string) []CommandError {
	return finalAuthorityReadErrors(path, "doctor")
}

func finalPolicyMutationErrors(path, recovery string) []CommandError {
	return mutationCommandErrors(path, recovery,
		declaredCommandError(fault.KindUnavailable, "final_authority_mutation_recovery_required", false, "status", "Read and recover the preserved final-authority decision through the exact initiating command; do not remove authority files manually."),
		declaredCommandError(fault.KindRejected, "legacy_state_present", false, "doctor", "Reset or recreate this pre-release installation before using final Policy Memory."),
		declaredCommandError(fault.KindInvalidInput, "invalid_policy_candidate_ref", false, "policy candidates", "Use an emitted candidate reference unchanged."),
		declaredCommandError(fault.KindInvalidInput, "invalid_policy_rule_ref", false, "policy rules", "Use an emitted rule reference unchanged."),
		declaredCommandError(fault.KindRejected, "policy_review_changed", false, "review permissions", "Review the current complete final decision set again."),
		declaredCommandError(fault.KindNotFound, "policy_target_not_found", false, recovery, "Rediscover current final Policy Memory authority."),
		declaredCommandError(fault.KindContract, "invalid_policy_memory_result", false, recovery, "Reconcile the confirmed final Policy Memory result."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the final Policy Memory adapter."),
	)
}

func finalPolicyCandidatesSpec() CommandSpec {
	return CommandSpec{Path: "policy candidates", Summary: "Discover final Policy Memory candidates", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{
		CapabilityID: "policy.learning", Outcome: "Return every exact pending candidate from one coherent final authority envelope", Inputs: []CommandInput{formatInput()},
		Output:        CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: finalPolicyCandidateFields(), Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive, JSONEnvelope: "policy_candidates", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: tobari.WorkspaceAuthorityPolicyReadSchemaVersion},
		Prerequisites: []string{}, Errors: finalPolicyReadErrors("policy candidates"),
	}, handler: runFinalPolicyCandidates}
}

func finalPolicyReviewSpec() CommandSpec {
	return CommandSpec{Path: "review permissions", Summary: "Review final Policy Memory candidates", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{
		CapabilityID: "policy.learning", Outcome: "Inspect the coherent final pending set without rediscovering predecessor denial logs", Inputs: []CommandInput{formatInput()},
		Output:        CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: finalPolicyCandidateFields(), Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive, JSONEnvelope: "policy_review", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: tobari.WorkspaceAuthorityPolicyReadSchemaVersion},
		Prerequisites: []string{}, Errors: append(finalPolicyReadErrors("review permissions"), declaredCommandError(fault.KindUnavailable, "final_policy_review_unavailable", false, "policy candidates", "Use direct exact decisions until the complete reviewed-set owner is configured.")),
	}, handler: runFinalPolicyReview}
}

func finalPolicyRulesSpec() CommandSpec {
	return CommandSpec{Path: "policy rules", Summary: "Inspect final Policy Memory rules", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{
		CapabilityID: "policy.learning", Outcome: "Return every current Context-owned remembered decision from one coherent final authority envelope", Inputs: []CommandInput{formatInput()},
		Output:        CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: finalPolicyRuleFields(), Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive, JSONEnvelope: "policy_rules", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: tobari.WorkspaceAuthorityPolicyReadSchemaVersion},
		Prerequisites: []string{}, Errors: finalPolicyReadErrors("policy rules"), Interactive: &InteractiveWorkflowContract{ActionCommand: "policy reset", SelectionReferenceKind: tobari.PolicyRuleKind, SelectionOutputField: "id", Confirmation: "explicit_yes", NonInteractiveBehavior: "read_only"},
	}, handler: runFinalPolicyRules}
}

func finalPolicyDirectResultFields() []OutputField {
	return []OutputField{
		{Name: "task", Type: OutputFieldTypeString, Description: "Confirmed final Policy Memory task."},
		{Name: "decision", Type: OutputFieldTypeString, Description: "Applied decision.", Enum: []string{string(tobari.PolicyMemoryAllow), string(tobari.PolicyMemoryDeny)}},
		{Name: "applied", Type: OutputFieldTypeBoolean, Description: "Always true after exact active publication."},
		{Name: "active_revision", Type: OutputFieldTypeString, Description: "Exact active Context Policy Memory revision."},
	}
}

func finalPolicyCandidateMutationSpec(path, summary string, handler commandHandler) CommandSpec {
	return CommandSpec{Path: path, Summary: summary, Args: "--id <policy-candidate-ref> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{
		CapabilityID: "policy.learning", Outcome: summary, Inputs: []CommandInput{finalReferenceInput("--id", "Opaque exact final Policy candidate reference.", tobari.PolicyCandidateKind), formatInput()},
		Output:        CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: finalPolicyDirectResultFields(), Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable, JSONEnvelope: "result", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: tobari.WorkspaceAuthorityPolicyReadSchemaVersion},
		Prerequisites: []string{"The candidate remains present in the same complete final authority."}, Errors: finalPolicyMutationErrors(path, "policy candidates"), Mutation: &MutationContract{TargetKind: tobari.PolicyCandidateKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: workspaceauthoritycmd.PolicyMemoryImpact()},
	}, handler: handler}
}

func finalPolicyAllowSpec() CommandSpec {
	return finalPolicyCandidateMutationSpec("policy allow", "Remember and activate one exact Allow", runFinalPolicyAllow)
}
func finalPolicyDenySpec() CommandSpec {
	return finalPolicyCandidateMutationSpec("policy deny", "Remember and activate one exact Deny", runFinalPolicyDeny)
}

func finalPolicyResetSpec() CommandSpec {
	return CommandSpec{Path: "policy reset", Summary: "Remove one final Policy Memory rule", Args: "--id <policy-rule-ref> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{
		CapabilityID: "policy.learning", Outcome: "Remove one exact current remembered decision and activate the resulting Policy Memory", Inputs: []CommandInput{finalReferenceInput("--id", "Opaque exact final Policy Memory rule reference.", tobari.PolicyRuleKind), formatInput()},
		Output:        CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: []OutputField{{Name: "task", Type: OutputFieldTypeString, Description: "Confirmed final Policy Memory task."}, {Name: "removed", Type: OutputFieldTypeBoolean, Description: "Always true after exact active removal."}, {Name: "active_revision", Type: OutputFieldTypeString, Description: "Exact active Context Policy Memory revision."}}, Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable, JSONEnvelope: "result", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: tobari.WorkspaceAuthorityPolicyReadSchemaVersion},
		Prerequisites: []string{"The rule remains present in current final Policy Memory."}, Errors: finalPolicyMutationErrors("policy reset", "policy rules"), Mutation: &MutationContract{TargetKind: tobari.PolicyRuleKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: workspaceauthoritycmd.PolicyMemoryImpact()},
	}, handler: runFinalPolicyReset}
}

func finalPolicyApplyReviewedSpec() CommandSpec {
	return CommandSpec{Path: "policy apply-reviewed", Summary: "Apply one confirmed complete reviewed set", Args: "", Effect: operation.EffectCreate, Role: RoleAct, Visibility: CommandVisibilityInternal, Agent: AgentContract{
		CapabilityID: "policy.learning", Outcome: "Apply one immutable complete multi-Context reviewed decision set under one global settlement", Inputs: []CommandInput{},
		Output: CommandOutput{
			Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText,
			TextPresentation: TextPresentationSemanticTokens,
			Fields: []OutputField{
				{Name: "task", Type: OutputFieldTypeString, Description: "Confirmed reviewed-set task."},
				{Name: "allow_count", Type: OutputFieldTypeInteger, Description: "Applied Allow count."},
				{Name: "deny_count", Type: OutputFieldTypeInteger, Description: "Applied Deny count."},
				{Name: "applied", Type: OutputFieldTypeBoolean, Description: "Always true after exact activation."},
				{Name: "active_revision", Type: OutputFieldTypeString, Description: "Exact global aggregate revision."},
				{Name: "decisions", Type: OutputFieldTypeArray, Description: "Canonical applied decisions.", Items: &OutputField{
					Type: OutputFieldTypeObject, Description: "One exact applied decision.",
					Fields: []OutputField{
						{Name: "review_item_id", Type: OutputFieldTypeString, Description: "Unchanged reviewed item identity."},
						{Name: "rule_id", Type: OutputFieldTypeString, Description: "Resulting actionable Policy Memory rule reference.", ReferenceKind: tobari.PolicyRuleKind},
						{Name: "decision", Type: OutputFieldTypeString, Description: "Applied decision.", Enum: []string{string(tobari.PolicyMemoryAllow), string(tobari.PolicyMemoryDeny)}},
						{Name: "match", Type: OutputFieldTypeString, Description: "Applied match.", Enum: tobari.PolicyMatchValues()},
					},
				}},
			},
			Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
			JSONEnvelope: "result", JSONEnvelopeType: OutputFieldTypeObject,
			JSONSchemaVersion: tobari.WorkspaceAuthorityPolicyReadSchemaVersion,
		},
		Prerequisites: []string{"Permission Inbox owns one non-empty immutable complete final reviewed set."}, FixedTarget: &FixedTarget{Kind: tobari.PolicyDecisionSetKind, ID: tobari.PolicyDecisionSetID, Description: "The one staged complete reviewed decision set.", Scope: FixedTargetScopeToolLocal}, Errors: finalPolicyMutationErrors("review permissions", "review permissions"), Mutation: &MutationContract{TargetKind: tobari.PolicyDecisionSetKind, TargetInputs: []string{}, Impact: workspaceauthoritycmd.PolicyMemoryImpact()},
	}, handler: runFinalPolicyApplyReviewed}
}
