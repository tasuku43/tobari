//go:build !unix

package dockerruntime

import "os"

// Runtime execution on non-Unix hosts is unsupported. Keep release archive
// cross-compilation available while every Unix-owner-authorizing path fails
// closed if reached.
func fileOwnerUID(os.FileInfo) (int, bool) {
	return 0, false
}

func isConnectionRefused(error) bool {
	return false
}
