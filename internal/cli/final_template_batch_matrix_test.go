package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

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

func runFinalTemplateBatchCommand(t *testing.T, port *finalTemplateBatchPort, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalTemplates = workspaceauthoritycmd.NewTemplateService(port)
	code := command.RunContext(context.Background(), args)
	return code, stdout.String(), stderr.String()
}

func TestFinalTemplatePublicMutationMatrixReachesOneTypedDeltaPort(t *testing.T) {
	port := newFinalTemplateBatchPort(t)
	templateRef, _ := tobari.WorkspaceTemplateRef(port.template.ID)
	runtimeID := "01912345-6789-7abc-8def-0123456789b7"
	runtimeRevision := "sha256:" + strings.Repeat("b", 64)
	runtimeRef := tobari.RuntimeRevisionRef(runtimeID, runtimeRevision)
	tests := []struct {
		name string
		args []string
		kind tobari.WorkspaceTemplateChangeKind
	}{
		{"shell", []string{"config", "shell", "--id", templateRef, "--variable", "TERM", "--source", "literal", "--value", "xterm-256color", "--format", "json"}, tobari.WorkspaceTemplateChangeShell},
		{"Git inherit", []string{"config", "git", "--id", templateRef, "--source", "inherit", "--format", "json"}, tobari.WorkspaceTemplateChangeGit},
		{"Git literal", []string{"config", "git", "--id", templateRef, "--source", "literal", "--name", "Example User", "--email", "user@example.com", "--format", "json"}, tobari.WorkspaceTemplateChangeGit},
		{"Git default", []string{"config", "git", "--id", templateRef, "--source", "default", "--format", "json"}, tobari.WorkspaceTemplateChangeGit},
		{"AWS configure", []string{"config", "bootstrap", "aws", "--id", templateRef, "--profile", "engineering", "--format", "json"}, tobari.WorkspaceTemplateChangeBootstrapAWS},
		{"AWS refresh", []string{"config", "bootstrap", "aws", "--id", templateRef, "--refresh", "--format", "json"}, tobari.WorkspaceTemplateChangeBootstrapAWS},
		{"EKS configure", []string{"config", "bootstrap", "kubernetes", "eks", "--id", templateRef, "--kube-context", "platform", "--format", "json"}, tobari.WorkspaceTemplateChangeBootstrapEKS},
		{"EKS refresh", []string{"config", "bootstrap", "kubernetes", "eks", "--id", templateRef, "--refresh", "--format", "json"}, tobari.WorkspaceTemplateChangeBootstrapEKS},
		{"EKS remove", []string{"config", "bootstrap", "kubernetes", "eks", "--id", templateRef, "--remove", "--format", "json"}, tobari.WorkspaceTemplateChangeBootstrapEKS},
		{"AWS remove", []string{"config", "bootstrap", "aws", "--id", templateRef, "--remove", "--format", "json"}, tobari.WorkspaceTemplateChangeBootstrapAWS},
		{"Runtime", []string{"template", "runtime", "set", "--id", templateRef, "--runtime", runtimeRef, "--format", "json"}, tobari.WorkspaceTemplateChangeRuntime},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := len(port.changes)
			code, stdout, stderr := runFinalTemplateBatchCommand(t, port, test.args...)
			if code != ExitOK || len(port.changes) != before+1 || port.changes[len(port.changes)-1].Kind != test.kind {
				t.Fatalf("code=%d changes=%+v stdout=%q stderr=%q", code, port.changes, stdout, stderr)
			}
		})
	}
}

func TestFinalTemplateRuntimeSetRejectsRawWorkspaceTemplateIDAsInvalidReference(t *testing.T) {
	port := newFinalTemplateBatchPort(t)
	runtimeRef := tobari.RuntimeRevisionRef("01912345-6789-7abc-8def-0123456789b7", "sha256:"+strings.Repeat("b", 64))
	code, stdout, stderr := runFinalTemplateBatchCommand(t, port,
		"template", "runtime", "set", "--id", string(port.template.ID), "--runtime", runtimeRef, "--format", "json")
	if code == ExitOK || len(port.changes) != 0 {
		t.Fatalf("raw Template ID unexpectedly reached the mutation port: code=%d changes=%+v stdout=%q stderr=%q", code, port.changes, stdout, stderr)
	}
	if !strings.Contains(stdout+stderr, "invalid_template_ref") || strings.Contains(stdout+stderr, "undeclared_fault_contract") {
		t.Fatalf("raw Template ID fault=%q%q", stdout, stderr)
	}
}

func TestFinalTemplateMutationJSONAndHumanMatchNonProducerCatalogShape(t *testing.T) {
	port := newFinalTemplateBatchPort(t)
	templateRef, _ := tobari.WorkspaceTemplateRef(port.template.ID)
	code, stdout, stderr := runFinalTemplateBatchCommand(t, port,
		"config", "git", "--id", templateRef, "--source", "inherit", "--format", "json")
	if code != ExitOK {
		t.Fatalf("JSON code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatal(err)
	}
	if keys := sortedRawKeys(document); !reflect.DeepEqual(keys, []string{"schema_version", "template"}) {
		t.Fatalf("document keys=%v output=%s", keys, stdout)
	}
	var template map[string]json.RawMessage
	if err := json.Unmarshal(document["template"], &template); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"entry_slice_digest", "generation", "graphql_endpoints", "name", "policy_slice_digest", "revision", "runtime_id", "runtime_revision", "source_access", "workspace_template_id"}
	if keys := sortedRawKeys(template); !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("Template mutation keys=%v want=%v output=%s", keys, wantKeys, stdout)
	}
	for _, forbidden := range []string{"template_ref", "current_revision_ref", "manifest_id", "workspace_manifest_id"} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("Template mutation JSON exposes %q: %s", forbidden, stdout)
		}
	}

	code, stdout, stderr = runFinalTemplateBatchCommand(t, port,
		"config", "git", "--id", templateRef, "--source", "literal", "--name", "Example User", "--email", "user@example.com")
	if code != ExitOK {
		t.Fatalf("human code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	for _, want := range []string{"Template standard", "Generation", "Revision"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("human output lacks %q: %q", want, stdout)
		}
	}
	if strings.Contains(stdout, "Reference") || strings.Contains(stdout, templateRef) {
		t.Fatalf("mutation human output acts as a Template reference producer: %q", stdout)
	}
}

func TestFinalTemplateMutationCatalogMatrixHasOneExactTargetAndRuntimeOnlyParent(t *testing.T) {
	catalog := DefaultCatalog()
	paths := []string{
		"config shell", "config git", "config bootstrap aws", "config bootstrap kubernetes eks", "template runtime set",
	}
	for _, path := range paths {
		spec, found := catalog.Lookup(path)
		if !found || spec.Agent.Mutation == nil {
			t.Fatalf("%s mutation contract missing", path)
		}
		mutation := spec.Agent.Mutation
		if mutation.TargetIDInput != "--id" || mutation.TargetKind != tobari.WorkspaceTemplateReferenceKind {
			t.Fatalf("%s target contract=%+v", path, mutation)
		}
		if path == "template runtime set" {
			if mutation.ParentInput != "--runtime" || !reflect.DeepEqual(mutation.TargetInputs, []string{"--id", "--runtime"}) {
				t.Fatalf("Runtime mutation contract=%+v", mutation)
			}
		} else if mutation.ParentInput != "" || !reflect.DeepEqual(mutation.TargetInputs, []string{"--id"}) {
			t.Fatalf("%s acquired a parent=%+v", path, mutation)
		}
		if got := spec.ProducedRefs(); len(got) != 0 {
			t.Errorf("%s produced refs=%+v", path, got)
		}
	}

	git, _ := catalog.Lookup("config git")
	for _, input := range git.Agent.Inputs {
		if input.Name == "--source" && !reflect.DeepEqual(input.AllowedValues, []string{"default", "inherit", "literal"}) {
			t.Fatalf("Git source values=%v", input.AllowedValues)
		}
	}
}

func sortedRawKeys(values map[string]json.RawMessage) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
