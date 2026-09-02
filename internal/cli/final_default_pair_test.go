package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/statuscmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type statusHomePortFixture struct{ observation tobari.StatusHomeObservation }

func (f statusHomePortFixture) ObserveStatusHome(context.Context) (tobari.StatusHomeObservation, error) {
	return f.observation, nil
}

func TestStatusHomeFreshJSONAndHumanAreCWDFirstAndZeroAuthority(t *testing.T) {
	observation := tobari.StatusHomeObservation{Present: false, ProjectRoot: "/workspace/fresh"}
	jsonOutput := runStatusHomeFixture(t, observation, "json")
	var document struct {
		SchemaVersion int                        `json:"schema_version"`
		Status        map[string]json.RawMessage `json:"status"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != tobari.StatusHomeSchemaVersion {
		t.Fatalf("schema_version=%d", document.SchemaVersion)
	}
	wantKeys := []string{"attention", "authority_state", "cluster", "context", "login_validity", "next", "permissions", "project_root", "runtime", "services", "siblings", "task", "template", "template_state", "workspace"}
	if got := sortedStatusKeys(document.Status); !reflect.DeepEqual(got, wantKeys) {
		t.Fatalf("status keys=%v want=%v\n%s", got, wantKeys, jsonOutput)
	}
	for name, want := range map[string]string{"authority_state": `"empty"`, "template_state": `"absent"`, "project_root": `"/workspace/fresh"`, "template": "null", "context": "null"} {
		if got := string(document.Status[name]); got != want {
			t.Errorf("status.%s=%s want=%s", name, got, want)
		}
	}
	human := runStatusHomeFixture(t, observation, "text")
	for _, want := range []string{"Tobari · Project Status", "Project        /workspace/fresh", "Template       no default Template", "Current        no Context or Workspace", "Next           " + ProgramName} {
		if !strings.Contains(human, want) {
			t.Errorf("human output missing %q: %q", want, human)
		}
	}
	for _, rejected := range []string{"Manifest", "--manifest", "Auth Broker", "service URL"} {
		if strings.Contains(human, rejected) {
			t.Errorf("human output exposed %q: %q", rejected, human)
		}
	}
}

func TestStatusHomeJSONPreservesIndependentTemplateContextWorkspaceAxes(t *testing.T) {
	snapshot, _, activeTemplate, activeMemory, applied := finalDesiredActiveSnapshotFixture(t, true)
	observation := statusObservationFromSnapshot(t, snapshot)
	observation.Live.Runtime = tobari.StatusRuntimeObservation{Authority: tobari.StatusRuntimeAuthorityReady, Availability: tobari.RuntimeAvailabilityAvailable, Compatibility: tobari.StatusNativeCompatible}
	observation.Live.Workspace = tobari.StatusWorkspaceObservation{State: tobari.StatusWorkspaceRuntimeRunning}
	observation.Live.Attachment = tobari.StatusAttachmentDetached
	service := tobari.ServiceSummary{SchemaVersion: 1, Observation: tobari.ServiceObservationComplete, PendingCount: 0, ActiveCount: 0, UnavailableOwnerCount: 0, Attention: false}
	observation.Live.Services = &service
	out := runStatusHomeFixture(t, observation, "json")
	var document struct {
		Status tobari.StatusHomeSnapshot `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &document); err != nil {
		t.Fatal(err)
	}
	status := document.Status
	if status.Template == nil || status.Template.Revision != snapshot.Template.Current.Revision || status.Context == nil || status.Context.ActiveTemplatePolicy == nil || *status.Context.ActiveTemplatePolicy != activeTemplate || status.Context.ActivePolicyMemory == nil || *status.Context.ActivePolicyMemory != activeMemory {
		t.Fatalf("independent desired/active axes lost: %+v", status)
	}
	if status.Workspace.AppliedEntry == nil || !reflect.DeepEqual(*status.Workspace.AppliedEntry, applied) || status.Workspace.EntryState != tobari.StatusEntryPending || status.Workspace.ObservedRuntimeState != tobari.StatusWorkspaceRuntimeRunning {
		t.Fatalf("applied/live axes lost: %+v", status.Workspace)
	}
	for _, forbidden := range []string{"manifest", "workspace_home", "image\"", "last_used", "service_request_ref", "service_exposure_ref", "snapshot_path", "\"url\"", "\"port\""} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("status leaked retired/private/inferred field %q: %s", forbidden, out)
		}
	}
	human := runStatusHomeFixture(t, observation, "text")
	if !strings.Contains(human, "Current        Workspace-bound Context · Workspace present") || !strings.Contains(human, "Next           "+ProgramName+" —") {
		t.Fatalf("human Current/Next separation is unclear: %q", human)
	}
}

func TestStatusHomeWorkspaceWithoutAppliedEntryKeepsOptionalLiveAxesNotObserved(t *testing.T) {
	snapshot, _, _, _, _ := finalDesiredActiveSnapshotFixture(t, true)
	snapshot.Workspace.LastSuccessfulEntry = nil
	observation := statusObservationFromSnapshot(t, snapshot)
	out := runStatusHomeFixture(t, observation, "json")
	var document struct {
		Status tobari.StatusHomeSnapshot `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &document); err != nil {
		t.Fatal(err)
	}
	if document.Status.Workspace.Presence != "present" || document.Status.Workspace.EntryState != tobari.StatusEntryAbsent || document.Status.Workspace.ObservedRuntimeState != tobari.StatusWorkspaceRuntimeNotObserved || document.Status.Workspace.AttachmentState != tobari.StatusAttachmentNotObserved {
		t.Fatalf("unapplied Workspace axes=%+v", document.Status.Workspace)
	}
}

func runStatusHomeFixture(t *testing.T, observation tobari.StatusHomeObservation, format string) string {
	t.Helper()
	var out, errOut bytes.Buffer
	command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
	command.statusHome = statuscmd.New(statusHomePortFixture{observation: observation})
	args := []string{"status"}
	if format == "json" {
		args = append(args, "--format=json")
	}
	if code := command.RunContext(context.Background(), args); code != ExitOK {
		t.Fatalf("status code=%d stderr=%q", code, errOut.String())
	}
	return out.String()
}

func statusObservationFromSnapshot(t *testing.T, snapshot tobari.ContextAuthoritySnapshot) tobari.StatusHomeObservation {
	t.Helper()
	record := tobari.WorkspaceAuthorityContextRecord{Context: snapshot.Context, PolicyMemory: snapshot.PolicyMemory, ActiveTemplatePolicy: snapshot.ActiveTemplatePolicy, ActivePolicyMemory: snapshot.ActivePolicyMemory, ActivePolicyMemoryRef: snapshot.ActivePolicyMemoryRef}
	workspaces := []tobari.WorkspaceBinding{}
	if snapshot.Workspace != nil {
		workspaces = append(workspaces, *snapshot.Workspace)
	}
	defaultID := snapshot.Template.ID
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection([]tobari.WorkspaceTemplate{snapshot.Template}, []tobari.WorkspaceAuthorityContextRecord{record}, workspaces, []tobari.PolicyCandidateAuthority{}, &defaultID, nil)
	if err != nil {
		t.Fatal(err)
	}
	return tobari.StatusHomeObservation{Collection: collection, Present: true, ProjectRoot: snapshot.Workspace.ProjectRoot}
}

func sortedStatusKeys(values map[string]json.RawMessage) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
