//go:build linux || darwin

package credentialhost

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	claudeInputPollInterval = 50 * time.Millisecond
	maxClaudeInputBytes     = 16 << 10
)

func runClaudeTerminalCommand(ctx context.Context, command ClaudeCommand) error {
	if ctx == nil || command.Stdout == nil || command.Stderr != nil {
		return ErrClaudeTTYRequired
	}
	input, ok := command.Stdin.(*os.File)
	if !ok {
		return ErrClaudeTTYRequired
	}
	inputInfo, err := input.Stat()
	if err != nil || inputInfo.Mode()&os.ModeCharDevice == 0 {
		return ErrClaudeTTYRequired
	}
	privateInput, err := openClaudeHostInput(input)
	if err != nil {
		return ErrClaudeTTYRequired
	}
	defer privateInput.Close()
	master, slave, err := openClaudePTY()
	if err != nil {
		return ErrClaudeLoginSetup
	}
	defer master.Close()
	defer slave.Close()
	if err := configureClaudePTY(slave); err != nil {
		return ErrClaudeLoginSetup
	}

	processContext, cancel := context.WithCancel(ctx)
	defer cancel()
	process := exec.CommandContext(processContext, command.Path, command.Args...) // #nosec G204 -- ClaudeDriver validates and digest-binds the absolute executable; argv is fixed.
	process.Env = append([]string(nil), command.Env...)
	process.Dir = command.Dir
	// All three child streams use the private PTY. This keeps /dev/tty output
	// inside the parser boundary. A separate nonblocking descriptor for the
	// same trusted host terminal copies bounded interactive input into master.
	process.Stdin = slave
	process.Stdout = slave
	process.Stderr = slave
	process.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	process.WaitDelay = claudeProcessWaitDelay
	if err := process.Start(); err != nil {
		return ErrClaudeLoginFailed
	}
	if err := slave.Close(); err != nil {
		cancel()
		_ = process.Wait()
		return ErrClaudeLoginFailed
	}

	readResult := make(chan error, 1)
	go func() {
		readResult <- copyClaudePTYOutput(command.Stdout, master)
	}()
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- process.Wait()
	}()
	inputDone := make(chan struct{})
	inputResult := make(chan error, 1)
	go func() {
		inputResult <- copyClaudeHostInput(processContext, privateInput, master, inputDone)
	}()

	var runErr error
	var readErr error
	var inputErr error
	inputFinished := false
	select {
	case readErr = <-readResult:
		if readErr != nil {
			cancel()
		}
		runErr = <-waitResult
	case runErr = <-waitResult:
		select {
		case readErr = <-readResult:
		case <-time.After(claudeProcessWaitDelay):
			_ = master.Close()
			readErr = <-readResult
		}
	case <-ctx.Done():
		cancel()
		runErr = <-waitResult
		_ = master.Close()
		readErr = <-readResult
	case inputErr = <-inputResult:
		inputFinished = true
		if inputErr != nil {
			cancel()
		}
		runErr = <-waitResult
		_ = master.Close()
		readErr = <-readResult
	}
	close(inputDone)
	if !inputFinished {
		inputErr = <-inputResult
	}
	if inputErr != nil && !errors.Is(inputErr, context.Canceled) && !errors.Is(inputErr, context.DeadlineExceeded) {
		return ErrClaudeTTYRequired
	}
	if readErr != nil {
		return readErr
	}
	return runErr
}

func openClaudeHostInput(input *os.File) (*os.File, error) {
	path, err := claudeHostInputPath(input)
	if err != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path || !strings.HasPrefix(path, "/dev/") {
		return nil, ErrClaudeTTYRequired
	}
	sourceInfo, err := input.Stat()
	if err != nil || sourceInfo.Mode()&os.ModeCharDevice == 0 {
		return nil, ErrClaudeTTYRequired
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || pathInfo.Mode()&os.ModeCharDevice == 0 {
		return nil, ErrClaudeTTYRequired
	}
	deviceRoot, err := os.OpenRoot("/dev")
	if err != nil {
		return nil, ErrClaudeTTYRequired
	}
	defer deviceRoot.Close()
	privateInput, err := deviceRoot.OpenFile(
		strings.TrimPrefix(path, "/dev/"),
		os.O_RDONLY|syscall.O_NOCTTY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,
		0,
	)
	if err != nil {
		return nil, ErrClaudeTTYRequired
	}
	privateInfo, err := privateInput.Stat()
	if err != nil || !os.SameFile(sourceInfo, privateInfo) {
		_ = privateInput.Close()
		return nil, ErrClaudeTTYRequired
	}
	return privateInput, nil
}

func copyClaudeHostInput(
	ctx context.Context,
	input *os.File,
	destination *os.File,
	done <-chan struct{},
) error {
	fd := int(input.Fd())
	if fd < 0 || fd >= claudeFDSetCapacity() {
		return ErrClaudeTTYRequired
	}
	transferred := 0
	buffer := make([]byte, 256)
	for {
		select {
		case <-done:
			return nil
		default:
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		wait := claudeInputPollInterval
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return context.DeadlineExceeded
			}
			if remaining < wait {
				wait = remaining
			}
		}
		ready, err := claudeInputReady(fd, wait)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			return ErrClaudeTTYRequired
		}
		if !ready {
			continue
		}
		count, err := syscall.Read(fd, buffer)
		if err != nil {
			if errors.Is(err, syscall.EINTR) || errors.Is(err, syscall.EAGAIN) {
				continue
			}
			return ErrClaudeTTYRequired
		}
		if count <= 0 || count > maxClaudeInputBytes-transferred {
			return ErrClaudeTTYRequired
		}
		written, err := destination.Write(buffer[:count])
		if err != nil || written != count {
			return ErrClaudeTTYRequired
		}
		transferred += count
	}
}

func copyClaudePTYOutput(destination io.Writer, source *os.File) error {
	buffer := make([]byte, 4096)
	for {
		count, readErr := source.Read(buffer)
		if count > 0 {
			written, writeErr := destination.Write(buffer[:count])
			if writeErr != nil {
				return writeErr
			}
			if written != count {
				return io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, syscall.EIO) || errors.Is(readErr, os.ErrClosed) || errors.Is(readErr, io.EOF) {
				return nil
			}
			return ErrClaudeLoginFailed
		}
	}
}
