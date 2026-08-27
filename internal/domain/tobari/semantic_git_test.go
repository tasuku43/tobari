package tobari

import "testing"

func semanticGitRule(host, service string) SemanticGitRule {
	return SemanticGitRule{
		SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: host, Port: 443},
		Service:               service, Repository: "/team/repo.git",
	}
}

func TestSemanticGitRuleMatchesDiscoveryAndRPCForExactServiceRepository(t *testing.T) {
	rule := semanticGitRule("git.example.com", "upload-pack")
	identity := PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolGit, GitService: "upload-pack", GitRepository: "/team/repo.git"}
	for _, effect := range []SemanticRequestEffect{
		{Scheme: "https", Host: rule.Host, Port: 443, Method: "GET", Path: "/team/repo.git/info/refs", Identity: identity},
		{Scheme: "https", Host: rule.Host, Port: 443, Method: "POST", Path: "/team/repo.git/git-upload-pack", Identity: identity},
	} {
		if !rule.Matches(effect) {
			t.Fatalf("Git rule did not match exact Smart HTTP effect: %+v", effect)
		}
	}
	changed := SemanticRequestEffect{Scheme: "https", Host: rule.Host, Port: 443, Method: "POST", Path: "/team/repo.git/git-receive-pack", Identity: identity}
	if rule.Matches(changed) {
		t.Fatal("Git rule matched a transport path inconsistent with the projected service")
	}
	changed.Path = "/other/repo.git/git-upload-pack"
	if rule.Matches(changed) {
		t.Fatal("Git rule matched a different repository transport")
	}
}

func TestSemanticGitRuleSeparatesFetchAndPush(t *testing.T) {
	fetch := semanticGitRule("git.example.com", "upload-pack")
	push := semanticGitRule("git.example.com", "receive-pack")
	effect := SemanticRequestEffect{
		Scheme: "https", Host: push.Host, Port: 443, Method: "POST", Path: "/team/repo.git/git-receive-pack",
		Identity: push.identity(),
	}
	if fetch.Matches(effect) || !push.Matches(effect) {
		t.Fatal("Git upload-pack and receive-pack authority were not separated")
	}
}

func TestSemanticGitPolicyRejectsDuplicatesAndCombinedShadow(t *testing.T) {
	allow := semanticGitRule("git-a.example.com", "receive-pack")
	allow.Host, allow.Hosts = "", []string{"git-a.example.com", "git-b.example.com"}
	first := semanticGitRule("git-a.example.com", "receive-pack")
	second := semanticGitRule("git-b.example.com", "receive-pack")
	policy := SemanticGitPolicy{
		Allow: SemanticGitRuleSet{Rules: []SemanticGitRule{allow}},
		Deny:  SemanticGitRuleSet{Rules: []SemanticGitRule{first, second}},
	}
	if err := policy.Validate(); err == nil {
		t.Fatal("Git Allow covered by combined Deny rules was accepted")
	}
	duplicates := SemanticGitRuleSet{Rules: []SemanticGitRule{first, first}}
	if err := duplicates.Validate(); err == nil {
		t.Fatal("duplicate Git rule was accepted")
	}
}

func TestSemanticGitRuleRejectsInvalidRepository(t *testing.T) {
	rule := semanticGitRule("git.example.com", "upload-pack")
	for _, repository := range []string{"team/repo.git", "/team/../repo.git", "/team/repo%2egit", "/team/repo.git\u2028"} {
		rule.Repository = repository
		if err := rule.Validate(); err == nil {
			t.Fatalf("invalid Git repository accepted: %q", repository)
		}
	}
}

func TestGitDynamicAuthorityRejectsTransportInconsistentProjection(t *testing.T) {
	denial := validPolicyDenial()
	denial.PolicyProtocolIdentity = PolicyProtocolIdentity{
		Scheme: "https", Protocol: PolicyProtocolGit,
		GitService: "receive-pack", GitRepository: "/team/repo.git",
	}
	denial.Host = "git.example.com"
	denial.Method = "POST"
	denial.Path = "/team/repo.git/git-receive-pack"
	if err := denial.Validate(); err != nil {
		t.Fatal(err)
	}
	candidate, err := NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	allow, err := NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	deny, err := NewExactPolicyDenyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}

	invalidDenial := denial
	invalidDenial.Method, invalidDenial.Path = "GET", "/unrelated"
	if err := invalidDenial.Validate(); err == nil {
		t.Fatal("transport-inconsistent Git denial was accepted")
	}

	allow.Method, allow.Path, allow.Examples = "GET", "/unrelated", []string{"/unrelated"}
	allow.ID = learnedRuleIDWithIdentity(
		allow.Match, allow.WorkspaceManifestID, allow.ProjectID, allow.Host, allow.Port,
		allow.Method, allow.Path, allow.Examples, allow.SourceCandidates, allow.PolicyProtocolIdentity,
	)
	if err := allow.Validate(); err == nil {
		t.Fatal("transport-inconsistent Git learned rule was accepted")
	}

	deny.Method, deny.Path = "GET", "/unrelated"
	deny.ID = policyDenyRuleIDWithIdentity(
		deny.WorkspaceManifestID, deny.ProjectID, deny.Host, deny.Port,
		deny.Method, deny.Path, deny.SourceCandidates, deny.PolicyProtocolIdentity,
	)
	if err := deny.Validate(); err == nil {
		t.Fatal("transport-inconsistent Git Deny rule was accepted")
	}

	body := PolicyMemoryRuleBody{
		PolicyProtocolIdentity: denial.PolicyProtocolIdentity,
		Match:                  PolicyMatchExact, Host: denial.Host, Port: denial.Port,
		Method: "GET", Path: "/unrelated", Examples: []string{"/unrelated"},
		SourceCandidates: []string{candidate.ID},
	}
	if err := body.Validate(PolicyMemoryAllow); err == nil {
		t.Fatal("transport-inconsistent Git Policy Memory body was accepted")
	}

	if semanticExactEffectMatches(
		SemanticRequestEffect{Scheme: denial.Scheme, Host: denial.Host, Port: denial.Port, Method: "GET", Path: "/unrelated", Identity: denial.PolicyProtocolIdentity},
		SemanticRequestEffect{Scheme: denial.Scheme, Host: denial.Host, Port: denial.Port, Method: "GET", Path: "/unrelated", Identity: denial.PolicyProtocolIdentity},
	) {
		t.Fatal("invalid Git effects matched exactly")
	}
}
