package tobari

import (
	"reflect"
	"testing"
)

func TestPolicyFiniteVocabulariesAreCanonicalAndCallerImmutable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		values func() []string
		want   []string
	}{
		{name: "match", values: PolicyMatchValues, want: []string{PolicyMatchExact, PolicyMatchPathTemplate}},
		{name: "protocol", values: PolicyProtocolValues, want: []string{PolicyProtocolHTTP, PolicyProtocolGraphQL, PolicyProtocolMCP, PolicyProtocolAWS, PolicyProtocolKubernetes, PolicyProtocolGit, PolicyProtocolOCI}},
		{name: "decision", values: PolicyDecisionValues, want: []string{PolicyDecisionAllow, PolicyDecisionDeny}},
		{name: "state change", values: PolicyStateChangeValues, want: []string{PolicyStateChangeNone, PolicyStateChangePossible, PolicyStateChangeInteractive, PolicyStateChangeUnknown}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			first := test.values()
			if !reflect.DeepEqual(first, test.want) {
				t.Fatalf("values = %v, want %v", first, test.want)
			}
			seen := make(map[string]struct{}, len(first))
			for _, value := range first {
				if value == "" {
					t.Fatal("canonical vocabulary contains an empty value")
				}
				if _, duplicate := seen[value]; duplicate {
					t.Fatalf("canonical vocabulary contains duplicate %q", value)
				}
				seen[value] = struct{}{}
			}
			first[0] = "caller_mutation"
			first = append(first, "caller_append")
			if got := test.values(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("caller changed canonical vocabulary: %v", got)
			}
		})
	}
}

func TestPolicyFiniteVocabularyValidationAndDerivedStateChangeStayClosed(t *testing.T) {
	t.Parallel()
	if err := (PolicyProtocolIdentity{Scheme: "https", Protocol: "future"}).Validate(); err == nil {
		t.Fatal("unknown policy protocol was accepted")
	}
	candidate, err := NewPolicyCandidate(validPolicyDenial())
	if err != nil {
		t.Fatal(err)
	}
	rule, err := NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	unknownMatch := rule
	unknownMatch.Match = "future"
	if err := unknownMatch.Validate(); err == nil {
		t.Fatal("unknown policy match was accepted")
	}
	item, err := NewPolicyRuleFromLearned(rule)
	if err != nil {
		t.Fatal(err)
	}
	item.Decision = "future"
	if err := item.Validate(); err == nil {
		t.Fatal("unknown policy decision was accepted")
	}

	identities := []PolicyProtocolIdentity{
		{Scheme: "https", Protocol: PolicyProtocolHTTP},
		{Scheme: "https", Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationQuery, GraphQLRootField: "viewer"},
		{Scheme: "https", Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationMutation, GraphQLRootField: "updateIssue"},
		{Scheme: "https", Protocol: PolicyProtocolKubernetes, KubernetesVerb: "connect", KubernetesResource: "core/v1/pods/exec", KubernetesDryRun: "none"},
	}
	for _, identity := range identities {
		if err := identity.Validate(); err != nil {
			t.Fatalf("valid identity %+v: %v", identity, err)
		}
		if !policyVocabularyContains(policyStateChangeValues[:], identity.StateChangePotential()) {
			t.Fatalf("derived state change %q is outside canonical values", identity.StateChangePotential())
		}
	}
}
