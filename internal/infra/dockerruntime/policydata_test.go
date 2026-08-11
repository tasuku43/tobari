package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func learnedRuleFixture(t *testing.T, path string) tobari.LearnedPolicyRule {
	t.Helper()
	candidate, err := tobari.NewPolicyCandidate(tobari.PolicyDenial{
		Timestamp:   "2026-07-30T10:41:11Z",
		RequestID:   "7185da2688d7469aae9cd9068e920b0b",
		ContextID:   "01912345-6789-7abc-8def-0123456789ad",
		ContextName: "default",
		ProjectID:   "01912345-6789-7abc-8def-0123456789ab",
		ProjectRoot: "/workspace/project",
		Host:        "api.github.com",
		Port:        443,
		Method:      "GET",
		Path:        path,
		Reason:      "request did not match an allow rule",
		StatusCode:  403,
		Learnable:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func deniedRuleFixture(t *testing.T, path string) tobari.PolicyDenyRule {
	t.Helper()
	candidate, err := tobari.NewPolicyCandidate(tobari.PolicyDenial{
		Timestamp:   "2026-07-30T10:41:11Z",
		RequestID:   "8185da2688d7469aae9cd9068e920b0b",
		ContextID:   "01912345-6789-7abc-8def-0123456789ad",
		ContextName: "default",
		ProjectID:   "01912345-6789-7abc-8def-0123456789ab",
		ProjectRoot: "/workspace/project",
		Host:        "api.github.com",
		Port:        443,
		Method:      "GET",
		Path:        path,
		Reason:      "request did not match an allow rule",
		StatusCode:  403,
		Learnable:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewExactPolicyDenyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func writePolicyFixture(t *testing.T, state tobari.State, data string) {
	t.Helper()
	if err := os.MkdirAll(state.PolicyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(state.PolicyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.PolicyDirectory, "data.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(state.PolicyDirectory, "tobari.rego"),
		[]byte("package tobari.test\n\nimport rego.v1\n\ntest_policy_data if { true }\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
}

const minimalPolicyDataFixture = `{"tobari":{"schema_version":2,"boundary":{"ports":{"https":[443],"http":[8080]},"authorities":[],"methods":{"read":["GET"],"write":[]}},"credentials":{},"rules":{"baseline_denies":[],"learned_allows":[],"learned_denies":[]}}}
`

func TestPolicyDataValidatesDeclaredGraphQLEndpoints(t *testing.T) {
	t.Parallel()
	validEndpoint := `{"scheme":"https","host":"api.example.com","port":443,"path":"/graphql"}`
	tests := []struct {
		name      string
		endpoints string
		wantError bool
	}{
		{name: "absent remains legacy HTTP", endpoints: ""},
		{name: "empty declaration", endpoints: `,"graphql_endpoints":[]`},
		{name: "exact endpoint", endpoints: `,"graphql_endpoints":[` + validEndpoint + `]`},
		{name: "duplicate", endpoints: `,"graphql_endpoints":[` + validEndpoint + `,` + validEndpoint + `]`, wantError: true},
		{name: "unnormalized host", endpoints: `,"graphql_endpoints":[{"scheme":"https","host":"API.example.com","port":443,"path":"/graphql"}]`, wantError: true},
		{name: "query in path", endpoints: `,"graphql_endpoints":[{"scheme":"https","host":"api.example.com","port":443,"path":"/graphql?x=1"}]`, wantError: true},
		{name: "null", endpoints: `,"graphql_endpoints":null`, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := tobari.State{PolicyDirectory: filepath.Join(t.TempDir(), "policy")}
			fixture := `{"tobari":{"schema_version":2,"boundary":{"ports":{"https":[443],"http":[8080]},"authorities":[]` + test.endpoints + `,"methods":{"read":["GET"],"write":[]}},"credentials":{},"rules":{"baseline_denies":[],"learned_allows":[],"learned_denies":[]}}}`
			writePolicyFixture(t, state, fixture)
			file, err := readPolicyData(state.PolicyDirectory)
			if test.wantError {
				if err == nil {
					t.Fatal("invalid GraphQL endpoint declaration was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if test.endpoints != "" && strings.Contains(test.endpoints, validEndpoint) && len(file.graphqlEndpoints) != 1 {
				t.Fatalf("GraphQL endpoints = %#v", file.graphqlEndpoints)
			}
		})
	}
}

type concurrentPolicyRunner struct{}

func (concurrentPolicyRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (concurrentPolicyRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) > 0 && (args[0] == "inspect" || (args[0] == "volume" && len(args) > 1 && args[1] == "inspect")) {
		return []byte(ownerValue + "\n"), nil
	}
	return nil, nil
}

func contextRuleFixture(t *testing.T, manifest tobari.ContextManifest, projectID, path string) tobari.LearnedPolicyRule {
	t.Helper()
	candidate, err := tobari.NewPolicyCandidate(tobari.PolicyDenial{
		Timestamp: "2026-08-08T08:00:00Z", RequestID: strings.Repeat("a", 32),
		ContextID: manifest.ID, ContextName: manifest.Name,
		ProjectID: projectID, ProjectRoot: "/workspace/project",
		Host: "api.example.com", Port: 443, Method: "POST", Path: path,
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func contextDenyFixture(t *testing.T, manifest tobari.ContextManifest, projectID, path string) tobari.PolicyDenyRule {
	t.Helper()
	candidate, err := tobari.NewPolicyCandidate(tobari.PolicyDenial{
		Timestamp: "2026-08-08T08:00:00Z", RequestID: strings.Repeat("b", 32),
		ContextID: manifest.ID, ContextName: manifest.Name,
		ProjectID: projectID, ProjectRoot: "/workspace/project",
		Host: "api.example.com", Port: 443, Method: "POST", Path: path,
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewExactPolicyDenyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func TestConcurrentCrossContextPolicyMutationsNeverLoseAnUpdate(t *testing.T) {
	root := t.TempDir()
	runtimeStore, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), concurrentPolicyRunner{})
	if _, err := runtimeStore.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeStore.CreateContext(context.Background(), "restricted", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided); err != nil {
		t.Fatal(err)
	}
	defaultContext, _, err := runtimeStore.resolveContext("default")
	if err != nil {
		t.Fatal(err)
	}
	restrictedContext, _, err := runtimeStore.resolveContext("restricted")
	if err != nil {
		t.Fatal(err)
	}
	projection, err := runtimeStore.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	state.AggregateRevision = projection.Revision
	state.ContextCount = projection.ContextCount
	state.PolicyDirectory = projection.PolicyDirectory
	state.CredentialConfig = projection.CredentialConfig
	state.CredentialDir = projection.CredentialDirectory
	if err := runtimeStore.writeState(state); err != nil {
		t.Fatal(err)
	}
	rules := []tobari.LearnedPolicyRule{
		contextRuleFixture(t, defaultContext, "01912345-6789-7abc-8def-0123456789ab", "/default"),
		contextRuleFixture(t, restrictedContext, "01912345-6789-7abc-8def-0123456789ac", "/restricted"),
	}
	type result struct {
		index int
		err   error
	}
	results := make(chan result, len(rules))
	var wait sync.WaitGroup
	for index, rule := range rules {
		wait.Add(1)
		go func() {
			defer wait.Done()
			results <- result{index: index, err: runtimeStore.ApplyLearnedPolicyRules(context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{rule})}
		}()
	}
	wait.Wait()
	close(results)
	winner, loser, successes := -1, -1, 0
	for result := range results {
		if result.err == nil {
			winner = result.index
			successes++
			continue
		}
		public, ok := fault.PublicCopy(result.err)
		if !ok || public.Code != "policy_data_changed" {
			t.Fatalf("concurrent mutation %d error = %v", result.index, result.err)
		}
		loser = result.index
	}
	if successes != 1 || winner < 0 || loser < 0 {
		t.Fatalf("concurrent results successes=%d winner=%d loser=%d", successes, winner, loser)
	}
	stored, exists, err := runtimeStore.LoadState(context.Background())
	if err != nil || !exists {
		t.Fatalf("LoadState() = %+v, exists=%t, error=%v", stored, exists, err)
	}
	current, err := runtimeStore.ReadLearnedPolicyRules(context.Background(), stored)
	if err != nil || len(current) != 1 || current[0].ID != rules[winner].ID {
		t.Fatalf("first committed rules = %+v, error=%v", current, err)
	}
	updated := append(append([]tobari.LearnedPolicyRule{}, current...), rules[loser])
	if err := runtimeStore.ApplyLearnedPolicyRules(context.Background(), stored, current, updated); err != nil {
		t.Fatalf("retry after rediscovery failed: %v", err)
	}
	latest, exists, err := runtimeStore.LoadState(context.Background())
	if err != nil || !exists {
		t.Fatalf("LoadState() after retry = %+v, exists=%t, error=%v", latest, exists, err)
	}
	committed, err := runtimeStore.ReadLearnedPolicyRules(context.Background(), latest)
	if err != nil || len(committed) != 2 {
		t.Fatalf("committed cross-Context rules = %+v, error=%v", committed, err)
	}

	defaultDeny := contextDenyFixture(t, defaultContext, "01912345-6789-7abc-8def-0123456789ab", "/default-deny")
	var restrictedDeny tobari.PolicyDenyRule
	for index := 0; index < 128; index++ {
		restrictedDeny = contextDenyFixture(
			t, restrictedContext, "01912345-6789-7abc-8def-0123456789ac", fmt.Sprintf("/restricted-deny-%d", index),
		)
		if restrictedDeny.ID < defaultDeny.ID {
			break
		}
	}
	if restrictedDeny.ID >= defaultDeny.ID {
		t.Fatal("could not construct reverse Context-order deny IDs")
	}
	if err := runtimeStore.ApplyPolicyDenyRules(
		context.Background(), latest, committed, []tobari.PolicyDenyRule{}, []tobari.PolicyDenyRule{defaultDeny},
	); err != nil {
		t.Fatalf("apply default deny: %v", err)
	}
	latest, _, err = runtimeStore.LoadState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	denies, err := runtimeStore.ReadPolicyDenyRules(context.Background(), latest)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.ApplyPolicyDenyRules(
		context.Background(), latest, committed, denies.Exact, append(append([]tobari.PolicyDenyRule{}, denies.Exact...), restrictedDeny),
	); err != nil {
		t.Fatalf("apply restricted deny: %v", err)
	}
	latest, _, err = runtimeStore.LoadState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	denies, err = runtimeStore.ReadPolicyDenyRules(context.Background(), latest)
	if err != nil || len(denies.Exact) != 2 || denies.Exact[0].ID != restrictedDeny.ID || denies.Exact[1].ID != defaultDeny.ID {
		t.Fatalf("aggregate deny order = %+v, error=%v", denies.Exact, err)
	}
	if err := runtimeStore.ApplyPolicyDenyRules(
		context.Background(), latest, committed, denies.Exact, denies.Exact[1:],
	); err != nil {
		t.Fatalf("reset after deterministic cross-Context discovery: %v", err)
	}
}

func TestApplyPolicyDecisionSetRejectsMultipleContextSourcesBeforeDocker(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtimeStore, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), concurrentPolicyRunner{})
	if _, err := runtimeStore.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeStore.CreateContext(context.Background(), "restricted", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided); err != nil {
		t.Fatal(err)
	}
	defaultContext, _, err := runtimeStore.resolveContext("default")
	if err != nil {
		t.Fatal(err)
	}
	restrictedContext, _, err := runtimeStore.resolveContext("restricted")
	if err != nil {
		t.Fatal(err)
	}
	updated := []tobari.LearnedPolicyRule{
		contextRuleFixture(t, defaultContext, "01912345-6789-7abc-8def-0123456789ab", "/default-reviewed"),
		contextRuleFixture(t, restrictedContext, "01912345-6789-7abc-8def-0123456789ac", "/restricted-reviewed"),
	}
	_, err = runtimeStore.ApplyPolicyDecisionSet(
		context.Background(), runtimeState(root),
		[]tobari.LearnedPolicyRule{}, updated,
		[]tobari.PolicyDenyRule{}, []tobari.PolicyDenyRule{},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "policy_review_scope_mixed" {
		t.Fatalf("mixed-Context runtime decision set error = %v", err)
	}
}

func TestApplyPolicyDecisionSetReturnsTheActivatedAggregateRevision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtimeStore, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), concurrentPolicyRunner{})
	if _, err := runtimeStore.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := runtimeStore.resolveContext("default")
	if err != nil {
		t.Fatal(err)
	}
	projection, err := runtimeStore.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	state.AggregateRevision = projection.Revision
	state.ContextCount = projection.ContextCount
	state.PolicyDirectory = projection.PolicyDirectory
	state.CredentialConfig = projection.CredentialConfig
	state.CredentialDir = projection.CredentialDirectory
	if err := runtimeStore.writeState(state); err != nil {
		t.Fatal(err)
	}
	rule := contextRuleFixture(t, manifest, "01912345-6789-7abc-8def-0123456789ab", "/reviewed")
	activeRevision, err := runtimeStore.ApplyPolicyDecisionSet(
		context.Background(), state,
		[]tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{rule},
		[]tobari.PolicyDenyRule{}, []tobari.PolicyDenyRule{},
	)
	if err != nil {
		t.Fatal(err)
	}
	stored, configured, err := runtimeStore.LoadState(context.Background())
	if err != nil || !configured {
		t.Fatalf("load activated state: configured=%v err=%v", configured, err)
	}
	freshProjection, err := runtimeStore.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if activeRevision == "" || activeRevision == state.AggregateRevision ||
		activeRevision != stored.AggregateRevision || activeRevision != freshProjection.Revision {
		t.Fatalf(
			"returned=%q original=%q stored=%q projection=%q",
			activeRevision, state.AggregateRevision, stored.AggregateRevision, freshProjection.Revision,
		)
	}
}

func TestApplyLearnedPolicyRulesPreservesHostDataAndActivatesTestedCopy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writePolicyFixture(t, state, `{
  "host_owned": {"keep": true},
  "tobari": {
    "schema_version": 2,
    "boundary": {
      "ports": {"https": [443], "http": [8080]},
      "authorities": [],
      "methods": {"read": ["GET"], "write": []}
    },
    "credentials": {},
    "rules": {
      "baseline_denies": [],
      "learned_allows": [],
      "learned_denies": []
    },
    "host_extension": {"keep": true}
  }
}
`)
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	rule := learnedRuleFixture(t, "/repos/cli/cli")

	if err := runtime.ApplyLearnedPolicyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{rule},
	); err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 7 {
		t.Fatalf("Docker calls = %v", runner.outputs)
	}
	if got := runner.outputs[0].args; len(got) < 2 || got[0] != "run" {
		t.Fatalf("first call did not preflight a private policy copy: %v", got)
	}
	if got := runner.outputs[1].args; len(got) < 2 || got[0] != "run" {
		t.Fatalf("second call did not retest the activated host policy: %v", got)
	}
	data, err := os.ReadFile(filepath.Join(state.PolicyDirectory, "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if _, exists := document["host_owned"]; !exists {
		t.Fatalf("host-owned top-level member was removed: %s", data)
	}
	var tobariData map[string]json.RawMessage
	if err := json.Unmarshal(document["tobari"], &tobariData); err != nil {
		t.Fatal(err)
	}
	if _, exists := tobariData["host_extension"]; !exists {
		t.Fatalf("host-owned tobari member was removed: %s", data)
	}
	var ruleData map[string]json.RawMessage
	if err := json.Unmarshal(tobariData[policyRulesDataName], &ruleData); err != nil {
		t.Fatal(err)
	}
	var rules []tobari.LearnedPolicyRule
	if err := json.Unmarshal(ruleData[learnedPolicyDataName], &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].ID != rule.ID {
		t.Fatalf("learned rules = %+v", rules)
	}
	info, err := os.Stat(filepath.Join(state.PolicyDirectory, "data.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("data.json mode = %v, error = %v", info.Mode().Perm(), err)
	}
	if err := runtime.ApplyLearnedPolicyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{rule}, []tobari.LearnedPolicyRule{},
	); err != nil {
		t.Fatal(err)
	}
	read, err := runtime.ReadLearnedPolicyRules(context.Background(), state)
	if err != nil || len(read) != 0 || len(runner.outputs) != 20 {
		t.Fatalf("removed learned rule = %+v, error = %v, Docker calls = %v", read, err, runner.outputs)
	}
}

func TestApplyLearnedPolicyRulesRejectsChangedDataBeforeDockerOrWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writePolicyFixture(t, state, minimalPolicyDataFixture)
	before, err := os.ReadFile(filepath.Join(state.PolicyDirectory, "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	stale := learnedRuleFixture(t, "/repos/cli/stale")

	err = runtime.ApplyLearnedPolicyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{stale}, []tobari.LearnedPolicyRule{},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "policy_data_changed" {
		t.Fatalf("error = %v", err)
	}
	after, readErr := os.ReadFile(filepath.Join(state.PolicyDirectory, "data.json"))
	if readErr != nil || string(after) != string(before) || len(runner.outputs) != 0 {
		t.Fatalf("rejected update changed state: read=%v calls=%v data=%s", readErr, runner.outputs, after)
	}
}

func TestApplyPolicyDenyRulesPreservesAllowsAndActivatesExactDeny(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writePolicyFixture(t, state, `{
  "tobari": {
    "schema_version": 2,
    "boundary": {
      "ports": {"https": [443], "http": [8080]},
      "authorities": [],
      "methods": {"read": ["GET"], "write": []}
    },
    "credentials": {},
    "rules": {
      "baseline_denies": [],
      "learned_allows": [],
      "learned_denies": []
    }
  }
}
`)
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	rule := deniedRuleFixture(t, "/user/settings")

	if err := runtime.ApplyPolicyDenyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{},
		[]tobari.PolicyDenyRule{}, []tobari.PolicyDenyRule{rule},
	); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(state.PolicyDirectory, "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	var tobariData map[string]json.RawMessage
	if err := json.Unmarshal(document["tobari"], &tobariData); err != nil {
		t.Fatal(err)
	}
	var ruleData map[string]json.RawMessage
	if err := json.Unmarshal(tobariData[policyRulesDataName], &ruleData); err != nil {
		t.Fatal(err)
	}
	var got []tobari.PolicyDenyRule
	if err := json.Unmarshal(ruleData[learnedDenyDataName], &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != rule.ID {
		t.Fatalf("learned deny rules = %+v", got)
	}
	read, err := runtime.ReadPolicyDenyRules(context.Background(), state)
	if err != nil || len(read.Exact) != 1 || read.Exact[0].ID != rule.ID {
		t.Fatalf("read deny rules = %+v, error = %v", read, err)
	}
	if err := runtime.ApplyPolicyDenyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.PolicyDenyRule{rule}, []tobari.PolicyDenyRule{},
	); err != nil {
		t.Fatal(err)
	}
	read, err = runtime.ReadPolicyDenyRules(context.Background(), state)
	if err != nil || len(read.Exact) != 0 || len(runner.outputs) != 20 {
		t.Fatalf("removed deny rule = %+v, error = %v, Docker calls = %v", read, err, runner.outputs)
	}
}

func TestApplyPolicyDenyRulesRejectsChangedDenySnapshotBeforeDockerOrWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writePolicyFixture(t, state, minimalPolicyDataFixture)
	before, err := os.ReadFile(filepath.Join(state.PolicyDirectory, "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	rule := deniedRuleFixture(t, "/user/settings")

	err = runtime.ApplyPolicyDenyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{},
		[]tobari.PolicyDenyRule{rule}, []tobari.PolicyDenyRule{},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "policy_data_changed" {
		t.Fatalf("error = %v", err)
	}
	after, readErr := os.ReadFile(filepath.Join(state.PolicyDirectory, "data.json"))
	if readErr != nil || string(after) != string(before) || len(runner.outputs) != 0 {
		t.Fatalf("rejected deny update changed state: read=%v calls=%v data=%s", readErr, runner.outputs, after)
	}
}

func TestApplyLearnedPolicyRulesRejectsFailedPreflightBeforeWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writePolicyFixture(t, state, minimalPolicyDataFixture)
	before, err := os.ReadFile(filepath.Join(state.PolicyDirectory, "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{outputData: []byte("FAIL"), outputErr: errors.New("exit 2")}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	rule := learnedRuleFixture(t, "/repos/cli/cli")

	err = runtime.ApplyLearnedPolicyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{rule},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "policy_preflight_failed" {
		t.Fatalf("error = %v", err)
	}
	after, readErr := os.ReadFile(filepath.Join(state.PolicyDirectory, "data.json"))
	if readErr != nil || string(after) != string(before) || len(runner.outputs) != 1 {
		t.Fatalf("failed preflight changed state: read=%v calls=%v data=%s", readErr, runner.outputs, after)
	}
}

func TestApplyLearnedPolicyRulesRejectsHostEditDuringPreflight(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	dataPath := filepath.Join(state.PolicyDirectory, "data.json")
	writePolicyFixture(t, state, `{"host_owned":{"revision":1},"tobari":{"schema_version":2,"boundary":{"ports":{"https":[443],"http":[8080]},"authorities":[],"methods":{"read":["GET"],"write":[]}},"credentials":{},"rules":{"baseline_denies":[],"learned_allows":[],"learned_denies":[]}}}`+"\n")
	runner := &recordingRunner{}
	runner.onOutput = func(call int) {
		if call != 1 {
			return
		}
		if err := os.WriteFile(
			dataPath,
			[]byte(`{"host_owned":{"revision":2},"tobari":{"schema_version":2,"boundary":{"ports":{"https":[443],"http":[8080]},"authorities":[],"methods":{"read":["GET"],"write":[]}},"credentials":{},"rules":{"baseline_denies":[],"learned_allows":[],"learned_denies":[]}}}`+"\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	rule := learnedRuleFixture(t, "/repos/cli/cli")

	err := runtime.ApplyLearnedPolicyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{rule},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "policy_data_changed" {
		t.Fatalf("error = %v", err)
	}
	data, readErr := os.ReadFile(dataPath)
	if readErr != nil || !bytes.Contains(data, []byte(`"revision":2`)) || len(runner.outputs) != 1 {
		t.Fatalf("concurrent edit was overwritten: read=%v calls=%v data=%s", readErr, runner.outputs, data)
	}
}

func TestManagedPolicyDataRejectsAmbiguousOrUnsafeHostFiles(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*testing.T, tobari.State){
		"legacy flat shape": func(t *testing.T, state tobari.State) {
			writePolicyFixture(t, state, `{"tobari":{"allowed_hosts":["api.github.com"],"learned_allow_rules":[]}}`)
		},
		"duplicate key": func(t *testing.T, state tobari.State) {
			writePolicyFixture(t, state, `{"tobari":{"schema_version":2,"boundary":{"ports":{"https":[443],"http":[8080]},"authorities":[],"methods":{"read":["GET"],"write":[]}},"credentials":{},"rules":{"baseline_denies":[],"learned_allows":[],"learned_allows":[],"learned_denies":[]}}}`)
		},
		"data symlink": func(t *testing.T, state tobari.State) {
			writePolicyFixture(t, state, minimalPolicyDataFixture)
			target := filepath.Join(filepath.Dir(state.PolicyDirectory), "outside.json")
			if err := os.WriteFile(target, []byte(minimalPolicyDataFixture), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(state.PolicyDirectory, "data.json")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(state.PolicyDirectory, "data.json")); err != nil {
				t.Fatal(err)
			}
		},
		"unsafe child mode": func(t *testing.T, state tobari.State) {
			writePolicyFixture(t, state, minimalPolicyDataFixture)
			if err := os.Chmod(filepath.Join(state.PolicyDirectory, "tobari.rego"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"unsafe directory mode": func(t *testing.T, state tobari.State) {
			writePolicyFixture(t, state, minimalPolicyDataFixture)
			if err := os.Chmod(state.PolicyDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, prepare := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			state := runtimeState(root)
			prepare(t, state)
			runner := &recordingRunner{}
			runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			rule := learnedRuleFixture(t, "/repos/cli/cli")
			err := runtime.ApplyLearnedPolicyRules(
				context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{rule},
			)
			if err == nil || len(runner.outputs) != 0 {
				t.Fatalf("unsafe policy was accepted: error=%v calls=%v", err, runner.outputs)
			}
		})
	}
}
