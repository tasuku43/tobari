//go:build unix

package authproviders

import (
	"fmt"
	"os"
	"syscall"
)

const currentUserOwnershipSupported = true

func validateCurrentUserOwner(info os.FileInfo) error {
	metadata, ok := info.Sys().(*syscall.Stat_t)
	if !ok || metadata == nil {
		return fmt.Errorf("provider ownership metadata is unavailable")
	}
	if int64(metadata.Uid) != int64(os.Geteuid()) {
		return fmt.Errorf("provider path must be owned by the current user")
	}
	return nil
}
