//go:build tobari_dev && tobari_research

package cli

import (
	"strings"

	"github.com/tasuku43/tobari/internal/app/authcmd"
	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const authCapabilityID = "authentication.broker"

const (
	authContextRecoveryCommand = "context list"
	authContextRecoveryReason  = "Run context list, then select one exact Context with context use or pass it unchanged to auth status."
)

func authCommandSpecs() []CommandSpec {
	return []CommandSpec{authLoginSpec(), authImportSpec(), authStatusSpec(), authLogoutSpec()}
}

func finalAuthContextInput(description string) CommandInput {
	return CommandInput{
		Name: "--context", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: description, AllowedValues: []string{}, ReferenceKind: tobari.ContextReferenceKind,
	}
}

func authProviderInput(description string) CommandInput {
	return CommandInput{Name: "provider", Source: InputSourceArgument, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: description, AllowedValues: []string{}}
}

func authLoginSpec() CommandSpec {
	providerIDs := authbroker.ReviewedLoginProviderIDs()
	inputs := []CommandInput{
		finalAuthContextInput("Invocation-local opaque Context override; omission uses the installation current Context without observing CWD."),
		{Name: "--provider", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Reviewed provider to authenticate; omission opens the bounded interactive provider selector.", AllowedValues: providerIDs},
	}
	args := "[--context <context-ref>] [--provider " + strings.Join(providerIDs, "|") + "]"
	if authbroker.SupportsReviewedLoginProvider(authbroker.BuiltinAWSProviderID) {
		inputs = append(inputs, CommandInput{Name: "--method", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Reviewed AWS login method; requires an explicit provider.", AllowedValues: []string{string(authcmd.LoginMethodIdentityCenter), string(authcmd.LoginMethodConsole)}, Requires: []string{"--provider"}})
		args += " [--method identity-center|console]"
	}
	inputs = append(inputs, formatInput())
	return CommandSpec{
		Path: "auth login", Summary: "Rotate one final Context provider credential through a reviewed login driver",
		Args: args + " [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: authCapabilityID,
			Outcome:      "Acquire and durably bind one reviewed provider credential to the current or explicitly overridden final Context",
			Inputs:       inputs, Output: finalAuthResultOutput(),
			Prerequisites: []string{"The exact final Context and reviewed provider authority exist.", "The research Auth Broker is ready and unlocked.", "Interactive login has terminal stdin and stderr; Runtime-backed providers have exact immutable Runtime material."},
			Errors: authMutationErrors("auth login",
				declaredCommandError(fault.KindUnsupported, "provider_login_unsupported", false, authContextRecoveryCommand, authContextRecoveryReason),
				declaredCommandError(fault.KindInvalidInput, "auth_login_selector_unavailable", false, "help auth login", "Pass an explicit reviewed provider or use the interactive selector."),
				declaredCommandError(fault.KindInvalidInput, "auth_login_tty_required", false, "help auth login", "Run the reviewed login from an interactive terminal."),
			),
			Mutation: &MutationContract{TargetKind: authbroker.ContextCredentialTargetKind, TargetInputs: []string{"--context"}, ParentInput: "--context", CurrentContextFallback: true, Impact: authcmd.FinalContextMutationImpact()},
		},
		handler: runAuthLogin,
	}
}

func authImportSpec() CommandSpec {
	return CommandSpec{
		Path: "auth import", Summary: "Import one final Context provider credential through protected stdin",
		Args: "<provider> [--context <context-ref>] [--format text|json]", Effect: operation.EffectCreate, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: authCapabilityID,
			Outcome:      "Import one bounded credential from stdin and bind it to the current or explicitly overridden final Context",
			Inputs: []CommandInput{
				authProviderInput("Reviewed provider whose primary credential is supplied on stdin."),
				finalAuthContextInput("Invocation-local opaque Context override; omission uses the installation current Context without observing CWD."), formatInput(),
				{Name: "credential", Source: InputSourceStdin, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "One bounded opaque credential read only from non-terminal stdin.", AllowedValues: []string{}},
			},
			Output:        finalAuthResultOutput(),
			Prerequisites: []string{"The exact final Context and reviewed provider authority exist.", "The research Auth Broker is ready and unlocked.", "Exactly one credential is available on non-terminal stdin."},
			Errors: authMutationErrors("auth import",
				declaredCommandError(fault.KindInvalidInput, "invalid_credential_input", false, "help auth import", "Provide one bounded credential through stdin."),
				declaredCommandError(fault.KindUnsupported, "provider_import_unsupported", false, authContextRecoveryCommand, authContextRecoveryReason),
			),
			Mutation: &MutationContract{TargetKind: authbroker.ContextCredentialTargetKind, TargetInputs: []string{"--context"}, ParentInput: "--context", CurrentContextFallback: true, Impact: authcmd.FinalContextMutationImpact()},
		}, handler: runAuthImport,
	}
}

func authStatusSpec() CommandSpec {
	return CommandSpec{
		Path: "auth status", Summary: "Inspect one final Context credential inventory",
		Args: "[--context <context-ref>] [--format text|json]", Effect: operation.EffectRead, Role: RoleDiscover,
		Agent: AgentContract{
			CapabilityID:  authCapabilityID,
			Outcome:       "Return one bounded exhaustive secret-free provider inventory for the current or explicitly overridden final Context",
			Inputs:        []CommandInput{finalAuthContextInput("Invocation-local opaque Context override; omission uses the installation current Context without observing CWD."), formatInput()},
			Output:        finalAuthStatusOutput(),
			Prerequisites: []string{"The exact final Context exists; status performs no mutation and creates no lifecycle state."},
			Errors: readCommandErrors("auth status", true,
				declaredCommandError(fault.KindRejected, "current_context_required", false, "context list", "Discover a Context, then select it with context use or pass an explicit override."),
				declaredCommandError(fault.KindInvalidInput, "invalid_context_ref", false, "context list", "Choose one current Context reference."),
				declaredCommandError(fault.KindUnavailable, "auth_status_failed", false, "doctor", "Inspect final Context and research Auth Broker authority."),
				declaredCommandError(fault.KindRejected, "legacy_state_present", false, "doctor", "Reset or recreate this pre-release installation before using final authority."),
			),
		}, handler: runAuthStatus,
	}
}

func authLogoutSpec() CommandSpec {
	return CommandSpec{
		Path: "auth logout", Summary: "Remove one final Context provider credential",
		Args: "<provider> [--context <context-ref>] [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID:  authCapabilityID,
			Outcome:       "Remove one exact Context-bound provider credential, or confirm it is already absent, without contacting the provider",
			Inputs:        []CommandInput{authProviderInput("Reviewed provider credential to remove."), finalAuthContextInput("Invocation-local opaque Context override; omission uses the installation current Context without observing CWD."), formatInput()},
			Output:        finalAuthResultOutput(),
			Prerequisites: []string{"The exact final Context and reviewed provider authority exist.", "The research Auth Broker is ready and unlocked; the credential itself may already be absent."},
			Errors:        authMutationErrors("auth logout"),
			Mutation:      &MutationContract{TargetKind: tobari.ContextReferenceKind, TargetInputs: []string{"--context"}, TargetIDInput: "--context", CurrentContextFallback: true, Impact: authcmd.FinalContextMutationImpact()},
		}, handler: runAuthLogout,
	}
}

func finalAuthResultOutput() CommandOutput {
	return CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "task", Type: OutputFieldTypeString, Description: "Exact completed authentication task.", Enum: []string{authbroker.TaskLogin, authbroker.TaskImport, authbroker.TaskLogout}},
			{Name: "provider", Type: OutputFieldTypeString, Description: "Reviewed provider ID."},
			{Name: "configured", Type: OutputFieldTypeBoolean, Description: "Whether this Context currently has the credential."},
			{Name: "account_label", Type: OutputFieldTypeString, Description: "Secret-free account label when configured and known.", Nullable: true},
			{Name: "storage_backend", Type: OutputFieldTypeString, Description: "Owner-only encrypted credential storage backend.", Enum: []string{"macos_keychain", "xdg_file"}},
			{Name: "broker_state", Type: OutputFieldTypeString, Description: "Exact Broker state.", Enum: []string{"ready"}},
			{Name: "credential_revision", Type: OutputFieldTypeString, Description: "Secret-free credential revision, empty when absent."},
			{Name: "change", Type: OutputFieldTypeString, Description: "Confirmed mutation disposition.", Enum: []string{"changed", "no_change"}},
		}, Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable, JSONEnvelope: "auth", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 2}
}

func finalAuthStatusOutput() CommandOutput {
	return CommandOutput{Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "task", Type: OutputFieldTypeString, Description: "Exact authentication status task.", Enum: []string{authbroker.TaskStatus}},
			{Name: "context_ref", Type: OutputFieldTypeString, Description: "Unchanged opaque Context reference.", ReferenceKind: tobari.ContextReferenceKind},
			{Name: "storage_backend", Type: OutputFieldTypeString, Description: "Owner-only encrypted credential storage backend.", Enum: []string{"macos_keychain", "xdg_file"}},
			{Name: "broker_state", Type: OutputFieldTypeString, Description: "Observed Broker state.", Enum: []string{"locked", "ready", "unavailable"}},
			{Name: "providers", Type: OutputFieldTypeArray, Description: "Bounded exhaustive Context-scoped provider inventory.", SemanticScope: "Every credential or reviewed provider authority for the exact Context at one coherent observation.", Items: &OutputField{Type: OutputFieldTypeObject, Description: "One secret-free provider status.", Fields: []OutputField{
				{Name: "provider", Type: OutputFieldTypeString, Description: "Reviewed or retained credential provider ID."},
				{Name: "state", Type: OutputFieldTypeString, Description: "Credential state.", Enum: []string{"configured", "not_configured", "unavailable"}},
				{Name: "account_label", Type: OutputFieldTypeString, Description: "Secret-free account label when configured and known.", Nullable: true},
				{Name: "credential_revision", Type: OutputFieldTypeString, Description: "Secret-free credential revision, empty when absent."},
			}}},
		}, Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive, JSONEnvelope: "auth", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 2}
}

func authMutationErrors(path string, extras ...CommandError) []CommandError {
	base := []CommandError{
		declaredCommandError(fault.KindRejected, "current_context_required", false, "context list", "Discover a Context, then select it with context use or pass an explicit override."),
		declaredCommandError(fault.KindInvalidInput, "invalid_context_ref", false, authContextRecoveryCommand, authContextRecoveryReason),
		declaredCommandError(fault.KindInvalidInput, "invalid_provider", false, authContextRecoveryCommand, authContextRecoveryReason),
		declaredCommandError(fault.KindNotFound, "provider_not_installed", false, authContextRecoveryCommand, authContextRecoveryReason),
		declaredCommandError(fault.KindUnavailable, "research_auth_mutation_interrupted", false, authContextRecoveryCommand, authContextRecoveryReason),
		declaredCommandError(fault.KindUnavailable, "research_auth_result_delivery_interrupted", false, authContextRecoveryCommand, authContextRecoveryReason),
		declaredCommandError(fault.KindRejected, "legacy_state_present", false, "doctor", "Reset or recreate this pre-release installation before using final authority."),
	}
	errors := mutationCommandErrors(path, authContextRecoveryCommand, append(base, extras...)...)
	for errorIndex := range errors {
		for actionIndex := range errors[errorIndex].NextActions {
			if errors[errorIndex].NextActions[actionIndex].Command == authContextRecoveryCommand {
				errors[errorIndex].NextActions[actionIndex].Reason = authContextRecoveryReason
			}
		}
	}
	return errors
}
