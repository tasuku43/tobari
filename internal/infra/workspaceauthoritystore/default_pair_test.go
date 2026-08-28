package workspaceauthoritystore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type defaultPairRootFixture struct {
	cwd    string
	root   string
	inside bool
}

func (f defaultPairRootFixture) CurrentDirectory(context.Context) (string, error) { return f.cwd, nil }
func (f defaultPairRootFixture) ResolveProjectRoot(context.Context, string) (string, error) {
	return f.root, nil
}
func (f defaultPairRootFixture) InsideProject(context.Context) bool { return f.inside }

func TestDefaultPairAdapterRequiresGuardedFinalStoreAndFreshReadCreatesNothing(t *testing.T) {
	authorityRoot := filepath.Join(t.TempDir(), "final-authority")
	raw, err := New(authorityRoot)
	if err != nil {
		t.Fatal(err)
	}
	root := defaultPairRootFixture{cwd: "/workspace/fresh/subdir", root: "/workspace/fresh"}
	if _, err := NewDefaultPairAdapter(raw, root); err == nil {
		t.Fatal("default-pair adapter accepted an unguarded raw Store")
	}
	guard := &legacyGuardFake{}
	store, err := NewFinalOnly(authorityRoot, guard)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewDefaultPairAdapter(store, root)
	if err != nil {
		t.Fatal(err)
	}
	projectRoot, err := adapter.ObserveFinalCanonicalProjectRoot(context.Background())
	if err != nil || projectRoot != root.root {
		t.Fatalf("canonical root=%q err=%v", projectRoot, err)
	}
	observation, err := adapter.ObserveFinalDefaultPair(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if observation.CollectionPresent || observation.ProjectRoot != root.root || observation.DefaultTemplate != nil || observation.Context != nil || guard.calls != 2 {
		t.Fatalf("fresh final observation=%+v guard_calls=%d", observation, guard.calls)
	}
	if _, err := os.Lstat(authorityRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fresh default-pair read created final authority: %v", err)
	}
}

func TestDefaultPairAdapterReadsOnlyCompleteFinalAuthority(t *testing.T) {
	collection := storeCollectionFixture(t)
	authorityRoot := filepath.Join(t.TempDir(), "final-authority")
	materializeCollection(t, authorityRoot, collection)
	guard := &legacyGuardFake{}
	store, err := NewFinalOnly(authorityRoot, guard)
	if err != nil {
		t.Fatal(err)
	}
	root := defaultPairRootFixture{cwd: collection.Contexts[0].Context.ProjectRoot, root: collection.Contexts[0].Context.ProjectRoot}
	adapter, err := NewDefaultPairAdapter(store, root)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := adapter.ObserveFinalDefaultPair(context.Background(), root.root)
	if err != nil {
		t.Fatal(err)
	}
	if !observation.CollectionPresent || observation.CollectionGeneration != collection.Generation || observation.CollectionRevision != collection.Revision || observation.DefaultTemplate == nil || observation.DefaultTemplate.ID != *collection.DefaultTemplateID || observation.Context == nil || observation.Context.Context.ID != collection.Contexts[0].Context.ID || guard.calls != 2 {
		t.Fatalf("final-only observation=%+v guard_calls=%d", observation, guard.calls)
	}
}

func TestDefaultPairAdapterObservesAncestorCandidatesWithoutSelectingOrCreating(t *testing.T) {
	collection := storeCollectionFixture(t)
	authorityRoot := filepath.Join(t.TempDir(), "final-authority")
	materializeCollection(t, authorityRoot, collection)
	guard := &legacyGuardFake{}
	store, err := NewFinalOnly(authorityRoot, guard)
	if err != nil {
		t.Fatal(err)
	}
	ancestor := collection.Contexts[0].Context.ProjectRoot
	cwd := filepath.Join(ancestor, "src", "pkg")
	adapter, err := NewDefaultPairAdapter(store, defaultPairRootFixture{cwd: cwd, root: cwd})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := adapter.ObserveFinalDefaultPairSelection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if selection.CanonicalCWD != cwd || !selection.RequiresChoice() || len(selection.Candidates) != 1 || selection.Candidates[0].Snapshot.Context.ProjectRoot != ancestor {
		t.Fatalf("ancestor selection=%+v", selection)
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || !reflect.DeepEqual(after, collection) {
		t.Fatalf("selection changed final authority: present=%t err=%v", present, err)
	}
}

func TestDefaultPairAdapterReportsWorkspacePresenceWithoutStateRead(t *testing.T) {
	authorityRoot := filepath.Join(t.TempDir(), "final-authority")
	store, err := NewFinalOnly(authorityRoot, &legacyGuardFake{})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewDefaultPairAdapter(store, defaultPairRootFixture{cwd: "/workspace/example", root: "/workspace/example", inside: true})
	if err != nil {
		t.Fatal(err)
	}
	if !adapter.InsideFinalWorkspace(context.Background()) {
		t.Fatal("Workspace process presence was lost")
	}
	if _, err := os.Lstat(authorityRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("presence observation touched final authority: %v", err)
	}
}

func TestDefaultPairAdapterLegacyFailureDoesNotDecodeOrMutatePredecessorBytes(t *testing.T) {
	temporary := t.TempDir()
	authorityRoot := filepath.Join(temporary, "final-authority")
	legacyPath := filepath.Join(temporary, "legacy-contexts")
	hostile := []byte(`{"schema_version":999,"truncated":`)
	if err := os.WriteFile(legacyPath, hostile, 0o600); err != nil {
		t.Fatal(err)
	}
	guard := &legacyGuardFake{errors: []error{errors.New("legacy authority is present")}}
	store, err := NewFinalOnly(authorityRoot, guard)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewDefaultPairAdapter(store, defaultPairRootFixture{cwd: "/workspace/fresh", root: "/workspace/fresh"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ObserveFinalDefaultPair(context.Background(), "/workspace/fresh"); !errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
		t.Fatalf("legacy observation error=%v", err)
	}
	if guard.calls != 1 {
		t.Fatalf("legacy guard calls=%d, want one fail-closed observation", guard.calls)
	}
	after, err := os.ReadFile(legacyPath)
	if err != nil || !reflect.DeepEqual(after, hostile) {
		t.Fatalf("legacy bytes changed or were replaced: bytes=%q err=%v", after, err)
	}
	if _, err := os.Lstat(authorityRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy rejection created final authority: %v", err)
	}
}

func TestInitializeFinalDefaultPairPublishesOneCompleteEnvelopeAndReplaysExactly(t *testing.T) {
	store, mutator, lifecycle, _, _ := newMutationFixture(t, nil)
	body := storeCollectionFixture(t).Templates[0].Current.Body
	publication, err := mutator.seedFinalDefaultPairForLegacyMigration(context.Background(), "/workspace/fresh", body)
	if err != nil {
		t.Fatal(err)
	}
	if !publication.Changed || lifecycle.attempts.Load() != 1 {
		t.Fatalf("fresh initialization disposition mismatch: publication=%+v lifecycle=%d", publication, lifecycle.attempts.Load())
	}
	collection, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || len(collection.Templates) != 1 || len(collection.Contexts) != 1 || len(collection.Workspaces) != 0 || collection.DefaultTemplateID == nil || *collection.DefaultTemplateID != collection.Templates[0].ID || collection.Contexts[0].Context.ProjectRoot != "/workspace/fresh" {
		t.Fatalf("fresh complete envelope mismatch: present=%t collection=%+v err=%v", present, collection, err)
	}
	replay, err := mutator.seedFinalDefaultPairForLegacyMigration(context.Background(), "/workspace/fresh", body)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Changed || !reflect.DeepEqual(replay.Previous, replay.Current) || lifecycle.attempts.Load() != 2 {
		t.Fatalf("default-pair replay was not exact no-op: publication=%+v lifecycle=%d", replay, lifecycle.attempts.Load())
	}
}

func TestInitializeFinalDefaultPairPreservesExistingActiveWorkspaceOnExactNoOp(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, lifecycle, _, _ := newMutationFixture(t, &existing)
	publication, err := mutator.seedFinalDefaultPairForLegacyMigration(context.Background(), existing.Contexts[0].Context.ProjectRoot, existing.Templates[0].Current.Body)
	if err != nil {
		t.Fatal(err)
	}
	if publication.Changed || !reflect.DeepEqual(publication.Previous, publication.Current) || publication.Current.Context == nil || publication.Current.Context.Workspace == nil || publication.Current.Context.ActiveTemplatePolicy == nil || publication.Current.Context.ActivePolicyMemory == nil || publication.Current.Context.ActivePolicyMemoryRef == nil || lifecycle.attempts.Load() != 1 {
		t.Fatalf("existing active default pair was not preserved exactly: publication=%+v lifecycle=%d", publication, lifecycle.attempts.Load())
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || !reflect.DeepEqual(existing, after) {
		t.Fatalf("exact no-op changed existing authority: present=%t err=%v\nwant=%+v\ngot=%+v", present, err, existing, after)
	}
}

func TestInitializeFinalDefaultPairRejectsInitializedAuthorityWithoutDefaultZeroWrite(t *testing.T) {
	base := storeCollectionFixture(t)
	withoutDefault, changed, err := tobari.PublishWorkspaceAuthorityCollection(base.Templates, []tobari.WorkspaceAuthorityContextRecord{}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, &base)
	if err != nil || !changed {
		t.Fatal(err)
	}
	store, mutator, _, _, _ := newMutationFixture(t, &withoutDefault)
	before, _, err := store.ReadComplete(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = mutator.seedFinalDefaultPairForLegacyMigration(context.Background(), "/workspace/new", base.Templates[0].Current.Body)
	if !errors.Is(err, tobari.ErrDefaultTemplateSelectionRequired) {
		t.Fatalf("missing default error=%v", err)
	}
	after, _, err := store.ReadComplete(context.Background())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("missing-default rejection changed authority: err=%v\nbefore=%+v\nafter=%+v", err, before, after)
	}
}

func TestInitializeFinalDefaultPairClassifiesRenameSuccessDespiteCancellation(t *testing.T) {
	store, mutator, _, _, _ := newMutationFixture(t, nil)
	body := storeCollectionFixture(t).Templates[0].Current.Body
	originalRename := mutator.rename
	mutator.rename = func(oldPath, newPath string) error {
		if err := originalRename(oldPath, newPath); err != nil {
			return err
		}
		return context.Canceled
	}
	publication, err := mutator.seedFinalDefaultPairForLegacyMigration(context.Background(), "/workspace/fresh", body)
	if err != nil || !publication.Changed {
		t.Fatalf("confirmed rename was not classified as success: publication=%+v err=%v", publication, err)
	}
	if _, present, err := store.ReadComplete(context.Background()); err != nil || !present {
		t.Fatalf("confirmed initialization is unavailable: present=%t err=%v", present, err)
	}
	if _, err := os.Stat(mutationStagePath(store.root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("initialization stage remains after confirmed rename: %v", err)
	}
}
