//go:build !unix

package workspaceauthoritysource

import "os"

func ownedByCurrentUser(os.FileInfo) bool { return true }
func linkCount(os.FileInfo) uint64        { return 1 }
