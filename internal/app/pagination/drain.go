// Package pagination provides bounded, all-or-nothing page traversal.
package pagination

import (
	"context"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/page"
)

// Budget prevents a CLI from silently traversing an unbounded remote result.
type Budget struct {
	PageSize int
	MaxPages int
	MaxItems int
}

// Validate rejects omitted limits; each derived command must choose explicit
// bounds in its product contract.
func (b Budget) Validate() error {
	if b.PageSize <= 0 || b.MaxPages <= 0 || b.MaxItems <= 0 {
		return fault.New(
			fault.KindContract,
			"invalid_pagination_budget",
			"page size, maximum pages, and maximum items must be positive",
			false,
		)
	}
	return nil
}

// Fetch retrieves one page for the exact opaque request token.
type Fetch[T any] func(context.Context, page.Request) (page.Result[T], error)

// Drain returns a complete result or no result. It detects cursor loops,
// enforces page/item budgets, and never converts an incomplete traversal into
// successful partial output.
func Drain[T any](ctx context.Context, budget Budget, fetch Fetch[T]) ([]T, error) {
	if err := budget.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, fault.New(fault.KindContract, "missing_context", "pagination context is not configured", false)
	}
	if fetch == nil {
		return nil, fault.New(fault.KindContract, "missing_page_fetcher", "page fetcher is not configured", false)
	}
	if err := ctx.Err(); err != nil {
		return nil, fault.Wrap(fault.KindCanceled, "operation_canceled", "pagination was canceled", true, err)
	}

	items := make([]T, 0)
	seenTokens := map[string]struct{}{"": {}}
	token := ""
	for pageNumber := 1; ; pageNumber++ {
		if pageNumber > budget.MaxPages {
			return nil, fault.New(
				fault.KindContract,
				"pagination_page_limit",
				"pagination did not complete within the declared page limit",
				false,
			)
		}
		if err := ctx.Err(); err != nil {
			return nil, fault.Wrap(fault.KindCanceled, "operation_canceled", "pagination was canceled", true, err)
		}
		request := page.Request{Token: token, Size: budget.PageSize}
		result, err := fetch(ctx, request)
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, fault.Wrap(fault.KindCanceled, "operation_canceled", "pagination was canceled", true, contextErr)
		}
		if err != nil {
			if structured, ok := fault.PublicCopy(err); ok {
				return nil, structured
			}
			return nil, fault.Wrap(
				fault.KindUnavailable,
				"page_fetch_failed",
				"a page could not be fetched; no partial result was emitted",
				true,
				err,
			)
		}
		if err := result.Validate(); err != nil {
			return nil, fault.Wrap(fault.KindContract, "invalid_page_contract", "the page response contract is invalid", false, err)
		}
		if len(result.Items) > request.Size {
			return nil, fault.New(
				fault.KindContract,
				"invalid_page_contract",
				"the page response exceeded the requested page size; no partial result was emitted",
				false,
			)
		}
		if len(result.Items) > budget.MaxItems-len(items) {
			return nil, fault.New(
				fault.KindContract,
				"pagination_item_limit",
				"pagination did not complete within the declared item limit",
				false,
			)
		}
		items = append(items, result.Items...)
		if result.NextToken == "" {
			if contextErr := ctx.Err(); contextErr != nil {
				return nil, fault.Wrap(fault.KindCanceled, "operation_canceled", "pagination was canceled before completion", true, contextErr)
			}
			return items, nil
		}
		if _, exists := seenTokens[result.NextToken]; exists {
			return nil, fault.New(
				fault.KindContract,
				"pagination_cursor_loop",
				"pagination returned a repeated cursor; no partial result was emitted",
				false,
			)
		}
		seenTokens[result.NextToken] = struct{}{}
		token = result.NextToken
	}
}
