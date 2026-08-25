package dockerruntime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

type exposureHelperAssetRunner struct {
	architecture    string
	archive         []byte
	metadata        []byte
	metadataByImage map[string][]byte
	outputs         [][]string
	runs            [][]string
	copyErr         error
}

func (r *exposureHelperAssetRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, append([]string{}, args...))
	if len(args) >= 2 && args[0] == "image" && args[1] == "inspect" {
		if r.metadataByImage != nil {
			if metadata, ok := r.metadataByImage[args[len(args)-1]]; ok {
				return append([]byte(nil), metadata...), nil
			}
		}
		if r.metadata != nil {
			return append([]byte(nil), r.metadata...), nil
		}
		source, err := runtimeassets.ExposureHelperSourceVersion()
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf(`{"architecture":%q,"os":"linux","exposure_api":"1","exposure_source":%q,"permission_api":"1","permission_source":%q}`, r.architecture, source, source)), nil
	}
	if len(args) >= 1 && args[0] == "version" {
		return []byte(fmt.Sprintf(`{"Arch":%q,"Os":"linux"}`, r.architecture)), nil
	}
	return nil, fmt.Errorf("unexpected output argv: %v", args)
}

func TestFinalWorkspaceHelpersUseCanonicalBaseForCustomRuntime(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	validSource, err := runtimeassets.ExposureHelperSourceVersion()
	if err != nil {
		t.Fatal(err)
	}
	validMetadata := []byte(fmt.Sprintf(
		`{"architecture":"arm64","os":"linux","exposure_api":"1","exposure_source":%q,"permission_api":"1","permission_source":%q}`,
		validSource, validSource,
	))
	customMetadata := []byte(`{"architecture":"arm64","os":"linux","exposure_api":"","exposure_source":"","permission_api":"","permission_source":""}`)
	defaultImage := "tobari-runtime:base"
	customImage := "tobari-runtime-custom:revision"
	runner := &exposureHelperAssetRunner{
		architecture: "arm64",
		archive:      exposureHelperArchive(t, syntheticExposureHelperELF("arm64"), "arm64", nil),
		metadataByImage: map[string][]byte{
			defaultImage: validMetadata,
			customImage:  customMetadata,
		},
	}
	runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureFinalWorkspaceHelpers(context.Background(), customImage); err != nil {
		t.Fatalf("custom Workspace image prevented canonical helper materialization: %v", err)
	}
	for _, args := range runner.outputs {
		if len(args) >= 2 && args[0] == "image" && args[1] == "inspect" && args[len(args)-1] != defaultImage {
			t.Fatalf("helper identity observation used custom Workspace image: %v", args)
		}
	}
}

func TestWorkspaceHelpersRequireBothVerifiedImageIdentities(t *testing.T) {
	t.Parallel()
	source, err := runtimeassets.ExposureHelperSourceVersion()
	if err != nil {
		t.Fatal(err)
	}
	runner := &exposureHelperAssetRunner{architecture: "amd64", metadata: []byte(fmt.Sprintf(
		`{"architecture":"amd64","os":"linux","exposure_api":"1","exposure_source":%q,"permission_api":"","permission_source":%q}`,
		source, source,
	))}
	base := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.materializeWorkspaceHelpers(context.Background(), "tobari-runtime:test"); err == nil {
		t.Fatal("runtime image without permission helper identity was accepted")
	}
	if len(runner.runs) != 0 {
		t.Fatalf("invalid image identity started extraction: %v", runner.runs)
	}
}

func (r *exposureHelperAssetRunner) Run(_ context.Context, args, _ []string, _ io.Reader, out, _ io.Writer) error {
	r.runs = append(r.runs, append([]string{}, args...))
	if len(args) >= 2 && args[0] == "container" && args[1] == "cp" {
		if r.copyErr != nil {
			return r.copyErr
		}
		_, err := out.Write(r.archive)
		return err
	}
	if len(args) >= 2 && args[0] == "container" && (args[1] == "create" || args[1] == "start" || args[1] == "rm") {
		return nil
	}
	return fmt.Errorf("unexpected run argv: %v", args)
}

func TestWorkspaceServiceHelperIsExtractedFromVerifiedEngineImageAtomically(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	binary := syntheticExposureHelperELF("arm64")
	runner := &exposureHelperAssetRunner{architecture: "arm64", archive: exposureHelperArchive(t, binary, "arm64", nil)}
	runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	version, err := runtimeassets.Version()
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "state", "runtime", version, "helpers", exposureHelperImageBinary)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("stale-helper"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runtime.materializeWorkspaceHelpers(context.Background(), "tobari-runtime:test"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(data, binary) {
		t.Fatalf("helper bytes=%x err=%v", data, err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("helper info=%v err=%v", info, err)
	}
	permissionTarget := filepath.Join(filepath.Dir(target), permissionHelperImageBinary)
	permissionData, permissionErr := os.ReadFile(permissionTarget)
	if permissionErr != nil || !bytes.Equal(permissionData, binary) {
		t.Fatalf("permission helper bytes=%x err=%v", permissionData, permissionErr)
	}
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil || len(entries) != 2 || entries[0].Name() != exposureHelperImageBinary || entries[1].Name() != permissionHelperImageBinary {
		t.Fatalf("helper directory=%v err=%v", entries, err)
	}
	if !runnerSaw(runner.runs, "container", "create") || !runnerSaw(runner.runs, "container", "start") || !runnerSaw(runner.runs, "container", "cp") || !runnerSaw(runner.runs, "container", "rm") {
		t.Fatalf("docker calls=%v", runner.runs)
	}
	create := runner.runs[0]
	for _, required := range []string{
		ownerLabel + "=" + ownerValue,
		componentLabel + "=exposure-helper-extract",
		"--rm", "--network", "none", "--read-only", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--user", "65534:65534",
		"--entrypoint", "/bin/sleep", fmt.Sprintf("%d", exposureHelperAutoRemoveSeconds),
	} {
		if !slices.Contains(create, required) {
			t.Fatalf("create argv=%v missing %q", create, required)
		}
	}
}

func TestWorkspaceServiceHelperExtractionRejectsUntrustedArtifactsAndAlwaysCleansContainer(t *testing.T) {
	t.Parallel()
	validBinary := syntheticExposureHelperELF("amd64")
	tests := []struct {
		name         string
		architecture string
		archive      func(*testing.T) []byte
	}{
		{name: "Mach-O host binary", architecture: "amd64", archive: func(t *testing.T) []byte { return exposureHelperArchive(t, []byte("Mach-O"), "amd64", nil) }},
		{name: "wrong ELF architecture", architecture: "amd64", archive: func(t *testing.T) []byte {
			return exposureHelperArchive(t, syntheticExposureHelperELF("arm64"), "amd64", nil)
		}},
		{name: "stale API", architecture: "amd64", archive: func(t *testing.T) []byte {
			return exposureHelperArchive(t, validBinary, "amd64", func(identity *exposureHelperIdentity) { identity.API++ })
		}},
		{name: "stale source", architecture: "amd64", archive: func(t *testing.T) []byte {
			return exposureHelperArchive(t, validBinary, "amd64", func(identity *exposureHelperIdentity) { identity.Source = strings.Repeat("0", 64) })
		}},
		{name: "wrong digest", architecture: "amd64", archive: func(t *testing.T) []byte {
			return exposureHelperArchive(t, validBinary, "amd64", func(identity *exposureHelperIdentity) { identity.SHA256 = strings.Repeat("0", 64) })
		}},
		{name: "permission helper Mach-O", architecture: "amd64", archive: func(t *testing.T) []byte {
			return workspaceHelperArchive(t, validBinary, []byte("Mach-O"), "amd64", nil, nil)
		}},
		{name: "permission helper stale API", architecture: "amd64", archive: func(t *testing.T) []byte {
			return workspaceHelperArchive(t, validBinary, validBinary, "amd64", nil, func(identity *exposureHelperIdentity) { identity.API++ })
		}},
		{name: "permission helper wrong digest", architecture: "amd64", archive: func(t *testing.T) []byte {
			return workspaceHelperArchive(t, validBinary, validBinary, "amd64", nil, func(identity *exposureHelperIdentity) { identity.SHA256 = strings.Repeat("0", 64) })
		}},
		{name: "symlink entry", architecture: "amd64", archive: func(t *testing.T) []byte { return exposureHelperSymlinkArchive(t) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			runner := &exposureHelperAssetRunner{architecture: test.architecture, archive: test.archive(t)}
			runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.materializeWorkspaceHelpers(context.Background(), "tobari-runtime:test"); err == nil {
				t.Fatal("expected verified extraction failure")
			}
			if !runnerSaw(runner.runs, "container", "rm") {
				t.Fatalf("extraction container was not cleaned: %v", runner.runs)
			}
		})
	}
}

func TestWorkspaceServiceHelperRejectsNonRegularExistingTargetBeforeReplacement(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	binary := syntheticExposureHelperELF("amd64")
	runner := &exposureHelperAssetRunner{architecture: "amd64", archive: exposureHelperArchive(t, binary, "amd64", nil)}
	runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	version, _ := runtimeassets.Version()
	target := filepath.Join(base, "state", "runtime", version, "helpers", exposureHelperImageBinary)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("elsewhere", target); err != nil {
		t.Fatal(err)
	}
	if err := runtime.materializeWorkspaceHelpers(context.Background(), "tobari-runtime:test"); err == nil {
		t.Fatal("expected symlink target rejection")
	}
	if !runnerSaw(runner.runs, "container", "rm") {
		t.Fatalf("extraction container was not cleaned: %v", runner.runs)
	}
}

func TestLiveWorkspaceServiceHelperExtractionAndCustomRuntimeMount(t *testing.T) {
	if os.Getenv("TOBARI_LIVE_DOCKER_HELPER") != "1" {
		t.Skip("set TOBARI_LIVE_DOCKER_HELPER=1 after building tobari-runtime:dev")
	}
	base := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(base, "config"), filepath.Join(base, "state"), filepath.Join(base, "data"), osCommandRunner{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := runtime.materializeWorkspaceHelpers(ctx, "tobari-runtime:dev"); err != nil {
		t.Fatal(err)
	}
	version, err := runtimeassets.Version()
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "state", "runtime", version, "helpers", exposureHelperImageBinary)
	permissionTarget := filepath.Join(base, "state", "runtime", version, "helpers", permissionHelperImageBinary)
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	server, err := runtime.inspectDockerServer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, engineArchitecture := normalizePlatform(server.OS, server.Architecture)
	if err := validateExposureHelperELF(data, engineArchitecture); err != nil {
		t.Fatal(err)
	}
	permissionData, err := os.ReadFile(permissionTarget)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateExposureHelperELF(permissionData, engineArchitecture); err != nil {
		t.Fatal(err)
	}

	suffix, err := randomExposureHelperExtractionName()
	if err != nil {
		t.Fatal(err)
	}
	image := "tobari-helper-live:" + strings.TrimPrefix(suffix, "tobari-helper-extract-")
	defer func() {
		_ = runtime.runner.Run(context.Background(), []string{"image", "rm", "--force", image}, os.Environ(), nil, io.Discard, io.Discard)
	}()
	dockerfile := strings.NewReader("FROM tobari-runtime:dev\nLABEL io.tobari.integration=workspace-service-helper\n")
	if err := runtime.runner.Run(ctx, []string{"build", "--tag", image, "--file", "-", base}, os.Environ(), dockerfile, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := runtime.runner.Run(ctx, []string{
		"run", "--rm", "--read-only", "--network", "none",
		"--mount", "type=bind,src=" + target + ",dst=/usr/local/bin/tobari-expose,readonly",
		"--entrypoint", "/usr/local/bin/tobari-expose", image, "help",
	}, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := runtime.runner.Run(ctx, []string{
		"run", "--rm", "--read-only", "--network", "none",
		"--mount", "type=bind,src=" + permissionTarget + ",dst=/usr/local/bin/tobari-permission,readonly",
		"--entrypoint", "/usr/local/bin/tobari-permission", image, "help",
	}, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := runtime.runner.Run(ctx, []string{
		"run", "--rm", "--read-only", "--network", "none", "--user", "0:0",
		"--mount", "type=bind,src=" + target + ",dst=/usr/local/bin/tobari-expose,readonly",
		"--entrypoint", "/bin/sh", image, "-c", "printf tamper > /usr/local/bin/tobari-expose",
	}, os.Environ(), nil, io.Discard, io.Discard); err == nil {
		t.Fatal("read-only helper mount accepted a write")
	}
	containers, err := runtime.runner.Output(ctx, []string{"container", "ls", "--all", "--filter", "label=" + componentLabel + "=exposure-helper-extract", "--format", "{{.Names}}"}, os.Environ())
	if err != nil || len(bytes.TrimSpace(containers)) != 0 {
		t.Fatalf("extraction containers remain: %q err=%v", containers, err)
	}
}

func runnerSaw(calls [][]string, prefix ...string) bool {
	for _, call := range calls {
		if len(call) >= len(prefix) && slices.Equal(call[:len(prefix)], prefix) {
			return true
		}
	}
	return false
}

func exposureHelperArchive(t *testing.T, executable []byte, architecture string, mutate func(*exposureHelperIdentity)) []byte {
	t.Helper()
	result, err := exposureHelperArchiveBytes(executable, architecture, mutate)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func exposureHelperArchiveBytes(executable []byte, architecture string, mutate func(*exposureHelperIdentity)) ([]byte, error) {
	return workspaceHelperArchiveBytes(executable, executable, architecture, mutate, nil)
}

func workspaceHelperArchive(
	t *testing.T,
	exposureExecutable, permissionExecutable []byte,
	architecture string,
	mutateExposure, mutatePermission func(*exposureHelperIdentity),
) []byte {
	t.Helper()
	result, err := workspaceHelperArchiveBytes(exposureExecutable, permissionExecutable, architecture, mutateExposure, mutatePermission)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func workspaceHelperArchiveBytes(
	exposureExecutable, permissionExecutable []byte,
	architecture string,
	mutateExposure, mutatePermission func(*exposureHelperIdentity),
) ([]byte, error) {
	source, err := runtimeassets.ExposureHelperSourceVersion()
	if err != nil {
		return nil, err
	}
	exposureDigest := sha256.Sum256(exposureExecutable)
	exposureIdentity := exposureHelperIdentity{
		SchemaVersion: 1, API: exposureHelperAPI, Source: source,
		Architecture: architecture, SHA256: hex.EncodeToString(exposureDigest[:]),
	}
	if mutateExposure != nil {
		mutateExposure(&exposureIdentity)
	}
	permissionDigest := sha256.Sum256(permissionExecutable)
	permissionIdentity := exposureHelperIdentity{
		SchemaVersion: 1, API: exposureHelperAPI, Source: source,
		Architecture: architecture, SHA256: hex.EncodeToString(permissionDigest[:]),
	}
	if mutatePermission != nil {
		mutatePermission(&permissionIdentity)
	}
	exposureIdentityData := []byte(fmt.Sprintf(`{"schema_version":%d,"api":%d,"source":%q,"architecture":%q,"sha256":%q}`, exposureIdentity.SchemaVersion, exposureIdentity.API, exposureIdentity.Source, exposureIdentity.Architecture, exposureIdentity.SHA256))
	permissionIdentityData := []byte(fmt.Sprintf(`{"schema_version":%d,"api":%d,"source":%q,"architecture":%q,"sha256":%q}`, permissionIdentity.SchemaVersion, permissionIdentity.API, permissionIdentity.Source, permissionIdentity.Architecture, permissionIdentity.SHA256))
	var result bytes.Buffer
	archive := tar.NewWriter(&result)
	for name, data := range map[string][]byte{
		exposureHelperImageBinary: exposureExecutable, exposureHelperImageIdentity: exposureIdentityData,
		permissionHelperImageBinary: permissionExecutable, permissionHelperImageIdentity: permissionIdentityData,
	} {
		if err := archive.WriteHeader(&tar.Header{Name: name, Mode: 0o700, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return nil, err
		}
		if _, err := archive.Write(data); err != nil {
			return nil, err
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func exposureHelperSymlinkArchive(t *testing.T) []byte {
	t.Helper()
	var result bytes.Buffer
	archive := tar.NewWriter(&result)
	if err := archive.WriteHeader(&tar.Header{Name: exposureHelperImageBinary, Typeflag: tar.TypeSymlink, Linkname: "/bin/true"}); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return result.Bytes()
}

func syntheticExposureHelperELF(architecture string) []byte {
	result := make([]byte, 64)
	copy(result, []byte{0x7f, 'E', 'L', 'F'})
	result[4] = byte(2)                                     // ELFCLASS64
	result[5] = byte(1)                                     // ELFDATA2LSB
	result[6] = byte(1)                                     // EV_CURRENT
	binary.LittleEndian.PutUint16(result[16:18], uint16(2)) // ET_EXEC
	machine := uint16(62)                                   // EM_X86_64
	if architecture == "arm64" {
		machine = 183 // EM_AARCH64
	}
	binary.LittleEndian.PutUint16(result[18:20], machine)
	binary.LittleEndian.PutUint32(result[20:24], 1)
	binary.LittleEndian.PutUint16(result[52:54], 64)
	return result
}
