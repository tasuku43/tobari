package workspaceauthoritycmd

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type defaultPairFixture struct {
	root            string
	secondRoot      string
	rootCalls       int
	present         bool
	selected        bool
	template        *tobari.WorkspaceTemplate
	snapshot        *tobari.ContextAuthoritySnapshot
	legacyErr       error
	initializeCalls int
	templateCreates int
	defaultWrites   int
	contextCreates  int
	entries         int
	entryErr        error
	generation      uint64
	revisionDigit   byte
}

type finalSelectionAuthorityFixture struct {
	*defaultPairFixture
	selection tobari.FinalDefaultPairSelection
	inside    bool
	reads     int
	driftAt   int
}

func (f *finalSelectionAuthorityFixture) ObserveFinalDefaultPairSelection(context.Context) (tobari.FinalDefaultPairSelection, error) {
	f.reads++
	result := f.selection.Clone()
	if f.driftAt > 0 && f.reads >= f.driftAt {
		result.CollectionGeneration++
		result.CollectionRevision = digest("e")
	}
	return result, nil
}

func (f *finalSelectionAuthorityFixture) InsideFinalWorkspace(context.Context) bool { return f.inside }

type finalSelectionChoiceFixture struct {
	choice tobari.FinalDefaultPairSelectionChoice
	calls  int
}

func (f *finalSelectionChoiceFixture) Select(context.Context, tobari.FinalDefaultPairSelection, io.Reader, io.Writer) (tobari.FinalDefaultPairSelectionChoice, error) {
	f.calls++
	return f.choice, nil
}

func finalAncestorSelectionFixture(t *testing.T) (*finalSelectionAuthorityFixture, tobari.ContextID) {
	t.Helper()
	base := &defaultPairFixture{root: "/workspace/example", revisionDigit: 'a'}
	if _, err := base.InitializeFinalDefaultPair(context.Background(), base.root, bodyFixture("/first-use")); err != nil {
		t.Fatal(err)
	}
	base.initializeCalls = 0
	base.templateCreates = 0
	base.defaultWrites = 0
	base.contextCreates = 0
	observation, err := base.ObserveFinalDefaultPair(context.Background(), base.root)
	if err != nil {
		t.Fatal(err)
	}
	observation.Context.Workspace = &tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion,
		ID:            workspaceID, ContextID: observation.Context.Context.ID,
		ProjectRoot: base.root, Home: "/workspace/home",
		CreationDefaults: observation.Context.Template.Current.Slices.CreationDefaultsDigest,
	}
	observation.Context.ContextHome = observation.Context.Workspace.Home
	observation.Context.ContextCreationDefaults = observation.Context.Workspace.CreationDefaults
	observation.Context.Workspaces = []tobari.WorkspaceBinding{*observation.Context.Workspace}
	selection := tobari.FinalDefaultPairSelection{
		SchemaVersion: tobari.FinalDefaultPairSelectionSchemaVersion, CollectionPresent: true,
		CollectionGeneration: observation.CollectionGeneration, CollectionRevision: observation.CollectionRevision,
		CanonicalCWD: "/workspace/example/src/pkg", DefaultTemplate: observation.DefaultTemplate,
		Candidates: []tobari.FinalDefaultPairCandidate{{Snapshot: observation.Context.Clone()}},
	}
	if err := selection.Validate(); err != nil {
		t.Fatal(err)
	}
	return &finalSelectionAuthorityFixture{defaultPairFixture: base, selection: selection}, observation.Context.Context.ID
}

func (f *defaultPairFixture) ObserveFinalCanonicalProjectRoot(context.Context) (string, error) {
	f.rootCalls++
	if f.secondRoot != "" && f.rootCalls%2 == 0 {
		return f.secondRoot, nil
	}
	return f.root, nil
}

func (f *defaultPairFixture) ObserveFinalDefaultPair(context.Context, string) (tobari.FinalDefaultPairObservation, error) {
	if f.legacyErr != nil {
		return tobari.FinalDefaultPairObservation{}, f.legacyErr
	}
	result := tobari.FinalDefaultPairObservation{SchemaVersion: tobari.FinalDefaultPairObservationSchemaVersion, CollectionPresent: f.present, ProjectRoot: f.root}
	if f.present {
		result.CollectionGeneration = f.generation
		result.CollectionRevision = digest(string([]byte{f.revisionDigit}))
	}
	if f.selected && f.template != nil {
		value := f.template.Clone()
		result.DefaultTemplate = &value
	}
	if f.snapshot != nil {
		value := f.snapshot.Clone()
		result.Context = &value
	}
	return result, result.Validate()
}

func (f *defaultPairFixture) ObserveFinalDefaultPairContext(_ context.Context, root string, id tobari.ContextID) (tobari.FinalDefaultPairObservation, error) {
	result, err := f.ObserveFinalDefaultPair(context.Background(), root)
	if err != nil {
		return tobari.FinalDefaultPairObservation{}, err
	}
	if f.snapshot == nil || f.snapshot.Context.ID != id {
		return tobari.FinalDefaultPairObservation{}, tobari.ErrContextBindingNotFound
	}
	value := f.snapshot.Clone()
	result.Context = &value
	return result, result.Validate()
}

func (f *defaultPairFixture) InitializeFinalDefaultPair(_ context.Context, root string, body tobari.WorkspaceTemplateBody) (tobari.FinalDefaultPairPublication, error) {
	return f.initializeFinalDefaultPair(root, templateID, body, false)
}

func (f *defaultPairFixture) InitializeFinalDefaultPairWithTemplateID(_ context.Context, root string, id tobari.WorkspaceTemplateID, body tobari.WorkspaceTemplateBody) (tobari.FinalDefaultPairPublication, error) {
	return f.initializeFinalDefaultPair(root, id, body, true)
}

func (f *defaultPairFixture) InitializeFinalDefaultPairWithConfiguratorIDs(_ context.Context, root string, id tobari.WorkspaceTemplateID, context tobari.ContextID, body tobari.WorkspaceTemplateBody) (tobari.FinalDefaultPairPublication, error) {
	publication, err := f.initializeFinalDefaultPair(root, id, body, true)
	if err == nil && publication.Current.Context != nil && publication.Current.Context.Context.ID != context {
		return tobari.FinalDefaultPairPublication{}, tobari.ErrResourceSourceRecoveryRequired
	}
	return publication, err
}

func (f *defaultPairFixture) initializeFinalDefaultPair(root string, id tobari.WorkspaceTemplateID, body tobari.WorkspaceTemplateBody, recoverExact bool) (tobari.FinalDefaultPairPublication, error) {
	f.initializeCalls++
	previous, err := f.ObserveFinalDefaultPair(context.Background(), root)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	changed := false
	if f.present && !f.selected {
		if !recoverExact || f.template == nil || f.template.ID != id || !reflect.DeepEqual(f.template.Current.Body, body) {
			return tobari.FinalDefaultPairPublication{}, tobari.ErrDefaultTemplateSelectionRequired
		}
		f.selected = true
		f.defaultWrites++
		f.advance()
		changed = true
	}
	if !f.present {
		revision, err := tobari.NewWorkspaceTemplateRevision(id, 1, body)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: id, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
		f.template = &template
		f.selected = true
		f.templateCreates++
		f.defaultWrites++
		f.advance()
		changed = true
	}
	if f.snapshot == nil {
		memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		value := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: f.template.ID}, Template: f.template.Clone(), PolicyMemory: memory}
		f.snapshot = &value
		f.contextCreates++
		f.advance()
		changed = true
	}
	current, err := f.ObserveFinalDefaultPair(context.Background(), root)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	return tobari.FinalDefaultPairPublication{Previous: previous, Current: current, Changed: changed}, nil
}

func (f *defaultPairFixture) ListWorkspaceTemplates(context.Context) ([]tobari.WorkspaceTemplate, error) {
	if f.template == nil {
		return []tobari.WorkspaceTemplate{}, nil
	}
	return []tobari.WorkspaceTemplate{f.template.Clone()}, nil
}

func (f *defaultPairFixture) DiscoverWorkspaceTemplate(context.Context, string) (tobari.WorkspaceTemplate, error) {
	if f.template == nil {
		return tobari.WorkspaceTemplate{}, tobari.ErrWorkspaceTemplateNotFound
	}
	return f.template.Clone(), nil
}

func (f *defaultPairFixture) CreateWorkspaceTemplate(_ context.Context, name string, body tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplate, error) {
	f.templateCreates++
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		return tobari.WorkspaceTemplate{}, err
	}
	value := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: name, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	f.template = &value
	f.advance()
	return value.Clone(), nil
}

func (f *defaultPairFixture) CopyWorkspaceTemplateByRevisionReference(context.Context, string, string) (tobari.WorkspaceTemplateCopyPublication, error) {
	return tobari.WorkspaceTemplateCopyPublication{}, errors.New("unexpected copy")
}

func (f *defaultPairFixture) UpdateWorkspaceTemplateByReference(context.Context, string, tobari.WorkspaceTemplateChange) (tobari.WorkspaceTemplateRevisionPublication, error) {
	return tobari.WorkspaceTemplateRevisionPublication{}, errors.New("unexpected update")
}

func (f *defaultPairFixture) SetDefaultWorkspaceTemplateByReference(context.Context, string) (tobari.WorkspaceTemplateSelectionResult, error) {
	f.defaultWrites++
	f.selected = true
	f.advance()
	return tobari.WorkspaceTemplateSelectionResult{TemplateID: f.template.ID, Selected: true}, nil
}

func (f *defaultPairFixture) DeleteWorkspaceTemplateByReference(context.Context, string) (tobari.WorkspaceTemplateDeleteResult, error) {
	return tobari.WorkspaceTemplateDeleteResult{}, errors.New("unexpected delete")
}

func (f *defaultPairFixture) ListContextAuthority(context.Context) ([]tobari.ContextAuthoritySnapshot, error) {
	if f.snapshot == nil {
		return []tobari.ContextAuthoritySnapshot{}, nil
	}
	return []tobari.ContextAuthoritySnapshot{f.snapshot.Clone()}, nil
}

func (f *defaultPairFixture) ReadContextAuthorityByReference(context.Context, string) (tobari.ContextAuthoritySnapshot, error) {
	if f.snapshot == nil {
		return tobari.ContextAuthoritySnapshot{}, tobari.ErrContextBindingNotFound
	}
	return f.snapshot.Clone(), nil
}

func (f *defaultPairFixture) CreateContextByTemplateReference(context.Context, string, string) (tobari.ContextAuthoritySnapshot, error) {
	f.contextCreates++
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, err
	}
	value := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: f.template.ID}, Template: f.template.Clone(), PolicyMemory: memory}
	f.snapshot = &value
	f.advance()
	return value.Clone(), nil
}

func (f *defaultPairFixture) EnterContextByReference(context.Context, string, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (tobari.ContextEntryPublication, error) {
	f.entries++
	if f.entryErr != nil {
		return tobari.ContextEntryPublication{}, f.entryErr
	}
	value := f.snapshot.Clone()
	templateReceipt := tobari.TemplatePolicyActivationReceipt{ContextID: contextID, TemplateID: templateID, PolicySliceDigest: value.Template.Current.Slices.PolicySliceDigest}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: value.PolicyMemory.Revision}
	activeMemory := value.PolicyMemory.Clone()
	value.ActiveTemplatePolicy = &templateReceipt
	value.ActivePolicyMemory = &activeMemory
	value.ActivePolicyMemoryRef = &memoryReceipt
	applied := tobari.WorkspaceAppliedEntry{ContextID: contextID, TemplateID: templateID, TemplateRevision: value.Template.Current.Revision, EntrySliceDigest: value.Template.Current.Slices.EntrySliceDigest, RuntimeID: tobari.StandardRuntimeID, RuntimeRevision: value.Template.Current.Slices.RuntimeRevision, ResolvedSpec: digest("9"), ReconciledAt: time.Unix(10, 0).UTC()}
	workspace := tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID, ProjectRoot: f.root, Home: "/workspace/home", CreationDefaults: value.Template.Current.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied}
	value.ContextHome = workspace.Home
	value.ContextCreationDefaults = workspace.CreationDefaults
	value.Workspaces = []tobari.WorkspaceBinding{workspace}
	value.Workspace = &workspace
	f.snapshot = &value
	f.advance()
	return tobari.ContextEntryPublication{Snapshot: value.Clone(), Outcome: tobari.WorkspaceSessionOutcome{ExitCode: 0, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{}}}, nil
}

func (f *defaultPairFixture) EnterFinalDefaultPair(ctx context.Context, expected tobari.FinalDefaultPairObservation, invocationRoot string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (tobari.ContextEntryPublication, error) {
	if err := tobari.ValidateRootContains(expected.ProjectRoot, invocationRoot); err != nil {
		return tobari.ContextEntryPublication{}, err
	}
	observed, err := f.ObserveFinalDefaultPair(ctx, expected.ProjectRoot)
	if err != nil {
		return tobari.ContextEntryPublication{}, err
	}
	if !reflect.DeepEqual(observed, expected) {
		return tobari.ContextEntryPublication{}, errors.New("default pair changed before entry")
	}
	contextRef, err := tobari.ContextRef(expected.Context.Context.ID)
	if err != nil {
		return tobari.ContextEntryPublication{}, err
	}
	return f.EnterContextByReference(ctx, contextRef, session, in, out, errOut)
}

func TestDefaultPairConfirmedInitializationIsPartialWhenEntryDoesNotStart(t *testing.T) {
	entryErr := fault.WithClassification(fault.New(fault.KindCanceled, "operation_canceled", "entry canceled before execution", true), fault.PhasePrecondition, fault.ChangeNone)
	fixture := &defaultPairFixture{root: "/workspace/example", revisionDigit: 'a', entryErr: entryErr}
	service := NewDefaultPairService(fixture, fixture, NewContextService(fixture))
	intent := operation.Intent{Command: TaskDefaultPairEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}, Impact: DefaultPairEnterImpact()}
	_, err := service.Enter(context.Background(), intent, bodyFixture("/first-use"), tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "default_pair_initialized" || public.ChangeState != fault.ChangePartial || fixture.initializeCalls != 1 || fixture.entries != 1 {
		t.Fatalf("confirmed initialization was not retained as partial: err=%v public=%+v fixture=%+v", err, public, fixture)
	}
}

func TestDefaultPairAncestorSelectionPreservesInvocationRootAndNeverInitializesNestedContext(t *testing.T) {
	fixture, contextID := finalAncestorSelectionFixture(t)
	selector := &finalSelectionChoiceFixture{choice: tobari.FinalDefaultPairSelectionChoice{Kind: tobari.FinalDefaultPairSelectionUse, ContextID: contextID}}
	service := NewDefaultPairService(fixture, fixture, NewContextService(fixture), selector)
	selected, err := service.Select(context.Background(), strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	intent := operation.Intent{Command: TaskDefaultPairEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}, Impact: DefaultPairEnterImpact()}
	resolution, err := service.ResolveSelected(context.Background(), intent, nil, selected)
	if err != nil {
		t.Fatal(err)
	}
	if selector.calls != 1 || resolution.InvocationRoot != fixture.selection.CanonicalCWD || resolution.Observation.ProjectRoot != fixture.root || resolution.AuthorityChanged || fixture.initializeCalls != 0 || fixture.contextCreates != 0 {
		t.Fatalf("selection=%+v selector=%d init=%d contexts=%d", resolution, selector.calls, fixture.initializeCalls, fixture.contextCreates)
	}
	observed, err := service.observeResolved(context.Background(), resolution)
	if err != nil {
		t.Fatalf("reobserve selected ancestor: %v", err)
	}
	if observed.ProjectRoot != fixture.root || observed.Context == nil || observed.Context.Context.ID != contextID {
		t.Fatalf("reobserved=%+v", observed)
	}
}

func TestDefaultPairSelectionDriftFailsBeforeNestedInitialization(t *testing.T) {
	fixture, contextID := finalAncestorSelectionFixture(t)
	fixture.driftAt = 3
	selector := &finalSelectionChoiceFixture{choice: tobari.FinalDefaultPairSelectionChoice{Kind: tobari.FinalDefaultPairSelectionUse, ContextID: contextID}}
	service := NewDefaultPairService(fixture, fixture, NewContextService(fixture), selector)
	selected, err := service.Select(context.Background(), strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	intent := operation.Intent{Command: TaskDefaultPairEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}, Impact: DefaultPairEnterImpact()}
	_, err = service.ResolveSelected(context.Background(), intent, nil, selected)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "default_pair_changed" || fixture.initializeCalls != 0 || fixture.contextCreates != 0 {
		t.Fatalf("drift fault=%+v err=%v init=%d contexts=%d", public, err, fixture.initializeCalls, fixture.contextCreates)
	}
}

func TestDefaultPairRejectsWorkspaceRecursionBeforeAuthorityObservation(t *testing.T) {
	fixture, _ := finalAncestorSelectionFixture(t)
	fixture.inside = true
	service := NewDefaultPairService(fixture, fixture, NewContextService(fixture), &finalSelectionChoiceFixture{})
	_, err := service.Select(context.Background(), strings.NewReader(""), io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "already_inside" || fixture.reads != 0 || fixture.initializeCalls != 0 || fixture.entries != 0 {
		t.Fatalf("inside fault=%+v err=%v reads=%d init=%d entries=%d", public, err, fixture.reads, fixture.initializeCalls, fixture.entries)
	}
}

func TestDefaultPairInitializationFaultClassifiesUnknownPublication(t *testing.T) {
	public, ok := fault.PublicCopy(defaultPairInitializationFault(errors.New("synthetic initialization failure")))
	if !ok || public.Code != "invalid_default_pair_initialization" || public.Kind != fault.KindContract || public.Phase != fault.PhaseVerification || public.ChangeState != fault.ChangeUnknown {
		t.Fatalf("initialization failure fault=%#v ok=%t", public, ok)
	}
}

func TestDefaultPairEntryInterruptionOverridesNestedPreconditionFault(t *testing.T) {
	nested := workspaceMountGuardFaultForApplicationTest()
	public, ok := fault.PublicCopy(defaultPairMutationFault(errors.Join(tobari.ErrWorkspaceEntryInterrupted, nested)))
	if !ok || public.Code != "workspace_entry_interrupted" || public.Kind != fault.KindUnavailable || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangePartial {
		t.Fatalf("interrupted default-pair entry fault=%#v ok=%t", public, ok)
	}
}

func workspaceMountGuardFaultForApplicationTest() error {
	return fault.WithClassification(
		fault.New(fault.KindRejected, "workspace_entry_overlap_unsafe", "synthetic mount guard", false),
		fault.PhasePrecondition,
		fault.ChangeNone,
	)
}

func (f *defaultPairFixture) DeleteContextByReference(context.Context, string) (tobari.ContextDeleteResult, error) {
	return tobari.ContextDeleteResult{}, errors.New("unexpected delete")
}

func (f *defaultPairFixture) advance() {
	f.present = true
	f.generation++
	if f.generation == 0 {
		f.generation = 1
	}
	f.revisionDigit = byte('a' + f.generation%20)
}

func TestDefaultPairFreshEntryComposesExactTemplateContextAndEntryTasks(t *testing.T) {
	fixture := &defaultPairFixture{root: "/workspace/example", revisionDigit: 'a'}
	contexts := NewContextService(fixture)
	service := NewDefaultPairService(fixture, fixture, contexts)
	intent := operation.Intent{Command: TaskDefaultPairEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}, Impact: DefaultPairEnterImpact()}
	result, err := service.Enter(context.Background(), intent, bodyFixture("/first-use"), tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if fixture.initializeCalls != 1 || fixture.templateCreates != 1 || fixture.defaultWrites != 1 || fixture.contextCreates != 1 || fixture.entries != 1 || result.ContextRef == "" || result.WorkspaceRef == "" {
		t.Fatalf("first-use task chain mismatch: creates=%d default=%d contexts=%d entries=%d result=%+v", fixture.templateCreates, fixture.defaultWrites, fixture.contextCreates, fixture.entries, result)
	}
	status, err := service.Status(context.Background())
	if err != nil || status.SchemaVersion != 3 || status.WorkspaceRef != result.WorkspaceRef || status.AppliedEntry == nil {
		t.Fatalf("unexpected final default-pair status: %+v err=%v", status, err)
	}
}

func TestDefaultPairEntryRejectsInitializedCollectionWithoutExactDefaultSelection(t *testing.T) {
	body := bodyFixture("/first-use")
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	fixture := &defaultPairFixture{root: "/workspace/example", present: true, template: &template, generation: 1, revisionDigit: 'b'}
	service := NewDefaultPairService(fixture, fixture, NewContextService(fixture))
	intent := operation.Intent{Command: TaskDefaultPairEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}, Impact: DefaultPairEnterImpact()}
	_, err = service.Enter(context.Background(), intent, body, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "default_template_required" || fixture.templateCreates != 0 || fixture.defaultWrites != 0 || fixture.contextCreates != 0 || fixture.entries != 0 {
		t.Fatalf("initialized collection without default was not rejected before mutation: err=%v fixture=%+v", err, fixture)
	}
}

func TestConfiguratorExactTemplateResumesAfterTemplatePublicationBeforeDefaultSelection(t *testing.T) {
	body := bodyFixture("/first-use")
	preferred := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ba")
	revision, err := tobari.NewWorkspaceTemplateRevision(preferred, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: preferred, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	fixture := &defaultPairFixture{root: "/workspace/example", present: true, template: &template, generation: 1, revisionDigit: 'b'}
	service := NewDefaultPairService(fixture, fixture, NewContextService(fixture))
	selected, err := service.Select(context.Background(), strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	intent := operation.Intent{Command: TaskDefaultPairEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}, Impact: DefaultPairEnterImpact()}
	resolution, err := service.ResolveSelectedWithConfiguratorIDs(context.Background(), intent, &body, preferred, contextID, selected)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Observation.Context == nil || resolution.Observation.Context.Template.ID != preferred || fixture.templateCreates != 0 || fixture.defaultWrites != 1 || fixture.contextCreates != 1 {
		t.Fatalf("exact recovery resolution=%+v fixture=%+v", resolution, fixture)
	}
}

func TestDefaultPairDriftAndLegacyAuthorityFailBeforeTaskMutation(t *testing.T) {
	for _, test := range []struct {
		name    string
		fixture *defaultPairFixture
		code    string
	}{
		{name: "cwd drift", fixture: &defaultPairFixture{root: "/workspace/a", secondRoot: "/workspace/b"}, code: "default_pair_changed"},
		{name: "legacy", fixture: &defaultPairFixture{root: "/workspace/a", legacyErr: tobari.ErrPreReleaseLegacyAuthority}, code: "legacy_state_present"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := NewDefaultPairService(test.fixture, test.fixture, NewContextService(test.fixture))
			intent := operation.Intent{Command: TaskDefaultPairEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}, Impact: DefaultPairEnterImpact()}
			_, err := service.Enter(context.Background(), intent, bodyFixture("/first-use"), tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != test.code || test.fixture.templateCreates != 0 || test.fixture.defaultWrites != 0 || test.fixture.contextCreates != 0 || test.fixture.entries != 0 {
				t.Fatalf("drift/legacy did not fail before mutation: err=%v public=%+v fixture=%+v", err, public, test.fixture)
			}
		})
	}
}

func TestDefaultPairStatusRejectsOwnerlessWorkspaceRef(t *testing.T) {
	status := DefaultPairStatus{SchemaVersion: DefaultPairStatusSchemaVersion, Task: TaskDefaultPairStatus, ProjectRoot: "/workspace/example", AuthorityState: "initialized", DefaultTemplateState: "selected", WorkspaceTemplateID: templateID, TemplateName: "default", DesiredTemplateGeneration: 1, DesiredTemplateRevision: digest("a"), DesiredTemplatePolicySliceDigest: digest("c"), ContextID: contextID, CurrentPolicyMemoryRevision: digest("b"), WorkspaceID: workspaceID, WorkspaceRef: "wrk_" + strings.Repeat("0", 32), WorkspaceHome: "/workspace/home"}
	if err := status.Validate(); err == nil {
		t.Fatal("unrelated Workspace reference validated")
	}
}

func TestDefaultPairStatusPreservesFreshFinalEmptyAuthority(t *testing.T) {
	fixture := &defaultPairFixture{root: "/workspace/fresh", revisionDigit: 'a'}
	status, err := NewDefaultPairService(fixture, fixture, NewContextService(fixture)).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.SchemaVersion != DefaultPairStatusSchemaVersion || status.Task != TaskDefaultPairStatus || status.ProjectRoot != fixture.root || status.AuthorityState != "empty" || status.DefaultTemplateState != "absent" {
		t.Fatalf("fresh status = %+v", status)
	}
	if fixture.initializeCalls != 0 || fixture.templateCreates != 0 || fixture.defaultWrites != 0 || fixture.contextCreates != 0 || fixture.entries != 0 {
		t.Fatalf("fresh status mutated authority: %+v", fixture)
	}
}

func TestDefaultPairStatusOwnsIndependentDesiredActiveAndAppliedAuthority(t *testing.T) {
	bodyA := bodyFixture("/policy-a")
	revisionA, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, bodyA)
	if err != nil {
		t.Fatal(err)
	}
	bodyB := bodyFixture("/policy-b")
	revisionB, changed, err := tobari.AdvanceWorkspaceTemplateRevision(revisionA, bodyB)
	if err != nil || !changed {
		t.Fatalf("advance Template revision: changed=%t err=%v", changed, err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revisionB, Retained: []tobari.WorkspaceTemplateRevision{revisionA.Clone(), revisionB.Clone()}}
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}
	memoryA, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewPolicyMemoryRule(contextID, tobari.PolicyMemoryDeny, tobari.PolicyMemoryRuleBody{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Match:                  tobari.PolicyMatchExact,
		Host:                   "api.example.dev",
		Port:                   443,
		Method:                 "POST",
		Path:                   "/later",
		Segments:               []string{},
		Examples:               []string{"/later"},
		SourceCandidates:       []string{"pcy_0123456789abcdef0123456789abcdef"},
	})
	if err != nil {
		t.Fatal(err)
	}
	memoryB, changed, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{rule}, &memoryA)
	if err != nil || !changed {
		t.Fatalf("advance Policy Memory: changed=%t err=%v", changed, err)
	}
	activeTemplate := tobari.TemplatePolicyActivationReceipt{ContextID: contextID, TemplateID: templateID, PolicySliceDigest: revisionA.Slices.PolicySliceDigest}
	activeMemory := memoryA.Clone()
	activeMemoryRef := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: memoryA.Revision}
	applied := tobari.WorkspaceAppliedEntry{ContextID: contextID, TemplateID: templateID, TemplateRevision: revisionA.Revision, EntrySliceDigest: revisionA.Slices.EntrySliceDigest, RuntimeID: tobari.StandardRuntimeID, RuntimeRevision: revisionA.Slices.RuntimeRevision, ResolvedSpec: digest("7"), ReconciledAt: time.Unix(1, 0).UTC()}
	workspace := tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID, ProjectRoot: "/workspace/example", Home: "/workspace/home", CreationDefaults: revisionA.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied}
	snapshot := tobari.ContextAuthoritySnapshot{Context: binding, Template: template, ContextHome: workspace.Home, ContextCreationDefaults: workspace.CreationDefaults, PolicyMemory: memoryB, ActiveTemplatePolicy: &activeTemplate, ActivePolicyMemory: &activeMemory, ActivePolicyMemoryRef: &activeMemoryRef, Workspaces: []tobari.WorkspaceBinding{workspace}, Workspace: &workspace}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	observation := tobari.FinalDefaultPairObservation{SchemaVersion: tobari.FinalDefaultPairObservationSchemaVersion, CollectionPresent: true, CollectionGeneration: 9, CollectionRevision: digest("9"), ProjectRoot: workspace.ProjectRoot, DefaultTemplate: &template, Context: &snapshot}
	status, err := NewDefaultPairStatus(observation)
	if err != nil {
		t.Fatal(err)
	}
	if status.DesiredTemplateRevision != revisionB.Revision || status.CurrentPolicyMemoryRevision != memoryB.Revision {
		t.Fatalf("desired/current authority mismatch: status=%+v", status)
	}
	typeOfStatus := reflect.TypeOf(status)
	valueOfStatus := reflect.ValueOf(status)
	for _, required := range []struct {
		name      string
		nullable  bool
		wantValue any
	}{
		{name: "DesiredTemplateGeneration", wantValue: revisionB.Generation},
		{name: "DesiredTemplateRevision", wantValue: revisionB.Revision},
		{name: "DesiredTemplatePolicySliceDigest", wantValue: revisionB.Slices.PolicySliceDigest},
		{name: "ActiveTemplatePolicySliceDigest", nullable: true, wantValue: revisionA.Slices.PolicySliceDigest},
		{name: "CurrentPolicyMemoryRevision", wantValue: memoryB.Revision},
		{name: "ActivePolicyMemoryRevision", nullable: true, wantValue: memoryA.Revision},
		{name: "AppliedEntry", nullable: true, wantValue: applied},
	} {
		field, ok := typeOfStatus.FieldByName(required.name)
		if !ok {
			t.Errorf("DefaultPairStatus is missing %s", required.name)
			continue
		}
		if required.nullable && field.Type.Kind() != reflect.Pointer {
			t.Errorf("DefaultPairStatus.%s type=%s, want pointer-backed nullable authority", required.name, field.Type)
			continue
		}
		got := valueOfStatus.FieldByIndex(field.Index)
		if required.nullable {
			if got.IsNil() {
				t.Errorf("DefaultPairStatus.%s is nil, want %#v", required.name, required.wantValue)
				continue
			}
			got = got.Elem()
		}
		if !reflect.DeepEqual(got.Interface(), required.wantValue) {
			t.Errorf("DefaultPairStatus.%s = %#v, want %#v", required.name, got.Interface(), required.wantValue)
		}
	}
}
