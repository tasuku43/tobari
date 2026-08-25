//go:build darwin || linux

package rootkey

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const unixTestContextID = "01912345-6789-7abc-8def-0123456789ab"

func TestEncryptedStateExistsPreservesMissingPathsAndFindsVault(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	if exists, err := EncryptedStateExists(state); err != nil || exists {
		t.Fatalf("missing auth state: exists=%t err=%v", exists, err)
	}
	if _, err := os.Lstat(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("encrypted-state inspection created a missing path: %v", err)
	}

	directory := filepath.Join(state, "auth", "contexts", unixTestContextID)
	makeVaultDirectory(t, directory)
	if exists, err := EncryptedStateExists(state); err != nil || exists {
		t.Fatalf("empty Context state: exists=%t err=%v", exists, err)
	}
	if err := os.WriteFile(filepath.Join(directory, "vault.enc"), []byte("synthetic ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}
	if exists, err := EncryptedStateExists(state); err != nil || !exists {
		t.Fatalf("vault state: exists=%t err=%v", exists, err)
	}
}

func TestEncryptedStateExistsRejectsUnsafeIntermediatePaths(t *testing.T) {
	tests := map[string]func(*testing.T, string){
		"symlink state": func(t *testing.T, state string) {
			target := filepath.Join(t.TempDir(), "state-target")
			writeUnixTestVault(t, target)
			if err := os.Symlink(target, state); err != nil {
				t.Fatal(err)
			}
		},
		"symlink auth": func(t *testing.T, state string) {
			makeVaultDirectory(t, state)
			target := filepath.Join(t.TempDir(), "auth-target")
			writeUnixTestVaultBelowAuth(t, target)
			if err := os.Symlink(target, filepath.Join(state, "auth")); err != nil {
				t.Fatal(err)
			}
		},
		"symlink contexts": func(t *testing.T, state string) {
			makeVaultDirectory(t, filepath.Join(state, "auth"))
			target := filepath.Join(t.TempDir(), "contexts-target")
			writeUnixTestVaultBelowContexts(t, target)
			if err := os.Symlink(target, filepath.Join(state, "auth", "contexts")); err != nil {
				t.Fatal(err)
			}
		},
		"symlink Context": func(t *testing.T, state string) {
			contexts := filepath.Join(state, "auth", "contexts")
			makeVaultDirectory(t, contexts)
			target := filepath.Join(t.TempDir(), "context-target")
			makeVaultDirectory(t, target)
			if err := os.WriteFile(filepath.Join(target, "vault.enc"), []byte("synthetic ciphertext"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(contexts, unixTestContextID)); err != nil {
				t.Fatal(err)
			}
		},
		"symlink vault": func(t *testing.T, state string) {
			directory := filepath.Join(state, "auth", "contexts", unixTestContextID)
			makeVaultDirectory(t, directory)
			target := filepath.Join(t.TempDir(), "vault-target")
			if err := os.WriteFile(target, []byte("synthetic ciphertext"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(directory, "vault.enc")); err != nil {
				t.Fatal(err)
			}
		},
		"broad state mode": func(t *testing.T, state string) {
			writeUnixTestVault(t, state)
			if err := os.Chmod(state, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"broad auth mode": func(t *testing.T, state string) {
			writeUnixTestVault(t, state)
			if err := os.Chmod(filepath.Join(state, "auth"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"broad contexts mode": func(t *testing.T, state string) {
			writeUnixTestVault(t, state)
			if err := os.Chmod(filepath.Join(state, "auth", "contexts"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"broad Context mode": func(t *testing.T, state string) {
			writeUnixTestVault(t, state)
			if err := os.Chmod(filepath.Join(state, "auth", "contexts", unixTestContextID), 0o755); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, prepare := range tests {
		t.Run(name, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "state")
			prepare(t, state)
			if _, err := EncryptedStateExists(state); !errors.Is(err, ErrUnsafe) {
				t.Fatalf("expected unsafe path error, got %v", err)
			}
		})
	}
}

func TestPrepareBrokerDirectoriesCreatesMissingChildrenAndRejectsSymlinkParent(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	makeVaultDirectory(t, state)
	if err := PrepareBrokerDirectories(state); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"auth", "auth/contexts", "auth/runtime", "auth/projection"} {
		info, err := os.Lstat(filepath.Join(state, filepath.FromSlash(relative)))
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("prepared directory %s: info=%v err=%v", relative, info, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(state, "auth", "projects")); !os.IsNotExist(err) {
		t.Fatalf("lazy Workspace auth registry was created: %v", err)
	}

	unsafeState := filepath.Join(t.TempDir(), "state")
	makeVaultDirectory(t, unsafeState)
	target := filepath.Join(t.TempDir(), "auth-target")
	makeVaultDirectory(t, target)
	if err := os.Symlink(target, filepath.Join(unsafeState, "auth")); err != nil {
		t.Fatal(err)
	}
	if err := PrepareBrokerDirectories(unsafeState); !errors.Is(err, ErrUnsafe) {
		t.Fatalf("expected unsafe symlink parent, got %v", err)
	}
}

func makeVaultDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func writeUnixTestVault(t *testing.T, state string) {
	t.Helper()
	writeUnixTestVaultBelowAuth(t, filepath.Join(state, "auth"))
}

func writeUnixTestVaultBelowAuth(t *testing.T, auth string) {
	t.Helper()
	writeUnixTestVaultBelowContexts(t, filepath.Join(auth, "contexts"))
}

func writeUnixTestVaultBelowContexts(t *testing.T, contexts string) {
	t.Helper()
	directory := filepath.Join(contexts, unixTestContextID)
	makeVaultDirectory(t, directory)
	if err := os.WriteFile(filepath.Join(directory, "vault.enc"), []byte("synthetic ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}
}
