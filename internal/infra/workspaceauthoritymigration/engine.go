// Package workspaceauthoritymigration owns the dormant journaled cutover from
// the exact WP11 predecessor authority into the final Workspace authority
// envelope. No current command or ordinary reader selects this engine yet.
package workspaceauthoritymigration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

const (
	journalSchemaVersion = 1
	journalFileName      = "journal.json"
	lockFileName         = "migration.lock"
	finalStageSuffix     = ".wp11-stage"
	rolledFinalSuffix    = ".wp11-rolled-back"
	authorityFileName    = "authority.json"
	maxJournalBytes      = 96 << 20
	maxSourceBytes       = 128 << 20
	maxSourceEntries     = 64 * 1024
)

type SourceKind string

const (
	SourceFile      SourceKind = "file"
	SourceDirectory SourceKind = "directory"
)

// SourceItem is one exact predecessor reader input. Key is a private stable
// journal key, not a public identity. Cutoff is the one item whose removal
// makes the predecessor ordinary reader unavailable; it always moves first
// and restores last.
type SourceItem struct {
	Key          string                `json:"key"`
	Path         string                `json:"path"`
	BackupPath   string                `json:"backup_path"`
	Kind         SourceKind            `json:"kind"`
	Digest       tobari.SemanticDigest `json:"digest"`
	Cutoff       bool                  `json:"cutoff"`
	Research     bool                  `json:"research"`
	LinuxRootKey bool                  `json:"linux_root_key"`
}

type PreservedHome struct {
	WorkspaceID tobari.WorkspaceID    `json:"workspace_id"`
	Path        string                `json:"path"`
	Digest      tobari.SemanticDigest `json:"digest"`
}

type PreparedPredecessor struct {
	Input          tobari.WorkspaceAuthorityMigrationInput
	Sources        []SourceItem
	StandardHomes  []PreservedHome
	FreshAuthPaths []string
}

// Quiescence is supplied by the exact Docker/attachment observer selected at
// the later atomic cutover. The engine calls it under its exclusive lock and
// does not accept equivalent booleans from persisted predecessor data.
type Quiescence struct {
	ClusterStopped  bool
	LiveAttachments int
}

type ReaderDisposition struct {
	PredecessorComplete    bool
	PredecessorUnavailable bool
	FinalComplete          bool
	FinalAbsent            bool
}

type PreflightPort interface {
	Prepare(context.Context) (PreparedPredecessor, error)
	ObserveQuiescence(context.Context) (Quiescence, error)
	ObserveReaders(context.Context) (ReaderDisposition, error)
}

type Result struct {
	Changed             bool
	SourceDigest        tobari.SemanticDigest
	PlanDigest          tobari.SemanticDigest
	CollectionRevision  tobari.SemanticDigest
	ContextAssignments  []tobari.ContextIDAssignment
	ResearchDisposition tobari.ResearchAuthDisposition
}

type journalPhase string

const (
	phasePrepared       journalPhase = "prepared"
	phaseMoving         journalPhase = "moving_predecessor"
	phaseFinalPublished journalPhase = "final_published"
	phaseCommitted      journalPhase = "committed"
	phaseRollback       journalPhase = "rollback"
	phaseRolledBack     journalPhase = "rolled_back"
)

type migrationJournal struct {
	SchemaVersion       int                                 `json:"schema_version"`
	Phase               journalPhase                        `json:"phase"`
	Moved               int                                 `json:"moved"`
	SourceDigest        tobari.SemanticDigest               `json:"source_digest"`
	ResearchDigest      tobari.SemanticDigest               `json:"research_digest,omitempty"`
	PlanDigest          tobari.SemanticDigest               `json:"plan_digest"`
	Sources             []SourceItem                        `json:"sources"`
	StandardHomes       []PreservedHome                     `json:"standard_homes"`
	FreshAuthPaths      []string                            `json:"fresh_auth_paths"`
	ContextAssignments  []tobari.ContextIDAssignment        `json:"context_assignments"`
	ResearchDisposition tobari.ResearchAuthDisposition      `json:"research_auth_disposition"`
	Collection          tobari.WorkspaceAuthorityCollection `json:"collection"`
}

type Engine struct {
	finalRoot          string
	transactionRoot    string
	preflight          PreflightPort
	rename             func(string, string) error
	removeAll          func(string) error
	afterJournalRename func() error
	afterStageMkdir    func() error
	afterStageWrite    func() error
}

func New(finalRoot, transactionRoot string, preflight PreflightPort) (*Engine, error) {
	if !exactAbsoluteChild(finalRoot) || !exactAbsoluteChild(transactionRoot) || preflight == nil || pathsOverlap(finalRoot, transactionRoot) {
		return nil, fmt.Errorf("Workspace authority migration paths or preflight are invalid")
	}
	return &Engine{finalRoot: finalRoot, transactionRoot: transactionRoot, preflight: preflight, rename: os.Rename, removeAll: os.RemoveAll}, nil
}

// Apply is also the sole forward recovery seam. A repeated call resumes the
// journaled direction; once committed it verifies exact read-back and returns
// Changed=false without regenerating Context IDs or overwriting backup.
func (e *Engine) Apply(ctx context.Context) (Result, error) {
	if err := e.ensureTransactionRoot(); err != nil {
		return Result{}, err
	}
	unlock, err := e.acquireLock()
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	journal, present, err := e.readJournal()
	if err != nil {
		return Result{}, err
	}
	if !present {
		journal, err = e.prepare(ctx)
		if err != nil {
			return Result{}, err
		}
		if err := e.writeJournal(journal); err != nil {
			return Result{}, err
		}
	} else if journal.Phase == phaseRollback {
		if err := e.resumeRollback(ctx, &journal); err != nil {
			return Result{}, err
		}
		return Result{}, fmt.Errorf("Workspace authority migration rollback completed; a reviewed new migration transaction is required")
	} else if journal.Phase == phaseRolledBack {
		if err := e.validateRolledBack(ctx, journal); err != nil {
			return Result{}, err
		}
		return Result{}, fmt.Errorf("Workspace authority migration rollback is terminal; a reviewed new migration transaction is required")
	}
	if journal.Phase == phaseCommitted {
		if err := e.validateCommitted(ctx, journal); err != nil {
			return Result{}, err
		}
		return resultFromJournal(journal, false), nil
	}
	if err := e.resumeForward(ctx, &journal); err != nil {
		return Result{}, err
	}
	return resultFromJournal(journal, true), nil
}

// Rollback is an internal cutover seam, not a new public recovery command. It
// never merges. Final authority becomes unavailable before predecessor items
// restore, and the old reader cutoff restores last.
func (e *Engine) Rollback(ctx context.Context) error {
	if err := e.ensureTransactionRoot(); err != nil {
		return err
	}
	unlock, err := e.acquireLock()
	if err != nil {
		return err
	}
	defer unlock()
	journal, present, err := e.readJournal()
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("Workspace authority migration journal is absent")
	}
	if journal.Phase == phaseRolledBack {
		return e.validateRolledBack(ctx, journal)
	}
	if journal.Phase != phaseCommitted && journal.Phase != phaseFinalPublished && journal.Phase != phaseMoving && journal.Phase != phasePrepared && journal.Phase != phaseRollback {
		return fmt.Errorf("Workspace authority migration journal phase cannot roll back")
	}
	if err := e.requireQuiescence(ctx); err != nil {
		return err
	}
	journal.Phase = phaseRollback
	if err := e.writeJournal(journal); err != nil {
		return err
	}
	return e.resumeRollback(ctx, &journal)
}

func (e *Engine) prepare(ctx context.Context) (migrationJournal, error) {
	if err := ctx.Err(); err != nil {
		return migrationJournal{}, err
	}
	prepared, err := e.preflight.Prepare(ctx)
	if err != nil {
		return migrationJournal{}, fmt.Errorf("prepare exact Workspace authority predecessor: %w", err)
	}
	if err := validatePrepared(prepared, e.finalRoot, e.transactionRoot); err != nil {
		return migrationJournal{}, err
	}
	quiescence, err := e.preflight.ObserveQuiescence(ctx)
	if err != nil || !quiescence.ClusterStopped || quiescence.LiveAttachments != 0 {
		return migrationJournal{}, fmt.Errorf("Workspace authority migration requires stopped cluster and zero attachments")
	}
	disposition, err := e.preflight.ObserveReaders(ctx)
	if err != nil || !disposition.PredecessorComplete || !disposition.FinalAbsent || disposition.PredecessorUnavailable || disposition.FinalComplete {
		return migrationJournal{}, fmt.Errorf("predecessor/final reader disposition is not safe before migration")
	}

	sources := append([]SourceItem{}, prepared.Sources...)
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Cutoff != sources[j].Cutoff {
			return sources[i].Cutoff
		}
		return sources[i].Key < sources[j].Key
	})
	sourceDigest, researchDigest, err := validateAndDigestSources(sources)
	if err != nil {
		return migrationJournal{}, err
	}
	if err := validateHomes(prepared.StandardHomes, prepared.Input.Workspaces, sources); err != nil {
		return migrationJournal{}, err
	}
	finalStore, err := workspaceauthoritystore.New(e.finalRoot)
	if err != nil {
		return migrationJournal{}, err
	}
	if _, present, err := finalStore.ReadComplete(ctx); err != nil || present {
		return migrationJournal{}, fmt.Errorf("final Workspace authority already exists or is unsafe")
	}
	for _, reserved := range []string{e.finalRoot + finalStageSuffix, e.finalRoot + rolledFinalSuffix} {
		if _, err := os.Lstat(reserved); err == nil || !errors.Is(err, os.ErrNotExist) {
			return migrationJournal{}, fmt.Errorf("reserved final Workspace authority path is not absent before migration")
		}
	}

	input := prepared.Input
	input.Source = tobari.WorkspaceAuthorityMigrationSource
	input.SourceDigest = sourceDigest
	input.PredecessorComplete = true
	input.FinalAuthorityPresent = false
	input.ClusterStopped = quiescence.ClusterStopped
	input.LiveAttachments = quiescence.LiveAttachments
	if input.ResearchAuthority.Present {
		if err := validateResearchSources(sources, input.ResearchAuthority.Platform); err != nil {
			return migrationJournal{}, err
		}
		input.ResearchAuthority.Complete = researchDigest != ""
		input.ResearchAuthority.SourceDigest = researchDigest
	} else if researchDigest != "" {
		return migrationJournal{}, fmt.Errorf("research source exists without predecessor research authority")
	}
	plan, err := tobari.BuildWorkspaceAuthorityMigrationPlan(input)
	if err != nil {
		return migrationJournal{}, fmt.Errorf("build exact Workspace authority migration plan: %w", err)
	}
	collection, err := collectionFromPlan(plan)
	if err != nil {
		return migrationJournal{}, err
	}
	if _, err := workspaceauthoritystore.EncodeComplete(collection); err != nil {
		return migrationJournal{}, fmt.Errorf("final Workspace authority is not publishable before migration: %w", err)
	}
	journal := migrationJournal{
		SchemaVersion: journalSchemaVersion, Phase: phasePrepared, SourceDigest: sourceDigest,
		ResearchDigest: researchDigest, PlanDigest: plan.PlanDigest, Sources: sources,
		StandardHomes:       append([]PreservedHome{}, prepared.StandardHomes...),
		FreshAuthPaths:      append([]string{}, prepared.FreshAuthPaths...),
		ContextAssignments:  append([]tobari.ContextIDAssignment{}, plan.ContextAssignments...),
		ResearchDisposition: plan.ResearchAuthDisposition, Collection: collection,
	}
	if err := journal.Validate(e.finalRoot, e.transactionRoot); err != nil {
		return migrationJournal{}, err
	}
	return journal, nil
}

func collectionFromPlan(plan tobari.WorkspaceAuthorityMigrationPlan) (tobari.WorkspaceAuthorityCollection, error) {
	if err := plan.Validate(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, err
	}
	memoryByContext := make(map[tobari.ContextID]tobari.PolicyMemoryRevision, len(plan.PolicyMemories))
	for _, memory := range plan.PolicyMemories {
		memoryByContext[memory.ContextID] = memory
	}
	records := make([]tobari.WorkspaceAuthorityContextRecord, 0, len(plan.Contexts))
	for _, binding := range plan.Contexts {
		memory, exists := memoryByContext[binding.ID]
		if !exists {
			return tobari.WorkspaceAuthorityCollection{}, fmt.Errorf("migration plan Context has no Policy Memory")
		}
		records = append(records, tobari.WorkspaceAuthorityContextRecord{Context: binding, PolicyMemory: memory})
	}
	workspaces := make([]tobari.WorkspaceBinding, len(plan.Workspaces))
	for index, workspace := range plan.Workspaces {
		workspaces[index] = workspace.Binding
	}
	candidates := make([]tobari.PolicyCandidateAuthority, len(plan.PendingCandidates))
	for index, migrated := range plan.PendingCandidates {
		authority, err := migrated.Authority()
		if err != nil {
			return tobari.WorkspaceAuthorityCollection{}, err
		}
		candidates[index] = authority
	}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(plan.Templates, records, workspaces, candidates, plan.DefaultTemplateID, nil)
	return collection, err
}

func (e *Engine) resumeForward(ctx context.Context, journal *migrationJournal) error {
	if err := journal.Validate(e.finalRoot, e.transactionRoot); err != nil {
		return err
	}
	if err := e.validateHomes(journal.StandardHomes); err != nil {
		return err
	}
	if err := e.requireQuiescence(ctx); err != nil {
		return err
	}
	if journal.Phase == phasePrepared {
		if err := e.publishFinal(ctx, journal.Collection); err != nil {
			return err
		}
		journal.Phase = phaseFinalPublished
		if err := e.writeJournal(*journal); err != nil {
			return err
		}
	}
	if journal.Phase == phaseFinalPublished {
		cutoff := journal.Sources[0]
		canonical, backup := observeExactPath(cutoff.Path, cutoff), observeExactPath(e.sourceBackupPath(cutoff), cutoff)
		if canonical == pathAbsent && backup == pathExact {
			disposition, err := e.preflight.ObserveReaders(ctx)
			if err != nil || !disposition.PredecessorUnavailable || !disposition.FinalComplete || disposition.PredecessorComplete || disposition.FinalAbsent {
				return fmt.Errorf("interrupted cutoff did not leave complete final authority selected")
			}
			journal.Phase, journal.Moved = phaseMoving, 1
			if err := e.writeJournal(*journal); err != nil {
				return err
			}
		} else {
			if canonical != pathExact || backup != pathAbsent {
				return fmt.Errorf("migration cutoff disposition is ambiguous")
			}
			if err := e.validateCanonicalSources(*journal); err != nil {
				return fmt.Errorf("predecessor changed before atomic reader cutoff: %w", err)
			}
			disposition, err := e.preflight.ObserveReaders(ctx)
			if err != nil || !disposition.PredecessorComplete || disposition.PredecessorUnavailable || disposition.FinalComplete || disposition.FinalAbsent {
				return fmt.Errorf("complete predecessor was not selected immediately before migration cutoff")
			}
		}
	}
	if journal.Phase == phaseFinalPublished || journal.Phase == phaseMoving {
		journal.Phase = phaseMoving
		for index := journal.Moved; index < len(journal.Sources); index++ {
			if err := e.requireQuiescence(ctx); err != nil {
				return err
			}
			if err := e.moveSource(journal.Sources[index], false); err != nil {
				return err
			}
			journal.Moved = index + 1
			if err := e.writeJournal(*journal); err != nil {
				return err
			}
			if index == 0 {
				disposition, err := e.preflight.ObserveReaders(ctx)
				if err != nil || !disposition.PredecessorUnavailable || !disposition.FinalComplete || disposition.PredecessorComplete || disposition.FinalAbsent {
					return fmt.Errorf("migration cutoff did not atomically select complete final authority")
				}
			}
		}
	}
	if journal.Phase == phaseMoving && journal.Moved == len(journal.Sources) {
		if err := e.validateFinal(ctx, journal.Collection); err != nil {
			return err
		}
		disposition, err := e.preflight.ObserveReaders(ctx)
		if err != nil || !disposition.PredecessorUnavailable || !disposition.FinalComplete || disposition.PredecessorComplete || disposition.FinalAbsent {
			return fmt.Errorf("reader disposition is not complete after final publication")
		}
		journal.Phase = phaseCommitted
		if err := e.writeJournal(*journal); err != nil {
			return err
		}
	}
	return e.validateCommitted(ctx, *journal)
}

func (e *Engine) validateCommitted(ctx context.Context, journal migrationJournal) error {
	if journal.Phase != phaseCommitted || journal.Moved != len(journal.Sources) {
		return fmt.Errorf("Workspace authority migration is not completely committed")
	}
	if err := e.validateBackupSources(journal); err != nil {
		return err
	}
	if err := e.validateFinal(ctx, journal.Collection); err != nil {
		return err
	}
	if err := e.validateHomes(journal.StandardHomes); err != nil {
		return err
	}
	disposition, err := e.preflight.ObserveReaders(ctx)
	if err != nil || !disposition.PredecessorUnavailable || !disposition.FinalComplete || disposition.PredecessorComplete || disposition.FinalAbsent {
		return fmt.Errorf("committed reader disposition drifted")
	}
	return nil
}

func (e *Engine) resumeRollback(ctx context.Context, journal *migrationJournal) error {
	if err := journal.Validate(e.finalRoot, e.transactionRoot); err != nil {
		return err
	}
	if err := e.requireQuiescence(ctx); err != nil {
		return err
	}
	for _, path := range journal.FreshAuthPaths {
		if _, err := os.Lstat(path); err == nil || !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("fresh canonical authentication state blocks rollback")
		}
	}
	if err := e.removeOwnedFinalStage(ctx, journal.Collection); err != nil {
		return err
	}
	for index := journal.Moved - 1; index >= 0; index-- {
		if err := e.requireQuiescence(ctx); err != nil {
			return err
		}
		if err := e.moveSource(journal.Sources[index], true); err != nil {
			return err
		}
		journal.Moved = index
		if err := e.writeJournal(*journal); err != nil {
			return err
		}
		if index == 0 {
			disposition, err := e.preflight.ObserveReaders(ctx)
			if err != nil || !disposition.PredecessorComplete || disposition.PredecessorUnavailable || disposition.FinalComplete || disposition.FinalAbsent {
				return fmt.Errorf("rollback cutoff did not atomically restore complete predecessor authority")
			}
		}
	}
	// The cutoff is source index zero and restores last. Until that exact
	// rename, final authority remains selected. Only after the predecessor is
	// complete again may final authority leave its canonical path.
	if err := e.requireQuiescence(ctx); err != nil {
		return err
	}
	if err := e.retireFinal(ctx, journal.Collection); err != nil {
		return err
	}
	journal.Phase = phaseRolledBack
	if err := e.writeJournal(*journal); err != nil {
		return err
	}
	return e.validateRolledBack(ctx, *journal)
}

func (e *Engine) requireQuiescence(ctx context.Context) error {
	quiescence, err := e.preflight.ObserveQuiescence(ctx)
	if err != nil || !quiescence.ClusterStopped || quiescence.LiveAttachments != 0 {
		return fmt.Errorf("Workspace authority migration requires stopped cluster and zero attachments before resumed mutation")
	}
	return nil
}

func (e *Engine) validateRolledBack(ctx context.Context, journal migrationJournal) error {
	if journal.Phase != phaseRolledBack || journal.Moved != 0 {
		return fmt.Errorf("Workspace authority rollback is incomplete")
	}
	if err := e.validateCanonicalSources(journal); err != nil {
		return err
	}
	store, err := workspaceauthoritystore.New(e.finalRoot)
	if err != nil {
		return err
	}
	if _, present, err := store.ReadComplete(ctx); err != nil || present {
		return fmt.Errorf("final authority remained reachable after rollback")
	}
	if _, err := os.Lstat(e.finalRoot + finalStageSuffix); err == nil || !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("final authority stage remained after rollback")
	}
	rolled := e.finalRoot + rolledFinalSuffix
	if _, err := os.Lstat(rolled); err == nil {
		if err := validateStoredCollection(ctx, rolled, journal.Collection); err != nil {
			return fmt.Errorf("private rolled final authority drifted: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := e.validateHomes(journal.StandardHomes); err != nil {
		return err
	}
	disposition, err := e.preflight.ObserveReaders(ctx)
	if err != nil || !disposition.PredecessorComplete || !disposition.FinalAbsent || disposition.PredecessorUnavailable || disposition.FinalComplete {
		return fmt.Errorf("rolled-back reader disposition is not exact predecessor")
	}
	return nil
}

func resultFromJournal(journal migrationJournal, changed bool) Result {
	return Result{
		Changed: changed, SourceDigest: journal.SourceDigest, PlanDigest: journal.PlanDigest,
		CollectionRevision:  journal.Collection.Revision,
		ContextAssignments:  append([]tobari.ContextIDAssignment{}, journal.ContextAssignments...),
		ResearchDisposition: journal.ResearchDisposition,
	}
}

func (e *Engine) sourceBackupPath(item SourceItem) string {
	return item.BackupPath
}

func (e *Engine) moveSource(item SourceItem, restore bool) error {
	source, target := item.Path, e.sourceBackupPath(item)
	if restore {
		source, target = target, item.Path
	}
	sourceState := observeExactPath(source, item)
	targetState := observeExactPath(target, item)
	if sourceState == pathAbsent && targetState == pathExact {
		return nil
	}
	if sourceState != pathExact || targetState != pathAbsent {
		return fmt.Errorf("predecessor authority %q has an ambiguous move disposition", item.Key)
	}
	err := e.rename(source, target)
	if err == nil {
		return syncRenameParents(source, target)
	}
	sourceState = observeExactPath(source, item)
	targetState = observeExactPath(target, item)
	if sourceState == pathAbsent && targetState == pathExact {
		return syncRenameParents(source, target)
	}
	return fmt.Errorf("move predecessor authority %q is unknown after rename failure: %w", item.Key, err)
}

func (e *Engine) publishFinal(ctx context.Context, collection tobari.WorkspaceAuthorityCollection) error {
	if err := e.validateFinal(ctx, collection); err == nil {
		return nil
	}
	if _, err := os.Lstat(e.finalRoot); err == nil || !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("final Workspace authority canonical path is occupied or unsafe")
	}
	stage := e.finalRoot + finalStageSuffix
	if _, err := os.Lstat(stage); err == nil {
		stageStore, _ := workspaceauthoritystore.New(stage)
		_, present, readErr := stageStore.ReadComplete(ctx)
		if readErr == nil && present {
			if err := validateStoredCollection(ctx, stage, collection); err != nil {
				return fmt.Errorf("final Workspace authority stage contains different complete authority")
			}
		} else {
			if err := e.reconcileOwnedPartialStage(stage); err != nil {
				return fmt.Errorf("final Workspace authority stage is ambiguous: %w", err)
			}
			if err := os.Mkdir(stage, 0o700); err != nil {
				return err
			}
			if e.afterStageMkdir != nil {
				if err := e.afterStageMkdir(); err != nil {
					return err
				}
			}
			if err := e.writeFinalStage(stage, collection); err != nil {
				return err
			}
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(stage, 0o700); err != nil {
			return err
		}
		if e.afterStageMkdir != nil {
			if err := e.afterStageMkdir(); err != nil {
				return err
			}
		}
		if err := e.writeFinalStage(stage, collection); err != nil {
			return err
		}
	} else {
		return err
	}
	if err := validateStoredCollection(ctx, stage, collection); err != nil {
		return fmt.Errorf("read back final Workspace authority stage: %w", err)
	}
	err := e.rename(stage, e.finalRoot)
	if err == nil {
		return syncRenameParents(stage, e.finalRoot)
	}
	if validateErr := e.validateFinal(ctx, collection); validateErr == nil {
		return syncRenameParents(stage, e.finalRoot)
	}
	return fmt.Errorf("final Workspace authority publication is unknown after rename failure: %w", err)
}

func (e *Engine) writeFinalStage(stage string, collection tobari.WorkspaceAuthorityCollection) error {
	data, err := workspaceauthoritystore.EncodeComplete(collection)
	if err != nil {
		return err
	}
	if err := writePrivateFile(filepath.Join(stage, authorityFileName), data); err != nil {
		return err
	}
	if e.afterStageWrite != nil {
		if err := e.afterStageWrite(); err != nil {
			return err
		}
	}
	return syncDirectory(stage)
}

func (e *Engine) reconcileOwnedPartialStage(stage string) error {
	before, err := os.Lstat(stage)
	if err != nil || !safeDirectory(before) {
		return fmt.Errorf("partial stage root is unsafe")
	}
	entries, err := os.ReadDir(stage)
	if err != nil || len(entries) > 1 {
		return fmt.Errorf("partial stage contents are not engine-owned")
	}
	if len(entries) == 1 {
		if entries[0].Name() != authorityFileName {
			return fmt.Errorf("partial stage contains an unknown entry")
		}
		info, err := os.Lstat(filepath.Join(stage, authorityFileName))
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) || info.Size() < 0 || info.Size() > maxJournalBytes {
			return fmt.Errorf("partial stage authority file is unsafe")
		}
	}
	after, err := os.Lstat(stage)
	if err != nil || !os.SameFile(before, after) || after.Mode() != before.Mode() {
		return fmt.Errorf("partial stage changed during reconciliation")
	}
	if err := e.removeAll(stage); err != nil { // #nosec G301 -- exact reserved engine-owned stage was validated above.
		return err
	}
	return syncDirectory(filepath.Dir(stage))
}

func (e *Engine) removeOwnedFinalStage(ctx context.Context, collection tobari.WorkspaceAuthorityCollection) error {
	stage := e.finalRoot + finalStageSuffix
	if _, err := os.Lstat(stage); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Lstat(e.finalRoot); err == nil || !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("canonical final authority and its stage both exist")
	}
	store, err := workspaceauthoritystore.New(stage)
	if err != nil {
		return err
	}
	_, present, readErr := store.ReadComplete(ctx)
	if readErr != nil || !present {
		return e.reconcileOwnedPartialStage(stage)
	}
	if err := validateStoredCollection(ctx, stage, collection); err != nil {
		return fmt.Errorf("complete final authority stage differs from the journal: %w", err)
	}
	if err := e.removeAll(stage); err != nil { // #nosec G301 -- exact noncanonical journal-owned final stage was validated above.
		return err
	}
	return syncDirectory(filepath.Dir(stage))
}

func (e *Engine) retireFinal(ctx context.Context, collection tobari.WorkspaceAuthorityCollection) error {
	rolled := e.finalRoot + rolledFinalSuffix
	if _, err := os.Lstat(e.finalRoot); errors.Is(err, os.ErrNotExist) {
		if _, rolledErr := os.Lstat(rolled); rolledErr == nil {
			return validateStoredCollection(ctx, rolled, collection)
		}
		return nil
	}
	if err := e.validateFinal(ctx, collection); err != nil {
		return fmt.Errorf("final Workspace authority drift blocks rollback: %w", err)
	}
	if _, err := os.Lstat(rolled); err == nil {
		if err := validateStoredCollection(ctx, rolled, collection); err != nil {
			return fmt.Errorf("rolled-back final authority target is ambiguous: %w", err)
		}
		return fmt.Errorf("canonical and rolled-back final authority both exist; rollback will not delete or merge either copy")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	err := e.rename(e.finalRoot, rolled)
	if err == nil {
		return syncRenameParents(e.finalRoot, rolled)
	}
	if _, finalErr := os.Lstat(e.finalRoot); errors.Is(finalErr, os.ErrNotExist) {
		store, _ := workspaceauthoritystore.New(rolled)
		stored, present, readErr := store.ReadComplete(ctx)
		if readErr == nil && present && stored.Revision == collection.Revision {
			return syncRenameParents(e.finalRoot, rolled)
		}
	}
	return fmt.Errorf("retire final Workspace authority is unknown after rename failure: %w", err)
}

func (e *Engine) validateFinal(ctx context.Context, want tobari.WorkspaceAuthorityCollection) error {
	return validateStoredCollection(ctx, e.finalRoot, want)
}

func validateStoredCollection(ctx context.Context, root string, want tobari.WorkspaceAuthorityCollection) error {
	store, err := workspaceauthoritystore.New(root)
	if err != nil {
		return err
	}
	got, present, err := store.ReadComplete(ctx)
	if err != nil || !present || got.Revision != want.Revision || got.Generation != want.Generation {
		return fmt.Errorf("final Workspace authority does not match journaled collection")
	}
	return nil
}

func (e *Engine) validateBackupSources(journal migrationJournal) error {
	for _, item := range journal.Sources {
		if observeExactPath(item.Path, item) != pathAbsent || observeExactPath(e.sourceBackupPath(item), item) != pathExact {
			return fmt.Errorf("predecessor backup is incomplete or canonical authority reappeared")
		}
	}
	return nil
}

func (e *Engine) validateCanonicalSources(journal migrationJournal) error {
	for _, item := range journal.Sources {
		if observeExactPath(item.Path, item) != pathExact || observeExactPath(e.sourceBackupPath(item), item) != pathAbsent {
			return fmt.Errorf("canonical predecessor authority is incomplete or backup remains mixed")
		}
	}
	return nil
}

func (e *Engine) validateHomes(homes []PreservedHome) error { return validateHomePaths(homes) }

func (e *Engine) ensureTransactionRoot() error {
	parent := filepath.Dir(e.transactionRoot)
	info, err := os.Lstat(parent)
	if err != nil || !safeDirectory(info) {
		return fmt.Errorf("Workspace authority migration parent must be a real owner-only directory")
	}
	if err := os.Mkdir(e.transactionRoot, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err = os.Lstat(e.transactionRoot)
	if err != nil || !safeDirectory(info) {
		return fmt.Errorf("Workspace authority migration root must be a real owner-only directory")
	}
	return nil
}

func (e *Engine) acquireLock() (func(), error) {
	path := filepath.Join(e.transactionRoot, lockFileName)
	before, statErr := os.Lstat(path)
	if statErr == nil && (before.Mode()&os.ModeSymlink != 0 || !before.Mode().IsRegular() || before.Mode().Perm() != 0o600 || !ownedByCurrentUser(before)) {
		return nil, fmt.Errorf("Workspace authority migration lock is unsafe")
	}
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("Workspace authority migration is already active or lock is unsafe: %w", err)
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0o600 || !ownedByCurrentUser(opened) {
		_ = file.Close()
		return nil, fmt.Errorf("Workspace authority migration lock is unsafe after open")
	}
	if before != nil && !os.SameFile(before, opened) {
		_ = file.Close()
		return nil, fmt.Errorf("Workspace authority migration lock changed during safe open")
	}
	if err := lockExclusive(file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("Workspace authority migration is already active: %w", err)
	}
	return func() {
		_ = unlockExclusive(file)
		_ = file.Close()
	}, nil
}

func (e *Engine) journalPath() string { return filepath.Join(e.transactionRoot, journalFileName) }

func (e *Engine) readJournal() (migrationJournal, bool, error) {
	data, err := readPrivateFile(e.journalPath(), maxJournalBytes)
	if errors.Is(err, os.ErrNotExist) {
		return migrationJournal{}, false, nil
	}
	if err != nil {
		return migrationJournal{}, false, err
	}
	var journal migrationJournal
	if err := decodeStrictJSON(data, &journal); err != nil {
		return migrationJournal{}, false, fmt.Errorf("Workspace authority migration journal is invalid: %w", err)
	}
	if err := journal.Validate(e.finalRoot, e.transactionRoot); err != nil {
		return migrationJournal{}, false, err
	}
	return journal, true, nil
}

func (e *Engine) writeJournal(journal migrationJournal) error {
	if err := journal.Validate(e.finalRoot, e.transactionRoot); err != nil {
		return err
	}
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > maxJournalBytes {
		return fmt.Errorf("Workspace authority migration journal exceeds %d bytes", maxJournalBytes)
	}
	file, err := os.CreateTemp(e.transactionRoot, ".journal-stage-")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
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
	err = e.rename(temporary, e.journalPath())
	if err == nil && e.afterJournalRename != nil {
		err = e.afterJournalRename()
	}
	if err == nil {
		return syncDirectory(e.transactionRoot)
	}
	readBack, readErr := readPrivateFile(e.journalPath(), maxJournalBytes)
	if readErr == nil && bytes.Equal(readBack, data) {
		return syncDirectory(e.transactionRoot)
	}
	return fmt.Errorf("Workspace authority migration journal publication is unknown: %w", err)
}

func (j migrationJournal) Validate(finalRoot, transactionRoot string) error {
	if j.SchemaVersion != journalSchemaVersion || j.SourceDigest.Validate() != nil || j.PlanDigest.Validate() != nil || j.Sources == nil || len(j.Sources) == 0 || j.StandardHomes == nil || j.FreshAuthPaths == nil || j.ContextAssignments == nil {
		return fmt.Errorf("Workspace authority migration journal is incomplete")
	}
	if j.Phase != phasePrepared && j.Phase != phaseMoving && j.Phase != phaseFinalPublished && j.Phase != phaseCommitted && j.Phase != phaseRollback && j.Phase != phaseRolledBack {
		return fmt.Errorf("Workspace authority migration journal phase is invalid")
	}
	if j.Moved < 0 || j.Moved > len(j.Sources) || (j.Phase == phaseCommitted && j.Moved != len(j.Sources)) || (j.Phase == phaseRolledBack && j.Moved != 0) {
		return fmt.Errorf("Workspace authority migration journal progress is invalid")
	}
	if err := validateSourceDeclarations(j.Sources, finalRoot, transactionRoot); err != nil {
		return err
	}
	if err := j.Collection.Validate(); err != nil {
		return err
	}
	if err := j.ResearchDisposition.Validate(); err != nil {
		return err
	}
	if (j.ResearchDigest != "") != (j.ResearchDisposition == tobari.ResearchAuthReauthenticationRequired) {
		return fmt.Errorf("research backup identity and disposition disagree")
	}
	contexts := make(map[tobari.ContextID]tobari.ContextBinding, len(j.Collection.Contexts))
	for _, record := range j.Collection.Contexts {
		contexts[record.Context.ID] = record.Context
	}
	seen := make(map[tobari.ContextID]struct{}, len(j.ContextAssignments))
	for _, assignment := range j.ContextAssignments {
		if err := assignment.Validate(); err != nil {
			return err
		}
		binding, exists := contexts[assignment.ContextID]
		if !exists || binding.ProjectRoot != assignment.ProjectRoot || string(binding.TemplateID) != assignment.PredecessorManifestID {
			return fmt.Errorf("journaled Context assignment crosses final authority")
		}
		if _, duplicate := seen[assignment.ContextID]; duplicate {
			return fmt.Errorf("journaled Context assignment is duplicated")
		}
		seen[assignment.ContextID] = struct{}{}
	}
	if len(seen) != len(contexts) {
		return fmt.Errorf("journaled Context assignments are incomplete")
	}
	if err := validateFreshAuthPaths(j.FreshAuthPaths, j.StandardHomes, j.Sources, finalRoot, transactionRoot); err != nil {
		return err
	}
	return validateHomeDeclarations(j.StandardHomes, j.Sources)
}

func validatePrepared(prepared PreparedPredecessor, finalRoot, transactionRoot string) error {
	if prepared.Sources == nil || len(prepared.Sources) == 0 || prepared.StandardHomes == nil || prepared.FreshAuthPaths == nil {
		return fmt.Errorf("predecessor preflight is incomplete")
	}
	if info, err := os.Lstat(filepath.Dir(finalRoot)); err != nil || !safeDirectory(info) {
		return fmt.Errorf("final Workspace authority parent must be a real owner-only directory")
	}
	if err := validateSourceDeclarations(prepared.Sources, finalRoot, transactionRoot); err != nil {
		return err
	}
	return validateFreshAuthPaths(prepared.FreshAuthPaths, prepared.StandardHomes, prepared.Sources, finalRoot, transactionRoot)
}

func validateSourceDeclarations(sources []SourceItem, finalRoot, transactionRoot string) error {
	keys := make(map[string]struct{}, len(sources))
	paths := make([]string, 0, len(sources))
	cutoffs := 0
	for _, item := range sources {
		if item.Key == "" || item.Key != filepath.Base(item.Key) || strings.ContainsAny(item.Key, `/\\`) || !exactAbsoluteChild(item.Path) || !exactAbsoluteChild(item.BackupPath) || filepath.Dir(item.Path) != filepath.Dir(item.BackupPath) || item.Path == item.BackupPath || (item.Kind != SourceFile && item.Kind != SourceDirectory) || item.Digest.Validate() != nil || pathsOverlap(item.Path, finalRoot) || pathsOverlap(item.Path, transactionRoot) || pathsOverlap(item.BackupPath, finalRoot) || pathsOverlap(item.BackupPath, transactionRoot) || (item.LinuxRootKey && (!item.Research || item.Kind != SourceFile || filepath.Base(item.Path) != "root.key")) {
			return fmt.Errorf("predecessor source declaration is invalid")
		}
		if _, duplicate := keys[item.Key]; duplicate {
			return fmt.Errorf("predecessor source key is duplicated")
		}
		if parent, err := os.Lstat(filepath.Dir(item.Path)); err != nil || !safeDirectory(parent) {
			return fmt.Errorf("predecessor source parent is unsafe")
		}
		for _, path := range paths {
			if pathsOverlap(path, item.Path) || pathsOverlap(path, item.BackupPath) {
				return fmt.Errorf("predecessor source paths overlap")
			}
		}
		keys[item.Key] = struct{}{}
		paths = append(paths, item.Path, item.BackupPath)
		if item.Cutoff {
			cutoffs++
		}
	}
	if cutoffs != 1 {
		return fmt.Errorf("predecessor source requires exactly one reader cutoff")
	}
	return nil
}

func validateResearchSources(sources []SourceItem, platform tobari.ResearchAuthorityPlatform) error {
	research, rootKeys := 0, 0
	for _, source := range sources {
		if source.Research {
			research++
		}
		if source.LinuxRootKey {
			rootKeys++
		}
	}
	if research == 0 {
		return fmt.Errorf("predecessor research authority has no exact filesystem set")
	}
	if platform == tobari.ResearchAuthorityLinux && rootKeys != 1 {
		return fmt.Errorf("Linux predecessor research authority requires one exact filesystem root key")
	}
	if platform == tobari.ResearchAuthorityMacOS && rootKeys != 0 {
		return fmt.Errorf("macOS predecessor research authority cannot move Keychain recovery material as a filesystem root key")
	}
	return nil
}

func validateAndDigestSources(sources []SourceItem) (tobari.SemanticDigest, tobari.SemanticDigest, error) {
	type receipt struct {
		Key, Kind, Digest string
		Cutoff, Research  bool
	}
	receipts := make([]receipt, 0, len(sources))
	research := make([]receipt, 0)
	for index := range sources {
		if _, err := os.Lstat(sources[index].BackupPath); err == nil || !errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("predecessor backup target %q is not absent", sources[index].Key)
		}
		digest, kind, err := digestOwnedPath(sources[index].Path)
		if err != nil || kind != sources[index].Kind || digest != sources[index].Digest {
			return "", "", fmt.Errorf("predecessor source %q changed or is unsafe", sources[index].Key)
		}
		value := receipt{Key: sources[index].Key, Kind: string(kind), Digest: string(digest), Cutoff: sources[index].Cutoff, Research: sources[index].Research}
		receipts = append(receipts, value)
		if sources[index].Research {
			research = append(research, value)
		}
	}
	sort.Slice(receipts, func(i, j int) bool { return receipts[i].Key < receipts[j].Key })
	sort.Slice(research, func(i, j int) bool { return research[i].Key < research[j].Key })
	allDigest, err := jsonDigest(receipts)
	if err != nil {
		return "", "", err
	}
	var researchDigest tobari.SemanticDigest
	if len(research) > 0 {
		researchDigest, err = jsonDigest(research)
	}
	return allDigest, researchDigest, err
}

func validateHomes(homes []PreservedHome, workspaces []tobari.PredecessorWorkspace, sources []SourceItem) error {
	byWorkspace := make(map[tobari.WorkspaceID]PreservedHome, len(homes))
	for _, home := range homes {
		if home.WorkspaceID.Validate() != nil || !exactAbsoluteChild(home.Path) || home.Digest.Validate() != nil {
			return fmt.Errorf("standard Workspace home evidence is invalid")
		}
		if _, duplicate := byWorkspace[home.WorkspaceID]; duplicate {
			return fmt.Errorf("standard Workspace home evidence is duplicated")
		}
		for _, source := range sources {
			if pathsOverlap(home.Path, source.Path) || pathsOverlap(home.Path, source.BackupPath) {
				return fmt.Errorf("standard Workspace home entered the migration mutation set")
			}
		}
		byWorkspace[home.WorkspaceID] = home
	}
	for _, workspace := range workspaces {
		home, exists := byWorkspace[tobari.WorkspaceID(workspace.ID)]
		if !exists || home.Path != workspace.Home || home.Digest != workspace.HomeDigest {
			return fmt.Errorf("migration does not preserve one exact standard home per Workspace")
		}
	}
	if len(byWorkspace) != len(workspaces) {
		return fmt.Errorf("standard Workspace home evidence has unknown owners")
	}
	return validateHomePaths(homes)
}

func validateHomeDeclarations(homes []PreservedHome, sources []SourceItem) error {
	seen := map[tobari.WorkspaceID]struct{}{}
	for _, home := range homes {
		if home.WorkspaceID.Validate() != nil || !exactAbsoluteChild(home.Path) || home.Digest.Validate() != nil {
			return fmt.Errorf("journaled standard Workspace home is invalid")
		}
		if _, duplicate := seen[home.WorkspaceID]; duplicate {
			return fmt.Errorf("journaled standard Workspace home is duplicated")
		}
		for _, source := range sources {
			if pathsOverlap(home.Path, source.Path) || pathsOverlap(home.Path, source.BackupPath) {
				return fmt.Errorf("journaled standard Workspace home overlaps mutation source")
			}
		}
		seen[home.WorkspaceID] = struct{}{}
	}
	return nil
}

func validateHomePaths(homes []PreservedHome) error {
	for _, home := range homes {
		info, err := os.Lstat(home.Path)
		if err != nil || !safeDirectory(info) {
			return fmt.Errorf("standard Workspace home is unavailable or unsafe")
		}
	}
	return nil
}

func validateFreshAuthPaths(paths []string, homes []PreservedHome, sources []SourceItem, finalRoot, transactionRoot string) error {
	seen := map[string]struct{}{}
	for _, path := range paths {
		if !exactAbsoluteChild(path) || pathsOverlap(path, finalRoot) || pathsOverlap(path, transactionRoot) {
			return fmt.Errorf("fresh authentication path is invalid")
		}
		if _, duplicate := seen[path]; duplicate {
			return fmt.Errorf("fresh authentication path is duplicated")
		}
		for _, home := range homes {
			if pathsOverlap(path, home.Path) {
				return fmt.Errorf("fresh authentication path overlaps standard Workspace home")
			}
		}
		for _, source := range sources {
			if pathsOverlap(path, source.Path) || pathsOverlap(path, source.BackupPath) {
				return fmt.Errorf("fresh authentication path overlaps predecessor authority")
			}
		}
		seen[path] = struct{}{}
	}
	return nil
}

type pathState int

const (
	pathUnknown pathState = iota
	pathAbsent
	pathExact
)

func observeExactPath(path string, item SourceItem) pathState {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return pathAbsent
	} else if err != nil {
		return pathUnknown
	}
	digest, kind, err := digestOwnedPath(path)
	if err != nil || digest != item.Digest || kind != item.Kind {
		return pathUnknown
	}
	return pathExact
}

func digestOwnedPath(path string) (tobari.SemanticDigest, SourceKind, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !ownedByCurrentUser(info) {
		return "", "", fmt.Errorf("path is symlinked or not current-owner")
	}
	hash := sha256.New()
	count, total := 0, int64(0)
	var walk func(string, string) error
	walk = func(current, relative string) error {
		entryInfo, err := os.Lstat(current)
		if err != nil || entryInfo.Mode()&os.ModeSymlink != 0 || !ownedByCurrentUser(entryInfo) {
			return fmt.Errorf("source entry is unsafe")
		}
		count++
		if count > maxSourceEntries {
			return fmt.Errorf("source entry count exceeds bound")
		}
		if entryInfo.IsDir() {
			if entryInfo.Mode().Perm() != 0o700 {
				return fmt.Errorf("source directory is not owner-only")
			}
			_, _ = io.WriteString(hash, "d\x00"+relative+"\x00")
			entries, err := os.ReadDir(current)
			if err != nil {
				return err
			}
			sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
			for _, entry := range entries {
				if err := walk(filepath.Join(current, entry.Name()), filepath.Join(relative, entry.Name())); err != nil {
					return err
				}
			}
			return nil
		}
		if !entryInfo.Mode().IsRegular() || entryInfo.Mode().Perm() != 0o600 || entryInfo.Size() < 0 {
			return fmt.Errorf("source file is not owner-only regular data")
		}
		total += entryInfo.Size()
		if total > maxSourceBytes {
			return fmt.Errorf("source bytes exceed bound")
		}
		file, err := os.Open(current) // #nosec G304 -- exact preflight-owned source path is revalidated around open.
		if err != nil {
			return err
		}
		opened, statErr := file.Stat()
		if statErr != nil || !os.SameFile(entryInfo, opened) || !ownedByCurrentUser(opened) || opened.Mode().Perm() != 0o600 {
			_ = file.Close()
			return fmt.Errorf("source file changed during safe open")
		}
		_, _ = io.WriteString(hash, "f\x00"+relative+"\x00")
		written, copyErr := io.Copy(hash, io.LimitReader(file, maxSourceBytes-total+entryInfo.Size()+1))
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || written != entryInfo.Size() {
			return fmt.Errorf("source file changed during bounded read")
		}
		return nil
	}
	if err := walk(path, "."); err != nil {
		return "", "", err
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, after) || after.Mode() != info.Mode() || after.Size() != info.Size() {
		return "", "", fmt.Errorf("source root changed during bounded observation")
	}
	digest := tobari.SemanticDigest("sha256:" + hex.EncodeToString(hash.Sum(nil)))
	if info.IsDir() {
		return digest, SourceDirectory, nil
	}
	return digest, SourceFile, nil
}

func jsonDigest(value any) (tobari.SemanticDigest, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return tobari.SemanticDigest(fmt.Sprintf("sha256:%x", digest[:])), nil
}

func exactAbsoluteChild(path string) bool {
	return path != "" && filepath.IsAbs(path) && filepath.Clean(path) == path && path != string(filepath.Separator)
}

func pathsOverlap(first, second string) bool {
	if first == second {
		return true
	}
	separator := string(filepath.Separator)
	return strings.HasPrefix(first, second+separator) || strings.HasPrefix(second, first+separator)
}

func safeDirectory(info os.FileInfo) bool {
	return info != nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir() && info.Mode().Perm() == 0o700 && ownedByCurrentUser(info)
}

func writePrivateFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
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
	return file.Close()
}

func readPrivateFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) || info.Size() <= 0 || info.Size() > maximum {
		return nil, fmt.Errorf("private migration file is unsafe or outside bounds")
	}
	file, err := os.Open(path) // #nosec G304 -- caller owns an exact migration-private path.
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !ownedByCurrentUser(opened) || opened.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("private migration file changed during safe open")
	}
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || len(data) == 0 || int64(len(data)) > maximum {
		return nil, fmt.Errorf("private migration file is unreadable or outside bounds")
	}
	return data, nil
}

func decodeStrictJSON(data []byte, target any) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON contains trailing data")
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var parse func() error
	parse = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		seen := map[string]struct{}{}
		for decoder.More() {
			if delimiter == '{' {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("JSON key is not a string")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = struct{}{}
			}
			if err := parse(); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || (delimiter == '{' && closing != json.Delim('}')) || (delimiter == '[' && closing != json.Delim(']')) {
			return fmt.Errorf("JSON collection is not closed")
		}
		return nil
	}
	if err := parse(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON contains trailing data")
	}
	return nil
}

func syncRenameParents(source, target string) error {
	if err := syncDirectory(filepath.Dir(source)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(filepath.Dir(target))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path) // #nosec G304 -- exact migration-owned parent.
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
