//go:build unix

package dockerruntime

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestUnixFileOwnerAdapterPreservesExactUIDAndRefusedClassification(t *testing.T) {
	path := t.TempDir()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	uid, ok := fileOwnerUID(info)
	if !ok || uid != os.Getuid() {
		t.Fatalf("owner UID = %d, available=%t, want %d", uid, ok, os.Getuid())
	}
	if !isConnectionRefused(syscall.ECONNREFUSED) {
		t.Fatal("Unix connection-refused error was not recognized")
	}
}

func TestUnixFileOwnerAdapterRejectsHardLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata")
	if err := os.WriteFile(path, []byte("metadata\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !isOwnerOnlySingleLink(info) {
		t.Fatal("owner-only single-link file was rejected")
	}

	linkedPath := filepath.Join(filepath.Dir(path), "linked-metadata")
	if err := os.Link(path, linkedPath); err != nil {
		t.Fatal(err)
	}
	linkedInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if isOwnerOnlySingleLink(linkedInfo) {
		t.Fatal("hard-linked metadata was accepted")
	}
}
