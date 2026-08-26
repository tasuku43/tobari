package authcmd

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalContextReaderFixture struct {
	snapshot tobari.ContextAuthoritySnapshot
}

func (f *finalContextReaderFixture) ReadContextAuthorityByReference(context.Context, string) (tobari.ContextAuthoritySnapshot, error) {
	return f.snapshot.Clone(), nil
}

type finalContextRuntimeFixture struct {
	observation authbroker.ContextMutationObservation
	status      authbroker.ContextStatusObservation
	loginCalls  int
	importCalls int
	statusCalls int
	logoutCalls int
}

func (*finalContextRuntimeFixture) IsInputTerminal(io.Reader) bool { return true }
func (*finalContextRuntimeFixture) IsTerminal(io.Writer) bool      { return true }
func (f *finalContextRuntimeFixture) LoginFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority, string, string, io.Reader, io.Writer) (authbroker.ContextMutationObservation, error) {
	f.loginCalls++
	return f.observation, nil
}
func (f *finalContextRuntimeFixture) ImportFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority, string, io.Reader) (authbroker.ContextMutationObservation, error) {
	f.importCalls++
	return f.observation, nil
}
func (f *finalContextRuntimeFixture) StatusFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority) (authbroker.ContextStatusObservation, error) {
	f.statusCalls++
	return f.status, nil
}
func (f *finalContextRuntimeFixture) LogoutFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority, string) (authbroker.ContextMutationObservation, error) {
	f.logoutCalls++
	return f.observation, nil
}

func finalContextSnapshotFixture(t *testing.T) (tobari.ContextAuthoritySnapshot, authbroker.ContextAuthenticationAuthority) {
	t.Helper()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789a1")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789a2")
	body := tobari.WorkspaceTemplateBody{
		Boundary:        tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadOnly, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}},
		Policy:          tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, Mode: tobari.ManifestPolicyModeGuided, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
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
	snapshot := tobari.ContextAuthoritySnapshot{Context: binding, Template: template, PolicyMemory: memory}
	ref, _ := tobari.ContextRef(contextID)
	authority, err := authbroker.NewContextAuthenticationAuthority(snapshot, ref)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, authority
}

func finalContextIntent(command string, effect operation.Effect, contextRef string) operation.Intent {
	target := operation.TargetRef{Kind: authbroker.ContextCredentialTargetKind, ParentID: contextRef}
	if effect == operation.EffectWrite {
		target = operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: contextRef}
	}
	return operation.Intent{Command: command, Effect: effect, Target: target, Impact: FinalContextMutationImpact()}
}

func TestFinalContextAuthValidatesExactIntentBeforeAdapterAndKeepsManyImpact(t *testing.T) {
	snapshot, authority := finalContextSnapshotFixture(t)
	runtime := &finalContextRuntimeFixture{}
	service := NewFinalContext(&finalContextReaderFixture{snapshot: snapshot}, runtime)
	intent := finalContextIntent("auth import", operation.EffectCreate, authority.ContextRef)
	intent.Impact.Cardinality = operation.CardinalityOne
	if _, err := service.Import(context.Background(), intent, authority.ContextRef, authbroker.BuiltinGitHubProviderID, panicReader{}); err == nil || runtime.importCalls != 0 {
		t.Fatalf("impact mismatch crossed adapter: calls=%d err=%v", runtime.importCalls, err)
	}
	intent = finalContextIntent("auth import", operation.EffectCreate, authority.ContextRef)
	intent.Target.ParentID = "context:01912345-6789-7abc-8def-0123456789ff"
	if _, err := service.Import(context.Background(), intent, authority.ContextRef, authbroker.BuiltinGitHubProviderID, panicReader{}); err == nil || runtime.importCalls != 0 {
		t.Fatalf("cross-Context parent crossed adapter: calls=%d err=%v", runtime.importCalls, err)
	}
	impact := FinalContextMutationImpact()
	if impact.Cardinality != operation.CardinalityMany || impact.Notification != operation.DeclarationNo || impact.AccessChange != operation.DeclarationYes || impact.Destructive != operation.DeclarationYes {
		t.Fatalf("final auth impact=%#v", impact)
	}
}

func TestFinalContextStatusReemitsUnchangedContextRefAndRejectsCrossKind(t *testing.T) {
	snapshot, authority := finalContextSnapshotFixture(t)
	status, err := authbroker.NewContextStatusObservation(authority, authbroker.StorageBackendXDGFile, authbroker.BrokerStateReady, []authbroker.ProviderStatus{}, true)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &finalContextRuntimeFixture{status: status}
	service := NewFinalContext(&finalContextReaderFixture{snapshot: snapshot}, runtime)
	result, err := service.Status(context.Background(), authority.ContextRef)
	if err != nil || result.ContextRef != authority.ContextRef || runtime.statusCalls != 1 {
		t.Fatalf("status=%#v calls=%d err=%v", result, runtime.statusCalls, err)
	}
	if _, err := service.Status(context.Background(), "workspace:01912345-6789-7abc-8def-0123456789a2"); err == nil || runtime.statusCalls != 1 {
		t.Fatalf("cross-kind status reached adapter: calls=%d err=%v", runtime.statusCalls, err)
	}
}
