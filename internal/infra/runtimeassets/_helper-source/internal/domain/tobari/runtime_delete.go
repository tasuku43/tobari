package tobari

import (
	"fmt"
	"sort"
)

// RuntimeDeleteMaterialTarget is one exact successful revision or settled
// failed-build material observation owned by a whole-Runtime deletion. Docker
// selectors and content IDs remain infrastructure evidence.
type RuntimeDeleteMaterialTarget struct {
	Candidate      RuntimePruneCandidate `json:"candidate"`
	Availability   RuntimeAvailability   `json:"availability"`
	TagPresent     bool                  `json:"tag_present"`
	SharedContent  bool                  `json:"shared_content"`
	ContentPresent bool                  `json:"content_present"`
}

func (t RuntimeDeleteMaterialTarget) Validate() error {
	if err := t.Candidate.Validate(); err != nil {
		return err
	}
	if err := t.Availability.Validate(); err != nil {
		return err
	}
	switch t.Availability {
	case RuntimeAvailabilityAvailable:
		if !t.TagPresent || !t.ContentPresent {
			return fmt.Errorf("available Runtime delete material lacks exact tag content")
		}
	case RuntimeAvailabilityMissing, RuntimeAvailabilityPruned:
		if t.TagPresent {
			return fmt.Errorf("absent Runtime delete material retains its expected tag")
		}
	default:
		return fmt.Errorf("Runtime delete material availability is not actionable")
	}
	if t.SharedContent && !t.ContentPresent {
		return fmt.Errorf("shared Runtime delete content is absent")
	}
	return nil
}

// RuntimeDeleteTarget is the complete semantic authority captured before one
// whole managed Runtime is retired. It includes every successful revision and
// settled failed-build candidate, but no mutable path or Docker selector is a
// public action identity.
type RuntimeDeleteTarget struct {
	Runtime   RuntimeManifest               `json:"runtime"`
	Storage   RuntimeStorageObservation     `json:"storage"`
	Materials []RuntimeDeleteMaterialTarget `json:"materials"`
}

func (t RuntimeDeleteTarget) Validate() error {
	if err := t.Runtime.Validate(); err != nil {
		return err
	}
	if t.Runtime.Kind != RuntimeKindManaged || t.Runtime.RuntimeRef != RuntimeRef(t.Runtime.ID) {
		return fmt.Errorf("Runtime delete target is not exact managed authority")
	}
	if err := t.Storage.Validate(); err != nil {
		return err
	}
	if t.Storage.RuntimeID != t.Runtime.ID || t.Storage.Name != t.Runtime.Name || t.Materials == nil {
		return fmt.Errorf("Runtime delete storage authority is invalid")
	}
	wantStorage := make(map[string]RuntimeSnapshotStorage, len(t.Storage.Snapshots))
	for _, snapshot := range t.Storage.Snapshots {
		wantStorage[runtimeSnapshotStorageKey(snapshot)] = snapshot
	}
	wantSuccessful := make(map[string]RuntimeRevision, len(t.Runtime.Revisions))
	for _, revision := range t.Runtime.Revisions {
		if revision.RuntimeRef != RuntimeRef(t.Runtime.ID) || revision.RevisionRef != RuntimeRevisionRef(t.Runtime.ID, revision.Revision) {
			return fmt.Errorf("Runtime delete revision authority is invalid")
		}
		wantSuccessful[revision.Revision] = revision
	}
	seen := make(map[string]struct{}, len(t.Materials))
	for index, material := range t.Materials {
		if err := material.Validate(); err != nil {
			return err
		}
		candidate := material.Candidate
		if candidate.RuntimeID != t.Runtime.ID || candidate.RuntimeRef != RuntimeRef(t.Runtime.ID) || candidate.Name != t.Runtime.Name ||
			candidate.SourceLogicalBytes != t.Storage.SourceLogicalBytes {
			return fmt.Errorf("Runtime delete material owner authority is invalid")
		}
		key := string(candidate.Kind) + "\x00" + candidate.Revision
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("Runtime delete materials duplicate semantic authority")
		}
		seen[key] = struct{}{}
		stored, exists := wantStorage[key]
		if !exists || stored.LogicalBytes != candidate.SnapshotLogicalBytes {
			return fmt.Errorf("Runtime delete material lacks exact snapshot storage")
		}
		delete(wantStorage, key)
		switch candidate.Kind {
		case RuntimePruneCandidateRevision:
			revision, exists := wantSuccessful[candidate.Revision]
			if !exists || candidate.Ordinal != revision.Ordinal || candidate.RevisionRef != revision.RevisionRef {
				return fmt.Errorf("Runtime delete successful revision authority is invalid")
			}
			delete(wantSuccessful, candidate.Revision)
		case RuntimePruneCandidateFailedBuild:
			if _, overlaps := wantSuccessful[candidate.Revision]; overlaps {
				return fmt.Errorf("Runtime delete failed material overlaps successful history")
			}
		}
		if index > 0 && runtimeDeleteMaterialKey(t.Materials[index-1]) >= runtimeDeleteMaterialKey(material) {
			return fmt.Errorf("Runtime delete materials are not unique canonical order")
		}
	}
	if len(wantStorage) != 0 || len(wantSuccessful) != 0 {
		return fmt.Errorf("Runtime delete material inventory is incomplete")
	}
	return nil
}

// RuntimeDeleteTargetFrom binds one opaque Runtime reference to one complete
// fail-closed lifecycle observation. Protection and current use block the
// whole Runtime; missing/pruned image tags remain truthful no-effect material.
func RuntimeDeleteTargetFrom(snapshot RuntimeLifecycleSnapshot, runtimeRef string) (RuntimeDeleteTarget, error) {
	if err := snapshot.Validate(); err != nil {
		return RuntimeDeleteTarget{}, fmt.Errorf("%w: %w", ErrRuntimeRetirementObservationUnknown, err)
	}
	if err := ValidateRuntimeRef(runtimeRef); err != nil {
		return RuntimeDeleteTarget{}, err
	}
	var runtime RuntimeManifest
	found := false
	for _, candidate := range snapshot.Runtimes {
		if RuntimeRef(candidate.ID) == runtimeRef {
			runtime, found = candidate, true
			break
		}
	}
	if !found {
		return RuntimeDeleteTarget{}, ErrRuntimeNotFound
	}
	if runtime.Kind != RuntimeKindManaged || runtime.ID == StandardRuntimeID {
		return RuntimeDeleteTarget{}, ErrRuntimeDeleteProtected
	}
	for _, protection := range snapshot.Protection.Items {
		if protection.RuntimeID == runtime.ID {
			return RuntimeDeleteTarget{}, ErrRuntimeDeleteProtected
		}
	}
	for _, activity := range snapshot.Journals.Active {
		if activity.RuntimeID == runtime.ID {
			return RuntimeDeleteTarget{}, ErrRuntimeLifecycleActive
		}
	}
	var storage RuntimeStorageObservation
	foundStorage := false
	for _, candidate := range snapshot.Storage {
		if candidate.RuntimeID == runtime.ID {
			storage, foundStorage = candidate, true
			break
		}
	}
	if !foundStorage {
		return RuntimeDeleteTarget{}, fmt.Errorf("%w: Runtime storage is absent", ErrRuntimeRetirementObservationUnknown)
	}
	storageByAuthority := make(map[string]RuntimeSnapshotStorage, len(storage.Snapshots))
	for _, item := range storage.Snapshots {
		storageByAuthority[runtimeSnapshotStorageKey(item)] = item
	}

	manifest := runtime
	manifest.Revisions = make([]RuntimeRevision, len(runtime.Revisions))
	copy(manifest.Revisions, runtime.Revisions)
	manifest.RuntimeRef = RuntimeRef(runtime.ID)
	for index := range manifest.Revisions {
		manifest.Revisions[index].RuntimeRef = RuntimeRef(runtime.ID)
		manifest.Revisions[index].RevisionRef = RuntimeRevisionRef(runtime.ID, manifest.Revisions[index].Revision)
	}
	materials := make([]RuntimeDeleteMaterialTarget, 0, len(runtime.Revisions)+len(snapshot.Journals.FailedBuilds))
	materialByRevision := make(map[string]RuntimeMaterialObservation, len(snapshot.Materials))
	for _, material := range snapshot.Materials {
		if material.RuntimeID == runtime.ID {
			materialByRevision[material.Revision] = material
		}
	}
	for _, revision := range manifest.Revisions {
		material, exists := materialByRevision[revision.Revision]
		if !exists {
			return RuntimeDeleteTarget{}, fmt.Errorf("%w: Runtime material is absent", ErrRuntimeRetirementObservationUnknown)
		}
		candidate := RuntimePruneCandidate{
			Kind: RuntimePruneCandidateRevision, RuntimeID: runtime.ID, Revision: revision.Revision,
			RuntimeRef: RuntimeRef(runtime.ID), RevisionRef: revision.RevisionRef, Name: runtime.Name, Ordinal: revision.Ordinal,
			LastUsed: RuntimeLastUsedUnknown, SourceLogicalBytes: storage.SourceLogicalBytes,
			SnapshotLogicalBytes: storageByAuthority[string(RuntimePruneCandidateRevision)+"\x00"+revision.Revision].LogicalBytes,
			ImageVirtualBytes:    material.ImageVirtualBytes,
		}
		target, err := runtimeDeleteMaterialTarget(candidate, material)
		if err != nil {
			return RuntimeDeleteTarget{}, err
		}
		materials = append(materials, target)
	}
	for _, artifact := range snapshot.Journals.FailedBuilds {
		if artifact.RuntimeID != runtime.ID {
			continue
		}
		candidate := RuntimePruneCandidate{
			Kind: RuntimePruneCandidateFailedBuild, RuntimeID: runtime.ID, Revision: artifact.Revision,
			RuntimeRef: RuntimeRef(runtime.ID), Name: runtime.Name, LastUsed: RuntimeLastUsedUnknown,
			SourceLogicalBytes:   storage.SourceLogicalBytes,
			SnapshotLogicalBytes: storageByAuthority[string(RuntimePruneCandidateFailedBuild)+"\x00"+artifact.Revision].LogicalBytes,
			ImageVirtualBytes:    artifact.Material.ImageVirtualBytes,
		}
		target, err := runtimeDeleteMaterialTarget(candidate, artifact.Material)
		if err != nil {
			return RuntimeDeleteTarget{}, err
		}
		materials = append(materials, target)
	}
	sort.Slice(materials, func(i, j int) bool {
		return runtimeDeleteMaterialKey(materials[i]) < runtimeDeleteMaterialKey(materials[j])
	})
	target := RuntimeDeleteTarget{Runtime: manifest, Storage: storage, Materials: materials}
	if err := target.Validate(); err != nil {
		return RuntimeDeleteTarget{}, fmt.Errorf("%w: %w", ErrRuntimeRetirementObservationUnknown, err)
	}
	return target, nil
}

func runtimeDeleteMaterialTarget(candidate RuntimePruneCandidate, material RuntimeMaterialObservation) (RuntimeDeleteMaterialTarget, error) {
	if material.WorkspaceInUse || material.ExternalInUse {
		return RuntimeDeleteMaterialTarget{}, ErrRuntimeDeleteProtected
	}
	switch material.Availability {
	case RuntimeAvailabilityAvailable, RuntimeAvailabilityMissing, RuntimeAvailabilityPruned:
	case RuntimeAvailabilityMismatched, RuntimeAvailabilityUnknown:
		return RuntimeDeleteMaterialTarget{}, ErrRuntimeRetirementObservationUnknown
	default:
		return RuntimeDeleteMaterialTarget{}, ErrRuntimeRetirementObservationUnknown
	}
	target := RuntimeDeleteMaterialTarget{
		Candidate: candidate, Availability: material.Availability, TagPresent: material.TagPresent,
		SharedContent: material.SharedContent, ContentPresent: material.ContentPresent,
	}
	if err := target.Validate(); err != nil {
		return RuntimeDeleteMaterialTarget{}, fmt.Errorf("%w: %w", ErrRuntimeRetirementObservationUnknown, err)
	}
	return target, nil
}

type RuntimeDeleteState string

const (
	RuntimeDeleted        RuntimeDeleteState = "deleted"
	RuntimeAlreadyDeleted RuntimeDeleteState = "already_deleted"
)

type RuntimeDeleteAuthorityDisposition string

const RuntimeDeleteAuthorityRemoved RuntimeDeleteAuthorityDisposition = "removed"

// RuntimeDeleteResult confirms whole-Runtime retirement while stating every
// higher- and lower-lifetime resource class that deletion must preserve.
type RuntimeDeleteResult struct {
	Task                        string                            `json:"task"`
	RuntimeID                   string                            `json:"runtime_id"`
	RuntimeRef                  string                            `json:"runtime_ref"`
	Name                        string                            `json:"name"`
	State                       RuntimeDeleteState                `json:"state"`
	SourceLogicalBytes          int64                             `json:"source_logical_bytes"`
	SnapshotLogicalBytes        int64                             `json:"snapshot_logical_bytes"`
	SourceDisposition           RuntimeDeleteAuthorityDisposition `json:"source_disposition"`
	SnapshotsDisposition        RuntimeDeleteAuthorityDisposition `json:"snapshots_disposition"`
	HistoryDisposition          RuntimeDeleteAuthorityDisposition `json:"history_disposition"`
	Items                       []RuntimePruneItemResult          `json:"items"`
	RemovedTagCount             int                               `json:"removed_tag_count"`
	ReclaimedBytes              *int64                            `json:"reclaimed_bytes"`
	ReceiptRevision             uint64                            `json:"receipt_revision"`
	WorkspaceManifestsPreserved bool                              `json:"workspace_manifests_preserved"`
	WorkspacesPreserved         bool                              `json:"workspaces_preserved"`
	WorkspaceIDsPreserved       bool                              `json:"workspace_ids_preserved"`
	WorkspaceHomesPreserved     bool                              `json:"workspace_homes_preserved"`
	AppliedReceiptsPreserved    bool                              `json:"applied_receipts_preserved"`
	ProjectRootsPreserved       bool                              `json:"project_roots_preserved"`
	CredentialsPreserved        bool                              `json:"credentials_preserved"`
	SharedResourcesPreserved    bool                              `json:"shared_resources_preserved"`
}

func (r RuntimeDeleteResult) Validate() error {
	if r.Task != TaskRuntimeDelete || ValidateRuntimeID(r.RuntimeID) != nil || r.RuntimeRef != RuntimeRef(r.RuntimeID) || ValidateName(r.Name) != nil ||
		r.SourceLogicalBytes < 0 || r.SnapshotLogicalBytes < 0 || r.SourceDisposition != RuntimeDeleteAuthorityRemoved ||
		r.SnapshotsDisposition != RuntimeDeleteAuthorityRemoved || r.HistoryDisposition != RuntimeDeleteAuthorityRemoved ||
		r.Items == nil || r.ReclaimedBytes != nil || r.ReceiptRevision == 0 || !r.WorkspaceManifestsPreserved || !r.WorkspacesPreserved ||
		!r.WorkspaceIDsPreserved || !r.WorkspaceHomesPreserved || !r.AppliedReceiptsPreserved || !r.ProjectRootsPreserved ||
		!r.CredentialsPreserved || !r.SharedResourcesPreserved {
		return fmt.Errorf("Runtime delete result authority is invalid")
	}
	wantTags := 0
	wantSnapshotBytes := int64(0)
	previous := ""
	for _, item := range r.Items {
		if err := item.Validate(); err != nil || item.RuntimeID != r.RuntimeID || item.RuntimeRef != r.RuntimeRef || item.Name != r.Name || item.SourceLogicalBytes != r.SourceLogicalBytes {
			return fmt.Errorf("Runtime delete material result is invalid")
		}
		key := runtimePruneItemResultKey(item)
		if previous >= key {
			return fmt.Errorf("Runtime delete material results are not unique canonical order")
		}
		previous = key
		wantTags += item.RemovedTagCount
		wantSnapshotBytes += item.SnapshotLogicalBytes
		if wantSnapshotBytes < 0 {
			return fmt.Errorf("Runtime delete snapshot byte total overflowed")
		}
	}
	if r.RemovedTagCount != wantTags || r.SnapshotLogicalBytes != wantSnapshotBytes {
		return fmt.Errorf("Runtime delete result totals are invalid")
	}
	if r.State != RuntimeDeleted && r.State != RuntimeAlreadyDeleted {
		return fmt.Errorf("Runtime delete result state is invalid")
	}
	return nil
}

func runtimeDeleteMaterialKey(target RuntimeDeleteMaterialTarget) string {
	return runtimeCandidateAuthorityKey(target.Candidate)
}
