package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type managedRuntimeTestImage struct {
	id     string
	labels map[string]string
}

type managedRuntimeBuildRunner struct {
	runs                 []runnerCall
	outputs              []runnerCall
	images               map[string]managedRuntimeTestImage
	containerLists       map[string]string
	containers           map[string]runtimeContainerObservation
	failBuild            bool
	corruptEvidence      string
	duringBuild          func(string)
	inspectFailure       string
	inspectOverflow      bool
	failImageRemove      bool
	removeThenFail       bool
	keepImageOnRemove    bool
	blockInspect         bool
	blockCompatibility   bool
	compatibilityPayload []byte
}

func newManagedRuntimeBuildRunner() *managedRuntimeBuildRunner {
	return &managedRuntimeBuildRunner{images: make(map[string]managedRuntimeTestImage), containerLists: make(map[string]string), containers: make(map[string]runtimeContainerObservation)}
}

func (r *managedRuntimeBuildRunner) Run(ctx context.Context, args, _ []string, _ io.Reader, out, errOut io.Writer) error {
	if len(args) >= 2 && args[0] == "container" && args[1] == "ls" {
		r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
		for _, arg := range args {
			if strings.HasPrefix(arg, "ancestor=") {
				_, err := io.WriteString(out, r.containerLists[strings.TrimPrefix(arg, "ancestor=")])
				return err
			}
		}
		return fmt.Errorf("container discovery lacks exact image filter")
	}
	if len(args) >= 5 && args[0] == "container" && args[1] == "inspect" {
		r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
		observed, ok := r.containers[args[4]]
		if !ok {
			return errors.New("synthetic container disappeared")
		}
		encoded, err := json.Marshal(observed)
		if err != nil {
			return err
		}
		_, err = out.Write(encoded)
		return err
	}
	if len(args) >= 5 && args[0] == "image" && args[1] == "inspect" {
		r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
		if strings.Contains(args[3], tobari.RuntimeImageAPILabel) {
			if r.blockCompatibility {
				<-ctx.Done()
				return ctx.Err()
			}
			payload := r.compatibilityPayload
			if payload == nil {
				payload = compatibleImageInspection()
			}
			_, err := out.Write(payload)
			return err
		}
		if r.blockInspect {
			<-ctx.Done()
			return ctx.Err()
		}
		image, ok := r.images[args[4]]
		if !ok {
			diagnostic := r.inspectFailure
			if diagnostic == "" {
				diagnostic = "Error: No such image: " + args[4]
			}
			_, _ = io.WriteString(errOut, diagnostic)
			return errors.New("image inspect failed")
		}
		if r.inspectOverflow {
			_, _ = io.WriteString(out, strings.Repeat("x", 8192))
			return nil
		}
		if strings.Contains(args[3], `"repo_tags"`) {
			tags := make([]string, 0)
			for tag, candidate := range r.images {
				if candidate.id == image.id {
					tags = append(tags, tag)
				}
			}
			sort.Strings(tags)
			observation := runtimeImageObservation{
				ID: image.id, Size: 1024, RepoTags: tags,
				Owner: image.labels[ownerLabel], Component: image.labels[componentLabel],
				RuntimeID: image.labels[managedRuntimeIDLabel], Revision: image.labels[managedRuntimeRevisionLabel],
			}
			data, err := json.Marshal(observation)
			if err != nil {
				return err
			}
			_, err = out.Write(data)
			return err
		}
		evidence := managedRuntimeBuildEvidence{ID: image.id, Owner: image.labels[ownerLabel], Component: image.labels[componentLabel], RuntimeID: image.labels[managedRuntimeIDLabel], Revision: image.labels[managedRuntimeRevisionLabel], AttemptID: image.labels[managedRuntimeBuildAttemptLabel]}
		switch r.corruptEvidence {
		case "digest":
			evidence.ID = "not-a-digest"
		case "owner":
			evidence.Owner = "foreign"
		case "component":
			evidence.Component = "foreign"
		case "runtime":
			evidence.RuntimeID = "018bcfe5-687b-7000-8000-000000000099"
		case "revision":
			evidence.Revision = "sha256:" + strings.Repeat("f", 64)
		}
		data, err := json.Marshal(evidence)
		if err != nil {
			return err
		}
		_, err = out.Write(data)
		return err
	}
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	if len(args) >= 2 && args[0] == "buildx" && args[1] == "build" {
		labels := make(map[string]string)
		for index := 0; index+1 < len(args); index++ {
			if args[index] != "--label" {
				continue
			}
			key, value, ok := strings.Cut(args[index+1], "=")
			if ok {
				labels[key] = value
			}
		}
		tag := args[slices.Index(args, "--tag")+1]
		r.images[tag] = managedRuntimeTestImage{id: "sha256:" + strings.Repeat("c", 64), labels: labels}
		if r.duringBuild != nil {
			r.duringBuild(args[len(args)-1])
		}
		if r.failBuild {
			return errors.New("synthetic build failure")
		}
		return nil
	}
	if len(args) == 4 && args[0] == "image" && args[1] == "tag" {
		image, ok := r.images[args[2]]
		if !ok {
			return errors.New("source image missing")
		}
		r.images[args[3]] = image
		return nil
	}
	if len(args) == 3 && args[0] == "image" && args[1] == "rm" {
		if (!r.failImageRemove || r.removeThenFail) && !r.keepImageOnRemove {
			delete(r.images, args[2])
		}
		if r.failImageRemove {
			return errors.New("synthetic image removal uncertainty")
		}
		return nil
	}
	return fmt.Errorf("unexpected Runtime build mutation: %v", args)
}

func (r *managedRuntimeBuildRunner) Output(ctx context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) < 5 || args[0] != "image" || args[1] != "inspect" {
		return nil, fmt.Errorf("unexpected Runtime build observation: %v", args)
	}
	format, imageName := args[3], args[4]
	if strings.Contains(format, tobari.RuntimeImageAPILabel) {
		if r.blockCompatibility {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return compatibleImageInspection(), nil
	}
	return nil, fmt.Errorf("unexpected Runtime build observation: %v (%s)", args, imageName)
}

func TestManagedRuntimeBuildCreatesImmutableRevisionWithoutChangingContext(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	before, err := runtime.ShowContext(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if !created.Created || created.Runtime.SourcePath != filepath.Join(root, "config", "runtimes", "frontend", "source") {
		t.Fatalf("created = %+v", created)
	}
	install := filepath.Join(created.Runtime.SourcePath, "install.sh")
	if err := os.WriteFile(install, []byte("#!/bin/sh\nset -eu\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	built, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !built.Built || built.NoChange || len(built.Runtime.Revisions) != 1 {
		t.Fatalf("built = %+v", built)
	}
	revision := built.Runtime.Revisions[0]
	if !strings.Contains(revision.SnapshotPath, filepath.Join("revisions", strings.TrimPrefix(revision.Revision, "sha256:"), "source")) {
		t.Fatalf("snapshot = %q", revision.SnapshotPath)
	}
	after, err := runtime.ShowContext(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if before.Image != after.Image || before.Runtime.Revision != after.Runtime.Revision {
		t.Fatalf("Runtime build changed Context: before=%+v after=%+v", before.Runtime, after.Runtime)
	}

	noChange, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !noChange.NoChange || noChange.Built || len(noChange.Runtime.Revisions) != 1 || len(runner.runs) != 3 {
		t.Fatalf("no-change build = %+v, runs=%d", noChange, len(runner.runs))
	}
	buildArgs := runner.runs[0].args
	for _, label := range []string{
		ownerLabel + "=" + ownerValue,
		componentLabel + "=" + managedRuntimeComponentLabel,
		managedRuntimeIDLabel + "=" + built.Runtime.ID,
		managedRuntimeRevisionLabel + "=" + revision.Revision,
	} {
		if !slices.Contains(buildArgs, label) {
			t.Fatalf("managed Runtime build lacks trusted label %q: %v", label, buildArgs)
		}
	}
	if _, exists := runner.images[revision.Image]; !exists {
		t.Fatalf("published Runtime tag is absent: %v", runner.images)
	}
	if _, exists := runner.images[managedRuntimeStagingImage(built.Runtime.ID, revision.Revision)]; exists {
		t.Fatalf("successful build retained staging tag: %v", runner.images)
	}
	if _, err := os.Lstat(runtime.runtimeBuildJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful build retained journal: %v", err)
	}
}

func TestManagedRuntimeBuildKeepsExactFailedArtifactJournal(t *testing.T) {
	for _, test := range []struct {
		name            string
		failBuild       bool
		corruptEvidence string
	}{
		{name: "build failure", failBuild: true},
		{name: "ownership verification failure", corruptEvidence: "owner"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := newManagedRuntimeBuildRunner()
			runner.failBuild = test.failBuild
			runner.corruptEvidence = test.corruptEvidence
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil {
				t.Fatal("unsafe managed Runtime build succeeded")
			}
			manifest, err := runtime.readRuntimeManifest("frontend")
			if err != nil || len(manifest.Revisions) != 0 {
				t.Fatalf("failed build published history = %+v/%v", manifest, err)
			}
			journal, err := runtime.readRuntimeBuildJournalObserved()
			if err != nil || journal == nil || journal.Phase != runtimeBuildPhaseFailed || journal.RuntimeID != created.Runtime.ID || journal.Revision == "" || journal.StagingImage != managedRuntimeStagingImage(journal.RuntimeID, journal.Revision) {
				t.Fatalf("failed build journal = %+v/%v", journal, err)
			}
			if info, err := os.Stat(journal.SnapshotPath); err != nil || !info.IsDir() {
				t.Fatalf("failed build staging snapshot = %v/%v", info, err)
			}
		})
	}
}

func TestManagedRuntimeBuildRollsBackJournalBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(created.Runtime.SourcePath, "Dockerfile")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(created.Runtime.SourcePath, "README"), []byte("no Dockerfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil {
		t.Fatal("invalid pre-Docker source built")
	}
	if len(runner.runs) != 0 {
		t.Fatalf("invalid source crossed Docker mutation: %v", runner.runs)
	}
	if _, err := os.Lstat(runtime.runtimeBuildJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-Docker failure retained journal: %v", err)
	}
	if _, err := os.Lstat(filepath.Dir(runtime.runtimeBuildSnapshotPath())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-Docker failure retained staging snapshot: %v", err)
	}
}

func TestManagedRuntimeBuildSurfacesPreDockerCleanupFailure(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(created.Runtime.SourcePath, "Dockerfile")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(created.Runtime.SourcePath, "README"), []byte("no Dockerfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime.runtimeBuildCleanup = func(runtimeBuildJournal) error { return errors.New("synthetic cleanup failure") }
	_, err = runtime.BuildManagedRuntime(context.Background(), "frontend", nil)
	if err == nil || !strings.Contains(err.Error(), "requires reconciliation") || !strings.Contains(err.Error(), "synthetic cleanup failure") {
		t.Fatalf("cleanup failure = %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("cleanup failure crossed Docker mutation: %v", runner.runs)
	}
	journal, journalErr := runtime.readRuntimeBuildJournalObserved()
	if journalErr != nil || journal == nil || journal.Phase != runtimeBuildPhaseCompleting || journal.CleanupFrom != runtimeBuildPhasePrepared {
		t.Fatalf("cleanup failure lost journal authority = %+v/%v", journal, journalErr)
	}
}

func TestManagedRuntimeBuildSurfacesSnapshotRemovalFailure(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(created.Runtime.SourcePath, "Dockerfile")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(created.Runtime.SourcePath, "README"), []byte("no Dockerfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime.runtimeBuildSnapshotRemove = func(string) error { return errors.New("synthetic snapshot removal failure") }
	if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil || !strings.Contains(err.Error(), "requires reconciliation") || !strings.Contains(err.Error(), "snapshot removal") {
		t.Fatalf("snapshot removal failure = %v", err)
	}
	journal, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || journal == nil || journal.Phase != runtimeBuildPhaseCompleting || journal.CleanupFrom != runtimeBuildPhasePrepared {
		t.Fatalf("snapshot removal failure lost journal = %+v/%v", journal, err)
	}
}

func TestManagedRuntimeBuildRetainsBuiltAuthorityWhenManifestPublicationIsUncertain(t *testing.T) {
	for _, test := range []struct {
		name      string
		postWrite bool
	}{
		{name: "before rename"},
		{name: "after rename before parent sync", postWrite: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := newManagedRuntimeBuildRunner()
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
			if err != nil {
				t.Fatal(err)
			}
			runtime.runtimeBuildManifestWrite = func(path string, value any) error {
				if test.postWrite {
					if err := writeAtomicJSON(path, value); err != nil {
						return err
					}
				}
				return errors.New("synthetic manifest publication uncertainty")
			}
			if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil || !strings.Contains(err.Error(), "requires reconciliation") {
				t.Fatalf("manifest publication uncertainty = %v", err)
			}
			journal, err := runtime.readRuntimeBuildJournalObserved()
			if err != nil || journal == nil || journal.Phase != runtimeBuildPhaseSnapshotPublished {
				t.Fatalf("retained built journal = %+v/%v", journal, err)
			}
			final := filepath.Join(runtime.runtimeRevisionsDirectory("frontend"), strings.TrimPrefix(journal.Revision, "sha256:"), "source")
			rehashed, err := digestRuntimeSnapshot(context.Background(), final)
			if err != nil || rehashed != journal.Revision {
				t.Fatalf("retained final snapshot = %q/%v", rehashed, err)
			}
			manifest, err := runtime.readRuntimeManifest("frontend")
			if err != nil || manifest.ID != created.Runtime.ID || len(manifest.Revisions) != map[bool]int{false: 0, true: 1}[test.postWrite] {
				t.Fatalf("manifest after uncertain publication = %+v/%v", manifest, err)
			}
		})
	}
}

func TestManagedRuntimeBuildDurabilityOrderAndFinalSyncFailure(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
		t.Fatal(err)
	}
	events := []string{}
	runtime.runtimeBuildSnapshotSync = func(path string) error {
		if strings.Contains(path, string(filepath.Separator)+"revisions"+string(filepath.Separator)) {
			events = append(events, "sync-final")
		} else {
			events = append(events, "sync-staging")
		}
		return syncRuntimeSnapshotTree(path)
	}
	runtime.runtimeBuildDirectorySync = func(path string) error {
		events = append(events, "sync-dir:"+filepath.Base(path))
		return syncDirectory(path)
	}
	runtime.runtimeBuildRename = func(source, target string) error {
		events = append(events, "rename-final")
		return os.Rename(source, target)
	}
	runtime.runtimeBuildFreeze = func(path string) error {
		events = append(events, "freeze-staging")
		return freezeRuntimeSnapshot(path)
	}
	runtime.runtimeBuildManifestWrite = func(path string, value any) error {
		events = append(events, "publish-manifest")
		return writeAtomicJSON(path, value)
	}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err != nil {
		t.Fatal(err)
	}
	index := func(event string) int {
		for i, observed := range events {
			if observed == event {
				return i
			}
		}
		return -1
	}
	for _, pair := range [][2]string{{"sync-staging", "freeze-staging"}, {"freeze-staging", "rename-final"}, {"rename-final", "sync-final"}, {"sync-final", "publish-manifest"}} {
		if left, right := index(pair[0]), index(pair[1]); left < 0 || right < 0 || left >= right {
			t.Fatalf("durability order %q before %q: %v", pair[0], pair[1], events)
		}
	}

	root = t.TempDir()
	runner = newManagedRuntimeBuildRunner()
	runtime, err = newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
		t.Fatal(err)
	}
	runtime.runtimeBuildSnapshotSync = func(path string) error {
		if strings.Contains(path, string(filepath.Separator)+"revisions"+string(filepath.Separator)) {
			return errors.New("synthetic final snapshot sync failure")
		}
		return syncRuntimeSnapshotTree(path)
	}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil || !strings.Contains(err.Error(), "requires reconciliation") {
		t.Fatalf("final sync failure = %v", err)
	}
	journal, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || journal == nil || journal.Phase != runtimeBuildPhaseStagingReleased {
		t.Fatalf("final sync failure lost authority = %+v/%v", journal, err)
	}
}

func TestManagedRuntimeBuildEvidenceRequiresExactOwnershipAndDigestPublication(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	tag := managedRuntimeStagingImage(id, revision)
	for _, corruption := range []string{"owner", "component", "runtime", "revision", "digest"} {
		t.Run(corruption, func(t *testing.T) {
			runner := newManagedRuntimeBuildRunner()
			runner.images[tag] = managedRuntimeTestImage{id: "sha256:" + strings.Repeat("c", 64), labels: map[string]string{ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel, managedRuntimeIDLabel: id, managedRuntimeRevisionLabel: revision}}
			runner.corruptEvidence = corruption
			runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.inspectManagedRuntimeBuildEvidence(context.Background(), tag, id, revision); err == nil {
				t.Fatalf("%s evidence was accepted", corruption)
			}
		})
	}

	runner := newManagedRuntimeBuildRunner()
	final := managedLibraryRuntimeImage("frontend", id, revision)
	runner.images[final] = managedRuntimeTestImage{id: "sha256:" + strings.Repeat("d", 64), labels: map[string]string{ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel, managedRuntimeIDLabel: id, managedRuntimeRevisionLabel: revision}}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	journal := runtimeBuildJournal{SchemaVersion: runtimeBuildJournalSchema, Phase: runtimeBuildPhaseBuilt, RuntimeID: id, RuntimeName: "frontend", AttemptID: strings.Repeat("1", 64), Revision: revision, StagingImage: tag, FinalImage: final, ImageDigest: "sha256:" + strings.Repeat("c", 64), SnapshotPath: runtime.runtimeBuildSnapshotPath(), StagingArtifact: runtimeBuildStagingOwned, AttemptSettlement: runtimeBuildAttemptSettled, CreatedAt: "2026-01-02T03:04:05Z"}
	if err := runtime.publishManagedRuntimeTag(context.Background(), journal); err == nil {
		t.Fatal("published Runtime tag with different digest was accepted")
	}
}

func TestRuntimeBuildJournalTransitionsRequireExactCurrentAuthority(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), newManagedRuntimeBuildRunner())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := runtime.beginRuntimeBuildJournal(context.Background(), "018bcfe5-687b-7000-8000-000000000077", "frontend")
	if err != nil {
		t.Fatal(err)
	}
	prepared := journal
	prepared.Phase = runtimeBuildPhasePrepared
	prepared.Revision = "sha256:" + strings.Repeat("a", 64)
	prepared.StagingImage = managedRuntimeStagingImage(prepared.RuntimeID, prepared.Revision)
	prepared.FinalImage = managedLibraryRuntimeImage(prepared.RuntimeName, prepared.RuntimeID, prepared.Revision)
	if err := runtime.writeRuntimeBuildJournal(journal, prepared); err != nil {
		t.Fatal(err)
	}

	built := prepared
	built.Phase = runtimeBuildPhaseBuilt
	built.ImageDigest = "sha256:" + strings.Repeat("b", 64)
	built.StagingArtifact = runtimeBuildStagingOwned
	built.AttemptSettlement = runtimeBuildAttemptSettled
	built.CreatedAt = "2026-01-02T03:04:05Z"
	missingBuiltProvenance := built
	missingBuiltProvenance.StagingArtifact = ""
	if err := missingBuiltProvenance.Validate(runtime); err == nil {
		t.Fatal("built journal without staging provenance was accepted")
	}
	missingFailedProvenance := prepared
	missingFailedProvenance.Phase = runtimeBuildPhaseFailed
	if err := missingFailedProvenance.Validate(runtime); err == nil {
		t.Fatal("failed journal without staging disposition was accepted")
	}
	if err := runtime.writeRuntimeBuildJournal(prepared, built); err == nil {
		t.Fatal("prepared journal skipped the building phase")
	}
	drifted := prepared
	drifted.RuntimeID = "018bcfe5-687b-7000-8000-000000000088"
	drifted.StagingImage = managedRuntimeStagingImage(drifted.RuntimeID, drifted.Revision)
	if err := runtime.writeRuntimeBuildJournal(prepared, drifted); err == nil {
		t.Fatal("journal immutable identity drift was accepted")
	}
	building := prepared
	building.Phase = runtimeBuildPhaseBuilding
	building.StagingArtifact = runtimeBuildStagingUnknown
	building.AttemptSettlement = runtimeBuildAttemptUnsettled
	if err := runtime.writeRuntimeBuildJournal(journal, building); err == nil {
		t.Fatal("stale journal authority overwrote the current phase")
	}
	if err := runtime.writeRuntimeBuildJournal(prepared, building); err != nil {
		t.Fatal(err)
	}
	regressed := building
	regressed.Phase = runtimeBuildPhasePrepared
	if err := runtime.writeRuntimeBuildJournal(building, regressed); err == nil {
		t.Fatal("journal phase regression was accepted")
	}
	if err := runtime.completeRuntimeBuildJournal(context.Background(), prepared); err == nil {
		t.Fatal("stale journal authority completed the active transaction")
	}
	driftedCurrent := building
	driftedCurrent.Phase = runtimeBuildPhaseFailed
	driftedCurrent.StagingArtifact = runtimeBuildStagingUnknown
	if err := writeAtomicJSON(runtime.runtimeBuildJournalPath(), driftedCurrent); err != nil {
		t.Fatal(err)
	}
	if err := runtime.rollbackRuntimeBuildBeforeDocker(context.Background(), errors.New("synthetic transition uncertainty"), building); err == nil || !strings.Contains(err.Error(), "requires reconciliation") {
		t.Fatalf("drifted current rollback = %v", err)
	}
	observed, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || observed == nil || *observed != driftedCurrent {
		t.Fatalf("drifted journal was cleaned = %+v/%v", observed, err)
	}
}

func preparedRuntimeBuildJournalFixture(t *testing.T) (*Runtime, *managedRuntimeBuildRunner, runtimeBuildJournal) {
	t.Helper()
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := runtime.beginRuntimeBuildJournal(context.Background(), "018bcfe5-687b-7000-8000-000000000077", "frontend")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(journal.SnapshotPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journal.SnapshotPath, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared := journal
	prepared.Phase = runtimeBuildPhasePrepared
	prepared.Revision, err = digestRuntimeSnapshot(context.Background(), journal.SnapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	prepared.StagingImage = managedRuntimeStagingImage(prepared.RuntimeID, prepared.Revision)
	prepared.FinalImage = managedLibraryRuntimeImage(prepared.RuntimeName, prepared.RuntimeID, prepared.Revision)
	if err := runtime.writeRuntimeBuildJournal(journal, prepared); err != nil {
		t.Fatal(err)
	}
	return runtime, runner, prepared
}

func TestRuntimeBuildCleanupPhaseIsCrashRepresentableAndRetryable(t *testing.T) {
	t.Run("before completing write", func(t *testing.T) {
		runtime, _, prepared := preparedRuntimeBuildJournalFixture(t)
		runtime.runtimeBuildCompletionWrite = func(runtimeBuildJournal) error { return errors.New("synthetic pre-write crash") }
		if err := runtime.completeRuntimeBuildJournal(context.Background(), prepared); err == nil {
			t.Fatal("cleanup crossed failed completing publication")
		}
		observed, err := runtime.readRuntimeBuildJournalObserved()
		if err != nil || observed == nil || *observed != prepared {
			t.Fatalf("pre-write crash authority = %+v/%v", observed, err)
		}
		if _, err := os.Lstat(prepared.SnapshotPath); err != nil {
			t.Fatalf("pre-write crash removed snapshot: %v", err)
		}
		runtime.runtimeBuildCompletionWrite = nil
		if err := runtime.completeRuntimeBuildJournal(context.Background(), prepared); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("after completing write", func(t *testing.T) {
		runtime, _, prepared := preparedRuntimeBuildJournalFixture(t)
		runtime.runtimeBuildCompletionWrite = func(completing runtimeBuildJournal) error {
			if err := writeAtomicJSON(runtime.runtimeBuildJournalPath(), completing); err != nil {
				return err
			}
			return errors.New("synthetic post-write crash")
		}
		runtime.runtimeBuildCleanup = func(runtimeBuildJournal) error { return errors.New("synthetic crash after completing write") }
		if err := runtime.completeRuntimeBuildJournal(context.Background(), prepared); err == nil {
			t.Fatal("cleanup did not stop after completing write")
		}
		observed, err := runtime.readRuntimeBuildJournalObserved()
		if err != nil || observed == nil || observed.Phase != runtimeBuildPhaseCompleting || observed.CleanupFrom != runtimeBuildPhasePrepared {
			t.Fatalf("post-write crash authority = %+v/%v", observed, err)
		}
		runtime.runtimeBuildCompletionWrite = nil
		runtime.runtimeBuildCleanup = nil
		if err := runtime.RecoverRuntimeBuildCleanup(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("after snapshot removal", func(t *testing.T) {
		runtime, _, prepared := preparedRuntimeBuildJournalFixture(t)
		runtime.runtimeBuildSnapshotRemove = func(path string) error {
			if err := removeRuntimeSnapshot(path); err != nil {
				return err
			}
			return errors.New("synthetic crash after snapshot removal")
		}
		if err := runtime.completeRuntimeBuildJournal(context.Background(), prepared); err == nil {
			t.Fatal("cleanup did not stop after snapshot removal")
		}
		observed, err := runtime.readRuntimeBuildJournalObserved()
		if err != nil || observed == nil || observed.Phase != runtimeBuildPhaseCompleting {
			t.Fatalf("snapshot-removal crash authority = %+v/%v", observed, err)
		}
		if _, err := os.Lstat(filepath.Dir(prepared.SnapshotPath)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("snapshot-removal crash retained snapshot: %v", err)
		}
		runtime.runtimeBuildSnapshotRemove = nil
		if err := runtime.RecoverRuntimeBuildCleanup(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("after journal unlink before parent sync", func(t *testing.T) {
		runtime, _, prepared := preparedRuntimeBuildJournalFixture(t)
		unlinked := false
		runtime.runtimeBuildJournalRemove = func(path string) error {
			if err := os.Remove(path); err != nil {
				return err
			}
			unlinked = true
			return nil
		}
		runtime.runtimeBuildDirectorySync = func(path string) error {
			if unlinked && path == runtime.runtimeLifecycleDirectory() {
				return errors.New("synthetic crash before journal parent sync")
			}
			return syncDirectory(path)
		}
		if err := runtime.completeRuntimeBuildJournal(context.Background(), prepared); err == nil {
			t.Fatal("cleanup did not surface journal parent sync uncertainty")
		}
		if _, err := os.Lstat(runtime.runtimeBuildJournalPath()); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("journal unlink crash retained journal: %v", err)
		}
		runtime.runtimeBuildJournalRemove = nil
		runtime.runtimeBuildDirectorySync = nil
		if err := runtime.RecoverRuntimeBuildCleanup(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRuntimeBuildCleanupDoesNotInferPreparedMissingSnapshot(t *testing.T) {
	runtime, _, prepared := preparedRuntimeBuildJournalFixture(t)
	if err := removeRuntimeSnapshot(prepared.SnapshotPath); err != nil {
		t.Fatal(err)
	}
	if err := runtime.RecoverRuntimeBuildCleanup(context.Background()); err == nil {
		t.Fatal("ordinary prepared journal was treated as explicit cleanup")
	}
	if err := runtime.completeRuntimeBuildJournal(context.Background(), prepared); err == nil {
		t.Fatal("prepared journal with missing snapshot entered cleanup")
	}
	observed, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || observed == nil || *observed != prepared {
		t.Fatalf("prepared missing-snapshot authority = %+v/%v", observed, err)
	}
}

func TestBuildingJournalWriteUncertaintyNeverOwnsRacingStagingTag(t *testing.T) {
	runtime, runner, prepared := preparedRuntimeBuildJournalFixture(t)
	building := prepared
	building.Phase = runtimeBuildPhaseBuilding
	building.StagingArtifact = runtimeBuildStagingUnknown
	building.AttemptSettlement = runtimeBuildAttemptUnsettled
	if err := runtime.writeRuntimeBuildJournal(prepared, building); err != nil {
		t.Fatal(err)
	}
	runner.images[building.StagingImage] = managedRuntimeTestImage{id: "sha256:" + strings.Repeat("b", 64), labels: map[string]string{ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel, managedRuntimeIDLabel: building.RuntimeID, managedRuntimeRevisionLabel: building.Revision}}
	if err := runtime.rollbackRuntimeBuildBeforeDocker(context.Background(), errors.New("synthetic building publication uncertainty"), prepared); err == nil || !strings.Contains(err.Error(), "requires reconciliation") {
		t.Fatalf("building publication uncertainty = %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("building publication uncertainty removed racing tag = %+v", runner.runs)
	}
	if _, exists := runner.images[building.StagingImage]; !exists {
		t.Fatal("building publication uncertainty lost racing tag")
	}
	observed, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || observed == nil || *observed != building {
		t.Fatalf("building publication uncertainty authority = %+v/%v", observed, err)
	}
}

func exactBuildingRuntimeBuildFixture(t *testing.T) (*Runtime, *managedRuntimeBuildRunner, runtimeBuildJournal) {
	t.Helper()
	runtime, runner, prepared := preparedRuntimeBuildJournalFixture(t)
	building := prepared
	building.Phase = runtimeBuildPhaseBuilding
	building.StagingArtifact = runtimeBuildStagingUnknown
	building.AttemptSettlement = runtimeBuildAttemptUnsettled
	if err := runtime.writeRuntimeBuildJournal(prepared, building); err != nil {
		t.Fatal(err)
	}
	runner.images[building.StagingImage] = managedRuntimeTestImage{
		id: "sha256:" + strings.Repeat("c", 64),
		labels: map[string]string{
			ownerLabel:                      ownerValue,
			componentLabel:                  managedRuntimeComponentLabel,
			managedRuntimeIDLabel:           building.RuntimeID,
			managedRuntimeRevisionLabel:     building.Revision,
			managedRuntimeBuildAttemptLabel: building.AttemptID,
		},
	}
	return runtime, runner, building
}

func failedRuntimeBuildAttemptFixture(t *testing.T, artifact string) (*Runtime, *managedRuntimeBuildRunner, runtimeBuildJournal) {
	t.Helper()
	runtime, runner, building := exactBuildingRuntimeBuildFixture(t)
	failed := building
	failed.Phase = runtimeBuildPhaseFailed
	failed.StagingArtifact = artifact
	failed.AttemptSettlement = runtimeBuildAttemptUnsettled
	if artifact == runtimeBuildStagingOwned {
		failed.ImageDigest = runner.images[building.StagingImage].id
	} else {
		delete(runner.images, building.StagingImage)
	}
	if err := runtime.writeRuntimeBuildJournal(building, failed); err != nil {
		t.Fatal(err)
	}
	runtimeRoot := runtime.runtimeDirectory(failed.RuntimeName)
	if err := os.MkdirAll(filepath.Join(runtimeRoot, "source"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(runtimeRoot, "revisions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeRoot, "source", "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest := tobari.RuntimeManifest{SchemaVersion: tobari.RuntimeSchemaVersion, ID: failed.RuntimeID, Name: failed.RuntimeName, Kind: tobari.RuntimeKindManaged, SourcePath: filepath.Join(runtimeRoot, "source"), Revisions: []tobari.RuntimeRevision{}}
	if err := writeAtomicJSON(runtime.runtimeManifestPath(failed.RuntimeName), manifest); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WithLifecycleLock(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	return runtime, runner, failed
}

func TestFailedRuntimeBuildSettlementClosesOwnedAndAbsentAttempts(t *testing.T) {
	for _, artifact := range []string{runtimeBuildStagingOwned, runtimeBuildStagingAbsent} {
		t.Run(artifact, func(t *testing.T) {
			runtime, runner, failed := failedRuntimeBuildAttemptFixture(t, artifact)
			before, err := os.ReadFile(runtime.runtimeBuildJournalPath())
			if err != nil {
				t.Fatal(err)
			}
			for review := 0; review < 2; review++ {
				recovery, found, err := runtime.ReadRuntimeBuildRecovery(context.Background())
				if err != nil || !found || recovery.Kind != tobari.RuntimeBuildRecoveryFailed || recovery.RuntimeRef != tobari.RuntimeRef(failed.RuntimeID) {
					t.Fatalf("failed recovery review = %+v/%t/%v", recovery, found, err)
				}
			}
			after, err := os.ReadFile(runtime.runtimeBuildJournalPath())
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("repeated review mutated failed journal: equal=%t err=%v", bytes.Equal(before, after), err)
			}

			if err := runtime.RecoverRuntimeBuildFailed(context.Background()); err != nil {
				t.Fatalf("settle and clean failed attempt: %v", err)
			}
			if journal, err := runtime.readRuntimeBuildJournalObserved(); err != nil || journal != nil {
				t.Fatalf("failed cleanup retained journal = %+v/%v", journal, err)
			}
			if _, exists := runner.images[failed.StagingImage]; exists {
				t.Fatal("failed cleanup retained exact staging image")
			}
			if _, err := runtime.BuildManagedRuntime(context.Background(), failed.RuntimeName, io.Discard); err != nil {
				t.Fatalf("next build remained unreachable: %v", err)
			}
		})
	}
}

func TestFailedRuntimeBuildUnknownOrInUseIsNonActionableAndZeroWrite(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		runtime, runner, _ := failedRuntimeBuildAttemptFixture(t, runtimeBuildStagingUnknown)
		runner.inspectFailure = "synthetic Docker observation failure"
		before, err := os.ReadFile(runtime.runtimeBuildJournalPath())
		if err != nil {
			t.Fatal(err)
		}
		if _, found, err := runtime.ReadRuntimeBuildRecovery(context.Background()); err == nil || found {
			t.Fatalf("unknown failed attempt was actionable = %t/%v", found, err)
		}
		if err := runtime.RecoverRuntimeBuildFailed(context.Background()); err == nil {
			t.Fatal("unknown failed attempt crossed mutation")
		}
		after, err := os.ReadFile(runtime.runtimeBuildJournalPath())
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("unknown observation mutated journal: equal=%t err=%v", bytes.Equal(before, after), err)
		}
	})

	t.Run("attempt label mismatch", func(t *testing.T) {
		runtime, runner, failed := failedRuntimeBuildAttemptFixture(t, runtimeBuildStagingOwned)
		image := runner.images[failed.StagingImage]
		image.labels[managedRuntimeBuildAttemptLabel] = strings.Repeat("2", 64)
		runner.images[failed.StagingImage] = image
		before, err := os.ReadFile(runtime.runtimeBuildJournalPath())
		if err != nil {
			t.Fatal(err)
		}
		if _, found, err := runtime.ReadRuntimeBuildRecovery(context.Background()); err == nil || found {
			t.Fatalf("mismatched failed attempt was actionable = %t/%v", found, err)
		}
		after, err := os.ReadFile(runtime.runtimeBuildJournalPath())
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("mismatched attempt mutated journal: equal=%t err=%v", bytes.Equal(before, after), err)
		}
	})

	t.Run("in use", func(t *testing.T) {
		runtime, runner, failed := failedRuntimeBuildAttemptFixture(t, runtimeBuildStagingOwned)
		containerID := strings.Repeat("e", 64)
		runner.containerLists[failed.ImageDigest] = containerID + "\n"
		runner.containers[containerID] = runtimeContainerObservation{ID: containerID, Image: failed.ImageDigest}
		before, err := os.ReadFile(runtime.runtimeBuildJournalPath())
		if err != nil {
			t.Fatal(err)
		}
		if _, found, err := runtime.ReadRuntimeBuildRecovery(context.Background()); err == nil || found {
			t.Fatalf("in-use failed artifact was actionable = %t/%v", found, err)
		}
		after, err := os.ReadFile(runtime.runtimeBuildJournalPath())
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("in-use review mutated journal: equal=%t err=%v", bytes.Equal(before, after), err)
		}
	})
}

func TestFailedRuntimeBuildLateEffectRemainsAttributable(t *testing.T) {
	runtime, runner, failed := failedRuntimeBuildAttemptFixture(t, runtimeBuildStagingAbsent)
	runtime.runtimeBuildCompletionWrite = func(runtimeBuildJournal) error { return errors.New("synthetic interruption after settlement") }
	if err := runtime.RecoverRuntimeBuildFailed(context.Background()); err == nil {
		t.Fatal("failed recovery crossed settlement interruption")
	}
	settled, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || settled == nil || settled.Phase != runtimeBuildPhaseFailed || settled.AttemptSettlement != runtimeBuildAttemptSettled || settled.StagingArtifact != runtimeBuildStagingAbsent {
		t.Fatalf("interrupted settlement authority = %+v/%v", settled, err)
	}
	runtime.runtimeBuildCompletionWrite = nil
	lateDigest := "sha256:" + strings.Repeat("f", 64)
	runner.images[failed.StagingImage] = managedRuntimeTestImage{id: lateDigest, labels: map[string]string{
		ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel,
		managedRuntimeIDLabel: failed.RuntimeID, managedRuntimeRevisionLabel: failed.Revision,
		managedRuntimeBuildAttemptLabel: failed.AttemptID,
	}}
	if _, found, err := runtime.ReadRuntimeBuildRecovery(context.Background()); err != nil || !found {
		t.Fatalf("late exact effect was not reviewable = %t/%v", found, err)
	}
	if err := runtime.RecoverRuntimeBuildFailed(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, exists := runner.images[failed.StagingImage]; exists {
		t.Fatal("reviewed late effect remained after same-command retry")
	}
	if journal, err := runtime.readRuntimeBuildJournalObserved(); err != nil || journal != nil {
		t.Fatalf("same-command retry retained authority = %+v/%v", journal, err)
	}
}

func TestFailedRuntimeBuildInterruptedCleanupResumesThroughSameReviewRecovery(t *testing.T) {
	runtime, runner, failed := failedRuntimeBuildAttemptFixture(t, runtimeBuildStagingAbsent)
	runtime.runtimeBuildCompletionWrite = func(completing runtimeBuildJournal) error {
		if err := writeAtomicJSON(runtime.runtimeBuildJournalPath(), completing); err != nil {
			return err
		}
		return errors.New("synthetic interruption after cleanup authority write")
	}
	runtime.runtimeBuildDirectorySync = func(string) error { return errors.New("synthetic interrupted directory sync") }
	if err := runtime.RecoverRuntimeBuildFailed(context.Background()); err == nil {
		t.Fatalf("interrupted failed cleanup = %v", err)
	}
	runtime.runtimeBuildCompletionWrite = nil
	runtime.runtimeBuildDirectorySync = nil
	completing, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || completing == nil || completing.Phase != runtimeBuildPhaseCompleting || completing.CleanupFrom != runtimeBuildPhaseFailed || completing.StagingArtifact != runtimeBuildStagingAbsent || completing.RemoveStaging {
		t.Fatalf("interrupted cleanup authority = %+v/%v", completing, err)
	}

	lateDigest := "sha256:" + strings.Repeat("a", 64)
	runner.images[failed.StagingImage] = managedRuntimeTestImage{id: lateDigest, labels: map[string]string{
		ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel,
		managedRuntimeIDLabel: failed.RuntimeID, managedRuntimeRevisionLabel: failed.Revision,
		managedRuntimeBuildAttemptLabel: failed.AttemptID,
	}}
	recovery, found, err := runtime.ReadRuntimeBuildRecovery(context.Background())
	if err != nil || !found || recovery.Kind != tobari.RuntimeBuildRecoveryCleanup || recovery.RuntimeRef != tobari.RuntimeRef(failed.RuntimeID) {
		t.Fatalf("interrupted cleanup review = %+v/%t/%v", recovery, found, err)
	}
	if err := runtime.RecoverRuntimeBuildByReference(context.Background(), string(recovery.RuntimeRef), recovery.Kind); err != nil {
		t.Fatalf("same review recovery retry: %v", err)
	}
	if _, exists := runner.images[failed.StagingImage]; exists {
		t.Fatal("reviewed late effect remained after cleanup retry")
	}
	if journal, err := runtime.readRuntimeBuildJournalObserved(); err != nil || journal != nil {
		t.Fatalf("cleanup retry retained authority = %+v/%v", journal, err)
	}
}

func TestRuntimeBuildJournalRequiresAttemptIdentity(t *testing.T) {
	runtime, _, building := exactBuildingRuntimeBuildFixture(t)
	building.AttemptID = ""
	if err := building.Validate(runtime); err == nil {
		t.Fatal("Runtime build journal accepted missing attempt identity")
	}
	previous := building
	previous.AttemptID = strings.Repeat("1", 64)
	next := previous
	next.Phase = runtimeBuildPhaseFailed
	next.AttemptID = strings.Repeat("2", 64)
	next.StagingArtifact = runtimeBuildStagingAbsent
	next.AttemptSettlement = runtimeBuildAttemptUnsettled
	if err := validateRuntimeBuildJournalTransition(previous, next); err == nil {
		t.Fatal("Runtime build transition accepted changed attempt identity")
	}
}

func TestRuntimeBuildingRecoveryClosesOutcomeUnknownWithoutInferringCleanup(t *testing.T) {
	t.Run("exact success becomes built", func(t *testing.T) {
		runtime, _, building := exactBuildingRuntimeBuildFixture(t)
		fixed := time.Date(2026, 2, 3, 4, 5, 6, 7, time.UTC)
		runtime.identities.now = func() time.Time { return fixed }
		if err := runtime.RecoverRuntimeBuildBuilding(context.Background()); err != nil {
			t.Fatal(err)
		}
		observed, err := runtime.readRuntimeBuildJournalObserved()
		if err != nil || observed == nil || observed.Phase != runtimeBuildPhaseBuilt || observed.StagingArtifact != runtimeBuildStagingOwned || observed.ImageDigest == "" || observed.CreatedAt != fixed.Format(time.RFC3339Nano) {
			t.Fatalf("reconciled building authority = %+v/%v", observed, err)
		}
		if _, err := os.Lstat(building.SnapshotPath); err != nil {
			t.Fatalf("building recovery inferred cleanup: %v", err)
		}
	})

	t.Run("confirmed absence becomes failed", func(t *testing.T) {
		runtime, runner, building := exactBuildingRuntimeBuildFixture(t)
		delete(runner.images, building.StagingImage)
		if err := runtime.RecoverRuntimeBuildBuilding(context.Background()); err != nil {
			t.Fatal(err)
		}
		observed, err := runtime.readRuntimeBuildJournalObserved()
		if err != nil || observed == nil || observed.Phase != runtimeBuildPhaseFailed || observed.StagingArtifact != runtimeBuildStagingAbsent || observed.ImageDigest != "" {
			t.Fatalf("absent building authority = %+v/%v", observed, err)
		}
	})

	for _, test := range []struct {
		name  string
		alter func(*Runtime, *managedRuntimeBuildRunner, runtimeBuildJournal)
	}{
		{name: "replacement", alter: func(_ *Runtime, runner *managedRuntimeBuildRunner, _ runtimeBuildJournal) {
			runner.corruptEvidence = "runtime"
		}},
		{name: "snapshot drift", alter: func(_ *Runtime, _ *managedRuntimeBuildRunner, journal runtimeBuildJournal) {
			if err := os.WriteFile(filepath.Join(journal.SnapshotPath, "drift"), []byte("changed"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name+" remains building", func(t *testing.T) {
			runtime, runner, building := exactBuildingRuntimeBuildFixture(t)
			test.alter(runtime, runner, building)
			if err := runtime.RecoverRuntimeBuildBuilding(context.Background()); err == nil {
				t.Fatal("uncertain building authority was accepted")
			}
			observed, err := runtime.readRuntimeBuildJournalObserved()
			if err != nil || observed == nil || *observed != building {
				t.Fatalf("uncertain building journal changed = %+v/%v", observed, err)
			}
		})
	}
}

func TestRuntimeBuildingRecoveryBoundsCompatibilityOutputWithoutJournalDrift(t *testing.T) {
	validOversized := []byte(`{"api":"1","lifetime":"command","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint","` + strings.Repeat("x", 8192) + `"]}`)
	invalidOversized := []byte(strings.Repeat("x", 8192))
	for _, test := range []struct {
		name    string
		payload []byte
	}{
		{name: "valid", payload: validOversized},
		{name: "invalid", payload: invalidOversized},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, runner, building := exactBuildingRuntimeBuildFixture(t)
			runner.compatibilityPayload = test.payload
			before, err := os.ReadFile(runtime.runtimeBuildJournalPath())
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.RecoverRuntimeBuildBuilding(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds the observation bound") {
				t.Fatalf("oversized %s compatibility = %v", test.name, err)
			}
			after, err := os.ReadFile(runtime.runtimeBuildJournalPath())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("oversized compatibility changed journal bytes")
			}
			observed, err := runtime.readRuntimeBuildJournalObserved()
			if err != nil || observed == nil || *observed != building {
				t.Fatalf("oversized compatibility journal = %+v/%v", observed, err)
			}
		})
	}
}

func TestRuntimeBuildingRecoveryHandlesBuiltWriteUncertaintyAndRetry(t *testing.T) {
	for _, afterWrite := range []bool{false, true} {
		t.Run(fmt.Sprintf("after_write_%t", afterWrite), func(t *testing.T) {
			runtime, _, building := exactBuildingRuntimeBuildFixture(t)
			fixed := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
			runtime.identities.now = func() time.Time { return fixed }
			runtime.runtimeBuildJournalWrite = func(_, next runtimeBuildJournal) error {
				if afterWrite {
					if err := writeAtomicJSON(runtime.runtimeBuildJournalPath(), next); err != nil {
						return err
					}
				}
				return errors.New("synthetic built publication uncertainty")
			}
			err := runtime.RecoverRuntimeBuildBuilding(context.Background())
			runtime.runtimeBuildJournalWrite = nil
			if afterWrite {
				if err != nil {
					t.Fatalf("post-write authority was not reobserved: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("pre-write failure was hidden")
				}
				observed, observeErr := runtime.readRuntimeBuildJournalObserved()
				if observeErr != nil || observed == nil || *observed != building {
					t.Fatalf("pre-write failure changed authority = %+v/%v", observed, observeErr)
				}
				if retryErr := runtime.RecoverRuntimeBuildBuilding(context.Background()); retryErr != nil {
					t.Fatalf("building retry: %v", retryErr)
				}
			}
			observed, observeErr := runtime.readRuntimeBuildJournalObserved()
			if observeErr != nil || observed == nil || observed.Phase != runtimeBuildPhaseBuilt || observed.CreatedAt != fixed.Format(time.RFC3339Nano) {
				t.Fatalf("built recovery authority = %+v/%v", observed, observeErr)
			}
		})
	}
}

func TestRuntimeBuildingAbsentAttemptRemainsDurableAcrossLateEffect(t *testing.T) {
	runtime, runner, building := exactBuildingRuntimeBuildFixture(t)
	delete(runner.images, building.StagingImage)
	if err := runtime.RecoverRuntimeBuildBuilding(context.Background()); err != nil {
		t.Fatal(err)
	}
	failed, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || failed == nil || failed.Phase != runtimeBuildPhaseFailed || failed.StagingArtifact != runtimeBuildStagingAbsent || failed.AttemptSettlement != runtimeBuildAttemptUnsettled {
		t.Fatalf("unsettled absent attempt = %+v/%v", failed, err)
	}
	if err := runtime.completeRuntimeBuildJournal(context.Background(), *failed); err == nil {
		t.Fatal("one absent observation erased unsettled attempt authority")
	}
	if _, err := runtime.beginRuntimeBuildJournal(context.Background(), building.RuntimeID, building.RuntimeName); err == nil {
		t.Fatal("new build reused an unresolved attempt authority")
	}
	runner.images[building.StagingImage] = managedRuntimeTestImage{
		id: "sha256:" + strings.Repeat("c", 64),
		labels: map[string]string{
			ownerLabel:                  ownerValue,
			componentLabel:              managedRuntimeComponentLabel,
			managedRuntimeIDLabel:       building.RuntimeID,
			managedRuntimeRevisionLabel: building.Revision,
		},
	}
	observed, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || observed == nil || *observed != *failed {
		t.Fatalf("late effect lost durable attempt attribution = %+v/%v", observed, err)
	}
	digest, err := runtime.inspectManagedRuntimeBuildEvidence(context.Background(), building.StagingImage, building.RuntimeID, building.Revision)
	if err != nil || digest == "" {
		t.Fatalf("late exact staging effect is not reviewable = %q/%v", digest, err)
	}
}

func TestRuntimePreDockerRecoveryIsExplicitAndCrashReachable(t *testing.T) {
	for _, partialSnapshot := range []bool{false, true} {
		t.Run(fmt.Sprintf("snapshotting_partial_%t", partialSnapshot), func(t *testing.T) {
			root := t.TempDir()
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), newManagedRuntimeBuildRunner())
			if err != nil {
				t.Fatal(err)
			}
			journal, err := runtime.beginRuntimeBuildJournal(context.Background(), "018bcfe5-687b-7000-8000-000000000077", "frontend")
			if err != nil {
				t.Fatal(err)
			}
			if partialSnapshot {
				if err := os.MkdirAll(journal.SnapshotPath, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(journal.SnapshotPath, "partial"), []byte("partial"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := runtime.RecoverRuntimeBuildPreDocker(context.Background()); err != nil {
				t.Fatal(err)
			}
			if observed, err := runtime.readRuntimeBuildJournalObserved(); err != nil || observed != nil {
				t.Fatalf("snapshotting recovery retained authority = %+v/%v", observed, err)
			}
		})
	}

	t.Run("prepared absent selector cleans", func(t *testing.T) {
		runtime, _, _ := preparedRuntimeBuildJournalFixture(t)
		if err := runtime.RecoverRuntimeBuildPreDocker(context.Background()); err != nil {
			t.Fatal(err)
		}
		if observed, err := runtime.readRuntimeBuildJournalObserved(); err != nil || observed != nil {
			t.Fatalf("prepared recovery retained authority = %+v/%v", observed, err)
		}
	})

	for _, exact := range []bool{true, false} {
		t.Run(fmt.Sprintf("prepared_conflict_exact_%t", exact), func(t *testing.T) {
			runtime, runner, prepared := preparedRuntimeBuildJournalFixture(t)
			labels := map[string]string{ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel, managedRuntimeIDLabel: prepared.RuntimeID, managedRuntimeRevisionLabel: prepared.Revision}
			if !exact {
				labels[managedRuntimeIDLabel] = "018bcfe5-687b-7000-8000-000000000099"
			}
			runner.images[prepared.StagingImage] = managedRuntimeTestImage{id: "sha256:" + strings.Repeat("c", 64), labels: labels}
			if err := runtime.RecoverRuntimeBuildPreDocker(context.Background()); err == nil {
				t.Fatal("prepared staging conflict was hidden")
			}
			observed, err := runtime.readRuntimeBuildJournalObserved()
			want := runtimeBuildOrphanUnknown
			if exact {
				want = runtimeBuildOrphanExactManaged
			}
			if err != nil || observed == nil || observed.Phase != runtimeBuildPhaseOrphanStaging || observed.OrphanStaging != want {
				t.Fatalf("prepared orphan authority = %+v/%v", observed, err)
			}
			if _, exists := runner.images[prepared.StagingImage]; !exists {
				t.Fatal("pre-Docker recovery deleted a conflicting tag")
			}
		})
	}
}

func TestRuntimeBuildCompletingRejectsPublicationOrigins(t *testing.T) {
	runtime, _, building := exactBuildingRuntimeBuildFixture(t)
	built := building
	built.Phase = runtimeBuildPhaseBuilt
	built.ImageDigest = "sha256:" + strings.Repeat("c", 64)
	built.StagingArtifact = runtimeBuildStagingOwned
	built.AttemptSettlement = runtimeBuildAttemptSettled
	built.CreatedAt = "2026-02-03T04:05:06Z"
	for _, origin := range []runtimeBuildJournal{
		building,
		built,
		func() runtimeBuildJournal { value := built; value.Phase = runtimeBuildPhaseFinalTagged; return value }(),
		func() runtimeBuildJournal {
			value := built
			value.Phase = runtimeBuildPhaseStagingReleased
			return value
		}(),
		func() runtimeBuildJournal {
			value := built
			value.Phase = runtimeBuildPhaseSnapshotPublished
			return value
		}(),
	} {
		t.Run(origin.Phase, func(t *testing.T) {
			completing := origin
			completing.Phase = runtimeBuildPhaseCompleting
			completing.CleanupFrom = origin.Phase
			if err := completing.Validate(runtime); err == nil {
				t.Fatalf("completing accepted forbidden origin %s", origin.Phase)
			}
		})
	}
	failed := building
	failed.Phase = runtimeBuildPhaseFailed
	failed.StagingArtifact = runtimeBuildStagingAbsent
	failed.AttemptSettlement = runtimeBuildAttemptUnsettled
	completing := failed
	completing.Phase = runtimeBuildPhaseCompleting
	completing.CleanupFrom = runtimeBuildPhaseFailed
	if err := completing.Validate(runtime); err == nil {
		t.Fatal("completing accepted an unsettled absent build attempt")
	}
}

func TestRuntimeBuildPublicationEdgesPreserveExactAuthority(t *testing.T) {
	_, _, building := exactBuildingRuntimeBuildFixture(t)
	base := building
	base.Phase = runtimeBuildPhaseBuilt
	base.ImageDigest = "sha256:" + strings.Repeat("c", 64)
	base.StagingArtifact = runtimeBuildStagingOwned
	base.AttemptSettlement = runtimeBuildAttemptSettled
	base.CreatedAt = "2026-02-03T04:05:06Z"
	edges := [][2]string{
		{runtimeBuildPhaseBuilt, runtimeBuildPhaseFinalTagged},
		{runtimeBuildPhaseFinalTagged, runtimeBuildPhaseStagingReleased},
		{runtimeBuildPhaseStagingReleased, runtimeBuildPhaseSnapshotPublished},
		{runtimeBuildPhaseSnapshotPublished, runtimeBuildPhaseManifestCommitted},
	}
	for _, edge := range edges {
		t.Run(edge[0]+"_to_"+edge[1], func(t *testing.T) {
			previous := base
			previous.Phase = edge[0]
			next := previous
			next.Phase = edge[1]
			next.ImageDigest = "sha256:" + strings.Repeat("d", 64)
			if err := validateRuntimeBuildJournalTransition(previous, next); err == nil {
				t.Fatal("post-target edge accepted changed digest authority")
			}
		})
	}
}

func TestRuntimeBuildCleanupSupportsEveryTerminalOrigin(t *testing.T) {
	for _, phase := range []string{runtimeBuildPhaseSnapshotting, runtimeBuildPhasePrepared, runtimeBuildPhaseFailed} {
		t.Run(phase, func(t *testing.T) {
			if phase == runtimeBuildPhaseSnapshotting {
				root := t.TempDir()
				runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), newManagedRuntimeBuildRunner())
				if err != nil {
					t.Fatal(err)
				}
				journal, err := runtime.beginRuntimeBuildJournal(context.Background(), "018bcfe5-687b-7000-8000-000000000077", "frontend")
				if err != nil {
					t.Fatal(err)
				}
				if err := runtime.completeRuntimeBuildJournal(context.Background(), journal); err != nil {
					t.Fatal(err)
				}
				return
			}
			runtime, _, journal := preparedRuntimeBuildJournalFixture(t)
			if phase != runtimeBuildPhasePrepared {
				building := journal
				building.Phase = runtimeBuildPhaseBuilding
				building.StagingArtifact = runtimeBuildStagingUnknown
				building.AttemptSettlement = runtimeBuildAttemptUnsettled
				if err := runtime.writeRuntimeBuildJournal(journal, building); err != nil {
					t.Fatal(err)
				}
				journal = building
				next := journal
				next.Phase = phase
				if phase == runtimeBuildPhaseFailed {
					next.StagingArtifact = runtimeBuildStagingAbsent
					next.AttemptSettlement = runtimeBuildAttemptSettled
				}
				if err := runtime.writeRuntimeBuildJournal(journal, next); err != nil {
					t.Fatal(err)
				}
				journal = next
			}
			if err := runtime.completeRuntimeBuildJournal(context.Background(), journal); err != nil {
				t.Fatal(err)
			}
			if observed, err := runtime.readRuntimeBuildJournalObserved(); err != nil || observed != nil {
				t.Fatalf("completed %s authority = %+v/%v", phase, observed, err)
			}
		})
	}
}

func TestManagedRuntimeBuildRehashesBeforePreparedAndBuiltPublication(t *testing.T) {
	for _, test := range []struct {
		name       string
		boundary   int
		beforeHash bool
	}{
		{name: "before initial rehash", boundary: 1, beforeHash: true},
		{name: "after initial rehash", boundary: 1},
		{name: "before post-build rehash", boundary: 2, beforeHash: true},
		{name: "after post-build rehash", boundary: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := newManagedRuntimeBuildRunner()
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			runtime.runtimeBuildRehashBoundary = func(_ string, before bool) error {
				if before {
					calls++
				}
				if calls == test.boundary && before == test.beforeHash {
					return errors.New("synthetic rehash boundary crash")
				}
				return nil
			}
			if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil {
				t.Fatal("build crossed rehash boundary crash")
			}
			journal, journalErr := runtime.readRuntimeBuildJournalObserved()
			if test.boundary == 1 {
				if journalErr != nil || journal != nil || len(runner.runs) != 0 {
					t.Fatalf("initial rehash crash authority/runs = %+v/%v/%v", journal, journalErr, runner.runs)
				}
				return
			}
			if journalErr != nil || journal == nil || journal.Phase != runtimeBuildPhaseFailed || journal.ImageDigest == "" {
				t.Fatalf("post-build rehash crash authority = %+v/%v", journal, journalErr)
			}
			if _, exists := runner.images[journal.FinalImage]; exists {
				t.Fatal("post-build rehash crash published the normal tag")
			}
			if _, exists := runner.images[journal.StagingImage]; !exists {
				t.Fatal("post-build rehash crash lost staging evidence")
			}
			if journal.RuntimeID != created.Runtime.ID {
				t.Fatalf("post-build rehash crash runtime = %q", journal.RuntimeID)
			}
		})
	}
}

func TestManagedRuntimeBuildSnapshotMutationDuringDockerNeverPublishesFinalTag(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runner.duringBuild = func(snapshot string) {
		if err := os.WriteFile(filepath.Join(snapshot, "mutated-during-build"), []byte("drift\n"), 0o600); err != nil {
			t.Error(err)
		}
	}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil || !strings.Contains(err.Error(), "drifted before image publication") {
		t.Fatalf("snapshot mutation result = %v", err)
	}
	journal, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || journal == nil || journal.Phase != runtimeBuildPhaseFailed || journal.ImageDigest == "" {
		t.Fatalf("snapshot mutation authority = %+v/%v", journal, err)
	}
	if _, exists := runner.images[journal.FinalImage]; exists {
		t.Fatal("mutated build snapshot published final tag")
	}
	if _, exists := runner.images[journal.StagingImage]; !exists {
		t.Fatal("mutated build snapshot lost staging artifact")
	}
}

func TestManagedRuntimeBuildEvidenceIsBoundedBeforeAllocationAndMissingIsExact(t *testing.T) {
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	image := managedRuntimeStagingImage(id, revision)

	runner := newManagedRuntimeBuildRunner()
	runner.images[image] = managedRuntimeTestImage{id: "sha256:" + strings.Repeat("b", 64), labels: map[string]string{ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel, managedRuntimeIDLabel: id, managedRuntimeRevisionLabel: revision}}
	runner.inspectOverflow = true
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.inspectManagedRuntimeBuildEvidence(context.Background(), image, id, revision); err == nil || !strings.Contains(err.Error(), "exceeds the observation bound") {
		t.Fatalf("hostile inspect output = %v", err)
	}

	runner = newManagedRuntimeBuildRunner()
	runtime, err = newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range []string{
		"plugin lookup not found while inspecting unrelated state",
		"wrapper: Error response from daemon: No such image: " + image,
		"Error response from daemon: No such image: " + image + " (wrapped)",
		"Error response from daemon: No such image: " + image + "\nunrelated failure",
		"Error response from daemon: No such image: unrelated\nError response from daemon: No such image: " + image,
	} {
		runner.inspectFailure = diagnostic
		if _, err := runtime.inspectManagedRuntimeBuildEvidence(context.Background(), image, id, revision); err == nil || errors.Is(err, errManagedRuntimeImageMissing) {
			t.Fatalf("diagnostic authorized image absence %q = %v", diagnostic, err)
		}
	}
	runner.inspectFailure = "Error response from daemon: No such image: " + image
	if _, err := runtime.inspectManagedRuntimeBuildEvidence(context.Background(), image, id, revision); !errors.Is(err, errManagedRuntimeImageMissing) {
		t.Fatalf("exact missing image diagnostic = %v", err)
	}
}

func TestManagedRuntimeStagingSelectorUsesFullStableAuthority(t *testing.T) {
	baseID := "018bcfe5-687b-7000-8000-000000000077"
	otherID := "018bcfe5-687b-7000-8000-000000000088"
	firstRevision := "sha256:" + strings.Repeat("a", 63) + "1"
	otherRevision := "sha256:" + strings.Repeat("a", 63) + "2"
	first := managedRuntimeStagingImage(baseID, firstRevision)
	if first == managedRuntimeStagingImage(otherID, firstRevision) || first == managedRuntimeStagingImage(baseID, otherRevision) {
		t.Fatal("private Runtime staging selector truncated stable authority")
	}
	if !strings.Contains(first, baseID) || !strings.Contains(first, strings.TrimPrefix(firstRevision, "sha256:")) || tobari.ValidateImageSelector(first) != nil {
		t.Fatalf("full staging selector = %q", first)
	}
	final := managedLibraryRuntimeImage("frontend", baseID, firstRevision)
	if final == managedLibraryRuntimeImage("frontend", otherID, firstRevision) || final == managedLibraryRuntimeImage("frontend", baseID, otherRevision) || tobari.ValidateImageSelector(final) != nil {
		t.Fatalf("full published selector = %q", final)
	}
}

func TestManagedRuntimeNoChangeRejectsOrphanStagingBeforeCompletion(t *testing.T) {
	for _, corruption := range []string{"exact", "owner", "revision"} {
		t.Run(corruption, func(t *testing.T) {
			root := t.TempDir()
			runner := newManagedRuntimeBuildRunner()
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
				t.Fatal(err)
			}
			built, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil)
			if err != nil {
				t.Fatal(err)
			}
			revision := built.Runtime.Revisions[0].Revision
			staging := managedRuntimeStagingImage(built.Runtime.ID, revision)
			labels := map[string]string{ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel, managedRuntimeIDLabel: built.Runtime.ID, managedRuntimeRevisionLabel: revision}
			switch corruption {
			case "owner":
				labels[ownerLabel] = "foreign"
			case "revision":
				labels[managedRuntimeRevisionLabel] = "sha256:" + strings.Repeat("f", 64)
			}
			runner.images[staging] = managedRuntimeTestImage{id: "sha256:" + strings.Repeat("d", 64), labels: labels}
			runsBefore := len(runner.runs)
			if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil {
				t.Fatal("no-change build ignored orphan staging authority")
			}
			if len(runner.runs) != runsBefore {
				t.Fatalf("pre-existing orphan staging crossed mutation: %+v", runner.runs[runsBefore:])
			}
			if _, exists := runner.images[staging]; !exists {
				t.Fatal("pre-existing orphan staging was mutated")
			}
			journal, journalErr := runtime.readRuntimeBuildJournalObserved()
			wantDisposition := runtimeBuildOrphanUnknown
			if corruption == "exact" {
				wantDisposition = runtimeBuildOrphanExactManaged
			}
			if journalErr != nil || journal == nil || journal.Phase != runtimeBuildPhaseOrphanStaging || journal.OrphanStaging != wantDisposition {
				t.Fatalf("orphan staging lost durable blocker = %+v/%v", journal, journalErr)
			}
			if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil || len(runner.runs) != runsBefore {
				t.Fatalf("orphan retry crossed authority = %v/%+v", err, runner.runs[runsBefore:])
			}
			if corruption != "exact" {
				if err := runtime.RecoverRuntimeBuildOrphanStaging(context.Background()); err == nil || len(runner.runs) != runsBefore {
					t.Fatalf("unknown orphan recovery crossed mutation = %v/%+v", err, runner.runs[runsBefore:])
				}
				return
			}
			if err := runtime.RecoverRuntimeBuildOrphanStaging(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(runner.runs) != runsBefore+1 || !slices.Equal(runner.runs[runsBefore].args, []string{"image", "rm", staging}) {
				t.Fatalf("explicit orphan recovery = %+v", runner.runs[runsBefore:])
			}
			if _, exists := runner.images[staging]; exists {
				t.Fatal("explicit orphan recovery retained staging")
			}
			if journal, err := runtime.readRuntimeBuildJournalObserved(); err != nil || journal != nil {
				t.Fatalf("explicit orphan recovery retained journal = %+v/%v", journal, err)
			}
		})
	}
}

func exactOrphanRuntimeBuildFixture(t *testing.T) (*Runtime, *managedRuntimeBuildRunner, string) {
	t.Helper()
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
		t.Fatal(err)
	}
	built, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil)
	if err != nil {
		t.Fatal(err)
	}
	revision := built.Runtime.Revisions[0].Revision
	staging := managedRuntimeStagingImage(built.Runtime.ID, revision)
	runner.images[staging] = managedRuntimeTestImage{id: "sha256:" + strings.Repeat("d", 64), labels: map[string]string{ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel, managedRuntimeIDLabel: built.Runtime.ID, managedRuntimeRevisionLabel: revision}}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil {
		t.Fatal("fixture did not persist orphan staging blocker")
	}
	return runtime, runner, staging
}

func unknownOrphanRuntimeBuildFixture(t *testing.T) (*Runtime, *managedRuntimeBuildRunner, string) {
	t.Helper()
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
		t.Fatal(err)
	}
	built, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil)
	if err != nil {
		t.Fatal(err)
	}
	revision := built.Runtime.Revisions[0].Revision
	staging := managedRuntimeStagingImage(built.Runtime.ID, revision)
	runner.images[staging] = managedRuntimeTestImage{id: "sha256:" + strings.Repeat("d", 64), labels: map[string]string{ownerLabel: "foreign", componentLabel: managedRuntimeComponentLabel, managedRuntimeIDLabel: built.Runtime.ID, managedRuntimeRevisionLabel: revision}}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil {
		t.Fatal("fixture did not persist unknown orphan staging blocker")
	}
	return runtime, runner, staging
}

func TestRuntimeUnknownOrphanSettlementRequiresObservedAbsence(t *testing.T) {
	t.Run("foreign present remains unchanged", func(t *testing.T) {
		runtime, runner, _ := unknownOrphanRuntimeBuildFixture(t)
		before, err := os.ReadFile(runtime.runtimeBuildJournalPath())
		if err != nil {
			t.Fatal(err)
		}
		runsBefore := len(runner.runs)
		if err := runtime.RecoverRuntimeBuildOrphanStaging(context.Background()); err == nil {
			t.Fatal("present foreign orphan was settled")
		}
		after, err := os.ReadFile(runtime.runtimeBuildJournalPath())
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) || len(runner.runs) != runsBefore {
			t.Fatalf("foreign orphan changed authority or crossed mutation = %t/%+v", bytes.Equal(before, after), runner.runs[runsBefore:])
		}
	})

	t.Run("external removal settles without image mutation", func(t *testing.T) {
		runtime, runner, staging := unknownOrphanRuntimeBuildFixture(t)
		delete(runner.images, staging)
		runsBefore := len(runner.runs)
		if err := runtime.RecoverRuntimeBuildOrphanStaging(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(runner.runs) != runsBefore {
			t.Fatalf("absent unknown orphan crossed image mutation = %+v", runner.runs[runsBefore:])
		}
		if observed, err := runtime.readRuntimeBuildJournalObserved(); err != nil || observed != nil {
			t.Fatalf("absent unknown orphan retained journal = %+v/%v", observed, err)
		}
	})

	t.Run("late reappearance blocks completing cleanup", func(t *testing.T) {
		runtime, runner, staging := unknownOrphanRuntimeBuildFixture(t)
		foreign := runner.images[staging]
		delete(runner.images, staging)
		runtime.runtimeBuildCompletionWrite = func(completing runtimeBuildJournal) error {
			if err := writeAtomicJSON(runtime.runtimeBuildJournalPath(), completing); err != nil {
				return err
			}
			runner.images[staging] = foreign
			return nil
		}
		runsBefore := len(runner.runs)
		if err := runtime.RecoverRuntimeBuildOrphanStaging(context.Background()); err == nil {
			t.Fatal("late foreign reappearance crossed cleanup")
		}
		if len(runner.runs) != runsBefore {
			t.Fatalf("late foreign reappearance was mutated = %+v", runner.runs[runsBefore:])
		}
		observed, err := runtime.readRuntimeBuildJournalObserved()
		if err != nil || observed == nil || observed.Phase != runtimeBuildPhaseCompleting || observed.OrphanStaging != runtimeBuildOrphanAbsent || observed.RemoveStaging {
			t.Fatalf("late reappearance cleanup authority = %+v/%v", observed, err)
		}
		runtime.runtimeBuildCompletionWrite = nil
		delete(runner.images, staging)
		if err := runtime.RecoverRuntimeBuildCleanup(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestRuntimeBuildRecoveryByReferenceRejectsTargetOrKindDriftBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name       string
		runtimeRef string
		kind       tobari.RuntimeBuildRecoveryKind
	}{
		{name: "reference", runtimeRef: "018bcfe5-687b-7000-8000-000000000099", kind: tobari.RuntimeBuildRecoveryOrphan},
		{name: "kind", kind: tobari.RuntimeBuildRecoveryBuilding},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, runner, _ := exactOrphanRuntimeBuildFixture(t)
			journal, err := runtime.readRuntimeBuildJournalObserved()
			if err != nil || journal == nil {
				t.Fatalf("orphan journal = %+v/%v", journal, err)
			}
			ref := test.runtimeRef
			if ref == "" {
				ref = tobari.RuntimeRef(journal.RuntimeID)
			}
			before, err := os.ReadFile(runtime.runtimeBuildJournalPath())
			if err != nil {
				t.Fatal(err)
			}
			runsBefore := len(runner.runs)
			if err := runtime.RecoverRuntimeBuildByReference(context.Background(), ref, test.kind); err == nil {
				t.Fatal("drifted recovery target was accepted")
			}
			after, err := os.ReadFile(runtime.runtimeBuildJournalPath())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || len(runner.runs) != runsBefore {
				t.Fatalf("drifted recovery crossed mutation = bytes %t runs %+v", bytes.Equal(before, after), runner.runs[runsBefore:])
			}
		})
	}
}

func TestRuntimeOrphanStagingRecoveryPreservesDecisionAcrossCrashes(t *testing.T) {
	for _, afterWrite := range []bool{false, true} {
		t.Run(map[bool]string{false: "before completing write", true: "after completing write"}[afterWrite], func(t *testing.T) {
			runtime, runner, staging := exactOrphanRuntimeBuildFixture(t)
			runsBefore := len(runner.runs)
			runtime.runtimeBuildCompletionWrite = func(completing runtimeBuildJournal) error {
				if afterWrite {
					if err := writeAtomicJSON(runtime.runtimeBuildJournalPath(), completing); err != nil {
						return err
					}
				}
				return errors.New("synthetic orphan completion publication crash")
			}
			if afterWrite {
				runtime.runtimeBuildCleanup = func(runtimeBuildJournal) error {
					return errors.New("synthetic stop after orphan completion publication")
				}
			}
			if err := runtime.RecoverRuntimeBuildOrphanStaging(context.Background()); err == nil {
				t.Fatal("orphan completion publication crash was hidden")
			}
			observed, err := runtime.readRuntimeBuildJournalObserved()
			if err != nil || observed == nil {
				t.Fatalf("orphan completion crash authority = %+v/%v", observed, err)
			}
			if afterWrite {
				if observed.Phase != runtimeBuildPhaseCompleting || observed.CleanupFrom != runtimeBuildPhaseOrphanStaging || !observed.RemoveStaging || observed.OrphanStaging != runtimeBuildOrphanExactManaged {
					t.Fatalf("persisted orphan cleanup decision = %+v", observed)
				}
			} else if observed.Phase != runtimeBuildPhaseOrphanStaging {
				t.Fatalf("pre-write orphan authority = %+v", observed)
			}
			if len(runner.runs) != runsBefore {
				t.Fatalf("publication crash crossed image mutation = %+v", runner.runs[runsBefore:])
			}
			runtime.runtimeBuildCompletionWrite = nil
			runtime.runtimeBuildCleanup = nil
			if afterWrite {
				err = runtime.RecoverRuntimeBuildCleanup(context.Background())
			} else {
				err = runtime.RecoverRuntimeBuildOrphanStaging(context.Background())
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := runner.images[staging]; exists {
				t.Fatal("retried explicit orphan recovery retained staging")
			}
		})
	}

	for _, afterRemove := range []bool{false, true} {
		t.Run(map[bool]string{false: "image rm before effect failure", true: "image rm outcome unknown"}[afterRemove], func(t *testing.T) {
			runtime, runner, staging := exactOrphanRuntimeBuildFixture(t)
			runner.failImageRemove = true
			runner.removeThenFail = afterRemove
			if err := runtime.RecoverRuntimeBuildOrphanStaging(context.Background()); err == nil {
				t.Fatal("image removal uncertainty was hidden")
			}
			observed, err := runtime.readRuntimeBuildJournalObserved()
			if err != nil || observed == nil || observed.Phase != runtimeBuildPhaseCompleting || !observed.RemoveStaging || observed.CleanupFrom != runtimeBuildPhaseOrphanStaging {
				t.Fatalf("image removal uncertainty lost decision = %+v/%v", observed, err)
			}
			_, exists := runner.images[staging]
			if exists == afterRemove {
				t.Fatalf("image removal effect evidence exists=%t after_remove=%t", exists, afterRemove)
			}
			runner.failImageRemove = false
			runner.removeThenFail = false
			if err := runtime.RecoverRuntimeBuildCleanup(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, exists := runner.images[staging]; exists {
				t.Fatal("retried image removal retained staging")
			}
		})
	}

	t.Run("successful rm that leaves tag", func(t *testing.T) {
		runtime, runner, staging := exactOrphanRuntimeBuildFixture(t)
		runner.keepImageOnRemove = true
		if err := runtime.RecoverRuntimeBuildOrphanStaging(context.Background()); err == nil {
			t.Fatal("successful image rm that retained the tag was accepted")
		}
		observed, err := runtime.readRuntimeBuildJournalObserved()
		if err != nil || observed == nil || observed.Phase != runtimeBuildPhaseCompleting || !observed.RemoveStaging {
			t.Fatalf("retained-tag outcome lost cleanup authority = %+v/%v", observed, err)
		}
		if _, exists := runner.images[staging]; !exists {
			t.Fatal("retained-tag fixture removed staging")
		}
		runner.keepImageOnRemove = false
		if err := runtime.RecoverRuntimeBuildCleanup(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	for _, replacement := range []string{"foreign", "revision", "digest"} {
		t.Run(replacement+" replacement", func(t *testing.T) {
			runtime, runner, staging := exactOrphanRuntimeBuildFixture(t)
			runner.failImageRemove = true
			if err := runtime.RecoverRuntimeBuildOrphanStaging(context.Background()); err == nil {
				t.Fatal("pre-effect removal failure was hidden")
			}
			runner.failImageRemove = false
			image := runner.images[staging]
			switch replacement {
			case "foreign":
				image.labels[ownerLabel] = "foreign"
			case "revision":
				image.labels[managedRuntimeRevisionLabel] = "sha256:" + strings.Repeat("e", 64)
			case "digest":
				image.id = "sha256:" + strings.Repeat("e", 64)
			}
			runner.images[staging] = image
			runsBefore := len(runner.runs)
			if err := runtime.RecoverRuntimeBuildCleanup(context.Background()); err == nil {
				t.Fatal("replacement staging crossed cleanup")
			}
			if len(runner.runs) != runsBefore {
				t.Fatalf("replacement staging was mutated = %+v", runner.runs[runsBefore:])
			}
			observed, err := runtime.readRuntimeBuildJournalObserved()
			if err != nil || observed == nil || observed.Phase != runtimeBuildPhaseCompleting || !observed.RemoveStaging {
				t.Fatalf("replacement staging lost cleanup decision = %+v/%v", observed, err)
			}
		})
	}
}

func completingBuiltRuntimeFixture(t *testing.T) (*Runtime, *managedRuntimeBuildRunner, runtimeBuildJournal) {
	t.Helper()
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
		t.Fatal(err)
	}
	runtime.runtimeBuildCleanup = func(journal runtimeBuildJournal) error {
		if journal.CleanupFrom != runtimeBuildPhaseManifestCommitted {
			t.Fatalf("unexpected cleanup origin = %+v", journal)
		}
		return errors.New("synthetic crash after built completing publication")
	}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil {
		t.Fatal("built completion crash was hidden")
	}
	journal, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || journal == nil || journal.Phase != runtimeBuildPhaseCompleting || journal.CleanupFrom != runtimeBuildPhaseManifestCommitted || journal.RemoveStaging {
		t.Fatalf("built completing fixture = %+v/%v", journal, err)
	}
	runtime.runtimeBuildCleanup = nil
	return runtime, runner, *journal
}

func TestCompletingBuiltRecoveryRequiresExactCommittedAuthority(t *testing.T) {
	t.Run("exact", func(t *testing.T) {
		runtime, _, _ := completingBuiltRuntimeFixture(t)
		if err := runtime.RecoverRuntimeBuildCleanup(context.Background()); err != nil {
			t.Fatal(err)
		}
	})

	for _, drift := range []string{"manifest", "snapshot", "final tag missing", "final tag foreign", "final tag digest"} {
		t.Run(drift, func(t *testing.T) {
			runtime, runner, journal := completingBuiltRuntimeFixture(t)
			manifest, err := runtime.readRuntimeManifest(journal.RuntimeName)
			if err != nil {
				t.Fatal(err)
			}
			revision := manifest.Revisions[len(manifest.Revisions)-1]
			switch drift {
			case "manifest":
				manifest.Revisions = manifest.Revisions[:len(manifest.Revisions)-1]
				if err := writeAtomicJSON(runtime.runtimeManifestPath(manifest.Name), manifest); err != nil {
					t.Fatal(err)
				}
			case "snapshot":
				if err := os.Rename(revision.SnapshotPath, revision.SnapshotPath+".moved"); err != nil {
					t.Fatal(err)
				}
			case "final tag missing":
				delete(runner.images, journal.FinalImage)
			case "final tag foreign":
				image := runner.images[journal.FinalImage]
				image.labels[ownerLabel] = "foreign"
				runner.images[journal.FinalImage] = image
			case "final tag digest":
				image := runner.images[journal.FinalImage]
				image.id = "sha256:" + strings.Repeat("e", 64)
				runner.images[journal.FinalImage] = image
			}
			if err := runtime.RecoverRuntimeBuildCleanup(context.Background()); err == nil {
				t.Fatal("built cleanup erased drifted committed authority")
			}
			observed, err := runtime.readRuntimeBuildJournalObserved()
			if err != nil || observed == nil || *observed != journal {
				t.Fatalf("built drift lost completing authority = %+v/%v", observed, err)
			}
		})
	}
}

func postManifestBuiltRuntimeFixture(t *testing.T) (*Runtime, *managedRuntimeBuildRunner, runtimeBuildJournal) {
	t.Helper()
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime.runtimeBuildManifestWrite = func(path string, value any) error {
		if err := writeAtomicJSON(path, value); err != nil {
			return err
		}
		cancel()
		return nil
	}
	if _, err := runtime.BuildManagedRuntime(ctx, "frontend", nil); err == nil {
		t.Fatal("post-manifest cancellation was hidden")
	}
	runtime.runtimeBuildManifestWrite = nil
	journal, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || journal == nil || journal.Phase != runtimeBuildPhaseManifestCommitted {
		t.Fatalf("post-manifest built fixture = %+v/%v", journal, err)
	}
	return runtime, runner, *journal
}

func TestBuiltJournalHasReachableExactRecoveryAfterManifestCommit(t *testing.T) {
	t.Run("cancel then retry", func(t *testing.T) {
		runtime, _, _ := postManifestBuiltRuntimeFixture(t)
		if err := runtime.RecoverRuntimeBuildPublication(context.Background()); err != nil {
			t.Fatal(err)
		}
		if journal, err := runtime.readRuntimeBuildJournalObserved(); err != nil || journal != nil {
			t.Fatalf("built recovery retained journal = %+v/%v", journal, err)
		}
	})

	for _, drift := range []string{"manifest", "snapshot", "final image"} {
		t.Run(drift, func(t *testing.T) {
			runtime, runner, journal := postManifestBuiltRuntimeFixture(t)
			manifest, err := runtime.readRuntimeManifest(journal.RuntimeName)
			if err != nil {
				t.Fatal(err)
			}
			revision := manifest.Revisions[len(manifest.Revisions)-1]
			switch drift {
			case "manifest":
				manifest.Revisions = manifest.Revisions[:len(manifest.Revisions)-1]
				if err := writeAtomicJSON(runtime.runtimeManifestPath(manifest.Name), manifest); err != nil {
					t.Fatal(err)
				}
			case "snapshot":
				if err := os.Rename(revision.SnapshotPath, revision.SnapshotPath+".moved"); err != nil {
					t.Fatal(err)
				}
			case "final image":
				delete(runner.images, journal.FinalImage)
			}
			if err := runtime.RecoverRuntimeBuildPublication(context.Background()); err == nil {
				t.Fatal("drifted built journal entered cleanup")
			}
			observed, err := runtime.readRuntimeBuildJournalObserved()
			if err != nil || observed == nil || *observed != journal {
				t.Fatalf("drifted built journal was erased = %+v/%v", observed, err)
			}
		})
	}
}

func TestRuntimeBuildPublicationResumesEveryJournalBoundary(t *testing.T) {
	for _, target := range []string{runtimeBuildPhaseBuilt, runtimeBuildPhaseFinalTagged, runtimeBuildPhaseStagingReleased, runtimeBuildPhaseSnapshotPublished, runtimeBuildPhaseManifestCommitted} {
		for _, afterWrite := range []bool{false, true} {
			if target == runtimeBuildPhaseBuilt && !afterWrite {
				continue
			}
			t.Run(target+map[bool]string{false: "_before_phase_write", true: "_after_phase_write"}[afterWrite], func(t *testing.T) {
				root := t.TempDir()
				runner := newManagedRuntimeBuildRunner()
				runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
				if err != nil {
					t.Fatal(err)
				}
				created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
				if err != nil {
					t.Fatal(err)
				}
				runtime.runtimeBuildJournalWrite = func(_, next runtimeBuildJournal) error {
					if next.Phase != target {
						return writeAtomicJSON(runtime.runtimeBuildJournalPath(), next)
					}
					if afterWrite {
						if err := writeAtomicJSON(runtime.runtimeBuildJournalPath(), next); err != nil {
							return err
						}
					}
					return errors.New("synthetic process death at publication phase boundary")
				}
				if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil {
					t.Fatal("publication boundary crash was hidden")
				}
				journal, err := runtime.readRuntimeBuildJournalObserved()
				if err != nil || journal == nil {
					t.Fatalf("publication boundary lost journal = %+v/%v", journal, err)
				}
				runtime.runtimeBuildJournalWrite = nil
				if err := runtime.RecoverRuntimeBuildPublication(context.Background()); err != nil {
					t.Fatalf("resume %s after_write=%t: %v (journal=%+v)", target, afterWrite, err, journal)
				}
				manifest, err := runtime.readRuntimeManifest("frontend")
				if err != nil || manifest.ID != created.Runtime.ID || len(manifest.Revisions) != 1 {
					t.Fatalf("resumed publication manifest = %+v/%v", manifest, err)
				}
				if observed, err := runtime.readRuntimeBuildJournalObserved(); err != nil || observed != nil {
					t.Fatalf("resumed publication retained journal = %+v/%v", observed, err)
				}
			})
		}
	}
}

func assertRuntimeBuildRecoveryReleasesLocks(t *testing.T, runtime *Runtime) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		return runtime.withRuntimeStoreLock(lockContext, func() error { return nil })
	}); err != nil {
		t.Fatalf("recovery retained lifecycle/store lock: %v", err)
	}
}

func TestRuntimeBuildRecoveriesShareOneBoundedDockerContext(t *testing.T) {
	t.Run("pre-Docker", func(t *testing.T) {
		runtime, runner, _ := preparedRuntimeBuildJournalFixture(t)
		runtime.runtimeBuildRecoveryTimeout = 20 * time.Millisecond
		runner.blockInspect = true
		if err := runtime.RecoverRuntimeBuildPreDocker(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("pre-Docker recovery budget = %v", err)
		}
		runner.blockInspect = false
		assertRuntimeBuildRecoveryReleasesLocks(t, runtime)
	})

	t.Run("building inspect", func(t *testing.T) {
		runtime, runner, _ := exactBuildingRuntimeBuildFixture(t)
		runtime.runtimeBuildRecoveryTimeout = 20 * time.Millisecond
		runner.blockInspect = true
		if err := runtime.RecoverRuntimeBuildBuilding(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("building inspect recovery budget = %v", err)
		}
		runner.blockInspect = false
		assertRuntimeBuildRecoveryReleasesLocks(t, runtime)
	})

	t.Run("building compatibility", func(t *testing.T) {
		runtime, runner, _ := exactBuildingRuntimeBuildFixture(t)
		runtime.runtimeBuildRecoveryTimeout = 20 * time.Millisecond
		runner.blockCompatibility = true
		if err := runtime.RecoverRuntimeBuildBuilding(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("building compatibility recovery budget = %v", err)
		}
		runner.blockCompatibility = false
		assertRuntimeBuildRecoveryReleasesLocks(t, runtime)
	})

	t.Run("orphan", func(t *testing.T) {
		runtime, runner, _ := exactOrphanRuntimeBuildFixture(t)
		runtime.runtimeBuildRecoveryTimeout = 20 * time.Millisecond
		runner.blockInspect = true
		if err := runtime.RecoverRuntimeBuildOrphanStaging(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("orphan recovery budget = %v", err)
		}
		runner.blockInspect = false
		assertRuntimeBuildRecoveryReleasesLocks(t, runtime)
	})

	t.Run("cleanup", func(t *testing.T) {
		runtime, runner, _ := exactOrphanRuntimeBuildFixture(t)
		runner.failImageRemove = true
		if err := runtime.RecoverRuntimeBuildOrphanStaging(context.Background()); err == nil {
			t.Fatal("fixture did not enter completing cleanup")
		}
		runner.failImageRemove = false
		runner.blockInspect = true
		runtime.runtimeBuildRecoveryTimeout = 20 * time.Millisecond
		if err := runtime.RecoverRuntimeBuildCleanup(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("cleanup recovery budget = %v", err)
		}
		runner.blockInspect = false
		assertRuntimeBuildRecoveryReleasesLocks(t, runtime)
	})

	t.Run("publication", func(t *testing.T) {
		runtime, runner, _ := postManifestBuiltRuntimeFixture(t)
		runner.blockInspect = true
		runtime.runtimeBuildRecoveryTimeout = 20 * time.Millisecond
		if err := runtime.RecoverRuntimeBuildPublication(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("publication recovery budget = %v", err)
		}
		runner.blockInspect = false
		assertRuntimeBuildRecoveryReleasesLocks(t, runtime)
	})

	t.Run("settled failed", func(t *testing.T) {
		runtime, runner, building := exactBuildingRuntimeBuildFixture(t)
		failed := building
		failed.Phase = runtimeBuildPhaseFailed
		failed.ImageDigest = runner.images[building.StagingImage].id
		failed.StagingArtifact = runtimeBuildStagingOwned
		failed.AttemptSettlement = runtimeBuildAttemptSettled
		if err := runtime.writeRuntimeBuildJournal(building, failed); err != nil {
			t.Fatal(err)
		}
		runner.blockInspect = true
		runtime.runtimeBuildRecoveryTimeout = 20 * time.Millisecond
		if err := runtime.RecoverRuntimeBuildFailed(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("failed recovery budget = %v", err)
		}
		runner.blockInspect = false
		assertRuntimeBuildRecoveryReleasesLocks(t, runtime)
	})
}

func TestRuntimeBuildRecoveryBudgetIncludesLockAcquisition(t *testing.T) {
	runtime, _, _ := preparedRuntimeBuildJournalFixture(t)
	runtime.runtimeBuildRecoveryTimeout = 20 * time.Millisecond
	storeAcquired := make(chan struct{})
	releaseStore := make(chan struct{})
	storeDone := make(chan error, 1)
	go func() {
		storeDone <- runtime.withRuntimeStoreLock(context.Background(), func() error {
			close(storeAcquired)
			<-releaseStore
			return nil
		})
	}()
	<-storeAcquired
	if err := runtime.RecoverRuntimeBuildPreDocker(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("recovery lock-wait budget = %v", err)
	}
	close(releaseStore)
	if err := <-storeDone; err != nil {
		t.Fatal(err)
	}
	assertRuntimeBuildRecoveryReleasesLocks(t, runtime)
}

func TestRuntimeStoreLockWaitHonorsContextAndCreateLockOrder(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), newManagedRuntimeBuildRunner())
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runtime.withRuntimeStoreLock(context.Background(), func() error {
			close(acquired)
			<-release
			return nil
		})
	}()
	<-acquired
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := runtime.withRuntimeStoreLock(ctx, func() error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Runtime store lock cancellation = %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	lifecycleAttempted := false
	runtime.lifecycleLockAttempt = func() {
		mu.Lock()
		lifecycleAttempted = true
		mu.Unlock()
	}
	runtime.runtimeStoreLockAttempt = func() {
		mu.Lock()
		defer mu.Unlock()
		if !lifecycleAttempted {
			panic("Runtime store lock attempted before lifecycle lock")
		}
	}
	if _, err := runtime.CreateRuntime(context.Background(), "ordered", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
		t.Fatal(err)
	}
}

func TestManagedRuntimeBuildReferenceCannotRetargetSameNameReuse(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	old, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	oldRef := tobari.RuntimeRef(old.Runtime.ID)
	resolved, err := runtime.ResolveRuntimeReference(context.Background(), oldRef)
	if err != nil || resolved.ID != old.Runtime.ID {
		t.Fatalf("application resolution = %+v/%v", resolved, err)
	}

	// Model retirement and same-name recreation in the window between the
	// application read and the effect boundary. The effect must re-resolve the
	// stable ID while locked instead of inheriting authority from this name.
	if err := os.RemoveAll(runtime.runtimeDirectory("frontend")); err != nil {
		t.Fatal(err)
	}
	fresh, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Runtime.ID == old.Runtime.ID {
		t.Fatalf("same-name recreation reused Runtime ID %q", fresh.Runtime.ID)
	}
	manifestBefore, err := os.ReadFile(runtime.runtimeManifestPath("frontend"))
	if err != nil {
		t.Fatal(err)
	}
	sourceBefore, err := os.ReadFile(filepath.Join(fresh.Runtime.SourcePath, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runtime.BuildManagedRuntimeByReference(context.Background(), oldRef, nil); !errors.Is(err, tobari.ErrRuntimeNotFound) {
		t.Fatalf("retired Runtime build error = %v", err)
	}
	manifestAfter, err := os.ReadFile(runtime.runtimeManifestPath("frontend"))
	if err != nil {
		t.Fatal(err)
	}
	sourceAfter, err := os.ReadFile(filepath.Join(fresh.Runtime.SourcePath, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if string(manifestAfter) != string(manifestBefore) || string(sourceAfter) != string(sourceBefore) {
		t.Fatalf("fresh same-name Runtime changed: manifest=%t source=%t", string(manifestAfter) != string(manifestBefore), string(sourceAfter) != string(sourceBefore))
	}
	if len(runner.runs) != 0 {
		t.Fatalf("Docker build crossed stale Runtime authority: %+v", runner.runs)
	}
	if len(runner.outputs) != 0 {
		t.Fatalf("Docker inspection crossed stale Runtime authority: %+v", runner.outputs)
	}
}

func TestResolveRuntimeReferenceCannotRetargetSameNameReuse(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	old, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	oldRef := tobari.RuntimeRef(old.Runtime.ID)
	if err := os.RemoveAll(runtime.runtimeDirectory("frontend")); err != nil {
		t.Fatal(err)
	}
	fresh, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ResolveRuntimeReference(context.Background(), oldRef); !errors.Is(err, tobari.ErrRuntimeNotFound) {
		t.Fatalf("retired reference resolved after same-name replacement: %v", err)
	}
	resolved, err := runtime.ResolveRuntimeReference(context.Background(), tobari.RuntimeRef(fresh.Runtime.ID))
	if err != nil || resolved.ID != fresh.Runtime.ID {
		t.Fatalf("fresh reference resolution = %+v/%v", resolved, err)
	}
}

func TestRuntimeCreateCopiesManagedEditableBaseAsStandaloneSource(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	base, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(base.Runtime.SourcePath, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(bin, "tool")
	if err := os.WriteFile(tool, []byte("synthetic executable\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(base.Runtime.SourcePath, "empty")
	if err := os.Mkdir(empty, 0o500); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err != nil {
		t.Fatal(err)
	}

	created, err := runtime.CreateRuntime(context.Background(), "mobile", tobari.RuntimeCopySource("frontend"))
	if err != nil {
		t.Fatal(err)
	}
	if !created.Created || created.Runtime.ID == base.Runtime.ID || created.Runtime.Name != "mobile" ||
		len(created.Runtime.Revisions) != 0 || created.Runtime.SourcePath == base.Runtime.SourcePath {
		t.Fatalf("standalone created Runtime = %+v, base = %+v", created, base)
	}
	copiedTool := filepath.Join(created.Runtime.SourcePath, "bin", "tool")
	data, err := os.ReadFile(copiedTool)
	if err != nil || string(data) != "synthetic executable\n" {
		t.Fatalf("copied tool = %q/%v", data, err)
	}
	if info, err := os.Stat(copiedTool); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("copied tool mode = %v/%v", info, err)
	}
	if info, err := os.Stat(filepath.Join(created.Runtime.SourcePath, "empty")); err != nil || info.Mode().Perm() != 0o500 {
		t.Fatalf("copied empty directory mode = %v/%v", info, err)
	}
	if err := os.WriteFile(tool, []byte("later Base edit\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(copiedTool)
	if err != nil || string(data) != "synthetic executable\n" {
		t.Fatalf("target changed after Base edit = %q/%v", data, err)
	}
}

func TestRuntimeCreateFromMissingOrInvalidBasePublishesNoTarget(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, *Runtime)
		base  tobari.RuntimeCopySource
		code  string
	}{
		{name: "missing", setup: func(*testing.T, *Runtime) {}, base: "missing"},
		{name: "invalid source", setup: func(t *testing.T, runtime *Runtime) {
			created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("Dockerfile", filepath.Join(created.Runtime.SourcePath, "link")); err != nil {
				t.Fatal(err)
			}
		}, base: "frontend", code: "runtime_source_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
			if err != nil {
				t.Fatal(err)
			}
			test.setup(t, runtime)
			_, err = runtime.CreateRuntime(context.Background(), "mobile", test.base)
			if test.code == "" {
				if !errors.Is(err, tobari.ErrRuntimeNotFound) {
					t.Fatalf("missing Base error = %v", err)
				}
			} else if public, ok := fault.PublicCopy(err); !ok || public.Code != test.code {
				t.Fatalf("invalid Base source fault = %+v/%v", public, err)
			}
			if _, statErr := os.Lstat(runtime.runtimeDirectory("mobile")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("failed creation published target: %v", statErr)
			}
		})
	}
}

func TestRuntimeCreateCancellationPublishesNoTarget(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runtime.CreateRuntime(ctx, "mobile", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled create error = %v", err)
	}
	if _, err := os.Lstat(runtime.runtimeDirectory("mobile")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled creation published target: %v", err)
	}
}

func TestContextRuntimeSetPinsExactReadyRevision(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
		t.Fatal(err)
	}
	built, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil)
	if err != nil {
		t.Fatal(err)
	}

	selected, err := runtime.SetContextRuntime(context.Background(), "default", "frontend@1")
	if err != nil {
		t.Fatal(err)
	}
	want := built.Runtime.Revisions[0]
	if selected.Task != tobari.TaskManifestRuntimeSet || selected.Runtime.RuntimeID != built.Runtime.ID || selected.Runtime.Revision != want.Revision || selected.Image != want.Image {
		t.Fatalf("selected = %+v", selected)
	}

	rolledBack, err := runtime.SetContextRuntime(context.Background(), "default", tobari.StandardRuntimeName)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Runtime.RuntimeID != tobari.StandardRuntimeID || rolledBack.Runtime.Status != tobari.ManifestRuntimeStatusOfficial {
		t.Fatalf("rolled back = %+v", rolledBack)
	}
}

func TestRuntimeSourceRejectsSymlinksBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "unsafe", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("Dockerfile", filepath.Join(created.Runtime.SourcePath, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "unsafe", nil); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("build error = %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("Docker ran for unsafe source: %+v", runner.runs)
	}
}

func TestRuntimeSourceAcceptsPrivateBinaryWithinStreamedBounds(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "binary", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(created.Runtime.SourcePath, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(bin, "tool")
	file, err := os.OpenFile(tool, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(10 * 1024 * 1024); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	built, err := runtime.BuildManagedRuntime(context.Background(), "binary", nil)
	if err != nil || !built.Built || len(runner.runs) != 3 {
		t.Fatalf("binary build = %+v/%v runs=%d", built, err, len(runner.runs))
	}
	snapshot := filepath.Join(built.Runtime.Revisions[0].SnapshotPath, "bin", "tool")
	if info, err := os.Stat(snapshot); err != nil || info.Size() != 10*1024*1024 {
		t.Fatalf("binary snapshot = %v/%v", info, err)
	}
}

func TestRuntimeSourceSizeFailureReportsPathActualAndLimitBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "oversized", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(created.Runtime.SourcePath, "tool")
	file, err := os.OpenFile(tool, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxRuntimeSourceFile + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.BuildManagedRuntime(context.Background(), "oversized", nil)
	public, ok := fault.PublicCopy(err)
	for _, want := range []string{`"tool"`, "33554433 bytes", "33554432 bytes", "32 MiB"} {
		if !ok || !strings.Contains(public.Message, want) {
			t.Fatalf("source size fault = %+v/%v, missing %q", public, err, want)
		}
	}
	if public.Code != "runtime_source_invalid" || len(runner.runs) != 0 {
		t.Fatalf("source size code/runs = %q/%d", public.Code, len(runner.runs))
	}
}

func TestRuntimeSourcePermissionFailureReportsCorrectionBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "permissions", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(created.Runtime.SourcePath, "tool")
	if err := os.WriteFile(tool, []byte("synthetic"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tool, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.BuildManagedRuntime(context.Background(), "permissions", nil)
	public, ok := fault.PublicCopy(err)
	for _, want := range []string{`"tool"`, "0755", "group/other", "owner-only", "0600 or 0700"} {
		if !ok || !strings.Contains(public.Message, want) {
			t.Fatalf("source permission fault = %+v/%v, missing %q", public, err, want)
		}
	}
	if public.Code != "runtime_source_invalid" || len(runner.runs) != 0 {
		t.Fatalf("source permission code/runs = %q/%d", public.Code, len(runner.runs))
	}
}

func TestRuntimeSourceDirectoryPermissionFailureReportsCorrectionBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "directory-permissions", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(created.Runtime.SourcePath, "bin")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.BuildManagedRuntime(context.Background(), "directory-permissions", nil)
	public, ok := fault.PublicCopy(err)
	for _, want := range []string{`"bin"`, "0755", "group/other", "owner-only", "0700"} {
		if !ok || !strings.Contains(public.Message, want) {
			t.Fatalf("source directory permission fault = %+v/%v, missing %q", public, err, want)
		}
	}
	if public.Code != "runtime_source_invalid" || len(runner.runs) != 0 {
		t.Fatalf("source directory permission code/runs = %q/%d", public.Code, len(runner.runs))
	}
}

func TestRuntimeSourceTotalFailureReportsActualAndLimitBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "total", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	dockerfile, err := os.Stat(filepath.Join(created.Runtime.SourcePath, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	for name, size := range map[string]int64{
		"a": maxRuntimeSourceFile,
		"b": maxRuntimeSourceTotal - maxRuntimeSourceFile - dockerfile.Size() + 1,
	} {
		file, err := os.OpenFile(filepath.Join(created.Runtime.SourcePath, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(size); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}

	_, err = runtime.BuildManagedRuntime(context.Background(), "total", nil)
	public, ok := fault.PublicCopy(err)
	for _, want := range []string{`"b"`, "67108865 bytes", "67108864 bytes", "64 MiB"} {
		if !ok || !strings.Contains(public.Message, want) {
			t.Fatalf("source total fault = %+v/%v, missing %q", public, err, want)
		}
	}
	if public.Code != "runtime_source_invalid" || len(runner.runs) != 0 {
		t.Fatalf("source total code/runs = %q/%d", public.Code, len(runner.runs))
	}
}

func TestRuntimeSourceCountBoundsRejectBeforeDocker(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string)
		want  string
	}{
		{
			name: "files",
			setup: func(t *testing.T, source string) {
				for index := 0; index < maxRuntimeSourceFiles; index++ {
					path := filepath.Join(source, fmt.Sprintf("file-%04d", index))
					if err := os.WriteFile(path, nil, 0o600); err != nil {
						t.Fatal(err)
					}
				}
			},
			want: "contains 1025 files; the limit is 1024",
		},
		{
			name: "directories",
			setup: func(t *testing.T, source string) {
				for index := 0; index <= maxRuntimeSourceDirs; index++ {
					path := filepath.Join(source, fmt.Sprintf("dir-%03d", index))
					if err := os.Mkdir(path, 0o700); err != nil {
						t.Fatal(err)
					}
				}
			},
			want: "contains 257 directories; the limit is 256",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &recordingRunner{}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			created, err := runtime.CreateRuntime(context.Background(), test.name, tobari.RuntimeCopySource(tobari.StandardRuntimeName))
			if err != nil {
				t.Fatal(err)
			}
			test.setup(t, created.Runtime.SourcePath)
			_, err = runtime.BuildManagedRuntime(context.Background(), test.name, nil)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != "runtime_source_invalid" || !strings.Contains(public.Message, test.want) || len(runner.runs) != 0 {
				t.Fatalf("source count fault/runs = %+v/%v/%d", public, err, len(runner.runs))
			}
		})
	}
}
