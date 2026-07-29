//go:build windows

package authconfigfile

import "os"

// os.Root.Rename requests replace-existing behavior on Windows. The portable
// API exposes no directory Sync and makes no atomicity or durability guarantee
// for replacement, so there is no additional operation available here.
func syncDirectory(*os.Root) error {
	return nil
}
