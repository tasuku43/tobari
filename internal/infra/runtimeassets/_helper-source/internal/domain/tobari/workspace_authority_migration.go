package tobari

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const WorkspaceAuthorityMigrationSource = "workspace_manifest_v1"

// PredecessorTemplateBody is the exact complete predecessor Manifest body
// required by the one migration boundary. It is intentionally a distinct
// source type even though its typed components map losslessly into the final
// WorkspaceTemplateBody.
type PredecessorTemplateBody struct {
	Boundary         WorkspaceTemplateBoundary         `json:"boundary"`
	Policy           WorkspaceTemplatePolicyBody       `json:"policy"`
	EntryDefaults    WorkspaceTemplateEntryDefaults    `json:"entry_defaults"`
	SessionDefaults  WorkspaceTemplateSessionDefaults  `json:"session_defaults"`
	CreationDefaults WorkspaceTemplateCreationDefaults `json:"creation_defaults"`
}

func (b PredecessorTemplateBody) Validate() error {
	return b.WorkspaceTemplateBody().Validate()
}

func (b PredecessorTemplateBody) WorkspaceTemplateBody() WorkspaceTemplateBody {
	return WorkspaceTemplateBody{
		Boundary: b.Boundary.Clone(), Policy: b.Policy.Clone(), EntryDefaults: b.EntryDefaults,
		SessionDefaults: b.SessionDefaults.Clone(), CreationDefaults: b.CreationDefaults.Clone(),
	}
}

func predecessorTemplateBody(body WorkspaceTemplateBody) PredecessorTemplateBody {
	clone := body.Clone()
	return PredecessorTemplateBody{
		Boundary: clone.Boundary, Policy: clone.Policy, EntryDefaults: clone.EntryDefaults,
		SessionDefaults: clone.SessionDefaults, CreationDefaults: clone.CreationDefaults,
	}
}

type PredecessorTemplateRevision struct {
	Generation uint64                  `json:"generation"`
	Revision   SemanticDigest          `json:"manifest_revision"`
	Body       PredecessorTemplateBody `json:"body"`
}

func (r PredecessorTemplateRevision) Validate() error {
	if r.Generation == 0 || r.Revision.Validate() != nil {
		return fmt.Errorf("predecessor Manifest revision metadata is invalid")
	}
	return r.Body.Validate()
}

func (r PredecessorTemplateRevision) TemplateBody() WorkspaceTemplateBody {
	return r.Body.WorkspaceTemplateBody()
}

type PredecessorTemplate struct {
	ID                string                        `json:"workspace_manifest_id"`
	Name              string                        `json:"name"`
	CurrentGeneration uint64                        `json:"current_generation"`
	CurrentRevision   SemanticDigest                `json:"current_manifest_revision"`
	Revisions         []PredecessorTemplateRevision `json:"revisions"`
}

func (m PredecessorTemplate) Validate() error {
	if err := ValidateWorkspaceManifestID(m.ID); err != nil {
		return err
	}
	if err := ValidateName(m.Name); err != nil {
		return err
	}
	if m.Revisions == nil || len(m.Revisions) == 0 || m.CurrentGeneration == 0 || m.CurrentRevision.Validate() != nil {
		return fmt.Errorf("predecessor Manifest revision collection is invalid")
	}
	foundCurrent := false
	var boundary SemanticDigest
	var previousGeneration uint64
	contentByRevision := make(map[SemanticDigest]SemanticDigest, len(m.Revisions))
	for index, revision := range m.Revisions {
		if err := revision.Validate(); err != nil {
			return err
		}
		if index > 0 && revision.Generation <= previousGeneration {
			return fmt.Errorf("predecessor Manifest generations must increase")
		}
		if index > 0 && revision.Revision == m.Revisions[index-1].Revision {
			return fmt.Errorf("predecessor Manifest history contains a semantic no-op")
		}
		body := revision.TemplateBody()
		slices, err := body.slices()
		if err != nil {
			return err
		}
		bodyDigest, err := semanticIdentity(body)
		if err != nil {
			return err
		}
		if index == 0 {
			boundary = slices.BoundaryFingerprint
		} else if slices.BoundaryFingerprint != boundary {
			return fmt.Errorf("predecessor Manifest retained revisions cross Boundary")
		}
		if prior, exists := contentByRevision[revision.Revision]; exists && prior != bodyDigest {
			return fmt.Errorf("predecessor Manifest digest identifies different immutable bodies")
		}
		contentByRevision[revision.Revision] = bodyDigest
		if revision.Generation == m.CurrentGeneration && revision.Revision == m.CurrentRevision {
			foundCurrent = true
		}
		previousGeneration = revision.Generation
	}
	if !foundCurrent {
		return fmt.Errorf("predecessor Manifest current revision is not retained exactly")
	}
	return nil
}

type ContextIDAssignment struct {
	ProjectRoot           string    `json:"project_root"`
	PredecessorManifestID string    `json:"workspace_manifest_id"`
	ContextID             ContextID `json:"context_id"`
}

func (a ContextIDAssignment) Validate() error {
	if err := ValidateCanonicalRoot(a.ProjectRoot); err != nil {
		return err
	}
	if err := ValidateWorkspaceManifestID(a.PredecessorManifestID); err != nil {
		return err
	}
	return a.ContextID.Validate()
}

type PredecessorWorkspaceAppliedEntry struct {
	ManifestGeneration uint64         `json:"manifest_generation"`
	ManifestRevision   SemanticDigest `json:"manifest_revision"`
	RuntimeID          string         `json:"runtime_id"`
	RuntimeRevision    SemanticDigest `json:"runtime_revision"`
	ResolvedSpec       SemanticDigest `json:"resolved_spec_revision"`
	ReconciledAt       time.Time      `json:"reconciled_at"`
}

type PredecessorDockerObservationState string

const (
	PredecessorDockerObservationExactOwned PredecessorDockerObservationState = "exact_owned"
	PredecessorDockerObservationMissing    PredecessorDockerObservationState = "missing"
	PredecessorDockerObservationMismatched PredecessorDockerObservationState = "mismatched"
	PredecessorDockerObservationUnknown    PredecessorDockerObservationState = "unknown"
)

// PredecessorWorkspaceDockerObservation is bounded evidence supplied by the
// future infrastructure preflight. Only ExactOwned can retain an AppliedEntry;
// every other ordinary Docker outcome becomes explicit unverified state.
type PredecessorWorkspaceDockerObservation struct {
	State              PredecessorDockerObservationState `json:"state"`
	WorkspaceID        string                            `json:"workspace_id"`
	ManifestGeneration uint64                            `json:"manifest_generation,omitempty"`
	ManifestRevision   SemanticDigest                    `json:"manifest_revision,omitempty"`
	RuntimeID          string                            `json:"runtime_id,omitempty"`
	RuntimeRevision    SemanticDigest                    `json:"runtime_revision,omitempty"`
	ResolvedSpec       SemanticDigest                    `json:"resolved_spec_revision,omitempty"`
}

func (o PredecessorWorkspaceDockerObservation) Validate(workspaceID string, applied *PredecessorWorkspaceAppliedEntry) error {
	if ValidateWorkspaceID(o.WorkspaceID) != nil || o.WorkspaceID != workspaceID {
		return fmt.Errorf("predecessor Docker observation Workspace authority is invalid")
	}
	if o.State == PredecessorDockerObservationExactOwned {
		if applied == nil || o.ManifestGeneration != applied.ManifestGeneration || o.ManifestRevision != applied.ManifestRevision || o.RuntimeID != applied.RuntimeID || o.RuntimeRevision != applied.RuntimeRevision || o.ResolvedSpec != applied.ResolvedSpec {
			return fmt.Errorf("exact owned-Docker evidence does not bind the predecessor AppliedEntry")
		}
		return nil
	}
	if o.State != PredecessorDockerObservationMissing && o.State != PredecessorDockerObservationMismatched && o.State != PredecessorDockerObservationUnknown {
		return fmt.Errorf("predecessor Docker observation state is invalid")
	}
	if o.ManifestGeneration != 0 || o.ManifestRevision != "" || o.RuntimeID != "" || o.RuntimeRevision != "" || o.ResolvedSpec != "" {
		return fmt.Errorf("non-exact Docker observation cannot carry applied authority")
	}
	return nil
}

func (a PredecessorWorkspaceAppliedEntry) Validate() error {
	if a.ManifestGeneration == 0 || a.ManifestRevision.Validate() != nil || a.RuntimeRevision.Validate() != nil || a.ResolvedSpec.Validate() != nil {
		return fmt.Errorf("predecessor AppliedEntry authority is invalid")
	}
	if a.RuntimeID != StandardRuntimeID {
		if err := ValidateRuntimeID(a.RuntimeID); err != nil {
			return err
		}
	}
	if a.ReconciledAt.IsZero() || a.ReconciledAt.Location() != time.UTC {
		return fmt.Errorf("predecessor AppliedEntry time is invalid")
	}
	return nil
}

type PredecessorWorkspace struct {
	ID                  string                                `json:"workspace_id"`
	ProjectRoot         string                                `json:"project_root"`
	ManifestID          string                                `json:"workspace_manifest_id"`
	Home                string                                `json:"home"`
	HomeDigest          SemanticDigest                        `json:"home_digest"`
	CreationDefaults    SemanticDigest                        `json:"creation_defaults_digest"`
	LastSuccessfulEntry *PredecessorWorkspaceAppliedEntry     `json:"last_successful_entry,omitempty"`
	DockerObservation   PredecessorWorkspaceDockerObservation `json:"docker_observation"`
}

func (w PredecessorWorkspace) Validate() error {
	if err := ValidateWorkspaceID(w.ID); err != nil {
		return err
	}
	if err := ValidateCanonicalRoot(w.ProjectRoot); err != nil {
		return err
	}
	if err := ValidateWorkspaceManifestID(w.ManifestID); err != nil {
		return err
	}
	if w.Home == "" || !filepath.IsAbs(w.Home) || filepath.Clean(w.Home) != w.Home || w.HomeDigest.Validate() != nil || w.CreationDefaults.Validate() != nil {
		return fmt.Errorf("predecessor Workspace home or creation receipt is invalid")
	}
	if w.LastSuccessfulEntry != nil {
		if err := w.LastSuccessfulEntry.Validate(); err != nil {
			return err
		}
	}
	return w.DockerObservation.Validate(w.ID, w.LastSuccessfulEntry)
}

type PredecessorPolicyRule struct {
	ID       string               `json:"id"`
	Decision PolicyMemoryDecision `json:"decision"`
	Body     PolicyMemoryRuleBody `json:"body"`
}

func (r PredecessorPolicyRule) Validate() error {
	if err := ValidatePolicyRuleID(r.ID); err != nil {
		return err
	}
	if (r.Decision == PolicyMemoryAllow && !strings.HasPrefix(r.ID, "plr_")) || (r.Decision == PolicyMemoryDeny && !strings.HasPrefix(r.ID, "pdr_")) {
		return fmt.Errorf("predecessor policy rule ID and decision disagree")
	}
	return r.Body.Validate(r.Decision)
}

type PredecessorPolicySet struct {
	ManifestID  string                  `json:"workspace_manifest_id"`
	WorkspaceID string                  `json:"workspace_id"`
	ProjectRoot string                  `json:"project_root"`
	Rules       []PredecessorPolicyRule `json:"rules"`
}

func (s PredecessorPolicySet) Validate() error {
	if ValidateWorkspaceManifestID(s.ManifestID) != nil || ValidateWorkspaceID(s.WorkspaceID) != nil || ValidateCanonicalRoot(s.ProjectRoot) != nil || s.Rules == nil {
		return fmt.Errorf("predecessor policy owner is invalid")
	}
	seen := make(map[string]struct{}, len(s.Rules))
	for _, rule := range s.Rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		if _, exists := seen[rule.ID]; exists {
			return fmt.Errorf("predecessor policy rule is duplicated")
		}
		seen[rule.ID] = struct{}{}
	}
	return nil
}

type PredecessorPendingCandidate struct {
	ID            string                `json:"id"`
	ManifestID    string                `json:"workspace_manifest_id"`
	WorkspaceID   string                `json:"workspace_id"`
	ProjectRoot   string                `json:"project_root"`
	PayloadDigest SemanticDigest        `json:"payload_digest"`
	Effect        PolicyCandidateEffect `json:"effect"`
}

func (c PredecessorPendingCandidate) Validate() error {
	if ValidatePolicyCandidateID(c.ID) != nil || ValidateWorkspaceManifestID(c.ManifestID) != nil || ValidateWorkspaceID(c.WorkspaceID) != nil || ValidateCanonicalRoot(c.ProjectRoot) != nil || c.PayloadDigest.Validate() != nil {
		return fmt.Errorf("predecessor pending candidate is invalid")
	}
	payload, err := policyCandidateEffectDigest(c.Effect)
	if err != nil || payload != c.PayloadDigest {
		return fmt.Errorf("predecessor pending candidate payload does not bind its exact effect")
	}
	return nil
}

type ResearchAuthorityPlatform string

const (
	ResearchAuthorityMacOS ResearchAuthorityPlatform = "macos"
	ResearchAuthorityLinux ResearchAuthorityPlatform = "linux"
)

type PredecessorResearchAuthority struct {
	Present      bool                      `json:"present"`
	Complete     bool                      `json:"complete"`
	Platform     ResearchAuthorityPlatform `json:"platform,omitempty"`
	SourceDigest SemanticDigest            `json:"source_digest,omitempty"`
}

func (r PredecessorResearchAuthority) Validate() error {
	if !r.Present {
		if r.Complete || r.Platform != "" || r.SourceDigest != "" {
			return fmt.Errorf("absent research authority contains migration material")
		}
		return nil
	}
	if !r.Complete || (r.Platform != ResearchAuthorityMacOS && r.Platform != ResearchAuthorityLinux) || r.SourceDigest.Validate() != nil {
		return fmt.Errorf("research authority is incomplete or ambiguous")
	}
	return nil
}

type WorkspaceAuthorityMigrationInput struct {
	Source                string                        `json:"source"`
	SourceDigest          SemanticDigest                `json:"source_digest"`
	PredecessorComplete   bool                          `json:"predecessor_complete"`
	FinalAuthorityPresent bool                          `json:"final_authority_present"`
	ClusterStopped        bool                          `json:"cluster_stopped"`
	LiveAttachments       int                           `json:"live_attachments"`
	Templates             []PredecessorTemplate         `json:"workspace_manifests"`
	Workspaces            []PredecessorWorkspace        `json:"workspaces"`
	ContextAssignments    []ContextIDAssignment         `json:"context_assignments"`
	PolicySets            []PredecessorPolicySet        `json:"policy_sets"`
	PendingCandidates     []PredecessorPendingCandidate `json:"pending_candidates"`
	DefaultManifestID     *string                       `json:"default_workspace_manifest_id"`
	ResearchAuthority     PredecessorResearchAuthority  `json:"research_authority"`
}

type MigratedPendingCandidate struct {
	ID                   string                `json:"id"`
	PredecessorID        string                `json:"predecessor_id"`
	ContextID            ContextID             `json:"context_id"`
	ObservingWorkspaceID WorkspaceID           `json:"observing_workspace_id"`
	PayloadDigest        SemanticDigest        `json:"payload_digest"`
	Effect               PolicyCandidateEffect `json:"effect"`
}

func (c MigratedPendingCandidate) Validate() error {
	if ValidatePolicyCandidateID(c.PredecessorID) != nil || c.ContextID.Validate() != nil || c.ObservingWorkspaceID.Validate() != nil || c.PayloadDigest.Validate() != nil {
		return fmt.Errorf("migrated pending candidate authority is invalid")
	}
	if _, err := c.Authority(); err != nil {
		return fmt.Errorf("migrated pending candidate ID does not bind its final authority: %w", err)
	}
	return nil
}

func (c MigratedPendingCandidate) Authority() (PolicyCandidateAuthority, error) {
	authority := PolicyCandidateAuthority{
		ID: c.ID, ContextID: c.ContextID, ObservingWorkspaceID: c.ObservingWorkspaceID,
		PayloadDigest: c.PayloadDigest, Effect: c.Effect.Clone(),
	}
	return authority, authority.Validate()
}

type WorkspaceMigrationAdoption string

const (
	WorkspaceMigrationCurrent    WorkspaceMigrationAdoption = "current"
	WorkspaceMigrationPending    WorkspaceMigrationAdoption = "pending"
	WorkspaceMigrationUnverified WorkspaceMigrationAdoption = "unverified"
)

type MigratedWorkspace struct {
	Binding             WorkspaceBinding           `json:"binding"`
	Adoption            WorkspaceMigrationAdoption `json:"adoption"`
	PreservedHomeDigest SemanticDigest             `json:"preserved_home_digest"`
}

type ResearchQuarantinePlan struct {
	Required               bool                      `json:"required"`
	Platform               ResearchAuthorityPlatform `json:"platform,omitempty"`
	SourceDigest           SemanticDigest            `json:"source_digest,omitempty"`
	LeaveKeychainUntouched bool                      `json:"leave_keychain_untouched"`
	MoveFilesystemRootKey  bool                      `json:"move_filesystem_root_key"`
}

func (p ResearchQuarantinePlan) Validate() error {
	if !p.Required {
		if p.Platform != "" || p.SourceDigest != "" || p.LeaveKeychainUntouched || p.MoveFilesystemRootKey {
			return fmt.Errorf("unneeded research quarantine contains actions")
		}
		return nil
	}
	if p.SourceDigest.Validate() != nil {
		return fmt.Errorf("research quarantine source digest is invalid")
	}
	switch p.Platform {
	case ResearchAuthorityMacOS:
		if !p.LeaveKeychainUntouched || p.MoveFilesystemRootKey {
			return fmt.Errorf("macOS research quarantine changes Keychain recovery material")
		}
	case ResearchAuthorityLinux:
		if p.LeaveKeychainUntouched || !p.MoveFilesystemRootKey {
			return fmt.Errorf("Linux research quarantine omits filesystem root-key material")
		}
	default:
		return fmt.Errorf("research quarantine platform is invalid")
	}
	return nil
}

type WorkspaceAuthorityMigrationPlan struct {
	Source                  string                     `json:"source"`
	SourceDigest            SemanticDigest             `json:"source_digest"`
	Templates               []WorkspaceTemplate        `json:"workspace_templates"`
	Contexts                []ContextBinding           `json:"contexts"`
	PolicyMemories          []PolicyMemoryRevision     `json:"policy_memories"`
	Workspaces              []MigratedWorkspace        `json:"workspaces"`
	PendingCandidates       []MigratedPendingCandidate `json:"pending_candidates"`
	DefaultTemplateID       *WorkspaceTemplateID       `json:"default_workspace_template_id"`
	ContextAssignments      []ContextIDAssignment      `json:"context_assignments"`
	ResearchAuthDisposition ResearchAuthDisposition    `json:"research_auth_disposition"`
	ResearchQuarantine      ResearchQuarantinePlan     `json:"research_quarantine"`
	PlanDigest              SemanticDigest             `json:"plan_digest"`
}

func BuildWorkspaceAuthorityMigrationPlan(input WorkspaceAuthorityMigrationInput) (WorkspaceAuthorityMigrationPlan, error) {
	if err := validateWorkspaceAuthorityMigrationInput(input); err != nil {
		return WorkspaceAuthorityMigrationPlan{}, err
	}

	plan := WorkspaceAuthorityMigrationPlan{
		Source: input.Source, SourceDigest: input.SourceDigest,
		Templates:               make([]WorkspaceTemplate, 0, len(input.Templates)),
		Contexts:                make([]ContextBinding, 0, len(input.Workspaces)),
		PolicyMemories:          make([]PolicyMemoryRevision, 0, len(input.Workspaces)),
		Workspaces:              make([]MigratedWorkspace, 0, len(input.Workspaces)),
		PendingCandidates:       make([]MigratedPendingCandidate, 0, len(input.PendingCandidates)),
		ContextAssignments:      append([]ContextIDAssignment{}, input.ContextAssignments...),
		ResearchAuthDisposition: ResearchAuthNotPresent,
	}

	templateByID := make(map[string]WorkspaceTemplate, len(input.Templates))
	legacyRevisionMap := make(map[string]WorkspaceTemplateRevision)
	for _, predecessor := range input.Templates {
		id := WorkspaceTemplateID(predecessor.ID)
		retained := make([]WorkspaceTemplateRevision, 0, len(predecessor.Revisions))
		var current WorkspaceTemplateRevision
		for _, oldRevision := range predecessor.Revisions {
			revision, err := NewWorkspaceTemplateRevision(id, oldRevision.Generation, oldRevision.TemplateBody())
			if err != nil {
				return WorkspaceAuthorityMigrationPlan{}, err
			}
			retained = append(retained, revision)
			legacyKey := predecessor.ID + "\x00" + fmt.Sprint(oldRevision.Generation) + "\x00" + string(oldRevision.Revision)
			legacyRevisionMap[legacyKey] = revision
			if oldRevision.Generation == predecessor.CurrentGeneration && oldRevision.Revision == predecessor.CurrentRevision {
				current = revision
			}
		}
		template := WorkspaceTemplate{SchemaVersion: WorkspaceTemplateSchemaVersion, ID: id, Name: predecessor.Name, Current: current, Retained: retained}
		if err := template.Validate(); err != nil {
			return WorkspaceAuthorityMigrationPlan{}, err
		}
		plan.Templates = append(plan.Templates, template)
		templateByID[predecessor.ID] = template
	}

	assignmentByPair := make(map[string]ContextIDAssignment, len(input.ContextAssignments))
	for _, assignment := range input.ContextAssignments {
		assignmentByPair[assignment.ProjectRoot+"\x00"+assignment.PredecessorManifestID] = assignment
	}
	policyByWorkspace := make(map[string]PredecessorPolicySet, len(input.PolicySets))
	for _, policy := range input.PolicySets {
		policyByWorkspace[policy.WorkspaceID] = policy
	}

	contextByWorkspace := make(map[string]ContextBinding, len(input.Workspaces))
	for _, oldWorkspace := range input.Workspaces {
		assignment := assignmentByPair[oldWorkspace.ProjectRoot+"\x00"+oldWorkspace.ManifestID]
		context := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: assignment.ContextID, ProjectRoot: oldWorkspace.ProjectRoot, TemplateID: WorkspaceTemplateID(oldWorkspace.ManifestID)}
		plan.Contexts = append(plan.Contexts, context)
		contextByWorkspace[oldWorkspace.ID] = context

		oldRules := policyByWorkspace[oldWorkspace.ID].Rules
		rules := make([]PolicyMemoryRule, 0, len(oldRules))
		for _, oldRule := range oldRules {
			rule, err := NewPolicyMemoryRule(context.ID, oldRule.Decision, oldRule.Body)
			if err != nil {
				return WorkspaceAuthorityMigrationPlan{}, err
			}
			rules = append(rules, rule)
		}
		memory, _, err := PublishPolicyMemory(context.ID, rules, nil)
		if err != nil {
			return WorkspaceAuthorityMigrationPlan{}, err
		}
		plan.PolicyMemories = append(plan.PolicyMemories, memory)

		workspaceID := WorkspaceID(oldWorkspace.ID)
		template := templateByID[oldWorkspace.ManifestID]
		creationReceiptFound := false
		for _, revision := range template.Retained {
			if revision.Slices.CreationDefaultsDigest == oldWorkspace.CreationDefaults {
				creationReceiptFound = true
				break
			}
		}
		if !creationReceiptFound {
			return WorkspaceAuthorityMigrationPlan{}, fmt.Errorf("predecessor Workspace creation receipt has no exact retained Template body")
		}
		binding := WorkspaceBinding{SchemaVersion: WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: context.ID, ProjectRoot: context.ProjectRoot, Home: oldWorkspace.Home, CreationDefaults: oldWorkspace.CreationDefaults}
		adoption := WorkspaceMigrationUnverified
		if oldWorkspace.LastSuccessfulEntry != nil && oldWorkspace.DockerObservation.State == PredecessorDockerObservationExactOwned {
			oldApplied := oldWorkspace.LastSuccessfulEntry
			legacyKey := oldWorkspace.ManifestID + "\x00" + fmt.Sprint(oldApplied.ManifestGeneration) + "\x00" + string(oldApplied.ManifestRevision)
			mapped, exists := legacyRevisionMap[legacyKey]
			if !exists || oldApplied.RuntimeID != mapped.Slices.RuntimeID || oldApplied.RuntimeRevision != mapped.Slices.RuntimeRevision {
				return WorkspaceAuthorityMigrationPlan{}, fmt.Errorf("predecessor AppliedEntry does not match an exact retained Template revision")
			}
			binding.LastSuccessfulEntry = &WorkspaceAppliedEntry{
				ContextID: context.ID, TemplateID: context.TemplateID, TemplateRevision: mapped.Revision,
				EntrySliceDigest: mapped.Slices.EntrySliceDigest, RuntimeID: oldApplied.RuntimeID,
				RuntimeRevision: oldApplied.RuntimeRevision, ResolvedSpec: oldApplied.ResolvedSpec,
				ReconciledAt: oldApplied.ReconciledAt,
			}
			if mapped.Generation == templateByID[oldWorkspace.ManifestID].Current.Generation && mapped.Revision == templateByID[oldWorkspace.ManifestID].Current.Revision {
				adoption = WorkspaceMigrationCurrent
			} else {
				adoption = WorkspaceMigrationPending
			}
		}
		if err := binding.ValidateFor(context); err != nil {
			return WorkspaceAuthorityMigrationPlan{}, err
		}
		plan.Workspaces = append(plan.Workspaces, MigratedWorkspace{Binding: binding, Adoption: adoption, PreservedHomeDigest: oldWorkspace.HomeDigest})
	}

	for _, oldCandidate := range input.PendingCandidates {
		context := contextByWorkspace[oldCandidate.WorkspaceID]
		workspaceID := WorkspaceID(oldCandidate.WorkspaceID)
		candidate := MigratedPendingCandidate{
			ID: policyCandidateAuthorityID(context.ID, workspaceID, oldCandidate.PayloadDigest), PredecessorID: oldCandidate.ID,
			ContextID: context.ID, ObservingWorkspaceID: workspaceID, PayloadDigest: oldCandidate.PayloadDigest, Effect: oldCandidate.Effect.Clone(),
		}
		if err := candidate.Validate(); err != nil {
			return WorkspaceAuthorityMigrationPlan{}, err
		}
		plan.PendingCandidates = append(plan.PendingCandidates, candidate)
	}

	if input.DefaultManifestID != nil {
		id := WorkspaceTemplateID(*input.DefaultManifestID)
		plan.DefaultTemplateID = &id
	}
	if input.ResearchAuthority.Present {
		plan.ResearchAuthDisposition = ResearchAuthReauthenticationRequired
		plan.ResearchQuarantine = ResearchQuarantinePlan{Required: true, Platform: input.ResearchAuthority.Platform, SourceDigest: input.ResearchAuthority.SourceDigest, LeaveKeychainUntouched: input.ResearchAuthority.Platform == ResearchAuthorityMacOS, MoveFilesystemRootKey: input.ResearchAuthority.Platform == ResearchAuthorityLinux}
	}

	sort.Slice(plan.Templates, func(i, j int) bool { return plan.Templates[i].ID < plan.Templates[j].ID })
	sort.Slice(plan.Contexts, func(i, j int) bool { return plan.Contexts[i].ID < plan.Contexts[j].ID })
	sort.Slice(plan.PolicyMemories, func(i, j int) bool { return plan.PolicyMemories[i].ContextID < plan.PolicyMemories[j].ContextID })
	sort.Slice(plan.Workspaces, func(i, j int) bool { return plan.Workspaces[i].Binding.ID < plan.Workspaces[j].Binding.ID })
	sort.Slice(plan.PendingCandidates, func(i, j int) bool { return plan.PendingCandidates[i].ID < plan.PendingCandidates[j].ID })
	sort.Slice(plan.ContextAssignments, func(i, j int) bool {
		return plan.ContextAssignments[i].ContextID < plan.ContextAssignments[j].ContextID
	})

	digest, err := migrationPlanDigest(plan)
	if err != nil {
		return WorkspaceAuthorityMigrationPlan{}, err
	}
	plan.PlanDigest = digest
	return plan, plan.Validate()
}

func validateWorkspaceAuthorityMigrationInput(input WorkspaceAuthorityMigrationInput) error {
	if input.Source != WorkspaceAuthorityMigrationSource || input.SourceDigest.Validate() != nil || !input.PredecessorComplete || input.FinalAuthorityPresent || !input.ClusterStopped || input.LiveAttachments != 0 {
		return fmt.Errorf("Workspace authority migration preconditions are not satisfied")
	}
	if input.Templates == nil || len(input.Templates) == 0 || input.Workspaces == nil || input.ContextAssignments == nil || input.PolicySets == nil || input.PendingCandidates == nil {
		return fmt.Errorf("Workspace authority migration collections are incomplete")
	}
	if err := input.ResearchAuthority.Validate(); err != nil {
		return err
	}
	templates := make(map[string]struct{}, len(input.Templates))
	names := make(map[string]struct{}, len(input.Templates))
	for _, template := range input.Templates {
		if err := template.Validate(); err != nil {
			return err
		}
		if _, exists := templates[template.ID]; exists {
			return fmt.Errorf("predecessor Manifest ID is duplicated")
		}
		if _, exists := names[template.Name]; exists {
			return fmt.Errorf("predecessor Manifest name is duplicated")
		}
		templates[template.ID], names[template.Name] = struct{}{}, struct{}{}
	}
	if input.DefaultManifestID != nil {
		if _, exists := templates[*input.DefaultManifestID]; !exists {
			return fmt.Errorf("predecessor default Manifest is missing")
		}
	}

	workspaces := make(map[string]PredecessorWorkspace, len(input.Workspaces))
	pairs := make(map[string]struct{}, len(input.Workspaces))
	for _, workspace := range input.Workspaces {
		if err := workspace.Validate(); err != nil {
			return err
		}
		if _, exists := templates[workspace.ManifestID]; !exists {
			return fmt.Errorf("predecessor Workspace references an unknown Manifest")
		}
		pair := workspace.ProjectRoot + "\x00" + workspace.ManifestID
		if _, exists := workspaces[workspace.ID]; exists {
			return fmt.Errorf("predecessor Workspace ID is duplicated")
		}
		if _, exists := pairs[pair]; exists {
			return fmt.Errorf("predecessor Workspace pair is duplicated")
		}
		workspaces[workspace.ID], pairs[pair] = workspace, struct{}{}
	}

	assignments := make(map[string]struct{}, len(input.ContextAssignments))
	contextIDs := make(map[ContextID]struct{}, len(input.ContextAssignments))
	for _, assignment := range input.ContextAssignments {
		if err := assignment.Validate(); err != nil {
			return err
		}
		pair := assignment.ProjectRoot + "\x00" + assignment.PredecessorManifestID
		if _, exists := pairs[pair]; !exists {
			return fmt.Errorf("Context assignment has no exact predecessor Workspace")
		}
		if _, exists := assignments[pair]; exists {
			return fmt.Errorf("Context assignment is duplicated")
		}
		if _, exists := contextIDs[assignment.ContextID]; exists {
			return fmt.Errorf("Context assignment reuses an ID")
		}
		if _, exists := templates[string(assignment.ContextID)]; exists {
			return fmt.Errorf("fresh Context ID collides with preserved Template identity")
		}
		if _, exists := workspaces[string(assignment.ContextID)]; exists {
			return fmt.Errorf("fresh Context ID collides with preserved Workspace identity")
		}
		assignments[pair], contextIDs[assignment.ContextID] = struct{}{}, struct{}{}
	}
	if len(assignments) != len(pairs) {
		return fmt.Errorf("Context assignments do not cover every predecessor Workspace")
	}

	policyOwners := make(map[string]struct{}, len(input.PolicySets))
	for _, policy := range input.PolicySets {
		if err := policy.Validate(); err != nil {
			return err
		}
		workspace, exists := workspaces[policy.WorkspaceID]
		if !exists || workspace.ManifestID != policy.ManifestID || workspace.ProjectRoot != policy.ProjectRoot {
			return fmt.Errorf("predecessor policy set crosses Workspace authority")
		}
		if _, exists := policyOwners[policy.WorkspaceID]; exists {
			return fmt.Errorf("predecessor policy set owner is duplicated")
		}
		policyOwners[policy.WorkspaceID] = struct{}{}
	}
	candidates := make(map[string]struct{}, len(input.PendingCandidates))
	for _, candidate := range input.PendingCandidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
		workspace, exists := workspaces[candidate.WorkspaceID]
		if !exists || workspace.ManifestID != candidate.ManifestID || workspace.ProjectRoot != candidate.ProjectRoot {
			return fmt.Errorf("predecessor candidate crosses Workspace authority")
		}
		if _, exists := candidates[candidate.ID]; exists {
			return fmt.Errorf("predecessor candidate is duplicated")
		}
		candidates[candidate.ID] = struct{}{}
	}
	return nil
}

func migrationPlanDigest(plan WorkspaceAuthorityMigrationPlan) (SemanticDigest, error) {
	copy := plan.Clone()
	copy.PlanDigest = ""
	return semanticIdentity(copy)
}

func (p WorkspaceAuthorityMigrationPlan) Validate() error {
	if p.Source != WorkspaceAuthorityMigrationSource || p.SourceDigest.Validate() != nil || p.Templates == nil || p.Contexts == nil || p.PolicyMemories == nil || p.Workspaces == nil || p.PendingCandidates == nil || p.ContextAssignments == nil {
		return fmt.Errorf("Workspace authority migration plan is incomplete")
	}
	templateByID := make(map[WorkspaceTemplateID]WorkspaceTemplate, len(p.Templates))
	for _, template := range p.Templates {
		if err := template.Validate(); err != nil {
			return err
		}
		if _, exists := templateByID[template.ID]; exists {
			return fmt.Errorf("migration plan Template is duplicated")
		}
		templateByID[template.ID] = template
	}
	if err := ValidateContextBindings(p.Contexts); err != nil {
		return err
	}
	contextByID := make(map[ContextID]ContextBinding, len(p.Contexts))
	for _, context := range p.Contexts {
		if _, exists := templateByID[context.TemplateID]; !exists {
			return fmt.Errorf("Context has no Template")
		}
		contextByID[context.ID] = context
	}
	memoryOwners := make(map[ContextID]struct{}, len(p.PolicyMemories))
	for _, memory := range p.PolicyMemories {
		if err := memory.Validate(); err != nil {
			return err
		}
		if _, exists := contextByID[memory.ContextID]; !exists {
			return fmt.Errorf("Policy Memory has no Context")
		}
		if _, exists := memoryOwners[memory.ContextID]; exists {
			return fmt.Errorf("Context has more than one Policy Memory")
		}
		memoryOwners[memory.ContextID] = struct{}{}
	}
	if len(memoryOwners) != len(contextByID) {
		return fmt.Errorf("every Context requires one Policy Memory")
	}
	workspaceIDs := make(map[WorkspaceID]struct{}, len(p.Workspaces))
	workspaceContexts := make(map[ContextID]struct{}, len(p.Workspaces))
	workspaceByID := make(map[WorkspaceID]MigratedWorkspace, len(p.Workspaces))
	for _, workspace := range p.Workspaces {
		context, exists := contextByID[workspace.Binding.ContextID]
		if !exists || workspace.Binding.ValidateFor(context) != nil || workspace.PreservedHomeDigest.Validate() != nil {
			return fmt.Errorf("migrated Workspace is invalid")
		}
		if workspace.Adoption != WorkspaceMigrationCurrent && workspace.Adoption != WorkspaceMigrationPending && workspace.Adoption != WorkspaceMigrationUnverified {
			return fmt.Errorf("migrated Workspace adoption is invalid")
		}
		if (workspace.Binding.LastSuccessfulEntry == nil) != (workspace.Adoption == WorkspaceMigrationUnverified) {
			return fmt.Errorf("migrated Workspace adoption contradicts AppliedEntry")
		}
		if _, exists := workspaceIDs[workspace.Binding.ID]; exists {
			return fmt.Errorf("migrated Workspace is duplicated")
		}
		if _, exists := workspaceContexts[workspace.Binding.ContextID]; exists {
			return fmt.Errorf("Context has more than one Workspace")
		}
		template := templateByID[context.TemplateID]
		creationReceiptFound := false
		for _, revision := range template.Retained {
			if revision.Slices.CreationDefaultsDigest == workspace.Binding.CreationDefaults {
				creationReceiptFound = true
				break
			}
		}
		if !creationReceiptFound {
			return fmt.Errorf("migrated Workspace creation receipt has no exact retained Template body")
		}
		if workspace.Binding.LastSuccessfulEntry != nil {
			entry := workspace.Binding.LastSuccessfulEntry
			found := false
			for _, revision := range template.Retained {
				if entry.ValidateForRevision(context, revision) == nil {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("migrated AppliedEntry has no exact retained Template revision")
			}
			want := WorkspaceMigrationPending
			if entry.TemplateRevision == template.Current.Revision {
				want = WorkspaceMigrationCurrent
			}
			if workspace.Adoption != want {
				return fmt.Errorf("migrated Workspace adoption does not match Template current")
			}
		}
		workspaceIDs[workspace.Binding.ID] = struct{}{}
		workspaceContexts[workspace.Binding.ContextID] = struct{}{}
		workspaceByID[workspace.Binding.ID] = workspace
	}
	for _, candidate := range p.PendingCandidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
		workspace, exists := workspaceByID[candidate.ObservingWorkspaceID]
		if !exists || workspace.Binding.ContextID != candidate.ContextID {
			return fmt.Errorf("migrated pending candidate crosses Context or Workspace")
		}
	}
	if p.DefaultTemplateID != nil {
		found := false
		_, found = templateByID[*p.DefaultTemplateID]
		if !found {
			return fmt.Errorf("default Template is not present")
		}
	}
	if err := p.ResearchAuthDisposition.Validate(); err != nil {
		return err
	}
	if err := p.ResearchQuarantine.Validate(); err != nil {
		return err
	}
	if (p.ResearchAuthDisposition == ResearchAuthReauthenticationRequired) != p.ResearchQuarantine.Required {
		return fmt.Errorf("research authentication disposition is inconsistent")
	}
	assignmentByPair := make(map[string]ContextID, len(p.ContextAssignments))
	for _, assignment := range p.ContextAssignments {
		if err := assignment.Validate(); err != nil {
			return err
		}
		pair := assignment.ProjectRoot + "\x00" + assignment.PredecessorManifestID
		if _, exists := assignmentByPair[pair]; exists {
			return fmt.Errorf("migration plan Context assignment is duplicated")
		}
		context, exists := contextByID[assignment.ContextID]
		if !exists || context.ProjectRoot != assignment.ProjectRoot || string(context.TemplateID) != assignment.PredecessorManifestID {
			return fmt.Errorf("migration plan Context assignment does not match final Context")
		}
		assignmentByPair[pair] = assignment.ContextID
	}
	if len(assignmentByPair) != len(contextByID) {
		return fmt.Errorf("migration plan Context assignments are incomplete")
	}
	want, err := migrationPlanDigest(p)
	if err != nil {
		return err
	}
	if p.PlanDigest != want {
		return fmt.Errorf("migration plan digest does not bind the complete plan")
	}
	return nil
}

func (p WorkspaceAuthorityMigrationPlan) Clone() WorkspaceAuthorityMigrationPlan {
	result := p
	result.Templates = make([]WorkspaceTemplate, len(p.Templates))
	for index, template := range p.Templates {
		result.Templates[index] = template.Clone()
	}
	result.Contexts = append([]ContextBinding{}, p.Contexts...)
	result.PolicyMemories = make([]PolicyMemoryRevision, len(p.PolicyMemories))
	for index, memory := range p.PolicyMemories {
		result.PolicyMemories[index] = memory.Clone()
	}
	result.Workspaces = append([]MigratedWorkspace{}, p.Workspaces...)
	for index := range result.Workspaces {
		if p.Workspaces[index].Binding.LastSuccessfulEntry != nil {
			entry := *p.Workspaces[index].Binding.LastSuccessfulEntry
			result.Workspaces[index].Binding.LastSuccessfulEntry = &entry
		}
	}
	result.PendingCandidates = append([]MigratedPendingCandidate{}, p.PendingCandidates...)
	for index := range result.PendingCandidates {
		result.PendingCandidates[index].Effect = p.PendingCandidates[index].Effect.Clone()
	}
	result.ContextAssignments = append([]ContextIDAssignment{}, p.ContextAssignments...)
	if p.DefaultTemplateID != nil {
		id := *p.DefaultTemplateID
		result.DefaultTemplateID = &id
	}
	return result
}

type WorkspaceAuthorityRollbackObservation struct {
	JournalComplete                  bool           `json:"journal_complete"`
	BackupComplete                   bool           `json:"backup_complete"`
	JournalSourceDigest              SemanticDigest `json:"journal_source_digest"`
	BackupSourceDigest               SemanticDigest `json:"backup_source_digest"`
	FinalStateMatchesJournaledPlan   bool           `json:"final_state_matches_journaled_plan"`
	PredecessorCanonicalStatePresent bool           `json:"predecessor_canonical_state_present"`
	FreshCanonicalAuthStatePresent   bool           `json:"fresh_canonical_auth_state_present"`
}

type WorkspaceAuthorityRollbackReason string

const (
	WorkspaceAuthorityRollbackEligible        WorkspaceAuthorityRollbackReason = "eligible"
	WorkspaceAuthorityRollbackIncomplete      WorkspaceAuthorityRollbackReason = "incomplete_journal_or_backup"
	WorkspaceAuthorityRollbackDigestMismatch  WorkspaceAuthorityRollbackReason = "source_digest_mismatch"
	WorkspaceAuthorityRollbackFinalDrift      WorkspaceAuthorityRollbackReason = "final_state_drift"
	WorkspaceAuthorityRollbackSourceCollision WorkspaceAuthorityRollbackReason = "predecessor_state_collision"
	WorkspaceAuthorityRollbackAuthCollision   WorkspaceAuthorityRollbackReason = "fresh_auth_state_collision"
)

type WorkspaceAuthorityRollbackEligibility struct {
	Eligible bool                             `json:"eligible"`
	Reason   WorkspaceAuthorityRollbackReason `json:"reason"`
}

func EvaluateWorkspaceAuthorityRollback(observation WorkspaceAuthorityRollbackObservation) WorkspaceAuthorityRollbackEligibility {
	if !observation.JournalComplete || !observation.BackupComplete {
		return WorkspaceAuthorityRollbackEligibility{Reason: WorkspaceAuthorityRollbackIncomplete}
	}
	if observation.JournalSourceDigest.Validate() != nil || observation.BackupSourceDigest.Validate() != nil || observation.JournalSourceDigest != observation.BackupSourceDigest {
		return WorkspaceAuthorityRollbackEligibility{Reason: WorkspaceAuthorityRollbackDigestMismatch}
	}
	if !observation.FinalStateMatchesJournaledPlan {
		return WorkspaceAuthorityRollbackEligibility{Reason: WorkspaceAuthorityRollbackFinalDrift}
	}
	if observation.PredecessorCanonicalStatePresent {
		return WorkspaceAuthorityRollbackEligibility{Reason: WorkspaceAuthorityRollbackSourceCollision}
	}
	if observation.FreshCanonicalAuthStatePresent {
		return WorkspaceAuthorityRollbackEligibility{Reason: WorkspaceAuthorityRollbackAuthCollision}
	}
	return WorkspaceAuthorityRollbackEligibility{Eligible: true, Reason: WorkspaceAuthorityRollbackEligible}
}
