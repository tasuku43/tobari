package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalDefaultPairStatusFixture struct {
	observation tobari.FinalDefaultPairObservation
}

func (f finalDefaultPairStatusFixture) ObserveFinalCanonicalProjectRoot(context.Context) (string, error) {
	return f.observation.ProjectRoot, nil
}

func (f finalDefaultPairStatusFixture) ObserveFinalDefaultPair(context.Context, string) (tobari.FinalDefaultPairObservation, error) {
	return f.observation.Clone(), nil
}

func (finalDefaultPairStatusFixture) InitializeFinalDefaultPair(context.Context, string, tobari.WorkspaceTemplateBody) (tobari.FinalDefaultPairPublication, error) {
	panic("status must not initialize final authority")
}

func TestBareStatusJSONExecutesSchemaThreeDesiredActiveAppliedContract(t *testing.T) {
	fresh, err := tobari.NewFinalDefaultPairObservation(tobari.WorkspaceAuthorityCollection{}, false, "/workspace/fresh")
	if err != nil {
		t.Fatal(err)
	}
	inactiveSnapshot, _, _, _, _ := finalDesiredActiveSnapshotFixture(t, false)
	inactive := finalDefaultPairObservationFixture(t, inactiveSnapshot)
	activeSnapshot, _, activeTemplate, activeMemory, applied := finalDesiredActiveSnapshotFixture(t, true)
	active := finalDefaultPairObservationFixture(t, activeSnapshot)

	for _, test := range []struct {
		name        string
		observation tobari.FinalDefaultPairObservation
		wantKeys    []string
		wantNull    []string
		wantValues  map[string]any
	}{
		{
			name: "fresh final empty", observation: fresh,
			wantKeys:   finalDefaultPairStatusKeys(false),
			wantNull:   []string{"active_policy_memory_revision", "active_template_policy_slice_digest", "applied_entry", "context_id", "current_policy_memory_revision", "desired_template_generation", "desired_template_policy_slice_digest", "desired_template_revision", "template_name", "workspace_home", "workspace_id", "workspace_template_id"},
			wantValues: map[string]any{"authority_state": "empty", "default_template_state": "absent", "project_root": "/workspace/fresh"},
		},
		{
			name: "selected inactive", observation: inactive,
			wantKeys: finalDefaultPairStatusKeys(false),
			wantNull: []string{"active_policy_memory_revision", "active_template_policy_slice_digest", "applied_entry", "workspace_home", "workspace_id"},
			wantValues: map[string]any{
				"authority_state": "initialized", "default_template_state": "selected", "project_root": inactiveSnapshot.Context.ProjectRoot,
				"workspace_template_id": inactiveSnapshot.Template.ID, "template_name": inactiveSnapshot.Template.Name,
				"desired_template_generation": inactiveSnapshot.Template.Current.Generation, "desired_template_revision": inactiveSnapshot.Template.Current.Revision,
				"desired_template_policy_slice_digest": inactiveSnapshot.Template.Current.Slices.PolicySliceDigest,
				"context_id":                           inactiveSnapshot.Context.ID, "current_policy_memory_revision": inactiveSnapshot.PolicyMemory.Revision,
			},
		},
		{
			name: "A active B desired", observation: active,
			wantKeys: finalDefaultPairStatusKeys(true), wantNull: []string{},
			wantValues: map[string]any{
				"authority_state": "initialized", "default_template_state": "selected", "project_root": activeSnapshot.Context.ProjectRoot,
				"workspace_template_id": activeSnapshot.Template.ID, "template_name": activeSnapshot.Template.Name,
				"desired_template_generation": activeSnapshot.Template.Current.Generation, "desired_template_revision": activeSnapshot.Template.Current.Revision,
				"desired_template_policy_slice_digest": activeSnapshot.Template.Current.Slices.PolicySliceDigest,
				"active_template_policy_slice_digest":  activeTemplate, "context_id": activeSnapshot.Context.ID,
				"current_policy_memory_revision": activeSnapshot.PolicyMemory.Revision, "active_policy_memory_revision": activeMemory,
				"workspace_id": activeSnapshot.Workspace.ID, "workspace_ref": mustFinalWorkspaceRef(t, activeSnapshot.Workspace.ID),
				"workspace_home": activeSnapshot.Workspace.Home, "applied_entry": applied,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := runBareStatusFixture(t, test.observation, "json")
			var document struct {
				SchemaVersion int                        `json:"schema_version"`
				Status        map[string]json.RawMessage `json:"status"`
			}
			if err := json.Unmarshal([]byte(out), &document); err != nil {
				t.Fatal(err)
			}
			if document.SchemaVersion != 3 {
				t.Fatalf("schema_version=%d, want 3", document.SchemaVersion)
			}
			if got := sortedDefaultPairJSONKeys(document.Status); !reflect.DeepEqual(got, test.wantKeys) {
				t.Fatalf("status keys=%v, want %v; output=%s", got, test.wantKeys, out)
			}
			for _, name := range test.wantNull {
				if got := document.Status[name]; string(got) != "null" {
					t.Errorf("status.%s=%s, want null", name, got)
				}
			}
			for name, want := range test.wantValues {
				assertJSONFieldEqual(t, document.Status, name, want)
			}
		})
	}
}

func TestBareStatusHumanExecutesFreshInactiveAndPendingAxisSemantics(t *testing.T) {
	fresh, err := tobari.NewFinalDefaultPairObservation(tobari.WorkspaceAuthorityCollection{}, false, "/workspace/fresh")
	if err != nil {
		t.Fatal(err)
	}
	inactiveSnapshot, _, _, _, _ := finalDesiredActiveSnapshotFixture(t, false)
	activeSnapshot, _, activeTemplate, activeMemory, applied := finalDesiredActiveSnapshotFixture(t, true)

	for _, test := range []struct {
		name        string
		observation tobari.FinalDefaultPairObservation
		want        []string
		reject      []string
	}{
		{
			name: "fresh final empty", observation: fresh,
			want:   []string{"Final authority empty", "Project /workspace/fresh", "Default Template absent", "Active Template policy absent", "Active Policy Memory absent", "Applied entry absent"},
			reject: []string{"Desired Template revision", "Current Policy Memory"},
		},
		{
			name: "selected inactive", observation: finalDefaultPairObservationFixture(t, inactiveSnapshot),
			want: []string{
				"Final authority initialized", "Default Template selected", "Context " + string(inactiveSnapshot.Context.ID),
				"Desired Template generation " + strconv.FormatUint(inactiveSnapshot.Template.Current.Generation, 10),
				"Desired Template revision " + string(inactiveSnapshot.Template.Current.Revision),
				"Desired Template policy " + string(inactiveSnapshot.Template.Current.Slices.PolicySliceDigest),
				"Active Template policy absent", "Current Policy Memory " + string(inactiveSnapshot.PolicyMemory.Revision),
				"Active Policy Memory absent", "Applied entry absent",
			},
		},
		{
			name: "A active B desired", observation: finalDefaultPairObservationFixture(t, activeSnapshot),
			want: []string{
				"Desired Template generation " + strconv.FormatUint(activeSnapshot.Template.Current.Generation, 10),
				"Desired Template revision " + string(activeSnapshot.Template.Current.Revision),
				"Active Template policy " + string(activeTemplate), "Current Policy Memory " + string(activeSnapshot.PolicyMemory.Revision),
				"Active Policy Memory " + string(activeMemory), "Applied entry " + string(applied.TemplateRevision) + " / " + string(applied.EntrySliceDigest),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := runBareStatusFixture(t, test.observation, "text")
			for _, want := range test.want {
				if !strings.Contains(out, want) {
					t.Errorf("human status missing %q: %q", want, out)
				}
			}
			for _, rejected := range test.reject {
				if strings.Contains(out, rejected) {
					t.Errorf("human status inferred %q from absent authority: %q", rejected, out)
				}
			}
		})
	}
}

func runBareStatusFixture(t *testing.T, observation tobari.FinalDefaultPairObservation, format string) string {
	t.Helper()
	fixture := finalDefaultPairStatusFixture{observation: observation.Clone()}
	var out, errOut bytes.Buffer
	command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
	command.finalDefaultPair = workspaceauthoritycmd.NewDefaultPairService(fixture, fixture, workspaceauthoritycmd.NewContextService(nil))
	args := []string{"status"}
	if format == "json" {
		args = append(args, "--format=json")
	}
	if code := command.RunContext(context.Background(), args); code != ExitOK {
		t.Fatalf("status code=%d stderr=%q", code, errOut.String())
	}
	return out.String()
}

func finalDefaultPairObservationFixture(t *testing.T, snapshot tobari.ContextAuthoritySnapshot) tobari.FinalDefaultPairObservation {
	t.Helper()
	template := snapshot.Template.Clone()
	value := snapshot.Clone()
	result := tobari.FinalDefaultPairObservation{
		SchemaVersion: tobari.FinalDefaultPairObservationSchemaVersion, CollectionPresent: true,
		CollectionGeneration: 9, CollectionRevision: tobari.SemanticDigest("sha256:" + strings.Repeat("9", 64)),
		ProjectRoot: snapshot.Context.ProjectRoot, DefaultTemplate: &template, Context: &value,
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	return result
}

func finalDefaultPairStatusKeys(withWorkspaceRef bool) []string {
	result := []string{
		"active_policy_memory_revision", "active_template_policy_slice_digest", "applied_entry", "authority_state", "context_id",
		"current_policy_memory_revision", "default_template_state", "desired_template_generation", "desired_template_policy_slice_digest",
		"desired_template_revision", "project_root", "template_name", "workspace_home", "workspace_id", "workspace_template_id",
	}
	if withWorkspaceRef {
		result = append(result, "workspace_ref")
	}
	sort.Strings(result)
	return result
}

func sortedDefaultPairJSONKeys(values map[string]json.RawMessage) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func mustFinalWorkspaceRef(t *testing.T, id tobari.WorkspaceID) string {
	t.Helper()
	result, err := tobari.WorkspaceRef(id)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
