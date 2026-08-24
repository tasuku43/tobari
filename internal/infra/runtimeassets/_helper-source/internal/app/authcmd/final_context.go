package authcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// FinalContextReader supplies one coherent final-only Context authority. It is
// deliberately separate from the research credential adapter so application
// code never asks the latter to rediscover an owner by name or UUID.
type FinalContextReader interface {
	ReadContextAuthorityByReference(context.Context, string) (tobari.ContextAuthoritySnapshot, error)
}

// FinalContextRuntimePort is the complete dormant external boundary for final
// Context research authentication. Implementations must revalidate Authority
// against the current final envelope under the installation lifecycle lock.
type FinalContextRuntimePort interface {
	IsInputTerminal(io.Reader) bool
	IsTerminal(io.Writer) bool
	LoginFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority, string, string, io.Reader, io.Writer) (authbroker.ContextMutationObservation, error)
	ImportFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority, string, io.Reader) (authbroker.ContextMutationObservation, error)
	StatusFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority) (authbroker.ContextStatusObservation, error)
	LogoutFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority, string) (authbroker.ContextMutationObservation, error)
}

type finalContextMutationPolicy struct{}

func (finalContextMutationPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Command == "auth login" || intent.Command == "auth import" {
		if intent.Effect == operation.EffectCreate && intent.Target.Kind == authbroker.ContextCredentialTargetKind && intent.Target.ParentID != "" && intent.Target.ID == "" {
			if _, err := tobari.ParseContextRef(intent.Target.ParentID); err == nil {
				return nil
			}
		}
	}
	if intent.Command == "auth logout" && intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.ContextReferenceKind && intent.Target.ParentID == "" {
		if _, err := tobari.ParseContextRef(intent.Target.ID); err == nil {
			return nil
		}
	}
	return fault.New(fault.KindRejected, "mutation_rejected", "Research authentication mutation target is not an exact final Context authority", false)
}

// FinalContextService owns the final schema-v2 task semantics. It remains
// dormant until the atomic Catalog/composition cutover.
type FinalContextService struct {
	reader  FinalContextReader
	runtime FinalContextRuntimePort
	mutator *execution.Invoker
}

func NewFinalContext(reader FinalContextReader, runtime FinalContextRuntimePort) *FinalContextService {
	return &FinalContextService{reader: reader, runtime: runtime, mutator: execution.New(finalContextMutationPolicy{})}
}

func FinalContextMutationImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes}
}

func (s *FinalContextService) Login(ctx context.Context, intent operation.Intent, contextRef, provider, method string, input io.Reader, errOut io.Writer) (authbroker.ContextResult, error) {
	if err := validateFinalContextMutationInput(contextRef, provider); err != nil {
		return authbroker.ContextResult{}, err
	}
	if !SupportsLoginProvider(provider) {
		return authbroker.ContextResult{}, fault.New(fault.KindUnsupported, "provider_login_unsupported", "The selected provider does not support a built-in login helper.", false)
	}
	loginMethod, err := normalizeLoginMethod(provider, method)
	if err != nil {
		return authbroker.ContextResult{}, err
	}
	return s.invokeFinalContextMutation(ctx, intent, "auth login", operation.EffectCreate, contextRef, provider, func(actionContext context.Context, authority authbroker.ContextAuthenticationAuthority) (authbroker.ContextMutationObservation, error) {
		if !s.runtime.IsInputTerminal(input) || !s.runtime.IsTerminal(errOut) {
			return authbroker.ContextMutationObservation{}, finalAuthLoginTerminalRequiredFault()
		}
		return s.runtime.LoginFinalContextAuth(actionContext, authority, provider, string(loginMethod), input, errOut)
	})
}

func finalAuthLoginTerminalRequiredFault() error {
	return fault.New(fault.KindInvalidInput, "auth_login_tty_required", "Built-in provider login requires interactive terminal streams on stdin and stderr.", false, fault.NextAction{Command: "help auth login", Reason: "Run trusted-host provider login from an interactive terminal."})
}

func (s *FinalContextService) Import(ctx context.Context, intent operation.Intent, contextRef, provider string, input io.Reader) (authbroker.ContextResult, error) {
	if err := validateFinalContextMutationInput(contextRef, provider); err != nil {
		return authbroker.ContextResult{}, err
	}
	if input == nil {
		return authbroker.ContextResult{}, invalidCredentialInput("Credential stdin is not configured.", nil)
	}
	return s.invokeFinalContextMutation(ctx, intent, "auth import", operation.EffectCreate, contextRef, provider, func(actionContext context.Context, authority authbroker.ContextAuthenticationAuthority) (authbroker.ContextMutationObservation, error) {
		if s.runtime.IsInputTerminal(input) {
			return authbroker.ContextMutationObservation{}, invalidCredentialInput("Interactive terminal credential input is not supported; pipe or redirect one credential through stdin.", nil)
		}
		secret, err := readCredentialInput(input)
		if err != nil {
			return authbroker.ContextMutationObservation{}, err
		}
		defer clear(secret)
		return s.runtime.ImportFinalContextAuth(actionContext, authority, provider, bytes.NewReader(secret))
	})
}

func (s *FinalContextService) Logout(ctx context.Context, intent operation.Intent, contextRef, provider string) (authbroker.ContextResult, error) {
	if err := validateFinalContextMutationInput(contextRef, provider); err != nil {
		return authbroker.ContextResult{}, err
	}
	return s.invokeFinalContextMutation(ctx, intent, "auth logout", operation.EffectWrite, contextRef, provider, func(actionContext context.Context, authority authbroker.ContextAuthenticationAuthority) (authbroker.ContextMutationObservation, error) {
		return s.runtime.LogoutFinalContextAuth(actionContext, authority, provider)
	})
}

func (s *FinalContextService) Status(ctx context.Context, contextRef string) (authbroker.ContextStatusResult, error) {
	if err := validateFinalContextRef(contextRef); err != nil {
		return authbroker.ContextStatusResult{}, err
	}
	if err := s.requireFinalContextPorts(); err != nil {
		return authbroker.ContextStatusResult{}, err
	}
	authority, err := s.readFinalContextAuthority(ctx, contextRef)
	if err != nil {
		return authbroker.ContextStatusResult{}, err
	}
	observed, err := s.runtime.StatusFinalContextAuth(ctx, authority)
	if err != nil {
		if public, ok := fault.PublicCopy(err); ok {
			return authbroker.ContextStatusResult{}, public
		}
		return authbroker.ContextStatusResult{}, fault.Wrap(fault.KindUnavailable, "auth_status_failed", "Authentication status could not be read.", false, err, fault.NextAction{Command: "doctor", Reason: "Inspect the final Context and research Auth Broker stores."})
	}
	result, err := authbroker.NewContextStatusResult(contextRef, observed)
	if err != nil {
		return authbroker.ContextStatusResult{}, invalidResult(err)
	}
	return result, nil
}

func (s *FinalContextService) invokeFinalContextMutation(ctx context.Context, intent operation.Intent, command string, effect operation.Effect, contextRef, provider string, action func(context.Context, authbroker.ContextAuthenticationAuthority) (authbroker.ContextMutationObservation, error)) (authbroker.ContextResult, error) {
	if err := s.requireFinalContextPorts(); err != nil {
		return authbroker.ContextResult{}, err
	}
	target := operation.TargetRef{Kind: authbroker.ContextCredentialTargetKind, ParentID: contextRef}
	if effect == operation.EffectWrite {
		target = operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: contextRef}
	}
	request := execution.Request{Intent: intent, ExpectedCommand: command, ExpectedEffect: effect, ExpectedTarget: target, ExpectedImpact: FinalContextMutationImpact()}
	var result authbroker.ContextResult
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		authority, err := s.readFinalContextAuthority(actionContext, contextRef)
		if err != nil {
			return err
		}
		observation, err := action(actionContext, authority)
		if err != nil {
			return err
		}
		result, err = authbroker.NewContextResult(taskForAuthCommand(command), contextRef, provider, observation)
		if err != nil {
			return invalidResult(err)
		}
		return nil
	})
	if err != nil {
		return authbroker.ContextResult{}, err
	}
	return result, nil
}

func (s *FinalContextService) readFinalContextAuthority(ctx context.Context, contextRef string) (authbroker.ContextAuthenticationAuthority, error) {
	snapshot, err := s.reader.ReadContextAuthorityByReference(ctx, contextRef)
	if err != nil {
		return authbroker.ContextAuthenticationAuthority{}, err
	}
	return authbroker.NewContextAuthenticationAuthority(snapshot, contextRef)
}

func (s *FinalContextService) requireFinalContextPorts() error {
	if s == nil || portcheck.IsNil(s.reader) || portcheck.IsNil(s.runtime) || s.mutator == nil {
		return fault.New(fault.KindInternal, "missing_runtime", "Final Context research authentication is not configured", false)
	}
	return nil
}

func validateFinalContextRef(contextRef string) error {
	if _, err := tobari.ParseContextRef(contextRef); err != nil {
		return fault.Wrap(fault.KindInvalidInput, "invalid_context_ref", "Context reference is invalid.", false, err, fault.NextAction{Command: "context list", Reason: "Choose one current Context reference."})
	}
	return nil
}

func validateFinalContextMutationInput(contextRef, provider string) error {
	if err := validateFinalContextRef(contextRef); err != nil {
		return err
	}
	return validateProvider(provider)
}

func taskForAuthCommand(command string) string {
	switch command {
	case "auth login":
		return authbroker.TaskLogin
	case "auth import":
		return authbroker.TaskImport
	case "auth logout":
		return authbroker.TaskLogout
	default:
		panic(fmt.Sprintf("unreachable final auth command %q", command))
	}
}
