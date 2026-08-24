package workspaceauthoritystore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
}

func (l *mutationLifecycle) WithLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	l.attempts.Add(1)
	l.lock.Lock()
	defer l.lock.Unlock()
	l.held.Store(true)
	defer l.held.Store(false)
	return action(ctx)
}

type deletionAuthorityFixture struct {
	retired              []tobari.WorkspaceID
	retirementRefs       []string
	credentialChecks     []tobari.ContextID
	workspaceErr         error
	confirmationErr      error
	contextCredentialErr error
	confirmations        int
	onRetire             func()
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
	err                   error
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

func (d *deletionAuthorityFixture) RetireWorkspace(_ context.Context, workspace tobari.WorkspaceBinding, _ bool, decisionRef string) error {
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
	lifecycle := &mutationLifecycle{}
	deletion := &deletionAuthorityFixture{}
	activation := &policyActivationFixture{}
	settlement := &finalSettlementFixture{activation: activation}
	mutator, err := NewMutator(store, lifecycle, deletion, activation, settlement)
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
	selected, err := mutator.SetDefaultWorkspaceTemplateByReference(context.Background(), templateRef)
	if err != nil || !selected.Selected || selected.TemplateID != created.ID {
		t.Fatalf("selected=%#v err=%v", selected, err)
	}
	selectedCollection, _, _ := store.ReadComplete(context.Background())
	if selectedCollection.Generation != first.Generation+1 {
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
	if lifecycle.attempts.Load() != 7 {
		t.Fatalf("lifecycle attempts=%d", lifecycle.attempts.Load())
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
