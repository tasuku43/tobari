//go:build linux || darwin

package terminal

import (
	"fmt"
	"io"
	"os"
	"sync"
)

type unixMode struct{}

func New() Mode { return unixMode{} }

func (unixMode) Enter(reader io.Reader) (func() error, error) {
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
	configureRaw(&raw)
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
