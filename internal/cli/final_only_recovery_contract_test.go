package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type legacyTemplateReadPort struct{}

func (legacyTemplateReadPort) ListWorkspaceTemplates(context.Context) ([]tobari.WorkspaceTemplate, error) {
	return nil, fmt.Errorf("%w: synthetic predecessor root", tobari.ErrPreReleaseLegacyAuthority)
}

func (legacyTemplateReadPort) DiscoverWorkspaceTemplate(context.Context, string) (tobari.WorkspaceTemplate, error) {
	return tobari.WorkspaceTemplate{}, fmt.Errorf("%w: synthetic predecessor root", tobari.ErrPreReleaseLegacyAuthority)
}

func TestFinalOnlyLegacyGuidanceNamesOnlyCatalogedReadRecovery(t *testing.T) {
	catalog := DefaultCatalog()
	if _, found := catalog.Lookup("migrate apply"); found {
		t.Fatal("retired migrate apply command remains in the public Catalog")
	}

	inspector := passingInspector("observed")
	inspector.observations = map[doctor.CheckID]doctor.Observation{
		doctor.CheckIDContext: {
			Status: doctor.CheckStatusFail,
			Detail: "unsupported pre-release authority is present",
			Cause:  doctor.ObservationCauseLegacyStatePresent,
		},
	}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor", "--format=json"}); code != ExitRejected {
		t.Fatalf("doctor code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var document doctorJSONDocument
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("decode doctor result: %v", err)
	}
	var recovery *doctorJSONRecovery
	for _, check := range document.Report {
		if check.Check == string(doctor.CheckIDContext) {
			recovery = check.Recovery
			break
		}
	}
	if recovery == nil || recovery.NextCommand != "help" ||
		!strings.Contains(recovery.Action, "legacy_state_present") ||
		!strings.Contains(recovery.Action, "reset or recreate") {
		t.Fatalf("doctor legacy recovery = %+v", recovery)
	}
	if _, found := catalog.Lookup(recovery.NextCommand); !found {
		t.Fatalf("doctor recovery %q is not cataloged", recovery.NextCommand)
	}
	if strings.Contains(stdout.String(), "migrate apply") {
		t.Fatalf("doctor result advertises retired migration: %q", stdout.String())
	}

	_, err := workspaceauthoritycmd.NewTemplateService(legacyTemplateReadPort{}).List(context.Background())
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "legacy_state_present" || len(public.NextActions) != 1 || public.NextActions[0].Command != "help" {
		t.Fatalf("legacy PublicCopy = %+v, ok=%t", public, ok)
	}
	if _, found := catalog.Lookup(public.NextActions[0].Command); !found {
		t.Fatalf("legacy PublicCopy recovery %q is not cataloged", public.NextActions[0].Command)
	}
	if strings.Contains(public.NextActions[0].Command, "migrate") || strings.Contains(public.NextActions[0].Reason, "migrate apply") {
		t.Fatalf("legacy PublicCopy advertises retired migration: %+v", public.NextActions[0])
	}
}
