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
		Name: "--provider", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Credential provider to authenticate. Omission opens an interactive provider selector. Each current reviewed built-in has one supported Workspace client tool, displayed and selected automatically: github uses GitHub CLI (gh); aws uses AWS CLI (aws); datadog uses pup. AWS --method selects acquisition inside the aws pairing, not another tool.",
		AllowedValues: []string{},
	}
	return CommandSpec{
		Path: "auth login", Summary: "Authenticate one Context through a reviewed host CLI driver",
		Args: "[--provider <provider>] [--method identity-center|console] [--context <name>] [--format text|json]", Effect: operation.EffectWrite, Role: RoleAct,
		Agent: AgentContract{
			CapabilityID: authCapabilityID,
			Outcome:      "Acquire one supported provider credential on the trusted host for an explicit or current Context without exposing it to a Workspace",
			Inputs: []CommandInput{
				provider,
				{
					Name: "--method", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description:   "AWS login method. Omission selects identity-center for backward compatibility; console selects AWS CLI console-based local-development login. The flag requires an explicit provider and is invalid for other providers.",
					AllowedValues: []string{string(authcmd.LoginMethodIdentityCenter), string(authcmd.LoginMethodConsole)},
					Requires:      []string{"--provider"},
				},
				executionContextInput(),
				formatInput(),
			},
			Output: authResultOutput(),
			Prerequisites: []string{
				"The selected Context exists.",
				"The shared Auth Broker is already running, ready, and unlocked.",
				"When provider is omitted, stdin and stderr are interactive terminals and the caller explicitly selects one installed reviewed login provider.",
				"The reviewed GitHub CLI, AWS CLI, or pup executable is available through the trusted-host PATH from a conventional installation root (/bin, /usr/bin, /usr/local, /opt/homebrew, /opt/local, /nix/store, or /snap); project, temporary, and home-local executable paths are rejected.",
				"The caller has interactive terminal streams on stdin and stderr and can complete the selected github, aws, or datadog provider flow on the trusted host.",
				"For aws identity-center, the caller knows the access-portal start URL, SSO region, 12-digit account ID, and role name; request region remains ordinary Context or command configuration.",
				"For aws console, AWS CLI 2.32 or newer is installed, the caller chooses one commercial AWS region, and the caller can paste the browser authorization code into the trusted-host terminal.",
			},
			FixedTarget: fixedAuthCatalogTarget(),
			Errors: authMutationErrors("auth login",
				declaredCommandError(fault.KindUnsupported, "provider_login_unsupported", false, "help auth import", "Import one credential through protected stdin instead."),
				declaredCommandError(fault.KindInvalidInput, "auth_login_tty_required", false, "help auth login", "Run trusted-host provider login from an interactive terminal."),
				declaredCommandError(fault.KindUnavailable, "github_cli_unavailable", false, "auth login", "Install the reviewed GitHub CLI on the trusted host and retry."),
				declaredCommandError(fault.KindRejected, "github_login_cancelled", false, "auth login", "Retry the trusted-host GitHub login when ready."),
				declaredCommandError(fault.KindRejected, "github_login_failed", false, "auth login", "Retry the trusted-host GitHub login after inspecting the failure."),
				declaredCommandError(fault.KindUnavailable, "datadog_cli_unavailable", false, "auth login", "Install the reviewed pup CLI on the trusted host and retry."),
				declaredCommandError(fault.KindRejected, "datadog_login_cancelled", false, "auth login", "Retry the trusted-host Datadog login when ready."),
				declaredCommandError(fault.KindRejected, "datadog_login_timeout", false, "auth login", "Start a new Datadog OAuth login and complete it within the bounded window."),
				declaredCommandError(fault.KindUnavailable, "datadog_login_failed", false, "auth login", "Retry the isolated trusted-host pup login after inspecting the failure."),
				declaredCommandError(fault.KindUnavailable, "aws_cli_unavailable", false, "auth login", "Install the reviewed AWS CLI on the trusted host and retry."),
				declaredCommandError(fault.KindInvalidInput, "auth_login_method_not_applicable", false, "help auth login", "Remove --method for non-AWS providers."),
				declaredCommandError(fault.KindUnsupported, "aws_console_login_unsupported", false, "auth login", "Install AWS CLI 2.32 or newer on the trusted host, then retry console login."),
				declaredCommandError(fault.KindInvalidInput, "aws_console_config_invalid", false, "help auth login", "Provide a valid commercial AWS region for console login."),
				declaredCommandError(fault.KindRejected, "aws_console_login_cancelled", false, "auth login", "Retry the trusted-host AWS console login when ready."),
				declaredCommandError(fault.KindRejected, "aws_console_login_timeout", false, "auth login", "Start a new AWS console login and complete it within the bounded window."),
				declaredCommandError(fault.KindUnavailable, "aws_console_login_failed", false, "auth login", "Retry the trusted-host AWS console login after inspecting the failure."),
				declaredCommandError(fault.KindRejected, "aws_sso_login_cancelled", false, "auth login", "Retry the trusted-host AWS IAM Identity Center login when ready."),
				declaredCommandError(fault.KindInvalidInput, "aws_sso_config_invalid", false, "help auth login", "Provide valid AWS IAM Identity Center login fields."),
				declaredCommandError(fault.KindRejected, "aws_sso_login_timeout", false, "auth login", "Start a new AWS IAM Identity Center device login and complete it within the bounded window."),
				declaredCommandError(fault.KindUnavailable, "aws_sso_login_failed", false, "auth login", "Retry the trusted-host AWS IAM Identity Center login after inspecting the failure."),
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
			Outcome:      "Remove one Context-owned provider credential and revoke its Workspace handles without contacting the provider",
			Inputs:       []CommandInput{authProviderInput("Configured provider credential to remove."), executionContextInput(), formatInput()},
			Output:       authResultOutput(),
			Prerequisites: []string{
				"The selected Context and provider credential exist.",
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
			{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable host-resolved Context authority identity."},
			{Name: "configured", Type: OutputFieldTypeBoolean, Description: "Whether this Context currently has the reported provider credential."},
			{Name: "account_label", Type: OutputFieldTypeString, Description: "Secret-free provider account label when known, otherwise null."},
			{Name: "storage_backend", Type: OutputFieldTypeString, Description: "Host root-key storage backend used for the encrypted Context vault."},
			{Name: "broker_state", Type: OutputFieldTypeString, Description: "Observed locked, ready, or unavailable Auth Broker state."},
			{Name: "credential_revision", Type: OutputFieldTypeString, Description: "Opaque secret-free credential revision, or empty when no credential is configured."},
			{Name: "workspace_activation", Type: OutputFieldTypeObject, Description: "Explicit activation state and guidance that credential ownership is Context-wide, each permanently bound project receives a distinct handle, and existing sessions must leave and re-enter."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope: "auth", JSONSchemaVersion: 1,
	}
}

func authStatusOutput() CommandOutput {
	return CommandOutput{
		Formats: []OutputFormat{OutputFormatText, OutputFormatJSON}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
		Fields: []OutputField{
			{Name: "context", Type: OutputFieldTypeString, Description: "Context display name selected by this task; it is not authority."},
			{Name: "context_id", Type: OutputFieldTypeString, Description: "Stable host-resolved Context authority identity."},
			{Name: "storage_backend", Type: OutputFieldTypeString, Description: "Host root-key storage backend used for encrypted Context vaults."},
			{Name: "broker_state", Type: OutputFieldTypeString, Description: "Observed locked, ready, or unavailable Auth Broker state."},
			{Name: "providers", Type: OutputFieldTypeArray, Description: "Complete installed provider collection with explicit configured, not_configured, or unavailable state plus configuration, account-label, and credential-revision facts."},
			{Name: "workspace_activation", Type: OutputFieldTypeObject, Description: "When any provider is configured, guidance that credential ownership is Context-wide, each permanently bound project receives a distinct handle, and existing sessions must leave and re-enter."},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
		JSONEnvelope: "auth", JSONSchemaVersion: 1,
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
