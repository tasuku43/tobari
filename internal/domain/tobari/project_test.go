package tobari

import (
	"bytes"
	"testing"
	"time"
)

const testContextID = "018bcfe5-687b-7000-8000-000000000099"

func testRootIndex(root, id string) RootIndex {
	return RootIndex{SchemaVersion: ProjectStateSchemaVersion, Root: root, InstanceID: id, ContextID: testContextID, ContextName: DefaultContextName}
}

func TestNewProjectIDProducesUUIDv7(t *testing.T) {
	t.Parallel()
	id, err := NewProjectID(time.UnixMilli(1_700_000_000_123), bytes.NewReader(make([]byte, 10)))
	if err != nil {
		t.Fatalf("NewProjectID() error = %v", err)
	}
	if id != "018bcfe5-687b-7000-8000-000000000000" {
		t.Fatalf("NewProjectID() = %q", id)
	}
	if err := ValidateProjectID(id); err != nil {
		t.Fatalf("ValidateProjectID() error = %v", err)
	}
}

func TestProjectInstanceDoesNotRequireRuntimeResources(t *testing.T) {
	t.Parallel()
	instance, err := NewProjectInstance(
		time.UnixMilli(1_700_000_000_123), bytes.NewReader(make([]byte, 10)), "/workspace/project", BuiltinImageSelector,
	)
	if err != nil {
		t.Fatalf("NewProjectInstance() error = %v", err)
	}
	if instance.Runtime.ContainerID != "" || instance.Runtime.NetworkID != "" {
		t.Fatalf("new instance runtime = %+v, want absent diagnostic resources", instance.Runtime)
	}
	if err := instance.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestProjectStatusKeepsLogicalExistenceSeparateFromRuntimeDiagnostic(t *testing.T) {
	t.Parallel()
	status := ProjectStatus{
		Task: TaskStatus, Exists: true, Root: "/tmp/project",
		ID: "01912345-6789-7abc-8def-0123456789ab", Home: "/tmp/state/home",
		Runtime:   RuntimeDiagnosticMissing,
		ContextID: testContextID, ContextName: DefaultContextName,
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("ProjectStatus.Validate() error = %v", err)
	}
	if !status.Exists || status.Runtime != RuntimeDiagnosticMissing {
		t.Fatalf("status = %+v", status)
	}

	notExists := ProjectStatus{Task: TaskStatus, Runtime: RuntimeDiagnosticUnknown}
	if err := notExists.Validate(); err != nil {
		t.Fatalf("not-existing ProjectStatus.Validate() error = %v", err)
	}
}

func TestProjectStatusAcceptsIncompleteLogicalStateDiagnostic(t *testing.T) {
	t.Parallel()
	status := ProjectStatus{
		Task: TaskStatus, Exists: true, Root: "/tmp/project",
		ID: "01912345-6789-7abc-8def-0123456789ab", Home: "/tmp/state/home",
		Runtime:   RuntimeDiagnosticIncomplete,
		ContextID: testContextID, ContextName: DefaultContextName,
	}
	if err := status.Validate(); err != nil {
		t.Fatalf("ProjectStatus.Validate() error = %v", err)
	}
}

func TestProjectListResultAcceptsKnownEmptyScope(t *testing.T) {
	t.Parallel()
	result := ProjectListResult{Task: TaskProjectList, Items: []ProjectListItem{}}
	if err := result.Validate(); err != nil {
		t.Fatalf("ProjectListResult.Validate() error = %v", err)
	}
}

func TestProjectListResultValidatesCurrentIDAgainstItems(t *testing.T) {
	t.Parallel()
	item := ProjectListItem{
		Root: "/tmp/project", ID: "01912345-6789-7abc-8def-0123456789ab",
		Home: "/tmp/state/home", Runtime: RuntimeDiagnosticReady,
		ContextID: testContextID, ContextName: DefaultContextName,
	}
	valid := ProjectListResult{
		Task: TaskProjectList, CurrentID: item.ID, Items: []ProjectListItem{item},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid ProjectListResult.Validate() error = %v", err)
	}
	invalid := valid
	invalid.CurrentID = "01912345-6789-7abc-8def-0123456789ac"
	if err := invalid.Validate(); err == nil {
		t.Fatal("ProjectListResult.Validate() accepted a current ID absent from items")
	}
}

func TestProjectListResultRejectsDuplicateWorkspaceRoots(t *testing.T) {
	t.Parallel()
	item := ProjectListItem{
		Root: "/tmp/project", ID: "01912345-6789-7abc-8def-0123456789ab",
		Home: "/tmp/state/home", Runtime: RuntimeDiagnosticReady,
		ContextID: testContextID, ContextName: DefaultContextName,
	}
	duplicate := item
	duplicate.ID = "01912345-6789-7abc-8def-0123456789ac"
	result := ProjectListResult{Task: TaskProjectList, Items: []ProjectListItem{item, duplicate}}
	if err := result.Validate(); err == nil {
		t.Fatal("ProjectListResult.Validate() accepted duplicate canonical Workspace roots")
	}
}

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
	ancestor := ProjectSelectionCandidate{
		ID: "018bcfe5-687b-7000-8000-000000000000", Root: "/src/project",
		Runtime:   RuntimeDiagnosticReady,
		ContextID: testContextID, ContextName: DefaultContextName,
	}
	selection := ProjectSelection{
		CWD: "/src/project/internal", Candidates: []ProjectSelectionCandidate{ancestor}, CanCreate: true,
	}
	if err := selection.Validate(); err != nil {
		t.Fatalf("ProjectSelection.Validate() error = %v", err)
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
	exact.Candidates = []ProjectSelectionCandidate{{
		ID: ancestor.ID, Root: selection.CWD, Runtime: RuntimeDiagnosticReady, ContextID: testContextID, ContextName: DefaultContextName,
	}}
	exact.CanCreate = false
	if err := exact.Validate(); err != nil {
		t.Fatalf("exact ProjectSelection.Validate() error = %v", err)
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
	selection := ProjectSelection{
		CWD: "/src/project/internal", CanCreate: true,
		Candidates: []ProjectSelectionCandidate{{
			ID: "018bcfe5-687b-7000-8000-000000000000", Root: "/src/project",
			Runtime:   RuntimeDiagnosticIncomplete,
			ContextID: testContextID, ContextName: DefaultContextName,
		}},
	}
	if err := selection.Validate(); err != nil {
		t.Fatalf("ProjectSelection.Validate() error = %v", err)
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
