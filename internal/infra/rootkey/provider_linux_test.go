//go:build linux

package rootkey

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxProviderCreatesAndLoadsExactOwnerOnlyKey(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	provider, err := newLinuxProvider(state, bytes.NewReader(bytes.Repeat([]byte{0x42}, Size)))
	if err != nil {
		t.Fatal(err)
	}
	created, err := provider.LoadOrCreate(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if created.Backend() != BackendLinuxFile || !bytes.Equal(created.Bytes(), bytes.Repeat([]byte{0x42}, Size)) {
		t.Fatalf("unexpected material: backend=%q bytes=%x", created.Backend(), created.Bytes())
	}
	for _, path := range []string{state, filepath.Join(state, "auth"), filepath.Join(state, "auth", "keys")} {
		info, err := os.Lstat(path)
		if err != nil || info.Mode().Perm() != 0o700 || !info.IsDir() {
			t.Fatalf("unsafe directory %s: info=%v err=%v", path, info, err)
		}
	}
	keyPath := filepath.Join(state, "auth", "keys", "root.key")
	if info, err := os.Lstat(keyPath); err != nil || info.Mode().Perm() != 0o600 || !info.Mode().IsRegular() {
		t.Fatalf("unsafe key file: info=%v err=%v", info, err)
	}
	loaded, err := provider.LoadOrCreate(context.Background(), true)
	if err != nil || !bytes.Equal(loaded.Bytes(), created.Bytes()) {
		t.Fatalf("load existing key: material=%x err=%v", loaded.Bytes(), err)
	}
	if backend, exists, err := provider.Inspect(context.Background(), true); err != nil || !exists || backend != BackendLinuxFile {
		t.Fatalf("inspect existing key: backend=%q exists=%t err=%v", backend, exists, err)
	}
}

func TestLinuxProviderMissingKeyWithVaultFailsWithoutGeneration(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	provider, err := newLinuxProvider(state, bytes.NewReader(bytes.Repeat([]byte{0x42}, Size)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.LoadOrCreate(context.Background(), true); !errors.Is(err, ErrMissingWithVault) {
		t.Fatalf("expected missing-with-vault, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(state, "auth", "keys", "root.key")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("provider generated a replacement key: %v", err)
	}
}

func TestLinuxProviderInspectPreservesMissingPath(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	provider, err := newLinuxProvider(state, bytes.NewReader(bytes.Repeat([]byte{0x42}, Size)))
	if err != nil {
		t.Fatal(err)
	}
	backend, exists, err := provider.Inspect(context.Background(), false)
	if err != nil || exists || backend != BackendLinuxFile {
		t.Fatalf("inspect missing key: backend=%q exists=%t err=%v", backend, exists, err)
	}
	if _, err := os.Lstat(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only inspection created the missing state path: %v", err)
	}
}

func TestLinuxProviderRejectsUnsafePaths(t *testing.T) {
	tests := map[string]func(*testing.T, string){
		"broad key mode": func(t *testing.T, state string) {
			path := filepath.Join(state, "auth", "keys", "root.key")
			if err := os.WriteFile(path, bytes.Repeat([]byte{1}, Size), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"symlink key": func(t *testing.T, state string) {
			target := filepath.Join(t.TempDir(), "key")
			if err := os.WriteFile(target, bytes.Repeat([]byte{1}, Size), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(state, "auth", "keys", "root.key")); err != nil {
				t.Fatal(err)
			}
		},
		"truncated key": func(t *testing.T, state string) {
			if err := os.WriteFile(filepath.Join(state, "auth", "keys", "root.key"), []byte("short"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, prepare := range tests {
		t.Run(name, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "state")
			if err := os.MkdirAll(filepath.Join(state, "auth", "keys"), 0o700); err != nil {
				t.Fatal(err)
			}
			prepare(t, state)
			provider, _ := newLinuxProvider(state, bytes.NewReader(bytes.Repeat([]byte{2}, Size)))
			if _, err := provider.LoadOrCreate(context.Background(), false); !errors.Is(err, ErrUnsafe) {
				t.Fatalf("expected unsafe error, got %v", err)
			}
		})
	}
}

func TestLinuxProviderRejectsUnsafeIntermediateDirectories(t *testing.T) {
	tests := map[string]func(*testing.T, string){
		"symlink state": func(t *testing.T, state string) {
			target := filepath.Join(t.TempDir(), "state-target")
			writeLinuxRootKey(t, target)
			if err := os.Symlink(target, state); err != nil {
				t.Fatal(err)
			}
		},
		"symlink auth": func(t *testing.T, state string) {
			makeLinuxDirectory(t, state)
			target := filepath.Join(t.TempDir(), "auth-target")
			writeLinuxRootKeyBelowAuth(t, target)
			if err := os.Symlink(target, filepath.Join(state, "auth")); err != nil {
				t.Fatal(err)
			}
		},
		"symlink keys": func(t *testing.T, state string) {
			makeLinuxDirectory(t, filepath.Join(state, "auth"))
			target := filepath.Join(t.TempDir(), "keys-target")
			makeLinuxDirectory(t, target)
			if err := os.WriteFile(filepath.Join(target, "root.key"), bytes.Repeat([]byte{0x31}, Size), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(state, "auth", "keys")); err != nil {
				t.Fatal(err)
			}
		},
		"broad state mode": func(t *testing.T, state string) {
			writeLinuxRootKey(t, state)
			if err := os.Chmod(state, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"broad auth mode": func(t *testing.T, state string) {
			writeLinuxRootKey(t, state)
			if err := os.Chmod(filepath.Join(state, "auth"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"broad keys mode": func(t *testing.T, state string) {
			writeLinuxRootKey(t, state)
			if err := os.Chmod(filepath.Join(state, "auth", "keys"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, prepare := range tests {
		t.Run(name, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "state")
			prepare(t, state)
			provider, err := newLinuxProvider(state, bytes.NewReader(bytes.Repeat([]byte{0x42}, Size)))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := provider.LoadOrCreate(context.Background(), false); !errors.Is(err, ErrUnsafe) {
				t.Fatalf("load accepted unsafe intermediate directory: %v", err)
			}
			backend, exists, err := provider.Inspect(context.Background(), false)
			if !errors.Is(err, ErrUnsafe) || exists || backend != BackendLinuxFile {
				t.Fatalf("inspect unsafe intermediate directory: backend=%q exists=%t err=%v", backend, exists, err)
			}
		})
	}
}

func makeLinuxDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func writeLinuxRootKey(t *testing.T, state string) {
	t.Helper()
	writeLinuxRootKeyBelowAuth(t, filepath.Join(state, "auth"))
}

func writeLinuxRootKeyBelowAuth(t *testing.T, auth string) {
	t.Helper()
	keys := filepath.Join(auth, "keys")
	makeLinuxDirectory(t, keys)
	if err := os.WriteFile(filepath.Join(keys, "root.key"), bytes.Repeat([]byte{0x31}, Size), 0o600); err != nil {
		t.Fatal(err)
	}
}
