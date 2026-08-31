package dockerruntime

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	finalProjectionTemplateID tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789c1"
	finalProjectionContextID  tobari.ContextID           = "01912345-6789-7abc-8def-0123456789c2"
	finalProjectionWorkspaceA tobari.WorkspaceID         = "01912345-6789-7abc-8def-0123456789c3"
	finalProjectionWorkspaceB tobari.WorkspaceID         = "01912345-6789-7abc-8def-0123456789c4"
)

func finalProjectionCollectionFixture(t *testing.T, workspaceID tobari.WorkspaceID) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	body := tobari.WorkspaceTemplateBody{
		Boundary: tobari.WorkspaceTemplateBoundary{
			SourceAccess:       tobari.ManifestSourceAccessReadOnly,
			DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}},
			MethodPolicy:       tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}},
		},
		Policy: tobari.WorkspaceTemplatePolicyBody{
			AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled,
			BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{},
			MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{},
			GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{},
		},
		EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{
			RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: string(finalSessionDigest("f")), Ordinal: 1, Image: "tobari-runtime:test",
		}},
		SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(finalProjectionTemplateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: finalProjectionTemplateID, Name: "restricted", Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	contextBinding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: finalProjectionContextID, TemplateID: finalProjectionTemplateID}
	ruleBody := tobari.PolicyMemoryRuleBody{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Match:                  tobari.PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/remembered",
		Segments: []string{}, Examples: []string{"/remembered"}, SourceCandidates: []string{"pcy_0123456789abcdef0123456789abcdef"},
	}
	rule, err := tobari.NewPolicyMemoryRule(contextBinding.ID, tobari.PolicyMemoryAllow, ruleBody)
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := tobari.PublishPolicyMemory(contextBinding.ID, []tobari.PolicyMemoryRule{rule}, nil)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt := tobari.TemplatePolicyActivationReceipt{ContextID: contextBinding.ID, TemplateID: template.ID, PolicySliceDigest: revision.Slices.PolicySliceDigest}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextBinding.ID, Revision: memory.Revision}
	activeMemory := memory.Clone()
	record := tobari.WorkspaceAuthorityContextRecord{Context: contextBinding, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: &activeMemory, ActivePolicyMemoryRef: &memoryReceipt}
	workspaces := []tobari.WorkspaceBinding{}
	if workspaceID != "" {
		applied := tobari.WorkspaceAppliedEntry{
			ContextID: contextBinding.ID, TemplateID: template.ID, TemplateRevision: revision.Revision,
			EntrySliceDigest: revision.Slices.EntrySliceDigest, RuntimeID: revision.Slices.RuntimeID,
			RuntimeRevision: revision.Slices.RuntimeRevision, ResolvedSpec: finalSessionDigest("7"), ReconciledAt: time.Unix(4, 0).UTC(),
		}
		workspaces = append(workspaces, tobari.WorkspaceBinding{
			SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextBinding.ID,
			ProjectRoot: "/workspace/example", Home: "/workspace/home-" + string(workspaceID), CreationDefaults: revision.Slices.CreationDefaultsDigest,
			LastSuccessfulEntry: &applied,
		})
	}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection([]tobari.WorkspaceTemplate{template}, []tobari.WorkspaceAuthorityContextRecord{record}, workspaces, []tobari.PolicyCandidateAuthority{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return collection
}

func TestFinalAggregateContextBindsMemoryToCurrentProjectedWorkspace(t *testing.T) {
	withA := finalProjectionCollectionFixture(t, finalProjectionWorkspaceA)
	planA, err := tobari.BuildClusterWorkspacePolicyProjection(withA)
	if err != nil {
		t.Fatal(err)
	}
	allowsA, deniesA, _, err := finalPolicyMemoryRows(planA.Contexts[0])
	allowID := ""
	if len(allowsA) == 1 {
		allowID, _ = allowsA[0]["id"].(string)
	}
	if err != nil || len(allowsA) != 1 || len(deniesA) != 0 || allowsA[0]["project_id"] != string(finalProjectionWorkspaceA) || !strings.HasPrefix(allowID, "plr_") {
		t.Fatalf("Workspace A rows=%#v denies=%#v err=%v", allowsA, deniesA, err)
	}

	withoutWorkspace := finalProjectionCollectionFixture(t, "")
	withoutPlan, err := tobari.BuildClusterWorkspacePolicyProjection(withoutWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	allowsNone, deniesNone, _, err := finalPolicyMemoryRows(withoutPlan.Contexts[0])
	if err != nil || len(allowsNone) != 0 || len(deniesNone) != 0 {
		t.Fatalf("Context-only memory became executable: allows=%#v denies=%#v err=%v", allowsNone, deniesNone, err)
	}

	withB := finalProjectionCollectionFixture(t, finalProjectionWorkspaceB)
	planB, err := tobari.BuildClusterWorkspacePolicyProjection(withB)
	if err != nil {
		t.Fatal(err)
	}
	allowsB, _, _, err := finalPolicyMemoryRows(planB.Contexts[0])
	encoded, _ := json.Marshal(allowsB)
	if err != nil || len(allowsB) != 1 || allowsB[0]["project_id"] != string(finalProjectionWorkspaceB) || strings.Contains(string(encoded), string(finalProjectionWorkspaceA)) {
		t.Fatalf("Workspace B rebind rows=%s err=%v", encoded, err)
	}
}

func TestFinalPolicyMemoryRowsRetainMeaningfulEmptyProtocolCoordinates(t *testing.T) {
	tests := []struct {
		name     string
		identity tobari.PolicyProtocolIdentity
		key      string
	}{
		{
			name: "kubernetes core group",
			identity: tobari.PolicyProtocolIdentity{
				Scheme: "https", Protocol: tobari.PolicyProtocolKubernetes,
				KubernetesKind: "resource", KubernetesVerb: "list", KubernetesGroup: "",
				KubernetesVersion: "v1", KubernetesResource: "pods", KubernetesDryRun: "none",
			},
			key: "kubernetes_group",
		},
		{
			name: "OCI catalog repository",
			identity: tobari.PolicyProtocolIdentity{
				Scheme: "https", Protocol: tobari.PolicyProtocolOCI,
				OCIAction: "list", OCIRepository: "", OCIObject: "catalog",
			},
			key: "oci_repository",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := map[string]any{"protocol": tt.identity.Protocol}
			completeFinalPolicyProtocolCoordinates(row, tt.identity)
			value, present := row[tt.key]
			if !present || value != "" {
				t.Fatalf("meaningful empty coordinate %s = %#v, present=%t", tt.key, value, present)
			}
		})
	}
}

func TestFinalPolicyMemoryRowsPreserveVariantExactCoordinates(t *testing.T) {
	collection := finalProjectionCollectionFixture(t, finalProjectionWorkspaceA)
	projection, err := tobari.BuildClusterWorkspacePolicyProjection(collection)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		body      tobari.PolicyMemoryRuleBody
		required  map[string]any
		forbidden []string
	}{
		{
			name: "AWS JSON",
			body: tobari.PolicyMemoryRuleBody{
				PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolAWS, AWSWireProtocol: tobari.AWSWireProtocolJSON, AWSService: "dynamodb", AWSTargetNamespace: "DynamoDB_20120810", AWSOperation: "ListTables"},
				Match:                  tobari.PolicyMatchExact, Host: "dynamodb.us-east-1.amazonaws.com", Port: 443, Method: "POST", Path: "/", Segments: []string{}, Examples: []string{"/"}, SourceCandidates: []string{"pcy_0123456789abcdef0123456789abcdef"},
			},
			required:  map[string]any{"aws_target_namespace": "DynamoDB_20120810", "aws_operation": "ListTables"},
			forbidden: []string{"aws_protocol_version"},
		},
		{
			name: "Kubernetes non-resource",
			body: tobari.PolicyMemoryRuleBody{
				PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolKubernetes, KubernetesKind: tobari.KubernetesRequestNonResource, KubernetesVerb: "get", KubernetesNonResourcePath: "/openapi/v3"},
				Match:                  tobari.PolicyMatchExact, Host: "cluster.us-east-1.eks.amazonaws.com", Port: 443, Method: "GET", Path: "/openapi/v3", Segments: []string{}, Examples: []string{"/openapi/v3"}, SourceCandidates: []string{"pcy_0123456789abcdef0123456789abcdef"},
			},
			required:  map[string]any{"kubernetes_kind": tobari.KubernetesRequestNonResource, "kubernetes_non_resource_path": "/openapi/v3"},
			forbidden: []string{"kubernetes_group", "kubernetes_version", "kubernetes_resource", "kubernetes_dry_run"},
		},
		{
			name: "MCP non-call",
			body: tobari.PolicyMemoryRuleBody{
				PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolMCP, MCPMethod: "tools/list"},
				Match:                  tobari.PolicyMatchExact, Host: "mcp.example.dev", Port: 443, Method: "POST", Path: "/mcp", Segments: []string{}, Examples: []string{"/mcp"}, SourceCandidates: []string{"pcy_0123456789abcdef0123456789abcdef"},
			},
			required: map[string]any{"mcp_method": "tools/list"}, forbidden: []string{"mcp_tool_name"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := tobari.NewPolicyMemoryRule(finalProjectionContextID, tobari.PolicyMemoryAllow, tt.body)
			if err != nil {
				t.Fatal(err)
			}
			memory, _, err := tobari.PublishPolicyMemory(finalProjectionContextID, []tobari.PolicyMemoryRule{rule}, nil)
			if err != nil {
				t.Fatal(err)
			}
			authority := projection.Contexts[0]
			authority.PolicyMemory = memory
			authority.MemoryReceipt = tobari.PolicyMemoryActivationReceipt{ContextID: finalProjectionContextID, Revision: memory.Revision}
			rows, _, _, err := finalPolicyMemoryRows(authority)
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows=%#v err=%v", rows, err)
			}
			for key, want := range tt.required {
				if got, present := rows[0][key]; !present || got != want {
					t.Fatalf("%s=%#v present=%t want=%#v row=%#v", key, got, present, want, rows[0])
				}
			}
			for _, key := range tt.forbidden {
				if value, present := rows[0][key]; present {
					t.Fatalf("inapplicable %s=%#v was emitted in row=%#v", key, value, rows[0])
				}
			}
		})
	}
}

func TestFinalAggregateContextRejectsUntypedOrMismatchedRenderedContent(t *testing.T) {
	collection := finalProjectionCollectionFixture(t, finalProjectionWorkspaceA)
	plan, err := tobari.BuildClusterWorkspacePolicyProjection(collection)
	if err != nil {
		t.Fatal(err)
	}
	item, err := finalAggregateContext(plan.Contexts[0])
	if err != nil || item.validateIdentity() != nil {
		t.Fatalf("valid final item err=%v identity=%v", err, item.validateIdentity())
	}
	item.data["policy"].(map[string]any)["destination_mode"] = "public_https"
	if err := item.validateIdentity(); err == nil {
		t.Fatal("typed final authority accepted mismatched rendered policy content")
	}
	legacy := item
	legacy.finalAuthority = nil
	legacy.manifest = tobari.WorkspaceManifest{}
	if err := legacy.validateIdentity(); err == nil {
		t.Fatal("arbitrary aggregate identity passed without typed final or valid legacy authority")
	}
}

func TestFinalProjectionRejectsRunningUnhealthyWorkspaceEvidence(t *testing.T) {
	for _, health := range []string{"unhealthy", "none"} {
		observation := finalWorkspaceContainerObservation{
			ID: strings.Repeat("a", 64), Owner: ownerValue, Component: "tobari", Workspace: string(finalProjectionWorkspaceA),
			Role: projectWorkRole, Spec: string(finalSessionDigest("7")), Running: true, Health: health,
		}
		if err := observation.validateFor(finalProjectionWorkspaceA, finalSessionDigest("7"), ""); err == nil {
			t.Fatalf("running %s Workspace became executable principal evidence", health)
		}
	}
}
