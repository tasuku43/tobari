package tobari

import (
	"bytes"
	"reflect"
	"testing"
)

func TestParseBoundedGraphQLEndpointUsesExistingExactRuleShape(t *testing.T) {
	endpoint, err := ParseBoundedGraphQLEndpoint("https://graphql.example.dev:8443/graphql")
	if err != nil {
		t.Fatal(err)
	}
	want := ManifestPolicyExactRule{Scheme: "https", Host: "graphql.example.dev", Port: 8443, Method: "POST", Path: "/graphql"}
	if !reflect.DeepEqual(endpoint, want) {
		t.Fatalf("endpoint=%+v want=%+v", endpoint, want)
	}
	if err := endpoint.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestParseBoundedGraphQLEndpointRejectsUnboundedURLShapes(t *testing.T) {
	for _, raw := range []string{
		"http://graphql.example.dev:8443/graphql",
		"https://graphql.example.dev/graphql",
		"https://graphql.example.dev:8443/graphql?query=1",
		"https://user:pass@graphql.example.dev:8443/graphql",
		"https://graphql.example.dev:8443/graphql#fragment",
		"https://graphql.example.dev:8443",
		"https://graphql.example.test:8443/graphql",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseBoundedGraphQLEndpoint(raw); err == nil {
				t.Fatalf("ParseBoundedGraphQLEndpoint(%q) unexpectedly succeeded", raw)
			}
		})
	}
}

func TestDefaultContextPolicySnapshotIsNormalizedAndStable(t *testing.T) {
	policy, ok := DefaultContextPolicySnapshot()
	if !ok {
		t.Fatal("default Workspace Manifest policy snapshot is unavailable")
	}
	normalized, encoded, revision, err := NormalizeContextPolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(policy, normalized) {
		t.Fatalf("default Workspace Manifest policy is not normalized: %#v != %#v", policy, normalized)
	}
	if !bytes.HasSuffix(encoded, []byte("\n")) {
		t.Fatalf("normalized Workspace Manifest policy snapshot has no trailing newline: %q", encoded)
	}
	if revision != DefaultContextPolicyRevision() || revision == "" {
		t.Fatalf("default Workspace Manifest policy revision = %q, helper = %q", revision, DefaultContextPolicyRevision())
	}
	if policy.Name != "default" || policy.MethodPolicy.Default != ManifestMethodExactReview || len(policy.BaselineGrants) == 0 {
		t.Fatalf("default Workspace Manifest policy baseline = %+v", policy)
	}
}

func TestComposeContextMethodPolicyOwnsTheCompleteMethodCeiling(t *testing.T) {
	policy, ok := DefaultContextPolicySnapshot()
	if !ok {
		t.Fatal("default Workspace Manifest policy snapshot is unavailable")
	}
	composed, err := ComposeContextMethodPolicy(policy, ManifestMethodPolicy{
		Default: ManifestMethodDeny,
		Overrides: []ManifestMethodOverride{{
			Method: "GET", Decision: ManifestMethodExactReview,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := composed.Validate(); err != nil {
		t.Fatal(err)
	}
	if composed.MethodPolicy.Decision("GET") != ManifestMethodExactReview || composed.MethodPolicy.Decision("POST") != ManifestMethodDeny {
		t.Fatalf("composed method policy = %+v", composed.MethodPolicy)
	}
	for _, rule := range composed.BaselineGrants {
		if rule.Method != "GET" {
			t.Fatalf("denied baseline grant survived composition: %+v", rule)
		}
	}
}
