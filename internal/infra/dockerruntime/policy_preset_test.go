package dockerruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func newPolicyPresetTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &contextSwitchRunner{})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func TestPolicyPresetStoreListsBuiltinsAndCreatesOwnerOnlyCustomWithoutOverwrite(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	listed, err := runtime.ListPolicyPresets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 4 {
		t.Fatalf("fresh preset catalog = %+v", listed.Items)
	}
	if listed.Items[0].Origin != tobari.DefaultPolicyPresetOrigin || listed.Items[0].ImmediateGrantCount == 0 {
		t.Fatalf("agent-ready default is missing from preset catalog: %+v", listed.Items)
	}
	created, err := runtime.InitPolicyPreset(context.Background(), "restricted")
	if err != nil {
		t.Fatal(err)
	}
	if created.Origin != "custom/restricted" || created.Preset == nil || created.Preset.Guardrail != tobari.PolicyPresetGuardrailOffline {
		t.Fatalf("created preset = %+v", created)
	}
	path := filepath.Join(runtime.policyPresetCustomDirectory(), "restricted.json")
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("custom preset mode = %v", info.Mode())
	}
	before, _ := os.ReadFile(path)
	if _, err := runtime.InitPolicyPreset(context.Background(), "restricted"); err == nil {
		t.Fatal("custom preset was overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed init changed existing preset")
	}
}

func TestPolicyPresetListFailsClosedOnUnsafeCustomCatalogEntry(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	if err := runtime.ensurePrivateDirectory(filepath.Dir(runtime.policyPresetCustomDirectory())); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensurePrivateDirectory(runtime.policyPresetCustomDirectory()); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(runtime.policyPresetCustomDirectory(), "unsafe.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListPolicyPresets(context.Background()); err == nil {
		t.Fatal("unsafe custom catalog entry was silently omitted")
	}
}

func TestPolicyPresetStoreRejectsUnknownDuplicateExecutableAndUnsafeSources(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	if _, err := runtime.InitPolicyPreset(context.Background(), "hostile"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runtime.policyPresetCustomDirectory(), "hostile.json")
	fixtures := []string{
		`{"schema_version":1,"name":"hostile","name":"hostile","guardrail":"offline","authorities":[],"methods":[],"baseline_grants":[],"baseline_denies":[],"graphql_endpoints":[]}`,
		`{"schema_version":1,"name":"hostile","guardrail":"offline","authorities":[],"methods":[],"baseline_grants":[],"baseline_denies":[],"graphql_endpoints":[],"rego":"allow=true"}`,
		`{"schema_version":1,"name":"hostile","guardrail":"offline","authorities":[],"methods":[],"baseline_grants":[],"baseline_denies":[],"graphql_endpoints":[],"include":"https://example.invalid/preset"}`,
		`{"schema_version":1,"name":"hostile","guardrail":"reviewed_exact","authorities":[{"scheme":"https","host":"127.0.0.1","port":443}],"methods":["GET"],"baseline_grants":[],"baseline_denies":[],"graphql_endpoints":[]}`,
	}
	for _, fixture := range fixtures {
		if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.ValidatePolicyPreset(context.Background(), "custom/hostile"); err == nil {
			t.Fatalf("hostile preset accepted: %s", fixture)
		}
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ValidatePolicyPreset(context.Background(), "custom/hostile"); err == nil {
		t.Fatal("group/world-readable preset accepted")
	}
}

func TestContextSnapshotsPresetAndIgnoresLaterSourceEdit(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	if _, err := runtime.InitPolicyPreset(context.Background(), "frozen"); err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateContextWithPreset(context.Background(), "frozen-context", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite, "custom/frozen")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(runtime.contextPresetPath("frozen-context"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(runtime.policyPresetCustomDirectory(), "frozen.json")
	edited := strings.Replace(string(before), `"offline"`, `"get_only_reviewed"`, 1)
	if err := os.WriteFile(source, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(runtime.contextPresetPath("frozen-context"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || created.PolicyGuardrail != tobari.PolicyPresetGuardrailOffline {
		t.Fatal("source edit changed Context snapshot authority")
	}
	manifest, err := runtime.readContextManifest("frozen-context")
	if err != nil {
		t.Fatal(err)
	}
	preset, err := runtime.readContextPreset(manifest)
	if err != nil || preset.Guardrail != tobari.PolicyPresetGuardrailOffline {
		t.Fatalf("snapshotted preset = %+v, %v", preset, err)
	}
}

func TestDefaultContextCanReuseSnapshottedCustomPresetAfterSourceEdit(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	if _, err := runtime.InitPolicyPreset(context.Background(), "snapshot"); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(runtime.policyPresetCustomDirectory(), "snapshot.json")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), `"offline"`, `"reviewed_exact"`, 1)
	if err := os.WriteFile(source, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContextWithPreset(context.Background(), tobari.DefaultContextName, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite, "custom/snapshot"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := runtime.UseContext(context.Background(), tobari.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Active || report.PolicyPresetOrigin != "custom/snapshot" || report.PolicyGuardrail != tobari.PolicyPresetGuardrailReviewedExact {
		t.Fatalf("reused custom preset Context = %+v", report)
	}
}

func TestAgentReadyContextPersistsCoreSnapshotAndProjectsBinaryReadiness(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	if _, err := runtime.CreateContext(context.Background(), "binary-ready", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite); err != nil {
		t.Fatal(err)
	}
	manifest, err := runtime.readContextManifest("binary-ready")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.readContextPreset(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range snapshot.BaselineGrants {
		if rule.Host == "auth.atlassian.com" || rule.Path == "/login/device/code" || rule.Path == "/api/accounts/deviceauth/usercode" || rule.Path == "/api/oauth/claude_cli/roles" {
			t.Fatalf("native readiness was persisted in the Context snapshot: %+v", rule)
		}
	}
	if len(snapshot.GraphQLEndpoints) != 0 {
		t.Fatalf("native readiness endpoint was persisted in the Context snapshot: %+v", snapshot.GraphQLEndpoints)
	}
	items, err := runtime.readAggregateContexts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertTWGReadinessProjected(t, aggregatePresetForContext(t, items, "binary-ready"))
	assertPupReadinessProjected(t, aggregatePresetForContext(t, items, "binary-ready"))
}

func TestExistingAgentReadySnapshotReceivesCurrentBinaryReadinessWithoutRewrite(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	if _, err := runtime.CreateContext(context.Background(), "legacy-ready", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite); err != nil {
		t.Fatal(err)
	}
	manifest, err := runtime.readContextManifest("legacy-ready")
	if err != nil {
		t.Fatal(err)
	}
	legacy, _ := tobari.BuiltinPolicyPreset(tobari.DefaultPolicyPresetOrigin)
	legacyGrants := legacy.BaselineGrants[:0]
	for _, rule := range legacy.BaselineGrants {
		if rule.Host != "auth.atlassian.com" {
			legacyGrants = append(legacyGrants, rule)
		}
	}
	legacy.BaselineGrants = legacyGrants
	_, legacyBytes, legacyRevision, err := tobari.NormalizePolicyPreset(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime.contextPresetPath(manifest.Name), legacyBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest.PolicyPresetRevision = legacyRevision
	if err := writeAtomicJSON(runtime.contextManifestPath(manifest.Name), manifest); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(runtime.contextPresetPath(manifest.Name))
	if err != nil {
		t.Fatal(err)
	}
	items, err := runtime.readAggregateContexts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertTWGReadinessProjected(t, aggregatePresetForContext(t, items, "legacy-ready"))
	assertPupReadinessProjected(t, aggregatePresetForContext(t, items, "legacy-ready"))
	after, err := os.ReadFile(runtime.contextPresetPath(manifest.Name))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("aggregate generation rewrote the legacy Context snapshot")
	}
}

func assertTWGReadinessProjected(t *testing.T, preset tobari.PolicyPreset) {
	t.Helper()
	want := map[string]bool{
		"auth.atlassian.com POST /oauth/device/code":          false,
		"auth.atlassian.com POST /oauth/token":                false,
		"auth.atlassian.com POST /oauth/revoke":               false,
		"api.atlassian.com POST /accessible-products":         false,
		"teamwork-graph.atlassian.com GET /cli/manifest.json": false,
	}
	for _, rule := range preset.BaselineGrants {
		if strings.Contains(rule.Host, "atlassian") && rule.Protocol == "" {
			if rule.Scheme != "https" || rule.Port != 443 {
				t.Fatalf("non-canonical Atlassian rule was projected: %+v", rule)
			}
			key := rule.Host + " " + rule.Method + " " + rule.Path
			if _, expected := want[key]; !expected {
				t.Fatalf("neighboring Atlassian rule was projected: %+v", rule)
			}
			want[key] = true
		}
	}
	for effect, found := range want {
		if !found {
			t.Fatalf("binary TWG readiness was not projected: %s", effect)
		}
	}
	wantEndpoint := tobari.PolicyPresetExactRule{Scheme: "https", Host: "api.atlassian.com", Port: 443, Method: "POST", Path: "/graphql"}
	foundEndpoint := false
	for _, endpoint := range preset.GraphQLEndpoints {
		if endpoint.Host == "api.atlassian.com" {
			if endpoint != wantEndpoint {
				t.Fatalf("neighboring Atlassian GraphQL endpoint was projected: %+v", endpoint)
			}
			foundEndpoint = true
		}
	}
	if !foundEndpoint {
		t.Fatalf("binary TWG GraphQL endpoint was not projected: %+v", preset.GraphQLEndpoints)
	}
	wantGraphQL := tobari.PolicyPresetExactRule{Scheme: "https", Host: "api.atlassian.com", Port: 443, Method: "POST", Path: "/graphql", Protocol: tobari.PolicyProtocolGraphQL, GraphQLOperationType: tobari.GraphQLOperationQuery, GraphQLRootField: "me"}
	foundGraphQL := false
	for _, rule := range preset.BaselineGrants {
		if rule.Host == "api.atlassian.com" && rule.Protocol == tobari.PolicyProtocolGraphQL {
			if rule != wantGraphQL {
				t.Fatalf("neighboring Atlassian GraphQL rule was projected: %+v", rule)
			}
			foundGraphQL = true
		}
	}
	if !foundGraphQL {
		t.Fatal("binary TWG query/me readiness was not projected")
	}
}

func assertPupReadinessProjected(t *testing.T, preset tobari.PolicyPreset) {
	t.Helper()
	want := map[string]bool{
		"POST /api/v2/oauth2/register": false,
		"POST /oauth2/v1/token":        false,
	}
	for _, rule := range preset.BaselineGrants {
		if strings.Contains(rule.Host, "datadog") {
			key := rule.Method + " " + rule.Path
			if rule.Host != "api.datadoghq.com" || rule.Port != 443 || rule.Scheme != "https" || rule.Protocol != "" {
				t.Fatalf("neighboring Datadog rule was projected: %+v", rule)
			}
			if _, expected := want[key]; !expected {
				t.Fatalf("neighboring Datadog rule was projected: %+v", rule)
			}
			want[key] = true
		}
	}
	for effect, found := range want {
		if !found {
			t.Fatalf("binary pup readiness was not projected: %s", effect)
		}
	}
}

func aggregatePresetForContext(t *testing.T, items []aggregateContext, name string) tobari.PolicyPreset {
	t.Helper()
	for _, item := range items {
		if item.manifest.Name == name {
			return item.preset
		}
	}
	t.Fatalf("aggregate Context %q is missing", name)
	return tobari.PolicyPreset{}
}
