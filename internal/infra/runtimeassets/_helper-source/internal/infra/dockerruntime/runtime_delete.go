package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	runtimeDeleteJournalFile     = "delete.json"
	runtimeDeleteReceiptsDir     = "delete-receipts"
	runtimeDeleteQuarantineDir   = ".runtime-delete-quarantine"
	runtimeDeleteJournalSchema   = 1
	runtimeDeleteReceiptRevision = 1
	runtimeDeleteMutationBudget  = 2 * time.Minute
)

type runtimeDeletePhase string

const (
	runtimeDeleteMaterials    runtimeDeletePhase = "materials"
	runtimeDeleteQuarantining runtimeDeletePhase = "quarantining"
	runtimeDeleteQuarantined  runtimeDeletePhase = "quarantined"
	runtimeDeleteRemoving     runtimeDeletePhase = "removing"
	runtimeDeleteRemoved      runtimeDeletePhase = "removed"
)

type runtimeDeleteJournal struct {
	SchemaVersion int                        `json:"schema_version"`
	Target        tobari.RuntimeDeleteTarget `json:"target"`
	Phase         runtimeDeletePhase         `json:"phase"`
	Items         []runtimePruneJournalItem  `json:"items"`
}

func (j runtimeDeleteJournal) Validate() error {
	if j.SchemaVersion != runtimeDeleteJournalSchema || j.Target.Validate() != nil || j.Items == nil || len(j.Items) != len(j.Target.Materials) {
		return fmt.Errorf("Runtime delete journal authority is invalid")
	}
	allTerminal := true
	for index, item := range j.Items {
		if err := item.Candidate.Validate(); err != nil || !reflect.DeepEqual(item.Candidate, j.Target.Materials[index].Candidate) {
			return fmt.Errorf("Runtime delete journal material authority is invalid")
		}
		switch item.State {
		case runtimePrunePending, runtimePruneRemoving:
			allTerminal = false
			if item.Disposition != "" {
				return fmt.Errorf("active Runtime delete material has terminal evidence")
			}
		case runtimePruneTerminal:
			if err := runtimePruneItemResult(item).Validate(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("Runtime delete material phase is invalid")
		}
	}
	switch j.Phase {
	case runtimeDeleteMaterials:
	case runtimeDeleteQuarantining, runtimeDeleteQuarantined, runtimeDeleteRemoving, runtimeDeleteRemoved:
		if !allTerminal {
			return fmt.Errorf("Runtime delete directory phase has active material")
		}
	default:
		return fmt.Errorf("Runtime delete journal phase is invalid")
	}
	return nil
}

func (j runtimeDeleteJournal) activity() tobari.RuntimeLifecycleActivity {
	return tobari.RuntimeLifecycleActivity{Kind: tobari.RuntimeLifecycleActivityDelete, RuntimeID: j.Target.Runtime.ID, Revisions: []string{}}
}

func (r *Runtime) runtimeDeleteJournalPath() string {
	return filepath.Join(r.runtimeLifecycleDirectory(), runtimeDeleteJournalFile)
}

func (r *Runtime) runtimeDeleteReceiptsDirectory() string {
	return filepath.Join(r.runtimeLifecycleDirectory(), runtimeDeleteReceiptsDir)
}

func (r *Runtime) runtimeDeleteReceiptPath(runtimeID string) string {
	return filepath.Join(r.runtimeDeleteReceiptsDirectory(), runtimeID+".json")
}

func (r *Runtime) runtimeDeleteQuarantineDirectory() string {
	// XDG config and state roots may be different mounts. Keep the journal in
	// state, but bind the atomic Runtime-directory rename to this private sibling
	// of runtimes/ on the config filesystem.
	return filepath.Join(r.configDirectory, runtimeDeleteQuarantineDir)
}

func (r *Runtime) runtimeDeleteQuarantinePath(runtimeID string) string {
	return filepath.Join(r.runtimeDeleteQuarantineDirectory(), runtimeID)
}

func (r *Runtime) readRuntimeDeleteJournalObserved() (*runtimeDeleteJournal, error) {
	if err := r.validateRuntimeDeleteReceiptDirectory(); err != nil {
		return nil, err
	}
	path := r.runtimeDeleteJournalPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := r.validateRuntimeDeleteQuarantineInventory(nil); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("Runtime delete journal must be a regular owner-only file")
	}
	var journal runtimeDeleteJournal
	if err := readStrictJSON(path, &journal); err != nil {
		return nil, err
	}
	if err := journal.Validate(); err != nil {
		return nil, err
	}
	if err := r.validateRuntimeDeleteTargetPaths(journal.Target); err != nil {
		return nil, err
	}
	if err := r.validateRuntimeDeleteQuarantineInventory(&journal); err != nil {
		return nil, err
	}
	return &journal, nil
}

func (r *Runtime) validateRuntimeDeleteReceiptDirectory() error {
	directory := r.runtimeDeleteReceiptsDirectory()
	if err := requirePrivateDirectory(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (r *Runtime) validateRuntimeDeleteTargetPaths(target tobari.RuntimeDeleteTarget) error {
	if target.Runtime.SourcePath != r.runtimeSourceDirectory(target.Runtime.Name) {
		return fmt.Errorf("Runtime delete source path is not canonical")
	}
	for _, revision := range target.Runtime.Revisions {
		want := filepath.Join(r.runtimeRevisionsDirectory(target.Runtime.Name), strings.TrimPrefix(revision.Revision, "sha256:"), "source")
		if revision.SnapshotPath != want || revision.Image != managedLibraryRuntimeImage(target.Runtime.Name, target.Runtime.ID, revision.Revision) {
			return fmt.Errorf("Runtime delete revision authority is not canonical")
		}
	}
	return nil
}

func (r *Runtime) validateRuntimeDeleteQuarantineInventory(journal *runtimeDeleteJournal) error {
	directory := r.runtimeDeleteQuarantineDirectory()
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		if journal != nil && (journal.Phase == runtimeDeleteQuarantined || journal.Phase == runtimeDeleteRemoving) {
			return fmt.Errorf("Runtime delete quarantine authority is absent")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if err := requirePrivateDirectory(directory); err != nil {
		return err
	}
	if journal == nil {
		if len(entries) != 0 {
			return fmt.Errorf("Runtime delete quarantine lacks journal authority")
		}
		return nil
	}
	want := journal.Target.Runtime.ID
	if len(entries) > 1 || (len(entries) == 1 && (entries[0].Name() != want || !entries[0].IsDir() || entries[0].Type()&os.ModeSymlink != 0)) {
		return fmt.Errorf("Runtime delete quarantine inventory is invalid")
	}
	present := len(entries) == 1
	switch journal.Phase {
	case runtimeDeleteMaterials:
		if present {
			return fmt.Errorf("Runtime delete material phase has quarantine state")
		}
	case runtimeDeleteQuarantining:
		// Rename outcome can be either side of the durable phase write.
	case runtimeDeleteQuarantined:
		if !present {
			return fmt.Errorf("Runtime delete quarantine authority is absent")
		}
	case runtimeDeleteRemoving:
		// Removal outcome can be either side of the durable phase write.
	case runtimeDeleteRemoved:
		if present {
			return fmt.Errorf("removed Runtime retains quarantine state")
		}
	}
	return nil
}

func validateRuntimeDeleteJournalTransition(previous, next runtimeDeleteJournal) error {
	if err := previous.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(previous.Target, next.Target) || len(previous.Items) != len(next.Items) {
		return fmt.Errorf("Runtime delete journal target authority changed")
	}
	if previous.Phase == next.Phase {
		if previous.Phase != runtimeDeleteMaterials {
			return fmt.Errorf("Runtime delete material transition is outside its phase")
		}
		changed := 0
		for index := range previous.Items {
			before, after := previous.Items[index], next.Items[index]
			if !reflect.DeepEqual(before.Candidate, after.Candidate) {
				return fmt.Errorf("Runtime delete material authority changed")
			}
			if before == after {
				continue
			}
			changed++
			if !((before.State == runtimePrunePending && after.State == runtimePruneRemoving) ||
				(before.State == runtimePrunePending && after.State == runtimePruneTerminal) ||
				(before.State == runtimePruneRemoving && after.State == runtimePruneTerminal)) {
				return fmt.Errorf("Runtime delete material transition is invalid")
			}
		}
		if changed != 1 {
			return fmt.Errorf("Runtime delete journal transition must change one material")
		}
		return nil
	}
	if !reflect.DeepEqual(previous.Items, next.Items) {
		return fmt.Errorf("Runtime delete directory transition changed material evidence")
	}
	allowed := previous.Phase == runtimeDeleteMaterials && next.Phase == runtimeDeleteQuarantining ||
		previous.Phase == runtimeDeleteQuarantining && next.Phase == runtimeDeleteQuarantined ||
		previous.Phase == runtimeDeleteQuarantined && next.Phase == runtimeDeleteRemoving ||
		previous.Phase == runtimeDeleteRemoving && next.Phase == runtimeDeleteRemoved
	if !allowed {
		return fmt.Errorf("Runtime delete directory transition is invalid")
	}
	return nil
}

func (r *Runtime) writeRuntimeDeleteJournal(previous *runtimeDeleteJournal, next runtimeDeleteJournal) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if err := validateRuntimePruneStateSize(next); err != nil {
		return err
	}
	current, err := r.readRuntimeDeleteJournalObserved()
	if err != nil {
		return err
	}
	if previous == nil {
		if current != nil {
			return fmt.Errorf("another Runtime delete journal is active")
		}
	} else {
		if current == nil || !reflect.DeepEqual(*current, *previous) {
			return fmt.Errorf("Runtime delete journal current authority changed")
		}
		if err := validateRuntimeDeleteJournalTransition(*previous, next); err != nil {
			return err
		}
	}
	write := func() error { return writeAtomicJSON(r.runtimeDeleteJournalPath(), next) }
	if r.runtimeDeleteJournalWrite != nil {
		write = func() error { return r.runtimeDeleteJournalWrite(previous, next) }
	}
	if writeErr := write(); writeErr != nil {
		observed, observeErr := r.readRuntimeDeleteJournalObserved()
		if observeErr != nil || observed == nil || !reflect.DeepEqual(*observed, next) {
			return errors.Join(writeErr, observeErr)
		}
	}
	return nil
}

func (r *Runtime) readRuntimeDeleteReceiptObserved(runtimeID string) (*tobari.RuntimeDeleteResult, error) {
	if err := tobari.ValidateRuntimeID(runtimeID); err != nil {
		return nil, err
	}
	directory := r.runtimeDeleteReceiptsDirectory()
	if err := requirePrivateDirectory(directory); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	path := r.runtimeDeleteReceiptPath(runtimeID)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("Runtime delete receipt must be a regular owner-only file")
	}
	var receipt tobari.RuntimeDeleteResult
	if err := readStrictJSON(path, &receipt); err != nil {
		return nil, err
	}
	if err := receipt.Validate(); err != nil || receipt.State != tobari.RuntimeDeleted || receipt.RuntimeID != runtimeID || receipt.ReceiptRevision != runtimeDeleteReceiptRevision {
		return nil, fmt.Errorf("Runtime delete receipt authority is invalid")
	}
	return &receipt, nil
}

func (r *Runtime) writeRuntimeDeleteReceipt(result tobari.RuntimeDeleteResult) error {
	result.State = tobari.RuntimeDeleted
	result.ReceiptRevision = runtimeDeleteReceiptRevision
	if err := result.Validate(); err != nil {
		return err
	}
	if err := validateRuntimePruneStateSize(result); err != nil {
		return err
	}
	current, err := r.readRuntimeDeleteReceiptObserved(result.RuntimeID)
	if err != nil {
		return err
	}
	if current != nil {
		if !reflect.DeepEqual(*current, result) {
			return fmt.Errorf("Runtime delete receipt authority changed")
		}
		return nil
	}
	write := func() error { return writeAtomicJSON(r.runtimeDeleteReceiptPath(result.RuntimeID), result) }
	if r.runtimeDeleteReceiptWrite != nil {
		write = func() error { return r.runtimeDeleteReceiptWrite(result) }
	}
	if writeErr := write(); writeErr != nil {
		observed, observeErr := r.readRuntimeDeleteReceiptObserved(result.RuntimeID)
		if observeErr != nil || observed == nil || !reflect.DeepEqual(*observed, result) {
			return errors.Join(writeErr, observeErr)
		}
	}
	return nil
}

func (r *Runtime) removeRuntimeDeleteJournal(expected runtimeDeleteJournal) error {
	current, err := r.readRuntimeDeleteJournalObserved()
	if err != nil || current == nil || !reflect.DeepEqual(*current, expected) {
		return fmt.Errorf("Runtime delete completion authority changed: %w", err)
	}
	remove := os.Remove
	if r.runtimeDeleteJournalRemove != nil {
		remove = r.runtimeDeleteJournalRemove
	}
	if err := remove(r.runtimeDeleteJournalPath()); err != nil {
		if _, observeErr := os.Lstat(r.runtimeDeleteJournalPath()); !errors.Is(observeErr, os.ErrNotExist) {
			return errors.Join(err, observeErr)
		}
	}
	return syncDirectoryIfPresent(r.runtimeLifecycleDirectory())
}

func (r *Runtime) requireNoRuntimeDeleteRecoveryConflict() error {
	journal, err := r.readRuntimeDeleteJournalObserved()
	if err != nil {
		return err
	}
	if journal != nil {
		return fmt.Errorf("%w: Runtime build recovery is blocked by active whole-Runtime deletion", tobari.ErrRuntimeDeleteInterrupted)
	}
	return nil
}

// ReadRuntimeDeleteRecovery observes only the exact local delete journal under
// the non-creating lifecycle read lock. Docker/material revalidation remains
// owned by the later confirmed delete action.
func (r *Runtime) ReadRuntimeDeleteRecovery(ctx context.Context) (tobari.RuntimeSummary, bool, error) {
	var summary tobari.RuntimeSummary
	found := false
	err := r.withLifecycleObservation(ctx, func(lockContext context.Context) error {
		journal, err := r.readRuntimeDeleteJournalObserved()
		if err != nil || journal == nil {
			return err
		}
		summary = tobari.RuntimeSummaryFrom(journal.Target.Runtime)
		if err := summary.Validate(); err != nil || summary.Kind != tobari.RuntimeKindManaged {
			if err == nil {
				err = fmt.Errorf("Runtime delete recovery target is not managed")
			}
			return err
		}
		found = true
		return lockContext.Err()
	})
	if err != nil {
		return tobari.RuntimeSummary{}, false, err
	}
	return summary, found, nil
}

// DeleteManagedRuntimeByReference retires exactly one managed Runtime. The
// caller supplies only the opaque Runtime reference; mutable names and Docker
// selectors are rederived under the lifecycle and Runtime-store locks.
func (r *Runtime) DeleteManagedRuntimeByReference(ctx context.Context, runtimeRef string) (tobari.RuntimeDeleteResult, error) {
	if err := tobari.ValidateRuntimeRef(runtimeRef); err != nil {
		return tobari.RuntimeDeleteResult{}, err
	}
	if runtimeRef == tobari.StandardRuntimeID {
		return tobari.RuntimeDeleteResult{}, tobari.ErrRuntimeDeleteProtected
	}
	mutationContext, cancel := context.WithTimeout(ctx, runtimeDeleteMutationBudget)
	defer cancel()
	var result tobari.RuntimeDeleteResult
	err := r.WithLifecycleLock(mutationContext, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			var err error
			result, err = r.deleteManagedRuntimeReferenceLocked(lockContext, runtimeRef)
			return err
		})
	})
	return result, err
}

func (r *Runtime) deleteManagedRuntimeReferenceLocked(ctx context.Context, runtimeRef string) (tobari.RuntimeDeleteResult, error) {
	runtimeID := strings.TrimPrefix(runtimeRef, "runtime/")
	receipt, err := r.readRuntimeDeleteReceiptObserved(runtimeID)
	if err != nil {
		return tobari.RuntimeDeleteResult{}, fmt.Errorf("%w: %w", tobari.ErrRuntimeDeleteInterrupted, err)
	}
	if receipt != nil {
		journal, journalErr := r.readRuntimeDeleteJournalObserved()
		if journalErr != nil {
			return tobari.RuntimeDeleteResult{}, fmt.Errorf("%w: %w", tobari.ErrRuntimeDeleteInterrupted, journalErr)
		}
		if journal != nil {
			if journal.Target.Runtime.ID != runtimeID || journal.Phase != runtimeDeleteRemoved {
				return tobari.RuntimeDeleteResult{}, fmt.Errorf("%w: another Runtime delete requires recovery", tobari.ErrRuntimeDeleteInterrupted)
			}
			if err := r.removeRuntimeDeleteJournal(*journal); err != nil {
				return tobari.RuntimeDeleteResult{}, fmt.Errorf("%w: %w", tobari.ErrRuntimeDeleteInterrupted, err)
			}
		}
		result := *receipt
		result.State = tobari.RuntimeAlreadyDeleted
		return result, result.Validate()
	}

	journal, err := r.readRuntimeDeleteJournalObserved()
	if err != nil {
		return tobari.RuntimeDeleteResult{}, fmt.Errorf("%w: %w", tobari.ErrRuntimeRetirementObservationUnknown, err)
	}
	budget := runtimeLifecycleBudget{remaining: runtimeLifecycleCallBudget}
	if journal == nil {
		if prune, err := r.readRuntimePruneJournalObserved(); err != nil {
			return tobari.RuntimeDeleteResult{}, err
		} else if prune != nil {
			return tobari.RuntimeDeleteResult{}, tobari.ErrRuntimeLifecycleActive
		}
		snapshot, _, err := r.readRuntimeLifecycleSnapshotLockedWithBudget(ctx, &budget)
		if err != nil {
			return tobari.RuntimeDeleteResult{}, fmt.Errorf("%w: %w", tobari.ErrRuntimeRetirementObservationUnknown, err)
		}
		target, err := tobari.RuntimeDeleteTargetFrom(snapshot, runtimeRef)
		if err != nil {
			return tobari.RuntimeDeleteResult{}, err
		}
		created := runtimeDeleteJournal{SchemaVersion: runtimeDeleteJournalSchema, Target: target, Phase: runtimeDeleteMaterials, Items: make([]runtimePruneJournalItem, len(target.Materials))}
		for index, material := range target.Materials {
			created.Items[index] = runtimePruneJournalItem{Candidate: material.Candidate, State: runtimePrunePending}
		}
		if err := r.writeRuntimeDeleteJournal(nil, created); err != nil {
			return tobari.RuntimeDeleteResult{}, fmt.Errorf("%w: %w", tobari.ErrRuntimeDeleteInterrupted, err)
		}
		journal = &created
	} else if journal.Target.Runtime.ID != runtimeID || tobari.RuntimeRef(runtimeID) != runtimeRef {
		return tobari.RuntimeDeleteResult{}, fmt.Errorf("%w: another Runtime delete requires recovery", tobari.ErrRuntimeDeleteInterrupted)
	}

	result, err := r.applyRuntimeDeleteLocked(ctx, &budget, *journal)
	if err != nil {
		return tobari.RuntimeDeleteResult{}, fmt.Errorf("%w: %w", tobari.ErrRuntimeDeleteInterrupted, err)
	}
	return result, nil
}

func (r *Runtime) applyRuntimeDeleteLocked(ctx context.Context, budget *runtimeLifecycleBudget, initial runtimeDeleteJournal) (tobari.RuntimeDeleteResult, error) {
	journal := initial
	if journal.Phase == runtimeDeleteMaterials {
		for index := range journal.Items {
			if journal.Items[index].State == runtimePruneTerminal {
				continue
			}
			snapshot, _, err := r.readRuntimeLifecycleSnapshotLockedWithBudget(ctx, budget)
			if err != nil {
				return tobari.RuntimeDeleteResult{}, err
			}
			material, selector, err := r.validateRuntimeDeleteResumeSnapshot(snapshot, journal, index)
			if err != nil {
				return tobari.RuntimeDeleteResult{}, err
			}
			if journal.Items[index].State == runtimePrunePending && !material.TagPresent {
				next := cloneRuntimeDeleteJournal(journal)
				next.Items[index].State = runtimePruneTerminal
				next.Items[index].Disposition = tobari.RuntimePruneAlreadyAbsent
				if err := r.writeRuntimeDeleteJournal(&journal, next); err != nil {
					return tobari.RuntimeDeleteResult{}, err
				}
				journal = next
				continue
			}
			if journal.Items[index].State == runtimePrunePending {
				next := cloneRuntimeDeleteJournal(journal)
				next.Items[index].State = runtimePruneRemoving
				if err := r.writeRuntimeDeleteJournal(&journal, next); err != nil {
					return tobari.RuntimeDeleteResult{}, err
				}
				journal = next
			}
			effectSnapshot, _, err := r.readRuntimeLifecycleSnapshotLockedWithBudget(ctx, budget)
			if err != nil {
				return tobari.RuntimeDeleteResult{}, err
			}
			material, selector, err = r.validateRuntimeDeleteResumeSnapshot(effectSnapshot, journal, index)
			if err != nil {
				return tobari.RuntimeDeleteResult{}, err
			}
			if material.TagPresent {
				if r.runtimeDeleteBeforeImageRemove != nil {
					if err := r.runtimeDeleteBeforeImageRemove(journal.Items[index].Candidate); err != nil {
						return tobari.RuntimeDeleteResult{}, err
					}
				}
				_, _, _ = budget.run(ctx, r.runner, []string{"image", "rm", selector}, os.Environ(), maxRuntimeLifecycleInspect)
			}
			after, _, err := r.readRuntimeLifecycleSnapshotLockedWithBudget(ctx, budget)
			if err != nil {
				return tobari.RuntimeDeleteResult{}, err
			}
			observed, _, err := r.validateRuntimeDeleteResumeSnapshot(after, journal, index)
			if err != nil || observed.TagPresent {
				return tobari.RuntimeDeleteResult{}, fmt.Errorf("Runtime delete material outcome requires reconciliation: %w", err)
			}
			disposition := tobari.RuntimePruneRemoved
			if observed.SharedContent {
				disposition = tobari.RuntimePrunePreservedShared
			}
			next := cloneRuntimeDeleteJournal(journal)
			next.Items[index].State = runtimePruneTerminal
			next.Items[index].Disposition = disposition
			if err := r.writeRuntimeDeleteJournal(&journal, next); err != nil {
				return tobari.RuntimeDeleteResult{}, err
			}
			journal = next
		}
		// Revalidate the complete catalog, protections, storage, and exact
		// Runtime directory once more immediately before its first filesystem
		// retirement effect.
		snapshot, _, err := r.readRuntimeLifecycleSnapshotLockedWithBudget(ctx, budget)
		if err != nil {
			return tobari.RuntimeDeleteResult{}, err
		}
		if err := r.validateRuntimeDeleteResumeSnapshotAll(snapshot, journal); err != nil {
			return tobari.RuntimeDeleteResult{}, err
		}
		next := cloneRuntimeDeleteJournal(journal)
		next.Phase = runtimeDeleteQuarantining
		if err := r.writeRuntimeDeleteJournal(&journal, next); err != nil {
			return tobari.RuntimeDeleteResult{}, err
		}
		journal = next
	}
	if journal.Phase == runtimeDeleteQuarantining {
		if err := r.quarantineRuntimeDeleteTarget(ctx, journal); err != nil {
			return tobari.RuntimeDeleteResult{}, err
		}
		next := cloneRuntimeDeleteJournal(journal)
		next.Phase = runtimeDeleteQuarantined
		if err := r.writeRuntimeDeleteJournal(&journal, next); err != nil {
			return tobari.RuntimeDeleteResult{}, err
		}
		journal = next
	}
	if journal.Phase == runtimeDeleteQuarantined {
		if err := r.revalidateProjectedRuntimeDelete(ctx, budget, journal); err != nil {
			return tobari.RuntimeDeleteResult{}, err
		}
		for _, item := range journal.Items {
			if err := r.completeRuntimePrunedFailedBuild(ctx, item.Candidate); err != nil {
				return tobari.RuntimeDeleteResult{}, err
			}
		}
		next := cloneRuntimeDeleteJournal(journal)
		next.Phase = runtimeDeleteRemoving
		if err := r.writeRuntimeDeleteJournal(&journal, next); err != nil {
			return tobari.RuntimeDeleteResult{}, err
		}
		journal = next
	}
	if journal.Phase == runtimeDeleteRemoving {
		if err := r.revalidateProjectedRuntimeDelete(ctx, budget, journal); err != nil {
			return tobari.RuntimeDeleteResult{}, err
		}
		if err := r.removeRuntimeDeleteQuarantine(ctx, journal); err != nil {
			return tobari.RuntimeDeleteResult{}, err
		}
		next := cloneRuntimeDeleteJournal(journal)
		next.Phase = runtimeDeleteRemoved
		if err := r.writeRuntimeDeleteJournal(&journal, next); err != nil {
			return tobari.RuntimeDeleteResult{}, err
		}
		journal = next
	}
	result := runtimeDeleteResultFromJournal(journal)
	if err := r.writeRuntimeDeleteReceipt(result); err != nil {
		return tobari.RuntimeDeleteResult{}, err
	}
	if err := r.removeRuntimeDeleteJournal(journal); err != nil {
		return tobari.RuntimeDeleteResult{}, err
	}
	return result, result.Validate()
}

func (r *Runtime) revalidateProjectedRuntimeDelete(ctx context.Context, budget *runtimeLifecycleBudget, journal runtimeDeleteJournal) error {
	snapshot, _, err := r.readRuntimeLifecycleSnapshotLockedWithBudget(ctx, budget)
	if err != nil {
		return err
	}
	return r.validateRuntimeDeleteResumeSnapshotAll(snapshot, journal)
}

func (r *Runtime) validateRuntimeDeleteResumeSnapshot(snapshot tobari.RuntimeLifecycleSnapshot, journal runtimeDeleteJournal, itemIndex int) (tobari.RuntimeMaterialObservation, string, error) {
	if err := r.validateRuntimeDeleteResumeSnapshotAll(snapshot, journal); err != nil {
		return tobari.RuntimeMaterialObservation{}, "", err
	}
	item := journal.Items[itemIndex]
	material, selector, _, err := r.validateRuntimePruneCandidate(snapshot, item.Candidate, item.State)
	return material, selector, err
}

func (r *Runtime) validateRuntimeDeleteResumeSnapshotAll(snapshot tobari.RuntimeLifecycleSnapshot, journal runtimeDeleteJournal) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if len(snapshot.Journals.Active) != 1 || !reflect.DeepEqual(snapshot.Journals.Active[0], journal.activity()) {
		return fmt.Errorf("Runtime delete lifecycle authority changed")
	}
	for _, protection := range snapshot.Protection.Items {
		if protection.RuntimeID == journal.Target.Runtime.ID {
			return tobari.ErrRuntimeDeleteProtected
		}
	}
	var manifest *tobari.RuntimeManifest
	for index := range snapshot.Runtimes {
		if snapshot.Runtimes[index].ID == journal.Target.Runtime.ID {
			copy := snapshot.Runtimes[index]
			copy.RuntimeRef = tobari.RuntimeRef(copy.ID)
			copy.Revisions = append([]tobari.RuntimeRevision{}, copy.Revisions...)
			for revision := range copy.Revisions {
				copy.Revisions[revision].RuntimeRef = tobari.RuntimeRef(copy.ID)
				copy.Revisions[revision].RevisionRef = tobari.RuntimeRevisionRef(copy.ID, copy.Revisions[revision].Revision)
			}
			manifest = &copy
			break
		}
	}
	if manifest == nil || !reflect.DeepEqual(*manifest, journal.Target.Runtime) {
		return fmt.Errorf("Runtime delete manifest authority changed")
	}
	for _, storage := range snapshot.Storage {
		if storage.RuntimeID == journal.Target.Runtime.ID && reflect.DeepEqual(storage, journal.Target.Storage) {
			return nil
		}
	}
	return fmt.Errorf("Runtime delete storage authority changed")
}

func (r *Runtime) quarantineRuntimeDeleteTarget(ctx context.Context, journal runtimeDeleteJournal) error {
	source := r.runtimeDirectory(journal.Target.Runtime.Name)
	destination := r.runtimeDeleteQuarantinePath(journal.Target.Runtime.ID)
	sourceInfo, sourceErr := os.Lstat(source)
	destinationInfo, destinationErr := os.Lstat(destination)
	if errors.Is(sourceErr, os.ErrNotExist) && destinationErr == nil {
		if !destinationInfo.IsDir() || destinationInfo.Mode()&os.ModeSymlink != 0 || destinationInfo.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("Runtime delete quarantine is unsafe")
		}
		return r.validateRuntimeDeleteQuarantinedTarget(ctx, journal)
	}
	if sourceErr != nil {
		return sourceErr
	}
	if !sourceInfo.IsDir() || sourceInfo.Mode()&os.ModeSymlink != 0 || sourceInfo.Mode().Perm()&0o077 != 0 || !errors.Is(destinationErr, os.ErrNotExist) {
		return fmt.Errorf("Runtime delete quarantine precondition changed")
	}
	if err := r.ensurePrivateDirectory(r.runtimeDeleteQuarantineDirectory()); err != nil {
		return err
	}
	if r.runtimeDeleteBeforeQuarantine != nil {
		if err := r.runtimeDeleteBeforeQuarantine(source, destination); err != nil {
			return err
		}
	}
	rename := os.Rename
	if r.runtimeDeleteRename != nil {
		rename = r.runtimeDeleteRename
	}
	if err := rename(source, destination); err != nil {
		if sourceNow, sourceObserveErr := os.Lstat(source); !errors.Is(sourceObserveErr, os.ErrNotExist) || sourceNow != nil {
			return errors.Join(err, sourceObserveErr)
		}
		if destinationNow, destinationObserveErr := os.Lstat(destination); destinationObserveErr != nil || !destinationNow.IsDir() || destinationNow.Mode()&os.ModeSymlink != 0 {
			return errors.Join(err, destinationObserveErr)
		}
	}
	if err := syncDirectoryIfPresent(r.runtimesDirectory()); err != nil {
		return err
	}
	if err := syncDirectoryIfPresent(r.runtimeDeleteQuarantineDirectory()); err != nil {
		return err
	}
	return r.validateRuntimeDeleteQuarantinedTarget(ctx, journal)
}

func (r *Runtime) validateRuntimeDeleteQuarantinedTarget(ctx context.Context, journal runtimeDeleteJournal) error {
	root := r.runtimeDeleteQuarantinePath(journal.Target.Runtime.ID)
	if err := requirePrivateDirectory(root); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	want := map[string]bool{"runtime.json": true, "source": true, "revisions": true}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !want[entry.Name()] {
			return fmt.Errorf("Runtime delete quarantine contains an unknown child")
		}
		delete(want, entry.Name())
	}
	if len(want) != 0 {
		return fmt.Errorf("Runtime delete quarantine inventory is incomplete")
	}
	manifestPath := filepath.Join(root, "runtime.json")
	info, err := os.Lstat(manifestPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("Runtime delete quarantine manifest is unsafe: %w", err)
	}
	var manifest tobari.RuntimeManifest
	if err := readStrictJSON(manifestPath, &manifest); err != nil {
		return fmt.Errorf("Runtime delete quarantine manifest authority changed: %w", err)
	}
	manifest.RuntimeRef = tobari.RuntimeRef(manifest.ID)
	manifest.Revisions = append([]tobari.RuntimeRevision{}, manifest.Revisions...)
	for index := range manifest.Revisions {
		manifest.Revisions[index].RuntimeRef = tobari.RuntimeRef(manifest.ID)
		manifest.Revisions[index].RevisionRef = tobari.RuntimeRevisionRef(manifest.ID, manifest.Revisions[index].Revision)
	}
	if !reflect.DeepEqual(manifest, journal.Target.Runtime) {
		return fmt.Errorf("Runtime delete quarantine manifest authority changed")
	}
	sourceBytes, err := observeRuntimeTreeLogicalBytes(ctx, filepath.Join(root, "source"))
	if err != nil || sourceBytes != journal.Target.Storage.SourceLogicalBytes {
		return fmt.Errorf("Runtime delete quarantine source authority changed: %w", err)
	}
	revisionsRoot := filepath.Join(root, "revisions")
	if err := requirePrivateDirectory(revisionsRoot); err != nil {
		return err
	}
	revisionEntries, err := os.ReadDir(revisionsRoot)
	if err != nil {
		return err
	}
	wantRevisions := make(map[string]tobari.RuntimeSnapshotStorage, len(journal.Target.Runtime.Revisions))
	storage := make(map[string]tobari.RuntimeSnapshotStorage, len(journal.Target.Storage.Snapshots))
	for _, item := range journal.Target.Storage.Snapshots {
		storage[string(item.Kind)+"\x00"+item.Revision] = item
	}
	for _, revision := range journal.Target.Runtime.Revisions {
		item, exists := storage[string(tobari.RuntimePruneCandidateRevision)+"\x00"+revision.Revision]
		if !exists {
			return fmt.Errorf("Runtime delete quarantine revision lacks storage authority")
		}
		wantRevisions[strings.TrimPrefix(revision.Revision, "sha256:")] = item
	}
	if len(revisionEntries) != len(wantRevisions) {
		return fmt.Errorf("Runtime delete quarantine revision inventory changed")
	}
	for _, entry := range revisionEntries {
		item, exists := wantRevisions[entry.Name()]
		if !exists || !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Runtime delete quarantine revision authority changed")
		}
		snapshot := filepath.Join(revisionsRoot, entry.Name(), "source")
		fingerprint, logicalBytes, err := observeImmutableRuntimeSnapshot(ctx, snapshot, item.Revision)
		if err != nil || fingerprint != item.SemanticFingerprint || logicalBytes != item.LogicalBytes {
			return fmt.Errorf("Runtime delete quarantine revision content changed: %w", err)
		}
	}
	return nil
}

func (r *Runtime) removeRuntimeDeleteQuarantine(ctx context.Context, journal runtimeDeleteJournal) (resultErr error) {
	path := r.runtimeDeleteQuarantinePath(journal.Target.Runtime.ID)
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := r.validateRuntimeDeleteQuarantinedTarget(ctx, journal); err != nil {
		return err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer func() {
		if root != nil {
			if closeErr := root.Close(); resultErr == nil && closeErr != nil {
				resultErr = closeErr
			}
		}
	}()
	if err := filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || (!entry.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("Runtime delete quarantine contains unsafe evidence: %w", err)
		}
		if entry.IsDir() {
			relative, err := filepath.Rel(path, current)
			if err != nil {
				return err
			}
			return root.Chmod(relative, 0o700) // #nosec G302 -- exact owner-only delete quarantine traversal.
		}
		return nil
	}); err != nil {
		return err
	}
	if err := root.Close(); err != nil {
		return err
	}
	root = nil
	remove := os.RemoveAll
	if r.runtimeDeleteQuarantineRemove != nil {
		remove = r.runtimeDeleteQuarantineRemove
	}
	if err := remove(path); err != nil {
		if _, observeErr := os.Lstat(path); !errors.Is(observeErr, os.ErrNotExist) {
			return errors.Join(err, observeErr)
		}
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return fmt.Errorf("Runtime delete quarantine was not removed")
		}
		return err
	}
	return syncDirectoryIfPresent(r.runtimeDeleteQuarantineDirectory())
}

func cloneRuntimeDeleteJournal(journal runtimeDeleteJournal) runtimeDeleteJournal {
	journal.Items = append([]runtimePruneJournalItem{}, journal.Items...)
	return journal
}

func runtimeDeleteResultFromJournal(journal runtimeDeleteJournal) tobari.RuntimeDeleteResult {
	items := make([]tobari.RuntimePruneItemResult, len(journal.Items))
	removedTags := 0
	snapshotBytes := int64(0)
	for index, item := range journal.Items {
		items[index] = runtimePruneItemResult(item)
		removedTags += items[index].RemovedTagCount
		snapshotBytes += items[index].SnapshotLogicalBytes
	}
	return tobari.RuntimeDeleteResult{
		Task: tobari.TaskRuntimeDelete, RuntimeID: journal.Target.Runtime.ID, RuntimeRef: tobari.RuntimeRef(journal.Target.Runtime.ID), Name: journal.Target.Runtime.Name,
		State: tobari.RuntimeDeleted, SourceLogicalBytes: journal.Target.Storage.SourceLogicalBytes, SnapshotLogicalBytes: snapshotBytes,
		SourceDisposition: tobari.RuntimeDeleteAuthorityRemoved, SnapshotsDisposition: tobari.RuntimeDeleteAuthorityRemoved, HistoryDisposition: tobari.RuntimeDeleteAuthorityRemoved,
		Items: items, RemovedTagCount: removedTags, ReceiptRevision: runtimeDeleteReceiptRevision,
		WorkspaceManifestsPreserved: true, WorkspacesPreserved: true, WorkspaceIDsPreserved: true, WorkspaceHomesPreserved: true,
		AppliedReceiptsPreserved: true, ProjectRootsPreserved: true, CredentialsPreserved: true, SharedResourcesPreserved: true,
	}
}
