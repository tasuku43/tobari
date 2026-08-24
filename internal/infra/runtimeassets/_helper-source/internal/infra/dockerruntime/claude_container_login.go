package dockerruntime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

const (
	maxClaudeCredentialArchive = 40 << 10
	maxClaudeExecutableArchive = 512 << 20
	maxClaudeDiagnosticOutput  = 4 << 10
	claudeLoginCleanupTimeout  = 10 * time.Second
	claudeLoginMemoryLimit     = "512m"
)

func (r *Runtime) loginClaudeInContextContainer(
	ctx context.Context, contextID string, input io.Reader, visible io.Writer,
) (payload hostCredentialPayload, resultErr error) {
	manifest, _, err := r.contextByID(contextID)
	if err != nil {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "anthropic", stage: hostCLIStageClaudeContextSelection}
	}
	return r.loginClaudeInRuntimeImage(ctx, manifest.Image, input, visible)
}

func (r *Runtime) loginClaudeInRuntimeImage(
	ctx context.Context, runtimeImage string, input io.Reader, visible io.Writer,
) (payload hostCredentialPayload, resultErr error) {
	image := r.resolveBuiltinImageSelector(runtimeImage)
	if err := r.validateCompatibleImage(ctx, image); err != nil {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "anthropic", stage: hostCLIStageClaudeImageContract}
	}
	imageOutput, err := r.boundedClaudeDockerOutput(ctx, []string{"image", "inspect", "--format", "{{.Id}}", image})
	imageID := strings.TrimSpace(string(imageOutput))
	if err != nil || !claudeImageIDPattern.MatchString(imageID) {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "anthropic", stage: hostCLIStageClaudeImageContract}
	}
	name, err := randomClaudeLoginContainerName()
	if err != nil {
		return hostCredentialPayload{}, credentialhost.ErrClaudeLoginSetup
	}
	created := false
	defer func() {
		if !created {
			return
		}
		cleanupContext, cancelCleanup := context.WithTimeout(ctx, claudeLoginCleanupTimeout)
		defer cancelCleanup()
		cleanupErr := r.runner.Run(cleanupContext, []string{"container", "rm", "--force", name}, os.Environ(), nil, io.Discard, io.Discard)
		if cleanupErr != nil {
			payload.clear()
			if resultErr == nil {
				resultErr = credentialhost.ErrClaudeLoginCleanup
			} else {
				resultErr = errors.Join(resultErr, credentialhost.ErrClaudeLoginCleanup)
			}
		}
	}()
	createArgs := []string{
		"container", "create", "--name", name, "--label", ownerLabel + "=" + ownerValue,
		"--label", componentLabel + "=claude-login", "--interactive", "--tty",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "128",
		"--memory", claudeLoginMemoryLimit, "--memory-swap", claudeLoginMemoryLimit, "--cpus", "1", "--hostname", "tobari-claude-login",
		"--entrypoint", "/usr/bin/tini", image, "--", "/usr/bin/sleep", "infinity",
	}
	if err := r.runner.Run(ctx, createArgs, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		return hostCredentialPayload{}, credentialhost.ErrClaudeLoginSetup
	}
	created = true
	if err := r.runner.Run(ctx, []string{"container", "start", name}, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		return hostCredentialPayload{}, credentialhost.ErrClaudeLoginSetup
	}
	versionOutput, err := r.boundedClaudeDockerOutput(ctx, []string{"container", "exec", name, "/usr/local/bin/claude", "--version"})
	if err != nil || string(versionOutput) != "2.1.220 (Claude Code)\n" {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "anthropic", stage: hostCLIStageClaudeVersionObservation}
	}
	digest, err := r.hashClaudeExecutable(ctx, name)
	if err != nil {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "anthropic", stage: hostCLIStageClaudeExecutableIdentity}
	}
	loginArgs := []string{
		"container", "exec", "--interactive", "--tty", "--env", "NO_COLOR=1", "--env", "DISABLE_AUTOUPDATER=1",
		name, "/usr/local/bin/claude", "auth", "login", "--claudeai",
	}
	if err := r.runner.Run(ctx, loginArgs, os.Environ(), input, visible, visible); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return hostCredentialPayload{}, contextErr
		}
		return hostCredentialPayload{}, credentialhost.ErrClaudeLoginFailed
	}
	if err := writeClaudeProgress(visible, claudeLoginCaptureFeedback); err != nil {
		return hostCredentialPayload{}, err
	}
	archive := &claudeArchiveBuffer{limit: maxClaudeCredentialArchive}
	var copyErr bytes.Buffer
	if err := r.runner.Run(ctx, []string{"container", "cp", name + ":/var/lib/tobari/.claude/.credentials.json", "-"}, os.Environ(), nil, archive, &copyErr); err != nil || archive.failure != nil {
		return hostCredentialPayload{}, credentialhost.NewClaudeCredentialCaptureError(
			credentialhost.ClaudeCaptureFileExport, credentialhost.ErrClaudeTokenCapture,
		)
	}
	authJSON, err := readClaudeCredentialArchive(archive.buffer.Bytes())
	if err != nil {
		return hostCredentialPayload{}, err
	}
	defer clear(authJSON)
	credential, err := credentialhost.NewClaudeNativeCredential(authJSON, imageID, digest, "2.1.220")
	if err != nil {
		return hostCredentialPayload{}, err
	}
	defer credential.Clear()
	if output, ok := visible.(*loginVisibleOutput); ok {
		requested, observed := output.requestedClaudeOAuthScopes()
		if !observed || !scopesAreSubset(credential.OAuthScopes(), requested) {
			return hostCredentialPayload{}, credentialhost.NewClaudeCredentialCaptureError(
				credentialhost.ClaudeCaptureOAuthScopeSet, credentialhost.ErrInvalidClaudeNativeCredential,
			)
		}
	}
	encoded, err := credential.Encode()
	if err != nil {
		return hostCredentialPayload{}, err
	}
	return hostCredentialPayload{
		secret: encoded, accountLabel: credential.AccountLabel(), driverID: credential.DriverID(), driverRevision: credential.DriverRevision(),
	}, nil
}

func scopesAreSubset(granted, requested []string) bool {
	for _, scope := range granted {
		if !slices.Contains(requested, scope) {
			return false
		}
	}
	return true
}

func (r *Runtime) boundedClaudeDockerOutput(ctx context.Context, arguments []string) ([]byte, error) {
	output := &claudeArchiveBuffer{limit: maxClaudeDiagnosticOutput}
	if err := r.runner.Run(ctx, arguments, os.Environ(), nil, output, output); err != nil || output.failure != nil {
		return nil, credentialhost.ErrClaudeOutputLimit
	}
	return append([]byte(nil), output.buffer.Bytes()...), nil
}

// hashClaudeExecutable reads the executable archive through the Docker Engine
// rather than trusting a hash command supplied by the selected Context image.
// The binary is streamed and discarded; it never becomes host executable state.
func (r *Runtime) hashClaudeExecutable(ctx context.Context, container string) (string, error) {
	reader, writer := io.Pipe()
	runResult := make(chan error, 1)
	go func() {
		err := r.runner.Run(
			ctx, []string{"container", "cp", container + ":/usr/local/bin/claude", "-"},
			os.Environ(), nil, writer, io.Discard,
		)
		_ = writer.CloseWithError(err)
		runResult <- err
	}()

	archive := tar.NewReader(reader)
	header, parseErr := archive.Next()
	if parseErr == nil && (header.Typeflag != tar.TypeReg || path.Base(header.Name) != "claude" ||
		header.Size <= 0 || header.Size > maxClaudeExecutableArchive || header.Mode&0111 == 0) {
		parseErr = credentialhost.ErrClaudeExecutable
	}
	hasher := sha256.New()
	if parseErr == nil {
		var copied int64
		copied, parseErr = io.CopyN(hasher, archive, header.Size)
		if parseErr == nil && copied != header.Size {
			parseErr = io.ErrUnexpectedEOF
		}
	}
	if parseErr == nil {
		if _, err := archive.Next(); !errors.Is(err, io.EOF) {
			parseErr = credentialhost.ErrClaudeExecutable
		}
	}
	if parseErr != nil {
		_ = reader.CloseWithError(parseErr)
	} else {
		_ = reader.Close()
	}
	runErr := <-runResult
	if parseErr != nil || runErr != nil {
		return "", credentialhost.ErrClaudeExecutable
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func randomClaudeLoginContainerName() (string, error) {
	var value [12]byte
	if _, err := io.ReadFull(rand.Reader, value[:]); err != nil {
		return "", err
	}
	return "tobari-claude-login-" + hex.EncodeToString(value[:]), nil
}

func readClaudeCredentialArchive(encoded []byte) ([]byte, error) {
	reader := tar.NewReader(bytes.NewReader(encoded))
	header, err := reader.Next()
	if err != nil || header.Typeflag != tar.TypeReg || header.Size <= 0 || header.Size > maxClaudeCredentialArchive {
		return nil, credentialhost.NewClaudeCredentialCaptureError(
			credentialhost.ClaudeCaptureArchiveEnvelope, credentialhost.ErrClaudeTokenCapture,
		)
	}
	if header.Mode&077 != 0 {
		return nil, credentialhost.NewClaudeCredentialCaptureError(
			credentialhost.ClaudeCaptureFilePermissions, credentialhost.ErrClaudeTokenCapture,
		)
	}
	content, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
	if err != nil || int64(len(content)) != header.Size {
		clear(content)
		return nil, credentialhost.NewClaudeCredentialCaptureError(
			credentialhost.ClaudeCaptureArchiveEnvelope, credentialhost.ErrClaudeTokenCapture,
		)
	}
	if _, err := reader.Next(); !errors.Is(err, io.EOF) {
		clear(content)
		return nil, credentialhost.NewClaudeCredentialCaptureError(
			credentialhost.ClaudeCaptureArchiveEnvelope, credentialhost.ErrClaudeTokenCapture,
		)
	}
	return content, nil
}

func writeClaudeProgress(visible io.Writer, value string) error {
	if output, ok := visible.(*loginVisibleOutput); ok {
		return output.writeClaudeProgress(value)
	}
	_, err := io.WriteString(visible, value+"\n")
	return err
}

var claudeImageIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type claudeArchiveBuffer struct {
	buffer  bytes.Buffer
	limit   int
	failure error
}

func (b *claudeArchiveBuffer) Write(content []byte) (int, error) {
	if b.failure != nil {
		return 0, b.failure
	}
	if len(content) > b.limit-b.buffer.Len() {
		b.failure = credentialhost.ErrClaudeOutputLimit
		return 0, b.failure
	}
	return b.buffer.Write(content)
}
