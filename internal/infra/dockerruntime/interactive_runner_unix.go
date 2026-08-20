//go:build darwin || linux

package dockerruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/tasuku43/tobari/internal/infra/terminal"
	"github.com/tasuku43/tobari/internal/infra/terminalstyle"
)

func (osCommandRunner) RunInteractive(
	ctx context.Context,
	args, environment []string,
	in io.Reader,
	out, errOut io.Writer,
	colorize bool,
) error {
	command := exec.CommandContext(ctx, "docker", args...) // #nosec G204 -- executable and argv boundary are fixed.
	command.Env = environment
	if !colorize || !terminal.IsTerminal(in) || !terminal.IsTerminal(out) {
		command.Stdin, command.Stdout, command.Stderr = in, out, errOut
		return command.Run()
	}
	return runInteractivePTY(ctx, command, in, out, errOut)
}

func runInteractivePTY(ctx context.Context, command *exec.Cmd, in io.Reader, out, errOut io.Writer) error {
	input, ok := in.(*os.File)
	if !ok {
		command.Stdin, command.Stdout, command.Stderr = in, out, errOut
		return command.Run()
	}
	// StartWithSize fills only nil streams. Keep Docker CLI diagnostics on the
	// caller's stderr while its attached child stream uses the relay PTY.
	command.Stderr = errOut
	if command.Stderr == nil {
		command.Stderr = io.Discard
	}

	restore, err := terminal.New().Enter(input)
	if err != nil {
		return err
	}

	var size *pty.Winsize
	if rows, cols, sizeErr := pty.Getsize(input); sizeErr == nil {
		size = &pty.Winsize{Rows: boundedPTYDimension(rows), Cols: boundedPTYDimension(cols)}
	}
	master, err := pty.StartWithSize(command, size)
	if err != nil {
		_ = restore()
		return err
	}

	styled := terminalstyle.NewStructuredWriter(out, true)
	outputDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(styled, master)
		if flushErr := styled.Flush(); copyErr == nil {
			copyErr = flushErr
		}
		outputDone <- copyErr
	}()

	inputDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(master, input)
		inputDone <- copyErr
	}()

	resizeDone := make(chan struct{})
	resizeStop := make(chan struct{})
	resizeSignals := make(chan os.Signal, 1)
	signal.Notify(resizeSignals, syscall.SIGWINCH)
	go func() {
		defer close(resizeDone)
		for {
			select {
			case <-resizeSignals:
				_ = pty.InheritSize(input, master)
			case <-ctx.Done():
				return
			case <-resizeStop:
				return
			}
		}
	}()

	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()

	var runErr error
	var outputErr error
	waitResult := waitDone
	outputResult := outputDone
	for waitResult != nil || outputResult != nil {
		select {
		case runErr = <-waitResult:
			waitResult = nil
		case outputErr = <-outputResult:
			outputResult = nil
			if outputErr != nil && !isPTYCloseError(outputErr) {
				_ = command.Process.Kill()
				if waitResult != nil {
					runErr = <-waitResult
					waitResult = nil
				}
			}
		}
	}

	close(resizeStop)
	signal.Stop(resizeSignals)
	<-resizeDone
	_ = master.Close()
	if outputResult != nil {
		outputErr = <-outputResult
	}
	select {
	case <-inputDone:
	case <-time.After(250 * time.Millisecond):
	}

	restoreErr := restore()
	if outputErr != nil && !isPTYCloseError(outputErr) {
		return outputErr
	}
	if runErr != nil {
		return runErr
	}
	return restoreErr
}

func isPTYCloseError(err error) bool {
	return err == nil || errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EIO)
}

func boundedPTYDimension(value int) uint16 {
	if value <= 0 {
		return 0
	}
	if value > int(^uint16(0)) {
		return ^uint16(0)
	}
	return uint16(value) // #nosec G115 -- the explicit bound above protects the PTY field conversion.
}
