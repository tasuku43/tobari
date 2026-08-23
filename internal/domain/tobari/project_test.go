package tobari

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

const testContextID = "018bcfe5-687b-7000-8000-000000000099"

func testDesiredEntry() DesiredEntry {
	digest := "sha256:" + strings.Repeat("a", 64)
	return DesiredEntry{ManifestGeneration: 1, ManifestRevision: digest, EntryRevision: digest, RuntimeID: StandardRuntimeID, RuntimeRevision: digest}
}

func testRootIndex(root, id string) RootIndex {
	return RootIndex{SchemaVersion: WorkspaceStateSchemaVersion, Root: root, InstanceID: id, WorkspaceManifestID: testContextID, WorkspaceManifestName: DefaultManifestName}
}

func TestNewProjectIDProducesUUIDv7(t *testing.T) {
	t.Parallel()
	id, err := NewWorkspaceID(time.UnixMilli(1_700_000_000_123), bytes.NewReader(make([]byte, 10)))
	if err != nil {
		t.Fatalf("NewWorkspaceID() error = %v", err)
	}
	if id != "018bcfe5-687b-7000-8000-000000000000" {
		t.Fatalf("NewWorkspaceID() = %q", id)
	}
	if err := ValidateWorkspaceID(id); err != nil {
		t.Fatalf("ValidateWorkspaceID() error = %v", err)
	}
}

func TestProjectInstanceDoesNotRequireRuntimeResources(t *testing.T) {
	t.Parallel()
	instance, err := NewProjectInstance(
		time.UnixMilli(1_700_000_000_123), bytes.NewReader(make([]byte, 10)), ProjectInstanceRequest{
			Root:                     "/workspace/project",
			WorkspaceManifestID:      "00000000-0000-7000-8000-000000000000",
			WorkspaceManifestName:    DefaultManifestName,
			Image:                    BuiltinImageSelector,
			CreationDefaultsRevision: "sha256:" + strings.Repeat("b", 64),
			CreatedAt:                time.Unix(1, 0).UTC(),
		},
	)
	if err != nil {
		t.Fatalf("NewProjectInstance() error = %v", err)
	}
	if instance.Runtime.ContainerID != "" || instance.Runtime.NetworkID != "" {
		t.Fatalf("new instance runtime = %+v, want absent diagnostic resources", instance.Runtime)
	}
	if instance.ID != "018bcfe5-687b-7000-8000-000000000000" ||
		instance.Root != "/workspace/project" || instance.WorkspaceManifestID != "00000000-0000-7000-8000-000000000000" ||
		instance.WorkspaceManifestName != DefaultManifestName || instance.Image != BuiltinImageSelector {
		t.Fatalf("new instance = %+v", instance)
	}
	if err := instance.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestProjectStatusKeepsLogicalExistenceSeparateFromRuntimeDiagnostic(t *testing.T) {
	t.Parallel()
	status := WorkspaceStatus{
		Task: TaskStatus, ManifestState: ManifestObservationPersisted, Exists: true, Root: "/tmp/project",
		ID: "01912345-6789-7abc-8def-0123456789ab", Home: "/tmp/state/home",
		Runtime:             RuntimeDiagnosticMissing,
		WorkspaceManifestID: testContextID, WorkspaceManifestName: DefaultManifestName,
		Attachment: AttachmentDetached,
		Adoption:   WorkspaceAdoptionNeverApplied, Next: ptrDesiredEntry(testDesiredEntry()),
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("WorkspaceStatus.Validate() error = %v", err)
	}
	if !status.Exists || status.Runtime != RuntimeDiagnosticMissing {
		t.Fatalf("status = %+v", status)
	}

	notExists := WorkspaceStatus{
		Task: TaskStatus, ManifestState: ManifestObservationPersisted, Runtime: RuntimeDiagnosticUnknown,
		WorkspaceManifestID: testContextID, WorkspaceManifestName: DefaultManifestName,
		Attachment: AttachmentNotApplicable,
	}
	if err := notExists.Validate(); err != nil {
		t.Fatalf("not-existing WorkspaceStatus.Validate() error = %v", err)
	}
}

func TestProjectStatusAllowsSyntheticDefaultOnlyForAbsentWorkspace(t *testing.T) {
	t.Parallel()
	status := WorkspaceStatus{
		Task: TaskStatus, ManifestState: ManifestObservationAbsent,
		WorkspaceManifestName: DefaultManifestName, Runtime: RuntimeDiagnosticUnknown,
		Attachment: AttachmentNotApplicable,
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("synthetic absent status = %v", err)
	}
	status.WorkspaceManifestID = testContextID
	if err := status.Validate(); err == nil {
		t.Fatal("synthetic absent status accepted authority ID")
	}
}

func TestProjectStatusAcceptsIncompleteLogicalStateDiagnostic(t *testing.T) {
	t.Parallel()
	status := WorkspaceStatus{
		Task: TaskStatus, ManifestState: ManifestObservationPersisted, Exists: true, Root: "/tmp/project",
		ID: "01912345-6789-7abc-8def-0123456789ab", Home: "/tmp/state/home",
		Runtime:             RuntimeDiagnosticIncomplete,
		WorkspaceManifestID: testContextID, WorkspaceManifestName: DefaultManifestName,
		Attachment: AttachmentDetached,
		Adoption:   WorkspaceAdoptionNeverApplied, Next: ptrDesiredEntry(testDesiredEntry()),
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("WorkspaceStatus.Validate() error = %v", err)
	}
}

func TestProjectListResultAcceptsKnownEmptyScope(t *testing.T) {
	t.Parallel()
	result := WorkspaceListResult{Task: TaskWorkspaceList, Items: []WorkspaceListItem{}}
	if err := result.Validate(); err != nil {
		t.Fatalf("WorkspaceListResult.Validate() error = %v", err)
	}
}

func TestProjectListResultValidatesCurrentIDAgainstItems(t *testing.T) {
	t.Parallel()
	item := WorkspaceListItem{
		Root: "/tmp/project", ID: "01912345-6789-7abc-8def-0123456789ab",
		Home: "/tmp/state/home", Runtime: RuntimeDiagnosticReady,
		WorkspaceManifestID: testContextID, WorkspaceManifestName: DefaultManifestName,
		Adoption: WorkspaceAdoptionNeverApplied, Next: testDesiredEntry(),
	}
	valid := WorkspaceListResult{
		Task: TaskWorkspaceList, CurrentID: item.ID, Items: []WorkspaceListItem{item},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid WorkspaceListResult.Validate() error = %v", err)
	}
	invalid := valid
	invalid.CurrentID = "01912345-6789-7abc-8def-0123456789ac"
	if err := invalid.Validate(); err == nil {
		t.Fatal("WorkspaceListResult.Validate() accepted a current ID absent from items")
	}
}

func TestProjectListResultRejectsDuplicateWorkspaceRoots(t *testing.T) {
	t.Parallel()
	item := WorkspaceListItem{
		Root: "/tmp/project", ID: "01912345-6789-7abc-8def-0123456789ab",
		Home: "/tmp/state/home", Runtime: RuntimeDiagnosticReady,
		WorkspaceManifestID: testContextID, WorkspaceManifestName: DefaultManifestName,
		Adoption: WorkspaceAdoptionNeverApplied, Next: testDesiredEntry(),
	}
	duplicate := item
	duplicate.ID = "01912345-6789-7abc-8def-0123456789ac"
	result := WorkspaceListResult{Task: TaskWorkspaceList, Items: []WorkspaceListItem{item, duplicate}}
	if err := result.Validate(); err == nil {
		t.Fatal("WorkspaceListResult.Validate() accepted duplicate canonical Workspace roots")
	}
}

func ptrDesiredEntry(value DesiredEntry) *DesiredEntry { return &value }

func TestNearestRootSelectsNearestAncestor(t *testing.T) {
	t.Parallel()
	indexes := []RootIndex{
		testRootIndex("/src/project", "018bcfe5-687b-7000-8000-000000000000"),
		testRootIndex("/src/project/internal", "018bcfe5-687b-7000-8000-000000000001"),
	}
	index, found, err := NearestRoot("/src/project/internal/cli", indexes)
	if err != nil {
		t.Fatalf("NearestRoot() error = %v", err)
	}
	if !found || index.Root != "/src/project/internal" {
		t.Fatalf("NearestRoot() = (%+v, %t), want nearest internal root", index, found)
	}
}

func TestContainingRootsReturnsEveryAncestorNearestFirst(t *testing.T) {
	t.Parallel()
	indexes := []RootIndex{
		testRootIndex("/src", "018bcfe5-687b-7000-8000-000000000000"),
		testRootIndex("/src/project", "018bcfe5-687b-7000-8000-000000000001"),
		testRootIndex("/src/project/internal", "018bcfe5-687b-7000-8000-000000000002"),
	}
	got, err := ContainingRoots("/src/project/internal/cli", indexes)
	if err != nil {
		t.Fatalf("ContainingRoots() error = %v", err)
	}
	want := []string{"/src/project/internal", "/src/project", "/src"}
	if len(got) != len(want) {
		t.Fatalf("ContainingRoots() = %+v, want %d roots", got, len(want))
	}
	for index, root := range want {
		if got[index].Root != root {
			t.Fatalf("ContainingRoots()[%d] = %q, want %q", index, got[index].Root, root)
		}
	}
}

func TestValidateRootIndexesRejectsDuplicateCanonicalRoots(t *testing.T) {
	t.Parallel()
	indexes := []RootIndex{
		testRootIndex("/src/project", "018bcfe5-687b-7000-8000-000000000000"),
		testRootIndex("/src/project", "018bcfe5-687b-7000-8000-000000000001"),
	}
	if err := ValidateRootIndexes(indexes); err == nil {
		t.Fatal("ValidateRootIndexes() accepted duplicate canonical roots")
	}
	if _, err := ContainingRoots("/src/project/internal", indexes); err == nil {
		t.Fatal("ContainingRoots() accepted duplicate canonical roots")
	}
}

func TestValidateRootIndexesRejectsDuplicateWorkspaceIDs(t *testing.T) {
	t.Parallel()
	indexes := []RootIndex{
		testRootIndex("/src/project", "018bcfe5-687b-7000-8000-000000000000"),
		testRootIndex("/src/project/internal", "018bcfe5-687b-7000-8000-000000000000"),
	}
	if err := ValidateRootIndexes(indexes); err == nil {
		t.Fatal("ValidateRootIndexes() accepted duplicate Workspace IDs")
	}
}

func TestProjectSelectionDistinguishesAncestorReuseAndExplicitCreate(t *testing.T) {
	t.Parallel()
	ancestor := WorkspaceSelectionCandidate{
		ID: "018bcfe5-687b-7000-8000-000000000000", Root: "/src/project",
		Runtime:             RuntimeDiagnosticReady,
		WorkspaceManifestID: testContextID, WorkspaceManifestName: DefaultManifestName,
	}
	selection := WorkspaceSelection{
		CWD: "/src/project/internal", Candidates: []WorkspaceSelectionCandidate{ancestor}, CanCreate: true,
	}
	if err := selection.Validate(); err != nil {
		t.Fatalf("WorkspaceSelection.Validate() error = %v", err)
	}
	if !selection.RequiresChoice() {
		t.Fatal("ancestor selection did not require a choice")
	}
	if err := selection.ValidateChoice(ProjectSelectionChoice{Kind: ProjectSelectionUse, ID: ancestor.ID}); err != nil {
		t.Fatalf("use choice rejected: %v", err)
	}
	if err := selection.ValidateChoice(ProjectSelectionChoice{Kind: ProjectSelectionCreate}); err != nil {
		t.Fatalf("create choice rejected: %v", err)
	}

	exact := selection
	exact.Candidates = []WorkspaceSelectionCandidate{{
		ID: ancestor.ID, Root: selection.CWD, Runtime: RuntimeDiagnosticReady, WorkspaceManifestID: testContextID, WorkspaceManifestName: DefaultManifestName,
	}}
	exact.CanCreate = false
	if err := exact.Validate(); err != nil {
		t.Fatalf("exact WorkspaceSelection.Validate() error = %v", err)
	}
	if exact.RequiresChoice() {
		t.Fatal("exact current-root selection unexpectedly required a choice")
	}
	if err := exact.ValidateChoice(ProjectSelectionChoice{Kind: ProjectSelectionCreate}); err == nil {
		t.Fatal("exact current-root selection accepted an explicit create choice")
	}
}

func TestProjectSelectionRejectsMissingAndIncompleteChoices(t *testing.T) {
	t.Parallel()
	selection := WorkspaceSelection{
		CWD: "/src/project/internal", CanCreate: true,
		Candidates: []WorkspaceSelectionCandidate{{
			ID: "018bcfe5-687b-7000-8000-000000000000", Root: "/src/project",
			Runtime:             RuntimeDiagnosticIncomplete,
			WorkspaceManifestID: testContextID, WorkspaceManifestName: DefaultManifestName,
		}},
	}
	if err := selection.Validate(); err != nil {
		t.Fatalf("WorkspaceSelection.Validate() error = %v", err)
	}
	for _, choice := range []ProjectSelectionChoice{
		{Kind: ProjectSelectionUse},
		{Kind: ProjectSelectionUse, ID: "018bcfe5-687b-7000-8000-000000000001"},
		{Kind: ProjectSelectionUse, ID: "018bcfe5-687b-7000-8000-000000000000"},
	} {
		if err := selection.ValidateChoice(choice); err == nil {
			t.Fatalf("selection accepted invalid choice %+v", choice)
		}
	}
}

func TestNearestRootRejectsPathPrefixConfusion(t *testing.T) {
	t.Parallel()
	indexes := []RootIndex{testRootIndex("/src/project", "018bcfe5-687b-7000-8000-000000000000")}
	_, found, err := NearestRoot("/src/project-other", indexes)
	if err != nil {
		t.Fatalf("NearestRoot() error = %v", err)
	}
	if found {
		t.Fatal("NearestRoot() matched a textual prefix outside the root")
	}
}

func TestProjectResourceNamesUseStableID(t *testing.T) {
	t.Parallel()
	container, network, err := ProjectResourceNames("018bcfe5-687b-7000-8000-000000000000")
	if err != nil {
		t.Fatalf("ProjectResourceNames() error = %v", err)
	}
	if container != "tobari-018bcfe5687b-work" || network != "tobari-018bcfe5687b-net" {
		t.Fatalf("ProjectResourceNames() = (%q, %q)", container, network)
	}
}

func TestMapProjectCWDUsesMirroredAbsoluteHostPath(t *testing.T) {
	t.Parallel()
	got, err := MapProjectCWD("/work", "/work/root")
	if err != nil {
		t.Fatalf("MapProjectCWD() error = %v", err)
	}
	if got != "/workspace/work/root" {
		t.Fatalf("MapProjectCWD() = %q, want /workspace/work/root", got)
	}
}

func TestMapProjectCWDRejectsSibling(t *testing.T) {
	t.Parallel()
	if _, err := MapProjectCWD("/work", "/work-other/root"); err == nil {
		t.Fatal("MapProjectCWD() accepted a sibling path")
	}
}
