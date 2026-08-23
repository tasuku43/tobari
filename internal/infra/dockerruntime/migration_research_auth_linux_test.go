//go:build linux

package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestLinuxResearchRootKeyMovesAndRestoresWithFilesystemAuthority(t *testing.T) {
	runtimeStore, _ := newResearchAuthMigrationRuntime(t)
	want := bytes.Repeat([]byte{0x33}, 32)
	if _, err := runtimeStore.MigrateInstallation(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	journal, exists, err := runtimeStore.readResearchAuthJournal()
	if err != nil || !exists {
		t.Fatalf("journal exists=%t err=%v", exists, err)
	}
	quarantined := filepath.Join(runtimeStore.researchAuthStateQuarantine(journal.Digest), "state-auth", "keys", "root.key")
	if got, err := os.ReadFile(quarantined); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("quarantined Linux root key differs: %x, %v", got, err)
	}
	if err := runtimeStore.rollbackResearchAuthQuarantine(); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(runtimeStore.stateDirectory, "auth", "keys", "root.key")
	if got, err := os.ReadFile(restored); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("restored Linux root key differs: %x, %v", got, err)
	}
}

func TestLinuxResearchVaultWithoutRootKeyFailsClosed(t *testing.T) {
	runtimeStore, _ := newResearchAuthMigrationRuntime(t)
	key := filepath.Join(runtimeStore.stateDirectory, "auth", "keys", "root.key")
	if err := os.Remove(key); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeStore.MigrateInstallation(context.Background(), io.Discard); !errors.Is(err, tobari.ErrMigrationSourceUnsafe) {
		t.Fatalf("missing Linux root key error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(runtimeStore.stateDirectory, "auth")); err != nil {
		t.Fatalf("missing root key preflight moved authority: %v", err)
	}
}
