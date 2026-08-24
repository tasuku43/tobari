package tobari

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

const (
	testTemplateAuthorityID  WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789a1"
	testContextAuthorityID   ContextID           = "01912345-6789-7abc-8def-0123456789a2"
	testWorkspaceAuthorityID WorkspaceID         = "01912345-6789-7abc-8def-0123456789a3"
)

func authorityDigest(character string) SemanticDigest {
	return SemanticDigest("sha256:" + strings.Repeat(character, 64))
}

func templateBodyFixture(policyPath string) WorkspaceTemplateBody {
	return WorkspaceTemplateBody{
		Boundary: WorkspaceTemplateBoundary{
			SourceAccess: ManifestSourceAccessReadOnly,
			DestinationCeiling: ManifestPolicyDestinationCeiling{
				Mode: "exact", Authorities: []ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}},
			},
			MethodPolicy: ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{{Method: "GET", Decision: ManifestMethodAllow}}},
		},
		Policy: WorkspaceTemplatePolicyBody{
			AgentProfile: DefaultProfile, Mode: ManifestPolicyModeGuided, NativeReadiness: ManifestNativeReadinessEnabled,
			BaselineGrants:    []ManifestPolicyExactRule{{Scheme: "https", Host: "api.example.dev", Port: 443, Method: "GET", Path: "/" + policyPath}},
			BaselineTemplates: []ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []ManifestPolicyMCPRule{},
			BaselineDenies: []ManifestPolicyExactRule{}, GraphQLEndpoints: []ManifestPolicyExactRule{}, MCPEndpoints: []ManifestPolicyExactRule{},
		},
		EntryDefaults: WorkspaceTemplateEntryDefaults{Runtime: RuntimeBinding{
			RuntimeID: StandardRuntimeID, Name: StandardRuntimeName, Revision: string(authorityDigest("f")), Ordinal: 1, Image: OfficialRuntimeBase,
		}},
		SessionDefaults:  WorkspaceTemplateSessionDefaults{ShellEnvironment: []ManifestShellEnvironmentSetting{}},
		CreationDefaults: WorkspaceTemplateCreationDefaults{},
	}
}

func TestAuthorityIDsIssueAndRejectCrossKindReferences(t *testing.T) {
	now := time.UnixMilli(1_725_000_000_000).UTC()
	for name, issue := range map[string]func(time.Time, *bytes.Reader) (string, error){
		"template": func(at time.Time, source *bytes.Reader) (string, error) {
			id, err := IssueWorkspaceTemplateID(at, source)
			return string(id), err
		},
		"context": func(at time.Time, source *bytes.Reader) (string, error) {
			id, err := IssueContextID(at, source)
			return string(id), err
		},
		"workspace": func(at time.Time, source *bytes.Reader) (string, error) {
			id, err := IssueWorkspaceID(at, source)
			return string(id), err
		},
	} {
		t.Run(name, func(t *testing.T) {
			id, err := issue(now, bytes.NewReader(make([]byte, 10)))
			if err != nil || !contextIDPattern.MatchString(id) {
				t.Fatalf("issued ID = %q, %v", id, err)
			}
		})
	}

	templateRef, err := WorkspaceTemplateRef(testTemplateAuthorityID)
	if err != nil {
		t.Fatal(err)
	}
	contextRef, err := ContextRef(testContextAuthorityID)
	if err != nil {
		t.Fatal(err)
	}
	workspaceRef, err := WorkspaceRef(testWorkspaceAuthorityID)
	if err != nil {
		t.Fatal(err)
	}
	revisionRef, err := WorkspaceTemplateRevisionRef(testTemplateAuthorityID, authorityDigest("1"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ParseWorkspaceTemplateRef(templateRef); err != nil || got != testTemplateAuthorityID {
		t.Fatalf("template round trip = %q, %v", got, err)
	}
	if got, err := ParseContextRef(contextRef); err != nil || got != testContextAuthorityID {
		t.Fatalf("context round trip = %q, %v", got, err)
	}
	if got, err := ParseWorkspaceRef(workspaceRef); err != nil || got != testWorkspaceAuthorityID {
		t.Fatalf("workspace round trip = %q, %v", got, err)
	}
	if gotID, gotRevision, err := ParseWorkspaceTemplateRevisionRef(revisionRef); err != nil || gotID != testTemplateAuthorityID || gotRevision != authorityDigest("1") {
		t.Fatalf("revision round trip = %q, %q, %v", gotID, gotRevision, err)
	}
	for name, invalid := range map[string]func() error{
		"template as context":    func() error { _, err := ParseContextRef(templateRef); return err },
		"context as workspace":   func() error { _, err := ParseWorkspaceRef(contextRef); return err },
		"workspace as template":  func() error { _, err := ParseWorkspaceTemplateRef(workspaceRef); return err },
		"name as template":       func() error { _, err := ParseWorkspaceTemplateRef("default"); return err },
		"generation as revision": func() error { _, _, err := ParseWorkspaceTemplateRevisionRef("1"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if invalid() == nil {
				t.Fatal("cross-kind or reconstructed reference passed")
			}
		})
	}
}

func TestWorkspaceTemplateRevisionNoOpAndABAReturn(t *testing.T) {
	a, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 1, templateBodyFixture("a"))
	if err != nil {
		t.Fatal(err)
	}
	noOp, changed, err := AdvanceWorkspaceTemplateRevision(a, templateBodyFixture("a"))
	if err != nil || changed || noOp.Generation != a.Generation || noOp.Revision != a.Revision {
		t.Fatalf("no-op = %#v, changed=%v, err=%v", noOp, changed, err)
	}
	b, changed, err := AdvanceWorkspaceTemplateRevision(a, templateBodyFixture("b"))
	if err != nil || !changed || b.Generation != 2 || b.Revision == a.Revision {
		t.Fatalf("B = %#v, changed=%v, err=%v", b, changed, err)
	}
	returnedA, changed, err := AdvanceWorkspaceTemplateRevision(b, templateBodyFixture("a"))
	if err != nil || !changed || returnedA.Generation != 3 || returnedA.Revision != a.Revision {
		t.Fatalf("A->B->A = %#v, changed=%v, err=%v", returnedA, changed, err)
	}
	if err := ValidateWorkspaceTemplateHistory([]WorkspaceTemplateRevision{a, b, returnedA}); err != nil {
		t.Fatal(err)
	}

	widened := templateBodyFixture("b")
	widened.Boundary.SourceAccess = ManifestSourceAccessReadWrite
	if _, _, err := AdvanceWorkspaceTemplateRevision(b, widened); err == nil {
		t.Fatal("same-Template Boundary change passed")
	}
	staleNoOp := b
	staleNoOp.Generation++
	if err := ValidateWorkspaceTemplateHistory([]WorkspaceTemplateRevision{b, staleNoOp}); err == nil {
		t.Fatal("published semantic no-op passed history validation")
	}
}

func TestWorkspaceTemplateRequiresExactRetainedCurrentAndCloneIsolation(t *testing.T) {
	first, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 1, templateBodyFixture("a"))
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := AdvanceWorkspaceTemplateRevision(first, templateBodyFixture("b"))
	if err != nil {
		t.Fatal(err)
	}
	template := WorkspaceTemplate{SchemaVersion: WorkspaceTemplateSchemaVersion, ID: testTemplateAuthorityID, Name: "restricted", Current: second, Retained: []WorkspaceTemplateRevision{first, second}}
	if err := template.Validate(); err != nil {
		t.Fatal(err)
	}
	clone := template.Clone()
	clone.Retained[0].Body.Policy.BaselineGrants[0].Path = "/changed"
	if template.Retained[0].Body.Policy.BaselineGrants[0].Path != "/a" {
		t.Fatal("Template clone shares retained storage")
	}
	missing := template
	missing.Retained = []WorkspaceTemplateRevision{first}
	if err := missing.Validate(); err == nil {
		t.Fatal("unretained current revision passed")
	}
}

func TestWorkspaceTemplateAdvancedPolicyIsOneClosedExactPair(t *testing.T) {
	valid := []WorkspaceTemplateAdvancedPolicyFile{
		{Path: WorkspaceTemplateAdvancedPolicyPath, Content: "package tobari_template\nallow := false\n"},
		{Path: WorkspaceTemplateAdvancedPolicyTestPath, Content: "package tobari_template\ntest_deny := true\n"},
	}
	sources, err := NewWorkspaceTemplateAdvancedPolicySources(valid)
	if err != nil {
		t.Fatal(err)
	}
	files, err := sources.Files()
	if err != nil || len(files) != 2 || files[0].Path != WorkspaceTemplateAdvancedPolicyPath || files[1].Path != WorkspaceTemplateAdvancedPolicyTestPath {
		t.Fatalf("closed Advanced projection = %#v, %v", files, err)
	}

	for name, files := range map[string][]WorkspaceTemplateAdvancedPolicyFile{
		"missing": {valid[0]},
		"renamed": {
			{Path: "custom.rego", Content: valid[0].Content},
			valid[1],
		},
		"duplicate": {valid[0], valid[0]},
		"extra": {
			valid[0], valid[1], {Path: "extra.rego", Content: "package extra"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewWorkspaceTemplateAdvancedPolicySources(files); err == nil {
				t.Fatal("non-canonical Advanced source set became Template authority")
			}
		})
	}

	guided := templateBodyFixture("guided")
	guided.Policy.AdvancedPolicy = &sources
	if err := guided.Validate(); err == nil {
		t.Fatal("Guided Template accepted Advanced executable sources")
	}
	advanced := templateBodyFixture("advanced")
	advanced.Policy.Mode = ManifestPolicyModeAdvanced
	if err := advanced.Validate(); err == nil {
		t.Fatal("Advanced Template accepted a missing executable source pair")
	}
	tooLarge := sources
	tooLarge.Tobari = strings.Repeat("x", WorkspaceTemplateAdvancedPolicyMaxBytes)
	advanced.Policy.AdvancedPolicy = &tooLarge
	if err := advanced.Validate(); err == nil {
		t.Fatal("Advanced Template accepted an over-limit executable source pair")
	}
}

func TestWorkspaceTemplateCompleteBodyDrivesCopyAndEntryWithoutParallelAuthority(t *testing.T) {
	literal := "workspace"
	gitName, gitEmail := "Example User", "user@example.dev"
	bootstrap, err := NewContextBootstrapSnapshot(1, ManifestAWSBootstrap{
		Profile: "engineering", SSOSession: "company", SSOStartURL: "https://example.awsapps.com/start",
		SSORegion: "us-east-1", SSORegistrationScopes: []string{"sso:account:access"}, AccountID: "123456789012",
		RoleName: "Developer", Region: "ap-northeast-1", Output: "json",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := templateBodyFixture("entry")
	body.Policy.Mode = ManifestPolicyModeAdvanced
	body.Policy.AdvancedPolicy = &WorkspaceTemplateAdvancedPolicySources{
		Tobari: "package tobari_template\nallow := false\n", TobariTest: "package tobari_template\ntest_deny := true\n",
	}
	body.SessionDefaults = WorkspaceTemplateSessionDefaults{
		ShellEnvironment: []ManifestShellEnvironmentSetting{{Variable: "PS1", Source: ManifestShellEnvironmentLiteral, Value: &literal}},
		GitIdentity:      &ManifestGitIdentitySetting{Source: ManifestGitIdentityLiteral, Name: &gitName, Email: &gitEmail},
	}
	body.CreationDefaults.Bootstrap = &bootstrap
	source, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 7, body)
	if err != nil {
		t.Fatal(err)
	}

	clone := source.Clone()
	clone.Body.Boundary.DestinationCeiling.Authorities[0].Host = "changed.example.dev"
	clone.Body.Boundary.MethodPolicy.Overrides[0].Decision = ManifestMethodDeny
	clone.Body.Policy.BaselineGrants[0].Path = "/changed"
	clone.Body.Policy.AdvancedPolicy.Tobari = "package changed"
	*clone.Body.SessionDefaults.ShellEnvironment[0].Value = "changed"
	*clone.Body.SessionDefaults.GitIdentity.Name = "Changed User"
	clone.Body.CreationDefaults.Bootstrap.AWS.SSORegistrationScopes[0] = "changed"
	if source.Body.Boundary.DestinationCeiling.Authorities[0].Host != "api.example.dev" ||
		source.Body.Boundary.MethodPolicy.Overrides[0].Decision != ManifestMethodAllow ||
		source.Body.Policy.BaselineGrants[0].Path != "/entry" ||
		source.Body.Policy.AdvancedPolicy.Tobari != "package tobari_template\nallow := false\n" ||
		*source.Body.SessionDefaults.ShellEnvironment[0].Value != literal ||
		*source.Body.SessionDefaults.GitIdentity.Name != gitName ||
		source.Body.CreationDefaults.Bootstrap.AWS.SSORegistrationScopes[0] != "sso:account:access" {
		t.Fatal("Template revision clone shares nested complete body storage")
	}

	tamperedBody := source.Clone()
	tamperedBody.Body.Policy.BaselineGrants[0].Path = "/drift"
	if err := tamperedBody.Validate(); err == nil {
		t.Fatal("Template revision accepted body drift without recomputed slices and revision")
	}
	tamperedSlice := source.Clone()
	tamperedSlice.Slices.EntrySliceDigest = authorityDigest("4")
	if err := tamperedSlice.Validate(); err == nil {
		t.Fatal("Template revision accepted a slice not derived from its body")
	}

	copyID := WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789a4")
	copied, err := CopyWorkspaceTemplateRevision(copyID, "copied", source)
	if err != nil {
		t.Fatal(err)
	}
	if copied.ID != copyID || copied.Current.Generation != 1 || copied.Current.Revision != source.Revision || copied.Current.TemplateID == source.TemplateID {
		t.Fatalf("independent Template copy = %#v", copied)
	}
	copied.Current.Body.Policy.BaselineGrants[0].Path = "/copy-drift"
	if source.Body.Policy.BaselineGrants[0].Path != "/entry" {
		t.Fatal("Template copy aliases its immutable source revision")
	}

	entry, err := DeriveWorkspaceTemplateEntryAuthority(source)
	if err != nil {
		t.Fatal(err)
	}
	if entry.TemplateID != source.TemplateID || entry.TemplateRevision != source.Revision || entry.EntrySliceDigest != source.Slices.EntrySliceDigest || entry.Runtime != body.EntryDefaults.Runtime || *entry.SessionDefaults.ShellEnvironment[0].Value != literal || entry.CreationDefaults.Bootstrap.Revision != bootstrap.Revision {
		t.Fatalf("entry authority did not derive the complete final operation input: %#v", entry)
	}
	tamperedEntry := entry
	tamperedEntry.SessionDefaults = entry.SessionDefaults.Clone()
	*tamperedEntry.SessionDefaults.ShellEnvironment[0].Value = "tampered"
	if err := tamperedEntry.ValidateFor(source); err == nil {
		t.Fatal("entry authority accepted session defaults outside its exact Template revision")
	}
	*entry.SessionDefaults.ShellEnvironment[0].Value = "entry-changed"
	if *source.Body.SessionDefaults.ShellEnvironment[0].Value != literal {
		t.Fatal("derived entry authority aliases its source revision")
	}
}

func TestContextBindingsEnforceOneProjectTemplatePair(t *testing.T) {
	first := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: testContextAuthorityID, ProjectRoot: "/workspace/example", TemplateID: testTemplateAuthorityID}
	if err := ValidateContextBindings([]ContextBinding{first}); err != nil {
		t.Fatal(err)
	}
	duplicatePair := first
	duplicatePair.ID = "01912345-6789-7abc-8def-0123456789a4"
	if err := ValidateContextBindings([]ContextBinding{first, duplicatePair}); err == nil {
		t.Fatal("duplicate Project/Template Context passed")
	}
	otherTemplate := duplicatePair
	otherTemplate.TemplateID = "01912345-6789-7abc-8def-0123456789a5"
	if err := ValidateContextBindings([]ContextBinding{first, otherTemplate}); err != nil {
		t.Fatalf("same Project with another Template failed: %v", err)
	}
}

func policyMemoryBodyFixture(path string) PolicyMemoryRuleBody {
	return PolicyMemoryRuleBody{
		PolicyProtocolIdentity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP},
		Match:                  PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: path,
		Segments: []string{}, Examples: []string{path}, SourceCandidates: []string{"pcy_0123456789abcdef0123456789abcdef"},
	}
}

func TestPolicyMemoryPublishesCompleteIndependentRevisions(t *testing.T) {
	allow, err := NewPolicyMemoryRule(testContextAuthorityID, PolicyMemoryAllow, policyMemoryBodyFixture("/items/1"))
	if err != nil {
		t.Fatal(err)
	}
	deny, err := NewPolicyMemoryRule(testContextAuthorityID, PolicyMemoryDeny, policyMemoryBodyFixture("/items/2"))
	if err != nil {
		t.Fatal(err)
	}
	first, changed, err := PublishPolicyMemory(testContextAuthorityID, []PolicyMemoryRule{deny, allow}, nil)
	if err != nil || !changed || first.Generation != 1 || len(first.Rules) != 2 || first.Rules[0].ID > first.Rules[1].ID {
		t.Fatalf("first memory = %#v, changed=%v, err=%v", first, changed, err)
	}
	noOp, changed, err := PublishPolicyMemory(testContextAuthorityID, []PolicyMemoryRule{allow, deny}, &first)
	if err != nil || changed || noOp.Generation != 1 || noOp.Revision != first.Revision {
		t.Fatalf("memory no-op = %#v, changed=%v, err=%v", noOp, changed, err)
	}
	second, changed, err := PublishPolicyMemory(testContextAuthorityID, []PolicyMemoryRule{allow}, &first)
	if err != nil || !changed || second.Generation != 2 || second.Revision == first.Revision {
		t.Fatalf("second memory = %#v, changed=%v, err=%v", second, changed, err)
	}

	clone := first.Clone()
	clone.Rules[0].Body.Examples[0] = "/changed"
	if first.Rules[0].Body.Examples[0] == "/changed" {
		t.Fatal("Policy Memory clone shares rule evidence")
	}
	otherContext := ContextID("01912345-6789-7abc-8def-0123456789a6")
	if err := allow.Validate(otherContext); err == nil {
		t.Fatal("Policy Memory rule crossed Context authority")
	}
}

func TestWorkspaceBindingAndIndependentReceiptsRejectCrossOwnerState(t *testing.T) {
	context := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: testContextAuthorityID, ProjectRoot: "/workspace/example", TemplateID: testTemplateAuthorityID}
	revision, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 1, templateBodyFixture("a"))
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := PublishPolicyMemory(testContextAuthorityID, []PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	applied := WorkspaceAppliedEntry{
		ContextID: testContextAuthorityID, TemplateID: testTemplateAuthorityID, TemplateRevision: revision.Revision,
		EntrySliceDigest: revision.Slices.EntrySliceDigest, RuntimeID: StandardRuntimeID,
		RuntimeRevision: revision.Slices.RuntimeRevision, ResolvedSpec: authorityDigest("7"), ReconciledAt: time.Unix(1, 0).UTC(),
	}
	workspace := WorkspaceBinding{SchemaVersion: WorkspaceBindingSchemaVersion, ID: testWorkspaceAuthorityID, ContextID: testContextAuthorityID, ProjectRoot: context.ProjectRoot, Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied}
	if err := workspace.ValidateFor(context); err != nil {
		t.Fatal(err)
	}
	templateReceipt := TemplatePolicyActivationReceipt{ContextID: context.ID, TemplateID: context.TemplateID, PolicySliceDigest: revision.Slices.PolicySliceDigest}
	if err := templateReceipt.ValidateFor(context, revision); err != nil {
		t.Fatal(err)
	}
	memoryReceipt := PolicyMemoryActivationReceipt{ContextID: context.ID, Revision: memory.Revision}
	if err := memoryReceipt.ValidateFor(context, memory); err != nil {
		t.Fatal(err)
	}

	other := context
	other.ID = "01912345-6789-7abc-8def-0123456789a6"
	if err := workspace.ValidateFor(other); err == nil {
		t.Fatal("Workspace crossed Context owner")
	}
	wrongPolicy := templateReceipt
	wrongPolicy.PolicySliceDigest = memory.Revision
	if err := wrongPolicy.ValidateFor(context, revision); err == nil {
		t.Fatal("Policy Memory digest passed as Template policy receipt")
	}
	wrongMemory := memoryReceipt
	wrongMemory.Revision = revision.Slices.PolicySliceDigest
	if err := wrongMemory.ValidateFor(context, memory); err == nil {
		t.Fatal("Template policy digest passed as Policy Memory receipt")
	}
	wrongEntry := applied
	wrongEntry.RuntimeRevision = authorityDigest("4")
	if err := wrongEntry.ValidateForRevision(context, revision); err == nil {
		t.Fatal("AppliedEntry with another Runtime revision passed exact Template validation")
	}
}

func TestWorkspaceEntryPlanAndExactContainerReceiptBindCurrentAuthority(t *testing.T) {
	contextBinding := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: testContextAuthorityID, ProjectRoot: "/workspace/example", TemplateID: testTemplateAuthorityID}
	revision, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 1, templateBodyFixture("entry"))
	if err != nil {
		t.Fatal(err)
	}
	template := WorkspaceTemplate{SchemaVersion: WorkspaceTemplateSchemaVersion, ID: testTemplateAuthorityID, Name: "restricted", Current: revision, Retained: []WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, err := PublishPolicyMemory(testContextAuthorityID, []PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt := TemplatePolicyActivationReceipt{ContextID: testContextAuthorityID, TemplateID: testTemplateAuthorityID, PolicySliceDigest: revision.Slices.PolicySliceDigest}
	memoryReceipt := PolicyMemoryActivationReceipt{ContextID: testContextAuthorityID, Revision: memory.Revision}
	activeMemory := memory.Clone()
	snapshot := ContextAuthoritySnapshot{Context: contextBinding, Template: template, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: &activeMemory, ActivePolicyMemoryRef: &memoryReceipt}
	applied := WorkspaceAppliedEntry{
		ContextID: testContextAuthorityID, TemplateID: testTemplateAuthorityID, TemplateRevision: revision.Revision,
		EntrySliceDigest: revision.Slices.EntrySliceDigest, RuntimeID: revision.Slices.RuntimeID,
		RuntimeRevision: revision.Slices.RuntimeRevision, ResolvedSpec: authorityDigest("7"), ReconciledAt: time.Unix(2, 0).UTC(),
	}
	workspace := WorkspaceBinding{SchemaVersion: WorkspaceBindingSchemaVersion, ID: testWorkspaceAuthorityID, ContextID: testContextAuthorityID, ProjectRoot: contextBinding.ProjectRoot, Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied}
	plan := WorkspaceEntryReconciliationPlan{Workspace: workspace, Applied: applied}
	if err := plan.ValidateFor(snapshot); err != nil {
		t.Fatal(err)
	}
	receipt := WorkspaceEntryReconciliationReceipt{WorkspaceID: workspace.ID, ContextID: contextBinding.ID, Applied: applied, ContainerID: strings.Repeat("a", 64)}
	if err := receipt.ValidateFor(plan); err != nil {
		t.Fatal(err)
	}

	for name, containerID := range map[string]string{
		"container name":  "tobari-workspace-container",
		"short ID":        strings.Repeat("a", 12),
		"uppercase":       strings.Repeat("A", 64),
		"digest-prefixed": "sha256:" + strings.Repeat("a", 64),
	} {
		t.Run(name, func(t *testing.T) {
			invalid := receipt
			invalid.ContainerID = containerID
			if err := invalid.ValidateFor(plan); err == nil {
				t.Fatal("non-exact Docker container identity passed")
			}
		})
	}

	wrongWorkspace := receipt
	wrongWorkspace.WorkspaceID = "01912345-6789-7abc-8def-0123456789a4"
	if err := wrongWorkspace.ValidateFor(plan); err == nil {
		t.Fatal("container receipt crossed Workspace authority")
	}
	snapshot.Workspace = &workspace
	changed := plan.Clone()
	changed.Workspace.Home = "/workspace/other-home"
	if err := changed.ValidateFor(snapshot); err == nil {
		t.Fatal("entry plan changed create-once Workspace authority")
	}
	clone := plan.Clone()
	clone.Workspace.LastSuccessfulEntry.ResolvedSpec = authorityDigest("8")
	if plan.Workspace.LastSuccessfulEntry.ResolvedSpec == clone.Workspace.LastSuccessfulEntry.ResolvedSpec {
		t.Fatal("entry plan clone aliases AppliedEntry")
	}
}

func TestWorkspaceSessionBindingCarriesCompleteFinalPrincipalAndEntryAuthority(t *testing.T) {
	literal := "workspace"
	body := templateBodyFixture("session")
	body.SessionDefaults.ShellEnvironment = []ManifestShellEnvironmentSetting{{Variable: "PS1", Source: ManifestShellEnvironmentLiteral, Value: &literal}}
	revision, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := WorkspaceTemplate{SchemaVersion: WorkspaceTemplateSchemaVersion, ID: testTemplateAuthorityID, Name: "restricted", Current: revision, Retained: []WorkspaceTemplateRevision{revision.Clone()}}
	contextBinding := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: testContextAuthorityID, ProjectRoot: "/workspace/example", TemplateID: testTemplateAuthorityID}
	memory, _, err := PublishPolicyMemory(testContextAuthorityID, []PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt := TemplatePolicyActivationReceipt{ContextID: contextBinding.ID, TemplateID: template.ID, PolicySliceDigest: revision.Slices.PolicySliceDigest}
	memoryReceipt := PolicyMemoryActivationReceipt{ContextID: contextBinding.ID, Revision: memory.Revision}
	applied := WorkspaceAppliedEntry{
		ContextID: contextBinding.ID, TemplateID: template.ID, TemplateRevision: revision.Revision,
		EntrySliceDigest: revision.Slices.EntrySliceDigest, RuntimeID: revision.Slices.RuntimeID,
		RuntimeRevision: revision.Slices.RuntimeRevision, ResolvedSpec: authorityDigest("7"), ReconciledAt: time.Unix(3, 0).UTC(),
	}
	workspace := WorkspaceBinding{
		SchemaVersion: WorkspaceBindingSchemaVersion, ID: testWorkspaceAuthorityID, ContextID: contextBinding.ID,
		ProjectRoot: contextBinding.ProjectRoot, Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest,
		LastSuccessfulEntry: &applied,
	}
	snapshot := ContextAuthoritySnapshot{
		Context: contextBinding, Template: template, PolicyMemory: memory, Workspace: &workspace,
		ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: &memory, ActivePolicyMemoryRef: &memoryReceipt,
	}
	receipt := WorkspaceEntryReconciliationReceipt{
		WorkspaceID: workspace.ID, ContextID: contextBinding.ID, Applied: applied, ContainerID: strings.Repeat("a", 64),
	}
	binding, err := NewWorkspaceSessionBinding(snapshot, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if binding.ContextID != contextBinding.ID || binding.WorkspaceID != workspace.ID || binding.TemplateID != template.ID ||
		binding.ContextPresentation != template.Name || binding.ProjectRoot != contextBinding.ProjectRoot ||
		binding.ContainerID != receipt.ContainerID || binding.AppliedEntry != applied {
		t.Fatalf("final session binding = %#v", binding)
	}
	identity, err := NewWorkspaceSessionIdentity(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if identity.ContextID != binding.ContextID || identity.WorkspaceID != binding.WorkspaceID ||
		identity.TemplateID != binding.TemplateID || identity.ContextPresentation != binding.ContextPresentation ||
		identity.ProjectRoot != binding.ProjectRoot {
		t.Fatalf("persistent session identity = %#v", identity)
	}
	fromBinding, err := binding.Identity()
	if err != nil || fromBinding != identity {
		t.Fatalf("binding identity = %#v, %v; snapshot identity = %#v", fromBinding, err, identity)
	}
	changedIdentity := identity
	changedIdentity.ProjectRoot = "/workspace/other"
	if err := changedIdentity.Validate(); err == nil {
		t.Fatal("mutated persistent session identity passed")
	}

	for name, mutate := range map[string]func(*WorkspaceSessionBinding){
		"Context":              func(value *WorkspaceSessionBinding) { value.ContextID = "01912345-6789-7abc-8def-0123456789a6" },
		"Workspace":            func(value *WorkspaceSessionBinding) { value.WorkspaceID = "01912345-6789-7abc-8def-0123456789a6" },
		"Template":             func(value *WorkspaceSessionBinding) { value.TemplateID = "01912345-6789-7abc-8def-0123456789a6" },
		"Template revision":    func(value *WorkspaceSessionBinding) { value.TemplateRevision = authorityDigest("9") },
		"presentation":         func(value *WorkspaceSessionBinding) { value.ContextPresentation = "bad name" },
		"Project root":         func(value *WorkspaceSessionBinding) { value.ProjectRoot = "/workspace/other" },
		"Workspace home":       func(value *WorkspaceSessionBinding) { value.WorkspaceHome = "/workspace/other-home" },
		"session slice digest": func(value *WorkspaceSessionBinding) { value.SessionDefaultsDigest = authorityDigest("9") },
		"container":            func(value *WorkspaceSessionBinding) { value.ContainerID = strings.Repeat("A", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			changed := binding.Clone()
			mutate(&changed)
			if err := changed.Validate(); err == nil {
				t.Fatal("cross-authority final session binding passed")
			}
		})
	}
	clone := binding.Clone()
	*clone.SessionDefaults.ShellEnvironment[0].Value = "changed"
	if *binding.SessionDefaults.ShellEnvironment[0].Value != literal {
		t.Fatal("final Workspace session binding clone aliases Template defaults")
	}
}
