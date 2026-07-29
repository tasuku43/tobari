package runtimeassets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeAndVersion(t *testing.T) {
	t.Parallel()
	destination := filepath.Join(t.TempDir(), "runtime")
	if err := Materialize(destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "compose.yaml")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(destination, "gateway", "entrypoint.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("entrypoint mode = %o, want 700", info.Mode().Perm())
	}
	version, err := Version()
	if err != nil {
		t.Fatal(err)
	}
	if len(version) != 16 {
		t.Fatalf("version length = %d, want 16", len(version))
	}
}

func TestVersionsAreDigestPinned(t *testing.T) {
	t.Parallel()
	versions, err := Versions()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"MITMPROXY_IMAGE", "OPA_IMAGE", "DEBIAN_IMAGE"} {
		if value := versions[key]; len(value) < 72 || value[len(value)-71:len(value)-64] != "sha256:" {
			t.Fatalf("%s is not digest pinned: %q", key, value)
		}
	}
}
