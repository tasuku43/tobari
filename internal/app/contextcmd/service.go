// Package contextcmd owns the host-side Context composition workflow.
package contextcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// RuntimePort is the smallest boundary needed to inspect and change the
// host-owned current/default Context. It deliberately exposes no secret-reading API.
type RuntimePort interface {
	ListContexts(context.Context) (tobari.ContextListResult, error)
	ShowContext(context.Context, string) (tobari.ContextReport, error)
	CreateContext(context.Context, string, string, tobari.ContextPolicyMode, tobari.ContextSourceAccess) (tobari.ContextReport, error)
	UseContext(context.Context, string) (tobari.ContextReport, error)
	ConfigureContextShell(context.Context, string, []tobari.ContextShellEnvironmentSetting) (tobari.ContextReport, error)
	ConfigureContextGit(context.Context, string, tobari.ContextGitIdentitySetting) (tobari.ContextReport, error)
	InitRuntime(context.Context) (tobari.ContextReport, error)
	BuildRuntime(context.Context) (tobari.ContextReport, error)
}

type contextUseProgressRuntimePort interface {
	UseContextWithProgress(context.Context, string, tobari.ClusterUpProgressSink) (tobari.ContextReport, error)
}

type policyPresetContextRuntimePort interface {
	CreateContextWithPreset(context.Context, string, string, tobari.ContextPolicyMode, tobari.ContextSourceAccess, string, ...tobari.ContextNativeReadiness) (tobari.ContextReport, error)
}

type composedContextRuntimePort interface {
	CreateContextWithComposition(context.Context, string, string, tobari.ContextPolicyMode, tobari.ContextSourceAccess, tobari.ContextCreateComposition) (tobari.ContextReport, error)
}

type contextAWSBootstrapRuntimePort interface {
	PrepareContextAWSBootstrap(context.Context, string) (tobari.ContextBootstrapSnapshot, error)
	PreviewContextAWSBootstrap(context.Context, string, string) (tobari.ContextBootstrapPreview, error)
	ConfigureContextAWSBootstrap(context.Context, string, string, string, bool) (tobari.ContextReport, error)
}

func (s *Service) PreviewAWSBootstrap(ctx context.Context, contextName, profile string) (tobari.ContextBootstrapPreview, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextBootstrapPreview{}, err
	}
	runtime, ok := s.runtime.(contextAWSBootstrapRuntimePort)
	if !ok {
		return tobari.ContextBootstrapPreview{}, fault.New(fault.KindInternal, "missing_runtime", "Context bootstrap runtime is unavailable", false)
	}
	preview, err := runtime.PreviewContextAWSBootstrap(ctx, contextName, profile)
	if errors.Is(err, tobari.ErrContextBootstrapNotConfigured) {
		return tobari.ContextBootstrapPreview{}, fault.New(fault.KindNotFound, "bootstrap_not_configured", "the selected Context has no AWS bootstrap snapshot to refresh", false, fault.NextAction{Command: "help config bootstrap aws", Reason: "Configure an IAM Identity Center profile first."})
	}
	if err != nil {
		return tobari.ContextBootstrapPreview{}, fault.Wrap(fault.KindRejected, "aws_bootstrap_source_rejected", "Host AWS IAM Identity Center configuration could not be normalized", false, err, fault.NextAction{Command: "help config bootstrap aws", Reason: "Use a strict IAM Identity Center profile without credentials, helpers, or unsupported directives."})
	}
	if err := preview.Validate(); err != nil {
		return tobari.ContextBootstrapPreview{}, fault.Wrap(fault.KindContract, "invalid_bootstrap_preview", "AWS bootstrap preview is invalid", false, err)
	}
	return preview, nil
}

type contextDeleteRuntimePort interface {
	DeleteContext(context.Context, string) (tobari.ContextDeleteResult, error)
}

type contextRuntimeBuildProgressPort interface {
	BuildRuntimeWithProgress(context.Context, io.Writer, tobari.RuntimeBuildProgressSink) (tobari.ContextReport, error)
}

type contextLifecycleRuntimePort interface {
	WithLifecycleLock(context.Context, func(context.Context) error) error
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Effect == operation.EffectCreate &&
		intent.Target.Kind == tobari.ContextCatalogTargetKind &&
		intent.Target.ParentID == tobari.ContextCatalogTargetID && intent.Target.ID == "" {
		return nil
	}
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == tobari.ContextTargetKind &&
		intent.Target.ID == tobari.ActiveContextTargetID {
		return nil
	}
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == tobari.ContextCatalogTargetKind &&
		intent.Target.ID == tobari.ContextCatalogTargetID {
		return nil
	}
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == tobari.ContextShellTargetKind &&
		intent.Target.ID == tobari.ContextShellTargetID {
		return nil
	}
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == tobari.ContextGitIdentityTargetKind &&
		intent.Target.ID == tobari.ContextGitIdentityTargetID {
		return nil
	}
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == tobari.ContextBootstrapTargetKind &&
		intent.Target.ID == tobari.ContextBootstrapTargetID {
		return nil
	}
	if intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.ContextRuntimeTargetKind &&
		intent.Target.ParentID == tobari.ActiveContextRuntimeID && intent.Target.ID == "" {
		return nil
	}
	if intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.ContextRuntimeTargetKind &&
		intent.Target.ID == tobari.ActiveContextRuntimeID {
		return nil
	}
	return fault.New(fault.KindRejected, "mutation_rejected", "Context mutation target is not owned by Tobari", false)
}

// PrepareAWSBootstrap normalizes one host AWS IAM Identity Center profile into
// a secret-free snapshot suitable for Context creation. The application never
// receives credential or token-cache bytes.
func (s *Service) PrepareAWSBootstrap(ctx context.Context, profile string) (tobari.ContextBootstrapSnapshot, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextBootstrapSnapshot{}, err
	}
	if profile == "" {
		return tobari.ContextBootstrapSnapshot{}, fault.New(fault.KindInvalidInput, "invalid_aws_bootstrap_profile", "AWS bootstrap profile is required", false, fault.NextAction{Command: "help config bootstrap aws", Reason: "Choose one IAM Identity Center profile from the host AWS shared config."})
	}
	runtime, ok := s.runtime.(contextAWSBootstrapRuntimePort)
	if !ok {
		return tobari.ContextBootstrapSnapshot{}, fault.New(fault.KindInternal, "missing_runtime", "Context bootstrap runtime is unavailable", false, fault.NextAction{Command: "doctor", Reason: "Configure the Tobari runtime."})
	}
	snapshot, err := runtime.PrepareContextAWSBootstrap(ctx, profile)
	if err != nil {
		return tobari.ContextBootstrapSnapshot{}, fault.Wrap(fault.KindRejected, "aws_bootstrap_source_rejected", "Host AWS IAM Identity Center configuration could not be normalized", false, err, fault.NextAction{Command: "help config bootstrap aws", Reason: "Use a strict IAM Identity Center profile without credentials, helpers, or unsupported directives."})
	}
	if err := snapshot.Validate(); err != nil {
		return tobari.ContextBootstrapSnapshot{}, fault.Wrap(fault.KindContract, "invalid_bootstrap_snapshot", "AWS bootstrap snapshot is invalid", false, err)
	}
	return snapshot, nil
}

// ConfigureAWSBootstrap replaces, refreshes, or removes the AWS recipe used by
// future Workspaces. Existing Workspace homes are outside this mutation.
func (s *Service) ConfigureAWSBootstrap(ctx context.Context, intent operation.Intent, contextName, profile, expectedRevision string, remove bool) (tobari.ContextReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextReport{}, err
	}
	if contextName != "" {
		if err := tobari.ValidateName(contextName); err != nil {
			return tobari.ContextReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_context_name", "Context name is invalid", false, err, fault.NextAction{Command: "context list", Reason: "Choose a named Context from the local collection."})
		}
	}
	if remove && profile != "" {
		return tobari.ContextReport{}, fault.New(fault.KindInvalidInput, "invalid_aws_bootstrap_change", "AWS bootstrap removal cannot select a profile", false, fault.NextAction{Command: "help config bootstrap aws", Reason: "Choose configure/refresh or remove."})
	}
	runtime, ok := s.runtime.(contextAWSBootstrapRuntimePort)
	if !ok {
		return tobari.ContextReport{}, fault.New(fault.KindInternal, "missing_runtime", "Context bootstrap runtime is unavailable", false, fault.NextAction{Command: "doctor", Reason: "Configure the Tobari runtime."})
	}
	request := execution.Request{Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite, ExpectedTarget: operation.TargetRef{Kind: tobari.ContextBootstrapTargetKind, ID: tobari.ContextBootstrapTargetID}, ExpectedImpact: intent.Impact}
	var result tobari.ContextReport
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			configured, configureErr := runtime.ConfigureContextAWSBootstrap(actionContext, contextName, profile, expectedRevision, remove)
			switch {
			case errors.Is(configureErr, tobari.ErrContextNotFound):
				return fault.New(fault.KindNotFound, "context_not_found", "the named Context does not exist", false, fault.NextAction{Command: "context list", Reason: "Choose an existing Context."})
			case errors.Is(configureErr, tobari.ErrContextBootstrapNotConfigured):
				return fault.New(fault.KindNotFound, "bootstrap_not_configured", "the selected Context has no AWS bootstrap snapshot to refresh", false, fault.NextAction{Command: "help config bootstrap aws", Reason: "Configure an IAM Identity Center profile first."})
			case errors.Is(configureErr, tobari.ErrContextBootstrapSourceChanged):
				return fault.New(fault.KindRejected, "bootstrap_source_changed", "Host AWS configuration changed during review; no Context change was applied", true, fault.NextAction{Command: "config bootstrap aws", Reason: "Review a fresh semantic diff before applying."})
			case configureErr != nil:
				return fault.Wrap(fault.KindRejected, "config_bootstrap_failed", "Context AWS bootstrap could not be changed", false, configureErr, fault.NextAction{Command: "context show", Reason: "Inspect the current future-Workspace bootstrap recipe."})
			}
			if configured.Task != tobari.TaskConfigBootstrapAWS || configured.ContextState != tobari.ContextObservationPersisted {
				return fault.New(fault.KindContract, "invalid_context_report", "Context bootstrap report is invalid", false, fault.NextAction{Command: "context show", Reason: "Reconcile the confirmed Context bootstrap change."})
			}
			if remove && configured.Bootstrap.Resolved().State != tobari.ContextBootstrapNotConfigured {
				return fault.New(fault.KindContract, "invalid_context_report", "Context bootstrap removal was not confirmed", false)
			}
			if !remove && configured.Bootstrap.Resolved().State != tobari.ContextBootstrapConfigured {
				return fault.New(fault.KindContract, "invalid_context_report", "Context bootstrap configuration was not confirmed", false)
			}
			result = configured
			return nil
		})
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

// ConfigureShell changes one or more distinct allowlisted shell variable policies for one
// explicit or current Context. Host values are never read by the application;
// infrastructure resolves inherited values only when a session starts.
func (s *Service) ConfigureShell(
	ctx context.Context, intent operation.Intent, contextName string,
	changes []tobari.ContextShellEnvironmentSetting,
) (tobari.ContextReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextReport{}, err
	}
	if contextName != "" {
		if err := tobari.ValidateName(contextName); err != nil {
			return tobari.ContextReport{}, fault.Wrap(
				fault.KindInvalidInput, "invalid_context_name", "Context name is invalid", false, err,
				fault.NextAction{Command: "context list", Reason: "Choose a named Context from the local collection."},
			)
		}
	}
	if _, err := tobari.ApplyContextShellEnvironmentSettings(nil, changes); err != nil {
		return tobari.ContextReport{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_shell_environment", "Context shell environment setting is invalid", false, err,
			fault.NextAction{Command: "help config shell", Reason: "Choose an allowlisted variable and a valid source/value combination."},
		)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ContextShellTargetKind, ID: tobari.ContextShellTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ContextReport
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			configured, configureErr := s.runtime.ConfigureContextShell(actionContext, contextName, changes)
			if errors.Is(configureErr, tobari.ErrContextNotFound) {
				return fault.New(
					fault.KindNotFound, "context_not_found", "the named Context does not exist", false,
					fault.NextAction{Command: "context list", Reason: "Choose an existing Context."},
				)
			}
			if configureErr != nil {
				return fault.Wrap(
					fault.KindRejected, "config_shell_failed", "Context shell environment could not be updated", false,
					configureErr,
					fault.NextAction{Command: "context show", Reason: "Inspect the Context shell environment before retrying."},
				)
			}
			if err := validateConfiguredShellResult(configured, contextName, changes); err != nil {
				return fault.Wrap(
					fault.KindContract, "invalid_context_report", "Context report is invalid", false, err,
					fault.NextAction{Command: "context show", Reason: "Reconcile the confirmed Context shell setting."},
				)
			}
			result = configured
			return nil
		})
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

// ConfigureGit changes the atomic Git identity policy for one explicit or
// current Context. Host values are never read by the application; inherited
// values are resolved by infrastructure for a specific Workspace root.
func (s *Service) ConfigureGit(
	ctx context.Context, intent operation.Intent, contextName string,
	change tobari.ContextGitIdentitySetting,
) (tobari.ContextReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextReport{}, err
	}
	if contextName != "" {
		if err := tobari.ValidateName(contextName); err != nil {
			return tobari.ContextReport{}, fault.Wrap(
				fault.KindInvalidInput, "invalid_context_name", "Context name is invalid", false, err,
				fault.NextAction{Command: "context list", Reason: "Choose a named Context from the local collection."},
			)
		}
	}
	if err := change.Validate(true); err != nil {
		return tobari.ContextReport{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_git_identity", "Context Git identity setting is invalid", false, err,
			fault.NextAction{Command: "help config git", Reason: "Choose default, inherit, or a complete literal name and email pair."},
		)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ContextGitIdentityTargetKind, ID: tobari.ContextGitIdentityTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ContextReport
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			configured, configureErr := s.runtime.ConfigureContextGit(actionContext, contextName, change)
			if errors.Is(configureErr, tobari.ErrContextNotFound) {
				return fault.New(
					fault.KindNotFound, "context_not_found", "the named Context does not exist", false,
					fault.NextAction{Command: "context list", Reason: "Choose an existing Context."},
				)
			}
			if configureErr != nil {
				return fault.Wrap(
					fault.KindRejected, "config_git_failed", "Context Git identity could not be updated", false,
					configureErr,
					fault.NextAction{Command: "context show", Reason: "Inspect the Context Git identity before retrying."},
				)
			}
			if err := validateConfiguredGitResult(configured, contextName, change); err != nil {
				return fault.Wrap(
					fault.KindContract, "invalid_context_report", "Context report is invalid", false, err,
					fault.NextAction{Command: "context show", Reason: "Reconcile the confirmed Context Git identity setting."},
				)
			}
			result = configured
			return nil
		})
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func validateConfiguredContextResult(result tobari.ContextReport, task, contextName string) error {
	if err := validateSelectedContextResult(result, task, contextName); err != nil {
		return err
	}
	if result.Cluster != tobari.ContextClusterStatusNotApplicable {
		return errors.New("Context configuration report has an invalid cluster outcome")
	}
	return nil
}

func validateSelectedContextResult(result tobari.ContextReport, task, contextName string) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if result.Task != task {
		return errors.New("Context report task does not match the request")
	}
	if contextName != "" {
		if result.Name != contextName {
			return errors.New("Context report name does not match the request")
		}
	} else if !result.Active {
		return errors.New("Context report does not identify the active Context selected by omission")
	}
	return nil
}

func validateConfiguredShellResult(
	result tobari.ContextReport, contextName string, changes []tobari.ContextShellEnvironmentSetting,
) error {
	if err := validateConfiguredContextResult(result, tobari.TaskConfigShell, contextName); err != nil {
		return err
	}
	for _, change := range changes {
		matched := false
		for _, setting := range result.ShellEnvironment {
			if setting.Variable == change.Variable {
				matched = true
				if setting.Source != change.Source || !sameOptionalString(setting.Value, change.Value) {
					return errors.New("Context report shell setting does not match the configuration request")
				}
				break
			}
		}
		if !matched {
			return errors.New("Context report omits a configured shell setting")
		}
	}
	return nil
}

func validateConfiguredGitResult(
	result tobari.ContextReport, contextName string, change tobari.ContextGitIdentitySetting,
) error {
	if err := validateConfiguredContextResult(result, tobari.TaskConfigGit, contextName); err != nil {
		return err
	}
	if result.GitIdentity.Source != change.Source ||
		!sameOptionalString(result.GitIdentity.Name, change.Name) ||
		!sameOptionalString(result.GitIdentity.Email, change.Email) {
		return errors.New("Context report Git identity does not match the configuration request")
	}
	return nil
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// Service coordinates Context tasks without depending on the filesystem or
// Docker implementation.
type Service struct {
	runtime RuntimePort
	mutator *execution.Invoker
}

func New(runtime RuntimePort) *Service {
	return &Service{runtime: runtime, mutator: execution.New(ownedPolicy{})}
}

func (s *Service) requireRuntime() error {
	if s == nil || portcheck.IsNil(s.runtime) {
		return fault.New(fault.KindInternal, "missing_runtime", "Context runtime is not configured", false)
	}
	return nil
}

func (s *Service) List(ctx context.Context) (tobari.ContextListResult, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextListResult{}, err
	}
	result, err := s.runtime.ListContexts(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return tobari.ContextListResult{}, err
		}
		return tobari.ContextListResult{}, fault.Wrap(
			fault.KindInternal, "context_read_failed", "Context list could not be read", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the host Context stores."},
		)
	}
	if err := result.Validate(); err != nil {
		return tobari.ContextListResult{}, fault.Wrap(
			fault.KindContract, "invalid_context_list", "Context list is invalid", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the host Context stores."},
		)
	}
	return result, nil
}

func (s *Service) Show(ctx context.Context, name string) (tobari.ContextReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextReport{}, err
	}
	if name != "" {
		if err := tobari.ValidateName(name); err != nil {
			return tobari.ContextReport{}, fault.Wrap(
				fault.KindInvalidInput, "invalid_context_name", "Context name is invalid", false, err,
				fault.NextAction{Command: "context list", Reason: "Choose a named Context from the local collection."},
			)
		}
	}
	result, err := s.runtime.ShowContext(ctx, name)
	if err != nil {
		return tobari.ContextReport{}, s.readError(err)
	}
	if err := validateSelectedContextResult(result, tobari.TaskContextShow, name); err != nil {
		return tobari.ContextReport{}, fault.Wrap(
			fault.KindContract, "invalid_context_report", "Context report is invalid", false, err,
			fault.NextAction{Command: "context list", Reason: "Inspect the local Context collection."},
		)
	}
	return result, nil
}

func (s *Service) Create(
	ctx context.Context, intent operation.Intent, name, image string, mode tobari.ContextPolicyMode, sourceAccess tobari.ContextSourceAccess, selections ...string,
) (tobari.ContextReport, error) {
	presetOrigin := tobari.DefaultPolicyPresetOrigin
	nativeReadiness := tobari.ContextNativeReadinessEnabled
	if len(selections) > 2 {
		return tobari.ContextReport{}, fault.New(fault.KindInvalidInput, "invalid_context", "Context policy preset selection is invalid", false)
	}
	if len(selections) >= 1 {
		presetOrigin = selections[0]
	}
	if len(selections) == 2 {
		nativeReadiness = tobari.ContextNativeReadiness(selections[1])
	}
	return s.CreateWithComposition(ctx, intent, name, image, mode, sourceAccess, tobari.ContextCreateComposition{
		PolicyPresetOrigin: presetOrigin,
		NativeReadiness:    nativeReadiness,
	})
}

// CreateWithComposition creates one Context from a selected preset and an
// optional complete method-policy replacement collected by the human wizard.
func (s *Service) CreateWithComposition(
	ctx context.Context,
	intent operation.Intent,
	name, image string,
	mode tobari.ContextPolicyMode,
	sourceAccess tobari.ContextSourceAccess,
	composition tobari.ContextCreateComposition,
) (tobari.ContextReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := validateCreateInput(name, image, mode, sourceAccess); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := composition.Validate(); err != nil {
		return tobari.ContextReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_context", "Context policy preset selection is invalid", false, err)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ContextReport
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var created tobari.ContextReport
		var createErr error
		if runtime, ok := s.runtime.(composedContextRuntimePort); ok {
			created, createErr = runtime.CreateContextWithComposition(actionContext, name, image, mode, sourceAccess, composition.Clone())
		} else if composition.MethodPolicy == nil && composition.Bootstrap == nil {
			if runtime, ok := s.runtime.(policyPresetContextRuntimePort); ok {
				created, createErr = runtime.CreateContextWithPreset(actionContext, name, image, mode, sourceAccess, composition.PolicyPresetOrigin, composition.NativeReadiness)
			} else if composition.PolicyPresetOrigin == tobari.DefaultPolicyPresetOrigin && composition.NativeReadiness == tobari.ContextNativeReadinessEnabled {
				created, createErr = s.runtime.CreateContext(actionContext, name, image, mode, sourceAccess)
			} else {
				createErr = errors.New("policy preset store is unavailable")
			}
		} else if composition.PolicyPresetOrigin == tobari.DefaultPolicyPresetOrigin && composition.NativeReadiness == tobari.ContextNativeReadinessEnabled {
			createErr = errors.New("composed policy preset store is unavailable")
		} else {
			createErr = errors.New("composed policy preset store is unavailable")
		}
		if errors.Is(createErr, tobari.ErrContextExists) {
			return fault.New(
				fault.KindRejected, "context_exists", "the named Context already exists", false,
				fault.NextAction{Command: "context list", Reason: "List existing Contexts before choosing another name."},
			)
		}
		if createErr != nil {
			return fault.Wrap(fault.KindRejected, "context_create_failed", "Context could not be created", false, createErr,
				fault.NextAction{Command: "context list", Reason: "Inspect the local Context collection."})
		}
		contractErr := created.Validate()
		createdReadiness, readinessErr := tobari.ResolveContextNativeReadiness(created.NativeReadiness, created.PolicyPresetOrigin)
		if contractErr == nil {
			contractErr = readinessErr
		}
		if contractErr == nil && (created.Task != tobari.TaskContextCreate ||
			created.Name != name || created.SourceAccess != sourceAccess || created.PolicyPresetOrigin != composition.PolicyPresetOrigin || createdReadiness != composition.NativeReadiness) {
			contractErr = fmt.Errorf("created Context identity or source access does not match the request")
		}
		if contractErr == nil && composition.MethodPolicy != nil &&
			!reflect.DeepEqual(created.MethodPolicy, *composition.MethodPolicy) {
			contractErr = fmt.Errorf("created Context method policy does not match the request")
		}
		if contractErr == nil && composition.Bootstrap != nil &&
			!reflect.DeepEqual(created.Bootstrap.Resolved(), tobari.ContextBootstrapReportFrom(composition.Bootstrap)) {
			contractErr = fmt.Errorf("created Context bootstrap does not match the request")
		}
		if contractErr != nil {
			return fault.Wrap(
				fault.KindContract, "invalid_context_report", "Context report is invalid", false, contractErr,
				fault.NextAction{Command: "context list", Reason: "Reconcile the confirmed Context creation."},
			)
		}
		result = created
		return nil
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func (s *Service) Use(ctx context.Context, intent operation.Intent, name string) (tobari.ContextReport, error) {
	return s.UseWithProgress(ctx, intent, name, nil)
}

// Delete removes one non-current Context only after infrastructure proves no
// logical Workspace still binds its stable authority identity.
func (s *Service) Delete(
	ctx context.Context, intent operation.Intent, name string,
) (tobari.ContextDeleteResult, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.ContextDeleteResult{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_context_name", "Context name is invalid", false, err,
			fault.NextAction{Command: "context list", Reason: "Choose a named Context from the local collection."},
		)
	}
	runtime, ok := s.runtime.(contextDeleteRuntimePort)
	if !ok || portcheck.IsNil(runtime) {
		return tobari.ContextDeleteResult{}, fault.New(fault.KindInternal, "missing_runtime", "Context deletion runtime is not configured", false)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ID: tobari.ContextCatalogTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ContextDeleteResult
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			deleted, deleteErr := runtime.DeleteContext(actionContext, name)
			switch {
			case errors.Is(deleteErr, tobari.ErrContextNotFound):
				return fault.New(
					fault.KindNotFound, "context_not_found", "the named Context does not exist", false,
					fault.NextAction{Command: "context list", Reason: "Choose an existing Context."},
				)
			case errors.Is(deleteErr, tobari.ErrContextActive):
				return fault.New(
					fault.KindRejected, "context_is_current", "the current/default Context cannot be deleted", false,
					fault.NextAction{Command: "context use", Reason: "Select another Context before deleting this one."},
				)
			case errors.Is(deleteErr, tobari.ErrContextProtected):
				return fault.New(
					fault.KindRejected, "context_is_protected", "the foundational default Context cannot be deleted", false,
					fault.NextAction{Command: "context show", Reason: "Keep the default Context and delete only additional named Contexts."},
				)
			case errors.Is(deleteErr, tobari.ErrContextHasWorkspaces):
				return fault.New(
					fault.KindRejected, "context_has_workspaces", "the Context still owns one or more Workspaces", false,
					fault.NextAction{Command: "list", Reason: "Delete every Workspace bound to this Context first."},
				)
			case deleteErr != nil:
				return fault.Wrap(
					fault.KindRejected, "context_delete_failed", "Context could not be deleted", false, deleteErr,
					fault.NextAction{Command: "context list", Reason: "Inspect the Context collection before retrying."},
				)
			}
			if err := deleted.Validate(); err != nil || deleted.Name != name {
				if err == nil {
					err = fmt.Errorf("deleted Context identity does not match the request")
				}
				return fault.Wrap(
					fault.KindContract, "invalid_context_delete_result", "Context deletion result is invalid", false, err,
					fault.NextAction{Command: "context list", Reason: "Reconcile the Context collection after deletion."},
				)
			}
			result = deleted
			return nil
		})
	})
	if err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	return result, nil
}

// UseWithProgress selects a Context and optionally forwards the bounded
// cluster-reconcile lifecycle to the human CLI. The application owns the
// mutation and lifecycle lock; infrastructure owns the actual reconciliation.
func (s *Service) UseWithProgress(
	ctx context.Context, intent operation.Intent, name string, progress tobari.ClusterUpProgressSink,
) (tobari.ContextReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.ContextReport{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_context_name", "Context name is invalid", false, err,
			fault.NextAction{Command: "context list", Reason: "Choose a named Context from the local collection."},
		)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ContextTargetKind, ID: tobari.ActiveContextTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ContextReport
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			var used tobari.ContextReport
			var useErr error
			if runtime, ok := s.runtime.(contextUseProgressRuntimePort); ok {
				used, useErr = runtime.UseContextWithProgress(actionContext, name, progress)
			} else {
				used, useErr = s.runtime.UseContext(actionContext, name)
			}
			if errors.Is(useErr, tobari.ErrContextNotFound) {
				return fault.New(
					fault.KindNotFound, "context_not_found", "the named Context does not exist", false,
					fault.NextAction{Command: "context list", Reason: "Choose an existing Context or create it first."},
				)
			}
			if useErr != nil {
				return fault.Wrap(fault.KindRejected, "context_use_failed", "the current/default Context could not be updated", false, useErr,
					fault.NextAction{Command: "context show", Reason: "Inspect the current/default Context marker."})
			}
			result = used
			return nil
		})
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func (s *Service) withLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	if runtime, ok := s.runtime.(contextLifecycleRuntimePort); ok && !portcheck.IsNil(runtime) {
		return runtime.WithLifecycleLock(ctx, action)
	}
	return action(ctx)
}

// InitRuntime creates the current Context's recipe template without changing
// its selected image.
func (s *Service) InitRuntime(ctx context.Context, intent operation.Intent) (tobari.ContextReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextReport{}, err
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ContextRuntimeTargetKind, ParentID: tobari.ActiveContextRuntimeID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ContextReport
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		initialized, initErr := s.runtime.InitRuntime(actionContext)
		if errors.Is(initErr, tobari.ErrRuntimeRecipeExists) {
			return fault.New(
				fault.KindRejected, "runtime_recipe_exists", "the current Context already has a runtime recipe", false,
				fault.NextAction{Command: "context show", Reason: "Inspect the existing runtime recipe before editing it."},
			)
		}
		if initErr != nil {
			return fault.Wrap(
				fault.KindRejected, "runtime_init_failed", "the current Context runtime recipe could not be created", false,
				initErr,
				fault.NextAction{Command: "context show", Reason: "Inspect the current Context stores."},
			)
		}
		result = initialized
		return nil
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

// BuildRuntime builds and atomically selects the current Context's recipe.
func (s *Service) BuildRuntime(ctx context.Context, intent operation.Intent) (tobari.ContextReport, error) {
	return s.BuildRuntimeWithProgress(ctx, intent, nil, nil)
}

// BuildRuntimeWithProgress builds the current Context runtime while forwarding
// Docker diagnostics and bounded semantic stage events to presentation. The
// diagnostic writer is purpose-bound to this one build and is never retained.
func (s *Service) BuildRuntimeWithProgress(
	ctx context.Context,
	intent operation.Intent,
	diagnostics io.Writer,
	progress tobari.RuntimeBuildProgressSink,
) (tobari.ContextReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextReport{}, err
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ContextRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ContextReport
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var built tobari.ContextReport
		var buildErr error
		if runtime, ok := s.runtime.(contextRuntimeBuildProgressPort); ok && !portcheck.IsNil(runtime) {
			built, buildErr = runtime.BuildRuntimeWithProgress(actionContext, diagnostics, progress)
		} else {
			built, buildErr = s.runtime.BuildRuntime(actionContext)
		}
		if errors.Is(buildErr, tobari.ErrRuntimeRecipeMissing) {
			return fault.New(
				fault.KindInvalidInput, "runtime_recipe_missing", "the current Context has no runtime recipe", false,
				fault.NextAction{Command: "runtime init", Reason: "Create the current Context runtime template first."},
			)
		}
		if buildErr != nil {
			if structured, ok := fault.PublicCopy(buildErr); ok {
				return structured
			}
			return fault.Wrap(
				fault.KindRejected, "runtime_build_failed", "the current Context runtime could not be built", false,
				buildErr,
				fault.NextAction{Command: "context show", Reason: "Inspect the unchanged selected runtime and recipe state."},
			)
		}
		result = built
		return nil
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func validateCreateInput(name, image string, mode tobari.ContextPolicyMode, sourceAccess tobari.ContextSourceAccess) error {
	manifest := tobari.ContextManifest{
		SchemaVersion: tobari.ContextSchemaVersion, ID: "00000000-0000-7000-8000-000000000000", Name: name,
		AgentProfile: tobari.DefaultProfile, Image: image, PolicyMode: mode, SourceAccess: sourceAccess,
		PolicyPresetOrigin: tobari.DefaultPolicyPresetOrigin, PolicyPresetRevision: tobari.DefaultPolicyPresetRevision(),
	}
	if err := manifest.Validate(); err != nil {
		return fault.Wrap(
			fault.KindInvalidInput, "invalid_context", "Context definition is invalid", false, err,
			fault.NextAction{Command: "help context create", Reason: "Correct the Context name, image, policy mode, or source access."},
		)
	}
	return nil
}

func (s *Service) readError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, tobari.ErrContextNotFound) {
		return fault.New(fault.KindNotFound, "context_not_found", "the named Context does not exist", false,
			fault.NextAction{Command: "context list", Reason: "Choose an existing Context."})
	}
	return fault.Wrap(fault.KindInternal, "context_read_failed", "Context could not be read", false, err,
		fault.NextAction{Command: "doctor", Reason: "Inspect the host Context stores."})
}
