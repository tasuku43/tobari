//go:build windows

package dockerruntime

import (
	"errors"
	"syscall"
)

const windowsErrorInvalidParameter syscall.Errno = 87

func serviceOwnerProcessIsAlive(pid int) bool {
	handle, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// ERROR_INVALID_PARAMETER is Windows' missing-process result for
		// OpenProcess. Access-denied or another observation failure cannot
		// prove absence, so retain the record and let rendezvous validation
		// classify it.
		return !errors.Is(err, windowsErrorInvalidParameter)
	}
	defer syscall.CloseHandle(handle)
	event, err := syscall.WaitForSingleObject(handle, 0)
	if err != nil {
		return true
	}
	return event == syscall.WAIT_TIMEOUT
}
