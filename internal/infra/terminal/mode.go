// Package terminal owns the narrow host terminal capability used by rich
// interactive presentation. It deliberately has no third-party dependency.
package terminal

import (
	"errors"
	"io"
)

var ErrUnsupported = errors.New("raw terminal mode is unsupported")

// Mode temporarily switches one terminal input into byte-at-a-time mode.
// Implementations must return a restore function that is safe to call once.
type Mode interface {
	Enter(io.Reader) (func() error, error)
}
