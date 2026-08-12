package tobari

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuiltinPolicyPresetsHaveNoImmediateGrantsAndStableRevisions(t *testing.T) {
	for _, origin := range []string{"builtin/offline", DefaultPolicyPresetOrigin, "builtin/get-only-reviewed"} {
		preset, ok := BuiltinPolicyPreset(origin)
		if !ok {
			t.Fatalf("missing built-in %q", origin)
		}
		normalized, first, revision, err := NormalizePolicyPreset(preset)
		if err != nil {
			t.Fatal(err)
		}
		_, second, repeated, err := NormalizePolicyPreset(normalized)
		if err != nil || !bytes.Equal(first, second) || revision != repeated || len(normalized.BaselineGrants) != 0 || !strings.HasPrefix(revision, "sha256:") {
			t.Fatalf("built-in normalization = %q/%q grants=%d err=%v", revision, repeated, len(normalized.BaselineGrants), err)
		}
	}
}

func TestPolicyPresetRejectsNonExactOrReservedDestinations(t *testing.T) {
	for _, host := range []string{"mock-upstream", "*.example.com", "127.0.0.1", "::1", "localhost", "service.local", "service.internal", "service.test", "service.invalid", "service.example", "example.com", "UPPER.example.net", "bad_name.example.net", "trailing.example.net.", "é.example.net"} {
		authority := PolicyPresetAuthority{Scheme: "https", Host: host, Port: 443}
		if err := authority.Validate(); err == nil {
			t.Fatalf("unsafe destination %q was accepted", host)
		}
	}
	if err := (PolicyPresetAuthority{Scheme: "https", Host: "api.github.com", Port: 443}).Validate(); err != nil {
		t.Fatalf("public exact destination rejected: %v", err)
	}
}

func TestPolicyPresetRejectsDuplicatesAndExecutableShapedUnknownDataAtStrictDecoder(t *testing.T) {
	preset, _ := BuiltinPolicyPreset(DefaultPolicyPresetOrigin)
	preset.MethodCeiling = PolicyPresetMethodCeiling{Mode: "exact", Methods: []string{"GET", "GET"}}
	if err := preset.Validate(); err == nil {
		t.Fatal("duplicate methods accepted")
	}
	preset, _ = BuiltinPolicyPreset(DefaultPolicyPresetOrigin)
	preset.DestinationCeiling = PolicyPresetDestinationCeiling{Mode: "exact", Authorities: []PolicyPresetAuthority{{Scheme: "https", Host: "api.github.com", Port: 443}, {Scheme: "https", Host: "api.github.com", Port: 443}}}
	if err := preset.Validate(); err == nil {
		t.Fatal("duplicate authorities accepted")
	}
}

func TestPolicyPresetRulesCannotExceedDestinationOrMethodCeilings(t *testing.T) {
	base := PolicyPreset{SchemaVersion: 1, Name: "bounded", Guardrail: PolicyPresetGuardrailReviewedExact, DestinationCeiling: PolicyPresetDestinationCeiling{Mode: "exact", Authorities: []PolicyPresetAuthority{{Scheme: "https", Host: "api.github.com", Port: 443}}}, MethodCeiling: PolicyPresetMethodCeiling{Mode: "exact", Methods: []string{"GET", "POST"}}, BaselineGrants: []PolicyPresetExactRule{}, BaselineDenies: []PolicyPresetExactRule{}, GraphQLEndpoints: []PolicyPresetExactRule{}}
	inside := PolicyPresetExactRule{Scheme: "https", Host: "api.github.com", Port: 443, Method: "GET", Path: "/user"}
	for name, assign := range map[string]func(*PolicyPreset){
		"grant destination": func(p *PolicyPreset) {
			p.BaselineGrants = []PolicyPresetExactRule{{Scheme: "https", Host: "uploads.github.com", Port: 443, Method: "GET", Path: "/"}}
		},
		"deny method": func(p *PolicyPreset) {
			p.BaselineDenies = []PolicyPresetExactRule{{Scheme: "https", Host: "api.github.com", Port: 443, Method: "DELETE", Path: "/"}}
		},
		"GraphQL scheme": func(p *PolicyPreset) {
			p.GraphQLEndpoints = []PolicyPresetExactRule{{Scheme: "http", Host: "api.github.com", Port: 443, Method: "GET", Path: "/graphql"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			assign(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("out-of-ceiling rule accepted")
			}
		})
	}
	for _, assign := range []func(*PolicyPreset){func(p *PolicyPreset) { p.BaselineGrants = []PolicyPresetExactRule{inside} }, func(p *PolicyPreset) { p.BaselineDenies = []PolicyPresetExactRule{inside} }, func(p *PolicyPreset) {
		graphql := inside
		graphql.Method = "POST"
		p.GraphQLEndpoints = []PolicyPresetExactRule{graphql}
	}} {
		candidate := base
		assign(&candidate)
		if err := candidate.Validate(); err != nil {
			t.Fatalf("inside-ceiling rule rejected: %v", err)
		}
	}
	preset := base
	preset.GraphQLEndpoints = []PolicyPresetExactRule{inside}
	if err := preset.Validate(); err == nil {
		t.Fatal("non-POST GraphQL endpoint accepted")
	}
}

func TestSchemeChangesGuidedCandidateIdentity(t *testing.T) {
	base := PolicyDenial{PolicyProtocolIdentity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP}, Timestamp: "2026-08-12T00:00:00Z", RequestID: strings.Repeat("a", 32), ContextID: "018bcfe5-687b-7000-8000-000000000000", ContextName: "default", ProjectID: "018bcfe5-687b-7000-8000-000000000001", ProjectRoot: "/project", Host: "api.github.com", Port: 443, Method: "GET", Path: "/repos", Reason: "review", StatusCode: 403, Learnable: true}
	httpsCandidate, err := NewPolicyCandidate(base)
	if err != nil {
		t.Fatal(err)
	}
	base.Scheme = "http"
	httpCandidate, err := NewPolicyCandidate(base)
	if err != nil {
		t.Fatal(err)
	}
	if httpsCandidate.ID == httpCandidate.ID {
		t.Fatal("candidate identity omitted scheme")
	}
}
