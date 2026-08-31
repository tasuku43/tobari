package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

const ExitInterrupted = 130

const workspaceCleanupAttention = "Tobari cleanup needs attention; Next: tobari status."

const firstEntryClassificationTimeout = 5 * time.Second

func runFinalDefaultPairEnter(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalDefaultPair == nil || c.finalEntryReadiness == nil || c.finalCluster == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	session := tobari.NewWorkspaceShellSession()
	if inputs.Provided("command") {
		var err error
		session, err = tobari.NewWorkspaceDirectSession(inputs.Values("command"))
		if err != nil {
			return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help tobari", "Supply exact child argv after --.")
		}
	}

	selected, err := c.finalDefaultPair.Select(ctx, c.In, c.Err)
	if err != nil {
		return c.failRootBeforeHandoff(ctx, err)
	}
	fresh := !selected.Selection.CollectionPresent
	var customized *tobari.WorkspaceTemplateBody
	var preparedFirstUse bool
	if fresh {
		if c.interactive == nil || !c.interactive(c.In, c.Out, c.Err) {
			return c.failRootBeforeHandoff(ctx, fault.WithClassification(fault.New(
				fault.KindRejected, "first_use_interactive_required",
				"Fresh first use requires one interactive review before Tobari creates authority or uses Docker.", false,
				fault.NextAction{Command: "help tobari", Reason: "Open an interactive terminal in the Project, then run tobari."},
			), fault.PhasePrecondition, fault.ChangeNone))
		}
		prepareErr := c.finalEntryReadiness.Check(ctx)
		if prepareErr != nil {
			return c.failRootBeforeHandoff(ctx, prepareErr)
		}
		preparedFirstUse = true
		draft, draftErr := tobari.NewRecommendedFirstUseDraft(selected.Selection.CanonicalCWD, session)
		if draftErr != nil {
			return c.failRootBeforeHandoff(ctx, fault.WithClassification(fault.Wrap(
				fault.KindContract, "invalid_first_use_draft", "The recommended first-use draft is invalid.", false, draftErr,
				fault.NextAction{Command: "help tobari", Reason: "Inspect the root first-use contract."},
			), fault.PhasePrecondition, fault.ChangeNone))
		}
		reviewer := c.firstUse
		if reviewer == nil {
			reviewer = newRecommendedFirstUseReviewerWithStyle(!c.noColor)
		}
		action, reviewErr := reviewer.Review(ctx, draft, c.In, c.Err)
		if reviewErr != nil {
			return c.failRootBeforeHandoff(ctx, normalizeFirstUseReviewError(reviewErr))
		}
		switch action {
		case recommendedFirstUseStart:
		case recommendedFirstUseCustomize:
			body, customizeErr := c.firstUseCustomizedTemplateBody(ctx, draft)
			if customizeErr != nil {
				return c.failRootBeforeHandoff(ctx, normalizeFirstUseReviewError(customizeErr))
			}
			customized = &body
		case recommendedFirstUseCancel:
			return c.failRootBeforeHandoff(ctx, context.Canceled)
		default:
			return c.failRootBeforeHandoff(ctx, fault.WithClassification(fault.New(
				fault.KindContract, "invalid_first_use_action", "The first-use review returned an invalid action.", false,
				fault.NextAction{Command: "help tobari", Reason: "Inspect the root first-use contract."},
			), fault.PhasePrecondition, fault.ChangeNone))
		}
	}

	intent.Target = operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	if !fresh && selectedDefaultPairHasWorkspace(selected) {
		resolution, resolveErr := c.finalDefaultPair.ResolveSelected(ctx, intent, nil, selected)
		if resolveErr != nil {
			return c.failRootBeforeHandoff(ctx, resolveErr)
		}
		if err := rootCancellationAfterResolution(ctx, resolution); err != nil {
			return c.failRootBeforeHandoff(ctx, err)
		}
		if currentWorkspaceAuthorityReady(resolution) {
			result, entryErr := c.finalDefaultPair.EnterResolvedCurrent(ctx, resolution, session, c.In, c.Out, c.Err)
			if entryErr == nil {
				emitWorkspaceCleanupAttention(c.Err, result.Outcome)
				return result.Outcome.ExitCode
			}
			if !errors.Is(entryErr, tobari.ErrWorkspaceEntryRuntimeNotCurrent) && !errors.Is(entryErr, tobari.ErrWorkspaceEntryProtectionNotCurrent) {
				return c.failRootBeforeHandoff(ctx, entryErr)
			}
		}
	}

	progress := newFirstEntryProgress(
		c.Err, !fresh, terminal.IsTerminal(c.Err), humanStyleAllowed(ctx, c, c.Err),
	)
	if preparedFirstUse {
		// Fresh readiness is confirmed before the interactive review so no
		// Docker mutation can precede that gate. After review, project the
		// already-confirmed checkpoint without repeating the observation.
		if err := progress.Start(tobari.FirstEntryCheckRequirements); err != nil {
			return c.failRootBeforeHandoff(ctx, err)
		}
		if err := progress.Finish(tobari.FirstEntryStageSucceeded); err != nil {
			return c.failRootBeforeHandoff(ctx, err)
		}
	} else {
		if err := progress.Start(tobari.FirstEntryCheckRequirements); err != nil {
			return c.failRootBeforeHandoff(ctx, err)
		}
		if err := c.finalEntryReadiness.Check(ctx); err != nil {
			_ = progress.Finish(firstEntryFailureState(err))
			return c.failRootBeforeHandoff(ctx, err)
		}
		if err := progress.Finish(tobari.FirstEntryStageSucceeded); err != nil {
			return c.failRootBeforeHandoff(ctx, err)
		}
	}

	if err := progress.Start(tobari.FirstEntryResolveContext); err != nil {
		return c.failRootBeforeHandoff(ctx, err)
	}
	var freshBody *tobari.WorkspaceTemplateBody
	if fresh {
		if customized != nil {
			body := customized.Clone()
			freshBody = &body
		} else {
			body, bodyErr := c.firstUseStandardTemplateBody(ctx)
			if bodyErr != nil {
				_ = progress.Finish(firstEntryFailureState(bodyErr))
				return c.failRootBeforeHandoff(ctx, bodyErr)
			}
			freshBody = &body
		}
	}
	var resolution workspaceauthoritycmd.DefaultPairResolution
	resolution, err = c.finalDefaultPair.ResolveSelected(ctx, intent, freshBody, selected)
	if err != nil {
		_ = progress.Finish(firstEntryFailureState(err))
		return c.failRootBeforeHandoff(ctx, err)
	}
	if err := rootCancellationAfterResolution(ctx, resolution); err != nil {
		_ = progress.Finish(firstEntryFailureState(err))
		return c.failRootBeforeHandoff(ctx, err)
	}
	if err := progress.Finish(tobari.FirstEntryStageSucceeded); err != nil {
		return c.failRootBeforeHandoff(ctx, err)
	}

	resumeEntry, recoveryErr := finalDefaultPairCanResumeEntry(ctx, c.finalDefaultPair, resolution)
	if recoveryErr != nil {
		_ = progress.Finish(firstEntryFailureState(recoveryErr))
		return c.failRootBeforeHandoff(ctx, recoveryErr)
	}
	if err := progress.Start(tobari.FirstEntryPrepareProtection); err != nil {
		return c.failRootBeforeHandoff(ctx, err)
	}
	if resumeEntry {
		// The active decision was created only after the prior invocation
		// confirmed this exact Context's protection. Re-enter it directly so a
		// pending Workspace decision can reconcile the missing runtime instead
		// of being blocked by a second, unrelated cluster mutation.
	} else {
		clusterIntent, clusterErr := c.firstEntryClusterIntent()
		var clusterResult workspaceauthoritycmd.FinalClusterReconciliation
		if clusterErr == nil {
			clusterResult, clusterErr = c.finalCluster.Reconcile(ctx, clusterIntent)
		}
		if clusterErr != nil {
			_ = progress.Finish(firstEntryFailureState(clusterErr))
			return c.failRootBeforeHandoff(ctx, clusterErr)
		}
		classificationContext, cancelClassification := firstEntryClassificationContext(c.processLifetime)
		resolution, err = c.finalDefaultPair.RefreshAfterCluster(classificationContext, resolution, clusterResult)
		cancelClassification()
		if err != nil {
			_ = progress.Finish(firstEntryFailureState(err))
			return c.failRootBeforeHandoff(ctx, err)
		}
	}
	if ctx.Err() != nil {
		err = rootConfirmedCheckpointInterrupted(ctx.Err())
		_ = progress.Finish(firstEntryFailureState(err))
		return c.failRootBeforeHandoff(ctx, err)
	}
	if err := progress.Finish(tobari.FirstEntryStageSucceeded); err != nil {
		return c.failRootBeforeHandoff(ctx, err)
	}

	if err := progress.Start(tobari.FirstEntryPrepareWorkspace); err != nil {
		return c.failRootBeforeHandoff(ctx, err)
	}
	freshWorkspace := resolution.Observation.Context != nil && resolution.Observation.Context.Workspace == nil
	progressSink := tobari.FirstEntryProgressSink(func(event tobari.FirstEntryProgress) {
		_ = progress.Apply(event)
		if freshWorkspace && !session.Direct() && event.Stage == tobari.FirstEntryPrepareWorkspace && event.State == tobari.FirstEntryStageSucceeded {
			_, _ = fmt.Fprintln(c.Err, "Credentials stay in this Workspace; sign in with the tool when needed.")
		}
	})
	result, err := c.finalDefaultPair.EnterResolved(ctx, resolution, session, progressSink, c.In, c.Out, c.Err)
	if err != nil {
		_ = progress.Finish(firstEntryFailureState(err))
		return c.failRootBeforeHandoff(ctx, err)
	}
	emitWorkspaceCleanupAttention(c.Err, result.Outcome)
	return result.Outcome.ExitCode
}

func selectedDefaultPairHasWorkspace(selected workspaceauthoritycmd.SelectedDefaultPair) bool {
	observation, err := selected.Selection.Observation(selected.Choice)
	return err == nil && observation.Context != nil && observation.Context.Workspace != nil
}

func currentWorkspaceAuthorityReady(resolution workspaceauthoritycmd.DefaultPairResolution) bool {
	if resolution.Validate() != nil || resolution.AuthorityChanged || resolution.Observation.DefaultTemplate == nil || resolution.Observation.Context == nil || resolution.Observation.Context.Workspace == nil {
		return false
	}
	snapshot := resolution.Observation.Context
	workspace := snapshot.Workspace
	applied := workspace.LastSuccessfulEntry
	if snapshot.ActiveTemplatePolicy == nil || snapshot.ActivePolicyMemory == nil || snapshot.ActivePolicyMemoryRef == nil || applied == nil ||
		snapshot.ActiveTemplatePolicy.ValidateFor(snapshot.Context, snapshot.Template.Current) != nil ||
		snapshot.ActivePolicyMemory.Revision != snapshot.PolicyMemory.Revision || snapshot.ActivePolicyMemoryRef.ValidateFor(snapshot.Context, snapshot.PolicyMemory) != nil {
		return false
	}
	current := snapshot.Template.Current
	return applied.ContextID == snapshot.Context.ID && applied.TemplateID == snapshot.Template.ID && applied.TemplateRevision == current.Revision &&
		applied.EntrySliceDigest == current.Slices.EntrySliceDigest && applied.RuntimeID == current.Slices.RuntimeID && applied.RuntimeRevision == current.Slices.RuntimeRevision
}

type finalDefaultPairMutationRecoveryObserver interface {
	ObserveMutationRecovery(context.Context) (tobari.FinalAuthorityMutationObservation, error)
}

func finalDefaultPairCanResumeEntry(ctx context.Context, pair finalDefaultPairEntry, resolution workspaceauthoritycmd.DefaultPairResolution) (bool, error) {
	observer, ok := pair.(finalDefaultPairMutationRecoveryObserver)
	if !ok {
		return false, nil
	}
	recovery, err := observer.ObserveMutationRecovery(ctx)
	if err != nil {
		return false, err
	}
	if !recovery.ActiveDecision || recovery.Operation != "context-entry" || resolution.Observation.Context == nil {
		return false, nil
	}
	contextRef, err := tobari.ContextRef(resolution.Observation.Context.Context.ID)
	if err != nil {
		return false, err
	}
	return recovery.Target == contextRef, nil
}

func emitWorkspaceCleanupAttention(out io.Writer, outcome tobari.WorkspaceSessionOutcome) {
	if out == nil || len(outcome.CleanupIssues) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, workspaceCleanupAttention)
}

func firstEntryClassificationContext(processLifetime context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(processLifetime, firstEntryClassificationTimeout)
}

func rootCancellationAfterResolution(ctx context.Context, resolution workspaceauthoritycmd.DefaultPairResolution) error {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	if resolution.AuthorityChanged {
		return rootConfirmedCheckpointInterrupted(ctx.Err())
	}
	return ctx.Err()
}

func rootConfirmedCheckpointInterrupted(err error) error {
	return fault.WithClassification(fault.Wrap(
		fault.KindUnavailable, "default_pair_initialized",
		"Tobari confirmed setup authority before interruption; Workspace entry did not complete.", false, err,
		fault.NextAction{Command: "status", Reason: "Observe retained setup authority before entering again."},
	), fault.PhaseMutation, fault.ChangePartial)
}

func (c *CLI) firstUseStandardTemplateBody(ctx context.Context) (tobari.WorkspaceTemplateBody, error) {
	if c.firstUseTemplateBody != nil {
		return c.firstUseTemplateBody(ctx)
	}
	return c.reviewedStandardTemplateBody(ctx)
}

func (c *CLI) firstUseCustomizedTemplateBody(ctx context.Context, draft tobari.RecommendedFirstUseDraft) (tobari.WorkspaceTemplateBody, error) {
	if c.firstUseCustomize != nil {
		return c.firstUseCustomize(ctx, draft)
	}
	return c.customizeRecommendedTemplateBody(ctx, draft)
}

func (c *CLI) firstEntryClusterIntent() (operation.Intent, error) {
	command, found := c.catalog.lookupRegistered("cluster up")
	if !found || command.Agent.Mutation == nil {
		return operation.Intent{}, fault.WithClassification(fault.New(
			fault.KindContract, "invalid_catalog", "The canonical cluster activation contract is missing.", false,
			fault.NextAction{Command: "help cluster up", Reason: "Repair the Catalog-owned cluster activation contract."},
		), fault.PhasePrecondition, fault.ChangeNone)
	}
	return operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ParentID: tobari.ClusterTargetID},
		Impact: command.Agent.Mutation.Impact,
	}, nil
}

func (c *CLI) failRootBeforeHandoff(ctx context.Context, err error) int {
	code := c.fail(ctx, err)
	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		return ExitInterrupted
	}
	return code
}

func normalizeFirstUseReviewError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, ok := fault.PublicCopy(err); ok {
		return err
	}
	return fault.WithClassification(fault.Wrap(
		fault.KindInternal, "first_use_review_failed", "The recommended first-use review failed before creating authority or using Docker.", false, err,
		fault.NextAction{Command: "help tobari", Reason: "Open an interactive terminal and retry the reviewed first-use flow."},
	), fault.PhasePrecondition, fault.ChangeNone)
}

func firstEntryFailureState(err error) tobari.FirstEntryStageState {
	public, ok := fault.PublicCopy(err)
	if !ok {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return tobari.FirstEntryStageBlocked
		}
		return tobari.FirstEntryStageFailed
	}
	if public.ChangeState == fault.ChangeUnknown {
		return tobari.FirstEntryStageUnknown
	}
	switch public.Kind {
	case fault.KindUnavailable, fault.KindRejected, fault.KindCanceled, fault.KindUnsupported:
		return tobari.FirstEntryStageBlocked
	default:
		return tobari.FirstEntryStageFailed
	}
}

func runFinalDefaultPairStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.statusHome == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	status, err := c.statusHome.Snapshot(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	if format == successFormatJSON {
		encoded, err := marshalCommandJSON(command.Path, map[string]any{"schema_version": tobari.StatusHomeSchemaVersion, "status": status})
		if err != nil {
			return c.fail(ctx, err)
		}
		return c.emitResult(ctx, append(encoded, '\n'))
	}
	return c.emitResult(ctx, renderStatusHomeWithColor(status, humanStyleAllowed(ctx, c, c.Out)))
}

func renderStatusHome(status tobari.StatusHomeSnapshot) []byte {
	return renderStatusHomeWithColor(status, false)
}

func renderStatusHomeWithColor(status tobari.StatusHomeSnapshot, color bool) []byte {
	output := newHumanOutput(color)
	output.section("Tobari · Project Status")
	output.row("Project", safeExternalText(status.ProjectRoot), styleText)
	if status.Template == nil {
		output.row("Template", "no default Template", styleWarning)
		output.row("Current", "no Context or Workspace", styleWarning)
	} else {
		output.row("Template", fmt.Sprintf("%s · generation %d", safeExternalText(status.Template.Name), status.Template.Generation), styleText)
		if status.Context == nil {
			output.row("Current", "Context absent", styleWarning)
		} else {
			output.row("Current", "Context selected · Workspace "+string(status.Workspace.Presence), humanStatusToken(string(status.Workspace.Presence)))
			output.row("Policy", "Template "+string(status.Context.TemplatePolicyActivation)+" · Memory "+string(status.Context.PolicyMemoryActivation), humanStatusToken(string(status.Context.PolicyMemoryActivation)))
			output.row("Workspace", string(status.Workspace.Presence)+" · entry "+string(status.Workspace.EntryState)+" · runtime "+string(status.Workspace.ObservedRuntimeState)+" · "+string(status.Workspace.AttachmentState), humanStatusToken(string(status.Workspace.EntryState)))
			output.row("Runtime", string(status.Runtime.Authority)+" · "+string(status.Runtime.Availability)+" · native "+string(status.Runtime.Compatibility), humanStatusToken(string(status.Runtime.Availability)))
			output.row("Cluster", string(status.Cluster.Runtime)+" · receipt "+string(status.Cluster.Receipt), humanStatusToken(string(status.Cluster.Runtime)))
			reviewToken := styleMuted
			if status.Permissions.PendingCount > 0 || status.Services.PendingCount > 0 {
				reviewToken = styleWarning
			}
			output.row("Review", fmt.Sprintf("%d permissions · %d services pending · %d active", status.Permissions.PendingCount, status.Services.PendingCount, status.Services.ActiveCount), reviewToken)
		}
	}
	if len(status.Siblings) > 0 {
		output.row("Other", fmt.Sprintf("%d same-root Contexts", len(status.Siblings)), styleWarning)
	}
	if status.Next.Path != nil {
		output.next(*status.Next.Path, status.Next.Reason)
	} else if status.Next.Guidance != nil {
		output.row("Next", safeExternalText(*status.Next.Guidance)+" — "+safeExternalText(status.Next.Reason), styleAccent)
	}
	return output.bytes()
}
