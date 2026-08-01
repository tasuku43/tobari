package tobari

import "testing"

func TestClusterUpProgressValidatesBoundedVocabulary(t *testing.T) {
	t.Parallel()
	for _, status := range []ClusterUpProgressStatus{
		ClusterUpProgressStarted,
		ClusterUpProgressUpdated,
		ClusterUpProgressCompleted,
		ClusterUpProgressFailed,
	} {
		step := ClusterUpProgressPrepare
		if status == ClusterUpProgressUpdated {
			step = ClusterUpProgressWaitForHealth
		}
		if err := (ClusterUpProgress{Step: step, Status: status}).Validate(); err != nil {
			t.Errorf("valid progress (%q, %q) rejected: %v", step, status, err)
		}
	}
	if err := (ClusterUpProgress{Step: "unknown", Status: ClusterUpProgressStarted}).Validate(); err == nil {
		t.Fatal("unknown progress step was accepted")
	}
	if err := (ClusterUpProgress{Step: ClusterUpProgressPrepare, Status: "unknown"}).Validate(); err == nil {
		t.Fatal("unknown progress status was accepted")
	}
	if err := (ClusterUpProgress{Step: ClusterUpProgressPolicy, Status: ClusterUpProgressUpdated}).Validate(); err == nil {
		t.Fatal("policy update was accepted outside health polling")
	}
}
