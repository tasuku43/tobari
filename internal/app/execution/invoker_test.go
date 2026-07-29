package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
)

type policyStub struct {
	err   error
	calls int
	seen  operation.Intent
}

type typedNilPolicy struct{}

func (*typedNilPolicy) Check(context.Context, operation.Intent) error {
	panic("typed nil policy must not be called")
}

func (p *policyStub) Check(_ context.Context, intent operation.Intent) error {
	p.calls++
	p.seen = intent
	return p.err
}

func mutationRequest() Request {
	intent := operation.Intent{
		Command: "items update",
		Effect:  operation.EffectWrite,
		Target:  operation.TargetRef{Kind: "item", ParentID: "space-1", ID: "item-1"},
		Impact: operation.Impact{
			Cardinality:  operation.CardinalityOne,
			Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo,
			Destructive:  operation.DeclarationNo,
		},
	}
	return Request{
		Intent:          intent,
		ExpectedCommand: intent.Command,
		ExpectedEffect:  intent.Effect,
		ExpectedTarget:  intent.Target,
		ExpectedImpact:  intent.Impact,
	}
}

func TestInvokerAppliesPolicyAndExecutesExactlyOnce(t *testing.T) {
	policy := &policyStub{}
	actionCalls := 0
	request := mutationRequest()
	err := New(policy).Invoke(context.Background(), request, func(_ context.Context, got operation.Intent) error {
		actionCalls++
		if got != request.Intent {
			t.Fatalf("intent = %+v, want %+v", got, request.Intent)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if policy.calls != 1 || policy.seen != request.Intent || actionCalls != 1 {
		t.Fatalf("policy calls = %d, action calls = %d", policy.calls, actionCalls)
	}
}

func TestInvokerFailuresBeforeExecutionMakeZeroMutationAttempts(t *testing.T) {
	denied := errors.New("denied")
	tests := []struct {
		name    string
		invoker *Invoker
		ctx     context.Context
		request Request
		action  Action
	}{
		{name: "nil invoker", ctx: context.Background(), request: mutationRequest(), action: func(context.Context, operation.Intent) error { return nil }},
		{name: "nil policy", invoker: New(nil), ctx: context.Background(), request: mutationRequest(), action: func(context.Context, operation.Intent) error { return nil }},
		{name: "typed nil policy", invoker: New((*typedNilPolicy)(nil)), ctx: context.Background(), request: mutationRequest(), action: func(context.Context, operation.Intent) error { return nil }},
		{name: "nil context", invoker: New(&policyStub{}), request: mutationRequest(), action: func(context.Context, operation.Intent) error { return nil }},
		{name: "nil action", invoker: New(&policyStub{}), ctx: context.Background(), request: mutationRequest()},
		{name: "policy denial", invoker: New(&policyStub{err: denied}), ctx: context.Background(), request: mutationRequest(), action: func(context.Context, operation.Intent) error { return nil }},
	}
	mismatch := mutationRequest()
	mismatch.ExpectedTarget.ID = "item-2"
	tests = append(tests, struct {
		name    string
		invoker *Invoker
		ctx     context.Context
		request Request
		action  Action
	}{name: "target mismatch", invoker: New(&policyStub{}), ctx: context.Background(), request: mismatch, action: func(context.Context, operation.Intent) error { return nil }})

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests = append(tests, struct {
		name    string
		invoker *Invoker
		ctx     context.Context
		request Request
		action  Action
	}{name: "canceled", invoker: New(&policyStub{}), ctx: canceled, request: mutationRequest(), action: func(context.Context, operation.Intent) error { return nil }})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attempts := 0
			action := test.action
			if action != nil {
				action = func(ctx context.Context, intent operation.Intent) error {
					attempts++
					return test.action(ctx, intent)
				}
			}
			err := test.invoker.Invoke(test.ctx, test.request, action)
			if err == nil {
				t.Fatal("Invoke() succeeded")
			}
			if attempts != 0 {
				t.Fatalf("mutation attempts = %d, want 0", attempts)
			}
			var structured *fault.Error
			if !errors.As(err, &structured) || structured.Validate() != nil {
				t.Fatalf("error = %#v, want valid structured fault", err)
			}
			if test.name == "canceled" && !structured.Retryable {
				t.Fatalf("pre-action cancellation = %+v, want retryable after zero attempts", structured)
			}
		})
	}
}

func TestInvokerPreservesStructuredMutationOutcomeAndStripsCause(t *testing.T) {
	const canary = "private-provider-canary"
	providerFault := fault.Wrap(
		fault.KindUnavailable,
		"mutation_outcome_unknown",
		"the provider did not confirm the mutation outcome",
		false,
		fmt.Errorf("%s: %w", canary, context.DeadlineExceeded),
		fault.NextAction{Command: "items read", Reason: "reconcile the target before another mutation"},
	)
	actionCalls := 0
	err := New(&policyStub{}).Invoke(context.Background(), mutationRequest(), func(context.Context, operation.Intent) error {
		actionCalls++
		return providerFault
	})
	if actionCalls != 1 || err == nil {
		t.Fatalf("action calls = %d, error = %v", actionCalls, err)
	}
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Kind != fault.KindUnavailable ||
		structured.Code != "mutation_outcome_unknown" || structured.Retryable {
		t.Fatalf("structured outcome = %+v", structured)
	}
	if strings.Contains(err.Error(), canary) || errors.Unwrap(err) != nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("structured outcome retained private cause: %#v", err)
	}
}

func TestInvokerCollapsesUnclassifiedPostActionErrorsToUnknownOutcome(t *testing.T) {
	const canary = "private-action-canary"
	tests := map[string]func(context.CancelFunc) error{
		"plain error": func(context.CancelFunc) error { return errors.New(canary) },
		"raw cancellation": func(cancel context.CancelFunc) error {
			cancel()
			return fmt.Errorf("%s: %w", canary, context.Canceled)
		},
		"invalid structured fault": func(context.CancelFunc) error {
			return fault.Wrap(fault.KindUnavailable, "INVALID", "invalid adapter fault", false, errors.New(canary))
		},
	}
	for name, actionError := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			actionCalls := 0
			err := New(&policyStub{}).Invoke(ctx, mutationRequest(), func(context.Context, operation.Intent) error {
				actionCalls++
				return actionError(cancel)
			})
			if actionCalls != 1 || err == nil {
				t.Fatalf("action calls = %d, error = %v", actionCalls, err)
			}
			var structured *fault.Error
			if !errors.As(err, &structured) || structured.Kind != fault.KindContract ||
				structured.Code != "unclassified_mutation_outcome" || structured.Retryable {
				t.Fatalf("structured outcome = %+v", structured)
			}
			if strings.Contains(err.Error(), canary) || errors.Unwrap(err) != nil || errors.Is(err, context.Canceled) {
				t.Fatalf("unclassified outcome retained private cause: %#v", err)
			}
		})
	}
}

func TestInvokerDoesNotOverwriteConfirmedSuccessWithLateCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := New(&policyStub{}).Invoke(ctx, mutationRequest(), func(context.Context, operation.Intent) error {
		cancel()
		return nil
	})
	if err != nil || ctx.Err() == nil {
		t.Fatalf("Invoke() error = %v, context error = %v", err, ctx.Err())
	}
}
