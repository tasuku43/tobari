package dockerruntime

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

type policyContentTestRunner struct {
	recordingRunner
	opaTests          int
	rejected          int
	rejectPolicyTests bool
}

func TestOrthogonalReadinessRemainsBehindTerminalGuardrails(t *testing.T) {
	router, err := aggregateRouter()
	if err != nil {
		t.Fatal(err)
	}
	text := string(router)
	terminal := strings.Index(text, `decision := {"allow": false, "reason": "denied by Context policy ceiling"`)
	grant := strings.Index(text, `decision := {"allow": true, "reason": "allowed by Context policy"`)
	if terminal < 0 || grant < 0 || terminal > grant {
		t.Fatalf("readiness can precede terminal policy ceiling:\n%s", text)
	}
}

func TestAggregateRouterPublishesAtomicPermissionWaitObservation(t *testing.T) {
	t.Parallel()
	router, err := aggregateRouter()
	if err != nil {
		t.Fatal(err)
	}
	want := `permission_wait_observation := {"revision": data.tobari.aggregate_revision, "decision": decision}`
	if strings.Count(string(router), want) != 1 {
		t.Fatalf("atomic permission observation rule count drifted:\n%s", router)
	}
}

func (r *policyContentTestRunner) Output(ctx context.Context, args, environment []string) ([]byte, error) {
	output, err := r.recordingRunner.Output(ctx, args, environment)
	if !containsArg(args, "test") || len(r.inputs) == 0 {
		return output, err
	}
	files, archiveErr := policyTestArchiveFiles(r.inputs[len(r.inputs)-1])
	if archiveErr != nil {
		return nil, archiveErr
	}
	tests, ok := files["tobari_test.rego"]
	if !ok {
		return output, err
	}
	r.opaTests++
	if r.rejectPolicyTests {
		if bytes.Contains(files["data.json"], []byte("/invalid")) {
			r.rejected++
			return []byte("FAIL"), errors.New("opa rejected invalid candidate")
		}
	}
	if bytes.Contains(tests, []byte("INVALID_CANDIDATE")) {
		r.rejected++
		return []byte("FAIL"), errors.New("opa rejected invalid candidate")
	}
	return output, err
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func containsArgPrefix(args []string, prefix string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

func policyTestArchiveFiles(data []byte) (map[string][]byte, error) {
	files := make(map[string][]byte)
	reader := tar.NewReader(bytes.NewReader(data))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return files, nil
		}
		if err != nil {
			return nil, err
		}
		contents, err := io.ReadAll(reader)
		if err != nil {
			return nil, err
		}
		files[header.Name] = contents
	}
}

func TestFixedEvaluatorModuleUsesContextDataNamespace(t *testing.T) {
	t.Parallel()
	transformed, err := fixedEvaluatorModule()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(transformed, []byte("package tobari.system.guided")) ||
		!bytes.Contains(transformed, []byte("data.tobari_contexts[input.principal.context_id]")) ||
		bytes.Contains(transformed, []byte("package tobari.contexts")) {
		t.Fatalf("built-in evaluator was not bound to the system/context data namespace:\n%s", transformed)
	}
}

func TestCanonicalEvaluatorModuleRemainsThePreflightTestTarget(t *testing.T) {
	t.Parallel()
	canonical, err := canonicalEvaluatorModule()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(canonical, []byte("package tobari.http")) ||
		bytes.Contains(canonical, []byte("package tobari.system")) ||
		!bytes.Contains(canonical, []byte("data.tobari.rules")) {
		t.Fatalf("preflight evaluator escaped its canonical source namespace:\n%s", canonical)
	}
}

func TestAggregateSeparatesEvaluatorAndPolicyDataIdentity(t *testing.T) {
	canonical, err := runtimeassets.Read("opa/policy/tobari.rego")
	if err != nil {
		t.Fatal(err)
	}
	manifest := tobari.WorkspaceManifest{
		SchemaVersion: tobari.WorkspaceManifestSchemaVersion, ID: "01912345-6789-7abc-8def-0123456789ad", Name: "restricted",
		AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector,
		SourceAccess: tobari.ManifestSourceAccessReadWrite, PolicyRevision: tobari.DefaultContextPolicyRevision(),
		NativeReadiness: tobari.ManifestNativeReadinessEnabled,
	}
	items := []aggregateContext{{
		contextID: manifest.ID, presentation: manifest.Name,
		manifest: manifest, data: map[string]any{"policy": map[string]any{"effect": "review"}},
	}}
	evaluatorA, dataA, err := aggregateIdentities(items)
	if err != nil {
		t.Fatal(err)
	}
	evaluatorMaterial, err := aggregateEvaluatorMaterial()
	if err != nil {
		t.Fatal(err)
	}
	if evaluatorA != policyEvaluatorIdentityForBytes(evaluatorMaterial) {
		t.Fatalf("aggregate evaluator identity is not bound to its complete fixed material: %+v", evaluatorA)
	}
	revisionA, err := aggregateRevision(items)
	if err != nil {
		t.Fatal(err)
	}
	items[0].data["policy"].(map[string]any)["effect"] = "allow"
	evaluatorB, dataB, err := aggregateIdentities(items)
	if err != nil {
		t.Fatal(err)
	}
	revisionB, err := aggregateRevision(items)
	if err != nil {
		t.Fatal(err)
	}
	if evaluatorA != evaluatorB {
		t.Fatalf("policy-data change altered evaluator identity: before=%+v after=%+v", evaluatorA, evaluatorB)
	}
	if dataA == dataB || revisionA == revisionB {
		t.Fatalf("policy-data change did not alter independent data/aggregate identity: data=%+v/%+v revision=%s/%s", dataA, dataB, revisionA, revisionB)
	}
	evaluatorC := policyEvaluatorIdentityForBytes(append(append([]byte{}, canonical...), '\n'))
	_, dataC, err := aggregateIdentities(items)
	if err != nil {
		t.Fatal(err)
	}
	if dataC != dataB {
		t.Fatalf("evaluator change altered policy-data identity: data=%+v/%+v", dataB, dataC)
	}
	if evaluatorC == evaluatorB {
		t.Fatalf("evaluator change did not alter evaluator identity: evaluator=%+v/%+v", evaluatorB, evaluatorC)
	}
	revisionC, err := aggregateRevisionForIdentities(items, evaluatorC, dataB)
	if err != nil {
		t.Fatal(err)
	}
	if revisionC == revisionB {
		t.Fatalf("evaluator change did not alter aggregate identity: revision=%s/%s", revisionB, revisionC)
	}
}

func TestAggregateEvaluatorMaterialMatchesMountedFixedModules(t *testing.T) {
	t.Parallel()
	router, module, err := fixedAggregateEvaluatorModules()
	if err != nil {
		t.Fatal(err)
	}
	material, err := aggregateEvaluatorMaterial()
	if err != nil {
		t.Fatal(err)
	}
	want := aggregateEvaluatorMaterialForModules(router, module)
	if !bytes.Equal(material, want) {
		t.Fatal("aggregate evaluator identity material drifted from the fixed mounted modules")
	}
	evaluator, _, err := aggregateIdentities(nil)
	if err != nil {
		t.Fatal(err)
	}
	if evaluator != policyEvaluatorIdentityForBytes(want) {
		t.Fatalf("aggregate evaluator identity is not the hash of mounted modules: %+v", evaluator)
	}
}

func TestAggregateEvaluatorIdentityIgnoresContextMembershipAndData(t *testing.T) {
	t.Parallel()
	first := aggregateContext{
		contextID: "01912345-6789-7abc-8def-0123456789ad",
		data:      map[string]any{"policy": map[string]any{"effect": "review"}},
	}
	second := aggregateContext{
		contextID: "01912345-6789-7abc-8def-0123456789ae",
		data:      map[string]any{"policy": map[string]any{"effect": "allow"}},
	}
	emptyEvaluator, _, err := aggregateIdentities(nil)
	if err != nil {
		t.Fatal(err)
	}
	oneEvaluator, oneData, err := aggregateIdentities([]aggregateContext{first})
	if err != nil {
		t.Fatal(err)
	}
	twoEvaluator, twoData, err := aggregateIdentities([]aggregateContext{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if emptyEvaluator != oneEvaluator || oneEvaluator != twoEvaluator {
		t.Fatalf("Context membership or IDs altered evaluator identity: empty=%+v one=%+v two=%+v", emptyEvaluator, oneEvaluator, twoEvaluator)
	}
	if oneData == twoData {
		t.Fatalf("Context membership/data did not alter policy-data identity: one=%+v two=%+v", oneData, twoData)
	}
}

func TestAggregateEvaluatorIdentityChangesWithEitherFixedModule(t *testing.T) {
	t.Parallel()
	router, module, err := fixedAggregateEvaluatorModules()
	if err != nil {
		t.Fatal(err)
	}
	base := policyEvaluatorIdentityForBytes(aggregateEvaluatorMaterialForModules(router, module))
	changedRouter := append(append([]byte{}, router...), '\n')
	changedModule := append(append([]byte{}, module...), '\n')
	if got := policyEvaluatorIdentityForBytes(aggregateEvaluatorMaterialForModules(changedRouter, module)); got == base {
		t.Fatalf("router byte change did not alter evaluator identity: %+v", got)
	}
	if got := policyEvaluatorIdentityForBytes(aggregateEvaluatorMaterialForModules(router, changedModule)); got == base {
		t.Fatalf("fixed evaluator byte change did not alter evaluator identity: %+v", got)
	}
}

func TestAggregateRouterAlwaysUsesSystemEvaluatorForGraphQL(t *testing.T) {
	t.Parallel()
	router, err := aggregateRouter()
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`object.get(input.request, "graphql", null) != null`,
		`result := data.tobari.system.guided.decision`,
		`object.get(input.request, "graphql", null) == null`,
		`object.get(input.request, "aws", null) != null`,
		`object.get(input.request, "aws", null) == null`,
	} {
		if !bytes.Contains(router, []byte(required)) {
			t.Fatalf("aggregate router omitted %q:\n%s", required, router)
		}
	}
	if bytes.Contains(router, []byte("data.tobari.contexts.c0191234567897abc8def0123456789ad.http.decision")) {
		t.Fatalf("aggregate router retained a user-owned evaluator route:\n%s", router)
	}
	if strings.Count(string(router), `object.get(input.request, "aws", null) == null`) < 3 {
		t.Fatalf("aggregate router does not exclude AWS from every coarse HTTP path:\n%s", router)
	}
}

func TestAggregateRouterKeepsHostLoopbackAuthorityAttachmentScoped(t *testing.T) {
	t.Parallel()
	router, err := aggregateRouter()
	if err != nil {
		t.Fatal(err)
	}
	text := string(router)
	for _, required := range []string{`kind == "host_loopback"`, `input.request.authority.scheme == "http"`, `input.request.authority.host == "host.tobari.internal"`, `input.request.authority.port >= 1024`, `input.request.authority.port <= 65535`, `host_loopback_identity_valid`, `^att_[0-9a-f]{32}$`, `grant.lifetime == "attachment"`, `grant.attachment_epoch_id == input.destination.attachment_epoch_id`, `grant.target_port == input.request.authority.port`, `"Host Loopback requires attachment policy review"`, `not host_loopback_request`} {
		if !strings.Contains(text, required) {
			t.Fatalf("Host Loopback router omitted %q:\n%s", required, text)
		}
	}
	if strings.Contains(text, `grant.lifetime == "persistent"`) {
		t.Fatalf("Host Loopback router accepts persistent grant:\n%s", text)
	}
	for _, marker := range []string{
		`"reason": "denied by attachment policy"`,
		`"reason": "allowed by attachment policy"`,
		`"reason": "Host Loopback requires attachment policy review"`,
	} {
		position := strings.Index(text, marker)
		if position < 0 {
			t.Fatalf("Host Loopback decision omitted %q:\n%s", marker, text)
		}
		start := strings.LastIndex(text[:position], "decision :=")
		end := strings.Index(text[position:], "\n\n")
		if start < 0 || end < 0 {
			t.Fatalf("Host Loopback decision framing omitted for %q:\n%s", marker, text)
		}
		clause := text[start : position+end]
		for _, ordinaryAuthority := range []string{"terminal_policy", "exact_denied", "context_policy_granted", "tobari.system.guided", "tobari.contexts."} {
			if strings.Contains(clause, ordinaryAuthority) {
				t.Fatalf("Host Loopback decision %q imported ordinary authority %q:\n%s", marker, ordinaryAuthority, clause)
			}
		}
	}
	for _, marker := range []string{
		`"reason": "denied by Context policy ceiling"`,
		`"reason": "denied by exact policy"`,
		`"reason": "allowed by Context policy"`,
	} {
		position := strings.Index(text, marker)
		if position < 0 {
			t.Fatalf("ordinary decision omitted %q:\n%s", marker, text)
		}
		start := strings.LastIndex(text[:position], "decision :=")
		end := strings.Index(text[position:], "\n\n")
		if start < 0 || end < 0 || !strings.Contains(text[start:position+end], "not host_loopback_request") {
			t.Fatalf("ordinary decision %q can consume attachment traffic:\n%s", marker, text)
		}
	}
}

func TestAggregateRouterMakesContextPolicyCeilingTerminalBeforeBuiltinEvaluator(t *testing.T) {
	t.Parallel()
	router, err := aggregateRouter()
	if err != nil {
		t.Fatal(err)
	}
	text := string(router)
	terminal := strings.Index(text, `decision := {"allow": false, "reason": "denied by Context policy ceiling"`)
	evaluator := strings.LastIndex(text, "decision := result if {")
	if terminal < 0 || evaluator < 0 || terminal > evaluator {
		t.Fatalf("guardrail is not declared before the built-in evaluator:\n%s", text)
	}
	evaluatorClause := text[evaluator:]
	for _, required := range []string{"not terminal_policy", "not exact_denied", "not context_policy_granted"} {
		if !strings.Contains(evaluatorClause, required) {
			t.Fatalf("built-in evaluator route can bypass %q:\n%s", required, evaluatorClause)
		}
	}
	for _, required := range []string{`context_policy_method_decision == "deny"`, `override.method == input.request.method`, `policy.method_default`} {
		if !strings.Contains(text, required) {
			t.Fatalf("method policy guardrail omitted %q:\n%s", required, text)
		}
	}
}

func TestAggregateRouterMakesExactDenyTerminalOverAgentReadyBaseline(t *testing.T) {
	t.Parallel()
	router, err := aggregateRouter()
	if err != nil {
		t.Fatal(err)
	}
	text := string(router)
	deny := strings.Index(text, `decision := {"allow": false, "reason": "denied by exact policy"`)
	grant := strings.Index(text, `decision := {"allow": true, "reason": "allowed by Context policy"`)
	if deny < 0 || grant < 0 || deny > grant {
		t.Fatalf("exact Deny is not declared before baseline grant:\n%s", text)
	}
	grantClause := text[grant:]
	if !strings.Contains(grantClause, "not exact_denied") || !strings.Contains(grantClause, "context_policy_granted") {
		t.Fatalf("agent-ready baseline can bypass exact Deny:\n%s", grantClause)
	}
}

func TestAggregateRouterKeepsGitHubGraphQLBaselineSemanticAndAllRootsExact(t *testing.T) {
	t.Parallel()
	router, err := aggregateRouter()
	if err != nil {
		t.Fatal(err)
	}
	text := string(router)
	for _, required := range []string{
		`rule.graphql_operation_type == input.request.graphql.operation_type`,
		`rule.graphql_root_field == root_field`,
		`count(input.request.graphql.root_fields) > 0`,
		`every root_field in input.request.graphql.root_fields`,
		`object.get(rule, "protocol", "http") == "http"`,
		`some rule in data.tobari_contexts[input.principal.context_id].policy.baseline_grants; rule.protocol == "graphql"`,
		`exact_denied if { learned_graphql_denied }`,
		`context_policy_granted if { context_policy_graphql_granted }`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("GraphQL baseline router omitted %q:\n%s", required, text)
		}
	}
	if strings.Contains(text, `context_policy_exact_granted if { some rule`) {
		t.Fatalf("ordinary HTTP baseline lost its semantic exclusion:\n%s", text)
	}
}

func TestAggregateRouterKeepsGitSmartHTTPOutsideBroadHTTPAuthority(t *testing.T) {
	t.Parallel()
	router, err := aggregateRouter()
	if err != nil {
		t.Fatal(err)
	}
	text := string(router)
	for _, required := range []string{
		`object.get(input.request, "git", null) == null`,
		`exact_denied if { learned_git_denied }`,
		`object.get(input.request, "git", null) != null`,
		`rule.git_service == input.request.git.service`,
		`rule.git_repository == input.request.git.repository`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Git Smart HTTP router omitted %q:\n%s", required, text)
		}
	}
}

func TestAggregateRouterKeepsOCIDistributionOutsideBroadHTTPAuthority(t *testing.T) {
	t.Parallel()
	router, err := aggregateRouter()
	if err != nil {
		t.Fatal(err)
	}
	text := string(router)
	for _, required := range []string{
		`object.get(input.request, "oci", null) == null`,
		`exact_denied if { learned_oci_denied }`,
		`object.get(input.request, "oci", null) != null`,
		`rule.oci_action == input.request.oci.action`,
		`rule.oci_repository == input.request.oci.repository`,
		`rule.oci_object == input.request.oci.object`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("OCI Distribution router omitted %q:\n%s", required, text)
		}
	}
}

func TestAggregateRouterMakesBuiltinHTTPSCeilingTerminalBeforeBuiltinEvaluator(t *testing.T) {
	t.Parallel()
	router, err := aggregateRouter()
	if err != nil {
		t.Fatal(err)
	}
	text := string(router)
	if strings.Contains(text, "guardrail.custom") {
		t.Fatalf("default policy ceiling still depends on a custom-only marker:\n%s", text)
	}
	if !strings.Contains(text, `destination_mode == "public_https"; input.request.authority.scheme != "https"`) {
		t.Fatalf("default policy does not terminally reject plain HTTP:\n%s", text)
	}
	evaluator := strings.LastIndex(text, "decision := result if {")
	if evaluator < 0 {
		t.Fatalf("built-in evaluator route is missing:\n%s", text)
	}
	evaluatorClause := text[evaluator:]
	if !strings.Contains(evaluatorClause, "not terminal_policy") {
		t.Fatalf("built-in evaluator can bypass the default policy ceiling:\n%s", evaluatorClause)
	}
	if strings.Contains(text, "data.tobari.contexts.") {
		t.Fatalf("aggregate router retained a user-owned evaluator route:\n%s", text)
	}
}

func TestGatewayProjectionCarriesOnlyValidatedGraphQLEndpoints(t *testing.T) {
	t.Parallel()
	endpoint := tobari.GraphQLEndpoint{Scheme: "https", Host: "api.example.com", Port: 443, Path: "/graphql"}
	projection := rewriteGatewayProjection(aggregateContext{
		manifest:            tobari.WorkspaceManifest{Name: "default", ID: "01912345-6789-7abc-8def-0123456789ad"},
		graphqlEndpoints:    []tobari.GraphQLEndpoint{endpoint},
		kubernetesEndpoints: []tobari.GraphQLEndpoint{{Scheme: "https", Host: "cluster.us-east-1.eks.amazonaws.com", Port: 443, Path: "/"}},
	})
	endpoints, ok := projection["graphql_endpoints"].([]tobari.GraphQLEndpoint)
	if !ok || len(endpoints) != 1 || endpoints[0] != endpoint {
		t.Fatalf("GraphQL endpoint projection = %#v", projection["graphql_endpoints"])
	}
	kubernetes, ok := projection["kubernetes_endpoints"].([]tobari.GraphQLEndpoint)
	if !ok || len(kubernetes) != 1 || kubernetes[0].Host != "cluster.us-east-1.eks.amazonaws.com" {
		t.Fatalf("Kubernetes endpoint projection = %#v", projection["kubernetes_endpoints"])
	}
}

func TestAggregateKubernetesEndpointsRequiresValidatedEKSBootstrap(t *testing.T) {
	manifest := tobari.WorkspaceManifest{Bootstrap: &tobari.ManifestBootstrapSnapshot{EKS: &tobari.ManifestEKSBootstrap{
		WorkspaceManifestName: "engineering", ClusterName: "platform", Region: "ap-northeast-1",
		Server: "https://abc.gr7.ap-northeast-1.eks.amazonaws.com", CertificateAuthorityData: syntheticEKSCA(t),
	}}}
	endpoints, err := aggregateKubernetesEndpoints(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if len(endpoints) != 1 || endpoints[0] != (tobari.GraphQLEndpoint{
		Scheme: "https", Host: "abc.gr7.ap-northeast-1.eks.amazonaws.com", Port: 443, Path: "/",
	}) {
		t.Fatalf("Kubernetes endpoints = %#v", endpoints)
	}
	manifest.Bootstrap.EKS.Server = "https://proxy.example.com"
	if _, err := aggregateKubernetesEndpoints(manifest); err == nil {
		t.Fatal("expected non-EKS endpoint rejection")
	}
}

func TestAggregateGraphQLEndpointsIncludesContextPolicySnapshotInExactBoundary(t *testing.T) {
	t.Parallel()
	shared := tobari.GraphQLEndpoint{Scheme: "https", Host: "api.example.com", Port: 443, Path: "/graphql"}
	presetOnly := tobari.ManifestPolicyExactRule{
		Scheme: "https", Host: "graphql.example.com", Port: 8443, Method: "POST", Path: "/v1/graphql",
	}
	endpoints, err := aggregateGraphQLEndpoints(
		[]tobari.GraphQLEndpoint{shared},
		[]tobari.ManifestPolicyExactRule{
			{Scheme: shared.Scheme, Host: shared.Host, Port: shared.Port, Method: "POST", Path: shared.Path},
			presetOnly,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []tobari.GraphQLEndpoint{
		shared,
		{Scheme: presetOnly.Scheme, Host: presetOnly.Host, Port: presetOnly.Port, Path: presetOnly.Path},
	}
	if !reflect.DeepEqual(endpoints, want) {
		t.Fatalf("aggregate GraphQL endpoints = %+v, want %+v", endpoints, want)
	}
	if _, err := aggregateGraphQLEndpoints(nil, []tobari.ManifestPolicyExactRule{{
		Scheme: "https", Host: "api.example.com", Port: 443, Method: "GET", Path: "/graphql",
	}}); err == nil {
		t.Fatal("non-POST preset GraphQL endpoint entered the aggregate boundary")
	}
}

func TestAggregateRejectsContextOwnedRego(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	_, paths, err := runtime.resolveContext(tobari.DefaultManifestName)
	if err != nil {
		t.Fatal(err)
	}
	for _, entryName := range []string{"tobari.rego", "tobari_test.rego"} {
		if _, err := os.Lstat(filepath.Join(paths.PolicyDirectory, entryName)); !os.IsNotExist(err) {
			t.Fatalf("Context unexpectedly owns executable source %q: %v", entryName, err)
		}
	}
	first, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Context policy state has one exact typed-data layout. A stale or manually
	// added local evaluator is unsupported source, not compatibility input to ignore.
	unsupportedPath := filepath.Join(paths.PolicyDirectory, "unsupported.policy")
	if err := os.WriteFile(unsupportedPath, []byte("package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { input.schema_version == 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.buildAggregateProjection(context.Background()); err == nil {
		t.Fatal("Context accepted an unsupported local Rego source")
	}
	projected, err := os.ReadFile(filepath.Join(first.PolicyDirectory, "data.json"))
	if err != nil || bytes.Contains(projected, []byte("input.schema_version == 2")) {
		t.Fatalf("original fixed-evaluator aggregate data was changed: error=%v\n%s", err, projected)
	}
	for _, name := range []string{"router.rego", "guided.rego"} {
		if _, err := os.Lstat(filepath.Join(first.PolicyDirectory, name)); !os.IsNotExist(err) {
			t.Fatalf("aggregate host projection created evaluator source %q: %v", name, err)
		}
	}
}

func TestAggregateIntegrityRejectsRevisionNotDesiredByCurrentBinary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	initializeTestWorkspaceManifest(t, runtime)
	projection, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: filepath.Join(root, "runtime"),
		AggregateRevision: projection.Revision, ManifestCount: projection.ManifestCount,
		PolicyDirectory: projection.PolicyDirectory, GatewayConfig: projection.GatewayConfig,
		AssetVersion: "asset", EvaluatorIdentity: projection.EvaluatorIdentity, PolicyDataIdentity: projection.PolicyDataIdentity,
	}
	if got := runtime.inspectAggregatePolicyIntegrity(context.Background(), state); got != "valid" {
		t.Fatalf("fresh aggregate integrity = %q", got)
	}
	state.AggregateRevision = strings.Repeat("f", 64)
	if state.AggregateRevision == projection.Revision {
		state.AggregateRevision = strings.Repeat("e", 64)
	}
	if got := runtime.inspectAggregatePolicyIntegrity(context.Background(), state); got != "invalid" {
		t.Fatalf("stale aggregate integrity = %q", got)
	}
}

func TestAggregateIntegrityRejectsStateIdentityDrift(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	initializeTestWorkspaceManifest(t, runtime)
	projection, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: filepath.Join(root, "runtime"),
		AggregateRevision: projection.Revision, ManifestCount: projection.ManifestCount,
		PolicyDirectory: projection.PolicyDirectory, GatewayConfig: projection.GatewayConfig,
		AssetVersion: "asset", EvaluatorIdentity: projection.EvaluatorIdentity, PolicyDataIdentity: projection.PolicyDataIdentity,
	}
	state.EvaluatorIdentity = policyEvaluatorIdentityForBytes([]byte("tampered evaluator"))
	if got := runtime.inspectAggregatePolicyIntegrity(context.Background(), state); got != "invalid" {
		t.Fatalf("tampered state evaluator identity = %q", got)
	}
	state.EvaluatorIdentity = projection.EvaluatorIdentity
	state.PolicyDataIdentity = tobari.PolicyDataIdentity{SchemaVersion: 1, Digest: tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))}
	if got := runtime.inspectAggregatePolicyIntegrity(context.Background(), state); got != "invalid" {
		t.Fatalf("tampered state policy-data identity = %q", got)
	}
}

func TestPersistedAggregateStateRejectsForgedTargetsAndArtifactDrift(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*testing.T, *Runtime, *tobari.State){
		"forged policy path": func(t *testing.T, _ *Runtime, state *tobari.State) {
			state.PolicyDirectory = filepath.Join(filepath.Dir(state.PolicyDirectory), "forged-policy")
			if err := state.ValidateAggregateProjectionRoot(filepath.Dir(filepath.Dir(state.PolicyDirectory))); err == nil {
				t.Fatal("forged policy path passed the state root binding")
			}
		},
		"forged Gateway path": func(t *testing.T, _ *Runtime, state *tobari.State) {
			state.GatewayConfig = filepath.Join(filepath.Dir(state.GatewayConfig), "forged-gateway.json")
			if err := state.ValidateAggregateProjectionRoot(filepath.Dir(filepath.Dir(state.PolicyDirectory))); err == nil {
				t.Fatal("forged Gateway path passed the state root binding")
			}
		},
		"router drift": func(t *testing.T, _ *Runtime, state *tobari.State) {
			path := filepath.Join(state.PolicyDirectory, "router.rego")
			if err := os.WriteFile(path, []byte("package tobari.router\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"data drift with copied metadata": func(t *testing.T, _ *Runtime, state *tobari.State) {
			path := filepath.Join(state.PolicyDirectory, "data.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			contexts := document["tobari_contexts"].(map[string]any)
			for _, value := range contexts {
				value.(map[string]any)["policy"].(map[string]any)["method_default"] = "deny"
				break
			}
			if err := writeAtomicJSON(path, document); err != nil {
				t.Fatal(err)
			}
		},
		"Gateway drift": func(t *testing.T, _ *Runtime, state *tobari.State) {
			path := state.GatewayConfig
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			document["version"] = "drifted"
			if err := writeAtomicJSON(path, document); err != nil {
				t.Fatal(err)
			}
		},
		"policy symlink": func(t *testing.T, _ *Runtime, state *tobari.State) {
			backup := state.PolicyDirectory + ".backup"
			if err := os.Rename(state.PolicyDirectory, backup); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(backup, state.PolicyDirectory); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
			initializeTestWorkspaceManifest(t, runtime)
			projection, err := runtime.buildAggregateProjection(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			state := tobari.State{
				SchemaVersion: 1, RuntimeDirectory: filepath.Join(root, "runtime"),
				AggregateRevision: projection.Revision, ManifestCount: projection.ManifestCount,
				PolicyDirectory: projection.PolicyDirectory, GatewayConfig: projection.GatewayConfig,
				AssetVersion: "asset", EvaluatorIdentity: projection.EvaluatorIdentity,
				PolicyDataIdentity: projection.PolicyDataIdentity,
			}
			mutate(t, runtime, &state)
			if _, _, err := runtime.verifyPersistedAggregateState(context.Background(), state); err == nil {
				t.Fatal("forged or drifted persisted aggregate was accepted")
			}
		})
	}
}

func TestPersistedAggregateStateValidatesBeforePolicyPublication(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	initializeTestWorkspaceManifest(t, runtime)
	projection, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: filepath.Join(root, "runtime"),
		AggregateRevision: projection.Revision, ManifestCount: projection.ManifestCount,
		PolicyDirectory: projection.PolicyDirectory, GatewayConfig: projection.GatewayConfig,
		AssetVersion: "asset", EvaluatorIdentity: projection.EvaluatorIdentity,
		PolicyDataIdentity: projection.PolicyDataIdentity,
	}
	state.PolicyDirectory = filepath.Join(root, "arbitrary-policy")
	runner.runs = nil
	if err := runtime.publishPolicyBundle(context.Background(), state); err == nil {
		t.Fatal("publication accepted a forged persisted policy path")
	}
	if len(runner.runs) != 0 {
		t.Fatalf("forged publication reached Docker: %+v", runner.runs)
	}
}

func TestAggregatePolicyDirectoryVerifierRejectsDrift(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(t *testing.T, projection aggregateProjection, items []aggregateContext)
		valid  bool
	}{
		{name: "valid", valid: true},
		{name: "router drift", mutate: func(t *testing.T, projection aggregateProjection, _ []aggregateContext) {
			path := filepath.Join(projection.PolicyDirectory, "router.rego")
			if err := os.WriteFile(path, []byte("package tobari.router\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "guided drift", mutate: func(t *testing.T, projection aggregateProjection, _ []aggregateContext) {
			path := filepath.Join(projection.PolicyDirectory, "guided.rego")
			if err := os.WriteFile(path, []byte("package tobari.guided\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing module", mutate: func(t *testing.T, projection aggregateProjection, _ []aggregateContext) {
			if err := os.Remove(filepath.Join(projection.PolicyDirectory, "data.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "extra module", mutate: func(t *testing.T, projection aggregateProjection, _ []aggregateContext) {
			if err := os.WriteFile(filepath.Join(projection.PolicyDirectory, "extra.rego"), []byte("package tobari.extra\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "metadata mismatch", mutate: func(t *testing.T, projection aggregateProjection, _ []aggregateContext) {
			path := filepath.Join(projection.PolicyDirectory, "data.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			document["tobari"].(map[string]any)["aggregate_revision"] = strings.Repeat("a", 64)
			if err := writeAtomicJSON(path, document); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "projected data drift with copied metadata", mutate: func(t *testing.T, projection aggregateProjection, items []aggregateContext) {
			path := filepath.Join(projection.PolicyDirectory, "data.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			contextData := document["tobari_contexts"].(map[string]any)[items[0].contextID].(map[string]any)
			contextData["policy"].(map[string]any)["method_default"] = "deny"
			if err := writeAtomicJSON(path, document); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "gateway drift", mutate: func(t *testing.T, projection aggregateProjection, _ []aggregateContext) {
			path := projection.GatewayConfig
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var document map[string]any
			if err := json.Unmarshal(data, &document); err != nil {
				t.Fatal(err)
			}
			document["version"] = "drifted"
			if err := writeAtomicJSON(path, document); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
			initializeTestWorkspaceManifest(t, runtime)
			projection, err := runtime.buildAggregateProjection(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			items, err := runtime.readAggregateContexts(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if test.mutate != nil {
				test.mutate(t, projection, items)
			}
			err = verifyAggregatePolicyDirectory(projection.PolicyDirectory, items, projection)
			if test.valid && err != nil {
				t.Fatalf("valid aggregate rejected: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("drifted aggregate was accepted")
			}
		})
	}
}

func TestAggregateRouterAWSBehaviorInOPA(t *testing.T) {
	versions, err := runtimeassets.Versions()
	if err != nil {
		t.Fatal(err)
	}
	opaImage := versions["OPA_IMAGE"]
	if opaImage == "" {
		t.Skip("OPA image is not configured")
	}
	runner := osCommandRunner{}
	if _, err := runner.Output(context.Background(), []string{"version", "--format", "{{.Server.Version}}"}, os.Environ()); err != nil {
		t.Skipf("Docker Engine unavailable: %v", err)
	}
	router, module, err := fixedAggregateEvaluatorModules()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(".", ".aggregate-opa-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	policyDirectory := filepath.Join(root, "policy")
	if err := os.MkdirAll(policyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string][]byte{
		"router.rego":                router,
		"guided.rego":                module,
		"aggregate_router_test.rego": []byte(aggregateAWSRouterBehaviorTestRego),
	} {
		if err := os.WriteFile(filepath.Join(policyDirectory, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	contextID := "01912345-6789-7abc-8def-0123456789ad"
	data := map[string]any{
		"tobari_contexts": map[string]any{contextID: map[string]any{
			"schema_version": 1,
			"boundary":       map[string]any{"graphql_endpoints": []any{}, "mcp_endpoints": []any{}, "kubernetes_endpoints": []any{}},
			"rules":          map[string]any{"learned_allows": []any{}, "learned_denies": []any{}},
			"policy": map[string]any{
				"destination_mode": "public_https", "authorities": []any{}, "method_default": "allow", "method_overrides": []any{},
				"baseline_grants":    []any{map[string]any{"scheme": "https", "host": "sts.us-east-1.amazonaws.com", "port": 443, "method": "POST", "path": "/"}},
				"baseline_templates": []any{}, "mcp_baseline_grants": []any{}, "baseline_denies": []any{},
			},
		}},
		"tobari": map[string]any{
			"aggregate_schema_version": 1, "aggregate_revision": strings.Repeat("a", 64),
			"evaluator_identity":   map[string]any{"schema_version": 1, "version": "tobari-evaluator-v1", "digest": "sha256:" + strings.Repeat("b", 64)},
			"policy_data_identity": map[string]any{"schema_version": 1, "digest": "sha256:" + strings.Repeat("c", 64)},
		},
	}
	if err := writeAtomicJSON(filepath.Join(policyDirectory, "data.json"), data); err != nil {
		t.Fatal(err)
	}
	uid, gid := currentIDs()
	mount := "type=bind,src=" + root + ",dst=/policy,readonly"
	output, err := runner.Output(context.Background(), []string{
		"run", "--rm", "--user", fmt.Sprintf("%d:%d", uid, gid), "--mount", mount, opaImage, "test", "/policy", "-v",
	}, os.Environ())
	if err != nil {
		t.Fatalf("composed aggregate AWS OPA behavior failed: %v\n%s", err, boundedDiagnostic(output))
	}
}

const aggregateAWSRouterBehaviorTestRego = `package tobari.http

import rego.v1

context_id := "01912345-6789-7abc-8def-0123456789ad"
project_id := "01912345-6789-7abc-8def-0123456789ab"

base_request := {
	"schema_version": 1,
	"principal": {"cluster": "default", "context_id": context_id, "project_id": project_id},
	"request": {
		"authority": {"scheme": "https", "host": "sts.us-east-1.amazonaws.com", "port": 443},
		"method": "POST", "path": {"raw": "/", "segments": []}, "query": {}, "headers": {},
		"aws": {"wire_protocol": "query", "service": "sts", "operation": "GetCallerIdentity"},
	},
	"authorization": {"broker_provider": null},
}

http_baseline_rule := {"scheme": "https", "host": "sts.us-east-1.amazonaws.com", "port": 443, "method": "POST", "path": "/"}

aws_allow_rule := {
	"id": "plr_0123456789abcdef0123456789abcdef", "match": "exact", "context_id": context_id, "project_id": project_id,
	"scheme": "https", "host": "sts.us-east-1.amazonaws.com", "port": 443, "method": "POST", "path": "/",
	"protocol": "aws", "aws_wire_protocol": "query", "aws_service": "sts", "aws_operation": "GetCallerIdentity",
	"examples": ["/"], "source_candidates": ["pcy_0123456789abcdef0123456789abcdef"],
}

aggregate_contexts(baseline_grants, learned_allows, learned_denies) := object.union(
	{},
	{"01912345-6789-7abc-8def-0123456789ad": {
		"schema_version": 1,
		"boundary": {"graphql_endpoints": [], "mcp_endpoints": [], "kubernetes_endpoints": []},
		"rules": {"learned_allows": learned_allows, "learned_denies": learned_denies},
		"policy": {
			"destination_mode": "public_https", "authorities": [], "method_default": "allow", "method_overrides": [],
			"baseline_grants": baseline_grants, "baseline_templates": [], "mcp_baseline_grants": [], "baseline_denies": [],
		},
	}},
)

test_coarse_http_baseline_does_not_allow_classified_aws if {
	result := decision with input as base_request
		with data.tobari_contexts as aggregate_contexts([http_baseline_rule], [], [])
	not result.allow
}

test_matching_learned_aws_allow_is_required_and_works if {
	result := decision with input as base_request
		with data.tobari_contexts as aggregate_contexts([http_baseline_rule], [aws_allow_rule], [])
	result.allow
	evidence := decision_evidence with input as base_request
		with data.tobari_contexts as aggregate_contexts([http_baseline_rule], [aws_allow_rule], [])
	evidence.decision == result
	evidence.policy_layer == "learned_allow"
	evidence.rule_refs == [aws_allow_rule.id]
	evidence.default_overridden
	evidence.semantic_effect.protocol == "aws"
	evidence.semantic_effect.coordinates == {"wire_protocol": "query", "service": "sts", "operation": "GetCallerIdentity"}
}

test_matching_learned_aws_deny_is_terminal if {
	deny := object.union(aws_allow_rule, {"id": "pdr_0123456789abcdef0123456789abcdef"})
	result := decision with input as base_request
		with data.tobari_contexts as aggregate_contexts([http_baseline_rule], [aws_allow_rule], [deny])
	not result.allow
	not result.learnable
	evidence := decision_evidence with input as base_request
		with data.tobari_contexts as aggregate_contexts([http_baseline_rule], [aws_allow_rule], [deny])
	evidence.decision == result
	evidence.policy_layer == "learned_deny"
	evidence.rule_refs == [deny.id]
	evidence.default_overridden
}

test_mismatched_aws_coordinate_does_not_fallback_to_http if {
	request := object.union(base_request, {"request": object.union(base_request.request, {"aws": {"wire_protocol": "query", "service": "sts", "operation": "AssumeRole"}})})
	result := decision with input as request
		with data.tobari_contexts as aggregate_contexts([http_baseline_rule], [aws_allow_rule], [])
	not result.allow
	result.learnable
}

test_malformed_aws_coordinate_fails_closed_without_fallback if {
	request := object.union(base_request, {"request": object.union(base_request.request, {"aws": {"wire_protocol": "query", "service": "sts", "operation": "Get-Caller-Identity"}})})
	result := decision with input as request
		with data.tobari_contexts as aggregate_contexts([http_baseline_rule], [], [])
	not result.allow
	not result.learnable
}

test_static_and_terminal_router_evidence_are_exact if {
	http_request := object.remove(base_request.request, {"aws"})
	http_input := object.union(object.remove(base_request, {"request"}), {"request": http_request})
	static := decision_evidence with input as http_input
		with data.tobari_contexts as aggregate_contexts([http_baseline_rule], [], [])
	static.decision.allow
	static.policy_layer == "static_allow"
	static.rule_refs == ["baseline:http:https:sts.us-east-1.amazonaws.com:443:POST:/"]
	static.default_overridden

	contexts := aggregate_contexts([], [], [])
	context := contexts[context_id]
	denied_policy := object.union(context.policy, {"method_default": "deny"})
	denied_contexts := {context_id: object.union(context, {"policy": denied_policy})}
	terminal := decision_evidence with input as http_input
		with data.tobari_contexts as denied_contexts
	not terminal.decision.allow
	terminal.policy_layer == "terminal_ceiling"
	terminal.rule_refs == ["method_default:deny"]
	terminal.default_overridden
}

test_attachment_router_evidence_uses_stable_grant_reference if {
	grant := {
		"id": "pag_0123456789abcdef0123456789abcdef", "decision": "allow", "lifetime": "attachment",
		"destination_kind": "host_loopback", "context_id": context_id, "project_id": project_id,
		"attachment_epoch_id": "att_0123456789abcdef0123456789abcdef", "host": "host.tobari.internal",
		"target_port": 8123, "method": "GET", "path": "/ready", "source_candidate": "pcy_0123456789abcdef0123456789abcdef",
	}
	request := object.union(base_request.request, {
		"authority": {"scheme": "http", "host": "host.tobari.internal", "port": 8123},
		"method": "GET", "path": {"raw": "/ready", "segments": ["ready"]},
	})
	attachment_input := object.union(object.remove(base_request, {"request", "authorization"}), {
		"request": request,
		"destination": {"kind": "host_loopback", "attachment_epoch_id": "att_0123456789abcdef0123456789abcdef"},
		"authorization": {"broker_provider": null, "attachment_grants": [grant]},
	})
	evidence := decision_evidence with input as attachment_input
		with data.tobari_contexts as aggregate_contexts([], [], [])
	evidence.decision.allow
	evidence.policy_layer == "attachment_allow"
	evidence.rule_refs == [grant.id]
	evidence.default_overridden
}`

func TestInvalidContextPolicyDoesNotReplaceKnownGoodAggregate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	knownGood, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(context.Background(), "broken", tobari.BuiltinImageSelector, tobari.ManifestSourceAccessReadWrite); err != nil {
		t.Fatal(err)
	}
	_, paths, err := runtime.resolveContext("broken")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.PolicyDirectory, "unsupported.policy"), []byte("package tobari.system\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.buildAggregateProjection(context.Background()); err == nil {
		t.Fatal("invalid Context policy was accepted")
	}
	if _, err := os.Stat(filepath.Dir(knownGood.PolicyDirectory)); err != nil {
		t.Fatalf("known-good aggregate was removed: %v", err)
	}
}

func TestAggregateReusesOneCandidateReceiptAndRetainsAggregateTests(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	manifest, paths, err := runtime.resolveContext(tobari.DefaultManifestName)
	if err != nil {
		t.Fatal(err)
	}
	original, err := readPolicyData(paths.PolicyDirectory)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := original.withPolicyRules([]tobari.LearnedPolicyRule{
		contextRuleFixture(t, manifest, "01912345-6789-7abc-8def-0123456789ab", "/receipt"),
	}, []tobari.PolicyDenyRule{})
	if err != nil {
		t.Fatal(err)
	}
	validation, err := runtime.testContextPolicyCandidate(
		context.Background(), manifest, paths.PolicyDirectory, candidate,
	)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := beginPolicySourceTransaction(paths.PolicyDirectory, original.sources, candidate.sources)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transaction.rollback() })
	if err := transaction.bindCandidateValidation(validation); err != nil {
		t.Fatal(err)
	}
	transactions := map[string]*policySourceTransaction{paths.PolicyDirectory: transaction}
	projection, err := runtime.buildAggregateProjectionWithTransactions(context.Background(), transactions)
	if err != nil {
		t.Fatal(err)
	}
	if got := opaPolicyTestCallCount(runner.outputs); got != 2 {
		t.Fatalf("OPA tests after first build = %d, want target preflight and aggregate candidate: %v", got, runner.outputs)
	}
	firstTests := policyTestCalls(runner.outputs)
	if len(firstTests) != 2 {
		t.Fatalf("first build did not retain target and aggregate-candidate OPA tests: %v", runner.outputs)
	}
	for _, call := range firstTests {
		if !containsArg(call.args, "test") || !containsArg(call.args, "type=volume,src="+policyBundleVolume+",dst=/bundle") || containsArg(call.args, "type=bind") {
			t.Fatalf("first build used an unsafe OPA test target: %v", call.args)
		}
	}
	if !transaction.candidateValidationConsumed {
		t.Fatal("matching candidate receipt was not consumed")
	}
	if _, err := runtime.buildAggregateProjectionWithTransactions(context.Background(), transactions); err != nil {
		t.Fatal(err)
	}
	if got := opaPolicyTestCallCount(runner.outputs); got != 4 {
		t.Fatalf("OPA tests after rebuild = %d, want per-Context retest and existing aggregate revalidation: %v", got, runner.outputs)
	}
	secondTests := policyTestCalls(runner.outputs)
	if len(secondTests) != 4 {
		t.Fatalf("rebuild did not retain per-Context and existing-aggregate OPA tests: %v", runner.outputs)
	}
	if _, err := os.Stat(projection.PolicyDirectory); err != nil {
		t.Fatalf("validated aggregate projection is unavailable: %v", err)
	}
}

func policyTestCalls(calls []runnerCall) []runnerCall {
	result := make([]runnerCall, 0)
	for _, call := range calls {
		if containsArg(call.args, "test") {
			result = append(result, call)
		}
	}
	return result
}

func TestAggregateReceiptMismatchRetestsAndRejectsInvalidContextPolicy(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &policyContentTestRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(
		context.Background(), "candidate", tobari.BuiltinImageSelector,
		tobari.ManifestSourceAccessReadWrite,
	); err != nil {
		t.Fatal(err)
	}
	manifest, paths, err := runtime.resolveContext("candidate")
	if err != nil {
		t.Fatal(err)
	}
	original, err := readPolicyData(paths.PolicyDirectory)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := original.withPolicyRules([]tobari.LearnedPolicyRule{
		contextRuleFixture(t, manifest, "01912345-6789-7abc-8def-0123456789ab", "/invalid"),
	}, []tobari.PolicyDenyRule{})
	if err != nil {
		t.Fatal(err)
	}
	validation, err := runtime.testContextPolicyCandidate(
		context.Background(), manifest, paths.PolicyDirectory, candidate,
	)
	if err != nil {
		t.Fatal(err)
	}
	// The embedded evaluator is immutable. Simulate a stale validation receipt
	// so aggregate construction must retest the candidate rather than relying
	// on a removed user-authored test module.
	validation.preflightDigest = strings.Repeat("f", 64)
	transaction, err := beginPolicySourceTransaction(paths.PolicyDirectory, original.sources, candidate.sources)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transaction.rollback() })
	if err := transaction.bindCandidateValidation(validation); err != nil {
		t.Fatal(err)
	}
	beforeAggregate := runner.opaTests
	runner.rejectPolicyTests = true
	_, err = runtime.buildAggregateProjectionWithTransactions(
		context.Background(), map[string]*policySourceTransaction{paths.PolicyDirectory: transaction},
	)
	if err == nil || !strings.Contains(err.Error(), `Context "candidate" policy tests`) {
		t.Fatalf("invalid candidate error = %v", err)
	}
	if runner.rejected != 1 || runner.opaTests <= beforeAggregate {
		t.Fatalf("OPA tests=%d rejected=%d calls=%v", runner.opaTests, runner.rejected, runner.outputs)
	}
	if transaction.candidateValidationConsumed {
		t.Fatal("mismatched receipt was consumed instead of re-testing the invalid candidate")
	}
}
