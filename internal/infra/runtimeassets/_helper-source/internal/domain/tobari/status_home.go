package tobari

import "fmt"

const (
	StatusHomeSchemaVersion = 3
	TaskStatusHome          = "status"
	// StatusHomeDockerCallCeiling permits one six-call coherent observation
	// attempt and one bounded retry after authority/anchor churn.
	StatusHomeDockerCallsPerAttempt = 6
	StatusHomeDockerCallCeiling     = 12
)

const (
	statusHomePathEntry                 = "tobari"
	statusHomePathTemplateList          = "template list"
	statusHomePathClusterStatus         = "cluster status"
	statusHomePathClusterUp             = "cluster up"
	statusHomePathDoctor                = "doctor"
	statusHomePathReviewRuntimes        = "review runtimes"
	statusHomePathReviewPermissions     = "review permissions"
	statusHomePathReviewServices        = "review services"
	statusHomePathHelpContextDelete     = "help context delete"
	statusHomePathHelpWorkspaceDelete   = "help workspace delete"
	statusHomePathHelpPolicyAllow       = "help policy allow"
	statusHomePathHelpPolicyDeny        = "help policy deny"
	statusHomePathHelpPolicyReset       = "help policy reset"
	statusHomePathHelpReviewPermissions = "help review permissions"
	statusHomePathHelpClusterUp         = "help cluster up"
	statusHomePathHelpClusterDown       = "help cluster down"
	statusHomeGuidanceWaitForDetach     = "wait_for_detach"
	statusHomeGuidanceContinue          = "continue_attached"
)

// StatusHomeRecoveryPaths exposes the exact Catalog tasks owned by schema 3
// status so the composition root can check them without recreating status
// decision logic.
func StatusHomeRecoveryPaths() []string {
	return []string{
		statusHomePathEntry,
		statusHomePathTemplateList,
		statusHomePathClusterStatus,
		statusHomePathClusterUp,
		statusHomePathDoctor,
		statusHomePathReviewRuntimes,
		statusHomePathReviewPermissions,
		statusHomePathReviewServices,
		statusHomePathHelpContextDelete,
		statusHomePathHelpWorkspaceDelete,
		statusHomePathHelpPolicyAllow,
		statusHomePathHelpPolicyDeny,
		statusHomePathHelpPolicyReset,
		statusHomePathHelpReviewPermissions,
		statusHomePathHelpClusterUp,
		statusHomePathHelpClusterDown,
	}
}

// StatusHomeRecoveryGuidance exposes typed non-command terminal conditions;
// these values must never resolve as Catalog commands.
func StatusHomeRecoveryGuidance() []string {
	return []string{statusHomeGuidanceWaitForDetach, statusHomeGuidanceContinue}
}

type StatusObservationState string

const (
	StatusNotObserved StatusObservationState = "not_observed"
	StatusObserved    StatusObservationState = "observed"
	StatusUnknown     StatusObservationState = "unknown"
)

type StatusEntryState string

const (
	StatusEntryNotObserved     StatusEntryState = "not_observed"
	StatusEntryAbsent          StatusEntryState = "absent"
	StatusEntryCurrent         StatusEntryState = "current"
	StatusEntryPending         StatusEntryState = "pending"
	StatusEntryBlockedAttached StatusEntryState = "blocked_attached"
	StatusEntryUnknown         StatusEntryState = "unknown"
)

type StatusWorkspaceRuntimeState string

const (
	StatusWorkspaceRuntimeNotObserved StatusWorkspaceRuntimeState = "not_observed"
	StatusWorkspaceRuntimeAbsent      StatusWorkspaceRuntimeState = "absent"
	StatusWorkspaceRuntimeStopped     StatusWorkspaceRuntimeState = "stopped"
	StatusWorkspaceRuntimeRunning     StatusWorkspaceRuntimeState = "running"
	StatusWorkspaceRuntimeDrifted     StatusWorkspaceRuntimeState = "drifted"
	StatusWorkspaceRuntimeUnknown     StatusWorkspaceRuntimeState = "unknown"
)

type StatusRuntimeAuthorityState string

const (
	StatusRuntimeAuthorityNotObserved StatusRuntimeAuthorityState = "not_observed"
	StatusRuntimeAuthorityReady       StatusRuntimeAuthorityState = "ready"
	StatusRuntimeAuthorityNotReady    StatusRuntimeAuthorityState = "not_ready"
	StatusRuntimeAuthorityUnknown     StatusRuntimeAuthorityState = "unknown"
)

type StatusNativeCompatibility string

const (
	StatusNativeNotObserved  StatusNativeCompatibility = "not_observed"
	StatusNativeCompatible   StatusNativeCompatibility = "compatible"
	StatusNativeIncompatible StatusNativeCompatibility = "incompatible"
	StatusNativeUnknown      StatusNativeCompatibility = "unknown"
)

type StatusAttachmentState string

const (
	StatusAttachmentNotObserved StatusAttachmentState = "not_observed"
	StatusAttachmentDetached    StatusAttachmentState = "detached"
	StatusAttachmentAttached    StatusAttachmentState = "attached"
	StatusAttachmentUnknown     StatusAttachmentState = "unknown"
)

type StatusHomeTemplate struct {
	ID                WorkspaceTemplateID      `json:"workspace_template_id"`
	Name              string                   `json:"name"`
	Generation        uint64                   `json:"generation"`
	Revision          SemanticDigest           `json:"revision"`
	PolicySliceDigest SemanticDigest           `json:"policy_slice_digest"`
	EntrySliceDigest  SemanticDigest           `json:"entry_slice_digest"`
	SourceAccess      ManifestSourceAccess     `json:"source_access"`
	NativeReadiness   ManifestNativeReadiness  `json:"native_readiness"`
	Runtime           StatusHomeRuntimeBinding `json:"runtime"`
}

type StatusHomeRuntimeBinding struct {
	RuntimeID string `json:"runtime_id"`
	Name      string `json:"name"`
	Revision  string `json:"revision"`
	Ordinal   int    `json:"ordinal"`
}

type StatusHomeContext struct {
	ID                       ContextID       `json:"context_id"`
	ActiveTemplatePolicy     *SemanticDigest `json:"active_template_policy_slice_digest"`
	CurrentPolicyMemory      SemanticDigest  `json:"current_policy_memory_revision"`
	ActivePolicyMemory       *SemanticDigest `json:"active_policy_memory_revision"`
	TemplatePolicyActivation string          `json:"template_policy_activation"`
	PolicyMemoryActivation   string          `json:"policy_memory_activation"`
}

type StatusHomeSibling struct {
	ContextID        ContextID           `json:"context_id"`
	TemplateID       WorkspaceTemplateID `json:"workspace_template_id"`
	TemplateName     string              `json:"template_name"`
	WorkspacePresent bool                `json:"workspace_present"`
}

type StatusRuntimeObservation struct {
	Authority     StatusRuntimeAuthorityState `json:"revision_authority"`
	Availability  RuntimeAvailability         `json:"execution_material_availability"`
	Compatibility StatusNativeCompatibility   `json:"native_compatibility"`
}

type StatusWorkspaceObservation struct {
	State StatusWorkspaceRuntimeState `json:"observed_runtime_state"`
}

type StatusHomeWorkspace struct {
	Presence             string                      `json:"presence"`
	ID                   *WorkspaceID                `json:"workspace_id"`
	Ref                  *string                     `json:"workspace_ref,omitempty"`
	AppliedEntry         *WorkspaceAppliedEntry      `json:"applied_entry"`
	EntryState           StatusEntryState            `json:"entry_state"`
	ObservedRuntimeState StatusWorkspaceRuntimeState `json:"observed_runtime_state"`
	AttachmentState      StatusAttachmentState       `json:"attachment_state"`
}

type StatusHomeCluster struct {
	Observation StatusObservationState   `json:"observation"`
	Runtime     FinalClusterRuntimeState `json:"runtime"`
	Receipt     FinalClusterReceiptState `json:"receipt"`
}

type StatusPermissionSummary struct {
	Observation  StatusObservationState `json:"observation"`
	PendingCount int                    `json:"pending_count"`
}

type StatusServiceSummary struct {
	Observation           string `json:"observation"`
	PendingCount          int    `json:"pending_count"`
	ActiveCount           int    `json:"active_count"`
	UnavailableOwnerCount int    `json:"unavailable_owner_count"`
}

type StatusNextInput struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type StatusPrimaryNext struct {
	Kind     string            `json:"kind"`
	Path     *string           `json:"path"`
	Inputs   []StatusNextInput `json:"inputs"`
	Guidance *string           `json:"guidance"`
	Reason   string            `json:"reason"`
}

type StatusAttentionItem struct {
	Kind        string            `json:"kind"`
	Count       int               `json:"count"`
	Observation string            `json:"observation"`
	Path        string            `json:"path"`
	Inputs      []StatusNextInput `json:"inputs"`
}

// FinalAuthorityMutationObservation is a read-only view of the reserved
// final-authority recovery journal. The decision and stage remain private
// infrastructure artifacts; status and doctor receive only the bounded
// operation identity needed to select a safe exact recovery command.
type FinalAuthorityMutationObservation struct {
	ActiveDecision bool
	StagePresent   bool
	Operation      string
	Target         string
}

func (o FinalAuthorityMutationObservation) Validate() error {
	if !o.ActiveDecision && (o.Operation != "" || o.Target != "") {
		return fmt.Errorf("final-authority mutation observation has identity without an active decision")
	}
	if o.ActiveDecision && (o.Operation == "" || o.Target == "") {
		return fmt.Errorf("final-authority mutation observation is missing active decision identity")
	}
	return nil
}

// StatusHomeSnapshot is the single task-owned aggregate consumed by every
// presentation. It preserves independent desired, active, applied, and live
// facts and deliberately has no overall status field.
type StatusHomeSnapshot struct {
	Task                 string                             `json:"task"`
	AuthorityState       string                             `json:"authority_state"`
	ProjectRoot          string                             `json:"project_root"`
	DefaultTemplateState string                             `json:"template_state"`
	Template             *StatusHomeTemplate                `json:"template"`
	Context              *StatusHomeContext                 `json:"context"`
	Workspace            StatusHomeWorkspace                `json:"workspace"`
	Runtime              StatusRuntimeObservation           `json:"runtime"`
	Cluster              StatusHomeCluster                  `json:"cluster"`
	Permissions          StatusPermissionSummary            `json:"permissions"`
	Services             StatusServiceSummary               `json:"services"`
	LoginValidity        StatusObservationState             `json:"login_validity"`
	Siblings             []StatusHomeSibling                `json:"siblings"`
	Next                 StatusPrimaryNext                  `json:"next"`
	Attention            []StatusAttentionItem              `json:"attention"`
	mutationRecovery     *FinalAuthorityMutationObservation `json:"-"`
}

type StatusHomeLiveEvidence struct {
	Cluster          *FinalClusterStatus
	Runtime          StatusRuntimeObservation
	Workspace        StatusWorkspaceObservation
	Attachment       StatusAttachmentState
	Services         *ServiceSummary
	MutationRecovery *FinalAuthorityMutationObservation
}

type StatusHomeObservation struct {
	Collection  WorkspaceAuthorityCollection
	Present     bool
	ProjectRoot string
	Live        StatusHomeLiveEvidence
}

func NewStatusHomeSnapshot(collection WorkspaceAuthorityCollection, present bool, root string, live StatusHomeLiveEvidence) (StatusHomeSnapshot, error) {
	if ValidateCanonicalRoot(root) != nil {
		return StatusHomeSnapshot{}, fmt.Errorf("status Project root is invalid")
	}
	result := StatusHomeSnapshot{
		Task: TaskStatusHome, AuthorityState: "empty", ProjectRoot: root, DefaultTemplateState: "absent",
		Workspace:   StatusHomeWorkspace{Presence: "absent", EntryState: StatusEntryNotObserved, ObservedRuntimeState: StatusWorkspaceRuntimeNotObserved, AttachmentState: StatusAttachmentNotObserved},
		Runtime:     StatusRuntimeObservation{Authority: StatusRuntimeAuthorityNotObserved, Availability: RuntimeAvailabilityUnknown, Compatibility: StatusNativeNotObserved},
		Cluster:     StatusHomeCluster{Observation: StatusNotObserved, Runtime: FinalClusterRuntimeUnknown, Receipt: FinalClusterReceiptAbsent},
		Permissions: StatusPermissionSummary{Observation: StatusNotObserved}, Services: StatusServiceSummary{Observation: string(StatusNotObserved)},
		LoginValidity: StatusNotObserved, Siblings: []StatusHomeSibling{}, Attention: []StatusAttentionItem{},
	}
	if live.MutationRecovery != nil {
		if err := live.MutationRecovery.Validate(); err != nil {
			return StatusHomeSnapshot{}, err
		}
		result.mutationRecovery = live.MutationRecovery
		result.Attention = append(result.Attention, StatusAttentionItem{
			Kind: "mutation_recovery", Count: 1, Observation: string(StatusObserved),
			Path: statusHomeMutationRecoveryPath(live.MutationRecovery), Inputs: []StatusNextInput{},
		})
		result.Next = statusCommandNext(statusHomeMutationRecoveryPath(live.MutationRecovery), "A preserved final-authority mutation requires its exact recovery command.")
	}
	if !present {
		if result.mutationRecovery == nil {
			result.Next = statusCommandNext(statusHomePathEntry, "Review the recommended Template and enter this Project.")
		}
		return result, result.Validate()
	}
	if err := collection.Validate(); err != nil {
		return StatusHomeSnapshot{}, err
	}
	result.AuthorityState = "initialized"
	if collection.DefaultTemplateID == nil {
		if result.mutationRecovery == nil {
			result.Next = statusCommandNext(statusHomePathTemplateList, "Choose and set one installation default Template.")
		}
		return result, result.Validate()
	}
	var selected WorkspaceTemplate
	for _, candidate := range collection.Templates {
		if candidate.ID == *collection.DefaultTemplateID {
			selected = candidate.Clone()
			break
		}
	}
	if selected.ID == "" {
		return StatusHomeSnapshot{}, fmt.Errorf("default Template authority is missing")
	}
	current := selected.Current
	result.DefaultTemplateState = "selected"
	binding := current.Body.EntryDefaults.Runtime
	result.Template = &StatusHomeTemplate{ID: selected.ID, Name: selected.Name, Generation: current.Generation, Revision: current.Revision, PolicySliceDigest: current.Slices.PolicySliceDigest, EntrySliceDigest: current.Slices.EntrySliceDigest, SourceAccess: current.Body.Boundary.SourceAccess, NativeReadiness: current.Body.Policy.NativeReadiness, Runtime: StatusHomeRuntimeBinding{RuntimeID: binding.RuntimeID, Name: binding.Name, Revision: binding.Revision, Ordinal: binding.Ordinal}}
	var selectedSnapshot *ContextAuthoritySnapshot
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		return StatusHomeSnapshot{}, err
	}
	for _, snapshot := range snapshots {
		if snapshot.Workspace == nil || snapshot.Workspace.ProjectRoot != root {
			continue
		}
		if snapshot.Context.TemplateID == selected.ID {
			copy := snapshot.Clone()
			selectedSnapshot = &copy
			continue
		}
		result.Siblings = append(result.Siblings, StatusHomeSibling{ContextID: snapshot.Context.ID, TemplateID: snapshot.Template.ID, TemplateName: snapshot.Template.Name, WorkspacePresent: snapshot.Workspace != nil})
	}
	if selectedSnapshot == nil {
		if result.mutationRecovery == nil {
			result.Next = statusCommandNext(statusHomePathEntry, "Create the default Template's Context and enter it.")
		}
		return result, result.Validate()
	}
	axes, err := NewContextAuthorityAxes(*selectedSnapshot)
	if err != nil {
		return StatusHomeSnapshot{}, err
	}
	contextStatus := &StatusHomeContext{ID: axes.ContextID, ActiveTemplatePolicy: axes.ActiveTemplatePolicySliceDigest, CurrentPolicyMemory: axes.CurrentPolicyMemoryRevision, ActivePolicyMemory: axes.ActivePolicyMemoryRevision}
	contextStatus.TemplatePolicyActivation = statusActivation(current.Slices.PolicySliceDigest, axes.ActiveTemplatePolicySliceDigest)
	contextStatus.PolicyMemoryActivation = statusActivation(axes.CurrentPolicyMemoryRevision, axes.ActivePolicyMemoryRevision)
	result.Context = contextStatus
	result.Permissions = StatusPermissionSummary{Observation: StatusObserved}
	for _, candidate := range collection.PendingCandidates {
		if candidate.ContextID == axes.ContextID {
			result.Permissions.PendingCount++
		}
	}
	if live.Cluster != nil {
		if err := live.Cluster.Validate(); err != nil {
			return StatusHomeSnapshot{}, err
		}
		result.Cluster = StatusHomeCluster{Observation: StatusObserved, Runtime: live.Cluster.Runtime, Receipt: live.Cluster.Receipt}
	}
	if live.Runtime.Authority != "" || live.Runtime.Availability != "" || live.Runtime.Compatibility != "" {
		result.Runtime = live.Runtime
	}
	if selectedSnapshot.Workspace != nil {
		workspace := selectedSnapshot.Workspace
		id, ref := workspace.ID, ""
		ref, _ = WorkspaceRef(id)
		result.Workspace.Presence, result.Workspace.ID, result.Workspace.Ref = "present", &id, &ref
		if workspace.LastSuccessfulEntry != nil {
			value := *workspace.LastSuccessfulEntry
			result.Workspace.AppliedEntry = &value
		}
		if live.Workspace.State != "" {
			result.Workspace.ObservedRuntimeState = live.Workspace.State
		}
		if live.Attachment != "" {
			result.Workspace.AttachmentState = live.Attachment
		}
		result.Workspace.EntryState = statusEntryState(current, workspace.LastSuccessfulEntry, live.Attachment)
	}
	if live.Services != nil {
		if err := live.Services.Validate(); err != nil {
			return StatusHomeSnapshot{}, err
		}
		result.Services = StatusServiceSummary{Observation: string(live.Services.Observation), PendingCount: live.Services.PendingCount, ActiveCount: live.Services.ActiveCount, UnavailableOwnerCount: live.Services.UnavailableOwnerCount}
	}
	result.deriveAttentionAndNext()
	return result, result.Validate()
}

func statusActivation(current SemanticDigest, active *SemanticDigest) string {
	if active == nil {
		return "absent"
	}
	if *active == current {
		return "current"
	}
	return "pending"
}

func statusEntryState(current WorkspaceTemplateRevision, applied *WorkspaceAppliedEntry, attachment StatusAttachmentState) StatusEntryState {
	if applied == nil {
		return StatusEntryAbsent
	}
	if applied.TemplateRevision == current.Revision && applied.EntrySliceDigest == current.Slices.EntrySliceDigest && applied.RuntimeID == current.Slices.RuntimeID && applied.RuntimeRevision == current.Slices.RuntimeRevision {
		return StatusEntryCurrent
	}
	if attachment == StatusAttachmentAttached {
		return StatusEntryBlockedAttached
	}
	if attachment == StatusAttachmentUnknown {
		return StatusEntryUnknown
	}
	return StatusEntryPending
}

func statusCommandNext(path, reason string) StatusPrimaryNext {
	return StatusPrimaryNext{Kind: "command", Path: &path, Inputs: []StatusNextInput{}, Reason: reason}
}

func statusGuidanceNext(value, reason string) StatusPrimaryNext {
	return StatusPrimaryNext{Kind: "guidance", Inputs: []StatusNextInput{}, Guidance: &value, Reason: reason}
}

func (s *StatusHomeSnapshot) deriveAttentionAndNext() {
	if s.mutationRecovery != nil {
		return
	}
	if s.Permissions.PendingCount > 0 {
		s.Attention = append(s.Attention, StatusAttentionItem{Kind: "permissions", Count: s.Permissions.PendingCount, Observation: string(s.Permissions.Observation), Path: statusHomePathReviewPermissions, Inputs: []StatusNextInput{}})
	}
	if s.Services.PendingCount > 0 || s.Services.UnavailableOwnerCount > 0 {
		s.Attention = append(s.Attention, StatusAttentionItem{Kind: "services", Count: s.Services.PendingCount, Observation: s.Services.Observation, Path: statusHomePathReviewServices, Inputs: []StatusNextInput{}})
	}
	if s.Cluster.Observation == StatusObserved {
		switch s.Cluster.Runtime {
		case FinalClusterRuntimeUnknown, FinalClusterRuntimeDrifted, FinalClusterRuntimeUnhealthy:
			s.Next = statusCommandNext(statusHomePathClusterStatus, "Inspect the shared cluster before entry.")
			return
		case FinalClusterRuntimeAbsent, FinalClusterRuntimeStopped:
			s.Next = statusCommandNext(statusHomePathClusterUp, "Activate the shared cluster before entry.")
			return
		}
	}
	if s.Runtime.Authority != StatusRuntimeAuthorityReady || s.Runtime.Availability != RuntimeAvailabilityAvailable || s.Runtime.Compatibility != StatusNativeCompatible {
		s.Next = statusCommandNext(statusHomePathReviewRuntimes, "Inspect the exact Runtime revision and execution material.")
		return
	}
	switch s.Workspace.EntryState {
	case StatusEntryBlockedAttached:
		s.Next = statusGuidanceNext(statusHomeGuidanceWaitForDetach, "End the attached session before adopting the desired entry.")
		return
	case StatusEntryAbsent, StatusEntryPending, StatusEntryUnknown:
		s.Next = statusCommandNext(statusHomePathEntry, "Reconcile and enter the default Context's Workspace.")
		return
	}
	if s.Permissions.PendingCount > 0 {
		s.Next = statusCommandNext(statusHomePathReviewPermissions, "Review pending remembered-permission decisions.")
		return
	}
	if s.Services.PendingCount > 0 {
		s.Next = statusCommandNext(statusHomePathReviewServices, "Review pending Workspace service requests.")
		return
	}
	if s.Workspace.AttachmentState == StatusAttachmentAttached {
		s.Next = statusGuidanceNext(statusHomeGuidanceContinue, "Continue in the current attached Workspace session.")
		return
	}
	s.Next = statusCommandNext(statusHomePathEntry, "Enter or resume the current Workspace.")
}

func statusHomeMutationRecoveryPath(observation *FinalAuthorityMutationObservation) string {
	if observation == nil {
		return statusHomePathDoctor
	}
	switch observation.Operation {
	case "context-entry":
		return statusHomePathEntry
	case "context-delete":
		return statusHomePathHelpContextDelete
	case "workspace-delete", "workspace-delete-force":
		return statusHomePathHelpWorkspaceDelete
	case "policy-allow":
		return statusHomePathHelpPolicyAllow
	case "policy-deny":
		return statusHomePathHelpPolicyDeny
	case "policy-reset":
		return statusHomePathHelpPolicyReset
	case "policy-apply-reviewed":
		return statusHomePathHelpReviewPermissions
	case "cluster-reconcile":
		return statusHomePathHelpClusterUp
	case "cluster-down":
		return statusHomePathHelpClusterDown
	}
	return statusHomePathDoctor
}

func (s StatusHomeSnapshot) Validate() error {
	if s.Task != TaskStatusHome || ValidateCanonicalRoot(s.ProjectRoot) != nil || s.Siblings == nil || s.Attention == nil || s.Next.Inputs == nil || s.LoginValidity != StatusNotObserved {
		return fmt.Errorf("status home metadata is invalid")
	}
	if s.AuthorityState != "empty" && s.AuthorityState != "initialized" {
		return fmt.Errorf("status authority state is invalid")
	}
	if s.DefaultTemplateState != "absent" && s.DefaultTemplateState != "selected" {
		return fmt.Errorf("status default Template state is invalid")
	}
	if s.DefaultTemplateState == "selected" && s.Template == nil || s.DefaultTemplateState == "absent" && (s.Template != nil || s.Context != nil) {
		return fmt.Errorf("status Template selection is inconsistent")
	}
	if s.Workspace.Presence != "absent" && s.Workspace.Presence != "present" {
		return fmt.Errorf("status Workspace presence is invalid")
	}
	if s.Workspace.Presence == "present" && (s.Workspace.ID == nil || s.Workspace.Ref == nil) {
		return fmt.Errorf("status Workspace identity is incomplete")
	}
	if !validStatusEntryState(s.Workspace.EntryState) || !validStatusWorkspaceRuntimeState(s.Workspace.ObservedRuntimeState) || !validStatusAttachmentState(s.Workspace.AttachmentState) {
		return fmt.Errorf("status Workspace axes are invalid")
	}
	if !validStatusRuntimeObservation(s.Runtime) {
		return fmt.Errorf("status Runtime axes are invalid")
	}
	if s.Next.Kind == "command" && (s.Next.Path == nil || s.Next.Guidance != nil) || s.Next.Kind == "guidance" && (s.Next.Guidance == nil || s.Next.Path != nil) {
		return fmt.Errorf("status Next is invalid")
	}
	if s.Next.Kind != "command" && s.Next.Kind != "guidance" {
		return fmt.Errorf("status Next kind is invalid")
	}
	for _, item := range s.Attention {
		if item.Count < 0 || item.Path == "" || item.Inputs == nil {
			return fmt.Errorf("status Attention is invalid")
		}
	}
	return nil
}

func validStatusEntryState(value StatusEntryState) bool {
	switch value {
	case StatusEntryNotObserved, StatusEntryAbsent, StatusEntryCurrent, StatusEntryPending, StatusEntryBlockedAttached, StatusEntryUnknown:
		return true
	default:
		return false
	}
}

func validStatusWorkspaceRuntimeState(value StatusWorkspaceRuntimeState) bool {
	switch value {
	case StatusWorkspaceRuntimeNotObserved, StatusWorkspaceRuntimeAbsent, StatusWorkspaceRuntimeStopped, StatusWorkspaceRuntimeRunning, StatusWorkspaceRuntimeDrifted, StatusWorkspaceRuntimeUnknown:
		return true
	default:
		return false
	}
}

func validStatusAttachmentState(value StatusAttachmentState) bool {
	switch value {
	case StatusAttachmentNotObserved, StatusAttachmentDetached, StatusAttachmentAttached, StatusAttachmentUnknown:
		return true
	default:
		return false
	}
}

func validStatusRuntimeObservation(value StatusRuntimeObservation) bool {
	switch value.Authority {
	case StatusRuntimeAuthorityNotObserved, StatusRuntimeAuthorityReady, StatusRuntimeAuthorityNotReady, StatusRuntimeAuthorityUnknown:
	default:
		return false
	}
	if value.Availability.Validate() != nil {
		return false
	}
	switch value.Compatibility {
	case StatusNativeNotObserved, StatusNativeCompatible, StatusNativeIncompatible, StatusNativeUnknown:
		return true
	default:
		return false
	}
}
