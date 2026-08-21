//go:build !darwin

package rootkey

import (
	"context"
	"io"
)

type securityRunner interface{}
type osSecurityRunner struct{}
type macOSProvider struct{}

func newMacOSProvider(securityRunner, io.Reader) *macOSProvider { return &macOSProvider{} }
func newMacOSProviderForService(securityRunner, io.Reader, string) *macOSProvider {
	return &macOSProvider{}
}
func (p *macOSProvider) LoadOrCreate(context.Context, bool) (Material, error) {
	return Material{}, ErrUnavailable
}
func (p *macOSProvider) Inspect(context.Context, bool) (Backend, bool, error) {
	return BackendMacOSKeychain, false, ErrUnavailable
}
