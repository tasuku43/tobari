//go:build darwin

package dockerruntime

import (
	"os"
	"syscall"
	"unsafe"
)

const darwinHostLoginPathSize = 1024

func hostLoginInputPath(file *os.File) (string, error) {
	var value [darwinHostLoginPathSize]byte
	if _, _, errno := syscall.Syscall(
		syscall.SYS_FCNTL,
		file.Fd(),
		uintptr(syscall.F_GETPATH),
		uintptr(unsafe.Pointer(&value[0])), // #nosec G103 -- F_GETPATH writes into this fixed-size path buffer.
	); errno != 0 {
		return "", errno
	}
	end := 0
	for end < len(value) && value[end] != 0 {
		end++
	}
	if end == 0 || end == len(value) {
		return "", errHostLoginPrompt
	}
	return string(value[:end]), nil
}
