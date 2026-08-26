package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	containerLists    map[string]string
	outputs           []runnerCall
	changeSecondImage bool
	imageObservations int
	onFirstImage      func()
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
		listing := r.containerList
		for _, arg := range args {
			if strings.HasPrefix(arg, "ancestor=") && r.containerLists != nil {
				listing = r.containerLists[strings.TrimPrefix(arg, "ancestor=")]
			}
		}
		_, err := io.WriteString(stdout, listing)
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
		if strings.Contains(args[3], tobari.RuntimeImageAPILabel) {
			_, err := stdout.Write(compatibleImageInspection())
			return err
		}
		fixture, ok := r.images[args[4]]
		if !ok || fixture.missing {
			_, _ = io.WriteString(stderr, "Error response from daemon: No such image: "+args[4])
			return errors.New("image missing")
		}
		r.imageObservations++
		if r.imageObservations == 1 && r.onFirstImage != nil {
			r.onFirstImage()
		}
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
		listing := r.containerList
		for _, arg := range args {
			if strings.HasPrefix(arg, "ancestor=") && r.containerLists != nil {
				listing = r.containerLists[strings.TrimPrefix(arg, "ancestor=")]
			}
		}
		return []byte(listing), nil
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

func runtimeLifecycleFixtureRevision(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	revision, err := digestRuntimeSnapshot(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func installRuntimeLifecycleRevision(t *testing.T, runtime *Runtime, id, name, imageDigest, content string) tobari.RuntimeManifest {
	t.Helper()
	if err := os.MkdirAll(runtime.stateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(runtime.stateDirectory, "lifecycle.lock")
	if _, err := os.Lstat(lockPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(lockPath, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.finalRuntimeProtectionSource == nil {
		bindEmptyFinalRuntimeProtection(t, runtime)
	}
	revision := runtimeLifecycleFixtureRevision(t, content)
	image := managedLibraryRuntimeImage(name, id, revision)
	root := runtime.runtimeDirectory(id)
	if err := os.MkdirAll(filepath.Join(root, "source"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeRuntimeSourceMetadata(runtime.runtimeMetadataPath(id), runtimeSourceMetadata{SchemaVersion: 1, RuntimeID: id, Name: name}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(runtime.runtimeRevisionsDirectory(id), strings.TrimPrefix(revision, "sha256:"), "source"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source", "Dockerfile"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.runtimeRevisionsDirectory(id), strings.TrimPrefix(revision, "sha256:"), "source", "Dockerfile"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := tobari.RuntimeManifest{SchemaVersion: tobari.RuntimeSchemaVersion, ID: id, Name: name, Kind: tobari.RuntimeKindManaged, SourcePath: filepath.Join(root, "source"), Revisions: []tobari.RuntimeRevision{{Ordinal: 1, Revision: revision, Image: image, ImageDigest: imageDigest, CreatedAt: time.Unix(1, 0).UTC(), SnapshotPath: filepath.Join(runtime.runtimeRevisionsDirectory(id), strings.TrimPrefix(revision, "sha256:"), "source")}}}
	if err := writeAtomicJSON(runtime.runtimeManifestPath(id), manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.readRuntimeManifestByID(id); err != nil {
		t.Fatalf("installed split Runtime fixture is unreadable: %v", err)
	}
	budget := runtimeLifecycleBudget{remaining: runtimeLifecycleCallBudget}
	if _, err := runtime.readRuntimeLifecycleLocalObserved(context.Background(), &budget); err != nil {
		t.Fatalf("installed split Runtime local observation failed: %v", err)
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
	bindEmptyFinalRuntimeProtection(t, runtime)
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
	imageDigest := "sha256:" + strings.Repeat("b", 64)
	if err := os.MkdirAll(filepath.Join(root, "config", "runtimes"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := installRuntimeLifecycleRevision(t, runtime, id, "frontend", imageDigest, "FROM example.invalid/runtime\n")
	revision := manifest.Revisions[0].Revision
	tag := manifest.Revisions[0].Image
	runner.images[tag] = lifecycleImageFixture{observation: managedLifecycleImage(id, revision, tag)}
	runner.changeSecondImage = true
	if _, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background()); err == nil {
		t.Fatal("drifting Docker evidence produced a coherent snapshot")
	}
}

func TestRuntimeLifecycleSnapshotMeasuresExactLogicalStorageAndRejectsDrift(t *testing.T) {
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
	imageDigest := "sha256:" + strings.Repeat("b", 64)
	manifest := installRuntimeLifecycleRevision(t, runtime, id, "frontend", imageDigest, "FROM example.invalid/runtime\n")
	revision := manifest.Revisions[0].Revision
	tag := manifest.Revisions[0].Image
	runner.images[tag] = lifecycleImageFixture{observation: managedLifecycleImage(id, revision, tag)}
	sourceInfo, err := os.Stat(filepath.Join(manifest.SourcePath, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	snapshotInfo, err := os.Stat(filepath.Join(manifest.Revisions[0].SnapshotPath, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil || len(snapshot.Storage) != 1 || snapshot.Storage[0].SourceLogicalBytes != sourceInfo.Size() || len(snapshot.Storage[0].Snapshots) != 1 || snapshot.Storage[0].Snapshots[0].LogicalBytes != snapshotInfo.Size() {
		t.Fatalf("logical storage = %+v/%v", snapshot.Storage, err)
	}

	runner.imageObservations = 0
	runner.onFirstImage = func() {
		if writeErr := os.WriteFile(filepath.Join(manifest.SourcePath, "Dockerfile"), []byte("FROM example.invalid/runtime\nRUN true\n"), 0o600); writeErr != nil {
			t.Errorf("mutate Runtime source: %v", writeErr)
		}
	}
	if _, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background()); err == nil {
		t.Fatal("source storage drift produced a coherent lifecycle snapshot")
	}
}

func TestObserveRuntimeTreeLogicalBytesRejectsUnsafeOrUnboundedTrees(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := observeRuntimeTreeLogicalBytes(context.Background(), root); err == nil {
		t.Fatal("empty Runtime tree was measured")
	}
	if err := os.WriteFile(filepath.Join(root, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := observeRuntimeTreeLogicalBytes(context.Background(), root); err != nil {
		t.Fatalf("private bounded Runtime tree: %v", err)
	}
	if err := os.Symlink("Dockerfile", filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := observeRuntimeTreeLogicalBytes(context.Background(), root); err == nil {
		t.Fatal("Runtime storage symlink was measured")
	}
}

func TestRuntimeLifecycleSnapshotRehashesEveryImmutableRevisionTree(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{name: "nested symlink", mutate: func(t *testing.T, snapshot string) {
			if err := os.Mkdir(filepath.Join(snapshot, "nested"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("../Dockerfile", filepath.Join(snapshot, "nested", "link")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "nested broad mode", mutate: func(t *testing.T, snapshot string) {
			path := filepath.Join(snapshot, "nested")
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "content drift", mutate: func(t *testing.T, snapshot string) {
			if err := os.WriteFile(filepath.Join(snapshot, "Dockerfile"), []byte("FROM example.invalid/drifted\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "file count", mutate: func(t *testing.T, snapshot string) {
			for index := 0; index < maxRuntimeSourceFiles; index++ {
				if err := os.WriteFile(filepath.Join(snapshot, fmt.Sprintf("extra-%04d", index)), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}},
		{name: "file size", mutate: func(t *testing.T, snapshot string) {
			path := filepath.Join(snapshot, "oversized")
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Truncate(path, maxRuntimeSourceFile+1); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			manifest := installRuntimeLifecycleRevision(t, runtime, "018bcfe5-687b-7000-8000-000000000077", "frontend", "sha256:"+strings.Repeat("b", 64), "FROM example.invalid/runtime\n")
			test.mutate(t, manifest.Revisions[0].SnapshotPath)
			_, _, err = runtime.ReadRuntimeLifecycleSnapshot(context.Background())
			var fault tobari.RuntimeProtectionInventoryError
			if !errors.As(err, &fault) || fault.Reason != tobari.RuntimeProtectionInventoryIncomplete {
				t.Fatalf("unsafe immutable snapshot error = %v", err)
			}
			if len(runner.outputs) != 0 {
				t.Fatalf("unsafe immutable snapshot crossed Docker observation: %v", runner.outputs)
			}
		})
	}
}

func TestRuntimeLifecycleObservationHasGlobalWallAndCallBudgets(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), blockingLifecycleRunner{})
	if err != nil {
		t.Fatal(err)
	}
	id := "018bcfe5-687b-7000-8000-000000000077"
	installRuntimeLifecycleRevision(t, runtime, id, "frontend", "sha256:"+strings.Repeat("b", 64), "FROM example.invalid/runtime\n")
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

func TestRuntimeLifecycleSnapshotRejectsPersistedNonCanonicalImageSelectorBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	id := "018bcfe5-687b-7000-8000-000000000077"
	manifest := installRuntimeLifecycleRevision(t, runtime, id, "frontend", "sha256:"+strings.Repeat("b", 64), "FROM example.invalid/runtime\n")
	manifest.Revisions[0].Image = "example.invalid/tampered:selector"
	if err := writeAtomicJSON(runtime.runtimeManifestPath(manifest.ID), manifest); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background()); err == nil {
		t.Fatal("persisted non-canonical Runtime image selector was accepted")
	}
	if len(runner.outputs) != 0 {
		t.Fatalf("tampered persisted selector crossed Docker observation: %v", runner.outputs)
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
	tag := managedLibraryRuntimeImage("frontend", id, revision)
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

func TestRuntimeMaterialRequiresExactContentDigestCorrelation(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	digest := "sha256:" + strings.Repeat("b", 64)
	other := "sha256:" + strings.Repeat("c", 64)
	tag := managedLibraryRuntimeImage("frontend", id, revision)
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{
		tag:    {missing: true},
		digest: {observation: runtimeImageObservation{ID: other, Size: 2048, RepoTags: []string{"example.invalid/shared:fixture"}, Owner: ownerValue, Component: managedRuntimeComponentLabel, RuntimeID: id, Revision: revision}},
	}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	target := runtimeMaterialTarget{RuntimeID: id, Revision: revision, TagRole: tobari.RuntimeMaterialTagPublishedRevision, Selector: tag, RecordedDigest: digest, Name: "frontend"}
	if _, err := runtime.observeRuntimeMaterial(context.Background(), target, map[string]runtimeContentUse{}, lifecycleTestBudget()); err == nil {
		t.Fatal("content-digest inspect accepted a contradictory image ID")
	}

	target.Selector = "example.invalid/tampered:selector"
	runner.outputs = nil
	if _, err := runtime.observeRuntimeMaterial(context.Background(), target, map[string]runtimeContentUse{}, lifecycleTestBudget()); err == nil {
		t.Fatal("non-canonical material selector crossed observation")
	}
	if len(runner.outputs) != 0 {
		t.Fatalf("non-canonical material selector crossed Docker: %v", runner.outputs)
	}
}

func TestRuntimeLifecycleContainerListRejectsOpaqueSelectors(t *testing.T) {
	runner := &lifecycleObservationRunner{containerList: strings.Repeat("a", 63) + " bad\n", images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.observeRuntimeContainerUse(context.Background(), map[string]runtimeWorkspaceContainerAuthority{}, map[string]string{"runtime\x00revision": "sha256:" + strings.Repeat("b", 64)}, lifecycleTestBudget()); err == nil {
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
	uses, err = runtime.observeRuntimeContainerUse(context.Background(), map[string]runtimeWorkspaceContainerAuthority{}, map[string]string{runtimeID + "\x00" + revision: digest}, lifecycleTestBudget())
	if err != nil || uses[digest].workspace || !uses[digest].external {
		t.Fatalf("unjoined container authority = %+v/%v", uses, err)
	}
}

func TestRuntimeLifecycleContainerDiscoveryIsCandidateScopedAndBounded(t *testing.T) {
	digest := "sha256:" + strings.Repeat("b", 64)
	runner := &lifecycleObservationRunner{
		images:         map[string]lifecycleImageFixture{},
		containers:     map[string]runtimeContainerObservation{},
		containerLists: map[string]string{digest: ""},
	}
	for index := 0; index < maxRuntimeContainersPerImage+1; index++ {
		id := fmt.Sprintf("%064x", index+1)
		runner.containers[id] = runtimeContainerObservation{ID: id, Image: "sha256:" + strings.Repeat("c", 64)}
	}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	uses, err := runtime.observeRuntimeContainerUse(context.Background(), map[string]runtimeWorkspaceContainerAuthority{}, map[string]string{"runtime\x00revision": digest}, lifecycleTestBudget())
	if err != nil || len(uses) != 0 {
		t.Fatalf("unrelated daemon population affected candidate observation = %+v/%v", uses, err)
	}
	if len(runner.outputs) != 1 || !slices.Contains(runner.outputs[0].args, "ancestor="+digest) {
		t.Fatalf("container observation was not candidate-scoped: %v", runner.outputs)
	}

	ids := make([]string, 0, maxRuntimeContainersPerImage+1)
	for index := 0; index <= maxRuntimeContainersPerImage; index++ {
		ids = append(ids, fmt.Sprintf("%064x", index+1))
	}
	runner.containerLists[digest] = strings.Join(ids, "\n") + "\n"
	runner.outputs = nil
	if _, err := runtime.observeRuntimeContainerUse(context.Background(), map[string]runtimeWorkspaceContainerAuthority{}, map[string]string{"runtime\x00revision": digest}, lifecycleTestBudget()); err == nil {
		t.Fatal("over-bound exact candidate users were accepted")
	}
	if len(runner.outputs) != 1 {
		t.Fatalf("over-bound candidate users crossed inspect: %v", runner.outputs)
	}
}

func TestRuntimeLifecycleContainerDiscoveryRequiresFilterCorrelation(t *testing.T) {
	digest := "sha256:" + strings.Repeat("b", 64)
	id := strings.Repeat("a", 64)
	runner := &lifecycleObservationRunner{
		images:         map[string]lifecycleImageFixture{},
		containerLists: map[string]string{digest: id + "\n"},
		containers: map[string]runtimeContainerObservation{
			id: {ID: id, Image: "sha256:" + strings.Repeat("c", 64)},
		},
	}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.observeRuntimeContainerUse(context.Background(), map[string]runtimeWorkspaceContainerAuthority{}, map[string]string{"runtime\x00revision": digest}, lifecycleTestBudget()); err == nil {
		t.Fatal("candidate-filter result with contradictory image authority was accepted")
	}
}

func TestRuntimeMaterialWithoutTrustedBuildLabelsIsMigrationUnverified(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	digest := "sha256:" + strings.Repeat("b", 64)
	tag := managedLibraryRuntimeImage("frontend", id, revision)
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
	publishedManifest := installRuntimeLifecycleRevision(t, runtime, id, "frontend", "sha256:"+strings.Repeat("d", 64), "FROM example.invalid/published\n")
	published := publishedManifest.Revisions[0].Image
	revision := runtimeLifecycleFixtureRevision(t, "FROM example.invalid/runtime\n")
	var journal runtimeBuildJournal
	if err := runtime.WithLifecycleLock(context.Background(), func(context.Context) error {
		created, err := runtime.beginRuntimeBuildJournal(context.Background(), id, "frontend")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(created.SnapshotPath, 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(created.SnapshotPath, "Dockerfile"), []byte("FROM example.invalid/runtime\n"), 0o600); err != nil {
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
			bindEmptyFinalRuntimeProtection(t, runtime)
			created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
			if err != nil {
				t.Fatal(err)
			}
			journal, err := runtime.beginRuntimeBuildJournal(context.Background(), created.Runtime.ID, created.Runtime.Name)
			if err != nil {
				t.Fatal(err)
			}
			const snapshotContent = "FROM example.invalid/runtime\n"
			revision := runtimeLifecycleFixtureRevision(t, snapshotContent)
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
				if err := os.WriteFile(filepath.Join(journal.SnapshotPath, "Dockerfile"), []byte(snapshotContent), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			final := filepath.Join(runtime.runtimeRevisionsDirectory(journal.RuntimeID), strings.TrimPrefix(revision, "sha256:"), "source")
			if test.final {
				if err := os.MkdirAll(final, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(final, "Dockerfile"), []byte(snapshotContent), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if test.manifest {
				manifest, err := runtime.readRuntimeManifest(journal.RuntimeName)
				if err != nil {
					t.Fatal(err)
				}
				manifest.Revisions = []tobari.RuntimeRevision{{Ordinal: 1, Revision: revision, Image: journal.FinalImage, ImageDigest: journal.ImageDigest, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), SnapshotPath: final}}
				if err := writeAtomicJSON(runtime.runtimeManifestPath(manifest.ID), manifest); err != nil {
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
				bindEmptyFinalRuntimeProtection(t, runtime)
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
			bindEmptyFinalRuntimeProtection(t, runtime)
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
