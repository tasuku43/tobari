package dockerruntime

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime"
)

const githubDeviceURL = "https://github.com/login/device"

var awsSSODeviceURLPattern = regexp.MustCompile(
	`^https://device\.sso\.[a-z]{2}(?:-[a-z0-9]+){1,3}-[1-9][0-9]?\.amazonaws\.com/$`,
)

type hostBrowserOpener interface {
	Open(context.Context, string) error
}

type osHostBrowserOpener struct{}

func (osHostBrowserOpener) Open(ctx context.Context, target string) error {
	executable, args, err := hostBrowserCommand(runtime.GOOS, target)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- executable is fixed and URL passes the closed GitHub/AWS login allowlist below.
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
	return target == githubDeviceURL || awsSSODeviceURLPattern.MatchString(target)
}
