package pagination

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/page"
)

func testBudget() Budget {
	return Budget{PageSize: 2, MaxPages: 3, MaxItems: 5}
}

func TestDrainReturnsOnlyCompleteResultsAndPreservesCursors(t *testing.T) {
	wantTokens := []string{"", "opaque/+==", " final token "}
	responses := []page.Result[int]{
		{Items: []int{1, 2}, NextToken: wantTokens[1]},
		{Items: []int{3}, NextToken: wantTokens[2]},
		{Items: []int{4}},
	}
	var gotTokens []string
	got, err := Drain(context.Background(), testBudget(), func(_ context.Context, request page.Request) (page.Result[int], error) {
		gotTokens = append(gotTokens, request.Token)
		return responses[len(gotTokens)-1], nil
	})
	if err != nil {
		t.Fatalf("Drain() error = %v", err)
	}
	if !reflect.DeepEqual(got, []int{1, 2, 3, 4}) || !reflect.DeepEqual(gotTokens, wantTokens) {
		t.Fatalf("items = %v, tokens = %q", got, gotTokens)
	}
}

func TestDrainHandlesEmptyAndSinglePageResults(t *testing.T) {
	tests := []struct {
		name string
		page page.Result[string]
		want []string
	}{
		{name: "empty", page: page.Result[string]{}, want: []string{}},
		{name: "single", page: page.Result[string]{Items: []string{"one"}}, want: []string{"one"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			got, err := Drain(context.Background(), testBudget(), func(_ context.Context, request page.Request) (page.Result[string], error) {
				calls++
				if request.Token != "" {
					t.Fatalf("first token = %q, want empty", request.Token)
				}
				return test.page, nil
			})
			if err != nil {
				t.Fatalf("Drain() error = %v", err)
			}
			if calls != 1 || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("calls = %d, items = %#v, want %#v", calls, got, test.want)
			}
		})
	}
}

func TestDrainRejectsIncompleteTraversalWithoutPartialResults(t *testing.T) {
	tests := []struct {
		name  string
		fetch Fetch[int]
		limit Budget
	}{
		{
			name: "fetch failure after first page",
			fetch: func() Fetch[int] {
				calls := 0
				return func(context.Context, page.Request) (page.Result[int], error) {
					calls++
					if calls == 1 {
						return page.Result[int]{Items: []int{1}, NextToken: "next"}, nil
					}
					return page.Result[int]{}, errors.New("offline")
				}
			}(),
			limit: testBudget(),
		},
		{
			name: "cursor loop",
			fetch: func(_ context.Context, request page.Request) (page.Result[int], error) {
				if request.Token == "" {
					return page.Result[int]{Items: []int{1}, NextToken: "same"}, nil
				}
				return page.Result[int]{Items: []int{2}, NextToken: "same"}, nil
			},
			limit: testBudget(),
		},
		{
			name: "item limit",
			fetch: func(context.Context, page.Request) (page.Result[int], error) {
				return page.Result[int]{Items: []int{1, 2}, NextToken: "more"}, nil
			},
			limit: Budget{PageSize: 2, MaxPages: 2, MaxItems: 1},
		},
		{
			name: "page limit",
			fetch: func(context.Context, page.Request) (page.Result[int], error) {
				return page.Result[int]{Items: []int{1}, NextToken: "more"}, nil
			},
			limit: Budget{PageSize: 1, MaxPages: 1, MaxItems: 5},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			items, err := Drain(context.Background(), test.limit, test.fetch)
			if err == nil || items != nil {
				t.Fatalf("items = %v, error = %v", items, err)
			}
			var structured *fault.Error
			if !errors.As(err, &structured) || structured.Validate() != nil {
				t.Fatalf("error = %#v, want valid structured fault", err)
			}
		})
	}
}

func TestDrainHonorsCancellationBeforeFetch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	items, err := Drain(ctx, testBudget(), func(context.Context, page.Request) (page.Result[int], error) {
		calls++
		return page.Result[int]{}, nil
	})
	if err == nil || items != nil || calls != 0 {
		t.Fatalf("items = %v, error = %v, calls = %d", items, err, calls)
	}
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Code != "operation_canceled" || !structured.Retryable {
		t.Fatalf("error = %#v, want retryable no-result cancellation", err)
	}
}

func TestDrainHonorsCancellationImmediatelyAfterFetchWithoutPartialResults(t *testing.T) {
	tests := []struct {
		name     string
		fetchErr error
	}{
		{name: "fetch returned a page"},
		{name: "fetch returned an error", fetchErr: errors.New("provider unavailable")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			calls := 0
			items, err := Drain(ctx, testBudget(), func(context.Context, page.Request) (page.Result[int], error) {
				calls++
				cancel()
				return page.Result[int]{Items: []int{1}}, test.fetchErr
			})
			if items != nil || err == nil || calls != 1 {
				t.Fatalf("items = %v, error = %v, calls = %d", items, err, calls)
			}
			var structured *fault.Error
			if !errors.As(err, &structured) || structured.Kind != fault.KindCanceled || structured.Code != "operation_canceled" || !structured.Retryable {
				t.Fatalf("error = %#v, want canceled structured fault", err)
			}
		})
	}
}

type stagedCancelContext struct {
	context.Context
	cancel   context.CancelFunc
	checks   int
	cancelAt int
}

func newStagedCancelContext(cancelAt int) *stagedCancelContext {
	ctx, cancel := context.WithCancel(context.Background())
	return &stagedCancelContext{Context: ctx, cancel: cancel, cancelAt: cancelAt}
}

func (c *stagedCancelContext) Err() error {
	c.checks++
	if c.checks == c.cancelAt {
		c.cancel()
	}
	return c.Context.Err()
}

func TestDrainRechecksCancellationBeforeSuccessfulCompletion(t *testing.T) {
	ctx := newStagedCancelContext(4)
	items, err := Drain[int](ctx, testBudget(), func(context.Context, page.Request) (page.Result[int], error) {
		return page.Result[int]{Items: []int{1}}, nil
	})
	if items != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("items = %v, error = %v, want nil and context.Canceled", items, err)
	}
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Kind != fault.KindCanceled || structured.Code != "operation_canceled" || !structured.Retryable {
		t.Fatalf("error = %#v, want canceled structured fault", err)
	}
	if ctx.checks != 4 {
		t.Fatalf("context checks = %d, want final success-boundary check", ctx.checks)
	}
}

func TestDrainReturnsPublicCopyOfStructuredFetchFailure(t *testing.T) {
	const canary = "raw-authorization-canary"
	privateCause := errors.New("provider response contained " + canary)
	providerFault := fault.Wrap(
		fault.KindUnavailable,
		"provider_unavailable",
		"the provider is unavailable",
		true,
		privateCause,
	)
	wrapped := fmt.Errorf("request header contained %s: %w", canary, providerFault)

	items, err := Drain(context.Background(), testBudget(), func(context.Context, page.Request) (page.Result[int], error) {
		return page.Result[int]{}, wrapped
	})
	if items != nil || err == nil {
		t.Fatalf("items = %v, error = %v", items, err)
	}
	if strings.Contains(err.Error(), canary) || errors.Unwrap(err) != nil || errors.Is(err, privateCause) {
		t.Fatalf("pagination leaked private fetch failure: %#v", err)
	}
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Code != "provider_unavailable" {
		t.Fatalf("error = %#v, want provider_unavailable", err)
	}
}

func TestDrainRejectsPageLargerThanRequestedWithoutPartialResults(t *testing.T) {
	calls := 0
	items, err := Drain(context.Background(), testBudget(), func(context.Context, page.Request) (page.Result[int], error) {
		calls++
		return page.Result[int]{Items: []int{1, 2, 3}}, nil
	})
	if items != nil || err == nil || calls != 1 {
		t.Fatalf("items = %v, error = %v, calls = %d", items, err, calls)
	}
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Kind != fault.KindContract || structured.Code != "invalid_page_contract" {
		t.Fatalf("error = %#v, want invalid_page_contract", err)
	}
}
