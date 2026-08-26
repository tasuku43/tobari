package cli

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// policyReviewRuntimeFake is shared by CLI tests that need the legacy runtime
// solely for terminal detection or for an unrelated compatibility fixture.
// Final policy reads and mutations use workspaceauthoritycmd ports in their own
// Batch C/D fixtures.
type policyReviewRuntimeFake struct {
	state      tobari.State
	denials    []tobari.PolicyDenial
	rules      []tobari.LearnedPolicyRule
	denyRules  []tobari.PolicyDenyRule
	applyCalls int
	denyCalls  int
	terminal   bool
}

func (f *policyReviewRuntimeFake) CurrentDirectory(context.Context) (string, error) {
	return "/tmp/project", nil
}

func (f *policyReviewRuntimeFake) IsTerminal(io.Writer) bool      { return f.terminal }
func (f *policyReviewRuntimeFake) IsInputTerminal(io.Reader) bool { return f.terminal }

func (f *policyReviewRuntimeFake) ObserveDoctorCheck(_ context.Context, _ string, id doctor.CheckID) (doctor.Observation, error) {
	observation := doctor.Observation{Status: doctor.CheckStatusPass, Detail: "available"}
	if id == doctor.CheckIDDockerEngine {
		observation.Detail = "24.0.0"
		observation.Value = "24.0.0"
	}
	return observation, nil
}

func (f *policyReviewRuntimeFake) ResolveImageSelector(context.Context, string) (string, error) {
	return "test-image", nil
}

func (f *policyReviewRuntimeFake) ValidateClusterBuildIdentity(context.Context) error { return nil }

func (f *policyReviewRuntimeFake) ClusterUp(context.Context) (tobari.State, error) {
	return f.state, nil
}

func (f *policyReviewRuntimeFake) LoadState(context.Context) (tobari.State, bool, error) {
	return f.state, true, nil
}

func (f *policyReviewRuntimeFake) InspectCluster(context.Context, tobari.State) (tobari.ClusterStatus, error) {
	return tobari.ClusterStatus{
		Configured: true, Running: true, PolicyProjection: "valid",
		PrincipalRegistry: "valid", GatewayProjection: "valid",
	}, nil
}

func (f *policyReviewRuntimeFake) ClusterLogs(context.Context, tobari.State, tobari.LogRequest) ([]byte, error) {
	return nil, nil
}

func (f *policyReviewRuntimeFake) ClusterDenials(context.Context, tobari.State, int) (tobari.DenialRead, error) {
	return tobari.DenialRead{Items: append([]tobari.PolicyDenial{}, f.denials...)}, nil
}

func (f *policyReviewRuntimeFake) ReadLearnedPolicyRules(context.Context, tobari.State) ([]tobari.LearnedPolicyRule, error) {
	return append([]tobari.LearnedPolicyRule{}, f.rules...), nil
}

func (f *policyReviewRuntimeFake) ReadPolicyDenyRules(context.Context, tobari.State) (tobari.PolicyDenyRuleSet, error) {
	return tobari.PolicyDenyRuleSet{Exact: append([]tobari.PolicyDenyRule{}, f.denyRules...)}, nil
}

func (f *policyReviewRuntimeFake) ApplyLearnedPolicyRules(
	_ context.Context, state tobari.State, _ []tobari.LearnedPolicyRule, _ []tobari.LearnedPolicyRule,
) (tobari.PolicyActivationReceipt, error) {
	return policyReviewActivationReceipt(state), nil
}

func (f *policyReviewRuntimeFake) ApplyPolicyDenyRules(
	_ context.Context, state tobari.State, _ []tobari.LearnedPolicyRule,
	_ []tobari.PolicyDenyRule, updated []tobari.PolicyDenyRule,
) (tobari.PolicyActivationReceipt, error) {
	f.denyCalls++
	f.denyRules = append([]tobari.PolicyDenyRule{}, updated...)
	return policyReviewActivationReceipt(state), nil
}

func (f *policyReviewRuntimeFake) ClusterDown(context.Context, tobari.State, bool) error { return nil }

type policyReviewRuntimeApplyingFake struct {
	policyReviewRuntimeFake
}

func (f *policyReviewRuntimeApplyingFake) ApplyLearnedPolicyRules(
	_ context.Context, state tobari.State, _ []tobari.LearnedPolicyRule, updated []tobari.LearnedPolicyRule,
) (tobari.PolicyActivationReceipt, error) {
	f.applyCalls++
	f.rules = append([]tobari.LearnedPolicyRule{}, updated...)
	return policyReviewActivationReceipt(state), nil
}

func (f *policyReviewRuntimeApplyingFake) ApplyPolicyDecisionSet(
	_ context.Context, state tobari.State,
	_ []tobari.LearnedPolicyRule, updatedAllows []tobari.LearnedPolicyRule,
	_ []tobari.PolicyDenyRule, updatedDenies []tobari.PolicyDenyRule,
) (tobari.PolicyActivationReceipt, error) {
	f.applyCalls++
	f.rules = append([]tobari.LearnedPolicyRule{}, updatedAllows...)
	f.denyRules = append([]tobari.PolicyDenyRule{}, updatedDenies...)
	f.denyCalls += len(updatedDenies)
	return policyReviewActivationReceipt(state), nil
}

func policyReviewActivationReceipt(state tobari.State) tobari.PolicyActivationReceipt {
	return tobari.PolicyActivationReceipt{
		ActiveRevision:     strings.Repeat("b", 64),
		EvaluatorIdentity:  testCLIProjectionIdentity(strings.Repeat("b", 64)).EvaluatorIdentity,
		PolicyDataIdentity: testCLIProjectionIdentity(strings.Repeat("b", 64)).PolicyDataIdentity,
	}
}

func testCLIProjectionIdentity(revision string) tobari.PolicyProjectionIdentity {
	return tobari.PolicyProjectionIdentity{
		AggregateRevision:  revision,
		EvaluatorIdentity:  tobari.PolicyEvaluatorIdentity{SchemaVersion: 1, Version: "tobari-evaluator-v1", Digest: tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))},
		PolicyDataIdentity: tobari.PolicyDataIdentity{SchemaVersion: 1, Digest: tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64))},
	}
}

func validClusterComponentStatuses() []tobari.ComponentStatus {
	components := []tobari.ComponentStatus{
		{Name: "gateway", State: "running", Health: "healthy"},
		{Name: "opa", State: "running", Health: "healthy"},
	}
	if buildIdentityHasBroker() {
		components = append([]tobari.ComponentStatus{{Name: "auth-broker", State: "running", Health: "healthy"}}, components...)
	}
	return components
}

func TestDoctorDefaultsRootToCurrentDirectory(t *testing.T) {
	runtime := &policyReviewRuntimeFake{}
	inspector := passingInspector("unused")
	command, stdout, stderr := newTestCLI(inspector)
	command.tobari = tobaricmd.New(runtime)
	if code := runCLI(command, []string{"doctor"}); code != ExitOK {
		t.Fatalf("Run(doctor) code = %d, stderr = %q", code, stderr.String())
	}
	if len(inspector.roots) != len(doctor.CheckInventory()) || inspector.roots[0] != "." {
		t.Fatalf("doctor roots = %q, want current directory default for every check", inspector.roots)
	}
	if !strings.Contains(stdout.String(), "docker_cli     pass") {
		t.Fatalf("doctor output = %q", stdout.String())
	}
}

func TestDoctorHonorsExplicitRoot(t *testing.T) {
	runtime := &policyReviewRuntimeFake{}
	inspector := passingInspector("unused")
	command, _, stderr := newTestCLI(inspector)
	command.tobari = tobaricmd.New(runtime)
	if code := runCLI(command, []string{"doctor", "--root", "/tmp/project"}); code != ExitOK {
		t.Fatalf("Run(doctor --root /tmp/project) code = %d, stderr = %q", code, stderr.String())
	}
	if len(inspector.roots) != len(doctor.CheckInventory()) || inspector.roots[0] != "/tmp/project" {
		t.Fatalf("doctor roots = %q, want explicit root for every check", inspector.roots)
	}
}

func TestDoctorRejectsExplicitEmptyRootBeforeInspection(t *testing.T) {
	inspector := passingInspector("unused")
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor", "--root="}); code != ExitUsage {
		t.Fatalf("Run(doctor --root=) code = %d, stderr = %q", code, stderr.String())
	}
	if inspector.calls != 0 || stdout.Len() != 0 || !humanOutputHasRow(stderr.String(), "Code", "invalid_arguments") {
		t.Fatalf("calls = %d, stdout = %q, stderr = %q", inspector.calls, stdout.String(), stderr.String())
	}
}
