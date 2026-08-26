package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/companionruntime"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const (
	finalClusterStoppedSchema = 1
	finalClusterDownPrepared  = "prepared"
	finalClusterDownRuntime   = "runtime_removed"
	finalClusterDownAuthority = "authority_retired"
)

type finalClusterStoppedReceipt struct {
	SchemaVersion      int                                 `json:"schema_version"`
	Operation          string                              `json:"operation"`
	DecisionRef        string                              `json:"decision_ref"`
	Phase              string                              `json:"phase"`
	PreviousGeneration uint64                              `json:"previous_generation"`
	PreviousRevision   tobari.SemanticDigest               `json:"previous_revision"`
	NextGeneration     uint64                              `json:"next_generation"`
	NextRevision       tobari.SemanticDigest               `json:"next_revision"`
	Active             finalPolicyActivationRecord         `json:"active"`
	Gateway            appliedClusterComponentObservation  `json:"gateway"`
	OPA                appliedClusterComponentObservation  `json:"opa"`
	AuthBroker         *appliedClusterComponentObservation `json:"auth_broker,omitempty"`
	CompanionState     string                              `json:"companion_state,omitempty"`
}

func (r *Runtime) finalClusterDownJournalPath() string {
	return filepath.Join(r.finalGatewaySettlementRoot(), "down.json")
}
func (r *Runtime) finalClusterStoppedReceiptPath() string {
	return filepath.Join(r.finalGatewaySettlementRoot(), "stopped.json")
}

func (v finalClusterStoppedReceipt) validate(r *Runtime) error {
	if v.SchemaVersion != finalClusterStoppedSchema || v.Operation == "" || v.DecisionRef == "" ||
		v.PreviousGeneration == 0 || v.PreviousRevision.Validate() != nil || v.NextGeneration == 0 || v.NextRevision.Validate() != nil ||
		v.Active.validate(r) != nil || !containerIDPattern.MatchString(v.Gateway.ContainerID) || !containerIDPattern.MatchString(v.OPA.ContainerID) ||
		v.Gateway.Owner != ownerValue || v.Gateway.Component != "gateway" || v.Gateway.Role != gatewayRole ||
		v.OPA.Owner != ownerValue || v.OPA.Component != "opa" || !imageIDPattern.MatchString(v.Gateway.ImageID) || !imageIDPattern.MatchString(v.OPA.ImageID) {
		return fmt.Errorf("final cluster stopped receipt is invalid")
	}
	if brokerRuntimeEnabled {
		if v.AuthBroker == nil || v.AuthBroker.Owner != ownerValue || v.AuthBroker.Component != "auth-broker" || !containerIDPattern.MatchString(v.AuthBroker.ContainerID) || !imageIDPattern.MatchString(v.AuthBroker.ImageID) || v.CompanionState != "ready" {
			return fmt.Errorf("final cluster stopped research closure is incomplete")
		}
	} else if v.AuthBroker != nil || v.CompanionState != "" {
		return fmt.Errorf("release final cluster stopped receipt contains research authority")
	}
	switch v.Phase {
	case finalClusterDownPrepared, finalClusterDownRuntime, finalClusterDownAuthority:
	default:
		return fmt.Errorf("final cluster down phase is invalid")
	}
	return nil
}

func (r *Runtime) readFinalClusterStopped(path string) (finalClusterStoppedReceipt, bool, error) {
	var value finalClusterStoppedReceipt
	if err := readStrictJSON(path, &value); errors.Is(err, os.ErrNotExist) {
		return value, false, nil
	} else if err != nil {
		return value, false, err
	}
	if err := value.validate(r); err != nil {
		return value, false, err
	}
	return value, true, nil
}

func (r *Runtime) writeFinalClusterStopped(path string, value finalClusterStoppedReceipt) error {
	if err := value.validate(r); err != nil {
		return err
	}
	if err := r.ensurePrivateDirectory(r.finalGatewaySettlementRoot()); err != nil {
		return err
	}
	return writeAtomicJSON(path, value)
}

func (r *Runtime) removeFinalClusterStopped(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectoryIfPresent(r.finalGatewaySettlementRoot())
}

// observeFinalClusterComponentRaw keeps absence, health failure, ownership
// drift, and transport uncertainty distinct. It performs no repair.
func (r *Runtime) observeFinalClusterComponentRaw(ctx context.Context, component, container string) (appliedClusterComponentObservation, bool, error) {
	output, err := r.runner.Output(ctx, []string{"inspect", "--format", appliedClusterInspectTemplate, container}, os.Environ())
	if err != nil {
		if isMissingDockerResource(err, output) || bytes.Contains(output, []byte("No such object")) {
			return appliedClusterComponentObservation{}, true, nil
		}
		return appliedClusterComponentObservation{}, false, fmt.Errorf("observe final %s: %w: %s", component, err, boundedDiagnostic(output))
	}
	var observed appliedClusterComponentObservation
	if err := decodeStrictJSON(output, &observed); err != nil {
		return observed, false, err
	}
	observed.NetworkAddresses = make(map[string]string, len(observed.Networks))
	for network, raw := range observed.Networks {
		var endpoint struct {
			IPAddress string `json:"IPAddress"`
		}
		if err := json.Unmarshal(raw, &endpoint); err != nil {
			return observed, false, err
		}
		observed.NetworkAddresses[network] = endpoint.IPAddress
	}
	sort.Strings(observed.MountDestinations)
	return observed, false, nil
}

// observeFinalResearchClusterPair keeps the supported research status path
// within the same Docker-call budget as the standard build. The exact
// component label joins each observation; Docker output order is never used
// as authority. An ambiguous or partial result degrades both optional live
// observations instead of triggering extra calls outside the frozen budget.
func (r *Runtime) observeFinalResearchClusterPair(ctx context.Context) (appliedClusterComponentObservation, bool, error, appliedClusterComponentObservation, bool, error) {
	output, err := r.runner.Output(ctx, []string{"inspect", "--format", appliedClusterInspectTemplate, opaContainer, authBrokerContainer}, os.Environ())
	if err != nil {
		observationErr := fmt.Errorf("observe final research component pair: %w: %s", err, boundedDiagnostic(output))
		return appliedClusterComponentObservation{}, false, observationErr, appliedClusterComponentObservation{}, false, observationErr
	}
	lines := bytes.Split(bytes.TrimSpace(output), []byte("\n"))
	if len(lines) != 2 {
		observationErr := fmt.Errorf("final research component pair is incomplete")
		return appliedClusterComponentObservation{}, false, observationErr, appliedClusterComponentObservation{}, false, observationErr
	}
	observed := make(map[string]appliedClusterComponentObservation, 2)
	for _, line := range lines {
		var component appliedClusterComponentObservation
		if err := decodeStrictJSON(line, &component); err != nil {
			observationErr := fmt.Errorf("decode final research component pair: %w", err)
			return appliedClusterComponentObservation{}, false, observationErr, appliedClusterComponentObservation{}, false, observationErr
		}
		if component.Component != "opa" && component.Component != "auth-broker" || observed[component.Component].Component != "" {
			observationErr := fmt.Errorf("final research component pair has unknown or duplicate identity")
			return appliedClusterComponentObservation{}, false, observationErr, appliedClusterComponentObservation{}, false, observationErr
		}
		component.NetworkAddresses = make(map[string]string, len(component.Networks))
		for network, raw := range component.Networks {
			var endpoint struct {
				IPAddress string `json:"IPAddress"`
			}
			if err := json.Unmarshal(raw, &endpoint); err != nil {
				observationErr := fmt.Errorf("decode final research component network: %w", err)
				return appliedClusterComponentObservation{}, false, observationErr, appliedClusterComponentObservation{}, false, observationErr
			}
			component.NetworkAddresses[network] = endpoint.IPAddress
		}
		sort.Strings(component.MountDestinations)
		observed[component.Component] = component
	}
	opa, opaPresent := observed["opa"]
	broker, brokerPresent := observed["auth-broker"]
	if !opaPresent || !brokerPresent {
		observationErr := fmt.Errorf("final research component pair omitted selected identity")
		return appliedClusterComponentObservation{}, false, observationErr, appliedClusterComponentObservation{}, false, observationErr
	}
	return opa, false, nil, broker, false, nil
}

func finalComponentStatus(name string, observed appliedClusterComponentObservation, missing bool, err error) tobari.FinalClusterComponentObservation {
	result := tobari.FinalClusterComponentObservation{Name: name, Identity: tobari.FinalClusterEvidenceUnknown, Topology: tobari.FinalClusterEvidenceUnknown}
	if err != nil {
		result.State = tobari.FinalClusterRuntimeUnknown
		return result
	}
	if missing {
		result.State = tobari.FinalClusterRuntimeAbsent
		result.Identity, result.Topology = tobari.FinalClusterEvidenceAbsent, tobari.FinalClusterEvidenceAbsent
		return result
	}
	result.Health = observed.Health
	if observed.Owner != ownerValue || observed.Component != name || name == "gateway" && observed.Role != gatewayRole || !containerIDPattern.MatchString(observed.ContainerID) || !imageIDPattern.MatchString(observed.ImageID) {
		result.State = tobari.FinalClusterRuntimeDrifted
		result.Identity, result.Topology = tobari.FinalClusterEvidenceDrifted, tobari.FinalClusterEvidenceUnknown
	} else if observed.State != "running" {
		result.State = tobari.FinalClusterRuntimeStopped
		result.Identity, result.Topology = tobari.FinalClusterEvidenceExact, tobari.FinalClusterEvidenceExact
	} else if observed.Health != "healthy" {
		result.State = tobari.FinalClusterRuntimeUnhealthy
		result.Identity, result.Topology = tobari.FinalClusterEvidenceExact, tobari.FinalClusterEvidenceExact
	} else {
		result.State = tobari.FinalClusterRuntimeRunning
		result.Identity, result.Topology = tobari.FinalClusterEvidenceExact, tobari.FinalClusterEvidenceExact
	}
	return result
}

type statusClusterImageObservation struct {
	ID          string   `json:"id"`
	RepoTags    []string `json:"repo_tags"`
	RepoDigests []string `json:"repo_digests"`
}

// resolveStatusClusterImageIDs resolves the selected OPA and, on the research
// surface, Auth Broker image identities in one bounded Docker call. Each
// result is joined by its exact selected reference rather than Docker output
// order, so the call-count bound does not weaken component identity evidence.
func (r *Runtime) resolveStatusClusterImageIDs(ctx context.Context, references ...string) (map[string]string, error) {
	result := make(map[string]string, len(references))
	if len(references) == 0 {
		return result, nil
	}
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if reference == "" {
			return nil, fmt.Errorf("status cluster image reference is empty")
		}
		if _, duplicate := seen[reference]; duplicate {
			return nil, fmt.Errorf("status cluster image reference is ambiguous")
		}
		seen[reference] = struct{}{}
	}
	format := `{"id":{{json .Id}},"repo_tags":{{json .RepoTags}},"repo_digests":{{json .RepoDigests}}}`
	args := append([]string{"image", "inspect", "--format", format}, references...)
	stdout, stderr := &boundedBuffer{limit: 32 * 1024}, &boundedBuffer{limit: 4096}
	if err := r.runner.Run(ctx, args, os.Environ(), nil, stdout, stderr); err != nil {
		return nil, fmt.Errorf("resolve status cluster image authority: %w: %s", err, boundedDiagnostic(stderr.buffer.Bytes()))
	}
	if stdout.overflow || stderr.overflow || len(bytes.TrimSpace(stderr.buffer.Bytes())) != 0 {
		return nil, fmt.Errorf("status cluster image authority exceeds its bound or emitted diagnostics")
	}
	lines := bytes.Split(bytes.TrimSpace(stdout.buffer.Bytes()), []byte("\n"))
	if len(lines) != len(references) {
		return nil, fmt.Errorf("status cluster image authority is incomplete")
	}
	for _, line := range lines {
		var observed statusClusterImageObservation
		if err := decodeStrictJSON(line, &observed); err != nil || !imageIDPattern.MatchString(observed.ID) {
			return nil, fmt.Errorf("status cluster image authority is invalid")
		}
		matched := ""
		for _, reference := range references {
			if slices.Contains(observed.RepoTags, reference) || slices.Contains(observed.RepoDigests, reference) {
				if matched != "" {
					return nil, fmt.Errorf("status cluster image authority matches multiple references")
				}
				matched = reference
			}
		}
		if matched == "" || result[matched] != "" {
			return nil, fmt.Errorf("status cluster image authority is unknown or duplicated")
		}
		result[matched] = observed.ID
	}
	for _, reference := range references {
		if result[reference] == "" {
			return nil, fmt.Errorf("status cluster image authority omitted %s", strings.SplitN(reference, "@", 2)[0])
		}
	}
	return result, nil
}

func (r *Runtime) ObserveFinalCluster(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, present bool) (tobari.FinalClusterStatus, error) {
	status := tobari.FinalClusterStatus{SchemaVersion: tobari.FinalClusterStatusSchemaVersion, Task: tobari.TaskClusterStatus, Authority: tobari.FinalClusterAuthorityAbsent, Runtime: tobari.FinalClusterRuntimeUnknown, Receipt: tobari.FinalClusterReceiptAbsent, Contexts: []tobari.FinalClusterContextReceiptObservation{}, Components: []tobari.FinalClusterComponentObservation{}}
	if present {
		if err := collection.Validate(); err != nil {
			return status, err
		}
		status.Authority, status.Generation, status.CollectionRevision = tobari.FinalClusterAuthorityPresent, collection.Generation, collection.Revision
		status.TemplateCount, status.ContextCount, status.WorkspaceCount = len(collection.Templates), len(collection.Contexts), len(collection.Workspaces)
		for _, record := range collection.Contexts {
			item := tobari.FinalClusterContextReceiptObservation{ContextID: record.Context.ID}
			if record.ActiveTemplatePolicy != nil {
				copy := *record.ActiveTemplatePolicy
				item.TemplatePolicy = &copy
			}
			if record.ActivePolicyMemoryRef != nil {
				copy := *record.ActivePolicyMemoryRef
				item.PolicyMemory = &copy
			}
			status.Contexts = append(status.Contexts, item)
		}
	}
	active, activePresent, activeErr := r.readOptionalFinalPolicyActivation(r.finalPolicyActiveReceiptPath())
	stopped, stoppedPresent, stoppedErr := r.readFinalClusterStopped(r.finalClusterStoppedReceiptPath())
	_, bootstrapPresent, bootstrapErr := r.readFinalClusterBootstrap()
	_, gatewayJournalPresent, gatewayJournalErr := r.readFinalGatewaySettlementJournal()
	_, downJournalPresent, downJournalErr := r.readFinalClusterStopped(r.finalClusterDownJournalPath())
	switch {
	case activeErr != nil || stoppedErr != nil:
		status.Receipt = tobari.FinalClusterReceiptUnknown
	case activePresent && stoppedPresent:
		status.Receipt = tobari.FinalClusterReceiptDrifted
	case activePresent:
		status.Receipt = tobari.FinalClusterReceiptActive
	case stoppedPresent:
		status.Receipt = tobari.FinalClusterReceiptStopped
	default:
		status.Receipt = tobari.FinalClusterReceiptAbsent
	}
	journalCount := 0
	for _, value := range []bool{bootstrapPresent, gatewayJournalPresent, downJournalPresent} {
		if value {
			journalCount++
		}
	}
	journalUnknown := bootstrapErr != nil || gatewayJournalErr != nil || downJournalErr != nil
	if journalUnknown {
		status.Receipt = tobari.FinalClusterReceiptUnknown
	}
	gateway, gatewayMissing, gatewayErr := r.observeFinalClusterComponentRaw(ctx, "gateway", gatewayContainer)
	var opa, broker appliedClusterComponentObservation
	var opaMissing, brokerMissing bool
	var opaErr, brokerErr error
	if brokerRuntimeEnabled && activePresent {
		opa, opaMissing, opaErr, broker, brokerMissing, brokerErr = r.observeFinalResearchClusterPair(ctx)
	} else {
		opa, opaMissing, opaErr = r.observeFinalClusterComponentRaw(ctx, "opa", opaContainer)
		broker, brokerMissing, brokerErr = r.observeFinalClusterComponentRaw(ctx, "auth-broker", authBrokerContainer)
	}
	status.Components = append(status.Components, finalComponentStatus("gateway", gateway, gatewayMissing, gatewayErr), finalComponentStatus("opa", opa, opaMissing, opaErr))
	brokerStatus := finalComponentStatus("auth-broker", broker, brokerMissing, brokerErr)
	companionStatus := tobari.FinalClusterComponentObservation{Name: "credential-companion", State: tobari.FinalClusterRuntimeAbsent, Identity: tobari.FinalClusterEvidenceAbsent, Topology: tobari.FinalClusterEvidenceAbsent}
	if brokerRuntimeEnabled {
		if brokerErr != nil {
			companionStatus.State, companionStatus.Identity, companionStatus.Topology = tobari.FinalClusterRuntimeUnknown, tobari.FinalClusterEvidenceUnknown, tobari.FinalClusterEvidenceUnknown
		} else if brokerMissing {
			stopped, err := companionruntime.ObserveStopped(r.stateDirectory)
			if err != nil {
				companionStatus.State, companionStatus.Identity, companionStatus.Topology = tobari.FinalClusterRuntimeUnknown, tobari.FinalClusterEvidenceUnknown, tobari.FinalClusterEvidenceUnknown
			} else if !stopped {
				companionStatus.State = tobari.FinalClusterRuntimeDrifted
				companionStatus.Identity, companionStatus.Topology = tobari.FinalClusterEvidenceExact, tobari.FinalClusterEvidenceDrifted
			} else if stoppedPresent {
				companionStatus.State, companionStatus.Health = tobari.FinalClusterRuntimeStopped, "stopped"
				companionStatus.Identity, companionStatus.Topology = tobari.FinalClusterEvidenceExact, tobari.FinalClusterEvidenceExact
			}
		} else {
			state, _, err := r.credentialCompanionStatus(ctx)
			companionStatus.Health = state
			companionStatus.Identity, companionStatus.Topology = tobari.FinalClusterEvidenceExact, tobari.FinalClusterEvidenceExact
			switch {
			case err != nil:
				companionStatus.State, companionStatus.Identity, companionStatus.Topology = tobari.FinalClusterRuntimeUnknown, tobari.FinalClusterEvidenceUnknown, tobari.FinalClusterEvidenceUnknown
			case state == "ready":
				companionStatus.State = tobari.FinalClusterRuntimeRunning
			case state == "stopped" || state == "absent":
				companionStatus.State = tobari.FinalClusterRuntimeStopped
			default:
				companionStatus.State = tobari.FinalClusterRuntimeUnhealthy
			}
		}
	} else if brokerErr != nil {
		brokerStatus.State = tobari.FinalClusterRuntimeUnknown
	} else if !brokerMissing {
		brokerStatus.State = tobari.FinalClusterRuntimeDrifted
		brokerStatus.Identity = tobari.FinalClusterEvidenceDrifted
	}
	if brokerRuntimeEnabled {
		status.Components = append(status.Components, brokerStatus, companionStatus)
	}
	if gatewayErr != nil || opaErr != nil {
		status.Runtime = tobari.FinalClusterRuntimeUnknown
	} else if gatewayMissing && opaMissing {
		if status.Receipt == tobari.FinalClusterReceiptStopped {
			status.Runtime = tobari.FinalClusterRuntimeStopped
		} else {
			status.Runtime = tobari.FinalClusterRuntimeAbsent
		}
	} else if gatewayMissing != opaMissing {
		status.Runtime = tobari.FinalClusterRuntimeDrifted
	} else if status.Components[0].State == tobari.FinalClusterRuntimeDrifted || status.Components[1].State == tobari.FinalClusterRuntimeDrifted {
		status.Runtime = tobari.FinalClusterRuntimeDrifted
	} else if status.Components[0].State == tobari.FinalClusterRuntimeUnhealthy || status.Components[1].State == tobari.FinalClusterRuntimeUnhealthy {
		status.Runtime = tobari.FinalClusterRuntimeUnhealthy
	} else if status.Components[0].State == tobari.FinalClusterRuntimeStopped || status.Components[1].State == tobari.FinalClusterRuntimeStopped {
		status.Runtime = tobari.FinalClusterRuntimeStopped
	} else {
		status.Runtime = tobari.FinalClusterRuntimeRunning
	}
	if brokerRuntimeEnabled {
		if brokerStatus.State == tobari.FinalClusterRuntimeUnknown || companionStatus.State == tobari.FinalClusterRuntimeUnknown {
			status.Runtime = tobari.FinalClusterRuntimeUnknown
		} else if brokerStatus.State == tobari.FinalClusterRuntimeAbsent &&
			(companionStatus.State == tobari.FinalClusterRuntimeAbsent || companionStatus.State == tobari.FinalClusterRuntimeStopped) &&
			(status.Runtime == tobari.FinalClusterRuntimeAbsent || status.Runtime == tobari.FinalClusterRuntimeStopped) {
			// Exact host-side companion absence completes a stopped/absent research closure without Broker control.
		} else if brokerStatus.State == tobari.FinalClusterRuntimeDrifted {
			status.Runtime = tobari.FinalClusterRuntimeDrifted
		} else if brokerStatus.State != tobari.FinalClusterRuntimeRunning || companionStatus.State != tobari.FinalClusterRuntimeRunning {
			status.Runtime = tobari.FinalClusterRuntimeUnhealthy
		}
	} else if brokerStatus.State == tobari.FinalClusterRuntimeUnknown {
		status.Runtime = tobari.FinalClusterRuntimeUnknown
	} else if brokerStatus.State != tobari.FinalClusterRuntimeAbsent {
		status.Runtime = tobari.FinalClusterRuntimeDrifted
	}
	if activePresent && !gatewayMissing && !opaMissing && (active.Material.Gateway.ContainerID != gateway.ContainerID || active.Material.Gateway.ImageID != gateway.ImageID || !reflect.DeepEqual(active.Material.Gateway.Networks, componentNetworkRows(gateway))) {
		status.Runtime = tobari.FinalClusterRuntimeDrifted
	}
	selectedImages := make([]string, 0, 2)
	selectedOPA, selectedBroker := "", ""
	if !opaMissing && opaErr == nil {
		versions, versionsErr := runtimeassets.Versions()
		if versionsErr != nil {
			status.Runtime = tobari.FinalClusterRuntimeUnknown
		} else {
			selectedOPA = versions["OPA_IMAGE"]
			selectedImages = append(selectedImages, selectedOPA)
		}
	}
	if brokerRuntimeEnabled && !brokerMissing && brokerErr == nil {
		selection, selectErr := r.selectAuthBrokerImage(ctx)
		if selectErr != nil {
			status.Runtime = tobari.FinalClusterRuntimeUnknown
		} else {
			selectedBroker = selection.Image
			selectedImages = append(selectedImages, selectedBroker)
		}
	}
	if len(selectedImages) != 0 {
		resolved, identityErr := r.resolveStatusClusterImageIDs(ctx, selectedImages...)
		if identityErr != nil {
			status.Runtime = tobari.FinalClusterRuntimeUnknown
		} else if (selectedOPA != "" && opa.ImageID != resolved[selectedOPA]) || (selectedBroker != "" && broker.ImageID != resolved[selectedBroker]) {
			status.Runtime = tobari.FinalClusterRuntimeDrifted
		}
	}
	if activePresent && present {
		projection, projectionErr := tobari.BuildActiveWorkspacePolicyProjection(collection)
		if projectionErr != nil || projection.ContentDigest != active.Material.Plan.ContentDigest {
			status.Runtime = tobari.FinalClusterRuntimeDrifted
			status.Receipt = tobari.FinalClusterReceiptDrifted
		}
	} else if activePresent {
		status.Runtime = tobari.FinalClusterRuntimeDrifted
		status.Receipt = tobari.FinalClusterReceiptDrifted
	}
	if status.Runtime == tobari.FinalClusterRuntimeRunning && status.Receipt != tobari.FinalClusterReceiptActive ||
		status.Runtime == tobari.FinalClusterRuntimeAbsent && status.Receipt == tobari.FinalClusterReceiptActive {
		status.Runtime = tobari.FinalClusterRuntimeDrifted
	}
	if stoppedPresent && !gatewayMissing && gateway.ContainerID != stopped.Gateway.ContainerID {
		status.Runtime = tobari.FinalClusterRuntimeDrifted
	}
	if journalUnknown {
		status.Runtime = tobari.FinalClusterRuntimeUnknown
	} else if journalCount != 0 {
		status.Runtime = tobari.FinalClusterRuntimeDrifted
		status.Receipt = tobari.FinalClusterReceiptUnknown
	}
	activeAfter, activePresentAfter, activeErrAfter := r.readOptionalFinalPolicyActivation(r.finalPolicyActiveReceiptPath())
	if (activeErr == nil) != (activeErrAfter == nil) || activePresent != activePresentAfter ||
		activeErr == nil && activeErrAfter == nil && activePresent && !reflect.DeepEqual(active, activeAfter) {
		return tobari.FinalClusterStatus{}, tobari.ErrFinalClusterObservationChanged
	}
	if status.Authority == tobari.FinalClusterAuthorityPresent && status.Receipt == tobari.FinalClusterReceiptActive {
		if !activePresent || activeErr != nil {
			return tobari.FinalClusterStatus{}, fmt.Errorf("active final cluster status has no verified aggregate receipt")
		}
		identity := tobari.PolicyProjectionIdentity{
			AggregateRevision:  active.Aggregate.AggregateRevision,
			EvaluatorIdentity:  active.Aggregate.EvaluatorIdentity,
			PolicyDataIdentity: active.Aggregate.PolicyDataIdentity,
		}
		if err := identity.Validate(); err != nil {
			return tobari.FinalClusterStatus{}, fmt.Errorf("active final cluster aggregate identity is invalid: %w", err)
		}
		aggregateRevision := identity.AggregateRevision
		evaluatorIdentity := identity.EvaluatorIdentity
		policyDataIdentity := identity.PolicyDataIdentity
		status.AggregateRevision = &aggregateRevision
		status.EvaluatorIdentity = &evaluatorIdentity
		status.PolicyDataIdentity = &policyDataIdentity
	}
	if err := status.Validate(); err != nil {
		return tobari.FinalClusterStatus{}, err
	}
	return status, nil
}

func (r *Runtime) SettleFinalClusterDown(ctx context.Context, previous, next tobari.WorkspaceAuthorityCollection, operation, decisionRef string) error {
	transition, err := tobari.PlanWorkspaceAuthorityClusterDown(previous)
	if err != nil || transition.Plan.ValidateTransition(previous, next) != nil || operation == "" || decisionRef == "" {
		return fmt.Errorf("final cluster down request is invalid: %w", err)
	}
	if err := r.requireNoFinalGatewaySettlement(ctx); err != nil {
		return err
	}
	if _, present, err := r.readFinalClusterBootstrap(); err != nil {
		return err
	} else if present {
		return fmt.Errorf("final cluster bootstrap is interrupted; resume cluster up")
	}
	journal, present, err := r.readFinalClusterStopped(r.finalClusterDownJournalPath())
	if err != nil {
		return err
	}
	if present {
		if journal.Operation != operation || journal.DecisionRef != decisionRef || journal.PreviousRevision != previous.Revision || journal.NextRevision != next.Revision {
			return fmt.Errorf("another final cluster down requires exact same-action recovery")
		}
		return r.resumeFinalClusterDown(ctx, journal)
	}
	if stopped, stoppedPresent, stoppedErr := r.readFinalClusterStopped(r.finalClusterStoppedReceiptPath()); stoppedErr != nil {
		return stoppedErr
	} else if stoppedPresent {
		if stopped.Operation != operation || stopped.DecisionRef != decisionRef || stopped.PreviousRevision != previous.Revision || stopped.NextRevision != next.Revision {
			return fmt.Errorf("completed final cluster down belongs to another durable decision")
		}
		if _, activePresent, activeErr := r.readOptionalFinalPolicyActivation(r.finalPolicyActiveReceiptPath()); activeErr != nil || activePresent {
			return fmt.Errorf("completed final cluster down retained active authority: %w", activeErr)
		}
		components := []struct{ component, name string }{{"gateway", gatewayContainer}, {"opa", opaContainer}}
		if brokerRuntimeEnabled {
			components = append(components, struct{ component, name string }{"auth-broker", authBrokerContainer})
		}
		for _, item := range components {
			if _, missing, observeErr := r.observeFinalClusterComponentRaw(ctx, item.component, item.name); observeErr != nil || !missing {
				return fmt.Errorf("completed final cluster down has live or unknown %s: %w", item.component, observeErr)
			}
		}
		return nil
	}
	if err := r.ConfirmNoFinalWorkspaceSessions(ctx); err != nil {
		return err
	}
	active, err := r.readFinalPolicyActivation(r.finalPolicyActiveReceiptPath())
	if err != nil {
		return fmt.Errorf("read exact final active authority before down: %w", err)
	}
	gateway, missing, err := r.observeFinalClusterComponentRaw(ctx, "gateway", gatewayContainer)
	if err != nil || missing {
		return fmt.Errorf("observe exact final Gateway before down: %w", err)
	}
	opa, missing, err := r.observeFinalClusterComponentRaw(ctx, "opa", opaContainer)
	if err != nil || missing {
		return fmt.Errorf("observe exact final OPA before down: %w", err)
	}
	if gateway.State != "running" || gateway.Health != "healthy" || opa.State != "running" || opa.Health != "healthy" || active.Material.Gateway.ContainerID != gateway.ContainerID || active.Material.Gateway.ImageID != gateway.ImageID {
		return fmt.Errorf("final active component authority is unhealthy or drifted")
	}
	versions, err := runtimeassets.Versions()
	if err != nil {
		return err
	}
	expectedOPA, err := r.resolveCandidateImageID(ctx, versions["OPA_IMAGE"])
	if err != nil || opa.ImageID != expectedOPA {
		return fmt.Errorf("final OPA differs from selected image authority: %w", err)
	}
	journal = finalClusterStoppedReceipt{SchemaVersion: finalClusterStoppedSchema, Operation: operation, DecisionRef: decisionRef, Phase: finalClusterDownPrepared, PreviousGeneration: previous.Generation, PreviousRevision: previous.Revision, NextGeneration: next.Generation, NextRevision: next.Revision, Active: active, Gateway: gateway, OPA: opa}
	if brokerRuntimeEnabled {
		broker, brokerMissing, brokerErr := r.observeFinalClusterComponentRaw(ctx, "auth-broker", authBrokerContainer)
		companion, _, companionErr := r.credentialCompanionStatus(ctx)
		selection, selectErr := r.selectAuthBrokerImage(ctx)
		if selectErr != nil {
			return fmt.Errorf("select final Auth Broker authority: %w", selectErr)
		}
		expectedBroker, identityErr := r.resolveCandidateImageID(ctx, selection.Image)
		if identityErr != nil || brokerErr != nil || brokerMissing || broker.State != "running" || broker.Health != "healthy" || broker.ImageID != expectedBroker || companionErr != nil || companion != "ready" {
			return fmt.Errorf("final research cluster closure is not exactly ready")
		}
		journal.AuthBroker, journal.CompanionState = &broker, companion
	} else if _, brokerMissing, brokerErr := r.observeFinalClusterComponentRaw(ctx, "auth-broker", authBrokerContainer); brokerErr != nil || !brokerMissing {
		return fmt.Errorf("release final cluster contains unexpected Auth Broker authority")
	}
	if err := r.writeFinalClusterStopped(r.finalClusterDownJournalPath(), journal); err != nil {
		return err
	}
	return r.resumeFinalClusterDown(ctx, journal)
}

func (r *Runtime) removeExactFinalClusterContainer(ctx context.Context, name string, expected appliedClusterComponentObservation) error {
	current, missing, err := r.observeFinalClusterComponentRaw(ctx, expected.Component, name)
	if err != nil {
		return err
	}
	if missing {
		return nil
	}
	if current.ContainerID != expected.ContainerID || current.Owner != ownerValue || current.Component != expected.Component || current.ImageID != expected.ImageID {
		return fmt.Errorf("final cluster component ownership or identity changed before removal")
	}
	output, err := r.runner.Output(ctx, []string{"container", "rm", "-f", expected.ContainerID}, os.Environ())
	if err != nil {
		return fmt.Errorf("remove exact final %s: %w: %s", expected.Component, err, boundedDiagnostic(output))
	}
	return nil
}

func (r *Runtime) resumeFinalClusterDown(ctx context.Context, journal finalClusterStoppedReceipt) error {
	if err := journal.validate(r); err != nil {
		return err
	}
	if journal.Phase == finalClusterDownPrepared {
		if err := r.ConfirmNoFinalWorkspaceSessions(ctx); err != nil {
			return err
		}
		if err := r.removeExactFinalClusterContainer(ctx, gatewayContainer, journal.Gateway); err != nil {
			return err
		}
		if err := r.removeExactFinalClusterContainer(ctx, opaContainer, journal.OPA); err != nil {
			return err
		}
		if journal.AuthBroker != nil {
			if err := r.removeExactFinalClusterContainer(ctx, authBrokerContainer, *journal.AuthBroker); err != nil {
				return err
			}
			if err := r.waitForCredentialCompanionStopped(ctx); err != nil {
				return err
			}
		}
		for _, network := range []string{"tobari-control", "tobari-egress"} {
			if err := r.verifyOwned(ctx, "network", network); errors.Is(err, errOwnedResourceMissing) {
				continue
			} else if err != nil {
				return err
			}
			if output, err := r.runner.Output(ctx, []string{"network", "rm", network}, os.Environ()); err != nil {
				return fmt.Errorf("remove exact final shared network %s: %w: %s", network, err, boundedDiagnostic(output))
			}
		}
		journal.Phase = finalClusterDownRuntime
		if err := r.writeFinalClusterStopped(r.finalClusterDownJournalPath(), journal); err != nil {
			return err
		}
	}
	if journal.Phase == finalClusterDownRuntime {
		if err := r.replaceProjectPrincipalRegistry(ctx, []projectPrincipalBinding{}); err != nil {
			return err
		}
		journal.Phase = finalClusterDownAuthority
		if err := r.writeFinalClusterStopped(r.finalClusterDownJournalPath(), journal); err != nil {
			return err
		}
	}
	if err := r.writeFinalClusterStopped(r.finalClusterStoppedReceiptPath(), journal); err != nil {
		return err
	}
	for _, path := range []string{r.finalPolicyActivationJournalPath(), r.finalPolicyActiveReceiptPath()} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return r.removeFinalClusterStopped(r.finalClusterDownJournalPath())
}

func (r *Runtime) ConfirmFinalClusterDownSettled(ctx context.Context, current tobari.WorkspaceAuthorityCollection) error {
	if len(current.Workspaces) != 0 {
		return fmt.Errorf("final cluster down retained Workspaces")
	}
	for _, record := range current.Contexts {
		if record.ActiveTemplatePolicy != nil || record.ActivePolicyMemory != nil || record.ActivePolicyMemoryRef != nil {
			return fmt.Errorf("final cluster down retained active receipts")
		}
	}
	if _, present, err := r.readOptionalFinalPolicyActivation(r.finalPolicyActiveReceiptPath()); err != nil || present {
		return fmt.Errorf("final active receipt remains after down: %w", err)
	}
	if _, present, err := r.readFinalClusterStopped(r.finalClusterStoppedReceiptPath()); err != nil || !present {
		return fmt.Errorf("final stopped receipt is unavailable: %w", err)
	}
	components := []struct{ component, name string }{{"gateway", gatewayContainer}, {"opa", opaContainer}}
	if brokerRuntimeEnabled {
		components = append(components, struct{ component, name string }{"auth-broker", authBrokerContainer})
	}
	for _, item := range components {
		if _, missing, err := r.observeFinalClusterComponentRaw(ctx, item.component, item.name); err != nil || !missing {
			return fmt.Errorf("final %s is not absent after down: %w", item.component, err)
		}
	}
	return nil
}
