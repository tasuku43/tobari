package dockerruntime

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

const githubDeviceURL = "https://github.com/login/device"

const (
	twgAuthorizationHost = "auth.atlassian.com"
	twgAuthorizationPath = "/oauth/activate"
)

const (
	githubAuthorizationHost      = "github.com"
	githubAuthorizationPath      = "/login/oauth/authorize"
	githubAuthorizationClientID  = "178c6fc778ccc68e1d6a"
	githubAuthorizationScope     = "repo read:org gist workflow"
	githubCallbackPath           = "/callback"
	codexAuthorizationHost       = "auth.openai.com"
	codexAuthorizationPath       = "/oauth/authorize"
	codexAuthorizationClientID   = "app_EMoamEEZ73f0CkXaXp7hrann"
	codexAuthorizationScope      = "openid profile email offline_access api.connectors.read api.connectors.invoke"
	codexCallbackPath            = "/auth/callback"
	workspaceMinimumCallbackPort = 1024
	codexOriginatorCLI           = "codex_cli_rs"
	codexOriginatorTUI           = "codex-tui"
)

var awsSSODeviceURLPattern = regexp.MustCompile(
	`^https://device\.sso\.[a-z]{2}(?:-[a-z0-9]+){1,3}-[1-9][0-9]?\.amazonaws\.com/$`,
)

var (
	awsConsoleRegionPattern = regexp.MustCompile(`^(?:us-(?:east|west)|eu-(?:central|north|south|west)|ap-(?:east|northeast|south|southeast)|ca-(?:central|west)|sa-east|me-(?:central|south)|af-south|il-central|mx-central|nz-north)-[0-9]+$`)
	awsConsoleStatePattern  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	awsPKCEChallengePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
	codexOAuthStatePattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)
	githubOAuthStatePattern = regexp.MustCompile(`^[0-9a-f]{20}$`)
	twgUserCodeQueryPattern = regexp.MustCompile(`^user_code=[A-Za-z0-9_-]{1,128}$`)
)

const awsConsoleClientID = "arn:aws:signin:::devtools/cross-device"

type hostBrowserOpener interface {
	Open(context.Context, string) error
}

type osHostBrowserOpener struct{}

func (osHostBrowserOpener) Open(ctx context.Context, target string) error {
	executable, args, err := hostBrowserCommand(runtime.GOOS, target)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- executable is fixed and URL passes the closed reviewed login allowlist below.
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}

func hostBrowserCommand(goos, target string) (string, []string, error) {
	if !validLoginBrowserTarget(target) {
		return "", nil, fmt.Errorf("unsupported browser target")
	}
	switch goos {
	case "darwin":
		return "/usr/bin/open", []string{target}, nil
	case "linux":
		return "/usr/bin/xdg-open", []string{target}, nil
	default:
		return "", nil, fmt.Errorf("host browser opening is unsupported on %s", goos)
	}
}

func validLoginBrowserTarget(target string) bool {
	if _, ok := parseWorkspaceLoginBrowserAction(target); ok {
		return true
	}
	return awsSSODeviceURLPattern.MatchString(target) || validAWSConsoleAuthorizationURL(target, "") ||
		validClaudeLoginAuthorizationURL(target) || validPupLoginAuthorizationURL(target)
}

func validTWGWorkspaceLoginVerificationURL(target string) bool {
	parsed, err := url.Parse(target)
	return err == nil && parsed.Scheme == "https" && parsed.User == nil && parsed.Host == twgAuthorizationHost &&
		parsed.Path == twgAuthorizationPath && parsed.RawPath == "" && !parsed.ForceQuery && parsed.Fragment == "" &&
		twgUserCodeQueryPattern.MatchString(parsed.RawQuery)
}

func validWorkspaceClaudeLoginAuthorizationURL(target string) bool {
	scopes, ok := claudeLoginAuthorizationScopes(target)
	if !ok {
		return false
	}
	want := strings.Fields(claudeWorkspaceLoginScope)
	slices.Sort(want)
	return slices.Equal(scopes, want)
}

type workspaceLoginAuthorization struct {
	callbackPort int
}

func validGitHubLoginAuthorizationURL(target string) bool {
	_, ok := parseGitHubLoginAuthorizationURL(target)
	return ok
}

func parseGitHubLoginAuthorizationURL(target string) (workspaceLoginAuthorization, bool) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Host != githubAuthorizationHost ||
		parsed.Path != githubAuthorizationPath || parsed.RawPath != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return workspaceLoginAuthorization{}, false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) != 4 || len(query["client_id"]) != 1 ||
		query["client_id"][0] != githubAuthorizationClientID || len(query["scope"]) != 1 ||
		!validGitHubScopeSubset(query["scope"][0]) || len(query["state"]) != 1 ||
		!githubOAuthStatePattern.MatchString(query["state"][0]) || len(query["redirect_uri"]) != 1 {
		return workspaceLoginAuthorization{}, false
	}
	for key := range query {
		switch key {
		case "client_id", "redirect_uri", "scope", "state":
		default:
			return workspaceLoginAuthorization{}, false
		}
	}
	callbackPort, ok := parseWorkspaceCallbackPort(query["redirect_uri"][0], "127.0.0.1", githubCallbackPath)
	if !ok {
		return workspaceLoginAuthorization{}, false
	}
	return workspaceLoginAuthorization{callbackPort: callbackPort}, true
}

func validGitHubScopeSubset(value string) bool {
	return validRequiredScopeSubset(value, githubAuthorizationScope, []string{"repo", "read:org", "gist"})
}

func validRequiredScopeSubset(value, ceiling string, required []string) bool {
	if value == "" || strings.ContainsAny(value, "\t\r\n") {
		return false
	}
	allowed := make(map[string]struct{})
	for _, scope := range strings.Split(ceiling, " ") {
		allowed[scope] = struct{}{}
	}
	seen := make(map[string]struct{})
	actual := strings.Split(value, " ")
	if len(actual) == 0 || len(actual) > len(allowed) {
		return false
	}
	for _, scope := range actual {
		if scope == "" {
			return false
		}
		if _, ok := allowed[scope]; !ok {
			return false
		}
		if _, duplicate := seen[scope]; duplicate {
			return false
		}
		seen[scope] = struct{}{}
	}
	for _, scope := range required {
		if _, ok := seen[scope]; !ok {
			return false
		}
	}
	return true
}

func validCodexLoginAuthorizationURL(target string) bool {
	_, ok := parseCodexLoginAuthorizationURL(target)
	return ok
}

func parseCodexLoginAuthorizationURL(target string) (workspaceLoginAuthorization, bool) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Host != codexAuthorizationHost ||
		parsed.Path != codexAuthorizationPath || parsed.RawPath != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return workspaceLoginAuthorization{}, false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) < 7 || len(query) > 10 {
		return workspaceLoginAuthorization{}, false
	}
	required := map[string]string{
		"response_type":         "code",
		"client_id":             codexAuthorizationClientID,
		"code_challenge_method": "S256",
	}
	optional := map[string]string{
		"id_token_add_organizations": "true",
		"codex_cli_simplified_flow":  "true",
	}
	for key, expected := range required {
		if len(query[key]) != 1 || query[key][0] != expected {
			return workspaceLoginAuthorization{}, false
		}
	}
	for key, expected := range optional {
		if values, present := query[key]; present && (len(values) != 1 || values[0] != expected) {
			return workspaceLoginAuthorization{}, false
		}
	}
	if values, present := query["originator"]; present && (len(values) != 1 ||
		(values[0] != codexOriginatorCLI && values[0] != codexOriginatorTUI)) {
		return workspaceLoginAuthorization{}, false
	}
	allowedKeys := map[string]struct{}{
		"response_type": {}, "client_id": {}, "code_challenge_method": {},
		"id_token_add_organizations": {}, "codex_cli_simplified_flow": {}, "originator": {},
		"code_challenge": {}, "state": {}, "scope": {}, "redirect_uri": {},
	}
	for key := range query {
		if _, allowed := allowedKeys[key]; !allowed {
			return workspaceLoginAuthorization{}, false
		}
	}
	if len(query["code_challenge"]) != 1 || !awsPKCEChallengePattern.MatchString(query["code_challenge"][0]) ||
		len(query["state"]) != 1 || !codexOAuthStatePattern.MatchString(query["state"][0]) ||
		len(query["scope"]) != 1 || !validCodexScopeSubset(query["scope"][0]) || len(query["redirect_uri"]) != 1 {
		return workspaceLoginAuthorization{}, false
	}
	callbackPort, ok := parseCodexCallbackPort(query["redirect_uri"][0])
	if !ok {
		return workspaceLoginAuthorization{}, false
	}
	return workspaceLoginAuthorization{callbackPort: callbackPort}, true
}

func validCodexScopeSubset(value string) bool {
	if value == "" || strings.ContainsAny(value, "\t\r\n") {
		return false
	}
	expected := strings.Split(codexAuthorizationScope, " ")
	actual := strings.Split(value, " ")
	if len(actual) == 0 || len(actual) > len(expected) {
		return false
	}
	remaining := make(map[string]struct{}, len(expected))
	for _, scope := range expected {
		remaining[scope] = struct{}{}
	}
	for _, scope := range actual {
		if scope == "" {
			return false
		}
		if _, ok := remaining[scope]; !ok {
			return false
		}
		delete(remaining, scope)
	}
	_, hasOpenID := remaining["openid"]
	return !hasOpenID
}

func parseCodexCallbackPort(target string) (int, bool) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawPath != "" ||
		parsed.Path != codexCallbackPath || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return 0, false
	}
	host := parsed.Hostname()
	if host != "localhost" && host != "127.0.0.1" {
		return 0, false
	}
	portText := parsed.Port()
	port, err := strconv.Atoi(portText)
	if err != nil || port < workspaceMinimumCallbackPort || port > 65535 || strconv.Itoa(port) != portText {
		return 0, false
	}
	if parsed.Host != host+":"+portText {
		return 0, false
	}
	return port, true
}

func parseWorkspaceCallbackPort(target, expectedHost, expectedPath string) (int, bool) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawPath != "" ||
		parsed.Path != expectedPath || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" ||
		parsed.Hostname() != expectedHost {
		return 0, false
	}
	portText := parsed.Port()
	port, err := strconv.Atoi(portText)
	if err != nil || port < workspaceMinimumCallbackPort || port > 65535 || strconv.Itoa(port) != portText ||
		parsed.Host != expectedHost+":"+portText {
		return 0, false
	}
	return port, true
}

func validAWSConsoleAuthorizationURL(target, expectedRegion string) bool {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" ||
		parsed.Path != "/v1/authorize" || parsed.RawPath != "" || parsed.ForceQuery ||
		parsed.Fragment != "" {
		return false
	}
	region := expectedRegion
	if region == "" {
		const suffix = ".signin.aws.amazon.com"
		host := parsed.Hostname()
		if len(host) <= len(suffix) || host[len(host)-len(suffix):] != suffix {
			return false
		}
		region = host[:len(host)-len(suffix)]
	}
	if !awsConsoleRegionPattern.MatchString(region) || parsed.Hostname() != region+".signin.aws.amazon.com" ||
		parsed.Host != parsed.Hostname() {
		return false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) != 7 {
		return false
	}
	values := map[string]string{
		"response_type":         "code",
		"client_id":             awsConsoleClientID,
		"code_challenge_method": "SHA-256",
		"scope":                 "openid",
		"redirect_uri":          "https://" + region + ".signin.aws.amazon.com/v1/sessions/confirmation",
	}
	for key, expected := range values {
		if len(query[key]) != 1 || query[key][0] != expected {
			return false
		}
	}
	if len(query["state"]) != 1 || !awsConsoleStatePattern.MatchString(query["state"][0]) ||
		len(query["code_challenge"]) != 1 || !awsPKCEChallengePattern.MatchString(query["code_challenge"][0]) {
		return false
	}
	return true
}
