package dockerruntime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestAdvancedPolicyReceivesContextNamespaceAndCannotClaimSystemPackages(t *testing.T) {
	t.Parallel()
	item := aggregateContext{
		manifest: tobari.ContextManifest{
			SchemaVersion:        tobari.ContextSchemaVersion,
			ID:                   "01912345-6789-7abc-8def-0123456789ad",
			Name:                 "restricted",
			AgentProfile:         tobari.DefaultProfile,
			PolicyMode:           tobari.ContextPolicyModeAdvanced,
			SourceAccess:         tobari.ContextSourceAccessReadWrite,
			PolicyPresetOrigin:   tobari.DefaultPolicyPresetOrigin,
			PolicyPresetRevision: tobari.DefaultPolicyPresetRevision(),
			Image:                tobari.BuiltinImageSelector,
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
	item := aggregateContext{manifest: tobari.ContextManifest{
		SchemaVersion:        tobari.ContextSchemaVersion,
		ID:                   "01912345-6789-7abc-8def-0123456789ad",
		Name:                 "restricted",
		AgentProfile:         tobari.DefaultProfile,
		PolicyMode:           tobari.ContextPolicyModeAdvanced,
		SourceAccess:         tobari.ContextSourceAccessReadWrite,
		PolicyPresetOrigin:   tobari.DefaultPolicyPresetOrigin,
		PolicyPresetRevision: tobari.DefaultPolicyPresetRevision(),
		Image:                tobari.BuiltinImageSelector,
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

func TestAggregateRouterMakesPresetGuardrailTerminalBeforeAdvancedOrGuidedPolicy(t *testing.T) {
	t.Parallel()
	manifest := tobari.ContextManifest{SchemaVersion: tobari.ContextSchemaVersion, ID: "01912345-6789-7abc-8def-0123456789ad", Name: "restricted", AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector, PolicyMode: tobari.ContextPolicyModeAdvanced, SourceAccess: tobari.ContextSourceAccessReadWrite, PolicyPresetOrigin: "builtin/offline", PolicyPresetRevision: tobari.DefaultPolicyPresetRevision()}
	router, err := aggregateRouter([]aggregateContext{{manifest: manifest}})
	if err != nil {
		t.Fatal(err)
	}
	text := string(router)
	terminal := strings.Index(text, `decision := {"allow": false, "reason": "denied by Context policy preset guardrail"`)
	advanced := strings.Index(text, `result := data.tobari.contexts.c0191234567897abc8def0123456789ad.http.decision`)
	if terminal < 0 || advanced < 0 || terminal > advanced {
		t.Fatalf("guardrail is not declared before Advanced routing:\n%s", text)
	}
	advancedClause := text[strings.LastIndex(text[:advanced], "decision := result if {"):advanced]
	for _, required := range []string{"not terminal_guardrail", "not exact_denied", "not preset_exact_granted"} {
		if !strings.Contains(advancedClause, required) {
			t.Fatalf("Advanced route can bypass %q:\n%s", required, advancedClause)
		}
	}
	if !strings.Contains(text, `kind == "get_only_reviewed"; input.request.method != "GET"`) {
		t.Fatalf("GET-only guardrail does not terminally reject HEAD and non-GET:\n%s", text)
	}
}

func TestAggregateRouterMakesBuiltinHTTPSCeilingTerminalBeforeAdvancedPolicy(t *testing.T) {
	t.Parallel()
	for _, origin := range []string{tobari.DefaultPolicyPresetOrigin, "builtin/get-only-reviewed"} {
		origin := origin
		t.Run(origin, func(t *testing.T) {
			t.Parallel()
			preset, ok := tobari.BuiltinPolicyPreset(origin)
			if !ok {
				t.Fatalf("builtin preset %q not found", origin)
			}
			revision, err := tobari.PolicyPresetRevision(preset)
			if err != nil {
				t.Fatal(err)
			}
			manifest := tobari.ContextManifest{SchemaVersion: tobari.ContextSchemaVersion, ID: "01912345-6789-7abc-8def-0123456789ad", Name: "restricted", AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector, PolicyMode: tobari.ContextPolicyModeAdvanced, SourceAccess: tobari.ContextSourceAccessReadWrite, PolicyPresetOrigin: origin, PolicyPresetRevision: revision}
			router, err := aggregateRouter([]aggregateContext{{manifest: manifest}})
			if err != nil {
				t.Fatal(err)
			}
			text := string(router)
			if strings.Contains(text, "guardrail.custom") {
				t.Fatalf("builtin guardrail still depends on a custom-only marker:\n%s", text)
			}
			if !strings.Contains(text, `destination_mode == "public_https"; input.request.authority.scheme != "https"`) {
				t.Fatalf("builtin %q does not terminally reject plain HTTP:\n%s", origin, text)
			}
			advanced := strings.Index(text, `result := data.tobari.contexts.c0191234567897abc8def0123456789ad.http.decision`)
			if advanced < 0 {
				t.Fatalf("Advanced route for builtin %q is missing:\n%s", origin, text)
			}
			advancedStart := strings.LastIndex(text[:advanced], "decision := result if {")
			if advancedStart < 0 {
				t.Fatalf("Advanced clause for builtin %q is missing:\n%s", origin, text)
			}
			advancedClause := text[advancedStart:advanced]
			if !strings.Contains(advancedClause, "not terminal_guardrail") {
				t.Fatalf("Advanced policy can bypass builtin %q HTTPS ceiling:\n%s", origin, advancedClause)
			}
		})
	}
}

func TestGatewayProjectionCarriesOnlyValidatedGraphQLEndpoints(t *testing.T) {
	t.Parallel()
	endpoint := tobari.GraphQLEndpoint{Scheme: "https", Host: "api.example.com", Port: 443, Path: "/graphql"}
	projection := rewriteGatewayProjection(aggregateContext{
		manifest:         tobari.ContextManifest{Name: "default", ID: "01912345-6789-7abc-8def-0123456789ad"},
		graphqlEndpoints: []tobari.GraphQLEndpoint{endpoint},
	})
	endpoints, ok := projection["graphql_endpoints"].([]tobari.GraphQLEndpoint)
	if !ok || len(endpoints) != 1 || endpoints[0] != endpoint {
		t.Fatalf("GraphQL endpoint projection = %#v", projection["graphql_endpoints"])
	}
}

func TestAggregateRejectsUnsupportedOrAmbiguousSourceInputSchema(t *testing.T) {
	t.Parallel()
	manifest := tobari.ContextManifest{
		SchemaVersion:        tobari.ContextSchemaVersion,
		ID:                   "01912345-6789-7abc-8def-0123456789ad",
		Name:                 "restricted",
		AgentProfile:         tobari.DefaultProfile,
		PolicyMode:           tobari.ContextPolicyModeAdvanced,
		SourceAccess:         tobari.ContextSourceAccessReadWrite,
		PolicyPresetOrigin:   tobari.DefaultPolicyPresetOrigin,
		PolicyPresetRevision: tobari.DefaultPolicyPresetRevision(),
		Image:                tobari.BuiltinImageSelector,
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
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, paths, err := runtime.resolveContext(tobari.DefaultContextName)
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

func TestInvalidContextPolicyDoesNotReplaceKnownGoodAggregate(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	knownGood, err := runtime.buildAggregateProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(context.Background(), "broken", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeAdvanced, tobari.ContextSourceAccessReadWrite); err != nil {
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
