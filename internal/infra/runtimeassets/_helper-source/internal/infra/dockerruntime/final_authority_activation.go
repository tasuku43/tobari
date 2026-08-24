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
	"os"
	"path/filepath"
	"reflect"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const finalPolicyActivationSchema = 1

type finalPolicyActivationRecord struct {
	SchemaVersion     int                              `json:"schema_version"`
	ReviewedSetDigest tobari.SemanticDigest            `json:"reviewed_set_digest,omitempty"`
	Material          FinalWorkspacePolicyProjection   `json:"material"`
	Aggregate         FinalAggregateProjection         `json:"aggregate"`
	Receipt           FinalAggregatePublicationReceipt `json:"receipt"`
}

func (r finalPolicyActivationRecord) validate(runtime *Runtime) error {
	if r.SchemaVersion != finalPolicyActivationSchema || runtime == nil {
		return fmt.Errorf("final policy activation record metadata is invalid")
	}
	if err := r.Material.Validate(); err != nil {
		return fmt.Errorf("final policy activation material is inconsistent: %w", err)
	}
	if r.Material.Plan.Mode == tobari.WorkspacePolicyProjectionReviewed {
		if r.ReviewedSetDigest.Validate() != nil {
			return fmt.Errorf("final policy activation reviewed-set identity is invalid")
		}
	} else if r.ReviewedSetDigest != "" {
		return fmt.Errorf("final policy activation carries reviewed-set identity outside reviewed mode")
	}
	if r.Aggregate.MaterializedDigest != r.Material.MaterializedDigest || !aggregateRevisionPattern.MatchString(r.Aggregate.AggregateRevision) {
		return fmt.Errorf("final policy activation aggregate is inconsistent")
	}
	for _, path := range []string{r.Aggregate.PolicyDirectory, r.Aggregate.GatewayConfig} {
		relative, err := filepath.Rel(runtime.aggregateRoot(), path)
		if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || len(relative) >= 3 && relative[:3] == "../" {
			return fmt.Errorf("final policy activation artifact crosses the owned aggregate root")
		}
	}
	if err := r.Receipt.ValidateFor(r.Material); err != nil {
		return err
	}
	return runtime.ConfirmFinalAggregatePublicationReceipt(r.Material, r.Aggregate, r.Receipt)
}

func (r *Runtime) finalPolicyActivationRoot() string {
	return filepath.Join(r.stateDirectory, "workspace-authority-policy")
}

func (r *Runtime) finalPolicyActivationJournalPath() string {
	return filepath.Join(r.finalPolicyActivationRoot(), "activation.json")
}

func (r *Runtime) finalPolicyActiveReceiptPath() string {
	return filepath.Join(r.finalPolicyActivationRoot(), "active.json")
}

// ActivatePolicyMemory satisfies the dormant final-authority mutation port.
// The caller already owns the installation lifecycle lock; this method adds
// only the existing lifecycle->policy projection lock and never consults the
// predecessor Manifest store or shared State.
func (r *Runtime) ActivatePolicyMemory(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID) (tobari.PolicyMemoryActivationReceipt, error) {
	if err := r.requireNoFinalGatewaySettlement(ctx); err != nil {
		return tobari.PolicyMemoryActivationReceipt{}, err
	}
	plan, err := tobari.BuildHotWorkspacePolicyProjection(collection, contextID)
	if err != nil {
		return tobari.PolicyMemoryActivationReceipt{}, err
	}
	var result tobari.PolicyMemoryActivationReceipt
	err = r.withPolicyProjectionLock(ctx, func() error {
		record, err := r.prepareFinalPolicyActivation(ctx, plan)
		if err != nil {
			return err
		}
		if err := r.resumeFinalPolicyActivation(ctx, record); err != nil {
			return err
		}
		for _, item := range record.Material.Plan.Contexts {
			if item.ContextID == contextID {
				result = item.MemoryReceipt
				return nil
			}
		}
		return fmt.Errorf("final policy activation omitted its exact target Context")
	})
	return result, err
}

// ConfirmPolicyMemoryActive proves the target receipt only through the exact
// complete live aggregate receipt. A per-Context receipt cannot confirm a
// different OPA or principal projection.
func (r *Runtime) ConfirmPolicyMemoryActive(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID, expected tobari.PolicyMemoryActivationReceipt) error {
	plan, err := tobari.BuildHotWorkspacePolicyProjection(collection, contextID)
	if err != nil {
		return err
	}
	return r.confirmFinalPolicyActivation(ctx, plan, func(item tobari.WorkspacePolicyProjectionContext) bool {
		return item.ContextID == contextID && item.MemoryReceipt == expected
	})
}

// ConfirmTemplatePolicyActive proves the independently selected Template
// policy axis through the same complete live aggregate receipt.
func (r *Runtime) ConfirmTemplatePolicyActive(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID, expected tobari.TemplatePolicyActivationReceipt) error {
	plan, err := buildCurrentActiveWorkspacePolicyProjection(collection, contextID)
	if err != nil {
		return err
	}
	return r.confirmFinalPolicyActivation(ctx, plan, func(item tobari.WorkspacePolicyProjectionContext) bool {
		return item.ContextID == contextID && item.TemplateReceipt == expected
	})
}

// PrepareFinalClusterPolicyReconciliation constructs the exact dormant
// cluster candidate. It deliberately performs no policy, principal-registry,
// Gateway, or reader-selection mutation: the current cluster reconciler must
// later consume this candidate in the same decision that publishes final
// principals and switches the Gateway mount.
func (r *Runtime) PrepareFinalClusterPolicyReconciliation(ctx context.Context, collection tobari.WorkspaceAuthorityCollection) (FinalWorkspacePolicyProjection, FinalAggregateProjection, FinalAggregatePublicationReceipt, error) {
	plan, err := tobari.BuildClusterWorkspacePolicyProjection(collection)
	if err != nil {
		return FinalWorkspacePolicyProjection{}, FinalAggregateProjection{}, FinalAggregatePublicationReceipt{}, err
	}
	material, err := r.ObserveFinalWorkspacePolicyProjection(ctx, plan)
	if err != nil {
		return FinalWorkspacePolicyProjection{}, FinalAggregateProjection{}, FinalAggregatePublicationReceipt{}, err
	}
	aggregate, err := r.BuildFinalAggregateProjection(ctx, material)
	if err != nil {
		return FinalWorkspacePolicyProjection{}, FinalAggregateProjection{}, FinalAggregatePublicationReceipt{}, err
	}
	receipt, err := r.NewFinalAggregatePublicationReceipt(material, aggregate)
	return material, aggregate, receipt, err
}

func (r *Runtime) prepareFinalPolicyActivation(
	ctx context.Context,
	plan tobari.WorkspacePolicyProjection,
	reviewedSetDigest ...tobari.SemanticDigest,
) (finalPolicyActivationRecord, error) {
	var setDigest tobari.SemanticDigest
	if len(reviewedSetDigest) > 1 {
		return finalPolicyActivationRecord{}, fmt.Errorf("final policy activation has ambiguous reviewed-set identity")
	}
	if len(reviewedSetDigest) == 1 {
		setDigest = reviewedSetDigest[0]
	}
	if plan.Mode == tobari.WorkspacePolicyProjectionReviewed {
		if setDigest.Validate() != nil {
			return finalPolicyActivationRecord{}, fmt.Errorf("final policy activation requires exact reviewed-set identity")
		}
	} else if setDigest != "" {
		return finalPolicyActivationRecord{}, fmt.Errorf("final policy activation rejects reviewed-set identity outside reviewed mode")
	}
	if err := r.ensurePrivateDirectory(r.finalPolicyActivationRoot()); err != nil {
		return finalPolicyActivationRecord{}, err
	}
	parent := filepath.Dir(r.finalPolicyActivationRoot())
	if err := requirePrivateDirectory(parent); err != nil {
		return finalPolicyActivationRecord{}, fmt.Errorf("validate final policy activation parent: %w", err)
	}
	syncRoot := syncDirectory
	if r.finalPolicyRootSync != nil {
		syncRoot = r.finalPolicyRootSync
	}
	if err := syncRoot(parent); err != nil {
		return finalPolicyActivationRecord{}, fmt.Errorf("publish final policy activation root durably: %w", err)
	}
	var existing finalPolicyActivationRecord
	if recovered, err := r.readFinalPolicyActivation(r.finalPolicyActivationJournalPath()); err == nil {
		existing = recovered
		if !reflect.DeepEqual(existing.Material.Plan, plan) || existing.ReviewedSetDigest != setDigest {
			return finalPolicyActivationRecord{}, fmt.Errorf("another final policy activation requires exact recovery")
		}
		return existing, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return finalPolicyActivationRecord{}, fmt.Errorf("read final policy activation recovery: %w", err)
	}
	material, err := r.ObserveFinalWorkspacePolicyProjection(ctx, plan)
	if err != nil {
		return finalPolicyActivationRecord{}, err
	}
	aggregate, err := r.BuildFinalAggregateProjection(ctx, material)
	if err != nil {
		return finalPolicyActivationRecord{}, err
	}
	receipt, err := r.NewFinalAggregatePublicationReceipt(material, aggregate)
	if err != nil {
		return finalPolicyActivationRecord{}, err
	}
	record := finalPolicyActivationRecord{
		SchemaVersion: finalPolicyActivationSchema, ReviewedSetDigest: setDigest,
		Material: material, Aggregate: aggregate, Receipt: receipt,
	}
	if err := record.validate(r); err != nil {
		return finalPolicyActivationRecord{}, err
	}
	if active, activeErr := r.readFinalPolicyActivation(r.finalPolicyActiveReceiptPath()); activeErr == nil && reflect.DeepEqual(active.Receipt, record.Receipt) {
		if err := r.confirmFinalPolicyRecord(ctx, record); err == nil {
			return record, nil
		}
	} else if activeErr != nil && !errors.Is(activeErr, os.ErrNotExist) {
		return finalPolicyActivationRecord{}, activeErr
	}
	if err := r.writeFinalPolicyActivation(r.finalPolicyActivationJournalPath(), record); err != nil {
		return finalPolicyActivationRecord{}, err
	}
	return record, nil
}

func (r *Runtime) resumeFinalPolicyActivation(ctx context.Context, record finalPolicyActivationRecord) error {
	if err := record.validate(r); err != nil {
		return err
	}
	if active, err := r.readFinalPolicyActivation(r.finalPolicyActiveReceiptPath()); err == nil && reflect.DeepEqual(active.Receipt, record.Receipt) {
		if err := r.confirmFinalPolicyRecord(ctx, record); err != nil {
			return err
		}
		return r.removeFinalPolicyActivationJournal()
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if r.finalProjectionBeforeEffect != nil {
		r.finalProjectionBeforeEffect()
	}
	observed, err := r.ObserveFinalWorkspacePolicyProjection(ctx, record.Material.Plan)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(observed, record.Material) {
		return fmt.Errorf("final principal authority changed after aggregate artifact review")
	}
	if err := r.confirmExactFinalPrincipalRegistry(record.Material); err != nil {
		return err
	}
	if err := r.confirmLiveFinalGatewayArtifact(ctx, record.Material, record.Aggregate, record.Receipt); err != nil {
		return err
	}
	if err := r.ConfirmFinalAggregatePublicationReceipt(record.Material, record.Aggregate, record.Receipt); err != nil {
		return err
	}
	if err := r.ApplyFinalAggregatePolicy(ctx, record.Aggregate); err != nil {
		return err
	}
	if r.finalPolicyAfterApply != nil {
		if err := r.finalPolicyAfterApply(); err != nil {
			return err
		}
	}
	if err := r.writeFinalPolicyActivation(r.finalPolicyActiveReceiptPath(), record); err != nil {
		return err
	}
	if err := r.confirmFinalPolicyRecord(ctx, record); err != nil {
		return err
	}
	return r.removeFinalPolicyActivationJournal()
}

func (r *Runtime) confirmFinalPolicyActivation(ctx context.Context, plan tobari.WorkspacePolicyProjection, match func(tobari.WorkspacePolicyProjectionContext) bool) error {
	matched := false
	for _, item := range plan.Contexts {
		matched = matched || match(item)
	}
	if !matched {
		return fmt.Errorf("final active policy receipt does not match the requested Context axis")
	}
	return r.confirmFinalPolicyProjection(ctx, plan)
}

// confirmFinalPolicyProjection verifies one complete route-independent live
// publication. Unlike the per-Context confirmation wrapper, it also supports
// an empty Context set after exact Context deletion.
func (r *Runtime) confirmFinalPolicyProjection(ctx context.Context, plan tobari.WorkspacePolicyProjection) error {
	return r.withPolicyProjectionLock(ctx, func() error {
		active, err := r.readFinalPolicyActivation(r.finalPolicyActiveReceiptPath())
		if err != nil {
			return fmt.Errorf("read final active policy receipt: %w", err)
		}
		current, err := r.ObserveFinalWorkspacePolicyProjection(ctx, plan)
		if err != nil {
			return err
		}
		if current.MaterializedDigest != active.Receipt.MaterializedDigest || !reflect.DeepEqual(current.Principals, active.Material.Principals) {
			return fmt.Errorf("final active content does not match complete current authority")
		}
		if err := active.Receipt.ValidateFor(current); err != nil {
			return err
		}
		aggregate, err := r.BuildFinalAggregateProjection(ctx, current)
		if err != nil {
			return err
		}
		if aggregate.AggregateRevision != active.Aggregate.AggregateRevision {
			return fmt.Errorf("final active aggregate does not match complete current authority")
		}
		if err := r.ConfirmFinalAggregatePublicationReceipt(current, aggregate, active.Receipt); err != nil {
			return err
		}
		if err := r.confirmExactFinalPrincipalRegistry(current); err != nil {
			return err
		}
		if err := r.confirmLiveFinalGatewayArtifact(ctx, current, aggregate, active.Receipt); err != nil {
			return err
		}
		return r.waitForPolicyRevision(ctx, aggregate.AggregateRevision)
	})
}

func (r *Runtime) confirmFinalPolicyRecord(ctx context.Context, record finalPolicyActivationRecord) error {
	if err := record.validate(r); err != nil {
		return err
	}
	observed, err := r.ObserveFinalWorkspacePolicyProjection(ctx, record.Material.Plan)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(observed, record.Material) {
		return fmt.Errorf("final active principal projection changed")
	}
	if err := r.confirmExactFinalPrincipalRegistry(record.Material); err != nil {
		return err
	}
	if err := r.confirmLiveFinalGatewayArtifact(ctx, record.Material, record.Aggregate, record.Receipt); err != nil {
		return err
	}
	return r.waitForPolicyRevision(ctx, record.Aggregate.AggregateRevision)
}

const finalGatewayMountObservationLimit = 64 * 1024

type finalGatewayMountObservation struct {
	ContainerID string                     `json:"container_id"`
	Owner       string                     `json:"owner"`
	Component   string                     `json:"component"`
	Role        string                     `json:"role"`
	ImageID     string                     `json:"image_id"`
	State       string                     `json:"state"`
	Health      string                     `json:"health"`
	Networks    map[string]json.RawMessage `json:"networks"`
	Mounts      []struct {
		Type        string `json:"type"`
		Source      string `json:"source"`
		Destination string `json:"destination"`
	} `json:"mounts"`
}

func (r *Runtime) confirmLiveFinalGatewayArtifact(ctx context.Context, material FinalWorkspacePolicyProjection, aggregate FinalAggregateProjection, receipt FinalAggregatePublicationReceipt) error {
	stdout := &boundedBuffer{limit: finalGatewayMountObservationLimit / 2}
	stderr := &boundedBuffer{limit: finalGatewayMountObservationLimit / 2}
	format := `{"container_id":{{json .Id}},"owner":{{json (index .Config.Labels "io.tobari.owner")}},"component":{{json (index .Config.Labels "io.tobari.component")}},"role":{{json (index .Config.Labels "io.tobari.gateway-role")}},"image_id":{{json .Image}},"state":{{json .State.Status}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}"none"{{end}},"networks":{{json .NetworkSettings.Networks}},"mounts":[{{range $index,$mount := .Mounts}}{{if $index}},{{end}}{"type":{{json $mount.Type}},"source":{{json $mount.Source}},"destination":{{json $mount.Destination}}}{{end}}]}`
	err := r.runner.Run(ctx, []string{"container", "inspect", "--format", format, gatewayContainer}, os.Environ(), nil, stdout, stderr)
	if err != nil || stdout.overflow || stderr.overflow || len(bytes.TrimSpace(stderr.buffer.Bytes())) != 0 {
		return fmt.Errorf("observe live Gateway configuration mount: %w: %s", err, boundedDiagnostic(stderr.buffer.Bytes()))
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout.buffer.Bytes()))
	decoder.DisallowUnknownFields()
	var observed finalGatewayMountObservation
	if err := decoder.Decode(&observed); err != nil {
		return fmt.Errorf("decode live Gateway configuration mount: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("live Gateway configuration mount contains trailing data")
	}
	componentObservation := appliedClusterComponentObservation{
		ContainerID: observed.ContainerID, ImageID: observed.ImageID, Role: observed.Role, State: observed.State, Health: observed.Health,
		NetworkAddresses: make(map[string]string, len(observed.Networks)),
	}
	for network, raw := range observed.Networks {
		var endpoint struct {
			IPAddress string `json:"IPAddress"`
		}
		if err := json.Unmarshal(raw, &endpoint); err != nil {
			return fmt.Errorf("decode live Gateway network endpoint: %w", err)
		}
		componentObservation.NetworkAddresses[network] = endpoint.IPAddress
	}
	component, componentErr := finalGatewayComponentAuthority(componentObservation)
	if observed.Owner != ownerValue || observed.Component != "gateway" || componentErr != nil || !reflect.DeepEqual(component, material.Gateway) {
		return fmt.Errorf("live Gateway component does not match the exact reviewed healthy authority")
	}
	const destination = "/run/tobari/config/gateway.json"
	source := ""
	for _, mount := range observed.Mounts {
		if mount.Destination != destination {
			continue
		}
		if source != "" || mount.Type != "bind" {
			return fmt.Errorf("live Gateway configuration mount is ambiguous")
		}
		source = mount.Source
	}
	relative, relErr := filepath.Rel(r.aggregateRoot(), source)
	if source == "" || !filepath.IsAbs(source) || filepath.Clean(source) != source || relErr != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || len(relative) >= 3 && relative[:3] == "../" {
		return fmt.Errorf("live Gateway configuration mount is outside the owned aggregate root")
	}
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("live Gateway configuration source is unsafe: %w", err)
	}
	data, err := readOwnerPolicyFile(source, 256*1024)
	if err != nil {
		return err
	}
	// GatewayArtifact is the SHA-256 of the exact bytes rather than the JSON
	// value digest used for structured private records.
	byteDigest := sha256.Sum256(data)
	digest := tobari.SemanticDigest("sha256:" + hex.EncodeToString(byteDigest[:]))
	if digest != receipt.GatewayArtifact {
		return fmt.Errorf("live Gateway configuration bytes changed after review")
	}
	return nil
}

func (r *Runtime) confirmExactFinalPrincipalRegistry(material FinalWorkspacePolicyProjection) error {
	registry, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return fmt.Errorf("read exact final principal registry: %w", err)
	}
	want := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: make([]projectPrincipalBinding, len(material.Principals))}
	for index, principal := range material.Principals {
		want.Bindings[index] = principal.gatewayBinding()
	}
	if err := want.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(registry, want) {
		return fmt.Errorf("frozen principal registry does not match final authority; hot policy activation cannot rewrite principals")
	}
	return nil
}

func (r *Runtime) readFinalPolicyActivation(path string) (finalPolicyActivationRecord, error) {
	var record finalPolicyActivationRecord
	if err := r.readFinalPolicyActivationFile(path, &record); err != nil {
		return record, err
	}
	return record, record.validate(r)
}

const maxFinalPolicyActivationBytes = 96 * 1024 * 1024

func (r *Runtime) finalPolicyActivationMaximum() int64 {
	if r != nil && r.finalPolicyActivationLimit > 0 {
		return r.finalPolicyActivationLimit
	}
	return maxFinalPolicyActivationBytes
}

func (r *Runtime) writeFinalPolicyActivation(path string, record finalPolicyActivationRecord) error {
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	maximum := r.finalPolicyActivationMaximum()
	if int64(len(encoded)+1) > maximum {
		return fmt.Errorf("final policy activation receipt exceeds %d bytes", maximum)
	}
	return writeAtomicJSON(path, record)
}

func (r *Runtime) readFinalPolicyActivationFile(path string, record *finalPolicyActivationRecord) error {
	maximum := r.finalPolicyActivationMaximum()
	file, err := os.Open(path) // #nosec G304 -- caller supplies one fixed owner-only activation child.
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > maximum {
		return fmt.Errorf("final policy activation receipt is unsafe: %w", err)
	}
	pathInfo, err := os.Lstat(path)
	if err != nil || pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, pathInfo) {
		return fmt.Errorf("final policy activation receipt path changed during open: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return fmt.Errorf("read final policy activation receipt: %w", err)
	}
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return fmt.Errorf("final policy activation receipt is ambiguous: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(record); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("final policy activation receipt contains trailing data")
	}
	return nil
}

func (r *Runtime) removeFinalPolicyActivationJournal() error {
	if err := os.Remove(r.finalPolicyActivationJournalPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	directory, err := os.Open(r.finalPolicyActivationRoot())
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// buildCurrentActiveWorkspacePolicyProjection constructs the same selected
// active axes as a hot projection without authorizing a new memory revision.
func buildCurrentActiveWorkspacePolicyProjection(collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID) (tobari.WorkspacePolicyProjection, error) {
	plan, err := tobari.BuildHotWorkspacePolicyProjection(collection, contextID)
	if err != nil {
		return tobari.WorkspacePolicyProjection{}, err
	}
	for index := range plan.Contexts {
		if plan.Contexts[index].ContextID == contextID {
			for _, record := range collection.Contexts {
				if record.Context.ID == contextID && record.ActivePolicyMemory != nil && record.ActivePolicyMemoryRef != nil {
					plan.Contexts[index].PolicyMemory = record.ActivePolicyMemory.Clone()
					plan.Contexts[index].MemoryReceipt = *record.ActivePolicyMemoryRef
					break
				}
			}
		}
	}
	return tobari.NewWorkspacePolicyProjection(plan.Mode, plan.CollectionRevision, plan.TargetContextID, plan.Contexts)
}
