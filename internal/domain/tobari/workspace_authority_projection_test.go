package tobari

import (
	"reflect"
	"testing"
)

func TestWorkspacePolicyProjectionKeepsHotAndClusterActivationAxesIndependent(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	oldTemplate := base.Templates[0].Current.Clone()
	nextBody := oldTemplate.Body.Clone()
	nextBody.Policy.BaselineGrants[0].Path = "/next"
	nextTemplate, changed, err := AdvanceWorkspaceTemplateRevision(oldTemplate, nextBody)
	if err != nil || !changed {
		t.Fatalf("advance Template: changed=%t err=%v", changed, err)
	}
	templates := cloneWorkspaceTemplates(base.Templates)
	templates[0].Current = nextTemplate
	templates[0].Retained = append(templates[0].Retained, nextTemplate.Clone())

	previousMemory := base.Contexts[0].PolicyMemory.Clone()
	rule, err := NewPolicyMemoryRule(base.Contexts[0].Context.ID, PolicyMemoryDeny, policyMemoryBodyFixture("/remembered"))
	if err != nil {
		t.Fatal(err)
	}
	currentMemory, memoryChanged, err := PublishPolicyMemory(base.Contexts[0].Context.ID, []PolicyMemoryRule{rule}, &previousMemory)
	if err != nil || !memoryChanged {
		t.Fatalf("advance memory: changed=%t err=%v", memoryChanged, err)
	}
	contexts := cloneWorkspaceAuthorityContextRecords(base.Contexts)
	contexts[0].PolicyMemory = currentMemory
	// The active axes deliberately remain the predecessor Template policy and
	// predecessor Policy Memory until their respective reconciliation acts.
	collection, _, err := PublishWorkspaceAuthorityCollection(
		templates, contexts, base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, &base,
	)
	if err != nil {
		t.Fatal(err)
	}

	hot, err := BuildHotWorkspacePolicyProjection(collection, collection.Contexts[0].Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hot.Contexts[0].TemplatePolicy.PolicySliceDigest != oldTemplate.Slices.PolicySliceDigest ||
		hot.Contexts[0].PolicyMemory.Revision != currentMemory.Revision {
		t.Fatalf("hot projection adopted pending current axes: %#v", hot.Contexts[0])
	}
	cluster, err := BuildClusterWorkspacePolicyProjection(collection)
	if err != nil {
		t.Fatal(err)
	}
	if cluster.Contexts[0].TemplatePolicy.PolicySliceDigest != nextTemplate.Slices.PolicySliceDigest ||
		cluster.Contexts[0].PolicyMemory.Revision != currentMemory.Revision {
		t.Fatalf("cluster projection did not select current axes: %#v", cluster.Contexts[0])
	}
	if hot.ContentDigest == cluster.ContentDigest {
		t.Fatal("different selected Template policy content collapsed")
	}
	baseHot, err := BuildHotWorkspacePolicyProjection(base, base.Contexts[0].Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	baseCluster, err := BuildClusterWorkspacePolicyProjection(base)
	if err != nil || baseHot.ContentDigest != baseCluster.ContentDigest || baseHot.PlanDigest == baseCluster.PlanDigest {
		t.Fatalf("content/plan identities did not separate route: hot=%#v cluster=%#v err=%v", baseHot, baseCluster, err)
	}
}

func TestWorkspacePolicyProjectionIsCompleteSortedAndCloneIsolated(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	withoutWorkspace, _, err := PublishWorkspaceAuthorityCollection(
		collection.Templates, collection.Contexts, []WorkspaceBinding{}, []PolicyCandidateAuthority{},
		collection.DefaultTemplateID, &collection,
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := BuildClusterWorkspacePolicyProjection(withoutWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Contexts) != 1 || projection.Contexts[0].Principal != nil {
		t.Fatalf("Context without Workspace produced a principal: %#v", projection.Contexts)
	}
	clone := projection.Clone()
	clone.Contexts[0].TemplatePolicy.Policy.BaselineGrants[0].Path = "/mutated"
	clone.Contexts[0].PolicyMemory.Rules = append(clone.Contexts[0].PolicyMemory.Rules, PolicyMemoryRule{})
	if reflect.DeepEqual(clone, projection) || projection.Contexts[0].TemplatePolicy.Policy.BaselineGrants[0].Path == "/mutated" || len(projection.Contexts[0].PolicyMemory.Rules) != 0 {
		t.Fatal("projection clone shares nested authority")
	}
}

func TestWorkspacePolicyProjectionUsesWorkspaceRetainedCreationAuthority(t *testing.T) {
	bodyA := templateBodyFixture("items")
	bootstrapA, err := NewContextBootstrapSnapshotWithEKS(1, testAWSBootstrap(), testEKSBootstrap(t))
	if err != nil {
		t.Fatal(err)
	}
	bodyA.CreationDefaults.Bootstrap = &bootstrapA
	revisionA, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 1, bodyA)
	if err != nil {
		t.Fatal(err)
	}
	bodyB := bodyA.Clone()
	eksB := *bodyB.CreationDefaults.Bootstrap.EKS
	eksB.Server = "https://def.gr7.ap-northeast-1.eks.amazonaws.com"
	bootstrapB, err := NewContextBootstrapSnapshotWithEKS(2, testAWSBootstrap(), eksB)
	if err != nil {
		t.Fatal(err)
	}
	bodyB.CreationDefaults.Bootstrap = &bootstrapB
	revisionB, changed, err := AdvanceWorkspaceTemplateRevision(revisionA, bodyB)
	if err != nil || !changed || revisionA.Slices.PolicySliceDigest != revisionB.Slices.PolicySliceDigest {
		t.Fatalf("creation-only revision: changed=%t err=%v", changed, err)
	}
	template := WorkspaceTemplate{
		SchemaVersion: WorkspaceTemplateSchemaVersion, ID: testTemplateAuthorityID, Name: "restricted",
		Current: revisionB, Retained: []WorkspaceTemplateRevision{revisionA, revisionB},
	}
	contextBinding := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: testContextAuthorityID, ProjectRoot: "/workspace/example", TemplateID: testTemplateAuthorityID}
	memory, _, err := PublishPolicyMemory(contextBinding.ID, []PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt := TemplatePolicyActivationReceipt{ContextID: contextBinding.ID, TemplateID: template.ID, PolicySliceDigest: revisionA.Slices.PolicySliceDigest}
	memoryReceipt := PolicyMemoryActivationReceipt{ContextID: contextBinding.ID, Revision: memory.Revision}
	record := WorkspaceAuthorityContextRecord{Context: contextBinding, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: collectionPolicyMemoryPtr(memory), ActivePolicyMemoryRef: &memoryReceipt}
	applied := WorkspaceAppliedEntry{
		ContextID: contextBinding.ID, TemplateID: template.ID, TemplateRevision: revisionA.Revision,
		EntrySliceDigest: revisionA.Slices.EntrySliceDigest, RuntimeID: revisionA.Slices.RuntimeID,
		RuntimeRevision: revisionA.Slices.RuntimeRevision, ResolvedSpec: authorityDigest("7"), ReconciledAt: workspaceAuthorityCollectionFixture(t).Workspaces[0].LastSuccessfulEntry.ReconciledAt,
	}
	workspace := WorkspaceBinding{
		SchemaVersion: WorkspaceBindingSchemaVersion, ID: testWorkspaceAuthorityID, ContextID: contextBinding.ID,
		ProjectRoot: contextBinding.ProjectRoot, Home: "/workspace/home", CreationDefaults: revisionA.Slices.CreationDefaultsDigest,
		LastSuccessfulEntry: &applied,
	}
	collection, _, err := PublishWorkspaceAuthorityCollection([]WorkspaceTemplate{template}, []WorkspaceAuthorityContextRecord{record}, []WorkspaceBinding{workspace}, []PolicyCandidateAuthority{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	before, err := BuildClusterWorkspacePolicyProjection(collection)
	if err != nil {
		t.Fatal(err)
	}
	if got := before.Contexts[0].Principal.CreationDefaults.Bootstrap.EKS.Server; got != bootstrapA.EKS.Server {
		t.Fatalf("live Workspace adopted Template-current creation defaults: %q", got)
	}

	workspace.CreationDefaults = revisionB.Slices.CreationDefaultsDigest
	applied.TemplateRevision = revisionB.Revision
	applied.EntrySliceDigest = revisionB.Slices.EntrySliceDigest
	workspace.LastSuccessfulEntry = &applied
	afterCollection, _, err := PublishWorkspaceAuthorityCollection([]WorkspaceTemplate{template}, []WorkspaceAuthorityContextRecord{record}, []WorkspaceBinding{workspace}, []PolicyCandidateAuthority{}, nil, &collection)
	if err != nil {
		t.Fatal(err)
	}
	after, err := BuildClusterWorkspacePolicyProjection(afterCollection)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.Contexts[0].Principal.CreationDefaults.Bootstrap.EKS.Server; got != bootstrapB.EKS.Server || before.ContentDigest == after.ContentDigest {
		t.Fatalf("Workspace replacement did not adopt exact B creation authority: %q", got)
	}
}

func TestWorkspacePolicyProjectionRejectsMissingOrDriftingCompleteAuthority(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	valid, err := BuildHotWorkspacePolicyProjection(collection, collection.Contexts[0].Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*WorkspacePolicyProjection){
		"missing Context": func(value *WorkspacePolicyProjection) { value.Contexts = []WorkspacePolicyProjectionContext{} },
		"Template body drift": func(value *WorkspacePolicyProjection) {
			value.Contexts[0].TemplatePolicy.Policy.BaselineGrants[0].Path = "/drift"
		},
		"Policy Memory drift": func(value *WorkspacePolicyProjection) {
			value.Contexts[0].PolicyMemory.Generation++
		},
		"principal mismatch": func(value *WorkspacePolicyProjection) {
			value.Contexts[0].Principal.ProjectRoot = "/workspace/other"
		},
		"plan digest mismatch": func(value *WorkspacePolicyProjection) {
			value.PlanDigest = authorityDigest("9")
		},
		"mode precondition drift": func(value *WorkspacePolicyProjection) {
			value.Mode = WorkspacePolicyProjectionCluster
		},
		"collection precondition drift": func(value *WorkspacePolicyProjection) {
			value.CollectionRevision = authorityDigest("9")
		},
		"target precondition drift": func(value *WorkspacePolicyProjection) {
			other := ContextID("01912345-6789-7abc-8def-0123456789a4")
			value.TargetContextID = &other
		},
		"content digest mismatch": func(value *WorkspacePolicyProjection) {
			value.ContentDigest = authorityDigest("8")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid.Clone()
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("partial or drifting projection passed")
			}
		})
	}

	outsideBody := policyMemoryBodyFixture("/outside")
	outsideBody.Host = "outside.example.dev"
	outsideRule, err := NewPolicyMemoryRule(valid.Contexts[0].ContextID, PolicyMemoryAllow, outsideBody)
	if err != nil {
		t.Fatal(err)
	}
	outsideMemory, _, err := PublishPolicyMemory(valid.Contexts[0].ContextID, []PolicyMemoryRule{outsideRule}, nil)
	if err != nil {
		t.Fatal(err)
	}
	outside := valid.Clone()
	outside.Contexts[0].PolicyMemory = outsideMemory
	outside.Contexts[0].MemoryReceipt = PolicyMemoryActivationReceipt{ContextID: outside.Contexts[0].ContextID, Revision: outsideMemory.Revision}
	outside.ContentDigest, _ = workspacePolicyProjectionContentDigest(outside)
	outside.PlanDigest, _ = workspacePolicyProjectionPlanDigest(outside)
	if err := outside.Validate(); err == nil {
		t.Fatal("remembered rule outside the selected Template Boundary passed")
	}

	missingAxis := collection.Clone()
	missingAxis.Contexts[0].ActiveTemplatePolicy = nil
	missingAxis.Revision, _ = workspaceAuthorityCollectionRevision(missingAxis)
	if _, err := BuildHotWorkspacePolicyProjection(missingAxis, missingAxis.Contexts[0].Context.ID); err == nil {
		t.Fatal("hot projection inferred a missing active Template axis")
	}
}
