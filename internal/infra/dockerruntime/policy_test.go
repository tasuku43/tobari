package dockerruntime

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
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

const awsDenyAuditLine = `{"schema_version":1,"cluster":"default","context":"default","context_id":"01912345-6789-7abc-8def-0123456789ad","decision":"deny","duration_ms":3,"host":"sts.us-east-1.amazonaws.com","learnable":true,"method":"POST","path":"/","port":443,"project_id":"01912345-6789-7abc-8def-0123456789ab","project_root":"/workspace/project","protocol":"aws","aws_wire_protocol":"query","aws_service":"sts","aws_operation":"GetCallerIdentity","reason":"request did not match an allow rule","request_id":"7185da2688d7469aae9cd9068e920b0b","scheme":"https","timestamp":"2026-07-30T10:41:11Z","upstream_status":403}`

const unregisteredPrincipalDenyAuditLine = `{"cluster":"default","context":null,"context_id":null,"decision":"deny","duration_ms":0,"host":"api.anthropic.com","learnable":false,"method":"POST","path":"/api/event_logging/v2/batch","port":443,"project_id":null,"project_root":null,"protocol":"http","reason":"project principal is not registered","request_id":"7886e3bf2e4f4e4d86f6e8ef61acc718","schema_version":1,"scheme":"https","timestamp":"2026-08-21T01:02:47Z","upstream_status":403}`

func writePolicyArchiveFixture(t *testing.T, state tobari.State) {
	t.Helper()
	if err := os.MkdirAll(state.PolicyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.PolicyDirectory, "data.json"), []byte(`{"tobari":{}}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func aggregatePolicyTestState(t *testing.T, runtime *Runtime) tobari.State {
	t.Helper()
	initializeTestWorkspaceManifest(t, runtime)
	projection, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return tobari.State{
		SchemaVersion: 1, RuntimeDirectory: filepath.Join(runtime.stateDirectory, "runtime"),
		AggregateRevision: projection.Revision, ManifestCount: projection.ManifestCount,
		PolicyDirectory: projection.PolicyDirectory, GatewayConfig: projection.GatewayConfig,
		AssetVersion: "asset", EvaluatorIdentity: projection.EvaluatorIdentity,
		PolicyDataIdentity: projection.PolicyDataIdentity,
	}
}

func TestAggregatePolicyBundleArchiveUsesOwnedFixedModules(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	state := runtimeState(root)
	writePolicyArchiveFixture(t, state)

	archive, err := aggregatePolicyBundleArchive(state.PolicyDirectory)
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(bytes.NewReader(archive))
	for _, want := range []string{"router.rego", "guided.rego", "data.json"} {
		header, err := reader.Next()
		if err != nil {
			t.Fatal(err)
		}
		if header.Name != want || header.Mode&0o077 != 0 || header.Uid != 0 || header.Gid != 0 {
			t.Fatalf("policy archive header = %+v, want %q", header, want)
		}
		if _, err := io.Copy(io.Discard, reader); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.Chmod(filepath.Join(state.PolicyDirectory, "data.json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := aggregatePolicyBundleArchive(state.PolicyDirectory); err == nil {
		t.Fatal("non-owner-only aggregate policy was archived")
	}
	_ = runtime
}

func TestAggregatePolicyBundleStagingReceivesExactEmbeddedFixedModules(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	state := aggregatePolicyTestState(t, runtime)
	runner.inputs = nil
	runner.runs = nil
	if err := runtime.publishPolicyBundle(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if len(runner.inputs) != 1 {
		t.Fatalf("Docker-owned policy staging inputs = %d, want one archive", len(runner.inputs))
	}
	files, err := policyTestArchiveFiles(runner.inputs[0])
	if err != nil {
		t.Fatal(err)
	}
	router, module, err := fixedAggregateEvaluatorModules()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || !bytes.Equal(files["router.rego"], router) || !bytes.Equal(files["guided.rego"], module) {
		t.Fatalf("Docker-owned aggregate modules differ from embedded fixed modules: files=%v", files)
	}
	wantData, err := os.ReadFile(filepath.Join(state.PolicyDirectory, "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(files["data.json"], wantData) {
		t.Fatal("Docker-owned aggregate staging did not receive the verified typed data bytes")
	}
}

func TestAggregatePolicyPublicationRejectsDataDriftBeforeDockerStaging(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	state := aggregatePolicyTestState(t, runtime)
	original, err := os.ReadFile(filepath.Join(state.PolicyDirectory, "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hookErr error
	runtime.policyBeforeBundleAssembly = func() {
		var document map[string]any
		if err := json.Unmarshal(original, &document); err != nil {
			hookErr = err
			return
		}
		contexts, ok := document["tobari_contexts"].(map[string]any)
		if !ok || len(contexts) == 0 {
			hookErr = errors.New("aggregate fixture has no Context data")
			return
		}
		for _, raw := range contexts {
			contextData, ok := raw.(map[string]any)
			if !ok {
				hookErr = errors.New("aggregate fixture Context data is invalid")
				return
			}
			policy, ok := contextData["policy"].(map[string]any)
			if !ok {
				hookErr = errors.New("aggregate fixture policy data is invalid")
				return
			}
			policy["method_default"] = "deny"
			break
		}
		hookErr = writeAtomicJSON(filepath.Join(state.PolicyDirectory, "data.json"), document)
	}
	runner.runs = nil
	runner.outputs = nil
	runner.inputs = nil
	if err := runtime.publishPolicyBundle(context.Background(), state); err == nil {
		t.Fatal("publication accepted data drift injected after the first verification")
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if len(runner.runs) != 0 || len(runner.inputs) != 0 {
		t.Fatalf("data drift reached Docker-owned staging: runs=%v inputs=%d", runner.runs, len(runner.inputs))
	}
}

func TestFinalAggregatePolicyPublicationRejectsDataDriftBeforeDockerStaging(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	state := aggregatePolicyTestState(t, runtime)
	projection := FinalAggregateProjection{
		AggregateRevision: state.AggregateRevision, PolicyDirectory: state.PolicyDirectory,
		GatewayConfig: state.GatewayConfig, EvaluatorIdentity: state.EvaluatorIdentity,
		PolicyDataIdentity: state.PolicyDataIdentity,
	}
	original, err := os.ReadFile(filepath.Join(state.PolicyDirectory, "data.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hookErr error
	runtime.policyBeforeBundleAssembly = func() {
		var document map[string]any
		if err := json.Unmarshal(original, &document); err != nil {
			hookErr = err
			return
		}
		document["unexpected"] = true
		hookErr = writeAtomicJSON(filepath.Join(state.PolicyDirectory, "data.json"), document)
	}
	runner.runs = nil
	runner.outputs = nil
	if err := runtime.ApplyFinalAggregatePolicy(context.Background(), projection); err == nil {
		t.Fatal("final publication accepted data drift injected after the first verification")
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	for _, call := range runner.runs {
		joined := strings.Join(call.args, "\n")
		if strings.Contains(joined, "/bundle/.source-"+state.AggregateRevision) {
			t.Fatalf("final data drift reached Docker-owned source staging: %v", call.args)
		}
	}
	for _, call := range runner.outputs {
		joined := strings.Join(call.args, "\n")
		if strings.Contains(joined, ".candidate-"+state.AggregateRevision+".tar.gz") {
			t.Fatalf("final data drift reached Docker-owned publication: %v", call.args)
		}
	}
}

func TestParseGatewayDenialsFiltersUnrelatedAndAllowedLines(t *testing.T) {
	t.Parallel()
	data := []byte(
		"mitmproxy startup\n" +
			`{"decision":"allow","host":"api.github.com"}` + "\n" +
			denyAuditLine + "\n",
	)
	result := parseGatewayDenials(data)
	if len(result.Items) != 1 || result.UnparsedLines != 0 || result.Items[0].Host != "api.github.com" ||
		result.Items[0].ProjectID != "01912345-6789-7abc-8def-0123456789ab" ||
		result.Items[0].WorkspaceManifestID != "01912345-6789-7abc-8def-0123456789ad" ||
		result.Items[0].WorkspaceManifestName != "default" ||
		result.Items[0].Scheme != "https" ||
		result.Items[0].Path != "/repos/cli/cli" || result.Items[0].StatusCode != 403 ||
		!result.Items[0].Learnable {
		t.Fatalf("denials = %+v", result)
	}
	projected, err := json.Marshal(result.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	visible := string(projected)
	for _, required := range []string{`"workspace_id"`, `"workspace_manifest_id"`, `"workspace_manifest"`} {
		if !strings.Contains(visible, required) {
			t.Errorf("public denial projection %s lacks %s", visible, required)
		}
	}
	for _, forbidden := range []string{`"project_id"`, `"context_id"`, `"context"`} {
		if strings.Contains(visible, forbidden) {
			t.Errorf("public denial projection %s leaked Gateway wire key %s", visible, forbidden)
		}
	}
}

func TestParseGatewayDenialsRejectsRenamedGatewayIdentityAliases(t *testing.T) {
	t.Parallel()
	renamed := strings.NewReplacer(
		`"project_id":`, `"workspace_id":`,
		`"context_id":`, `"workspace_manifest_id":`,
		`"context":`, `"workspace_manifest":`,
	).Replace(denyAuditLine)
	result := parseGatewayDenials([]byte(renamed + "\n"))
	if len(result.Items) != 0 || result.UnparsedLines != 1 {
		t.Fatalf("renamed Gateway audit aliases entered domain projection: %+v", result)
	}
}

func TestParseGatewayDenialsPreservesGraphQLRootIdentity(t *testing.T) {
	t.Parallel()
	result := parseGatewayDenials([]byte(graphqlDenyAuditLine + "\n"))
	if len(result.Items) != 1 || result.UnparsedLines != 0 || result.Items[0].EffectiveProtocol() != tobari.PolicyProtocolGraphQL ||
		result.Items[0].GraphQLOperationType != tobari.GraphQLOperationMutation ||
		result.Items[0].GraphQLRootField != "updateIssue" {
		t.Fatalf("GraphQL denial = %+v", result)
	}
}

func TestParseGatewayDenialsPreservesOnlyMCPSemanticIdentity(t *testing.T) {
	t.Parallel()
	result := parseGatewayDenials([]byte(mcpDenyAuditLine + "\n"))
	if len(result.Items) != 1 || result.UnparsedLines != 0 || result.Items[0].EffectiveProtocol() != tobari.PolicyProtocolMCP || result.Items[0].MCPMethod != "tools/call" || result.Items[0].MCPToolName != "codex_apps.search" {
		t.Fatalf("MCP denial = %+v", result)
	}
	if strings.Contains(mcpDenyAuditLine, "arguments") {
		t.Fatal("MCP audit retained arguments")
	}
}

func TestParseGatewayDenialsPreservesOnlyAWSWireOperationIdentity(t *testing.T) {
	t.Parallel()
	result := parseGatewayDenials([]byte(awsDenyAuditLine + "\n"))
	if len(result.Items) != 1 || result.UnparsedLines != 0 || result.Items[0].EffectiveProtocol() != tobari.PolicyProtocolAWS ||
		result.Items[0].AWSWireProtocol != tobari.AWSWireProtocolQuery || result.Items[0].AWSService != "sts" ||
		result.Items[0].AWSOperation != "GetCallerIdentity" {
		t.Fatalf("AWS denial = %+v", result)
	}
	for _, forbidden := range []string{"Resource", "arn:", "credential"} {
		if strings.Contains(awsDenyAuditLine, forbidden) {
			t.Fatalf("AWS audit retained request parameter %q", forbidden)
		}
	}
}

func TestParseGatewayDenialsIsolatesUnprojectableDenialsAndKeepsValidEvidence(t *testing.T) {
	t.Parallel()
	jiraComment := strings.NewReplacer(
		`"host":"api.github.com"`, `"host":"api.atlassian.com"`,
		`"method":"GET"`, `"method":"POST"`,
		`"path":"/repos/cli/cli"`, `"path":"/rest/api/3/issue/EXAMPLE-1/comment"`,
	).Replace(denyAuditLine)
	oidcToken := strings.NewReplacer(
		`"host":"api.github.com"`, `"host":"oidc.ap-northeast-1.amazonaws.com"`,
		`"method":"GET"`, `"method":"POST"`,
		`"path":"/repos/cli/cli"`, `"path":"/token"`,
		`7185da2688d7469aae9cd9068e920b0b`, `8185da2688d7469aae9cd9068e920b0b`,
	).Replace(denyAuditLine)
	secondOIDCToken := strings.Replace(
		oidcToken, `8185da2688d7469aae9cd9068e920b0b`, `9185da2688d7469aae9cd9068e920b0b`, 1,
	)
	result := parseGatewayDenials([]byte(
		jiraComment + "\n" +
			unregisteredPrincipalDenyAuditLine + "\n" +
			oidcToken + "\n" +
			secondOIDCToken + "\n",
	))
	if len(result.Items) != 3 || result.UnparsedLines != 1 ||
		result.Items[0].Host != "api.atlassian.com" ||
		result.Items[1].Host != "oidc.ap-northeast-1.amazonaws.com" ||
		result.Items[2].Path != "/token" {
		t.Fatalf("denial read = %+v", result)
	}
}

func TestParseGatewayDenialsCountsMalformedDenyWithoutReflectingIt(t *testing.T) {
	t.Parallel()
	result := parseGatewayDenials([]byte(
		`{"decision":"deny","host":"api.github.com","authorization":"secret"}` + "\n",
	))
	if len(result.Items) != 0 || result.UnparsedLines != 1 {
		t.Fatalf("denial read = %+v", result)
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
		WorkspaceManifestID: route.ContextID, WorkspaceManifestName: route.ContextPresentation, ProjectID: route.WorkspaceID, ProjectRoot: route.ProjectRoot,
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
	state := aggregatePolicyTestState(t, runtime)
	runner.outputs = nil
	runner.runs = nil
	if err := runtime.ApplyPolicy(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	var build, publish, confirm []string
	for _, call := range runner.outputs {
		joined := strings.Join(call.args, "\n")
		switch {
		case slices.Contains(call.args, "build"):
			build = call.args
		case slices.Contains(call.args, "tobari-policy-publish"):
			publish = call.args
		case slices.Contains(call.args, "exec") && strings.Contains(joined, state.AggregateRevision):
			confirm = call.args
		}
	}
	if len(build) == 0 || len(publish) == 0 || len(confirm) == 0 {
		t.Fatalf("missing Docker-only policy lifecycle calls: %v", runner.outputs)
	}
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
	if !slices.Contains(publish, "--entrypoint") || !slices.Contains(publish, "sh") ||
		!slices.Contains(publish, "/bundle/bundle.tar.gz") ||
		!strings.Contains(strings.Join(publish, "\n"), ".candidate-"+state.AggregateRevision+".tar.gz") {
		t.Fatalf("atomic bundle publication argv = %v", publish)
	}
	if len(runner.runs) < 2 {
		t.Fatalf("policy source staging calls = %v", runner.runs)
	}
	hasTestStage, hasSourceStage := false, false
	for _, stage := range runner.runs {
		staging := strings.Join(stage.args, "\n")
		if !strings.Contains(staging, "tobari-policy-stage") ||
			!strings.Contains(staging, "--interactive") || strings.Contains(staging, "type=bind") {
			t.Fatalf("policy source staging argv = %v", stage.args)
		}
		hasTestStage = hasTestStage || strings.Contains(staging, "/bundle/.test-")
		hasSourceStage = hasSourceStage || strings.Contains(staging, "/bundle/.source-")
	}
	if !hasTestStage || !hasSourceStage {
		t.Fatalf("policy staging omitted test/source stages: %v", runner.runs)
	}
	for _, call := range runner.outputs {
		if slices.Contains(call.args, "compose") || slices.Contains(call.args, "--force-recreate") {
			t.Fatalf("policy activation unexpectedly recreates a service: %v", call.args)
		}
	}
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
	bootstrap := &recordingRunner{}
	runtime, _ := newRuntime(root+"/config", root+"/state", bootstrap)
	state := aggregatePolicyTestState(t, runtime)
	runner := &recordingRunner{outputData: []byte("FAIL"), outputErr: errors.New("exit 2")}
	runtime.runner = runner
	err := runtime.ApplyPolicy(context.Background(), state)
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
	bootstrap := &recordingRunner{}
	runtime, _ := newRuntime(root+"/config", root+"/state", bootstrap)
	state := aggregatePolicyTestState(t, runtime)
	runner := &bundleBuildFailureRunner{}
	runtime.runner = runner
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
