package fault

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestFaultValidatesAndPreservesCause(t *testing.T) {
	cause := errors.New("upstream sentinel with secret-canary")
	err := Wrap(
		KindNotFound,
		"sample_not_found",
		"the requested sample does not exist",
		false,
		cause,
		NextAction{Command: "sample list", Reason: "discover a current sample ID"},
	)
	if validateErr := err.Validate(); validateErr != nil {
		t.Fatalf("Validate() error = %v", validateErr)
	}
	if !errors.Is(err, cause) {
		t.Fatal("fault did not preserve its cause")
	}
	if err.Error() != "the requested sample does not exist" {
		t.Fatalf("public error = %q, want reviewed message without cause", err.Error())
	}
	if err.NextActions[0].Command != "sample list" {
		t.Fatalf("next actions = %+v", err.NextActions)
	}
}

func TestPublicCopyStripsOuterWrappersAndPrivateCause(t *testing.T) {
	const canary = "credential-canary"
	cause := errors.New("upstream " + canary)
	original := Wrap(
		KindUnavailable,
		"provider_unavailable",
		"the provider is unavailable",
		true,
		cause,
		NextAction{Command: "items list", Reason: "retry the bounded read"},
	)
	wrapped := fmt.Errorf("request Authorization header contained %s: %w", canary, original)

	public, ok := PublicCopy(wrapped)
	if !ok {
		t.Fatal("PublicCopy() rejected a valid structured fault")
	}
	if strings.Contains(public.Error(), canary) || errors.Unwrap(public) != nil || errors.Is(public, cause) {
		t.Fatalf("public copy retained a private error: %#v", public)
	}
	data, err := json.Marshal(public)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), canary) {
		t.Fatalf("public JSON leaked cause: %s", data)
	}

	public.NextActions[0].Command = "changed"
	if original.NextActions[0].Command != "items list" {
		t.Fatal("PublicCopy() retained shared next-action storage")
	}
}

func TestPublicCopyRejectsNilAndMalformedFaults(t *testing.T) {
	if public, ok := PublicCopy(nil); ok || public != nil {
		t.Fatalf("PublicCopy(nil) = %#v, %t", public, ok)
	}
	malformed := &Error{Kind: KindUnavailable, Message: "missing code"}
	if public, ok := PublicCopy(fmt.Errorf("wrapped: %w", malformed)); ok || public != nil {
		t.Fatalf("PublicCopy(malformed) = %#v, %t", public, ok)
	}
}

func TestFaultRejectsIncompleteMachineContracts(t *testing.T) {
	tests := []*Error{
		nil,
		{},
		{Kind: Kind("new_kind"), Code: "code", Message: "message"},
		{Kind: KindInternal, Code: "INVALID", Message: "message"},
		{Kind: KindInternal, Code: "valid", Message: " "},
		{Kind: KindInternal, Code: "valid", Message: "line\nbreak"},
		{Kind: KindInternal, Code: "valid", Message: "line\u2028break"},
		{Kind: KindInternal, Code: "valid", Message: "paragraph\u2029break"},
		{Kind: KindRateLimited, Code: "rate_limited", Message: "wait", Retryable: true, RetryAfter: -time.Second},
		{Kind: KindNotFound, Code: "missing", Message: "missing", NextActions: []NextAction{{Command: "sample list"}}},
		{Kind: KindNotFound, Code: "missing", Message: "missing", NextActions: []NextAction{{Command: "sample list\nunsafe", Reason: "retry"}}},
		{Kind: KindNotFound, Code: "missing", Message: "missing", NextActions: []NextAction{{Command: "sample list", Reason: "line\u2028unsafe"}}},
	}
	for index, item := range tests {
		if err := item.Validate(); err == nil {
			t.Errorf("case %d Validate() succeeded", index)
		}
	}
}

func TestRateLimitTimingDoesNotAuthorizeRetry(t *testing.T) {
	rateLimited := &Error{
		Kind:       KindRateLimited,
		Code:       "mutation_rate_limited",
		Message:    "wait before deciding whether to retry",
		RetryAfter: 10 * time.Second,
	}
	if err := rateLimited.Validate(); err != nil {
		t.Fatalf("non-retryable rate-limit timing = %v", err)
	}

	unavailable := &Error{
		Kind:       KindUnavailable,
		Code:       "provider_unavailable",
		Message:    "the provider is unavailable",
		RetryAfter: time.Second,
	}
	if err := unavailable.Validate(); err == nil {
		t.Fatal("non-retryable non-rate-limit timing was accepted")
	}
}
