package cli

import (
	"github.com/tasuku43/tobari/internal/app/authcmd"
	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

const authCapabilityID = "authentication.broker"

func authCommandSpecs() []CommandSpec {
	return []CommandSpec{authLoginSpec(), authImportSpec(), authStatusSpec(), authLogoutSpec()}
}

func authLoginSpec() CommandSpec {
	provider := CommandInput{
		Name: "--provider", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Required reviewed provider. First public V1 supports only GitHub through GitHub CLI (gh); omission is invalid and opens no selector.",
		AllowedValues: []string{authcmd.BuiltinGitHubProviderID},
	}
	return CommandSpec{
		Path: "auth login", Summary: "Authenticate one Context through a reviewed host CLI driver",
		Args: "--provider=github [--context <name>] [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: authCapabilityID,
			Outcome:      "Acquire one GitHub credential on the trusted host for an explicit or current Context without exposing it to a Workspace",
			Inputs: []CommandInput{
				provider,
				executionContextInput(),
				formatInput(),
			},
			Output: authResultOutput(),
			Prerequisites: []string{
				"The selected Context exists.",
				"The shared Auth Broker is already running, ready, and unlocked.",
				"The reviewed GitHub CLI executable is available through the trusted-host PATH from a conventional installation root (/bin, /usr/bin, /usr/local, /opt/homebrew, /opt/local, /nix/store, or /snap); project, temporary, and home-local executable paths are rejected.",
				"The caller has interactive terminal streams on stdin and stderr and can complete GitHub's device flow on the trusted host.",
			},
			FixedTarget: fixedAuthCatalogTarget(),
			Errors: authMutationErrors("auth login",
				declaredCommandError(fault.KindUnsupported, "provider_login_unsupported", false, "auth status", "Inspect the installed providers and their declared acquisition modes."),
				declaredCommandError(fault.KindInvalidInput, "auth_login_tty_required", false, "help auth login", "Run trusted-host provider login from an interactive terminal."),
				declaredCommandError(fault.KindUnavailable, "github_cli_unavailable", false, "auth login", "Install the reviewed GitHub CLI on the trusted host and retry."),
				declaredCommandError(fault.KindRejected, "github_login_cancelled", false, "auth login", "Retry the trusted-host GitHub login when ready."),
				declaredCommandError(fault.KindRejected, "github_login_failed", false, "auth login", "Retry the trusted-host GitHub login after inspecting the failure."),
			),
			Mutation: authMutationContract(),
		},
		handler: runAuthLogin,
	}
}

func authImportSpec() CommandSpec {
	return CommandSpec{
		Path: "auth import", Summary: "Import one provider credential through protected stdin",
		Args: "<provider> [--context <name>] [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: authCapabilityID,
			Outcome:      "Import one bounded opaque provider credential from stdin for an explicit or current Context without accepting it in argv or environment",
			Inputs: []CommandInput{
				authProviderInput("Installed provider manifest whose primary credential is supplied on stdin."),
				executionContextInput(),
				formatInput(),
				{
					Name: "credential", Source: InputSourceStdin, Required: true,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description: "One bounded opaque credential read only from stdin after mutation validation; never accepted in argv or environment.", AllowedValues: []string{},
				},
			},
			Output: authResultOutput(),
			Prerequisites: []string{
				"The selected Context and provider manifest exist.",
				"The shared Auth Broker is already running, ready, and unlocked.",
				"Exactly one credential is available on piped or redirected stdin; interactive terminal stdin is rejected before any credential bytes are read.",
			},
			FixedTarget: fixedAuthCatalogTarget(),
			Errors: authMutationErrors("auth import",
				declaredCommandError(fault.KindInvalidInput, "invalid_credential_input", false, "help auth import", "Provide one bounded credential through stdin."),
				declaredCommandError(fault.KindUnsupported, "provider_import_unsupported", false, "auth login", "Use the provider's reviewed built-in login helper."),
			),
			Mutation: authMutationContract(),
		},
		handler: runAuthImport,
	}
}

func authStatusSpec() CommandSpec {
	return CommandSpec{
		Path: "auth status", Summary: "Inspect one Context's Auth Broker state",
		Args: "[--context <name>] [--format text|json]", Effect: operation.EffectRead, Role: RoleUtility,
		Agent: AgentContract{
			CapabilityID: authCapabilityID,
			Outcome:      "Inspect the complete installed provider collection and Workspace activation state for an explicit or current Context without reading secret material",
			Inputs:       []CommandInput{executionContextInput(), formatInput()},
			Output:       authStatusOutput(),
			Prerequisites: []string{
				"The selected Context exists.",
			},
			Errors: readCommandErrors("auth status", true, append(authCommonErrors(),
				declaredCommandError(fault.KindUnavailable, "auth_status_failed", false, "doctor", "Inspect the Auth Broker and Context credential stores."),
			)...),
		},
		handler: runAuthStatus,
	}
}

func authLogoutSpec() CommandSpec {
	return CommandSpec{
		Path: "auth logout", Summary: "Remove one Context provider credential",
		Args: "<provider> [--context <name>] [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: authCapabilityID,
			Outcome:      "Confirm whether one Context-owned provider credential changed: remove and revoke it when present, or report no_change when already absent, without contacting the provider",
			Inputs:       []CommandInput{authProviderInput("Installed provider whose Context credential should be removed if present."), executionContextInput(), formatInput()},
			Output:       authResultOutput(),
			Prerequisites: []string{
				"The selected Context and provider manifest exist; the credential may already be absent.",
				"The shared Auth Broker is already running, ready, and unlocked.",
			},
			FixedTarget: fixedAuthCatalogTarget(),
			Errors:      authMutationErrors("auth logout"),
			Mutation:    authMutationContract(),
		},
		handler: runAuthLogout,
	}
}

func authProviderInput(description string) CommandInput {
	return CommandInput{
		Name: "provider", Source: InputSourceArgument, Required: true,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: description, AllowedValues: []string{},
	}
}

func fixedAuthCatalogTarget() *FixedTarget {
	return &FixedTarget{
		Kind: authbroker.CredentialCatalogTargetKind, ID: authbroker.CredentialCatalogTargetID,
		Description: "This installation's Context-scoped Auth Broker credential catalog.",
		Scope:       FixedTargetScopeToolLocal,
	}
}

func authMutationContract() *MutationContract {
	return &MutationContract{
		TargetKind: authbroker.CredentialCatalogTargetKind, TargetInputs: []string{},
		Impact: authcmd.MutationImpact(),
	}
}

func authResultOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "provider", Type: OutputFieldTypeString, Description: "Requested provider ID."},
			{Name: "context", Type: OutputFieldTypeString, Description: "Context display name selected by this task; it is not authority."},
			{Name: "context_state", Type: OutputFieldTypeString, Description: "Persisted Context authority for this mutation.", Enum: []string{"persisted"}},
			{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable host-resolved Context authority identity."},
			{Name: "configured", Type: OutputFieldTypeBoolean, Description: "Whether this Context currently has the reported provider credential."},
			{Name: "account_label", Type: OutputFieldTypeString, Description: "Secret-free provider account label when known, otherwise null.", Nullable: true},
			{Name: "storage_backend", Type: OutputFieldTypeString, Description: "Host root-key storage backend used for the encrypted Context vault.", Enum: []string{"macos_keychain", "xdg_file"}},
			{Name: "broker_state", Type: OutputFieldTypeString, Description: "Observed locked, ready, or unavailable Auth Broker state.", Enum: []string{"locked", "ready", "unavailable"}},
			{Name: "credential_revision", Type: OutputFieldTypeString, Description: "Opaque secret-free credential revision, or null when no credential is configured.", Nullable: true},
			{Name: "change", Type: OutputFieldTypeString, Description: "Confirmed mutation outcome: changed, or no_change only for an already-absent logout.", Enum: []string{"changed", "no_change"}},
			{Name: "workspace_activation", Type: OutputFieldTypeObject, Description: "Context-scoped Workspace projection observations; exact re-entry actions appear only for stale or missing projections.", Fields: workspaceActivationOutputFields()},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope: "auth", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1,
	}
}

func authStatusOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "context", Type: OutputFieldTypeString, Description: "Context display name selected by this task; it is not authority."},
			{Name: "context_state", Type: OutputFieldTypeString, Description: "Persisted authority or a display-only synthetic default.", Enum: []string{"persisted", "synthetic_default"}},
			{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable host-resolved Context authority identity, or null before authority is persisted.", Nullable: true},
			{Name: "storage_backend", Type: OutputFieldTypeString, Description: "Host root-key storage backend used for encrypted Context vaults.", Enum: []string{"macos_keychain", "xdg_file"}},
			{Name: "broker_state", Type: OutputFieldTypeString, Description: "Observed locked, ready, or unavailable Auth Broker state.", Enum: []string{"locked", "ready", "unavailable"}},
			{Name: "providers", Type: OutputFieldTypeArray, Description: "Complete installed provider collection with explicit configured, not_configured, or unavailable state plus configuration, account-label, and credential-revision facts.", SemanticScope: "Every installed provider for the selected Context at one observation.", Items: &OutputField{
				Type: OutputFieldTypeObject, Description: "One installed provider status.", Fields: []OutputField{
					{Name: "provider", Type: OutputFieldTypeString, Description: "Installed provider ID."},
					{Name: "state", Type: OutputFieldTypeString, Description: "Provider credential state.", Enum: []string{"configured", "not_configured", "unavailable"}},
					{Name: "account_label", Type: OutputFieldTypeString, Description: "Secret-free account label, or null.", Nullable: true},
					{Name: "credential_revision", Type: OutputFieldTypeString, Description: "Secret-free credential revision, or null.", Nullable: true},
				},
			}},
			{Name: "workspace_activation", Type: OutputFieldTypeObject, Description: "Context-scoped Workspace projection observations with explicit coverage; configured provider state alone does not imply re-entry.", Fields: workspaceActivationOutputFields()},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
		JSONEnvelope: "auth", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1,
	}
}

func workspaceActivationOutputFields() []OutputField {
	return []OutputField{
		{Name: "state", Type: OutputFieldTypeString, Description: "Aggregate Workspace projection activation state.", Enum: []string{"not_applicable", "ready", "workspace_reentry_required", "unavailable", "unresolved"}},
		{Name: "coverage", Type: OutputFieldTypeString, Description: "Whether eligible Workspace enumeration is exhaustive, unavailable, or not applicable.", Enum: []string{"not_applicable", "exhaustive", "unavailable"}},
		{Name: "context", Type: OutputFieldTypeString, Description: "Context display name bound to the observation; empty only when coverage is not_applicable."},
		{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable Context identity bound to the observation; empty only when coverage is not_applicable."},
		{Name: "workspaces", Type: OutputFieldTypeArray, Description: "Eligible logical Workspace observations for the exact Context when coverage is exhaustive.", SemanticScope: "Every project whose authoritative binding targets the selected Context.", Items: &OutputField{
			Type: OutputFieldTypeObject, Description: "One project-identified Workspace activation observation.", Fields: []OutputField{
				{Name: "project_id", Type: OutputFieldTypeString, Description: "Stable logical Workspace identity."},
				{Name: "root", Type: OutputFieldTypeString, Description: "Separately validated canonical working directory for this observation."},
				{Name: "context", Type: OutputFieldTypeString, Description: "Context display name bound to this row."},
				{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable Context identity bound to this row."},
				{Name: "scope_state", Type: OutputFieldTypeString, Description: "Whether all row authority facts were readable.", Enum: []string{"complete", "incomplete"}},
				{Name: "state", Type: OutputFieldTypeString, Description: "Workspace activation state derived from provider projections.", Enum: []string{"not_applicable", "ready", "workspace_reentry_required", "unavailable", "unresolved"}},
				{Name: "providers", Type: OutputFieldTypeArray, Description: "Provider projection observations for this Workspace.", SemanticScope: "Installed providers plus any safely observed stale registry provider IDs.", Items: &OutputField{
					Type: OutputFieldTypeObject, Description: "One provider projection observation.", Fields: []OutputField{
						{Name: "provider", Type: OutputFieldTypeString, Description: "Provider ID."},
						{Name: "state", Type: OutputFieldTypeString, Description: "Projection freshness.", Enum: []string{"not_applicable", "current", "missing", "stale", "unavailable"}},
					},
				}},
				{Name: "next_action", Type: OutputFieldTypeObject, Description: "Exact re-entry action for this row, otherwise null.", Nullable: true, Fields: []OutputField{
					{Name: "working_directory", Type: OutputFieldTypeString, Description: "Canonical directory from which to run argv."},
					{Name: "argv", Type: OutputFieldTypeArray, Description: "Exact argument vector, including executable and bound Context.", Items: &OutputField{Type: OutputFieldTypeString, Description: "One argv element."}},
				}},
			},
		}},
		{Name: "guidance", Type: OutputFieldTypeString, Description: "Stable guidance emitted only for an aggregate re-entry-required state."},
	}
}

func authMutationErrors(path string, extra ...CommandError) []CommandError {
	errors := append(authCommonErrors(), declaredCommandError(
		fault.KindInvalidInput, "invalid_provider", false, "auth status", "Inspect the Context's current authentication state.",
	))
	errors = append(errors, declaredCommandError(
		fault.KindContract,
		"auth_mutation_outcome_unknown",
		false,
		"auth status",
		"Reconcile the selected Context's credential state before another mutation.",
	))
	errors = append(errors, extra...)
	return mutationCommandErrors(path, "auth status", errors...)
}

func authCommonErrors() []CommandError {
	return []CommandError{
		declaredCommandError(fault.KindInvalidInput, "invalid_context_name", false, "context list", "Choose an existing Context name."),
		declaredCommandError(fault.KindNotFound, "context_not_found", false, "context list", "Choose an existing Context or create it first."),
		declaredCommandError(fault.KindUnavailable, "auth_broker_unavailable", true, "cluster up", "Start or reconcile the shared cluster before using authentication."),
		declaredCommandError(fault.KindUnavailable, "auth_broker_request_failed", false, "cluster up", "Reconcile the shared cluster before another Auth Broker request."),
		declaredCommandError(fault.KindUnavailable, "auth_broker_locked", false, "cluster up", "Reconcile the shared cluster and unlock the Auth Broker."),
		declaredCommandError(fault.KindUnavailable, "root_key_unavailable", false, "doctor", "Inspect the host root-key provider."),
		declaredCommandError(fault.KindRejected, "root_key_missing_with_vault", false, "doctor", "Restore the original root key or explicitly remove local authentication state."),
		declaredCommandError(fault.KindRejected, "root_key_unsafe", false, "doctor", "Repair unsafe root-key or Auth Broker state paths."),
		declaredCommandError(fault.KindUnavailable, "keychain_denied", false, "cluster up", "Allow trusted-host Keychain access and retry cluster reconciliation."),
		declaredCommandError(fault.KindRejected, "auth_vault_invalid", false, "doctor", "Inspect the Context vault integrity without printing its contents."),
		declaredCommandError(fault.KindUnsupported, "auth_vault_version_unsupported", false, "doctor", "Upgrade or repair the unsupported Context vault."),
		declaredCommandError(fault.KindRejected, "invalid_provider_manifest", false, "doctor", "Repair the owner-controlled provider manifest collection."),
		declaredCommandError(fault.KindRejected, "ambiguous_provider_http_binding", false, "doctor", "Remove the overlapping exact provider HTTP binding."),
		declaredCommandError(fault.KindNotFound, "provider_not_installed", false, "auth status", "Install or choose an available provider manifest."),
		declaredCommandError(fault.KindNotFound, "provider_credential_not_configured", false, "auth status", "Inspect the Context before choosing login or import."),
		declaredCommandError(fault.KindContract, "invalid_auth_broker_metadata", false, "doctor", "Inspect the Auth Broker and provider helper contract."),
		declaredCommandError(fault.KindContract, "invalid_auth_result", false, "auth status", "Reconcile the Context's authentication state before another mutation."),
		declaredCommandError(fault.KindContract, "output_encoding_failed", false, "auth status", "Repair the secret-free authentication JSON projection."),
		declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
	}
}
