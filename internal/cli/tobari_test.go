package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type policyReviewRuntimeFake struct {
	state         tobari.State
	denials       []tobari.PolicyDenial
	rules         []tobari.LearnedPolicyRule
	denyRules     []tobari.PolicyDenyRule
	applyCalls    int
	denyCalls     int
	terminal      bool
	denialReads   int
	denialsByRead [][]tobari.PolicyDenial
}

func (f *policyReviewRuntimeFake) ResolveRoot(_ context.Context, root string) (string, error) {
	return filepath.Clean(root), nil
}
func (f *policyReviewRuntimeFake) CurrentDirectory(context.Context) (string, error) {
	return "/tmp/project", nil
}
func (f *policyReviewRuntimeFake) IsTerminal(io.Writer) bool      { return f.terminal }
func (f *policyReviewRuntimeFake) IsInputTerminal(io.Reader) bool { return f.terminal }
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
	return tobari.ClusterStatus{Configured: true, Running: true}, nil
}
func (f *policyReviewRuntimeFake) ClusterLogs(context.Context, tobari.State, tobari.LogRequest) ([]byte, error) {
	return nil, nil
}
func (f *policyReviewRuntimeFake) ClusterDenials(context.Context, tobari.State, int) ([]tobari.PolicyDenial, error) {
	if len(f.denialsByRead) > 0 {
		index := f.denialReads
		if index >= len(f.denialsByRead) {
			index = len(f.denialsByRead) - 1
		}
		f.denialReads++
		return append([]tobari.PolicyDenial{}, f.denialsByRead[index]...), nil
	}
	return append([]tobari.PolicyDenial{}, f.denials...), nil
}
func (f *policyReviewRuntimeFake) ReadLearnedPolicyRules(context.Context, tobari.State) ([]tobari.LearnedPolicyRule, error) {
	return append([]tobari.LearnedPolicyRule{}, f.rules...), nil
}
func (f *policyReviewRuntimeFake) ReadPolicyDenyRules(context.Context, tobari.State) (tobari.PolicyDenyRuleSet, error) {
	return tobari.PolicyDenyRuleSet{
		Baseline: []tobari.PolicyBaselineDenyRule{}, Exact: append([]tobari.PolicyDenyRule{}, f.denyRules...),
	}, nil
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
func TestDoctorDefaultsRootToCurrentDirectory(t *testing.T) {
	runtime := &policyReviewRuntimeFake{}
	inspector := passingInspector("unused")
	command, stdout, stderr := newTestCLI(inspector)
	command.tobari = tobaricmd.New(runtime)
	if code := runCLI(command, []string{"doctor"}); code != ExitOK {
		t.Fatalf("Run(doctor) code = %d, stderr = %q", code, stderr.String())
	}
	if len(inspector.roots) != len(doctorCheckIDValues()) || inspector.roots[0] != "." {
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
	if len(inspector.roots) != len(doctorCheckIDValues()) || inspector.roots[0] != "/tmp/project" {
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

// applyPolicyReviewRuntimeFake is kept separate from the port method above so
// the test can assert the exact learned rule was the selected candidate.
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
	policyDirectory := state.PolicyDirectory
	if policyDirectory == "" {
		policyDirectory = "/tmp/policy"
	}
	return tobari.PolicyActivationReceipt{
		PolicyDirectory: policyDirectory,
		ActiveRevision:  strings.Repeat("b", 64),
	}
}
func TestDefaultCatalogPublishesCWDOwnedLifecycleWithoutActionIDs(t *testing.T) {
	t.Parallel()
	catalog := DefaultCatalog()
	for _, path := range []string{"attach", "shell", "exec", "logs", "detach"} {
		if _, found := catalog.Lookup(path); found {
			t.Fatalf("retired command %q is still public", path)
		}
	}
	for _, path := range []string{"tobari", "delete"} {
		command, found := catalog.Lookup(path)
		if !found || command.Role != RoleAct || command.Agent.FixedTarget == nil ||
			command.Agent.FixedTarget.Kind != tobari.CurrentDirectoryTargetKind ||
			len(command.ConsumedRefs()) != 0 {
			t.Fatalf("%s fixed target = %+v", path, command.Agent.FixedTarget)
		}
	}

	for _, path := range []string{"policy candidates", "policy review", "policy tail"} {
		command, found := catalog.Lookup(path)
		want := []ProducedRef{{Kind: tobari.PolicyCandidateKind, Field: "id"}}
		if !found || command.Role != RoleDiscover || !reflect.DeepEqual(command.ProducedRefs(), want) {
			t.Fatalf("%s reference contract = %+v", path, command.ProducedRefs())
		}
	}
	rules, found := catalog.Lookup("policy rules")
	if !found || rules.Role != RoleDiscover ||
		!reflect.DeepEqual(rules.ProducedRefs(), []ProducedRef{{Kind: tobari.PolicyRuleKind, Field: "id"}}) ||
		rules.Agent.Interactive == nil || rules.Agent.Interactive.ActionCommand != "policy reset" ||
		rules.Agent.Interactive.SelectionReferenceKind != tobari.PolicyRuleKind ||
		rules.Agent.Interactive.NonInteractiveBehavior != "read_only" {
		t.Fatalf("policy rules reference contract = %+v", rules)
	}
	reset, found := catalog.Lookup("policy reset")
	if !found || reset.Role != RoleAct ||
		!reflect.DeepEqual(reset.ConsumedRefs(), []ConsumedRef{{Kind: tobari.PolicyRuleKind, Argument: "--id"}}) ||
		reset.Agent.Mutation == nil || reset.Agent.Mutation.TargetKind != tobari.PolicyRuleKind ||
		reset.Agent.Mutation.TargetIDInput != "--id" {
		t.Fatalf("policy reset reference contract = %+v", reset)
	}
	review, found := catalog.Lookup("policy review")
	if !found || review.Agent.Interactive != nil {
		t.Fatalf("policy review interactive workflow = %+v", review.Agent.Interactive)
	}
	internalReview, found := catalog.lookupRegistered("policy review")
	if !found || internalReview.Agent.Interactive == nil ||
		internalReview.Agent.Interactive.ActionCommand != "policy apply-reviewed" ||
		internalReview.Agent.Interactive.SelectionReferenceKind != tobari.PolicyCandidateKind ||
		internalReview.Agent.Interactive.SelectionOutputField != "id" ||
		internalReview.Agent.Interactive.Confirmation != "explicit_yes" ||
		internalReview.Agent.Interactive.NonInteractiveBehavior != "read_only" {
		t.Fatalf("registered policy review workflow = %+v", internalReview.Agent.Interactive)
	}
	allow, found := catalog.Lookup("policy allow")
	if !found || allow.Role != RoleAct ||
		!reflect.DeepEqual(allow.ConsumedRefs(), []ConsumedRef{{
			Kind: tobari.PolicyCandidateKind, Argument: "--id",
		}}) ||
		allow.Agent.Mutation == nil ||
		allow.Agent.Mutation.TargetKind != tobari.PolicyCandidateKind ||
		allow.Agent.Mutation.TargetIDInput != "--id" {
		t.Fatalf("policy allow reference contract = %+v", allow)
	}
	compactions, found := catalog.Lookup("policy compactions")
	if !found || compactions.Role != RoleDiscover ||
		!reflect.DeepEqual(compactions.ProducedRefs(), []ProducedRef{{
			Kind: tobari.PolicyCompactionKind, Field: "id",
		}}) {
		t.Fatalf("policy compactions reference contract = %+v", compactions.ProducedRefs())
	}
	compact, found := catalog.Lookup("policy compact")
	if !found || compact.Role != RoleAct ||
		!reflect.DeepEqual(compact.ConsumedRefs(), []ConsumedRef{{
			Kind: tobari.PolicyCompactionKind, Argument: "--id",
		}}) ||
		compact.Agent.Mutation == nil ||
		compact.Agent.Mutation.TargetKind != tobari.PolicyCompactionKind ||
		compact.Agent.Mutation.TargetIDInput != "--id" {
		t.Fatalf("policy compact reference contract = %+v", compact)
	}
}

func TestPolicyCatalogPublishesGraphQLIdentityContracts(t *testing.T) {
	t.Parallel()
	wantVersions := map[string]int{
		"cluster denials":   4,
		"policy candidates": 5,
		"policy review":     5,
		"policy rules":      3,
	}
	for path, version := range wantVersions {
		spec, found := DefaultCatalog().Lookup(path)
		if !found {
			t.Fatalf("catalog lacks %q", path)
		}
		if spec.Agent.Output.JSONSchemaVersion != version {
			t.Fatalf("%s schema version = %d, want %d", path, spec.Agent.Output.JSONSchemaVersion, version)
		}
		if !strings.Contains(strings.ToLower(spec.Agent.Outcome), "graphql") {
			t.Fatalf("%s outcome does not declare GraphQL identity: %q", path, spec.Agent.Outcome)
		}
	}

	for _, path := range []string{"policy candidates", "policy review", "policy rules", "policy allow", "policy deny"} {
		spec, found := DefaultCatalog().Lookup(path)
		if !found {
			t.Fatalf("catalog lacks %q", path)
		}
		fields := make(map[string]bool, len(spec.Agent.Output.Fields))
		for _, field := range spec.Agent.Output.Fields {
			fields[field.Name] = true
		}
		for _, name := range []string{"protocol", "graphql_operation_type", "graphql_root_field"} {
			if !fields[name] {
				t.Fatalf("%s output does not declare %q: %+v", path, name, spec.Agent.Output.Fields)
			}
		}
	}
}

func TestDefaultCatalogDoesNotPublishDevContainerRuntimeSelection(t *testing.T) {
	t.Parallel()
	catalog := DefaultCatalog()
	if _, found := catalog.Lookup("attach"); found {
		t.Fatal("retired named attach command is still public")
	}
	for _, command := range catalog.Commands() {
		if strings.Contains(strings.ToLower(command.Usage()), "devcontainer") {
			t.Fatalf("public command %q still exposes Dev Container input: %q", command.Path, command.Usage())
		}
	}
}

func TestPolicyReviewTTYStagesExactAllowAndAppliesOnce(t *testing.T) {
	t.Parallel()
	denial := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project", Host: "api.example.com", Port: 443,
		Method: "POST", Path: "/repos/example/issues", Reason: "request did not match an allow rule",
		StatusCode: 403, Learnable: true,
	}
	candidate, err := tobari.NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &policyReviewRuntimeApplyingFake{
		policyReviewRuntimeFake: policyReviewRuntimeFake{
			state:    tobari.State{PolicyDirectory: "/tmp/policy"},
			denials:  []tobari.PolicyDenial{denial},
			terminal: true,
		},
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("1\na\np\ny\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.tobari = tobaricmd.New(runtime)
	if code := command.RunContext(context.Background(), []string{"policy", "review"}); code != ExitOK {
		t.Fatalf("policy review code = %d, stderr = %q", code, stderr.String())
	}
	if runtime.applyCalls != 1 || len(runtime.rules) != 1 ||
		len(runtime.rules[0].SourceCandidates) != 1 || runtime.rules[0].SourceCandidates[0] != candidate.ID {
		t.Fatalf("delegated policy = calls:%d rules:%+v candidate:%s", runtime.applyCalls, runtime.rules, candidate.ID)
	}
	if !strings.Contains(stdout.String(), "Reviewed permissions applied") ||
		!strings.Contains(stdout.String(), "1 (1 Allow, 0 Deny)") ||
		!strings.Contains(stdout.String(), strings.Repeat("b", 64)) ||
		!strings.Contains(stdout.String(), candidate.ID) ||
		!strings.Contains(stdout.String(), "retry the blocked request") {
		t.Fatalf("review output did not show one reviewed-set Apply: %q", stdout.String())
	}
}

func TestPolicyReviewTTYAppliesSeveralDecisionsWithOneRuntimeCall(t *testing.T) {
	t.Parallel()
	first := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
		Host: "api.example.com", Port: 443, Method: "POST", Path: "/one",
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	}
	second := first
	second.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	second.Path = "/two"
	runtime := &policyReviewRuntimeApplyingFake{policyReviewRuntimeFake: policyReviewRuntimeFake{
		state:   tobari.State{PolicyDirectory: "/tmp/policy"},
		denials: []tobari.PolicyDenial{first, second}, terminal: true,
	}}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("1\na\n2\nd\np\ny\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.tobari = tobaricmd.New(runtime)
	if code := command.RunContext(context.Background(), []string{"policy", "review"}); code != ExitOK {
		t.Fatalf("policy review code = %d, stderr = %q", code, stderr.String())
	}
	if runtime.applyCalls != 1 || len(runtime.rules) != 1 || len(runtime.denyRules) != 1 {
		t.Fatalf("reviewed set calls=%d allows=%+v denies=%+v", runtime.applyCalls, runtime.rules, runtime.denyRules)
	}
	if !strings.Contains(stdout.String(), "2 (1 Allow, 1 Deny)") ||
		!strings.Contains(stdout.String(), "1. Allow exact") ||
		!strings.Contains(stdout.String(), "2. Deny exact") {
		t.Fatalf("reviewed-set summary = %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "2 exact decisions ready for one Apply") {
		t.Fatalf("staged-set feedback = %q", stdout.String())
	}
}

func TestPolicyReviewTTYRefreshReconcilesStagedDecisionsByCandidateID(t *testing.T) {
	t.Parallel()
	first := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
		Host: "api.example.com", Port: 443, Method: "POST", Path: "/one",
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	}
	second := first
	second.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	second.Path = "/two"
	secondCandidate, err := tobari.NewPolicyCandidate(second)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &policyReviewRuntimeApplyingFake{policyReviewRuntimeFake: policyReviewRuntimeFake{
		state: tobari.State{PolicyDirectory: "/tmp/policy"}, terminal: true,
		denialsByRead: [][]tobari.PolicyDenial{{first, second}, {second}, {second}},
	}}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("1\na\n2\nd\nr\np\ny\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.tobari = tobaricmd.New(runtime)
	if code := command.RunContext(context.Background(), []string{"policy", "review"}); code != ExitOK {
		t.Fatalf("policy review code = %d, stderr = %q", code, stderr.String())
	}
	if runtime.applyCalls != 1 || len(runtime.rules) != 0 || len(runtime.denyRules) != 1 ||
		len(runtime.denyRules[0].SourceCandidates) != 1 || runtime.denyRules[0].SourceCandidates[0] != secondCandidate.ID {
		t.Fatalf("reconciled Apply calls=%d allows=%+v denies=%+v", runtime.applyCalls, runtime.rules, runtime.denyRules)
	}
	for _, want := range []string{"1 stale staged decision removed", "remaining decisions preserved", secondCandidate.ID, "1 (0 Allow, 1 Deny)"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("refresh output %q lacks %q", stdout.String(), want)
		}
	}
}

func TestPolicyReviewTTYRefreshDoesNotTransferStageToDifferentCandidateWithMatchingLabels(t *testing.T) {
	t.Parallel()
	original := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
		Host: "api.example.com", Port: 443, Method: "POST", Path: "/one",
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	}
	replacement := original
	replacement.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	replacement.Path = "/replacement"
	runtime := &policyReviewRuntimeApplyingFake{policyReviewRuntimeFake: policyReviewRuntimeFake{
		state: tobari.State{PolicyDirectory: "/tmp/policy"}, terminal: true,
		denialsByRead: [][]tobari.PolicyDenial{{original}, {replacement}},
	}}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("1\na\nr\np\nq\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.tobari = tobaricmd.New(runtime)
	if code := command.RunContext(context.Background(), []string{"policy", "review"}); code != ExitCanceled {
		t.Fatalf("policy review code = %d, stderr = %q", code, stderr.String())
	}
	if runtime.applyCalls != 0 || len(runtime.rules) != 0 || len(runtime.denyRules) != 0 {
		t.Fatalf("replacement candidate inherited authority: calls=%d allows=%+v denies=%+v", runtime.applyCalls, runtime.rules, runtime.denyRules)
	}
	for _, want := range []string{"1 stale staged decision removed", "Inspect a permission and stage Allow exact or Deny exact first."} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("replacement output %q lacks %q", stdout.String(), want)
		}
	}
}

func TestPolicyReviewTTYKeepsOneContextPerStagedApply(t *testing.T) {
	t.Parallel()
	first := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
		Host: "api.example.com", Port: 443, Method: "POST", Path: "/one",
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	}
	second := first
	second.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	second.ContextID = "01912345-6789-7abc-8def-0123456789ae"
	second.ContextName = "restricted"
	second.ProjectID = "01912345-6789-7abc-8def-0123456789ac"
	second.ProjectRoot = "/workspace/restricted"
	second.Path = "/two"
	runtime := &policyReviewRuntimeApplyingFake{policyReviewRuntimeFake: policyReviewRuntimeFake{
		state:   tobari.State{PolicyDirectory: "/tmp/policy"},
		denials: []tobari.PolicyDenial{first, second}, terminal: true,
	}}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("1\na\n2\nd\np\ny\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.tobari = tobaricmd.New(runtime)
	if code := command.RunContext(context.Background(), []string{"policy", "review"}); code != ExitOK {
		t.Fatalf("policy review code = %d, stderr = %q", code, stderr.String())
	}
	if runtime.applyCalls != 1 || len(runtime.rules)+len(runtime.denyRules) != 1 {
		t.Fatalf("mixed Context staging calls=%d allows=%+v denies=%+v", runtime.applyCalls, runtime.rules, runtime.denyRules)
	}
	if !strings.Contains(stdout.String(), "Apply or discard the staged decisions before switching Context.") {
		t.Fatalf("mixed Context staging feedback = %q", stdout.String())
	}
}

func TestPolicyReviewRedirectedInputStaysReadOnly(t *testing.T) {
	t.Parallel()
	denial := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project", Host: "api.example.com", Port: 443,
		Method: "POST", Path: "/repos/example/issues", Reason: "request did not match an allow rule",
		StatusCode: 403, Learnable: true,
	}
	runtime := &policyReviewRuntimeApplyingFake{
		policyReviewRuntimeFake: policyReviewRuntimeFake{
			state:    tobari.State{PolicyDirectory: "/tmp/policy"},
			denials:  []tobari.PolicyDenial{denial},
			terminal: false,
		},
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("1\na\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.tobari = tobaricmd.New(runtime)
	if code := command.RunContext(context.Background(), []string{"policy", "review"}); code != ExitOK {
		t.Fatalf("redirected policy review code = %d, stderr = %q", code, stderr.String())
	}
	if runtime.applyCalls != 0 || len(runtime.rules) != 0 {
		t.Fatalf("redirected review mutated policy: calls:%d rules:%+v", runtime.applyCalls, runtime.rules)
	}
	if !strings.Contains(stdout.String(), "Allow exact") {
		t.Fatalf("redirected review did not remain a review queue: %q", stdout.String())
	}
}

func TestPolicyRulesTTYResetsDecisionAndRefreshesInventory(t *testing.T) {
	t.Parallel()
	denial := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project", Host: "api.example.com", Port: 443,
		Method: "POST", Path: "/repos/example/issues", Reason: "request did not match an allow rule",
		StatusCode: 403, Learnable: true,
	}
	candidate, err := tobari.NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &policyReviewRuntimeApplyingFake{
		policyReviewRuntimeFake: policyReviewRuntimeFake{
			state:    tobari.State{PolicyDirectory: "/tmp/policy"},
			rules:    []tobari.LearnedPolicyRule{rule},
			terminal: true,
		},
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("1\nr\ny\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.tobari = tobaricmd.New(runtime)
	if code := command.RunContext(context.Background(), []string{"policy", "rules"}); code != ExitOK {
		t.Fatalf("policy rules code = %d, stderr = %q", code, stderr.String())
	}
	if runtime.applyCalls != 1 || len(runtime.rules) != 0 {
		t.Fatalf("policy rules reset calls=%d rules=%+v", runtime.applyCalls, runtime.rules)
	}
	output := stdout.String()
	if !strings.Contains(output, "Policy decision reset") || !strings.Contains(output, "No learned policy decisions") {
		t.Fatalf("policy rules output did not show reset and refresh: %q", output)
	}
}

func TestPolicyRulesJSONIsReadOnlyAndMatchesCatalog(t *testing.T) {
	t.Parallel()
	denial := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project", Host: "api.example.com", Port: 443,
		Method: "POST", Path: "/repos/example/issues", Reason: "request did not match an allow rule",
		StatusCode: 403, Learnable: true,
	}
	candidate, err := tobari.NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &policyReviewRuntimeApplyingFake{
		policyReviewRuntimeFake: policyReviewRuntimeFake{
			state:    tobari.State{PolicyDirectory: "/tmp/policy"},
			rules:    []tobari.LearnedPolicyRule{rule},
			terminal: false,
		},
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("1\nr\ny\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.tobari = tobaricmd.New(runtime)
	if code := command.RunContext(context.Background(), []string{"policy", "rules", "--format", "json"}); code != ExitOK {
		t.Fatalf("policy rules JSON code = %d, stderr = %q", code, stderr.String())
	}
	if runtime.applyCalls != 0 || len(runtime.rules) != 1 {
		t.Fatalf("JSON policy rules mutated state: calls=%d rules=%+v", runtime.applyCalls, runtime.rules)
	}
	spec, found := DefaultCatalog().Lookup("policy rules")
	if !found {
		t.Fatal("policy rules is absent")
	}
	assertJSONItemFieldsMatchCatalog(t, stdout.Bytes(), spec)
}

func TestPolicyRulesHumanStylesOnlyDecisionsAndResetCommands(t *testing.T) {
	t.Parallel()
	report := tobari.PolicyRuleReport{
		Task: tobari.TaskPolicyRules, PolicyDirectory: "/tmp/policy",
		Items: []tobari.PolicyRule{
			{ID: "prl_allow", Decision: tobari.PolicyDecisionAllow, Match: tobari.PolicyMatchExact, ProjectID: "project-allow", Host: "api.example.com", Port: 443, Method: "GET", Path: "/allowed", Examples: []string{"/allowed"}, SourceCandidates: []string{"candidate-allow"}},
			{ID: "prl_deny", Decision: tobari.PolicyDecisionDeny, Match: tobari.PolicyMatchExact, ProjectID: "project-deny", Host: "api.example.com", Port: 443, Method: "POST", Path: "/denied", Examples: []string{}, SourceCandidates: []string{"candidate-deny"}},
		},
	}
	output := string(renderPolicyRulesHuman(report, "tobari policy reset", true))
	for _, want := range []string{
		applyStyleToken(true, styleSuccess, "✓"),
		applyStyleToken(true, styleSuccess, "Allowed (1)"),
		applyStyleToken(true, styleDanger, "Denied (1)"),
		applyStyleToken(true, styleAccent, "tobari policy reset --id prl_allow"),
		applyStyleToken(true, styleAccent, "tobari policy reset --id prl_deny"),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("policy rules output %q lacks %q", output, want)
		}
	}
	for _, ordinary := range []string{"Learned policy decisions (2)", "/tmp/policy", "prl_allow", "prl_deny", "project-allow", "project-deny", "api.example.com:443 GET /allowed", "api.example.com:443 POST /denied"} {
		for _, token := range []styleToken{styleMuted, styleAccent, styleSuccess, styleWarning, styleDanger} {
			if strings.Contains(output, applyStyleToken(true, token, ordinary)) {
				t.Fatalf("policy rules ordinary value %q used %s: %q", ordinary, token, output)
			}
		}
	}
}

func TestDeleteCatalogDescribesDetachedDefaultAndAttachedForceGuard(t *testing.T) {
	t.Parallel()
	command, found := DefaultCatalog().Lookup("delete")
	if !found {
		t.Fatal("delete command is absent from the catalog")
	}
	if !strings.Contains(command.Summary, "no session is attached") ||
		!strings.Contains(command.Agent.Outcome, "--force") {
		t.Fatalf("delete contract does not describe detached default and force override: %+v", command)
	}
	var force CommandInput
	var forceFound bool
	for _, input := range command.Agent.Inputs {
		if input.Name == "--force" {
			force, forceFound = input, true
			break
		}
	}
	if !forceFound || !strings.Contains(force.Description, "attached-session") {
		t.Fatalf("delete --force input = %+v", force)
	}
	var attached, confirmation bool
	for _, declared := range command.Agent.Errors {
		switch declared.Code {
		case "project_session_attached":
			attached = declared.Kind == fault.KindRejected && !declared.Retryable
			if len(declared.NextActions) != 1 || declared.NextActions[0].Command != "delete" {
				t.Fatalf("attached-session recovery = %+v, want direct delete recovery", declared.NextActions)
			}
		case "confirmation_required":
			confirmation = true
		}
	}
	if !attached || confirmation {
		t.Fatalf("delete faults attached=%t confirmation=%t", attached, confirmation)
	}
}

func TestProjectSessionClosedSummaryStaysOnHostLifecycleStream(t *testing.T) {
	t.Parallel()
	var hostStderr bytes.Buffer
	childStdout := &bytes.Buffer{}
	if _, err := writeOnce(&hostStderr, renderProjectSessionClosed(false)); err != nil {
		t.Fatalf("writeOnce() error = %v", err)
	}
	if childStdout.Len() != 0 {
		t.Fatalf("child stdout received host lifecycle guidance: %q", childStdout.String())
	}
	if got, want := hostStderr.String(), "Workspace session closed.\nWorkspace remains available.\n\nResume: tobari\nRemove: tobari delete\nIf another session is attached: tobari delete --force\n"; got != want {
		t.Fatalf("host lifecycle guidance = %q, want %q", got, want)
	}
}

func TestProjectSessionClosedStylesLabelsCommandsAndProseSeparately(t *testing.T) {
	t.Parallel()
	output := string(renderProjectSessionClosed(true))
	for _, prose := range []string{"Workspace session closed.", "Workspace remains available."} {
		for _, token := range []styleToken{styleMuted, styleAccent, styleSuccess, styleWarning, styleDanger} {
			if strings.Contains(output, applyStyleToken(true, token, prose)) {
				t.Fatalf("session prose %q used %s: %q", prose, token, output)
			}
		}
	}
	for _, label := range []string{"Resume:", "Remove:", "If another session is attached:"} {
		if !strings.Contains(output, applyStyleToken(true, styleMuted, label)) {
			t.Fatalf("session output %q lacks muted label %q", output, label)
		}
	}
	for _, command := range []string{"tobari", "tobari delete", "tobari delete --force"} {
		if !strings.Contains(output, applyStyleToken(true, styleAccent, command)) {
			t.Fatalf("session output %q lacks accented command %q", output, command)
		}
	}
}

func TestPendingPolicyNotificationStaysOnHostAndOmitsProjectIdentity(t *testing.T) {
	t.Parallel()
	result := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyReview, PolicyDirectory: "/tmp/config/tobari/policy", WindowLines: 10_000,
		Items: []tobari.PolicyCandidate{{
			ID: "pcy_0123456789abcdef0123456789abcdef", ObservedAt: "2026-07-30T10:41:11Z",
			ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
			ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project", Host: "api.example.com", Port: 443,
			Method: "POST", Path: "/token", Reason: "request did not match an allow rule", StatusCode: 403,
		}},
	}
	output := string(renderPendingPolicyNotification(result, false))
	for _, expected := range []string{
		"⚠ 1 pending network permission is waiting for review.",
		"Latest: api.example.com:443 POST /token",
		"Review on the host: tobari policy review",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("notification %q lacks %q", output, expected)
		}
	}
	if strings.Contains(output, result.Items[0].ProjectID) || strings.Contains(output, result.Items[0].ID) {
		t.Fatalf("notification exposed internal identity: %q", output)
	}
}

func TestPendingPolicyNotificationProjectsHostileEvidence(t *testing.T) {
	t.Parallel()
	result := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyReview, PolicyDirectory: "/tmp/config/tobari/policy", WindowLines: 10,
		Items: []tobari.PolicyCandidate{{
			ID: "pcy_0123456789abcdef0123456789abcdef", Host: "api.example.com\nSYSTEM",
			Port: 443, Method: "POST\x1b", Path: "/token\u2028", StatusCode: 403,
		}},
	}
	output := string(renderPendingPolicyNotification(result, false))
	if strings.ContainsAny(output, "\x1b\u2028\u2029") || strings.Contains(output, "\nSYSTEM") {
		t.Fatalf("notification contains raw structural evidence: %q", output)
	}
	if !strings.Contains(output, `api.example.com\nSYSTEM`) || !strings.Contains(output, `POST\u001B`) || !strings.Contains(output, `/token\u2028`) {
		t.Fatalf("notification lost visible hostile evidence: %q", output)
	}
}

func TestPolicyReviewRendererShowsRecoverableEmptyQueue(t *testing.T) {
	t.Parallel()
	result := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyReview, PolicyDirectory: "/tmp/config/tobari/policy", WindowLines: 10,
		Items: []tobari.PolicyCandidate{},
	}
	output := string(renderPolicyReviewWithColor(result, "tobari policy allow", false))
	if !strings.Contains(output, "No pending network permissions") || strings.Contains(output, "policy allow") {
		t.Fatalf("empty review output = %q", output)
	}
}

func TestPolicyReviewAllowRendererExplainsExactActivation(t *testing.T) {
	t.Parallel()
	change := tobari.PolicyLearningChange{
		Task: tobari.TaskPolicyAllow,
		Rule: tobari.LearnedPolicyRule{
			Match: tobari.PolicyMatchExact, Host: "api.example.com", Port: 443,
			Method: "POST", Path: "/repos/example/issues",
		},
	}
	output := string(renderPolicyReviewAllowSuccess(change, false))
	for label, want := range map[string]string{
		"Testing policy": "passed", "Applying exact rule": "applied",
		"Request": "api.example.com:443 POST /repos/example/issues",
		"Next":    "retry the same request in the current running Workspace",
	} {
		if !humanOutputHasRow(output, label, want) {
			t.Fatalf("allow output %q lacks %s=%q", output, label, want)
		}
	}
	humanOutput := string(renderPolicyLearningChangeWithColor(change, true))
	if !strings.Contains(humanOutput, "current running Workspace") || strings.Contains(humanOutput, "Re-enter") {
		t.Fatalf("allow recovery output is not same-session guidance: %q", humanOutput)
	}
}

func TestTobariListRendererPreservesOpaqueIDAndEmptyScope(t *testing.T) {
	t.Parallel()
	id := "tbr_0123456789abcdef0123456789abcdef"
	result := tobari.ListResult{
		Task: tobari.TaskList,
		Items: []tobari.ItemStatus{{
			ID: id, Name: "work", Root: "/tmp/work", Image: "workbench:dev",
			Running: true, Container: "tobari-work",
		}},
	}
	output, err := renderTobariList(result, successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document tobariListDocument
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Tobari) != 1 || document.Tobari[0].ID != id {
		t.Fatalf("list output = %+v", document)
	}
	empty, err := renderTobariList(
		tobari.ListResult{Task: tobari.TaskList, Items: []tobari.ItemStatus{}},
		successFormatJSON,
	)
	if err != nil || string(empty) != "{\"schema_version\":2,\"tobari\":[]}\n" {
		t.Fatalf("empty list = %q, error = %v", empty, err)
	}
}

func TestTobariListRendererMatchesCatalogFields(t *testing.T) {
	t.Parallel()
	result := tobari.ProjectListResult{
		Task: tobari.TaskProjectList, CurrentID: "01912345-6789-7abc-8def-0123456789ab",
		Items: []tobari.ProjectListItem{{
			Root: "/tmp/project", ID: "01912345-6789-7abc-8def-0123456789ab",
			Home: "/tmp/state/home", ContextID: "018bcfe5-687b-7000-8000-000000000099", ContextName: "default", Runtime: tobari.RuntimeDiagnosticReady,
		}},
	}
	output, err := renderProjectList(result, successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(document["tobari"], &items); err != nil || len(items) != 1 {
		t.Fatalf("list items = %v, error = %v", items, err)
	}
	gotFields := make([]string, 0, len(items[0]))
	for field := range items[0] {
		gotFields = append(gotFields, field)
	}
	spec, found := DefaultCatalog().Lookup("list")
	if !found {
		t.Fatal("list command is absent from the catalog")
	}
	wantFields := make([]string, 0, len(spec.Agent.Output.Fields))
	for _, field := range spec.Agent.Output.Fields {
		wantFields = append(wantFields, field.Name)
	}
	sort.Strings(gotFields)
	sort.Strings(wantFields)
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("list JSON fields = %v, catalog = %v", gotFields, wantFields)
	}
	if strings.Contains(string(output), "current") {
		t.Fatalf("list JSON leaked presentation-only selection metadata: %q", output)
	}
}

func TestProjectListHumanRendererUsesWorkspaceLayoutAndTextValues(t *testing.T) {
	t.Parallel()
	result := tobari.ProjectListResult{
		Task: tobari.TaskProjectList, CurrentID: "01912345-6789-7abc-8def-0123456789ab",
		Items: []tobari.ProjectListItem{
			{
				Root: "/tmp/parent", ID: "01912345-6789-7abc-8def-0123456789aa",
				Home: "/tmp/state/parent", ContextID: "018bcfe5-687b-7000-8000-000000000099", ContextName: "default", Runtime: tobari.RuntimeDiagnosticMissing,
			},
			{
				Root: "/tmp/project", ID: "01912345-6789-7abc-8def-0123456789ab",
				Home: "/tmp/state/project", ContextID: "018bcfe5-687b-7000-8000-000000000099", ContextName: "default", Runtime: tobari.RuntimeDiagnosticReady,
			},
		},
	}
	output, err := renderProjectListWithColor(result, successFormatText, true)
	if err != nil {
		t.Fatal(err)
	}
	value := string(output)
	for _, want := range []string{
		applyStyleToken(true, styleSuccess, "✓"),
		"Workspaces (2)",
		"  /tmp/parent",
		"▸ /tmp/project",
		applyStyleToken(true, styleSuccess, "ready"),
		applyStyleToken(true, styleWarning, "missing"),
		"01912345-6789-7abc-8def-0123456789ab",
	} {
		if !strings.Contains(value, want) {
			t.Fatalf("workspace list output %q lacks %q", value, want)
		}
	}
	if strings.Contains(value, "Project 1") || strings.Contains(value, "current") {
		t.Fatalf("workspace list output retained retired labels: %q", value)
	}
	for _, token := range []styleToken{styleMuted, styleAccent, styleSuccess, styleWarning, styleDanger} {
		if strings.Contains(value, applyStyleToken(true, token, "01912345-6789-7abc-8def-0123456789ab")) {
			t.Fatalf("workspace ID used %s instead of text: %q", token, value)
		}
	}
	for _, token := range []styleToken{styleMuted, styleAccent, styleSuccess, styleWarning, styleDanger} {
		if strings.Contains(value, applyStyleToken(true, token, "/tmp/parent")) {
			t.Fatalf("workspace path used %s instead of text: %q", token, value)
		}
	}
}

func TestProjectDeleteHumanRendererPreservesPlainInformationUnion(t *testing.T) {
	t.Parallel()
	result := tobari.ProjectDeleteResult{
		Task: tobari.TaskDelete, Deleted: true,
		Root: "/tmp/project", ID: "01912345-6789-7abc-8def-0123456789ab",
		Home:      "/tmp/state/project/home",
		ContextID: "018bcfe5-687b-7000-8000-000000000099", ContextName: "default",
	}
	output := string(renderProjectDeleteWithColor(result, true))
	if !strings.Contains(output, "Tobari deleted") || !strings.Contains(output, "/tmp/project") || !strings.Contains(output, "tobari") {
		t.Fatalf("delete output lost the user-facing result: %q", output)
	}
	for label, want := range map[string]string{
		"Deleted": "yes", "Root": result.Root, "Context": result.ContextName,
		"Context ID": result.ContextID, "Diagnostic ID": result.ID, "Diagnostic home": result.Home,
	} {
		if !humanOutputHasRow(output, label, want) {
			t.Fatalf("delete output lost legacy plain fact %s=%q: %q", label, want, output)
		}
	}
	if strings.Contains(output, applyStyleToken(true, styleAccent, "Tobari deleted")) ||
		strings.Contains(output, applyStyleToken(true, styleSuccess, "Tobari deleted")) {
		t.Fatalf("delete output styles the full heading: %q", output)
	}
	for _, token := range []styleToken{styleMuted, styleAccent, styleSuccess, styleWarning, styleDanger} {
		if strings.Contains(output, applyStyleToken(true, token, result.Root)) {
			t.Fatalf("delete root used %s instead of text: %q", token, output)
		}
	}
	if !strings.Contains(output, applyStyleToken(true, styleAccent, "tobari")) ||
		strings.Contains(output, applyStyleToken(true, styleAccent, "— Create or enter a Tobari from this project directory.")) {
		t.Fatalf("delete next action does not isolate command emphasis: %q", output)
	}
}

func TestProjectStatusHumanStylesLabelsStateValuesAndNextCommand(t *testing.T) {
	t.Parallel()
	result := tobari.ProjectStatus{
		Task: tobari.TaskStatus, ContextState: tobari.ContextObservationPersisted, Exists: true, Root: "/tmp/project",
		ID: "01912345-6789-7abc-8def-0123456789ab", Home: "/tmp/state/project/home",
		Runtime:   tobari.RuntimeDiagnosticReady,
		ContextID: "018bcfe5-687b-7000-8000-000000000099", ContextName: "default",
		Attachment: tobari.AttachmentDetached,
	}
	output, err := renderProjectStatusWithColor(result, successFormatText, true)
	if err != nil {
		t.Fatal(err)
	}
	value := string(output)
	for _, label := range []string{"Root", "Runtime", "Session", "ID", "Home", "Next"} {
		padded := fmt.Sprintf("%-*s", humanOutputLabelWidth, label)
		if !strings.Contains(value, applyStyleToken(true, styleMuted, padded)) {
			t.Fatalf("status output %q lacks muted label %q", value, label)
		}
	}
	for _, want := range []string{
		applyStyleToken(true, styleSuccess, "✓"),
		applyStyleToken(true, styleSuccess, "ready"),
		applyStyleToken(true, styleAccent, "tobari --context default"),
	} {
		if !strings.Contains(value, want) {
			t.Fatalf("status output %q lacks %q", value, want)
		}
	}
	for _, ordinary := range []string{"Tobari ready", result.Root, result.ID, result.Home} {
		for _, token := range []styleToken{styleMuted, styleAccent, styleSuccess, styleWarning, styleDanger} {
			if strings.Contains(value, applyStyleToken(true, token, ordinary)) {
				t.Fatalf("status ordinary value %q used %s: %q", ordinary, token, value)
			}
		}
	}
}

func TestProjectStatusRecoveryPreservesNonActiveContextInTextAndJSON(t *testing.T) {
	t.Parallel()
	result := tobari.ProjectStatus{
		Task: tobari.TaskStatus, ContextState: tobari.ContextObservationPersisted, Exists: false, Runtime: tobari.RuntimeDiagnosticUnknown,
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "toolbox",
		Attachment: tobari.AttachmentNotApplicable,
	}
	textOutput, err := renderProjectStatusWithColor(result, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	if !humanOutputHasRow(string(textOutput), "Next", "tobari --context toolbox — Create or enter a Workspace in this Context.") {
		t.Fatalf("plain recovery lost Context: %q", textOutput)
	}
	humanOutput, err := renderProjectStatusWithColor(result, successFormatText, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(humanOutput), applyStyleToken(true, styleAccent, "tobari --context toolbox")) {
		t.Fatalf("human recovery lost Context: %q", humanOutput)
	}
	jsonOutput, err := renderProjectStatusWithColor(result, successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	var document projectStatusDocument
	if err := json.Unmarshal(jsonOutput, &document); err != nil {
		t.Fatal(err)
	}
	if got, want := document.Status.NextArgv, []string{"tobari", "--context", "toolbox"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON next argv = %q, want %q", got, want)
	}
}

func TestSyntheticProjectStatusUsesOmissionBasedRootRecovery(t *testing.T) {
	t.Parallel()
	result := tobari.ProjectStatus{
		Task: tobari.TaskStatus, ContextState: tobari.ContextObservationSyntheticDefault,
		ContextName: tobari.DefaultContextName, Runtime: tobari.RuntimeDiagnosticUnknown,
		Attachment: tobari.AttachmentNotApplicable,
	}
	textOutput, err := renderProjectStatusWithColor(result, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	if !humanOutputHasRow(string(textOutput), "Next", "tobari — Create or enter a Workspace in this Context.") ||
		strings.Contains(string(textOutput), "--context default") {
		t.Fatalf("synthetic status recovery claims an explicit Context selector: %q", textOutput)
	}
	jsonOutput, err := renderProjectStatusWithColor(result, successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	var document projectStatusDocument
	if err := json.Unmarshal(jsonOutput, &document); err != nil {
		t.Fatal(err)
	}
	if got, want := document.Status.NextArgv, []string{"tobari"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("synthetic JSON next argv = %q, want %q", got, want)
	}
	if routed := assertPublicNextArgvRoutes(t, document.Status.NextArgv); routed.Path != "tobari" {
		t.Fatalf("synthetic recovery routes to %q, want root entry", routed.Path)
	}
}

func TestProjectEntryDeclaresClusterProjectionStaleRecovery(t *testing.T) {
	t.Parallel()
	catalog := DefaultCatalog()
	commands := catalog.Commands()
	found := false
	for index := range commands {
		if commands[index].Path != "tobari" {
			continue
		}
		found = true
		commands[index].handler = func(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
			return c.fail(ctx, fault.New(
				fault.KindRejected,
				"cluster_projection_stale",
				"the shared cluster has not loaded the complete Context catalog",
				false,
				fault.NextAction{Command: "untrusted recovery", Reason: "untrusted runtime prose"},
			))
		}
		break
	}
	if !found {
		t.Fatal("root entry is absent from the catalog")
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, NewCatalog(commands...), nil)
	if code := command.RunContext(context.Background(), []string{"--error-format=json"}); code != ExitRejected {
		t.Fatalf("stale projection exit = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"cluster_projection_stale"`) ||
		!strings.Contains(stderr.String(), `"command":"cluster up"`) ||
		strings.Contains(stderr.String(), "undeclared_fault_contract") || strings.Contains(stderr.String(), "untrusted recovery") {
		t.Fatalf("stale projection fault was not aligned to catalog recovery: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if routed := assertPublicNextArgvRoutes(t, []string{ProgramName, "cluster", "up"}); routed.Path != "cluster up" {
		t.Fatalf("stale projection recovery routes to %q", routed.Path)
	}
}

func TestSyntheticProjectStatusJSONHasNoContextAuthority(t *testing.T) {
	t.Parallel()
	result := tobari.ProjectStatus{
		Task: tobari.TaskStatus, ContextState: tobari.ContextObservationSyntheticDefault,
		ContextName: tobari.DefaultContextName, Runtime: tobari.RuntimeDiagnosticUnknown,
		Attachment: tobari.AttachmentNotApplicable,
	}
	encoded, err := renderProjectStatus(result, successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document projectStatusDocument
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 4 || document.Status.ContextState != tobari.ContextObservationSyntheticDefault ||
		document.Status.ContextID != nil || document.Status.Exists {
		t.Fatalf("synthetic lifecycle status claims Context authority: %+v", document)
	}
}

func TestLifecycleScopedHelpPublishesBothContextPlacements(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"tobari", "status", "delete"} {
		command, found := DefaultCatalog().Lookup(path)
		if !found {
			t.Fatalf("catalog lacks %q", path)
		}
		help := string(renderCommandHelp(command))
		for _, placement := range []string{"tobari --context toolbox status", "tobari status --context toolbox"} {
			if !strings.Contains(help, placement) {
				t.Fatalf("scoped help for %q lacks placement %q:\n%s", path, placement, help)
			}
		}
	}
}

func TestClusterStatusRendererExposesXDGPolicyAndTobariCount(t *testing.T) {
	t.Parallel()
	status := tobari.ClusterStatus{
		Task: tobari.TaskClusterStatus, Configured: true, Running: true,
		Proxy: "http://gateway:8080", Policy: "/tmp/config/tobari/policy",
		PolicyRevision: strings.Repeat("a", 64), PolicyProjection: "valid",
		PrincipalRegistry: "valid", CredentialProjection: "valid",
		AuthProviderProjection: "valid", AuthBrokerState: "ready", CredentialCompanionState: "ready", RootKeyBackend: "xdg_file",
		TobariCount: 2, Components: []tobari.ComponentStatus{
			{Name: "auth-broker", State: "running", Health: "healthy"},
			{Name: "gateway", State: "running", Health: "healthy"},
			{Name: "opa", State: "running", Health: "healthy"},
		},
	}
	output := string(renderClusterStatusText(status))
	for _, expected := range []string{
		"✓ Cluster ready", "  Auth     healthy", "  Gateway  healthy", "  OPA      healthy",
		"providers valid", "broker ready / companion ready / root key xdg_file",
		"  Policy   /tmp/config/tobari/policy", "  Tobari   2",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("status output %q lacks %q", output, expected)
		}
	}
	for _, omitted := range []string{"Configured", "Running", "Proxy"} {
		if strings.Contains(output, omitted) {
			t.Fatalf("ready summary retained redundant detail %q: %q", omitted, output)
		}
	}
}

func TestClusterStatusTextUsesSameSummaryForUnconfiguredAndNotReadyStates(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		status tobari.ClusterStatus
		want   []string
	}{
		{
			name: "unconfigured",
			status: tobari.ClusterStatus{
				Task: tobari.TaskClusterStatus, Components: []tobari.ComponentStatus{},
			},
			want: []string{"○ Cluster not configured"},
		},
		{
			name: "removed",
			status: tobari.ClusterStatus{
				Task: tobari.TaskClusterDown, Components: []tobari.ComponentStatus{},
			},
			want: []string{"✓ Cluster removed"},
		},
		{
			name: "not ready",
			status: tobari.ClusterStatus{
				Task: tobari.TaskClusterStatus, Configured: true, Running: false,
				Proxy: "http://gateway:8080", Policy: "/tmp/config/tobari/policy",
				Components: []tobari.ComponentStatus{{
					Name: "gateway", State: "running", Health: "unhealthy",
				}}, RecentError: "Gateway healthcheck failed\ninspect logs",
			},
			want: []string{
				"! Cluster not ready", "  Gateway  running · unhealthy",
				"Recent error  Gateway healthcheck failed\\ninspect logs",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := string(renderClusterStatusText(test.status))
			for _, expected := range test.want {
				if !strings.Contains(output, expected) {
					t.Fatalf("status output %q lacks %q", output, expected)
				}
			}
		})
	}
}

func TestClusterStatusTextUsesSemanticColorTokens(t *testing.T) {
	t.Parallel()
	status := tobari.ClusterStatus{
		Task: tobari.TaskClusterStatus, Configured: true, Running: true,
		Policy: "/tmp/config/tobari/policy", TobariCount: 0,
		Components: []tobari.ComponentStatus{{Name: "gateway", State: "running", Health: "healthy"}},
	}
	output := string(renderClusterStatusTextWithColor(status, true))
	for _, expected := range []string{
		applyStyleToken(true, styleSuccess, "✓"),
		applyStyleToken(true, styleSuccess, "healthy"),
		ansiStyleTokens[styleMuted] + "Tobari",
		ansiStyleTokens[styleMuted] + "Policy",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("colored status output %q lacks %q", output, expected)
		}
	}
	if strings.Contains(output, applyStyleToken(true, styleDanger, "healthy")) {
		t.Fatalf("healthy status used error color: %q", output)
	}
	for _, ordinary := range []string{"Cluster ready", status.Policy} {
		for _, token := range []styleToken{styleMuted, styleAccent, styleSuccess, styleWarning, styleDanger} {
			if strings.Contains(output, applyStyleToken(true, token, ordinary)) {
				t.Fatalf("cluster status ordinary value %q used %s: %q", ordinary, token, output)
			}
		}
	}
}

func TestClusterStatusTextColorsWarningAndFailureStates(t *testing.T) {
	t.Parallel()
	status := tobari.ClusterStatus{
		Task: tobari.TaskClusterStatus, Configured: true, Running: false,
		Components: []tobari.ComponentStatus{{Name: "gateway", State: "running", Health: "unhealthy"}},
	}
	output := string(renderClusterStatusTextWithColor(status, true))
	for _, expected := range []string{
		applyStyleToken(true, styleWarning, "!"),
		applyStyleToken(true, styleDanger, "unhealthy"),
		applyStyleToken(true, styleSuccess, "running"),
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("warning/failure output %q lacks %q", output, expected)
		}
	}
}

func TestClusterUpTextAddsNextActionToSharedSummary(t *testing.T) {
	t.Parallel()
	status := tobari.ClusterStatus{
		Task: tobari.TaskClusterUp, Configured: true, Running: true,
		Policy: "/tmp/config/tobari/policy", Components: []tobari.ComponentStatus{},
	}
	output := string(renderClusterUpText(status, false))
	for _, expected := range []string{
		"✓ Cluster ready", "Next: from a project directory, run `tobari`.",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("cluster up output %q lacks %q", output, expected)
		}
	}
}

func TestClusterStatusJSONDoesNotContainTerminalColors(t *testing.T) {
	t.Parallel()
	status := tobari.ClusterStatus{
		Task: tobari.TaskClusterStatus, Configured: true, Running: true,
		Proxy: "http://127.0.0.1:18080", Policy: "/tmp/config/tobari/policy",
		ContextCount: 1, PolicyRevision: strings.Repeat("a", 64), Components: validClusterComponentStatuses(),
		PolicyProjection: "valid", PrincipalRegistry: "valid", CredentialProjection: "valid",
		AuthProviderProjection: "valid", AuthBrokerState: "ready", CredentialCompanionState: "ready", RootKeyBackend: "xdg_file",
	}
	output, err := renderClusterStatus(status, successFormatJSON, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), "\x1b[") {
		t.Fatalf("cluster status JSON contains terminal colors: %q", output)
	}
}

func TestClusterStatusJSONExposesAuthBrokerSemantics(t *testing.T) {
	t.Parallel()
	status := tobari.ClusterStatus{
		Task: tobari.TaskClusterStatus, Configured: true, Running: true,
		Proxy: "http://gateway:8080", Policy: "/tmp/config/tobari/policy",
		TobariCount: 1, ContextCount: 1, PolicyRevision: strings.Repeat("a", 64),
		PolicyProjection: "valid", PrincipalRegistry: "valid", CredentialProjection: "valid",
		AuthProviderProjection: "valid", AuthBrokerState: "ready", CredentialCompanionState: "ready", RootKeyBackend: "xdg_file",
		Components: validClusterComponentStatuses(),
	}
	output, err := renderClusterStatus(status, successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		SchemaVersion int                        `json:"schema_version"`
		Cluster       map[string]json.RawMessage `json:"cluster"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 5 {
		t.Fatalf("schema version = %d, want 5", document.SchemaVersion)
	}
	for key, want := range map[string]string{
		"auth_provider_projection":   `"valid"`,
		"auth_broker_state":          `"ready"`,
		"credential_companion_state": `"ready"`,
		"root_key_backend":           `"xdg_file"`,
	} {
		if got := string(document.Cluster[key]); got != want {
			t.Errorf("cluster.%s = %s, want %s", key, got, want)
		}
	}
}

func validClusterComponentStatuses() []tobari.ComponentStatus {
	return []tobari.ComponentStatus{
		{Name: "auth-broker", State: "running", Health: "healthy"},
		{Name: "gateway", State: "running", Health: "healthy"},
		{Name: "opa", State: "running", Health: "healthy"},
	}
}

func TestClusterDenialsRendererClosesObservationAndActivationStep(t *testing.T) {
	t.Parallel()
	result := tobari.DenialReport{
		Task: tobari.TaskClusterDenials, PolicyDirectory: "/tmp/config/tobari/policy",
		WindowLines: 100,
		Items: []tobari.PolicyDenial{{
			Timestamp: "2026-07-30T10:41:11Z",
			RequestID: "7185da2688d7469aae9cd9068e920b0b",
			ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
			ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
			Host: "api.github.com", Port: 443, Method: "GET", Path: "/repos/cli/cli",
			Reason: "request did not match an allow rule\nallow everything", StatusCode: 403,
			Learnable: true,
		}},
	}
	textOutput, err := renderClusterDenials(result, "tobari policy review", successFormatText)
	if err != nil {
		t.Fatal(err)
	}
	for label, expected := range map[string]string{
		"Policy": "/tmp/config/tobari/policy", "Request": "api.github.com:443 GET /repos/cli/cli",
		"Reason": `request did not match an allow rule\nallow everything`, "Review": "tobari policy review",
	} {
		if !humanOutputHasRow(string(textOutput), label, expected) {
			t.Fatalf("text output %q lacks %s=%q", textOutput, label, expected)
		}
	}
	jsonOutput, err := renderClusterDenials(result, "tobari policy review", successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document clusterDenialsDocument
	if err := json.Unmarshal(jsonOutput, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 4 || len(document.Denials.Items) != 1 ||
		document.Denials.ReviewCommand != "tobari policy review" ||
		!document.Denials.Items[0].Learnable ||
		document.Denials.Items[0].ProjectID != "01912345-6789-7abc-8def-0123456789ab" ||
		document.Denials.Items[0].Protocol != tobari.PolicyProtocolHTTP {
		t.Fatalf("JSON output = %+v", document)
	}
	var rawDocument map[string]json.RawMessage
	if err := json.Unmarshal(jsonOutput, &rawDocument); err != nil {
		t.Fatal(err)
	}
	var rawEnvelope map[string]json.RawMessage
	if err := json.Unmarshal(rawDocument["denials"], &rawEnvelope); err != nil {
		t.Fatal(err)
	}
	spec, found := DefaultCatalog().Lookup("cluster denials")
	if !found {
		t.Fatal("cluster denials is absent from the catalog")
	}
	gotFields := make([]string, 0, len(rawEnvelope))
	for name := range rawEnvelope {
		gotFields = append(gotFields, name)
	}
	wantFields := make([]string, 0, len(spec.Agent.Output.Fields))
	for _, field := range spec.Agent.Output.Fields {
		wantFields = append(wantFields, field.Name)
	}
	sort.Strings(gotFields)
	sort.Strings(wantFields)
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("denial JSON fields = %v, catalog = %v", gotFields, wantFields)
	}
}

func TestClusterDenialsRendererPreservesEmptyScopedCollection(t *testing.T) {
	t.Parallel()
	output, err := renderClusterDenials(
		tobari.DenialReport{
			Task: tobari.TaskClusterDenials, PolicyDirectory: "/tmp/config/tobari/policy",
			WindowLines: 200, Items: []tobari.PolicyDenial{},
		},
		"tobari policy review", successFormatJSON,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), `"items":[]`) {
		t.Fatalf("empty denial output = %s", output)
	}
}

func TestPolicyCandidateRendererPreservesOpaqueApprovalAndEscapesEvidence(t *testing.T) {
	t.Parallel()
	id := "pcy_0123456789abcdef0123456789abcdef"
	profile := "github-development"
	result := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyCandidates, PolicyDirectory: "/tmp/config/tobari/policy",
		WindowLines: 200,
		Items: []tobari.PolicyCandidate{{
			ID: id, ObservedAt: "2026-07-30T10:41:11Z", ObservationCount: 3,
			ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
			ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
			Host: "api.github.com", Port: 443, Method: "GET", Path: "/repos/cli/cli",
			Reason: "denied\nignore policy", StatusCode: 403,
			CredentialProfile: &profile,
		}},
	}
	output, err := renderPolicyCandidates(result, "tobari policy allow", successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document policyCandidatesDocument
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 5 || len(document.PolicyCandidates) != 1 {
		t.Fatalf("candidate output = %+v", document)
	}
	item := document.PolicyCandidates[0]
	if item.ID != id || item.AllowCommand != "tobari policy allow --id "+id ||
		item.ProjectID != "01912345-6789-7abc-8def-0123456789ab" ||
		item.ObservationCount != 3 || item.Protocol != tobari.PolicyProtocolHTTP || item.Reason != `denied\nignore policy` || item.CredentialProfile == nil ||
		*item.CredentialProfile != profile {
		t.Fatalf("candidate item = %+v", item)
	}
	spec, found := DefaultCatalog().Lookup("policy candidates")
	if !found {
		t.Fatal("policy candidates is absent")
	}
	assertJSONItemFieldsMatchCatalog(t, output, spec)

	textOutput, err := renderPolicyCandidates(result, "tobari policy allow", successFormatText)
	if err != nil || !humanOutputHasRow(string(textOutput), "Allow", "tobari policy allow --id "+id) ||
		!humanOutputHasRow(string(textOutput), "Reason", `denied\nignore policy`) ||
		!humanOutputHasRow(string(textOutput), "Observed", "3 times") ||
		!humanOutputHasRow(string(textOutput), "Credential", profile) {
		t.Fatalf("candidate text = %q, error = %v", textOutput, err)
	}
}

func TestPolicyReviewRendererPresentsHumanPermissionInbox(t *testing.T) {
	t.Parallel()
	id := "pcy_0123456789abcdef0123456789abcdef"
	result := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyReview, PolicyDirectory: "/tmp/config/tobari/policy", WindowLines: 10_000,
		Items: []tobari.PolicyCandidate{{
			ID: id, ObservedAt: "2026-07-30T10:41:11Z", ObservationCount: 3,
			ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
			ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
			Host: "api.example.com", Port: 443, Method: "POST", Path: "/token",
			Reason: "request did not match an allow rule", StatusCode: 403,
		}},
	}
	output := string(renderPolicyReviewWithColor(result, "tobari policy allow", false))
	for _, expected := range []string{
		"Pending network permissions (1)",
		"Context        default",
		"Tobari         /workspace/project",
		"Request        api.example.com:443 POST /token",
		"Observed       3 times",
		"Latest         2026-07-30T10:41:11Z",
		"Allow exact    tobari policy allow --id " + id,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("review output %q lacks %q", output, expected)
		}
	}
	if strings.Contains(output, "01912345-6789-7abc-8def-0123456789ab") || strings.Contains(output, "PolicyDirectory") {
		t.Fatalf("review output exposed unnecessary internal detail: %q", output)
	}
}

func TestPolicyReviewJSONIsReadOnlyProjectionWithBothActions(t *testing.T) {
	t.Parallel()
	id := "pcy_0123456789abcdef0123456789abcdef"
	result := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyReview, PolicyDirectory: "/tmp/config/tobari/policy", WindowLines: 10_000,
		Items: []tobari.PolicyCandidate{{
			ID: id, ObservedAt: "2026-07-30T10:41:11Z",
			ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
			ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
			Host: "api.example.com", Port: 443, Method: "POST", Path: "/token",
			Reason: "request did not match an allow rule", StatusCode: 403,
		}},
	}
	output, err := renderPolicyReviewWithCommands(
		result, "tobari policy allow", "tobari policy deny", successFormatJSON, false,
	)
	if err != nil {
		t.Fatal(err)
	}
	var document policyReviewDocument
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 5 || len(document.PolicyReview) != 1 {
		t.Fatalf("review output = %+v", document)
	}
	item := document.PolicyReview[0]
	if item.ObservationCount != 1 || item.AllowCommand != "tobari policy allow --id "+id ||
		item.DenyCommand != "tobari policy deny --id "+id {
		t.Fatalf("review actions = %+v", item)
	}
	spec, found := DefaultCatalog().Lookup("policy review")
	if !found {
		t.Fatal("policy review is absent")
	}
	assertJSONItemFieldsMatchCatalog(t, output, spec)
}

func TestGraphQLPolicyIdentityAppearsAcrossPublicPolicyOutputs(t *testing.T) {
	t.Parallel()
	denial := tobari.PolicyDenial{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{
			Protocol: tobari.PolicyProtocolGraphQL, GraphQLOperationType: tobari.GraphQLOperationMutation, GraphQLRootField: "updateIssue",
		},
		Timestamp: "2026-08-09T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
		Host: "api.example.com", Port: 443, Method: "POST", Path: "/graphql",
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	}
	candidate, err := tobari.NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	learned, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewPolicyRuleFromLearned(learned)
	if err != nil {
		t.Fatal(err)
	}
	denied, err := tobari.NewExactPolicyDenyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}

	denialJSON, err := renderClusterDenials(tobari.DenialReport{
		Task: tobari.TaskClusterDenials, PolicyDirectory: "/tmp/policy", WindowLines: 200, Items: []tobari.PolicyDenial{denial},
	}, "tobari policy review", successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var denialDocument clusterDenialsDocument
	if err := json.Unmarshal(denialJSON, &denialDocument); err != nil {
		t.Fatal(err)
	}
	if denialDocument.SchemaVersion != 4 || len(denialDocument.Denials.Items) != 1 {
		t.Fatalf("GraphQL denial document = %+v", denialDocument)
	}
	assertGraphQLPolicyOutput(t, denialDocument.Denials.Items[0].Protocol,
		denialDocument.Denials.Items[0].GraphQLOperationType, denialDocument.Denials.Items[0].GraphQLRootField)

	candidateReport := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyCandidates, PolicyDirectory: "/tmp/policy", WindowLines: 200, Items: []tobari.PolicyCandidate{candidate},
	}
	candidateJSON, err := renderPolicyCandidates(candidateReport, "tobari policy allow", successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var candidateDocument policyCandidatesDocument
	if err := json.Unmarshal(candidateJSON, &candidateDocument); err != nil {
		t.Fatal(err)
	}
	if candidateDocument.SchemaVersion != 5 || len(candidateDocument.PolicyCandidates) != 1 {
		t.Fatalf("GraphQL candidate document = %+v", candidateDocument)
	}
	assertGraphQLPolicyOutput(t, candidateDocument.PolicyCandidates[0].Protocol,
		candidateDocument.PolicyCandidates[0].GraphQLOperationType, candidateDocument.PolicyCandidates[0].GraphQLRootField)
	candidateText, err := renderPolicyCandidates(candidateReport, "tobari policy allow", successFormatText)
	if err != nil {
		t.Fatal(err)
	}
	for label, field := range map[string]string{"Protocol": "graphql", "GraphQL": "mutation.updateIssue"} {
		if !humanOutputHasRow(string(candidateText), label, field) {
			t.Fatalf("GraphQL candidate text %q lacks %s=%q", candidateText, label, field)
		}
	}

	reviewHuman := string(renderPolicyReviewWithColor(candidateReport, "tobari policy allow", false))
	if !strings.Contains(reviewHuman, "GraphQL        mutation.updateIssue") {
		t.Fatalf("GraphQL review output lacks exact coordinate: %q", reviewHuman)
	}
	if got := policyReviewCandidateRequest(candidate); !strings.Contains(got, "GraphQL mutation.updateIssue") {
		t.Fatalf("GraphQL selector request = %q", got)
	}

	ruleReport := tobari.PolicyRuleReport{
		Task: tobari.TaskPolicyRules, PolicyDirectory: "/tmp/policy", Items: []tobari.PolicyRule{rule},
	}
	ruleJSON, err := renderPolicyRulesWithCommands(ruleReport, "tobari policy reset", successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	var ruleDocument policyRulesDocument
	if err := json.Unmarshal(ruleJSON, &ruleDocument); err != nil {
		t.Fatal(err)
	}
	if ruleDocument.SchemaVersion != 3 || len(ruleDocument.PolicyRules) != 1 {
		t.Fatalf("GraphQL rule document = %+v", ruleDocument)
	}
	assertGraphQLPolicyOutput(t, ruleDocument.PolicyRules[0].Protocol,
		ruleDocument.PolicyRules[0].GraphQLOperationType, ruleDocument.PolicyRules[0].GraphQLRootField)
	if got := policyRuleRequest(rule); !strings.Contains(got, "GraphQL mutation.updateIssue") {
		t.Fatalf("GraphQL rule selector request = %q", got)
	}

	allowChange := string(renderPolicyLearningChange(tobari.PolicyLearningChange{
		Task: tobari.TaskPolicyAllow, PolicyDirectory: "/tmp/policy", TargetID: candidate.ID,
		Rule: learned, SourceRuleCount: 1, Applied: true,
	}))
	denyChange := string(renderPolicyDenyChangeWithColor(tobari.PolicyDenyChange{
		Task: tobari.TaskPolicyDeny, PolicyDirectory: "/tmp/policy", TargetID: candidate.ID,
		Rule: denied, SourceRuleCount: 1, Applied: true,
	}, false))
	for name, output := range map[string]string{"allow": allowChange, "deny": denyChange} {
		for label, field := range map[string]string{"Protocol": "graphql", "GraphQL": "mutation.updateIssue"} {
			if !humanOutputHasRow(output, label, field) {
				t.Fatalf("GraphQL %s change output %q lacks %s=%q", name, output, label, field)
			}
		}
	}

	httpCandidate := candidate
	httpCandidate.PolicyProtocolIdentity = tobari.PolicyProtocolIdentity{}
	if got, want := policyReviewCandidateRequest(httpCandidate), "api.example.com:443 POST /graphql"; got != want {
		t.Fatalf("legacy HTTP review request = %q, want %q", got, want)
	}
	httpRule := rule
	httpRule.PolicyProtocolIdentity = tobari.PolicyProtocolIdentity{}
	if got, want := policyRuleRequest(httpRule), "api.example.com:443 POST /graphql"; got != want {
		t.Fatalf("legacy HTTP rule request = %q, want %q", got, want)
	}
}

func assertGraphQLPolicyOutput(t *testing.T, protocol, operationType, rootField string) {
	t.Helper()
	if protocol != tobari.PolicyProtocolGraphQL || operationType != tobari.GraphQLOperationMutation || rootField != "updateIssue" {
		t.Fatalf("GraphQL policy identity = protocol:%q operation:%q root:%q", protocol, operationType, rootField)
	}
}

func TestPolicyDenyRendererReportsExactTerminalDecision(t *testing.T) {
	t.Parallel()
	candidate := tobari.PolicyCandidate{
		ID:         "pcy_0123456789abcdef0123456789abcdef",
		ObservedAt: "2026-07-30T10:41:11Z",
		ContextID:  "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
		Host: "api.example.com", Port: 443, Method: "POST", Path: "/token",
		Reason: "request did not match an allow rule", StatusCode: 403,
	}
	rule, err := tobari.NewExactPolicyDenyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	output := string(renderPolicyDenyChangeWithColor(tobari.PolicyDenyChange{
		Task: tobari.TaskPolicyDeny, PolicyDirectory: "/tmp/config/tobari/policy",
		TargetID: candidate.ID, Rule: rule, SourceRuleCount: 1, Applied: true,
	}, false))
	for label, expected := range map[string]string{
		"Target ID": candidate.ID, "Rule ID": rule.ID, "Request": "api.example.com:443 POST /token",
		"Source rules": "1", "Applied": "yes",
	} {
		if !humanOutputHasRow(output, label, expected) {
			t.Fatalf("deny output %q lacks %s=%q", output, label, expected)
		}
	}
}

func TestPolicyCompactionRendererShowsEvidenceAndExactAction(t *testing.T) {
	t.Parallel()
	rules := make([]tobari.LearnedPolicyRule, 0, 3)
	for index, path := range []string{
		"/api/v1/items/one", "/api/v1/items/two", "/api/v1/items/three",
	} {
		candidate := tobari.PolicyCandidate{
			ID:         "pcy_" + strings.Repeat(string(rune('1'+index)), 32),
			ObservedAt: "2026-07-30T10:41:11Z",
			ContextID:  "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
			ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
			Host: "mock-upstream", Port: 8080, Method: "POST", Path: path,
			Reason: "request did not match an allow rule", StatusCode: 403,
		}
		rule, err := tobari.NewExactLearnedPolicyRule(candidate)
		if err != nil {
			t.Fatal(err)
		}
		rules = append(rules, rule)
	}
	items, err := tobari.PolicyCompactions(rules)
	if err != nil || len(items) != 1 {
		t.Fatalf("compactions = %+v, error = %v", items, err)
	}
	report := tobari.PolicyCompactionReport{
		Task: tobari.TaskPolicyCompactions, PolicyDirectory: "/tmp/config/tobari/policy",
		Items: items,
	}
	output, err := renderPolicyCompactions(report, "tobari policy compact", successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document policyCompactionsDocument
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 3 || len(document.PolicyCompactions) != 1 {
		t.Fatalf("compaction output = %+v", document)
	}
	item := document.PolicyCompactions[0]
	if item.ID != items[0].ID || item.SourceRuleCount != 3 ||
		item.ProjectID != "01912345-6789-7abc-8def-0123456789ab" ||
		item.PathPrefix != "/api/v1/items/" ||
		item.OutsideCanary != "/api/v1/items-outside-tobari-canary" ||
		item.CompactCommand != "tobari policy compact --id "+items[0].ID {
		t.Fatalf("compaction item = %+v", item)
	}
	spec, found := DefaultCatalog().Lookup("policy compactions")
	if !found {
		t.Fatal("policy compactions is absent")
	}
	assertJSONItemFieldsMatchCatalog(t, output, spec)
}

func TestPolicyLearningMutationRendererReportsStoredScope(t *testing.T) {
	t.Parallel()
	candidate := tobari.PolicyCandidate{
		ID:         "pcy_0123456789abcdef0123456789abcdef",
		ObservedAt: "2026-07-30T10:41:11Z",
		ContextID:  "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
		Host: "api.github.com", Port: 443, Method: "GET", Path: "/repos/cli/cli",
		Reason: "request did not match an allow rule", StatusCode: 403,
	}
	rule, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	output := string(renderPolicyLearningChange(tobari.PolicyLearningChange{
		Task: tobari.TaskPolicyAllow, PolicyDirectory: "/tmp/config/tobari/policy",
		TargetID: candidate.ID, Rule: rule, SourceRuleCount: 1, Applied: true,
	}))
	for label, expected := range map[string]string{
		"Target ID": candidate.ID, "Rule ID": rule.ID, "Match": "exact",
		"Request": "api.github.com:443 GET /repos/cli/cli", "Source rules": "1", "Applied": "yes",
	} {
		if !humanOutputHasRow(output, label, expected) {
			t.Fatalf("mutation output %q lacks %s=%q", output, label, expected)
		}
	}
}

func assertJSONItemFieldsMatchCatalog(
	t *testing.T, output []byte, spec CommandSpec,
) {
	t.Helper()
	var rawDocument map[string]json.RawMessage
	if err := json.Unmarshal(output, &rawDocument); err != nil {
		t.Fatal(err)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawDocument[spec.Agent.Output.JSONEnvelope], &items); err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("test fixture has no output item")
	}
	gotFields := make([]string, 0, len(items[0]))
	for name := range items[0] {
		gotFields = append(gotFields, name)
	}
	wantFields := make([]string, 0, len(spec.Agent.Output.Fields))
	for _, field := range spec.Agent.Output.Fields {
		wantFields = append(wantFields, field.Name)
	}
	sort.Strings(gotFields)
	sort.Strings(wantFields)
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("JSON fields = %v, catalog = %v", gotFields, wantFields)
	}
}

func TestRetiredNamedCommandsAreUnknown(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"attach", "lower", "enter", "lift"} {
		if _, found := DefaultCatalog().Lookup(path); found {
			t.Fatalf("retired command %q is still public", path)
		}
	}
}
