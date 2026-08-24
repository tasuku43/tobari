package workspaceauthoritystore

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const hostLoopbackPolicyDenialWindow = 10_000

type hostLoopbackPolicyRuntime interface {
	HasLiveFinalWorkspaceSession(context.Context) (bool, error)
	ReadFinalClusterDenials(context.Context, int) (tobari.DenialRead, error)
	ApplyAttachmentGrantDecisionSet(context.Context, []tobari.AttachmentGrant) (tobari.PolicyActivationReceipt, error)
}

// HostLoopbackPolicyAdapter joins attachment-local Host Loopback candidates to
// the final policy discovery task without publishing them into Policy Memory.
// Candidate application re-reads the exact live epoch and changes only the
// private attachment grant registry.
type HostLoopbackPolicyAdapter struct {
	store   *Store
	runtime hostLoopbackPolicyRuntime
}

func NewHostLoopbackPolicyAdapter(store *Store, runtime any) (*HostLoopbackPolicyAdapter, error) {
	port, ok := runtime.(hostLoopbackPolicyRuntime)
	if store == nil || !ok {
		return nil, fmt.Errorf("Host Loopback policy authority is unavailable")
	}
	return &HostLoopbackPolicyAdapter{store: store, runtime: port}, nil
}

func (a *HostLoopbackPolicyAdapter) ListPolicyCandidatesIncludingAttachments(ctx context.Context) (tobari.PolicyCandidateAuthorityList, error) {
	if a == nil || a.store == nil || a.runtime == nil {
		return tobari.PolicyCandidateAuthorityList{}, fmt.Errorf("Host Loopback policy authority is unavailable")
	}
	collection, present, err := a.store.ReadComplete(ctx)
	if err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	live, err := a.runtime.HasLiveFinalWorkspaceSession(ctx)
	if err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	if !live {
		return tobari.NewPolicyCandidateAuthorityList(collection, present)
	}
	read, err := a.runtime.ReadFinalClusterDenials(ctx, hostLoopbackPolicyDenialWindow)
	if err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	denials := make([]tobari.PolicyDenial, 0, len(read.Items))
	for _, denial := range read.Items {
		if denial.EffectiveDestinationKind() == tobari.PolicyDestinationHostLoopback {
			denials = append(denials, denial)
		}
	}
	attachments, err := tobari.PolicyCandidatesWithDenyRules(
		denials,
		[]tobari.LearnedPolicyRule{},
		tobari.PolicyDenyRuleSet{Exact: []tobari.PolicyDenyRule{}},
	)
	if err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	if err := a.store.ConfirmSelected(ctx, collection, present); err != nil {
		return tobari.PolicyCandidateAuthorityList{}, err
	}
	return tobari.NewPolicyCandidateAuthorityListWithAttachments(collection, present, attachments)
}

func (a *HostLoopbackPolicyAdapter) ApplyAttachmentPolicyCandidate(
	ctx context.Context,
	ref string,
	decision tobari.PolicyMemoryDecision,
) (tobari.AttachmentGrantPublication, bool, error) {
	if err := tobari.ValidatePolicyCandidateID(ref); err != nil {
		return tobari.AttachmentGrantPublication{}, false, err
	}
	if err := decision.Validate(); err != nil {
		return tobari.AttachmentGrantPublication{}, false, err
	}
	list, err := a.ListPolicyCandidatesIncludingAttachments(ctx)
	if err != nil {
		return tobari.AttachmentGrantPublication{}, false, err
	}
	var candidate tobari.PolicyCandidate
	found := false
	for _, item := range list.Items {
		if item.ID == ref && item.AttachmentAuthority != nil {
			candidate, found = *item.AttachmentAuthority, true
			break
		}
	}
	if !found {
		return tobari.AttachmentGrantPublication{}, false, nil
	}
	grant, err := tobari.NewAttachmentGrantFromCandidate(string(decision), candidate)
	if err != nil {
		return tobari.AttachmentGrantPublication{}, true, err
	}
	activation, err := a.runtime.ApplyAttachmentGrantDecisionSet(ctx, []tobari.AttachmentGrant{grant})
	if err != nil {
		return tobari.AttachmentGrantPublication{}, true, err
	}
	publication := tobari.AttachmentGrantPublication{Candidate: candidate, Grant: grant, Activation: activation}
	if err := publication.ValidateFor(ref, decision); err != nil {
		return tobari.AttachmentGrantPublication{}, true, err
	}
	return publication, true, nil
}
