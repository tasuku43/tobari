//go:build linux || darwin

package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

func (osCommandRunner) RunWithTerminalOutput(
	ctx context.Context, args, environment []string, input io.Reader,
	terminalOutput, observedOutput, errorOutput io.Writer,
) error {
	return runWithTerminalOutput(
		ctx, "docker", args, environment, input,
		terminalOutput, observedOutput, errorOutput,
	)
}

func runWithTerminalOutput(
	ctx context.Context, executable string, args, environment []string, input io.Reader,
	terminalOutput, observedOutput, errorOutput io.Writer,
) error {
	display, ok := terminalOutput.(*os.File)
	if !ok || !terminal.IsTerminal(display) {
		return fmt.Errorf("attached terminal output is required")
	}
	master, slave, err := pty.Open()
	if err != nil {
		return fmt.Errorf("open attached output PTY: %w", err)
	}
	defer master.Close()
	defer slave.Close()

	// A raw slave keeps the relay byte-transparent while retaining the PTY
	// identity Docker needs for native input mode and terminal negotiation.
	restoreSlave, err := terminal.New().Enter(slave)
	if err != nil {
		return fmt.Errorf("configure attached output PTY: %w", err)
	}
	restored := false
	restore := func() error {
		if restored {
			return nil
		}
		restored = true
		return restoreSlave()
	}
	defer restore() //nolint:errcheck -- the explicit completion path reports cleanup failure.

	if err := pty.InheritSize(display, master); err != nil {
		return fmt.Errorf("inherit attached terminal size: %w", err)
	}
	command := exec.CommandContext(ctx, executable, args...) // #nosec G204 -- production executable is fixed to Docker; tests supply only their own binary.
	command.Env = environment
	command.Stdin = input
	command.Stdout = slave
	command.Stderr = errorOutput
	if err := command.Start(); err != nil {
		return fmt.Errorf("start attached terminal command: %w", err)
	}

	resizeSignals := make(chan os.Signal, 1)
	resizeDone := make(chan struct{})
	resizeStopped := make(chan struct{})
	signal.Notify(resizeSignals, syscall.SIGWINCH)
	go func() {
		defer close(resizeStopped)
		for {
			select {
			case <-resizeSignals:
				if pty.InheritSize(display, master) == nil {
					_ = command.Process.Signal(syscall.SIGWINCH)
				}
			case <-resizeDone:
				return
			}
		}
	}()
	stopResize := func() {
		signal.Stop(resizeSignals)
		close(resizeDone)
		<-resizeStopped
	}

	copyResult := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(observedOutput, master)
		if errors.Is(copyErr, syscall.EIO) {
			copyErr = nil
		}
		copyResult <- copyErr
	}()
	waitResult := make(chan error, 1)
	go func() { waitResult <- command.Wait() }()

	var waitErr, copyErr error
	select {
	case waitErr = <-waitResult:
		stopResize()
		restoreErr := restore()
		_ = slave.Close()
		copyErr = <-copyResult
		return errors.Join(waitErr, copyErr, restoreErr)
	case copyErr = <-copyResult:
		if copyErr != nil {
			stopResize()
			_ = master.Close()
			_ = command.Process.Signal(os.Interrupt)
			waitErr = <-waitResult
			restoreErr := restore()
			return errors.Join(waitErr, copyErr, restoreErr)
		}
		waitErr = <-waitResult
		stopResize()
		restoreErr := restore()
		return errors.Join(waitErr, restoreErr)
	}
}
