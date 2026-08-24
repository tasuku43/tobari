package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
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

	observed, err := c.finalDefaultPair.Observe(ctx)
	if err != nil {
		return c.failRootBeforeHandoff(ctx, err)
	}
	fresh := !observed.CollectionPresent
	var customized *tobari.WorkspaceTemplateBody
	if fresh {
		if c.firstUseInteractive == nil || !c.firstUseInteractive(c.In, c.Out, c.Err) {
			return c.failRootBeforeHandoff(ctx, fault.WithClassification(fault.New(
				fault.KindRejected, "first_use_interactive_required",
				"Fresh first use requires one interactive review before Tobari creates authority or uses Docker.", false,
				fault.NextAction{Command: "help tobari", Reason: "Open an interactive terminal in the Project, then run tobari."},
			), fault.PhasePrecondition, fault.ChangeNone))
		}
		draft, draftErr := tobari.NewRecommendedFirstUseDraft(observed.ProjectRoot, session)
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

	progress := newFirstEntryProgress(c.Err, !fresh)
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
	intent.Target = operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	resolution, err := c.finalDefaultPair.Resolve(ctx, intent, freshBody)
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

	if err := progress.Start(tobari.FirstEntryPrepareProtection); err != nil {
		return c.failRootBeforeHandoff(ctx, err)
	}
	clusterIntent, clusterErr := c.firstEntryClusterIntent()
	var clusterResult workspaceauthoritycmd.FinalClusterReconciliation
	if clusterErr == nil {
		clusterResult, clusterErr = c.finalCluster.Reconcile(ctx, clusterIntent)
	}
	if clusterErr != nil {
		_ = progress.Finish(firstEntryFailureState(clusterErr))
		return c.failRootBeforeHandoff(ctx, clusterErr)
	}
	classificationContext, cancelClassification := firstEntryClassificationContext(ctx)
	resolution, err = c.finalDefaultPair.RefreshAfterCluster(classificationContext, resolution, clusterResult)
	cancelClassification()
	if err != nil {
		_ = progress.Finish(firstEntryFailureState(err))
		return c.failRootBeforeHandoff(ctx, err)
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

func emitWorkspaceCleanupAttention(out io.Writer, outcome tobari.WorkspaceSessionOutcome) {
	if out == nil || len(outcome.CleanupIssues) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, workspaceCleanupAttention)
}

func firstEntryClassificationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), firstEntryClassificationTimeout)
	}
	return context.WithTimeout(context.WithoutCancel(ctx), firstEntryClassificationTimeout)
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
	return c.emitResult(ctx, renderStatusHome(status))
}

func renderStatusHome(status tobari.StatusHomeSnapshot) []byte {
	var text strings.Builder
	fmt.Fprintf(&text, "Project   %s\n", safeExternalText(status.ProjectRoot))
	if status.Template == nil {
		text.WriteString("Template  no default Template\nCurrent   no Context or Workspace\n")
	} else {
		fmt.Fprintf(&text, "Template  %s · generation %d\n", safeExternalText(status.Template.Name), status.Template.Generation)
		if status.Context == nil {
			text.WriteString("Current   Context absent\n")
		} else {
			fmt.Fprintf(&text, "Current   Context selected · Workspace %s\n", status.Workspace.Presence)
			fmt.Fprintf(&text, "Policy    Template %s · Memory %s\n", status.Context.TemplatePolicyActivation, status.Context.PolicyMemoryActivation)
			fmt.Fprintf(&text, "Workspace %s · entry %s · runtime %s · %s\n", status.Workspace.Presence, status.Workspace.EntryState, status.Workspace.ObservedRuntimeState, status.Workspace.AttachmentState)
			fmt.Fprintf(&text, "Runtime   %s · %s · native %s\n", status.Runtime.Authority, status.Runtime.Availability, status.Runtime.Compatibility)
			fmt.Fprintf(&text, "Cluster   %s · receipt %s\n", status.Cluster.Runtime, status.Cluster.Receipt)
			fmt.Fprintf(&text, "Review    %d permissions · %d services pending · %d active\n", status.Permissions.PendingCount, status.Services.PendingCount, status.Services.ActiveCount)
		}
	}
	if len(status.Siblings) > 0 {
		fmt.Fprintf(&text, "Other     %d same-root Contexts\n", len(status.Siblings))
	}
	if status.Next.Path != nil {
		path := *status.Next.Path
		if path == WorkspaceEntryCommandPath {
			fmt.Fprintf(&text, "Next      tobari — %s\n", status.Next.Reason)
		} else {
			fmt.Fprintf(&text, "Next      tobari %s — %s\n", path, status.Next.Reason)
		}
	} else if status.Next.Guidance != nil {
		fmt.Fprintf(&text, "Next      %s — %s\n", *status.Next.Guidance, status.Next.Reason)
	}
	return []byte(text.String())
}
