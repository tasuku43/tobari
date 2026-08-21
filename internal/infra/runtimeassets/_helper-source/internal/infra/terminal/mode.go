// Package terminal owns the narrow host terminal capability used by rich
// interactive presentation. It deliberately has no third-party dependency.
package terminal

import (
	"errors"
	"io"
	"os"
)

var ErrUnsupported = errors.New("raw terminal mode is unsupported")

// Mode temporarily switches one terminal input into byte-at-a-time mode.
// Implementations must return a restore function that is safe to call once.
type Mode interface {
	Enter(io.Reader) (func() error, error)
}

// IsCharDevice reports whether a stream is backed by a host character device.
// The CLI uses this narrow capability to distinguish a raw-terminal polling
// timeout from EOF on an ordinary redirected reader.
func IsCharDevice(value io.Reader) bool {
	file, ok := value.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
