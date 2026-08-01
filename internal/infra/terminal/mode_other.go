//go:build !linux && !darwin

package terminal

import "io"

type unsupportedMode struct{}

func New() Mode { return unsupportedMode{} }

func (unsupportedMode) Enter(io.Reader) (func() error, error) {
	return nil, ErrUnsupported
}
