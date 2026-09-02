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

func openConfiguratorStageLockFile(path string, create bool) (*os.File, error) {
	flags := syscall.O_RDWR | syscall.O_CLOEXEC | syscall.O_NOFOLLOW
	if create {
		flags |= syscall.O_CREAT | syscall.O_EXCL
	}
	fd, err := syscall.Open(path, flags, 0o600) // #nosec G304,G703 -- caller validates the fixed child and descriptor identity under a private root.
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = syscall.Close(fd)
		return nil, os.ErrInvalid
	}
	return file, nil
}
