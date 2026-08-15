package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

const (
	githubEphemeralPlaintextWarning = "! Authentication credentials saved in plain text"
	githubManualBrowserFallback     = "The host browser did not open; visit " + githubDeviceURL + " manually to continue.\n"
	loginBrowserOpenedFeedback      = "↗ Opened in your default browser.\n"
	awsBrowserLinePrefix            = "Open "
	maxLoginVisibleLine             = 64 * 1024
	maxLoginVisibleBytes            = 64 * 1024
	hostBrowserOpenTimeout          = 5 * time.Second
	loginSGRReset                   = "\x1b[0m"
	loginSGRUpstreamMuted           = "\x1b[90m"
	loginSGRUpstreamAccent          = "\x1b[94m"
	loginStyleMuted                 = "\x1b[38;5;250m"
	loginStyleAccent                = "\x1b[1;38;5;45m"
	loginStyleSuccess               = "\x1b[38;5;42m"
	claudeLoginOpening              = "Opening browser to sign in…"
	claudeLoginPastePrompt          = "Paste code here if prompted > "
	claudeLoginBrowserOpened        = "↗ Opened in your default browser."
	claudeLoginTTYLineEnding        = "\r\n"
	claudeLoginOpeningFeedback      = "Opening Claude sign-in…" + claudeLoginTTYLineEnding
	claudeLoginOpenedFeedback       = "✓ Opened in your default browser." + claudeLoginTTYLineEnding
	claudeLoginPromptFeedback       = claudeLoginTTYLineEnding + "If Claude shows a code, paste it here:" + claudeLoginTTYLineEnding + "> "
	claudeLoginURLPrefix            = "https://claude.com/cai/oauth/authorize?"
	claudeLoginClientID             = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	claudeLoginRedirectURI          = "https://platform.claude.com/oauth/code/callback"
	claudeLoginScopes               = "org:create_api_key user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	claudeHyperlinkPrefix           = "If the browser didn't open, visit: \x1b]8;;"
	claudeHyperlinkClose            = "\x1b]8;;\a"
)

var errLoginVisibleOutputLimit = errors.New("host login visible output exceeded its limit")

var claudeOAuthOpaquePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

type loginVisibleOutput struct {
	mu                     sync.Mutex
	destination            io.Writer
	openBrowser            func(string) error
	provider               string
	consoleRegion          string
	pending                []byte
	opened                 bool
	claudePrompted         bool
	claudePromptLineClosed bool
	color                  bool
	written                int
	visible                int
	failure                error
}

func (w *loginVisibleOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failure != nil {
		return 0, w.failure
	}
	remaining := maxLoginVisibleBytes - w.written
	if remaining <= 0 {
		w.failure = errLoginVisibleOutputLimit
		return 0, w.failure
	}
	if len(data) > remaining {
		written, err := w.write(data[:remaining])
		w.written += written
		if err != nil {
			w.failure = err
			return written, err
		}
		w.failure = errLoginVisibleOutputLimit
		return written, w.failure
	}
	written, err := w.write(data)
	w.written += written
	if err != nil {
		w.failure = err
	}
	return written, err
}

func (w *loginVisibleOutput) write(data []byte) (int, error) {
	for index, value := range data {
		w.pending = append(w.pending, value)
		if len(w.pending) > maxLoginVisibleLine {
			w.pending = w.pending[:len(w.pending)-1]
			return index, errLoginVisibleOutputLimit
		}
		if value == '\n' {
			if err := w.flushPending(); err != nil {
				return index + 1, err
			}
		} else if w.provider == authbroker.BuiltinAnthropicProviderID && claudePastePromptPending(w.pending) {
			w.pending = nil
			if err := w.writeClaudePastePrompt(); err != nil {
				return index + 1, err
			}
		}
	}
	return len(data), nil
}

func (w *loginVisibleOutput) flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failure != nil {
		return w.failure
	}
	if err := w.flushPending(); err != nil {
		w.failure = err
		return err
	}
	return nil
}

func (w *loginVisibleOutput) flushPending() error {
	if len(w.pending) == 0 {
		return nil
	}
	line := append([]byte(nil), w.pending...)
	w.pending = nil
	hasNewline := bytes.HasSuffix(line, []byte{'\n'})
	if hasNewline {
		line = line[:len(line)-1]
	}
	line = bytes.TrimSuffix(line, []byte{'\r'})
	normalized := string(line)
	if w.provider == authbroker.BuiltinAnthropicProviderID {
		handled, err := w.flushClaudeLoginLine(normalized)
		if handled || err != nil {
			return err
		}
	}
	if projected, ok := projectClaudeLoginHyperlink(normalized); ok {
		normalized = projected
	}
	if normalized == githubEphemeralPlaintextWarning {
		return nil
	}
	visible := projectLoginVisibleText(normalized, w.color)
	if hasNewline {
		if w.provider == authbroker.BuiltinAnthropicProviderID {
			visible += claudeLoginTTYLineEnding
		} else {
			visible += "\n"
		}
	}
	if err := w.writeVisible(visible); err != nil {
		return err
	}
	target, recognized := loginBrowserTarget(normalized, w.consoleRegion)
	if !w.opened && recognized && w.openBrowser != nil {
		w.opened = true
		if err := w.openBrowser(target); err != nil {
			return w.writeVisible(manualBrowserFallback(target))
		}
		return w.writeVisible(loginBrowserOpenedText(w.color))
	}
	return nil
}

func (w *loginVisibleOutput) flushClaudeLoginLine(line string) (bool, error) {
	semantic := line
	if index := strings.LastIndexByte(semantic, '\r'); index >= 0 {
		semantic = semantic[index+1:]
	}
	trimmed := strings.Trim(semantic, " ")
	if w.claudePrompted && !w.claudePromptLineClosed && trimmed != strings.TrimSpace(claudeLoginPastePrompt) {
		w.claudePromptLineClosed = true
		if err := w.writeVisible(claudeLoginTTYLineEnding); err != nil {
			return true, err
		}
	}
	switch trimmed {
	case "":
		return true, nil
	case claudeLoginOpening:
		return true, w.writeVisible(claudeLoginOpeningFeedback)
	case strings.TrimSpace(claudeLoginPastePrompt):
		return true, w.writeClaudePastePrompt()
	case claudeLoginBrowserOpened:
		// The reviewed browser result is Tobari's exact opener result below, not
		// the container's cursor-oriented claim about its own environment.
		return true, nil
	}
	projected, ok := projectClaudeLoginHyperlink(strings.TrimLeft(semantic, " "))
	if !ok {
		return false, nil
	}
	target := strings.TrimPrefix(projected, "If the browser didn't open, visit: ")
	if w.opened {
		return true, nil
	}
	w.opened = true
	if w.openBrowser == nil {
		return true, w.writeVisible(claudeManualBrowserFallback(target))
	}
	if err := w.openBrowser(target); err != nil {
		return true, w.writeVisible(claudeManualBrowserFallback(target))
	}
	return true, w.writeVisible(claudeLoginOpenedText(w.color))
}

func (w *loginVisibleOutput) writeClaudePastePrompt() error {
	if w.claudePrompted {
		return nil
	}
	w.claudePrompted = true
	return w.writeVisible(claudeLoginPromptFeedback)
}

func claudePastePromptPending(pending []byte) bool {
	for len(pending) > 0 && (pending[0] == ' ' || pending[0] == '\r') {
		pending = pending[1:]
	}
	return bytes.Equal(pending, []byte(claudeLoginPastePrompt))
}

func (w *loginVisibleOutput) writeVisible(value string) error {
	if len(value) > maxLoginVisibleBytes-w.visible {
		return errLoginVisibleOutputLimit
	}
	written, err := io.WriteString(w.destination, value)
	w.visible += written
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}

func projectLoginVisibleText(value string, color bool) string {
	var output strings.Builder
	styled := false
	for len(value) > 0 {
		switch {
		case strings.HasPrefix(value, loginSGRReset):
			if color && styled {
				output.WriteString(loginSGRReset)
			}
			styled = false
			value = value[len(loginSGRReset):]
			continue
		case strings.HasPrefix(value, loginSGRUpstreamMuted):
			if color {
				output.WriteString(loginStyleMuted)
				styled = true
			}
			value = value[len(loginSGRUpstreamMuted):]
			continue
		case strings.HasPrefix(value, loginSGRUpstreamAccent):
			if color {
				output.WriteString(loginStyleAccent)
				styled = true
			}
			value = value[len(loginSGRUpstreamAccent):]
			continue
		}
		character, size := utf8.DecodeRuneInString(value)
		value = value[size:]
		switch {
		case character == '\\':
			output.WriteString(`\\`)
		case character == '\u2028' || character == '\u2029' || unicode.Is(unicode.C, character):
			if character <= 0xffff {
				_, _ = fmt.Fprintf(&output, `\u%04X`, character)
			} else {
				_, _ = fmt.Fprintf(&output, `\U%08X`, character)
			}
		default:
			output.WriteRune(character)
		}
	}
	if color && styled {
		output.WriteString(loginSGRReset)
	}
	return output.String()
}

func loginBrowserTarget(line, consoleRegion string) (string, bool) {
	line = stripApprovedLoginSGR(line)
	if strings.Contains(line, githubDeviceURL) {
		return githubDeviceURL, true
	}
	if awsSSODeviceURLPattern.MatchString(line) {
		return line, true
	}
	if consoleRegion != "" && validAWSConsoleAuthorizationURL(line, consoleRegion) {
		return line, true
	}
	if index := strings.Index(line, claudeLoginURLPrefix); index >= 0 {
		target := strings.TrimSpace(line[index:])
		if validClaudeLoginAuthorizationURL(target) {
			return target, true
		}
	}
	if !strings.HasPrefix(line, awsBrowserLinePrefix) {
		return "", false
	}
	target := strings.TrimPrefix(line, awsBrowserLinePrefix)
	if !awsSSODeviceURLPattern.MatchString(target) {
		return "", false
	}
	return target, true
}

func projectClaudeLoginHyperlink(line string) (string, bool) {
	if !strings.HasPrefix(line, claudeHyperlinkPrefix) || !strings.HasSuffix(line, claudeHyperlinkClose) {
		return "", false
	}
	rest := strings.TrimPrefix(line, claudeHyperlinkPrefix)
	openEnd := strings.IndexByte(rest, '\a')
	if openEnd <= 0 {
		return "", false
	}
	target := rest[:openEnd]
	labelAndClose := rest[openEnd+1:]
	label := strings.TrimSuffix(labelAndClose, claudeHyperlinkClose)
	if target != label || !validClaudeLoginAuthorizationURL(target) {
		return "", false
	}
	return "If the browser didn't open, visit: " + target, true
}

func validClaudeLoginAuthorizationURL(target string) bool {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "claude.com" || parsed.User != nil ||
		parsed.Path != "/cai/oauth/authorize" || parsed.RawPath != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.RawFragment != "" {
		return false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(query) != 8 {
		return false
	}
	want := map[string]string{
		"code": "true", "client_id": claudeLoginClientID, "response_type": "code",
		"redirect_uri": claudeLoginRedirectURI, "scope": claudeLoginScopes,
		"code_challenge_method": "S256",
	}
	for key, value := range want {
		if len(query[key]) != 1 || query.Get(key) != value {
			return false
		}
	}
	for _, key := range []string{"code_challenge", "state"} {
		if len(query[key]) != 1 || !claudeOAuthOpaquePattern.MatchString(query.Get(key)) {
			return false
		}
	}
	return true
}

func stripApprovedLoginSGR(value string) string {
	return strings.NewReplacer(
		loginSGRReset, "",
		loginSGRUpstreamMuted, "",
		loginSGRUpstreamAccent, "",
	).Replace(value)
}

func loginBrowserOpenedText(color bool) string {
	if !color {
		return loginBrowserOpenedFeedback
	}
	return loginStyleSuccess + loginBrowserOpenedFeedback + loginSGRReset
}

func claudeLoginOpenedText(color bool) string {
	if !color {
		return claudeLoginOpenedFeedback
	}
	return loginStyleSuccess + claudeLoginOpenedFeedback + loginSGRReset
}

func claudeManualBrowserFallback(target string) string {
	return "! Browser did not open." + claudeLoginTTYLineEnding + "Visit: " + target + claudeLoginTTYLineEnding
}

func manualBrowserFallback(target string) string {
	if target == githubDeviceURL {
		return githubManualBrowserFallback
	}
	return fmt.Sprintf("The host browser did not open; visit %s manually to continue.\n", target)
}

func (r *Runtime) LoginAuth(
	ctx context.Context, contextName, providerID, method string, input io.Reader, errOut io.Writer,
) (authbroker.MutationObservation, error) {
	manifest, provider, err := r.authOperationTarget(ctx, contextName, providerID)
	if err != nil {
		return authbroker.MutationObservation{}, err
	}
	if !supportsBuiltinAuthHelper(provider) {
		return authbroker.MutationObservation{}, fault.New(
			fault.KindUnsupported, "provider_login_unsupported",
			"The selected provider does not support interactive login.", false,
			fault.NextAction{Command: "help auth import", Reason: "Use protected stdin import for a compatible user provider."},
		)
	}
	if !r.IsInputTerminal(input) || !r.IsTerminal(errOut) {
		return authbroker.MutationObservation{}, authLoginTerminalRequiredFault()
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.MutationObservation{}, classifyRootKeyError(err)
	}
	if err := r.requireAuthBroker(ctx); err != nil {
		return authbroker.MutationObservation{}, err
	}
	response, err := r.runHostCredentialLogin(ctx, manifest.ID, provider.ID, input, errOut, method)
	if err != nil {
		return authbroker.MutationObservation{}, classifyHostLoginError(err, provider.ID, method)
	}
	return r.buildAuthMutationObservation(ctx, authbroker.TaskLogin, manifest.Name, manifest.ID, provider.ID, response, true, true, backend)
}

func authLoginTerminalRequiredFault() error {
	return fault.New(
		fault.KindInvalidInput,
		"auth_login_tty_required",
		"Built-in provider login requires interactive terminal streams on stdin and stderr.",
		false,
		fault.NextAction{Command: "help auth login", Reason: "Run trusted-host provider login from an interactive terminal."},
	)
}

func supportsBuiltinAuthHelper(provider authbroker.Provider) bool {
	if provider.Acquisition.Mode != authbroker.AcquisitionBuiltinHelper {
		return false
	}
	expectedHelper, reviewedProvider := authbroker.ReviewedLoginProviderHelper(provider.ID)
	_, compiledDriver := reviewedHostLoginDriverForProvider(provider.ID)
	return reviewedProvider && compiledDriver && provider.Acquisition.Helper == expectedHelper
}

func classifyHostLoginError(err error, provider string, methods ...string) error {
	method := ""
	if len(methods) == 1 {
		method = methods[0]
	}
	if public, ok := fault.PublicCopy(err); ok {
		return public
	}
	var unavailable hostCLIUnavailableError
	if errors.As(err, &unavailable) {
		code := provider + "_cli_unavailable"
		name := provider
		if provider == "github" {
			name = "GitHub"
		} else if provider == "aws" {
			name = "AWS"
		} else if provider == "datadog" {
			name = "Datadog pup"
		} else if provider == "openai" {
			name = "Codex"
		} else if provider == "anthropic" {
			return fault.New(
				fault.KindUnavailable, code,
				"The selected Context runtime cannot provide the reviewed Claude Code login contract at diagnostic stage "+string(normalizeHostCLIUnavailableStage(unavailable.stage))+"; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "help runtime", Reason: "Install Claude Code 2.1.220 in the selected Context runtime, build it, and retry login."},
			)
		}
		return fault.New(
			fault.KindUnavailable, code,
			"The trusted-host "+name+" CLI is unavailable at diagnostic stage "+string(normalizeHostCLIUnavailableStage(unavailable.stage))+"; the previous Context credential remains unchanged.", false,
			fault.NextAction{Command: "auth login", Reason: "Install the reviewed host CLI and retry this login."},
		)
	}
	if provider == "github" {
		if hostLoginCancelled(err) {
			return fault.New(
				fault.KindRejected, "github_login_cancelled",
				"GitHub login was cancelled; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host GitHub login when ready."},
			)
		}
		if errors.Is(err, credentialhost.ErrGitHubExecutable) {
			return classifyHostLoginError(hostCLIUnavailableError{provider: provider, stage: hostCLIStageExecutableIdentity}, provider, method)
		}
		if hostLoginFailureIsCredentialDriver(err) {
			return fault.New(
				fault.KindRejected, "github_login_failed",
				"GitHub login did not complete; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host GitHub login after inspecting the failure."},
			)
		}
		return classifyBrokerError(err, "auth login github")
	}
	if provider == "datadog" {
		if hostLoginTimedOut(err) {
			return fault.New(
				fault.KindRejected, "datadog_login_timeout",
				"The bounded Datadog OAuth login timed out; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Start a new Datadog login and complete browser consent within the bounded window."},
			)
		}
		if hostLoginCancelled(err) {
			return fault.New(
				fault.KindRejected, "datadog_login_cancelled",
				"Datadog OAuth login was cancelled; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host Datadog login when ready."},
			)
		}
		if errors.Is(err, credentialhost.ErrInvalidExecutable) {
			return classifyHostLoginError(hostCLIUnavailableError{provider: provider, stage: hostCLIStageExecutableIdentity}, provider, method)
		}
		if hostLoginFailureIsCredentialDriver(err) {
			return fault.New(
				fault.KindUnavailable, "datadog_login_failed",
				"Datadog OAuth login did not complete; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the isolated trusted-host pup login after inspecting the failure."},
			)
		}
		return classifyBrokerError(err, "auth login datadog")
	}
	if provider == "openai" {
		if hostLoginTimedOut(err) {
			return fault.New(
				fault.KindRejected, "openai_login_timeout",
				"The bounded Codex ChatGPT OAuth login timed out; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Start a new OpenAI login and complete device authorization within the bounded window."},
			)
		}
		if hostLoginCancelled(err) {
			return fault.New(
				fault.KindRejected, "openai_login_cancelled",
				"Codex ChatGPT OAuth login was cancelled; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host OpenAI login when ready."},
			)
		}
		if errors.Is(err, credentialhost.ErrCodexExecutable) {
			return classifyHostLoginError(hostCLIUnavailableError{provider: provider, stage: hostCLIStageCodexExecutableIdentity}, provider, method)
		}
		if errors.Is(err, credentialhost.ErrCodexVersion) {
			return classifyHostLoginError(hostCLIUnavailableError{provider: provider, stage: hostCLIStageCodexVersionObservation}, provider, method)
		}
		if hostLoginFailureIsCredentialDriver(err) {
			return fault.New(
				fault.KindUnavailable, "openai_login_failed",
				"Codex ChatGPT OAuth login did not complete; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the isolated trusted-host Codex login after inspecting the failure."},
			)
		}
		return classifyBrokerError(err, "auth login openai")
	}
	if provider == "anthropic" {
		if hostLoginTimedOut(err) {
			return fault.New(
				fault.KindRejected, "anthropic_login_timeout",
				"The bounded native Claude account login timed out; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Start a new Anthropic login and complete browser authorization within the bounded window."},
			)
		}
		if hostLoginCancelled(err) {
			return fault.New(
				fault.KindRejected, "anthropic_login_cancelled",
				"Native Claude account login was cancelled; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the isolated Context-runtime Anthropic login when ready."},
			)
		}
		if errors.Is(err, credentialhost.ErrClaudeExecutable) {
			return classifyHostLoginError(hostCLIUnavailableError{provider: provider, stage: hostCLIStageClaudeExecutableIdentity}, provider, method)
		}
		if errors.Is(err, credentialhost.ErrClaudeVersion) {
			return classifyHostLoginError(hostCLIUnavailableError{provider: provider, stage: hostCLIStageClaudeVersionObservation}, provider, method)
		}
		if errors.Is(err, credentialhost.ErrClaudeLoginSetup) {
			return fault.New(
				fault.KindUnavailable, "anthropic_login_setup_failed",
				"Tobari could not start the one-shot Claude login container; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "doctor", Reason: "Inspect the local Docker runtime before retrying Anthropic login."},
			)
		}
		if errors.Is(err, credentialhost.ErrClaudeLoginFailed) {
			return fault.New(
				fault.KindUnavailable, "anthropic_authorization_failed",
				"Claude account authorization did not complete in the one-shot login container; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Start a new isolated Context-runtime Claude login and paste the complete browser code when prompted."},
			)
		}
		if errors.Is(err, credentialhost.ErrClaudeOutputLimit) || errors.Is(err, errLoginVisibleOutputLimit) {
			return fault.New(
				fault.KindUnavailable, "anthropic_login_output_failed",
				"Claude login exceeded Tobari's bounded control-safe output contract; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "help runtime", Reason: "Verify the selected Context still provides the reviewed Claude Code 2.1.220 contract."},
			)
		}
		if errors.Is(err, credentialhost.ErrClaudeTokenCapture) ||
			errors.Is(err, credentialhost.ErrInvalidClaudeNativeCredential) {
			return fault.New(
				fault.KindUnavailable, "anthropic_credential_capture_failed",
				"Claude completed account login, but Tobari could not validate its native credential state; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Start a new isolated Context-runtime Claude login."},
			)
		}
		if errors.Is(err, credentialhost.ErrClaudeLoginCleanup) {
			return fault.New(
				fault.KindUnavailable, "anthropic_login_cleanup_failed",
				"The one-shot Claude login container could not be removed, so Tobari did not commit its credential; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "doctor", Reason: "Inspect the local Docker runtime before retrying Anthropic login."},
			)
		}
		if hostLoginFailureIsCredentialDriver(err) {
			return fault.New(
				fault.KindUnavailable, "anthropic_login_failed",
				"Native Claude account login did not complete; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the isolated Context-runtime Claude Code login after inspecting the failure."},
			)
		}
		return classifyBrokerError(err, "auth login anthropic")
	}
	if provider != "aws" {
		return classifyBrokerError(err, "auth login "+provider)
	}
	if method == awsConsoleMethod {
		if errors.Is(err, credentialhost.ErrConsoleLoginUnsupported) {
			return fault.New(
				fault.KindUnsupported, "aws_console_login_unsupported",
				"The trusted-host AWS CLI does not support console-based login; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Install AWS CLI 2.32 or newer on the trusted host, then retry console login."},
			)
		}
		if errors.Is(err, credentialhost.ErrInvalidProfile) {
			return fault.New(
				fault.KindInvalidInput, "aws_console_config_invalid",
				"The AWS console login configuration is invalid; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "help auth login", Reason: "Provide a valid commercial AWS region for console login."},
			)
		}
		if hostLoginTimedOut(err) {
			return fault.New(
				fault.KindRejected, "aws_console_login_timeout",
				"The bounded AWS console login timed out; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Start a new AWS console login and complete it within the bounded window."},
			)
		}
		if hostLoginCancelled(err) {
			return fault.New(
				fault.KindRejected, "aws_console_login_cancelled",
				"AWS console login was cancelled; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host AWS console login when ready."},
			)
		}
		if errors.Is(err, credentialhost.ErrInvalidExecutable) {
			return classifyHostLoginError(hostCLIUnavailableError{provider: provider, stage: hostCLIStageExecutableIdentity}, provider, method)
		}
		if hostLoginFailureIsCredentialDriver(err) {
			return fault.New(
				fault.KindUnavailable, "aws_console_login_failed",
				"AWS console login did not complete; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host AWS console login after inspecting the failure."},
			)
		}
		return classifyBrokerError(err, "auth login aws")
	}
	if errors.Is(err, credentialhost.ErrInvalidProfile) {
		return fault.New(
			fault.KindInvalidInput, "aws_sso_config_invalid",
			"The AWS IAM Identity Center login configuration is invalid; the previous Context credential remains unchanged.", false,
			fault.NextAction{Command: "help auth login", Reason: "Provide valid AWS IAM Identity Center login fields."},
		)
	}
	if hostLoginTimedOut(err) {
		return fault.New(
			fault.KindRejected, "aws_sso_login_timeout",
			"The bounded AWS IAM Identity Center device login timed out; the previous Context credential remains unchanged.",
			false,
			fault.NextAction{Command: "auth login", Reason: "Start a new AWS IAM Identity Center login and complete it within the bounded window."},
		)
	}
	if hostLoginCancelled(err) {
		return fault.New(
			fault.KindRejected, "aws_sso_login_cancelled",
			"AWS IAM Identity Center login was cancelled; the previous Context credential remains unchanged.", false,
			fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host AWS IAM Identity Center login when ready."},
		)
	}
	if errors.Is(err, credentialhost.ErrInvalidExecutable) {
		return classifyHostLoginError(hostCLIUnavailableError{provider: provider, stage: hostCLIStageExecutableIdentity}, provider, method)
	}
	if hostLoginFailureIsCredentialDriver(err) {
		return fault.New(
			fault.KindUnavailable, "aws_sso_login_failed",
			"AWS IAM Identity Center login did not complete; the previous Context credential remains unchanged.", false,
			fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host AWS IAM Identity Center login after inspecting the failure."},
		)
	}
	return classifyBrokerError(err, "auth login aws")
}

func normalizeHostCLIUnavailableStage(stage hostCLIUnavailableStage) hostCLIUnavailableStage {
	switch stage {
	case hostCLIStageDriverDependency,
		hostCLIStageExecutableLookup,
		hostCLIStageExecutableSymlink,
		hostCLIStageExecutableCanonicalPath,
		hostCLIStageExecutableTrustedRoot,
		hostCLIStageExecutableIdentity,
		hostCLIStageCodexChatGPTAppBundle,
		hostCLIStageCodexExecutableIdentity,
		hostCLIStageCodexVersionObservation,
		hostCLIStageClaudeContextSelection,
		hostCLIStageClaudeImageContract,
		hostCLIStageClaudeExecutableIdentity,
		hostCLIStageClaudeVersionObservation:
		return stage
	default:
		return hostCLIStageDriverDependency
	}
}

func (r *Runtime) ImportAuth(
	ctx context.Context, contextName, providerID string, secret io.Reader,
) (authbroker.MutationObservation, error) {
	manifest, provider, err := r.authOperationTarget(ctx, contextName, providerID)
	if err != nil {
		return authbroker.MutationObservation{}, err
	}
	if provider.Acquisition.Mode != authbroker.AcquisitionStdinImport {
		return authbroker.MutationObservation{}, fault.New(
			fault.KindUnsupported, "provider_import_unsupported",
			"The selected provider does not support credential import.", false,
			fault.NextAction{Command: "auth login", Reason: "Use the provider's reviewed built-in acquisition helper."},
		)
	}
	if secret == nil {
		return authbroker.MutationObservation{}, fault.New(fault.KindInvalidInput, "invalid_credential_input", "Credential stdin is unavailable.", false)
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.MutationObservation{}, classifyRootKeyError(err)
	}
	if err := r.requireAuthBroker(ctx); err != nil {
		return authbroker.MutationObservation{}, err
	}
	response, err := r.runBrokerControl(
		ctx, secret, "import", "--context-id", manifest.ID, "--provider", provider.ID,
	)
	if err != nil {
		return authbroker.MutationObservation{}, classifyBrokerError(err, "auth import "+provider.ID)
	}
	return r.buildAuthMutationObservation(ctx, authbroker.TaskImport, manifest.Name, manifest.ID, provider.ID, response, true, true, backend)
}

func (r *Runtime) AuthStatus(ctx context.Context, contextName string) (authbroker.StatusObservation, error) {
	observed, err := r.observeContext(contextName)
	if err != nil {
		if errors.Is(err, tobari.ErrContextNotFound) {
			return authbroker.StatusObservation{}, fault.New(
				fault.KindNotFound, "context_not_found", "The selected Context does not exist.", false,
				fault.NextAction{Command: "context list", Reason: "Choose an existing Context before using authentication."},
			)
		}
		return authbroker.StatusObservation{}, err
	}
	projection, err := r.loadAuthProviders()
	if err != nil {
		return authbroker.StatusObservation{}, err
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.StatusObservation{}, classifyRootKeyError(err)
	}
	result := authbroker.StatusObservation{
		ContextState: observed.state,
		Context:      observed.manifest.Name, ContextID: observed.manifest.ID,
		StorageBackend: backend, BrokerState: authbroker.BrokerStateUnavailable,
		Providers: []authbroker.ProviderStatus{},
		Workspaces: authbroker.WorkspaceObservation{
			Coverage:   authbroker.WorkspaceActivationCoverageNotApplicable,
			Workspaces: []authbroker.WorkspaceProjectionObservation{},
		},
	}
	for _, provider := range projection.Providers {
		result.Providers = append(result.Providers, authbroker.ProviderStatus{
			Provider: provider.ID,
			State:    authbroker.ProviderCredentialUnavailable,
		})
	}
	sort.Slice(result.Providers, func(left, right int) bool {
		return result.Providers[left].Provider < result.Providers[right].Provider
	})
	if observed.state != tobari.ContextObservationPersisted {
		return result, nil
	}
	manifest := observed.manifest
	if _, configured, stateErr := r.LoadState(ctx); stateErr != nil {
		return authbroker.StatusObservation{}, stateErr
	} else if configured {
		state, stateErr := r.brokerState(ctx)
		if stateErr == nil && state != authbroker.BrokerStateUnavailable {
			result.BrokerState = state
		}
	}
	for index := range result.Providers {
		status := result.Providers[index]
		if result.BrokerState == authbroker.BrokerStateReady {
			response, statusErr := r.runBrokerControl(
				ctx, nil, "status", "--context-id", manifest.ID, "--provider", status.Provider,
			)
			if statusErr != nil {
				return authbroker.StatusObservation{}, classifyBrokerError(statusErr, "auth status")
			}
			if response.State == "ready" {
				status.State = authbroker.ProviderCredentialConfigured
				status.CredentialRevision = response.Revision
				status.AccountLabel, err = validatedAccountLabel(response.AccountLabel)
				if err != nil {
					return authbroker.StatusObservation{}, err
				}
			} else {
				status.State = authbroker.ProviderCredentialNotConfigured
			}
		}
		result.Providers[index] = status
	}
	result.Workspaces = r.observeWorkspaceActivation(ctx, manifest.ID, result.Providers, projection)
	return result, nil
}

func (r *Runtime) LogoutAuth(
	ctx context.Context, contextName, providerID string,
) (authbroker.MutationObservation, error) {
	manifest, provider, err := r.authOperationTarget(ctx, contextName, providerID)
	if err != nil {
		return authbroker.MutationObservation{}, err
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.MutationObservation{}, classifyRootKeyError(err)
	}
	if err := r.requireAuthBroker(ctx); err != nil {
		return authbroker.MutationObservation{}, err
	}
	response, err := r.runBrokerControl(
		ctx, nil, "logout", "--context-id", manifest.ID, "--provider", provider.ID,
	)
	if err != nil {
		return authbroker.MutationObservation{}, classifyBrokerError(err, "auth logout")
	}
	changed := response.Changed != nil && *response.Changed
	return r.buildAuthMutationObservation(ctx, authbroker.TaskLogout, manifest.Name, manifest.ID, provider.ID, response, false, changed, backend)
}

func (r *Runtime) authOperationTarget(
	ctx context.Context, contextName, providerID string,
) (manifestResult struct {
	ID   string
	Name string
}, provider authbroker.Provider, err error) {
	manifest, err := r.resolveAuthContext(ctx, contextName)
	if err != nil {
		return manifestResult, authbroker.Provider{}, err
	}
	manifestResult.ID, manifestResult.Name = manifest.ID, manifest.Name
	projection, err := r.loadAuthProviders()
	if err != nil {
		return manifestResult, authbroker.Provider{}, err
	}
	provider, found := findAuthProvider(projection, providerID)
	if !found {
		return manifestResult, authbroker.Provider{}, fault.New(
			fault.KindNotFound, "provider_not_installed", "The credential provider is not installed.", false,
			fault.NextAction{Command: "auth status", Reason: "Inspect the installed built-in and user provider collection."},
		)
	}
	return manifestResult, provider, nil
}

func (r *Runtime) buildAuthMutationObservation(
	ctx context.Context,
	_ string, contextName, contextID, provider string,
	response brokerControlResponse,
	configured, changed bool,
	backend authbroker.StorageBackend,
) (authbroker.MutationObservation, error) {
	result := authbroker.MutationObservation{
		ContextState: tobari.ContextObservationPersisted,
		Provider:     provider, Context: contextName, ContextID: contextID,
		Configured: configured, StorageBackend: backend, BrokerState: authbroker.BrokerStateReady,
		Changed: changed, Providers: []authbroker.ProviderStatus{},
		Workspaces: authbroker.WorkspaceObservation{Coverage: authbroker.WorkspaceActivationCoverageUnavailable, Workspaces: []authbroker.WorkspaceProjectionObservation{}},
	}
	if !changed {
		result.Workspaces = authbroker.WorkspaceObservation{Coverage: authbroker.WorkspaceActivationCoverageNotApplicable, Workspaces: []authbroker.WorkspaceProjectionObservation{}}
	}
	if configured {
		result.CredentialRevision = response.Revision
		result.AccountLabel = response.AccountLabel
	}
	if changed {
		projection, projectionErr := r.loadAuthProviders()
		if projectionErr == nil {
			statuses := make([]authbroker.ProviderStatus, 0, len(projection.Providers))
			for _, installed := range projection.Providers {
				status := authbroker.ProviderStatus{Provider: installed.ID, State: authbroker.ProviderCredentialUnavailable}
				observed, statusErr := r.runBrokerControl(
					ctx, nil, "status", "--context-id", contextID, "--provider", installed.ID,
				)
				if statusErr == nil {
					switch observed.State {
					case "not_configured":
						status.State = authbroker.ProviderCredentialNotConfigured
					case "ready":
						status.State = authbroker.ProviderCredentialConfigured
						status.CredentialRevision = observed.Revision
					}
				}
				statuses = append(statuses, status)
			}
			result.Providers = statuses
			result.Workspaces = r.observeWorkspaceActivation(ctx, contextID, statuses, projection)
		}
	}
	return result, nil
}

func (r *Runtime) observeWorkspaceActivation(
	ctx context.Context,
	targetContextID string,
	statuses []authbroker.ProviderStatus,
	projection authbroker.Projection,
) authbroker.WorkspaceObservation {
	unavailable := func() authbroker.WorkspaceObservation {
		return authbroker.WorkspaceObservation{
			Coverage:   authbroker.WorkspaceActivationCoverageUnavailable,
			Workspaces: []authbroker.WorkspaceProjectionObservation{},
		}
	}
	if len(statuses) > authbroker.MaxWorkspaceActivationProviders ||
		len(projection.Providers) > authbroker.MaxWorkspaceActivationProviders {
		return unavailable()
	}
	projects, err := r.ListProjects(ctx)
	if err != nil || len(projects) > authbroker.MaxWorkspaceActivationItems {
		return unavailable()
	}
	statusByProvider := make(map[string]authbroker.ProviderStatus, len(statuses))
	providerByID := make(map[string]authbroker.Provider, len(projection.Providers))
	for _, status := range statuses {
		statusByProvider[status.Provider] = status
	}
	for _, provider := range projection.Providers {
		providerByID[provider.ID] = provider
	}
	type bindingCheck struct {
		providerIndex  int
		workspaceIndex int
		projectID      string
		providerID     string
		revision       string
		bindings       []byte
	}
	workspaces := make([]authbroker.WorkspaceProjectionObservation, 0, len(projects))
	checks := make([]bindingCheck, 0)
	bindingChecks := 0
	for _, project := range projects {
		workspace := authbroker.WorkspaceProjectionObservation{
			ProjectID: project.ID, Root: project.Root, ProjectContextID: project.ContextID,
			Incomplete: project.Incomplete, Providers: []authbroker.WorkspaceProviderObservation{},
		}
		registry, registryErr := r.readProjectAuthRegistry(project.ID)
		if registryErr != nil {
			workspaces = append(workspaces, workspace)
			continue
		}
		workspace.RegistryAvailable = true
		workspace.RegistryProjectID = registry.ProjectID
		if len(registry.Providers) > authbroker.MaxWorkspaceActivationProviders {
			return unavailable()
		}
		observed := make(map[string]projectAuthProviderBinding, len(registry.Providers))
		providerIDs := make(map[string]struct{}, len(statuses)+len(registry.Providers))
		for _, status := range statuses {
			providerIDs[status.Provider] = struct{}{}
		}
		for _, binding := range registry.Providers {
			observed[binding.Provider] = binding
			providerIDs[binding.Provider] = struct{}{}
		}
		if len(providerIDs) > authbroker.MaxWorkspaceActivationProviders {
			return unavailable()
		}
		ordered := make([]string, 0, len(providerIDs))
		for providerID := range providerIDs {
			ordered = append(ordered, providerID)
		}
		sort.Strings(ordered)
		for _, providerID := range ordered {
			status, installed := statusByProvider[providerID]
			current, projected := observed[providerID]
			fact := authbroker.WorkspaceProviderObservation{Provider: providerID, BindingState: authbroker.BrokerBindingNotObserved}
			if projected {
				fact.RegistryPresent = true
				fact.RegistryRevision = current.Revision
				fact.RegistryBindingDigest = current.BindingDigest
			}
			var encoded []byte
			if provider, exists := providerByID[providerID]; exists {
				_, encodedBindings, digest, bindingErr := brokerBindingsForProvider(projection, provider.ID)
				if bindingErr == nil {
					fact.ExpectedBindingDigest = digest
					encoded = encodedBindings
				}
			}
			workspace.Providers = append(workspace.Providers, fact)
			if project.ContextID == targetContextID && installed && status.State == authbroker.ProviderCredentialConfigured && projected &&
				fact.ExpectedBindingDigest != "" && current.Revision == status.CredentialRevision &&
				current.BindingDigest == fact.ExpectedBindingDigest {
				checks = append(checks, bindingCheck{
					workspaceIndex: len(workspaces), providerIndex: len(workspace.Providers) - 1,
					projectID: project.ID, providerID: providerID, revision: current.Revision, bindings: encoded,
				})
				bindingChecks++
			}
		}
		workspaces = append(workspaces, workspace)
	}
	if bindingChecks > authbroker.MaxWorkspaceActivationBindingChecks {
		return unavailable()
	}
	for _, check := range checks {
		fact := &workspaces[check.workspaceIndex].Providers[check.providerIndex]
		fact.BindingProvider = check.providerID
		fact.BindingRevision = check.revision
		fact.BindingState = authbroker.BrokerBindingUnavailable
		binding, bindingErr := r.runBrokerControl(
			ctx, nil, "binding_status", "--context-id", workspaces[check.workspaceIndex].ProjectContextID,
			"--project-id", check.projectID, "--provider", check.providerID,
			"--revision", check.revision, "--bindings", string(check.bindings),
		)
		if bindingErr == nil {
			switch binding.State {
			case "ready":
				fact.BindingState = authbroker.BrokerBindingReady
			case "missing":
				fact.BindingState = authbroker.BrokerBindingMissing
			case "stale":
				fact.BindingState = authbroker.BrokerBindingStale
			}
		}
	}
	return authbroker.WorkspaceObservation{
		Coverage:   authbroker.WorkspaceActivationCoverageExhaustive,
		Workspaces: workspaces,
	}
}

func validatedAccountLabel(label *string) (*string, error) {
	if label == nil || *label == "" {
		return nil, nil
	}
	value := *label
	if authbroker.ValidateSecretFreeText("account label", value, 128) != nil {
		return nil, fault.New(
			fault.KindContract, "invalid_auth_broker_metadata",
			"The Auth Broker returned invalid non-secret account metadata.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the Auth Broker and provider helper contract."},
		)
	}
	return &value, nil
}
