package dockerruntime

import (
	"net/url"
	"regexp"
	"strings"
)

const (
	awsSSOWorkspaceAuthorizationPath = "/authorize"
	awsSSOWorkspaceCallbackHost      = "127.0.0.1"
	awsSSOWorkspaceCallbackPath      = "/oauth/callback"
	awsSSOWorkspaceScope             = "sso:account:access"
)

var awsSSOWorkspaceClientIDPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{1,256}$`)

func parseAWSSSOWorkspaceAuthorizationURL(target string) (workspaceLoginAuthorization, bool) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" ||
		parsed.Path != awsSSOWorkspaceAuthorizationPath || parsed.RawPath != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.RawFragment != "" {
		return workspaceLoginAuthorization{}, false
	}
	const hostPrefix = "oidc."
	const hostSuffix = ".amazonaws.com"
	region, ok := strings.CutPrefix(parsed.Hostname(), hostPrefix)
	if !ok {
		return workspaceLoginAuthorization{}, false
	}
	region, ok = strings.CutSuffix(region, hostSuffix)
	if !ok || !awsConsoleRegionPattern.MatchString(region) || parsed.Host != hostPrefix+region+hostSuffix {
		return workspaceLoginAuthorization{}, false
	}

	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) != 7 {
		return workspaceLoginAuthorization{}, false
	}
	want := map[string]string{
		"response_type":         "code",
		"code_challenge_method": "S256",
		"scopes":                awsSSOWorkspaceScope,
	}
	for key, expected := range want {
		if len(query[key]) != 1 || query[key][0] != expected {
			return workspaceLoginAuthorization{}, false
		}
	}
	for key := range query {
		switch key {
		case "response_type", "client_id", "redirect_uri", "state", "scopes", "code_challenge", "code_challenge_method":
		default:
			return workspaceLoginAuthorization{}, false
		}
	}
	if len(query["client_id"]) != 1 || !awsSSOWorkspaceClientIDPattern.MatchString(query["client_id"][0]) ||
		len(query["state"]) != 1 || !awsConsoleStatePattern.MatchString(query["state"][0]) ||
		len(query["code_challenge"]) != 1 || !awsPKCEChallengePattern.MatchString(query["code_challenge"][0]) ||
		len(query["redirect_uri"]) != 1 {
		return workspaceLoginAuthorization{}, false
	}
	callbackPort, ok := parseWorkspaceCallbackPort(
		query["redirect_uri"][0], awsSSOWorkspaceCallbackHost, awsSSOWorkspaceCallbackPath,
	)
	if !ok {
		return workspaceLoginAuthorization{}, false
	}
	return workspaceLoginAuthorization{callbackPort: callbackPort}, true
}
