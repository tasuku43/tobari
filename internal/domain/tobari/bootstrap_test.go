package tobari

import "testing"

func testAWSBootstrap() ContextAWSBootstrap {
	return ContextAWSBootstrap{Profile: "engineering", SSOSession: "company", SSOStartURL: "https://example.awsapps.com/start", SSORegion: "us-east-1", SSORegistrationScopes: []string{"sso:account:access"}, AccountID: "123456789012", RoleName: "Developer", Region: "ap-northeast-1", Output: "json"}
}

func TestContextBootstrapSemanticPreviewAndWorkspaceStates(t *testing.T) {
	first, err := NewContextBootstrapSnapshot(1, testAWSBootstrap())
	if err != nil {
		t.Fatal(err)
	}
	changedAWS := testAWSBootstrap()
	changedAWS.RoleName = "ReadOnly"
	second, err := NewContextBootstrapSnapshot(2, changedAWS)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := NewContextBootstrapPreview("coding", &first, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Changes) != 1 || preview.Changes[0] != "aws.sso_role_name" {
		t.Fatalf("preview changes = %+v", preview.Changes)
	}
	for _, test := range []struct {
		name    string
		applied string
		current *ContextBootstrapSnapshot
		want    string
	}{
		{name: "none", want: WorkspaceBootstrapNotConfigured},
		{name: "legacy existing", current: &second, want: WorkspaceBootstrapNotApplied},
		{name: "current", applied: second.Revision, current: &second, want: WorkspaceBootstrapCurrent},
		{name: "older", applied: first.Revision, current: &second, want: WorkspaceBootstrapOlder},
		{name: "removed recipe", applied: first.Revision, want: WorkspaceBootstrapOlder},
	} {
		t.Run(test.name, func(t *testing.T) {
			report, err := ResolveWorkspaceBootstrapReport(test.applied, test.current)
			if err != nil || report.State != test.want {
				t.Fatalf("report = %+v, error=%v, want %s", report, err, test.want)
			}
			if err := report.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestContextAWSBootstrapRejectsNonAWSStartURLAndScopeExplosion(t *testing.T) {
	invalid := testAWSBootstrap()
	invalid.SSOStartURL = "https://127.0.0.1/start"
	if err := invalid.Validate(); err == nil {
		t.Fatal("private non-AWS start URL was accepted")
	}
	invalid = testAWSBootstrap()
	invalid.SSORegistrationScopes = make([]string, 17)
	for index := range invalid.SSORegistrationScopes {
		invalid.SSORegistrationScopes[index] = string(rune('a' + index))
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("unbounded registration scopes were accepted")
	}
}
