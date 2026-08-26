package workspaceauthoritystore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type InstallationMigrationSourceStage interface {
	ExpectedIdentity() (tobari.SemanticDigest, error)
	Commit(context.Context) error
	Verify(context.Context) error
	Rollback(context.Context) error
	Complete(context.Context) error
	Abort(context.Context) error
}

type InstallationMigrationSourcePreparer func(context.Context, tobari.WorkspaceAuthorityCollection, bool) (InstallationMigrationSourceStage, error)
type InstallationMigrationRuntimeObserver func(context.Context, tobari.WorkspaceAuthorityCollection) (tobari.SemanticDigest, error)

type installationMigrationTransaction struct {
	SchemaVersion  int                                 `json:"schema_version"`
	PlanRef        string                              `json:"plan_ref"`
	Plan           tobari.InstallationMigrationPlan    `json:"plan"`
	Phase          string                              `json:"phase"`
	Collection     tobari.WorkspaceAuthorityCollection `json:"collection"`
	SourceIdentity tobari.SemanticDigest               `json:"source_identity"`
}

type installationMigrationAcceptedReceipt struct {
	SchemaVersion        int                              `json:"schema_version"`
	Plan                 tobari.InstallationMigrationPlan `json:"plan"`
	ActiveGeneration     uint64                           `json:"active_generation"`
	ActiveRevision       tobari.SemanticDigest            `json:"active_revision"`
	InstallationIdentity tobari.SemanticDigest            `json:"installation_identity"`
	ReceiptDigest        tobari.SemanticDigest            `json:"receipt_digest"`
}

func (m *Mutator) installationMigrationBoundaryCall(phase string) error {
	if m.installationMigrationBoundary != nil {
		return m.installationMigrationBoundary(phase)
	}
	return nil
}

func (m *Mutator) PlanInstallationMigration(ctx context.Context, observeRuntime InstallationMigrationRuntimeObserver) (tobari.InstallationMigrationPlan, error) {
	if m == nil || m.store == nil {
		return tobari.InstallationMigrationPlan{}, fmt.Errorf("installation migration planner is unavailable")
	}
	collection, digest, err := m.store.readLegacyTypedAuthority(ctx)
	if err != nil {
		return tobari.InstallationMigrationPlan{}, err
	}
	if observeRuntime == nil {
		return tobari.InstallationMigrationPlan{}, fmt.Errorf("installation Runtime migration observer is unavailable")
	}
	runtimeDigest, err := observeRuntime(ctx, collection.Clone())
	if err != nil {
		return tobari.InstallationMigrationPlan{}, err
	}
	return tobari.NewInstallationMigrationPlan(tobari.SemanticDigest(digest), runtimeDigest, collection)
}

func (m *Mutator) ApplyInstallationMigration(ctx context.Context, planRef string, observeRuntime InstallationMigrationRuntimeObserver, prepareSources InstallationMigrationSourcePreparer) (tobari.InstallationMigrationResult, error) {
	if tobari.ParseInstallationMigrationPlanRef(planRef) != nil || observeRuntime == nil || prepareSources == nil {
		return tobari.InstallationMigrationResult{}, tobari.ErrMigrationSourceUnsafe
	}
	var result tobari.InstallationMigrationResult
	err := m.lifecycle.WithLifecycleLock(ctx, func(locked context.Context) error {
		if accepted, recovered, err := m.recoverInstallationMigrationTransaction(locked, planRef, prepareSources); err != nil {
			return err
		} else if accepted {
			result = recovered
			return nil
		}
		collection, digest, err := m.store.readLegacyTypedAuthority(locked)
		if err != nil {
			return err
		}
		runtimeDigest, err := observeRuntime(locked, collection.Clone())
		if err != nil {
			return err
		}
		plan, err := tobari.NewInstallationMigrationPlan(tobari.SemanticDigest(digest), runtimeDigest, collection)
		if err != nil {
			return err
		}
		if plan.PlanRef != planRef {
			return tobari.ErrMigrationSourceChanged
		}
		sourceStage, err := prepareSources(locked, collection.Clone(), false)
		if err != nil {
			return err
		}
		sourceCommitted := false
		defer func() {
			if !sourceCommitted {
				_ = sourceStage.Abort(locked)
			}
		}()
		sourceIdentity, err := sourceStage.ExpectedIdentity()
		if err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		second, secondDigest, err := m.store.readLegacyTypedAuthority(locked)
		if err != nil {
			return err
		}
		if secondDigest != digest || second.Generation != collection.Generation || second.Revision != collection.Revision {
			return tobari.ErrMigrationSourceChanged
		}
		secondRuntimeDigest, err := observeRuntime(locked, collection.Clone())
		if err != nil || secondRuntimeDigest != runtimeDigest {
			return errors.Join(tobari.ErrMigrationSourceChanged, err)
		}
		prepared, err := prepareAuthorityGenerationWithMigrationProvenance(collection, &plan)
		if err != nil {
			return err
		}
		parent := filepath.Dir(m.store.root)
		stage := m.store.root + ".migration-stage"
		backup := m.store.root + ".migration-old"
		if _, inspectErr := os.Lstat(stage); inspectErr == nil {
			if _, backupErr := os.Lstat(backup); !errors.Is(backupErr, os.ErrNotExist) {
				return tobari.ErrMigrationWriteFailed
			}
			stagedStore, openErr := New(stage)
			if openErr != nil {
				return errors.Join(tobari.ErrMigrationWriteFailed, openErr)
			}
			staged, present, readErr := stagedStore.readGenerationRaw(locked)
			stagedPlan, provenancePresent, provenanceErr := stagedStore.readActiveInstallationMigrationProvenance(locked, staged)
			if readErr != nil || !present || provenanceErr != nil || !provenancePresent || stagedPlan != plan || staged.Generation != collection.Generation || staged.Revision != collection.Revision {
				return errors.Join(tobari.ErrMigrationWriteFailed, readErr)
			}
			if err := removeMigrationTree(stage, m.store.root); err != nil {
				return errors.Join(tobari.ErrMigrationWriteFailed, err)
			}
			if err := m.sync(parent); err != nil {
				return errors.Join(tobari.ErrMigrationWriteFailed, err)
			}
		} else if !errors.Is(inspectErr, os.ErrNotExist) {
			return errors.Join(tobari.ErrMigrationWriteFailed, inspectErr)
		}
		for _, reserved := range []string{stage, backup} {
			if _, inspectErr := os.Lstat(reserved); !errors.Is(inspectErr, os.ErrNotExist) {
				return tobari.ErrMigrationWriteFailed
			}
		}
		if err := materializePreparedGeneration(stage, prepared); err != nil {
			_ = removeMigrationTree(stage, m.store.root)
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.installationMigrationBoundaryCall("authority_stage_materialized"); err != nil {
			return err
		}
		if err := syncAuthorityTree(stage, m.sync); err != nil {
			_ = removeMigrationTree(stage, m.store.root)
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.installationMigrationBoundaryCall("authority_stage_synced"); err != nil {
			return err
		}
		stagedStore, _ := New(stage)
		observed, present, err := stagedStore.readGenerationRaw(locked)
		observedPlan, provenancePresent, provenanceErr := stagedStore.readActiveInstallationMigrationProvenance(locked, observed)
		if err != nil || !present || provenanceErr != nil || !provenancePresent || observedPlan != plan || observed.Generation != collection.Generation || observed.Revision != collection.Revision {
			_ = removeMigrationTree(stage, m.store.root)
			return errors.Join(tobari.ErrMigrationWriteFailed, err, provenanceErr)
		}
		if err := m.installationMigrationBoundaryCall("authority_stage_readback"); err != nil {
			return err
		}
		transaction := installationMigrationTransaction{SchemaVersion: 3, PlanRef: planRef, Plan: plan, Phase: "prepared", Collection: collection.Clone(), SourceIdentity: sourceIdentity}
		if err := m.writeInstallationMigrationTransaction(transaction); err != nil {
			_ = removeMigrationTree(stage, m.store.root)
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.installationMigrationBoundaryCall("outer_journal_written"); err != nil {
			return err
		}
		if err := sourceStage.Commit(locked); err != nil {
			abortErr := sourceStage.Abort(locked)
			if abortErr == nil {
				_ = m.removeInstallationMigrationTransaction()
			}
			return errors.Join(tobari.ErrMigrationWriteFailed, err, abortErr)
		}
		if err := m.installationMigrationBoundaryCall("components_committed"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		sourceCommitted = true
		transaction.Phase = "sources_committed"
		if err := m.replaceInstallationMigrationTransaction(transaction); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.installationMigrationBoundaryCall("outer_sources_phase_written"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		rollbackSources := func() error {
			sourceCommitted = false
			return sourceStage.Rollback(locked)
		}
		if err := m.rename(m.store.root, backup); err != nil {
			_ = removeMigrationTree(stage, m.store.root)
			return errors.Join(tobari.ErrMigrationWriteFailed, err, rollbackSources())
		}
		if err := m.installationMigrationBoundaryCall("authority_legacy_quarantined"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.rename(stage, m.store.root); err != nil {
			rollbackErr := m.rename(backup, m.store.root)
			cleanupErr := removeMigrationTree(stage, m.store.root)
			return errors.Join(tobari.ErrMigrationWriteFailed, err, rollbackErr, cleanupErr, rollbackSources())
		}
		if err := m.installationMigrationBoundaryCall("authority_new_published"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.sync(parent); err != nil {
			rollbackErr := m.rollbackInstallationMigration(parent, stage, backup)
			return errors.Join(tobari.ErrMigrationWriteFailed, err, rollbackErr, rollbackSources())
		}
		if err := m.installationMigrationBoundaryCall("authority_parent_synced"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		transaction.Phase = "authority_published"
		if err := m.replaceInstallationMigrationTransaction(transaction); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.installationMigrationBoundaryCall("outer_authority_phase_written"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		active, activePresent, err := m.store.readGenerationRaw(locked)
		activePlan, provenancePresent, provenanceErr := m.store.readActiveInstallationMigrationProvenance(locked, active)
		if err != nil || !activePresent || provenanceErr != nil || !provenancePresent || activePlan != plan || active.Generation != collection.Generation || active.Revision != collection.Revision {
			rollbackErr := m.rollbackInstallationMigration(parent, stage, backup)
			return errors.Join(tobari.ErrMigrationWriteFailed, err, provenanceErr, rollbackErr, rollbackSources())
		}
		if err := m.installationMigrationBoundaryCall("authority_readback"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := sourceStage.Verify(locked); err != nil {
			rollbackErr := m.rollbackInstallationMigration(parent, stage, backup)
			return errors.Join(tobari.ErrMigrationWriteFailed, err, rollbackErr, rollbackSources())
		}
		if err := m.installationMigrationBoundaryCall("components_readback"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		transaction.Phase = "verified"
		if err := m.replaceInstallationMigrationTransaction(transaction); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.installationMigrationBoundaryCall("outer_verified_phase_written"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := removeMigrationTree(backup, m.store.root); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.installationMigrationBoundaryCall("authority_backup_removed"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.sync(parent); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.installationMigrationBoundaryCall("authority_backup_remove_synced"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		transaction.Phase = "accepted"
		if err := m.replaceInstallationMigrationTransaction(transaction); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.installationMigrationBoundaryCall("outer_accepted_phase_written"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := sourceStage.Complete(locked); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.installationMigrationBoundaryCall("components_completed"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		sourceCommitted = true
		result = tobari.InstallationMigrationResult{SchemaVersion: 1, PlanRef: planRef, ActiveGeneration: active.Generation, ActiveRevision: active.Revision, Changed: true}
		if err := result.Validate(); err != nil {
			return err
		}
		if err := m.writeInstallationMigrationAcceptedReceipt(plan, result); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.removeInstallationMigrationTransaction(); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.installationMigrationBoundaryCall("outer_journal_removed"); err != nil {
			return errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		return nil
	})
	return result, err
}

func (m *Mutator) installationMigrationTransactionPath() string {
	return m.store.root + ".migration-transaction.json"
}

func (m *Mutator) installationMigrationAcceptedReceiptPath() string {
	return filepath.Join(m.store.root, "journal", "installation-migration-accepted.json")
}

// The accepted receipt is immutable installation provenance and is retained
// with authority history. Automatic retirement would reopen the response-loss
// window; only a future explicit history-retention mutation may remove it.
func (m *Mutator) installationMigrationIdentity() tobari.SemanticDigest {
	return tobari.SemanticDigest(digestBytes([]byte("tobari-installation-migration\x00" + filepath.Clean(m.store.root))))
}

func installationMigrationAcceptedReceiptDigest(receipt installationMigrationAcceptedReceipt) tobari.SemanticDigest {
	receipt.ReceiptDigest = ""
	data, _ := json.Marshal(receipt)
	return tobari.SemanticDigest(digestBytes(data))
}

func (m *Mutator) newInstallationMigrationAcceptedReceipt(plan tobari.InstallationMigrationPlan, result tobari.InstallationMigrationResult) (installationMigrationAcceptedReceipt, error) {
	if err := plan.Validate(); err != nil {
		return installationMigrationAcceptedReceipt{}, err
	}
	if err := result.Validate(); err != nil {
		return installationMigrationAcceptedReceipt{}, err
	}
	if result.PlanRef != plan.PlanRef || result.ActiveGeneration != plan.TargetGeneration || result.ActiveRevision != plan.SourceRevision {
		return installationMigrationAcceptedReceipt{}, fmt.Errorf("installation migration result does not match its exact plan authority")
	}
	receipt := installationMigrationAcceptedReceipt{
		SchemaVersion:        2,
		Plan:                 plan,
		ActiveGeneration:     result.ActiveGeneration,
		ActiveRevision:       result.ActiveRevision,
		InstallationIdentity: m.installationMigrationIdentity(),
	}
	receipt.ReceiptDigest = installationMigrationAcceptedReceiptDigest(receipt)
	return receipt, m.validateInstallationMigrationAcceptedReceipt(receipt)
}

func (m *Mutator) validateInstallationMigrationAcceptedReceipt(receipt installationMigrationAcceptedReceipt) error {
	if receipt.SchemaVersion != 2 || receipt.Plan.Validate() != nil || receipt.ActiveGeneration == 0 || receipt.ActiveRevision.Validate() != nil || receipt.InstallationIdentity.Validate() != nil || receipt.ReceiptDigest.Validate() != nil {
		return fmt.Errorf("installation migration accepted receipt is invalid")
	}
	if receipt.InstallationIdentity != m.installationMigrationIdentity() || receipt.ActiveGeneration != receipt.Plan.TargetGeneration || receipt.ActiveRevision != receipt.Plan.SourceRevision || receipt.ReceiptDigest != installationMigrationAcceptedReceiptDigest(receipt) {
		return fmt.Errorf("installation migration accepted receipt does not match its authority")
	}
	return nil
}

func installationMigrationPlanMatchesCollection(plan tobari.InstallationMigrationPlan, collection tobari.WorkspaceAuthorityCollection) bool {
	return plan.SourceGeneration == collection.Generation &&
		plan.TargetGeneration == collection.Generation &&
		plan.SourceRevision == collection.Revision &&
		plan.TemplateCount == len(collection.Templates) &&
		plan.ContextCount == len(collection.Contexts) &&
		plan.PolicyMemoryCount == len(collection.Contexts) &&
		plan.WorkspaceCount == len(collection.Workspaces)
}

func removeSafeInstallationMigrationReceiptStage(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) {
		return fmt.Errorf("installation migration accepted receipt stage is unsafe")
	}
	file, err := os.Open(path) // #nosec G304 -- exact deterministic transaction child.
	if err != nil {
		return err
	}
	opened, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil || closeErr != nil || !opened.Mode().IsRegular() || opened.Mode().Perm() != 0o600 || !ownedByCurrentUser(opened) || !os.SameFile(info, opened) {
		return errors.Join(fmt.Errorf("installation migration accepted receipt stage changed during safe open"), statErr, closeErr)
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, current) {
		return errors.Join(fmt.Errorf("installation migration accepted receipt stage changed before cleanup"), err)
	}
	return os.Remove(path)
}

// writeInstallationMigrationAcceptedReceipt is called only while an exact
// accepted outer transaction remains durable. That transaction authorizes
// replacing a safe partial .next file, but never an existing conflicting
// canonical receipt.
func (m *Mutator) writeInstallationMigrationAcceptedReceipt(plan tobari.InstallationMigrationPlan, result tobari.InstallationMigrationResult) (resultErr error) {
	receipt, err := m.newInstallationMigrationAcceptedReceipt(plan, result)
	if err != nil {
		return err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	path := m.installationMigrationAcceptedReceiptPath()
	temp := path + ".next"
	parent := filepath.Dir(path)
	if existing, err := readAuthorityFile(path); err == nil {
		var observed installationMigrationAcceptedReceipt
		if decodeStrictJSON(existing, &observed) != nil || m.validateInstallationMigrationAcceptedReceipt(observed) != nil || observed != receipt {
			return fmt.Errorf("installation migration accepted receipt conflicts with the requested result")
		}
		if err := removeSafeInstallationMigrationReceiptStage(temp); err != nil {
			return err
		}
		return m.sync(parent)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if staged, err := readAuthorityFile(temp); err == nil {
		var observed installationMigrationAcceptedReceipt
		if decodeStrictJSON(staged, &observed) == nil && m.validateInstallationMigrationAcceptedReceipt(observed) == nil && observed == receipt {
			if err := m.rename(temp, path); err != nil {
				return err
			}
			if err := m.installationMigrationBoundaryCall("accepted_receipt_renamed"); err != nil {
				return err
			}
			if err := m.sync(parent); err != nil {
				return err
			}
			return m.installationMigrationBoundaryCall("accepted_receipt_parent_synced")
		}
		if err := removeSafeInstallationMigrationReceiptStage(temp); err != nil {
			return err
		}
		if err := m.sync(parent); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		// A zero-length or otherwise partial but safely owned regular file is
		// transaction-owned crash residue and may be retired before regeneration.
		if cleanupErr := removeSafeInstallationMigrationReceiptStage(temp); cleanupErr != nil {
			return errors.Join(err, cleanupErr)
		}
		if syncErr := m.sync(parent); syncErr != nil {
			return syncErr
		}
	}
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- deterministic reserved child under the installation lock.
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); resultErr == nil && closeErr != nil {
				resultErr = closeErr
			}
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := m.installationMigrationBoundaryCall("accepted_receipt_temp_written"); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := m.installationMigrationBoundaryCall("accepted_receipt_temp_synced"); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	closed = true
	if err := m.rename(temp, path); err != nil {
		return err
	}
	if err := m.installationMigrationBoundaryCall("accepted_receipt_renamed"); err != nil {
		return err
	}
	if err := m.sync(parent); err != nil {
		return err
	}
	return m.installationMigrationBoundaryCall("accepted_receipt_parent_synced")
}

func (m *Mutator) readInstallationMigrationAcceptedReceipt(ctx context.Context, planRef string) (tobari.InstallationMigrationResult, bool, error) {
	path := m.installationMigrationAcceptedReceiptPath()
	data, err := readAuthorityFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return tobari.InstallationMigrationResult{}, false, nil
	}
	if err != nil {
		return tobari.InstallationMigrationResult{}, false, errors.Join(tobari.ErrMigrationWriteFailed, err)
	}
	var receipt installationMigrationAcceptedReceipt
	if err := decodeStrictJSON(data, &receipt); err != nil || m.validateInstallationMigrationAcceptedReceipt(receipt) != nil {
		return tobari.InstallationMigrationResult{}, false, errors.Join(tobari.ErrMigrationWriteFailed, fmt.Errorf("installation migration accepted receipt is invalid"), err)
	}
	if receipt.Plan.PlanRef != planRef {
		return tobari.InstallationMigrationResult{}, false, tobari.ErrMigrationSourceChanged
	}
	active, present, err := m.store.readGenerationRaw(ctx)
	if err != nil || !present || active.Generation != receipt.ActiveGeneration || active.Revision != receipt.ActiveRevision || !installationMigrationPlanMatchesCollection(receipt.Plan, active) {
		return tobari.InstallationMigrationResult{}, false, errors.Join(tobari.ErrMigrationWriteFailed, fmt.Errorf("installation migration accepted receipt does not match active authority"), err)
	}
	provenance, provenancePresent, provenanceErr := m.store.readActiveInstallationMigrationProvenance(ctx, active)
	if provenanceErr != nil || !provenancePresent || provenance != receipt.Plan {
		return tobari.InstallationMigrationResult{}, false, errors.Join(tobari.ErrMigrationWriteFailed, fmt.Errorf("installation migration accepted receipt does not match active generation provenance"), provenanceErr)
	}
	if err := removeSafeInstallationMigrationReceiptStage(path + ".next"); err != nil {
		return tobari.InstallationMigrationResult{}, false, errors.Join(tobari.ErrMigrationWriteFailed, err)
	}
	if err := m.sync(filepath.Dir(path)); err != nil {
		return tobari.InstallationMigrationResult{}, false, errors.Join(tobari.ErrMigrationWriteFailed, err)
	}
	result := tobari.InstallationMigrationResult{SchemaVersion: 1, PlanRef: receipt.Plan.PlanRef, ActiveGeneration: receipt.ActiveGeneration, ActiveRevision: receipt.ActiveRevision, Changed: true}
	return result, true, result.Validate()
}

func (m *Mutator) writeInstallationMigrationTransaction(transaction installationMigrationTransaction) error {
	path := m.installationMigrationTransactionPath()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(tobari.ErrMigrationWriteFailed, err)
	}
	return m.replaceInstallationMigrationTransaction(transaction)
}

func (m *Mutator) replaceInstallationMigrationTransaction(transaction installationMigrationTransaction) error {
	data, err := json.Marshal(transaction)
	if err != nil {
		return err
	}
	path := m.installationMigrationTransactionPath()
	temp := path + ".next"
	if _, err := os.Lstat(temp); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(tobari.ErrMigrationWriteFailed, fmt.Errorf("migration transaction phase stage is occupied: %w", err))
	}
	if err := writeMutationFile(temp, data); err != nil {
		return err
	}
	if err := m.installationMigrationBoundaryCall("outer_phase_temp_synced:" + transaction.Phase); err != nil {
		return err
	}
	if err := m.rename(temp, path); err != nil {
		return err
	}
	if err := m.installationMigrationBoundaryCall("outer_phase_renamed:" + transaction.Phase); err != nil {
		return err
	}
	if err := m.sync(filepath.Dir(path)); err != nil {
		return err
	}
	return m.installationMigrationBoundaryCall("outer_phase_parent_synced:" + transaction.Phase)
}

func (m *Mutator) removeInstallationMigrationTransaction() error {
	path := m.installationMigrationTransactionPath()
	if err := m.installationMigrationBoundaryCall("outer_cleanup_prepared"); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := m.installationMigrationBoundaryCall("outer_transaction_removed"); err != nil {
		return err
	}
	if err := os.Remove(path + ".next"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := m.sync(filepath.Dir(path)); err != nil {
		return err
	}
	return m.installationMigrationBoundaryCall("outer_cleanup_synced")
}

// recoverInstallationMigrationTransaction restores the exact legacy root when
// a crash interrupted either source selection or the authority swap. If the
// legacy backup has already been durably retired, the new generation was the
// accepted outcome and the exact same plan is settled idempotently.
func (m *Mutator) recoverInstallationMigrationTransaction(ctx context.Context, planRef string, prepareSources InstallationMigrationSourcePreparer) (bool, tobari.InstallationMigrationResult, error) {
	path := m.installationMigrationTransactionPath()
	if receipt, present, err := m.readInstallationMigrationAcceptedReceipt(ctx, planRef); err != nil {
		return false, tobari.InstallationMigrationResult{}, err
	} else if present {
		if err := m.removeInstallationMigrationTransaction(); err != nil {
			return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		return true, receipt, nil
	}
	data, err := readAuthorityFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if removeErr := os.Remove(path + ".next"); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, removeErr)
		}
		return false, tobari.InstallationMigrationResult{}, nil
	}
	if err != nil {
		return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
	}
	if removeErr := os.Remove(path + ".next"); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, removeErr)
	}
	var transaction installationMigrationTransaction
	if err := decodeStrictJSON(data, &transaction); err != nil || transaction.SchemaVersion != 3 || transaction.PlanRef != planRef || transaction.Plan.PlanRef != planRef || transaction.Plan.Validate() != nil || transaction.Collection.Validate() != nil || transaction.SourceIdentity.Validate() != nil || !installationMigrationPlanMatchesCollection(transaction.Plan, transaction.Collection) {
		return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, fmt.Errorf("migration transaction does not match the requested plan"))
	}
	parent := filepath.Dir(m.store.root)
	stage := m.store.root + ".migration-stage"
	backup := m.store.root + ".migration-old"
	sourceStage, prepareErr := prepareSources(ctx, transaction.Collection.Clone(), true)
	if prepareErr != nil {
		return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, prepareErr)
	}
	observedSourceIdentity, identityErr := sourceStage.ExpectedIdentity()
	if identityErr != nil || observedSourceIdentity != transaction.SourceIdentity {
		return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, fmt.Errorf("migration staged source identity differs from the durable transaction"), identityErr)
	}
	if _, err := os.Lstat(backup); err == nil {
		if _, rootErr := os.Lstat(m.store.root); rootErr == nil {
			_ = removeMigrationTree(stage, m.store.root)
			if err := m.rename(m.store.root, stage); err != nil {
				return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
			}
		}
		if err := m.rename(backup, m.store.root); err != nil {
			return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.sync(parent); err != nil {
			return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := removeMigrationTree(stage, m.store.root); err != nil {
			return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := sourceStage.Rollback(ctx); err != nil {
			return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := sourceStage.Abort(ctx); err != nil {
			return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
		}
		if err := m.removeInstallationMigrationTransaction(); err != nil {
			return false, tobari.InstallationMigrationResult{}, err
		}
		return false, tobari.InstallationMigrationResult{}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
	}
	active, present, err := m.store.readGenerationRaw(ctx)
	if err != nil || !present {
		if _, _, legacyErr := m.store.readLegacyTypedAuthority(ctx); legacyErr == nil {
			if rollbackErr := sourceStage.Rollback(ctx); rollbackErr != nil {
				return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, rollbackErr)
			}
			if abortErr := sourceStage.Abort(ctx); abortErr != nil {
				return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, abortErr)
			}
			// The prepared authority generation predates every component Commit.
			// A process death during source publication therefore leaves it behind
			// even though the legacy root remains canonical. Retire that exact
			// transaction-owned stage before dropping the outer journal so the same
			// stale-bound plan can safely prepare again after restart.
			if cleanupErr := removeMigrationTree(stage, m.store.root); cleanupErr != nil {
				return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, cleanupErr)
			}
			if syncErr := m.sync(parent); syncErr != nil {
				return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, syncErr)
			}
			if removeErr := m.removeInstallationMigrationTransaction(); removeErr != nil {
				return false, tobari.InstallationMigrationResult{}, removeErr
			}
			return false, tobari.InstallationMigrationResult{}, nil
		}
		return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
	}
	if active.Generation != transaction.Collection.Generation || active.Revision != transaction.Collection.Revision {
		return false, tobari.InstallationMigrationResult{}, tobari.ErrMigrationWriteFailed
	}
	provenance, provenancePresent, provenanceErr := m.store.readActiveInstallationMigrationProvenance(ctx, active)
	if provenanceErr != nil || !provenancePresent || provenance != transaction.Plan {
		return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, fmt.Errorf("active generation migration provenance does not match the durable transaction"), provenanceErr)
	}
	if err := sourceStage.Verify(ctx); err != nil {
		return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
	}
	if err := sourceStage.Complete(ctx); err != nil {
		return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
	}
	result := tobari.InstallationMigrationResult{SchemaVersion: 1, PlanRef: planRef, ActiveGeneration: active.Generation, ActiveRevision: active.Revision, Changed: true}
	if err := result.Validate(); err != nil {
		return false, tobari.InstallationMigrationResult{}, err
	}
	if err := m.writeInstallationMigrationAcceptedReceipt(transaction.Plan, result); err != nil {
		return false, tobari.InstallationMigrationResult{}, errors.Join(tobari.ErrMigrationWriteFailed, err)
	}
	if err := m.removeInstallationMigrationTransaction(); err != nil {
		return false, tobari.InstallationMigrationResult{}, err
	}
	return true, result, nil
}

// rollbackInstallationMigration restores authority.json to the canonical root
// after the prepared generation has been swapped into place but before that
// generation has been accepted. The rejected generation is displaced back to
// the reserved stage name first, so the old and new authorities are never both
// selectable through the canonical path.
func (m *Mutator) rollbackInstallationMigration(parent, stage, backup string) error {
	if err := m.rename(m.store.root, stage); err != nil {
		return fmt.Errorf("displace rejected generation: %w", err)
	}
	if err := m.rename(backup, m.store.root); err != nil {
		restoreRejectedErr := m.rename(stage, m.store.root)
		return errors.Join(fmt.Errorf("restore legacy authority: %w", err), restoreRejectedErr)
	}
	syncErr := m.sync(parent)
	cleanupErr := removeMigrationTree(stage, m.store.root)
	finalSyncErr := m.sync(parent)
	return errors.Join(syncErr, cleanupErr, finalSyncErr)
}

func (s *Store) readLegacyTypedAuthority(ctx context.Context) (tobari.WorkspaceAuthorityCollection, string, error) {
	if s == nil || s.root == "" {
		return tobari.WorkspaceAuthorityCollection{}, "", tobari.ErrMigrationSourceUnsafe
	}
	if err := ctx.Err(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, "", err
	}
	rootInfo, err := os.Lstat(s.root)
	if err != nil || validateOwnedDirectoryInfo(rootInfo) != nil {
		return tobari.WorkspaceAuthorityCollection{}, "", tobari.ErrMigrationSourceUnsafe
	}
	entries, err := os.ReadDir(s.root)
	if err != nil || len(entries) != 1 || entries[0].Name() != legacyAuthorityFileName {
		return tobari.WorkspaceAuthorityCollection{}, "", tobari.ErrMigrationNotSupported
	}
	data, err := readAuthorityFile(filepath.Join(s.root, legacyAuthorityFileName))
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, "", errors.Join(tobari.ErrMigrationSourceUnsafe, err)
	}
	if err := rejectLegacyAdvancedAuthorityBytes(data); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, "", errors.Join(tobari.ErrMigrationNotSupported, err)
	}
	var collection tobari.WorkspaceAuthorityCollection
	if err := decodeStrictJSON(data, &collection); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, "", errors.Join(tobari.ErrMigrationNotSupported, err)
	}
	if err := collection.Validate(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, "", errors.Join(tobari.ErrMigrationNotSupported, err)
	}
	return collection, digestBytes(data), nil
}

func materializePreparedGeneration(root string, prepared preparedAuthorityGeneration) error {
	if err := os.Mkdir(root, 0o700); err != nil {
		return err
	}
	for _, directory := range authorityConceptDirectories {
		if err := ensureAuthorityDirectory(filepath.Join(root, directory)); err != nil {
			return err
		}
	}
	for relative, data := range prepared.objects {
		if err := writeImmutableAuthorityFile(filepath.Join(root, filepath.FromSlash(relative)), data); err != nil {
			return err
		}
	}
	component, _ := digestFileComponent(prepared.manifestDigest)
	if err := writeImmutableAuthorityFile(filepath.Join(root, "generations", component+".json"), prepared.manifestData); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, activeFileName), prepared.pointerData, 0o600)
}

func removeMigrationTree(path, authorityRoot string) error {
	if filepath.Dir(path) != filepath.Dir(authorityRoot) || (path != authorityRoot+".migration-stage" && path != authorityRoot+".migration-old") {
		return fmt.Errorf("migration cleanup target is outside the reserved boundary")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !ownedByCurrentUser(info) {
		return fmt.Errorf("migration cleanup target is unsafe")
	}
	return os.RemoveAll(path)
}
