package tobaricmd

import (
	"context"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func (s *Service) policyCandidates(
	ctx context.Context, tail int, task string,
) (tobari.PolicyCandidateReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyCandidateReport{}, err
	}
	request := tobari.LogRequest{Component: "gateway", Tail: tail}
	if err := request.ValidateCluster(); err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_candidate_request",
			"policy candidate request is invalid", false, err,
		)
	}
	state, err := s.readyCluster(ctx)
	if err != nil {
		return tobari.PolicyCandidateReport{}, err
	}
	denials, err := s.runtime.ClusterDenials(ctx, state, tail)
	if err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindInternal, "denials_failed", "cluster denials could not be read", false, err,
		)
	}
	rules, err := s.runtime.ReadLearnedPolicyRules(ctx, state)
	if err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindRejected, "policy_data_invalid",
			"learned policy data could not be read safely", false, err,
		)
	}
	denyRules, err := s.runtime.ReadPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindRejected, "policy_data_invalid",
			"policy deny data could not be read safely", false, err,
		)
	}
	items, err := tobari.PolicyCandidatesWithDenyRules(denials, rules, denyRules)
	if err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract",
			"policy candidates are invalid", false, err,
		)
	}
	result := tobari.PolicyCandidateReport{
		Task: task, PolicyDirectory: state.PolicyDirectory, WindowLines: tail, Items: items,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract",
			"policy candidate result is invalid", false, err,
		)
	}
	return result, nil
}

// PolicyCandidates discovers pending exact-rule proposals from retained denials.
func (s *Service) PolicyCandidates(
	ctx context.Context, tail int,
) (tobari.PolicyCandidateReport, error) {
	return s.policyCandidates(ctx, tail, tobari.TaskPolicyCandidates)
}

// PolicyReview discovers the bounded exact-permission queue for a human host
// review. It is intentionally read-only; policy allow remains the separate
// opaque-reference-bound mutation.
func (s *Service) PolicyReview(
	ctx context.Context, tail int,
) (tobari.PolicyCandidateReport, error) {
	return s.policyCandidates(ctx, tail, tobari.TaskPolicyReview)
}

// PolicyRules returns the complete current learned-decision inventory. It is
// separate from PolicyReview because covered decisions intentionally disappear
// from the pending denial queue but remain user-manageable state.
func (s *Service) PolicyRules(
	ctx context.Context,
) (tobari.PolicyRuleReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyRuleReport{}, err
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyRuleReport{}, err
	}
	denyRules, err := s.readPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyRuleReport{}, err
	}
	items, err := tobari.CurrentPolicyRules(rules, denyRules.Exact)
	if err != nil {
		return tobari.PolicyRuleReport{}, fault.Wrap(
			fault.KindContract, "invalid_policy_rule_report",
			"current policy rules are invalid", false, err,
		)
	}
	result := tobari.PolicyRuleReport{
		Task: tobari.TaskPolicyRules, PolicyDirectory: state.PolicyDirectory, Items: items,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyRuleReport{}, fault.Wrap(
			fault.KindContract, "invalid_policy_rule_report",
			"current policy rule report is invalid", false, err,
		)
	}
	return result, nil
}

func (s *Service) loadPolicyState(
	ctx context.Context,
) (tobari.State, []tobari.LearnedPolicyRule, error) {
	state, err := s.readyCluster(ctx)
	if err != nil {
		return tobari.State{}, nil, err
	}
	rules, err := s.runtime.ReadLearnedPolicyRules(ctx, state)
	if err != nil {
		return tobari.State{}, nil, fault.Wrap(
			fault.KindRejected, "policy_data_invalid",
			"learned policy data could not be read safely", false, err,
		)
	}
	return state, rules, nil
}

func (s *Service) readPolicyDenyRules(
	ctx context.Context, state tobari.State,
) (tobari.PolicyDenyRuleSet, error) {
	denyRules, err := s.runtime.ReadPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyDenyRuleSet{}, fault.Wrap(
			fault.KindRejected, "policy_data_invalid",
			"policy deny data could not be read safely", false, err,
		)
	}
	if err := denyRules.Validate(); err != nil {
		return tobari.PolicyDenyRuleSet{}, fault.Wrap(
			fault.KindContract, "invalid_policy_deny", "policy deny data is invalid", false, err,
		)
	}
	return denyRules, nil
}
