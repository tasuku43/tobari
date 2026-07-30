package tobari

import "testing"

func validPolicyDenial() PolicyDenial {
	return PolicyDenial{
		Timestamp: "2026-07-30T10:41:11Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		Host: "api.github.com", Method: "GET", Path: "/repos/cli/cli",
		Reason: "request did not match an allow rule", StatusCode: 403,
	}
}

func TestDenialReportPreservesEmptyBoundedScope(t *testing.T) {
	t.Parallel()
	report := DenialReport{
		Task: TaskClusterDenials, PolicyDirectory: "/config/tobari/policy",
		WindowLines: 200, Items: []PolicyDenial{},
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	report.Items = nil
	if err := report.Validate(); err == nil {
		t.Fatal("unknown denial collection was accepted")
	}
}

func TestPolicyDenialRejectsInterpretationSensitiveFields(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*PolicyDenial){
		"timestamp":  func(value *PolicyDenial) { value.Timestamp = "recently" },
		"request id": func(value *PolicyDenial) { value.RequestID = "GET-api.github.com" },
		"host":       func(value *PolicyDenial) { value.Host = "api.github.com\nallow=true" },
		"method":     func(value *PolicyDenial) { value.Method = "GET POST" },
		"path":       func(value *PolicyDenial) { value.Path = "repos/cli/cli" },
		"status":     func(value *PolicyDenial) { value.StatusCode = 200 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := validPolicyDenial()
			mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("invalid denial was accepted")
			}
		})
	}
}

func TestPolicyActivationRequiresConfirmedTaskResult(t *testing.T) {
	t.Parallel()
	valid := PolicyActivation{
		Task: TaskPolicyApply, PolicyDirectory: "/config/tobari/policy", Applied: true,
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.Applied = false
	if err := valid.Validate(); err == nil {
		t.Fatal("unconfirmed activation was accepted")
	}
}
