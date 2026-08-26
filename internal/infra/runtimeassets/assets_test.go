package runtimeassets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
	if err := filepath.WalkDir(destination, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(entry.Name(), ".rego") {
			t.Errorf("materialized evaluator source on host filesystem: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	version, err := Version()
	if err != nil {
		t.Fatal(err)
	}
	if len(version) != 16 {
		t.Fatalf("version length = %d, want 16", len(version))
	}
}

func TestMaterializeRejectsStaleEvaluatorSourceBeforeWriting(t *testing.T) {
	for _, relative := range []string{"stale.rego", filepath.Join("policy", "nested.rego")} {
		t.Run(relative, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "runtime")
			if err := os.MkdirAll(filepath.Dir(filepath.Join(destination, relative)), 0o700); err != nil {
				t.Fatal(err)
			}
			stalePath := filepath.Join(destination, relative)
			stale := []byte("package user.supplied\n")
			if err := os.WriteFile(stalePath, stale, 0o600); err != nil {
				t.Fatal(err)
			}
			markerPath := filepath.Join(destination, "owner-marker")
			marker := []byte("unchanged")
			if err := os.WriteFile(markerPath, marker, 0o600); err != nil {
				t.Fatal(err)
			}

			err := Materialize(destination)
			if err == nil || !strings.Contains(err.Error(), "executable evaluator source") || !strings.Contains(err.Error(), "reset or recreate") {
				t.Fatalf("stale evaluator source result = %v", err)
			}
			if _, err := os.Lstat(filepath.Join(destination, "compose.yaml")); !os.IsNotExist(err) {
				t.Fatalf("materialization wrote compose before rejecting stale evaluator: %v", err)
			}
			for path, want := range map[string][]byte{stalePath: stale, markerPath: marker} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != string(want) {
					t.Fatalf("pre-existing %s changed: got=%q err=%v", path, got, err)
				}
			}
		})
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

func TestStandardRuntimeImageUsesComponentSourceIdentity(t *testing.T) {
	t.Parallel()
	sourceID, err := ComponentVersion("tobari")
	if err != nil {
		t.Fatal(err)
	}
	image, err := StandardRuntimeImage()
	if err != nil {
		t.Fatal(err)
	}
	if want := "tobari-runtime:base-" + sourceID; image != want {
		t.Fatalf("standard Runtime image = %q, want %q", image, want)
	}
}

func TestStandardRuntimeImageRejectsInvalidSourceIdentity(t *testing.T) {
	t.Parallel()
	for _, sourceID := range []string{"", strings.Repeat("0", 63), strings.Repeat("0", 64)[:63] + "g"} {
		if _, err := standardRuntimeImageForSourceID(sourceID); err == nil {
			t.Errorf("standard Runtime image accepted invalid source identity %q", sourceID)
		}
	}
}

func TestStandardRuntimeImageDoesNotAliasDistinctSourceIdentities(t *testing.T) {
	t.Parallel()
	first, err := standardRuntimeImageForSourceID(strings.Repeat("0", 64))
	if err != nil {
		t.Fatal(err)
	}
	second, err := standardRuntimeImageForSourceID(strings.Repeat("0", 63) + "1")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("distinct source identities aliased to %q", first)
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
		`FROM --platform=$BUILDPLATFORM ${GO_BUILDER_IMAGE} AS workspace-helper-builder`,
		`go build -tags=tobari_exposure_helper -buildvcs=false -trimpath`,
		`io.tobari.exposure-helper-api="1"`,
		`io.tobari.exposure-helper-source="${TOBARI_EXPOSURE_HELPER_SOURCE}"`,
		`io.tobari.permission-helper-api="1"`,
		`io.tobari.permission-helper-source="${TOBARI_EXPOSURE_HELPER_SOURCE}"`,
		`COPY --from=workspace-helper-builder /out/tobari-expose /opt/tobari/libexec/tobari-expose`,
		`COPY --from=workspace-helper-builder /out/tobari-permission /opt/tobari/libexec/tobari-permission`,
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
		"TOBARI_INTERACTIVE_ATTACHMENT_REGISTRY: /run/tobari/interactive-attachments/sessions.json",
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
		"TOBARI_PERMISSION_INGESTION_TRANSPORT",
		"TOBARI_PERMISSION_INGESTION_DIRECTORY",
		"${TOBARI_PERMISSION_INGESTION_DIR}",
	} {
		if strings.Contains(spec, forbidden) {
			t.Errorf("compose spec contains forbidden boundary %q", forbidden)
		}
	}
	unixData, err := Read("compose.permission-unix.yaml")
	if err != nil {
		t.Fatal(err)
	}
	unix := string(unixData)
	for _, required := range []string{
		"TOBARI_PERMISSION_INGESTION_TRANSPORT: unix",
		"TOBARI_PERMISSION_INGESTION_DIRECTORY: /run/tobari/permission-ingestion",
		"${TOBARI_PERMISSION_INGESTION_DIR}:/run/tobari/permission-ingestion:ro",
	} {
		if !strings.Contains(unix, required) {
			t.Errorf("Unix permission profile is missing %q", required)
		}
	}
	loopbackData, err := Read("compose.permission-loopback_tcp.yaml")
	if err != nil {
		t.Fatal(err)
	}
	loopback := string(loopbackData)
	if !strings.Contains(loopback, "TOBARI_PERMISSION_INGESTION_TRANSPORT: loopback_tcp") || strings.Contains(loopback, "volumes:") || strings.Contains(loopback, "INGESTION_DIRECTORY") {
		t.Fatalf("loopback permission profile widens its mount boundary: %s", loopback)
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

func TestPermissionProfilesKeepPrivateStateGatewayOnly(t *testing.T) {
	t.Parallel()
	type service struct {
		Environment map[string]string `yaml:"environment"`
		Volumes     []string          `yaml:"volumes"`
	}
	type compose struct {
		Services map[string]service `yaml:"services"`
	}
	decode := func(name string) compose {
		data, err := Read(name)
		if err != nil {
			t.Fatal(err)
		}
		var document compose
		if err := yaml.Unmarshal(data, &document); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		return document
	}
	merge := func(documents ...compose) map[string]service {
		merged := map[string]service{}
		for _, document := range documents {
			for name, overlay := range document.Services {
				value := merged[name]
				if value.Environment == nil {
					value.Environment = map[string]string{}
				}
				for key, environmentValue := range overlay.Environment {
					value.Environment[key] = environmentValue
				}
				value.Volumes = append(value.Volumes, overlay.Volumes...)
				merged[name] = value
			}
		}
		return merged
	}
	base := decode("compose.yaml")
	experimental := decode("compose.experimental.yaml")
	for _, shape := range []struct {
		name      string
		documents []compose
	}{
		{name: "standard", documents: []compose{base}},
		{name: "research", documents: []compose{base, experimental}},
	} {
		for _, profileName := range []string{"compose.permission-unix.yaml", "compose.permission-loopback_tcp.yaml"} {
			profile := decode(profileName)
			merged := merge(append(append([]compose{}, shape.documents...), profile)...)
			label := shape.name + "/" + profileName
			registryEnvironmentCount, registryMountCount := 0, 0
			ingestionDirectoryCount, ingestionMountCount := 0, 0
			for name, value := range merged {
				joinedVolumes := strings.Join(value.Volumes, "\n")
				_, hasRegistryEnvironment := value.Environment["TOBARI_INTERACTIVE_ATTACHMENT_REGISTRY"]
				registryEnvironmentCount += boolInt(hasRegistryEnvironment)
				registryMounts := strings.Count(joinedVolumes, "/run/tobari/interactive-attachments:ro")
				registryMountCount += registryMounts
				hasRegistryMount := registryMounts > 0
				_, hasTransport := value.Environment["TOBARI_PERMISSION_INGESTION_TRANSPORT"]
				_, hasDirectory := value.Environment["TOBARI_PERMISSION_INGESTION_DIRECTORY"]
				ingestionDirectoryCount += boolInt(hasDirectory)
				ingestionMounts := strings.Count(joinedVolumes, "/run/tobari/permission-ingestion:ro")
				ingestionMountCount += ingestionMounts
				hasIngestionMount := ingestionMounts > 0
				if name == "gateway" {
					if !hasRegistryEnvironment || !hasRegistryMount || !hasTransport {
						t.Fatalf("%s Gateway private boundary is incomplete: %+v", label, value)
					}
					if profileName == "compose.permission-unix.yaml" && (!hasDirectory || !hasIngestionMount) {
						t.Fatalf("%s Gateway Unix boundary is incomplete: %+v", label, value)
					}
					if profileName == "compose.permission-loopback_tcp.yaml" && (hasDirectory || hasIngestionMount) {
						t.Fatalf("%s Gateway gained a Unix boundary: %+v", label, value)
					}
					continue
				}
				if hasRegistryEnvironment || hasRegistryMount || hasTransport || hasDirectory || hasIngestionMount {
					t.Fatalf("%s leaked permission authority to service %s: %+v", label, name, value)
				}
			}
			if registryEnvironmentCount != 1 || registryMountCount != 1 {
				t.Fatalf("%s registry projection counts = env:%d mount:%d", label, registryEnvironmentCount, registryMountCount)
			}
			wantUnixCount := 0
			if profileName == "compose.permission-unix.yaml" {
				wantUnixCount = 1
			}
			if ingestionDirectoryCount != wantUnixCount || ingestionMountCount != wantUnixCount {
				t.Fatalf("%s ingestion projection counts = env:%d mount:%d", label, ingestionDirectoryCount, ingestionMountCount)
			}
		}
	}
	for _, asset := range []string{"gateway/network-guard.sh", "tobari/Dockerfile"} {
		data, err := Read(asset)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"interactive-attachments", "permission-ingestion", "TOBARI_PERMISSION_INGESTION", "pwt_", "pws_"} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("%s received permission authority marker %q", asset, forbidden)
			}
		}
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
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
