package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func runContextList(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.context.List(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help context list", "Correct the command arguments.")
	}
	output, err := renderContextList(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runContextShow(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.context.Show(ctx, inputs.One("--name"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help context show", "Correct the command arguments.")
	}
	output, err := renderContextReport(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runConfigShell(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help config shell", "Correct the command arguments.")
	}
	contextName, err := selectedConfigurationContext(ctx, inputs)
	if err != nil {
		return c.fail(ctx, err)
	}
	var changes []tobari.ContextShellEnvironmentSetting
	if shellSettingInputsOmitted(inputs) {
		if format != successFormatText || invocationErrorFormat(ctx) == errorFormatJSON ||
			c.tobari == nil || !c.tobari.IsInteractive(c.In, c.Err) {
			return configurationWizardUnavailable(ctx, c, command)
		}
		current, showErr := c.context.Show(ctx, contextName)
		if showErr != nil {
			return c.fail(ctx, showErr)
		}
		// Bind Apply to the Context that was shown. The omitted active Context
		// may change in another process while the user is reviewing the wizard.
		contextName = current.Name
		wizard := c.config
		if wizard == nil {
			wizard = newContextConfigurationWizard()
		}
		changes, err = wizard.ConfigureShell(ctx, current, c.In, c.Err)
		if err != nil {
			return c.fail(ctx, normalizeConfigurationWizardError(command.Path, err))
		}
	} else {
		var value *string
		if inputs.Provided("--value") {
			literal := inputs.One("--value")
			value = &literal
		}
		changes = []tobari.ContextShellEnvironmentSetting{{
			Variable: inputs.One("--variable"),
			Source:   tobari.ContextShellEnvironmentSource(inputs.One("--source")),
			Value:    value,
		}}
	}
	intent.Target = operation.TargetRef{Kind: tobari.ContextShellTargetKind, ID: tobari.ContextShellTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.context.ConfigureShell(ctx, intent, contextName, changes)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderContextReport(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runConfigGit(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help config git", "Correct the command arguments.")
	}
	contextName, err := selectedConfigurationContext(ctx, inputs)
	if err != nil {
		return c.fail(ctx, err)
	}
	var change tobari.ContextGitIdentitySetting
	if gitSettingInputsOmitted(inputs) {
		if format != successFormatText || invocationErrorFormat(ctx) == errorFormatJSON ||
			c.tobari == nil || !c.tobari.IsInteractive(c.In, c.Err) {
			return configurationWizardUnavailable(ctx, c, command)
		}
		current, showErr := c.context.Show(ctx, contextName)
		if showErr != nil {
			return c.fail(ctx, showErr)
		}
		// Bind Apply to the Context that was shown. The omitted active Context
		// may change in another process while the user is reviewing the wizard.
		contextName = current.Name
		wizard := c.config
		if wizard == nil {
			wizard = newContextConfigurationWizard()
		}
		change, err = wizard.ConfigureGit(ctx, current, c.In, c.Err)
		if err != nil {
			return c.fail(ctx, normalizeConfigurationWizardError(command.Path, err))
		}
	} else {
		var name, email *string
		if inputs.Provided("--name") {
			value := inputs.One("--name")
			name = &value
		}
		if inputs.Provided("--email") {
			value := inputs.One("--email")
			email = &value
		}
		change = tobari.ContextGitIdentitySetting{
			Source: tobari.ContextGitIdentitySource(inputs.One("--source")),
			Name:   name, Email: email,
		}
	}
	intent.Target = operation.TargetRef{Kind: tobari.ContextGitIdentityTargetKind, ID: tobari.ContextGitIdentityTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.context.ConfigureGit(ctx, intent, contextName, change)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderContextReport(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func selectedConfigurationContext(ctx context.Context, inputs ParsedInputs) (string, error) {
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

func shellSettingInputsOmitted(inputs ParsedInputs) bool {
	return !inputs.Provided("--variable") && !inputs.Provided("--source") && !inputs.Provided("--value")
}

func gitSettingInputsOmitted(inputs ParsedInputs) bool {
	return !inputs.Provided("--source") && !inputs.Provided("--name") && !inputs.Provided("--email")
}

func configurationWizardUnavailable(ctx context.Context, c *CLI, command CommandSpec) int {
	return c.failUsage(
		ctx,
		"configuration_wizard_unavailable",
		"Omitted setting flags require text success/error output and interactive terminal stdin and stderr; usage: "+command.Usage(),
		"help "+command.Path,
		"Supply every setting flag or run the wizard with text success/error output on interactive stdin and stderr.",
	)
}

func normalizeConfigurationWizardError(path string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fault.Wrap(
		fault.KindInternal,
		"configuration_wizard_failed",
		"The configuration wizard failed before applying a change.",
		false,
		err,
		fault.NextAction{Command: "help " + path, Reason: "Retry with complete setting flags or repair the interactive terminal streams."},
	)
}

func runContextCreate(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	mode := tobari.ContextPolicyMode(inputs.One("--mode"))
	sourceAccess := tobari.ContextSourceAccess(inputs.One("--source-access"))
	result, err := c.context.Create(ctx, intent, inputs.One("--name"), inputs.One("--image"), mode, sourceAccess, inputs.One("--policy-preset"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help context create", "Correct the command arguments.")
	}
	output, err := renderContextReport(result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runContextUse(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.ContextTargetKind, ID: tobari.ActiveContextTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help context use", "Correct the command arguments.")
	}
	var progress *clusterUpProgress
	var progressSink tobari.ClusterUpProgressSink
	if format == successFormatText && c.tobari != nil && c.tobari.IsTerminal(c.Err) && clusterUpProgressAllowed(ctx) {
		progress = newClusterUpProgress(c.Err, humanStyleAllowed(ctx, c, c.Err))
		progress.Start()
		progressSink = progress.Report
		defer progress.Close()
	}
	result, err := c.context.UseWithProgress(ctx, intent, inputs.One("--name"), progressSink)
	if err != nil {
		if progress != nil {
			progress.Fail()
		}
		return c.fail(ctx, err)
	}
	output, err := renderContextReport(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runRuntimeInit(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.ContextRuntimeTargetKind, ParentID: tobari.ActiveContextRuntimeID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.context.InitRuntime(ctx, intent)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime init", "Correct the command arguments.")
	}
	output, err := renderContextReport(result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runRuntimeBuild(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.ContextRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID}
	intent.Impact = command.Agent.Mutation.Impact
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime build", "Correct the command arguments.")
	}
	buildOutput := newRuntimeBuildOutput(c.Err, humanStyleAllowed(ctx, c, c.Err))
	result, err := c.context.BuildRuntimeWithProgress(ctx, intent, buildOutput, buildOutput.Report)
	if err != nil {
		code := c.fail(ctx, err)
		if invocationErrorFormat(ctx) == errorFormatText {
			buildOutput.WriteFailureSummary()
		}
		return code
	}
	buildOutput.Flush()
	output, err := renderContextReport(result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

type contextListDocument struct {
	SchemaVersion int `json:"schema_version"`
	Contexts      struct {
		ContextState tobari.ContextObservationState `json:"context_state"`
		Active       string                         `json:"active"`
		Items        []contextSummaryJSONProjection `json:"items"`
	} `json:"contexts"`
}

type contextSummaryJSONProjection struct {
	ID                   *string                        `json:"id"`
	Name                 string                         `json:"name"`
	ContextState         tobari.ContextObservationState `json:"context_state"`
	Active               bool                           `json:"active"`
	AgentProfile         string                         `json:"agent_profile"`
	Image                string                         `json:"image"`
	PolicyMode           tobari.ContextPolicyMode       `json:"policy_mode"`
	SourceAccess         tobari.ContextSourceAccess     `json:"source_access"`
	PolicyPresetOrigin   string                         `json:"policy_preset_origin"`
	PolicyPresetRevision string                         `json:"policy_preset_revision"`
	RuntimeStatus        tobari.ContextRuntimeStatus    `json:"runtime_status,omitempty"`
}

type contextReportDocument struct {
	SchemaVersion int                         `json:"schema_version"`
	Context       contextReportJSONProjection `json:"context"`
}

type contextReportJSONProjection struct {
	Task                 string                                  `json:"task"`
	ContextState         tobari.ContextObservationState          `json:"context_state"`
	ID                   *string                                 `json:"id"`
	Name                 string                                  `json:"name"`
	Active               bool                                    `json:"active"`
	AgentProfile         string                                  `json:"agent_profile"`
	Image                string                                  `json:"image"`
	PolicyMode           tobari.ContextPolicyMode                `json:"policy_mode"`
	SourceAccess         tobari.ContextSourceAccess              `json:"source_access"`
	PolicyPresetOrigin   string                                  `json:"policy_preset_origin"`
	PolicyPresetRevision string                                  `json:"policy_preset_revision"`
	PolicyGuardrail      tobari.PolicyPresetGuardrail            `json:"policy_guardrail"`
	ShellEnvironment     []tobari.ContextShellEnvironmentSetting `json:"shell_environment"`
	GitIdentity          tobari.ContextGitIdentitySetting        `json:"git_identity"`
	Stores               *tobari.ContextStorePaths               `json:"stores"`
	Runtime              tobari.ContextRuntimeReport             `json:"runtime"`
	Cluster              tobari.ContextClusterStatus             `json:"cluster"`
	Authentication       contextAuthenticationJSONProjection     `json:"authentication"`
}

type contextAuthenticationJSONProjection struct {
	BrokerState        string                              `json:"broker_state"`
	DeclaredBindings   authbroker.AuthenticationRoute      `json:"declared_bindings"`
	UndeclaredBindings authbroker.AuthenticationRoute      `json:"undeclared_bindings"`
	Providers          []contextAuthProviderJSONProjection `json:"providers"`
}

type contextAuthProviderJSONProjection struct {
	Provider           string  `json:"provider"`
	State              string  `json:"state"`
	AccountLabel       *string `json:"account_label"`
	CredentialRevision *string `json:"credential_revision"`
}

func contextReportJSONDocument(result tobari.ContextReport) contextReportDocument {
	providers := make([]contextAuthProviderJSONProjection, 0, len(result.Authentication.Providers))
	if result.Authentication.Providers == nil {
		providers = nil
	} else {
		for _, provider := range result.Authentication.Providers {
			providers = append(providers, contextAuthProviderJSONProjection{
				Provider: provider.Provider, State: provider.State, AccountLabel: provider.AccountLabel,
				CredentialRevision: optionalString(provider.CredentialRevision),
			})
		}
	}
	return contextReportDocument{
		SchemaVersion: 1,
		Context: contextReportJSONProjection{
			Task: result.Task, ContextState: result.ContextState, ID: optionalString(result.ID), Name: result.Name, Active: result.Active,
			AgentProfile: result.AgentProfile, Image: result.Image, PolicyMode: result.PolicyMode,
			SourceAccess:       result.SourceAccess,
			PolicyPresetOrigin: result.PolicyPresetOrigin, PolicyPresetRevision: result.PolicyPresetRevision, PolicyGuardrail: result.PolicyGuardrail,
			ShellEnvironment: result.ShellEnvironment, GitIdentity: result.GitIdentity, Stores: optionalContextStores(result),
			Runtime: result.Runtime, Cluster: result.Cluster,
			Authentication: contextAuthenticationJSONProjection{
				BrokerState:        result.Authentication.BrokerState,
				DeclaredBindings:   authbroker.AuthenticationRouteBrokerRequired,
				UndeclaredBindings: authbroker.AuthenticationRouteWorkspaceOwnedCompatibility,
				Providers:          providers,
			},
		},
	}
}

func optionalContextStores(result tobari.ContextReport) *tobari.ContextStorePaths {
	if result.ContextState == tobari.ContextObservationSyntheticDefault {
		return nil
	}
	stores := result.Stores
	return &stores
}

func contextReportCommand(task string) string {
	return map[string]string{
		tobari.TaskContextShow: "context show", tobari.TaskContextCreate: "context create",
		tobari.TaskContextUse: "context use", tobari.TaskConfigShell: "config shell",
		tobari.TaskConfigGit: "config git", tobari.TaskRuntimeInit: "runtime init",
		tobari.TaskRuntimeBuild: "runtime build",
	}[task]
}

func renderContextList(result tobari.ContextListResult, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_context_list", "Context list is invalid", false, err)
	}
	if format == successFormatJSON {
		document := contextListDocument{SchemaVersion: 1}
		document.Contexts.ContextState = result.ContextState
		document.Contexts.Active = result.Active
		document.Contexts.Items = make([]contextSummaryJSONProjection, 0, len(result.Items))
		for _, item := range result.Items {
			document.Contexts.Items = append(document.Contexts.Items, contextSummaryJSONProjection{
				ID: optionalString(item.ID), Name: item.Name, ContextState: item.ContextState, Active: item.Active,
				AgentProfile: item.AgentProfile, Image: item.Image, PolicyMode: item.PolicyMode,
				SourceAccess: item.SourceAccess, RuntimeStatus: item.RuntimeStatus,
				PolicyPresetOrigin: item.PolicyPresetOrigin, PolicyPresetRevision: item.PolicyPresetRevision,
			})
		}
		output, err := marshalCommandJSON("context list", document)
		if err != nil {
			return nil, err
		}
		return append(output, '\n'), nil
	}
	var output strings.Builder
	writeStyledLine(&output, color, "Current Context:", safeExternalText(result.Active), styleText)
	writeStyledLine(&output, color, "Context state:", string(result.ContextState), humanStatusToken(string(result.ContextState)))
	output.WriteString("\n")
	output.WriteString(applyStyleToken(color, styleAccent, "Contexts:"))
	output.WriteString("\n")
	for _, item := range result.Items {
		marker := " "
		markerToken := styleMuted
		if item.Active {
			marker = "*"
			markerToken = styleAccent
		}
		fmt.Fprintf(
			&output, "%s %s\t%s=%s\t%s=%s\t%s=%s\t%s=%s\t%s=%s\n",
			applyStyleToken(color, markerToken, marker),
			applyStyleToken(color, styleText, safeExternalText(item.Name)),
			applyStyleToken(color, styleMuted, "mode"), applyStyleToken(color, styleText, string(item.PolicyMode)),
			applyStyleToken(color, styleMuted, "source"), applyStyleToken(color, styleText, "direct "+string(item.SourceAccess)),
			applyStyleToken(color, styleMuted, "image"), applyStyleToken(color, styleText, safeExternalText(item.Image)),
			applyStyleToken(color, styleMuted, "runtime"), applyStyleToken(color, humanStatusToken(string(item.RuntimeStatus)), string(item.RuntimeStatus)),
			applyStyleToken(color, styleMuted, "agent"), applyStyleToken(color, styleText, safeExternalText(item.AgentProfile)),
		)
	}
	return []byte(output.String()), nil
}

func renderContextReport(result tobari.ContextReport, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_context_report", "Context report is invalid", false, err)
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON(contextReportCommand(result.Task), contextReportJSONDocument(result))
		if err != nil {
			return nil, err
		}
		return append(output, '\n'), nil
	}
	return renderContextReportText(result, color), nil
}

func renderContextReportText(result tobari.ContextReport, color bool) []byte {
	if result.Task == tobari.TaskRuntimeInit {
		return renderRuntimeInitReportText(result, color)
	}

	var output strings.Builder
	writeStyledLine(&output, color, "Context:", safeExternalText(result.Name), styleText)
	writeStyledLine(&output, color, "Context state:", string(result.ContextState), humanStatusToken(string(result.ContextState)))
	writeStyledLine(&output, color, "Active:", fmt.Sprintf("%t", result.Active), styleText)
	writeStyledLine(&output, color, "Image:", safeExternalText(result.Image), styleText)
	writeStyledLine(&output, color, "Agent profile:", safeExternalText(result.AgentProfile), styleText)
	writeStyledLine(&output, color, "Policy mode:", string(result.PolicyMode), styleText)
	writeStyledLine(&output, color, "Source access:", "direct "+string(result.SourceAccess), styleText)
	writeStyledLine(&output, color, "Policy preset:", safeExternalText(result.PolicyPresetOrigin), styleText)
	if result.PolicyPresetRevision != "" {
		writeStyledLine(&output, color, "Policy preset revision:", result.PolicyPresetRevision, styleText)
	}
	writeStyledLine(&output, color, "Policy guardrail:", string(result.PolicyGuardrail), styleText)
	for _, setting := range result.ShellEnvironment {
		value := string(setting.Source)
		if setting.Source == tobari.ContextShellEnvironmentLiteral && setting.Value != nil {
			value += " " + fmt.Sprintf("%q", safeExternalText(*setting.Value))
		}
		writeStyledLine(&output, color, "Shell "+setting.Variable+":", value, styleText)
	}
	writeStyledLine(&output, color, "Git identity:", string(result.GitIdentity.Source), styleText)
	if result.GitIdentity.Source == tobari.ContextGitIdentityLiteral && result.GitIdentity.Name != nil && result.GitIdentity.Email != nil {
		writeStyledLine(&output, color, "Git user.name:", safeExternalText(*result.GitIdentity.Name), styleText)
		writeStyledLine(&output, color, "Git user.email:", safeExternalText(*result.GitIdentity.Email), styleText)
	}
	if result.Task == tobari.TaskContextShow {
		writeStyledLine(&output, color, "Auth Broker:", result.Authentication.BrokerState, humanStatusToken(result.Authentication.BrokerState))
		writeStyledLine(&output, color, "Declared bindings:", string(authbroker.AuthenticationRouteBrokerRequired), styleText)
		writeStyledLine(&output, color, "Undeclared bindings:", string(authbroker.AuthenticationRouteWorkspaceOwnedCompatibility), styleText)
		writeStyledLine(&output, color, "Authentication scope:", "Declared bindings require a project handle; each permanently bound project receives a distinct handle on its next Workspace entry.", styleText)
		for _, provider := range result.Authentication.Providers {
			value := provider.State
			if provider.AccountLabel != nil {
				value += " (account " + safeExternalText(*provider.AccountLabel) + ")"
			}
			writeStyledLine(&output, color, "Auth provider "+safeExternalText(provider.Provider)+":", value, humanStatusToken(provider.State))
		}
		writeStyledLine(
			&output, color, "Next:",
			"run `"+safeExternalText(strings.Join(contextAuthStatusNextArgv(result), " "))+"` for activation guidance.",
			styleText,
		)
	}
	if result.Task == tobari.TaskContextCreate || result.Task == tobari.TaskContextUse {
		writeStyledLine(&output, color, "Cluster:", string(result.Cluster), humanStatusToken(string(result.Cluster)))
	}
	if result.Runtime.Kind != "" {
		writeStyledLine(
			&output, color, "Runtime:",
			string(result.Runtime.Kind)+" ("+string(result.Runtime.Status)+")",
			humanStatusToken(string(result.Runtime.Status)),
		)
		if result.Runtime.Dockerfile != "" {
			writeStyledLine(&output, color, "Runtime Dockerfile:", safeExternalText(result.Runtime.Dockerfile), styleText)
		}
		if result.Runtime.BaseReference != "" {
			writeStyledLine(&output, color, "Runtime base:", safeExternalText(result.Runtime.BaseReference), styleText)
		}
		if result.Runtime.SourceDigest != "" {
			writeStyledLine(&output, color, "Runtime source digest:", safeExternalText(result.Runtime.SourceDigest), styleText)
		}
		if result.Runtime.ImageDigest != "" {
			writeStyledLine(&output, color, "Runtime image digest:", safeExternalText(result.Runtime.ImageDigest), styleText)
		}
	}
	if result.Runtime.Status == tobari.ContextRuntimeStatusOfficial {
		writeStyledLine(
			&output, color, "Tip:",
			strings.TrimPrefix(runtimeCustomizationHint(), "Tip: "),
			styleText,
		)
	}
	switch result.Task {
	case tobari.TaskContextCreate:
		if nextArgv := contextCreateNextArgv(result); len(nextArgv) > 0 {
			writeStyledCommandLine(
				&output, color, "Next:", "run ",
				"`"+safeExternalText(strings.Join(nextArgv, " "))+"`",
				" to load every Context into the shared cluster.",
			)
		}
	case tobari.TaskConfigShell:
		writeStyledCommandLine(&output, color, "Next:", "start a new session with ", "`tobari`", "; running sessions are unchanged.")
	case tobari.TaskConfigGit:
		writeStyledCommandLine(&output, color, "Next:", "re-enter a matching Workspace with ", "`tobari`", " to reconcile its Git fallback; this command does not change running sessions.")
	case tobari.TaskRuntimeBuild:
		writeStyledLine(&output, color, "Note:", "existing Workspaces keep their home. On the next `tobari`, Tobari recreates only the work container when this runtime image changes the spec.", styleText)
		writeStyledCommandLine(&output, color, "Next:", "run ", "`tobari`", " from a project directory.")
	case tobari.TaskContextUse:
		switch result.Cluster {
		case tobari.ContextClusterStatusDefaultUpdated:
			writeStyledCommandLine(
				&output, color, "Next:", "run ", "`tobari`",
				" from a project directory to create or enter a Workspace using the new default Context.",
			)
		case tobari.ContextClusterStatusReconciled, tobari.ContextClusterStatusAlreadyReady:
			writeStyledCommandLine(&output, color, "Next:", "run ", "`tobari`", " from a project directory.")
		case tobari.ContextClusterStatusNotConfigured, tobari.ContextClusterStatusNotRunning:
			writeStyledCommandLine(&output, color, "Next:", "run ", "`tobari cluster up`, then `tobari`", " from a project directory.")
		}
	}
	if result.ContextState != tobari.ContextObservationSyntheticDefault {
		writeStyledLine(&output, color, "Policy:", safeExternalText(result.Stores.PolicyDirectory), styleText)
	}
	return []byte(output.String())
}

func contextAuthStatusNextArgv(result tobari.ContextReport) []string {
	argv := []string{ProgramName, "auth", "status"}
	if result.ContextState != tobari.ContextObservationSyntheticDefault {
		argv = append(argv, "--context", result.Name)
	}
	return argv
}

func contextCreateNextArgv(result tobari.ContextReport) []string {
	if result.Task != tobari.TaskContextCreate {
		return nil
	}
	switch result.Cluster {
	case tobari.ContextClusterStatusNotApplicable, tobari.ContextClusterStatusRequiresReconcile:
		return []string{ProgramName, "cluster", "up"}
	default:
		return nil
	}
}

func renderRuntimeInitReportText(result tobari.ContextReport, color bool) []byte {
	output := newHumanOutput(color)
	output.heading("✓", "Runtime Dockerfile created", styleSuccess)
	output.section("Next")
	output.nextStep(1, "Edit the Dockerfile", safeExternalText(result.Runtime.Dockerfile), styleText)
	output.nextStep(2, "Build the runtime", recoveryCommand("runtime build"), styleAccent)
	output.section("Details")
	output.row("Context", safeExternalText(result.Name), styleText)
	output.row("Base image", safeExternalText(result.Runtime.BaseReference), styleText)
	output.row("Status", string(result.Runtime.Status), humanStatusToken(string(result.Runtime.Status)))
	return output.bytes()
}

func writeStyledLine(output *strings.Builder, enabled bool, label, value string, token styleToken) {
	fmt.Fprintf(
		output, "%s %s\n",
		applyStyleToken(enabled, styleMuted, label),
		applyStyleToken(enabled, token, value),
	)
}

func runtimeCustomizationHint() string {
	return "Tip: this Context is using the base runtime. For ongoing work, run `tobari runtime init`, edit the Dockerfile, then run `tobari runtime build` on the host."
}
