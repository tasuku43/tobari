//go:build unix

package dockerruntime

import (
	"os"
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
