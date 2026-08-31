package dockerruntime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func finalRuntimeMaterialFixture(t *testing.T, state string) (*Runtime, *lifecycleObservationRunner, tobari.WorkspaceAuthorityCollection, tobari.RuntimeBinding) {
	t.Helper()
	root := t.TempDir()
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.runtimesDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}

	const runtimeID = "018bcfe5-687b-7000-8000-000000000077"
	const runtimeName = "doctor-tools"
	content := "FROM example.invalid/runtime\n"
	revision := runtimeLifecycleFixtureRevision(t, content)
	imageDigest := "sha256:" + strings.Repeat("b", 64)
	binding := tobari.RuntimeBinding{
		RuntimeID: runtimeID, Name: runtimeName, Revision: revision, Ordinal: 1,
		Image: managedLibraryRuntimeImage(runtimeName, runtimeID, revision),
	}

	base := finalProjectionCollectionFixture(t, "")
	template := base.Templates[0]
	body := template.Current.Body.Clone()
	body.EntryDefaults.Runtime = binding
	templateRevision, err := tobari.NewWorkspaceTemplateRevision(template.ID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template.Current = templateRevision
	template.Retained = []tobari.WorkspaceTemplateRevision{templateRevision.Clone()}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{template}, base.Contexts, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	bindFinalRuntimeProtectionCollection(t, runtime, collection, true)
	manifest := installRuntimeLifecycleRevision(t, runtime, runtimeID, runtimeName, imageDigest, content)
	if observed, err := manifest.Binding(1); err != nil || !reflect.DeepEqual(observed, binding) {
		t.Fatalf("installed binding=%+v want=%+v err=%v", observed, binding, err)
	}

	image := managedLifecycleImage(runtimeID, revision, binding.Image)
	switch state {
	case "available":
		runner.images[binding.Image] = lifecycleImageFixture{observation: image}
	case "missing":
		runner.images[binding.Image] = lifecycleImageFixture{missing: true}
	case "unknown":
		image.Component = "foreign"
		runner.images[binding.Image] = lifecycleImageFixture{observation: image}
	default:
		t.Fatalf("unsupported material state %q", state)
	}
	return runtime, runner, collection, binding
}

func TestObserveFinalRuntimeMaterialsRequiresAvailableOwnedCompleteCompatibleMaterial(t *testing.T) {
	for _, test := range []struct {
		name    string
		wantErr bool
	}{
		{name: "available"},
		{name: "missing", wantErr: true},
		{name: "unknown", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, runner, collection, binding := finalRuntimeMaterialFixture(t, test.name)
			observed, err := runtime.ObserveFinalRuntimeMaterials(context.Background(), collection)
			if (err != nil) != test.wantErr {
				t.Fatalf("ObserveFinalRuntimeMaterials() = %+v, %v", observed, err)
			}
			if !test.wantErr && (len(observed) != 1 || !reflect.DeepEqual(observed[0], binding)) {
				t.Fatalf("available bindings = %+v, want %+v", observed, binding)
			}
			if test.name != "missing" && runner.imageObservations != 2 {
				t.Fatalf("lifecycle image observations = %d, want one coherent before/after snapshot", runner.imageObservations)
			}
		})
	}
}

func TestObserveFinalRuntimeMaterialsRejectsNonCanonicalTemplateBinding(t *testing.T) {
	runtime, _, collection, _ := finalRuntimeMaterialFixture(t, "available")
	template := collection.Templates[0]
	body := template.Current.Body.Clone()
	body.EntryDefaults.Runtime.Image = "tobari-runtime-doctor-tools:other"
	revision, err := tobari.NewWorkspaceTemplateRevision(template.ID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template.Current = revision
	template.Retained = []tobari.WorkspaceTemplateRevision{revision.Clone()}
	drifted, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{template}, collection.Contexts, collection.Workspaces, collection.PendingCandidates, collection.DefaultTemplateID, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	// Bind the exact changed complete collection. The lifecycle catalog still
	// rejects its selector metadata by DeepEqual; no name/selector fallback is
	// allowed to turn the same ID+revision into a pass.
	authority, err := tobari.NewFinalRuntimeProtectionAuthority(drifted, true)
	if err != nil {
		t.Fatal(err)
	}
	runtime.finalRuntimeProtectionSource = &finalRuntimeProtectionSourceFixture{authority: authority}
	if _, err := runtime.ObserveFinalRuntimeMaterials(context.Background(), drifted); err == nil {
		t.Fatal("non-canonical final Template Runtime binding passed material observation")
	}
}

func TestObserveFinalRuntimeMaterialsAcceptsExactHistoricalStandardBinding(t *testing.T) {
	root := t.TempDir()
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	binding := historicalStandardRuntimeBinding(strings.Repeat("3", 64))
	base := finalProjectionCollectionFixture(t, "")
	template := base.Templates[0]
	body := template.Current.Body.Clone()
	body.EntryDefaults.Runtime = binding
	revision, err := tobari.NewWorkspaceTemplateRevision(template.ID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template.Current = revision
	template.Retained = []tobari.WorkspaceTemplateRevision{revision.Clone()}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{template}, base.Contexts, base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	bindFinalRuntimeProtectionCollection(t, runtime, collection, true)
	observed, err := runtime.ObserveFinalRuntimeMaterials(context.Background(), collection)
	if err != nil || len(observed) != 1 || !reflect.DeepEqual(observed[0], binding) {
		t.Fatalf("historical standard bindings=%+v err=%v", observed, err)
	}
}

func TestObserveFinalRuntimeMaterialsFreshEmptyIsExactZeroWrite(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &lifecycleObservationRunner{})
	if err != nil {
		t.Fatal(err)
	}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{}, []tobari.WorkspaceAuthorityContextRecord{}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := runtime.ObserveFinalRuntimeMaterials(context.Background(), collection)
	if err != nil || observed == nil || len(observed) != 0 {
		t.Fatalf("fresh observation = %+v, %v", observed, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("fresh material observation created state: %v", entries)
	}
}
