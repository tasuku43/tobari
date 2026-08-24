package tobari

import "fmt"

// PublishWorkspaceEntryAuthority derives one complete final envelope from the
// current Context authority and one exact runtime entry plan. Entry activates
// the Context's current Template-policy and Policy-Memory axes in the same
// publication as its AppliedEntry; it never requires a separate public
// cluster reconciliation command.
func PublishWorkspaceEntryAuthority(previous WorkspaceAuthorityCollection, plan WorkspaceEntryReconciliationPlan) (WorkspaceAuthorityCollection, bool, error) {
	if err := previous.Validate(); err != nil {
		return WorkspaceAuthorityCollection{}, false, err
	}
	snapshots, err := previous.ContextSnapshots()
	if err != nil {
		return WorkspaceAuthorityCollection{}, false, err
	}
	var snapshot *ContextAuthoritySnapshot
	for index := range snapshots {
		if snapshots[index].Context.ID == plan.Workspace.ContextID {
			value := snapshots[index].Clone()
			snapshot = &value
			break
		}
	}
	if snapshot == nil {
		return WorkspaceAuthorityCollection{}, false, fmt.Errorf("Workspace entry Context authority is unavailable")
	}
	if err := plan.ValidateFor(*snapshot); err != nil {
		return WorkspaceAuthorityCollection{}, false, err
	}

	contexts := make([]WorkspaceAuthorityContextRecord, len(previous.Contexts))
	foundContext := false
	for index := range previous.Contexts {
		contexts[index] = previous.Contexts[index].Clone()
		if contexts[index].Context.ID != plan.Workspace.ContextID {
			continue
		}
		foundContext = true
		templateReceipt := TemplatePolicyActivationReceipt{
			ContextID: contexts[index].Context.ID, TemplateID: snapshot.Template.ID,
			PolicySliceDigest: snapshot.Template.Current.Slices.PolicySliceDigest,
		}
		if err := templateReceipt.ValidateFor(contexts[index].Context, snapshot.Template.Current); err != nil {
			return WorkspaceAuthorityCollection{}, false, err
		}
		memory := contexts[index].PolicyMemory.Clone()
		memoryReceipt := PolicyMemoryActivationReceipt{ContextID: contexts[index].Context.ID, Revision: memory.Revision}
		if err := memoryReceipt.ValidateFor(contexts[index].Context, memory); err != nil {
			return WorkspaceAuthorityCollection{}, false, err
		}
		contexts[index].ActiveTemplatePolicy = &templateReceipt
		contexts[index].ActivePolicyMemory = &memory
		contexts[index].ActivePolicyMemoryRef = &memoryReceipt
	}
	if !foundContext {
		return WorkspaceAuthorityCollection{}, false, fmt.Errorf("Workspace entry Context record is unavailable")
	}

	workspaces := make([]WorkspaceBinding, len(previous.Workspaces))
	copy(workspaces, previous.Workspaces)
	replaced := false
	for index := range workspaces {
		if workspaces[index].ContextID == plan.Workspace.ContextID {
			workspaces[index] = plan.Workspace
			replaced = true
			break
		}
	}
	if !replaced {
		workspaces = append(workspaces, plan.Workspace)
	}
	next, changed, err := PublishWorkspaceAuthorityCollection(
		previous.Templates, contexts, workspaces, previous.PendingCandidates, previous.DefaultTemplateID, &previous,
	)
	if err != nil {
		return WorkspaceAuthorityCollection{}, false, err
	}
	nextSnapshots, err := next.ContextSnapshots()
	if err != nil {
		return WorkspaceAuthorityCollection{}, false, err
	}
	for _, current := range nextSnapshots {
		if current.Context.ID != plan.Workspace.ContextID {
			continue
		}
		if err := plan.ValidateFor(current); err != nil {
			return WorkspaceAuthorityCollection{}, false, err
		}
		if current.ActiveTemplatePolicy == nil || current.ActiveTemplatePolicy.PolicySliceDigest != current.Template.Current.Slices.PolicySliceDigest ||
			current.ActivePolicyMemory == nil || current.ActivePolicyMemoryRef == nil || current.ActivePolicyMemory.Revision != current.PolicyMemory.Revision || current.ActivePolicyMemoryRef.Revision != current.PolicyMemory.Revision ||
			current.Workspace == nil || current.Workspace.LastSuccessfulEntry == nil || *current.Workspace.LastSuccessfulEntry != plan.Applied {
			return WorkspaceAuthorityCollection{}, false, fmt.Errorf("Workspace entry publication is incomplete")
		}
		return next, changed, nil
	}
	return WorkspaceAuthorityCollection{}, false, fmt.Errorf("Workspace entry publication lost its Context")
}
