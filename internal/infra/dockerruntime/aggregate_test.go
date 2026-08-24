package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type policyContentTestRunner struct {
	recordingRunner
	opaTests int
	rejected int
}

func TestOrthogonalReadinessRemainsBehindTerminalGuardrails(t *testing.T) {
	manifest := tobari.WorkspaceManifest{SchemaVersion: tobari.WorkspaceManifestSchemaVersion, ID: "01912345-6789-7abc-8def-0123456789ad", Name: "restricted", AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector, PolicyMode: tobari.ManifestPolicyModeGuided, SourceAccess: tobari.ManifestSourceAccessReadWrite, PolicyRevision: tobari.DefaultContextPolicyRevision(), NativeReadiness: tobari.ManifestNativeReadinessEnabled}
	router, err := aggregateRouter([]aggregateContext{{manifest: manifest}})
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
	router, err := aggregateRouter([]aggregateContext{})
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
	policyDirectory, ok := mountedPolicyTestDirectory(args)
	if !ok {
		return output, err
	}
	r.opaTests++
	tests, readErr := os.ReadFile(filepath.Join(policyDirectory, "tobari_test.rego"))
	if readErr != nil {
		return nil, readErr
	}
	if bytes.Contains(tests, []byte("INVALID_CANDIDATE")) {
		r.rejected++
		return []byte("FAIL"), errors.New("opa rejected invalid candidate")
	}
	return output, err
}

func mountedPolicyTestDirectory(args []string) (string, bool) {
	if len(args) < 2 || args[len(args)-2] != "test" || args[len(args)-1] != "/policy" {
		return "", false
	}
	const prefix = "type=bind,src="
	const suffix = ",dst=/policy,readonly"
	for _, argument := range args {
		if strings.HasPrefix(argument, prefix) && strings.HasSuffix(argument, suffix) {
			return strings.TrimSuffix(strings.TrimPrefix(argument, prefix), suffix), true
		}
	}
	return "", false
}

func TestAdvancedPolicyReceivesContextNamespaceAndCannotClaimSystemPackages(t *testing.T) {
	t.Parallel()
	item := aggregateContext{
		manifest: tobari.WorkspaceManifest{
			SchemaVersion:  tobari.WorkspaceManifestSchemaVersion,
			ID:             "01912345-6789-7abc-8def-0123456789ad",
			Name:           "restricted",
			AgentProfile:   tobari.DefaultProfile,
			PolicyMode:     tobari.ManifestPolicyModeAdvanced,
			SourceAccess:   tobari.ManifestSourceAccessReadWrite,
			PolicyRevision: tobari.DefaultContextPolicyRevision(),
			Image:          tobari.BuiltinImageSelector,
		},
		rego: []byte("package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { input.schema_version == 1; data.tobari.schema_version == 1 }\n"),
	}
	transformed, err := transformContextRego(item)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(transformed, []byte("package tobari.contexts.c0191234567897abc8def0123456789ad.http")) ||
		!bytes.Contains(transformed, []byte("input.schema_version == 1")) ||
		!bytes.Contains(transformed, []byte("data.tobari_contexts[input.principal.context_id]")) {
		t.Fatalf("advanced Context policy was not safely namespaced:\n%s", transformed)
	}
	item.rego = []byte("package tobari.system\n")
	if _, err := transformContextRego(item); err == nil {
		t.Fatal("user policy claimed the system package")
	}
}

func TestAggregateRouterAlwaysUsesSystemEvaluatorForGraphQL(t *testing.T) {
	t.Parallel()
	item := aggregateContext{manifest: tobari.WorkspaceManifest{
		SchemaVersion:  tobari.WorkspaceManifestSchemaVersion,
		ID:             "01912345-6789-7abc-8def-0123456789ad",
		Name:           "restricted",
		AgentProfile:   tobari.DefaultProfile,
		PolicyMode:     tobari.ManifestPolicyModeAdvanced,
		SourceAccess:   tobari.ManifestSourceAccessReadWrite,
		PolicyRevision: tobari.DefaultContextPolicyRevision(),
		Image:          tobari.BuiltinImageSelector,
	}}
	router, err := aggregateRouter([]aggregateContext{item})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`object.get(input.request, "graphql", null) != null`,
		`result := data.tobari.system.guided.decision`,
		`object.get(input.request, "graphql", null) == null`,
		`result := data.tobari.contexts.c0191234567897abc8def0123456789ad.http.decision`,
	} {
		if !bytes.Contains(router, []byte(required)) {
			t.Fatalf("aggregate router omitted %q:\n%s", required, router)
		}
	}
}

func TestAggregateRouterKeepsHostLoopbackAuthorityAttachmentScoped(t *testing.T) {
	t.Parallel()
	router, err := aggregateRouter([]aggregateContext{})
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

func TestAggregateRouterMakesContextPolicyCeilingTerminalBeforeAdvancedOrGuidedPolicy(t *testing.T) {
	t.Parallel()
	manifest := tobari.WorkspaceManifest{SchemaVersion: tobari.WorkspaceManifestSchemaVersion, ID: "01912345-6789-7abc-8def-0123456789ad", Name: "restricted", AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector, PolicyMode: tobari.ManifestPolicyModeAdvanced, SourceAccess: tobari.ManifestSourceAccessReadWrite, PolicyRevision: tobari.DefaultContextPolicyRevision()}
	router, err := aggregateRouter([]aggregateContext{{manifest: manifest}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(router)
	terminal := strings.Index(text, `decision := {"allow": false, "reason": "denied by Context policy ceiling"`)
	advanced := strings.Index(text, `result := data.tobari.contexts.c0191234567897abc8def0123456789ad.http.decision`)
	if terminal < 0 || advanced < 0 || terminal > advanced {
		t.Fatalf("guardrail is not declared before Advanced routing:\n%s", text)
	}
	advancedClause := text[strings.LastIndex(text[:advanced], "decision := result if {"):advanced]
	for _, required := range []string{"not terminal_policy", "not exact_denied", "not context_policy_granted"} {
		if !strings.Contains(advancedClause, required) {
			t.Fatalf("Advanced route can bypass %q:\n%s", required, advancedClause)
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
	manifest := tobari.WorkspaceManifest{SchemaVersion: tobari.WorkspaceManifestSchemaVersion, ID: "01912345-6789-7abc-8def-0123456789ad", Name: "agent-ready", AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector, PolicyMode: tobari.ManifestPolicyModeGuided, SourceAccess: tobari.ManifestSourceAccessReadWrite, PolicyRevision: tobari.DefaultContextPolicyRevision()}
	router, err := aggregateRouter([]aggregateContext{{manifest: manifest}})
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
	manifest := tobari.WorkspaceManifest{SchemaVersion: tobari.WorkspaceManifestSchemaVersion, ID: "01912345-6789-7abc-8def-0123456789ad", Name: "agent-ready", AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector, PolicyMode: tobari.ManifestPolicyModeGuided, SourceAccess: tobari.ManifestSourceAccessReadWrite, PolicyRevision: tobari.DefaultContextPolicyRevision()}
	router, err := aggregateRouter([]aggregateContext{{manifest: manifest}})
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
	manifest := tobari.WorkspaceManifest{SchemaVersion: tobari.WorkspaceManifestSchemaVersion, ID: "01912345-6789-7abc-8def-0123456789ad", Name: "agent-ready", AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector, PolicyMode: tobari.ManifestPolicyModeGuided, SourceAccess: tobari.ManifestSourceAccessReadWrite, PolicyRevision: tobari.DefaultContextPolicyRevision()}
	router, err := aggregateRouter([]aggregateContext{{manifest: manifest}})
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
	manifest := tobari.WorkspaceManifest{SchemaVersion: tobari.WorkspaceManifestSchemaVersion, ID: "01912345-6789-7abc-8def-0123456789ad", Name: "agent-ready", AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector, PolicyMode: tobari.ManifestPolicyModeGuided, SourceAccess: tobari.ManifestSourceAccessReadWrite, PolicyRevision: tobari.DefaultContextPolicyRevision()}
	router, err := aggregateRouter([]aggregateContext{{manifest: manifest}})
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

func TestAggregateRouterMakesBuiltinHTTPSCeilingTerminalBeforeAdvancedPolicy(t *testing.T) {
	t.Parallel()
	manifest := tobari.WorkspaceManifest{SchemaVersion: tobari.WorkspaceManifestSchemaVersion, ID: "01912345-6789-7abc-8def-0123456789ad", Name: "restricted", AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector, PolicyMode: tobari.ManifestPolicyModeAdvanced, SourceAccess: tobari.ManifestSourceAccessReadWrite, PolicyRevision: tobari.DefaultContextPolicyRevision()}
	router, err := aggregateRouter([]aggregateContext{{manifest: manifest}})
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
	advanced := strings.Index(text, `result := data.tobari.contexts.c0191234567897abc8def0123456789ad.http.decision`)
	if advanced < 0 {
		t.Fatalf("Advanced route is missing:\n%s", text)
	}
	advancedStart := strings.LastIndex(text[:advanced], "decision := result if {")
	if advancedStart < 0 {
		t.Fatalf("Advanced clause is missing:\n%s", text)
	}
	advancedClause := text[advancedStart:advanced]
	if !strings.Contains(advancedClause, "not terminal_policy") {
		t.Fatalf("Advanced policy can bypass the default policy ceiling:\n%s", advancedClause)
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

func TestAggregateRejectsUnsupportedOrAmbiguousSourceInputSchema(t *testing.T) {
	t.Parallel()
	manifest := tobari.WorkspaceManifest{
		SchemaVersion:  tobari.WorkspaceManifestSchemaVersion,
		ID:             "01912345-6789-7abc-8def-0123456789ad",
		Name:           "restricted",
		AgentProfile:   tobari.DefaultProfile,
		PolicyMode:     tobari.ManifestPolicyModeAdvanced,
		SourceAccess:   tobari.ManifestSourceAccessReadWrite,
		PolicyRevision: tobari.DefaultContextPolicyRevision(),
		Image:          tobari.BuiltinImageSelector,
	}
	for _, source := range []string{
		"package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { input.schema_version == 2 }\n",
		"package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { input.schema_version == 1; input.schema_version == 2 }\n",
	} {
		if _, err := transformContextRego(aggregateContext{manifest: manifest, rego: []byte(source)}); err == nil {
			t.Fatalf("unsupported policy source was accepted:\n%s", source)
		}
	}
}

func TestGuidedAggregateRejectsContextOwnedRego(t *testing.T) {
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
	regoPath := filepath.Join(paths.PolicyDirectory, "tobari.rego")
	if _, err := os.Lstat(regoPath); !os.IsNotExist(err) {
		t.Fatalf("guided Context unexpectedly owns Rego: %v", err)
	}
	first, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Guided mode has one exact source layout. A stale or manually added local
	// evaluator is an unsupported source, not compatibility input to ignore.
	if err := os.WriteFile(regoPath, []byte("package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { input.schema_version == 2 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.buildAggregateProjection(context.Background()); err == nil {
		t.Fatal("guided Context accepted an unsupported local Rego source")
	}
	projected, err := os.ReadFile(filepath.Join(first.PolicyDirectory, "guided.rego"))
	if err != nil || bytes.Contains(projected, []byte("input.schema_version == 2")) {
		t.Fatalf("original guided aggregate was changed: error=%v\n%s", err, projected)
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
		AssetVersion: "asset",
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
	if _, err := runtime.CreateContext(context.Background(), "broken", tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeAdvanced, tobari.ManifestSourceAccessReadWrite); err != nil {
		t.Fatal(err)
	}
	_, paths, err := runtime.resolveContext("broken")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.PolicyDirectory, "tobari.rego"), []byte("package tobari.system\n"), 0o600); err != nil {
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
	firstPolicy, firstOK := mountedPolicyTestDirectory(runner.outputs[0].args)
	secondPolicy, secondOK := mountedPolicyTestDirectory(runner.outputs[1].args)
	if !firstOK || !strings.Contains(filepath.Base(firstPolicy), "preflight-") ||
		!secondOK || !strings.Contains(secondPolicy, filepath.Join("cluster-projections", ".candidate-")) {
		t.Fatalf("first build did not retain target and aggregate-candidate OPA tests: %v", runner.outputs)
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
	thirdPolicy, thirdOK := mountedPolicyTestDirectory(runner.outputs[2].args)
	fourthPolicy, fourthOK := mountedPolicyTestDirectory(runner.outputs[3].args)
	if !thirdOK || !strings.Contains(filepath.Base(thirdPolicy), "preflight-") ||
		!fourthOK || fourthPolicy != projection.PolicyDirectory {
		t.Fatalf("rebuild did not retain per-Context and existing-aggregate OPA tests: %v", runner.outputs)
	}
	if _, err := os.Stat(projection.PolicyDirectory); err != nil {
		t.Fatalf("validated aggregate projection is unavailable: %v", err)
	}
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
		context.Background(), "advanced", tobari.OfficialRuntimeBase,
		tobari.ManifestPolicyModeAdvanced, tobari.ManifestSourceAccessReadWrite,
	); err != nil {
		t.Fatal(err)
	}
	manifest, paths, err := runtime.resolveContext("advanced")
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
	transaction, err := beginPolicySourceTransaction(paths.PolicyDirectory, original.sources, candidate.sources)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transaction.rollback() })
	if err := transaction.bindCandidateValidation(validation); err != nil {
		t.Fatal(err)
	}
	beforeAggregate := runner.opaTests
	invalidTests := []byte("package tobari.http\n\nINVALID_CANDIDATE\n")
	if err := os.WriteFile(filepath.Join(paths.PolicyDirectory, "tobari_test.rego"), invalidTests, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.buildAggregateProjectionWithTransactions(
		context.Background(), map[string]*policySourceTransaction{paths.PolicyDirectory: transaction},
	)
	if err == nil || !strings.Contains(err.Error(), `Context "advanced" policy tests`) {
		t.Fatalf("invalid candidate error = %v", err)
	}
	if runner.rejected != 1 || runner.opaTests <= beforeAggregate {
		t.Fatalf("OPA tests=%d rejected=%d calls=%v", runner.opaTests, runner.rejected, runner.outputs)
	}
	if transaction.candidateValidationConsumed {
		t.Fatal("mismatched receipt was consumed instead of re-testing the invalid candidate")
	}
}
