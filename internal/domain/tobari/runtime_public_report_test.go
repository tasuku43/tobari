package tobari

import (
	"reflect"
	"strings"
	"testing"
)

func TestRuntimePublicReportProjectsSemanticLifecycleWithoutInfrastructureIdentity(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	digest := "sha256:" + strings.Repeat("a", 64)
	runtime := lifecycleRuntime(id, "frontend", digest)
	virtualBytes := int64(2048)
	snapshot := lifecycleSnapshot([]RuntimeManifest{runtime}, []RuntimeProtection{}, []RuntimeMaterialObservation{{
		RuntimeID: id, Revision: digest, Availability: RuntimeAvailabilityPruned, ObservationComplete: true, ImageVirtualBytes: &virtualBytes,
	}})

	report, err := RuntimeReportFromLifecycleSnapshot(snapshot, TaskRuntimeHistory, runtime.Name)
	if err != nil {
		t.Fatal(err)
	}
	if report.Public == nil || len(report.Public.Runtime.Revisions) != 1 {
		t.Fatalf("public Runtime report = %+v", report.Public)
	}
	revision := report.Public.Runtime.Revisions[0]
	if revision.SourceDigest != digest || revision.RevisionRef != RuntimeRevisionRef(id, digest) ||
		revision.Availability.State != RuntimeAvailabilityPruned || revision.Storage == nil ||
		revision.Storage.SourceLogicalBytes == nil || *revision.Storage.SourceLogicalBytes != 42 ||
		revision.Storage.SnapshotLogicalBytes == nil || *revision.Storage.SnapshotLogicalBytes != 100 ||
		revision.Storage.ImageVirtualBytes == nil || *revision.Storage.ImageVirtualBytes != virtualBytes ||
		revision.Storage.ReclaimableBytes != nil || revision.LastUsed.State != RuntimeLastUsedUnknown ||
		revision.LastUsed.ObservedAt != nil || revision.Snapshot.State != RuntimeSnapshotRetained {
		t.Fatalf("public Runtime revision = %+v", revision)
	}
	if report.Runtime.Revisions[0].Image == "" || report.Runtime.Revisions[0].ImageDigest == "" || report.Runtime.Revisions[0].SnapshotPath == "" {
		t.Fatal("public projection mutated persisted infrastructure authority")
	}
}

func TestRuntimeRecoveryIsDistinctFromSourceNoChange(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	digest := "sha256:" + strings.Repeat("a", 64)
	runtime := lifecycleRuntime(id, "frontend", digest)
	if err := (RuntimeReport{Task: TaskRuntimeBuildV1, Runtime: runtime, Recovered: true}).Validate(); err != nil {
		t.Fatalf("recovery-only report = %v", err)
	}
	if err := (RuntimeReport{Task: TaskRuntimeBuildV1, Runtime: runtime, Recovered: true, NoChange: true}).Validate(); err == nil {
		t.Fatal("Runtime recovery claimed an unrelated source no-change build")
	}
	snapshot := lifecycleSnapshot([]RuntimeManifest{runtime}, []RuntimeProtection{}, []RuntimeMaterialObservation{{
		RuntimeID: id, Revision: digest, Availability: RuntimeAvailabilityAvailable, TagPresent: true,
		ContentPresent: true, OwnershipVerified: true, ObservationComplete: true,
	}})
	projected, err := RuntimeReportWithLifecycleEvidence(RuntimeReport{Task: TaskRuntimeBuildV1, Runtime: runtime, NoChange: true}, snapshot)
	if err != nil || projected.Public == nil {
		t.Fatalf("source no-change projection = %+v/%v", projected.Public, err)
	}
	public := *projected.Public
	public.Recovered = true
	if err := public.Validate(); err == nil {
		t.Fatal("public Runtime recovery claimed an unrelated source no-change build")
	}
}

func TestRuntimeListKeepsHistoryReadySeparateFromPrunedHead(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	digest := "sha256:" + strings.Repeat("a", 64)
	runtime := lifecycleRuntime(id, "frontend", digest)
	snapshot := lifecycleSnapshot([]RuntimeManifest{runtime}, []RuntimeProtection{}, []RuntimeMaterialObservation{{
		RuntimeID: id, Revision: digest, Availability: RuntimeAvailabilityPruned, ObservationComplete: true,
	}})

	result, err := RuntimeListFromLifecycleSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Items[0].Kind != RuntimeKindBuiltin || result.Items[0].Storage != nil ||
		result.Items[0].Availability == nil || result.Items[0].Availability.State != RuntimeAvailabilityUnknown {
		t.Fatalf("standard Runtime summary = %+v", result.Items[0])
	}
	managed := result.Items[1]
	if !managed.Ready || managed.Availability == nil || managed.Availability.State != RuntimeAvailabilityPruned || managed.Storage == nil {
		t.Fatalf("managed Runtime summary = %+v", managed)
	}
}

func TestRuntimePublicAvailabilityPreservesEveryObservedState(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	digest := "sha256:" + strings.Repeat("a", 64)
	runtime := lifecycleRuntime(id, "frontend", digest)
	tests := map[RuntimeAvailability]RuntimeMaterialObservation{
		RuntimeAvailabilityAvailable:  {TagPresent: true, ContentPresent: true, OwnershipVerified: true},
		RuntimeAvailabilityMissing:    {},
		RuntimeAvailabilityMismatched: {TagPresent: true},
		RuntimeAvailabilityUnknown:    {MigrationUnverified: true},
		RuntimeAvailabilityPruned:     {},
	}
	for availability, evidence := range tests {
		t.Run(string(availability), func(t *testing.T) {
			evidence.RuntimeID, evidence.Revision = id, digest
			evidence.Availability, evidence.ObservationComplete = availability, true
			snapshot := lifecycleSnapshot([]RuntimeManifest{runtime}, []RuntimeProtection{}, []RuntimeMaterialObservation{evidence})
			report, err := RuntimeReportFromLifecycleSnapshot(snapshot, TaskRuntimeShow, runtime.Name)
			if err != nil {
				t.Fatal(err)
			}
			if got := report.Public.Runtime.Revisions[0].Availability.State; got != availability {
				t.Fatalf("availability = %q, want %q", got, availability)
			}
		})
	}
}

func TestRuntimePublicProjectionRejectsAuthorityDriftAndDoesNotAliasBytes(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	digest := "sha256:" + strings.Repeat("a", 64)
	runtime := lifecycleRuntime(id, "frontend", digest)
	virtualBytes := int64(2048)
	snapshot := lifecycleSnapshot([]RuntimeManifest{runtime}, []RuntimeProtection{}, []RuntimeMaterialObservation{{
		RuntimeID: id, Revision: digest, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true,
		OwnershipVerified: true, ObservationComplete: true, ImageVirtualBytes: &virtualBytes,
	}})
	drifted := runtime
	drifted.Revisions = append([]RuntimeRevision{}, runtime.Revisions...)
	drifted.Revisions[0].ImageDigest = "sha256:" + strings.Repeat("e", 64)
	if _, err := RuntimeReportWithLifecycleEvidence(RuntimeReport{Task: TaskRuntimeShow, Runtime: drifted}, snapshot); err == nil {
		t.Fatal("drifted Runtime authority received a semantic projection")
	}

	report, err := RuntimeReportFromLifecycleSnapshot(snapshot, TaskRuntimeShow, runtime.Name)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshot.Materials[0]
	*report.Public.Runtime.Revisions[0].Storage.ImageVirtualBytes = 4096
	if !reflect.DeepEqual(snapshot.Materials[0], before) {
		t.Fatal("public byte evidence aliases coherent lifecycle authority")
	}
}
