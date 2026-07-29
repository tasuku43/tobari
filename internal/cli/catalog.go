package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/authn"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
)

const (
	// ProgramName is intentionally a single bootstrap replacement token.
	ProgramName = "agentic-cli-foundry"

	// maxAgentIndexEntryBytes bounds the selection-only root help cost per
	// command. Detailed invocation contracts belong in scoped help.
	maxAgentIndexEntryBytes = 512
)

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

// FixedTargetScope identifies where a command-bound target is owned. The
// template currently permits only a singleton owned by this CLI installation.
type FixedTargetScope string

const (
	FixedTargetScopeUnknown   FixedTargetScope = ""
	FixedTargetScopeToolLocal FixedTargetScope = "tool_local"
)

// FixedTarget identifies one stable target selected entirely by the command
// path. It is mutually exclusive with produced or consumed opaque references.
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

// CommandInput is one executable machine-readable input contract.
// DefaultValue is nil when omission has no catalog-owned default. Minimum and
// Maximum apply only to integer values. Requires and ConflictsWith are checked
// against explicitly supplied command-line inputs. ReferenceKind is empty only
// when the input is not an opaque reference.
type CommandInput struct {
	Name          string           `json:"name"`
	Source        InputSource      `json:"source"`
	Required      bool             `json:"required"`
	ValueKind     InputValueKind   `json:"value_kind"`
	Cardinality   InputCardinality `json:"cardinality"`
	Description   string           `json:"description"`
	AllowedValues []string         `json:"allowed_values"`
	DefaultValue  *string          `json:"default_value,omitempty"`
	Minimum       *int64           `json:"minimum,omitempty"`
	Maximum       *int64           `json:"maximum,omitempty"`
	Requires      []string         `json:"requires,omitempty"`
	ConflictsWith []string         `json:"conflicts_with,omitempty"`
	ReferenceKind string           `json:"reference_kind,omitempty"`
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
	ReferenceKind string          `json:"reference_kind,omitempty"`
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

// CommandOutput is the stable logical result and its supported presentations.
// Fields describe values inside JSONEnvelope, never top-level metadata.
type CommandOutput struct {
	Formats            []OutputFormat     `json:"formats"`
	DefaultFormat      OutputFormat       `json:"default_format"`
	Fields             []OutputField      `json:"fields"`
	Delivery           OutputDelivery     `json:"delivery"`
	CollectionCoverage CollectionCoverage `json:"collection_coverage"`
	JSONEnvelope       string             `json:"json_envelope,omitempty"`
	JSONSchemaVersion  int                `json:"json_schema_version,omitempty"`
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
	Retryable   bool               `json:"retryable"`
	NextActions []fault.NextAction `json:"next_actions"`
}

// MutationContract connects a mutating command's public inputs to the target
// and generic impact facts consumed by the project-specific policy gate.
type MutationContract struct {
	TargetKind    string           `json:"target_kind"`
	TargetInputs  []string         `json:"target_inputs"`
	ParentInput   string           `json:"parent_input,omitempty"`
	TargetIDInput string           `json:"target_id_input,omitempty"`
	Impact        operation.Impact `json:"impact"`
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
		TargetKind    string         `json:"target_kind"`
		TargetInputs  []string       `json:"target_inputs"`
		ParentInput   string         `json:"parent_input,omitempty"`
		TargetIDInput string         `json:"target_id_input,omitempty"`
		Impact        impactDocument `json:"impact"`
	}
	return json.Marshal(mutationDocument{
		TargetKind: m.TargetKind, TargetInputs: m.TargetInputs,
		ParentInput: m.ParentInput, TargetIDInput: m.TargetIDInput,
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
	CapabilityID   string              `json:"capability_id"`
	Outcome        string              `json:"outcome"`
	Inputs         []CommandInput      `json:"inputs"`
	Output         CommandOutput       `json:"output"`
	Pagination     *PaginationContract `json:"pagination,omitempty"`
	Prerequisites  []string            `json:"prerequisites"`
	Authentication *authn.Requirement  `json:"authentication,omitempty"`
	FixedTarget    *FixedTarget        `json:"fixed_target,omitempty"`
	Errors         []CommandError      `json:"errors"`
	Mutation       *MutationContract   `json:"mutation,omitempty"`
}

// CommandSpec is the single source of truth for dispatch, human help, and the
// machine-readable agent specification.
type CommandSpec struct {
	Path    string
	Summary string
	Args    string
	Effect  operation.Effect
	Role    CommandRole
	Agent   AgentContract
	handler commandHandler
}

// Usage returns the complete command invocation without optional prose.
func (s CommandSpec) Usage() string {
	usage := ProgramName + " " + s.Path
	if s.Args != "" {
		usage += " " + s.Args
	}
	return usage
}

// Catalog owns the complete set of public command paths.
type Catalog struct {
	commands []CommandSpec
}

// NewCatalog creates a catalog from declarative command specifications.
func NewCatalog(commands ...CommandSpec) Catalog {
	cloned := make([]CommandSpec, len(commands))
	for index, command := range commands {
		cloned[index] = cloneCommandSpec(command)
	}
	return Catalog{commands: cloned}
}

func declaredCommandError(kind fault.Kind, code string, retryable bool, command, reason string) CommandError {
	return CommandError{
		Kind:        kind,
		Code:        code,
		Retryable:   retryable,
		NextActions: []fault.NextAction{{Command: command, Reason: reason}},
	}
}

func stringPointer(value string) *string {
	return &value
}

// DefaultCatalog returns the public CLI contract.
func DefaultCatalog() Catalog {
	return NewCatalog(
		CommandSpec{
			Path:    "doctor",
			Summary: "Run local, read-only diagnostics",
			Args:    "[--format tsv|json]",
			Effect:  operation.EffectRead,
			Role:    RoleUtility,
			Agent: AgentContract{
				CapabilityID: "system.diagnostics",
				Outcome:      "Inspect the local runtime and receive a validated diagnostic report",
				Inputs: []CommandInput{
					{
						Name: "--format", Source: InputSourceFlag, Required: false,
						ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
						Description: "Select the complete report representation.", AllowedValues: []string{"tsv", "json"},
						DefaultValue: stringPointer("tsv"),
					},
				},
				Output: CommandOutput{
					Formats:       []OutputFormat{OutputFormatTSV, OutputFormatJSON},
					DefaultFormat: OutputFormatTSV,
					Fields: []OutputField{
						{Name: "check", Type: OutputFieldTypeString, Description: "Stable diagnostic name with unsafe structural runes rendered as visible escapes."},
						{Name: "status", Type: OutputFieldTypeString, Description: "Diagnostic result: pass, warn, or fail."},
						{Name: "detail", Type: OutputFieldTypeString, Description: "Diagnostic detail with unsafe structural runes rendered as visible escapes."},
					},
					Delivery:           OutputDeliveryComplete,
					CollectionCoverage: CollectionCoverageExhaustive,
					JSONEnvelope:       "report",
					JSONSchemaVersion:  1,
				},
				Prerequisites: []string{},
				Errors: []CommandError{
					declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help doctor", "Correct the command arguments."),
					declaredCommandError(fault.KindRejected, "diagnostic_failed", false, "doctor", "Review the failed diagnostic and correct the local prerequisite."),
					declaredCommandError(fault.KindContract, "output_contract_exceeded", false, "doctor", "Review the bounded output contract and diagnostic adapter."),
					declaredCommandError(fault.KindContract, "output_encoding_failed", false, "doctor", "Repair the diagnostic JSON projection."),
					declaredCommandError(fault.KindInternal, "internal_error", false, "doctor", "Retry after investigating the local diagnostic adapter."),
					declaredCommandError(fault.KindInternal, "output_write_failed", true, "doctor", "Retry with a writable output stream."),
					declaredCommandError(fault.KindCanceled, "operation_canceled", true, "doctor", "Retry when the caller is ready."),
				},
			},
			handler: runDoctor,
		},
		CommandSpec{
			Path:    "help",
			Summary: "Show human help or the agent command specification",
			Args:    "[command] [--format text|agent]",
			Effect:  operation.EffectRead,
			Role:    RoleUtility,
			Agent: AgentContract{
				CapabilityID: "cli.discovery",
				Outcome:      "Discover command usage, contracts, workflows, and next actions without external I/O",
				Inputs: []CommandInput{
					{
						Name: "command", Source: InputSourceArgument, Required: false,
						ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable,
						Description: "Select an exact command path or canonical command namespace as one or more path words.", AllowedValues: []string{},
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
					DefaultFormat: OutputFormatText,
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
					JSONSchemaVersion:  6,
				},
				Prerequisites: []string{},
				Errors: []CommandError{
					declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help", "Use text or agent format and an exact catalog command path."),
					declaredCommandError(fault.KindContract, "output_encoding_failed", false, "help", "Repair the agent help JSON projection."),
					declaredCommandError(fault.KindInternal, "output_write_failed", true, "help", "Retry with a writable output stream."),
					declaredCommandError(fault.KindCanceled, "operation_canceled", true, "help", "Retry when the caller is ready."),
				},
			},
			handler: runHelp,
		},
		CommandSpec{
			Path:    "sample list",
			Summary: "Discover offline samples and their opaque IDs",
			Args:    "[--format tsv|json]",
			Effect:  operation.EffectRead,
			Role:    RoleDiscover,
			Agent: AgentContract{
				CapabilityID: "sample.inspect",
				Outcome:      "Discover every offline sample and its stable opaque reference",
				Inputs: []CommandInput{
					{
						Name: "--format", Source: InputSourceFlag, Required: false,
						ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
						Description: "Select the complete sample collection representation.", AllowedValues: []string{"tsv", "json"},
						DefaultValue: stringPointer("tsv"),
					},
				},
				Output: CommandOutput{
					Formats:       []OutputFormat{OutputFormatTSV, OutputFormatJSON},
					DefaultFormat: OutputFormatTSV,
					Fields: []OutputField{
						{Name: "id", Type: OutputFieldTypeString, Description: "Opaque sample reference accepted unchanged by sample read.", ReferenceKind: "sample"},
						{Name: "name", Type: OutputFieldTypeString, Description: "Human-readable label with unsafe structural runes visibly escaped; never use it as an identifier."},
					},
					Delivery:           OutputDeliveryComplete,
					CollectionCoverage: CollectionCoverageExhaustive,
					JSONEnvelope:       "items",
					JSONSchemaVersion:  1,
				},
				Prerequisites: []string{},
				Errors: []CommandError{
					declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help sample list", "Correct the command arguments."),
					declaredCommandError(fault.KindUnavailable, "page_fetch_failed", true, "sample list", "Retry after the sample source is available."),
					declaredCommandError(fault.KindContract, "invalid_page_contract", false, "sample list", "Inspect the sample adapter page contract."),
					declaredCommandError(fault.KindContract, "pagination_page_limit", false, "sample list", "Review the declared pagination page budget."),
					declaredCommandError(fault.KindContract, "pagination_item_limit", false, "sample list", "Review the declared pagination item budget."),
					declaredCommandError(fault.KindContract, "pagination_cursor_loop", false, "sample list", "Inspect the adapter cursor sequence."),
					declaredCommandError(fault.KindContract, "output_contract_exceeded", false, "sample list", "Review the bounded output contract and sample adapter."),
					declaredCommandError(fault.KindContract, "output_encoding_failed", false, "sample list", "Repair the sample list JSON projection."),
					declaredCommandError(fault.KindInternal, "internal_error", false, "sample list", "Inspect the sample adapter and returned items."),
					declaredCommandError(fault.KindInternal, "output_write_failed", true, "sample list", "Retry with a writable output stream."),
					declaredCommandError(fault.KindCanceled, "operation_canceled", true, "sample list", "Retry when the caller is ready."),
				},
			},
			handler: runSampleList,
		},
		CommandSpec{
			Path:    "sample read",
			Summary: "Read exactly one offline sample by opaque ID",
			Args:    "--id <sample-id> [--format tsv|json]",
			Effect:  operation.EffectRead,
			Role:    RoleAct,
			Agent: AgentContract{
				CapabilityID: "sample.inspect",
				Outcome:      "Read one uniquely identified offline sample without rediscovery",
				Inputs: []CommandInput{
					{
						Name: "--id", Source: InputSourceFlag, Required: true,
						ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
						Description: "Pass an id from sample list byte-for-byte without parsing or transformation.", AllowedValues: []string{}, ReferenceKind: "sample",
					},
					{
						Name: "--format", Source: InputSourceFlag, Required: false,
						ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
						Description: "Select the single sample representation.", AllowedValues: []string{"tsv", "json"},
						DefaultValue: stringPointer("tsv"),
					},
				},
				Output: CommandOutput{
					Formats:       []OutputFormat{OutputFormatTSV, OutputFormatJSON},
					DefaultFormat: OutputFormatTSV,
					Fields: []OutputField{
						{Name: "id", Type: OutputFieldTypeString, Description: "Exact opaque sample ID requested by the caller."},
						{Name: "name", Type: OutputFieldTypeString, Description: "Human-readable label with unsafe structural runes rendered as visible escapes."},
						{Name: "content", Type: OutputFieldTypeString, Description: "Complete content with unsafe structural runes rendered as visible escapes."},
					},
					Delivery:           OutputDeliveryComplete,
					CollectionCoverage: CollectionCoverageNotApplicable,
					JSONEnvelope:       "item",
					JSONSchemaVersion:  1,
				},
				Prerequisites: []string{},
				Errors: []CommandError{
					declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help sample read", "Pass exactly one opaque sample ID through --id and choose a supported format."),
					declaredCommandError(fault.KindNotFound, "sample_not_found", false, "sample list", "Discover a current opaque sample ID."),
					declaredCommandError(fault.KindContract, "output_contract_exceeded", false, "sample read", "Review the bounded output contract and sample adapter."),
					declaredCommandError(fault.KindContract, "output_encoding_failed", false, "sample read", "Repair the sample JSON projection."),
					declaredCommandError(fault.KindInternal, "internal_error", false, "sample read", "Inspect the sample adapter and returned item."),
					declaredCommandError(fault.KindInternal, "output_write_failed", true, "sample read", "Retry with a writable output stream."),
					declaredCommandError(fault.KindCanceled, "operation_canceled", true, "sample read", "Retry when the caller is ready."),
				},
			},
			handler: runSampleRead,
		},
		CommandSpec{
			Path:    "version",
			Summary: "Print version information",
			Effect:  operation.EffectRead,
			Role:    RoleUtility,
			Agent: AgentContract{
				CapabilityID: "cli.version",
				Outcome:      "Read the executable version and optional source commit identity",
				Inputs:       []CommandInput{},
				Output: CommandOutput{
					Formats:       []OutputFormat{OutputFormatText},
					DefaultFormat: OutputFormatText,
					Fields: []OutputField{
						{Name: "version", Type: OutputFieldTypeString, Description: "Release version embedded in the executable."},
						{Name: "commit", Type: OutputFieldTypeString, Description: "Optional source commit embedded in the executable."},
					},
					Delivery:           OutputDeliveryComplete,
					CollectionCoverage: CollectionCoverageNotApplicable,
				},
				Prerequisites: []string{},
				Errors: []CommandError{
					declaredCommandError(fault.KindInvalidInput, "invalid_arguments", false, "help version", "Run version without command arguments."),
					declaredCommandError(fault.KindInternal, "output_write_failed", true, "version", "Retry with a writable output stream."),
					declaredCommandError(fault.KindCanceled, "operation_canceled", true, "version", "Retry when the caller is ready."),
				},
			},
			handler: runVersion,
		},
	)
}

// Validate rejects incomplete command declarations before any handler runs.
func (c Catalog) Validate() error {
	if len(c.commands) == 0 {
		return fmt.Errorf("command catalog is empty")
	}
	seen := make(map[string]struct{}, len(c.commands))
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
		if err := validateAgentContract(command); err != nil {
			return fmt.Errorf("catalog command %q: %w", command.Path, err)
		}
		if err := validateAgentIndexEntry(command); err != nil {
			return fmt.Errorf("catalog command %q: %w", command.Path, err)
		}
		if err := validateCommandReferenceRole(command); err != nil {
			return fmt.Errorf("catalog command %q: %w", command.Path, err)
		}
		if command.handler == nil {
			return fmt.Errorf("catalog command %q has no handler", command.Path)
		}
		for existing := range seen {
			if strings.HasPrefix(command.Path, existing+" ") || strings.HasPrefix(existing, command.Path+" ") {
				return fmt.Errorf("catalog command paths %q and %q collide at a command/namespace boundary", existing, command.Path)
			}
		}
		if _, exists := seen[command.Path]; exists {
			return fmt.Errorf("catalog contains duplicate command %q", command.Path)
		}
		seen[command.Path] = struct{}{}
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
				nextCommand, err := c.resolveRecoveryCommand(action.Command)
				if err != nil {
					return fmt.Errorf("catalog command %q error %q: %w", command.Path, declaredError.Code, err)
				}
				requiresReadOnlyRecovery := declaredError.Code == "unclassified_mutation_outcome" ||
					declaredError.Code == "mutation_output_write_failed" ||
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
	words := strings.Fields(value)
	if len(words) == 0 || strings.Join(words, " ") != value {
		return CommandSpec{}, fmt.Errorf("next command %q is not canonical", value)
	}
	if command, found := c.Lookup(value); found {
		return command, nil
	}
	if words[0] != "help" || len(words) == 1 {
		return CommandSpec{}, fmt.Errorf("next command %q is not an exact catalog path", value)
	}
	help, found := c.Lookup("help")
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
	selected, _ := c.Select(selector)
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
		}
		if input.ValueKind != InputValueInteger && (input.Minimum != nil || input.Maximum != nil) {
			return fmt.Errorf("agent non-integer input %q cannot declare numeric bounds", input.Name)
		}
		if input.Minimum != nil && input.Maximum != nil && *input.Minimum > *input.Maximum {
			return fmt.Errorf("agent integer input %q minimum exceeds maximum", input.Name)
		}
		if input.ValueKind == InputValueBoolean && len(input.AllowedValues) != 0 {
			return fmt.Errorf("agent boolean input %q uses the fixed true/false grammar rather than allowed values", input.Name)
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
	seenFields := make(map[string]struct{}, len(contract.Output.Fields))
	for index, field := range contract.Output.Fields {
		if err := validateOutputFieldName(field.Name); err != nil {
			return fmt.Errorf("agent output field %d: %w", index, err)
		}
		if err := field.Type.validate(); err != nil {
			return fmt.Errorf("agent output field %q: %w", field.Name, err)
		}
		if err := validateContractText("output field description", field.Description); err != nil {
			return fmt.Errorf("agent output field %q: %w", field.Name, err)
		}
		if _, exists := seenFields[field.Name]; exists {
			return fmt.Errorf("agent output field %q is declared more than once", field.Name)
		}
		seenFields[field.Name] = struct{}{}
		if field.ReferenceKind != "" {
			if err := validateReferenceName(field.ReferenceKind); err != nil {
				return fmt.Errorf("agent output field %q reference kind: %w", field.Name, err)
			}
			if field.Type != OutputFieldTypeString {
				return fmt.Errorf("agent output reference field %q must have string type", field.Name)
			}
		}
	}
	if err := contract.Output.Delivery.validate(); err != nil {
		return err
	}
	if err := contract.Output.CollectionCoverage.validate(); err != nil {
		return err
	}
	if _, none := seenFormats[OutputFormatNone]; none && contract.Output.CollectionCoverage != CollectionCoverageNotApplicable {
		return fmt.Errorf("none output format requires collection coverage %q", CollectionCoverageNotApplicable)
	}
	_, supportsJSON := seenFormats[OutputFormatJSON]
	if supportsJSON {
		if err := validateOutputFieldName(contract.Output.JSONEnvelope); err != nil {
			return fmt.Errorf("agent JSON envelope: %w", err)
		}
		if contract.Output.JSONSchemaVersion <= 0 {
			return fmt.Errorf("agent JSON schema version must be positive")
		}
	} else if contract.Output.JSONEnvelope != "" || contract.Output.JSONSchemaVersion != 0 {
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
	if contract.Authentication != nil {
		if err := contract.Authentication.Validate(); err != nil {
			return fmt.Errorf("agent authentication requirement: %w", err)
		}
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
	_, hasMutationOutputFailure := seenErrors["mutation_output_write_failed"]
	if command.Effect == operation.EffectRead && hasMutationOutputFailure {
		return fmt.Errorf("read command must not declare mutation_output_write_failed")
	}
	if command.Effect != operation.EffectRead && hasReadOutputFailure {
		return fmt.Errorf("mutating command must not declare retryable output_write_failed")
	}
	if noOutput && (hasReadOutputFailure || hasMutationOutputFailure) {
		return fmt.Errorf("command without output must not declare an output write failure")
	}
	if !noOutput && command.Effect == operation.EffectRead {
		if err := requireAgentError(seenErrors, "output_write_failed", fault.KindInternal, true); err != nil {
			return err
		}
	}
	if contract.Authentication != nil {
		for _, required := range []struct {
			code      string
			kind      fault.Kind
			retryable bool
		}{
			{code: "missing_authentication_context", kind: fault.KindContract},
			{code: "missing_authenticated_action", kind: fault.KindContract},
			{code: "invalid_authentication_requirement", kind: fault.KindContract},
			{code: "missing_authenticator", kind: fault.KindAuthentication},
			{code: "missing_authentication_clock", kind: fault.KindContract},
			{code: "invalid_authentication_session", kind: fault.KindAuthentication},
			{code: "authentication_evaluation_failed", kind: fault.KindContract},
			{code: "insufficient_authentication_capability", kind: fault.KindPermission},
			{code: "authentication_expired", kind: fault.KindAuthentication},
			{code: "authentication_context_mismatch", kind: fault.KindAuthentication},
			{code: "authentication_failed", kind: fault.KindAuthentication},
			{code: "authentication_canceled", kind: fault.KindCanceled},
			{code: "unclassified_authenticated_action_error", kind: fault.KindInternal},
		} {
			if err := requireAgentError(seenErrors, required.code, required.kind, required.retryable); err != nil {
				return err
			}
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
		parent, err := validateMutationBinding(mutation.ParentInput, mutation.TargetInputs, inputsByName)
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
		target, err := validateMutationBinding(mutation.TargetIDInput, mutation.TargetInputs, inputsByName)
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
			parent, err := validateMutationBinding(mutation.ParentInput, mutation.TargetInputs, inputsByName)
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

func validateMutationBinding(name string, targetInputs []string, inputs map[string]CommandInput) (CommandInput, error) {
	input, exists := inputs[name]
	if !exists {
		return CommandInput{}, fmt.Errorf("input %q is not a structured input", name)
	}
	if input.Source != InputSourceArgument && input.Source != InputSourceFlag {
		return CommandInput{}, fmt.Errorf("input %q must be a command argument or flag", name)
	}
	if !input.Required {
		return CommandInput{}, fmt.Errorf("input %q must be required", name)
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
	Required      bool
	AllowedValues []string
	TakesValue    bool
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
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if strings.HasPrefix(token.Value, "--") {
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

		if strings.HasPrefix(token.Value, "<") && strings.HasSuffix(token.Value, ">") {
			name := strings.Trim(token.Value, "<>")
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
			inputs[name] = argumentSyntaxInput{Required: !token.Optional, AllowedValues: []string{}, TakesValue: true}
			continue
		}

		if token.Optional && !strings.ContainsAny(token.Value, "|<>=") {
			if err := validateInputName(CommandInput{Name: token.Value, Source: InputSourceArgument}); err != nil {
				return nil, nil, fmt.Errorf("argument syntax: %w", err)
			}
			if _, exists := inputs[token.Value]; exists {
				return nil, nil, fmt.Errorf("argument syntax input %q is declared more than once", token.Value)
			}
			optionalPositionalSeen = true
			positionals = append(positionals, token.Value)
			inputs[token.Value] = argumentSyntaxInput{Required: false, AllowedValues: []string{}, TakesValue: true}
			continue
		}
		return nil, nil, fmt.Errorf("argument syntax token %q is outside the supported grammar", token.Value)
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
func (s CommandSpec) ProducedRefs() []ProducedRef {
	references := make([]ProducedRef, 0, len(s.Agent.Output.Fields)+1)
	for _, field := range s.Agent.Output.Fields {
		if field.ReferenceKind != "" {
			references = append(references, ProducedRef{Kind: field.ReferenceKind, Field: field.Name})
		}
	}
	if pagination := s.Agent.Pagination; pagination != nil && pagination.CursorOutput.ReferenceKind != "" {
		references = append(references, ProducedRef{
			Kind:  pagination.CursorOutput.ReferenceKind,
			Field: pagination.CursorOutput.Name,
		})
	}
	return references
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
	produced := command.ProducedRefs()
	for _, reference := range produced {
		if err := validateReferenceName(reference.Kind); err != nil {
			return fmt.Errorf("produced reference kind: %w", err)
		}
		if err := validateOutputFieldName(reference.Field); err != nil {
			return fmt.Errorf("produced reference field: %w", err)
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
			if len(produced) != 0 || len(consumed) != 0 {
				return fmt.Errorf("fixed-target act commands must not produce or consume references")
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
		if !hasRequiredReference {
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
	commands := make([]CommandSpec, len(c.commands))
	for index, command := range c.commands {
		commands[index] = cloneCommandSpec(command)
	}
	return commands
}

// Lookup finds one exact command path.
func (c Catalog) Lookup(path string) (CommandSpec, bool) {
	for _, command := range c.commands {
		if command.Path == path {
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
	}
	contract.Output.Formats = cloneSlice(contract.Output.Formats)
	contract.Output.Fields = cloneSlice(contract.Output.Fields)
	if contract.Pagination != nil {
		pagination := *contract.Pagination
		contract.Pagination = &pagination
	}
	contract.Prerequisites = cloneSlice(contract.Prerequisites)
	if contract.Authentication != nil {
		authentication := contract.Authentication.Clone()
		contract.Authentication = &authentication
	}
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
	return contract
}

func cloneSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	cloned := make([]T, len(values))
	copy(cloned, values)
	return cloned
}
