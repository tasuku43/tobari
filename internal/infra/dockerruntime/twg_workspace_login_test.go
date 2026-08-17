package dockerruntime

import (
	"strings"
	"testing"
)

const syntheticTWGVerificationURL = "https://auth.atlassian.com/oauth/activate?user_code=ABCD-EFGH"

func TestTWGWorkspaceLoginVerificationURLIsExactAndHostOpenOnly(t *testing.T) {
	if !validTWGWorkspaceLoginVerificationURL(syntheticTWGVerificationURL) || !validLoginBrowserTarget(syntheticTWGVerificationURL) {
		t.Fatal("reviewed TWG verification URL was rejected")
	}
	action, ok := parseWorkspaceLoginBrowserAction(syntheticTWGVerificationURL)
	if !ok || action.relayCallback || action.callbackPort != 0 {
		t.Fatalf("TWG browser action = %+v, ok=%t", action, ok)
	}
	for _, target := range []string{
		"http://auth.atlassian.com/oauth/activate?user_code=ABCD-EFGH",
		"https://AUTH.ATLASSIAN.COM/oauth/activate?user_code=ABCD-EFGH",
		"https://user@auth.atlassian.com/oauth/activate?user_code=ABCD-EFGH",
		"https://auth.atlassian.com:443/oauth/activate?user_code=ABCD-EFGH",
		"https://auth.atlassian.com/oauth/activate/?user_code=ABCD-EFGH",
		"https://auth.atlassian.com/oauth/device?user_code=ABCD-EFGH",
		"https://auth.atlassian.com/oauth/activate",
		"https://auth.atlassian.com/oauth/activate?user_code=",
		"https://auth.atlassian.com/oauth/activate?user_code=ABCD%2DEFGH",
		"https://auth.atlassian.com/oauth/activate?user_code=ABCD+EFGH",
		"https://auth.atlassian.com/oauth/activate?user_code=ABCD-EFGH&next=other",
		"https://auth.atlassian.com/oauth/activate?user_code=ABCD-EFGH&user_code=IJKL-MNOP",
		"https://auth.atlassian.com/oauth/activate?user_code=ABCD-EFGH#fragment",
		"https://auth.atlassian.example/oauth/activate?user_code=ABCD-EFGH",
		"https://auth.atlassian.com/oauth/activate?user_code=" + strings.Repeat("A", 129),
	} {
		if validTWGWorkspaceLoginVerificationURL(target) {
			t.Fatalf("unsafe TWG verification URL accepted: %q", target)
		}
	}
}
