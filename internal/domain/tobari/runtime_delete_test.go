package tobari

import (
	"errors"
	"strings"
	"testing"
)

func TestRuntimeDeleteTargetBindsCompleteManagedAuthority(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	first := "sha256:" + strings.Repeat("a", 64)
	second := "sha256:" + strings.Repeat("b", 64)
	failed := "sha256:" + strings.Repeat("c", 64)
	manifest := lifecycleRuntime(id, "frontend", first, second)
	availableBytes := int64(300)
	materials := []RuntimeMaterialObservation{
		{RuntimeID: id, Revision: first, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityMissing, ObservationComplete: true},
		{RuntimeID: id, Revision: second, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, SharedContent: true, OwnershipVerified: true, ObservationComplete: true, ImageVirtualBytes: &availableBytes},
	}
	snapshot := lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{}, materials)
	failedMaterial := RuntimeMaterialObservation{RuntimeID: id, Revision: failed, TagRole: RuntimeMaterialTagJournaledStaging, Availability: RuntimeAvailabilityPruned, ObservationComplete: true}
	snapshot.Journals.FailedBuilds = []RuntimeFailedBuildArtifact{{RuntimeID: id, Revision: failed, RuntimeRef: RuntimeRef(id), Name: manifest.Name, Material: failedMaterial}}
	snapshot.Storage[0].Snapshots = append([]RuntimeSnapshotStorage{{Kind: RuntimePruneCandidateFailedBuild, Revision: failed, SemanticFingerprint: failed, LogicalBytes: 400}}, snapshot.Storage[0].Snapshots...)

	target, err := RuntimeDeleteTargetFrom(snapshot, RuntimeRef(id))
	if err != nil {
		t.Fatal(err)
	}
	if target.Runtime.RuntimeRef != RuntimeRef(id) || len(target.Materials) != 3 ||
		target.Materials[0].Candidate.Revision != first || target.Materials[0].Availability != RuntimeAvailabilityMissing ||
		target.Materials[1].Candidate.Revision != second || !target.Materials[1].SharedContent || target.Materials[1].Candidate.ImageVirtualBytes == nil ||
		target.Materials[2].Candidate.Kind != RuntimePruneCandidateFailedBuild || target.Materials[2].Candidate.Revision != failed {
		t.Fatalf("delete target = %+v", target)
	}
	for _, revision := range target.Runtime.Revisions {
		if revision.RuntimeRef != RuntimeRef(id) || revision.RevisionRef != RuntimeRevisionRef(id, revision.Revision) {
			t.Fatalf("delete revision authority = %+v", revision)
		}
	}
	for _, revision := range snapshot.Runtimes[1].Revisions {
		if revision.RuntimeRef != "" || revision.RevisionRef != "" {
			t.Fatalf("delete target derivation mutated its input snapshot: %+v", snapshot.Runtimes[1])
		}
	}
	storageBefore := snapshot.Storage[0].Snapshots[0]
	materialBytesBefore := *snapshot.Materials[1].ImageVirtualBytes
	target.Storage.Snapshots[0].LogicalBytes++
	*target.Materials[1].Candidate.ImageVirtualBytes++
	if snapshot.Storage[0].Snapshots[0] != storageBefore || *snapshot.Materials[1].ImageVirtualBytes != materialBytesBefore {
		t.Fatalf("mutating returned delete target changed input snapshot: storage=%+v material=%+v", snapshot.Storage[0], snapshot.Materials[1])
	}
}

func TestRuntimeDeleteTargetAllowsZeroRevisionManagedRuntime(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	manifest := lifecycleRuntime(id, "draft")
	target, err := RuntimeDeleteTargetFrom(lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{}, []RuntimeMaterialObservation{}), RuntimeRef(id))
	if err != nil || len(target.Materials) != 0 || len(target.Runtime.Revisions) != 0 || len(target.Storage.Snapshots) != 0 {
		t.Fatalf("zero-revision delete target = %+v/%v", target, err)
	}
}

func TestRuntimeDeleteTargetRejectsCrossRuntimeLifecycleAuthority(t *testing.T) {
	targetID := "018bcfe5-687b-7000-8000-000000000077"
	otherID := "018bcfe5-687b-7000-8000-000000000099"
	otherRevision := "sha256:" + strings.Repeat("9", 64)
	target := lifecycleRuntime(targetID, "frontend")
	other := lifecycleRuntime(otherID, "other")

	t.Run("active build", func(t *testing.T) {
		snapshot := lifecycleSnapshot([]RuntimeManifest{target, other}, []RuntimeProtection{}, []RuntimeMaterialObservation{})
		snapshot.Journals.Active = []RuntimeLifecycleActivity{{Kind: RuntimeLifecycleActivityBuild, RuntimeID: otherID, Revisions: []string{otherRevision}}}
		if _, err := RuntimeDeleteTargetFrom(snapshot, RuntimeRef(targetID)); !errors.Is(err, ErrRuntimeLifecycleActive) {
			t.Fatalf("cross-Runtime active build fault = %v", err)
		}
	})

	t.Run("settled failed build", func(t *testing.T) {
		material := RuntimeMaterialObservation{RuntimeID: otherID, Revision: otherRevision, TagRole: RuntimeMaterialTagJournaledStaging, Availability: RuntimeAvailabilityPruned, ObservationComplete: true}
		snapshot := lifecycleSnapshot([]RuntimeManifest{target, other}, []RuntimeProtection{}, []RuntimeMaterialObservation{})
		snapshot.Journals.FailedBuilds = []RuntimeFailedBuildArtifact{{RuntimeID: otherID, Revision: otherRevision, RuntimeRef: RuntimeRef(otherID), Name: other.Name, Material: material}}
		snapshot.Storage[1].Snapshots = []RuntimeSnapshotStorage{{Kind: RuntimePruneCandidateFailedBuild, Revision: otherRevision, SemanticFingerprint: otherRevision, LogicalBytes: 1}}
		if _, err := RuntimeDeleteTargetFrom(snapshot, RuntimeRef(targetID)); !errors.Is(err, ErrRuntimeLifecycleActive) {
			t.Fatalf("cross-Runtime failed-build authority fault = %v", err)
		}
	})
}

func TestRuntimeDeleteTargetFailsClosedOnProtectionUseAndUnknownEvidence(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	manifest := lifecycleRuntime(id, "frontend", revision)
	available := RuntimeMaterialObservation{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityAvailable, TagPresent: true, ContentPresent: true, OwnershipVerified: true, ObservationComplete: true}
	base := lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{}, []RuntimeMaterialObservation{available})
	protection := RuntimeProtection{RuntimeID: id, RuntimeRevision: revision, Reason: RuntimeProtectedByTemplateCurrent, WorkspaceTemplateID: "018bcfe5-687b-7000-8000-000000000088", TemplateRevision: SemanticDigest("sha256:" + strings.Repeat("d", 64))}

	tests := []struct {
		name string
		edit func(*RuntimeLifecycleSnapshot)
		want error
	}{
		{name: "current Template protection", edit: func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Protection.Items = []RuntimeProtection{protection} }, want: ErrRuntimeDeleteProtected},
		{name: "Workspace applied protection", edit: func(snapshot *RuntimeLifecycleSnapshot) {
			protection.Reason = RuntimeProtectedByWorkspaceApplied
			protection.ContextID = "018bcfe5-687b-7000-8000-000000000098"
			protection.WorkspaceID = "018bcfe5-687b-7000-8000-000000000099"
			snapshot.Protection.Items = []RuntimeProtection{protection}
		}, want: ErrRuntimeDeleteProtected},
		{name: "Workspace container use", edit: func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Materials[0].WorkspaceInUse = true }, want: ErrRuntimeDeleteProtected},
		{name: "external container use", edit: func(snapshot *RuntimeLifecycleSnapshot) { snapshot.Materials[0].ExternalInUse = true }, want: ErrRuntimeDeleteProtected},
		{name: "unknown material", edit: func(snapshot *RuntimeLifecycleSnapshot) {
			snapshot.Materials[0] = RuntimeMaterialObservation{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityUnknown, ObservationComplete: true}
		}, want: ErrRuntimeRetirementObservationUnknown},
		{name: "mismatched material", edit: func(snapshot *RuntimeLifecycleSnapshot) {
			snapshot.Materials[0] = RuntimeMaterialObservation{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityMismatched, TagPresent: true, ObservationComplete: true}
		}, want: ErrRuntimeRetirementObservationUnknown},
		{name: "active restore", edit: func(snapshot *RuntimeLifecycleSnapshot) {
			snapshot.Journals.Active = []RuntimeLifecycleActivity{{Kind: RuntimeLifecycleActivityRestore, RuntimeID: id, Revisions: []string{revision}}}
		}, want: ErrRuntimeLifecycleActive},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := base
			snapshot.Materials = append([]RuntimeMaterialObservation{}, base.Materials...)
			snapshot.Protection.Items = append([]RuntimeProtection{}, base.Protection.Items...)
			snapshot.Journals.Active = append([]RuntimeLifecycleActivity{}, base.Journals.Active...)
			test.edit(&snapshot)
			_, err := RuntimeDeleteTargetFrom(snapshot, RuntimeRef(id))
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}

	if _, err := RuntimeDeleteTargetFrom(base, StandardRuntimeID); !errors.Is(err, ErrRuntimeDeleteProtected) {
		t.Fatalf("standard delete error = %v", err)
	}
	if _, err := RuntimeDeleteTargetFrom(base, "frontend"); err == nil {
		t.Fatal("mutable Runtime name was accepted as delete authority")
	}
}

func TestRuntimeDeleteTargetRejectsEveryProtectionOwnerKind(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	templateRevision := SemanticDigest("sha256:" + strings.Repeat("d", 64))
	templateID := WorkspaceTemplateID("018bcfe5-687b-7000-8000-000000000088")
	contextID := ContextID("018bcfe5-687b-7000-8000-000000000098")
	workspaceID := WorkspaceID("018bcfe5-687b-7000-8000-000000000099")
	manifest := lifecycleRuntime(id, "frontend", revision)
	material := RuntimeMaterialObservation{RuntimeID: id, Revision: revision, TagRole: RuntimeMaterialTagPublishedRevision, Availability: RuntimeAvailabilityMissing, ObservationComplete: true}

	for _, reason := range []RuntimeProtectionReason{
		RuntimeProtectedByTemplateCurrent,
		RuntimeProtectedByTemplateRetained,
		RuntimeProtectedByContextDesired,
		RuntimeProtectedByWorkspaceApplied,
		RuntimeProtectedByWorkspacePending,
		RuntimeProtectedByWorkspaceObserved,
	} {
		t.Run(string(reason), func(t *testing.T) {
			protection := RuntimeProtection{RuntimeID: id, RuntimeRevision: revision, Reason: reason, WorkspaceTemplateID: templateID, TemplateRevision: templateRevision}
			if reason == RuntimeProtectedByContextDesired || reason == RuntimeProtectedByWorkspaceApplied || reason == RuntimeProtectedByWorkspacePending || reason == RuntimeProtectedByWorkspaceObserved {
				protection.ContextID = contextID
			}
			if reason == RuntimeProtectedByWorkspaceApplied || reason == RuntimeProtectedByWorkspacePending || reason == RuntimeProtectedByWorkspaceObserved {
				protection.WorkspaceID = workspaceID
			}
			snapshot := lifecycleSnapshot([]RuntimeManifest{manifest}, []RuntimeProtection{protection}, []RuntimeMaterialObservation{material})
			if _, err := RuntimeDeleteTargetFrom(snapshot, RuntimeRef(id)); !errors.Is(err, ErrRuntimeDeleteProtected) {
				t.Fatalf("protection %q error = %v", reason, err)
			}
		})
	}
}

func TestRuntimeDeleteResultRequiresExactPreservationAndTotals(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	item := RuntimePruneItemResult{
		Kind: RuntimePruneCandidateRevision, RuntimeID: id, Revision: revision, RuntimeRef: RuntimeRef(id), RevisionRef: RuntimeRevisionRef(id, revision),
		Name: "frontend", Ordinal: 1, LastUsed: RuntimeLastUsedUnknown, SourceLogicalBytes: 100, SnapshotLogicalBytes: 200,
		Disposition: RuntimePrunePreservedShared, RemovedTagCount: 1,
	}
	valid := RuntimeDeleteResult{
		Task: TaskRuntimeDelete, RuntimeID: id, RuntimeRef: RuntimeRef(id), Name: "frontend", State: RuntimeDeleted,
		SourceLogicalBytes: 100, SnapshotLogicalBytes: 200, SourceDisposition: RuntimeDeleteAuthorityRemoved,
		SnapshotsDisposition: RuntimeDeleteAuthorityRemoved, HistoryDisposition: RuntimeDeleteAuthorityRemoved,
		Items: []RuntimePruneItemResult{item}, RemovedTagCount: 1, ReceiptRevision: 1,
		WorkspaceManifestsPreserved: true, WorkspacesPreserved: true, WorkspaceIDsPreserved: true,
		WorkspaceHomesPreserved: true, AppliedReceiptsPreserved: true, ProjectRootsPreserved: true,
		CredentialsPreserved: true, SharedResourcesPreserved: true,
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	replayed := valid
	replayed.State = RuntimeAlreadyDeleted
	if err := replayed.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := []RuntimeDeleteResult{valid, valid, valid, valid, valid, valid}
	invalid[0].WorkspaceHomesPreserved = false
	invalid[1].AppliedReceiptsPreserved = false
	invalid[2].ProjectRootsPreserved = false
	invalid[3].RemovedTagCount = 0
	invalid[4].SnapshotLogicalBytes = 199
	invalid[5].Items = nil
	for _, result := range invalid {
		if err := result.Validate(); err == nil {
			t.Fatalf("invalid delete result passed: %+v", result)
		}
	}
}
