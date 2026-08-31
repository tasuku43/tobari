package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthorityresources"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritysource"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

type firstUseIntegrationLifecycle struct {
	parent string
	lock   sync.Mutex
}

func (l *firstUseIntegrationLifecycle) WithLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(l.parent, 0o700); err != nil {
		return err
	}
	l.lock.Lock()
	defer l.lock.Unlock()
	return action(ctx)
}

type firstUseIntegrationRuntime struct {
	revision string
	refs     []string
}

func (r *firstUseIntegrationRuntime) ResolveWorkspaceTemplateRuntimeRevision(_ context.Context, reference string) (tobari.RuntimeBinding, error) {
	r.refs = append(r.refs, reference)
	id, revision, err := tobari.ParseRuntimeRevisionRef(reference)
	if err != nil {
		return tobari.RuntimeBinding{}, err
	}
	if id != tobari.StandardRuntimeID || revision != r.revision {
		return tobari.RuntimeBinding{}, fmt.Errorf("unexpected first-use Runtime reference: %s", reference)
	}
	return tobari.RuntimeBinding{
		RuntimeID: tobari.StandardRuntimeID,
		Name:      tobari.StandardRuntimeName,
		Revision:  revision,
		Ordinal:   1,
		Image:     "tobari-runtime:test",
	}, nil
}

func (r *firstUseIntegrationRuntime) ResolveRetainedWorkspaceTemplateRuntimeBinding(_ context.Context, binding tobari.RuntimeBinding) (tobari.RuntimeBinding, error) {
	return binding, binding.Validate()
}

type firstUseIntegrationActivation struct {
	confirmCalls int
}

func (a *firstUseIntegrationActivation) ActivatePolicyMemory(_ context.Context, collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID) (tobari.PolicyMemoryActivationReceipt, error) {
	for _, record := range collection.Contexts {
		if record.Context.ID == contextID {
			return tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: record.PolicyMemory.Revision}, nil
		}
	}
	return tobari.PolicyMemoryActivationReceipt{}, fmt.Errorf("Policy Memory Context is unavailable")
}

func (a *firstUseIntegrationActivation) ConfirmPolicyMemoryActive(_ context.Context, collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID, receipt tobari.PolicyMemoryActivationReceipt) error {
	a.confirmCalls++
	for _, record := range collection.Contexts {
		if record.Context.ID != contextID {
			continue
		}
		if record.ActivePolicyMemory == nil || record.ActivePolicyMemoryRef == nil {
			return fmt.Errorf("Policy Memory is not active")
		}
		if *record.ActivePolicyMemoryRef != receipt || record.ActivePolicyMemory.Revision != receipt.Revision {
			return fmt.Errorf("Policy Memory activation receipt does not match")
		}
		return nil
	}
	return fmt.Errorf("Policy Memory Context is unavailable")
}

type firstUseIntegrationSettlement struct {
	clusterCalls int
	entryCalls   int
}

func (s *firstUseIntegrationSettlement) SettleFinalAuthority(context.Context, tobari.WorkspaceAuthorityCollection, tobari.WorkspaceAuthorityCollection, tobari.ContextID, string, string) error {
	s.entryCalls++
	return nil
}

func (s *firstUseIntegrationSettlement) ConfirmFinalAuthoritySettled(context.Context, tobari.WorkspaceAuthorityCollection, tobari.ContextID) error {
	return nil
}

func (s *firstUseIntegrationSettlement) SettleFinalContextDeletion(context.Context, tobari.WorkspaceAuthorityCollection, tobari.WorkspaceAuthorityCollection, tobari.ContextID, string, string) error {
	return nil
}

func (s *firstUseIntegrationSettlement) ConfirmFinalContextDeletionSettled(context.Context, tobari.WorkspaceAuthorityCollection, tobari.ContextID) error {
	return nil
}

func (s *firstUseIntegrationSettlement) SettleFinalReviewedPolicyAuthority(context.Context, tobari.WorkspaceAuthorityCollection, tobari.WorkspaceAuthorityCollection, tobari.PolicyMemoryReviewedDecisionSet, string, string) (tobari.PolicyMemoryReviewedSettlementReceipt, error) {
	return tobari.PolicyMemoryReviewedSettlementReceipt{}, errors.New("reviewed policy settlement is not part of first-use bootstrap")
}

func (s *firstUseIntegrationSettlement) ConfirmFinalReviewedPolicyAuthority(context.Context, tobari.WorkspaceAuthorityCollection, tobari.PolicyMemoryReviewedDecisionSet) (tobari.PolicyMemoryReviewedSettlementReceipt, error) {
	return tobari.PolicyMemoryReviewedSettlementReceipt{}, errors.New("reviewed policy settlement is not part of first-use bootstrap")
}

func (s *firstUseIntegrationSettlement) PreflightFinalClusterAuthority(_ context.Context, plan tobari.WorkspacePolicyProjection) error {
	return plan.Validate()
}

func (s *firstUseIntegrationSettlement) ReconcileFinalClusterAuthorityWithIdentity(_ context.Context, previous, next tobari.WorkspaceAuthorityCollection, _, _ string) (tobari.PolicyProjectionIdentity, error) {
	transition, err := tobari.PlanWorkspaceAuthorityClusterReconciliation(previous)
	if err != nil {
		return tobari.PolicyProjectionIdentity{}, err
	}
	if err := transition.Plan.ValidateTransition(previous, next); err != nil {
		return tobari.PolicyProjectionIdentity{}, err
	}
	s.clusterCalls++
	return tobari.PolicyProjectionIdentity{
		AggregateRevision: strings.TrimPrefix(string(next.Revision), "sha256:"),
		EvaluatorIdentity: tobari.PolicyEvaluatorIdentity{
			SchemaVersion: 1,
			Version:       "first-use-test-evaluator",
			Digest:        firstUseIntegrationDigest("a"),
		},
		PolicyDataIdentity: tobari.PolicyDataIdentity{SchemaVersion: 1, Digest: firstUseIntegrationDigest("b")},
	}, nil
}

func (s *firstUseIntegrationSettlement) ConfirmFinalClusterAuthoritySettled(_ context.Context, current tobari.WorkspaceAuthorityCollection, expected tobari.PolicyProjectionIdentity) error {
	transition, err := tobari.PlanWorkspaceAuthorityClusterReconciliation(current)
	if err != nil {
		return err
	}
	if err := transition.Plan.ValidateCurrent(current); err != nil {
		return err
	}
	if expected.AggregateRevision != strings.TrimPrefix(string(current.Revision), "sha256:") {
		return fmt.Errorf("cluster aggregate identity does not match")
	}
	return expected.Validate()
}

func firstUseIntegrationDigest(value string) tobari.SemanticDigest {
	return tobari.SemanticDigest("sha256:" + strings.Repeat(value, 64))
}

type firstUseIntegrationEntryRuntime struct {
	prepareCalls   int
	planCalls      int
	reconcileCalls int
	confirmCalls   int
}

func (r *firstUseIntegrationEntryRuntime) AcquireWorkspaceEntryAttachment(_ context.Context, _ tobari.ContextID, _ string) (func() error, error) {
	return func() error { return nil }, nil
}

func (r *firstUseIntegrationEntryRuntime) AcquireWorkspaceReconciliationFence(context.Context) (func() error, error) {
	return func() error { return nil }, nil
}

func (r *firstUseIntegrationEntryRuntime) ContextHomeForID(_ context.Context, id tobari.ContextID) (string, error) {
	return "/context/home-" + string(id), nil
}

func (r *firstUseIntegrationEntryRuntime) PrepareWorkspaceRuntimeMaterial(_ context.Context, binding tobari.RuntimeBinding) error {
	if binding.RuntimeID != tobari.StandardRuntimeID {
		return fmt.Errorf("first-use prepared Runtime ID = %q, want %q", binding.RuntimeID, tobari.StandardRuntimeID)
	}
	r.prepareCalls++
	return nil
}

func (r *firstUseIntegrationEntryRuntime) PlanWorkspaceEntry(_ context.Context, snapshot tobari.ContextAuthoritySnapshot, authority tobari.WorkspaceTemplateEntryAuthority, projectRoot string, workspaceID tobari.WorkspaceID, reconciledAt time.Time) (tobari.WorkspaceEntryReconciliationPlan, error) {
	r.planCalls++
	if err := authority.ValidateFor(snapshot.Template.Current); err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	container, network, err := tobari.ProjectResourceNames(string(workspaceID))
	if err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	_ = container
	workspace := tobari.WorkspaceBinding{
		SchemaVersion:    tobari.WorkspaceBindingSchemaVersion,
		ID:               workspaceID,
		ContextID:        snapshot.Context.ID,
		ProjectRoot:      projectRoot,
		Home:             "/context/home-" + string(snapshot.Context.ID),
		CreationDefaults: snapshot.Template.Current.Slices.CreationDefaultsDigest,
	}
	applied := tobari.WorkspaceAppliedEntry{
		ContextID:        snapshot.Context.ID,
		TemplateID:       snapshot.Template.ID,
		TemplateRevision: snapshot.Template.Current.Revision,
		EntrySliceDigest: snapshot.Template.Current.Slices.EntrySliceDigest,
		RuntimeID:        authority.Runtime.RuntimeID,
		RuntimeRevision:  tobari.SemanticDigest(authority.Runtime.Revision),
		ResolvedSpec:     firstUseIntegrationDigest("c"),
		ReconciledAt:     reconciledAt,
	}
	workspace.LastSuccessfulEntry = &applied
	return tobari.WorkspaceEntryReconciliationPlan{
		Workspace:        workspace,
		Applied:          applied,
		Authority:        authority,
		CreationDefaults: authority.CreationDefaults.Clone(),
		Network: tobari.WorkspaceRuntimeNetworkAuthority{
			Network:       network,
			Subnet:        "10.64.0.0/24",
			DockerGateway: "10.64.0.1",
			GatewayIP:     "10.64.0.2",
			WorkspaceIP:   "10.64.0.3",
		},
	}, nil
}

func (r *firstUseIntegrationEntryRuntime) ReconcileWorkspaceEntry(_ context.Context, plan tobari.WorkspaceEntryReconciliationPlan, _ string) (tobari.WorkspaceEntryReconciliationReceipt, error) {
	r.reconcileCalls++
	return firstUseIntegrationEntryReceipt(plan), nil
}

func (r *firstUseIntegrationEntryRuntime) ConfirmWorkspaceEntry(_ context.Context, plan tobari.WorkspaceEntryReconciliationPlan, _ string) (tobari.WorkspaceEntryReconciliationReceipt, error) {
	r.confirmCalls++
	return firstUseIntegrationEntryReceipt(plan), nil
}

func firstUseIntegrationEntryReceipt(plan tobari.WorkspaceEntryReconciliationPlan) tobari.WorkspaceEntryReconciliationReceipt {
	return tobari.WorkspaceEntryReconciliationReceipt{
		WorkspaceID: plan.Workspace.ID,
		ContextID:   plan.Workspace.ContextID,
		Applied:     plan.Applied,
		ContainerID: strings.Repeat("d", 64),
	}
}

type firstUseIntegrationTemplatePolicy struct {
	memory *firstUseIntegrationActivation
}

func (firstUseIntegrationTemplatePolicy) ObserveWorkspacePolicyAxesCurrent(context.Context, tobari.WorkspaceAuthorityCollection, tobari.ContextID, tobari.TemplatePolicyActivationReceipt, tobari.PolicyMemoryActivationReceipt) (bool, error) {
	return true, nil
}

func (f firstUseIntegrationTemplatePolicy) ConfirmWorkspacePolicyAxesActive(_ context.Context, collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID, templateReceipt tobari.TemplatePolicyActivationReceipt, memoryReceipt tobari.PolicyMemoryActivationReceipt) error {
	for _, record := range collection.Contexts {
		if record.Context.ID != contextID {
			continue
		}
		for _, template := range collection.Templates {
			if template.ID == record.Context.TemplateID {
				if err := templateReceipt.ValidateFor(record.Context, template.Current); err != nil {
					return err
				}
				if record.ActivePolicyMemory == nil || record.ActivePolicyMemoryRef == nil || memoryReceipt != *record.ActivePolicyMemoryRef || memoryReceipt.Revision != record.ActivePolicyMemory.Revision {
					return fmt.Errorf("Policy Memory active receipt mismatch")
				}
				if f.memory != nil {
					f.memory.confirmCalls++
				}
				return nil
			}
		}
	}
	return fmt.Errorf("Template policy Context is unavailable")
}

type firstUseIntegrationSession struct {
	owner firstUseIntegrationSessionOwner
	begin int
	run   int
	close int
}

type firstUseIntegrationSessionOwner struct {
	parent *firstUseIntegrationSession
}

func (s *firstUseIntegrationSession) BeginWorkspaceSession(_ context.Context, binding tobari.WorkspaceSessionBinding, invocationRoot string) (workspaceauthoritystore.WorkspaceSessionOwner, error) {
	s.begin++
	if err := binding.Validate(); err != nil {
		return nil, err
	}
	if err := tobari.ValidateRootContains(binding.ProjectRoot, invocationRoot); err != nil {
		return nil, err
	}
	s.owner.parent = s
	return &s.owner, nil
}

func (s *firstUseIntegrationSessionOwner) Run(context.Context, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (tobari.WorkspaceSessionOutcome, error) {
	s.parent.run++
	return tobari.WorkspaceSessionOutcome{ExitCode: 0, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{}}, nil
}

func (s *firstUseIntegrationSessionOwner) Close(context.Context) error {
	s.parent.close++
	return nil
}

type firstUseIntegrationReadiness struct{}

func (firstUseIntegrationReadiness) Check(context.Context) error { return nil }

type firstUseIntegrationReviewer struct{}

func (firstUseIntegrationReviewer) Review(context.Context, tobari.RecommendedFirstUseDraft, io.Reader, io.Writer) (recommendedFirstUseAction, error) {
	return recommendedFirstUseStart, nil
}

type firstUseIntegrationMigrationStage struct{}

func (firstUseIntegrationMigrationStage) ExpectedIdentity() (tobari.SemanticDigest, error) {
	return firstUseIntegrationDigest("m"), nil
}
func (firstUseIntegrationMigrationStage) Commit(context.Context) error   { return nil }
func (firstUseIntegrationMigrationStage) Verify(context.Context) error   { return nil }
func (firstUseIntegrationMigrationStage) Rollback(context.Context) error { return nil }
func (firstUseIntegrationMigrationStage) Complete(context.Context) error { return nil }
func (firstUseIntegrationMigrationStage) Abort(context.Context) error    { return nil }

func TestFinalRootFreshStartBootstrapsAuthorityClusterAndWorkspaceFromEmptyXDG(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	stateHome := filepath.Join(root, "state")
	dataHome := filepath.Join(root, "data")
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	configRoot := filepath.Join(configHome, "tobari")
	stateRoot := filepath.Join(stateHome, "tobari")
	authorityRoot := filepath.Join(stateRoot, "authority")
	for _, path := range []string{configRoot, stateRoot} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("fresh XDG path %s already exists: %v", path, err)
		}
	}
	lifetime := context.Background()
	lifecycle := &firstUseIntegrationLifecycle{parent: stateRoot}
	guard, err := dockerruntime.New(lifetime)
	if err != nil {
		t.Fatal(err)
	}
	store, err := workspaceauthoritystore.NewFinalOnly(authorityRoot, guard)
	if err != nil {
		t.Fatal(err)
	}
	revision := "sha256:" + strings.Repeat("f", 64)
	runtime := &firstUseIntegrationRuntime{revision: revision}
	activation := &firstUseIntegrationActivation{}
	settlement := &firstUseIntegrationSettlement{}
	mutator, err := workspaceauthoritystore.NewMutator(lifetime, store, lifecycle, runtime, nil, activation, settlement)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := workspaceauthoritysource.New(configRoot)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := workspaceauthorityresources.New(
		store, mutator, sources,
		func(context.Context, tobari.WorkspaceAuthorityCollection) (tobari.SemanticDigest, error) {
			return firstUseIntegrationDigest("r"), nil
		},
		func(context.Context, tobari.WorkspaceAuthorityCollection, bool) (workspaceauthoritystore.InstallationMigrationSourceStage, error) {
			return firstUseIntegrationMigrationStage{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(root, "project")
	defaultPairAuthority, err := workspaceauthoritystore.NewDefaultPairAdapter(store, firstUseIntegrationProjectRoot{root: projectRoot})
	if err != nil {
		t.Fatal(err)
	}
	entryRuntime := &firstUseIntegrationEntryRuntime{}
	sessions := &firstUseIntegrationSession{}
	entry, err := workspaceauthoritystore.NewContextEntryAdapter(mutator, entryRuntime, firstUseIntegrationTemplatePolicy{memory: activation}, sessions, lifetime, resources)
	if err != nil {
		t.Fatal(err)
	}
	finalAuthority := &finalWorkspaceAuthorityAdapter{Adapter: resources, ContextEntryAdapter: entry}
	contexts := workspaceauthoritycmd.NewContextService(finalAuthority)
	pair := workspaceauthoritycmd.NewDefaultPairService(defaultPairAuthority, resources, contexts)
	clusterAdapter, err := workspaceauthoritystore.NewClusterAdapter(mutator)
	if err != nil {
		t.Fatal(err)
	}
	cluster := workspaceauthoritycmd.NewFinalClusterService(clusterAdapter)

	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), nil)
	command.processLifetime = lifetime
	command.finalDefaultPair = pair
	command.finalContexts = contexts
	command.finalEntryReadiness = firstUseIntegrationReadiness{}
	command.finalCluster = cluster
	command.firstUse = firstUseIntegrationReviewer{}
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	command.firstUseTemplateBody = func(context.Context) (tobari.WorkspaceTemplateBody, error) {
		body := finalAxisTemplateBody("/bootstrap")
		body.Boundary.SourceAccess = tobari.ManifestSourceAccessReadWrite
		body.EntryDefaults.Runtime.Revision = revision
		if body.EntryDefaults.Runtime.RuntimeID != tobari.StandardRuntimeID {
			return tobari.WorkspaceTemplateBody{}, fmt.Errorf("first-use Template Runtime ID = %q, want %q", body.EntryDefaults.Runtime.RuntimeID, tobari.StandardRuntimeID)
		}
		if err := body.Validate(); err != nil {
			return tobari.WorkspaceTemplateBody{}, err
		}
		return body, nil
	}
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("fresh first-use exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if len(runtime.refs) == 0 {
		t.Fatal("first-use Template source did not resolve its Runtime revision")
	}
	expectedRuntimeRef := tobari.RuntimeRevisionRef(tobari.StandardRuntimeID, revision)
	for _, ref := range runtime.refs {
		if ref != expectedRuntimeRef {
			t.Fatalf("first-use Runtime reference=%q want=%q", ref, expectedRuntimeRef)
		}
	}
	if settlement.clusterCalls != 1 || settlement.entryCalls != 1 || entryRuntime.prepareCalls != 1 || entryRuntime.planCalls != 1 || entryRuntime.reconcileCalls != 1 || entryRuntime.confirmCalls != 1 || sessions.begin != 1 || sessions.run != 1 || sessions.close != 1 || activation.confirmCalls != 1 {
		t.Fatalf("bootstrap calls cluster=%d entry=%d plan=%d reconcile=%d confirm=%d session=%d/%d/%d activation=%d", settlement.clusterCalls, settlement.entryCalls, entryRuntime.planCalls, entryRuntime.reconcileCalls, entryRuntime.confirmCalls, sessions.begin, sessions.run, sessions.close, activation.confirmCalls)
	}

	collection, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || len(collection.Templates) != 1 || len(collection.Contexts) != 1 || len(collection.Workspaces) != 1 || collection.DefaultTemplateID == nil {
		t.Fatalf("fresh final authority present=%t collection=%#v err=%v", present, collection, err)
	}
	if collection.Contexts[0].ActiveTemplatePolicy == nil || collection.Contexts[0].ActivePolicyMemory == nil || collection.Contexts[0].ActivePolicyMemoryRef == nil || collection.Workspaces[0].LastSuccessfulEntry == nil {
		t.Fatalf("fresh final authority did not retain active cluster and Workspace entry axes: %#v", collection)
	}
	observed, err := pair.Observe(context.Background())
	if err != nil || !observed.CollectionPresent || observed.Context == nil {
		t.Fatalf("post-create default-pair observation=%#v err=%v", observed, err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"context", "list", "--format=json"}); code != ExitOK {
		t.Fatalf("post-create context list exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "legacy_state_present") || strings.Contains(stdout.String()+stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("post-create context list regressed to legacy failure: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	drafts, err := resources.ListWorkspaceTemplateDrafts(context.Background())
	if err != nil || len(drafts) != 0 {
		t.Fatalf("fresh bootstrap left unbound Template drafts=%#v err=%v", drafts, err)
	}
	for _, path := range []string{configRoot, stateRoot, authorityRoot, filepath.Join(authorityRoot, "active.json")} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("fresh bootstrap did not publish %s: %v", path, err)
		}
	}
}

type firstUseIntegrationProjectRoot struct {
	root string
}

func (r firstUseIntegrationProjectRoot) CurrentDirectory(context.Context) (string, error) {
	return r.root, nil
}
func (r firstUseIntegrationProjectRoot) ResolveProjectRoot(context.Context, string) (string, error) {
	return r.root, nil
}

var _ workspaceauthoritystore.LifecycleAuthority = (*firstUseIntegrationLifecycle)(nil)
var _ workspaceauthoritystore.WorkspaceTemplateRuntimeRevisionAuthority = (*firstUseIntegrationRuntime)(nil)
var _ workspaceauthoritystore.PolicyMemoryActivationAuthority = (*firstUseIntegrationActivation)(nil)
var _ workspaceauthoritystore.FinalAuthoritySettlementAuthority = (*firstUseIntegrationSettlement)(nil)
var _ workspaceauthoritystore.WorkspaceEntryRuntimeAuthority = (*firstUseIntegrationEntryRuntime)(nil)
var _ workspaceauthoritystore.WorkspacePolicyActivationAuthority = firstUseIntegrationTemplatePolicy{}
var _ workspaceauthoritystore.WorkspaceSessionAuthority = (*firstUseIntegrationSession)(nil)
var _ workspaceauthoritystore.FinalCanonicalProjectRootAuthority = firstUseIntegrationProjectRoot{}
