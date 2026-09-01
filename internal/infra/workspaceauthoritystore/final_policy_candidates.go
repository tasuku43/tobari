package workspaceauthoritystore

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// finalPolicyCandidateRuntime is the read-only evidence boundary for current
// Gateway denials. It never receives credentials or a mutation callback.
type finalPolicyCandidateRuntime interface {
	HasLiveFinalWorkspaceSession(context.Context) (bool, error)
	ReadFinalClusterDenials(context.Context, int) (tobari.DenialRead, error)
}

// FinalPolicyCandidateAdapter joins durable final candidates with current
// denial evidence. Ordinary candidates remain persistent Policy Memory
// actions; Host Loopback candidates are delegated to their attachment-owned
// grant registry.
type FinalPolicyCandidateAdapter struct {
	store      *Store
	runtime    finalPolicyCandidateRuntime
	mutator    *Mutator
	attachment *HostLoopbackPolicyAdapter
}

func NewFinalPolicyCandidateAdapter(
	store *Store, runtime any, mutator *Mutator, attachment *HostLoopbackPolicyAdapter,
) (*FinalPolicyCandidateAdapter, error) {
	port, ok := runtime.(finalPolicyCandidateRuntime)
	if store == nil || !ok || mutator == nil || attachment == nil {
		return nil, fmt.Errorf("final Policy candidate authority is unavailable")
	}
	adapter := &FinalPolicyCandidateAdapter{store: store, runtime: port, mutator: mutator, attachment: attachment}
	mutator.bindPolicyCandidateObservation(adapter)
	return adapter, nil
}

func (a *FinalPolicyCandidateAdapter) ListPolicyCandidatesIncludingAttachments(ctx context.Context) (tobari.PolicyCandidateAuthorityList, error) {
	if a == nil || a.store == nil || a.runtime == nil {
		return tobari.PolicyCandidateAuthorityList{}, fmt.Errorf("final Policy candidate authority is unavailable")
	}
	collection, present, err := a.store.ReadComplete(ctx)
	if err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	read, err := a.runtime.ReadFinalClusterDenials(ctx, hostLoopbackPolicyDenialWindow)
	if err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	allows, denyRules, err := finalPolicyMemoryInputs(collection)
	if err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	persistentDenials, attachmentDenials := finalCurrentDenials(collection, read.Items)
	if len(attachmentDenials) > 0 {
		live, err := a.runtime.HasLiveFinalWorkspaceSession(ctx)
		if err != nil {
			return tobari.PolicyCandidateAuthorityList{}, err
		}
		if !live {
			// Ordinary external denials belong to the current final Workspace
			// even when no interactive attachment owner is present. Only Host
			// Loopback evidence requires the separate live attachment session
			// authority.
			attachmentDenials = nil
		}
	}
	persistent, err := tobari.PolicyCandidatesWithDenyRules(persistentDenials, allows, denyRules)
	if err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	observed := make([]tobari.PolicyCandidateAuthority, 0, len(persistent))
	for _, candidate := range persistent {
		contextID, err := tobari.ParseContextID(candidate.WorkspaceManifestID)
		if err != nil {
			return tobari.PolicyCandidateAuthorityList{}, err
		}
		workspaceID, err := tobari.ParseWorkspaceID(candidate.ProjectID)
		if err != nil {
			return tobari.PolicyCandidateAuthorityList{}, err
		}
		effect := tobari.PolicyCandidateEffect{
			PolicyProtocolIdentity: candidate.PolicyProtocolIdentity,
			Match:                  tobari.PolicyMatchExact,
			Host:                   candidate.Host,
			Port:                   candidate.Port,
			Method:                 candidate.Method,
			Path:                   candidate.Path,
			Segments:               []string{},
			Examples:               []string{candidate.Path},
		}
		authority, err := tobari.NewPolicyCandidateAuthority(contextID, workspaceID, effect)
		if err != nil {
			return tobari.PolicyCandidateAuthorityList{}, err
		}
		observed = append(observed, authority)
	}
	attachments, err := tobari.PolicyCandidatesWithDenyRules(
		attachmentDenials, []tobari.LearnedPolicyRule{}, tobari.PolicyDenyRuleSet{Exact: []tobari.PolicyDenyRule{}},
	)
	if err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	if err := a.store.ConfirmSelected(ctx, collection, present); err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	return tobari.NewPolicyCandidateAuthorityListWithObservations(collection, present, observed, attachments)
}

// ReadPolicyMemoryReviewSnapshot joins the complete final collection and the
// bounded live denial observations only when both reads retain the same exact
// collection receipt.
func (a *FinalPolicyCandidateAdapter) ReadPolicyMemoryReviewSnapshot(ctx context.Context) (tobari.PolicyMemoryReviewSnapshot, error) {
	if a == nil || a.store == nil {
		return tobari.PolicyMemoryReviewSnapshot{}, fmt.Errorf("final Policy candidate authority is unavailable")
	}
	candidates, err := a.ListPolicyCandidatesIncludingAttachments(ctx)
	if err != nil {
		return tobari.PolicyMemoryReviewSnapshot{}, err
	}
	snapshot, err := a.store.ReadPolicyMemoryReviewSnapshot(ctx)
	if err != nil {
		return tobari.PolicyMemoryReviewSnapshot{}, err
	}
	return tobari.JoinPolicyMemoryReviewCandidates(snapshot, candidates)
}

func (a *FinalPolicyCandidateAdapter) ApplyAttachmentPolicyCandidate(
	ctx context.Context, ref string, decision tobari.PolicyMemoryDecision,
) (tobari.AttachmentGrantPublication, bool, error) {
	return a.attachment.ApplyAttachmentPolicyCandidate(ctx, ref, decision)
}

func (a *FinalPolicyCandidateAdapter) AllowPolicyCandidateByReference(ctx context.Context, ref string) (tobari.PolicyCandidatePublication, error) {
	return a.mutator.AllowPolicyCandidateByReference(ctx, ref)
}

func (a *FinalPolicyCandidateAdapter) DenyPolicyCandidateByReference(ctx context.Context, ref string) (tobari.PolicyCandidatePublication, error) {
	return a.mutator.DenyPolicyCandidateByReference(ctx, ref)
}

// ResetPolicyMemoryRuleByReference exposes the existing final-authority rule
// mutation through the same adapter that owns candidate actions. Keeping this
// forwarding method here ensures the composed CLI port remains complete when
// the application service discovers its PolicyRulePort.
func (a *FinalPolicyCandidateAdapter) ResetPolicyMemoryRuleByReference(ctx context.Context, ref string) (tobari.PolicyRuleResetPublication, error) {
	return a.mutator.ResetPolicyMemoryRuleByReference(ctx, ref)
}

// ApplyReviewedPolicyMemory exposes the one complete reviewed-set mutation
// through the final-authority adapter. The Mutator remains the sole owner of
// lifecycle locking, settlement, publication, and recovery.
func (a *FinalPolicyCandidateAdapter) ApplyReviewedPolicyMemory(
	ctx context.Context, set tobari.PolicyMemoryReviewedDecisionSet,
) (tobari.PolicyMemoryReviewedSetPublication, error) {
	return a.mutator.ApplyReviewedPolicyMemory(ctx, set)
}

func finalCurrentDenials(
	collection tobari.WorkspaceAuthorityCollection, denials []tobari.PolicyDenial,
) ([]tobari.PolicyDenial, []tobari.PolicyDenial) {
	contexts := make(map[tobari.ContextID]tobari.WorkspaceAuthorityContextRecord, len(collection.Contexts))
	templates := make(map[tobari.WorkspaceTemplateID]string, len(collection.Templates))
	workspaces := make(map[tobari.WorkspaceID]tobari.WorkspaceBinding, len(collection.Workspaces))
	for _, record := range collection.Contexts {
		contexts[record.Context.ID] = record
	}
	for _, template := range collection.Templates {
		templates[template.ID] = template.Name
	}
	for _, workspace := range collection.Workspaces {
		workspaces[workspace.ID] = workspace
	}
	persistent := make([]tobari.PolicyDenial, 0, len(denials))
	attachments := make([]tobari.PolicyDenial, 0, len(denials))
	for _, denial := range denials {
		contextID, contextErr := tobari.ParseContextID(denial.WorkspaceManifestID)
		workspaceID, workspaceErr := tobari.ParseWorkspaceID(denial.ProjectID)
		record, contextFound := contexts[contextID]
		workspace, workspaceFound := workspaces[workspaceID]
		templateName := templates[record.Context.TemplateID]
		if contextErr != nil || workspaceErr != nil || !contextFound || !workspaceFound ||
			workspace.ContextID != contextID ||
			workspace.ProjectRoot != denial.ProjectRoot || templateName != denial.WorkspaceManifestName {
			continue
		}
		if denial.EffectiveDestinationKind() == tobari.PolicyDestinationHostLoopback {
			attachments = append(attachments, denial)
		} else {
			persistent = append(persistent, denial)
		}
	}
	return persistent, attachments
}

func finalPolicyMemoryInputs(
	collection tobari.WorkspaceAuthorityCollection,
) ([]tobari.LearnedPolicyRule, tobari.PolicyDenyRuleSet, error) {
	templates := make(map[tobari.WorkspaceTemplateID]string, len(collection.Templates))
	for _, template := range collection.Templates {
		templates[template.ID] = template.Name
	}
	workspaces := make(map[tobari.ContextID]tobari.WorkspaceBinding, len(collection.Workspaces))
	for _, workspace := range collection.Workspaces {
		workspaces[workspace.ContextID] = workspace
	}
	allows := make([]tobari.LearnedPolicyRule, 0)
	denies := make([]tobari.PolicyDenyRule, 0)
	for _, record := range collection.Contexts {
		workspace, present := workspaces[record.Context.ID]
		if !present {
			continue
		}
		name := templates[record.Context.TemplateID]
		for _, rule := range record.PolicyMemory.Rules {
			switch rule.Decision {
			case tobari.PolicyMemoryAllow:
				converted, err := tobari.NewLearnedPolicyRuleFromPolicyMemory(
					record.Context.ID, name, workspace.ID, workspace.ProjectRoot, rule,
				)
				if err != nil {
					return nil, tobari.PolicyDenyRuleSet{}, err
				}
				allows = append(allows, converted)
			case tobari.PolicyMemoryDeny:
				converted, err := tobari.NewPolicyDenyRuleFromPolicyMemory(
					record.Context.ID, name, workspace.ID, workspace.ProjectRoot, rule,
				)
				if err != nil {
					return nil, tobari.PolicyDenyRuleSet{}, err
				}
				denies = append(denies, converted)
			default:
				return nil, tobari.PolicyDenyRuleSet{}, fmt.Errorf("Policy Memory decision is invalid")
			}
		}
	}
	return allows, tobari.PolicyDenyRuleSet{Exact: denies}, nil
}
