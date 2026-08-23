package dockerruntime

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	clusterJournalSchema = 3
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
	CandidateImages     *candidateClusterImages            `json:"candidate_images,omitempty"`
	CandidateCompose    *candidateComposeClosureReceipt    `json:"candidate_compose,omitempty"`
	FreshResources      *freshClusterResourceAuthority     `json:"fresh_resources,omitempty"`
	PreviousPrincipals  *projectPrincipalRegistry          `json:"previous_principals,omitempty"`
	CandidatePrincipals *projectPrincipalRegistry          `json:"candidate_principals,omitempty"`
}

// composeAssetReceipt is the exact cleanup execution authority for one
// materialized candidate Compose input. Recovery never derives this tuple
// from the current binary or from an unverified RuntimeDirectory.
type composeAssetReceipt struct {
	Path     string `json:"path"`
	OwnerUID int    `json:"owner_uid"`
	Mode     uint32 `json:"mode"`
	SHA256   string `json:"sha256"`
}

func (r composeAssetReceipt) Validate() error {
	digest, err := hex.DecodeString(r.SHA256)
	if !filepath.IsAbs(r.Path) || filepath.Clean(r.Path) != r.Path || r.OwnerUID < 0 ||
		r.Mode != uint32(0o600) || err != nil || len(digest) != 32 {
		return fmt.Errorf("candidate Compose asset receipt is invalid")
	}
	return nil
}

// candidateComposeClosureReceipt binds every Compose file that fresh cleanup
// may execute. Build and permission overlays are closed by the recorded build
// and permission profiles rather than rediscovered during recovery.
type candidateComposeClosureReceipt struct {
	RuntimeDirectory string                             `json:"runtime_directory"`
	AssetVersion     string                             `json:"asset_version"`
	Profile          tobari.SharedClusterAppliedProfile `json:"profile"`
	Base             composeAssetReceipt                `json:"base"`
	Build            *composeAssetReceipt               `json:"build,omitempty"`
	Permission       composeAssetReceipt                `json:"permission"`
}

func (r candidateComposeClosureReceipt) Validate() error {
	if !filepath.IsAbs(r.RuntimeDirectory) || filepath.Clean(r.RuntimeDirectory) != r.RuntimeDirectory ||
		r.AssetVersion == "" || r.Profile.Validate() != nil || r.Profile == tobari.SharedClusterProfilePrePlatform ||
		r.Base.Validate() != nil || r.Permission.Validate() != nil {
		return fmt.Errorf("candidate Compose closure receipt is invalid")
	}
	if brokerRuntimeEnabled {
		if r.Build == nil || r.Build.Validate() != nil {
			return fmt.Errorf("candidate research Compose closure receipt is incomplete")
		}
	} else if r.Build != nil {
		return fmt.Errorf("candidate standard Compose closure contains research authority")
	}
	return nil
}

type candidateClusterImages struct {
	Gateway    string `json:"gateway"`
	OPA        string `json:"opa"`
	AuthBroker string `json:"auth_broker,omitempty"`
}

func (i candidateClusterImages) Validate() error {
	if !imageIDPattern.MatchString(i.Gateway) || !imageIDPattern.MatchString(i.OPA) {
		return fmt.Errorf("cluster journal candidate image authority is invalid")
	}
	if brokerRuntimeEnabled {
		if !imageIDPattern.MatchString(i.AuthBroker) {
			return fmt.Errorf("cluster journal candidate Auth Broker image authority is invalid")
		}
	} else if i.AuthBroker != "" {
		return fmt.Errorf("standard cluster journal contains candidate Auth Broker authority")
	}
	return nil
}

func (j clusterReconcileJournal) Validate() error {
	if j.SchemaVersion == 1 {
		if j.Operation != clusterOperationDown ||
			(j.Phase != clusterPhaseStarted && j.Phase != clusterPhaseRuntime) ||
			j.PreviousState != nil || j.CandidateState != nil || j.CandidateProfile != "" ||
			j.CandidateImages != nil || j.CandidateCompose != nil || j.FreshResources != nil ||
			j.PreviousPrincipals != nil || j.CandidatePrincipals != nil {
			return fmt.Errorf("predecessor cluster journal is not an exact down recovery record")
		}
		return nil
	}
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
		if j.CandidateState == nil || j.PreviousPrincipals == nil || j.CandidateImages == nil || j.CandidateCompose == nil {
			return fmt.Errorf("cluster up journal omits recovery authority")
		}
		if err := j.CandidateImages.Validate(); err != nil {
			return err
		}
		if err := j.CandidateProfile.Validate(); err != nil || j.CandidateProfile == tobari.SharedClusterProfilePrePlatform {
			return fmt.Errorf("cluster up journal candidate profile is invalid")
		}
		if err := j.CandidateState.Validate(); err != nil {
			return fmt.Errorf("cluster up journal candidate: %w", err)
		}
		if err := j.CandidateCompose.Validate(); err != nil ||
			j.CandidateCompose.RuntimeDirectory != j.CandidateState.RuntimeDirectory ||
			j.CandidateCompose.AssetVersion != j.CandidateState.AssetVersion ||
			j.CandidateCompose.Profile != j.CandidateProfile {
			return fmt.Errorf("cluster up journal candidate Compose authority is invalid")
		}
		if j.PreviousState != nil {
			if err := j.PreviousState.Validate(); err != nil {
				return fmt.Errorf("cluster up journal previous state: %w", err)
			}
			if j.PreviousState.SchemaVersion != 2 {
				return fmt.Errorf("cluster up journal previous state must be schema 2")
			}
			if j.FreshResources != nil {
				return fmt.Errorf("existing cluster up journal contains fresh resource authority")
			}
		} else if j.FreshResources == nil {
			return fmt.Errorf("fresh cluster up journal omits resource authority")
		} else if err := j.FreshResources.Validate(); err != nil {
			return err
		}
		if err := j.PreviousPrincipals.Validate(); err != nil {
			return fmt.Errorf("cluster up journal principals: %w", err)
		}
		if j.CandidatePrincipals != nil {
			if err := j.CandidatePrincipals.Validate(); err != nil {
				return fmt.Errorf("cluster up journal candidate principals: %w", err)
			}
		}
		if j.Phase == clusterPhaseRuntime {
			if j.CandidateState.SchemaVersion != 2 || j.CandidatePrincipals == nil {
				return fmt.Errorf("reconciled cluster up journal omits candidate authority")
			}
			if j.CandidateState.Applied.GatewayImageID != j.CandidateImages.Gateway ||
				j.CandidateState.Applied.OPAImageID != j.CandidateImages.OPA ||
				j.CandidateState.Applied.AuthBrokerImageID != j.CandidateImages.AuthBroker {
				return fmt.Errorf("reconciled cluster up journal candidate image authority drifted")
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
	principals projectPrincipalRegistry, freshResources *freshClusterResourceAuthority,
	images candidateClusterImages, compose candidateComposeClosureReceipt,
) error {
	return r.writeClusterJournal(clusterReconcileJournal{
		SchemaVersion: clusterJournalSchema, Operation: clusterOperationUp, Phase: clusterPhaseStarted,
		PreviousState: previous, CandidateState: &candidate, CandidateProfile: profile,
		PreviousPrincipals: &principals, FreshResources: freshResources, CandidateImages: &images,
		CandidateCompose: &compose,
	})
}

func (r *Runtime) recordClusterUpCandidatePrincipals(principals projectPrincipalRegistry) error {
	journal, exists, err := r.readClusterJournal()
	if err != nil {
		return err
	}
	if !exists || journal.Operation != clusterOperationUp || journal.Phase != clusterPhaseStarted {
		return fmt.Errorf("cluster up reconcile journal is not in its started phase")
	}
	journal.CandidatePrincipals = &principals
	return r.writeClusterJournal(journal)
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
