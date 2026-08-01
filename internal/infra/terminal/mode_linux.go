//go:build linux

package terminal

import (
	"syscall"
	"unsafe"
)

type termios = syscall.Termios

func getTermios(fd uintptr) (termios, error) {
	var value termios
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&value))); errno != 0 { // #nosec G103 -- the fd is the validated caller terminal and ioctl only reads termios.
		return termios{}, errno
	}
	return value, nil
}

func setTermios(fd uintptr, value *termios) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(value))); errno != 0 { // #nosec G103 -- the fd is the validated caller terminal and ioctl restores/configures termios.
		return errno
	}
	return nil
}

func configureRaw(value *termios) {
	value.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	value.Oflag &^= syscall.OPOST
	value.Cflag |= syscall.CS8
	value.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	value.Cc[syscall.VMIN] = 0
	value.Cc[syscall.VTIME] = 1
}
