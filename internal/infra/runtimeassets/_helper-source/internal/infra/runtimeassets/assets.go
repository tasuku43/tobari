// Package runtimeassets owns the immutable container runtime embedded in Tobari.
package runtimeassets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
	names, err := assetNames()
	if err != nil {
		return "", err
	}
	prefixes := []string{"assets/" + component + "/", "assets/versions.env"}
	if component == "tobari" {
		helperNames, err := namesBelow("_helper-source")
		if err != nil {
			return "", err
		}
		names = append(names, helperNames...)
		prefixes = append(prefixes, "_helper-source/")
	}
	return versionForPrefixesInNames(names, prefixes...)
}

// StandardRuntimeImage returns the exact local image name for the embedded
// standard Runtime source. The image name is derived from the checked source
// identity, not from a resolver channel or repository state.
func StandardRuntimeImage() (string, error) {
	sourceID, err := ComponentVersion("tobari")
	if err != nil {
		return "", fmt.Errorf("derive standard Runtime source identity: %w", err)
	}
	return standardRuntimeImageForSourceID(sourceID)
}

func standardRuntimeImageForSourceID(sourceID string) (string, error) {
	if len(sourceID) != sha256.Size*2 {
		return "", fmt.Errorf("standard Runtime source identity must be a %d-character SHA-256 digest", sha256.Size*2)
	}
	for _, character := range sourceID {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return "", fmt.Errorf("standard Runtime source identity contains a non-hex character")
		}
	}
	return "tobari-runtime:base-" + sourceID, nil
}

// ExposureHelperSourceVersion identifies the exact Linux helper source closure
// embedded in the host binary and supplied to the pinned Docker builder.
func ExposureHelperSourceVersion() (string, error) {
	names, err := namesBelow("_helper-source")
	if err != nil {
		return "", err
	}
	return versionForNames(names)
}

func versionForPrefixes(prefixes ...string) (string, error) {
	names, err := assetNames()
	if err != nil {
		return "", err
	}
	return versionForPrefixesInNames(names, prefixes...)
}

func versionForPrefixesInNames(names []string, prefixes ...string) (string, error) {
	selected := make([]string, 0, len(names))
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
		selected = append(selected, name)
	}
	return versionForNames(selected)
}

func versionForNames(names []string) (string, error) {
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
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// MaterializeExposureHelperSource writes the exact checked source closure used
// only by the engine-native helper build.
func MaterializeExposureHelperSource(destination string) error {
	if !filepath.IsAbs(destination) {
		return fmt.Errorf("helper source destination must be absolute")
	}
	names, err := namesBelow("_helper-source")
	if err != nil {
		return err
	}
	for _, name := range names {
		data, err := embedded.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read embedded helper source %s: %w", name, err)
		}
		relative := strings.TrimPrefix(name, "_helper-source/")
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("create helper source directory: %w", err)
		}
		if err := replaceEmbeddedFile(target, data, 0o600); err != nil {
			return err
		}
	}
	return nil
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
		if strings.HasSuffix(relative, ".sh") || relative == "browser/tobari-open" {
			mode = 0o700
		}
		if err := replaceEmbeddedFile(target, data, mode); err != nil {
			return err
		}
	}
	return nil
}

func replaceEmbeddedFile(target string, data []byte, mode os.FileMode) error {
	if current, err := os.ReadFile(target); err == nil && string(current) == string(data) { // #nosec G304 -- target is formed only from an absolute caller state root and embed.FS-walked local asset names.
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("set embedded file mode: %w", err)
		}
		return nil
	}
	temporary := target + ".tmp"
	if err := os.WriteFile(temporary, data, mode); err != nil {
		return fmt.Errorf("stage embedded file: %w", err)
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace embedded file: %w", err)
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
	for _, required := range []string{"MITMPROXY_IMAGE", "OPA_IMAGE", "DEBIAN_IMAGE", "GO_BUILDER_IMAGE"} {
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
	return namesBelow("assets")
}

func namesBelow(root string) ([]string, error) {
	var names []string
	err := fs.WalkDir(embedded, root, func(path string, entry fs.DirEntry, walkErr error) error {
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
