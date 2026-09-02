package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tasuku43/tobari/internal/domain/capabilitysurface"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	// ReleaseProgramName is the protected release executable identity.
	ReleaseProgramName = "tobari"
	// ResearchProgramName is the repository-only research executable identity.
	ResearchProgramName = "tobari-research"
	// ExposureProgramName is the attachment-local service helper executable.
	ExposureProgramName = "tobari-expose"
	// PermissionProgramName is the attachment-local reviewed-permission observer.
	PermissionProgramName = "tobari-permission"
	// WorkspaceEntryCommandPath is the common Catalog task path for entering
	// the current Workspace. Its invocation presentation is the compiled
	// ProgramName, so research binaries do not publish a second root command.
	WorkspaceEntryCommandPath = "tobari"

	// maxAgentIndexEntryBytes bounds the selection-only root help cost per
	// command. Detailed invocation contracts belong in scoped help.
	maxAgentIndexEntryBytes = 512
)

// ProgramName is the compile-time canonical program identity used by catalog
// help, recovery, and JSON. Runtime input and a copied filename cannot change
// it.
var ProgramName = canonicalProgramName()

func canonicalProgramName() string {
	if capabilitysurface.Compiled().IncludesResearch() {
		return ResearchProgramName
	}
	return ReleaseProgramName
}

// invocationForPath renders one public Catalog path as an executable argv
// prefix. The common Workspace entry path is the executable itself.
func invocationForPath(path string) string {
	if path == WorkspaceEntryCommandPath {
		return ProgramName
	}
	return ProgramName + " " + path
}

type readOutputSettlement uint8

const (
	readOutputSettlementRetryable readOutputSettlement = iota
	readOutputSettlementConsumed
)

const consumedReadOutputWriteFailureCode = "consumed_read_output_write_failed"

type commandHandler func(context.Context, *CLI, CommandSpec, operation.Intent, ParsedInputs) int

type catalogFaultSignature struct {
	command   string
	kind      fault.Kind
	retryable bool
}

// CommandRole describes how a command participates in a deterministic task
// flow. RoleUnknown is the zero value so missing declarations fail closed.
type CommandRole uint8

const (
	RoleUnknown CommandRole = iota
	RoleUtility
	RoleDiscover
	RoleAct
)

func (r CommandRole) String() string {
	switch r {
	case RoleUtility:
		return "utility"
	case RoleDiscover:
		return "discover"
	case RoleAct:
		return "act"
	default:
		return "unknown"
	}
}

func (r CommandRole) validate() error {
	switch r {
	case RoleUtility, RoleDiscover, RoleAct:
		return nil
	default:
		return fmt.Errorf("command role is missing or invalid: %d", r)
	}
}

// ProducedRef declares an opaque reference written to one output field.
type ProducedRef struct {
	Kind  string `json:"kind"`
	Field string `json:"field"`
}

// ConsumedRef declares an opaque reference accepted by one argument.
type ConsumedRef struct {
	Kind     string `json:"kind"`
	Argument string `json:"argument"`
}

// FixedTargetScope identifies where a command-bound target is owned. Tobari
// permits only a singleton owned by this CLI installation.
type FixedTargetScope string

const (
	FixedTargetScopeUnknown   FixedTargetScope = ""
	FixedTargetScopeToolLocal FixedTargetScope = "tool_local"
)

// FixedTarget identifies one stable target selected entirely by the command
// path. It is mutually exclusive with consumed opaque references. Only a
// create may produce confirmed child references distinct from this scope kind.
type FixedTarget struct {
	Kind        string           `json:"kind"`
	ID          string           `json:"id"`
	Description string           `json:"description"`
	Scope       FixedTargetScope `json:"scope"`
}

// InputSource identifies the public channel through which one command input is
// supplied. InputSourceUnknown is invalid so an omitted source fails closed.
type InputSource string

const (
	InputSourceUnknown       InputSource = ""
	InputSourceArgument      InputSource = "argument"
	InputSourceFlag          InputSource = "flag"
	InputSourceStdin         InputSource = "stdin"
	InputSourceEnvironment   InputSource = "environment"
	InputSourceConfiguration InputSource = "configuration"
)

func (s InputSource) validate() error {
	switch s {
	case InputSourceArgument, InputSourceFlag, InputSourceStdin, InputSourceEnvironment, InputSourceConfiguration:
		return nil
	default:
		return fmt.Errorf("input source is missing or invalid: %q", s)
	}
}

// InputValueKind identifies the value grammar enforced by the catalog-owned
// command-line parser. The zero value is invalid so a new input cannot silently
// fall back to unbounded text.
type InputValueKind string

const (
	InputValueUnknown InputValueKind = ""
	InputValueText    InputValueKind = "text"
	InputValueInteger InputValueKind = "integer"
	InputValueBoolean InputValueKind = "boolean"
)

func (k InputValueKind) validate() error {
	switch k {
	case InputValueText, InputValueInteger, InputValueBoolean:
		return nil
	default:
		return fmt.Errorf("input value kind is missing or invalid: %q", k)
	}
}

// InputCardinality states whether one input name may contribute one value or
// several. Required independently distinguishes zero-or-one from exactly-one,
// and zero-or-more from one-or-more.
type InputCardinality string

const (
	InputCardinalityUnknown    InputCardinality = ""
	InputCardinalitySingle     InputCardinality = "single"
	InputCardinalityRepeatable InputCardinality = "repeatable"
)

func (c InputCardinality) validate() error {
	switch c {
	case InputCardinalitySingle, InputCardinalityRepeatable:
		return nil
	default:
		return fmt.Errorf("input cardinality is missing or invalid: %q", c)
	}
}

// InputCompletion identifies a typed, side-effect-free candidate source for
// interactive shell completion. Empty means that no candidates are declared;
// finite AllowedValues and booleans are completed directly from their existing
// input contract and do not need a separate source.
type InputCompletion string

const (
	InputCompletionNone                  InputCompletion = ""
	InputCompletionCommand               InputCompletion = "command"
	InputCompletionContextName           InputCompletion = "manifest_name"
	InputCompletionRuntimeName           InputCompletion = "runtime_name"
	InputCompletionManagedRuntimeName    InputCompletion = "managed_runtime_name"
	InputCompletionReadyRuntimeReference InputCompletion = "ready_runtime_reference"
	InputCompletionDirectory             InputCompletion = "directory"
)

func (c InputCompletion) validate() error {
	switch c {
	case InputCompletionNone, InputCompletionCommand, InputCompletionContextName,
		InputCompletionRuntimeName, InputCompletionManagedRuntimeName,
		InputCompletionReadyRuntimeReference, InputCompletionDirectory:
		return nil
	default:
		return fmt.Errorf("input completion source is invalid: %q", c)
	}
}

// CommandInput is one executable machine-readable input contract.
// DefaultValue is nil when omission has no catalog-owned default. Minimum and
// Maximum apply only to integer values; MinimumLength and MaximumLength apply
// only to text and count UTF-8 bytes. Requires and ConflictsWith are checked against explicitly
// supplied command-line inputs. ReferenceKind is empty only when the input is
// not an opaque reference.
type CommandInput struct {
	Name           string           `json:"name"`
	Source         InputSource      `json:"source"`
	Required       bool             `json:"required"`
	ValueKind      InputValueKind   `json:"value_kind"`
	Cardinality    InputCardinality `json:"cardinality"`
	Description    string           `json:"description"`
	AllowedValues  []string         `json:"allowed_values"`
	DefaultValue   *string          `json:"default_value,omitempty"`
	Minimum        *int64           `json:"minimum,omitempty"`
	Maximum        *int64           `json:"maximum,omitempty"`
	MinimumLength  *int64           `json:"minimum_length,omitempty"`
	MaximumLength  *int64           `json:"maximum_length,omitempty"`
	Requires       []string         `json:"requires,omitempty"`
	ConflictsWith  []string         `json:"conflicts_with,omitempty"`
	ReferenceKind  string           `json:"reference_kind,omitempty"`
	Completion     InputCompletion  `json:"completion,omitempty"`
	PositionalOnly bool             `json:"positional_only,omitempty"`
}

// OutputFormat identifies one stable presentation supported by a command.
type OutputFormat string

const (
	OutputFormatUnknown OutputFormat = ""
	OutputFormatNone    OutputFormat = "none"
	OutputFormatText    OutputFormat = "text"
	OutputFormatTSV     OutputFormat = "tsv"
	OutputFormatJSON    OutputFormat = "json"
)

func (f OutputFormat) validate() error {
	switch f {
	case OutputFormatNone, OutputFormatText, OutputFormatTSV, OutputFormatJSON:
		return nil
	default:
		return fmt.Errorf("output format is missing or invalid: %q", f)
	}
}

// OutputFieldType is the stable machine type of one logical output field.
type OutputFieldType string

const (
	OutputFieldTypeUnknown OutputFieldType = ""
	OutputFieldTypeString  OutputFieldType = "string"
	OutputFieldTypeBoolean OutputFieldType = "boolean"
	OutputFieldTypeInteger OutputFieldType = "integer"
	OutputFieldTypeObject  OutputFieldType = "object"
	OutputFieldTypeArray   OutputFieldType = "array"
)

func (t OutputFieldType) validate() error {
	switch t {
	case OutputFieldTypeString, OutputFieldTypeBoolean, OutputFieldTypeInteger, OutputFieldTypeObject, OutputFieldTypeArray:
		return nil
	default:
		return fmt.Errorf("output field type is missing or invalid: %q", t)
	}
}

// OutputField declares one logical field independently of its presentation.
// ReferenceKind is empty only when the field is not an opaque reference.
type OutputField struct {
	Name          string          `json:"name"`
	Type          OutputFieldType `json:"type"`
	Description   string          `json:"description"`
	Optional      bool            `json:"-"`
	Nullable      bool            `json:"nullable"`
	Enum          []string        `json:"enum"`
	ReferenceKind string          `json:"reference_kind,omitempty"`
	SemanticScope string          `json:"semantic_scope,omitempty"`
	Fields        []OutputField   `json:"fields,omitempty"`
	Items         *OutputField    `json:"items,omitempty"`
}

const (
	maxOutputFieldDepth = 8
	maxOutputFieldCount = 512
)

// MarshalJSON publishes required rather than the declaration-oriented
// Optional bit. Fields are required by default so existing scalar declarations
// stay fail-closed while optionality remains an explicit exception.
func (f OutputField) MarshalJSON() ([]byte, error) {
	type outputFieldDocument struct {
		Name          string          `json:"name,omitempty"`
		Type          OutputFieldType `json:"type"`
		Description   string          `json:"description"`
		Required      bool            `json:"required"`
		Nullable      bool            `json:"nullable"`
		Enum          []string        `json:"enum"`
		ReferenceKind string          `json:"reference_kind,omitempty"`
		SemanticScope string          `json:"semantic_scope,omitempty"`
		Fields        []OutputField   `json:"fields,omitempty"`
		Items         *OutputField    `json:"items,omitempty"`
	}
	enum := cloneSlice(f.Enum)
	if enum == nil {
		enum = []string{}
	}
	return json.Marshal(outputFieldDocument{
		Name: f.Name, Type: f.Type, Description: f.Description,
		Required: !f.Optional, Nullable: f.Nullable, Enum: enum,
		ReferenceKind: f.ReferenceKind, SemanticScope: f.SemanticScope,
		Fields: f.Fields, Items: f.Items,
	})
}

func (f *OutputField) UnmarshalJSON(encoded []byte) error {
	type outputFieldDocument struct {
		Name          string          `json:"name"`
		Type          OutputFieldType `json:"type"`
		Description   string          `json:"description"`
		Required      *bool           `json:"required"`
		Nullable      bool            `json:"nullable"`
		Enum          []string        `json:"enum"`
		ReferenceKind string          `json:"reference_kind"`
		SemanticScope string          `json:"semantic_scope"`
		Fields        []OutputField   `json:"fields"`
		Items         *OutputField    `json:"items"`
	}
	var document outputFieldDocument
	if err := json.Unmarshal(encoded, &document); err != nil {
		return err
	}
	required := true
	if document.Required != nil {
		required = *document.Required
	}
	*f = OutputField{
		Name: document.Name, Type: document.Type, Description: document.Description,
		Optional: !required, Nullable: document.Nullable, Enum: document.Enum,
		ReferenceKind: document.ReferenceKind, SemanticScope: document.SemanticScope,
		Fields: document.Fields, Items: document.Items,
	}
	return nil
}

// OutputDelivery states whether one invocation returns its complete selected
// result or one page in a public cursor protocol. It makes no claim about how
// much of an external collection the task selected.
type OutputDelivery string

const (
	OutputDeliveryUnknown  OutputDelivery = ""
	OutputDeliveryComplete OutputDelivery = "complete"
	OutputDeliveryPaged    OutputDelivery = "paged"
)

func (d OutputDelivery) validate() error {
	switch d {
	case OutputDeliveryComplete, OutputDeliveryPaged:
		return nil
	default:
		return fmt.Errorf("output delivery is missing or invalid: %q", d)
	}
}

// CollectionCoverage states what completing the delivery protocol covers
// within the exact declared task scope and observation. It never means every
// object or all history in the provider universe.
type CollectionCoverage string

const (
	CollectionCoverageUnknown            CollectionCoverage = ""
	CollectionCoverageNotApplicable      CollectionCoverage = "not_applicable"
	CollectionCoverageExhaustive         CollectionCoverage = "exhaustive"
	CollectionCoverageBoundedWindow      CollectionCoverage = "bounded_window"
	CollectionCoverageDifferentialWindow CollectionCoverage = "differential_window"
)

func (c CollectionCoverage) validate() error {
	switch c {
	case CollectionCoverageNotApplicable, CollectionCoverageExhaustive,
		CollectionCoverageBoundedWindow, CollectionCoverageDifferentialWindow:
		return nil
	default:
		return fmt.Errorf("collection coverage is missing or invalid: %q", c)
	}
}

// TextPresentation classifies CLI-owned text before rendering. Text-producing
// commands must opt in explicitly so a new command cannot bypass the shared
// semantic-token presentation boundary by omission.
type TextPresentation uint8

const (
	TextPresentationUnknown TextPresentation = iota
	TextPresentationSemanticTokens
)

// CommandOutput is the stable logical result and its supported presentations.
// Fields describe values inside JSONEnvelope, never top-level metadata.
type CommandOutput struct {
	Formats            []OutputFormat     `json:"formats"`
	DefaultFormat      OutputFormat       `json:"default_format"`
	Fields             []OutputField      `json:"fields"`
	Delivery           OutputDelivery     `json:"delivery"`
	CollectionCoverage CollectionCoverage `json:"collection_coverage"`
	JSONEnvelope       string             `json:"json_envelope,omitempty"`
	JSONEnvelopeType   OutputFieldType    `json:"json_envelope_type,omitempty"`
	JSONSchemaVersion  int                `json:"json_schema_version,omitempty"`
	TextPresentation   TextPresentation   `json:"-"`
	readSettlement     readOutputSettlement
}

// PaginationCompletion states the one machine-readable condition that marks
// traversal complete. A missing, null, or omitted cursor is not completion.
type PaginationCompletion string

const (
	PaginationCompletionUnknown     PaginationCompletion = ""
	PaginationCompletionEmptyCursor PaginationCompletion = "empty_cursor"
)

func (c PaginationCompletion) validate() error {
	if c != PaginationCompletionEmptyCursor {
		return fmt.Errorf("pagination completion is missing or invalid: %q", c)
	}
	return nil
}

// PaginationContract binds one optional public cursor input to the top-level
// string cursor field returned beside schema_version and the JSON envelope.
type PaginationContract struct {
	CursorInput  string               `json:"cursor_input"`
	CursorOutput OutputField          `json:"cursor_output"`
	Completion   PaginationCompletion `json:"completion"`
}

// CommandError declares one stable failure agents may handle without parsing
// prose. Kind and Code use the exact runtime fault taxonomy.
type CommandError struct {
	Code        string             `json:"code"`
	Kind        fault.Kind         `json:"kind"`
	Phase       fault.Phase        `json:"phase"`
	ChangeState fault.ChangeState  `json:"change_state"`
	Retryable   bool               `json:"retryable"`
	NextActions []fault.NextAction `json:"next_actions"`
}

// MutationContract connects a mutating command's public inputs to the target
// and generic impact facts consumed by the project-specific policy gate.
type MutationContract struct {
	TargetKind             string           `json:"target_kind"`
	TargetInputs           []string         `json:"target_inputs"`
	ParentInput            string           `json:"parent_input,omitempty"`
	TargetIDInput          string           `json:"target_id_input,omitempty"`
	CurrentContextFallback bool             `json:"current_context_fallback,omitempty"`
	Impact                 operation.Impact `json:"impact"`
}

// InteractiveWorkflowContract describes a human-only composition of one
// discover command and an existing action command. A reference-bound action
// receives one selected opaque reference unchanged; a fixed-target action owns
// one bounded typed set whose entries retain those references unchanged.
// Redirected and machine-readable invocations follow NonInteractiveBehavior.
type InteractiveWorkflowContract struct {
	ActionCommand          string   `json:"action_command,omitempty"`
	ActionCommands         []string `json:"action_commands,omitempty"`
	SelectionReferenceKind string   `json:"selection_reference_kind"`
	SelectionOutputField   string   `json:"selection_output_field"`
	Confirmation           string   `json:"confirmation"`
	NonInteractiveBehavior string   `json:"non_interactive_behavior"`
	ProjectsActionErrors   bool     `json:"projects_action_errors,omitempty"`
}

func (workflow InteractiveWorkflowContract) actionCommands() []string {
	if len(workflow.ActionCommands) != 0 {
		return append([]string{}, workflow.ActionCommands...)
	}
	if workflow.ActionCommand != "" {
		return []string{workflow.ActionCommand}
	}
	return []string{}
}

// MarshalJSON projects policy-relevant impact enums as stable words rather
// than implementation-specific integer values.
func (m MutationContract) MarshalJSON() ([]byte, error) {
	type impactDocument struct {
		Cardinality  string `json:"cardinality"`
		Notification string `json:"notification"`
		AccessChange string `json:"access_change"`
		Destructive  string `json:"destructive"`
	}
	type mutationDocument struct {
		TargetKind             string         `json:"target_kind"`
		TargetInputs           []string       `json:"target_inputs"`
		ParentInput            string         `json:"parent_input,omitempty"`
		TargetIDInput          string         `json:"target_id_input,omitempty"`
		CurrentContextFallback bool           `json:"current_context_fallback,omitempty"`
		Impact                 impactDocument `json:"impact"`
	}
	return json.Marshal(mutationDocument{
		TargetKind: m.TargetKind, TargetInputs: m.TargetInputs,
		ParentInput: m.ParentInput, TargetIDInput: m.TargetIDInput,
		CurrentContextFallback: m.CurrentContextFallback,
		Impact: impactDocument{
			Cardinality: m.Impact.Cardinality.String(), Notification: m.Impact.Notification.String(),
			AccessChange: m.Impact.AccessChange.String(), Destructive: m.Impact.Destructive.String(),
		},
	})
}

// AgentContract contains the bounded information needed to invoke and
// interpret a command without exploratory calls. Nil slices mean unknown and
// are invalid; non-nil empty slices explicitly mean none.
type AgentContract struct {
	CapabilityID  string                       `json:"capability_id"`
	Outcome       string                       `json:"outcome"`
	Inputs        []CommandInput               `json:"inputs"`
	Output        CommandOutput                `json:"output"`
	Pagination    *PaginationContract          `json:"pagination,omitempty"`
	Prerequisites []string                     `json:"prerequisites"`
	FixedTarget   *FixedTarget                 `json:"fixed_target,omitempty"`
	Errors        []CommandError               `json:"errors"`
	Mutation      *MutationContract            `json:"mutation,omitempty"`
	Interactive   *InteractiveWorkflowContract `json:"interactive,omitempty"`
	// projectedInteractiveMutationErrors is set only by the Catalog when a
	// public read projection hides its internal composed action path.
	projectedInteractiveMutationErrors bool
}

// CommandSpec is the single source of truth for dispatch, human help, and the
// machine-readable agent specification.
type CommandSpec struct {
	// Program identifies the executable that exposes this command. An empty
	// value is normalized to ProgramName for existing declarations.
	Program    string
	Path       string
	Summary    string
	Args       string
	Effect     operation.Effect
	Role       CommandRole
	Visibility CommandVisibility
	Agent      AgentContract
	handler    commandHandler
}

// CommandVisibility keeps internal composition mechanics in the canonical
// catalog without making them public discovery or routing surfaces.
type CommandVisibility uint8

const (
	CommandVisibilityPublic CommandVisibility = iota
	CommandVisibilityInternal
)

func (v CommandVisibility) validate() error {
	switch v {
	case CommandVisibilityPublic, CommandVisibilityInternal:
		return nil
	default:
		return fmt.Errorf("command visibility is invalid: %d", v)
	}
}

// Usage returns the complete command invocation without optional prose.
func (s CommandSpec) Usage() string {
	program := s.programName()
	usage := program
	if s.Path != WorkspaceEntryCommandPath && s.Path != program {
		usage += " " + s.Path
	}
	if s.Args != "" {
		usage += " " + s.Args
	}
	return usage
}

func (s CommandSpec) programName() string {
	if s.Program == "" {
		return ProgramName
	}
	return s.Program
}

// Catalog owns the complete set of public command paths.
type Catalog struct {
	commands []CommandSpec
	program  string
}

// NewCatalog creates a catalog from declarative command specifications.
func NewCatalog(commands ...CommandSpec) Catalog {
	cloned := make([]CommandSpec, len(commands))
	for index, command := range commands {
		cloned[index] = cloneCommandSpec(command)
		for errorIndex := range cloned[index].Agent.Errors {
			declared := &cloned[index].Agent.Errors[errorIndex]
			if declared.Phase == "" && declared.ChangeState == "" {
				declared.Phase, declared.ChangeState = defaultErrorClassification(command.Effect, declared.Kind, declared.Code)
			}
		}
	}
	return Catalog{commands: cloned, program: ProgramName}
}

func defaultErrorClassification(effect operation.Effect, kind fault.Kind, code string) (fault.Phase, fault.ChangeState) {
	if effect == operation.EffectRead {
		if code == "output_write_failed" || code == consumedReadOutputWriteFailureCode || code == "output_encoding_failed" || code == "output_contract_exceeded" {
			return fault.PhasePresentation, fault.ChangeNotApplicable
		}
		return fault.PhaseObservation, fault.ChangeNotApplicable
	}
	switch code {
	case "invalid_arguments", "operation_canceled", "invalid_mutation_contract", "missing_mutation_action", "missing_mutation_policy", "mutation_rejected", "missing_context", "test_failed":
		return fault.PhasePrecondition, fault.ChangeNone
	case "mutation_output_write_failed":
		return fault.PhasePresentation, fault.ChangeConfirmed
	case "invalid_manifest_report", "invalid_runtime_report", "invalid_migration_report", "status_failed":
		return fault.PhaseVerification, fault.ChangeConfirmed
	case "enter_failed":
		return fault.PhaseAttachment, fault.ChangeConfirmed
	default:
		if strings.HasSuffix(code, "_not_configured") || strings.HasSuffix(code, "_not_found") || strings.HasSuffix(code, "_not_ready") ||
			strings.HasSuffix(code, "_wizard_failed") || strings.HasSuffix(code, "_review_failed") || strings.HasSuffix(code, "_choice_failed") ||
			strings.HasSuffix(code, "_stale") {
			return fault.PhasePrecondition, fault.ChangeNone
		}
		switch kind {
		case fault.KindInvalidInput, fault.KindAuthentication, fault.KindPermission,
			fault.KindNotFound, fault.KindAmbiguous, fault.KindRejected,
			fault.KindRateLimited, fault.KindUnsupported:
			return fault.PhasePrecondition, fault.ChangeNone
		}
		if strings.HasPrefix(code, "invalid_") || strings.HasPrefix(code, "missing_") || strings.HasSuffix(code, "_invalid") {
			return fault.PhasePrecondition, fault.ChangeNone
		}
		return fault.PhaseMutation, fault.ChangeUnknown
	}
}

// ForProgram returns a routing and help view for one executable while
// retaining the global command graph for validation and reference closure.
func (c Catalog) ForProgram(program string) Catalog {
	copy := c
	copy.program = program
	return copy
}

func (c Catalog) programName() string {
	if c.program == "" {
		return ProgramName
	}
	return c.program
}

func commandCatalogKey(command CommandSpec) string {
	return command.programName() + "\x00" + command.Path
}

func declaredCommandError(kind fault.Kind, code string, retryable bool, command, reason string) CommandError {
	return CommandError{
		Kind:        kind,
		Code:        code,
		Retryable:   retryable,
		NextActions: []fault.NextAction{{Command: command, Reason: reason}},
	}
}

func declaredCommandErrorWithActions(kind fault.Kind, code string, retryable bool, actions ...fault.NextAction) CommandError {
	return CommandError{
		Kind: kind, Code: code, Retryable: retryable,
		NextActions: append([]fault.NextAction{}, actions...),
	}
}

func classifiedCommandError(
	kind fault.Kind, code string, retryable bool, phase fault.Phase, state fault.ChangeState,
	command, reason string,
) CommandError {
	declared := declaredCommandError(kind, code, retryable, command, reason)
	declared.Phase = phase
	declared.ChangeState = state
	return declared
}

func stringPointer(value string) *string {
	return &value
}

func int64Pointer(value int64) *int64 {
	return &value
}

func doctorCheckIDValues() []string {
	inventory := doctor.CheckInventory()
	values := make([]string, 0, len(inventory))
	for _, spec := range inventory {
		if !buildIdentityHasBroker() && isBrokerDoctorCheck(spec.ID) {
			continue
		}
		values = append(values, string(spec.ID))
	}
	return values
}

func isBrokerDoctorCheck(id doctor.CheckID) bool {
	switch id {
	case doctor.CheckIDAuthProviderManifests, doctor.CheckIDAuthVaultPaths,
		doctor.CheckIDAuthRootKey, doctor.CheckIDAuthBroker,
		doctor.CheckIDCredentialCompanion, doctor.CheckIDAuthVaultIntegrity,
		doctor.CheckIDAuthProjectHandles:
		return true
	default:
		return false
	}
}

func defaultCatalog() Catalog {
	catalog := NewCatalog(
		CommandSpec{
			Path:    "doctor",
			Summary: "Run local, read-only diagnostics",
			Args:    "[--root <path>] [--format text|tsv|json]",
			Effect:  operation.EffectRead,
			Role:    RoleUtility,
			Agent: AgentContract{
				CapabilityID: "system.diagnostics",
				Outcome:      "Inspect the local runtime and receive a validated diagnostic report",
				Inputs: []CommandInput{
					doctorFormatInput(),
					{
						Name: "--root", Source: InputSourceFlag, Required: false,
						ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
						Description: "Validate an existing host directory as a prospective Workspace project root; defaults to the current directory.", AllowedValues: []string{}, DefaultValue: stringPointer("."), MinimumLength: int64Pointer(1), Completion: InputCompletionDirectory,
					},
				},
				Output: CommandOutput{
					Formats:       []OutputFormat{OutputFormatText, OutputFormatTSV, OutputFormatJSON},
					DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
					Fields: []OutputField{
						{Name: "check", Type: OutputFieldTypeString, Description: "Stable diagnostic name with unsafe structural runes rendered as visible escapes.", Enum: doctorCheckIDValues()},
						{Name: "status", Type: OutputFieldTypeString, Description: "Diagnostic result: pass, warn, fail, or blocked.", Enum: []string{"pass", "warn", "fail", "blocked"}},
						{Name: "detail", Type: OutputFieldTypeString, Description: "Diagnostic detail with unsafe structural runes rendered as visible escapes."},
						{Name: "blocked_by", Type: OutputFieldTypeString, Description: "Direct prerequisite that did not pass, or null when this check was observed.", Nullable: true, Enum: doctorCheckIDValues()},
						{Name: "recovery", Type: OutputFieldTypeObject, Description: "Task-owned recovery for an observed failure or selected warning, or null when no recovery applies.", Nullable: true, Fields: []OutputField{
							{Name: "action", Type: OutputFieldTypeString, Description: "Concrete prerequisite correction in plain language."},
							{Name: "next_command", Type: OutputFieldTypeString, Description: "Exact Tobari command to run after the correction."},
						}},
					},
					Delivery:           OutputDeliveryComplete,
					CollectionCoverage: CollectionCoverageExhaustive,
					JSONEnvelope:       "report",
					JSONEnvelopeType:   OutputFieldTypeArray,
					JSONSchemaVersion:  1,
				},
				Prerequisites: []string{},
				Errors: []CommandError{
					declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help doctor", "Correct the command arguments."),
					declaredCommandError(fault.KindRejected, "diagnostic_failed", false, "doctor", "Execute the first failed row recovery, then rerun diagnostics."),
					declaredCommandError(fault.KindInternal, "doctor_failed", false, "version", "Report the exact build identity for diagnostic-runtime investigation."),
					declaredCommandError(fault.KindContract, "invalid_doctor_contract", false, "version", "Report the exact build identity for diagnostic-contract repair."),
					declaredCommandError(fault.KindInternal, "missing_runtime", false, "version", "Report the exact build identity for composition repair."),
					declaredCommandError(fault.KindContract, "output_contract_exceeded", false, "version", "Report the exact build identity for bounded-output investigation."),
					declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating JSON encoding."),
					declaredCommandError(fault.KindInternal, "internal_error", false, "version", "Report the exact build identity for diagnostic-adapter investigation."),
					declaredCommandError(fault.KindInternal, "output_write_failed", true, "doctor", "Retry with a writable output stream."),
					declaredCommandError(fault.KindCanceled, "operation_canceled", true, "doctor", "Retry when the caller is ready."),
				},
			},
			handler: runDoctor,
		},
		CommandSpec{
			Path:    "help",
			Summary: "Show human help or the agent command specification",
			Args:    "[<command>...] [--format text|agent]",
			Effect:  operation.EffectRead,
			Role:    RoleUtility,
			Agent: AgentContract{
				CapabilityID: "cli.discovery",
				Outcome:      "Discover command usage, contracts, workflows, and next actions without external I/O",
				Inputs: []CommandInput{
					{
						Name: "command", Source: InputSourceArgument, Required: false,
						ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable,
						Description: "Select an exact command path or canonical command namespace as one or more path words.", AllowedValues: []string{}, Completion: InputCompletionCommand,
					},
					{
						Name: "--format", Source: InputSourceFlag, Required: false,
						ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
						Description: "Select human text or the machine-readable agent contract.", AllowedValues: []string{"text", "agent"},
						DefaultValue: stringPointer("text"),
					},
				},
				Output: CommandOutput{
					Formats:       []OutputFormat{OutputFormatText, OutputFormatJSON},
					DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
					Fields: []OutputField{
						{Name: "path", Type: OutputFieldTypeString, Description: "Exact command path accepted as a scoped help selector."},
						{Name: "namespace", Type: OutputFieldTypeString, Description: "Canonical top-level namespace accepted as a scoped help selector."},
						{Name: "summary", Type: OutputFieldTypeString, Description: "Concise description of the command task."},
						{Name: "capability_id", Type: OutputFieldTypeString, Description: "Stable product capability identifier."},
						{Name: "outcome", Type: OutputFieldTypeString, Description: "User outcome the command can achieve."},
						{Name: "effect", Type: OutputFieldTypeString, Description: "Declared read, create, or write effect."},
						{Name: "role", Type: OutputFieldTypeString, Description: "Declared utility, discover, or act workflow role."},
					},
					Delivery:           OutputDeliveryComplete,
					CollectionCoverage: CollectionCoverageExhaustive,
					JSONEnvelope:       "commands",
					JSONEnvelopeType:   OutputFieldTypeArray,
					JSONSchemaVersion:  1,
				},
				Prerequisites: []string{},
				Errors: []CommandError{
					declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help", "Use text or agent format and an exact catalog command path."),
					declaredCommandError(fault.KindContract, "output_encoding_failed", false, "version", "Report the exact build identity without repeating agent-help encoding."),
					declaredCommandError(fault.KindInternal, "output_write_failed", true, "help", "Retry with a writable output stream."),
					declaredCommandError(fault.KindCanceled, "operation_canceled", true, "help", "Retry when the caller is ready."),
				},
			},
			handler: runHelp,
		},
		CommandSpec{
			Path:    "version",
			Summary: "Print deterministic build and runtime resolver identity",
			Args:    "[--format text|json]",
			Effect:  operation.EffectRead,
			Role:    RoleUtility,
			Agent: AgentContract{
				CapabilityID: "cli.version",
				Outcome:      "Read the executable version, source commit, resolver channel, and required versus selected component APIs",
				Inputs:       []CommandInput{formatInput()},
				Output: CommandOutput{
					Formats:       []OutputFormat{OutputFormatText, OutputFormatJSON},
					DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
					Fields:             versionOutputFields(),
					Delivery:           OutputDeliveryComplete,
					CollectionCoverage: CollectionCoverageNotApplicable,
					JSONEnvelope:       "build_identity",
					JSONEnvelopeType:   OutputFieldTypeObject,
					JSONSchemaVersion:  1,
				},
				Prerequisites: []string{},
				Errors: []CommandError{
					declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help version", "Run version without command arguments."),
					declaredCommandError(fault.KindContract, "invalid_build_identity", false, "doctor", "Inspect the installed executable and embedded runtime metadata."),
					declaredCommandError(fault.KindContract, "output_encoding_failed", false, "help version", "Inspect the build-identity contract without repeating JSON encoding."),
					declaredCommandError(fault.KindInternal, "output_write_failed", true, "version", "Retry with a writable output stream."),
					declaredCommandError(fault.KindCanceled, "operation_canceled", true, "version", "Retry when the caller is ready."),
				},
			},
			handler: runVersion,
		},
	)
	commands := catalog.commands
	commands = append(commands, completionCommandSpecs()...)
	commands = append(commands, runtimeCommandSpecs()...)
	commands = append(commands, serviceExposureCommandSpecs()...)
	return NewCatalog(append(commands, permissionWaitCommandSpecs()...)...)
}

// DefaultCatalog returns the public Tobari CLI contract.
func DefaultCatalog() Catalog {
	return defaultCatalog()
}

// Validate rejects incomplete command declarations before any handler runs.
func (c Catalog) Validate() error {
	if len(c.commands) == 0 {
		return fmt.Errorf("command catalog is empty")
	}
	if _, err := validateOutputFields(defaultAgentErrorContract().Fields); err != nil {
		return fmt.Errorf("agent error output contract: %w", err)
	}
	seen := make(map[string]struct{}, len(c.commands))
	commandsByPath := make(map[string]CommandSpec, len(c.commands))
	producedKinds := make(map[string][]string)
	consumedKinds := make(map[string][]string)
	paginationKindOwners := make(map[string]string)
	faultSignatures := make(map[string]catalogFaultSignature)
	for _, declaredError := range defaultAgentErrorContract().GlobalErrors {
		faultSignatures[declaredError.Code] = catalogFaultSignature{
			command:   "agent-help global errors",
			kind:      declaredError.Kind,
			retryable: declaredError.Retryable,
		}
	}
	for index, command := range c.commands {
		if err := operation.ValidateCommandPath(command.programName()); err != nil || strings.Contains(command.programName(), " ") {
			return fmt.Errorf("catalog command %d has invalid program %q", index, command.programName())
		}
		if err := operation.ValidateCommandPath(command.Path); err != nil {
			return fmt.Errorf("catalog command %d: %w", index, err)
		}
		if err := validateContractText("command summary", command.Summary); err != nil {
			return fmt.Errorf("catalog command %q has an invalid summary", command.Path)
		}
		if !utf8.ValidString(command.Args) || strings.TrimSpace(command.Args) != command.Args ||
			strings.IndexFunc(command.Args, isUnsafeContractRune) >= 0 {
			return fmt.Errorf("catalog command %q has invalid argument syntax", command.Path)
		}
		if err := command.Effect.Validate(); err != nil {
			return fmt.Errorf("catalog command %q: %w", command.Path, err)
		}
		if err := command.Role.validate(); err != nil {
			return fmt.Errorf("catalog command %q: %w", command.Path, err)
		}
		if err := command.Visibility.validate(); err != nil {
			return fmt.Errorf("catalog command %q: %w", command.Path, err)
		}
		if err := validateAgentContract(command); err != nil {
			return fmt.Errorf("catalog command %q: %w", command.Path, err)
		}
		if command.Visibility == CommandVisibilityPublic {
			if err := validateAgentIndexEntry(command); err != nil {
				return fmt.Errorf("catalog command %q: %w", command.Path, err)
			}
		}
		if err := validateCommandReferenceRole(command); err != nil {
			return fmt.Errorf("catalog command %q: %w", command.Path, err)
		}
		if command.handler == nil {
			return fmt.Errorf("catalog command %q has no handler", command.Path)
		}
		key := commandCatalogKey(command)
		for existing := range seen {
			existingProgram, existingPath, _ := strings.Cut(existing, "\x00")
			if existingProgram == command.programName() && (strings.HasPrefix(command.Path, existingPath+" ") || strings.HasPrefix(existingPath, command.Path+" ")) {
				return fmt.Errorf("catalog command paths %q and %q collide in program %q at a command/namespace boundary", existingPath, command.Path, command.programName())
			}
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("catalog contains duplicate command %q for program %q", command.Path, command.programName())
		}
		seen[key] = struct{}{}
		commandsByPath[key] = command
		for _, declaredError := range command.Agent.Errors {
			got := catalogFaultSignature{
				command:   command.Path,
				kind:      declaredError.Kind,
				retryable: declaredError.Retryable,
			}
			if previous, exists := faultSignatures[declaredError.Code]; exists &&
				(previous.kind != got.kind || previous.retryable != got.retryable) {
				return fmt.Errorf(
					"catalog fault code %q has conflicting signatures: command %q declares kind %q retryable=%t; command %q declares kind %q retryable=%t",
					declaredError.Code,
					previous.command,
					previous.kind,
					previous.retryable,
					got.command,
					got.kind,
					got.retryable,
				)
			}
			faultSignatures[declaredError.Code] = got
		}
		for _, produced := range command.ProducedRefs() {
			producedKinds[produced.Kind] = append(producedKinds[produced.Kind], command.Path)
		}
		for _, consumed := range command.ConsumedRefs() {
			consumedKinds[consumed.Kind] = append(consumedKinds[consumed.Kind], command.Path)
		}
		if command.Agent.Pagination != nil {
			paginationKindOwners[command.Agent.Pagination.CursorOutput.ReferenceKind] = command.Path
		}
	}
	for _, command := range c.commands {
		workflow := command.Agent.Interactive
		if workflow == nil {
			continue
		}
		actions := workflow.actionCommands()
		for _, actionPath := range actions {
			action, found := commandsByPath[command.programName()+"\x00"+actionPath]
			if !found {
				return fmt.Errorf("catalog command %q interactive action %q is not registered", command.Path, actionPath)
			}
			if (action.Effect != operation.EffectCreate && action.Effect != operation.EffectWrite) || action.Role != RoleAct {
				return fmt.Errorf("catalog command %q interactive action %q must be a create or write act command", command.Path, actionPath)
			}
			if workflow.ProjectsActionErrors {
				projected := make(map[string]CommandError, len(command.Agent.Errors))
				for _, declared := range command.Agent.Errors {
					projected[declared.Code] = declared
				}
				for _, actionError := range action.Agent.Errors {
					declared, found := projected[actionError.Code]
					if !found || declared.Kind != actionError.Kind || declared.Retryable != actionError.Retryable ||
						declared.Phase != actionError.Phase || declared.ChangeState != actionError.ChangeState ||
						!reflect.DeepEqual(declared.NextActions, actionError.NextActions) {
						return fmt.Errorf("catalog command %q must project interactive action %q error %q exactly", command.Path, actionPath, actionError.Code)
					}
				}
			}
			if action.Agent.FixedTarget != nil {
				continue
			}
			matched := 0
			for _, consumed := range action.ConsumedRefs() {
				if consumed.Kind == workflow.SelectionReferenceKind {
					matched++
				}
			}
			if matched != 1 {
				return fmt.Errorf("catalog command %q interactive action %q must consume exactly one %q reference", command.Path, actionPath, workflow.SelectionReferenceKind)
			}
		}
	}
	for kind, owner := range paginationKindOwners {
		producers := producedKinds[kind]
		consumers := consumedKinds[kind]
		if len(producers) != 1 || producers[0] != owner || len(consumers) != 1 || consumers[0] != owner {
			return fmt.Errorf("pagination reference kind %q must be dedicated to command %q", kind, owner)
		}
	}
	for kind, producers := range producedKinds {
		if len(consumedKinds[kind]) == 0 {
			return fmt.Errorf("reference kind %q is produced by %s but has no consumer", kind, strings.Join(producers, ", "))
		}
	}
	for kind, consumers := range consumedKinds {
		if len(producedKinds[kind]) == 0 {
			return fmt.Errorf("reference kind %q is consumed by %s but has no producer", kind, strings.Join(consumers, ", "))
		}
	}
	if err := validateReferenceReachability(c.commands); err != nil {
		return err
	}
	for _, command := range c.commands {
		for _, declaredError := range command.Agent.Errors {
			for _, action := range declaredError.NextActions {
				nextCommand, err := c.resolveRecoveryCommandForProgram(command.programName(), action.Command)
				if err != nil {
					return fmt.Errorf("catalog command %q error %q: %w", command.Path, declaredError.Code, err)
				}
				requiresReadOnlyRecovery := (command.Effect != operation.EffectRead &&
					(declaredError.ChangeState == fault.ChangePartial ||
						declaredError.ChangeState == fault.ChangeConfirmed ||
						declaredError.ChangeState == fault.ChangeUnknown)) ||
					(command.Effect != operation.EffectRead && declaredError.Kind == fault.KindRateLimited && !declaredError.Retryable)
				if requiresReadOnlyRecovery && nextCommand.Effect != operation.EffectRead {
					return fmt.Errorf("catalog command %q error %q must point to a read-only reconciliation command", command.Path, declaredError.Code)
				}
			}
		}
	}
	return nil
}

// resolveRecoveryCommand validates the deliberately small recovery grammar:
// either one exact command path, or help followed by one exact command path or
// canonical namespace. Argument-bearing recovery needs a future typed contract
// rather than an unchecked prose suffix.
func (c Catalog) resolveRecoveryCommand(value string) (CommandSpec, error) {
	return c.resolveRecoveryCommandForProgram(c.programName(), value)
}

func (c Catalog) resolveRecoveryCommandForProgram(program, value string) (CommandSpec, error) {
	view := c.ForProgram(program)
	words := strings.Fields(value)
	if len(words) == 0 || strings.Join(words, " ") != value {
		return CommandSpec{}, fmt.Errorf("next command %q is not canonical", value)
	}
	if command, found := view.Lookup(value); found {
		return command, nil
	}
	if words[0] != "help" || len(words) == 1 {
		return CommandSpec{}, fmt.Errorf("next command %q is not an exact catalog path", value)
	}
	help, found := view.Lookup("help")
	if !found {
		return CommandSpec{}, fmt.Errorf("next command %q requires the catalog help command", value)
	}
	hasSelector := false
	for _, input := range help.Agent.Inputs {
		if input.Name == "command" && input.Source == InputSourceArgument && !input.Required {
			hasSelector = true
			break
		}
	}
	if !hasSelector {
		return CommandSpec{}, fmt.Errorf("next command %q requires help to declare its optional command selector", value)
	}
	selector := strings.Join(words[1:], " ")
	selected, _ := view.Select(selector)
	if len(selected) == 0 {
		return CommandSpec{}, fmt.Errorf("next command %q has an unknown help selector", value)
	}
	return help, nil
}

func validateAgentContract(command CommandSpec) error {
	contract := command.Agent
	if err := validateCapabilityID(contract.CapabilityID); err != nil {
		return err
	}
	if err := validateContractText("outcome", contract.Outcome); err != nil {
		return err
	}
	if contract.FixedTarget != nil {
		if err := validateFixedTarget(*contract.FixedTarget); err != nil {
			return err
		}
	}
	if contract.Inputs == nil {
		return fmt.Errorf("agent inputs are unknown; use an explicit empty list when there are none")
	}
	seenInputs := make(map[string]struct{}, len(contract.Inputs))
	inputsByName := make(map[string]CommandInput, len(contract.Inputs))
	commandLineInputs := make(map[string]struct{})
	repeatableArgumentSeen := false
	for index, input := range contract.Inputs {
		if err := input.Source.validate(); err != nil {
			return fmt.Errorf("agent input %d: %w", index, err)
		}
		if err := input.ValueKind.validate(); err != nil {
			return fmt.Errorf("agent input %d: %w", index, err)
		}
		if err := input.Cardinality.validate(); err != nil {
			return fmt.Errorf("agent input %d: %w", index, err)
		}
		if err := input.Completion.validate(); err != nil {
			return fmt.Errorf("agent input %d: %w", index, err)
		}
		if err := validateInputName(input); err != nil {
			return fmt.Errorf("agent input %d: %w", index, err)
		}
		if err := validateContractText("input description", input.Description); err != nil {
			return fmt.Errorf("agent input %q: %w", input.Name, err)
		}
		if input.AllowedValues == nil {
			return fmt.Errorf("agent input %q allowed values are unknown; use an explicit empty list for free-form values", input.Name)
		}
		seenValues := make(map[string]struct{}, len(input.AllowedValues))
		for _, value := range input.AllowedValues {
			if err := validateContractText("input allowed value", value); err != nil {
				return fmt.Errorf("agent input %q: %w", input.Name, err)
			}
			if err := validateStableInputLiteral(value); err != nil {
				return fmt.Errorf("agent input %q allowed value: %w", input.Name, err)
			}
			if _, exists := seenValues[value]; exists {
				return fmt.Errorf("agent input %q allowed value %q is declared more than once", input.Name, value)
			}
			seenValues[value] = struct{}{}
		}
		if input.Required && input.DefaultValue != nil {
			return fmt.Errorf("agent input %q cannot be required and declare a default", input.Name)
		}
		if input.Cardinality == InputCardinalityRepeatable && input.DefaultValue != nil {
			return fmt.Errorf("agent repeatable input %q cannot declare one scalar default", input.Name)
		}
		if input.Source != InputSourceArgument && input.Source != InputSourceFlag && input.Cardinality != InputCardinalitySingle {
			return fmt.Errorf("agent non-command-line input %q must use single cardinality", input.Name)
		}
		if input.ValueKind == InputValueBoolean && input.Cardinality == InputCardinalityRepeatable {
			return fmt.Errorf("agent boolean input %q cannot be repeatable", input.Name)
		}
		if input.Source == InputSourceArgument {
			if repeatableArgumentSeen {
				return fmt.Errorf("agent argument input %q follows a repeatable positional input", input.Name)
			}
			repeatableArgumentSeen = input.Cardinality == InputCardinalityRepeatable
		} else if input.PositionalOnly {
			return fmt.Errorf("agent input %q requires positional-only syntax but is not an argument", input.Name)
		}
		if input.ValueKind != InputValueInteger && (input.Minimum != nil || input.Maximum != nil) {
			return fmt.Errorf("agent non-integer input %q cannot declare numeric bounds", input.Name)
		}
		if input.ValueKind != InputValueText && (input.MinimumLength != nil || input.MaximumLength != nil) {
			return fmt.Errorf("agent non-text input %q cannot declare text length bounds", input.Name)
		}
		if input.MinimumLength != nil && *input.MinimumLength < 0 {
			return fmt.Errorf("agent text input %q minimum length is negative", input.Name)
		}
		if input.MaximumLength != nil && *input.MaximumLength < 0 {
			return fmt.Errorf("agent text input %q maximum length is negative", input.Name)
		}
		if input.MinimumLength != nil && input.MaximumLength != nil && *input.MinimumLength > *input.MaximumLength {
			return fmt.Errorf("agent text input %q minimum length exceeds maximum length", input.Name)
		}
		if input.Minimum != nil && input.Maximum != nil && *input.Minimum > *input.Maximum {
			return fmt.Errorf("agent integer input %q minimum exceeds maximum", input.Name)
		}
		if input.ValueKind == InputValueBoolean && len(input.AllowedValues) != 0 {
			return fmt.Errorf("agent boolean input %q uses the fixed true/false grammar rather than allowed values", input.Name)
		}
		if input.Completion != InputCompletionNone {
			if input.Source != InputSourceArgument && input.Source != InputSourceFlag {
				return fmt.Errorf("agent input %q completion requires a command-line input", input.Name)
			}
			if input.ValueKind != InputValueText {
				return fmt.Errorf("agent input %q completion requires text values", input.Name)
			}
			if len(input.AllowedValues) != 0 {
				return fmt.Errorf("agent input %q completion conflicts with finite allowed values", input.Name)
			}
			if input.ReferenceKind != "" {
				return fmt.Errorf("agent input %q completion must not expose opaque references", input.Name)
			}
			if input.Completion == InputCompletionCommand && input.Source != InputSourceArgument {
				return fmt.Errorf("agent input %q command completion requires a positional selector", input.Name)
			}
		}
		for _, value := range input.AllowedValues {
			if err := validateInputScalar(input, value); err != nil {
				return fmt.Errorf("agent input %q has invalid allowed value %q: %w", input.Name, value, err)
			}
		}
		if input.DefaultValue != nil {
			if err := validateStableInputLiteral(*input.DefaultValue); err != nil {
				return fmt.Errorf("agent input %q has invalid default: %w", input.Name, err)
			}
			if err := validateInputValue(input, *input.DefaultValue); err != nil {
				return fmt.Errorf("agent input %q has invalid default: %w", input.Name, err)
			}
		}
		if _, exists := seenInputs[input.Name]; exists {
			return fmt.Errorf("agent input %q is declared more than once", input.Name)
		}
		seenInputs[input.Name] = struct{}{}
		inputsByName[input.Name] = input
		if input.ReferenceKind != "" {
			if err := validateReferenceName(input.ReferenceKind); err != nil {
				return fmt.Errorf("agent input %q reference kind: %w", input.Name, err)
			}
			if len(input.AllowedValues) != 0 {
				return fmt.Errorf("agent reference input %q must accept opaque values rather than an enumeration", input.Name)
			}
			if input.ValueKind != InputValueText {
				return fmt.Errorf("agent reference input %q must use text values", input.Name)
			}
		}
		if input.Source == InputSourceArgument || input.Source == InputSourceFlag {
			commandLineInputs[input.Name] = struct{}{}
		}
	}
	for _, input := range contract.Inputs {
		if err := validateInputRelations(input, inputsByName); err != nil {
			return err
		}
	}
	if err := validateInputRelationSatisfiability(inputsByName); err != nil {
		return err
	}
	syntaxInputs, syntaxPositionals, err := parseArgumentSyntaxInputs(command.Args)
	if err != nil {
		return err
	}
	declaredPositionals := make([]string, 0)
	for _, input := range contract.Inputs {
		if input.Source == InputSourceArgument {
			declaredPositionals = append(declaredPositionals, input.Name)
		}
	}
	if !equalStrings(declaredPositionals, syntaxPositionals) {
		return fmt.Errorf("agent positional input order %v does not match argument syntax order %v", declaredPositionals, syntaxPositionals)
	}
	for input := range commandLineInputs {
		syntax, exists := syntaxInputs[input]
		if !exists {
			return fmt.Errorf("agent input %q is not present in argument syntax %q", input, command.Args)
		}
		declared := inputsByName[input]
		if declared.Required != syntax.Required {
			return fmt.Errorf("agent input %q required=%t does not match argument syntax required=%t", input, declared.Required, syntax.Required)
		}
		if !equalStrings(declared.AllowedValues, syntax.AllowedValues) {
			return fmt.Errorf("agent input %q allowed values %v do not match argument syntax values %v", input, declared.AllowedValues, syntax.AllowedValues)
		}
		if declared.Source == InputSourceFlag {
			declaredTakesValue := declared.ValueKind != InputValueBoolean
			if declaredTakesValue != syntax.TakesValue {
				return fmt.Errorf("agent input %q value kind %q does not match whether argument syntax takes a value", input, declared.ValueKind)
			}
		} else {
			if (declared.Cardinality == InputCardinalityRepeatable) != syntax.Repeatable {
				return fmt.Errorf("agent positional input %q repeatability does not match argument syntax", input)
			}
			if declared.PositionalOnly != syntax.PositionalOnly {
				return fmt.Errorf("agent positional input %q positional-only requirement does not match argument syntax", input)
			}
		}
	}
	for input := range syntaxInputs {
		if _, exists := commandLineInputs[input]; !exists {
			return fmt.Errorf("argument syntax input %q is not described by the agent contract", input)
		}
	}

	if contract.Output.Formats == nil || len(contract.Output.Formats) == 0 {
		return fmt.Errorf("agent output formats are unknown")
	}
	seenFormats := make(map[OutputFormat]struct{}, len(contract.Output.Formats))
	for _, format := range contract.Output.Formats {
		if err := format.validate(); err != nil {
			return err
		}
		if _, exists := seenFormats[format]; exists {
			return fmt.Errorf("agent output format %q is declared more than once", format)
		}
		seenFormats[format] = struct{}{}
	}
	if err := contract.Output.DefaultFormat.validate(); err != nil {
		return fmt.Errorf("agent default output format: %w", err)
	}
	if _, exists := seenFormats[contract.Output.DefaultFormat]; !exists {
		return fmt.Errorf("agent default output format %q is not supported", contract.Output.DefaultFormat)
	}
	if _, none := seenFormats[OutputFormatNone]; none && len(seenFormats) != 1 {
		return fmt.Errorf("none output format cannot be combined with another format")
	}
	if contract.Output.Fields == nil {
		return fmt.Errorf("agent output fields are unknown; use an explicit empty list when there are none")
	}
	if _, none := seenFormats[OutputFormatNone]; none {
		if len(contract.Output.Fields) != 0 {
			return fmt.Errorf("none output format must not declare fields")
		}
	} else if len(contract.Output.Fields) == 0 {
		return fmt.Errorf("agent output must declare at least one field")
	}
	fieldsByName, err := validateOutputFields(contract.Output.Fields)
	if err != nil {
		return err
	}
	if contract.Interactive != nil {
		if err := validateInteractiveWorkflow(command, fieldsByName); err != nil {
			return err
		}
	}
	if err := contract.Output.Delivery.validate(); err != nil {
		return err
	}
	if err := contract.Output.CollectionCoverage.validate(); err != nil {
		return err
	}
	_, supportsText := seenFormats[OutputFormatText]
	if supportsText && contract.Output.TextPresentation != TextPresentationSemanticTokens {
		return fmt.Errorf("text output requires semantic-token presentation")
	}
	if _, none := seenFormats[OutputFormatNone]; none && contract.Output.CollectionCoverage != CollectionCoverageNotApplicable {
		return fmt.Errorf("none output format requires collection coverage %q", CollectionCoverageNotApplicable)
	}
	_, supportsJSON := seenFormats[OutputFormatJSON]
	if supportsJSON {
		if err := validateOutputFieldName(contract.Output.JSONEnvelope); err != nil {
			return fmt.Errorf("agent JSON envelope: %w", err)
		}
		if contract.Output.JSONEnvelopeType != OutputFieldTypeObject && contract.Output.JSONEnvelopeType != OutputFieldTypeArray &&
			contract.Output.JSONEnvelopeType != OutputFieldTypeString {
			return fmt.Errorf("agent JSON envelope type must be object, array, or a declared scalar string")
		}
		if contract.Output.JSONEnvelopeType == OutputFieldTypeString &&
			(len(contract.Output.Fields) != 1 || contract.Output.Fields[0].Name != contract.Output.JSONEnvelope || contract.Output.Fields[0].Type != OutputFieldTypeString) {
			return fmt.Errorf("agent scalar JSON envelope must have one same-named string field")
		}
		if contract.Output.JSONSchemaVersion <= 0 {
			return fmt.Errorf("agent JSON schema version must be positive")
		}
	} else if contract.Output.JSONEnvelope != "" || contract.Output.JSONEnvelopeType != OutputFieldTypeUnknown || contract.Output.JSONSchemaVersion != 0 {
		return fmt.Errorf("agent JSON metadata requires JSON output support")
	}
	if err := validatePaginationContract(contract.Output, contract.Pagination, inputsByName); err != nil {
		return err
	}

	if contract.Prerequisites == nil {
		return fmt.Errorf("agent prerequisites are unknown; use an explicit empty list when there are none")
	}
	seenPrerequisites := make(map[string]struct{}, len(contract.Prerequisites))
	for index, prerequisite := range contract.Prerequisites {
		if err := validateContractText(fmt.Sprintf("prerequisite %d", index), prerequisite); err != nil {
			return err
		}
		if _, exists := seenPrerequisites[prerequisite]; exists {
			return fmt.Errorf("agent prerequisite %q is declared more than once", prerequisite)
		}
		seenPrerequisites[prerequisite] = struct{}{}
	}
	if contract.Errors == nil || len(contract.Errors) == 0 {
		return fmt.Errorf("agent error contract is unknown")
	}
	seenErrors := make(map[string]CommandError, len(contract.Errors))
	for index, declaredError := range contract.Errors {
		if declaredError.NextActions == nil || len(declaredError.NextActions) == 0 {
			return fmt.Errorf("agent error %q next actions are unknown", declaredError.Code)
		}
		candidate := fault.New(
			declaredError.Kind,
			declaredError.Code,
			"catalog-declared failure",
			declaredError.Retryable,
			declaredError.NextActions...,
		)
		candidate = fault.WithClassification(candidate, declaredError.Phase, declaredError.ChangeState)
		if err := candidate.Validate(); err != nil {
			return fmt.Errorf("agent error %d: %w", index, err)
		}
		for _, action := range declaredError.NextActions {
			if err := validateContractText("error next command", action.Command); err != nil {
				return fmt.Errorf("agent error %q: %w", declaredError.Code, err)
			}
			if err := validateContractText("error next reason", action.Reason); err != nil {
				return fmt.Errorf("agent error %q: %w", declaredError.Code, err)
			}
		}
		if _, exists := seenErrors[declaredError.Code]; exists {
			return fmt.Errorf("agent error code %q is declared more than once", declaredError.Code)
		}
		seenErrors[declaredError.Code] = declaredError
	}
	if err := requireAgentError(seenErrors, "operation_canceled", fault.KindCanceled, true); err != nil {
		return err
	}
	if err := requireAgentError(seenErrors, "invalid_arguments", fault.KindInvalidInput, false); err != nil {
		return err
	}
	_, noOutput := seenFormats[OutputFormatNone]
	_, hasReadOutputFailure := seenErrors["output_write_failed"]
	_, hasConsumedReadOutputFailure := seenErrors[consumedReadOutputWriteFailureCode]
	_, hasMutationOutputFailure := seenErrors["mutation_output_write_failed"]
	projectsMutationErrors := contract.projectedInteractiveMutationErrors || contract.Interactive != nil && contract.Interactive.ProjectsActionErrors
	if command.Effect == operation.EffectRead && hasMutationOutputFailure && !projectsMutationErrors {
		return fmt.Errorf("read command must not declare mutation_output_write_failed")
	}
	if command.Effect != operation.EffectRead && hasReadOutputFailure {
		return fmt.Errorf("mutating command must not declare retryable output_write_failed")
	}
	if command.Effect != operation.EffectRead && hasConsumedReadOutputFailure {
		return fmt.Errorf("mutating command must not declare consumed read output failure")
	}
	if noOutput && (hasReadOutputFailure || hasConsumedReadOutputFailure || hasMutationOutputFailure) {
		return fmt.Errorf("command without output must not declare an output write failure")
	}
	if !noOutput && command.Effect == operation.EffectRead {
		switch contract.Output.readSettlement {
		case readOutputSettlementRetryable:
			if hasConsumedReadOutputFailure {
				return fmt.Errorf("retryable read must not declare consumed read output failure")
			}
			if err := requireAgentError(seenErrors, "output_write_failed", fault.KindInternal, true); err != nil {
				return err
			}
		case readOutputSettlementConsumed:
			if hasReadOutputFailure {
				return fmt.Errorf("consumed read must not declare retryable output_write_failed")
			}
			if err := requireAgentError(seenErrors, consumedReadOutputWriteFailureCode, fault.KindInternal, false); err != nil {
				return err
			}
		default:
			return fmt.Errorf("read output settlement is invalid")
		}
	}
	if command.Effect == operation.EffectRead {
		if contract.Mutation != nil {
			return fmt.Errorf("read command must not declare a mutation contract")
		}
		return nil
	}
	if contract.Mutation == nil {
		return fmt.Errorf("mutating command must declare a mutation contract")
	}
	for _, required := range []struct {
		code      string
		kind      fault.Kind
		retryable bool
	}{
		{code: "invalid_mutation_contract", kind: fault.KindContract},
		{code: "missing_mutation_action", kind: fault.KindContract},
		{code: "missing_mutation_policy", kind: fault.KindRejected},
		{code: "mutation_rejected", kind: fault.KindRejected},
		{code: "unclassified_mutation_outcome", kind: fault.KindContract},
	} {
		if err := requireAgentError(seenErrors, required.code, required.kind, required.retryable); err != nil {
			return err
		}
	}
	if !noOutput {
		if err := requireAgentError(seenErrors, "mutation_output_write_failed", fault.KindInternal, false); err != nil {
			return err
		}
	}
	mutation := contract.Mutation
	if err := validateReferenceName(mutation.TargetKind); err != nil {
		return fmt.Errorf("mutation target kind: %w", err)
	}
	if contract.FixedTarget != nil {
		if mutation.TargetKind != contract.FixedTarget.Kind {
			return fmt.Errorf("mutation target kind must match fixed target kind %q", contract.FixedTarget.Kind)
		}
		if mutation.TargetInputs == nil {
			return fmt.Errorf("fixed-target mutation target_inputs must be an explicit empty list")
		}
		if len(mutation.TargetInputs) != 0 {
			return fmt.Errorf("fixed-target mutation target_inputs must be empty")
		}
		if mutation.ParentInput != "" || mutation.TargetIDInput != "" {
			return fmt.Errorf("fixed-target mutation must not declare parent_input or target_id_input")
		}
		if err := mutation.Impact.Validate(); err != nil {
			return fmt.Errorf("mutation impact: %w", err)
		}
		return nil
	}
	if mutation.TargetInputs == nil || len(mutation.TargetInputs) == 0 {
		return fmt.Errorf("mutation target inputs are unknown")
	}
	seenTargets := make(map[string]struct{}, len(mutation.TargetInputs))
	for _, name := range mutation.TargetInputs {
		if _, exists := seenInputs[name]; !exists {
			return fmt.Errorf("mutation target input %q is not a structured input", name)
		}
		if _, exists := seenTargets[name]; exists {
			return fmt.Errorf("mutation target input %q is declared more than once", name)
		}
		seenTargets[name] = struct{}{}
	}
	if err := mutation.Impact.Validate(); err != nil {
		return fmt.Errorf("mutation impact: %w", err)
	}
	if command.Effect == operation.EffectCreate {
		if mutation.ParentInput == "" || mutation.TargetIDInput != "" {
			return fmt.Errorf("create mutation requires parent_input and must not declare target_id_input")
		}
		parent, err := validateMutationBinding(mutation.ParentInput, mutation.TargetInputs, inputsByName, mutation.CurrentContextFallback)
		if err != nil {
			return fmt.Errorf("create mutation parent: %w", err)
		}
		if parent.ReferenceKind == "" {
			return fmt.Errorf("create mutation parent input must consume an opaque reference")
		}
		if len(mutation.TargetInputs) != 1 {
			return fmt.Errorf("create mutation target_inputs must contain only parent_input")
		}
	}
	if command.Effect == operation.EffectWrite {
		if mutation.TargetIDInput == "" {
			return fmt.Errorf("write mutation requires target_id_input")
		}
		target, err := validateMutationBinding(mutation.TargetIDInput, mutation.TargetInputs, inputsByName, mutation.CurrentContextFallback)
		if err != nil {
			return fmt.Errorf("write mutation target ID: %w", err)
		}
		if target.ReferenceKind == "" || target.ReferenceKind != mutation.TargetKind {
			return fmt.Errorf("write mutation target ID must consume the opaque %q reference", mutation.TargetKind)
		}
		expectedTargetInputs := 1
		if mutation.ParentInput != "" {
			if mutation.ParentInput == mutation.TargetIDInput {
				return fmt.Errorf("write mutation parent_input and target_id_input must be distinct")
			}
			parent, err := validateMutationBinding(mutation.ParentInput, mutation.TargetInputs, inputsByName, mutation.CurrentContextFallback)
			if err != nil {
				return fmt.Errorf("write mutation parent: %w", err)
			}
			if parent.ReferenceKind == "" {
				return fmt.Errorf("write mutation parent input must consume an opaque reference")
			}
			expectedTargetInputs++
		}
		if len(mutation.TargetInputs) != expectedTargetInputs {
			return fmt.Errorf("write mutation target_inputs must contain only target_id_input and optional parent_input")
		}
	}
	return nil
}

func validateOutputFields(fields []OutputField) (map[string]OutputField, error) {
	byName := make(map[string]OutputField, len(fields))
	count := 0
	for index, field := range fields {
		if err := validateOutputField(field, 1, true, &count); err != nil {
			return nil, fmt.Errorf("agent output field %d: %w", index, err)
		}
		if _, duplicate := byName[field.Name]; duplicate {
			return nil, fmt.Errorf("agent output field %q is declared more than once", field.Name)
		}
		byName[field.Name] = field
	}
	return byName, nil
}

func validateOutputField(field OutputField, depth int, named bool, count *int) error {
	*count++
	if *count > maxOutputFieldCount {
		return fmt.Errorf("recursive output declaration exceeds maximum field count %d", maxOutputFieldCount)
	}
	if depth > maxOutputFieldDepth {
		return fmt.Errorf("recursive output declaration exceeds maximum depth %d", maxOutputFieldDepth)
	}
	if named {
		if err := validateOutputFieldName(field.Name); err != nil {
			return err
		}
	} else if field.Name != "" {
		return fmt.Errorf("array item shape cannot declare field name %q", field.Name)
	}
	label := field.Name
	if label == "" {
		label = "array item"
	}
	if err := field.Type.validate(); err != nil {
		return fmt.Errorf("output field %q: %w", label, err)
	}
	if err := validateContractText("output field description", field.Description); err != nil {
		return fmt.Errorf("output field %q: %w", label, err)
	}
	if field.SemanticScope != "" {
		if err := validateContractText("output field semantic scope", field.SemanticScope); err != nil {
			return fmt.Errorf("output field %q: %w", label, err)
		}
	}
	seenEnum := make(map[string]struct{}, len(field.Enum))
	for _, value := range field.Enum {
		if field.Type != OutputFieldTypeString {
			return fmt.Errorf("output field %q enum requires string type", label)
		}
		if value == "" {
			return fmt.Errorf("output field %q enum cannot contain an empty sentinel", label)
		}
		if err := validateStableInputLiteral(value); err != nil {
			return fmt.Errorf("output field %q enum: %w", label, err)
		}
		if _, duplicate := seenEnum[value]; duplicate {
			return fmt.Errorf("output field %q enum value %q is declared more than once", label, value)
		}
		seenEnum[value] = struct{}{}
	}
	if field.ReferenceKind != "" {
		if err := validateReferenceName(field.ReferenceKind); err != nil {
			return fmt.Errorf("output field %q reference kind: %w", label, err)
		}
		if field.Type != OutputFieldTypeString {
			return fmt.Errorf("output reference field %q must have string type", label)
		}
		if field.Nullable {
			return fmt.Errorf("output field %q opaque reference cannot be nullable", label)
		}
		if len(field.Enum) != 0 {
			return fmt.Errorf("output field %q opaque reference cannot declare enum values", label)
		}
	}
	switch field.Type {
	case OutputFieldTypeObject:
		if field.Fields == nil || len(field.Fields) == 0 {
			return fmt.Errorf("output field %q object fields are unknown", label)
		}
		if field.Items != nil {
			return fmt.Errorf("output field %q object cannot declare array items", label)
		}
		if len(field.Enum) != 0 || field.ReferenceKind != "" {
			return fmt.Errorf("output field %q object cannot declare enum or reference metadata", label)
		}
		seen := make(map[string]struct{}, len(field.Fields))
		for _, child := range field.Fields {
			if err := validateOutputField(child, depth+1, true, count); err != nil {
				return err
			}
			if _, duplicate := seen[child.Name]; duplicate {
				return fmt.Errorf("output field %q child %q is declared more than once", label, child.Name)
			}
			seen[child.Name] = struct{}{}
		}
	case OutputFieldTypeArray:
		if field.Items == nil {
			return fmt.Errorf("output field %q array item shape is unknown", label)
		}
		if field.Fields != nil {
			return fmt.Errorf("output field %q array cannot declare object children directly", label)
		}
		if len(field.Enum) != 0 || field.ReferenceKind != "" {
			return fmt.Errorf("output field %q array cannot declare enum or reference metadata", label)
		}
		if err := validateOutputField(*field.Items, depth+1, false, count); err != nil {
			return err
		}
	default:
		if field.Fields != nil || field.Items != nil {
			return fmt.Errorf("output field %q scalar cannot declare children or array items", label)
		}
	}
	return nil
}

func validateInteractiveWorkflow(command CommandSpec, fields map[string]OutputField) error {
	workflow := command.Agent.Interactive
	if workflow == nil {
		return nil
	}
	if command.Effect != operation.EffectRead || command.Role != RoleDiscover {
		return fmt.Errorf("interactive workflow must belong to a read-only discover command")
	}
	actions := workflow.actionCommands()
	if len(actions) == 0 {
		return fmt.Errorf("interactive action command is missing")
	}
	seenActions := make(map[string]bool, len(actions))
	for _, action := range actions {
		if err := operation.ValidateCommandPath(action); err != nil {
			return fmt.Errorf("interactive action command: %w", err)
		}
		if action == command.Path {
			return fmt.Errorf("interactive action command must be separate from the discover command")
		}
		if seenActions[action] {
			return fmt.Errorf("interactive action commands must be unique")
		}
		seenActions[action] = true
	}
	if err := validateReferenceName(workflow.SelectionReferenceKind); err != nil {
		return fmt.Errorf("interactive selection reference kind: %w", err)
	}
	selectionExists := false
	for _, produced := range command.ProducedRefs() {
		if produced.Field != workflow.SelectionOutputField {
			continue
		}
		selectionExists = true
		if produced.Kind != workflow.SelectionReferenceKind {
			return fmt.Errorf("interactive selection output field %q must produce reference kind %q", workflow.SelectionOutputField, workflow.SelectionReferenceKind)
		}
	}
	if !selectionExists {
		return fmt.Errorf("interactive selection output field %q is not declared", workflow.SelectionOutputField)
	}
	if workflow.Confirmation != "explicit_yes" && workflow.Confirmation != "explicit_action" {
		return fmt.Errorf("interactive confirmation must be explicit_yes or explicit_action")
	}
	if workflow.NonInteractiveBehavior != "read_only" {
		return fmt.Errorf("interactive non-interactive behavior must be read_only")
	}
	return nil
}

func validateMutationBinding(name string, targetInputs []string, inputs map[string]CommandInput, allowCurrentContextFallback bool) (CommandInput, error) {
	input, exists := inputs[name]
	if !exists {
		return CommandInput{}, fmt.Errorf("input %q is not a structured input", name)
	}
	if input.Source != InputSourceArgument && input.Source != InputSourceFlag {
		return CommandInput{}, fmt.Errorf("input %q must be a command argument or flag", name)
	}
	if !input.Required && !allowCurrentContextFallback {
		return CommandInput{}, fmt.Errorf("input %q must be required", name)
	}
	if allowCurrentContextFallback && input.Required {
		return CommandInput{}, fmt.Errorf("current-Context fallback input %q must be optional", name)
	}
	if allowCurrentContextFallback && input.ReferenceKind != tobari.ContextReferenceKind {
		return CommandInput{}, fmt.Errorf("current-Context fallback input %q must consume a Context reference", name)
	}
	for _, target := range targetInputs {
		if target == name {
			return input, nil
		}
	}
	return CommandInput{}, fmt.Errorf("input %q is not included in target_inputs", name)
}

func validatePaginationContract(output CommandOutput, pagination *PaginationContract, inputs map[string]CommandInput) error {
	switch output.Delivery {
	case OutputDeliveryComplete:
		if pagination != nil {
			return fmt.Errorf("complete output must not declare a pagination binding")
		}
		return nil
	case OutputDeliveryPaged:
		if pagination == nil {
			return fmt.Errorf("paged output must declare a pagination binding")
		}
		if output.CollectionCoverage == CollectionCoverageNotApplicable {
			return fmt.Errorf("paged output requires collection coverage")
		}
	default:
		return nil // Delivery validation reports the governing error.
	}
	if len(output.Formats) != 1 || output.Formats[0] != OutputFormatJSON || output.DefaultFormat != OutputFormatJSON {
		return fmt.Errorf("paged output must support only JSON and use JSON as its default format")
	}

	cursorInput, exists := inputs[pagination.CursorInput]
	if !exists {
		return fmt.Errorf("pagination cursor input %q is not a structured input", pagination.CursorInput)
	}
	if cursorInput.Required {
		return fmt.Errorf("pagination cursor input %q must be optional", pagination.CursorInput)
	}
	if cursorInput.Source != InputSourceArgument && cursorInput.Source != InputSourceFlag {
		return fmt.Errorf("pagination cursor input %q must be a command argument or flag", pagination.CursorInput)
	}
	if cursorInput.ReferenceKind == "" {
		return fmt.Errorf("pagination cursor input %q must consume an opaque reference", pagination.CursorInput)
	}
	if err := validateOutputFieldName(pagination.CursorOutput.Name); err != nil {
		return fmt.Errorf("pagination cursor output: %w", err)
	}
	if pagination.CursorOutput.Name == "schema_version" || pagination.CursorOutput.Name == output.JSONEnvelope {
		return fmt.Errorf("pagination cursor output %q collides with top-level JSON metadata", pagination.CursorOutput.Name)
	}
	if pagination.CursorOutput.Type != OutputFieldTypeString {
		return fmt.Errorf("pagination cursor output %q must have string type", pagination.CursorOutput.Name)
	}
	if err := validateContractText("pagination cursor output description", pagination.CursorOutput.Description); err != nil {
		return err
	}
	if err := validateReferenceName(pagination.CursorOutput.ReferenceKind); err != nil {
		return fmt.Errorf("pagination cursor output %q reference kind: %w", pagination.CursorOutput.Name, err)
	}
	if cursorInput.ReferenceKind != pagination.CursorOutput.ReferenceKind {
		return fmt.Errorf("pagination cursor input and output must use the same reference kind")
	}
	if err := pagination.Completion.validate(); err != nil {
		return err
	}

	for name, input := range inputs {
		if name != pagination.CursorInput && input.ReferenceKind == cursorInput.ReferenceKind {
			return fmt.Errorf("pagination reference kind %q has an extra cursor input %q", cursorInput.ReferenceKind, name)
		}
	}
	for _, field := range output.Fields {
		if field.ReferenceKind == cursorInput.ReferenceKind {
			return fmt.Errorf("pagination reference kind %q has an extra cursor output %q", cursorInput.ReferenceKind, field.Name)
		}
	}
	return nil
}

func requireAgentError(declared map[string]CommandError, code string, kind fault.Kind, retryable bool) error {
	contract, exists := declared[code]
	if !exists {
		return fmt.Errorf("agent error contract must declare runtime error %q", code)
	}
	if contract.Kind != kind || contract.Retryable != retryable {
		return fmt.Errorf("agent runtime error %q must declare kind %q and retryable=%t", code, kind, retryable)
	}
	return nil
}

func validateCapabilityID(value string) error {
	if value == "" || strings.Trim(value, ".") != value {
		return fmt.Errorf("agent capability ID is missing or invalid: %q", value)
	}
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return fmt.Errorf("agent capability ID must contain lowercase dot-separated segments: %q", value)
	}
	for _, part := range parts {
		if err := validateReferenceName(part); err != nil {
			return fmt.Errorf("agent capability ID %q: %w", value, err)
		}
	}
	return nil
}

func validateInputName(input CommandInput) error {
	if input.Name == "" || len(input.Name) > 4096 || !utf8.ValidString(input.Name) || strings.TrimSpace(input.Name) != input.Name ||
		strings.IndexFunc(input.Name, func(r rune) bool { return unicode.IsSpace(r) || isUnsafeContractRune(r) }) >= 0 {
		return fmt.Errorf("input name is missing or invalid: %q", input.Name)
	}
	switch input.Source {
	case InputSourceFlag:
		if !strings.HasPrefix(input.Name, "--") {
			return fmt.Errorf("flag input %q must be a long flag", input.Name)
		}
		if err := validateReferenceName(strings.TrimPrefix(input.Name, "--")); err != nil {
			return fmt.Errorf("flag input: %w", err)
		}
	case InputSourceArgument:
		if err := validateReferenceName(input.Name); err != nil {
			return fmt.Errorf("argument input: %w", err)
		}
	case InputSourceStdin:
		if err := validateOutputFieldName(input.Name); err != nil {
			return fmt.Errorf("stdin input: %w", err)
		}
	case InputSourceEnvironment:
		if err := validateEnvironmentInputName(input.Name); err != nil {
			return err
		}
	case InputSourceConfiguration:
		for _, segment := range strings.Split(input.Name, ".") {
			if err := validateOutputFieldName(segment); err != nil {
				return fmt.Errorf("configuration input name is invalid: %q", input.Name)
			}
		}
	}
	return nil
}

func validateEnvironmentInputName(value string) error {
	if value == "" {
		return fmt.Errorf("environment input name is empty")
	}
	for index, character := range value {
		switch {
		case character >= 'A' && character <= 'Z':
		case index > 0 && character >= '0' && character <= '9':
		case index > 0 && character == '_':
		default:
			return fmt.Errorf("environment input name is invalid: %q", value)
		}
	}
	return nil
}

func validateInputValue(input CommandInput, value string) error {
	if len(input.AllowedValues) != 0 {
		allowed := false
		for _, candidate := range input.AllowedValues {
			if value == candidate {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("value must be one of %s", strings.Join(input.AllowedValues, ", "))
		}
	}
	return validateInputScalar(input, value)
}

func validateInputScalar(input CommandInput, value string) error {
	if !utf8.ValidString(value) || strings.IndexFunc(value, isUnsafeContractRune) >= 0 {
		return fmt.Errorf("value contains invalid structural text")
	}
	switch input.ValueKind {
	case InputValueText:
		if input.MinimumLength != nil && int64(len(value)) < *input.MinimumLength {
			return fmt.Errorf("value must contain at least %d byte(s)", *input.MinimumLength)
		}
		if input.MaximumLength != nil && int64(len(value)) > *input.MaximumLength {
			return fmt.Errorf("value must contain at most %d byte(s)", *input.MaximumLength)
		}
		return nil
	case InputValueInteger:
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("value must be a base-10 integer")
		}
		if input.Minimum != nil && parsed < *input.Minimum {
			return fmt.Errorf("value must be at least %d", *input.Minimum)
		}
		if input.Maximum != nil && parsed > *input.Maximum {
			return fmt.Errorf("value must be at most %d", *input.Maximum)
		}
		return nil
	case InputValueBoolean:
		if value != "true" && value != "false" {
			return fmt.Errorf("value must be true or false")
		}
		return nil
	default:
		return fmt.Errorf("value kind is invalid")
	}
}

func validateStableInputLiteral(value string) error {
	for _, character := range []byte(value) {
		if character < 0x20 || character > 0x7e {
			return fmt.Errorf("catalog-owned values must use stable printable ASCII bytes")
		}
	}
	return nil
}

func validateInputRelations(input CommandInput, inputs map[string]CommandInput) error {
	requires := make(map[string]struct{}, len(input.Requires))
	for _, name := range input.Requires {
		if name == input.Name {
			return fmt.Errorf("agent input %q cannot require itself", input.Name)
		}
		if _, duplicate := requires[name]; duplicate {
			return fmt.Errorf("agent input %q requires %q more than once", input.Name, name)
		}
		target, exists := inputs[name]
		if !exists {
			return fmt.Errorf("agent input %q requires unknown input %q", input.Name, name)
		}
		if !isCommandLineInput(input) || !isCommandLineInput(target) {
			return fmt.Errorf("agent input %q relation to %q is not enforceable by the command-line parser", input.Name, name)
		}
		if input.Required && !target.Required {
			return fmt.Errorf("required agent input %q makes optional required input %q effectively mandatory", input.Name, name)
		}
		requires[name] = struct{}{}
	}
	conflicts := make(map[string]struct{}, len(input.ConflictsWith))
	for _, name := range input.ConflictsWith {
		if name == input.Name {
			return fmt.Errorf("agent input %q cannot conflict with itself", input.Name)
		}
		if _, duplicate := conflicts[name]; duplicate {
			return fmt.Errorf("agent input %q conflicts with %q more than once", input.Name, name)
		}
		target, exists := inputs[name]
		if !exists {
			return fmt.Errorf("agent input %q conflicts with unknown input %q", input.Name, name)
		}
		if !isCommandLineInput(input) || !isCommandLineInput(target) {
			return fmt.Errorf("agent input %q relation to %q is not enforceable by the command-line parser", input.Name, name)
		}
		if _, required := requires[name]; required {
			return fmt.Errorf("agent input %q both requires and conflicts with %q", input.Name, name)
		}
		if input.Required && target.Required {
			return fmt.Errorf("required agent inputs %q and %q cannot conflict", input.Name, name)
		}
		conflicts[name] = struct{}{}
	}
	return nil
}

// validateInputRelationSatisfiability proves that the required invocation and
// every optional input can appear in at least one valid invocation after the
// transitive requires closure is applied. Conflicts are symmetric presence
// constraints even when declared on only one endpoint.
func validateInputRelationSatisfiability(inputs map[string]CommandInput) error {
	names := make([]string, 0, len(inputs))
	required := make(map[string]bool, len(inputs))
	for name, input := range inputs {
		names = append(names, name)
		if input.Required {
			required[name] = true
		}
	}
	sort.Strings(names)
	if left, right, conflict := inputPresenceConflict(required, inputs); conflict {
		return fmt.Errorf("required agent inputs %q and %q conflict after dependency expansion", left, right)
	}
	for _, name := range names {
		if inputs[name].Required {
			continue
		}
		selected := make(map[string]bool, len(required)+1)
		for requiredName := range required {
			selected[requiredName] = true
		}
		selected[name] = true
		if left, right, conflict := inputPresenceConflict(selected, inputs); conflict {
			return fmt.Errorf("optional agent input %q is unusable because %q and %q conflict after dependency expansion", name, left, right)
		}
	}
	return nil
}

func inputPresenceConflict(selected map[string]bool, inputs map[string]CommandInput) (string, string, bool) {
	queue := make([]string, 0, len(selected))
	for name := range selected {
		queue = append(queue, name)
	}
	sort.Strings(queue)
	for index := 0; index < len(queue); index++ {
		name := queue[index]
		for _, required := range inputs[name].Requires {
			if !selected[required] {
				selected[required] = true
				queue = append(queue, required)
			}
		}
	}
	selectedNames := make([]string, 0, len(selected))
	for name := range selected {
		selectedNames = append(selectedNames, name)
	}
	sort.Strings(selectedNames)
	for _, name := range selectedNames {
		for _, conflict := range inputs[name].ConflictsWith {
			if selected[conflict] {
				return name, conflict, true
			}
		}
	}
	return "", "", false
}

func isCommandLineInput(input CommandInput) bool {
	return input.Source == InputSourceArgument || input.Source == InputSourceFlag
}

type argumentSyntaxInput struct {
	Required       bool
	AllowedValues  []string
	TakesValue     bool
	Repeatable     bool
	PositionalOnly bool
}

type argumentSyntaxToken struct {
	Value    string
	Optional bool
}

func parseArgumentSyntaxInputs(syntax string) (map[string]argumentSyntaxInput, []string, error) {
	inputs := make(map[string]argumentSyntaxInput)
	positionals := make([]string, 0)
	rawTokens := strings.Fields(syntax)
	tokens := make([]argumentSyntaxToken, 0, len(rawTokens))
	inOptional := false
	for _, raw := range rawTokens {
		opens := strings.HasPrefix(raw, "[")
		closes := strings.HasSuffix(raw, "]")
		if opens {
			if inOptional {
				return nil, nil, fmt.Errorf("argument syntax contains nested optional groups")
			}
			inOptional = true
		}
		if closes && !inOptional {
			return nil, nil, fmt.Errorf("argument syntax contains an unmatched closing bracket")
		}
		value := strings.Trim(raw, "[]()")
		if value == "" {
			return nil, nil, fmt.Errorf("argument syntax contains an empty token")
		}
		tokens = append(tokens, argumentSyntaxToken{Value: value, Optional: inOptional})
		if closes {
			inOptional = false
		}
	}
	if inOptional {
		return nil, nil, fmt.Errorf("argument syntax contains an unclosed optional group")
	}

	optionalPositionalSeen := false
	positionalOnly := false
	positionalAfterMarker := false
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if token.Value == "--" {
			if positionalOnly {
				return nil, nil, fmt.Errorf("argument syntax contains more than one positional-only marker")
			}
			positionalOnly = true
			continue
		}
		if strings.HasPrefix(token.Value, "--") {
			if positionalOnly {
				return nil, nil, fmt.Errorf("argument syntax flag %q follows the positional-only marker", token.Value)
			}
			parts := strings.SplitN(token.Value, "=", 2)
			name := parts[0]
			if err := validateInputName(CommandInput{Name: name, Source: InputSourceFlag}); err != nil {
				return nil, nil, fmt.Errorf("argument syntax: %w", err)
			}
			valueSyntax := ""
			if len(parts) == 2 {
				valueSyntax = parts[1]
			} else if index+1 < len(tokens) && tokens[index+1].Optional == token.Optional && isArgumentValueSyntax(tokens[index+1].Value) {
				index++
				valueSyntax = tokens[index].Value
			}
			allowed, err := argumentSyntaxAllowedValues(valueSyntax)
			if err != nil {
				return nil, nil, err
			}
			if _, exists := inputs[name]; exists {
				return nil, nil, fmt.Errorf("argument syntax input %q is declared more than once", name)
			}
			inputs[name] = argumentSyntaxInput{Required: !token.Optional, AllowedValues: allowed, TakesValue: valueSyntax != ""}
			continue
		}

		repeatable := strings.HasSuffix(token.Value, "...")
		positionalToken := strings.TrimSuffix(token.Value, "...")
		if strings.HasPrefix(positionalToken, "<") && strings.HasSuffix(positionalToken, ">") {
			name := strings.Trim(positionalToken, "<>")
			if err := validateInputName(CommandInput{Name: name, Source: InputSourceArgument}); err != nil {
				return nil, nil, fmt.Errorf("argument syntax: %w", err)
			}
			if _, exists := inputs[name]; exists {
				return nil, nil, fmt.Errorf("argument syntax input %q is declared more than once", name)
			}
			if !token.Optional && optionalPositionalSeen {
				return nil, nil, fmt.Errorf("required positional input %q follows an optional positional input", name)
			}
			optionalPositionalSeen = optionalPositionalSeen || token.Optional
			positionals = append(positionals, name)
			inputs[name] = argumentSyntaxInput{Required: !token.Optional, AllowedValues: []string{}, TakesValue: true, Repeatable: repeatable, PositionalOnly: positionalOnly}
			positionalAfterMarker = positionalAfterMarker || positionalOnly
			continue
		}

		if token.Optional && !strings.ContainsAny(positionalToken, "|<>=") {
			if err := validateInputName(CommandInput{Name: positionalToken, Source: InputSourceArgument}); err != nil {
				return nil, nil, fmt.Errorf("argument syntax: %w", err)
			}
			if _, exists := inputs[positionalToken]; exists {
				return nil, nil, fmt.Errorf("argument syntax input %q is declared more than once", positionalToken)
			}
			optionalPositionalSeen = true
			positionals = append(positionals, positionalToken)
			inputs[positionalToken] = argumentSyntaxInput{Required: false, AllowedValues: []string{}, TakesValue: true, Repeatable: repeatable, PositionalOnly: positionalOnly}
			positionalAfterMarker = positionalAfterMarker || positionalOnly
			continue
		}
		return nil, nil, fmt.Errorf("argument syntax token %q is outside the supported grammar", token.Value)
	}
	if positionalOnly && !positionalAfterMarker {
		return nil, nil, fmt.Errorf("argument syntax positional-only marker has no positional input")
	}
	return inputs, positionals, nil
}

func isArgumentValueSyntax(value string) bool {
	return (strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) || strings.Contains(value, "|")
}

func argumentSyntaxAllowedValues(value string) ([]string, error) {
	if value == "" || (strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) {
		return []string{}, nil
	}
	values := strings.Split(value, "|")
	for _, candidate := range values {
		if err := validateContractText("argument syntax value", candidate); err != nil || strings.ContainsAny(candidate, "[]()<>|=") {
			return nil, fmt.Errorf("argument syntax value %q is invalid", candidate)
		}
	}
	return values, nil
}

func equalStrings(left, right []string) bool {
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

func validateContractText(label, value string) error {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
		strings.IndexFunc(value, isUnsafeContractRune) >= 0 {
		return fmt.Errorf("agent %s is missing or invalid", label)
	}
	return nil
}

func isUnsafeContractRune(r rune) bool {
	return unicode.Is(unicode.C, r) || r == '\u2028' || r == '\u2029'
}

// ProducedRefs derives the opaque references exposed by structured output.
// Catalog validation rejects declarations that exceed the bounded recursive
// output shape, so a derivation error here can only belong to an invalid spec.
func (s CommandSpec) ProducedRefs() []ProducedRef {
	references, err := s.producedRefs()
	if err != nil {
		return nil
	}
	return references
}

func (s CommandSpec) producedRefs() ([]ProducedRef, error) {
	references := make([]ProducedRef, 0, len(s.Agent.Output.Fields)+1)
	count := 0
	for _, field := range s.Agent.Output.Fields {
		if err := appendOutputFieldProducedRefs(&references, field, field.Name, 1, &count); err != nil {
			return nil, err
		}
	}
	if pagination := s.Agent.Pagination; pagination != nil && pagination.CursorOutput.ReferenceKind != "" {
		references = append(references, ProducedRef{
			Kind:  pagination.CursorOutput.ReferenceKind,
			Field: pagination.CursorOutput.Name,
		})
	}
	seen := make(map[string]string, len(references))
	for _, reference := range references {
		if previous, duplicate := seen[reference.Field]; duplicate {
			return nil, fmt.Errorf("produced reference field path %q is declared for both %q and %q", reference.Field, previous, reference.Kind)
		}
		seen[reference.Field] = reference.Kind
	}
	return references, nil
}

// appendOutputFieldProducedRefs is the single recursive producer-reference
// derivation. Paths describe the declared value shape rather than a renderer:
// object children use dots and array items use []. Declaration order is kept.
func appendOutputFieldProducedRefs(references *[]ProducedRef, field OutputField, path string, depth int, count *int) error {
	*count++
	if *count > maxOutputFieldCount {
		return fmt.Errorf("produced reference traversal exceeds maximum field count %d", maxOutputFieldCount)
	}
	if depth > maxOutputFieldDepth {
		return fmt.Errorf("produced reference traversal exceeds maximum depth %d", maxOutputFieldDepth)
	}
	if field.ReferenceKind != "" {
		*references = append(*references, ProducedRef{Kind: field.ReferenceKind, Field: path})
	}
	for _, child := range field.Fields {
		if err := appendOutputFieldProducedRefs(references, child, path+"."+child.Name, depth+1, count); err != nil {
			return err
		}
	}
	if field.Items != nil {
		if err := appendOutputFieldProducedRefs(references, *field.Items, path+"[]", depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

// ConsumedRefs derives the opaque references accepted by structured input.
func (s CommandSpec) ConsumedRefs() []ConsumedRef {
	references := make([]ConsumedRef, 0)
	for _, input := range s.Agent.Inputs {
		if input.ReferenceKind != "" {
			references = append(references, ConsumedRef{Kind: input.ReferenceKind, Argument: input.Name})
		}
	}
	return references
}

func validateCommandReferenceRole(command CommandSpec) error {
	produced, err := command.producedRefs()
	if err != nil {
		return err
	}
	for _, reference := range produced {
		if err := validateReferenceName(reference.Kind); err != nil {
			return fmt.Errorf("produced reference kind: %w", err)
		}
	}

	consumed := command.ConsumedRefs()
	for _, reference := range consumed {
		if err := validateReferenceName(reference.Kind); err != nil {
			return fmt.Errorf("consumed reference kind: %w", err)
		}
	}
	if command.Effect != operation.EffectRead && command.Role != RoleAct {
		return fmt.Errorf("mutating commands must use the act role")
	}

	switch command.Role {
	case RoleUtility:
		if command.Agent.FixedTarget != nil {
			return fmt.Errorf("only act commands may declare a fixed target")
		}
		if len(produced) != 0 || len(consumed) != 0 {
			return fmt.Errorf("utility commands must not produce or consume references")
		}
	case RoleDiscover:
		if command.Agent.FixedTarget != nil {
			return fmt.Errorf("only act commands may declare a fixed target")
		}
		if command.Effect != operation.EffectRead {
			return fmt.Errorf("discover commands must have read effect")
		}
		if len(produced) == 0 {
			return fmt.Errorf("discover commands must produce at least one reference")
		}
	case RoleAct:
		if command.Agent.FixedTarget != nil {
			if len(consumed) != 0 {
				return fmt.Errorf("fixed-target act commands must not consume references")
			}
			if len(produced) != 0 {
				if command.Effect != operation.EffectCreate {
					return fmt.Errorf("only fixed-target create commands may produce references")
				}
				for _, reference := range produced {
					if reference.Kind == command.Agent.FixedTarget.Kind {
						return fmt.Errorf("fixed-target create commands must not produce their creation-scope reference kind %q", reference.Kind)
					}
				}
			}
			return nil
		}
		if len(consumed) == 0 {
			return fmt.Errorf("act commands must consume at least one reference")
		}
		hasRequiredReference := false
		for _, input := range command.Agent.Inputs {
			if input.Required && input.ReferenceKind != "" {
				hasRequiredReference = true
				break
			}
		}
		if !hasRequiredReference && (command.Agent.Mutation == nil || !command.Agent.Mutation.CurrentContextFallback) {
			return fmt.Errorf("act commands must require at least one opaque reference")
		}
	}
	return nil
}

func validateFixedTarget(target FixedTarget) error {
	if err := validateReferenceName(target.Kind); err != nil {
		return fmt.Errorf("fixed target kind: %w", err)
	}
	if err := validateReferenceName(target.ID); err != nil {
		return fmt.Errorf("fixed target ID: %w", err)
	}
	if err := validateContractText("fixed target description", target.Description); err != nil {
		return err
	}
	if target.Scope != FixedTargetScopeToolLocal {
		return fmt.Errorf("fixed target scope must be %q", FixedTargetScopeToolLocal)
	}
	return nil
}

func validateAgentIndexEntry(command CommandSpec) error {
	encoded, err := json.Marshal(projectAgentIndexCommand(command))
	if err != nil {
		return fmt.Errorf("agent index entry cannot be encoded: %w", err)
	}
	if len(encoded) > maxAgentIndexEntryBytes {
		return fmt.Errorf("agent index entry is %d bytes; maximum is %d", len(encoded), maxAgentIndexEntryBytes)
	}
	return nil
}

// validateReferenceReachability rejects closed reference cycles. A kind is
// reachable only when some producer can run after all of its required opaque
// inputs are themselves reachable. Optional inputs, including a first-page
// cursor, do not prevent a command from seeding a workflow.
func validateReferenceReachability(commands []CommandSpec) error {
	reachable := make(map[string]struct{})
	for {
		progress := false
		for _, command := range commands {
			ready := true
			for _, input := range command.Agent.Inputs {
				if !input.Required || input.ReferenceKind == "" {
					continue
				}
				if _, exists := reachable[input.ReferenceKind]; !exists {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			for _, produced := range command.ProducedRefs() {
				if _, exists := reachable[produced.Kind]; exists {
					continue
				}
				reachable[produced.Kind] = struct{}{}
				progress = true
			}
		}
		if !progress {
			break
		}
	}

	for _, command := range commands {
		for _, produced := range command.ProducedRefs() {
			if _, exists := reachable[produced.Kind]; !exists {
				return fmt.Errorf("reference kind %q is trapped in a closed required-reference cycle", produced.Kind)
			}
		}
	}
	return nil
}

func validateReferenceName(value string) error {
	if value == "" {
		return fmt.Errorf("reference name is empty")
	}
	for index, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case index > 0 && r >= '0' && r <= '9':
		case index > 0 && r == '-':
		default:
			return fmt.Errorf("reference name is invalid: %q", value)
		}
	}
	return nil
}

func validateOutputFieldName(value string) error {
	if value == "" {
		return fmt.Errorf("output field name is empty")
	}
	for index, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case index > 0 && r >= '0' && r <= '9':
		case index > 0 && (r == '-' || r == '_'):
		default:
			return fmt.Errorf("output field name is invalid: %q", value)
		}
	}
	return nil
}

// Commands returns a copy in the curated display order.
func (c Catalog) Commands() []CommandSpec {
	commands := make([]CommandSpec, 0, len(c.commands))
	for _, command := range c.commands {
		if command.Visibility == CommandVisibilityInternal || command.programName() != c.programName() {
			continue
		}
		commands = append(commands, c.publicCommandProjection(command))
	}
	return commands
}

// PublicCommands returns every public command across Program boundaries in
// curated registration order. Repository contract tools use this global view;
// routing and help continue to use the exact Commands Program view.
func (c Catalog) PublicCommands() []CommandSpec {
	commands := make([]CommandSpec, 0, len(c.commands))
	for _, command := range c.commands {
		if command.Visibility == CommandVisibilityInternal {
			continue
		}
		commands = append(commands, c.publicCommandProjection(command))
	}
	return commands
}

// registeredCommands returns the detached global registry for contract tests
// and catalog-owned composition. Public routing and help must use Commands.
func (c Catalog) registeredCommands() []CommandSpec {
	commands := make([]CommandSpec, 0, len(c.commands))
	for _, command := range c.commands {
		commands = append(commands, cloneCommandSpec(command))
	}
	return commands
}

// Lookup finds one exact command path.
func (c Catalog) Lookup(path string) (CommandSpec, bool) {
	for _, command := range c.commands {
		if command.programName() == c.programName() && command.Path == path && command.Visibility == CommandVisibilityPublic {
			return c.publicCommandProjection(command), true
		}
	}
	return CommandSpec{}, false
}

func (c Catalog) publicCommandProjection(command CommandSpec) CommandSpec {
	projected := cloneCommandSpec(command)
	if projected.Agent.Interactive == nil {
		return projected
	}
	for _, action := range projected.Agent.Interactive.actionCommands() {
		registered, found := c.lookupRegisteredForProgram(command.programName(), action)
		if found && registered.Visibility == CommandVisibilityInternal {
			projected.Agent.projectedInteractiveMutationErrors = projected.Agent.Interactive.ProjectsActionErrors
			projected.Agent.Interactive = nil
			break
		}
	}
	return projected
}

// lookupRegistered resolves public and internal entries for catalog-owned
// composition. It is deliberately unexported so public routing cannot use it.
func (c Catalog) lookupRegistered(path string) (CommandSpec, bool) {
	return c.lookupRegisteredForProgram(c.programName(), path)
}

func (c Catalog) lookupRegisteredForProgram(program, path string) (CommandSpec, bool) {
	for _, command := range c.commands {
		if command.programName() == program && command.Path == path {
			return cloneCommandSpec(command), true
		}
	}
	return CommandSpec{}, false
}

// Match selects the longest catalog path that prefixes args.
func (c Catalog) Match(args []string) (CommandSpec, []string, bool) {
	var (
		matched      CommandSpec
		matchedWords int
	)
	for _, command := range c.commands {
		if command.Visibility == CommandVisibilityInternal || command.programName() != c.programName() {
			continue
		}
		words := strings.Split(command.Path, " ")
		if len(words) <= matchedWords || len(words) > len(args) {
			continue
		}
		match := true
		for index := range words {
			if args[index] != words[index] {
				match = false
				break
			}
		}
		if match {
			matched = command
			matchedWords = len(words)
		}
	}
	if matchedWords == 0 {
		return CommandSpec{}, nil, false
	}
	return cloneCommandSpec(matched), args[matchedWords:], true
}

func cloneCommandSpec(command CommandSpec) CommandSpec {
	command.Agent = cloneAgentContract(command.Agent)
	return command
}

func cloneAgentContract(contract AgentContract) AgentContract {
	contract.Inputs = cloneSlice(contract.Inputs)
	for index := range contract.Inputs {
		contract.Inputs[index].AllowedValues = cloneSlice(contract.Inputs[index].AllowedValues)
		contract.Inputs[index].Requires = cloneSlice(contract.Inputs[index].Requires)
		contract.Inputs[index].ConflictsWith = cloneSlice(contract.Inputs[index].ConflictsWith)
		if contract.Inputs[index].DefaultValue != nil {
			value := *contract.Inputs[index].DefaultValue
			contract.Inputs[index].DefaultValue = &value
		}
		if contract.Inputs[index].Minimum != nil {
			value := *contract.Inputs[index].Minimum
			contract.Inputs[index].Minimum = &value
		}
		if contract.Inputs[index].Maximum != nil {
			value := *contract.Inputs[index].Maximum
			contract.Inputs[index].Maximum = &value
		}
		if contract.Inputs[index].MinimumLength != nil {
			value := *contract.Inputs[index].MinimumLength
			contract.Inputs[index].MinimumLength = &value
		}
		if contract.Inputs[index].MaximumLength != nil {
			value := *contract.Inputs[index].MaximumLength
			contract.Inputs[index].MaximumLength = &value
		}
	}
	contract.Output.Formats = cloneSlice(contract.Output.Formats)
	contract.Output.Fields = cloneOutputFields(contract.Output.Fields)
	if contract.Pagination != nil {
		pagination := *contract.Pagination
		contract.Pagination = &pagination
	}
	contract.Prerequisites = cloneSlice(contract.Prerequisites)
	if contract.FixedTarget != nil {
		fixedTarget := *contract.FixedTarget
		contract.FixedTarget = &fixedTarget
	}
	contract.Errors = cloneSlice(contract.Errors)
	for index := range contract.Errors {
		contract.Errors[index].NextActions = cloneSlice(contract.Errors[index].NextActions)
	}
	if contract.Mutation != nil {
		mutation := *contract.Mutation
		mutation.TargetInputs = cloneSlice(mutation.TargetInputs)
		contract.Mutation = &mutation
	}
	if contract.Interactive != nil {
		interactive := *contract.Interactive
		contract.Interactive = &interactive
	}
	return contract
}

func cloneOutputFields(fields []OutputField) []OutputField {
	if fields == nil {
		return nil
	}
	cloned := make([]OutputField, len(fields))
	for index, field := range fields {
		cloned[index] = field
		cloned[index].Enum = cloneSlice(field.Enum)
		cloned[index].Fields = cloneOutputFields(field.Fields)
		if field.Items != nil {
			item := cloneOutputFields([]OutputField{*field.Items})[0]
			cloned[index].Items = &item
		}
	}
	return cloned
}

func cloneSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	cloned := make([]T, len(values))
	copy(cloned, values)
	return cloned
}
