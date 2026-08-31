package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/app/runtimecmd"
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalAuthorityReadFixture struct{}

type unavailableFinalProjectRoot struct{}

type invalidFinalProjectRoot struct{}

func (invalidFinalProjectRoot) CurrentDirectory(context.Context) (string, error) { return "/tmp", nil }
func (invalidFinalProjectRoot) ResolveProjectRoot(context.Context, string) (string, error) {
	return "relative/project", nil
}

func (unavailableFinalProjectRoot) CurrentDirectory(context.Context) (string, error) {
	return "", errors.New("current directory unavailable")
}

func (unavailableFinalProjectRoot) ResolveProjectRoot(context.Context, string) (string, error) {
	return "", errors.New("unexpected project root resolution")
}

type finalContextPlanErrorFixture struct{ err error }

func (f finalContextPlanErrorFixture) PlanContextSourceByReference(context.Context, string) (tobari.ContextActivationPlan, error) {
	return tobari.ContextActivationPlan{}, f.err
}

type finalTemplateCreateCapture struct {
	calls int
	name  string
	body  tobari.WorkspaceTemplateBody
}

type finalTemplateCopyCapture struct {
	calls int
	ref   string
	name  string
}

type finalTemplateDefaultCapture struct {
	calls int
	ref   string
}

func (f *finalTemplateDefaultCapture) SetDefaultWorkspaceTemplateByReference(_ context.Context, ref string) (tobari.WorkspaceTemplateSelectionResult, error) {
	f.calls++
	f.ref = ref
	id, err := tobari.ParseWorkspaceTemplateRef(ref)
	return tobari.WorkspaceTemplateSelectionResult{TemplateID: id, Selected: true}, err
}

type finalContextMutationCapture struct {
	createCalls int
	applyCalls  int
	templateRef string
	planRef     string
	snapshot    tobari.ContextAuthoritySnapshot
}

func (f *finalContextMutationCapture) CreateContextDraftByTemplateReference(_ context.Context, templateRef string) (tobari.ContextDraft, error) {
	f.createCalls++
	f.templateRef = templateRef
	templateID, err := tobari.ParseWorkspaceTemplateRef(templateRef)
	if err != nil {
		return tobari.ContextDraft{}, err
	}
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789b3")
	revision := tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64))
	return tobari.ContextDraft{
		Source:      tobari.ContextSource{SchemaVersion: tobari.ContextSourceSchemaVersion, ContextID: contextID, TemplateID: templateID},
		Observation: tobari.ResourceSourceObservation{Path: "/config/tobari/contexts/01912345-6789-7abc-8def-0123456789b3/context.yaml", State: tobari.ResourceSourceModified, SourceRevision: &revision},
	}, nil
}

func (f *finalContextMutationCapture) ApplyContextSourceByPlan(_ context.Context, planRef string) (tobari.ContextAuthoritySnapshot, bool, error) {
	f.applyCalls++
	f.planRef = planRef
	return f.snapshot.Clone(), true, nil
}

func (f *finalContextMutationCapture) ObserveContextSource(_ context.Context, binding tobari.ContextBinding) (tobari.ResourceSourceObservation, error) {
	revision, err := tobari.ContextSourceSemanticRevision(binding)
	if err != nil {
		return tobari.ResourceSourceObservation{}, err
	}
	return tobari.ResourceSourceObservation{
		Path: "/config/tobari/contexts/" + string(binding.ID) + "/context.yaml", State: tobari.ResourceSourceInSync,
		SourceRevision: &revision, ActiveRevision: &revision,
	}, nil
}

func (f *finalTemplateCopyCapture) CopyWorkspaceTemplateDraftByRevisionReference(_ context.Context, ref, name string) (tobari.WorkspaceTemplateDraft, error) {
	f.calls++
	f.ref, f.name = ref, name
	id := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789b2")
	body := finalAxisTemplateBody("/copied")
	revision, err := tobari.NewWorkspaceTemplateRevision(id, 1, body)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	value := revision.Revision
	return tobari.WorkspaceTemplateDraft{
		ID: id, Name: name, Body: body,
		Source: tobari.ResourceSourceObservation{Path: "/config/tobari/templates/01912345-6789-7abc-8def-0123456789b2/template.yaml", State: tobari.ResourceSourceModified, SourceRevision: &value},
	}, nil
}

func (f *finalTemplateCreateCapture) CreateWorkspaceTemplateDraft(_ context.Context, name string, body tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplateDraft, error) {
	f.calls++
	f.name, f.body = name, body.Clone()
	id := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789b1")
	revision, err := tobari.NewWorkspaceTemplateRevision(id, 1, body)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	value := revision.Revision
	return tobari.WorkspaceTemplateDraft{ID: id, Name: name, Body: body.Clone(), Source: tobari.ResourceSourceObservation{Path: "/config/tobari/templates/01912345-6789-7abc-8def-0123456789b1/template.yaml", State: tobari.ResourceSourceModified, SourceRevision: &value}}, nil
}

func finalTemplateCreateRuntime() *runtimeCatalogCLI {
	return &runtimeCatalogCLI{manifest: tobari.RuntimeManifest{
		SchemaVersion: tobari.RuntimeSchemaVersion, ID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Kind: tobari.RuntimeKindBuiltin,
		Revisions: []tobari.RuntimeRevision{{Ordinal: 1, Revision: "sha256:" + strings.Repeat("f", 64), Image: "tobari-runtime:test", CreatedAt: time.Unix(1, 0).UTC()}},
	}}
}

func (finalAuthorityReadFixture) ListWorkspaceTemplates(context.Context) ([]tobari.WorkspaceTemplate, error) {
	return []tobari.WorkspaceTemplate{}, nil
}
func (finalAuthorityReadFixture) DiscoverWorkspaceTemplate(context.Context, string) (tobari.WorkspaceTemplate, error) {
	return tobari.WorkspaceTemplate{}, tobari.ErrWorkspaceTemplateNotFound
}
func (finalAuthorityReadFixture) ListContextAuthority(context.Context) ([]tobari.ContextAuthoritySnapshot, error) {
	return []tobari.ContextAuthoritySnapshot{}, nil
}
func (finalAuthorityReadFixture) ReadContextAuthorityByReference(context.Context, string) (tobari.ContextAuthoritySnapshot, error) {
	return tobari.ContextAuthoritySnapshot{}, tobari.ErrContextBindingNotFound
}
func (finalAuthorityReadFixture) ListWorkspaceAuthority(context.Context) ([]tobari.ContextAuthoritySnapshot, error) {
	return []tobari.ContextAuthoritySnapshot{}, nil
}
func (finalAuthorityReadFixture) ReadWorkspaceAuthorityByReference(context.Context, string) (tobari.ContextAuthoritySnapshot, error) {
	return tobari.ContextAuthoritySnapshot{}, tobari.ErrWorkspaceBindingNotFound
}

type finalContextAxisReadFixture struct {
	finalAuthorityReadFixture
	snapshot tobari.ContextAuthoritySnapshot
}

func (f finalContextAxisReadFixture) ReadContextAuthorityByReference(context.Context, string) (tobari.ContextAuthoritySnapshot, error) {
	return f.snapshot.Clone(), nil
}

type finalAuthorityDeleteCounter struct{ calls int }

type finalAuthorityMissingFixture struct{ finalAuthorityReadFixture }

type finalInvalidReferencePorts struct{ finalAuthorityMissingFixture }

func (finalInvalidReferencePorts) PlanWorkspaceTemplateSourceByReference(context.Context, string) (tobari.WorkspaceTemplateChangePlan, error) {
	return tobari.WorkspaceTemplateChangePlan{}, errors.New("unexpected Template plan call")
}
func (finalInvalidReferencePorts) PlanWorkspaceTemplatePolicyMigrationByReference(context.Context, string) (tobari.WorkspaceTemplatePolicyMigrationPlan, error) {
	return tobari.WorkspaceTemplatePolicyMigrationPlan{}, errors.New("unexpected Template migration plan call")
}
func (finalInvalidReferencePorts) PlanContextSourceByReference(context.Context, string) (tobari.ContextActivationPlan, error) {
	return tobari.ContextActivationPlan{}, errors.New("unexpected Context plan call")
}

func (finalAuthorityMissingFixture) DeleteWorkspaceTemplateByReference(context.Context, string) (tobari.WorkspaceTemplateDeleteResult, error) {
	return tobari.WorkspaceTemplateDeleteResult{}, tobari.ErrWorkspaceTemplateNotFound
}
func (finalAuthorityMissingFixture) DeleteContextByReference(context.Context, string) (tobari.ContextDeleteResult, error) {
	return tobari.ContextDeleteResult{}, tobari.ErrContextBindingNotFound
}
func (finalAuthorityMissingFixture) DeleteWorkspaceByReference(context.Context, string, bool) (tobari.WorkspaceAuthorityDeleteResult, error) {
	return tobari.WorkspaceAuthorityDeleteResult{}, tobari.ErrWorkspaceBindingNotFound
}

func (f *finalAuthorityDeleteCounter) DeleteWorkspaceTemplateByReference(context.Context, string) (tobari.WorkspaceTemplateDeleteResult, error) {
	f.calls++
	return tobari.WorkspaceTemplateDeleteResult{}, nil
}
func (f *finalAuthorityDeleteCounter) DeleteContextByReference(context.Context, string) (tobari.ContextDeleteResult, error) {
	f.calls++
	return tobari.ContextDeleteResult{}, nil
}
func (f *finalAuthorityDeleteCounter) DeleteWorkspaceByReference(context.Context, string, bool) (tobari.WorkspaceAuthorityDeleteResult, error) {
	f.calls++
	return tobari.WorkspaceAuthorityDeleteResult{}, nil
}
func (f *finalAuthorityDeleteCounter) UpdateWorkspaceTemplateByReference(context.Context, string, tobari.WorkspaceTemplateChange) (tobari.WorkspaceTemplateRevisionPublication, error) {
	f.calls++
	return tobari.WorkspaceTemplateRevisionPublication{}, nil
}

type finalFirstEntryFixture struct {
	publication tobari.ContextEntryPublication
	calls       int
	session     tobari.WorkspaceSessionRequest
	err         error
}

func (f *finalFirstEntryFixture) EnterContextByReference(_ context.Context, _ string, session tobari.WorkspaceSessionRequest, _ io.Reader, _, _ io.Writer) (tobari.ContextEntryPublication, error) {
	f.calls++
	f.session = session
	return f.publication, f.err
}

func (f *finalFirstEntryFixture) EnterContextByReferenceAtRoot(ctx context.Context, ref, _ string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (tobari.ContextEntryPublication, error) {
	return f.EnterContextByReference(ctx, ref, session, in, out, errOut)
}

func TestFinalWorkspaceAuthorityCatalogOwnsExactReferenceGraph(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	for _, path := range []string{
		"template list", "template show", "template create", "template copy", "template migration plan", "template migration apply", "template plan", "template apply", "template default set", "template delete",
		"context list", "context show", "context apply", "context create", "context enter", "context delete",
		"workspace list", "workspace status", "workspace delete",
	} {
		if _, found := catalog.Lookup(path); !found {
			t.Fatalf("final command %q is absent", path)
		}
	}
	for _, retired := range []string{"manifest list", "manifest show", "manifest create", "manifest default set", "manifest delete", "manifest runtime set", "list", "delete", "config shell", "config git", "config bootstrap aws", "config bootstrap kubernetes eks", "template runtime set"} {
		if _, found := catalog.Lookup(retired); found {
			t.Fatalf("retired command %q remains reachable", retired)
		}
	}

	wantProduced := map[string][]ProducedRef{
		"context apply":            {{Kind: tobari.ContextReferenceKind, Field: "context_ref"}},
		"template apply":           {{Kind: tobari.WorkspaceTemplateReferenceKind, Field: "template_ref"}, {Kind: tobari.WorkspaceTemplateRevisionReferenceKind, Field: "current_revision_ref"}},
		"template migration plan":  {{Kind: tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind, Field: "plan_ref"}, {Kind: tobari.WorkspaceTemplateReferenceKind, Field: "template_ref"}},
		"template migration apply": {{Kind: tobari.WorkspaceTemplateReferenceKind, Field: "template_ref"}},
		"template plan":            {{Kind: tobari.WorkspaceTemplateChangePlanReferenceKind, Field: "plan_ref"}, {Kind: tobari.WorkspaceTemplateReferenceKind, Field: "template_ref"}, {Kind: tobari.ContextReferenceKind, Field: "contexts[].context_ref"}, {Kind: tobari.WorkspaceReferenceKind, Field: "contexts[].workspace_ref"}},
		"template create":          {{Kind: tobari.WorkspaceTemplateReferenceKind, Field: "template_ref"}},
		"template list":            {{Kind: tobari.WorkspaceTemplateReferenceKind, Field: "items[].template_ref"}},
		"template show":            {{Kind: tobari.WorkspaceTemplateReferenceKind, Field: "template_ref"}, {Kind: tobari.WorkspaceTemplateRevisionReferenceKind, Field: "current_revision_ref"}},
		"context list":             {{Kind: tobari.ContextReferenceKind, Field: "items[].context_ref"}},
		"context show":             {{Kind: tobari.ContextReferenceKind, Field: "context_ref"}},
		"workspace list":           {{Kind: tobari.WorkspaceReferenceKind, Field: "items[].workspace_ref"}},
		"workspace status":         {{Kind: tobari.WorkspaceReferenceKind, Field: "workspace_ref"}},
		"context enter":            {{Kind: tobari.WorkspaceReferenceKind, Field: "workspace_ref"}},
	}
	for path, want := range wantProduced {
		command, _ := catalog.Lookup(path)
		if got := command.ProducedRefs(); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s ProducedRefs() = %+v, want %+v", path, got, want)
		}
	}
	for _, path := range []string{"template apply", "template migration apply", "template default set", "template delete", "context show", "context apply", "context enter", "context delete", "workspace status", "workspace delete"} {
		command, _ := catalog.Lookup(path)
		if command.Role == RoleUtility || len(command.ConsumedRefs()) == 0 {
			t.Fatalf("%s role/consumed refs = %q %+v", path, command.Role, command.ConsumedRefs())
		}
	}
	apply, _ := catalog.Lookup("template apply")
	if got, want := apply.ConsumedRefs(), []ConsumedRef{{Kind: tobari.WorkspaceTemplateChangePlanReferenceKind, Argument: "--plan"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("template apply refs = %+v, want %+v", got, want)
	}
	if apply.Agent.Mutation == nil || apply.Agent.Mutation.TargetIDInput != "--plan" || apply.Agent.Mutation.ParentInput != "" {
		t.Fatalf("template apply mutation = %+v", apply.Agent.Mutation)
	}
	migrationApply, _ := catalog.Lookup("template migration apply")
	if got, want := migrationApply.ConsumedRefs(), []ConsumedRef{{Kind: tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind, Argument: "--plan"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("template migration apply refs = %+v, want %+v", got, want)
	}
	defaultSet, _ := catalog.Lookup("template default set")
	if got := defaultSet.Agent.Output.Fields[1].Name; got != "selected" {
		t.Fatalf("template default output state = %q", got)
	}
	templateDelete, _ := catalog.Lookup("template delete")
	if got := templateDelete.Agent.Output.Fields[1].Name; got != "deleted" {
		t.Fatalf("template delete output state = %q", got)
	}
	contextEnter, _ := catalog.Lookup("context enter")
	if !reflect.DeepEqual(contextEnter.Agent.Output.Formats, []OutputFormat{OutputFormatText, OutputFormatJSON}) || contextEnter.Agent.Output.JSONEnvelope != "entry" || contextEnter.Agent.Output.JSONSchemaVersion != 1 {
		t.Fatalf("context enter output must expose the final entry receipt without writing to child stdout: %+v", contextEnter.Agent.Output)
	}
}

func TestFinalContextPlanEmitsDeclaredLegacyReadFault(t *testing.T) {
	contextRef, err := tobari.ContextRef(tobari.ContextID("01912345-6789-7abc-8def-0123456789a2"))
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalContexts = workspaceauthoritycmd.NewContextService(finalContextPlanErrorFixture{err: fmt.Errorf("%w: synthetic legacy root", tobari.ErrPreReleaseLegacyAuthority)})
	if code := command.RunContext(context.Background(), []string{"context", "plan", "--id", contextRef}); code != ExitRejected {
		t.Fatalf("context plan code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "legacy_state_present") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("context plan fault=%q", stderr.String())
	}
}

func TestFinalAuthorityCRUDCatalogDeclaresApplicationFaultClassifications(t *testing.T) {
	tests := []struct {
		path   string
		code   string
		kind   fault.Kind
		phase  fault.Phase
		change fault.ChangeState
	}{
		{path: "template list", code: "invalid_template_list", kind: fault.KindContract, phase: fault.PhaseVerification, change: fault.ChangeUnknown},
		{path: "template show", code: "template_not_found", kind: fault.KindNotFound, phase: fault.PhaseObservation, change: fault.ChangeNotApplicable},
		{path: "template show", code: "invalid_template_name", kind: fault.KindInvalidInput, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "template show", code: "invalid_template", kind: fault.KindContract, phase: fault.PhaseVerification, change: fault.ChangeUnknown},
		{path: "template plan", code: "invalid_template_ref", kind: fault.KindInvalidInput, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "template migration plan", code: "invalid_template_ref", kind: fault.KindInvalidInput, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "template create", code: "invalid_template_create_result", kind: fault.KindContract, phase: fault.PhaseVerification, change: fault.ChangeUnknown},
		{path: "template create", code: "invalid_standard_template_body", kind: fault.KindContract, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "template apply", code: "resource_source_recovery_required", kind: fault.KindUnavailable, phase: fault.PhaseMutation, change: fault.ChangePartial},
		{path: "template migration apply", code: "resource_source_recovery_required", kind: fault.KindUnavailable, phase: fault.PhaseMutation, change: fault.ChangePartial},
		{path: "template delete", code: "template_not_found", kind: fault.KindNotFound, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "template delete", code: "invalid_template_delete_result", kind: fault.KindContract, phase: fault.PhaseVerification, change: fault.ChangeUnknown},
		{path: "context list", code: "invalid_context_list", kind: fault.KindContract, phase: fault.PhaseVerification, change: fault.ChangeUnknown},
		{path: "context show", code: "context_not_found", kind: fault.KindNotFound, phase: fault.PhaseObservation, change: fault.ChangeNotApplicable},
		{path: "context show", code: "invalid_context_ref", kind: fault.KindInvalidInput, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "context show", code: "invalid_context", kind: fault.KindContract, phase: fault.PhaseVerification, change: fault.ChangeUnknown},
		{path: "context plan", code: "invalid_context_ref", kind: fault.KindInvalidInput, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "context create", code: "invalid_context_create_result", kind: fault.KindContract, phase: fault.PhaseVerification, change: fault.ChangeUnknown},
		{path: "context enter", code: "invalid_context_ref", kind: fault.KindInvalidInput, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "context enter", code: "invalid_root", kind: fault.KindInvalidInput, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "context enter", code: "invalid_project_root_resolution", kind: fault.KindContract, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "context enter", code: "workspace_entry_cleanup_failed", kind: fault.KindUnavailable, phase: fault.PhaseMutation, change: fault.ChangePartial},
		{path: "context delete", code: "context_not_found", kind: fault.KindNotFound, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "context delete", code: "invalid_context_delete_result", kind: fault.KindContract, phase: fault.PhaseVerification, change: fault.ChangeUnknown},
		{path: "workspace list", code: "invalid_workspace_list", kind: fault.KindContract, phase: fault.PhaseVerification, change: fault.ChangeUnknown},
		{path: "workspace status", code: "workspace_not_found", kind: fault.KindNotFound, phase: fault.PhaseObservation, change: fault.ChangeNotApplicable},
		{path: "workspace status", code: "invalid_workspace_ref", kind: fault.KindInvalidInput, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "workspace status", code: "invalid_workspace", kind: fault.KindContract, phase: fault.PhaseVerification, change: fault.ChangeUnknown},
		{path: "workspace delete", code: "workspace_not_found", kind: fault.KindNotFound, phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{path: "workspace delete", code: "invalid_workspace_delete_result", kind: fault.KindContract, phase: fault.PhaseVerification, change: fault.ChangeUnknown},
	}
	catalog := DefaultCatalog()
	for _, test := range tests {
		t.Run(test.path+"/"+test.code, func(t *testing.T) {
			spec, found := catalog.Lookup(test.path)
			if !found {
				t.Fatalf("command %q is absent", test.path)
			}
			declared := commandErrorByCode(t, spec.Agent.Errors, test.code)
			if declared.Kind != test.kind || declared.Phase != test.phase || declared.ChangeState != test.change {
				t.Fatalf("%s %s = kind=%q phase=%q change=%q, want %q/%q/%q", test.path, test.code, declared.Kind, declared.Phase, declared.ChangeState, test.kind, test.phase, test.change)
			}
		})
	}
}

func TestFinalContextEnterInterruptedRecoveryPreservesExplicitTargetWorkflow(t *testing.T) {
	spec, found := DefaultCatalog().Lookup("context enter")
	if !found {
		t.Fatal("context enter is absent")
	}
	declared := commandErrorByCode(t, spec.Agent.Errors, "workspace_entry_interrupted")
	if len(declared.NextActions) != 1 || declared.NextActions[0].Command != "help context enter" {
		t.Fatalf("workspace entry interrupted recovery = %+v", declared.NextActions)
	}
}

func TestFinalAuthorityReadInvalidReferencesNeverCollapseToUndeclaredContract(t *testing.T) {
	fixture := finalInvalidReferencePorts{}
	tests := []struct {
		name string
		args []string
		wire func(*CLI)
	}{
		{name: "template plan", args: []string{"template", "plan", "--id", "garbage"}, wire: func(c *CLI) { c.finalTemplates = workspaceauthoritycmd.NewTemplateService(fixture) }},
		{name: "template migration plan", args: []string{"template", "migration", "plan", "--id", "garbage"}, wire: func(c *CLI) { c.finalTemplates = workspaceauthoritycmd.NewTemplateService(fixture) }},
		{name: "context show", args: []string{"context", "show", "--id", "garbage"}, wire: func(c *CLI) { c.finalContexts = workspaceauthoritycmd.NewContextService(fixture) }},
		{name: "context plan", args: []string{"context", "plan", "--id", "garbage"}, wire: func(c *CLI) { c.finalContexts = workspaceauthoritycmd.NewContextService(fixture) }},
		{name: "workspace status", args: []string{"workspace", "status", "--id", "garbage"}, wire: func(c *CLI) { c.finalWorkspaces = workspaceauthoritycmd.NewWorkspaceService(fixture) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
			test.wire(command)
			if code := command.RunContext(context.Background(), test.args); code != ExitUsage {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), "invalid_") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestFinalAuthorityInvalidSelectorsNeverCollapseToUndeclaredContract(t *testing.T) {
	fixture := finalInvalidReferencePorts{}
	tests := []struct {
		name string
		args []string
		wire func(*CLI)
		code string
	}{
		{name: "template show name", args: []string{"template", "show", "--name=-invalid"}, wire: func(c *CLI) { c.finalTemplates = workspaceauthoritycmd.NewTemplateService(fixture) }, code: "invalid_template_name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
			test.wire(command)
			if code := command.RunContext(context.Background(), test.args); code != ExitUsage {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.code) || strings.Contains(stderr.String(), "undeclared_fault_contract") {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestFinalContextEnterInvalidRootNeverCollapsesToUndeclaredContract(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalContexts = workspaceauthoritycmd.NewContextService(finalInvalidReferencePorts{})
	command.finalProjectRoot = unavailableFinalProjectRoot{}
	if code := command.RunContext(context.Background(), []string{"--error-format=json", "context", "enter", "--id=ctx1_01912345-6789-7abc-8def-0123456789a2"}); code != ExitUsage {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !json.Valid(stderr.Bytes()) || !strings.Contains(stderr.String(), "invalid_root") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestFinalContextEnterInvalidResolvedRootNeverCollapsesToUndeclaredContract(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalContexts = workspaceauthoritycmd.NewContextService(finalInvalidReferencePorts{})
	command.finalProjectRoot = invalidFinalProjectRoot{}
	if code := command.RunContext(context.Background(), []string{"--error-format=json", "context", "enter", "--id=ctx1_01912345-6789-7abc-8def-0123456789a2"}); code != ExitContract {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !json.Valid(stderr.Bytes()) || !strings.Contains(stderr.String(), "invalid_project_root_resolution") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestFinalAuthorityCRUDNotFoundFaultsNeverCollapseToUndeclaredContract(t *testing.T) {
	const (
		templateRef  = "wtpl1_01912345-6789-7abc-8def-0123456789a1"
		contextRef   = "ctx1_01912345-6789-7abc-8def-0123456789a2"
		workspaceRef = "wsp1_01912345-6789-7abc-8def-0123456789a3"
	)
	fixture := finalAuthorityMissingFixture{}
	tests := []struct {
		name string
		args []string
		code string
		wire func(*CLI)
	}{
		{name: "template show", args: []string{"template", "show", "--name", "missing"}, code: "template_not_found", wire: func(c *CLI) { c.finalTemplates = workspaceauthoritycmd.NewTemplateService(fixture) }},
		{name: "context show", args: []string{"context", "show", "--id", contextRef}, code: "context_not_found", wire: func(c *CLI) { c.finalContexts = workspaceauthoritycmd.NewContextService(fixture) }},
		{name: "workspace status", args: []string{"workspace", "status", "--id", workspaceRef}, code: "workspace_not_found", wire: func(c *CLI) { c.finalWorkspaces = workspaceauthoritycmd.NewWorkspaceService(fixture) }},
		{name: "template stale delete", args: []string{"template", "delete", "--id", templateRef, "--confirm=delete"}, code: "template_not_found", wire: func(c *CLI) { c.finalTemplates = workspaceauthoritycmd.NewTemplateService(fixture) }},
		{name: "context stale delete", args: []string{"context", "delete", "--id", contextRef, "--confirm=delete"}, code: "context_not_found", wire: func(c *CLI) { c.finalContexts = workspaceauthoritycmd.NewContextService(fixture) }},
		{name: "workspace stale delete", args: []string{"workspace", "delete", "--id", workspaceRef, "--confirm=delete"}, code: "workspace_not_found", wire: func(c *CLI) { c.finalWorkspaces = workspaceauthoritycmd.NewWorkspaceService(fixture) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
			test.wire(command)
			if code := command.RunContext(context.Background(), test.args); code != ExitNotFound {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.code) || strings.Contains(stderr.String(), "undeclared_fault_contract") {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestPublicCatalogDoesNotExposeLegacyExecutablePolicy(t *testing.T) {
	for _, command := range DefaultCatalog().PublicCommands() {
		encoded, err := json.Marshal(command)
		if err != nil {
			t.Fatalf("marshal %q: %v", command.Path, err)
		}
		contract := strings.ToLower(string(encoded))
		for _, forbidden := range []string{"--mode", "policy_mode", "advanced_policy", "tobari.rego", "tobari_test.rego"} {
			if strings.Contains(contract, forbidden) {
				t.Fatalf("public command %q exposes retired executable-policy surface %q: %s", command.Path, forbidden, encoded)
			}
		}
	}
}

func TestFinalTemplateCreateCanPublishReadAccessAndBoundedGraphQLEndpoint(t *testing.T) {
	const endpoint = "https://graphql.example.dev:8443/graphql"
	for _, sourceAccess := range []string{"read-only", "read-write"} {
		t.Run(sourceAccess, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			capture := &finalTemplateCreateCapture{}
			command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
			command.runtime = runtimecmd.New(finalTemplateCreateRuntime())
			command.finalTemplates = workspaceauthoritycmd.NewTemplateService(capture)
			args := []string{"template", "create", "--name", "custom-" + strings.ReplaceAll(sourceAccess, "-", ""), "--source-access", sourceAccess, "--graphql-endpoint", endpoint, "--format", "json"}
			if code := command.RunContext(context.Background(), args); code != ExitOK {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if capture.calls != 1 || capture.body.Boundary.SourceAccess != tobari.ManifestSourceAccess(sourceAccess) || capture.body.Policy.SemanticModules == nil || len(capture.body.Policy.SemanticModules.Protocols.HTTP.GraphQL.Endpoints) == 0 {
				t.Fatalf("calls=%d source=%q policy=%+v", capture.calls, capture.body.Boundary.SourceAccess, capture.body.Policy)
			}
			parsed, err := parseBoundedGraphQLEndpoint(endpoint)
			got := finalTemplateGraphQLEndpoints(capture.body.Policy)
			if err != nil || !reflect.DeepEqual(got[len(got)-1], parsed) {
				t.Fatalf("custom endpoint=%+v parsed=%+v err=%v", got, parsed, err)
			}
			var document struct {
				Template finalTemplateDraftProjection `json:"template"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
				t.Fatal(err)
			}
			wantTemplateRef, err := tobari.WorkspaceTemplateRef(tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789b1"))
			if err != nil {
				t.Fatal(err)
			}
			if document.Template.TemplateRef != wantTemplateRef || document.Template.SourceAccess != sourceAccess || len(document.Template.GraphQLEndpoints) == 0 {
				t.Fatalf("result=%+v", document.Template)
			}
		})
	}

	var stdout, stderr bytes.Buffer
	capture := &finalTemplateCreateCapture{}
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(finalTemplateCreateRuntime())
	command.finalTemplates = workspaceauthoritycmd.NewTemplateService(capture)
	if code := command.RunContext(context.Background(), []string{"template", "create", "--name", "invalid-endpoint", "--graphql-endpoint", "https://graphql.example.dev/graphql"}); code == ExitOK {
		t.Fatalf("invalid endpoint unexpectedly succeeded: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if capture.calls != 0 {
		t.Fatalf("invalid endpoint reached create port: calls=%d", capture.calls)
	}
}

func TestFinalTemplateCopyHandlerConsumesTheExactRevisionReference(t *testing.T) {
	sourceID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789b1")
	sourceRevision := tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))
	sourceRef, err := tobari.WorkspaceTemplateRevisionRef(sourceID, sourceRevision)
	if err != nil {
		t.Fatal(err)
	}
	capture := &finalTemplateCopyCapture{}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalTemplates = workspaceauthoritycmd.NewTemplateService(capture)
	if code := command.RunContext(context.Background(), []string{"template", "copy", "--from", sourceRef, "--name", "copied", "--format=json"}); code != ExitOK {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if capture.calls != 1 || capture.ref != sourceRef || capture.name != "copied" {
		t.Fatalf("calls=%d ref=%q name=%q", capture.calls, capture.ref, capture.name)
	}
	var document struct {
		Template finalTemplateDraftProjection `json:"template"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.Template.Name != "copied" || document.Template.Lifecycle != "draft" || document.Template.TemplateRef == "" {
		t.Fatalf("copy output=%+v", document.Template)
	}
}

func TestFinalTemplateDefaultHandlerConsumesTheExactTemplateReference(t *testing.T) {
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789b1")
	templateRef, err := tobari.WorkspaceTemplateRef(templateID)
	if err != nil {
		t.Fatal(err)
	}
	capture := &finalTemplateDefaultCapture{}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalTemplates = workspaceauthoritycmd.NewTemplateService(capture)
	if code := command.RunContext(context.Background(), []string{"template", "default", "set", "--id", templateRef, "--format=json"}); code != ExitOK {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if capture.calls != 1 || capture.ref != templateRef || !strings.Contains(stdout.String(), `"selected":true`) {
		t.Fatalf("calls=%d ref=%q stdout=%q", capture.calls, capture.ref, stdout.String())
	}
}

func TestFinalContextCreateAndApplyHandlersBindTheirExactPublicScopes(t *testing.T) {
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789b1")
	templateRef, err := tobari.WorkspaceTemplateRef(templateID)
	if err != nil {
		t.Fatal(err)
	}
	capture := &finalContextMutationCapture{}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalContexts = workspaceauthoritycmd.NewContextService(capture)
	if code := command.RunContext(context.Background(), []string{"context", "create", "--template", templateRef, "--format=json"}); code != ExitOK {
		t.Fatalf("create code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if capture.createCalls != 1 || capture.templateRef != templateRef || !strings.Contains(stdout.String(), `"lifecycle":"draft"`) {
		t.Fatalf("create calls=%d template=%q stdout=%q", capture.createCalls, capture.templateRef, stdout.String())
	}

	capture.snapshot, _, _, _, _ = finalDesiredActiveSnapshotFixture(t, false)
	contextID := capture.snapshot.Context.ID
	planRef := "ctxplan1_" + string(contextID) + "_" + strings.Repeat("c", 64)
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"context", "apply", "--plan", planRef, "--format=json"}); code != ExitOK {
		t.Fatalf("apply code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if capture.applyCalls != 1 || capture.planRef != planRef || !strings.Contains(stdout.String(), `"changed":true`) {
		t.Fatalf("apply calls=%d plan=%q stdout=%q", capture.applyCalls, capture.planRef, stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"context", "apply", "--plan", planRef}); code != ExitOK {
		t.Fatalf("human apply code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Template revision") {
		t.Fatalf("human apply mislabeled desired Template revision: %q", stdout.String())
	}
}

func TestParseBoundedGraphQLEndpointPreservesAcceptedURLContract(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want tobari.ManifestPolicyExactRule
	}{
		{
			name: "ordinary endpoint",
			raw:  "https://graphql.example.dev:8443/graphql",
			want: tobari.ManifestPolicyExactRule{Scheme: "https", Host: "graphql.example.dev", Port: 8443, Method: "POST", Path: "/graphql"},
		},
		{
			name: "escaped path",
			raw:  "https://graphql.example.dev:8443/graphql%2Fv1",
			want: tobari.ManifestPolicyExactRule{Scheme: "https", Host: "graphql.example.dev", Port: 8443, Method: "POST", Path: "/graphql%2Fv1"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseBoundedGraphQLEndpoint(test.raw)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("endpoint=%+v want=%+v", got, test.want)
			}
		})
	}
}

func TestParseBoundedGraphQLEndpointRejectsBeforeTemplateMutation(t *testing.T) {
	for _, raw := range []string{
		"http://graphql.example.dev:8443/graphql",
		"https://graphql.example.dev/graphql",
		"https://graphql.example.dev:8443/graphql?query=1",
		"https://user@graphql.example.dev:8443/graphql",
		"https://graphql.example.dev:8443/graphql#fragment",
		"https://graphql.example.dev:8443",
		"https://graphql.example.test:8443/graphql",
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := parseBoundedGraphQLEndpoint(raw); err == nil {
				t.Fatalf("parseBoundedGraphQLEndpoint(%q) unexpectedly succeeded", raw)
			}
		})
	}
}

func TestFinalTemplateCreateCatalogDeclaresExistingBoundedDimensions(t *testing.T) {
	spec, found := DefaultCatalog().Lookup("template create")
	if !found {
		t.Fatal("template create is absent")
	}
	wantArgs := "--name <name> [--source-access read-only|read-write] [--graphql-endpoint <https-url>] [--format text|json]"
	if spec.Args != wantArgs {
		t.Fatalf("Args=%q want=%q", spec.Args, wantArgs)
	}
	var sourceAccess, endpoint *CommandInput
	for index := range spec.Agent.Inputs {
		switch spec.Agent.Inputs[index].Name {
		case "--source-access":
			sourceAccess = &spec.Agent.Inputs[index]
		case "--graphql-endpoint":
			endpoint = &spec.Agent.Inputs[index]
		}
	}
	if sourceAccess == nil || !reflect.DeepEqual(sourceAccess.AllowedValues, []string{"read-only", "read-write"}) || sourceAccess.DefaultValue == nil || *sourceAccess.DefaultValue != "read-write" {
		t.Fatalf("source-access input=%+v", sourceAccess)
	}
	if endpoint == nil || endpoint.MinimumLength == nil || *endpoint.MinimumLength != 1 || endpoint.ReferenceKind != "" {
		t.Fatalf("GraphQL endpoint input=%+v", endpoint)
	}
	if spec.Agent.Mutation == nil || spec.Agent.Mutation.TargetKind != tobari.WorkspaceTemplateCatalogTargetKind || len(spec.Agent.Mutation.TargetInputs) != 0 || len(spec.ConsumedRefs()) != 0 || !reflect.DeepEqual(spec.ProducedRefs(), []ProducedRef{{Kind: tobari.WorkspaceTemplateReferenceKind, Field: "template_ref"}}) {
		t.Fatalf("create authority contract=%+v consumed=%+v produced=%+v", spec.Agent.Mutation, spec.ConsumedRefs(), spec.ProducedRefs())
	}
	fields := map[string]bool{}
	for _, field := range spec.Agent.Output.Fields {
		fields[field.Name] = true
	}
	if !fields["source_access"] || !fields["graphql_endpoints"] {
		t.Fatalf("create output fields=%v", fields)
	}
}

func TestFinalTemplateMutationOutputsDoNotProduceDiscoveryReferences(t *testing.T) {
	catalog := DefaultCatalog()
	for _, path := range []string{
		"template default set", "template delete",
	} {
		spec, found := catalog.Lookup(path)
		if !found {
			t.Fatalf("missing %q", path)
		}
		if got := spec.ProducedRefs(); len(got) != 0 {
			t.Errorf("%s ProducedRefs() = %+v, want no mutation-produced discovery references", path, got)
		}
	}
	create, found := catalog.Lookup("template create")
	if !found || !reflect.DeepEqual(create.ProducedRefs(), []ProducedRef{{Kind: tobari.WorkspaceTemplateReferenceKind, Field: "template_ref"}}) {
		t.Errorf("template create must return its confirmed opaque reference: found=%t refs=%+v", found, create.ProducedRefs())
	}
	for _, path := range []string{"template list", "template show"} {
		spec, found := catalog.Lookup(path)
		if !found || len(spec.ProducedRefs()) == 0 {
			t.Errorf("%s must remain an invocable Template reference producer: found=%t refs=%+v", path, found, spec.ProducedRefs())
		}
	}
}

func TestFinalDeletesRequireLiteralConfirmationBeforeAdapter(t *testing.T) {
	for _, path := range []string{"template delete", "context delete", "workspace delete"} {
		command, _ := DefaultCatalog().Lookup(path)
		var confirmation CommandInput
		found := false
		for _, input := range command.Agent.Inputs {
			if input.Name == "--confirm" {
				confirmation, found = input, true
				break
			}
		}
		if !found || !confirmation.Required || !reflect.DeepEqual(confirmation.AllowedValues, []string{"delete"}) || command.Effect != operation.EffectWrite {
			t.Fatalf("%s confirmation = %+v effect=%q", path, confirmation, command.Effect)
		}
	}
}

func TestFinalDeletesRejectMissingConfirmationBeforeAdapter(t *testing.T) {
	const (
		templateRef  = "wtpl1_01912345-6789-7abc-8def-0123456789a1"
		contextRef   = "ctx1_01912345-6789-7abc-8def-0123456789a2"
		workspaceRef = "wsp1_01912345-6789-7abc-8def-0123456789a3"
	)
	for _, test := range []struct {
		args []string
		wire func(*CLI, *finalAuthorityDeleteCounter)
	}{
		{args: []string{"template", "delete", "--id", templateRef}, wire: func(c *CLI, p *finalAuthorityDeleteCounter) {
			c.finalTemplates = workspaceauthoritycmd.NewTemplateService(p)
		}},
		{args: []string{"context", "delete", "--id", contextRef}, wire: func(c *CLI, p *finalAuthorityDeleteCounter) {
			c.finalContexts = workspaceauthoritycmd.NewContextService(p)
		}},
		{args: []string{"workspace", "delete", "--id", workspaceRef}, wire: func(c *CLI, p *finalAuthorityDeleteCounter) {
			c.finalWorkspaces = workspaceauthoritycmd.NewWorkspaceService(p)
		}},
	} {
		var out, errOut bytes.Buffer
		port := &finalAuthorityDeleteCounter{}
		command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
		test.wire(command, port)
		if code := command.RunContext(context.Background(), test.args); code == ExitOK {
			t.Fatalf("%v unexpectedly succeeded", test.args)
		}
		if port.calls != 0 || out.Len() != 0 {
			t.Fatalf("%v calls=%d stdout=%q", test.args, port.calls, out.String())
		}
	}
}

func TestFinalEmptyAuthorityListsEmitSchemaOneExplicitArrays(t *testing.T) {
	for _, test := range []struct {
		path []string
		want string
		wire func(*CLI)
	}{
		{path: []string{"template", "list", "--format=json"}, want: `{"schema_version":1,"templates":{"items":[]}}` + "\n", wire: func(c *CLI) { c.finalTemplates = workspaceauthoritycmd.NewTemplateService(finalAuthorityReadFixture{}) }},
		{path: []string{"context", "list", "--format=json"}, want: `{"contexts":{"items":[]},"schema_version":1}` + "\n", wire: func(c *CLI) { c.finalContexts = workspaceauthoritycmd.NewContextService(finalAuthorityReadFixture{}) }},
		{path: []string{"workspace", "list", "--format=json"}, want: `{"schema_version":1,"workspaces":{"items":[]}}` + "\n", wire: func(c *CLI) {
			c.finalWorkspaces = workspaceauthoritycmd.NewWorkspaceService(finalAuthorityReadFixture{})
		}},
	} {
		var out, errOut bytes.Buffer
		command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
		test.wire(command)
		if code := command.RunContext(context.Background(), test.path); code != ExitOK {
			t.Fatalf("%v code=%d stderr=%q", test.path, code, errOut.String())
		}
		if got := out.String(); got != test.want {
			t.Fatalf("%v output=%q, want %q", test.path, got, test.want)
		}
	}
}

func TestFinalAuthorityJSONOmitsAbsentLowerLifetimeAuthority(t *testing.T) {
	contextValue := finalContextProjection{Lifecycle: "active", ContextRef: "ctx1_01912345-6789-7abc-8def-0123456789a2", ContextID: "01912345-6789-7abc-8def-0123456789a2", TemplateID: "01912345-6789-7abc-8def-0123456789a1", TemplateName: "standard", DesiredTemplateGeneration: 1, DesiredTemplateRevision: "sha256:" + strings.Repeat("b", 64), DesiredTemplatePolicySliceDigest: "sha256:" + strings.Repeat("c", 64), CurrentPolicyMemoryRevision: "sha256:" + strings.Repeat("a", 64), SourcePath: "/tmp/tobari/contexts/01912345-6789-7abc-8def-0123456789a2/context.yaml", SourceState: string(tobari.ResourceSourceInSync), SourceRevision: stringPointer("sha256:" + strings.Repeat("d", 64)), ActiveRevision: "sha256:" + strings.Repeat("d", 64)}
	encoded, err := finalAuthorityOutput("context show", "context", contextValue, successFormatJSON, nil)
	if err != nil {
		t.Fatalf("context JSON error = %v", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	var projected map[string]any
	if err := json.Unmarshal(document["context"], &projected); err != nil {
		t.Fatal(err)
	}
	if _, exists := projected["workspace_id"]; exists {
		t.Fatalf("absent Workspace was serialized: %s", encoded)
	}
	wantContext := `{"context":{"context_ref":"ctx1_01912345-6789-7abc-8def-0123456789a2","context_id":"01912345-6789-7abc-8def-0123456789a2","workspace_template_id":"01912345-6789-7abc-8def-0123456789a1","template_name":"standard","desired_template_generation":1,"desired_template_revision":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","desired_template_policy_slice_digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","active_template_policy_slice_digest":null,"current_policy_memory_revision":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","active_policy_memory_revision":null,"applied_entry":null,"source_path":"/tmp/tobari/contexts/01912345-6789-7abc-8def-0123456789a2/context.yaml","source_state":"in_sync","source_revision":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","active_revision":"sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"},"schema_version":1}` + "\n"
	wantContext = strings.Replace(wantContext, `{"context":{`, `{"context":{"lifecycle":"active",`, 1)
	if got := string(encoded); got != wantContext {
		t.Fatalf("context JSON = %q, want %q", got, wantContext)
	}

	workspaceValue := finalWorkspaceProjection{WorkspaceRef: "wsp1_01912345-6789-7abc-8def-0123456789a3", WorkspaceID: "01912345-6789-7abc-8def-0123456789a3", ContextID: contextValue.ContextID, TemplateID: contextValue.TemplateID, TemplateName: contextValue.TemplateName, ProjectRoot: "/workspace/example", WorkspaceHome: "/workspace/home", Applied: false}
	encoded, err = finalAuthorityOutput("workspace status", "workspace", workspaceValue, successFormatJSON, nil)
	if err != nil {
		t.Fatalf("workspace JSON error = %v", err)
	}
	if strings.Contains(string(encoded), "applied_entry_revision") {
		t.Fatalf("absent applied entry was serialized: %s", encoded)
	}
	wantWorkspace := `{"schema_version":1,"workspace":{"workspace_ref":"wsp1_01912345-6789-7abc-8def-0123456789a3","workspace_id":"01912345-6789-7abc-8def-0123456789a3","context_id":"01912345-6789-7abc-8def-0123456789a2","workspace_template_id":"01912345-6789-7abc-8def-0123456789a1","template_name":"standard","project_root":"/workspace/example","workspace_home":"/workspace/home","applied":false}}` + "\n"
	if got := string(encoded); got != wantWorkspace {
		t.Fatalf("workspace JSON = %q, want %q", got, wantWorkspace)
	}
}

func TestFinalContextProjectionKeepsDesiredActiveAndAppliedAxesIndependent(t *testing.T) {
	snapshot, desired, activeTemplateDigest, activeMemoryRevision, applied := finalDesiredActiveSnapshotFixture(t, true)
	contextRef, err := tobari.ContextRef(snapshot.Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	sourceRevision, err := tobari.ContextSourceSemanticRevision(snapshot.Context)
	if err != nil {
		t.Fatal(err)
	}
	source := tobari.ResourceSourceObservation{Path: "/tmp/tobari/contexts/01912345-6789-7abc-8def-0123456789b2/context.yaml", State: tobari.ResourceSourceInSync, SourceRevision: &sourceRevision, ActiveRevision: &sourceRevision}
	projection, err := finalContextFromView(workspaceauthoritycmd.ContextView{Snapshot: snapshot, ContextRef: contextRef, Source: &source})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := finalAuthorityOutput("context show", "context", projection, successFormatJSON, nil)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		SchemaVersion int                        `json:"schema_version"`
		Context       map[string]json.RawMessage `json:"context"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{
		"active_policy_memory_revision", "active_revision", "active_template_policy_slice_digest", "applied_entry", "context_id", "context_ref",
		"current_policy_memory_revision", "desired_template_generation", "desired_template_policy_slice_digest", "desired_template_revision",
		"lifecycle", "source_path", "source_revision", "source_state", "template_name", "workspace_id", "workspace_template_id",
	}
	if got := sortedJSONKeys(document.Context); !reflect.DeepEqual(got, wantKeys) {
		t.Fatalf("context keys = %v, want %v; output=%s", got, wantKeys, encoded)
	}
	assertJSONFieldEqual(t, document.Context, "desired_template_generation", desired.Template.Current.Generation)
	assertJSONFieldEqual(t, document.Context, "desired_template_revision", desired.Template.Current.Revision)
	assertJSONFieldEqual(t, document.Context, "desired_template_policy_slice_digest", desired.Template.Current.Slices.PolicySliceDigest)
	assertJSONFieldEqual(t, document.Context, "active_template_policy_slice_digest", activeTemplateDigest)
	assertJSONFieldEqual(t, document.Context, "current_policy_memory_revision", desired.PolicyMemory.Revision)
	assertJSONFieldEqual(t, document.Context, "active_policy_memory_revision", activeMemoryRevision)
	assertJSONFieldEqual(t, document.Context, "applied_entry", applied)
}

func TestFinalContextProjectionEmitsNullForInactiveAxes(t *testing.T) {
	snapshot, _, _, _, _ := finalDesiredActiveSnapshotFixture(t, false)
	contextRef, err := tobari.ContextRef(snapshot.Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	sourceRevision, err := tobari.ContextSourceSemanticRevision(snapshot.Context)
	if err != nil {
		t.Fatal(err)
	}
	source := tobari.ResourceSourceObservation{Path: "/tmp/tobari/contexts/01912345-6789-7abc-8def-0123456789b2/context.yaml", State: tobari.ResourceSourceInSync, SourceRevision: &sourceRevision, ActiveRevision: &sourceRevision}
	projection, err := finalContextFromView(workspaceauthoritycmd.ContextView{Snapshot: snapshot, ContextRef: contextRef, Source: &source})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := finalAuthorityOutput("context show", "context", projection, successFormatJSON, nil)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Context map[string]json.RawMessage `json:"context"`
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"active_template_policy_slice_digest", "active_policy_memory_revision", "applied_entry"} {
		value, exists := document.Context[field]
		if !exists || string(value) != "null" {
			t.Errorf("inactive context field %s = %s exists=%t, want explicit null", field, value, exists)
		}
	}
	for _, retired := range []string{"template_policy_active", "policy_memory_active", "policy_memory_revision"} {
		if _, exists := document.Context[retired]; exists {
			t.Errorf("inactive context retained inference field %q: %s", retired, encoded)
		}
	}
}

func TestFinalContextHumanOutputNamesDesiredActiveAndAppliedAxes(t *testing.T) {
	snapshot, _, activeTemplateDigest, activeMemoryRevision, applied := finalDesiredActiveSnapshotFixture(t, true)
	contextRef, err := tobari.ContextRef(snapshot.Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
	command.finalContexts = workspaceauthoritycmd.NewContextService(finalContextAxisReadFixture{snapshot: snapshot})
	if code := command.RunContext(context.Background(), []string{"context", "show", "--id", contextRef}); code != ExitOK {
		t.Fatalf("context show code=%d stderr=%q", code, errOut.String())
	}
	for _, want := range []string{
		"Desired Template generation", fmt.Sprint(snapshot.Template.Current.Generation),
		"Desired Template revision", string(snapshot.Template.Current.Revision),
		"Desired Template policy", string(snapshot.Template.Current.Slices.PolicySliceDigest),
		"Active Template policy", string(activeTemplateDigest),
		"Current Policy Memory", string(snapshot.PolicyMemory.Revision),
		"Active Policy Memory", string(activeMemoryRevision),
		"Applied entry", string(applied.TemplateRevision), string(applied.EntrySliceDigest),
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("context show output missing %q: %q", want, out.String())
		}
	}
}

func TestBareStatusSchemaThreeOwnsFinalDefaultPairContract(t *testing.T) {
	spec, found := DefaultCatalog().Lookup("status")
	if !found {
		t.Fatal("bare status is absent")
	}
	if spec.Role != RoleDiscover || spec.Effect != operation.EffectRead || spec.Agent.Output.JSONSchemaVersion != 3 || spec.Agent.Output.JSONEnvelope != "status" {
		t.Errorf("bare status contract = role=%q effect=%q schema=%d envelope=%q", spec.Role, spec.Effect, spec.Agent.Output.JSONSchemaVersion, spec.Agent.Output.JSONEnvelope)
	}
	if strings.Contains(spec.Args, "--manifest") {
		t.Errorf("bare status retains predecessor selector: %q", spec.Args)
	}
	for _, input := range spec.Agent.Inputs {
		if input.Name == "--manifest" || input.ReferenceKind != "" {
			t.Errorf("bare status input = %+v, want no predecessor or reference input", input)
		}
	}
	wantFields := []string{
		"task", "authority_state", "project_root", "default_template_state", "template", "context", "workspace", "runtime", "cluster",
		"permissions", "services", "login_validity", "siblings", "next", "attention",
	}
	gotFields := make([]string, len(spec.Agent.Output.Fields))
	for index, field := range spec.Agent.Output.Fields {
		gotFields[index] = field.Name
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Errorf("bare status fields = %v, want %v", gotFields, wantFields)
	}
	for _, name := range []string{"template", "context"} {
		field, ok := findFinalOutputField(spec.Agent.Output.Fields, name)
		if !ok || !field.Nullable {
			t.Errorf("bare status field %q = %+v found=%t, want nullable", name, field, ok)
		}
	}
	workspace, ok := findFinalOutputField(spec.Agent.Output.Fields, "workspace")
	if !ok {
		t.Fatal("bare status workspace object is absent")
	}
	if field, ok := findFinalOutputField(workspace.Fields, "workspace_ref"); !ok || !field.Optional || field.Nullable {
		t.Errorf("bare status workspace_ref = %+v found=%t, want optional non-null opaque reference", field, ok)
	}
	if got, want := spec.ProducedRefs(), []ProducedRef{{Kind: tobari.WorkspaceReferenceKind, Field: "workspace.workspace_ref"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("bare status ProducedRefs() = %+v, want %+v", got, want)
	}
	if got := spec.ConsumedRefs(); len(got) != 0 {
		t.Errorf("bare status ConsumedRefs() = %+v, want none", got)
	}
}

func TestFinalAuthorityHelpAndCompletionComeFromAtomicCatalog(t *testing.T) {
	command := &CLI{catalog: DefaultCatalog()}
	records, err := command.planCompletion(context.Background(), 4, []string{"tobari", "template", "delete", "--"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"candidate:--id", "candidate:--confirm", "candidate:--format"} {
		if !containsFinalCompletion(completionRecordValues(records), want) {
			t.Fatalf("completion = %v, missing %q", completionRecordValues(records), want)
		}
	}

	for _, path := range []string{"template apply", "template delete", "context delete", "workspace delete"} {
		spec, found := command.catalog.Lookup(path)
		if !found {
			t.Fatalf("missing %q", path)
		}
		human := string(renderCommandHelp(spec))
		if !strings.Contains(human, spec.Usage()) || strings.Contains(human, "--manifest") {
			t.Fatalf("%s human help = %q", path, human)
		}
	}
}

func TestFinalContextEnterHelpAndInvocationPermitFirstEntrySettlement(t *testing.T) {
	const (
		templateID  tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789a1"
		contextID   tobari.ContextID           = "01912345-6789-7abc-8def-0123456789a2"
		workspaceID tobari.WorkspaceID         = "01912345-6789-7abc-8def-0123456789a3"
	)
	digest := func(value string) tobari.SemanticDigest {
		return tobari.SemanticDigest("sha256:" + strings.Repeat(value, 64))
	}
	body := tobari.WorkspaceTemplateBody{
		Boundary:      tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadOnly, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}}},
		Policy:        tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{{Scheme: "https", Host: "api.example.dev", Port: 443, Method: "GET", Path: "/items"}}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: string(digest("f")), Ordinal: 1, Image: "tobari-runtime:test"}}, SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: "standard", Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt := tobari.TemplatePolicyActivationReceipt{ContextID: contextID, TemplateID: templateID, PolicySliceDigest: revision.Slices.PolicySliceDigest}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: memory.Revision}
	activeMemory := memory.Clone()
	applied := tobari.WorkspaceAppliedEntry{ContextID: contextID, TemplateID: templateID, TemplateRevision: revision.Revision, EntrySliceDigest: revision.Slices.EntrySliceDigest, RuntimeID: tobari.StandardRuntimeID, RuntimeRevision: revision.Slices.RuntimeRevision, ResolvedSpec: digest("7"), ReconciledAt: time.Unix(1, 0).UTC()}
	snapshot := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}, Template: template, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: &activeMemory, ActivePolicyMemoryRef: &memoryReceipt,
		Workspace: &tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID, ProjectRoot: "/workspace/example", Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied}}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	port := &finalFirstEntryFixture{publication: tobari.ContextEntryPublication{Snapshot: snapshot, Outcome: tobari.WorkspaceSessionOutcome{ExitCode: 0, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{}}}}
	var out, errOut bytes.Buffer
	command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
	command.finalContexts = workspaceauthoritycmd.NewContextService(port)
	command.finalProjectRoot = firstUseIntegrationProjectRoot{root: "/workspace/example"}
	spec, _ := command.catalog.Lookup("context enter")
	if help := string(renderCommandHelp(spec)); strings.Contains(help, "already active") || strings.Contains(help, "cluster up") {
		t.Fatalf("first-entry help retained an external activation prerequisite: %s", help)
	}
	contextRef, _ := tobari.ContextRef(contextID)
	if code := command.RunContext(context.Background(), []string{"context", "enter", "--id", contextRef}); code != ExitOK || port.calls != 1 {
		t.Fatalf("first entry code=%d calls=%d stdout=%q stderr=%q", code, port.calls, out.String(), errOut.String())
	}
	directScript := `port=$1; printf "%s\n" "$TOBARI_CAPABILITIES_JSON" > /var/lib/tobari/host-probe`
	if code := command.RunContext(context.Background(), []string{"context", "enter", "--id", contextRef, "--", "/bin/bash", "-lc", directScript, "bash", "3000"}); code != ExitOK {
		t.Fatalf("direct first entry code=%d stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	wantArgv := []string{"/bin/bash", "-lc", directScript, "bash", "3000"}
	if !port.session.Direct() || !reflect.DeepEqual(port.session.Argv(), wantArgv) {
		t.Fatalf("direct session argv=%q", port.session.Argv())
	}
}

func TestFinalContextEnterEmitsDeclaredWorkspaceBusyFault(t *testing.T) {
	const contextRef = "ctx1_01912345-6789-7abc-8def-0123456789a2"
	port := &finalFirstEntryFixture{err: errors.Join(tobari.ErrWorkspaceEntryObservationUnavailable, tobari.ErrContextBindingProtected)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalContexts = workspaceauthoritycmd.NewContextService(port)
	command.finalProjectRoot = firstUseIntegrationProjectRoot{root: "/workspace/example"}
	if code := command.RunContext(context.Background(), []string{"context", "enter", "--id", contextRef}); code != ExitUnavailable {
		t.Fatalf("context enter exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "workspace_entry_busy") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("context enter busy fault=%q", stderr.String())
	}
}

func TestFinalContextEnterPreservesChildStatusAndReportsSecondaryCleanup(t *testing.T) {
	snapshot := finalCurrentContextEntrySnapshotFixture(t)
	contextRef, err := tobari.ContextRef(snapshot.Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	port := &finalFirstEntryFixture{publication: tobari.ContextEntryPublication{
		Snapshot: snapshot,
		Outcome: tobari.WorkspaceSessionOutcome{ExitCode: 29, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{
			tobari.WorkspaceCleanupPermissionChannel,
		}},
	}}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalContexts = workspaceauthoritycmd.NewContextService(port)
	command.finalProjectRoot = firstUseIntegrationProjectRoot{root: "/workspace/example"}
	if code := command.RunContext(context.Background(), []string{"context", "enter", "--id", contextRef, "--", "codex", "exec"}); code != 29 {
		t.Fatalf("child exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 || strings.Count(stderr.String(), "Tobari cleanup needs attention") != 1 || !strings.Contains(stderr.String(), "Next: tobari status") || strings.Contains(stderr.String(), "Command failed") {
		t.Fatalf("child boundary stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func finalCurrentContextEntrySnapshotFixture(t *testing.T) tobari.ContextAuthoritySnapshot {
	t.Helper()
	snapshot, _, _, _, _ := finalDesiredActiveSnapshotFixture(t, true)
	current := snapshot.Template.Current
	snapshot.ActiveTemplatePolicy.PolicySliceDigest = current.Slices.PolicySliceDigest
	activeMemory := snapshot.PolicyMemory.Clone()
	snapshot.ActivePolicyMemory = &activeMemory
	snapshot.ActivePolicyMemoryRef.Revision = activeMemory.Revision
	snapshot.Workspace.CreationDefaults = current.Slices.CreationDefaultsDigest
	snapshot.Workspace.LastSuccessfulEntry.TemplateRevision = current.Revision
	snapshot.Workspace.LastSuccessfulEntry.EntrySliceDigest = current.Slices.EntrySliceDigest
	snapshot.Workspace.LastSuccessfulEntry.RuntimeRevision = current.Slices.RuntimeRevision
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestRetiredGranularTemplateWritersNeverReachAdapter(t *testing.T) {
	const templateRef = "wtpl1_01912345-6789-7abc-8def-0123456789a1"
	for _, args := range [][]string{
		{"config", "git", "--id", templateRef, "--source", "literal"},
		{"config", "shell", "--id", templateRef},
		{"config", "bootstrap", "aws", "--id", templateRef},
		{"config", "bootstrap", "kubernetes", "eks", "--id", templateRef},
		{"template", "runtime", "set", "--id", templateRef},
	} {
		var out, errOut bytes.Buffer
		port := &finalAuthorityDeleteCounter{}
		command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
		command.finalTemplates = workspaceauthoritycmd.NewTemplateService(port)
		if code := command.RunContext(context.Background(), args); code == ExitOK {
			t.Fatalf("%v unexpectedly succeeded", args)
		}
		if port.calls != 0 || out.Len() != 0 || !strings.Contains(errOut.String(), "unknown_command") {
			t.Fatalf("%v calls=%d stdout=%q stderr=%q", args, port.calls, out.String(), errOut.String())
		}
	}
}

func containsFinalCompletion(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestFinalBootstrapCommandsAreNotCompetingTemplateAuthority(t *testing.T) {
	for _, path := range [][]string{{"config", "bootstrap", "aws", "--id", "wtpl1_01912345-6789-7abc-8def-0123456789a1"}, {"config", "bootstrap", "kubernetes", "eks", "--id", "wtpl1_01912345-6789-7abc-8def-0123456789a1"}} {
		var out, errOut bytes.Buffer
		port := &finalAuthorityDeleteCounter{}
		command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
		command.finalTemplates = workspaceauthoritycmd.NewTemplateService(port)
		if code := command.RunContext(context.Background(), path); code == ExitOK || !strings.Contains(errOut.String(), "unknown_command") {
			t.Fatalf("%v code=%d stderr=%q", path, code, errOut.String())
		}
		if out.Len() != 0 || port.calls != 0 {
			t.Fatalf("%v calls=%d wrote success output %q", path, port.calls, out.String())
		}
	}
}

func finalDesiredActiveSnapshotFixture(t *testing.T, active bool) (tobari.ContextAuthoritySnapshot, tobari.ContextAuthoritySnapshot, tobari.SemanticDigest, tobari.SemanticDigest, tobari.WorkspaceAppliedEntry) {
	t.Helper()
	const (
		templateID  tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789b1"
		contextID   tobari.ContextID           = "01912345-6789-7abc-8def-0123456789b2"
		workspaceID tobari.WorkspaceID         = "01912345-6789-7abc-8def-0123456789b3"
	)
	bodyA := finalAxisTemplateBody("/policy-a")
	revisionA, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, bodyA)
	if err != nil {
		t.Fatal(err)
	}
	bodyB := finalAxisTemplateBody("/policy-b")
	revisionB, changed, err := tobari.AdvanceWorkspaceTemplateRevision(revisionA, bodyB)
	if err != nil || !changed {
		t.Fatalf("advance Template: changed=%t err=%v", changed, err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: "standard", Current: revisionB, Retained: []tobari.WorkspaceTemplateRevision{revisionA.Clone(), revisionB.Clone()}}
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
		SourceCandidates:       []string{"pcy_abcdef0123456789abcdef0123456789"},
	})
	if err != nil {
		t.Fatal(err)
	}
	memoryB, changed, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{rule}, &memoryA)
	if err != nil || !changed {
		t.Fatalf("advance Policy Memory: changed=%t err=%v", changed, err)
	}
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}
	snapshot := tobari.ContextAuthoritySnapshot{Context: binding, Template: template, PolicyMemory: memoryB}
	applied := tobari.WorkspaceAppliedEntry{
		ContextID: contextID, TemplateID: templateID, TemplateRevision: revisionA.Revision, EntrySliceDigest: revisionA.Slices.EntrySliceDigest,
		RuntimeID: tobari.StandardRuntimeID, RuntimeRevision: revisionA.Slices.RuntimeRevision,
		ResolvedSpec: tobari.SemanticDigest("sha256:" + strings.Repeat("7", 64)), ReconciledAt: time.Unix(1, 0).UTC(),
	}
	if active {
		templateReceipt := tobari.TemplatePolicyActivationReceipt{ContextID: contextID, TemplateID: templateID, PolicySliceDigest: revisionA.Slices.PolicySliceDigest}
		activeMemory := memoryA.Clone()
		memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: memoryA.Revision}
		workspace := tobari.WorkspaceBinding{
			SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID, ProjectRoot: "/workspace/example",
			Home: "/workspace/home", CreationDefaults: revisionA.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied,
		}
		snapshot.ActiveTemplatePolicy = &templateReceipt
		snapshot.ActivePolicyMemory = &activeMemory
		snapshot.ActivePolicyMemoryRef = &memoryReceipt
		snapshot.Workspace = &workspace
	}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	return snapshot.Clone(), snapshot.Clone(), revisionA.Slices.PolicySliceDigest, memoryA.Revision, applied
}

func finalAxisTemplateBody(path string) tobari.WorkspaceTemplateBody {
	digest := tobari.SemanticDigest("sha256:" + strings.Repeat("f", 64))
	modules := tobari.EmptyWorkspaceTemplateSemanticModules()
	modules.Protocols.HTTP.Generic.Allow.Rules = append(modules.Protocols.HTTP.Generic.Allow.Rules, tobari.SemanticHTTPRule{
		SemanticRuleAuthority: tobari.SemanticRuleAuthority{Scheme: "https", Host: "api.example.dev", Port: 443},
		Method:                "GET",
		Path:                  path,
	})
	return tobari.WorkspaceTemplateBody{
		Boundary: tobari.WorkspaceTemplateBoundary{
			SourceAccess:       tobari.ManifestSourceAccessReadOnly,
			DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []tobari.ManifestPolicyAuthority{}},
			MethodPolicy:       tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
		},
		Policy: tobari.WorkspaceTemplatePolicyBody{
			AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled,
			SemanticModules: &modules, BaselineGrants: []tobari.ManifestPolicyExactRule{},
			BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{},
		},
		EntryDefaults:   tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: string(digest), Ordinal: 1, Image: "tobari-runtime:test"}},
		SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
}

func sortedJSONKeys(values map[string]json.RawMessage) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func assertJSONFieldEqual(t *testing.T, fields map[string]json.RawMessage, name string, want any) {
	t.Helper()
	raw, exists := fields[name]
	if !exists {
		t.Errorf("missing JSON field %q", name)
		return
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(raw, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wantJSON, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("JSON field %s = %#v, want %#v", name, gotValue, wantValue)
	}
}

func findFinalOutputField(fields []OutputField, name string) (OutputField, bool) {
	for _, field := range fields {
		if field.Name == name {
			return field, true
		}
	}
	return OutputField{}, false
}

func TestFinalAuthorityMutationRecoveryContractPreservesPreconditionClassification(t *testing.T) {
	declaredCount := 0
	for _, spec := range DefaultCatalog().Commands() {
		for _, declared := range spec.Agent.Errors {
			if declared.Code != "final_authority_mutation_recovery_required" {
				continue
			}
			declaredCount++
			if declared.Kind != fault.KindUnavailable || declared.Phase != fault.PhasePrecondition || declared.ChangeState != fault.ChangeNone {
				t.Errorf("%s final-authority recovery = %+v", spec.Path, declared)
			}
		}
	}
	if declaredCount == 0 {
		t.Fatal("Catalog has no final-authority mutation recovery declaration")
	}
}

func TestFinalTemplateApplyDeclaresPostPublicationVerificationFailure(t *testing.T) {
	spec, found := DefaultCatalog().Lookup("template apply")
	if !found {
		t.Fatal("Catalog lacks template apply")
	}
	for _, declared := range spec.Agent.Errors {
		if declared.Code == "invalid_template_apply_result" {
			if declared.Kind != fault.KindContract || declared.Phase != fault.PhaseVerification || declared.ChangeState != fault.ChangeUnknown {
				t.Fatalf("template apply result verification = %+v", declared)
			}
			return
		}
	}
	t.Fatal("template apply lacks invalid_template_apply_result")
}
