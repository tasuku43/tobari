package cli

import (
	"context"
	"encoding/json"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

func runAuthLogin(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.auth == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help auth login", "Correct the command arguments.")
	}
	contextName, err := selectedAuthContext(ctx, inputs)
	if err != nil {
		return c.fail(ctx, err)
	}
	bindAuthMutationIntent(&intent, command)
	result, err := c.auth.Login(ctx, intent, contextName, inputs.One("provider"), c.In, c.Err)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderAuthResult(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runAuthImport(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.auth == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help auth import", "Correct the command arguments.")
	}
	contextName, err := selectedAuthContext(ctx, inputs)
	if err != nil {
		return c.fail(ctx, err)
	}
	bindAuthMutationIntent(&intent, command)
	result, err := c.auth.Import(ctx, intent, contextName, inputs.One("provider"), c.In)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderAuthResult(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runAuthStatus(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.auth == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help auth status", "Correct the command arguments.")
	}
	contextName, err := selectedAuthContext(ctx, inputs)
	if err != nil {
		return c.fail(ctx, err)
	}
	result, err := c.auth.Status(ctx, contextName)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderAuthStatus(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runAuthLogout(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.auth == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help auth logout", "Correct the command arguments.")
	}
	contextName, err := selectedAuthContext(ctx, inputs)
	if err != nil {
		return c.fail(ctx, err)
	}
	bindAuthMutationIntent(&intent, command)
	result, err := c.auth.Logout(ctx, intent, contextName, inputs.One("provider"))
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderAuthResult(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func bindAuthMutationIntent(intent *operation.Intent, command CommandSpec) {
	intent.Target = operation.TargetRef{
		Kind: authbroker.CredentialCatalogTargetKind,
		ID:   authbroker.CredentialCatalogTargetID,
	}
	intent.Impact = command.Agent.Mutation.Impact
}

func selectedAuthContext(ctx context.Context, inputs ParsedInputs) (string, error) {
	if inputs.Provided("--context") {
		name := inputs.One("--context")
		if name == "" {
			return "", fault.New(
				fault.KindInvalidInput,
				"invalid_context_name",
				"Context name is invalid.",
				false,
				fault.NextAction{Command: "context list", Reason: "Choose an existing Context name."},
			)
		}
		return name, nil
	}
	return executionContextName(ctx), nil
}

type authResultProjection struct {
	Provider            string                         `json:"provider"`
	Context             string                         `json:"context"`
	ContextID           string                         `json:"context_id"`
	Configured          bool                           `json:"configured"`
	AccountLabel        *string                        `json:"account_label"`
	StorageBackend      authbroker.StorageBackend      `json:"storage_backend"`
	BrokerState         authbroker.BrokerState         `json:"broker_state"`
	CredentialRevision  string                         `json:"credential_revision"`
	WorkspaceActivation authbroker.WorkspaceActivation `json:"workspace_activation"`
}

type authResultDocument struct {
	SchemaVersion int                  `json:"schema_version"`
	Auth          authResultProjection `json:"auth"`
}

type authStatusProjection struct {
	Context             string                         `json:"context"`
	ContextID           string                         `json:"context_id"`
	StorageBackend      authbroker.StorageBackend      `json:"storage_backend"`
	BrokerState         authbroker.BrokerState         `json:"broker_state"`
	Providers           []authbroker.ProviderStatus    `json:"providers"`
	WorkspaceActivation authbroker.WorkspaceActivation `json:"workspace_activation"`
}

type authStatusDocument struct {
	SchemaVersion int                  `json:"schema_version"`
	Auth          authStatusProjection `json:"auth"`
}

func renderAuthResult(result authbroker.Result, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(
			fault.KindContract,
			"invalid_auth_result",
			"Authentication result is invalid.",
			false,
			err,
			fault.NextAction{Command: "auth status", Reason: "Reconcile the Context's authentication state before another mutation."},
		)
	}
	projection := authResultProjection{
		Provider: result.Provider, Context: result.Context, ContextID: result.ContextID,
		Configured: result.Configured, AccountLabel: result.AccountLabel,
		StorageBackend: result.StorageBackend, BrokerState: result.BrokerState,
		CredentialRevision: result.CredentialRevision, WorkspaceActivation: result.WorkspaceActivation,
	}
	if format == successFormatJSON {
		output, err := json.Marshal(authResultDocument{SchemaVersion: 1, Auth: projection})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Authentication output could not be encoded.", false, err)
		}
		return append(output, '\n'), nil
	}
	return renderAuthResultText(projection, color), nil
}

func renderAuthStatus(result authbroker.StatusResult, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(
			fault.KindContract,
			"invalid_auth_result",
			"Authentication status result is invalid.",
			false,
			err,
			fault.NextAction{Command: "auth status", Reason: "Reconcile the Context's authentication state before another mutation."},
		)
	}
	projection := authStatusProjection{
		Context: result.Context, ContextID: result.ContextID,
		StorageBackend: result.StorageBackend, BrokerState: result.BrokerState,
		Providers:           append([]authbroker.ProviderStatus{}, result.Providers...),
		WorkspaceActivation: result.WorkspaceActivation,
	}
	if format == successFormatJSON {
		output, err := json.Marshal(authStatusDocument{SchemaVersion: 1, Auth: projection})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Authentication status output could not be encoded.", false, err)
		}
		return append(output, '\n'), nil
	}
	return renderAuthStatusText(projection, color), nil
}

func renderAuthResultText(result authResultProjection, color bool) []byte {
	output := newHumanOutput(color)
	if result.Configured {
		output.heading("✓", "Context authentication configured", styleSuccess)
	} else {
		output.heading("○", "Context authentication not configured", styleMuted)
	}
	output.row("Context", safeExternalText(result.Context), styleText)
	output.row("Context ID", result.ContextID, styleText)
	provider := result.Provider
	if provider == "" {
		provider = "none"
	}
	output.row("Provider", safeExternalText(provider), styleText)
	output.row("Configured", humanBool(result.Configured), humanOutcomeBoolToken(result.Configured))
	account := "not available"
	if result.AccountLabel != nil {
		account = safeExternalText(*result.AccountLabel)
	}
	output.row("Account", account, styleText)
	output.row("Storage", string(result.StorageBackend), styleText)
	output.row("Broker", string(result.BrokerState), humanStatusToken(string(result.BrokerState)))
	revision := result.CredentialRevision
	if revision == "" {
		revision = "none"
	}
	output.row("Revision", revision, styleText)
	output.row("Workspaces", string(result.WorkspaceActivation.State), humanStatusToken(string(result.WorkspaceActivation.State)))
	if result.WorkspaceActivation.Guidance != "" {
		output.row("Guidance", safeExternalText(result.WorkspaceActivation.Guidance), styleText)
	}
	return output.bytes()
}

func renderAuthStatusText(result authStatusProjection, color bool) []byte {
	output := newHumanOutput(color)
	output.heading("○", "Context authentication status", styleText)
	output.row("Context", safeExternalText(result.Context), styleText)
	output.row("Context ID", result.ContextID, styleText)
	output.row("Storage", string(result.StorageBackend), styleText)
	output.row("Broker", string(result.BrokerState), humanStatusToken(string(result.BrokerState)))
	output.row("Workspaces", string(result.WorkspaceActivation.State), humanStatusToken(string(result.WorkspaceActivation.State)))
	if result.WorkspaceActivation.Guidance != "" {
		output.row("Guidance", safeExternalText(result.WorkspaceActivation.Guidance), styleText)
	}
	if len(result.Providers) == 0 {
		output.empty("No authentication providers installed", "The Context provider collection is explicitly empty.", "", "")
		return output.bytes()
	}
	for _, provider := range result.Providers {
		output.section("Provider: " + safeExternalText(provider.Provider))
		output.row("State", string(provider.State), humanStatusToken(string(provider.State)))
		output.row("Configured", humanBool(provider.Configured), humanOutcomeBoolToken(provider.Configured))
		account := "not available"
		if provider.AccountLabel != nil {
			account = safeExternalText(*provider.AccountLabel)
		}
		output.row("Account", account, styleText)
		revision := provider.CredentialRevision
		if revision == "" {
			revision = "none"
		}
		output.row("Revision", revision, styleText)
	}
	return output.bytes()
}
