package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	finalClusterBootstrapSchema   = 1
	finalClusterBootstrapPrepared = "prepared"
	finalClusterBootstrapRunning  = "running"
)

type finalClusterBootstrapJournal struct {
	SchemaVersion     int                                `json:"schema_version"`
	Operation         string                             `json:"operation"`
	DecisionRef       string                             `json:"decision_ref"`
	Phase             string                             `json:"phase"`
	Plan              tobari.WorkspacePolicyProjection   `json:"plan"`
	State             tobari.State                       `json:"state"`
	Profile           tobari.SharedClusterAppliedProfile `json:"profile"`
	Compose           candidateComposeClosureReceipt     `json:"compose"`
	GatewayImageID    string                             `json:"gateway_image_id"`
	OPAImageID        string                             `json:"opa_image_id"`
	AuthBrokerImage   string                             `json:"auth_broker_image,omitempty"`
	AuthBrokerImageID string                             `json:"auth_broker_image_id,omitempty"`
	Fresh             freshClusterResourceAuthority      `json:"fresh_resources"`
}

func (r *Runtime) finalClusterBootstrapJournalPath() string {
	return filepath.Join(r.finalGatewaySettlementRoot(), "bootstrap.json")
}

func (j finalClusterBootstrapJournal) validate(r *Runtime) error {
	if j.SchemaVersion != finalClusterBootstrapSchema || j.Operation == "" || j.DecisionRef == "" || j.Plan.Validate() != nil ||
		j.State.Validate() != nil || j.Profile.Validate() != nil || j.Profile == tobari.SharedClusterProfilePrePlatform ||
		j.Compose.Validate() != nil || j.Compose.RuntimeDirectory != j.State.RuntimeDirectory || j.Compose.Profile != j.Profile ||
		!imageIDPattern.MatchString(j.GatewayImageID) || !imageIDPattern.MatchString(j.OPAImageID) || j.Fresh.Validate() != nil {
		return fmt.Errorf("final cluster bootstrap journal is invalid")
	}
	if brokerRuntimeEnabled {
		if j.AuthBrokerImage == "" || !imageIDPattern.MatchString(j.AuthBrokerImageID) {
			return fmt.Errorf("final cluster bootstrap Auth Broker authority is incomplete")
		}
	} else if j.AuthBrokerImage != "" || j.AuthBrokerImageID != "" {
		return fmt.Errorf("release final cluster bootstrap contains Auth Broker authority")
	}
	if j.Phase != finalClusterBootstrapPrepared && j.Phase != finalClusterBootstrapRunning {
		return fmt.Errorf("final cluster bootstrap phase is invalid")
	}
	return r.validateCandidateComposeClosure(j.State, j.Profile, j.Compose)
}

func (r *Runtime) readFinalClusterBootstrap() (finalClusterBootstrapJournal, bool, error) {
	var journal finalClusterBootstrapJournal
	if err := readStrictJSON(r.finalClusterBootstrapJournalPath(), &journal); errors.Is(err, os.ErrNotExist) {
		return journal, false, nil
	} else if err != nil {
		return journal, false, err
	}
	if err := journal.validate(r); err != nil {
		return journal, false, err
	}
	return journal, true, nil
}

func (r *Runtime) writeFinalClusterBootstrap(journal finalClusterBootstrapJournal) error {
	if err := journal.validate(r); err != nil {
		return err
	}
	if err := r.ensurePrivateDirectory(r.finalGatewaySettlementRoot()); err != nil {
		return err
	}
	return writeAtomicJSON(r.finalClusterBootstrapJournalPath(), journal)
}

func (r *Runtime) removeFinalClusterBootstrap() error {
	if err := os.Remove(r.finalClusterBootstrapJournalPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectoryIfPresent(r.finalGatewaySettlementRoot())
}

// ensureFinalClusterBaseComponents creates only a fresh, fixed shared base.
// Existing or partial unjournaled resources are ambiguity and fail closed.
func (r *Runtime) ensureFinalClusterBaseComponents(ctx context.Context, plan tobari.WorkspacePolicyProjection, operation, decisionRef string) error {
	gateway, gatewayMissing, gatewayErr := r.observeAppliedClusterComponent(ctx, "gateway", gatewayContainer)
	opa, opaMissing, opaErr := r.observeAppliedClusterComponent(ctx, "opa", opaContainer)
	if gatewayErr == nil && opaErr == nil && !gatewayMissing && !opaMissing && gateway.State == "running" && gateway.Health == "healthy" && opa.State == "running" && opa.Health == "healthy" {
		if brokerRuntimeEnabled {
			// Gateway/OPA alone are not the research base closure. Observe Broker
			// and companion now; settlement below owns any exact repair.
			if _, _, err := r.observeAppliedClusterComponent(ctx, "auth-broker", authBrokerContainer); err != nil {
				return err
			}
		}
		return nil
	}
	if stoppedReceipt, stopped, err := r.readFinalClusterStopped(r.finalClusterStoppedReceiptPath()); err != nil {
		return err
	} else if stopped && gatewayMissing && opaMissing {
		return r.confirmFinalStoppedRestartPrerequisites(ctx, stoppedReceipt)
	}
	journal, present, err := r.readFinalClusterBootstrap()
	if err != nil {
		return err
	}
	if present {
		if journal.Operation != operation || journal.DecisionRef != decisionRef || !reflectWorkspacePolicyPlan(journal.Plan, plan) {
			return fmt.Errorf("another final cluster bootstrap requires exact same-action recovery")
		}
		return r.resumeFinalClusterBootstrap(ctx, journal)
	}
	if gatewayErr != nil || opaErr != nil || !gatewayMissing || !opaMissing {
		return fmt.Errorf("final cluster base resources are partial, unhealthy, or ambiguous")
	}
	if operation == "" || decisionRef == "" || plan.Validate() != nil {
		return fmt.Errorf("final cluster bootstrap request is invalid")
	}
	// The base Compose closure bind-mounts both canonical attachment stores.
	// Initialize their exact empty authority before Docker can create either
	// source as a root-owned directory and before the first policy fence uses
	// the WP07 registry as its global zero-owner proof.
	if err := r.ensureHostLoopbackStore(ctx); err != nil {
		return fmt.Errorf("prepare canonical Host Loopback authority: %w", err)
	}
	if err := r.ensureInteractiveAttachmentStore(ctx); err != nil {
		return fmt.Errorf("prepare canonical interactive attachment authority: %w", err)
	}
	aggregate, _, _, err := r.buildFinalSettlementArtifacts(ctx, plan)
	if err != nil {
		return err
	}
	profile, err := tobari.SharedClusterProfileForTransport(r.permissionIngestionTransport)
	if err != nil {
		return err
	}
	state, compose, gatewayImage, opaImage, err := r.prepareFinalGatewayComposeAuthority(ctx, aggregate, profile, len(plan.Contexts))
	if err != nil {
		return err
	}
	fresh, err := r.proveFreshClusterResourcesAbsent(ctx)
	if err != nil {
		return err
	}
	if err := r.ensureProjectPrincipalRegistry(ctx); err != nil {
		return err
	}
	principals, err := r.readProjectPrincipalRegistry()
	if err != nil || len(principals.Bindings) != 0 {
		return fmt.Errorf("fresh final cluster principal registry is not empty: %w", err)
	}
	journal = finalClusterBootstrapJournal{SchemaVersion: finalClusterBootstrapSchema, Operation: operation, DecisionRef: decisionRef, Phase: finalClusterBootstrapPrepared, Plan: plan.Clone(), State: state, Profile: profile, Compose: compose, GatewayImageID: gatewayImage, OPAImageID: opaImage, Fresh: fresh}
	if brokerRuntimeEnabled {
		selection, selectErr := r.selectAuthBrokerImage(ctx)
		if selectErr != nil {
			return selectErr
		}
		if err := r.verifyAuthBrokerImage(ctx, selection.Image, selection.RequireDigest); err != nil {
			return err
		}
		journal.AuthBrokerImage = selection.Image
		journal.AuthBrokerImageID, err = r.resolveCandidateImageID(ctx, selection.Image)
		if err != nil {
			return err
		}
		if _, err := r.prepareAuthProjection(); err != nil {
			return err
		}
	}
	if err := r.writeFinalClusterBootstrap(journal); err != nil {
		return err
	}
	return r.resumeFinalClusterBootstrap(ctx, journal)
}

func (r *Runtime) confirmFinalStoppedRestartPrerequisites(ctx context.Context, receipt finalClusterStoppedReceipt) error {
	if err := receipt.validate(r); err != nil {
		return err
	}
	components := []struct{ component, name string }{{"gateway", gatewayContainer}, {"opa", opaContainer}}
	if brokerRuntimeEnabled {
		components = append(components, struct{ component, name string }{"auth-broker", authBrokerContainer})
	}
	for _, item := range components {
		if _, missing, err := r.observeFinalClusterComponentRaw(ctx, item.component, item.name); err != nil || !missing {
			return fmt.Errorf("stopped final %s absence is unknown: %w", item.component, err)
		}
	}
	for _, network := range []string{"tobari-control", "tobari-egress"} {
		if err := r.requireDockerResourceAbsent(ctx, "network", network); err != nil {
			return fmt.Errorf("stopped final shared network absence: %w", err)
		}
	}
	for _, volume := range []string{"tobari-gateway-ca", policyBundleVolume, "tobari-public-ca"} {
		if err := r.verifyOwned(ctx, "volume", volume); err != nil {
			return fmt.Errorf("stopped final retained volume closure: %w", err)
		}
	}
	return nil
}

func reflectWorkspacePolicyPlan(left, right tobari.WorkspacePolicyProjection) bool {
	return left.PlanDigest == right.PlanDigest && left.CollectionRevision == right.CollectionRevision && left.Mode == right.Mode
}

func (r *Runtime) resumeFinalClusterBootstrap(ctx context.Context, journal finalClusterBootstrapJournal) error {
	if err := journal.validate(r); err != nil {
		return err
	}
	if journal.Phase == finalClusterBootstrapRunning {
		return r.waitForFinalClusterBase(ctx, journal.GatewayImageID, journal.OPAImageID, journal.AuthBrokerImageID)
	}
	if err := r.validateCandidateComposeClosure(journal.State, journal.Profile, journal.Compose); err != nil {
		return err
	}
	projection := FinalAggregateProjection{
		AggregateRevision:  journal.State.AggregateRevision,
		PolicyDirectory:    journal.State.PolicyDirectory,
		GatewayConfig:      journal.State.GatewayConfig,
		MaterializedDigest: journal.Plan.ContentDigest,
		EvaluatorIdentity:  journal.State.EvaluatorIdentity,
		PolicyDataIdentity: journal.State.PolicyDataIdentity,
	}
	if err := r.prepareFinalPolicyBundle(ctx, projection); err != nil {
		return err
	}
	transport, ok := journal.Profile.PermissionTransport()
	if !ok {
		return fmt.Errorf("final cluster bootstrap permission profile is invalid")
	}
	environment, err := r.composeEnvironmentForTransport(journal.State, transport)
	if err != nil {
		return err
	}
	environment = replaceEnvironmentValue(environment, "TOBARI_GATEWAY_IMAGE", journal.GatewayImageID)
	environment = replaceEnvironmentValue(environment, "TOBARI_OPA_IMAGE", journal.OPAImageID)
	if brokerRuntimeEnabled {
		environment = replaceEnvironmentValue(environment, "TOBARI_AUTH_BROKER_IMAGE", journal.AuthBrokerImage)
	}
	environment = applyFinalGatewayComposeEnvironment(environment, selectedFinalGatewayEnvironment(journal.Profile))
	args := []string{"compose", "--project-directory", journal.State.RuntimeDirectory}
	args = append(args, composeFileArgs(journal.State.RuntimeDirectory)...)
	profileArgs, err := permissionSessionComposeFileArgsForTransport(journal.State.RuntimeDirectory, transport)
	if err != nil {
		return err
	}
	args = append(args, profileArgs...)
	args = append(args, "up", "-d", "--no-build", "--no-deps", "opa", "gateway")
	if brokerRuntimeEnabled {
		args = append(args, "auth-broker")
	}
	var output bytes.Buffer
	if err := r.runner.Run(ctx, args, environment, nil, &output, &output); err != nil {
		return fmt.Errorf("activate fresh final cluster base: %w: %s", err, boundedDiagnostic(output.Bytes()))
	}
	if err := r.ensureFinalClusterBootstrapTopology(ctx); err != nil {
		return err
	}
	if brokerRuntimeEnabled {
		rootKey, err := r.unlockAuthBroker(ctx)
		if err != nil {
			return err
		}
		defer clear(rootKey)
		if err := r.startCredentialCompanion(ctx, rootKey); err != nil {
			return err
		}
	}
	if err := r.waitForFinalClusterBase(ctx, journal.GatewayImageID, journal.OPAImageID, journal.AuthBrokerImageID); err != nil {
		return err
	}
	gateway, gm, ge := r.observeFinalClusterComponentRaw(ctx, "gateway", gatewayContainer)
	opa, om, oe := r.observeFinalClusterComponentRaw(ctx, "opa", opaContainer)
	if ge != nil || oe != nil || gm || om || gateway.ImageID != journal.GatewayImageID || opa.ImageID != journal.OPAImageID {
		return fmt.Errorf("fresh final cluster base differs from journaled image authority")
	}
	journal.Phase = finalClusterBootstrapRunning
	return r.writeFinalClusterBootstrap(journal)
}

func (r *Runtime) ensureFinalClusterBootstrapTopology(ctx context.Context) error {
	for _, network := range []string{"tobari-control", "tobari-egress"} {
		if err := r.verifyOwned(ctx, "network", network); err != nil {
			return fmt.Errorf("verify fresh final shared network %s: %w", network, err)
		}
	}
	type component struct {
		name     string
		label    string
		networks []string
		connect  func(context.Context, string) error
	}
	components := []component{
		{name: opaContainer, label: "OPA", networks: []string{"tobari-control"}, connect: r.ensureOPANetwork},
	}
	if brokerRuntimeEnabled {
		components = append(components, component{name: authBrokerContainer, label: "Auth Broker", networks: finalAuthBrokerNetworkNames(), connect: r.ensureAuthBrokerNetwork})
	}
	components = append(components, component{
		name: gatewayContainer, label: "Gateway", networks: []string{"tobari-control", "tobari-egress"}, connect: r.ensureGatewayNetwork,
	})

	restart := make([]string, 0, len(components))
	for _, item := range components {
		observed, err := r.observeClusterContainerNetworks(ctx, item.name)
		if err != nil {
			return fmt.Errorf("observe fresh final %s topology: %w", item.label, err)
		}
		allowed := make(map[string]struct{}, len(item.networks))
		for _, network := range item.networks {
			allowed[network] = struct{}{}
		}
		for network := range observed {
			if _, ok := allowed[network]; !ok {
				return fmt.Errorf("fresh final %s has an unexpected network attachment", item.label)
			}
		}
		changed := false
		for _, network := range item.networks {
			if _, ok := observed[network]; ok {
				continue
			}
			if err := item.connect(ctx, network); err != nil {
				return err
			}
			changed = true
		}
		if changed {
			restart = append(restart, item.name)
		}
		confirmed, err := r.observeClusterContainerNetworks(ctx, item.name)
		if err != nil || len(confirmed) != len(item.networks) {
			return fmt.Errorf("confirm fresh final %s topology: %w", item.label, err)
		}
		for _, network := range item.networks {
			if _, ok := confirmed[network]; !ok {
				return fmt.Errorf("fresh final %s is missing network %s", item.label, network)
			}
		}
	}
	if len(restart) != 0 {
		if err := r.restartFinalReplacementComponents(ctx, restart); err != nil {
			return fmt.Errorf("restart fresh final component topology: %w", err)
		}
	}
	return nil
}

func (r *Runtime) waitForFinalClusterBase(ctx context.Context, gatewayImage, opaImage, brokerImage string) error {
	var last error
	for attempt := 0; attempt < 60; attempt++ {
		gateway, gm, ge := r.observeFinalClusterComponentRaw(ctx, "gateway", gatewayContainer)
		opa, om, oe := r.observeFinalClusterComponentRaw(ctx, "opa", opaContainer)
		if ge == nil && oe == nil && !gm && !om && gateway.ImageID == gatewayImage && opa.ImageID == opaImage && gateway.State == "running" && gateway.Health == "healthy" && opa.State == "running" && opa.Health == "healthy" {
			if !brokerRuntimeEnabled {
				return nil
			}
			broker, bm, be := r.observeAppliedClusterComponent(ctx, "auth-broker", authBrokerContainer)
			companion, _, ce := r.credentialCompanionStatus(ctx)
			if be == nil && !bm && broker.ImageID == brokerImage && companion == "ready" && ce == nil {
				return nil
			}
			last = errors.Join(be, ce)
		}
		last = errors.Join(ge, oe)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("fresh final Gateway/OPA did not become exactly healthy: %w", last)
}
