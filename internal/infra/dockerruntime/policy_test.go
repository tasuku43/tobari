package dockerruntime

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

const denyAuditLine = `{"cluster":"default","credential_profile":null,"decision":"deny","duration_ms":3,"host":"api.github.com","learnable":true,"method":"GET","path":"/repos/cli/cli","reason":"request did not match an allow rule","request_id":"7185da2688d7469aae9cd9068e920b0b","timestamp":"2026-07-30T10:41:11Z","upstream_status":403}`

func TestParseGatewayDenialsFiltersUnrelatedAndAllowedLines(t *testing.T) {
	t.Parallel()
	data := []byte(
		"mitmproxy startup\n" +
			`{"decision":"allow","host":"api.github.com"}` + "\n" +
			denyAuditLine + "\n",
	)
	items, err := parseGatewayDenials(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Host != "api.github.com" ||
		items[0].Path != "/repos/cli/cli" || items[0].StatusCode != 403 ||
		!items[0].Learnable {
		t.Fatalf("denials = %+v", items)
	}
}

func TestParseGatewayDenialsRejectsMalformedDenyAudit(t *testing.T) {
	t.Parallel()
	_, err := parseGatewayDenials([]byte(
		`{"decision":"deny","host":"api.github.com","authorization":"secret"}` + "\n",
	))
	if err == nil {
		t.Fatal("malformed deny audit was ignored")
	}
}

func TestApplyPolicyTestsThenRecreatesOnlyOwnedOPA(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputQueue: [][]byte{nil, []byte("default\n"), nil}}
	runtime, _ := newRuntime(root+"/config", root+"/state", runner)
	state := runtimeState(root)
	if err := runtime.ApplyPolicy(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 3 {
		t.Fatalf("output calls = %v", runner.outputs)
	}
	recreate := runner.outputs[2].args
	for _, required := range []string{
		"compose", "--no-deps", "--force-recreate", "--wait", "opa",
	} {
		if !slices.Contains(recreate, required) {
			t.Fatalf("recreate argv = %v, lacks %q", recreate, required)
		}
	}
	if slices.Contains(recreate, "gateway") {
		t.Fatalf("policy activation unexpectedly recreates Gateway: %v", recreate)
	}
}

func TestApplyPolicyRejectsFailedTestsBeforeOPARecreation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputData: []byte("FAIL"), outputErr: errors.New("exit 2")}
	runtime, _ := newRuntime(root+"/config", root+"/state", runner)
	err := runtime.ApplyPolicy(context.Background(), runtimeState(root))
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "policy_test_failed" {
		t.Fatalf("error = %v", err)
	}
	if len(runner.outputs) != 1 {
		t.Fatalf("calls after failed policy test = %v", runner.outputs)
	}
}
