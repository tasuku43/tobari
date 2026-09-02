package tobari

import (
	"testing"
	"time"
)

func workspaceAuthorityCollectionFixture(t *testing.T) WorkspaceAuthorityCollection {
	t.Helper()
	revision, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 1, templateBodyFixture("items"))
	if err != nil {
		t.Fatal(err)
	}
	template := WorkspaceTemplate{
		SchemaVersion: WorkspaceTemplateSchemaVersion, ID: testTemplateAuthorityID, Name: "restricted",
		Current: revision, Retained: []WorkspaceTemplateRevision{revision.Clone()},
	}
	context := ContextBinding{
		SchemaVersion: ContextBindingSchemaVersion, ID: testContextAuthorityID,
		TemplateID: testTemplateAuthorityID,
	}
	memory, _, err := PublishPolicyMemory(testContextAuthorityID, []PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt := TemplatePolicyActivationReceipt{
		ContextID: context.ID, TemplateID: template.ID, PolicySliceDigest: revision.Slices.PolicySliceDigest,
	}
	memoryReceipt := PolicyMemoryActivationReceipt{ContextID: context.ID, Revision: memory.Revision}
	record := WorkspaceAuthorityContextRecord{
		Context: context, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt,
		ActivePolicyMemory: collectionPolicyMemoryPtr(memory), ActivePolicyMemoryRef: &memoryReceipt,
	}
	applied := WorkspaceAppliedEntry{
		ContextID: context.ID, TemplateID: template.ID, TemplateRevision: revision.Revision,
		EntrySliceDigest: revision.Slices.EntrySliceDigest, RuntimeID: revision.Slices.RuntimeID,
		RuntimeRevision: revision.Slices.RuntimeRevision, ResolvedSpec: authorityDigest("7"), ReconciledAt: time.Unix(1, 0).UTC(),
	}
	workspace := WorkspaceBinding{
		SchemaVersion: WorkspaceBindingSchemaVersion, ID: testWorkspaceAuthorityID, ContextID: context.ID,
		ProjectRoot: "/workspace/example", Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest,
		LastSuccessfulEntry: &applied,
	}
	effect := PolicyCandidateEffect{
		PolicyProtocolIdentity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP},
		Match:                  PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/candidate",
		Segments: []string{}, Examples: []string{"/candidate"},
	}
	candidate, err := NewPolicyCandidateAuthority(context.ID, workspace.ID, effect)
	if err != nil {
		t.Fatal(err)
	}
	collection, changed, err := PublishWorkspaceAuthorityCollection(
		[]WorkspaceTemplate{template}, []WorkspaceAuthorityContextRecord{record}, []WorkspaceBinding{workspace},
		[]PolicyCandidateAuthority{candidate}, ptrTemplateID(template.ID), nil,
	)
	if err != nil || !changed {
		t.Fatalf("publish collection: changed=%t err=%v", changed, err)
	}
	return collection
}

func collectionPolicyMemoryPtr(memory PolicyMemoryRevision) *PolicyMemoryRevision {
	clone := memory.Clone()
	return &clone
}

func TestWorkspaceAuthorityCollectionRequiresPendingCandidatesToBeUnconsumed(t *testing.T) {
	valid := workspaceAuthorityCollectionFixture(t)
	candidate := valid.PendingCandidates[0]
	rule, err := NewPolicyMemoryRule(candidate.ContextID, PolicyMemoryAllow, candidate.Effect.RuleBody(candidate.ID))
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := PublishPolicyMemory(candidate.ContextID, []PolicyMemoryRule{rule}, &valid.Contexts[0].PolicyMemory)
	if err != nil {
		t.Fatal(err)
	}

	consumed := valid.Clone()
	consumed.Contexts[0].PolicyMemory = memory
	consumed.PendingCandidates = []PolicyCandidateAuthority{}
	consumed.Revision, _ = workspaceAuthorityCollectionRevision(consumed)
	if err := consumed.Validate(); err != nil {
		t.Fatalf("consumed candidate without pending record must remain valid: %v", err)
	}

	retained := consumed.Clone()
	retained.PendingCandidates = []PolicyCandidateAuthority{candidate}
	retained.Revision, _ = workspaceAuthorityCollectionRevision(retained)
	if err := retained.Validate(); err == nil {
		t.Fatal("consumed candidate remained actionable as pending")
	}
}

func TestWorkspaceAuthorityCollectionCurrentContextIsLocationFreeAndRevisionBound(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	selectedID := base.Contexts[0].Context.ID
	selected, changed, err := PublishWorkspaceAuthorityCollectionWithCurrentContext(
		base.Templates, base.Contexts, base.Workspaces, base.PendingCandidates,
		base.DefaultTemplateID, &selectedID, &base,
	)
	if err != nil || !changed || selected.CurrentContextID == nil || *selected.CurrentContextID != selectedID || selected.Revision == base.Revision {
		t.Fatalf("selected=%+v changed=%t err=%v", selected.CurrentContextID, changed, err)
	}
	clone := selected.Clone()
	*clone.CurrentContextID = ContextID("01912345-6789-7abc-8def-0123456789ff")
	if *selected.CurrentContextID != selectedID {
		t.Fatal("current Context clone aliases collection state")
	}
	invalid := selected.Clone()
	unknown := ContextID("01912345-6789-7abc-8def-0123456789ff")
	invalid.CurrentContextID = &unknown
	invalid.Revision, _ = workspaceAuthorityCollectionRevision(invalid)
	if err := invalid.Validate(); err == nil {
		t.Fatal("unknown current Context validated")
	}
}

func TestPolicyMemoryBoundaryTighteningSupersedesOnlyOutsideAllows(t *testing.T) {
	context := ContextBinding{
		SchemaVersion: ContextBindingSchemaVersion,
		ID:            testContextAuthorityID,
		TemplateID:    testTemplateAuthorityID,
	}
	allowOutside, err := NewPolicyMemoryRule(context.ID, PolicyMemoryAllow, policyMemoryBodyFixture("/items/1"))
	if err != nil {
		t.Fatal(err)
	}
	allowInsideBody := policyMemoryBodyFixture("/items/2")
	allowInsideBody.Method = "POST"
	allowInside, err := NewPolicyMemoryRule(context.ID, PolicyMemoryAllow, allowInsideBody)
	if err != nil {
		t.Fatal(err)
	}
	denyBody := policyMemoryBodyFixture("/items/3")
	deny, err := NewPolicyMemoryRule(context.ID, PolicyMemoryDeny, denyBody)
	if err != nil {
		t.Fatal(err)
	}
	memory, changed, err := PublishPolicyMemory(context.ID, []PolicyMemoryRule{allowOutside, allowInside, deny}, nil)
	if err != nil || !changed {
		t.Fatalf("publish memory: changed=%t err=%v", changed, err)
	}
	receipt := PolicyMemoryActivationReceipt{ContextID: context.ID, Revision: memory.Revision}
	record := WorkspaceAuthorityContextRecord{
		Context: context, PolicyMemory: memory,
		ActivePolicyMemory: collectionPolicyMemoryPtr(memory), ActivePolicyMemoryRef: &receipt,
	}

	tightenedBody := templateBodyFixture("items")
	tightenedBody.Boundary.MethodPolicy.Overrides[0].Decision = ManifestMethodDeny
	tightenedBody.Policy.BaselineGrants[0].Method = "POST"
	tightened, err := NewWorkspaceTemplateRevision(context.TemplateID, 2, tightenedBody)
	if err != nil {
		t.Fatal(err)
	}
	settled, removed, err := SupersedePolicyMemoryAllowsOutsideBoundary(record, tightened)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || settled.PolicyMemory.Generation != memory.Generation+1 {
		t.Fatalf("removed=%d generation=%d", removed, settled.PolicyMemory.Generation)
	}
	if settled.ActivePolicyMemory != nil || settled.ActivePolicyMemoryRef != nil {
		t.Fatal("tightening retained stale active Policy Memory receipts")
	}
	if len(settled.PolicyMemory.Rules) != 2 {
		t.Fatalf("remaining rules=%d", len(settled.PolicyMemory.Rules))
	}
	remaining := map[string]PolicyMemoryDecision{}
	for _, rule := range settled.PolicyMemory.Rules {
		remaining[rule.ID] = rule.Decision
	}
	if remaining[allowInside.ID] != PolicyMemoryAllow || remaining[deny.ID] != PolicyMemoryDeny {
		t.Fatalf("unexpected remaining rules: %#v", remaining)
	}
	if _, exists := remaining[allowOutside.ID]; exists {
		t.Fatal("outside Allow survived tightening")
	}
	if len(settled.SupersededAllows) != 1 {
		t.Fatalf("supersession history=%d", len(settled.SupersededAllows))
	}
	history := settled.SupersededAllows[0]
	if history.Rule.ID != allowOutside.ID || history.SourceMemoryRevision != memory.Revision ||
		history.TemplateID != tightened.TemplateID || history.TemplateRevision != tightened.Revision {
		t.Fatalf("supersession provenance=%#v", history)
	}
	if err := history.Validate(); err != nil {
		t.Fatalf("supersession provenance invalid: %v", err)
	}
	if len(record.PolicyMemory.Rules) != 3 || record.ActivePolicyMemory == nil || record.ActivePolicyMemoryRef == nil {
		t.Fatal("supersession mutated its input record")
	}
}

func TestPolicyMemoryBoundaryNoOpPreservesRevisionAndActiveReceipts(t *testing.T) {
	context := ContextBinding{
		SchemaVersion: ContextBindingSchemaVersion,
		ID:            testContextAuthorityID,
		TemplateID:    testTemplateAuthorityID,
	}
	allow, err := NewPolicyMemoryRule(context.ID, PolicyMemoryAllow, policyMemoryBodyFixture("/items/1"))
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := PublishPolicyMemory(context.ID, []PolicyMemoryRule{allow}, nil)
	if err != nil {
		t.Fatal(err)
	}
	receipt := PolicyMemoryActivationReceipt{ContextID: context.ID, Revision: memory.Revision}
	record := WorkspaceAuthorityContextRecord{
		Context: context, PolicyMemory: memory,
		ActivePolicyMemory: collectionPolicyMemoryPtr(memory), ActivePolicyMemoryRef: &receipt,
	}
	template, err := NewWorkspaceTemplateRevision(context.TemplateID, 1, templateBodyFixture("items"))
	if err != nil {
		t.Fatal(err)
	}

	settled, removed, err := SupersedePolicyMemoryAllowsOutsideBoundary(record, template)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 || settled.PolicyMemory.Revision != memory.Revision || settled.PolicyMemory.Generation != memory.Generation {
		t.Fatalf("unexpected no-op settlement: removed=%d memory=%#v", removed, settled.PolicyMemory)
	}
	if settled.ActivePolicyMemory == nil || settled.ActivePolicyMemoryRef == nil || settled.ActivePolicyMemoryRef.Revision != memory.Revision {
		t.Fatal("no-op supersession changed active Policy Memory receipts")
	}
	if len(settled.SupersededAllows) != 0 {
		t.Fatal("no-op supersession created provenance")
	}
}

func TestBoundaryTighteningSupersedesPendingCandidatesWithoutGrantingAuthority(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	tightenedBody := base.Templates[0].Current.Body.Clone()
	for index := range tightenedBody.Boundary.MethodPolicy.Overrides {
		if tightenedBody.Boundary.MethodPolicy.Overrides[index].Method == "GET" {
			tightenedBody.Boundary.MethodPolicy.Overrides[index].Decision = ManifestMethodDeny
		}
	}
	tightenedBody.Policy.BaselineGrants[0].Method = "POST"
	tightened, err := NewWorkspaceTemplateRevision(base.Templates[0].ID, 2, tightenedBody)
	if err != nil {
		t.Fatal(err)
	}
	records, pending, removed, err := SupersedePolicyCandidatesOutsideBoundary(base.Contexts, base.PendingCandidates, tightened)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || len(pending) != 0 || len(records[0].SupersededCandidates) != 1 {
		t.Fatalf("removed=%d pending=%d history=%d", removed, len(pending), len(records[0].SupersededCandidates))
	}
	history := records[0].SupersededCandidates[0]
	if history.Candidate.ID != base.PendingCandidates[0].ID || history.TemplateRevision != tightened.Revision || history.TemplateID != tightened.TemplateID {
		t.Fatalf("supersession provenance=%#v", history)
	}
	if records[0].PolicyMemory.Revision != base.Contexts[0].PolicyMemory.Revision || len(records[0].PolicyMemory.Rules) != 0 {
		t.Fatal("pending candidate supersession mutated or widened Policy Memory")
	}
	if len(base.PendingCandidates) != 1 || len(base.Contexts[0].SupersededCandidates) != 0 {
		t.Fatal("candidate supersession mutated its inputs")
	}
	template := base.Templates[0].Clone()
	template.Current = tightened
	template.Retained = append(template.Retained, tightened)
	settled, changed, err := PublishWorkspaceAuthorityCollection([]WorkspaceTemplate{template}, records, base.Workspaces, pending, base.DefaultTemplateID, &base)
	if err != nil || !changed || len(settled.PendingCandidates) != 0 {
		t.Fatalf("publish tightened collection: changed=%t pending=%d err=%v", changed, len(settled.PendingCandidates), err)
	}
}

func ptrTemplateID(id WorkspaceTemplateID) *WorkspaceTemplateID {
	value := id
	return &value
}

func TestWorkspaceAuthorityCollectionPublishesOneCoherentEnvelope(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	if err := collection.Validate(); err != nil {
		t.Fatal(err)
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil || len(snapshots) != 1 || snapshots[0].Workspace == nil || snapshots[0].Workspace.ID != testWorkspaceAuthorityID {
		t.Fatalf("snapshots=%#v err=%v", snapshots, err)
	}
	noOp, changed, err := PublishWorkspaceAuthorityCollection(
		collection.Templates, collection.Contexts, collection.Workspaces, collection.PendingCandidates,
		collection.DefaultTemplateID, &collection,
	)
	if err != nil || changed || noOp.Generation != collection.Generation || noOp.Revision != collection.Revision {
		t.Fatalf("no-op=%#v changed=%t err=%v", noOp, changed, err)
	}
	clone := collection.Clone()
	clone.Templates[0].Current.Body.Policy.BaselineGrants[0].Path = "/changed"
	clone.Contexts[0].PolicyMemory.Rules = append(clone.Contexts[0].PolicyMemory.Rules, PolicyMemoryRule{})
	clone.PendingCandidates[0].Effect.Examples[0] = "/changed"
	clone.Workspaces[0].Home = "/changed"
	if collection.Templates[0].Current.Body.Policy.BaselineGrants[0].Path == "/changed" || len(collection.Contexts[0].PolicyMemory.Rules) != 0 || collection.PendingCandidates[0].Effect.Examples[0] == "/changed" || collection.Workspaces[0].Home == "/changed" {
		t.Fatal("Workspace authority collection clone shares nested authority")
	}
}

func TestWorkspaceAuthorityCollectionRejectsPartialOrMixedAuthority(t *testing.T) {
	valid := workspaceAuthorityCollectionFixture(t)
	tests := map[string]func(*WorkspaceAuthorityCollection){
		"missing Template": func(value *WorkspaceAuthorityCollection) { value.Templates = []WorkspaceTemplate{} },
		"duplicate Workspace for Context": func(value *WorkspaceAuthorityCollection) {
			copy := value.Workspaces[0]
			copy.ID = "01912345-6789-7abc-8def-0123456789a4"
			value.Workspaces = append(value.Workspaces, copy)
		},
		"candidate crosses Workspace": func(value *WorkspaceAuthorityCollection) {
			value.PendingCandidates[0].ObservingWorkspaceID = "01912345-6789-7abc-8def-0123456789a4"
		},
		"active Policy Memory crosses Context": func(value *WorkspaceAuthorityCollection) {
			value.Contexts[0].ActivePolicyMemoryRef.ContextID = "01912345-6789-7abc-8def-0123456789a4"
		},
		"default Template missing": func(value *WorkspaceAuthorityCollection) {
			id := WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789a4")
			value.DefaultTemplateID = &id
		},
		"Workspace creation receipt missing": func(value *WorkspaceAuthorityCollection) {
			value.Workspaces[0].CreationDefaults = authorityDigest("4")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid.Clone()
			mutate(&candidate)
			candidate.Revision, _ = workspaceAuthorityCollectionRevision(candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("partial or mixed Workspace authority passed")
			}
		})
	}
}
