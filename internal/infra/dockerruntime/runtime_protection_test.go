package dockerruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimeProtectionRunner struct {
	containerID string
	workspaceID string
	spec        string
	calls       [][]string
	fail        bool
}

type finalRuntimeProtectionSourceFixture struct {
	authority   tobari.FinalRuntimeProtectionAuthority
	authorities []tobari.FinalRuntimeProtectionAuthority
	err         error
	calls       int
}

func (s *finalRuntimeProtectionSourceFixture) ReadFinalRuntimeProtectionAuthority(context.Context) (tobari.FinalRuntimeProtectionAuthority, error) {
	s.calls++
	if len(s.authorities) != 0 {
		index := s.calls - 1
		if index >= len(s.authorities) {
			index = len(s.authorities) - 1
		}
		return s.authorities[index].Clone(), s.err
	}
	return s.authority.Clone(), s.err
}

func bindFinalRuntimeProtectionCollection(t *testing.T, runtime *Runtime, collection tobari.WorkspaceAuthorityCollection, present bool) *finalRuntimeProtectionSourceFixture {
	t.Helper()
	authority, err := tobari.NewFinalRuntimeProtectionAuthority(collection, present)
	if err != nil {
		t.Fatal(err)
	}
	source := &finalRuntimeProtectionSourceFixture{authority: authority}
	if err := runtime.BindFinalRuntimeProtectionSource(source); err != nil {
		t.Fatal(err)
	}
	return source
}

func bindEmptyFinalRuntimeProtection(t *testing.T, runtime *Runtime) *finalRuntimeProtectionSourceFixture {
	t.Helper()
	return bindFinalRuntimeProtectionCollection(t, runtime, tobari.WorkspaceAuthorityCollection{}, false)
}

func (r *runtimeProtectionRunner) Run(_ context.Context, args, _ []string, _ io.Reader, stdout, stderr io.Writer) error {
	r.calls = append(r.calls, slices.Clone(args))
	if r.fail {
		_, _ = io.WriteString(stderr, "synthetic Docker observation failure")
		return errors.New("synthetic Docker observation failure")
	}
	_, err := io.WriteString(stdout, `{"id":"`+r.containerID+`","owner":"`+ownerValue+`","component":"tobari","workspace":"`+r.workspaceID+`","role":"`+projectWorkRole+`","spec":"`+r.spec+`"}`)
	return err
}

func (r *runtimeProtectionRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.calls = append(r.calls, slices.Clone(args))
	if r.fail {
		return []byte("synthetic Docker observation failure"), errors.New("synthetic Docker observation failure")
	}
	return []byte(`{"id":"` + r.containerID + `","owner":"` + ownerValue + `","component":"tobari","workspace":"` + r.workspaceID + `","role":"` + projectWorkRole + `","spec":"` + r.spec + `"}`), nil
}

func newRuntimeProtectionFixture(t *testing.T, runner *runtimeProtectionRunner) (*Runtime, tobari.Workspace, tobari.WorkspaceManifest) {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(root, "project")
	if err := os.Mkdir(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, err := runtime.activeContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.stateDirectory, "lifecycle.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	bindEmptyFinalRuntimeProtection(t, runtime)
	return runtime, workspace, manifest
}

func TestRuntimeProtectionFreshObservationIsZeroWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &runtimeProtectionRunner{}
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	bindEmptyFinalRuntimeProtection(t, runtime)
	inventory, err := runtime.ReadRuntimeProtectionInventory(context.Background())
	if err != nil || !inventory.Complete || len(inventory.Items) != 0 {
		t.Fatalf("ReadRuntimeProtectionInventory() = %+v, %v", inventory, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 || len(runner.calls) != 0 {
		t.Fatalf("fresh protection observation mutated state: entries=%v calls=%v", entries, runner.calls)
	}
}

func TestLifecycleObservationRejectsStateAppearingDuringFreshRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &runtimeProtectionRunner{})
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.withLifecycleObservation(context.Background(), func(context.Context) error {
		return os.Mkdir(runtime.stateDirectory, 0o700)
	})
	var fault tobari.RuntimeProtectionInventoryError
	if !errors.As(err, &fault) || fault.Reason != tobari.RuntimeProtectionInventoryObservationUnknown {
		t.Fatalf("withLifecycleObservation() error = %v", err)
	}
}

func TestReadOnlyObservationLocksCloseOnCancellationBeforeAcquisition(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("Windows lock adapter does not contend")
	}
	for name, setup := range map[string]func(*testing.T, *Runtime) (string, func(context.Context) error){
		"lifecycle": func(t *testing.T, runtime *Runtime) (string, func(context.Context) error) {
			if err := os.MkdirAll(runtime.stateDirectory, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(runtime.stateDirectory, "lifecycle.lock")
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			return path, func(ctx context.Context) error {
				return runtime.withLifecycleObservation(ctx, func(context.Context) error {
					t.Fatal("canceled observation ran its action")
					return nil
				})
			}
		},
		"project": func(t *testing.T, runtime *Runtime) (string, func(context.Context) error) {
			if err := os.MkdirAll(runtime.stateDirectory, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(runtime.stateDirectory, "project.lock")
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			return path, func(ctx context.Context) error {
				return runtime.withExistingProjectLock(ctx, func() error {
					t.Fatal("canceled observation ran its action")
					return nil
				})
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &runtimeProtectionRunner{})
			if err != nil {
				t.Fatal(err)
			}
			path, observe := setup(t, runtime)
			holder, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			acquired, err := tryLockProjectFile(holder)
			if err != nil || !acquired {
				_ = holder.Close()
				t.Fatalf("hold test lock = %t, %v", acquired, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			err = observe(ctx)
			cancel()
			unlockProjectFile(holder)
			if closeErr := holder.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("canceled observation error = %v", err)
			}
			probe, err := os.OpenFile(path, os.O_RDONLY, 0)
			if err != nil {
				t.Fatalf("reopen observation lock after cancellation: %v", err)
			}
			if err := probe.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeProtectionRejectsStateWithoutLifecycleLockWithoutCreatingIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := filepath.Join(root, "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), state, filepath.Join(root, "data"), &runtimeProtectionRunner{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.ReadRuntimeProtectionInventory(context.Background())
	var fault tobari.RuntimeProtectionInventoryError
	if !errors.As(err, &fault) || fault.Reason != tobari.RuntimeProtectionInventoryObservationUnknown {
		t.Fatalf("ReadRuntimeProtectionInventory() error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(state, "lifecycle.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read created lifecycle lock: %v", err)
	}
}

func TestRuntimeProtectionOmitsStandardBindings(t *testing.T) {
	runtime, _, _ := newRuntimeProtectionFixture(t, &runtimeProtectionRunner{})
	inventory, err := runtime.ReadRuntimeProtectionInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Items) != 0 {
		t.Fatalf("standard Runtime entered managed lifecycle inventory: %+v", inventory.Items)
	}
}

func finalManagedRuntimeProtectionCollection(t *testing.T) (tobari.WorkspaceAuthorityCollection, string, tobari.SemanticDigest) {
	t.Helper()
	base := finalProjectionCollectionFixture(t, "")
	template := base.Templates[0]
	managedID := "018bcfe5-687b-7000-8000-000000000077"
	firstRuntimeRevision := tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64))
	body := template.Current.Body.Clone()
	body.EntryDefaults.Runtime = tobari.RuntimeBinding{RuntimeID: managedID, Name: "tools", Revision: string(firstRuntimeRevision), Ordinal: 1, Image: "tobari-runtime-tools:bbbbbbbbbbbb"}
	first, err := tobari.NewWorkspaceTemplateRevision(template.ID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	body.EntryDefaults.Runtime.Revision = "sha256:" + strings.Repeat("c", 64)
	body.EntryDefaults.Runtime.Ordinal = 2
	body.EntryDefaults.Runtime.Image = "tobari-runtime-tools:cccccccccccc"
	current, changed, err := tobari.AdvanceWorkspaceTemplateRevision(first, body)
	if err != nil || !changed {
		t.Fatalf("advance managed Template = %+v/%t/%v", current, changed, err)
	}
	template.Current = current
	template.Retained = []tobari.WorkspaceTemplateRevision{first.Clone(), current.Clone()}
	binding := base.Contexts[0].Context
	spec := tobari.SemanticDigest("sha256:" + strings.Repeat("d", 64))
	entry := tobari.WorkspaceAppliedEntry{
		ContextID: binding.ID, TemplateID: template.ID, TemplateRevision: first.Revision,
		EntrySliceDigest: first.Slices.EntrySliceDigest, RuntimeID: first.Slices.RuntimeID,
		RuntimeRevision: first.Slices.RuntimeRevision, ResolvedSpec: spec, ReconciledAt: time.Unix(4, 0).UTC(),
	}
	workspace := tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: finalProjectionWorkspaceA, ContextID: binding.ID,
		ProjectRoot: binding.ProjectRoot, Home: "/workspace/runtime-protection-home", CreationDefaults: first.Slices.CreationDefaultsDigest,
		LastSuccessfulEntry: &entry,
	}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{template}, base.Contexts, []tobari.WorkspaceBinding{workspace}, []tobari.PolicyCandidateAuthority{}, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return collection, strings.Repeat("e", 64), spec
}

func TestRuntimeProtectionUsesOnlyFinalTemplateContextWorkspaceAuthority(t *testing.T) {
	runner := &runtimeProtectionRunner{}
	root := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	collection, containerID, spec := finalManagedRuntimeProtectionCollection(t)
	bindFinalRuntimeProtectionCollection(t, runtime, collection, true)
	runner.containerID, runner.workspaceID, runner.spec = containerID, string(finalProjectionWorkspaceA), string(spec)
	inventory, err := runtime.ReadRuntimeProtectionInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantReasons := []tobari.RuntimeProtectionReason{
		tobari.RuntimeProtectedByTemplateCurrent, tobari.RuntimeProtectedByTemplateRetained,
		tobari.RuntimeProtectedByContextDesired,
		tobari.RuntimeProtectedByWorkspaceObserved, tobari.RuntimeProtectedByWorkspacePending,
	}
	for _, reason := range wantReasons {
		if !slices.ContainsFunc(inventory.Items, func(item tobari.RuntimeProtection) bool { return item.Reason == reason }) {
			t.Fatalf("final Runtime protection omitted %q: %+v", reason, inventory.Items)
		}
	}
	for _, item := range inventory.Items {
		if item.WorkspaceTemplateID != finalProjectionTemplateID || item.TemplateRevision == "" {
			t.Fatalf("protection did not retain exact final Template authority: %+v", item)
		}
		if item.Reason == tobari.RuntimeProtectedByContextDesired && (item.ContextID != finalProjectionContextID || item.WorkspaceID != "") {
			t.Fatalf("Context protection crossed final owner: %+v", item)
		}
		if item.Reason == tobari.RuntimeProtectedByWorkspaceObserved || item.Reason == tobari.RuntimeProtectedByWorkspacePending {
			if item.ContextID != finalProjectionContextID || item.WorkspaceID != finalProjectionWorkspaceA {
				t.Fatalf("Workspace protection crossed final owner: %+v", item)
			}
		}
	}
}

func TestRuntimeLifecycleRejectsFinalCollectionReceiptDriftWithSameProtectionContent(t *testing.T) {
	runner := &runtimeProtectionRunner{}
	root := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	collection, containerID, spec := finalManagedRuntimeProtectionCollection(t)
	defaultID := collection.Templates[0].ID
	next, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		collection.Templates, collection.Contexts, collection.Workspaces, collection.PendingCandidates, &defaultID, &collection,
	)
	if err != nil || !changed {
		t.Fatalf("publish protection-equivalent collection = %+v/%t/%v", next, changed, err)
	}
	first, err := tobari.NewFinalRuntimeProtectionAuthority(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tobari.NewFinalRuntimeProtectionAuthority(next, true)
	if err != nil {
		t.Fatal(err)
	}
	runtime.finalRuntimeProtectionSource = &finalRuntimeProtectionSourceFixture{authorities: []tobari.FinalRuntimeProtectionAuthority{first, second}}
	runner.containerID, runner.workspaceID, runner.spec = containerID, string(finalProjectionWorkspaceA), string(spec)
	_, _, err = runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	var fault tobari.RuntimeProtectionInventoryError
	if !errors.As(err, &fault) || fault.Reason != tobari.RuntimeProtectionInventoryObservationUnknown {
		t.Fatalf("collection receipt drift error = %v", err)
	}
}

func TestMissingRuntimeContainerInspectRequiresExactDiagnostic(t *testing.T) {
	t.Parallel()
	containerID := strings.Repeat("d", 64)
	err := errors.New("container inspect failed")
	for _, diagnostic := range []string{
		"No such container: " + containerID,
		"wrapper: Error response from daemon: No such container: " + containerID,
		"Error response from daemon: No such container: " + containerID + " (wrapped)",
		"Error response from daemon: No such container: " + containerID + "\nunrelated failure",
		"Error response from daemon: No such container: unrelated\nError response from daemon: No such container: " + containerID,
	} {
		if isMissingRuntimeContainerInspect(err, []byte(diagnostic), containerID) {
			t.Fatalf("diagnostic authorized container absence %q", diagnostic)
		}
	}
	if !isMissingRuntimeContainerInspect(err, []byte("Error response from daemon: No such container: "+containerID), containerID) {
		t.Fatal("exact missing-container diagnostic was rejected")
	}
	if isMissingRuntimeContainerInspect(nil, []byte("Error response from daemon: No such container: "+containerID), containerID) {
		t.Fatal("successful inspect authorized container absence")
	}
}

func TestRuntimeProtectionIgnoresPredecessorManifestWorkspaceFiles(t *testing.T) {
	runtime, _, _ := newRuntimeProtectionFixture(t, &runtimeProtectionRunner{})
	before, err := runtime.ReadRuntimeProtectionInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.contextsDirectory(), "predecessor-only"), []byte("not final authority"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.stateDirectory, "project-journal.json"), []byte("not final authority"), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := runtime.ReadRuntimeProtectionInventory(context.Background())
	if err != nil || !slices.Equal(before.Items, after.Items) {
		t.Fatalf("predecessor files influenced final Runtime protection: before=%+v after=%+v err=%v", before, after, err)
	}
}

func TestRuntimeProtectionOrderingIncludesRetainedTemplateRevision(t *testing.T) {
	base := tobari.RuntimeProtection{
		RuntimeID: "018bcfe5-687b-7000-8000-000000000077", RuntimeRevision: "sha256:" + strings.Repeat("d", 64),
		Reason: tobari.RuntimeProtectedByTemplateRetained, WorkspaceTemplateID: "01912345-6789-7abc-8def-0123456789ad",
	}
	first, second := base, base
	first.TemplateRevision = tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))
	second.TemplateRevision = tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64))
	items := []tobari.RuntimeProtection{second, first}
	slices.SortFunc(items, func(left, right tobari.RuntimeProtection) int {
		return strings.Compare(runtimeProtectionSortKey(left), runtimeProtectionSortKey(right))
	})
	if items[0].TemplateRevision != first.TemplateRevision || items[1].TemplateRevision != second.TemplateRevision {
		t.Fatalf("retained protection ordering = %+v", items)
	}
}

func TestRuntimeProtectionCanonicalizesRepeatedRetainedHistory(t *testing.T) {
	base := tobari.RuntimeProtection{
		RuntimeID: "018bcfe5-687b-7000-8000-000000000077", RuntimeRevision: "sha256:" + strings.Repeat("d", 64),
		Reason: tobari.RuntimeProtectedByTemplateRetained, WorkspaceTemplateID: "01912345-6789-7abc-8def-0123456789ad",
	}
	a, b := base, base
	a.TemplateRevision = tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))
	b.TemplateRevision = tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64))
	items := []tobari.RuntimeProtection{a, a, b, b}
	slices.SortFunc(items, func(left, right tobari.RuntimeProtection) int {
		return strings.Compare(runtimeProtectionSortKey(left), runtimeProtectionSortKey(right))
	})
	canonical, err := canonicalRuntimeProtectionItems(items)
	if err != nil || len(canonical) != 2 || canonical[0] != a || canonical[1] != b {
		t.Fatalf("A-B-A-B retained projection = %+v/%v", canonical, err)
	}
	if err := (tobari.RuntimeProtectionInventory{Complete: true, Items: canonical}).Validate(); err != nil {
		t.Fatalf("canonical retained projection is invalid: %v", err)
	}

	conflict := a
	conflict.RuntimeRevision = "sha256:" + strings.Repeat("e", 64)
	conflicting := []tobari.RuntimeProtection{a, conflict}
	slices.SortFunc(conflicting, func(left, right tobari.RuntimeProtection) int {
		return strings.Compare(runtimeProtectionSortKey(left), runtimeProtectionSortKey(right))
	})
	if _, err := canonicalRuntimeProtectionItems(conflicting); err == nil {
		t.Fatal("conflicting retained authority was deduplicated")
	}
}
