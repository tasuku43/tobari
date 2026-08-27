package tobari

import (
	"strings"
	"testing"
)

func semanticOCIRule(host, action, repository, object string) SemanticOCIRule {
	return SemanticOCIRule{
		SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: host, Port: 443},
		Action:                action, Repository: repository, Object: object,
	}
}

func TestSemanticOCIRulesMatchExactDistributionEffects(t *testing.T) {
	tests := []struct {
		action, repository, object, method, path string
	}{
		{"list", "", "catalog", "GET", "/v2/_catalog"},
		{"list", "team/app", "tags", "GET", "/v2/team/app/tags/list"},
		{"pull", "team/app", "manifest:latest", "GET", "/v2/team/app/manifests/latest"},
		{"pull", "team/app", "blob:sha256:abc", "HEAD", "/v2/team/app/blobs/sha256:abc"},
		{"pull", "team/app", "referrers:sha256:abc", "GET", "/v2/team/app/referrers/sha256:abc"},
		{"push", "team/app", "manifest:latest", "PUT", "/v2/team/app/manifests/latest"},
		{"start_upload", "team/app", "upload", "POST", "/v2/team/app/blobs/uploads/"},
		{"upload_status", "team/app", "upload:session-1", "GET", "/v2/team/app/blobs/uploads/session-1"},
		{"upload_chunk", "team/app", "upload:session-1", "PATCH", "/v2/team/app/blobs/uploads/session-1"},
		{"complete_upload", "team/app", "blob:sha256%3Aabc", "POST", "/v2/team/app/blobs/uploads/"},
		{"complete_upload", "team/app", "upload:session-1:blob:sha256%3Aabc", "PUT", "/v2/team/app/blobs/uploads/session-1"},
		{"mount", "team/app", "mount:sha256%3Aabc:from:shared%2Fbase", "POST", "/v2/team/app/blobs/uploads/"},
		{"cancel_upload", "team/app", "upload:session-1", "DELETE", "/v2/team/app/blobs/uploads/session-1"},
	}
	for _, tt := range tests {
		rule := semanticOCIRule("registry.example.com", tt.action, tt.repository, tt.object)
		effect := SemanticRequestEffect{
			Scheme: "https", Host: rule.Host, Port: 443, Method: tt.method, Path: tt.path,
			Identity: rule.identity(),
		}
		if !rule.Matches(effect) {
			t.Fatalf("OCI rule did not match exact effect: %+v", effect)
		}
		changed := effect
		changed.Path += "/other"
		if rule.Matches(changed) {
			t.Fatalf("OCI rule matched changed path: %+v", changed)
		}
	}
}

func TestSemanticOCIPolicyRejectsCombinedShadowAndInvalidCoordinates(t *testing.T) {
	allow := semanticOCIRule("registry-a.example.com", "push", "team/app", "manifest:latest")
	allow.Host, allow.Hosts = "", []string{"registry-a.example.com", "registry-b.example.com"}
	first := semanticOCIRule("registry-a.example.com", "push", "team/app", "manifest:latest")
	second := semanticOCIRule("registry-b.example.com", "push", "team/app", "manifest:latest")
	policy := SemanticOCIPolicy{
		Allow: SemanticOCIRuleSet{Rules: []SemanticOCIRule{allow}},
		Deny:  SemanticOCIRuleSet{Rules: []SemanticOCIRule{first, second}},
	}
	if err := policy.Validate(); err == nil {
		t.Fatal("OCI Allow covered by combined Deny rules was accepted")
	}
	first.Repository = "team/../app"
	if err := first.Validate(); err == nil {
		t.Fatal("invalid OCI repository was accepted")
	}
	for _, object := range []string{
		"manifest:a/b",
		"upload:%41:blob:sha256%3Aabc",
		"upload:%FF:blob:sha256%3Aabc",
		"mount:%41:from:..%2Fbad",
		"blob:a/b",
		"blob:" + strings.Repeat("a", 513),
	} {
		identity := PolicyProtocolIdentity{
			Scheme: "https", Protocol: PolicyProtocolOCI,
			OCIAction: "complete_upload", OCIRepository: "team/app", OCIObject: object,
		}
		if strings.HasPrefix(object, "manifest:") {
			identity.OCIAction = "pull"
		} else if strings.HasPrefix(object, "mount:") {
			identity.OCIAction = "mount"
		}
		if err := identity.Validate(); err == nil {
			t.Fatalf("invalid OCI object was accepted: %q", object)
		}
	}
}

func TestOCIDynamicAuthorityRejectsTransportInconsistentProjection(t *testing.T) {
	identity := PolicyProtocolIdentity{
		Scheme: "https", Protocol: PolicyProtocolOCI,
		OCIAction: "push", OCIRepository: "team/app", OCIObject: "manifest:latest",
	}
	invalid := SemanticRequestEffect{
		Scheme: "https", Host: "registry.example.com", Port: 443,
		Method: "GET", Path: "/unrelated", Identity: identity,
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("transport-inconsistent OCI effect was accepted")
	}
	if semanticExactEffectMatches(invalid, invalid) {
		t.Fatal("transport-inconsistent OCI effects matched")
	}
}

func TestOCICompleteUploadBindsExactSession(t *testing.T) {
	identity := PolicyProtocolIdentity{
		Scheme: "https", Protocol: PolicyProtocolOCI,
		OCIAction: "complete_upload", OCIRepository: "team/app",
		OCIObject: "upload:session-1:blob:sha256%3Aabc",
	}
	valid := SemanticRequestEffect{
		Scheme: "https", Host: "registry.example.com", Port: 443,
		Method: "PUT", Path: "/v2/team/app/blobs/uploads/session-1", Identity: identity,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("exact OCI upload completion was rejected: %v", err)
	}
	changed := valid
	changed.Path = "/v2/team/app/blobs/uploads/session-2"
	if err := changed.Validate(); err == nil {
		t.Fatal("OCI upload completion matched a different session")
	}
}

func TestOCIReferrersMethodBoundaryShadowUsesGETOnly(t *testing.T) {
	modules := EmptyWorkspaceTemplateSemanticModules()
	modules.Protocols.HTTP.OCI.Allow.Rules = []SemanticOCIRule{
		semanticOCIRule("registry.example.com", "pull", "team/app", "referrers:sha256:abc"),
	}
	if err := modules.Validate([]string{"GET"}); err == nil {
		t.Fatal("GET-denied referrers Allow remained reachable")
	}

	for _, object := range []string{"manifest:latest", "blob:sha256:abc"} {
		modules.Protocols.HTTP.OCI.Allow.Rules[0].Object = object
		if err := modules.Validate([]string{"GET"}); err != nil {
			t.Fatalf("GET-denied %s Allow lost its HEAD path: %v", object, err)
		}
	}
}
