package tobaricmd

import (
	"context"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type policyDecisionSetRuntimePort interface {
	ApplyPolicyDecisionSet(
		context.Context, tobari.State,
		[]tobari.LearnedPolicyRule, []tobari.LearnedPolicyRule,
		[]tobari.PolicyDenyRule, []tobari.PolicyDenyRule,
	) (tobari.PolicyActivationReceipt, error)
}

type attachmentGrantDecisionSetRuntimePort interface {
	ApplyAttachmentGrantDecisionSet(context.Context, []tobari.AttachmentGrant) (tobari.PolicyActivationReceipt, error)
}

// ApplyPolicyReviewDecisionSet revalidates every staged opaque candidate
// against fresh retained evidence, then records and activates the complete set
// through one command-owned installation policy target.
func (s *Service) ApplyPolicyReviewDecisionSet(
	ctx context.Context, intent operation.Intent, set tobari.PolicyReviewDecisionSet,
) (tobari.PolicyReviewChange, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyReviewChange{}, err
	}
	if err := set.Validate(); err != nil {
		return tobari.PolicyReviewChange{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_policy_review_set",
			"reviewed policy decisions are invalid", false, err,
		)
	}
	if err := validatePolicyMutationTarget(intent, tobari.PolicyDecisionSetKind, tobari.PolicyDecisionSetID); err != nil {
		return tobari.PolicyReviewChange{}, err
	}
	runtime, ok := s.runtime.(policyDecisionSetRuntimePort)
	if !ok || portcheck.IsNil(runtime) {
		return tobari.PolicyReviewChange{}, fault.New(
			fault.KindInternal, "missing_runtime", "reviewed policy apply is not configured", false,
		)
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyReviewChange{}, err
	}
	denyRules, err := s.readPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyReviewChange{}, err
	}
	denialRead, err := s.runtime.ClusterDenials(ctx, state, 10_000)
	if err != nil {
		return tobari.PolicyReviewChange{}, fault.Wrap(
			fault.KindInternal, "denials_failed", "cluster denials could not be read", false, err,
		)
	}
	candidates, err := tobari.PolicyCandidatesWithDenyRules(denialRead.Items, rules, denyRules)
	if err != nil {
		return tobari.PolicyReviewChange{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract", "policy candidates are invalid", false, err,
		)
	}
	reviewItems, err := tobari.PolicyReviewItems(candidates, rules)
	if err != nil {
		return tobari.PolicyReviewChange{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract", "review permissions items are invalid", false, err,
		)
	}
	byID := make(map[string]tobari.PolicyReviewItem, len(reviewItems))
	for _, item := range reviewItems {
		byID[item.ID] = item
	}
	hasAttachment := false
	for _, decision := range set.Decisions {
		if item, found := byID[decision.ReviewItemID]; found && item.Candidate != nil && item.Candidate.EffectiveDestinationKind() == tobari.PolicyDestinationHostLoopback {
			hasAttachment = true
		}
	}
	if hasAttachment {
		return s.applyAttachmentPolicyReview(ctx, intent, set, byID)
	}
	updatedAllows := append([]tobari.LearnedPolicyRule{}, rules...)
	updatedDenies := append([]tobari.PolicyDenyRule{}, denyRules.Exact...)
	allowCount, denyCount := 0, 0
	receipt := make([]tobari.PolicyReviewAppliedDecision, 0, len(set.Decisions))
	reviewContextID := ""
	for _, decision := range set.Decisions {
		item, found := byID[decision.ReviewItemID]
		if !found {
			return tobari.PolicyReviewChange{}, fault.New(
				fault.KindRejected, "policy_review_changed",
				"the reviewed permission set changed before Apply", false,
				fault.NextAction{Command: "review permissions", Reason: "Review the current pending queue again."},
			)
		}
		contextID := policyReviewItemContextID(item)
		if reviewContextID == "" {
			reviewContextID = contextID
		} else if contextID != reviewContextID {
			return tobari.PolicyReviewChange{}, fault.New(
				fault.KindRejected, "policy_review_scope_mixed",
				"one reviewed Apply cannot span multiple Context policy sources", false,
				fault.NextAction{Command: "review permissions", Reason: "Apply or discard the current Context decisions before reviewing another Context."},
			)
		}
		if item.Match == tobari.PolicyMatchExact {
			if decision.Match != tobari.PolicyMatchExact {
				return tobari.PolicyReviewChange{}, policyReviewChangedFault()
			}
			var applyErr error
			updatedAllows, updatedDenies, receipt, allowCount, denyCount, applyErr = applyExactPolicyReviewCandidate(
				*item.Candidate, decision, updatedAllows, updatedDenies, receipt, allowCount, denyCount,
			)
			if applyErr != nil {
				return tobari.PolicyReviewChange{}, applyErr
			}
			continue
		}
		proposal := *item.Template
		if decision.Match == tobari.PolicyMatchPathTemplate {
			if decision.Decision != tobari.PolicyDecisionAllow {
				return tobari.PolicyReviewChange{}, policyReviewChangedFault()
			}
			remove := make(map[string]struct{}, len(proposal.SourceRuleIDs))
			for _, id := range proposal.SourceRuleIDs {
				remove[id] = struct{}{}
			}
			kept := updatedAllows[:0]
			for _, existing := range updatedAllows {
				if _, replaced := remove[existing.ID]; !replaced {
					kept = append(kept, existing)
				}
			}
			updatedAllows = kept
			rule, ruleErr := tobari.NewPathTemplateLearnedPolicyRule(proposal)
			if ruleErr != nil {
				return tobari.PolicyReviewChange{}, fault.Wrap(fault.KindContract, "invalid_candidate_contract", "policy proposal cannot become a path-template rule", false, ruleErr)
			}
			updatedAllows = append(updatedAllows, rule)
			applied, receiptErr := tobari.NewPolicyReviewAppliedAllow(item.ID, rule)
			if receiptErr != nil {
				return tobari.PolicyReviewChange{}, fault.Wrap(fault.KindContract, "invalid_candidate_contract", "path-template rule cannot become a reviewed receipt", false, receiptErr)
			}
			receipt = append(receipt, applied)
			allowCount++
			continue
		}
		for _, candidate := range proposal.PendingCandidates {
			var applyErr error
			updatedAllows, updatedDenies, receipt, allowCount, denyCount, applyErr = applyExactPolicyReviewCandidate(
				candidate, decision, updatedAllows, updatedDenies, receipt, allowCount, denyCount,
			)
			if applyErr != nil {
				return tobari.PolicyReviewChange{}, applyErr
			}
		}
	}
	if allowCount+denyCount > tobari.MaxPolicyReviewDecisions {
		return tobari.PolicyReviewChange{}, fault.New(
			fault.KindInvalidInput, "invalid_policy_review_set", "reviewed policy decisions exceed the bounded rule count", false,
		)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: "policy apply-reviewed", ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	activation := tobari.PolicyActivationReceipt{}
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var applyErr error
		activation, applyErr = runtime.ApplyPolicyDecisionSet(
			actionContext, state, rules, updatedAllows, denyRules.Exact, updatedDenies,
		)
		if applyErr != nil {
			return applyErr
		}
		return activation.Validate()
	})
	if err != nil {
		if _, structured := fault.PublicCopy(err); structured {
			return tobari.PolicyReviewChange{}, err
		}
		return tobari.PolicyReviewChange{}, fault.Wrap(
			fault.KindUnavailable, "policy_learning_failed",
			"reviewed policy activation did not complete; inspect cluster status", false, err,
			fault.NextAction{Command: "cluster status", Reason: "Reconcile OPA and current policy state."},
		)
	}
	result := tobari.PolicyReviewChange{
		Task: tobari.TaskPolicyReviewApply, PolicyDirectory: activation.PolicyDirectory,
		AllowCount: allowCount, DenyCount: denyCount, Applied: true,
		ActiveRevision: activation.ActiveRevision, Decisions: receipt,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyReviewChange{}, fault.Wrap(
			fault.KindContract, "invalid_policy_review_result", "reviewed policy result is invalid", false, err,
		)
	}
	return result, nil
}

func (s *Service) applyAttachmentPolicyReview(
	ctx context.Context, intent operation.Intent, set tobari.PolicyReviewDecisionSet,
	byID map[string]tobari.PolicyReviewItem,
) (tobari.PolicyReviewChange, error) {
	runtime, ok := s.runtime.(attachmentGrantDecisionSetRuntimePort)
	if !ok || portcheck.IsNil(runtime) {
		return tobari.PolicyReviewChange{}, fault.New(fault.KindInternal, "missing_runtime", "attachment policy apply is not configured", false)
	}
	grants := make([]tobari.AttachmentGrant, 0, len(set.Decisions))
	receipts := make([]tobari.PolicyReviewAppliedDecision, 0, len(set.Decisions))
	allowCount, denyCount := 0, 0
	epochID := ""
	for _, decision := range set.Decisions {
		item, found := byID[decision.ReviewItemID]
		if !found || item.Match != tobari.PolicyMatchExact || item.Candidate == nil || decision.Match != tobari.PolicyMatchExact || item.Candidate.EffectiveDestinationKind() != tobari.PolicyDestinationHostLoopback {
			return tobari.PolicyReviewChange{}, fault.New(fault.KindRejected, "policy_review_scope_mixed", "one Apply cannot mix persistent and attachment-scoped decisions", false,
				fault.NextAction{Command: "review permissions", Reason: "Apply attachment-scoped decisions separately."})
		}
		candidate := *item.Candidate
		if epochID == "" {
			epochID = candidate.AttachmentEpochID
		} else if epochID != candidate.AttachmentEpochID {
			return tobari.PolicyReviewChange{}, policyReviewChangedFault()
		}
		grant, err := tobari.NewAttachmentGrantFromCandidate(decision.Decision, candidate)
		if err != nil {
			return tobari.PolicyReviewChange{}, fault.Wrap(fault.KindContract, "invalid_candidate_contract", "attachment candidate cannot become a grant", false, err)
		}
		receipt, err := tobari.NewPolicyReviewAppliedAttachment(candidate, grant)
		if err != nil {
			return tobari.PolicyReviewChange{}, fault.Wrap(fault.KindContract, "invalid_candidate_contract", "attachment grant cannot become a reviewed receipt", false, err)
		}
		grants, receipts = append(grants, grant), append(receipts, receipt)
		if decision.Decision == tobari.PolicyDecisionAllow {
			allowCount++
		} else {
			denyCount++
		}
	}
	request := execution.Request{Intent: intent, ExpectedCommand: "policy apply-reviewed", ExpectedEffect: operation.EffectWrite, ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact}
	activation := tobari.PolicyActivationReceipt{}
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var applyErr error
		activation, applyErr = runtime.ApplyAttachmentGrantDecisionSet(actionContext, grants)
		return applyErr
	})
	if err != nil {
		return tobari.PolicyReviewChange{}, fault.Wrap(fault.KindUnavailable, "attachment_policy_failed", "attachment policy activation did not complete", false, err,
			fault.NextAction{Command: "review permissions", Reason: "Review the still-active attachment again."})
	}
	result := tobari.PolicyReviewChange{Task: tobari.TaskPolicyReviewApply, PolicyDirectory: activation.PolicyDirectory,
		AllowCount: allowCount, DenyCount: denyCount, Applied: true, ActiveRevision: activation.ActiveRevision, Decisions: receipts}
	if err := result.Validate(); err != nil {
		return tobari.PolicyReviewChange{}, fault.Wrap(fault.KindContract, "invalid_policy_review_result", "attachment policy result is invalid", false, err)
	}
	return result, nil
}

func policyReviewChangedFault() error {
	return fault.New(
		fault.KindRejected, "policy_review_changed",
		"the reviewed permission set changed before Apply", false,
		fault.NextAction{Command: "review permissions", Reason: "Review the current pending queue again."},
	)
}

func policyReviewItemContextID(item tobari.PolicyReviewItem) string {
	if item.Candidate != nil {
		return item.Candidate.ContextID
	}
	if item.Template != nil {
		return item.Template.ContextID
	}
	return ""
}

func applyExactPolicyReviewCandidate(
	candidate tobari.PolicyCandidate, decision tobari.PolicyReviewDecision,
	allows []tobari.LearnedPolicyRule, denies []tobari.PolicyDenyRule,
	receipts []tobari.PolicyReviewAppliedDecision, allowCount, denyCount int,
) ([]tobari.LearnedPolicyRule, []tobari.PolicyDenyRule, []tobari.PolicyReviewAppliedDecision, int, int, error) {
	if decision.Decision == tobari.PolicyDecisionAllow {
		rule, err := tobari.NewExactLearnedPolicyRule(candidate)
		if err != nil {
			return nil, nil, nil, 0, 0, fault.Wrap(fault.KindContract, "invalid_candidate_contract", "policy candidate cannot become an exact rule", false, err)
		}
		applied, err := tobari.NewPolicyReviewAppliedAllow(decision.ReviewItemID, rule)
		if err != nil {
			return nil, nil, nil, 0, 0, fault.Wrap(fault.KindContract, "invalid_candidate_contract", "exact allow cannot become a reviewed receipt", false, err)
		}
		return append(allows, rule), denies, append(receipts, applied), allowCount + 1, denyCount, nil
	}
	rule, err := tobari.NewExactPolicyDenyRule(candidate)
	if err != nil {
		return nil, nil, nil, 0, 0, fault.Wrap(fault.KindContract, "invalid_candidate_contract", "policy candidate cannot become an exact deny", false, err)
	}
	applied, err := tobari.NewPolicyReviewAppliedDeny(decision.ReviewItemID, rule)
	if err != nil {
		return nil, nil, nil, 0, 0, fault.Wrap(fault.KindContract, "invalid_candidate_contract", "exact deny cannot become a reviewed receipt", false, err)
	}
	return allows, append(denies, rule), append(receipts, applied), allowCount, denyCount + 1, nil
}

func validatePolicyMutationTarget(intent operation.Intent, kind, id string) error {
	if intent.Target.Kind != kind || intent.Target.ID != id {
		return fault.New(
			fault.KindContract, "invalid_mutation_contract",
			"policy mutation target does not match the consumed opaque ID", false,
		)
	}
	return nil
}

func (s *Service) applyLearnedRules(
	ctx context.Context, intent operation.Intent, expectedCommand string,
	state tobari.State, expected, updated []tobari.LearnedPolicyRule,
) (tobari.PolicyActivationReceipt, error) {
	request := execution.Request{
		Intent: intent, ExpectedCommand: expectedCommand, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	receipt := tobari.PolicyActivationReceipt{}
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var actionErr error
		receipt, actionErr = s.runtime.ApplyLearnedPolicyRules(actionContext, state, expected, updated)
		if actionErr == nil {
			return receipt.Validate()
		}
		if _, structured := fault.PublicCopy(actionErr); structured {
			return actionErr
		}
		return fault.Wrap(
			fault.KindUnavailable, "policy_learning_failed",
			"learned policy activation did not complete; inspect cluster status", false, actionErr,
			fault.NextAction{
				Command: "cluster status",
				Reason:  "Reconcile OPA health and the current policy before another mutation.",
			},
		)
	})
	return receipt, err
}

func (s *Service) applyPolicyDenies(
	ctx context.Context, intent operation.Intent, expectedCommand string, state tobari.State,
	expectedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
) (tobari.PolicyActivationReceipt, error) {
	request := execution.Request{
		Intent: intent, ExpectedCommand: expectedCommand, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	receipt := tobari.PolicyActivationReceipt{}
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var actionErr error
		receipt, actionErr = s.runtime.ApplyPolicyDenyRules(
			actionContext, state, expectedAllows, expectedDenies, updatedDenies,
		)
		if actionErr == nil {
			return receipt.Validate()
		}
		if _, structured := fault.PublicCopy(actionErr); structured {
			return actionErr
		}
		return fault.Wrap(
			fault.KindUnavailable, "policy_learning_failed",
			"policy deny activation did not complete; inspect cluster status", false, actionErr,
			fault.NextAction{
				Command: "cluster status",
				Reason:  "Reconcile OPA health and the current policy before another mutation.",
			},
		)
	})
	return receipt, err
}

// AllowPolicyCandidate records and activates one exact retained denial.
func (s *Service) AllowPolicyCandidate(
	ctx context.Context, intent operation.Intent, id string,
) (tobari.PolicyLearningChange, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	if err := tobari.ValidatePolicyCandidateID(id); err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_policy_candidate_id",
			"policy candidate ID is invalid", false, err,
		)
	}
	if err := validatePolicyMutationTarget(intent, tobari.PolicyCandidateKind, id); err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	denyRules, err := s.readPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	denialRead, err := s.runtime.ClusterDenials(ctx, state, 10_000)
	if err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindInternal, "denials_failed", "cluster denials could not be read", false, err,
		)
	}
	candidates, err := tobari.PolicyCandidatesWithDenyRules(denialRead.Items, rules, denyRules)
	if err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract",
			"policy candidates are invalid", false, err,
		)
	}
	var candidate tobari.PolicyCandidate
	found := false
	for _, item := range candidates {
		if item.ID == id {
			candidate, found = item, true
			break
		}
	}
	if !found {
		return tobari.PolicyLearningChange{}, fault.New(
			fault.KindInvalidInput, "policy_candidate_not_found",
			"policy candidate is stale, already covered, or outside retained logs", false,
		)
	}
	rule, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract",
			"policy candidate cannot become an exact rule", false, err,
		)
	}
	updated := append(append([]tobari.LearnedPolicyRule{}, rules...), rule)
	if err := tobari.ValidateLearnedPolicyRules(updated); err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindContract, "invalid_learned_policy",
			"exact learned policy is invalid", false, err,
		)
	}
	activation, err := s.applyLearnedRules(ctx, intent, "policy allow", state, rules, updated)
	if err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	result := tobari.PolicyLearningChange{
		Task: tobari.TaskPolicyAllow, PolicyDirectory: activation.PolicyDirectory,
		TargetID: id, Rule: rule, SourceRuleCount: 1, Applied: true,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindContract, "invalid_policy_learning_result",
			"policy allow result is invalid", false, err,
		)
	}
	return result, nil
}

// DenyPolicyCandidate records and activates one exact project-bound denial.
func (s *Service) DenyPolicyCandidate(
	ctx context.Context, intent operation.Intent, id string,
) (tobari.PolicyDenyChange, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyDenyChange{}, err
	}
	if err := tobari.ValidatePolicyCandidateID(id); err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_policy_candidate_id",
			"policy candidate ID is invalid", false, err,
		)
	}
	if err := validatePolicyMutationTarget(intent, tobari.PolicyCandidateKind, id); err != nil {
		return tobari.PolicyDenyChange{}, err
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyDenyChange{}, err
	}
	denyRules, err := s.readPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyDenyChange{}, err
	}
	denialRead, err := s.runtime.ClusterDenials(ctx, state, 10_000)
	if err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
			fault.KindInternal, "denials_failed", "cluster denials could not be read", false, err,
		)
	}
	candidates, err := tobari.PolicyCandidatesWithDenyRules(denialRead.Items, rules, denyRules)
	if err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract",
			"policy candidates are invalid", false, err,
		)
	}
	var candidate tobari.PolicyCandidate
	found := false
	for _, item := range candidates {
		if item.ID == id {
			candidate, found = item, true
			break
		}
	}
	if !found {
		return tobari.PolicyDenyChange{}, fault.New(
			fault.KindInvalidInput, "policy_candidate_not_found",
			"policy candidate is stale, already covered, or outside retained logs", false,
		)
	}
	rule, err := tobari.NewExactPolicyDenyRule(candidate)
	if err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract",
			"policy candidate cannot become an exact deny rule", false, err,
		)
	}
	updatedDenies := append(append([]tobari.PolicyDenyRule{}, denyRules.Exact...), rule)
	updatedSet := tobari.PolicyDenyRuleSet{Exact: updatedDenies}
	if err := updatedSet.Validate(); err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
			fault.KindContract, "invalid_policy_deny", "exact policy deny is invalid", false, err,
		)
	}
	activation, err := s.applyPolicyDenies(ctx, intent, "policy deny", state, rules, denyRules.Exact, updatedDenies)
	if err != nil {
		return tobari.PolicyDenyChange{}, err
	}
	result := tobari.PolicyDenyChange{
		Task: tobari.TaskPolicyDeny, PolicyDirectory: activation.PolicyDirectory,
		TargetID: id, Rule: rule, SourceRuleCount: 1, Applied: true,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
			fault.KindContract, "invalid_policy_deny_result",
			"policy deny result is invalid", false, err,
		)
	}
	return result, nil
}

// ResetPolicyRule removes one current learned decision and returns the exact
// effect to default deny. It never creates a replacement Allow or Deny.
func (s *Service) ResetPolicyRule(
	ctx context.Context, intent operation.Intent, id string,
) (tobari.PolicyRuleReset, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyRuleReset{}, err
	}
	if err := tobari.ValidatePolicyRuleID(id); err != nil {
		return tobari.PolicyRuleReset{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_policy_rule_id",
			"policy rule ID is invalid", false, err,
		)
	}
	if err := validatePolicyMutationTarget(intent, tobari.PolicyRuleKind, id); err != nil {
		return tobari.PolicyRuleReset{}, err
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyRuleReset{}, err
	}
	denyRules, err := s.readPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyRuleReset{}, err
	}
	updatedRules, updatedDenies, removed, err := tobari.RemovePolicyRule(rules, denyRules.Exact, id)
	if err != nil {
		return tobari.PolicyRuleReset{}, fault.Wrap(
			fault.KindInvalidInput, "policy_rule_not_found",
			"policy rule is stale, baseline-owned, or no longer current", false, err,
		)
	}
	activation := tobari.PolicyActivationReceipt{}
	if removed.Decision == tobari.PolicyDecisionAllow {
		activation, err = s.applyLearnedRules(ctx, intent, "policy reset", state, rules, updatedRules)
		if err != nil {
			return tobari.PolicyRuleReset{}, err
		}
	} else {
		activation, err = s.applyPolicyDenies(ctx, intent, "policy reset", state, rules, denyRules.Exact, updatedDenies)
		if err != nil {
			return tobari.PolicyRuleReset{}, err
		}
	}
	result := tobari.PolicyRuleReset{
		Task: tobari.TaskPolicyReset, PolicyDirectory: activation.PolicyDirectory,
		TargetID: id, Decision: removed.Decision, Applied: true,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyRuleReset{}, fault.Wrap(
			fault.KindContract, "invalid_policy_rule_reset_result",
			"policy rule reset result is invalid", false, err,
		)
	}
	return result, nil
}
