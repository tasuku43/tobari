package companionruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

var ErrAlreadyRunning = errors.New("credential companion is already running")

const companionLockPoll = 50 * time.Millisecond

type lifetimeLock interface {
	Close() error
}

func companionDirectory(stateDirectory string) string {
	return filepath.Join(stateDirectory, "auth", "companion")
}

func lifetimeLockPath(stateDirectory string) string {
	return filepath.Join(companionDirectory(stateDirectory), "lifetime.lock")
}

func prepareCompanionDirectory(stateDirectory string) error {
	if stateDirectory == "" || !filepath.IsAbs(stateDirectory) || filepath.Clean(stateDirectory) != stateDirectory {
		return ErrInvalidBootstrap
	}
	for _, path := range []string{stateDirectory, filepath.Join(stateDirectory, "auth")} {
		if err := requireOwnerDirectory(path); err != nil {
			return err
		}
	}
	directory := companionDirectory(stateDirectory)
	if err := os.Mkdir(directory, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%w: prepare lifetime directory", ErrUnavailable)
	}
	return requireOwnerDirectory(directory)
}

// WaitForStopped waits until no companion holds the owner-only lifetime lock.
// The lock file itself is non-secret and may remain after a clean shutdown.
func WaitForStopped(ctx context.Context, stateDirectory string) error {
	if ctx == nil {
		return ErrUnavailable
	}
	if err := prepareCompanionDirectory(stateDirectory); err != nil {
		return err
	}
	for {
		lock, err := tryAcquireLifetimeLock(lifetimeLockPath(stateDirectory))
		if err == nil {
			return lock.Close()
		}
		if !errors.Is(err, ErrAlreadyRunning) {
			return err
		}
		timer := time.NewTimer(companionLockPoll)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// ObserveStopped is the non-creating status seam. Missing owner directories or
// a missing lock are exact absence; unsafe state and lock errors are unknown.
func ObserveStopped(stateDirectory string) (bool, error) {
	if stateDirectory == "" || !filepath.IsAbs(stateDirectory) || filepath.Clean(stateDirectory) != stateDirectory {
		return false, ErrInvalidBootstrap
	}
	for _, path := range []string{stateDirectory, filepath.Join(stateDirectory, "auth"), companionDirectory(stateDirectory)} {
		if err := requireOwnerDirectory(path); err != nil {
			if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrUnavailable) {
				if _, statErr := os.Lstat(path); errors.Is(statErr, os.ErrNotExist) {
					return true, nil
				}
			}
			return false, err
		}
	}
	return observeLifetimeLock(lifetimeLockPath(stateDirectory))
}
