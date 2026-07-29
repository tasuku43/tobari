//go:build !windows

package dockerruntime

import "os"

func currentIDs() (int, int) {
	return os.Getuid(), os.Getgid()
}
