//go:build windows

package workspaceauthoritysource

import "os"

func tryLockConfiguratorStageFile(_ *os.File) (bool, error) { return true, nil }
func unlockConfiguratorStageFile(_ *os.File)                {}
