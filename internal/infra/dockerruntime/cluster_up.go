package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

// ClusterUp materializes assets and reconciles the shared Gateway, OPA, and
// Auth Broker.
func (r *Runtime) ClusterUp(ctx context.Context) (tobari.State, error) {
	return r.ClusterUpWithProgress(ctx, nil)
}

// ClusterUpWithProgress materializes assets and reconciles the shared Gateway,
// OPA, and Auth Broker while emitting only fixed, secret-free lifecycle signals.
func (r *Runtime) ClusterUpWithProgress(
	ctx context.Context, progress tobari.ClusterUpProgressSink,
) (tobari.State, error) {
	return r.clusterUpWithProgressMode(ctx, progress, false)
}

// ValidateClusterBuildIdentity rejects a compiled resolver/API mismatch before
// the application enters its lifecycle mutation boundary.
func (r *Runtime) ValidateClusterBuildIdentity(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.validateResolverCompatibility()
}

func (r *Runtime) clusterUpWithProgressMode(
	ctx context.Context, progress tobari.ClusterUpProgressSink, forceRecreate bool,
) (tobari.State, error) {
	if err := ctx.Err(); err != nil {
		return tobari.State{}, err
	}
	if err := r.validateResolverCompatibility(); err != nil {
		return tobari.State{}, err
	}
	emitClusterUpProgress(progress, tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressStarted,
	})
	existing, exists, err := r.LoadState(ctx)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	var authBrokerSelection sharedImageSelection
	if brokerRuntimeEnabled {
		authBrokerSelection, err = r.selectAuthBrokerImage(ctx)
		if err != nil {
			emitClusterUpProgress(progress, tobari.ClusterUpProgress{
				Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
			})
			return tobari.State{}, err
		}
	}
	state, err := r.prepareState(ctx)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	if exists {
		state.RecentError = existing.RecentError
	}
	recordAttemptError := func(message string) {
		if exists {
			_ = r.recordRecentError(existing, message)
		}
	}
	activationAttempted := false
	activationCommitted := false
	var gatewayImage string
	defer func() {
		if !activationAttempted || activationCommitted || !exists || existing.AggregateRevision == state.AggregateRevision {
			return
		}
		rollbackContext, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		rollbackEnvironment, environmentErr := r.composeEnvironment(existing)
		if environmentErr != nil {
			return
		}
		if brokerRuntimeEnabled {
			rollbackEnvironment = replaceEnvironmentValue(
				rollbackEnvironment, "TOBARI_AUTH_BROKER_IMAGE", authBrokerSelection.Image,
			)
		}
		rollbackEnvironment = replaceEnvironmentValue(
			rollbackEnvironment, "TOBARI_GATEWAY_IMAGE", gatewayImage,
		)
		rollbackArgs := []string{"compose", "--project-directory", existing.RuntimeDirectory}
		rollbackArgs = append(rollbackArgs, composeFileArgs(existing.RuntimeDirectory)...)
		rollbackArgs = append(rollbackArgs, "up", "-d", "--no-build", "--remove-orphans", "--force-recreate", "--wait")
		_ = r.runner.Run(
			rollbackContext,
			rollbackArgs,
			rollbackEnvironment, nil, io.Discard, io.Discard,
		)
		_ = r.ensureGatewayNetworkGuard(rollbackContext)
	}()
	var environment []string
	environment, err = r.composeEnvironment(state)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	if brokerRuntimeEnabled {
		if err = r.verifyAuthBrokerImage(ctx, authBrokerSelection.Image, authBrokerSelection.RequireDigest); err != nil {
			emitClusterUpProgress(progress, tobari.ClusterUpProgress{
				Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
			})
			return tobari.State{}, err
		}
		environment = replaceEnvironmentValue(environment, "TOBARI_AUTH_BROKER_IMAGE", authBrokerSelection.Image)
	}
	gatewayImage, err = r.prepareGatewayImage(ctx)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	environment = replaceEnvironmentValue(environment, "TOBARI_GATEWAY_IMAGE", gatewayImage)
	if err := r.startClusterReconcile(clusterOperationUp); err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, fmt.Errorf("start cluster reconcile journal: %w", err)
	}
	emitClusterUpProgress(progress, tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressCompleted,
	})

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressPolicy, func() error {
		if err := r.testPolicy(ctx, state); err != nil {
			_ = r.clearClusterJournal()
			return fault.Wrap(fault.KindRejected, "policy_test_failed", policyTestFailureMessage, false, err)
		}
		return r.preparePolicyBundle(ctx, state)
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressPrepareImages, func() error {
		return r.prepareContextImages(ctx)
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressStartServices, func() error {
		activationAttempted = true
		var output bytes.Buffer
		composeUpArgs := []string{"compose", "--project-directory", state.RuntimeDirectory}
		composeUpArgs = append(composeUpArgs, composeFileArgs(state.RuntimeDirectory)...)
		composeUpArgs = append(composeUpArgs, "up", "-d", "--no-build", "--remove-orphans")
		if forceRecreate {
			composeUpArgs = append(composeUpArgs, "--force-recreate")
		}
		err := r.runner.Run(
			ctx,
			composeUpArgs,
			environment, nil, &output, &output,
		)
		if err != nil {
			recordAttemptError("Cluster startup did not complete; inspect component logs.")
			return fmt.Errorf("docker compose up: %w: %s", err, boundedDiagnostic(output.Bytes()))
		}
		if err := r.ensureGatewayNetworkGuard(ctx); err != nil {
			recordAttemptError("Gateway network guard did not become ready; inspect Docker kernel support.")
			return err
		}
		if brokerRuntimeEnabled {
			rootKey, err := r.unlockAuthBroker(ctx)
			if err != nil {
				recordAttemptError("Auth Broker did not unlock; inspect root-key and broker state.")
				return err
			}
			defer clear(rootKey)
			if err := r.startCredentialCompanion(ctx, rootKey); err != nil {
				recordAttemptError("Credential companion did not become ready; inspect Auth Broker and host runtime state.")
				return err
			}
		}
		return nil
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressConnectNetworks, func() error {
		for _, sharedNetwork := range []string{"tobari-control", "tobari-egress"} {
			if err := r.ensureGatewayNetwork(ctx, sharedNetwork); err != nil {
				recordAttemptError("Gateway did not rejoin the shared cluster network; inspect cluster status.")
				return err
			}
			if sharedNetwork == "tobari-control" {
				if err := r.ensureOPANetwork(ctx, sharedNetwork); err != nil {
					recordAttemptError("OPA did not rejoin the shared control network; inspect cluster status.")
					return err
				}
			}
			if brokerRuntimeEnabled {
				if err := r.ensureAuthBrokerNetwork(ctx, sharedNetwork); err != nil {
					recordAttemptError("Auth Broker did not rejoin the shared cluster network; inspect cluster status.")
					return err
				}
			}
		}
		return nil
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressWaitForHealth, func() error {
		if err := r.waitForClusterReady(ctx, progress); err != nil {
			recordAttemptError("Cluster components did not become healthy; inspect component status.")
			return err
		}
		if err := r.waitForPolicyRevision(ctx, state.AggregateRevision); err != nil {
			recordAttemptError("OPA did not activate the expected aggregate policy; inspect OPA logs.")
			return err
		}
		return nil
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressReconcileProjects, func() error {
		projects, err := r.ListProjects(ctx)
		if err != nil {
			return fmt.Errorf("read CWD-owned projects for Gateway reconciliation: %w", err)
		}
		if err := r.syncProjectPrincipalRegistry(ctx, projects); err != nil {
			recordAttemptError("Gateway did not rejoin every Tobari network; inspect cluster status.")
			return err
		}
		return nil
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressFinalize, func() error {
		if err := r.markClusterRuntimeReconciled(clusterOperationUp); err != nil {
			return fmt.Errorf("mark cluster reconcile complete: %w", err)
		}
		state.RecentError = ""
		if err := r.writeState(state); err != nil {
			return err
		}
		if err := r.clearClusterJournal(); err != nil {
			return fmt.Errorf("clear cluster reconcile journal: %w", err)
		}
		activationCommitted = true
		return nil
	}); err != nil {
		return tobari.State{}, err
	}
	return state, nil
}

func runClusterUpProgressStep(
	progress tobari.ClusterUpProgressSink, step tobari.ClusterUpProgressStep, action func() error,
) error {
	emitClusterUpProgress(progress, tobari.ClusterUpProgress{Step: step, Status: tobari.ClusterUpProgressStarted})
	if err := action(); err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{Step: step, Status: tobari.ClusterUpProgressFailed})
		return err
	}
	emitClusterUpProgress(progress, tobari.ClusterUpProgress{Step: step, Status: tobari.ClusterUpProgressCompleted})
	return nil
}

func emitClusterUpProgress(
	progress tobari.ClusterUpProgressSink, event tobari.ClusterUpProgress,
) {
	if progress == nil {
		return
	}
	if err := event.Validate(); err != nil {
		return
	}
	progress(event)
}

func (r *Runtime) waitForClusterReady(
	ctx context.Context, progress tobari.ClusterUpProgressSink,
) error {
	const attempts = 60
	for attempt := 0; attempt < attempts; attempt++ {
		ready := true
		statuses := make([]tobari.ComponentStatus, 0, len(clusterContainers))
		for _, name := range clusterComponentOrder {
			component, err := r.inspectContainer(ctx, name, clusterContainers[name])
			if err != nil {
				return err
			}
			statuses = append(statuses, component)
			if component.State != "running" || component.Health != "healthy" {
				ready = false
			}
		}
		if brokerRuntimeEnabled {
			if brokerState, err := r.brokerState(ctx); err != nil || brokerState != "ready" {
				ready = false
			}
			if companionState, _, err := r.credentialCompanionStatus(ctx); err != nil || companionState != "ready" {
				ready = false
			}
		}
		if ready {
			return nil
		}
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressWaitForHealth, Status: tobari.ClusterUpProgressUpdated,
		})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("cluster components did not become healthy")
}

func (r *Runtime) ensureGatewayNetwork(ctx context.Context, network string) error {
	return r.ensureClusterContainerNetwork(ctx, gatewayContainer, "Gateway", "gateway", network)
}

func (r *Runtime) ensureOPANetwork(ctx context.Context, network string) error {
	return r.ensureClusterContainerNetwork(ctx, opaContainer, "OPA", "opa", network)
}

func (r *Runtime) ensureAuthBrokerNetwork(ctx context.Context, network string) error {
	return r.ensureClusterContainerNetwork(ctx, authBrokerContainer, "Auth Broker", "auth-broker", network)
}

func (r *Runtime) ensureClusterContainerNetwork(
	ctx context.Context,
	container string,
	component string,
	alias string,
	network string,
) error {
	output, err := r.runner.Output(
		ctx,
		[]string{"inspect", "--format", "{{json .NetworkSettings.Networks}}", container},
		os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("inspect %s networks: %w: %s", component, err, boundedDiagnostic(output))
	}
	var networks map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(output), &networks); err != nil {
		return fmt.Errorf("decode %s networks: %w", component, err)
	}
	if _, connected := networks[network]; connected {
		return nil
	}
	output, err = r.runner.Output(
		ctx,
		[]string{"network", "connect", "--alias", alias, network, container},
		os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("connect %s to Tobari network: %w: %s", component, err, boundedDiagnostic(output))
	}
	return nil
}

func (r *Runtime) composeEnvironment(state tobari.State) ([]string, error) {
	versions, err := runtimeassets.Versions()
	if err != nil {
		return nil, fmt.Errorf("read embedded runtime versions: %w", err)
	}
	uid, gid := currentIDs()
	environment := append([]string{}, os.Environ()...)
	environment = append(
		environment,
		"TOBARI_POLICY_DIR="+state.PolicyDirectory,
		"TOBARI_GATEWAY_CONFIG="+state.GatewayConfig,
		"TOBARI_PRINCIPAL_DIR="+r.principalRegistryDirectory(),
		"TOBARI_HOST_LOOPBACK_DIR="+r.hostLoopbackDirectory(),
		"TOBARI_INTERACTIVE_ATTACHMENT_DIR="+r.interactiveAttachmentDirectory(),
		"TOBARI_PERMISSION_INGESTION_DIR="+r.interactiveAttachmentSocketDirectory(),
		"TOBARI_ASSET_VERSION="+state.AssetVersion,
		"TOBARI_UID="+strconv.Itoa(uid), "TOBARI_GID="+strconv.Itoa(gid),
		"TOBARI_MITMPROXY_IMAGE="+versions["MITMPROXY_IMAGE"],
		"TOBARI_GATEWAY_IMAGE=unselected",
		"TOBARI_OPA_IMAGE="+versions["OPA_IMAGE"],
		"TOBARI_DEBIAN_IMAGE="+versions["DEBIAN_IMAGE"],
	)
	if brokerRuntimeEnabled {
		environment = append(environment,
			"TOBARI_AUTH_PROVIDER_DIR="+r.authProviderProjectionDirectory(),
			"TOBARI_AUTH_CONTEXTS_DIR="+r.authContextsDirectory(),
			"TOBARI_AUTH_RUNTIME_DIR="+r.authRuntimeDirectory(),
			"TOBARI_AUTH_BROKER_IMAGE=unselected",
		)
	}
	return environment, nil
}

func replaceEnvironmentValue(environment []string, key, value string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, prefix+value)
}
