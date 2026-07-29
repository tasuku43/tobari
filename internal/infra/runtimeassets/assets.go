// Package runtimeassets owns the immutable container runtime embedded in Tobari.
package runtimeassets

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed all:assets
var embedded embed.FS

// Version returns a deterministic digest prefix over every embedded runtime file.
func Version() (string, error) {
	names, err := assetNames()
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, name := range names {
		data, err := embedded.ReadFile(name)
		if err != nil {
			return "", fmt.Errorf("read embedded asset %s: %w", name, err)
		}
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))[:16], nil
}

// Materialize writes the exact embedded runtime below destination. An existing
// file with different bytes is replaced; unrelated files are left untouched.
func Materialize(destination string) error {
	if !filepath.IsAbs(destination) {
		return fmt.Errorf("runtime destination must be absolute")
	}
	names, err := assetNames()
	if err != nil {
		return err
	}
	for _, name := range names {
		data, err := embedded.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read embedded asset %s: %w", name, err)
		}
		relative := strings.TrimPrefix(name, "assets/")
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("create runtime directory: %w", err)
		}
		mode := os.FileMode(0o600)
		if strings.HasSuffix(relative, ".sh") {
			mode = 0o700
		}
		if current, err := os.ReadFile(target); err == nil && string(current) == string(data) { // #nosec G304 -- target is formed only from an absolute caller state root and embed.FS-walked local asset names.
			if err := os.Chmod(target, mode); err != nil {
				return fmt.Errorf("set runtime asset mode: %w", err)
			}
			continue
		}
		temporary := target + ".tmp"
		if err := os.WriteFile(temporary, data, mode); err != nil {
			return fmt.Errorf("stage runtime asset: %w", err)
		}
		if err := os.Rename(temporary, target); err != nil {
			_ = os.Remove(temporary)
			return fmt.Errorf("replace runtime asset: %w", err)
		}
	}
	return nil
}

// Read returns one embedded asset for initialization of user-editable defaults.
func Read(name string) ([]byte, error) {
	if name == "" || filepath.IsAbs(name) || !filepath.IsLocal(name) {
		return nil, fmt.Errorf("asset name is invalid")
	}
	data, err := embedded.ReadFile("assets/" + filepath.ToSlash(name))
	if err != nil {
		return nil, fmt.Errorf("read embedded asset %s: %w", name, err)
	}
	return append([]byte(nil), data...), nil
}

// Versions parses the immutable image references used by the compose runtime.
func Versions() (map[string]string, error) {
	data, err := Read("versions.env")
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || key == "" || value == "" {
			return nil, fmt.Errorf("embedded versions.env contains an invalid line")
		}
		values[key] = value
	}
	for _, required := range []string{"MITMPROXY_IMAGE", "OPA_IMAGE", "DEBIAN_IMAGE"} {
		if values[required] == "" {
			return nil, fmt.Errorf("embedded versions.env is missing %s", required)
		}
	}
	return values, nil
}

func assetNames() ([]string, error) {
	var names []string
	err := fs.WalkDir(embedded, "assets", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			names = append(names, path)
		}
		return nil
	})
	sort.Strings(names)
	return names, err
}
