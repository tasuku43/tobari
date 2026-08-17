package dockerruntime

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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

const mcpDenyAuditLine = `{"schema_version":1,"cluster":"default","context":"default","context_id":"01912345-6789-7abc-8def-0123456789ad","decision":"deny","duration_ms":3,"host":"chatgpt.com","learnable":true,"method":"POST","path":"/backend-api/ps/mcp","port":443,"project_id":"01912345-6789-7abc-8def-0123456789ab","project_root":"/workspace/project","protocol":"mcp","mcp_method":"tools/call","mcp_tool_name":"codex_apps.search","reason":"request did not match an allow rule","request_id":"7185da2688d7469aae9cd9068e920b0b","scheme":"https","timestamp":"2026-07-30T10:41:11Z","upstream_status":403}`

func writePolicyArchiveFixture(t *testing.T, state tobari.State) {
	t.Helper()
	if err := os.MkdirAll(state.PolicyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.PolicyDirectory, "data.json"), []byte(`{"tobari":{}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestPolicySourceArchivePreservesOwnerOnlyProjection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	state := runtimeState(root)
	writePolicyArchiveFixture(t, state)

	archive, cleanup, err := runtime.policySourceArchive(state.PolicyDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	header, err := tar.NewReader(archive).Next()
	if err != nil {
		t.Fatal(err)
	}
	if header.Name != "data.json" || header.Mode&0o077 != 0 || header.Uid != 0 || header.Gid != 0 {
		t.Fatalf("policy archive header = %+v", header)
	}

	if err := os.Chmod(filepath.Join(state.PolicyDirectory, "data.json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, cleanup, err := runtime.policySourceArchive(state.PolicyDirectory); err == nil {
		cleanup()
		t.Fatal("non-owner-only aggregate policy was archived")
	}
}

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

func TestParseGatewayDenialsPreservesOnlyMCPSemanticIdentity(t *testing.T) {
	t.Parallel()
	items, err := parseGatewayDenials([]byte(mcpDenyAuditLine + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].EffectiveProtocol() != tobari.PolicyProtocolMCP || items[0].MCPMethod != "tools/call" || items[0].MCPToolName != "codex_apps.search" {
		t.Fatalf("MCP denial = %+v", items)
	}
	if strings.Contains(mcpDenyAuditLine, "arguments") {
		t.Fatal("MCP audit retained arguments")
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

func TestBindActiveHostLoopbackDenialAddsOnlyCurrentEpochAndPort(t *testing.T) {
	t.Parallel()
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	project := projectRuntimeInstance(t, runtime)
	if err := runtime.ensureHostLoopbackStore(context.Background()); err != nil {
		t.Fatal(err)
	}
	route, err := tobari.NewAttachmentHostLoopbackRoute("att_0123456789abcdef0123456789abcdef", project, 43179, strings.Repeat("3", 64))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicJSON(runtime.hostLoopbackRegistryPath(), tobari.HostLoopbackRegistry{SchemaVersion: tobari.HostLoopbackRegistrySchema, Routes: []tobari.AttachmentHostLoopbackRoute{route}}); err != nil {
		t.Fatal(err)
	}
	denial := tobari.PolicyDenial{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "http", Protocol: tobari.PolicyProtocolHTTP},
		Timestamp:              "2026-08-17T12:00:00Z", RequestID: strings.Repeat("1", 32),
		ContextID: route.ContextID, ContextName: route.ContextName, ProjectID: route.ProjectID, ProjectRoot: route.ProjectRoot,
		Host: tobari.HostLoopbackHostname, Port: 3000, Method: "GET", Path: "/health",
		Reason: "review", StatusCode: 403, Learnable: true,
	}
	bound, err := runtime.bindActiveHostLoopbackDenials([]tobari.PolicyDenial{denial})
	if err != nil || len(bound) != 1 || bound[0].AttachmentEpochID != route.EpochID || bound[0].Port != 3000 || bound[0].EffectiveDestinationKind() != tobari.PolicyDestinationHostLoopback {
		t.Fatalf("bound Host Loopback denial = %+v, %v", bound, err)
	}
	if err := writeAtomicJSON(runtime.hostLoopbackRegistryPath(), emptyHostLoopbackRegistry()); err != nil {
		t.Fatal(err)
	}
	stale, err := runtime.bindActiveHostLoopbackDenials([]tobari.PolicyDenial{denial})
	if err != nil || len(stale) != 0 {
		t.Fatalf("stale Host Loopback denial = %+v, %v", stale, err)
	}
}

func TestApplyPolicyTestsPublishesBundleAndKeepsOPAStable(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, _ := newRuntime(root+"/config", root+"/state", runner)
	state := runtimeState(root)
	writePolicyArchiveFixture(t, state)
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
	if strings.Contains(strings.Join(build, "\n"), "type=bind") ||
		!strings.Contains(strings.Join(build, "\n"), "/bundle/.source-") {
		t.Fatalf("bundle builder consumed a host bind instead of staged policy: %v", build)
	}
	publish := runner.outputs[10].args
	if !slices.Contains(publish, "--entrypoint") || !slices.Contains(publish, "sh") ||
		!slices.Contains(publish, "/bundle/bundle.tar.gz") ||
		!strings.Contains(strings.Join(publish, "\n"), ".candidate-"+state.AggregateRevision+".tar.gz") {
		t.Fatalf("atomic bundle publication argv = %v", publish)
	}
	if len(runner.runs) != 2 {
		t.Fatalf("policy source staging calls = %v", runner.runs)
	}
	for _, stage := range runner.runs {
		staging := strings.Join(stage.args, "\n")
		if !strings.Contains(staging, "tobari-policy-stage") ||
			!strings.Contains(staging, "--interactive") || strings.Contains(staging, "type=bind") {
			t.Fatalf("policy source staging argv = %v", stage.args)
		}
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

func TestPreparePolicyBundleSkipsUnchangedReadyRevision(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputQueue: [][]byte{
		[]byte(ownerValue + "\n"),
		[]byte("true true true true true true\n"),
	}}
	runtime, _ := newRuntime(root+"/config", root+"/state", runner)
	state := runtimeState(root)

	if err := runtime.preparePolicyBundle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if len(runner.outputs) != 2 {
		t.Fatalf("output calls = %v", runner.outputs)
	}
	probe := strings.Join(runner.outputs[1].args, "\n")
	if !strings.Contains(probe, state.AggregateRevision) ||
		!strings.Contains(probe, "/v1/data/tobari/http/decision") {
		t.Fatalf("policy readiness probe = %v", runner.outputs[1].args)
	}
	for _, call := range runner.outputs {
		if slices.Contains(call.args, "build") || strings.Contains(strings.Join(call.args, "\n"), "tobari-policy-publish") {
			t.Fatalf("unchanged ready policy was republished: %v", call.args)
		}
	}
}

func TestPolicyRevisionReadyRejectsDefinedFalseResult(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputData: []byte("false\n")}
	runtime := &Runtime{runner: runner}

	ready, output := runtime.policyRevisionReady(context.Background(), strings.Repeat("a", 64))
	if ready || string(output) != "false\n" {
		t.Fatalf("policy readiness = %v, output = %q", ready, output)
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
	state := runtimeState(root)
	writePolicyArchiveFixture(t, state)
	if err := runtime.ApplyPolicy(context.Background(), state); err == nil {
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
