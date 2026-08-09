//go:build linux

package dockerruntime

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

func openHostLoginPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile(
		"/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0,
	)
	if err != nil {
		t.Fatalf("open PTY master: %v", err)
	}
	closeMaster := true
	defer func() {
		if closeMaster {
			_ = master.Close()
		}
	}()
	var unlocked int32
	if err := linuxPTYIoctl(
		master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlocked)), // #nosec G103 -- ioctl reads the bounded unlock scalar.
	); err != nil {
		t.Fatalf("unlock PTY: %v", err)
	}
	var number uint32
	if err := linuxPTYIoctl(
		master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&number)), // #nosec G103 -- ioctl writes the bounded PTY number scalar.
	); err != nil {
		t.Fatalf("read PTY number: %v", err)
	}
	path := fmt.Sprintf("/dev/pts/%d", number)
	slave, err := os.OpenFile(
		path, os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0,
	)
	if err != nil {
		t.Fatalf("open PTY slave %q: %v", path, err)
	}
	closeMaster = false
	return master, slave
}

func setHostLoginPTYNoncanonical(t *testing.T, terminal *os.File, minimum, timeout uint8) {
	t.Helper()
	var state syscall.Termios
	if err := linuxPTYIoctl(
		terminal.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&state)), // #nosec G103 -- ioctl reads the test PTY termios structure.
	); err != nil {
		t.Fatalf("read PTY mode: %v", err)
	}
	state.Lflag &^= syscall.ICANON
	state.Cc[syscall.VMIN] = minimum
	state.Cc[syscall.VTIME] = timeout
	if err := linuxPTYIoctl(
		terminal.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&state)), // #nosec G103 -- ioctl sets only the test PTY termios structure.
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

func linuxPTYIoctl(fd uintptr, request uintptr, argument uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, argument); errno != 0 {
		return errno
	}
	return nil
}
