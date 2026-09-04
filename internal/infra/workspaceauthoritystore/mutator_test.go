package workspaceauthoritystore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
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

func (l *mutationLifecycle) WithLifecycleObservation(ctx context.Context, action func(context.Context) error) error {
	return l.WithLifecycleLock(ctx, action)
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
	running map[tobari.WorkspaceID]bool
}

func runningWorkspaceFixture(collection tobari.WorkspaceAuthorityCollection) map[tobari.WorkspaceID]bool {
	result := make(map[tobari.WorkspaceID]bool, len(collection.Workspaces))
	for _, workspace := range collection.Workspaces {
		result[workspace.ID] = true
	}
	return result
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

func (r *templateRuntimeRevisionFixture) ResolveRetainedWorkspaceTemplateRuntimeBinding(_ context.Context, binding tobari.RuntimeBinding) (tobari.RuntimeBinding, error) {
	if binding.Validate() != nil || binding.RuntimeID != tobari.StandardRuntimeID {
		return tobari.RuntimeBinding{}, tobari.ErrRuntimeRevisionNotFound
	}
	return binding, nil
}

func (r *templateRuntimeRevisionFixture) ObserveStatusWorkspace(_ context.Context, snapshot tobari.ContextAuthoritySnapshot) (tobari.StatusWorkspaceObservation, error) {
	state := tobari.StatusWorkspaceRuntimeRunning
	if r.running != nil && (snapshot.Workspace == nil || !r.running[snapshot.Workspace.ID]) {
		state = tobari.StatusWorkspaceRuntimeAbsent
	}
	return tobari.StatusWorkspaceObservation{State: state}, nil
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
	live []tobari.PolicyCandidateAuthority,
	_, _ string,
) (tobari.PolicyMemoryReviewedSettlementReceipt, error) {
	if s.err != nil {
		return tobari.PolicyMemoryReviewedSettlementReceipt{}, s.err
	}
	if err := tobari.ValidatePolicyMemoryReviewedTransitionWithLiveSources(previous, next, set, live); err != nil {
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

func (s *finalSettlementFixture) SettleFinalWorkspaceRetirement(ctx context.Context, previous, next tobari.WorkspaceAuthorityCollection, workspace tobari.WorkspaceBinding, operation, decisionRef string) error {
	return s.SettleFinalAuthority(ctx, previous, next, workspace.ContextID, operation, decisionRef)
}

func (s *finalSettlementFixture) ConfirmFinalWorkspaceRetirementSettled(ctx context.Context, next tobari.WorkspaceAuthorityCollection, workspace tobari.WorkspaceBinding) error {
	return s.ConfirmFinalAuthoritySettled(ctx, next, workspace.ContextID)
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

func TestContextApplyRechecksSourceAtFinalPublicationFence(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	id := tobari.ContextID("01912345-6789-7abc-8def-0123456789f1")
	source := tobari.ContextSource{SchemaVersion: tobari.ContextSourceSchemaVersion, ContextID: id, TemplateID: existing.Templates[0].ID}
	firstFingerprint := strings.Repeat("a", 64)
	plan, err := tobari.NewContextActivationPlan(existing, source, firstFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	loads := 0
	_, changed, err := mutator.ApplyContextSourceByPlan(context.Background(), plan.PlanRef, func(context.Context) (tobari.ContextSource, string, error) {
		loads++
		if loads == 1 {
			return source, firstFingerprint, nil
		}
		return source, strings.Repeat("b", 64), nil
	})
	if !errors.Is(err, tobari.ErrResourceSourceChanged) || changed || loads != 2 {
		t.Fatalf("concurrent Context edit err=%v changed=%t loads=%d", err, changed, loads)
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || after.Revision != existing.Revision || len(after.Contexts) != len(existing.Contexts) {
		t.Fatalf("Context edit crossed publication fence: after=%+v present=%t err=%v", after, present, err)
	}
}

func TestMutatorPersistsCurrentContextSelectionWithoutWorkspaceMutation(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, lifecycle, _, _ := newMutationFixture(t, &existing)
	contextID := existing.Contexts[0].Context.ID
	contextRef, err := tobari.ContextRef(contextID)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := mutator.SetCurrentContextByReference(context.Background(), contextRef)
	if err != nil || !selected.Selected || selected.ContextID != contextID {
		t.Fatalf("selected=%+v err=%v", selected, err)
	}
	current, err := store.ReadCurrentContextAuthority(context.Background())
	if err != nil || current.Context.ID != contextID {
		t.Fatalf("current=%+v err=%v", current.Context, err)
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || after.CurrentContextID == nil || *after.CurrentContextID != contextID || !reflect.DeepEqual(after.Workspaces, existing.Workspaces) {
		t.Fatalf("after current=%+v workspaces=%+v err=%v", after.CurrentContextID, after.Workspaces, err)
	}
	if lifecycle.attempts.Load() != 1 {
		t.Fatalf("lifecycle attempts=%d", lifecycle.attempts.Load())
	}
	// The Workspace remains the deletion blocker; current selection alone is
	// cleared atomically when an otherwise-empty Context is deleted.
	if _, err := mutator.DeleteContextByReference(context.Background(), contextRef); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("current Context deletion err=%v", err)
	}
}

func TestFirstContextActivationBecomesCurrentWithoutCreatingWorkspace(t *testing.T) {
	base := storeCollectionFixture(t)
	templateOnly, _, err := tobari.PublishWorkspaceAuthorityCollection(
		base.Templates, []tobari.WorkspaceAuthorityContextRecord{}, []tobari.WorkspaceBinding{},
		[]tobari.PolicyCandidateAuthority{}, base.DefaultTemplateID, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	store, mutator, _, _, _ := newMutationFixture(t, &templateOnly)
	id := tobari.ContextID("01912345-6789-7abc-8def-0123456789f2")
	source := tobari.ContextSource{SchemaVersion: tobari.ContextSourceSchemaVersion, ContextID: id, TemplateID: templateOnly.Templates[0].ID}
	fingerprint := strings.Repeat("c", 64)
	plan, err := tobari.NewContextActivationPlan(templateOnly, source, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	_, changed, err := mutator.ApplyContextSourceByPlan(context.Background(), plan.PlanRef, func(context.Context) (tobari.ContextSource, string, error) {
		return source, fingerprint, nil
	})
	if err != nil || !changed {
		t.Fatalf("apply first Context: changed=%t err=%v", changed, err)
	}
	current, err := store.ReadCurrentContextAuthority(context.Background())
	if err != nil || current.Context.ID != id || current.Workspace != nil {
		t.Fatalf("current=%+v err=%v", current, err)
	}
	ref, err := tobari.ContextRef(id)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := mutator.DeleteContextByReference(context.Background(), ref)
	if err != nil || !deleted.Deleted || deleted.ContextID != id {
		t.Fatalf("delete current=%+v err=%v", deleted, err)
	}
	if _, err := store.ReadCurrentContextAuthority(context.Background()); !errors.Is(err, tobari.ErrCurrentContextRequired) {
		t.Fatalf("deleted current selector err=%v", err)
	}
}

func TestTemplatePolicyMigrationFenceSerializesDeleteAndRejectsMissingActiveRevision(t *testing.T) {
	existing := storeCollectionFixture(t)
	secondID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789e8")
	revision, err := tobari.NewWorkspaceTemplateRevision(secondID, 1, existing.Templates[0].Current.Body.Clone())
	if err != nil {
		t.Fatal(err)
	}
	second := tobari.WorkspaceTemplate{
		SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: secondID, Name: "migration-target",
		Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()},
	}
	existing, _, err = tobari.PublishWorkspaceAuthorityCollection(
		append(existing.Templates, second), existing.Contexts, existing.Workspaces,
		existing.PendingCandidates, existing.DefaultTemplateID, &existing,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, mutator, _, _, _ := newMutationFixture(t, &existing)
	wrongCalled := false
	wrongRevision := tobari.SemanticDigest("sha256:" + strings.Repeat("d", 64))
	if err := mutator.WithWorkspaceTemplatePolicyMigrationFence(context.Background(), secondID, wrongRevision, func(context.Context, tobari.WorkspaceTemplate) error {
		wrongCalled = true
		return nil
	}); !errors.Is(err, tobari.ErrWorkspaceTemplatePolicyMigrationStale) || wrongCalled {
		t.Fatalf("revision-drift fence err=%v called=%t", err, wrongCalled)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	fenceDone := make(chan error, 1)
	go func() {
		fenceDone <- mutator.WithWorkspaceTemplatePolicyMigrationFence(context.Background(), secondID, revision.Revision, func(context.Context, tobari.WorkspaceTemplate) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	deleteDone := make(chan error, 1)
	ref, _ := tobari.WorkspaceTemplateRef(secondID)
	go func() {
		_, deleteErr := mutator.DeleteWorkspaceTemplateByReference(context.Background(), ref)
		deleteDone <- deleteErr
	}()
	select {
	case err := <-deleteDone:
		t.Fatalf("Template delete crossed the migration lifecycle fence: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-fenceDone; err != nil {
		t.Fatalf("migration fence: %v", err)
	}
	if err := <-deleteDone; err != nil {
		t.Fatalf("serialized Template delete: %v", err)
	}
	called := false
	err = mutator.WithWorkspaceTemplatePolicyMigrationFence(context.Background(), secondID, revision.Revision, func(context.Context, tobari.WorkspaceTemplate) error {
		called = true
		return nil
	})
	if !errors.Is(err, tobari.ErrWorkspaceTemplatePolicyMigrationStale) || called {
		t.Fatalf("missing active Template fence err=%v called=%t", err, called)
	}
}

func TestDeletedStableIDsCannotReenterTemplateOrContextApply(t *testing.T) {
	existing := storeCollectionFixture(t)
	_, mutator, _, _, _ := newMutationFixture(t, &existing)
	template := existing.Templates[0]
	if err := mutator.purgeDeletedAuthority("templates", string(template.ID), tobari.WorkspaceTemplateDeleteResult{TemplateID: template.ID, Deleted: true}); err != nil {
		t.Fatal(err)
	}
	templateSource, err := tobari.NewWorkspaceTemplateSource(template)
	if err != nil {
		t.Fatal(err)
	}
	templateRef, _ := tobari.WorkspaceTemplateRef(template.ID)
	if _, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return templateSource, strings.Repeat("a", 64), nil
	}); !errors.Is(err, tobari.ErrResourceIdentityDeleted) {
		t.Fatalf("tombstoned Template plan = %v", err)
	}

	contextID := existing.Contexts[0].Context.ID
	if err := mutator.purgeDeletedAuthority("contexts", string(contextID), tobari.ContextDeleteResult{ContextID: contextID, Deleted: true}); err != nil {
		t.Fatal(err)
	}
	contextSource, _ := tobari.NewContextSource(existing.Contexts[0].Context)
	contextRef, _ := tobari.ContextRef(contextID)
	if _, err := mutator.PlanContextSourceByReference(context.Background(), contextRef, func(context.Context) (tobari.ContextSource, string, error) {
		return contextSource, strings.Repeat("b", 64), nil
	}); !errors.Is(err, tobari.ErrResourceIdentityDeleted) {
		t.Fatalf("tombstoned Context plan = %v", err)
	}
}

func TestTemplatePlanClassifiesAbsentAuthorityAndLoadsLaterDraftSource(t *testing.T) {
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789a1")
	templateRef, err := tobari.WorkspaceTemplateRef(templateID)
	if err != nil {
		t.Fatal(err)
	}
	_, mutator, _, _, _ := newMutationFixture(t, nil)
	_, err = mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return tobari.WorkspaceTemplateSource{}, "", tobari.ErrResourceSourceMissing
	})
	if !errors.Is(err, tobari.ErrWorkspaceTemplateNotFound) {
		t.Fatalf("absent-authority Template plan error=%v", err)
	}

	existing := storeCollectionFixture(t)
	_, mutator, _, _, _ = newMutationFixture(t, &existing)
	unknownID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789af")
	unknownRef, err := tobari.WorkspaceTemplateRef(unknownID)
	if err != nil {
		t.Fatal(err)
	}
	loaderCalled := false
	_, err = mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), unknownRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		loaderCalled = true
		return tobari.WorkspaceTemplateSource{}, "", tobari.ErrResourceSourceMissing
	})
	if !errors.Is(err, tobari.ErrResourceSourceMissing) || !loaderCalled {
		t.Fatalf("missing later draft source error=%v loader_called=%t", err, loaderCalled)
	}
}

func TestTemplateApplySamePlanSettlesPublishedAuthorityBeforeBaseBookkeeping(t *testing.T) {
	existing := storeCollectionFixture(t)
	_, mutator, _, _, _ := newMutationFixture(t, &existing)
	template := existing.Templates[0]
	mutator.runtimeRevision.(*templateRuntimeRevisionFixture).binding = template.Current.Body.EntryDefaults.Runtime
	source, err := tobari.NewWorkspaceTemplateSource(template)
	if err != nil {
		t.Fatal(err)
	}
	source.Template.Name = "renamed-by-source"
	fingerprint := strings.Repeat("c", 64)
	templateRef, _ := tobari.WorkspaceTemplateRef(template.ID)
	plan, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source, fingerprint, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), plan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source, fingerprint, nil
	})
	if err != nil || !first.Changed || first.Template.Name != source.Template.Name {
		t.Fatalf("first Apply=%+v err=%v", first, err)
	}
	second, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), plan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source, fingerprint, nil
	})
	if err != nil || !reflect.DeepEqual(first.Template, second.Template) || first.Current.Revision != second.Current.Revision || first.Changed != second.Changed {
		t.Fatalf("same-plan settlement=%+v err=%v", second, err)
	}
	if err := mutator.CompleteWorkspaceTemplateApplySettlement(plan.PlanRef); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateApplyStaleWhenRunningWorkspaceSetChangesAtEqualCount(t *testing.T) {
	existing := storeCollectionFixture(t)
	secondContextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789b2")
	secondWorkspaceID := tobari.WorkspaceID("01912345-6789-7abc-8def-0123456789b3")
	secondMemory, _, err := tobari.PublishPolicyMemory(secondContextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	secondContext := tobari.WorkspaceAuthorityContextRecord{
		Context:      tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: secondContextID, TemplateID: storeTemplateID},
		PolicyMemory: secondMemory,
	}
	secondWorkspace := existing.Workspaces[0]
	if secondWorkspace.LastSuccessfulEntry != nil {
		entry := *secondWorkspace.LastSuccessfulEntry
		secondWorkspace.LastSuccessfulEntry = &entry
	}
	secondWorkspace.ID = secondWorkspaceID
	secondWorkspace.ContextID = secondContextID
	secondWorkspace.ProjectRoot = "/workspace/second"
	secondWorkspace.Home = "/workspace/second-home"
	if secondWorkspace.LastSuccessfulEntry != nil {
		secondWorkspace.LastSuccessfulEntry.ContextID = secondContextID
	}
	existing, _, err = tobari.PublishWorkspaceAuthorityCollection(
		existing.Templates,
		append(existing.Contexts, secondContext),
		append(existing.Workspaces, secondWorkspace),
		existing.PendingCandidates,
		existing.DefaultTemplateID,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	runtime := mutator.runtimeRevision.(*templateRuntimeRevisionFixture)
	runtime.binding = existing.Templates[0].Current.Body.EntryDefaults.Runtime
	runtime.running = map[tobari.WorkspaceID]bool{storeWorkspaceID: true, secondWorkspaceID: false}
	source, err := tobari.NewWorkspaceTemplateSource(existing.Templates[0])
	if err != nil {
		t.Fatal(err)
	}
	source.Template.Name = "running-set-reviewed"
	fingerprint := strings.Repeat("d", 64)
	templateRef, _ := tobari.WorkspaceTemplateRef(storeTemplateID)
	plan, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source, fingerprint, nil
	})
	if err != nil || plan.RunningWorkspaceCount != 1 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	runtime.running = map[tobari.WorkspaceID]bool{storeWorkspaceID: false, secondWorkspaceID: true}
	if _, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), plan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source, fingerprint, nil
	}); !errors.Is(err, tobari.ErrWorkspaceTemplateChangePlanStale) {
		t.Fatalf("equal-count running-set drift Apply = %v", err)
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || current.Templates[0].Name != existing.Templates[0].Name {
		t.Fatalf("stale running-set Apply published authority: present=%t name=%q err=%v", present, current.Templates[0].Name, err)
	}
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

func testSemanticDigest(t *testing.T, value any) tobari.SemanticDigest {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	return tobari.SemanticDigest("sha256:" + hex.EncodeToString(digest[:]))
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
		TemplateID: secondTemplateID,
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
		ProjectRoot: first.Workspaces[0].ProjectRoot, Home: "/workspace/home-" + string(secondWorkspaceID),
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

	created, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "restricted", body)
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
	copyPublication, err := mutator.seedWorkspaceTemplateCopyForLegacyMigration(context.Background(), revisionRef, "copied")
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

func TestApplyWorkspaceTemplateSourcePublishesOneMovingHeadRevision(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	active := existing.Templates[0]
	mutator.runtimeRevision = &templateRuntimeRevisionFixture{binding: active.Current.Body.EntryDefaults.Runtime}
	source, err := tobari.NewWorkspaceTemplateSource(active)
	if err != nil {
		t.Fatal(err)
	}
	source.Policy.Semantic.Protocols.HTTP.Generic.Allow.Rules[0].Path = "/edited"
	fingerprint := strings.Repeat("a", 64)
	plan, err := tobari.NewWorkspaceTemplateChangePlan(existing, active.ID, source, active.Current.Body.EntryDefaults.Runtime, runningWorkspaceFixture(existing), fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	loads := 0
	publication, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), plan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		loads++
		return source.Clone(), fingerprint, nil
	})
	if err != nil || !publication.Changed || publication.Current.Generation != active.Current.Generation+1 || loads != 2 {
		t.Fatalf("Apply = %+v, loads=%d, err=%v", publication, loads, err)
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || current.Templates[0].Current.Body.Policy.SemanticModules.Protocols.HTTP.Generic.Allow.Rules[0].Path != "/edited" || current.Contexts[0].Context.TemplateID != active.ID {
		t.Fatalf("active moving head = %+v, present=%t, err=%v", current, present, err)
	}
}

func TestTemplateApplyPreservesLivePolicyAxesForNonPolicyEntryChange(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	active := existing.Templates[0]
	mutator.runtimeRevision = &templateRuntimeRevisionFixture{binding: active.Current.Body.EntryDefaults.Runtime}
	source, err := tobari.NewWorkspaceTemplateSource(active)
	if err != nil {
		t.Fatal(err)
	}
	if source.Template.SourceAccess == tobari.ManifestSourceAccessReadOnly {
		source.Template.SourceAccess = tobari.ManifestSourceAccessReadWrite
	} else {
		source.Template.SourceAccess = tobari.ManifestSourceAccessReadOnly
	}
	fingerprint := strings.Repeat("6", 64)
	templateRef, _ := tobari.WorkspaceTemplateRef(active.ID)
	plan, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source.Clone(), fingerprint, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	publication, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), plan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source.Clone(), fingerprint, nil
	})
	if err != nil || !publication.Changed {
		t.Fatalf("non-policy Template Apply=%+v err=%v", publication, err)
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("read non-policy Template Apply: present=%t err=%v", present, err)
	}
	before, after := existing.Contexts[0], current.Contexts[0]
	if !reflect.DeepEqual(after.ActiveTemplatePolicy, before.ActiveTemplatePolicy) ||
		!reflect.DeepEqual(after.ActivePolicyMemory, before.ActivePolicyMemory) ||
		!reflect.DeepEqual(after.ActivePolicyMemoryRef, before.ActivePolicyMemoryRef) {
		t.Fatalf("non-policy Template Apply invalidated live policy axes: before=%+v after=%+v", before, after)
	}
	if current.Workspaces[0].LastSuccessfulEntry == nil || current.Workspaces[0].LastSuccessfulEntry.TemplateRevision == publication.Current.Revision {
		t.Fatalf("non-policy Template Apply invented a current Workspace entry: %+v", current.Workspaces[0])
	}
}

func TestTemplateApplyPreservesLiveTemplatePolicyUntilReconciliation(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	active := existing.Templates[0]
	mutator.runtimeRevision = &templateRuntimeRevisionFixture{binding: active.Current.Body.EntryDefaults.Runtime}
	source, err := tobari.NewWorkspaceTemplateSource(active)
	if err != nil {
		t.Fatal(err)
	}
	source.Policy.Semantic.Protocols.HTTP.Generic.Allow.Rules[0].Path = "/policy-changed"
	fingerprint := strings.Repeat("7", 64)
	templateRef, _ := tobari.WorkspaceTemplateRef(active.ID)
	plan, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source.Clone(), fingerprint, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), plan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source.Clone(), fingerprint, nil
	}); err != nil {
		t.Fatal(err)
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("read policy Template Apply: present=%t err=%v", present, err)
	}
	if !reflect.DeepEqual(current.Contexts[0].ActiveTemplatePolicy, existing.Contexts[0].ActiveTemplatePolicy) {
		t.Fatalf("policy-changing Template Apply replaced independently active Template receipt: before=%+v after=%+v", existing.Contexts[0].ActiveTemplatePolicy, current.Contexts[0].ActiveTemplatePolicy)
	}
	if current.Contexts[0].ActiveTemplatePolicy == nil || current.Contexts[0].ActiveTemplatePolicy.PolicySliceDigest == current.Templates[0].Current.Slices.PolicySliceDigest {
		t.Fatalf("policy-changing Template Apply inferred desired policy as active: context=%+v template=%+v", current.Contexts[0], current.Templates[0])
	}
}

func TestHistoricalStandardRuntimeBindingSurvivesTemplatePlanAndApply(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	runtime := mutator.runtimeRevision.(*templateRuntimeRevisionFixture)
	runtime.err = tobari.ErrRuntimeRevisionNotFound
	active := existing.Templates[0]
	source, err := tobari.NewWorkspaceTemplateSource(active)
	if err != nil {
		t.Fatal(err)
	}
	source.Policy.Semantic.Protocols.HTTP.Generic.Allow.Rules[0].Path = "/historical-edited"
	fingerprint := strings.Repeat("8", 64)
	templateRef, _ := tobari.WorkspaceTemplateRef(active.ID)
	plan, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source.Clone(), fingerprint, nil
	})
	if err != nil {
		t.Fatalf("plan retained standard binding: %v", err)
	}
	publication, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), plan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source.Clone(), fingerprint, nil
	})
	if err != nil || !publication.Changed || publication.ResolvedRuntime == nil || *publication.ResolvedRuntime != active.Current.Body.EntryDefaults.Runtime {
		t.Fatalf("changed retained publication=%+v err=%v", publication, err)
	}
	if err := mutator.CompleteWorkspaceTemplateApplySettlement(plan.PlanRef); err != nil {
		t.Fatal(err)
	}

	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("read changed authority: present=%t err=%v", present, err)
	}
	noOpSource, err := tobari.NewWorkspaceTemplateSource(current.Templates[0])
	if err != nil {
		t.Fatal(err)
	}
	noOpFingerprint := strings.Repeat("9", 64)
	noOpPlan, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return noOpSource.Clone(), noOpFingerprint, nil
	})
	if err != nil {
		t.Fatalf("plan retained no-op: %v", err)
	}
	noOp, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), noOpPlan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return noOpSource.Clone(), noOpFingerprint, nil
	})
	if err != nil || noOp.Changed || noOp.ResolvedRuntime == nil || *noOp.ResolvedRuntime != active.Current.Body.EntryDefaults.Runtime {
		t.Fatalf("no-op retained publication=%+v err=%v", noOp, err)
	}
}

func TestTemplatePlanCanRevertFromManagedRuntimeToRetainedHistoricalStandard(t *testing.T) {
	existing := storeCollectionFixture(t)
	original := existing.Templates[0].Current.Body.EntryDefaults.Runtime
	template := existing.Templates[0].Clone()
	body := template.Current.Body.Clone()
	body.EntryDefaults.Runtime = tobari.RuntimeBinding{
		RuntimeID: "01912345-6789-7abc-8def-0123456789f1",
		Name:      "managed-current",
		Revision:  "sha256:" + strings.Repeat("e", 64),
		Ordinal:   1,
		Image:     "tobari-runtime-managed:eeeeeeeeeeee",
	}
	managed, err := tobari.NewWorkspaceTemplateRevision(template.ID, template.Current.Generation+1, body)
	if err != nil {
		t.Fatal(err)
	}
	template.Current = managed
	template.Retained = append(template.Retained, managed.Clone())
	existing, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{template}, existing.Contexts, existing.Workspaces,
		existing.PendingCandidates, existing.DefaultTemplateID, &existing,
	)
	if err != nil || !changed {
		t.Fatalf("publish managed desired Runtime: changed=%t err=%v", changed, err)
	}
	_, mutator, _, _, _ := newMutationFixture(t, &existing)
	runtime := mutator.runtimeRevision.(*templateRuntimeRevisionFixture)
	runtime.err = tobari.ErrRuntimeRevisionNotFound
	source, err := tobari.NewWorkspaceTemplateSource(template)
	if err != nil {
		t.Fatal(err)
	}
	source.Template.EntryDefaults.Runtime = tobari.RuntimeSourceRefFrom(original)
	fingerprint := strings.Repeat("5", 64)
	templateRef, _ := tobari.WorkspaceTemplateRef(template.ID)
	plan, err := mutator.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source.Clone(), fingerprint, nil
	})
	if err != nil {
		t.Fatalf("plan retained historical standard revert=%+v err=%v", plan, err)
	}
	publication, err := mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), plan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		return source.Clone(), fingerprint, nil
	})
	if err != nil || !publication.Changed || publication.Current.Body.EntryDefaults.Runtime != original {
		t.Fatalf("apply retained historical standard revert=%+v err=%v", publication, err)
	}
}

func TestApplyWorkspaceTemplateSourceFencesConcurrentBytesAndStaleBase(t *testing.T) {
	for _, test := range []struct {
		name  string
		stale bool
	}{
		{name: "source changed"},
		{name: "stale base", stale: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			existing := storeCollectionFixture(t)
			store, mutator, _, _, _ := newMutationFixture(t, &existing)
			active := existing.Templates[0]
			mutator.runtimeRevision = &templateRuntimeRevisionFixture{binding: active.Current.Body.EntryDefaults.Runtime}
			source, err := tobari.NewWorkspaceTemplateSource(active)
			if err != nil {
				t.Fatal(err)
			}
			source.Policy.Semantic.Protocols.HTTP.Generic.Allow.Rules[0].Path = "/edited"
			if test.stale {
				value := tobari.SemanticDigest("sha256:" + strings.Repeat("e", 64))
				source.Template.BaseRevision = &value
			}
			fingerprint := strings.Repeat("a", 64)
			plannedSource := source.Clone()
			if test.stale {
				plannedSource, err = tobari.NewWorkspaceTemplateSource(active)
				if err != nil {
					t.Fatal(err)
				}
				plannedSource.Policy.Semantic.Protocols.HTTP.Generic.Allow.Rules[0].Path = "/edited"
			}
			plan, planErr := tobari.NewWorkspaceTemplateChangePlan(existing, active.ID, plannedSource, active.Current.Body.EntryDefaults.Runtime, runningWorkspaceFixture(existing), fingerprint)
			if planErr != nil {
				t.Fatal(planErr)
			}
			loads := 0
			_, err = mutator.ApplyWorkspaceTemplateSourceByReference(context.Background(), plan.PlanRef, func(context.Context) (tobari.WorkspaceTemplateSource, string, error) {
				loads++
				observed := fingerprint
				if loads > 1 {
					observed = strings.Repeat("b", 64)
				}
				return source.Clone(), observed, nil
			})
			if test.stale && !errors.Is(err, tobari.ErrResourceSourceModified) {
				t.Fatalf("stale Apply error = %v", err)
			}
			if !test.stale && !errors.Is(err, tobari.ErrResourceSourceChanged) {
				t.Fatalf("concurrent Apply error = %v", err)
			}
			current, present, readErr := store.ReadComplete(context.Background())
			if readErr != nil || !present || current.Revision != existing.Revision {
				t.Fatalf("failed Apply changed active authority = %+v/%t/%v", current, present, readErr)
			}
		})
	}
}

func TestMutatorRejectsInvalidTemplateCreationBeforePublication(t *testing.T) {
	store, mutator, lifecycle, _, _ := newMutationFixture(t, nil)
	body := storeCollectionFixture(t).Templates[0].Current.Body.Clone()
	invalidBody := body.Clone()
	invalidBody.Boundary.SourceAccess = "snapshot"
	beforeAttempts := lifecycle.attempts.Load()
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "invalid-body", invalidBody); err == nil {
		t.Fatal("invalid Template body unexpectedly succeeded")
	}
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "bad/name", body); err == nil {
		t.Fatal("invalid Template name unexpectedly succeeded")
	}
	afterAttempts := lifecycle.attempts.Load()
	if afterAttempts != beforeAttempts {
		t.Fatalf("invalid creates entered mutation lifecycle: before=%d after=%d", beforeAttempts, afterAttempts)
	}
	collection, present, err := store.ReadComplete(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if present || len(collection.Templates) != 0 || collection.Generation != 0 {
		t.Fatalf("invalid creates changed authority: present=%t collection=%+v", present, collection)
	}
}

func TestMutatorContextCreateDeleteAndWorkspaceRetirementPreserveOwners(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, deletion, _ := newMutationFixture(t, &existing)
	templateRef, _ := tobari.WorkspaceTemplateRef(storeTemplateID)
	if _, err := mutator.seedContextForLegacyMigration(context.Background(), templateRef, "/workspace/example"); err != nil {
		t.Fatalf("location-free Context sharing a Template failed: %v", err)
	}

	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	workspaceResult, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, true)
	if err != nil || !workspaceResult.Deleted || len(deletion.retired) != 1 || deletion.retired[0] != storeWorkspaceID {
		t.Fatalf("workspace result=%#v retired=%#v err=%v", workspaceResult, deletion.retired, err)
	}
	afterWorkspace, _, err := store.ReadComplete(context.Background())
	if err != nil || len(afterWorkspace.Workspaces) != 0 || len(afterWorkspace.Contexts) != 2 || len(afterWorkspace.PendingCandidates) != 0 {
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
	if err != nil || len(afterContext.Contexts) != 1 || len(afterContext.Workspaces) != 0 || len(afterContext.Templates) != 1 {
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
	stage := mutationStagePath(store.root)
	authority := filepath.Join(store.root, authorityFileName)
	mutator.rename = func(source, target string) error {
		if source == stage && target == authority {
			return fmt.Errorf("injected death after Context aggregate settlement")
		}
		return realRename(source, target)
	}
	contextRef, _ := tobari.ContextRef(storeContextID)
	readinessCalls := 0
	readiness := func(context.Context) error {
		readinessCalls++
		return nil
	}
	if _, err := mutator.DeleteContextByReferenceWithReadiness(context.Background(), contextRef, readiness); err == nil {
		t.Fatal("interrupted Context deletion was reported as complete")
	}
	current, _, err := store.ReadComplete(context.Background())
	if err != nil || contextRecordIndex(current, storeContextID) < 0 || settlement.contextDeleteCalls != 1 {
		t.Fatalf("interrupted Context authority=%#v settlements=%d err=%v", current, settlement.contextDeleteCalls, err)
	}
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "blocked-by-context-delete", existing.Templates[0].Current.Body); err == nil {
		t.Fatal("different mutation crossed active Context deletion")
	}

	mutator.rename = realRename
	result, err := mutator.DeleteContextByReferenceWithReadiness(context.Background(), contextRef, readiness)
	if err != nil || !result.Deleted || result.ContextID != storeContextID || settlement.contextDeleteCalls != 2 {
		t.Fatalf("resumed Context delete=%#v settlements=%d err=%v", result, settlement.contextDeleteCalls, err)
	}
	calls := settlement.contextDeleteCalls
	replayed, err := mutator.DeleteContextByReferenceWithReadiness(context.Background(), contextRef, readiness)
	if err != nil || !replayed.Deleted || replayed.ContextID != storeContextID || settlement.contextDeleteCalls != calls || settlement.contextDeleteConfirms != 1 {
		t.Fatalf("terminal Context replay=%#v settlements=%d confirms=%d err=%v", replayed, settlement.contextDeleteCalls, settlement.contextDeleteConfirms, err)
	}
	if readinessCalls != 1 {
		t.Fatalf("Context delete readiness calls=%d, want one fresh-action check", readinessCalls)
	}
}

func TestContextDeleteReadinessFailurePrecedesDurableDecision(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	if _, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, true); err != nil {
		t.Fatal(err)
	}
	before, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("read before Context delete: present=%t err=%v", present, err)
	}
	readinessCalls := 0
	readinessErr := fault.WithClassification(
		fault.New(fault.KindUnavailable, "docker_context_unavailable", "synthetic selected Docker context is unavailable", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the selected Docker context."}),
		fault.PhasePrecondition,
		fault.ChangeNone,
	)
	contextRef, _ := tobari.ContextRef(storeContextID)
	_, err = mutator.DeleteContextByReferenceWithReadiness(context.Background(), contextRef, func(context.Context) error {
		readinessCalls++
		return readinessErr
	})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "docker_context_unavailable" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
		t.Fatalf("Context delete readiness fault=%#v ok=%t err=%v", public, ok, err)
	}
	if readinessCalls != 1 || mutator.settlement.(*finalSettlementFixture).contextDeleteCalls != 0 {
		t.Fatalf("readiness calls=%d settlement calls=%d", readinessCalls, mutator.settlement.(*finalSettlementFixture).contextDeleteCalls)
	}
	after, present, readErr := store.ReadComplete(context.Background())
	if readErr != nil || !present || after.Generation != before.Generation || after.Revision != before.Revision {
		t.Fatalf("readiness failure changed authority: before=%#v after=%#v present=%t err=%v", before, after, present, readErr)
	}
	if _, active, decisionErr := mutator.readEffectDecision(); decisionErr != nil || active {
		t.Fatalf("readiness failure retained durable decision: active=%t err=%v", active, decisionErr)
	}
	if _, stageErr := os.Lstat(mutationStagePath(store.root)); !errors.Is(stageErr, os.ErrNotExist) {
		t.Fatalf("readiness failure retained mutation stage: %v", stageErr)
	}
}

func TestContextDeleteClassifiesInterruptedExternalSettlement(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	if _, err := mutator.DeleteWorkspaceByReference(context.Background(), workspaceRef, true); err != nil {
		t.Fatal(err)
	}
	mutator.settlement.(*finalSettlementFixture).err = errors.New("private settlement detail")
	contextRef, _ := tobari.ContextRef(storeContextID)
	_, err := mutator.DeleteContextByReference(context.Background(), contextRef)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "final_authority_mutation_interrupted" || public.Kind != fault.KindUnavailable || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangePartial || strings.Contains(public.Message, "private settlement detail") {
		t.Fatalf("fault = %+v, %v", public, err)
	}
	current, _, readErr := store.ReadComplete(context.Background())
	if readErr != nil || contextRecordIndex(current, storeContextID) < 0 {
		t.Fatalf("interrupted Context delete published authority: %#v, %v", current, readErr)
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

func TestMutatorCandidateDecisionConsumesLegacyAndCurrentAliases(t *testing.T) {
	existing := storeCollectionFixture(t)
	current := existing.PendingCandidates[0].Clone()
	legacy := current.Clone()
	material, err := json.Marshal(struct {
		ContextID   tobari.ContextID
		WorkspaceID tobari.WorkspaceID
		Payload     tobari.SemanticDigest
	}{legacy.ContextID, legacy.ObservingWorkspaceID, legacy.PayloadDigest})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(material)
	legacy.ID = "pcy_" + hex.EncodeToString(digest[:16])
	existing, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		existing.Templates, existing.Contexts, existing.Workspaces,
		[]tobari.PolicyCandidateAuthority{legacy, current}, existing.DefaultTemplateID, &existing,
	)
	if err != nil || !changed {
		t.Fatalf("publish candidate aliases: changed=%t err=%v", changed, err)
	}
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	publication, err := mutator.AllowPolicyCandidateByReference(context.Background(), legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := publication.ValidateFor(legacy.ID, tobari.PolicyMemoryAllow); err != nil {
		t.Fatal(err)
	}
	currentCollection, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || len(currentCollection.PendingCandidates) != 0 {
		t.Fatalf("candidate alias survived direct apply: pending=%#v present=%t err=%v", currentCollection.PendingCandidates, present, err)
	}
}

func TestPolicyMutationPreflightReadFailureIsObservationFault(t *testing.T) {
	existing := storeCollectionFixture(t)
	store, mutator, lifecycle, _, _ := newMutationFixture(t, &existing)
	activePath := filepath.Join(store.root, activeFileName)
	if err := os.WriteFile(activePath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := mutator.AllowPolicyCandidateByReference(context.Background(), existing.PendingCandidates[0].ID)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "final_authority_read_failed" || public.Kind != fault.KindUnavailable || public.Phase != fault.PhaseObservation || public.ChangeState != fault.ChangeNotApplicable || len(public.NextActions) != 1 || public.NextActions[0].Command != "status" {
		t.Fatalf("preflight read fault=%#v ok=%t err=%v", public, ok, err)
	}
	if lifecycle.attempts.Load() != 0 {
		t.Fatalf("preflight read failure entered mutation lifecycle: attempts=%d", lifecycle.attempts.Load())
	}
}

func TestPolicyMutationPreflightCancellationIsRetryablePrecondition(t *testing.T) {
	existing := storeCollectionFixture(t)
	_, mutator, lifecycle, _, _ := newMutationFixture(t, &existing)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mutator.AllowPolicyCandidateByReference(ctx, existing.PendingCandidates[0].ID)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "operation_canceled" || public.Kind != fault.KindCanceled || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone || !public.Retryable {
		t.Fatalf("preflight cancellation fault=%#v ok=%t err=%v", public, ok, err)
	}
	if lifecycle.attempts.Load() != 0 {
		t.Fatalf("preflight cancellation entered mutation lifecycle: attempts=%d", lifecycle.attempts.Load())
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
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "post-rename", body); err != nil {
		t.Fatalf("exact read-back did not classify confirmed publication: %v", err)
	}
	confirmed, _, _ := store.ReadComplete(context.Background())
	if len(confirmed.Templates) != 2 {
		t.Fatalf("confirmed=%#v", confirmed)
	}

	mutator.rename = func(string, string) error { return fmt.Errorf("injected pre-rename failure") }
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "pre-rename", body); err == nil {
		t.Fatal("pre-rename failure was reported as success")
	}
	after, _, _ := store.ReadComplete(context.Background())
	if after.Generation != confirmed.Generation || after.Revision != confirmed.Revision {
		t.Fatalf("pre-rename failure changed authority: %#v", after)
	}
	stageInfo, err := os.Lstat(mutationStagePath(store.root))
	if err != nil || !stageInfo.Mode().IsRegular() {
		t.Fatalf("recoverable stage info=%#v err=%v", stageInfo, err)
	}
	mutator.rename = realRename
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "retry", body); err != nil {
		t.Fatalf("next lifecycle mutation did not reconcile stage: %v", err)
	}
}

func TestMutatorFirstPublicationStageIsRecoverableAndReadersSeeNoPartialRoot(t *testing.T) {
	store, mutator, _, _, _ := newMutationFixture(t, nil)
	stage := mutationStagePath(store.root)
	if err := os.MkdirAll(filepath.Dir(stage), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, authorityFileName), []byte(`{"partial":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, present, err := store.ReadComplete(context.Background()); err != nil || present {
		t.Fatalf("owned initial publication affected ordinary authority: present=%t err=%v", present, err)
	}
	body := storeCollectionFixture(t).Templates[0].Current.Body.Clone()
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "recovered", body); err != nil {
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
			_, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), name, body)
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
	if err := os.Symlink(foreign, mutationStagePath(store.root)); err != nil {
		t.Fatal(err)
	}
	body := existing.Templates[0].Current.Body.Clone()
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "blocked", body); err == nil {
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
	stage := mutationStagePath(store.root)
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
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "blocked-by-delete", body); err == nil {
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
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "after-delete", body); err != nil {
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
	stage := mutationStagePath(store.root)
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
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "blocked-by-policy", body); err == nil {
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
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "unrelated-after-policy", body); err != nil {
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
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "unrelated-after-reviewed", body); err != nil {
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
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "concurrent-template", body); err != nil {
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
				stage := mutationStagePath(mutator.store.root)
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
			if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "blocked-during-reviewed", body); err == nil {
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

func TestApplyReviewedResumesInterruptedLegacyPathTemplateDecision(t *testing.T) {
	existing := storeCollectionFixture(t)
	firstEffect := existing.PendingCandidates[0].Effect.Clone()
	firstEffect.Path = "/teams/first"
	firstEffect.Examples = []string{"/teams/first"}
	first, err := tobari.NewPolicyCandidateAuthority(storeContextID, storeWorkspaceID, firstEffect)
	if err != nil {
		t.Fatal(err)
	}
	secondEffect := existing.PendingCandidates[0].Effect.Clone()
	secondEffect.Path = "/teams/second"
	secondEffect.Examples = []string{"/teams/second"}
	second, err := tobari.NewPolicyCandidateAuthority(storeContextID, storeWorkspaceID, secondEffect)
	if err != nil {
		t.Fatal(err)
	}
	existing, _, err = tobari.PublishWorkspaceAuthorityCollection(existing.Templates, existing.Contexts, existing.Workspaces,
		[]tobari.PolicyCandidateAuthority{first, second}, existing.DefaultTemplateID, &existing)
	if err != nil {
		t.Fatal(err)
	}
	candidates := []tobari.PolicyCandidateAuthority{first, second}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	sources := []string{candidates[0].ID, candidates[1].ID}
	sort.Strings(sources)
	rule := tobari.PolicyMemoryRuleBody{
		PolicyProtocolIdentity: candidates[0].Effect.PolicyProtocolIdentity,
		Match:                  tobari.PolicyMatchPathTemplate, Host: candidates[0].Effect.Host, Port: candidates[0].Effect.Port, Method: candidates[0].Effect.Method,
		Path: "/teams/" + tobari.PolicyPathTemplatePlaceholder, Segments: []string{"teams", tobari.PolicyPathTemplatePlaceholder}, Examples: []string{"/teams/first", "/teams/second"}, SourceCandidates: sources,
	}
	itemID, err := tobari.PolicyMemoryReviewedPathTemplateItemID(storeContextID, rule)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := tobari.NewPolicyMemoryReviewedDecision(itemID, candidates, nil, tobari.PolicyMemoryAllow, rule)
	if err != nil {
		t.Fatal(err)
	}
	set, err := tobari.NewPolicyMemoryReviewedDecisionSet(existing, []tobari.PolicyMemoryReviewedDecision{decision})
	if err != nil {
		t.Fatal(err)
	}
	_, mutator, _, _, _ := newMutationFixture(t, &existing)
	settlement := mutator.settlement.(*finalSettlementFixture)
	settlement.err = errors.New("interrupt predecessor after durable decision")
	if _, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set); err == nil {
		t.Fatal("predecessor interruption was reported complete")
	}
	data, err := os.ReadFile(mutator.effectDecisionPath())
	if err != nil {
		t.Fatal(err)
	}
	var journal effectDecision
	if err := json.Unmarshal(data, &journal); err != nil {
		t.Fatal(err)
	}
	legacy := journal.ReviewedSet.Decisions[0]
	material := []string{"tobari-policy-path-template-v1", string(storeContextID), string(storeWorkspaceID), rule.Host, strconv.Itoa(rule.Port), rule.Method, rule.Path, rule.PolicyProtocolIdentity.Scheme}
	legacyIDSum := sha256.Sum256([]byte(strings.Join(material, "\x00")))
	legacy.ReviewItemID = "ptp_" + hex.EncodeToString(legacyIDSum[:16])
	legacy.Digest = testSemanticDigest(t, struct {
		ReviewItemID   string
		ProposalDigest tobari.SemanticDigest
		Decision       tobari.PolicyMemoryDecision
	}{legacy.ReviewItemID, legacy.ProposalDigest, legacy.Decision})
	journal.ReviewedSet.Decisions[0] = legacy
	journal.ReviewedSet.Digest = testSemanticDigest(t, struct {
		TargetID           string
		ObservedGeneration uint64
		ObservedRevision   tobari.SemanticDigest
		Decisions          []tobari.PolicyMemoryReviewedDecision
	}{journal.ReviewedSet.TargetID, journal.ReviewedSet.ObservedGeneration, journal.ReviewedSet.ObservedRevision, journal.ReviewedSet.Decisions})
	encoded, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mutator.effectDecisionPath(), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	settlement.err = nil
	publication, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set)
	if err != nil || publication.Validate() != nil || !reflect.DeepEqual(publication.DecisionSet, set) {
		t.Fatalf("candidate recovery publication=%#v err=%v validate=%v", publication, err, publication.Validate())
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
