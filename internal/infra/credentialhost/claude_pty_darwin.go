//go:build darwin

package credentialhost

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
	if err := claudePTYIoctl(master.Fd(), syscall.TIOCPTYGRANT, 0); err != nil {
		return nil, nil, err
	}
	if err := claudePTYIoctl(master.Fd(), syscall.TIOCPTYUNLK, 0); err != nil {
		return nil, nil, err
	}
	var name [128]byte
	if err := claudePTYIoctl(
		master.Fd(), syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0])), // #nosec G103 -- ioctl writes only the fixed-size PTY path buffer.
	); err != nil {
		return nil, nil, err
	}
	terminator := bytes.IndexByte(name[:], 0)
	if terminator <= 0 {
		return nil, nil, syscall.EINVAL
	}
	path := string(name[:terminator])
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !strings.HasPrefix(path, "/dev/") {
		return nil, nil, syscall.EINVAL
	}
	deviceRoot, err := os.OpenRoot("/dev")
	if err != nil {
		return nil, nil, err
	}
	defer deviceRoot.Close()
	slave, err := deviceRoot.OpenFile(
		strings.TrimPrefix(path, "/dev/"),
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
		slave.Fd(), syscall.TIOCGETA, uintptr(unsafe.Pointer(&state)), // #nosec G103 -- ioctl writes the bounded terminal-mode structure.
	); err != nil {
		return err
	}
	state.Lflag &^= syscall.ECHO | syscall.ECHONL
	return claudePTYIoctl(
		slave.Fd(), syscall.TIOCSETA, uintptr(unsafe.Pointer(&state)), // #nosec G103 -- ioctl reads the bounded terminal-mode structure.
	)
}

func claudeHostInputPath(file *os.File) (string, error) {
	var value [1024]byte
	if _, _, errno := syscall.Syscall(
		syscall.SYS_FCNTL,
		file.Fd(),
		uintptr(syscall.F_GETPATH),
		uintptr(unsafe.Pointer(&value[0])), // #nosec G103 -- F_GETPATH writes into this fixed-size path buffer.
	); errno != 0 {
		return "", errno
	}
	end := bytes.IndexByte(value[:], 0)
	if end <= 0 {
		return "", syscall.EINVAL
	}
	return string(value[:end]), nil
}

func claudeFDSetCapacity() int { return len(syscall.FdSet{}.Bits) * 32 }

func claudeInputReady(fd int, wait time.Duration) (bool, error) {
	readSet := syscall.FdSet{}
	readSet.Bits[fd/32] |= int32(1) << uint(fd%32)
	timeout := syscall.NsecToTimeval(wait.Nanoseconds())
	if err := syscall.Select(fd+1, &readSet, nil, nil, &timeout); err != nil {
		return false, err
	}
	return readSet.Bits[fd/32]&(int32(1)<<uint(fd%32)) != 0, nil
}

func claudePTYIoctl(fd uintptr, request uintptr, argument uintptr) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, argument); errno != 0 {
		return errno
	}
	return nil
}
