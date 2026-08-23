package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	runtimePruneJournalFile    = "prune.json"
	runtimePruneReceiptsFile   = "prune-receipts.json"
	runtimePruneJournalSchema  = 1
	runtimePruneReceiptSchema  = 1
	runtimePruneReceiptLimit   = 16
	runtimePruneMutationBudget = 2 * time.Minute
)

type runtimePruneItemState string

const (
	runtimePrunePending  runtimePruneItemState = "pending"
	runtimePruneRemoving runtimePruneItemState = "removing"
	runtimePruneTerminal runtimePruneItemState = "terminal"
)

type runtimePruneJournalItem struct {
	Candidate   tobari.RuntimePruneCandidate   `json:"candidate"`
	State       runtimePruneItemState          `json:"state"`
	Disposition tobari.RuntimePruneDisposition `json:"disposition,omitempty"`
}

type runtimePruneJournal struct {
	SchemaVersion int                       `json:"schema_version"`
	Plan          tobari.RuntimePrunePlan   `json:"plan"`
	Items         []runtimePruneJournalItem `json:"items"`
}

func (j runtimePruneJournal) Validate() error {
	if j.SchemaVersion != runtimePruneJournalSchema || j.Plan.Validate() != nil || !j.Plan.Applicable || len(j.Items) == 0 || len(j.Items) != len(j.Plan.Candidates) {
		return fmt.Errorf("Runtime prune journal authority is invalid")
	}
	previous := ""
	semantic := make(map[string]bool, len(j.Items))
	for index, item := range j.Items {
		if err := item.Candidate.Validate(); err != nil {
			return err
		}
		if !reflect.DeepEqual(item.Candidate, j.Plan.Candidates[index]) {
			return fmt.Errorf("Runtime prune journal item does not match reviewed plan authority")
		}
		key := runtimePruneJournalItemKey(item)
		if previous >= key {
			return fmt.Errorf("Runtime prune journal items are not unique canonical order")
		}
		previous = key
		semanticKey := item.Candidate.RuntimeID + "\x00" + item.Candidate.Revision
		if semantic[semanticKey] {
			return fmt.Errorf("Runtime prune journal duplicates semantic material authority")
		}
		semantic[semanticKey] = true
		switch item.State {
		case runtimePrunePending, runtimePruneRemoving:
			if item.Disposition != "" {
				return fmt.Errorf("active Runtime prune item has terminal evidence")
			}
		case runtimePruneTerminal:
			result := runtimePruneItemResult(item)
			if err := result.Validate(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("Runtime prune journal item state is invalid")
		}
	}
	return nil
}

func (j runtimePruneJournal) activities() []tobari.RuntimeLifecycleActivity {
	byRuntime := make(map[string]map[string]bool)
	for _, item := range j.Items {
		if byRuntime[item.Candidate.RuntimeID] == nil {
			byRuntime[item.Candidate.RuntimeID] = make(map[string]bool)
		}
		byRuntime[item.Candidate.RuntimeID][item.Candidate.Revision] = true
	}
	activities := make([]tobari.RuntimeLifecycleActivity, 0, len(byRuntime))
	for runtimeID, revisionsSet := range byRuntime {
		revisions := make([]string, 0, len(revisionsSet))
		for revision := range revisionsSet {
			revisions = append(revisions, revision)
		}
		sort.Strings(revisions)
		activities = append(activities, tobari.RuntimeLifecycleActivity{Kind: tobari.RuntimeLifecycleActivityPrune, RuntimeID: runtimeID, Revisions: revisions})
	}
	sort.Slice(activities, func(i, k int) bool { return activities[i].RuntimeID < activities[k].RuntimeID })
	return activities
}

type runtimePruneReceiptStore struct {
	SchemaVersion int                         `json:"schema_version"`
	NextRevision  uint64                      `json:"next_revision"`
	Results       []tobari.RuntimePruneResult `json:"results"`
}

func emptyRuntimePruneReceiptStore() runtimePruneReceiptStore {
	return runtimePruneReceiptStore{SchemaVersion: runtimePruneReceiptSchema, NextRevision: 1, Results: []tobari.RuntimePruneResult{}}
}

func (s runtimePruneReceiptStore) Validate() error {
	if s.SchemaVersion != runtimePruneReceiptSchema || s.NextRevision == 0 || s.Results == nil || len(s.Results) > runtimePruneReceiptLimit {
		return fmt.Errorf("Runtime prune receipt store is invalid")
	}
	var previous uint64
	seen := make(map[string]bool)
	for _, result := range s.Results {
		if err := result.Validate(); err != nil || result.State != tobari.RuntimePruneApplied || result.ReceiptRevision <= previous || result.ReceiptRevision >= s.NextRevision || seen[result.PlanRef] {
			return fmt.Errorf("Runtime prune receipt authority is invalid")
		}
		previous = result.ReceiptRevision
		seen[result.PlanRef] = true
	}
	return nil
}

func (r *Runtime) runtimePruneJournalPath() string {
	return r.runtimeLifecycleDirectory() + string(os.PathSeparator) + runtimePruneJournalFile
}

func (r *Runtime) runtimePruneReceiptsPath() string {
	return r.runtimeLifecycleDirectory() + string(os.PathSeparator) + runtimePruneReceiptsFile
}

func (r *Runtime) readRuntimePruneJournalObserved() (*runtimePruneJournal, error) {
	path := r.runtimePruneJournalPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("Runtime prune journal must be a regular owner-only file")
	}
	var journal runtimePruneJournal
	if err := readStrictJSON(path, &journal); err != nil {
		return nil, err
	}
	if err := journal.Validate(); err != nil {
		return nil, err
	}
	return &journal, nil
}

func (r *Runtime) readRuntimePruneReceiptStoreObserved() (runtimePruneReceiptStore, error) {
	path := r.runtimePruneReceiptsPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyRuntimePruneReceiptStore(), nil
	}
	if err != nil {
		return runtimePruneReceiptStore{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return runtimePruneReceiptStore{}, fmt.Errorf("Runtime prune receipts must be a regular owner-only file")
	}
	var store runtimePruneReceiptStore
	if err := readStrictJSON(path, &store); err != nil {
		return runtimePruneReceiptStore{}, err
	}
	if err := store.Validate(); err != nil {
		return runtimePruneReceiptStore{}, err
	}
	return store, nil
}

func validateRuntimePruneJournalTransition(previous, next runtimePruneJournal) error {
	if err := previous.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(previous.Plan, next.Plan) || len(previous.Items) != len(next.Items) {
		return fmt.Errorf("Runtime prune journal authority changed")
	}
	changed := 0
	for index := range previous.Items {
		before, after := previous.Items[index], next.Items[index]
		if !reflect.DeepEqual(before.Candidate, after.Candidate) {
			return fmt.Errorf("Runtime prune candidate authority changed")
		}
		if before == after {
			continue
		}
		changed++
		allowed := before.State == runtimePrunePending && after.State == runtimePruneRemoving || before.State == runtimePrunePending && after.State == runtimePruneTerminal || before.State == runtimePruneRemoving && after.State == runtimePruneTerminal
		if !allowed {
			return fmt.Errorf("Runtime prune journal phase transition is invalid")
		}
	}
	if changed != 1 {
		return fmt.Errorf("Runtime prune journal transition must change exactly one item")
	}
	return nil
}

func (r *Runtime) writeRuntimePruneJournal(previous *runtimePruneJournal, next runtimePruneJournal) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if err := validateRuntimePruneStateSize(next); err != nil {
		return err
	}
	current, err := r.readRuntimePruneJournalObserved()
	if err != nil {
		return err
	}
	if previous == nil {
		if current != nil {
			return fmt.Errorf("another Runtime prune journal is active")
		}
	} else {
		if current == nil || !reflect.DeepEqual(*current, *previous) {
			return fmt.Errorf("Runtime prune journal current authority changed")
		}
		if err := validateRuntimePruneJournalTransition(*previous, next); err != nil {
			return err
		}
	}
	write := func() error { return writeAtomicJSON(r.runtimePruneJournalPath(), next) }
	if r.runtimePruneJournalWrite != nil {
		write = func() error { return r.runtimePruneJournalWrite(previous, next) }
	}
	if writeErr := write(); writeErr != nil {
		observed, observeErr := r.readRuntimePruneJournalObserved()
		if observeErr != nil || observed == nil || !reflect.DeepEqual(*observed, next) {
			return errors.Join(writeErr, observeErr)
		}
	}
	return nil
}

func (r *Runtime) writeRuntimePruneReceipt(store runtimePruneReceiptStore, result tobari.RuntimePruneResult) (runtimePruneReceiptStore, error) {
	current, err := r.readRuntimePruneReceiptStoreObserved()
	if err != nil || !reflect.DeepEqual(current, store) {
		return runtimePruneReceiptStore{}, fmt.Errorf("Runtime prune receipt authority changed: %w", err)
	}
	result.State = tobari.RuntimePruneApplied
	result.ReceiptRevision = store.NextRevision
	if err := result.Validate(); err != nil {
		return runtimePruneReceiptStore{}, err
	}
	next := store
	next.NextRevision++
	next.Results = append(append([]tobari.RuntimePruneResult{}, store.Results...), result)
	for len(next.Results) > runtimePruneReceiptLimit {
		next.Results = append([]tobari.RuntimePruneResult{}, next.Results[1:]...)
	}
	for validateRuntimePruneStateSize(next) != nil && len(next.Results) > 1 {
		next.Results = append([]tobari.RuntimePruneResult{}, next.Results[1:]...)
	}
	if err := next.Validate(); err != nil {
		return runtimePruneReceiptStore{}, err
	}
	if err := validateRuntimePruneStateSize(next); err != nil {
		return runtimePruneReceiptStore{}, err
	}
	write := func() error { return writeAtomicJSON(r.runtimePruneReceiptsPath(), next) }
	if r.runtimePruneReceiptWrite != nil {
		write = func() error { return r.runtimePruneReceiptWrite(next) }
	}
	if writeErr := write(); writeErr != nil {
		observed, observeErr := r.readRuntimePruneReceiptStoreObserved()
		if observeErr != nil || !reflect.DeepEqual(observed, next) {
			return runtimePruneReceiptStore{}, errors.Join(writeErr, observeErr)
		}
	}
	return next, nil
}

func validateRuntimePruneStateSize(value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(encoded)+1 > maxProjectStateBytes {
		return fmt.Errorf("Runtime prune durable state exceeds the bounded state size")
	}
	return nil
}

func runtimePruneReceipt(store runtimePruneReceiptStore, planRef string) (tobari.RuntimePruneResult, bool) {
	for _, result := range store.Results {
		if result.PlanRef == planRef {
			result.State = tobari.RuntimePruneAlreadyApplied
			return result, true
		}
	}
	return tobari.RuntimePruneResult{}, false
}

func (r *Runtime) removeRuntimePruneJournal(expected runtimePruneJournal) error {
	current, err := r.readRuntimePruneJournalObserved()
	if err != nil || current == nil || !reflect.DeepEqual(*current, expected) {
		return fmt.Errorf("Runtime prune completion authority changed: %w", err)
	}
	remove := os.Remove
	if r.runtimePruneJournalRemove != nil {
		remove = r.runtimePruneJournalRemove
	}
	if err := remove(r.runtimePruneJournalPath()); err != nil {
		if _, observeErr := os.Lstat(r.runtimePruneJournalPath()); !errors.Is(observeErr, os.ErrNotExist) {
			return errors.Join(err, observeErr)
		}
	}
	return syncDirectoryIfPresent(r.runtimeLifecycleDirectory())
}

// ApplyRuntimePrune executes one previously reviewed exact plan. It is an
// internal effect boundary until the application and Catalog graph closure are
// added. Every effect is preceded by a coherent re-observation while the
// installation and Runtime-store locks remain held.
func (r *Runtime) ApplyRuntimePrune(ctx context.Context, plan tobari.RuntimePrunePlan) (tobari.RuntimePruneResult, error) {
	if err := plan.Validate(); err != nil {
		return tobari.RuntimePruneResult{}, err
	}
	mutationContext, cancel := context.WithTimeout(ctx, runtimePruneMutationBudget)
	defer cancel()
	var result tobari.RuntimePruneResult
	err := r.WithLifecycleLock(mutationContext, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			var err error
			result, err = r.applyRuntimePruneLocked(lockContext, plan)
			return err
		})
	})
	return result, err
}

func (r *Runtime) applyRuntimePruneLocked(ctx context.Context, plan tobari.RuntimePrunePlan) (tobari.RuntimePruneResult, error) {
	budget := runtimeLifecycleBudget{remaining: runtimeLifecycleCallBudget}
	store, err := r.readRuntimePruneReceiptStoreObserved()
	if err != nil {
		return tobari.RuntimePruneResult{}, err
	}
	if receipt, ok := runtimePruneReceipt(store, plan.PlanRef); ok {
		if journal, journalErr := r.readRuntimePruneJournalObserved(); journalErr != nil {
			return tobari.RuntimePruneResult{}, journalErr
		} else if journal != nil {
			if journal.Plan.PlanRef != plan.PlanRef {
				return tobari.RuntimePruneResult{}, fmt.Errorf("another Runtime prune journal requires recovery")
			}
			if err := r.removeRuntimePruneJournal(*journal); err != nil {
				return tobari.RuntimePruneResult{}, err
			}
		}
		return receipt, receipt.Validate()
	}

	journal, err := r.readRuntimePruneJournalObserved()
	if err != nil {
		return tobari.RuntimePruneResult{}, err
	}
	if journal == nil {
		snapshot, observedAt, err := r.readRuntimeLifecycleSnapshotLockedWithBudget(ctx, &budget)
		if err != nil {
			return tobari.RuntimePruneResult{}, err
		}
		current, err := tobari.PlanRuntimePrune(snapshot, observedAt)
		if err != nil || current.PlanRef != plan.PlanRef || !current.Applicable {
			return tobari.RuntimePruneResult{}, fmt.Errorf("Runtime prune plan requires a fresh review: %w", err)
		}
		if current.Empty {
			result := tobari.RuntimePruneResult{Task: tobari.TaskRuntimePruneApply, PlanRef: plan.PlanRef, State: tobari.RuntimePruneEmpty, Items: []tobari.RuntimePruneItemResult{}, SourcePreserved: true, SnapshotsPreserved: true, HistoryPreserved: true}
			return result, result.Validate()
		}
		created := runtimePruneJournal{SchemaVersion: runtimePruneJournalSchema, Plan: current, Items: make([]runtimePruneJournalItem, len(current.Candidates))}
		for index, candidate := range current.Candidates {
			created.Items[index] = runtimePruneJournalItem{Candidate: candidate, State: runtimePrunePending}
		}
		if err := r.writeRuntimePruneJournal(nil, created); err != nil {
			return tobari.RuntimePruneResult{}, err
		}
		journal = &created
	} else if journal.Plan.PlanRef != plan.PlanRef {
		return tobari.RuntimePruneResult{}, fmt.Errorf("another Runtime prune journal requires recovery")
	}

	for index := range journal.Items {
		if journal.Items[index].State == runtimePruneTerminal {
			continue
		}
		snapshot, _, err := r.readRuntimeLifecycleSnapshotLockedWithBudget(ctx, &budget)
		if err != nil {
			return tobari.RuntimePruneResult{}, err
		}
		if err := validateRuntimePruneResumeSnapshot(snapshot, *journal); err != nil {
			return tobari.RuntimePruneResult{}, err
		}
		material, selector, build, err := r.validateRuntimePruneCandidate(snapshot, journal.Items[index].Candidate, journal.Items[index].State)
		if err != nil {
			return tobari.RuntimePruneResult{}, err
		}
		if journal.Items[index].State == runtimePrunePending && !material.TagPresent {
			next := cloneRuntimePruneJournal(*journal)
			next.Items[index].State = runtimePruneTerminal
			next.Items[index].Disposition = tobari.RuntimePruneAlreadyAbsent
			if build != nil {
				if err := r.completeRuntimeBuildJournal(ctx, *build); err != nil {
					return tobari.RuntimePruneResult{}, err
				}
			}
			if err := r.writeRuntimePruneJournal(journal, next); err != nil {
				return tobari.RuntimePruneResult{}, err
			}
			journal = &next
			continue
		}
		if journal.Items[index].State == runtimePrunePending {
			next := cloneRuntimePruneJournal(*journal)
			next.Items[index].State = runtimePruneRemoving
			if err := r.writeRuntimePruneJournal(journal, next); err != nil {
				return tobari.RuntimePruneResult{}, err
			}
			journal = &next
		}
		effectSnapshot, _, err := r.readRuntimeLifecycleSnapshotLockedWithBudget(ctx, &budget)
		if err != nil {
			return tobari.RuntimePruneResult{}, err
		}
		if err := validateRuntimePruneResumeSnapshot(effectSnapshot, *journal); err != nil {
			return tobari.RuntimePruneResult{}, err
		}
		material, selector, build, err = r.validateRuntimePruneCandidate(effectSnapshot, journal.Items[index].Candidate, journal.Items[index].State)
		if err != nil {
			return tobari.RuntimePruneResult{}, err
		}
		if material.TagPresent {
			if r.runtimePruneBeforeRemove != nil {
				if err := r.runtimePruneBeforeRemove(journal.Items[index].Candidate); err != nil {
					return tobari.RuntimePruneResult{}, err
				}
			}
			_, stderr, removeErr := budget.run(ctx, r.runner, []string{"image", "rm", selector}, os.Environ(), maxRuntimeLifecycleInspect)
			if removeErr != nil && ctx.Err() != nil {
				return tobari.RuntimePruneResult{}, ctx.Err()
			}
			_ = stderr
		}
		after, _, err := r.readRuntimeLifecycleSnapshotLockedWithBudget(ctx, &budget)
		if err != nil {
			return tobari.RuntimePruneResult{}, err
		}
		if err := validateRuntimePruneResumeSnapshot(after, *journal); err != nil {
			return tobari.RuntimePruneResult{}, err
		}
		observed, _, currentBuild, err := r.validateRuntimePruneCandidate(after, journal.Items[index].Candidate, runtimePruneRemoving)
		if err != nil || observed.TagPresent {
			return tobari.RuntimePruneResult{}, fmt.Errorf("Runtime prune removal outcome requires reconciliation: %w", err)
		}
		if currentBuild != nil {
			if err := r.completeRuntimeBuildJournal(ctx, *currentBuild); err != nil {
				return tobari.RuntimePruneResult{}, err
			}
		} else if build != nil {
			return tobari.RuntimePruneResult{}, fmt.Errorf("failed Runtime build cleanup authority changed")
		}
		disposition := tobari.RuntimePruneRemoved
		if observed.SharedContent {
			disposition = tobari.RuntimePrunePreservedShared
		}
		next := cloneRuntimePruneJournal(*journal)
		next.Items[index].State = runtimePruneTerminal
		next.Items[index].Disposition = disposition
		if err := r.writeRuntimePruneJournal(journal, next); err != nil {
			return tobari.RuntimePruneResult{}, err
		}
		journal = &next
	}

	result := runtimePruneResultFromJournal(*journal)
	store, err = r.writeRuntimePruneReceipt(store, result)
	if err != nil {
		return tobari.RuntimePruneResult{}, err
	}
	receipt, ok := runtimePruneReceipt(store, plan.PlanRef)
	if !ok {
		return tobari.RuntimePruneResult{}, fmt.Errorf("Runtime prune receipt publication is incomplete")
	}
	receipt.State = tobari.RuntimePruneApplied
	if err := r.removeRuntimePruneJournal(*journal); err != nil {
		return tobari.RuntimePruneResult{}, err
	}
	return receipt, receipt.Validate()
}

func validateRuntimePruneResumeSnapshot(snapshot tobari.RuntimeLifecycleSnapshot, journal runtimePruneJournal) error {
	plan, err := tobari.PlanRuntimePrune(snapshot, time.Now().UTC())
	if err != nil {
		return err
	}
	items := make(map[string]runtimePruneJournalItem, len(journal.Items))
	for _, item := range journal.Items {
		items[item.Candidate.RuntimeID+"\x00"+item.Candidate.Revision] = item
	}
	for _, blocker := range plan.Blockers {
		item, belongs := items[blocker.RuntimeID+"\x00"+blocker.Revision]
		if !belongs {
			return fmt.Errorf("Runtime prune observation gained an unrelated blocker")
		}
		switch blocker.Reason {
		case tobari.RuntimeBlockedByActiveRetirement:
			continue
		case tobari.RuntimeBlockedByActiveBuild:
			if item.Candidate.Kind == tobari.RuntimePruneCandidateFailedBuild {
				continue
			}
		case tobari.RuntimeBlockedByImageMissing, tobari.RuntimeBlockedByImageTagMissing, tobari.RuntimeBlockedByImageTagShared, tobari.RuntimeBlockedByImagePruned,
			tobari.RuntimeBlockedByStagingMissing, tobari.RuntimeBlockedByStagingTagMissing, tobari.RuntimeBlockedByStagingTagShared:
			continue
		}
		return fmt.Errorf("Runtime prune observation is not safely applicable")
	}
	return nil
}

func (r *Runtime) validateRuntimePruneCandidate(snapshot tobari.RuntimeLifecycleSnapshot, candidate tobari.RuntimePruneCandidate, state runtimePruneItemState) (tobari.RuntimeMaterialObservation, string, *runtimeBuildJournal, error) {
	for _, protection := range snapshot.Protection.Items {
		if protection.RuntimeID == candidate.RuntimeID && protection.RuntimeRevision == candidate.Revision {
			return tobari.RuntimeMaterialObservation{}, "", nil, fmt.Errorf("Runtime revision became referenced after review")
		}
	}
	var material *tobari.RuntimeMaterialObservation
	var selector string
	var build *runtimeBuildJournal
	if candidate.Kind == tobari.RuntimePruneCandidateRevision {
		for _, manifest := range snapshot.Runtimes {
			if manifest.ID != candidate.RuntimeID || manifest.Kind != tobari.RuntimeKindManaged {
				continue
			}
			for _, revision := range manifest.Revisions {
				if revision.Revision == candidate.Revision {
					selector = managedLibraryRuntimeImage(manifest.Name, manifest.ID, revision.Revision)
				}
			}
		}
		for index := range snapshot.Materials {
			if snapshot.Materials[index].RuntimeID == candidate.RuntimeID && snapshot.Materials[index].Revision == candidate.Revision {
				material = &snapshot.Materials[index]
			}
		}
	} else {
		selector = managedRuntimeStagingImage(candidate.RuntimeID, candidate.Revision)
		for index := range snapshot.Journals.FailedBuilds {
			artifact := &snapshot.Journals.FailedBuilds[index]
			if artifact.RuntimeID == candidate.RuntimeID && artifact.Revision == candidate.Revision {
				material = &artifact.Material
			}
		}
		observed, err := r.readRuntimeBuildJournalObserved()
		if err != nil {
			return tobari.RuntimeMaterialObservation{}, "", nil, err
		}
		if observed != nil && observed.RuntimeID == candidate.RuntimeID && observed.Revision == candidate.Revision && (observed.Phase == runtimeBuildPhaseFailed || observed.Phase == runtimeBuildPhaseCompleting) {
			build = observed
		}
	}
	if material == nil {
		if candidate.Kind == tobari.RuntimePruneCandidateFailedBuild && state == runtimePruneRemoving && build != nil {
			return tobari.RuntimeMaterialObservation{RuntimeID: candidate.RuntimeID, Revision: candidate.Revision, TagRole: tobari.RuntimeMaterialTagJournaledStaging, Availability: tobari.RuntimeAvailabilityMissing, ObservationComplete: true}, selector, build, nil
		}
		return tobari.RuntimeMaterialObservation{}, "", build, fmt.Errorf("Runtime prune material authority changed")
	}
	if material.WorkspaceInUse || material.ExternalInUse || !material.ObservationComplete || material.MigrationUnverified || material.Availability == tobari.RuntimeAvailabilityMismatched || material.Availability == tobari.RuntimeAvailabilityUnknown {
		return tobari.RuntimeMaterialObservation{}, "", build, fmt.Errorf("Runtime prune material is no longer safe")
	}
	if material.TagPresent && (!material.OwnershipVerified || material.Availability != tobari.RuntimeAvailabilityAvailable) {
		return tobari.RuntimeMaterialObservation{}, "", build, fmt.Errorf("Runtime prune material ownership changed")
	}
	return *material, selector, build, nil
}

func cloneRuntimePruneJournal(journal runtimePruneJournal) runtimePruneJournal {
	journal.Items = append([]runtimePruneJournalItem{}, journal.Items...)
	return journal
}

func runtimePruneJournalItemKey(item runtimePruneJournalItem) string {
	return item.Candidate.RuntimeID + "\x00" + item.Candidate.Revision + "\x00" + string(item.Candidate.Kind)
}

func runtimePruneItemResult(item runtimePruneJournalItem) tobari.RuntimePruneItemResult {
	removed := 0
	if item.Disposition == tobari.RuntimePruneRemoved || item.Disposition == tobari.RuntimePrunePreservedShared {
		removed = 1
	}
	return tobari.RuntimePruneItemResult{Kind: item.Candidate.Kind, RuntimeID: item.Candidate.RuntimeID, Revision: item.Candidate.Revision, RuntimeRef: item.Candidate.RuntimeRef, RevisionRef: item.Candidate.RevisionRef, Name: item.Candidate.Name, Ordinal: item.Candidate.Ordinal, LastUsed: item.Candidate.LastUsed, SourceLogicalBytes: item.Candidate.SourceLogicalBytes, SnapshotLogicalBytes: item.Candidate.SnapshotLogicalBytes, Disposition: item.Disposition, RemovedTagCount: removed, ImageVirtualBytes: item.Candidate.ImageVirtualBytes}
}

func runtimePruneResultFromJournal(journal runtimePruneJournal) tobari.RuntimePruneResult {
	items := make([]tobari.RuntimePruneItemResult, len(journal.Items))
	removed := 0
	for index, item := range journal.Items {
		items[index] = runtimePruneItemResult(item)
		removed += items[index].RemovedTagCount
	}
	return tobari.RuntimePruneResult{Task: tobari.TaskRuntimePruneApply, PlanRef: journal.Plan.PlanRef, State: tobari.RuntimePruneApplied, Items: items, RemovedTagCount: removed, SourcePreserved: true, SnapshotsPreserved: true, HistoryPreserved: true}
}
