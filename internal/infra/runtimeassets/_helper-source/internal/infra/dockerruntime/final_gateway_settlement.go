package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const (
	finalGatewaySettlementSchema = 1
	finalGatewaySettlementOPA    = "opa_only"
	finalGatewaySettlementFull   = "gateway"
	finalGatewayPhasePrepared    = "prepared"
	finalGatewayPhaseFenced      = "fenced"
	finalGatewayPhaseReplaced    = "gateway_replaced"
	finalGatewayPhasePrincipals  = "principals_published"
	finalGatewayPhasePolicy      = "policy_active"
	finalGatewayPhaseReceipt     = "receipt_published"
)

var finalGatewayInheritedCAPath = path.Join("/", "home", "mitmproxy", ".mitmproxy")

// finalGatewaySettlementCandidate binds every selectable value before an
// effect: the complete final policy plan, exact Workspace/container/network
// rows, selected component images, fixed Compose closure, and immutable
// aggregate artifacts. The replacement ContainerID is intentionally absent;
// Docker allocates it during the effect and it enters Candidate only after an
// exact post-replacement observation has been journaled.
type finalGatewaySettlementCandidate struct {
	Plan               tobari.WorkspacePolicyProjection   `json:"plan"`
	ReviewedSetDigest  tobari.SemanticDigest              `json:"reviewed_set_digest,omitempty"`
	Principals         []FinalWorkspacePrincipalRow       `json:"principals"`
	GatewayNetworks    []FinalGatewayNetworkAddress       `json:"gateway_networks"`
	OPANetworks        []FinalGatewayNetworkAddress       `json:"opa_networks"`
	GatewayImageID     string                             `json:"gateway_image_id"`
	OPAImageID         string                             `json:"opa_image_id"`
	AuthBrokerImage    string                             `json:"auth_broker_image,omitempty"`
	AuthBrokerImageID  string                             `json:"auth_broker_image_id,omitempty"`
	AuthBrokerNetworks []FinalGatewayNetworkAddress       `json:"auth_broker_networks,omitempty"`
	ReviewedGateway    string                             `json:"reviewed_gateway_container_id"`
	GatewayEnv         []string                           `json:"gateway_environment"`
	Profile            tobari.SharedClusterAppliedProfile `json:"profile"`
	Compose            candidateComposeClosureReceipt     `json:"compose"`
	Aggregate          FinalAggregateProjection           `json:"aggregate"`
	PolicyArtifact     tobari.SemanticDigest              `json:"policy_artifact_digest"`
	GatewayArtifact    tobari.SemanticDigest              `json:"gateway_artifact_digest"`
}

func finalAuthBrokerNetworkNames() []string {
	return []string{"tobari-control", "tobari-egress"}
}

func (c finalGatewaySettlementCandidate) validate(runtime *Runtime) error {
	if runtime == nil || c.Plan.Validate() != nil || !imageIDPattern.MatchString(c.GatewayImageID) ||
		!imageIDPattern.MatchString(c.OPAImageID) || c.Profile.Validate() != nil || c.Compose.Validate() != nil ||
		!aggregateRevisionPattern.MatchString(c.Aggregate.AggregateRevision) || c.PolicyArtifact.Validate() != nil ||
		c.GatewayArtifact.Validate() != nil || c.Principals == nil || c.GatewayNetworks == nil || c.OPANetworks == nil ||
		validateFinalGatewayEnvironment(c.GatewayEnv, c.Profile) != nil || !containerIDPattern.MatchString(c.ReviewedGateway) {
		return fmt.Errorf("final Gateway settlement candidate metadata is invalid")
	}
	if c.Plan.Mode == tobari.WorkspacePolicyProjectionReviewed {
		if c.ReviewedSetDigest.Validate() != nil {
			return fmt.Errorf("final Gateway settlement reviewed-set identity is invalid")
		}
	} else if c.ReviewedSetDigest != "" {
		return fmt.Errorf("final Gateway settlement carries reviewed-set identity outside reviewed mode")
	}
	if brokerRuntimeEnabled {
		wantNetworks := finalAuthBrokerNetworkNames()
		if c.AuthBrokerImage == "" || !imageIDPattern.MatchString(c.AuthBrokerImageID) || len(c.AuthBrokerNetworks) != len(wantNetworks) {
			return fmt.Errorf("final Gateway settlement Auth Broker successor authority is incomplete")
		}
		for index, network := range c.AuthBrokerNetworks {
			if network.Name != wantNetworks[index] {
				return fmt.Errorf("final Gateway settlement Auth Broker topology is invalid")
			}
			address, err := netip.ParseAddr(network.Address)
			if err != nil || !address.Is4() || !address.IsGlobalUnicast() {
				return fmt.Errorf("final Gateway settlement Auth Broker topology is invalid")
			}
		}
	} else if c.AuthBrokerImage != "" || c.AuthBrokerImageID != "" || len(c.AuthBrokerNetworks) != 0 {
		return fmt.Errorf("release final Gateway settlement contains Auth Broker authority")
	}
	if c.Aggregate.MaterializedDigest != c.Plan.ContentDigest {
		return fmt.Errorf("final Gateway settlement candidate aggregate crosses its plan")
	}
	if err := validateFinalSettlementPrincipals(c.Plan, c.Principals); err != nil {
		return err
	}
	previous := ""
	for _, network := range c.GatewayNetworks {
		address, err := netip.ParseAddr(network.Address)
		if !projectPrincipalNetworkPattern.MatchString(network.Name) || err != nil || !address.Is4() || !address.IsGlobalUnicast() || previous != "" && network.Name <= previous {
			return fmt.Errorf("final Gateway settlement topology is invalid")
		}
		previous = network.Name
	}
	if len(c.OPANetworks) != 1 || c.OPANetworks[0].Name != "tobari-control" {
		return fmt.Errorf("final OPA settlement topology is invalid")
	}
	if address, err := netip.ParseAddr(c.OPANetworks[0].Address); err != nil || !address.Is4() || !address.IsGlobalUnicast() {
		return fmt.Errorf("final OPA settlement address is invalid")
	}
	if c.Compose.Profile != c.Profile || c.Compose.RuntimeDirectory == "" || filepath.Dir(c.Aggregate.GatewayConfig) == "." {
		return fmt.Errorf("final Gateway settlement fixed resources are inconsistent")
	}
	policyDigest, err := digestFinalArtifactTree(c.Aggregate.PolicyDirectory, 64*1024*1024)
	if err != nil || policyDigest != c.PolicyArtifact {
		return fmt.Errorf("final Gateway settlement policy artifact changed: %w", err)
	}
	gateway, err := readOwnerPolicyFile(c.Aggregate.GatewayConfig, 256*1024)
	if err != nil {
		return err
	}
	digest, err := digestFinalArtifactBytes(gateway)
	if err != nil || digest != c.GatewayArtifact {
		return fmt.Errorf("final Gateway settlement Gateway artifact changed: %w", err)
	}
	return nil
}

type finalGatewaySettlementJournal struct {
	SchemaVersion      int                             `json:"schema_version"`
	Operation          string                          `json:"operation"`
	DecisionRef        string                          `json:"decision_ref"`
	EffectClass        string                          `json:"effect_class"`
	Phase              string                          `json:"phase"`
	PreviousGeneration uint64                          `json:"previous_generation"`
	PreviousRevision   tobari.SemanticDigest           `json:"previous_revision"`
	NextGeneration     uint64                          `json:"next_generation"`
	NextRevision       tobari.SemanticDigest           `json:"next_revision"`
	PreviousActive     *finalPolicyActivationRecord    `json:"previous_active,omitempty"`
	PreviousPrincipals projectPrincipalRegistry        `json:"previous_principals"`
	Candidate          finalGatewaySettlementCandidate `json:"candidate"`
	Applied            *finalPolicyActivationRecord    `json:"applied,omitempty"`
}

func (j finalGatewaySettlementJournal) validate(runtime *Runtime) error {
	if j.SchemaVersion != finalGatewaySettlementSchema || j.Operation == "" || j.DecisionRef == "" ||
		j.PreviousGeneration == 0 || j.NextGeneration == 0 || j.PreviousRevision.Validate() != nil ||
		j.NextRevision.Validate() != nil {
		return fmt.Errorf("final Gateway settlement journal metadata is invalid")
	}
	if err := j.PreviousPrincipals.Validate(); err != nil {
		return fmt.Errorf("final Gateway settlement predecessor principals are invalid: %w", err)
	}
	if err := j.Candidate.validate(runtime); err != nil {
		return fmt.Errorf("final Gateway settlement candidate is invalid: %w", err)
	}
	switch j.EffectClass {
	case finalGatewaySettlementOPA:
		if j.Phase != finalGatewayPhasePrepared && j.Phase != finalGatewayPhaseReceipt {
			return fmt.Errorf("OPA-only settlement journal phase is invalid")
		}
	case finalGatewaySettlementFull:
		switch j.Phase {
		case finalGatewayPhasePrepared, finalGatewayPhaseFenced, finalGatewayPhaseReplaced,
			finalGatewayPhasePrincipals, finalGatewayPhasePolicy, finalGatewayPhaseReceipt:
		default:
			return fmt.Errorf("Gateway settlement journal phase is invalid")
		}
	default:
		return fmt.Errorf("final Gateway settlement effect class is invalid")
	}
	if j.PreviousActive != nil && j.PreviousActive.validate(runtime) != nil {
		return fmt.Errorf("final Gateway settlement predecessor receipt is invalid")
	}
	if j.Applied != nil {
		if j.Applied.validate(runtime) != nil || !reflect.DeepEqual(j.Applied.Material.Plan, j.Candidate.Plan) {
			return fmt.Errorf("final Gateway settlement applied receipt is invalid")
		}
		if j.Phase == finalGatewayPhasePrepared || j.Phase == finalGatewayPhaseFenced || j.Phase == finalGatewayPhasePrincipals {
			return fmt.Errorf("final Gateway settlement published applied evidence before replacement")
		}
	}
	return nil
}

func (r *Runtime) finalGatewaySettlementRoot() string {
	return filepath.Join(r.stateDirectory, "workspace-authority-gateway")
}

func (r *Runtime) finalGatewaySettlementJournalPath() string {
	return filepath.Join(r.finalGatewaySettlementRoot(), "settlement.json")
}

// SettleFinalAuthority is the dormant shared consumer used by Context entry,
// Workspace retirement, and direct Policy Memory decisions. Its caller owns
// the installation lifecycle lock. This method adds the existing policy lock,
// durably fixes the effect class, and never reads predecessor Manifest policy
// state or the legacy shared State as final content authority.
func (r *Runtime) SettleFinalAuthority(
	ctx context.Context,
	previous, next tobari.WorkspaceAuthorityCollection,
	target tobari.ContextID,
	operation, decisionRef string,
) error {
	plan, err := tobari.BuildHotWorkspacePolicyProjection(next, target)
	if err != nil {
		return err
	}
	return r.settleFinalAuthority(ctx, previous, next, plan, operation, decisionRef)
}

// SettleFinalContextDeletion removes one Context from the complete active
// projection without adopting any remaining Context's pending Template or
// Policy-Memory axis. The deleted Context ID binds the task decision even
// though it is intentionally absent from the candidate plan.
func (r *Runtime) SettleFinalContextDeletion(
	ctx context.Context,
	previous, next tobari.WorkspaceAuthorityCollection,
	deleted tobari.ContextID,
	operation, decisionRef string,
) error {
	if err := deleted.Validate(); err != nil {
		return err
	}
	previousFound := false
	for _, record := range previous.Contexts {
		previousFound = previousFound || record.Context.ID == deleted
	}
	if !previousFound {
		return fmt.Errorf("deleted Context was absent from predecessor authority")
	}
	for _, record := range next.Contexts {
		if record.Context.ID == deleted {
			return fmt.Errorf("deleted Context remains in final authority")
		}
	}
	plan, err := tobari.BuildActiveWorkspacePolicyProjection(next)
	if err != nil {
		return err
	}
	return r.settleFinalAuthority(ctx, previous, next, plan, operation, decisionRef)
}

// ReconcileFinalClusterAuthority uses the same dormant coordinator while
// selecting current/current axes for every Context. The caller supplies the
// exact next envelope whose independent active receipts have already been
// prepared; this method does not publish that envelope itself.
func (r *Runtime) ReconcileFinalClusterAuthority(
	ctx context.Context,
	previous, next tobari.WorkspaceAuthorityCollection,
	operation, decisionRef string,
) error {
	plan, err := tobari.BuildClusterWorkspacePolicyProjection(next)
	if err != nil {
		return err
	}
	return r.settleFinalAuthority(ctx, previous, next, plan, operation, decisionRef)
}

// SettleFinalReviewedPolicyAuthority applies one fixed reviewed set as one
// global projection. It never sequences Contexts and never selects Template
// current/current as cluster reconciliation would.
func (r *Runtime) SettleFinalReviewedPolicyAuthority(
	ctx context.Context,
	previous, next tobari.WorkspaceAuthorityCollection,
	set tobari.PolicyMemoryReviewedDecisionSet,
	operation, decisionRef string,
) (tobari.PolicyMemoryReviewedSettlementReceipt, error) {
	if err := tobari.ValidatePolicyMemoryReviewedTransition(previous, next, set); err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, fmt.Errorf("reviewed Policy Memory settlement transition: %w", err)
	}
	targets, err := set.TargetContextIDs()
	if err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, err
	}
	plan, err := tobari.BuildReviewedWorkspacePolicyProjection(next, targets)
	if err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, err
	}
	if err := r.settleFinalAuthority(ctx, previous, next, plan, operation, decisionRef, set.Digest); err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, err
	}
	return r.confirmFinalReviewedPolicyAuthority(ctx, plan, set)
}

func (r *Runtime) ConfirmFinalReviewedPolicyAuthority(
	ctx context.Context,
	current tobari.WorkspaceAuthorityCollection,
	set tobari.PolicyMemoryReviewedDecisionSet,
) (tobari.PolicyMemoryReviewedSettlementReceipt, error) {
	targets, err := set.TargetContextIDs()
	if err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, err
	}
	plan, err := tobari.BuildReviewedWorkspacePolicyProjection(current, targets)
	if err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, err
	}
	return r.confirmFinalReviewedPolicyAuthority(ctx, plan, set)
}

func (r *Runtime) confirmFinalReviewedPolicyAuthority(
	ctx context.Context,
	plan tobari.WorkspacePolicyProjection,
	set tobari.PolicyMemoryReviewedDecisionSet,
) (tobari.PolicyMemoryReviewedSettlementReceipt, error) {
	if plan.Mode != tobari.WorkspacePolicyProjectionReviewed {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, fmt.Errorf("reviewed Policy Memory confirmation plan is invalid")
	}
	if err := r.confirmFinalPolicyProjection(ctx, plan); err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, err
	}
	active, err := r.readFinalPolicyActivation(r.finalPolicyActiveReceiptPath())
	if err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, err
	}
	if active.ReviewedSetDigest != set.Digest || active.Material.Plan.ContentDigest != plan.ContentDigest ||
		active.Receipt.MaterializedDigest != active.Material.MaterializedDigest {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, fmt.Errorf("reviewed Policy Memory active receipt crosses selected content")
	}
	receipt := tobari.PolicyMemoryReviewedSettlementReceipt{
		DecisionSetDigest: set.Digest, PlanDigest: plan.PlanDigest, ContentDigest: plan.ContentDigest,
		AggregateRevision: active.Aggregate.AggregateRevision, PolicyArtifact: active.Receipt.PolicyArtifact,
		GatewayArtifact: active.Receipt.GatewayArtifact, PrincipalDigest: active.Receipt.PrincipalDigest,
	}
	return receipt, receipt.Validate()
}

func (r *Runtime) ConfirmFinalAuthoritySettled(
	ctx context.Context, next tobari.WorkspaceAuthorityCollection, target tobari.ContextID,
) error {
	plan, err := tobari.BuildHotWorkspacePolicyProjection(next, target)
	if err != nil {
		return err
	}
	return r.confirmFinalPolicyActivation(ctx, plan, func(item tobari.WorkspacePolicyProjectionContext) bool {
		return item.ContextID == target
	})
}

func (r *Runtime) ConfirmFinalContextDeletionSettled(
	ctx context.Context, next tobari.WorkspaceAuthorityCollection, deleted tobari.ContextID,
) error {
	if err := deleted.Validate(); err != nil {
		return err
	}
	for _, record := range next.Contexts {
		if record.Context.ID == deleted {
			return fmt.Errorf("deleted Context remains in final authority")
		}
	}
	plan, err := tobari.BuildActiveWorkspacePolicyProjection(next)
	if err != nil {
		return err
	}
	return r.confirmFinalPolicyProjection(ctx, plan)
}

func (r *Runtime) settleFinalAuthority(
	ctx context.Context,
	previous, next tobari.WorkspaceAuthorityCollection,
	plan tobari.WorkspacePolicyProjection,
	operation, decisionRef string,
	reviewedSetDigest ...tobari.SemanticDigest,
) error {
	var setDigest tobari.SemanticDigest
	if len(reviewedSetDigest) > 1 {
		return fmt.Errorf("final Gateway settlement has ambiguous reviewed-set identity")
	}
	if len(reviewedSetDigest) == 1 {
		setDigest = reviewedSetDigest[0]
	}
	if r == nil || previous.Validate() != nil || next.Validate() != nil || plan.Validate() != nil || plan.CollectionRevision != next.Revision || operation == "" || decisionRef == "" ||
		plan.Mode == tobari.WorkspacePolicyProjectionReviewed && setDigest.Validate() != nil ||
		plan.Mode != tobari.WorkspacePolicyProjectionReviewed && setDigest != "" {
		return fmt.Errorf("final Gateway settlement request is invalid")
	}
	if _, present, err := r.readFinalClusterStopped(r.finalClusterDownJournalPath()); err != nil {
		return fmt.Errorf("read interrupted final cluster down: %w", err)
	} else if present {
		return fmt.Errorf("final cluster down is interrupted; resume its exact initiating action")
	}
	if err := r.requireNoInterruptedClusterReconcile(ctx); err != nil {
		return err
	}
	return r.withPolicyProjectionLock(ctx, func() error {
		journal, present, err := r.readFinalGatewaySettlementJournal()
		if err != nil {
			return err
		}
		if present {
			if journal.Operation != operation || journal.DecisionRef != decisionRef ||
				journal.PreviousGeneration != previous.Generation || journal.PreviousRevision != previous.Revision ||
				journal.NextGeneration != next.Generation || journal.NextRevision != next.Revision ||
				!reflect.DeepEqual(journal.Candidate.Plan, plan) || journal.Candidate.ReviewedSetDigest != setDigest {
				return fmt.Errorf("another final Gateway settlement requires exact same-action recovery")
			}
			return r.resumeFinalGatewaySettlement(ctx, journal)
		}
		prepared, err := r.prepareFinalGatewaySettlement(ctx, previous, next, plan, operation, decisionRef, setDigest)
		if err != nil {
			return err
		}
		if err := r.writeFinalGatewaySettlementJournal(prepared); err != nil {
			return err
		}
		return r.resumeFinalGatewaySettlement(ctx, prepared)
	})
}

func (r *Runtime) requireNoFinalGatewaySettlement(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, present, err := r.readFinalGatewaySettlementJournal(); err != nil {
		return fmt.Errorf("read interrupted final Gateway settlement: %w", err)
	} else if present {
		return fmt.Errorf("final Gateway settlement is interrupted; resume its exact initiating action before shared-cluster reconciliation")
	}
	return nil
}

func (r *Runtime) prepareFinalGatewaySettlement(
	ctx context.Context,
	previous, next tobari.WorkspaceAuthorityCollection,
	plan tobari.WorkspacePolicyProjection,
	operation, decisionRef string,
	reviewedSetDigest tobari.SemanticDigest,
) (finalGatewaySettlementJournal, error) {
	if err := r.ensureFinalClusterBaseComponents(ctx, plan, operation, decisionRef); err != nil {
		return finalGatewaySettlementJournal{}, err
	}
	if err := r.ensurePrivateDirectory(r.finalGatewaySettlementRoot()); err != nil {
		return finalGatewaySettlementJournal{}, err
	}
	parent := filepath.Dir(r.finalGatewaySettlementRoot())
	if err := requirePrivateDirectory(parent); err != nil {
		return finalGatewaySettlementJournal{}, err
	}
	if err := syncDirectory(parent); err != nil {
		return finalGatewaySettlementJournal{}, fmt.Errorf("publish final Gateway settlement root durably: %w", err)
	}
	if _, err := r.readFinalPolicyActivation(r.finalPolicyActivationJournalPath()); err == nil {
		return finalGatewaySettlementJournal{}, fmt.Errorf("an interrupted final policy activation must recover before Gateway settlement")
	} else if !errors.Is(err, os.ErrNotExist) {
		return finalGatewaySettlementJournal{}, fmt.Errorf("read interrupted final policy activation: %w", err)
	}
	previousActive, activePresent, err := r.readOptionalFinalPolicyActivation(r.finalPolicyActiveReceiptPath())
	if err != nil {
		return finalGatewaySettlementJournal{}, err
	}
	principals, networks, gateway, opa, err := r.observeFinalGatewaySettlementCandidate(ctx, plan)
	if err != nil {
		return finalGatewaySettlementJournal{}, err
	}
	aggregate, policyDigest, gatewayDigest, err := r.buildFinalSettlementArtifacts(ctx, plan)
	if err != nil {
		return finalGatewaySettlementJournal{}, err
	}
	profile, err := tobari.SharedClusterProfileForTransport(r.permissionIngestionTransport)
	if err != nil {
		return finalGatewaySettlementJournal{}, err
	}
	state, compose, candidateGatewayImage, candidateOPAImage, err := r.prepareFinalGatewayComposeAuthority(
		ctx, aggregate, profile, len(plan.Contexts),
	)
	if err != nil {
		return finalGatewaySettlementJournal{}, err
	}
	_ = state // state is reconstructed from the journaled closure during replacement.
	registry, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return finalGatewaySettlementJournal{}, err
	}
	candidate := finalGatewaySettlementCandidate{
		Plan: plan.Clone(), ReviewedSetDigest: reviewedSetDigest, Principals: principals, GatewayNetworks: networks,
		GatewayImageID: candidateGatewayImage, OPAImageID: candidateOPAImage, ReviewedGateway: gateway.ContainerID,
		GatewayEnv: selectedFinalGatewayEnvironment(profile), Profile: profile,
		Compose: compose, Aggregate: aggregate, PolicyArtifact: policyDigest, GatewayArtifact: gatewayDigest,
	}
	if brokerRuntimeEnabled {
		selection, selectErr := r.selectAuthBrokerImage(ctx)
		if selectErr != nil {
			return finalGatewaySettlementJournal{}, selectErr
		}
		if err := r.verifyAuthBrokerImage(ctx, selection.Image, selection.RequireDigest); err != nil {
			return finalGatewaySettlementJournal{}, err
		}
		candidate.AuthBrokerImage = selection.Image
		candidate.AuthBrokerImageID, err = r.resolveCandidateImageID(ctx, selection.Image)
		if err != nil {
			return finalGatewaySettlementJournal{}, err
		}
		broker, missing, observeErr := r.observeAppliedClusterComponent(ctx, "auth-broker", authBrokerContainer)
		if observeErr != nil {
			return finalGatewaySettlementJournal{}, observeErr
		}
		if !missing {
			candidate.AuthBrokerNetworks = componentNetworkRows(broker)
		} else if stopped, stoppedPresent, stoppedErr := r.readFinalClusterStopped(r.finalClusterStoppedReceiptPath()); stoppedErr != nil {
			return finalGatewaySettlementJournal{}, stoppedErr
		} else if stoppedPresent && stopped.AuthBroker != nil {
			candidate.AuthBrokerNetworks = componentNetworkRows(*stopped.AuthBroker)
		} else {
			for _, network := range finalAuthBrokerNetworkNames() {
				address, selectErr := r.selectFinalGatewayNetworkAddress(ctx, network, "")
				if selectErr != nil {
					return finalGatewaySettlementJournal{}, selectErr
				}
				candidate.AuthBrokerNetworks = append(candidate.AuthBrokerNetworks, FinalGatewayNetworkAddress{Name: network, Address: address})
			}
		}
	}
	opaControl := opa.NetworkAddresses["tobari-control"]
	if opaControl == "" {
		opaControl, err = r.selectFinalGatewayNetworkAddress(ctx, "tobari-control", "")
		if err != nil {
			return finalGatewaySettlementJournal{}, err
		}
	}
	candidate.OPANetworks = []FinalGatewayNetworkAddress{{Name: "tobari-control", Address: opaControl}}
	if err := candidate.validate(r); err != nil {
		return finalGatewaySettlementJournal{}, err
	}
	liveGatewayArtifactExact := false
	if verifySelectedFinalComponentClosure(gateway, opa, candidate) == nil {
		material, materialErr := reviewedFinalSettlementMaterial(plan, principals, gateway)
		if materialErr == nil {
			liveAggregate := candidate.Aggregate
			liveAggregate.MaterializedDigest = material.MaterializedDigest
			receipt, receiptErr := r.NewFinalAggregatePublicationReceipt(material, liveAggregate)
			if receiptErr == nil && receipt.GatewayArtifact == candidate.GatewayArtifact &&
				r.confirmLiveFinalGatewayArtifact(ctx, material, liveAggregate, receipt) == nil {
				liveGatewayArtifactExact = true
			}
		}
	}
	if brokerRuntimeEnabled && r.confirmSelectedFinalResearchClosure(ctx, candidate) != nil {
		liveGatewayArtifactExact = false
	}
	effectClass := classifyFinalSettlementEffect(activePresent, previousActive, candidate, registry, gateway, opa, liveGatewayArtifactExact)
	journal := finalGatewaySettlementJournal{
		SchemaVersion: finalGatewaySettlementSchema, Operation: operation, DecisionRef: decisionRef,
		EffectClass: effectClass, Phase: finalGatewayPhasePrepared,
		PreviousGeneration: previous.Generation, PreviousRevision: previous.Revision,
		NextGeneration: next.Generation, NextRevision: next.Revision,
		PreviousPrincipals: registry, Candidate: candidate,
	}
	if activePresent {
		copy := previousActive
		journal.PreviousActive = &copy
	}
	return journal, journal.validate(r)
}

func classifyFinalSettlementEffect(
	activePresent bool,
	active finalPolicyActivationRecord,
	candidate finalGatewaySettlementCandidate,
	registry projectPrincipalRegistry,
	gateway, opa appliedClusterComponentObservation,
	liveGatewayArtifactExact bool,
) string {
	if activePresent && liveGatewayArtifactExact && verifySelectedFinalComponentClosure(gateway, opa, candidate) == nil &&
		sameFinalSettlementContent(active, candidate, registry) {
		return finalGatewaySettlementOPA
	}
	return finalGatewaySettlementFull
}

func reviewedFinalSettlementMaterial(
	plan tobari.WorkspacePolicyProjection,
	principals []FinalWorkspacePrincipalRow,
	gateway appliedClusterComponentObservation,
) (FinalWorkspacePolicyProjection, error) {
	gatewayAuthority, err := finalGatewayComponentAuthority(gateway)
	if err != nil {
		return FinalWorkspacePolicyProjection{}, err
	}
	material := FinalWorkspacePolicyProjection{
		Plan: plan.Clone(), Principals: append([]FinalWorkspacePrincipalRow(nil), principals...), Gateway: gatewayAuthority,
	}
	material.MaterializedDigest, err = finalWorkspacePolicyProjectionDigest(material.Plan.Contexts, material.Principals, material.Gateway)
	if err != nil {
		return FinalWorkspacePolicyProjection{}, err
	}
	return material, material.Validate()
}

func sameFinalSettlementContent(active finalPolicyActivationRecord, candidate finalGatewaySettlementCandidate, registry projectPrincipalRegistry) bool {
	want := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: make([]projectPrincipalBinding, len(candidate.Principals))}
	for index, principal := range candidate.Principals {
		want.Bindings[index] = principal.gatewayBinding()
	}
	if !sameProjectPrincipalRegistry(registry, want) || active.Receipt.GatewayArtifact != candidate.GatewayArtifact || active.Receipt.PrincipalDigest == "" {
		return false
	}
	wantPrincipal, err := digestFinalValue(candidate.Principals)
	if err != nil || active.Receipt.PrincipalDigest != wantPrincipal {
		return false
	}
	return reflect.DeepEqual(active.Material.Gateway.Networks, candidate.GatewayNetworks)
}

func selectedFinalGatewayEnvironment(profile tobari.SharedClusterAppliedProfile) []string {
	selected := map[string]string{
		"HOME":                                   "/var/lib/mitmproxy",
		"TOBARI_ATTACHMENT_GRANT_REGISTRY":       "/run/tobari/host-loopback/grants.json",
		"TOBARI_CLUSTER":                         "default",
		"TOBARI_GATEWAY_CONFIG":                  "/run/tobari/config/gateway.json",
		"TOBARI_HOST_LOOPBACK_REGISTRY":          "/run/tobari/host-loopback/routes.json",
		"TOBARI_INTERACTIVE_ATTACHMENT_REGISTRY": "/run/tobari/interactive-attachments/sessions.json",
		"TOBARI_OPA_TIMEOUT_SECONDS":             environmentValueOrDefault("TOBARI_OPA_TIMEOUT_SECONDS", "2"),
		"TOBARI_OPA_URL":                         "http://opa:8181/v1/data/tobari/http/decision",
		"TOBARI_PRINCIPAL_REGISTRY":              "/run/tobari/principal-registry/principals.json",
		"TOBARI_UPSTREAM_TIMEOUT_SECONDS":        environmentValueOrDefault("TOBARI_UPSTREAM_TIMEOUT_SECONDS", "30"),
	}
	switch profile {
	case tobari.SharedClusterProfileUnix:
		selected["TOBARI_PERMISSION_INGESTION_TRANSPORT"] = "unix"
		selected["TOBARI_PERMISSION_INGESTION_DIRECTORY"] = "/run/tobari/permission-ingestion"
	case tobari.SharedClusterProfileLoopbackTCP:
		selected["TOBARI_PERMISSION_INGESTION_TRANSPORT"] = "loopback_tcp"
	}
	if brokerRuntimeEnabled {
		selected["TOBARI_AUTH_PROVIDER_PROJECTION"] = prePlatformAuthProviderProjection
		selected["TOBARI_AUTH_BROKER_SOCKET"] = prePlatformAuthBrokerSocket
		selected["TOBARI_AUTH_BROKER_TIMEOUT_SECONDS"] = environmentValueOrDefault("TOBARI_AUTH_BROKER_TIMEOUT_SECONDS", prePlatformAuthBrokerTimeout)
	}
	result := make([]string, 0, len(selected))
	for key, value := range selected {
		result = append(result, key+"="+value)
	}
	sort.Strings(result)
	return result
}

func environmentValueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func validateFinalGatewayEnvironment(environment []string, profile tobari.SharedClusterAppliedProfile) error {
	if profile.Validate() != nil || len(environment) == 0 || !sort.StringsAreSorted(environment) {
		return fmt.Errorf("selected final Gateway environment is invalid")
	}
	allowed := make(map[string]struct{}, len(selectedFinalGatewayEnvironment(profile)))
	for _, entry := range selectedFinalGatewayEnvironment(profile) {
		key, _, _ := strings.Cut(entry, "=")
		allowed[key] = struct{}{}
	}
	seen := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			return fmt.Errorf("selected final Gateway environment is invalid")
		}
		if _, exists := allowed[key]; !exists {
			return fmt.Errorf("selected final Gateway environment contains an unmanaged key")
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("selected final Gateway environment contains a duplicate key")
		}
		seen[key] = value
	}
	if len(seen) != len(allowed) {
		return fmt.Errorf("selected final Gateway environment is incomplete")
	}
	constants := selectedFinalGatewayEnvironment(profile)
	for _, entry := range constants {
		key, value, _ := strings.Cut(entry, "=")
		if key == "TOBARI_OPA_TIMEOUT_SECONDS" || key == "TOBARI_UPSTREAM_TIMEOUT_SECONDS" || key == "TOBARI_AUTH_BROKER_TIMEOUT_SECONDS" {
			continue
		}
		if seen[key] != value {
			return fmt.Errorf("selected final Gateway environment changes fixed runtime authority")
		}
	}
	for _, key := range []string{"TOBARI_OPA_TIMEOUT_SECONDS", "TOBARI_UPSTREAM_TIMEOUT_SECONDS"} {
		value, err := strconv.ParseUint(seen[key], 10, 31)
		if err != nil || value == 0 || key == "TOBARI_OPA_TIMEOUT_SECONDS" && value > 10 {
			return fmt.Errorf("selected final Gateway timeout is invalid")
		}
	}
	if brokerRuntimeEnabled {
		value, err := strconv.ParseUint(seen["TOBARI_AUTH_BROKER_TIMEOUT_SECONDS"], 10, 31)
		if err != nil || value < 70 || value > 90 {
			return fmt.Errorf("selected final Auth Broker timeout is invalid")
		}
	}
	return nil
}

func finalGatewayMountDestinations(profile tobari.SharedClusterAppliedProfile) []string {
	mounts := []string{
		finalGatewayInheritedCAPath,
		"/run/tobari/ca-public",
		"/run/tobari/config/gateway.json",
		"/run/tobari/host-loopback",
		"/run/tobari/interactive-attachments",
		"/run/tobari/principal-registry",
		"/tmp",
		"/var/lib/mitmproxy/.mitmproxy",
	}
	if profile == tobari.SharedClusterProfileUnix {
		mounts = append(mounts, "/run/tobari/permission-ingestion")
	}
	if brokerRuntimeEnabled {
		mounts = append(mounts, prePlatformAuthProviderMount, prePlatformAuthRuntimeMount)
	}
	sort.Strings(mounts)
	return mounts
}

func finalOPAMountDestinations() []string {
	return []string{"/bundle", "/tmp"}
}

func verifySelectedFinalComponentClosure(
	gateway, opa appliedClusterComponentObservation, candidate finalGatewaySettlementCandidate,
) error {
	if gateway.ImageID != candidate.GatewayImageID || opa.ImageID != candidate.OPAImageID {
		return fmt.Errorf("selected final component image identity is not applied")
	}
	if err := verifyAppliedPermissionProfile(gateway, candidate.Profile); err != nil {
		return err
	}
	if !slices.Equal(gateway.MountDestinations, finalGatewayMountDestinations(candidate.Profile)) ||
		!slices.Equal(opa.MountDestinations, finalOPAMountDestinations()) {
		return fmt.Errorf("selected final component mount closure is not applied")
	}
	wantEnvironment := make(map[string]string, len(candidate.GatewayEnv))
	for _, entry := range candidate.GatewayEnv {
		key, value, _ := strings.Cut(entry, "=")
		wantEnvironment[key] = value
	}
	observed := make(map[string]string, len(wantEnvironment))
	for _, entry := range gateway.Environment {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			return fmt.Errorf("applied Gateway environment is malformed")
		}
		if _, managed := wantEnvironment[key]; !managed {
			continue
		}
		if _, duplicate := observed[key]; duplicate {
			return fmt.Errorf("applied Gateway environment duplicates managed authority")
		}
		observed[key] = value
	}
	if !reflect.DeepEqual(observed, wantEnvironment) {
		return fmt.Errorf("selected final Gateway environment is not applied")
	}
	if !reflect.DeepEqual(componentNetworkRows(gateway), candidate.GatewayNetworks) ||
		!reflect.DeepEqual(componentNetworkRows(opa), candidate.OPANetworks) {
		return fmt.Errorf("selected final component network topology is not applied")
	}
	return nil
}

func componentNetworkRows(component appliedClusterComponentObservation) []FinalGatewayNetworkAddress {
	rows := make([]FinalGatewayNetworkAddress, 0, len(component.NetworkAddresses))
	for name, address := range component.NetworkAddresses {
		rows = append(rows, FinalGatewayNetworkAddress{Name: name, Address: address})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

func (r *Runtime) confirmSelectedFinalComponentClosure(ctx context.Context, candidate finalGatewaySettlementCandidate) error {
	firstGateway, firstOPA, err := r.observeSelectedFinalComponents(ctx)
	if err != nil {
		return err
	}
	secondGateway, secondOPA, err := r.observeSelectedFinalComponents(ctx)
	if err != nil {
		return err
	}
	if !sameAppliedClusterComponent(firstGateway, secondGateway) || !sameAppliedClusterComponent(firstOPA, secondOPA) {
		return fmt.Errorf("selected final component closure changed during confirmation")
	}
	if err := verifySelectedFinalComponentClosure(secondGateway, secondOPA, candidate); err != nil {
		return err
	}
	return r.confirmSelectedFinalResearchClosure(ctx, candidate)
}

func (r *Runtime) confirmSelectedFinalResearchClosure(ctx context.Context, candidate finalGatewaySettlementCandidate) error {
	if !brokerRuntimeEnabled {
		return nil
	}
	first, missing, err := r.observeAppliedClusterComponent(ctx, "auth-broker", authBrokerContainer)
	if err != nil || missing {
		return fmt.Errorf("observe selected final Auth Broker: %w", err)
	}
	second, missing, err := r.observeAppliedClusterComponent(ctx, "auth-broker", authBrokerContainer)
	if err != nil || missing || !sameAppliedClusterComponent(first, second) {
		return fmt.Errorf("selected final Auth Broker changed during confirmation: %w", err)
	}
	state, _, err := r.credentialCompanionStatus(ctx)
	if !selectedFinalResearchClosureExact(candidate, second, false, state, err) {
		return fmt.Errorf("selected final Auth Broker/companion closure is unhealthy or drifted: %w", err)
	}
	return nil
}

func selectedFinalResearchClosureExact(candidate finalGatewaySettlementCandidate, broker appliedClusterComponentObservation, missing bool, companionState string, err error) bool {
	return brokerRuntimeEnabled && err == nil && !missing && companionState == "ready" &&
		broker.Owner == ownerValue && broker.Component == "auth-broker" && broker.State == "running" && broker.Health == "healthy" &&
		broker.ImageID == candidate.AuthBrokerImageID && reflect.DeepEqual(componentNetworkRows(broker), candidate.AuthBrokerNetworks)
}

func (r *Runtime) validateFinalGatewayComposeCandidate(candidate finalGatewaySettlementCandidate) error {
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: candidate.Compose.RuntimeDirectory,
		AggregateRevision: candidate.Aggregate.AggregateRevision, ManifestCount: finalComposeProjectionCount(len(candidate.Plan.Contexts)),
		PolicyDirectory: candidate.Aggregate.PolicyDirectory, GatewayConfig: candidate.Aggregate.GatewayConfig,
		AssetVersion: candidate.Compose.AssetVersion,
	}
	if err := state.Validate(); err != nil {
		return err
	}
	return r.validateCandidateComposeClosure(state, candidate.Profile, candidate.Compose)
}

func (r *Runtime) waitForFinalSelectedComponents(ctx context.Context, candidate finalGatewaySettlementCandidate) error {
	const attempts = 60
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := r.confirmSelectedFinalComponentClosure(ctx, candidate); err == nil {
			return nil
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("selected final components did not become exactly healthy: %w", last)
}

func (r *Runtime) observeSelectedFinalComponents(ctx context.Context) (appliedClusterComponentObservation, appliedClusterComponentObservation, error) {
	gateway, missing, err := r.observeAppliedClusterComponent(ctx, "gateway", gatewayContainer)
	if err != nil || missing {
		return gateway, appliedClusterComponentObservation{}, fmt.Errorf("observe selected final Gateway: %w", err)
	}
	opa, missing, err := r.observeAppliedClusterComponent(ctx, "opa", opaContainer)
	if err != nil || missing {
		return gateway, opa, fmt.Errorf("observe selected final OPA: %w", err)
	}
	return gateway, opa, nil
}

func (r *Runtime) resumeFinalGatewaySettlement(ctx context.Context, journal finalGatewaySettlementJournal) error {
	if err := journal.validate(r); err != nil {
		return err
	}
	if bootstrap, present, err := r.readFinalClusterBootstrap(); err != nil {
		return err
	} else if present {
		if bootstrap.Operation != journal.Operation || bootstrap.DecisionRef != journal.DecisionRef {
			return fmt.Errorf("final cluster bootstrap crosses Gateway settlement recovery")
		}
	}
	if err := r.validateFinalGatewayComposeCandidate(journal.Candidate); err != nil {
		return err
	}
	if journal.EffectClass == finalGatewaySettlementOPA {
		return r.resumeFinalGatewayOPAOnly(ctx, journal)
	}
	if journal.Phase == finalGatewayPhaseReceipt {
		if journal.Applied == nil {
			return fmt.Errorf("terminal final Gateway settlement omits its applied receipt")
		}
		if err := r.confirmFinalPolicyRecord(ctx, *journal.Applied); err != nil {
			return err
		}
		if err := r.removeFinalClusterStopped(r.finalClusterStoppedReceiptPath()); err != nil {
			return err
		}
		if err := r.removeFinalClusterBootstrap(); err != nil {
			return err
		}
		return r.removeFinalGatewaySettlementJournal()
	}
	if journal.Phase == finalGatewayPhasePrepared {
		if err := r.ConfirmNoFinalWorkspaceSessions(ctx); err != nil {
			return err
		}
		if r.finalGatewayAfterFirstSessionFence != nil {
			r.finalGatewayAfterFirstSessionFence()
		}
		fenceDirectory, fenceRevision, cleanup, err := r.finalPolicyFenceTarget(journal.Candidate.Aggregate.AggregateRevision)
		if err != nil {
			return err
		}
		defer cleanup()
		if err := r.applyPolicyTarget(ctx, fenceDirectory, fenceRevision); err != nil {
			return err
		}
		journal.Phase = finalGatewayPhaseFenced
		if err := r.writeFinalGatewaySettlementJournal(journal); err != nil {
			return err
		}
	}
	if journal.Phase == finalGatewayPhaseFenced {
		if err := r.ConfirmNoFinalWorkspaceSessions(ctx); err != nil {
			return err
		}
		if err := r.publishFinalSettlementPrincipals(ctx, journal); err != nil {
			return err
		}
		if err := r.interruptFinalGatewaySettlement("principals_published"); err != nil {
			return err
		}
		journal.Phase = finalGatewayPhasePrincipals
		if err := r.writeFinalGatewaySettlementJournal(journal); err != nil {
			return err
		}
	}
	if journal.Phase == finalGatewayPhasePrincipals {
		applied, err := r.observeExactSettledGateway(ctx, journal.Candidate)
		if err != nil {
			if err := r.replaceFinalGateway(ctx, journal.Candidate); err != nil {
				return err
			}
			applied, err = r.observeExactSettledGateway(ctx, journal.Candidate)
			if err != nil {
				return err
			}
		}
		// A replacement creates a new network namespace. Reapply the reviewed
		// redirect/forwarding guard after topology and dependencies are exact,
		// and before this namespace can become published authority. Running the
		// idempotent guard on recovery also closes a failure after replacement
		// without repeating the physical component effect.
		if err := r.ensureGatewayNetworkGuard(ctx); err != nil {
			return err
		}
		if err := r.interruptFinalGatewaySettlement("components_replaced"); err != nil {
			return err
		}
		journal.Applied = &applied
		journal.Phase = finalGatewayPhaseReplaced
		if err := r.writeFinalGatewaySettlementJournal(journal); err != nil {
			return err
		}
	}
	if journal.Phase == finalGatewayPhaseReplaced {
		if journal.Applied == nil {
			return fmt.Errorf("final Gateway settlement lost its applied material")
		}
		if err := r.ApplyFinalAggregatePolicy(ctx, journal.Applied.Aggregate); err != nil {
			return err
		}
		if err := r.interruptFinalGatewaySettlement("policy_active"); err != nil {
			return err
		}
		journal.Phase = finalGatewayPhasePolicy
		if err := r.writeFinalGatewaySettlementJournal(journal); err != nil {
			return err
		}
	}
	if journal.Phase == finalGatewayPhasePolicy {
		if journal.Applied == nil {
			return fmt.Errorf("final Gateway settlement lost its publication receipt")
		}
		if err := r.writeFinalPolicyActivation(r.finalPolicyActiveReceiptPath(), *journal.Applied); err != nil {
			return err
		}
		if err := r.confirmFinalPolicyRecord(ctx, *journal.Applied); err != nil {
			return err
		}
		if err := r.interruptFinalGatewaySettlement("receipt_published"); err != nil {
			return err
		}
		journal.Phase = finalGatewayPhaseReceipt
		if err := r.writeFinalGatewaySettlementJournal(journal); err != nil {
			return err
		}
	}
	if err := r.removeFinalClusterStopped(r.finalClusterStoppedReceiptPath()); err != nil {
		return err
	}
	if err := r.removeFinalClusterBootstrap(); err != nil {
		return err
	}
	return r.removeFinalGatewaySettlementJournal()
}

func (r *Runtime) interruptFinalGatewaySettlement(boundary string) error {
	if r.finalGatewayAfterEffect == nil {
		return nil
	}
	return r.finalGatewayAfterEffect(boundary)
}

func (r *Runtime) resumeFinalGatewayOPAOnly(ctx context.Context, journal finalGatewaySettlementJournal) error {
	if journal.Phase == finalGatewayPhaseReceipt {
		return r.removeFinalGatewaySettlementJournal()
	}
	if err := r.confirmSelectedFinalComponentClosure(ctx, journal.Candidate); err != nil {
		return err
	}
	if err := r.validateFinalGatewayComposeCandidate(journal.Candidate); err != nil {
		return err
	}
	record, err := r.prepareFinalPolicyActivation(ctx, journal.Candidate.Plan, journal.Candidate.ReviewedSetDigest)
	if err != nil {
		return err
	}
	if record.Material.Gateway.ContainerID != journal.Candidate.ReviewedGateway ||
		!reflect.DeepEqual(record.Material.Principals, journal.Candidate.Principals) {
		return fmt.Errorf("OPA-only settlement authority changed after effect classification")
	}
	if record.Receipt.GatewayArtifact != journal.Candidate.GatewayArtifact || record.Receipt.PolicyArtifact != journal.Candidate.PolicyArtifact {
		return fmt.Errorf("OPA-only settlement content changed after effect classification")
	}
	if err := r.resumeFinalPolicyActivation(ctx, record); err != nil {
		return err
	}
	journal.Applied = &record
	journal.Phase = finalGatewayPhaseReceipt
	if err := r.writeFinalGatewaySettlementJournal(journal); err != nil {
		return err
	}
	return r.removeFinalGatewaySettlementJournal()
}

func (r *Runtime) observeExactSettledGateway(ctx context.Context, candidate finalGatewaySettlementCandidate) (finalPolicyActivationRecord, error) {
	if err := r.confirmSelectedFinalComponentClosure(ctx, candidate); err != nil {
		return finalPolicyActivationRecord{}, err
	}
	material, err := r.ObserveFinalWorkspacePolicyProjection(ctx, candidate.Plan)
	if err != nil {
		return finalPolicyActivationRecord{}, err
	}
	if !reflect.DeepEqual(material.Principals, candidate.Principals) || material.Gateway.ImageID != candidate.GatewayImageID || !reflect.DeepEqual(material.Gateway.Networks, candidate.GatewayNetworks) {
		return finalPolicyActivationRecord{}, fmt.Errorf("settled Gateway does not match the exact candidate topology or component image")
	}
	opa, missing, err := r.observeAppliedClusterComponent(ctx, "opa", opaContainer)
	if err != nil || missing || opa.ImageID != candidate.OPAImageID || opa.State != "running" || opa.Health != "healthy" {
		return finalPolicyActivationRecord{}, fmt.Errorf("settled OPA does not match the exact selected healthy component: %w", err)
	}
	opaNetworks := make([]FinalGatewayNetworkAddress, 0, len(opa.NetworkAddresses))
	for name, address := range opa.NetworkAddresses {
		opaNetworks = append(opaNetworks, FinalGatewayNetworkAddress{Name: name, Address: address})
	}
	sort.Slice(opaNetworks, func(i, j int) bool { return opaNetworks[i].Name < opaNetworks[j].Name })
	if !reflect.DeepEqual(opaNetworks, candidate.OPANetworks) {
		return finalPolicyActivationRecord{}, fmt.Errorf("settled OPA topology differs from the selected runtime closure")
	}
	aggregate, err := r.BuildFinalAggregateProjection(ctx, material)
	if err != nil {
		return finalPolicyActivationRecord{}, err
	}
	if aggregate.AggregateRevision != candidate.Aggregate.AggregateRevision || aggregate.PolicyDirectory != candidate.Aggregate.PolicyDirectory || aggregate.GatewayConfig != candidate.Aggregate.GatewayConfig {
		return finalPolicyActivationRecord{}, fmt.Errorf("settled Gateway aggregate differs from the reviewed candidate")
	}
	receipt, err := r.NewFinalAggregatePublicationReceipt(material, aggregate)
	if err != nil {
		return finalPolicyActivationRecord{}, err
	}
	record := finalPolicyActivationRecord{
		SchemaVersion: finalPolicyActivationSchema, ReviewedSetDigest: candidate.ReviewedSetDigest,
		Material: material, Aggregate: aggregate, Receipt: receipt,
	}
	if receipt.PolicyArtifact != candidate.PolicyArtifact || receipt.GatewayArtifact != candidate.GatewayArtifact {
		return finalPolicyActivationRecord{}, fmt.Errorf("settled Gateway artifacts differ from the reviewed candidate")
	}
	if err := r.confirmLiveFinalGatewayArtifact(ctx, material, aggregate, receipt); err != nil {
		return finalPolicyActivationRecord{}, err
	}
	return record, record.validate(r)
}

func (r *Runtime) publishFinalSettlementPrincipals(ctx context.Context, journal finalGatewaySettlementJournal) error {
	want := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: make([]projectPrincipalBinding, len(journal.Candidate.Principals))}
	for index, principal := range journal.Candidate.Principals {
		want.Bindings[index] = principal.gatewayBinding()
	}
	current, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return err
	}
	if sameProjectPrincipalRegistry(current, want) {
		return nil
	}
	if !sameProjectPrincipalRegistry(current, journal.PreviousPrincipals) {
		return fmt.Errorf("principal registry changed outside the exact final Gateway decision")
	}
	if err := r.replaceProjectPrincipalRegistryIfCurrent(ctx, journal.PreviousPrincipals, want.Bindings); err != nil {
		return err
	}
	observed, err := r.readProjectPrincipalRegistry()
	if err != nil || !sameProjectPrincipalRegistry(observed, want) {
		return fmt.Errorf("confirm exact final principal registry publication: %w", err)
	}
	return nil
}

func (r *Runtime) replaceFinalGateway(ctx context.Context, candidate finalGatewaySettlementCandidate) error {
	if r.finalGatewayReplaceComponents != nil {
		return r.finalGatewayReplaceComponents(ctx, candidate)
	}
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: candidate.Compose.RuntimeDirectory,
		AggregateRevision: candidate.Aggregate.AggregateRevision, ManifestCount: finalComposeProjectionCount(len(candidate.Plan.Contexts)),
		PolicyDirectory: candidate.Aggregate.PolicyDirectory, GatewayConfig: candidate.Aggregate.GatewayConfig,
		AssetVersion: candidate.Compose.AssetVersion,
	}
	if err := state.Validate(); err != nil {
		return err
	}
	if err := r.validateCandidateComposeClosure(state, candidate.Profile, candidate.Compose); err != nil {
		return err
	}
	transport, ok := candidate.Profile.PermissionTransport()
	if !ok {
		return fmt.Errorf("final Gateway settlement profile is invalid")
	}
	environment, err := r.composeEnvironmentForTransport(state, transport)
	if err != nil {
		return err
	}
	environment = finalGatewayReplacementEnvironment(environment, candidate)
	args := []string{"compose", "--project-directory", state.RuntimeDirectory}
	args = append(args, composeFileArgs(state.RuntimeDirectory)...)
	profileArgs, err := permissionSessionComposeFileArgsForTransport(state.RuntimeDirectory, transport)
	if err != nil {
		return err
	}
	args = append(args, profileArgs...)
	// OPA already belongs to the selected candidate closure. Recreating it
	// while Gateway is replaced removes the stable Docker DNS endpoint that
	// the new Gateway must use for its first health and policy checks.
	args = append(args, "up", "-d", "--no-build", "--no-deps", "--force-recreate", "gateway")
	if brokerRuntimeEnabled {
		args = append(args, "auth-broker")
	}
	var output bytes.Buffer
	if err := r.runner.Run(ctx, args, environment, nil, &output, &output); err != nil {
		return fmt.Errorf("replace final Gateway: %w: %s", err, boundedDiagnostic(output.Bytes()))
	}
	if err := r.reconcileFinalComponentTopology(ctx, gatewayContainer, "gateway", candidate.GatewayNetworks); err != nil {
		return err
	}
	if brokerRuntimeEnabled {
		if err := r.reconcileFinalComponentTopology(ctx, authBrokerContainer, "auth-broker", candidate.AuthBrokerNetworks); err != nil {
			return err
		}
		rootKey, err := r.unlockAuthBroker(ctx)
		if err != nil {
			return err
		}
		defer clear(rootKey)
		if err := r.startCredentialCompanion(ctx, rootKey); err != nil {
			return err
		}
	}
	// Compose starts Gateway before the exact post-effect aliases and, on the
	// research surface, the companion are restored. Restart only the replaced
	// Gateway after those dependencies are exact; the selected OPA stays live.
	if err := r.restartFinalReplacementComponents(ctx, []string{gatewayContainer}); err != nil {
		return err
	}
	if err := r.waitForFinalSelectedComponents(ctx, candidate); err != nil {
		return err
	}
	if brokerRuntimeEnabled {
		broker, missing, err := r.observeAppliedClusterComponent(ctx, "auth-broker", authBrokerContainer)
		state, _, companionErr := r.credentialCompanionStatus(ctx)
		if err != nil || missing || broker.ImageID != candidate.AuthBrokerImageID || !reflect.DeepEqual(componentNetworkRows(broker), candidate.AuthBrokerNetworks) || companionErr != nil || state != "ready" {
			return fmt.Errorf("restored final research cluster closure is not exactly ready")
		}
	}
	return nil
}

func finalGatewayReplacementEnvironment(environment []string, candidate finalGatewaySettlementCandidate) []string {
	result := replaceEnvironmentValue(environment, "TOBARI_GATEWAY_IMAGE", candidate.GatewayImageID)
	result = replaceEnvironmentValue(result, "TOBARI_OPA_IMAGE", candidate.OPAImageID)
	if brokerRuntimeEnabled {
		result = replaceEnvironmentValue(result, "TOBARI_AUTH_BROKER_IMAGE", candidate.AuthBrokerImageID)
	}
	return applyFinalGatewayComposeEnvironment(result, candidate.GatewayEnv)
}

func applyFinalGatewayComposeEnvironment(environment, selected []string) []string {
	result := append([]string(nil), environment...)
	for _, entry := range selected {
		key, value, _ := strings.Cut(entry, "=")
		switch key {
		case "TOBARI_OPA_TIMEOUT_SECONDS", "TOBARI_UPSTREAM_TIMEOUT_SECONDS", "TOBARI_AUTH_BROKER_TIMEOUT_SECONDS":
			result = replaceEnvironmentValue(result, key, value)
		}
	}
	// Every other selected value is fixed inside the Compose service. Keeping
	// it out of the host process environment preserves HOME (Docker's plugin
	// discovery root) and host bind-source variables such as GatewayConfig.
	return result
}

func applyFinalGatewayEnvironment(environment, selected []string) []string {
	result := append([]string(nil), environment...)
	for _, entry := range selected {
		key, value, _ := strings.Cut(entry, "=")
		result = replaceEnvironmentValue(result, key, value)
	}
	return result
}

func (r *Runtime) reconcileFinalComponentTopology(ctx context.Context, container, alias string, expected []FinalGatewayNetworkAddress) error {
	want := make(map[string]string, len(expected))
	for _, network := range expected {
		want[network.Name] = network.Address
	}
	current, err := r.containerNetworkAddresses(ctx, container, alias)
	if err != nil {
		return err
	}
	for network, address := range current {
		if wanted, exists := want[network]; exists && wanted == address {
			continue
		}
		if err := r.runBoundedNetworkMutation(ctx, []string{"network", "disconnect", "-f", network, container}); err != nil {
			return fmt.Errorf("disconnect stale final %s network: %w", alias, err)
		}
	}
	current, err = r.containerNetworkAddresses(ctx, container, alias)
	if err != nil {
		return err
	}
	for _, network := range expected {
		if current[network.Name] == network.Address {
			continue
		}
		if err := r.runBoundedNetworkMutation(ctx, []string{"network", "connect", "--alias", alias, "--ip", network.Address, network.Name, container}); err != nil {
			return fmt.Errorf("connect exact final %s network: %w", alias, err)
		}
	}
	return r.confirmFinalComponentTopology(ctx, container, alias, expected)
}

func (r *Runtime) confirmFinalComponentTopology(ctx context.Context, container, alias string, expected []FinalGatewayNetworkAddress) error {
	networks, err := r.observeClusterContainerNetworks(ctx, container)
	if err != nil {
		return fmt.Errorf("confirm final %s network topology: %w", alias, err)
	}
	if len(networks) != len(expected) {
		return fmt.Errorf("final %s network topology contains an extra or missing attachment", alias)
	}
	for _, want := range expected {
		raw, present := networks[want.Name]
		var endpoint struct {
			IPAddress string   `json:"IPAddress"`
			Aliases   []string `json:"Aliases"`
		}
		if !present || json.Unmarshal(raw, &endpoint) != nil || endpoint.IPAddress != want.Address || !slices.Contains(endpoint.Aliases, alias) {
			return fmt.Errorf("final %s network attachment or alias differs from selected authority", alias)
		}
	}
	return nil
}

func (r *Runtime) restartFinalReplacementComponents(ctx context.Context, containers []string) error {
	restartContext, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	stdout := &boundedBuffer{limit: appliedClusterInspectLimit / 2}
	stderr := &boundedBuffer{limit: appliedClusterInspectLimit / 2}
	args := append([]string{"container", "restart"}, containers...)
	if err := r.runner.Run(restartContext, args, os.Environ(), nil, stdout, stderr); err != nil {
		return fmt.Errorf("restart exact final replacement components: %w: %s", err, boundedDiagnostic(stderr.buffer.Bytes()))
	}
	if stdout.overflow || stderr.overflow || len(bytes.TrimSpace(stderr.buffer.Bytes())) != 0 {
		return fmt.Errorf("final replacement component restart output is ambiguous")
	}
	if lines := strings.Fields(stdout.buffer.String()); !slices.Equal(lines, containers) {
		return fmt.Errorf("final replacement component restart confirmation is incomplete")
	}
	return nil
}

func (r *Runtime) observeFinalGatewaySettlementCandidate(
	ctx context.Context, plan tobari.WorkspacePolicyProjection,
) ([]FinalWorkspacePrincipalRow, []FinalGatewayNetworkAddress, appliedClusterComponentObservation, appliedClusterComponentObservation, error) {
	firstPrincipals, firstNetworks, firstGateway, firstOPA, err := r.observeFinalGatewaySettlementCandidatePass(ctx, plan)
	if err != nil {
		return nil, nil, appliedClusterComponentObservation{}, appliedClusterComponentObservation{}, err
	}
	secondPrincipals, secondNetworks, secondGateway, secondOPA, err := r.observeFinalGatewaySettlementCandidatePass(ctx, plan)
	if err != nil {
		return nil, nil, appliedClusterComponentObservation{}, appliedClusterComponentObservation{}, err
	}
	if !reflect.DeepEqual(firstPrincipals, secondPrincipals) || !reflect.DeepEqual(firstNetworks, secondNetworks) ||
		!sameAppliedClusterComponent(firstGateway, secondGateway) || !sameAppliedClusterComponent(firstOPA, secondOPA) {
		return nil, nil, appliedClusterComponentObservation{}, appliedClusterComponentObservation{}, fmt.Errorf("final Gateway settlement authority changed between complete observations")
	}
	return secondPrincipals, secondNetworks, secondGateway, secondOPA, nil
}

func (r *Runtime) observeFinalGatewaySettlementCandidatePass(
	ctx context.Context, plan tobari.WorkspacePolicyProjection,
) ([]FinalWorkspacePrincipalRow, []FinalGatewayNetworkAddress, appliedClusterComponentObservation, appliedClusterComponentObservation, error) {
	gateway, missing, err := r.observeAppliedClusterComponent(ctx, "gateway", gatewayContainer)
	if missing && err == nil {
		stopped, stoppedPresent, stoppedErr := r.readFinalClusterStopped(r.finalClusterStoppedReceiptPath())
		if stoppedErr != nil {
			return nil, nil, gateway, appliedClusterComponentObservation{}, stoppedErr
		}
		if stoppedPresent {
			for _, item := range plan.Contexts {
				if item.Principal != nil {
					return nil, nil, gateway, appliedClusterComponentObservation{}, fmt.Errorf("stopped final cluster cannot recover a live Workspace principal")
				}
			}
			return []FinalWorkspacePrincipalRow{}, componentNetworkRows(stopped.Gateway), stopped.Gateway, stopped.OPA, nil
		}
	}
	if err != nil || missing || gateway.State != "running" || gateway.Health != "healthy" {
		return nil, nil, gateway, appliedClusterComponentObservation{}, fmt.Errorf("observe selected healthy Gateway: %w", err)
	}
	opa, missing, err := r.observeAppliedClusterComponent(ctx, "opa", opaContainer)
	if err != nil || missing || opa.State != "running" || opa.Health != "healthy" {
		return nil, nil, gateway, opa, fmt.Errorf("observe selected healthy OPA: %w", err)
	}
	rows := make([]FinalWorkspacePrincipalRow, 0)
	networks := map[string]string{}
	for _, shared := range []string{"tobari-control", "tobari-egress"} {
		address := gateway.NetworkAddresses[shared]
		if address == "" {
			address, err = r.selectFinalGatewayNetworkAddress(ctx, shared, "")
			if err != nil {
				return nil, nil, gateway, opa, fmt.Errorf("select exact %s Gateway address: %w", shared, err)
			}
		}
		networks[shared] = address
	}
	for _, item := range plan.Contexts {
		if item.Principal == nil {
			continue
		}
		authority := *item.Principal
		container, network, err := tobari.ProjectResourceNames(string(authority.WorkspaceID))
		if err != nil {
			return nil, nil, gateway, opa, err
		}
		if err := r.verifyOwnedProjectResource(ctx, "network", network, string(authority.WorkspaceID), projectNetRole); err != nil {
			return nil, nil, gateway, opa, err
		}
		observation, err := r.observeFinalWorkspaceContainer(ctx, container, authority)
		if err != nil {
			return nil, nil, gateway, opa, err
		}
		workspaceAddress, err := r.workspaceNetworkAddress(ctx, container, network)
		if err != nil {
			return nil, nil, gateway, opa, err
		}
		gatewayAddress := gateway.NetworkAddresses[network]
		if gatewayAddress == "" {
			gatewayAddress, err = r.selectFinalGatewayNetworkAddress(ctx, network, workspaceAddress)
			if err != nil {
				return nil, nil, gateway, opa, err
			}
		}
		subnet, err := r.projectNetworkSubnet(ctx, network)
		if err != nil || validateProjectNetworkEndpoints(subnet, workspaceAddress, gatewayAddress) != nil {
			return nil, nil, gateway, opa, fmt.Errorf("validate final Workspace network endpoints: %w", err)
		}
		networks[network] = gatewayAddress
		rows = append(rows, FinalWorkspacePrincipalRow{
			ContextID: authority.ContextID, WorkspaceID: authority.WorkspaceID, TemplateID: authority.TemplateID,
			Presentation: authority.Presentation, ProjectRoot: authority.ProjectRoot,
			ContainerID: observation.ID, ResolvedSpec: authority.AppliedEntry.ResolvedSpec,
			WorkspaceIP: workspaceAddress, GatewayIP: gatewayAddress, Network: network,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].WorkspaceID < rows[j].WorkspaceID })
	topology := make([]FinalGatewayNetworkAddress, 0, len(networks))
	for name, address := range networks {
		topology = append(topology, FinalGatewayNetworkAddress{Name: name, Address: address})
	}
	sort.Slice(topology, func(i, j int) bool { return topology[i].Name < topology[j].Name })
	if err := validateFinalSettlementPrincipals(plan, rows); err != nil {
		return nil, nil, gateway, opa, err
	}
	return rows, topology, gateway, opa, nil
}

func (r *Runtime) observeFinalWorkspaceContainer(ctx context.Context, container string, authority tobari.WorkspacePolicyPrincipalAuthority) (finalWorkspaceContainerObservation, error) {
	format := `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
		`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},` +
		`"spec":{{json (index .Config.Labels "` + projectSpecLabel + `")}},` +
		`"running":{{json .State.Running}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}"none"{{end}}}`
	output, err := r.runner.Output(ctx, []string{"container", "inspect", "--format", format, container}, os.Environ())
	if err != nil {
		return finalWorkspaceContainerObservation{}, fmt.Errorf("observe final Workspace container: %w: %s", err, boundedDiagnostic(output))
	}
	var observed finalWorkspaceContainerObservation
	if err := decodeStrictJSON(output, &observed); err != nil {
		return observed, err
	}
	if err := observed.validateFor(authority.WorkspaceID, authority.AppliedEntry.ResolvedSpec, ""); err != nil {
		return observed, err
	}
	return observed, nil
}

func (r *Runtime) selectFinalGatewayNetworkAddress(ctx context.Context, network, workspaceAddress string) (string, error) {
	output, err := r.runner.Output(ctx, []string{"network", "inspect", "--format", `{"ipam":{{json .IPAM.Config}},"containers":{{json .Containers}}}`, network}, os.Environ())
	if err != nil {
		return "", fmt.Errorf("observe final Workspace network allocation: %w: %s", err, boundedDiagnostic(output))
	}
	var observed struct {
		IPAM []struct {
			Subnet  string `json:"Subnet"`
			Gateway string `json:"Gateway"`
		} `json:"ipam"`
		Containers map[string]struct {
			Name        string `json:"Name"`
			EndpointID  string `json:"EndpointID"`
			MacAddress  string `json:"MacAddress"`
			IPv4Address string `json:"IPv4Address"`
			IPv6Address string `json:"IPv6Address"`
		} `json:"containers"`
	}
	if err := decodeStrictJSON(output, &observed); err != nil || len(observed.IPAM) != 1 {
		return "", fmt.Errorf("decode final Workspace network allocation: %w", err)
	}
	prefix, err := netip.ParsePrefix(observed.IPAM[0].Subnet)
	if err != nil || !prefix.Addr().Is4() {
		return "", fmt.Errorf("final Workspace network subnet is invalid")
	}
	used := map[netip.Addr]struct{}{}
	for _, raw := range []string{workspaceAddress, observed.IPAM[0].Gateway} {
		if address, parseErr := netip.ParseAddr(strings.Split(raw, "/")[0]); parseErr == nil {
			used[address] = struct{}{}
		}
	}
	for _, endpoint := range observed.Containers {
		if address, parseErr := netip.ParseAddr(strings.Split(endpoint.IPv4Address, "/")[0]); parseErr == nil {
			used[address] = struct{}{}
		}
	}
	address := prefix.Masked().Addr().Next()
	for attempts := 0; attempts < 65536 && prefix.Contains(address); attempts++ {
		if _, occupied := used[address]; !occupied && address.IsGlobalUnicast() {
			return address.String(), nil
		}
		address = address.Next()
	}
	return "", fmt.Errorf("final Workspace network has no bounded free Gateway address")
}

func validateFinalSettlementPrincipals(plan tobari.WorkspacePolicyProjection, principals []FinalWorkspacePrincipalRow) error {
	expected := map[tobari.WorkspaceID]tobari.WorkspacePolicyPrincipalAuthority{}
	for _, item := range plan.Contexts {
		if item.Principal != nil {
			expected[item.Principal.WorkspaceID] = *item.Principal
		}
	}
	previous := tobari.WorkspaceID("")
	bindings := make([]projectPrincipalBinding, 0, len(principals))
	for _, principal := range principals {
		if previous != "" && principal.WorkspaceID <= previous {
			return fmt.Errorf("final Gateway settlement principals are not unique and sorted")
		}
		authority, exists := expected[principal.WorkspaceID]
		if !exists || principal.validateFor(authority) != nil {
			return fmt.Errorf("final Gateway settlement principal crosses complete authority")
		}
		delete(expected, principal.WorkspaceID)
		bindings = append(bindings, principal.gatewayBinding())
		previous = principal.WorkspaceID
	}
	if len(expected) != 0 {
		return fmt.Errorf("final Gateway settlement omits a principal")
	}
	return (projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: bindings}).Validate()
}

func (r *Runtime) buildFinalSettlementArtifacts(ctx context.Context, plan tobari.WorkspacePolicyProjection) (FinalAggregateProjection, tobari.SemanticDigest, tobari.SemanticDigest, error) {
	items := make([]aggregateContext, 0, len(plan.Contexts))
	for _, authority := range plan.Contexts {
		item, err := finalAggregateContext(authority)
		if err != nil {
			return FinalAggregateProjection{}, "", "", err
		}
		if err := r.testFinalContextPolicy(ctx, authority, item); err != nil {
			return FinalAggregateProjection{}, "", "", err
		}
		items = append(items, item)
	}
	projection, err := r.materializeAggregateProjection(ctx, items, nil)
	if err != nil {
		return FinalAggregateProjection{}, "", "", err
	}
	aggregate := FinalAggregateProjection{
		AggregateRevision: projection.Revision, PolicyDirectory: projection.PolicyDirectory,
		GatewayConfig: projection.GatewayConfig, MaterializedDigest: plan.ContentDigest,
	}
	policyDigest, err := digestFinalArtifactTree(aggregate.PolicyDirectory, 64*1024*1024)
	if err != nil {
		return FinalAggregateProjection{}, "", "", err
	}
	gateway, err := readOwnerPolicyFile(aggregate.GatewayConfig, 256*1024)
	if err != nil {
		return FinalAggregateProjection{}, "", "", err
	}
	gatewayDigest, err := digestFinalArtifactBytes(gateway)
	return aggregate, policyDigest, gatewayDigest, err
}

func digestFinalArtifactBytes(data []byte) (tobari.SemanticDigest, error) {
	digest := sha256.Sum256(data)
	return tobari.SemanticDigest("sha256:" + hex.EncodeToString(digest[:])), nil
}

func (r *Runtime) prepareFinalGatewayComposeAuthority(
	ctx context.Context, aggregate FinalAggregateProjection, profile tobari.SharedClusterAppliedProfile,
	contextCount int,
) (tobari.State, candidateComposeClosureReceipt, string, string, error) {
	version, err := runtimeassets.Version()
	if err != nil {
		return tobari.State{}, candidateComposeClosureReceipt{}, "", "", err
	}
	runtimeDirectory := filepath.Join(r.stateDirectory, "runtime", version)
	if err := runtimeassets.Materialize(runtimeDirectory); err != nil {
		return tobari.State{}, candidateComposeClosureReceipt{}, "", "", err
	}
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: runtimeDirectory,
		AggregateRevision: aggregate.AggregateRevision, PolicyDirectory: aggregate.PolicyDirectory,
		GatewayConfig: aggregate.GatewayConfig, AssetVersion: version,
	}
	state.ManifestCount = finalComposeProjectionCount(contextCount)
	if err := state.Validate(); err != nil {
		return tobari.State{}, candidateComposeClosureReceipt{}, "", "", err
	}
	_, compose, err := r.captureCandidateComposeClosure(state, profile)
	if err != nil {
		return tobari.State{}, candidateComposeClosureReceipt{}, "", "", err
	}
	_, gatewayImageID, err := r.prepareGatewayImage(ctx)
	if err != nil {
		return tobari.State{}, candidateComposeClosureReceipt{}, "", "", err
	}
	versions, err := runtimeassets.Versions()
	if err != nil {
		return tobari.State{}, candidateComposeClosureReceipt{}, "", "", err
	}
	opaImageID, err := r.resolveCandidateImageID(ctx, versions["OPA_IMAGE"])
	if err != nil {
		return tobari.State{}, candidateComposeClosureReceipt{}, "", "", err
	}
	return state, compose, gatewayImageID, opaImageID, nil
}

// finalComposeProjectionCount satisfies the predecessor State validator used
// only to derive and verify Compose assets. Final selected content remains
// authoritative in the complete projection and may validly contain zero
// active Contexts; this sentinel is never persisted as final authority.
func finalComposeProjectionCount(contextCount int) int {
	if contextCount == 0 {
		return 1
	}
	return contextCount
}

func (r *Runtime) readOptionalFinalPolicyActivation(path string) (finalPolicyActivationRecord, bool, error) {
	record, err := r.readFinalPolicyActivation(path)
	if errors.Is(err, os.ErrNotExist) {
		return finalPolicyActivationRecord{}, false, nil
	}
	return record, err == nil, err
}

func (r *Runtime) writeFinalGatewaySettlementJournal(journal finalGatewaySettlementJournal) error {
	if err := journal.validate(r); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return err
	}
	if int64(len(encoded)+1) > maxFinalPolicyActivationBytes {
		return fmt.Errorf("final Gateway settlement journal exceeds %d bytes", maxFinalPolicyActivationBytes)
	}
	return writeAtomicJSON(r.finalGatewaySettlementJournalPath(), journal)
}

func (r *Runtime) readFinalGatewaySettlementJournal() (finalGatewaySettlementJournal, bool, error) {
	path := r.finalGatewaySettlementJournalPath()
	file, err := os.Open(path) // #nosec G304 -- fixed owner-only settlement child.
	if errors.Is(err, os.ErrNotExist) {
		return finalGatewaySettlementJournal{}, false, nil
	}
	if err != nil {
		return finalGatewaySettlementJournal{}, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > maxFinalPolicyActivationBytes {
		return finalGatewaySettlementJournal{}, false, fmt.Errorf("final Gateway settlement journal is unsafe: %w", err)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, pathInfo) {
		return finalGatewaySettlementJournal{}, false, fmt.Errorf("final Gateway settlement journal path changed during open: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFinalPolicyActivationBytes+1))
	if err != nil || len(data) > maxFinalPolicyActivationBytes {
		return finalGatewaySettlementJournal{}, false, fmt.Errorf("read final Gateway settlement journal: %w", err)
	}
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return finalGatewaySettlementJournal{}, false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var journal finalGatewaySettlementJournal
	if err := decoder.Decode(&journal); err != nil {
		return journal, false, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return journal, false, fmt.Errorf("final Gateway settlement journal contains trailing data")
	}
	return journal, true, journal.validate(r)
}

func (r *Runtime) removeFinalGatewaySettlementJournal() error {
	if err := os.Remove(r.finalGatewaySettlementJournalPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(r.finalGatewaySettlementRoot())
}
