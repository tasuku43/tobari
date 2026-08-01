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

func TestComposeSpecOwnsOnlySharedLeastPrivilegeServices(t *testing.T) {
	t.Parallel()
	data, err := Read("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	spec := string(data)
	for _, required := range []string{
		"internal: true",
		"user: \"${TOBARI_UID}:${TOBARI_GID}\"",
		"TOBARI_UID: ${TOBARI_UID}",
		"TOBARI_GID: ${TOBARI_GID}",
		"http://127.0.0.1:8181/health",
		"http://opa:8181/health",
		"cap_drop: [ALL]",
		"no-new-privileges:true",
		"--watch",
		"${TOBARI_POLICY_DIR}:/policy:ro",
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
		"tobari-realm",
		"${TOBARI_ROOT}",
		"/policy:rw",
	} {
		if strings.Contains(spec, forbidden) {
			t.Errorf("compose spec contains forbidden boundary %q", forbidden)
		}
	}
}

func TestComposeSpecCapsSharedServiceLogs(t *testing.T) {
	data, err := Read("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	spec := string(data)
	fragment := "    logging:\n" +
		"      driver: json-file\n" +
		"      options:\n" +
		"        max-size: \"10m\"\n" +
		"        max-file: \"3\"\n"
	if count := strings.Count(spec, fragment); count != 2 {
		t.Fatalf("compose spec has %d fixed shared log blocks, want 2", count)
	}
}

func TestComposeSpecCapsSharedServiceResources(t *testing.T) {
	data, err := Read("compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	spec := string(data)
	for _, service := range []struct {
		name string
		body string
	}{
		{
			name: "opa",
			body: "    cpus: \"1.0\"\n    mem_limit: 512m\n    memswap_limit: 512m\n    pids_limit: 128\n",
		},
		{
			name: "gateway",
			body: "    cpus: \"2.0\"\n    mem_limit: 1g\n    memswap_limit: 1g\n    pids_limit: 256\n",
		},
	} {
		serviceIndex := strings.Index(spec, "  "+service.name+":\n")
		if serviceIndex < 0 {
			t.Fatalf("compose spec is missing %s", service.name)
		}
		if !strings.Contains(spec[serviceIndex:], service.body) {
			t.Errorf("%s is missing fixed shared resource bounds", service.name)
		}
	}
}

func TestGatewayEntrypointCapsBufferedHTTPBodies(t *testing.T) {
	data, err := Read("gateway/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "--set body_size_limit=8m") {
		t.Fatal("Gateway entrypoint does not set a fixed mitmproxy body_size_limit")
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
