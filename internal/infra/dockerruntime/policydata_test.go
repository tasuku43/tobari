package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func learnedRuleFixture(t *testing.T, path string) tobari.LearnedPolicyRule {
	t.Helper()
	candidate, err := tobari.NewPolicyCandidate(tobari.PolicyDenial{
		Timestamp:  "2026-07-30T10:41:11Z",
		RequestID:  "7185da2688d7469aae9cd9068e920b0b",
		ProjectID:  "01912345-6789-7abc-8def-0123456789ab",
		Host:       "api.github.com",
		Port:       443,
		Method:     "GET",
		Path:       path,
		Reason:     "request did not match an allow rule",
		StatusCode: 403,
		Learnable:  true,
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
		Timestamp:  "2026-07-30T10:41:11Z",
		RequestID:  "8185da2688d7469aae9cd9068e920b0b",
		ProjectID:  "01912345-6789-7abc-8def-0123456789ab",
		Host:       "api.github.com",
		Port:       443,
		Method:     "GET",
		Path:       path,
		Reason:     "request did not match an allow rule",
		StatusCode: 403,
		Learnable:  true,
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

func TestApplyLearnedPolicyRulesPreservesHostDataAndActivatesTestedCopy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writePolicyFixture(t, state, `{
  "host_owned": {"keep": true},
  "tobari": {
    "allowed_hosts": ["api.github.com"],
    "host_extension": {"keep": true},
    "learned_allow_rules": []
  }
}
`)
	runner := &recordingRunner{outputQueue: [][]byte{
		nil,
		nil,
		[]byte("default\n"),
		nil,
	}}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	rule := learnedRuleFixture(t, "/repos/cli/cli")

	if err := runtime.ApplyLearnedPolicyRules(
		context.Background(), state, []tobari.LearnedPolicyRule{}, []tobari.LearnedPolicyRule{rule},
	); err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 4 {
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
	var rules []tobari.LearnedPolicyRule
	if err := json.Unmarshal(tobariData[learnedPolicyDataName], &rules); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].ID != rule.ID {
		t.Fatalf("learned rules = %+v", rules)
	}
	info, err := os.Stat(filepath.Join(state.PolicyDirectory, "data.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("data.json mode = %v, error = %v", info.Mode().Perm(), err)
	}
}

func TestApplyLearnedPolicyRulesRejectsChangedDataBeforeDockerOrWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writePolicyFixture(t, state, `{"tobari":{"learned_allow_rules":[]}}`+"\n")
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
    "allowed_hosts": ["api.github.com"],
    "learned_allow_rules": [],
    "learned_deny_rules": []
  }
}
`)
	runner := &recordingRunner{outputQueue: [][]byte{nil, nil, []byte("default\n"), nil}}
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
	var got []tobari.PolicyDenyRule
	if err := json.Unmarshal(tobariData[learnedDenyDataName], &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != rule.ID {
		t.Fatalf("learned deny rules = %+v", got)
	}
	read, err := runtime.ReadPolicyDenyRules(context.Background(), state)
	if err != nil || len(read.Exact) != 1 || read.Exact[0].ID != rule.ID {
		t.Fatalf("read deny rules = %+v, error = %v", read, err)
	}
}

func TestApplyPolicyDenyRulesRejectsChangedDenySnapshotBeforeDockerOrWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := runtimeState(root)
	writePolicyFixture(t, state, `{"tobari":{"learned_allow_rules":[],"learned_deny_rules":[]}}`+"\n")
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
	writePolicyFixture(t, state, `{"tobari":{"learned_allow_rules":[]}}`+"\n")
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
	writePolicyFixture(t, state, `{"host_owned":{"revision":1},"tobari":{"learned_allow_rules":[]}}`+"\n")
	runner := &recordingRunner{}
	runner.onOutput = func(call int) {
		if call != 1 {
			return
		}
		if err := os.WriteFile(
			dataPath,
			[]byte(`{"host_owned":{"revision":2},"tobari":{"learned_allow_rules":[]}}`+"\n"),
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
		"duplicate key": func(t *testing.T, state tobari.State) {
			writePolicyFixture(t, state, `{"tobari":{"learned_allow_rules":[],"learned_allow_rules":[]}}`)
		},
		"data symlink": func(t *testing.T, state tobari.State) {
			writePolicyFixture(t, state, `{"tobari":{"learned_allow_rules":[]}}`)
			target := filepath.Join(filepath.Dir(state.PolicyDirectory), "outside.json")
			if err := os.WriteFile(target, []byte(`{"tobari":{"learned_allow_rules":[]}}`), 0o600); err != nil {
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
			writePolicyFixture(t, state, `{"tobari":{"learned_allow_rules":[]}}`)
			if err := os.Chmod(filepath.Join(state.PolicyDirectory, "tobari.rego"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"unsafe directory mode": func(t *testing.T, state tobari.State) {
			writePolicyFixture(t, state, `{"tobari":{"learned_allow_rules":[]}}`)
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
