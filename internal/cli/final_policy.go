package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func finalPolicyOutput(path, envelope string, value any, format successFormat, text []byte) ([]byte, error) {
	if format == successFormatJSON {
		encoded, err := marshalCommandJSON(path, map[string]any{"schema_version": tobari.WorkspaceAuthorityPolicyReadSchemaVersion, envelope: value})
		if err != nil {
			return nil, err
		}
		return append(encoded, '\n'), nil
	}
	return text, nil
}

func runFinalPolicyCandidates(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalPolicy == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.finalPolicy.Candidates(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	return emitFinalPolicyCandidates(ctx, c, command, inputs, result, "policy_candidates")
}

func runFinalPolicyReview(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalPolicy == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	snapshot, err := c.finalPolicy.ReviewSnapshot(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	if format == successFormatText && policyReviewInteractiveAllowed(ctx, c) {
		return runFinalPolicyReviewInteractive(ctx, c, snapshot)
	}
	candidates, candidateErr := tobari.NewPolicyCandidateAuthorityList(snapshot.Collection, snapshot.CollectionPresent)
	if candidateErr != nil {
		return c.fail(ctx, fault.Wrap(fault.KindContract, "invalid_policy_review_snapshot", "Permission Inbox snapshot is invalid", false, candidateErr))
	}
	return emitFinalPolicyCandidatesWithFormat(ctx, c, command, candidates, "policy_review", format)
}

func runFinalPolicyReviewInteractive(ctx context.Context, c *CLI, snapshot tobari.PolicyMemoryReviewSnapshot) int {
	reader := bufio.NewReader(c.In)
	staged := map[string]tobari.PolicyMemoryDecision{}
	selected := ""
	for {
		if err := writeFinalPolicyReviewFrame(c.Out, snapshot, selected, staged); err != nil {
			return c.fail(ctx, fault.Wrap(fault.KindInternal, "output_write_failed", "Permission Inbox could not be written", false, err))
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return c.fail(ctx, fault.Wrap(fault.KindInternal, "terminal_input_failed", "Permission Inbox input could not be read", false, err))
		}
		choice := strings.TrimSpace(line)
		if choice == "" && err == io.EOF {
			return c.fail(ctx, context.Canceled)
		}
		switch choice {
		case "q":
			return c.fail(ctx, context.Canceled)
		case "r":
			fresh, refreshErr := c.finalPolicy.ReviewSnapshot(ctx)
			if refreshErr != nil {
				return c.fail(ctx, refreshErr)
			}
			removed := len(staged)
			snapshot, selected, staged = fresh, "", map[string]tobari.PolicyMemoryDecision{}
			if removed > 0 {
				_, _ = fmt.Fprintf(c.Out, "%d stale staged decision removed after refresh.\n", removed)
			}
		case "a", "d":
			item, found := finalPolicyReviewItemByID(snapshot, selected)
			if !found {
				_, _ = io.WriteString(c.Out, "Select one Permission Inbox item first.\n")
				continue
			}
			decision := tobari.PolicyMemoryAllow
			if choice == "d" {
				decision = tobari.PolicyMemoryDeny
			}
			if _, decisionErr := item.ReviewedDecision(decision); decisionErr != nil {
				_, _ = io.WriteString(c.Out, "That decision is not available for the selected item.\n")
				continue
			}
			staged[item.ID] = decision
		case "p":
			if len(staged) == 0 {
				_, _ = io.WriteString(c.Out, "No reviewed decisions are staged.\n")
				continue
			}
			_, _ = io.WriteString(c.Out, "Apply this complete reviewed set? [y/N]\n")
			confirmation, confirmErr := reader.ReadString('\n')
			if confirmErr != nil && confirmErr != io.EOF {
				return c.fail(ctx, fault.Wrap(fault.KindInternal, "terminal_input_failed", "Permission Inbox confirmation could not be read", false, confirmErr))
			}
			if strings.TrimSpace(confirmation) != "y" {
				continue
			}
			set, setErr := snapshot.ReviewedSet(staged)
			if setErr != nil {
				return c.fail(ctx, fault.Wrap(fault.KindContract, "invalid_policy_review_set", "Reviewed Permission Inbox set is invalid", false, setErr))
			}
			apply, found := c.catalog.lookupRegistered("policy apply-reviewed")
			if !found || apply.Agent.Mutation == nil || apply.Agent.FixedTarget == nil {
				return c.fail(ctx, fault.New(fault.KindContract, "invalid_catalog", "reviewed Policy Memory Apply contract is missing", false))
			}
			intent := operation.Intent{Command: apply.Path, Effect: apply.Effect, Target: operation.TargetRef{Kind: apply.Agent.FixedTarget.Kind, ParentID: apply.Agent.FixedTarget.ID}, Impact: apply.Agent.Mutation.Impact}
			actionCtx := withCommandPath(ctx, apply.Path)
			publication, applyErr := c.finalPolicy.ApplyReviewed(actionCtx, intent, set)
			if applyErr != nil {
				return c.fail(actionCtx, applyErr)
			}
			return emitFinalPolicyReviewedPublication(actionCtx, c, apply, publication, successFormatText)
		default:
			index, numberErr := strconv.Atoi(choice)
			if numberErr == nil && index > 0 && index <= len(snapshot.Items) {
				selected = snapshot.Items[index-1].ID
			}
		}
	}
}

func writeFinalPolicyReviewFrame(out io.Writer, snapshot tobari.PolicyMemoryReviewSnapshot, selected string, staged map[string]tobari.PolicyMemoryDecision) error {
	var text strings.Builder
	text.WriteString("Permission Inbox\n")
	if len(snapshot.Items) == 0 {
		text.WriteString("  No final Policy Memory candidates.\n")
	}
	for index, item := range snapshot.Items {
		marker := " "
		if item.ID == selected {
			marker = ">"
		}
		decision := ""
		if value, found := staged[item.ID]; found {
			decision = " [" + string(value) + "]"
		}
		fmt.Fprintf(&text, "%s %d  %s  %s  %s:%d%s%s\n", marker, index+1, item.Match, safeExternalText(item.Template), safeExternalText(item.Rule.Host), item.Rule.Port, safeExternalText(item.Rule.Path), decision)
	}
	text.WriteString("Commands: number select · a allow · d deny · r refresh · p apply · q cancel\n")
	_, err := io.WriteString(out, text.String())
	return err
}

func finalPolicyReviewItemByID(snapshot tobari.PolicyMemoryReviewSnapshot, id string) (tobari.PolicyMemoryReviewItem, bool) {
	for _, item := range snapshot.Items {
		if item.ID == id {
			return item, true
		}
	}
	return tobari.PolicyMemoryReviewItem{}, false
}

func emitFinalPolicyCandidates(ctx context.Context, c *CLI, command CommandSpec, inputs ParsedInputs, result tobari.PolicyCandidateAuthorityList, envelope string) int {
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	return emitFinalPolicyCandidatesWithFormat(ctx, c, command, result, envelope, format)
}

func emitFinalPolicyCandidatesWithFormat(ctx context.Context, c *CLI, command CommandSpec, result tobari.PolicyCandidateAuthorityList, envelope string, format successFormat) int {
	var text strings.Builder
	if len(result.Items) == 0 {
		text.WriteString("No final Policy Memory candidates.\n")
	} else {
		for _, item := range result.Items {
			fmt.Fprintf(&text, "%s  %s  %s  %s:%d%s\n", item.ID, safeExternalText(item.TemplateName), safeExternalText(item.ProjectRoot), safeExternalText(item.Effect.Host), item.Effect.Port, safeExternalText(item.Effect.Path))
		}
	}
	output, err := finalPolicyOutput(command.Path, envelope, result.Items, format, []byte(text.String()))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runFinalPolicyRules(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalPolicy == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.finalPolicy.Rules(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	var text strings.Builder
	if len(result.Items) == 0 {
		text.WriteString("No final Policy Memory rules.\n")
	} else {
		for _, item := range result.Items {
			fmt.Fprintf(&text, "%s  %s  %s  %s  %s:%d%s\n", item.ID, item.Decision, safeExternalText(item.TemplateName), safeExternalText(item.ProjectRoot), safeExternalText(item.Body.Host), item.Body.Port, safeExternalText(item.Body.Path))
		}
	}
	output, err := finalPolicyOutput(command.Path, "policy_rules", result.Items, format, []byte(text.String()))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

type finalPolicyDirectResult struct {
	Task           string                      `json:"task"`
	Decision       tobari.PolicyMemoryDecision `json:"decision"`
	Applied        bool                        `json:"applied"`
	ActiveRevision string                      `json:"active_revision"`
}

func runFinalPolicyAllow(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	return runFinalPolicyCandidateMutation(ctx, c, command, inputs, tobari.PolicyMemoryAllow)
}

func runFinalPolicyDeny(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	return runFinalPolicyCandidateMutation(ctx, c, command, inputs, tobari.PolicyMemoryDeny)
}

func runFinalPolicyCandidateMutation(ctx context.Context, c *CLI, command CommandSpec, inputs ParsedInputs, decision tobari.PolicyMemoryDecision) int {
	if c == nil || c.finalPolicy == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	var publication tobari.PolicyCandidateDecisionPublication
	var err error
	candidateRef := inputs.One("--id")
	intent := operation.Intent{Command: command.Path, Effect: command.Effect, Target: operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: candidateRef}, Impact: command.Agent.Mutation.Impact}
	if decision == tobari.PolicyMemoryAllow {
		publication, err = c.finalPolicy.Allow(ctx, intent, candidateRef)
	} else {
		publication, err = c.finalPolicy.Deny(ctx, intent, candidateRef)
	}
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	result := finalPolicyDirectResult{Task: command.Path, Decision: decision, Applied: true, ActiveRevision: publication.ActiveRevision()}
	text := []byte(fmt.Sprintf("%s applied.\nActive revision %s\n", safeExternalText(command.Summary), result.ActiveRevision))
	output, err := finalPolicyOutput(command.Path, "result", result, format, text)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

type finalPolicyResetResult struct {
	Task           string `json:"task"`
	Removed        bool   `json:"removed"`
	ActiveRevision string `json:"active_revision"`
}

func runFinalPolicyReset(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil || c.finalPolicy == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	ruleRef := inputs.One("--id")
	intent := operation.Intent{Command: command.Path, Effect: command.Effect, Target: operation.TargetRef{Kind: tobari.PolicyRuleKind, ID: ruleRef}, Impact: command.Agent.Mutation.Impact}
	publication, err := c.finalPolicy.Reset(ctx, intent, ruleRef)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, code, ok := finalFormat(ctx, c, command, inputs)
	if !ok {
		return code
	}
	result := finalPolicyResetResult{Task: command.Path, Removed: true, ActiveRevision: string(publication.Memory.Snapshot.PolicyMemory.Revision)}
	output, err := finalPolicyOutput(command.Path, "result", result, format, []byte(fmt.Sprintf("Policy Memory rule removed.\nActive revision %s\n", result.ActiveRevision)))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runFinalPolicyApplyReviewed(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
	return c.fail(ctx, fault.New(fault.KindUnavailable, "final_policy_review_unavailable", "No complete final reviewed decision set crossed the Permission Inbox boundary.", false, fault.NextAction{Command: "review permissions", Reason: "Reopen the final Permission Inbox after its complete-set owner is configured."}))
}

func emitFinalPolicyReviewedPublication(ctx context.Context, c *CLI, command CommandSpec, publication tobari.PolicyMemoryReviewedSetPublication, format successFormat) int {
	result, resultErr := tobari.NewPolicyMemoryReviewedResult(publication)
	if resultErr != nil {
		return c.fail(ctx, fault.Wrap(fault.KindContract, "invalid_policy_memory_result", "Reviewed Policy Memory result is invalid", false, resultErr))
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Reviewed Policy Memory applied.\nActive revision %s\n", publication.ActiveRevision)
	for _, decision := range publication.AppliedDecisions {
		fmt.Fprintf(&text, "%s  %s  %s  %s\n", decision.ReviewItemID, decision.RuleID, decision.Decision, decision.Match)
	}
	output, err := finalPolicyOutput(command.Path, "result", result, format, []byte(text.String()))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}
