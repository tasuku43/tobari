package dockerruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	contextRuntimeFileMode = 0o600
	maxRuntimeRecipeBytes  = 256 * 1024
)

const contextRuntimeTemplate = `# This file defines the runtime for the active Tobari Context.
# Add the tools and configuration your coding agent needs.
FROM %s

USER root

# Example:
# RUN apt-get update \
#     && apt-get install -y --no-install-recommends nodejs npm \
#     && rm -rf /var/lib/apt/lists/*

USER tobari
`

func runtimeRecipeTemplate(baseImage string) string {
	return fmt.Sprintf(contextRuntimeTemplate, baseImage)
}

func (r *Runtime) contextRuntimeDirectory(name string) string {
	return filepath.Join(r.contextDirectory(name), "runtime")
}

func (r *Runtime) contextRuntimeDockerfile(name string) string {
	return filepath.Join(r.contextRuntimeDirectory(name), "Dockerfile")
}

func (r *Runtime) contextRuntimeSourceDigest(name string) (string, error) {
	if err := requirePrivateDirectory(r.contextRuntimeDirectory(name)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", tobari.ErrRuntimeRecipeMissing
		}
		return "", fmt.Errorf("inspect runtime recipe directory: %w", err)
	}
	path := r.contextRuntimeDockerfile(name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", tobari.ErrRuntimeRecipeMissing
	}
	if err != nil {
		return "", fmt.Errorf("inspect runtime Dockerfile: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("runtime Dockerfile must be a regular owner-only file")
	}
	if info.Size() == 0 || info.Size() > maxRuntimeRecipeBytes {
		return "", fmt.Errorf("runtime Dockerfile must be between 1 and %d bytes", maxRuntimeRecipeBytes)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is the fixed child of a validated Context.
	if err != nil {
		return "", fmt.Errorf("read runtime Dockerfile: %w", err)
	}
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

func (r *Runtime) contextRuntimeUsesRefreshableBase(name string) (bool, error) {
	data, err := os.ReadFile(r.contextRuntimeDockerfile(name)) // #nosec G304 -- path is the fixed child of a validated Context.
	if err != nil {
		return false, fmt.Errorf("read runtime Dockerfile for base refresh: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || fields[0] != "FROM" {
			continue
		}
		for _, field := range fields[1:] {
			if strings.EqualFold(field, "AS") {
				break
			}
			if strings.HasPrefix(field, "--") {
				continue
			}
			return field == r.defaultRuntimeImage() && r.imageResolver().ShouldPullRuntimeImage(field), nil
		}
	}
	return false, nil
}

func (r *Runtime) contextRuntimeReport(manifest tobari.ContextManifest) (tobari.ContextRuntimeReport, error) {
	if manifest.RuntimeBinding != nil {
		binding := *manifest.RuntimeBinding
		if err := binding.Validate(); err != nil {
			return tobari.ContextRuntimeReport{}, err
		}
		kind := tobari.ContextRuntimeKindManaged
		status := tobari.ContextRuntimeStatusReady
		if binding.RuntimeID == tobari.StandardRuntimeID {
			kind, status = tobari.ContextRuntimeKindOfficial, tobari.ContextRuntimeStatusOfficial
		}
		return tobari.ContextRuntimeReport{
			Kind: kind, Status: status, Image: binding.Image,
			RuntimeID: binding.RuntimeID, Name: binding.Name, Revision: binding.Revision, Ordinal: binding.Ordinal,
		}, nil
	}
	if manifest.Runtime == nil {
		return tobari.ContextRuntimeReport{
			Kind:          tobari.ContextRuntimeKindOfficial,
			Status:        tobari.ContextRuntimeStatusOfficial,
			BaseReference: r.defaultRuntimeImage(),
		}, nil
	}
	recipe := *manifest.Runtime
	if err := recipe.Validate(); err != nil {
		return tobari.ContextRuntimeReport{}, err
	}
	report := tobari.ContextRuntimeReport{
		Kind:          recipe.Kind,
		Status:        tobari.ContextRuntimeStatusPendingBuild,
		Dockerfile:    r.contextRuntimeDockerfile(manifest.Name),
		BaseReference: recipe.BaseReference,
		SourceDigest:  recipe.SourceDigest,
	}
	sourceDigest, err := r.contextRuntimeSourceDigest(manifest.Name)
	if errors.Is(err, tobari.ErrRuntimeRecipeMissing) {
		report.Status = tobari.ContextRuntimeStatusInvalid
		return report, nil
	}
	if err != nil {
		return tobari.ContextRuntimeReport{}, err
	}
	report.SourceDigest = sourceDigest
	if recipe.LastBuild == nil {
		return report, nil
	}
	report.ImageDigest = recipe.LastBuild.ImageDigest
	if recipe.LastBuild.SourceDigest != sourceDigest || manifest.Image != recipe.LastBuild.Image {
		return report, nil
	}
	report.Status = tobari.ContextRuntimeStatusReady
	return report, nil
}

func (r *Runtime) contextReport(ctx context.Context, task string, manifest tobari.ContextManifest, active string) (tobari.ContextReport, error) {
	runtimeReport, err := r.contextRuntimeReport(manifest)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	shellEnvironment, err := tobari.CompleteContextShellEnvironment(manifest.ShellEnvironment)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	gitIdentity := tobari.DefaultContextGitIdentityReport()
	if manifest.GitIdentity != nil {
		gitIdentity = *manifest.GitIdentity
	}
	nativeReadiness, err := tobari.ResolveContextNativeReadiness(manifest.NativeReadiness)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	result := tobari.ContextReport{
		Task:             task,
		ContextState:     tobari.ContextObservationPersisted,
		ID:               manifest.ID,
		Name:             manifest.Name,
		Active:           manifest.Name == active,
		AgentProfile:     manifest.AgentProfile,
		Image:            manifest.Image,
		PolicyMode:       manifest.PolicyMode,
		SourceAccess:     manifest.SourceAccess,
		PolicyRevision:   manifest.PolicyRevision,
		NativeReadiness:  nativeReadiness,
		ShellEnvironment: shellEnvironment,
		GitIdentity:      gitIdentity,
		Stores:           r.contextPaths(manifest.Name),
		Runtime:          runtimeReport,
		Cluster:          tobari.ContextClusterStatusNotApplicable,
		Authentication: tobari.ContextAuthentication{
			Mode: tobari.ContextAuthenticationModeNotApplicable, BrokerState: tobari.ContextAuthBrokerNotApplicable,
		},
		Bootstrap: tobari.ContextBootstrapReportFrom(manifest.Bootstrap),
	}
	policy, policyErr := r.readContextPolicy(manifest)
	if policyErr != nil {
		return tobari.ContextReport{}, policyErr
	}
	result.MethodPolicy = policy.MethodPolicy
	if task == tobari.TaskContextShow {
		result.Authentication, err = r.contextAuthentication(ctx, manifest.ID)
		if err != nil {
			return tobari.ContextReport{}, err
		}
	}
	if err := result.Validate(); err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func (r *Runtime) nonPersistedContextReport(observed observedContext, active string) (tobari.ContextReport, error) {
	manifest := observed.manifest
	runtimeReport, err := r.contextRuntimeReport(manifest)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	shellEnvironment, err := tobari.CompleteContextShellEnvironment(manifest.ShellEnvironment)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	gitIdentity := tobari.DefaultContextGitIdentityReport()
	if manifest.GitIdentity != nil {
		gitIdentity = *manifest.GitIdentity
	}
	result := tobari.ContextReport{
		Task: tobari.TaskContextShow, ContextState: observed.state, Name: manifest.Name,
		Active: manifest.Name == active, AgentProfile: manifest.AgentProfile, Image: manifest.Image,
		PolicyMode: manifest.PolicyMode, SourceAccess: manifest.SourceAccess,
		NativeReadiness:  tobari.ContextNativeReadinessEnabled,
		MethodPolicy:     tobari.ContextMethodPolicy{Default: tobari.ContextMethodExactReview, Overrides: []tobari.ContextMethodOverride{}},
		ShellEnvironment: shellEnvironment, GitIdentity: gitIdentity,
		Stores: tobari.ContextStorePaths{}, Runtime: runtimeReport, Cluster: tobari.ContextClusterStatusNotApplicable,
		Authentication: nativeOrUnavailableContextAuthentication(),
		Bootstrap:      tobari.ContextBootstrapReportFrom(nil),
	}
	if err := result.Validate(); err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func (r *Runtime) contextAuthentication(ctx context.Context, contextID string) (tobari.ContextAuthentication, error) {
	if !brokerRuntimeEnabled {
		return tobari.ContextAuthentication{
			Mode: tobari.ContextAuthenticationModeNative, Providers: []tobari.ContextAuthProvider{},
		}, nil
	}
	projection, err := r.loadAuthProviders()
	if err != nil {
		return tobari.ContextAuthentication{}, err
	}
	var state authbroker.BrokerState
	var stateErr error
	report := tobari.ContextAuthentication{
		Mode:        tobari.ContextAuthenticationModeBroker,
		BrokerState: tobari.ContextAuthBrokerUnavailable,
		Providers:   make([]tobari.ContextAuthProvider, 0, len(projection.Providers)),
	}
	_, configured, loadErr := r.LoadState(ctx)
	if loadErr != nil {
		return tobari.ContextAuthentication{}, loadErr
	}
	if !configured {
		stateErr = nil
		state = authbroker.BrokerStateUnavailable
	} else {
		state, stateErr = r.brokerState(ctx)
	}
	if stateErr == nil {
		switch state {
		case authbroker.BrokerStateReady:
			report.BrokerState = tobari.ContextAuthBrokerReady
		case authbroker.BrokerStateLocked:
			report.BrokerState = tobari.ContextAuthBrokerLocked
		}
	}
	for _, provider := range projection.Providers {
		item := tobari.ContextAuthProvider{
			Provider: provider.ID,
			State:    tobari.ContextAuthProviderUnavailable,
		}
		if report.BrokerState == tobari.ContextAuthBrokerReady {
			response, statusErr := r.runBrokerControl(
				ctx, nil, "status", "--context-id", contextID, "--provider", provider.ID,
			)
			if statusErr != nil {
				return tobari.ContextAuthentication{}, classifyBrokerError(statusErr, "context show")
			}
			if response.Provider != provider.ID {
				return tobari.ContextAuthentication{}, fault.New(
					fault.KindContract, "invalid_auth_broker_metadata",
					"The Auth Broker returned provider status for the wrong provider.", false,
					fault.NextAction{Command: "doctor", Reason: "Inspect Auth Broker and provider projection consistency."},
				)
			}
			switch response.State {
			case "ready":
				item.State = tobari.ContextAuthProviderConfigured
				item.CredentialRevision = response.Revision
				item.AccountLabel, err = validatedAccountLabel(response.AccountLabel)
				if err != nil {
					return tobari.ContextAuthentication{}, err
				}
			case "not_configured":
				item.State = tobari.ContextAuthProviderNotConfigured
			default:
				return tobari.ContextAuthentication{}, fault.New(
					fault.KindContract, "invalid_auth_broker_metadata",
					"The Auth Broker returned an invalid provider status.", false,
					fault.NextAction{Command: "doctor", Reason: "Inspect Auth Broker and provider projection consistency."},
				)
			}
		}
		report.Providers = append(report.Providers, item)
	}
	if err := report.Validate(true); err != nil {
		return tobari.ContextAuthentication{}, fmt.Errorf("Context authentication report is invalid: %w", err)
	}
	return report, nil
}

func nativeOrUnavailableContextAuthentication() tobari.ContextAuthentication {
	if !brokerRuntimeEnabled {
		return tobari.ContextAuthentication{
			Mode: tobari.ContextAuthenticationModeNative, Providers: []tobari.ContextAuthProvider{},
		}
	}
	return tobari.ContextAuthentication{
		Mode:        tobari.ContextAuthenticationModeBroker,
		BrokerState: tobari.ContextAuthBrokerUnavailable, Providers: []tobari.ContextAuthProvider{},
	}
}

// InitRuntime creates the active Context's recipe without changing its
// selected image. The manifest update is atomic; an existing recipe is never
// overwritten.
func (r *Runtime) InitRuntime(ctx context.Context) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	var result tobari.ContextReport
	err := r.withClusterLock(func() error {
		manifest, _, err := r.activeContext()
		if err != nil {
			return err
		}
		if manifest.Runtime != nil {
			return tobari.ErrRuntimeRecipeExists
		}
		directory := r.contextRuntimeDirectory(manifest.Name)
		if err := r.ensurePrivateDirectory(directory); err != nil {
			return fmt.Errorf("prepare Context runtime directory: %w", err)
		}
		path := r.contextRuntimeDockerfile(manifest.Name)
		if _, err := os.Lstat(path); err == nil {
			return tobari.ErrRuntimeRecipeExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect Context runtime recipe: %w", err)
		}

		previous := manifest
		manifest.SchemaVersion = tobari.ContextSchemaVersion
		manifest.RuntimeBinding = nil
		manifest.Runtime = &tobari.ContextRuntimeRecipe{
			Kind:          tobari.ContextRuntimeKindDockerfile,
			File:          tobari.ContextRuntimeRecipeFile,
			BaseReference: r.defaultRuntimeImage(),
		}
		if err := manifest.Validate(); err != nil {
			return err
		}
		if err := writeAtomicJSON(r.contextManifestPath(manifest.Name), manifest); err != nil {
			return fmt.Errorf("write runtime recipe metadata: %w", err)
		}
		if err := initializeBytes(path, []byte(runtimeRecipeTemplate(r.defaultRuntimeImage())), contextRuntimeFileMode); err != nil {
			_ = writeAtomicJSON(r.contextManifestPath(previous.Name), previous)
			return fmt.Errorf("write runtime Dockerfile: %w", err)
		}
		active, err := r.readActiveContext()
		if err != nil {
			return err
		}
		result, err = r.contextReport(ctx, tobari.TaskRuntimeInit, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

// BuildRuntime builds, validates, and atomically selects the active Context's
// generated local image. The previous selected image remains authoritative
// until atomic promotion succeeds; a later reporting failure is surfaced as
// already promoted instead of claiming that the previous image remains.
func (r *Runtime) BuildRuntime(ctx context.Context) (tobari.ContextReport, error) {
	return r.BuildRuntimeWithProgress(ctx, nil, nil)
}

// BuildRuntimeWithProgress is the diagnostic extension of BuildRuntime. It
// forwards both Docker output streams without retaining the caller's writer
// and emits only validated, secret-free semantic stage metadata.
func (r *Runtime) BuildRuntimeWithProgress(
	ctx context.Context,
	diagnostics io.Writer,
	progress tobari.RuntimeBuildProgressSink,
) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	var result tobari.ContextReport
	err := r.withClusterLock(func() error {
		manifest, _, err := r.activeContext()
		if err != nil {
			return err
		}
		if manifest.Runtime == nil {
			return tobari.ErrRuntimeRecipeMissing
		}
		recipe := *manifest.Runtime
		if err := recipe.Validate(); err != nil {
			return err
		}
		sourceDigest, err := r.contextRuntimeSourceDigest(manifest.Name)
		if err != nil {
			return err
		}
		image := managedRuntimeImage(manifest.Name, sourceDigest)
		buildProgress := tobari.RuntimeBuildProgress{
			ContextName:    manifest.Name,
			Dockerfile:     r.contextRuntimeDockerfile(manifest.Name),
			PreviousImage:  manifest.Image,
			CandidateImage: image,
			Selection:      tobari.RuntimeBuildSelectionUnchanged,
		}
		emitRuntimeBuildProgress(progress, buildProgress, tobari.RuntimeBuildStagePrepare, tobari.RuntimeBuildProgressStarted)
		pullBase, err := r.contextRuntimeUsesRefreshableBase(manifest.Name)
		if err != nil {
			emitRuntimeBuildProgress(progress, buildProgress, tobari.RuntimeBuildStagePrepare, tobari.RuntimeBuildProgressFailed)
			return err
		}
		emitRuntimeBuildProgress(progress, buildProgress, tobari.RuntimeBuildStagePrepare, tobari.RuntimeBuildProgressCompleted)
		buildArgs := []string{"buildx", "build", "--progress=plain", "--load"}
		if pullBase {
			buildArgs = append(buildArgs, "--pull")
		}
		buildArgs = append(
			buildArgs,
			"--tag", image,
			"--file", r.contextRuntimeDockerfile(manifest.Name),
			r.contextRuntimeDirectory(manifest.Name),
		)
		var tail runtimeBuildDiagnosticTail
		diagnosticOutput := &bestEffortDiagnosticWriter{writer: diagnostics}
		stream := io.MultiWriter(diagnosticOutput, &tail)
		if err := runRuntimeBuildStage(progress, buildProgress, tobari.RuntimeBuildStageBuild, func() error {
			return r.runner.Run(ctx, buildArgs, os.Environ(), nil, stream, stream)
		}); err != nil {
			return fmt.Errorf("build Context runtime: %w: %s", err, boundedDiagnostic(tail.Bytes()))
		}
		if err := runRuntimeBuildStage(progress, buildProgress, tobari.RuntimeBuildStageValidate, func() error {
			return r.validateCompatibleImage(ctx, image)
		}); err != nil {
			return err
		}
		var imageDigest string
		err = runRuntimeBuildStage(progress, buildProgress, tobari.RuntimeBuildStageInspect, func() error {
			var inspectErr error
			imageDigest, inspectErr = r.inspectImageDigest(ctx, image)
			return inspectErr
		})
		if err != nil {
			return err
		}
		previous := manifest
		manifest.SchemaVersion = tobari.ContextSchemaVersion
		manifest.Image = image
		recipe.SourceDigest = sourceDigest
		recipe.LastBuild = &tobari.ContextRuntimeBuild{
			Image: image, ImageDigest: imageDigest, SourceDigest: sourceDigest,
		}
		manifest.Runtime = &recipe
		emitRuntimeBuildProgress(progress, buildProgress, tobari.RuntimeBuildStagePromote, tobari.RuntimeBuildProgressStarted)
		if err := manifest.Validate(); err != nil {
			emitRuntimeBuildProgress(progress, buildProgress, tobari.RuntimeBuildStagePromote, tobari.RuntimeBuildProgressFailed)
			return err
		}
		if err := writeAtomicJSON(r.contextManifestPath(manifest.Name), manifest); err != nil {
			buildProgress.Selection = tobari.RuntimeBuildSelectionUncertain
			emitRuntimeBuildProgress(progress, buildProgress, tobari.RuntimeBuildStagePromote, tobari.RuntimeBuildProgressFailed)
			_ = writeAtomicJSON(r.contextManifestPath(previous.Name), previous)
			return fmt.Errorf("promote Context runtime: %w", err)
		}
		buildProgress.Selection = tobari.RuntimeBuildSelectionPromoted
		emitRuntimeBuildProgress(progress, buildProgress, tobari.RuntimeBuildStagePromote, tobari.RuntimeBuildProgressCompleted)
		emitRuntimeBuildProgress(progress, buildProgress, tobari.RuntimeBuildStageReport, tobari.RuntimeBuildProgressStarted)
		active, err := r.readActiveContext()
		if err != nil {
			emitRuntimeBuildProgress(progress, buildProgress, tobari.RuntimeBuildStageReport, tobari.RuntimeBuildProgressFailed)
			return err
		}
		result, err = r.contextReport(ctx, tobari.TaskRuntimeBuild, manifest, active)
		if err != nil {
			emitRuntimeBuildProgress(progress, buildProgress, tobari.RuntimeBuildStageReport, tobari.RuntimeBuildProgressFailed)
			return err
		}
		emitRuntimeBuildProgress(progress, buildProgress, tobari.RuntimeBuildStageReport, tobari.RuntimeBuildProgressCompleted)
		return err
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func runRuntimeBuildStage(
	progress tobari.RuntimeBuildProgressSink,
	metadata tobari.RuntimeBuildProgress,
	stage tobari.RuntimeBuildStage,
	action func() error,
) error {
	emitRuntimeBuildProgress(progress, metadata, stage, tobari.RuntimeBuildProgressStarted)
	if err := action(); err != nil {
		emitRuntimeBuildProgress(progress, metadata, stage, tobari.RuntimeBuildProgressFailed)
		return err
	}
	emitRuntimeBuildProgress(progress, metadata, stage, tobari.RuntimeBuildProgressCompleted)
	return nil
}

func emitRuntimeBuildProgress(
	progress tobari.RuntimeBuildProgressSink,
	metadata tobari.RuntimeBuildProgress,
	stage tobari.RuntimeBuildStage,
	status tobari.RuntimeBuildProgressStatus,
) {
	if progress == nil {
		return
	}
	metadata.Stage = stage
	metadata.Status = status
	if metadata.Validate() == nil {
		progress(metadata)
	}
}

type bestEffortDiagnosticWriter struct {
	mu     sync.Mutex
	writer io.Writer
}

func (w *bestEffortDiagnosticWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writer != nil {
		_, _ = w.writer.Write(data)
	}
	return len(data), nil
}

const runtimeBuildDiagnosticTailBytes = 64 * 1024

type runtimeBuildDiagnosticTail struct {
	mu   sync.Mutex
	data []byte
}

func (b *runtimeBuildDiagnosticTail) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(data)
	if len(data) >= runtimeBuildDiagnosticTailBytes {
		b.data = append(b.data[:0], data[len(data)-runtimeBuildDiagnosticTailBytes:]...)
		return written, nil
	}
	if overflow := len(b.data) + len(data) - runtimeBuildDiagnosticTailBytes; overflow > 0 {
		copy(b.data, b.data[overflow:])
		b.data = b.data[:len(b.data)-overflow]
	}
	b.data = append(b.data, data...)
	return written, nil
}

func (b *runtimeBuildDiagnosticTail) Bytes() []byte {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...)
}

func (r *Runtime) inspectImageDigest(ctx context.Context, image string) (string, error) {
	output, err := r.runner.Output(ctx, []string{"image", "inspect", "--format", "{{.Id}}", image}, os.Environ())
	if err != nil {
		return "", fmt.Errorf("inspect built Context runtime: %w: %s", err, boundedDiagnostic(output))
	}
	digest := strings.TrimSpace(string(output))
	if err := tobari.ValidateDigest(digest); err != nil {
		return "", fmt.Errorf("built Context runtime returned invalid image identity: %w", err)
	}
	return digest, nil
}

func managedRuntimeImage(contextName, sourceDigest string) string {
	short := strings.TrimPrefix(sourceDigest, "sha256:")
	if len(short) > 12 {
		short = short[:12]
	}
	return "tobari-context-" + contextName + ":" + short
}
