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
	TagPresent          bool                `json:"tag_present"`
	ContentPresent      bool                `json:"content_present"`
	SharedContent       bool                `json:"shared_content"`
	OwnershipVerified   bool                `json:"ownership_verified"`
	MigrationUnverified bool                `json:"migration_unverified"`
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
	if o.Availability == RuntimeAvailabilityAvailable && (!o.TagPresent || !o.ContentPresent || !o.OwnershipVerified) {
		return fmt.Errorf("available Runtime material lacks ownership evidence")
	}
	if o.Availability == RuntimeAvailabilityMissing && o.TagPresent {
		return fmt.Errorf("missing Runtime material cannot have its normal tag")
	}
	if o.Availability == RuntimeAvailabilityMismatched && !o.TagPresent {
		return fmt.Errorf("mismatched Runtime material requires an observed normal tag")
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
	Journals        RuntimeLifecycleJournals     `json:"journals"`
}

func (s RuntimeLifecycleSnapshot) Validate() error {
	if !s.CatalogComplete || s.Runtimes == nil || s.Materials == nil {
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
	failedBuilds := make(map[string]struct{}, len(s.Journals.FailedBuilds))
	for _, artifact := range s.Journals.FailedBuilds {
		failedBuilds[artifact.RuntimeID+"\x00"+artifact.Revision] = struct{}{}
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
				if !successful && !(activity.Kind == RuntimeLifecycleActivityPrune && failed) {
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
	case RuntimeLifecycleActivityBuild, RuntimeLifecycleActivityRestore:
		if len(a.Revisions) != 1 {
			return fmt.Errorf("Runtime build or restore activity requires one semantic revision")
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
	return a.Material.Validate()
}

type RuntimeLifecycleJournals struct {
	Complete     bool                         `json:"complete"`
	Active       []RuntimeLifecycleActivity   `json:"active"`
	FailedBuilds []RuntimeFailedBuildArtifact `json:"failed_builds"`
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
	Kind              RuntimePruneCandidateKind `json:"kind"`
	RuntimeID         string                    `json:"runtime_id"`
	Revision          string                    `json:"revision"`
	RuntimeRef        string                    `json:"runtime_ref"`
	RevisionRef       string                    `json:"revision_ref"`
	Name              string                    `json:"name"`
	Ordinal           int                       `json:"ordinal"`
	ImageVirtualBytes *int64                    `json:"image_virtual_bytes"`
}

type RuntimePruneCandidateKind string

const (
	RuntimePruneCandidateRevision    RuntimePruneCandidateKind = "runtime_revision"
	RuntimePruneCandidateFailedBuild RuntimePruneCandidateKind = "failed_build"
)

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
	if c.ImageVirtualBytes != nil && *c.ImageVirtualBytes < 0 {
		return fmt.Errorf("Runtime prune candidate image bytes cannot be negative")
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
		if b.Reason != RuntimeBlockedByActiveRetirement {
			return fmt.Errorf("Runtime material blocker revision is required")
		}
	} else if err := ValidateDigest(b.Revision); err != nil {
		return err
	}
	switch b.Reason {
	case RuntimeBlockedByWorkspaceContainer, RuntimeBlockedByExternalContainer, RuntimeBlockedByImageMissing,
		RuntimeBlockedByImageTagMissing, RuntimeBlockedByImageTagShared, RuntimeBlockedByImageMismatched,
		RuntimeBlockedByObservationUnknown, RuntimeBlockedByMigrationUnverified, RuntimeBlockedByImagePruned,
		RuntimeBlockedByActiveBuild, RuntimeBlockedByActiveRetirement:
		return nil
	default:
		return fmt.Errorf("Runtime material blocker reason is invalid")
	}
}

type RuntimePrunePlan struct {
	Task       string                   `json:"task"`
	PlanRef    string                   `json:"plan_ref"`
	ObservedAt time.Time                `json:"observed_at"`
	Empty      bool                     `json:"empty"`
	Applicable bool                     `json:"applicable"`
	Candidates []RuntimePruneCandidate  `json:"candidates"`
	Protected  []RuntimeProtection      `json:"protected"`
	Blockers   []RuntimeMaterialBlocker `json:"blockers"`
}

func (p RuntimePrunePlan) Validate() error {
	if p.Task != TaskRuntimePruneDryRun || ValidateRuntimePrunePlanRef(p.PlanRef) != nil || p.ObservedAt.IsZero() || p.ObservedAt.Location() != time.UTC || p.Candidates == nil || p.Protected == nil || p.Blockers == nil || p.Empty != (len(p.Candidates) == 0) || p.Applicable != runtimePrunePlanApplicable(p.Blockers) {
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
	want, err := runtimePrunePlanAuthorityRef(p.Candidates, p.Protected, p.Blockers)
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
	candidates := make([]RuntimePruneCandidate, 0)
	blockers := make([]RuntimeMaterialBlocker, 0)
	activeExact := make(map[string]bool)
	activeRuntime := make(map[string]bool)
	for _, activity := range snapshot.Journals.Active {
		reason := RuntimeBlockedByActiveRetirement
		if activity.Kind == RuntimeLifecycleActivityBuild {
			reason = RuntimeBlockedByActiveBuild
		}
		if activity.Kind == RuntimeLifecycleActivityDelete {
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
				candidates = append(candidates, RuntimePruneCandidate{Kind: RuntimePruneCandidateRevision, RuntimeID: runtime.ID, Revision: revision.Revision, RuntimeRef: RuntimeRef(runtime.ID), RevisionRef: RuntimeRevisionRef(runtime.ID, revision.Revision), Name: runtime.Name, Ordinal: revision.Ordinal, ImageVirtualBytes: material.ImageVirtualBytes})
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
		candidates = append(candidates, RuntimePruneCandidate{Kind: RuntimePruneCandidateFailedBuild, RuntimeID: artifact.RuntimeID, Revision: artifact.Revision, RuntimeRef: artifact.RuntimeRef, Name: artifact.Name, ImageVirtualBytes: artifact.Material.ImageVirtualBytes})
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
	planRef, err := runtimePrunePlanAuthorityRef(candidates, protectedItems, blockers)
	if err != nil {
		return RuntimePrunePlan{}, err
	}
	plan := RuntimePrunePlan{Task: TaskRuntimePruneDryRun, PlanRef: planRef, ObservedAt: observedAt, Empty: len(candidates) == 0, Applicable: runtimePrunePlanApplicable(blockers), Candidates: candidates, Protected: protectedItems, Blockers: blockers}
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
		switch {
		case material.ContentPresent && material.SharedContent:
			appendReason(RuntimeBlockedByImageTagShared)
		case material.ContentPresent:
			appendReason(RuntimeBlockedByImageTagMissing)
		default:
			appendReason(RuntimeBlockedByImageMissing)
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

func runtimePrunePlanAuthorityRef(candidates []RuntimePruneCandidate, protected []RuntimeProtection, blockers []RuntimeMaterialBlocker) (string, error) {
	type candidateAuthority struct {
		Kind      RuntimePruneCandidateKind `json:"kind"`
		RuntimeID string                    `json:"runtime_id"`
		Revision  string                    `json:"revision"`
	}
	authorities := make([]candidateAuthority, len(candidates))
	for index, candidate := range candidates {
		authorities[index] = candidateAuthority{Kind: candidate.Kind, RuntimeID: candidate.RuntimeID, Revision: candidate.Revision}
	}
	canonical := struct {
		Schema     int                      `json:"schema"`
		Candidates []candidateAuthority     `json:"candidates"`
		Protected  []RuntimeProtection      `json:"protected"`
		Blockers   []RuntimeMaterialBlocker `json:"blockers"`
	}{Schema: 1, Candidates: authorities, Protected: protected, Blockers: blockers}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
