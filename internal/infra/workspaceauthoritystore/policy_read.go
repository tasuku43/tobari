package workspaceauthoritystore

import (
	"context"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// ReadPolicyMemoryReviewSnapshot derives every review item and source rule
// from one guarded complete-envelope observation. Callers never compose the
// independently useful candidates and rules readers into a torn Inbox.
func (s *Store) ReadPolicyMemoryReviewSnapshot(ctx context.Context) (tobari.PolicyMemoryReviewSnapshot, error) {
	collection, present, err := s.ReadComplete(ctx)
	if err != nil {
		return tobari.PolicyMemoryReviewSnapshot{}, err
	}
	return tobari.NewPolicyMemoryReviewSnapshot(collection, present)
}

// ListPendingPolicyCandidateAuthority returns the exhaustive pending-candidate
// projection from one coherent final-envelope observation. It never consults
// denial logs, predecessor policy directories, or another ambient source.
func (s *Store) ListPendingPolicyCandidateAuthority(ctx context.Context) (tobari.PolicyCandidateAuthorityList, error) {
	collection, present, err := s.ReadComplete(ctx)
	if err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	return tobari.NewPolicyCandidateAuthorityList(collection, present)
}

// ListPolicyMemoryRuleAuthority returns every current remembered rule from the
// same complete final-envelope boundary, including an exact known-empty result
// for a fresh final installation.
func (s *Store) ListPolicyMemoryRuleAuthority(ctx context.Context) (tobari.PolicyMemoryRuleList, error) {
	collection, present, err := s.ReadComplete(ctx)
	if err != nil {
		return tobari.PolicyMemoryRuleList{}, err
	}
	return tobari.NewPolicyMemoryRuleList(collection, present)
}
