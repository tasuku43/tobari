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

func TestPlanRuntimePruneIsDeterministicAndExcludesProtectedMaterial(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	first := "sha256:" + strings.Repeat("a", 64)
	second := "sha256:" + strings.Repeat("c", 64)
	bytes := int64(2048)
	manifest := lifecycleRuntime(id, "frontend", first, second)
	protection := RuntimeProtection{RuntimeID: id, RuntimeRevision: first, Reason: RuntimeProtectedByManifestCurrent, WorkspaceManifestID: "018bcfe5-687b-7000-8000-000000000088", ManifestRevision: "sha256:" + strings.Repeat("d", 64)}
	snapshot := RuntimeLifecycleSnapshot{
		Runtimes:   []RuntimeManifest{manifest},
		Protection: RuntimeProtectionInventory{Complete: true, Items: []RuntimeProtection{protection}},
		Materials: []RuntimeMaterialObservation{
			{RuntimeID: id, Revision: second, Availability: RuntimeAvailabilityAvailable, OwnershipVerified: true, ObservationComplete: true, ImageVirtualBytes: &bytes},
			{RuntimeID: id, Revision: first, Availability: RuntimeAvailabilityAvailable, OwnershipVerified: true, ObservationComplete: true},
		},
	}
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
	if planA.Empty || len(planA.Candidates) != 1 || planA.Candidates[0].Revision != second || len(planA.Protected) != 1 || planA.Protected[0] != protection {
		t.Fatalf("prune plan = %+v", planA)
	}
	if planA.Candidates[0].RevisionRef != RuntimeRevisionRef(id, second) || planA.Candidates[0].ImageVirtualBytes == nil || *planA.Candidates[0].ImageVirtualBytes != bytes {
		t.Fatalf("candidate authority/evidence = %+v", planA.Candidates[0])
	}
}

func TestPlanRuntimePruneFailsClosedOnUnknownOrUnownedMaterial(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	base := RuntimeLifecycleSnapshot{Runtimes: []RuntimeManifest{lifecycleRuntime(id, "frontend", revision)}, Protection: RuntimeProtectionInventory{Complete: true, Items: []RuntimeProtection{}}, Materials: []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityAvailable, OwnershipVerified: true, ObservationComplete: true}}}
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
	snapshot := RuntimeLifecycleSnapshot{Runtimes: []RuntimeManifest{lifecycleRuntime(id, "frontend", revision)}, Protection: RuntimeProtectionInventory{Complete: true, Items: []RuntimeProtection{}}, Materials: []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityAvailable, OwnershipVerified: true, ObservationComplete: true}}}
	plan, err := PlanRuntimePrune(snapshot, time.Unix(10, 0).UTC())
	if err != nil || len(plan.Candidates) != 1 {
		t.Fatalf("unprotected head plan = %+v/%v", plan, err)
	}
}
