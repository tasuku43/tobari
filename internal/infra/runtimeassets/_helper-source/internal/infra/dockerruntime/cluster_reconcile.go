package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

const (
	clusterJournalSchema = 1
	clusterOperationUp   = "up"
	clusterOperationDown = "down"
	clusterPhaseStarted  = "started"
	clusterPhaseRuntime  = "runtime_reconciled"
)

type clusterReconcileJournal struct {
	SchemaVersion int    `json:"schema_version"`
	Operation     string `json:"operation"`
	Phase         string `json:"phase"`
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
	return r.writeClusterJournal(clusterReconcileJournal{
		SchemaVersion: clusterJournalSchema,
		Operation:     operation,
		Phase:         clusterPhaseStarted,
	})
}

func (r *Runtime) markClusterRuntimeReconciled(operation string) error {
	return r.writeClusterJournal(clusterReconcileJournal{
		SchemaVersion: clusterJournalSchema,
		Operation:     operation,
		Phase:         clusterPhaseRuntime,
	})
}
