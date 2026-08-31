package cli

import (
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func serviceExposureCommandSpecs() []CommandSpec {
	return []CommandSpec{
		exposureHelperRootSpec(), exposureHelperStatusSpec(), exposureHelperStopSpec(), exposureHelperHelpSpec(),
		serviceReviewSpec(), serviceStatusSpec(), serviceAllowSpec(), serviceDenySpec(), serviceOpenSpec(), serviceStopSpec(),
	}
}

func serviceOutput(fields []OutputField, coverage CollectionCoverage, envelope string) CommandOutput {
	return CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: fields, Delivery: OutputDeliveryComplete, CollectionCoverage: coverage, JSONEnvelope: envelope, JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1}
}

func serviceJSONOutput(fields []OutputField, coverage CollectionCoverage, envelope string) CommandOutput {
	return CommandOutput{Formats: []OutputFormat{OutputFormatJSON}, DefaultFormat: OutputFormatJSON, Fields: fields, Delivery: OutputDeliveryComplete, CollectionCoverage: coverage, JSONEnvelope: envelope, JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1}
}

func serviceIdentityFields() []OutputField {
	return []OutputField{
		{Name: "context", Type: OutputFieldTypeString, Description: "Template-derived display name of the owning Context."},
		{Name: "project_root", Type: OutputFieldTypeString, Description: "Canonical owning Project root."},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Exact final Context authority identity."},
		{Name: "workspace_id", Type: OutputFieldTypeString, Description: "Exact final Workspace authority identity."},
		{Name: "attachment_id", Type: OutputFieldTypeString, Description: "Exact Service-controller attachment epoch."},
	}
}

func exposureFields(reference bool) []OutputField {
	id := OutputField{Name: "id", Type: OutputFieldTypeString, Description: "Opaque attachment-owned exposure reference."}
	if reference {
		id.ReferenceKind = tobari.ServiceExposureKind
	}
	fields := []OutputField{id}
	fields = append(fields, serviceIdentityFields()...)
	return append(fields,
		OutputField{Name: "url", Type: OutputFieldTypeString, Description: "Exact generated per-exposure localhost root URL."},
		OutputField{Name: "target_port", Type: OutputFieldTypeInteger, Description: "Exact Workspace-loopback target port."},
		OutputField{Name: "host_port", Type: OutputFieldTypeInteger, Description: "OS-assigned IPv4 host-loopback port."},
		OutputField{Name: "state", Type: OutputFieldTypeString, Description: "Passive listener state.", Enum: []string{tobari.ServiceStateListening, tobari.ServiceStateRelaying, tobari.ServiceStateUnavailable}},
		OutputField{Name: "connections", Type: OutputFieldTypeInteger, Description: "Bounded active relay count."},
	)
}

func serviceRequestFields(reference bool) []OutputField {
	id := OutputField{Name: "id", Type: OutputFieldTypeString, Description: "Opaque live service-request reference."}
	if reference {
		id.ReferenceKind = tobari.ServiceRequestKind
	}
	fields := []OutputField{id}
	fields = append(fields, serviceIdentityFields()...)
	return append(fields,
		OutputField{Name: "target_port", Type: OutputFieldTypeInteger, Description: "Exact requested Workspace-loopback port."},
		OutputField{Name: "state", Type: OutputFieldTypeString, Description: "Request state.", Enum: []string{tobari.ServiceStatePending}},
	)
}

func serviceObservationFields() []OutputField {
	return []OutputField{
		{Name: "scope", Type: OutputFieldTypeString, Description: "Fixed host-wide live Service-owner scope."},
		{Name: "anchor", Type: OutputFieldTypeString, Description: "Opaque bounded observation anchor."},
		{Name: "coverage", Type: OutputFieldTypeString, Description: "Bounded owner-registry collection coverage.", Enum: []string{tobari.ServiceBoundedWindow}},
		{Name: "observation", Type: OutputFieldTypeString, Description: "Owner observation state.", Enum: []string{string(tobari.ServiceObservationComplete), string(tobari.ServiceObservationPartial), string(tobari.ServiceObservationUnavailable)}},
		{Name: "observed_owner_count", Type: OutputFieldTypeInteger, Description: "Bounded owners observed successfully."},
		{Name: "unavailable_owner_count", Type: OutputFieldTypeInteger, Description: "Bounded anchored owners unavailable during collection."},
	}
}

func serviceReviewFields() []OutputField {
	fields := serviceObservationFields()
	return append(fields, OutputField{Name: "requests", Type: OutputFieldTypeArray, Description: "Pending requests only.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact pending request.", Fields: serviceRequestFields(true)}})
}

func serviceStatusFields() []OutputField {
	fields := serviceObservationFields()
	return append(fields,
		OutputField{Name: "requests", Type: OutputFieldTypeArray, Description: "Pending requests.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact pending request.", Fields: serviceRequestFields(true)}},
		OutputField{Name: "exposures", Type: OutputFieldTypeArray, Description: "Active exposures.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact active exposure.", Fields: exposureFields(true)}},
	)
}

func helperStatusFields() []OutputField {
	return []OutputField{
		{Name: "scope", Type: OutputFieldTypeString, Description: "Fixed current-attachment scope."},
		{Name: "attachment_id", Type: OutputFieldTypeString, Description: "Exact current Service-controller attachment epoch."},
		{Name: "pending", Type: OutputFieldTypeArray, Description: "Current-attachment pending state without host mutation references.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One pending target.", Fields: []OutputField{{Name: "target_port", Type: OutputFieldTypeInteger, Description: "Exact Workspace-loopback target port."}, {Name: "state", Type: OutputFieldTypeString, Description: "Always pending.", Enum: []string{tobari.ServiceStatePending}}}}},
		{Name: "exposures", Type: OutputFieldTypeArray, Description: "Current-attachment active exposures with exact helper Stop refs.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One exact active exposure.", Fields: exposureFields(true)}},
	}
}

func serviceReadErrors(path, recovery string) []CommandError {
	return []CommandError{
		declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help "+path, "Correct the command arguments."),
		declaredCommandError(fault.KindContract, "invalid_service_review", false, recovery, "Retry from a fresh bounded owner snapshot."),
		declaredCommandError(fault.KindContract, "invalid_service_status", false, recovery, "Retry from a fresh bounded owner snapshot."),
		declaredCommandError(fault.KindContract, "invalid_service_attachment_status", false, recovery, "Retry from the current attachment."),
		declaredCommandError(fault.KindContract, "unsafe_service_owner", false, recovery, "Repair unsafe owner-only Service state outside this read."),
		declaredCommandError(fault.KindContract, "duplicate_service_owner", false, recovery, "Resolve contradictory Service owner authority."),
		declaredCommandError(fault.KindUnavailable, "service_observation_incomplete", false, recovery, "Retry from a fresh bounded owner snapshot."),
		classifiedCommandError(fault.KindUnavailable, "service_attachment_unavailable", false, fault.PhasePrecondition, fault.ChangeNone, recovery, "Use the helper from a live attached Workspace."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, recovery, "Use the command in its declared host or Workspace scope."),
		declaredCommandError(fault.KindInternal, "output_write_failed", true, path, "Retry with a writable output stream."),
		declaredCommandError(fault.KindCanceled, "operation_canceled", true, path, "Retry when the caller is ready."),
	}
}

func serviceMutationErrors(path, recovery string) []CommandError {
	return mutationCommandErrors(path, recovery,
		declaredCommandError(fault.KindInvalidInput, "invalid_service_port", false, "help "+path, "Choose an exact non-privileged Workspace port."),
		declaredCommandError(fault.KindInvalidInput, "invalid_service_request", false, recovery, "Use one opaque request reference unchanged."),
		declaredCommandError(fault.KindInvalidInput, "invalid_service_exposure", false, recovery, "Use one opaque exposure reference unchanged."),
		classifiedCommandError(fault.KindContract, "ambiguous_service_request", false, fault.PhasePrecondition, fault.ChangeNone, recovery, "Resolve contradictory live request ownership before retrying."),
		classifiedCommandError(fault.KindContract, "ambiguous_service_exposure", false, fault.PhasePrecondition, fault.ChangeNone, recovery, "Resolve contradictory live exposure ownership before retrying."),
		classifiedCommandError(fault.KindContract, "invalid_service_exposure_result", false, fault.PhaseVerification, fault.ChangeUnknown, recovery, "Reconcile the live attachment before another Service mutation."),
		declaredCommandError(fault.KindNotFound, "service_request_not_found", false, recovery, "Refresh the live service-request snapshot."),
		declaredCommandError(fault.KindNotFound, "service_exposure_not_found", false, recovery, "Refresh the live service status snapshot."),
		declaredCommandError(fault.KindRejected, "service_request_stale", false, recovery, "Refresh the live service-request snapshot."),
		declaredCommandError(fault.KindRejected, "service_exposure_stale", false, recovery, "Refresh the live service status snapshot."),
		declaredCommandError(fault.KindRejected, "service_request_denied", false, recovery, "Request exposure again if it is still needed."),
		declaredCommandError(fault.KindRejected, "service_attachment_closed", false, recovery, "Enter the Workspace again before requesting exposure."),
		classifiedCommandError(fault.KindUnavailable, "service_attachment_unavailable", false, fault.PhasePrecondition, fault.ChangeNone, recovery, "Use the helper from a live attached Workspace."),
		declaredCommandError(fault.KindUnavailable, "service_observation_incomplete", false, recovery, "Retry after every anchored owner can be revalidated."),
		declaredCommandError(fault.KindContract, "invalid_service_open_result", false, recovery, "Reconcile the active exposure before opening again."),
		declaredCommandError(fault.KindInternal, "service_instruction_write_failed", false, recovery, "Retry with a writable terminal before another request."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, recovery, "Use the command in its declared host or Workspace scope."),
	)
}

func exposureHelperRootSpec() CommandSpec {
	minimum, maximum := int64(tobari.ServicePortMinimum), int64(tobari.ServicePortMaximum)
	return CommandSpec{Program: ExposureProgramName, Path: ExposureProgramName, Summary: "Request one reviewed Workspace HTTP service", Args: "<port>", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{
		CapabilityID: "workspace.service-exposure", Outcome: "Request trusted-host approval for one exact Workspace-loopback HTTP port and return the confirmed attachment-owned exposure",
		Inputs: []CommandInput{{Name: "port", Source: InputSourceArgument, Required: true, ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle, Description: "Exact non-privileged Workspace-loopback service port.", AllowedValues: []string{}, Minimum: &minimum, Maximum: &maximum}},
		Output: serviceJSONOutput(exposureFields(true), CollectionCoverageNotApplicable, "exposure"), Prerequisites: []string{"Run inside one live interactive Workspace attachment; approval remains trusted-host-only."},
		FixedTarget: &FixedTarget{Kind: tobari.ServiceAttachmentServicesKind, ID: tobari.ServiceAttachmentServicesTargetID, Description: "The current Service-controller attachment request scope.", Scope: FixedTargetScopeToolLocal}, Errors: serviceMutationErrors(ExposureProgramName, "status"),
		Mutation: &MutationContract{TargetKind: tobari.ServiceAttachmentServicesKind, TargetInputs: []string{}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}},
	}, handler: runExposureRequest}
}

func exposureHelperStatusSpec() CommandSpec {
	return CommandSpec{Program: ExposureProgramName, Path: "status", Summary: "Show current-attachment service state", Args: "", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Return complete current-attachment pending and active state with exact active Stop references", Inputs: []CommandInput{}, Output: serviceJSONOutput(helperStatusFields(), CollectionCoverageExhaustive, "service_status"), Prerequisites: []string{"Run inside the same live Workspace attachment."}, Errors: serviceReadErrors("status", "help status")}, handler: runExposureStatus}
}

func exposureHelperStopSpec() CommandSpec {
	return CommandSpec{Program: ExposureProgramName, Path: "stop", Summary: "Stop one current-attachment service exposure", Args: "<exposure-ref>", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Close one exact attachment-owned host listener and its active relays", Inputs: []CommandInput{{Name: "exposure-ref", Source: InputSourceArgument, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Opaque exposure reference emitted by helper create or status.", AllowedValues: []string{}, ReferenceKind: tobari.ServiceExposureKind}}, Output: serviceJSONOutput([]OutputField{{Name: "stopped", Type: OutputFieldTypeBoolean, Description: "Always true after confirmed closure."}}, CollectionCoverageNotApplicable, "result"), Prerequisites: []string{"The opaque reference belongs to the current live attachment."}, Errors: serviceMutationErrors("stop", "status"), Mutation: &MutationContract{TargetKind: tobari.ServiceExposureKind, TargetInputs: []string{"exposure-ref"}, TargetIDInput: "exposure-ref", Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}}, handler: runExposureStop}
}

func exposureHelperHelpSpec() CommandSpec {
	return CommandSpec{Program: ExposureProgramName, Path: "help", Summary: "Show Workspace service helper help", Args: "[<command>...] [--format text|agent]", Effect: operation.EffectRead, Role: RoleUtility, Agent: AgentContract{CapabilityID: "cli.discovery", Outcome: "Discover only the attachment-local service helper commands and contracts", Inputs: []CommandInput{{Name: "command", Source: InputSourceArgument, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable, Description: "Select an exact helper command path.", AllowedValues: []string{}, Completion: InputCompletionCommand}, {Name: "--format", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Select human text or the machine-readable agent contract.", AllowedValues: []string{"text", "agent"}, DefaultValue: stringPointer("text")}}, Output: CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens, Fields: []OutputField{{Name: "path", Type: OutputFieldTypeString, Description: "Exact helper command path."}, {Name: "namespace", Type: OutputFieldTypeString, Description: "Canonical helper namespace."}, {Name: "summary", Type: OutputFieldTypeString, Description: "Command summary."}, {Name: "capability_id", Type: OutputFieldTypeString, Description: "Stable capability identity."}, {Name: "outcome", Type: OutputFieldTypeString, Description: "User outcome."}, {Name: "effect", Type: OutputFieldTypeString, Description: "Declared effect."}, {Name: "role", Type: OutputFieldTypeString, Description: "Declared role."}}, Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive, JSONEnvelope: "commands", JSONEnvelopeType: OutputFieldTypeArray, JSONSchemaVersion: 1}, Prerequisites: []string{}, Errors: []CommandError{declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help", "Use an exact helper command selector."), declaredCommandError(fault.KindContract, "output_encoding_failed", false, "help help", "Inspect helper help without repeating agent-help encoding."), declaredCommandError(fault.KindInternal, "output_write_failed", true, "help", "Retry with a writable output stream."), declaredCommandError(fault.KindCanceled, "operation_canceled", true, "help", "Retry when ready.")}}, handler: runHelp}
}

func serviceReviewWatchInput() CommandInput {
	return CommandInput{Name: "--watch", Source: InputSourceFlag, Required: false, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Keep the trusted-host Service review open and refresh bounded snapshots.", AllowedValues: []string{}, DefaultValue: stringPointer("false")}
}
func serviceReviewNotifyInput() CommandInput {
	return CommandInput{Name: "--notify", Source: InputSourceFlag, Required: false, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Choose the evidence-free terminal attention cue while watching.", AllowedValues: []string{"auto", "osc9", "bel", "off"}, DefaultValue: stringPointer("auto"), Requires: []string{"--watch"}}
}

func serviceReviewSpec() CommandSpec {
	return CommandSpec{Path: "review services", Summary: "Review Workspace service exposure requests", Args: "[--watch] [--notify auto|osc9|bel|off] [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Review pending Service requests; one complete effect card and one action key or line token is confirmation", Inputs: []CommandInput{serviceReviewWatchInput(), serviceReviewNotifyInput(), formatInput()}, Output: serviceOutput(serviceReviewFields(), CollectionCoverageBoundedWindow, "service_review"), Prerequisites: []string{"Actions, watch, and notifications require a trusted interactive text TTY; JSON and redirected operation are read-only."}, Errors: append(serviceReadErrors("review services", "service status"), declaredCommandError(fault.KindInvalidInput, "service_review_requires_tty", false, "help review services", "Read the trusted-terminal requirements before reviewing Services again.")), Interactive: &InteractiveWorkflowContract{ActionCommands: []string{"service allow", "service deny"}, SelectionReferenceKind: tobari.ServiceRequestKind, SelectionOutputField: "requests[].id", Confirmation: "explicit_action", NonInteractiveBehavior: "read_only"}}, handler: runServiceReview}
}

func serviceStatusSpec() CommandSpec {
	return CommandSpec{Path: "service status", Summary: "Show host-wide Workspace service state", Args: "[--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover, Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Return one bounded host-wide snapshot of pending requests and active exposures", Inputs: []CommandInput{formatInput()}, Output: serviceOutput(serviceStatusFields(), CollectionCoverageBoundedWindow, "service_status"), Prerequisites: []string{"Owner-only Service registry state is readable from the trusted host."}, Errors: serviceReadErrors("service status", "doctor")}, handler: runServiceStatus}
}

func serviceReferenceInput(kind string) CommandInput {
	return CommandInput{Name: "--id", Source: InputSourceFlag, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Opaque exact Service reference emitted by a declared producer.", AllowedValues: []string{}, ReferenceKind: kind}
}

func serviceAllowSpec() CommandSpec {
	return CommandSpec{Path: "service allow", Summary: "Allow one Workspace service request once", Args: "--id <request-ref> [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Create one attachment-owned exposure from one freshly revalidated pending request", Inputs: []CommandInput{serviceReferenceInput(tobari.ServiceRequestKind), formatInput()}, Output: serviceOutput(exposureFields(true), CollectionCoverageNotApplicable, "exposure"), Prerequisites: []string{"The request remains pending under its exact live owner."}, Errors: serviceMutationErrors("service allow", "review services"), Mutation: &MutationContract{TargetKind: tobari.ServiceExposureKind, TargetInputs: []string{"--id"}, ParentInput: "--id", Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}}, handler: runServiceAllow}
}

func serviceDenySpec() CommandSpec {
	return CommandSpec{Path: "service deny", Summary: "Deny one Workspace service request", Args: "--id <request-ref> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Resolve one exact pending request without creating a listener", Inputs: []CommandInput{serviceReferenceInput(tobari.ServiceRequestKind), formatInput()}, Output: serviceOutput([]OutputField{{Name: "denied", Type: OutputFieldTypeBoolean, Description: "Always true after confirmed denial."}}, CollectionCoverageNotApplicable, "result"), Prerequisites: []string{"The request remains pending under its exact live owner."}, Errors: serviceMutationErrors("service deny", "review services"), Mutation: &MutationContract{TargetKind: tobari.ServiceRequestKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}}}, handler: runServiceDeny}
}

func serviceOpenSpec() CommandSpec {
	return CommandSpec{Path: "service open", Summary: "Open one confirmed Workspace service exposure", Args: "--id <exposure-ref> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Ask the platform opener for the exact owner-derived exposure root URL", Inputs: []CommandInput{serviceReferenceInput(tobari.ServiceExposureKind), formatInput()}, Output: serviceOutput([]OutputField{{Name: "id", Type: OutputFieldTypeString, Description: "Consumed exposure identity."}, {Name: "url", Type: OutputFieldTypeString, Description: "Confirmed owner-derived exposure root URL."}, {Name: "outcome", Type: OutputFieldTypeString, Description: "Bounded platform dispatch outcome.", Enum: []string{string(tobari.ServiceOpenNotDispatched), string(tobari.ServiceOpenRequested), string(tobari.ServiceOpenOutcomeUnknown)}}}, CollectionCoverageNotApplicable, "open"), Prerequisites: []string{"The exposure remains active under its exact live owner."}, Errors: serviceMutationErrors("service open", "service status"), Mutation: &MutationContract{TargetKind: tobari.ServiceExposureKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationYes, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}}}, handler: runServiceOpen}
}

func serviceStopSpec() CommandSpec {
	return CommandSpec{Path: "service stop", Summary: "Stop one Workspace service exposure", Args: "--id <exposure-ref> [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct, Agent: AgentContract{CapabilityID: "workspace.service-exposure", Outcome: "Close one exact host listener and its relays through the live owner", Inputs: []CommandInput{serviceReferenceInput(tobari.ServiceExposureKind), formatInput()}, Output: serviceOutput([]OutputField{{Name: "stopped", Type: OutputFieldTypeBoolean, Description: "Always true after confirmed closure."}}, CollectionCoverageNotApplicable, "result"), Prerequisites: []string{"The exposure remains active under its exact live owner."}, Errors: serviceMutationErrors("service stop", "service status"), Mutation: &MutationContract{TargetKind: tobari.ServiceExposureKind, TargetInputs: []string{"--id"}, TargetIDInput: "--id", Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}}, handler: runServiceStop}
}
