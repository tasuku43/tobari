package tobari

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
)

var (
	ErrNativeLoginBridgeUnavailable         = errors.New("native login bridge became unavailable")
	ErrConfiguratorTransientCleanupUnknown  = errors.New("configurator transient resource cleanup is incomplete")
	ErrConfiguratorTaskRetirementIncomplete = errors.New("configurator task retirement is incomplete")
)

// ConfiguratorAgent is the closed set of agent clients supported by the
// isolated configuration-authoring environment.
type ConfiguratorAgent string

const (
	ConfiguratorAgentCodex  ConfiguratorAgent = "codex"
	ConfiguratorAgentClaude ConfiguratorAgent = "claude"

	ConfiguratorDraftTargetKind    = "configuration-draft"
	ConfiguratorDraftTargetID      = "current-project"
	ProjectConfigurationTargetKind = "project-configuration"
	ProjectConfigurationTargetID   = "current-project"

	ConfiguratorSeedSchemaVersion          = 1
	ConfiguratorDraftSchemaVersion         = 1
	ConfiguratorSubmissionSchemaVersion    = 1
	ConfiguratorStageSchemaVersion         = 1
	ConfiguratorRuntimeSourceSchemaVersion = 1
)

// ConfiguratorTask is the closed internal editing task selected by a public
// assist command. Aggregate exists only to read and test pre-release retained
// implementation material; no public command selects it.
type ConfiguratorTask string

const (
	ConfiguratorTaskAggregate ConfiguratorTask = "aggregate"
	ConfiguratorTaskRuntime   ConfiguratorTask = "runtime"
	ConfiguratorTaskPolicy    ConfiguratorTask = "policy"
)

func (t ConfiguratorTask) Validate() error {
	switch t {
	case ConfiguratorTaskAggregate, ConfiguratorTaskRuntime, ConfiguratorTaskPolicy:
		return nil
	default:
		return fmt.Errorf("Configurator task is invalid")
	}
}

type ConfiguratorStage struct {
	SchemaVersion     int            `json:"schema_version"`
	TemplateRef       string         `json:"template_ref"`
	SourceRevision    SemanticDigest `json:"source_revision"`
	SourceFingerprint string         `json:"source_fingerprint"`
}

type ConfiguratorPendingStage struct {
	Submission     ConfiguratorSubmission `json:"submission"`
	Stage          ConfiguratorStage      `json:"stage"`
	PlanRef        string                 `json:"plan_ref,omitempty"`
	ApplyConfirmed bool                   `json:"apply_confirmed"`
}

func (p ConfiguratorPendingStage) Validate() error {
	if p.Submission.Draft.Purpose != ConfiguratorPurposeEvolve || p.Stage.ValidateFor(p.Submission) != nil {
		return fmt.Errorf("pending Configurator stage is invalid")
	}
	if p.PlanRef == "" {
		if p.ApplyConfirmed {
			return fmt.Errorf("pending Configurator stage confirms an absent Plan")
		}
		return nil
	}
	id, err := ParseWorkspaceTemplateChangePlanRef(p.PlanRef)
	if err != nil || id != p.Submission.Draft.TemplateID {
		return fmt.Errorf("pending Configurator stage Plan is invalid")
	}
	return nil
}

func (s ConfiguratorStage) ValidateFor(submission ConfiguratorSubmission) error {
	id, err := ParseWorkspaceTemplateRef(s.TemplateRef)
	if err != nil || s.SchemaVersion != ConfiguratorStageSchemaVersion || id != submission.Draft.TemplateID || s.SourceRevision != submission.SourceRevision || !validSourceFingerprint(s.SourceFingerprint) {
		return fmt.Errorf("Configurator stage does not bind the frozen submission")
	}
	return submission.Validate()
}

// ConfiguratorPurpose is an internal deterministic state. It is deliberately
// absent from the public CLI grammar and human chooser.
type ConfiguratorPurpose string

const (
	ConfiguratorPurposeBootstrap ConfiguratorPurpose = "bootstrap"
	ConfiguratorPurposeEvolve    ConfiguratorPurpose = "evolve"
)

var configuratorDraftIDPattern = regexp.MustCompile(`^cfg1_[0-9a-f]{64}$`)

func ParseConfiguratorAgent(value string) (ConfiguratorAgent, error) {
	agent := ConfiguratorAgent(value)
	if err := agent.Validate(); err != nil {
		return "", err
	}
	return agent, nil
}

func (a ConfiguratorAgent) Validate() error {
	switch a {
	case ConfiguratorAgentCodex, ConfiguratorAgentClaude:
		return nil
	default:
		return fmt.Errorf("Configurator agent is invalid")
	}
}

func (p ConfiguratorPurpose) Validate() error {
	switch p {
	case ConfiguratorPurposeBootstrap, ConfiguratorPurposeEvolve:
		return nil
	default:
		return fmt.Errorf("Configurator purpose is invalid")
	}
}

// ConfiguratorEvolutionSnapshot is the bounded read-only authority projection
// copied into a working Home. It contains no Workspace, Home path, active
// authority store, or credential state.
type ConfiguratorEvolutionSnapshot struct {
	SchemaVersion int                       `json:"schema_version"`
	Context       *ContextBinding           `json:"context,omitempty"`
	Template      WorkspaceTemplateRevision `json:"template"`
	PolicyMemory  *PolicyMemoryRevision     `json:"policy_memory,omitempty"`
}

func NewConfiguratorEvolutionSnapshot(snapshot ContextAuthoritySnapshot) (ConfiguratorEvolutionSnapshot, error) {
	if err := snapshot.Validate(); err != nil {
		return ConfiguratorEvolutionSnapshot{}, err
	}
	result := ConfiguratorEvolutionSnapshot{
		SchemaVersion: ConfiguratorSeedSchemaVersion,
		Context:       &snapshot.Context,
		Template:      snapshot.Template.Current.Clone(),
		PolicyMemory:  &snapshot.PolicyMemory,
	}
	return result, result.Validate()
}

func NewDetachedConfiguratorEvolutionSnapshot(template WorkspaceTemplateRevision) (ConfiguratorEvolutionSnapshot, error) {
	result := ConfiguratorEvolutionSnapshot{SchemaVersion: ConfiguratorSeedSchemaVersion, Template: template.Clone()}
	return result, result.Validate()
}

func (s ConfiguratorEvolutionSnapshot) Validate() error {
	if s.SchemaVersion != ConfiguratorSeedSchemaVersion || s.Template.Validate() != nil {
		return fmt.Errorf("Configurator evolution snapshot is invalid")
	}
	if (s.Context == nil) != (s.PolicyMemory == nil) {
		return fmt.Errorf("Configurator evolution snapshot has partial Context authority")
	}
	if s.Context != nil && (s.Context.Validate() != nil || s.PolicyMemory.Validate() != nil || s.Context.TemplateID != s.Template.TemplateID || s.Context.ID != s.PolicyMemory.ContextID) {
		return fmt.Errorf("Configurator evolution snapshot crosses authority owners")
	}
	return nil
}

func (s ConfiguratorEvolutionSnapshot) Clone() ConfiguratorEvolutionSnapshot {
	result := s
	result.Template = s.Template.Clone()
	if s.Context != nil {
		value := *s.Context
		result.Context = &value
	}
	if s.PolicyMemory != nil {
		value := s.PolicyMemory.Clone()
		result.PolicyMemory = &value
	}
	return result
}

// ConfiguratorSeed binds one authoring session to its complete starting facts.
// Bootstrap has no Context snapshot. Evolve always carries one exact snapshot.
type ConfiguratorSeed struct {
	SchemaVersion         int                            `json:"schema_version"`
	Task                  ConfiguratorTask               `json:"task"`
	Purpose               ConfiguratorPurpose            `json:"purpose"`
	ProjectRoot           string                         `json:"project_root,omitempty"`
	Initial               WorkspaceTemplateBody          `json:"initial"`
	ExecutionRuntime      RuntimeBinding                 `json:"execution_runtime"`
	TargetRuntimeID       string                         `json:"target_runtime_id,omitempty"`
	TargetRuntimeRevision SemanticDigest                 `json:"target_runtime_revision,omitempty"`
	Evolution             *ConfiguratorEvolutionSnapshot `json:"evolution,omitempty"`
}

func NewBootstrapConfiguratorSeed(projectRoot string, initial WorkspaceTemplateBody) (ConfiguratorSeed, error) {
	seed := ConfiguratorSeed{SchemaVersion: ConfiguratorSeedSchemaVersion, Task: ConfiguratorTaskAggregate, Purpose: ConfiguratorPurposeBootstrap, ProjectRoot: projectRoot, Initial: initial.Clone(), ExecutionRuntime: initial.EntryDefaults.Runtime}
	return seed, seed.Validate()
}

func NewEvolveConfiguratorSeed(projectRoot string, snapshot ContextAuthoritySnapshot) (ConfiguratorSeed, error) {
	evolution, err := NewConfiguratorEvolutionSnapshot(snapshot)
	if err != nil {
		return ConfiguratorSeed{}, err
	}
	if err := ValidateCanonicalRoot(projectRoot); err != nil {
		return ConfiguratorSeed{}, fmt.Errorf("aggregate Context authoring Project root is invalid: %w", err)
	}
	seed := ConfiguratorSeed{
		SchemaVersion:    ConfiguratorSeedSchemaVersion,
		Task:             ConfiguratorTaskAggregate,
		Purpose:          ConfiguratorPurposeEvolve,
		ProjectRoot:      projectRoot,
		Initial:          snapshot.Template.Current.Body.Clone(),
		ExecutionRuntime: snapshot.Template.Current.Body.EntryDefaults.Runtime,
		Evolution:        &evolution,
	}
	return seed, seed.Validate()
}

// NewDetachedEvolveConfiguratorSeed owns the later-use case where Tobari has
// a default Template but the selected Project does not yet have a Context.
// Authoring runs from the standard Runtime and updates that exact Template
// before the Context is created and the draft Home is adopted.
func NewDetachedEvolveConfiguratorSeed(projectRoot string, template WorkspaceTemplateRevision, standard RuntimeBinding) (ConfiguratorSeed, error) {
	evolution, err := NewDetachedConfiguratorEvolutionSnapshot(template)
	if err != nil {
		return ConfiguratorSeed{}, err
	}
	seed := ConfiguratorSeed{
		SchemaVersion:    ConfiguratorSeedSchemaVersion,
		Task:             ConfiguratorTaskAggregate,
		Purpose:          ConfiguratorPurposeEvolve,
		ProjectRoot:      projectRoot,
		Initial:          template.Body.Clone(),
		ExecutionRuntime: standard,
		Evolution:        &evolution,
	}
	return seed, seed.Validate()
}

func NewRuntimeAssistConfiguratorSeed(execution RuntimeBinding, runtimeID string, sourceRevision SemanticDigest) (ConfiguratorSeed, error) {
	seed := ConfiguratorSeed{
		SchemaVersion: ConfiguratorSeedSchemaVersion, Task: ConfiguratorTaskRuntime,
		Purpose: ConfiguratorPurposeEvolve, ExecutionRuntime: execution,
		TargetRuntimeID: runtimeID, TargetRuntimeRevision: sourceRevision,
	}
	return seed, seed.Validate()
}

func NewPolicyAssistConfiguratorSeed(snapshot ContextAuthoritySnapshot) (ConfiguratorSeed, error) {
	evolution, err := NewConfiguratorEvolutionSnapshot(snapshot)
	if err != nil {
		return ConfiguratorSeed{}, err
	}
	seed := ConfiguratorSeed{
		SchemaVersion: ConfiguratorSeedSchemaVersion, Task: ConfiguratorTaskPolicy,
		Purpose: ConfiguratorPurposeEvolve,
		Initial: snapshot.Template.Current.Body.Clone(), ExecutionRuntime: snapshot.Template.Current.Body.EntryDefaults.Runtime,
		Evolution: &evolution,
	}
	return seed, seed.Validate()
}

func (s ConfiguratorSeed) Validate() error {
	if s.SchemaVersion != ConfiguratorSeedSchemaVersion || s.Task.Validate() != nil || s.Purpose.Validate() != nil || s.ExecutionRuntime.Validate() != nil {
		return fmt.Errorf("Configurator seed is invalid")
	}
	if s.Task == ConfiguratorTaskRuntime {
		if s.Purpose != ConfiguratorPurposeEvolve || s.ProjectRoot != "" || !reflect.DeepEqual(s.Initial, WorkspaceTemplateBody{}) || !isCanonicalStandardConfiguratorRuntime(s.ExecutionRuntime) || s.Evolution != nil || ValidateRuntimeID(s.TargetRuntimeID) != nil || s.TargetRuntimeID == StandardRuntimeID || s.TargetRuntimeRevision.Validate() != nil {
			return fmt.Errorf("Runtime assist Configurator seed is invalid")
		}
		return nil
	}
	if s.Task == ConfiguratorTaskPolicy {
		if s.Purpose != ConfiguratorPurposeEvolve || s.ProjectRoot != "" || s.Initial.Validate() != nil || s.Evolution == nil || s.Evolution.Context == nil || s.Evolution.PolicyMemory == nil || s.ExecutionRuntime != s.Evolution.Template.Body.EntryDefaults.Runtime || s.TargetRuntimeID != "" || s.TargetRuntimeRevision != "" {
			return fmt.Errorf("policy assist Configurator seed lacks exact Context authority")
		}
		return nil
	}
	if ValidateCanonicalRoot(s.ProjectRoot) != nil || s.Initial.Validate() != nil {
		return fmt.Errorf("Configurator seed Project authority is invalid")
	}
	if s.TargetRuntimeID != "" || s.TargetRuntimeRevision != "" {
		return fmt.Errorf("aggregate Configurator seed carries a task target")
	}
	switch s.Purpose {
	case ConfiguratorPurposeBootstrap:
		if s.Evolution != nil || !isCanonicalStandardConfiguratorRuntime(s.ExecutionRuntime) || !reflect.DeepEqual(s.ExecutionRuntime, s.Initial.EntryDefaults.Runtime) {
			return fmt.Errorf("bootstrap Configurator seed carries existing authority")
		}
	case ConfiguratorPurposeEvolve:
		if s.Evolution == nil || s.Evolution.Validate() != nil || !reflect.DeepEqual(s.Evolution.Template.Body, s.Initial) {
			return fmt.Errorf("evolve Configurator seed is inconsistent")
		}
		if s.Evolution.Context == nil && !isCanonicalStandardConfiguratorRuntime(s.ExecutionRuntime) {
			return fmt.Errorf("detached Configurator must use the canonical standard Runtime")
		}
		if s.Evolution.Context != nil && s.ExecutionRuntime != s.Evolution.Template.Body.EntryDefaults.Runtime {
			return fmt.Errorf("Context Configurator must use the exact selected Runtime")
		}
	}
	return nil
}

func isCanonicalStandardConfiguratorRuntime(binding RuntimeBinding) bool {
	return binding.RuntimeID == StandardRuntimeID && binding.Name == StandardRuntimeName && binding.Ordinal == 1
}

func (s ConfiguratorSeed) Clone() ConfiguratorSeed {
	result := s
	if s.Task != ConfiguratorTaskRuntime {
		result.Initial = s.Initial.Clone()
	}
	if s.Evolution != nil {
		value := s.Evolution.Clone()
		result.Evolution = &value
	}
	return result
}

func (s ConfiguratorSeed) Runtime() RuntimeBinding { return s.ExecutionRuntime }

// ConfiguratorScopeKey is an internal persistence and attachment scope. It is
// derived only from validated task authority and never from ambient CWD.
func (s ConfiguratorSeed) ConfiguratorScopeKey() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	switch s.Task {
	case ConfiguratorTaskRuntime:
		return "runtime-" + s.TargetRuntimeID, nil
	case ConfiguratorTaskPolicy:
		return "context-" + string(s.Evolution.Context.ID), nil
	case ConfiguratorTaskAggregate:
		return "project-" + s.ProjectRoot, nil
	default:
		return "", fmt.Errorf("Configurator task scope is invalid")
	}
}

func ConfiguratorDraftID(seed ConfiguratorSeed, agent ConfiguratorAgent) (string, error) {
	if err := seed.Validate(); err != nil {
		return "", err
	}
	if err := agent.Validate(); err != nil {
		return "", err
	}
	version := "tobari-configurator-v3"
	if seed.Task == ConfiguratorTaskPolicy {
		version = "tobari-configurator-v4"
	} else if seed.Task == ConfiguratorTaskRuntime {
		version = "tobari-configurator-v5-runtime-installation"
	}
	base := version + "\x00" + seed.ProjectRoot + "\x00" + string(agent) + "\x00" + string(seed.Task) + "\x00" + string(seed.Purpose) + "\x00" + seed.Runtime().RuntimeID + "\x00" + seed.Runtime().Revision + "\x00" + seed.TargetRuntimeID
	if seed.Task != ConfiguratorTaskAggregate {
		base += "\x00" + string(seed.TargetRuntimeRevision)
	}
	if seed.Evolution != nil {
		base += "\x00" + string(seed.Evolution.Template.Revision)
		if seed.Evolution.Context != nil && seed.Evolution.PolicyMemory != nil {
			base += "\x00" + string(seed.Evolution.Context.ID) + "\x00" + string(seed.Evolution.PolicyMemory.Revision)
		}
	}
	digest := sha256.Sum256([]byte(base))
	return "cfg1_" + hex.EncodeToString(digest[:]), nil
}

// ConfiguratorDraft identifies persistent desired configuration. Its working
// copy lives below its managed Home and is never live authority.
type ConfiguratorDraft struct {
	SchemaVersion            int                 `json:"schema_version"`
	ID                       string              `json:"id"`
	ProjectRoot              string              `json:"project_root,omitempty"`
	Agent                    ConfiguratorAgent   `json:"agent"`
	Task                     ConfiguratorTask    `json:"task"`
	Purpose                  ConfiguratorPurpose `json:"purpose"`
	TemplateID               WorkspaceTemplateID `json:"workspace_template_id,omitempty"`
	ContextID                ContextID           `json:"context_id,omitempty"`
	AdoptionContextID        ContextID           `json:"adoption_context_id,omitempty"`
	BaseTemplateRevision     SemanticDigest      `json:"base_template_revision,omitempty"`
	BasePolicyMemoryRevision SemanticDigest      `json:"base_policy_memory_revision,omitempty"`
	Runtime                  RuntimeBinding      `json:"runtime"`
	TargetRuntimeID          string              `json:"target_runtime_id,omitempty"`
	TargetRuntimeRevision    SemanticDigest      `json:"target_runtime_revision,omitempty"`
}

func NewConfiguratorDraft(seed ConfiguratorSeed, agent ConfiguratorAgent, templateID WorkspaceTemplateID, adoptionContextIDs ...ContextID) (ConfiguratorDraft, error) {
	id, err := ConfiguratorDraftID(seed, agent)
	if err != nil {
		return ConfiguratorDraft{}, err
	}
	draft := ConfiguratorDraft{
		SchemaVersion: ConfiguratorDraftSchemaVersion,
		ID:            id, ProjectRoot: seed.ProjectRoot, Agent: agent, Task: seed.Task, Purpose: seed.Purpose,
		TemplateID: templateID, Runtime: seed.Runtime(), TargetRuntimeID: seed.TargetRuntimeID, TargetRuntimeRevision: seed.TargetRuntimeRevision,
	}
	if seed.Evolution != nil {
		if seed.Evolution.Context != nil {
			draft.ContextID = seed.Evolution.Context.ID
		}
		draft.BaseTemplateRevision = seed.Evolution.Template.Revision
		if seed.Evolution.PolicyMemory != nil {
			draft.BasePolicyMemoryRevision = seed.Evolution.PolicyMemory.Revision
		}
	}
	if len(adoptionContextIDs) > 1 {
		return ConfiguratorDraft{}, fmt.Errorf("Configurator draft has ambiguous adoption Context authority")
	}
	if len(adoptionContextIDs) == 1 {
		draft.AdoptionContextID = adoptionContextIDs[0]
	}
	return draft, draft.Validate()
}

func (d ConfiguratorDraft) Validate() error {
	if d.SchemaVersion != ConfiguratorDraftSchemaVersion || !configuratorDraftIDPattern.MatchString(d.ID) || d.Agent.Validate() != nil || d.Task.Validate() != nil || d.Purpose.Validate() != nil || d.Runtime.Validate() != nil {
		return fmt.Errorf("Configurator draft identity is invalid")
	}
	if d.Task == ConfiguratorTaskRuntime {
		if d.Purpose != ConfiguratorPurposeEvolve || d.ProjectRoot != "" || d.TemplateID != "" || d.ContextID != "" || d.AdoptionContextID != "" || d.BaseTemplateRevision != "" || d.BasePolicyMemoryRevision != "" || !isCanonicalStandardConfiguratorRuntime(d.Runtime) || ValidateRuntimeID(d.TargetRuntimeID) != nil || d.TargetRuntimeID == StandardRuntimeID || d.TargetRuntimeRevision.Validate() != nil {
			return fmt.Errorf("Runtime assist Configurator draft is invalid")
		}
		return nil
	}
	if d.Task == ConfiguratorTaskPolicy {
		if d.Purpose != ConfiguratorPurposeEvolve || d.ProjectRoot != "" || d.TemplateID.Validate() != nil || d.ContextID.Validate() != nil || d.AdoptionContextID != "" || d.BaseTemplateRevision.Validate() != nil || d.BasePolicyMemoryRevision.Validate() != nil || d.TargetRuntimeID != "" || d.TargetRuntimeRevision != "" {
			return fmt.Errorf("policy assist Configurator draft lacks exact Context authority")
		}
		return nil
	}
	if ValidateCanonicalRoot(d.ProjectRoot) != nil || d.TemplateID.Validate() != nil {
		return fmt.Errorf("Configurator draft Project authority is invalid")
	}
	if d.TargetRuntimeID != "" || d.TargetRuntimeRevision != "" {
		return fmt.Errorf("aggregate Configurator draft carries a task target")
	}
	switch d.Purpose {
	case ConfiguratorPurposeBootstrap:
		if d.ContextID != "" || d.AdoptionContextID.Validate() != nil || d.BaseTemplateRevision != "" || d.BasePolicyMemoryRevision != "" || !isCanonicalStandardConfiguratorRuntime(d.Runtime) {
			return fmt.Errorf("bootstrap Configurator draft carries existing authority")
		}
	case ConfiguratorPurposeEvolve:
		if d.BaseTemplateRevision.Validate() != nil {
			return fmt.Errorf("evolve Configurator draft lacks base authority")
		}
		if d.ContextID == "" {
			if d.AdoptionContextID.Validate() != nil || d.BasePolicyMemoryRevision != "" || !isCanonicalStandardConfiguratorRuntime(d.Runtime) {
				return fmt.Errorf("detached evolve Configurator draft is invalid")
			}
		} else if d.AdoptionContextID != "" || d.ContextID.Validate() != nil || d.BasePolicyMemoryRevision.Validate() != nil {
			return fmt.Errorf("Context evolve Configurator draft lacks base authority")
		}
	}
	return nil
}

// ConfiguratorSubmission is the host-frozen, typed result reviewed after the
// container exits. Apply consumes this value, never the mutable Home again.
type ConfiguratorSubmission struct {
	SchemaVersion  int                        `json:"schema_version"`
	Draft          ConfiguratorDraft          `json:"draft"`
	Body           WorkspaceTemplateBody      `json:"body"`
	Revision       SemanticDigest             `json:"revision"`
	SourceRevision SemanticDigest             `json:"source_revision"`
	RuntimeSource  *ConfiguratorRuntimeSource `json:"runtime_source,omitempty"`
}

type ConfiguratorRuntimeSource struct {
	SchemaVersion  int            `json:"schema_version"`
	RuntimeID      string         `json:"runtime_id"`
	BaseRevision   SemanticDigest `json:"base_revision"`
	FrozenRevision SemanticDigest `json:"frozen_revision"`
	Changed        bool           `json:"changed"`
}

func (s ConfiguratorRuntimeSource) ValidateFor(draft ConfiguratorDraft) error {
	targetRuntimeID := draft.Runtime.RuntimeID
	if draft.Task == ConfiguratorTaskRuntime {
		targetRuntimeID = draft.TargetRuntimeID
	}
	if s.SchemaVersion != ConfiguratorRuntimeSourceSchemaVersion || ValidateRuntimeID(s.RuntimeID) != nil || s.RuntimeID == StandardRuntimeID || s.RuntimeID != targetRuntimeID || s.BaseRevision.Validate() != nil || s.FrozenRevision.Validate() != nil || s.Changed != (s.BaseRevision != s.FrozenRevision) {
		return fmt.Errorf("Configurator Runtime source receipt is invalid")
	}
	if draft.Task == ConfiguratorTaskRuntime && s.BaseRevision != draft.TargetRuntimeRevision {
		return fmt.Errorf("Runtime assist source does not match its observed target generation")
	}
	return draft.Validate()
}

func NewConfiguratorSubmission(draft ConfiguratorDraft, body WorkspaceTemplateBody, sourceRevision ...SemanticDigest) (ConfiguratorSubmission, error) {
	result := ConfiguratorSubmission{SchemaVersion: ConfiguratorSubmissionSchemaVersion, Draft: draft}
	if draft.Validate() != nil || (draft.Task == ConfiguratorTaskRuntime && !reflect.DeepEqual(body, WorkspaceTemplateBody{})) || (draft.Task != ConfiguratorTaskRuntime && body.Validate() != nil) {
		return ConfiguratorSubmission{}, fmt.Errorf("Configurator submission input is invalid")
	}
	if draft.Task != ConfiguratorTaskRuntime {
		result.Body = body.Clone()
	}
	revision, err := configuratorSubmissionRevision(draft, body, nil)
	if err != nil {
		return ConfiguratorSubmission{}, err
	}
	result.Revision = revision
	if draft.Task == ConfiguratorTaskRuntime {
		if len(sourceRevision) != 0 {
			return ConfiguratorSubmission{}, fmt.Errorf("Runtime assistance submission source is task-owned")
		}
		result.SourceRevision = draft.TargetRuntimeRevision
	} else if len(sourceRevision) == 1 {
		result.SourceRevision = sourceRevision[0]
	} else if len(sourceRevision) == 0 {
		result.SourceRevision, err = semanticIdentity(body.Clone())
	} else {
		return ConfiguratorSubmission{}, fmt.Errorf("Configurator submission has ambiguous source revision")
	}
	if result.SourceRevision.Validate() != nil {
		return ConfiguratorSubmission{}, fmt.Errorf("Configurator submission source revision is invalid")
	}
	return result, result.Validate()
}

func (s ConfiguratorSubmission) Validate() error {
	if s.SchemaVersion != ConfiguratorSubmissionSchemaVersion || s.Draft.Validate() != nil || s.Revision.Validate() != nil || s.SourceRevision.Validate() != nil {
		return fmt.Errorf("Configurator submission is invalid")
	}
	if s.Draft.Task == ConfiguratorTaskRuntime {
		if !reflect.DeepEqual(s.Body, WorkspaceTemplateBody{}) {
			return fmt.Errorf("Runtime assistance submission carries Template authority")
		}
	} else if s.Body.Validate() != nil {
		return fmt.Errorf("Configurator submission is invalid")
	}
	want, err := configuratorSubmissionRevision(s.Draft, s.Body, s.RuntimeSource)
	if err != nil || want != s.Revision {
		return fmt.Errorf("Configurator submission revision changed")
	}
	if s.Draft.Task == ConfiguratorTaskRuntime {
		wantSource := s.Draft.TargetRuntimeRevision
		if s.RuntimeSource != nil {
			wantSource = s.RuntimeSource.FrozenRevision
		}
		if s.SourceRevision != wantSource {
			return fmt.Errorf("Runtime assistance submission source revision changed")
		}
	} else {
		wantSource, err := semanticIdentity(s.Body.Clone())
		if err != nil || wantSource != s.SourceRevision {
			return fmt.Errorf("Configurator submission source revision changed")
		}
	}
	if s.RuntimeSource != nil && s.RuntimeSource.ValidateFor(s.Draft) != nil {
		return fmt.Errorf("Configurator submission Runtime source changed")
	}
	return nil
}

func (d ConfiguratorDraft) NeedsHomeAdoption() bool { return d.AdoptionContextID != "" }

func (d ConfiguratorDraft) ConfiguratorScopeKey() (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	switch d.Task {
	case ConfiguratorTaskRuntime:
		return "runtime-" + d.TargetRuntimeID, nil
	case ConfiguratorTaskPolicy:
		return "context-" + string(d.ContextID), nil
	case ConfiguratorTaskAggregate:
		return "project-" + d.ProjectRoot, nil
	default:
		return "", fmt.Errorf("Configurator task scope is invalid")
	}
}

func (d ConfiguratorDraft) UsesInstallationHome() bool {
	return d.Task == ConfiguratorTaskRuntime
}

func NewConfiguratorSubmissionWithoutValidation(draft ConfiguratorDraft, body WorkspaceTemplateBody) (SemanticDigest, error) {
	return semanticIdentity(struct {
		Draft ConfiguratorDraft
		Body  WorkspaceTemplateBody
	}{draft, body.Clone()})
}

func (s ConfiguratorSubmission) WithRuntimeSource(source ConfiguratorRuntimeSource) (ConfiguratorSubmission, error) {
	if err := s.Validate(); err != nil || source.ValidateFor(s.Draft) != nil || s.RuntimeSource != nil {
		return ConfiguratorSubmission{}, fmt.Errorf("Configurator Runtime source cannot be attached: %w", err)
	}
	result := s
	result.RuntimeSource = &source
	if result.Draft.Task == ConfiguratorTaskRuntime {
		result.SourceRevision = source.FrozenRevision
	}
	revision, err := configuratorSubmissionRevision(result.Draft, result.Body, result.RuntimeSource)
	if err != nil {
		return ConfiguratorSubmission{}, err
	}
	result.Revision = revision
	return result, result.Validate()
}

func (s ConfiguratorSubmission) WithAppliedRuntime(binding RuntimeBinding) (ConfiguratorSubmission, error) {
	if err := s.Validate(); err != nil {
		return ConfiguratorSubmission{}, fmt.Errorf("Configurator built Runtime does not match its frozen source: %w", err)
	}
	if s.RuntimeSource == nil || !s.RuntimeSource.Changed || binding.Validate() != nil || binding.RuntimeID != s.RuntimeSource.RuntimeID || binding.Revision != string(s.RuntimeSource.FrozenRevision) {
		return ConfiguratorSubmission{}, fmt.Errorf("Configurator built Runtime does not match its frozen source")
	}
	result := s
	result.Body = s.Body.Clone()
	result.Body.EntryDefaults.Runtime = binding
	result.SourceRevision, _ = semanticIdentity(result.Body.Clone())
	revision, err := configuratorSubmissionRevision(result.Draft, result.Body, result.RuntimeSource)
	if err != nil {
		return ConfiguratorSubmission{}, err
	}
	result.Revision = revision
	return result, result.Validate()
}

func configuratorSubmissionRevision(draft ConfiguratorDraft, body WorkspaceTemplateBody, runtime *ConfiguratorRuntimeSource) (SemanticDigest, error) {
	return semanticIdentity(struct {
		Draft   ConfiguratorDraft
		Body    WorkspaceTemplateBody
		Runtime *ConfiguratorRuntimeSource
	}{draft, body.Clone(), runtime})
}

// ConfiguratorIsolation is the executable trust-boundary contract. Direct
// egress is accepted only with one whole managed Home and no ambient host
// authority.
type ConfiguratorIsolation struct {
	DirectEgress         bool
	ManagedHomeReadWrite bool
	ProjectMounted       bool
	HostHomeMounted      bool
	OtherContextMounted  bool
	DockerSocketMounted  bool
	AuthorityMounted     bool
}

func DirectEgressConfiguratorIsolation() ConfiguratorIsolation {
	return ConfiguratorIsolation{DirectEgress: true, ManagedHomeReadWrite: true}
}

func (i ConfiguratorIsolation) Validate() error {
	if !i.DirectEgress || !i.ManagedHomeReadWrite || i.ProjectMounted || i.HostHomeMounted || i.OtherContextMounted || i.DockerSocketMounted || i.AuthorityMounted {
		return fmt.Errorf("Configurator isolation boundary is invalid")
	}
	return nil
}

func ConfiguratorWorkingDirectory(draft ConfiguratorDraft) (string, error) {
	if err := draft.Validate(); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(".tobari", "configurator", draft.ID, "working")), nil
}

// ConfiguratorSourceDirectory is the closed resource-source tree below the
// attachment directory. Guidance stays beside it so source validation never
// has to accept agent-owned auxiliary files.
func ConfiguratorSourceDirectory(draft ConfiguratorDraft) (string, error) {
	working, err := ConfiguratorWorkingDirectory(draft)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(working, "configuration")), nil
}
