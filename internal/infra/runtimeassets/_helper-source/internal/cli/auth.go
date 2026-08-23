package cli

import (
	"context"
	"strings"

	"github.com/tasuku43/tobari/internal/app/authcmd"
	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
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
	if err := intent.Validate(); err != nil {
		return c.fail(ctx, fault.Wrap(
			fault.KindContract,
			"invalid_mutation_contract",
			"Authentication mutation contract is invalid.",
			false,
			err,
		))
	}
	provider := inputs.One("--provider")
	if !inputs.Provided("--provider") {
		var selectedContext string
		provider, selectedContext, err = c.selectAuthLoginProvider(ctx, contextName)
		if err != nil {
			return c.fail(ctx, err)
		}
		contextName = selectedContext
	}
	result, err := c.auth.Login(
		ctx, intent, contextName, provider, inputs.One("--method"), c.In, c.Err,
	)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderAuthResult(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func (c *CLI) selectAuthLoginProvider(ctx context.Context, contextName string) (string, string, error) {
	if invocationErrorFormat(ctx) == errorFormatJSON {
		return "", "", fault.New(
			fault.KindInvalidInput,
			"auth_login_selector_unavailable",
			"Omitted provider selection requires human text error output on an interactive terminal.",
			false,
			fault.NextAction{Command: "help auth login", Reason: "Pass an explicit reviewed provider or use human text error output for provider selection."},
		)
	}
	if err := c.auth.ValidateLoginTerminal(c.In, c.Err); err != nil {
		return "", "", err
	}
	status, err := c.auth.Status(ctx, contextName)
	if err != nil {
		return "", "", err
	}
	providers := make([]authbroker.ProviderStatus, 0, len(status.Providers))
	for _, provider := range status.Providers {
		if authcmd.SupportsLoginProvider(provider.Provider) {
			providers = append(providers, provider)
		}
	}
	if len(providers) == 0 {
		return "", "", fault.New(
			fault.KindUnsupported,
			"provider_login_unsupported",
			"No installed provider supports the reviewed interactive login flow.",
			false,
			fault.NextAction{Command: "auth status", Reason: "Inspect the installed providers and their declared acquisition modes."},
		)
	}
	if c.authLogin == nil {
		return "", "", fault.New(
			fault.KindContract,
			"invalid_auth_result",
			"Authentication provider selection is not configured.",
			false,
			fault.NextAction{Command: "auth status", Reason: "Reconcile the Workspace Manifest's authentication state before another mutation."},
		)
	}
	selected, err := c.authLogin.Select(ctx, status.Context, providers, c.In, c.Err)
	if err != nil {
		return "", "", err
	}
	for _, provider := range providers {
		if selected == provider.Provider {
			return selected, status.Context, nil
		}
	}
	return "", "", fault.New(
		fault.KindContract,
		"invalid_auth_result",
		"Authentication provider selection did not match the installed provider snapshot.",
		false,
		fault.NextAction{Command: "auth status", Reason: "Reconcile the Workspace Manifest's authentication state before another mutation."},
	)
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
	if inputs.Provided("--manifest") {
		name := inputs.One("--manifest")
		if name == "" {
			return "", fault.New(
				fault.KindInvalidInput,
				"invalid_manifest_name",
				"Workspace Manifest name is invalid.",
				false,
				fault.NextAction{Command: "manifest list", Reason: "Choose an existing Workspace Manifest name."},
			)
		}
		return name, nil
	}
	return executionContextName(ctx), nil
}

type authResultProjection struct {
	ManifestState       tobari.ManifestObservationState `json:"workspace_manifest_state"`
	Provider            string                          `json:"provider"`
	Context             string                          `json:"workspace_manifest"`
	WorkspaceManifestID *string                         `json:"workspace_manifest_id"`
	Configured          bool                            `json:"configured"`
	AccountLabel        *string                         `json:"account_label"`
	StorageBackend      authbroker.StorageBackend       `json:"storage_backend"`
	BrokerState         authbroker.BrokerState          `json:"broker_state"`
	CredentialRevision  *string                         `json:"credential_revision"`
	Change              authbroker.MutationChange       `json:"change"`
	WorkspaceActivation authbroker.WorkspaceActivation  `json:"workspace_activation"`
}

type authResultDocument struct {
	SchemaVersion int                  `json:"schema_version"`
	Auth          authResultProjection `json:"auth"`
}

type authStatusProjection struct {
	ManifestState       tobari.ManifestObservationState `json:"workspace_manifest_state"`
	Context             string                          `json:"workspace_manifest"`
	WorkspaceManifestID *string                         `json:"workspace_manifest_id"`
	StorageBackend      authbroker.StorageBackend       `json:"storage_backend"`
	BrokerState         authbroker.BrokerState          `json:"broker_state"`
	DeclaredBindings    authbroker.AuthenticationRoute  `json:"declared_bindings"`
	UndeclaredBindings  authbroker.AuthenticationRoute  `json:"undeclared_bindings"`
	Providers           []authProviderStatusProjection  `json:"providers"`
	WorkspaceActivation authbroker.WorkspaceActivation  `json:"workspace_activation"`
}

type authProviderStatusProjection struct {
	Provider           string                             `json:"provider"`
	State              authbroker.ProviderCredentialState `json:"state"`
	AccountLabel       *string                            `json:"account_label"`
	CredentialRevision *string                            `json:"credential_revision"`
}

type authStatusDocument struct {
	SchemaVersion int                  `json:"schema_version"`
	Auth          authStatusProjection `json:"auth"`
}

func optionalDisplay(value *string, absent string) string {
	if value == nil {
		return absent
	}
	return *value
}

func renderAuthResult(result authbroker.Result, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(
			fault.KindContract,
			"invalid_auth_result",
			"Authentication result is invalid.",
			false,
			err,
			fault.NextAction{Command: "auth status", Reason: "Reconcile the Workspace Manifest's authentication state before another mutation."},
		)
	}
	projection := authResultProjection{
		ManifestState: result.ManifestState, Provider: result.Provider, Context: result.Context, WorkspaceManifestID: optionalString(result.WorkspaceManifestID),
		Configured: result.Configured, AccountLabel: result.AccountLabel,
		StorageBackend: result.StorageBackend, BrokerState: result.BrokerState,
		CredentialRevision: optionalString(result.CredentialRevision), Change: result.Change,
		WorkspaceActivation: result.WorkspaceActivation,
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON(authResultCommand(result.Task), authResultDocument{SchemaVersion: 1, Auth: projection})
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
			fault.NextAction{Command: "auth status", Reason: "Reconcile the Workspace Manifest's authentication state before another mutation."},
		)
	}
	providers := make([]authProviderStatusProjection, 0, len(result.Providers))
	for _, provider := range result.Providers {
		providers = append(providers, authProviderStatusProjection{
			Provider: provider.Provider, State: provider.State,
			AccountLabel: provider.AccountLabel, CredentialRevision: optionalString(provider.CredentialRevision),
		})
	}
	projection := authStatusProjection{
		ManifestState: result.ManifestState, Context: result.Context, WorkspaceManifestID: optionalString(result.WorkspaceManifestID),
		StorageBackend: result.StorageBackend, BrokerState: result.BrokerState,
		DeclaredBindings:    authbroker.AuthenticationRouteBrokerRequired,
		UndeclaredBindings:  authbroker.AuthenticationRouteWorkspaceOwnedCompatibility,
		Providers:           providers,
		WorkspaceActivation: result.WorkspaceActivation,
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON("auth status", authStatusDocument{SchemaVersion: 1, Auth: projection})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Authentication status output could not be encoded.", false, err)
		}
		return append(output, '\n'), nil
	}
	return renderAuthStatusText(projection, color), nil
}

func authResultCommand(task string) string {
	return map[string]string{
		authbroker.TaskLogin:  "auth login",
		authbroker.TaskImport: "auth import",
		authbroker.TaskLogout: "auth logout",
	}[task]
}

func renderAuthResultText(result authResultProjection, color bool) []byte {
	output := newHumanOutput(color)
	switch {
	case result.Change == authbroker.MutationChangeNoChange:
		output.heading("○", "Workspace Manifest credential unchanged", styleMuted)
	case result.Configured:
		output.heading("✓", "Workspace Manifest credential changed", styleSuccess)
	default:
		output.heading("✓", "Workspace Manifest credential removed", styleSuccess)
	}
	output.row("Workspace Manifest", safeExternalText(result.Context), styleText)
	output.row("Workspace Manifest state", string(result.ManifestState), humanStatusToken(string(result.ManifestState)))
	output.row("Workspace Manifest ID", optionalDisplay(result.WorkspaceManifestID, "not initialized"), styleText)
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
	revision := "none"
	if result.CredentialRevision != nil {
		revision = *result.CredentialRevision
	}
	output.row("Revision", revision, styleText)
	output.row("Change", string(result.Change), humanStatusToken(string(result.Change)))
	renderWorkspaceActivation(output, result.WorkspaceActivation)
	return output.bytes()
}

func renderAuthStatusText(result authStatusProjection, color bool) []byte {
	output := newHumanOutput(color)
	output.heading("○", "Workspace Manifest authentication status", styleText)
	output.row("Workspace Manifest", safeExternalText(result.Context), styleText)
	output.row("Workspace Manifest state", string(result.ManifestState), humanStatusToken(string(result.ManifestState)))
	output.row("Workspace Manifest ID", optionalDisplay(result.WorkspaceManifestID, "not initialized"), styleText)
	output.row("Storage", string(result.StorageBackend), styleText)
	output.row("Broker", string(result.BrokerState), humanStatusToken(string(result.BrokerState)))
	output.row("Declared bindings", string(result.DeclaredBindings), styleText)
	output.row("Undeclared bindings", string(result.UndeclaredBindings), styleText)
	renderWorkspaceActivation(output, result.WorkspaceActivation)
	if len(result.Providers) == 0 {
		output.empty("No authentication providers installed", "The Workspace Manifest provider collection is explicitly empty.", "", "")
		return output.bytes()
	}
	for _, provider := range result.Providers {
		output.section("Provider: " + safeExternalText(provider.Provider))
		output.row("State", string(provider.State), humanStatusToken(string(provider.State)))
		account := "not available"
		if provider.AccountLabel != nil {
			account = safeExternalText(*provider.AccountLabel)
		}
		output.row("Account", account, styleText)
		revision := "none"
		if provider.CredentialRevision != nil {
			revision = *provider.CredentialRevision
		}
		output.row("Revision", revision, styleText)
	}
	return output.bytes()
}

func renderWorkspaceActivation(output *humanOutput, activation authbroker.WorkspaceActivation) {
	output.row("Workspaces", string(activation.State), humanStatusToken(string(activation.State)))
	output.row("Workspace coverage", string(activation.Coverage), humanStatusToken(string(activation.Coverage)))
	if activation.Guidance != "" {
		output.row("Guidance", safeExternalText(activation.Guidance), styleText)
	}
	for _, workspace := range activation.Workspaces {
		output.section("Workspace: " + safeExternalText(workspace.ProjectID))
		output.row("Root", safeExternalText(workspace.Root), styleText)
		output.row("Workspace Manifest", safeExternalText(workspace.Context), styleText)
		output.row("Workspace Manifest ID", workspace.WorkspaceManifestID, styleText)
		output.row("Scope", string(workspace.ScopeState), humanStatusToken(string(workspace.ScopeState)))
		output.row("Activation", string(workspace.State), humanStatusToken(string(workspace.State)))
		for _, provider := range workspace.Providers {
			output.row("Projection "+safeExternalText(provider.Provider), string(provider.State), humanStatusToken(string(provider.State)))
		}
		if workspace.NextAction != nil {
			output.row("Working directory", safeExternalText(workspace.NextAction.WorkingDirectory), styleText)
			output.row("Action", safeExternalText(strings.Join(workspace.NextAction.Argv, " ")), styleText)
		}
	}
}
