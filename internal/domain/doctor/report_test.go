package doctor

import (
	"reflect"
	"testing"
)

func TestWorkspaceStartReadinessProfileIsClosedAndDefensive(t *testing.T) {
	checks, err := ReadinessChecks(ReadinessProfileWorkspaceStart)
	if err != nil {
		t.Fatal(err)
	}
	want := []CheckID{CheckIDDockerCLI, CheckIDDockerEngine, CheckIDDockerContext, CheckIDDockerCompose}
	if !reflect.DeepEqual(checks, want) {
		t.Fatalf("readiness checks = %v, want %v", checks, want)
	}
	checks[0] = CheckIDRoot
	again, _ := ReadinessChecks(ReadinessProfileWorkspaceStart)
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("readiness profile was mutated: %v", again)
	}
	if _, err := ReadinessChecks("provider_backend"); err == nil {
		t.Fatal("unknown readiness profile passed validation")
	}
}

func TestCheckInventoryIsFiniteTopologicalDAG(t *testing.T) {
	want := expectedCheckInventory()
	got := CheckInventory()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CheckInventory() = %#v, want %#v", got, want)
	}
	if err := ValidateInventory(got); err != nil {
		t.Fatalf("ValidateInventory() error = %v", err)
	}
}

func TestObservationCauseIsClosedAndFailureOnly(t *testing.T) {
	if err := (Observation{Status: CheckStatusFail, Cause: ObservationCauseLegacyStatePresent}).Validate(); err != nil {
		t.Fatalf("migration-required observation rejected: %v", err)
	}
	for _, observation := range []Observation{
		{Status: CheckStatusPass, Cause: ObservationCauseLegacyStatePresent},
		{Status: CheckStatusFail, Cause: ObservationCause("future")},
	} {
		if err := observation.Validate(); err == nil {
			t.Fatalf("invalid observation accepted: %+v", observation)
		}
	}
}

func TestReportValidateRequiresCompleteInventoryAndTypedDependencies(t *testing.T) {
	valid := completePassReport()
	if err := valid.Validate(); err != nil {
		t.Fatalf("complete report rejected: %v", err)
	}

	tests := map[string]Report{
		"missing": func() Report {
			result := completePassReport()
			result.Checks = result.Checks[:len(result.Checks)-1]
			return result
		}(),
		"reordered": func() Report {
			result := completePassReport()
			result.Checks[0], result.Checks[1] = result.Checks[1], result.Checks[0]
			return result
		}(),
		"fail without recovery": func() Report {
			result := completePassReport()
			result.Checks[0].Status = CheckStatusFail
			return result
		}(),
		"blocked without blocker": func() Report {
			result := completePassReport()
			result.Checks[1].Status = CheckStatusBlocked
			return result
		}(),
		"blocked by non-prerequisite": func() Report {
			result := completePassReport()
			blockedBy := CheckIDRoot
			result.Checks[1].Status = CheckStatusBlocked
			result.Checks[1].BlockedBy = &blockedBy
			return result
		}(),
		"blocked with recovery": func() Report {
			result := completePassReport()
			blockedBy := CheckIDDockerCLI
			result.Checks[1].Status = CheckStatusBlocked
			result.Checks[1].BlockedBy = &blockedBy
			result.Checks[1].Recovery = &Recovery{Action: "must not duplicate root recovery", NextCommand: "doctor"}
			return result
		}(),
		"blocked by passing direct prerequisite": func() Report {
			result := completePassReport()
			blockedBy := CheckIDDockerCLI
			result.Checks[1].Status = CheckStatusBlocked
			result.Checks[1].BlockedBy = &blockedBy
			return result
		}(),
		"pass with blocker": func() Report {
			result := completePassReport()
			blockedBy := CheckIDDockerCLI
			result.Checks[1].BlockedBy = &blockedBy
			return result
		}(),
		"pass with recovery": func() Report {
			result := completePassReport()
			result.Checks[0].Recovery = &Recovery{Action: "not applicable", NextCommand: "doctor"}
			return result
		}(),
	}
	for name, report := range tests {
		t.Run(name, func(t *testing.T) {
			if err := report.Validate(); err == nil {
				t.Fatal("invalid report passed validation")
			}
		})
	}
}

func completePassReport() Report {
	inventory := CheckInventory()
	checks := make([]Check, 0, len(inventory))
	for _, spec := range inventory {
		checks = append(checks, Check{Name: spec.ID, Status: CheckStatusPass, Detail: "observed"})
	}
	return Report{Checks: checks}
}

func TestReportValidate(t *testing.T) {
	valid := completePassReport()
	valid.Checks[CheckIDIndex(CheckIDRootSharing)].Status = CheckStatusWarn
	valid.Checks[CheckIDIndex(CheckIDRootSharing)].Detail = "using bounded observation"
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	invalid := []Report{{}, {Checks: []Check{{Name: CheckIDDockerCLI}}}}
	for index, report := range invalid {
		if err := report.Validate(); err == nil {
			t.Errorf("invalid report %d passed validation", index)
		}
	}
}

func CheckIDIndex(id CheckID) int {
	for index, spec := range CheckInventory() {
		if spec.ID == id {
			return index
		}
	}
	return -1
}

func TestReportHealthy(t *testing.T) {
	if !((Report{Checks: []Check{{Name: "runtime", Status: CheckStatusPass}}}).Healthy()) {
		t.Fatal("pass report is not healthy")
	}
	if !((Report{Checks: []Check{{Name: "configuration", Status: CheckStatusWarn}}}).Healthy()) {
		t.Fatal("warning report is not healthy")
	}
	if (Report{Checks: []Check{{Name: "runtime", Status: CheckStatusFail}}}).Healthy() {
		t.Fatal("failed report is healthy")
	}
}

func TestPrimaryRecoveryPrioritizesFailureThenActionableWarning(t *testing.T) {
	warning := Recovery{Action: "reconcile cluster", NextCommand: "cluster up"}
	failure := Recovery{Action: "install Docker", NextCommand: "doctor"}
	report := Report{Checks: []Check{
		{Name: CheckIDState, Status: CheckStatusWarn, Recovery: &warning},
		{Name: CheckIDDockerCLI, Status: CheckStatusFail, Recovery: &failure},
	}}
	if got, exists := report.PrimaryRecovery(); !exists || got != failure {
		t.Fatalf("PrimaryRecovery() = (%+v, %t), want failure", got, exists)
	}
	report.Checks[1] = Check{Name: CheckIDDockerCLI, Status: CheckStatusPass}
	if got, exists := report.PrimaryRecovery(); !exists || got != warning {
		t.Fatalf("PrimaryRecovery() = (%+v, %t), want actionable warning", got, exists)
	}
}
