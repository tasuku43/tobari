//go:build darwin || linux

package companionruntime

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

type unixLifetimeLock struct{ file *os.File }

func effectiveUID() (uint32, bool) {
	uid := os.Geteuid()
	if uid < 0 || uid>>32 != 0 {
		return 0, false
	}
	// #nosec G115 -- uid is non-negative and values wider than uint32 are rejected above.
	return uint32(uid), true
}

func requireOwnerDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%w: inspect owner directory", ErrUnavailable)
	}
	metadata, ok := info.Sys().(*syscall.Stat_t)
	ownerUID, uidOK := effectiveUID()
	if !ok || !uidOK || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
		metadata.Uid != ownerUID || info.Mode().Perm() != 0o700 {
		return fmt.Errorf("%w: owner directory is unsafe", ErrUnavailable)
	}
	return nil
}

func tryAcquireLifetimeLock(path string) (lifetimeLock, error) {
	descriptor, err := syscall.Open(
		path,
		syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: open lifetime lock", ErrUnavailable)
	}
	file := os.NewFile(uintptr(descriptor), path)
	failed := true
	defer func() {
		if failed {
			_ = file.Close()
		}
	}()
	var metadata syscall.Stat_t
	ownerUID, uidOK := effectiveUID()
	if err := syscall.Fstat(descriptor, &metadata); err != nil ||
		!uidOK ||
		metadata.Mode&syscall.S_IFMT != syscall.S_IFREG ||
		metadata.Uid != ownerUID || metadata.Mode&0o777 != 0o600 || metadata.Nlink != 1 {
		return nil, fmt.Errorf("%w: lifetime lock is unsafe", ErrUnavailable)
	}
	if err := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("%w: acquire lifetime lock", ErrUnavailable)
	}
	failed = false
	return &unixLifetimeLock{file: file}, nil
}

func observeLifetimeLock(path string) (bool, error) {
	descriptor, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: open lifetime lock", ErrUnavailable)
	}
	file := os.NewFile(uintptr(descriptor), path)
	defer file.Close()
	var metadata syscall.Stat_t
	ownerUID, uidOK := effectiveUID()
	if err := syscall.Fstat(descriptor, &metadata); err != nil || !uidOK || metadata.Mode&syscall.S_IFMT != syscall.S_IFREG || metadata.Uid != ownerUID || metadata.Mode&0o777 != 0o600 || metadata.Nlink != 1 {
		return false, fmt.Errorf("%w: lifetime lock is unsafe", ErrUnavailable)
	}
	if err := syscall.Flock(descriptor, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return false, nil
		}
		return false, fmt.Errorf("%w: observe lifetime lock", ErrUnavailable)
	}
	if err := syscall.Flock(descriptor, syscall.LOCK_UN); err != nil {
		return false, fmt.Errorf("%w: release observed lifetime lock", ErrUnavailable)
	}
	return true, nil
}

func (l *unixLifetimeLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	descriptor := int(l.file.Fd())
	unlockErr := syscall.Flock(descriptor, syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	if unlockErr != nil || closeErr != nil {
		return fmt.Errorf("%w: release lifetime lock", ErrUnavailable)
	}
	return nil
}
