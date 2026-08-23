package tobari

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	RuntimePrunePlanReferenceKind = "runtime-prune-plan"
	TaskRuntimePruneDryRun        = "runtime.prune.dry-run"
	TaskRuntimePruneApply         = "runtime.prune.apply"
	TaskRuntimeDelete             = "runtime.delete"
	TaskRuntimeRestore            = "runtime.restore"
)

type RuntimeRestoreState string

const (
	RuntimeRestored         RuntimeRestoreState = "restored"
	RuntimeAlreadyAvailable RuntimeRestoreState = "already_available"
)

type RuntimeRestoreArtifactDisposition string

const (
	RuntimeRestoreArtifactNotCreated RuntimeRestoreArtifactDisposition = "not_created"
	RuntimeRestoreArtifactRemoved    RuntimeRestoreArtifactDisposition = "removed"
)

// RuntimeRestoreTarget is task-owned internal authority derived from one
// complete lifecycle snapshot. Paths and Docker selectors stay below the
// application boundary; the recorded content digest is exact comparison
// evidence, not a public target.
type RuntimeRestoreTarget struct {
	RuntimeID            string              `json:"runtime_id"`
	RuntimeRef           string              `json:"runtime_ref"`
	Revision             string              `json:"revision"`
	RevisionRef          string              `json:"revision_ref"`
	Name                 string              `json:"name"`
	Ordinal              int                 `json:"ordinal"`
	RecordedImageDigest  string              `json:"recorded_image_digest"`
	SnapshotLogicalBytes int64               `json:"snapshot_logical_bytes"`
	Availability         RuntimeAvailability `json:"availability"`
}

func (t RuntimeRestoreTarget) Validate() error {
	if ValidateRuntimeID(t.RuntimeID) != nil || t.RuntimeRef != RuntimeRef(t.RuntimeID) || ValidateDigest(t.Revision) != nil ||
		t.RevisionRef != RuntimeRevisionRef(t.RuntimeID, t.Revision) || ValidateName(t.Name) != nil || t.Ordinal < 1 ||
		ValidateDigest(t.RecordedImageDigest) != nil || t.SnapshotLogicalBytes < 0 {
		return fmt.Errorf("Runtime restore target authority is invalid")
	}
	switch t.Availability {
	case RuntimeAvailabilityAvailable, RuntimeAvailabilityMissing, RuntimeAvailabilityPruned:
		return nil
	default:
		return fmt.Errorf("Runtime restore target availability is not actionable")
	}
}

// RuntimeRestoreTargetFrom consumes one complete coherent snapshot and binds
// an exact immutable managed revision. Unknown, mismatched, or concurrent
// lifecycle evidence fails closed before the restore effect boundary.
func RuntimeRestoreTargetFrom(snapshot RuntimeLifecycleSnapshot, reference string) (RuntimeRestoreTarget, error) {
	if err := snapshot.Validate(); err != nil {
		return RuntimeRestoreTarget{}, fmt.Errorf("%w: %w", ErrRuntimeRetirementObservationUnknown, err)
	}
	runtimeID, revisionDigest, err := ParseRuntimeRevisionRef(reference)
	if err != nil {
		return RuntimeRestoreTarget{}, err
	}
	var runtime RuntimeManifest
	foundRuntime := false
	for _, candidate := range snapshot.Runtimes {
		if candidate.ID == runtimeID {
			runtime, foundRuntime = candidate, true
			break
		}
	}
	if !foundRuntime {
		return RuntimeRestoreTarget{}, ErrRuntimeNotFound
	}
	if runtime.Kind != RuntimeKindManaged {
		return RuntimeRestoreTarget{}, ErrRuntimeRevisionUnrestorable
	}
	var revision RuntimeRevision
	foundRevision := false
	for _, candidate := range runtime.Revisions {
		if candidate.Revision == revisionDigest {
			revision, foundRevision = candidate, true
			break
		}
	}
	if !foundRevision {
		return RuntimeRestoreTarget{}, ErrRuntimeRevisionNotFound
	}
	for _, activity := range snapshot.Journals.Active {
		if activity.RuntimeID == runtimeID {
			return RuntimeRestoreTarget{}, ErrRuntimeLifecycleActive
		}
	}
	var material RuntimeMaterialObservation
	foundMaterial := false
	for _, candidate := range snapshot.Materials {
		if candidate.RuntimeID == runtimeID && candidate.Revision == revisionDigest {
			material, foundMaterial = candidate, true
			break
		}
	}
	if !foundMaterial {
		return RuntimeRestoreTarget{}, fmt.Errorf("%w: Runtime material is absent", ErrRuntimeRetirementObservationUnknown)
	}
	switch material.Availability {
	case RuntimeAvailabilityAvailable, RuntimeAvailabilityMissing, RuntimeAvailabilityPruned:
	case RuntimeAvailabilityMismatched:
		return RuntimeRestoreTarget{}, ErrRuntimeRevisionUnrestorable
	default:
		return RuntimeRestoreTarget{}, ErrRuntimeRetirementObservationUnknown
	}
	snapshotBytes := int64(-1)
	for _, storage := range snapshot.Storage {
		if storage.RuntimeID != runtimeID {
			continue
		}
		for _, candidate := range storage.Snapshots {
			if candidate.Kind == RuntimePruneCandidateRevision && candidate.Revision == revisionDigest && candidate.SemanticFingerprint == revisionDigest {
				snapshotBytes = candidate.LogicalBytes
				break
			}
		}
		break
	}
	target := RuntimeRestoreTarget{
		RuntimeID: runtime.ID, RuntimeRef: RuntimeRef(runtime.ID), Revision: revision.Revision,
		RevisionRef: reference, Name: runtime.Name, Ordinal: revision.Ordinal,
		RecordedImageDigest: revision.ImageDigest, SnapshotLogicalBytes: snapshotBytes, Availability: material.Availability,
	}
	if err := target.Validate(); err != nil {
		return RuntimeRestoreTarget{}, fmt.Errorf("%w: %w", ErrRuntimeRetirementObservationUnknown, err)
	}
	return target, nil
}

// RuntimeRestoreResult confirms only the availability facet of one immutable
// revision. It cannot append or rewrite revision, Manifest, or Workspace
// authority.
type RuntimeRestoreResult struct {
	Task                string                            `json:"task"`
	RuntimeID           string                            `json:"runtime_id"`
	RuntimeRef          string                            `json:"runtime_ref"`
	Revision            string                            `json:"revision"`
	RevisionRef         string                            `json:"revision_ref"`
	Name                string                            `json:"name"`
	Ordinal             int                               `json:"ordinal"`
	State               RuntimeRestoreState               `json:"state"`
	DigestMatch         bool                              `json:"digest_match"`
	ArtifactDisposition RuntimeRestoreArtifactDisposition `json:"artifact_disposition"`
	RevisionAppended    bool                              `json:"revision_appended"`
	ManifestChanged     bool                              `json:"manifest_changed"`
	WorkspaceChanged    bool                              `json:"workspace_changed"`
}

func (r RuntimeRestoreResult) Validate() error {
	if r.Task != TaskRuntimeRestore || ValidateRuntimeID(r.RuntimeID) != nil || r.RuntimeRef != RuntimeRef(r.RuntimeID) ||
		ValidateDigest(r.Revision) != nil || r.RevisionRef != RuntimeRevisionRef(r.RuntimeID, r.Revision) ||
		ValidateName(r.Name) != nil || r.Ordinal < 1 || !r.DigestMatch || r.RevisionAppended || r.ManifestChanged || r.WorkspaceChanged {
		return fmt.Errorf("Runtime restore result authority is invalid")
	}
	switch r.State {
	case RuntimeRestored:
		if r.ArtifactDisposition != RuntimeRestoreArtifactRemoved {
			return fmt.Errorf("restored Runtime revision lacks staging cleanup evidence")
		}
	case RuntimeAlreadyAvailable:
		if r.ArtifactDisposition != RuntimeRestoreArtifactNotCreated {
			return fmt.Errorf("already-available Runtime revision has build artifact evidence")
		}
	default:
		return fmt.Errorf("Runtime restore result state is invalid")
	}
	return nil
}

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
	RuntimeID           string                 `json:"runtime_id"`
	Revision            string                 `json:"revision"`
	TagRole             RuntimeMaterialTagRole `json:"tag_role"`
	Availability        RuntimeAvailability    `json:"availability"`
	TagPresent          bool                   `json:"tag_present"`
	ContentPresent      bool                   `json:"content_present"`
	SharedContent       bool                   `json:"shared_content"`
	OwnershipVerified   bool                   `json:"ownership_verified"`
	MigrationUnverified bool                   `json:"migration_unverified"`
	ObservationComplete bool                   `json:"observation_complete"`
	WorkspaceInUse      bool                   `json:"workspace_in_use"`
	ExternalInUse       bool                   `json:"external_in_use"`
	ImageVirtualBytes   *int64                 `json:"image_virtual_bytes"`
}

type RuntimeMaterialTagRole string

const (
	RuntimeMaterialTagPublishedRevision RuntimeMaterialTagRole = "published_revision"
	RuntimeMaterialTagJournaledStaging  RuntimeMaterialTagRole = "journaled_staging"
)

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
	if o.TagRole != RuntimeMaterialTagPublishedRevision && o.TagRole != RuntimeMaterialTagJournaledStaging {
		return fmt.Errorf("Runtime material tag role is invalid")
	}
	if !o.ObservationComplete {
		return RuntimeProtectionInventoryError{Reason: RuntimeProtectionInventoryObservationUnknown}
	}
	if o.Availability == RuntimeAvailabilityAvailable && (!o.TagPresent || !o.ContentPresent || !o.OwnershipVerified) {
		return fmt.Errorf("available Runtime material lacks ownership evidence")
	}
	if o.Availability == RuntimeAvailabilityMissing && o.TagPresent {
		return fmt.Errorf("missing Runtime material cannot have its expected owned tag")
	}
	if o.Availability == RuntimeAvailabilityMismatched && !o.TagPresent {
		return fmt.Errorf("mismatched Runtime material requires an observed expected owned tag")
	}
	if o.OwnershipVerified && !o.ContentPresent {
		return fmt.Errorf("Runtime material ownership requires recorded content")
	}
	if o.SharedContent && !o.ContentPresent {
		return fmt.Errorf("shared Runtime content must be present")
	}
	if o.MigrationUnverified && o.Availability != RuntimeAvailabilityUnknown {
		return fmt.Errorf("migration-unverified Runtime material must remain unknown")
	}
	if o.ImageVirtualBytes != nil && *o.ImageVirtualBytes < 0 {
		return fmt.Errorf("Runtime image virtual bytes cannot be negative")
	}
	return nil
}

type RuntimeLifecycleSnapshot struct {
	CatalogComplete bool                         `json:"catalog_complete"`
	Runtimes        []RuntimeManifest            `json:"runtimes"`
	Protection      RuntimeProtectionInventory   `json:"protection"`
	Materials       []RuntimeMaterialObservation `json:"materials"`
	Storage         []RuntimeStorageObservation  `json:"storage"`
	Journals        RuntimeLifecycleJournals     `json:"journals"`
	// RetirementGenerations is internal durable authority that distinguishes a
	// newly reviewed prune after an exact restore from replay of an older prune
	// receipt. Public Runtime projections deliberately omit it.
	RetirementGenerations []RuntimeRetirementGeneration `json:"-"`
}

type RuntimeRetirementGeneration struct {
	RuntimeID  string `json:"runtime_id"`
	Revision   string `json:"revision"`
	Generation uint64 `json:"generation"`
}

func (g RuntimeRetirementGeneration) Validate() error {
	if ValidateRuntimeID(g.RuntimeID) != nil || ValidateDigest(g.Revision) != nil || g.Generation == 0 {
		return fmt.Errorf("Runtime retirement generation authority is invalid")
	}
	return nil
}

type RuntimeSnapshotStorage struct {
	Kind                RuntimePruneCandidateKind `json:"kind"`
	Revision            string                    `json:"revision"`
	SemanticFingerprint string                    `json:"semantic_fingerprint"`
	LogicalBytes        int64                     `json:"logical_bytes"`
}

func (s RuntimeSnapshotStorage) Validate() error {
	if s.Kind != RuntimePruneCandidateRevision && s.Kind != RuntimePruneCandidateFailedBuild {
		return fmt.Errorf("Runtime snapshot storage kind is invalid")
	}
	if err := ValidateDigest(s.Revision); err != nil {
		return err
	}
	if ValidateDigest(s.SemanticFingerprint) != nil || s.SemanticFingerprint != s.Revision {
		return fmt.Errorf("Runtime snapshot semantic fingerprint is invalid")
	}
	if s.LogicalBytes < 0 {
		return fmt.Errorf("Runtime snapshot logical bytes cannot be negative")
	}
	return nil
}

type RuntimeStorageObservation struct {
	RuntimeID          string                   `json:"runtime_id"`
	Name               string                   `json:"name"`
	SourceLogicalBytes int64                    `json:"source_logical_bytes"`
	Snapshots          []RuntimeSnapshotStorage `json:"snapshots"`
}

func (s RuntimeStorageObservation) Validate() error {
	if ValidateRuntimeID(s.RuntimeID) != nil || ValidateName(s.Name) != nil || s.SourceLogicalBytes < 0 || s.Snapshots == nil {
		return fmt.Errorf("Runtime storage observation is invalid")
	}
	for index, snapshot := range s.Snapshots {
		if err := snapshot.Validate(); err != nil {
			return err
		}
		if index > 0 && runtimeSnapshotStorageKey(s.Snapshots[index-1]) >= runtimeSnapshotStorageKey(snapshot) {
			return fmt.Errorf("Runtime snapshot storage is not unique canonical order")
		}
	}
	return nil
}

func (s RuntimeLifecycleSnapshot) Validate() error {
	if !s.CatalogComplete || s.Runtimes == nil || s.Materials == nil || s.Storage == nil {
		return fmt.Errorf("Runtime lifecycle snapshot is incomplete")
	}
	if err := s.Protection.Validate(); err != nil {
		return err
	}
	if err := s.Journals.Validate(); err != nil {
		return err
	}
	runtimes := make(map[string]RuntimeManifest, len(s.Runtimes))
	revisions := make(map[string]struct{})
	standard := 0
	for _, runtime := range s.Runtimes {
		if err := runtime.Validate(); err != nil {
			return err
		}
		if _, exists := runtimes[runtime.ID]; exists {
			return fmt.Errorf("Runtime lifecycle snapshot contains duplicate Runtime identity")
		}
		runtimes[runtime.ID] = runtime
		if runtime.ID == StandardRuntimeID {
			standard++
		}
		if runtime.Kind == RuntimeKindManaged {
			for _, revision := range runtime.Revisions {
				revisions[runtime.ID+"\x00"+revision.Revision] = struct{}{}
			}
		}
	}
	if standard != 1 {
		return fmt.Errorf("Runtime lifecycle snapshot requires exact built-in standard evidence")
	}
	for index, generation := range s.RetirementGenerations {
		if err := generation.Validate(); err != nil {
			return err
		}
		if _, exists := revisions[generation.RuntimeID+"\x00"+generation.Revision]; !exists {
			return fmt.Errorf("Runtime retirement generation has no immutable revision authority")
		}
		if index > 0 && runtimeRetirementGenerationKey(s.RetirementGenerations[index-1]) >= runtimeRetirementGenerationKey(generation) {
			return fmt.Errorf("Runtime retirement generations are not unique canonical order")
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
		if material.TagRole != RuntimeMaterialTagPublishedRevision {
			return fmt.Errorf("successful Runtime revision requires published-tag evidence")
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
	failedBuilds := make(map[string]struct{}, len(s.Journals.FailedBuilds))
	for _, artifact := range s.Journals.FailedBuilds {
		failedBuilds[artifact.RuntimeID+"\x00"+artifact.Revision] = struct{}{}
	}
	activeBuilds := make(map[string]struct{})
	for _, activity := range s.Journals.Active {
		if activity.Kind == RuntimeLifecycleActivityBuild {
			for _, revision := range activity.Revisions {
				activeBuilds[activity.RuntimeID+"\x00"+revision] = struct{}{}
			}
		}
	}
	for _, activity := range s.Journals.Active {
		runtime, exists := runtimes[activity.RuntimeID]
		if !exists || runtime.Kind != RuntimeKindManaged {
			return fmt.Errorf("Runtime lifecycle activity has no managed Runtime authority")
		}
		if activity.Kind == RuntimeLifecycleActivityRestore || activity.Kind == RuntimeLifecycleActivityPrune {
			for _, revision := range activity.Revisions {
				key := activity.RuntimeID + "\x00" + revision
				_, successful := revisions[key]
				_, failed := failedBuilds[key]
				_, activeBuild := activeBuilds[key]
				if !successful && !(activity.Kind == RuntimeLifecycleActivityPrune && (failed || activeBuild)) {
					return fmt.Errorf("Runtime lifecycle activity has no immutable revision authority")
				}
			}
		}
	}
	for _, artifact := range s.Journals.FailedBuilds {
		runtime, exists := runtimes[artifact.RuntimeID]
		if !exists || runtime.Kind != RuntimeKindManaged || runtime.Name != artifact.Name {
			return fmt.Errorf("failed Runtime build artifact has no managed Runtime authority")
		}
		if _, exists := revisions[artifact.RuntimeID+"\x00"+artifact.Revision]; exists {
			return fmt.Errorf("failed Runtime build artifact overlaps successful history")
		}
	}
	storageByRuntime := make(map[string]RuntimeStorageObservation, len(s.Storage))
	for index, storage := range s.Storage {
		if err := storage.Validate(); err != nil {
			return err
		}
		if index > 0 && runtimeStorageObservationKey(s.Storage[index-1]) >= runtimeStorageObservationKey(storage) {
			return fmt.Errorf("Runtime storage observations are not unique canonical order")
		}
		runtime, exists := runtimes[storage.RuntimeID]
		if !exists || runtime.Kind != RuntimeKindManaged || runtime.Name != storage.Name {
			return fmt.Errorf("Runtime storage has no managed Runtime authority")
		}
		if _, duplicate := storageByRuntime[storage.RuntimeID]; duplicate {
			return fmt.Errorf("Runtime lifecycle snapshot contains duplicate Runtime storage")
		}
		want := make(map[string]struct{}, len(runtime.Revisions)+len(s.Journals.FailedBuilds))
		for _, revision := range runtime.Revisions {
			want[string(RuntimePruneCandidateRevision)+"\x00"+revision.Revision] = struct{}{}
		}
		for _, artifact := range s.Journals.FailedBuilds {
			if artifact.RuntimeID == runtime.ID {
				want[string(RuntimePruneCandidateFailedBuild)+"\x00"+artifact.Revision] = struct{}{}
			}
		}
		for _, snapshot := range storage.Snapshots {
			key := runtimeSnapshotStorageKey(snapshot)
			if _, exists := want[key]; !exists {
				return fmt.Errorf("Runtime snapshot storage has no revision authority")
			}
			delete(want, key)
		}
		if len(want) != 0 {
			return fmt.Errorf("Runtime snapshot storage inventory is incomplete")
		}
		storageByRuntime[storage.RuntimeID] = storage
	}
	for _, runtime := range s.Runtimes {
		if runtime.Kind == RuntimeKindManaged {
			if _, exists := storageByRuntime[runtime.ID]; !exists {
				return fmt.Errorf("Runtime source storage inventory is incomplete")
			}
		}
	}
	return nil
}

type RuntimeLifecycleActivityKind string

const (
	RuntimeLifecycleActivityBuild   RuntimeLifecycleActivityKind = "build"
	RuntimeLifecycleActivityRestore RuntimeLifecycleActivityKind = "restore"
	RuntimeLifecycleActivityPrune   RuntimeLifecycleActivityKind = "prune"
	RuntimeLifecycleActivityDelete  RuntimeLifecycleActivityKind = "delete"
)

type RuntimeLifecycleActivity struct {
	Kind      RuntimeLifecycleActivityKind `json:"kind"`
	RuntimeID string                       `json:"runtime_id"`
	Revisions []string                     `json:"revisions"`
}

func (a RuntimeLifecycleActivity) Validate() error {
	if err := ValidateRuntimeID(a.RuntimeID); err != nil {
		return err
	}
	if a.Revisions == nil {
		return fmt.Errorf("Runtime lifecycle activity revisions are incomplete")
	}
	switch a.Kind {
	case RuntimeLifecycleActivityBuild:
		if len(a.Revisions) > 1 {
			return fmt.Errorf("Runtime build activity has at most one observed semantic revision")
		}
	case RuntimeLifecycleActivityRestore:
		if len(a.Revisions) != 1 {
			return fmt.Errorf("Runtime restore activity requires one semantic revision")
		}
	case RuntimeLifecycleActivityPrune:
		if len(a.Revisions) == 0 {
			return fmt.Errorf("Runtime prune activity requires exact semantic revisions")
		}
	case RuntimeLifecycleActivityDelete:
		if len(a.Revisions) != 0 {
			return fmt.Errorf("Runtime delete activity is Runtime-wide")
		}
	default:
		return fmt.Errorf("Runtime lifecycle activity kind is invalid")
	}
	previous := ""
	for _, revision := range a.Revisions {
		if err := ValidateDigest(revision); err != nil {
			return err
		}
		if previous >= revision {
			return fmt.Errorf("Runtime lifecycle activity revisions are not unique canonical order")
		}
		previous = revision
	}
	return nil
}

type RuntimeFailedBuildArtifact struct {
	RuntimeID  string                     `json:"runtime_id"`
	Revision   string                     `json:"revision"`
	RuntimeRef string                     `json:"runtime_ref"`
	Name       string                     `json:"name"`
	Material   RuntimeMaterialObservation `json:"material"`
}

func (a RuntimeFailedBuildArtifact) Validate() error {
	if err := ValidateRuntimeID(a.RuntimeID); err != nil {
		return err
	}
	if err := ValidateDigest(a.Revision); err != nil {
		return err
	}
	if a.RuntimeRef != RuntimeRef(a.RuntimeID) || ValidateName(a.Name) != nil || a.Material.RuntimeID != a.RuntimeID || a.Material.Revision != a.Revision {
		return fmt.Errorf("failed Runtime build artifact authority is invalid")
	}
	if a.Material.TagRole != RuntimeMaterialTagJournaledStaging {
		return fmt.Errorf("failed Runtime build artifact requires journaled staging-tag evidence")
	}
	return a.Material.Validate()
}

type RuntimeLifecycleJournals struct {
	Complete     bool                         `json:"complete"`
	Active       []RuntimeLifecycleActivity   `json:"active"`
	FailedBuilds []RuntimeFailedBuildArtifact `json:"failed_builds"`
}

type RuntimeBuildRecoveryKind string

const (
	RuntimeBuildRecoveryPreDocker   RuntimeBuildRecoveryKind = "pre_docker"
	RuntimeBuildRecoveryBuilding    RuntimeBuildRecoveryKind = "building"
	RuntimeBuildRecoveryPublication RuntimeBuildRecoveryKind = "publication"
	RuntimeBuildRecoveryCleanup     RuntimeBuildRecoveryKind = "cleanup"
	RuntimeBuildRecoveryOrphan      RuntimeBuildRecoveryKind = "orphan_staging"
	RuntimeBuildRecoveryFailed      RuntimeBuildRecoveryKind = "failed_build"
)

type RuntimeBuildRecovery struct {
	RuntimeID     string                   `json:"runtime_id"`
	RuntimeRef    string                   `json:"runtime_ref"`
	RevisionRef   string                   `json:"revision_ref,omitempty"`
	Name          string                   `json:"name"`
	Kind          RuntimeBuildRecoveryKind `json:"kind"`
	RestoreFailed bool                     `json:"restore_failed,omitempty"`
}

func (r RuntimeBuildRecovery) Validate() error {
	if ValidateRuntimeID(r.RuntimeID) != nil || ValidateRuntimeRef(r.RuntimeRef) != nil || r.RuntimeRef != RuntimeRef(r.RuntimeID) || ValidateName(r.Name) != nil {
		return fmt.Errorf("Runtime build recovery identity is invalid")
	}
	if r.RevisionRef != "" {
		runtimeID, _, err := ParseRuntimeRevisionRef(r.RevisionRef)
		if err != nil || runtimeID != r.RuntimeID {
			return fmt.Errorf("Runtime restore recovery revision authority is invalid")
		}
	} else if r.RestoreFailed {
		return fmt.Errorf("Runtime build recovery cannot carry restore failure evidence")
	}
	if r.RestoreFailed && r.Kind != RuntimeBuildRecoveryFailed && r.Kind != RuntimeBuildRecoveryCleanup {
		return fmt.Errorf("Runtime restore failure evidence has invalid recovery phase")
	}
	switch r.Kind {
	case RuntimeBuildRecoveryPreDocker, RuntimeBuildRecoveryBuilding, RuntimeBuildRecoveryPublication, RuntimeBuildRecoveryCleanup, RuntimeBuildRecoveryOrphan, RuntimeBuildRecoveryFailed:
		return nil
	default:
		return fmt.Errorf("Runtime build recovery kind is invalid")
	}
}

func (j RuntimeLifecycleJournals) Validate() error {
	if !j.Complete || j.Active == nil || j.FailedBuilds == nil {
		return fmt.Errorf("Runtime lifecycle journal inventory is incomplete")
	}
	for index, activity := range j.Active {
		if err := activity.Validate(); err != nil {
			return err
		}
		if index > 0 && runtimeLifecycleActivityKey(j.Active[index-1]) >= runtimeLifecycleActivityKey(activity) {
			return fmt.Errorf("Runtime lifecycle activities are not unique canonical order")
		}
	}
	for index, artifact := range j.FailedBuilds {
		if err := artifact.Validate(); err != nil {
			return err
		}
		if index > 0 && runtimeFailedBuildArtifactKey(j.FailedBuilds[index-1]) >= runtimeFailedBuildArtifactKey(artifact) {
			return fmt.Errorf("failed Runtime build artifacts are not unique canonical order")
		}
	}
	return nil
}

type RuntimePruneCandidate struct {
	Kind                 RuntimePruneCandidateKind `json:"kind"`
	RuntimeID            string                    `json:"runtime_id"`
	Revision             string                    `json:"revision"`
	RuntimeRef           string                    `json:"runtime_ref"`
	RevisionRef          string                    `json:"revision_ref"`
	Name                 string                    `json:"name"`
	Ordinal              int                       `json:"ordinal"`
	LastUsed             RuntimeLastUsedState      `json:"last_used"`
	SourceLogicalBytes   int64                     `json:"source_logical_bytes"`
	SnapshotLogicalBytes int64                     `json:"snapshot_logical_bytes"`
	ImageVirtualBytes    *int64                    `json:"image_virtual_bytes"`
	ReclaimableBytes     *int64                    `json:"reclaimable_bytes"`
}

type RuntimePruneCandidateKind string

const (
	RuntimePruneCandidateRevision    RuntimePruneCandidateKind = "runtime_revision"
	RuntimePruneCandidateFailedBuild RuntimePruneCandidateKind = "failed_build"
)

type RuntimeLastUsedState string

const RuntimeLastUsedUnknown RuntimeLastUsedState = "unknown"

func (c RuntimePruneCandidate) Validate() error {
	if err := ValidateRuntimeID(c.RuntimeID); err != nil {
		return err
	}
	if err := ValidateDigest(c.Revision); err != nil {
		return err
	}
	if c.RuntimeRef != RuntimeRef(c.RuntimeID) {
		return fmt.Errorf("Runtime prune candidate references are invalid")
	}
	switch c.Kind {
	case RuntimePruneCandidateRevision:
		if c.RevisionRef != RuntimeRevisionRef(c.RuntimeID, c.Revision) || c.Ordinal < 1 {
			return fmt.Errorf("Runtime revision prune candidate authority is invalid")
		}
	case RuntimePruneCandidateFailedBuild:
		if c.RevisionRef != "" || c.Ordinal != 0 {
			return fmt.Errorf("failed Runtime build prune candidate authority is invalid")
		}
	default:
		return fmt.Errorf("Runtime prune candidate kind is invalid")
	}
	if err := ValidateName(c.Name); err != nil {
		return fmt.Errorf("Runtime prune candidate presentation is invalid")
	}
	if c.LastUsed != RuntimeLastUsedUnknown || c.ReclaimableBytes != nil {
		return fmt.Errorf("Runtime prune candidate certainty is invalid")
	}
	if c.SourceLogicalBytes < 0 || c.SnapshotLogicalBytes < 0 || (c.ImageVirtualBytes != nil && *c.ImageVirtualBytes < 0) {
		return fmt.Errorf("Runtime prune candidate byte evidence cannot be negative")
	}
	return nil
}

type RuntimeMaterialBlockerReason string

const (
	RuntimeBlockedByWorkspaceContainer  RuntimeMaterialBlockerReason = "workspace_container"
	RuntimeBlockedByExternalContainer   RuntimeMaterialBlockerReason = "external_container"
	RuntimeBlockedByImageMissing        RuntimeMaterialBlockerReason = "image_missing"
	RuntimeBlockedByImageTagMissing     RuntimeMaterialBlockerReason = "image_tag_missing_content_present"
	RuntimeBlockedByImageTagShared      RuntimeMaterialBlockerReason = "image_tag_missing_content_shared"
	RuntimeBlockedByStagingMissing      RuntimeMaterialBlockerReason = "staging_image_missing"
	RuntimeBlockedByStagingTagMissing   RuntimeMaterialBlockerReason = "staging_tag_missing_content_present"
	RuntimeBlockedByStagingTagShared    RuntimeMaterialBlockerReason = "staging_tag_missing_content_shared"
	RuntimeBlockedByImageMismatched     RuntimeMaterialBlockerReason = "image_mismatched"
	RuntimeBlockedByObservationUnknown  RuntimeMaterialBlockerReason = "observation_unknown"
	RuntimeBlockedByMigrationUnverified RuntimeMaterialBlockerReason = "migration_unverified"
	RuntimeBlockedByImagePruned         RuntimeMaterialBlockerReason = "image_pruned"
	RuntimeBlockedByActiveBuild         RuntimeMaterialBlockerReason = "active_build"
	RuntimeBlockedByActiveRetirement    RuntimeMaterialBlockerReason = "active_retirement"
)

type RuntimeMaterialBlocker struct {
	RuntimeID string                       `json:"runtime_id"`
	Revision  string                       `json:"revision"`
	Reason    RuntimeMaterialBlockerReason `json:"reason"`
}

func (b RuntimeMaterialBlocker) Validate() error {
	if err := ValidateRuntimeID(b.RuntimeID); err != nil {
		return err
	}
	if b.Revision == "" {
		if b.Reason != RuntimeBlockedByActiveRetirement && b.Reason != RuntimeBlockedByActiveBuild {
			return fmt.Errorf("Runtime material blocker revision is required")
		}
	} else if err := ValidateDigest(b.Revision); err != nil {
		return err
	}
	switch b.Reason {
	case RuntimeBlockedByWorkspaceContainer, RuntimeBlockedByExternalContainer, RuntimeBlockedByImageMissing,
		RuntimeBlockedByImageTagMissing, RuntimeBlockedByImageTagShared, RuntimeBlockedByImageMismatched,
		RuntimeBlockedByStagingMissing, RuntimeBlockedByStagingTagMissing, RuntimeBlockedByStagingTagShared,
		RuntimeBlockedByObservationUnknown, RuntimeBlockedByMigrationUnverified, RuntimeBlockedByImagePruned,
		RuntimeBlockedByActiveBuild, RuntimeBlockedByActiveRetirement:
		return nil
	default:
		return fmt.Errorf("Runtime material blocker reason is invalid")
	}
}

type RuntimePrunePlan struct {
	Task                  string                        `json:"task"`
	PlanRef               string                        `json:"plan_ref"`
	ObservedAt            time.Time                     `json:"observed_at"`
	Empty                 bool                          `json:"empty"`
	Applicable            bool                          `json:"applicable"`
	Candidates            []RuntimePruneCandidate       `json:"candidates"`
	Protected             []RuntimeProtection           `json:"protected"`
	Blockers              []RuntimeMaterialBlocker      `json:"blockers"`
	Storage               []RuntimeStorageObservation   `json:"storage"`
	RetirementGenerations []RuntimeRetirementGeneration `json:"-"`
}

type RuntimePruneResultState string

const (
	RuntimePruneApplied        RuntimePruneResultState = "applied"
	RuntimePruneAlreadyApplied RuntimePruneResultState = "already_applied"
	RuntimePruneEmpty          RuntimePruneResultState = "empty"
)

type RuntimePruneDisposition string

const (
	RuntimePruneRemoved         RuntimePruneDisposition = "removed"
	RuntimePruneAlreadyAbsent   RuntimePruneDisposition = "already_absent"
	RuntimePrunePreservedShared RuntimePruneDisposition = "preserved_shared"
)

type RuntimePruneItemResult struct {
	Kind                 RuntimePruneCandidateKind `json:"kind"`
	RuntimeID            string                    `json:"runtime_id"`
	Revision             string                    `json:"revision"`
	RuntimeRef           string                    `json:"runtime_ref"`
	RevisionRef          string                    `json:"revision_ref"`
	Name                 string                    `json:"name"`
	Ordinal              int                       `json:"ordinal"`
	LastUsed             RuntimeLastUsedState      `json:"last_used"`
	SourceLogicalBytes   int64                     `json:"source_logical_bytes"`
	SnapshotLogicalBytes int64                     `json:"snapshot_logical_bytes"`
	Disposition          RuntimePruneDisposition   `json:"disposition"`
	RemovedTagCount      int                       `json:"removed_tag_count"`
	ImageVirtualBytes    *int64                    `json:"image_virtual_bytes"`
	ReclaimedBytes       *int64                    `json:"reclaimed_bytes"`
}

func (i RuntimePruneItemResult) Validate() error {
	candidate := RuntimePruneCandidate{
		Kind: i.Kind, RuntimeID: i.RuntimeID, Revision: i.Revision, RuntimeRef: i.RuntimeRef,
		RevisionRef: i.RevisionRef, Name: i.Name, Ordinal: i.Ordinal, LastUsed: i.LastUsed,
		SourceLogicalBytes: i.SourceLogicalBytes, SnapshotLogicalBytes: i.SnapshotLogicalBytes, ImageVirtualBytes: i.ImageVirtualBytes,
	}
	if err := candidate.Validate(); err != nil {
		return err
	}
	if i.ReclaimedBytes != nil {
		return fmt.Errorf("V1 Runtime prune reclaimed bytes must remain unknown")
	}
	switch i.Disposition {
	case RuntimePruneAlreadyAbsent:
		if i.RemovedTagCount != 0 {
			return fmt.Errorf("already-absent Runtime prune result has removal evidence")
		}
	case RuntimePruneRemoved, RuntimePrunePreservedShared:
		if i.RemovedTagCount != 1 {
			return fmt.Errorf("Runtime prune result lacks exact tag removal")
		}
	default:
		return fmt.Errorf("Runtime prune disposition is invalid")
	}
	return nil
}

type RuntimePruneResult struct {
	Task               string                   `json:"task"`
	PlanRef            string                   `json:"plan_ref"`
	State              RuntimePruneResultState  `json:"state"`
	Items              []RuntimePruneItemResult `json:"items"`
	RemovedTagCount    int                      `json:"removed_tag_count"`
	ReclaimedBytes     *int64                   `json:"reclaimed_bytes"`
	ReceiptRevision    uint64                   `json:"receipt_revision"`
	SourcePreserved    bool                     `json:"source_preserved"`
	SnapshotsPreserved bool                     `json:"snapshots_preserved"`
	HistoryPreserved   bool                     `json:"history_preserved"`
}

func (r RuntimePruneResult) Validate() error {
	if r.Task != TaskRuntimePruneApply || ValidateRuntimePrunePlanRef(r.PlanRef) != nil || r.Items == nil || r.ReclaimedBytes != nil || !r.SourcePreserved || !r.SnapshotsPreserved || !r.HistoryPreserved {
		return fmt.Errorf("Runtime prune result is invalid")
	}
	wantTags := 0
	previous := ""
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		key := runtimePruneItemResultKey(item)
		if previous >= key {
			return fmt.Errorf("Runtime prune results are not unique canonical order")
		}
		previous = key
		wantTags += item.RemovedTagCount
	}
	if r.RemovedTagCount != wantTags {
		return fmt.Errorf("Runtime prune result totals are invalid")
	}
	switch r.State {
	case RuntimePruneEmpty:
		if len(r.Items) != 0 || r.ReceiptRevision != 0 || r.RemovedTagCount != 0 {
			return fmt.Errorf("empty Runtime prune result has mutation evidence")
		}
	case RuntimePruneApplied, RuntimePruneAlreadyApplied:
		if len(r.Items) == 0 || r.ReceiptRevision == 0 {
			return fmt.Errorf("applied Runtime prune result lacks receipt evidence")
		}
	default:
		return fmt.Errorf("Runtime prune result state is invalid")
	}
	return nil
}

func (p RuntimePrunePlan) Validate() error {
	if p.Task != TaskRuntimePruneDryRun || ValidateRuntimePrunePlanRef(p.PlanRef) != nil || p.ObservedAt.IsZero() || p.ObservedAt.Location() != time.UTC || p.Candidates == nil || p.Protected == nil || p.Blockers == nil || p.Storage == nil || p.Empty != (len(p.Candidates) == 0) || p.Applicable != runtimePrunePlanApplicable(p.Blockers) {
		return fmt.Errorf("Runtime prune plan is invalid")
	}
	candidateAuthorities := make(map[string]struct{}, len(p.Candidates))
	for index, candidate := range p.Candidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
		semantic := runtimeCandidateSemanticKey(candidate)
		if _, exists := candidateAuthorities[semantic]; exists {
			return fmt.Errorf("Runtime prune candidates duplicate semantic authority")
		}
		candidateAuthorities[semantic] = struct{}{}
		if index > 0 && runtimeCandidateAuthorityKey(p.Candidates[index-1]) >= runtimeCandidateAuthorityKey(candidate) {
			return fmt.Errorf("Runtime prune candidates are not unique canonical authority order")
		}
	}
	for index, protection := range p.Protected {
		if err := protection.Validate(); err != nil {
			return err
		}
		if index > 0 && runtimeProtectionAuthorityKey(p.Protected[index-1]) >= runtimeProtectionAuthorityKey(protection) {
			return fmt.Errorf("Runtime prune protections are not unique canonical authority order")
		}
	}
	for index, blocker := range p.Blockers {
		if err := blocker.Validate(); err != nil {
			return err
		}
		if index > 0 && runtimeMaterialBlockerKey(p.Blockers[index-1]) >= runtimeMaterialBlockerKey(blocker) {
			return fmt.Errorf("Runtime prune blockers are not unique canonical authority order")
		}
	}
	storage := make(map[string]RuntimeSnapshotStorage)
	sourceStorage := make(map[string]int64)
	for index, item := range p.Storage {
		if err := item.Validate(); err != nil {
			return err
		}
		if index > 0 && runtimeStorageObservationKey(p.Storage[index-1]) >= runtimeStorageObservationKey(item) {
			return fmt.Errorf("Runtime storage observations are not unique canonical order")
		}
		for _, snapshot := range item.Snapshots {
			storage[item.RuntimeID+"\x00"+runtimeSnapshotStorageKey(snapshot)] = snapshot
		}
		sourceStorage[item.RuntimeID] = item.SourceLogicalBytes
	}
	for index, generation := range p.RetirementGenerations {
		if err := generation.Validate(); err != nil {
			return err
		}
		if index > 0 && runtimeRetirementGenerationKey(p.RetirementGenerations[index-1]) >= runtimeRetirementGenerationKey(generation) {
			return fmt.Errorf("Runtime retirement generations are not unique canonical order")
		}
		if _, exists := storage[generation.RuntimeID+"\x00"+string(RuntimePruneCandidateRevision)+"\x00"+generation.Revision]; !exists {
			return fmt.Errorf("Runtime retirement generation lacks exact snapshot storage evidence")
		}
	}
	for _, candidate := range p.Candidates {
		key := candidate.RuntimeID + "\x00" + string(candidate.Kind) + "\x00" + candidate.Revision
		observed, exists := storage[key]
		if !exists || candidate.SnapshotLogicalBytes != observed.LogicalBytes || candidate.SourceLogicalBytes != sourceStorage[candidate.RuntimeID] {
			return fmt.Errorf("Runtime prune candidate lacks exact snapshot storage evidence")
		}
	}
	for _, protection := range p.Protected {
		if _, overlaps := candidateAuthorities[protection.RuntimeID+"\x00"+protection.RuntimeRevision]; overlaps {
			return fmt.Errorf("Runtime prune candidate overlaps protection evidence")
		}
	}
	for _, blocker := range p.Blockers {
		for authority := range candidateAuthorities {
			if authority == blocker.RuntimeID+"\x00"+blocker.Revision || (blocker.Revision == "" && strings.HasPrefix(authority, blocker.RuntimeID+"\x00")) {
				return fmt.Errorf("Runtime prune candidate overlaps blocker evidence")
			}
		}
	}
	want, err := runtimePrunePlanAuthorityRef(p.Candidates, p.Protected, p.Blockers, p.Storage, p.RetirementGenerations)
	if err != nil || p.PlanRef != want {
		return fmt.Errorf("Runtime prune plan reference does not match authority")
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
	storageByRuntime := make(map[string]RuntimeStorageObservation, len(snapshot.Storage))
	for _, storage := range snapshot.Storage {
		storageByRuntime[storage.RuntimeID] = storage
	}
	candidates := make([]RuntimePruneCandidate, 0)
	blockers := make([]RuntimeMaterialBlocker, 0)
	activeExact := make(map[string]bool)
	activeRuntime := make(map[string]bool)
	for _, activity := range snapshot.Journals.Active {
		reason := RuntimeBlockedByActiveRetirement
		if activity.Kind == RuntimeLifecycleActivityBuild {
			reason = RuntimeBlockedByActiveBuild
		}
		if activity.Kind == RuntimeLifecycleActivityDelete || len(activity.Revisions) == 0 {
			activeRuntime[activity.RuntimeID] = true
			blockers = append(blockers, RuntimeMaterialBlocker{RuntimeID: activity.RuntimeID, Reason: reason})
			continue
		}
		for _, revision := range activity.Revisions {
			activeExact[activity.RuntimeID+"\x00"+revision] = true
			blockers = append(blockers, RuntimeMaterialBlocker{RuntimeID: activity.RuntimeID, Revision: revision, Reason: reason})
		}
	}
	for _, material := range snapshot.Materials {
		blockers = append(blockers, runtimeMaterialBlockers(material)...)
		authority := material.RuntimeID + "\x00" + material.Revision
		if !runtimeMaterialPruneEligible(material) || protected[authority] || activeExact[authority] || activeRuntime[material.RuntimeID] {
			continue
		}
		runtime := byID[material.RuntimeID]
		if runtime.Kind != RuntimeKindManaged {
			continue
		}
		for _, revision := range runtime.Revisions {
			if revision.Revision == material.Revision {
				storage := storageByRuntime[runtime.ID]
				candidates = append(candidates, RuntimePruneCandidate{Kind: RuntimePruneCandidateRevision, RuntimeID: runtime.ID, Revision: revision.Revision, RuntimeRef: RuntimeRef(runtime.ID), RevisionRef: RuntimeRevisionRef(runtime.ID, revision.Revision), Name: runtime.Name, Ordinal: revision.Ordinal, LastUsed: RuntimeLastUsedUnknown, SourceLogicalBytes: storage.SourceLogicalBytes, SnapshotLogicalBytes: runtimeSnapshotLogicalBytes(storage, RuntimePruneCandidateRevision, revision.Revision), ImageVirtualBytes: material.ImageVirtualBytes})
				break
			}
		}
	}
	for _, artifact := range snapshot.Journals.FailedBuilds {
		blockers = append(blockers, runtimeMaterialBlockers(artifact.Material)...)
		authority := artifact.RuntimeID + "\x00" + artifact.Revision
		if !runtimeMaterialPruneEligible(artifact.Material) || protected[authority] || activeExact[authority] || activeRuntime[artifact.RuntimeID] {
			continue
		}
		storage := storageByRuntime[artifact.RuntimeID]
		candidates = append(candidates, RuntimePruneCandidate{Kind: RuntimePruneCandidateFailedBuild, RuntimeID: artifact.RuntimeID, Revision: artifact.Revision, RuntimeRef: artifact.RuntimeRef, Name: artifact.Name, LastUsed: RuntimeLastUsedUnknown, SourceLogicalBytes: storage.SourceLogicalBytes, SnapshotLogicalBytes: runtimeSnapshotLogicalBytes(storage, RuntimePruneCandidateFailedBuild, artifact.Revision), ImageVirtualBytes: artifact.Material.ImageVirtualBytes})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return runtimeCandidateAuthorityKey(candidates[i]) < runtimeCandidateAuthorityKey(candidates[j])
	})
	protectedItems := append([]RuntimeProtection{}, snapshot.Protection.Items...)
	sort.Slice(protectedItems, func(i, j int) bool {
		return runtimeProtectionAuthorityKey(protectedItems[i]) < runtimeProtectionAuthorityKey(protectedItems[j])
	})
	sort.Slice(blockers, func(i, j int) bool {
		return runtimeMaterialBlockerKey(blockers[i]) < runtimeMaterialBlockerKey(blockers[j])
	})
	generationAuthority := append([]RuntimeRetirementGeneration(nil), snapshot.RetirementGenerations...)
	planRef, err := runtimePrunePlanAuthorityRef(candidates, protectedItems, blockers, storageByRuntimeValues(storageByRuntime), generationAuthority)
	if err != nil {
		return RuntimePrunePlan{}, err
	}
	storage := append([]RuntimeStorageObservation{}, snapshot.Storage...)
	sort.Slice(storage, func(i, j int) bool {
		return runtimeStorageObservationKey(storage[i]) < runtimeStorageObservationKey(storage[j])
	})
	plan := RuntimePrunePlan{Task: TaskRuntimePruneDryRun, PlanRef: planRef, ObservedAt: observedAt, Empty: len(candidates) == 0, Applicable: runtimePrunePlanApplicable(blockers), Candidates: candidates, Protected: protectedItems, Blockers: blockers, Storage: storage, RetirementGenerations: generationAuthority}
	return plan, plan.Validate()
}

func runtimeMaterialBlockers(material RuntimeMaterialObservation) []RuntimeMaterialBlocker {
	result := make([]RuntimeMaterialBlocker, 0, 3)
	appendReason := func(reason RuntimeMaterialBlockerReason) {
		result = append(result, RuntimeMaterialBlocker{RuntimeID: material.RuntimeID, Revision: material.Revision, Reason: reason})
	}
	if material.WorkspaceInUse {
		appendReason(RuntimeBlockedByWorkspaceContainer)
	}
	if material.ExternalInUse {
		appendReason(RuntimeBlockedByExternalContainer)
	}
	switch material.Availability {
	case RuntimeAvailabilityMissing:
		if material.TagRole == RuntimeMaterialTagJournaledStaging {
			switch {
			case material.ContentPresent && material.SharedContent:
				appendReason(RuntimeBlockedByStagingTagShared)
			case material.ContentPresent:
				appendReason(RuntimeBlockedByStagingTagMissing)
			default:
				appendReason(RuntimeBlockedByStagingMissing)
			}
		} else {
			switch {
			case material.ContentPresent && material.SharedContent:
				appendReason(RuntimeBlockedByImageTagShared)
			case material.ContentPresent:
				appendReason(RuntimeBlockedByImageTagMissing)
			default:
				appendReason(RuntimeBlockedByImageMissing)
			}
		}
	case RuntimeAvailabilityMismatched:
		appendReason(RuntimeBlockedByImageMismatched)
	case RuntimeAvailabilityUnknown:
		if material.MigrationUnverified {
			appendReason(RuntimeBlockedByMigrationUnverified)
		} else {
			appendReason(RuntimeBlockedByObservationUnknown)
		}
	case RuntimeAvailabilityPruned:
		appendReason(RuntimeBlockedByImagePruned)
	}
	return result
}

func runtimeMaterialPruneEligible(material RuntimeMaterialObservation) bool {
	return material.Availability == RuntimeAvailabilityAvailable && !material.WorkspaceInUse && !material.ExternalInUse
}

func runtimePrunePlanApplicable(blockers []RuntimeMaterialBlocker) bool {
	for _, blocker := range blockers {
		switch blocker.Reason {
		case RuntimeBlockedByImageMismatched, RuntimeBlockedByObservationUnknown, RuntimeBlockedByMigrationUnverified,
			RuntimeBlockedByActiveBuild, RuntimeBlockedByActiveRetirement:
			return false
		}
	}
	return true
}

func runtimeCandidateAuthorityKey(candidate RuntimePruneCandidate) string {
	return candidate.RuntimeID + "\x00" + candidate.Revision + "\x00" + string(candidate.Kind)
}

func runtimeCandidateSemanticKey(candidate RuntimePruneCandidate) string {
	return candidate.RuntimeID + "\x00" + candidate.Revision
}

func runtimePruneItemResultKey(item RuntimePruneItemResult) string {
	return item.RuntimeID + "\x00" + item.Revision + "\x00" + string(item.Kind)
}

func runtimeProtectionAuthorityKey(item RuntimeProtection) string {
	return item.RuntimeID + "\x00" + item.RuntimeRevision + "\x00" + string(item.Reason) + "\x00" + item.WorkspaceManifestID + "\x00" + item.ManifestRevision + "\x00" + item.WorkspaceID
}

func runtimeMaterialBlockerKey(blocker RuntimeMaterialBlocker) string {
	return blocker.RuntimeID + "\x00" + blocker.Revision + "\x00" + string(blocker.Reason)
}

func runtimeLifecycleActivityKey(activity RuntimeLifecycleActivity) string {
	return activity.RuntimeID + "\x00" + string(activity.Kind) + "\x00" + strings.Join(activity.Revisions, "\x00")
}

func runtimeFailedBuildArtifactKey(artifact RuntimeFailedBuildArtifact) string {
	return artifact.RuntimeID + "\x00" + artifact.Revision
}

func runtimeSnapshotStorageKey(snapshot RuntimeSnapshotStorage) string {
	return string(snapshot.Kind) + "\x00" + snapshot.Revision
}

func runtimeStorageObservationKey(storage RuntimeStorageObservation) string {
	return storage.RuntimeID
}

func runtimeRetirementGenerationKey(generation RuntimeRetirementGeneration) string {
	return generation.RuntimeID + "\x00" + generation.Revision
}

func runtimeSnapshotLogicalBytes(storage RuntimeStorageObservation, kind RuntimePruneCandidateKind, revision string) int64 {
	for _, snapshot := range storage.Snapshots {
		if snapshot.Kind == kind && snapshot.Revision == revision {
			return snapshot.LogicalBytes
		}
	}
	return -1
}

func storageByRuntimeValues(byRuntime map[string]RuntimeStorageObservation) []RuntimeStorageObservation {
	result := make([]RuntimeStorageObservation, 0, len(byRuntime))
	for _, storage := range byRuntime {
		result = append(result, storage)
	}
	sort.Slice(result, func(i, j int) bool {
		return runtimeStorageObservationKey(result[i]) < runtimeStorageObservationKey(result[j])
	})
	return result
}

func runtimePrunePlanAuthorityRef(candidates []RuntimePruneCandidate, protected []RuntimeProtection, blockers []RuntimeMaterialBlocker, storage []RuntimeStorageObservation, generations []RuntimeRetirementGeneration) (string, error) {
	type candidateAuthority struct {
		Kind      RuntimePruneCandidateKind `json:"kind"`
		RuntimeID string                    `json:"runtime_id"`
		Revision  string                    `json:"revision"`
	}
	authorities := make([]candidateAuthority, len(candidates))
	for index, candidate := range candidates {
		authorities[index] = candidateAuthority{Kind: candidate.Kind, RuntimeID: candidate.RuntimeID, Revision: candidate.Revision}
	}
	type snapshotProof struct {
		RuntimeID   string                    `json:"runtime_id"`
		Kind        RuntimePruneCandidateKind `json:"kind"`
		Revision    string                    `json:"revision"`
		Fingerprint string                    `json:"fingerprint"`
	}
	proofs := make([]snapshotProof, 0)
	for _, runtimeStorage := range storage {
		for _, snapshot := range runtimeStorage.Snapshots {
			proofs = append(proofs, snapshotProof{RuntimeID: runtimeStorage.RuntimeID, Kind: snapshot.Kind, Revision: snapshot.Revision, Fingerprint: snapshot.SemanticFingerprint})
		}
	}
	sort.Slice(proofs, func(i, j int) bool {
		left := proofs[i].RuntimeID + "\x00" + string(proofs[i].Kind) + "\x00" + proofs[i].Revision + "\x00" + proofs[i].Fingerprint
		right := proofs[j].RuntimeID + "\x00" + string(proofs[j].Kind) + "\x00" + proofs[j].Revision + "\x00" + proofs[j].Fingerprint
		return left < right
	})
	generationAuthority := append([]RuntimeRetirementGeneration{}, generations...)
	for index, generation := range generationAuthority {
		if err := generation.Validate(); err != nil {
			return "", err
		}
		if index > 0 && runtimeRetirementGenerationKey(generationAuthority[index-1]) >= runtimeRetirementGenerationKey(generation) {
			return "", fmt.Errorf("Runtime retirement generations are not unique canonical order")
		}
	}
	canonical := struct {
		Schema      int                           `json:"schema"`
		Candidates  []candidateAuthority          `json:"candidates"`
		Protected   []RuntimeProtection           `json:"protected"`
		Blockers    []RuntimeMaterialBlocker      `json:"blockers"`
		Proofs      []snapshotProof               `json:"snapshot_proofs"`
		Generations []RuntimeRetirementGeneration `json:"retirement_generations"`
	}{Schema: 3, Candidates: authorities, Protected: protected, Blockers: blockers, Proofs: proofs, Generations: generationAuthority}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
