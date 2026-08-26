package authbroker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func finalContextAuthorityFixture(t *testing.T) ContextAuthenticationAuthority {
	t.Helper()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789a1")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789a2")
	body := tobari.WorkspaceTemplateBody{
		Boundary: tobari.WorkspaceTemplateBoundary{
			SourceAccess:       tobari.ManifestSourceAccessReadOnly,
			DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}},
			MethodPolicy:       tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
		},
		Policy: tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled,
			BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults:   tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 1, Image: "tobari-runtime:test"}},
		SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}},
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: "research", Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, ProjectRoot: "/workspace/example", TemplateID: templateID}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := tobari.ContextRef(contextID)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := NewContextAuthenticationAuthority(tobari.ContextAuthoritySnapshot{Context: binding, Template: template, PolicyMemory: memory}, ref)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func TestContextAuthDecisionReferenceBindsEveryPrivateRequestDimension(t *testing.T) {
	authority := finalContextAuthorityFixture(t)
	previous := ProviderStatus{Provider: BuiltinAWSProviderID, State: ProviderCredentialNotConfigured}
	decision := ContextAuthDecisionAuthority{Task: TaskLogin, Context: authority, Provider: BuiltinAWSProviderID, ProviderAuthority: syntheticAWSProvider(), LoginMethod: "console", Previous: previous}
	reference, err := decision.Reference()
	if err != nil {
		t.Fatal(err)
	}
	if err := decision.ValidateReference(reference); err != nil {
		t.Fatal(err)
	}
	replacement := byte('a')
	if reference[len(reference)-1] == replacement {
		replacement = 'b'
	}
	wrongDigest := reference[:len(reference)-1] + string(replacement)
	mutations := map[string]func(*ContextAuthDecisionAuthority){
		"valid other digest": func(value *ContextAuthDecisionAuthority) {},
		"template revision": func(value *ContextAuthDecisionAuthority) {
			value.Context.TemplateRevision = tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64))
		},
		"runtime revision": func(value *ContextAuthDecisionAuthority) {
			value.Context.Runtime.Revision = "sha256:" + strings.Repeat("c", 64)
		},
		"provider": func(value *ContextAuthDecisionAuthority) {
			value.Provider, value.Previous.Provider = BuiltinGitHubProviderID, BuiltinGitHubProviderID
			value.ProviderAuthority = syntheticProvider()
			value.ProviderAuthority.ID = BuiltinGitHubProviderID
			value.LoginMethod = ""
		},
		"reviewed provider body": func(value *ContextAuthDecisionAuthority) {
			value.ProviderAuthority.DisplayName = "Changed reviewed provider"
		},
		"login method": func(value *ContextAuthDecisionAuthority) { value.LoginMethod = "identity-center" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := decision
			mutate(&changed)
			candidate := reference
			if name == "valid other digest" {
				candidate = wrongDigest
			}
			if err := changed.ValidateReference(candidate); err == nil {
				t.Fatal("relabeled private decision receipt passed")
			}
		})
	}
	configured := ProviderStatus{Provider: BuiltinAWSProviderID, State: ProviderCredentialConfigured, CredentialRevision: "revision-2"}
	observation := ContextMutationObservation{Authority: authority, Decision: decision, Provider: configured, StorageBackend: StorageBackendXDGFile, BrokerState: BrokerStateReady, Changed: true, DecisionRef: reference}
	if err := observation.ValidateFor(TaskLogin, authority.ContextRef, BuiltinAWSProviderID); err != nil {
		t.Fatal(err)
	}
	relabeled := observation
	relabeled.DecisionRef = wrongDigest
	if err := relabeled.ValidateFor(TaskLogin, authority.ContextRef, BuiltinAWSProviderID); err == nil {
		t.Fatal("terminal observation accepted an unrelated valid digest")
	}
}

func TestFinalContextStatusRequiresBoundedExhaustiveInventoryAndPubliclyReemitsOnlyContextRef(t *testing.T) {
	authority := finalContextAuthorityFixture(t)
	providers := []ProviderStatus{
		{Provider: BuiltinGitHubProviderID, State: ProviderCredentialConfigured, CredentialRevision: "revision-1"},
		{Provider: "removed-owner", State: ProviderCredentialConfigured, CredentialRevision: "revision-2"},
	}
	observation, err := NewContextStatusObservation(authority, StorageBackendXDGFile, BrokerStateReady, providers, true)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewContextStatusResult(authority.ContextRef, observation)
	if err != nil {
		t.Fatal(err)
	}
	if result.CredentialsAbsent() {
		t.Fatal("one of multiple retained provider credentials was treated as exhaustive absence")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatal(err)
	}
	want := []string{"broker_state", "context_ref", "providers", "storage_backend", "task"}
	got := make([]string, 0, len(keys))
	for key := range keys {
		got = append(got, key)
	}
	// JSON key ordering is irrelevant; compare as sets.
	for _, key := range want {
		if _, ok := keys[key]; !ok {
			t.Fatalf("public status omitted %q: %s", key, encoded)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("public status exposed private authority: %s", encoded)
	}

	empty, err := NewContextStatusObservation(authority, StorageBackendXDGFile, BrokerStateReady, []ProviderStatus{}, true)
	if err != nil {
		t.Fatal(err)
	}
	emptyResult, err := NewContextStatusResult(authority.ContextRef, empty)
	if err != nil || !emptyResult.CredentialsAbsent() {
		t.Fatalf("explicit exhaustive zero inventory was not absence: %v", err)
	}
	incomplete, err := NewContextStatusObservation(authority, StorageBackendXDGFile, BrokerStateLocked, []ProviderStatus{}, false)
	if err != nil {
		t.Fatal(err)
	}
	incompleteResult, err := NewContextStatusResult(authority.ContextRef, incomplete)
	if err != nil || incompleteResult.CredentialsAbsent() {
		t.Fatalf("incomplete inventory became absence: %v", err)
	}
	over := make([]ProviderStatus, MaxContextProviderInventory+1)
	for index := range over {
		over[index] = ProviderStatus{Provider: "owner-" + strings.Repeat("a", 4) + string(rune('a'+index%26)) + strings.Repeat("b", index/26), State: ProviderCredentialNotConfigured}
	}
	if _, err := NewContextStatusObservation(authority, StorageBackendXDGFile, BrokerStateReady, over, true); err == nil {
		t.Fatal("over-bound exhaustive provider inventory passed")
	}
}
