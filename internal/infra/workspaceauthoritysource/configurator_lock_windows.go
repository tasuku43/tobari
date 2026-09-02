//go:build windows

package workspaceauthoritysource

import "os"

func tryLockConfiguratorStageFile(_ *os.File) (bool, error) { return true, nil }
func unlockConfiguratorStageFile(_ *os.File)                {}

func openConfiguratorStageLockFile(path string, create bool) (*os.File, error) {
	flags := os.O_RDWR
	if create {
		flags |= os.O_CREATE | os.O_EXCL
	}
	return os.OpenFile(path, flags, 0o600) // #nosec G304,G703 -- caller validates the fixed child and descriptor identity under a private root.
}
