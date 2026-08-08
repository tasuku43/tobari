//go:build darwin || linux

package rootkey

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func requireOwner(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int64(stat.Uid) != int64(os.Geteuid()) {
		return fmt.Errorf("%w: path is not owned by the current user", ErrUnsafe)
	}
	return nil
}

func requireSafeDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return fmt.Errorf("%w: directory must be owner-only and must not be a symlink", ErrUnsafe)
	}
	return requireOwner(info)
}

// requireSafeDirectoryPrefix validates every existing directory in one
// caller-owned path chain without following a symbolic link. A missing
// component means the remaining descendants cannot exist and is reported as
// an ordinary absent chain so read-only inspection preserves first-use
// behavior.
func requireSafeDirectoryPrefix(paths ...string) (bool, error) {
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("%w: inspect owner-only directory", ErrUnsafe)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0o700 {
			return false, fmt.Errorf("%w: directory must be owner-only and must not be a symlink", ErrUnsafe)
		}
		if err := requireOwner(info); err != nil {
			return false, err
		}
	}
	return true, nil
}

func ensureSafeDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return requireSafeDirectory(path)
}

func requireSafeRegular(path string, expected os.FileMode) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != expected {
		return nil, fmt.Errorf("%w: file must be a regular owner-only file", ErrUnsafe)
	}
	if err := requireOwner(info); err != nil {
		return nil, err
	}
	return info, nil
}
