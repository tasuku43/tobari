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
	opener, err := os.Stat(filepath.Join(destination, "browser", "tobari-open"))
	if err != nil {
		t.Fatal(err)
	}
	if opener.Mode().Perm() != 0o700 {
		t.Fatalf("browser opener mode = %o, want 700", opener.Mode().Perm())
	}
	version, err := Version()
	if err != nil {
		t.Fatal(err)
	}
	if len(version) != 16 {
		t.Fatalf("version length = %d, want 16", len(version))
	}
}

func TestComponentVersionsUseFullContentDigest(t *testing.T) {
	t.Parallel()
	for _, component := range []string{"tobari", "gateway", "authbroker"} {
		version, err := ComponentVersion(component)
		if err != nil {
			t.Fatal(err)
		}
		if len(version) != 64 {
			t.Fatalf("%s version length = %d, want 64", component, len(version))
		}
	}
}

func TestComponentSnapshotsExcludeNonImageInputs(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"authbroker/README.md",
		"authbroker/tests/test_broker.py",
		"gateway/addon/broker_credentials.py",
		"gateway/addon/reviewed_credential_profiles.py",
		"gateway/config.example.json",
		"gateway/test_tobari_gateway.py",
	} {
		if _, err := Read(name); err == nil {
			t.Errorf("non-image source %s is embedded in the runtime snapshot", name)
		}
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
		`FROM --platform=$BUILDPLATFORM ${GO_BUILDER_IMAGE} AS exposure-helper-builder`,
		`go build -tags=tobari_exposure_helper -buildvcs=false -trimpath`,
		`io.tobari.exposure-helper-api="1"`,
		`io.tobari.exposure-helper-source="${TOBARI_EXPOSURE_HELPER_SOURCE}"`,
		`COPY --from=exposure-helper-builder /out/tobari-expose /opt/tobari/libexec/tobari-expose`,
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
		"--bundle",
		"policy-bundle:/bundle:ro",
		"name: tobari-policy-bundle",
		"${TOBARI_PRINCIPAL_DIR}:/run/tobari/principal-registry:ro",
		"TOBARI_PRINCIPAL_REGISTRY: /run/tobari/principal-registry/principals.json",
		"${TOBARI_INTERACTIVE_ATTACHMENT_DIR}:/run/tobari/interactive-attachments:ro",
		"${TOBARI_PERMISSION_INGESTION_DIR}:/run/tobari/permission-ingestion:ro",
		"TOBARI_INTERACTIVE_ATTACHMENT_REGISTRY: /run/tobari/interactive-attachments/sessions.json",
		"TOBARI_PERMISSION_INGESTION_DIRECTORY: /run/tobari/permission-ingestion",
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
		"${TOBARI_AUTH_PROVIDER_CONFIG}:/run/tobari/auth/providers.json:ro",
		"${TOBARI_PRINCIPAL_CONFIG}",
		"/policy:rw",
		"${TOBARI_POLICY_DIR}",
		"auth-broker:",
		"TOBARI_AUTH_PROVIDER_PROJECTION",
		"TOBARI_AUTH_BROKER_SOCKET",
	} {
		if strings.Contains(spec, forbidden) {
			t.Errorf("compose spec contains forbidden boundary %q", forbidden)
		}
	}
	experimentalData, err := Read("compose.experimental.yaml")
	if err != nil {
		t.Fatal(err)
	}
	experimental := string(experimentalData)
	for _, required := range []string{"auth-broker:", "image: ${TOBARI_AUTH_BROKER_IMAGE}", "TOBARI_AUTH_PROVIDER_PROJECTION: /run/tobari/auth/providers.json", "TOBARI_AUTH_BROKER_SOCKET: /run/tobari-auth/runtime/broker.sock", "      egress: {}"} {
		if !strings.Contains(experimental, required) {
			t.Errorf("experimental compose spec is missing %q", required)
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
		t.Fatalf("standard compose spec has %d fixed shared log blocks, want 2", count)
	}
	experimental, err := Read("compose.experimental.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(experimental), fragment); count != 1 {
		t.Fatalf("experimental compose override has %d fixed shared log blocks, want 1", count)
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
	experimental, err := Read("compose.experimental.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(experimental), "  auth-broker:\n") || !strings.Contains(string(experimental), "    cpus: \"1.0\"\n    mem_limit: 512m\n    memswap_limit: 512m\n    pids_limit: 128\n") {
		t.Fatal("experimental Auth Broker is missing fixed shared resource bounds")
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

func TestGatewayAssetsExposeOnlyTransparentIngress(t *testing.T) {
	t.Parallel()
	entrypoint, err := Read("gateway/entrypoint.sh")
	if err != nil {
		t.Fatal(err)
	}
	entrypointText := string(entrypoint)
	if !strings.Contains(entrypointText, "--mode transparent@15001") ||
		strings.Contains(entrypointText, "--mode regular") || strings.Contains(entrypointText, "regular@8080") {
		t.Fatalf("Gateway entrypoint retained a non-transparent ingress: %s", entrypointText)
	}
	guard, err := Read("gateway/network-guard.sh")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(guard), "dport 8080") || strings.Contains(string(guard), "8080,") {
		t.Fatalf("network guard retained explicit proxy exception: %s", guard)
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

func TestRuntimeVersionsKeepOnlyThirdPartyImagesPinned(t *testing.T) {
	t.Parallel()
	versions, err := Versions()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"MITMPROXY_IMAGE", "OPA_IMAGE", "DEBIAN_IMAGE"} {
		if value := versions[key]; validateImmutableImageReference(value) != nil {
			t.Fatalf("%s is not digest pinned: %q", key, value)
		}
	}
	for _, removed := range []string{"GATEWAY_IMAGE", "GATEWAY_IMAGE_API", "AUTH_BROKER_IMAGE", "AUTH_BROKER_IMAGE_API"} {
		if _, ok := versions[removed]; ok {
			t.Fatalf("generated release authority %s remains in source", removed)
		}
	}
}

func TestImmutableImageReferenceValidation(t *testing.T) {
	t.Parallel()
	valid := "registry.example.com/component@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	if err := validateImmutableImageReference(valid); err != nil {
		t.Fatalf("valid reference rejected: %v", err)
	}
	for _, invalid := range []string{
		"unpublished",
		"registry.example.com/component:latest",
		"registry.example.com/component@sha256:0000000000000000000000000000000000000000000000000000000000000000",
		"registry.example.com/component@sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdeg",
	} {
		if err := validateImmutableImageReference(invalid); err == nil {
			t.Fatalf("invalid reference accepted: %q", invalid)
		}
	}
}
