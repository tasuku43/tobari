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

const (
	TaskPolicyCandidates = "policy candidates"
	TaskPolicyRules      = "policy rules"
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

type PolicyMemoryReadPort interface {
	ListPendingPolicyCandidateAuthority(context.Context) (tobari.PolicyCandidateAuthorityList, error)
	ListPolicyMemoryRuleAuthority(context.Context) (tobari.PolicyMemoryRuleList, error)
}

type AttachmentPolicyCandidatePort interface {
	ListPolicyCandidatesIncludingAttachments(context.Context) (tobari.PolicyCandidateAuthorityList, error)
	ApplyAttachmentPolicyCandidate(context.Context, string, tobari.PolicyMemoryDecision) (tobari.AttachmentGrantPublication, bool, error)
}

type PolicyMemoryReviewPort interface {
	ReadPolicyMemoryReviewSnapshot(context.Context) (tobari.PolicyMemoryReviewSnapshot, error)
}

type policyMemoryMutationPolicy struct{}

func (policyMemoryMutationPolicy) Check(_ context.Context, intent operation.Intent) error {
	switch {
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.PolicyCandidateKind && intent.Target.ID != "" && intent.Target.ParentID == "":
		return tobari.ValidatePolicyCandidateID(intent.Target.ID)
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.PolicyRuleKind && intent.Target.ID != "" && intent.Target.ParentID == "":
		return tobari.ValidatePolicyMemoryRuleID(intent.Target.ID)
	case intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.PolicyDecisionSetKind && intent.Target.ParentID == tobari.PolicyDecisionSetID && intent.Target.ID == "":
		return nil
	default:
		return fault.New(fault.KindRejected, "mutation_rejected", "Policy Memory mutation target is not owned by Tobari", false)
	}
}

type PolicyMemoryService struct {
	read       PolicyMemoryReadPort
	review     PolicyMemoryReviewPort
	candidate  PolicyCandidatePort
	attachment AttachmentPolicyCandidatePort
	rule       PolicyRulePort
	reviewed   PolicyReviewedPort
	mutator    *execution.Invoker
}

func NewPolicyMemoryService(port any) *PolicyMemoryService {
	service := &PolicyMemoryService{mutator: execution.New(policyMemoryMutationPolicy{})}
	service.read, _ = port.(PolicyMemoryReadPort)
	service.review, _ = port.(PolicyMemoryReviewPort)
	service.candidate, _ = port.(PolicyCandidatePort)
	service.attachment, _ = port.(AttachmentPolicyCandidatePort)
	service.rule, _ = port.(PolicyRulePort)
	service.reviewed, _ = port.(PolicyReviewedPort)
	return service
}

func (s *PolicyMemoryService) ReviewSnapshot(ctx context.Context) (tobari.PolicyMemoryReviewSnapshot, error) {
	if s == nil || portcheck.IsNil(s.review) {
		return tobari.PolicyMemoryReviewSnapshot{}, missingPort("Permission Inbox")
	}
	result, err := s.review.ReadPolicyMemoryReviewSnapshot(ctx)
	if err != nil {
		return tobari.PolicyMemoryReviewSnapshot{}, readFault(err, "policy_review_read_failed", "Permission Inbox could not be read")
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyMemoryReviewSnapshot{}, contractFault("invalid_policy_review_snapshot", "Permission Inbox snapshot is invalid", err)
	}
	return result.Clone(), nil
}

func (s *PolicyMemoryService) Candidates(ctx context.Context) (tobari.PolicyCandidateAuthorityList, error) {
	if s != nil && !portcheck.IsNil(s.attachment) {
		result, err := s.attachment.ListPolicyCandidatesIncludingAttachments(ctx)
		if err != nil {
			return tobari.PolicyCandidateAuthorityList{}, readFault(err, "policy_candidate_read_failed", "Policy candidates could not be read")
		}
		if err := result.Validate(); err != nil {
			return tobari.PolicyCandidateAuthorityList{}, contractFault("invalid_policy_candidate_list", "Policy candidate list is invalid", err)
		}
		return result.Clone(), nil
	}
	if s == nil || portcheck.IsNil(s.read) {
		return tobari.PolicyCandidateAuthorityList{}, missingPort("Policy candidate read")
	}
	result, err := s.read.ListPendingPolicyCandidateAuthority(ctx)
	if err != nil {
		return tobari.PolicyCandidateAuthorityList{}, readFault(err, "policy_candidate_read_failed", "Policy candidates could not be read")
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyCandidateAuthorityList{}, contractFault("invalid_policy_candidate_list", "Policy candidate list is invalid", err)
	}
	return result.Clone(), nil
}

func (s *PolicyMemoryService) Rules(ctx context.Context) (tobari.PolicyMemoryRuleList, error) {
	if s == nil || portcheck.IsNil(s.read) {
		return tobari.PolicyMemoryRuleList{}, missingPort("Policy Memory rule read")
	}
	result, err := s.read.ListPolicyMemoryRuleAuthority(ctx)
	if err != nil {
		return tobari.PolicyMemoryRuleList{}, readFault(err, "policy_rule_read_failed", "Policy Memory rules could not be read")
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyMemoryRuleList{}, contractFault("invalid_policy_rule_list", "Policy Memory rule list is invalid", err)
	}
	return result.Clone(), nil
}

func PolicyMemoryImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}
}

func (s *PolicyMemoryService) Allow(ctx context.Context, intent operation.Intent, candidateRef string) (tobari.PolicyCandidateDecisionPublication, error) {
	return s.applyCandidate(ctx, intent, candidateRef, tobari.PolicyMemoryAllow)
}

func (s *PolicyMemoryService) Deny(ctx context.Context, intent operation.Intent, candidateRef string) (tobari.PolicyCandidateDecisionPublication, error) {
	return s.applyCandidate(ctx, intent, candidateRef, tobari.PolicyMemoryDeny)
}

func (s *PolicyMemoryService) applyCandidate(ctx context.Context, intent operation.Intent, candidateRef string, decision tobari.PolicyMemoryDecision) (tobari.PolicyCandidateDecisionPublication, error) {
	if err := tobari.ValidatePolicyCandidateID(candidateRef); err != nil {
		return tobari.PolicyCandidateDecisionPublication{}, invalidFault("invalid_policy_candidate_ref", "policy candidate reference is invalid", err, "policy candidates")
	}
	if s == nil || portcheck.IsNil(s.candidate) {
		return tobari.PolicyCandidateDecisionPublication{}, missingPort("Policy Memory candidate")
	}
	command := TaskPolicyAllow
	if decision == tobari.PolicyMemoryDeny {
		command = TaskPolicyDeny
	}
	target := operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: candidateRef}
	request := execution.Request{Intent: intent, ExpectedCommand: command, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: PolicyMemoryImpact()}
	var result tobari.PolicyCandidateDecisionPublication
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		if !portcheck.IsNil(s.attachment) {
			publication, handled, attachmentErr := s.attachment.ApplyAttachmentPolicyCandidate(actionContext, candidateRef, decision)
			if attachmentErr != nil {
				return policyMemoryMutationFault(attachmentErr)
			}
			if handled {
				result = tobari.NewAttachmentPolicyCandidateDecisionPublication(publication)
				if err := result.ValidateFor(candidateRef, decision); err != nil {
					return contractFault("invalid_policy_memory_result", "attachment policy candidate publication is invalid", err)
				}
				return nil
			}
		}
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
		result = tobari.NewPersistentPolicyCandidateDecisionPublication(publication)
		return nil
	})
	return result, err
}

func (s *PolicyMemoryService) Reset(ctx context.Context, intent operation.Intent, ruleRef string) (tobari.PolicyRuleResetPublication, error) {
	if err := tobari.ValidatePolicyMemoryRuleID(ruleRef); err != nil {
		return tobari.PolicyRuleResetPublication{}, invalidFault("invalid_policy_rule_ref", "policy rule reference is invalid", err, "policy rules")
	}
	if s == nil || portcheck.IsNil(s.rule) {
		return tobari.PolicyRuleResetPublication{}, missingPort("Policy Memory rule")
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
	if err := set.Validate(); err != nil {
		return tobari.PolicyMemoryReviewedSetPublication{}, invalidFault(
			"invalid_policy_review_set", "reviewed Policy Memory decision set is invalid", err, "review permissions",
		)
	}
	if s == nil || portcheck.IsNil(s.reviewed) {
		return tobari.PolicyMemoryReviewedSetPublication{}, missingPort("reviewed Policy Memory")
	}
	target := operation.TargetRef{Kind: tobari.PolicyDecisionSetKind, ParentID: tobari.PolicyDecisionSetID}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskPolicyApply, ExpectedEffect: operation.EffectCreate, ExpectedTarget: target, ExpectedImpact: PolicyMemoryImpact()}
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
	if classified, ok := preReleaseLegacyMutationFault(err); ok {
		return classified
	}
	if classified, ok := finalAuthorityMutationRecoveryFault(err); ok {
		return classified
	}
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
