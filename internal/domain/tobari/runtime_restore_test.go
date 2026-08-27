package tobari

import (
	"errors"
	"strings"
	"testing"
)

func TestRuntimeRevisionReferenceIsExactOpaqueManagedAuthority(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	reference := RuntimeRevisionRef(id, revision)
	gotID, gotRevision, err := ParseRuntimeRevisionRef(reference)
	if err != nil || gotID != id || gotRevision != revision {
		t.Fatalf("ParseRuntimeRevisionRef() = %q/%q/%v", gotID, gotRevision, err)
	}
	for _, invalid := range []string{
		"", "frontend@1", id, id + "/1", id + "/" + strings.Repeat("a", 64),
		id + "/sha256:" + strings.Repeat("a", 63),
		id + "/" + revision + "/extra", "tobari-runtime-frontend:test",
	} {
		if _, _, err := ParseRuntimeRevisionRef(invalid); err == nil {
			t.Errorf("invalid Runtime revision reference accepted: %q", invalid)
		}
	}
}

func TestBuiltinRuntimeRevisionReferenceRoundTripsExactly(t *testing.T) {
	revision := "sha256:" + strings.Repeat("f", 64)
	reference := RuntimeRevisionRef(StandardRuntimeID, revision)
	gotID, gotRevision, err := ParseRuntimeRevisionRef(reference)
	if err != nil || gotID != StandardRuntimeID || gotRevision != revision {
		t.Fatalf("ParseRuntimeRevisionRef() = %q/%q/%v", gotID, gotRevision, err)
	}
	if got := RuntimeRevisionRef(gotID, gotRevision); got != reference {
		t.Fatalf("Runtime revision reference round trip = %q, want %q", got, reference)
	}
}

func TestRuntimeRestoreTargetRequiresCompleteExactRetainedAuthority(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	reference := RuntimeRevisionRef(id, revision)
	manifest := lifecycleRuntime(id, "frontend", revision)
	material := RuntimeMaterialObservation{
		RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityPruned,
		ObservationComplete: true,
	}
	snapshot := lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{}, []RuntimeMaterialObservation{material})
	target, err := RuntimeRestoreTargetFrom(snapshot, reference)
	if err != nil {
		t.Fatal(err)
	}
	if target.RuntimeID != id || target.RuntimeRef != RuntimeRef(id) || target.Revision != revision || target.RevisionRef != reference ||
		target.Name != "frontend" || target.Ordinal != 1 || target.RecordedImageDigest != manifest.Revisions[0].ImageDigest ||
		target.SnapshotLogicalBytes != 100 || target.Availability != RuntimeAvailabilityPruned {
		t.Fatalf("restore target = %+v", target)
	}

	available := snapshot
	available.Materials = append([]RuntimeMaterialObservation{}, snapshot.Materials...)
	available.Materials[0] = RuntimeMaterialObservation{
		RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision,
		Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true,
		OwnershipVerified: true, ObservationComplete: true,
	}
	target, err = RuntimeRestoreTargetFrom(available, reference)
	if err != nil || target.Availability != RuntimeAvailabilityAvailable {
		t.Fatalf("already-available target = %+v/%v", target, err)
	}
}

func TestRuntimeRestoreTargetFailsClosedBeforeEffect(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	other := "sha256:" + strings.Repeat("c", 64)
	reference := RuntimeRevisionRef(id, revision)
	manifest := lifecycleRuntime(id, "frontend", revision)
	base := lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{}, []RuntimeMaterialObservation{{RuntimeID: id, Revision: revision, Availability: RuntimeAvailabilityPruned, ObservationComplete: true}})

	tests := []struct {
		name string
		ref  string
		edit func(*RuntimeLifecycleSnapshot)
		want error
	}{
		{name: "malformed reference", ref: "frontend@1", want: nil},
		{name: "missing Runtime", ref: RuntimeRevisionRef("018bcfe5-687b-7000-8000-000000000099", revision), want: ErrRuntimeNotFound},
		{name: "missing revision", ref: RuntimeRevisionRef(id, other), want: ErrRuntimeRevisionNotFound},
		{name: "unknown observation", ref: reference, edit: func(snapshot *RuntimeLifecycleSnapshot) {
			snapshot.Materials[0].Availability = RuntimeAvailabilityUnknown
		}, want: ErrRuntimeRetirementObservationUnknown},
		{name: "mismatched material", ref: reference, edit: func(snapshot *RuntimeLifecycleSnapshot) {
			snapshot.Materials[0].Availability = RuntimeAvailabilityMismatched
			snapshot.Materials[0].TagPresent = true
		}, want: ErrRuntimeRevisionUnrestorable},
		{name: "active lifecycle", ref: reference, edit: func(snapshot *RuntimeLifecycleSnapshot) {
			snapshot.Journals.Active = []RuntimeLifecycleActivity{{Kind: RuntimeLifecycleActivityRestore, RuntimeID: id, Revisions: []string{revision}}}
		}, want: ErrRuntimeLifecycleActive},
		{name: "drifted snapshot", ref: reference, edit: func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Storage[0].Snapshots[0].SemanticFingerprint = other }, want: ErrRuntimeRetirementObservationUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := base
			snapshot.Runtimes = append([]RuntimeManifest{}, base.Runtimes...)
			snapshot.Materials = append([]RuntimeMaterialObservation{}, base.Materials...)
			snapshot.Storage = append([]RuntimeStorageObservation{}, base.Storage...)
			snapshot.Storage[0].Snapshots = append([]RuntimeSnapshotStorage{}, base.Storage[0].Snapshots...)
			snapshot.Journals.Active = append([]RuntimeLifecycleActivity{}, base.Journals.Active...)
			if test.edit != nil {
				test.edit(&snapshot)
			}
			_, err := RuntimeRestoreTargetFrom(snapshot, test.ref)
			if err == nil {
				t.Fatal("unsafe restore target was accepted")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestRuntimeRestoreResultPreservesImmutableAuthority(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	valid := RuntimeRestoreResult{
		Task: TaskRuntimeRestore, RuntimeID: id, RuntimeRef: RuntimeRef(id), Revision: revision,
		RevisionRef: RuntimeRevisionRef(id, revision), Name: "frontend", Ordinal: 1,
		State: RuntimeRestored, DigestMatch: true, ArtifactDisposition: RuntimeRestoreArtifactRemoved,
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	already := valid
	already.State = RuntimeAlreadyAvailable
	already.ArtifactDisposition = RuntimeRestoreArtifactNotCreated
	if err := already.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := []RuntimeRestoreResult{valid, valid, valid, valid, valid, valid}
	invalid[0].RevisionRef = RuntimeRevisionRef(id, "sha256:"+strings.Repeat("b", 64))
	invalid[1].DigestMatch = false
	invalid[2].RevisionAppended = true
	invalid[3].ManifestChanged = true
	invalid[4].WorkspaceChanged = true
	invalid[5].ArtifactDisposition = RuntimeRestoreArtifactNotCreated
	for _, result := range invalid {
		if err := result.Validate(); err == nil {
			t.Fatalf("invalid restore result accepted: %+v", result)
		}
	}
}

func TestRevisionReferencesJoinDiscoveryOnlyWithRestoreConsumer(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	report := RuntimeReport{Task: TaskRuntimeShow, Runtime: lifecycleRuntime(id, "frontend", revision)}
	withoutConsumer, err := RuntimeReportWithReferences(report)
	if err != nil {
		t.Fatal(err)
	}
	if withoutConsumer.Runtime.Revisions[0].RevisionRef != "" {
		t.Fatal("revision reference was published without its consumer")
	}
	withConsumer, err := RuntimeReportWithRevisionReferences(report)
	if err != nil {
		t.Fatal(err)
	}
	if withConsumer.Runtime.Revisions[0].RevisionRef != RuntimeRevisionRef(id, revision) {
		t.Fatalf("revision reference = %q", withConsumer.Runtime.Revisions[0].RevisionRef)
	}
	parsedID, parsedRevision, err := ParseRuntimeRevisionRef(withConsumer.Runtime.Revisions[0].RevisionRef)
	if err != nil || parsedID != id || parsedRevision != revision {
		t.Fatalf("managed producer/consumer round trip = %q/%q/%v", parsedID, parsedRevision, err)
	}

	standard := RuntimeReport{Task: TaskRuntimeShow, Runtime: lifecycleStandard()}
	standardWithConsumer, err := RuntimeReportWithRevisionReferences(standard)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range standardWithConsumer.Runtime.Revisions {
		if item.RevisionRef != "" {
			t.Fatalf("built-in Runtime exposed an ineligible restore reference: %q", item.RevisionRef)
		}
	}
}
