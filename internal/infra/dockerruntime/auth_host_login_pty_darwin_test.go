//go:build darwin

package dockerruntime

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func openHostLoginPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skipf("open PTY master: %v", err)
	}
	closeMaster := true
	defer func() {
		if closeMaster {
			_ = master.Close()
		}
	}()
	if err := darwinPTYIoctl(master.Fd(), syscall.TIOCPTYGRANT, 0); err != nil {
		t.Fatalf("grant PTY: %v", err)
	}
	if err := darwinPTYIoctl(master.Fd(), syscall.TIOCPTYUNLK, 0); err != nil {
		t.Fatalf("unlock PTY: %v", err)
	}
	var name [128]byte
	if err := darwinPTYIoctl(
		master.Fd(), syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0])), // #nosec G103 -- ioctl writes the bounded PTY name buffer.
	); err != nil {
		t.Fatalf("read PTY name: %v", err)
	}
	end := 0
	for end < len(name) && name[end] != 0 {
		end++
	}
	path := string(name[:end])
	if path == "" || !strings.HasPrefix(path, "/dev/") {
		t.Fatalf("invalid PTY path %q", path)
	}
	slave, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatalf("open PTY slave: %v", err)
	}
	closeMaster = false
	return master, slave
}

func setHostLoginPTYNoncanonical(t *testing.T, terminal *os.File, minimum, timeout uint8) {
	t.Helper()
	var state syscall.Termios
	if err := darwinPTYIoctl(
		terminal.Fd(), syscall.TIOCGETA, uintptr(unsafe.Pointer(&state)), // #nosec G103 -- ioctl reads the test PTY termios structure.
	); err != nil {
		t.Fatalf("read PTY mode: %v", err)
	}
	state.Lflag &^= syscall.ICANON
	state.Cc[syscall.VMIN] = minimum
	state.Cc[syscall.VTIME] = timeout
	if err := darwinPTYIoctl(
		terminal.Fd(), syscall.TIOCSETA, uintptr(unsafe.Pointer(&state)), // #nosec G103 -- ioctl sets only the test PTY termios structure.
	); err != nil {
		t.Fatalf("set PTY mode: %v", err)
	}
}

func hostLoginFileStatusFlags(t *testing.T, file *os.File) int {
	t.Helper()
	flags, _, errno := syscall.Syscall(
		syscall.SYS_FCNTL, file.Fd(), uintptr(syscall.F_GETFL), 0,
	)
	if errno != 0 {
		t.Fatalf("read file status flags: %v", errno)
	}
	return int(flags)
}

func darwinPTYIoctl(fd uintptr, request uintptr, argument uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, argument); errno != 0 {
		return errno
	}
	return nil
}
