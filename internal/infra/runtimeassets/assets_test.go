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

func TestTobariDockerfileDeclaresRuntimeContract(t *testing.T) {
	t.Parallel()
	data, err := Read("tobari/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	spec := string(data)
	for _, required := range []string{
		`io.tobari.runtime-api="1"`,
		`io.tobari.runtime-lifetime-command="sleep infinity"`,
		`USER tobari`,
		`ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"]`,
		`CMD ["sleep", "infinity"]`,
	} {
		if !strings.Contains(spec, required) {
			t.Errorf("Tobari Dockerfile is missing %q", required)
		}
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
		"image: ${TOBARI_GATEWAY_IMAGE}",
		"user: \"${TOBARI_UID}:${TOBARI_GID}\"",
		"http://127.0.0.1:8181/health",
		"http://opa:8181/health",
		"cap_drop: [ALL]",
		"no-new-privileges:true",
		"--watch",
		"${TOBARI_POLICY_DIR}:/policy:ro",
		"${TOBARI_PRINCIPAL_DIR}:/run/tobari/principal-registry:ro",
		"TOBARI_PRINCIPAL_REGISTRY: /run/tobari/principal-registry/principals.json",
	} {
		if !strings.Contains(spec, required) {
			t.Errorf("compose spec is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"    build:",
		"privileged: true",
		"network_mode: host",
		"/var/run/docker.sock",
		"cap_add:",
		"tobari-realm",
		"${TOBARI_ROOT}",
		"${TOBARI_PRINCIPAL_CONFIG}",
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

func TestGatewayDockerfileDeclaresStableContractAndHostIndependentRuntime(t *testing.T) {
	t.Parallel()
	data, err := Read("gateway/Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	spec := string(data)
	for _, required := range []string{
		`io.tobari.gateway-api="1"`,
		`io.tobari.gateway-role="enforcement"`,
		"USER 1000:1000",
		"chmod 0777",
	} {
		if !strings.Contains(spec, required) {
			t.Errorf("Gateway Dockerfile is missing %q", required)
		}
	}
	for _, forbidden := range []string{"ARG TOBARI_UID", "ARG TOBARI_GID", "chown -R \"${TOBARI_UID}", "USER ${TOBARI_UID}"} {
		if strings.Contains(spec, forbidden) {
			t.Errorf("Gateway Dockerfile still depends on host-specific build identity %q", forbidden)
		}
	}
}

func TestVersionsAreDigestPinned(t *testing.T) {
	t.Parallel()
	versions, err := Versions()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"MITMPROXY_IMAGE", "GATEWAY_IMAGE", "OPA_IMAGE", "DEBIAN_IMAGE"} {
		if value := versions[key]; len(value) < 72 || value[len(value)-71:len(value)-64] != "sha256:" {
			t.Fatalf("%s is not digest pinned: %q", key, value)
		}
	}
}
