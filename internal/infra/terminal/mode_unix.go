//go:build linux || darwin

package terminal

import (
	"fmt"
	"io"
	"os"
	"sync"
)

type unixMode struct {
	minimum byte
	timeout byte
}

// New returns the polling raw mode used by interactive selectors. An idle
// read completes after one decisecond so those callers can observe context
// cancellation between key presses.
func New() Mode { return unixMode{minimum: 0, timeout: 1} }

// NewStream returns the blocking raw mode used by byte-stream relays. Reads
// wait for at least one byte so an idle terminal cannot look like EOF to Go.
func NewStream() Mode { return unixMode{minimum: 1, timeout: 0} }

func (mode unixMode) Enter(reader io.Reader) (func() error, error) {
	file, ok := reader.(*os.File)
	if !ok {
		return nil, ErrUnsupported
	}
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect terminal input: %w", err)
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return nil, ErrUnsupported
	}
	original, err := getTermios(file.Fd())
	if err != nil {
		return nil, fmt.Errorf("read terminal mode: %w", err)
	}
	raw := original
	configureRaw(&raw, mode.minimum, mode.timeout)
	if err := setTermios(file.Fd(), &raw); err != nil {
		return nil, fmt.Errorf("set terminal mode: %w", err)
	}

	var once sync.Once
	return func() error {
		var restoreErr error
		once.Do(func() { restoreErr = setTermios(file.Fd(), &original) })
		return restoreErr
	}, nil
}
