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
	version, err := versionForPrefixes()
	if err != nil {
		return "", err
	}
	return version[:16], nil
}

// ComponentVersion returns the deterministic source identity used by the
// development image builder and resolver. versions.env is included because it
// carries the reviewed parent images used by both component Dockerfiles.
func ComponentVersion(component string) (string, error) {
	if component != "tobari" && component != "gateway" && component != "authbroker" {
		return "", fmt.Errorf("unknown runtime component %q", component)
	}
	return versionForPrefixes("assets/"+component+"/", "assets/versions.env")
}

func versionForPrefixes(prefixes ...string) (string, error) {
	names, err := assetNames()
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, name := range names {
		if len(prefixes) > 0 {
			matched := false
			for _, prefix := range prefixes {
				if name == prefix || strings.HasPrefix(name, prefix) {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		data, err := embedded.ReadFile(name)
		if err != nil {
			return "", fmt.Errorf("read embedded asset %s: %w", name, err)
		}
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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

// Versions parses reviewed third-party image authorities embedded in source.
// Tobari-owned release image outputs are injected from a generated component
// lock and deliberately do not appear in this file.
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
		if _, duplicate := values[key]; duplicate {
			return nil, fmt.Errorf("embedded versions.env contains duplicate %s", key)
		}
		values[key] = value
	}
	for _, required := range []string{"MITMPROXY_IMAGE", "OPA_IMAGE", "DEBIAN_IMAGE"} {
		if values[required] == "" {
			return nil, fmt.Errorf("embedded versions.env is missing %s", required)
		}
		if err := validateImmutableImageReference(values[required]); err != nil {
			return nil, fmt.Errorf("embedded versions.env %s: %w", required, err)
		}
	}
	return values, nil
}

func validateImmutableImageReference(image string) error {
	name, digest, found := strings.Cut(image, "@sha256:")
	if !found || name == "" || len(digest) != 64 {
		return fmt.Errorf("image reference must contain a 64-character sha256 digest")
	}
	for _, character := range digest {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return fmt.Errorf("image digest contains a non-hex character")
		}
	}
	if strings.Trim(digest, "0") == "" {
		return fmt.Errorf("image digest cannot be all zeroes")
	}
	return nil
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
