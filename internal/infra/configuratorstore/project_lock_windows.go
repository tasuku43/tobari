//go:build windows

package configuratorstore

import "os"

func tryLockProjectFile(_ *os.File) (bool, error) { return true, nil }
func unlockProjectFile(_ *os.File)                {}
