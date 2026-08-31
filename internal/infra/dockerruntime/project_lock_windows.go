//go:build windows

package dockerruntime

import "os"

// Windows builds retain the same API. The Windows-specific runtime adapter is
// intentionally unavailable until its Docker transport support is introduced.
func tryLockProjectFile(_ *os.File) (bool, error) { return true, nil }

func tryLockSharedProjectFile(_ *os.File) (bool, error) { return true, nil }

func unlockProjectFile(_ *os.File) {}
