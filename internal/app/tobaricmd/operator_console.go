package tobaricmd

import (
	"context"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// OperatorConsoleSnapshot composes existing read tasks without creating a
// second interpretation path for browser presentation.
func (s *Service) OperatorConsoleSnapshot(
	ctx context.Context, tail int,
) (tobari.OperatorConsoleSnapshot, error) {
	cluster, err := s.ClusterStatus(ctx)
	if err != nil {
		return tobari.OperatorConsoleSnapshot{}, err
	}
	workspaces, err := s.ProjectList(ctx)
	if err != nil {
		return tobari.OperatorConsoleSnapshot{}, err
	}
	review, err := s.PolicyReview(ctx, tail)
	if err != nil {
		return tobari.OperatorConsoleSnapshot{}, err
	}
	rules, err := s.PolicyRules(ctx)
	if err != nil {
		return tobari.OperatorConsoleSnapshot{}, err
	}
	result := tobari.OperatorConsoleSnapshot{
		Task: tobari.TaskOperatorConsoleSnapshot, Cluster: cluster,
		Workspaces: workspaces, WindowLines: review.WindowLines,
		ReviewItems: append([]tobari.PolicyReviewItem{}, review.ReviewItems...),
		Rules:       rules,
	}
	if err := result.Validate(); err != nil {
		return tobari.OperatorConsoleSnapshot{}, fault.Wrap(
			fault.KindContract, "invalid_operator_console_snapshot",
			"operator console snapshot is invalid", false, err,
		)
	}
	return result, nil
}
