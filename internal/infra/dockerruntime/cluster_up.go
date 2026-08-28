package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
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
) (result tobari.State, resultErr error) {
	if err := ctx.Err(); err != nil {
		return tobari.State{}, err
	}
	if err := r.requireNoFinalGatewaySettlement(ctx); err != nil {
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
	if err := r.recoverInterruptedClusterUp(ctx, existing, exists); err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	// Recovery may have committed or removed state, so re-read the exact
	// authority before preparing another activation.
	existing, exists, err = r.LoadState(ctx)
	if err != nil {
		return tobari.State{}, err
	}
	if exists && existing.SchemaVersion == 1 {
		existing, err = r.migratePrePlatformSharedClusterState(ctx, existing)
		if err != nil {
			emitClusterUpProgress(progress, tobari.ClusterUpProgress{
				Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
			})
			return tobari.State{}, err
		}
	}
	if exists {
		if err := r.validateRollbackClosure(existing); err != nil {
			emitClusterUpProgress(progress, tobari.ClusterUpProgress{
				Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
			})
			return tobari.State{}, fmt.Errorf("verify retained rollback closure before candidate preparation: %w", err)
		}
		if err := r.validateRetainedAggregateBeforeClusterUp(ctx, existing); err != nil {
			emitClusterUpProgress(progress, tobari.ClusterUpProgress{
				Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
			})
			return tobari.State{}, fmt.Errorf("verify retained aggregate authority before candidate preparation: %w", err)
		}
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
	attemptErrorMessage := "Cluster activation did not complete; inspect shared-cluster status."
	activationAttempted := false
	activationCommitted := false
	rollbackPermitted := true
	journalStarted := false
	var appliedProfile tobari.SharedClusterAppliedProfile
	var composeAssets tobari.SharedClusterComposeAssets
	var candidateCompose candidateComposeClosureReceipt
	var freshResources *freshClusterResourceAuthority
	previousPrincipals := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: []projectPrincipalBinding{}}
	candidatePrincipals := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: []projectPrincipalBinding{}}
	var journaledCandidatePrincipals *projectPrincipalRegistry
	defer func() {
		if activationCommitted || !rollbackPermitted || !journalStarted {
			return
		}
		if !activationAttempted {
			if clearErr := r.clearClusterJournal(); clearErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("clear unstarted cluster reconcile journal: %w", clearErr))
			}
			return
		}
		rollbackContext, cancel := context.WithTimeout(r.lifetimeParent(ctx), 30*time.Second)
		defer cancel()
		var rollbackErr error
		if exists {
			rollbackErr = r.rollbackSharedClusterActivation(rollbackContext, existing, previousPrincipals)
		} else {
			rollbackErr = r.cleanupFreshClusterActivation(
				rollbackContext, state, appliedProfile, candidateCompose, freshResources,
				previousPrincipals, journaledCandidatePrincipals,
			)
		}
		if rollbackErr == nil {
			if clearErr := r.clearClusterJournal(); clearErr != nil {
				resultErr = fmt.Errorf(
					"cluster activation failed after rollback but recovery journal cleanup did not complete: %w",
					errors.Join(resultErr, clearErr),
				)
				return
			}
			if exists {
				if recordErr := r.recordRecentError(existing, attemptErrorMessage); recordErr != nil {
					resultErr = fmt.Errorf(
						"cluster activation failed after rollback and recovery evidence could not be recorded: %w",
						errors.Join(resultErr, recordErr),
					)
				}
			}
			return
		}
		resultErr = fmt.Errorf(
			"cluster activation failed and rollback did not complete: %w",
			errors.Join(resultErr, rollbackErr),
		)
	}()
	if brokerRuntimeEnabled {
		if err = r.verifyAuthBrokerImage(ctx, authBrokerSelection.Image, authBrokerSelection.RequireDigest); err != nil {
			emitClusterUpProgress(progress, tobari.ClusterUpProgress{
				Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
			})
			return tobari.State{}, err
		}
	}
	_, gatewayImageID, err := r.prepareGatewayImage(ctx)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	appliedProfile, err = tobari.SharedClusterProfileForTransport(r.permissionIngestionTransport)
	if err != nil {
		return tobari.State{}, fmt.Errorf("select applied permission profile: %w", err)
	}
	environment, err := r.composeEnvironmentForTransport(state, r.permissionIngestionTransport)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	composeAssets, candidateCompose, err = r.captureCandidateComposeClosure(state, appliedProfile)
	if err != nil {
		return tobari.State{}, fmt.Errorf("verify candidate Compose closure: %w", err)
	}
	if brokerRuntimeEnabled {
		environment = replaceEnvironmentValue(environment, "TOBARI_AUTH_BROKER_IMAGE", authBrokerSelection.Image)
	}
	environment = replaceEnvironmentValue(environment, "TOBARI_GATEWAY_IMAGE", gatewayImageID)
	versions, err := runtimeassets.Versions()
	if err != nil {
		return tobari.State{}, fmt.Errorf("resolve candidate component images: %w", err)
	}
	candidateImages := candidateClusterImages{Gateway: gatewayImageID}
	if brokerRuntimeEnabled {
		candidateImages.AuthBroker, err = r.resolveCandidateImageID(ctx, authBrokerSelection.Image)
		if err != nil {
			return tobari.State{}, fmt.Errorf("resolve candidate Auth Broker image: %w", err)
		}
	}
	emitClusterUpProgress(progress, tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressCompleted,
	})

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressPolicy, func() error {
		if err := r.testPolicy(ctx, state); err != nil {
			return fault.Wrap(fault.KindRejected, "policy_test_failed", policyTestFailureMessage, false, err)
		}
		candidateImages.OPA, err = r.resolveCandidateImageID(ctx, versions["OPA_IMAGE"])
		if err != nil {
			return fmt.Errorf("resolve candidate OPA image: %w", err)
		}
		previousPrincipals, err = r.readProjectPrincipalRegistry()
		if err != nil {
			return fmt.Errorf("capture previous project principal publication: %w", err)
		}
		if exists {
			if err := r.validateRollbackClosure(existing); err != nil {
				return fmt.Errorf("verify retained rollback closure before mutation: %w", err)
			}
			if err := r.verifyAppliedSharedCluster(ctx, existing, previousPrincipals); err != nil {
				return fmt.Errorf("verify last-successful shared-cluster entry before mutation: %w", err)
			}
		} else {
			if len(previousPrincipals.Bindings) != 0 {
				return fmt.Errorf("fresh shared-cluster principal registry is not empty")
			}
			authority, err := r.proveFreshClusterResourcesAbsent(ctx)
			if err != nil {
				return fmt.Errorf("prove fresh shared-cluster resources absent: %w", err)
			}
			freshResources = &authority
		}
		var previous *tobari.State
		if exists {
			previous = &existing
		}
		if err := r.startClusterUpReconcile(
			previous, state, appliedProfile, previousPrincipals, freshResources, candidateImages, candidateCompose,
		); err != nil {
			return fmt.Errorf("start cluster reconcile journal: %w", err)
		}
		journalStarted = true
		if exists {
			activationAttempted = true
			return r.preparePolicyBundle(ctx, state)
		}
		return nil
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressPrepareImages, func() error {
		return r.prepareContextImages(ctx)
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressStartServices, func() error {
		if !exists {
			if freshResources == nil {
				return fmt.Errorf("fresh shared-cluster activation omits resource authority")
			}
			if err := r.verifyFreshClusterResourcesAbsent(ctx, *freshResources); err != nil {
				return fmt.Errorf("fence fresh shared-cluster resources before mutation: %w", err)
			}
			if err := r.validateCandidateComposeClosure(state, appliedProfile, candidateCompose); err != nil {
				return fmt.Errorf("fence candidate Compose cleanup closure before mutation: %w", err)
			}
			currentPrincipals, err := r.readProjectPrincipalRegistry()
			if err != nil {
				return fmt.Errorf("fence fresh project principal registry before mutation: %w", err)
			}
			if len(currentPrincipals.Bindings) != 0 || !sameProjectPrincipalRegistry(currentPrincipals, previousPrincipals) {
				return fmt.Errorf("fresh project principal registry changed before mutation")
			}
			activationAttempted = true
			if err := r.preparePolicyBundle(ctx, state); err != nil {
				return err
			}
		}
		var output bytes.Buffer
		composeUpArgs := []string{"compose", "--project-directory", state.RuntimeDirectory}
		composeUpArgs = append(composeUpArgs, composeFileArgs(state.RuntimeDirectory)...)
		permissionProfileArgs, err := permissionSessionComposeFileArgsForTransport(
			state.RuntimeDirectory, r.permissionIngestionTransport,
		)
		if err != nil {
			return err
		}
		composeUpArgs = append(composeUpArgs, permissionProfileArgs...)
		composeUpArgs = append(composeUpArgs, "up", "-d", "--no-build", "--remove-orphans")
		if forceRecreate {
			composeUpArgs = append(composeUpArgs, "--force-recreate")
		}
		err = r.runner.Run(
			ctx,
			composeUpArgs,
			environment, nil, &output, &output,
		)
		if err != nil {
			attemptErrorMessage = "Cluster startup did not complete; inspect component logs."
			return fmt.Errorf("docker compose up: %w: %s", err, boundedDiagnostic(output.Bytes()))
		}
		if err := r.ensureGatewayNetworkGuard(ctx); err != nil {
			attemptErrorMessage = "Gateway network guard did not become ready; inspect Docker kernel support."
			return err
		}
		if brokerRuntimeEnabled {
			rootKey, err := r.unlockAuthBroker(ctx)
			if err != nil {
				attemptErrorMessage = "Auth Broker did not unlock; inspect root-key and broker state."
				return err
			}
			defer clear(rootKey)
			if err := r.startCredentialCompanion(ctx, rootKey); err != nil {
				attemptErrorMessage = "Credential companion did not become ready; inspect Auth Broker and host runtime state."
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
				attemptErrorMessage = "Gateway did not rejoin the shared cluster network; inspect cluster status."
				return err
			}
			if sharedNetwork == "tobari-control" {
				if err := r.ensureOPANetwork(ctx, sharedNetwork); err != nil {
					attemptErrorMessage = "OPA did not rejoin the shared control network; inspect cluster status."
					return err
				}
			}
			if brokerRuntimeEnabled {
				if err := r.ensureAuthBrokerNetwork(ctx, sharedNetwork); err != nil {
					attemptErrorMessage = "Auth Broker did not rejoin the shared cluster network; inspect cluster status."
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
			attemptErrorMessage = "Cluster components did not become healthy; inspect component status."
			return err
		}
		if err := r.waitForPolicyRevision(ctx, state.AggregateRevision); err != nil {
			attemptErrorMessage = "OPA did not activate the expected aggregate policy; inspect OPA logs."
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
		observed, err := r.syncProjectPrincipalRegistryFrom(ctx, projects, previousPrincipals, func(candidate projectPrincipalRegistry) error {
			if err := r.recordClusterUpCandidatePrincipals(candidate); err != nil {
				return fmt.Errorf("record candidate project principal publication: %w", err)
			}
			copy := candidate
			journaledCandidatePrincipals = &copy
			return nil
		})
		if err != nil {
			attemptErrorMessage = "Gateway did not rejoin every Tobari network; inspect cluster status."
			return err
		}
		candidatePrincipals = observed
		return nil
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressFinalize, func() error {
		snapshot, err := r.observeAppliedClusterSnapshot(ctx)
		if err != nil {
			return fmt.Errorf("observe applied shared-cluster component identities: %w", err)
		}
		if snapshot.images.gateway != candidateImages.Gateway || snapshot.images.opa != candidateImages.OPA ||
			snapshot.images.authBroker != candidateImages.AuthBroker {
			return fmt.Errorf("applied component images differ from the verified candidate authority")
		}
		state.SchemaVersion = 2
		state.Applied = tobari.SharedClusterAppliedEntry{
			AggregateRevision:  state.AggregateRevision,
			AssetVersion:       state.AssetVersion,
			EvaluatorIdentity:  state.EvaluatorIdentity,
			PolicyDataIdentity: state.PolicyDataIdentity,
			ComposeAssets:      composeAssets,
			GatewayImageID:     candidateImages.Gateway,
			OPAImageID:         candidateImages.OPA,
			AuthBrokerImageID:  candidateImages.AuthBroker,
			PermissionProfile:  appliedProfile,
		}
		state.RecentError = ""
		if err := state.Validate(); err != nil {
			return fmt.Errorf("validate applied shared-cluster state: %w", err)
		}
		if err := validateAppliedSharedClusterEntryForBuild(state.Applied); err != nil {
			return err
		}
		observedPrincipals, err := r.readProjectPrincipalRegistry()
		if err != nil {
			return fmt.Errorf("capture applied project principal publication: %w", err)
		}
		if observedPrincipals.SchemaVersion != candidatePrincipals.SchemaVersion ||
			!slices.Equal(observedPrincipals.Bindings, candidatePrincipals.Bindings) {
			return fmt.Errorf("candidate project principal publication drifted before commit")
		}
		if err := r.verifyAppliedSharedCluster(ctx, state, candidatePrincipals); err != nil {
			return fmt.Errorf("verify complete candidate shared-cluster effect before publication: %w", err)
		}
		if err := r.markClusterUpRuntimeReconciled(state, candidatePrincipals); err != nil {
			return fmt.Errorf("mark cluster reconcile complete: %w", err)
		}
		var previous *tobari.State
		if exists {
			previous = &existing
		}
		publication, publicationErr := r.publishStateWithVerification(ctx, previous, state)
		switch publication {
		case statePublicationNew:
			activationCommitted = true
			if publicationErr != nil {
				return fmt.Errorf("shared-cluster state committed with uncertain write completion: %w", publicationErr)
			}
		case statePublicationPrevious:
			return fmt.Errorf("shared-cluster state was not published: %w", publicationErr)
		default:
			rollbackPermitted = false
			return fmt.Errorf("shared-cluster state publication is unknown: %w", publicationErr)
		}
		if err := r.clearClusterJournal(); err != nil {
			return fmt.Errorf("clear cluster reconcile journal: %w", err)
		}
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
	networks, err := r.observeClusterContainerNetworks(ctx, container)
	if err != nil {
		return fmt.Errorf("inspect %s networks: %w", component, err)
	}
	if _, connected := networks[network]; connected {
		return nil
	}
	if err := r.runBoundedNetworkMutation(
		ctx, []string{"network", "connect", "--alias", alias, network, container},
	); err != nil {
		return fmt.Errorf("connect %s to Tobari network: %w", component, err)
	}
	return nil
}

func permissionTransportForAppliedState(state tobari.State) (tobari.PermissionSessionTransport, error) {
	if state.SchemaVersion == 1 || state.Applied.PermissionProfile == tobari.SharedClusterProfilePrePlatform {
		return "", nil
	}
	if err := state.Applied.PermissionProfile.Validate(); err != nil {
		return "", err
	}
	transport, ok := state.Applied.PermissionProfile.PermissionTransport()
	if !ok {
		return "", fmt.Errorf("applied permission ingestion profile has no transport")
	}
	return transport, nil
}

func (r *Runtime) rollbackSharedClusterActivation(
	ctx context.Context,
	state tobari.State,
	principals projectPrincipalRegistry,
) error {
	// Crash recovery revalidates the destructive execution closure at the last
	// possible point. Forward-path preflight cannot authorize retained assets
	// after an interruption or concurrent filesystem drift.
	if err := r.validateRollbackClosure(state); err != nil {
		return fmt.Errorf("validate rollback Compose closure: %w", err)
	}
	if err := state.Validate(); err != nil {
		return err
	}
	if err := r.verifyKnownGoodAggregateState(ctx, state); err != nil {
		return fmt.Errorf("validate restored aggregate policy before Compose recovery: %w", err)
	}
	if err := validateAppliedSharedClusterEntryForBuild(state.Applied); err != nil {
		return err
	}
	environment, err := r.composeEnvironment(state)
	if err != nil {
		return err
	}
	if brokerRuntimeEnabled {
		environment = replaceEnvironmentValue(
			environment, "TOBARI_AUTH_BROKER_IMAGE", state.Applied.AuthBrokerImageID,
		)
	}
	environment = replaceEnvironmentValue(environment, "TOBARI_GATEWAY_IMAGE", state.Applied.GatewayImageID)
	environment = replaceEnvironmentValue(environment, "TOBARI_OPA_IMAGE", state.Applied.OPAImageID)
	args := []string{"compose", "--project-directory", state.RuntimeDirectory}
	args = append(args, composeFileArgs(state.RuntimeDirectory)...)
	permissionTransport, err := permissionTransportForAppliedState(state)
	if err != nil {
		return err
	}
	permissionProfileArgs, err := permissionSessionComposeFileArgsForTransport(state.RuntimeDirectory, permissionTransport)
	if err != nil {
		return err
	}
	args = append(args, permissionProfileArgs...)
	args = append(args, "up", "-d", "--no-build", "--remove-orphans", "--force-recreate", "--wait")
	var output bytes.Buffer
	if err := r.runner.Run(ctx, args, environment, nil, &output, &output); err != nil {
		return fmt.Errorf("restore last successful shared-cluster entry: %w: %s", err, boundedDiagnostic(output.Bytes()))
	}
	if err := r.ensureGatewayNetworkGuard(ctx); err != nil {
		return fmt.Errorf("restore Gateway network guard: %w", err)
	}
	if brokerRuntimeEnabled {
		rootKey, err := r.unlockAuthBroker(ctx)
		if err != nil {
			return fmt.Errorf("unlock restored Auth Broker: %w", err)
		}
		defer clear(rootKey)
		if err := r.startCredentialCompanion(ctx, rootKey); err != nil {
			return fmt.Errorf("restore credential companion: %w", err)
		}
	}
	if err := r.publishKnownGoodPolicyBundle(ctx, state); err != nil {
		return fmt.Errorf("restore last-successful aggregate policy: %w", err)
	}
	if err := r.restoreAppliedNetworkTopology(ctx, principals); err != nil {
		return err
	}
	if err := r.waitForClusterReady(ctx, nil); err != nil {
		return fmt.Errorf("verify restored cluster health: %w", err)
	}
	if err := r.waitForPolicyRevision(ctx, state.AggregateRevision); err != nil {
		return fmt.Errorf("verify restored aggregate policy: %w", err)
	}
	if err := r.verifyAppliedSharedCluster(ctx, state, principals); err != nil {
		return fmt.Errorf("verify restored shared-cluster authority: %w", err)
	}
	return nil
}

func (r *Runtime) recoverInterruptedClusterUp(ctx context.Context, state tobari.State, exists bool) error {
	journal, journalExists, err := r.readClusterJournal()
	if err != nil {
		return fmt.Errorf("read interrupted cluster reconcile journal: %w", err)
	}
	if !journalExists {
		return nil
	}
	if journal.Operation != clusterOperationUp || journal.CandidateState == nil || journal.PreviousPrincipals == nil {
		return fmt.Errorf("interrupted shared-cluster reconcile lacks automatic recovery authority")
	}
	if exists && journal.Phase == clusterPhaseRuntime && state == *journal.CandidateState && journal.CandidatePrincipals != nil {
		if err := r.verifyAppliedSharedCluster(ctx, state, *journal.CandidatePrincipals); err != nil {
			return fmt.Errorf("verify interrupted committed shared-cluster activation: %w", err)
		}
		return r.clearClusterJournal()
	}
	if journal.PreviousState == nil {
		if exists {
			return fmt.Errorf("fresh shared-cluster recovery conflicts with persisted state")
		}
		if err := r.cleanupFreshClusterActivation(
			ctx, *journal.CandidateState, journal.CandidateProfile, *journal.CandidateCompose,
			journal.FreshResources, *journal.PreviousPrincipals, journal.CandidatePrincipals,
		); err != nil {
			return fmt.Errorf("recover interrupted fresh shared-cluster activation: %w", err)
		}
		return r.clearClusterJournal()
	}
	if !exists || state != *journal.PreviousState {
		return fmt.Errorf("interrupted shared-cluster state authority is unknown")
	}
	if err := r.rollbackSharedClusterActivation(ctx, state, *journal.PreviousPrincipals); err != nil {
		return fmt.Errorf("recover interrupted shared-cluster activation: %w", err)
	}
	return r.clearClusterJournal()
}

// RecoverInterruptedClusterDown consumes only a fresh activation journal for
// which no public shared-cluster State was ever published. It gives explicit
// cluster down a bounded recovery path without inventing configured state.
func (r *Runtime) RecoverInterruptedClusterDown(ctx context.Context, purge bool) (bool, error) {
	if err := r.requireNoFinalGatewaySettlement(ctx); err != nil {
		return false, err
	}
	journal, exists, err := r.readClusterJournal()
	if err != nil {
		return false, fmt.Errorf("read interrupted cluster reconcile journal: %w", err)
	}
	if !exists {
		return false, nil
	}
	if journal.Operation != clusterOperationUp || journal.PreviousState != nil ||
		journal.CandidateState == nil || journal.FreshResources == nil {
		return false, fmt.Errorf("interrupted cluster down recovery authority is ambiguous")
	}
	if _, stateExists, err := r.LoadState(ctx); err != nil {
		return false, err
	} else if stateExists {
		return false, fmt.Errorf("fresh cluster down recovery conflicts with persisted state")
	}
	if err := r.cleanupFreshClusterActivation(
		ctx, *journal.CandidateState, journal.CandidateProfile, *journal.CandidateCompose,
		journal.FreshResources, *journal.PreviousPrincipals, journal.CandidatePrincipals,
	); err != nil {
		return false, err
	}
	if purge {
		// Fresh cleanup already removes the exact candidate Compose volumes;
		// no additional global Docker prune or resource discovery is allowed.
	}
	if err := r.clearClusterJournal(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Runtime) cleanupFreshClusterActivation(
	ctx context.Context, state tobari.State, profile tobari.SharedClusterAppliedProfile,
	compose candidateComposeClosureReceipt, resources *freshClusterResourceAuthority,
	previousPrincipals projectPrincipalRegistry, candidatePrincipals *projectPrincipalRegistry,
) error {
	if resources == nil {
		return fmt.Errorf("fresh cluster cleanup omits exact resource authority")
	}
	if err := resources.Validate(); err != nil {
		return err
	}
	if err := previousPrincipals.Validate(); err != nil || len(previousPrincipals.Bindings) != 0 {
		return fmt.Errorf("fresh cluster cleanup predecessor principal authority is invalid")
	}
	if err := r.validateCandidateComposeClosure(state, profile, compose); err != nil {
		return fmt.Errorf("validate fresh cleanup Compose closure: %w", err)
	}
	preCleanupPrincipals, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return fmt.Errorf("observe fresh project principal publication before cleanup: %w", err)
	}
	if !sameProjectPrincipalRegistry(preCleanupPrincipals, previousPrincipals) &&
		(candidatePrincipals == nil || !sameProjectPrincipalRegistry(preCleanupPrincipals, *candidatePrincipals)) {
		return fmt.Errorf("fresh project principal publication drifted before cleanup")
	}
	transport, ok := profile.PermissionTransport()
	if !ok {
		return fmt.Errorf("fresh cluster cleanup profile is invalid")
	}
	environment, err := r.composeEnvironmentForTransport(state, transport)
	if err != nil {
		return err
	}
	var output bytes.Buffer
	args := []string{"compose", "--project-directory", state.RuntimeDirectory}
	args = append(args, composeFileArgs(state.RuntimeDirectory)...)
	args = append(args, "down", "--remove-orphans", "--volumes")
	if err := r.runner.Run(ctx, args, environment, nil, &output, &output); err != nil {
		return fmt.Errorf("clean fresh shared-cluster activation: %w: %s", err, boundedDiagnostic(output.Bytes()))
	}
	if brokerRuntimeEnabled {
		if err := r.waitForCredentialCompanionStopped(ctx); err != nil {
			return fmt.Errorf("verify fresh credential companion stopped: %w", err)
		}
	}
	currentPrincipals, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return fmt.Errorf("observe fresh project principal publication before cleanup: %w", err)
	}
	if !sameProjectPrincipalRegistry(currentPrincipals, previousPrincipals) {
		if candidatePrincipals == nil || !sameProjectPrincipalRegistry(currentPrincipals, *candidatePrincipals) {
			return fmt.Errorf("fresh project principal publication drifted before cleanup")
		}
		if err := r.replaceProjectPrincipalRegistryIfCurrent(ctx, currentPrincipals, previousPrincipals.Bindings); err != nil {
			return fmt.Errorf("clear exact fresh project principal publication: %w", err)
		}
	}
	if err := r.verifyFreshClusterResourcesAbsent(ctx, *resources); err != nil {
		return fmt.Errorf("verify fresh shared-cluster cleanup: %w", err)
	}
	principals, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return fmt.Errorf("verify cleared fresh project principal publication: %w", err)
	}
	if len(principals.Bindings) != 0 {
		return fmt.Errorf("fresh project principal publication was not cleared")
	}
	return nil
}

func (r *Runtime) restoreAppliedNetworkTopology(ctx context.Context, principals projectPrincipalRegistry) error {
	for _, sharedNetwork := range []string{"tobari-control", "tobari-egress"} {
		if err := r.ensureGatewayNetwork(ctx, sharedNetwork); err != nil {
			return fmt.Errorf("restore Gateway shared network: %w", err)
		}
		if sharedNetwork == "tobari-control" {
			if err := r.ensureOPANetwork(ctx, sharedNetwork); err != nil {
				return fmt.Errorf("restore OPA shared network: %w", err)
			}
		}
		if brokerRuntimeEnabled {
			if err := r.ensureAuthBrokerNetwork(ctx, sharedNetwork); err != nil {
				return fmt.Errorf("restore Auth Broker shared network: %w", err)
			}
		}
	}
	for _, binding := range principals.Bindings {
		if err := r.verifyOwnedProjectResource(ctx, "network", binding.Network, binding.ProjectID, projectNetRole); err != nil {
			return fmt.Errorf("verify retained Workspace network: %w", err)
		}
		if err := r.ensureGatewayNetworkAtAddress(ctx, binding.Network, binding.GatewayIP); err != nil {
			return err
		}
	}
	if err := r.replaceProjectPrincipalRegistry(ctx, principals.Bindings); err != nil {
		return fmt.Errorf("restore project principal publication: %w", err)
	}
	return nil
}

func (r *Runtime) ensureGatewayNetworkAtAddress(ctx context.Context, network, expected string) error {
	// Rollback force-recreates Gateway before restoring retained Workspace
	// networks. A bounded single-frame observation distinguishes exact retained
	// attachment from absence; drift never authorizes a reconnect over it.
	networks, err := r.observeClusterContainerNetworks(ctx, gatewayContainer)
	if err != nil {
		return fmt.Errorf("observe Gateway retained networks: %w", err)
	}
	if raw, connected := networks[network]; connected {
		var endpoint struct {
			IPAddress string `json:"IPAddress"`
		}
		if err := json.Unmarshal(raw, &endpoint); err != nil || endpoint.IPAddress != expected {
			return fmt.Errorf("Gateway retained network address drifted")
		}
		return nil
	}
	if err := r.runBoundedNetworkMutation(ctx, []string{
		"network", "connect", "--alias", "gateway", "--ip", expected, network, gatewayContainer,
	}); err != nil {
		return fmt.Errorf("restore Gateway retained network: %w", err)
	}
	return nil
}

func (r *Runtime) verifyAppliedSharedCluster(
	ctx context.Context, state tobari.State, principals projectPrincipalRegistry,
) error {
	if state.SchemaVersion != 2 {
		return fmt.Errorf("applied shared-cluster verification requires schema 2")
	}
	snapshot, err := r.observeAppliedClusterSnapshot(ctx)
	if err != nil {
		return err
	}
	if snapshot.images.gateway != state.Applied.GatewayImageID ||
		snapshot.images.opa != state.Applied.OPAImageID ||
		snapshot.images.authBroker != state.Applied.AuthBrokerImageID {
		return fmt.Errorf("applied shared-cluster component image identity drifted")
	}
	if err := verifyAppliedPermissionProfile(snapshot.gateway, state.Applied.PermissionProfile); err != nil {
		return err
	}
	if err := verifyAppliedNetworkTopology(snapshot, principals); err != nil {
		return err
	}
	observedPrincipals, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return err
	}
	if observedPrincipals.SchemaVersion != principals.SchemaVersion ||
		!slices.Equal(observedPrincipals.Bindings, principals.Bindings) {
		return fmt.Errorf("project principal publication drifted")
	}
	if ready, _ := r.policyRevisionReady(ctx, state.AggregateRevision); !ready {
		return fmt.Errorf("applied aggregate policy revision is not active")
	}
	return nil
}

func verifyAppliedNetworkTopology(snapshot appliedClusterSnapshot, principals projectPrincipalRegistry) error {
	gatewayExpected := map[string]string{"tobari-control": "", "tobari-egress": ""}
	for _, binding := range principals.Bindings {
		gatewayExpected[binding.Network] = binding.GatewayIP
	}
	if len(snapshot.gateway.NetworkAddresses) != len(gatewayExpected) {
		return fmt.Errorf("Gateway network topology is incomplete or ambiguous")
	}
	for network, expectedAddress := range gatewayExpected {
		observed, exists := snapshot.gateway.NetworkAddresses[network]
		if !exists || (expectedAddress != "" && observed != expectedAddress) {
			return fmt.Errorf("Gateway network topology drifted")
		}
	}
	if len(snapshot.opa.NetworkAddresses) != 1 || snapshot.opa.NetworkAddresses["tobari-control"] == "" {
		return fmt.Errorf("OPA network topology drifted")
	}
	if brokerRuntimeEnabled {
		if len(snapshot.authBroker.NetworkAddresses) != 2 ||
			snapshot.authBroker.NetworkAddresses["tobari-control"] == "" ||
			snapshot.authBroker.NetworkAddresses["tobari-egress"] == "" {
			return fmt.Errorf("Auth Broker network topology drifted")
		}
	}
	return nil
}

func verifyAppliedPermissionProfile(
	gateway appliedClusterComponentObservation, profile tobari.SharedClusterAppliedProfile,
) error {
	transportValues := make([]string, 0, 1)
	directoryValues := make([]string, 0, 1)
	for _, entry := range gateway.Environment {
		if value, ok := strings.CutPrefix(entry, "TOBARI_PERMISSION_INGESTION_TRANSPORT="); ok {
			transportValues = append(transportValues, value)
		}
		if value, ok := strings.CutPrefix(entry, "TOBARI_PERMISSION_INGESTION_DIRECTORY="); ok {
			directoryValues = append(directoryValues, value)
		}
	}
	mounts := 0
	for _, destination := range gateway.MountDestinations {
		if destination == "/run/tobari/permission-ingestion" {
			mounts++
		}
	}
	switch profile {
	case tobari.SharedClusterProfilePrePlatform:
		if len(transportValues) != 0 || len(directoryValues) != 0 || mounts != 0 {
			return fmt.Errorf("pre-platform Gateway contains successor permission projection")
		}
	case tobari.SharedClusterProfileUnix:
		if !slices.Equal(transportValues, []string{"unix"}) ||
			!slices.Equal(directoryValues, []string{"/run/tobari/permission-ingestion"}) || mounts != 1 {
			return fmt.Errorf("Unix Gateway permission projection drifted")
		}
	case tobari.SharedClusterProfileLoopbackTCP:
		if !slices.Equal(transportValues, []string{"loopback_tcp"}) || len(directoryValues) != 0 || mounts != 0 {
			return fmt.Errorf("Darwin Gateway permission projection drifted")
		}
	default:
		return fmt.Errorf("applied permission profile is invalid")
	}
	return nil
}

func (r *Runtime) composeEnvironment(state tobari.State) ([]string, error) {
	permissionTransport, err := permissionTransportForAppliedState(state)
	if err != nil {
		return nil, fmt.Errorf("select permission ingestion support profile: %w", err)
	}
	return r.composeEnvironmentForTransport(state, permissionTransport)
}

func (r *Runtime) composeEnvironmentForTransport(
	state tobari.State, permissionTransport tobari.PermissionSessionTransport,
) ([]string, error) {
	if permissionTransport != "" {
		if err := permissionTransport.Validate(); err != nil {
			return nil, fmt.Errorf("select permission ingestion support profile: %w", err)
		}
	}
	versions, err := runtimeassets.Versions()
	if err != nil {
		return nil, fmt.Errorf("read embedded runtime versions: %w", err)
	}
	uid, gid := currentIDs()
	environment := append([]string{}, os.Environ()...)
	environment = append(
		environment,
		"TOBARI_GATEWAY_CONFIG="+state.GatewayConfig,
		"TOBARI_PRINCIPAL_DIR="+r.principalRegistryDirectory(),
		"TOBARI_HOST_LOOPBACK_DIR="+r.hostLoopbackDirectory(),
		"TOBARI_INTERACTIVE_ATTACHMENT_DIR="+r.interactiveAttachmentDirectory(),
		"TOBARI_ASSET_VERSION="+state.AssetVersion,
		"TOBARI_UID="+strconv.Itoa(uid), "TOBARI_GID="+strconv.Itoa(gid),
		"TOBARI_MITMPROXY_IMAGE="+versions["MITMPROXY_IMAGE"],
		"TOBARI_GATEWAY_IMAGE=unselected",
		"TOBARI_OPA_IMAGE="+versions["OPA_IMAGE"],
		"TOBARI_DEBIAN_IMAGE="+versions["DEBIAN_IMAGE"],
	)
	if permissionTransport == tobari.PermissionSessionTransportUnix {
		environment = append(environment, "TOBARI_PERMISSION_INGESTION_DIR="+r.interactiveAttachmentSocketDirectory())
	}
	if brokerRuntimeEnabled {
		environment = append(environment,
			"TOBARI_AUTH_PROVIDER_DIR="+r.authProviderProjectionDirectory(),
			"TOBARI_AUTH_CONTEXTS_DIR="+r.authContextsDirectory(),
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
