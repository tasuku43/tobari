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
	active, err := BuildActiveWorkspacePolicyProjection(collection)
	if err != nil {
		t.Fatal(err)
	}
	if active.Mode != WorkspacePolicyProjectionActive || active.TargetContextID != nil ||
		active.Contexts[0].TemplatePolicy.PolicySliceDigest != oldTemplate.Slices.PolicySliceDigest ||
		active.Contexts[0].PolicyMemory.Revision != previousMemory.Revision {
		t.Fatalf("active projection adopted a pending axis: %#v", active)
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

func TestWorkspacePolicyProjectionProjectsAndRetiresExactSiblingPrincipal(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	sibling := base.Workspaces[0]
	sibling.ID = WorkspaceID("01912345-6789-7abc-8def-0123456789a4")
	sibling.ProjectRoot = "/workspace/sibling"
	workspaces := append(cloneWorkspaceBindings(base.Workspaces), sibling)
	collection, _, err := PublishWorkspaceAuthorityCollection(base.Templates, base.Contexts, workspaces, base.PendingCandidates, base.DefaultTemplateID, &base)
	if err != nil {
		t.Fatal(err)
	}
	active, err := BuildActiveWorkspacePolicyProjection(collection)
	if err != nil || len(active.Contexts) != 1 || len(active.Contexts[0].Principals) != 2 {
		t.Fatalf("sibling principals=%+v err=%v", active.Contexts, err)
	}
	remaining, _, err := PublishWorkspaceAuthorityCollection(base.Templates, base.Contexts, base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, &collection)
	if err != nil {
		t.Fatal(err)
	}
	retired, err := BuildWorkspaceRetirementPolicyProjection(active, remaining, sibling)
	if err != nil || len(retired.Contexts) != 1 || len(retired.Contexts[0].Principals) != 1 || retired.Contexts[0].Principals[0].WorkspaceID != base.Workspaces[0].ID {
		t.Fatalf("exact sibling retirement=%+v err=%v", retired.Contexts, err)
	}
}

func TestActiveWorkspacePolicyProjectionRepresentsEmptyAuthority(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	empty, _, err := PublishWorkspaceAuthorityCollection(
		base.Templates, []WorkspaceAuthorityContextRecord{}, []WorkspaceBinding{}, []PolicyCandidateAuthority{},
		base.DefaultTemplateID, &base,
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := BuildActiveWorkspacePolicyProjection(empty)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Mode != WorkspacePolicyProjectionActive || projection.TargetContextID != nil || len(projection.Contexts) != 0 {
		t.Fatalf("empty active projection is inconsistent: %#v", projection)
	}
}

func TestWorkspaceRetirementProjectionUsesVerifiedLiveAxesWhenDesiredReceiptsArePartial(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	active, err := BuildHotWorkspacePolicyProjection(base, base.Contexts[0].Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := base.Workspaces[0]
	contexts := cloneWorkspaceAuthorityContextRecords(base.Contexts)
	contexts[0].ActiveTemplatePolicy = nil
	contexts[0].ActivePolicyMemory = nil
	contexts[0].ActivePolicyMemoryRef = nil
	next, changed, err := PublishWorkspaceAuthorityCollection(
		base.Templates, contexts, []WorkspaceBinding{}, []PolicyCandidateAuthority{}, base.DefaultTemplateID, &base,
	)
	if err != nil || !changed {
		t.Fatalf("publish partial legacy retirement state: changed=%t err=%v", changed, err)
	}
	if _, err := BuildHotWorkspacePolicyProjection(next, workspace.ContextID); err == nil {
		t.Fatal("partial legacy authority unexpectedly built an ordinary hot projection")
	}

	retirement, err := BuildWorkspaceRetirementPolicyProjection(active, next, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if retirement.Mode != WorkspacePolicyProjectionActive || retirement.CollectionRevision != next.Revision ||
		len(retirement.Contexts) != len(active.Contexts) || len(retirement.Contexts[0].Principals) != 0 ||
		retirement.Contexts[0].TemplatePolicy.PolicySliceDigest != active.Contexts[0].TemplatePolicy.PolicySliceDigest ||
		retirement.Contexts[0].PolicyMemory.Revision != active.Contexts[0].PolicyMemory.Revision {
		t.Fatalf("retirement projection did not preserve live axes and remove only its principal: active=%+v retirement=%+v", active, retirement)
	}
	replayed, err := BuildWorkspaceRetirementPolicyProjection(retirement, next, workspace)
	if err != nil || !reflect.DeepEqual(replayed, retirement) {
		t.Fatalf("settled retirement projection did not replay exactly: replayed=%+v err=%v", replayed, err)
	}
}

func TestWorkspaceRetirementProjectionRejectsRetainedOrReboundTarget(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	active, err := BuildHotWorkspacePolicyProjection(base, base.Contexts[0].Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	workspace := base.Workspaces[0]
	if _, err := BuildWorkspaceRetirementPolicyProjection(active, base, workspace); err == nil {
		t.Fatal("retirement projection accepted a still-bound Workspace")
	}
	next, _, err := PublishWorkspaceAuthorityCollection(
		base.Templates, base.Contexts, []WorkspaceBinding{}, []PolicyCandidateAuthority{}, base.DefaultTemplateID, &base,
	)
	if err != nil {
		t.Fatal(err)
	}
	rebound := active.Clone()
	rebound.Contexts[0].Principals[0].ProjectRoot = "/foreign"
	rebound.ContentDigest, _ = workspacePolicyProjectionContentDigest(rebound)
	rebound.PlanDigest, _ = workspacePolicyProjectionPlanDigest(rebound)
	if _, err := BuildWorkspaceRetirementPolicyProjection(rebound, next, workspace); err == nil {
		t.Fatal("retirement projection accepted a rebound principal")
	}
}

func TestWorkspacePolicyProjectionPreservesFullyInactiveContexts(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	inactiveID := ContextID("01912345-6789-7abc-8def-0123456789a4")
	inactiveMemory, _, err := PublishPolicyMemory(inactiveID, []PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	inactive := WorkspaceAuthorityContextRecord{
		Context:      ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: inactiveID, TemplateID: base.Templates[0].ID},
		PolicyMemory: inactiveMemory,
	}
	contexts := append(cloneWorkspaceAuthorityContextRecords(base.Contexts), inactive)
	collection, _, err := PublishWorkspaceAuthorityCollection(
		base.Templates, contexts, base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, &base,
	)
	if err != nil {
		t.Fatal(err)
	}
	hot, err := BuildHotWorkspacePolicyProjection(collection, base.Contexts[0].Context.ID)
	if err != nil || len(hot.Contexts) != 1 || hot.Contexts[0].ContextID != base.Contexts[0].Context.ID {
		t.Fatalf("hot active Context with inactive sibling: %#v err=%v", hot, err)
	}
	active, err := BuildActiveWorkspacePolicyProjection(collection)
	if err != nil || len(active.Contexts) != 1 || active.Contexts[0].ContextID != base.Contexts[0].Context.ID {
		t.Fatalf("complete active authority included inactive sibling: %#v err=%v", active, err)
	}
	withoutInactive, _, err := PublishWorkspaceAuthorityCollection(
		base.Templates, base.Contexts, base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, &collection,
	)
	if err != nil {
		t.Fatal(err)
	}
	afterInactiveDelete, err := BuildActiveWorkspacePolicyProjection(withoutInactive)
	if err != nil || afterInactiveDelete.ContentDigest != active.ContentDigest || afterInactiveDelete.PlanDigest == active.PlanDigest {
		t.Fatalf("inactive Context deletion changed live content or lost plan identity: before=%#v after=%#v err=%v", active, afterInactiveDelete, err)
	}
	cluster, err := BuildClusterWorkspacePolicyProjection(collection)
	if err != nil || len(cluster.Contexts) != 2 {
		t.Fatalf("cluster reconciliation did not adopt both desired axes: %#v err=%v", cluster, err)
	}

	afterDelete, _, err := PublishWorkspaceAuthorityCollection(
		base.Templates, []WorkspaceAuthorityContextRecord{inactive}, []WorkspaceBinding{}, []PolicyCandidateAuthority{},
		base.DefaultTemplateID, &collection,
	)
	if err != nil {
		t.Fatal(err)
	}
	afterActive, err := BuildActiveWorkspacePolicyProjection(afterDelete)
	if err != nil || len(afterActive.Contexts) != 0 {
		t.Fatalf("deleting the active Context adopted its inactive sibling: %#v err=%v", afterActive, err)
	}

	partial := collection.Clone()
	receipt := TemplatePolicyActivationReceipt{ContextID: inactiveID, TemplateID: base.Templates[0].ID, PolicySliceDigest: base.Templates[0].Current.Slices.PolicySliceDigest}
	partial.Contexts[1].ActiveTemplatePolicy = &receipt
	partial.Revision, _ = workspaceAuthorityCollectionRevision(partial)
	if _, err := BuildActiveWorkspacePolicyProjection(partial); err == nil {
		t.Fatal("partial active Context authority was accepted")
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
	if len(projection.Contexts) != 1 || len(projection.Contexts[0].Principals) != 0 {
		t.Fatalf("Context without Workspace produced a principal: %#v", projection.Contexts)
	}
	clone := projection.Clone()
	clone.Contexts[0].TemplatePolicy.Policy.BaselineGrants[0].Path = "/mutated"
	clone.Contexts[0].PolicyMemory.Rules = append(clone.Contexts[0].PolicyMemory.Rules, PolicyMemoryRule{})
	if reflect.DeepEqual(clone, projection) || projection.Contexts[0].TemplatePolicy.Policy.BaselineGrants[0].Path == "/mutated" || len(projection.Contexts[0].PolicyMemory.Rules) != 0 {
		t.Fatal("projection clone shares nested authority")
	}
}

func TestWorkspacePolicyProjectionClonePreservesPlanIdentityForEveryMode(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	hot, err := BuildHotWorkspacePolicyProjection(collection, collection.Contexts[0].Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := BuildClusterWorkspacePolicyProjection(collection)
	if err != nil {
		t.Fatal(err)
	}
	active, err := BuildActiveWorkspacePolicyProjection(collection)
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := BuildReviewedWorkspacePolicyProjection(collection, []ContextID{collection.Contexts[0].Context.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range []WorkspacePolicyProjection{hot, cluster, active, reviewed} {
		clone := projection.Clone()
		if err := clone.Validate(); err != nil || clone.PlanDigest != projection.PlanDigest || clone.ContentDigest != projection.ContentDigest ||
			(projection.TargetContextIDs == nil) != (clone.TargetContextIDs == nil) {
			t.Fatalf("mode=%q clone=%#v err=%v", projection.Mode, clone, err)
		}
	}
	noncanonical := hot.Clone()
	noncanonical.TargetContextIDs = []ContextID{}
	if err := noncanonical.Validate(); err == nil {
		t.Fatal("hot projection accepted a noncanonical empty reviewed-target slice")
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
	contextBinding := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: testContextAuthorityID, TemplateID: testTemplateAuthorityID}
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
		ProjectRoot: "/workspace/example", Home: "/workspace/home", CreationDefaults: revisionA.Slices.CreationDefaultsDigest,
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
	if got := before.Contexts[0].Principals[0].CreationDefaults.Bootstrap.EKS.Server; got != bootstrapA.EKS.Server {
		t.Fatalf("live Workspace adopted Template-current creation defaults: %q", got)
	}

	workspace.CreationDefaults = revisionB.Slices.CreationDefaultsDigest
	applied.TemplateRevision = revisionB.Revision
	applied.EntrySliceDigest = revisionB.Slices.EntrySliceDigest
	workspace.LastSuccessfulEntry = &applied
	afterCollection, _, err := PublishWorkspaceAuthorityCollection([]WorkspaceTemplate{template}, []WorkspaceAuthorityContextRecord{record}, []WorkspaceBinding{workspace}, []PolicyCandidateAuthority{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := BuildClusterWorkspacePolicyProjection(afterCollection)
	if err != nil {
		t.Fatal(err)
	}
	if got := after.Contexts[0].Principals[0].CreationDefaults.Bootstrap.EKS.Server; got != bootstrapB.EKS.Server || before.ContentDigest == after.ContentDigest {
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
			value.Contexts[0].Principals[0].ProjectRoot = "/workspace/other"
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
