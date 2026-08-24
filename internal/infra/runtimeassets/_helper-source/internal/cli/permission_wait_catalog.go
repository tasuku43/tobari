package cli

import (
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func permissionWaitCommandSpecs() []CommandSpec {
	return []CommandSpec{permissionWaitSpec(), permissionWaitHelpSpec()}
}

func permissionWaitSpec() CommandSpec {
	length := int64(36)
	return CommandSpec{
		Program: PermissionProgramName, Path: "wait", Summary: "Wait for one reviewed permission result",
		Args: "--id <permission-wait-id> [--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "policy.permission-wait",
			Outcome:      "Observe one attachment-owned reviewed disposition as Allow, Deny, or Expired without mutating policy or retrying the denied request",
			Inputs: []CommandInput{
				{Name: "--id", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description: "Exact attachment-local permission wait correlation from one eligible Gateway denial.", AllowedValues: []string{}, MinimumLength: &length, MaximumLength: &length},
				formatInput(),
			},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText,
				TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{{Name: "result", Type: OutputFieldTypeString, Description: "Terminal reviewed disposition observation; Allow is retry-readiness only.", Enum: []string{
					string(tobari.PermissionWaitResultAllow), string(tobari.PermissionWaitResultDeny), string(tobari.PermissionWaitResultExpired),
				}}},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
				JSONEnvelope: "result", JSONEnvelopeType: OutputFieldTypeString, JSONSchemaVersion: 1,
				readSettlement: readOutputSettlementConsumed,
			},
			Prerequisites: []string{"Run inside the same live interactive Workspace attachment that received the schema-2 Gateway denial handoff."},
			Errors:        permissionWaitErrors(),
		},
		handler: runPermissionWait,
	}
}

func permissionWaitErrors() []CommandError {
	recovery := "help wait"
	return []CommandError{
		declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, recovery, "Use the exact wait ID from a supported Gateway denial."),
		declaredCommandError(fault.KindNotFound, "invalid_permission_wait", false, recovery, "A fresh eligible denial must issue a new attachment-local wait ID."),
		declaredCommandError(fault.KindContract, "invalid_permission_wait_record", false, recovery, "A fresh eligible denial must issue a valid wait record."),
		declaredCommandError(fault.KindContract, "invalid_permission_wait_result", false, recovery, "The helper accepts only Allow, Deny, or Expired."),
		declaredCommandError(fault.KindUnavailable, "permission_wait_owner_unavailable", false, recovery, "The attachment owner is gone; a fresh denial in a live attachment is required."),
		declaredCommandError(fault.KindUnavailable, "permission_wait_unavailable", true, recovery, "Keep review separate and inspect the exact wait contract before retrying within the lease."),
		declaredCommandError(fault.KindCanceled, "permission_wait_interrupted", true, recovery, "Retry only within the bounded connection-attempt budget while the same owner remains live."),
		declaredCommandError(fault.KindContract, "permission_wait_transport_failed", false, recovery, "Do not trust or replay an invalid attachment transport response."),
		declaredCommandError(fault.KindContract, "output_encoding_failed", false, recovery, "Repair the helper output contract before another observation."),
		declaredCommandError(fault.KindInternal, consumedReadOutputWriteFailureCode, false, recovery, "The terminal result was consumed; use a fresh denial instead of replaying this ID."),
		declaredCommandError(fault.KindCanceled, "operation_canceled", true, recovery, "Retry only while the same attachment owner and wait lease remain live."),
	}
}

func permissionWaitHelpSpec() CommandSpec {
	return CommandSpec{
		Program: PermissionProgramName, Path: "help", Summary: "Show permission wait helper help",
		Args: "[<command>...] [--format text|agent]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: "cli.discovery", Outcome: "Discover only the attachment-local permission wait command and its non-authoritative result contract",
			Inputs: []CommandInput{
				{Name: "command", Source: InputSourceArgument, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable,
					Description: "Select the exact helper command path.", AllowedValues: []string{}, Completion: InputCompletionCommand},
				{Name: "--format", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description: "Select human text or the machine-readable agent contract.", AllowedValues: []string{"text", "agent"}, DefaultValue: stringPointer("text")},
			},
			Output: CommandOutput{
				Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
				Fields: []OutputField{
					{Name: "path", Type: OutputFieldTypeString, Description: "Exact helper command path."},
					{Name: "namespace", Type: OutputFieldTypeString, Description: "Canonical helper namespace."},
					{Name: "summary", Type: OutputFieldTypeString, Description: "Command summary."},
					{Name: "capability_id", Type: OutputFieldTypeString, Description: "Stable capability identity."},
					{Name: "outcome", Type: OutputFieldTypeString, Description: "User outcome."},
					{Name: "effect", Type: OutputFieldTypeString, Description: "Declared effect."},
					{Name: "role", Type: OutputFieldTypeString, Description: "Declared role."},
				},
				Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				JSONEnvelope: "commands", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: 1,
			},
			Prerequisites: []string{},
			Errors: []CommandError{
				declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help", "Use an exact helper command selector."),
				declaredCommandError(fault.KindContract, "output_encoding_failed", false, "help help", "Inspect helper help without repeating agent-help encoding."),
				declaredCommandError(fault.KindInternal, "output_write_failed", true, "help", "Retry with a writable output stream."),
				declaredCommandError(fault.KindCanceled, "operation_canceled", true, "help", "Retry when ready."),
			},
		},
		handler: runHelp,
	}
}
