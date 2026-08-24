package workspaceauthoritycmd

import (
	"context"
	"errors"
	"reflect"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type PolicyCandidatePort interface {
	AllowPolicyCandidateByReference(context.Context, string) (tobari.PolicyCandidatePublication, error)
	DenyPolicyCandidateByReference(context.Context, string) (tobari.PolicyCandidatePublication, error)
}

type PolicyRulePort interface {
	ResetPolicyMemoryRuleByReference(context.Context, string) (tobari.PolicyRuleResetPublication, error)
}

type PolicyReviewedPort interface {
	ApplyReviewedPolicyMemory(context.Context, tobari.PolicyMemoryReviewedDecisionSet) (tobari.PolicyMemoryReviewedSetPublication, error)
}

type policyMemoryMutationPolicy struct{}

func (policyMemoryMutationPolicy) Check(_ context.Context, intent operation.Intent) error {
	switch {
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.PolicyCandidateKind && intent.Target.ID != "" && intent.Target.ParentID == "":
		return tobari.ValidatePolicyCandidateID(intent.Target.ID)
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.PolicyRuleKind && intent.Target.ID != "" && intent.Target.ParentID == "":
		return tobari.ValidatePolicyMemoryRuleID(intent.Target.ID)
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.PolicyDecisionSetKind && intent.Target.ID == tobari.PolicyDecisionSetID && intent.Target.ParentID == "":
		return nil
	default:
		return fault.New(fault.KindRejected, "mutation_rejected", "Policy Memory mutation target is not owned by Tobari", false)
	}
}

type PolicyMemoryService struct {
	candidate PolicyCandidatePort
	rule      PolicyRulePort
	reviewed  PolicyReviewedPort
	mutator   *execution.Invoker
}

func NewPolicyMemoryService(port any) *PolicyMemoryService {
	service := &PolicyMemoryService{mutator: execution.New(policyMemoryMutationPolicy{})}
	service.candidate, _ = port.(PolicyCandidatePort)
	service.rule, _ = port.(PolicyRulePort)
	service.reviewed, _ = port.(PolicyReviewedPort)
	return service
}

func PolicyMemoryImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}
}

func (s *PolicyMemoryService) Allow(ctx context.Context, intent operation.Intent, candidateRef string) (tobari.PolicyCandidatePublication, error) {
	return s.applyCandidate(ctx, intent, candidateRef, tobari.PolicyMemoryAllow)
}

func (s *PolicyMemoryService) Deny(ctx context.Context, intent operation.Intent, candidateRef string) (tobari.PolicyCandidatePublication, error) {
	return s.applyCandidate(ctx, intent, candidateRef, tobari.PolicyMemoryDeny)
}

func (s *PolicyMemoryService) applyCandidate(ctx context.Context, intent operation.Intent, candidateRef string, decision tobari.PolicyMemoryDecision) (tobari.PolicyCandidatePublication, error) {
	if s == nil || portcheck.IsNil(s.candidate) {
		return tobari.PolicyCandidatePublication{}, missingPort("Policy Memory candidate")
	}
	if err := tobari.ValidatePolicyCandidateID(candidateRef); err != nil {
		return tobari.PolicyCandidatePublication{}, invalidFault("invalid_policy_candidate_ref", "policy candidate reference is invalid", err, "policy candidates")
	}
	command := TaskPolicyAllow
	if decision == tobari.PolicyMemoryDeny {
		command = TaskPolicyDeny
	}
	target := operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: candidateRef}
	request := execution.Request{Intent: intent, ExpectedCommand: command, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: PolicyMemoryImpact()}
	var result tobari.PolicyCandidatePublication
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var publication tobari.PolicyCandidatePublication
		var err error
		if decision == tobari.PolicyMemoryAllow {
			publication, err = s.candidate.AllowPolicyCandidateByReference(actionContext, candidateRef)
		} else {
			publication, err = s.candidate.DenyPolicyCandidateByReference(actionContext, candidateRef)
		}
		if err != nil {
			return policyMemoryMutationFault(err)
		}
		if err := publication.ValidateFor(candidateRef, decision); err != nil {
			return contractFault("invalid_policy_memory_result", "Policy Memory candidate publication is invalid", err)
		}
		result = publication
		return nil
	})
	return result, err
}

func (s *PolicyMemoryService) Reset(ctx context.Context, intent operation.Intent, ruleRef string) (tobari.PolicyRuleResetPublication, error) {
	if s == nil || portcheck.IsNil(s.rule) {
		return tobari.PolicyRuleResetPublication{}, missingPort("Policy Memory rule")
	}
	if err := tobari.ValidatePolicyMemoryRuleID(ruleRef); err != nil {
		return tobari.PolicyRuleResetPublication{}, invalidFault("invalid_policy_rule_ref", "policy rule reference is invalid", err, "policy rules")
	}
	target := operation.TargetRef{Kind: tobari.PolicyRuleKind, ID: ruleRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskPolicyReset, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: PolicyMemoryImpact()}
	var result tobari.PolicyRuleResetPublication
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		publication, err := s.rule.ResetPolicyMemoryRuleByReference(actionContext, ruleRef)
		if err != nil {
			return policyMemoryMutationFault(err)
		}
		if err := publication.ValidateFor(ruleRef); err != nil {
			return contractFault("invalid_policy_memory_result", "Policy Memory reset publication is invalid", err)
		}
		result = publication
		return nil
	})
	return result, err
}

func (s *PolicyMemoryService) ApplyReviewed(
	ctx context.Context,
	intent operation.Intent,
	set tobari.PolicyMemoryReviewedDecisionSet,
) (tobari.PolicyMemoryReviewedSetPublication, error) {
	if s == nil || portcheck.IsNil(s.reviewed) {
		return tobari.PolicyMemoryReviewedSetPublication{}, missingPort("reviewed Policy Memory")
	}
	if err := set.Validate(); err != nil {
		return tobari.PolicyMemoryReviewedSetPublication{}, invalidFault(
			"invalid_policy_review_set", "reviewed Policy Memory decision set is invalid", err, "review permissions",
		)
	}
	target := operation.TargetRef{Kind: tobari.PolicyDecisionSetKind, ID: tobari.PolicyDecisionSetID}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskPolicyApply, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: PolicyMemoryImpact()}
	var result tobari.PolicyMemoryReviewedSetPublication
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		publication, err := s.reviewed.ApplyReviewedPolicyMemory(actionContext, set.Clone())
		if err != nil {
			return policyMemoryMutationFault(err)
		}
		if err := publication.Validate(); err != nil {
			return contractFault("invalid_policy_memory_result", "reviewed Policy Memory publication is invalid", err)
		}
		if !reflect.DeepEqual(publication.DecisionSet, set) {
			return contractFault("invalid_policy_memory_result", "reviewed Policy Memory publication is invalid", errors.New("reviewed decision set changed across the task boundary"))
		}
		result = publication
		return nil
	})
	return result, err
}

func policyMemoryMutationFault(err error) error {
	if errors.Is(err, tobari.ErrPolicyReviewChanged) {
		return fault.WithClassification(fault.New(
			fault.KindRejected, "policy_review_changed", "reviewed Policy Memory authority changed before Apply", false,
			fault.NextAction{Command: "review permissions", Reason: "Review the current complete decision set again."},
		), fault.PhasePrecondition, fault.ChangeNone)
	}
	if errors.Is(err, tobari.ErrPolicyMemoryTargetNotFound) {
		return fault.WithClassification(fault.New(fault.KindNotFound, "policy_target_not_found", "Policy Memory target no longer exists", false, fault.NextAction{Command: "policy rules", Reason: "Read current remembered authority."}), fault.PhasePrecondition, fault.ChangeNone)
	}
	return err
}
