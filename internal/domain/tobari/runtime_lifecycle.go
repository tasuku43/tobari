package tobari

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

const (
	RuntimePrunePlanReferenceKind = "runtime-prune-plan"
	TaskRuntimePruneDryRun        = "runtime.prune.dry-run"
	TaskRuntimePruneApply         = "runtime.prune.apply"
	TaskRuntimeDelete             = "runtime.delete"
	TaskRuntimeRestore            = "runtime.restore"
)

type RuntimeAvailability string

const (
	RuntimeAvailabilityAvailable  RuntimeAvailability = "available"
	RuntimeAvailabilityMissing    RuntimeAvailability = "missing"
	RuntimeAvailabilityMismatched RuntimeAvailability = "mismatched"
	RuntimeAvailabilityUnknown    RuntimeAvailability = "unknown"
	RuntimeAvailabilityPruned     RuntimeAvailability = "pruned"
)

func (a RuntimeAvailability) Validate() error {
	switch a {
	case RuntimeAvailabilityAvailable, RuntimeAvailabilityMissing, RuntimeAvailabilityMismatched, RuntimeAvailabilityUnknown, RuntimeAvailabilityPruned:
		return nil
	default:
		return fmt.Errorf("Runtime availability is invalid")
	}
}

// RuntimeMaterialObservation contains bounded infrastructure evidence for one
// immutable managed revision. Docker selectors and IDs remain below domain.
type RuntimeMaterialObservation struct {
	RuntimeID           string              `json:"runtime_id"`
	Revision            string              `json:"revision"`
	Availability        RuntimeAvailability `json:"availability"`
	OwnershipVerified   bool                `json:"ownership_verified"`
	ObservationComplete bool                `json:"observation_complete"`
	WorkspaceInUse      bool                `json:"workspace_in_use"`
	ExternalInUse       bool                `json:"external_in_use"`
	ImageVirtualBytes   *int64              `json:"image_virtual_bytes"`
}

func (o RuntimeMaterialObservation) Validate() error {
	if err := ValidateRuntimeID(o.RuntimeID); err != nil {
		return err
	}
	if err := ValidateDigest(o.Revision); err != nil {
		return err
	}
	if err := o.Availability.Validate(); err != nil {
		return err
	}
	if !o.ObservationComplete {
		return RuntimeProtectionInventoryError{Reason: RuntimeProtectionInventoryObservationUnknown}
	}
	if o.Availability == RuntimeAvailabilityAvailable && !o.OwnershipVerified {
		return fmt.Errorf("available Runtime material lacks ownership evidence")
	}
	if o.ImageVirtualBytes != nil && *o.ImageVirtualBytes < 0 {
		return fmt.Errorf("Runtime image virtual bytes cannot be negative")
	}
	return nil
}

type RuntimeLifecycleSnapshot struct {
	Runtimes   []RuntimeManifest            `json:"runtimes"`
	Protection RuntimeProtectionInventory   `json:"protection"`
	Materials  []RuntimeMaterialObservation `json:"materials"`
}

func (s RuntimeLifecycleSnapshot) Validate() error {
	if s.Runtimes == nil || s.Materials == nil {
		return fmt.Errorf("Runtime lifecycle snapshot is incomplete")
	}
	if err := s.Protection.Validate(); err != nil {
		return err
	}
	runtimes := make(map[string]RuntimeManifest, len(s.Runtimes))
	revisions := make(map[string]struct{})
	for _, runtime := range s.Runtimes {
		if err := runtime.Validate(); err != nil {
			return err
		}
		if _, exists := runtimes[runtime.ID]; exists {
			return fmt.Errorf("Runtime lifecycle snapshot contains duplicate Runtime identity")
		}
		runtimes[runtime.ID] = runtime
		if runtime.Kind == RuntimeKindManaged {
			for _, revision := range runtime.Revisions {
				revisions[runtime.ID+"\x00"+revision.Revision] = struct{}{}
			}
		}
	}
	for _, protection := range s.Protection.Items {
		if _, exists := revisions[protection.RuntimeID+"\x00"+protection.RuntimeRevision]; !exists {
			return fmt.Errorf("Runtime protection has no immutable revision authority")
		}
	}
	seen := make(map[string]struct{}, len(s.Materials))
	for _, material := range s.Materials {
		if err := material.Validate(); err != nil {
			return err
		}
		key := material.RuntimeID + "\x00" + material.Revision
		if _, exists := revisions[key]; !exists {
			return fmt.Errorf("Runtime material has no immutable revision authority")
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("Runtime lifecycle snapshot contains duplicate material")
		}
		seen[key] = struct{}{}
	}
	if len(seen) != len(revisions) {
		return RuntimeProtectionInventoryError{Reason: RuntimeProtectionInventoryObservationUnknown}
	}
	return nil
}

type RuntimePruneCandidate struct {
	RuntimeID         string `json:"runtime_id"`
	Revision          string `json:"revision"`
	RuntimeRef        string `json:"runtime_ref"`
	RevisionRef       string `json:"revision_ref"`
	Name              string `json:"name"`
	Ordinal           int    `json:"ordinal"`
	ImageVirtualBytes *int64 `json:"image_virtual_bytes"`
}

func (c RuntimePruneCandidate) Validate() error {
	if err := ValidateRuntimeID(c.RuntimeID); err != nil {
		return err
	}
	if err := ValidateDigest(c.Revision); err != nil {
		return err
	}
	if c.RuntimeRef != RuntimeRef(c.RuntimeID) || c.RevisionRef != RuntimeRevisionRef(c.RuntimeID, c.Revision) {
		return fmt.Errorf("Runtime prune candidate references are invalid")
	}
	if err := ValidateName(c.Name); err != nil || c.Ordinal < 1 {
		return fmt.Errorf("Runtime prune candidate presentation is invalid")
	}
	return nil
}

type RuntimePrunePlan struct {
	Task       string                  `json:"task"`
	PlanRef    string                  `json:"plan_ref"`
	ObservedAt time.Time               `json:"observed_at"`
	Empty      bool                    `json:"empty"`
	Candidates []RuntimePruneCandidate `json:"candidates"`
	Protected  []RuntimeProtection     `json:"protected"`
}

func (p RuntimePrunePlan) Validate() error {
	if p.Task != TaskRuntimePruneDryRun || ValidateRuntimePrunePlanRef(p.PlanRef) != nil || p.ObservedAt.IsZero() || p.ObservedAt.Location() != time.UTC || p.Candidates == nil || p.Protected == nil || p.Empty != (len(p.Candidates) == 0) {
		return fmt.Errorf("Runtime prune plan is invalid")
	}
	for _, candidate := range p.Candidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func ValidateRuntimePrunePlanRef(reference string) error {
	if err := ValidateDigest(reference); err != nil {
		return fmt.Errorf("Runtime prune plan reference: %w", err)
	}
	return nil
}

func PlanRuntimePrune(snapshot RuntimeLifecycleSnapshot, observedAt time.Time) (RuntimePrunePlan, error) {
	if err := snapshot.Validate(); err != nil {
		return RuntimePrunePlan{}, err
	}
	if observedAt.IsZero() || observedAt.Location() != time.UTC {
		return RuntimePrunePlan{}, fmt.Errorf("Runtime prune observation time must be UTC")
	}
	protected := make(map[string]bool)
	for _, item := range snapshot.Protection.Items {
		protected[item.RuntimeID+"\x00"+item.RuntimeRevision] = true
	}
	byID := make(map[string]RuntimeManifest, len(snapshot.Runtimes))
	for _, runtime := range snapshot.Runtimes {
		byID[runtime.ID] = runtime
	}
	candidates := make([]RuntimePruneCandidate, 0)
	for _, material := range snapshot.Materials {
		if material.Availability != RuntimeAvailabilityAvailable || protected[material.RuntimeID+"\x00"+material.Revision] || material.WorkspaceInUse || material.ExternalInUse {
			continue
		}
		runtime := byID[material.RuntimeID]
		if runtime.Kind != RuntimeKindManaged {
			continue
		}
		for _, revision := range runtime.Revisions {
			if revision.Revision == material.Revision {
				candidates = append(candidates, RuntimePruneCandidate{RuntimeID: runtime.ID, Revision: revision.Revision, RuntimeRef: RuntimeRef(runtime.ID), RevisionRef: RuntimeRevisionRef(runtime.ID, revision.Revision), Name: runtime.Name, Ordinal: revision.Ordinal, ImageVirtualBytes: material.ImageVirtualBytes})
				break
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].RuntimeID != candidates[j].RuntimeID {
			return candidates[i].RuntimeID < candidates[j].RuntimeID
		}
		return candidates[i].Revision < candidates[j].Revision
	})
	protectedItems := append([]RuntimeProtection{}, snapshot.Protection.Items...)
	sort.Slice(protectedItems, func(i, j int) bool {
		a, b := protectedItems[i], protectedItems[j]
		return a.RuntimeID+"\x00"+a.RuntimeRevision+"\x00"+string(a.Reason)+"\x00"+a.WorkspaceManifestID+"\x00"+a.ManifestRevision+"\x00"+a.WorkspaceID < b.RuntimeID+"\x00"+b.RuntimeRevision+"\x00"+string(b.Reason)+"\x00"+b.WorkspaceManifestID+"\x00"+b.ManifestRevision+"\x00"+b.WorkspaceID
	})
	canonical := struct {
		Schema     int                     `json:"schema"`
		Candidates []RuntimePruneCandidate `json:"candidates"`
		Protected  []RuntimeProtection     `json:"protected"`
	}{Schema: 1, Candidates: candidates, Protected: protectedItems}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return RuntimePrunePlan{}, err
	}
	digest := sha256.Sum256(encoded)
	plan := RuntimePrunePlan{Task: TaskRuntimePruneDryRun, PlanRef: "sha256:" + hex.EncodeToString(digest[:]), ObservedAt: observedAt, Empty: len(candidates) == 0, Candidates: candidates, Protected: protectedItems}
	return plan, plan.Validate()
}
