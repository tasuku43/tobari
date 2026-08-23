package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type recommendedFirstUseFixture struct {
	ProjectRoot string   `json:"project_root"`
	Argv        []string `json:"argv"`
}

type recommendedFirstUseAnswer struct {
	WorkspaceManifestName string `json:"manifest_name"`
	SourceAccess          string `json:"source_access"`
	RoutineTraffic        string `json:"routine_traffic"`
	OtherRequests         string `json:"other_requests"`
	PrivateTargets        string `json:"private_targets"`
	RuntimeSelection      string `json:"runtime_selection"`
	HostImport            string `json:"host_import"`
	SessionKind           string `json:"session_kind"`
	Executable            string `json:"executable"`
}

func TestRecommendedFirstUsePresentationUsesOneSemanticFixture(t *testing.T) {
	var fixture recommendedFirstUseFixture
	readJSONTestFile(t, "recommended_first_use_fixture.json", &fixture)
	var answer recommendedFirstUseAnswer
	readJSONTestFile(t, "recommended_first_use_answer.json", &answer)
	session, err := tobari.NewWorkspaceDirectSession(fixture.Argv)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewRecommendedFirstUseDraft(fixture.ProjectRoot, session)
	if err != nil {
		t.Fatal(err)
	}
	gotAnswer := recommendedFirstUseAnswer{
		WorkspaceManifestName: draft.WorkspaceManifestName, SourceAccess: string(draft.Access.SourceAccess),
		RoutineTraffic: string(draft.Access.RoutineTraffic), OtherRequests: string(draft.Access.MethodPolicy.Default),
		PrivateTargets: string(draft.Access.PrivateTargets), RuntimeSelection: draft.RuntimeSelection,
		HostImport: string(draft.HostConfiguration), SessionKind: string(draft.Session.Kind),
		Executable: draft.Session.Executable,
	}
	if gotAnswer != answer {
		t.Fatalf("typed first-use answer = %+v, want %+v", gotAnswer, answer)
	}

	line := &bytes.Buffer{}
	reviewer := &terminalRecommendedFirstUseReviewer{chooser: &terminalContextConfigurationWizard{mode: nil}}
	action, err := reviewer.Review(context.Background(), draft, strings.NewReader("\n"), line)
	if err != nil || action != recommendedFirstUseStart {
		t.Fatalf("line review action/error = %v/%v", action, err)
	}
	wantLine, err := os.ReadFile(filepath.Join("testdata", "recommended_first_use_line.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(line.String(), ": ") {
		t.Fatalf("line first-use prompt lost its input space: %q", line.String())
	}
	visibleLine := strings.TrimSuffix(line.String(), " ")
	if visibleLine != strings.TrimSuffix(string(wantLine), "\n") {
		t.Fatalf("line first-use screen changed\n--- got ---\n%s\n--- want ---\n%s", line.String(), wantLine)
	}

	raw := &bytes.Buffer{}
	if _, err := renderConfigurationWizardRaw(raw, recommendedFirstUseMenu(draft), 0, "", 0, false); err != nil {
		t.Fatal(err)
	}
	visibleRaw := strings.NewReplacer(
		selectorAlternateScreenEnter, "<alternate-enter>", selectorCursorHide, "<cursor-hide>",
		selectorCursorHome, "<cursor-home>", selectorEraseDisplay, "<erase-display>",
		"\x1b[2K\r", "",
	).Replace(raw.String())
	wantRaw, err := os.ReadFile(filepath.Join("testdata", "recommended_first_use_raw.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if visibleRaw != string(wantRaw) {
		t.Fatalf("raw first-use screen changed\n--- got ---\n%s\n--- want ---\n%s", visibleRaw, wantRaw)
	}
}

func readJSONTestFile(t *testing.T, name string, target any) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, target); err != nil {
		t.Fatal(err)
	}
}

func TestRecommendedFirstUseMenuDoesNotInferFromLabels(t *testing.T) {
	draft, err := tobari.NewRecommendedFirstUseDraft("/workspace/example", tobari.NewWorkspaceShellSession())
	if err != nil {
		t.Fatal(err)
	}
	draft.Access.RoutineTraffic = tobari.ManifestRoutineTrafficLimited
	reviewer := &terminalRecommendedFirstUseReviewer{chooser: &terminalContextConfigurationWizard{mode: nil}}
	if _, err := reviewer.Review(context.Background(), draft, strings.NewReader("\n"), &bytes.Buffer{}); err == nil {
		t.Fatal("invalid typed Access unexpectedly reached presentation")
	}
}
