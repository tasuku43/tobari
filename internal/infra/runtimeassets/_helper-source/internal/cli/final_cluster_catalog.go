package cli

import (
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func finalClusterUpSpec() CommandSpec {
	return CommandSpec{
		Path: "cluster up", Summary: finalClusterSurfaceSummary("Activate", "activation"),
		Args: "[--format text|json]", Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "cluster.lifecycle",
			Outcome:      "Activate the exact final collection's current Template-policy and Policy-Memory axes and reconcile the selected shared component closure",
			Inputs:       []CommandInput{formatInput()},
			Output:       finalClusterUpOutput(),
			Prerequisites: []string{
				"Docker Engine and Docker Compose v2 are available.",
				"The final authority collection is valid and any interrupted same-action cluster activation can be recovered.",
			},
			FixedTarget: fixedClusterTarget(),
			Errors:      finalClusterUpErrors(),
			Mutation: &MutationContract{
				TargetKind: tobari.ClusterTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo},
			},
		},
		handler: runFinalClusterUp,
	}
}

func finalClusterUpErrors() []CommandError {
	errors := []CommandError{
		classifiedCommandError(fault.KindContract, "invalid_cluster_reconciliation_result", false, fault.PhaseVerification, fault.ChangeUnknown, "cluster status", "Inspect final authority and component state."),
		declaredCommandError(fault.KindUnavailable, "cluster_reconcile_interrupted", false, "cluster status", "Inspect the retained final activation decision."),
		declaredCommandError(fault.KindInternal, "missing_port", false, "doctor", "Configure the final cluster lifecycle adapter."),
	}
	if buildIdentityHasBroker() {
		errors = append(errors,
			declaredCommandError(fault.KindUnavailable, "auth_broker_image_unavailable", true, "doctor", "Inspect Docker image availability before reconciling the shared cluster."),
			declaredCommandError(fault.KindContract, "auth_broker_image_incompatible", false, "doctor", "Inspect the Auth Broker image API, digest, entrypoint, user, and architecture contract."),
			declaredCommandError(fault.KindUnavailable, "credential_companion_unavailable", true, "cluster status", "Inspect shared authentication-service state before reconciliation."),
			declaredCommandError(fault.KindUnavailable, "auth_broker_unavailable", true, "cluster status", "Inspect shared-cluster state before another broker reconciliation."),
			declaredCommandError(fault.KindUnavailable, "auth_broker_request_failed", false, "cluster status", "Inspect partial shared-cluster state before another reconcile."),
			declaredCommandError(fault.KindUnavailable, "auth_broker_locked", false, "cluster status", "Inspect the locked Auth Broker state before another reconciliation."),
			declaredCommandError(fault.KindContract, "auth_broker_unlock_failed", false, "doctor", "Inspect Auth Broker and root-key provider state."),
			declaredCommandError(fault.KindUnavailable, "root_key_unavailable", false, "doctor", "Inspect the host root-key provider."),
			declaredCommandError(fault.KindRejected, "root_key_missing_with_vault", false, "doctor", "Restore the original root key or explicitly remove local authentication state."),
			declaredCommandError(fault.KindRejected, "root_key_unsafe", false, "doctor", "Repair unsafe root-key or Auth Broker state paths."),
			declaredCommandError(fault.KindUnavailable, "keychain_denied", false, "doctor", "Inspect trusted-host root-key readiness before cluster reconciliation."),
			declaredCommandError(fault.KindRejected, "auth_vault_invalid", false, "doctor", "Inspect the final Context vault integrity without printing its contents."),
			declaredCommandError(fault.KindUnsupported, "auth_vault_version_unsupported", false, "doctor", "Upgrade or repair the unsupported final Context vault."),
			declaredCommandError(fault.KindRejected, "invalid_provider_manifest", false, "doctor", "Repair the owner-controlled provider manifest collection."),
			declaredCommandError(fault.KindRejected, "ambiguous_provider_http_binding", false, "doctor", "Remove the overlapping exact provider HTTP binding."),
		)
	}
	return mutationCommandErrors("cluster up", "cluster status", errors...)
}

func finalClusterStatusSpec() CommandSpec {
	return CommandSpec{
		Path: "cluster status", Summary: finalClusterSurfaceSummary("Observe", "status"),
		Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID:  "cluster.lifecycle",
			Outcome:       "Observe one bounded final collection, its active or stopped receipt consequence, and the selected shared component closure without repair",
			Inputs:        []CommandInput{formatInput()},
			Output:        finalClusterStatusOutput(),
			Prerequisites: []string{},
			Errors: readCommandErrors("cluster status", true,
				declaredCommandError(fault.KindContract, "invalid_cluster_status_result", false, "doctor", "Repair the final cluster observation contract."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating final-cluster JSON encoding."),
				declaredCommandError(fault.KindInternal, "missing_port", false, "doctor", "Configure the final cluster status adapter."),
			),
		},
		handler: runFinalClusterStatus,
	}
}

func finalClusterDownSpec() CommandSpec {
	return CommandSpec{
		Path: "cluster down", Summary: finalClusterSurfaceSummary("Stop", "retirement"),
		Args: "[--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "cluster.lifecycle",
			Outcome:      "Stop the exact final shared component closure and clear every active Context receipt while preserving Templates, Contexts, current Policy Memory, and the final envelope",
			Inputs:       []CommandInput{formatInput()},
			Output:       finalClusterDownOutput(),
			Prerequisites: []string{
				"The final collection contains zero Workspaces.",
				"Canonical global Workspace session ownership is exactly empty; ambiguous ownership fails closed.",
			},
			FixedTarget: fixedClusterTarget(),
			Errors: mutationCommandErrors("cluster down", "cluster status",
				declaredCommandError(fault.KindRejected, "cluster_not_empty", false, "workspace list", "Delete every final Workspace explicitly."),
				declaredCommandError(fault.KindUnavailable, "cluster_reconcile_interrupted", false, "cluster status", "Inspect the retained final lifecycle decision."),
				declaredCommandError(fault.KindContract, "invalid_cluster_down_result", false, "cluster status", "Inspect the final stopped consequence."),
				declaredCommandError(fault.KindInternal, "missing_port", false, "doctor", "Configure the final cluster lifecycle adapter."),
			),
			Mutation: &MutationContract{
				TargetKind: tobari.ClusterTargetKind, TargetInputs: []string{},
				Impact: operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes},
			},
		},
		handler: runFinalClusterDown,
	}
}

func finalClusterLogsSpec() CommandSpec {
	summary := "Read final Gateway and OPA logs"
	args := "[--component gateway|opa|all] [--tail <lines>]"
	components := []string{"gateway", "opa", "all"}
	if buildIdentityHasBroker() {
		summary = "Read final Auth Broker, Gateway, and OPA logs"
		args = "[--component auth-broker|gateway|opa|all] [--tail <lines>]"
		components = []string{"auth-broker", "gateway", "opa", "all"}
	}
	return CommandSpec{Path: "cluster logs", Summary: summary, Args: args, Effect: operation.EffectRead, Role: RoleUtility, Agent: AgentContract{
		CapabilityID: "cluster.logs", Outcome: "Inspect one bounded redacted window from the surface-selected final shared components", Inputs: []CommandInput{
			{Name: "--component", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Select one final component or the complete selected closure.", AllowedValues: components, DefaultValue: stringPointer("all")}, tailInput(),
		}, Output: logOutput(), Prerequisites: []string{"The selected final cluster receipt and live component closure are exact and active."}, Errors: readCommandErrors("cluster logs", true,
			declaredCommandError(fault.KindInvalidInput, "invalid_log_request", false, "help cluster logs", "Select a valid component and bound."),
			declaredCommandError(fault.KindRejected, "legacy_state_present", false, "doctor", "Reset or recreate this pre-release installation."),
			declaredCommandError(fault.KindUnavailable, "cluster_not_running", false, "cluster status", "Reconcile the final cluster before reading logs."),
			declaredCommandError(fault.KindInternal, "logs_failed", false, "cluster status", "Inspect the final component closure."),
			declaredCommandError(fault.KindInternal, "missing_port", false, "doctor", "Configure the final cluster read adapter.")),
	}, handler: runFinalClusterLogs}
}

func finalClusterDenialsSpec() CommandSpec {
	return CommandSpec{Path: "cluster denials", Summary: "Read final policy-denial evidence", Args: "[--tail <lines>] [--format text|json]", Effect: operation.EffectRead, Role: RoleUtility, Agent: AgentContract{
		CapabilityID: "policy.learning", Outcome: "Inspect one bounded Gateway denial window correlated to exact final Context, Template, and Workspace authority", Inputs: []CommandInput{denialTailInput(), formatInput()},
		Output: CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
			Fields: []OutputField{
				{Name: "task", Type: OutputFieldTypeString, Description: "Final denial observation task identity."},
				{Name: "window_lines", Type: OutputFieldTypeInteger, Description: "Maximum recent Gateway lines inspected."},
				{Name: "unparsed_lines", Type: OutputFieldTypeInteger, Description: "Denial-shaped lines rejected by strict decoding."},
				{Name: "items", Type: OutputFieldTypeArray, Description: "Validated denials correlated to complete final authority.", SemanticScope: "Every valid denial in the requested bounded Gateway window.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One final denial observation.", Fields: finalClusterDenialFields()}},
				{Name: "review_command", Type: OutputFieldTypeString, Description: "Exact Permission Inbox command."},
			}, Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageBoundedWindow, JSONEnvelope: "denials", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: tobari.FinalClusterDenialSchemaVersion},
		Prerequisites: []string{"The selected final cluster receipt and live component closure are exact and active."}, Errors: readCommandErrors("cluster denials", true,
			declaredCommandError(fault.KindInvalidInput, "invalid_denial_request", false, "help cluster denials", "Select a valid bounded window."),
			declaredCommandError(fault.KindRejected, "legacy_state_present", false, "doctor", "Reset or recreate this pre-release installation."),
			declaredCommandError(fault.KindUnavailable, "cluster_not_running", false, "cluster status", "Reconcile the final cluster before reading denials."),
			declaredCommandError(fault.KindInternal, "denials_failed", false, "cluster status", "Inspect the final Gateway observation."),
			declaredCommandError(fault.KindContract, "invalid_denial_contract", false, "cluster status", "Repair the final denial projection."),
			declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating denial JSON encoding."),
			declaredCommandError(fault.KindInternal, "missing_port", false, "doctor", "Configure the final cluster read adapter.")),
	}, handler: runFinalClusterDenials}
}

func finalClusterDenialFields() []OutputField {
	fields := []OutputField{
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Exact final Context identity."},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Exact final Template identity."},
		{Name: "template_name", Type: OutputFieldTypeString, Description: "Final Template display name."},
		{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Exact final Workspace identity."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Canonical final Project root."},
	}
	for _, field := range policyDenialOutputFields() {
		switch field.Name {
		case "context_id", "context", "workspace_id", "project_root":
			continue
		default:
			fields = append(fields, field)
		}
	}
	return fields
}

func finalClusterSurfaceSummary(verb, noun string) string {
	if buildIdentityHasBroker() {
		return verb + " final Gateway, OPA, Auth Broker, and companion " + noun
	}
	return verb + " final Gateway and OPA " + noun
}

func finalClusterStatusOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText,
		TextPresentation: TextPresentationSemanticTokens, Fields: finalClusterStatusFields(),
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope: "cluster", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: tobari.FinalClusterLifecycleSchemaVersion,
	}
}

func finalClusterUpOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText,
		TextPresentation: TextPresentationSemanticTokens, Fields: []OutputField{
			{Name: "task", Type: OutputFieldTypeString, Description: "Confirmed final cluster activation task."},
			{Name: "generation", Type: OutputFieldTypeInteger, Description: "Final collection generation carrying the active consequence."},
			{Name: "collection_revision", Type: OutputFieldTypeString, Description: "Exact final collection revision."},
			{Name: "content_digest", Type: OutputFieldTypeString, Description: "Exact active policy content digest."},
			{Name: "plan_digest", Type: OutputFieldTypeString, Description: "Exact active policy plan digest."},
			{Name: "envelope_changed", Type: OutputFieldTypeBoolean, Description: "Whether activation published a new final envelope generation."},
			{Name: "applied", Type: OutputFieldTypeBoolean, Description: "Always true after exact activation confirmation."},
			{Name: "contexts", Type: OutputFieldTypeArray, Description: "Complete activated Context-axis receipt collection.", SemanticScope: "Every Context selected by the final activation plan.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One Context activation.", Fields: finalClusterActivationFields()}},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope: "cluster_up", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: tobari.FinalClusterLifecycleSchemaVersion,
	}
}

func finalClusterDownOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText,
		TextPresentation: TextPresentationSemanticTokens, Fields: []OutputField{
			{Name: "task", Type: OutputFieldTypeString, Description: "Confirmed final cluster retirement task."},
			{Name: "stopped", Type: OutputFieldTypeBoolean, Description: "Always true after exact stopped confirmation."},
			{Name: "generation", Type: OutputFieldTypeInteger, Description: "Final collection generation carrying the stopped consequence."},
			{Name: "collection_revision", Type: OutputFieldTypeString, Description: "Exact stopped final collection revision."},
			{Name: "envelope_changed", Type: OutputFieldTypeBoolean, Description: "Whether active Context receipts were cleared in a new envelope generation."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope: "cluster_down", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: tobari.FinalClusterLifecycleSchemaVersion,
	}
}

func finalClusterStatusFields() []OutputField {
	return []OutputField{
		{Name: "task", Type: OutputFieldTypeString, Description: "Final cluster observation task identity."},
		{Name: "authority", Type: OutputFieldTypeString, Description: "Whether final collection authority is present.", Enum: []string{"absent", "present"}},
		{Name: "generation", Type: OutputFieldTypeInteger, Description: "Observed final collection generation; omitted with absent authority."},
		{Name: "collection_revision", Type: OutputFieldTypeString, Description: "Observed final collection revision; omitted with absent authority."},
		{Name: "template_count", Type: OutputFieldTypeInteger, Description: "Number of retained final Templates."},
		{Name: "context_count", Type: OutputFieldTypeInteger, Description: "Number of retained final Contexts."},
		{Name: "workspace_count", Type: OutputFieldTypeInteger, Description: "Number of retained final Workspaces."},
		{Name: "runtime", Type: OutputFieldTypeString, Description: "Bounded selected component-closure state.", Enum: []string{"absent", "stopped", "running", "unhealthy", "drifted", "unknown"}},
		{Name: "receipt", Type: OutputFieldTypeString, Description: "Bounded final active or stopped receipt state.", Enum: []string{"absent", "active", "stopped", "drifted", "unknown"}},
		{Name: "contexts", Type: OutputFieldTypeArray, Description: "Complete per-Context active receipt summary.", SemanticScope: "Every Context in the selected final collection at one observation.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One Context receipt summary.", Fields: []OutputField{
			{Name: "context_id", Type: OutputFieldTypeString, Description: "Final Context identity."},
			{Name: "template_policy", Type: OutputFieldTypeObject, Description: "Active Template-policy receipt when present.", Nullable: true, Fields: []OutputField{
				{Name: "context_id", Type: OutputFieldTypeString, Description: "Receipt Context identity."},
				{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Receipt Template identity."},
				{Name: "policy_slice_digest", Type: OutputFieldTypeString, Description: "Active Template-policy slice digest."},
			}},
			{Name: "policy_memory", Type: OutputFieldTypeObject, Description: "Active Policy-Memory receipt when present.", Nullable: true, Fields: []OutputField{
				{Name: "context_id", Type: OutputFieldTypeString, Description: "Receipt Context identity."},
				{Name: "revision", Type: OutputFieldTypeString, Description: "Active Policy-Memory revision."},
			}},
		}}},
		{Name: "components", Type: OutputFieldTypeArray, Description: "Build-surface-selected semantic component observations.", SemanticScope: finalClusterComponentScope(), Items: &OutputField{Type: OutputFieldTypeObject, Description: "One selected component observation.", Fields: []OutputField{
			{Name: "name", Type: OutputFieldTypeString, Description: "Semantic component name.", Enum: finalClusterComponentNames()},
			{Name: "state", Type: OutputFieldTypeString, Description: "Observed component runtime state.", Enum: []string{"absent", "stopped", "running", "unhealthy", "drifted", "unknown"}},
			{Name: "health", Type: OutputFieldTypeString, Description: "Accepted component health summary when present."},
			{Name: "identity", Type: OutputFieldTypeString, Description: "Correlation to accepted component identity evidence.", Enum: []string{"absent", "exact", "drifted", "unknown"}},
			{Name: "topology", Type: OutputFieldTypeString, Description: "Correlation to accepted topology evidence.", Enum: []string{"absent", "exact", "drifted", "unknown"}},
		}}},
	}
}

func finalClusterActivationFields() []OutputField {
	return []OutputField{
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Activated final Context identity."},
		{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Activated final Template identity."},
		{Name: "template_policy", Type: OutputFieldTypeObject, Description: "Exact active Template-policy receipt.", Fields: []OutputField{
			{Name: "context_id", Type: OutputFieldTypeString, Description: "Receipt Context identity."},
			{Name: "workspace_template_id", Type: OutputFieldTypeString, Description: "Receipt Template identity."},
			{Name: "policy_slice_digest", Type: OutputFieldTypeString, Description: "Active Template-policy slice digest."},
		}},
		{Name: "policy_memory", Type: OutputFieldTypeObject, Description: "Exact active Policy-Memory receipt.", Fields: []OutputField{
			{Name: "context_id", Type: OutputFieldTypeString, Description: "Receipt Context identity."},
			{Name: "revision", Type: OutputFieldTypeString, Description: "Active Policy-Memory revision."},
		}},
	}
}

func finalClusterComponentNames() []string {
	if buildIdentityHasBroker() {
		return []string{"gateway", "opa", "auth-broker", "credential-companion"}
	}
	return []string{"gateway", "opa"}
}

func finalClusterComponentScope() string {
	if buildIdentityHasBroker() {
		return "Exactly Gateway, OPA, Auth Broker, and credential companion in semantic order."
	}
	return "Exactly Gateway and OPA in semantic order."
}
