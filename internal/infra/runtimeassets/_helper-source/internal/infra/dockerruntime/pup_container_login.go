package dockerruntime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

const (
	pupExecutablePath          = "/usr/local/bin/pup"
	pupConfigDirectory         = "/var/lib/tobari/.config/pup"
	pupLoginContainerTimeout   = 5 * time.Minute
	pupLoginCleanupTimeout     = 10 * time.Second
	pupLoginMemoryLimit        = "256m"
	maxPupExecutableArchive    = 256 << 20
	maxPupCredentialArchive    = 40 << 10
	maxPupDiagnosticOutput     = 40 << 10
	maxPupVersionOutput        = 160
	pupLoginValidationFeedback = "Datadog authorization complete. Validating this Context credential…\n"
)

var (
	pupImageIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	pupVersionPattern = regexp.MustCompile(`^pup v?((?:0|[1-9][0-9]{0,5})\.(?:0|[1-9][0-9]{0,5})\.(?:0|[1-9][0-9]{0,5})(?:-[0-9A-Za-z][0-9A-Za-z.-]{0,63})?(?:\+[0-9A-Za-z][0-9A-Za-z.-]{0,63})?)\n$`)
)

type pupRuntimeIdentity struct {
	version string
	digest  string
}

func (r *Runtime) loginPupInContextContainer(
	ctx context.Context, contextID string, _ io.Reader, visible io.Writer,
) (payload hostCredentialPayload, resultErr error) {
	if ctx == nil || visible == nil {
		return hostCredentialPayload{}, credentialhost.ErrPupLoginSetup
	}
	manifest, _, err := r.contextByID(contextID)
	if err != nil {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupContextSelection}
	}
	image := r.resolveBuiltinImageSelector(manifest.Image)
	if err := r.validateCompatibleImage(ctx, image); err != nil {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupImageContract}
	}
	imageOutput, err := r.boundedPupDockerOutput(ctx, []string{"image", "inspect", "--format", "{{.Id}}", image})
	imageID := strings.TrimSpace(string(imageOutput))
	if err != nil || !pupImageIDPattern.MatchString(imageID) {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupImageContract}
	}
	identity, err := r.preflightPupContextImage(ctx, imageID)
	if err != nil {
		return hostCredentialPayload{}, err
	}
	name, err := randomPupLoginContainerName()
	if err != nil {
		return hostCredentialPayload{}, credentialhost.ErrPupLoginSetup
	}
	created := false
	defer func() {
		if !created {
			return
		}
		cleanupContext, cancelCleanup := context.WithTimeout(ctx, pupLoginCleanupTimeout)
		defer cancelCleanup()
		cleanupErr := r.runner.Run(cleanupContext, []string{"container", "rm", "--force", name}, os.Environ(), nil, io.Discard, io.Discard)
		if cleanupErr != nil {
			payload.clear()
			if resultErr == nil {
				resultErr = credentialhost.ErrPupLoginCleanup
			} else {
				resultErr = errors.Join(resultErr, credentialhost.ErrPupLoginCleanup)
			}
		}
	}()

	createArgs := []string{
		"container", "create", "--name", name, "--label", ownerLabel + "=" + ownerValue,
		"--label", componentLabel + "=pup-login", "--interactive",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "64",
		"--memory", pupLoginMemoryLimit, "--memory-swap", pupLoginMemoryLimit, "--cpus", "1", "--hostname", "tobari-pup-login",
		"--entrypoint", "/usr/bin/tini", imageID, "--", "/usr/bin/sleep", "infinity",
	}
	if err := r.runner.Run(ctx, createArgs, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		return hostCredentialPayload{}, credentialhost.ErrPupLoginSetup
	}
	created = true
	if err := r.runner.Run(ctx, []string{"container", "start", name}, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		return hostCredentialPayload{}, credentialhost.ErrPupLoginSetup
	}
	containerImage, err := r.boundedPupDockerOutput(ctx, []string{"container", "inspect", "--format", "{{.Image}}", name})
	if err != nil || strings.TrimSpace(string(containerImage)) != imageID {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupImageContract}
	}
	loginContext, cancelLogin := context.WithTimeout(ctx, pupLoginContainerTimeout)
	defer cancelLogin()
	callbackReader, callbackWriter := io.Pipe()
	relayFactory := r.pupRelayFactory
	if relayFactory == nil {
		relayFactory = newPupLoginRelay
	}
	relay, err := relayFactory(loginContext, callbackWriter)
	if err != nil {
		_ = callbackReader.Close()
		_ = callbackWriter.Close()
		return hostCredentialPayload{}, credentialhost.ErrPupLoginSetup
	}
	relayCompleted := false
	defer func() {
		if !relayCompleted {
			relay.Complete(resultErr)
		}
		_ = callbackReader.Close()
		if closeErr := relay.Close(); closeErr != nil {
			payload.clear()
			if resultErr == nil {
				resultErr = credentialhost.ErrPupLoginCleanup
			} else {
				resultErr = errors.Join(resultErr, credentialhost.ErrPupLoginCleanup)
			}
		}
	}()

	loginArgs := append([]string{"container", "exec", "--interactive"}, pupExecEnvironment()...)
	loginArgs = append(loginArgs, name, pupExecutablePath, "--no-agent", "auth", "login", "--site", credentialhost.PupSite, "--callback-port", "8000")
	runErr := r.runner.Run(loginContext, loginArgs, os.Environ(), callbackReader, visible, visible)
	if loginContext.Err() != nil {
		relay.Complete(loginContext.Err())
		relayCompleted = true
		return hostCredentialPayload{}, loginContext.Err()
	}
	if runErr != nil {
		relay.Complete(credentialhost.ErrPupLoginFailed)
		relayCompleted = true
		return hostCredentialPayload{}, credentialhost.ErrPupLoginFailed
	}
	if _, err := io.WriteString(visible, pupLoginValidationFeedback); err != nil {
		return hostCredentialPayload{}, err
	}

	if err := r.validatePupContainerStatus(ctx, name); err != nil {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupCaptureContract}
	}
	client, err := r.copyPupCredentialFile(ctx, name, "client_datadoghq_com.json", 8<<10)
	if err != nil {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupCaptureContract}
	}
	defer clear(client)
	tokens, err := r.copyPupCredentialFile(ctx, name, "tokens_datadoghq_com.json", 32<<10)
	if err != nil {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupCaptureContract}
	}
	defer clear(tokens)
	sessions, err := r.copyPupCredentialFile(ctx, name, "sessions.json", 8<<10)
	if err != nil {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupCaptureContract}
	}
	defer clear(sessions)
	state, err := credentialhost.NewPupStateFromNativeFiles(
		pupExecutablePath, identity.digest, client, tokens, sessions,
	)
	if err != nil {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupStateContract}
	}
	defer state.Clear()
	encoded, err := state.Encode()
	if err != nil {
		return hostCredentialPayload{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupStateContract}
	}
	relay.Complete(nil)
	relayCompleted = true
	return hostCredentialPayload{
		secret: encoded, accountLabel: credentialhost.PupAccountLabel,
		driverID: state.DriverID(), driverRevision: state.DriverRevision(),
	}, nil
}

// preflightPupContextImage establishes the immutable writer identity before a
// credential-bearing container exists. Version syntax is diagnostic evidence,
// not a compiled compatibility allowlist; the fixed login and strict capture
// below decide structural conformance.
func (r *Runtime) preflightPupContextImage(
	ctx context.Context, imageID string,
) (identity pupRuntimeIdentity, resultErr error) {
	name, err := randomPupContainerName("preflight")
	if err != nil {
		return pupRuntimeIdentity{}, credentialhost.ErrPupLoginSetup
	}
	created := false
	defer func() {
		if !created {
			return
		}
		cleanupContext, cancelCleanup := context.WithTimeout(ctx, pupLoginCleanupTimeout)
		defer cancelCleanup()
		cleanupErr := r.runner.Run(
			cleanupContext, []string{"container", "rm", "--force", name},
			os.Environ(), nil, io.Discard, io.Discard,
		)
		if cleanupErr != nil {
			identity = pupRuntimeIdentity{}
			if resultErr == nil {
				resultErr = credentialhost.ErrPupLoginCleanup
			} else {
				resultErr = errors.Join(resultErr, credentialhost.ErrPupLoginCleanup)
			}
		}
	}()

	createArgs := []string{
		"container", "create", "--name", name,
		"--label", ownerLabel + "=" + ownerValue,
		"--label", componentLabel + "=pup-login-preflight",
		"--network", "none", "--read-only",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "32", "--memory", pupLoginMemoryLimit,
		"--memory-swap", pupLoginMemoryLimit, "--cpus", "1",
		"--entrypoint", pupExecutablePath, imageID, "--version",
	}
	if err := r.runner.Run(ctx, createArgs, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		return pupRuntimeIdentity{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupExecutableIdentity}
	}
	created = true
	versionOutput := &pupBoundedBuffer{limit: maxPupVersionOutput}
	if err := r.runner.Run(
		ctx, []string{"container", "start", "--attach", name},
		os.Environ(), nil, versionOutput, versionOutput,
	); err != nil || versionOutput.failure != nil {
		return pupRuntimeIdentity{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupExecutableIdentity}
	}
	match := pupVersionPattern.FindSubmatch(versionOutput.buffer.Bytes())
	if len(match) != 2 {
		return pupRuntimeIdentity{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupVersionObservation}
	}
	digest, err := r.hashPupExecutable(ctx, name)
	if err != nil {
		return pupRuntimeIdentity{}, hostCLIUnavailableError{provider: "datadog", stage: hostCLIStagePupExecutableIdentity}
	}
	return pupRuntimeIdentity{version: string(match[1]), digest: digest}, nil
}

func pupExecEnvironment() []string {
	return []string{
		"--env", "HOME=/var/lib/tobari",
		"--env", "PUP_CONFIG_DIR=" + pupConfigDirectory,
		"--env", "DD_TOKEN_STORAGE=file",
		"--env", "DD_SITE=" + credentialhost.PupSite,
		"--env", "PUP_OAUTH_CALLBACK_PORT=8000",
		"--env", "BROWSER=/bin/false",
		"--env", "NO_COLOR=1",
		"--env", "LC_ALL=C",
	}
}

func (r *Runtime) validatePupContainerStatus(ctx context.Context, container string) error {
	arguments := append([]string{"container", "exec"}, pupExecEnvironment()...)
	arguments = append(arguments, container, pupExecutablePath, "--no-agent", "auth", "status", "--site", credentialhost.PupSite)
	stdout := &pupBoundedBuffer{limit: maxPupDiagnosticOutput}
	stderr := &pupBoundedBuffer{limit: maxPupDiagnosticOutput}
	if err := r.runner.Run(ctx, arguments, os.Environ(), nil, stdout, stderr); err != nil || stdout.failure != nil || stderr.failure != nil {
		return credentialhost.ErrInvalidPupState
	}
	defer clear(stdout.buffer.Bytes())
	var status struct {
		Authenticated bool     `json:"authenticated"`
		ExpiresAt     string   `json:"expires_at"`
		HasRefresh    bool     `json:"has_refresh"`
		Org           *string  `json:"org"`
		Scopes        []string `json:"scopes"`
		Site          string   `json:"site"`
		Status        string   `json:"status"`
		TokenType     string   `json:"token_type"`
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout.buffer.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&status); err != nil || status.Org != nil || !status.Authenticated || !status.HasRefresh ||
		status.ExpiresAt == "" || len(status.Scopes) == 0 || status.Site != credentialhost.PupSite ||
		status.Status != "valid" || status.TokenType != "Bearer" {
		return credentialhost.ErrInvalidPupState
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return credentialhost.ErrInvalidPupState
	}
	return nil
}

func (r *Runtime) copyPupCredentialFile(ctx context.Context, container, name string, limit int64) ([]byte, error) {
	archive := &pupBoundedBuffer{limit: maxPupCredentialArchive}
	defer func() { clear(archive.buffer.Bytes()) }()
	if err := r.runner.Run(
		ctx, []string{"container", "cp", container + ":" + pupConfigDirectory + "/" + name, "-"},
		os.Environ(), nil, archive, io.Discard,
	); err != nil || archive.failure != nil {
		return nil, credentialhost.ErrInvalidPupState
	}
	reader := tar.NewReader(bytes.NewReader(archive.buffer.Bytes()))
	header, err := reader.Next()
	if err != nil || header.Typeflag != tar.TypeReg || path.Base(header.Name) != name ||
		header.Size <= 0 || header.Size > limit || header.Mode&0o777 != 0o600 {
		return nil, credentialhost.ErrInvalidPupState
	}
	content, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
	if err != nil || int64(len(content)) != header.Size {
		clear(content)
		return nil, credentialhost.ErrInvalidPupState
	}
	if _, err := reader.Next(); !errors.Is(err, io.EOF) {
		clear(content)
		return nil, credentialhost.ErrInvalidPupState
	}
	return content, nil
}

func (r *Runtime) boundedPupDockerOutput(ctx context.Context, arguments []string) ([]byte, error) {
	output := &pupBoundedBuffer{limit: maxPupDiagnosticOutput}
	if err := r.runner.Run(ctx, arguments, os.Environ(), nil, output, output); err != nil || output.failure != nil {
		return nil, credentialhost.ErrPupOutputLimit
	}
	return append([]byte(nil), output.buffer.Bytes()...), nil
}

func (r *Runtime) hashPupExecutable(ctx context.Context, container string) (string, error) {
	reader, writer := io.Pipe()
	runResult := make(chan error, 1)
	go func() {
		err := r.runner.Run(
			ctx, []string{"container", "cp", container + ":" + pupExecutablePath, "-"},
			os.Environ(), nil, writer, io.Discard,
		)
		_ = writer.CloseWithError(err)
		runResult <- err
	}()
	archive := tar.NewReader(reader)
	header, parseErr := archive.Next()
	if parseErr == nil && (header.Typeflag != tar.TypeReg || path.Base(header.Name) != "pup" ||
		header.Size <= 0 || header.Size > maxPupExecutableArchive || header.Mode&0o111 == 0) {
		parseErr = credentialhost.ErrInvalidExecutable
	}
	hasher := sha256.New()
	if parseErr == nil {
		_, parseErr = io.CopyN(hasher, archive, header.Size)
	}
	if parseErr == nil {
		if _, err := archive.Next(); !errors.Is(err, io.EOF) {
			parseErr = credentialhost.ErrInvalidExecutable
		}
	}
	if parseErr != nil {
		_ = reader.CloseWithError(parseErr)
	} else {
		_ = reader.Close()
	}
	runErr := <-runResult
	if parseErr != nil || runErr != nil {
		return "", credentialhost.ErrInvalidExecutable
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func randomPupLoginContainerName() (string, error) {
	return randomPupContainerName("login")
}

func randomPupContainerName(component string) (string, error) {
	var value [12]byte
	if _, err := io.ReadFull(rand.Reader, value[:]); err != nil {
		return "", err
	}
	return "tobari-pup-" + component + "-" + hex.EncodeToString(value[:]), nil
}

type pupBoundedBuffer struct {
	buffer  bytes.Buffer
	limit   int
	failure error
}

func (b *pupBoundedBuffer) Write(content []byte) (int, error) {
	if b.failure != nil {
		return 0, b.failure
	}
	if len(content) > b.limit-b.buffer.Len() {
		b.failure = credentialhost.ErrPupOutputLimit
		return 0, b.failure
	}
	return b.buffer.Write(content)
}
