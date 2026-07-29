package runtimeassets

import (
	"os"
	"path/filepath"
	"strings"
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

func TestComposeSpecKeepsRealmOutsideControlAndEgress(t *testing.T) {
	t.Parallel()
	data, err := Read("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	spec := string(data)
	for _, required := range []string{
		"name: tobari-realm-net",
		"internal: true",
		"HTTP_PROXY: http://tobari-gateway:8080",
		"HTTPS_PROXY: http://tobari-gateway:8080",
		"user: \"${TOBARI_UID}:${TOBARI_GID}\"",
		"TOBARI_UID: ${TOBARI_UID}",
		"TOBARI_GID: ${TOBARI_GID}",
		"http://127.0.0.1:8181/health",
		"http://tobari-opa:8181/health",
		"cap_drop: [ALL]",
		"no-new-privileges:true",
		"${TOBARI_ROOT}:/workspace:rw",
	} {
		if !strings.Contains(spec, required) {
			t.Errorf("compose spec is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"privileged: true",
		"network_mode: host",
		"/var/run/docker.sock",
		"cap_add:",
	} {
		if strings.Contains(spec, forbidden) {
			t.Errorf("compose spec contains forbidden boundary %q", forbidden)
		}
	}
	realmSection, _, found := strings.Cut(spec, "\n  realm:\n")
	if !found || realmSection == "" {
		t.Fatal("realm service is absent")
	}
	realmService, _, found := strings.Cut(strings.TrimPrefix(spec, realmSection+"\n  realm:\n"), "\nnetworks:")
	if !found {
		t.Fatal("realm service boundary is absent")
	}
	if strings.Contains(realmService, "control:") || strings.Contains(realmService, "egress:") {
		t.Fatal("Realm service joins a trusted network")
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
