package tobari

import (
	"strings"
	"testing"
	"time"
)

func lifecycleRuntime(id, name string, revisions ...string) RuntimeManifest {
	items := make([]RuntimeRevision, 0, len(revisions))
	for index, revision := range revisions {
		items = append(items, RuntimeRevision{Ordinal: index + 1, Revision: revision, Image: "tobari-runtime-" + name + ":test", ImageDigest: "sha256:" + strings.Repeat(string(rune('b'+index)), 64), CreatedAt: time.Unix(int64(index+1), 0).UTC(), SnapshotPath: "/tmp/tobari/runtimes/" + name + "/revisions/" + revision[7:19] + "/source"})
	}
	return RuntimeManifest{SchemaVersion: RuntimeSchemaVersion, ID: id, Name: name, Kind: RuntimeKindManaged, SourcePath: "/tmp/tobari/runtimes/" + name + "/source", Revisions: items}
}

func lifecycleStandard() RuntimeManifest {
	return RuntimeManifest{
		SchemaVersion: RuntimeSchemaVersion,
		ID:            StandardRuntimeID,
		Name:          StandardRuntimeName,
		Kind:          RuntimeKindBuiltin,
		Revisions: []RuntimeRevision{{
			Ordinal:   1,
			Revision:  "sha256:" + strings.Repeat("f", 64),
			Image:     OfficialRuntimeBase,
			CreatedAt: time.Unix(1, 0).UTC(),
		}},
	}
}

func lifecycleSnapshot(runtimes []RuntimeManifest, protection []RuntimeProtection, materials []RuntimeMaterialObservation) RuntimeLifecycleSnapshot {
	materialCopy := append([]RuntimeMaterialObservation{}, materials...)
	for index := range materialCopy {
		if materialCopy[index].TagRole == "" {
			materialCopy[index].TagRole = RuntimeMaterialTagPublishedRevision
		}
	}
	storage := make([]RuntimeStorageObservation, 0, len(runtimes))
	for _, runtime := range runtimes {
		snapshots := make([]RuntimeSnapshotStorage, 0, len(runtime.Revisions))
		for _, revision := range runtime.Revisions {
			snapshots = append(snapshots, RuntimeSnapshotStorage{Kind: RuntimePruneCandidateRevision, Revision: revision.Revision, SemanticFingerprint: revision.Revision, LogicalBytes: int64(revision.Ordinal * 100)})
		}
		storage = append(storage, RuntimeStorageObservation{RuntimeID: runtime.ID, Name: runtime.Name, SourceLogicalBytes: 42, Snapshots: snapshots})
	}
	return RuntimeLifecycleSnapshot{
		CatalogComplete: true,
		Runtimes:        append([]RuntimeManifest{lifecycleStandard()}, runtimes...),
		Protection:      RuntimeProtectionInventory{Complete: true, Items: protection},
		Materials:       materialCopy,
		Storage:         storage,
		Journals:        RuntimeLifecycleJournals{Complete: true, Active: []RuntimeLifecycleActivity{}, FailedBuilds: []RuntimeFailedBuildArtifact{}},
	}
}

func TestRuntimeLifecycleSnapshotRequiresCompleteCatalogAndExactStandard(t *testing.T) {
	valid := lifecycleSnapshot([]RuntimeManifest{}, []RuntimeProtection{}, []RuntimeMaterialObservation{})
	if err := valid.Validate(); err != nil {
		t.Fatalf("complete standard-only catalog: %v", err)
	}
	plan, err := PlanRuntimePrune(valid, time.Unix(10, 0).UTC())
	if err != nil || !plan.Empty || len(plan.Candidates) != 0 || len(plan.Protected) != 0 || len(plan.Blockers) != 0 {
		t.Fatalf("standard-only plan = %+v/%v", plan, err)
	}

	tests := map[string]func(*RuntimeLifecycleSnapshot){
		"assertion absent": func(snapshot *RuntimeLifecycleSnapshot) { snapshot.CatalogComplete = false },
		"nil runtimes":     func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Runtimes = nil },
		"nil materials":    func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Materials = nil },
		"nil storage":      func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Storage = nil },
		"standard absent":  func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Runtimes = []RuntimeManifest{} },
		"standard repeated": func(snapshot *RuntimeLifecycleSnapshot) {
			snapshot.Runtimes = append(snapshot.Runtimes, lifecycleStandard())
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := valid
			snapshot.Runtimes = append([]RuntimeManifest{}, valid.Runtimes...)
			mutate(&snapshot)
			if err := snapshot.Validate(); err == nil {
				t.Fatalf("incomplete catalog validated: %+v", snapshot)
			}
		})
	}
}

func TestPlanRuntimePruneIsDeterministicAndExcludesProtectedMaterial(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	first := "sha256:" + strings.Repeat("a", 64)
	second := "sha256:" + strings.Repeat("c", 64)
	bytes := int64(2048)
	manifest := lifecycleRuntime(id, "frontend", first, second)
	protection := RuntimeProtection{RuntimeID: id, RuntimeRevision: first, Reason: RuntimeProtectedByManifestCurrent, WorkspaceManifestID: "018bcfe5-687b-7000-8000-000000000088", ManifestRevision: "sha256:" + strings.Repeat("d", 64)}
	snapshot := lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{protection}, []RuntimeMaterialObservation{
		{RuntimeID: id, Revision: second, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true, ImageVirtualBytes: &bytes},
		{RuntimeID: id, Revision: first, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true},
	})
	planA, err := PlanRuntimePrune(snapshot, time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	planB, err := PlanRuntimePrune(snapshot, time.Unix(20, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if planA.PlanRef != planB.PlanRef || planA.ObservedAt == planB.ObservedAt {
		t.Fatalf("plan identity/time = %q/%q %v/%v", planA.PlanRef, planB.PlanRef, planA.ObservedAt, planB.ObservedAt)
	}
	if planA.Empty || len(planA.Candidates) != 1 || planA.Candidates[0].Revision != second || len(planA.Protected) != 1 || planA.Protected[0] != protection || len(planA.Blockers) != 0 {
		t.Fatalf("prune plan = %+v", planA)
	}
	if planA.Candidates[0].RevisionRef != RuntimeRevisionRef(id, second) || planA.Candidates[0].ImageVirtualBytes == nil || *planA.Candidates[0].ImageVirtualBytes != bytes {
		t.Fatalf("candidate authority/evidence = %+v", planA.Candidates[0])
	}
}

func TestPlanRuntimePruneEmitsEveryMaterialBlockerDeterministically(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revisions := []string{
		"sha256:" + strings.Repeat("a", 64),
		"sha256:" + strings.Repeat("b", 64),
		"sha256:" + strings.Repeat("c", 64),
		"sha256:" + strings.Repeat("d", 64),
		"sha256:" + strings.Repeat("e", 64),
	}
	snapshot := lifecycleSnapshot([]RuntimeManifest{lifecycleRuntime(id, "frontend", revisions...)}, []RuntimeProtection{}, []RuntimeMaterialObservation{
		{RuntimeID: id, Revision: revisions[4], Availability: RuntimeAvailabilityPruned, ObservationComplete: true},
		{RuntimeID: id, Revision: revisions[3], Availability: RuntimeAvailabilityUnknown, ObservationComplete: true},
		{RuntimeID: id, Revision: revisions[2], Availability: RuntimeAvailabilityMismatched, TagPresent: true, ObservationComplete: true},
		{RuntimeID: id, Revision: revisions[1], Availability: RuntimeAvailabilityMissing, ObservationComplete: true},
		{RuntimeID: id, Revision: revisions[0], Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true, WorkspaceInUse: true, ExternalInUse: true},
	})
	plan, err := PlanRuntimePrune(snapshot, time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	want := []RuntimeMaterialBlocker{
		{RuntimeID: id, Revision: revisions[0], Reason: RuntimeBlockedByExternalContainer},
		{RuntimeID: id, Revision: revisions[0], Reason: RuntimeBlockedByWorkspaceContainer},
		{RuntimeID: id, Revision: revisions[1], Reason: RuntimeBlockedByImageMissing},
		{RuntimeID: id, Revision: revisions[2], Reason: RuntimeBlockedByImageMismatched},
		{RuntimeID: id, Revision: revisions[3], Reason: RuntimeBlockedByObservationUnknown},
		{RuntimeID: id, Revision: revisions[4], Reason: RuntimeBlockedByImagePruned},
	}
	if !plan.Empty || len(plan.Candidates) != 0 || len(plan.Blockers) != len(want) {
		t.Fatalf("blocked plan = %+v", plan)
	}
	for index := range want {
		if plan.Blockers[index] != want[index] {
			t.Fatalf("blocker[%d] = %+v, want %+v", index, plan.Blockers[index], want[index])
		}
	}
}

func TestRuntimePrunePlanIdentityIgnoresPresentationAndEstimates(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	bytes := int64(2048)
	snapshot := lifecycleSnapshot([]RuntimeManifest{lifecycleRuntime(id, "frontend", revision)}, []RuntimeProtection{}, []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true, ImageVirtualBytes: &bytes}})
	plan, err := PlanRuntimePrune(snapshot, time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}

	changed := plan
	changed.Candidates = append([]RuntimePruneCandidate{}, plan.Candidates...)
	otherBytes := int64(8192)
	changed.Candidates[0].Name = "renamed"
	changed.Candidates[0].Ordinal = 99
	changed.Candidates[0].ImageVirtualBytes = &otherBytes
	changed.Candidates[0].SourceLogicalBytes = 4096
	changed.Candidates[0].SnapshotLogicalBytes = 8192
	changed.Storage = append([]RuntimeStorageObservation{}, plan.Storage...)
	changed.Storage[0].SourceLogicalBytes = 4096
	changed.Storage[0].Snapshots = append([]RuntimeSnapshotStorage{}, plan.Storage[0].Snapshots...)
	changed.Storage[0].Snapshots[0].LogicalBytes = 8192
	changed.ObservedAt = time.Unix(20, 0).UTC()
	if err := changed.Validate(); err != nil {
		t.Fatalf("presentation-only change invalidated plan: %v", err)
	}
	if changed.PlanRef != plan.PlanRef {
		t.Fatalf("presentation changed authority: %q != %q", changed.PlanRef, plan.PlanRef)
	}
	changedProof := append([]RuntimeStorageObservation{}, plan.Storage...)
	changedProof[0].Snapshots = append([]RuntimeSnapshotStorage{}, plan.Storage[0].Snapshots...)
	changedProof[0].Snapshots[0].SemanticFingerprint = "sha256:" + strings.Repeat("f", 64)
	changedRef, err := runtimePrunePlanAuthorityRef(plan.Candidates, plan.Protected, plan.Blockers, changedProof)
	if err != nil {
		t.Fatal(err)
	}
	if changedRef == plan.PlanRef {
		t.Fatal("immutable snapshot fingerprint did not change plan authority")
	}
}

func TestRuntimeMaterialObservationPreservesTagContentAndMigrationDistinctions(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	valid := []RuntimeMaterialObservation{
		{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, SharedContent: true, OwnershipVerified: true, ObservationComplete: true},
		{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityMissing, ContentPresent: true, SharedContent: true, OwnershipVerified: true, ObservationComplete: true, WorkspaceInUse: true},
		{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityMissing, ObservationComplete: true},
		{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityUnknown, MigrationUnverified: true, ObservationComplete: true},
	}
	for _, observation := range valid {
		if err := observation.Validate(); err != nil {
			t.Fatalf("valid material distinction %+v: %v", observation, err)
		}
	}

	invalid := []RuntimeMaterialObservation{
		{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityAvailable, OwnershipVerified: true, ObservationComplete: true},
		{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityMissing, TagPresent: true, ObservationComplete: true},
		{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityMismatched, ObservationComplete: true},
		{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityMissing, SharedContent: true, ObservationComplete: true},
		{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityMissing, OwnershipVerified: true, ObservationComplete: true},
		{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityMissing, MigrationUnverified: true, ObservationComplete: true},
	}
	for _, observation := range invalid {
		if err := observation.Validate(); err == nil {
			t.Fatalf("invalid material distinction validated: %+v", observation)
		}
	}

	snapshot := lifecycleSnapshot([]RuntimeManifest{lifecycleRuntime(id, "frontend", revision)}, []RuntimeProtection{}, []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityUnknown, MigrationUnverified: true, ObservationComplete: true}})
	plan, err := PlanRuntimePrune(snapshot, time.Unix(10, 0).UTC())
	if err != nil || plan.Applicable || len(plan.Blockers) != 1 || plan.Blockers[0].Reason != RuntimeBlockedByMigrationUnverified {
		t.Fatalf("migration-unverified blocker = %+v/%v", plan, err)
	}

	shared := lifecycleSnapshot([]RuntimeManifest{lifecycleRuntime(id, "frontend", revision)}, []RuntimeProtection{}, []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityMissing, ContentPresent: true, SharedContent: true, OwnershipVerified: true, ObservationComplete: true}})
	plan, err = PlanRuntimePrune(shared, time.Unix(10, 0).UTC())
	if err != nil || !plan.Applicable || len(plan.Blockers) != 1 || plan.Blockers[0].Reason != RuntimeBlockedByImageTagShared {
		t.Fatalf("tag-missing shared-content blocker = %+v/%v", plan, err)
	}
}

func TestRuntimePrunePlanRepresentsCompleteJournalState(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	manifest := lifecycleRuntime(id, "frontend", revision)
	available := RuntimeMaterialObservation{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true}

	incomplete := lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{}, []RuntimeMaterialObservation{available})
	incomplete.Journals.Complete = false
	if _, err := PlanRuntimePrune(incomplete, time.Unix(10, 0).UTC()); err == nil {
		t.Fatal("incomplete lifecycle journals planned")
	}

	active := lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{}, []RuntimeMaterialObservation{available})
	active.Journals.Active = []RuntimeLifecycleActivity{{Kind: RuntimeLifecycleActivityPrune, RuntimeID: id, Revisions: []string{revision}}}
	plan, err := PlanRuntimePrune(active, time.Unix(10, 0).UTC())
	if err != nil || plan.Applicable || len(plan.Candidates) != 0 || len(plan.Blockers) != 1 || plan.Blockers[0].Reason != RuntimeBlockedByActiveRetirement {
		t.Fatalf("active retirement plan = %+v/%v", plan, err)
	}

	failedRevision := "sha256:" + strings.Repeat("e", 64)
	failedMaterial := RuntimeMaterialObservation{RuntimeID: id, Revision: failedRevision, TagRole: RuntimeMaterialTagJournaledStaging, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true}
	failed := lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{}, []RuntimeMaterialObservation{available})
	failed.Journals.FailedBuilds = []RuntimeFailedBuildArtifact{{RuntimeID: id, Revision: failedRevision, RuntimeRef: RuntimeRef(id), Name: manifest.Name, Material: failedMaterial}}
	failed.Storage[0].Snapshots = append([]RuntimeSnapshotStorage{{Kind: RuntimePruneCandidateFailedBuild, Revision: failedRevision, SemanticFingerprint: failedRevision, LogicalBytes: 300}}, failed.Storage[0].Snapshots...)
	plan, err = PlanRuntimePrune(failed, time.Unix(10, 0).UTC())
	if err != nil || !plan.Applicable || len(plan.Candidates) != 2 || plan.Candidates[1].Kind != RuntimePruneCandidateFailedBuild || plan.Candidates[1].RevisionRef != "" || plan.Candidates[1].Ordinal != 0 {
		t.Fatalf("journaled failed-build plan = %+v/%v", plan, err)
	}

	wrongSuccessfulRole := lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{}, []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagJournaledStaging, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true}})
	if err := wrongSuccessfulRole.Validate(); err == nil {
		t.Fatal("successful revision accepted staging-tag evidence")
	}
	wrongFailedRole := failed
	wrongFailedRole.Journals.FailedBuilds = append([]RuntimeFailedBuildArtifact{}, failed.Journals.FailedBuilds...)
	wrongFailedRole.Journals.FailedBuilds[0].Material.TagRole = RuntimeMaterialTagPublishedRevision
	if err := wrongFailedRole.Validate(); err == nil {
		t.Fatal("failed build accepted published-tag evidence")
	}

	stagingMissing := failed
	stagingMissing.Journals.FailedBuilds = append([]RuntimeFailedBuildArtifact{}, failed.Journals.FailedBuilds...)
	stagingMissing.Journals.FailedBuilds[0].Material = RuntimeMaterialObservation{RuntimeID: id, Revision: failedRevision, TagRole: RuntimeMaterialTagJournaledStaging, Availability: RuntimeAvailabilityMissing, ContentPresent: true, SharedContent: true, OwnershipVerified: true, ObservationComplete: true}
	plan, err = PlanRuntimePrune(stagingMissing, time.Unix(10, 0).UTC())
	if err != nil || len(plan.Blockers) != 1 || plan.Blockers[0].Reason != RuntimeBlockedByStagingTagShared {
		t.Fatalf("staging-tag blocker = %+v/%v", plan, err)
	}
}

func TestRuntimePrunePlanIdentityChangesWithAuthorityEvidence(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	manifest := lifecycleRuntime(id, "frontend", revision)
	available := []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true}}
	candidate, err := PlanRuntimePrune(lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{}, available), time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	protection := RuntimeProtection{RuntimeID: id, RuntimeRevision: revision, Reason: RuntimeProtectedByManifestCurrent, WorkspaceManifestID: "018bcfe5-687b-7000-8000-000000000088", ManifestRevision: "sha256:" + strings.Repeat("d", 64)}
	protected, err := PlanRuntimePrune(lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{protection}, available), time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	blockedMaterials := []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true, ExternalInUse: true}}
	blocked, err := PlanRuntimePrune(lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{}, blockedMaterials), time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if candidate.PlanRef == protected.PlanRef || candidate.PlanRef == blocked.PlanRef || protected.PlanRef == blocked.PlanRef {
		t.Fatalf("authority evidence did not change identity: %q %q %q", candidate.PlanRef, protected.PlanRef, blocked.PlanRef)
	}
}

func TestRuntimePrunePlanIdentityIncludesCandidateKind(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	revisionCandidate := RuntimePruneCandidate{Kind: RuntimePruneCandidateRevision, RuntimeID: id, Revision: revision, RuntimeRef: RuntimeRef(id), RevisionRef: RuntimeRevisionRef(id, revision), Name: "frontend", Ordinal: 1}
	failedCandidate := RuntimePruneCandidate{Kind: RuntimePruneCandidateFailedBuild, RuntimeID: id, Revision: revision, RuntimeRef: RuntimeRef(id), Name: "frontend"}
	revisionRef, err := runtimePrunePlanAuthorityRef([]RuntimePruneCandidate{revisionCandidate}, []RuntimeProtection{}, []RuntimeMaterialBlocker{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	failedRef, err := runtimePrunePlanAuthorityRef([]RuntimePruneCandidate{failedCandidate}, []RuntimeProtection{}, []RuntimeMaterialBlocker{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if revisionRef == failedRef {
		t.Fatalf("candidate kind did not change plan authority: %q", revisionRef)
	}
}

func TestRuntimePrunePlanValidateRejectsDirectInvalidConstruction(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	bytes := int64(2048)
	valid, err := PlanRuntimePrune(lifecycleSnapshot([]RuntimeManifest{lifecycleRuntime(id, "frontend", revision)}, []RuntimeProtection{}, []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true, ImageVirtualBytes: &bytes}}), time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*RuntimePrunePlan){
		"missing last-used certainty": func(plan *RuntimePrunePlan) { plan.Candidates[0].LastUsed = "" },
		"invented reclaimable bytes": func(plan *RuntimePrunePlan) {
			reclaimable := int64(1)
			plan.Candidates[0].ReclaimableBytes = &reclaimable
		},
		"negative bytes": func(plan *RuntimePrunePlan) {
			negative := int64(-1)
			plan.Candidates[0].ImageVirtualBytes = &negative
		},
		"candidate source bytes drift":   func(plan *RuntimePrunePlan) { plan.Candidates[0].SourceLogicalBytes++ },
		"candidate snapshot bytes drift": func(plan *RuntimePrunePlan) { plan.Candidates[0].SnapshotLogicalBytes++ },
		"nil storage":                    func(plan *RuntimePrunePlan) { plan.Storage = nil },
		"negative source storage":        func(plan *RuntimePrunePlan) { plan.Storage[0].SourceLogicalBytes = -1 },
		"snapshot fingerprint drift": func(plan *RuntimePrunePlan) {
			plan.Storage[0].Snapshots[0].SemanticFingerprint = "sha256:" + strings.Repeat("f", 64)
		},
		"missing snapshot storage": func(plan *RuntimePrunePlan) { plan.Storage[0].Snapshots = []RuntimeSnapshotStorage{} },
		"duplicate candidate":      func(plan *RuntimePrunePlan) { plan.Candidates = append(plan.Candidates, plan.Candidates[0]) },
		"invalid protection": func(plan *RuntimePrunePlan) {
			plan.Protected = []RuntimeProtection{{RuntimeID: id, RuntimeRevision: revision}}
		},
		"duplicate protection": func(plan *RuntimePrunePlan) {
			item := RuntimeProtection{RuntimeID: id, RuntimeRevision: revision, Reason: RuntimeProtectedByManifestCurrent, WorkspaceManifestID: "018bcfe5-687b-7000-8000-000000000088", ManifestRevision: "sha256:" + strings.Repeat("d", 64)}
			plan.Protected = []RuntimeProtection{item, item}
		},
		"duplicate blocker": func(plan *RuntimePrunePlan) {
			blocker := RuntimeMaterialBlocker{RuntimeID: id, Revision: revision, Reason: RuntimeBlockedByExternalContainer}
			plan.Blockers = []RuntimeMaterialBlocker{blocker, blocker}
		},
		"corrupt authority": func(plan *RuntimePrunePlan) { plan.PlanRef = "sha256:" + strings.Repeat("0", 64) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			plan := valid
			plan.Candidates = append([]RuntimePruneCandidate{}, valid.Candidates...)
			plan.Protected = append([]RuntimeProtection{}, valid.Protected...)
			plan.Blockers = append([]RuntimeMaterialBlocker{}, valid.Blockers...)
			plan.Storage = append([]RuntimeStorageObservation{}, valid.Storage...)
			plan.Storage[0].Snapshots = append([]RuntimeSnapshotStorage{}, valid.Storage[0].Snapshots...)
			mutate(&plan)
			if err := plan.Validate(); err == nil {
				t.Fatalf("invalid direct construction validated: %+v", plan)
			}
		})
	}
}

func TestRuntimePruneResultRequiresExactDurableEvidence(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	plan, err := PlanRuntimePrune(lifecycleSnapshot([]RuntimeManifest{lifecycleRuntime(id, "frontend", revision)}, []RuntimeProtection{}, []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true}}), time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	candidate := plan.Candidates[0]
	item := RuntimePruneItemResult{Kind: candidate.Kind, RuntimeID: candidate.RuntimeID, Revision: candidate.Revision, RuntimeRef: candidate.RuntimeRef, RevisionRef: candidate.RevisionRef, Name: candidate.Name, Ordinal: candidate.Ordinal, LastUsed: candidate.LastUsed, SourceLogicalBytes: candidate.SourceLogicalBytes, SnapshotLogicalBytes: candidate.SnapshotLogicalBytes, Disposition: RuntimePruneRemoved, RemovedTagCount: 1}
	valid := RuntimePruneResult{Task: TaskRuntimePruneApply, PlanRef: plan.PlanRef, State: RuntimePruneApplied, Items: []RuntimePruneItemResult{item}, RemovedTagCount: 1, ReceiptRevision: 1, SourcePreserved: true, SnapshotsPreserved: true, HistoryPreserved: true}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid Runtime prune result: %v", err)
	}
	already := valid
	already.State = RuntimePruneAlreadyApplied
	if err := already.Validate(); err != nil {
		t.Fatalf("idempotent Runtime prune result: %v", err)
	}
	empty := RuntimePruneResult{Task: TaskRuntimePruneApply, PlanRef: plan.PlanRef, State: RuntimePruneEmpty, Items: []RuntimePruneItemResult{}, SourcePreserved: true, SnapshotsPreserved: true, HistoryPreserved: true}
	if err := empty.Validate(); err != nil {
		t.Fatalf("empty Runtime prune result: %v", err)
	}

	tests := map[string]func(*RuntimePruneResult){
		"missing preservation":     func(result *RuntimePruneResult) { result.SourcePreserved = false },
		"invented reclaimed bytes": func(result *RuntimePruneResult) { value := int64(1); result.ReclaimedBytes = &value },
		"missing receipt":          func(result *RuntimePruneResult) { result.ReceiptRevision = 0 },
		"wrong total":              func(result *RuntimePruneResult) { result.RemovedTagCount = 0 },
		"duplicate item": func(result *RuntimePruneResult) {
			result.Items = append(result.Items, result.Items[0])
			result.RemovedTagCount++
		},
		"wrong revision authority":    func(result *RuntimePruneResult) { result.Items[0].RevisionRef = "" },
		"missing last-used certainty": func(result *RuntimePruneResult) { result.Items[0].LastUsed = "" },
		"invented item reclaim": func(result *RuntimePruneResult) {
			value := int64(1)
			result.Items[0].ReclaimedBytes = &value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			result := valid
			result.Items = append([]RuntimePruneItemResult{}, valid.Items...)
			mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatalf("invalid Runtime prune result validated: %+v", result)
			}
		})
	}
}

func TestRuntimeLifecycleSnapshotKeepsPruneRecoveryReachableDuringExactBuildCleanup(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	snapshot := lifecycleSnapshot([]RuntimeManifest{lifecycleRuntime(id, "frontend")}, []RuntimeProtection{}, []RuntimeMaterialObservation{})
	snapshot.Journals.Active = []RuntimeLifecycleActivity{
		{Kind: RuntimeLifecycleActivityBuild, RuntimeID: id, Revisions: []string{revision}},
		{Kind: RuntimeLifecycleActivityPrune, RuntimeID: id, Revisions: []string{revision}},
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("exact build-cleanup/prune recovery snapshot: %v", err)
	}
	drifted := snapshot
	drifted.Journals.Active = append([]RuntimeLifecycleActivity{}, snapshot.Journals.Active...)
	drifted.Journals.Active[0].Revisions = []string{"sha256:" + strings.Repeat("b", 64)}
	if err := drifted.Validate(); err == nil {
		t.Fatal("prune recovery borrowed unrelated build authority")
	}
}

func TestRuntimePrunePlanValidateRequiresCanonicalProtectionAndBlockerOrder(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	manifestID := "018bcfe5-687b-7000-8000-000000000088"
	manifestRevision := "sha256:" + strings.Repeat("d", 64)
	protections := []RuntimeProtection{
		{RuntimeID: id, RuntimeRevision: revision, Reason: RuntimeProtectedByManifestRetained, WorkspaceManifestID: manifestID, ManifestRevision: manifestRevision},
		{RuntimeID: id, RuntimeRevision: revision, Reason: RuntimeProtectedByManifestCurrent, WorkspaceManifestID: manifestID, ManifestRevision: manifestRevision},
	}
	blockers := []RuntimeMaterialBlocker{
		{RuntimeID: id, Revision: revision, Reason: RuntimeBlockedByWorkspaceContainer},
		{RuntimeID: id, Revision: revision, Reason: RuntimeBlockedByExternalContainer},
	}
	plan := RuntimePrunePlan{
		Task:       TaskRuntimePruneDryRun,
		ObservedAt: time.Unix(10, 0).UTC(),
		Empty:      true,
		Applicable: true,
		Candidates: []RuntimePruneCandidate{},
		Protected:  protections,
		Blockers:   blockers,
		Storage:    []RuntimeStorageObservation{},
	}
	var err error
	plan.PlanRef, err = runtimePrunePlanAuthorityRef(plan.Candidates, plan.Protected, plan.Blockers, plan.Storage)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err == nil {
		t.Fatal("non-canonical protections and blockers validated")
	}

	plan.Protected[0], plan.Protected[1] = plan.Protected[1], plan.Protected[0]
	plan.PlanRef, err = runtimePrunePlanAuthorityRef(plan.Candidates, plan.Protected, plan.Blockers, plan.Storage)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err == nil {
		t.Fatal("non-canonical blockers validated")
	}

	plan.Blockers[0], plan.Blockers[1] = plan.Blockers[1], plan.Blockers[0]
	plan.PlanRef, err = runtimePrunePlanAuthorityRef(plan.Candidates, plan.Protected, plan.Blockers, plan.Storage)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("canonical direct plan: %v", err)
	}
}

func TestRuntimePrunePlanValidateRejectsCandidateProtectionOrBlockerOverlap(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	available := RuntimeMaterialObservation{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true}
	valid, err := PlanRuntimePrune(lifecycleSnapshot([]RuntimeManifest{lifecycleRuntime(id, "frontend", revision)}, []RuntimeProtection{}, []RuntimeMaterialObservation{available}), time.Unix(10, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}

	protectionOverlap := valid
	protectionOverlap.Protected = []RuntimeProtection{{RuntimeID: id, RuntimeRevision: revision, Reason: RuntimeProtectedByManifestCurrent, WorkspaceManifestID: "018bcfe5-687b-7000-8000-000000000088", ManifestRevision: "sha256:" + strings.Repeat("d", 64)}}
	protectionOverlap.PlanRef, err = runtimePrunePlanAuthorityRef(protectionOverlap.Candidates, protectionOverlap.Protected, protectionOverlap.Blockers, protectionOverlap.Storage)
	if err != nil {
		t.Fatal(err)
	}
	if err := protectionOverlap.Validate(); err == nil {
		t.Fatal("candidate/protection overlap validated")
	}

	blockerOverlap := valid
	blockerOverlap.Blockers = []RuntimeMaterialBlocker{{RuntimeID: id, Revision: revision, Reason: RuntimeBlockedByExternalContainer}}
	blockerOverlap.PlanRef, err = runtimePrunePlanAuthorityRef(blockerOverlap.Candidates, blockerOverlap.Protected, blockerOverlap.Blockers, blockerOverlap.Storage)
	if err != nil {
		t.Fatal(err)
	}
	if err := blockerOverlap.Validate(); err == nil {
		t.Fatal("candidate/blocker overlap validated")
	}
}

func TestPlanRuntimePruneFailsClosedOnUnknownOrUnownedMaterial(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	base := lifecycleSnapshot([]RuntimeManifest{lifecycleRuntime(id, "frontend", revision)}, []RuntimeProtection{}, []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true}})
	for _, mutate := range []func(*RuntimeLifecycleSnapshot){
		func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Materials[0].ObservationComplete = false },
		func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Materials[0].OwnershipVerified = false },
		func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Protection.Complete = false },
		func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Materials = []RuntimeMaterialObservation{} },
	} {
		snapshot := base
		snapshot.Materials = append([]RuntimeMaterialObservation{}, base.Materials...)
		mutate(&snapshot)
		if _, err := PlanRuntimePrune(snapshot, time.Unix(10, 0).UTC()); err == nil {
			t.Fatalf("unsafe snapshot planned: %+v", snapshot)
		}
	}
}

func TestPlanRuntimePruneDoesNotTreatHeadOrAppliedTimestampAsUsageAuthority(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	snapshot := lifecycleSnapshot([]RuntimeManifest{lifecycleRuntime(id, "frontend", revision)}, []RuntimeProtection{}, []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true}})
	plan, err := PlanRuntimePrune(snapshot, time.Unix(10, 0).UTC())
	if err != nil || len(plan.Candidates) != 1 {
		t.Fatalf("unprotected head plan = %+v/%v", plan, err)
	}
}

func TestRuntimeBuildRecoveryRequiresExactStableIdentityAndKnownKind(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	for _, kind := range []RuntimeBuildRecoveryKind{RuntimeBuildRecoveryPreDocker, RuntimeBuildRecoveryBuilding, RuntimeBuildRecoveryPublication, RuntimeBuildRecoveryCleanup, RuntimeBuildRecoveryOrphan, RuntimeBuildRecoveryFailed} {
		recovery := RuntimeBuildRecovery{RuntimeID: id, RuntimeRef: RuntimeRef(id), Name: "frontend", Kind: kind}
		if err := recovery.Validate(); err != nil {
			t.Fatalf("recovery kind %q: %v", kind, err)
		}
	}
	for _, recovery := range []RuntimeBuildRecovery{
		{RuntimeID: id, RuntimeRef: RuntimeRef("018bcfe5-687b-7000-8000-000000000099"), Name: "frontend", Kind: RuntimeBuildRecoveryCleanup},
		{RuntimeID: id, RuntimeRef: RuntimeRef(id), RevisionRef: RuntimeRevisionRef("018bcfe5-687b-7000-8000-000000000099", "sha256:"+strings.Repeat("a", 64)), Name: "frontend", Kind: RuntimeBuildRecoveryCleanup},
		{RuntimeID: id, RuntimeRef: RuntimeRef(id), Name: "frontend", Kind: RuntimeBuildRecoveryCleanup, RestoreFailed: true},
		{RuntimeID: id, RuntimeRef: RuntimeRef(id), RevisionRef: RuntimeRevisionRef(id, "sha256:"+strings.Repeat("a", 64)), Name: "frontend", Kind: RuntimeBuildRecoveryPublication, RestoreFailed: true},
		{RuntimeID: id, RuntimeRef: RuntimeRef(id), Name: "frontend", Kind: "unknown"},
	} {
		if err := recovery.Validate(); err == nil {
			t.Fatalf("invalid recovery validated: %+v", recovery)
		}
	}
	restore := RuntimeBuildRecovery{RuntimeID: id, RuntimeRef: RuntimeRef(id), RevisionRef: RuntimeRevisionRef(id, "sha256:"+strings.Repeat("a", 64)), Name: "frontend", Kind: RuntimeBuildRecoveryFailed, RestoreFailed: true}
	if err := restore.Validate(); err != nil {
		t.Fatalf("exact Runtime restore recovery rejected: %v", err)
	}
}
