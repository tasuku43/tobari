package dockerruntime

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
)

const githubDeviceURL = "https://github.com/login/device"

var awsSSODeviceURLPattern = regexp.MustCompile(
	`^https://device\.sso\.[a-z]{2}(?:-[a-z0-9]+){1,3}-[1-9][0-9]?\.amazonaws\.com/$`,
)

var (
	awsConsoleRegionPattern = regexp.MustCompile(`^(?:us-(?:east|west)|eu-(?:central|north|south|west)|ap-(?:east|northeast|south|southeast)|ca-(?:central|west)|sa-east|me-(?:central|south)|af-south|il-central|mx-central|nz-north)-[0-9]+$`)
	awsConsoleStatePattern  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	awsPKCEChallengePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
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
	command := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- executable is fixed and URL passes the closed GitHub/AWS/Claude login allowlist below.
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
	return target == githubDeviceURL || awsSSODeviceURLPattern.MatchString(target) ||
		validAWSConsoleAuthorizationURL(target, "") || validClaudeLoginAuthorizationURL(target)
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
