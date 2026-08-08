package dockerruntime

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

const githubDeviceURL = "https://github.com/login/device"

type hostBrowserOpener interface {
	Open(context.Context, string) error
}

type osHostBrowserOpener struct{}

func (osHostBrowserOpener) Open(ctx context.Context, target string) error {
	executable, args, err := hostBrowserCommand(runtime.GOOS, target)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- executable and URL are selected from fixed trusted-host constants.
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}

func hostBrowserCommand(goos, target string) (string, []string, error) {
	if target != githubDeviceURL {
		return "", nil, fmt.Errorf("unsupported browser target")
	}
	switch goos {
	case "darwin":
		return "/usr/bin/open", []string{githubDeviceURL}, nil
	case "linux":
		return "xdg-open", []string{githubDeviceURL}, nil
	default:
		return "", nil, fmt.Errorf("host browser opening is unsupported on %s", goos)
	}
}
