//go:build !unix

package workspaceauthoritymigration

import (
	"fmt"
	"os"
)

func ownedByCurrentUser(os.FileInfo) bool { return false }

func lockExclusive(*os.File) error {
	return fmt.Errorf("Workspace authority migration owner and lock validation is unsupported on this platform")
}
func unlockExclusive(*os.File) error { return nil }
