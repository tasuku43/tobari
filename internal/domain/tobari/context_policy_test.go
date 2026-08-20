package tobari

import (
	"bytes"
	"reflect"
	"testing"
)

func TestDefaultContextPolicySnapshotIsNormalizedAndStable(t *testing.T) {
	policy, ok := DefaultContextPolicySnapshot()
	if !ok {
		t.Fatal("default Context policy snapshot is unavailable")
	}
	normalized, encoded, revision, err := NormalizeContextPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(policy, normalized) {
		t.Fatalf("default Context policy is not normalized: %#v != %#v", policy, normalized)
	}
	if !bytes.HasSuffix(encoded, []byte("\n")) {
		t.Fatalf("normalized Context policy snapshot has no trailing newline: %q", encoded)
	}
	if revision != DefaultContextPolicyRevision() || revision == "" {
		t.Fatalf("default Context policy revision = %q, helper = %q", revision, DefaultContextPolicyRevision())
	}
	if policy.Name != "default" || policy.MethodPolicy.Default != ContextMethodExactReview || len(policy.BaselineGrants) == 0 {
		t.Fatalf("default Context policy baseline = %+v", policy)
	}
}

func TestComposeContextMethodPolicyOwnsTheCompleteMethodCeiling(t *testing.T) {
	policy, ok := DefaultContextPolicySnapshot()
	if !ok {
		t.Fatal("default Context policy snapshot is unavailable")
	}
	composed, err := ComposeContextMethodPolicy(policy, ContextMethodPolicy{
		Default: ContextMethodDeny,
		Overrides: []ContextMethodOverride{{
			Method: "GET", Decision: ContextMethodExactReview,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := composed.Validate(); err != nil {
		t.Fatal(err)
	}
	if composed.MethodPolicy.Decision("GET") != ContextMethodExactReview || composed.MethodPolicy.Decision("POST") != ContextMethodDeny {
		t.Fatalf("composed method policy = %+v", composed.MethodPolicy)
	}
	for _, rule := range composed.BaselineGrants {
		if rule.Method != "GET" {
			t.Fatalf("denied baseline grant survived composition: %+v", rule)
		}
	}
}
