package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	finalSessionTemplateID  tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789b1"
	finalSessionContextID   tobari.ContextID           = "01912345-6789-7abc-8def-0123456789b2"
	finalSessionWorkspaceID tobari.WorkspaceID         = "01912345-6789-7abc-8def-0123456789b3"
)

type finalWorkspaceSessionRunner struct {
	binding tobari.WorkspaceSessionBinding
	health  string
	outputs int
	runs    int
}

func TestHasLiveFinalWorkspaceSessionDoesNotCreateFreshState(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(root+"/config", root+"/state", &finalWorkspaceSessionRunner{})
	if err != nil {
		t.Fatal(err)
	}
	live, err := runtime.HasLiveFinalWorkspaceSession(context.Background())
	if err != nil || live {
		t.Fatalf("fresh session observation live=%t err=%v", live, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("fresh session observation created state: %v", entries)
	}
}

func (r *finalWorkspaceSessionRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	r.runs++
	return nil
}

func (r *finalWorkspaceSessionRunner) Output(ctx context.Context, args, _ []string) ([]byte, error) {
	r.outputs++
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(args) == 5 && args[0] == "container" && args[1] == "inspect" && args[2] == "--format" && args[4] == r.binding.ContainerID {
		health := r.health
		if health == "" {
			health = "healthy"
		}
		return json.Marshal(finalWorkspaceContainerObservation{
			ID: r.binding.ContainerID, Owner: ownerValue, Component: "tobari",
			Workspace: string(r.binding.WorkspaceID), Role: projectWorkRole,
			Spec: string(r.binding.AppliedEntry.ResolvedSpec), Running: true, Health: health,
		})
	}
	return nil, errors.New("unexpected Docker observation")
}

func TestFinalWorkspaceSessionRejectsRunningUnhealthyContainerBeforeOwnerEffect(t *testing.T) {
	for _, health := range []string{"unhealthy", "none"} {
		t.Run(health, func(t *testing.T) {
			binding := finalSessionBindingFixture(t, finalSessionContextID, finalSessionWorkspaceID, "restricted", "/workspace/example")
			runner := &finalWorkspaceSessionRunner{binding: binding, health: health}
			root := t.TempDir()
			runtime, err := newRuntime(root+"/config", root+"/state", runner)
			if err != nil {
				t.Fatal(err)
			}
			prepareFinalSessionPrincipal(t, runtime, binding)
			if owner, err := runtime.BeginFinalWorkspaceSession(context.Background(), binding); err == nil || owner != nil {
				t.Fatalf("running %s Workspace acquired owner", health)
			}
			if runner.runs != 0 {
				t.Fatalf("running %s Workspace performed session effect", health)
			}
		})
	}
}

func finalSessionDigest(character string) tobari.SemanticDigest {
	return tobari.SemanticDigest("sha256:" + strings.Repeat(character, 64))
}

func finalSessionBindingFixture(t *testing.T, contextID tobari.ContextID, workspaceID tobari.WorkspaceID, templateName, projectRoot string) tobari.WorkspaceSessionBinding {
	return finalSessionBindingFixtureWithTemplateID(t, finalSessionTemplateID, contextID, workspaceID, templateName, projectRoot)
}

func finalSessionBindingFixtureWithTemplateID(t *testing.T, templateID tobari.WorkspaceTemplateID, contextID tobari.ContextID, workspaceID tobari.WorkspaceID, templateName, projectRoot string) tobari.WorkspaceSessionBinding {
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
		SessionDefaults:  tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}},
		CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: templateName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	contextBinding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, ProjectRoot: projectRoot, TemplateID: template.ID}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt := tobari.TemplatePolicyActivationReceipt{ContextID: contextID, TemplateID: template.ID, PolicySliceDigest: revision.Slices.PolicySliceDigest}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: memory.Revision}
	applied := tobari.WorkspaceAppliedEntry{
		ContextID: contextID, TemplateID: template.ID, TemplateRevision: revision.Revision,
		EntrySliceDigest: revision.Slices.EntrySliceDigest, RuntimeID: revision.Slices.RuntimeID,
		RuntimeRevision: revision.Slices.RuntimeRevision, ResolvedSpec: finalSessionDigest("7"), ReconciledAt: time.Unix(4, 0).UTC(),
	}
	workspace := tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID,
		ProjectRoot: projectRoot, Home: "/workspace/home-" + string(workspaceID), CreationDefaults: revision.Slices.CreationDefaultsDigest,
		LastSuccessfulEntry: &applied,
	}
	snapshot := tobari.ContextAuthoritySnapshot{
		Context: contextBinding, Template: template, PolicyMemory: memory, Workspace: &workspace,
		ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: &memory, ActivePolicyMemoryRef: &memoryReceipt,
	}
	receipt := tobari.WorkspaceEntryReconciliationReceipt{WorkspaceID: workspaceID, ContextID: contextID, Applied: applied, ContainerID: strings.Repeat("a", 64)}
	binding, err := tobari.NewWorkspaceSessionBinding(snapshot, receipt)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func prepareFinalSessionPrincipal(t *testing.T, runtime *Runtime, binding tobari.WorkspaceSessionBinding) projectPrincipalBinding {
	t.Helper()
	principal := projectPrincipalBinding{
		ProjectID: string(binding.WorkspaceID), WorkspaceManifestID: string(binding.ContextID),
		WorkspaceManifestName: binding.ContextPresentation, ProjectRoot: binding.ProjectRoot,
		WorkspaceIP: "172.30.0.2", GatewayIP: "172.30.0.1", Network: "tobari-final-session-network",
	}
	if err := runtime.ensureProjectPrincipalRegistry(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.replaceProjectPrincipalRegistry(context.Background(), []projectPrincipalBinding{principal}); err != nil {
		t.Fatal(err)
	}
	return principal
}

func TestFinalWorkspaceSessionReusesCanonicalOwnerAndObservesExactLiveness(t *testing.T) {
	binding := finalSessionBindingFixture(t, finalSessionContextID, finalSessionWorkspaceID, "restricted", "/workspace/example")
	runner := &finalWorkspaceSessionRunner{binding: binding}
	root := t.TempDir()
	runtime, err := newRuntime(root+"/config", root+"/state", runner)
	if err != nil {
		t.Fatal(err)
	}
	prepareFinalSessionPrincipal(t, runtime, binding)
	owner, err := runtime.BeginFinalWorkspaceSession(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	borrower, err := runtime.BeginFinalWorkspaceSession(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if borrower.attachment.owned || borrower.attachment.session.AttachmentID != owner.attachment.session.AttachmentID ||
		borrower.attachment.session.IngestionNonce != owner.attachment.session.IngestionNonce {
		t.Fatalf("final borrower=%+v owner=%+v", borrower.attachment.session, owner.attachment.session)
	}
	identity, err := binding.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if state, err := runtime.ObserveFinalWorkspaceSession(context.Background(), identity); err != nil || state != FinalWorkspaceSessionLive {
		t.Fatalf("live final session = %q, %v", state, err)
	}
	if err := borrower.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if state, err := runtime.ObserveFinalWorkspaceSession(context.Background(), identity); err != nil || state != FinalWorkspaceSessionLive {
		t.Fatalf("borrower close changed owner = %q, %v", state, err)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if state, err := runtime.ObserveFinalWorkspaceSession(context.Background(), identity); err != nil || state != FinalWorkspaceSessionAbsent {
		t.Fatalf("closed final session = %q, %v", state, err)
	}
}

func TestFinalWorkspaceSessionRejectsStalePrincipalProjectionAndCancellation(t *testing.T) {
	binding := finalSessionBindingFixture(t, finalSessionContextID, finalSessionWorkspaceID, "restricted", "/workspace/example")
	runner := &finalWorkspaceSessionRunner{binding: binding}
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", runner)
	prepareFinalSessionPrincipal(t, runtime, binding)

	for name, changed := range map[string]tobari.WorkspaceSessionBinding{
		"Context ID":   finalSessionBindingFixture(t, "01912345-6789-7abc-8def-0123456789b4", finalSessionWorkspaceID, "restricted", "/workspace/example"),
		"Workspace ID": finalSessionBindingFixture(t, finalSessionContextID, "01912345-6789-7abc-8def-0123456789b4", "restricted", "/workspace/example"),
		"presentation": finalSessionBindingFixture(t, finalSessionContextID, finalSessionWorkspaceID, "alternate", "/workspace/example"),
		"Project root": finalSessionBindingFixture(t, finalSessionContextID, finalSessionWorkspaceID, "restricted", "/workspace/alternate"),
	} {
		t.Run(name, func(t *testing.T) {
			runner.binding = changed
			if owner, err := runtime.BeginFinalWorkspaceSession(context.Background(), changed); err == nil || owner != nil {
				t.Fatal("stale final principal projection acquired canonical owner")
			}
		})
	}
	runner.binding = binding
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if owner, err := runtime.BeginFinalWorkspaceSession(ctx, binding); !errors.Is(err, context.Canceled) || owner != nil {
		t.Fatalf("canceled final owner = %#v, %v", owner, err)
	}
}

func TestFinalWorkspaceSessionAbsenceDoesNotRequireTransientEntryOrPrincipalProjection(t *testing.T) {
	binding := finalSessionBindingFixture(t, finalSessionContextID, finalSessionWorkspaceID, "restricted", "/workspace/example")
	identity, err := binding.Identity()
	if err != nil {
		t.Fatal(err)
	}
	runner := &finalWorkspaceSessionRunner{binding: binding}

	t.Run("registry never created", func(t *testing.T) {
		root := t.TempDir()
		runtime, _ := newRuntime(root+"/config", root+"/state", runner)
		if state, err := runtime.ObserveFinalWorkspaceSession(context.Background(), identity); err != nil || state != FinalWorkspaceSessionAbsent {
			t.Fatalf("missing canonical registry = %q, %v", state, err)
		}
	})

	t.Run("principal removed after close", func(t *testing.T) {
		root := t.TempDir()
		runtime, _ := newRuntime(root+"/config", root+"/state", runner)
		prepareFinalSessionPrincipal(t, runtime, binding)
		owner, err := runtime.BeginFinalWorkspaceSession(context.Background(), binding)
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := runtime.replaceProjectPrincipalRegistry(context.Background(), []projectPrincipalBinding{}); err != nil {
			t.Fatal(err)
		}
		if state, err := runtime.ObserveFinalWorkspaceSession(context.Background(), identity); err != nil || state != FinalWorkspaceSessionAbsent {
			t.Fatalf("empty registry after principal removal = %q, %v", state, err)
		}
	})

	t.Run("malformed registry remains ambiguous", func(t *testing.T) {
		root := t.TempDir()
		runtime, _ := newRuntime(root+"/config", root+"/state", runner)
		if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(runtime.interactiveAttachmentSessionRegistryPath(), []byte("{broken\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if state, err := runtime.ObserveFinalWorkspaceSession(context.Background(), identity); err == nil || state == FinalWorkspaceSessionAbsent {
			t.Fatalf("malformed registry = %q, %v", state, err)
		}
	})

	t.Run("unsafe registry remains ambiguous", func(t *testing.T) {
		root := t.TempDir()
		runtime, _ := newRuntime(root+"/config", root+"/state", runner)
		if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
			t.Fatal(err)
		}
		path := runtime.interactiveAttachmentSessionRegistryPath()
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("foreign.json", path); err != nil {
			t.Fatal(err)
		}
		if state, err := runtime.ObserveFinalWorkspaceSession(context.Background(), identity); err == nil || state == FinalWorkspaceSessionAbsent {
			t.Fatalf("unsafe registry = %q, %v", state, err)
		}
	})
}

func TestFinalWorkspaceSessionOwnerLossIsAmbiguousNotAbsent(t *testing.T) {
	binding := finalSessionBindingFixture(t, finalSessionContextID, finalSessionWorkspaceID, "restricted", "/workspace/example")
	runner := &finalWorkspaceSessionRunner{binding: binding}
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", runner)
	prepareFinalSessionPrincipal(t, runtime, binding)
	owner, err := runtime.BeginFinalWorkspaceSession(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.attachment.closeTransport(); err != nil {
		t.Fatal(err)
	}
	identity, err := binding.Identity()
	if err != nil {
		t.Fatal(err)
	}
	if state, err := runtime.ObserveFinalWorkspaceSession(context.Background(), identity); err == nil || state == FinalWorkspaceSessionAbsent {
		t.Fatalf("lost owner liveness = %q, %v", state, err)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestFinalWorkspaceSessionRunRejectsPrincipalDriftBeforeChildAttachments(t *testing.T) {
	binding := finalSessionBindingFixture(t, finalSessionContextID, finalSessionWorkspaceID, "restricted", "/workspace/example")
	runner := &finalWorkspaceSessionRunner{binding: binding}
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", runner)
	prepareFinalSessionPrincipal(t, runtime, binding)
	owner, err := runtime.BeginFinalWorkspaceSession(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	runtime.finalSessionAfterLiveness = func() {
		if err := runtime.replaceProjectPrincipalRegistry(context.Background(), []projectPrincipalBinding{}); err != nil {
			t.Errorf("replace principal after first complete owner observation: %v", err)
		}
	}
	if outcome, err := owner.Run(context.Background(), tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err == nil || outcome.ExitCode != 0 {
		t.Fatalf("stale final owner run = %#v, %v", outcome, err)
	}
	if runner.outputs != 2 || runner.runs != 0 {
		t.Fatalf("stale owner performed child effects: outputs=%d runs=%d", runner.outputs, runner.runs)
	}
	if _, err := os.Lstat(runtime.hostLoopbackDirectory()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale owner created Host Loopback state: %v", err)
	}
}

func TestFinalWorkspaceSessionFrozenWireUsesContextAndWorkspaceIdentityOnly(t *testing.T) {
	binding := finalSessionBindingFixture(t, finalSessionContextID, finalSessionWorkspaceID, "restricted", "/workspace/example")
	runner := &finalWorkspaceSessionRunner{binding: binding}
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", runner)
	principal := prepareFinalSessionPrincipal(t, runtime, binding)
	owner, err := runtime.BeginFinalWorkspaceSession(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	encoded, err := os.ReadFile(runtime.principalRegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, key := range []string{`"context_id"`, `"project_id"`, `"context"`} {
		if !strings.Contains(text, key) {
			t.Fatalf("frozen principal wire missing %s: %s", key, text)
		}
	}
	for _, forbidden := range []string{`"workspace_manifest_id"`, `"workspace_id"`, `"workspace_template_id"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("final domain token escaped onto frozen wire: %s", text)
		}
	}
	if owner.attachment.session.FrozenPrincipalFingerprint != frozenPrincipalFingerprint(principal) {
		t.Fatal("final owner did not reuse exact frozen principal fingerprint")
	}
}
