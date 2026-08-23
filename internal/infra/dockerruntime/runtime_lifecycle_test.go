package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type lifecycleImageFixture struct {
	observation runtimeImageObservation
	missing     bool
}

type lifecycleObservationRunner struct {
	images            map[string]lifecycleImageFixture
	containers        map[string]runtimeContainerObservation
	containerList     string
	outputs           []runnerCall
	changeSecondImage bool
	imageObservations int
}

type blockingLifecycleRunner struct{}

func (blockingLifecycleRunner) Run(ctx context.Context, _ []string, _ []string, _ io.Reader, _ io.Writer, _ io.Writer) error {
	<-ctx.Done()
	return ctx.Err()
}

func (blockingLifecycleRunner) Output(ctx context.Context, _ []string, _ []string) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func lifecycleTestBudget() *runtimeLifecycleBudget {
	return &runtimeLifecycleBudget{remaining: runtimeLifecycleCallBudget}
}

func (r *lifecycleObservationRunner) Run(_ context.Context, args, _ []string, _ io.Reader, stdout, stderr io.Writer) error {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) >= 2 && args[0] == "container" && args[1] == "ls" {
		_, err := io.WriteString(stdout, r.containerList)
		return err
	}
	if len(args) >= 5 && args[0] == "container" && args[1] == "inspect" {
		observed, ok := r.containers[args[4]]
		if !ok {
			_, _ = io.WriteString(stderr, "Error response from daemon: No such container: "+args[4])
			return errors.New("container missing")
		}
		encoded, err := json.Marshal(observed)
		if err != nil {
			return err
		}
		_, err = stdout.Write(encoded)
		return err
	}
	if len(args) >= 5 && args[0] == "image" && args[1] == "inspect" {
		fixture, ok := r.images[args[4]]
		if !ok || fixture.missing {
			_, _ = io.WriteString(stderr, "Error response from daemon: No such image: "+args[4])
			return errors.New("image missing")
		}
		r.imageObservations++
		observed := fixture.observation
		if r.changeSecondImage && r.imageObservations > 1 {
			observed.Size++
		}
		encoded, err := json.Marshal(observed)
		if err != nil {
			return err
		}
		_, err = stdout.Write(encoded)
		return err
	}
	return fmt.Errorf("unexpected lifecycle observation: %v", args)
}

func (r *lifecycleObservationRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) >= 2 && args[0] == "container" && args[1] == "ls" {
		return []byte(r.containerList), nil
	}
	if len(args) >= 5 && args[0] == "container" && args[1] == "inspect" {
		observed, ok := r.containers[args[4]]
		if !ok {
			return []byte("No such container"), errors.New("container missing")
		}
		return json.Marshal(observed)
	}
	if len(args) >= 5 && args[0] == "image" && args[1] == "inspect" {
		fixture, ok := r.images[args[4]]
		if !ok || fixture.missing {
			return []byte("No such image"), errors.New("image missing")
		}
		r.imageObservations++
		observed := fixture.observation
		if r.changeSecondImage && r.imageObservations > 1 {
			observed.Size++
		}
		return json.Marshal(observed)
	}
	return nil, fmt.Errorf("unexpected lifecycle observation: %v", args)
}

func installRuntimeLifecycleRevision(t *testing.T, runtime *Runtime, id, name, revision, image, imageDigest string) tobari.RuntimeManifest {
	t.Helper()
	root := runtime.runtimeDirectory(name)
	if err := os.MkdirAll(filepath.Join(root, "source"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "revisions", strings.TrimPrefix(revision, "sha256:"), "source"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "Dockerfile"), []byte("FROM example.invalid/runtime\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "revisions", strings.TrimPrefix(revision, "sha256:"), "source", "Dockerfile"), []byte("FROM example.invalid/runtime\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	manifest := tobari.RuntimeManifest{SchemaVersion: tobari.RuntimeSchemaVersion, ID: id, Name: name, Kind: tobari.RuntimeKindManaged, SourcePath: filepath.Join(root, "source"), Revisions: []tobari.RuntimeRevision{{Ordinal: 1, Revision: revision, Image: image, ImageDigest: imageDigest, CreatedAt: time.Unix(1, 0).UTC(), SnapshotPath: filepath.Join(root, "revisions", strings.TrimPrefix(revision, "sha256:"), "source")}}}
	if err := writeAtomicJSON(filepath.Join(root, "runtime.json"), manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func managedLifecycleImage(id, revision, tag string) runtimeImageObservation {
	return runtimeImageObservation{ID: "sha256:" + strings.Repeat("b", 64), Size: 4096, RepoTags: []string{tag}, Owner: ownerValue, Component: managedRuntimeComponentLabel, RuntimeID: id, Revision: revision}
}

func TestRuntimeLifecycleSnapshotIsZeroWriteAndRequiresStableDockerEvidence(t *testing.T) {
	root := t.TempDir()
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, observedAt, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil || observedAt.IsZero() || observedAt.Location() != time.UTC || !snapshot.CatalogComplete || len(snapshot.Runtimes) != 1 || snapshot.Runtimes[0].ID != tobari.StandardRuntimeID {
		t.Fatalf("fresh lifecycle snapshot = %+v/%v", snapshot, err)
	}
	for _, path := range []string{filepath.Join(root, "config"), filepath.Join(root, "state")} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read-only lifecycle snapshot created %s: %v", path, err)
		}
	}

	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	imageDigest := "sha256:" + strings.Repeat("b", 64)
	tag := "tobari-runtime-frontend:aaaaaaaaaaaa"
	if err := os.MkdirAll(filepath.Join(root, "config", "runtimes"), 0o700); err != nil {
		t.Fatal(err)
	}
	installRuntimeLifecycleRevision(t, runtime, id, "frontend", revision, tag, imageDigest)
	runner.images[tag] = lifecycleImageFixture{observation: managedLifecycleImage(id, revision, tag)}
	runner.changeSecondImage = true
	if _, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background()); err == nil {
		t.Fatal("drifting Docker evidence produced a coherent snapshot")
	}
}

func TestRuntimeLifecycleObservationHasGlobalWallAndCallBudgets(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), blockingLifecycleRunner{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, _, err := runtime.ReadRuntimeLifecycleSnapshot(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lifecycle wall budget cancellation = %v", err)
	}

	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	budget := &runtimeLifecycleBudget{remaining: 1}
	args := []string{"container", "ls", "--all", "--no-trunc", "--format", "{{.ID}}"}
	if _, _, err := budget.run(context.Background(), runner, args, nil, maxRuntimeLifecycleListBytes); err != nil {
		t.Fatal(err)
	}
	if _, _, err := budget.run(context.Background(), runner, args, nil, maxRuntimeLifecycleListBytes); err == nil || len(runner.outputs) != 1 {
		t.Fatalf("lifecycle call budget error/calls = %v/%d", err, len(runner.outputs))
	}
}

func TestRuntimeLifecycleSnapshotRejectsUnknownCatalogBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.runtimesDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.runtimesDirectory(), "unknown"), []byte("unsafe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background()); err == nil {
		t.Fatal("unknown Runtime catalog child was skipped")
	}
	if len(runner.outputs) != 0 {
		t.Fatalf("invalid catalog crossed Docker observation: %v", runner.outputs)
	}
}

func TestRuntimeLifecycleSnapshotRejectsUnknownJournalBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.WithLifecycleLock(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.runtimeLifecycleDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.runtimeLifecycleDirectory(), "unknown"), []byte("unsafe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background()); err == nil {
		t.Fatal("unknown lifecycle journal child was skipped")
	}
	if len(runner.outputs) != 0 {
		t.Fatalf("invalid journal crossed Docker observation: %v", runner.outputs)
	}
}

func TestRuntimeLifecycleBoundsCatalogBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index <= maxRuntimeLifecycleRuntimes; index++ {
		if err := os.MkdirAll(filepath.Join(runtime.runtimesDirectory(), fmt.Sprintf("runtime-%03d", index)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background()); err == nil {
		t.Fatal("unbounded Runtime catalog crossed lifecycle observation")
	}
	if len(runner.outputs) != 0 {
		t.Fatalf("unbounded catalog crossed Docker observation: %v", runner.outputs)
	}
}

func TestRuntimeMaterialObservesDigestUseWhenExpectedTagIsMissing(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	digest := "sha256:" + strings.Repeat("b", 64)
	tag := "tobari-runtime-frontend:aaaaaaaaaaaa"
	foreign := "example.invalid/shared:fixture"
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{
		tag:    {missing: true},
		digest: {observation: runtimeImageObservation{ID: digest, Size: 2048, RepoTags: []string{foreign}, Owner: ownerValue, Component: managedRuntimeComponentLabel, RuntimeID: id, Revision: revision}},
	}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	target := runtimeMaterialTarget{RuntimeID: id, Revision: revision, TagRole: tobari.RuntimeMaterialTagPublishedRevision, Selector: tag, RecordedDigest: digest, Name: "frontend"}
	material, err := runtime.observeRuntimeMaterial(context.Background(), target, map[string]runtimeContentUse{digest: {workspace: true}}, lifecycleTestBudget())
	if err != nil || material.Availability != tobari.RuntimeAvailabilityMissing || material.TagPresent || !material.ContentPresent || !material.SharedContent || !material.WorkspaceInUse || !material.OwnershipVerified {
		t.Fatalf("tag-missing used shared content = %+v/%v", material, err)
	}

	material, err = runtime.observeRuntimeMaterial(context.Background(), target, map[string]runtimeContentUse{}, lifecycleTestBudget())
	if err != nil || !material.ContentPresent || !material.SharedContent || material.WorkspaceInUse || material.ExternalInUse {
		t.Fatalf("tag-missing shared content = %+v/%v", material, err)
	}
}

func TestRuntimeLifecycleContainerListRejectsOpaqueSelectors(t *testing.T) {
	runner := &lifecycleObservationRunner{containerList: strings.Repeat("a", 63) + " bad\n", images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.observeRuntimeContainerUse(context.Background(), map[string]runtimeWorkspaceContainerAuthority{}, map[string]string{}, lifecycleTestBudget()); err == nil {
		t.Fatal("non-canonical container selector was accepted")
	}
	if len(runner.outputs) != 1 {
		t.Fatalf("untrusted selector crossed container inspect: %v", runner.outputs)
	}
}

func TestRuntimeLifecycleContainerUseRequiresExactWorkspaceAuthority(t *testing.T) {
	containerID := strings.Repeat("a", 64)
	digest := "sha256:" + strings.Repeat("b", 64)
	runtimeID := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("c", 64)
	workspaceID := "018bcfe5-687b-7000-8000-000000000088"
	runner := &lifecycleObservationRunner{
		containerList: containerID + "\n",
		images:        map[string]lifecycleImageFixture{},
		containers: map[string]runtimeContainerObservation{
			containerID: {ID: containerID, Image: digest, Owner: ownerValue, Component: "tobari", Workspace: workspaceID, Role: projectWorkRole, Spec: "sha256:" + strings.Repeat("d", 64)},
		},
	}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	authority := runtimeWorkspaceContainerAuthority{ContainerID: containerID, WorkspaceID: workspaceID, ResolvedSpec: runner.containers[containerID].Spec, RuntimeID: runtimeID, Revision: revision}
	uses, err := runtime.observeRuntimeContainerUse(context.Background(), map[string]runtimeWorkspaceContainerAuthority{containerID: authority}, map[string]string{runtimeID + "\x00" + revision: digest}, lifecycleTestBudget())
	if err != nil || !uses[digest].workspace || uses[digest].external {
		t.Fatalf("exact Workspace authority = %+v/%v", uses, err)
	}

	runner.outputs = nil
	spoofed := runner.containers[containerID]
	spoofed.Spec = "sha256:" + strings.Repeat("e", 64)
	runner.containers[containerID] = spoofed
	if _, err := runtime.observeRuntimeContainerUse(context.Background(), map[string]runtimeWorkspaceContainerAuthority{containerID: authority}, map[string]string{runtimeID + "\x00" + revision: digest}, lifecycleTestBudget()); err == nil {
		t.Fatal("Workspace-labeled container without exact stored authority was trusted")
	}

	runner.outputs = nil
	uses, err = runtime.observeRuntimeContainerUse(context.Background(), map[string]runtimeWorkspaceContainerAuthority{}, map[string]string{}, lifecycleTestBudget())
	if err != nil || uses[digest].workspace || !uses[digest].external {
		t.Fatalf("unjoined container authority = %+v/%v", uses, err)
	}
}

func TestRuntimeMaterialWithoutTrustedBuildLabelsIsMigrationUnverified(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	digest := "sha256:" + strings.Repeat("b", 64)
	tag := "tobari-runtime-frontend:aaaaaaaaaaaa"
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{
		tag: {observation: runtimeImageObservation{ID: digest, Size: 2048, RepoTags: []string{tag}}},
	}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	target := runtimeMaterialTarget{RuntimeID: id, Revision: revision, TagRole: tobari.RuntimeMaterialTagPublishedRevision, Selector: tag, RecordedDigest: digest, Name: "frontend"}
	material, err := runtime.observeRuntimeMaterial(context.Background(), target, map[string]runtimeContentUse{}, lifecycleTestBudget())
	if err != nil || material.Availability != tobari.RuntimeAvailabilityUnknown || !material.MigrationUnverified || material.OwnershipVerified {
		t.Fatalf("unlabeled Runtime material = %+v/%v", material, err)
	}
}

func TestRuntimeLifecycleSnapshotMapsFailedBuildStagingEvidence(t *testing.T) {
	root := t.TempDir()
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.runtimesDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	installRuntimeLifecycleRevision(t, runtime, id, "frontend", "sha256:"+strings.Repeat("c", 64), "tobari-runtime-frontend:cccccccccccc", "sha256:"+strings.Repeat("d", 64))
	var journal runtimeBuildJournal
	if err := runtime.WithLifecycleLock(context.Background(), func(context.Context) error {
		created, err := runtime.beginRuntimeBuildJournal(context.Background(), id, "frontend")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(created.SnapshotPath, 0o700); err != nil {
			return err
		}
		prepared := created
		prepared.Phase = runtimeBuildPhasePrepared
		prepared.Revision = revision
		prepared.StagingImage = managedRuntimeStagingImage(id, revision)
		prepared.FinalImage = managedLibraryRuntimeImage("frontend", id, revision)
		if err := runtime.writeRuntimeBuildJournal(created, prepared); err != nil {
			return err
		}
		building := prepared
		building.Phase = runtimeBuildPhaseBuilding
		building.StagingArtifact = runtimeBuildStagingUnknown
		building.AttemptSettlement = runtimeBuildAttemptUnsettled
		if err := runtime.writeRuntimeBuildJournal(prepared, building); err != nil {
			return err
		}
		journal = building
		journal.Phase = runtimeBuildPhaseFailed
		journal.StagingArtifact = runtimeBuildStagingOwned
		journal.AttemptSettlement = runtimeBuildAttemptSettled
		journal.ImageDigest = "sha256:" + strings.Repeat("e", 64)
		return runtime.writeRuntimeBuildJournal(building, journal)
	}); err != nil {
		t.Fatal(err)
	}
	runner.images[journal.StagingImage] = lifecycleImageFixture{observation: runtimeImageObservation{ID: "sha256:" + strings.Repeat("e", 64), Size: 1024, RepoTags: []string{journal.StagingImage}, Owner: ownerValue, Component: managedRuntimeComponentLabel, RuntimeID: id, Revision: revision}}
	published := "tobari-runtime-frontend:cccccccccccc"
	runner.images[published] = lifecycleImageFixture{observation: runtimeImageObservation{ID: "sha256:" + strings.Repeat("d", 64), Size: 2048, RepoTags: []string{published}, Owner: ownerValue, Component: managedRuntimeComponentLabel, RuntimeID: id, Revision: "sha256:" + strings.Repeat("c", 64)}}

	snapshot, observedAt, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil || len(snapshot.Journals.FailedBuilds) != 1 || snapshot.Journals.FailedBuilds[0].Material.TagRole != tobari.RuntimeMaterialTagJournaledStaging || len(snapshot.Journals.Active) != 0 {
		t.Fatalf("failed build lifecycle snapshot = %+v/%v", snapshot, err)
	}
	if observedAt.IsZero() || observedAt.Location() != time.UTC {
		t.Fatalf("failed build observation time = %v", observedAt)
	}
}

func TestReadRuntimeBuildRecoveryPublishesStableManagedReference(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), newManagedRuntimeBuildRunner())
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.beginRuntimeBuildJournal(context.Background(), created.Runtime.ID, created.Runtime.Name); err != nil {
		t.Fatal(err)
	}
	recovery, found, err := runtime.ReadRuntimeBuildRecovery(context.Background())
	want := tobari.RuntimeBuildRecovery{RuntimeID: created.Runtime.ID, RuntimeRef: tobari.RuntimeRef(created.Runtime.ID), Name: created.Runtime.Name, Kind: tobari.RuntimeBuildRecoveryPreDocker}
	if err != nil || !found || recovery != want {
		t.Fatalf("Runtime build recovery = %+v/%t/%v", recovery, found, err)
	}
}

func TestRuntimeLifecycleSnapshotKeepsEveryBuildRecoveryPhaseAsBlocker(t *testing.T) {
	for _, test := range []struct {
		name        string
		phase       string
		cleanupFrom string
		snapshot    bool
		final       bool
		manifest    bool
		orphan      string
		staging     string
		settlement  string
	}{
		{name: "prepared", phase: runtimeBuildPhasePrepared, snapshot: true},
		{name: "building", phase: runtimeBuildPhaseBuilding, snapshot: true, staging: runtimeBuildStagingUnknown, settlement: runtimeBuildAttemptUnsettled},
		{name: "orphan exact", phase: runtimeBuildPhaseOrphanStaging, snapshot: true, orphan: runtimeBuildOrphanExactManaged},
		{name: "orphan unknown", phase: runtimeBuildPhaseOrphanStaging, snapshot: true, orphan: runtimeBuildOrphanUnknown},
		{name: "orphan absent", phase: runtimeBuildPhaseOrphanStaging, snapshot: true, orphan: runtimeBuildOrphanAbsent},
		{name: "built", phase: runtimeBuildPhaseBuilt, snapshot: true, staging: runtimeBuildStagingOwned, settlement: runtimeBuildAttemptSettled},
		{name: "final tagged", phase: runtimeBuildPhaseFinalTagged, snapshot: true, staging: runtimeBuildStagingOwned, settlement: runtimeBuildAttemptSettled},
		{name: "staging released", phase: runtimeBuildPhaseStagingReleased, snapshot: true, staging: runtimeBuildStagingOwned, settlement: runtimeBuildAttemptSettled},
		{name: "snapshot published", phase: runtimeBuildPhaseSnapshotPublished, final: true, staging: runtimeBuildStagingOwned, settlement: runtimeBuildAttemptSettled},
		{name: "manifest committed", phase: runtimeBuildPhaseManifestCommitted, final: true, manifest: true, staging: runtimeBuildStagingOwned, settlement: runtimeBuildAttemptSettled},
		{name: "failed absent", phase: runtimeBuildPhaseFailed, snapshot: true, staging: runtimeBuildStagingAbsent, settlement: runtimeBuildAttemptUnsettled},
		{name: "failed unknown", phase: runtimeBuildPhaseFailed, snapshot: true, staging: runtimeBuildStagingUnknown, settlement: runtimeBuildAttemptUnsettled},
		{name: "completing prepared present", phase: runtimeBuildPhaseCompleting, cleanupFrom: runtimeBuildPhasePrepared, snapshot: true},
		{name: "completing prepared absent", phase: runtimeBuildPhaseCompleting, cleanupFrom: runtimeBuildPhasePrepared},
		{name: "completing failed present", phase: runtimeBuildPhaseCompleting, cleanupFrom: runtimeBuildPhaseFailed, snapshot: true, staging: runtimeBuildStagingAbsent, settlement: runtimeBuildAttemptSettled},
		{name: "completing failed absent", phase: runtimeBuildPhaseCompleting, cleanupFrom: runtimeBuildPhaseFailed, staging: runtimeBuildStagingAbsent, settlement: runtimeBuildAttemptSettled},
		{name: "completing committed", phase: runtimeBuildPhaseCompleting, cleanupFrom: runtimeBuildPhaseManifestCommitted, final: true, manifest: true, staging: runtimeBuildStagingOwned, settlement: runtimeBuildAttemptSettled},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
			if err != nil {
				t.Fatal(err)
			}
			journal, err := runtime.beginRuntimeBuildJournal(context.Background(), created.Runtime.ID, created.Runtime.Name)
			if err != nil {
				t.Fatal(err)
			}
			revision := "sha256:" + strings.Repeat("a", 64)
			journal.Phase = test.phase
			journal.Revision = revision
			journal.StagingImage = managedRuntimeStagingImage(journal.RuntimeID, revision)
			journal.FinalImage = managedLibraryRuntimeImage(journal.RuntimeName, journal.RuntimeID, revision)
			journal.OrphanStaging = test.orphan
			journal.StagingArtifact = test.staging
			journal.AttemptSettlement = test.settlement
			if test.orphan == runtimeBuildOrphanExactManaged || test.staging == runtimeBuildStagingOwned {
				journal.ImageDigest = "sha256:" + strings.Repeat("b", 64)
			}
			if test.phase == runtimeBuildPhaseBuilt || test.phase == runtimeBuildPhaseFinalTagged || test.phase == runtimeBuildPhaseStagingReleased || test.phase == runtimeBuildPhaseSnapshotPublished || test.phase == runtimeBuildPhaseManifestCommitted || test.cleanupFrom == runtimeBuildPhaseManifestCommitted {
				journal.CreatedAt = "2026-01-02T03:04:05Z"
			}
			if test.phase == runtimeBuildPhaseCompleting {
				journal.CleanupFrom = test.cleanupFrom
			}
			if test.snapshot {
				if err := os.MkdirAll(journal.SnapshotPath, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			final := filepath.Join(runtime.runtimeRevisionsDirectory(journal.RuntimeName), strings.TrimPrefix(revision, "sha256:"), "source")
			if test.final {
				if err := os.MkdirAll(final, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if test.manifest {
				manifest, err := runtime.readRuntimeManifest(journal.RuntimeName)
				if err != nil {
					t.Fatal(err)
				}
				manifest.Revisions = []tobari.RuntimeRevision{{Ordinal: 1, Revision: revision, Image: journal.FinalImage, ImageDigest: journal.ImageDigest, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), SnapshotPath: final}}
				if err := writeAtomicJSON(runtime.runtimeManifestPath(manifest.Name), manifest); err != nil {
					t.Fatal(err)
				}
			}
			if err := journal.Validate(runtime); err != nil {
				t.Fatal(err)
			}
			if err := writeAtomicJSON(runtime.runtimeBuildJournalPath(), journal); err != nil {
				t.Fatal(err)
			}
			snapshot, observedAt, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
			if err != nil || len(snapshot.Journals.Active) != 1 || len(snapshot.Journals.FailedBuilds) != 0 {
				t.Fatalf("phase %s lifecycle snapshot = %+v/%v", test.name, snapshot, err)
			}
			plan, err := tobari.PlanRuntimePrune(snapshot, observedAt)
			if err != nil || plan.Applicable {
				t.Fatalf("phase %s prune plan = %+v/%v", test.name, plan, err)
			}
		})
	}
}

func TestRuntimeLifecycleSnapshotRepresentsPartialSnapshottingRecovery(t *testing.T) {
	for _, completing := range []bool{false, true} {
		for _, shape := range []string{"absent", "empty", "source"} {
			t.Run(fmt.Sprintf("completing_%t_%s", completing, shape), func(t *testing.T) {
				root := t.TempDir()
				runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
				runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
				if err != nil {
					t.Fatal(err)
				}
				created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
				if err != nil {
					t.Fatal(err)
				}
				journal, err := runtime.beginRuntimeBuildJournal(context.Background(), created.Runtime.ID, created.Runtime.Name)
				if err != nil {
					t.Fatal(err)
				}
				if completing {
					journal.Phase = runtimeBuildPhaseCompleting
					journal.CleanupFrom = runtimeBuildPhaseSnapshotting
					if err := writeAtomicJSON(runtime.runtimeBuildJournalPath(), journal); err != nil {
						t.Fatal(err)
					}
				}
				switch shape {
				case "empty":
					if err := os.MkdirAll(filepath.Dir(journal.SnapshotPath), 0o700); err != nil {
						t.Fatal(err)
					}
				case "source":
					if err := os.MkdirAll(journal.SnapshotPath, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				recovery, found, err := runtime.ReadRuntimeBuildRecovery(context.Background())
				wantKind := tobari.RuntimeBuildRecoveryPreDocker
				if completing {
					wantKind = tobari.RuntimeBuildRecoveryCleanup
				}
				if err != nil || !found || recovery.Kind != wantKind {
					t.Fatalf("recovery = %+v/%t/%v", recovery, found, err)
				}
				snapshot, observedAt, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
				if err != nil || len(snapshot.Journals.Active) != 1 {
					t.Fatalf("snapshot = %+v/%v", snapshot, err)
				}
				plan, err := tobari.PlanRuntimePrune(snapshot, observedAt)
				if err != nil || plan.Applicable || len(plan.Blockers) != 1 || plan.Blockers[0].Reason != tobari.RuntimeBlockedByActiveBuild {
					t.Fatalf("plan = %+v/%v", plan, err)
				}
			})
		}
	}
}

func TestRuntimeLifecycleSnapshotRejectsInvalidPartialSnapshottingInventory(t *testing.T) {
	for _, shape := range []string{"file", "extra", "source_symlink"} {
		t.Run(shape, func(t *testing.T) {
			root := t.TempDir()
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &lifecycleObservationRunner{})
			if err != nil {
				t.Fatal(err)
			}
			created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
			if err != nil {
				t.Fatal(err)
			}
			journal, err := runtime.beginRuntimeBuildJournal(context.Background(), created.Runtime.ID, created.Runtime.Name)
			if err != nil {
				t.Fatal(err)
			}
			parent := filepath.Dir(journal.SnapshotPath)
			if err := os.MkdirAll(parent, 0o700); err != nil {
				t.Fatal(err)
			}
			switch shape {
			case "file":
				if err := os.WriteFile(filepath.Join(parent, "unexpected"), []byte("x"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "extra":
				if err := os.Mkdir(filepath.Join(parent, "unexpected"), 0o700); err != nil {
					t.Fatal(err)
				}
			case "source_symlink":
				target := t.TempDir()
				if err := os.Symlink(target, journal.SnapshotPath); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background()); err == nil {
				t.Fatal("unsafe partial snapshotting inventory was accepted")
			}
		})
	}
}
