//go:build !darwin && !linux

package rootkey

import (
	"fmt"
	"os"
)

func requireSafeDirectory(string) error {
	return fmt.Errorf("%w: unsupported platform", ErrUnavailable)
}
func ensureSafeDirectory(string) error { return fmt.Errorf("%w: unsupported platform", ErrUnavailable) }
func requireSafeRegular(string, os.FileMode) (os.FileInfo, error) {
	return nil, fmt.Errorf("%w: unsupported platform", ErrUnavailable)
}
