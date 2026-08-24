//go:build tobari_dev && tobari_research

package cli

import (
	"context"

	"github.com/tasuku43/tobari/internal/app/authcmd"
	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalAuthResultDocument struct {
	SchemaVersion int                         `json:"schema_version"`
	Auth          finalAuthMutationProjection `json:"auth"`
}

type finalAuthMutationProjection struct {
	Task               string                    `json:"task"`
	Provider           string                    `json:"provider"`
	Configured         bool                      `json:"configured"`
	AccountLabel       *string                   `json:"account_label"`
	StorageBackend     authbroker.StorageBackend `json:"storage_backend"`
	BrokerState        authbroker.BrokerState    `json:"broker_state"`
	CredentialRevision string                    `json:"credential_revision"`
	Change             authbroker.MutationChange `json:"change"`
}

type finalAuthStatusDocument struct {
	SchemaVersion int                            `json:"schema_version"`
	Auth          authbroker.ContextStatusResult `json:"auth"`
}

func runAuthLogin(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.finalAuth == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help auth login", "Correct the command arguments.")
	}
	contextRef := inputs.One("--context")
	provider := inputs.One("--provider")
	if !inputs.Provided("--provider") {
		provider, err = c.selectFinalAuthLoginProvider(ctx, contextRef)
		if err != nil {
			return c.fail(ctx, err)
		}
	}
	bindFinalAuthMutationIntent(&intent, command, contextRef)
	result, err := c.finalAuth.Login(ctx, intent, contextRef, provider, inputs.One("--method"), c.In, c.Err)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderFinalAuthResult(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func (c *CLI) selectFinalAuthLoginProvider(ctx context.Context, contextRef string) (string, error) {
	if invocationErrorFormat(ctx) == errorFormatJSON {
		return "", fault.New(fault.KindInvalidInput, "auth_login_selector_unavailable", "Omitted provider selection requires human text error output on an interactive terminal.", false, fault.NextAction{Command: "help auth login", Reason: "Pass an explicit reviewed provider or use the interactive selector."})
	}
	status, err := c.finalAuth.Status(ctx, contextRef)
	if err != nil {
		return "", err
	}
	providers := make([]authbroker.ProviderStatus, 0, len(status.Providers))
	for _, provider := range status.Providers {
		if authcmd.SupportsLoginProvider(provider.Provider) {
			providers = append(providers, provider)
		}
	}
	if len(providers) == 0 {
		return "", fault.New(fault.KindUnsupported, "provider_login_unsupported", "No reviewed provider supports interactive login for this Context.", false, fault.NextAction{Command: "auth status", Reason: "Inspect providers available for the exact Context."})
	}
	if c.authLogin == nil {
		return "", fault.New(fault.KindContract, "invalid_auth_result", "Authentication provider selection is not configured.", false, fault.NextAction{Command: "auth status", Reason: "Inspect the exact Context provider inventory."})
	}
	selected, err := c.authLogin.Select(ctx, contextRef, providers, c.In, c.Err)
	if err != nil {
		return "", err
	}
	for _, provider := range providers {
		if provider.Provider == selected {
			return selected, nil
		}
	}
	return "", fault.New(fault.KindContract, "invalid_auth_result", "Authentication provider selection did not match the observed Context inventory.", false, fault.NextAction{Command: "auth status", Reason: "Inspect the exact Context provider inventory."})
}

func runAuthImport(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.finalAuth == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help auth import", "Correct the command arguments.")
	}
	contextRef := inputs.One("--context")
	bindFinalAuthMutationIntent(&intent, command, contextRef)
	result, err := c.finalAuth.Import(ctx, intent, contextRef, inputs.One("provider"), c.In)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderFinalAuthResult(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runAuthStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.finalAuth == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help auth status", "Correct the command arguments.")
	}
	result, err := c.finalAuth.Status(ctx, inputs.One("--context"))
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderFinalAuthStatus(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runAuthLogout(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.finalAuth == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help auth logout", "Correct the command arguments.")
	}
	contextRef := inputs.One("--context")
	bindFinalAuthMutationIntent(&intent, command, contextRef)
	result, err := c.finalAuth.Logout(ctx, intent, contextRef, inputs.One("provider"))
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderFinalAuthResult(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func bindFinalAuthMutationIntent(intent *operation.Intent, command CommandSpec, contextRef string) {
	intent.Impact = command.Agent.Mutation.Impact
	if command.Effect == operation.EffectWrite {
		intent.Target = operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: contextRef}
		return
	}
	intent.Target = operation.TargetRef{Kind: authbroker.ContextCredentialTargetKind, ParentID: contextRef}
}

func renderFinalAuthResult(result authbroker.ContextResult, format successFormat, color bool) ([]byte, error) {
	if err := result.ValidateFor(result.Task, result.ContextRef, result.Provider); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_auth_result", "Final Context authentication result is invalid.", false, err, fault.NextAction{Command: "auth status", Reason: "Inspect the exact Context credential inventory."})
	}
	if format == successFormatJSON {
		projection := finalAuthMutationProjection{Task: result.Task, Provider: result.Provider, Configured: result.Configured, AccountLabel: result.AccountLabel, StorageBackend: result.StorageBackend, BrokerState: result.BrokerState, CredentialRevision: result.CredentialRevision, Change: result.Change}
		output, err := marshalCommandJSON(authResultCommand(result.Task), finalAuthResultDocument{SchemaVersion: 2, Auth: projection})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Authentication output could not be encoded.", false, err)
		}
		return append(output, '\n'), nil
	}
	view := newHumanOutput(color)
	heading := "Final Context credential changed"
	if result.Change == authbroker.MutationChangeNoChange {
		heading = "Final Context credential unchanged"
	} else if !result.Configured {
		heading = "Final Context credential removed"
	}
	view.heading("✓", heading, styleSuccess)
	view.row("Provider", safeExternalText(result.Provider), styleText)
	view.row("Configured", humanBool(result.Configured), humanOutcomeBoolToken(result.Configured))
	view.row("Account", optionalDisplay(result.AccountLabel, "not available"), styleText)
	view.row("Storage", string(result.StorageBackend), styleText)
	view.row("Broker", string(result.BrokerState), humanStatusToken(string(result.BrokerState)))
	revision := result.CredentialRevision
	if revision == "" {
		revision = "none"
	}
	view.row("Revision", safeExternalText(revision), styleText)
	view.row("Change", string(result.Change), humanStatusToken(string(result.Change)))
	return view.bytes(), nil
}

func renderFinalAuthStatus(result authbroker.ContextStatusResult, format successFormat, color bool) ([]byte, error) {
	if err := result.ValidateFor(result.ContextRef); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_auth_result", "Final Context authentication status is invalid.", false, err, fault.NextAction{Command: "auth status", Reason: "Inspect the exact Context credential inventory."})
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON("auth status", finalAuthStatusDocument{SchemaVersion: 2, Auth: result})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Authentication status output could not be encoded.", false, err)
		}
		return append(output, '\n'), nil
	}
	view := newHumanOutput(color)
	view.heading("○", "Final Context authentication status", styleText)
	view.row("Context", safeExternalText(result.ContextRef), styleText)
	view.row("Storage", string(result.StorageBackend), styleText)
	view.row("Broker", string(result.BrokerState), humanStatusToken(string(result.BrokerState)))
	if len(result.Providers) == 0 {
		view.empty("No Context credentials", "The exhaustive Context credential inventory is empty.", "", "")
		return view.bytes(), nil
	}
	for _, provider := range result.Providers {
		view.section("Provider: " + safeExternalText(provider.Provider))
		view.row("State", string(provider.State), humanStatusToken(string(provider.State)))
		view.row("Account", optionalDisplay(provider.AccountLabel, "not available"), styleText)
		revision := provider.CredentialRevision
		if revision == "" {
			revision = "none"
		}
		view.row("Revision", safeExternalText(revision), styleText)
	}
	return view.bytes(), nil
}

func authResultCommand(task string) string {
	return map[string]string{authbroker.TaskLogin: "auth login", authbroker.TaskImport: "auth import", authbroker.TaskLogout: "auth logout"}[task]
}

func optionalDisplay(value *string, absent string) string {
	if value == nil {
		return absent
	}
	return safeExternalText(*value)
}
