package workspaceauthorityresources

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritysource"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

func (a *Adapter) CreateWorkspaceTemplateDraft(ctx context.Context, name string, body tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplateDraft, error) {
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	defer releaseCatalog()
	if err := a.requireNoConfiguratorPublicationsLocked(ctx); err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	id, err := tobari.IssueWorkspaceTemplateID(time.Now().UTC(), rand.Reader)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	return a.createWorkspaceTemplateDraftWithID(ctx, id, name, body)
}

func (a *Adapter) createWorkspaceTemplateDraftWithID(ctx context.Context, id tobari.WorkspaceTemplateID, name string, body tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplateDraft, error) {
	if err := tobari.ValidateName(name); err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	if err := body.Validate(); err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	if err := id.Validate(); err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	ids, err := a.sources.ListTemplateIDs(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	for _, existingID := range ids {
		if existingID == id {
			source, present, readErr := a.sources.ReadTemplate(ctx, existingID)
			if readErr != nil || !present || source.Template.Name != name {
				return tobari.WorkspaceTemplateDraft{}, errors.Join(tobari.ErrResourceSourceRecoveryRequired, readErr)
			}
			resolved, resolveErr := a.mutator.ResolveWorkspaceTemplateRuntimeSource(ctx, source.Template.EntryDefaults.Runtime)
			if resolveErr != nil {
				return tobari.WorkspaceTemplateDraft{}, resolveErr
			}
			existingBody, bodyErr := source.Body(resolved)
			revision, revisionErr := source.SemanticRevision(resolved)
			if bodyErr != nil || revisionErr != nil || !reflect.DeepEqual(existingBody, body) {
				return tobari.WorkspaceTemplateDraft{}, errors.Join(tobari.ErrResourceSourceRecoveryRequired, bodyErr, revisionErr)
			}
			path, _ := a.sources.TemplatePath(id)
			return tobari.WorkspaceTemplateDraft{ID: id, Name: name, Body: existingBody, Source: tobari.ResourceSourceObservation{Path: path, State: tobari.ResourceSourceModified, SourceRevision: &revision}}, nil
		}
		source, present, readErr := a.sources.ReadTemplate(ctx, existingID)
		if readErr != nil {
			return tobari.WorkspaceTemplateDraft{}, readErr
		}
		if present && source.Template.Name == name {
			return tobari.WorkspaceTemplateDraft{}, tobari.ErrWorkspaceTemplateExists
		}
	}
	source, err := tobari.NewWorkspaceTemplateDraftSource(id, name, body)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	if err := a.sources.PublishTemplate(ctx, source); err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	path, _ := a.sources.TemplatePath(id)
	revision, _ := source.SemanticRevision(body.EntryDefaults.Runtime)
	return tobari.WorkspaceTemplateDraft{ID: id, Name: name, Body: body.Clone(), Source: tobari.ResourceSourceObservation{Path: path, State: tobari.ResourceSourceModified, SourceRevision: &revision}}, nil
}

// StageConfiguratorSubmission replaces only canonical desired source after the
// host has frozen and reviewed one evolve submission. Active authority is not
// changed here; the normal Template Plan/Apply path consumes this source.
func (a *Adapter) StageConfiguratorSubmission(ctx context.Context, submission tobari.ConfiguratorSubmission) (tobari.ConfiguratorStage, error) {
	if err := submission.Validate(); err != nil || submission.Draft.Purpose != tobari.ConfiguratorPurposeEvolve {
		return tobari.ConfiguratorStage{}, fmt.Errorf("Configurator submission is invalid: %w", err)
	}
	draft := submission.Draft
	releaseProject, err := a.acquireConfiguratorDraftStageLease(ctx, draft)
	if err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	defer releaseProject()
	projectReceipt, projectStagePresent, err := a.configuratorStageForDraftLocked(ctx, draft)
	if err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	if projectStagePresent && (projectReceipt.Submission.Draft.TemplateID != draft.TemplateID || !reflect.DeepEqual(projectReceipt.Submission, submission)) {
		return tobari.ConfiguratorStage{}, tobari.ErrResourceSourceRecoveryRequired
	}
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, draft.TemplateID)
	if err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	defer releaseStage()
	var template tobari.WorkspaceTemplate
	if draft.ContextID != "" {
		contextRef, err := tobari.ContextRef(draft.ContextID)
		if err != nil {
			return tobari.ConfiguratorStage{}, err
		}
		snapshot, err := a.Store.ReadContextAuthorityByReference(ctx, contextRef)
		if err != nil {
			return tobari.ConfiguratorStage{}, err
		}
		if snapshot.Context.TemplateID != draft.TemplateID || snapshot.Template.Current.Revision != draft.BaseTemplateRevision || snapshot.PolicyMemory.Revision != draft.BasePolicyMemoryRevision {
			return tobari.ConfiguratorStage{}, tobari.ErrWorkspaceTemplateChangePlanStale
		}
		template = snapshot.Template.Clone()
	} else {
		collection, present, err := a.Store.ReadComplete(ctx)
		if err != nil || !present || collection.DefaultTemplateID == nil || *collection.DefaultTemplateID != draft.TemplateID {
			return tobari.ConfiguratorStage{}, errors.Join(tobari.ErrWorkspaceTemplateChangePlanStale, err)
		}
		for _, candidate := range collection.Templates {
			if candidate.ID == draft.TemplateID {
				template = candidate.Clone()
				break
			}
		}
		if template.ID == "" || template.Current.Revision != draft.BaseTemplateRevision {
			return tobari.ConfiguratorStage{}, tobari.ErrWorkspaceTemplateChangePlanStale
		}
	}
	existing, staged, err := a.sources.ReadConfiguratorStage(ctx, draft.TemplateID)
	if err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	if staged && template.Current.Revision == existing.Submission.SourceRevision {
		observation, observeErr := a.ObserveWorkspaceTemplateSource(ctx, template)
		if observeErr != nil || observation.State != tobari.ResourceSourceInSync {
			return tobari.ConfiguratorStage{}, errors.Join(tobari.ErrResourceSourceRecoveryRequired, observeErr)
		}
		if err := a.sources.ClearConfiguratorStage(ctx, existing); err != nil {
			return tobari.ConfiguratorStage{}, err
		}
		staged = false
	}
	if staged && !reflect.DeepEqual(existing.Submission, submission) {
		return tobari.ConfiguratorStage{}, tobari.ErrResourceSourceRecoveryRequired
	}
	observation, err := a.ObserveWorkspaceTemplateSource(ctx, template)
	if err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	if !staged && observation.State != tobari.ResourceSourceInSync {
		return tobari.ConfiguratorStage{}, tobari.ErrResourceSourceModified
	}
	_, fingerprint, present, err := a.sources.ReadTemplateSnapshot(ctx, draft.TemplateID)
	if err != nil || !present {
		return tobari.ConfiguratorStage{}, errors.Join(tobari.ErrResourceSourceMissing, err)
	}
	source, err := tobari.NewWorkspaceTemplateDraftSource(draft.TemplateID, template.Name, submission.Body)
	if err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	base := draft.BaseTemplateRevision
	source.Template.BaseRevision = &base
	if err := source.Validate(); err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	resolved, err := a.mutator.ResolveWorkspaceTemplateRuntimeSourceWithRetainedBinding(
		ctx,
		source.Template.EntryDefaults.Runtime,
		template.Current.Body.EntryDefaults.Runtime,
	)
	if err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	revision, err := source.SemanticRevision(resolved)
	if err != nil || revision != submission.SourceRevision {
		return tobari.ConfiguratorStage{}, errors.Join(tobari.ErrResourceSourceModified, err)
	}
	stagedFingerprint, err := a.sources.ConfiguratorTemplateFingerprint(source)
	if err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	templateRef, _ := tobari.WorkspaceTemplateRef(draft.TemplateID)
	result := tobari.ConfiguratorStage{SchemaVersion: tobari.ConfiguratorStageSchemaVersion, TemplateRef: templateRef, SourceRevision: revision, SourceFingerprint: stagedFingerprint}
	baseFingerprint := fingerprint
	if staged {
		baseFingerprint = existing.BaseFingerprint
	}
	receipt := workspaceauthoritysource.ConfiguratorStageReceipt{Submission: submission, Stage: result, BaseFingerprint: baseFingerprint}
	if staged {
		if !reflect.DeepEqual(existing.Submission, receipt.Submission) || existing.Stage != receipt.Stage || existing.BaseFingerprint != receipt.BaseFingerprint {
			return tobari.ConfiguratorStage{}, tobari.ErrResourceSourceRecoveryRequired
		}
		if fingerprint == stagedFingerprint {
			return result, result.ValidateFor(submission)
		}
		if fingerprint != existing.BaseFingerprint {
			return tobari.ConfiguratorStage{}, tobari.ErrResourceSourceChanged
		}
	} else if err := a.sources.BeginConfiguratorStage(ctx, receipt); err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	if err := a.sources.ReplaceTemplateSnapshot(ctx, source, fingerprint); err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	return result, result.ValidateFor(submission)
}

// DiscardConfiguratorStage restores the exact active Template source only
// while the staged bytes and active base still match the reviewed receipt.
func (a *Adapter) DiscardConfiguratorStage(ctx context.Context, submission tobari.ConfiguratorSubmission, stage tobari.ConfiguratorStage) error {
	if err := stage.ValidateFor(submission); err != nil {
		return err
	}
	releaseProject, err := a.acquireConfiguratorDraftStageLease(ctx, submission.Draft)
	if err != nil {
		return err
	}
	defer releaseProject()
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, submission.Draft.TemplateID)
	if err != nil {
		return err
	}
	defer releaseStage()
	collection, present, err := a.Store.ReadComplete(ctx)
	if err != nil || !present {
		return errors.Join(tobari.ErrWorkspaceTemplateChangePlanStale, err)
	}
	var template tobari.WorkspaceTemplate
	for _, candidate := range collection.Templates {
		if candidate.ID == submission.Draft.TemplateID {
			template = candidate.Clone()
			break
		}
	}
	if template.ID == "" || template.Current.Revision != submission.Draft.BaseTemplateRevision {
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	receipt, present, err := a.sources.ReadConfiguratorStage(ctx, template.ID)
	if err != nil {
		return err
	}
	if !present {
		observation, observeErr := a.ObserveWorkspaceTemplateSource(ctx, template)
		if observeErr == nil && observation.State == tobari.ResourceSourceInSync {
			return nil
		}
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, observeErr)
	}
	if !reflect.DeepEqual(receipt.Submission, submission) || receipt.Stage != stage {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	_, fingerprint, sourcePresent, err := a.sources.ReadTemplateSnapshot(ctx, template.ID)
	if err != nil || !sourcePresent {
		return errors.Join(tobari.ErrResourceSourceChanged, err)
	}
	if fingerprint == receipt.BaseFingerprint {
		if err := a.sources.ClearConfiguratorStage(ctx, receipt); err != nil {
			return err
		}
		observation, observeErr := a.ObserveWorkspaceTemplateSource(ctx, template)
		if observeErr != nil || observation.State != tobari.ResourceSourceInSync {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, observeErr)
		}
		return nil
	}
	if fingerprint != stage.SourceFingerprint {
		return tobari.ErrResourceSourceChanged
	}
	source, err := tobari.NewWorkspaceTemplateDraftSource(template.ID, template.Name, template.Current.Body)
	if err != nil {
		return err
	}
	base := template.Current.Revision
	source.Template.BaseRevision = &base
	if err := a.sources.ReplaceTemplateSnapshot(ctx, source, fingerprint); err != nil {
		return err
	}
	if err := a.sources.ClearConfiguratorStage(ctx, receipt); err != nil {
		return err
	}
	observation, err := a.ObserveWorkspaceTemplateSource(ctx, template)
	if err != nil || observation.State != tobari.ResourceSourceInSync {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	return nil
}

func (a *Adapter) PendingConfiguratorStage(ctx context.Context, id tobari.WorkspaceTemplateID) (tobari.ConfiguratorPendingStage, bool, error) {
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, id)
	if err != nil {
		return tobari.ConfiguratorPendingStage{}, false, err
	}
	defer releaseStage()
	receipt, present, err := a.sources.ReadConfiguratorStage(ctx, id)
	if err != nil || !present {
		return tobari.ConfiguratorPendingStage{}, false, err
	}
	collection, authorityPresent, err := a.Store.ReadComplete(ctx)
	if err != nil || !authorityPresent {
		return tobari.ConfiguratorPendingStage{}, false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	var template tobari.WorkspaceTemplate
	for _, candidate := range collection.Templates {
		if candidate.ID == id {
			template = candidate.Clone()
			break
		}
	}
	if template.ID == "" {
		return tobari.ConfiguratorPendingStage{}, false, tobari.ErrResourceSourceRecoveryRequired
	}
	if template.Current.Revision != receipt.Submission.Draft.BaseTemplateRevision && template.Current.Revision != receipt.Submission.SourceRevision {
		return tobari.ConfiguratorPendingStage{}, false, tobari.ErrResourceSourceRecoveryRequired
	}
	if template.Current.Revision == receipt.Submission.SourceRevision && !receipt.ApplyConfirmed {
		observation, observeErr := a.ObserveWorkspaceTemplateSource(ctx, template)
		if observeErr != nil || observation.State != tobari.ResourceSourceInSync {
			return tobari.ConfiguratorPendingStage{}, false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, observeErr)
		}
		if err := a.sources.ClearConfiguratorStage(ctx, receipt); err != nil {
			return tobari.ConfiguratorPendingStage{}, false, err
		}
		return tobari.ConfiguratorPendingStage{}, false, nil
	}
	pending := tobari.ConfiguratorPendingStage{Submission: receipt.Submission, Stage: receipt.Stage, PlanRef: receipt.PlanRef, ApplyConfirmed: receipt.ApplyConfirmed}
	return pending, true, pending.Validate()
}

func (a *Adapter) PendingConfiguratorStageForProject(ctx context.Context, projectRoot string) (tobari.ConfiguratorPendingStage, bool, error) {
	if err := tobari.ValidateCanonicalRoot(projectRoot); err != nil {
		return tobari.ConfiguratorPendingStage{}, false, err
	}
	releaseProject, err := a.sources.AcquireConfiguratorProjectStageLease(ctx, projectRoot)
	if err != nil {
		return tobari.ConfiguratorPendingStage{}, false, err
	}
	defer releaseProject()
	receipt, present, err := a.configuratorStageForProjectLocked(ctx, projectRoot)
	if err != nil || !present {
		return tobari.ConfiguratorPendingStage{}, false, err
	}
	return a.PendingConfiguratorStage(ctx, receipt.Submission.Draft.TemplateID)
}

// configuratorStageForProjectLocked runs while the caller owns the Project
// lease. It takes each Template lease only long enough to read a stable receipt.
func (a *Adapter) configuratorStageForProjectLocked(ctx context.Context, projectRoot string) (workspaceauthoritysource.ConfiguratorStageReceipt, bool, error) {
	ids, err := a.sources.ListConfiguratorStageIDs(ctx)
	if err != nil {
		return workspaceauthoritysource.ConfiguratorStageReceipt{}, false, err
	}
	var matched workspaceauthoritysource.ConfiguratorStageReceipt
	presentMatch := false
	for _, id := range ids {
		release, leaseErr := a.sources.AcquireConfiguratorStageLease(ctx, id)
		if leaseErr != nil {
			return workspaceauthoritysource.ConfiguratorStageReceipt{}, false, leaseErr
		}
		receipt, present, readErr := a.sources.ReadConfiguratorStage(ctx, id)
		releaseErr := release()
		if readErr != nil || releaseErr != nil {
			return workspaceauthoritysource.ConfiguratorStageReceipt{}, false, errors.Join(readErr, releaseErr)
		}
		if present && isTaskScopedProjectStage(receipt, projectRoot) {
			if presentMatch {
				return workspaceauthoritysource.ConfiguratorStageReceipt{}, false, tobari.ErrResourceSourceRecoveryRequired
			}
			matched = receipt
			presentMatch = true
		}
	}
	return matched, presentMatch, nil
}

func (a *Adapter) acquireConfiguratorDraftStageLease(ctx context.Context, draft tobari.ConfiguratorDraft) (func() error, error) {
	if draft.Task == tobari.ConfiguratorTaskAggregate {
		return a.sources.AcquireConfiguratorProjectStageLease(ctx, draft.ProjectRoot)
	}
	scope, err := draft.ConfiguratorScopeKey()
	if err != nil {
		return nil, err
	}
	return a.sources.AcquireConfiguratorStageScopeLease(ctx, scope)
}

func (a *Adapter) configuratorStageForDraftLocked(ctx context.Context, draft tobari.ConfiguratorDraft) (workspaceauthoritysource.ConfiguratorStageReceipt, bool, error) {
	if draft.Task == tobari.ConfiguratorTaskAggregate {
		return a.configuratorStageForProjectLocked(ctx, draft.ProjectRoot)
	}
	scope, err := draft.ConfiguratorScopeKey()
	if err != nil {
		return workspaceauthoritysource.ConfiguratorStageReceipt{}, false, err
	}
	ids, err := a.sources.ListConfiguratorStageIDs(ctx)
	if err != nil {
		return workspaceauthoritysource.ConfiguratorStageReceipt{}, false, err
	}
	for _, id := range ids {
		receipt, present, readErr := a.sources.ReadConfiguratorStage(ctx, id)
		if readErr != nil {
			return workspaceauthoritysource.ConfiguratorStageReceipt{}, false, readErr
		}
		if !present {
			continue
		}
		candidateScope, scopeErr := receipt.Submission.Draft.ConfiguratorScopeKey()
		if scopeErr != nil {
			return workspaceauthoritysource.ConfiguratorStageReceipt{}, false, scopeErr
		}
		if candidateScope == scope {
			return receipt, true, nil
		}
	}
	return workspaceauthoritysource.ConfiguratorStageReceipt{}, false, nil
}

func isTaskScopedProjectStage(receipt workspaceauthoritysource.ConfiguratorStageReceipt, projectRoot string) bool {
	return receipt.Submission.Draft.ProjectRoot == projectRoot && receipt.Submission.Draft.Task != tobari.ConfiguratorTaskAggregate
}

func (a *Adapter) BindConfiguratorStagePlan(ctx context.Context, pending tobari.ConfiguratorPendingStage, planRef string) (tobari.ConfiguratorPendingStage, error) {
	if err := pending.Validate(); err != nil {
		return tobari.ConfiguratorPendingStage{}, err
	}
	releaseProject, err := a.acquireConfiguratorDraftStageLease(ctx, pending.Submission.Draft)
	if err != nil {
		return tobari.ConfiguratorPendingStage{}, err
	}
	defer releaseProject()
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, pending.Submission.Draft.TemplateID)
	if err != nil {
		return tobari.ConfiguratorPendingStage{}, err
	}
	defer releaseStage()
	receipt := workspaceauthoritysource.ConfiguratorStageReceipt{Submission: pending.Submission, Stage: pending.Stage, PlanRef: pending.PlanRef, ApplyConfirmed: pending.ApplyConfirmed}
	stored, present, err := a.sources.ReadConfiguratorStage(ctx, pending.Submission.Draft.TemplateID)
	if err != nil || !present {
		return tobari.ConfiguratorPendingStage{}, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	receipt.BaseFingerprint = stored.BaseFingerprint
	updated, err := a.sources.BindConfiguratorStagePlan(ctx, receipt, planRef)
	if err != nil {
		return tobari.ConfiguratorPendingStage{}, err
	}
	result := tobari.ConfiguratorPendingStage{Submission: updated.Submission, Stage: updated.Stage, PlanRef: updated.PlanRef, ApplyConfirmed: updated.ApplyConfirmed}
	return result, result.Validate()
}

func (a *Adapter) ConfirmConfiguratorStageApply(ctx context.Context, pending tobari.ConfiguratorPendingStage) (tobari.ConfiguratorPendingStage, error) {
	if err := pending.Validate(); err != nil || pending.PlanRef == "" {
		return tobari.ConfiguratorPendingStage{}, fmt.Errorf("Configurator stage confirmation is invalid: %w", err)
	}
	releaseProject, err := a.acquireConfiguratorDraftStageLease(ctx, pending.Submission.Draft)
	if err != nil {
		return tobari.ConfiguratorPendingStage{}, err
	}
	defer releaseProject()
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, pending.Submission.Draft.TemplateID)
	if err != nil {
		return tobari.ConfiguratorPendingStage{}, err
	}
	defer releaseStage()
	stored, present, err := a.sources.ReadConfiguratorStage(ctx, pending.Submission.Draft.TemplateID)
	if err != nil || !present {
		return tobari.ConfiguratorPendingStage{}, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	receipt := workspaceauthoritysource.ConfiguratorStageReceipt{Submission: pending.Submission, Stage: pending.Stage, BaseFingerprint: stored.BaseFingerprint, PlanRef: pending.PlanRef, ApplyConfirmed: pending.ApplyConfirmed}
	updated, err := a.sources.ConfirmConfiguratorStageApply(ctx, receipt)
	if err != nil {
		return tobari.ConfiguratorPendingStage{}, err
	}
	result := tobari.ConfiguratorPendingStage{Submission: updated.Submission, Stage: updated.Stage, PlanRef: updated.PlanRef, ApplyConfirmed: updated.ApplyConfirmed}
	return result, result.Validate()
}

func (a *Adapter) ConfirmConfiguratorPublication(ctx context.Context, submission tobari.ConfiguratorSubmission, snapshot tobari.ContextAuthoritySnapshot) error {
	if err := submission.Validate(); err != nil || snapshot.Validate() != nil {
		return fmt.Errorf("Configurator publication confirmation is invalid: %w", err)
	}
	ref, err := tobari.ContextRef(snapshot.Context.ID)
	if err != nil {
		return err
	}
	current, err := a.Store.ReadContextAuthorityByReference(ctx, ref)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, snapshot) || current.Template.ID != submission.Draft.TemplateID || current.Template.Current.Revision != submission.SourceRevision || submission.Draft.NeedsHomeAdoption() && current.Context.ID != submission.Draft.AdoptionContextID || !reflect.DeepEqual(current.Template.Current.Body, submission.Body) {
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	if submission.Draft.ProjectRoot != "" && (current.Workspace == nil || current.Workspace.ProjectRoot != submission.Draft.ProjectRoot) {
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	return nil
}

func (a *Adapter) CheckContextEntryPublicationBarrier(ctx context.Context, id tobari.ContextID) error {
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return err
	}
	defer releaseCatalog()
	return a.checkContextEntryPublicationBarrierLocked(ctx, id)
}

func (a *Adapter) checkContextEntryPublicationBarrierLocked(ctx context.Context, id tobari.ContextID) error {
	ref, err := tobari.ContextRef(id)
	if err != nil {
		return err
	}
	snapshot, err := a.Store.ReadContextAuthorityByReference(ctx, ref)
	if err != nil {
		return err
	}
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, snapshot.Template.ID)
	if err != nil {
		return err
	}
	defer releaseStage()
	if _, present, readErr := a.sources.ReadConfiguratorPublication(ctx, snapshot.Template.ID); readErr != nil {
		return readErr
	} else if present {
		return tobari.ErrContextBindingProtected
	}
	return nil
}

// acquireContextDeleteConfiguratorBarrierLocked additionally protects the exact
// Context named by a retained task Stage. Context entry only needs the
// publication barrier above; deletion must not retire the Home/authority that
// an unconfirmed or confirmed policy submission still binds. The caller keeps
// the returned Stage lease through authority, source, and Home deletion so a
// new Stage cannot enter after the check and before the deletion commit.
func (a *Adapter) acquireContextDeleteConfiguratorBarrierLocked(ctx context.Context, id tobari.ContextID) (func() error, error) {
	ref, err := tobari.ContextRef(id)
	if err != nil {
		return nil, err
	}
	snapshot, err := a.Store.ReadContextAuthorityByReference(ctx, ref)
	if err != nil {
		return nil, err
	}
	return a.acquireContextDeleteTemplateStageBarrierLocked(ctx, id, snapshot.Template.ID)
}

func (a *Adapter) acquireContextDeleteTemplateStageBarrierLocked(ctx context.Context, id tobari.ContextID, templateID tobari.WorkspaceTemplateID) (func() error, error) {
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if _, present, readErr := a.sources.ReadConfiguratorPublication(ctx, templateID); readErr != nil {
		return nil, errors.Join(readErr, releaseStage())
	} else if present {
		return nil, errors.Join(tobari.ErrContextBindingProtected, releaseStage())
	}
	receipt, present, err := a.sources.ReadConfiguratorStage(ctx, templateID)
	if err != nil {
		return nil, errors.Join(err, releaseStage())
	}
	if configuratorStageProtectsContextDelete(receipt, present, id) {
		return nil, errors.Join(tobari.ErrContextBindingProtected, releaseStage())
	}
	return releaseStage, nil
}

func configuratorStageProtectsContextDelete(receipt workspaceauthoritysource.ConfiguratorStageReceipt, present bool, id tobari.ContextID) bool {
	return present && receipt.Submission.Draft.ContextID == id
}

func (a *Adapter) BeginConfiguratorPublication(ctx context.Context, submission tobari.ConfiguratorSubmission) error {
	if err := submission.Validate(); err != nil || !submission.Draft.NeedsHomeAdoption() {
		return fmt.Errorf("Configurator publication barrier is invalid: %w", err)
	}
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return err
	}
	defer releaseCatalog()
	publicationIDs, err := a.sources.ListConfiguratorPublicationIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range publicationIDs {
		if id != submission.Draft.TemplateID {
			return tobari.ErrContextBindingProtected
		}
	}
	releaseProject, err := a.sources.AcquireConfiguratorProjectStageLease(ctx, submission.Draft.ProjectRoot)
	if err != nil {
		return err
	}
	defer releaseProject()
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, submission.Draft.TemplateID)
	if err != nil {
		return err
	}
	defer releaseStage()
	if current, present, readErr := a.sources.ReadConfiguratorPublication(ctx, submission.Draft.TemplateID); readErr != nil {
		return readErr
	} else if present {
		if reflect.DeepEqual(current, submission) {
			return nil
		}
		return tobari.ErrResourceSourceRecoveryRequired
	}
	if err := a.validateDetachedConfiguratorAttachmentLocked(ctx, submission.Draft); err != nil {
		return err
	}
	return a.sources.BeginConfiguratorPublication(ctx, submission)
}

func (a *Adapter) CompleteConfiguratorPublication(ctx context.Context, submission tobari.ConfiguratorSubmission) error {
	if err := submission.Validate(); err != nil || !submission.Draft.NeedsHomeAdoption() {
		return fmt.Errorf("Configurator publication settlement is invalid: %w", err)
	}
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return err
	}
	defer releaseCatalog()
	releaseProject, err := a.sources.AcquireConfiguratorProjectStageLease(ctx, submission.Draft.ProjectRoot)
	if err != nil {
		return err
	}
	defer releaseProject()
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, submission.Draft.TemplateID)
	if err != nil {
		return err
	}
	defer releaseStage()
	return a.sources.CompleteConfiguratorPublication(ctx, submission)
}

func (a *Adapter) PendingConfiguratorPublicationForProject(ctx context.Context, projectRoot string) (tobari.ConfiguratorSubmission, bool, error) {
	if err := tobari.ValidateCanonicalRoot(projectRoot); err != nil {
		return tobari.ConfiguratorSubmission{}, false, err
	}
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return tobari.ConfiguratorSubmission{}, false, err
	}
	defer releaseCatalog()
	ids, err := a.sources.ListConfiguratorPublicationIDs(ctx)
	if err != nil {
		return tobari.ConfiguratorSubmission{}, false, err
	}
	var result tobari.ConfiguratorSubmission
	found := false
	for _, id := range ids {
		submission, present, readErr := a.sources.ReadConfiguratorPublication(ctx, id)
		if readErr != nil {
			return tobari.ConfiguratorSubmission{}, false, readErr
		}
		if present && submission.Draft.ProjectRoot == projectRoot {
			if found {
				return tobari.ConfiguratorSubmission{}, false, tobari.ErrResourceSourceRecoveryRequired
			}
			result, found = submission, true
		}
	}
	return result, found, nil
}

func (a *Adapter) ListWorkspaceTemplateDrafts(ctx context.Context) ([]tobari.WorkspaceTemplateDraft, error) {
	ids, err := a.sources.ListTemplateIDs(ctx)
	if err != nil {
		return nil, err
	}
	active, err := a.Store.ListWorkspaceTemplates(ctx)
	if err != nil {
		return nil, err
	}
	activeIDs := make(map[tobari.WorkspaceTemplateID]struct{}, len(active))
	for _, template := range active {
		activeIDs[template.ID] = struct{}{}
	}
	result := make([]tobari.WorkspaceTemplateDraft, 0)
	for _, id := range ids {
		deleted, tombstoneErr := a.Store.IsAuthorityTombstoned("templates", string(id))
		if tombstoneErr != nil {
			return nil, tombstoneErr
		}
		if deleted {
			continue
		}
		if _, ok := activeIDs[id]; ok {
			continue
		}
		path, _ := a.sources.TemplatePath(id)
		source, present, readErr := a.sources.ReadTemplate(ctx, id)
		if readErr != nil || !present {
			draft := tobari.WorkspaceTemplateDraft{ID: id, Source: tobari.ResourceSourceObservation{Path: path, State: tobari.ResourceSourceInvalid}}
			if err := draft.Validate(); err != nil {
				return nil, err
			}
			result = append(result, draft)
			continue
		}
		resolved, err := a.mutator.ResolveWorkspaceTemplateRuntimeSource(ctx, source.Template.EntryDefaults.Runtime)
		if err != nil {
			return nil, err
		}
		revision, err := source.SemanticRevision(resolved)
		if err != nil {
			return nil, err
		}
		body, err := source.Body(resolved)
		if err != nil {
			return nil, err
		}
		draft := tobari.WorkspaceTemplateDraft{ID: id, Name: source.Template.Name, Body: body, Source: tobari.ResourceSourceObservation{Path: path, State: tobari.ResourceSourceModified, SourceRevision: &revision}}
		if err := draft.Validate(); err != nil {
			return nil, err
		}
		result = append(result, draft)
	}
	return result, nil
}

func (a *Adapter) DiscoverWorkspaceTemplateDraft(ctx context.Context, name string) (tobari.WorkspaceTemplateDraft, error) {
	drafts, err := a.ListWorkspaceTemplateDrafts(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	for _, draft := range drafts {
		if draft.Name == name {
			return draft, nil
		}
	}
	return tobari.WorkspaceTemplateDraft{}, tobari.ErrWorkspaceTemplateNotFound
}

// Adapter couples editable resource sources to the existing atomic active
// snapshot. The embedded store and mutator retain their task-specific ports;
// methods below add source validation/publication at the same public mutation
// boundary.
type Adapter struct {
	*workspaceauthoritystore.Store
	mutator        *workspaceauthoritystore.Mutator
	sources        *workspaceauthoritysource.Store
	observeRuntime workspaceauthoritystore.InstallationMigrationRuntimeObserver
	prepareRuntime workspaceauthoritystore.InstallationMigrationSourcePreparer
	contextHomes   ContextHomeLifecycle
}

type ContextHomeLifecycle interface {
	AcquireConfiguratorAttachment(context.Context, tobari.ConfiguratorDraft) (func() error, error)
	AcquireTombstonedContextHomeRetirement(context.Context, tobari.ContextID) (func() error, error)
	AcquireContextHomeRetirement(context.Context, tobari.ContextID) (func() error, error)
	RemoveContextHome(context.Context, tobari.ContextID) error
}

// ConfiguratorPolicyPublished verifies the task receipt against live Template
// authority under the same per-Template stage fence as canonical Apply. A
// surviving Stage receipt means Apply settlement still has work to do even
// when the reviewed policy is a semantic no-op. Policy Memory may continue
// evolving and is deliberately not interpreted as static-policy publication.
func (a *Adapter) ConfiguratorPolicyPublished(ctx context.Context, submission tobari.ConfiguratorSubmission) (bool, string, error) {
	if submission.Validate() != nil || submission.Draft.Task != tobari.ConfiguratorTaskPolicy {
		return false, "", fmt.Errorf("Policy assist publication receipt is invalid")
	}
	release, err := a.sources.AcquireConfiguratorStageLease(ctx, submission.Draft.TemplateID)
	if err != nil {
		return false, "", err
	}
	defer release()
	receipt, stagePresent, err := a.sources.ReadConfiguratorStage(ctx, submission.Draft.TemplateID)
	if err != nil {
		return false, "", err
	}
	if stagePresent {
		confirmed, receiptErr := validatePolicyAssistStageRecoveryReceipt(receipt, submission)
		if receiptErr != nil {
			return false, "", receiptErr
		}
		if !confirmed {
			return false, "", nil
		}
	}
	collection, present, err := a.Store.ReadComplete(ctx)
	if err != nil || !present {
		return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	contextPresent := false
	for _, record := range collection.Contexts {
		if record.Context.ID == submission.Draft.ContextID && record.Context.TemplateID == submission.Draft.TemplateID {
			contextPresent = true
			break
		}
	}
	if !contextPresent {
		return false, "", tobari.ErrResourceSourceRecoveryRequired
	}
	if stagePresent {
		pendingPlan, pending, pendingErr := a.mutator.PendingWorkspaceTemplateApplySettlement(submission.Draft.TemplateID)
		if pendingErr != nil {
			return false, "", pendingErr
		}
		if pending {
			settle, settleErr := policyAssistStageCanSettle(receipt, pendingPlan, true)
			if settleErr != nil {
				return false, "", settleErr
			}
			if !settle {
				return false, receipt.PlanRef, nil
			}
		}
	}
	for _, template := range collection.Templates {
		if template.ID == submission.Draft.TemplateID {
			if template.Current.Revision == submission.SourceRevision && reflect.DeepEqual(template.Current.Body, submission.Body) {
				if stagePresent {
					observation, err := a.ObserveWorkspaceTemplateSource(ctx, template)
					if err != nil {
						return false, "", err
					}
					if observation.State != tobari.ResourceSourceInSync || observation.SourceRevision == nil || *observation.SourceRevision != submission.SourceRevision || observation.ActiveRevision == nil || *observation.ActiveRevision != submission.SourceRevision {
						return false, "", tobari.ErrResourceSourceRecoveryRequired
					}
					if err := a.sources.SettleConfiguratorStage(ctx, template.ID, receipt.Stage.SourceFingerprint); err != nil {
						return false, "", err
					}
				}
				return true, "", nil
			}
			if template.Current.Revision == submission.Draft.BaseTemplateRevision {
				return false, "", nil
			}
			return false, "", tobari.ErrResourceSourceRecoveryRequired
		}
	}
	return false, "", tobari.ErrResourceSourceRecoveryRequired
}

func validatePolicyAssistStageRecoveryReceipt(receipt workspaceauthoritysource.ConfiguratorStageReceipt, submission tobari.ConfiguratorSubmission) (bool, error) {
	if !reflect.DeepEqual(receipt.Submission, submission) {
		return false, tobari.ErrResourceSourceRecoveryRequired
	}
	return receipt.ApplyConfirmed && receipt.PlanRef != "", nil
}

func policyAssistStageCanSettle(receipt workspaceauthoritysource.ConfiguratorStageReceipt, pendingPlan string, pending bool) (bool, error) {
	if pending && pendingPlan != receipt.PlanRef {
		return false, tobari.ErrResourceSourceRecoveryRequired
	}
	return !pending, nil
}

// AcquireConfiguratorAuthorAttachment closes selection-to-attachment races by
// re-reading the draft's exact authority while the catalog is fenced, then
// acquiring the Context/Project lease before releasing that fence.
func (a *Adapter) AcquireConfiguratorAuthorAttachment(ctx context.Context, draft tobari.ConfiguratorDraft) (func() error, error) {
	if err := draft.Validate(); err != nil || a.contextHomes == nil {
		return nil, errors.Join(fmt.Errorf("Configurator attachment authority is unavailable"), err)
	}
	if draft.Task == tobari.ConfiguratorTaskRuntime {
		// Installation-wide Runtime assistance owns neither Context nor Project
		// authority. Its Runtime reference and source generation were validated
		// before reservation; the installation Home/target attachment fence is
		// the only live authoring lease it needs.
		return a.contextHomes.AcquireConfiguratorAttachment(ctx, draft)
	}
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseCatalog()
	if draft.ContextID != "" {
		ref, err := tobari.ContextRef(draft.ContextID)
		if err != nil {
			return nil, err
		}
		snapshot, err := a.Store.ReadContextAuthorityByReference(ctx, ref)
		if err != nil {
			return nil, err
		}
		if err := validateConfiguratorAttachmentAuthority(draft, snapshot); err != nil {
			return nil, err
		}
	} else if err := a.validateDetachedConfiguratorAttachmentLocked(ctx, draft); err != nil {
		return nil, err
	}
	return a.contextHomes.AcquireConfiguratorAttachment(ctx, draft)
}

// AcquireConfiguratorPublicationAttachment resumes the exact durable
// publication across both the pre-authority and post-authority adoption phases.
func (a *Adapter) AcquireConfiguratorPublicationAttachment(ctx context.Context, submission tobari.ConfiguratorSubmission) (func() error, error) {
	if err := submission.Validate(); err != nil || !submission.Draft.NeedsHomeAdoption() || a.contextHomes == nil {
		return nil, errors.Join(fmt.Errorf("Configurator publication attachment authority is unavailable"), err)
	}
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseCatalog()
	stored, present, err := a.sources.ReadConfiguratorPublication(ctx, submission.Draft.TemplateID)
	if err != nil || !present || !reflect.DeepEqual(stored, submission) {
		return nil, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := a.validateConfiguratorPublicationAttachmentLocked(ctx, submission); err != nil {
		return nil, err
	}
	return a.contextHomes.AcquireConfiguratorAttachment(ctx, submission.Draft)
}

func (a *Adapter) validateConfiguratorPublicationAttachmentLocked(ctx context.Context, submission tobari.ConfiguratorSubmission) error {
	collection, present, err := a.Store.ReadComplete(ctx)
	if err != nil {
		return err
	}
	return validateConfiguratorPublicationAttachmentCollection(submission, collection, present)
}

func validateConfiguratorPublicationAttachmentCollection(submission tobari.ConfiguratorSubmission, collection tobari.WorkspaceAuthorityCollection, present bool) error {
	if err := submission.Validate(); err != nil || !submission.Draft.NeedsHomeAdoption() {
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	draft := submission.Draft
	if !present {
		if draft.Purpose == tobari.ConfiguratorPurposeBootstrap {
			return nil
		}
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	contextPublished := false
	for _, record := range collection.Contexts {
		if record.Context.ID == draft.AdoptionContextID {
			if record.Context.TemplateID != draft.TemplateID {
				return tobari.ErrWorkspaceTemplateChangePlanStale
			}
			contextPublished = true
			continue
		}
	}
	var current *tobari.WorkspaceTemplate
	for index := range collection.Templates {
		if collection.Templates[index].ID == draft.TemplateID {
			current = &collection.Templates[index]
			break
		}
	}
	if current == nil || collection.DefaultTemplateID != nil && *collection.DefaultTemplateID != draft.TemplateID {
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	if contextPublished {
		if current.Current.Revision == submission.SourceRevision && reflect.DeepEqual(current.Current.Body, submission.Body) {
			return nil
		}
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	if current.Current.Revision == submission.SourceRevision && reflect.DeepEqual(current.Current.Body, submission.Body) {
		return nil
	}
	if draft.Purpose == tobari.ConfiguratorPurposeEvolve && collection.DefaultTemplateID != nil && current.Current.Revision == draft.BaseTemplateRevision {
		return nil
	}
	return tobari.ErrWorkspaceTemplateChangePlanStale
}

func validateConfiguratorAttachmentAuthority(draft tobari.ConfiguratorDraft, snapshot tobari.ContextAuthoritySnapshot) error {
	if draft.Validate() != nil || snapshot.Validate() != nil || draft.ContextID == "" || snapshot.Context.ID != draft.ContextID || snapshot.Context.TemplateID != draft.TemplateID || snapshot.Template.Current.Revision != draft.BaseTemplateRevision || snapshot.PolicyMemory.Revision != draft.BasePolicyMemoryRevision {
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	return nil
}

func (a *Adapter) validateDetachedConfiguratorAttachmentLocked(ctx context.Context, draft tobari.ConfiguratorDraft) error {
	collection, present, err := a.Store.ReadComplete(ctx)
	if err != nil {
		return err
	}
	if draft.Purpose == tobari.ConfiguratorPurposeBootstrap {
		return validateBootstrapConfiguratorAttachmentState(present)
	}
	if !present || collection.DefaultTemplateID == nil || *collection.DefaultTemplateID != draft.TemplateID {
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	for _, template := range collection.Templates {
		if template.ID == draft.TemplateID && template.Current.Revision == draft.BaseTemplateRevision {
			for _, workspace := range collection.Workspaces {
				if workspace.ProjectRoot == draft.ProjectRoot {
					return tobari.ErrWorkspaceTemplateChangePlanStale
				}
			}
			return nil
		}
	}
	return tobari.ErrWorkspaceTemplateChangePlanStale
}

func validateBootstrapConfiguratorAttachmentState(collectionPresent bool) error {
	if collectionPresent {
		// Bootstrap seeds intentionally carry no collection revision. Once any
		// active collection exists, the pre-author empty observation cannot be
		// re-proven and the chooser must run again rather than create a stale Home.
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	return nil
}

func New(store *workspaceauthoritystore.Store, mutator *workspaceauthoritystore.Mutator, sources *workspaceauthoritysource.Store, observeRuntime workspaceauthoritystore.InstallationMigrationRuntimeObserver, prepareRuntime workspaceauthoritystore.InstallationMigrationSourcePreparer, contextHomes ...ContextHomeLifecycle) (*Adapter, error) {
	if store == nil || mutator == nil || sources == nil || observeRuntime == nil || prepareRuntime == nil {
		return nil, fmt.Errorf("file-backed final authority requires active and source stores")
	}
	adapter := &Adapter{Store: store, mutator: mutator, sources: sources, observeRuntime: observeRuntime, prepareRuntime: prepareRuntime}
	if len(contextHomes) > 0 {
		adapter.contextHomes = contextHomes[0]
	}
	return adapter, nil
}

func (a *Adapter) PlanInstallationMigration(ctx context.Context) (tobari.InstallationMigrationPlan, error) {
	return a.mutator.PlanInstallationMigration(ctx, a.observeRuntime)
}

func (a *Adapter) ApplyInstallationMigration(ctx context.Context, planRef string) (tobari.InstallationMigrationResult, error) {
	return a.mutator.ApplyInstallationMigration(ctx, planRef, a.observeRuntime, func(prepareContext context.Context, collection tobari.WorkspaceAuthorityCollection, recovery bool) (workspaceauthoritystore.InstallationMigrationSourceStage, error) {
		sources, err := a.sources.PrepareInstallationMigrationSources(prepareContext, collection, recovery)
		if err != nil {
			return nil, err
		}
		runtimes, err := a.prepareRuntime(prepareContext, collection, recovery)
		if err != nil {
			_ = sources.Abort(prepareContext)
			return nil, err
		}
		return &combinedInstallationMigrationStage{sources: sources, runtimes: runtimes}, nil
	})
}

type combinedInstallationMigrationStage struct {
	sources  workspaceauthoritystore.InstallationMigrationSourceStage
	runtimes workspaceauthoritystore.InstallationMigrationSourceStage
}

func (s *combinedInstallationMigrationStage) ExpectedIdentity() (tobari.SemanticDigest, error) {
	source, err := s.sources.ExpectedIdentity()
	if err != nil {
		return "", err
	}
	runtime, err := s.runtimes.ExpectedIdentity()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte("source\x00" + string(source) + "\x00runtime\x00" + string(runtime)))
	return tobari.SemanticDigest(fmt.Sprintf("sha256:%x", digest)), nil
}

func (s *combinedInstallationMigrationStage) Commit(ctx context.Context) error {
	if err := s.sources.Commit(ctx); err != nil {
		return err
	}
	if err := s.runtimes.Commit(ctx); err != nil {
		return errors.Join(err, s.sources.Rollback(ctx))
	}
	return nil
}
func (s *combinedInstallationMigrationStage) Verify(ctx context.Context) error {
	return errors.Join(s.sources.Verify(ctx), s.runtimes.Verify(ctx))
}
func (s *combinedInstallationMigrationStage) Rollback(ctx context.Context) error {
	return errors.Join(s.runtimes.Rollback(ctx), s.sources.Rollback(ctx))
}
func (s *combinedInstallationMigrationStage) Complete(ctx context.Context) error {
	if err := s.sources.Complete(ctx); err != nil {
		return err
	}
	return s.runtimes.Complete(ctx)
}
func (s *combinedInstallationMigrationStage) Abort(ctx context.Context) error {
	return errors.Join(s.runtimes.Abort(ctx), s.sources.Abort(ctx))
}

func (a *Adapter) ObserveWorkspaceTemplateSource(ctx context.Context, template tobari.WorkspaceTemplate) (tobari.ResourceSourceObservation, error) {
	if err := template.Validate(); err != nil {
		return tobari.ResourceSourceObservation{}, err
	}
	path, err := a.sources.TemplatePath(template.ID)
	if err != nil {
		return tobari.ResourceSourceObservation{}, err
	}
	result := tobari.ResourceSourceObservation{Path: path, ActiveRevision: digestPointer(template.Current.Revision)}
	source, present, readErr := a.sources.ReadTemplate(ctx, template.ID)
	if !present && readErr == nil {
		result.State = tobari.ResourceSourceMissing
		return result, result.Validate()
	}
	if readErr != nil {
		result.State = tobari.ResourceSourceInvalid
		return result, result.Validate()
	}
	revision, err := source.SemanticRevision(template.Current.Body.EntryDefaults.Runtime)
	if err != nil {
		result.State = tobari.ResourceSourceInvalid
		return result, result.Validate()
	}
	result.SourceRevision = &revision
	if source.ValidateFor(template) == nil && source.Template.Name == template.Name && revision == template.Current.Revision {
		result.State = tobari.ResourceSourceInSync
	} else {
		result.State = tobari.ResourceSourceModified
	}
	return result, result.Validate()
}

func (a *Adapter) ObserveContextSource(ctx context.Context, binding tobari.ContextBinding) (tobari.ResourceSourceObservation, error) {
	if err := binding.Validate(); err != nil {
		return tobari.ResourceSourceObservation{}, err
	}
	path, err := a.sources.ContextPath(binding.ID)
	if err != nil {
		return tobari.ResourceSourceObservation{}, err
	}
	active, err := contextSourceRevision(binding)
	if err != nil {
		return tobari.ResourceSourceObservation{}, err
	}
	result := tobari.ResourceSourceObservation{Path: path, ActiveRevision: digestPointer(active)}
	source, present, readErr := a.sources.ReadContext(ctx, binding.ID)
	if !present && readErr == nil {
		result.State = tobari.ResourceSourceMissing
		return result, result.Validate()
	}
	if readErr != nil {
		result.State = tobari.ResourceSourceInvalid
		return result, result.Validate()
	}
	revision, err := contextSourceDocumentRevision(source)
	if err != nil {
		result.State = tobari.ResourceSourceInvalid
		return result, result.Validate()
	}
	result.SourceRevision = &revision
	if source.ValidateFor(binding) == nil && revision == active {
		result.State = tobari.ResourceSourceInSync
	} else {
		result.State = tobari.ResourceSourceModified
	}
	return result, result.Validate()
}

func (a *Adapter) ListContextDrafts(ctx context.Context) ([]tobari.ContextDraft, error) {
	ids, err := a.sources.ListContextIDs(ctx)
	if err != nil {
		return nil, err
	}
	active, err := a.Store.ListContextAuthority(ctx)
	if err != nil {
		return nil, err
	}
	activeIDs := make(map[tobari.ContextID]struct{}, len(active))
	for _, snapshot := range active {
		activeIDs[snapshot.Context.ID] = struct{}{}
	}
	result := make([]tobari.ContextDraft, 0)
	for _, id := range ids {
		deleted, tombstoneErr := a.Store.IsAuthorityTombstoned("contexts", string(id))
		if tombstoneErr != nil {
			return nil, tombstoneErr
		}
		if deleted {
			continue
		}
		if _, exists := activeIDs[id]; exists {
			continue
		}
		path, _ := a.sources.ContextPath(id)
		source, present, readErr := a.sources.ReadContext(ctx, id)
		if readErr != nil || !present {
			draft := tobari.ContextDraft{Source: tobari.ContextSource{ContextID: id}, Observation: tobari.ResourceSourceObservation{Path: path, State: tobari.ResourceSourceInvalid}}
			if err := draft.Validate(); err != nil {
				return nil, err
			}
			result = append(result, draft)
			continue
		}
		revision, err := contextSourceDocumentRevision(source)
		if err != nil {
			return nil, err
		}
		draft := tobari.ContextDraft{Source: source, Observation: tobari.ResourceSourceObservation{Path: path, State: tobari.ResourceSourceModified, SourceRevision: &revision}}
		if err := draft.Validate(); err != nil {
			return nil, err
		}
		result = append(result, draft)
	}
	return result, nil
}

func digestPointer(value tobari.SemanticDigest) *tobari.SemanticDigest { copy := value; return &copy }

func (a *Adapter) InitializeFinalDefaultPair(ctx context.Context, root string, body tobari.WorkspaceTemplateBody) (tobari.FinalDefaultPairPublication, error) {
	return a.initializeFinalDefaultPair(ctx, root, "", "", body)
}

func (a *Adapter) requireNoConfiguratorPublicationsLocked(ctx context.Context) error {
	ids, err := a.sources.ListConfiguratorPublicationIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		submission, present, readErr := a.sources.ReadConfiguratorPublication(ctx, id)
		if readErr != nil {
			return readErr
		}
		if present && submission.Draft.Task != tobari.ConfiguratorTaskAggregate {
			return tobari.ErrContextBindingProtected
		}
	}
	return nil
}

func (a *Adapter) InitializeFinalDefaultPairWithTemplateID(ctx context.Context, root string, templateID tobari.WorkspaceTemplateID, body tobari.WorkspaceTemplateBody) (tobari.FinalDefaultPairPublication, error) {
	if err := templateID.Validate(); err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	return a.initializeFinalDefaultPair(ctx, root, templateID, "", body)
}

func (a *Adapter) InitializeFinalDefaultPairWithConfiguratorIDs(ctx context.Context, root string, templateID tobari.WorkspaceTemplateID, contextID tobari.ContextID, body tobari.WorkspaceTemplateBody) (tobari.FinalDefaultPairPublication, error) {
	if err := templateID.Validate(); err != nil || contextID.Validate() != nil {
		return tobari.FinalDefaultPairPublication{}, errors.Join(err, contextID.Validate())
	}
	return a.initializeFinalDefaultPair(ctx, root, templateID, contextID, body)
}

func (a *Adapter) initializeFinalDefaultPair(ctx context.Context, root string, preferredTemplateID tobari.WorkspaceTemplateID, preferredContextID tobari.ContextID, body tobari.WorkspaceTemplateBody) (tobari.FinalDefaultPairPublication, error) {
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	defer releaseCatalog()
	publicationIDs, err := a.sources.ListConfiguratorPublicationIDs(ctx)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	activePublicationIDs := make([]tobari.WorkspaceTemplateID, 0, len(publicationIDs))
	for _, id := range publicationIDs {
		submission, present, readErr := a.sources.ReadConfiguratorPublication(ctx, id)
		if readErr != nil {
			return tobari.FinalDefaultPairPublication{}, readErr
		}
		if present && submission.Draft.Task != tobari.ConfiguratorTaskAggregate {
			activePublicationIDs = append(activePublicationIDs, id)
		}
	}
	publicationIDs = activePublicationIDs
	if preferredContextID != "" && (len(publicationIDs) != 1 || publicationIDs[0] != preferredTemplateID) {
		return tobari.FinalDefaultPairPublication{}, tobari.ErrResourceSourceRecoveryRequired
	}
	for _, id := range publicationIDs {
		if preferredContextID == "" || id != preferredTemplateID {
			return tobari.FinalDefaultPairPublication{}, tobari.ErrContextBindingProtected
		}
		submission, present, readErr := a.sources.ReadConfiguratorPublication(ctx, id)
		if readErr != nil || !present || submission.Draft.ProjectRoot != root || submission.Draft.AdoptionContextID != preferredContextID || !reflect.DeepEqual(submission.Body, body) {
			return tobari.FinalDefaultPairPublication{}, errors.Join(tobari.ErrResourceSourceRecoveryRequired, readErr)
		}
	}
	collection, present, err := a.Store.ReadComplete(ctx)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	if present && preferredContextID != "" {
		pendingPlan, pending, pendingErr := a.mutator.PendingWorkspaceTemplateApplySettlement(preferredTemplateID)
		if pendingErr != nil {
			return tobari.FinalDefaultPairPublication{}, pendingErr
		}
		if pending {
			if _, applyErr := a.applyWorkspaceTemplateSourceByReference(ctx, pendingPlan, true); applyErr != nil {
				return tobari.FinalDefaultPairPublication{}, applyErr
			}
			collection, present, err = a.Store.ReadComplete(ctx)
			if err != nil {
				return tobari.FinalDefaultPairPublication{}, err
			}
		}
	}
	previous, err := tobari.NewFinalDefaultPairObservation(collection, present, root)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	var template tobari.WorkspaceTemplate
	selectPreferredDefault := false
	if !present {
		var draft tobari.WorkspaceTemplateDraft
		if preferredTemplateID != "" {
			draft, err = a.createWorkspaceTemplateDraftWithID(ctx, preferredTemplateID, tobari.DefaultManifestName, body)
		} else {
			issued, issueErr := tobari.IssueWorkspaceTemplateID(time.Now().UTC(), rand.Reader)
			if issueErr != nil {
				return tobari.FinalDefaultPairPublication{}, issueErr
			}
			draft, err = a.createWorkspaceTemplateDraftWithID(ctx, issued, tobari.DefaultManifestName, body)
		}
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		expectedSource, err := tobari.NewWorkspaceTemplateDraftSource(draft.ID, draft.Name, body)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		expectedRevision, err := expectedSource.SemanticRevision(body.EntryDefaults.Runtime)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		expectedFingerprint, err := a.sources.ConfiguratorTemplateFingerprint(expectedSource)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		ref, _ := tobari.WorkspaceTemplateRef(draft.ID)
		plan, err := a.PlanWorkspaceTemplateSourceByReference(ctx, ref)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		if err := validateInitializerTemplatePlan(plan, ref, expectedRevision, expectedFingerprint); err != nil {
			return tobari.FinalDefaultPairPublication{}, tobari.ErrWorkspaceTemplateChangePlanStale
		}
		applied, err := a.applyWorkspaceTemplateSourceByReference(ctx, plan.PlanRef, true)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		template = applied.Template.Clone()
		ref, _ = tobari.WorkspaceTemplateRef(template.ID)
		if _, err := a.mutator.SetDefaultWorkspaceTemplateByReference(ctx, ref); err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
	} else {
		if collection.DefaultTemplateID == nil {
			if preferredTemplateID == "" {
				return tobari.FinalDefaultPairPublication{}, tobari.ErrDefaultTemplateSelectionRequired
			}
			for _, candidate := range collection.Templates {
				if candidate.ID == preferredTemplateID {
					template = candidate.Clone()
					break
				}
			}
			if template.ID == "" || !reflect.DeepEqual(template.Current.Body, body) {
				return tobari.FinalDefaultPairPublication{}, tobari.ErrResourceSourceRecoveryRequired
			}
			selectPreferredDefault = true
		} else {
			for _, candidate := range collection.Templates {
				if candidate.ID == *collection.DefaultTemplateID {
					template = candidate.Clone()
					break
				}
			}
		}
		if preferredTemplateID != "" && template.ID != preferredTemplateID {
			return tobari.FinalDefaultPairPublication{}, tobari.ErrWorkspaceTemplateChangePlanStale
		}
		if err := a.requireCurrentTemplateSourceByValue(ctx, template); err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		if selectPreferredDefault {
			ref, _ := tobari.WorkspaceTemplateRef(template.ID)
			if _, err := a.mutator.SetDefaultWorkspaceTemplateByReference(ctx, ref); err != nil {
				return tobari.FinalDefaultPairPublication{}, err
			}
		}
	}
	current, _, err := a.Store.ReadComplete(ctx)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	selectedContextID := preferredContextID
	contextPresent := false
	for _, record := range current.Contexts {
		if record.Context.TemplateID == template.ID && preferredContextID != "" && record.Context.ID == preferredContextID {
			contextPresent = true
			break
		}
	}
	if !contextPresent {
		var draft tobari.ContextDraft
		drafts, listErr := a.ListContextDrafts(ctx)
		if listErr != nil {
			return tobari.FinalDefaultPairPublication{}, listErr
		}
		for _, candidate := range drafts {
			if candidate.Source.TemplateID == template.ID && (preferredContextID == "" || candidate.Source.ContextID == preferredContextID) {
				if preferredContextID != "" && candidate.Source.ContextID != preferredContextID {
					return tobari.FinalDefaultPairPublication{}, tobari.ErrResourceSourceRecoveryRequired
				}
				if draft.Source.ContextID != "" {
					return tobari.FinalDefaultPairPublication{}, tobari.ErrResourceSourceRecoveryRequired
				}
				draft = candidate
			}
		}
		if draft.Source.ContextID == "" {
			templateRef, _ := tobari.WorkspaceTemplateRef(template.ID)
			if preferredContextID != "" {
				draft, err = a.createContextDraftWithID(ctx, templateRef, preferredContextID)
			} else {
				issued, issueErr := tobari.IssueContextID(time.Now().UTC(), rand.Reader)
				if issueErr != nil {
					return tobari.FinalDefaultPairPublication{}, issueErr
				}
				draft, err = a.createContextDraftWithID(ctx, templateRef, issued)
			}
			if err != nil {
				return tobari.FinalDefaultPairPublication{}, err
			}
		}
		selectedContextID = draft.Source.ContextID
		contextRef, _ := tobari.ContextRef(draft.Source.ContextID)
		expectedContextSource := draft.Source
		observedContextSource, expectedContextFingerprint, sourcePresent, err := a.sources.ReadContextSnapshot(ctx, draft.Source.ContextID)
		if err != nil || !sourcePresent || !reflect.DeepEqual(observedContextSource, expectedContextSource) {
			return tobari.FinalDefaultPairPublication{}, errors.Join(tobari.ErrResourceSourceChanged, err)
		}
		plan, err := a.PlanContextSourceByReference(ctx, contextRef)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		templateRef, _ := tobari.WorkspaceTemplateRef(template.ID)
		if err := validateInitializerContextPlan(plan, contextRef, templateRef, expectedContextFingerprint); err != nil {
			return tobari.FinalDefaultPairPublication{}, tobari.ErrWorkspaceTemplateChangePlanStale
		}
		if _, _, err := a.applyContextSourceByPlan(ctx, plan.PlanRef); err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
	}
	current, _, err = a.Store.ReadComplete(ctx)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	confirmed, err := tobari.NewFinalDefaultPairObservation(current, true, root)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	if confirmed.Context == nil && selectedContextID != "" {
		snapshots, snapshotErr := current.ContextSnapshots()
		if snapshotErr != nil {
			return tobari.FinalDefaultPairPublication{}, snapshotErr
		}
		for _, snapshot := range snapshots {
			if snapshot.Context.ID == selectedContextID {
				value := snapshot.Clone()
				confirmed.Context = &value
				break
			}
		}
	}
	publication := tobari.FinalDefaultPairPublication{Previous: previous, Current: confirmed, Changed: !present || previous.Context == nil}
	if err := publication.ValidateFor(root, body); err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	return publication, nil
}

func validateInitializerTemplatePlan(plan tobari.WorkspaceTemplateChangePlan, templateRef string, revision tobari.SemanticDigest, fingerprint string) error {
	if plan.TemplateRef != templateRef || plan.SourceRevision != revision || plan.SourceFingerprint != fingerprint {
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	return nil
}

func validateInitializerContextPlan(plan tobari.ContextActivationPlan, contextRef, templateRef, fingerprint string) error {
	if plan.ContextRef != contextRef || plan.TemplateRef != templateRef || plan.SourceFingerprint != fingerprint {
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	return nil
}

func (a *Adapter) CopyWorkspaceTemplateDraftByRevisionReference(ctx context.Context, ref, name string) (tobari.WorkspaceTemplateDraft, error) {
	sourceID, sourceRevision, err := tobari.ParseWorkspaceTemplateRevisionRef(ref)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	sourceRef, err := tobari.WorkspaceTemplateRef(sourceID)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	if err := a.requireCurrentTemplateSource(ctx, sourceRef); err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	collection, present, err := a.Store.ReadComplete(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	if !present {
		return tobari.WorkspaceTemplateDraft{}, tobari.ErrWorkspaceTemplateNotFound
	}
	for _, template := range collection.Templates {
		if template.ID != sourceID {
			continue
		}
		for _, revision := range template.Retained {
			if revision.Revision == sourceRevision {
				return a.CreateWorkspaceTemplateDraft(ctx, name, revision.Body)
			}
		}
	}
	return tobari.WorkspaceTemplateDraft{}, tobari.ErrWorkspaceTemplateNotFound
}

// SetDefaultWorkspaceTemplateByReference selects presentation/default routing
// only. It cannot mutate any retained Template revision or source document.
func (a *Adapter) SetDefaultWorkspaceTemplateByReference(ctx context.Context, ref string) (tobari.WorkspaceTemplateSelectionResult, error) {
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateSelectionResult{}, err
	}
	defer releaseCatalog()
	publicationIDs, err := a.sources.ListConfiguratorPublicationIDs(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateSelectionResult{}, err
	}
	for _, id := range publicationIDs {
		if _, present, readErr := a.sources.ReadConfiguratorPublication(ctx, id); readErr != nil {
			return tobari.WorkspaceTemplateSelectionResult{}, readErr
		} else if present {
			return tobari.WorkspaceTemplateSelectionResult{}, tobari.ErrContextBindingProtected
		}
	}
	templates, err := a.Store.ListWorkspaceTemplates(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateSelectionResult{}, err
	}
	ids := make([]tobari.WorkspaceTemplateID, 0, len(templates))
	for _, template := range templates {
		ids = append(ids, template.ID)
	}
	release, err := a.acquireConfiguratorStageSet(ctx, ids)
	if err != nil {
		return tobari.WorkspaceTemplateSelectionResult{}, err
	}
	defer release()
	for _, id := range ids {
		if _, present, readErr := a.sources.ReadConfiguratorStage(ctx, id); readErr != nil {
			return tobari.WorkspaceTemplateSelectionResult{}, readErr
		} else if present {
			return tobari.WorkspaceTemplateSelectionResult{}, tobari.ErrContextBindingProtected
		}
		if _, present, readErr := a.sources.ReadConfiguratorPublication(ctx, id); readErr != nil {
			return tobari.WorkspaceTemplateSelectionResult{}, readErr
		} else if present {
			return tobari.WorkspaceTemplateSelectionResult{}, tobari.ErrContextBindingProtected
		}
	}
	return a.mutator.SetDefaultWorkspaceTemplateByReference(ctx, ref)
}

// DeleteWorkspaceByReference is an explicit dependency-checked resource
// deletion, not a Template semantic writer.
func (a *Adapter) DeleteWorkspaceByReference(ctx context.Context, ref string, force bool) (tobari.WorkspaceAuthorityDeleteResult, error) {
	return a.mutator.DeleteWorkspaceByReference(ctx, ref, force)
}

func (a *Adapter) ApplyWorkspaceTemplateSourceByReference(ctx context.Context, ref string) (tobari.WorkspaceTemplateRevisionPublication, error) {
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	defer releaseCatalog()
	return a.applyWorkspaceTemplateSourceByReference(ctx, ref, false)
}

func (a *Adapter) applyWorkspaceTemplateSourceByReference(ctx context.Context, ref string, allowExactPublicationRecovery bool) (tobari.WorkspaceTemplateRevisionPublication, error) {
	id, err := tobari.ParseWorkspaceTemplateChangePlanRef(ref)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, id)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	defer releaseStage()
	configuratorReceipt, configuratorPresent, err := a.sources.ReadConfiguratorStage(ctx, id)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	if err := validateConfiguratorStageApplyAuthorization(configuratorReceipt, configuratorPresent, ref); err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	publicationReceipt, publicationPresent, err := a.sources.ReadConfiguratorPublication(ctx, id)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	var publicationFingerprint string
	if publicationPresent {
		if err := validateConfiguratorPublicationApplyAuthorization(configuratorPresent, allowExactPublicationRecovery); err != nil {
			return tobari.WorkspaceTemplateRevisionPublication{}, err
		}
		collection, authorityPresent, readErr := a.Store.ReadComplete(ctx)
		if readErr != nil {
			return tobari.WorkspaceTemplateRevisionPublication{}, readErr
		}
		active := false
		if authorityPresent {
			for _, template := range collection.Templates {
				active = active || template.ID == id
			}
		}
		if !configuratorPresent && (active || publicationReceipt.Draft.Purpose != tobari.ConfiguratorPurposeBootstrap) {
			pendingPlan, pending, pendingErr := a.mutator.PendingWorkspaceTemplateApplySettlement(id)
			if pendingErr != nil {
				return tobari.WorkspaceTemplateRevisionPublication{}, pendingErr
			}
			if !pending || pendingPlan != ref {
				return tobari.WorkspaceTemplateRevisionPublication{}, tobari.ErrContextBindingProtected
			}
		}
		expected, sourceErr := tobari.NewWorkspaceTemplateDraftSource(id, tobari.DefaultManifestName, publicationReceipt.Body)
		if sourceErr != nil {
			return tobari.WorkspaceTemplateRevisionPublication{}, sourceErr
		}
		publicationFingerprint, err = a.sources.ConfiguratorTemplateFingerprint(expected)
		if err != nil {
			return tobari.WorkspaceTemplateRevisionPublication{}, err
		}
	}
	var selectedFingerprint string
	publication, err := a.mutator.ApplyWorkspaceTemplateSourceByReference(ctx, ref, func(loadContext context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		source, fingerprint, present, loadErr := a.sources.ReadTemplateSnapshot(loadContext, id)
		if loadErr != nil {
			return tobari.WorkspaceTemplateSource{}, "", errors.Join(tobari.ErrResourceSourceInvalid, loadErr)
		}
		if !present {
			return tobari.WorkspaceTemplateSource{}, "", tobari.ErrResourceSourceMissing
		}
		if configuratorPresent && fingerprint != configuratorReceipt.Stage.SourceFingerprint {
			return tobari.WorkspaceTemplateSource{}, "", tobari.ErrWorkspaceTemplateChangePlanStale
		}
		if publicationPresent && !configuratorPresent && fingerprint != publicationFingerprint {
			return tobari.WorkspaceTemplateSource{}, "", tobari.ErrWorkspaceTemplateChangePlanStale
		}
		if selectedFingerprint == "" {
			selectedFingerprint = fingerprint
		}
		return source, fingerprint, nil
	})
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	if publication.Current.Revision != publication.Previous.Revision {
		_, err := a.sources.AdvanceTemplateBase(ctx, id, selectedFingerprint, publication.Current.Revision, publication.Current.Body.EntryDefaults.Runtime, func(postFingerprint string) error {
			return a.mutator.RecordWorkspaceTemplateApplyPostFingerprint(ref, postFingerprint)
		})
		if err != nil {
			return tobari.WorkspaceTemplateRevisionPublication{}, recoveryError("applied Template base revision", err)
		}
	}
	if err := a.mutator.CompleteWorkspaceTemplateApplySettlement(ref); err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, recoveryError("applied Template settlement", err)
	}
	if err := a.sources.SettleConfiguratorStage(ctx, id, selectedFingerprint); err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, recoveryError("settled Configurator stage", err)
	}
	return publication, nil
}

func validateConfiguratorPublicationApplyAuthorization(confirmedStagePresent, exactInitializerRecovery bool) error {
	if !confirmedStagePresent && !exactInitializerRecovery {
		return tobari.ErrContextBindingProtected
	}
	return nil
}

func validateConfiguratorStageApplyAuthorization(receipt workspaceauthoritysource.ConfiguratorStageReceipt, present bool, planRef string) error {
	if !present {
		return nil
	}
	if !receipt.ApplyConfirmed || receipt.PlanRef != planRef {
		return tobari.ErrWorkspaceTemplateChangePlanStale
	}
	return nil
}

func (a *Adapter) PlanWorkspaceTemplateSourceByReference(ctx context.Context, ref string) (tobari.WorkspaceTemplateChangePlan, error) {
	id, err := tobari.ParseWorkspaceTemplateRef(ref)
	if err != nil {
		return tobari.WorkspaceTemplateChangePlan{}, err
	}
	return a.mutator.PlanWorkspaceTemplateSourceByReference(ctx, ref, func(loadContext context.Context) (tobari.WorkspaceTemplateSource, string, error) {
		source, fingerprint, present, loadErr := a.sources.ReadTemplateSnapshot(loadContext, id)
		if loadErr != nil {
			return tobari.WorkspaceTemplateSource{}, "", errors.Join(tobari.ErrResourceSourceInvalid, loadErr)
		}
		if !present {
			return tobari.WorkspaceTemplateSource{}, "", tobari.ErrResourceSourceMissing
		}
		return source, fingerprint, nil
	})
}

func (a *Adapter) PlanWorkspaceTemplatePolicyMigrationByReference(ctx context.Context, ref string) (tobari.WorkspaceTemplatePolicyMigrationPlan, error) {
	id, err := tobari.ParseWorkspaceTemplateRef(ref)
	if err != nil {
		return tobari.WorkspaceTemplatePolicyMigrationPlan{}, err
	}
	template, err := a.workspaceTemplateByID(ctx, id)
	if err != nil {
		return tobari.WorkspaceTemplatePolicyMigrationPlan{}, err
	}
	alpha, migrated, sourceFingerprint, targetFingerprint, present, err := a.sources.ReadTemplatePolicyMigrationSnapshot(ctx, id, template.Current.Body.EntryDefaults.Runtime)
	if err != nil {
		return tobari.WorkspaceTemplatePolicyMigrationPlan{}, err
	}
	if !present {
		return tobari.WorkspaceTemplatePolicyMigrationPlan{}, tobari.ErrResourceSourceMissing
	}
	return tobari.NewWorkspaceTemplatePolicyMigrationPlan(template, alpha, migrated, sourceFingerprint, targetFingerprint)
}

func (a *Adapter) ApplyWorkspaceTemplatePolicyMigrationByReference(ctx context.Context, ref string) (tobari.WorkspaceTemplatePolicyMigrationResult, error) {
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return tobari.WorkspaceTemplatePolicyMigrationResult{}, err
	}
	defer releaseCatalog()
	plan, err := tobari.WorkspaceTemplatePolicyMigrationPlanFromRef(ref)
	if err != nil {
		return tobari.WorkspaceTemplatePolicyMigrationResult{}, err
	}
	id, _ := tobari.ParseWorkspaceTemplateRef(plan.TemplateRef)
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, id)
	if err != nil {
		return tobari.WorkspaceTemplatePolicyMigrationResult{}, err
	}
	defer releaseStage()
	if _, present, readErr := a.sources.ReadConfiguratorPublication(ctx, id); readErr != nil {
		return tobari.WorkspaceTemplatePolicyMigrationResult{}, readErr
	} else if present {
		return tobari.WorkspaceTemplatePolicyMigrationResult{}, tobari.ErrContextBindingProtected
	}
	var result tobari.WorkspaceTemplatePolicyMigrationResult
	err = a.mutator.WithWorkspaceTemplatePolicyMigrationFence(ctx, id, plan.ActiveRevision, func(locked context.Context, template tobari.WorkspaceTemplate) error {
		fingerprint, changed, applyErr := a.sources.ApplyTemplatePolicyMigration(locked, plan, template.Current.Body.EntryDefaults.Runtime)
		if applyErr != nil {
			return applyErr
		}
		source, observed, present, readErr := a.sources.ReadTemplateSnapshot(locked, id)
		if readErr != nil || !present || observed != fingerprint || observed != plan.TargetFingerprint || source.ValidateFor(template) != nil || source.Template.Name != template.Name {
			return recoveryError("migrated Template policy source", errors.Join(tobari.ErrResourceSourceRecoveryRequired, readErr))
		}
		result = tobari.WorkspaceTemplatePolicyMigrationResult{
			TemplateID: id, TemplateRef: plan.TemplateRef, ActiveRevision: plan.ActiveRevision,
			SourceFingerprint: fingerprint, Changed: changed,
		}
		return result.Validate()
	})
	return result, err
}

func (a *Adapter) workspaceTemplateByID(ctx context.Context, id tobari.WorkspaceTemplateID) (tobari.WorkspaceTemplate, error) {
	templates, err := a.Store.ListWorkspaceTemplates(ctx)
	if err != nil {
		return tobari.WorkspaceTemplate{}, err
	}
	for _, template := range templates {
		if template.ID == id {
			return template, nil
		}
	}
	return tobari.WorkspaceTemplate{}, tobari.ErrWorkspaceTemplateNotFound
}

func (a *Adapter) DeleteWorkspaceTemplateByReference(ctx context.Context, ref string) (tobari.WorkspaceTemplateDeleteResult, error) {
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateDeleteResult{}, err
	}
	defer releaseCatalog()
	id, parseErr := tobari.ParseWorkspaceTemplateRef(ref)
	if parseErr != nil {
		return tobari.WorkspaceTemplateDeleteResult{}, parseErr
	}
	releaseStage, err := a.sources.AcquireConfiguratorStageLease(ctx, id)
	if err != nil {
		return tobari.WorkspaceTemplateDeleteResult{}, err
	}
	defer releaseStage()
	if _, present, readErr := a.sources.ReadConfiguratorStage(ctx, id); readErr != nil {
		return tobari.WorkspaceTemplateDeleteResult{}, readErr
	} else if present {
		return tobari.WorkspaceTemplateDeleteResult{}, tobari.ErrContextBindingProtected
	}
	if _, present, readErr := a.sources.ReadConfiguratorPublication(ctx, id); readErr != nil {
		return tobari.WorkspaceTemplateDeleteResult{}, readErr
	} else if present {
		return tobari.WorkspaceTemplateDeleteResult{}, tobari.ErrContextBindingProtected
	}
	if deleted, err := a.Store.IsAuthorityTombstoned("templates", string(id)); err != nil {
		return tobari.WorkspaceTemplateDeleteResult{}, err
	} else if deleted {
		if err := a.sources.DeleteTemplate(ctx, id); err != nil && !errors.Is(err, os.ErrNotExist) {
			return tobari.WorkspaceTemplateDeleteResult{}, recoveryError("deleted Template source", err)
		}
		return tobari.WorkspaceTemplateDeleteResult{TemplateID: id, Deleted: true}, nil
	}
	result, err := a.mutator.DeleteWorkspaceTemplateByReference(ctx, ref)
	if err != nil {
		var handled bool
		result, handled, err = a.deleteUnpublishedWorkspaceTemplateDraft(ctx, id, err)
		if err != nil || !handled {
			return tobari.WorkspaceTemplateDeleteResult{}, err
		}
		return result, nil
	}
	if err := a.sources.DeleteTemplate(ctx, result.TemplateID); err != nil && !errors.Is(err, os.ErrNotExist) {
		return tobari.WorkspaceTemplateDeleteResult{}, recoveryError("deleted Template source", err)
	}
	return result, nil
}

func (a *Adapter) deleteUnpublishedWorkspaceTemplateDraft(
	ctx context.Context, id tobari.WorkspaceTemplateID, authorityErr error,
) (tobari.WorkspaceTemplateDeleteResult, bool, error) {
	if !errors.Is(authorityErr, tobari.ErrWorkspaceTemplateNotFound) && !errors.Is(authorityErr, tobari.ErrFinalAuthorityNotFound) {
		return tobari.WorkspaceTemplateDeleteResult{}, false, authorityErr
	}
	_, present, err := a.sources.ReadTemplate(ctx, id)
	if err != nil {
		return tobari.WorkspaceTemplateDeleteResult{}, true, err
	}
	if !present {
		return tobari.WorkspaceTemplateDeleteResult{}, true, tobari.ErrWorkspaceTemplateNotFound
	}
	if err := a.sources.DeleteTemplate(ctx, id); err != nil && !errors.Is(err, os.ErrNotExist) {
		return tobari.WorkspaceTemplateDeleteResult{}, true, recoveryError("deleted unpublished Template source", err)
	}
	return tobari.WorkspaceTemplateDeleteResult{TemplateID: id, Deleted: true}, true, nil
}

func (a *Adapter) acquireConfiguratorStageSet(ctx context.Context, ids []tobari.WorkspaceTemplateID) (func() error, error) {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	releases := make([]func() error, 0, len(ids))
	for _, id := range ids {
		release, err := a.sources.AcquireConfiguratorStageLease(ctx, id)
		if err != nil {
			var releaseErr error
			for index := len(releases) - 1; index >= 0; index-- {
				releaseErr = errors.Join(releaseErr, releases[index]())
			}
			return nil, errors.Join(err, releaseErr)
		}
		releases = append(releases, release)
	}
	return func() error {
		var result error
		for index := len(releases) - 1; index >= 0; index-- {
			result = errors.Join(result, releases[index]())
		}
		return result
	}, nil
}

func (a *Adapter) CreateContextDraftByTemplateReference(ctx context.Context, ref string) (tobari.ContextDraft, error) {
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return tobari.ContextDraft{}, err
	}
	defer releaseCatalog()
	if err := a.requireNoConfiguratorPublicationsLocked(ctx); err != nil {
		return tobari.ContextDraft{}, err
	}
	id, err := tobari.IssueContextID(time.Now().UTC(), rand.Reader)
	if err != nil {
		return tobari.ContextDraft{}, err
	}
	return a.createContextDraftWithID(ctx, ref, id)
}

func (a *Adapter) createContextDraftWithID(ctx context.Context, ref string, id tobari.ContextID) (tobari.ContextDraft, error) {
	templateID, err := tobari.ParseWorkspaceTemplateRef(ref)
	if err != nil {
		return tobari.ContextDraft{}, err
	}
	if err := id.Validate(); err != nil {
		return tobari.ContextDraft{}, err
	}
	templates, err := a.Store.ListWorkspaceTemplates(ctx)
	if err != nil {
		return tobari.ContextDraft{}, err
	}
	found := false
	for _, template := range templates {
		if template.ID == templateID {
			found = true
			break
		}
	}
	if !found {
		return tobari.ContextDraft{}, tobari.ErrWorkspaceTemplateNotFound
	}
	active, err := a.Store.ListContextAuthority(ctx)
	if err != nil {
		return tobari.ContextDraft{}, err
	}
	for _, snapshot := range active {
		if snapshot.Context.ID == id {
			return tobari.ContextDraft{}, tobari.ErrContextBindingExists
		}
	}
	ids, err := a.sources.ListContextIDs(ctx)
	if err != nil {
		return tobari.ContextDraft{}, err
	}
	for _, existingID := range ids {
		source, present, readErr := a.sources.ReadContext(ctx, existingID)
		if readErr != nil {
			return tobari.ContextDraft{}, readErr
		}
		if present && source.ContextID == id {
			return tobari.ContextDraft{}, tobari.ErrContextBindingExists
		}
	}
	source := tobari.ContextSource{SchemaVersion: tobari.ContextSourceSchemaVersion, ContextID: id, TemplateID: templateID}
	if err := source.Validate(); err != nil {
		return tobari.ContextDraft{}, err
	}
	if err := a.sources.PublishContext(ctx, source); err != nil {
		return tobari.ContextDraft{}, err
	}
	path, _ := a.sources.ContextPath(id)
	revision, _ := tobari.ContextSourceSemanticRevision(tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: id, TemplateID: templateID})
	draft := tobari.ContextDraft{Source: source, Observation: tobari.ResourceSourceObservation{Path: path, State: tobari.ResourceSourceModified, SourceRevision: &revision}}
	return draft, draft.Validate()
}

func (a *Adapter) PlanContextSourceByReference(ctx context.Context, ref string) (tobari.ContextActivationPlan, error) {
	id, err := tobari.ParseContextRef(ref)
	if err != nil {
		return tobari.ContextActivationPlan{}, err
	}
	return a.mutator.PlanContextSourceByReference(ctx, ref, a.contextSourceLoader(id))
}

func (a *Adapter) ApplyContextSourceByPlan(ctx context.Context, planRef string) (tobari.ContextAuthoritySnapshot, bool, error) {
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, false, err
	}
	defer releaseCatalog()
	if err := a.requireNoConfiguratorPublicationsLocked(ctx); err != nil {
		return tobari.ContextAuthoritySnapshot{}, false, err
	}
	return a.applyContextSourceByPlan(ctx, planRef)
}

func (a *Adapter) applyContextSourceByPlan(ctx context.Context, planRef string) (tobari.ContextAuthoritySnapshot, bool, error) {
	id, err := tobari.ParseContextActivationPlanRef(planRef)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, false, err
	}
	return a.mutator.ApplyContextSourceByPlan(ctx, planRef, a.contextSourceLoader(id))
}

func (a *Adapter) contextSourceLoader(id tobari.ContextID) workspaceauthoritystore.ContextSourceLoader {
	return func(loadContext context.Context) (tobari.ContextSource, string, error) {
		source, fingerprint, present, err := a.sources.ReadContextSnapshot(loadContext, id)
		if err != nil {
			return tobari.ContextSource{}, "", errors.Join(tobari.ErrResourceSourceInvalid, err)
		}
		if !present {
			return tobari.ContextSource{}, "", tobari.ErrResourceSourceMissing
		}
		return source, fingerprint, nil
	}
}

func (a *Adapter) DeleteContextByReference(ctx context.Context, ref string) (tobari.ContextDeleteResult, error) {
	releaseCatalog, err := a.sources.AcquireConfiguratorCatalogLease(ctx)
	if err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	defer releaseCatalog()
	id, err := tobari.ParseContextRef(ref)
	if err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	if deleted, tombstoneErr := a.Store.IsAuthorityTombstoned("contexts", string(id)); tombstoneErr != nil {
		return tobari.ContextDeleteResult{}, tombstoneErr
	} else if deleted {
		var release func() error
		if a.contextHomes != nil {
			release, err = a.contextHomes.AcquireTombstonedContextHomeRetirement(ctx, id)
			if err != nil {
				return tobari.ContextDeleteResult{}, err
			}
			defer release()
		}
		if err := a.sources.DeleteContext(ctx, id); err != nil && !errors.Is(err, os.ErrNotExist) {
			return tobari.ContextDeleteResult{}, recoveryError("deleted Context source", err)
		}
		if a.contextHomes != nil {
			if err := a.contextHomes.RemoveContextHome(ctx, id); err != nil {
				return tobari.ContextDeleteResult{}, recoveryError("deleted Context Home", err)
			}
		}
		return tobari.ContextDeleteResult{ContextID: id, Deleted: true}, nil
	}
	releaseStage, barrierErr := a.acquireContextDeleteConfiguratorBarrierLocked(ctx, id)
	if barrierErr != nil {
		return tobari.ContextDeleteResult{}, barrierErr
	}
	defer releaseStage()
	var release func() error
	if a.contextHomes != nil {
		_, readErr := a.Store.ReadContextAuthorityByReference(ctx, ref)
		if readErr != nil {
			return tobari.ContextDeleteResult{}, readErr
		}
		release, err = a.contextHomes.AcquireContextHomeRetirement(ctx, id)
		if err != nil {
			return tobari.ContextDeleteResult{}, err
		}
		defer release()
	}
	result, err := a.mutator.DeleteContextByReference(ctx, ref)
	if err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	if err := a.sources.DeleteContext(ctx, id); err != nil && !errors.Is(err, os.ErrNotExist) {
		return tobari.ContextDeleteResult{}, recoveryError("deleted Context source", err)
	}
	if a.contextHomes != nil {
		if err := a.contextHomes.RemoveContextHome(ctx, id); err != nil {
			return tobari.ContextDeleteResult{}, recoveryError("deleted Context Home", err)
		}
	}
	return result, nil
}

func (a *Adapter) requireCurrentTemplateSource(ctx context.Context, ref string) error {
	id, err := tobari.ParseWorkspaceTemplateRef(ref)
	if err != nil {
		return err
	}
	templates, err := a.Store.ListWorkspaceTemplates(ctx)
	if err != nil {
		return err
	}
	for _, template := range templates {
		if template.ID == id {
			return a.requireCurrentTemplateSourceByValue(ctx, template)
		}
	}
	return tobari.ErrWorkspaceTemplateNotFound
}

func (a *Adapter) requireCurrentTemplateSourceByValue(ctx context.Context, template tobari.WorkspaceTemplate) error {
	observation, err := a.ObserveWorkspaceTemplateSource(ctx, template)
	if err != nil {
		return err
	}
	switch observation.State {
	case tobari.ResourceSourceInSync:
		return nil
	case tobari.ResourceSourceMissing:
		return tobari.ErrResourceSourceMissing
	default:
		return tobari.ErrResourceSourceModified
	}
}

func (a *Adapter) requireCurrentContextSource(ctx context.Context, ref string) error {
	snapshot, err := a.Store.ReadContextAuthorityByReference(ctx, ref)
	if err != nil {
		return err
	}
	observation, err := a.ObserveContextSource(ctx, snapshot.Context)
	if err != nil {
		return err
	}
	switch observation.State {
	case tobari.ResourceSourceInSync:
		return nil
	case tobari.ResourceSourceMissing:
		return tobari.ErrResourceSourceMissing
	default:
		return tobari.ErrResourceSourceModified
	}
}

func (a *Adapter) publishTemplate(ctx context.Context, template tobari.WorkspaceTemplate) error {
	source, err := tobari.NewWorkspaceTemplateSource(template)
	if err != nil {
		return err
	}
	return a.sources.PublishTemplate(ctx, source)
}

func (a *Adapter) publishContext(ctx context.Context, binding tobari.ContextBinding) error {
	source, err := tobari.NewContextSource(binding)
	if err != nil {
		return err
	}
	return a.sources.PublishContext(ctx, source)
}

func recoveryError(subject string, err error) error {
	if err == nil {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	return errors.Join(tobari.ErrResourceSourceRecoveryRequired, fmt.Errorf("%s: %w", subject, err))
}

func mustContextRef(id tobari.ContextID) string {
	ref, err := tobari.ContextRef(id)
	if err != nil {
		panic(err)
	}
	return ref
}

func contextSourceRevision(binding tobari.ContextBinding) (tobari.SemanticDigest, error) {
	source, err := tobari.NewContextSource(binding)
	if err != nil {
		return "", err
	}
	return contextSourceDocumentRevision(source)
}

func contextSourceDocumentRevision(source tobari.ContextSource) (tobari.SemanticDigest, error) {
	if err := source.Validate(); err != nil {
		return "", err
	}
	// Context source identity is not an authority digest elsewhere. Reuse the
	// canonical domain digest by constructing the equivalent immutable binding.
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: source.ContextID, TemplateID: source.TemplateID}
	return tobari.ContextSourceSemanticRevision(binding)
}
