package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestFinalTemplatePlanJSONOmitsPrivatePlanningFields(t *testing.T) {
	port := newFinalTemplateBatchPort(t)
	templateRef, err := tobari.WorkspaceTemplateRef(port.template.ID)
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runFinalTemplateBatchCommand(t, port, "template", "plan", "--id", templateRef, "--format=json")
	if code != ExitOK {
		t.Fatalf("template plan code=%d stderr=%q", code, stderr)
	}
	var document struct {
		Plan map[string]any `json:"template_change_plan"`
	}
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatal(err)
	}
	if _, exists := document.Plan["schema_version"]; exists {
		t.Fatalf("Template plan exposed its private validation version: %s", stdout)
	}
	projection := finalTemplateChangePlanFrom(tobari.WorkspaceTemplateChangePlan{Contexts: []tobari.WorkspaceTemplateChangeContext{{
		ContextRef: "ctx1_01912345-6789-7abc-8def-0123456789a2", WorkspaceRunning: true,
	}}})
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("schema_version")) || bytes.Contains(encoded, []byte("workspace_running")) {
		t.Fatalf("Template plan exposed private planning fields: %s", encoded)
	}
}

type finalTemplateBatchPort struct {
	mu       sync.Mutex
	template tobari.WorkspaceTemplate
	changes  []tobari.WorkspaceTemplateChange
	aws      tobari.ManifestBootstrapSnapshot
	eks      tobari.ManifestBootstrapSnapshot
}

func newFinalTemplateBatchPort(t *testing.T) *finalTemplateBatchPort {
	t.Helper()
	const id tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789a1"
	revision, err := tobari.NewWorkspaceTemplateRevision(id, 1, finalAxisTemplateBody("/items"))
	if err != nil {
		t.Fatal(err)
	}
	bootstrap := newContextCreateBootstrapFixture(t, true)
	return &finalTemplateBatchPort{template: tobari.WorkspaceTemplate{
		SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: id, Name: "standard",
		Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()},
	}, aws: bootstrap.aws.Clone(), eks: bootstrap.eks.Clone()}
}

func (p *finalTemplateBatchPort) UpdateWorkspaceTemplateByReference(
	_ context.Context,
	ref string,
	change tobari.WorkspaceTemplateChange,
) (tobari.WorkspaceTemplateRevisionPublication, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if id, err := tobari.ParseWorkspaceTemplateRef(ref); err != nil || id != p.template.ID {
		if err != nil {
			return tobari.WorkspaceTemplateRevisionPublication{}, err
		}
		return tobari.WorkspaceTemplateRevisionPublication{}, tobari.ErrWorkspaceTemplateNotFound
	}
	previous := p.template.Current.Clone()
	var resolved *tobari.RuntimeBinding
	if change.Kind == tobari.WorkspaceTemplateChangeRuntime {
		id, revision, err := tobari.ParseRuntimeRevisionRef(change.RuntimeRevisionRef)
		if err != nil {
			return tobari.WorkspaceTemplateRevisionPublication{}, err
		}
		value := tobari.RuntimeBinding{RuntimeID: id, Name: "managed", Revision: revision, Ordinal: 3, Image: "tobari-runtime-managed:bbbbbbbbbbbb"}
		resolved = &value
	}
	nextBody, err := tobari.ApplyWorkspaceTemplateChange(previous.Body, change, resolved)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	next, changed, err := tobari.AdvanceWorkspaceTemplateRevision(previous, nextBody)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	if changed {
		p.template.Current = next.Clone()
		p.template.Retained = append(p.template.Retained, next.Clone())
	}
	p.changes = append(p.changes, change.Clone())
	return tobari.WorkspaceTemplateRevisionPublication{
		Template: p.template.Clone(), Previous: previous, Current: next,
		ResolvedRuntime: resolved, Changed: changed,
	}, nil
}

func (p *finalTemplateBatchPort) UpdateWorkspaceTemplateBootstrapByReference(
	ctx context.Context,
	ref string,
	request tobari.WorkspaceTemplateBootstrapRequest,
) (tobari.WorkspaceTemplateRevisionPublication, tobari.WorkspaceTemplateChange, error) {
	change := tobari.WorkspaceTemplateChange{Kind: request.Kind}
	if request.Action != tobari.WorkspaceTemplateBootstrapRemove {
		switch request.Kind {
		case tobari.WorkspaceTemplateChangeBootstrapAWS:
			value := p.aws.AWS.Clone()
			change.AWS = &value
		case tobari.WorkspaceTemplateChangeBootstrapEKS:
			value := *p.eks.EKS
			change.EKS = &value
		}
	}
	publication, err := p.UpdateWorkspaceTemplateByReference(ctx, ref, change)
	return publication, change, err
}

func (p *finalTemplateBatchPort) ApplyWorkspaceTemplateSourceByReference(_ context.Context, ref string) (tobari.WorkspaceTemplateRevisionPublication, error) {
	id, err := tobari.ParseWorkspaceTemplateChangePlanRef(ref)
	if err != nil || id != p.template.ID {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	current := p.template.Current.Clone()
	return tobari.WorkspaceTemplateRevisionPublication{Template: p.template.Clone(), Previous: current, Current: current, Changed: false}, nil
}

func (p *finalTemplateBatchPort) PlanWorkspaceTemplateSourceByReference(_ context.Context, ref string) (tobari.WorkspaceTemplateChangePlan, error) {
	id, err := tobari.ParseWorkspaceTemplateRef(ref)
	if err != nil || id != p.template.ID {
		return tobari.WorkspaceTemplateChangePlan{}, err
	}
	source, err := tobari.NewWorkspaceTemplateSource(p.template)
	if err != nil {
		return tobari.WorkspaceTemplateChangePlan{}, err
	}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection([]tobari.WorkspaceTemplate{p.template.Clone()}, []tobari.WorkspaceAuthorityContextRecord{}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil)
	if err != nil {
		return tobari.WorkspaceTemplateChangePlan{}, err
	}
	return tobari.NewWorkspaceTemplateChangePlan(collection, p.template.ID, source, p.template.Current.Body.EntryDefaults.Runtime, nil, strings.Repeat("a", 64))
}

func (p *finalTemplateBatchPort) ObserveWorkspaceTemplateSource(context.Context, tobari.WorkspaceTemplate) (tobari.ResourceSourceObservation, error) {
	revision := p.template.Current.Revision
	return tobari.ResourceSourceObservation{Path: "/tmp/tobari/templates/01912345-6789-7abc-8def-0123456789a1/template.yaml", State: tobari.ResourceSourceInSync, SourceRevision: &revision, ActiveRevision: &revision}, nil
}

func runFinalTemplateBatchCommand(t *testing.T, port *finalTemplateBatchPort, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalTemplates = workspaceauthoritycmd.NewTemplateService(port)
	code := command.RunContext(context.Background(), args)
	return code, stdout.String(), stderr.String()
}

func TestFinalTemplateGranularMutationCommandsAreRetired(t *testing.T) {
	port := newFinalTemplateBatchPort(t)
	templateRef, _ := tobari.WorkspaceTemplateRef(port.template.ID)
	tests := [][]string{
		{"config", "shell", "--id", templateRef}, {"config", "git", "--id", templateRef},
		{"config", "bootstrap", "aws", "--id", templateRef}, {"config", "bootstrap", "kubernetes", "eks", "--id", templateRef},
		{"template", "runtime", "set", "--id", templateRef},
	}
	for _, args := range tests {
		code, stdout, stderr := runFinalTemplateBatchCommand(t, port, args...)
		if code == ExitOK || len(port.changes) != 0 || !strings.Contains(stdout+stderr, "unknown_command") {
			t.Fatalf("retired command %v = code %d changes=%+v output=%q", args, code, port.changes, stdout+stderr)
		}
	}
}

func TestFinalTemplateApplyRejectsRawWorkspaceTemplateIDAsInvalidReference(t *testing.T) {
	port := newFinalTemplateBatchPort(t)
	code, stdout, stderr := runFinalTemplateBatchCommand(t, port,
		"template", "apply", "--plan", string(port.template.ID), "--format", "json")
	if code == ExitOK || len(port.changes) != 0 {
		t.Fatalf("raw Template ID unexpectedly reached the mutation port: code=%d changes=%+v stdout=%q stderr=%q", code, port.changes, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "invalid_template_change_plan_ref") || strings.Contains(stdout+stderr, "undeclared_fault_contract") {
		t.Fatalf("raw Template ID fault=%q%q", stdout, stderr)
	}
}

func TestFinalTemplateApplyPublishesReferencesAndChangedState(t *testing.T) {
	port := newFinalTemplateBatchPort(t)
	templateRef, _ := tobari.WorkspaceTemplateRef(port.template.ID)
	plan, err := port.PlanWorkspaceTemplateSourceByReference(context.Background(), templateRef)
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runFinalTemplateBatchCommand(t, port,
		"template", "apply", "--plan", plan.PlanRef, "--format", "json")
	if code != ExitOK {
		t.Fatalf("JSON code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, want := range []string{"\"template_ref\"", "\"current_revision_ref\"", "\"changed\":false"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("Apply output lacks %q: %q", want, stdout)
		}
	}
}

func TestFinalTemplateApplyIsTheOnlyGeneralTemplateSemanticWriter(t *testing.T) {
	catalog := DefaultCatalog()
	paths := []string{
		"config shell", "config git", "config bootstrap aws", "config bootstrap kubernetes eks", "template runtime set",
	}
	for _, path := range paths {
		if _, found := catalog.Lookup(path); found {
			t.Fatalf("competing semantic writer %q remains public", path)
		}
	}
	apply, found := catalog.Lookup("template apply")
	if !found || apply.Agent.Mutation == nil || apply.Agent.Mutation.TargetIDInput != "--plan" || !reflect.DeepEqual(apply.Agent.Mutation.TargetInputs, []string{"--plan"}) {
		t.Fatalf("Template Apply contract = %+v, found=%t", apply.Agent.Mutation, found)
	}
}
