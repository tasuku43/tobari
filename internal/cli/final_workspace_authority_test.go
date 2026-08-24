package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalAuthorityReadFixture struct{}

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
}

func (f *finalFirstEntryFixture) EnterContextByReference(_ context.Context, _ string, _ tobari.WorkspaceSessionRequest, _ io.Reader, _, _ io.Writer) (tobari.ContextEntryPublication, error) {
	f.calls++
	return f.publication, nil
}

func TestFinalWorkspaceAuthorityCatalogOwnsExactReferenceGraph(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	for _, path := range []string{
		"template list", "template show", "template create", "template copy", "template default set", "template delete", "template runtime set",
		"context list", "context show", "context create", "context enter", "context delete",
		"workspace list", "workspace status", "workspace delete", "config shell", "config git", "config bootstrap aws", "config bootstrap kubernetes eks",
	} {
		if _, found := catalog.Lookup(path); !found {
			t.Fatalf("final command %q is absent", path)
		}
	}
	for _, retired := range []string{"manifest list", "manifest show", "manifest create", "manifest default set", "manifest delete", "manifest runtime set", "list", "delete"} {
		if _, found := catalog.Lookup(retired); found {
			t.Fatalf("retired command %q remains reachable", retired)
		}
	}

	wantProduced := map[string][]ProducedRef{
		"template list":    {{Kind: tobari.WorkspaceTemplateReferenceKind, Field: "items[].template_ref"}},
		"template show":    {{Kind: tobari.WorkspaceTemplateReferenceKind, Field: "template_ref"}, {Kind: tobari.WorkspaceTemplateRevisionReferenceKind, Field: "current_revision_ref"}},
		"context list":     {{Kind: tobari.ContextReferenceKind, Field: "items[].context_ref"}},
		"context show":     {{Kind: tobari.ContextReferenceKind, Field: "context_ref"}},
		"workspace list":   {{Kind: tobari.WorkspaceReferenceKind, Field: "items[].workspace_ref"}},
		"workspace status": {{Kind: tobari.WorkspaceReferenceKind, Field: "workspace_ref"}},
		"context enter":    {{Kind: tobari.WorkspaceReferenceKind, Field: "workspace_ref"}},
	}
	for path, want := range wantProduced {
		command, _ := catalog.Lookup(path)
		if got := command.ProducedRefs(); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s ProducedRefs() = %+v, want %+v", path, got, want)
		}
	}
	for _, path := range []string{"template default set", "template delete", "context show", "context enter", "context delete", "workspace status", "workspace delete", "config shell", "config git", "template runtime set"} {
		command, _ := catalog.Lookup(path)
		if command.Role == RoleUtility || len(command.ConsumedRefs()) == 0 {
			t.Fatalf("%s role/consumed refs = %q %+v", path, command.Role, command.ConsumedRefs())
		}
	}
	runtimeSet, _ := catalog.Lookup("template runtime set")
	if got, want := runtimeSet.ConsumedRefs(), []ConsumedRef{{Kind: tobari.WorkspaceTemplateReferenceKind, Argument: "--id"}, {Kind: tobari.RuntimeRevisionReferenceKind, Argument: "--runtime"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("template runtime set refs = %+v, want %+v", got, want)
	}
	if runtimeSet.Agent.Mutation == nil || runtimeSet.Agent.Mutation.TargetIDInput != "--id" || runtimeSet.Agent.Mutation.ParentInput != "--runtime" {
		t.Fatalf("template runtime set mutation = %+v", runtimeSet.Agent.Mutation)
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

func TestFinalTemplateMutationOutputsDoNotProduceDiscoveryReferences(t *testing.T) {
	catalog := DefaultCatalog()
	for _, path := range []string{
		"template create", "template copy", "template default set", "template delete", "template runtime set",
		"config shell", "config git", "config bootstrap aws", "config bootstrap kubernetes eks",
	} {
		spec, found := catalog.Lookup(path)
		if !found {
			t.Fatalf("missing %q", path)
		}
		if got := spec.ProducedRefs(); len(got) != 0 {
			t.Errorf("%s ProducedRefs() = %+v, want no mutation-produced discovery references", path, got)
		}
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
	contextValue := finalContextProjection{ContextRef: "ctx1_01912345-6789-7abc-8def-0123456789a2", ContextID: "01912345-6789-7abc-8def-0123456789a2", TemplateID: "01912345-6789-7abc-8def-0123456789a1", TemplateName: "standard", ProjectRoot: "/workspace/example", DesiredTemplateGeneration: 1, DesiredTemplateRevision: "sha256:" + strings.Repeat("b", 64), DesiredTemplatePolicySliceDigest: "sha256:" + strings.Repeat("c", 64), CurrentPolicyMemoryRevision: "sha256:" + strings.Repeat("a", 64)}
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
	wantContext := `{"context":{"context_ref":"ctx1_01912345-6789-7abc-8def-0123456789a2","context_id":"01912345-6789-7abc-8def-0123456789a2","workspace_template_id":"01912345-6789-7abc-8def-0123456789a1","template_name":"standard","project_root":"/workspace/example","desired_template_generation":1,"desired_template_revision":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","desired_template_policy_slice_digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","active_template_policy_slice_digest":null,"current_policy_memory_revision":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","active_policy_memory_revision":null,"applied_entry":null},"schema_version":1}` + "\n"
	if got := string(encoded); got != wantContext {
		t.Fatalf("context JSON = %q, want %q", got, wantContext)
	}

	workspaceValue := finalWorkspaceProjection{WorkspaceRef: "wsp1_01912345-6789-7abc-8def-0123456789a3", WorkspaceID: "01912345-6789-7abc-8def-0123456789a3", ContextID: contextValue.ContextID, TemplateID: contextValue.TemplateID, TemplateName: contextValue.TemplateName, ProjectRoot: contextValue.ProjectRoot, WorkspaceHome: "/workspace/home", Applied: false}
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
	projection, err := finalContextFrom(snapshot, contextRef)
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
		"active_policy_memory_revision", "active_template_policy_slice_digest", "applied_entry", "context_id", "context_ref",
		"current_policy_memory_revision", "desired_template_generation", "desired_template_policy_slice_digest", "desired_template_revision",
		"project_root", "template_name", "workspace_id", "workspace_template_id",
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
	projection, err := finalContextFrom(snapshot, contextRef)
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
		"authority_state", "project_root", "default_template_state", "workspace_template_id", "template_name",
		"desired_template_generation", "desired_template_revision", "desired_template_policy_slice_digest", "active_template_policy_slice_digest",
		"context_id", "current_policy_memory_revision", "active_policy_memory_revision", "workspace_id", "workspace_ref", "workspace_home", "applied_entry",
	}
	gotFields := make([]string, len(spec.Agent.Output.Fields))
	for index, field := range spec.Agent.Output.Fields {
		gotFields[index] = field.Name
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Errorf("bare status fields = %v, want %v", gotFields, wantFields)
	}
	for _, name := range []string{"active_template_policy_slice_digest", "active_policy_memory_revision", "workspace_id", "workspace_home", "applied_entry"} {
		field, ok := findFinalOutputField(spec.Agent.Output.Fields, name)
		if !ok || !field.Nullable {
			t.Errorf("bare status field %q = %+v found=%t, want nullable", name, field, ok)
		}
	}
	if field, ok := findFinalOutputField(spec.Agent.Output.Fields, "workspace_ref"); !ok || !field.Optional || field.Nullable {
		t.Errorf("bare status workspace_ref = %+v found=%t, want optional non-null opaque reference", field, ok)
	}
	if got, want := spec.ProducedRefs(), []ProducedRef{{Kind: tobari.WorkspaceReferenceKind, Field: "workspace_ref"}}; !reflect.DeepEqual(got, want) {
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

	for _, path := range []string{"template delete", "context delete", "workspace delete", "config shell", "template runtime set"} {
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
		Policy:        tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, Mode: tobari.ManifestPolicyModeGuided, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{{Scheme: "https", Host: "api.example.dev", Port: 443, Method: "GET", Path: "/items"}}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: string(digest("f")), Ordinal: 1, Image: tobari.OfficialRuntimeBase}}, SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
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
	snapshot := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, ProjectRoot: "/workspace/example", TemplateID: templateID}, Template: template, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: &activeMemory, ActivePolicyMemoryRef: &memoryReceipt,
		Workspace: &tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID, ProjectRoot: "/workspace/example", Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied}}
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	port := &finalFirstEntryFixture{publication: tobari.ContextEntryPublication{Snapshot: snapshot, Outcome: tobari.WorkspaceSessionOutcome{ExitCode: 0, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{}}}}
	var out, errOut bytes.Buffer
	command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
	command.finalContexts = workspaceauthoritycmd.NewContextService(port)
	spec, _ := command.catalog.Lookup("context enter")
	if help := string(renderCommandHelp(spec)); strings.Contains(help, "already active") || strings.Contains(help, "cluster up") {
		t.Fatalf("first-entry help retained an external activation prerequisite: %s", help)
	}
	contextRef, _ := tobari.ContextRef(contextID)
	if code := command.RunContext(context.Background(), []string{"context", "enter", "--id", contextRef}); code != ExitOK || port.calls != 1 {
		t.Fatalf("first entry code=%d calls=%d stdout=%q stderr=%q", code, port.calls, out.String(), errOut.String())
	}
}

func TestFinalConfigGitValidatesConditionalSourceDimensionsBeforeAdapter(t *testing.T) {
	const templateRef = "wtpl1_01912345-6789-7abc-8def-0123456789a1"
	for _, args := range [][]string{
		{"config", "git", "--id", templateRef, "--source", "literal"},
		{"config", "git", "--id", templateRef, "--source", "literal", "--name", "Example"},
		{"config", "git", "--id", templateRef, "--source", "inherit", "--name", "Example", "--email", "dev@example.com"},
		{"config", "git", "--id", templateRef, "--source", "default", "--name", "Example", "--email", "dev@example.com"},
	} {
		var out, errOut bytes.Buffer
		port := &finalAuthorityDeleteCounter{}
		command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
		command.finalTemplates = workspaceauthoritycmd.NewTemplateService(port)
		if code := command.RunContext(context.Background(), args); code == ExitOK {
			t.Fatalf("%v unexpectedly succeeded", args)
		}
		if port.calls != 0 || out.Len() != 0 {
			t.Fatalf("%v calls=%d stdout=%q stderr=%q", args, port.calls, out.String(), errOut.String())
		}
	}

	for _, source := range []string{"default", "inherit"} {
		var out, errOut bytes.Buffer
		port := &finalAuthorityDeleteCounter{}
		command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
		command.finalTemplates = workspaceauthoritycmd.NewTemplateService(port)
		_ = command.RunContext(context.Background(), []string{"config", "git", "--id", templateRef, "--source", source})
		if port.calls != 1 {
			t.Fatalf("source %q calls=%d stderr=%q", source, port.calls, errOut.String())
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

func TestFinalBootstrapRequiresOneExplicitActionBeforeMutation(t *testing.T) {
	for _, path := range [][]string{{"config", "bootstrap", "aws", "--id", "wtpl1_01912345-6789-7abc-8def-0123456789a1"}, {"config", "bootstrap", "kubernetes", "eks", "--id", "wtpl1_01912345-6789-7abc-8def-0123456789a1"}} {
		var out, errOut bytes.Buffer
		port := &finalAuthorityDeleteCounter{}
		command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
		command.finalTemplates = workspaceauthoritycmd.NewTemplateService(port)
		if code := command.RunContext(context.Background(), path); code == ExitOK || !strings.Contains(errOut.String(), "invalid_arguments") {
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
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, ProjectRoot: "/workspace/example", TemplateID: templateID}
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
			SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID, ProjectRoot: binding.ProjectRoot,
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
	return tobari.WorkspaceTemplateBody{
		Boundary: tobari.WorkspaceTemplateBoundary{
			SourceAccess:       tobari.ManifestSourceAccessReadOnly,
			DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}},
			MethodPolicy:       tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}},
		},
		Policy: tobari.WorkspaceTemplatePolicyBody{
			AgentProfile: tobari.DefaultProfile, Mode: tobari.ManifestPolicyModeGuided, NativeReadiness: tobari.ManifestNativeReadinessEnabled,
			BaselineGrants:    []tobari.ManifestPolicyExactRule{{Scheme: "https", Host: "api.example.dev", Port: 443, Method: "GET", Path: path}},
			BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{},
		},
		EntryDefaults:   tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: string(digest), Ordinal: 1, Image: tobari.OfficialRuntimeBase}},
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
