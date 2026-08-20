//go:build darwin || linux

package dockerruntime

import (
	"context"
	"io"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestEnterProjectRuntimeSelectsStructuredPresentationRunnerOnlyForTTY(t *testing.T) {
	clearNOColorForTest(t)
	runner := &interactiveRecordingRunner{recordingRunner: &recordingRunner{}}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	manifest := projectRuntimeContext(t, runtime, instance)
	_, input := openTestPTY(t)
	_, output := openTestPTY(t)
	if _, err := runtime.EnterProjectRuntime(context.Background(), instance, manifest, instance.Root, tobari.NewWorkspaceShellSession(), input, output, io.Discard); err != nil {
		t.Fatal(err)
	}
	if runner.interactiveCalls != 1 || !runner.colorize {
		t.Fatalf("interactive runner calls = %d, colorize=%t; want one colorized call", runner.interactiveCalls, runner.colorize)
	}

	noTTYRunner := &interactiveRecordingRunner{recordingRunner: &recordingRunner{}}
	noTTYRuntime, err := newRuntime(t.TempDir(), t.TempDir(), noTTYRunner)
	if err != nil {
		t.Fatal(err)
	}
	noTTYInstance := projectRuntimeInstance(t, noTTYRuntime)
	noTTYManifest := projectRuntimeContext(t, noTTYRuntime, noTTYInstance)
	if _, err := noTTYRuntime.EnterProjectRuntime(context.Background(), noTTYInstance, noTTYManifest, noTTYInstance.Root, tobari.NewWorkspaceShellSession(), nil, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if noTTYRunner.interactiveCalls != 0 {
		t.Fatalf("non-TTY interactive runner calls = %d, want 0", noTTYRunner.interactiveCalls)
	}
}

type interactiveRecordingRunner struct {
	*recordingRunner
	interactiveCalls int
	colorize         bool
}

func (r *interactiveRecordingRunner) RunInteractive(
	ctx context.Context,
	args, environment []string,
	in io.Reader,
	out, errOut io.Writer,
	colorize bool,
) error {
	r.interactiveCalls++
	r.colorize = colorize
	return r.recordingRunner.Run(ctx, args, environment, in, out, errOut)
}
