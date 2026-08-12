package dockerruntime

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type bundleBuildFailureRunner struct {
	outputs []runnerCall
}

func (r *bundleBuildFailureRunner) Run(
	_ context.Context, _ []string, _ []string, _ io.Reader, _ io.Writer, _ io.Writer,
) error {
	return nil
}

func (r *bundleBuildFailureRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) > 0 && (args[0] == "inspect" || (args[0] == "volume" && len(args) > 1 && args[1] == "inspect")) {
		return []byte(ownerValue + "\n"), nil
	}
	if slices.Contains(args, "build") {
		return []byte("synthetic invalid candidate"), errors.New("exit 1")
	}
	return nil, nil
}

const denyAuditLine = `{"schema_version":1,"cluster":"default","context":"default","context_id":"01912345-6789-7abc-8def-0123456789ad","decision":"deny","duration_ms":3,"host":"api.github.com","learnable":true,"method":"GET","path":"/repos/cli/cli","port":443,"project_id":"01912345-6789-7abc-8def-0123456789ab","project_root":"/workspace/project","protocol":"http","reason":"request did not match an allow rule","request_id":"7185da2688d7469aae9cd9068e920b0b","scheme":"https","timestamp":"2026-07-30T10:41:11Z","upstream_status":403}`

const graphqlDenyAuditLine = `{"schema_version":1,"cluster":"default","context":"default","context_id":"01912345-6789-7abc-8def-0123456789ad","decision":"deny","duration_ms":3,"host":"api.github.com","learnable":true,"method":"POST","path":"/graphql","port":443,"project_id":"01912345-6789-7abc-8def-0123456789ab","project_root":"/workspace/project","protocol":"graphql","graphql_operation_type":"mutation","graphql_root_field":"updateIssue","reason":"request did not match an allow rule","request_id":"7185da2688d7469aae9cd9068e920b0b","scheme":"https","timestamp":"2026-07-30T10:41:11Z","upstream_status":403}`

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
		items[0].Scheme != "https" ||
		items[0].Path != "/repos/cli/cli" || items[0].StatusCode != 403 ||
		!items[0].Learnable {
		t.Fatalf("denials = %+v", items)
	}
}

func TestParseGatewayDenialsPreservesGraphQLRootIdentity(t *testing.T) {
	t.Parallel()
	items, err := parseGatewayDenials([]byte(graphqlDenyAuditLine + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].EffectiveProtocol() != tobari.PolicyProtocolGraphQL ||
		items[0].GraphQLOperationType != tobari.GraphQLOperationMutation ||
		items[0].GraphQLRootField != "updateIssue" {
		t.Fatalf("GraphQL denial = %+v", items)
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

func TestApplyPolicyTestsPublishesBundleAndKeepsOPAStable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, _ := newRuntime(root+"/config", root+"/state", runner)
	state := runtimeState(root)
	if err := runtime.ApplyPolicy(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 12 {
		t.Fatalf("output calls = %v", runner.outputs)
	}
	build := runner.outputs[9].args
	for _, required := range []string{
		"run", "--network", "none", "--read-only", "--cap-drop", "ALL",
		"type=volume,src=tobari-policy-bundle,dst=/bundle", "build",
		"--revision", state.AggregateRevision,
	} {
		if !slices.Contains(build, required) {
			t.Fatalf("bundle build argv = %v, lacks %q", build, required)
		}
	}
	publish := runner.outputs[10].args
	if !slices.Contains(publish, "--entrypoint") || !slices.Contains(publish, "sh") ||
		!slices.Contains(publish, "/bundle/bundle.tar.gz") ||
		!strings.Contains(strings.Join(publish, "\n"), ".candidate-"+state.AggregateRevision+".tar.gz") {
		t.Fatalf("atomic bundle publication argv = %v", publish)
	}
	for _, call := range runner.outputs {
		if slices.Contains(call.args, "compose") || slices.Contains(call.args, "--force-recreate") {
			t.Fatalf("policy activation unexpectedly recreates a service: %v", call.args)
		}
	}
	confirm := runner.outputs[11].args
	if !slices.Contains(confirm, "exec") || !slices.Contains(confirm, opaContainer) ||
		!strings.Contains(strings.Join(confirm, "\n"), state.AggregateRevision) {
		t.Fatalf("revision confirmation argv = %v", confirm)
	}
}

func TestApplyPolicyRejectsFailedTestsBeforeBundlePublication(t *testing.T) {
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

func TestApplyPolicyBuildFailureNeverPublishesFinalBundle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &bundleBuildFailureRunner{}
	runtime, _ := newRuntime(root+"/config", root+"/state", runner)
	if err := runtime.ApplyPolicy(context.Background(), runtimeState(root)); err == nil {
		t.Fatal("invalid bundle build unexpectedly succeeded")
	}
	foundBuild := false
	for _, call := range runner.outputs {
		if slices.Contains(call.args, "build") {
			foundBuild = true
			if slices.Contains(call.args, "/bundle/bundle.tar.gz") ||
				!strings.Contains(strings.Join(call.args, "\n"), "/bundle/.candidate-") {
				t.Fatalf("failed build targeted the active bundle: %v", call.args)
			}
		}
		if slices.Contains(call.args, "tobari-policy-publish") {
			t.Fatalf("failed build reached atomic publication: %v", call.args)
		}
	}
	if !foundBuild {
		t.Fatalf("bundle builder was not invoked: %v", runner.outputs)
	}
}
