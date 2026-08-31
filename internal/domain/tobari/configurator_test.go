package tobari

import (
	"strings"
	"testing"
)

func TestConfiguratorDraftIdentityBindsPurposeRuntimeAndAuthority(t *testing.T) {
	body := templateBodyFixture("configurator")
	bootstrap, err := NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	first, err := ConfiguratorDraftID(bootstrap, ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	body.EntryDefaults.Runtime.Revision = "sha256:" + repeatHex("b")
	body.EntryDefaults.Runtime.Image = "tobari-runtime:other"
	otherRuntime, err := NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	other, err := ConfiguratorDraftID(otherRuntime, ConfiguratorAgentCodex)
	if err != nil {
		t.Fatal(err)
	}
	if first == other || first == mustConfiguratorDraftID(t, bootstrap, ConfiguratorAgentClaude) {
		t.Fatalf("draft identity omitted agent or Runtime: first=%q other=%q", first, other)
	}
}

func TestConfiguratorIsolationRequiresWholeManagedHomeAndRejectsAmbientAuthority(t *testing.T) {
	valid := DirectEgressConfiguratorIsolation()
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	unsafe := map[string]func(*ConfiguratorIsolation){
		"missing managed Home": func(v *ConfiguratorIsolation) { v.ManagedHomeReadWrite = false },
		"project":              func(v *ConfiguratorIsolation) { v.ProjectMounted = true },
		"host home":            func(v *ConfiguratorIsolation) { v.HostHomeMounted = true },
		"other Context":        func(v *ConfiguratorIsolation) { v.OtherContextMounted = true },
		"Docker socket":        func(v *ConfiguratorIsolation) { v.DockerSocketMounted = true },
		"authority":            func(v *ConfiguratorIsolation) { v.AuthorityMounted = true },
	}
	for name, mutate := range unsafe {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if candidate.Validate() == nil {
				t.Fatal("unsafe Configurator isolation accepted")
			}
		})
	}
}

func TestConfiguratorSeedRejectsExecutionRuntimeOutsideDeterministicModeRule(t *testing.T) {
	body := templateBodyFixture("configurator-runtime-mode")
	bootstrap, err := NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	managed := bootstrap
	managed.ExecutionRuntime = RuntimeBinding{RuntimeID: "018bcfe5-687b-7000-8000-000000000077", Name: "tools", Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 1, Image: "runtime:tools"}
	if managed.Validate() == nil {
		t.Fatal("bootstrap accepted a managed execution Runtime")
	}
	forgedStandard := bootstrap
	forgedStandard.ExecutionRuntime.Revision = "sha256:" + strings.Repeat("b", 64)
	forgedStandard.ExecutionRuntime.Image = "tobari-runtime:forged"
	if forgedStandard.Validate() == nil {
		t.Fatal("bootstrap accepted standard Runtime material different from its reviewed initial body")
	}
	if _, err := NewConfiguratorDraft(bootstrap, ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab"); err == nil {
		t.Fatal("bootstrap draft accepted no reserved adoption Context")
	}

	templateID := WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, _ := NewWorkspaceTemplateRevision(templateID, 1, body)
	template := WorkspaceTemplate{SchemaVersion: WorkspaceTemplateSchemaVersion, ID: templateID, Name: DefaultManifestName, Current: revision, Retained: []WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, _ := PublishPolicyMemory(contextID, []PolicyMemoryRule{}, nil)
	workspace := WorkspaceBinding{SchemaVersion: WorkspaceBindingSchemaVersion, ID: WorkspaceID("01912345-6789-7abc-8def-0123456789ad"), ContextID: contextID, ProjectRoot: "/workspace/example", Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest}
	snapshot := ContextAuthoritySnapshot{Context: ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}, Template: template, PolicyMemory: memory, Workspace: &workspace}
	evolve, err := NewEvolveConfiguratorSeed(workspace.ProjectRoot, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	evolve.ExecutionRuntime.Revision = "sha256:" + strings.Repeat("b", 64)
	evolve.ExecutionRuntime.Image = "runtime:other"
	if evolve.Validate() == nil {
		t.Fatal("Context evolve accepted a Runtime other than the selected exact binding")
	}
}

func TestConfiguratorSubmissionBindsFrozenRuntimeSourceAndAppliedRevision(t *testing.T) {
	body := templateBodyFixture("configurator-runtime")
	body.EntryDefaults.Runtime = RuntimeBinding{RuntimeID: "018bcfe5-687b-7000-8000-000000000077", Name: "project-tools", Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 1, Image: "runtime:old"}
	templateID := WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, _ := NewWorkspaceTemplateRevision(templateID, 1, body)
	template := WorkspaceTemplate{SchemaVersion: WorkspaceTemplateSchemaVersion, ID: templateID, Name: DefaultManifestName, Current: revision, Retained: []WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, _ := PublishPolicyMemory(contextID, []PolicyMemoryRule{}, nil)
	workspace := WorkspaceBinding{SchemaVersion: WorkspaceBindingSchemaVersion, ID: WorkspaceID("01912345-6789-7abc-8def-0123456789ad"), ContextID: contextID, ProjectRoot: "/workspace/example", Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest}
	snapshot := ContextAuthoritySnapshot{Context: ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}, Template: template, PolicyMemory: memory, Workspace: &workspace}
	seed, err := NewEvolveConfiguratorSeed(workspace.ProjectRoot, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := NewConfiguratorDraft(seed, ConfiguratorAgentCodex, templateID)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := NewConfiguratorSubmission(draft, body)
	if err != nil {
		t.Fatal(err)
	}
	source := ConfiguratorRuntimeSource{SchemaVersion: ConfiguratorRuntimeSourceSchemaVersion, RuntimeID: body.EntryDefaults.Runtime.RuntimeID, BaseRevision: SemanticDigest("sha256:" + strings.Repeat("a", 64)), FrozenRevision: SemanticDigest("sha256:" + strings.Repeat("b", 64)), Changed: true}
	submission, err = submission.WithRuntimeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	binding := body.EntryDefaults.Runtime
	binding.Revision, binding.Ordinal, binding.Image = string(source.FrozenRevision), 2, "runtime:new"
	applied, err := submission.WithAppliedRuntime(binding)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Body.EntryDefaults.Runtime != binding || applied.SourceRevision == submission.SourceRevision || applied.Revision == submission.Revision {
		t.Fatalf("applied submission did not bind built Runtime: before=%+v after=%+v", submission, applied)
	}
}

func TestTaskScopedAssistSeparatesInstallationRuntimeFromContextPolicy(t *testing.T) {
	body := templateBodyFixture("task-assist")
	templateID := WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, _ := NewWorkspaceTemplateRevision(templateID, 1, body)
	template := WorkspaceTemplate{SchemaVersion: WorkspaceTemplateSchemaVersion, ID: templateID, Name: DefaultManifestName, Current: revision, Retained: []WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, _ := PublishPolicyMemory(contextID, []PolicyMemoryRule{}, nil)
	snapshot := ContextAuthoritySnapshot{Context: ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}, Template: template, PolicyMemory: memory}

	runtimeSeed, err := NewRuntimeAssistConfiguratorSeed(snapshot.Template.Current.Body.EntryDefaults.Runtime, "018bcfe5-687b-7000-8000-000000000077", SemanticDigest("sha256:"+strings.Repeat("d", 64)))
	if err != nil {
		t.Fatal(err)
	}
	policySeed, err := NewPolicyAssistConfiguratorSeed(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if runtimeSeed.Task != ConfiguratorTaskRuntime || runtimeSeed.TargetRuntimeID == runtimeSeed.ExecutionRuntime.RuntimeID || runtimeSeed.ExecutionRuntime != body.EntryDefaults.Runtime {
		t.Fatalf("Runtime assist target/execution binding=%+v", runtimeSeed)
	}
	if policySeed.Task != ConfiguratorTaskPolicy || policySeed.TargetRuntimeID != "" || policySeed.ExecutionRuntime != body.EntryDefaults.Runtime || policySeed.Evolution.PolicyMemory.Revision != memory.Revision {
		t.Fatalf("policy assist authority binding=%+v", policySeed)
	}
	if mustConfiguratorDraftID(t, runtimeSeed, ConfiguratorAgentCodex) == mustConfiguratorDraftID(t, policySeed, ConfiguratorAgentCodex) {
		t.Fatal("task-scoped draft identity omitted its task")
	}
	nextRuntimeSource := runtimeSeed
	nextRuntimeSource.TargetRuntimeRevision = SemanticDigest("sha256:" + strings.Repeat("e", 64))
	if mustConfiguratorDraftID(t, runtimeSeed, ConfiguratorAgentCodex) == mustConfiguratorDraftID(t, nextRuntimeSource, ConfiguratorAgentCodex) {
		t.Fatal("Runtime task draft identity omitted the target editable-source generation")
	}
	invalid := policySeed
	invalid.Evolution.PolicyMemory = nil
	if invalid.Validate() == nil {
		t.Fatal("policy assist accepted absent exact Policy Memory")
	}
	invalid = runtimeSeed
	invalid.ExecutionRuntime.Revision = "sha256:" + strings.Repeat("b", 64)
	if invalid.Validate() != nil {
		t.Fatal("Runtime assist incorrectly coupled its standard execution Runtime to Context material")
	}
	invalid = runtimeSeed
	invalid.ProjectRoot = "/workspace/example"
	if invalid.Validate() == nil {
		t.Fatal("Runtime assist accepted an ambient Project root")
	}
	invalid = runtimeSeed
	invalid.Evolution = policySeed.Evolution
	if invalid.Validate() == nil {
		t.Fatal("Runtime assist accepted Context authority")
	}
}

func mustConfiguratorDraftID(t *testing.T, seed ConfiguratorSeed, agent ConfiguratorAgent) string {
	t.Helper()
	id, err := ConfiguratorDraftID(seed, agent)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func repeatHex(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
