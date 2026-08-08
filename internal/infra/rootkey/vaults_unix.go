//go:build darwin || linux

package rootkey

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var contextIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// PrepareBrokerDirectories creates only the fixed owner-only directories that
// are mounted into the Auth Broker. Existing unsafe entries are rejected;
// their permissions are never silently broadened or repaired.
func PrepareBrokerDirectories(stateDirectory string) error {
	if !filepath.IsAbs(stateDirectory) || filepath.Clean(stateDirectory) != stateDirectory {
		return fmt.Errorf("%w: auth state path is not canonical and absolute", ErrUnsafe)
	}
	if err := requireSafeDirectory(stateDirectory); err != nil {
		return err
	}
	for _, directory := range []string{
		filepath.Join(stateDirectory, "auth"),
		filepath.Join(stateDirectory, "auth", "contexts"),
		filepath.Join(stateDirectory, "auth", "runtime"),
		filepath.Join(stateDirectory, "auth", "projection"),
		filepath.Join(stateDirectory, "auth", "projects"),
	} {
		if err := ensureSafeDirectory(directory); err != nil {
			return fmt.Errorf("%w: prepare Auth Broker directory", ErrUnsafe)
		}
	}
	return nil
}

// EncryptedStateExists inspects only the fixed auth/contexts subtree. Unsafe
// paths fail closed so a missing key can never be replaced after an attacker
// hides or redirects an existing vault.
func EncryptedStateExists(stateDirectory string) (bool, error) {
	if !filepath.IsAbs(stateDirectory) || filepath.Clean(stateDirectory) != stateDirectory {
		return false, fmt.Errorf("%w: auth state path is not canonical and absolute", ErrUnsafe)
	}
	auth := filepath.Join(stateDirectory, "auth")
	contexts := filepath.Join(stateDirectory, "auth", "contexts")
	exists, err := requireSafeDirectoryPrefix(stateDirectory, auth, contexts)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	entries, err := os.ReadDir(contexts)
	if err != nil {
		return false, fmt.Errorf("%w: read auth Context directory", ErrUnsafe)
	}
	found := false
	for _, entry := range entries {
		if !contextIDPattern.MatchString(entry.Name()) {
			return false, fmt.Errorf("%w: auth Context directory contains an unknown entry", ErrUnsafe)
		}
		directory := filepath.Join(contexts, entry.Name())
		if err := requireSafeDirectory(directory); err != nil {
			return false, err
		}
		children, err := os.ReadDir(directory)
		if err != nil {
			return false, fmt.Errorf("%w: read Context vault directory", ErrUnsafe)
		}
		for _, child := range children {
			if child.Name() != "vault.enc" {
				return false, fmt.Errorf("%w: Context vault directory contains an unknown entry", ErrUnsafe)
			}
			if _, err := requireSafeRegular(filepath.Join(directory, child.Name()), 0o600); err != nil {
				return false, err
			}
			found = true
		}
	}
	return found, nil
}
