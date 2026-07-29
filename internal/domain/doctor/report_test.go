package doctor

import "testing"

func TestReportValidate(t *testing.T) {
	valid := Report{Checks: []Check{
		{Name: "runtime", Status: CheckStatusPass, Detail: "runtime-version linux/amd64"},
		{Name: "configuration", Status: CheckStatusWarn, Detail: "using defaults"},
	}}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	invalid := []Report{
		{},
		{Checks: []Check{{Name: "runtime"}}},
		{Checks: []Check{{Name: "bad\tname", Status: CheckStatusPass}}},
		{Checks: []Check{{Name: "runtime", Status: CheckStatusPass}, {Name: "runtime", Status: CheckStatusPass}}},
	}
	for index, report := range invalid {
		if err := report.Validate(); err == nil {
			t.Errorf("invalid report %d passed validation", index)
		}
	}
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
