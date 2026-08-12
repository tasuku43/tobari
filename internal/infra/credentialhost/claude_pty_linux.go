//go:build linux

package credentialhost

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

type claudePTYWindow struct {
	rows    uint16
	columns uint16
	xpixels uint16
	ypixels uint16
}

func openClaudePTY() (*os.File, *os.File, error) {
	master, err := os.OpenFile(
		"/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0,
	)
	if err != nil {
		return nil, nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = master.Close()
		}
	}()
	var unlocked int32
	if err := claudePTYIoctl(
		master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlocked)), // #nosec G103 -- ioctl reads the bounded unlock scalar.
	); err != nil {
		return nil, nil, err
	}
	var number uint32
	if err := claudePTYIoctl(
		master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&number)), // #nosec G103 -- ioctl writes the bounded PTY number scalar.
	); err != nil {
		return nil, nil, err
	}
	slave, err := os.OpenFile(
		fmt.Sprintf("/dev/pts/%d", number),
		os.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC,
		0,
	)
	if err != nil {
		return nil, nil, err
	}
	failed = false
	return master, slave, nil
}

func configureClaudePTY(slave *os.File) error {
	window := claudePTYWindow{rows: 40, columns: uint16(claudeTerminalColumns)}
	if err := claudePTYIoctl(
		slave.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&window)), // #nosec G103 -- ioctl reads the fixed bounded window structure.
	); err != nil {
		return err
	}
	var state syscall.Termios
	if err := claudePTYIoctl(
		slave.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&state)), // #nosec G103 -- ioctl writes the bounded terminal-mode structure.
	); err != nil {
		return err
	}
	state.Lflag &^= syscall.ECHO | syscall.ECHONL
	return claudePTYIoctl(
		slave.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&state)), // #nosec G103 -- ioctl reads the bounded terminal-mode structure.
	)
}

func claudeHostInputPath(file *os.File) (string, error) {
	return os.Readlink("/proc/self/fd/" + strconv.FormatUint(uint64(file.Fd()), 10))
}

func claudeFDSetCapacity() int { return len(syscall.FdSet{}.Bits) * 64 }

func claudeInputReady(fd int, wait time.Duration) (bool, error) {
	readSet := syscall.FdSet{}
	readSet.Bits[fd/64] |= int64(1) << uint(fd%64)
	timeout := syscall.NsecToTimeval(wait.Nanoseconds())
	ready, err := syscall.Select(fd+1, &readSet, nil, nil, &timeout)
	return ready > 0, err
}

func claudePTYIoctl(fd uintptr, request uintptr, argument uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, argument); errno != 0 {
		return errno
	}
	return nil
}
