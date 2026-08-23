package dockerruntime

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
)

const (
	migrationPolicyGuardrail   = "method_policy"
	legacyStandardRuntimeImage = "tobari-runtime:dev"
)

type legacyContextManifest struct {
	SchemaVersion        int                                      `json:"schema_version"`
	ID                   string                                   `json:"id"`
	Name                 string                                   `json:"name"`
	AgentProfile         string                                   `json:"agent_profile"`
	Image                string                                   `json:"image"`
	PolicyMode           tobari.ManifestPolicyMode                `json:"policy_mode"`
	SourceAccess         tobari.ManifestSourceAccess              `json:"source_access"`
	NativeReadiness      tobari.ManifestNativeReadiness           `json:"native_readiness,omitempty"`
	PolicyPresetOrigin   string                                   `json:"policy_preset_origin"`
	PolicyPresetRevision string                                   `json:"policy_preset_revision"`
	Runtime              *tobari.ManifestRuntimeRecipe            `json:"runtime,omitempty"`
	ShellEnvironment     []tobari.ManifestShellEnvironmentSetting `json:"shell_environment,omitempty"`
	GitIdentity          *tobari.ManifestGitIdentitySetting       `json:"git_identity,omitempty"`
	Bootstrap            *tobari.ManifestBootstrapSnapshot        `json:"bootstrap,omitempty"`
}

type legacyContextPolicy struct {
	SchemaVersion      int                                     `json:"schema_version"`
	Name               string                                  `json:"name"`
	DestinationCeiling tobari.ManifestPolicyDestinationCeiling `json:"destination_ceiling"`
	MethodPolicy       tobari.ManifestMethodPolicy             `json:"method_policy"`
	BaselineGrants     []tobari.ManifestPolicyExactRule        `json:"baseline_grants"`
	BaselineTemplates  []tobari.ManifestPolicyPathTemplateRule `json:"baseline_templates"`
	MCPBaselineGrants  []tobari.ManifestPolicyMCPRule          `json:"mcp_baseline_grants"`
	BaselineDenies     []tobari.ManifestPolicyExactRule        `json:"baseline_denies"`
	GraphQLEndpoints   []tobari.ManifestPolicyExactRule        `json:"graphql_endpoints"`
	MCPEndpoints       []tobari.ManifestPolicyExactRule        `json:"mcp_endpoints"`
	Guardrail          string                                  `json:"guardrail"`
}

func (p legacyContextPolicy) current() tobari.ManifestPolicy {
	return tobari.ManifestPolicy{
		SchemaVersion: p.SchemaVersion, Name: p.Name,
		DestinationCeiling: p.DestinationCeiling, MethodPolicy: p.MethodPolicy,
		BaselineGrants: p.BaselineGrants, BaselineTemplates: p.BaselineTemplates,
		MCPBaselineGrants: p.MCPBaselineGrants, BaselineDenies: p.BaselineDenies,
		GraphQLEndpoints: p.GraphQLEndpoints, MCPEndpoints: p.MCPEndpoints,
	}
}

type migrationContextPlan struct {
	name               string
	legacy             bool
	removeLegacyPolicy bool
	sourceManifest     []byte
	sourcePolicy       []byte
	sourceDockerfile   []byte
	manifest           tobari.WorkspaceManifest
	policy             []byte
	runtimeSelection   string
}

type migrationBackupManifest struct {
	SchemaVersion int      `json:"schema_version"`
	Source        string   `json:"source"`
	Digest        string   `json:"digest"`
	Contexts      []string `json:"contexts"`
}

// MigrateInstallation is the only infrastructure reader for the enumerated
// unpublished predecessor. Current Context readers remain strict.
func (r *Runtime) MigrateInstallation(ctx context.Context, diagnostics io.Writer) (tobari.MigrationReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.MigrationReport{}, err
	}
	var result tobari.MigrationReport
	err := r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		journal, journalExists, err := r.readResearchAuthJournal()
		if err != nil {
			return fmt.Errorf("%w: research auth journal: %v", tobari.ErrMigrationSourceUnsafe, err)
		}
		if journalExists && journal.Committed {
			plans, planErr := r.planInstallationMigration(lockContext)
			if planErr != nil {
				return planErr
			}
			for _, plan := range plans {
				if migrationPlanChanges(plan) {
					return fmt.Errorf("%w: predecessor state reappeared after committed migration", tobari.ErrMigrationSourceUnsafe)
				}
			}
			result, err = r.committedMigrationReport(lockContext, journal)
			return err
		}
		if journalExists && journal.ContextsCommitted {
			if err := r.resumeResearchAuthQuarantine(journal); err != nil {
				return err
			}
			if err := r.commitMigrationDefaultSelector(journal.DefaultManifest); err != nil {
				return err
			}
			journal.SelectorCommitted, journal.Committed = true, true
			if err := writeAtomicJSON(r.researchAuthJournalPath(), journal); err != nil {
				return err
			}
			result, err = r.committedMigrationReport(lockContext, journal)
			if err == nil {
				result.Changed = true
				recovery := journal.RecoveryID
				result.RecoveryID = &recovery
			}
			return err
		}
		plans, err := r.planInstallationMigration(lockContext)
		if err != nil {
			return err
		}
		legacyDefaultName, err := r.readMigrationActiveContext()
		if err != nil {
			return fmt.Errorf("%w: default Manifest source: %v", tobari.ErrMigrationSourceUnsafe, err)
		}
		var researchPlan researchAuthPlan
		if journalExists {
			researchPlan = researchAuthPlan{
				Digest: journal.Digest, StateDigest: journal.StateDigest, ConfigDigest: journal.ConfigDigest,
				StatePresent: journal.StatePresent, ConfigPresent: journal.ConfigPresent,
				Artifacts: append([]researchAuthArtifact{}, journal.Artifacts...),
			}
			if err := r.resumeResearchAuthQuarantine(journal); err != nil {
				return err
			}
		} else {
			researchPlan, err = r.planResearchAuthMigration(plans)
			if err != nil {
				return err
			}
		}
		if err := r.validateMigrationRuntimeQuiescence(lockContext); err != nil {
			return err
		}
		changeCount := 1 // the predecessor name marker always becomes an ID-bound selector
		for index := range plans {
			if !migrationPlanChanges(plans[index]) {
				continue
			}
			changeCount++
			if !plans[index].legacy {
				continue
			}
			if plans[index].manifest.Runtime == nil {
				binding, bindingErr := r.standardRuntimeManifest().Binding(1)
				if bindingErr != nil {
					return fmt.Errorf("%w: resolve standard Runtime: %v", tobari.ErrMigrationRuntimeFailed, bindingErr)
				}
				applyMigrationRuntimeBinding(&plans[index], binding)
				continue
			}
			binding, bindingErr := r.prepareLegacyMigrationRuntime(lockContext, plans[index].name, plans[index].sourceDockerfile, diagnostics)
			if bindingErr != nil {
				return bindingErr
			}
			applyMigrationRuntimeBinding(&plans[index], binding)
		}

		var recoveryID *string
		if changeCount > 0 {
			_, recovery, backupErr := r.createMigrationBackup(plans, researchPlan.Digest)
			if backupErr != nil {
				return fmt.Errorf("%w: %v", tobari.ErrMigrationBackupFailed, backupErr)
			}
			recoveryID = &recovery
			if !journalExists {
				if quarantineErr := r.quarantineResearchAuth(researchPlan, legacyDefaultName); quarantineErr != nil {
					return fmt.Errorf("%w: research auth quarantine: %v", tobari.ErrMigrationWriteFailed, quarantineErr)
				}
				journal, journalExists, err = r.readResearchAuthJournal()
				if err != nil || !journalExists {
					return fmt.Errorf("%w: research auth journal unavailable: %v", tobari.ErrMigrationWriteFailed, err)
				}
			}
			journal.RecoveryID = recovery
			if err := writeAtomicJSON(r.researchAuthJournalPath(), journal); err != nil {
				return err
			}
			if commitErr := r.commitMigrationPlans(plans); commitErr != nil {
				return commitErr
			}
			journal.ContextsCommitted = true
			if err := writeAtomicJSON(r.researchAuthJournalPath(), journal); err != nil {
				return err
			}
			if err := r.commitMigrationDefaultSelector(legacyDefaultName); err != nil {
				return err
			}
			journal.SelectorCommitted, journal.Committed = true, true
			if err := writeAtomicJSON(r.researchAuthJournalPath(), journal); err != nil {
				return err
			}
		}

		items := make([]tobari.MigrationContextResult, 0, len(plans))
		for _, plan := range plans {
			state := tobari.MigrationContextCurrent
			if migrationPlanChanges(plan) {
				state = tobari.MigrationContextMigrated
			}
			items = append(items, tobari.MigrationContextResult{
				ID: plan.manifest.ID, Name: plan.name, State: state,
				Runtime: plan.runtimeSelection, PolicyRevision: plan.manifest.PolicyRevision,
			})
		}
		result = tobari.MigrationReport{
			Task: tobari.TaskMigrationApply, Source: tobari.MigrationSourcePreV1ContextPolicyRuntime,
			Changed: changeCount > 0, RecoveryID: recoveryID, Contexts: items,
			ResearchAuthDisposition: researchAuthDisposition(researchPlan),
		}
		return result.Validate()
	})
	if err != nil {
		return tobari.MigrationReport{}, err
	}
	return result, nil
}

func researchAuthDisposition(plan researchAuthPlan) tobari.ResearchAuthDisposition {
	if plan.StatePresent || plan.ConfigPresent || len(plan.Artifacts) != 0 {
		return tobari.ResearchAuthReauthenticationRequired
	}
	return tobari.ResearchAuthNotPresent
}

func researchAuthDispositionFromJournal(journal researchAuthJournal) tobari.ResearchAuthDisposition {
	return researchAuthDisposition(researchAuthPlan{
		StatePresent: journal.StatePresent, ConfigPresent: journal.ConfigPresent, Artifacts: journal.Artifacts,
	})
}

func (r *Runtime) commitMigrationDefaultSelector(name string) error {
	selected, err := r.readContextManifest(name)
	if err != nil {
		return fmt.Errorf("%w: migrated default Manifest: %v", tobari.ErrMigrationWriteFailed, err)
	}
	if err := r.writeDefaultManifest(selected); err != nil {
		return fmt.Errorf("%w: default Manifest selector: %v", tobari.ErrMigrationWriteFailed, err)
	}
	if err := os.Remove(r.activeContextPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: predecessor selector: %v", tobari.ErrMigrationWriteFailed, err)
	}
	return syncDirectoryIfPresent(r.contextsDirectory())
}

func (r *Runtime) committedMigrationReport(ctx context.Context, journal researchAuthJournal) (tobari.MigrationReport, error) {
	listed, err := r.ListContexts(ctx)
	if err != nil {
		return tobari.MigrationReport{}, err
	}
	items := make([]tobari.MigrationContextResult, 0, len(listed.Items))
	for _, manifest := range listed.Items {
		items = append(items, tobari.MigrationContextResult{
			ID: manifest.ID, Name: manifest.Name, State: tobari.MigrationContextCurrent,
			Runtime: manifest.RuntimeSelection, PolicyRevision: manifest.PolicyRevision,
		})
	}
	result := tobari.MigrationReport{
		Task: tobari.TaskMigrationApply, Source: tobari.MigrationSourcePreV1ContextPolicyRuntime,
		Changed: false, RecoveryID: nil, ResearchAuthDisposition: researchAuthDispositionFromJournal(journal), Contexts: items,
	}
	return result, result.Validate()
}

func migrationPlanChanges(plan migrationContextPlan) bool {
	return plan.legacy || plan.removeLegacyPolicy
}

func applyMigrationRuntimeBinding(plan *migrationContextPlan, binding tobari.RuntimeBinding) {
	plan.manifest.Runtime = nil
	copy := binding
	plan.manifest.RuntimeBinding = &copy
	plan.manifest.Image = binding.Image
	plan.runtimeSelection = binding.Name
	if binding.RuntimeID != tobari.StandardRuntimeID {
		plan.runtimeSelection = fmt.Sprintf("%s@%d", binding.Name, binding.Ordinal)
	}
}

func (r *Runtime) planInstallationMigration(ctx context.Context) ([]migrationContextPlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := requirePrivateDirectory(r.contextsDirectory()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, tobari.ErrMigrationNotSupported
		}
		return nil, fmt.Errorf("%w: Context directory: %v", tobari.ErrMigrationSourceUnsafe, err)
	}
	entries, err := os.ReadDir(r.contextsDirectory())
	if err != nil {
		return nil, fmt.Errorf("%w: list Contexts: %v", tobari.ErrMigrationSourceUnsafe, err)
	}
	var names []string
	var legacySelector, currentSelector bool
	for _, entry := range entries {
		if !entry.IsDir() {
			switch entry.Name() {
			case "active.json":
				legacySelector = true
				continue
			case "default.json":
				currentSelector = true
				continue
			}
			return nil, fmt.Errorf("%w: unexpected Context store entry", tobari.ErrMigrationSourceUnsafe)
		}
		if err := tobari.ValidateName(entry.Name()); err != nil {
			return nil, fmt.Errorf("%w: invalid Context directory", tobari.ErrMigrationSourceUnsafe)
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return nil, tobari.ErrMigrationNotSupported
	}
	if legacySelector == currentSelector {
		return nil, fmt.Errorf("%w: Manifest selector state is ambiguous", tobari.ErrMigrationSourceUnsafe)
	}
	sort.Strings(names)
	plans := make([]migrationContextPlan, 0, len(names))
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		plan, err := r.planContextMigration(name)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	var active string
	if legacySelector {
		active, err = r.readMigrationActiveContext()
	} else {
		active, err = r.readDefaultManifestName()
	}
	if err != nil {
		return nil, fmt.Errorf("%w: active Context: %v", tobari.ErrMigrationSourceUnsafe, err)
	}
	if _, found := sort.Find(len(names), func(index int) int { return strings.Compare(active, names[index]) }); !found {
		return nil, fmt.Errorf("%w: active Context is unavailable", tobari.ErrMigrationSourceUnsafe)
	}
	return plans, nil
}

func (r *Runtime) readMigrationActiveContext() (string, error) {
	data, err := readPrivateMigrationFile(r.activeContextPath(), maxActiveContextDocumentBytes)
	if err != nil {
		return "", err
	}
	var document legacyActiveContextDocument
	if err := decodeStrictMigrationJSON(data, &document); err != nil {
		return "", err
	}
	if err := tobari.ValidateName(document.Name); err != nil {
		return "", err
	}
	return document.Name, nil
}

func (r *Runtime) planContextMigration(name string) (migrationContextPlan, error) {
	manifestPath := r.contextManifestPath(name)
	rawManifest, err := readPrivateMigrationFile(manifestPath, maxContextManifestBytes)
	if err != nil {
		return migrationContextPlan{}, fmt.Errorf("%w: Context manifest: %v", tobari.ErrMigrationSourceUnsafe, err)
	}
	if current, err := r.readContextManifestRaw(name); err == nil {
		if current.RuntimeBinding == nil || current.Runtime != nil {
			return migrationContextPlan{}, fmt.Errorf("%w: current Context lacks an exact Runtime binding", tobari.ErrMigrationSourceUnsafe)
		}
		plan := migrationContextPlan{
			name: name, sourceManifest: rawManifest, manifest: current,
			runtimeSelection: migrationRuntimeSelection(*current.RuntimeBinding),
		}
		legacyPolicyPath := filepath.Join(r.contextPolicyDirectory(name), "preset.json")
		if _, statErr := os.Lstat(legacyPolicyPath); errors.Is(statErr, os.ErrNotExist) {
			return plan, nil
		} else if statErr != nil {
			return migrationContextPlan{}, fmt.Errorf("%w: inspect residual Context policy: %v", tobari.ErrMigrationSourceUnsafe, statErr)
		}
		rawPolicy, readErr := readPrivateMigrationFile(legacyPolicyPath, maxContextPolicyBytes)
		if readErr != nil {
			return migrationContextPlan{}, fmt.Errorf("%w: residual Context policy: %v", tobari.ErrMigrationSourceUnsafe, readErr)
		}
		digest := sha256.Sum256(rawPolicy)
		_, _, revision, convertErr := convertLegacyContextPolicy(rawPolicy, "sha256:"+hex.EncodeToString(digest[:]))
		if convertErr != nil || revision != current.PolicyRevision {
			return migrationContextPlan{}, fmt.Errorf("%w: residual Context policy does not match current authority", tobari.ErrMigrationNotSupported)
		}
		plan.removeLegacyPolicy = true
		plan.sourcePolicy = rawPolicy
		return plan, nil
	}

	legacy, err := decodeLegacyContextManifest(rawManifest, name)
	if err != nil {
		return migrationContextPlan{}, fmt.Errorf("%w: Context %q: %v", tobari.ErrMigrationNotSupported, name, err)
	}
	policyPath := filepath.Join(r.contextPolicyDirectory(name), "preset.json")
	rawPolicy, err := readPrivateMigrationFile(policyPath, maxContextPolicyBytes)
	if err != nil {
		return migrationContextPlan{}, fmt.Errorf("%w: Context %q policy: %v", tobari.ErrMigrationSourceUnsafe, name, err)
	}
	policy, normalized, revision, err := convertLegacyContextPolicy(rawPolicy, legacy.PolicyPresetRevision)
	if err != nil {
		return migrationContextPlan{}, fmt.Errorf("%w: Context %q policy: %v", tobari.ErrMigrationSourceUnsafe, name, err)
	}
	_ = policy
	readiness, err := tobari.ResolveContextNativeReadiness(legacy.NativeReadiness)
	if err != nil {
		return migrationContextPlan{}, fmt.Errorf("%w: Context %q readiness: %v", tobari.ErrMigrationSourceUnsafe, name, err)
	}
	manifest := tobari.WorkspaceManifest{
		SchemaVersion: legacy.SchemaVersion, ID: legacy.ID, Name: legacy.Name,
		AgentProfile: legacy.AgentProfile, Image: legacy.Image, PolicyMode: legacy.PolicyMode,
		SourceAccess: legacy.SourceAccess, PolicyRevision: revision, NativeReadiness: readiness,
		Runtime: legacy.Runtime, ShellEnvironment: legacy.ShellEnvironment,
		GitIdentity: legacy.GitIdentity, Bootstrap: legacy.Bootstrap,
	}
	var dockerfile []byte
	if legacy.Runtime == nil {
		if legacy.Image != legacyStandardRuntimeImage {
			return migrationContextPlan{}, fmt.Errorf("%w: Context %q does not use the standard image", tobari.ErrMigrationNotSupported, name)
		}
	} else {
		path := filepath.Join(r.contextDirectory(name), legacy.Runtime.File)
		dockerfile, err = readPrivateMigrationFile(path, maxRuntimeSourceFile)
		if err != nil {
			return migrationContextPlan{}, fmt.Errorf("%w: Context %q Runtime source: %v", tobari.ErrMigrationSourceUnsafe, name, err)
		}
		actual := sha256.Sum256(dockerfile)
		if legacy.Runtime.SourceDigest != "sha256:"+hex.EncodeToString(actual[:]) || legacy.Runtime.LastBuild == nil ||
			legacy.Runtime.LastBuild.SourceDigest != legacy.Runtime.SourceDigest || legacy.Runtime.LastBuild.Image != legacy.Image {
			return migrationContextPlan{}, fmt.Errorf("%w: Context %q Runtime source or build evidence drifted", tobari.ErrMigrationSourceUnsafe, name)
		}
	}
	return migrationContextPlan{
		name: name, legacy: true, removeLegacyPolicy: true, sourceManifest: rawManifest, sourcePolicy: rawPolicy,
		sourceDockerfile: dockerfile, manifest: manifest, policy: normalized,
	}, nil
}

func decodeLegacyContextManifest(data []byte, name string) (legacyContextManifest, error) {
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return legacyContextManifest{}, err
	}
	var manifest legacyContextManifest
	if err := decodeStrictMigrationJSON(data, &manifest); err != nil {
		return legacyContextManifest{}, err
	}
	if manifest.Name != name || manifest.PolicyPresetOrigin != "builtin/agent-ready" {
		return legacyContextManifest{}, fmt.Errorf("legacy Context identity is unsupported")
	}
	if err := tobari.ValidateDigest(manifest.PolicyPresetRevision); err != nil {
		return legacyContextManifest{}, err
	}
	probe := tobari.WorkspaceManifest{
		SchemaVersion: manifest.SchemaVersion, ID: manifest.ID, Name: manifest.Name,
		AgentProfile: manifest.AgentProfile, Image: manifest.Image, PolicyMode: manifest.PolicyMode,
		SourceAccess: manifest.SourceAccess, PolicyRevision: tobari.DefaultContextPolicyRevision(),
		NativeReadiness: manifest.NativeReadiness, Runtime: manifest.Runtime,
		ShellEnvironment: manifest.ShellEnvironment, GitIdentity: manifest.GitIdentity, Bootstrap: manifest.Bootstrap,
	}
	if err := probe.Validate(); err != nil {
		return legacyContextManifest{}, err
	}
	return manifest, nil
}

func convertLegacyContextPolicy(data []byte, declaredRevision string) (tobari.ManifestPolicy, []byte, string, error) {
	actual := sha256.Sum256(data)
	if declaredRevision != "sha256:"+hex.EncodeToString(actual[:]) {
		return tobari.ManifestPolicy{}, nil, "", fmt.Errorf("legacy policy revision mismatch")
	}
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return tobari.ManifestPolicy{}, nil, "", err
	}
	var legacy legacyContextPolicy
	if err := decodeStrictMigrationJSON(data, &legacy); err != nil {
		return tobari.ManifestPolicy{}, nil, "", err
	}
	if legacy.Name != "agent-ready" || legacy.Guardrail != migrationPolicyGuardrail {
		return tobari.ManifestPolicy{}, nil, "", fmt.Errorf("legacy policy identity is unsupported")
	}
	policy := legacy.current()
	policy.Name = "default"
	converted, err := tobari.ApplyNativeToolAuthReadiness(false, true, policy)
	if err != nil {
		return tobari.ManifestPolicy{}, nil, "", err
	}
	return tobari.NormalizeContextPolicy(converted)
}

func decodeStrictMigrationJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON contains trailing data")
	}
	return nil
}

func readPrivateMigrationFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() <= 0 || info.Size() > maximum {
		return nil, fmt.Errorf("file is not a bounded private regular file")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- caller supplies a fixed migration child.
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != info.Size() {
		return nil, fmt.Errorf("file changed while reading")
	}
	return data, nil
}

func migrationRuntimeSelection(binding tobari.RuntimeBinding) string {
	if binding.RuntimeID == tobari.StandardRuntimeID {
		return tobari.StandardRuntimeName
	}
	return fmt.Sprintf("%s@%d", binding.Name, binding.Ordinal)
}

func migrationRuntimeName(contextName, contextID string) string {
	candidate := "legacy-" + contextName
	if err := tobari.ValidateName(candidate); err == nil {
		return candidate
	}
	digest := sha256.Sum256([]byte(contextID))
	name := contextName
	if len(name) > 32 {
		name = name[:32]
	}
	return fmt.Sprintf("legacy-%s-%x", strings.TrimRight(name, "-"), digest[:4])
}

func (r *Runtime) prepareLegacyMigrationRuntime(ctx context.Context, contextName string, dockerfile []byte, diagnostics io.Writer) (tobari.RuntimeBinding, error) {
	manifestPath := r.contextManifestPath(contextName)
	raw, err := readPrivateMigrationFile(manifestPath, maxContextManifestBytes)
	if err != nil {
		return tobari.RuntimeBinding{}, fmt.Errorf("%w: re-read Context Runtime source: %v", tobari.ErrMigrationSourceChanged, err)
	}
	legacy, err := decodeLegacyContextManifest(raw, contextName)
	if err != nil {
		return tobari.RuntimeBinding{}, fmt.Errorf("%w: re-read Context Runtime source: %v", tobari.ErrMigrationSourceChanged, err)
	}
	name := migrationRuntimeName(contextName, legacy.ID)
	manifest, err := r.readRuntimeManifest(name)
	if errors.Is(err, tobari.ErrRuntimeNotFound) {
		if _, err := r.CreateRuntime(ctx, name, tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
			return tobari.RuntimeBinding{}, fmt.Errorf("%w: create managed Runtime: %v", tobari.ErrMigrationRuntimeFailed, err)
		}
		if err := writeAtomicBytes(filepath.Join(r.runtimeSourceDirectory(name), "Dockerfile"), dockerfile); err != nil {
			return tobari.RuntimeBinding{}, fmt.Errorf("%w: initialize managed Runtime source: %v", tobari.ErrMigrationRuntimeFailed, err)
		}
	} else if err != nil {
		return tobari.RuntimeBinding{}, fmt.Errorf("%w: inspect managed Runtime: %v", tobari.ErrMigrationRuntimeConflict, err)
	} else {
		if manifest.Kind != tobari.RuntimeKindManaged {
			return tobari.RuntimeBinding{}, tobari.ErrMigrationRuntimeConflict
		}
		entries, readErr := os.ReadDir(manifest.SourcePath)
		if readErr != nil || len(entries) != 1 || entries[0].Name() != "Dockerfile" || entries[0].IsDir() {
			return tobari.RuntimeBinding{}, tobari.ErrMigrationRuntimeConflict
		}
		existing, readErr := readPrivateMigrationFile(filepath.Join(manifest.SourcePath, "Dockerfile"), maxRuntimeSourceFile)
		if readErr != nil || !bytes.Equal(existing, dockerfile) {
			return tobari.RuntimeBinding{}, tobari.ErrMigrationRuntimeConflict
		}
	}
	report, err := r.buildManagedRuntimeLifecycleLocked(ctx, name, "", diagnostics)
	if err != nil {
		return tobari.RuntimeBinding{}, fmt.Errorf("%w: %v", tobari.ErrMigrationRuntimeFailed, err)
	}
	for index := len(report.Runtime.Revisions) - 1; index >= 0; index-- {
		revision := report.Runtime.Revisions[index]
		snapshot, readErr := readPrivateMigrationFile(filepath.Join(revision.SnapshotPath, "Dockerfile"), maxRuntimeSourceFile)
		if readErr == nil && bytes.Equal(snapshot, dockerfile) {
			return report.Runtime.Binding(revision.Ordinal)
		}
	}
	return tobari.RuntimeBinding{}, fmt.Errorf("%w: ready Runtime does not contain the exact legacy source", tobari.ErrMigrationRuntimeConflict)
}

func (r *Runtime) createMigrationBackup(plans []migrationContextPlan, researchAuthDigest string) (string, string, error) {
	hash := sha256.New()
	_, _ = hash.Write([]byte(researchAuthDigest))
	var names []string
	for _, plan := range plans {
		if !migrationPlanChanges(plan) {
			continue
		}
		names = append(names, plan.name)
		_, _ = hash.Write([]byte(plan.name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(plan.sourceManifest)
		_, _ = hash.Write(plan.sourcePolicy)
		_, _ = hash.Write(plan.sourceDockerfile)
	}
	digest := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	root := filepath.Join(r.configDirectory, "migrations", "pre-v1-"+strings.TrimPrefix(digest, "sha256:")[:12])
	if err := r.ensurePrivateDirectory(filepath.Dir(root)); err != nil {
		return "", "", err
	}
	if err := r.ensurePrivateDirectory(root); err != nil {
		return "", "", err
	}
	contextsRoot := filepath.Join(root, "contexts")
	if err := r.ensurePrivateDirectory(contextsRoot); err != nil {
		return "", "", err
	}
	for _, plan := range plans {
		if !migrationPlanChanges(plan) {
			continue
		}
		base := filepath.Join(contextsRoot, plan.name)
		if err := r.ensurePrivateDirectory(base); err != nil {
			return "", "", err
		}
		if err := initializeOrVerifyMigrationBytes(filepath.Join(base, "context.json"), plan.sourceManifest); err != nil {
			return "", "", err
		}
		if err := r.ensurePrivateDirectory(filepath.Join(base, "policy")); err != nil {
			return "", "", err
		}
		if err := initializeOrVerifyMigrationBytes(filepath.Join(base, "policy", "preset.json"), plan.sourcePolicy); err != nil {
			return "", "", err
		}
		if len(plan.sourceDockerfile) > 0 {
			if err := r.ensurePrivateDirectory(filepath.Join(base, "runtime")); err != nil {
				return "", "", err
			}
			if err := initializeOrVerifyMigrationBytes(filepath.Join(base, "runtime", "Dockerfile"), plan.sourceDockerfile); err != nil {
				return "", "", err
			}
		}
	}
	manifest := migrationBackupManifest{SchemaVersion: 1, Source: tobari.MigrationSourcePreV1ContextPolicyRuntime, Digest: digest, Contexts: names}
	path := filepath.Join(root, "backup.json")
	if data, err := readPrivateMigrationFile(path, maxContextManifestBytes); err == nil {
		var existing migrationBackupManifest
		if decodeErr := decodeStrictMigrationJSON(data, &existing); decodeErr != nil || existing.Digest != digest || !migrationEqualStrings(existing.Contexts, names) {
			return "", "", fmt.Errorf("migration backup identity mismatch")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := writeAtomicJSON(path, manifest); err != nil {
			return "", "", err
		}
	} else {
		return "", "", err
	}
	return root, digest, nil
}

func initializeOrVerifyMigrationBytes(path string, data []byte) error {
	if err := requirePrivateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if existing, err := readPrivateMigrationFile(path, int64(len(data))); err == nil {
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("migration backup content mismatch")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return initializeBytes(path, data, 0o600)
}

func migrationEqualStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (r *Runtime) commitMigrationPlans(plans []migrationContextPlan) error {
	return r.withContextStoreLock(func() error {
		for _, plan := range plans {
			if !migrationPlanChanges(plan) {
				continue
			}
			currentManifest, err := readPrivateMigrationFile(r.contextManifestPath(plan.name), maxContextManifestBytes)
			if err != nil || !bytes.Equal(currentManifest, plan.sourceManifest) {
				return fmt.Errorf("%w: Context %q manifest drifted", tobari.ErrMigrationSourceChanged, plan.name)
			}
			legacyPolicyPath := filepath.Join(r.contextPolicyDirectory(plan.name), "preset.json")
			currentPolicy, err := readPrivateMigrationFile(legacyPolicyPath, maxContextPolicyBytes)
			if err != nil || !bytes.Equal(currentPolicy, plan.sourcePolicy) {
				return fmt.Errorf("%w: Context %q policy drifted", tobari.ErrMigrationSourceChanged, plan.name)
			}
			if !plan.legacy {
				if err := removeMigrationFile(legacyPolicyPath); err != nil {
					return fmt.Errorf("%w: Context %q residual policy: %v", tobari.ErrMigrationWriteFailed, plan.name, err)
				}
				continue
			}
			if len(plan.sourceDockerfile) > 0 {
				currentDockerfile, readErr := readPrivateMigrationFile(filepath.Join(r.contextDirectory(plan.name), "runtime", "Dockerfile"), maxRuntimeSourceFile)
				if readErr != nil || !bytes.Equal(currentDockerfile, plan.sourceDockerfile) {
					return fmt.Errorf("%w: Context %q Runtime source drifted", tobari.ErrMigrationSourceChanged, plan.name)
				}
			}
			published, err := tobari.PublishWorkspaceManifest(plan.manifest, nil)
			if err != nil {
				return fmt.Errorf("%w: Context %q candidate: %v", tobari.ErrMigrationWriteFailed, plan.name, err)
			}
			if err := writeAtomicBytes(r.contextPolicyPath(plan.name), plan.policy); err != nil {
				return fmt.Errorf("%w: Context %q policy: %v", tobari.ErrMigrationWriteFailed, plan.name, err)
			}
			if err := r.retainWorkspaceManifestRevision(published); err != nil {
				return fmt.Errorf("%w: Manifest %q retained revision: %v", tobari.ErrMigrationWriteFailed, plan.name, err)
			}
			if err := writeAtomicJSON(r.contextManifestPath(plan.name), published); err != nil {
				return fmt.Errorf("%w: Context %q manifest: %v", tobari.ErrMigrationWriteFailed, plan.name, err)
			}
			if err := removeMigrationFile(legacyPolicyPath); err != nil {
				return fmt.Errorf("%w: Context %q residual policy: %v", tobari.ErrMigrationWriteFailed, plan.name, err)
			}
		}
		return nil
	})
}

func writeAtomicBytes(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".tobari-migration-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	parent, err := os.Open(directory) // #nosec G304 -- parent of a fixed runtime-owned path.
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

func removeMigrationFile(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	parent, err := os.Open(filepath.Dir(path)) // #nosec G304 -- parent of a validated fixed migration child.
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}
