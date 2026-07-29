// Package samplecmd implements the template's discover-to-act sample flow.
package samplecmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/tasuku43/agentic-cli-foundry/internal/app/pagination"
	"github.com/tasuku43/agentic-cli-foundry/internal/app/portcheck"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/page"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/sample"
)

// ErrNotFound indicates that a canonical sample ID has no matching item.
var ErrNotFound = errors.New("sample not found")

// RepositoryPort is the exact storage capability required by the sample use
// cases. Infrastructure satisfies it without importing app.
type RepositoryPort interface {
	ListPage(context.Context, page.Request) (page.Result[sample.Summary], error)
	Get(context.Context, string) (sample.Item, bool, error)
}

var sampleListBudget = pagination.Budget{PageSize: 100, MaxPages: 100, MaxItems: 10_000}

// Service coordinates sample discovery and exact-ID reads.
type Service struct {
	repository RepositoryPort
}

// New creates a sample service.
func New(repository RepositoryPort) *Service {
	return &Service{repository: repository}
}

// List discovers samples and preserves every repository ID byte-for-byte.
func (s *Service) List(ctx context.Context, intent operation.Intent) ([]sample.Summary, error) {
	if err := validateContextAndIntent(ctx, intent, "sample list"); err != nil {
		return nil, err
	}
	if s == nil || portcheck.IsNil(s.repository) {
		return nil, fmt.Errorf("sample repository is not configured")
	}
	items, err := pagination.Drain(ctx, sampleListBudget, s.repository.ListPage)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("sample %d: %w", index, err)
		}
		if _, exists := seen[item.ID]; exists {
			return nil, fmt.Errorf("sample repository returned duplicate ID")
		}
		seen[item.ID] = struct{}{}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]sample.Summary(nil), items...), nil
}

// Read acts on exactly one canonical opaque ID. It does not search by name or
// transform URL-like input into an ID.
func (s *Service) Read(ctx context.Context, intent operation.Intent, id string) (sample.Item, error) {
	if err := validateContextAndIntent(ctx, intent, "sample read"); err != nil {
		return sample.Item{}, err
	}
	validatedID, err := sample.ValidateID(id)
	if err != nil {
		return sample.Item{}, err
	}
	if s == nil || portcheck.IsNil(s.repository) {
		return sample.Item{}, fmt.Errorf("sample repository is not configured")
	}
	item, found, err := s.repository.Get(ctx, validatedID)
	if contextErr := ctx.Err(); contextErr != nil {
		return sample.Item{}, contextErr
	}
	if err != nil {
		return sample.Item{}, fmt.Errorf("read sample: %w", err)
	}
	if !found {
		return sample.Item{}, fault.Wrap(
			fault.KindNotFound,
			"sample_not_found",
			"The requested sample was not found.",
			false,
			fmt.Errorf("%w: %s", ErrNotFound, validatedID),
			fault.NextAction{Command: "sample list", Reason: "Discover a current opaque sample ID."},
		)
	}
	if err := item.Validate(); err != nil {
		return sample.Item{}, fmt.Errorf("invalid sample: %w", err)
	}
	if item.ID != validatedID {
		return sample.Item{}, fmt.Errorf("sample repository returned a different ID")
	}
	return item, nil
}

func validateContextAndIntent(ctx context.Context, intent operation.Intent, command string) error {
	if ctx == nil {
		return fmt.Errorf("sample context is nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := intent.Validate(); err != nil {
		return fmt.Errorf("sample intent: %w", err)
	}
	if intent.Command != command || intent.Effect != operation.EffectRead {
		return fmt.Errorf("%s requires its declared read intent", command)
	}
	return nil
}
