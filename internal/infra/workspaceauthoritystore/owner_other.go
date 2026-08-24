//go:build !unix

package workspaceauthoritystore

import "os"

func ownedByCurrentUser(os.FileInfo) bool {
	// Platforms without a portable UID still receive exact mode, type,
	// symlink, inode, size, JSON, and domain validation.
	return true
}
