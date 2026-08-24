package dockerruntime

import (
	"context"
	"io"
	"os/exec"
	"runtime"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type serviceBrowserDispatcher interface {
	Dispatch(context.Context, string) tobari.ServiceOpenOutcome
}

type osServiceBrowserDispatcher struct{}

func serviceBrowserCommand(goos, target string) (string, []string, bool) {
	if _, _, err := tobari.ParseServiceExposureURL(target); err != nil {
		return "", nil, false
	}
	switch goos {
	case "darwin":
		return "/usr/bin/open", []string{target}, true
	case "linux":
		return "/usr/bin/xdg-open", []string{target}, true
	default:
		return "", nil, false
	}
}

func (osServiceBrowserDispatcher) Dispatch(ctx context.Context, target string) tobari.ServiceOpenOutcome {
	executable, args, ok := serviceBrowserCommand(runtime.GOOS, target)
	if !ok || ctx.Err() != nil {
		return tobari.ServiceOpenNotDispatched
	}
	started := make(chan error, 1)
	command := exec.Command(executable, args...) // #nosec G204 -- executable is fixed and target passed the exact Service exposure URL parser.
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	go func() { started <- command.Start() }()
	select {
	case err := <-started:
		if err != nil {
			return tobari.ServiceOpenNotDispatched
		}
		go func() { _ = command.Wait() }()
		return tobari.ServiceOpenRequested
	case <-ctx.Done():
		// Start may have crossed the platform boundary concurrently. Do not kill,
		// retry, or claim either dispatch or non-dispatch.
		go func() {
			if <-started == nil {
				_ = command.Wait()
			}
		}()
		return tobari.ServiceOpenOutcomeUnknown
	}
}

func (r *Runtime) dispatchServiceExposureBrowser(ctx context.Context, target string) tobari.ServiceOpenOutcome {
	if r == nil || r.serviceBrowser == nil {
		return tobari.ServiceOpenNotDispatched
	}
	return r.serviceBrowser.Dispatch(ctx, target)
}
