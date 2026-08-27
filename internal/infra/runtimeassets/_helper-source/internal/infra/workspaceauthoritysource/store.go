package workspaceauthoritysource

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
	"strconv"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"gopkg.in/yaml.v3"
)

const (
	templateFileName = "template.yaml"
	policyFileName   = "policy.yaml"
	contextFileName  = "context.yaml"
	maxSourceBytes   = 4 << 20
)

// Store owns the user-editable, concept-separated Template and Context source
// trees. It does not read or write active authority or Policy Memory.
type Store struct {
	configRoot string
	phase      func(string) error
	removeAll  func(string) error
	sync       func(string) error
}

// SetInstallationMigrationBoundaryForTest installs a process-death boundary
// used only by the composed storage crash matrix.
func (s *Store) SetInstallationMigrationBoundaryForTest(boundary func(string) error) {
	if s != nil {
		s.phase = boundary
	}
}

const templateBaseRepairSchema = 1

type templateBaseRepairJournal struct {
	SchemaVersion   int    `json:"schema_version"`
	Phase           string `json:"phase"`
	Expected        string `json:"expected_fingerprint"`
	OldFingerprint  string `json:"old_fingerprint"`
	PostFingerprint string `json:"post_fingerprint"`
	Revision        string `json:"revision"`
}

func New(configRoot string) (*Store, error) {
	if configRoot == "" || !filepath.IsAbs(configRoot) || filepath.Clean(configRoot) != configRoot || configRoot == string(filepath.Separator) {
		return nil, fmt.Errorf("resource source root must be an exact absolute child path")
	}
	return &Store{configRoot: configRoot, phase: func(string) error { return nil }, removeAll: os.RemoveAll, sync: syncDirectory}, nil
}

func (s *Store) TemplatePath(id tobari.WorkspaceTemplateID) (string, error) {
	if s == nil {
		return "", fmt.Errorf("resource source store is unavailable")
	}
	if err := id.Validate(); err != nil {
		return "", err
	}
	return filepath.Join(s.configRoot, "templates", string(id), templateFileName), nil
}

func (s *Store) ContextPath(id tobari.ContextID) (string, error) {
	if s == nil {
		return "", fmt.Errorf("resource source store is unavailable")
	}
	if err := id.Validate(); err != nil {
		return "", err
	}
	return filepath.Join(s.configRoot, "contexts", string(id), contextFileName), nil
}

func (s *Store) ListTemplateIDs(ctx context.Context) ([]tobari.WorkspaceTemplateID, error) {
	if s == nil {
		return nil, fmt.Errorf("resource source store is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	concept := filepath.Join(s.configRoot, "templates")
	info, err := os.Lstat(concept)
	if errors.Is(err, os.ErrNotExist) {
		return []tobari.WorkspaceTemplateID{}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := validatePrivateDirectory(info); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(concept)
	if err != nil {
		return nil, err
	}
	ids := make([]tobari.WorkspaceTemplateID, 0, len(entries))
	for _, entry := range entries {
		id, parseErr := tobari.ParseWorkspaceTemplateID(entry.Name())
		childInfo, statErr := os.Lstat(filepath.Join(concept, entry.Name()))
		if parseErr != nil || statErr != nil || validatePrivateDirectory(childInfo) != nil {
			return nil, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("Template source concept contains an unsafe resource directory"))
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (s *Store) ReadTemplate(ctx context.Context, id tobari.WorkspaceTemplateID) (tobari.WorkspaceTemplateSource, bool, error) {
	source, _, present, err := s.ReadTemplateSnapshot(ctx, id)
	return source, present, err
}

func (s *Store) ReadTemplateSnapshot(ctx context.Context, id tobari.WorkspaceTemplateID) (tobari.WorkspaceTemplateSource, string, bool, error) {
	path, err := s.TemplatePath(id)
	if err != nil {
		return tobari.WorkspaceTemplateSource{}, "", false, err
	}
	templateData, present, err := s.read(ctx, path, templateFileName, templateFileName, policyFileName)
	if err != nil || !present {
		return tobari.WorkspaceTemplateSource{}, "", present, err
	}
	policyPath := filepath.Join(filepath.Dir(path), policyFileName)
	policyData, policyPresent, err := s.read(ctx, policyPath, policyFileName, templateFileName, policyFileName)
	if err != nil || !policyPresent {
		return tobari.WorkspaceTemplateSource{}, "", policyPresent, err
	}
	var source tobari.WorkspaceTemplateSource
	if err := decodeStrictYAML(templateData, &source.Template); err != nil {
		return tobari.WorkspaceTemplateSource{}, "", true, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("decode Template source: %w", err))
	}
	if err := validateFinalPolicySourceTopology(policyData); err != nil {
		return tobari.WorkspaceTemplateSource{}, "", true, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("decode Template policy source: %w", err))
	}
	if err := decodeStrictYAML(policyData, &source.Policy); err != nil {
		return tobari.WorkspaceTemplateSource{}, "", true, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("decode Template policy source: %w", err))
	}
	if err := source.Validate(); err != nil {
		return tobari.WorkspaceTemplateSource{}, "", true, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("validate Template source: %w", err))
	}
	if source.Template.TemplateID != id || source.Policy.TemplateID != id {
		return tobari.WorkspaceTemplateSource{}, "", true, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("Template source identity does not match its directory"))
	}
	secondTemplate, _, err := s.read(ctx, path, templateFileName, templateFileName, policyFileName)
	if err != nil || !bytes.Equal(templateData, secondTemplate) {
		return tobari.WorkspaceTemplateSource{}, "", true, fmt.Errorf("Template source pair changed during observation")
	}
	secondPolicy, _, err := s.read(ctx, policyPath, policyFileName, templateFileName, policyFileName)
	if err != nil || !bytes.Equal(policyData, secondPolicy) {
		return tobari.WorkspaceTemplateSource{}, "", true, fmt.Errorf("Template source pair changed during observation")
	}
	return source, sourceFingerprint(templateData, policyData), true, nil
}

func validateFinalPolicySourceTopology(data []byte) error {
	var root map[string]any
	if err := decodeStrictYAML(data, &root); err != nil {
		return err
	}
	requireMap := func(parent map[string]any, key string) (map[string]any, error) {
		value, present := parent[key]
		if !present || value == nil {
			return nil, fmt.Errorf("%s must be explicit", key)
		}
		mapping, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be a mapping", key)
		}
		return mapping, nil
	}
	boundary, err := requireMap(root, "boundary")
	if err != nil {
		return err
	}
	methods, err := requireMap(boundary, "methods")
	if err != nil {
		return err
	}
	if deny, present := methods["deny"]; !present || deny == nil {
		return fmt.Errorf("boundary.methods.deny must be explicit")
	} else if _, ok := deny.([]any); !ok {
		return fmt.Errorf("boundary.methods.deny must be a sequence")
	}
	semantic, err := requireMap(root, "semantic")
	if err != nil {
		return err
	}
	protocols, err := requireMap(semantic, "protocols")
	if err != nil {
		return err
	}
	http, err := requireMap(protocols, "http")
	if err != nil {
		return err
	}
	providers, err := requireMap(semantic, "providers")
	if err != nil {
		return err
	}
	validateModule := func(parent map[string]any, name string, endpoints bool) error {
		value, present := parent[name]
		if !present {
			return nil
		}
		module, ok := value.(map[string]any)
		if !ok || module == nil {
			return fmt.Errorf("semantic module %s must be a mapping", name)
		}
		if endpoints {
			if value, present := module["endpoints"]; !present || value == nil {
				return fmt.Errorf("semantic module %s endpoints must be explicit", name)
			} else if _, ok := value.([]any); !ok {
				return fmt.Errorf("semantic module %s endpoints must be a sequence", name)
			}
		}
		for _, effect := range []string{"allow", "deny"} {
			set, err := requireMap(module, effect)
			if err != nil {
				return fmt.Errorf("semantic module %s: %w", name, err)
			}
			if rules, present := set["rules"]; !present || rules == nil {
				return fmt.Errorf("semantic module %s %s.rules must be explicit", name, effect)
			} else if _, ok := rules.([]any); !ok {
				return fmt.Errorf("semantic module %s %s.rules must be a sequence", name, effect)
			}
		}
		return nil
	}
	for _, module := range []struct {
		name      string
		endpoints bool
	}{{"generic", false}, {"graphql", true}, {"mcp", true}, {"git", false}, {"oci", false}} {
		if err := validateModule(http, module.name, module.endpoints); err != nil {
			return err
		}
	}
	for _, name := range []string{"aws", "kubernetes"} {
		if err := validateModule(providers, name, false); err != nil {
			return err
		}
	}
	return nil
}

// ReadTemplatePolicyMigrationSnapshot is the sole alpha decoder. It is used
// only by the explicit non-activating migration Plan/Apply path; ordinary
// source reads remain V1-only.
func (s *Store) ReadTemplatePolicyMigrationSnapshot(ctx context.Context, id tobari.WorkspaceTemplateID, resolved tobari.RuntimeBinding) (tobari.WorkspaceTemplateAlphaSource, tobari.WorkspaceTemplateSource, string, string, bool, error) {
	templateData, policyData, present, err := s.readTemplatePairBytes(ctx, id)
	if err != nil || !present {
		return tobari.WorkspaceTemplateAlphaSource{}, tobari.WorkspaceTemplateSource{}, "", "", present, err
	}
	var alpha tobari.WorkspaceTemplateAlphaSource
	if err := decodeStrictYAML(templateData, &alpha.Template); err != nil {
		return tobari.WorkspaceTemplateAlphaSource{}, tobari.WorkspaceTemplateSource{}, "", "", true, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("decode Template source: %w", err))
	}
	if err := decodeStrictYAML(policyData, &alpha.Policy); err != nil {
		return tobari.WorkspaceTemplateAlphaSource{}, tobari.WorkspaceTemplateSource{}, "", "", true, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("decode alpha Template policy source: %w", err))
	}
	if err := alpha.Validate(); err != nil || alpha.Template.TemplateID != id {
		return tobari.WorkspaceTemplateAlphaSource{}, tobari.WorkspaceTemplateSource{}, "", "", true, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("validate alpha Template source: %w", err))
	}
	migrated, err := alpha.MigrateToV1(resolved)
	if err != nil {
		return tobari.WorkspaceTemplateAlphaSource{}, tobari.WorkspaceTemplateSource{}, "", "", true, errors.Join(tobari.ErrResourceSourceInvalid, err)
	}
	targetPolicy, err := encodeCanonicalYAML(migrated.Policy)
	if err != nil {
		return tobari.WorkspaceTemplateAlphaSource{}, tobari.WorkspaceTemplateSource{}, "", "", true, err
	}
	return alpha, migrated, sourceFingerprint(templateData, policyData), sourceFingerprint(templateData, targetPolicy), true, nil
}

func (s *Store) readTemplatePairBytes(ctx context.Context, id tobari.WorkspaceTemplateID) ([]byte, []byte, bool, error) {
	path, err := s.TemplatePath(id)
	if err != nil {
		return nil, nil, false, err
	}
	templateData, present, err := s.read(ctx, path, templateFileName, templateFileName, policyFileName)
	if err != nil || !present {
		return nil, nil, present, err
	}
	policyData, policyPresent, err := s.read(ctx, filepath.Join(filepath.Dir(path), policyFileName), policyFileName, templateFileName, policyFileName)
	if err != nil || !policyPresent {
		return nil, nil, policyPresent, err
	}
	return templateData, policyData, true, nil
}

// ApplyTemplatePolicyMigration replaces only the desired source directory.
// Active authority is never read or written here. The caller binds the exact
// active revision through the content-addressed plan.
func (s *Store) ApplyTemplatePolicyMigration(ctx context.Context, plan tobari.WorkspaceTemplatePolicyMigrationPlan, resolved tobari.RuntimeBinding) (string, bool, error) {
	if err := plan.Validate(); err != nil {
		return "", false, err
	}
	id, _ := tobari.ParseWorkspaceTemplatePolicyMigrationPlanRef(plan.PlanRef)
	path, err := s.TemplatePath(id)
	if err != nil {
		return "", false, err
	}
	dir := filepath.Dir(path)
	parent := filepath.Dir(dir)
	stage := filepath.Join(parent, ".template-policy-migration-"+string(id)+"-new")
	snapshot := filepath.Join(parent, ".template-policy-migration-"+string(id)+"-snapshot")
	discard := filepath.Join(parent, ".template-policy-migration-"+string(id)+"-old")
	journalPath := filepath.Join(parent, ".template-policy-migration-"+string(id)+"-repair.json")
	if journal, present, readErr := s.readTemplateBaseRepairJournal(journalPath); readErr != nil {
		return "", false, readErr
	} else if present {
		if journal.Expected != plan.SourceFingerprint || journal.PostFingerprint != plan.TargetFingerprint || journal.Revision != string(plan.ActiveRevision) {
			return "", false, tobari.ErrResourceSourceRecoveryRequired
		}
		fingerprint, settleErr := s.settleTemplateBaseRepair(dir, stage, snapshot, discard, journalPath, journal)
		return fingerprint, settleErr == nil, settleErr
	}
	current, currentPresent, err := optionalTemplateDirectoryFingerprint(dir)
	if err != nil || !currentPresent {
		return "", false, errors.Join(tobari.ErrResourceSourceMissing, err)
	}
	if current == plan.TargetFingerprint {
		return current, false, nil
	}
	if current != plan.SourceFingerprint {
		return "", false, tobari.ErrWorkspaceTemplatePolicyMigrationStale
	}
	for _, reserved := range []string{stage, snapshot, discard} {
		if _, statErr := os.Lstat(reserved); statErr == nil {
			if removeErr := s.removeAll(reserved); removeErr != nil {
				return "", false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, removeErr)
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", false, statErr
		}
	}
	if err := s.sync(parent); err != nil {
		return "", false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	templateData, policyData, present, err := s.readTemplatePairBytes(ctx, id)
	if err != nil || !present || sourceFingerprint(templateData, policyData) != plan.SourceFingerprint {
		return "", false, errors.Join(tobari.ErrWorkspaceTemplatePolicyMigrationStale, err)
	}
	var alpha tobari.WorkspaceTemplateAlphaSource
	if decodeStrictYAML(templateData, &alpha.Template) != nil || decodeStrictYAML(policyData, &alpha.Policy) != nil || alpha.Validate() != nil {
		return "", false, tobari.ErrResourceSourceInvalid
	}
	migrated, err := alpha.MigrateToV1(resolved)
	if err != nil {
		return "", false, errors.Join(tobari.ErrResourceSourceInvalid, err)
	}
	targetPolicy, err := encodeCanonicalYAML(migrated.Policy)
	if err != nil || sourceFingerprint(templateData, targetPolicy) != plan.TargetFingerprint {
		return "", false, errors.Join(tobari.ErrWorkspaceTemplatePolicyMigrationStale, err)
	}
	for target, files := range map[string]map[string][]byte{
		stage:    {templateFileName: templateData, policyFileName: targetPolicy},
		snapshot: {templateFileName: templateData, policyFileName: policyData},
	} {
		if err := os.Mkdir(target, 0o700); err != nil {
			return "", false, err
		}
		for _, name := range []string{templateFileName, policyFileName} {
			if err := writeDurableSourceFile(filepath.Join(target, name), files[name]); err != nil {
				return "", false, err
			}
		}
		if err := syncDirectory(target); err != nil {
			return "", false, err
		}
	}
	if err := s.sync(parent); err != nil {
		return "", false, err
	}
	if observed, err := templateDirectoryFingerprint(dir); err != nil || observed != plan.SourceFingerprint {
		return "", false, errors.Join(tobari.ErrWorkspaceTemplatePolicyMigrationStale, err)
	}
	journal := templateBaseRepairJournal{
		SchemaVersion:   templateBaseRepairSchema,
		Phase:           "prepared",
		Expected:        plan.SourceFingerprint,
		OldFingerprint:  plan.SourceFingerprint,
		PostFingerprint: plan.TargetFingerprint,
		Revision:        string(plan.ActiveRevision),
	}
	if err := s.writeTemplateBaseRepairJournal(journalPath, journal); err != nil {
		return "", false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	fingerprint, err := s.settleTemplateBaseRepair(dir, stage, snapshot, discard, journalPath, journal)
	return fingerprint, err == nil, err
}

func (s *Store) ReadContext(ctx context.Context, id tobari.ContextID) (tobari.ContextSource, bool, error) {
	source, _, present, err := s.ReadContextSnapshot(ctx, id)
	return source, present, err
}

func (s *Store) ReadContextSnapshot(ctx context.Context, id tobari.ContextID) (tobari.ContextSource, string, bool, error) {
	path, err := s.ContextPath(id)
	if err != nil {
		return tobari.ContextSource{}, "", false, err
	}
	data, present, err := s.read(ctx, path, contextFileName, contextFileName)
	if err != nil || !present {
		return tobari.ContextSource{}, "", present, err
	}
	var source tobari.ContextSource
	if err := decodeStrictYAML(data, &source); err != nil {
		return tobari.ContextSource{}, "", true, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("decode Context source: %w", err))
	}
	if err := source.Validate(); err != nil {
		return tobari.ContextSource{}, "", true, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("validate Context source: %w", err))
	}
	if source.ContextID != id {
		return tobari.ContextSource{}, "", true, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("Context source identity does not match its directory"))
	}
	digest := sha256.Sum256(data)
	return source, hex.EncodeToString(digest[:]), true, nil
}

func (s *Store) ListContextIDs(ctx context.Context) ([]tobari.ContextID, error) {
	if s == nil {
		return nil, fmt.Errorf("resource source store is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	concept := filepath.Join(s.configRoot, "contexts")
	info, err := os.Lstat(concept)
	if errors.Is(err, os.ErrNotExist) {
		return []tobari.ContextID{}, nil
	}
	if err != nil {
		return nil, err
	}
	if err := validatePrivateDirectory(info); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(concept)
	if err != nil {
		return nil, err
	}
	ids := make([]tobari.ContextID, 0, len(entries))
	for _, entry := range entries {
		id, parseErr := tobari.ParseContextID(entry.Name())
		childInfo, statErr := os.Lstat(filepath.Join(concept, entry.Name()))
		if parseErr != nil || statErr != nil || validatePrivateDirectory(childInfo) != nil {
			return nil, errors.Join(tobari.ErrResourceSourceInvalid, fmt.Errorf("Context source concept contains an unsafe resource directory"))
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (s *Store) PublishTemplate(ctx context.Context, source tobari.WorkspaceTemplateSource) error {
	if err := source.Validate(); err != nil {
		return err
	}
	templateData, err := encodeCanonicalYAML(source.Template)
	if err != nil {
		return err
	}
	policyData, err := encodeCanonicalYAML(source.Policy)
	if err != nil {
		return err
	}
	return s.publishNewResourceDirectory(ctx, "templates", string(source.Template.TemplateID), map[string][]byte{
		templateFileName: templateData,
		policyFileName:   policyData,
	})
}

func (s *Store) PublishContext(ctx context.Context, source tobari.ContextSource) error {
	if err := source.Validate(); err != nil {
		return err
	}
	data, err := encodeCanonicalYAML(source)
	if err != nil {
		return err
	}
	return s.publishNewResourceDirectory(ctx, "contexts", string(source.ContextID), map[string][]byte{contextFileName: data})
}

// publishNewResourceDirectory publishes one complete closed source set with a
// single directory rename. Existing source is never replaced: user edits are
// desired authority and must be preserved for explicit review/apply.
func (s *Store) publishNewResourceDirectory(ctx context.Context, concept, id string, files map[string][]byte) (resultErr error) {
	if s == nil || s.phase == nil || (concept != "templates" && concept != "contexts") || id == "" {
		return fmt.Errorf("resource source publication is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for name, data := range files {
		if filepath.Base(name) != name || len(data) == 0 || len(data) > maxSourceBytes {
			return fmt.Errorf("encoded resource source exceeds closed-set bounds")
		}
	}
	if err := ensurePrivateDirectoryTree(s.configRoot); err != nil {
		return err
	}
	conceptPath := filepath.Join(s.configRoot, concept)
	if err := ensurePrivateDirectory(conceptPath); err != nil {
		return err
	}
	finalPath := filepath.Join(conceptPath, id)
	if filepath.Dir(finalPath) != conceptPath {
		return fmt.Errorf("resource source identity is outside its concept root")
	}
	if info, err := os.Lstat(finalPath); err == nil {
		if err := validatePrivateDirectory(info); err != nil {
			return err
		}
		equal, compareErr := resourceDirectoryEquals(finalPath, files)
		if compareErr != nil {
			return compareErr
		}
		if equal {
			return nil
		}
		return tobari.ErrResourceSourceChanged
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stage, err := os.MkdirTemp(conceptPath, ".source-stage-")
	if err != nil {
		return err
	}
	if err := os.Chmod(stage, 0o700); err != nil { // #nosec G302 -- this is a directory narrowed to exact owner-only traversal.
		_ = os.RemoveAll(stage)
		return err
	}
	published := false
	defer func() {
		if resultErr == nil {
			return
		}
		if published {
			if rollbackErr := os.Rename(finalPath, stage); rollbackErr != nil {
				resultErr = errors.Join(tobari.ErrResourceSourceRecoveryRequired, resultErr, rollbackErr)
				return
			}
			published = false
		}
		cleanupErr := os.RemoveAll(stage)
		syncErr := syncDirectory(conceptPath)
		resultErr = errors.Join(resultErr, cleanupErr, syncErr)
	}()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := writeDurableSourceFile(filepath.Join(stage, name), files[name]); err != nil {
			return err
		}
		if err := s.phase("source_file_written:" + name); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	if complete, err := closedEntrySet(entries, names); err != nil || !complete {
		return errors.Join(tobari.ErrResourceSourceInvalid, err)
	}
	if err := syncDirectory(stage); err != nil {
		return err
	}
	if err := s.phase("source_stage_durable"); err != nil {
		return err
	}
	if _, err := os.Lstat(finalPath); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return tobari.ErrResourceSourceChanged
		}
		return err
	}
	if err := s.phase("source_before_publish"); err != nil {
		return err
	}
	if err := os.Rename(stage, finalPath); err != nil {
		return err
	}
	published = true
	if err := s.phase("source_directory_published"); err != nil {
		return err
	}
	if err := syncDirectory(conceptPath); err != nil {
		return err
	}
	published = false
	return nil
}

func writeDurableSourceFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- path is a closed child of a private staging directory.
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func resourceDirectoryEquals(path string, files map[string][]byte) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	if complete, err := closedEntrySet(entries, names); err != nil || !complete {
		return false, err
	}
	for name, want := range files {
		info, err := os.Lstat(filepath.Join(path, name))
		if err != nil || validatePrivateFile(info) != nil {
			return false, errors.Join(err, tobari.ErrResourceSourceInvalid)
		}
		got, err := os.ReadFile(filepath.Join(path, name)) // #nosec G304 -- closed validated resource directory.
		if err != nil {
			return false, err
		}
		if !bytes.Equal(got, want) {
			return false, nil
		}
	}
	return true, nil
}

// AdvanceTemplateBase records the exact revision activated from one observed
// source pair. It preserves YAML comments and ordering and never overwrites
// intervening edits.
func (s *Store) AdvanceTemplateBase(ctx context.Context, id tobari.WorkspaceTemplateID, expectedFingerprint string, revision tobari.SemanticDigest, resolved tobari.RuntimeBinding, beforePublish func(string) error) (string, error) {
	if expectedFingerprint == "" {
		return "", fmt.Errorf("Template source fingerprint is empty")
	}
	if err := revision.Validate(); err != nil {
		return "", err
	}
	path, err := s.TemplatePath(id)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(path)
	concept := filepath.Dir(dir)
	stage := filepath.Join(concept, ".template-base-"+string(id)+"-new")
	quarantine := filepath.Join(concept, ".template-base-"+string(id)+"-old")
	discard := filepath.Join(concept, ".template-base-"+string(id)+"-published")
	if settled, fingerprint, err := s.recoverTemplateBasePublication(dir, stage, quarantine, discard, expectedFingerprint, revision, resolved, beforePublish); settled || err != nil {
		return fingerprint, err
	}
	templateData, present, err := s.read(ctx, path, templateFileName, templateFileName, policyFileName)
	if err != nil {
		return "", err
	}
	if !present {
		return "", tobari.ErrResourceSourceMissing
	}
	policyPath := filepath.Join(filepath.Dir(path), policyFileName)
	policyData, present, err := s.read(ctx, policyPath, policyFileName, templateFileName, policyFileName)
	if err != nil {
		return "", err
	}
	if !present {
		return "", tobari.ErrResourceSourceMissing
	}
	if sourceFingerprint(templateData, policyData) != expectedFingerprint {
		var current tobari.WorkspaceTemplateSource
		if decodeStrictYAML(templateData, &current.Template) == nil && decodeStrictYAML(policyData, &current.Policy) == nil && current.Validate() == nil && current.Template.BaseRevision != nil && *current.Template.BaseRevision == revision {
			semantic, semanticErr := current.SemanticRevision(resolved)
			if semanticErr == nil && semantic == revision {
				return sourceFingerprint(templateData, policyData), syncDirectory(filepath.Dir(path))
			}
		}
		return "", tobari.ErrResourceSourceChanged
	}
	updated, err := replaceTopLevelScalar(templateData, "base_revision", string(revision))
	if err != nil {
		return "", errors.Join(tobari.ErrResourceSourceInvalid, err)
	}
	postFingerprint := sourceFingerprint(updated, policyData)
	// Bind both exact sides of the source-directory CAS before the first
	// publication rename. A recovery journal written after publication cannot
	// distinguish our bytes from an intervening external edit after a crash.
	if beforePublish != nil {
		if err := beforePublish(postFingerprint); err != nil {
			return "", err
		}
	}
	if err := os.Mkdir(stage, 0o700); err != nil {
		return "", err
	}
	stageOwned := true
	defer func() {
		if stageOwned {
			_ = os.RemoveAll(stage)
		}
	}()
	if err := writeDurableSourceFile(filepath.Join(stage, templateFileName), updated); err != nil {
		return "", err
	}
	if err := writeDurableSourceFile(filepath.Join(stage, policyFileName), policyData); err != nil {
		return "", err
	}
	if err := syncDirectory(stage); err != nil {
		return "", err
	}
	if err := s.phase("template_base_stage_durable"); err != nil {
		return "", err
	}
	if err := os.Rename(dir, quarantine); err != nil {
		return "", err
	}
	if err := syncDirectory(concept); err != nil {
		return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := s.phase("template_base_source_quarantined"); err != nil {
		return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	observed, err := templateDirectoryFingerprint(quarantine)
	if err != nil || observed != expectedFingerprint {
		if _, statErr := os.Lstat(dir); errors.Is(statErr, os.ErrNotExist) {
			_ = os.Rename(quarantine, dir)
			_ = syncDirectory(concept)
		}
		return "", errors.Join(tobari.ErrResourceSourceChanged, err)
	}
	if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		return "", errors.Join(tobari.ErrResourceSourceChanged, err)
	}
	if err := os.Rename(stage, dir); err != nil {
		return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	stageOwned = false
	if err := s.phase("template_base_directory_published"); err != nil {
		return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := syncDirectory(concept); err != nil {
		return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if observed, err := templateDirectoryFingerprint(dir); err != nil || observed != postFingerprint {
		return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := s.phase("template_base_before_quarantine_cleanup"); err != nil {
		return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if finalOld, err := templateDirectoryFingerprint(quarantine); err != nil || finalOld != observed {
		return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := os.RemoveAll(quarantine); err != nil {
		return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := s.phase("template_base_durable"); err != nil {
		return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	return postFingerprint, syncDirectory(concept)
}

func templateDirectoryFingerprint(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	if complete, err := closedEntrySet(entries, []string{policyFileName, templateFileName}); err != nil || !complete {
		return "", errors.Join(tobari.ErrResourceSourceInvalid, err)
	}
	templateData, err := os.ReadFile(filepath.Join(dir, templateFileName)) // #nosec G304 -- exact closed Template resource directory.
	if err != nil {
		return "", err
	}
	policyData, err := os.ReadFile(filepath.Join(dir, policyFileName)) // #nosec G304 -- exact closed Template resource directory.
	if err != nil {
		return "", err
	}
	return sourceFingerprint(templateData, policyData), nil
}

func (s *Store) recoverTemplateBasePublication(dir, stage, quarantine, discard, expected string, revision tobari.SemanticDigest, resolved tobari.RuntimeBinding, beforePublish func(string) error) (bool, string, error) {
	repairPath := filepath.Join(filepath.Dir(dir), ".template-base-"+filepath.Base(dir)+"-repair.json")
	if journal, present, err := s.readTemplateBaseRepairJournal(repairPath); err != nil {
		return false, "", err
	} else if present {
		fingerprint, err := s.settleTemplateBaseRepair(dir, stage, quarantine, discard, repairPath, journal)
		return err == nil, fingerprint, err
	}
	if _, err := os.Lstat(quarantine); errors.Is(err, os.ErrNotExist) {
		if _, discardErr := os.Lstat(discard); discardErr == nil {
			return false, "", tobari.ErrResourceSourceRecoveryRequired
		} else if !errors.Is(discardErr, os.ErrNotExist) {
			return false, "", discardErr
		}
		if _, stageErr := os.Lstat(stage); stageErr == nil {
			if err := s.removeAll(stage); err != nil {
				return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := s.sync(filepath.Dir(dir)); err != nil {
				return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
		} else if !errors.Is(stageErr, os.ErrNotExist) {
			return false, "", stageErr
		}
		return false, "", nil
	} else if err != nil {
		return false, "", err
	}
	oldFingerprint, err := templateDirectoryFingerprint(quarantine)
	if err != nil {
		return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	oldTemplate, err := os.ReadFile(filepath.Join(quarantine, templateFileName)) // #nosec G304 -- exact migration-owned quarantine.
	if err != nil {
		return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	oldPolicy, err := os.ReadFile(filepath.Join(quarantine, policyFileName)) // #nosec G304 -- exact migration-owned quarantine.
	if err != nil {
		return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	expectedTemplate, err := replaceTopLevelScalar(oldTemplate, "base_revision", string(revision))
	if err != nil {
		return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	expectedPost := sourceFingerprint(expectedTemplate, oldPolicy)
	currentFingerprint, currentErr := templateDirectoryFingerprint(dir)
	if currentErr == nil {
		discardPresent := false
		if _, discardErr := os.Lstat(discard); discardErr == nil {
			discardPresent = true
		} else if !errors.Is(discardErr, os.ErrNotExist) {
			return false, "", discardErr
		}
		var current tobari.WorkspaceTemplateSource
		currentTemplate, templateErr := os.ReadFile(filepath.Join(dir, templateFileName)) // #nosec G304 -- exact canonical Template directory.
		currentPolicy, policyErr := os.ReadFile(filepath.Join(dir, policyFileName))       // #nosec G304 -- exact canonical Template directory.
		advanced := templateErr == nil && policyErr == nil && decodeStrictYAML(currentTemplate, &current.Template) == nil && decodeStrictYAML(currentPolicy, &current.Policy) == nil && current.Validate() == nil && current.Template.BaseRevision != nil && *current.Template.BaseRevision == revision
		semantic, semanticErr := current.SemanticRevision(resolved)
		if !advanced || semanticErr != nil {
			return false, "", tobari.ErrResourceSourceRecoveryRequired
		}
		if discardPresent && currentFingerprint == expectedPost {
			if finalOld, err := templateDirectoryFingerprint(quarantine); err != nil || finalOld != oldFingerprint {
				return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := os.RemoveAll(quarantine); err != nil {
				return false, "", err
			}
			if err := os.RemoveAll(discard); err != nil {
				return false, "", err
			}
			return true, currentFingerprint, syncDirectory(filepath.Dir(dir))
		}
		if oldFingerprint == expected {
			if currentFingerprint != expectedPost || semantic != revision {
				return false, "", tobari.ErrResourceSourceChanged
			}
		} else {
			// The old directory remained writable through an already-open file
			// descriptor after its quarantine rename. Preserve those exact edits,
			// advancing only base_revision, then CAS the desired directory again.
			if currentFingerprint != expected {
				return false, "", tobari.ErrResourceSourceRecoveryRequired
			}
			if beforePublish != nil {
				if err := beforePublish(expectedPost); err != nil {
					return false, "", err
				}
			}
			stagePresent := false
			if stagedFingerprint, present, err := optionalTemplateDirectoryFingerprint(stage); err == nil && present {
				if stagedFingerprint != expectedPost {
					return false, "", tobari.ErrResourceSourceRecoveryRequired
				}
				stagePresent = true
			} else if err != nil {
				return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if _, err := os.Lstat(discard); !errors.Is(err, os.ErrNotExist) {
				return false, "", tobari.ErrResourceSourceRecoveryRequired
			}
			if !stagePresent {
				if err := os.Mkdir(stage, 0o700); err != nil {
					return false, "", err
				}
				if err := writeDurableSourceFile(filepath.Join(stage, templateFileName), expectedTemplate); err != nil {
					return false, "", err
				}
				if err := writeDurableSourceFile(filepath.Join(stage, policyFileName), oldPolicy); err != nil {
					return false, "", err
				}
				if err := syncDirectory(stage); err != nil {
					return false, "", err
				}
			}
			if confirmCurrent, err := templateDirectoryFingerprint(dir); err != nil || confirmCurrent != expected {
				return false, "", errors.Join(tobari.ErrResourceSourceChanged, err)
			}
			if confirmOld, err := templateDirectoryFingerprint(quarantine); err != nil || confirmOld != oldFingerprint {
				return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			journal := templateBaseRepairJournal{SchemaVersion: templateBaseRepairSchema, Phase: "prepared", Expected: expected, OldFingerprint: oldFingerprint, PostFingerprint: expectedPost, Revision: string(revision)}
			if err := s.writeTemplateBaseRepairJournal(repairPath, journal); err != nil {
				return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			fingerprint, err := s.settleTemplateBaseRepair(dir, stage, quarantine, discard, repairPath, journal)
			if err != nil {
				return false, "", err
			}
			return true, fingerprint, nil
		}
		if finalOld, err := templateDirectoryFingerprint(quarantine); err != nil || finalOld != oldFingerprint {
			return false, "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		if err := os.RemoveAll(quarantine); err != nil {
			return false, "", err
		}
		if err := os.RemoveAll(stage); err != nil {
			return false, "", err
		}
		if err := os.RemoveAll(discard); err != nil {
			return false, "", err
		}
		return true, currentFingerprint, syncDirectory(filepath.Dir(dir))
	}
	if !errors.Is(currentErr, os.ErrNotExist) {
		return false, "", tobari.ErrResourceSourceRecoveryRequired
	}
	if oldFingerprint != expected {
		return false, "", tobari.ErrResourceSourceRecoveryRequired
	}
	if stagedFingerprint, stageErr := templateDirectoryFingerprint(stage); stageErr == nil {
		if stagedFingerprint != expectedPost {
			return false, "", tobari.ErrResourceSourceRecoveryRequired
		}
		if err := os.Rename(stage, dir); err != nil {
			return false, "", err
		}
		if err := syncDirectory(filepath.Dir(dir)); err != nil {
			return false, "", err
		}
		if err := os.RemoveAll(quarantine); err != nil {
			return false, "", err
		}
		return true, stagedFingerprint, syncDirectory(filepath.Dir(dir))
	}
	if err := os.Rename(quarantine, dir); err != nil {
		return false, "", err
	}
	return false, "", syncDirectory(filepath.Dir(dir))
}

func (s *Store) writeTemplateBaseRepairJournal(path string, journal templateBaseRepairJournal) error {
	if journal.SchemaVersion != templateBaseRepairSchema || journal.Expected == "" || journal.OldFingerprint == "" || journal.PostFingerprint == "" || tobari.ValidateDigest(journal.Revision) != nil {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	switch journal.Phase {
	case "prepared", "discard_renaming", "discard_renamed", "published_renaming", "published", "cleanup_started":
	default:
		return tobari.ErrResourceSourceRecoveryRequired
	}
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	temp := path + ".next"
	if info, err := os.Lstat(temp); err == nil {
		if validatePrivateFile(info) != nil {
			return tobari.ErrResourceSourceRecoveryRequired
		}
		if err := os.Remove(temp); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- exact Template concept repair journal successor.
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = s.phase("template_base_repair_journal_temp_written:" + journal.Phase)
	}
	if err == nil {
		err = file.Sync()
	}
	if err == nil {
		err = s.phase("template_base_repair_journal_temp_synced:" + journal.Phase)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		return err
	}
	if err := s.phase("template_base_repair_journal_renamed:" + journal.Phase); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	return s.phase("template_base_repair_journal_parent_synced:" + journal.Phase)
}

func (s *Store) readTemplateBaseRepairJournal(path string) (templateBaseRepairJournal, bool, error) {
	temp := path + ".next"
	if info, err := os.Lstat(temp); err == nil {
		if validatePrivateFile(info) != nil {
			return templateBaseRepairJournal{}, false, tobari.ErrResourceSourceRecoveryRequired
		}
		if err := os.Remove(temp); err != nil {
			return templateBaseRepairJournal{}, false, err
		}
		if err := syncDirectory(filepath.Dir(path)); err != nil {
			return templateBaseRepairJournal{}, false, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return templateBaseRepairJournal{}, false, err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- exact Template concept repair journal path.
	if errors.Is(err, os.ErrNotExist) {
		return templateBaseRepairJournal{}, false, nil
	}
	if err != nil {
		return templateBaseRepairJournal{}, false, err
	}
	var journal templateBaseRepairJournal
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil {
		return templateBaseRepairJournal{}, false, tobari.ErrResourceSourceRecoveryRequired
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return templateBaseRepairJournal{}, false, tobari.ErrResourceSourceRecoveryRequired
	}
	if err := validateTemplateBaseRepairJournal(journal); err != nil {
		return templateBaseRepairJournal{}, false, err
	}
	return journal, true, nil
}

func validateTemplateBaseRepairJournal(journal templateBaseRepairJournal) error {
	if journal.SchemaVersion != templateBaseRepairSchema || journal.Expected == "" || journal.OldFingerprint == "" || journal.PostFingerprint == "" || tobari.ValidateDigest(journal.Revision) != nil {
		return tobari.ErrResourceSourceRecoveryRequired
	}
	switch journal.Phase {
	case "prepared", "discard_renaming", "discard_renamed", "published_renaming", "published", "cleanup_started":
		return nil
	default:
		return tobari.ErrResourceSourceRecoveryRequired
	}
}

func optionalTemplateDirectoryFingerprint(path string) (string, bool, error) {
	fingerprint, err := templateDirectoryFingerprint(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return fingerprint, true, nil
}

func (s *Store) settleTemplateBaseRepair(dir, stage, quarantine, discard, journalPath string, journal templateBaseRepairJournal) (string, error) {
	parent := filepath.Dir(dir)
	for {
		current, currentPresent, err := optionalTemplateDirectoryFingerprint(dir)
		if err != nil {
			return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		staged, stagePresent, err := optionalTemplateDirectoryFingerprint(stage)
		if err != nil {
			return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		discarded, discardPresent, err := optionalTemplateDirectoryFingerprint(discard)
		if err != nil {
			return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		old, oldPresent, err := optionalTemplateDirectoryFingerprint(quarantine)
		if err != nil {
			return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		if oldPresent && old != journal.OldFingerprint {
			return "", tobari.ErrResourceSourceRecoveryRequired
		}

		switch {
		case currentPresent && current == journal.Expected && stagePresent && staged == journal.PostFingerprint && !discardPresent && oldPresent:
			journal.Phase = "discard_renaming"
			if err := s.writeTemplateBaseRepairJournal(journalPath, journal); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := s.phase("template_base_repair_before_discard_rename"); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := os.Rename(dir, discard); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := s.phase("template_base_repair_discard_renamed"); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := syncDirectory(parent); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := s.phase("template_base_repair_discard_sync"); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			journal.Phase = "discard_renamed"
			if err := s.writeTemplateBaseRepairJournal(journalPath, journal); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			continue
		case !currentPresent && stagePresent && staged == journal.PostFingerprint && discardPresent && discarded == journal.Expected && oldPresent:
			journal.Phase = "published_renaming"
			if err := s.writeTemplateBaseRepairJournal(journalPath, journal); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := s.phase("template_base_repair_before_publish_rename"); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := os.Rename(stage, dir); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := s.phase("template_base_repair_published_renamed"); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := syncDirectory(parent); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := s.phase("template_base_repair_publish_sync"); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			journal.Phase = "published"
			if err := s.writeTemplateBaseRepairJournal(journalPath, journal); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			continue
		case currentPresent && current == journal.PostFingerprint && !stagePresent && ((discardPresent && discarded == journal.Expected && oldPresent) || (journal.Phase == "cleanup_started" && (!discardPresent || discarded == journal.Expected))):
			journal.Phase = "cleanup_started"
			if err := s.writeTemplateBaseRepairJournal(journalPath, journal); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if oldPresent {
				if err := os.RemoveAll(quarantine); err != nil {
					return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
				}
				if err := s.phase("template_base_repair_quarantine_removed"); err != nil {
					return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
				}
			}
			if discardPresent {
				if err := os.RemoveAll(discard); err != nil {
					return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
				}
				if err := s.phase("template_base_repair_discard_removed"); err != nil {
					return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
				}
			}
			if err := syncDirectory(parent); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := s.phase("template_base_repair_cleanup_sync"); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := os.Remove(journalPath); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := s.phase("template_base_repair_journal_removed"); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := syncDirectory(parent); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			if err := s.phase("template_base_repair_journal_remove_synced"); err != nil {
				return "", errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
			}
			return journal.PostFingerprint, nil
		default:
			return "", tobari.ErrResourceSourceRecoveryRequired
		}
	}
}

func (s *Store) DeleteTemplate(ctx context.Context, id tobari.WorkspaceTemplateID) error {
	path, err := s.TemplatePath(id)
	if err != nil {
		return err
	}
	return s.deleteResourceDirectory(ctx, path, templateFileName, policyFileName)
}

func (s *Store) DeleteContext(ctx context.Context, id tobari.ContextID) error {
	path, err := s.ContextPath(id)
	if err != nil {
		return err
	}
	return s.deleteResourceDirectory(ctx, path, contextFileName)
}

func (s *Store) read(ctx context.Context, path, targetName string, expectedNames ...string) ([]byte, bool, error) {
	if filepath.Base(path) != targetName {
		return nil, false, fmt.Errorf("resource source target does not match its closed file set")
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	dir := filepath.Dir(path)
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := validatePrivateDirectory(info); err != nil {
		return nil, false, fmt.Errorf("resource source directory: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, false, err
	}
	complete, err := closedEntrySet(entries, expectedNames)
	if err != nil {
		return nil, false, err
	}
	if !complete {
		return nil, false, nil
	}
	before, err := os.Lstat(path)
	if err != nil {
		return nil, false, err
	}
	if err := validatePrivateFile(before); err != nil {
		return nil, false, fmt.Errorf("resource source file: %w", err)
	}
	if before.Size() <= 0 || before.Size() > maxSourceBytes {
		return nil, false, fmt.Errorf("resource source must contain 1..%d bytes", maxSourceBytes)
	}
	file, err := os.Open(path) // #nosec G304 -- exact child of validated ID-owned directory.
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, false, fmt.Errorf("resource source changed during safe open")
	}
	if err := validatePrivateFile(opened); err != nil {
		return nil, false, err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSourceBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxSourceBytes {
		return nil, false, fmt.Errorf("read bounded resource source: %w", err)
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) {
		return nil, false, fmt.Errorf("resource source changed during observation")
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	return data, true, nil
}

func (s *Store) publish(ctx context.Context, path, expectedName string, value any, allowedNames ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := encodeCanonicalYAML(value)
	if err != nil {
		return err
	}
	if len(data) == 0 || len(data) > maxSourceBytes {
		return fmt.Errorf("encoded resource source exceeds bounds")
	}
	return s.publishBytes(ctx, path, expectedName, data, allowedNames...)
}

func (s *Store) publishBytes(ctx context.Context, path, expectedName string, data []byte, allowedNames ...string) error {
	if filepath.Base(path) != expectedName {
		return fmt.Errorf("resource source target does not match its closed file set")
	}
	if len(data) == 0 || len(data) > maxSourceBytes {
		return fmt.Errorf("encoded resource source exceeds bounds")
	}
	if err := ensurePrivateDirectory(s.configRoot); err != nil {
		return err
	}
	concept := filepath.Dir(filepath.Dir(path))
	if err := ensurePrivateDirectory(concept); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := ensurePrivateDirectory(dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	allowed := make(map[string]struct{}, len(allowedNames))
	for _, name := range allowedNames {
		allowed[name] = struct{}{}
	}
	for _, entry := range entries {
		if _, ok := allowed[entry.Name()]; !ok {
			return fmt.Errorf("resource source directory contains an unknown entry")
		}
	}
	if info, err := os.Lstat(path); err == nil {
		if err := validatePrivateFile(info); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp, err := os.CreateTemp(dir, ".source-*")
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
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func sourceFingerprint(templateData, policyData []byte) string {
	digest := sha256.Sum256(append(append(append([]byte{}, templateData...), 0), policyData...))
	return hex.EncodeToString(digest[:])
}

func replaceTopLevelScalar(data []byte, key, value string) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("YAML document must contain one top-level mapping")
	}
	mapping := document.Content[0]
	found := false
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value != key {
			continue
		}
		scalar := mapping.Content[index+1]
		scalar.Kind = yaml.ScalarNode
		scalar.Tag = "!!str"
		scalar.Value = value
		scalar.Content = nil
		scalar.Alias = nil
		found = true
		break
	}
	if !found {
		return nil, fmt.Errorf("YAML document is missing %q", key)
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func (s *Store) deleteResourceDirectory(ctx context.Context, path string, expectedNames ...string) error {
	if s == nil || s.phase == nil {
		return fmt.Errorf("resource source deletion is unavailable")
	}
	dir := filepath.Dir(path)
	concept := filepath.Dir(dir)
	quarantineRoot := filepath.Join(s.configRoot, ".source-delete-quarantine", filepath.Base(concept))
	quarantine := filepath.Join(quarantineRoot, filepath.Base(dir))
	if filepath.Dir(quarantine) != quarantineRoot {
		return fmt.Errorf("resource source deletion target is unsafe")
	}
	if _, err := os.Lstat(quarantine); err == nil {
		if _, finalErr := os.Lstat(dir); !errors.Is(finalErr, os.ErrNotExist) {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, finalErr)
		}
		if err := os.RemoveAll(quarantine); err != nil {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		if err := syncDirectory(quarantineRoot); err != nil {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
		return cleanupSourceDeleteQuarantine(s.configRoot, quarantineRoot)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, name := range expectedNames {
		target := filepath.Join(dir, name)
		if _, present, err := s.read(ctx, target, name, expectedNames...); err != nil {
			return err
		} else if !present {
			return os.ErrNotExist
		}
	}
	if err := ensurePrivateDirectory(filepath.Join(s.configRoot, ".source-delete-quarantine")); err != nil {
		return err
	}
	if err := ensurePrivateDirectory(quarantineRoot); err != nil {
		return err
	}
	if err := os.Rename(dir, quarantine); err != nil {
		return err
	}
	if err := syncDirectory(concept); err != nil {
		rollbackErr := os.Rename(quarantine, dir)
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err, rollbackErr)
	}
	if err := s.phase("source_delete_quarantined"); err != nil {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := os.RemoveAll(quarantine); err != nil {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := s.phase("source_delete_removed"); err != nil {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if err := syncDirectory(quarantineRoot); err != nil {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	return cleanupSourceDeleteQuarantine(s.configRoot, quarantineRoot)
}

func cleanupSourceDeleteQuarantine(configRoot, conceptRoot string) error {
	parent := filepath.Dir(conceptRoot)
	if err := os.Remove(conceptRoot); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	if entries, err := os.ReadDir(parent); err == nil && len(entries) == 0 {
		if err := os.Remove(parent); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(tobari.ErrResourceSourceRecoveryRequired, err)
	}
	return syncDirectory(configRoot)
}

func closedEntrySet(entries []os.DirEntry, names []string) (bool, error) {
	want := make(map[string]struct{}, len(names))
	for _, name := range names {
		want[name] = struct{}{}
	}
	for _, entry := range entries {
		if _, ok := want[entry.Name()]; !ok {
			return false, fmt.Errorf("resource source directory contains unknown entry %q", entry.Name())
		}
		delete(want, entry.Name())
	}
	return len(want) == 0, nil
}

func ensurePrivateDirectory(path string) error {
	info, err := os.Lstat(path) // #nosec G703 -- callers provide one canonical absolute concept root or exact child and this function validates owner/mode before use.
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o700); err != nil { // #nosec G703 -- same exact validated concept-root boundary; no request path segment is accepted here.
			return err
		}
		info, err = os.Lstat(path) // #nosec G703 -- rechecks the exact directory just created before returning authority over it.
	}
	if err != nil {
		return err
	}
	return validatePrivateDirectory(info)
}

// ensurePrivateDirectoryTree owns initial creation of the configured source
// root. Its XDG parent may legitimately be absent on a user's first command;
// concept and resource children remain single-level creations through
// ensurePrivateDirectory.
func ensurePrivateDirectoryTree(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil { // #nosec G703 -- path is the installation-owned canonical config root, not a request-derived resource path.
		return err
	}
	info, err := os.Lstat(path) // #nosec G703 -- validates the exact root after recursive first-use creation before authority is published beneath it.
	if err != nil {
		return err
	}
	return validatePrivateDirectory(info)
}

func validatePrivateDirectory(info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0o700 || !ownedByCurrentUser(info) {
		return fmt.Errorf("must be one real owner-only directory")
	}
	return nil
}

func validatePrivateFile(info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) || linkCount(info) != 1 {
		return fmt.Errorf("must be one real owner-only regular file")
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path) // #nosec G304,G703 -- exact canonical owner directory validated by the source transaction.
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func decodeStrictYAML(data []byte, target any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	if len(document.Content) != 1 {
		return fmt.Errorf("YAML document must contain one value")
	}
	value, err := yamlJSONValue(document.Content[0])
	if err != nil {
		return err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("YAML source contains multiple documents")
		}
		return err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	jsonDecoder := json.NewDecoder(bytes.NewReader(encoded))
	jsonDecoder.DisallowUnknownFields()
	if err := jsonDecoder.Decode(target); err != nil {
		return err
	}
	if jsonDecoder.More() {
		return fmt.Errorf("decoded source contains trailing JSON")
	}
	return nil
}

func yamlJSONValue(node *yaml.Node) (any, error) {
	if node == nil || node.Alias != nil || node.Kind == yaml.AliasNode || node.Anchor != "" {
		return nil, fmt.Errorf("YAML aliases and anchors are unsupported")
	}
	switch node.Kind {
	case yaml.MappingNode:
		if node.Tag != "!!map" || len(node.Content)%2 != 0 {
			return nil, fmt.Errorf("YAML mapping is invalid")
		}
		result := make(map[string]any, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "<<" {
				return nil, fmt.Errorf("YAML mapping keys must be plain strings without merges")
			}
			if _, exists := result[key.Value]; exists {
				return nil, fmt.Errorf("YAML mapping contains duplicate key %q", key.Value)
			}
			value, err := yamlJSONValue(node.Content[index+1])
			if err != nil {
				return nil, err
			}
			result[key.Value] = value
		}
		return result, nil
	case yaml.SequenceNode:
		if node.Tag != "!!seq" {
			return nil, fmt.Errorf("YAML sequence tag is unsupported")
		}
		result := make([]any, len(node.Content))
		for index := range node.Content {
			value, err := yamlJSONValue(node.Content[index])
			if err != nil {
				return nil, err
			}
			result[index] = value
		}
		return result, nil
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!str":
			return node.Value, nil
		case "!!bool":
			return strconv.ParseBool(node.Value)
		case "!!int":
			return strconv.ParseInt(node.Value, 10, 64)
		case "!!null":
			return nil, nil
		default:
			return nil, fmt.Errorf("YAML scalar tag %q is unsupported", node.Tag)
		}
	default:
		return nil, fmt.Errorf("YAML node kind is unsupported")
	}
}

func encodeCanonicalYAML(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var generic any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&generic); err != nil {
		return nil, err
	}
	node, err := jsonYAMLNode(generic)
	if err != nil {
		return nil, err
	}
	document := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{node}}
	var output bytes.Buffer
	yamlEncoder := yaml.NewEncoder(&output)
	yamlEncoder.SetIndent(2)
	if err := yamlEncoder.Encode(document); err != nil {
		return nil, err
	}
	if err := yamlEncoder.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func jsonYAMLNode(value any) (*yaml.Node, error) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, key := range keys {
			child, err := jsonYAMLNode(typed[key])
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, child)
		}
		return node, nil
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, value := range typed {
			child, err := jsonYAMLNode(value)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, child)
		}
		return node, nil
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: typed}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(typed)}, nil
	case json.Number:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: typed.String()}, nil
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	default:
		return nil, fmt.Errorf("JSON-compatible source value is unsupported")
	}
}
