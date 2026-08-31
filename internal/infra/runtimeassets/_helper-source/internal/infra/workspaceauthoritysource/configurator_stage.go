package workspaceauthoritysource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const configuratorStageJournalSchema = 1

type configuratorPublicationJournal struct {
	SchemaVersion int                           `json:"schema_version"`
	Submission    tobari.ConfiguratorSubmission `json:"submission"`
}

// AcquireConfiguratorProjectStageLease serializes discovery, reservation, and
// settlement of the single pending Configurator submission owned by a Project.
// Callers acquire this lease before any Template-scoped stage lease.
func (s *Store) AcquireConfiguratorProjectStageLease(ctx context.Context, projectRoot string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := tobari.ValidateCanonicalRoot(projectRoot); err != nil {
		return nil, err
	}
	return s.AcquireConfiguratorStageScopeLease(ctx, "project-"+projectRoot)
}

// AcquireConfiguratorStageScopeLease serializes one location-free task owner,
// such as a Context-bound policy edit. The scope is derived from validated
// task authority and never from ambient CWD.
func (s *Store) AcquireConfiguratorStageScopeLease(ctx context.Context, scope string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if scope == "" || len(scope) > 4096 || strings.ContainsAny(scope, "\r\n") {
		return nil, fmt.Errorf("Configurator stage scope is invalid")
	}
	root := filepath.Join(s.configRoot, ".configurator-stage-scope-locks")
	if err := ensurePrivateDirectoryTree(s.configRoot); err != nil {
		return nil, err
	}
	if err := ensurePrivateDirectory(root); err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(scope))
	file, err := os.OpenFile(filepath.Join(root, fmt.Sprintf("%x.lock", digest)), os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- the filename is a fixed-size digest below a private root.
	if err != nil {
		return nil, err
	}
	locked, lockErr := tryLockConfiguratorStageFile(file)
	if lockErr != nil || !locked {
		_ = file.Close()
		return nil, errors.Join(tobari.ErrContextBindingProtected, lockErr)
	}
	return func() error {
		unlockConfiguratorStageFile(file)
		return file.Close()
	}, nil
}

// AcquireConfiguratorCatalogLease serializes publication-barrier changes with
// catalog-wide default selection, whose target set cannot be known in advance.
func (s *Store) AcquireConfiguratorCatalogLease(ctx context.Context) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root := filepath.Join(s.configRoot, ".configurator-catalog-locks")
	if err := ensurePrivateDirectoryTree(s.configRoot); err != nil {
		return nil, err
	}
	if err := ensurePrivateDirectory(root); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(root, "catalog.lock"), os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- fixed lock filename below the private Configurator catalog-lock root.
	if err != nil {
		return nil, err
	}
	locked, lockErr := tryLockConfiguratorStageFile(file)
	if lockErr != nil || !locked {
		_ = file.Close()
		return nil, errors.Join(tobari.ErrContextBindingProtected, lockErr)
	}
	return func() error {
		unlockConfiguratorStageFile(file)
		return file.Close()
	}, nil
}

func (s *Store) AcquireConfiguratorStageLease(ctx context.Context, id tobari.WorkspaceTemplateID) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := id.Validate(); err != nil {
		return nil, err
	}
	root := filepath.Join(s.configRoot, ".configurator-stage-locks")
	if err := ensurePrivateDirectoryTree(s.configRoot); err != nil {
		return nil, err
	}
	if err := ensurePrivateDirectory(root); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(root, string(id)+".lock"), os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- validated Template ID below a private root.
	if err != nil {
		return nil, err
	}
	locked, lockErr := tryLockConfiguratorStageFile(file)
	if lockErr != nil || !locked {
		_ = file.Close()
		return nil, errors.Join(tobari.ErrContextBindingProtected, lockErr)
	}
	return func() error {
		unlockConfiguratorStageFile(file)
		return file.Close()
	}, nil
}

type configuratorStageJournal struct {
	SchemaVersion   int                           `json:"schema_version"`
	Submission      tobari.ConfiguratorSubmission `json:"submission"`
	Stage           tobari.ConfiguratorStage      `json:"stage"`
	BaseFingerprint string                        `json:"base_fingerprint"`
	PlanRef         string                        `json:"plan_ref,omitempty"`
	ApplyConfirmed  bool                          `json:"apply_confirmed"`
}

// ConfiguratorStageReceipt is the durable pre-Apply receipt for one exact
// source replacement. It is infrastructure-owned and never exposed as a
// public command value.
type ConfiguratorStageReceipt struct {
	Submission      tobari.ConfiguratorSubmission
	Stage           tobari.ConfiguratorStage
	BaseFingerprint string
	PlanRef         string
	ApplyConfirmed  bool
}

func (s *Store) BeginConfiguratorPublication(ctx context.Context, submission tobari.ConfiguratorSubmission) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := submission.Validate(); err != nil || !submission.Draft.NeedsHomeAdoption() {
		return fmt.Errorf("Configurator publication barrier is invalid: %w", err)
	}
	current, present, err := s.ReadConfiguratorPublication(ctx, submission.Draft.TemplateID)
	if err != nil {
		return err
	}
	if present {
		if reflect.DeepEqual(current, submission) {
			return nil
		}
		return tobari.ErrResourceSourceRecoveryRequired
	}
	path := s.configuratorPublicationPath(submission.Draft.TemplateID)
	if err := ensureDurableConfiguratorDirectoryTree(s.configRoot, s.sync); err != nil {
		return err
	}
	if err := ensureDurableConfiguratorDirectoryTree(filepath.Dir(path), s.sync); err != nil {
		return err
	}
	data, err := json.Marshal(configuratorPublicationJournal{SchemaVersion: 1, Submission: submission})
	if err != nil {
		return err
	}
	if err := writeAtomicConfiguratorReceipt(path, append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) ReadConfiguratorPublication(ctx context.Context, id tobari.WorkspaceTemplateID) (tobari.ConfiguratorSubmission, bool, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ConfiguratorSubmission{}, false, err
	}
	if err := id.Validate(); err != nil {
		return tobari.ConfiguratorSubmission{}, false, err
	}
	path := s.configuratorPublicationPath(id)
	data, err := os.ReadFile(path) // #nosec G304 -- validated Template ID below the private source root.
	if errors.Is(err, os.ErrNotExist) {
		return tobari.ConfiguratorSubmission{}, false, nil
	}
	if err != nil || len(data) == 0 || len(data) > maxSourceBytes {
		return tobari.ConfiguratorSubmission{}, false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	info, err := os.Lstat(path)
	if err != nil || validatePrivateFile(info) != nil {
		return tobari.ConfiguratorSubmission{}, false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if parent, parentErr := os.Lstat(filepath.Dir(path)); parentErr != nil || validatePrivateDirectory(parent) != nil {
		return tobari.ConfiguratorSubmission{}, false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, parentErr)
	}
	var journal configuratorPublicationJournal
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil || journal.SchemaVersion != 1 || journal.Submission.Validate() != nil || !journal.Submission.Draft.NeedsHomeAdoption() || journal.Submission.Draft.TemplateID != id {
		return tobari.ConfiguratorSubmission{}, false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return tobari.ConfiguratorSubmission{}, false, tobari.ErrResourceSourceRecoveryRequired
	}
	return journal.Submission, true, nil
}

func (s *Store) ListConfiguratorPublicationIDs(ctx context.Context) ([]tobari.WorkspaceTemplateID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.configRoot, ".configurator-publications")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []tobari.WorkspaceTemplateID{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]tobari.WorkspaceTemplateID, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			if entry.IsDir() || !strings.HasPrefix(entry.Name(), ".configurator-receipt-") || !strings.HasSuffix(entry.Name(), ".tmp") {
				return nil, tobari.ErrResourceSourceRecoveryRequired
			}
			path := filepath.Join(dir, entry.Name())
			info, infoErr := os.Lstat(path)
			if infoErr != nil || validatePrivateFile(info) != nil {
				return nil, errors.Join(tobari.ErrResourceSourceRecoveryRequired, infoErr)
			}
			if err := os.Remove(path); err != nil {
				return nil, err
			}
			continue
		}
		if entry.IsDir() {
			return nil, tobari.ErrResourceSourceRecoveryRequired
		}
		id, parseErr := tobari.ParseWorkspaceTemplateID(strings.TrimSuffix(entry.Name(), ".json"))
		if parseErr != nil {
			return nil, tobari.ErrResourceSourceRecoveryRequired
		}
		result = append(result, id)
	}
	if err := syncDirectory(dir); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func (s *Store) CompleteConfiguratorPublication(ctx context.Context, submission tobari.ConfiguratorSubmission) error {
	current, present, err := s.ReadConfiguratorPublication(ctx, submission.Draft.TemplateID)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	if !reflect.DeepEqual(current, submission) {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	path := s.configuratorPublicationPath(submission.Draft.TemplateID)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func (s *Store) ListConfiguratorStageIDs(ctx context.Context) ([]tobari.WorkspaceTemplateID, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := filepath.Join(s.configRoot, ".configurator-stages")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []tobari.WorkspaceTemplateID{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]tobari.WorkspaceTemplateID, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, tobari.ErrResourceSourceRecoveryRequired
		}
		id, parseErr := tobari.ParseWorkspaceTemplateID(strings.TrimSuffix(entry.Name(), ".json"))
		if parseErr != nil {
			return nil, tobari.ErrResourceSourceRecoveryRequired
		}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, nil
}

func (r ConfiguratorStageReceipt) validate() error {
	if r.Submission.Validate() != nil || r.Stage.ValidateFor(r.Submission) != nil || !validFingerprint(r.BaseFingerprint) {
		return fmt.Errorf("Configurator stage receipt is invalid")
	}
	return (tobari.ConfiguratorPendingStage{Submission: r.Submission, Stage: r.Stage, PlanRef: r.PlanRef, ApplyConfirmed: r.ApplyConfirmed}).Validate()
}

func (s *Store) ConfiguratorTemplateFingerprint(source tobari.WorkspaceTemplateSource) (string, error) {
	if err := source.Validate(); err != nil {
		return "", err
	}
	templateData, err := encodeCanonicalYAML(source.Template)
	if err != nil {
		return "", err
	}
	policyData, err := encodeCanonicalYAML(source.Policy)
	if err != nil {
		return "", err
	}
	return sourceFingerprint(templateData, policyData), nil
}

func (s *Store) BeginConfiguratorStage(ctx context.Context, receipt ConfiguratorStageReceipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := receipt.validate(); err != nil {
		return err
	}
	path := s.configuratorStagePath(receipt.Submission.Draft.TemplateID)
	if current, present, err := s.ReadConfiguratorStage(ctx, receipt.Submission.Draft.TemplateID); err != nil {
		return err
	} else if present {
		if reflect.DeepEqual(current, receipt) {
			return nil
		}
		return tobari.ErrResourceSourceRecoveryRequired
	}
	dir := filepath.Dir(path)
	if err := ensurePrivateDirectoryTree(s.configRoot); err != nil {
		return err
	}
	if err := ensurePrivateDirectory(dir); err != nil {
		return err
	}
	if err := s.writeConfiguratorStageJournal(receipt); err != nil {
		return err
	}
	return nil
}

func (s *Store) ReadConfiguratorStage(ctx context.Context, id tobari.WorkspaceTemplateID) (ConfiguratorStageReceipt, bool, error) {
	if err := ctx.Err(); err != nil {
		return ConfiguratorStageReceipt{}, false, err
	}
	path := s.configuratorStagePath(id)
	data, err := os.ReadFile(path) // #nosec G304 -- validated Template ID under the private source root.
	if errors.Is(err, os.ErrNotExist) {
		return ConfiguratorStageReceipt{}, false, nil
	}
	if err != nil {
		return ConfiguratorStageReceipt{}, false, err
	}
	if len(data) == 0 || len(data) > maxSourceBytes {
		return ConfiguratorStageReceipt{}, false, tobari.ErrResourceSourceRecoveryRequired
	}
	info, err := os.Lstat(path)
	if err != nil || validatePrivateFile(info) != nil {
		return ConfiguratorStageReceipt{}, false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if parent, parentErr := os.Lstat(filepath.Dir(path)); parentErr != nil || validatePrivateDirectory(parent) != nil {
		return ConfiguratorStageReceipt{}, false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, parentErr)
	}
	var journal configuratorStageJournal
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil || journal.SchemaVersion != configuratorStageJournalSchema {
		return ConfiguratorStageReceipt{}, false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ConfiguratorStageReceipt{}, false, tobari.ErrResourceSourceRecoveryRequired
	}
	receipt := ConfiguratorStageReceipt{Submission: journal.Submission, Stage: journal.Stage, BaseFingerprint: journal.BaseFingerprint, PlanRef: journal.PlanRef, ApplyConfirmed: journal.ApplyConfirmed}
	if err := receipt.validate(); err != nil || receipt.Submission.Draft.TemplateID != id {
		return ConfiguratorStageReceipt{}, false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	return receipt, true, nil
}

func (s *Store) ClearConfiguratorStage(ctx context.Context, receipt ConfiguratorStageReceipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := receipt.validate(); err != nil {
		return err
	}
	current, present, err := s.ReadConfiguratorStage(ctx, receipt.Submission.Draft.TemplateID)
	if err != nil || !present {
		return err
	}
	if !reflect.DeepEqual(current, receipt) {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	path := s.configuratorStagePath(receipt.Submission.Draft.TemplateID)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func (s *Store) BindConfiguratorStagePlan(ctx context.Context, receipt ConfiguratorStageReceipt, planRef string) (ConfiguratorStageReceipt, error) {
	if err := ctx.Err(); err != nil {
		return ConfiguratorStageReceipt{}, err
	}
	current, present, err := s.ReadConfiguratorStage(ctx, receipt.Submission.Draft.TemplateID)
	if err != nil || !present {
		return ConfiguratorStageReceipt{}, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if !sameConfiguratorStageIdentity(current, receipt) || current.ApplyConfirmed || current.PlanRef != "" && current.PlanRef != planRef {
		return ConfiguratorStageReceipt{}, tobari.ErrResourceSourceRecoveryRequired
	}
	current.PlanRef = planRef
	if err := current.validate(); err != nil {
		return ConfiguratorStageReceipt{}, err
	}
	if err := s.writeConfiguratorStageJournal(current); err != nil {
		return ConfiguratorStageReceipt{}, err
	}
	return current, nil
}

func (s *Store) ConfirmConfiguratorStageApply(ctx context.Context, receipt ConfiguratorStageReceipt) (ConfiguratorStageReceipt, error) {
	if err := ctx.Err(); err != nil {
		return ConfiguratorStageReceipt{}, err
	}
	current, present, err := s.ReadConfiguratorStage(ctx, receipt.Submission.Draft.TemplateID)
	if err != nil || !present {
		return ConfiguratorStageReceipt{}, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if !reflect.DeepEqual(current, receipt) || current.PlanRef == "" {
		return ConfiguratorStageReceipt{}, tobari.ErrResourceSourceRecoveryRequired
	}
	current.ApplyConfirmed = true
	if err := current.validate(); err != nil {
		return ConfiguratorStageReceipt{}, err
	}
	if err := s.writeConfiguratorStageJournal(current); err != nil {
		return ConfiguratorStageReceipt{}, err
	}
	return current, nil
}

func sameConfiguratorStageIdentity(left, right ConfiguratorStageReceipt) bool {
	return reflect.DeepEqual(left.Submission, right.Submission) && left.Stage == right.Stage && left.BaseFingerprint == right.BaseFingerprint
}

func (s *Store) writeConfiguratorStageJournal(receipt ConfiguratorStageReceipt) error {
	value := configuratorStageJournal{SchemaVersion: configuratorStageJournalSchema, Submission: receipt.Submission, Stage: receipt.Stage, BaseFingerprint: receipt.BaseFingerprint, PlanRef: receipt.PlanRef, ApplyConfirmed: receipt.ApplyConfirmed}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	path := s.configuratorStagePath(receipt.Submission.Draft.TemplateID)
	dir := filepath.Dir(path)
	tempDir := filepath.Join(s.configRoot, ".configurator-stage-tmp")
	if err := ensureDurableConfiguratorDirectoryTree(s.configRoot, s.sync); err != nil {
		return err
	}
	if err := ensureDurableConfiguratorDirectoryTree(dir, s.sync); err != nil {
		return err
	}
	if err := ensureDurableConfiguratorDirectoryTree(tempDir, s.sync); err != nil {
		return err
	}
	if err := cleanupConfiguratorStageTemps(tempDir, receipt.Submission.Draft.TemplateID); err != nil {
		return err
	}
	temp, err := os.CreateTemp(tempDir, string(receipt.Submission.Draft.TemplateID)+"-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		return errors.Join(err, temp.Close())
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		return errors.Join(err, temp.Close())
	}
	if err := temp.Sync(); err != nil {
		return errors.Join(err, temp.Close())
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return errors.Join(syncDirectory(dir), syncDirectory(tempDir))
}

// ensureDurableConfiguratorDirectoryTree persists every newly-created
// directory entry from the exact configured root down. Receipt files are not
// recovery authority if a power loss can forget their owning directory after
// the write reports success.
func ensureDurableConfiguratorDirectoryTree(path string, syncFn func(string) error) error {
	if syncFn == nil {
		return fmt.Errorf("Configurator directory durability authority is unavailable")
	}
	missing := []string{}
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if current == path {
				if err := validatePrivateDirectory(info); err != nil {
					return err
				}
				return syncFn(filepath.Dir(current))
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("Configurator directory ancestor must be a real directory")
			}
			if err := syncFn(filepath.Dir(current)); err != nil {
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
			return fmt.Errorf("Configurator directory has no durable existing ancestor")
		}
	}
	for index := len(missing) - 1; index >= 0; index-- {
		created := missing[index]
		if err := os.Mkdir(created, 0o700); err != nil {
			return err
		}
		info, err := os.Lstat(created)
		if err != nil {
			return err
		}
		if err := validatePrivateDirectory(info); err != nil {
			return err
		}
		if err := syncFn(filepath.Dir(created)); err != nil {
			return err
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	return validatePrivateDirectory(info)
}

func cleanupConfiguratorStageTemps(dir string, id tobari.WorkspaceTemplateID) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	prefix := string(id) + "-"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".tmp") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, infoErr := os.Lstat(path)
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return tobari.ErrResourceSourceRecoveryRequired
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return syncDirectory(dir)
}

func writeAtomicConfiguratorReceipt(path string, data []byte) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".configurator-receipt-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		return errors.Join(err, temp.Close())
	}
	if _, err := temp.Write(data); err != nil {
		return errors.Join(err, temp.Close())
	}
	if err := temp.Sync(); err != nil {
		return errors.Join(err, temp.Close())
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func (s *Store) SettleConfiguratorStage(ctx context.Context, id tobari.WorkspaceTemplateID, selectedFingerprint string) error {
	receipt, present, err := s.ReadConfiguratorStage(ctx, id)
	if err != nil || !present {
		return err
	}
	if receipt.Stage.SourceFingerprint != selectedFingerprint {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	return s.ClearConfiguratorStage(ctx, receipt)
}

func (s *Store) configuratorStagePath(id tobari.WorkspaceTemplateID) string {
	return filepath.Join(s.configRoot, ".configurator-stages", string(id)+".json")
}

func (s *Store) configuratorPublicationPath(id tobari.WorkspaceTemplateID) string {
	return filepath.Join(s.configRoot, ".configurator-publications", string(id)+".json")
}
