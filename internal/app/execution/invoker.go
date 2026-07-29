// Package execution owns the policy-neutral boundary for external mutations.
package execution

import (
	"context"
	"fmt"

	"github.com/tasuku43/agentic-cli-foundry/internal/app/portcheck"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
)

// Policy applies the derived project's authorization, confirmation, dry-run,
// or other security decision. The template deliberately chooses none of those
// policies itself.
type Policy interface {
	Check(context.Context, operation.Intent) error
}

// Action performs one logical mutation. Transport-level retry, when proven
// safe by an API contract, stays behind the adapter called by this function.
type Action func(context.Context, operation.Intent) error

// Request binds a fully declared runtime intent to catalog- and parser-owned
// expectations. All fields are values so the invoker can snapshot them before
// policy evaluation.
type Request struct {
	Intent          operation.Intent
	ExpectedCommand string
	ExpectedEffect  operation.Effect
	ExpectedTarget  operation.TargetRef
	ExpectedImpact  operation.Impact
}

// Invoker is the only reusable application boundary supplied for mutations.
// It has no permissive default policy.
type Invoker struct {
	policy Policy
}

// New returns an invoker. A nil policy is retained and fails closed at Invoke
// time, which keeps composition errors observable without a panic.
func New(policy Policy) *Invoker {
	return &Invoker{policy: policy}
}

// Invoke validates and snapshots the declaration, applies policy, checks
// cancellation, and calls action exactly once. Every failure before the final
// call guarantees zero mutation attempts.
func (i *Invoker) Invoke(ctx context.Context, request Request, action Action) error {
	snapshot := request
	if err := validateRequest(snapshot); err != nil {
		return fault.Wrap(fault.KindContract, "invalid_mutation_contract", "mutation contract is invalid", false, err)
	}
	if action == nil {
		return fault.New(fault.KindContract, "missing_mutation_action", "mutation action is not configured", false)
	}
	if ctx == nil {
		return fault.New(fault.KindContract, "missing_context", "mutation context is not configured", false)
	}
	if err := ctx.Err(); err != nil {
		return fault.Wrap(fault.KindCanceled, "operation_canceled", "mutation was canceled before policy evaluation", true, err)
	}
	if i == nil || portcheck.IsNil(i.policy) {
		return fault.New(fault.KindRejected, "missing_mutation_policy", "mutation policy is not configured", false)
	}
	if err := i.policy.Check(ctx, snapshot.Intent); err != nil {
		return fault.Wrap(fault.KindRejected, "mutation_rejected", "mutation policy rejected the operation", false, err)
	}
	if err := ctx.Err(); err != nil {
		return fault.Wrap(fault.KindCanceled, "operation_canceled", "mutation was canceled before execution", true, err)
	}
	if err := action(ctx, snapshot.Intent); err != nil {
		return sanitizeMutationOutcomeError(err)
	}
	return nil
}

// sanitizeMutationOutcomeError runs only after the action was called. A valid
// structured adapter fault is authoritative and is detached from its private
// cause. Every other error, including raw context cancellation, leaves the
// mutation outcome unknown and must not become a retryable cancellation.
func sanitizeMutationOutcomeError(err error) error {
	if structured, ok := fault.PublicCopy(err); ok {
		return structured
	}
	return fault.New(
		fault.KindContract,
		"unclassified_mutation_outcome",
		"mutation action returned an unclassified outcome",
		false,
	)
}

func validateRequest(request Request) error {
	if request.ExpectedEffect != operation.EffectCreate && request.ExpectedEffect != operation.EffectWrite {
		return fmt.Errorf("execution boundary accepts create or write effects only")
	}
	if err := request.Intent.Validate(); err != nil {
		return err
	}
	if request.Intent.Command != request.ExpectedCommand {
		return fmt.Errorf("intent command does not match the resolved command")
	}
	if request.Intent.Effect != request.ExpectedEffect {
		return fmt.Errorf("intent effect does not match the catalog effect")
	}
	if request.Intent.Target != request.ExpectedTarget {
		return fmt.Errorf("intent target does not match the parsed target")
	}
	if request.Intent.Impact != request.ExpectedImpact {
		return fmt.Errorf("intent impact does not match the declared impact")
	}
	return nil
}
