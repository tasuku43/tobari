//go:build !darwin && !linux

package companionruntime

func requireOwnerDirectory(string) error { return ErrUnavailable }

func tryAcquireLifetimeLock(string) (lifetimeLock, error) {
	return nil, ErrUnavailable
}

func observeLifetimeLock(string) (bool, error) { return false, ErrUnavailable }
