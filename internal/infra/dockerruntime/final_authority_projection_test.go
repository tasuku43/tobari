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
			AgentProfile: tobari.DefaultProfile, Mode: tobari.ManifestPolicyModeGuided, NativeReadiness: tobari.ManifestNativeReadinessEnabled,
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
	contextBinding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: finalProjectionContextID, ProjectRoot: "/workspace/example", TemplateID: finalProjectionTemplateID}
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
			ProjectRoot: contextBinding.ProjectRoot, Home: "/workspace/home-" + string(workspaceID), CreationDefaults: revision.Slices.CreationDefaultsDigest,
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
