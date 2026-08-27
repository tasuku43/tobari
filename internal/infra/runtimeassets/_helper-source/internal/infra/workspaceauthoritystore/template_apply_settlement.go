package workspaceauthoritystore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type templateApplySettlement struct {
	SchemaVersion         int                                         `json:"schema_version"`
	PlanRef               string                                      `json:"plan_ref"`
	SourceFingerprint     string                                      `json:"source_fingerprint"`
	PostFingerprint       string                                      `json:"post_source_fingerprint,omitempty"`
	ReconciledFingerprint string                                      `json:"reconciled_source_fingerprint,omitempty"`
	Publication           tobari.WorkspaceTemplateRevisionPublication `json:"publication"`
}

func (m *Mutator) templateApplySettlementPath() string {
	return filepath.Join(m.store.root, "journal", "template-apply-settlement.json")
}

func (m *Mutator) writeTemplateApplySettlement(planRef, fingerprint string, publication tobari.WorkspaceTemplateRevisionPublication) error {
	if _, err := tobari.ParseWorkspaceTemplateChangePlanRef(planRef); err != nil || fingerprint == "" || publication.Template.Validate() != nil {
		return fmt.Errorf("Template Apply settlement is invalid")
	}
	data, _, err := encodeAuthorityObject(templateApplySettlement{SchemaVersion: 1, PlanRef: planRef, SourceFingerprint: fingerprint, Publication: publication})
	if err != nil {
		return err
	}
	path := m.templateApplySettlementPath()
	if err := ensureAuthorityDirectory(m.store.root); err != nil {
		return err
	}
	if err := ensureAuthorityDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return finalMutationRecoveryError("another Template Apply settlement is pending")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := writeMutationFile(path, data); err != nil {
		return err
	}
	if err := m.sync(filepath.Dir(path)); err != nil {
		return err
	}
	return m.sync(m.store.root)
}

func (m *Mutator) recoverTemplateApplySettlement(ctx context.Context, planRef string, load WorkspaceTemplateSourceLoader) (tobari.WorkspaceTemplateRevisionPublication, bool, error) {
	path := m.templateApplySettlementPath()
	data, err := readAuthorityFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return tobari.WorkspaceTemplateRevisionPublication{}, false, nil
	}
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, false, err
	}
	var settlement templateApplySettlement
	if err := decodeStrictJSON(data, &settlement); err != nil || settlement.SchemaVersion != 1 || settlement.PlanRef != planRef || settlement.SourceFingerprint == "" || settlement.Publication.Template.Validate() != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, false, finalMutationRecoveryError("Template Apply settlement does not match the requested plan")
	}
	source, fingerprint, err := load(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, false, err
	}
	if fingerprint != settlement.SourceFingerprint && fingerprint != settlement.PostFingerprint && fingerprint != settlement.ReconciledFingerprint {
		return tobari.WorkspaceTemplateRevisionPublication{}, false, tobari.ErrResourceSourceChanged
	}
	current, present, err := m.store.ReadComplete(ctx)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, false, err
	}
	found := false
	for _, template := range current.Templates {
		if template.ID != source.Template.TemplateID {
			continue
		}
		found = true
		if reflect.DeepEqual(template, settlement.Publication.Template) {
			return settlement.Publication, true, nil
		}
		if present && template.Current.Revision == settlement.Publication.Previous.Revision {
			if err := m.CompleteWorkspaceTemplateApplySettlement(planRef); err != nil {
				return tobari.WorkspaceTemplateRevisionPublication{}, false, err
			}
			return tobari.WorkspaceTemplateRevisionPublication{}, false, nil
		}
	}
	if !found {
		if err := m.CompleteWorkspaceTemplateApplySettlement(planRef); err != nil {
			return tobari.WorkspaceTemplateRevisionPublication{}, false, err
		}
		return tobari.WorkspaceTemplateRevisionPublication{}, false, nil
	}
	return tobari.WorkspaceTemplateRevisionPublication{}, false, finalMutationRecoveryError("Template Apply active authority is neither the planned previous nor published result")
}

// RecordWorkspaceTemplateApplyPostFingerprint binds the byte-exact source
// produced by bookkeeping before the settlement can be removed. A crash
// before this update is recoverable from the pre-fingerprint; a crash after it
// recognizes either exact side of the CAS transition.
func (m *Mutator) RecordWorkspaceTemplateApplyPostFingerprint(planRef, fingerprint string) error {
	if fingerprint == "" {
		return finalMutationRecoveryError("Template Apply post-source fingerprint is absent")
	}
	path := m.templateApplySettlementPath()
	data, err := readAuthorityFile(path)
	if err != nil {
		return err
	}
	var settlement templateApplySettlement
	if err := decodeStrictJSON(data, &settlement); err != nil || settlement.SchemaVersion != 1 || settlement.PlanRef != planRef {
		return finalMutationRecoveryError("Template Apply settlement does not match post-source publication")
	}
	if settlement.PostFingerprint != "" && settlement.PostFingerprint != fingerprint {
		if settlement.ReconciledFingerprint != "" && settlement.ReconciledFingerprint != fingerprint {
			return finalMutationRecoveryError("Template Apply reconciled source fingerprint changed")
		}
		settlement.ReconciledFingerprint = fingerprint
	} else {
		settlement.PostFingerprint = fingerprint
	}
	encoded, _, err := encodeAuthorityObject(settlement)
	if err != nil {
		return err
	}
	if err := writeMutationFile(path, encoded); err != nil {
		return err
	}
	return m.sync(filepath.Dir(path))
}

func (m *Mutator) CompleteWorkspaceTemplateApplySettlement(planRef string) error {
	path := m.templateApplySettlementPath()
	data, err := readAuthorityFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var settlement templateApplySettlement
	if err := decodeStrictJSON(data, &settlement); err != nil || settlement.SchemaVersion != 1 || settlement.PlanRef != planRef {
		return finalMutationRecoveryError("Template Apply settlement does not match completion")
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	return m.sync(filepath.Dir(path))
}
