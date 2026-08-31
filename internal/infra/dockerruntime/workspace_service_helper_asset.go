package dockerruntime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const (
	exposureHelperAPI               = 1
	exposureHelperImageDirectory    = "/opt/tobari/libexec"
	exposureHelperImageBinary       = "tobari-expose"
	exposureHelperImageIdentity     = "tobari-expose.identity.json"
	permissionHelperImageBinary     = "tobari-permission"
	permissionHelperImageIdentity   = "tobari-permission.identity.json"
	exposureHelperMaxBinaryBytes    = 64 << 20
	exposureHelperMaxIdentityBytes  = 4 << 10
	exposureHelperDiagnosticBytes   = 4 << 10
	exposureHelperExtractionTimeout = 10 * time.Second
	exposureHelperAutoRemoveSeconds = 15
	exposureHelperAPILabel          = "io.tobari.exposure-helper-api"
	exposureHelperSourceLabel       = "io.tobari.exposure-helper-source"
	permissionHelperAPILabel        = "io.tobari.permission-helper-api"
	permissionHelperSourceLabel     = "io.tobari.permission-helper-source"
)

type exposureHelperImageMetadata struct {
	ID               string                     `json:"id"`
	Architecture     string                     `json:"architecture"`
	OS               string                     `json:"os"`
	ExposureAPI      string                     `json:"exposure_api"`
	ExposureSource   string                     `json:"exposure_source"`
	PermissionAPI    string                     `json:"permission_api"`
	PermissionSource string                     `json:"permission_source"`
	Volumes          map[string]json.RawMessage `json:"volumes"`
}

type exposureHelperIdentity struct {
	SchemaVersion int    `json:"schema_version"`
	API           int    `json:"api"`
	Source        string `json:"source"`
	Architecture  string `json:"architecture"`
	SHA256        string `json:"sha256"`
}

type workspaceHelperArtifact struct {
	binaryName   string
	identityName string
}

var workspaceHelperArtifacts = []workspaceHelperArtifact{
	{binaryName: exposureHelperImageBinary, identityName: exposureHelperImageIdentity},
	{binaryName: permissionHelperImageBinary, identityName: permissionHelperImageIdentity},
}

func (r *Runtime) materializeWorkspaceHelpers(ctx context.Context, image string) (resultErr error) {
	expectedSource, err := runtimeassets.ExposureHelperSourceVersion()
	if err != nil {
		return err
	}
	metadata, err := r.inspectExposureHelperImage(ctx, image)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("inspect Workspace service helper image: %w", err)
	}
	server, err := r.inspectDockerServer(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("inspect Docker Engine for Workspace service helper: %w", err)
	}
	imageOS, imageArch := normalizePlatform(metadata.OS, metadata.Architecture)
	serverOS, serverArch := normalizePlatform(server.OS, server.Architecture)
	if imageOS != "linux" || (imageArch != "amd64" && imageArch != "arm64") ||
		serverOS != imageOS || serverArch != imageArch ||
		metadata.ExposureAPI != "1" || metadata.ExposureSource != expectedSource ||
		metadata.PermissionAPI != "1" || metadata.PermissionSource != expectedSource ||
		!imageIDPattern.MatchString(metadata.ID) || validateComponentImageVolumes(metadata.Volumes) != nil {
		return fmt.Errorf("Workspace helper image identity is incompatible with this Tobari build and Docker Engine")
	}

	name, err := randomExposureHelperExtractionName()
	if err != nil {
		return fmt.Errorf("name Workspace service helper extraction: %w", err)
	}
	created := false
	defer func() {
		if !created {
			return
		}
		// The extraction container is created with Docker auto-removal and a
		// finite sleep command, so caller cancellation leaves daemon-owned
		// cleanup bounded. While the caller remains active, remove it eagerly.
		if ctx.Err() != nil {
			return
		}
		cleanupContext, cancel := context.WithTimeout(ctx, exposureHelperExtractionTimeout)
		defer cancel()
		cleanupErr := r.runner.Run(cleanupContext, []string{"container", "rm", "--force", name}, os.Environ(), nil, io.Discard, io.Discard)
		if cleanupErr != nil {
			cleanupErr = fmt.Errorf("remove Workspace service helper extraction container: %w", cleanupErr)
			if resultErr == nil {
				resultErr = cleanupErr
			} else {
				resultErr = errors.Join(resultErr, cleanupErr)
			}
		}
	}()
	createDiagnostic := &boundedBuffer{limit: exposureHelperDiagnosticBytes}
	if err := r.runner.Run(ctx, []string{
		"container", "create", "--name", name,
		"--rm", "--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--user", "65534:65534",
		"--label", ownerLabel + "=" + ownerValue,
		"--label", componentLabel + "=exposure-helper-extract",
		"--entrypoint", "/bin/sleep", metadata.ID, fmt.Sprintf("%d", exposureHelperAutoRemoveSeconds),
	}, os.Environ(), nil, io.Discard, createDiagnostic); err != nil {
		return fmt.Errorf("create Workspace service helper extraction container: %w: %s", err, boundedDiagnostic(createDiagnostic.buffer.Bytes()))
	}
	created = true
	startDiagnostic := &boundedBuffer{limit: exposureHelperDiagnosticBytes}
	if err := r.runner.Run(ctx, []string{"container", "start", name}, os.Environ(), nil, io.Discard, startDiagnostic); err != nil {
		return fmt.Errorf("start bounded Workspace service helper extraction container: %w: %s", err, boundedDiagnostic(startDiagnostic.buffer.Bytes()))
	}

	binaries, identities, err := r.copyExposureHelperArchive(ctx, name)
	if err != nil {
		return err
	}
	for _, artifact := range workspaceHelperArtifacts {
		binary, identityData := binaries[artifact.binaryName], identities[artifact.identityName]
		var identity exposureHelperIdentity
		if decodeStrictJSON(identityData, &identity) != nil || identity.SchemaVersion != 1 ||
			identity.API != exposureHelperAPI || identity.Source != expectedSource ||
			identity.Architecture != imageArch || !validLowerHex(identity.SHA256, sha256.Size*2) {
			return fmt.Errorf("Workspace helper %q identity document is invalid", artifact.binaryName)
		}
		digest := sha256.Sum256(binary)
		if hex.EncodeToString(digest[:]) != identity.SHA256 {
			return fmt.Errorf("Workspace helper %q digest does not match its verified image identity", artifact.binaryName)
		}
		if err := validateExposureHelperELF(binary, imageArch); err != nil {
			return fmt.Errorf("validate Workspace helper %q: %w", artifact.binaryName, err)
		}
	}
	version, err := runtimeassets.Version()
	if err != nil {
		return err
	}
	for _, artifact := range workspaceHelperArtifacts {
		target := filepath.Join(r.stateDirectory, "runtime", version, "helpers", artifact.binaryName)
		if err := replaceExposureHelperFile(target, binaries[artifact.binaryName]); err != nil {
			return fmt.Errorf("activate Workspace helper %q: %w", artifact.binaryName, err)
		}
	}
	return nil
}

func (r *Runtime) inspectExposureHelperImage(ctx context.Context, image string) (exposureHelperImageMetadata, error) {
	observed, err := r.inspectBoundedComponentImage(ctx, image)
	if err != nil {
		return exposureHelperImageMetadata{}, err
	}
	labels := observed.Config.Labels
	metadata := exposureHelperImageMetadata{
		ID: observed.ID, Architecture: observed.Architecture, OS: observed.OS,
		ExposureAPI: labels[exposureHelperAPILabel], ExposureSource: labels[exposureHelperSourceLabel],
		PermissionAPI: labels[permissionHelperAPILabel], PermissionSource: labels[permissionHelperSourceLabel],
		Volumes: observed.Config.Volumes,
	}
	return metadata, nil
}

func (r *Runtime) copyExposureHelperArchive(ctx context.Context, container string) (map[string][]byte, map[string][]byte, error) {
	reader, writer := io.Pipe()
	runResult := make(chan error, 1)
	go func() {
		diagnostic := &boundedBuffer{limit: exposureHelperDiagnosticBytes}
		err := r.runner.Run(ctx, []string{"container", "cp", container + ":" + exposureHelperImageDirectory + "/.", "-"}, os.Environ(), nil, writer, diagnostic)
		if err != nil {
			err = fmt.Errorf("copy Workspace service helper from verified image: %w: %s", err, boundedDiagnostic(diagnostic.buffer.Bytes()))
		}
		_ = writer.CloseWithError(err)
		runResult <- err
	}()
	binaries, identities, parseErr := readExposureHelperArchive(reader)
	if parseErr != nil {
		_ = reader.CloseWithError(parseErr)
	} else {
		_ = reader.Close()
	}
	runErr := <-runResult
	if parseErr != nil {
		return nil, nil, parseErr
	}
	if runErr != nil {
		return nil, nil, runErr
	}
	return binaries, identities, nil
}

func readExposureHelperArchive(source io.Reader) (map[string][]byte, map[string][]byte, error) {
	archive := tar.NewReader(source)
	files := map[string][]byte{}
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read Workspace service helper archive: %w", err)
		}
		cleaned := strings.TrimPrefix(path.Clean("/"+header.Name), "/")
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg || path.Base(cleaned) != cleaned || files[cleaned] != nil {
			return nil, nil, fmt.Errorf("Workspace service helper archive contains an unexpected entry")
		}
		limit := int64(0)
		switch cleaned {
		case exposureHelperImageBinary, permissionHelperImageBinary:
			limit = exposureHelperMaxBinaryBytes
		case exposureHelperImageIdentity, permissionHelperImageIdentity:
			limit = exposureHelperMaxIdentityBytes
		default:
			return nil, nil, fmt.Errorf("Workspace service helper archive contains an unexpected file")
		}
		if header.Size < 1 || header.Size > limit {
			return nil, nil, fmt.Errorf("Workspace service helper archive file size is invalid")
		}
		data, err := io.ReadAll(io.LimitReader(archive, header.Size+1))
		if err != nil || int64(len(data)) != header.Size {
			return nil, nil, fmt.Errorf("Workspace service helper archive file is incomplete")
		}
		files[cleaned] = data
	}
	if len(files) != len(workspaceHelperArtifacts)*2 {
		return nil, nil, fmt.Errorf("Workspace service helper archive is incomplete")
	}
	binaries := map[string][]byte{}
	identities := map[string][]byte{}
	for _, artifact := range workspaceHelperArtifacts {
		if files[artifact.binaryName] == nil || files[artifact.identityName] == nil {
			return nil, nil, fmt.Errorf("Workspace service helper archive is incomplete")
		}
		binaries[artifact.binaryName] = files[artifact.binaryName]
		identities[artifact.identityName] = files[artifact.identityName]
	}
	return binaries, identities, nil
}

func validateExposureHelperELF(binary []byte, architecture string) error {
	parsed, err := elf.NewFile(bytes.NewReader(binary))
	if err != nil || parsed.Class != elf.ELFCLASS64 || parsed.Data != elf.ELFDATA2LSB || parsed.Type != elf.ET_EXEC {
		return fmt.Errorf("Workspace service helper is not a supported Linux ELF executable")
	}
	defer parsed.Close()
	wantMachine := elf.EM_X86_64
	if architecture == "arm64" {
		wantMachine = elf.EM_AARCH64
	}
	if parsed.Machine != wantMachine || (parsed.OSABI != elf.ELFOSABI_NONE && parsed.OSABI != elf.ELFOSABI_LINUX) {
		return fmt.Errorf("Workspace service helper architecture does not match the Docker Engine")
	}
	return nil
}

func replaceExposureHelperFile(target string, data []byte) error {
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("existing helper path must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	directory := filepath.Dir(target)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".tobari-helper-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o700); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return err
	}
	parent, err := os.Open(directory) // #nosec G304 -- fixed owner-state helper directory.
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

func randomExposureHelperExtractionName() (string, error) {
	var value [12]byte
	if _, err := io.ReadFull(rand.Reader, value[:]); err != nil {
		return "", err
	}
	return "tobari-helper-extract-" + hex.EncodeToString(value[:]), nil
}

func validLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}
