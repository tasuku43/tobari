package workspaceauthoritystore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type mutationLifecycle struct {
	lock     sync.Mutex
	attempts atomic.Int64
	held     atomic.Bool
	before   func()
}

type mutationLegacyGuard struct{ err error }

func (g mutationLegacyGuard) ConfirmNoPreReleaseLegacyAuthority(context.Context, bool) error {
	return g.err
}

func (l *mutationLifecycle) WithLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	l.attempts.Add(1)
	l.lock.Lock()
	defer l.lock.Unlock()
	l.held.Store(true)
	defer l.held.Store(false)
	if l.before != nil {
		l.before()
	}
	return action(ctx)
}

type deletionAuthorityFixture struct {
	retired              []tobari.WorkspaceID
	retirementRefs       []string
	prepared             []tobari.WorkspaceID
	prepareRefs          []string
	credentialChecks     []tobari.ContextID
	workspaceErr         error
	confirmationErr      error
	contextCredentialErr error
	confirmations        int
	onPrepare            func()
	onRetire             func()
}

type templateRuntimeRevisionFixture struct {
	binding tobari.RuntimeBinding
	err     error
	refs    []string
	check   func() error
}

func (r *templateRuntimeRevisionFixture) ResolveWorkspaceTemplateRuntimeRevision(_ context.Context, ref string) (tobari.RuntimeBinding, error) {
	r.refs = append(r.refs, ref)
	if r.check != nil {
		if err := r.check(); err != nil {
			return tobari.RuntimeBinding{}, err
		}
	}
	if r.err != nil {
		return tobari.RuntimeBinding{}, r.err
	}
	if r.binding.RuntimeID != "" {
		return r.binding, nil
	}
	id, revision, err := tobari.ParseRuntimeRevisionRef(ref)
	if err != nil {
		return tobari.RuntimeBinding{}, err
	}
	return tobari.RuntimeBinding{RuntimeID: id, Name: "managed", Revision: revision, Ordinal: 1, Image: "tobari-runtime-managed:aaaaaaaaaaaa"}, nil
}

type policyActivationFixture struct {
	calls        int
	confirmCalls int
	err          error
	confirmErr   error
}

type finalSettlementFixture struct {
	activation            *policyActivationFixture
	calls                 int
	confirms              int
	contextDeleteCalls    int
	contextDeleteConfirms int
	reviewedCalls         int
	reviewedConfirms      int
	err                   error
	onSettle              func()
}

func reviewedSettlementFixtureReceipt(
	next tobari.WorkspaceAuthorityCollection,
	set tobari.PolicyMemoryReviewedDecisionSet,
) (tobari.PolicyMemoryReviewedSettlementReceipt, error) {
	targets, err := set.TargetContextIDs()
	if err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, err
	}
	projection, err := tobari.BuildReviewedWorkspacePolicyProjection(next, targets)
	if err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, err
	}
	digest := func(value string) tobari.SemanticDigest {
		return tobari.SemanticDigest("sha256:" + strings.Repeat(value, 64))
	}
	return tobari.PolicyMemoryReviewedSettlementReceipt{
		DecisionSetDigest: set.Digest, PlanDigest: projection.PlanDigest, ContentDigest: projection.ContentDigest,
		AggregateRevision: strings.Repeat("a", 64), PolicyArtifact: digest("b"),
		GatewayArtifact: digest("c"), PrincipalDigest: digest("d"),
	}, nil
}

func (s *finalSettlementFixture) SettleFinalReviewedPolicyAuthority(
	_ context.Context,
	previous, next tobari.WorkspaceAuthorityCollection,
	set tobari.PolicyMemoryReviewedDecisionSet,
	_, _ string,
) (tobari.PolicyMemoryReviewedSettlementReceipt, error) {
	if s.err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, s.err
	}
	if err := tobari.ValidatePolicyMemoryReviewedTransition(previous, next, set); err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, err
	}
	s.reviewedCalls++
	return reviewedSettlementFixtureReceipt(next, set)
}

func (s *finalSettlementFixture) ConfirmFinalReviewedPolicyAuthority(
	_ context.Context,
	current tobari.WorkspaceAuthorityCollection,
	set tobari.PolicyMemoryReviewedDecisionSet,
) (tobari.PolicyMemoryReviewedSettlementReceipt, error) {
	if s.err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, s.err
	}
	s.reviewedConfirms++
	return reviewedSettlementFixtureReceipt(current, set)
}

func (s *finalSettlementFixture) SettleFinalContextDeletion(_ context.Context, _, next tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID, _, _ string) error {
	if s.err != nil {
		return s.err
	}
	for _, record := range next.Contexts {
		if record.Context.ID == contextID {
			return fmt.Errorf("deleted Context remains")
		}
	}
	s.contextDeleteCalls++
	return nil
}

func (s *finalSettlementFixture) ConfirmFinalContextDeletionSettled(_ context.Context, next tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID) error {
	for _, record := range next.Contexts {
		if record.Context.ID == contextID {
			return fmt.Errorf("deleted Context remains")
		}
	}
	s.contextDeleteConfirms++
	return nil
}

func (s *finalSettlementFixture) SettleFinalAuthority(_ context.Context, _ tobari.WorkspaceAuthorityCollection, next tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID, _, _ string) error {
	if s.err != nil {
		return s.err
	}
	s.calls++
	_, err := s.activation.ActivatePolicyMemory(context.Background(), next, contextID)
	if err == nil && s.onSettle != nil {
		s.onSettle()
	}
	return err
}

func (s *finalSettlementFixture) ConfirmFinalAuthoritySettled(ctx context.Context, next tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID) error {
	s.confirms++
	snapshot, err := snapshotForContext(next, contextID)
	if err != nil {
		return err
	}
	if snapshot.ActivePolicyMemoryRef == nil {
		return fmt.Errorf("settled final authority omits active Policy Memory")
	}
	return s.activation.ConfirmPolicyMemoryActive(ctx, next, contextID, *snapshot.ActivePolicyMemoryRef)
}

func (a *policyActivationFixture) ConfirmPolicyMemoryActive(_ context.Context, collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID, receipt tobari.PolicyMemoryActivationReceipt) error {
	if a.confirmErr != nil {
		return a.confirmErr
	}
	a.confirmCalls++
	snapshot, err := snapshotForContext(collection, contextID)
	if err != nil {
		return err
	}
	if snapshot.ActivePolicyMemory == nil || snapshot.ActivePolicyMemoryRef == nil || *snapshot.ActivePolicyMemoryRef != receipt || receipt.Revision != snapshot.ActivePolicyMemory.Revision {
		return fmt.Errorf("Policy Memory active receipt mismatch")
	}
	return nil
}

func (a *policyActivationFixture) ActivatePolicyMemory(_ context.Context, collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID) (tobari.PolicyMemoryActivationReceipt, error) {
	if a.err != nil {
		return tobari.PolicyMemoryActivationReceipt{}, a.err
	}
	a.calls++
	projection, err := tobari.BuildHotWorkspacePolicyProjection(collection, contextID)
	if err != nil {
		return tobari.PolicyMemoryActivationReceipt{}, err
	}
	for _, item := range projection.Contexts {
		if item.ContextID == contextID {
			return item.MemoryReceipt, nil
		}
	}
	if contextID == "" {
		return tobari.PolicyMemoryActivationReceipt{}, fmt.Errorf("activation input mismatch")
	}
	return tobari.PolicyMemoryActivationReceipt{}, fmt.Errorf("activation target is unavailable")
}

func (d *deletionAuthorityFixture) PrepareWorkspaceRetirement(_ context.Context, workspace tobari.WorkspaceBinding, _ bool, decisionRef string) error {
	if d.onPrepare != nil {
		d.onPrepare()
	}
	d.prepared = append(d.prepared, workspace.ID)
	d.prepareRefs = append(d.prepareRefs, decisionRef)
	return nil
}

func TestWorkspaceDeletePreparesTargetBeforeGlobalSettlementAndCompletesAfterward(t *testing.T) {
	existing := storeCollectionFixture(t)
	_, mutator, _, deletion, _ := newMutationFixture(t, &existing)
	settlement := mutator.settlement.(*finalSettlementFixture)
	order := make([]string, 0, 3)
	deletion.onPrepare = func() { order = append(order, "prepare") }
	settlement.onSettle = func() { order = append(order, "settle") }
	deletion.onRetire = func() { order = append(order, "complete") }
	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)

	result, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, true)
	if err != nil || !result.Deleted {
		t.Fatalf("delete result=%#v err=%v", result, err)
	}
	if !reflect.DeepEqual(order, []string{"prepare", "settle", "complete"}) {
		t.Fatalf("delete effect order=%v", order)
	}
}

func (d *deletionAuthorityFixture) CompleteWorkspaceRetirement(_ context.Context, workspace tobari.WorkspaceBinding, _ bool, decisionRef string) error {
	if d.onRetire != nil {
		d.onRetire()
	}
	d.retired = append(d.retired, workspace.ID)
	d.retirementRefs = append(d.retirementRefs, decisionRef)
	return nil
}

func (d *deletionAuthorityFixture) ConfirmWorkspaceRetirementAllowed(_ context.Context, _ tobari.WorkspaceBinding, _ bool) error {
	return d.workspaceErr
}

func (d *deletionAuthorityFixture) ConfirmWorkspaceRetired(_ context.Context, workspace tobari.WorkspaceBinding, decisionRef string) error {
	if d.confirmationErr != nil {
		return d.confirmationErr
	}
	if len(d.retired) == 0 || d.retired[len(d.retired)-1] != workspace.ID || len(d.retirementRefs) == 0 || d.retirementRefs[len(d.retirementRefs)-1] != decisionRef {
		return fmt.Errorf("Workspace retirement receipt mismatch")
	}
	d.confirmations++
	return nil
}

func (d *deletionAuthorityFixture) ConfirmContextCredentialAbsent(_ context.Context, id tobari.ContextID) error {
	if d.contextCredentialErr != nil {
		return d.contextCredentialErr
	}
	d.credentialChecks = append(d.credentialChecks, id)
	return nil
}

func newMutationFixture(t *testing.T, existing *tobari.WorkspaceAuthorityCollection) (*Store, *Mutator, *mutationLifecycle, *deletionAuthorityFixture, *policyActivationFixture) {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "workspace-authority")
	if existing != nil {
		materializeCollection(t, root, existing.Clone())
	}
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	store.legacyGuard = mutationLegacyGuard{}
	lifecycle := &mutationLifecycle{}
	deletion := &deletionAuthorityFixture{}
	activation := &policyActivationFixture{}
	settlement := &finalSettlementFixture{activation: activation}
	mutator, err := NewMutator(context.Background(), store, lifecycle, &templateRuntimeRevisionFixture{}, deletion, activation, settlement)
	if err != nil {
		t.Fatal(err)
	}
	mutator.clock = func() time.Time { return time.UnixMilli(1720000000000).UTC() }
	entropy := make([]byte, 2048)
	for index := range entropy {
		entropy[index] = byte(index + 1)
	}
	mutator.entropy = bytes.NewReader(entropy)
	return store, mutator, lifecycle, deletion, activation
}

func TestNewMutatorRejectsUnguardedStore(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "workspace-authority"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewMutator(context.Background(), store, &mutationLifecycle{}, &templateRuntimeRevisionFixture{}, &deletionAuthorityFixture{}, &policyActivationFixture{}, &finalSettlementFixture{}); err == nil || !strings.Contains(err.Error(), "final-only guarded") {
		t.Fatalf("unguarded mutator error=%v", err)
	}
}

func reviewedSetForCandidates(
	t *testing.T,
	collection tobari.WorkspaceAuthorityCollection,
	candidates ...tobari.PolicyCandidateAuthority,
) tobari.PolicyMemoryReviewedDecisionSet {
	t.Helper()
	decisions := make([]tobari.PolicyMemoryReviewedDecision, len(candidates))
	for index, candidate := range candidates {
		decision, err := tobari.NewPolicyMemoryReviewedDecision(
			candidate.ID, []tobari.PolicyCandidateAuthority{candidate}, nil, tobari.PolicyMemoryAllow,
			candidate.Effect.RuleBody(candidate.ID),
		)
		if err != nil {
			t.Fatal(err)
		}
		decisions[index] = decision
	}
	set, err := tobari.NewPolicyMemoryReviewedDecisionSet(collection, decisions)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func twoContextReviewedCollection(t *testing.T) (tobari.WorkspaceAuthorityCollection, tobari.PolicyCandidateAuthority, tobari.PolicyCandidateAuthority) {
	t.Helper()
	first := storeCollectionFixture(t)
	secondTemplateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789b1")
	secondContextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789b2")
	secondWorkspaceID := tobari.WorkspaceID("01912345-6789-7abc-8def-0123456789b3")
	secondTemplate, err := tobari.CopyWorkspaceTemplateRevision(secondTemplateID, "second", first.Templates[0].Current)
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := tobari.PublishPolicyMemory(secondContextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	binding := tobari.ContextBinding{
		SchemaVersion: tobari.ContextBindingSchemaVersion, ID: secondContextID,
		ProjectRoot: first.Contexts[0].Context.ProjectRoot, TemplateID: secondTemplateID,
	}
	templateReceipt := tobari.TemplatePolicyActivationReceipt{
		ContextID: secondContextID, TemplateID: secondTemplateID,
		PolicySliceDigest: secondTemplate.Current.Slices.PolicySliceDigest,
	}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: secondContextID, Revision: memory.Revision}
	active := memory.Clone()
	secondRecord := tobari.WorkspaceAuthorityContextRecord{
		Context: binding, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt,
		ActivePolicyMemory: &active, ActivePolicyMemoryRef: &memoryReceipt,
	}
	applied := tobari.WorkspaceAppliedEntry{
		ContextID: secondContextID, TemplateID: secondTemplateID, TemplateRevision: secondTemplate.Current.Revision,
		EntrySliceDigest: secondTemplate.Current.Slices.EntrySliceDigest, RuntimeID: secondTemplate.Current.Slices.RuntimeID,
		RuntimeRevision: secondTemplate.Current.Slices.RuntimeRevision,
		ResolvedSpec:    tobari.SemanticDigest("sha256:" + strings.Repeat("8", 64)), ReconciledAt: time.Unix(2, 0).UTC(),
	}
	secondWorkspace := tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: secondWorkspaceID, ContextID: secondContextID,
		ProjectRoot: binding.ProjectRoot, Home: "/workspace/home-" + string(secondWorkspaceID),
		CreationDefaults: secondTemplate.Current.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied,
	}
	effect := first.PendingCandidates[0].Effect
	effect.Path = "/second-candidate"
	effect.Examples = []string{effect.Path}
	secondCandidate, err := tobari.NewPolicyCandidateAuthority(secondContextID, secondWorkspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}
	pending := []tobari.PolicyCandidateAuthority{first.PendingCandidates[0].Clone(), secondCandidate}
	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })
	defaultID := storeTemplateID
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{first.Templates[0].Clone(), secondTemplate},
		[]tobari.WorkspaceAuthorityContextRecord{first.Contexts[0].Clone(), secondRecord},
		[]tobari.WorkspaceBinding{first.Workspaces[0], secondWorkspace}, pending, &defaultID, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return collection, first.PendingCandidates[0], secondCandidate
}

func addPendingCandidate(
	t *testing.T,
	mutator *Mutator,
	path string,
) (tobari.WorkspaceAuthorityCollection, tobari.PolicyCandidateAuthority) {
	t.Helper()
	effect := tobari.PolicyCandidateEffect{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Match:                  tobari.PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: path,
		Segments: []string{}, Examples: []string{path},
	}
	candidate, err := tobari.NewPolicyCandidateAuthority(storeContextID, storeWorkspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}
	err = mutator.mutate(context.Background(), func(_ context.Context, current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
		pending := append(clonePolicyCandidates(current.PendingCandidates), candidate)
		return publishCollection(current, present, current.Templates, current.Contexts, current.Workspaces, pending, current.DefaultTemplateID)
	})
	if err != nil {
		t.Fatal(err)
	}
	current, present, err := mutator.store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("read collection with pending candidate: present=%t err=%v", present, err)
	}
	return current, candidate
}

func TestMutatorCreatesCopiesSelectsAndDeletesTemplatesThroughOneEnvelope(t *testing.T) {
	store, mutator, lifecycle, _, _ := newMutationFixture(t, nil)
	body := storeCollectionFixture(t).Templates[0].Current.Body.Clone()

	created, err := mutator.CreateWorkspaceTemplate(context.Background(), "restricted", body)
	if err != nil {
		t.Fatal(err)
	}
	if created.Current.Generation != 1 || created.Current.Body.Policy.BaselineGrants[0].Path != "/items" {
		t.Fatalf("created=%#v", created)
	}
	first, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || first.Generation != 1 || len(first.Templates) != 1 {
		t.Fatalf("first=%#v present=%t err=%v", first, present, err)
	}

	templateRef, _ := tobari.WorkspaceTemplateRef(created.ID)
	value := "xterm-256color"
	change := tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeShell, Shell: []tobari.ManifestShellEnvironmentSetting{{Variable: "TERM", Source: tobari.ManifestShellEnvironmentLiteral, Value: &value}}}
	updated, err := mutator.UpdateWorkspaceTemplateByReference(context.Background(), templateRef, change)
	if err != nil || !updated.Changed || updated.Previous.Revision != created.Current.Revision || updated.Current.Generation != 2 || len(updated.Template.Retained) != 2 {
		t.Fatalf("updated=%#v err=%v", updated, err)
	}
	noChange, err := mutator.UpdateWorkspaceTemplateByReference(context.Background(), templateRef, change)
	if err != nil || noChange.Changed || noChange.Current.Revision != updated.Current.Revision {
		t.Fatalf("no-op update=%#v err=%v", noChange, err)
	}
	selected, err := mutator.SetDefaultWorkspaceTemplateByReference(context.Background(), templateRef)
	if err != nil || !selected.Selected || selected.TemplateID != created.ID {
		t.Fatalf("selected=%#v err=%v", selected, err)
	}
	selectedCollection, _, _ := store.ReadComplete(context.Background())
	if selectedCollection.Generation != first.Generation+2 {
		t.Fatalf("selection generation=%d", selectedCollection.Generation)
	}
	if _, err := mutator.SetDefaultWorkspaceTemplateByReference(context.Background(), templateRef); err != nil {
		t.Fatal(err)
	}
	noOp, _, _ := store.ReadComplete(context.Background())
	if noOp.Generation != selectedCollection.Generation || noOp.Revision != selectedCollection.Revision {
		t.Fatalf("semantic no-op advanced collection: %#v", noOp)
	}

	revisionRef, _ := tobari.WorkspaceTemplateRevisionRef(created.ID, created.Current.Revision)
	copyPublication, err := mutator.CopyWorkspaceTemplateByRevisionReference(context.Background(), revisionRef, "copied")
	if err != nil {
		t.Fatal(err)
	}
	if copyPublication.Source.Revision != created.Current.Revision || copyPublication.Created.ID == created.ID || copyPublication.Created.Current.Generation != 1 {
		t.Fatalf("copy=%#v", copyPublication)
	}
	copyRef, _ := tobari.WorkspaceTemplateRef(copyPublication.Created.ID)
	if _, err := mutator.DeleteWorkspaceTemplateByReference(context.Background(), templateRef); !errors.Is(err, tobari.ErrWorkspaceTemplateProtected) {
		t.Fatalf("default Template delete err=%v", err)
	}
	if _, err := mutator.SetDefaultWorkspaceTemplateByReference(context.Background(), copyRef); err != nil {
		t.Fatal(err)
	}
	deleted, err := mutator.DeleteWorkspaceTemplateByReference(context.Background(), templateRef)
	if err != nil || !deleted.Deleted || deleted.TemplateID != created.ID {
		t.Fatalf("deleted=%#v err=%v", deleted, err)
	}
	final, _, err := store.ReadComplete(context.Background())
	if err != nil || len(final.Templates) != 1 || final.Templates[0].ID != copyPublication.Created.ID {
		t.Fatalf("final=%#v err=%v", final, err)
	}
	if final.DefaultTemplateID == nil || *final.DefaultTemplateID != copyPublication.Created.ID {
		t.Fatalf("deleting nondefault Template cleared default: %#v", final.DefaultTemplateID)
	}
	if lifecycle.attempts.Load() != 9 {
		t.Fatalf("lifecycle attempts=%d", lifecycle.attempts.Load())
	}
}

func TestWorkspaceTemplateTypedChangesSerializeWithoutRevertingUnrelatedFields(t *testing.T) {
	store, mutator, _, _, _ := newMutationFixture(t, nil)
	created, err := mutator.CreateWorkspaceTemplate(context.Background(), "serialized", storeCollectionFixture(t).Templates[0].Current.Body)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := tobari.WorkspaceTemplateRef(created.ID)
	shellValue := "xterm-256color"
	name, email := "Example User", "user@example.com"
	shell := tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeShell, Shell: []tobari.ManifestShellEnvironmentSetting{{Variable: "TERM", Source: tobari.ManifestShellEnvironmentLiteral, Value: &shellValue}}}
	git := tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeGit, Git: &tobari.ManifestGitIdentitySetting{Source: tobari.ManifestGitIdentityLiteral, Name: &name, Email: &email}}

	start := make(chan struct{})
	errors := make(chan error, 2)
	for _, change := range []tobari.WorkspaceTemplateChange{shell, git} {
		change := change.Clone()
		go func() {
			<-start
			_, err := mutator.UpdateWorkspaceTemplateByReference(context.Background(), ref, change)
			errors <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	collection, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("read serialized Template: present=%t err=%v", present, err)
	}
	body := collection.Templates[0].Current.Body
	if len(body.SessionDefaults.ShellEnvironment) != 1 || body.SessionDefaults.ShellEnvironment[0].Variable != "TERM" ||
		body.SessionDefaults.GitIdentity == nil || body.SessionDefaults.GitIdentity.Name == nil || *body.SessionDefaults.GitIdentity.Name != name {
		t.Fatalf("unrelated serialized changes were not both retained: %#v", body.SessionDefaults)
	}

	lastValue := "screen-256color"
	last := tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeShell, Shell: []tobari.ManifestShellEnvironmentSetting{{Variable: "TERM", Source: tobari.ManifestShellEnvironmentLiteral, Value: &lastValue}}}
	if _, err := mutator.UpdateWorkspaceTemplateByReference(context.Background(), ref, last); err != nil {
		t.Fatal(err)
	}
	collection, _, err = store.ReadComplete(context.Background())
	if err != nil || collection.Templates[0].Current.Body.SessionDefaults.ShellEnvironment[0].Value == nil ||
		*collection.Templates[0].Current.Body.SessionDefaults.ShellEnvironment[0].Value != lastValue {
		t.Fatalf("same-field last successful change was not current: %#v err=%v", collection.Templates[0].Current.Body.SessionDefaults, err)
	}
}

func TestWorkspaceTemplateRuntimeChangeResolvesExactRevisionUnderLifecycleBeforeWrite(t *testing.T) {
	store, mutator, lifecycle, _, _ := newMutationFixture(t, nil)
	created, err := mutator.CreateWorkspaceTemplate(context.Background(), "runtime", storeCollectionFixture(t).Templates[0].Current.Body)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := tobari.WorkspaceTemplateRef(created.ID)
	runtimeID := "01912345-6789-7abc-8def-0123456789b7"
	revision := "sha256:" + strings.Repeat("b", 64)
	revisionRef := tobari.RuntimeRevisionRef(runtimeID, revision)
	resolver := &templateRuntimeRevisionFixture{
		binding: tobari.RuntimeBinding{RuntimeID: runtimeID, Name: "managed", Revision: revision, Ordinal: 3, Image: "tobari-runtime-managed:bbbbbbbbbbbb"},
		check: func() error {
			if !lifecycle.held.Load() {
				return errors.New("Runtime revision was resolved outside lifecycle authority")
			}
			return nil
		},
	}
	mutator.runtimeRevision = resolver
	change := tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeRuntime, RuntimeRevisionRef: revisionRef}
	publication, err := mutator.UpdateWorkspaceTemplateByReference(context.Background(), ref, change)
	if err != nil || !publication.Changed || publication.ResolvedRuntime == nil || publication.Current.Body.EntryDefaults.Runtime.Revision != revision ||
		len(resolver.refs) != 1 || resolver.refs[0] != revisionRef || lifecycle.held.Load() {
		t.Fatalf("Runtime publication=%#v refs=%v held-after=%t err=%v", publication, resolver.refs, lifecycle.held.Load(), err)
	}

	before, _, err := store.ReadComplete(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	resolver.err = errors.New("Runtime revision observation unknown")
	otherRef := tobari.RuntimeRevisionRef(runtimeID, "sha256:"+strings.Repeat("c", 64))
	if _, err := mutator.UpdateWorkspaceTemplateByReference(context.Background(), ref, tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeRuntime, RuntimeRevisionRef: otherRef}); err == nil {
		t.Fatal("unknown Runtime revision observation reached collection publication")
	}
	after, _, err := store.ReadComplete(context.Background())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("Runtime observation failure changed collection: err=%v", err)
	}
}

func TestMutatorContextCreateDeleteAndWorkspaceRetirementPreserveOwners(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, deletion, _ := newMutationFixture(t, &existing)
	templateRef, _ := tobari.WorkspaceTemplateRef(storeTemplateID)
	if _, err := mutator.CreateContextByTemplateReference(context.Background(), templateRef, "/workspace/example"); !errors.Is(err, tobari.ErrContextBindingExists) {
		t.Fatalf("duplicate Context err=%v", err)
	}

	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	workspaceResult, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, true)
	if err != nil || !workspaceResult.Deleted || len(deletion.retired) != 1 || deletion.retired[0] != storeWorkspaceID {
		t.Fatalf("workspace result=%#v retired=%#v err=%v", workspaceResult, deletion.retired, err)
	}
	afterWorkspace, _, err := store.ReadComplete(context.Background())
	if err != nil || len(afterWorkspace.Workspaces) != 0 || len(afterWorkspace.Contexts) != 1 || len(afterWorkspace.PendingCandidates) != 0 {
		t.Fatalf("after Workspace=%#v err=%v", afterWorkspace, err)
	}
	if afterWorkspace.Contexts[0].PolicyMemory.Revision != existing.Contexts[0].PolicyMemory.Revision {
		t.Fatal("Workspace deletion changed Policy Memory")
	}

	contextRef, _ := tobari.ContextRef(storeContextID)
	settlement := mutator.settlement.(*finalSettlementFixture)
	contextResult, err := mutator.DeleteContextByReference(context.Background(), contextRef)
	if err != nil || !contextResult.Deleted || len(deletion.credentialChecks) != 1 || deletion.credentialChecks[0] != storeContextID || settlement.contextDeleteCalls != 1 {
		t.Fatalf("Context result=%#v checks=%#v err=%v", contextResult, deletion.credentialChecks, err)
	}
	afterContext, _, err := store.ReadComplete(context.Background())
	if err != nil || len(afterContext.Contexts) != 0 || len(afterContext.Workspaces) != 0 || len(afterContext.Templates) != 1 {
		t.Fatalf("after Context=%#v err=%v", afterContext, err)
	}
}

func TestContextDeleteSettlementDecisionResumesAndReplaysExactOutcome(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	if _, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, true); err != nil {
		t.Fatal(err)
	}
	settlement := mutator.settlement.(*finalSettlementFixture)
	settlement.contextDeleteCalls = 0

	realRename := mutator.rename
	stage := store.root + ".wp11-mutation-stage"
	authority := filepath.Join(store.root, authorityFileName)
	mutator.rename = func(source, target string) error {
		if source == stage && target == authority {
			return fmt.Errorf("injected death after Context aggregate settlement")
		}
		return realRename(source, target)
	}
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := mutator.DeleteContextByReference(context.Background(), contextRef); err == nil {
		t.Fatal("interrupted Context deletion was reported as complete")
	}
	current, _, err := store.ReadComplete(context.Background())
	if err != nil || contextRecordIndex(current, storeContextID) < 0 || settlement.contextDeleteCalls != 1 {
		t.Fatalf("interrupted Context authority=%#v settlements=%d err=%v", current, settlement.contextDeleteCalls, err)
	}
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "blocked-by-context-delete", existing.Templates[0].Current.Body); err == nil {
		t.Fatal("different mutation crossed active Context deletion")
	}

	mutator.rename = realRename
	result, err := mutator.DeleteContextByReference(context.Background(), contextRef)
	if err != nil || !result.Deleted || result.ContextID != storeContextID || settlement.contextDeleteCalls != 2 {
		t.Fatalf("resumed Context delete=%#v settlements=%d err=%v", result, settlement.contextDeleteCalls, err)
	}
	calls := settlement.contextDeleteCalls
	replayed, err := mutator.DeleteContextByReference(context.Background(), contextRef)
	if err != nil || !replayed.Deleted || replayed.ContextID != storeContextID || settlement.contextDeleteCalls != calls || settlement.contextDeleteConfirms != 1 {
		t.Fatalf("terminal Context replay=%#v settlements=%d confirms=%d err=%v", replayed, settlement.contextDeleteCalls, settlement.contextDeleteConfirms, err)
	}
}

func TestMutatorDeletionFailsClosedBeforeEnvelopeChange(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, deletion, _ := newMutationFixture(t, &existing)
	before, _, _ := store.ReadComplete(context.Background())

	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := mutator.DeleteContextByReference(context.Background(), contextRef); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("Context with Workspace err=%v", err)
	}
	deletion.workspaceErr = tobari.ErrWorkspaceBindingProtected
	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	if _, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, false); !errors.Is(err, tobari.ErrWorkspaceBindingProtected) {
		t.Fatalf("attached Workspace err=%v", err)
	}
	deletion.workspaceErr = nil
	templateRef, _ := tobari.WorkspaceTemplateRef(storeTemplateID)
	if _, err := mutator.DeleteWorkspaceTemplateByReference(context.Background(), templateRef); !errors.Is(err, tobari.ErrWorkspaceTemplateProtected) {
		t.Fatalf("protected Template err=%v", err)
	}
	after, _, _ := store.ReadComplete(context.Background())
	if after.Generation != before.Generation || after.Revision != before.Revision {
		t.Fatalf("blocked deletion changed authority: before=%#v after=%#v", before, after)
	}
}

func TestMutatorPublishesExactCandidateAndResetAuthority(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, activation := newMutationFixture(t, &existing)
	candidate := existing.PendingCandidates[0]

	publication, err := mutator.AllowPolicyCandidateByReference(context.Background(), candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := publication.ValidateFor(candidate.ID, tobari.PolicyMemoryAllow); err != nil {
		t.Fatal(err)
	}
	if activation.calls != 1 {
		t.Fatalf("activation calls=%d", activation.calls)
	}
	afterAllow, _, err := store.ReadComplete(context.Background())
	if err != nil || len(afterAllow.PendingCandidates) != 0 || len(afterAllow.Contexts[0].PolicyMemory.Rules) != 1 {
		t.Fatalf("after allow=%#v err=%v", afterAllow, err)
	}
	if afterAllow.Contexts[0].ActivePolicyMemory == nil || afterAllow.Contexts[0].ActivePolicyMemory.Revision != afterAllow.Contexts[0].PolicyMemory.Revision {
		t.Fatal("direct decision did not record exact confirmed active Policy Memory")
	}

	reset, err := mutator.ResetPolicyMemoryRuleByReference(context.Background(), publication.RuleID)
	if err != nil {
		t.Fatal(err)
	}
	if err := reset.ValidateFor(publication.RuleID); err != nil {
		t.Fatal(err)
	}
	if activation.calls != 2 {
		t.Fatalf("activation calls=%d", activation.calls)
	}
	afterReset, _, err := store.ReadComplete(context.Background())
	if err != nil || len(afterReset.Contexts[0].PolicyMemory.Rules) != 0 {
		t.Fatalf("after reset=%#v err=%v", afterReset, err)
	}
	if _, err := mutator.AllowPolicyCandidateByReference(context.Background(), candidate.ID); !errors.Is(err, tobari.ErrPolicyMemoryTargetNotFound) {
		t.Fatalf("consumed candidate err=%v", err)
	}
}

func TestMutatorClassifiesPostRenameSuccessAndPreRenameFailure(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	body := existing.Templates[0].Current.Body.Clone()

	realRename := mutator.rename
	mutator.rename = func(source, target string) error {
		if err := realRename(source, target); err != nil {
			return err
		}
		return fmt.Errorf("injected post-rename failure")
	}
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "post-rename", body); err != nil {
		t.Fatalf("exact read-back did not classify confirmed publication: %v", err)
	}
	confirmed, _, _ := store.ReadComplete(context.Background())
	if len(confirmed.Templates) != 2 {
		t.Fatalf("confirmed=%#v", confirmed)
	}

	mutator.rename = func(string, string) error { return fmt.Errorf("injected pre-rename failure") }
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "pre-rename", body); err == nil {
		t.Fatal("pre-rename failure was reported as success")
	}
	after, _, _ := store.ReadComplete(context.Background())
	if after.Generation != confirmed.Generation || after.Revision != confirmed.Revision {
		t.Fatalf("pre-rename failure changed authority: %#v", after)
	}
	stageInfo, err := os.Lstat(store.root + ".wp11-mutation-stage")
	if err != nil || !stageInfo.Mode().IsRegular() {
		t.Fatalf("recoverable stage info=%#v err=%v", stageInfo, err)
	}
	mutator.rename = realRename
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "retry", body); err != nil {
		t.Fatalf("next lifecycle mutation did not reconcile stage: %v", err)
	}
}

func TestMutatorFirstPublicationStageIsRecoverableAndReadersSeeNoPartialRoot(t *testing.T) {
	store, mutator, _, _, _ := newMutationFixture(t, nil)
	stage := store.root + ".wp11-mutation-stage"
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, authorityFileName), []byte(`{"partial":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, present, err := store.ReadComplete(context.Background()); err != nil || present {
		t.Fatalf("partial stage affected ordinary reader: present=%t err=%v", present, err)
	}
	body := storeCollectionFixture(t).Templates[0].Current.Body.Clone()
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "recovered", body); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stage remained after publication: %v", err)
	}
	if collection, present, err := store.ReadComplete(context.Background()); err != nil || !present || len(collection.Templates) != 1 {
		t.Fatalf("collection=%#v present=%t err=%v", collection, present, err)
	}
}

func TestMutatorSerializesConcurrentFinalEnvelopeChanges(t *testing.T) {
	store, mutator, lifecycle, _, _ := newMutationFixture(t, nil)
	body := storeCollectionFixture(t).Templates[0].Current.Body.Clone()
	start := make(chan struct{})
	errorsByCall := make(chan error, 2)
	for _, name := range []string{"one", "two"} {
		name := name
		go func() {
			<-start
			_, err := mutator.CreateWorkspaceTemplate(context.Background(), name, body)
			errorsByCall <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-errorsByCall; err != nil {
			t.Fatal(err)
		}
	}
	collection, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || len(collection.Templates) != 2 || collection.Generation != 2 {
		t.Fatalf("collection=%#v present=%t err=%v", collection, present, err)
	}
	if lifecycle.attempts.Load() != 2 {
		t.Fatalf("lifecycle attempts=%d", lifecycle.attempts.Load())
	}
}

func TestMutatorRejectsUnsafeReservedStageWithoutChangingAuthority(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	foreign := filepath.Join(filepath.Dir(store.root), "foreign")
	if err := os.WriteFile(foreign, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreign, store.root+".wp11-mutation-stage"); err != nil {
		t.Fatal(err)
	}
	body := existing.Templates[0].Current.Body.Clone()
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "blocked", body); err == nil {
		t.Fatal("unsafe stage was accepted")
	}
	after, _, _ := store.ReadComplete(context.Background())
	if after.Generation != existing.Generation || after.Revision != existing.Revision {
		t.Fatal("unsafe stage changed final authority")
	}
}

func TestWorkspaceDeleteDecisionBlocksOtherMutationAndSameRefResumesAfterEffect(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, deletion, _ := newMutationFixture(t, &existing)
	realRename := mutator.rename
	stage := store.root + ".wp11-mutation-stage"
	authority := filepath.Join(store.root, authorityFileName)
	mutator.rename = func(source, target string) error {
		if source == stage && target == authority {
			return fmt.Errorf("injected death after external retirement")
		}
		return realRename(source, target)
	}
	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	if _, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, true); err == nil {
		t.Fatal("interrupted delete was reported as complete")
	}
	if len(deletion.retired) != 1 || deletion.confirmations != 1 {
		t.Fatalf("retirements=%d confirmations=%d", len(deletion.retired), deletion.confirmations)
	}
	if _, present, err := mutator.readEffectDecision(); err != nil || !present {
		t.Fatalf("active decision present=%t err=%v", present, err)
	}
	body := existing.Templates[0].Current.Body.Clone()
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "blocked-by-delete", body); err == nil {
		t.Fatal("different mutation crossed active delete decision")
	}
	current, _, _ := store.ReadComplete(context.Background())
	if len(current.Workspaces) != 1 {
		t.Fatal("failed publication changed the envelope")
	}

	mutator.rename = realRename
	// Once retirement is confirmed, missing retired material is not a fresh
	// precondition failure; exact active-decision recovery uses the receipt.
	deletion.workspaceErr = tobari.ErrWorkspaceBindingProtected
	result, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, true)
	if err != nil || !result.Deleted || result.WorkspaceID != storeWorkspaceID {
		t.Fatalf("resumed result=%#v err=%v", result, err)
	}
	if len(deletion.retired) != 2 || deletion.confirmations != 2 {
		t.Fatalf("resume did not re-observe exact retirement: retirements=%d confirmations=%d", len(deletion.retired), deletion.confirmations)
	}
	if _, present, err := mutator.readEffectDecision(); err != nil || present {
		t.Fatalf("active decision present=%t err=%v", present, err)
	}
	if _, present, err := mutator.readTerminalEffectDecision(); err != nil || !present {
		t.Fatalf("terminal receipt present=%t err=%v", present, err)
	}

	retirements := len(deletion.retired)
	replayed, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, true)
	if err != nil || !replayed.Deleted || len(deletion.retired) != retirements || deletion.confirmations != 3 {
		t.Fatalf("terminal replay=%#v retirements=%d confirmations=%d err=%v", replayed, len(deletion.retired), deletion.confirmations, err)
	}
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "after-delete", body); err != nil {
		t.Fatalf("bounded terminal receipt blocked a different later mutation: %v", err)
	}
	retirements = len(deletion.retired)
	replayedAfterUnrelated, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, true)
	if err != nil || !replayedAfterUnrelated.Deleted || len(deletion.retired) != retirements || deletion.confirmations != 4 {
		t.Fatalf("replay after unrelated mutation=%#v retirements=%d confirmations=%d err=%v", replayedAfterUnrelated, len(deletion.retired), deletion.confirmations, err)
	}
}

func TestWorkspaceDeleteRecoversAfterEnvelopePublicationBeforeTerminalReceipt(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, deletion, _ := newMutationFixture(t, &existing)
	realRename := mutator.rename
	mutator.rename = func(source, target string) error {
		if source == mutator.effectDecisionPath() && target == mutator.effectDecisionDonePath() {
			return fmt.Errorf("injected death before terminal receipt")
		}
		return realRename(source, target)
	}
	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	if _, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, false); err == nil {
		t.Fatal("terminal transition failure was reported as success")
	}
	current, _, err := store.ReadComplete(context.Background())
	if err != nil || len(current.Workspaces) != 0 {
		t.Fatalf("confirmed envelope=%#v err=%v", current, err)
	}
	if len(deletion.retired) != 1 || deletion.confirmations != 1 {
		t.Fatalf("retirements=%d confirmations=%d", len(deletion.retired), deletion.confirmations)
	}
	mutator.rename = realRename
	result, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, false)
	if err != nil || !result.Deleted || len(deletion.retired) != 1 || deletion.confirmations != 2 {
		t.Fatalf("post-publication recovery=%#v retirements=%d confirmations=%d err=%v", result, len(deletion.retired), deletion.confirmations, err)
	}
}

func TestWorkspaceDeleteReplaysWhenTerminalRenameSucceededButReportedError(t *testing.T) {
	existing := storeCollectionFixture(t)
	_, mutator, _, deletion, _ := newMutationFixture(t, &existing)
	realRename := mutator.rename
	mutator.rename = func(source, target string) error {
		if source == mutator.effectDecisionPath() && target == mutator.effectDecisionDonePath() {
			if err := realRename(source, target); err != nil {
				return err
			}
			return fmt.Errorf("injected post-terminal-rename uncertainty")
		}
		return realRename(source, target)
	}
	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	if _, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, false); err == nil {
		t.Fatal("post-terminal-rename uncertainty was reported as success")
	}
	if _, active, err := mutator.readEffectDecision(); err != nil || active {
		t.Fatalf("active decision remained=%t err=%v", active, err)
	}
	if terminal, present, err := mutator.readTerminalEffectDecision(); err != nil || !present || terminal.Target != workspaceRef {
		t.Fatalf("terminal=%#v present=%t err=%v", terminal, present, err)
	}
	if len(deletion.retired) != 1 || deletion.confirmations != 1 {
		t.Fatalf("retirements=%d confirmations=%d", len(deletion.retired), deletion.confirmations)
	}
	mutator.rename = realRename
	result, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, false)
	if err != nil || !result.Deleted || len(deletion.retired) != 1 || deletion.confirmations != 2 {
		t.Fatalf("terminal replay=%#v retirements=%d confirmations=%d err=%v", result, len(deletion.retired), deletion.confirmations, err)
	}
}

func TestPolicyDecisionUsesDurableEffectDecisionAndTerminalReplay(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, activation := newMutationFixture(t, &existing)
	realRename := mutator.rename
	stage := store.root + ".wp11-mutation-stage"
	authority := filepath.Join(store.root, authorityFileName)
	mutator.rename = func(source, target string) error {
		if source == stage && target == authority {
			return fmt.Errorf("injected death after Policy Memory activation")
		}
		return realRename(source, target)
	}
	candidate := existing.PendingCandidates[0]
	if _, err := mutator.AllowPolicyCandidateByReference(context.Background(), candidate.ID); err == nil {
		t.Fatal("interrupted Policy publication was reported as complete")
	}
	if activation.calls != 1 {
		t.Fatalf("activation calls=%d", activation.calls)
	}
	body := existing.Templates[0].Current.Body.Clone()
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "blocked-by-policy", body); err == nil {
		t.Fatal("different mutation crossed active Policy decision")
	}

	mutator.rename = realRename
	publication, err := mutator.AllowPolicyCandidateByReference(context.Background(), candidate.ID)
	if err != nil || publication.Candidate.ID != candidate.ID || activation.calls != 2 {
		t.Fatalf("resumed publication=%#v calls=%d err=%v", publication, activation.calls, err)
	}
	if err := publication.ValidateFor(candidate.ID, tobari.PolicyMemoryAllow); err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "unrelated-after-policy", body); err != nil {
		t.Fatalf("unrelated mutation after Policy outcome: %v", err)
	}
	calls := activation.calls
	replayed, err := mutator.AllowPolicyCandidateByReference(context.Background(), candidate.ID)
	if err != nil || replayed.RuleID != publication.RuleID || activation.calls != calls || activation.confirmCalls != 1 {
		t.Fatalf("terminal replay=%#v calls=%d confirms=%d err=%v", replayed, activation.calls, activation.confirmCalls, err)
	}
}

func TestApplyReviewedPublishesOneSetAndReplaysAcrossUnrelatedPureMutation(t *testing.T) {
	existing := storeCollectionFixture(t)
	_, mutator, _, _, _ := newMutationFixture(t, &existing)
	settlement := mutator.settlement.(*finalSettlementFixture)
	set := reviewedSetForCandidates(t, existing, existing.PendingCandidates[0])

	publication, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set)
	if err != nil || publication.Validate() != nil || settlement.reviewedCalls != 1 || len(publication.AppliedDecisions) != 1 {
		t.Fatalf("publication=%#v calls=%d err=%v validate=%v", publication, settlement.reviewedCalls, err, publication.Validate())
	}
	if publication.DecisionSet.Decisions[0].ReviewItemID != set.Decisions[0].ReviewItemID ||
		publication.DecisionSet.Decisions[0].ProposalDigest != set.Decisions[0].ProposalDigest {
		t.Fatal("durable reviewed result lost stable item or complete evidence identity")
	}
	body := existing.Templates[0].Current.Body.Clone()
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "unrelated-after-reviewed", body); err != nil {
		t.Fatal(err)
	}
	calls := settlement.reviewedCalls
	replayed, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set)
	if err != nil || replayed.Validate() != nil || settlement.reviewedCalls != calls || settlement.reviewedConfirms != 1 ||
		!reflect.DeepEqual(replayed.AppliedDecisions, publication.AppliedDecisions) || replayed.ActiveRevision != publication.ActiveRevision || !replayed.Changed {
		t.Fatalf("replay=%#v calls=%d confirms=%d err=%v validate=%v", replayed, settlement.reviewedCalls, settlement.reviewedConfirms, err, replayed.Validate())
	}
}

func TestApplyReviewedSettlesTwoContextAllowAndDenyAsOneGlobalMutation(t *testing.T) {
	existing, firstCandidate, secondCandidate := twoContextReviewedCollection(t)
	_, mutator, _, _, _ := newMutationFixture(t, &existing)
	settlement := mutator.settlement.(*finalSettlementFixture)
	allow, err := tobari.NewPolicyMemoryReviewedDecision(
		firstCandidate.ID, []tobari.PolicyCandidateAuthority{firstCandidate}, nil, tobari.PolicyMemoryAllow,
		firstCandidate.Effect.RuleBody(firstCandidate.ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	deny, err := tobari.NewPolicyMemoryReviewedDecision(
		secondCandidate.ID, []tobari.PolicyCandidateAuthority{secondCandidate}, nil, tobari.PolicyMemoryDeny,
		secondCandidate.Effect.RuleBody(secondCandidate.ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	set, err := tobari.NewPolicyMemoryReviewedDecisionSet(existing, []tobari.PolicyMemoryReviewedDecision{allow, deny})
	if err != nil {
		t.Fatal(err)
	}
	publication, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set)
	if err != nil || publication.Validate() != nil || settlement.reviewedCalls != 1 ||
		len(publication.Changes) != 2 || len(publication.AppliedDecisions) != 2 ||
		publication.AllowCount != 1 || publication.DenyCount != 1 {
		t.Fatalf("publication=%#v calls=%d err=%v validate=%v", publication, settlement.reviewedCalls, err, publication.Validate())
	}
	current, present, err := mutator.store.ReadComplete(context.Background())
	if err != nil || !present || len(current.PendingCandidates) != 0 || len(current.Contexts[0].PolicyMemory.Rules) != 1 || len(current.Contexts[1].PolicyMemory.Rules) != 1 {
		t.Fatalf("current=%#v present=%t err=%v", current, present, err)
	}
}

func TestApplyReviewedFixedTargetDoesNotReplayAnOlderDifferentSet(t *testing.T) {
	existing := storeCollectionFixture(t)
	_, mutator, _, _, _ := newMutationFixture(t, &existing)
	settlement := mutator.settlement.(*finalSettlementFixture)
	setA := reviewedSetForCandidates(t, existing, existing.PendingCandidates[0])
	if _, err := mutator.ApplyReviewedPolicyMemory(context.Background(), setA); err != nil {
		t.Fatal(err)
	}
	withB, candidateB := addPendingCandidate(t, mutator, "/candidate-b")
	setB := reviewedSetForCandidates(t, withB, candidateB)
	resultB, err := mutator.ApplyReviewedPolicyMemory(context.Background(), setB)
	if err != nil || resultB.Validate() != nil || settlement.reviewedCalls != 2 ||
		len(resultB.AppliedDecisions) != 1 || resultB.AppliedDecisions[0].ReviewItemID != candidateB.ID ||
		resultB.DecisionSet.Digest != setB.Digest || resultB.DecisionSet.Digest == setA.Digest {
		t.Fatalf("second result=%#v calls=%d err=%v validate=%v", resultB, settlement.reviewedCalls, err, resultB.Validate())
	}
	calls := settlement.reviewedCalls
	replayedB, err := mutator.ApplyReviewedPolicyMemory(context.Background(), setB)
	if err != nil || replayedB.AppliedDecisions[0].ReviewItemID != candidateB.ID || settlement.reviewedCalls != calls {
		t.Fatalf("same-set replay=%#v calls=%d err=%v", replayedB, settlement.reviewedCalls, err)
	}
}

func TestApplyReviewedRejectsConfirmedCollectionDriftBeforeDecisionOrEffect(t *testing.T) {
	existing := storeCollectionFixture(t)
	_, mutator, _, _, _ := newMutationFixture(t, &existing)
	settlement := mutator.settlement.(*finalSettlementFixture)
	set := reviewedSetForCandidates(t, existing, existing.PendingCandidates[0])
	body := existing.Templates[0].Current.Body.Clone()
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "concurrent-template", body); err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set); !errors.Is(err, tobari.ErrPolicyReviewChanged) {
		t.Fatalf("collection drift err=%v", err)
	}
	if settlement.reviewedCalls != 0 {
		t.Fatalf("collection drift reached settlement: calls=%d", settlement.reviewedCalls)
	}
	if _, active, err := mutator.readEffectDecision(); err != nil || active {
		t.Fatalf("collection drift left decision: active=%t err=%v", active, err)
	}
}

func TestApplyReviewedResumesOneGlobalDecisionAcrossPublicationBoundaries(t *testing.T) {
	tests := []struct {
		name             string
		installFailure   func(*Mutator) func()
		wantSettleCalls  int
		wantConfirmCalls int
	}{
		{
			name: "after settlement before terminal evidence",
			installFailure: func(mutator *Mutator) func() {
				realRename := mutator.rename
				decisionRenames := 0
				mutator.rename = func(source, target string) error {
					if source == mutator.effectDecisionTempPath() && target == mutator.effectDecisionPath() {
						decisionRenames++
						if decisionRenames == 2 {
							return fmt.Errorf("injected result-evidence publication interruption")
						}
					}
					return realRename(source, target)
				}
				return func() { mutator.rename = realRename }
			},
			wantSettleCalls: 2,
		},
		{
			name: "after terminal evidence before envelope",
			installFailure: func(mutator *Mutator) func() {
				realRename := mutator.rename
				stage := mutator.store.root + ".wp11-mutation-stage"
				authority := filepath.Join(mutator.store.root, authorityFileName)
				mutator.rename = func(source, target string) error {
					if source == stage && target == authority {
						return fmt.Errorf("injected envelope publication interruption")
					}
					return realRename(source, target)
				}
				return func() { mutator.rename = realRename }
			},
			wantSettleCalls: 2,
		},
		{
			name: "after envelope before terminal transition",
			installFailure: func(mutator *Mutator) func() {
				realRename := mutator.rename
				mutator.rename = func(source, target string) error {
					if source == mutator.effectDecisionPath() && target == mutator.effectDecisionDonePath() {
						return fmt.Errorf("injected terminal transition interruption")
					}
					return realRename(source, target)
				}
				return func() { mutator.rename = realRename }
			},
			wantSettleCalls: 1, wantConfirmCalls: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			existing := storeCollectionFixture(t)
			_, mutator, _, _, _ := newMutationFixture(t, &existing)
			settlement := mutator.settlement.(*finalSettlementFixture)
			set := reviewedSetForCandidates(t, existing, existing.PendingCandidates[0])
			restore := test.installFailure(mutator)
			if _, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set); err == nil {
				t.Fatal("interrupted reviewed application was reported complete")
			}
			if _, active, err := mutator.readEffectDecision(); err != nil || !active {
				t.Fatalf("active decision=%t err=%v", active, err)
			}
			body := existing.Templates[0].Current.Body.Clone()
			if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "blocked-during-reviewed", body); err == nil {
				t.Fatal("different mutation crossed active reviewed decision")
			}
			restore()
			publication, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set)
			if err != nil || publication.Validate() != nil || publication.DecisionSet.Digest != set.Digest ||
				settlement.reviewedCalls != test.wantSettleCalls || settlement.reviewedConfirms != test.wantConfirmCalls {
				t.Fatalf("publication=%#v calls=%d confirms=%d err=%v validate=%v", publication, settlement.reviewedCalls, settlement.reviewedConfirms, err, publication.Validate())
			}
		})
	}
}

func TestApplyReviewedReplaysAfterTerminalRenameResultUncertainty(t *testing.T) {
	existing := storeCollectionFixture(t)
	_, mutator, _, _, _ := newMutationFixture(t, &existing)
	settlement := mutator.settlement.(*finalSettlementFixture)
	set := reviewedSetForCandidates(t, existing, existing.PendingCandidates[0])
	realRename := mutator.rename
	mutator.rename = func(source, target string) error {
		if source == mutator.effectDecisionPath() && target == mutator.effectDecisionDonePath() {
			if err := realRename(source, target); err != nil {
				return err
			}
			return fmt.Errorf("injected post-terminal-rename uncertainty")
		}
		return realRename(source, target)
	}
	if _, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set); err == nil {
		t.Fatal("post-terminal-rename uncertainty was reported complete")
	}
	if _, active, err := mutator.readEffectDecision(); err != nil || active {
		t.Fatalf("active decision=%t err=%v", active, err)
	}
	mutator.rename = realRename
	calls := settlement.reviewedCalls
	publication, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set)
	if err != nil || publication.Validate() != nil || settlement.reviewedCalls != calls || settlement.reviewedConfirms != 1 || !publication.Changed {
		t.Fatalf("publication=%#v calls=%d confirms=%d err=%v", publication, settlement.reviewedCalls, settlement.reviewedConfirms, err)
	}
}

func TestConfirmedExternalEffectCompletesEnvelopeAfterCancellation(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, deletion, _ := newMutationFixture(t, &existing)
	ctx, cancel := context.WithCancel(context.Background())
	deletion.onRetire = cancel
	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	result, err := mutator.DeleteWorkspaceByReference(ctx, workspaceRef, false)
	if err != nil || !result.Deleted {
		t.Fatalf("confirmed cancellation result=%#v err=%v", result, err)
	}
	current, _, err := store.ReadComplete(context.Background())
	if err != nil || len(current.Workspaces) != 0 {
		t.Fatalf("confirmed cancellation envelope=%#v err=%v", current, err)
	}
}

func TestConfirmedPolicyEffectCompletesEnvelopeAfterCancellation(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	settlement := mutator.settlement.(*finalSettlementFixture)
	ctx, cancel := context.WithCancel(context.Background())
	settlement.onSettle = cancel
	publication, err := mutator.AllowPolicyCandidateByReference(ctx, existing.PendingCandidates[0].ID)
	if err != nil || !publication.Memory.Changed {
		t.Fatalf("confirmed policy cancellation publication=%#v err=%v", publication, err)
	}
	current, _, err := store.ReadComplete(context.Background())
	if err != nil || len(current.PendingCandidates) != 0 || current.Generation != existing.Generation+1 {
		t.Fatalf("confirmed policy cancellation envelope=%#v err=%v", current, err)
	}
}
