//go:build !linux

package rootkey

import (
	"context"
	"io"
)

type linuxProvider struct{}

func newLinuxProvider(string, io.Reader) (*linuxProvider, error) {
	return &linuxProvider{}, ErrUnavailable
}

func (*linuxProvider) LoadOrCreate(context.Context, bool) (Material, error) {
	return Material{}, ErrUnavailable
}
func (*linuxProvider) Inspect(context.Context, bool) (Backend, bool, error) {
	return BackendLinuxFile, false, ErrUnavailable
}
