package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

func finalPolicyOutput(path, envelope string, value any, format successFormat, text []byte) ([]byte, error) {
	if format == successFormatJSON {
		projected, err := finalPolicyJSONProjection(value)
		if err != nil {
			return nil, err
		}
		encoded, err := marshalCommandJSON(path, map[string]any{"schema_version": tobari.WorkspaceAuthorityPolicyReadSchemaVersion, envelope: projected})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Final Policy Memory JSON could not be encoded.", false, err)
		}
		return append(encoded, '\n'), nil
	}
	return text, nil
}

func finalPolicyJSONProjection(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var projected any
	if err := json.Unmarshal(encoded, &projected); err != nil {
		return nil, err
	}
	var complete func(any)
	complete = func(node any) {
		switch typed := node.(type) {
		case []any:
			for _, item := range typed {
				complete(item)
			}
		case map[string]any:
			for _, item := range typed {
				complete(item)
			}
			protocol, _ := typed["protocol"].(string)
			if protocol == tobari.PolicyProtocolKubernetes && typed["kubernetes_kind"] == tobari.KubernetesRequestResource {
				if _, present := typed["kubernetes_group"]; !present {
					typed["kubernetes_group"] = ""
				}
			}
			if protocol == tobari.PolicyProtocolOCI && typed["oci_action"] == "list" && typed["oci_object"] == "catalog" {
				if _, present := typed["oci_repository"]; !present {
					typed["oci_repository"] = ""
				}
			}
		}
	}
	complete(projected)
	return projected, nil
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
	candidates, candidateErr := snapshot.CandidateList()
	if candidateErr != nil {
		return c.fail(ctx, fault.WithClassification(
			fault.Wrap(fault.KindContract, "invalid_policy_review_snapshot", "Permission Inbox snapshot is invalid", false, candidateErr),
			fault.PhaseVerification, fault.ChangeUnknown,
		))
	}
	return emitFinalPolicyCandidatesWithFormat(ctx, c, command, candidates, "policy_review", format)
}

func runFinalPolicyReviewInteractive(ctx context.Context, c *CLI, snapshot tobari.PolicyMemoryReviewSnapshot) int {
	mode := terminal.New()
	if restore, rawErr := mode.Enter(c.In); rawErr == nil {
		result, selectErr := selectFinalPolicyReviewRaw(ctx, snapshot, c.In, c.Out, humanStyleAllowed(ctx, c, c.Out))
		finishErr := finishSelectorScreen(c.Out, result.lines)
		restoreErr := restore()
		if restoreErr != nil {
			restoreErr = fault.WithClassification(fault.Wrap(fault.KindInternal, "terminal_restore_failed", "Permission Inbox terminal state could not be restored", false, restoreErr), fault.PhasePresentation, fault.ChangeNotApplicable)
		}
		if interactionErr := finalPolicyReviewInteractionError(selectErr, finishErr, restoreErr); interactionErr != nil {
			return c.fail(ctx, interactionErr)
		}
		switch result.kind {
		case finalPolicyReviewRawCancel:
			return c.fail(ctx, context.Canceled)
		case finalPolicyReviewRawRefresh:
			fresh, refreshErr := c.finalPolicy.ReviewSnapshot(ctx)
			if refreshErr != nil {
				return c.fail(ctx, refreshErr)
			}
			return runFinalPolicyReviewInteractive(ctx, c, fresh)
		case finalPolicyReviewRawApply:
			return applyFinalPolicyReviewSet(ctx, c, snapshot, result.staged)
		default:
			return c.fail(ctx, fault.WithClassification(fault.New(fault.KindInternal, "invalid_policy_review_result", "Permission Inbox returned an invalid interaction result", false), fault.PhaseVerification, fault.ChangeUnknown))
		}
	}
	return runFinalPolicyReviewInteractiveLine(ctx, c, snapshot)
}

func finalPolicyReviewInteractionError(values ...error) error {
	for _, value := range values {
		if public, ok := fault.PublicCopy(value); ok {
			return public
		}
	}
	combined := errors.Join(values...)
	if combined == nil {
		return nil
	}
	if errors.Is(combined, context.Canceled) || errors.Is(combined, context.DeadlineExceeded) {
		allCancellation := true
		for _, value := range values {
			if value != nil && !errors.Is(value, context.Canceled) && !errors.Is(value, context.DeadlineExceeded) {
				allCancellation = false
				break
			}
		}
		if allCancellation {
			return combined
		}
	}
	return fault.WithClassification(
		fault.Wrap(fault.KindInternal, "policy_review_terminal_failed", "Permission Inbox terminal interaction failed", false, combined),
		fault.PhasePresentation,
		fault.ChangeNotApplicable,
	)
}

func runFinalPolicyReviewInteractiveLine(ctx context.Context, c *CLI, snapshot tobari.PolicyMemoryReviewSnapshot) int {
	reader := bufio.NewReader(c.In)
	staged := map[string]tobari.PolicyMemoryDecision{}
	selected := ""
	for {
		if err := writeFinalPolicyReviewFrame(c.Out, snapshot, selected, staged); err != nil {
			return c.fail(ctx, fault.Wrap(fault.KindInternal, "output_write_failed", "Permission Inbox could not be written", false, err))
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return c.fail(ctx, fault.WithClassification(fault.Wrap(fault.KindInternal, "terminal_input_failed", "Permission Inbox input could not be read", false, err), fault.PhasePresentation, fault.ChangeNotApplicable))
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
			if item.AttachmentCandidate == nil {
				if _, decisionErr := item.ReviewedDecision(decision); decisionErr != nil {
					_, _ = io.WriteString(c.Out, "That decision is not available for the selected item.\n")
					continue
				}
			} else if decision.Validate() != nil {
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
				return c.fail(ctx, fault.WithClassification(fault.Wrap(fault.KindInternal, "terminal_input_failed", "Permission Inbox confirmation could not be read", false, confirmErr), fault.PhasePresentation, fault.ChangeNotApplicable))
			}
			if strings.TrimSpace(confirmation) != "y" {
				continue
			}
			return applyFinalPolicyReviewSet(ctx, c, snapshot, staged)
		default:
			index, numberErr := strconv.Atoi(choice)
			if numberErr == nil && index > 0 && index <= len(snapshot.Items) {
				selected = snapshot.Items[index-1].ID
			}
		}
	}
}

func applyFinalPolicyReviewSet(ctx context.Context, c *CLI, snapshot tobari.PolicyMemoryReviewSnapshot, staged map[string]tobari.PolicyMemoryDecision) int {
	for id, decision := range staged {
		item, found := finalPolicyReviewItemByID(snapshot, id)
		if found && item.AttachmentCandidate != nil {
			if len(staged) != 1 {
				return c.fail(ctx, fault.WithClassification(fault.New(fault.KindRejected, "policy_review_scope_mixed", "one Apply cannot mix persistent and attachment-scoped decisions", false, fault.NextAction{Command: "review permissions", Reason: "Apply attachment-scoped decisions separately."}), fault.PhasePrecondition, fault.ChangeNone))
			}
			return runFinalPolicyReviewAttachment(ctx, c, item.ID, decision)
		}
	}
	set, setErr := snapshot.ReviewedSet(staged)
	if setErr != nil {
		return c.fail(ctx, fault.WithClassification(fault.Wrap(fault.KindContract, "invalid_policy_review_set", "Reviewed Permission Inbox set is invalid", false, setErr), fault.PhaseVerification, fault.ChangeNone))
	}
	apply, found := c.catalog.lookupRegistered("policy apply-reviewed")
	if !found || apply.Agent.Mutation == nil || apply.Agent.FixedTarget == nil {
		return c.fail(ctx, fault.WithClassification(fault.New(fault.KindContract, "invalid_catalog", "reviewed Policy Memory Apply contract is missing", false), fault.PhasePrecondition, fault.ChangeNone))
	}
	intent := operation.Intent{Command: apply.Path, Effect: apply.Effect, Target: operation.TargetRef{Kind: apply.Agent.FixedTarget.Kind, ParentID: apply.Agent.FixedTarget.ID}, Impact: apply.Agent.Mutation.Impact}
	actionCtx := withCommandPath(ctx, apply.Path)
	progress := newInteractiveWorkProgress(c.Err, "Applying reviewed permissions", terminal.IsTerminal(c.Err), humanStyleAllowed(actionCtx, c, c.Err))
	progress.Start()
	publication, applyErr := c.finalPolicy.ApplyReviewed(actionCtx, intent, set)
	progress.Stop()
	if applyErr != nil {
		return c.fail(actionCtx, applyErr)
	}
	return emitFinalPolicyReviewedPublication(actionCtx, c, apply, publication, successFormatText)
}

func runFinalPolicyReviewAttachment(ctx context.Context, c *CLI, candidateRef string, decision tobari.PolicyMemoryDecision) int {
	path := "policy allow"
	if decision == tobari.PolicyMemoryDeny {
		path = "policy deny"
	}
	command, found := c.catalog.lookupRegistered(path)
	if !found || command.Agent.Mutation == nil {
		return c.fail(ctx, fault.New(fault.KindContract, "invalid_catalog", "attachment Policy Apply contract is missing", false))
	}
	actionCtx := withCommandPath(ctx, command.Path)
	intent := operation.Intent{Command: command.Path, Effect: command.Effect, Target: operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: candidateRef}, Impact: command.Agent.Mutation.Impact}
	var publication tobari.PolicyCandidateDecisionPublication
	var err error
	if decision == tobari.PolicyMemoryAllow {
		publication, err = c.finalPolicy.Allow(actionCtx, intent, candidateRef)
	} else {
		publication, err = c.finalPolicy.Deny(actionCtx, intent, candidateRef)
	}
	if err != nil {
		return c.fail(actionCtx, err)
	}
	result := finalPolicyDirectResult{Task: command.Path, Decision: decision, Applied: true, ActiveRevision: publication.ActiveRevision()}
	text := finalPolicyDirectText(decision, result.ActiveRevision, humanStyleAllowed(actionCtx, c, c.Out))
	output, err := finalPolicyOutput(command.Path, "result", result, successFormatText, text)
	if err != nil {
		return c.fail(actionCtx, err)
	}
	return c.emitMutationResult(actionCtx, command, output)
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
		fmt.Fprintf(&text, "%s %d  %s  %s  %s%s\n", marker, index+1, item.Match, safeExternalText(item.Template), finalPolicyEffectSummary(item.Rule.PolicyProtocolIdentity, item.Rule.Method, item.Rule.Host, item.Rule.Port, item.Rule.Path), decision)
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
	text := finalPolicyCandidatesText(result, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	output, err := finalPolicyOutput(command.Path, envelope, result.Items, format, text)
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
	text := finalPolicyRulesText(result, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	output, err := finalPolicyOutput(command.Path, "policy_rules", result.Items, format, text)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func finalPolicyEffectSummary(identity tobari.PolicyProtocolIdentity, method, host string, port int, path string) string {
	result := fmt.Sprintf("%s %s://%s:%d%s", safeExternalText(method), safeExternalText(identity.Scheme), safeExternalText(host), port, safeExternalText(path))
	if coordinate := policyGraphQLCoordinate(identity); coordinate != "" {
		return result + " · GraphQL " + coordinate
	}
	if identity.EffectiveProtocol() == tobari.PolicyProtocolMCP {
		coordinate := safeExternalText(identity.MCPMethod)
		if identity.MCPToolName != "" {
			coordinate += " " + safeExternalText(identity.MCPToolName)
		}
		return result + " · MCP " + coordinate
	}
	if coordinate := policyAWSCoordinate(identity); coordinate != "" {
		return result + " · AWS " + coordinate
	}
	if coordinate := policyKubernetesCoordinate(identity); coordinate != "" {
		return result + " · Kubernetes " + coordinate + " dry-run=" + safeExternalText(identity.KubernetesDryRun)
	}
	if identity.EffectiveProtocol() == tobari.PolicyProtocolGit {
		return result + " · Git " + safeExternalText(identity.GitService+" "+identity.GitRepository)
	}
	if identity.EffectiveProtocol() == tobari.PolicyProtocolOCI {
		return result + " · OCI " + safeExternalText(identity.OCIAction+" "+identity.OCIRepository+" "+identity.OCIObject)
	}
	return result
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
	text := finalPolicyDirectText(decision, result.ActiveRevision, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
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
	text := newHumanOutput(format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	text.heading("✓", "Policy rule removed", styleSuccess)
	text.row("Rule", ruleRef, styleText)
	text.row("Active revision", result.ActiveRevision, styleText)
	output, err := finalPolicyOutput(command.Path, "result", result, format, text.bytes())
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
	text := newHumanOutput(format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	text.heading("✓", "Reviewed permissions applied", styleSuccess)
	text.row("Active revision", publication.ActiveRevision, styleText)
	text.row("Decisions", fmt.Sprintf("%d applied", len(publication.AppliedDecisions)), styleSuccess)
	for _, decision := range publication.AppliedDecisions {
		text.row(string(decision.Decision), decision.ReviewItemID+" · "+decision.RuleID+" · "+string(decision.Match), humanStatusToken(string(decision.Decision)))
	}
	contextRef := ""
	for _, decision := range publication.AppliedDecisions {
		if contextRef == "" {
			contextRef = decision.ContextRef
			continue
		}
		if contextRef != decision.ContextRef {
			contextRef = ""
			break
		}
	}
	if contextRef != "" {
		text.next("policy assist --context "+contextRef, "Use reviewed Policy Memory as evidence for a reusable static Template policy.")
	} else {
		text.next("context list", "Choose the exact Context whose reviewed Policy Memory should inform a reusable static Template policy.")
	}
	output, err := finalPolicyOutput(command.Path, "result", result, format, text.bytes())
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func finalPolicyDirectText(decision tobari.PolicyMemoryDecision, revision string, color bool) []byte {
	output := newHumanOutput(color)
	title, token := "Permission allowed", styleSuccess
	if decision == tobari.PolicyMemoryDeny {
		title, token = "Permission denied", styleWarning
	}
	output.heading("✓", title, token)
	output.row("Decision", string(decision), token)
	output.row("Active revision", revision, styleText)
	return output.bytes()
}

func finalPolicyCandidatesText(result tobari.PolicyCandidateAuthorityList, color bool) []byte {
	output := newHumanOutput(color)
	if len(result.Items) == 0 {
		output.empty("No permissions need review", "", "", "")
		return output.bytes()
	}
	output.section("Permission candidates")
	for _, item := range result.Items {
		output.subsection(safeExternalText(item.TemplateName))
		output.nestedRow("ID", item.ID, styleText)
		output.nestedRow("Request", finalPolicyEffectSummary(item.Effect.PolicyProtocolIdentity, item.Effect.Method, item.Effect.Host, item.Effect.Port, item.Effect.Path), styleWarning)
	}
	output.next("review permissions", "Inspect and decide these requests interactively.")
	return output.bytes()
}

func finalPolicyRulesText(result tobari.PolicyMemoryRuleList, color bool) []byte {
	output := newHumanOutput(color)
	if len(result.Items) == 0 {
		output.empty("No remembered permissions", "", "", "")
		return output.bytes()
	}
	output.section("Remembered permissions")
	for _, item := range result.Items {
		output.subsection(safeExternalText(item.TemplateName))
		output.nestedRow("Decision", string(item.Decision), humanStatusToken(string(item.Decision)))
		output.nestedRow("Rule", item.ID, styleText)
		output.nestedRow("Request", finalPolicyEffectSummary(item.Body.PolicyProtocolIdentity, item.Body.Method, item.Body.Host, item.Body.Port, item.Body.Path), styleText)
	}
	return output.bytes()
}
