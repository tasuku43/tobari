// Package configuratorstore persists non-authoritative working copies and the
// managed Home that is adopted by one Context after reviewed Apply.
package configuratorstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritysource"
)

const (
	metadataFile = "draft.json"
	adoptionFile = "home-adoption.json"
)

type metadata struct {
	SchemaVersion    int                            `json:"schema_version"`
	Draft            tobari.ConfiguratorDraft       `json:"draft"`
	Seed             tobari.ConfiguratorSeed        `json:"seed"`
	AdoptedContextID tobari.ContextID               `json:"adopted_context_id,omitempty"`
	FrozenSubmission *tobari.ConfiguratorSubmission `json:"frozen_submission,omitempty"`
	ApplyConfirmed   bool                           `json:"apply_confirmed,omitempty"`
	Settled          bool                           `json:"settled,omitempty"`
	Retiring         bool                           `json:"retiring,omitempty"`
	Retired          bool                           `json:"retired,omitempty"`
}

type homeAdoption struct {
	SchemaVersion    int                           `json:"schema_version"`
	Phase            string                        `json:"phase"`
	Submission       tobari.ConfiguratorSubmission `json:"submission"`
	TemplateID       tobari.WorkspaceTemplateID    `json:"workspace_template_id,omitempty"`
	TemplateRevision tobari.SemanticDigest         `json:"workspace_template_revision,omitempty"`
	ContextID        tobari.ContextID              `json:"context_id,omitempty"`
}

type Store struct {
	root                    string
	contextHomeRoot         string
	runtimes                RuntimeSourceResolver
	runtimeSources          RuntimeSourceManager
	now                     func() time.Time
	prepareAfterReservation func() error
	retireAfterMarker       func() error
}

type RuntimeSourceResolver interface {
	ResolveWorkspaceTemplateRuntimeSource(context.Context, tobari.RuntimeSourceRef) (tobari.RuntimeBinding, error)
	ResolveWorkspaceTemplateRuntimeSourceWithRetainedBinding(context.Context, tobari.RuntimeSourceRef, tobari.RuntimeBinding) (tobari.RuntimeBinding, error)
}

type RuntimeSourceManager interface {
	PrepareConfiguratorRuntimeSource(context.Context, tobari.ConfiguratorDraft) error
	FreezeConfiguratorRuntimeSource(context.Context, tobari.ConfiguratorDraft) (*tobari.ConfiguratorRuntimeSource, error)
}

func New(root, contextHomeRoot string, runtimes RuntimeSourceResolver, runtimeSources ...RuntimeSourceManager) (*Store, error) {
	for name, value := range map[string]string{"Configurator root": root, "Context Home root": contextHomeRoot} {
		if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value || value == string(filepath.Separator) {
			return nil, fmt.Errorf("%s must be an exact absolute child path", name)
		}
	}
	if root == contextHomeRoot {
		return nil, fmt.Errorf("Configurator and Context Home roots must be distinct")
	}
	if runtimes == nil {
		return nil, fmt.Errorf("Configurator Runtime source resolver is required")
	}
	store := &Store{root: root, contextHomeRoot: contextHomeRoot, runtimes: runtimes, now: time.Now}
	if len(runtimeSources) > 0 {
		store.runtimeSources = runtimeSources[0]
	}
	return store, nil
}

// Reserve durably binds one exact draft identity without touching its managed
// Home. Existing Context Homes are shared with live Workspaces, so production
// callers must acquire the Context attachment lease before Materialize.
func (s *Store) Reserve(ctx context.Context, seed tobari.ConfiguratorSeed, agent tobari.ConfiguratorAgent) (tobari.ConfiguratorDraft, error) {
	scope, err := seed.ConfiguratorScopeKey()
	if err != nil {
		return tobari.ConfiguratorDraft{}, err
	}
	var result tobari.ConfiguratorDraft
	err = s.withScopeLock(ctx, scope, func() error {
		var err error
		result, err = s.reserve(ctx, seed, agent)
		return err
	})
	return result, err
}

func (s *Store) reserve(ctx context.Context, seed tobari.ConfiguratorSeed, agent tobari.ConfiguratorAgent) (tobari.ConfiguratorDraft, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ConfiguratorDraft{}, err
	}
	if err := seed.Validate(); err != nil {
		return tobari.ConfiguratorDraft{}, err
	}
	if seed.Task != tobari.ConfiguratorTaskAggregate {
		if retained, found, err := s.retainedTaskDraft(ctx, seed, agent); err != nil || found {
			return retained, err
		}
	}
	if seed.Task == tobari.ConfiguratorTaskAggregate {
		if _, found, err := s.pendingHomeAdoption(ctx, seed.ProjectRoot); err != nil {
			return tobari.ConfiguratorDraft{}, err
		} else if found {
			return tobari.ConfiguratorDraft{}, fmt.Errorf("Configurator Home adoption must be settled before authoring")
		}
	}
	id, err := tobari.ConfiguratorDraftID(seed, agent)
	if err != nil {
		return tobari.ConfiguratorDraft{}, err
	}
	dir := filepath.Join(s.root, id)
	if current, present, err := s.readMetadata(dir); err != nil {
		return tobari.ConfiguratorDraft{}, err
	} else if present {
		if (current.Settled || current.Retired) && reflect.DeepEqual(current.Seed, seed) && current.Draft.Agent == agent {
			current.Settled = false
			current.Retired = false
			current.FrozenSubmission = nil
			current.ApplyConfirmed = false
			if err := writeAtomicJSON(filepath.Join(dir, metadataFile), current); err != nil {
				return tobari.ConfiguratorDraft{}, err
			}
			return current.Draft, nil
		}
		if current.AdoptedContextID != "" || !reflect.DeepEqual(current.Seed, seed) || current.Draft.Agent != agent {
			return tobari.ConfiguratorDraft{}, fmt.Errorf("retained Configurator draft does not match the requested target")
		}
		return current.Draft, nil
	}
	if err := ensureDurablePrivateDirectoryTree(dir); err != nil {
		return tobari.ConfiguratorDraft{}, err
	}
	templateID := tobari.WorkspaceTemplateID("")
	if seed.Task == tobari.ConfiguratorTaskRuntime {
		// Runtime assistance owns no Template or Context authority.
	} else if seed.Evolution != nil {
		templateID = seed.Evolution.Template.TemplateID
	} else {
		templateID, err = tobari.IssueWorkspaceTemplateID(s.now().UTC(), rand.Reader)
		if err != nil {
			return tobari.ConfiguratorDraft{}, err
		}
	}
	var adoptionContextIDs []tobari.ContextID
	if seed.Task != tobari.ConfiguratorTaskRuntime && (seed.Evolution == nil || seed.Evolution.Context == nil) {
		contextID, issueErr := tobari.IssueContextID(s.now().UTC(), rand.Reader)
		if issueErr != nil {
			return tobari.ConfiguratorDraft{}, issueErr
		}
		adoptionContextIDs = append(adoptionContextIDs, contextID)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, agent, templateID, adoptionContextIDs...)
	if err != nil {
		return tobari.ConfiguratorDraft{}, err
	}
	value := metadata{SchemaVersion: 1, Draft: draft, Seed: seed.Clone()}
	if err := writeExclusiveJSON(filepath.Join(dir, metadataFile), value); err != nil {
		return tobari.ConfiguratorDraft{}, err
	}
	if s.prepareAfterReservation != nil {
		if err := s.prepareAfterReservation(); err != nil {
			return tobari.ConfiguratorDraft{}, err
		}
	}
	return draft, nil
}

func (s *Store) retainedTaskDraft(ctx context.Context, seed tobari.ConfiguratorSeed, agent tobari.ConfiguratorAgent) (tobari.ConfiguratorDraft, bool, error) {
	scope, err := seed.ConfiguratorScopeKey()
	if err != nil {
		return tobari.ConfiguratorDraft{}, false, err
	}
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return tobari.ConfiguratorDraft{}, false, nil
	}
	if err != nil {
		return tobari.ConfiguratorDraft{}, false, err
	}
	var retained *metadata
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == ".locks" {
			continue
		}
		dir := filepath.Join(s.root, entry.Name())
		current, present, err := s.readMetadata(dir)
		if err != nil {
			return tobari.ConfiguratorDraft{}, false, err
		}
		if present && current.Retiring {
			if err := s.finishRetiringRuntimeTask(current); err != nil {
				return tobari.ConfiguratorDraft{}, false, err
			}
			continue
		}
		currentScope := ""
		if present {
			currentScope, err = current.Seed.ConfiguratorScopeKey()
		}
		if err != nil {
			return tobari.ConfiguratorDraft{}, false, err
		}
		if !present || currentScope != scope || current.Settled || current.Retired {
			continue
		}
		if !reflect.DeepEqual(current.Seed, seed) {
			return tobari.ConfiguratorDraft{}, false, tobari.ErrResourceSourceRecoveryRequired
		}
		if current.Draft.Agent != agent {
			return tobari.ConfiguratorDraft{}, false, tobari.ErrResourceSourceRecoveryRequired
		}
		if retained != nil && retained.Draft != current.Draft {
			return tobari.ConfiguratorDraft{}, false, tobari.ErrResourceSourceRecoveryRequired
		}
		copy := current
		retained = &copy
	}
	if retained == nil {
		return tobari.ConfiguratorDraft{}, false, nil
	}
	return retained.Draft, true, nil
}

// Materialize prepares the exact reserved draft below its managed Home. The
// caller owns the Context attachment lease for the complete call.
func (s *Store) Materialize(ctx context.Context, draft tobari.ConfiguratorDraft) error {
	if err := draft.Validate(); err != nil {
		return err
	}
	scope, err := draft.ConfiguratorScopeKey()
	if err != nil {
		return err
	}
	return s.withScopeLock(ctx, scope, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, present, err := s.readMetadata(filepath.Join(s.root, draft.ID))
		if err != nil || !present {
			return errors.Join(fmt.Errorf("Configurator draft reservation is unavailable"), err)
		}
		if current.AdoptedContextID != "" || current.Draft != draft {
			return fmt.Errorf("Configurator draft reservation changed before materialization")
		}
		if draft.Task == tobari.ConfiguratorTaskAggregate {
			if _, found, err := s.pendingHomeAdoption(ctx, draft.ProjectRoot); err != nil {
				return err
			} else if found {
				return fmt.Errorf("Configurator Home adoption must be settled before materialization")
			}
		}
		err = s.prepareDraftMaterial(ctx, current)
		if errors.Is(err, tobari.ErrResourceSourceChanged) && draft.Task == tobari.ConfiguratorTaskRuntime && current.FrozenSubmission == nil {
			if retireErr := s.retireUnmaterializedRuntimeTask(current); retireErr != nil {
				return errors.Join(tobari.ErrConfiguratorTaskRetirementIncomplete, err, retireErr)
			}
		}
		return err
	})
}

func (s *Store) retireUnmaterializedRuntimeTask(current metadata) error {
	if current.Draft.Task != tobari.ConfiguratorTaskRuntime || current.FrozenSubmission != nil || current.Settled || current.Retired {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	if !current.Retiring {
		current.Retiring = true
		if err := writeAtomicJSON(filepath.Join(s.root, current.Draft.ID, metadataFile), current); err != nil {
			return err
		}
	}
	if s.retireAfterMarker != nil {
		if err := s.retireAfterMarker(); err != nil {
			return err
		}
	}
	return s.finishRetiringRuntimeTask(current)
}

func (s *Store) finishRetiringRuntimeTask(current metadata) error {
	draft := current.Draft
	home := s.managedHomePath(draft)
	work := filepath.Dir(workingRoot(home, draft))
	if err := os.RemoveAll(work); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(work)); err != nil {
		return err
	}
	dir := filepath.Join(s.root, draft.ID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == metadataFile {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	current.Retired = true
	current.Retiring = false
	current.Settled = false
	current.ApplyConfirmed = false
	current.FrozenSubmission = nil
	return writeAtomicJSON(filepath.Join(dir, metadataFile), current)
}

// RetireUnmaterializedTask settles a reservation whose exact Context or
// Template authority disappeared before the shared attachment lease could be
// acquired. It never retires frozen or already-published work.
func (s *Store) RetireUnmaterializedTask(ctx context.Context, draft tobari.ConfiguratorDraft) error {
	if draft.Validate() != nil || draft.Task != tobari.ConfiguratorTaskRuntime {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	scope, err := draft.ConfiguratorScopeKey()
	if err != nil {
		return err
	}
	return s.withScopeLock(ctx, scope, func() error {
		current, present, err := s.readMetadata(filepath.Join(s.root, draft.ID))
		if err != nil || !present || current.Draft != draft {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		if current.Retired {
			return nil
		}
		return s.retireUnmaterializedRuntimeTask(current)
	})
}

func (s *Store) prepareDraftMaterial(ctx context.Context, value metadata) error {
	draft, seed := value.Draft, value.Seed
	home, err := s.prepareHome(draft)
	if err != nil {
		return err
	}
	if draft.Task != tobari.ConfiguratorTaskRuntime {
		sourceRoot := sourceRoot(home, draft)
		sourceStore, err := workspaceauthoritysource.New(sourceRoot)
		if err != nil {
			return err
		}
		source, err := tobari.NewWorkspaceTemplateDraftSource(draft.TemplateID, tobari.DefaultManifestName, seed.Initial)
		if err != nil {
			return err
		}
		if _, present, readErr := sourceStore.ReadTemplate(ctx, draft.TemplateID); readErr != nil {
			return readErr
		} else if present {
			// The agent is expected to evolve this valid task source between resumptions.
		} else if err := sourceStore.PublishTemplate(ctx, source); err != nil {
			return err
		}
	}
	workdir := workingRoot(home, draft)
	if err := ensureDurablePrivateDirectoryTree(workdir); err != nil {
		return err
	}
	if err := writeInstructions(workdir, seed); err != nil {
		return err
	}
	if err := writeObserved(workdir, seed); err != nil {
		return err
	}
	if seed.Task == tobari.ConfiguratorTaskPolicy {
		if err := writePolicyReference(workdir); err != nil {
			return err
		}
	}
	if s.runtimeSources != nil {
		if err := s.runtimeSources.PrepareConfiguratorRuntimeSource(ctx, draft); err != nil {
			return err
		}
	}
	return nil
}

// Freeze reads the mutable working copy after the container has exited and
// returns the immutable typed value consumed by review and Apply.
func (s *Store) Freeze(ctx context.Context, draft tobari.ConfiguratorDraft) (tobari.ConfiguratorSubmission, error) {
	if err := draft.Validate(); err != nil {
		return tobari.ConfiguratorSubmission{}, err
	}
	dir := filepath.Join(s.root, draft.ID)
	current, present, err := s.readMetadata(dir)
	if err != nil || !present || current.Draft != draft || current.AdoptedContextID != "" {
		return tobari.ConfiguratorSubmission{}, errors.Join(fmt.Errorf("Configurator draft metadata is unavailable, changed, or already adopted"), err)
	}
	home, err := s.homeForDraft(draft)
	if err != nil {
		return tobari.ConfiguratorSubmission{}, err
	}
	if draft.Task == tobari.ConfiguratorTaskPolicy {
		if err := writePolicyReference(workingRoot(home, draft)); err != nil {
			return tobari.ConfiguratorSubmission{}, fmt.Errorf("policy reference material changed: %w", err)
		}
	}
	body := current.Seed.Initial.Clone()
	var sourceRevision tobari.SemanticDigest
	if draft.Task != tobari.ConfiguratorTaskRuntime {
		store, err := workspaceauthoritysource.New(sourceRoot(home, draft))
		if err != nil {
			return tobari.ConfiguratorSubmission{}, err
		}
		if err := syncConfiguratorTemplateSource(store, draft.TemplateID, home); err != nil {
			return tobari.ConfiguratorSubmission{}, err
		}
		source, present, err := store.ReadTemplate(ctx, draft.TemplateID)
		if err != nil || !present {
			return tobari.ConfiguratorSubmission{}, errors.Join(fmt.Errorf("Configurator working copy is unavailable or invalid"), err)
		}
		resolved, err := s.runtimes.ResolveWorkspaceTemplateRuntimeSourceWithRetainedBinding(
			ctx,
			source.Template.EntryDefaults.Runtime,
			current.Seed.Initial.EntryDefaults.Runtime,
		)
		if err != nil {
			return tobari.ConfiguratorSubmission{}, fmt.Errorf("resolve submitted Runtime source: %w", err)
		}
		body, err = source.Body(resolved)
		if err != nil {
			return tobari.ConfiguratorSubmission{}, err
		}
		sourceRevision, err = source.SemanticRevision(resolved)
		if err != nil {
			return tobari.ConfiguratorSubmission{}, err
		}
		if draft.Task == tobari.ConfiguratorTaskPolicy {
			nonPolicy := body.Clone()
			nonPolicy.Policy = current.Seed.Initial.Policy.Clone()
			if !reflect.DeepEqual(nonPolicy, current.Seed.Initial) {
				return tobari.ConfiguratorSubmission{}, fmt.Errorf("policy assist changed source outside policy.yaml")
			}
		}
	}
	var submission tobari.ConfiguratorSubmission
	if sourceRevision == "" {
		submission, err = tobari.NewConfiguratorSubmission(draft, body)
	} else {
		submission, err = tobari.NewConfiguratorSubmission(draft, body, sourceRevision)
	}
	if err != nil {
		return tobari.ConfiguratorSubmission{}, err
	}
	if s.runtimeSources != nil {
		runtimeSource, err := s.runtimeSources.FreezeConfiguratorRuntimeSource(ctx, draft)
		if err != nil {
			return tobari.ConfiguratorSubmission{}, err
		}
		if runtimeSource != nil {
			submission, err = submission.WithRuntimeSource(*runtimeSource)
			if err != nil {
				return tobari.ConfiguratorSubmission{}, err
			}
		}
	}
	if draft.Task != tobari.ConfiguratorTaskAggregate {
		current.FrozenSubmission = &submission
		current.ApplyConfirmed = false
		current.Settled = false
		if err := writeAtomicJSON(filepath.Join(dir, metadataFile), current); err != nil {
			return tobari.ConfiguratorSubmission{}, err
		}
	}
	return submission, nil
}

// PendingTask returns the one non-settled task generation for a target. The
// caller must verify any confirmed publication against live authority before
// settling it; this Store never infers authority from a newly requested seed.
func (s *Store) PendingTask(ctx context.Context, scope string, task tobari.ConfiguratorTask, targetRuntimeID string) (tobari.ConfiguratorDraft, tobari.ConfiguratorSubmission, bool, bool, error) {
	if scope == "" || (task != tobari.ConfiguratorTaskRuntime && task != tobari.ConfiguratorTaskPolicy) {
		return tobari.ConfiguratorDraft{}, tobari.ConfiguratorSubmission{}, false, false, fmt.Errorf("Configurator task recovery scope is invalid")
	}
	var draft tobari.ConfiguratorDraft
	var submission tobari.ConfiguratorSubmission
	var frozen, confirmed bool
	err := s.withScopeLock(ctx, scope, func() error {
		entries, err := os.ReadDir(s.root)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Name() == ".locks" {
				continue
			}
			dir := filepath.Join(s.root, entry.Name())
			legacyAggregate, err := isPreReleaseAggregateMetadata(dir)
			if err != nil {
				return err
			}
			if legacyAggregate {
				continue
			}
			current, present, err := s.readMetadata(dir)
			if err != nil {
				return err
			}
			if present && current.Retiring {
				if err := s.finishRetiringRuntimeTask(current); err != nil {
					return err
				}
				continue
			}
			currentScope := ""
			if present {
				currentScope, err = current.Seed.ConfiguratorScopeKey()
			}
			if err != nil {
				return err
			}
			if !present || current.Settled || current.Retired || currentScope != scope || current.Draft.Task != task || current.Draft.TargetRuntimeID != targetRuntimeID {
				continue
			}
			if draft.ID != "" && draft != current.Draft {
				return tobari.ErrResourceSourceRecoveryRequired
			}
			draft = current.Draft
			confirmed = current.ApplyConfirmed
			if current.FrozenSubmission != nil {
				submission = *current.FrozenSubmission
				frozen = true
			}
		}
		return nil
	})
	return draft, submission, frozen, confirmed, err
}

// isPreReleaseAggregateMetadata recognizes only the exact deterministic V2
// aggregate identity used before task-scoped assistance existed. Those
// non-authoritative receipts are neither adopted nor deleted; current task
// recovery simply excludes them as required by the product contract.
func isPreReleaseAggregateMetadata(dir string) (bool, error) {
	data, present, err := readBoundedPrivateFile(filepath.Join(dir, metadataFile), 256<<10)
	if err != nil || !present {
		return false, err
	}
	var value struct {
		SchemaVersion int `json:"schema_version"`
		Draft         struct {
			ID              string                     `json:"id"`
			ProjectRoot     string                     `json:"project_root"`
			Agent           tobari.ConfiguratorAgent   `json:"agent"`
			Task            *tobari.ConfiguratorTask   `json:"task"`
			Purpose         tobari.ConfiguratorPurpose `json:"purpose"`
			Runtime         tobari.RuntimeBinding      `json:"runtime"`
			TargetRuntimeID string                     `json:"target_runtime_id"`
		} `json:"draft"`
		Seed struct {
			ProjectRoot           string                     `json:"project_root"`
			Task                  *tobari.ConfiguratorTask   `json:"task"`
			Purpose               tobari.ConfiguratorPurpose `json:"purpose"`
			ExecutionRuntime      tobari.RuntimeBinding      `json:"execution_runtime"`
			TargetRuntimeID       string                     `json:"target_runtime_id"`
			TargetRuntimeRevision tobari.SemanticDigest      `json:"target_runtime_revision"`
			Evolution             *struct {
				Template struct {
					Revision tobari.SemanticDigest `json:"revision"`
				} `json:"template"`
				Context *struct {
					ID tobari.ContextID `json:"id"`
				} `json:"context"`
				PolicyMemory *struct {
					Revision tobari.SemanticDigest `json:"revision"`
				} `json:"policy_memory"`
			} `json:"evolution"`
		} `json:"seed"`
	}
	if json.Unmarshal(data, &value) != nil || value.SchemaVersion != 1 || value.Draft.Task != nil || value.Seed.Task != nil || value.Draft.TargetRuntimeID != "" || value.Seed.TargetRuntimeID != "" || value.Seed.TargetRuntimeRevision != "" {
		return false, nil
	}
	if value.Draft.ProjectRoot != value.Seed.ProjectRoot || value.Draft.Purpose != value.Seed.Purpose || value.Draft.Runtime != value.Seed.ExecutionRuntime || value.Draft.Agent.Validate() != nil || value.Draft.Purpose.Validate() != nil || tobari.ValidateCanonicalRoot(value.Draft.ProjectRoot) != nil || value.Draft.Runtime.Validate() != nil {
		return false, nil
	}
	base := "tobari-configurator-v2\x00" + value.Draft.ProjectRoot + "\x00" + string(value.Draft.Agent) + "\x00" + string(value.Draft.Purpose) + "\x00" + value.Draft.Runtime.RuntimeID + "\x00" + value.Draft.Runtime.Revision
	if value.Seed.Evolution != nil {
		base += "\x00" + string(value.Seed.Evolution.Template.Revision)
		if value.Seed.Evolution.Context != nil && value.Seed.Evolution.PolicyMemory != nil {
			base += "\x00" + string(value.Seed.Evolution.Context.ID) + "\x00" + string(value.Seed.Evolution.PolicyMemory.Revision)
		}
	}
	digest := sha256.Sum256([]byte(base))
	want := "cfg1_" + hex.EncodeToString(digest[:])
	return value.Draft.ID == want && filepath.Base(dir) == want, nil
}

// ConfirmTask durably records the user's exact Apply decision before any
// canonical publication. A later process can therefore distinguish a reviewed
// no-op from an unconfirmed frozen draft.
func (s *Store) ConfirmTask(ctx context.Context, submission tobari.ConfiguratorSubmission) error {
	if submission.Validate() != nil || submission.Draft.Task == tobari.ConfiguratorTaskAggregate {
		return fmt.Errorf("Configurator task confirmation is invalid")
	}
	scope, err := submission.Draft.ConfiguratorScopeKey()
	if err != nil {
		return err
	}
	return s.withScopeLock(ctx, scope, func() error {
		dir := filepath.Join(s.root, submission.Draft.ID)
		current, present, err := s.readMetadata(dir)
		if err != nil || !present || current.Retiring || current.Retired || current.Settled || current.FrozenSubmission == nil || !reflect.DeepEqual(*current.FrozenSubmission, submission) {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		if current.ApplyConfirmed {
			return nil
		}
		current.ApplyConfirmed = true
		return writeAtomicJSON(filepath.Join(dir, metadataFile), current)
	})
}

// CompleteTask durably retires one exact reviewed task generation after its
// canonical publication has succeeded. The managed Context Home is retained.
func (s *Store) CompleteTask(ctx context.Context, submission tobari.ConfiguratorSubmission) error {
	if submission.Validate() != nil || submission.Draft.Task == tobari.ConfiguratorTaskAggregate {
		return fmt.Errorf("Configurator task settlement is invalid")
	}
	scope, err := submission.Draft.ConfiguratorScopeKey()
	if err != nil {
		return err
	}
	return s.withScopeLock(ctx, scope, func() error {
		dir := filepath.Join(s.root, submission.Draft.ID)
		current, present, err := s.readMetadata(dir)
		if err != nil || !present {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		if current.Settled {
			if current.FrozenSubmission != nil && reflect.DeepEqual(*current.FrozenSubmission, submission) {
				return nil
			}
			return tobari.ErrResourceSourceRecoveryRequired
		}
		if current.FrozenSubmission == nil || !reflect.DeepEqual(*current.FrozenSubmission, submission) {
			return tobari.ErrResourceSourceRecoveryRequired
		}
		if !current.ApplyConfirmed {
			return tobari.ErrResourceSourceRecoveryRequired
		}
		current.Settled = true
		return writeAtomicJSON(filepath.Join(dir, metadataFile), current)
	})
}

// ArmHomeAdoption durably binds a not-yet-owned draft Home to the exact frozen
// submission before any Template or Context authority can be published.
func (s *Store) ArmHomeAdoption(ctx context.Context, submission tobari.ConfiguratorSubmission) error {
	return s.withProjectLock(ctx, submission.Draft.ProjectRoot, func() error {
		return s.armHomeAdoption(ctx, submission)
	})
}

func (s *Store) armHomeAdoption(ctx context.Context, submission tobari.ConfiguratorSubmission) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := submission.Validate(); err != nil || !submission.Draft.NeedsHomeAdoption() {
		return fmt.Errorf("Configurator Home adoption arm is invalid: %w", err)
	}
	pendingExact := false
	if existing, found, err := s.pendingHomeAdoption(ctx, submission.Draft.ProjectRoot); err != nil {
		return err
	} else if found {
		if !reflect.DeepEqual(existing, submission) {
			return fmt.Errorf("another Configurator Home adoption is pending for this Project")
		}
		pendingExact = true
	}
	dir := filepath.Join(s.root, submission.Draft.ID)
	if pendingExact {
		journal, present, err := readAdoption(filepath.Join(dir, adoptionFile))
		if err != nil || !present {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		// Only armed is guaranteed to precede authority publication and Home
		// rename. Later phases may legitimately be in the rename→phase-write
		// crash gap; AdoptHome owns their exact source/target replay checks.
		if journal.Phase != "armed" {
			return nil
		}
	}
	current, present, err := s.readMetadata(dir)
	if err != nil || !present || current.Draft != submission.Draft || current.AdoptedContextID != "" {
		return errors.Join(fmt.Errorf("Configurator draft metadata is unavailable, changed, or already adopted"), err)
	}
	frozen, err := s.Freeze(ctx, submission.Draft)
	if err != nil || !reflect.DeepEqual(frozen, submission) {
		return errors.Join(fmt.Errorf("Configurator managed Home no longer matches the frozen submission"), err)
	}
	if pendingExact {
		return nil
	}
	return writeExclusiveJSON(filepath.Join(dir, adoptionFile), homeAdoption{SchemaVersion: 1, Phase: "armed", Submission: submission, TemplateID: submission.Draft.TemplateID, TemplateRevision: submission.SourceRevision, ContextID: submission.Draft.AdoptionContextID})
}

// PendingHomeAdoption returns the one exact frozen Project receipt, if any.
func (s *Store) PendingHomeAdoption(ctx context.Context, projectRoot string) (tobari.ConfiguratorSubmission, bool, error) {
	var result tobari.ConfiguratorSubmission
	var found bool
	err := s.withProjectLock(ctx, projectRoot, func() error {
		var err error
		result, found, err = s.pendingHomeAdoption(ctx, projectRoot)
		return err
	})
	return result, found, err
}

func (s *Store) pendingHomeAdoption(ctx context.Context, projectRoot string) (tobari.ConfiguratorSubmission, bool, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ConfiguratorSubmission{}, false, err
	}
	if tobari.ValidateCanonicalRoot(projectRoot) != nil {
		return tobari.ConfiguratorSubmission{}, false, fmt.Errorf("pending Home adoption Project root is invalid")
	}
	entries, err := os.ReadDir(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return tobari.ConfiguratorSubmission{}, false, nil
	}
	if err != nil {
		return tobari.ConfiguratorSubmission{}, false, err
	}
	var result tobari.ConfiguratorSubmission
	found := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(s.root, entry.Name())
		metadata, present, readErr := s.readMetadata(dir)
		if readErr != nil {
			return tobari.ConfiguratorSubmission{}, false, readErr
		}
		if !present || metadata.Draft.ProjectRoot != projectRoot {
			continue
		}
		adoption, present, readErr := readAdoption(filepath.Join(dir, adoptionFile))
		if readErr != nil {
			return tobari.ConfiguratorSubmission{}, false, readErr
		}
		if !present {
			continue
		}
		if validateHomeAdoption(adoption) != nil || adoption.Submission.Draft != metadata.Draft || adoption.TemplateID != adoption.Submission.Draft.TemplateID || adoption.TemplateRevision != adoption.Submission.SourceRevision || adoption.ContextID != adoption.Submission.Draft.AdoptionContextID {
			return tobari.ConfiguratorSubmission{}, false, fmt.Errorf("Configurator Home adoption receipt is invalid")
		}
		if adoption.Submission.Draft.Task == tobari.ConfiguratorTaskAggregate {
			continue
		}
		if found && !reflect.DeepEqual(result, adoption.Submission) {
			return tobari.ConfiguratorSubmission{}, false, fmt.Errorf("multiple Configurator Home adoptions target this Project")
		}
		result, found = adoption.Submission, true
	}
	return result, found, nil
}

// AdoptHome atomically transfers an armed draft Home to its resulting Context.
// The publication snapshot is the receipt: an arbitrary Context ID is never
// enough to select an adoption target.
func (s *Store) AdoptHome(ctx context.Context, submission tobari.ConfiguratorSubmission, snapshot tobari.ContextAuthoritySnapshot, settle ...func() error) error {
	return s.withProjectLock(ctx, submission.Draft.ProjectRoot, func() error {
		return s.adoptHome(ctx, submission, snapshot, settle...)
	})
}

func (s *Store) adoptHome(ctx context.Context, submission tobari.ConfiguratorSubmission, snapshot tobari.ContextAuthoritySnapshot, settle ...func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	draft := submission.Draft
	if err := submission.Validate(); err != nil {
		return fmt.Errorf("Configurator Home adoption submission is invalid: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("Configurator Home adoption publication is invalid: %w", err)
	}
	if !draft.NeedsHomeAdoption() || snapshot.Workspace == nil || snapshot.Context.ID != draft.AdoptionContextID || snapshot.Workspace.ProjectRoot != draft.ProjectRoot || snapshot.Context.TemplateID != draft.TemplateID || snapshot.Template.ID != draft.TemplateID || snapshot.Template.Current.Revision != submission.SourceRevision || !reflect.DeepEqual(snapshot.Template.Current.Body, submission.Body) {
		return fmt.Errorf("Configurator Home adoption request does not match its publication")
	}
	if draft.Purpose == tobari.ConfiguratorPurposeEvolve && snapshot.Template.ID != draft.TemplateID {
		return fmt.Errorf("Configurator Home adoption published another Template")
	}
	contextID := snapshot.Context.ID
	dir := filepath.Join(s.root, draft.ID)
	current, present, err := s.readMetadata(dir)
	if err != nil || !present || current.Draft != draft {
		return errors.Join(fmt.Errorf("Configurator draft metadata is unavailable or changed"), err)
	}
	if current.AdoptedContextID != "" {
		if current.AdoptedContextID != contextID {
			return fmt.Errorf("Configurator Home was adopted by another Context")
		}
		if _, err := requirePrivateDirectory(s.contextHome(contextID)); err != nil {
			return err
		}
		journalPath := filepath.Join(dir, adoptionFile)
		if existing, present, readErr := readAdoption(journalPath); readErr != nil {
			return readErr
		} else if present {
			if !adoptionMatchesPublication(existing, submission, snapshot) {
				return fmt.Errorf("Configurator Home adoption receipt changed after metadata commit")
			}
			if len(settle) > 0 && settle[0] != nil {
				if err := settle[0](); err != nil {
					return err
				}
			}
			if err := os.Remove(journalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return syncDirectory(dir)
		}
		return nil
	}
	source := filepath.Join(dir, "home")
	target := s.contextHome(contextID)
	journalPath := filepath.Join(dir, adoptionFile)
	journal := homeAdoption{SchemaVersion: 1, Phase: "authority_published", Submission: submission, TemplateID: snapshot.Template.ID, TemplateRevision: snapshot.Template.Current.Revision, ContextID: contextID}
	if existing, present, readErr := readAdoption(journalPath); readErr != nil {
		return readErr
	} else if present && !reflect.DeepEqual(existing, journal) && !adoptionMatchesPublication(existing, submission, snapshot) {
		return fmt.Errorf("another Configurator Home adoption is active")
	} else if !present {
		return fmt.Errorf("Configurator Home adoption was not armed before publication")
	} else if existing.Phase == "armed" {
		if err := writeAtomicJSON(journalPath, journal); err != nil {
			return err
		}
	}
	if _, err := requirePrivateDirectory(target); err == nil {
		if _, sourceErr := os.Lstat(source); !errors.Is(sourceErr, os.ErrNotExist) {
			return fmt.Errorf("Configurator Home adoption target already exists before source retirement")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		targetParent := filepath.Dir(target)
		if err := ensureDurablePrivateDirectoryTree(targetParent); err != nil {
			return err
		}
		// Persist the complete Context Home ancestor chain before retiring the
		// only durable draft Home, then persist the adopted Home entry inside
		// the exact Context-ID directory after the rename.
		if _, err := requirePrivateDirectory(source); err != nil {
			return fmt.Errorf("Configurator draft Home is unsafe: %w", err)
		}
		if err := os.Rename(source, target); err != nil {
			return fmt.Errorf("adopt Configurator Home: %w", err)
		}
		if err := syncDirectory(filepath.Dir(source)); err != nil {
			return err
		}
		if err := syncDirectory(targetParent); err != nil {
			return err
		}
	} else {
		return err
	}
	journal.Phase = "home_renamed"
	if err := writeAtomicJSON(journalPath, journal); err != nil {
		return err
	}
	current.AdoptedContextID = contextID
	if err := writeAtomicJSON(filepath.Join(dir, metadataFile), current); err != nil {
		return err
	}
	journal.Phase = "metadata_committed"
	if err := writeAtomicJSON(journalPath, journal); err != nil {
		return err
	}
	if len(settle) > 0 && settle[0] != nil {
		if err := settle[0](); err != nil {
			return err
		}
	}
	if err := os.Remove(journalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(dir)
}

func adoptionMatchesPublication(adoption homeAdoption, submission tobari.ConfiguratorSubmission, snapshot tobari.ContextAuthoritySnapshot) bool {
	return validateHomeAdoption(adoption) == nil && reflect.DeepEqual(adoption.Submission, submission) && adoption.TemplateID == snapshot.Template.ID && adoption.TemplateRevision == snapshot.Template.Current.Revision && adoption.ContextID == snapshot.Context.ID
}

func (s *Store) withProjectLock(ctx context.Context, projectRoot string, action func() error) error {
	if tobari.ValidateCanonicalRoot(projectRoot) != nil {
		return fmt.Errorf("Configurator Project lock target is invalid")
	}
	return s.withScopeLock(ctx, "project-"+projectRoot, action)
}

func (s *Store) withScopeLock(ctx context.Context, scope string, action func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if scope == "" || len(scope) > 4096 || action == nil {
		return fmt.Errorf("Configurator scope lock target is invalid")
	}
	lockRoot := filepath.Join(s.root, ".locks")
	if err := ensureDurablePrivateDirectoryTree(lockRoot); err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(scope))
	path := filepath.Join(lockRoot, hex.EncodeToString(digest[:])+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- digest-named child of a private Configurator root.
	if err != nil {
		return err
	}
	locked, lockErr := tryLockProjectFile(file)
	if lockErr != nil || !locked {
		_ = file.Close()
		return errors.Join(tobari.ErrContextBindingProtected, lockErr)
	}
	defer func() {
		unlockProjectFile(file)
		_ = file.Close()
	}()
	return action()
}

func (s *Store) prepareHome(draft tobari.ConfiguratorDraft) (string, error) {
	home := s.managedHomePath(draft)
	if err := ensureDurablePrivateDirectoryTree(home); err != nil {
		return "", err
	}
	agentStateDirectory := ""
	switch draft.Agent {
	case tobari.ConfiguratorAgentCodex:
		agentStateDirectory = ".codex"
	case tobari.ConfiguratorAgentClaude:
		agentStateDirectory = ".claude"
	default:
		return "", fmt.Errorf("Configurator agent state owner is invalid")
	}
	// Codex rejects an explicitly selected CODEX_HOME when the directory is
	// absent. The Store owns managed-Home materialization, so establish the
	// selected client's owner-only native state root before container handoff.
	if err := ensureDurablePrivateDirectoryTree(filepath.Join(home, agentStateDirectory)); err != nil {
		return "", fmt.Errorf("prepare Configurator agent state: %w", err)
	}
	return home, nil
}

func (s *Store) homeForDraft(draft tobari.ConfiguratorDraft) (string, error) {
	home := s.managedHomePath(draft)
	if _, err := requirePrivateDirectory(home); err != nil {
		return "", fmt.Errorf("Configurator managed Home is unsafe: %w", err)
	}
	return home, nil
}

func (s *Store) managedHomePath(draft tobari.ConfiguratorDraft) string {
	if draft.UsesInstallationHome() {
		return filepath.Join(s.root, ".runtime-assist-homes", string(draft.Agent), "home")
	}
	if draft.ContextID != "" {
		return s.contextHome(draft.ContextID)
	}
	return filepath.Join(s.root, draft.ID, "home")
}

func (s *Store) contextHome(id tobari.ContextID) string {
	return filepath.Join(s.contextHomeRoot, string(id), "home")
}

func workingRoot(home string, draft tobari.ConfiguratorDraft) string {
	return filepath.Join(home, ".tobari", "configurator", draft.ID, "working")
}

func sourceRoot(home string, draft tobari.ConfiguratorDraft) string {
	return filepath.Join(workingRoot(home, draft), "configuration")
}

func syncConfiguratorTemplateSource(store *workspaceauthoritysource.Store, id tobari.WorkspaceTemplateID, home string) error {
	path, err := store.TemplatePath(id)
	if err != nil {
		return err
	}
	for _, filePath := range []string{path, filepath.Join(filepath.Dir(path), "policy.yaml")} {
		info, err := os.Lstat(filePath)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("Configurator working source file is unsafe")
		}
		file, err := os.Open(filePath) // #nosec G304 -- exact Template source paths below the managed Home.
		if err != nil {
			return err
		}
		syncErr := file.Sync()
		closeErr := file.Close()
		if syncErr != nil || closeErr != nil {
			return errors.Join(syncErr, closeErr)
		}
	}
	for current := filepath.Dir(path); ; current = filepath.Dir(current) {
		if _, err := requirePrivateDirectory(current); err != nil {
			return err
		}
		if err := syncDirectory(current); err != nil {
			return err
		}
		if current == home {
			break
		}
		if current == filepath.Dir(current) || !strings.HasPrefix(current, home+string(filepath.Separator)) {
			return fmt.Errorf("Configurator working source escaped its managed Home")
		}
	}
	return nil
}

func (s *Store) readMetadata(dir string) (metadata, bool, error) {
	path := filepath.Join(dir, metadataFile)
	data, present, err := readBoundedPrivateFile(path, 256<<10)
	if err != nil || !present {
		return metadata{}, present, err
	}
	var value metadata
	if err := json.Unmarshal(data, &value); err != nil || value.SchemaVersion != 1 || value.Draft.Validate() != nil || value.Seed.Validate() != nil {
		return metadata{}, false, errors.Join(fmt.Errorf("Configurator draft metadata is invalid"), err)
	}
	want, err := tobari.ConfiguratorDraftID(value.Seed, value.Draft.Agent)
	if err != nil || want != value.Draft.ID || value.Seed.ProjectRoot != value.Draft.ProjectRoot || value.Seed.Runtime() != value.Draft.Runtime {
		return metadata{}, false, fmt.Errorf("Configurator draft metadata is inconsistent")
	}
	if value.AdoptedContextID != "" && (value.AdoptedContextID.Validate() != nil || !value.Draft.NeedsHomeAdoption()) {
		return metadata{}, false, fmt.Errorf("Configurator Home adoption metadata is invalid")
	}
	if value.FrozenSubmission != nil && (value.Draft.Task == tobari.ConfiguratorTaskAggregate || value.FrozenSubmission.Validate() != nil || value.FrozenSubmission.Draft != value.Draft) {
		return metadata{}, false, fmt.Errorf("Configurator frozen task submission is invalid")
	}
	if value.Settled && value.FrozenSubmission == nil {
		return metadata{}, false, fmt.Errorf("settled Configurator task lacks its frozen submission")
	}
	if value.ApplyConfirmed && (value.FrozenSubmission == nil || value.Retiring || value.Retired) {
		return metadata{}, false, fmt.Errorf("confirmed Configurator task metadata is invalid")
	}
	if value.Retiring && (value.Draft.Task != tobari.ConfiguratorTaskRuntime || value.FrozenSubmission != nil || value.Settled || value.Retired) {
		return metadata{}, false, fmt.Errorf("retiring Configurator task metadata is invalid")
	}
	if value.Retired && (value.Draft.Task != tobari.ConfiguratorTaskRuntime || value.FrozenSubmission != nil || value.Settled || value.Retiring) {
		return metadata{}, false, fmt.Errorf("retired Configurator task metadata is invalid")
	}
	return value, true, nil
}

func writeInstructions(dir string, seed tobari.ConfiguratorSeed) error {
	purpose := `You are creating the first Tobari configuration for this Project. Begin by understanding the user's development work, then help author the complete initial Template, static policy, Runtime binding or source request, typed bootstrap, shell, and Git defaults.`
	if seed.Purpose == tobari.ConfiguratorPurposeEvolve {
		purpose = `You are updating the existing Tobari configuration for this Project. Read observed.json before proposing changes. Use the current Template, Policy Memory, and Runtime binding as evidence. Policy Memory is read-only: suggest deliberate promotion into static policy when appropriate, but never claim to edit Policy Memory directly. Runtime source and build changes remain valid configuration work.`
	}
	if seed.Task == tobari.ConfiguratorTaskRuntime {
		purpose = `You are assisting with one managed Tobari Runtime source. Inspect runtime/source/Dockerfile and its bounded build context, then ask concrete questions about tools, language runtimes, package managers, and commands that must work. Edit only runtime/source/. Do not edit Template policy, Policy Memory, Context, Workspace defaults, or Project files.`
	}
	if seed.Task == tobari.ConfiguratorTaskPolicy {
		purpose = `You are assisting with one Tobari static Template policy. Read observed.json and the read-only policy reference material, then propose the smallest semantic rules justified by the user's reviewed evidence. Edit only configuration/templates/*/policy.yaml. Policy Memory is read-only evidence and must not be rewritten or erased.`
	}
	editable := `Edit only the resource source below configuration/ in this attachment directory.`
	if seed.Task == tobari.ConfiguratorTaskRuntime {
		editable = `Edit only runtime/source/ in this attachment directory.`
	}
	if seed.Task == tobari.ConfiguratorTaskPolicy {
		editable = `Edit only configuration/templates/<template-id>/policy.yaml in this attachment directory.`
	}
	agents := `# Tobari Configurator

` + purpose + `

Start in English. If the user's first substantive response is in another language, continue in that language. Preserve English machine keys, identifiers, schemas, and repository documentation.

` + editable + ` It is non-authoritative. The host Tobari process freezes and validates a submission after you exit, presents semantic review, and alone can Apply. Do not claim that edits are active.

This session has direct Internet access without Workspace Gateway/OPA policy. Its complete Tobari-managed Home is mounted read-write so Runtime-provided tools can use their native state. Project files, host Home, Docker socket, and live Tobari authority are unavailable. Do not ask the user to weaken those boundaries.
`
	if seed.Task == tobari.ConfiguratorTaskRuntime {
		agents += "\nThe Runtime being edited is not the Runtime executing this agent. The target source is runtime/source/. Tobari will freeze and review source publication on the host; a later Runtime build is a separate confirmed action.\n"
	} else if seed.Runtime().RuntimeID == tobari.StandardRuntimeID {
		agents += "\nThe standard Runtime source is immutable in this session. You may bind the Template to another existing exact Runtime revision, but do not claim to edit the standard Runtime.\n"
	} else {
		agents += "\nThe selected managed Runtime's editable working copy is under runtime/source/. You may edit it when the user's request requires Runtime changes. Tobari freezes, content-addresses, builds, and reviews it on the host; never edit the Template Runtime revision by guessing a future digest.\n"
	}
	if err := ensureExactPrivateFile(filepath.Join(dir, "AGENTS.md"), []byte(agents)); err != nil {
		return err
	}
	return ensureExactPrivateFile(filepath.Join(dir, "CLAUDE.md"), []byte("Read and follow AGENTS.md in this directory.\n"))
}

func writeObserved(dir string, seed tobari.ConfiguratorSeed) error {
	value := struct {
		SchemaVersion         int                                   `json:"schema_version"`
		Task                  tobari.ConfiguratorTask               `json:"task"`
		Purpose               tobari.ConfiguratorPurpose            `json:"purpose"`
		ProjectRoot           string                                `json:"project_root"`
		TargetRuntime         string                                `json:"target_runtime_id,omitempty"`
		TargetRuntimeRevision tobari.SemanticDigest                 `json:"target_runtime_revision,omitempty"`
		Evolution             *tobari.ConfiguratorEvolutionSnapshot `json:"evolution,omitempty"`
	}{1, seed.Task, seed.Purpose, seed.ProjectRoot, seed.TargetRuntimeID, seed.TargetRuntimeRevision, seed.Evolution}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return ensureExactPrivateFile(filepath.Join(dir, "observed.json"), append(data, '\n'))
}

const policyReference = `# Tobari static policy reference

This file is a Tobari-owned, read-only knowledge pack. The editable target is
only configuration/templates/<template-id>/policy.yaml. Treat observed.json,
including its Policy Memory snapshot, as evidence; never edit or replace it.

## Evaluation order

1. Boundary is evaluated first. The V1 destination ceiling is public HTTPS;
   boundary.methods.deny is the exact uppercase method ceiling.
2. An admitted request is projected into one closed semantic module.
3. Exact static Deny is terminal. A matching static Allow can admit the request
   only inside the Boundary. Context Policy Memory then supplies reviewed exact
   Allow or Deny decisions. Anything unresolved requires exact review and is
   denied unless that review confirms it.
4. Static policy is Template authority. Policy Memory is separate, exact,
   Context-owned authority; copying useful evidence into static policy is a
   deliberate widening that must remain visible in host review.

## Source envelope

policy.yaml uses schema tobari.dev/template-policy/v1 and keeps the existing
workspace_template_id. Required top-level containers are boundary and semantic.
Preserve the current file as the canonical YAML example; do not invent keys.

boundary:
  methods:
    deny: []       # exact uppercase HTTP methods only

semantic owns agent_profile, native_readiness, protocols, and providers. The
closed rule locations are:

- semantic.protocols.http.generic.<allow|deny>.rules
- semantic.protocols.http.graphql.<allow|deny>.rules
- semantic.protocols.http.mcp.<allow|deny>.rules
- semantic.protocols.http.git.<allow|deny>.rules
- semantic.protocols.http.oci.<allow|deny>.rules
- semantic.providers.aws.<allow|deny>.rules
- semantic.providers.kubernetes.<allow|deny>.rules

An absent module means known-none. A present module must contain explicit
non-null allow.rules and deny.rules arrays. Collections are semantic sets:
ordering grants no meaning, exact duplicates are invalid, and users author no
rule IDs. An Allow shadowed by exact Deny or by the method Boundary is invalid.

## Rule precision

Every rule has exact scheme and port, exactly one of host or hosts, and the
closed matcher for its module. Generic HTTP has one exact method and either an
exact path or a reviewed path template. A path template may contain exactly one
full {id} segment; never use glob syntax or partial-segment wildcards.

GraphQL rules use trusted declared endpoints plus exact operation type/root
field. MCP rules use trusted endpoint plus exact method/tool. Git rules use the
validated smart-HTTP service and repository identity. OCI rules use exact
distribution action, repository, and object coordinates.

AWS rules describe the observed wire request, not IAM: exact SigV4 service,
wire protocol, protocol version or target namespace where applicable, and a
case-sensitive operation matcher. The only supported widening is one terminal
* in the operation matcher. Never infer ARN, IAM action, access level,
idempotence, or retry safety.

Kubernetes rules describe the already-classified request: exact verb plus API
group/version/resource/namespace/name/subresource/dry-run dimensions, or an
exact non-resource path. Do not infer RBAC, discovery, CRD/OpenAPI semantics,
object-body meaning, or impersonation.

Prefer the smallest rules justified by observed evidence. When evidence is
insufficient, leave the operation at exact review rather than guessing a broad
static Allow.
`

func writePolicyReference(dir string) error {
	return ensureExactPrivateFile(filepath.Join(dir, "POLICY.md"), []byte(policyReference))
}

func ensureExactPrivateFile(path string, expected []byte) error {
	data, present, err := readBoundedPrivateFile(path, 256<<10)
	if err != nil {
		return err
	}
	if present {
		if !bytes.Equal(data, expected) {
			return fmt.Errorf("retained Configurator material changed before resume")
		}
		return nil
	}
	return writeExclusive(path, expected)
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(path, 0o700); err != nil { // #nosec G302 -- managed state is owner-only.
		return err
	}
	_, err := requirePrivateDirectory(path)
	return err
}

func ensureDurablePrivateDirectoryTree(path string) error {
	missing := []string{}
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("Configurator directory ancestor must be a real directory")
			}
			if err := syncDirectory(filepath.Dir(current)); err != nil {
				return err
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return fmt.Errorf("Configurator directory has no existing ancestor")
		}
	}
	for index := len(missing) - 1; index >= 0; index-- {
		created := missing[index]
		if err := os.Mkdir(created, 0o700); err != nil {
			return err
		}
		if _, err := requirePrivateDirectory(created); err != nil {
			return err
		}
		if err := syncDirectory(filepath.Dir(created)); err != nil {
			return err
		}
	}
	if _, err := requirePrivateDirectory(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func requirePrivateDirectory(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("directory must be owner-only and not a symlink")
	}
	return info, nil
}

func readBoundedPrivateFile(path string, limit int64) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() <= 0 || info.Size() > limit {
		return nil, false, errors.Join(fmt.Errorf("private state file is unsafe"), err)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- callers pass fixed filenames below validated managed roots.
	return data, true, err
}

func readAdoption(path string) (homeAdoption, bool, error) {
	data, present, err := readBoundedPrivateFile(path, 64<<10)
	if err != nil || !present {
		return homeAdoption{}, present, err
	}
	var value homeAdoption
	if err := json.Unmarshal(data, &value); err != nil || validateHomeAdoption(value) != nil {
		return homeAdoption{}, false, errors.Join(fmt.Errorf("Configurator Home adoption journal is invalid"), err)
	}
	return value, true, nil
}

func validateHomeAdoption(value homeAdoption) error {
	if value.SchemaVersion != 1 || value.Submission.Validate() != nil || !value.Submission.Draft.NeedsHomeAdoption() {
		return fmt.Errorf("Configurator Home adoption journal identity is invalid")
	}
	switch value.Phase {
	case "armed":
		if value.TemplateID != value.Submission.Draft.TemplateID || value.TemplateRevision != value.Submission.SourceRevision || value.ContextID != value.Submission.Draft.AdoptionContextID {
			return fmt.Errorf("armed Configurator Home adoption lost its reserved publication identity")
		}
	case "authority_published", "home_renamed", "metadata_committed":
		if value.TemplateID != value.Submission.Draft.TemplateID || value.TemplateRevision != value.Submission.SourceRevision || value.ContextID.Validate() != nil {
			return fmt.Errorf("Configurator Home adoption publication receipt is invalid")
		}
	default:
		return fmt.Errorf("Configurator Home adoption phase is invalid")
	}
	return nil
}

func configuratorDraftIDPatternForStore(value string) bool {
	if len(value) != len("cfg1_")+64 || value[:len("cfg1_")] != "cfg1_" {
		return false
	}
	for _, ch := range value[len("cfg1_"):] {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}

func writeExclusiveJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeExclusive(path, append(data, '\n'))
}

func writeAtomicJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".configurator-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func writeExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- callers use fixed names below validated owner-only roots.
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func syncDirectory(path string) error {
	dir, err := os.Open(path) // #nosec G304 -- exact managed directory selected by caller.
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
