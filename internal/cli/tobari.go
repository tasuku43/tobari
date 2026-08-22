package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
		progress = newClusterUpProgress(c.Err, humanStyleAllowed(ctx, c, c.Err))
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
	return c.emitMutationResult(ctx, command, renderClusterUpText(status, clusterStyleAllowed(ctx, c)))
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
	output, err := renderClusterStatus(status, format, clusterStyleAllowed(ctx, c))
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
	return c.emitResult(ctx, renderSafeLogs(output, humanStyleAllowed(ctx, c, c.Out)))
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
	review, found := c.catalog.Lookup("review permissions")
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "review permissions command is missing", false,
		))
	}
	output, err := renderClusterDenialsWithReviewCommand(
		result, ProgramName+" "+review.Path,
		format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out),
	)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runPolicyCandidates(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	return runPolicyCandidateQueue(ctx, c, command, inputs)
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
	watch, _ := inputs.Boolean("--watch")
	notifyMethod := inputs.One("--notify")
	if inputs.Provided("--notify") && !watch {
		return c.failUsage(
			ctx, "invalid_arguments", "--notify requires --watch=true; usage: "+command.Usage(),
			"help "+command.Path, "Use a notification method only with active watch mode.",
		)
	}
	if watch && (format != successFormatText || !policyReviewInteractiveAllowed(ctx, c)) {
		return c.fail(ctx, fault.New(
			fault.KindInvalidInput, "policy_review_watch_requires_tty",
			"review permissions --watch requires text output and interactive terminal input and output", false,
			fault.NextAction{Command: "help review permissions", Reason: "Run watch with text output in an interactive raw terminal."},
		))
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
			format == successFormatText && humanStyleAllowed(ctx, c, c.Out),
		)
		if renderErr != nil {
			return c.fail(ctx, renderErr)
		}
		return c.emitResult(ctx, output)
	}

	selectorFactory := c.policyReview
	if selectorFactory == nil {
		selectorFactory = newPolicyReviewSelectorWithStyle
	}
	selector := selectorFactory(humanStyleAllowed(ctx, c, c.Out))
	if watch {
		selector.EnableWatch(nil)
	}
	seenReviewIDs := policyReviewReportIDs(result)
	stagedContextID := ""
	for {
		if len(result.Items) == 0 && !watch {
			output, renderErr := renderPolicyReviewWithCommands(
				result, allowCommand, denyCommand, successFormatText,
				humanStyleAllowed(ctx, c, c.Out),
			)
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
			if watch {
				return ExitOK
			}
			return c.fail(ctx, context.Canceled)
		}
		if decision.Refresh {
			fresh, refreshErr := c.tobari.PolicyReview(ctx, int(tail))
			if refreshErr != nil {
				if !watch {
					return c.fail(ctx, refreshErr)
				}
				delay := selector.RefreshFailed()
				selector.notice = fmt.Sprintf(
					"Refresh failed · current inbox and staged decisions preserved. Retrying in %s. Run tobari cluster status if this continues.",
					delay,
				)
				continue
			}
			previousIDs := policyReviewReportIDs(result)
			newUnseen := policyReviewRememberNewIDs(seenReviewIDs, fresh)
			notificationFailed := false
			if watch && newUnseen > 0 && notifyMethod != "off" && c.policyNotify != nil {
				notificationFailed = c.policyNotify(c.Out, notifyMethod) != nil
			}
			staleIDs := policyReviewStaleDecisionIDs(selector.OrderedDecisions(), fresh)
			removed := selector.Reconcile(fresh)
			result = fresh
			selector.RefreshSucceeded()
			stagedContextID = policyReviewStagedContextID(result, selector.OrderedDecisions())
			newCount := policyReviewNewCandidateCount(previousIDs, fresh)
			if decision.AutomaticRefresh && removed == 0 && newCount == 0 && !notificationFailed {
				// A successful timer poll with no semantic change is deliberately
				// silent so the existing alternate-screen frame remains untouched.
			} else if removed > 0 {
				selector.notice = fmt.Sprintf(
					"Inbox refreshed · %d new · %d stale staged decision%s removed (%s); remaining decisions preserved.",
					newCount, removed, pluralSuffix(removed), strings.Join(staleIDs, ", "),
				)
			} else {
				selector.notice = fmt.Sprintf("Inbox refreshed · %d new · staged decisions preserved by exact candidate ID.", newCount)
			}
			if notificationFailed {
				selector.notice += " Terminal notification unavailable; watch continues."
			}
			continue
		}
		if decision.Apply {
			staged := selector.OrderedDecisions()
			if len(staged) == 0 {
				return c.fail(ctx, fault.New(
					fault.KindInvalidInput, "empty_policy_review_set",
					"stage at least one reviewed permission before Apply", false,
					fault.NextAction{Command: "review permissions", Reason: "Inspect a permission and stage one offered decision."},
				))
			}
			apply, applyFound := c.catalog.lookupRegistered("policy apply-reviewed")
			if !applyFound || apply.Agent.Mutation == nil || apply.Agent.FixedTarget == nil {
				return c.fail(ctx, fault.New(
					fault.KindContract, "invalid_catalog", "reviewed policy Apply contract is missing", false,
				))
			}
			set := tobari.PolicyReviewDecisionSet{Decisions: make([]tobari.PolicyReviewDecision, 0, len(staged))}
			for _, selected := range staged {
				value := tobari.PolicyDecisionAllow
				if selected.Action == policyReviewActionDeny {
					value = tobari.PolicyDecisionDeny
				}
				match := tobari.PolicyMatchExact
				if selected.Action == policyReviewActionAllowTemplate {
					match = tobari.PolicyMatchPathTemplate
				}
				set.Decisions = append(set.Decisions, tobari.PolicyReviewDecision{
					ReviewItemID: selected.CandidateID, Decision: value, Match: match,
				})
			}
			intent := operation.Intent{
				Command: apply.Path, Effect: apply.Effect,
				Target: operation.TargetRef{Kind: apply.Agent.FixedTarget.Kind, ID: apply.Agent.FixedTarget.ID},
				Impact: apply.Agent.Mutation.Impact,
			}
			actionCtx := withCommandPath(ctx, apply.Path)
			change, applyErr := c.tobari.ApplyPolicyReviewDecisionSet(actionCtx, intent, set)
			if applyErr != nil {
				return c.fail(actionCtx, applyErr)
			}
			code := c.emitMutationResult(
				actionCtx, apply, renderPolicyReviewChange(change, humanStyleAllowed(actionCtx, c, c.Out)),
			)
			if code != ExitOK || !watch {
				return code
			}
			selector.ClearAll()
			stagedContextID = ""
			// The pre-Apply snapshot is no longer eligible for another decision.
			// If the immediate read fails, keep watch alive on an explicit empty
			// snapshot instead of offering already-applied IDs again.
			result.Items = []tobari.PolicyCandidate{}
			result.ReviewItems = []tobari.PolicyReviewItem{}
			fresh, refreshErr := c.tobari.PolicyReview(ctx, int(tail))
			if refreshErr != nil {
				delay := selector.RefreshFailed()
				selector.notice = fmt.Sprintf(
					"Applied successfully; refresh failed. Retrying in %s. Run tobari cluster status if this continues.", delay,
				)
				continue
			}
			newUnseen := policyReviewRememberNewIDs(seenReviewIDs, fresh)
			notificationFailed := false
			if newUnseen > 0 && notifyMethod != "off" && c.policyNotify != nil {
				notificationFailed = c.policyNotify(c.Out, notifyMethod) != nil
			}
			result = fresh
			selector.RefreshSucceeded()
			selector.notice = "Applied decisions · watching for denied requests."
			if notificationFailed {
				selector.notice += " Terminal notification unavailable; watch continues."
			}
			continue
		}
		item, selected := policyReviewItemByID(result, decision.CandidateID)
		if !selected {
			return c.fail(ctx, fault.New(
				fault.KindContract, "invalid_policy_candidate_selection",
				"the interactive review selected an ID outside its validated snapshot", false,
				fault.NextAction{Command: "policy candidates", Reason: "Rediscover the current pending queue."},
			))
		}
		if stagedContextID != "" && policyReviewItemContext(item) != stagedContextID {
			selector.notice = "Apply or discard the staged decisions before switching Context."
			continue
		}

		if decision.Clear {
			selector.Clear(decision.CandidateID)
			stagedContextID = policyReviewStagedContextID(result, selector.OrderedDecisions())
			continue
		}
		stagedContextID = policyReviewItemContext(item)
		selector.Stage(decision.CandidateID, decision.Action)
	}
}

func policyReviewReportIDs(report tobari.PolicyCandidateReport) map[string]struct{} {
	report = groupPolicyReviewReport(report)
	ids := make(map[string]struct{}, len(report.Items))
	for _, item := range report.Items {
		ids[item.ID] = struct{}{}
	}
	return ids
}

func policyReviewNewCandidateCount(previous map[string]struct{}, fresh tobari.PolicyCandidateReport) int {
	count := 0
	for id := range policyReviewReportIDs(fresh) {
		if _, found := previous[id]; !found {
			count++
		}
	}
	return count
}

func policyReviewRememberNewIDs(seen map[string]struct{}, fresh tobari.PolicyCandidateReport) int {
	newCount := 0
	for id := range policyReviewReportIDs(fresh) {
		if _, found := seen[id]; !found {
			newCount++
		}
		seen[id] = struct{}{}
	}
	return newCount
}

func policyReviewStaleDecisionIDs(decisions []policyReviewDecision, fresh tobari.PolicyCandidateReport) []string {
	current := policyReviewReportIDs(fresh)
	stale := make([]string, 0)
	for _, decision := range decisions {
		if _, found := current[decision.CandidateID]; !found {
			stale = append(stale, decision.CandidateID)
		}
	}
	return stale
}

func policyReviewStagedContextID(
	report tobari.PolicyCandidateReport, decisions []policyReviewDecision,
) string {
	for _, decision := range decisions {
		if item, found := policyReviewItemByID(report, decision.CandidateID); found {
			return policyReviewItemContext(item)
		}
	}
	return ""
}

func policyReviewItemByID(result tobari.PolicyCandidateReport, id string) (tobari.PolicyReviewItem, bool) {
	items := result.ReviewItems
	if items == nil {
		var err error
		items, err = tobari.PolicyReviewItems(result.Items, []tobari.LearnedPolicyRule{})
		if err != nil {
			return tobari.PolicyReviewItem{}, false
		}
	}
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return tobari.PolicyReviewItem{}, false
}

func policyReviewItemContext(item tobari.PolicyReviewItem) string {
	if item.Candidate != nil {
		return item.Candidate.ContextID
	}
	if item.Template != nil {
		return item.Template.ContextID
	}
	return ""
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
			format == successFormatText && humanStyleAllowed(ctx, c, c.Out),
		)
		if renderErr != nil {
			return c.fail(ctx, renderErr)
		}
		return c.emitResult(ctx, output)
	}

	selector := newPolicyRuleSelectorWithStyle(humanStyleAllowed(ctx, c, c.Out))
	for {
		if len(result.Items) == 0 {
			output, renderErr := renderPolicyRulesWithCommands(
				result, resetCommand, successFormatText,
				humanStyleAllowed(ctx, c, c.Out),
			)
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
			return c.fail(ctx, context.Canceled)
		}
		if !policyRuleContainsID(result, decision.RuleID) {
			return c.fail(ctx, fault.New(
				fault.KindContract, "invalid_policy_rule_selection",
				"the interactive Permission Inbox selected an ID outside its validated snapshot", false,
				fault.NextAction{Command: "policy rules", Reason: "Rediscover the current learned decisions."},
			))
		}
		actionCtx := withCommandPath(ctx, reset.Path)
		change, resetErr := resetPolicyRule(actionCtx, c, reset, decision.RuleID)
		if resetErr != nil {
			return c.fail(actionCtx, resetErr)
		}
		if code := c.emitMutationResult(
			actionCtx, reset, renderPolicyRuleResetWithColor(change, humanStyleAllowed(actionCtx, c, c.Out)),
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
	_, found := policyReviewCandidateByID(result, id)
	return found
}

func policyReviewCandidateByID(result tobari.PolicyCandidateReport, id string) (tobari.PolicyCandidate, bool) {
	for _, candidate := range result.Items {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return tobari.PolicyCandidate{}, false
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
	ctx context.Context, c *CLI, command CommandSpec, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	result, err := c.tobari.PolicyCandidates(ctx, int(tail))
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
	allow, found := c.catalog.Lookup("policy allow")
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "policy allow command is missing", false,
		))
	}
	output, err := renderPolicyCandidatesWithColor(result, ProgramName+" "+allow.Path, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
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
	return c.emitMutationResult(ctx, command, renderPolicyLearningChangeWithColor(result, humanStyleAllowed(ctx, c, c.Out)))
}

func runPolicyApplyReviewed(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, _ ParsedInputs,
) int {
	return c.failUsage(
		ctx, "invalid_policy_review_session",
		command.Path+" is owned by the interactive Permission Inbox session",
		"review permissions", "Stage exact decisions in the Permission Inbox and use its final Apply action.",
	)
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
	return c.emitMutationResult(ctx, command, renderPolicyDenyChangeWithColor(result, humanStyleAllowed(ctx, c, c.Out)))
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
	return c.emitMutationResult(ctx, command, renderPolicyRuleResetWithColor(result, humanStyleAllowed(ctx, c, c.Out)))
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
	return c.emitMutationResult(ctx, command, renderClusterDownTextWithColor(status, purge, clusterStyleAllowed(ctx, c)))
}

func clusterStyleAllowed(ctx context.Context, c *CLI) bool {
	return humanStyleAllowed(ctx, c, c.Out) && clusterUpProgressAllowed(ctx)
}

func runProjectEnter(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	session := tobari.NewWorkspaceShellSession()
	if inputs.Provided("command") {
		var err error
		session, err = tobari.NewWorkspaceDirectSession(inputs.Values("command"))
		if err != nil {
			return c.failUsage(
				ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(),
				"help tobari", "Supply one non-empty executable and its exact arguments after --.",
			)
		}
	}
	contextName := inputs.One("--context")
	if code, continueEntry := prepareGuidedProjectEntry(ctx, c, contextName, session); !continueEntry {
		return code
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	code, err := c.tobari.EnterProjectSessionInContext(ctx, intent, contextName, session, c.In, c.Out, c.Err)
	if err != nil {
		return c.fail(ctx, err)
	}
	// The child interactive process owns stdout. Keep the host-side lifecycle
	// guidance on stderr so shell output from the session remains untouched.
	style := humanStyleAllowed(ctx, c, c.Err)
	message := renderProjectSessionClosed(style)
	if c.context != nil {
		if report, contextErr := c.context.Show(ctx, ""); contextErr == nil &&
			report.Runtime.Status == tobari.ContextRuntimeStatusOfficial && c.runtime != nil {
			if runtimes, runtimeErr := c.runtime.List(ctx); runtimeErr == nil {
				for _, item := range runtimes.Items {
					if item.Kind == tobari.RuntimeKindManaged && item.Ready {
						message = append(message, '\n')
						message = append(message, renderRuntimeCustomizationHint(style)...)
						message = append(message, '\n')
						break
					}
				}
			}
		}
	}
	if pending, reviewErr := c.tobari.PolicyReview(ctx, 10_000); reviewErr == nil {
		message = append(message, renderPendingPolicyNotification(pending, style)...)
	}
	_, _ = writeOnce(c.Err, message)
	return code
}

// prepareGuidedProjectEntry composes existing catalog-owned actions only for
// the interactive first-use state. Each mutation keeps its own command path,
// fixed target, impact, application invoker, and completion output boundary.
func prepareGuidedProjectEntry(
	ctx context.Context, c *CLI, contextName string, sessions ...tobari.WorkspaceSessionRequest,
) (int, bool) {
	if c == nil || c.tobari == nil || c.context == nil ||
		!c.tobari.IsInteractive(c.In, c.Err) || !c.tobari.IsTerminal(c.Out) {
		return ExitOK, true
	}
	if contextName != "" {
		showCtx := withCommandPath(ctx, "context show")
		report, err := c.context.Show(showCtx, contextName)
		if err != nil {
			return c.fail(showCtx, err), false
		}
		if runtimeErr := rootRuntimeReadinessFault(report); runtimeErr != nil {
			return c.fail(ctx, runtimeErr), false
		}
		if code := ensureClusterForGuidedEntry(ctx, c); code != ExitOK {
			return code, false
		}
		return ExitOK, true
	}
	listCtx := withCommandPath(ctx, "context list")
	contexts, err := c.context.List(listCtx)
	if err != nil {
		return c.fail(listCtx, err), false
	}
	if contexts.ContextState != tobari.ContextObservationSyntheticDefault {
		showCtx := withCommandPath(ctx, "context show")
		report, showErr := c.context.Show(showCtx, "")
		if showErr != nil {
			return c.fail(showCtx, showErr), false
		}
		if runtimeErr := rootRuntimeReadinessFault(report); runtimeErr != nil {
			return c.fail(ctx, runtimeErr), false
		}
		if code := ensureClusterForGuidedEntry(ctx, c); code != ExitOK {
			return code, false
		}
		return ExitOK, true
	}

	session := tobari.NewWorkspaceShellSession()
	if len(sessions) > 0 {
		session = sessions[0]
	}
	root, rootErr := c.tobari.CurrentProjectRoot(ctx)
	if rootErr != nil {
		return c.fail(ctx, rootErr), false
	}
	draft, draftErr := tobari.NewRecommendedFirstUseDraft(root, session)
	if draftErr != nil {
		return c.fail(ctx, fault.Wrap(
			fault.KindContract, "invalid_first_use_draft", "The recommended first-use draft is invalid.", false, draftErr,
			fault.NextAction{Command: "help tobari", Reason: "Inspect the root first-use contract."},
		)), false
	}
	reviewer := c.firstUse
	if reviewer == nil {
		reviewer = newRecommendedFirstUseReviewerWithStyle(!c.noColor)
	}
	action, reviewErr := reviewer.Review(ctx, draft, c.In, c.Err)
	if reviewErr != nil {
		if errors.Is(reviewErr, context.Canceled) || errors.Is(reviewErr, context.DeadlineExceeded) {
			return c.fail(ctx, reviewErr), false
		}
		return c.fail(ctx, fault.Wrap(
			fault.KindInternal, "first_use_review_failed", "The recommended first-use review failed before creating a Context.", false, reviewErr,
			fault.NextAction{Command: "tobari", Reason: "Retry in an interactive terminal."},
		)), false
	}
	if action == recommendedFirstUseCancel {
		return c.fail(ctx, context.Canceled), false
	}
	_, code := createContextForGuidedEntry(ctx, c, draft, action == recommendedFirstUseStart)
	if code != ExitOK {
		return code, false
	}
	if code = clusterUpForGuidedEntry(ctx, c); code != ExitOK {
		return code, false
	}

	return ExitOK, true
}

func ensureClusterForGuidedEntry(ctx context.Context, c *CLI) int {
	statusCtx := withCommandPath(ctx, "cluster status")
	status, err := c.tobari.ClusterStatus(statusCtx)
	if err != nil {
		return c.fail(statusCtx, err)
	}
	if status.Configured && status.Running && status.PolicyProjection == "valid" &&
		status.PrincipalRegistry == "valid" && status.GatewayProjection == "valid" {
		return ExitOK
	}
	return clusterUpForGuidedEntry(ctx, c)
}

func createContextForGuidedEntry(
	ctx context.Context, c *CLI, draft tobari.RecommendedFirstUseDraft, requireEmpty bool,
) (tobari.ContextReport, int) {
	command, found := c.catalog.lookupRegistered("context create")
	if !found || command.Agent.Mutation == nil {
		return tobari.ContextReport{}, c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "The guided Context creation contract is missing.", false,
			fault.NextAction{Command: "help context create", Reason: "Repair the Context creation command contract."},
		))
	}
	actionCtx := withCommandPath(ctx, command.Path)
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	var report tobari.ContextReport
	var err error
	if requireEmpty {
		report, err = c.context.CreateFirstWithComposition(
			actionCtx, intent, draft.ContextName, tobari.BuiltinImageSelector, draft.PolicyMode,
			draft.Access.SourceAccess, draft.Composition(),
		)
	} else {
		wizard := c.contextCreate
		if wizard == nil {
			wizard = newContextCreateWizardWithStyle(!c.noColor)
		}
		seeded, ok := wizard.(seededContextCreateWizard)
		if !ok {
			err = fault.New(fault.KindInternal, "context_create_wizard_failed", "The Context creation wizard cannot preserve recommended settings.", false)
		} else {
			if terminalWizard, terminalOK := wizard.(*terminalContextCreateWizard); terminalOK {
				if terminalWizard.bootstrap == nil {
					terminalWizard.bootstrap = c.context
				}
				if c.runtime != nil {
					if catalog, listErr := c.runtime.List(actionCtx); listErr == nil {
						terminalWizard.runtimes = catalog.Items
					}
				}
			}
			var selection contextCreateSelection
			selection, err = seeded.ComposeSeeded(actionCtx, c.In, c.Err, recommendedFirstUseSeed(draft))
			if err != nil {
				err = normalizeContextCreateWizardError(err)
			} else {
				policy := selection.MethodPolicy.Clone()
				composition := tobari.ContextCreateComposition{
					NativeReadiness: selection.NativeReadiness, MethodPolicy: &policy,
					RuntimeSelection: selection.RuntimeSelection,
				}
				if selection.Bootstrap != nil {
					bootstrap := selection.Bootstrap.Clone()
					composition.Bootstrap = &bootstrap
				}
				report, err = c.context.CreateWithComposition(
					actionCtx, intent, selection.Name, tobari.BuiltinImageSelector, draft.PolicyMode,
					selection.SourceAccess, composition,
				)
			}
		}
	}
	if err != nil {
		return tobari.ContextReport{}, c.fail(actionCtx, err)
	}
	stage := renderGuidedEntryStage("Context created", report.Name, humanStyleAllowed(actionCtx, c, c.Err))
	if code := c.emitMutationResultTo(actionCtx, command, stage, c.Err); code != ExitOK {
		return tobari.ContextReport{}, code
	}
	return report, ExitOK
}

func clusterUpForGuidedEntry(ctx context.Context, c *CLI) int {
	command, found := c.catalog.lookupRegistered("cluster up")
	if !found || command.Agent.Mutation == nil {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "The guided cluster startup contract is missing.", false,
			fault.NextAction{Command: "help cluster up", Reason: "Repair the cluster startup command contract."},
		))
	}
	actionCtx := withCommandPath(ctx, command.Path)
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ParentID: tobari.ClusterTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	var progress *clusterUpProgress
	var progressSink tobari.ClusterUpProgressSink
	if c.tobari.IsTerminal(c.Err) && clusterUpProgressAllowed(actionCtx) {
		progress = newClusterUpProgress(c.Err, humanStyleAllowed(actionCtx, c, c.Err))
		progress.Start()
		progressSink = progress.Report
	}
	_, err := c.tobari.ClusterUpWithProgress(actionCtx, intent, progressSink)
	if err != nil {
		if progress != nil {
			progress.Fail()
			progress.Close()
		}
		return c.fail(actionCtx, err)
	}
	if progress != nil {
		progress.Close()
	}
	stage := renderGuidedEntryStage("Shared services ready", "", humanStyleAllowed(actionCtx, c, c.Err))
	return c.emitMutationResultTo(actionCtx, command, stage, c.Err)
}

func rootRuntimeReadinessFault(report tobari.ContextReport) error {
	switch report.Runtime.Status {
	case tobari.ContextRuntimeStatusPendingBuild:
		return fault.New(
			fault.KindRejected, "runtime_build_required",
			"The current Context has a custom runtime recipe that must be built before creating or entering a Workspace.", false,
			fault.NextAction{Command: "runtime build", Reason: "Build, validate, and select the current Context runtime."},
		)
	case tobari.ContextRuntimeStatusInvalid:
		return fault.New(
			fault.KindRejected, "runtime_recipe_invalid",
			"The current Context runtime recipe is invalid and cannot be used to enter a Workspace.", false,
			fault.NextAction{Command: "context show", Reason: "Inspect the runtime recipe and selected image before rebuilding."},
		)
	default:
		return nil
	}
}

func renderGuidedEntryStage(label, detail string, style bool) []byte {
	var output strings.Builder
	line := "✓ " + label
	if detail != "" {
		line += ": " + safeExternalText(detail)
	}
	fmt.Fprintln(&output, applyStyleToken(style, styleSuccess, line))
	return []byte(output.String())
}

func renderGuidedEntryPaused(contextName string, style bool) []byte {
	var output strings.Builder
	output.WriteByte('\n')
	fmt.Fprintln(&output, applyStyleToken(style, styleText, "Setup is ready; no Workspace was created."))
	writeStyledLine(&output, style, "Context:", safeExternalText(contextName), styleText)
	writeStyledCommandLine(&output, style, "Continue:", "run ", "`tobari`", " from the project directory.")
	return []byte(output.String())
}

func renderGuidedRuntimeInitialized(report tobari.ContextReport, style bool) []byte {
	var output strings.Builder
	fmt.Fprintln(&output, applyStyleToken(style, styleSuccess, "✓ Runtime recipe created"))
	writeStyledLine(&output, style, "Dockerfile:", safeExternalText(report.Runtime.Dockerfile), styleText)
	output.WriteByte('\n')
	fmt.Fprintln(&output, applyStyleToken(style, styleText, "Edit the Dockerfile, then build and select it on the host."))
	writeStyledCommandLine(&output, style, "Build:", "run ", "`tobari runtime build`", "")
	writeStyledCommandLine(&output, style, "After the build succeeds:", "run ", "`tobari`", " from the project directory.")
	return []byte(output.String())
}

func renderProjectSessionClosed(style bool) []byte {
	var output strings.Builder
	fmt.Fprintln(&output, applyStyleToken(style, styleText, "Workspace session closed."))
	fmt.Fprintln(&output, applyStyleToken(style, styleText, "Workspace remains available."))
	output.WriteByte('\n')
	writeStyledCommandLine(&output, style, "Resume:", "", "tobari", "")
	writeStyledCommandLine(&output, style, "Remove:", "", "tobari delete", "")
	writeStyledCommandLine(&output, style, "If another session is attached:", "", "tobari delete --force", "")
	return []byte(output.String())
}

func renderPendingPolicyNotification(result tobari.PolicyCandidateReport, style bool) []byte {
	if len(result.Items) == 0 {
		return nil
	}
	latest := result.Items[len(result.Items)-1]
	var output strings.Builder
	var status strings.Builder
	fmt.Fprintf(&status, "⚠ %d pending network permission", len(result.Items))
	if len(result.Items) == 1 {
		status.WriteString(" is")
	} else {
		status.WriteString("s are")
	}
	status.WriteString(" waiting for review.")
	fmt.Fprintf(&output, "\n%s\n", applyStyleToken(style, styleWarning, status.String()))
	request := fmt.Sprintf(
		"Latest: %s:%d %s %s",
		safeExternalText(latest.Host), latest.Port, safeExternalText(latest.Method), safeExternalText(latest.Path),
	)
	fmt.Fprintln(&output, applyStyleToken(style, styleText, request))
	writeStyledCommandLine(&output, style, "Review on the host:", "", "tobari review permissions", "")
	return []byte(output.String())
}

func renderRuntimeCustomizationHint(style bool) []byte {
	return []byte(applyStyleToken(style, styleText, runtimeCustomizationHint()))
}

func runProjectStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.tobari.ProjectStatusInContext(ctx, inputs.One("--context"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help status", "Correct the command arguments.")
	}
	output, err := renderProjectStatusWithColor(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
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
	contextName := inputs.One("--context")
	expectedContextID := ""
	if force {
		preview, err := c.tobari.ProjectStatusInContext(ctx, contextName)
		if err != nil {
			return c.fail(ctx, err)
		}
		if preview.Exists && c.Err != nil {
			if humanStyleAllowed(ctx, c, c.Err) {
				previewOutput := newHumanOutput(true)
				previewOutput.heading("!", "Delete target", styleWarning)
				previewOutput.row("Context", safeExternalText(preview.ContextName), styleText)
				previewOutput.row("Context ID", preview.ContextID, styleText)
				previewOutput.row("Root", safeExternalText(preview.Root), styleText)
				previewOutput.row("Workspace ID", preview.ID, styleText)
				previewOutput.row("Session", safeExternalText(string(preview.Attachment)), humanStatusToken(string(preview.Attachment)))
				previewOutput.row("Home", safeExternalText(preview.Home), styleText)
				previewOutput.row("Removes", "owned runtime resources, persistent home, and tool-owned authentication state", styleWarning)
				previewOutput.row("Preserves", "mounted project root and its files", styleSuccess)
				_, _ = writeOnce(c.Err, previewOutput.bytes())
			} else {
				fmt.Fprintf(
					c.Err,
					"delete_target: context=%s\tcontext_id=%s\troot=%s\tid=%s\tattachment=%s\thome=%s\tremoves=owned_runtime,persistent_home,tool_auth\tpreserves=project_root\n",
					escapeTSVCell(preview.ContextName), preview.ContextID, escapeTSVCell(preview.Root), preview.ID,
					preview.Attachment, escapeTSVCell(preview.Home),
				)
			}
		}
		if preview.Exists {
			contextName = preview.ContextName
			expectedContextID = preview.ContextID
		}
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ID: tobari.CurrentDirectoryTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	result, err := c.tobari.DeleteProjectWithContextBinding(ctx, intent, contextName, expectedContextID, force)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderProjectDeleteWithColor(result, humanStyleAllowed(ctx, c, c.Out)))
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
	output, err := renderProjectListWithColor(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
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
	Policy        string               `json:"policy"`
	WindowLines   int                  `json:"window_lines"`
	UnparsedLines int                  `json:"unparsed_lines"`
	Items         []policyDenialOutput `json:"items"`
	ReviewCommand string               `json:"review_command"`
}

type policyDenialOutput struct {
	Timestamp            string `json:"timestamp"`
	RequestID            string `json:"request_id"`
	ContextID            string `json:"context_id"`
	Context              string `json:"context"`
	WorkspaceID          string `json:"workspace_id"`
	ProjectRoot          string `json:"project_root"`
	Scheme               string `json:"scheme"`
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
	Method               string `json:"method"`
	Path                 string `json:"path"`
	Protocol             string `json:"protocol"`
	StateChange          string `json:"state_change"`
	GraphQLOperationType string `json:"graphql_operation_type"`
	GraphQLRootField     string `json:"graphql_root_field"`
	MCPMethod            string `json:"mcp_method"`
	MCPToolName          string `json:"mcp_tool_name"`
	AWSWireProtocol      string `json:"aws_wire_protocol"`
	AWSService           string `json:"aws_service"`
	AWSOperation         string `json:"aws_operation"`
	KubernetesVerb       string `json:"kubernetes_verb"`
	KubernetesResource   string `json:"kubernetes_resource"`
	KubernetesDryRun     string `json:"kubernetes_dry_run"`
	GitService           string `json:"git_service"`
	GitRepository        string `json:"git_repository"`
	OCIAction            string `json:"oci_action"`
	OCIRepository        string `json:"oci_repository"`
	OCIObject            string `json:"oci_object"`
	Reason               string `json:"reason"`
	StatusCode           int    `json:"status_code"`
	Learnable            bool   `json:"learnable"`
	DestinationKind      string `json:"destination_kind"`
	AuthorityLifetime    string `json:"authority_lifetime"`
	AttachmentEpochID    string `json:"attachment_epoch_id"`
}

type policyCandidateOutput struct {
	ID                   string `json:"id"`
	ObservedAt           string `json:"observed_at"`
	ObservationCount     int    `json:"observation_count"`
	ContextID            string `json:"context_id"`
	Context              string `json:"context"`
	WorkspaceID          string `json:"workspace_id"`
	ProjectRoot          string `json:"project_root"`
	Scheme               string `json:"scheme"`
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
	Method               string `json:"method"`
	Path                 string `json:"path"`
	Protocol             string `json:"protocol"`
	StateChange          string `json:"state_change"`
	GraphQLOperationType string `json:"graphql_operation_type"`
	GraphQLRootField     string `json:"graphql_root_field"`
	MCPMethod            string `json:"mcp_method"`
	MCPToolName          string `json:"mcp_tool_name"`
	AWSWireProtocol      string `json:"aws_wire_protocol"`
	AWSService           string `json:"aws_service"`
	AWSOperation         string `json:"aws_operation"`
	KubernetesVerb       string `json:"kubernetes_verb"`
	KubernetesResource   string `json:"kubernetes_resource"`
	KubernetesDryRun     string `json:"kubernetes_dry_run"`
	GitService           string `json:"git_service"`
	GitRepository        string `json:"git_repository"`
	OCIAction            string `json:"oci_action"`
	OCIRepository        string `json:"oci_repository"`
	OCIObject            string `json:"oci_object"`
	Reason               string `json:"reason"`
	StatusCode           int    `json:"status_code"`
	AllowCommand         string `json:"allow_command"`
	DenyCommand          string `json:"deny_command"`
	DestinationKind      string `json:"destination_kind"`
	AuthorityLifetime    string `json:"authority_lifetime"`
	AttachmentEpochID    string `json:"attachment_epoch_id"`
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
	ID                   string   `json:"id"`
	Decision             string   `json:"decision"`
	Match                string   `json:"match"`
	ContextID            string   `json:"context_id"`
	Context              string   `json:"context"`
	WorkspaceID          string   `json:"workspace_id"`
	ProjectRoot          string   `json:"project_root"`
	Scheme               string   `json:"scheme"`
	Host                 string   `json:"host"`
	Port                 int      `json:"port"`
	Method               string   `json:"method"`
	Path                 string   `json:"path"`
	Protocol             string   `json:"protocol"`
	StateChange          string   `json:"state_change"`
	GraphQLOperationType string   `json:"graphql_operation_type"`
	GraphQLRootField     string   `json:"graphql_root_field"`
	MCPMethod            string   `json:"mcp_method"`
	MCPToolName          string   `json:"mcp_tool_name"`
	AWSWireProtocol      string   `json:"aws_wire_protocol"`
	AWSService           string   `json:"aws_service"`
	AWSOperation         string   `json:"aws_operation"`
	KubernetesVerb       string   `json:"kubernetes_verb"`
	KubernetesResource   string   `json:"kubernetes_resource"`
	KubernetesDryRun     string   `json:"kubernetes_dry_run"`
	GitService           string   `json:"git_service"`
	GitRepository        string   `json:"git_repository"`
	OCIAction            string   `json:"oci_action"`
	OCIRepository        string   `json:"oci_repository"`
	OCIObject            string   `json:"oci_object"`
	Examples             []string `json:"examples"`
	SourceCandidates     []string `json:"source_candidates"`
	ResetCommand         string   `json:"reset_command"`
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
		output, err := marshalCommandJSON("policy candidates", policyCandidatesDocument{
			SchemaVersion: 1, PolicyCandidates: items,
		})
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract, "output_encoding_failed",
				"policy candidates JSON could not be encoded", false, err,
			)
		}
		return append(output, '\n'), nil
	}
	if format == successFormatText {
		return renderPolicyCandidatesHuman(result, allowCommand, color), nil
	}
	var output bytes.Buffer
	for _, item := range result.Items {
		action := allowCommand + " --id " + item.ID
		fmt.Fprintf(
			&output,
			"id=%s\tobserved_at=%s\tobservation_count=%d\tcontext_id=%s\tcontext=%s\tworkspace_id=%s\tproject_root=%s\tscheme=%s\thost=%s\tport=%d\tmethod=%s\tpath=%s\treason=%s\tstatus_code=%d\tallow_command=%s\tdeny_command=%s\tprotocol=%s\tstate_change=%s\tgraphql_operation_type=%s\tgraphql_root_field=%s\tmcp_method=%s\tmcp_tool_name=%s\taws_wire_protocol=%s\taws_service=%s\taws_operation=%s\tkubernetes_verb=%s\tkubernetes_resource=%s\tkubernetes_dry_run=%s\tgit_service=%s\tgit_repository=%s\toci_action=%s\toci_repository=%s\toci_object=%s\tdestination_kind=%s\tauthority_lifetime=%s\tattachment_epoch_id=%s\n",
			item.ID, escapeTSVCell(item.ObservedAt), item.EffectiveObservationCount(), item.ContextID, escapeTSVCell(item.ContextName), item.ProjectID, escapeTSVCell(item.ProjectRoot), escapeTSVCell(item.Scheme),
			escapeTSVCell(item.Host), item.Port, escapeTSVCell(item.Method), escapeTSVCell(item.Path), escapeTSVCell(item.Reason),
			item.StatusCode, escapeTSVCell(action), escapeTSVCell(denyCommand+" --id "+item.ID),
			escapeTSVCell(item.EffectiveProtocol()), item.StateChangePotential(), escapeTSVCell(item.GraphQLOperationType), escapeTSVCell(item.GraphQLRootField), escapeTSVCell(item.MCPMethod), escapeTSVCell(item.MCPToolName),
			escapeTSVCell(item.AWSWireProtocol), escapeTSVCell(item.AWSService), escapeTSVCell(item.AWSOperation),
			escapeTSVCell(item.KubernetesVerb), escapeTSVCell(item.KubernetesResource), escapeTSVCell(item.KubernetesDryRun),
			escapeTSVCell(item.GitService), escapeTSVCell(item.GitRepository),
			escapeTSVCell(item.OCIAction), escapeTSVCell(item.OCIRepository), escapeTSVCell(item.OCIObject),
			item.EffectiveDestinationKind(), item.EffectiveAuthorityLifetime(), item.AttachmentEpochID,
		)
	}
	return semanticTextBytes(color, output.Bytes()), nil
}

func policyCandidateOutputs(
	result tobari.PolicyCandidateReport, allowCommand, denyCommand string,
) []policyCandidateOutput {
	items := make([]policyCandidateOutput, 0, len(result.Items))
	for _, item := range result.Items {
		allow := allowCommand + " --id " + item.ID
		deny := denyCommand + " --id " + item.ID
		if item.EffectiveDestinationKind() == tobari.PolicyDestinationHostLoopback {
			allow, deny = "", ""
		}
		items = append(items, policyCandidateOutput{
			ID: item.ID, ObservedAt: safeExternalText(item.ObservedAt), ObservationCount: item.EffectiveObservationCount(),
			ContextID: item.ContextID, Context: safeExternalText(item.ContextName),
			WorkspaceID: item.ProjectID, ProjectRoot: safeExternalText(item.ProjectRoot),
			Scheme: safeExternalText(item.Scheme), Host: safeExternalText(item.Host), Port: item.Port, Method: safeExternalText(item.Method),
			Path: safeExternalText(item.Path), Protocol: safeExternalText(item.EffectiveProtocol()), StateChange: item.StateChangePotential(),
			GraphQLOperationType: safeExternalText(item.GraphQLOperationType), GraphQLRootField: safeExternalText(item.GraphQLRootField),
			MCPMethod: safeExternalText(item.MCPMethod), MCPToolName: safeExternalText(item.MCPToolName),
			AWSWireProtocol: safeExternalText(item.AWSWireProtocol), AWSService: safeExternalText(item.AWSService), AWSOperation: safeExternalText(item.AWSOperation),
			KubernetesVerb: safeExternalText(item.KubernetesVerb), KubernetesResource: safeExternalText(item.KubernetesResource), KubernetesDryRun: safeExternalText(item.KubernetesDryRun),
			GitService: safeExternalText(item.GitService), GitRepository: safeExternalText(item.GitRepository),
			OCIAction: safeExternalText(item.OCIAction), OCIRepository: safeExternalText(item.OCIRepository), OCIObject: safeExternalText(item.OCIObject),
			Reason:          safeExternalText(item.Reason),
			StatusCode:      item.StatusCode,
			AllowCommand:    allow,
			DenyCommand:     deny,
			DestinationKind: item.EffectiveDestinationKind(), AuthorityLifetime: item.EffectiveAuthorityLifetime(),
			AttachmentEpochID: item.AttachmentEpochID,
		})
	}
	return items
}

func renderPolicyCandidatesHuman(result tobari.PolicyCandidateReport, allowCommand string, color bool) []byte {
	if len(result.Items) == 0 {
		output := newHumanOutput(color)
		output.heading("○", "No policy candidates", styleMuted)
		output.row("Policy", safeExternalText(result.PolicyDirectory), styleText)
		output.row("Window", fmt.Sprintf("%d Gateway lines", result.WindowLines), styleText)
		writeUnparsedDenialWarning(output, result.UnparsedLines)
		output.row("Details", "No retained denied request is ready for approval.", styleText)
		output.next("cluster denials", "Inspect the recent bounded denial evidence.")
		return output.bytes()
	}
	output := newHumanOutput(color)
	output.heading("✓", fmt.Sprintf("Policy candidates (%d)", len(result.Items)), styleSuccess)
	output.row("Policy", safeExternalText(result.PolicyDirectory), styleText)
	output.row("Window", fmt.Sprintf("%d lines", result.WindowLines), styleText)
	writeUnparsedDenialWarning(output, result.UnparsedLines)
	for index, item := range result.Items {
		output.section(fmt.Sprintf("Candidate %d", index+1))
		output.row("Context", safeExternalText(item.ContextName), styleText)
		output.row("Context ID", item.ContextID, styleText)
		output.row("Workspace", safeExternalText(item.ProjectRoot), styleText)
		request := fmt.Sprintf("%s://%s:%d %s %s", safeExternalText(item.Scheme), safeExternalText(item.Host), item.Port, safeExternalText(item.Method), safeExternalText(item.Path))
		output.row("Request", request, styleText)
		writePolicyGraphQLIdentity(output, item.PolicyProtocolIdentity)
		output.row("Candidate ID", item.ID, styleText)
		output.row("Workspace ID", safeExternalText(item.ProjectID), styleText)
		output.row("Protocol", safeExternalText(item.EffectiveProtocol()), styleText)
		output.row("Observed", policyCandidateObservationText(item), styleText)
		output.row("Latest", safeExternalText(item.ObservedAt), styleText)
		output.row("Reason", safeExternalText(item.Reason), styleDanger)
		output.row("Status", fmt.Sprintf("%d", item.StatusCode), styleDanger)
		output.row("Allow", allowCommand+" --id "+item.ID, styleAccent)
		output.row("Deny", pairedPolicyCommand(allowCommand, "allow", "deny")+" --id "+item.ID, styleAccent)
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
		return []byte("review permissions: output encoding failed\n")
	}
	return output
}

func renderPolicyReviewWithCommands(
	result tobari.PolicyCandidateReport, allowCommand, denyCommand string,
	format successFormat, color bool,
) ([]byte, error) {
	items := policyCandidateOutputs(result, allowCommand, denyCommand)
	if format == successFormatJSON {
		output, err := marshalCommandJSON("review permissions", policyReviewDocument{
			SchemaVersion: 1, PolicyReview: items,
		})
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract, "output_encoding_failed",
				"review permissions JSON could not be encoded", false, err,
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
		output.heading("○", "No pending network permissions", styleMuted)
		output.row("Policy", safeExternalText(result.PolicyDirectory), styleText)
		output.row("Window", fmt.Sprintf("%d Gateway lines", result.WindowLines), styleText)
		writeUnparsedDenialWarning(output, result.UnparsedLines)
		output.row("Details", "No retained exact permission is waiting for host review.", styleText)
		return output.bytes()
	}
	output := newHumanOutput(color)
	output.heading("⚠", fmt.Sprintf("Pending network permissions (%d)", len(result.Items)), styleWarning)
	output.row("Window", fmt.Sprintf("%d Gateway lines", result.WindowLines), styleText)
	writeUnparsedDenialWarning(output, result.UnparsedLines)
	for index, item := range result.Items {
		output.section(fmt.Sprintf("Permission %d", index+1))
		output.row("Context", safeExternalText(item.ContextName), styleText)
		output.row("Workspace", safeExternalText(item.ProjectRoot), styleText)
		request := fmt.Sprintf("%s://%s:%d %s %s", safeExternalText(item.Scheme), safeExternalText(item.Host), item.Port, safeExternalText(item.Method), safeExternalText(item.Path))
		output.row("Request", request, styleText)
		writePolicyGraphQLIdentity(output, item.PolicyProtocolIdentity)
		output.row("Observed", policyCandidateObservationText(item), styleText)
		output.row("Latest", safeExternalText(item.ObservedAt), styleText)
		output.row("Reason", safeExternalText(item.Reason), styleDanger)
		output.row("Status", fmt.Sprintf("%d", item.StatusCode), styleDanger)
		if item.EffectiveDestinationKind() == tobari.PolicyDestinationHostLoopback {
			output.row("Authority", "Host Loopback · attachment-scoped · Workspace audience", styleText)
			output.row("Decision", "Run review permissions in an interactive host terminal.", styleAccent)
		} else {
			output.row("Allow exact", allowCommand+" --id "+item.ID, styleAccent)
			output.row("Deny exact", denyCommand+" --id "+item.ID, styleAccent)
		}
	}
	return output.bytes()
}

func policyCandidateObservationText(candidate tobari.PolicyCandidate) string {
	count := candidate.EffectiveObservationCount()
	return fmt.Sprintf("%d time%s", count, pluralSuffix(count))
}

func policyGraphQLCoordinate(identity tobari.PolicyProtocolIdentity) string {
	if identity.EffectiveProtocol() != tobari.PolicyProtocolGraphQL {
		return ""
	}
	return safeExternalText(identity.GraphQLOperationType) + "." + safeExternalText(identity.GraphQLRootField)
}

func policyAWSCoordinate(identity tobari.PolicyProtocolIdentity) string {
	if identity.EffectiveProtocol() != tobari.PolicyProtocolAWS {
		return ""
	}
	return safeExternalText(identity.AWSService) + "/" + safeExternalText(identity.AWSOperation)
}

func writePolicyGraphQLIdentity(output *humanOutput, identity tobari.PolicyProtocolIdentity) {
	output.row("State change", safeExternalText(identity.StateChangePotential()), styleText)
	if coordinate := policyGraphQLCoordinate(identity); coordinate != "" {
		output.row("GraphQL", coordinate, styleText)
	}
	if identity.EffectiveProtocol() == tobari.PolicyProtocolMCP {
		coordinate := safeExternalText(identity.MCPMethod)
		if identity.MCPToolName != "" {
			coordinate += " · " + safeExternalText(identity.MCPToolName)
		}
		output.row("MCP", coordinate, styleText)
	}
	if coordinate := policyAWSCoordinate(identity); coordinate != "" {
		output.row("AWS operation", coordinate, styleText)
		output.row("AWS protocol", safeExternalText(identity.AWSWireProtocol), styleText)
	}
	if identity.EffectiveProtocol() == tobari.PolicyProtocolKubernetes {
		output.row("Kubernetes", safeExternalText(identity.KubernetesVerb+" "+identity.KubernetesResource), styleText)
		output.row("Dry run", safeExternalText(identity.KubernetesDryRun), styleText)
	}
	if identity.EffectiveProtocol() == tobari.PolicyProtocolGit {
		output.row("Git", safeExternalText(identity.GitService+" "+identity.GitRepository), styleText)
	}
	if identity.EffectiveProtocol() == tobari.PolicyProtocolOCI {
		output.row("OCI", safeExternalText(identity.OCIAction+" "+identity.OCIRepository+" "+identity.OCIObject), styleText)
	}
}

func renderPolicyReviewChange(result tobari.PolicyReviewChange, color bool) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, applyStyleToken(color, styleSuccess, "✓ Reviewed permissions applied"))
	fmt.Fprintf(&output, "  Revision  %s\n", result.ActiveRevision)
	fmt.Fprintf(&output, "  Decisions %d (%d Allow, %d Deny)\n", len(result.Decisions), result.AllowCount, result.DenyCount)
	for index, decision := range result.Decisions {
		fmt.Fprintln(&output)
		fmt.Fprintf(&output, "%d. %s\n", index+1, policyReviewDecisionLabel(decision))
		fmt.Fprintf(&output, "   Context   %s · %s\n", safeExternalText(decision.ContextName), decision.ContextID)
		fmt.Fprintf(&output, "   Workspace %s · %s\n", safeExternalText(decision.ProjectRoot), decision.ProjectID)
		fmt.Fprintf(&output, "   Effect    %s\n", policyReviewAppliedEffect(decision))
		fmt.Fprintf(&output, "   Rule      %s\n", decision.RuleID)
		fmt.Fprintf(&output, "   Review    %s\n", decision.ReviewItemID)
	}
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, applyStyleToken(color, styleMuted, "Next: ")+applyStyleToken(
		color, styleText, "keep the current Workspace and agent session running; retry the blocked request there now.",
	))
	return output.Bytes()
}

func policyReviewDecisionLabel(decision tobari.PolicyReviewAppliedDecision) string {
	if decision.Decision == tobari.PolicyDecisionDeny {
		return "Deny exact"
	}
	if decision.Match == tobari.PolicyMatchPathTemplate {
		return "Allow template"
	}
	return "Allow exact"
}

func policyReviewAppliedEffect(decision tobari.PolicyReviewAppliedDecision) string {
	effect := fmt.Sprintf(
		"%s %s://%s:%d%s", safeExternalText(decision.Method), safeExternalText(decision.Scheme), safeExternalText(decision.Host),
		decision.Port, safeExternalText(decision.Path),
	)
	if coordinate := policyGraphQLCoordinate(decision.PolicyProtocolIdentity); coordinate != "" {
		effect += " · GraphQL " + coordinate
	}
	if decision.EffectiveProtocol() == tobari.PolicyProtocolAWS {
		effect += " · AWS " + safeExternalText(decision.AWSService) + "/" + safeExternalText(decision.AWSOperation)
	}
	if decision.EffectiveProtocol() == tobari.PolicyProtocolKubernetes {
		effect += " · Kubernetes " + safeExternalText(decision.KubernetesVerb) + " " + safeExternalText(decision.KubernetesResource)
		if decision.KubernetesDryRun == "all" {
			effect += " · dry-run"
		}
	}
	if decision.EffectiveProtocol() == tobari.PolicyProtocolGit {
		effect += " · Git " + safeExternalText(decision.GitService) + " " + safeExternalText(decision.GitRepository)
	}
	if decision.EffectiveProtocol() == tobari.PolicyProtocolOCI {
		effect += " · OCI " + safeExternalText(decision.OCIAction) + " " + safeExternalText(decision.OCIRepository) + " " + safeExternalText(decision.OCIObject)
	}
	return effect
}

func renderPolicyRulesWithCommands(
	result tobari.PolicyRuleReport, resetCommand string, format successFormat, color bool,
) ([]byte, error) {
	items := policyRuleOutputs(result, resetCommand)
	if format == successFormatJSON {
		output, err := marshalCommandJSON("policy rules", policyRulesDocument{SchemaVersion: 1, PolicyRules: items})
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract, "output_encoding_failed",
				"policy rules JSON could not be encoded", false, err,
			)
		}
		return append(output, '\n'), nil
	}
	if format == successFormatText {
		return renderPolicyRulesHuman(result, resetCommand, color), nil
	}
	var output bytes.Buffer
	for _, item := range items {
		fmt.Fprintf(
			&output,
			"id=%s\tdecision=%s\tmatch=%s\tcontext_id=%s\tcontext=%s\tworkspace_id=%s\tproject_root=%s\tscheme=%s\thost=%s\tport=%d\tmethod=%s\tpath=%s\texamples=%s\tsource_candidates=%s\treset_command=%s\tprotocol=%s\tstate_change=%s\tgraphql_operation_type=%s\tgraphql_root_field=%s\tmcp_method=%s\tmcp_tool_name=%s\taws_wire_protocol=%s\taws_service=%s\taws_operation=%s\tkubernetes_verb=%s\tkubernetes_resource=%s\tkubernetes_dry_run=%s\tgit_service=%s\tgit_repository=%s\toci_action=%s\toci_repository=%s\toci_object=%s\n",
			item.ID, item.Decision, item.Match, item.ContextID, escapeTSVCell(item.Context), item.WorkspaceID, escapeTSVCell(item.ProjectRoot), escapeTSVCell(item.Scheme), escapeTSVCell(item.Host), item.Port,
			escapeTSVCell(item.Method), escapeTSVCell(item.Path), escapeTSVCell(strings.Join(item.Examples, ",")),
			escapeTSVCell(strings.Join(item.SourceCandidates, ",")), escapeTSVCell(item.ResetCommand), escapeTSVCell(item.Protocol),
			item.StateChange, escapeTSVCell(item.GraphQLOperationType), escapeTSVCell(item.GraphQLRootField), escapeTSVCell(item.MCPMethod), escapeTSVCell(item.MCPToolName),
			escapeTSVCell(item.AWSWireProtocol), escapeTSVCell(item.AWSService), escapeTSVCell(item.AWSOperation),
			escapeTSVCell(item.KubernetesVerb), escapeTSVCell(item.KubernetesResource), escapeTSVCell(item.KubernetesDryRun),
			escapeTSVCell(item.GitService), escapeTSVCell(item.GitRepository),
			escapeTSVCell(item.OCIAction), escapeTSVCell(item.OCIRepository), escapeTSVCell(item.OCIObject),
		)
	}
	return semanticTextBytes(color, output.Bytes()), nil
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
			ContextID: rule.ContextID, Context: safeExternalText(rule.ContextName),
			WorkspaceID: rule.ProjectID, ProjectRoot: safeExternalText(rule.ProjectRoot), Scheme: safeExternalText(rule.Scheme), Host: safeExternalText(rule.Host), Port: rule.Port,
			Method: safeExternalText(rule.Method), Path: safeExternalText(rule.Path), Protocol: safeExternalText(rule.EffectiveProtocol()), StateChange: rule.StateChangePotential(),
			GraphQLOperationType: safeExternalText(rule.GraphQLOperationType), GraphQLRootField: safeExternalText(rule.GraphQLRootField),
			MCPMethod: safeExternalText(rule.MCPMethod), MCPToolName: safeExternalText(rule.MCPToolName),
			AWSWireProtocol: safeExternalText(rule.AWSWireProtocol), AWSService: safeExternalText(rule.AWSService), AWSOperation: safeExternalText(rule.AWSOperation),
			KubernetesVerb: safeExternalText(rule.KubernetesVerb), KubernetesResource: safeExternalText(rule.KubernetesResource), KubernetesDryRun: safeExternalText(rule.KubernetesDryRun),
			GitService: safeExternalText(rule.GitService), GitRepository: safeExternalText(rule.GitRepository),
			OCIAction: safeExternalText(rule.OCIAction), OCIRepository: safeExternalText(rule.OCIRepository), OCIObject: safeExternalText(rule.OCIObject),
			Examples: examples, SourceCandidates: append([]string{}, rule.SourceCandidates...),
			ResetCommand: resetCommand + " --id " + rule.ID,
		})
	}
	return items
}

func renderPolicyRulesHuman(result tobari.PolicyRuleReport, resetCommand string, color bool) []byte {
	if len(result.Items) == 0 {
		output := newHumanOutput(color)
		output.heading("○", "No learned policy decisions", styleMuted)
		output.row("Policy", safeExternalText(result.PolicyDirectory), styleText)
		output.row("Details", "No current Allow or exact Deny decision is active.", styleText)
		output.next("review permissions", "Review retained denied permissions when one needs a decision.")
		return output.bytes()
	}
	output := newHumanOutput(color)
	output.heading("✓", fmt.Sprintf("Learned policy decisions (%d)", len(result.Items)), styleSuccess)
	output.row("Policy", safeExternalText(result.PolicyDirectory), styleText)
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
		output.sectionWithToken(fmt.Sprintf("%s (%d)", title, count), policyRuleDecisionToken(decision))
		for _, item := range result.Items {
			if item.Decision != decision {
				continue
			}
			output.row("Request", policyRuleRequest(item), styleText)
			writePolicyGraphQLIdentity(output, item.PolicyProtocolIdentity)
			output.row("Context", safeExternalText(item.ContextName), styleText)
			output.row("Context ID", item.ContextID, styleText)
			output.row("Workspace", safeExternalText(item.ProjectRoot), styleText)
			output.row("Rule ID", item.ID, styleText)
			output.row("Match", safeExternalText(item.Match), styleText)
			output.row("Workspace ID", safeExternalText(item.ProjectID), styleText)
			output.row("Protocol", safeExternalText(item.EffectiveProtocol()), styleText)
			if len(item.Examples) > 0 {
				values := make([]string, len(item.Examples))
				for index, value := range item.Examples {
					values[index] = safeExternalText(value)
				}
				output.row("Examples", strings.Join(values, ", "), styleText)
			}
			if len(item.SourceCandidates) > 0 {
				output.row("Source IDs", strings.Join(item.SourceCandidates, ", "), styleText)
			}
			output.row("Reset", resetCommand+" --id "+item.ID, styleAccent)
		}
	}
	return output.bytes()
}

func renderPolicyRuleResetWithColor(result tobari.PolicyRuleReset, color bool) []byte {
	output := newHumanOutput(color)
	output.heading("✓", "Policy decision reset", styleSuccess)
	output.row("Policy", safeExternalText(result.PolicyDirectory), styleText)
	output.row("Target", result.TargetID, styleText)
	output.row("Removed", safeExternalText(result.Decision), styleText)
	output.row("Default deny", humanBool(result.Applied), humanOutcomeBoolToken(result.Applied))
	output.next("review permissions", "Review the retained denied effect again before granting a new decision.")
	return output.bytes()
}

func renderPolicyReviewAllowSuccess(result tobari.PolicyLearningChange, color bool) []byte {
	output := newHumanOutput(color)
	output.heading("✓", "Permission allowed", styleSuccess)
	output.row("Testing policy", "passed", styleSuccess)
	output.row("Applying exact rule", "applied", styleSuccess)
	output.row("Context", safeExternalText(result.Rule.ContextName), styleText)
	output.row("Workspace", safeExternalText(result.Rule.ProjectRoot), styleText)
	output.row("Request", fmt.Sprintf(
		"%s://%s:%d %s %s", safeExternalText(result.Rule.Scheme), safeExternalText(result.Rule.Host), result.Rule.Port,
		safeExternalText(result.Rule.Method), safeExternalText(result.Rule.Path),
	), styleText)
	writePolicyGraphQLIdentity(output, result.Rule.PolicyProtocolIdentity)
	output.row("Next", "retry the same request in the current running Workspace", styleText)
	return output.bytes()
}

func renderPolicyLearningChange(result tobari.PolicyLearningChange) []byte {
	return renderPolicyLearningChangeWithColor(result, false)
}

func renderPolicyLearningChangeWithColor(result tobari.PolicyLearningChange, color bool) []byte {
	output := newHumanOutput(color)
	marker, title, token := "✓", "Policy rule updated", styleSuccess // #nosec G101 -- human-readable status text contains no credential.
	if !result.Applied {
		marker, title, token = "!", "Policy rule recorded", styleWarning // #nosec G101 -- human-readable status text contains no credential.
	}
	output.heading(marker, title, token)
	output.row("Policy", safeExternalText(result.PolicyDirectory), styleText)
	output.row("Target ID", result.TargetID, styleText)
	output.row("Rule ID", result.Rule.ID, styleText)
	output.row("Context", safeExternalText(result.Rule.ContextName), styleText)
	output.row("Context ID", result.Rule.ContextID, styleText)
	output.row("Workspace", safeExternalText(result.Rule.ProjectRoot), styleText)
	output.row("Match", safeExternalText(result.Rule.Match), styleText)
	output.row("Request", fmt.Sprintf("%s://%s:%d %s %s", safeExternalText(result.Rule.Scheme), safeExternalText(result.Rule.Host), result.Rule.Port, safeExternalText(result.Rule.Method), safeExternalText(result.Rule.Path)), styleText)
	writePolicyGraphQLIdentity(output, result.Rule.PolicyProtocolIdentity)
	output.row("Workspace ID", safeExternalText(result.Rule.ProjectID), styleText)
	output.row("Protocol", safeExternalText(result.Rule.EffectiveProtocol()), styleText)
	output.row("Source rules", fmt.Sprintf("%d", result.SourceRuleCount), styleText)
	output.row("Applied", humanBool(result.Applied), humanOutcomeBoolToken(result.Applied))
	output.row("Next", "retry the same request in the current running Workspace", styleText)
	return output.bytes()
}

func renderPolicyDenyChangeWithColor(result tobari.PolicyDenyChange, color bool) []byte {
	output := newHumanOutput(color)
	output.heading("✓", "Permission denied", styleSuccess)
	output.row("Context", safeExternalText(result.Rule.ContextName), styleText)
	output.row("Workspace", safeExternalText(result.Rule.ProjectRoot), styleText)
	output.row("Policy", safeExternalText(result.PolicyDirectory), styleText)
	output.row("Target ID", result.TargetID, styleText)
	output.row("Rule ID", result.Rule.ID, styleText)
	output.row("Context ID", result.Rule.ContextID, styleText)
	output.row("Request", fmt.Sprintf(
		"%s://%s:%d %s %s", safeExternalText(result.Rule.Scheme), safeExternalText(result.Rule.Host), result.Rule.Port,
		safeExternalText(result.Rule.Method), safeExternalText(result.Rule.Path),
	), styleText)
	writePolicyGraphQLIdentity(output, result.Rule.PolicyProtocolIdentity)
	output.row("Workspace ID", safeExternalText(result.Rule.ProjectID), styleText)
	output.row("Protocol", safeExternalText(result.Rule.EffectiveProtocol()), styleText)
	output.row("Source rules", fmt.Sprintf("%d", result.SourceRuleCount), styleText)
	output.row("Applied", humanBool(result.Applied), humanOutcomeBoolToken(result.Applied))
	output.next("policy rules", "Inspect the active exact Deny decision.")
	return output.bytes()
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
		items := make([]policyDenialOutput, 0, len(result.Items))
		for _, item := range result.Items {
			items = append(items, policyDenialOutput{
				Timestamp: safeExternalText(item.Timestamp), RequestID: safeExternalText(item.RequestID),
				ContextID: item.ContextID, Context: safeExternalText(item.ContextName),
				WorkspaceID: item.ProjectID, ProjectRoot: safeExternalText(item.ProjectRoot),
				Scheme: safeExternalText(item.Scheme), Host: safeExternalText(item.Host), Port: item.Port, Method: safeExternalText(item.Method), Path: safeExternalText(item.Path),
				Protocol: safeExternalText(item.EffectiveProtocol()), StateChange: item.StateChangePotential(), GraphQLOperationType: safeExternalText(item.GraphQLOperationType),
				GraphQLRootField: safeExternalText(item.GraphQLRootField), MCPMethod: safeExternalText(item.MCPMethod), MCPToolName: safeExternalText(item.MCPToolName), Reason: safeExternalText(item.Reason), StatusCode: item.StatusCode,
				AWSWireProtocol: safeExternalText(item.AWSWireProtocol), AWSService: safeExternalText(item.AWSService), AWSOperation: safeExternalText(item.AWSOperation),
				KubernetesVerb: safeExternalText(item.KubernetesVerb), KubernetesResource: safeExternalText(item.KubernetesResource), KubernetesDryRun: safeExternalText(item.KubernetesDryRun),
				GitService: safeExternalText(item.GitService), GitRepository: safeExternalText(item.GitRepository),
				OCIAction: safeExternalText(item.OCIAction), OCIRepository: safeExternalText(item.OCIRepository), OCIObject: safeExternalText(item.OCIObject),
				Learnable:       item.Learnable,
				DestinationKind: item.EffectiveDestinationKind(), AuthorityLifetime: item.EffectiveAuthorityLifetime(),
				AttachmentEpochID: item.AttachmentEpochID,
			})
		}
		output, err := marshalCommandJSON("cluster denials", clusterDenialsDocument{
			SchemaVersion: 1,
			Denials: clusterDenialsOutput{
				Policy: safeExternalText(result.PolicyDirectory), WindowLines: result.WindowLines,
				UnparsedLines: result.UnparsedLines, Items: items, ReviewCommand: reviewCommand,
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
	if format == successFormatText {
		return renderClusterDenialsHuman(result, reviewCommand, color), nil
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "policy: %s\n", escapeTSVCell(result.PolicyDirectory))
	fmt.Fprintf(&output, "window_lines: %d\n", result.WindowLines)
	fmt.Fprintf(&output, "unparsed_lines: %d\n", result.UnparsedLines)
	fmt.Fprintf(&output, "denial_count: %d\n", len(result.Items))
	for _, item := range result.Items {
		fmt.Fprintf(
			&output,
			"denial: timestamp=%s\trequest_id=%s\tcontext=%s\tcontext_id=%s\tworkspace_id=%s\tproject_root=%s\tscheme=%s\thost=%s\tport=%d\tmethod=%s\tpath=%s\tstatus_code=%d\treason=%s\tprotocol=%s\tstate_change=%s\tgraphql_operation_type=%s\tgraphql_root_field=%s\tmcp_method=%s\tmcp_tool_name=%s\taws_wire_protocol=%s\taws_service=%s\taws_operation=%s\tkubernetes_verb=%s\tkubernetes_resource=%s\tkubernetes_dry_run=%s\tgit_service=%s\tgit_repository=%s\toci_action=%s\toci_repository=%s\toci_object=%s\tdestination_kind=%s\tauthority_lifetime=%s\tattachment_epoch_id=%s\n",
			escapeTSVCell(item.Timestamp), escapeTSVCell(item.RequestID),
			escapeTSVCell(item.ContextName), item.ContextID, item.ProjectID, escapeTSVCell(item.ProjectRoot),
			escapeTSVCell(item.Scheme), escapeTSVCell(item.Host), item.Port, escapeTSVCell(item.Method),
			escapeTSVCell(item.Path), item.StatusCode, escapeTSVCell(item.Reason), escapeTSVCell(item.EffectiveProtocol()),
			item.StateChangePotential(), escapeTSVCell(item.GraphQLOperationType), escapeTSVCell(item.GraphQLRootField), escapeTSVCell(item.MCPMethod), escapeTSVCell(item.MCPToolName),
			escapeTSVCell(item.AWSWireProtocol), escapeTSVCell(item.AWSService), escapeTSVCell(item.AWSOperation),
			escapeTSVCell(item.KubernetesVerb), escapeTSVCell(item.KubernetesResource), escapeTSVCell(item.KubernetesDryRun),
			escapeTSVCell(item.GitService), escapeTSVCell(item.GitRepository),
			escapeTSVCell(item.OCIAction), escapeTSVCell(item.OCIRepository), escapeTSVCell(item.OCIObject),
			item.EffectiveDestinationKind(), item.EffectiveAuthorityLifetime(), item.AttachmentEpochID,
		)
	}
	fmt.Fprintf(&output, "review_command: %s\n", escapeTSVCell(reviewCommand))
	return semanticTextBytes(color, output.Bytes()), nil
}

func renderClusterDenialsHuman(result tobari.DenialReport, reviewCommand string, color bool) []byte {
	if len(result.Items) == 0 {
		output := newHumanOutput(color)
		output.heading("○", "No policy denials", styleMuted)
		output.row("Policy", safeExternalText(result.PolicyDirectory), styleText)
		output.row("Window", fmt.Sprintf("%d Gateway lines", result.WindowLines), styleText)
		writeUnparsedDenialWarning(output, result.UnparsedLines)
		output.row("Details", "The selected Gateway log window contains no denied requests.", styleText)
		output.next("policy candidates", "Check whether a new denied request has been retained.")
		return output.bytes()
	}
	output := newHumanOutput(color)
	output.heading("!", fmt.Sprintf("Policy denials (%d)", len(result.Items)), styleDanger)
	output.row("Policy", safeExternalText(result.PolicyDirectory), styleText)
	output.row("Window", fmt.Sprintf("%d lines", result.WindowLines), styleText)
	writeUnparsedDenialWarning(output, result.UnparsedLines)
	output.row("Review", reviewCommand, styleAccent)
	for index, item := range result.Items {
		output.section(fmt.Sprintf("Denial %d", index+1))
		output.row("Context", safeExternalText(item.ContextName), styleText)
		output.row("Context ID", item.ContextID, styleText)
		output.row("Workspace", safeExternalText(item.ProjectRoot), styleText)
		output.row("Request", fmt.Sprintf("%s://%s:%d %s %s", safeExternalText(item.Scheme), safeExternalText(item.Host), item.Port, safeExternalText(item.Method), safeExternalText(item.Path)), styleText)
		writePolicyGraphQLIdentity(output, item.PolicyProtocolIdentity)
		output.row("Timestamp", safeExternalText(item.Timestamp), styleText)
		output.row("Request ID", item.RequestID, styleText)
		output.row("Workspace ID", safeExternalText(item.ProjectID), styleText)
		output.row("Protocol", safeExternalText(item.EffectiveProtocol()), styleText)
		output.row("Status", fmt.Sprintf("%d", item.StatusCode), styleDanger)
		output.row("Reason", safeExternalText(item.Reason), styleDanger)
		learnable := "no"
		if item.Learnable {
			learnable = "yes"
		}
		output.row("Learnable", learnable, styleText)
	}
	return output.bytes()
}

func writeUnparsedDenialWarning(output *humanOutput, count int) {
	if count > 0 {
		output.row("Unparsed", fmt.Sprintf("%d denial-shaped Gateway line%s skipped", count, pluralSuffix(count)), styleWarning)
	}
}

type clusterStatusOutput struct {
	Configured               bool                     `json:"configured"`
	Running                  bool                     `json:"running"`
	Policy                   *string                  `json:"policy"`
	WorkspaceCount           int                      `json:"workspace_count"`
	ContextCount             int                      `json:"context_count"`
	PolicyRevision           *string                  `json:"policy_revision"`
	PolicyProjection         string                   `json:"policy_projection"`
	PrincipalRegistry        string                   `json:"principal_registry"`
	GatewayProjection        string                   `json:"gateway_projection"`
	AuthProviderProjection   string                   `json:"auth_provider_projection,omitempty"`
	AuthBrokerState          string                   `json:"auth_broker_state,omitempty"`
	CredentialCompanionState string                   `json:"credential_companion_state,omitempty"`
	RootKeyBackend           string                   `json:"root_key_backend,omitempty"`
	Components               []tobari.ComponentStatus `json:"components"`
	RecentError              *string                  `json:"recent_error"`
}

func renderClusterStatus(status tobari.ClusterStatus, format successFormat, color bool) ([]byte, error) {
	if err := status.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_status_contract", "cluster status is invalid", false, err)
	}
	if format == successFormatJSON {
		projection := clusterStatusOutput{
			Configured: status.Configured, Running: status.Running,
			Policy:         optionalExternalText(status.Policy),
			WorkspaceCount: status.TobariCount, ContextCount: status.ContextCount,
			PolicyRevision: optionalString(status.PolicyRevision), PolicyProjection: safeExternalText(status.PolicyProjection), PrincipalRegistry: safeExternalText(status.PrincipalRegistry),
			GatewayProjection: safeExternalText(status.GatewayProjection),
			Components:        append([]tobari.ComponentStatus{}, status.Components...),
			RecentError:       optionalExternalText(status.RecentError),
		}
		if buildIdentityHasBroker() {
			projection.AuthProviderProjection = safeExternalText(status.AuthProviderProjection)
			projection.AuthBrokerState = safeExternalText(status.AuthBrokerState)
			projection.CredentialCompanionState = safeExternalText(status.CredentialCompanionState)
			projection.RootKeyBackend = safeExternalText(status.RootKeyBackend)
		}
		document := clusterStatusDocument{
			SchemaVersion: 1,
			Cluster:       projection,
		}
		output, err := marshalCommandJSON("cluster status", document)
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "cluster status JSON could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}
	return renderClusterStatusTextWithColor(status, color), nil
}

func optionalExternalText(value string) *string {
	if value == "" {
		return nil
	}
	projected := safeExternalText(value)
	return &projected
}

func renderClusterStatusText(status tobari.ClusterStatus) []byte {
	return renderClusterStatusTextWithColor(status, false)
}

func renderClusterDownTextWithColor(status tobari.ClusterStatus, purge, color bool) []byte {
	if status.Task != tobari.TaskClusterDown || status.Configured || !purge {
		return renderClusterStatusTextWithColor(status, color)
	}
	output := newHumanOutput(color)
	output.heading("✓", "Cluster removed", styleSuccess)
	output.row("Removed", "shared CA volumes and active policy-bundle volume", styleText)
	output.row("Preserved", "encrypted Context vaults and installation root key", styleText)
	return output.bytes()
}

func renderClusterUpText(status tobari.ClusterStatus, color bool) []byte {
	output := renderClusterStatusTextWithColor(status, color)
	if !status.Configured || !status.Running {
		return output
	}
	var withNext bytes.Buffer
	withNext.Write(output)
	withNext.WriteByte('\n')
	writeStyledCommandLine(&withNext, color, "Next:", "from a project directory, run ", "`tobari`", ".")
	return withNext.Bytes()
}

func renderClusterStatusTextWithColor(status tobari.ClusterStatus, color bool) []byte {
	var output bytes.Buffer
	marker, heading, headingToken := clusterStatusHeading(status)
	fmt.Fprintf(
		&output, "%s %s\n",
		applyStyleToken(color, headingToken, marker),
		applyStyleToken(color, styleText, heading),
	)
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
		applyStyleToken(color, styleMuted, fmt.Sprintf("%-8s", "Workspaces")),
		status.TobariCount,
	)
	fmt.Fprintf(
		&output, "  %s %d\n",
		applyStyleToken(color, styleMuted, fmt.Sprintf("%-8s", "Contexts")),
		status.ContextCount,
	)
	if status.PolicyRevision != "" {
		fmt.Fprintf(&output, "  %s %s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("%-8s", "Revision")), status.PolicyRevision[:12])
		integrity := fmt.Sprintf("policy %s / principals %s / gateway %s", safeExternalText(status.PolicyProjection), safeExternalText(status.PrincipalRegistry), safeExternalText(status.GatewayProjection))
		if status.AuthProviderProjection != "" {
			integrity += " / providers " + safeExternalText(status.AuthProviderProjection)
		}
		fmt.Fprintf(&output, "  %s %s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("%-8s", "Integrity")), integrity)
	}
	if status.AuthBrokerState != "" || status.CredentialCompanionState != "" || status.RootKeyBackend != "" {
		fmt.Fprintf(&output, "  %s broker %s / companion %s / root key %s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("%-8s", "Auth")), safeExternalText(status.AuthBrokerState), safeExternalText(status.CredentialCompanionState), safeExternalText(status.RootKeyBackend))
	}
	if status.Policy != "" {
		fmt.Fprintln(&output)
		fmt.Fprintf(
			&output, "  %s %s\n",
			applyStyleToken(color, styleMuted, fmt.Sprintf("%-8s", "Policy")),
			applyStyleToken(color, styleText, escapeTSVCell(status.Policy)),
		)
	}
	renderClusterRecentError(&output, status.RecentError, color)
	return output.Bytes()
}

func clusterStatusHeading(status tobari.ClusterStatus) (string, string, styleToken) {
	switch {
	case status.Task == tobari.TaskClusterDown && !status.Configured:
		return "✓", "Cluster removed", styleSuccess
	case !status.Configured:
		return "○", "Cluster not configured", styleMuted
	case !status.Running:
		return "!", "Cluster not ready", styleWarning
	default:
		return "✓", "Cluster ready", styleSuccess
	}
}

func renderClusterComponent(output *bytes.Buffer, component tobari.ComponentStatus, ready, color bool) {
	name := clusterComponentName(component.Name)
	health := escapeTSVCell(component.Health)
	healthOutput := applyStyleToken(color, clusterHealthStyleToken(component.Health), health)
	if ready && component.Health == "healthy" {
		fmt.Fprintf(output, "  %s %s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("%-8s", name)), healthOutput)
		return
	}
	state := applyStyleToken(color, humanStatusToken(component.State), escapeTSVCell(component.State))
	fmt.Fprintf(output, "  %s %s · %s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("%-8s", name)), state, healthOutput)
}

func clusterComponentName(name string) string {
	switch strings.ToLower(name) {
	case "gateway":
		return "Gateway"
	case "opa":
		return "OPA"
	case "auth-broker":
		return "Auth"
	default:
		return escapeTSVCell(name)
	}
}

func clusterHealthStyleToken(health string) styleToken {
	switch strings.ToLower(health) {
	case "healthy":
		return styleSuccess
	case "starting", "pending", "unknown":
		return styleWarning
	case "unhealthy", "exited", "dead", "failed":
		return styleDanger
	default:
		return styleMuted
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
		applyStyleToken(color, styleMuted, "Recent error"),
		applyStyleToken(color, styleDanger, escapeTSVCell(recentError)),
	)
}

type projectStatusOutput struct {
	ContextState  tobari.ContextObservationState `json:"context_state"`
	Exists        bool                           `json:"exists"`
	ProjectRoot   string                         `json:"project_root"`
	WorkspaceID   string                         `json:"workspace_id"`
	WorkspaceHome string                         `json:"workspace_home"`
	Context       string                         `json:"context"`
	ContextID     *string                        `json:"context_id"`
	Runtime       string                         `json:"runtime"`
	Attachment    string                         `json:"attachment"`
	Bootstrap     projectBootstrapStatusOutput   `json:"bootstrap"`
	NextArgv      []string                       `json:"next_argv"`
}

type projectBootstrapStatusOutput struct {
	State           string `json:"state"`
	AppliedRevision string `json:"applied_revision"`
	CurrentRevision string `json:"current_revision"`
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
	nextArgv := []string{ProgramName}
	if result.ContextState != tobari.ContextObservationSyntheticDefault {
		nextArgv = append(nextArgv, "--context", result.ContextName)
	}
	bootstrap := result.Bootstrap.Resolved()
	value := projectStatusOutput{
		ContextState: result.ContextState, Exists: result.Exists, ProjectRoot: safeExternalText(result.Root), WorkspaceID: result.ID,
		WorkspaceHome: safeExternalText(result.Home), Context: safeExternalText(result.ContextName), ContextID: optionalString(result.ContextID),
		Runtime: string(result.Runtime), Attachment: string(result.Attachment),
		Bootstrap: projectBootstrapStatusOutput{State: bootstrap.State, AppliedRevision: bootstrap.AppliedRevision, CurrentRevision: bootstrap.CurrentRevision},
		NextArgv:  nextArgv,
	}
	nextCommand := strings.Join(value.NextArgv, " ")
	nextRecovery := ProgramName
	if len(value.NextArgv) > 1 {
		nextRecovery = strings.Join(value.NextArgv[1:], " ")
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON("status", projectStatusDocument{SchemaVersion: 1, Status: value})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "project status JSON could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}
	if format == successFormatText {
		summary, err := result.RoutineSummary()
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "invalid_status_contract", "Workspace routine summary is invalid", false, err)
		}
		if !result.Exists {
			output := newHumanOutput(color)
			output.heading("○", "No Workspace", styleMuted)
			output.row("Context", safeExternalText(result.ContextName), styleText)
			if result.ContextState == tobari.ContextObservationSyntheticDefault {
				output.row("Defaults", "Recommended · not saved", styleWarning)
			}
			output.next(nextRecovery, "Create or enter a Workspace in this Context.")
			return output.bytes(), nil
		}
		output := newHumanOutput(color)
		marker, title, token := "✓", "Workspace ready", styleSuccess
		if result.Runtime != tobari.RuntimeDiagnosticReady {
			marker, title, token = "!", "Workspace needs attention", styleWarning
		}
		output.heading(marker, title, token)
		output.row("Root", safeExternalText(result.Root), styleText)
		output.row("Context", safeExternalText(result.ContextName), styleText)
		runtimeValue := safeExternalText(string(result.Runtime))
		if summary.RuntimeSelection != "" {
			runtimeValue = safeExternalText(summary.RuntimeSelection) + " · " + runtimeValue
		}
		output.row("Runtime", runtimeValue, humanStatusToken(string(result.Runtime)))
		output.row("Session", safeExternalText(string(result.Attachment)), humanStatusToken(string(result.Attachment)))
		if summary.BootstrapAttention {
			output.row("Workspace defaults", humanWorkspaceBootstrapAttention(bootstrap.State), styleWarning)
		}
		if summary.Action == tobari.ProjectRoutineActionInspect {
			output.row("Action", "Inspect the local runtime", styleWarning)
			output.next("doctor", "Inspect the local runtime before entering the project.")
		} else {
			output.next(nextRecovery, "Enter the current directory's Workspace.")
		}
		return output.bytes(), nil
	}
	if !result.Exists {
		return []byte(fmt.Sprintf(
			"No Workspace exists for the current directory in Context %s\nNext: %s\n",
			escapeTSVCell(result.ContextName), nextCommand,
		)), nil
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "Workspace exists at %s\n", escapeTSVCell(result.Root))
	fmt.Fprintf(&output, "Context: %s\n", escapeTSVCell(result.ContextName))
	fmt.Fprintf(&output, "Runtime: %s\n", escapeTSVCell(string(result.Runtime)))
	fmt.Fprintf(&output, "Session: %s\n", escapeTSVCell(string(result.Attachment)))
	fmt.Fprintf(&output, "Bootstrap: %s\n", escapeTSVCell(value.Bootstrap.State))
	fmt.Fprintf(&output, "Next: %s\n", nextCommand)
	return semanticTextBytes(color, output.Bytes()), nil
}

func humanWorkspaceBootstrapAttention(state string) string {
	if state == tobari.WorkspaceBootstrapNotApplied {
		return "Not applied to this existing Workspace"
	}
	return "Older creation defaults retained"
}

type projectListOutput struct {
	ProjectRoot string `json:"project_root"`
	Context     string `json:"context"`
	ContextID   string `json:"context_id"`
	Runtime     string `json:"runtime"`
	WorkspaceID string `json:"workspace_id"`
}

type projectListDocument struct {
	SchemaVersion int                 `json:"schema_version"`
	Workspaces    []projectListOutput `json:"workspaces"`
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
			ProjectRoot: safeExternalText(item.Root), Context: safeExternalText(item.ContextName), ContextID: item.ContextID,
			Runtime: string(item.Runtime), WorkspaceID: item.ID,
		})
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON("list", projectListDocument{SchemaVersion: 1, Workspaces: items})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "project list JSON could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}
	if format == successFormatText {
		if len(items) == 0 {
			empty := newHumanOutput(color)
			empty.empty("No Workspaces", "No Workspace state is configured.", "tobari", "Create or enter a Workspace from the current directory.")
			return empty.bytes(), nil
		}
		output := newHumanOutput(color)
		output.heading("✓", fmt.Sprintf("Workspaces (%d)", len(items)), styleSuccess)
		for _, item := range items {
			marker := "  "
			if item.WorkspaceID == result.CurrentID {
				marker = "▸ "
			}
			output.sectionWithToken(marker+item.ProjectRoot, styleText)
			output.row("Context", item.Context, styleText)
			output.row("Runtime", item.Runtime, humanStatusToken(item.Runtime))
			output.row("Workspace ID", item.WorkspaceID, styleText)
		}
		return output.bytes(), nil
	}
	var output bytes.Buffer
	fmt.Fprintln(&output, "PROJECT_ROOT\tCONTEXT\tRUNTIME\tWORKSPACE_ID")
	for _, item := range items {
		fmt.Fprintf(&output, "%s\t%s\t%s\t%s\n", escapeTSVCell(item.ProjectRoot), escapeTSVCell(item.Context), item.Runtime, item.WorkspaceID)
	}
	return semanticTextBytes(color, output.Bytes()), nil
}

func renderProjectDelete(result tobari.ProjectDeleteResult) []byte {
	return renderProjectDeleteWithColor(result, false)
}

func renderProjectDeleteWithColor(result tobari.ProjectDeleteResult, color bool) []byte {
	output := newHumanOutput(color)
	marker, title, token := "✓", "Workspace deleted", styleSuccess
	if !result.Deleted {
		marker, title, token = "!", "Workspace not deleted", styleWarning // #nosec G101 -- human-readable status text contains no credential.
	}
	output.heading(marker, title, token)
	output.row("Deleted", humanBool(result.Deleted), humanOutcomeBoolToken(result.Deleted))
	output.row("Project root", safeExternalText(result.Root), styleText)
	output.row("Context", safeExternalText(result.ContextName), styleText)
	output.row("Context ID", result.ContextID, styleText)
	output.row("Workspace ID", result.ID, styleText)
	output.row("Workspace home", safeExternalText(result.Home), styleText)
	if result.Deleted {
		output.next("tobari", "Create or enter a Workspace from this project directory.")
	}
	return output.bytes()
}

func renderSafeLogs(raw []byte, style bool) []byte {
	var output strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		output.WriteString(safeExternalText(line))
		output.WriteByte('\n')
	}
	return semanticTextBytes(style, []byte(output.String()))
}

func missingRuntimeFault() *fault.Error {
	return fault.New(
		fault.KindInternal, "missing_runtime", "Tobari runtime is not configured.", false,
		fault.NextAction{Command: "doctor", Reason: "Inspect local runtime configuration."},
	)
}
