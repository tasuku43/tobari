//go:build tobari_dev && tobari_research

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/authcmd"
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalAuthCLIReader struct {
	snapshot tobari.ContextAuthoritySnapshot
}

func (f *finalAuthCLIReader) ListContextAuthority(context.Context) ([]tobari.ContextAuthoritySnapshot, error) {
	return []tobari.ContextAuthoritySnapshot{f.snapshot.Clone()}, nil
}

func (f *finalAuthCLIReader) ReadContextAuthorityByReference(context.Context, string) (tobari.ContextAuthoritySnapshot, error) {
	return f.snapshot.Clone(), nil
}

func (f *finalAuthCLIReader) ReadCurrentContextAuthority(context.Context) (tobari.ContextAuthoritySnapshot, error) {
	return f.snapshot.Clone(), nil
}

func (f *finalAuthCLIReader) SetCurrentContextByReference(context.Context, string) (tobari.ContextSelectionResult, error) {
	return tobari.ContextSelectionResult{}, errors.New("unexpected current Context mutation")
}

type finalAuthCLIRuntime struct {
	mutation authbroker.ContextMutationObservation
	status   authbroker.ContextStatusObservation
	calls    int
}

func (*finalAuthCLIRuntime) IsInputTerminal(io.Reader) bool { return true }
func (*finalAuthCLIRuntime) IsTerminal(io.Writer) bool      { return true }
func (f *finalAuthCLIRuntime) LoginFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority, string, string, io.Reader, io.Writer) (authbroker.ContextMutationObservation, error) {
	f.calls++
	return f.mutation, nil
}
func (f *finalAuthCLIRuntime) ImportFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority, string, io.Reader) (authbroker.ContextMutationObservation, error) {
	f.calls++
	return f.mutation, nil
}
func (f *finalAuthCLIRuntime) StatusFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority) (authbroker.ContextStatusObservation, error) {
	f.calls++
	return f.status, nil
}
func (f *finalAuthCLIRuntime) LogoutFinalContextAuth(context.Context, authbroker.ContextAuthenticationAuthority, string) (authbroker.ContextMutationObservation, error) {
	f.calls++
	return f.mutation, nil
}

func finalAuthCLIFixture(t *testing.T) (tobari.ContextAuthoritySnapshot, authbroker.ContextAuthenticationAuthority) {
	t.Helper()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789a1")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789a2")
	body := tobari.WorkspaceTemplateBody{
		Boundary:      tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadOnly, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}},
		Policy:        tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 1, Image: "tobari-runtime:test"}}, SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}},
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: "research", Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}
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

func finalAuthCLIProvider(t *testing.T) authbroker.Provider {
	t.Helper()
	data, err := os.ReadFile("../infra/authproviders/builtins/github-gh.json")
	if err != nil {
		t.Fatal(err)
	}
	var provider authbroker.Provider
	if err := json.Unmarshal(data, &provider); err != nil {
		t.Fatal(err)
	}
	if err := provider.Validate(); err != nil {
		t.Fatal(err)
	}
	return provider
}

func finalAuthCLIService(t *testing.T, task string) (*authcmd.FinalContextService, *workspaceauthoritycmd.ContextService, *finalAuthCLIRuntime, string) {
	t.Helper()
	snapshot, authority := finalAuthCLIFixture(t)
	status, err := authbroker.NewContextStatusObservation(authority, authbroker.StorageBackendXDGFile, authbroker.BrokerStateReady, []authbroker.ProviderStatus{{Provider: authbroker.BuiltinGitHubProviderID, State: authbroker.ProviderCredentialNotConfigured}}, true)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &finalAuthCLIRuntime{status: status}
	if task != authbroker.TaskStatus {
		previous := authbroker.ProviderStatus{Provider: authbroker.BuiltinGitHubProviderID, State: authbroker.ProviderCredentialNotConfigured}
		current := authbroker.ProviderStatus{Provider: authbroker.BuiltinGitHubProviderID, State: authbroker.ProviderCredentialConfigured, CredentialRevision: "revision-2"}
		changed := true
		if task == authbroker.TaskLogout {
			previous = current
			current = authbroker.ProviderStatus{Provider: authbroker.BuiltinGitHubProviderID, State: authbroker.ProviderCredentialNotConfigured}
		}
		decision := authbroker.ContextAuthDecisionAuthority{Task: task, Context: authority, Provider: authbroker.BuiltinGitHubProviderID, ProviderAuthority: finalAuthCLIProvider(t), Previous: previous}
		decisionRef, err := decision.Reference()
		if err != nil {
			t.Fatal(err)
		}
		runtime.mutation = authbroker.ContextMutationObservation{Authority: authority, Decision: decision, Provider: current, StorageBackend: authbroker.StorageBackendXDGFile, BrokerState: authbroker.BrokerStateReady, Changed: changed, DecisionRef: decisionRef}
	}
	reader := &finalAuthCLIReader{snapshot: snapshot}
	return authcmd.NewFinalContext(reader, runtime), workspaceauthoritycmd.NewContextService(reader), runtime, authority.ContextRef
}

func TestFinalAuthCatalogIsContextReferenceBoundSchemaTwo(t *testing.T) {
	t.Parallel()
	byPath := map[string]CommandSpec{}
	for _, spec := range authCommandSpecs() {
		byPath[spec.Path] = spec
		if spec.Agent.Output.JSONSchemaVersion != 2 {
			t.Fatalf("%s schema=%d", spec.Path, spec.Agent.Output.JSONSchemaVersion)
		}
		for _, input := range spec.Agent.Inputs {
			if input.Name == "--manifest" {
				t.Fatalf("%s retained predecessor selector", spec.Path)
			}
		}
	}
	for _, path := range []string{"auth login", "auth import"} {
		spec := byPath[path]
		if spec.Effect != operation.EffectCreate || spec.Agent.Mutation.ParentInput != "--context" || spec.Agent.Mutation.TargetIDInput != "" || !reflect.DeepEqual(spec.Agent.Mutation.TargetInputs, []string{"--context"}) || spec.Agent.Mutation.TargetKind != authbroker.ContextCredentialTargetKind {
			t.Fatalf("%s mutation=%#v effect=%s", path, spec.Agent.Mutation, spec.Effect)
		}
		if len(spec.ProducedRefs()) != 0 {
			t.Fatalf("%s unexpectedly produces refs: %+v", path, spec.ProducedRefs())
		}
	}
	logout := byPath["auth logout"]
	if logout.Agent.Mutation.TargetIDInput != "--context" || logout.Agent.Mutation.TargetKind != tobari.ContextReferenceKind {
		t.Fatalf("logout mutation=%#v", logout.Agent.Mutation)
	}
	if len(logout.ProducedRefs()) != 0 {
		t.Fatalf("logout unexpectedly produces refs: %+v", logout.ProducedRefs())
	}
	status := byPath["auth status"]
	if status.Role != RoleDiscover || status.Effect != operation.EffectRead {
		t.Fatalf("status role/effect=%s/%s", status.Role, status.Effect)
	}
	refs := status.ProducedRefs()
	if len(refs) != 1 || refs[0].Kind != tobari.ContextReferenceKind || refs[0].Field != "context_ref" {
		t.Fatalf("status refs=%#v", refs)
	}
}

func TestFinalAuthStatusJSONReemitsOnlyContextReferenceAndSecretFreeInventory(t *testing.T) {
	service, contexts, runtime, contextRef := finalAuthCLIService(t, authbroker.TaskStatus)
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalAuth = service
	command.finalContexts = contexts
	code := command.RunContext(context.Background(), []string{"auth", "status", "--context", contextRef, "--format=json"})
	if code != ExitOK || runtime.calls != 1 {
		t.Fatalf("code=%d calls=%d stderr=%q", code, runtime.calls, stderr.String())
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if string(document["schema_version"]) != "2" {
		t.Fatalf("document=%s", stdout.String())
	}
	var auth map[string]json.RawMessage
	if err := json.Unmarshal(document["auth"], &auth); err != nil {
		t.Fatal(err)
	}
	want := []string{"broker_state", "context_ref", "providers", "storage_backend", "task"}
	if len(auth) != len(want) {
		t.Fatalf("auth keys=%v", auth)
	}
	for _, key := range want {
		if _, ok := auth[key]; !ok {
			t.Fatalf("missing %s: %s", key, stdout.String())
		}
	}
	for _, forbidden := range []string{"template_ref", "manifest", "workspace_manifest_id", "context_id", "runtime", "decision", "secret", "handle"} {
		if strings.Contains(strings.ToLower(stdout.String()), forbidden) {
			t.Fatalf("status exposed %q: %s", forbidden, stdout.String())
		}
	}
}

func TestFinalAuthStatusHumanNamesOnlyFinalContextScope(t *testing.T) {
	service, contexts, runtime, contextRef := finalAuthCLIService(t, authbroker.TaskStatus)
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalAuth = service
	command.finalContexts = contexts
	code := command.RunContext(context.Background(), []string{"auth", "status", "--context", contextRef})
	if code != ExitOK || runtime.calls != 1 {
		t.Fatalf("code=%d calls=%d stderr=%q", code, runtime.calls, stderr.String())
	}
	for _, want := range []string{"Final Context authentication status", "Context", contextRef, "github", "not_configured"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("human output lacks %q: %s", want, stdout.String())
		}
	}
	for _, forbidden := range []string{"Workspace Manifest", "template", "UUID"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("human output exposed %q: %s", forbidden, stdout.String())
		}
	}
}

func TestFinalAuthLoginConsumesUnchangedContextRefWithoutBecomingAContextProducer(t *testing.T) {
	service, contexts, runtime, contextRef := finalAuthCLIService(t, authbroker.TaskLogin)
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalAuth = service
	command.finalContexts = contexts
	code := command.RunContext(context.Background(), []string{"auth", "login", "--context", contextRef, "--provider", authbroker.BuiltinGitHubProviderID, "--format=json"})
	if code != ExitOK || runtime.calls != 1 {
		t.Fatalf("code=%d calls=%d stderr=%q", code, runtime.calls, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"schema_version":2`) {
		t.Fatalf("output=%s", stdout.String())
	}
	for _, forbidden := range []string{"context_ref", "template_ref", "workspace_manifest", "manifest_state", "decision_ref"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("result exposed %q: %s", forbidden, stdout.String())
		}
	}
}

func TestFinalAuthRejectsOldAndCrossKindSelectorsBeforeAdapter(t *testing.T) {
	service, contexts, runtime, contextRef := finalAuthCLIService(t, authbroker.TaskStatus)
	tests := [][]string{
		{"auth", "status", "--manifest", "legacy", "--context", contextRef},
		{"auth", "status", "--context", "workspace:01912345-6789-7abc-8def-0123456789a2"},
	}
	for _, argv := range tests {
		var stdout, stderr bytes.Buffer
		command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
		command.finalAuth = service
		command.finalContexts = contexts
		if code := command.RunContext(context.Background(), argv); code == ExitOK {
			t.Fatalf("argv %v passed: %s", argv, stdout.String())
		}
	}
	if runtime.calls != 0 {
		t.Fatalf("invalid selectors crossed adapter: %d", runtime.calls)
	}
}

func TestFinalAuthUsesCurrentContextWhenOverrideIsOmitted(t *testing.T) {
	service, contexts, runtime, _ := finalAuthCLIService(t, authbroker.TaskStatus)
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalAuth = service
	command.finalContexts = contexts
	if code := command.RunContext(context.Background(), []string{"auth", "status", "--format=json"}); code != ExitOK || runtime.calls != 1 {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, runtime.calls, stdout.String(), stderr.String())
	}
}

func TestFinalAuthScopedHelpPublishesExactContextGrammarAndSchema(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	if code := command.RunContext(context.Background(), []string{"help", "auth", "status", "--format=agent"}); code != ExitOK {
		t.Fatalf("agent help code=%d stderr=%q", code, stderr.String())
	}
	for _, want := range []string{`"path":"auth status"`, `"json_schema_version":2`, `"name":"--context"`, `"reference_kind":"context"`, `"role":"discover"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("agent help lacks %q: %s", want, stdout.String())
		}
	}
	for _, forbidden := range []string{"--manifest", "workspace_manifest_id", "template_ref"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("agent help exposed %q: %s", forbidden, stdout.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"auth", "status", "--help"}); code != ExitOK {
		t.Fatalf("human help code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "auth status [--context <context-ref>]") || strings.Contains(stdout.String(), "--manifest") {
		t.Fatalf("human help=%s", stdout.String())
	}
}
