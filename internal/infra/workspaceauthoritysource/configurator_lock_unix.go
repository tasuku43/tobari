//go:build !windows

package workspaceauthoritysource

import (
	"errors"
	"os"
	"syscall"
)

func tryLockConfiguratorStageFile(file *os.File) (bool, error) {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

func unlockConfiguratorStageFile(file *os.File) { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
