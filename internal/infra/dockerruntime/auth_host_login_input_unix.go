//go:build linux || darwin

package dockerruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const hostLoginInputPollInterval = 50 * time.Millisecond

func openHostLoginInput(input io.Reader) (*os.File, error) {
	source, ok := input.(*os.File)
	if !ok {
		return nil, errHostLoginPrompt
	}
	path, err := hostLoginInputPath(source)
	if err != nil {
		return nil, errHostLoginPrompt
	}
	return openHostLoginInputAt(input, path)
}

// openHostLoginInputAt creates a private nonblocking open description for the
// same terminal device as inherited stdin. O_NONBLOCK therefore cannot leak
// back to the parent shell even if this process exits before closing it.
func openHostLoginInputAt(input io.Reader, path string) (*os.File, error) {
	source, ok := input.(*os.File)
	if !ok || !filepath.IsAbs(path) || filepath.Clean(path) != path ||
		!strings.HasPrefix(path, "/dev/") {
		return nil, errHostLoginPrompt
	}
	sourceInfo, err := source.Stat()
	if err != nil || sourceInfo.Mode()&os.ModeCharDevice == 0 {
		return nil, errHostLoginPrompt
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || pathInfo.Mode()&os.ModeCharDevice == 0 {
		return nil, errHostLoginPrompt
	}
	deviceRoot, err := os.OpenRoot("/dev")
	if err != nil {
		return nil, errHostLoginPrompt
	}
	defer deviceRoot.Close()
	privateInput, err := deviceRoot.OpenFile(
		strings.TrimPrefix(path, "/dev/"),
		os.O_RDONLY|syscall.O_NOCTTY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC,
		0,
	)
	if err != nil {
		return nil, errHostLoginPrompt
	}
	privateInfo, err := privateInput.Stat()
	if err != nil || !os.SameFile(sourceInfo, privateInfo) {
		_ = privateInput.Close()
		return nil, errHostLoginPrompt
	}
	return privateInput, nil
}

func readHostLoginInput(input io.Reader, destination []byte) (int, error) {
	_, fd, err := hostLoginInputFile(input)
	if err != nil {
		return 0, err
	}
	if len(destination) == 0 {
		return 0, errHostLoginPrompt
	}
	// Production validates canonical mode before and after readiness and reads
	// through a private O_NONBLOCK open description. Request one byte as the
	// descriptor's sole reader, then return to bounded readiness polling.
	return syscall.Read(fd, destination[:1])
}

// waitHostLoginInput waits for a canonical terminal line without handing the
// read to an uninterruptible goroutine. The short select window bounds the
// cancellation race while leaving line editing, echo, and signal handling to
// the host terminal.
func waitHostLoginInput(ctx context.Context, input io.Reader) error {
	_, fd, err := hostLoginInputFile(input)
	if err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		wait := hostLoginInputPollInterval
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return context.DeadlineExceeded
			}
			if remaining < wait {
				wait = remaining
			}
		}
		readSet := syscall.FdSet{}
		hostFDSet(&readSet, fd)
		timeout := syscall.NsecToTimeval(wait.Nanoseconds())
		ready, err := hostSelect(fd, &readSet, &timeout)
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			return err
		}
		if ready {
			if err := ctx.Err(); err != nil {
				return err
			}
			return nil
		}
	}
}

func hostLoginInputFile(input io.Reader) (*os.File, int, error) {
	file, ok := input.(*os.File)
	if !ok {
		return nil, 0, errHostLoginPrompt
	}
	fd := int(file.Fd())
	if fd < 0 || fd >= hostFDSetCapacity() {
		return nil, 0, errHostLoginPrompt
	}
	return file, fd, nil
}
