//go:build darwin || linux

package dockerruntime

import (
	"errors"
	"syscall"
)

func serviceOwnerProcessIsAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
