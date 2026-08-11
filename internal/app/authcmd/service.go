// Package authcmd owns Context-scoped Auth Broker workflows.
package authcmd

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	BuiltinGitHubProviderID               = "github"
	BuiltinAWSProviderID                  = "aws"
	BuiltinDatadogProviderID              = "datadog"
	LoginMethodIdentityCenter LoginMethod = "identity-center"
	LoginMethodConsole        LoginMethod = "console"
)

// LoginMethod selects one reviewed AWS CLI acquisition plan. The empty value
// is accepted only at the public boundary and normalizes to identity-center for
// AWS so the pre-existing command remains backward compatible.
type LoginMethod string

// MutationImpact is the application-owned generic impact contract shared by
// login, import, and logout. Each may rotate or revoke every project handle
// associated with the one Context credential being changed.
func MutationImpact() operation.Impact {
	return operation.Impact{
		Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
		AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes,
	}
}

// RuntimePort is the complete external boundary needed by the auth commands.
// Credential bytes enter only through the purpose-bound import reader.
type RuntimePort interface {
	IsInputTerminal(io.Reader) bool
	IsTerminal(io.Writer) bool
	LoginAuth(context.Context, string, string, string, io.Reader, io.Writer) (authbroker.MutationObservation, error)
	ImportAuth(context.Context, string, string, io.Reader) (authbroker.MutationObservation, error)
	AuthStatus(context.Context, string) (authbroker.StatusObservation, error)
	LogoutAuth(context.Context, string, string) (authbroker.MutationObservation, error)
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == authbroker.CredentialCatalogTargetKind &&
		intent.Target.ID == authbroker.CredentialCatalogTargetID {
		return nil
	}
	return fault.New(
		fault.KindRejected,
		"mutation_rejected",
		"Auth Broker mutation target is not owned by Tobari",
		false,
	)
}

// Service validates public auth tasks before crossing the trusted runtime
// boundary. It never retains or renders credential material.
type Service struct {
	runtime RuntimePort
	mutator *execution.Invoker
}

func New(runtime RuntimePort) *Service {
	return &Service{runtime: runtime, mutator: execution.New(ownedPolicy{})}
}

// ValidateLoginTerminal verifies the interactive stream boundary without
// reading input or starting provider acquisition. The CLI uses it before an
// omitted-provider menu; Login repeats the check immediately before the host
// driver runs.
func (s *Service) ValidateLoginTerminal(input io.Reader, errOut io.Writer) error {
	if err := s.requireRuntime(); err != nil {
		return err
	}
	if !s.runtime.IsInputTerminal(input) || !s.runtime.IsTerminal(errOut) {
		return fault.New(
			fault.KindInvalidInput,
			"auth_login_tty_required",
			"Built-in provider login requires interactive terminal streams on stdin and stderr.",
			false,
			fault.NextAction{Command: "help auth login", Reason: "Run trusted-host provider login from an interactive terminal."},
		)
	}
	return nil
}

// SupportsLoginProvider reports whether provider is backed by one of the
// closed, reviewed host login drivers compiled into Tobari.
func SupportsLoginProvider(provider string) bool {
	return supportsBuiltinLogin(provider)
}

func (s *Service) Login(
	ctx context.Context, intent operation.Intent, contextName, provider, method string,
	input io.Reader, errOut io.Writer,
) (authbroker.Result, error) {
	if err := validateContextName(contextName); err != nil {
		return authbroker.Result{}, err
	}
	if err := validateProvider(provider); err != nil {
		return authbroker.Result{}, err
	}
	if !supportsBuiltinLogin(provider) {
		return authbroker.Result{}, fault.New(
			fault.KindUnsupported,
			"provider_login_unsupported",
			"The selected provider does not support a built-in login helper.",
			false,
			fault.NextAction{Command: "auth status", Reason: "Inspect the installed providers and their declared acquisition modes."},
		)
	}
	loginMethod, err := normalizeLoginMethod(provider, method)
	if err != nil {
		return authbroker.Result{}, err
	}
	if err := s.requireRuntime(); err != nil {
		return authbroker.Result{}, err
	}
	return s.invokeMutation(ctx, intent, "auth login", authbroker.TaskLogin, contextName, provider, func(actionContext context.Context) (authbroker.Result, error) {
		if err := s.ValidateLoginTerminal(input, errOut); err != nil {
			return authbroker.Result{}, err
		}
		observed, err := s.runtime.LoginAuth(actionContext, contextName, provider, string(loginMethod), input, errOut)
		if err != nil {
			return authbroker.Result{}, err
		}
		return authbroker.NewResult(authbroker.TaskLogin, contextName, provider, observed)
	})
}

func normalizeLoginMethod(provider, method string) (LoginMethod, error) {
	if provider != BuiltinAWSProviderID {
		if method == "" {
			return "", nil
		}
		return "", fault.New(
			fault.KindInvalidInput,
			"auth_login_method_not_applicable",
			"The login method selector applies only to the AWS provider.",
			false,
			fault.NextAction{Command: "help auth login", Reason: "Remove --method for non-AWS providers."},
		)
	}
	if method == "" || method == string(LoginMethodIdentityCenter) {
		return LoginMethodIdentityCenter, nil
	}
	if method == string(LoginMethodConsole) {
		return LoginMethodConsole, nil
	}
	return "", fault.New(
		fault.KindInvalidInput,
		"invalid_aws_login_method",
		"The AWS login method is invalid.",
		false,
		fault.NextAction{Command: "help auth login", Reason: "Choose identity-center or console."},
	)
}

func supportsBuiltinLogin(provider string) bool {
	return provider == BuiltinGitHubProviderID || provider == BuiltinAWSProviderID || provider == BuiltinDatadogProviderID
}

func (s *Service) Import(
	ctx context.Context,
	intent operation.Intent,
	contextName, provider string,
	input io.Reader,
) (authbroker.Result, error) {
	if err := validateContextName(contextName); err != nil {
		return authbroker.Result{}, err
	}
	if err := validateProvider(provider); err != nil {
		return authbroker.Result{}, err
	}
	if input == nil {
		return authbroker.Result{}, invalidCredentialInput("Credential stdin is not configured.", nil)
	}
	if err := s.requireRuntime(); err != nil {
		return authbroker.Result{}, err
	}
	return s.invokeMutation(ctx, intent, "auth import", authbroker.TaskImport, contextName, provider, func(actionContext context.Context) (authbroker.Result, error) {
		// Inspect the original stdin stream only after the mutation contract is
		// accepted, but before reading credential bytes or invoking the runtime
		// mutation. Production uses the runtime's file-stat-backed terminal seam;
		// tests inject the same narrow capability.
		if s.runtime.IsInputTerminal(input) {
			return authbroker.Result{}, invalidCredentialInput(
				"Interactive terminal credential input is not supported; pipe or redirect one credential through stdin.",
				nil,
			)
		}
		secret, err := readCredentialInput(input)
		if err != nil {
			return authbroker.Result{}, err
		}
		defer clear(secret)
		observed, err := s.runtime.ImportAuth(actionContext, contextName, provider, bytes.NewReader(secret))
		if err != nil {
			return authbroker.Result{}, err
		}
		return authbroker.NewResult(authbroker.TaskImport, contextName, provider, observed)
	})
}

func (s *Service) Status(ctx context.Context, contextName string) (authbroker.StatusResult, error) {
	if err := validateContextName(contextName); err != nil {
		return authbroker.StatusResult{}, err
	}
	if err := s.requireRuntime(); err != nil {
		return authbroker.StatusResult{}, err
	}
	observed, err := s.runtime.AuthStatus(ctx, contextName)
	if err != nil {
		if public, ok := fault.PublicCopy(err); ok {
			return authbroker.StatusResult{}, public
		}
		return authbroker.StatusResult{}, fault.Wrap(
			fault.KindUnavailable,
			"auth_status_failed",
			"Authentication status could not be read.",
			false,
			err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the Auth Broker and Context credential stores."},
		)
	}
	result, err := authbroker.NewStatusResult(contextName, observed)
	if err != nil {
		return authbroker.StatusResult{}, invalidResult(err)
	}
	return result, nil
}

func validateStatusResult(result authbroker.StatusResult, contextName string) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if contextName != "" && result.Context != contextName {
		return errors.New("authentication status Context does not match the request")
	}
	return nil
}

func (s *Service) Logout(
	ctx context.Context, intent operation.Intent, contextName, provider string,
) (authbroker.Result, error) {
	if err := validateContextName(contextName); err != nil {
		return authbroker.Result{}, err
	}
	if err := validateProvider(provider); err != nil {
		return authbroker.Result{}, err
	}
	if err := s.requireRuntime(); err != nil {
		return authbroker.Result{}, err
	}
	return s.invokeMutation(ctx, intent, "auth logout", authbroker.TaskLogout, contextName, provider, func(actionContext context.Context) (authbroker.Result, error) {
		observed, err := s.runtime.LogoutAuth(actionContext, contextName, provider)
		if err != nil {
			return authbroker.Result{}, err
		}
		return authbroker.NewResult(authbroker.TaskLogout, contextName, provider, observed)
	})
}

func (s *Service) invokeMutation(
	ctx context.Context,
	intent operation.Intent,
	command, task, contextName, provider string,
	action func(context.Context) (authbroker.Result, error),
) (authbroker.Result, error) {
	request := execution.Request{
		Intent:          intent,
		ExpectedCommand: command,
		ExpectedEffect:  operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{
			Kind: authbroker.CredentialCatalogTargetKind,
			ID:   authbroker.CredentialCatalogTargetID,
		},
		ExpectedImpact: MutationImpact(),
	}
	var result authbroker.Result
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		candidate, actionErr := action(actionContext)
		if actionErr != nil {
			return actionErr
		}
		if err := validateResult(candidate, task, contextName, provider); err != nil {
			return invalidResult(err)
		}
		result = candidate
		return nil
	})
	if err != nil {
		return authbroker.Result{}, err
	}
	return result, nil
}

func (s *Service) requireRuntime() error {
	if s == nil || portcheck.IsNil(s.runtime) {
		return fault.New(fault.KindInternal, "missing_runtime", "Auth Broker runtime is not configured", false)
	}
	return nil
}

func validateContextName(name string) error {
	if name == "" {
		return nil
	}
	if err := tobari.ValidateName(name); err != nil {
		return fault.Wrap(
			fault.KindInvalidInput,
			"invalid_context_name",
			"Context name is invalid.",
			false,
			err,
			fault.NextAction{Command: "context list", Reason: "Choose an existing Context name."},
		)
	}
	return nil
}

func validateProvider(provider string) error {
	if err := authbroker.ValidateProviderID(provider); err != nil {
		return fault.Wrap(
			fault.KindInvalidInput,
			"invalid_provider",
			"Credential provider ID is invalid.",
			false,
			err,
			fault.NextAction{Command: "auth status", Reason: "Inspect the Context's current authentication state."},
		)
	}
	return nil
}

func readCredentialInput(input io.Reader) ([]byte, error) {
	secret, err := io.ReadAll(io.LimitReader(input, authbroker.MaxPrimarySecretBytes+1))
	if err != nil {
		clear(secret)
		return nil, invalidCredentialInput("Credential stdin could not be read.", err)
	}
	if len(secret) == 0 {
		return nil, invalidCredentialInput("Credential stdin is empty.", nil)
	}
	if len(secret) > authbroker.MaxPrimarySecretBytes {
		clear(secret)
		return nil, invalidCredentialInput("Credential stdin exceeds the supported size limit.", nil)
	}
	return secret, nil
}

func invalidCredentialInput(message string, cause error) error {
	return fault.Wrap(
		fault.KindInvalidInput,
		"invalid_credential_input",
		message,
		false,
		cause,
		fault.NextAction{Command: "help auth import", Reason: "Provide one bounded credential through stdin."},
	)
}

func validateResult(result authbroker.Result, task, contextName, provider string) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if result.Task != task {
		return errors.New("authentication result task does not match the request")
	}
	if contextName != "" && result.Context != contextName {
		return errors.New("authentication result Context does not match the request")
	}
	if provider != "" && result.Provider != provider {
		return errors.New("authentication result provider does not match the request")
	}
	return nil
}

func invalidResult(cause error) error {
	return fault.Wrap(
		fault.KindContract,
		"invalid_auth_result",
		"Authentication result is invalid.",
		false,
		cause,
		fault.NextAction{Command: "auth status", Reason: "Reconcile the Context's authentication state before another mutation."},
	)
}
