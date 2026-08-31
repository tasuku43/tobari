package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/tasuku43/tobari/internal/app/configuratorcmd"
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

type firstUseSetupChoice uint8

const (
	firstUseSetupCodex firstUseSetupChoice = iota
	firstUseSetupClaude
	firstUseSetupManual
)

type firstUseSetupSelector interface {
	Choose(context.Context, tobari.ConfiguratorSeed, io.Reader, io.Writer) (firstUseSetupChoice, error)
}

type terminalFirstUseSetupSelector struct {
	chooser *terminalContextConfigurationWizard
}

func newFirstUseSetupSelectorWithStyle(style bool) *terminalFirstUseSetupSelector {
	return &terminalFirstUseSetupSelector{chooser: newContextConfigurationWizardWithStyle(style)}
}

func (s *terminalFirstUseSetupSelector) Choose(ctx context.Context, seed tobari.ConfiguratorSeed, in io.Reader, out io.Writer) (firstUseSetupChoice, error) {
	action := "Create"
	guidance := "An agent will understand your Project, explain what it will configure, and prepare the first exact draft."
	prompt := "Choose how to configure this Project"
	options := []configurationWizardOption{
		{label: "Codex", description: "Use Codex with its native sign-in stored in this managed Home.", value: "codex"},
		{label: "Claude Code", description: "Use Claude Code with its native sign-in stored in this managed Home.", value: "claude"},
		{label: "Manual setup", description: "Leave agent configuration and use Tobari's explicit Template commands.", value: "manual"},
	}
	if seed.Purpose == tobari.ConfiguratorPurposeEvolve {
		action = "Update"
		guidance = "An agent will use the current Template, Policy Memory, and Runtime as context for the next exact draft."
	}
	if seed.Task == tobari.ConfiguratorTaskRuntime {
		action = "Edit"
		guidance = "An isolated coding agent will edit only this managed Runtime source. Building remains a separate host-confirmed action."
		prompt = "Choose an agent for Runtime assistance"
		options = options[:2]
	}
	if seed.Task == tobari.ConfiguratorTaskPolicy {
		action = "Edit"
		guidance = "An isolated coding agent will edit only static policy using read-only Policy Memory evidence."
		prompt = "Choose an agent for policy assistance"
		options = options[:2]
	}
	for {
		selected, err := s.chooser.choose(ctx, in, out, configurationWizardMenu{
			title: configuratorSelectorTitle(seed, action),
			informationLines: []configurationWizardLine{
				{{value: "", token: styleText}},
				{{value: guidance, token: styleText}},
				recommendedFirstUseSummaryRow("Runtime", safeExternalText(seed.Runtime().Name)+"@"+fmt.Sprint(seed.Runtime().Ordinal), styleText),
				recommendedFirstUseSummaryRow("Agent Home", "complete managed Home · read-write", styleText),
				recommendedFirstUseSummaryRow("Internet", "direct · Gateway policy is not active", styleWarning),
				recommendedFirstUseSummaryRow("Activation", "host-reviewed Apply only", styleSuccess),
				{{value: "Project files, host Home, Docker socket, and live authority remain unavailable.", token: styleMuted}},
			},
			prompt:  prompt,
			options: options,
		})
		if err != nil {
			return firstUseSetupManual, err
		}
		if selected < 0 || selected >= len(options) {
			return firstUseSetupManual, fmt.Errorf("first-use setup choice is invalid")
		}
		var choice firstUseSetupChoice
		switch selected {
		case 0:
			choice = firstUseSetupCodex
		case 1:
			choice = firstUseSetupClaude
		case 2:
			choice = firstUseSetupManual
		default:
			return firstUseSetupManual, fmt.Errorf("first-use setup choice is invalid")
		}
		agent, agentSelected := setupChoiceAgent(choice)
		if !agentSelected {
			return choice, nil
		}
		open, confirmErr := confirmConfiguratorHandoff(ctx, s.chooser, seed, agent, in, out)
		if confirmErr != nil {
			return firstUseSetupManual, confirmErr
		}
		if open {
			return choice, nil
		}
	}
}

func configuratorSelectorTitle(seed tobari.ConfiguratorSeed, action string) string {
	switch seed.Task {
	case tobari.ConfiguratorTaskRuntime:
		return "Tobari · Edit this Runtime source"
	case tobari.ConfiguratorTaskPolicy:
		return "Tobari · Edit this Template policy"
	default:
		return "Tobari · " + action + " this Project configuration"
	}
}

func confirmConfiguratorHandoff(
	ctx context.Context,
	chooser *terminalContextConfigurationWizard,
	seed tobari.ConfiguratorSeed,
	agent tobari.ConfiguratorAgent,
	in io.Reader,
	out io.Writer,
) (bool, error) {
	if chooser == nil {
		return false, fmt.Errorf("Configurator handoff selector is unavailable")
	}
	if err := seed.Validate(); err != nil {
		return false, err
	}
	agentName := configuratorAgentDisplayName(agent)
	action := "Create this Project configuration"
	home := "New Tobari-managed Home; adopted by the Context after Apply"
	if seed.Purpose == tobari.ConfiguratorPurposeEvolve {
		action = "Update this Project configuration"
		home = "Existing Context Home · read-write"
	}
	if seed.Task == tobari.ConfiguratorTaskRuntime {
		action = "Edit one managed Runtime source"
		home = "Installation Runtime-assistant Home · read-write"
	}
	if seed.Task == tobari.ConfiguratorTaskPolicy {
		action = "Edit one static Template policy"
		home = "Existing Context Home · read-write"
	}
	information := []configurationWizardLine{
		{{value: "", token: styleText}},
		{{value: agentName + " will open inside an isolated Configurator, not directly on your host.", token: styleText}},
	}
	if seed.Task == tobari.ConfiguratorTaskAggregate {
		information = append(information, recommendedFirstUseSummaryRow("Project", safeExternalText(seed.ProjectRoot), styleText))
	}
	information = append(information,
		recommendedFirstUseSummaryRow("Action", action, styleText),
		recommendedFirstUseSummaryRow("Runtime", safeExternalText(seed.Runtime().Name)+"@"+fmt.Sprint(seed.Runtime().Ordinal), styleText),
		recommendedFirstUseSummaryRow("Home", home, styleText),
		recommendedFirstUseSummaryRow("Internet", "direct · Gateway policy is not active", styleWarning),
		recommendedFirstUseSummaryRow("Unavailable", "Host Home · Project files · Docker socket · live authority", styleMuted),
		recommendedFirstUseSummaryRow("Activation", "host-reviewed Apply only", styleSuccess),
		configurationWizardLine{{value: "", token: styleText}},
		configurationWizardLine{{value: "The next screen is " + agentName + "'s native interface.", token: styleAccent}},
		configurationWizardLine{{value: "It may ask you to sign in inside this managed Home.", token: styleMuted}},
	)
	selected, err := chooser.choose(ctx, in, out, configurationWizardMenu{
		title:            "Tobari · " + agentName + " is ready",
		informationLines: information,
		prompt:           "Next",
		options: []configurationWizardOption{
			{label: "Open " + agentName, description: "Enter its native interface inside the isolated Configurator.", value: "open"},
			{label: "Go back", description: "Return without starting " + agentName + ".", value: "back"},
		},
	})
	if err != nil {
		return false, err
	}
	if selected < 0 || selected > 1 {
		return false, fmt.Errorf("Configurator handoff choice is invalid")
	}
	return selected == 0, nil
}

func configuratorAgentDisplayName(agent tobari.ConfiguratorAgent) string {
	switch agent {
	case tobari.ConfiguratorAgentCodex:
		return "Codex"
	case tobari.ConfiguratorAgentClaude:
		return "Claude Code"
	default:
		return safeExternalText(string(agent))
	}
}

type configuratorSubmissionAction uint8

const (
	configuratorSubmissionApply configuratorSubmissionAction = iota
	configuratorSubmissionEdit
	configuratorSubmissionCancel
)

type configuratorSubmissionReviewer interface {
	Review(context.Context, tobari.ConfiguratorSeed, tobari.ConfiguratorSubmission, io.Reader, io.Writer) (configuratorSubmissionAction, error)
}

type terminalConfiguratorSubmissionReviewer struct {
	chooser *terminalContextConfigurationWizard
}

func newConfiguratorSubmissionReviewerWithStyle(style bool) *terminalConfiguratorSubmissionReviewer {
	return &terminalConfiguratorSubmissionReviewer{chooser: newContextConfigurationWizardWithStyle(style)}
}

func (r *terminalConfiguratorSubmissionReviewer) Review(ctx context.Context, seed tobari.ConfiguratorSeed, submission tobari.ConfiguratorSubmission, in io.Reader, out io.Writer) (configuratorSubmissionAction, error) {
	if err := seed.Validate(); err != nil {
		return configuratorSubmissionCancel, err
	}
	if err := submission.Validate(); err != nil {
		return configuratorSubmissionCancel, err
	}
	if seed.Task == tobari.ConfiguratorTaskRuntime {
		if submission.RuntimeSource == nil {
			return configuratorSubmissionCancel, fmt.Errorf("Runtime assist submission has no frozen source")
		}
		state := "No source changes"
		if submission.RuntimeSource.Changed {
			state = string(submission.RuntimeSource.BaseRevision) + " → " + string(submission.RuntimeSource.FrozenRevision)
		}
		selected, err := r.chooser.choose(ctx, in, out, configurationWizardMenu{
			title: "Tobari · Review Runtime source", informationLines: []configurationWizardLine{
				{{value: "", token: styleText}},
				{{value: "Review the exact host-frozen source. The agent container has already stopped.", token: styleText}},
				recommendedFirstUseSummaryRow("Runtime", safeExternalText(submission.Draft.TargetRuntimeID), styleText),
				recommendedFirstUseSummaryRow("Source", state, changedStyle(submission.RuntimeSource.Changed)),
				recommendedFirstUseSummaryRow("Build", "not included · runtime build remains separate", styleMuted),
				recommendedFirstUseSummaryRow("Submission", string(submission.Revision), styleMuted),
			},
			prompt: "Action", options: []configurationWizardOption{
				{label: "Publish source", description: "Replace only this Runtime's editable source with the exact frozen tree.", value: "apply"},
				{label: "Continue editing", description: "Return the same installation-owned Home and working copy to the agent.", value: "edit"},
				{label: "Keep for later", description: "Retain the working copy without publishing it.", value: "cancel"},
			},
		})
		if err != nil {
			return configuratorSubmissionCancel, err
		}
		if selected < 0 || selected > 2 {
			return configuratorSubmissionCancel, fmt.Errorf("Runtime assist review action is invalid")
		}
		return configuratorSubmissionAction(selected), nil
	}
	if seed.Task == tobari.ConfiguratorTaskPolicy {
		beforeRules, afterRules := staticPolicyRuleCount(seed.Initial), staticPolicyRuleCount(submission.Body)
		selected, err := r.chooser.choose(ctx, in, out, configurationWizardMenu{
			title: "Tobari · Review static policy", informationLines: []configurationWizardLine{
				{{value: "", token: styleText}},
				{{value: "Review the exact host-frozen policy source. The agent container has already stopped.", token: styleText}},
				recommendedFirstUseSummaryRow("Template", safeExternalText(string(seed.Evolution.Template.TemplateID)), styleText),
				recommendedFirstUseSummaryRow("Static rules", fmt.Sprintf("%d → %d", beforeRules, afterRules), changedStyle(beforeRules != afterRules)),
				recommendedFirstUseSummaryRow("Policy Memory", "read-only · unchanged", styleMuted),
				recommendedFirstUseSummaryRow("Submission", string(submission.Revision), styleMuted),
			},
			prompt: "Action", options: []configurationWizardOption{
				{label: "Review activation", description: "Continue to the canonical Template Plan and final Apply review.", value: "apply"},
				{label: "Continue editing", description: "Return the same Context Home and policy working copy to the agent.", value: "edit"},
				{label: "Keep for later", description: "Retain the working copy without changing active policy.", value: "cancel"},
			},
		})
		if err != nil {
			return configuratorSubmissionCancel, err
		}
		if selected < 0 || selected > 2 {
			return configuratorSubmissionCancel, fmt.Errorf("policy assist review action is invalid")
		}
		return configuratorSubmissionAction(selected), nil
	}
	action := "Create"
	apply := "Create configuration"
	if seed.Purpose == tobari.ConfiguratorPurposeEvolve {
		action = "Update"
		apply = "Review impact"
	}
	before, after := seed.Initial, submission.Body
	lines := []configurationWizardLine{
		{{value: "", token: styleText}},
		{{value: "Review the host-frozen submission. The container cannot change it now.", token: styleText}},
		recommendedFirstUseSummaryRow("Project access", string(before.Boundary.SourceAccess)+" → "+string(after.Boundary.SourceAccess), changedStyle(before.Boundary.SourceAccess != after.Boundary.SourceAccess)),
		recommendedFirstUseSummaryRow("Runtime", safeExternalText(before.EntryDefaults.Runtime.Name)+"@"+fmt.Sprint(before.EntryDefaults.Runtime.Ordinal)+" → "+safeExternalText(after.EntryDefaults.Runtime.Name)+"@"+fmt.Sprint(after.EntryDefaults.Runtime.Ordinal), changedStyle(before.EntryDefaults.Runtime != after.EntryDefaults.Runtime)),
		recommendedFirstUseSummaryRow("Static policy", fmt.Sprintf("%d → %d rules", staticPolicyRuleCount(before), staticPolicyRuleCount(after)), changedStyle(staticPolicyRuleCount(before) != staticPolicyRuleCount(after))),
		recommendedFirstUseSummaryRow("Submission", string(submission.Revision), styleMuted),
		{{value: "", token: styleText}},
		{{value: "Current configuration", token: styleAccent}},
		{{value: renderConfiguratorJSON(before), token: styleMuted}},
		{{value: "Proposed configuration", token: styleAccent}},
		{{value: renderConfiguratorJSON(after), token: styleText}},
	}
	if submission.RuntimeSource != nil {
		state := "unchanged · " + string(submission.RuntimeSource.FrozenRevision)
		if submission.RuntimeSource.Changed {
			state = string(submission.RuntimeSource.BaseRevision) + " → " + string(submission.RuntimeSource.FrozenRevision)
		}
		lines = append(lines[:len(lines)-4], append([]configurationWizardLine{recommendedFirstUseSummaryRow("Runtime source", state, changedStyle(submission.RuntimeSource.Changed))}, lines[len(lines)-4:]...)...)
	}
	selected, err := r.chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · " + action + " this Project configuration", informationLines: lines,
		prompt: "Action", options: []configurationWizardOption{
			{label: apply, description: "Use this exact frozen submission.", value: "apply"},
			{label: "Continue editing", description: "Return the same managed Home to the agent.", value: "edit"},
			{label: "Cancel", description: "Keep the managed Home but do not Apply.", value: "cancel"},
		},
	})
	if err != nil {
		return configuratorSubmissionCancel, err
	}
	if selected < 0 || selected > 2 {
		return configuratorSubmissionCancel, fmt.Errorf("Configurator submission action is invalid")
	}
	return configuratorSubmissionAction(selected), nil
}

func renderConfiguratorJSON(value any) string {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "<invalid typed configuration>"
	}
	lines := strings.Split(string(data), "\n")
	for index := range lines {
		lines[index] = safeExternalText(lines[index])
	}
	return strings.Join(lines, "\n")
}

type configuratorPlanReviewer interface {
	Review(context.Context, tobari.ConfiguratorSubmission, tobari.WorkspaceTemplateChangePlan, io.Reader, io.Writer) (bool, error)
}

type terminalConfiguratorPlanReviewer struct {
	chooser *terminalContextConfigurationWizard
}

func newConfiguratorPlanReviewerWithStyle(style bool) *terminalConfiguratorPlanReviewer {
	return &terminalConfiguratorPlanReviewer{chooser: newContextConfigurationWizardWithStyle(style)}
}

func (r *terminalConfiguratorPlanReviewer) Review(ctx context.Context, submission tobari.ConfiguratorSubmission, plan tobari.WorkspaceTemplateChangePlan, in io.Reader, out io.Writer) (bool, error) {
	if err := submission.Validate(); err != nil || plan.Validate() != nil || plan.SourceRevision != submission.SourceRevision {
		return false, fmt.Errorf("Configurator Apply review is invalid: %w", err)
	}
	selected, err := r.chooser.choose(ctx, in, out, configurationWizardMenu{
		title: "Tobari · Apply this Project configuration",
		informationLines: []configurationWizardLine{
			{{value: "", token: styleText}},
			{{value: "This is the final Apply decision for the exact frozen configuration.", token: styleText}},
			recommendedFirstUseSummaryRow("Impact", string(plan.Impact), styleWarning),
			recommendedFirstUseSummaryRow("Changed fields", configuratorChangedFields(plan.Diff), styleText),
			recommendedFirstUseSummaryRow("Affected Contexts", fmt.Sprint(plan.AffectedContextCount), styleText),
			recommendedFirstUseSummaryRow("Running Workspaces", fmt.Sprint(plan.RunningWorkspaceCount), changedStyle(plan.RunningWorkspaceCount > 0)),
			recommendedFirstUseSummaryRow("Source revision", string(plan.SourceRevision), styleMuted),
			{{value: "", token: styleText}},
			{{value: "Exact proposed configuration", token: styleAccent}},
			{{value: renderConfiguratorJSON(submission.Body), token: styleText}},
		},
		prompt: "Action",
		options: []configurationWizardOption{
			{label: "Apply exact update", description: "Apply this reviewed Plan and frozen configuration.", value: "apply"},
			{label: "Cancel", description: "Leave active authority unchanged.", value: "cancel"},
		},
	})
	if err != nil {
		return false, err
	}
	return selected == 0, nil
}

func configuratorChangedFields(diff tobari.WorkspaceTemplateChangeDiff) string {
	fields := make([]string, 0, 6)
	for _, item := range []struct {
		name    string
		changed bool
	}{
		{"name", diff.Name}, {"boundary", diff.Boundary}, {"policy", diff.SemanticPolicy}, {"runtime", diff.Runtime}, {"session", diff.SessionDefaults}, {"workspace", diff.WorkspaceDefaults},
	} {
		if item.changed {
			fields = append(fields, item.name)
		}
	}
	if len(fields) == 0 {
		return "none"
	}
	return strings.Join(fields, ", ")
}

func changedStyle(changed bool) styleToken {
	if changed {
		return styleWarning
	}
	return styleText
}

func staticPolicyRuleCount(body tobari.WorkspaceTemplateBody) int {
	return len(body.Policy.BaselineGrants) + len(body.Policy.BaselineTemplates) + len(body.Policy.MCPBaselineGrants) + len(body.Policy.BaselineDenies) + len(body.Policy.GraphQLEndpoints) + len(body.Policy.MCPEndpoints)
}

func setupChoiceAgent(choice firstUseSetupChoice) (tobari.ConfiguratorAgent, bool) {
	switch choice {
	case firstUseSetupCodex:
		return tobari.ConfiguratorAgentCodex, true
	case firstUseSetupClaude:
		return tobari.ConfiguratorAgentClaude, true
	default:
		return "", false
	}
}

func (c *CLI) authorConfiguration(ctx context.Context, seed tobari.ConfiguratorSeed, agent tobari.ConfiguratorAgent) (tobari.ConfiguratorSubmission, error) {
	if c.configurator == nil {
		return tobari.ConfiguratorSubmission{}, fault.New(fault.KindInternal, "missing_configurator", "The Configurator service is unavailable.", false, fault.NextAction{Command: "doctor", Reason: "Inspect the Runtime and Configurator state boundary."})
	}
	intent := operation.Intent{
		Command: "configure", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID},
		Impact: configuratorcmd.Impact(),
	}
	submission, err := c.configurator.Author(ctx, intent, seed, agent, c.In, c.Err, configuratorTaskSettlementFactory(ctx, c))
	if err != nil {
		return tobari.ConfiguratorSubmission{}, err
	}
	writeConfiguratorReturn(c.Err, agent, seed, humanStyleAllowed(ctx, c, c.Err))
	return submission, nil
}

func writeConfiguratorBoundary(out io.Writer, agent tobari.ConfiguratorAgent, seed tobari.ConfiguratorSeed, style bool) error {
	home := "new managed"
	if seed.Purpose == tobari.ConfiguratorPurposeEvolve {
		home = "existing Context"
	}
	title := "Tobari · Entering isolated Configurator"
	confirmation := "host-reviewed configuration"
	if seed.Task == tobari.ConfiguratorTaskRuntime {
		title = "Tobari · Entering isolated Runtime assistant"
		home = "installation agent"
		confirmation = "source publication only"
	}
	if seed.Task == tobari.ConfiguratorTaskPolicy {
		title = "Tobari · Entering isolated Policy assistant"
		confirmation = "Template Plan and Apply"
	}
	_, err := fmt.Fprintf(out, "\n%s\n%s\n\n",
		applyStyleToken(style, styleAccent, title),
		applyStyleToken(style, styleMuted, "Agent: ")+applyStyleToken(style, styleAccent, configuratorAgentDisplayName(agent))+
			applyStyleToken(style, styleMuted, " · Home: ")+applyStyleToken(style, styleText, home)+
			applyStyleToken(style, styleMuted, " · Internet: ")+applyStyleToken(style, styleWarning, "direct")+
			applyStyleToken(style, styleMuted, " · Confirm: ")+applyStyleToken(style, styleSuccess, confirmation))
	return err
}

func configuratorBoundaryOutputFault(err error) error {
	return fault.WithClassification(fault.Wrap(
		fault.KindInternal, "configurator_boundary_output_failed",
		"The isolated Configurator boundary could not be written before Runtime preparation.", false, err,
		fault.NextAction{Command: "help configure", Reason: "Retry with a writable interactive terminal before preparing Runtime material."},
	), fault.PhasePrecondition, fault.ChangeNone)
}

func writeConfiguratorReturn(out io.Writer, agent tobari.ConfiguratorAgent, seed tobari.ConfiguratorSeed, style bool) {
	message := "The exact configuration draft is frozen for host review."
	if seed.Task == tobari.ConfiguratorTaskRuntime {
		message = "The exact Runtime source tree is frozen for host review; no build has run."
	}
	if seed.Task == tobari.ConfiguratorTaskPolicy {
		message = "The exact policy.yaml is frozen for host review; Policy Memory is unchanged."
	}
	_, _ = fmt.Fprintf(out, "\n%s\n%s\n\n",
		applyStyleToken(style, styleAccent, "Tobari · Returned from "+configuratorAgentDisplayName(agent)),
		applyStyleToken(style, styleMuted, message))
}

func runConfigure(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.configurator == nil || c.finalDefaultPair == nil || c.finalEntryReadiness == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	if c.interactive == nil || !c.interactive(c.In, c.Out, c.Err) {
		return c.fail(ctx, fault.WithClassification(fault.New(fault.KindRejected, "configurator_interactive_required", "Configurator requires an interactive terminal.", false, fault.NextAction{Command: "help configure", Reason: "Run the selected agent from an interactive terminal."}), fault.PhasePrecondition, fault.ChangeNone))
	}
	selected, err := c.finalDefaultPair.Select(ctx, c.In, c.Err)
	if err != nil {
		return c.fail(ctx, err)
	}
	observation, err := selected.Selection.Observation(selected.Choice)
	if err != nil {
		return c.fail(ctx, err)
	}
	configureIntent := operation.Intent{Command: "configure", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, Impact: configuratorcmd.Impact()}
	var resumedSubmission *tobari.ConfiguratorSubmission
	var resumedStage *tobari.ConfiguratorPendingStage
	if pending, found, pendingErr := c.configurator.PendingHomeAdoption(ctx, observation.ProjectRoot); pendingErr != nil {
		return c.fail(ctx, pendingErr)
	} else if found {
		if observation.Context != nil {
			if err := c.configurator.ArmHomeAdoption(ctx, configureIntent, pending); err != nil {
				return c.fail(ctx, err)
			}
			if adoptErr := withConfiguratorAttachment(ctx, c, configureIntent, pending, func() error {
				return c.configurator.AdoptHome(ctx, configureIntent, pending, *observation.Context)
			}); adoptErr != nil {
				return c.fail(ctx, adoptErr)
			}
			writeConfiguratorSuccess(c, "Configuration applied")
			return ExitOK
		}
		publicationReady := pending.Draft.Purpose == tobari.ConfiguratorPurposeBootstrap ||
			observation.DefaultTemplate != nil && pending.Draft.Purpose == tobari.ConfiguratorPurposeEvolve && pending.Draft.TemplateID == observation.DefaultTemplate.ID && pending.SourceRevision == observation.DefaultTemplate.Current.Revision
		if publicationReady {
			var resolution workspaceauthoritycmd.DefaultPairResolution
			if err := c.configurator.ArmHomeAdoption(ctx, configureIntent, pending); err != nil {
				return c.fail(ctx, err)
			}
			resolveErr := withConfiguratorAttachment(ctx, c, configureIntent, pending, func() error {
				rootIntent := operation.Intent{Command: workspaceauthoritycmd.TaskDefaultPairEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}, Impact: workspaceauthoritycmd.DefaultPairEnterImpact()}
				var body *tobari.WorkspaceTemplateBody
				if pending.Draft.Purpose == tobari.ConfiguratorPurposeBootstrap {
					value := pending.Body.Clone()
					body = &value
				}
				var err error
				resolution, err = c.finalDefaultPair.ResolveSelectedWithConfiguratorIDs(ctx, rootIntent, body, pending.Draft.TemplateID, pending.Draft.AdoptionContextID, selected)
				if err != nil {
					return err
				}
				if resolution.Observation.Context == nil {
					return fmt.Errorf("Configurator recovery created no Context")
				}
				return c.configurator.AdoptHome(ctx, configureIntent, pending, *resolution.Observation.Context)
			})
			if resolveErr != nil {
				return c.fail(ctx, resolveErr)
			}
			writeConfiguratorSuccess(c, "Configuration applied")
			return ExitOK
		}
		if pending.Draft.Purpose == tobari.ConfiguratorPurposeEvolve {
			value := pending
			resumedSubmission = &value
		}
	}
	pending, found, pendingErr := c.configurator.PendingStageForProject(ctx, observation.ProjectRoot)
	if pendingErr != nil {
		return c.fail(ctx, pendingErr)
	}
	if found {
		if pending.Submission.Draft.ProjectRoot != observation.ProjectRoot || observation.Context != nil && pending.Submission.Draft.ContextID != "" && pending.Submission.Draft.ContextID != observation.Context.Context.ID {
			return c.fail(ctx, tobari.ErrResourceSourceRecoveryRequired)
		}
		value := pending
		resumedStage = &value
		submission := pending.Submission
		resumedSubmission = &submission
	}
	var seed tobari.ConfiguratorSeed
	if observation.Context != nil {
		seed, err = tobari.NewEvolveConfiguratorSeed(observation.ProjectRoot, *observation.Context)
	} else if observation.CollectionPresent && observation.DefaultTemplate != nil {
		var standard tobari.WorkspaceTemplateBody
		standard, err = c.firstUseStandardTemplateBody(ctx)
		if err == nil {
			seed, err = tobari.NewDetachedEvolveConfiguratorSeed(observation.ProjectRoot, observation.DefaultTemplate.Current, standard.EntryDefaults.Runtime)
		}
	} else {
		var initial tobari.WorkspaceTemplateBody
		if observation.DefaultTemplate != nil {
			initial = observation.DefaultTemplate.Current.Body.Clone()
		} else {
			initial, err = c.firstUseStandardTemplateBody(ctx)
		}
		if err == nil {
			seed, err = tobari.NewBootstrapConfiguratorSeed(observation.ProjectRoot, initial)
		}
	}
	if err != nil {
		return c.fail(ctx, err)
	}
	intent = configureIntent
	var submission tobari.ConfiguratorSubmission
	if resumedSubmission != nil {
		submission = *resumedSubmission
	} else {
		if err := checkConfiguratorRequirements(ctx, c); err != nil {
			return c.fail(ctx, err)
		}
		var agent tobari.ConfiguratorAgent
		if inputs.Provided("--agent") {
			agent, err = tobari.ParseConfiguratorAgent(inputs.One("--agent"))
			if err == nil {
				open, confirmErr := confirmConfiguratorHandoff(
					ctx, newContextConfigurationWizardWithStyle(!c.noColor), seed, agent, c.In, c.Err,
				)
				if confirmErr != nil {
					return c.fail(ctx, confirmErr)
				}
				if !open {
					return c.fail(ctx, fault.New(fault.KindCanceled, "configuration_canceled", "Configuration was canceled before starting the selected agent.", false))
				}
			}
		} else {
			selector := c.firstUseSetup
			if selector == nil {
				selector = newFirstUseSetupSelectorWithStyle(!c.noColor)
			}
			choice, chooseErr := selector.Choose(ctx, seed, c.In, c.Err)
			if chooseErr != nil {
				return c.fail(ctx, chooseErr)
			}
			var selectedAgent bool
			agent, selectedAgent = setupChoiceAgent(choice)
			if !selectedAgent {
				return c.fail(ctx, fault.New(fault.KindCanceled, "configuration_canceled", "Configuration was canceled.", false))
			}
		}
		if err != nil {
			return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help configure", "Choose codex or claude.")
		}
		if err := writeConfiguratorBoundary(c.Err, agent, seed, humanStyleAllowed(ctx, c, c.Err)); err != nil {
			return c.fail(ctx, configuratorBoundaryOutputFault(err))
		}
		for {
			submission, err = c.authorConfiguration(ctx, seed, agent)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, tobari.ErrNativeLoginBridgeUnavailable) {
					err = configurationMaterialRetainedFault(err)
				}
				return c.fail(ctx, err)
			}
			reviewer := c.configuratorReview
			if reviewer == nil {
				reviewer = newConfiguratorSubmissionReviewerWithStyle(!c.noColor)
			}
			action, reviewErr := reviewer.Review(ctx, seed, submission, c.In, c.Err)
			if reviewErr != nil {
				return c.fail(ctx, normalizeFirstUseReviewError(reviewErr))
			}
			if action == configuratorSubmissionEdit {
				if boundaryErr := writeConfiguratorBoundary(c.Err, agent, seed, humanStyleAllowed(ctx, c, c.Err)); boundaryErr != nil {
					return c.fail(ctx, configurationMaterialRetainedFault(boundaryErr))
				}
				continue
			}
			if action != configuratorSubmissionApply {
				return c.fail(ctx, configurationMaterialRetainedFault(context.Canceled))
			}
			break
		}
	}
	if submission.RuntimeSource != nil && submission.RuntimeSource.Changed {
		reviewedRevision := submission.Revision
		buildOutput := newRuntimeBuildOutput(c.Err, humanStyleAllowed(ctx, c, c.Err))
		submission, err = c.configurator.ApplyRuntimeSource(ctx, intent, submission, buildOutput)
		if err != nil {
			return c.fail(ctx, err)
		}
		buildOutput.Flush()
		if submission.Revision != reviewedRevision {
			reviewer := c.configuratorReview
			if reviewer == nil {
				reviewer = newConfiguratorSubmissionReviewerWithStyle(!c.noColor)
			}
			action, reviewErr := reviewer.Review(ctx, seed, submission, c.In, c.Err)
			if reviewErr != nil {
				return c.fail(ctx, normalizeFirstUseReviewError(reviewErr))
			}
			if action != configuratorSubmissionApply {
				return c.fail(ctx, configurationRuntimeReviewPendingFault())
			}
		}
	}
	if seed.Purpose == tobari.ConfiguratorPurposeBootstrap {
		if armErr := c.configurator.ArmHomeAdoption(ctx, intent, submission); armErr != nil {
			return c.fail(ctx, armErr)
		}
		release, leaseErr := c.configurator.AcquirePublicationAttachment(ctx, intent, submission)
		if leaseErr != nil {
			return c.fail(ctx, leaseErr)
		}
		defer func() { _ = release() }()
		rootIntent := operation.Intent{Command: workspaceauthoritycmd.TaskDefaultPairEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}, Impact: workspaceauthoritycmd.DefaultPairEnterImpact()}
		body := submission.Body.Clone()
		resolution, applyErr := c.finalDefaultPair.ResolveSelectedWithConfiguratorIDs(ctx, rootIntent, &body, submission.Draft.TemplateID, submission.Draft.AdoptionContextID, selected)
		if applyErr != nil {
			return c.fail(ctx, applyErr)
		}
		if resolution.Observation.Context == nil {
			return c.fail(ctx, fmt.Errorf("Configurator Apply created no Context"))
		}
		if applyErr = c.configurator.AdoptHome(ctx, intent, submission, *resolution.Observation.Context); applyErr != nil {
			return c.fail(ctx, applyErr)
		}
		_ = release()
		release = func() error { return nil }
		writeConfiguratorSuccess(c, "Configuration applied")
		_, _ = fmt.Fprintln(c.Err, "\nRun tobari to prepare protection and enter the Workspace.")
		err = nil
	} else {
		if c.finalTemplates == nil {
			return c.fail(ctx, missingRuntimeFault())
		}
		if resumedStage != nil && resumedStage.ApplyConfirmed {
			return resumeConfirmedConfiguratorApply(ctx, c, intent, submission, *resumedStage)
		}
		stage, stageErr := c.configurator.Stage(ctx, intent, submission)
		if stageErr != nil {
			return c.fail(ctx, stageErr)
		}
		plan, planErr := c.finalTemplates.Plan(ctx, stage.TemplateRef)
		if planErr != nil {
			discardErr := discardConfiguratorStageAfterReview(ctx, c, intent, submission, stage)
			return c.fail(ctx, errors.Join(discardErr, planErr))
		}
		if plan.SourceRevision != stage.SourceRevision || plan.SourceFingerprint != stage.SourceFingerprint {
			discardErr := discardConfiguratorStageAfterReview(ctx, c, intent, submission, stage)
			return c.fail(ctx, errors.Join(discardErr, tobari.ErrWorkspaceTemplateChangePlanStale))
		}
		pending := tobari.ConfiguratorPendingStage{Submission: submission, Stage: stage}
		if resumedStage != nil {
			pending = *resumedStage
			if pending.Stage != stage || !reflect.DeepEqual(pending.Submission, submission) || pending.PlanRef != "" && pending.PlanRef != plan.PlanRef {
				return c.fail(ctx, tobari.ErrResourceSourceRecoveryRequired)
			}
		}
		pending, err = c.configurator.BindStagePlan(ctx, intent, pending, plan.PlanRef)
		if err != nil {
			return c.fail(ctx, err)
		}
		planReviewer := c.configuratorPlanReview
		if planReviewer == nil {
			planReviewer = newConfiguratorPlanReviewerWithStyle(!c.noColor)
		}
		confirmed, planReviewErr := planReviewer.Review(ctx, submission, plan, c.In, c.Err)
		if planReviewErr != nil {
			discardErr := discardConfiguratorStageAfterReview(ctx, c, intent, submission, stage)
			return c.fail(ctx, errors.Join(discardErr, normalizeFirstUseReviewError(planReviewErr)))
		}
		if !confirmed {
			if discardErr := discardConfiguratorStageAfterReview(ctx, c, intent, submission, stage); discardErr != nil {
				return c.fail(ctx, discardErr)
			}
			return c.fail(ctx, fault.New(fault.KindCanceled, "configuration_canceled", "Configuration was canceled before Apply.", false))
		}
		var adoptionRelease func() error
		if submission.Draft.NeedsHomeAdoption() {
			if armErr := c.configurator.ArmHomeAdoption(ctx, intent, submission); armErr != nil {
				return c.fail(ctx, armErr)
			}
			adoptionRelease, err = c.configurator.AcquirePublicationAttachment(ctx, intent, submission)
			if err != nil {
				return c.fail(ctx, err)
			}
			defer func() {
				if adoptionRelease != nil {
					_ = adoptionRelease()
				}
			}()
		}
		pending, err = c.configurator.ConfirmStageApply(ctx, intent, pending)
		if err != nil {
			return c.fail(ctx, err)
		}
		applyIntent := operation.Intent{Command: workspaceauthoritycmd.TaskTemplateApply, Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.WorkspaceTemplateChangePlanReferenceKind, ID: plan.PlanRef}, Impact: workspaceauthoritycmd.TemplateApplyImpact()}
		result, applyErr := c.finalTemplates.Apply(ctx, applyIntent, plan.PlanRef)
		if applyErr != nil {
			return c.fail(ctx, applyErr)
		}
		status := "Configuration already current"
		if result.Changed {
			status = "Configuration applied"
		}
		if submission.Draft.NeedsHomeAdoption() {
			postSelected, selectErr := c.finalDefaultPair.Select(ctx, c.In, c.Err)
			if selectErr != nil {
				return c.fail(ctx, selectErr)
			}
			rootIntent := operation.Intent{Command: workspaceauthoritycmd.TaskDefaultPairEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}, Impact: workspaceauthoritycmd.DefaultPairEnterImpact()}
			resolution, resolveErr := c.finalDefaultPair.ResolveSelectedWithConfiguratorIDs(ctx, rootIntent, nil, submission.Draft.TemplateID, submission.Draft.AdoptionContextID, postSelected)
			if resolveErr != nil {
				return c.fail(ctx, resolveErr)
			}
			if resolution.Observation.Context == nil {
				return c.fail(ctx, fmt.Errorf("Configurator Apply created no Context"))
			}
			if adoptErr := c.configurator.AdoptHome(ctx, intent, submission, *resolution.Observation.Context); adoptErr != nil {
				return c.fail(ctx, adoptErr)
			}
			_ = adoptionRelease()
			adoptionRelease = nil
		}
		writeConfiguratorSuccess(c, status)
		err = nil
	}
	if err != nil {
		return c.fail(ctx, err)
	}
	return ExitOK
}

func withConfiguratorAttachment(ctx context.Context, c *CLI, intent operation.Intent, submission tobari.ConfiguratorSubmission, action func() error) error {
	release, err := c.configurator.AcquirePublicationAttachment(ctx, intent, submission)
	if err != nil {
		return err
	}
	actionErr := action()
	releaseErr := release()
	if actionErr != nil {
		return errors.Join(actionErr, releaseErr)
	}
	// Home adoption is terminal confirmed state. A later lease-fd close
	// diagnostic cannot turn success into replay permission.
	return nil
}

func writeConfiguratorSuccess(c *CLI, status string) {
	if c == nil || c.Err == nil {
		return
	}
	_, _ = fmt.Fprintln(c.Err, applyStyleToken(!c.noColor, styleSuccess, "✓ "+status))
}

func configurationRuntimeReviewPendingFault() error {
	return fault.WithClassification(fault.New(
		fault.KindRejected,
		"configuration_runtime_review_pending",
		"The Runtime revision was published, but the final Runtime-bound configuration was not approved.",
		false,
		fault.NextAction{Command: "review runtimes", Reason: "Reconcile the confirmed Runtime revision before resuming configure."},
	), fault.PhaseVerification, fault.ChangeConfirmed)
}

func configurationMaterialRetainedFault(cause error) error {
	return configuratorcmd.MaterialRetainedFault(tobari.ConfiguratorTaskAggregate, cause)
}

func resumeConfirmedConfiguratorApply(ctx context.Context, c *CLI, intent operation.Intent, submission tobari.ConfiguratorSubmission, pending tobari.ConfiguratorPendingStage) int {
	var adoptionRelease func() error
	if submission.Draft.NeedsHomeAdoption() {
		if armErr := c.configurator.ArmHomeAdoption(ctx, intent, submission); armErr != nil {
			return c.fail(ctx, armErr)
		}
		var leaseErr error
		adoptionRelease, leaseErr = c.configurator.AcquirePublicationAttachment(ctx, intent, submission)
		if leaseErr != nil {
			return c.fail(ctx, leaseErr)
		}
		defer func() {
			if adoptionRelease != nil {
				_ = adoptionRelease()
			}
		}()
	}
	applyIntent := operation.Intent{Command: workspaceauthoritycmd.TaskTemplateApply, Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.WorkspaceTemplateChangePlanReferenceKind, ID: pending.PlanRef}, Impact: workspaceauthoritycmd.TemplateApplyImpact()}
	result, err := c.finalTemplates.Apply(ctx, applyIntent, pending.PlanRef)
	if err != nil {
		return c.fail(ctx, err)
	}
	if submission.Draft.NeedsHomeAdoption() {
		postSelected, selectErr := c.finalDefaultPair.Select(ctx, c.In, c.Err)
		if selectErr != nil {
			return c.fail(ctx, selectErr)
		}
		rootIntent := operation.Intent{Command: workspaceauthoritycmd.TaskDefaultPairEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}, Impact: workspaceauthoritycmd.DefaultPairEnterImpact()}
		resolution, resolveErr := c.finalDefaultPair.ResolveSelectedWithConfiguratorIDs(ctx, rootIntent, nil, submission.Draft.TemplateID, submission.Draft.AdoptionContextID, postSelected)
		if resolveErr != nil {
			return c.fail(ctx, resolveErr)
		}
		if resolution.Observation.Context == nil {
			return c.fail(ctx, fmt.Errorf("Configurator Apply recovery created no Context"))
		}
		if adoptErr := c.configurator.AdoptHome(ctx, intent, submission, *resolution.Observation.Context); adoptErr != nil {
			return c.fail(ctx, adoptErr)
		}
		_ = adoptionRelease()
		adoptionRelease = nil
	}
	status := "Configuration already current"
	if result.Changed {
		status = "Configuration applied"
	}
	writeConfiguratorSuccess(c, status)
	return ExitOK
}

func discardConfiguratorStageAfterReview(ctx context.Context, c *CLI, intent operation.Intent, submission tobari.ConfiguratorSubmission, stage tobari.ConfiguratorStage) error {
	settlementBase := ctx
	if c != nil && c.processLifetime != nil {
		settlementBase = c.processLifetime
	}
	settlementContext, cancel := context.WithTimeout(settlementBase, firstEntryClassificationTimeout)
	defer cancel()
	return c.configurator.DiscardStage(settlementContext, intent, submission, stage)
}

func checkConfiguratorRequirements(ctx context.Context, c *CLI) error {
	if err := c.finalEntryReadiness.Check(ctx); err != nil {
		return err
	}
	_, err := fmt.Fprintln(c.Err, applyStyleToken(!c.noColor, styleSuccess, "✓ Requirements ready"))
	return err
}

func prepareConfiguratorRuntime(ctx context.Context, c *CLI, seed tobari.ConfiguratorSeed) error {
	progress := newInteractiveWorkProgress(c.Err, "Preparing configuration Runtime", terminal.IsTerminal(c.Err), humanStyleAllowed(ctx, c, c.Err))
	progress.Start()
	intent := operation.Intent{Command: "configure", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, Impact: configuratorcmd.Impact()}
	err := c.configurator.PrepareRuntime(ctx, intent, seed)
	progress.Stop()
	if err != nil {
		return err
	}
	// Runtime material is confirmed before presentation. A cosmetic write
	// failure cannot turn that success into replay permission.
	_, _ = fmt.Fprintln(c.Err, applyStyleToken(!c.noColor, styleSuccess, "✓ Runtime ready")+"  "+applyStyleToken(!c.noColor, styleMuted, safeExternalText(seed.Runtime().Name)+"@"+fmt.Sprint(seed.Runtime().Ordinal)))
	return nil
}

func writeConfiguratorPlan(out io.Writer, plan tobari.WorkspaceTemplateChangePlan, style bool) error {
	_, err := fmt.Fprintf(out, "\n%s\n%s\n%s\n%s\n\n",
		applyStyleToken(style, styleAccent, "Tobari · Applying reviewed configuration"),
		applyStyleToken(style, styleMuted, "Impact")+"  "+applyStyleToken(style, styleWarning, string(plan.Impact)),
		applyStyleToken(style, styleMuted, "Contexts")+fmt.Sprintf("  %d", plan.AffectedContextCount),
		applyStyleToken(style, styleMuted, "Running Workspaces")+fmt.Sprintf("  %d", plan.RunningWorkspaceCount))
	return err
}
