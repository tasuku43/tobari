// Package rootkey owns the host-side installation root-key boundary.
package rootkey

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"runtime"
)

const Size = 32

const (
	keychainAccount = "tobari"
	keychainService = "io.tobari.auth-root.v1"
)

type Backend string

const (
	BackendMacOSKeychain Backend = "macos_keychain"
	BackendLinuxFile     Backend = "linux_xdg_file"
)

var (
	ErrUnavailable      = errors.New("root-key provider is unavailable")
	ErrMissingWithVault = errors.New("root key is missing while encrypted state exists")
	ErrUnsafe           = errors.New("root-key storage is unsafe")
	ErrDenied           = errors.New("root-key access was denied")
)

// Material is a purpose-bound root key returned only to the infrastructure
// operation that unlocks the broker. Bytes returns a copy so callers cannot
// mutate provider-owned state.
type Material struct {
	value   [Size]byte
	backend Backend
}

func newMaterial(value []byte, backend Backend) (Material, error) {
	if len(value) != Size {
		return Material{}, fmt.Errorf("%w: root key has an invalid size", ErrUnsafe)
	}
	var material Material
	copy(material.value[:], value)
	material.backend = backend
	return material, nil
}

func (m Material) Bytes() []byte {
	return append([]byte(nil), m.value[:]...)
}

func (m Material) Backend() Backend { return m.backend }

// Provider loads or creates the installation key. encryptedStateExists must
// report only validated vault presence; a true value forbids silent creation.
type Provider interface {
	LoadOrCreate(context.Context, bool) (Material, error)
	Inspect(context.Context, bool) (Backend, bool, error)
}

// New selects the one supported host implementation without exposing a
// platform switch to application or CLI code.
func New(stateDirectory string) (Provider, error) {
	switch runtime.GOOS {
	case "darwin":
		service, err := runtimeKeychainService()
		if err != nil {
			return nil, err
		}
		return newMacOSProviderForService(osSecurityRunner{}, rand.Reader, service), nil
	case "linux":
		return newLinuxProvider(stateDirectory, rand.Reader)
	default:
		return nil, fmt.Errorf("%w: host platform %s is unsupported", ErrUnavailable, runtime.GOOS)
	}
}

func readRandom(source io.Reader) ([]byte, error) {
	if source == nil {
		return nil, fmt.Errorf("%w: entropy source is unavailable", ErrUnavailable)
	}
	value := make([]byte, Size)
	if _, err := io.ReadFull(source, value); err != nil {
		return nil, fmt.Errorf("%w: generate root key", ErrUnavailable)
	}
	return value, nil
}
