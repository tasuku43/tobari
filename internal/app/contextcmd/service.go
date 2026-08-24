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
	ListContexts(context.Context) (tobari.ManifestListResult, error)
	ShowContext(context.Context, string) (tobari.ManifestReport, error)
	CreateContext(context.Context, string, string, tobari.ManifestPolicyMode, tobari.ManifestSourceAccess) (tobari.ManifestReport, error)
	SetDefaultManifest(context.Context, string) (tobari.ManifestReport, error)
	ConfigureContextShell(context.Context, string, []tobari.ManifestShellEnvironmentSetting) (tobari.ManifestReport, error)
	ConfigureContextGit(context.Context, string, tobari.ManifestGitIdentitySetting) (tobari.ManifestReport, error)
	InitRuntime(context.Context) (tobari.ManifestReport, error)
	BuildRuntime(context.Context) (tobari.ManifestReport, error)
}

type contextUseProgressRuntimePort interface {
	SetDefaultManifestWithProgress(context.Context, string, tobari.ClusterUpProgressSink) (tobari.ManifestReport, error)
}

type composedContextRuntimePort interface {
	CreateContextWithComposition(context.Context, string, string, tobari.ManifestPolicyMode, tobari.ManifestSourceAccess, tobari.ManifestCreateComposition) (tobari.ManifestReport, error)
}

type contextCreateBaseRuntimePort interface {
	ManifestCopySnapshot(context.Context, string) (tobari.ManifestCopySnapshot, error)
}

type contextAWSBootstrapRuntimePort interface {
	PrepareContextAWSBootstrap(context.Context, string) (tobari.ManifestBootstrapSnapshot, error)
	PreviewContextAWSBootstrap(context.Context, string, string) (tobari.ManifestBootstrapPreview, error)
	ConfigureContextAWSBootstrap(context.Context, string, string, string, bool) (tobari.ManifestReport, error)
}

type contextEKSBootstrapRuntimePort interface {
	PrepareContextEKSBootstrap(context.Context, tobari.ManifestBootstrapSnapshot, string) (tobari.ManifestBootstrapSnapshot, error)
	PreviewContextEKSBootstrap(context.Context, string, string) (tobari.ManifestBootstrapPreview, error)
	ConfigureContextEKSBootstrap(context.Context, string, string, string, bool) (tobari.ManifestReport, error)
}

type contextAWSBootstrapDiscoveryRuntimePort interface {
	DiscoverContextAWSBootstraps(context.Context) (tobari.ManifestAWSBootstrapDiscovery, error)
}

type contextEKSBootstrapDiscoveryRuntimePort interface {
	DiscoverContextEKSBootstraps(context.Context, tobari.ManifestBootstrapSnapshot) (tobari.ManifestEKSBootstrapDiscovery, error)
}

// DiscoverAWSBootstraps is the read-only, wizard-owned candidate boundary. A
// rejected source is returned as typed state so the human can explicitly
// continue without bootstrap; infrastructure errors remain failures.
func (s *Service) DiscoverAWSBootstraps(ctx context.Context) (tobari.ManifestAWSBootstrapDiscovery, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestAWSBootstrapDiscovery{}, err
	}
	runtime, ok := s.runtime.(contextAWSBootstrapDiscoveryRuntimePort)
	if !ok {
		return tobari.ManifestAWSBootstrapDiscovery{}, fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest bootstrap discovery is unavailable", false)
	}
	result, err := runtime.DiscoverContextAWSBootstraps(ctx)
	if err != nil {
		return tobari.ManifestAWSBootstrapDiscovery{}, fault.Wrap(fault.KindUnavailable, "bootstrap_discovery_failed", "Host AWS bootstrap candidates could not be inspected", true, err, fault.NextAction{Command: "help config bootstrap aws", Reason: "Inspect the fixed host AWS shared-config boundary."})
	}
	if err := result.Validate(); err != nil {
		return tobari.ManifestAWSBootstrapDiscovery{}, fault.Wrap(fault.KindContract, "invalid_bootstrap_candidates", "AWS bootstrap candidates are invalid", false, err)
	}
	return result, nil
}

func (s *Service) DiscoverEKSBootstraps(ctx context.Context, aws tobari.ManifestBootstrapSnapshot) (tobari.ManifestEKSBootstrapDiscovery, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestEKSBootstrapDiscovery{}, err
	}
	if err := aws.Validate(); err != nil || aws.EKS != nil {
		return tobari.ManifestEKSBootstrapDiscovery{}, fault.New(fault.KindInvalidInput, "invalid_aws_bootstrap_candidate", "A reviewed AWS-only candidate is required before EKS discovery", false)
	}
	runtime, ok := s.runtime.(contextEKSBootstrapDiscoveryRuntimePort)
	if !ok {
		return tobari.ManifestEKSBootstrapDiscovery{}, fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest EKS bootstrap discovery is unavailable", false)
	}
	result, err := runtime.DiscoverContextEKSBootstraps(ctx, aws)
	if err != nil {
		return tobari.ManifestEKSBootstrapDiscovery{}, fault.Wrap(fault.KindUnavailable, "bootstrap_discovery_failed", "Host EKS bootstrap candidates could not be inspected", true, err, fault.NextAction{Command: "help config bootstrap kubernetes eks", Reason: "Inspect the fixed host kubeconfig boundary."})
	}
	if err := result.Validate(); err != nil || result.AWSRevision != aws.Revision {
		if err == nil {
			err = fmt.Errorf("EKS candidates do not bind the requested AWS semantic revision")
		}
		return tobari.ManifestEKSBootstrapDiscovery{}, fault.Wrap(fault.KindContract, "invalid_bootstrap_candidates", "EKS bootstrap candidates are invalid", false, err)
	}
	return result, nil
}

func (s *Service) PreviewAWSBootstrap(ctx context.Context, contextName, profile string) (tobari.ManifestBootstrapPreview, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestBootstrapPreview{}, err
	}
	runtime, ok := s.runtime.(contextAWSBootstrapRuntimePort)
	if !ok {
		return tobari.ManifestBootstrapPreview{}, fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest bootstrap runtime is unavailable", false)
	}
	preview, err := runtime.PreviewContextAWSBootstrap(ctx, contextName, profile)
	if errors.Is(err, tobari.ErrContextBootstrapNotConfigured) {
		return tobari.ManifestBootstrapPreview{}, fault.New(fault.KindNotFound, "bootstrap_not_configured", "the selected Workspace Manifest has no AWS bootstrap snapshot to refresh", false, fault.NextAction{Command: "help config bootstrap aws", Reason: "Configure an IAM Identity Center profile first."})
	}
	if errors.Is(err, tobari.ErrContextBootstrapDependency) {
		return tobari.ManifestBootstrapPreview{}, fault.New(fault.KindRejected, "bootstrap_dependency", "AWS profile cannot be replaced while the EKS adapter depends on it", false, fault.NextAction{Command: "config bootstrap kubernetes eks", Reason: "Remove the EKS adapter first with --remove."})
	}
	if err != nil {
		return tobari.ManifestBootstrapPreview{}, fault.Wrap(fault.KindRejected, "aws_bootstrap_source_rejected", "Host AWS IAM Identity Center configuration could not be normalized", false, err, fault.NextAction{Command: "help config bootstrap aws", Reason: "Use a strict IAM Identity Center profile without credentials, helpers, or unsupported directives."})
	}
	if err := preview.Validate(); err != nil {
		return tobari.ManifestBootstrapPreview{}, fault.Wrap(fault.KindContract, "invalid_bootstrap_preview", "AWS bootstrap preview is invalid", false, err)
	}
	return preview, nil
}

func (s *Service) PreviewEKSBootstrap(ctx context.Context, contextName, kubeContext string) (tobari.ManifestBootstrapPreview, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestBootstrapPreview{}, err
	}
	runtime, ok := s.runtime.(contextEKSBootstrapRuntimePort)
	if !ok {
		return tobari.ManifestBootstrapPreview{}, fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest EKS bootstrap runtime is unavailable", false)
	}
	preview, err := runtime.PreviewContextEKSBootstrap(ctx, contextName, kubeContext)
	if errors.Is(err, tobari.ErrContextBootstrapNotConfigured) {
		return tobari.ManifestBootstrapPreview{}, fault.New(fault.KindNotFound, "bootstrap_not_configured", "the selected Workspace Manifest has no compatible AWS or EKS bootstrap snapshot", false, fault.NextAction{Command: "help config bootstrap kubernetes eks", Reason: "Configure the AWS bootstrap and then select one EKS context."})
	}
	if err != nil {
		return tobari.ManifestBootstrapPreview{}, fault.Wrap(fault.KindRejected, "eks_bootstrap_source_rejected", "Host EKS kubeconfig target could not be normalized", false, err, fault.NextAction{Command: "help config bootstrap kubernetes eks", Reason: "Use an AWS CLI-generated EKS context bound to the same AWS profile."})
	}
	if err := preview.Validate(); err != nil {
		return tobari.ManifestBootstrapPreview{}, fault.Wrap(fault.KindContract, "invalid_bootstrap_preview", "EKS bootstrap preview is invalid", false, err)
	}
	return preview, nil
}

type contextDeleteRuntimePort interface {
	DeleteContext(context.Context, string) (tobari.ManifestDeleteResult, error)
}

type contextRuntimeBuildProgressPort interface {
	BuildRuntimeWithProgress(context.Context, io.Writer, tobari.RuntimeBuildProgressSink) (tobari.ManifestReport, error)
}

type contextRuntimeSelectionPort interface {
	SetContextRuntime(context.Context, string, string) (tobari.ManifestReport, error)
}

type contextLifecycleRuntimePort interface {
	WithLifecycleLock(context.Context, func(context.Context) error) error
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Effect == operation.EffectCreate &&
		intent.Target.Kind == tobari.ManifestCatalogTargetKind &&
		intent.Target.ParentID == tobari.ManifestCatalogTargetID && intent.Target.ID == "" {
		return nil
	}
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == tobari.ManifestTargetKind &&
		intent.Target.ID == tobari.DefaultManifestSelectionTargetID {
		return nil
	}
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == tobari.ManifestCatalogTargetKind &&
		intent.Target.ID == tobari.ManifestCatalogTargetID {
		return nil
	}
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == tobari.ManifestShellTargetKind &&
		intent.Target.ID == tobari.ManifestShellTargetID {
		return nil
	}
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == tobari.ManifestGitIdentityTargetKind &&
		intent.Target.ID == tobari.ManifestGitIdentityTargetID {
		return nil
	}
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == tobari.ManifestBootstrapTargetKind &&
		intent.Target.ID == tobari.ManifestBootstrapTargetID {
		return nil
	}
	if intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.ManifestRuntimeTargetKind &&
		intent.Target.ParentID == tobari.ActiveContextRuntimeID && intent.Target.ID == "" {
		return nil
	}
	if intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.ManifestRuntimeTargetKind &&
		intent.Target.ID == tobari.ActiveContextRuntimeID {
		return nil
	}
	if intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.ManifestRuntimeBindingTargetKind &&
		intent.Target.ID == tobari.ManifestRuntimeBindingTargetID {
		return nil
	}
	return fault.New(fault.KindRejected, "mutation_rejected", "Workspace Manifest mutation target is not owned by Tobari", false)
}

// PrepareAWSBootstrap normalizes one host AWS IAM Identity Center profile into
// a secret-free snapshot suitable for Context creation. The application never
// receives credential or token-cache bytes.
func (s *Service) PrepareAWSBootstrap(ctx context.Context, profile string) (tobari.ManifestBootstrapSnapshot, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestBootstrapSnapshot{}, err
	}
	if profile == "" {
		return tobari.ManifestBootstrapSnapshot{}, fault.New(fault.KindInvalidInput, "invalid_aws_bootstrap_profile", "AWS bootstrap profile is required", false, fault.NextAction{Command: "help config bootstrap aws", Reason: "Choose one IAM Identity Center profile from the host AWS shared config."})
	}
	runtime, ok := s.runtime.(contextAWSBootstrapRuntimePort)
	if !ok {
		return tobari.ManifestBootstrapSnapshot{}, fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest bootstrap runtime is unavailable", false, fault.NextAction{Command: "doctor", Reason: "Configure the Tobari runtime."})
	}
	snapshot, err := runtime.PrepareContextAWSBootstrap(ctx, profile)
	if err != nil {
		return tobari.ManifestBootstrapSnapshot{}, fault.Wrap(fault.KindRejected, "aws_bootstrap_source_rejected", "Host AWS IAM Identity Center configuration could not be normalized", false, err, fault.NextAction{Command: "help config bootstrap aws", Reason: "Use a strict IAM Identity Center profile without credentials, helpers, or unsupported directives."})
	}
	if err := snapshot.Validate(); err != nil {
		return tobari.ManifestBootstrapSnapshot{}, fault.Wrap(fault.KindContract, "invalid_bootstrap_snapshot", "AWS bootstrap snapshot is invalid", false, err)
	}
	return snapshot, nil
}

func (s *Service) PrepareEKSBootstrap(ctx context.Context, base tobari.ManifestBootstrapSnapshot, kubeContext string) (tobari.ManifestBootstrapSnapshot, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestBootstrapSnapshot{}, err
	}
	if kubeContext == "" {
		return tobari.ManifestBootstrapSnapshot{}, fault.New(fault.KindInvalidInput, "invalid_eks_bootstrap_context", "Kubernetes context is required", false, fault.NextAction{Command: "help manifest create", Reason: "Choose one AWS CLI-generated EKS context from the host kubeconfig."})
	}
	runtime, ok := s.runtime.(contextEKSBootstrapRuntimePort)
	if !ok {
		return tobari.ManifestBootstrapSnapshot{}, fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest EKS bootstrap runtime is unavailable", false)
	}
	snapshot, err := runtime.PrepareContextEKSBootstrap(ctx, base, kubeContext)
	if err != nil {
		return tobari.ManifestBootstrapSnapshot{}, fault.Wrap(fault.KindRejected, "eks_bootstrap_source_rejected", "Host EKS kubeconfig target could not be normalized", false, err, fault.NextAction{Command: "help config bootstrap kubernetes eks", Reason: "Use an AWS CLI-generated EKS context bound to the selected AWS profile."})
	}
	return snapshot, nil
}

// ConfigureAWSBootstrap replaces, refreshes, or removes the AWS recipe used by
// future Workspaces. Existing Workspace homes are outside this mutation.
func (s *Service) ConfigureAWSBootstrap(ctx context.Context, intent operation.Intent, contextName, profile, expectedRevision string, remove bool) (tobari.ManifestReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestReport{}, err
	}
	if contextName != "" {
		if err := tobari.ValidateName(contextName); err != nil {
			return tobari.ManifestReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_manifest_name", "Workspace Manifest name is invalid", false, err, fault.NextAction{Command: "manifest list", Reason: "Choose a named Workspace Manifest from the local collection."})
		}
	}
	if remove && profile != "" {
		return tobari.ManifestReport{}, fault.New(fault.KindInvalidInput, "invalid_aws_bootstrap_change", "AWS bootstrap removal cannot select a profile", false, fault.NextAction{Command: "help config bootstrap aws", Reason: "Choose configure/refresh or remove."})
	}
	runtime, ok := s.runtime.(contextAWSBootstrapRuntimePort)
	if !ok {
		return tobari.ManifestReport{}, fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest bootstrap runtime is unavailable", false, fault.NextAction{Command: "doctor", Reason: "Configure the Tobari runtime."})
	}
	request := execution.Request{Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite, ExpectedTarget: operation.TargetRef{Kind: tobari.ManifestBootstrapTargetKind, ID: tobari.ManifestBootstrapTargetID}, ExpectedImpact: intent.Impact}
	var result tobari.ManifestReport
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		configured, configureErr := runtime.ConfigureContextAWSBootstrap(actionContext, contextName, profile, expectedRevision, remove)
		switch {
		case errors.Is(configureErr, tobari.ErrContextNotFound):
			return fault.New(fault.KindNotFound, "manifest_not_found", "the named Workspace Manifest does not exist", false, fault.NextAction{Command: "manifest list", Reason: "Choose an existing Workspace Manifest."})
		case errors.Is(configureErr, tobari.ErrContextBootstrapNotConfigured):
			return fault.New(fault.KindNotFound, "bootstrap_not_configured", "the selected Workspace Manifest has no AWS bootstrap snapshot to refresh", false, fault.NextAction{Command: "help config bootstrap aws", Reason: "Configure an IAM Identity Center profile first."})
		case errors.Is(configureErr, tobari.ErrContextBootstrapSourceChanged):
			return fault.New(fault.KindRejected, "bootstrap_source_changed", "Host AWS configuration changed during review; no Workspace Manifest change was applied", true, fault.NextAction{Command: "config bootstrap aws", Reason: "Review a fresh semantic diff before applying."})
		case errors.Is(configureErr, tobari.ErrContextBootstrapDependency):
			return fault.New(fault.KindRejected, "bootstrap_dependency", "AWS bootstrap cannot be removed while the EKS adapter depends on it", false, fault.NextAction{Command: "config bootstrap kubernetes eks", Reason: "Remove the EKS adapter first with --remove."})
		case configureErr != nil:
			return fault.Wrap(fault.KindRejected, "config_bootstrap_failed", "Workspace Manifest AWS bootstrap could not be changed", false, configureErr, fault.NextAction{Command: "manifest show", Reason: "Inspect the current future-Workspace bootstrap recipe."})
		}
		if configured.Task != tobari.TaskConfigBootstrapAWS || configured.ManifestState != tobari.ManifestObservationPersisted {
			return fault.New(fault.KindContract, "invalid_manifest_report", "Workspace Manifest bootstrap report is invalid", false, fault.NextAction{Command: "manifest show", Reason: "Reconcile the confirmed Workspace Manifest bootstrap change."})
		}
		if remove && configured.Bootstrap.Resolved().State != tobari.ManifestBootstrapNotConfigured {
			return fault.New(fault.KindContract, "invalid_manifest_report", "Workspace Manifest bootstrap removal was not confirmed", false)
		}
		if !remove && configured.Bootstrap.Resolved().State != tobari.ManifestBootstrapConfigured {
			return fault.New(fault.KindContract, "invalid_manifest_report", "Workspace Manifest bootstrap configuration was not confirmed", false)
		}
		result = configured
		return nil
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

func (s *Service) ConfigureEKSBootstrap(ctx context.Context, intent operation.Intent, contextName, kubeContext, expectedRevision string, remove bool) (tobari.ManifestReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestReport{}, err
	}
	if contextName != "" {
		if err := tobari.ValidateName(contextName); err != nil {
			return tobari.ManifestReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_manifest_name", "Workspace Manifest name is invalid", false, err, fault.NextAction{Command: "manifest list", Reason: "Choose a named Workspace Manifest from the local collection."})
		}
	}
	if remove && kubeContext != "" {
		return tobari.ManifestReport{}, fault.New(fault.KindInvalidInput, "invalid_eks_bootstrap_change", "EKS bootstrap removal cannot select a Kubernetes context", false, fault.NextAction{Command: "help config bootstrap kubernetes eks", Reason: "Choose configure, refresh, or remove."})
	}
	runtime, ok := s.runtime.(contextEKSBootstrapRuntimePort)
	if !ok {
		return tobari.ManifestReport{}, fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest EKS bootstrap runtime is unavailable", false)
	}
	request := execution.Request{Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite, ExpectedTarget: operation.TargetRef{Kind: tobari.ManifestBootstrapTargetKind, ID: tobari.ManifestBootstrapTargetID}, ExpectedImpact: intent.Impact}
	var result tobari.ManifestReport
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			configured, configureErr := runtime.ConfigureContextEKSBootstrap(actionContext, contextName, kubeContext, expectedRevision, remove)
			switch {
			case errors.Is(configureErr, tobari.ErrContextNotFound):
				return fault.New(fault.KindNotFound, "manifest_not_found", "the named Workspace Manifest does not exist", false, fault.NextAction{Command: "manifest list", Reason: "Choose an existing Workspace Manifest."})
			case errors.Is(configureErr, tobari.ErrContextBootstrapNotConfigured):
				return fault.New(fault.KindNotFound, "bootstrap_not_configured", "the selected Workspace Manifest has no compatible AWS or EKS bootstrap snapshot", false, fault.NextAction{Command: "help config bootstrap kubernetes eks", Reason: "Configure AWS bootstrap before EKS, or select EKS before refresh/remove."})
			case errors.Is(configureErr, tobari.ErrContextBootstrapSourceChanged):
				return fault.New(fault.KindRejected, "bootstrap_source_changed", "Host kubeconfig changed during review; no Workspace Manifest change was applied", true, fault.NextAction{Command: "config bootstrap kubernetes eks", Reason: "Review a fresh semantic diff before applying."})
			case configureErr != nil:
				return fault.Wrap(fault.KindRejected, "config_bootstrap_failed", "Workspace Manifest EKS bootstrap could not be changed", false, configureErr, fault.NextAction{Command: "manifest show", Reason: "Inspect the current future-Workspace bootstrap recipe."})
			}
			if configured.Task != tobari.TaskConfigBootstrapEKS || configured.ManifestState != tobari.ManifestObservationPersisted {
				return fault.New(fault.KindContract, "invalid_manifest_report", "Workspace Manifest EKS bootstrap report is invalid", false)
			}
			if remove && configured.Bootstrap.EKSContext != "" {
				return fault.New(fault.KindContract, "invalid_manifest_report", "Workspace Manifest EKS bootstrap removal was not confirmed", false)
			}
			if !remove && configured.Bootstrap.EKSContext == "" {
				return fault.New(fault.KindContract, "invalid_manifest_report", "Workspace Manifest EKS bootstrap configuration was not confirmed", false)
			}
			result = configured
			return nil
		})
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

// ConfigureShell changes one or more distinct allowlisted shell variable policies for one
// explicit or current Context. Host values are never read by the application;
// infrastructure resolves inherited values only when a session starts.
func (s *Service) ConfigureShell(
	ctx context.Context, intent operation.Intent, contextName string,
	changes []tobari.ManifestShellEnvironmentSetting,
) (tobari.ManifestReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestReport{}, err
	}
	if contextName != "" {
		if err := tobari.ValidateName(contextName); err != nil {
			return tobari.ManifestReport{}, fault.Wrap(
				fault.KindInvalidInput, "invalid_manifest_name", "Workspace Manifest name is invalid", false, err,
				fault.NextAction{Command: "manifest list", Reason: "Choose a named Workspace Manifest from the local collection."},
			)
		}
	}
	if _, err := tobari.ApplyContextShellEnvironmentSettings(nil, changes); err != nil {
		return tobari.ManifestReport{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_shell_environment", "Workspace Manifest shell environment setting is invalid", false, err,
			fault.NextAction{Command: "help config shell", Reason: "Choose an allowlisted variable and a valid source/value combination."},
		)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ManifestShellTargetKind, ID: tobari.ManifestShellTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ManifestReport
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			configured, configureErr := s.runtime.ConfigureContextShell(actionContext, contextName, changes)
			if errors.Is(configureErr, tobari.ErrContextNotFound) {
				return fault.New(
					fault.KindNotFound, "manifest_not_found", "the named Workspace Manifest does not exist", false,
					fault.NextAction{Command: "manifest list", Reason: "Choose an existing Workspace Manifest."},
				)
			}
			if configureErr != nil {
				return fault.Wrap(
					fault.KindRejected, "config_shell_failed", "Workspace Manifest shell environment could not be updated", false,
					configureErr,
					fault.NextAction{Command: "manifest show", Reason: "Inspect the Workspace Manifest shell environment before retrying."},
				)
			}
			if err := validateConfiguredShellResult(configured, contextName, changes); err != nil {
				return fault.Wrap(
					fault.KindContract, "invalid_manifest_report", "Workspace Manifest report is invalid", false, err,
					fault.NextAction{Command: "manifest show", Reason: "Reconcile the confirmed Workspace Manifest shell setting."},
				)
			}
			result = configured
			return nil
		})
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

// ConfigureGit changes the atomic Git identity policy for one explicit or
// current Context. Host values are never read by the application; inherited
// values are resolved by infrastructure for a specific Workspace root.
func (s *Service) ConfigureGit(
	ctx context.Context, intent operation.Intent, contextName string,
	change tobari.ManifestGitIdentitySetting,
) (tobari.ManifestReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestReport{}, err
	}
	if contextName != "" {
		if err := tobari.ValidateName(contextName); err != nil {
			return tobari.ManifestReport{}, fault.Wrap(
				fault.KindInvalidInput, "invalid_manifest_name", "Workspace Manifest name is invalid", false, err,
				fault.NextAction{Command: "manifest list", Reason: "Choose a named Workspace Manifest from the local collection."},
			)
		}
	}
	if err := change.Validate(true); err != nil {
		return tobari.ManifestReport{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_git_identity", "Workspace Manifest Git identity setting is invalid", false, err,
			fault.NextAction{Command: "help config git", Reason: "Choose default, inherit, or a complete literal name and email pair."},
		)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ManifestGitIdentityTargetKind, ID: tobari.ManifestGitIdentityTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ManifestReport
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			configured, configureErr := s.runtime.ConfigureContextGit(actionContext, contextName, change)
			if errors.Is(configureErr, tobari.ErrContextNotFound) {
				return fault.New(
					fault.KindNotFound, "manifest_not_found", "the named Workspace Manifest does not exist", false,
					fault.NextAction{Command: "manifest list", Reason: "Choose an existing Workspace Manifest."},
				)
			}
			if configureErr != nil {
				return fault.Wrap(
					fault.KindRejected, "config_git_failed", "Workspace Manifest Git identity could not be updated", false,
					configureErr,
					fault.NextAction{Command: "manifest show", Reason: "Inspect the Workspace Manifest Git identity before retrying."},
				)
			}
			if err := validateConfiguredGitResult(configured, contextName, change); err != nil {
				return fault.Wrap(
					fault.KindContract, "invalid_manifest_report", "Workspace Manifest report is invalid", false, err,
					fault.NextAction{Command: "manifest show", Reason: "Reconcile the confirmed Workspace Manifest Git identity setting."},
				)
			}
			result = configured
			return nil
		})
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

func validateConfiguredContextResult(result tobari.ManifestReport, task, contextName string) error {
	if err := validateSelectedContextResult(result, task, contextName); err != nil {
		return err
	}
	if result.Cluster != tobari.ManifestClusterStatusNotApplicable {
		return errors.New("Workspace Manifest configuration report has an invalid cluster outcome")
	}
	return nil
}

func validateSelectedContextResult(result tobari.ManifestReport, task, contextName string) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if result.Task != task {
		return errors.New("Workspace Manifest report task does not match the request")
	}
	if contextName != "" {
		if result.Name != contextName {
			return errors.New("Workspace Manifest report name does not match the request")
		}
	} else if !result.Default {
		return errors.New("Workspace Manifest report does not identify the active Workspace Manifest selected by omission")
	}
	return nil
}

func validateConfiguredShellResult(
	result tobari.ManifestReport, contextName string, changes []tobari.ManifestShellEnvironmentSetting,
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
					return errors.New("Workspace Manifest report shell setting does not match the configuration request")
				}
				break
			}
		}
		if !matched {
			return errors.New("Workspace Manifest report omits a configured shell setting")
		}
	}
	return nil
}

func validateConfiguredGitResult(
	result tobari.ManifestReport, contextName string, change tobari.ManifestGitIdentitySetting,
) error {
	if err := validateConfiguredContextResult(result, tobari.TaskConfigGit, contextName); err != nil {
		return err
	}
	if result.GitIdentity.Source != change.Source ||
		!sameOptionalString(result.GitIdentity.Name, change.Name) ||
		!sameOptionalString(result.GitIdentity.Email, change.Email) {
		return errors.New("Workspace Manifest report Git identity does not match the configuration request")
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
		return fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest runtime is not configured", false)
	}
	return nil
}

func (s *Service) List(ctx context.Context) (tobari.ManifestListResult, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestListResult{}, err
	}
	result, err := s.runtime.ListContexts(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return tobari.ManifestListResult{}, err
		}
		return tobari.ManifestListResult{}, fault.Wrap(
			fault.KindInternal, "manifest_read_failed", "Workspace Manifest list could not be read", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the host Workspace Manifest stores."},
		)
	}
	if err := result.Validate(); err != nil {
		return tobari.ManifestListResult{}, fault.Wrap(
			fault.KindContract, "invalid_manifest_list", "Workspace Manifest list is invalid", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the host Workspace Manifest stores."},
		)
	}
	return result, nil
}

func (s *Service) Show(ctx context.Context, name string) (tobari.ManifestReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestReport{}, err
	}
	if name != "" {
		if err := tobari.ValidateName(name); err != nil {
			return tobari.ManifestReport{}, fault.Wrap(
				fault.KindInvalidInput, "invalid_manifest_name", "Workspace Manifest name is invalid", false, err,
				fault.NextAction{Command: "manifest list", Reason: "Choose a named Workspace Manifest from the local collection."},
			)
		}
	}
	result, err := s.runtime.ShowContext(ctx, name)
	if err != nil {
		return tobari.ManifestReport{}, s.readError(err)
	}
	if err := validateSelectedContextResult(result, tobari.TaskManifestShow, name); err != nil {
		return tobari.ManifestReport{}, fault.Wrap(
			fault.KindContract, "invalid_manifest_report", "Workspace Manifest report is invalid", false, err,
			fault.NextAction{Command: "manifest list", Reason: "Inspect the local Workspace Manifest collection."},
		)
	}
	return result, nil
}

// CopySnapshot reads one exact Workspace Manifest revision for independent
// initialization. The returned revision must be presented back at creation so
// mutable defaults cannot change silently during review.
func (s *Service) CopySnapshot(ctx context.Context, name string) (tobari.ManifestCopySnapshot, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestCopySnapshot{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.ManifestCopySnapshot{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_manifest_copy_source", "Workspace Manifest copy source is invalid", false, err,
			fault.NextAction{Command: "manifest list", Reason: "Choose one existing Workspace Manifest as the copy source."},
		)
	}
	runtime, ok := s.runtime.(contextCreateBaseRuntimePort)
	if !ok {
		return tobari.ManifestCopySnapshot{}, fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest copy-source reader is unavailable", false)
	}
	base, err := runtime.ManifestCopySnapshot(ctx, name)
	if errors.Is(err, tobari.ErrContextNotFound) {
		return tobari.ManifestCopySnapshot{}, fault.New(
			fault.KindNotFound, "manifest_copy_source_not_found", "Workspace Manifest copy source does not exist", false,
			fault.NextAction{Command: "manifest list", Reason: "Choose one existing Workspace Manifest as the copy source."},
		)
	}
	if err != nil {
		return tobari.ManifestCopySnapshot{}, s.readError(err)
	}
	if err := base.Validate(); err != nil || base.Name != name {
		if err == nil {
			err = fmt.Errorf("Workspace Manifest copy-source name does not match the request")
		}
		return tobari.ManifestCopySnapshot{}, fault.Wrap(
			fault.KindContract, "invalid_manifest_copy_source", "Workspace Manifest copy source is invalid", false, err,
			fault.NextAction{Command: "manifest list", Reason: "Inspect the local Workspace Manifest collection."},
		)
	}
	return base.Clone(), nil
}

func (s *Service) Create(
	ctx context.Context, intent operation.Intent, name, image string, mode tobari.ManifestPolicyMode, sourceAccess tobari.ManifestSourceAccess, selections ...string,
) (tobari.ManifestReport, error) {
	nativeReadiness := tobari.ManifestNativeReadinessEnabled
	if len(selections) > 1 {
		return tobari.ManifestReport{}, fault.New(fault.KindInvalidInput, "invalid_context", "Workspace Manifest native readiness selection is invalid", false)
	}
	if len(selections) == 1 {
		nativeReadiness = tobari.ManifestNativeReadiness(selections[0])
	}
	return s.CreateWithComposition(ctx, intent, name, image, mode, sourceAccess, tobari.ManifestCreateComposition{
		NativeReadiness: nativeReadiness,
	})
}

// CreateWithComposition creates one Context from the fixed built-in baseline
// and an optional complete method-policy replacement collected by the wizard.
func (s *Service) CreateWithComposition(
	ctx context.Context,
	intent operation.Intent,
	name, image string,
	mode tobari.ManifestPolicyMode,
	sourceAccess tobari.ManifestSourceAccess,
	composition tobari.ManifestCreateComposition,
) (tobari.ManifestReport, error) {
	return s.createWithComposition(ctx, intent, name, image, mode, sourceAccess, composition, false)
}

// CreateFirstWithComposition creates through the canonical Context mutation
// only while the collection is still the exact known-empty first-use state.
// The revalidation shares the lifecycle lock with creation so concurrent
// Context changes cannot be silently adopted or overwritten.
func (s *Service) CreateFirstWithComposition(
	ctx context.Context,
	intent operation.Intent,
	name, image string,
	mode tobari.ManifestPolicyMode,
	sourceAccess tobari.ManifestSourceAccess,
	composition tobari.ManifestCreateComposition,
) (tobari.ManifestReport, error) {
	return s.createWithComposition(ctx, intent, name, image, mode, sourceAccess, composition, true)
}

func (s *Service) createWithComposition(
	ctx context.Context,
	intent operation.Intent,
	name, image string,
	mode tobari.ManifestPolicyMode,
	sourceAccess tobari.ManifestSourceAccess,
	composition tobari.ManifestCreateComposition,
	requireEmpty bool,
) (tobari.ManifestReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestReport{}, err
	}
	if err := validateCreateInput(name, image, mode, sourceAccess); err != nil {
		return tobari.ManifestReport{}, err
	}
	if err := composition.Validate(); err != nil {
		return tobari.ManifestReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_context", "Workspace Manifest policy selection is invalid", false, err)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ParentID: tobari.ManifestCatalogTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ManifestReport
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		if requireEmpty {
			observed, observeErr := s.runtime.ListContexts(lifecycleContext)
			if observeErr != nil {
				return fault.Wrap(fault.KindInternal, "manifest_read_failed", "Workspace Manifest collection could not be revalidated", false, observeErr,
					fault.NextAction{Command: "manifest list", Reason: "Inspect the Workspace Manifest collection."})
			}
			if validateErr := observed.Validate(); validateErr != nil {
				return fault.Wrap(fault.KindContract, "invalid_manifest_list", "Workspace Manifest collection revalidation is invalid", false, validateErr,
					fault.NextAction{Command: "manifest list", Reason: "Inspect the Workspace Manifest collection."})
			}
			if observed.ManifestState != tobari.ManifestObservationAbsent || observed.DefaultManifest != "" || observed.DefaultManifestID != "" || len(observed.Items) != 0 {
				return fault.WithClassification(
					fault.New(fault.KindRejected, "manifest_collection_changed", "Workspace Manifest collection changed during first-use review", true,
						fault.NextAction{Command: "manifest list", Reason: "Inspect the Workspace Manifest collection before retrying Tobari."}),
					fault.PhasePrecondition, fault.ChangeNone,
				)
			}
		}
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			var created tobari.ManifestReport
			var createErr error
			if runtime, ok := s.runtime.(composedContextRuntimePort); ok {
				created, createErr = runtime.CreateContextWithComposition(actionContext, name, image, mode, sourceAccess, composition.Clone())
			} else if composition.MethodPolicy == nil && composition.Bootstrap == nil && composition.NativeReadiness == tobari.ManifestNativeReadinessEnabled {
				created, createErr = s.runtime.CreateContext(actionContext, name, image, mode, sourceAccess)
			} else {
				createErr = errors.New("Workspace Manifest policy composition is unavailable")
			}
			if errors.Is(createErr, tobari.ErrContextExists) {
				return fault.New(
					fault.KindRejected, "manifest_exists", "the named Workspace Manifest already exists", false,
					fault.NextAction{Command: "manifest list", Reason: "List existing Workspace Manifests before choosing another name."},
				)
			}
			if errors.Is(createErr, tobari.ErrRuntimeNotFound) {
				return fault.New(fault.KindNotFound, "runtime_not_found", "the selected Runtime does not exist", false,
					fault.NextAction{Command: "runtime list", Reason: "Choose an existing Runtime."})
			}
			if errors.Is(createErr, tobari.ErrRuntimeNotReady) {
				return fault.New(fault.KindRejected, "runtime_revision_not_ready", "the selected Runtime revision does not exist", false,
					fault.NextAction{Command: "review runtimes", Reason: "Choose an existing successful revision."})
			}
			if errors.Is(createErr, tobari.ErrManifestCopySourceChanged) || (errors.Is(createErr, tobari.ErrContextNotFound) && composition.CopyFrom != nil) {
				return fault.New(fault.KindRejected, "manifest_copy_source_changed", "Workspace Manifest copy source changed during review", true,
					fault.NextAction{Command: "manifest list", Reason: "Review the copy source's current revision before creating."})
			}
			if createErr != nil {
				return fault.Wrap(fault.KindRejected, "manifest_create_failed", "Workspace Manifest could not be created", false, createErr,
					fault.NextAction{Command: "manifest list", Reason: "Inspect the local Workspace Manifest collection."})
			}
			contractErr := created.Validate()
			createdReadiness, readinessErr := tobari.ResolveContextNativeReadiness(created.NativeReadiness)
			if contractErr == nil {
				contractErr = readinessErr
			}
			if contractErr == nil && (created.Task != tobari.TaskManifestCreate ||
				created.Name != name || created.PolicyMode != mode || created.SourceAccess != sourceAccess || createdReadiness != composition.NativeReadiness) {
				contractErr = fmt.Errorf("created Workspace Manifest identity or Boundary does not match the request")
			}
			if contractErr == nil && composition.MethodPolicy != nil &&
				!reflect.DeepEqual(created.MethodPolicy, *composition.MethodPolicy) {
				contractErr = fmt.Errorf("created Workspace Manifest method policy does not match the request")
			}
			expectedBootstrap := composition.Bootstrap
			if expectedBootstrap == nil && composition.CopyFrom != nil {
				expectedBootstrap = composition.CopyFrom.Bootstrap
			}
			if contractErr == nil &&
				!reflect.DeepEqual(created.Bootstrap.Resolved(), tobari.ManifestBootstrapReportFrom(expectedBootstrap)) {
				contractErr = fmt.Errorf("created Workspace Manifest bootstrap does not match the request")
			}
			if contractErr == nil && composition.CopyFrom != nil && (created.ID == composition.CopyFrom.ID ||
				!reflect.DeepEqual(created.ShellEnvironment, composition.CopyFrom.ShellEnvironment) ||
				!reflect.DeepEqual(created.GitIdentity, composition.CopyFrom.GitIdentity)) {
				contractErr = fmt.Errorf("created Workspace Manifest copied invalid Base identity or Workspace defaults")
			}
			if contractErr == nil {
				createdRuntime, runtimeErr := created.Runtime.Selection()
				requestedRuntime, ordinal, requestedErr := tobari.ParseRuntimeSelection(composition.RuntimeSelection)
				if runtimeErr != nil || requestedErr != nil {
					contractErr = errors.Join(runtimeErr, requestedErr)
				} else {
					expectedRuntime := fmt.Sprintf("%s@%d", requestedRuntime, ordinal)
					if createdRuntime != expectedRuntime {
						contractErr = fmt.Errorf("created Workspace Manifest Runtime does not match the request")
					}
				}
			}
			if contractErr != nil {
				return fault.Wrap(
					fault.KindContract, "invalid_manifest_report", "Workspace Manifest report is invalid", false, contractErr,
					fault.NextAction{Command: "manifest list", Reason: "Reconcile the confirmed Workspace Manifest creation."},
				)
			}
			result = created
			return nil
		})
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

func (s *Service) Use(ctx context.Context, intent operation.Intent, name string) (tobari.ManifestReport, error) {
	return s.SetDefaultWithProgress(ctx, intent, name, nil)
}

// SetRuntime explicitly pins one Context to one ready Runtime revision.
func (s *Service) SetRuntime(ctx context.Context, intent operation.Intent, contextName, selection string) (tobari.ManifestReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestReport{}, err
	}
	if contextName != "" {
		if err := tobari.ValidateName(contextName); err != nil {
			return tobari.ManifestReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_manifest_name", "Workspace Manifest name is invalid", false, err)
		}
	}
	if _, _, err := tobari.ParseRuntimeSelection(selection); err != nil {
		return tobari.ManifestReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_runtime_selection", "Runtime selection is invalid", false, err, fault.NextAction{Command: "review runtimes", Reason: "Choose standard or one ready name@ordinal revision."})
	}
	runtime, ok := s.runtime.(contextRuntimeSelectionPort)
	if !ok || portcheck.IsNil(runtime) {
		return tobari.ManifestReport{}, fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest Runtime selection is unavailable", false)
	}
	request := execution.Request{Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ManifestRuntimeBindingTargetKind, ID: tobari.ManifestRuntimeBindingTargetID}, ExpectedImpact: intent.Impact}
	var result tobari.ManifestReport
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			updated, err := runtime.SetContextRuntime(actionContext, contextName, selection)
			switch {
			case errors.Is(err, tobari.ErrContextNotFound):
				return fault.New(fault.KindNotFound, "manifest_not_found", "the named Workspace Manifest does not exist", false, fault.NextAction{Command: "manifest list", Reason: "Choose an existing Workspace Manifest."})
			case errors.Is(err, tobari.ErrRuntimeNotFound):
				return fault.New(fault.KindNotFound, "runtime_not_found", "the named Runtime does not exist", false, fault.NextAction{Command: "runtime list", Reason: "Choose an existing Runtime."})
			case errors.Is(err, tobari.ErrRuntimeNotReady):
				return fault.New(fault.KindRejected, "runtime_revision_not_ready", "the selected Runtime revision does not exist", false, fault.NextAction{Command: "review runtimes", Reason: "Choose an existing successful revision."})
			case err != nil:
				return fault.Wrap(fault.KindRejected, "manifest_runtime_set_failed", "Workspace Manifest Runtime could not be changed", false, err, fault.NextAction{Command: "manifest show", Reason: "Inspect the unchanged Workspace Manifest Runtime binding."})
			}
			if err := updated.Validate(); err != nil || updated.Task != tobari.TaskManifestRuntimeSet {
				if err == nil {
					err = fmt.Errorf("updated Workspace Manifest Runtime task does not match the request")
				}
				return fault.Wrap(fault.KindContract, "invalid_manifest_report", "Workspace Manifest report is invalid", false, err)
			}
			result = updated
			return nil
		})
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

// Delete removes one non-default Workspace Manifest only after infrastructure proves no
// logical Workspace still binds its stable authority identity.
func (s *Service) Delete(
	ctx context.Context, intent operation.Intent, name string,
) (tobari.ManifestDeleteResult, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestDeleteResult{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.ManifestDeleteResult{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_manifest_name", "Workspace Manifest name is invalid", false, err,
			fault.NextAction{Command: "manifest list", Reason: "Choose a named Workspace Manifest from the local collection."},
		)
	}
	runtime, ok := s.runtime.(contextDeleteRuntimePort)
	if !ok || portcheck.IsNil(runtime) {
		return tobari.ManifestDeleteResult{}, fault.New(fault.KindInternal, "missing_runtime", "Workspace Manifest deletion runtime is not configured", false)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ID: tobari.ManifestCatalogTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ManifestDeleteResult
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			deleted, deleteErr := runtime.DeleteContext(actionContext, name)
			switch {
			case errors.Is(deleteErr, tobari.ErrContextNotFound):
				return fault.New(
					fault.KindNotFound, "manifest_not_found", "the named Workspace Manifest does not exist", false,
					fault.NextAction{Command: "manifest list", Reason: "Choose an existing Workspace Manifest."},
				)
			case errors.Is(deleteErr, tobari.ErrContextActive):
				return fault.New(
					fault.KindRejected, "manifest_is_default", "the default Workspace Manifest cannot be deleted", false,
					fault.NextAction{Command: "manifest default set", Reason: "Select another Workspace Manifest before deleting this one."},
				)
			case errors.Is(deleteErr, tobari.ErrContextProtected):
				return fault.New(
					fault.KindRejected, "manifest_is_protected", "the foundational default Workspace Manifest cannot be deleted", false,
					fault.NextAction{Command: "manifest show", Reason: "Keep the default Workspace Manifest and delete only additional named Workspace Manifests."},
				)
			case errors.Is(deleteErr, tobari.ErrContextHasWorkspaces):
				return fault.New(
					fault.KindRejected, "manifest_has_workspaces", "the Workspace Manifest still owns one or more Workspaces", false,
					fault.NextAction{Command: "list", Reason: "Delete every Workspace bound to this Workspace Manifest first."},
				)
			case deleteErr != nil:
				return fault.Wrap(
					fault.KindRejected, "manifest_delete_failed", "Workspace Manifest could not be deleted", false, deleteErr,
					fault.NextAction{Command: "manifest list", Reason: "Inspect the Workspace Manifest collection before retrying."},
				)
			}
			if err := deleted.Validate(); err != nil || deleted.Name != name {
				if err == nil {
					err = fmt.Errorf("deleted Workspace Manifest identity does not match the request")
				}
				return fault.Wrap(
					fault.KindContract, "invalid_manifest_delete_result", "Workspace Manifest deletion result is invalid", false, err,
					fault.NextAction{Command: "manifest list", Reason: "Reconcile the Workspace Manifest collection after deletion."},
				)
			}
			result = deleted
			return nil
		})
	})
	if err != nil {
		return tobari.ManifestDeleteResult{}, err
	}
	return result, nil
}

// SetDefaultWithProgress selects a Context and optionally forwards the bounded
// cluster-reconcile lifecycle to the human CLI. The application owns the
// mutation and lifecycle lock; infrastructure owns the actual reconciliation.
func (s *Service) SetDefaultWithProgress(
	ctx context.Context, intent operation.Intent, name string, progress tobari.ClusterUpProgressSink,
) (tobari.ManifestReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestReport{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.ManifestReport{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_manifest_name", "Workspace Manifest name is invalid", false, err,
			fault.NextAction{Command: "manifest list", Reason: "Choose a named Workspace Manifest from the local collection."},
		)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ManifestTargetKind, ID: tobari.DefaultManifestSelectionTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ManifestReport
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			var used tobari.ManifestReport
			var useErr error
			if runtime, ok := s.runtime.(contextUseProgressRuntimePort); ok {
				used, useErr = runtime.SetDefaultManifestWithProgress(actionContext, name, progress)
			} else {
				used, useErr = s.runtime.SetDefaultManifest(actionContext, name)
			}
			if errors.Is(useErr, tobari.ErrContextNotFound) {
				return fault.New(
					fault.KindNotFound, "manifest_not_found", "the named Workspace Manifest does not exist", false,
					fault.NextAction{Command: "manifest list", Reason: "Choose an existing Workspace Manifest or create it first."},
				)
			}
			if useErr != nil {
				return fault.Wrap(fault.KindRejected, "manifest_default_set_failed", "the default Workspace Manifest selection could not be updated", false, useErr,
					fault.NextAction{Command: "manifest show", Reason: "Inspect the default Workspace Manifest selection."})
			}
			result = used
			return nil
		})
	})
	if err != nil {
		return tobari.ManifestReport{}, err
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
func (s *Service) InitRuntime(ctx context.Context, intent operation.Intent) (tobari.ManifestReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestReport{}, err
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ManifestRuntimeTargetKind, ParentID: tobari.ActiveContextRuntimeID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ManifestReport
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		initialized, initErr := s.runtime.InitRuntime(actionContext)
		if errors.Is(initErr, tobari.ErrRuntimeRecipeExists) {
			return fault.New(
				fault.KindRejected, "runtime_recipe_exists", "the default Workspace Manifest already has a runtime recipe", false,
				fault.NextAction{Command: "manifest show", Reason: "Inspect the existing runtime recipe before editing it."},
			)
		}
		if initErr != nil {
			return fault.Wrap(
				fault.KindRejected, "runtime_init_failed", "the default Workspace Manifest runtime recipe could not be created", false,
				initErr,
				fault.NextAction{Command: "manifest show", Reason: "Inspect the default Workspace Manifest stores."},
			)
		}
		result = initialized
		return nil
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

// BuildRuntime builds and atomically selects the current Context's recipe.
func (s *Service) BuildRuntime(ctx context.Context, intent operation.Intent) (tobari.ManifestReport, error) {
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
) (tobari.ManifestReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ManifestReport{}, err
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ManifestRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ManifestReport
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var built tobari.ManifestReport
		var buildErr error
		if runtime, ok := s.runtime.(contextRuntimeBuildProgressPort); ok && !portcheck.IsNil(runtime) {
			built, buildErr = runtime.BuildRuntimeWithProgress(actionContext, diagnostics, progress)
		} else {
			built, buildErr = s.runtime.BuildRuntime(actionContext)
		}
		if errors.Is(buildErr, tobari.ErrRuntimeRecipeMissing) {
			return fault.New(
				fault.KindInvalidInput, "runtime_recipe_missing", "the default Workspace Manifest has no runtime recipe", false,
				fault.NextAction{Command: "runtime init", Reason: "Create the default Workspace Manifest runtime template first."},
			)
		}
		if buildErr != nil {
			if structured, ok := fault.PublicCopy(buildErr); ok {
				return structured
			}
			return fault.Wrap(
				fault.KindRejected, "runtime_build_failed", "the default Workspace Manifest runtime could not be built", false,
				buildErr,
				fault.NextAction{Command: "manifest show", Reason: "Inspect the unchanged selected runtime and recipe state."},
			)
		}
		result = built
		return nil
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

func validateCreateInput(name, image string, mode tobari.ManifestPolicyMode, sourceAccess tobari.ManifestSourceAccess) error {
	manifest := tobari.WorkspaceManifest{
		SchemaVersion: tobari.WorkspaceManifestSchemaVersion, ID: "00000000-0000-7000-8000-000000000000", Name: name,
		AgentProfile: tobari.DefaultProfile, Image: image, PolicyMode: mode, SourceAccess: sourceAccess,
		PolicyRevision: tobari.DefaultContextPolicyRevision(),
	}
	if err := manifest.Validate(); err != nil {
		return fault.Wrap(
			fault.KindInvalidInput, "invalid_context", "Workspace Manifest definition is invalid", false, err,
			fault.NextAction{Command: "help manifest create", Reason: "Correct the Workspace Manifest name, image, policy mode, or source access."},
		)
	}
	return nil
}

func (s *Service) readError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, tobari.ErrContextNotFound) {
		return fault.New(fault.KindNotFound, "manifest_not_found", "the named Workspace Manifest does not exist", false,
			fault.NextAction{Command: "manifest list", Reason: "Choose an existing Workspace Manifest."})
	}
	return fault.Wrap(fault.KindInternal, "manifest_read_failed", "Workspace Manifest could not be read", false, err,
		fault.NextAction{Command: "doctor", Reason: "Inspect the host Workspace Manifest stores."})
}
