package tobari

import "testing"

func validPolicyDenial() PolicyDenial {
	return PolicyDenial{
		Timestamp: "2026-07-30T10:41:11Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		Host: "api.github.com", Method: "GET", Path: "/repos/cli/cli",
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	}
}

func TestDenialReportPreservesEmptyBoundedScope(t *testing.T) {
	t.Parallel()
	report := DenialReport{
		Task: TaskClusterDenials, PolicyDirectory: "/config/tobari/policy",
		WindowLines: 200, Items: []PolicyDenial{},
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	report.Items = nil
	if err := report.Validate(); err == nil {
		t.Fatal("unknown denial collection was accepted")
	}
}

func TestPolicyDenialRejectsInterpretationSensitiveFields(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*PolicyDenial){
		"timestamp":  func(value *PolicyDenial) { value.Timestamp = "recently" },
		"request id": func(value *PolicyDenial) { value.RequestID = "GET-api.github.com" },
		"host":       func(value *PolicyDenial) { value.Host = "api.github.com\nallow=true" },
		"method":     func(value *PolicyDenial) { value.Method = "GET POST" },
		"path":       func(value *PolicyDenial) { value.Path = "repos/cli/cli" },
		"status":     func(value *PolicyDenial) { value.StatusCode = 200 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := validPolicyDenial()
			mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("invalid denial was accepted")
			}
		})
	}
	profile := "profile\nunsafe"
	value := validPolicyDenial()
	value.CredentialProfile = &profile
	if err := value.Validate(); err == nil {
		t.Fatal("control-bearing credential profile was accepted")
	}
}

func TestPolicyActivationRequiresConfirmedTaskResult(t *testing.T) {
	t.Parallel()
	valid := PolicyActivation{
		Task: TaskPolicyApply, PolicyDirectory: "/config/tobari/policy", Applied: true,
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.Applied = false
	if err := valid.Validate(); err == nil {
		t.Fatal("unconfirmed activation was accepted")
	}
}

func TestPolicyCandidatesDeduplicateLatestEffectAndHideCoveredRules(t *testing.T) {
	t.Parallel()
	first := validPolicyDenial()
	latest := first
	latest.Timestamp = "2026-07-30T10:42:11Z"
	latest.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	other := first
	other.RequestID = "9185da2688d7469aae9cd9068e920b0b"
	other.Path = "/repos/cli/other"
	coveredCandidate, err := NewPolicyCandidate(other)
	if err != nil {
		t.Fatal(err)
	}
	coveredRule, err := NewExactLearnedPolicyRule(coveredCandidate)
	if err != nil {
		t.Fatal(err)
	}

	items, err := PolicyCandidates(
		[]PolicyDenial{first, latest, other}, []LearnedPolicyRule{coveredRule},
	)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := NewPolicyCandidate(latest)
	if len(items) != 1 || items[0] != want {
		t.Fatalf("candidates = %+v, want %+v", items, want)
	}
	original, _ := NewPolicyCandidate(first)
	if original.ID != want.ID || original.ObservedAt == want.ObservedAt {
		t.Fatalf("repeated exact effect did not retain a stable ID with latest evidence: original=%+v latest=%+v", original, want)
	}
}

func TestPolicyCandidatesExcludeDenialsThatExactRulesCannotResolve(t *testing.T) {
	t.Parallel()
	denial := validPolicyDenial()
	denial.Learnable = false
	items, err := PolicyCandidates(
		[]PolicyDenial{denial}, []LearnedPolicyRule{},
	)
	if err != nil || len(items) != 0 {
		t.Fatalf("ineligible denial candidates = %+v, error = %v", items, err)
	}
	if _, err := NewPolicyCandidate(denial); err == nil {
		t.Fatal("ineligible denial became a direct policy candidate")
	}
}

func TestExactLearnedRuleBindsCandidateAndDoesNotBroadenPath(t *testing.T) {
	t.Parallel()
	candidate, err := NewPolicyCandidate(validPolicyDenial())
	if err != nil {
		t.Fatal(err)
	}
	rule, err := NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	if !rule.Matches(candidate.Host, candidate.Method, candidate.Path) {
		t.Fatal("exact rule did not match its approved effect")
	}
	for _, changed := range []struct {
		host, method, path string
	}{
		{"uploads.github.com", candidate.Method, candidate.Path},
		{candidate.Host, "POST", candidate.Path},
		{candidate.Host, candidate.Method, candidate.Path + "/child"},
	} {
		if rule.Matches(changed.host, changed.method, changed.path) {
			t.Fatalf("exact rule broadened to %+v", changed)
		}
	}
	rule.Path += "/changed"
	if err := rule.Validate(); err == nil {
		t.Fatal("content-mismatched learned rule ID was accepted")
	}
}

func exactRuleForPath(t *testing.T, requestID, path string) LearnedPolicyRule {
	t.Helper()
	denial := validPolicyDenial()
	denial.RequestID = requestID
	denial.Host = "api.example.com"
	denial.Path = path
	candidate, err := NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func TestPolicyCompactionRequiresThreeSpecificSiblingRules(t *testing.T) {
	t.Parallel()
	rules := []LearnedPolicyRule{
		exactRuleForPath(t, "1185da2688d7469aae9cd9068e920b0b", "/api/v1/items/one"),
		exactRuleForPath(t, "2185da2688d7469aae9cd9068e920b0b", "/api/v1/items/two"),
	}
	items, err := PolicyCompactions(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("two-rule compactions = %+v", items)
	}

	rules = append(
		rules,
		exactRuleForPath(t, "3185da2688d7469aae9cd9068e920b0b", "/api/v1/items/three"),
	)
	items, err = PolicyCompactions(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].PathPrefix != "/api/v1/items/" ||
		items[0].OutsideCanary != "/api/v1/items-outside-tobari-canary" {
		t.Fatalf("compactions = %+v", items)
	}

	shallow := []LearnedPolicyRule{
		exactRuleForPath(t, "4185da2688d7469aae9cd9068e920b0b", "/items/one"),
		exactRuleForPath(t, "5185da2688d7469aae9cd9068e920b0b", "/items/two"),
		exactRuleForPath(t, "6185da2688d7469aae9cd9068e920b0b", "/items/three"),
	}
	items, err = PolicyCompactions(shallow)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("unsafe shallow compactions = %+v", items)
	}

	for name, unsafePath := range map[string]string{
		"encoded separator": "/api/v1/items/%2Fadmin",
		"dot segment":       "/api/v1/items/../admin",
		"empty segment":     "/api/v1/items//admin",
		"backslash":         `/api/v1/items\admin`,
	} {
		t.Run(name, func(t *testing.T) {
			rules := []LearnedPolicyRule{
				exactRuleForPath(t, "7185da2688d7469aae9cd9068e920b0b", "/api/v1/items/one"),
				exactRuleForPath(t, "8185da2688d7469aae9cd9068e920b0b", "/api/v1/items/two"),
				exactRuleForPath(t, "9185da2688d7469aae9cd9068e920b0b", unsafePath),
			}
			items, err := PolicyCompactions(rules)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 0 {
				t.Fatalf("unsafe path produced compaction: %+v", items)
			}
		})
	}
}

func TestCompactLearnedPolicyRulesPreservesExamplesAndRejectsStaleReference(t *testing.T) {
	t.Parallel()
	rules := []LearnedPolicyRule{
		exactRuleForPath(t, "1185da2688d7469aae9cd9068e920b0b", "/api/v1/items/one"),
		exactRuleForPath(t, "2185da2688d7469aae9cd9068e920b0b", "/api/v1/items/two"),
		exactRuleForPath(t, "3185da2688d7469aae9cd9068e920b0b", "/api/v1/items/three"),
	}
	items, err := PolicyCompactions(rules)
	if err != nil {
		t.Fatal(err)
	}
	updated, selected, prefixRule, err := CompactLearnedPolicyRules(rules, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || prefixRule.Match != PolicyMatchPrefix ||
		len(prefixRule.Examples) != 3 || len(prefixRule.SourceCandidates) != 3 ||
		selected.OutsideCanary != "/api/v1/items-outside-tobari-canary" {
		t.Fatalf("updated=%+v selected=%+v rule=%+v", updated, selected, prefixRule)
	}
	for _, example := range prefixRule.Examples {
		if !prefixRule.Matches(prefixRule.Host, prefixRule.Method, example) {
			t.Fatalf("prefix rule lost example %q", example)
		}
	}
	if prefixRule.Matches(
		prefixRule.Host, prefixRule.Method, selected.OutsideCanary,
	) {
		t.Fatal("prefix rule matched its outside boundary canary")
	}
	if _, _, _, err := CompactLearnedPolicyRules(
		updated, selected.ID,
	); err == nil {
		t.Fatal("stale compaction reference was accepted")
	}
}

func TestPolicyCandidateRejectsControlPathAndOpaqueKindMismatch(t *testing.T) {
	t.Parallel()
	denial := validPolicyDenial()
	denial.Path = "/safe\nunsafe"
	if _, err := NewPolicyCandidate(denial); err == nil {
		t.Fatal("control-bearing path became a candidate")
	}
	if err := ValidatePolicyCandidateID("pcx_0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("compaction reference was accepted as a candidate")
	}
	if err := ValidatePolicyCompactionID("pcy_0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("candidate reference was accepted as a compaction")
	}
}
