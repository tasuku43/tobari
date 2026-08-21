package cli

import (
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func serviceExposureCommandSpecs() []CommandSpec {
	return []CommandSpec{
		exposureHelperRootSpec(), exposureHelperListSpec(), exposureHelperStopSpec(), exposureHelperHelpSpec(),
		serviceReviewSpec(), serviceRequestsSpec(), serviceAllowSpec(), serviceDenySpec(),
	}
}

func exposureFields() []OutputField {
	return []OutputField{
		{Name: "id", Type: OutputFieldTypeString, Description: "Opaque attachment-owned exposure reference.", ReferenceKind: tobari.ServiceExposureKind},
		{Name: "url", Type: OutputFieldTypeString, Description: "Exact host IPv4-loopback URL."},
		{Name: "target_port", Type: OutputFieldTypeInteger, Description: "Exact Workspace-loopback target port."},
		{Name: "state", Type: OutputFieldTypeString, Description: "Passive listener state.", Enum: []string{tobari.ServiceStateListening, tobari.ServiceStateRelaying, tobari.ServiceStateUnavailable}},
	}
}

func serviceRequestFields() []OutputField {
	return []OutputField{
		{Name: "id", Type: OutputFieldTypeString, Description: "Opaque live service-request reference.", ReferenceKind: tobari.ServiceRequestKind},
		{Name: "workspace", Type: OutputFieldTypeString, Description: "Canonical Workspace project root."},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable Context authority identity."},
		{Name: "project_id", Type: OutputFieldTypeString, Description: "Stable Workspace identity."},
		{Name: "attachment_id", Type: OutputFieldTypeString, Description: "Live attachment epoch identity."},
		{Name: "target_port", Type: OutputFieldTypeInteger, Description: "Exact requested Workspace-loopback port."},
		{Name: "state", Type: OutputFieldTypeString, Description: "Request state.", Enum: []string{tobari.ServiceStatePending}},
	}
}

func textOutput(fields []OutputField, coverage CollectionCoverage) CommandOutput {
	return CommandOutput{Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: fields, Delivery: OutputDeliveryComplete, CollectionCoverage: coverage}
}

func serviceReadErrors(path, recovery string) []CommandError {
	return []CommandError{
		declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help "+path, "Correct the command arguments."),
		declaredCommandError(fault.KindContract, "invalid_service_request_list", false, recovery, "Retry from a fresh live-owner snapshot."),
		declaredCommandError(fault.KindContract, "invalid_service_exposure_list", false, recovery, "Retry from the current attachment."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, recovery, "Use the command in its declared host or Workspace scope."),
		declaredCommandError(fault.KindInternal, "output_write_failed", true, path, "Retry with a writable output stream."),
		declaredCommandError(fault.KindCanceled, "operation_canceled", true, path, "Retry when the caller is ready."),
	}
}

func serviceMutationErrors(path, recovery string) []CommandError {
	return mutationCommandErrors(path, recovery,
		declaredCommandError(fault.KindInvalidInput, "invalid_service_port", false, "help "+path, "Choose an exact non-privileged Workspace port."),
		declaredCommandError(fault.KindInvalidInput, "invalid_service_request", false, recovery, "Use one opaque request reference unchanged."),
		declaredCommandError(fault.KindNotFound, "service_request_not_found", false, recovery, "Refresh the live service-request snapshot."),
		declaredCommandError(fault.KindRejected, "service_request_stale", false, recovery, "Refresh the live service-request snapshot."),
		declaredCommandError(fault.KindRejected, "service_request_denied", false, recovery, "Request exposure again if it is still needed."),
		declaredCommandError(fault.KindRejected, "service_attachment_closed", false, recovery, "Enter the Workspace again before requesting exposure."),
		declaredCommandError(fault.KindUnavailable, "service_attachment_unavailable", false, recovery, "Use the helper from a live attached Workspace."),
		declaredCommandError(fault.KindInternal, "service_instruction_write_failed", false, recovery, "Retry with a writable terminal before another request."),
		declaredCommandError(fault.KindNotFound, "service_exposure_not_found", false, recovery, "List current-attachment exposures."),
		declaredCommandError(fault.KindContract, "invalid_service_exposure", false, recovery, "Reconcile the live attachment-owned exposure."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, recovery, "Use the command in its declared host or Workspace scope."),
	)
}

func exposureHelperRootSpec() CommandSpec {
	minimum, maximum := int64(tobari.ServicePortMinimum), int64(tobari.ServicePortMaximum)
	return CommandSpec{
		Program: ExposureProgramName, Path: ExposureProgramName, Summary: "Request one Workspace HTTP service on host loopback", Args: "<port>", Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: "workspace.service-exposure", Outcome: "Request trusted-host approval for one exact Workspace-loopback HTTP port and return the confirmed attachment-owned exposure",
			Inputs:        []CommandInput{{Name: "port", Source: InputSourceArgument, Required: true, ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle, Description: "Exact non-privileged Workspace-loopback service port.", AllowedValues: []string{}, Minimum: &minimum, Maximum: &maximum}},
			Output:        textOutput(exposureFields(), CollectionCoverageNotApplicable),
			Prerequisites: []string{"The command runs inside a live interactive Workspace attachment and trusted-host review remains available in a separate terminal."},
			FixedTarget:   &FixedTarget{Kind: tobari.ServiceAttachmentServicesKind, ID: tobari.ServiceAttachmentServicesTargetID, Description: "The current attachment's one binary-owned Workspace service request scope.", Scope: FixedTargetScopeToolLocal},
			Errors:        serviceMutationErrors(ExposureProgramName, "list"),
			Mutation:      &MutationContract{TargetKind: tobari.ServiceAttachmentServicesKind, TargetInputs: []string{}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}},
		}, handler: runExposureRequest,
	}
}

func exposureHelperListSpec() CommandSpec {
	return CommandSpec{
		Program: ExposureProgramName, Path: "list", Summary: "List current-attachment service exposures", Args: "", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Return the exhaustive current-attachment exposure inventory with exact stop references", Inputs: []CommandInput{}, Output: textOutput(exposureFields(), CollectionCoverageExhaustive), Prerequisites: []string{"The command runs inside a live Workspace attachment."}, Errors: serviceReadErrors("list", "list")}, handler: runExposureList,
	}
}

func exposureHelperStopSpec() CommandSpec {
	return CommandSpec{
		Program: ExposureProgramName, Path: "stop", Summary: "Stop one current-attachment service exposure", Args: "<exposure-ref>", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Close one exact attachment-owned host listener and its active relays", Inputs: []CommandInput{{Name: "exposure-ref", Source: InputSourceArgument, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Opaque exposure reference emitted by request or list.", AllowedValues: []string{}, ReferenceKind: tobari.ServiceExposureKind}}, Output: textOutput([]OutputField{{Name: "stopped", Type: OutputFieldTypeBoolean, Description: "Always true after confirmed closure."}}, CollectionCoverageNotApplicable), Prerequisites: []string{"The opaque reference belongs to the current live attachment."}, Errors: serviceMutationErrors("stop", "list"), Mutation: &MutationContract{TargetKind: tobari.ServiceExposureKind, TargetInputs: []string{"exposure-ref"}, TargetIDInput: "exposure-ref", Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}}, handler: runExposureStop,
	}
}

func exposureHelperHelpSpec() CommandSpec {
	return CommandSpec{
		Program: ExposureProgramName, Path: "help", Summary: "Show Workspace service helper help", Args: "[<command>...] [--format text|agent]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{CapabilityID: "cli.discovery", Outcome: "Discover only the attachment-local service helper commands and contracts", Inputs: []CommandInput{{Name: "command", Source: InputSourceArgument, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable, Description: "Select an exact helper command path.", AllowedValues: []string{}, Completion: InputCompletionCommand}, {Name: "--format", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Select human text or the machine-readable agent contract.", AllowedValues: []string{"text", "agent"}, DefaultValue: stringPointer("text")}}, Output: CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: []OutputField{{Name: "path", Type: OutputFieldTypeString, Description: "Exact helper command path."}, {Name: "namespace", Type: OutputFieldTypeString, Description: "Canonical helper namespace."}, {Name: "summary", Type: OutputFieldTypeString, Description: "Command summary."}, {Name: "capability_id", Type: OutputFieldTypeString, Description: "Stable capability identity."}, {Name: "outcome", Type: OutputFieldTypeString, Description: "User outcome."}, {Name: "effect", Type: OutputFieldTypeString, Description: "Declared effect."}, {Name: "role", Type: OutputFieldTypeString, Description: "Declared role."}}, Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive, JSONEnvelope: "commands", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: 1}, Prerequisites: []string{}, Errors: []CommandError{declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help", "Use an exact helper command selector."), declaredCommandError(fault.KindContract, "output_encoding_failed", false, "help", "Repair helper agent help projection."), declaredCommandError(fault.KindInternal, "output_write_failed", true, "help", "Retry with a writable output stream."), declaredCommandError(fault.KindCanceled, "operation_canceled", true, "help", "Retry when ready.")}}, handler: runHelp,
	}
}

func serviceReviewSpec() CommandSpec {
	return CommandSpec{
		Path: "review", Summary: "Review Permission and Workspace service requests", Args: "", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{CapabilityID: "trusted-host.review", Outcome: "Choose the canonical Permission Inbox or review one live Workspace service request with immediate Allow once or Deny semantics", Inputs: []CommandInput{}, Output: textOutput(serviceRequestFields(), CollectionCoverageExhaustive), Prerequisites: []string{"Interactive decisions require a trusted host input and output terminal; redirected use remains read-only."}, Errors: append(serviceReadErrors("review", "review"), declaredCommandError(fault.KindInvalidInput, "review_requires_tty", false, "review", "Run trusted-host review from an interactive host terminal.")), Interactive: &InteractiveWorkflowContract{ActionCommands: []string{"service allow", "service deny"}, SelectionReferenceKind: tobari.ServiceRequestKind, SelectionOutputField: "id", Confirmation: "explicit_yes", NonInteractiveBehavior: "read_only"}}, handler: runUnifiedReview,
	}
}

func serviceRequestsSpec() CommandSpec {
	return CommandSpec{Path: "service requests", Summary: "Discover live Workspace service requests", Args: "", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Return one fresh exhaustive pending-request snapshot from all live attachment owners", Inputs: []CommandInput{}, Output: textOutput(serviceRequestFields(), CollectionCoverageExhaustive), Prerequisites: []string{"At least zero live attachment owners are discoverable from owner-only host state."}, Errors: serviceReadErrors("service requests", "review")}, handler: runServiceRequests}
}

func serviceAllowSpec() CommandSpec {
	return CommandSpec{Path: "service allow", Summary: "Allow one Workspace service request once", Args: "--id <request-ref>", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Create one attachment-owned loopback exposure from one freshly revalidated pending request", Inputs: []CommandInput{{Name: "--id", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Opaque live service-request reference.", AllowedValues: []string{}, ReferenceKind: tobari.ServiceRequestKind}}, Output: textOutput(exposureFields(), CollectionCoverageNotApplicable), Prerequisites: []string{"The request remains pending under its live attachment owner."}, Errors: serviceMutationErrors("service allow", "review"), Mutation: &MutationContract{TargetKind: tobari.ServiceExposureKind, TargetInputs: []string{"--id"}, ParentInput: "--id", Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}}, handler: runServiceAllow}
}

func serviceDenySpec() CommandSpec {
	return CommandSpec{Path: "service deny", Summary: "Deny one Workspace service request", Args: "--id <request-ref>", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Resolve one exact pending request without creating a listener", Inputs: []CommandInput{{Name: "--id", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Opaque live service-request reference.", AllowedValues: []string{}, ReferenceKind: tobari.ServiceRequestKind}}, Output: textOutput([]OutputField{{Name: "denied", Type: OutputFieldTypeBoolean, Description: "Always true after confirmed denial."}}, CollectionCoverageNotApplicable), Prerequisites: []string{"The request remains pending under its live attachment owner."}, Errors: serviceMutationErrors("service deny", "review"), Mutation: &MutationContract{TargetKind: tobari.ServiceRequestKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}}}, handler: runServiceDeny}
}
