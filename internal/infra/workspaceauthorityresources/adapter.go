package workspaceauthorityresources

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritysource"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

func (a *Adapter) CreateWorkspaceTemplateDraft(ctx context.Context, name string, body tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplateDraft, error) {
	if err := tobari.ValidateName(name); err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	if err := body.Validate(); err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	ids, err := a.sources.ListTemplateIDs(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
	}
	for _, id := range ids {
		source, present, readErr := a.sources.ReadTemplate(ctx, id)
		if readErr != nil {
			return tobari.WorkspaceTemplateDraft{}, readErr
		}
		if present && source.Template.Name == name {
			return tobari.WorkspaceTemplateDraft{}, tobari.ErrWorkspaceTemplateExists
		}
	}
	id, err := tobari.IssueWorkspaceTemplateID(time.Now().UTC(), rand.Reader)
	if err != nil {
		return tobari.WorkspaceTemplateDraft{}, err
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
}

func New(store *workspaceauthoritystore.Store, mutator *workspaceauthoritystore.Mutator, sources *workspaceauthoritysource.Store, observeRuntime workspaceauthoritystore.InstallationMigrationRuntimeObserver, prepareRuntime workspaceauthoritystore.InstallationMigrationSourcePreparer) (*Adapter, error) {
	if store == nil || mutator == nil || sources == nil || observeRuntime == nil || prepareRuntime == nil {
		return nil, fmt.Errorf("file-backed final authority requires active and source stores")
	}
	return &Adapter{Store: store, mutator: mutator, sources: sources, observeRuntime: observeRuntime, prepareRuntime: prepareRuntime}, nil
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
	collection, present, err := a.Store.ReadComplete(ctx)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	previous, err := tobari.NewFinalDefaultPairObservation(collection, present, root)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	var template tobari.WorkspaceTemplate
	if !present {
		draft, err := a.CreateWorkspaceTemplateDraft(ctx, tobari.DefaultManifestName, body)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		ref, _ := tobari.WorkspaceTemplateRef(draft.ID)
		plan, err := a.PlanWorkspaceTemplateSourceByReference(ctx, ref)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		applied, err := a.ApplyWorkspaceTemplateSourceByReference(ctx, plan.PlanRef)
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
			return tobari.FinalDefaultPairPublication{}, tobari.ErrDefaultTemplateSelectionRequired
		}
		for _, candidate := range collection.Templates {
			if candidate.ID == *collection.DefaultTemplateID {
				template = candidate.Clone()
				break
			}
		}
		if err := a.requireCurrentTemplateSourceByValue(ctx, template); err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
	}
	current, _, err := a.Store.ReadComplete(ctx)
	if err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	contextPresent := false
	for _, record := range current.Contexts {
		if record.Context.ProjectRoot == root && record.Context.TemplateID == template.ID {
			contextPresent = true
			break
		}
	}
	if !contextPresent {
		templateRef, _ := tobari.WorkspaceTemplateRef(template.ID)
		draft, err := a.CreateContextDraftByTemplateReference(ctx, templateRef, root)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		contextRef, _ := tobari.ContextRef(draft.Source.ContextID)
		plan, err := a.PlanContextSourceByReference(ctx, contextRef)
		if err != nil {
			return tobari.FinalDefaultPairPublication{}, err
		}
		if _, _, err := a.ApplyContextSourceByPlan(ctx, plan.PlanRef); err != nil {
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
	publication := tobari.FinalDefaultPairPublication{Previous: previous, Current: confirmed, Changed: !present || previous.Context == nil}
	if err := publication.ValidateFor(root, body); err != nil {
		return tobari.FinalDefaultPairPublication{}, err
	}
	return publication, nil
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
	return a.mutator.SetDefaultWorkspaceTemplateByReference(ctx, ref)
}

// DeleteWorkspaceByReference is an explicit dependency-checked resource
// deletion, not a Template semantic writer.
func (a *Adapter) DeleteWorkspaceByReference(ctx context.Context, ref string, force bool) (tobari.WorkspaceAuthorityDeleteResult, error) {
	return a.mutator.DeleteWorkspaceByReference(ctx, ref, force)
}

func (a *Adapter) ApplyWorkspaceTemplateSourceByReference(ctx context.Context, ref string) (tobari.WorkspaceTemplateRevisionPublication, error) {
	id, err := tobari.ParseWorkspaceTemplateChangePlanRef(ref)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
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
	return publication, nil
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
	plan, err := tobari.WorkspaceTemplatePolicyMigrationPlanFromRef(ref)
	if err != nil {
		return tobari.WorkspaceTemplatePolicyMigrationResult{}, err
	}
	id, _ := tobari.ParseWorkspaceTemplateRef(plan.TemplateRef)
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
	id, parseErr := tobari.ParseWorkspaceTemplateRef(ref)
	if parseErr != nil {
		return tobari.WorkspaceTemplateDeleteResult{}, parseErr
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
		return tobari.WorkspaceTemplateDeleteResult{}, err
	}
	if err := a.sources.DeleteTemplate(ctx, result.TemplateID); err != nil && !errors.Is(err, os.ErrNotExist) {
		return tobari.WorkspaceTemplateDeleteResult{}, recoveryError("deleted Template source", err)
	}
	return result, nil
}

func (a *Adapter) CreateContextDraftByTemplateReference(ctx context.Context, ref, root string) (tobari.ContextDraft, error) {
	templateID, err := tobari.ParseWorkspaceTemplateRef(ref)
	if err != nil {
		return tobari.ContextDraft{}, err
	}
	if err := tobari.ValidateCanonicalRoot(root); err != nil {
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
		if snapshot.Context.ProjectRoot == root && snapshot.Context.TemplateID == templateID {
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
		if present && source.ProjectRoot == root && source.TemplateID == templateID {
			return tobari.ContextDraft{}, tobari.ErrContextBindingExists
		}
	}
	id, err := tobari.IssueContextID(time.Now().UTC(), rand.Reader)
	if err != nil {
		return tobari.ContextDraft{}, err
	}
	source := tobari.ContextSource{SchemaVersion: tobari.ContextSourceSchemaVersion, ContextID: id, ProjectRoot: root, TemplateID: templateID}
	if err := source.Validate(); err != nil {
		return tobari.ContextDraft{}, err
	}
	if err := a.sources.PublishContext(ctx, source); err != nil {
		return tobari.ContextDraft{}, err
	}
	path, _ := a.sources.ContextPath(id)
	revision, _ := tobari.ContextSourceSemanticRevision(tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: id, ProjectRoot: root, TemplateID: templateID})
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
	id, err := tobari.ParseContextRef(ref)
	if err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	if deleted, tombstoneErr := a.Store.IsAuthorityTombstoned("contexts", string(id)); tombstoneErr != nil {
		return tobari.ContextDeleteResult{}, tombstoneErr
	} else if deleted {
		if err := a.sources.DeleteContext(ctx, id); err != nil && !errors.Is(err, os.ErrNotExist) {
			return tobari.ContextDeleteResult{}, recoveryError("deleted Context source", err)
		}
		return tobari.ContextDeleteResult{ContextID: id, Deleted: true}, nil
	}
	result, err := a.mutator.DeleteContextByReference(ctx, ref)
	if err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	if err := a.sources.DeleteContext(ctx, id); err != nil && !errors.Is(err, os.ErrNotExist) {
		return tobari.ContextDeleteResult{}, recoveryError("deleted Context source", err)
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
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: source.ContextID, ProjectRoot: source.ProjectRoot, TemplateID: source.TemplateID}
	return tobari.ContextSourceSemanticRevision(binding)
}
