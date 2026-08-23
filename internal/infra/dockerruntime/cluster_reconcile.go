package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	clusterJournalSchema = 2
	clusterOperationUp   = "up"
	clusterOperationDown = "down"
	clusterPhaseStarted  = "started"
	clusterPhaseRuntime  = "runtime_reconciled"
)

type clusterReconcileJournal struct {
	SchemaVersion       int                                `json:"schema_version"`
	Operation           string                             `json:"operation"`
	Phase               string                             `json:"phase"`
	PreviousState       *tobari.State                      `json:"previous_state,omitempty"`
	CandidateState      *tobari.State                      `json:"candidate_state,omitempty"`
	CandidateProfile    tobari.SharedClusterAppliedProfile `json:"candidate_profile,omitempty"`
	PreviousPrincipals  *projectPrincipalRegistry          `json:"previous_principals,omitempty"`
	CandidatePrincipals *projectPrincipalRegistry          `json:"candidate_principals,omitempty"`
}

func (j clusterReconcileJournal) Validate() error {
	if j.SchemaVersion != clusterJournalSchema {
		return fmt.Errorf("cluster journal schema version must be %d", clusterJournalSchema)
	}
	if j.Operation != clusterOperationUp && j.Operation != clusterOperationDown {
		return fmt.Errorf("cluster journal operation is invalid")
	}
	if j.Phase != clusterPhaseStarted && j.Phase != clusterPhaseRuntime {
		return fmt.Errorf("cluster journal phase is invalid")
	}
	if j.Operation == clusterOperationUp {
		if j.CandidateState == nil || j.PreviousPrincipals == nil {
			return fmt.Errorf("cluster up journal omits recovery authority")
		}
		if err := j.CandidateProfile.Validate(); err != nil || j.CandidateProfile == tobari.SharedClusterProfilePrePlatform {
			return fmt.Errorf("cluster up journal candidate profile is invalid")
		}
		if err := j.CandidateState.Validate(); err != nil {
			return fmt.Errorf("cluster up journal candidate: %w", err)
		}
		if j.PreviousState != nil {
			if err := j.PreviousState.Validate(); err != nil {
				return fmt.Errorf("cluster up journal previous state: %w", err)
			}
			if j.PreviousState.SchemaVersion != 2 {
				return fmt.Errorf("cluster up journal previous state must be schema 2")
			}
		}
		if err := j.PreviousPrincipals.Validate(); err != nil {
			return fmt.Errorf("cluster up journal principals: %w", err)
		}
		if j.Phase == clusterPhaseRuntime {
			if j.CandidateState.SchemaVersion != 2 || j.CandidatePrincipals == nil {
				return fmt.Errorf("reconciled cluster up journal omits candidate authority")
			}
			if err := j.CandidatePrincipals.Validate(); err != nil {
				return fmt.Errorf("cluster up journal candidate principals: %w", err)
			}
		}
	}
	return nil
}

func (r *Runtime) clusterJournalPath() string {
	return filepath.Join(r.stateDirectory, "cluster-reconcile.json")
}

func (r *Runtime) writeClusterJournal(journal clusterReconcileJournal) error {
	if err := journal.Validate(); err != nil {
		return err
	}
	return r.withClusterLock(func() error {
		return writeAtomicJSON(r.clusterJournalPath(), journal)
	})
}

func (r *Runtime) readClusterJournal() (clusterReconcileJournal, bool, error) {
	var journal clusterReconcileJournal
	if err := readStrictJSON(r.clusterJournalPath(), &journal); errors.Is(err, os.ErrNotExist) {
		return clusterReconcileJournal{}, false, nil
	} else if err != nil {
		return clusterReconcileJournal{}, false, err
	}
	if err := journal.Validate(); err != nil {
		return clusterReconcileJournal{}, false, err
	}
	return journal, true, nil
}

func (r *Runtime) clearClusterJournal() error {
	if r.clusterJournalClearHook != nil {
		if err := r.clusterJournalClearHook(); err != nil {
			return err
		}
	}
	return r.withClusterLock(func() error {
		if err := os.Remove(r.clusterJournalPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return syncDirectoryIfPresent(r.stateDirectory)
	})
}

func (r *Runtime) requireNoInterruptedClusterReconcile(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, exists, err := r.readClusterJournal(); err != nil {
		return fmt.Errorf("read interrupted cluster reconcile journal: %w", err)
	} else if exists {
		return fault.New(
			fault.KindUnavailable, "cluster_reconcile_interrupted",
			"the shared cluster reconcile was interrupted; rerun an explicit cluster operation", false,
			fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared Gateway, OPA, and Auth Broker cluster."},
			fault.NextAction{Command: "cluster down", Reason: "Explicitly clean up the shared cluster instead."},
		)
	}
	return nil
}

func (r *Runtime) startClusterReconcile(operation string) error {
	if operation == clusterOperationUp {
		return fmt.Errorf("cluster up reconcile requires explicit recovery authority")
	}
	return r.writeClusterJournal(clusterReconcileJournal{
		SchemaVersion: clusterJournalSchema,
		Operation:     operation,
		Phase:         clusterPhaseStarted,
	})
}

func (r *Runtime) startClusterUpReconcile(
	previous *tobari.State, candidate tobari.State, profile tobari.SharedClusterAppliedProfile,
	principals projectPrincipalRegistry,
) error {
	return r.writeClusterJournal(clusterReconcileJournal{
		SchemaVersion: clusterJournalSchema, Operation: clusterOperationUp, Phase: clusterPhaseStarted,
		PreviousState: previous, CandidateState: &candidate, CandidateProfile: profile,
		PreviousPrincipals: &principals,
	})
}

func (r *Runtime) markClusterRuntimeReconciled(operation string) error {
	if operation == clusterOperationUp {
		return fmt.Errorf("cluster up reconcile requires its exact candidate state")
	}
	return r.writeClusterJournal(clusterReconcileJournal{
		SchemaVersion: clusterJournalSchema,
		Operation:     operation,
		Phase:         clusterPhaseRuntime,
	})
}

func (r *Runtime) markClusterUpRuntimeReconciled(
	candidate tobari.State, principals projectPrincipalRegistry,
) error {
	journal, exists, err := r.readClusterJournal()
	if err != nil {
		return err
	}
	if !exists || journal.Operation != clusterOperationUp || journal.Phase != clusterPhaseStarted {
		return fmt.Errorf("cluster up reconcile journal is not in its started phase")
	}
	journal.Phase = clusterPhaseRuntime
	journal.CandidateState = &candidate
	journal.CandidatePrincipals = &principals
	return r.writeClusterJournal(journal)
}
