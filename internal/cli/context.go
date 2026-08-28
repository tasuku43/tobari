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
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help manifest list", "Correct the command arguments.")
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
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help manifest show", "Correct the command arguments.")
	}
	details, ok := inputs.Boolean("--details")
	if !ok {
		return c.failUsage(ctx, "invalid_arguments", "--details must be a boolean; usage: "+command.Usage(), "help manifest show", "Correct the command arguments.")
	}
	output, err := renderContextShowReport(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out), details)
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
	var changes []tobari.ManifestShellEnvironmentSetting
	if shellSettingInputsOmitted(inputs) {
		if format != successFormatText || invocationErrorFormat(ctx) == errorFormatJSON ||
			c.tobari == nil || !c.tobari.IsInteractive(c.In, c.Err) {
			return configurationWizardUnavailable(ctx, c, command)
		}
		current, showErr := c.context.Show(ctx, contextName)
		if showErr != nil {
			return c.fail(ctx, showErr)
		}
		// Bind Apply to the Workspace Manifest that was shown. The omitted default
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
		changes = []tobari.ManifestShellEnvironmentSetting{{
			Variable: inputs.One("--variable"),
			Source:   tobari.ManifestShellEnvironmentSource(inputs.One("--source")),
			Value:    value,
		}}
	}
	intent.Target = operation.TargetRef{Kind: tobari.ManifestShellTargetKind, ID: tobari.ManifestShellTargetID}
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
	var change tobari.ManifestGitIdentitySetting
	if gitSettingInputsOmitted(inputs) {
		if format != successFormatText || invocationErrorFormat(ctx) == errorFormatJSON ||
			c.tobari == nil || !c.tobari.IsInteractive(c.In, c.Err) {
			return configurationWizardUnavailable(ctx, c, command)
		}
		current, showErr := c.context.Show(ctx, contextName)
		if showErr != nil {
			return c.fail(ctx, showErr)
		}
		// Bind Apply to the Workspace Manifest that was shown. The omitted default
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
		change = tobari.ManifestGitIdentitySetting{
			Source: tobari.ManifestGitIdentitySource(inputs.One("--source")),
			Name:   name, Email: email,
		}
	}
	intent.Target = operation.TargetRef{Kind: tobari.ManifestGitIdentityTargetKind, ID: tobari.ManifestGitIdentityTargetID}
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
	_ = ctx
	_ = inputs
	return "", fault.New(fault.KindContract, "legacy_context_unavailable", "Legacy Context selection is unavailable.", false)
}

func runConfigBootstrapAWS(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "template show", "Correct the command arguments.")
	}
	contextName, err := selectedConfigurationContext(ctx, inputs)
	if err != nil {
		return c.fail(ctx, err)
	}
	profile := inputs.One("--profile")
	remove := inputs.Provided("--remove")
	expectedRevision := ""
	if !inputs.Provided("--profile") && !inputs.Provided("--refresh") && !remove {
		if format != successFormatText || invocationErrorFormat(ctx) == errorFormatJSON || c.tobari == nil || !c.tobari.IsInteractive(c.In, c.Err) {
			return configurationWizardUnavailable(ctx, c, command)
		}
		current, showErr := c.context.Show(ctx, contextName)
		if showErr != nil {
			return c.fail(ctx, showErr)
		}
		contextName = current.Name
		chooser := newContextConfigurationWizardWithStyle(!c.noColor)
		options := []configurationWizardOption{{label: "Configure profile", description: "Normalize a host IAM Identity Center profile.", value: "configure"}}
		if current.Bootstrap.Resolved().State == tobari.ManifestBootstrapConfigured {
			options = append(options, configurationWizardOption{label: "Refresh snapshot", description: "Re-read the currently selected host profile.", value: "refresh"}, configurationWizardOption{label: "Remove recipe", description: "Keep existing Workspaces unchanged.", value: "remove"})
		}
		index, chooseErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{title: "Tobari · AWS bootstrap", contextName: current.Name, current: current.Bootstrap.Resolved().State, information: []string{"Only future Workspaces receive the snapshot once.", "Credentials and SSO caches are never read."}, prompt: "Action", options: options})
		if chooseErr != nil {
			return c.fail(ctx, normalizeConfigurationWizardError(command.Path, chooseErr))
		}
		action := options[index].value
		remove = action == "remove"
		if action == "configure" {
			profile, err = readConfigurationWizardValue(ctx, c.In, c.Err, "AWS profile", 64)
			if err != nil {
				return c.fail(ctx, normalizeConfigurationWizardError(command.Path, err))
			}
			profile = strings.TrimSpace(profile)
		}
		information := []string{"Existing Workspace homes remain unchanged."}
		if !remove {
			preview, previewErr := c.context.PreviewAWSBootstrap(ctx, contextName, profile)
			if previewErr != nil {
				return c.fail(ctx, previewErr)
			}
			expectedRevision = preview.Candidate.Revision
			changes := "none"
			if len(preview.Changes) > 0 {
				changes = strings.Join(preview.Changes, ", ")
			}
			currentRevision := preview.Current.Resolved().Revision
			if currentRevision == "" {
				currentRevision = "not configured"
			}
			information = append(information, "Current revision: "+currentRevision, "Candidate revision: "+preview.Candidate.Revision, "Semantic changes: "+changes)
		} else {
			information = append(information, "The future-Workspace recipe will be removed.")
		}
		confirm, confirmErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{title: "Tobari · Review AWS bootstrap", contextName: contextName, current: current.Bootstrap.Resolved().State, information: information, prompt: "Commit", options: []configurationWizardOption{{label: "Apply", description: "Commit this Workspace Manifest recipe change.", value: "apply"}, {label: "Cancel", description: "Leave the Workspace Manifest unchanged.", value: "cancel"}}})
		if confirmErr != nil {
			return c.fail(ctx, normalizeConfigurationWizardError(command.Path, confirmErr))
		}
		if confirm != 0 {
			return c.fail(ctx, context.Canceled)
		}
	}
	intent.Target = operation.TargetRef{Kind: tobari.ManifestBootstrapTargetKind, ID: tobari.ManifestBootstrapTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.context.ConfigureAWSBootstrap(ctx, intent, contextName, profile, expectedRevision, remove)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderContextReport(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runConfigBootstrapEKS(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help config bootstrap kubernetes eks", "Correct the command arguments.")
	}
	contextName, err := selectedConfigurationContext(ctx, inputs)
	if err != nil {
		return c.fail(ctx, err)
	}
	kubeContext := inputs.One("--kube-context")
	remove := inputs.Provided("--remove")
	expectedRevision := ""
	if !inputs.Provided("--kube-context") && !inputs.Provided("--refresh") && !remove {
		if format != successFormatText || invocationErrorFormat(ctx) == errorFormatJSON || c.tobari == nil || !c.tobari.IsInteractive(c.In, c.Err) {
			return configurationWizardUnavailable(ctx, c, command)
		}
		current, showErr := c.context.Show(ctx, contextName)
		if showErr != nil {
			return c.fail(ctx, showErr)
		}
		contextName = current.Name
		chooser := newContextConfigurationWizardWithStyle(!c.noColor)
		options := []configurationWizardOption{{label: "Configure EKS context", description: "Normalize one AWS CLI-generated host kube context.", value: "configure"}}
		if current.Bootstrap.EKSContext != "" {
			options = append(options, configurationWizardOption{label: "Refresh EKS target", description: "Re-read the selected host kube context.", value: "refresh"}, configurationWizardOption{label: "Remove EKS target", description: "Preserve AWS and existing Workspaces.", value: "remove"})
		}
		index, chooseErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{title: "Tobari · EKS bootstrap", contextName: current.Name, current: current.Bootstrap.Resolved().State, information: []string{"Only future Workspaces receive the target once.", "Tokens, keys, arbitrary exec plugins, and network authority are never imported."}, prompt: "Action", options: options})
		if chooseErr != nil {
			return c.fail(ctx, normalizeConfigurationWizardError(command.Path, chooseErr))
		}
		action := options[index].value
		remove = action == "remove"
		if action == "configure" {
			kubeContext, err = readConfigurationWizardValue(ctx, c.In, c.Err, "Kubernetes context", 253)
			if err != nil {
				return c.fail(ctx, normalizeConfigurationWizardError(command.Path, err))
			}
			kubeContext = strings.TrimSpace(kubeContext)
		}
		information := []string{"Existing Workspace homes remain unchanged."}
		if !remove {
			preview, previewErr := c.context.PreviewEKSBootstrap(ctx, contextName, kubeContext)
			if previewErr != nil {
				return c.fail(ctx, previewErr)
			}
			expectedRevision = preview.Candidate.Revision
			changes := "none"
			if len(preview.Changes) > 0 {
				changes = strings.Join(preview.Changes, ", ")
			}
			information = append(information, "Candidate revision: "+preview.Candidate.Revision, "Semantic changes: "+changes)
		} else {
			information = append(information, "The EKS target will be removed; AWS remains configured.")
		}
		confirm, confirmErr := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{title: "Tobari · Review EKS bootstrap", contextName: contextName, current: current.Bootstrap.Resolved().State, information: information, prompt: "Commit", options: []configurationWizardOption{{label: "Apply", description: "Commit this Workspace Manifest recipe change.", value: "apply"}, {label: "Cancel", description: "Leave the Workspace Manifest unchanged.", value: "cancel"}}})
		if confirmErr != nil {
			return c.fail(ctx, normalizeConfigurationWizardError(command.Path, confirmErr))
		}
		if confirm != 0 {
			return c.fail(ctx, context.Canceled)
		}
	}
	intent.Target = operation.TargetRef{Kind: tobari.ManifestBootstrapTargetKind, ID: tobari.ManifestBootstrapTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.context.ConfigureEKSBootstrap(ctx, intent, contextName, kubeContext, expectedRevision, remove)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderContextReport(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
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
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help manifest create", "Correct the command arguments.")
	}
	result, err := createContext(ctx, c, command, intent, inputs, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderContextReport(result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func createContext(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent,
	inputs ParsedInputs, format successFormat,
) (tobari.ManifestReport, error) {
	intent.Target = operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ParentID: tobari.ManifestCatalogTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	sourceAccess := tobari.ManifestSourceAccess(inputs.One("--source-access"))
	name := inputs.One("--name")
	var base *tobari.ManifestCopySnapshot
	if inputs.Provided("--copy-from") {
		observed, baseErr := c.context.CopySnapshot(ctx, inputs.One("--copy-from"))
		if baseErr != nil {
			return tobari.ManifestReport{}, baseErr
		}
		base = &observed
	}
	if !contextCreateDirectInputsComplete(inputs) {
		if format != successFormatText || invocationErrorFormat(ctx) == errorFormatJSON ||
			c.tobari == nil || !c.tobari.IsInteractive(c.In, c.Err) {
			return tobari.ManifestReport{}, fault.New(
				fault.KindInvalidInput, "manifest_create_wizard_unavailable",
				"Incomplete Workspace Manifest creation requires text success/error output and interactive terminal stdin and stderr; otherwise supply --copy-from with --name, or explicitly supply --name, --runtime, --source-access, and --native-readiness; usage: "+command.Usage(), false,
				fault.NextAction{Command: "help manifest create", Reason: "Complete omitted settings interactively or supply the complete direct input group."},
			)
		}
		var availableBases []tobari.ManifestSummary
		if base == nil && !inputs.Provided("--copy-from") {
			selectedBase, summaries, selectErr := selectContextCreateBase(ctx, c)
			if selectErr != nil {
				return tobari.ManifestReport{}, selectErr
			}
			base = selectedBase
			availableBases = summaries
		}
		if base != nil && len(availableBases) == 0 {
			listed, listErr := c.context.List(ctx)
			if listErr != nil {
				return tobari.ManifestReport{}, listErr
			}
			availableBases = listed.Items
		}
		seed, seedErr := contextCreateSeedFromInputs(ctx, c, inputs, base)
		if seedErr != nil {
			return tobari.ManifestReport{}, seedErr
		}
		wizard := c.contextCreate
		if wizard == nil {
			wizard = newContextCreateWizardWithStyle(!c.noColor)
		}
		if terminalWizard, ok := wizard.(*terminalContextCreateWizard); ok && terminalWizard.bootstrap == nil {
			terminalWizard.bootstrap = c.context
		}
		if terminalWizard, ok := wizard.(*terminalContextCreateWizard); ok {
			terminalWizard.bases = availableBases
			terminalWizard.baseRead = c.context
		}
		if terminalWizard, ok := wizard.(*terminalContextCreateWizard); ok && c.runtime != nil && !seed.RuntimeProvided {
			if catalog, listErr := c.runtime.List(ctx); listErr == nil {
				terminalWizard.runtimes = catalog.Items
			}
		}
		var selection contextCreateSelection
		var wizardErr error
		if contextCreateCompositionInputProvided(inputs) || seed.Selection.CopyFrom != nil {
			seeded, ok := wizard.(seededContextCreateWizard)
			if !ok {
				return tobari.ManifestReport{}, fault.New(
					fault.KindInternal, "manifest_create_wizard_failed",
					"The Workspace Manifest creation wizard cannot preserve supplied partial inputs.", false,
					fault.NextAction{Command: "manifest create", Reason: "Retry with the built-in terminal wizard or the complete direct input group."},
				)
			}
			selection, wizardErr = seeded.ComposeSeeded(ctx, c.In, c.Err, seed)
		} else {
			selection, wizardErr = wizard.Compose(ctx, c.In, c.Err)
		}
		if wizardErr != nil {
			return tobari.ManifestReport{}, normalizeContextCreateWizardError(wizardErr)
		}
		policy := selection.MethodPolicy.Clone()
		var bootstrap *tobari.ManifestBootstrapSnapshot
		if selection.Bootstrap != nil {
			prepared := selection.Bootstrap.Clone()
			bootstrap = &prepared
		} else if selection.AWSBootstrapProfile != "" {
			prepared, prepareErr := c.context.PrepareAWSBootstrap(ctx, selection.AWSBootstrapProfile)
			if prepareErr != nil {
				return tobari.ManifestReport{}, prepareErr
			}
			if selection.EKSBootstrapContext != "" {
				prepared, prepareErr = c.context.PrepareEKSBootstrap(ctx, prepared, selection.EKSBootstrapContext)
				if prepareErr != nil {
					return tobari.ManifestReport{}, prepareErr
				}
			}
			bootstrap = &prepared
		}
		return c.context.CreateWithComposition(
			ctx, intent, selection.Name, tobari.BuiltinImageSelector, selection.SourceAccess,
			tobari.ManifestCreateComposition{
				NativeReadiness:  selection.NativeReadiness,
				MethodPolicy:     &policy,
				Bootstrap:        bootstrap,
				RuntimeSelection: selection.RuntimeSelection,
				CopyFrom:         selection.CopyFrom,
			},
		)
	}
	if name == "" {
		return tobari.ManifestReport{}, fault.New(
			fault.KindInvalidInput, "invalid_arguments",
			"--name is required in direct mode; usage: "+command.Usage(), false,
			fault.NextAction{Command: "help manifest create", Reason: "Supply --name or run the command without arguments to open the wizard."},
		)
	}
	if base != nil {
		sourceAccess = base.SourceAccess
		if inputs.Provided("--source-access") {
			sourceAccess = tobari.ManifestSourceAccess(inputs.One("--source-access"))
		}
		readiness := base.NativeReadiness
		if inputs.Provided("--native-readiness") {
			readiness = tobari.ManifestNativeReadiness(inputs.One("--native-readiness"))
		}
		runtimeSelection := base.RuntimeSelection
		if inputs.Provided("--runtime") {
			runtimeSelection = inputs.One("--runtime")
		}
		policy := base.MethodPolicy.Clone()
		bootstrap := base.Bootstrap
		if inputs.Provided("--bootstrap-aws-profile") {
			prepared, prepareErr := c.context.PrepareAWSBootstrap(ctx, inputs.One("--bootstrap-aws-profile"))
			if prepareErr != nil {
				return tobari.ManifestReport{}, prepareErr
			}
			if inputs.Provided("--bootstrap-eks-context") {
				prepared, prepareErr = c.context.PrepareEKSBootstrap(ctx, prepared, inputs.One("--bootstrap-eks-context"))
				if prepareErr != nil {
					return tobari.ManifestReport{}, prepareErr
				}
			}
			bootstrap = &prepared
		}
		return c.context.CreateWithComposition(ctx, intent, name, tobari.BuiltinImageSelector, sourceAccess, tobari.ManifestCreateComposition{
			NativeReadiness: readiness, MethodPolicy: &policy, Bootstrap: bootstrap,
			RuntimeSelection: runtimeSelection, CopyFrom: base,
		})
	}
	if inputs.Provided("--bootstrap-aws-profile") {
		prepared, prepareErr := c.context.PrepareAWSBootstrap(ctx, inputs.One("--bootstrap-aws-profile"))
		if prepareErr != nil {
			return tobari.ManifestReport{}, prepareErr
		}
		if inputs.Provided("--bootstrap-eks-context") {
			prepared, prepareErr = c.context.PrepareEKSBootstrap(ctx, prepared, inputs.One("--bootstrap-eks-context"))
			if prepareErr != nil {
				return tobari.ManifestReport{}, prepareErr
			}
		}
		return c.context.CreateWithComposition(ctx, intent, name, tobari.BuiltinImageSelector, sourceAccess, tobari.ManifestCreateComposition{NativeReadiness: tobari.ManifestNativeReadiness(inputs.One("--native-readiness")), Bootstrap: &prepared, RuntimeSelection: inputs.One("--runtime")})
	}
	return c.context.CreateWithComposition(ctx, intent, name, tobari.BuiltinImageSelector, sourceAccess, tobari.ManifestCreateComposition{NativeReadiness: tobari.ManifestNativeReadiness(inputs.One("--native-readiness")), RuntimeSelection: inputs.One("--runtime")})
}

func contextCreateDirectInputsComplete(inputs ParsedInputs) bool {
	if inputs.Provided("--copy-from") {
		return inputs.Provided("--name")
	}
	for _, name := range []string{"--name", "--runtime", "--source-access", "--native-readiness"} {
		if !inputs.Provided(name) {
			return false
		}
	}
	return true
}

func contextCreateCompositionInputProvided(inputs ParsedInputs) bool {
	for _, name := range []string{"--copy-from", "--name", "--runtime", "--source-access", "--native-readiness", "--bootstrap-aws-profile", "--bootstrap-eks-context"} {
		if inputs.Provided(name) {
			return true
		}
	}
	return false
}

func contextCreateSeedFromInputs(
	ctx context.Context, c *CLI, inputs ParsedInputs, observedBase *tobari.ManifestCopySnapshot,
) (contextCreateWizardSeed, error) {
	seed := contextCreateWizardSeed{}
	if observedBase != nil {
		base := observedBase.Clone()
		var bootstrap *tobari.ManifestBootstrapSnapshot
		if base.Bootstrap != nil {
			copy := base.Bootstrap.Clone()
			bootstrap = &copy
		}
		seed = contextCreateWizardSeed{
			Selection: contextCreateSelection{
				CopyFrom: &base, RuntimeSelection: base.RuntimeSelection,
				SourceAccess: base.SourceAccess, NativeReadiness: base.NativeReadiness,
				MethodPolicy: base.MethodPolicy.Clone(), Bootstrap: bootstrap,
			},
			FilesystemFilled: true, NetworkFilled: true, RuntimeProvided: true, BootstrapFilled: true,
		}
	}
	seed.Selection.Name = inputs.One("--name")
	seed.NameProvided = inputs.Provided("--name")
	if seed.Selection.MethodPolicy.Overrides == nil {
		seed.Selection.MethodPolicy = tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}
	}
	if inputs.Provided("--runtime") {
		seed.Selection.RuntimeSelection = inputs.One("--runtime")
		seed.RuntimeProvided = true
	}
	if inputs.Provided("--source-access") {
		seed.Selection.SourceAccess = tobari.ManifestSourceAccess(inputs.One("--source-access"))
		seed.FilesystemFilled = true
	}
	if inputs.Provided("--native-readiness") {
		seed.Selection.NativeReadiness = tobari.ManifestNativeReadiness(inputs.One("--native-readiness"))
	}
	if inputs.Provided("--native-readiness") {
		seed.NetworkFilled = true
	}
	if seed.Selection.RuntimeSelection == "" {
		seed.Selection.RuntimeSelection = tobari.StandardRuntimeName
	}
	if seed.Selection.SourceAccess == "" {
		seed.Selection.SourceAccess = tobari.ManifestSourceAccessReadWrite
	}
	if !inputs.Provided("--bootstrap-aws-profile") {
		return seed, nil
	}
	seed.BootstrapFilled = true
	prepared, err := c.context.PrepareAWSBootstrap(ctx, inputs.One("--bootstrap-aws-profile"))
	if err != nil {
		return contextCreateWizardSeed{}, err
	}
	if inputs.Provided("--bootstrap-eks-context") {
		prepared, err = c.context.PrepareEKSBootstrap(ctx, prepared, inputs.One("--bootstrap-eks-context"))
		if err != nil {
			return contextCreateWizardSeed{}, err
		}
	}
	seed.Selection.Bootstrap = &prepared
	return seed, nil
}

func selectContextCreateBase(
	ctx context.Context, c *CLI,
) (*tobari.ManifestCopySnapshot, []tobari.ManifestSummary, error) {
	listed, err := c.context.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(listed.Items) == 0 {
		return nil, nil, nil
	}
	options := []configurationWizardOption{{
		label: "Tobari recommended settings", description: "Start from the stable product defaults.", value: "",
	}}
	initial := 0
	for _, item := range listed.Items {
		description := "Initialize a standalone draft from this Workspace Manifest."
		if item.Default {
			description = "Current Workspace Manifest; initialize a standalone draft."
			initial = len(options)
		}
		options = append(options, configurationWizardOption{
			label: safeExternalText(item.Name), description: description, value: item.Name,
		})
	}
	chooser := newContextConfigurationWizardWithStyle(!c.noColor)
	selected, err := chooser.choose(ctx, c.In, c.Err, configurationWizardMenu{
		title: "Tobari · Create Manifest · Copy", current: safeExternalText(listed.DefaultManifest),
		information: []string{"Base initializes this draft once; it creates no lineage or inheritance."},
		prompt:      "Base", options: options, initial: initial,
	})
	if err != nil {
		return nil, listed.Items, normalizeContextCreateWizardError(err)
	}
	if options[selected].value == "" {
		return nil, listed.Items, nil
	}
	base, err := c.context.CopySnapshot(ctx, options[selected].value)
	if err != nil {
		return nil, listed.Items, err
	}
	return &base, listed.Items, nil
}

func normalizeContextCreateWizardError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fault.Wrap(
		fault.KindInternal,
		"manifest_create_wizard_failed",
		"The Workspace Manifest creation wizard failed before creating a Workspace Manifest.",
		false,
		err,
		fault.NextAction{Command: "manifest create", Reason: "Retry in an interactive terminal or use the complete direct input group."},
	)
}

func runManifestDefaultSet(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.ManifestTargetKind, ID: tobari.DefaultManifestSelectionTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help manifest use", "Correct the command arguments.")
	}
	var progress *clusterUpProgress
	var progressSink tobari.ClusterUpProgressSink
	if format == successFormatText && c.tobari != nil && c.tobari.IsTerminal(c.Err) && clusterUpProgressAllowed(ctx) {
		progress = newClusterUpProgress(c.Err, humanStyleAllowed(ctx, c, c.Err))
		progress.Start()
		progressSink = progress.Report
		defer progress.Close()
	}
	result, err := c.context.SetDefaultWithProgress(ctx, intent, inputs.One("--name"), progressSink)
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

func runContextDelete(
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
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help manifest delete", "Correct the command arguments.")
	}
	intent.Target = operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ID: tobari.ManifestCatalogTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.context.Delete(ctx, intent, inputs.One("--name"))
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderContextDelete(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runContextRuntimeSet(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help manifest runtime set", "Correct the command arguments.")
	}
	contextName := ""
	runtimeSelection := inputs.One("--runtime")
	if !inputs.Provided("--runtime") {
		if c.runtime == nil {
			return c.fail(ctx, missingRuntimeFault())
		}
		if !runtimeReviewAvailable(ctx, c, format) {
			return runtimeReviewUnavailable(ctx, c, command, "--runtime")
		}
		contextName, runtimeSelection, err = chooseContextRuntime(ctx, c, inputs)
		if err != nil {
			return c.fail(ctx, normalizeRuntimeReviewError(command.Path, err))
		}
	} else {
		contextName, err = selectedConfigurationContext(ctx, inputs)
		if err != nil {
			return c.fail(ctx, err)
		}
	}
	intent.Target = operation.TargetRef{Kind: tobari.ManifestRuntimeBindingTargetKind, ID: tobari.ManifestRuntimeBindingTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.context.SetRuntime(ctx, intent, contextName, runtimeSelection)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderContextReport(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

type contextListDocument struct {
	SchemaVersion      int `json:"schema_version"`
	WorkspaceManifests struct {
		ManifestState     tobari.ManifestObservationState `json:"workspace_manifest_state"`
		DefaultManifestID string                          `json:"default_manifest_id,omitempty"`
		DefaultManifest   string                          `json:"default_manifest,omitempty"`
		Items             []contextSummaryJSONProjection  `json:"items"`
	} `json:"workspace_manifests"`
}

type contextDeleteDocument struct {
	SchemaVersion    int                         `json:"schema_version"`
	ManifestDeletion tobari.ManifestDeleteResult `json:"workspace_manifest_deletion"`
}

func renderContextDelete(result tobari.ManifestDeleteResult, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_manifest_delete_result", "Workspace Manifest deletion result is invalid", false, err)
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON("manifest delete", contextDeleteDocument{SchemaVersion: 1, ManifestDeletion: result})
		if err != nil {
			return nil, err
		}
		return append(output, '\n'), nil
	}
	var output strings.Builder
	writeStyledLine(&output, color, "Workspace Manifest deleted:", safeExternalText(result.Name), styleSuccess)
	writeStyledLine(&output, color, "Workspace Manifest ID:", result.ID, styleText)
	writeStyledLine(&output, color, "Removed:", "Workspace Manifest-owned manifest, policy snapshot, runtime recipe, and authentication state", styleText)
	writeStyledLine(&output, color, "Preserved:", "project files and shared runtime images", styleText)
	writeStyledLine(&output, color, "Cluster:", string(result.Cluster), humanStatusToken(string(result.Cluster)))
	if result.Cluster == tobari.ManifestClusterStatusRequiresReconcile {
		writeStyledCommandLine(&output, color, "Next:", "run ", "`"+invocationForPath("cluster up")+"`", " to reconcile the shared policy aggregate.")
	}
	return []byte(output.String()), nil
}

type contextSummaryJSONProjection struct {
	ID              *string                          `json:"workspace_manifest_id"`
	Name            string                           `json:"name"`
	ManifestState   tobari.ManifestObservationState  `json:"workspace_manifest_state"`
	Default         bool                             `json:"default"`
	Desired         tobari.WorkspaceManifestRevision `json:"desired"`
	AgentProfile    string                           `json:"agent_profile"`
	Image           string                           `json:"image"`
	SourceAccess    tobari.ManifestSourceAccess      `json:"source_access"`
	PolicyRevision  string                           `json:"policy_revision"`
	NativeReadiness tobari.ManifestNativeReadiness   `json:"native_readiness"`
	MethodPolicy    tobari.ManifestMethodPolicy      `json:"method_policy"`
	RuntimeStatus   tobari.ManifestRuntimeStatus     `json:"runtime_status,omitempty"`
	Bootstrap       contextBootstrapJSONProjection   `json:"bootstrap"`
}

type contextBootstrapJSONProjection struct {
	State      string   `json:"state"`
	Generation uint64   `json:"generation"`
	Revision   string   `json:"revision"`
	Adapters   []string `json:"adapters"`
	AWSProfile string   `json:"aws_profile"`
	EKSContext string   `json:"kubernetes_eks_context"`
}

func contextBootstrapJSON(report tobari.ManifestBootstrapReport) contextBootstrapJSONProjection {
	report = report.Resolved()
	return contextBootstrapJSONProjection{State: report.State, Generation: report.Generation, Revision: report.Revision, Adapters: report.Adapters, AWSProfile: report.AWSProfile, EKSContext: report.EKSContext}
}

type contextReportDocument struct {
	SchemaVersion int                         `json:"schema_version"`
	Manifest      contextReportJSONProjection `json:"workspace_manifest"`
}

type contextReportJSONProjection struct {
	Task             string                                   `json:"task"`
	ManifestState    tobari.ManifestObservationState          `json:"workspace_manifest_state"`
	ID               *string                                  `json:"workspace_manifest_id"`
	Name             string                                   `json:"name"`
	Default          bool                                     `json:"default"`
	Desired          tobari.WorkspaceManifestRevision         `json:"desired"`
	AgentProfile     string                                   `json:"agent_profile"`
	Image            string                                   `json:"image"`
	SourceAccess     tobari.ManifestSourceAccess              `json:"source_access"`
	PolicyRevision   string                                   `json:"policy_revision"`
	NativeReadiness  tobari.ManifestNativeReadiness           `json:"native_readiness"`
	MethodPolicy     tobari.ManifestMethodPolicy              `json:"method_policy"`
	ShellEnvironment []tobari.ManifestShellEnvironmentSetting `json:"shell_environment"`
	GitIdentity      tobari.ManifestGitIdentitySetting        `json:"git_identity"`
	Stores           *tobari.ManifestStorePaths               `json:"stores"`
	Runtime          tobari.ManifestRuntimeReport             `json:"runtime"`
	Cluster          tobari.ManifestClusterStatus             `json:"cluster"`
	Authentication   contextAuthenticationJSONProjection      `json:"authentication"`
	Bootstrap        contextBootstrapJSONProjection           `json:"bootstrap"`
}

func contextReportJSONDocument(result tobari.ManifestReport) contextReportDocument {
	nativeReadiness, _ := tobari.ResolveContextNativeReadiness(result.NativeReadiness)
	return contextReportDocument{
		SchemaVersion: 2,
		Manifest: contextReportJSONProjection{
			Task: result.Task, ManifestState: result.ManifestState, ID: optionalString(result.ID), Name: result.Name, Default: result.Default,
			Desired:      result.Desired,
			AgentProfile: result.AgentProfile, Image: result.Image,
			SourceAccess:    result.SourceAccess,
			PolicyRevision:  result.PolicyRevision,
			NativeReadiness: nativeReadiness, MethodPolicy: result.MethodPolicy,
			ShellEnvironment: result.ShellEnvironment, GitIdentity: result.GitIdentity, Stores: optionalContextStores(result),
			Runtime: result.Runtime, Cluster: result.Cluster,
			Authentication: contextAuthenticationJSON(result.Authentication),
			Bootstrap:      contextBootstrapJSON(result.Bootstrap),
		},
	}
}

func contextAuthenticationMode(authentication tobari.ManifestAuthentication) string {
	if authentication.Mode != "" {
		return authentication.Mode
	}
	if authentication.BrokerState == tobari.ManifestAuthBrokerNotApplicable {
		return tobari.ManifestAuthenticationModeNotApplicable
	}
	return tobari.ManifestAuthenticationModeBroker
}

func optionalContextStores(result tobari.ManifestReport) *tobari.ManifestStorePaths {
	if result.ManifestState == tobari.ManifestObservationAbsent {
		return nil
	}
	stores := result.Stores
	return &stores
}

func contextReportCommand(task string) string {
	return map[string]string{
		tobari.TaskManifestShow: "manifest show", tobari.TaskManifestCreate: "manifest create",
		tobari.TaskManifestDefaultSet: "manifest default set", tobari.TaskConfigShell: "config shell",
		tobari.TaskConfigGit: "config git", tobari.TaskConfigBootstrapAWS: "config bootstrap aws", tobari.TaskConfigBootstrapEKS: "config bootstrap kubernetes eks", tobari.TaskRuntimeInit: "runtime init",
		tobari.TaskRuntimeBuild: "runtime build", tobari.TaskManifestRuntimeSet: "manifest runtime set",
	}[task]
}

func renderContextList(result tobari.ManifestListResult, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_manifest_list", "Workspace Manifest list is invalid", false, err)
	}
	if format == successFormatJSON {
		document := contextListDocument{SchemaVersion: 2}
		document.WorkspaceManifests.ManifestState = result.ManifestState
		document.WorkspaceManifests.DefaultManifestID = result.DefaultManifestID
		document.WorkspaceManifests.DefaultManifest = result.DefaultManifest
		document.WorkspaceManifests.Items = make([]contextSummaryJSONProjection, 0, len(result.Items))
		for _, item := range result.Items {
			nativeReadiness, _ := tobari.ResolveContextNativeReadiness(item.NativeReadiness)
			document.WorkspaceManifests.Items = append(document.WorkspaceManifests.Items, contextSummaryJSONProjection{
				ID: optionalString(item.ID), Name: item.Name, ManifestState: item.ManifestState, Default: item.Default,
				Desired:      item.Desired,
				AgentProfile: item.AgentProfile, Image: item.Image,
				SourceAccess: item.SourceAccess, RuntimeStatus: item.RuntimeStatus,
				PolicyRevision:  item.PolicyRevision,
				NativeReadiness: nativeReadiness, MethodPolicy: item.MethodPolicy,
				Bootstrap: contextBootstrapJSON(item.Bootstrap),
			})
		}
		output, err := marshalCommandJSON("manifest list", document)
		if err != nil {
			return nil, err
		}
		return append(output, '\n'), nil
	}
	if result.ManifestState == tobari.ManifestObservationAbsent {
		output := newHumanOutput(color)
		output.heading("·", "No saved Workspace Manifests", styleMuted)
		output.row("Defaults", "Recommended · not saved", styleWarning)
		output.next(ProgramName, "Review and save a Workspace Manifest, then enter a Workspace.")
		return output.bytes(), nil
	}
	var output strings.Builder
	output.WriteString(applyStyleToken(color, styleAccent, "Workspace Manifests"))
	output.WriteString("\n\n")
	for _, item := range result.Items {
		summary, err := item.RoutineSummary()
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "invalid_manifest_list", "Workspace Manifest list routine summary is invalid", false, err)
		}
		marker := " "
		markerToken := styleMuted
		if item.Default {
			marker = "*"
			markerToken = styleAccent
		}
		actionMarker := ""
		if summary.Action == tobari.ManifestRoutineActionBuildRuntime || summary.Action == tobari.ManifestRoutineActionSelectThenBuild {
			actionMarker = " " + applyStyleToken(color, styleWarning, "!")
		}
		fmt.Fprintf(&output, "%s %s%s\n", applyStyleToken(color, markerToken, marker), applyStyleToken(color, styleText, safeExternalText(item.Name)), actionMarker)
		access := humanContextSourceAccess(summary.Access.SourceAccess) + " · routine clients " + humanRoutineTraffic(summary.Access.RoutineTraffic) +
			" · other " + humanRoutineDecision(summary.Access.MethodPolicy.Default) + " · private denied"
		writeContextCardValue(&output, color, "Access", access, styleText)
		for _, override := range summary.Access.MethodPolicy.Overrides {
			writeContextCardValue(&output, color, "Access "+safeExternalText(override.Method), humanRoutineDecision(override.Decision), methodDecisionStyle(override.Decision))
		}
		writeContextCardValue(&output, color, "Runtime", safeExternalText(summary.RuntimeSelection), humanStatusToken(string(summary.RuntimeStatus)))
		output.WriteString("\n")
	}
	return []byte(strings.TrimRight(output.String(), "\n") + "\n"), nil
}

func writeContextCardRow(output *strings.Builder, color bool, section, subject, value string, token styleToken) {
	fmt.Fprintf(
		output,
		"    %s %s %s\n",
		applyStyleToken(color, styleMuted, fmt.Sprintf("%-10s", section)),
		applyStyleToken(color, styleMuted, fmt.Sprintf("%-10s", subject)),
		applyStyleToken(color, token, value),
	)
}

func writeContextCardValue(output *strings.Builder, color bool, label, value string, token styleToken) {
	fmt.Fprintf(output, "    %s %s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("%-10s", label)), applyStyleToken(color, token, value))
}

func humanMethodDecision(decision tobari.ManifestMethodDecision) string {
	if decision == tobari.ManifestMethodExactReview {
		return "exact-review"
	}
	return string(decision)
}

func renderContextReport(result tobari.ManifestReport, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_manifest_report", "Workspace Manifest report is invalid", false, err)
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON(contextReportCommand(result.Task), contextReportJSONDocument(result))
		if err != nil {
			return nil, err
		}
		return append(output, '\n'), nil
	}
	if result.Task == tobari.TaskManifestShow {
		summary, err := result.RoutineSummary()
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "invalid_manifest_report", "Workspace Manifest routine summary is invalid", false, err)
		}
		return renderContextShowSummary(result, summary, color), nil
	}
	return renderContextReportText(result, color), nil
}

func renderContextReportText(result tobari.ManifestReport, color bool) []byte {
	if result.Task == tobari.TaskManifestShow {
		return renderContextShowSummaryText(result, color)
	}
	if result.Task == tobari.TaskManifestCreate {
		return renderContextCreateSummaryText(result, color)
	}
	if result.Task == tobari.TaskRuntimeInit {
		return renderRuntimeInitReportText(result, color)
	}

	var output strings.Builder
	writeStyledLine(&output, color, "Workspace Manifest:", safeExternalText(result.Name), styleText)
	writeStyledLine(&output, color, "Workspace Manifest state:", string(result.ManifestState), humanStatusToken(string(result.ManifestState)))
	writeStyledLine(&output, color, "Default:", fmt.Sprintf("%t", result.Default), styleText)
	writeStyledLine(&output, color, "Image:", safeExternalText(result.Image), styleText)
	writeStyledLine(&output, color, "Agent profile:", safeExternalText(result.AgentProfile), styleText)
	writeStyledLine(&output, color, "Source access:", "direct "+string(result.SourceAccess), styleText)
	writeStyledLine(&output, color, "Policy revision:", result.PolicyRevision, styleText)
	nativeReadiness, _ := tobari.ResolveContextNativeReadiness(result.NativeReadiness)
	writeStyledLine(&output, color, "Native readiness:", string(nativeReadiness), styleText)
	writeStyledLine(&output, color, "Method default:", string(result.MethodPolicy.Default), styleText)
	for _, override := range result.MethodPolicy.Overrides {
		writeStyledLine(&output, color, "Method "+override.Method+":", string(override.Decision), styleText)
	}
	for _, setting := range result.ShellEnvironment {
		value := string(setting.Source)
		if setting.Source == tobari.ManifestShellEnvironmentLiteral && setting.Value != nil {
			value += " " + fmt.Sprintf("%q", safeExternalText(*setting.Value))
		}
		writeStyledLine(&output, color, "Shell "+setting.Variable+":", value, styleText)
	}
	writeStyledLine(&output, color, "Git identity:", string(result.GitIdentity.Source), styleText)
	if result.GitIdentity.Source == tobari.ManifestGitIdentityLiteral && result.GitIdentity.Name != nil && result.GitIdentity.Email != nil {
		writeStyledLine(&output, color, "Git user.name:", safeExternalText(*result.GitIdentity.Name), styleText)
		writeStyledLine(&output, color, "Git user.email:", safeExternalText(*result.GitIdentity.Email), styleText)
	}
	bootstrap := result.Bootstrap.Resolved()
	writeStyledLine(&output, color, "Workspace bootstrap:", bootstrap.State, humanStatusToken(bootstrap.State))
	if bootstrap.State == tobari.ManifestBootstrapConfigured {
		writeStyledLine(&output, color, "Bootstrap adapters:", strings.Join(bootstrap.Adapters, ", "), styleText)
		writeStyledLine(&output, color, "AWS profile:", safeExternalText(bootstrap.AWSProfile), styleText)
		if bootstrap.EKSContext != "" {
			writeStyledLine(&output, color, "Kubernetes EKS context:", safeExternalText(bootstrap.EKSContext), styleText)
		}
		writeStyledLine(&output, color, "Bootstrap generation:", fmt.Sprintf("%d", bootstrap.Generation), styleText)
		writeStyledLine(&output, color, "Bootstrap revision:", bootstrap.Revision, styleText)
	}
	if result.Task == tobari.TaskManifestShow {
		if contextAuthenticationMode(result.Authentication) == tobari.ManifestAuthenticationModeNative {
			writeStyledLine(&output, color, "Authentication:", "native Workspace-owned", styleSuccess)
			writeStyledLine(&output, color, "Credential state:", "created and persisted by each agent CLI inside this Workspace home; host credentials are not inherited", styleText)
		} else {
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
	}
	if result.Task == tobari.TaskManifestCreate || result.Task == tobari.TaskManifestDefaultSet {
		writeStyledLine(&output, color, "Cluster:", string(result.Cluster), humanStatusToken(string(result.Cluster)))
	}
	if result.Runtime.Kind != "" {
		writeStyledLine(
			&output, color, "Runtime:",
			contextRuntimeDisplay(result.Runtime),
			humanStatusToken(string(result.Runtime.Status)),
		)
		if result.Runtime.Dockerfile != "" {
			writeStyledLine(&output, color, "Runtime Dockerfile:", safeExternalText(result.Runtime.Dockerfile), styleText)
		}
		if result.Runtime.BaseReference != "" {
			writeStyledLine(&output, color, "Runtime base:", safeExternalText(result.Runtime.BaseReference), styleText)
		}
		if result.Runtime.RuntimeID != "" {
			writeStyledLine(&output, color, "Runtime ID:", safeExternalText(result.Runtime.RuntimeID), styleText)
		}
		if result.Runtime.Revision != "" {
			writeStyledLine(&output, color, "Runtime revision:", safeExternalText(result.Runtime.Revision), styleText)
		}
		if result.Runtime.SourceDigest != "" {
			writeStyledLine(&output, color, "Runtime source digest:", safeExternalText(result.Runtime.SourceDigest), styleText)
		}
		if result.Runtime.ImageDigest != "" {
			writeStyledLine(&output, color, "Runtime image digest:", safeExternalText(result.Runtime.ImageDigest), styleText)
		}
	}
	if result.Runtime.Status == tobari.ManifestRuntimeStatusOfficial {
		writeStyledLine(
			&output, color, "Tip:",
			strings.TrimPrefix(runtimeCustomizationHint(), "Tip: "),
			styleText,
		)
	}
	switch result.Task {
	case tobari.TaskManifestCreate:
		if nextArgv := contextCreateNextArgv(result); len(nextArgv) > 0 {
			writeStyledCommandLine(
				&output, color, "Next:", "run ",
				"`"+safeExternalText(strings.Join(nextArgv, " "))+"`",
				" from a project directory to prepare shared services and enter a Workspace.",
			)
		}
	case tobari.TaskConfigShell:
		writeStyledCommandLine(&output, color, "Next:", "start a new session with ", "`"+invocationForPath(WorkspaceEntryCommandPath)+"`", "; running sessions are unchanged.")
	case tobari.TaskConfigGit:
		writeStyledCommandLine(&output, color, "Next:", "re-enter a matching Workspace with ", "`"+invocationForPath(WorkspaceEntryCommandPath)+"`", " to reconcile its Git fallback; this command does not change running sessions.")
	case tobari.TaskConfigBootstrapAWS, tobari.TaskConfigBootstrapEKS:
		writeStyledLine(&output, color, "Scope:", "future Workspaces only; existing Workspace homes are unchanged", styleText)
		writeStyledCommandLine(&output, color, "Next:", "create a new Workspace with ", "`"+invocationForPath(WorkspaceEntryCommandPath)+"`", " to apply this snapshot once.")
	case tobari.TaskRuntimeBuild:
		writeStyledLine(&output, color, "Note:", "existing Workspaces keep their home. On the next `"+invocationForPath(WorkspaceEntryCommandPath)+"`, Tobari recreates only the work container when this runtime image changes the spec.", styleText)
		writeStyledCommandLine(&output, color, "Next:", "run ", "`"+invocationForPath(WorkspaceEntryCommandPath)+"`", " from a project directory.")
	case tobari.TaskManifestRuntimeSet:
		if nextArgv := contextRuntimeSetNextArgv(result); len(nextArgv) > 0 {
			writeStyledCommandLine(
				&output, color, "Next:", "run ",
				"`"+safeExternalText(strings.Join(nextArgv, " "))+"`",
				" from the project directory to adopt the selected Runtime on entry.",
			)
		}
	case tobari.TaskManifestDefaultSet:
		switch result.Cluster {
		case tobari.ManifestClusterStatusDefaultManifestUpdated:
			writeStyledCommandLine(
				&output, color, "Next:", "run ", "`"+invocationForPath(WorkspaceEntryCommandPath)+"`",
				" from a project directory to create or enter a Workspace using the new default Workspace Manifest.",
			)
		case tobari.ManifestClusterStatusReconciled, tobari.ManifestClusterStatusAlreadyReady:
			writeStyledCommandLine(&output, color, "Next:", "run ", "`"+invocationForPath(WorkspaceEntryCommandPath)+"`", " from a project directory.")
		case tobari.ManifestClusterStatusNotConfigured, tobari.ManifestClusterStatusNotRunning:
			writeStyledCommandLine(&output, color, "Next:", "run ", "`"+invocationForPath("cluster up")+"`, then `"+invocationForPath(WorkspaceEntryCommandPath)+"`", " from a project directory.")
		}
	}
	if result.ManifestState != tobari.ManifestObservationAbsent {
		writeStyledLine(&output, color, "Policy:", safeExternalText(result.Stores.PolicyDirectory), styleText)
	}
	return []byte(output.String())
}

func renderContextShowReport(result tobari.ManifestReport, format successFormat, color, details bool) ([]byte, error) {
	if result.Task != tobari.TaskManifestShow {
		return nil, fault.New(fault.KindContract, "invalid_manifest_report", "Workspace Manifest detail presentation received a different task result", false)
	}
	if format == successFormatJSON || !details {
		return renderContextReport(result, format, color)
	}
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_manifest_report", "Workspace Manifest report is invalid", false, err)
	}
	return renderContextShowDetailsText(result, color), nil
}

func renderContextShowSummaryText(result tobari.ManifestReport, color bool) []byte {
	summary, err := result.RoutineSummary()
	if err != nil {
		return nil
	}
	return renderContextShowSummary(result, summary, color)
}

func renderContextShowSummary(result tobari.ManifestReport, summary tobari.ManifestRoutineSummary, color bool) []byte {
	marker, token := contextShowMarkerFromSummary(result, summary)
	nextCommand, nextReason := contextShowNextFromSummary(result, summary)
	output := newHumanOutput(color)
	output.heading(marker, "Workspace Manifest "+safeExternalText(result.Name), token)
	if summary.RecommendedNotSaved {
		output.row("Defaults", "Recommended · not saved", styleWarning)
	}

	output.section("Boundary · fixed for this Workspace Manifest")
	source := humanContextSourceAccess(summary.Access.SourceAccess)
	if summary.Access.SourceAccess == tobari.ManifestSourceAccessReadWrite {
		source += " · changes affect this project directly"
	}
	output.row("Project files", source, styleText)
	output.row("Routine clients", humanRoutineTrafficTitle(summary.Access.RoutineTraffic), humanRoutineTrafficToken(summary.Access.RoutineTraffic))
	output.row("Other requests", humanRoutineDecisionTitle(summary.Access.MethodPolicy.Default), methodDecisionStyle(summary.Access.MethodPolicy.Default))
	for _, override := range summary.Access.MethodPolicy.Overrides {
		output.row(safeExternalText(override.Method)+" requests", humanRoutineDecisionTitle(override.Decision), methodDecisionStyle(override.Decision))
	}
	output.row("Private targets", "Denied", styleWarning)

	output.section("Runtime binding · adopted on next Workspace entry")
	output.row("Selected", safeExternalText(summary.RuntimeSelection), humanStatusToken(string(summary.RuntimeStatus)))

	output.section("Workspace defaults")
	output.subsection("Later entries and sessions")
	output.nestedRow("Shell", humanShellDefault(summary.Defaults.Shell), styleText)
	output.nestedRow("Git identity", humanGitDefault(summary.Defaults.Git), styleText)
	output.subsection("New Workspace homes only · existing homes unchanged")
	writeContextShowWorkspaceSetupDetails(output, result.Bootstrap)

	output.section("Login ownership")
	if summary.AuthenticationMode == tobari.ManifestAuthenticationModeNative {
		output.row("Owner", "Workspace tools · stays in each Workspace", styleSuccess)
	} else {
		output.row("Owner", contextShowAuthentication(result.Authentication), contextShowAuthenticationToken(result.Authentication))
		output.row("Auth status", strings.Join(contextAuthStatusNextArgv(result), " "), styleAccent)
	}

	output.section("Actions")
	output.row("Details", recoveryCommand(contextShowDetailsCommand(result)), styleAccent)
	output.next(nextCommand, nextReason)
	return output.bytes()
}

func humanContextSourceAccess(access tobari.ManifestSourceAccess) string {
	if access == tobari.ManifestSourceAccessReadWrite {
		return "Read-write"
	}
	return "Read-only"
}

func humanRoutineTraffic(state tobari.ManifestRoutineTrafficState) string {
	switch state {
	case tobari.ManifestRoutineTrafficReady:
		return "ready"
	case tobari.ManifestRoutineTrafficLimited:
		return "limited by Workspace Manifest"
	default:
		return "not enabled"
	}
}

func humanRoutineTrafficTitle(state tobari.ManifestRoutineTrafficState) string {
	value := humanRoutineTraffic(state)
	return strings.ToUpper(value[:1]) + value[1:]
}

func humanRoutineTrafficToken(state tobari.ManifestRoutineTrafficState) styleToken {
	switch state {
	case tobari.ManifestRoutineTrafficReady:
		return styleSuccess
	case tobari.ManifestRoutineTrafficLimited:
		return styleWarning
	default:
		return styleMuted
	}
}

func humanRoutineDecision(decision tobari.ManifestMethodDecision) string {
	switch decision {
	case tobari.ManifestMethodAllow:
		return "allowed"
	case tobari.ManifestMethodDeny:
		return "denied"
	default:
		return "exact review"
	}
}

func humanRoutineDecisionTitle(decision tobari.ManifestMethodDecision) string {
	value := humanRoutineDecision(decision)
	return strings.ToUpper(value[:1]) + value[1:]
}

func humanShellDefault(state tobari.ManifestShellDefaultState) string {
	switch state {
	case tobari.ManifestShellDefaultInherited:
		return "Inherited"
	case tobari.ManifestShellDefaultCustomized:
		return "Customized"
	default:
		return "Standard"
	}
}

func humanGitDefault(state tobari.ManifestGitDefaultState) string {
	switch state {
	case tobari.ManifestGitDefaultInherited:
		return "Inherited on entry"
	case tobari.ManifestGitDefaultConfigured:
		return "Configured"
	default:
		return "Not imported"
	}
}

func humanBootstrapDefault(state tobari.ManifestBootstrapDefaultState) string {
	if state == tobari.ManifestBootstrapDefaultConfigured {
		return "Configured for new Workspaces"
	}
	return "None"
}

func renderContextCreateSummaryText(result tobari.ManifestReport, color bool) []byte {
	var nextCommand string
	if nextArgv := contextCreateNextArgv(result); len(nextArgv) > 0 {
		nextCommand = ProgramName
		if len(nextArgv) > 1 {
			nextCommand = strings.Join(nextArgv[1:], " ")
		}
	}
	return renderContextSummaryText(result, color, contextSummaryPresentation{
		Marker:         "✓",
		MarkerToken:    styleSuccess,
		Title:          "Workspace Manifest " + safeExternalText(result.Name) + " created",
		IncludeCluster: true,
		DetailsCommand: contextShowDetailsCommand(result),
		NextCommand:    nextCommand,
		NextReason:     "Prepare shared services and enter a Workspace from a project directory.",
	})
}

type contextSummaryPresentation struct {
	Marker                string
	MarkerToken           styleToken
	Title                 string
	IncludeAuthentication bool
	IncludeCluster        bool
	DetailsCommand        string
	NextCommand           string
	NextReason            string
}

func renderContextSummaryText(result tobari.ManifestReport, color bool, presentation contextSummaryPresentation) []byte {
	output := newHumanOutput(color)
	output.heading(presentation.Marker, presentation.Title, presentation.MarkerToken)
	output.row("State", contextShowState(result), styleText)
	output.row("Source", "direct "+string(result.SourceAccess), styleText)
	output.row("Network", "default "+humanMethodDecision(result.MethodPolicy.Default), styleText)
	for _, override := range result.MethodPolicy.Overrides {
		output.row("Network "+safeExternalText(override.Method), humanMethodDecision(override.Decision), styleText)
	}
	nativeReadiness, _ := tobari.ResolveContextNativeReadiness(result.NativeReadiness)
	policyRevision := result.PolicyRevision
	if policyRevision == "" {
		policyRevision = "not persisted"
	}
	output.row("Policy", "revision "+policyRevision+" · readiness "+string(nativeReadiness), styleText)
	output.row("Profile", "agent "+safeExternalText(result.AgentProfile), styleText)
	output.row("Git identity", contextShowGitIdentity(result.GitIdentity), styleText)
	output.row("Runtime", contextRuntimeDisplay(result.Runtime), humanStatusToken(string(result.Runtime.Status)))
	output.row("Image", safeExternalText(result.Image), styleText)
	if presentation.IncludeAuthentication {
		output.row("Authentication", contextShowAuthentication(result.Authentication), contextShowAuthenticationToken(result.Authentication))
		if contextAuthenticationMode(result.Authentication) != tobari.ManifestAuthenticationModeNative {
			output.row("Auth status", strings.Join(contextAuthStatusNextArgv(result), " "), styleAccent)
		}
	}
	output.row("Bootstrap", contextShowBootstrap(result.Bootstrap), humanStatusToken(result.Bootstrap.Resolved().State))
	if result.Runtime.Dockerfile != "" {
		output.row("Dockerfile", safeExternalText(result.Runtime.Dockerfile), styleText)
	}
	if presentation.IncludeCluster {
		output.row("Cluster", string(result.Cluster), humanStatusToken(string(result.Cluster)))
	}
	if presentation.DetailsCommand != "" {
		output.row("Details", recoveryCommand(presentation.DetailsCommand), styleAccent)
	}
	if presentation.NextCommand != "" {
		output.next(presentation.NextCommand, presentation.NextReason)
	}
	return output.bytes()
}

func renderContextShowDetailsText(result tobari.ManifestReport, color bool) []byte {
	output := newHumanOutput(color)
	marker, token := contextShowMarker(result)
	output.heading(marker, "Workspace Manifest "+safeExternalText(result.Name), token)

	output.section("Workspace Manifest")
	output.row("State", string(result.ManifestState), styleText)
	output.row("Current", humanBool(result.Default), humanOutcomeBoolToken(result.Default))
	output.row("Agent profile", safeExternalText(result.AgentProfile), styleText)
	if result.ID == "" {
		output.row("Workspace Manifest ID", "not assigned", styleMuted)
	} else {
		output.row("Workspace Manifest ID", result.ID, styleText)
	}

	output.section("Boundary · fixed for this Workspace Manifest")
	output.row("Source access", "direct "+string(result.SourceAccess), styleText)
	output.row("Policy revision", result.PolicyRevision, styleText)
	nativeReadiness, _ := tobari.ResolveContextNativeReadiness(result.NativeReadiness)
	output.row("Native readiness", string(nativeReadiness), styleText)
	output.row("Method default", humanMethodDecision(result.MethodPolicy.Default), styleText)
	for _, override := range result.MethodPolicy.Overrides {
		output.row("Method "+safeExternalText(override.Method), humanMethodDecision(override.Decision), styleText)
	}

	output.section("Runtime binding · adopted on next Workspace entry")
	output.row("Selection", contextRuntimeDisplay(result.Runtime), humanStatusToken(string(result.Runtime.Status)))
	output.row("Image", safeExternalText(result.Image), styleText)
	if result.Runtime.Dockerfile != "" {
		output.row("Dockerfile", safeExternalText(result.Runtime.Dockerfile), styleText)
	}
	if result.Runtime.BaseReference != "" {
		output.row("Base image", safeExternalText(result.Runtime.BaseReference), styleText)
	}
	if result.Runtime.RuntimeID != "" {
		output.row("Runtime ID", safeExternalText(result.Runtime.RuntimeID), styleText)
		output.row("Revision", safeExternalText(result.Runtime.Revision), styleText)
	}

	output.section("Workspace defaults")
	output.subsection("Later entries and sessions")
	for _, setting := range result.ShellEnvironment {
		value := string(setting.Source)
		if setting.Source == tobari.ManifestShellEnvironmentLiteral && setting.Value != nil {
			value += " " + fmt.Sprintf("%q", safeExternalText(*setting.Value))
		}
		output.nestedRow("Shell "+setting.Variable, value, styleText)
	}
	output.nestedRow("Git identity", contextShowGitIdentity(result.GitIdentity), styleText)
	output.subsection("New Workspace homes only · existing homes unchanged")
	writeContextShowWorkspaceSetupDetails(output, result.Bootstrap)

	output.section("Login ownership")
	writeContextShowAuthenticationDetails(output, result)

	output.section("Stores and revisions")
	if result.PolicyRevision == "" {
		output.row("Policy revision", "not persisted", styleMuted)
	} else {
		output.row("Policy revision", result.PolicyRevision, styleText)
	}
	if result.Runtime.SourceDigest != "" {
		output.row("Source digest", safeExternalText(result.Runtime.SourceDigest), styleText)
	}
	if result.Runtime.ImageDigest != "" {
		output.row("Image digest", safeExternalText(result.Runtime.ImageDigest), styleText)
	}
	if result.Stores.PolicyDirectory == "" {
		output.row("Policy store", "not persisted", styleMuted)
	} else {
		output.row("Policy store", safeExternalText(result.Stores.PolicyDirectory), styleText)
	}
	if result.Stores.RuntimeDirectory != "" {
		output.row("Runtime store", safeExternalText(result.Stores.RuntimeDirectory), styleText)
		output.row("Runtime file", safeExternalText(result.Stores.RuntimeDockerfile), styleText)
	}

	output.section("Actions")
	nextCommand, nextReason := contextShowNext(result)
	output.next(nextCommand, nextReason)
	return output.bytes()
}

func contextRuntimeDisplay(runtime tobari.ManifestRuntimeReport) string {
	if runtime.Name != "" && runtime.Ordinal > 0 {
		return safeExternalText(fmt.Sprintf("%s@%d", runtime.Name, runtime.Ordinal))
	}
	return string(runtime.Kind) + " · " + string(runtime.Status)
}

func contextShowMarker(result tobari.ManifestReport) (string, styleToken) {
	summary, err := result.RoutineSummary()
	if err != nil {
		return "·", styleMuted
	}
	return contextShowMarkerFromSummary(result, summary)
}

func contextShowMarkerFromSummary(result tobari.ManifestReport, summary tobari.ManifestRoutineSummary) (string, styleToken) {
	if result.ManifestState == tobari.ManifestObservationAbsent {
		return "·", styleMuted
	}
	switch summary.Action {
	case tobari.ManifestRoutineActionEnterCurrent, tobari.ManifestRoutineActionEnterNamed:
		return "✓", styleSuccess
	case tobari.ManifestRoutineActionBuildRuntime, tobari.ManifestRoutineActionSelectThenBuild:
		return "!", styleWarning
	default:
		return "·", styleMuted
	}
}

func contextShowState(result tobari.ManifestReport) string {
	selection := "not current"
	if result.Default {
		selection = "current"
	}
	return string(result.ManifestState) + " · " + selection
}

func contextShowGitIdentity(identity tobari.ManifestGitIdentitySetting) string {
	value := string(identity.Source)
	if identity.Source == tobari.ManifestGitIdentityLiteral && identity.Name != nil && identity.Email != nil {
		value += " · " + safeExternalText(*identity.Name) + " <" + safeExternalText(*identity.Email) + ">"
	}
	return value
}

func contextShowAuthentication(authentication tobari.ManifestAuthentication) string {
	if contextAuthenticationMode(authentication) == tobari.ManifestAuthenticationModeNative {
		return "native Workspace-owned"
	}
	return "broker · " + safeExternalText(authentication.BrokerState) + " · " + fmt.Sprintf("%d providers", len(authentication.Providers))
}

func contextShowAuthenticationToken(authentication tobari.ManifestAuthentication) styleToken {
	if contextAuthenticationMode(authentication) == tobari.ManifestAuthenticationModeNative {
		return styleSuccess
	}
	return humanStatusToken(authentication.BrokerState)
}

func contextShowBootstrap(report tobari.ManifestBootstrapReport) string {
	bootstrap := report.Resolved()
	if bootstrap.State != tobari.ManifestBootstrapConfigured {
		return bootstrap.State
	}
	value := "configured · AWS " + safeExternalText(bootstrap.AWSProfile)
	if bootstrap.EKSContext != "" {
		value += " · EKS " + safeExternalText(bootstrap.EKSContext)
	}
	return value
}

func writeContextShowWorkspaceSetupDetails(output *humanOutput, report tobari.ManifestBootstrapReport) {
	bootstrap := report.Resolved()
	aws, eks := "Not configured", "Not configured"
	if bootstrap.State != tobari.ManifestBootstrapConfigured {
		output.nestedRow("AWS", aws, styleMuted)
		output.nestedRow("Kubernetes EKS", eks, styleMuted)
		return
	}
	aws = safeExternalText(bootstrap.AWSProfile)
	if bootstrap.EKSContext != "" {
		eks = safeExternalText(bootstrap.EKSContext)
	}
	output.nestedRow("AWS", aws, styleText)
	output.nestedRow("Kubernetes EKS", eks, styleText)
	output.nestedRow("Generation", fmt.Sprintf("%d", bootstrap.Generation), styleText)
	output.nestedRow("Setup revision", bootstrap.Revision, styleText)
}

func writeContextShowAuthenticationDetails(output *humanOutput, result tobari.ManifestReport) {
	if contextAuthenticationMode(result.Authentication) == tobari.ManifestAuthenticationModeNative {
		output.row("Owner", "Workspace tools", styleSuccess)
		output.row("Credentials", "agent CLI-owned in this Workspace home; host credentials are not inherited", styleText)
		return
	}
	output.row("Owner", "research Broker", styleWarning)
	output.row("Auth Broker", safeExternalText(result.Authentication.BrokerState), humanStatusToken(result.Authentication.BrokerState))
	output.row("Declared routes", string(authbroker.AuthenticationRouteBrokerRequired), styleText)
	output.row("Other routes", string(authbroker.AuthenticationRouteWorkspaceOwnedCompatibility), styleText)
	for _, provider := range result.Authentication.Providers {
		value := safeExternalText(provider.State)
		if provider.AccountLabel != nil {
			value += " · account " + safeExternalText(*provider.AccountLabel)
		}
		output.row("Auth "+safeExternalText(provider.Provider), value, humanStatusToken(provider.State))
	}
	output.row("Auth status", strings.Join(contextAuthStatusNextArgv(result), " "), styleAccent)
}

func contextShowDetailsCommand(result tobari.ManifestReport) string {
	command := "manifest show"
	if result.ManifestState != tobari.ManifestObservationAbsent && !result.Default {
		command += " --name " + safeExternalText(result.Name)
	}
	return command + " --details"
}

func contextShowNext(result tobari.ManifestReport) (string, string) {
	summary, err := result.RoutineSummary()
	if err != nil {
		return "doctor", "Inspect the Workspace Manifest before continuing."
	}
	return contextShowNextFromSummary(result, summary)
}

func contextShowNextFromSummary(result tobari.ManifestReport, summary tobari.ManifestRoutineSummary) (string, string) {
	switch summary.Action {
	case tobari.ManifestRoutineActionEnterNamed:
		return "--manifest " + safeExternalText(result.Name), "Enter or create a Workspace with this Workspace Manifest."
	case tobari.ManifestRoutineActionBuildRuntime:
		return "runtime build", "Build and validate the default Workspace Manifest runtime."
	case tobari.ManifestRoutineActionSelectThenBuild:
		return "manifest default set --name " + safeExternalText(result.Name), "Select this Workspace Manifest as the default before building its runtime."
	default:
		return ProgramName, "Enter or create a Workspace from a project directory."
	}
}

func contextAuthStatusNextArgv(result tobari.ManifestReport) []string {
	argv := []string{ProgramName, "auth", "status"}
	if result.ManifestState != tobari.ManifestObservationAbsent {
		argv = append(argv, "--manifest", result.Name)
	}
	return argv
}

func contextCreateNextArgv(result tobari.ManifestReport) []string {
	if result.Task != tobari.TaskManifestCreate {
		return nil
	}
	switch result.Cluster {
	case tobari.ManifestClusterStatusNotApplicable, tobari.ManifestClusterStatusRequiresReconcile:
		if result.Default {
			return []string{ProgramName}
		}
		return []string{ProgramName, "--manifest", result.Name}
	default:
		return nil
	}
}

func contextRuntimeSetNextArgv(result tobari.ManifestReport) []string {
	if result.Task != tobari.TaskManifestRuntimeSet {
		return nil
	}
	if result.Default {
		return []string{ProgramName}
	}
	return []string{ProgramName, "--manifest", result.Name}
}

func renderRuntimeInitReportText(result tobari.ManifestReport, color bool) []byte {
	output := newHumanOutput(color)
	output.heading("✓", "Runtime Dockerfile created", styleSuccess)
	output.section("Next")
	output.nextStep(1, "Edit the Dockerfile", safeExternalText(result.Runtime.Dockerfile), styleText)
	output.nextStep(2, "Build the runtime", recoveryCommand("runtime build"), styleAccent)
	output.section("Details")
	output.row("Workspace Manifest", safeExternalText(result.Name), styleText)
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
	return "Tip: create a reusable custom Runtime with `" + invocationForPath("runtime create") + "`, edit its source tree, then run `" + invocationForPath("runtime build") + "` and select the ready revision."
}
