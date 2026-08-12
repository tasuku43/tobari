package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func learnedRuleFixture(t *testing.T, path string) tobari.LearnedPolicyRule {
	return learnedRuleFixtureForHost(t, "api.github.com", path)
}

func learnedRuleFixtureForHost(t *testing.T, host, path string) tobari.LearnedPolicyRule {
	t.Helper()
	candidate, err := tobari.NewPolicyCandidate(tobari.PolicyDenial{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP}, Timestamp: "2026-07-30T10:41:11Z",
		RequestID:   "7185da2688d7469aae9cd9068e920b0b",
		ContextID:   "01912345-6789-7abc-8def-0123456789ad",
		ContextName: "default",
		ProjectID:   "01912345-6789-7abc-8def-0123456789ab",
		ProjectRoot: "/workspace/project",
		Host:        host,
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
	candidate, err := tobari.NewPolicyCandidate(tobari.PolicyDenial{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP}, Timestamp: "2026-07-30T10:41:11Z",
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

func writePolicyDomainFixture(t *testing.T, state tobari.State, domain, allow, deny string) {
	t.Helper()
	directory := filepath.Join(state.PolicyDirectory, policyDomainsName, domain)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{state.PolicyDirectory, filepath.Join(state.PolicyDirectory, policyDomainsName), directory} {
		if err := os.Chmod(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, policyAllowFileName), []byte(allow), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, policyDenyFileName), []byte(deny), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"tobari.rego":      "package tobari.http\n\nimport rego.v1\n\ndecision := {\"allow\": false} if { input.schema_version == 1 }\n",
		"tobari_test.rego": "package tobari.http\n\nimport rego.v1\n\ntest_policy_data if { true }\n",
	} {
		if err := os.WriteFile(filepath.Join(state.PolicyDirectory, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

const minimalPolicyAllowFixture = `{"schema_version":1,"host":"api.github.com","graphql_endpoints":[],"rules":[]}
`

const minimalPolicyDenyFixture = `{"schema_version":1,"host":"api.github.com","rules":[]}
`

func writeMinimalPolicyFixture(t *testing.T, state tobari.State) {
	t.Helper()
	writePolicyDomainFixture(t, state, "api.github.com", minimalPolicyAllowFixture, minimalPolicyDenyFixture)
}

func TestPolicyDataValidatesDeclaredGraphQLEndpoints(t *testing.T) {
	t.Parallel()
	validEndpoint := `{"scheme":"https","host":"api.example.com","port":443,"path":"/graphql"}`
	tests := []struct {
		name      string
		endpoints string
		wantError bool
	}{
		{name: "empty declaration", endpoints: `"graphql_endpoints":[]`},
		{name: "exact endpoint", endpoints: `"graphql_endpoints":[` + validEndpoint + `]`},
		{name: "duplicate", endpoints: `"graphql_endpoints":[` + validEndpoint + `,` + validEndpoint + `]`, wantError: true},
		{name: "unnormalized host", endpoints: `"graphql_endpoints":[{"scheme":"https","host":"API.example.com","port":443,"path":"/graphql"}]`, wantError: true},
		{name: "query in path", endpoints: `"graphql_endpoints":[{"scheme":"https","host":"api.example.com","port":443,"path":"/graphql?x=1"}]`, wantError: true},
		{name: "missing", endpoints: "", wantError: true},
		{name: "null", endpoints: `"graphql_endpoints":null`, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := tobari.State{PolicyDirectory: filepath.Join(t.TempDir(), "policy")}
			allow := `{"schema_version":1,"host":"api.example.com",` + test.endpoints + `,"rules":[]}`
			writePolicyDomainFixture(t, state, "api.example.com", allow, `{"schema_version":1,"host":"api.example.com","rules":[]}`)
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

func opaPolicyTestCallCount(calls []runnerCall) int {
	count := 0
	for _, call := range calls {
		if len(call.args) >= 2 && call.args[len(call.args)-2] == "test" && call.args[len(call.args)-1] == "/policy" {
			count++
		}
	}
	return count
}

func contextRuleFixture(t *testing.T, manifest tobari.ContextManifest, projectID, path string) tobari.LearnedPolicyRule {
	t.Helper()
	candidate, err := tobari.NewPolicyCandidate(tobari.PolicyDenial{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP}, Timestamp: "2026-08-08T08:00:00Z", RequestID: strings.Repeat("a", 32),
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
	candidate, err := tobari.NewPolicyCandidate(tobari.PolicyDenial{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP}, Timestamp: "2026-08-08T08:00:00Z", RequestID: strings.Repeat("b", 32),
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
	runtimePeer, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), concurrentPolicyRunner{})
	runtimes := []*Runtime{runtimeStore, runtimePeer}
	if _, err := runtimeStore.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeStore.CreateContext(context.Background(), "restricted", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite); err != nil {
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
	state.GatewayConfig = projection.GatewayConfig
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
			_, err := runtimes[index].ApplyLearnedPolicyRules(
				context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{rule},
			)
			results <- result{index: index, err: err}
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
	if _, err := runtimeStore.ApplyLearnedPolicyRules(context.Background(), stored, current, updated); err != nil {
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

	var defaultDeny, restrictedDeny tobari.PolicyDenyRule
	foundReverseOrder := false
	for defaultIndex := 0; defaultIndex < 64 && !foundReverseOrder; defaultIndex++ {
		defaultDeny = contextDenyFixture(
			t, defaultContext, "01912345-6789-7abc-8def-0123456789ab", fmt.Sprintf("/default-deny-%d", defaultIndex),
		)
		for restrictedIndex := 0; restrictedIndex < 64; restrictedIndex++ {
			restrictedDeny = contextDenyFixture(
				t, restrictedContext, "01912345-6789-7abc-8def-0123456789ac", fmt.Sprintf("/restricted-deny-%d", restrictedIndex),
			)
			if restrictedDeny.ID < defaultDeny.ID {
				foundReverseOrder = true
				break
			}
		}
	}
	if restrictedDeny.ID >= defaultDeny.ID {
		t.Fatal("could not construct reverse Context-order deny IDs")
	}
	if _, err := runtimeStore.ApplyPolicyDenyRules(
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
	if _, err := runtimeStore.ApplyPolicyDenyRules(
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
	if _, err := runtimeStore.ApplyPolicyDenyRules(
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
	if _, err := runtimeStore.CreateContext(context.Background(), "restricted", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite); err != nil {
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

func TestApplyPolicyDecisionSetReturnsTheActivatedAggregateProjection(t *testing.T) {
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
	state.GatewayConfig = projection.GatewayConfig
	if err := runtimeStore.writeState(state); err != nil {
		t.Fatal(err)
	}
	rule := contextRuleFixture(t, manifest, "01912345-6789-7abc-8def-0123456789ab", "/reviewed")
	receipt, err := runtimeStore.ApplyPolicyDecisionSet(
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
	if err := receipt.Validate(); err != nil {
		t.Fatalf("activation receipt: %v", err)
	}
	if receipt.ActiveRevision == state.AggregateRevision ||
		receipt.ActiveRevision != stored.AggregateRevision || receipt.ActiveRevision != freshProjection.Revision ||
		receipt.PolicyDirectory == state.PolicyDirectory || receipt.PolicyDirectory != stored.PolicyDirectory ||
		receipt.PolicyDirectory != freshProjection.PolicyDirectory {
		t.Fatalf(
			"receipt=%+v original=%+v stored=%+v projection=%+v",
			receipt, state, stored, freshProjection,
		)
	}
}

func TestSinglePolicyMutationsReturnTheActivatedAggregateProjection(t *testing.T) {
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
	state.GatewayConfig = projection.GatewayConfig
	if err := runtimeStore.writeState(state); err != nil {
		t.Fatal(err)
	}

	allow := contextRuleFixture(t, manifest, "01912345-6789-7abc-8def-0123456789ab", "/single-allow")
	allowReceipt, err := runtimeStore.ApplyLearnedPolicyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{allow},
	)
	if err != nil {
		t.Fatal(err)
	}
	stored, configured, err := runtimeStore.LoadState(context.Background())
	if err != nil || !configured {
		t.Fatalf("load allow state: configured=%v err=%v", configured, err)
	}
	if err := allowReceipt.Validate(); err != nil || allowReceipt.PolicyDirectory != stored.PolicyDirectory ||
		allowReceipt.ActiveRevision != stored.AggregateRevision || allowReceipt.PolicyDirectory == state.PolicyDirectory {
		t.Fatalf("allow receipt=%+v stored=%+v validate=%v", allowReceipt, stored, err)
	}

	deny := contextDenyFixture(t, manifest, "01912345-6789-7abc-8def-0123456789ab", "/single-deny")
	denyReceipt, err := runtimeStore.ApplyPolicyDenyRules(
		context.Background(), stored, []tobari.LearnedPolicyRule{allow},
		[]tobari.PolicyDenyRule{}, []tobari.PolicyDenyRule{deny},
	)
	if err != nil {
		t.Fatal(err)
	}
	latest, configured, err := runtimeStore.LoadState(context.Background())
	if err != nil || !configured {
		t.Fatalf("load deny state: configured=%v err=%v", configured, err)
	}
	if err := denyReceipt.Validate(); err != nil || denyReceipt.PolicyDirectory != latest.PolicyDirectory ||
		denyReceipt.ActiveRevision != latest.AggregateRevision || denyReceipt.PolicyDirectory == stored.PolicyDirectory {
		t.Fatalf("deny receipt=%+v latest=%+v validate=%v", denyReceipt, latest, err)
	}
}

func TestApplyLearnedPolicyRulesUpdatesOnlyTheTargetDomainAllowFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writeMinimalPolicyFixture(t, state)
	denyPath := filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyDenyFileName)
	denyBefore, err := os.ReadFile(denyPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	rule := learnedRuleFixture(t, "/repos/cli/cli")

	if _, err := runtime.ApplyLearnedPolicyRules(
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
	allowPath := filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyAllowFileName)
	data, err := os.ReadFile(allowPath)
	if err != nil {
		t.Fatal(err)
	}
	var allow policyDomainAllow
	if err := json.Unmarshal(data, &allow); err != nil {
		t.Fatal(err)
	}
	if len(allow.Rules) != 1 || allow.Rules[0].ID != rule.ID {
		t.Fatalf("learned rules = %+v", allow.Rules)
	}
	denyAfter, err := os.ReadFile(denyPath)
	if err != nil || !bytes.Equal(denyAfter, denyBefore) {
		t.Fatalf("deny source changed: error=%v before=%s after=%s", err, denyBefore, denyAfter)
	}
	info, err := os.Stat(allowPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("allow.json mode = %v, error = %v", info.Mode().Perm(), err)
	}
	if _, err := runtime.ApplyLearnedPolicyRules(
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
	writeMinimalPolicyFixture(t, state)
	allowPath := filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyAllowFileName)
	before, err := os.ReadFile(allowPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	stale := learnedRuleFixture(t, "/repos/cli/stale")

	_, err = runtime.ApplyLearnedPolicyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{stale}, []tobari.LearnedPolicyRule{},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "policy_data_changed" {
		t.Fatalf("error = %v", err)
	}
	after, readErr := os.ReadFile(allowPath)
	if readErr != nil || string(after) != string(before) || len(runner.outputs) != 0 {
		t.Fatalf("rejected update changed state: read=%v calls=%v data=%s", readErr, runner.outputs, after)
	}
}

func TestApplyPolicyDenyRulesPreservesAllowsAndActivatesExactDeny(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writeMinimalPolicyFixture(t, state)
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	rule := deniedRuleFixture(t, "/user/settings")

	if _, err := runtime.ApplyPolicyDenyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{},
		[]tobari.PolicyDenyRule{}, []tobari.PolicyDenyRule{rule},
	); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyDenyFileName))
	if err != nil {
		t.Fatal(err)
	}
	var document policyDomainDeny
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Rules) != 1 || document.Rules[0].ID != rule.ID {
		t.Fatalf("learned deny rules = %+v", document.Rules)
	}
	read, err := runtime.ReadPolicyDenyRules(context.Background(), state)
	if err != nil || len(read.Exact) != 1 || read.Exact[0].ID != rule.ID {
		t.Fatalf("read deny rules = %+v, error = %v", read, err)
	}
	if _, err := runtime.ApplyPolicyDenyRules(
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
	writeMinimalPolicyFixture(t, state)
	denyPath := filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyDenyFileName)
	before, err := os.ReadFile(denyPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	rule := deniedRuleFixture(t, "/user/settings")

	_, err = runtime.ApplyPolicyDenyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{},
		[]tobari.PolicyDenyRule{rule}, []tobari.PolicyDenyRule{},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "policy_data_changed" {
		t.Fatalf("error = %v", err)
	}
	after, readErr := os.ReadFile(denyPath)
	if readErr != nil || string(after) != string(before) || len(runner.outputs) != 0 {
		t.Fatalf("rejected deny update changed state: read=%v calls=%v data=%s", readErr, runner.outputs, after)
	}
}

func TestApplyLearnedPolicyRulesRejectsFailedPreflightBeforeWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writeMinimalPolicyFixture(t, state)
	allowPath := filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyAllowFileName)
	before, err := os.ReadFile(allowPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{outputData: []byte("FAIL"), outputErr: errors.New("exit 2")}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	rule := learnedRuleFixture(t, "/repos/cli/cli")

	_, err = runtime.ApplyLearnedPolicyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{rule},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "policy_preflight_failed" {
		t.Fatalf("error = %v", err)
	}
	after, readErr := os.ReadFile(allowPath)
	if readErr != nil || string(after) != string(before) || len(runner.outputs) != 1 {
		t.Fatalf("failed preflight changed state: read=%v calls=%v data=%s", readErr, runner.outputs, after)
	}
}

func TestApplyLearnedPolicyRulesRejectsHostEditDuringPreflight(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writeMinimalPolicyFixture(t, state)
	dataPath := filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyAllowFileName)
	runner := &recordingRunner{}
	runner.onOutput = func(call int) {
		if call != 1 {
			return
		}
		changed := strings.Replace(minimalPolicyAllowFixture, `"graphql_endpoints":[]`, `"graphql_endpoints":[{"scheme":"https","host":"api.github.com","port":443,"path":"/graphql"}]`, 1)
		if err := os.WriteFile(dataPath, []byte(changed), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	rule := learnedRuleFixture(t, "/repos/cli/cli")

	_, err := runtime.ApplyLearnedPolicyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{rule},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "policy_data_changed" {
		t.Fatalf("error = %v", err)
	}
	data, readErr := os.ReadFile(dataPath)
	if readErr != nil || !bytes.Contains(data, []byte(`"/graphql"`)) || len(runner.outputs) != 1 {
		t.Fatalf("concurrent edit was overwritten: read=%v calls=%v data=%s", readErr, runner.outputs, data)
	}
}

func TestManagedPolicyDataRejectsAmbiguousOrUnsafeHostFiles(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*testing.T, tobari.State){
		"unsupported flat shape": func(t *testing.T, state tobari.State) {
			writeMinimalPolicyFixture(t, state)
			if err := os.WriteFile(filepath.Join(state.PolicyDirectory, "data.json"), []byte(`{}`), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"duplicate key": func(t *testing.T, state tobari.State) {
			writeMinimalPolicyFixture(t, state)
			path := filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyAllowFileName)
			if err := os.WriteFile(path, []byte(`{"schema_version":1,"schema_version":1,"host":"api.github.com","graphql_endpoints":[],"rules":[]}`), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"allow symlink": func(t *testing.T, state tobari.State) {
			writeMinimalPolicyFixture(t, state)
			target := filepath.Join(filepath.Dir(state.PolicyDirectory), "outside.json")
			if err := os.WriteFile(target, []byte(minimalPolicyAllowFixture), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyAllowFileName)
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		},
		"unsafe child mode": func(t *testing.T, state tobari.State) {
			writeMinimalPolicyFixture(t, state)
			if err := os.Chmod(filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyAllowFileName), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"unsafe directory mode": func(t *testing.T, state tobari.State) {
			writeMinimalPolicyFixture(t, state)
			if err := os.Chmod(state.PolicyDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"extra domain file": func(t *testing.T, state tobari.State) {
			writeMinimalPolicyFixture(t, state)
			if err := os.WriteFile(
				filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", "notes.json"), []byte(`{}`), 0o600,
			); err != nil {
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
			_, err := runtime.ApplyLearnedPolicyRules(
				context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{rule},
			)
			if err == nil || len(runner.outputs) != 0 {
				t.Fatalf("unsafe policy was accepted: error=%v calls=%v", err, runner.outputs)
			}
		})
	}
}

func TestDomainPolicyJSONContractRejectsAmbiguousSources(t *testing.T) {
	t.Parallel()
	ruleData, err := json.Marshal(learnedRuleFixture(t, "/duplicate"))
	if err != nil {
		t.Fatal(err)
	}
	retiredPrefixRuleData := bytes.Replace(ruleData, []byte(`"match":"exact"`), []byte(`"match":"prefix"`), 1)
	tests := map[string]struct {
		domain string
		allow  string
		deny   string
	}{
		"unknown field": {
			domain: "api.github.com",
			allow:  strings.Replace(minimalPolicyAllowFixture, `"rules":[]`, `"future":true,"rules":[]`, 1),
			deny:   minimalPolicyDenyFixture,
		},
		"missing field": {
			domain: "api.github.com",
			allow:  strings.Replace(minimalPolicyAllowFixture, `"host":"api.github.com",`, "", 1),
			deny:   minimalPolicyDenyFixture,
		},
		"directory host mismatch": {
			domain: "example.com",
			allow:  minimalPolicyAllowFixture,
			deny:   minimalPolicyDenyFixture,
		},
		"retired authorities": {
			domain: "api.github.com",
			allow:  strings.Replace(minimalPolicyAllowFixture, `"graphql_endpoints":[]`, `"authorities":[{"scheme":"https","host":"api.github.com","ports":[443]}],"graphql_endpoints":[]`, 1),
			deny:   minimalPolicyDenyFixture,
		},
		"retired methods": {
			domain: "api.github.com",
			allow:  strings.Replace(minimalPolicyAllowFixture, `"graphql_endpoints":[]`, `"methods":{"read":["GET"],"write":[{"method":"POST","exclude_path_prefixes":["/repos/"]}]},"graphql_endpoints":[]`, 1),
			deny:   minimalPolicyDenyFixture,
		},
		"retired credential profiles": {
			domain: "api.github.com",
			allow:  strings.Replace(minimalPolicyAllowFixture, `"graphql_endpoints":[]`, `"credential_profiles":[{"profile":"github","host":"api.github.com"}],"graphql_endpoints":[]`, 1),
			deny:   minimalPolicyDenyFixture,
		},
		"retired baseline prefix deny": {
			domain: "api.github.com",
			allow:  minimalPolicyAllowFixture,
			deny:   strings.Replace(minimalPolicyDenyFixture, `"rules":[]`, `"baseline_rules":[{"host":"api.github.com","method":"POST","path_prefix":"/repos/"}],"rules":[]`, 1),
		},
		"duplicate learned rule ID": {
			domain: "api.github.com",
			allow:  strings.Replace(minimalPolicyAllowFixture, `"rules":[]`, `"rules":[`+string(ruleData)+`,`+string(ruleData)+`]`, 1),
			deny:   minimalPolicyDenyFixture,
		},
		"retired learned prefix rule": {
			domain: "api.github.com",
			allow:  strings.Replace(minimalPolicyAllowFixture, `"rules":[]`, `"rules":[`+string(retiredPrefixRuleData)+`]`, 1),
			deny:   minimalPolicyDenyFixture,
		},
		"wildcard host": {
			domain: "*.example.com",
			allow:  strings.ReplaceAll(minimalPolicyAllowFixture, "api.github.com", "*.example.com"),
			deny:   strings.ReplaceAll(minimalPolicyDenyFixture, "api.github.com", "*.example.com"),
		},
		"uppercase host": {
			domain: "API.github.com",
			allow:  strings.ReplaceAll(minimalPolicyAllowFixture, "api.github.com", "API.github.com"),
			deny:   strings.ReplaceAll(minimalPolicyDenyFixture, "api.github.com", "API.github.com"),
		},
		"trailing dot host": {
			domain: "api.github.com.",
			allow:  strings.ReplaceAll(minimalPolicyAllowFixture, "api.github.com", "api.github.com."),
			deny:   strings.ReplaceAll(minimalPolicyDenyFixture, "api.github.com", "api.github.com."),
		},
		"IP literal": {
			domain: "127.0.0.1",
			allow:  strings.ReplaceAll(minimalPolicyAllowFixture, "api.github.com", "127.0.0.1"),
			deny:   strings.ReplaceAll(minimalPolicyDenyFixture, "api.github.com", "127.0.0.1"),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state := runtimeState(t.TempDir())
			writePolicyDomainFixture(t, state, test.domain, test.allow, test.deny)
			if _, err := readPolicyData(state.PolicyDirectory); err == nil {
				t.Fatal("invalid domain policy source was accepted")
			}
		})
	}
}

func TestPolicySourceTransactionCreatesBothDomainFilesAndRollsBack(t *testing.T) {
	t.Parallel()
	state := runtimeState(t.TempDir())
	writeMinimalPolicyFixture(t, state)
	original, err := readPolicyData(state.PolicyDirectory)
	if err != nil {
		t.Fatal(err)
	}
	rule := learnedRuleFixtureForHost(t, "new.example.com", "/created")
	candidate, err := original.withPolicyRules([]tobari.LearnedPolicyRule{rule}, []tobari.PolicyDenyRule{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{policyAllowFileName, policyDenyFileName} {
		relative := filepath.Join(policyDomainsName, "api.github.com", name)
		if !bytes.Equal(candidate.sources[relative], original.sources[relative]) {
			t.Fatalf("unchanged %s was rewritten", relative)
		}
	}
	transaction, err := beginPolicySourceTransaction(state.PolicyDirectory, original.sources, candidate.sources)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{policyAllowFileName, policyDenyFileName} {
		if _, err := os.Lstat(filepath.Join(state.PolicyDirectory, policyDomainsName, "new.example.com", name)); err != nil {
			t.Fatalf("new domain %s is unavailable: %v", name, err)
		}
	}
	if _, err := readPolicyData(state.PolicyDirectory); err == nil {
		t.Fatal("ordinary reader accepted an uncommitted policy generation")
	}
	if err := transaction.rollback(); err != nil {
		t.Fatal(err)
	}
	restored, err := readPolicyData(state.PolicyDirectory)
	if err != nil || !reflect.DeepEqual(restored.sources, original.sources) {
		t.Fatalf("rollback did not restore original sources: error=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(state.PolicyDirectory, policyDomainsName, "new.example.com")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rolled-back domain remains visible: %v", err)
	}
}

func TestPolicyCandidateValidationReceiptIsTransactionBoundAndSingleUse(t *testing.T) {
	t.Parallel()
	policyDirectory := filepath.Join(t.TempDir(), "policy")
	candidateDigest := strings.Repeat("a", sha256.Size*2)
	preflightDigest := strings.Repeat("b", sha256.Size*2)
	transaction := &policySourceTransaction{
		policyDirectory: policyDirectory,
		journal:         policySourceJournal{CandidateDigest: candidateDigest},
	}
	if transaction.consumeCandidateValidation(policyDirectory, candidateDigest, preflightDigest) {
		t.Fatal("missing validation receipt skipped the Context OPA test")
	}
	if err := transaction.bindCandidateValidation(policyCandidateValidationReceipt{
		policyDirectory: policyDirectory,
		candidateDigest: strings.Repeat("c", sha256.Size*2),
		preflightDigest: preflightDigest,
	}); err == nil {
		t.Fatal("receipt for a different candidate was bound to the transaction")
	}
	if err := transaction.bindCandidateValidation(policyCandidateValidationReceipt{
		policyDirectory: policyDirectory,
		candidateDigest: candidateDigest,
		preflightDigest: preflightDigest,
	}); err != nil {
		t.Fatal(err)
	}
	if transaction.consumeCandidateValidation(policyDirectory, candidateDigest, strings.Repeat("d", sha256.Size*2)) {
		t.Fatal("mismatched preflight digest skipped the Context OPA test")
	}
	if !transaction.consumeCandidateValidation(policyDirectory, candidateDigest, preflightDigest) {
		t.Fatal("matching validation receipt was not reused")
	}
	if transaction.consumeCandidateValidation(policyDirectory, candidateDigest, preflightDigest) {
		t.Fatal("validation receipt was reused more than once")
	}
}

func TestGeneratedPolicyProjectionIsDeterministic(t *testing.T) {
	t.Parallel()
	state := runtimeState(t.TempDir())
	writeMinimalPolicyFixture(t, state)
	writePolicyDomainFixture(
		t, state, "example.com",
		strings.ReplaceAll(minimalPolicyAllowFixture, "api.github.com", "example.com"),
		strings.ReplaceAll(minimalPolicyDenyFixture, "api.github.com", "example.com"),
	)
	file, err := readPolicyData(state.PolicyDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 32; iteration++ {
		generated, err := composePolicyData(file.allows, file.denies)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(generated, file.source) {
			t.Fatalf("projection changed on iteration %d", iteration)
		}
	}
}

func TestPolicySourceTransactionRecoveryUsesDurableActivationRevision(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name            string
		activeCandidate bool
	}{
		{name: "rollback before state activation"},
		{name: "commit after state activation", activeCandidate: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := runtimeState(t.TempDir())
			writeMinimalPolicyFixture(t, state)
			original, err := readPolicyData(state.PolicyDirectory)
			if err != nil {
				t.Fatal(err)
			}
			rule := learnedRuleFixture(t, "/recovery")
			candidate, err := original.withPolicyRules([]tobari.LearnedPolicyRule{rule}, []tobari.PolicyDenyRule{})
			if err != nil {
				t.Fatal(err)
			}
			transaction, err := beginPolicySourceTransaction(state.PolicyDirectory, original.sources, candidate.sources)
			if err != nil {
				t.Fatal(err)
			}
			revision := strings.Repeat("a", 64)
			if err := transaction.setCandidateAggregateRevision(revision); err != nil {
				t.Fatal(err)
			}
			active := strings.Repeat("b", 64)
			want := original.sources
			if test.activeCandidate {
				active = revision
				want = candidate.sources
			}
			if err := recoverPolicySourceTransaction(state.PolicyDirectory, active); err != nil {
				t.Fatal(err)
			}
			current, err := readPolicyData(state.PolicyDirectory)
			if err != nil || !reflect.DeepEqual(current.sources, want) {
				t.Fatalf("recovered sources are wrong: error=%v", err)
			}
		})
	}
}

func TestPolicySourceTransactionRecoversIntermediateRenamePhases(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{policySourcePhasePrepared, policySourcePhaseOldMoved} {
		t.Run(phase, func(t *testing.T) {
			state := runtimeState(t.TempDir())
			writeMinimalPolicyFixture(t, state)
			original, err := readPolicyData(state.PolicyDirectory)
			if err != nil {
				t.Fatal(err)
			}
			candidate, err := original.withPolicyRules(
				[]tobari.LearnedPolicyRule{learnedRuleFixtureForHost(t, "new.example.com", "/partial")},
				[]tobari.PolicyDenyRule{},
			)
			if err != nil {
				t.Fatal(err)
			}
			contextDirectory := filepath.Dir(state.PolicyDirectory)
			stagePath, err := os.MkdirTemp(contextDirectory, ".policy-domains-stage-")
			if err != nil {
				t.Fatal(err)
			}
			if err := writePolicyDomainsSnapshot(stagePath, candidate.sources); err != nil {
				t.Fatal(err)
			}
			stageName := filepath.Base(stagePath)
			backupName := ".policy-domains-backup-" + strings.TrimPrefix(stageName, ".policy-domains-stage-")
			journal := policySourceJournal{
				SchemaVersion:   policySourceJournalSchema,
				Phase:           phase,
				StageName:       stageName,
				BackupName:      backupName,
				OriginalDigest:  policySourceDigest(original.sources),
				CandidateDigest: policySourceDigest(candidate.sources),
			}
			if phase == policySourcePhaseOldMoved {
				if err := os.Rename(
					filepath.Join(state.PolicyDirectory, policyDomainsName), filepath.Join(contextDirectory, backupName),
				); err != nil {
					t.Fatal(err)
				}
			}
			if err := writePolicySourceJournal(state.PolicyDirectory, journal); err != nil {
				t.Fatal(err)
			}
			if err := recoverPolicySourceTransaction(state.PolicyDirectory, strings.Repeat("b", 64)); err != nil {
				t.Fatal(err)
			}
			restored, err := readPolicyData(state.PolicyDirectory)
			if err != nil || !reflect.DeepEqual(restored.sources, original.sources) {
				t.Fatalf("intermediate phase did not restore original: error=%v", err)
			}
		})
	}
}

func TestPolicySourceTransactionFailsClosedAfterExternalEdit(t *testing.T) {
	t.Parallel()
	state := runtimeState(t.TempDir())
	writeMinimalPolicyFixture(t, state)
	original, err := readPolicyData(state.PolicyDirectory)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := original.withPolicyRules([]tobari.LearnedPolicyRule{learnedRuleFixture(t, "/edited")}, []tobari.PolicyDenyRule{})
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := beginPolicySourceTransaction(state.PolicyDirectory, original.sources, candidate.sources)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyAllowFileName)
	if err := os.WriteFile(path, append(candidate.sources[filepath.Join(policyDomainsName, "api.github.com", policyAllowFileName)], '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := transaction.rollback(); err == nil {
		t.Fatal("rollback overwrote a direct external edit")
	}
	if _, err := readPolicyData(state.PolicyDirectory); err == nil {
		t.Fatal("ordinary reader accepted a source with an unresolved transaction")
	}
}

func TestGuidedAndAdvancedPolicyPreflightRequireExactSourceLayouts(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtimeStore, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if _, err := runtimeStore.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeStore.CreateContext(
		context.Background(), "advanced", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeAdvanced, tobari.ContextSourceAccessReadWrite,
	); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{tobari.DefaultContextName, "advanced"} {
		manifest, paths, err := runtimeStore.resolveContext(name)
		if err != nil {
			t.Fatal(err)
		}
		candidate, err := readPolicyData(paths.PolicyDirectory)
		if err != nil {
			t.Fatal(err)
		}
		preflight, err := prepareContextPolicyPreflight(manifest, paths.PolicyDirectory, candidate)
		if err != nil {
			t.Fatalf("%s preflight: %v", name, err)
		}
		if err := os.RemoveAll(preflight); err != nil {
			t.Fatal(err)
		}
	}
	advanced, advancedPaths, err := runtimeStore.resolveContext("advanced")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(advancedPaths.PolicyDirectory, "tobari_test.rego")); err != nil {
		t.Fatal(err)
	}
	candidate, err := readPolicyData(advancedPaths.PolicyDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepareContextPolicyPreflight(advanced, advancedPaths.PolicyDirectory, candidate); err == nil {
		t.Fatal("Advanced preflight accepted a missing policy test file")
	}
}
