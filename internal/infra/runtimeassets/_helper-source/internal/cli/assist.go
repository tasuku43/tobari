package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/tasuku43/tobari/internal/app/configuratorcmd"
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func runRuntimeAssist(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.configurator == nil || c.finalEntryReadiness == nil || c.runtime == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	if c.interactive == nil || !c.interactive(c.In, c.Out, c.Err) {
		return c.fail(ctx, fault.WithClassification(fault.New(
			fault.KindRejected, "configurator_interactive_required", "Runtime assistance requires an interactive terminal.", false,
			fault.NextAction{Command: "help runtime assist", Reason: "Run the isolated coding agent from an interactive terminal."},
		), fault.PhasePrecondition, fault.ChangeNone))
	}
	runtimeRef := inputs.One("--id")
	manifest, sourceRevision, err := c.runtime.ResolveManagedSourceReference(ctx, runtimeRef)
	if err != nil {
		return c.fail(ctx, err)
	}
	executionRuntime, err := c.runtime.BindingByReference(ctx, tobari.StandardRuntimeID, 1)
	if err != nil {
		return c.fail(ctx, err)
	}
	seed, err := tobari.NewRuntimeAssistConfiguratorSeed(executionRuntime, manifest.ID, sourceRevision)
	if err != nil {
		return c.fail(ctx, err)
	}
	intent.Target = operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: runtimeRef}
	intent.Impact = command.Agent.Mutation.Impact
	settlement := configuratorTaskSettlementFactory(ctx, c)
	recovery, retained, err := c.configurator.ReconcileTask(ctx, seed)
	if err != nil {
		return c.fail(ctx, classifyAssistRecoveryError(seed.Task, err))
	}
	if retained {
		if err := validateExplicitAssistRecoveryAgent(recovery, inputs.One("--agent")); err != nil {
			return c.fail(ctx, classifyAssistRecoveryError(seed.Task, err))
		}
	}
	if retained && recovery.Published {
		if err := c.configurator.CompleteTask(ctx, intent, recovery.Submission, settlement); err != nil {
			return c.fail(ctx, err)
		}
		return c.emitMutationResult(ctx, command, renderRuntimeAssistSuccess(manifest, recovery.Submission, humanStyleAllowed(ctx, c, c.Out)))
	}
	var agent tobari.ConfiguratorAgent
	var submission tobari.ConfiguratorSubmission
	frozen, confirmed := false, false
	if retained {
		if err := validateAssistRecovery(seed, recovery, inputs.One("--agent")); err != nil {
			return c.fail(ctx, classifyAssistRecoveryError(seed.Task, err))
		}
		agent, submission, frozen, confirmed = recovery.Draft.Agent, recovery.Submission, recovery.Frozen, recovery.ApplyConfirmed
	} else {
		agent, err = chooseAssistAgent(ctx, c, seed, inputs.One("--agent"))
		if err != nil {
			return c.fail(ctx, err)
		}
	}
	for {
		if !frozen {
			if err := writeConfiguratorBoundary(c.Err, agent, seed, humanStyleAllowed(ctx, c, c.Err)); err != nil {
				return c.fail(ctx, configuratorBoundaryOutputFaultFor("runtime assist", err))
			}
			if err := c.finalEntryReadiness.Check(ctx); err != nil {
				return c.fail(ctx, err)
			}
			var authorErr error
			submission, authorErr = c.configurator.Author(ctx, intent, seed, agent, c.In, c.Err, settlement)
			if authorErr != nil {
				if errors.Is(authorErr, context.Canceled) || errors.Is(authorErr, context.DeadlineExceeded) || errors.Is(authorErr, tobari.ErrNativeLoginBridgeUnavailable) {
					authorErr = configuratorcmd.MaterialRetainedFault(seed.Task, authorErr)
				}
				return c.fail(ctx, authorErr)
			}
			writeConfiguratorReturn(c.Err, agent, seed, humanStyleAllowed(ctx, c, c.Err))
			frozen = true
		}
		if !confirmed {
			reviewer := c.configuratorReview
			if reviewer == nil {
				reviewer = newConfiguratorSubmissionReviewerWithStyle(!c.noColor)
			}
			action, reviewErr := reviewer.Review(ctx, seed, submission, c.In, c.Err)
			if reviewErr != nil {
				return c.fail(ctx, reviewErr)
			}
			if action == configuratorSubmissionEdit {
				frozen = false
				continue
			}
			if action != configuratorSubmissionApply {
				return c.fail(ctx, configuratorcmd.MaterialRetainedFault(seed.Task, context.Canceled))
			}
			confirmed = true
		}
		if err := c.configurator.ApplyRuntimeAssistSource(ctx, intent, submission, settlement); err != nil {
			return c.fail(ctx, err)
		}
		return c.emitMutationResult(ctx, command, renderRuntimeAssistSuccess(manifest, submission, humanStyleAllowed(ctx, c, c.Out)))
	}
}

func runPolicyAssist(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.configurator == nil || c.finalContexts == nil || c.finalEntryReadiness == nil || c.finalTemplates == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	if c.interactive == nil || !c.interactive(c.In, c.Out, c.Err) {
		return c.fail(ctx, fault.WithClassification(fault.New(
			fault.KindRejected, "configurator_interactive_required", "Policy assistance requires an interactive terminal.", false,
			fault.NextAction{Command: "help policy assist", Reason: "Run the isolated coding agent from an interactive terminal."},
		), fault.PhasePrecondition, fault.ChangeNone))
	}
	view, err := c.finalContexts.ResolveCurrentOrOverride(ctx, inputs.One("--context"))
	if err != nil {
		return c.fail(ctx, err)
	}
	contextRef := view.ContextRef
	seed, err := tobari.NewPolicyAssistConfiguratorSeed(view.Snapshot)
	if err != nil {
		return c.fail(ctx, err)
	}
	intent.Target = operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: contextRef}
	intent.Impact = command.Agent.Mutation.Impact
	settlement := configuratorTaskSettlementFactory(ctx, c)
	recovery, retained, err := c.configurator.ReconcileTask(ctx, seed)
	if err != nil {
		return c.fail(ctx, classifyAssistRecoveryError(seed.Task, err))
	}
	if retained {
		if err := validateExplicitAssistRecoveryAgent(recovery, inputs.One("--agent")); err != nil {
			return c.fail(ctx, classifyAssistRecoveryError(seed.Task, err))
		}
	}
	if retained && recovery.PendingPlanRef != "" {
		applyIntent := operation.Intent{Command: "template apply", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.WorkspaceTemplateChangePlanReferenceKind, ID: recovery.PendingPlanRef}, Impact: workspaceauthoritycmd.TemplateApplyImpact()}
		result, applyErr := c.finalTemplates.Apply(ctx, applyIntent, recovery.PendingPlanRef)
		if applyErr != nil {
			return c.fail(ctx, classifyAssistRecoveryError(seed.Task, applyErr))
		}
		if completeErr := c.configurator.CompleteTask(ctx, intent, recovery.Submission, settlement); completeErr != nil {
			return c.fail(ctx, completeErr)
		}
		return c.emitMutationResult(ctx, command, renderPolicyAssistSuccess(result.Changed, recovery.Submission, humanStyleAllowed(ctx, c, c.Out)))
	}
	if retained && recovery.Published {
		if err := c.configurator.CompleteTask(ctx, intent, recovery.Submission, settlement); err != nil {
			return c.fail(ctx, err)
		}
		changed := recovery.Submission.SourceRevision != recovery.Submission.Draft.BaseTemplateRevision
		return c.emitMutationResult(ctx, command, renderPolicyAssistSuccess(changed, recovery.Submission, humanStyleAllowed(ctx, c, c.Out)))
	}
	if retained {
		if err := validateAssistRecovery(seed, recovery, inputs.One("--agent")); err != nil {
			return c.fail(ctx, classifyAssistRecoveryError(seed.Task, err))
		}
	}

	pending, present, err := c.configurator.PendingStage(ctx, view.Snapshot.Template.ID)
	if err != nil {
		return c.fail(ctx, classifyAssistRecoveryError(seed.Task, err))
	}
	var submission tobari.ConfiguratorSubmission
	if present {
		if err := validatePendingPolicyAssist(seed, pending); err != nil {
			return c.fail(ctx, classifyAssistRecoveryError(seed.Task, err))
		}
		submission = pending.Submission
	} else {
		var agent tobari.ConfiguratorAgent
		frozen := false
		if retained {
			agent, submission, frozen = recovery.Draft.Agent, recovery.Submission, recovery.Frozen
		} else {
			var chooseErr error
			agent, chooseErr = chooseAssistAgent(ctx, c, seed, inputs.One("--agent"))
			if chooseErr != nil {
				return c.fail(ctx, chooseErr)
			}
		}
		for {
			if !frozen {
				if err := writeConfiguratorBoundary(c.Err, agent, seed, humanStyleAllowed(ctx, c, c.Err)); err != nil {
					return c.fail(ctx, configuratorBoundaryOutputFaultFor("policy assist", err))
				}
				if err := c.finalEntryReadiness.Check(ctx); err != nil {
					return c.fail(ctx, err)
				}
				submission, err = c.configurator.Author(ctx, intent, seed, agent, c.In, c.Err, settlement)
				if err != nil {
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, tobari.ErrNativeLoginBridgeUnavailable) {
						err = configuratorcmd.MaterialRetainedFault(seed.Task, err)
					}
					return c.fail(ctx, err)
				}
				writeConfiguratorReturn(c.Err, agent, seed, humanStyleAllowed(ctx, c, c.Err))
				frozen = true
			}
			reviewer := c.configuratorReview
			if reviewer == nil {
				reviewer = newConfiguratorSubmissionReviewerWithStyle(!c.noColor)
			}
			action, reviewErr := reviewer.Review(ctx, seed, submission, c.In, c.Err)
			if reviewErr != nil {
				return c.fail(ctx, reviewErr)
			}
			if action == configuratorSubmissionEdit {
				frozen = false
				continue
			}
			if action != configuratorSubmissionApply {
				return c.fail(ctx, configuratorcmd.MaterialRetainedFault(seed.Task, context.Canceled))
			}
			break
		}
	}
	return applyPolicyAssistSubmission(ctx, c, command, intent, submission, pending, present)
}

func validatePendingPolicyAssist(seed tobari.ConfiguratorSeed, pending tobari.ConfiguratorPendingStage) error {
	if pending.Validate() != nil || pending.Submission.Draft.Task != tobari.ConfiguratorTaskPolicy || pending.Submission.Draft.Agent.Validate() != nil {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	expected, err := tobari.NewConfiguratorDraft(seed, pending.Submission.Draft.Agent, pending.Submission.Draft.TemplateID)
	if err != nil || expected != pending.Submission.Draft {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	return nil
}

func applyPolicyAssistSubmission(
	ctx context.Context,
	c *CLI,
	command CommandSpec,
	intent operation.Intent,
	submission tobari.ConfiguratorSubmission,
	pending tobari.ConfiguratorPendingStage,
	resuming bool,
) int {
	stage := pending.Stage
	if !resuming {
		var err error
		stage, err = c.configurator.Stage(ctx, intent, submission)
		if err != nil {
			return c.fail(ctx, classifyAssistRecoveryError(submission.Draft.Task, err))
		}
		pending = tobari.ConfiguratorPendingStage{Submission: submission, Stage: stage}
	}
	plan, err := c.finalTemplates.Plan(ctx, stage.TemplateRef)
	if err != nil {
		if !resuming {
			err = errors.Join(discardConfiguratorStageAfterReview(ctx, c, intent, submission, stage), err)
		}
		return c.fail(ctx, classifyAssistRecoveryError(submission.Draft.Task, err))
	}
	if plan.SourceRevision != stage.SourceRevision || plan.SourceFingerprint != stage.SourceFingerprint {
		return c.fail(ctx, classifyAssistRecoveryError(submission.Draft.Task, tobari.ErrResourceSourceRecoveryRequired))
	}
	if pending.PlanRef != "" && pending.PlanRef != plan.PlanRef {
		return c.fail(ctx, classifyAssistRecoveryError(submission.Draft.Task, tobari.ErrResourceSourceRecoveryRequired))
	}
	if pending.PlanRef == "" {
		pending, err = c.configurator.BindStagePlan(ctx, intent, pending, plan.PlanRef)
		if err != nil {
			return c.fail(ctx, classifyAssistRecoveryError(submission.Draft.Task, err))
		}
	}
	if !pending.ApplyConfirmed {
		reviewer := c.configuratorPlanReview
		if reviewer == nil {
			reviewer = newConfiguratorPlanReviewerWithStyle(!c.noColor)
		}
		confirmed, reviewErr := reviewer.Review(ctx, submission, plan, c.In, c.Err)
		if reviewErr != nil || !confirmed {
			discardErr := discardConfiguratorStageAfterReview(ctx, c, intent, submission, stage)
			if reviewErr != nil {
				return c.fail(ctx, errors.Join(discardErr, reviewErr))
			}
			if discardErr != nil {
				return c.fail(ctx, discardErr)
			}
			return c.fail(ctx, fault.New(fault.KindCanceled, "configuration_canceled", "Policy assistance was canceled before Apply.", false))
		}
		pending, err = c.configurator.ConfirmStageApply(ctx, intent, pending)
		if err != nil {
			return c.fail(ctx, classifyAssistRecoveryError(submission.Draft.Task, err))
		}
	}
	if err := c.configurator.ConfirmTask(ctx, intent, submission); err != nil {
		return c.fail(ctx, classifyAssistRecoveryError(submission.Draft.Task, err))
	}
	applyIntent := operation.Intent{Command: "template apply", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.WorkspaceTemplateChangePlanReferenceKind, ID: plan.PlanRef}, Impact: workspaceauthoritycmd.TemplateApplyImpact()}
	result, err := c.finalTemplates.Apply(ctx, applyIntent, plan.PlanRef)
	if err != nil {
		return c.fail(ctx, classifyAssistRecoveryError(submission.Draft.Task, err))
	}
	err = c.configurator.CompleteTask(ctx, intent, submission, configuratorTaskSettlementFactory(ctx, c))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderPolicyAssistSuccess(result.Changed, submission, humanStyleAllowed(ctx, c, c.Out)))
}

func configuratorTaskSettlementFactory(ctx context.Context, c *CLI) configuratorcmd.SettlementContextFactory {
	return func() (context.Context, context.CancelFunc) {
		base := ctx
		if c != nil && c.processLifetime != nil {
			base = c.processLifetime
		}
		return context.WithTimeout(base, firstEntryClassificationTimeout)
	}
}

func validateExplicitAssistRecoveryAgent(recovery configuratorcmd.TaskRecovery, value string) error {
	if value == "" {
		return nil
	}
	agent, err := tobari.ParseConfiguratorAgent(value)
	if err != nil || recovery.Draft.Agent != agent {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	return nil
}

func classifyAssistRecoveryError(task tobari.ConfiguratorTask, err error) error {
	if _, structured := fault.PublicCopy(err); structured {
		return err
	}
	command := "help policy assist"
	if task == tobari.ConfiguratorTaskRuntime {
		command = "help runtime assist"
	}
	if errors.Is(err, tobari.ErrContextBindingProtected) {
		return fault.WithClassification(fault.Wrap(
			fault.KindUnavailable,
			"configuration_task_busy",
			"The exact assistance task is temporarily protected by another Workspace or configuration attachment.",
			true,
			err,
			fault.NextAction{Command: command, Reason: "Retry the same exact assistance task after the current attachment finishes."},
		), fault.PhasePrecondition, fault.ChangeNone)
	}
	if errors.Is(err, tobari.ErrFinalAuthorityMutationRecoveryRequired) {
		return fault.WithClassification(fault.Wrap(
			fault.KindUnavailable,
			"final_authority_mutation_recovery_required",
			"A preserved final-authority mutation must be recovered through its exact initiating command before assistance can continue.",
			false,
			err,
			fault.NextAction{Command: "help template apply", Reason: "Recover the preserved exact Template Apply before resuming assistance."},
		), fault.PhasePrecondition, fault.ChangeNone)
	}
	if errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) || errors.Is(err, tobari.ErrWorkspaceTemplateChangePlanStale) {
		return fault.WithClassification(fault.Wrap(
			fault.KindUnavailable,
			"configuration_task_recovery_required",
			"The retained assistance task no longer matches its exact source or authority receipt.",
			false,
			err,
			fault.NextAction{Command: command, Reason: "Reconcile the retained exact task before starting another assistance session."},
		), fault.PhasePrecondition, fault.ChangeNone)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fault.WithClassification(fault.Wrap(
		fault.KindUnavailable,
		"configuration_task_observation_failed",
		"The retained assistance task could not be observed exactly.",
		false,
		err,
		fault.NextAction{Command: command, Reason: "Inspect the retained exact task through the same assistance command before retrying."},
	), fault.PhaseObservation, fault.ChangeNotApplicable)
}

func validateAssistRecovery(seed tobari.ConfiguratorSeed, recovery configuratorcmd.TaskRecovery, requestedAgent string) error {
	if recovery.Draft.Validate() != nil || recovery.Draft.Task != seed.Task || recovery.Draft.ProjectRoot != seed.ProjectRoot || recovery.Draft.TargetRuntimeID != seed.TargetRuntimeID {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	var adoptionContextIDs []tobari.ContextID
	if recovery.Draft.AdoptionContextID != "" {
		adoptionContextIDs = append(adoptionContextIDs, recovery.Draft.AdoptionContextID)
	}
	expected, err := tobari.NewConfiguratorDraft(seed, recovery.Draft.Agent, recovery.Draft.TemplateID, adoptionContextIDs...)
	if err != nil || expected != recovery.Draft {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	if requestedAgent != "" {
		agent, err := tobari.ParseConfiguratorAgent(requestedAgent)
		if err != nil || agent != recovery.Draft.Agent {
			return tobari.ErrResourceSourceRecoveryRequired
		}
	}
	if recovery.Frozen && (recovery.Submission.Validate() != nil || recovery.Submission.Draft != recovery.Draft) {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	if recovery.ApplyConfirmed && !recovery.Frozen {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	return nil
}

func chooseAssistAgent(ctx context.Context, c *CLI, seed tobari.ConfiguratorSeed, value string) (tobari.ConfiguratorAgent, error) {
	if value != "" {
		agent, err := tobari.ParseConfiguratorAgent(value)
		if err != nil {
			return "", err
		}
		open, err := confirmConfiguratorHandoff(ctx, newContextConfigurationWizardWithStyle(!c.noColor), seed, agent, c.In, c.Err)
		if err != nil {
			return "", err
		}
		if !open {
			return "", context.Canceled
		}
		return agent, nil
	}
	selector := c.firstUseSetup
	if selector == nil {
		selector = newFirstUseSetupSelectorWithStyle(!c.noColor)
	}
	choice, err := selector.Choose(ctx, seed, c.In, c.Err)
	if err != nil {
		return "", err
	}
	agent, ok := setupChoiceAgent(choice)
	if !ok {
		return "", fmt.Errorf("assist selector returned no coding agent")
	}
	return agent, nil
}

func configuratorBoundaryOutputFaultFor(command string, err error) error {
	return fault.WithClassification(fault.Wrap(
		fault.KindInternal, "configurator_boundary_output_failed", "The isolated Configurator boundary could not be written before Runtime preparation.", false, err,
		fault.NextAction{Command: "help " + command, Reason: "Retry with a writable interactive terminal before preparing Runtime material."},
	), fault.PhasePrecondition, fault.ChangeNone)
}

func renderRuntimeAssistSuccess(manifest tobari.RuntimeManifest, submission tobari.ConfiguratorSubmission, color bool) []byte {
	output := newHumanOutput(color)
	output.section("Tobari · Runtime source reviewed")
	output.row("Runtime", safeExternalText(manifest.Name), styleText)
	if submission.RuntimeSource != nil {
		state := "unchanged · " + string(submission.RuntimeSource.FrozenRevision)
		if submission.RuntimeSource.Changed {
			state = string(submission.RuntimeSource.BaseRevision) + " → " + string(submission.RuntimeSource.FrozenRevision)
		}
		output.row("Source", state, changedStyle(submission.RuntimeSource.Changed))
	}
	output.row("Published", "editable source only · no Runtime revision built", styleSuccess)
	output.next("runtime build --id "+manifest.RuntimeRef, "Build this exact current source as a separately confirmed action.")
	output.row("Edit manually", safeExternalText(manifest.SourcePath), styleMuted)
	return output.bytes()
}

func renderPolicyAssistSuccess(changed bool, submission tobari.ConfiguratorSubmission, color bool) []byte {
	output := newHumanOutput(color)
	output.section("Tobari · Static policy reviewed")
	output.row("Template", safeExternalText(string(submission.Draft.TemplateID)), styleText)
	state := "already current"
	if changed {
		state = "published from the exact reviewed Plan"
	}
	output.row("Static policy", state, changedStyle(changed))
	output.row("Policy Memory", "unchanged · remains Context-owned", styleMuted)
	output.next("tobari", "Reconcile protection and enter the Workspace with the updated Template policy.")
	return output.bytes()
}
