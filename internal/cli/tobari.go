package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func runClusterUp(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ParentID: tobari.ClusterTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	var progress *clusterUpProgress
	var progressSink tobari.ClusterUpProgressSink
	if c.tobari.IsTerminal(c.Err) && clusterUpProgressAllowed(ctx) {
		progress = newClusterUpProgress(c.Err, true)
		progress.Start()
		progressSink = progress.Report
		defer progress.Close()
	}
	status, err := c.tobari.ClusterUpWithProgress(ctx, intent, progressSink)
	if err != nil {
		if progress != nil {
			progress.Fail()
		}
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderClusterUpText(status, clusterColorAllowed(ctx, c)))
}

func runClusterStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	status, err := c.tobari.ClusterStatus(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help cluster status", "Correct the command arguments.")
	}
	output, err := renderClusterStatus(status, format, clusterColorAllowed(ctx, c))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runClusterLogs(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	output, err := c.tobari.ClusterLogs(ctx, tobari.LogRequest{
		Component: inputs.One("--component"), Tail: int(tail),
	})
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, renderSafeLogs(output))
}

func runClusterDenials(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	result, err := c.tobari.ClusterDenials(ctx, int(tail))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(
			ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(),
			"help cluster denials", "Correct the command arguments.",
		)
	}
	review, found := c.catalog.Lookup("policy review")
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "policy review command is missing", false,
		))
	}
	output, err := renderClusterDenialsWithReviewCommand(
		result, ProgramName+" "+review.Path,
		format, format == successFormatText && humanColorAllowed(ctx, c, c.Out),
	)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runPolicyCandidates(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	return runPolicyCandidateQueue(ctx, c, command, inputs, false)
}

func runPolicyTail(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	return runPolicyCandidateQueue(ctx, c, command, inputs, true)
}

func runPolicyReview(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	format, formatErr := parseSuccessFormat(inputs.One("--format"))
	if formatErr != nil {
		return c.failUsage(
			ctx, "invalid_arguments", formatErr.Error()+"; usage: "+command.Usage(),
			"help "+command.Path, "Correct the command arguments.",
		)
	}
	result, err := c.tobari.PolicyReview(ctx, int(tail))
	if err != nil {
		return c.fail(ctx, err)
	}
	allow, found := c.catalog.Lookup("policy allow")
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "policy allow command is missing", false,
		))
	}
	allowCommand := ProgramName + " " + allow.Path
	deny, found := c.catalog.Lookup("policy deny")
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "policy deny command is missing", false,
		))
	}
	denyCommand := ProgramName + " " + deny.Path
	if format != successFormatText || !policyReviewInteractiveAllowed(ctx, c) {
		output, renderErr := renderPolicyReviewWithCommands(
			result, allowCommand, denyCommand, format,
			format == successFormatText && humanColorAllowed(ctx, c, c.Out),
		)
		if renderErr != nil {
			return c.fail(ctx, renderErr)
		}
		return c.emitResult(ctx, output)
	}

	selector := newPolicyReviewSelector()
	for {
		if len(result.Items) == 0 {
			output, renderErr := renderPolicyReviewWithCommands(result, allowCommand, denyCommand, successFormatText, true)
			if renderErr != nil {
				return c.fail(ctx, renderErr)
			}
			return c.emitResult(ctx, output)
		}
		decision, selectErr := selector.Select(ctx, result, c.In, c.Out)
		if selectErr != nil {
			return c.fail(ctx, selectErr)
		}
		if decision.Canceled {
			return c.emitResult(ctx, renderPolicyReviewCanceled())
		}
		if !policyReviewContainsID(result, decision.CandidateID) {
			return c.fail(ctx, fault.New(
				fault.KindContract, "invalid_policy_candidate_selection",
				"the interactive review selected an ID outside its validated snapshot", false,
				fault.NextAction{Command: "policy candidates", Reason: "Rediscover the current pending queue."},
			))
		}

		actionCommand := allow
		if decision.Action == policyReviewActionDeny {
			deny, denyFound := c.catalog.Lookup("policy deny")
			if !denyFound {
				return c.fail(ctx, fault.New(
					fault.KindContract, "invalid_catalog", "policy deny command is missing", false,
				))
			}
			actionCommand = deny
		}
		actionCtx := withCommandPath(ctx, actionCommand.Path)
		if decision.Action == policyReviewActionDeny {
			change, denyErr := denyPolicyCandidate(actionCtx, c, actionCommand, decision.CandidateID)
			if denyErr != nil {
				return c.fail(actionCtx, denyErr)
			}
			if code := c.emitMutationResult(
				actionCtx, actionCommand, renderPolicyDenyChangeWithColor(change, humanColorAllowed(actionCtx, c, c.Out)),
			); code != ExitOK {
				return code
			}
		} else {
			change, allowErr := allowPolicyCandidate(actionCtx, c, actionCommand, decision.CandidateID)
			if allowErr != nil {
				return c.fail(actionCtx, allowErr)
			}
			if code := c.emitMutationResult(
				actionCtx, actionCommand, renderPolicyReviewAllowSuccess(change, humanColorAllowed(actionCtx, c, c.Out)),
			); code != ExitOK {
				return code
			}
		}

		result, err = c.tobari.PolicyReview(ctx, int(tail))
		if err != nil {
			return c.fail(ctx, err)
		}
	}
}

func runPolicyRules(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, formatErr := parseSuccessFormat(inputs.One("--format"))
	if formatErr != nil {
		return c.failUsage(
			ctx, "invalid_arguments", formatErr.Error()+"; usage: "+command.Usage(),
			"help "+command.Path, "Correct the command arguments.",
		)
	}
	result, err := c.tobari.PolicyRules(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	reset, found := c.catalog.Lookup("policy reset")
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "policy reset command is missing", false,
		))
	}
	resetCommand := ProgramName + " " + reset.Path
	if format != successFormatText || !policyReviewInteractiveAllowed(ctx, c) {
		output, renderErr := renderPolicyRulesWithCommands(
			result, resetCommand, format,
			format == successFormatText && humanColorAllowed(ctx, c, c.Out),
		)
		if renderErr != nil {
			return c.fail(ctx, renderErr)
		}
		return c.emitResult(ctx, output)
	}

	selector := newPolicyRuleSelector()
	for {
		if len(result.Items) == 0 {
			output, renderErr := renderPolicyRulesWithCommands(result, resetCommand, successFormatText, true)
			if renderErr != nil {
				return c.fail(ctx, renderErr)
			}
			return c.emitResult(ctx, output)
		}
		decision, selectErr := selector.Select(ctx, result, c.In, c.Out)
		if selectErr != nil {
			return c.fail(ctx, selectErr)
		}
		if decision.Canceled {
			return c.emitResult(ctx, renderPolicyRulesCanceled())
		}
		if !policyRuleContainsID(result, decision.RuleID) {
			return c.fail(ctx, fault.New(
				fault.KindContract, "invalid_policy_rule_selection",
				"the interactive policy review selected an ID outside its validated snapshot", false,
				fault.NextAction{Command: "policy rules", Reason: "Rediscover the current learned decisions."},
			))
		}
		actionCtx := withCommandPath(ctx, reset.Path)
		change, resetErr := resetPolicyRule(actionCtx, c, reset, decision.RuleID)
		if resetErr != nil {
			return c.fail(actionCtx, resetErr)
		}
		if code := c.emitMutationResult(
			actionCtx, reset, renderPolicyRuleResetWithColor(change, humanColorAllowed(actionCtx, c, c.Out)),
		); code != ExitOK {
			return code
		}
		result, err = c.tobari.PolicyRules(ctx)

		if err != nil {
			return c.fail(ctx, err)
		}
	}
}

func policyReviewInteractiveAllowed(ctx context.Context, c *CLI) bool {
	return invocationErrorFormat(ctx) != errorFormatJSON && c != nil && c.tobari != nil &&
		c.tobari.IsInteractive(c.In, c.Out)
}

func policyReviewContainsID(result tobari.PolicyCandidateReport, id string) bool {
	for _, candidate := range result.Items {
		if candidate.ID == id {
			return true
		}
	}
	return false
}

func policyRuleContainsID(result tobari.PolicyRuleReport, id string) bool {
	for _, rule := range result.Items {
		if rule.ID == id {
			return true
		}
	}
	return false
}

func runPolicyCandidateQueue(
	ctx context.Context, c *CLI, command CommandSpec, inputs ParsedInputs, tailView bool,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	var (
		result tobari.PolicyCandidateReport
		err    error
	)
	if tailView {
		result, err = c.tobari.PolicyTail(ctx, int(tail))
	} else {
		result, err = c.tobari.PolicyCandidates(ctx, int(tail))
	}
	if err != nil {
		return c.fail(ctx, err)
	}
	format := successFormatText
	if !tailView {
		format, err = parseSuccessFormat(inputs.One("--format"))
		if err != nil {
			return c.failUsage(
				ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(),
				"help "+command.Path, "Correct the command arguments.",
			)
		}
	}
	allow, found := c.catalog.Lookup("policy allow")
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "policy allow command is missing", false,
		))
	}
	output, err := renderPolicyCandidatesWithColor(result, ProgramName+" "+allow.Path, format, format == successFormatText && humanColorAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runPolicyAllow(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	id := inputs.One("--id")
	result, err := allowPolicyCandidate(ctx, c, command, id)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderPolicyLearningChangeWithColor(result, humanColorAllowed(ctx, c, c.Out)))
}

func runPolicyDeny(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	id := inputs.One("--id")
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: id},
		Impact: command.Agent.Mutation.Impact,
	}
	result, err := c.tobari.DenyPolicyCandidate(ctx, intent, id)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderPolicyDenyChangeWithColor(result, humanColorAllowed(ctx, c, c.Out)))
}

func allowPolicyCandidate(
	ctx context.Context, c *CLI, command CommandSpec, id string,
) (tobari.PolicyLearningChange, error) {
	if c == nil || c.tobari == nil {
		return tobari.PolicyLearningChange{}, missingRuntimeFault()
	}
	if command.Agent.Mutation == nil {
		return tobari.PolicyLearningChange{}, fault.New(
			fault.KindContract, "invalid_catalog", "policy allow mutation contract is missing", false,
		)
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: id},
		Impact: command.Agent.Mutation.Impact,
	}
	return c.tobari.AllowPolicyCandidate(ctx, intent, id)
}

func denyPolicyCandidate(
	ctx context.Context, c *CLI, command CommandSpec, id string,
) (tobari.PolicyDenyChange, error) {
	if c == nil || c.tobari == nil {
		return tobari.PolicyDenyChange{}, missingRuntimeFault()
	}
	if command.Agent.Mutation == nil {
		return tobari.PolicyDenyChange{}, fault.New(
			fault.KindContract, "invalid_catalog", "policy deny mutation contract is missing", false,
		)
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: id},
		Impact: command.Agent.Mutation.Impact,
	}
	return c.tobari.DenyPolicyCandidate(ctx, intent, id)
}

func runPolicyReset(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	id := inputs.One("--id")
	result, err := resetPolicyRule(ctx, c, command, id)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderPolicyRuleResetWithColor(result, humanColorAllowed(ctx, c, c.Out)))
}

func resetPolicyRule(
	ctx context.Context, c *CLI, command CommandSpec, id string,
) (tobari.PolicyRuleReset, error) {
	if c == nil || c.tobari == nil {
		return tobari.PolicyRuleReset{}, missingRuntimeFault()
	}
	if command.Agent.Mutation == nil {
		return tobari.PolicyRuleReset{}, fault.New(
			fault.KindContract, "invalid_catalog", "policy reset mutation contract is missing", false,
		)
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.PolicyRuleKind, ID: id},
		Impact: command.Agent.Mutation.Impact,
	}
	return c.tobari.ResetPolicyRule(ctx, intent, id)
}

func runPolicyCompactions(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.tobari.PolicyCompactions(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(
			ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(),
			"help "+command.Path, "Correct the command arguments.",
		)
	}
	compact, found := c.catalog.Lookup("policy compact")
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "policy compact command is missing", false,
		))
	}
	output, err := renderPolicyCompactionsWithColor(result, ProgramName+" "+compact.Path, format, format == successFormatText && humanColorAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runPolicyCompact(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	id := inputs.One("--id")
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.PolicyCompactionKind, ID: id},
		Impact: command.Agent.Mutation.Impact,
	}
	result, err := c.tobari.CompactPolicy(ctx, intent, id)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderPolicyLearningChangeWithColor(result, humanColorAllowed(ctx, c, c.Out)))
}

func runClusterDown(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	purge, _ := inputs.Boolean("--purge")
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	status, err := c.tobari.ClusterDown(ctx, intent, purge)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderClusterStatusTextWithColor(status, clusterColorAllowed(ctx, c)))
}

func clusterColorAllowed(ctx context.Context, c *CLI) bool {
	return c != nil && c.tobari != nil && c.tobari.IsTerminal(c.Out) && clusterUpProgressAllowed(ctx)
}

func runProjectEnter(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, _ ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	code, err := c.tobari.EnterProject(ctx, intent, c.In, c.Out, c.Err)
	if err != nil {
		return c.fail(ctx, err)
	}
	// The child interactive process owns stdout. Keep the host-side lifecycle
	// guidance on stderr so shell output from the session remains untouched.
	message := renderProjectSessionClosed()
	if c.context != nil {
		if report, contextErr := c.context.Show(ctx, ""); contextErr == nil &&
			report.Runtime.Status == tobari.ContextRuntimeStatusOfficial {
			message = append(message, '\n')
			message = append(message, runtimeCustomizationHint()...)
			message = append(message, '\n')
		}
	}
	if pending, reviewErr := c.tobari.PolicyReview(ctx, 10_000); reviewErr == nil {
		message = append(message, renderPendingPolicyNotification(pending)...)
	}
	_, _ = writeOnce(c.Err, message)
	return code
}

func renderProjectSessionClosed() []byte {
	return []byte("Workspace session closed.\nWorkspace remains available.\n\nResume: tobari\nRemove: tobari delete\nIf another session is attached: tobari delete --force\n")
}

func renderPendingPolicyNotification(result tobari.PolicyCandidateReport) []byte {
	if len(result.Items) == 0 {
		return nil
	}
	latest := result.Items[len(result.Items)-1]
	var output strings.Builder
	fmt.Fprintf(&output, "\n⚠ %d pending network permission", len(result.Items))
	if len(result.Items) == 1 {
		output.WriteString(" is")
	} else {
		output.WriteString("s are")
	}
	output.WriteString(" waiting for review.\n")
	fmt.Fprintf(
		&output, "Latest: %s:%d %s %s\n",
		safeExternalText(latest.Host), latest.Port, safeExternalText(latest.Method), safeExternalText(latest.Path),
	)
	output.WriteString("Review on the host: tobari policy review\n")
	return []byte(output.String())
}

func runProjectStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.tobari.ProjectStatus(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help status", "Correct the command arguments.")
	}
	output, err := renderProjectStatusWithColor(result, format, format == successFormatText && humanColorAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runProjectDelete(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	force, _ := inputs.Boolean("--force")
	if force {
		preview, err := c.tobari.ProjectStatus(ctx)
		if err != nil {
			return c.fail(ctx, err)
		}
		if preview.Exists && c.Err != nil {
			if humanColorAllowed(ctx, c, c.Err) {
				previewOutput := newHumanOutput(true)
				previewOutput.heading("!", "Delete target", colorTokenWarning)
				previewOutput.row("Root", safeExternalText(preview.Root), colorTokenMuted)
				_, _ = writeOnce(c.Err, previewOutput.bytes())
			} else {
				fmt.Fprintf(
					c.Err,
					"delete_target: root=%s\tid=%s\thome=%s\truntime=%s\n",
					escapeTSVCell(preview.Root), preview.ID, escapeTSVCell(preview.Home), preview.Runtime,
				)
			}
		}
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ID: tobari.CurrentDirectoryTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	result, err := c.tobari.DeleteProject(ctx, intent, force)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderProjectDeleteWithColor(result, humanColorAllowed(ctx, c, c.Out)))
}

func runAttach(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ParentID: tobari.ClusterTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	instance, err := c.tobari.Attach(
		ctx, intent, inputs.One("--name"), inputs.One("--root"),
		inputs.One("--image"),
	)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderAttachResult(instance, humanColorAllowed(ctx, c, c.Out)))
}

func runList(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.tobari.ProjectList(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help list", "Correct the command arguments.")
	}
	output, err := renderProjectListWithColor(result, format, format == successFormatText && humanColorAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runTobariShell(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	code, err := c.tobari.Exec(
		ctx, inputs.One("--id"),
		tobari.ExecRequest{Command: []string{"/bin/bash"}, Interactive: true, TTY: true},
		c.In, c.Out, c.Err,
	)
	if err != nil {
		return c.fail(ctx, err)
	}
	return code
}

func runTobariExec(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	code, err := c.tobari.Exec(
		ctx, inputs.One("--id"),
		tobari.ExecRequest{
			HostCWD: inputs.One("--cwd"), Command: inputs.Values("command"),
			CWDExplicit: inputs.One("--cwd") != "", Interactive: true, TTY: true,
		},
		c.In, c.Out, c.Err,
	)
	if err != nil {
		return c.fail(ctx, err)
	}
	return code
}

func runTobariLogs(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	output, err := c.tobari.TobariLogs(ctx, inputs.One("--id"), int(tail))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, renderSafeLogs(output))
}

func runDetach(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	id := inputs.One("--id")
	purge, _ := inputs.Boolean("--purge")
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.TargetKind, ID: id},
		Impact: command.Agent.Mutation.Impact,
	}
	if err := c.tobari.Detach(ctx, intent, id, purge); err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderDetachedResult(humanColorAllowed(ctx, c, c.Out)))
}

type clusterStatusDocument struct {
	SchemaVersion int                 `json:"schema_version"`
	Cluster       clusterStatusOutput `json:"cluster"`
}

type clusterDenialsDocument struct {
	SchemaVersion int                  `json:"schema_version"`
	Denials       clusterDenialsOutput `json:"denials"`
}

type clusterDenialsOutput struct {
	Policy        string                `json:"policy"`
	WindowLines   int                   `json:"window_lines"`
	Items         []tobari.PolicyDenial `json:"items"`
	ReviewCommand string                `json:"review_command"`
}

type policyCandidateOutput struct {
	ID                string  `json:"id"`
	ObservedAt        string  `json:"observed_at"`
	ProjectID         string  `json:"project_id"`
	Host              string  `json:"host"`
	Port              int     `json:"port"`
	Method            string  `json:"method"`
	Path              string  `json:"path"`
	Reason            string  `json:"reason"`
	StatusCode        int     `json:"status_code"`
	CredentialProfile *string `json:"credential_profile"`
	AllowCommand      string  `json:"allow_command"`
	DenyCommand       string  `json:"deny_command"`
}

type policyCandidatesDocument struct {
	SchemaVersion    int                     `json:"schema_version"`
	PolicyCandidates []policyCandidateOutput `json:"policy_candidates"`
}

type policyReviewDocument struct {
	SchemaVersion int                     `json:"schema_version"`
	PolicyReview  []policyCandidateOutput `json:"policy_review"`
}

type policyRuleOutput struct {
	ID               string   `json:"id"`
	Decision         string   `json:"decision"`
	Match            string   `json:"match"`
	ProjectID        string   `json:"project_id"`
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	Method           string   `json:"method"`
	Path             string   `json:"path"`
	Examples         []string `json:"examples"`
	SourceCandidates []string `json:"source_candidates"`
	ResetCommand     string   `json:"reset_command"`
}

type policyRulesDocument struct {
	SchemaVersion int                `json:"schema_version"`
	PolicyRules   []policyRuleOutput `json:"policy_rules"`
}

func pairedPolicyCommand(command, from, to string) string {
	suffix := "policy " + from
	if strings.HasSuffix(command, suffix) {
		return strings.TrimSuffix(command, suffix) + "policy " + to
	}
	return ProgramName + " policy " + to
}

func renderPolicyCandidates(
	result tobari.PolicyCandidateReport, allowCommand string, format successFormat,
) ([]byte, error) {
	return renderPolicyCandidatesWithColor(result, allowCommand, format, false)
}

func renderPolicyCandidatesWithColor(
	result tobari.PolicyCandidateReport, allowCommand string, format successFormat, color bool,
) ([]byte, error) {
	denyCommand := pairedPolicyCommand(allowCommand, "allow", "deny")
	items := policyCandidateOutputs(result, allowCommand, denyCommand)
	if format == successFormatJSON {
		output, err := json.Marshal(policyCandidatesDocument{
			SchemaVersion: 2, PolicyCandidates: items,
		})
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract, "output_encoding_failed",
				"policy candidates JSON could not be encoded", false, err,
			)
		}
		return append(output, '\n'), nil
	}
	if color && format == successFormatText {
		return renderPolicyCandidatesHuman(result, allowCommand), nil
	}
	var output bytes.Buffer
	for _, item := range result.Items {
		action := allowCommand + " --id " + item.ID
		profile := "none"
		if item.CredentialProfile != nil {
			profile = *item.CredentialProfile
		}
		fmt.Fprintf(
			&output,
			"id=%s\tobserved_at=%s\tproject_id=%s\thost=%s\tport=%d\tmethod=%s\tpath=%s\treason=%s\tstatus_code=%d\tcredential_profile=%s\tallow_command=%s\tdeny_command=%s\n",
			item.ID, escapeTSVCell(item.ObservedAt), item.ProjectID, escapeTSVCell(item.Host),
			item.Port, escapeTSVCell(item.Method), escapeTSVCell(item.Path), escapeTSVCell(item.Reason),
			item.StatusCode, escapeTSVCell(profile), escapeTSVCell(action), escapeTSVCell(denyCommand+" --id "+item.ID),
		)
	}
	return output.Bytes(), nil
}

func policyCandidateOutputs(
	result tobari.PolicyCandidateReport, allowCommand, denyCommand string,
) []policyCandidateOutput {
	items := make([]policyCandidateOutput, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, policyCandidateOutput{
			ID: item.ID, ObservedAt: safeExternalText(item.ObservedAt),
			ProjectID: item.ProjectID,
			Host:      safeExternalText(item.Host), Port: item.Port, Method: safeExternalText(item.Method),
			Path: safeExternalText(item.Path), Reason: safeExternalText(item.Reason),
			StatusCode:        item.StatusCode,
			CredentialProfile: safeOptionalExternalText(item.CredentialProfile),
			AllowCommand:      allowCommand + " --id " + item.ID,
			DenyCommand:       denyCommand + " --id " + item.ID,
		})
	}
	return items
}

func renderPolicyCandidatesHuman(result tobari.PolicyCandidateReport, allowCommand string) []byte {
	if len(result.Items) == 0 {
		output := newHumanOutput(true)
		output.empty("No policy candidates", "No retained denied request is ready for approval.", "cluster denials", "Inspect the recent bounded denial evidence.")
		return output.bytes()
	}
	output := newHumanOutput(true)
	output.heading("✓", fmt.Sprintf("Policy candidates (%d)", len(result.Items)), colorTokenSuccess)
	output.row("Policy", safeExternalText(result.PolicyDirectory), colorTokenMuted)
	output.row("Window", fmt.Sprintf("%d lines", result.WindowLines), colorTokenMuted)
	for index, item := range result.Items {
		output.section(fmt.Sprintf("Candidate %d", index+1))
		request := fmt.Sprintf("%s:%d %s %s", safeExternalText(item.Host), item.Port, safeExternalText(item.Method), safeExternalText(item.Path))
		output.row("Request", request, colorTokenAccent)
		output.row("ID", item.ID, colorTokenAccent)
		output.row("Project", safeExternalText(item.ProjectID), colorTokenMuted)
		output.row("Observed", safeExternalText(item.ObservedAt), colorTokenMuted)
		output.row("Reason", safeExternalText(item.Reason), colorTokenWarning)
		output.row("Status", fmt.Sprintf("%d", item.StatusCode), colorTokenWarning)
		profile := "none"
		if item.CredentialProfile != nil {
			profile = safeExternalText(*item.CredentialProfile)
		}
		output.row("Credential", profile, colorTokenMuted)
		output.row("Allow", allowCommand+" --id "+item.ID, colorTokenAccent)
		output.row("Deny", pairedPolicyCommand(allowCommand, "allow", "deny")+" --id "+item.ID, colorTokenAccent)
	}
	return output.bytes()
}

func renderPolicyReviewWithColor(
	result tobari.PolicyCandidateReport, allowCommand string, color bool,
) []byte {
	output, err := renderPolicyReviewWithCommands(
		result, allowCommand, pairedPolicyCommand(allowCommand, "allow", "deny"), successFormatText, color,
	)
	if err != nil {
		return []byte("policy review: output encoding failed\n")
	}
	return output
}

func renderPolicyReviewWithCommands(
	result tobari.PolicyCandidateReport, allowCommand, denyCommand string,
	format successFormat, color bool,
) ([]byte, error) {
	items := policyCandidateOutputs(result, allowCommand, denyCommand)
	if format == successFormatJSON {
		output, err := json.Marshal(policyReviewDocument{
			SchemaVersion: 2, PolicyReview: items,
		})
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract, "output_encoding_failed",
				"policy review JSON could not be encoded", false, err,
			)
		}
		return append(output, '\n'), nil
	}
	return renderPolicyReviewHuman(result, allowCommand, denyCommand, color), nil
}

func renderPolicyReviewHuman(
	result tobari.PolicyCandidateReport, allowCommand, denyCommand string, color bool,
) []byte {
	if len(result.Items) == 0 {
		output := newHumanOutput(color)
		output.empty(
			"No pending network permissions",
			"No retained exact permission is waiting for host review.",
			"", "",
		)
		return output.bytes()
	}
	output := newHumanOutput(color)
	output.heading("⚠", fmt.Sprintf("Pending network permissions (%d)", len(result.Items)), colorTokenWarning)
	output.row("Scope", "Current Tobari only", colorTokenMuted)
	output.row("Window", fmt.Sprintf("%d Gateway lines", result.WindowLines), colorTokenMuted)
	for index, item := range result.Items {
		output.section(fmt.Sprintf("Permission %d", index+1))
		request := fmt.Sprintf("%s:%d %s %s", safeExternalText(item.Host), item.Port, safeExternalText(item.Method), safeExternalText(item.Path))
		output.row("Request", request, colorTokenAccent)
		output.row("Observed", safeExternalText(item.ObservedAt), colorTokenMuted)
		output.row("Reason", safeExternalText(item.Reason), colorTokenWarning)
		output.row("Status", fmt.Sprintf("%d", item.StatusCode), colorTokenWarning)
		output.row("Allow exact", allowCommand+" --id "+item.ID, colorTokenAccent)
		output.row("Deny exact", denyCommand+" --id "+item.ID, colorTokenAccent)
	}
	return output.bytes()
}

func renderPolicyReviewCanceled() []byte {
	output := newHumanOutput(true)
	output.heading("·", "Permission review canceled", colorTokenMuted)
	output.row("Changed", "No permissions changed.", colorTokenMuted)
	output.next("policy review", "Review pending permissions when you are ready.")
	return output.bytes()
}

func renderPolicyRulesWithCommands(
	result tobari.PolicyRuleReport, resetCommand string, format successFormat, color bool,
) ([]byte, error) {
	items := policyRuleOutputs(result, resetCommand)
	if format == successFormatJSON {
		output, err := json.Marshal(policyRulesDocument{SchemaVersion: 1, PolicyRules: items})
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract, "output_encoding_failed",
				"policy rules JSON could not be encoded", false, err,
			)
		}
		return append(output, '\n'), nil
	}
	if color && format == successFormatText {
		return renderPolicyRulesHuman(result, resetCommand), nil
	}
	var output bytes.Buffer
	for _, item := range items {
		fmt.Fprintf(
			&output,
			"id=%s\tdecision=%s\tmatch=%s\tproject_id=%s\thost=%s\tport=%d\tmethod=%s\tpath=%s\texamples=%s\tsource_candidates=%s\treset_command=%s\n",
			item.ID, item.Decision, item.Match, item.ProjectID, escapeTSVCell(item.Host), item.Port,
			escapeTSVCell(item.Method), escapeTSVCell(item.Path), escapeTSVCell(strings.Join(item.Examples, ",")),
			escapeTSVCell(strings.Join(item.SourceCandidates, ",")), escapeTSVCell(item.ResetCommand),
		)
	}
	return output.Bytes(), nil
}

func policyRuleOutputs(result tobari.PolicyRuleReport, resetCommand string) []policyRuleOutput {
	items := make([]policyRuleOutput, 0, len(result.Items))
	for _, rule := range result.Items {
		examples := make([]string, len(rule.Examples))
		for index, example := range rule.Examples {
			examples[index] = safeExternalText(example)
		}
		items = append(items, policyRuleOutput{
			ID: rule.ID, Decision: rule.Decision, Match: safeExternalText(rule.Match),
			ProjectID: rule.ProjectID, Host: safeExternalText(rule.Host), Port: rule.Port,
			Method: safeExternalText(rule.Method), Path: safeExternalText(rule.Path),
			Examples: examples, SourceCandidates: append([]string{}, rule.SourceCandidates...),
			ResetCommand: resetCommand + " --id " + rule.ID,
		})
	}
	return items
}

func renderPolicyRulesHuman(result tobari.PolicyRuleReport, resetCommand string) []byte {
	if len(result.Items) == 0 {
		output := newHumanOutput(true)
		output.empty(
			"No learned policy decisions",
			"No current Allow or exact Deny decision is active.",
			"policy review", "Review retained denied permissions when one needs a decision.",
		)
		return output.bytes()
	}
	output := newHumanOutput(true)
	output.heading("✓", fmt.Sprintf("Learned policy decisions (%d)", len(result.Items)), colorTokenSuccess)
	output.row("Policy", safeExternalText(result.PolicyDirectory), colorTokenMuted)
	output.row("Scope", "Current Context only", colorTokenMuted)
	for _, decision := range []string{tobari.PolicyDecisionAllow, tobari.PolicyDecisionDeny} {
		count := 0
		for _, item := range result.Items {
			if item.Decision == decision {
				count++
			}
		}
		if count == 0 {
			continue
		}
		title := "Allowed"
		if decision == tobari.PolicyDecisionDeny {
			title = "Denied"
		}
		output.section(fmt.Sprintf("%s (%d)", title, count))
		for _, item := range result.Items {
			if item.Decision != decision {
				continue
			}
			output.row("Request", policyRuleRequest(item), colorTokenAccent)
			output.row("ID", item.ID, colorTokenAccent)
			output.row("Match", safeExternalText(item.Match), colorTokenMuted)
			output.row("Project", safeExternalText(item.ProjectID), colorTokenMuted)
			output.row("Reset", resetCommand+" --id "+item.ID, colorTokenWarning)
		}
	}
	output.next("policy review", "Reset an existing decision only when you want to review the effect again.")
	return output.bytes()
}

func renderPolicyRulesCanceled() []byte {
	output := newHumanOutput(true)
	output.heading("·", "Policy decision review canceled", colorTokenMuted)
	output.row("Changed", "No policy decisions changed.", colorTokenMuted)
	output.next("policy rules", "Inspect current learned decisions when you are ready.")
	return output.bytes()
}

func renderPolicyRuleResetWithColor(result tobari.PolicyRuleReset, color bool) []byte {
	if color {
		output := newHumanOutput(true)
		output.heading("✓", "Policy decision reset", colorTokenSuccess)
		output.row("Policy", safeExternalText(result.PolicyDirectory), colorTokenMuted)
		output.row("Target", result.TargetID, colorTokenAccent)
		output.row("Removed", safeExternalText(result.Decision), colorTokenWarning)
		output.row("Default deny", humanBool(result.Applied), humanBoolToken(result.Applied))
		output.next("policy review", "Review the retained denied effect again before granting a new decision.")
		return output.bytes()
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "policy: %s\n", escapeTSVCell(result.PolicyDirectory))
	fmt.Fprintf(&output, "target_id: %s\n", result.TargetID)
	fmt.Fprintf(&output, "decision: %s\n", escapeTSVCell(result.Decision))
	fmt.Fprintf(&output, "applied: %t\n", result.Applied)
	fmt.Fprintln(&output, "next: tobari policy review")
	return output.Bytes()
}

func renderPolicyReviewAllowSuccess(result tobari.PolicyLearningChange, color bool) []byte {
	if !color {
		var output bytes.Buffer
		fmt.Fprintln(&output, "testing_policy: passed")
		fmt.Fprintln(&output, "applying_exact_rule: applied")
		fmt.Fprintln(&output, "permission_allowed: true")
		fmt.Fprintln(&output, "host: "+escapeTSVCell(result.Rule.Host))
		fmt.Fprintln(&output, "port: "+strconv.Itoa(result.Rule.Port))
		fmt.Fprintln(&output, "method: "+escapeTSVCell(result.Rule.Method))
		fmt.Fprintln(&output, "path: "+escapeTSVCell(result.Rule.Path))
		fmt.Fprintln(&output, "next: tobari")
		return output.Bytes()
	}

	output := newHumanOutput(true)
	output.heading("✓", "Permission allowed", colorTokenSuccess)
	output.row("Testing policy", "passed", colorTokenSuccess)
	output.row("Applying exact rule", "applied", colorTokenSuccess)
	output.row("Scope", "Current Tobari only", colorTokenMuted)
	output.row("Request", fmt.Sprintf(
		"%s:%d %s %s", safeExternalText(result.Rule.Host), result.Rule.Port,
		safeExternalText(result.Rule.Method), safeExternalText(result.Rule.Path),
	), colorTokenAccent)
	output.next("tobari", "Re-enter the Workspace and retry the same request.")
	return output.bytes()
}

type policyCompactionOutput struct {
	ID              string   `json:"id"`
	ProjectID       string   `json:"project_id"`
	Host            string   `json:"host"`
	Port            int      `json:"port"`
	Method          string   `json:"method"`
	PathPrefix      string   `json:"path_prefix"`
	SourceRuleCount int      `json:"source_rule_count"`
	Examples        []string `json:"examples"`
	OutsideCanary   string   `json:"outside_canary"`
	CompactCommand  string   `json:"compact_command"`
}

type policyCompactionsDocument struct {
	SchemaVersion     int                      `json:"schema_version"`
	PolicyCompactions []policyCompactionOutput `json:"policy_compactions"`
}

func renderPolicyCompactions(
	result tobari.PolicyCompactionReport, compactCommand string, format successFormat,
) ([]byte, error) {
	return renderPolicyCompactionsWithColor(result, compactCommand, format, false)
}

func renderPolicyCompactionsWithColor(
	result tobari.PolicyCompactionReport, compactCommand string, format successFormat, color bool,
) ([]byte, error) {
	items := make([]policyCompactionOutput, 0, len(result.Items))
	for _, item := range result.Items {
		examples := make([]string, len(item.Examples))
		for index, example := range item.Examples {
			examples[index] = safeExternalText(example)
		}
		items = append(items, policyCompactionOutput{
			ID: item.ID, ProjectID: item.ProjectID, Host: safeExternalText(item.Host), Port: item.Port, Method: safeExternalText(item.Method),
			PathPrefix: safeExternalText(item.PathPrefix), SourceRuleCount: len(item.SourceRuleIDs),
			Examples: examples, OutsideCanary: safeExternalText(item.OutsideCanary),
			CompactCommand: compactCommand + " --id " + item.ID,
		})
	}
	if format == successFormatJSON {
		output, err := json.Marshal(policyCompactionsDocument{
			SchemaVersion: 2, PolicyCompactions: items,
		})
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract, "output_encoding_failed",
				"policy compactions JSON could not be encoded", false, err,
			)
		}
		return append(output, '\n'), nil
	}
	if color && format == successFormatText {
		return renderPolicyCompactionsHuman(result, compactCommand), nil
	}
	var output bytes.Buffer
	for _, item := range result.Items {
		action := compactCommand + " --id " + item.ID
		fmt.Fprintf(
			&output,
			"id=%s\tproject_id=%s\thost=%s\tport=%d\tmethod=%s\tpath_prefix=%s\tsource_rule_count=%d\texamples=%s\toutside_canary=%s\tcompact_command=%s\n",
			item.ID, item.ProjectID, escapeTSVCell(item.Host), item.Port, escapeTSVCell(item.Method),
			escapeTSVCell(item.PathPrefix), len(item.SourceRuleIDs),
			escapeTSVCell(strings.Join(item.Examples, ",")), escapeTSVCell(item.OutsideCanary),
			escapeTSVCell(action),
		)
	}
	return output.Bytes(), nil
}

func renderPolicyCompactionsHuman(result tobari.PolicyCompactionReport, compactCommand string) []byte {
	if len(result.Items) == 0 {
		output := newHumanOutput(true)
		output.empty("No policy compactions", "No compatible exact rules are ready to be compacted.", "policy candidates", "Review the current exact policy candidates.")
		return output.bytes()
	}
	output := newHumanOutput(true)
	output.heading("✓", fmt.Sprintf("Policy compactions (%d)", len(result.Items)), colorTokenSuccess)
	output.row("Policy", safeExternalText(result.PolicyDirectory), colorTokenMuted)
	for index, item := range result.Items {
		output.section(fmt.Sprintf("Compaction %d", index+1))
		request := fmt.Sprintf("%s:%d %s %s", safeExternalText(item.Host), item.Port, safeExternalText(item.Method), safeExternalText(item.PathPrefix))
		output.row("Request", request, colorTokenAccent)
		output.row("ID", item.ID, colorTokenAccent)
		output.row("Project", safeExternalText(item.ProjectID), colorTokenMuted)
		output.row("Source rules", fmt.Sprintf("%d", len(item.SourceRuleIDs)), colorTokenMuted)
		examples := make([]string, len(item.Examples))
		for exampleIndex, example := range item.Examples {
			examples[exampleIndex] = safeExternalText(example)
		}
		output.row("Examples", strings.Join(examples, ", "), colorTokenMuted)
		output.row("Canary", safeExternalText(item.OutsideCanary), colorTokenWarning)
		output.row("Compact", compactCommand+" --id "+item.ID, colorTokenAccent)
	}
	return output.bytes()
}

func renderPolicyLearningChange(result tobari.PolicyLearningChange) []byte {
	return renderPolicyLearningChangeWithColor(result, false)
}

func renderPolicyLearningChangeWithColor(result tobari.PolicyLearningChange, color bool) []byte {
	if color {
		output := newHumanOutput(true)
		marker, title, token := "✓", "Policy rule updated", colorTokenSuccess // #nosec G101 -- human-readable status text contains no credential.
		if !result.Applied {
			marker, title, token = "!", "Policy rule recorded", colorTokenWarning // #nosec G101 -- human-readable status text contains no credential.
		}
		output.heading(marker, title, token)
		output.row("Policy", safeExternalText(result.PolicyDirectory), colorTokenMuted)
		output.row("Target", result.TargetID, colorTokenAccent)
		output.row("Rule", result.Rule.ID, colorTokenAccent)
		output.row("Match", safeExternalText(result.Rule.Match), colorTokenMuted)
		output.row("Request", fmt.Sprintf("%s:%d %s %s", safeExternalText(result.Rule.Host), result.Rule.Port, safeExternalText(result.Rule.Method), safeExternalText(result.Rule.Path)), colorTokenAccent)
		output.row("Project", safeExternalText(result.Rule.ProjectID), colorTokenMuted)
		output.row("Source rules", fmt.Sprintf("%d", result.SourceRuleCount), colorTokenMuted)
		output.row("Applied", humanBool(result.Applied), humanBoolToken(result.Applied))
		if result.Task == tobari.TaskPolicyAllow {
			output.next("tobari", "Re-enter the Workspace and retry the same request.")
		} else {
			output.next("cluster status", "Verify the shared policy component after the change.")
		}
		return output.bytes()
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "policy: %s\n", escapeTSVCell(result.PolicyDirectory))
	fmt.Fprintf(&output, "target_id: %s\n", result.TargetID)
	fmt.Fprintf(&output, "rule_id: %s\n", result.Rule.ID)
	fmt.Fprintf(&output, "match: %s\n", escapeTSVCell(result.Rule.Match))
	fmt.Fprintf(&output, "project_id: %s\n", result.Rule.ProjectID)
	fmt.Fprintf(&output, "host: %s\n", escapeTSVCell(result.Rule.Host))
	fmt.Fprintf(&output, "port: %d\n", result.Rule.Port)
	fmt.Fprintf(&output, "method: %s\n", escapeTSVCell(result.Rule.Method))
	fmt.Fprintf(&output, "path: %s\n", escapeTSVCell(result.Rule.Path))
	fmt.Fprintf(&output, "source_rule_count: %d\n", result.SourceRuleCount)
	fmt.Fprintf(&output, "applied: %t\n", result.Applied)
	return output.Bytes()
}

func renderPolicyDenyChangeWithColor(result tobari.PolicyDenyChange, color bool) []byte {
	if color {
		output := newHumanOutput(true)
		output.heading("✓", "Permission denied", colorTokenSuccess)
		output.row("Policy", safeExternalText(result.PolicyDirectory), colorTokenMuted)
		output.row("Target", result.TargetID, colorTokenAccent)
		output.row("Rule", result.Rule.ID, colorTokenAccent)
		output.row("Request", fmt.Sprintf(
			"%s:%d %s %s", safeExternalText(result.Rule.Host), result.Rule.Port,
			safeExternalText(result.Rule.Method), safeExternalText(result.Rule.Path),
		), colorTokenAccent)
		output.row("Project", safeExternalText(result.Rule.ProjectID), colorTokenMuted)
		output.row("Applied", humanBool(result.Applied), humanBoolToken(result.Applied))
		output.next("policy review", "Review the remaining pending permissions.")
		return output.bytes()
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "policy: %s\n", escapeTSVCell(result.PolicyDirectory))
	fmt.Fprintf(&output, "target_id: %s\n", result.TargetID)
	fmt.Fprintf(&output, "rule_id: %s\n", result.Rule.ID)
	fmt.Fprintf(&output, "project_id: %s\n", escapeTSVCell(result.Rule.ProjectID))
	fmt.Fprintf(&output, "host: %s\n", escapeTSVCell(result.Rule.Host))
	fmt.Fprintf(&output, "port: %d\n", result.Rule.Port)
	fmt.Fprintf(&output, "method: %s\n", escapeTSVCell(result.Rule.Method))
	fmt.Fprintf(&output, "path: %s\n", escapeTSVCell(result.Rule.Path))
	fmt.Fprintf(&output, "source_rule_count: %d\n", result.SourceRuleCount)
	fmt.Fprintf(&output, "applied: %t\n", result.Applied)
	return output.Bytes()
}

func renderClusterDenials(
	result tobari.DenialReport, reviewCommand string, format successFormat,
) ([]byte, error) {
	return renderClusterDenialsWithColor(result, reviewCommand, format, false)
}

func renderClusterDenialsWithColor(
	result tobari.DenialReport, reviewCommand string, format successFormat, color bool,
) ([]byte, error) {
	return renderClusterDenialsWithReviewCommand(
		result, reviewCommand, format, color,
	)
}

func renderClusterDenialsWithReviewCommand(
	result tobari.DenialReport, reviewCommand string, format successFormat, color bool,
) ([]byte, error) {
	if format == successFormatJSON {
		items := append([]tobari.PolicyDenial{}, result.Items...)
		for index := range items {
			items[index].Timestamp = safeExternalText(items[index].Timestamp)
			items[index].RequestID = safeExternalText(items[index].RequestID)
			items[index].Host = safeExternalText(items[index].Host)
			items[index].Method = safeExternalText(items[index].Method)
			items[index].Path = safeExternalText(items[index].Path)
			items[index].Reason = safeExternalText(items[index].Reason)
		}
		output, err := json.Marshal(clusterDenialsDocument{
			SchemaVersion: 2,
			Denials: clusterDenialsOutput{
				Policy: safeExternalText(result.PolicyDirectory), WindowLines: result.WindowLines,
				Items: items, ReviewCommand: reviewCommand,
			},
		})
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract, "output_encoding_failed",
				"cluster denials JSON could not be encoded", false, err,
			)
		}
		return append(output, '\n'), nil
	}
	if color && format == successFormatText {
		return renderClusterDenialsHuman(result, reviewCommand), nil
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "policy: %s\n", escapeTSVCell(result.PolicyDirectory))
	fmt.Fprintf(&output, "window_lines: %d\n", result.WindowLines)
	fmt.Fprintf(&output, "denial_count: %d\n", len(result.Items))
	for _, item := range result.Items {
		fmt.Fprintf(
			&output,
			"denial: timestamp=%s\trequest_id=%s\tproject_id=%s\thost=%s\tport=%d\tmethod=%s\tpath=%s\tstatus_code=%d\treason=%s\n",
			escapeTSVCell(item.Timestamp), escapeTSVCell(item.RequestID),
			item.ProjectID, escapeTSVCell(item.Host), item.Port, escapeTSVCell(item.Method),
			escapeTSVCell(item.Path), item.StatusCode, escapeTSVCell(item.Reason),
		)
	}
	fmt.Fprintf(&output, "review_command: %s\n", escapeTSVCell(reviewCommand))
	return output.Bytes(), nil
}

func renderClusterDenialsHuman(result tobari.DenialReport, reviewCommand string) []byte {
	if len(result.Items) == 0 {
		output := newHumanOutput(true)
		output.empty("No policy denials", "The selected Gateway log window contains no denied requests.", "policy candidates", "Check whether a new denied request has been retained.")
		return output.bytes()
	}
	output := newHumanOutput(true)
	output.heading("✓", fmt.Sprintf("Policy denials (%d)", len(result.Items)), colorTokenSuccess)
	output.row("Policy", safeExternalText(result.PolicyDirectory), colorTokenMuted)
	output.row("Window", fmt.Sprintf("%d lines", result.WindowLines), colorTokenMuted)
	output.row("Review", reviewCommand, colorTokenAccent)
	for index, item := range result.Items {
		output.section(fmt.Sprintf("Denial %d", index+1))
		output.row("Request", fmt.Sprintf("%s:%d %s %s", safeExternalText(item.Host), item.Port, safeExternalText(item.Method), safeExternalText(item.Path)), colorTokenAccent)
		output.row("Timestamp", safeExternalText(item.Timestamp), colorTokenMuted)
		output.row("Request ID", item.RequestID, colorTokenMuted)
		output.row("Project", safeExternalText(item.ProjectID), colorTokenMuted)
		output.row("Status", fmt.Sprintf("%d", item.StatusCode), colorTokenWarning)
		output.row("Reason", safeExternalText(item.Reason), colorTokenWarning)
		learnable := "no"
		if item.Learnable {
			learnable = "yes"
		}
		output.row("Learnable", learnable, humanBoolToken(item.Learnable))
	}
	return output.bytes()
}

type clusterStatusOutput struct {
	Configured  bool                     `json:"configured"`
	Running     bool                     `json:"running"`
	Proxy       string                   `json:"proxy"`
	Policy      string                   `json:"policy"`
	TobariCount int                      `json:"tobari_count"`
	Components  []tobari.ComponentStatus `json:"components"`
	RecentError string                   `json:"recent_error"`
}

func renderClusterStatus(status tobari.ClusterStatus, format successFormat, color bool) ([]byte, error) {
	if format == successFormatJSON {
		document := clusterStatusDocument{
			SchemaVersion: 1,
			Cluster: clusterStatusOutput{
				Configured: status.Configured, Running: status.Running,
				Proxy: safeExternalText(status.Proxy), Policy: safeExternalText(status.Policy),
				TobariCount: status.TobariCount,
				Components:  append([]tobari.ComponentStatus{}, status.Components...),
				RecentError: safeExternalText(status.RecentError),
			},
		}
		output, err := json.Marshal(document)
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "cluster status JSON could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}
	return renderClusterStatusTextWithColor(status, color), nil
}

func renderClusterStatusText(status tobari.ClusterStatus) []byte {
	return renderClusterStatusTextWithColor(status, false)
}

func renderClusterUpText(status tobari.ClusterStatus, color bool) []byte {
	output := renderClusterStatusTextWithColor(status, color)
	if !status.Configured || !status.Running {
		return output
	}
	var withNext bytes.Buffer
	withNext.Write(output)
	fmt.Fprintf(
		&withNext, "\n%s from a project directory, run `tobari`.\n",
		applyColorToken(color, colorTokenAccent, "Next:"),
	)
	return withNext.Bytes()
}

func renderClusterStatusTextWithColor(status tobari.ClusterStatus, color bool) []byte {
	var output bytes.Buffer
	marker, heading, headingToken := clusterStatusHeading(status)
	fmt.Fprintf(&output, "%s %s\n", applyColorToken(color, headingToken, marker), heading)
	if !status.Configured {
		renderClusterRecentError(&output, status.RecentError, color)
		return output.Bytes()
	}
	fmt.Fprintln(&output)
	for _, component := range status.Components {
		renderClusterComponent(&output, component, status.Running, color)
	}
	fmt.Fprintf(
		&output, "  %s %d\n",
		applyColorToken(color, colorTokenMuted, fmt.Sprintf("%-8s", "Tobari")), status.TobariCount,
	)
	if status.Policy != "" {
		fmt.Fprintln(&output)
		fmt.Fprintf(
			&output, "  %s %s\n",
			applyColorToken(color, colorTokenMuted, fmt.Sprintf("%-8s", "Policy")), escapeTSVCell(status.Policy),
		)
	}
	renderClusterRecentError(&output, status.RecentError, color)
	return output.Bytes()
}

func clusterStatusHeading(status tobari.ClusterStatus) (string, string, colorToken) {
	switch {
	case status.Task == tobari.TaskClusterDown && !status.Configured:
		return "✓", "Cluster removed", colorTokenSuccess
	case !status.Configured:
		return "○", "Cluster not configured", colorTokenMuted
	case !status.Running:
		return "!", "Cluster not ready", colorTokenWarning
	default:
		return "✓", "Cluster ready", colorTokenSuccess
	}
}

func renderClusterComponent(output *bytes.Buffer, component tobari.ComponentStatus, ready, color bool) {
	name := clusterComponentName(component.Name)
	health := escapeTSVCell(component.Health)
	healthOutput := applyColorToken(color, clusterHealthColorToken(component.Health), health)
	if ready && component.Health == "healthy" {
		fmt.Fprintf(output, "  %-8s %s\n", name, healthOutput)
		return
	}
	state := applyColorToken(color, colorTokenMuted, escapeTSVCell(component.State))
	fmt.Fprintf(output, "  %-8s %s · %s\n", name, state, healthOutput)
}

func clusterComponentName(name string) string {
	switch strings.ToLower(name) {
	case "gateway":
		return "Gateway"
	case "opa":
		return "OPA"
	default:
		return escapeTSVCell(name)
	}
}

func clusterHealthColorToken(health string) colorToken {
	switch strings.ToLower(health) {
	case "healthy":
		return colorTokenSuccess
	case "starting", "pending", "unknown":
		return colorTokenWarning
	case "unhealthy", "exited", "dead", "failed":
		return colorTokenError
	default:
		return colorTokenMuted
	}
}

func renderClusterRecentError(output *bytes.Buffer, recentError string, color bool) {
	if recentError == "" {
		return
	}
	if output.Len() > 0 && !strings.HasSuffix(output.String(), "\n\n") {
		fmt.Fprintln(output)
	}
	fmt.Fprintf(
		output, "  %s  %s\n",
		applyColorToken(color, colorTokenMuted, "Recent error"),
		applyColorToken(color, colorTokenError, escapeTSVCell(recentError)),
	)
}

type tobariListDocument struct {
	SchemaVersion int                 `json:"schema_version"`
	Tobari        []tobari.ItemStatus `json:"tobari"`
}

func renderTobariList(result tobari.ListResult, format successFormat) ([]byte, error) {
	return renderTobariListWithColor(result, format, false)
}

func renderTobariListWithColor(result tobari.ListResult, format successFormat, color bool) ([]byte, error) {
	if format == successFormatJSON {
		items := append([]tobari.ItemStatus{}, result.Items...)
		for index := range items {
			items[index].Name = safeExternalText(items[index].Name)
			items[index].Root = safeExternalText(items[index].Root)
			items[index].Image = safeExternalText(items[index].Image)
			items[index].Container = safeExternalText(items[index].Container)
		}
		output, err := json.Marshal(tobariListDocument{SchemaVersion: 2, Tobari: items})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Tobari list JSON could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}
	if color && format == successFormatText {
		if len(result.Items) == 0 {
			output := newHumanOutput(true)
			output.empty("No Tobari attached", "The shared cluster has no attached Tobari.", "tobari", "Create or enter a Tobari from the current project directory.")
			return output.bytes(), nil
		}
		output := newHumanOutput(true)
		output.heading("✓", fmt.Sprintf("Tobari (%d)", len(result.Items)), colorTokenSuccess)
		for index, item := range result.Items {
			output.section(fmt.Sprintf("Tobari %d", index+1))
			output.row("Name", safeExternalText(item.Name), colorTokenAccent)
			output.row("ID", item.ID, colorTokenAccent)
			output.row("Root", safeExternalText(item.Root), colorTokenMuted)
			output.row("Image", safeExternalText(item.Image), colorTokenMuted)
			output.row("Container", safeExternalText(item.Container), colorTokenMuted)
			output.row("Running", humanBool(item.Running), humanBoolToken(item.Running))
		}
		return output.bytes(), nil
	}
	var output bytes.Buffer
	for _, item := range result.Items {
		fmt.Fprintf(
			&output, "id=%s\tname=%s\troot=%s\timage=%s\trunning=%t\tcontainer=%s\n",
			item.ID, escapeTSVCell(item.Name), escapeTSVCell(item.Root), escapeTSVCell(item.Image),
			item.Running, escapeTSVCell(item.Container),
		)
	}
	return output.Bytes(), nil
}

type projectStatusOutput struct {
	Exists  bool   `json:"exists"`
	Root    string `json:"root"`
	ID      string `json:"id"`
	Home    string `json:"home"`
	Runtime string `json:"runtime"`
}

type projectStatusDocument struct {
	SchemaVersion int                 `json:"schema_version"`
	Status        projectStatusOutput `json:"status"`
}

func renderProjectStatus(result tobari.ProjectStatus, format successFormat) ([]byte, error) {
	return renderProjectStatusWithColor(result, format, false)
}

func renderProjectStatusWithColor(result tobari.ProjectStatus, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_status_contract", "project status is invalid", false, err)
	}
	value := projectStatusOutput{
		Exists: result.Exists, Root: safeExternalText(result.Root), ID: result.ID,
		Home: safeExternalText(result.Home), Runtime: string(result.Runtime),
	}
	if format == successFormatJSON {
		output, err := json.Marshal(projectStatusDocument{SchemaVersion: 1, Status: value})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "project status JSON could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}
	if color && format == successFormatText {
		if !result.Exists {
			output := newHumanOutput(true)
			output.empty("No Tobari for this directory", "The current directory is not associated with a CWD-owned Tobari.", "tobari", "Create or enter the current directory's Tobari.")
			return output.bytes(), nil
		}
		output := newHumanOutput(true)
		marker, title, token := "✓", "Tobari ready", colorTokenSuccess
		if result.Runtime != tobari.RuntimeDiagnosticReady {
			marker, title, token = "!", "Tobari needs attention", colorTokenWarning
		}
		output.heading(marker, title, token)
		output.row("Root", safeExternalText(result.Root), colorTokenMuted)
		output.row("Runtime", safeExternalText(string(result.Runtime)), humanStatusToken(string(result.Runtime)))
		output.row("ID", result.ID, colorTokenAccent)
		output.row("Home", safeExternalText(result.Home), colorTokenMuted)
		if result.Runtime != tobari.RuntimeDiagnosticReady {
			output.next("doctor", "Inspect the local runtime before entering the project.")
		} else {
			output.next("tobari", "Enter the current directory's Tobari.")
		}
		return output.bytes(), nil
	}
	if !result.Exists {
		return []byte("No Tobari exists for the current directory\n"), nil
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "Tobari exists at %s\n", escapeTSVCell(result.Root))
	fmt.Fprintf(&output, "Runtime: %s\n", escapeTSVCell(string(result.Runtime)))
	return output.Bytes(), nil
}

type projectListOutput struct {
	Root    string `json:"root"`
	Runtime string `json:"runtime"`
	ID      string `json:"id"`
}

type projectListDocument struct {
	SchemaVersion int                 `json:"schema_version"`
	Tobari        []projectListOutput `json:"tobari"`
}

func renderProjectList(result tobari.ProjectListResult, format successFormat) ([]byte, error) {
	return renderProjectListWithColor(result, format, false)
}

func renderProjectListWithColor(result tobari.ProjectListResult, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_list_contract", "project list is invalid", false, err)
	}
	items := make([]projectListOutput, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, projectListOutput{
			Root: safeExternalText(item.Root), Runtime: string(item.Runtime), ID: item.ID,
		})
	}
	if format == successFormatJSON {
		output, err := json.Marshal(projectListDocument{SchemaVersion: 1, Tobari: items})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "project list JSON could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}
	if color && format == successFormatText {
		if len(items) == 0 {
			empty := newHumanOutput(true)
			empty.empty("No Workspaces", "No Workspace state is configured.", "tobari", "Create or enter a Workspace from the current directory.")
			return empty.bytes(), nil
		}
		output := newHumanOutput(true)
		output.heading("✓", fmt.Sprintf("Workspaces (%d)", len(items)), colorTokenSuccess)
		for _, item := range items {
			marker := "  "
			rootToken := colorTokenAccent
			if item.ID == result.CurrentID {
				marker = "▸ "
				rootToken = colorTokenSelected
			}
			output.sectionWithToken(marker+item.Root, rootToken)
			output.row("Runtime", item.Runtime, humanStatusToken(item.Runtime))
			output.row("ID", item.ID, colorTokenMuted)
		}
		return output.bytes(), nil
	}
	var output bytes.Buffer
	fmt.Fprintln(&output, "ROOT\tRUNTIME\tID")
	for _, item := range items {
		fmt.Fprintf(&output, "%s\t%s\t%s\n", escapeTSVCell(item.Root), item.Runtime, item.ID)
	}
	return output.Bytes(), nil
}

func renderProjectDelete(result tobari.ProjectDeleteResult) []byte {
	return renderProjectDeleteWithColor(result, false)
}

func renderProjectDeleteWithColor(result tobari.ProjectDeleteResult, color bool) []byte {
	if color {
		output := newHumanOutput(true)
		marker, title, token := "✓", "Tobari deleted", colorTokenSuccess
		if !result.Deleted {
			marker, title, token = "!", "Tobari not deleted", colorTokenWarning // #nosec G101 -- human-readable status text contains no credential.
		}
		output.heading(marker, title, token)
		output.row("Root", safeExternalText(result.Root), colorTokenMuted)
		if result.Deleted {
			output.next("tobari", "Create or enter a Tobari from this project directory.")
		}
		return output.bytes()
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "deleted: %t\n", result.Deleted)
	fmt.Fprintf(&output, "root: %s\n", escapeTSVCell(result.Root))
	fmt.Fprintf(&output, "id: %s\n", result.ID)
	fmt.Fprintf(&output, "home: %s\n", escapeTSVCell(result.Home))
	return output.Bytes()
}

func renderAttachResult(instance tobari.Instance, color bool) []byte {
	if color {
		output := newHumanOutput(true)
		output.heading("✓", "Tobari attached", colorTokenSuccess)
		output.row("Name", safeExternalText(instance.Name), colorTokenAccent)
		output.row("Root", safeExternalText(instance.Root), colorTokenMuted)
		output.row("Image", safeExternalText(instance.Image), colorTokenMuted)
		output.next("list", "Review configured Tobari projects.")
		return output.bytes()
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "name: %s\n", escapeTSVCell(instance.Name))
	fmt.Fprintf(&output, "root: %s\n", escapeTSVCell(instance.Root))
	fmt.Fprintf(&output, "image: %s\n", escapeTSVCell(instance.Image))
	return output.Bytes()
}

func renderDetachedResult(color bool) []byte {
	if color {
		output := newHumanOutput(true)
		output.heading("✓", "Tobari detached", colorTokenSuccess)
		output.next("list", "Review the remaining configured Tobari projects.")
		return output.bytes()
	}
	return []byte("detached: true\n")
}

func renderSafeLogs(raw []byte) []byte {
	var output strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		output.WriteString(safeExternalText(line))
		output.WriteByte('\n')
	}
	return []byte(output.String())
}

func missingRuntimeFault() *fault.Error {
	return fault.New(
		fault.KindInternal, "missing_runtime", "Tobari runtime is not configured.", false,
		fault.NextAction{Command: "doctor", Reason: "Inspect local runtime configuration."},
	)
}
