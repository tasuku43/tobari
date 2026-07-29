package apicall

import (
	"testing"
	"time"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
)

func TestPolicyAllowsExplicitSafeContracts(t *testing.T) {
	tests := []struct {
		effect operation.Effect
		policy Policy
	}{
		{effect: operation.EffectRead, policy: SingleAttempt(5*time.Second, IdempotencySafe)},
		{effect: operation.EffectRead, policy: Policy{Timeout: time.Second, MaxAttempts: 3, Idempotency: IdempotencySafe}},
		{effect: operation.EffectWrite, policy: SingleAttempt(time.Second, IdempotencyUnsafe)},
		{effect: operation.EffectCreate, policy: Policy{Timeout: time.Second, MaxAttempts: 2, Idempotency: IdempotencyKeyed, IdempotencyKey: "logical-operation-1"}},
	}
	for _, test := range tests {
		if err := test.policy.Validate(test.effect); err != nil {
			t.Errorf("Validate(%s) error = %v", test.effect, err)
		}
	}
}

func TestPolicyRejectsOmittedOrUnsafeRetryContracts(t *testing.T) {
	tests := []struct {
		effect operation.Effect
		policy Policy
	}{
		{effect: operation.EffectRead, policy: Policy{}},
		{effect: operation.EffectRead, policy: Policy{Timeout: time.Second, MaxAttempts: 1}},
		{effect: operation.EffectWrite, policy: Policy{Timeout: time.Second, MaxAttempts: 2, Idempotency: IdempotencyUnsafe}},
		{effect: operation.EffectRead, policy: Policy{Timeout: time.Second, MaxAttempts: 2, Idempotency: IdempotencyUnsafe}},
		{effect: operation.EffectCreate, policy: Policy{Timeout: time.Second, MaxAttempts: 2, Idempotency: IdempotencyKeyed}},
		{effect: operation.EffectCreate, policy: Policy{Timeout: time.Second, MaxAttempts: 2, Idempotency: IdempotencyKeyed, IdempotencyKey: "key\nunsafe"}},
		{effect: operation.EffectCreate, policy: Policy{Timeout: time.Second, MaxAttempts: 2, Idempotency: IdempotencyKeyed, IdempotencyKey: "key\u2028unsafe"}},
		{effect: operation.EffectCreate, policy: Policy{Timeout: time.Second, MaxAttempts: 2, Idempotency: IdempotencyKeyed, IdempotencyKey: "key\u2029unsafe"}},
		{effect: operation.EffectRead, policy: Policy{Timeout: time.Second, MaxAttempts: 1, Idempotency: IdempotencySafe, IdempotencyKey: "unexpected"}},
	}
	for _, test := range tests {
		if err := test.policy.Validate(test.effect); err == nil {
			t.Errorf("Validate(%s, %+v) succeeded", test.effect, test.policy)
		}
	}
}
