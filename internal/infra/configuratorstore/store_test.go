package configuratorstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestPendingTaskIgnoresExactPreReleaseAggregateMetadata(t *testing.T) {
	state := t.TempDir()
	root := filepath.Join(state, "configurator")
	body := storeBodyFixture()
	store, err := New(root, filepath.Join(state, "contexts"), resolverFixture{binding: body.EntryDefaults.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	legacyBase := "tobari-configurator-v2\x00" + seed.ProjectRoot + "\x00" + string(tobari.ConfiguratorAgentCodex) + "\x00" + string(seed.Purpose) + "\x00" + seed.Runtime().RuntimeID + "\x00" + seed.Runtime().Revision
	legacyDigest := sha256.Sum256([]byte(legacyBase))
	legacyID := "cfg1_" + hex.EncodeToString(legacyDigest[:])
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab"), tobari.ContextID("01912345-6789-7abc-8def-0123456789ac"))
	if err != nil {
		t.Fatal(err)
	}
	draft.ID = legacyID
	legacy := struct {
		SchemaVersion int                      `json:"schema_version"`
		Draft         tobari.ConfiguratorDraft `json:"draft"`
		Seed          tobari.ConfiguratorSeed  `json:"seed"`
	}{SchemaVersion: 1, Draft: draft, Seed: seed}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, legacyID)
	if err := ensureDurablePrivateDirectoryTree(dir); err != nil {
		t.Fatal(err)
	}
	// Reproduce the V2 wire shape: task fields did not exist yet.
	data = bytes.ReplaceAll(data, []byte(`"task":"aggregate",`), nil)
	if err := os.WriteFile(filepath.Join(dir, metadataFile), data, 0o600); err != nil {
		t.Fatal(err)
	}
	draftResult, _, frozen, confirmed, err := store.PendingTask(context.Background(), seed.ProjectRoot, tobari.ConfiguratorTaskRuntime, "01912345-6789-7abc-8def-0123456789ad")
	if err != nil || draftResult.ID != "" || frozen || confirmed {
		t.Fatalf("pre-release aggregate blocked current task recovery: draft=%+v frozen=%v confirmed=%v err=%v", draftResult, frozen, confirmed, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, metadataFile)); err != nil {
		t.Fatalf("pre-release aggregate was changed or deleted: %v", err)
	}
}

func prepareTestDraft(ctx context.Context, store *Store, seed tobari.ConfiguratorSeed, agent tobari.ConfiguratorAgent) (tobari.ConfiguratorDraft, error) {
	draft, err := store.Reserve(ctx, seed, agent)
	if err != nil {
		return tobari.ConfiguratorDraft{}, err
	}
	if err := store.Materialize(ctx, draft); err != nil {
		return tobari.ConfiguratorDraft{}, err
	}
	return draft, nil
}

func TestPendingHomeAdoptionUsesProjectScopedProcessLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows file-lock fallback does not provide cross-process exclusion")
	}
	state := t.TempDir()
	body := storeBodyFixture()
	store, err := New(filepath.Join(state, "configurator"), filepath.Join(state, "contexts"), resolverFixture{binding: body.EntryDefaults.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := "/workspace/example"
	lockRoot := filepath.Join(state, "configurator", ".locks")
	if err := ensurePrivateDirectory(lockRoot); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte("project-" + projectRoot))
	file, err := os.OpenFile(filepath.Join(lockRoot, hex.EncodeToString(digest[:])+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	locked, err := tryLockProjectFile(file)
	if err != nil || !locked {
		t.Fatalf("hold project lock locked=%v err=%v", locked, err)
	}
	defer func() {
		unlockProjectFile(file)
		_ = file.Close()
	}()
	if _, _, err := store.PendingHomeAdoption(context.Background(), projectRoot); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("concurrent pending scan error=%v", err)
	}
}

type resolverFixture struct {
	binding tobari.RuntimeBinding
	seen    *tobari.RuntimeSourceRef
	err     error
}

type runtimeSourceManagerFixture struct {
	prepareErr error
}

func (f runtimeSourceManagerFixture) PrepareConfiguratorRuntimeSource(context.Context, tobari.ConfiguratorDraft) error {
	return f.prepareErr
}

func (runtimeSourceManagerFixture) FreezeConfiguratorRuntimeSource(context.Context, tobari.ConfiguratorDraft) (*tobari.ConfiguratorRuntimeSource, error) {
	return nil, nil
}

func (f resolverFixture) ResolveWorkspaceTemplateRuntimeSource(_ context.Context, source tobari.RuntimeSourceRef) (tobari.RuntimeBinding, error) {
	if f.seen != nil {
		*f.seen = source
	}
	if f.err != nil {
		return tobari.RuntimeBinding{}, f.err
	}
	return f.binding, nil
}

func (f resolverFixture) ResolveWorkspaceTemplateRuntimeSourceWithRetainedBinding(ctx context.Context, source tobari.RuntimeSourceRef, retained tobari.RuntimeBinding) (tobari.RuntimeBinding, error) {
	resolved, err := f.ResolveWorkspaceTemplateRuntimeSource(ctx, source)
	if errors.Is(err, tobari.ErrRuntimeRevisionNotFound) && source.Matches(retained) {
		return retained, nil
	}
	return resolved, err
}

func TestPrepareAndFreezeUseOnlyWorkingCopyBelowManagedHome(t *testing.T) {
	state := t.TempDir()
	root := filepath.Join(state, "configurator")
	homes := filepath.Join(state, "contexts")
	body := storeBodyFixture()
	store, err := New(root, homes, resolverFixture{binding: body.EntryDefaults.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	first, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("resumed draft changed: first=%+v second=%+v", first, second)
	}
	submission, err := store.Freeze(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if err := submission.Validate(); err != nil || submission.Body.EntryDefaults.Runtime != body.EntryDefaults.Runtime {
		t.Fatalf("submission is invalid or changed: submission=%+v err=%v", submission, err)
	}
	home := filepath.Join(root, first.ID, "home")
	relative, _ := tobari.ConfiguratorWorkingDirectory(first)
	for _, path := range []string{home, filepath.Join(home, ".codex"), filepath.Join(home, filepath.FromSlash(relative))} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("unsafe managed directory %q: info=%v err=%v", path, info, err)
		}
	}
	instructions, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(relative), "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(instructions, []byte("Start in English. If the user's first substantive response is in another language, continue in that language.")) {
		t.Fatalf("generated language-following guidance is absent: %q", instructions)
	}
	if _, err := os.Lstat(filepath.Join(root, first.ID, "source")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("working source escaped managed Home: %v", err)
	}
}

func TestPrepareCreatesSelectedClaudeNativeStateRoot(t *testing.T) {
	state := t.TempDir()
	body := storeBodyFixture()
	store, err := New(filepath.Join(state, "configurator"), filepath.Join(state, "contexts"), resolverFixture{binding: body.EntryDefaults.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentClaude)
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(state, "configurator", draft.ID, "home")
	if _, err := requirePrivateDirectory(filepath.Join(home, ".claude")); err != nil {
		t.Fatalf("Claude native state root is unavailable: %v", err)
	}
}

func TestPolicyAssistMaterializesExactKnowledgeAndFreezesOnlyPolicyYAML(t *testing.T) {
	state := t.TempDir()
	body := storeBodyFixture()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{
		Context:      tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID},
		Template:     tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}},
		PolicyMemory: memory,
	}
	seed, err := tobari.NewPolicyAssistConfiguratorSeed(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(filepath.Join(state, "configurator"), filepath.Join(state, "contexts"), resolverFixture{err: tobari.ErrRuntimeRevisionNotFound})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(state, "contexts", string(contextID), "home")
	work := workingRoot(home, draft)
	agents, err := os.ReadFile(filepath.Join(work, "AGENTS.md"))
	if err != nil || !bytes.Contains(agents, []byte("Edit only configuration/templates/<template-id>/policy.yaml")) || bytes.Contains(agents, []byte("Edit only the resource source below configuration/")) {
		t.Fatalf("policy task instructions are not target-exact: bytes=%q err=%v", agents, err)
	}
	referencePath := filepath.Join(work, "POLICY.md")
	reference, err := os.ReadFile(referencePath)
	if err != nil || !bytes.Equal(reference, []byte(policyReference)) {
		t.Fatalf("policy knowledge pack changed: err=%v bytes=%q", err, reference)
	}
	for _, phrase := range []string{"Boundary is evaluated first", "Exact static Deny is terminal", "Policy Memory is separate", "semantic.providers.aws", "Kubernetes rules"} {
		if !bytes.Contains(reference, []byte(phrase)) {
			t.Fatalf("policy knowledge pack omitted %q", phrase)
		}
	}
	policyPath := filepath.Join(sourceRoot(home, draft), "templates", string(templateID), "policy.yaml")
	policy, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(policy, []byte("native_readiness: enabled"), []byte("native_readiness: disabled"), 1)
	if bytes.Equal(changed, policy) {
		t.Fatalf("policy fixture lacks native_readiness: %q", policy)
	}
	if err := os.WriteFile(policyPath, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	submission, err := store.Freeze(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Body.Policy.NativeReadiness != tobari.ManifestNativeReadinessDisabled || !reflect.DeepEqual(submission.Body.Boundary, body.Boundary) || submission.Body.EntryDefaults.Runtime != body.EntryDefaults.Runtime || !reflect.DeepEqual(seed.Evolution.PolicyMemory, &memory) {
		t.Fatalf("policy-only freeze crossed its target: submission=%+v memory=%+v", submission, seed.Evolution.PolicyMemory)
	}
	if err := os.WriteFile(referencePath, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Freeze(context.Background(), draft); err == nil {
		t.Fatal("changed Tobari policy knowledge pack was accepted")
	}
	if err := os.WriteFile(referencePath, []byte(policyReference), 0o600); err != nil {
		t.Fatal(err)
	}
	templatePath := filepath.Join(sourceRoot(home, draft), "templates", string(templateID), "template.yaml")
	templateSource, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	changedTemplate := bytes.Replace(templateSource, []byte("source_access: read-write"), []byte("source_access: read-only"), 1)
	if bytes.Equal(changedTemplate, templateSource) {
		t.Fatalf("template fixture lacks source_access: %q", templateSource)
	}
	if err := os.WriteFile(templatePath, changedTemplate, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Freeze(context.Background(), draft); err == nil {
		t.Fatal("policy assist accepted a template.yaml edit")
	}
}

func TestTaskDraftRequiresExactAgentUntilPublishedGenerationCanSettle(t *testing.T) {
	state := t.TempDir()
	body := storeBodyFixture()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{
		Context:      tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID},
		Template:     tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}},
		PolicyMemory: memory,
	}
	seed, err := tobari.NewPolicyAssistConfiguratorSeed(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(filepath.Join(state, "configurator"), filepath.Join(state, "contexts"), resolverFixture{binding: body.EntryDefaults.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	if resumed, err := store.Reserve(context.Background(), seed, tobari.ConfiguratorAgentCodex); err != nil || resumed != draft {
		t.Fatalf("exact retained task did not resume: draft=%+v err=%v", resumed, err)
	}
	if _, err := store.Reserve(context.Background(), seed, tobari.ConfiguratorAgentClaude); !errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) {
		t.Fatalf("retained task allowed another agent: %v", err)
	}

	home := filepath.Join(state, "contexts", string(contextID), "home")
	policyPath := filepath.Join(sourceRoot(home, draft), "templates", string(templateID), "policy.yaml")
	policy, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(policy, []byte("native_readiness: enabled"), []byte("native_readiness: disabled"), 1)
	if bytes.Equal(changed, policy) || os.WriteFile(policyPath, changed, 0o600) != nil {
		t.Fatal("failed to prepare changed policy generation")
	}
	submission, err := store.Freeze(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	published, err := tobari.NewWorkspaceTemplateRevision(templateID, 2, submission.Body)
	if err != nil || published.Revision != submission.SourceRevision {
		t.Fatalf("published generation mismatch: revision=%+v submission=%+v err=%v", published, submission, err)
	}
	snapshot.Template.Current = published
	snapshot.Template.Retained = append(snapshot.Template.Retained, published.Clone())
	nextSeed, err := tobari.NewPolicyAssistConfiguratorSeed(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reserve(context.Background(), nextSeed, tobari.ConfiguratorAgentClaude); !errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) {
		t.Fatalf("Store inferred publication from caller-supplied authority: %v", err)
	}
	if err := store.ConfirmTask(context.Background(), submission); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteTask(context.Background(), submission); err != nil {
		t.Fatal(err)
	}
	next, err := store.Reserve(context.Background(), nextSeed, tobari.ConfiguratorAgentClaude)
	if err != nil {
		t.Fatalf("authority-verified retained generation did not settle: %v", err)
	}
	if next.ID == draft.ID || next.Agent != tobari.ConfiguratorAgentClaude {
		t.Fatalf("next task generation reused the retained draft: old=%+v next=%+v", draft, next)
	}
	old, present, err := store.readMetadata(filepath.Join(state, "configurator", draft.ID))
	if err != nil || !present || !old.Settled {
		t.Fatalf("published retained task was not durably settled: metadata=%+v present=%v err=%v", old, present, err)
	}
}

func TestRuntimeTaskRetiresUnmaterializedStaleSourceReservation(t *testing.T) {
	state := t.TempDir()
	body := storeBodyFixture()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{
		Context:      tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID},
		Template:     tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}},
		PolicyMemory: memory,
	}
	baseA := tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))
	seed, err := tobari.NewRuntimeAssistConfiguratorSeed(snapshot.Template.Current.Body.EntryDefaults.Runtime, "018bcfe5-687b-7000-8000-000000000077", baseA)
	if err != nil {
		t.Fatal(err)
	}
	instructionRoot := t.TempDir()
	if err := writeInstructions(instructionRoot, seed); err != nil {
		t.Fatal(err)
	}
	agents, err := os.ReadFile(filepath.Join(instructionRoot, "AGENTS.md"))
	if err != nil || !bytes.Contains(agents, []byte("Edit only runtime/source/")) || bytes.Contains(agents, []byte("Edit only the resource source below configuration/")) {
		t.Fatalf("Runtime task instructions are not target-exact: bytes=%q err=%v", agents, err)
	}
	root := filepath.Join(state, "configurator")
	store, err := New(root, filepath.Join(state, "contexts"), resolverFixture{binding: body.EntryDefaults.Runtime}, runtimeSourceManagerFixture{prepareErr: tobari.ErrResourceSourceChanged})
	if err != nil {
		t.Fatal(err)
	}
	draft, err := store.Reserve(context.Background(), seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, ".runtime-assist-homes", string(tobari.ConfiguratorAgentCodex), "home")
	nativeCanary := filepath.Join(home, ".codex", "auth.json")
	if err := os.MkdirAll(filepath.Dir(nativeCanary), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nativeCanary, []byte("native-state\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.retireAfterMarker = func() error { return errors.New("injected retirement interruption") }
	if err := store.Materialize(context.Background(), draft); !errors.Is(err, tobari.ErrResourceSourceChanged) || !errors.Is(err, tobari.ErrConfiguratorTaskRetirementIncomplete) {
		t.Fatalf("stale target materialization did not classify interrupted retirement: %v", err)
	}
	interrupted, present, err := store.readMetadata(filepath.Join(root, draft.ID))
	if err != nil || !present || !interrupted.Retiring || interrupted.Retired {
		t.Fatalf("interrupted retirement was not durably marked: metadata=%+v present=%v err=%v", interrupted, present, err)
	}
	store.retireAfterMarker = nil
	scope, err := seed.ConfiguratorScopeKey()
	if err != nil {
		t.Fatal(err)
	}
	if pending, _, _, _, err := store.PendingTask(context.Background(), scope, seed.Task, seed.TargetRuntimeID); err != nil || pending.ID != "" {
		t.Fatalf("interrupted retirement did not replay: pending=%+v err=%v", pending, err)
	}
	retired, present, err := store.readMetadata(filepath.Join(root, draft.ID))
	if err != nil || !present || !retired.Retired {
		t.Fatalf("stale unmaterialized draft was not retired: metadata=%+v present=%v err=%v", retired, present, err)
	}
	if _, err := os.Lstat(filepath.Dir(workingRoot(home, draft))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale task working material survived retirement: %v", err)
	}
	if data, err := os.ReadFile(nativeCanary); err != nil || string(data) != "native-state\n" {
		t.Fatalf("task retirement changed complete managed Home native state: data=%q err=%v", data, err)
	}
	// Replay a crash after the durable retiring marker but before cleanup.
	retired.Retired = false
	retired.Retiring = true
	if err := writeAtomicJSON(filepath.Join(root, draft.ID, metadataFile), retired); err != nil {
		t.Fatal(err)
	}
	residue := filepath.Join(root, draft.ID, "retirement-residue")
	if err := os.WriteFile(residue, []byte("partial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if pending, _, _, _, err := store.PendingTask(context.Background(), scope, seed.Task, seed.TargetRuntimeID); err != nil || pending.ID != "" {
		t.Fatalf("retiring replay remained pending: draft=%+v err=%v", pending, err)
	}
	retired, present, err = store.readMetadata(filepath.Join(root, draft.ID))
	if err != nil || !present || !retired.Retired || retired.Retiring {
		t.Fatalf("retiring replay did not converge: metadata=%+v present=%v err=%v", retired, present, err)
	}
	if _, err := os.Lstat(residue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retiring replay preserved partial residue: %v", err)
	}
	nextSeed := seed
	nextSeed.TargetRuntimeRevision = tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64))
	next, err := store.Reserve(context.Background(), nextSeed, tobari.ConfiguratorAgentClaude)
	if err != nil || next.ID == draft.ID {
		t.Fatalf("fresh target generation remained blocked: next=%+v err=%v", next, err)
	}
}

func TestReserveDoesNotMaterializeManagedHomeBeforeAttachmentLease(t *testing.T) {
	state := t.TempDir()
	body := storeBodyFixture()
	store, err := New(filepath.Join(state, "configurator"), filepath.Join(state, "contexts"), resolverFixture{binding: body.EntryDefaults.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := store.Reserve(context.Background(), seed, tobari.ConfiguratorAgentClaude)
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(state, "configurator", draft.ID, "home")
	if _, err := os.Lstat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reservation touched managed Home before attachment lease: %v", err)
	}
	if err := store.Materialize(context.Background(), draft); err != nil {
		t.Fatal(err)
	}
	if _, err := requirePrivateDirectory(filepath.Join(home, ".claude")); err != nil {
		t.Fatalf("leased materialization omitted Claude state root: %v", err)
	}
}

func TestPrepareResumesExactReservedIDsAfterCrashBeforeHomeMaterial(t *testing.T) {
	state := t.TempDir()
	body := storeBodyFixture()
	store, err := New(filepath.Join(state, "configurator"), filepath.Join(state, "contexts"), resolverFixture{binding: body.EntryDefaults.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("synthetic crash after reservation")
	store.prepareAfterReservation = func() error { return interrupted }
	if _, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentCodex); !errors.Is(err, interrupted) {
		t.Fatalf("reservation crash=%v", err)
	}
	draftID, err := tobari.ConfiguratorDraftID(seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	reserved, present, err := store.readMetadata(filepath.Join(state, "configurator", draftID))
	if err != nil || !present || reserved.Draft.AdoptionContextID == "" {
		t.Fatalf("durable reservation=%+v present=%v err=%v", reserved, present, err)
	}
	store.prepareAfterReservation = nil
	resumed, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	if resumed != reserved.Draft {
		t.Fatalf("resumed draft=%+v reserved=%+v", resumed, reserved.Draft)
	}
	if _, err := store.homeForDraft(resumed); err != nil {
		t.Fatalf("reserved Home material did not resume: %v", err)
	}
}

func TestFreezeResolvesRuntimeEditedByAgentInsteadOfForcingExecutionRuntime(t *testing.T) {
	state := t.TempDir()
	initial := storeBodyFixture()
	next := initial.Clone()
	next.EntryDefaults.Runtime.Revision = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	next.EntryDefaults.Runtime.Image = "tobari-runtime:next"
	var seen tobari.RuntimeSourceRef
	store, err := New(filepath.Join(state, "configurator"), filepath.Join(state, "contexts"), resolverFixture{binding: next.EntryDefaults.Runtime, seen: &seen})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := tobari.NewBootstrapConfiguratorSeed("/workspace/example", initial)
	draft, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(state, "configurator", draft.ID, "home")
	templatePath := filepath.Join(sourceRoot(home, draft), "templates", string(draft.TemplateID), "template.yaml")
	data, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.ReplaceAll(data, []byte(initial.EntryDefaults.Runtime.Revision), []byte(next.EntryDefaults.Runtime.Revision))
	if err := os.WriteFile(templatePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	submission, err := store.Freeze(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Body.EntryDefaults.Runtime != next.EntryDefaults.Runtime || !seen.Matches(next.EntryDefaults.Runtime) {
		t.Fatalf("submitted Runtime=%+v seen=%+v want=%+v", submission.Body.EntryDefaults.Runtime, seen, next.EntryDefaults.Runtime)
	}
}

func TestAdoptHomeMovesBootstrapHomeToExactContextAndIsReplaySafe(t *testing.T) {
	state := t.TempDir()
	body := storeBodyFixture()
	store, err := New(filepath.Join(state, "configurator"), filepath.Join(state, "contexts"), resolverFixture{binding: body.EntryDefaults.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	draft, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	contextID := draft.AdoptionContextID
	canaryContents := []byte("synthetic-native-auth-state\n")
	canarySource := filepath.Join(state, "configurator", draft.ID, "home", ".codex", "auth.json")
	if err := os.WriteFile(canarySource, canaryContents, 0o600); err != nil {
		t.Fatal(err)
	}
	canaryBefore, err := os.Lstat(canarySource)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := store.Freeze(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ArmHomeAdoption(context.Background(), submission); err != nil {
		t.Fatal(err)
	}
	pending, found, err := store.PendingHomeAdoption(context.Background(), draft.ProjectRoot)
	if err != nil || found || pending.Validate() == nil {
		t.Fatalf("pre-release aggregate adoption must remain stored but invisible to task recovery: pending=%+v found=%v err=%v", pending, found, err)
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(draft.TemplateID, 1, submission.Body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: draft.TemplateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	workspace := tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: tobari.WorkspaceID("01912345-6789-7abc-8def-0123456789ae"), ContextID: contextID, ProjectRoot: draft.ProjectRoot, Home: filepath.Join(state, "contexts", string(contextID), "home"), CreationDefaults: revision.Slices.CreationDefaultsDigest}
	snapshot := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: draft.TemplateID}, Template: template, ContextHome: workspace.Home, ContextCreationDefaults: workspace.CreationDefaults, PolicyMemory: memory, Workspaces: []tobari.WorkspaceBinding{workspace}, Workspace: &workspace}
	replacementID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	if replacementID == contextID {
		replacementID = "01912345-6789-7abc-8def-0123456789ad"
	}
	replacementMemory, _, err := tobari.PublishPolicyMemory(replacementID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	replacement := snapshot.Clone()
	replacement.Context.ID = replacementID
	replacement.PolicyMemory = replacementMemory
	if err := store.AdoptHome(context.Background(), submission, replacement); err == nil {
		t.Fatal("replacement Context accepted the armed draft Home")
	}
	if err := store.AdoptHome(context.Background(), submission, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.AdoptHome(context.Background(), submission, snapshot); err != nil {
		t.Fatalf("same adoption was not replay-safe: %v", err)
	}
	journalPath := filepath.Join(state, "configurator", draft.ID, adoptionFile)
	if err := writeAtomicJSON(journalPath, homeAdoption{SchemaVersion: 1, Phase: "metadata_committed", Submission: submission, TemplateID: snapshot.Template.ID, TemplateRevision: snapshot.Template.Current.Revision, ContextID: contextID}); err != nil {
		t.Fatal(err)
	}
	if err := store.AdoptHome(context.Background(), submission, snapshot); err != nil {
		t.Fatalf("metadata-committed adoption did not settle its lingering journal: %v", err)
	}
	if _, err := os.Lstat(journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("settled adoption journal survived: %v", err)
	}
	target := filepath.Join(state, "contexts", string(contextID), "home")
	if _, err := os.Lstat(target); err != nil {
		t.Fatalf("adopted Context Home absent: %v", err)
	}
	canaryTarget := filepath.Join(target, ".codex", "auth.json")
	canaryAfter, err := os.Lstat(canaryTarget)
	if err != nil {
		t.Fatal(err)
	}
	canaryData, err := os.ReadFile(canaryTarget)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(canaryBefore, canaryAfter) || !bytes.Equal(canaryData, canaryContents) {
		t.Fatalf("native state canary changed across Home adoption: same_file=%t data=%q", os.SameFile(canaryBefore, canaryAfter), canaryData)
	}
	if _, err := os.Lstat(filepath.Join(state, "configurator", draft.ID, "home")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("draft Home survived adoption: %v", err)
	}
	if _, err := store.Freeze(context.Background(), draft); err == nil {
		t.Fatal("adopted mutable working copy was frozen again")
	}
}

func TestArmHomeAdoptionRejectsMissingFrozenManagedHome(t *testing.T) {
	state := t.TempDir()
	body := storeBodyFixture()
	store, err := New(filepath.Join(state, "configurator"), filepath.Join(state, "contexts"), resolverFixture{binding: body.EntryDefaults.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	draft, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := store.Freeze(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(state, "configurator", draft.ID, "home")
	if err := os.Rename(home, home+"-missing"); err != nil {
		t.Fatal(err)
	}
	if err := store.ArmHomeAdoption(context.Background(), submission); err == nil {
		t.Fatal("missing managed Home was armed for authority publication")
	}
	if _, err := os.Lstat(filepath.Join(state, "configurator", draft.ID, adoptionFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed Home validation wrote adoption receipt: %v", err)
	}
}

func TestArmHomeAdoptionRevalidatesExistingArmedReceiptHome(t *testing.T) {
	state := t.TempDir()
	body := storeBodyFixture()
	store, err := New(filepath.Join(state, "configurator"), filepath.Join(state, "contexts"), resolverFixture{binding: body.EntryDefaults.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	draft, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := store.Freeze(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ArmHomeAdoption(context.Background(), submission); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(state, "configurator", draft.ID, "home")
	if err := os.Rename(home, home+"-missing"); err != nil {
		t.Fatal(err)
	}
	if err := store.ArmHomeAdoption(context.Background(), submission); err == nil {
		t.Fatal("existing armed receipt bypassed missing managed Home validation")
	}
}

func TestUnknownHomeAdoptionPhaseFailsClosed(t *testing.T) {
	state := t.TempDir()
	body := storeBodyFixture()
	store, err := New(filepath.Join(state, "configurator"), filepath.Join(state, "contexts"), resolverFixture{binding: body.EntryDefaults.Runtime})
	if err != nil {
		t.Fatal(err)
	}
	seed, _ := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	draft, err := prepareTestDraft(context.Background(), store, seed, tobari.ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := store.Freeze(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ArmHomeAdoption(context.Background(), submission); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(state, "configurator", draft.ID, adoptionFile)
	journal, present, err := readAdoption(journalPath)
	if err != nil || !present {
		t.Fatalf("read armed journal: present=%t err=%v", present, err)
	}
	journal.Phase = "future"
	if err := writeAtomicJSON(journalPath, journal); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.PendingHomeAdoption(context.Background(), draft.ProjectRoot); err == nil {
		t.Fatal("unknown Home adoption phase was accepted")
	}
}

func storeBodyFixture() tobari.WorkspaceTemplateBody {
	return tobari.WorkspaceTemplateBody{
		Boundary:        tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadWrite, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []tobari.ManifestPolicyAuthority{}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}},
		Policy:          tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults:   tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Ordinal: 1, Image: "tobari-runtime:test"}},
		SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
}

func TestDurablePrivateDirectoryTreeCreatesOwnerOnlyAncestorChain(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "workspace-authority-runtime", "contexts", "context-id")
	if err := ensureDurablePrivateDirectoryTree(target); err != nil {
		t.Fatal(err)
	}
	for current := filepath.Join(parent, "workspace-authority-runtime"); ; current = filepath.Join(current, "contexts") {
		if _, err := requirePrivateDirectory(current); err != nil {
			t.Fatalf("durable ancestor %q is unsafe: %v", current, err)
		}
		if current == filepath.Join(parent, "workspace-authority-runtime", "contexts") {
			break
		}
	}
	if _, err := requirePrivateDirectory(target); err != nil {
		t.Fatalf("durable target is unsafe: %v", err)
	}
}
