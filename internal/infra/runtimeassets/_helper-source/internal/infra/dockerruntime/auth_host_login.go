package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/infra/credentialhost"
	"github.com/tasuku43/tobari/internal/infra/terminal"
	"github.com/tasuku43/tobari/internal/infra/terminalstyle"
)

const (
	awsHostDriverID         = credentialhost.SSODriverID
	awsConsoleDriverID      = credentialhost.ConsoleDriverID
	awsIdentityCenterMethod = "identity-center"
	awsConsoleMethod        = "console"
	maxAWSLoginFieldSize    = 1024
	maxHostCLICandidates    = 256
)

type reviewedHostLoginDriverKind uint8

const (
	reviewedHostLoginDriverGitHub reviewedHostLoginDriverKind = iota + 1
	reviewedHostLoginDriverAWS
	reviewedHostLoginDriverDatadog
	reviewedHostLoginDriverOpenAI
	reviewedHostLoginDriverAnthropic
)

type reviewedHostLoginDriver struct {
	providerID           string
	executable           string
	kind                 reviewedHostLoginDriverKind
	persistDriverDetails bool
}

// reviewedHostLoginDrivers returns the fixed compiled driver table in the
// public selector order. Returning a fresh value keeps callers from turning
// the closed driver union into runtime registration state.
func reviewedHostLoginDrivers() []reviewedHostLoginDriver {
	return []reviewedHostLoginDriver{
		{providerID: authbroker.BuiltinGitHubProviderID, executable: "gh", kind: reviewedHostLoginDriverGitHub},
		{providerID: authbroker.BuiltinAWSProviderID, executable: "aws", kind: reviewedHostLoginDriverAWS, persistDriverDetails: true},
		{providerID: authbroker.BuiltinDatadogProviderID, executable: "pup", kind: reviewedHostLoginDriverDatadog, persistDriverDetails: true},
		{providerID: authbroker.BuiltinOpenAIProviderID, executable: "codex", kind: reviewedHostLoginDriverOpenAI, persistDriverDetails: true},
		{providerID: authbroker.BuiltinAnthropicProviderID, executable: "claude", kind: reviewedHostLoginDriverAnthropic, persistDriverDetails: true},
	}
}

func reviewedHostLoginDriverForProvider(providerID string) (reviewedHostLoginDriver, bool) {
	for _, driver := range reviewedHostLoginDrivers() {
		if driver.providerID == providerID {
			return driver, true
		}
	}
	return reviewedHostLoginDriver{}, false
}

func reviewedHostLoginDriverForExecutable(executable string) (reviewedHostLoginDriver, bool) {
	for _, driver := range reviewedHostLoginDrivers() {
		if driver.executable == executable {
			return driver, true
		}
	}
	return reviewedHostLoginDriver{}, false
}

var (
	hostDriverRevisionPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hostAWSAccountPattern     = regexp.MustCompile(`^[0-9]{12}$`)

	errHostLoginPrompt      = errors.New("host credential login prompt failed")
	errHostCredentialResult = errors.New("host credential result is invalid")
)

type hostCLIUnavailableStage string

// These closed stage identifiers carry only which reviewed check rejected an
// invocation. Raw resolver and process errors never cross the public boundary.
const (
	hostCLIStageDriverDependency         hostCLIUnavailableStage = "driver_dependency"
	hostCLIStageExecutableLookup         hostCLIUnavailableStage = "executable_lookup"
	hostCLIStageExecutableSymlink        hostCLIUnavailableStage = "executable_symlink_resolution"
	hostCLIStageExecutableCanonicalPath  hostCLIUnavailableStage = "executable_canonical_path"
	hostCLIStageExecutableTrustedRoot    hostCLIUnavailableStage = "executable_trusted_root"
	hostCLIStageExecutableIdentity       hostCLIUnavailableStage = "executable_identity"
	hostCLIStageCodexChatGPTAppBundle    hostCLIUnavailableStage = "codex_chatgpt_app_bundle"
	hostCLIStageCodexExecutableIdentity  hostCLIUnavailableStage = "codex_executable_identity"
	hostCLIStageCodexVersionObservation  hostCLIUnavailableStage = "codex_version_observation"
	hostCLIStagePupContextSelection      hostCLIUnavailableStage = "pup_context_runtime_selection"
	hostCLIStagePupImageContract         hostCLIUnavailableStage = "pup_context_runtime_image_contract"
	hostCLIStagePupExecutableIdentity    hostCLIUnavailableStage = "pup_context_runtime_executable_identity"
	hostCLIStagePupVersionObservation    hostCLIUnavailableStage = "pup_context_runtime_version_observation"
	hostCLIStagePupCaptureContract       hostCLIUnavailableStage = "pup_context_runtime_capture_contract"
	hostCLIStagePupStateContract         hostCLIUnavailableStage = "pup_context_runtime_state_contract"
	hostCLIStageClaudeContextSelection   hostCLIUnavailableStage = "claude_context_runtime_selection"
	hostCLIStageClaudeImageContract      hostCLIUnavailableStage = "claude_context_runtime_image_contract"
	hostCLIStageClaudeExecutableIdentity hostCLIUnavailableStage = "claude_context_runtime_executable_identity"
	hostCLIStageClaudeVersionObservation hostCLIUnavailableStage = "claude_context_runtime_version_observation"
)

const chatGPTAppCodexExecutable = "/Applications/ChatGPT.app/Contents/Resources/codex"

type hostCLIUnavailableError struct {
	provider string
	stage    hostCLIUnavailableStage
}

func (hostCLIUnavailableError) Error() string { return "trusted host provider CLI is unavailable" }

type hostCLIResolver interface {
	Resolve(string) (string, error)
}

type pathHostCLIResolver struct {
	lookPath  func(string) (string, error)
	pathValue func() string
}

func newPathHostCLIResolver() pathHostCLIResolver {
	return pathHostCLIResolver{
		lookPath:  exec.LookPath,
		pathValue: func() string { return os.Getenv("PATH") },
	}
}

// Resolve accepts only the five source-reviewed driver names and an absolute,
// canonical, non-writable executable under a conventional host installation
// root. PATH may select among those installations, but a relative, project,
// home-local, or temporary directory cannot become provider authority.
func (r pathHostCLIResolver) Resolve(name string) (string, error) {
	if _, reviewed := reviewedHostLoginDriverForExecutable(name); r.lookPath == nil || !reviewed {
		return "", hostCLIUnavailableError{provider: name, stage: hostCLIStageDriverDependency}
	}
	selected, err := r.lookPath(name)
	if err != nil {
		return "", hostCLIUnavailableError{provider: name, stage: hostCLIStageExecutableLookup}
	}
	canonical, rejected := resolveHostCLICandidate(name, selected)
	if rejected == "" {
		return canonical, nil
	}
	if r.pathValue != nil {
		for _, candidate := range hostCLIPathCandidates(r.pathValue(), name) {
			if candidate == selected {
				continue
			}
			canonical, candidateRejected := resolveHostCLICandidate(name, candidate)
			if candidateRejected == "" {
				return canonical, nil
			}
		}
	}
	return "", hostCLIUnavailableError{provider: name, stage: rejected}
}

// resolveHostCLICandidate validates one PATH candidate without executing it.
// Only the canonical executable that passes every existing trust check is
// returned to the provider-specific driver.
func resolveHostCLICandidate(name, selected string) (string, hostCLIUnavailableStage) {
	if !filepath.IsAbs(selected) || filepath.Base(selected) != name {
		return "", hostCLIStageExecutableLookup
	}
	canonical, err := filepath.EvalSymlinks(selected)
	if err != nil {
		return "", hostCLIStageExecutableSymlink
	}
	if !filepath.IsAbs(canonical) || filepath.Clean(canonical) != canonical {
		return "", hostCLIStageExecutableCanonicalPath
	}
	if !trustedHostCLIPath(canonical) {
		stage := hostCLIStageExecutableTrustedRoot
		if name == "codex" && canonical == chatGPTAppCodexExecutable {
			stage = hostCLIStageCodexChatGPTAppBundle
		}
		return "", stage
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 {
		return "", hostCLIStageExecutableIdentity
	}
	return canonical, ""
}

// hostCLIPathCandidates returns a finite PATH-ordered set of absolute
// candidates. Empty and relative entries would name the current directory and
// cannot participate in trusted-host authentication.
func hostCLIPathCandidates(pathValue, name string) []string {
	if name == "" || filepath.Base(name) != name {
		return []string{}
	}
	candidates := make([]string, 0)
	seen := make(map[string]struct{})
	for _, directory := range filepath.SplitList(pathValue) {
		if len(candidates) >= maxHostCLICandidates {
			break
		}
		if directory == "" || !filepath.IsAbs(directory) {
			continue
		}
		candidate := filepath.Join(directory, name)
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}
	return candidates
}

// trustedHostCLIPath limits reviewed provider drivers to conventional
// installation roots outside project and temporary directories. PATH still
// selects among installations, but cannot make a repository-owned absolute
// directory executable authority.
func trustedHostCLIPath(path string) bool {
	for _, root := range []string{
		"/bin", "/usr/bin", "/usr/local", "/opt/homebrew", "/opt/local",
		"/nix/store", "/snap",
	} {
		relative, err := filepath.Rel(root, path)
		if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

type hostCredentialPayload struct {
	secret         []byte
	accountLabel   string
	driverID       string
	driverRevision string
}

func (hostCredentialPayload) String() string {
	return "dockerruntime.hostCredentialPayload{redacted}"
}

func (hostCredentialPayload) GoString() string {
	return "dockerruntime.hostCredentialPayload{redacted}"
}

func (p *hostCredentialPayload) clear() {
	if p == nil {
		return
	}
	clear(p.secret)
	p.secret = nil
	p.accountLabel = ""
	p.driverID = ""
	p.driverRevision = ""
}

type hostCredentialAcquirer interface {
	LoginGitHub(
		context.Context,
		string,
		credentialhost.GitHubLoginStreams,
	) (hostCredentialPayload, error)
	LoginAWS(
		context.Context,
		string,
		credentialhost.ProfileConfig,
		credentialhost.VisibleOutput,
	) (hostCredentialPayload, error)
}

type hostConsoleCredentialAcquirer interface {
	LoginAWSConsole(
		context.Context,
		string,
		credentialhost.ConsoleProfileConfig,
		io.Reader,
		credentialhost.VisibleOutput,
	) (hostCredentialPayload, error)
}

type hostCodexCredentialAcquirer interface {
	LoginCodex(
		context.Context,
		string,
		credentialhost.CodexLoginStreams,
	) (hostCredentialPayload, error)
}

// hostLoginProfileReader owns interactive, context-bounded collection of the
// non-secret AWS profile fields. Keeping this terminal capability narrower
// than the credential acquirer prevents the provider driver and Broker commit
// from starting before prompt input has completed.
type hostLoginProfileReader interface {
	ReadAWSProfile(
		context.Context,
		io.Reader,
		io.Writer,
	) (credentialhost.ProfileConfig, error)
}

type hostConsoleProfileReader interface {
	ReadAWSConsoleProfile(
		context.Context,
		io.Reader,
		io.Writer,
	) (credentialhost.ConsoleProfileConfig, error)
}

type osHostLoginProfileReader struct {
	waitInput func(context.Context, io.Reader) error
	openInput func(io.Reader) (*os.File, error)
}

func (r osHostLoginProfileReader) ReadAWSProfile(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
) (credentialhost.ProfileConfig, error) {
	if ctx == nil {
		return credentialhost.ProfileConfig{}, errHostLoginPrompt
	}
	if err := ctx.Err(); err != nil {
		return credentialhost.ProfileConfig{}, err
	}
	if !terminal.IsCanonical(input) {
		return credentialhost.ProfileConfig{}, errHostLoginPrompt
	}
	openInput := r.openInput
	if openInput == nil {
		openInput = openHostLoginInput
	}
	privateInput, err := openInput(input)
	if err != nil || !terminal.IsCanonical(privateInput) {
		if privateInput != nil {
			_ = privateInput.Close()
		}
		return credentialhost.ProfileConfig{}, errHostLoginPrompt
	}
	waitInput := r.waitInput
	if waitInput == nil {
		waitInput = waitHostLoginInput
	}
	profile, readErr := readAWSLoginProfile(
		ctx, privateInput, output,
		func(ctx context.Context, input io.Reader) error {
			if err := waitInput(ctx, input); err != nil {
				return err
			}
			if !terminal.IsCanonical(input) {
				return errHostLoginPrompt
			}
			return nil
		},
		readHostLoginInput,
	)
	closeErr := privateInput.Close()
	if closeErr != nil {
		if readErr != nil {
			return credentialhost.ProfileConfig{}, errors.Join(readErr, errHostLoginPrompt)
		}
		return credentialhost.ProfileConfig{}, errHostLoginPrompt
	}
	return profile, readErr
}

func (r osHostLoginProfileReader) ReadAWSConsoleProfile(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
) (credentialhost.ConsoleProfileConfig, error) {
	if ctx == nil {
		return credentialhost.ConsoleProfileConfig{}, errHostLoginPrompt
	}
	if err := ctx.Err(); err != nil {
		return credentialhost.ConsoleProfileConfig{}, err
	}
	if !terminal.IsCanonical(input) {
		return credentialhost.ConsoleProfileConfig{}, errHostLoginPrompt
	}
	openInput := r.openInput
	if openInput == nil {
		openInput = openHostLoginInput
	}
	privateInput, err := openInput(input)
	if err != nil || !terminal.IsCanonical(privateInput) {
		if privateInput != nil {
			_ = privateInput.Close()
		}
		return credentialhost.ConsoleProfileConfig{}, errHostLoginPrompt
	}
	waitInput := r.waitInput
	if waitInput == nil {
		waitInput = waitHostLoginInput
	}
	profile, readErr := readAWSConsoleLoginProfile(
		ctx, privateInput, output,
		func(ctx context.Context, input io.Reader) error {
			if err := waitInput(ctx, input); err != nil {
				return err
			}
			if !terminal.IsCanonical(input) {
				return errHostLoginPrompt
			}
			return nil
		},
		readHostLoginInput,
	)
	closeErr := privateInput.Close()
	if closeErr != nil {
		if readErr != nil {
			return credentialhost.ConsoleProfileConfig{}, errors.Join(readErr, errHostLoginPrompt)
		}
		return credentialhost.ConsoleProfileConfig{}, errHostLoginPrompt
	}
	return profile, readErr
}

type osHostCredentialAcquirer struct {
	github *credentialhost.GitHubDriver
	aws    *credentialhost.Driver
	codex  *credentialhost.CodexDriver
}

func newOSHostCredentialAcquirer() *osHostCredentialAcquirer {
	return &osHostCredentialAcquirer{
		github: credentialhost.NewGitHubDriver(nil),
		aws:    credentialhost.NewDriver(nil),
		codex:  credentialhost.NewCodexDriver(nil),
	}
}

func (a *osHostCredentialAcquirer) LoginGitHub(
	ctx context.Context,
	executable string,
	streams credentialhost.GitHubLoginStreams,
) (hostCredentialPayload, error) {
	if a == nil || a.github == nil {
		return hostCredentialPayload{}, credentialhost.ErrGitHubLoginSetup
	}
	credential, err := a.github.Login(ctx, executable, streams)
	if err != nil {
		return hostCredentialPayload{}, err
	}
	defer credential.Clear()
	return hostCredentialPayload{
		secret:       credential.Token(),
		accountLabel: credential.AccountLabel(),
	}, nil
}

func (a *osHostCredentialAcquirer) LoginAWS(
	ctx context.Context,
	executable string,
	profile credentialhost.ProfileConfig,
	visible credentialhost.VisibleOutput,
) (hostCredentialPayload, error) {
	if a == nil || a.aws == nil {
		return hostCredentialPayload{}, credentialhost.ErrCommandFailed
	}
	state, err := a.aws.Login(ctx, executable, profile, visible)
	if err != nil {
		return hostCredentialPayload{}, err
	}
	defer state.Clear()
	encoded, err := state.Encode()
	if err != nil {
		return hostCredentialPayload{}, err
	}
	return hostCredentialPayload{
		secret:         encoded,
		accountLabel:   state.AccountID(),
		driverID:       state.DriverID(),
		driverRevision: state.DriverRevision(),
	}, nil
}

func (a *osHostCredentialAcquirer) LoginAWSConsole(
	ctx context.Context,
	executable string,
	profile credentialhost.ConsoleProfileConfig,
	input io.Reader,
	visible credentialhost.VisibleOutput,
) (hostCredentialPayload, error) {
	if a == nil || a.aws == nil {
		return hostCredentialPayload{}, credentialhost.ErrCommandFailed
	}
	state, err := a.aws.ConsoleLogin(ctx, executable, profile, input, visible)
	if err != nil {
		return hostCredentialPayload{}, err
	}
	defer state.Clear()
	encoded, err := state.Encode()
	if err != nil {
		return hostCredentialPayload{}, err
	}
	return hostCredentialPayload{
		secret: encoded, accountLabel: state.AccountID(), driverID: state.DriverID(),
		driverRevision: state.DriverRevision(),
	}, nil
}

func (a *osHostCredentialAcquirer) LoginCodex(
	ctx context.Context,
	executable string,
	streams credentialhost.CodexLoginStreams,
) (hostCredentialPayload, error) {
	if a == nil || a.codex == nil {
		return hostCredentialPayload{}, credentialhost.ErrCodexLoginSetup
	}
	state, err := a.codex.Login(ctx, executable, streams)
	if err != nil {
		return hostCredentialPayload{}, err
	}
	defer state.Clear()
	encoded, err := state.Encode()
	if err != nil {
		return hostCredentialPayload{}, err
	}
	return hostCredentialPayload{
		secret: encoded, accountLabel: state.AccountLabel(), driverID: state.DriverID(),
		driverRevision: state.DriverRevision(),
	}, nil
}

func (r *Runtime) runHostCredentialLogin(
	ctx context.Context,
	contextID string,
	provider string,
	input io.Reader,
	errOut io.Writer,
	methods ...string,
) (brokerControlResponse, error) {
	if !r.IsInputTerminal(input) || !r.IsTerminal(errOut) {
		return brokerControlResponse{}, authLoginTerminalRequiredFault()
	}
	return r.runHostCredentialLoginOnTTY(ctx, contextID, provider, input, errOut, methods...)
}

func (r *Runtime) runHostCredentialLoginOnTTY(
	ctx context.Context,
	contextID string,
	provider string,
	input io.Reader,
	errOut io.Writer,
	methods ...string,
) (brokerControlResponse, error) {
	method := ""
	if len(methods) > 1 {
		return brokerControlResponse{}, credentialhost.ErrInvalidProfile
	}
	if len(methods) == 1 {
		method = methods[0]
	}
	if provider == authbroker.BuiltinAWSProviderID && method == "" {
		method = awsIdentityCenterMethod
	}
	driver, reviewed := reviewedHostLoginDriverForProvider(provider)
	if !reviewed {
		return brokerControlResponse{}, hostCLIUnavailableError{provider: provider, stage: hostCLIStageDriverDependency}
	}
	usesContainerAcquisition := driver.kind == reviewedHostLoginDriverDatadog || driver.kind == reviewedHostLoginDriverAnthropic
	if !usesContainerAcquisition && (r.credentialHost == nil || r.hostCLIs == nil) {
		return brokerControlResponse{}, hostCLIUnavailableError{provider: provider, stage: hostCLIStageDriverDependency}
	}

	loginContext, cancel := context.WithTimeout(ctx, brokerLoginTimeout)
	defer cancel()
	executable := ""
	var err error
	if !usesContainerAcquisition {
		executable, err = r.hostCLIs.Resolve(driver.executable)
	}
	if err != nil {
		var unavailable hostCLIUnavailableError
		if errors.As(err, &unavailable) {
			unavailable.provider = provider
			return brokerControlResponse{}, unavailable
		}
		return brokerControlResponse{}, hostCLIUnavailableError{provider: provider, stage: hostCLIStageExecutableLookup}
	}

	visible := &loginVisibleOutput{
		destination: errOut,
		openBrowser: r.loginBrowserOpener(loginContext),
		provider:    provider,
		color:       !terminalstyle.NoColorRequested(),
	}
	var payload hostCredentialPayload
	switch driver.kind {
	case reviewedHostLoginDriverGitHub:
		payload, err = r.credentialHost.LoginGitHub(
			loginContext,
			executable,
			credentialhost.GitHubLoginStreams{
				Stdin: input, Stdout: visible, Stderr: visible,
			},
		)
	case reviewedHostLoginDriverAWS:
		var profile credentialhost.ProfileConfig
		if r.hostLoginProfiles == nil {
			return brokerControlResponse{}, hostCLIUnavailableError{provider: provider, stage: hostCLIStageDriverDependency}
		}
		switch method {
		case awsIdentityCenterMethod:
			profile, err = r.hostLoginProfiles.ReadAWSProfile(loginContext, input, errOut)
		case awsConsoleMethod:
			profileReader, ok := r.hostLoginProfiles.(hostConsoleProfileReader)
			acquirer, acquirerOK := r.credentialHost.(hostConsoleCredentialAcquirer)
			if !ok || !acquirerOK {
				return brokerControlResponse{}, hostCLIUnavailableError{provider: provider, stage: hostCLIStageDriverDependency}
			}
			var consoleProfile credentialhost.ConsoleProfileConfig
			consoleProfile, err = profileReader.ReadAWSConsoleProfile(loginContext, input, errOut)
			if err == nil {
				visible.consoleRegion = consoleProfile.Region
			}
			if err == nil {
				err = loginContext.Err()
			}
			if err == nil {
				payload, err = acquirer.LoginAWSConsole(
					loginContext, executable, consoleProfile, input,
					func(_ credentialhost.OutputStream, content []byte) error {
						_, writeErr := visible.Write(content)
						return writeErr
					},
				)
			}
		default:
			err = credentialhost.ErrInvalidProfile
		}
		if err == nil {
			err = loginContext.Err()
		}
		if err == nil && method == awsIdentityCenterMethod {
			payload, err = r.credentialHost.LoginAWS(
				loginContext,
				executable,
				profile,
				func(_ credentialhost.OutputStream, content []byte) error {
					_, writeErr := visible.Write(content)
					return writeErr
				},
			)
		}
	case reviewedHostLoginDriverDatadog:
		if r.pupContainerLogin != nil {
			payload, err = r.pupContainerLogin(loginContext, contextID, input, visible)
		} else {
			payload, err = r.loginPupInContextContainer(loginContext, contextID, input, visible)
		}
	case reviewedHostLoginDriverOpenAI:
		acquirer, ok := r.credentialHost.(hostCodexCredentialAcquirer)
		if !ok {
			return brokerControlResponse{}, hostCLIUnavailableError{provider: provider, stage: hostCLIStageDriverDependency}
		}
		payload, err = acquirer.LoginCodex(
			loginContext, executable,
			credentialhost.CodexLoginStreams{
				Stdin: input, Stdout: visible, Stderr: visible,
			},
		)
	case reviewedHostLoginDriverAnthropic:
		if r.claudeContainerLogin != nil {
			payload, err = r.claudeContainerLogin(loginContext, contextID, input, visible)
		} else {
			payload, err = r.loginClaudeInContextContainer(loginContext, contextID, input, visible)
		}
	default:
		return brokerControlResponse{}, hostCLIUnavailableError{provider: provider, stage: hostCLIStageDriverDependency}
	}
	flushErr := visible.flush()
	if err != nil {
		payload.clear()
		return brokerControlResponse{}, err
	}
	if flushErr != nil {
		payload.clear()
		return brokerControlResponse{}, flushErr
	}
	defer payload.clear()
	if err := validateHostCredentialPayload(provider, payload); err != nil {
		return brokerControlResponse{}, err
	}

	arguments := []string{
		"login",
		"--manifest-id", contextID,
		"--provider", provider,
		"--account-label", payload.accountLabel,
	}
	if driver.persistDriverDetails {
		arguments = append(
			arguments,
			"--driver-id", payload.driverID,
			"--driver-revision", payload.driverRevision,
		)
	}
	return r.runBrokerControl(loginContext, bytes.NewReader(payload.secret), arguments...)
}

func providerHostExecutable(provider string) string {
	driver, reviewed := reviewedHostLoginDriverForProvider(provider)
	if !reviewed {
		return ""
	}
	return driver.executable
}

func (r *Runtime) loginBrowserOpener(ctx context.Context) func(string) error {
	opener := r.browser
	if opener == nil {
		opener = osHostBrowserOpener{}
	}
	return func(target string) error {
		openContext, cancel := context.WithTimeout(ctx, hostBrowserOpenTimeout)
		defer cancel()
		return opener.Open(openContext, target)
	}
}

func readAWSLoginProfile(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	waitInput func(context.Context, io.Reader) error,
	readInput func(io.Reader, []byte) (int, error),
) (credentialhost.ProfileConfig, error) {
	values, err := readAWSLoginFields(ctx, input, output, waitInput, readInput, []string{
		"AWS IAM Identity Center access portal URL",
		"AWS IAM Identity Center region",
		"AWS account ID",
		"AWS role name",
	})
	if err != nil {
		return credentialhost.ProfileConfig{}, err
	}
	return credentialhost.ProfileConfig{
		StartURL: values[0], SSORegion: values[1],
		AccountID: values[2], RoleName: values[3],
	}, nil
}

func readAWSConsoleLoginProfile(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	waitInput func(context.Context, io.Reader) error,
	readInput func(io.Reader, []byte) (int, error),
) (credentialhost.ConsoleProfileConfig, error) {
	values, err := readAWSLoginFields(
		ctx, input, output, waitInput, readInput, []string{"AWS region for console login"},
	)
	if err != nil {
		return credentialhost.ConsoleProfileConfig{}, err
	}
	return credentialhost.ConsoleProfileConfig{Region: values[0]}, nil
}

func readAWSLoginFields(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	waitInput func(context.Context, io.Reader) error,
	readInput func(io.Reader, []byte) (int, error),
	prompts []string,
) ([]string, error) {
	if ctx == nil || input == nil || output == nil || waitInput == nil || readInput == nil {
		return nil, errHostLoginPrompt
	}
	if len(prompts) == 0 || len(prompts) > 4 {
		return nil, errHostLoginPrompt
	}
	maxBufferedProfileSize := len(prompts) * (maxAWSLoginFieldSize + 2)
	pending := make([]byte, 0, maxBufferedProfileSize)
	available := make([]byte, maxBufferedProfileSize)
	inputEnded := false
	values := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := fmt.Fprintf(output, "%s: ", prompt); err != nil {
			return nil, errHostLoginPrompt
		}
		for bytes.IndexByte(pending, '\n') < 0 {
			if len(pending) >= maxAWSLoginFieldSize+2 || inputEnded {
				return nil, errHostLoginPrompt
			}
			if err := waitInput(ctx, input); err != nil {
				if contextErr := ctx.Err(); contextErr != nil {
					return nil, contextErr
				}
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, err
				}
				return nil, errHostLoginPrompt
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			// Production readInput is one nonblocking read on the private
			// terminal description. A readiness flush or partial line therefore
			// returns to this bounded loop instead of entering a hidden second
			// read.
			count, readErr := readInput(input, available[:maxBufferedProfileSize-len(pending)])
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if count > 0 {
				pending = append(pending, available[:count]...)
			}
			if readErr != nil {
				if errors.Is(readErr, syscall.EAGAIN) || errors.Is(readErr, syscall.EWOULDBLOCK) {
					continue
				}
				if !errors.Is(readErr, io.EOF) {
					return nil, errHostLoginPrompt
				}
				inputEnded = true
			}
			if count == 0 && readErr == nil {
				return nil, errHostLoginPrompt
			}
		}
		lineEnd := bytes.IndexByte(pending, '\n')
		line := pending[:lineEnd+1]
		if len(line) > maxAWSLoginFieldSize+2 {
			return nil, errHostLoginPrompt
		}
		pending = pending[lineEnd+1:]
		line = bytes.TrimSuffix(line, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 || len(line) > maxAWSLoginFieldSize || bytes.IndexByte(line, 0) >= 0 {
			return nil, credentialhost.ErrInvalidProfile
		}
		values = append(values, string(line))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func validateHostCredentialPayload(provider string, payload hostCredentialPayload) error {
	if len(payload.secret) == 0 || len(payload.secret) > 32*1024 ||
		strings.IndexByte(payload.accountLabel, 0) >= 0 {
		return errHostCredentialResult
	}
	driver, reviewed := reviewedHostLoginDriverForProvider(provider)
	if !reviewed {
		return errHostCredentialResult
	}
	switch driver.kind {
	case reviewedHostLoginDriverGitHub:
		if payload.accountLabel == "" || payload.driverID != "" || payload.driverRevision != "" {
			return errHostCredentialResult
		}
	case reviewedHostLoginDriverAWS:
		if !hostAWSAccountPattern.MatchString(payload.accountLabel) ||
			(payload.driverID != awsHostDriverID && payload.driverID != awsConsoleDriverID) ||
			!hostDriverRevisionPattern.MatchString(payload.driverRevision) {
			return errHostCredentialResult
		}
	case reviewedHostLoginDriverDatadog:
		if payload.accountLabel != credentialhost.PupAccountLabel || payload.driverID != credentialhost.PupDriverID ||
			!hostDriverRevisionPattern.MatchString(payload.driverRevision) {
			return errHostCredentialResult
		}
	case reviewedHostLoginDriverOpenAI:
		if payload.accountLabel == "" ||
			authbroker.ValidateSecretFreeText("account label", payload.accountLabel, 128) != nil ||
			payload.driverID != credentialhost.CodexDriverID ||
			!hostDriverRevisionPattern.MatchString(payload.driverRevision) {
			return errHostCredentialResult
		}
	case reviewedHostLoginDriverAnthropic:
		if payload.accountLabel != credentialhost.ClaudeNativeAccountLabel ||
			payload.driverID != credentialhost.ClaudeNativeDriverID ||
			!hostDriverRevisionPattern.MatchString(payload.driverRevision) {
			return errHostCredentialResult
		}
	default:
		return errHostCredentialResult
	}
	return nil
}

func hostLoginTimedOut(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) ||
		errors.Is(err, credentialhost.ErrCodexLoginTimeout)
}

func hostLoginCancelled(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, credentialhost.ErrGitHubLoginCancelled) ||
		errors.Is(err, credentialhost.ErrCodexLoginCancelled) ||
		errors.Is(err, credentialhost.ErrClaudeLoginCancelled)
}

func hostLoginFailureIsCredentialDriver(err error) bool {
	return errors.Is(err, credentialhost.ErrGitHubExecutable) ||
		errors.Is(err, credentialhost.ErrGitHubLoginSetup) ||
		errors.Is(err, credentialhost.ErrGitHubTTYRequired) ||
		errors.Is(err, credentialhost.ErrGitHubLoginFailed) ||
		errors.Is(err, credentialhost.ErrGitHubAccountCapture) ||
		errors.Is(err, credentialhost.ErrGitHubTokenCapture) ||
		errors.Is(err, credentialhost.ErrGitHubOutputLimit) ||
		errors.Is(err, credentialhost.ErrGitHubLoginCleanup) ||
		errors.Is(err, credentialhost.ErrCommandFailed) ||
		errors.Is(err, credentialhost.ErrOutputLimit) ||
		errors.Is(err, credentialhost.ErrVisibleOutput) ||
		errors.Is(err, credentialhost.ErrInvalidCredentials) ||
		errors.Is(err, credentialhost.ErrInvalidExecutable) ||
		errors.Is(err, credentialhost.ErrInvalidState) ||
		errors.Is(err, credentialhost.ErrInvalidCache) ||
		errors.Is(err, credentialhost.ErrPupLoginFailed) ||
		errors.Is(err, credentialhost.ErrPupLoginSetup) ||
		errors.Is(err, credentialhost.ErrPupLoginCleanup) ||
		errors.Is(err, credentialhost.ErrPupOutputLimit) ||
		errors.Is(err, credentialhost.ErrInvalidPupState) ||
		errors.Is(err, credentialhost.ErrCodexExecutable) ||
		errors.Is(err, credentialhost.ErrCodexVersion) ||
		errors.Is(err, credentialhost.ErrCodexLoginSetup) ||
		errors.Is(err, credentialhost.ErrCodexLoginStreams) ||
		errors.Is(err, credentialhost.ErrCodexLoginFailed) ||
		errors.Is(err, credentialhost.ErrCodexOutputLimit) ||
		errors.Is(err, credentialhost.ErrCodexVisibleOutput) ||
		errors.Is(err, credentialhost.ErrCodexAuthCapture) ||
		errors.Is(err, credentialhost.ErrCodexLoginCleanup) ||
		errors.Is(err, credentialhost.ErrInvalidCodexCredential) ||
		errors.Is(err, credentialhost.ErrClaudeExecutable) ||
		errors.Is(err, credentialhost.ErrClaudeVersion) ||
		errors.Is(err, credentialhost.ErrClaudeLoginSetup) ||
		errors.Is(err, credentialhost.ErrClaudeLoginFailed) ||
		errors.Is(err, credentialhost.ErrClaudeOutputLimit) ||
		errors.Is(err, credentialhost.ErrClaudeTokenCapture) ||
		errors.Is(err, credentialhost.ErrClaudeLoginCleanup) ||
		errors.Is(err, errLoginVisibleOutputLimit) ||
		errors.Is(err, errHostLoginPrompt) ||
		errors.Is(err, errHostCredentialResult)
}
