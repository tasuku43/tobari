//go:build linux

package rootkey

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type linuxProvider struct {
	stateDirectory string
	random         io.Reader
}

func newLinuxProvider(stateDirectory string, random io.Reader) (*linuxProvider, error) {
	if !filepath.IsAbs(stateDirectory) || filepath.Clean(stateDirectory) != stateDirectory {
		return nil, fmt.Errorf("root-key state directory must be canonical and absolute")
	}
	return &linuxProvider{stateDirectory: stateDirectory, random: random}, nil
}

func (p *linuxProvider) keyDirectory() string {
	return filepath.Join(p.stateDirectory, "auth", "keys")
}

func (p *linuxProvider) keyPath() string {
	return filepath.Join(p.keyDirectory(), "root.key")
}

func (p *linuxProvider) directoryPrefix() []string {
	return []string{
		p.stateDirectory,
		filepath.Join(p.stateDirectory, "auth"),
		p.keyDirectory(),
	}
}

func (p *linuxProvider) LoadOrCreate(ctx context.Context, encryptedStateExists bool) (Material, error) {
	if err := ctx.Err(); err != nil {
		return Material{}, err
	}
	directoriesExist, err := requireSafeDirectoryPrefix(p.directoryPrefix()...)
	if err != nil {
		return Material{}, err
	}
	path := p.keyPath()
	if directoriesExist {
		if _, err := os.Lstat(path); err == nil {
			return p.load(path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return Material{}, fmt.Errorf("%w: inspect root-key file", ErrUnsafe)
		}
	}
	if encryptedStateExists {
		return Material{}, ErrMissingWithVault
	}
	if err := os.MkdirAll(p.stateDirectory, 0o700); err != nil {
		return Material{}, fmt.Errorf("%w: prepare root-key state directory", ErrUnavailable)
	}
	if err := requireSafeDirectory(p.stateDirectory); err != nil {
		return Material{}, err
	}
	if err := ensureSafeDirectory(filepath.Join(p.stateDirectory, "auth")); err != nil {
		return Material{}, err
	}
	if err := ensureSafeDirectory(p.keyDirectory()); err != nil {
		return Material{}, err
	}
	value, err := readRandom(p.random)
	if err != nil {
		return Material{}, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- fixed XDG state child with O_EXCL.
	if errors.Is(err, os.ErrExist) {
		return p.load(path)
	}
	if err != nil {
		return Material{}, fmt.Errorf("%w: create root-key file", ErrUnavailable)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(value); err != nil {
		return Material{}, fmt.Errorf("%w: write root-key file", ErrUnavailable)
	}
	if err := file.Sync(); err != nil {
		return Material{}, fmt.Errorf("%w: sync root-key file", ErrUnavailable)
	}
	if err := file.Close(); err != nil {
		return Material{}, fmt.Errorf("%w: close root-key file", ErrUnavailable)
	}
	remove = false
	return newMaterial(value, BackendLinuxFile)
}

func (p *linuxProvider) Inspect(ctx context.Context, encryptedStateExists bool) (Backend, bool, error) {
	if err := ctx.Err(); err != nil {
		return BackendLinuxFile, false, err
	}
	directoriesExist, err := requireSafeDirectoryPrefix(p.directoryPrefix()...)
	if err != nil {
		return BackendLinuxFile, false, err
	}
	if !directoriesExist {
		if encryptedStateExists {
			return BackendLinuxFile, false, ErrMissingWithVault
		}
		return BackendLinuxFile, false, nil
	}
	path := p.keyPath()
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if encryptedStateExists {
			return BackendLinuxFile, false, ErrMissingWithVault
		}
		return BackendLinuxFile, false, nil
	} else if err != nil {
		return BackendLinuxFile, false, fmt.Errorf("%w: inspect root-key file", ErrUnsafe)
	}
	material, err := p.load(path)
	if err != nil {
		return BackendLinuxFile, false, err
	}
	clear(material.value[:])
	return BackendLinuxFile, true, nil
}

func (p *linuxProvider) load(path string) (Material, error) {
	if exists, err := requireSafeDirectoryPrefix(p.directoryPrefix()...); err != nil {
		return Material{}, err
	} else if !exists {
		return Material{}, fmt.Errorf("%w: root-key directory is missing", ErrUnsafe)
	}
	info, err := requireSafeRegular(path, 0o600)
	if err != nil {
		return Material{}, err
	}
	if info.Size() != Size {
		return Material{}, fmt.Errorf("%w: root-key file has an invalid size", ErrUnsafe)
	}
	value, err := os.ReadFile(path) // #nosec G304 -- fixed validated owner-only XDG state child.
	if err != nil {
		return Material{}, fmt.Errorf("%w: read root-key file", ErrUnavailable)
	}
	return newMaterial(value, BackendLinuxFile)
}
