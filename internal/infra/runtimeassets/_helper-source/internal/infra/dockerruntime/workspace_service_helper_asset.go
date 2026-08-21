package dockerruntime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
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
	exposureHelperMaxBinaryBytes    = 64 << 20
	exposureHelperMaxIdentityBytes  = 4 << 10
	exposureHelperDiagnosticBytes   = 4 << 10
	exposureHelperExtractionTimeout = 10 * time.Second
	exposureHelperAutoRemoveSeconds = 15
	exposureHelperAPILabel          = "io.tobari.exposure-helper-api"
	exposureHelperSourceLabel       = "io.tobari.exposure-helper-source"
)

type exposureHelperImageMetadata struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	API          string `json:"api"`
	Source       string `json:"source"`
}

type exposureHelperIdentity struct {
	SchemaVersion int    `json:"schema_version"`
	API           int    `json:"api"`
	Source        string `json:"source"`
	Architecture  string `json:"architecture"`
	SHA256        string `json:"sha256"`
}

func (r *Runtime) materializeWorkspaceExposureHelper(ctx context.Context, image string) (resultErr error) {
	expectedSource, err := runtimeassets.ExposureHelperSourceVersion()
	if err != nil {
		return err
	}
	metadata, err := r.inspectExposureHelperImage(ctx, image)
	if err != nil {
		return fmt.Errorf("inspect Workspace service helper image: %w", err)
	}
	server, err := r.inspectDockerServer(ctx)
	if err != nil {
		return fmt.Errorf("inspect Docker Engine for Workspace service helper: %w", err)
	}
	imageOS, imageArch := normalizePlatform(metadata.OS, metadata.Architecture)
	serverOS, serverArch := normalizePlatform(server.OS, server.Architecture)
	if imageOS != "linux" || (imageArch != "amd64" && imageArch != "arm64") ||
		serverOS != imageOS || serverArch != imageArch || metadata.API != "1" || metadata.Source != expectedSource {
		return fmt.Errorf("Workspace service helper image identity is incompatible with this Tobari build and Docker Engine")
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
		"--entrypoint", "/bin/sleep", image, fmt.Sprintf("%d", exposureHelperAutoRemoveSeconds),
	}, os.Environ(), nil, io.Discard, createDiagnostic); err != nil {
		return fmt.Errorf("create Workspace service helper extraction container: %w: %s", err, boundedDiagnostic(createDiagnostic.buffer.Bytes()))
	}
	created = true
	startDiagnostic := &boundedBuffer{limit: exposureHelperDiagnosticBytes}
	if err := r.runner.Run(ctx, []string{"container", "start", name}, os.Environ(), nil, io.Discard, startDiagnostic); err != nil {
		return fmt.Errorf("start bounded Workspace service helper extraction container: %w: %s", err, boundedDiagnostic(startDiagnostic.buffer.Bytes()))
	}

	binary, identityData, err := r.copyExposureHelperArchive(ctx, name)
	if err != nil {
		return err
	}
	var identity exposureHelperIdentity
	if decodeStrictJSON(identityData, &identity) != nil || identity.SchemaVersion != 1 ||
		identity.API != exposureHelperAPI || identity.Source != expectedSource ||
		identity.Architecture != imageArch || !validLowerHex(identity.SHA256, sha256.Size*2) {
		return fmt.Errorf("Workspace service helper identity document is invalid")
	}
	digest := sha256.Sum256(binary)
	if hex.EncodeToString(digest[:]) != identity.SHA256 {
		return fmt.Errorf("Workspace service helper digest does not match its verified image identity")
	}
	if err := validateExposureHelperELF(binary, imageArch); err != nil {
		return err
	}
	version, err := runtimeassets.Version()
	if err != nil {
		return err
	}
	target := filepath.Join(r.stateDirectory, "runtime", version, "helpers", exposureHelperImageBinary)
	if err := replaceExposureHelperFile(target, binary); err != nil {
		return fmt.Errorf("activate Workspace service helper: %w", err)
	}
	return nil
}

func (r *Runtime) inspectExposureHelperImage(ctx context.Context, image string) (exposureHelperImageMetadata, error) {
	output, err := r.runner.Output(ctx, []string{
		"image", "inspect", "--format",
		`{"architecture":{{json .Architecture}},"os":{{json .Os}},"api":{{json (index .Config.Labels "` + exposureHelperAPILabel + `")}},"source":{{json (index .Config.Labels "` + exposureHelperSourceLabel + `")}}}`,
		image,
	}, os.Environ())
	if err != nil {
		return exposureHelperImageMetadata{}, err
	}
	var metadata exposureHelperImageMetadata
	if decodeStrictJSON(bytes.TrimSpace(output), &metadata) != nil {
		return exposureHelperImageMetadata{}, fmt.Errorf("decode Workspace service helper image identity")
	}
	return metadata, nil
}

func (r *Runtime) copyExposureHelperArchive(ctx context.Context, container string) ([]byte, []byte, error) {
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
	binary, identity, parseErr := readExposureHelperArchive(reader)
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
	return binary, identity, nil
}

func readExposureHelperArchive(source io.Reader) ([]byte, []byte, error) {
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
		case exposureHelperImageBinary:
			limit = exposureHelperMaxBinaryBytes
		case exposureHelperImageIdentity:
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
	if len(files) != 2 || files[exposureHelperImageBinary] == nil || files[exposureHelperImageIdentity] == nil {
		return nil, nil, fmt.Errorf("Workspace service helper archive is incomplete")
	}
	return files[exposureHelperImageBinary], files[exposureHelperImageIdentity], nil
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
	temporary, err := os.CreateTemp(directory, ".tobari-expose-*")
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
