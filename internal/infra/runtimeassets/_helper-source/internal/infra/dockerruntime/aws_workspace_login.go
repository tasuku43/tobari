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

var awsSSOWorkspaceAuthorizationQuerySchema = workspaceLoginQuerySchema{
	"response_type": {
		required: true,
		validate: exactWorkspaceLoginQueryValue("code"),
	},
	"client_id": {
		required: true,
		validate: awsSSOWorkspaceClientIDPattern.MatchString,
	},
	"redirect_uri": {
		required: true,
		validate: nonEmptyWorkspaceLoginQueryValue,
	},
	"state": {
		required: true,
		validate: awsConsoleStatePattern.MatchString,
	},
	"scopes": {
		required: true,
		validate: exactWorkspaceLoginQueryValue(awsSSOWorkspaceScope),
	},
	"code_challenge": {
		required: true,
		validate: awsPKCEChallengePattern.MatchString,
	},
	"code_challenge_method": {
		required: true,
		validate: exactWorkspaceLoginQueryValue("S256"),
	},
}

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
	if err != nil || !awsSSOWorkspaceAuthorizationQuerySchema.valid(query) {
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
