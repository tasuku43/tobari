package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type policyReviewRuntimeFake struct {
	state      tobari.State
	denials    []tobari.PolicyDenial
	rules      []tobari.LearnedPolicyRule
	denyRules  []tobari.PolicyDenyRule
	applyCalls int
	denyCalls  int
	terminal   bool
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
	context.Context, tobari.State, []tobari.LearnedPolicyRule, []tobari.LearnedPolicyRule,
) error {
	return nil
}
func (f *policyReviewRuntimeFake) ApplyPolicyDenyRules(
	_ context.Context, _ tobari.State, _ []tobari.LearnedPolicyRule,
	_ []tobari.PolicyDenyRule, updated []tobari.PolicyDenyRule,
) error {
	f.denyCalls++
	f.denyRules = append([]tobari.PolicyDenyRule{}, updated...)
	return nil
}
func (f *policyReviewRuntimeFake) ClusterDown(context.Context, tobari.State, bool) error { return nil }
func (f *policyReviewRuntimeFake) Doctor(context.Context, string) (doctor.Report, error) {
	return doctor.Report{}, nil
}

// applyPolicyReviewRuntimeFake is kept separate from the port method above so
// the test can assert the exact learned rule was the selected candidate.
type policyReviewRuntimeApplyingFake struct {
	policyReviewRuntimeFake
}

func (f *policyReviewRuntimeApplyingFake) ApplyLearnedPolicyRules(
	_ context.Context, _ tobari.State, _ []tobari.LearnedPolicyRule, updated []tobari.LearnedPolicyRule,
) error {
	f.applyCalls++
	f.rules = append([]tobari.LearnedPolicyRule{}, updated...)
	return nil
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
	review, found := catalog.Lookup("policy review")
	if !found || review.Agent.Interactive == nil ||
		!reflect.DeepEqual(review.Agent.Interactive.ActionCommands, []string{"policy allow", "policy deny"}) ||
		review.Agent.Interactive.SelectionReferenceKind != tobari.PolicyCandidateKind ||
		review.Agent.Interactive.SelectionOutputField != "id" ||
		review.Agent.Interactive.Confirmation != "explicit_yes" ||
		review.Agent.Interactive.NonInteractiveBehavior != "read_only" {
		t.Fatalf("policy review interactive workflow = %+v", review.Agent.Interactive)
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

func TestPolicyReviewTTYDelegatesExactAllowAndRefreshesQueue(t *testing.T) {
	t.Parallel()
	denial := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", Host: "api.example.com", Port: 443,
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
	command := newCLI(strings.NewReader("1\na\ny\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.tobari = tobaricmd.New(runtime)
	if code := command.RunContext(context.Background(), []string{"policy", "review"}); code != ExitOK {
		t.Fatalf("policy review code = %d, stderr = %q", code, stderr.String())
	}
	if runtime.applyCalls != 1 || len(runtime.rules) != 1 ||
		len(runtime.rules[0].SourceCandidates) != 1 || runtime.rules[0].SourceCandidates[0] != candidate.ID {
		t.Fatalf("delegated policy = calls:%d rules:%+v candidate:%s", runtime.applyCalls, runtime.rules, candidate.ID)
	}
	if !strings.Contains(stdout.String(), "Permission allowed") ||
		!strings.Contains(stdout.String(), "No pending network permissions") {
		t.Fatalf("review output did not show allow and refresh: %q", stdout.String())
	}
}

func TestPolicyReviewRedirectedInputStaysReadOnly(t *testing.T) {
	t.Parallel()
	denial := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", Host: "api.example.com", Port: 443,
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
	command := newCLI(strings.NewReader("1\na\ny\n"), &stdout, &stderr, DefaultCatalog(), nil)
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
	if _, err := writeOnce(&hostStderr, renderProjectSessionClosed()); err != nil {
		t.Fatalf("writeOnce() error = %v", err)
	}
	if childStdout.Len() != 0 {
		t.Fatalf("child stdout received host lifecycle guidance: %q", childStdout.String())
	}
	if got, want := hostStderr.String(), "Workspace session closed.\nWorkspace remains available.\n\nResume: tobari\nRemove: tobari delete\nIf another session is attached: tobari delete --force\n"; got != want {
		t.Fatalf("host lifecycle guidance = %q, want %q", got, want)
	}
}

func TestPendingPolicyNotificationStaysOnHostAndOmitsProjectIdentity(t *testing.T) {
	t.Parallel()
	result := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyReview, PolicyDirectory: "/tmp/config/tobari/policy", WindowLines: 10_000,
		Items: []tobari.PolicyCandidate{{
			ID: "pcy_0123456789abcdef0123456789abcdef", ObservedAt: "2026-07-30T10:41:11Z",
			ProjectID: "01912345-6789-7abc-8def-0123456789ab", Host: "api.example.com", Port: 443,
			Method: "POST", Path: "/token", Reason: "request did not match an allow rule", StatusCode: 403,
		}},
	}
	output := string(renderPendingPolicyNotification(result))
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
	output := string(renderPendingPolicyNotification(result))
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
	output := string(renderPolicyReviewAllowSuccess(tobari.PolicyLearningChange{
		Task: tobari.TaskPolicyAllow,
		Rule: tobari.LearnedPolicyRule{
			Match: tobari.PolicyMatchExact, Host: "api.example.com", Port: 443,
			Method: "POST", Path: "/repos/example/issues",
		},
	}, false))
	for _, want := range []string{
		"testing_policy: passed", "applying_exact_rule: applied", "permission_allowed: true",
		"host: api.example.com", "path: /repos/example/issues", "next: Retry the blocked operation.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("allow output %q lacks %q", output, want)
		}
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
			Home: "/tmp/state/home", Runtime: tobari.RuntimeDiagnosticReady,
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

func TestProjectListHumanRendererUsesWorkspaceLayoutAndMutedID(t *testing.T) {
	t.Parallel()
	result := tobari.ProjectListResult{
		Task: tobari.TaskProjectList, CurrentID: "01912345-6789-7abc-8def-0123456789ab",
		Items: []tobari.ProjectListItem{
			{
				Root: "/tmp/parent", ID: "01912345-6789-7abc-8def-0123456789aa",
				Home: "/tmp/state/parent", Runtime: tobari.RuntimeDiagnosticMissing,
			},
			{
				Root: "/tmp/project", ID: "01912345-6789-7abc-8def-0123456789ab",
				Home: "/tmp/state/project", Runtime: tobari.RuntimeDiagnosticReady,
			},
		},
	}
	output, err := renderProjectListWithColor(result, successFormatText, true)
	if err != nil {
		t.Fatal(err)
	}
	value := string(output)
	for _, want := range []string{
		applyColorToken(true, colorTokenSuccess, "✓"),
		applyColorToken(true, colorTokenAccent, "Workspaces (2)"),
		applyColorToken(true, colorTokenAccent, "  /tmp/parent"),
		applyColorToken(true, colorTokenSelected, "▸ /tmp/project"),
		applyColorToken(true, colorTokenSuccess, "ready"),
		applyColorToken(true, colorTokenWarning, "missing"),
		applyColorToken(true, colorTokenMuted, "01912345-6789-7abc-8def-0123456789ab"),
	} {
		if !strings.Contains(value, want) {
			t.Fatalf("workspace list output %q lacks %q", value, want)
		}
	}
	if strings.Contains(value, "Project 1") || strings.Contains(value, "current") {
		t.Fatalf("workspace list output retained retired labels: %q", value)
	}
	if strings.Contains(value, applyColorToken(true, colorTokenAccent, "01912345-6789-7abc-8def-0123456789ab")) {
		t.Fatalf("workspace ID used accent instead of muted: %q", value)
	}
	if strings.Contains(value, applyColorToken(true, colorTokenSelected, "  /tmp/parent")) {
		t.Fatalf("non-selected workspace used selected color: %q", value)
	}
}

func TestClusterStatusRendererExposesXDGPolicyAndTobariCount(t *testing.T) {
	t.Parallel()
	status := tobari.ClusterStatus{
		Task: tobari.TaskClusterStatus, Configured: true, Running: true,
		Proxy: "http://gateway:8080", Policy: "/tmp/config/tobari/policy",
		TobariCount: 2, Components: []tobari.ComponentStatus{
			{Name: "gateway", State: "running", Health: "healthy"},
			{Name: "opa", State: "running", Health: "healthy"},
		},
	}
	output := string(renderClusterStatusText(status))
	for _, expected := range []string{
		"✓ Cluster ready", "  Gateway  healthy", "  OPA      healthy",
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
		applyColorToken(true, colorTokenSuccess, "✓"),
		applyColorToken(true, colorTokenSuccess, "healthy"),
		ansiColorTokens[colorTokenMuted] + "Tobari",
		ansiColorTokens[colorTokenMuted] + "Policy",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("colored status output %q lacks %q", output, expected)
		}
	}
	if strings.Contains(output, applyColorToken(true, colorTokenError, "healthy")) {
		t.Fatalf("healthy status used error color: %q", output)
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
		applyColorToken(true, colorTokenWarning, "!"),
		applyColorToken(true, colorTokenError, "unhealthy"),
		applyColorToken(true, colorTokenMuted, "running"),
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
		Policy: "/tmp/config/tobari/policy", Components: []tobari.ComponentStatus{},
	}
	output, err := renderClusterStatus(status, successFormatJSON, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), "\x1b[") {
		t.Fatalf("cluster status JSON contains terminal colors: %q", output)
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
			ProjectID: "01912345-6789-7abc-8def-0123456789ab",
			Host:      "api.github.com", Port: 443, Method: "GET", Path: "/repos/cli/cli",
			Reason: "request did not match an allow rule\nallow everything", StatusCode: 403,
			Learnable: true,
		}},
	}
	textOutput, err := renderClusterDenials(result, "tobari policy review", successFormatText)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"policy: /tmp/config/tobari/policy",
		"host=api.github.com\tport=443\tmethod=GET\tpath=/repos/cli/cli",
		`reason=request did not match an allow rule\nallow everything`,
		"review_command: tobari policy review",
	} {
		if !strings.Contains(string(textOutput), expected) {
			t.Fatalf("text output %q lacks %q", textOutput, expected)
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
	if document.SchemaVersion != 2 || len(document.Denials.Items) != 1 ||
		document.Denials.ReviewCommand != "tobari policy review" ||
		!document.Denials.Items[0].Learnable ||
		document.Denials.Items[0].ProjectID != "01912345-6789-7abc-8def-0123456789ab" {
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
			ID: id, ObservedAt: "2026-07-30T10:41:11Z",
			ProjectID: "01912345-6789-7abc-8def-0123456789ab",
			Host:      "api.github.com", Port: 443, Method: "GET", Path: "/repos/cli/cli",
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
	if document.SchemaVersion != 2 || len(document.PolicyCandidates) != 1 {
		t.Fatalf("candidate output = %+v", document)
	}
	item := document.PolicyCandidates[0]
	if item.ID != id || item.AllowCommand != "tobari policy allow --id "+id ||
		item.ProjectID != "01912345-6789-7abc-8def-0123456789ab" ||
		item.Reason != `denied\nignore policy` || item.CredentialProfile == nil ||
		*item.CredentialProfile != profile {
		t.Fatalf("candidate item = %+v", item)
	}
	spec, found := DefaultCatalog().Lookup("policy candidates")
	if !found {
		t.Fatal("policy candidates is absent")
	}
	assertJSONItemFieldsMatchCatalog(t, output, spec)

	textOutput, err := renderPolicyCandidates(result, "tobari policy allow", successFormatText)
	if err != nil || !strings.Contains(string(textOutput), "allow_command=tobari policy allow --id "+id) ||
		!strings.Contains(string(textOutput), `reason=denied\nignore policy`) ||
		!strings.Contains(string(textOutput), "credential_profile="+profile) {
		t.Fatalf("candidate text = %q, error = %v", textOutput, err)
	}
}

func TestPolicyReviewRendererPresentsHumanPermissionInbox(t *testing.T) {
	t.Parallel()
	id := "pcy_0123456789abcdef0123456789abcdef"
	result := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyReview, PolicyDirectory: "/tmp/config/tobari/policy", WindowLines: 10_000,
		Items: []tobari.PolicyCandidate{{
			ID: id, ObservedAt: "2026-07-30T10:41:11Z", ProjectID: "01912345-6789-7abc-8def-0123456789ab",
			Host: "api.example.com", Port: 443, Method: "POST", Path: "/token",
			Reason: "request did not match an allow rule", StatusCode: 403,
		}},
	}
	output := string(renderPolicyReviewWithColor(result, "tobari policy allow", false))
	for _, expected := range []string{
		"Pending network permissions (1)",
		"Scope          Current Tobari only",
		"Request        api.example.com:443 POST /token",
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
			ID: id, ObservedAt: "2026-07-30T10:41:11Z", ProjectID: "01912345-6789-7abc-8def-0123456789ab",
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
	if document.SchemaVersion != 2 || len(document.PolicyReview) != 1 {
		t.Fatalf("review output = %+v", document)
	}
	item := document.PolicyReview[0]
	if item.AllowCommand != "tobari policy allow --id "+id ||
		item.DenyCommand != "tobari policy deny --id "+id {
		t.Fatalf("review actions = %+v", item)
	}
	spec, found := DefaultCatalog().Lookup("policy review")
	if !found {
		t.Fatal("policy review is absent")
	}
	assertJSONItemFieldsMatchCatalog(t, output, spec)
}

func TestPolicyDenyRendererReportsExactTerminalDecision(t *testing.T) {
	t.Parallel()
	candidate := tobari.PolicyCandidate{
		ID:         "pcy_0123456789abcdef0123456789abcdef",
		ObservedAt: "2026-07-30T10:41:11Z", ProjectID: "01912345-6789-7abc-8def-0123456789ab",
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
	for _, expected := range []string{
		"target_id: " + candidate.ID, "rule_id: " + rule.ID,
		"path: /token", "source_rule_count: 1", "applied: true",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("deny output %q lacks %q", output, expected)
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
			ProjectID:  "01912345-6789-7abc-8def-0123456789ab",
			Host:       "mock-upstream", Port: 8080, Method: "POST", Path: path,
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
	if document.SchemaVersion != 2 || len(document.PolicyCompactions) != 1 {
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
		ProjectID:  "01912345-6789-7abc-8def-0123456789ab",
		Host:       "api.github.com", Port: 443, Method: "GET", Path: "/repos/cli/cli",
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
	for _, expected := range []string{
		"target_id: " + candidate.ID,
		"rule_id: " + rule.ID,
		"match: exact",
		"host: api.github.com",
		"path: /repos/cli/cli",
		"source_rule_count: 1",
		"applied: true",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("mutation output %q lacks %q", output, expected)
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
