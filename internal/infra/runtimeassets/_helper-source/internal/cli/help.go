package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

type helpFormat uint8

const (
	helpFormatText helpFormat = iota
	helpFormatAgent
	agentHelpSchemaVersion = 1
)

type agentIndexDocument struct {
	SchemaVersion int                 `json:"schema_version"`
	View          string              `json:"view"`
	Program       string              `json:"program"`
	ScopeRequest  agentScopeRequest   `json:"scope_request"`
	Commands      []agentIndexCommand `json:"commands"`
}

type agentScopeRequest struct {
	InvocationTemplate           string   `json:"invocation_template"`
	SelectorFields               []string `json:"selector_fields"`
	UnknownOutcomeMaxInvocations int      `json:"unknown_outcome_max_invocations"`
	KnownPathMaxInvocations      int      `json:"known_path_max_invocations"`
}

type agentIndexCommand struct {
	Path         string `json:"path"`
	Namespace    string `json:"namespace"`
	Summary      string `json:"summary"`
	CapabilityID string `json:"capability_id"`
	Outcome      string `json:"outcome"`
	Effect       string `json:"effect"`
	Role         string `json:"role"`
}

type agentDocument struct {
	SchemaVersion     int                    `json:"schema_version"`
	View              string                 `json:"view"`
	Program           string                 `json:"program"`
	Scope             agentScope             `json:"scope"`
	InvocationGrammar agentInvocationGrammar `json:"invocation_grammar"`
	GlobalInputs      []CommandInput         `json:"global_inputs"`
	IOContract        agentIOContract        `json:"io_contract"`
	ErrorContract     agentErrorContract     `json:"error_contract"`
	Commands          []agentCommand         `json:"commands"`
	Workflows         []agentWorkflow        `json:"workflows"`
}

type agentScope struct {
	Selector string `json:"selector"`
	Kind     string `json:"kind"`
}

type agentInvocationGrammar struct {
	ValueFlagForms              []string `json:"value_flag_forms"`
	DashPrefixedFlagValueForm   string   `json:"dash_prefixed_flag_value_form"`
	BooleanFlagForms            []string `json:"boolean_flag_forms"`
	PositionalOnlyMarker        string   `json:"positional_only_marker"`
	DashPrefixedPositionalUsage string   `json:"dash_prefixed_positional_usage"`
	GlobalFlagPosition          string   `json:"global_flag_position"`
}

type agentIOContract struct {
	SuccessStream                      string `json:"success_stream"`
	ErrorStream                        string `json:"error_stream"`
	SuccessStatusRequiresCompleteWrite bool   `json:"success_status_requires_complete_write"`
	PartialOutputIsSuccess             bool   `json:"partial_output_is_success"`
	ExternalTextTrust                  string `json:"external_text_trust"`
	ExternalTextProjection             string `json:"external_text_projection"`
	OpaqueReferencePolicy              string `json:"opaque_reference_policy"`
}

type agentErrorContract struct {
	Formats            []string        `json:"formats"`
	DefaultFormat      string          `json:"default_format"`
	JSONSchemaVersion  int             `json:"json_schema_version"`
	Fields             []OutputField   `json:"fields"`
	ExitCodes          []agentExitCode `json:"exit_codes"`
	GlobalErrors       []CommandError  `json:"global_errors"`
	CommandErrorsField string          `json:"command_errors_field"`
}

type agentExitCode struct {
	Kind fault.Kind `json:"kind"`
	Code int        `json:"code"`
}

type agentCommand struct {
	Path               string                  `json:"path"`
	Summary            string                  `json:"summary"`
	Usage              string                  `json:"usage"`
	Args               string                  `json:"args,omitempty"`
	Effect             string                  `json:"effect"`
	Role               string                  `json:"role"`
	Contract           AgentContract           `json:"contract"`
	MachineInvocations agentMachineInvocations `json:"machine_invocations"`
	ProducesRefs       []ProducedRef           `json:"produces_refs"`
	ConsumesRefs       []ConsumedRef           `json:"consumes_refs"`
}

type agentMachineInvocations struct {
	SuccessJSON string `json:"success_json,omitempty"`
	ErrorJSON   string `json:"error_json"`
}

// agentWorkflow is the complete adjacency for one reference kind. Catalog
// validation makes same-kind endpoints interchangeable, so listing each unique
// endpoint once preserves the full producer-to-consumer edge set without
// serializing its Cartesian product.
type agentWorkflow struct {
	ReferenceKind string                  `json:"reference_kind"`
	Producers     []agentWorkflowProducer `json:"producers"`
	Consumers     []agentWorkflowConsumer `json:"consumers"`
}

type agentWorkflowProducer struct {
	Path  string `json:"path"`
	Usage string `json:"usage"`
	Field string `json:"field"`
}

type agentWorkflowConsumer struct {
	Path  string `json:"path"`
	Usage string `json:"usage"`
	Input string `json:"input"`
}

func runHelp(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	format, err := parseHelpFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error(), "help", "Use a supported format and canonical selector.")
	}
	selector := strings.Join(inputs.Values("command"), " ")

	commands := c.catalog.Commands()
	exact := false
	if selector != "" {
		commands, exact = c.catalog.Select(selector)
		if len(commands) == 0 {
			return c.failUsage(ctx, "invalid_arguments", fmt.Sprintf("Unknown help selector %q.", selector), "help", "Use an exact command path or namespace from the root help.")
		}
	}

	if format == helpFormatAgent {
		var output []byte
		if selector == "" {
			output, err = c.renderAgentIndex(commands)
		} else {
			output, err = c.renderAgentHelp(selector, exact, commands)
		}
		if err != nil {
			return c.fail(ctx, err)
		}
		return c.emitResult(ctx, output)
	}
	if selector == "" {
		return c.emitResult(ctx, c.renderRootHelpWithColor(humanStyleAllowed(ctx, c, c.Out)))
	}
	if exact {
		return c.emitResult(ctx, renderCommandHelpWithColor(commands[0], humanStyleAllowed(ctx, c, c.Out)))
	}
	return c.emitResult(ctx, renderNamespaceCommandIndexWithColor(selector, commands, humanStyleAllowed(ctx, c, c.Out)))
}

func parseHelpFormat(value string) (helpFormat, error) {
	switch value {
	case "text":
		return helpFormatText, nil
	case "agent":
		return helpFormatAgent, nil
	default:
		return helpFormatText, fmt.Errorf("--format must be text or agent")
	}
}

// Select returns an exact command or every command beneath a canonical word
// boundary namespace. Catalog order remains the stable presentation order.
func (c Catalog) Select(selector string) ([]CommandSpec, bool) {
	if err := operation.ValidateCommandPath(selector); err != nil {
		return []CommandSpec{}, false
	}
	if command, found := c.Lookup(selector); found {
		return []CommandSpec{command}, true
	}
	commands := make([]CommandSpec, 0)
	for _, command := range c.Commands() {
		if strings.HasPrefix(command.Path, selector+" ") {
			commands = append(commands, command)
		}
	}
	return commands, false
}

func (c *CLI) renderRootHelp() []byte {
	return c.renderRootHelpWithColor(false)
}

func (c *CLI) renderRootHelpWithColor(color bool) []byte {
	var output bytes.Buffer
	program := c.catalog.programName()
	title := "Tobari"
	if program == ExposureProgramName {
		title = "Tobari · Workspace service exposure"
	} else if program == PermissionProgramName {
		title = "Tobari · Permission wait"
	}
	fmt.Fprintln(&output, applyStyleToken(color, styleAccent, title))
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, applyStyleToken(color, styleAccent, "Usage:"))
	fmt.Fprintf(&output, "  %s\n", applyStyleToken(color, styleText, program+" [--error-format text|json] <command> [arguments]"))
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, applyStyleToken(color, styleAccent, "Global options:"))
	fmt.Fprintf(&output, "  %s  %s\n",
		applyStyleToken(color, styleText, "--error-format text|json"),
		applyStyleToken(color, styleText, "Select structured failure presentation (default: text)"),
	)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, applyStyleToken(color, styleAccent, "Start here:"))
	startHere := []struct{ path, description string }{
		{path: "version", description: "Inspect build channel and runtime API compatibility"},
		{path: ProgramName, description: "Prepare Shared services and enter or reuse the current project's Workspace"},
	}
	if program == ExposureProgramName {
		startHere = []struct{ path, description string }{{path: ExposureProgramName, description: "Request one exact Workspace-loopback HTTP service"}}
	} else if program == PermissionProgramName {
		startHere = []struct{ path, description string }{{path: "wait", description: "Wait for one reviewed attachment-owned permission result"}}
	}
	for _, start := range startHere {
		command, found := c.catalog.Lookup(start.path)
		if !found {
			continue
		}
		fmt.Fprintf(&output, "  %s  %s\n",
			applyStyleToken(color, styleAccent, command.Usage()),
			applyStyleToken(color, styleMuted, start.description),
		)
	}
	fmt.Fprintln(&output)
	output.Write(renderRootCommandIndexWithColor(c.catalog.Commands(), color))
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "%s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("Run '%s help <command-or-namespace>' for scoped details.", program)))
	fmt.Fprintf(&output, "%s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("Run '%s help <command-or-namespace> --format agent' for a scoped machine contract.", program)))
	return output.Bytes()
}

func renderRootCommandIndex(commands []CommandSpec) []byte {
	return renderRootCommandIndexWithColor(commands, false)
}

func renderRootCommandIndexWithColor(commands []CommandSpec, color bool) []byte {
	type namespaceEntry struct {
		name  string
		count int
	}
	direct := make([]CommandSpec, 0)
	namespaces := make([]namespaceEntry, 0)
	namespaceIndex := make(map[string]int)
	for _, command := range commands {
		boundary := strings.IndexByte(command.Path, ' ')
		if boundary < 0 {
			direct = append(direct, command)
			continue
		}
		name := command.Path[:boundary]
		index, exists := namespaceIndex[name]
		if !exists {
			index = len(namespaces)
			namespaceIndex[name] = index
			namespaces = append(namespaces, namespaceEntry{name: name})
		}
		namespaces[index].count++
	}

	var output bytes.Buffer
	fmt.Fprintln(&output, applyStyleToken(color, styleAccent, "Commands:"))
	width := 0
	for _, command := range direct {
		if len(command.Path) > width {
			width = len(command.Path)
		}
	}
	for _, namespace := range namespaces {
		if len(namespace.name) > width {
			width = len(namespace.name)
		}
	}
	for _, command := range direct {
		path := applyStyleToken(color, styleText, fmt.Sprintf("%-*s", width, command.Path))
		summary := applyStyleToken(color, styleText, command.Summary)
		fmt.Fprintf(&output, "  %s  %s\n", path, summary)
	}
	for _, namespace := range namespaces {
		name := applyStyleToken(color, styleText, fmt.Sprintf("%-*s", width, namespace.name))
		description := applyStyleToken(color, styleText, fmt.Sprintf("Namespace with %d commands", namespace.count))
		fmt.Fprintf(&output, "  %s  %s\n", name, description)
	}
	return output.Bytes()
}

func renderNamespaceCommandIndex(selector string, commands []CommandSpec) []byte {
	return renderNamespaceCommandIndexWithColor(selector, commands, false)
}

func renderNamespaceCommandIndexWithColor(selector string, commands []CommandSpec, color bool) []byte {
	labels := make([]string, 0, len(commands))
	for _, command := range commands {
		labels = append(labels, strings.TrimPrefix(command.Path, selector+" "))
	}
	return renderNamedCommandIndexWithColor("Commands in namespace "+selector+":", commands, labels, color)
}

func renderCommandIndex(title string, commands []CommandSpec) []byte {
	labels := make([]string, 0, len(commands))
	for _, command := range commands {
		labels = append(labels, command.Path)
	}
	return renderNamedCommandIndexWithColor(title, commands, labels, false)
}

func renderNamedCommandIndex(title string, commands []CommandSpec, labels []string) []byte {
	return renderNamedCommandIndexWithColor(title, commands, labels, false)
}

func renderNamedCommandIndexWithColor(title string, commands []CommandSpec, labels []string, color bool) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, applyStyleToken(color, styleAccent, title))
	width := 0
	for _, label := range labels {
		if len(label) > width {
			width = len(label)
		}
	}
	for index, command := range commands {
		label := applyStyleToken(color, styleText, fmt.Sprintf("%-*s", width, labels[index]))
		summary := applyStyleToken(color, styleText, command.Summary)
		fmt.Fprintf(&output, "  %s  %s\n", label, summary)
	}
	return output.Bytes()
}

func renderCommandHelp(command CommandSpec) []byte {
	return renderCommandHelpWithColor(command, false)
}

func renderCommandHelpWithColor(command CommandSpec, color bool) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, applyStyleToken(color, styleAccent, "Usage:"))
	fmt.Fprintln(&output, "  "+applyStyleToken(color, styleText, command.Usage()))
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, applyStyleToken(color, styleText, command.Summary+"."))
	fmt.Fprintln(&output)
	writeHelpKeyValue(&output, color, "Capability:", command.Agent.CapabilityID, styleText)
	writeHelpKeyValue(&output, color, "Outcome:", command.Agent.Outcome, styleText)
	writeHelpKeyValue(&output, color, "Effect:", command.Effect.String(), styleText)
	writeHelpKeyValue(&output, color, "Role:", command.Role.String(), styleText)
	fmt.Fprintln(&output)
	renderHumanInvocationGrammarWithColor(&output, color)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, applyStyleToken(color, styleAccent, "Inputs:"))
	if len(command.Agent.Inputs) == 0 {
		fmt.Fprintln(&output, "  "+applyStyleToken(color, styleMuted, "None"))
	}
	for _, input := range command.Agent.Inputs {
		fmt.Fprintf(&output, "  %s\n", applyStyleToken(color, styleText, input.Name))
		fmt.Fprintf(&output, "    %s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("source: %s; required: %t; value: %s; cardinality: %s", input.Source, input.Required, input.ValueKind, input.Cardinality)))
		fmt.Fprintf(&output, "    %s\n", applyStyleToken(color, styleText, input.Description))
		if len(input.AllowedValues) != 0 {
			fmt.Fprintf(&output, "    %s\n", applyStyleToken(color, styleText, "allowed: "+strings.Join(input.AllowedValues, " | ")))
		}
		if input.DefaultValue != nil {
			fmt.Fprintf(&output, "    %s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("default when omitted: %q", *input.DefaultValue)))
		}
		if input.Minimum != nil || input.Maximum != nil {
			minimum, maximum := "unbounded", "unbounded"
			if input.Minimum != nil {
				minimum = fmt.Sprintf("%d", *input.Minimum)
			}
			if input.Maximum != nil {
				maximum = fmt.Sprintf("%d", *input.Maximum)
			}
			fmt.Fprintf(&output, "    %s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("range: %s..%s", minimum, maximum)))
		}
		if input.MinimumLength != nil || input.MaximumLength != nil {
			minimum, maximum := "unbounded", "unbounded"
			if input.MinimumLength != nil {
				minimum = fmt.Sprintf("%d", *input.MinimumLength)
			}
			if input.MaximumLength != nil {
				maximum = fmt.Sprintf("%d", *input.MaximumLength)
			}
			fmt.Fprintf(&output, "    %s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("UTF-8 byte length: %s..%s", minimum, maximum)))
		}
		if len(input.Requires) != 0 {
			fmt.Fprintf(&output, "    %s\n", applyStyleToken(color, styleMuted, "requires when supplied: "+strings.Join(input.Requires, ", ")))
		}
		if len(input.ConflictsWith) != 0 {
			fmt.Fprintf(&output, "    %s\n", applyStyleToken(color, styleMuted, "conflicts with: "+strings.Join(input.ConflictsWith, ", ")))
		}
		if input.ReferenceKind != "" {
			fmt.Fprintf(&output, "    %s\n", applyStyleToken(color, styleText, "opaque reference kind: "+input.ReferenceKind))
		}
		if input.PositionalOnly {
			fmt.Fprintf(&output, "    %s\n", applyStyleToken(color, styleMuted, "positional-only marker required: --"))
		}
	}
	if target := command.Agent.FixedTarget; target != nil {
		fmt.Fprintf(&output, "%s\n", applyStyleToken(color, styleMuted, fmt.Sprintf("Fixed target: %s %s (%s) - %s", target.Kind, target.ID, target.Scope, target.Description)))
	}
	for _, reference := range command.ProducedRefs() {
		fmt.Fprintf(&output, "%s\n", applyStyleToken(color, styleText, fmt.Sprintf("Produces reference: %s in field %s", reference.Kind, reference.Field)))
	}
	for _, reference := range command.ConsumedRefs() {
		fmt.Fprintf(&output, "%s\n", applyStyleToken(color, styleText, fmt.Sprintf("Consumes reference: %s from input %s", reference.Kind, reference.Argument)))
	}
	return output.Bytes()
}

func renderHumanInvocationGrammar(output *bytes.Buffer) {
	renderHumanInvocationGrammarWithColor(output, false)
}

func renderHumanInvocationGrammarWithColor(output *bytes.Buffer, color bool) {
	grammar := defaultAgentInvocationGrammar()
	fmt.Fprintln(output, applyStyleToken(color, styleAccent, "Invocation grammar:"))
	fmt.Fprintf(output, "  %s\n", applyStyleToken(color, styleText, "Value flags: "+strings.Join(grammar.ValueFlagForms, " or ")))
	fmt.Fprintf(output, "  %s\n", applyStyleToken(color, styleText, "Dash-prefixed flag values: "+grammar.DashPrefixedFlagValueForm))
	fmt.Fprintf(output, "  %s\n", applyStyleToken(color, styleText, "Boolean flags: "+strings.Join(grammar.BooleanFlagForms, ", ")))
	fmt.Fprintf(output, "  %s\n", applyStyleToken(color, styleText, "Dash-prefixed positional values: "+grammar.DashPrefixedPositionalUsage))
}

func writeHelpKeyValue(output *bytes.Buffer, color bool, label, value string, token styleToken) {
	fmt.Fprintf(output, "%s %s\n", applyStyleToken(color, styleMuted, label), applyStyleToken(color, token, value))
}

func defaultAgentInvocationGrammar() agentInvocationGrammar {
	return agentInvocationGrammar{
		ValueFlagForms:              []string{"--flag value", "--flag=value"},
		DashPrefixedFlagValueForm:   "--flag=-value",
		BooleanFlagForms:            []string{"--flag", "--flag=true", "--flag=false"},
		PositionalOnlyMarker:        "--",
		DashPrefixedPositionalUsage: "-- -value",
		GlobalFlagPosition:          "before_command",
	}
}

func (c *CLI) renderAgentIndex(commands []CommandSpec) ([]byte, error) {
	program := c.catalog.programName()
	document := agentIndexDocument{
		SchemaVersion: agentHelpSchemaVersion,
		View:          "index",
		Program:       program,
		ScopeRequest: agentScopeRequest{
			InvocationTemplate:           program + " help <command-or-namespace> --format agent",
			SelectorFields:               []string{"commands[].path", "commands[].namespace"},
			UnknownOutcomeMaxInvocations: 2,
			KnownPathMaxInvocations:      1,
		},
		Commands: make([]agentIndexCommand, 0, len(commands)),
	}
	for _, command := range commands {
		document.Commands = append(document.Commands, projectAgentIndexCommand(command))
	}
	output, err := json.Marshal(document)
	if err != nil {
		return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "The agent help index could not be encoded.", false, err)
	}
	return append(output, '\n'), nil
}

func projectAgentIndexCommand(command CommandSpec) agentIndexCommand {
	return agentIndexCommand{
		Path:         command.Path,
		Namespace:    commandNamespace(command.Path),
		Summary:      command.Summary,
		CapabilityID: command.Agent.CapabilityID,
		Outcome:      command.Agent.Outcome,
		Effect:       command.Effect.String(),
		Role:         command.Role.String(),
	}
}

func commandNamespace(path string) string {
	if boundary := strings.IndexByte(path, ' '); boundary >= 0 {
		return path[:boundary]
	}
	return path
}

func (c *CLI) renderAgentHelp(selector string, exact bool, commands []CommandSpec) ([]byte, error) {
	program := c.catalog.programName()
	workflows := c.catalog.referenceWorkflows()
	scopeKind := "namespace"
	if exact {
		scopeKind = "command"
	}
	document := agentDocument{
		SchemaVersion:     agentHelpSchemaVersion,
		View:              "scope",
		Program:           program,
		Scope:             agentScope{Selector: selector, Kind: scopeKind},
		InvocationGrammar: defaultAgentInvocationGrammar(),
		GlobalInputs: []CommandInput{{
			Name: "--error-format", Source: InputSourceFlag, Required: false,
			ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
			Description:   "Select text or stable JSON stderr; place this global option before the command.",
			AllowedValues: []string{"text", "json"}, DefaultValue: stringPointer("text"),
		}},
		ErrorContract: defaultAgentErrorContract(),
		IOContract: agentIOContract{
			SuccessStream: "stdout", ErrorStream: "stderr",
			SuccessStatusRequiresCompleteWrite: true,
			PartialOutputIsSuccess:             false,
			ExternalTextTrust:                  "untrusted_data",
			ExternalTextProjection:             "visible_escape",
			OpaqueReferencePolicy:              "validated_exact_bytes",
		},
		Commands:  make([]agentCommand, 0, len(commands)),
		Workflows: workflowsForCommands(workflows, commands),
	}
	for _, command := range commands {
		contract := cloneAgentContract(command.Agent)
		if contract.Interactive != nil {
			for _, action := range contract.Interactive.actionCommands() {
				registered, found := c.catalog.lookupRegisteredForProgram(command.programName(), action)
				if found && registered.Visibility == CommandVisibilityInternal {
					contract.Interactive.ActionCommand = ""
					contract.Interactive.ActionCommands = []string{}
				}
			}
		}
		document.Commands = append(document.Commands, agentCommand{
			Path:               command.Path,
			Summary:            command.Summary,
			Usage:              command.Usage(),
			Args:               command.Args,
			Effect:             command.Effect.String(),
			Role:               command.Role.String(),
			Contract:           contract,
			MachineInvocations: machineInvocationsForCommand(command),
			ProducesRefs:       command.ProducedRefs(),
			ConsumesRefs:       command.ConsumedRefs(),
		})
	}
	output, err := json.Marshal(document)
	if err != nil {
		return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "The agent help document could not be encoded.", false, err)
	}
	return append(output, '\n'), nil
}

func machineInvocationsForCommand(command CommandSpec) agentMachineInvocations {
	program := command.programName()
	commandInvocation := program
	if command.Path != program {
		commandInvocation += " " + command.Path
	}
	for _, input := range command.Agent.Inputs {
		if !input.Required || (input.Source != InputSourceArgument && input.Source != InputSourceFlag) {
			continue
		}
		placeholder := "<" + strings.TrimLeft(input.Name, "-") + ">"
		if input.Source == InputSourceArgument {
			commandInvocation += " " + placeholder
		} else if input.ValueKind == InputValueBoolean {
			commandInvocation += " " + input.Name
		} else {
			commandInvocation += " " + input.Name + "=" + placeholder
		}
	}
	success := ""
	if supportsOutputFormat(command.Agent.Output.Formats, OutputFormatJSON) {
		formatValue := "json"
		if command.Path == "help" {
			formatValue = "agent"
		}
		success = commandInvocation + " --format=" + formatValue
		commandInvocation += " --format=" + formatValue
	}
	errorInvocation := strings.Replace(commandInvocation, program, program+" --error-format=json", 1)
	return agentMachineInvocations{SuccessJSON: success, ErrorJSON: errorInvocation}
}

func supportsOutputFormat(formats []OutputFormat, wanted OutputFormat) bool {
	for _, format := range formats {
		if format == wanted {
			return true
		}
	}
	return false
}

func defaultAgentErrorContract() agentErrorContract {
	contract := agentErrorContract{
		Formats:           []string{"text", "json"},
		DefaultFormat:     "text",
		JSONSchemaVersion: 2,
		Fields:            defaultAgentErrorFields(),
		ExitCodes: []agentExitCode{
			{Kind: fault.KindInvalidInput, Code: ExitUsage},
			{Kind: fault.KindAuthentication, Code: ExitAuthentication},
			{Kind: fault.KindPermission, Code: ExitPermission},
			{Kind: fault.KindNotFound, Code: ExitNotFound},
			{Kind: fault.KindAmbiguous, Code: ExitAmbiguous},
			{Kind: fault.KindRateLimited, Code: ExitRateLimited},
			{Kind: fault.KindUnavailable, Code: ExitUnavailable},
			{Kind: fault.KindRejected, Code: ExitRejected},
			{Kind: fault.KindCanceled, Code: ExitCanceled},
			{Kind: fault.KindUnsupported, Code: ExitUnsupported},
			{Kind: fault.KindContract, Code: ExitContract},
			{Kind: fault.KindInternal, Code: ExitInternal},
		},
		GlobalErrors: []CommandError{
			declaredCommandError(fault.KindInvalidInput, "invalid_root_options", false, "help", "Correct the global options."),
			declaredCommandError(fault.KindInvalidInput, "missing_command", false, "help", "Discover available command outcomes."),
			declaredCommandError(fault.KindInvalidInput, "unknown_command", false, "help", "Discover an exact command path or namespace."),
			declaredCommandError(fault.KindContract, "missing_context", false, "help", "Retry through a context-aware CLI entry point."),
			declaredCommandError(fault.KindContract, "invalid_catalog", false, "help", "Repair the catalog before dispatch."),
			declaredCommandError(fault.KindCanceled, "operation_canceled", true, "help", "Retry when the caller is ready."),
		},
		CommandErrorsField: "commands[].contract.errors",
	}
	for index := range contract.GlobalErrors {
		contract.GlobalErrors[index].Phase = fault.PhasePrecondition
		contract.GlobalErrors[index].ChangeState = fault.ChangeNone
	}
	return contract
}

func defaultAgentErrorFields() []OutputField {
	return []OutputField{
		{Name: "kind", Type: OutputFieldTypeString, Description: "Cross-command recovery class.", Enum: []string{"invalid_input", "authentication", "permission", "not_found", "ambiguous", "rate_limited", "unavailable", "rejected", "canceled", "unsupported", "contract", "internal"}},
		{Name: "code", Type: OutputFieldTypeString, Description: "Stable command-specific failure code."},
		{Name: "message", Type: OutputFieldTypeString, Description: "Safe human explanation that excludes upstream causes."},
		{Name: "phase", Type: OutputFieldTypeString, Description: "Closed command stage that established the failure.", Enum: []string{"precondition", "observation", "mutation", "verification", "attachment", "presentation"}},
		{Name: "change_state", Type: OutputFieldTypeString, Description: "Strongest proved state of the requested change.", Enum: []string{"not_applicable", "none", "partial", "confirmed", "unknown"}},
		{Name: "retryable", Type: OutputFieldTypeBoolean, Description: "Whether repeating the same logical command without changing intent is permitted."},
		{Name: "retry_after", Type: OutputFieldTypeString, Description: "Authoritative rate-window duration when known, otherwise null; timing never grants logical replay permission.", Nullable: true},
		{Name: "next_actions", Type: OutputFieldTypeArray, Description: "Structured commands and reasons for recovery.", Items: &OutputField{
			Type: OutputFieldTypeObject, Description: "One executable recovery action.", Fields: []OutputField{
				{Name: "command", Type: OutputFieldTypeString, Description: "Exact catalog path or help selector command."},
				{Name: "reason", Type: OutputFieldTypeString, Description: "Safe reason to choose this recovery."},
			},
		}},
	}
}

func (c Catalog) referenceWorkflows() []agentWorkflow {
	commands := c.Commands()
	workflows := make([]agentWorkflow, 0)
	workflowIndex := make(map[string]int)
	producerSeen := make(map[string]map[agentWorkflowProducer]struct{})
	for _, producer := range commands {
		for _, produced := range producer.ProducedRefs() {
			index, exists := workflowIndex[produced.Kind]
			if !exists {
				index = len(workflows)
				workflowIndex[produced.Kind] = index
				workflows = append(workflows, agentWorkflow{
					ReferenceKind: produced.Kind,
					Producers:     make([]agentWorkflowProducer, 0),
					Consumers:     make([]agentWorkflowConsumer, 0),
				})
				producerSeen[produced.Kind] = make(map[agentWorkflowProducer]struct{})
			}
			projected := agentWorkflowProducer{Path: producer.Path, Usage: producer.Usage(), Field: produced.Field}
			if _, duplicate := producerSeen[produced.Kind][projected]; duplicate {
				continue
			}
			producerSeen[produced.Kind][projected] = struct{}{}
			workflows[index].Producers = append(workflows[index].Producers, projected)
		}
	}

	consumerSeen := make(map[string]map[agentWorkflowConsumer]struct{}, len(workflows))
	for _, consumer := range commands {
		for _, consumed := range consumer.ConsumedRefs() {
			index, exists := workflowIndex[consumed.Kind]
			if !exists {
				continue
			}
			seen := consumerSeen[consumed.Kind]
			if seen == nil {
				seen = make(map[agentWorkflowConsumer]struct{})
				consumerSeen[consumed.Kind] = seen
			}
			projected := agentWorkflowConsumer{Path: consumer.Path, Usage: consumer.Usage(), Input: consumed.Argument}
			if _, duplicate := seen[projected]; duplicate {
				continue
			}
			seen[projected] = struct{}{}
			workflows[index].Consumers = append(workflows[index].Consumers, projected)
		}
	}

	complete := workflows[:0]
	for _, workflow := range workflows {
		if len(workflow.Producers) == 0 || len(workflow.Consumers) == 0 {
			continue
		}
		complete = append(complete, workflow)
	}
	return complete
}

func workflowsForCommands(workflows []agentWorkflow, commands []CommandSpec) []agentWorkflow {
	selected := make(map[string]struct{}, len(commands))
	for _, command := range commands {
		selected[command.Path] = struct{}{}
	}
	filtered := make([]agentWorkflow, 0)
	for _, workflow := range workflows {
		// Keep the complete kind adjacency when any endpoint is selected. Pruning
		// the other side would hide valid ways to enter or leave the scoped task.
		matches := false
		for _, producer := range workflow.Producers {
			if _, exists := selected[producer.Path]; exists {
				matches = true
				break
			}
		}
		if !matches {
			for _, consumer := range workflow.Consumers {
				if _, exists := selected[consumer.Path]; exists {
					matches = true
					break
				}
			}
		}
		if matches {
			filtered = append(filtered, workflow)
		}
	}
	return filtered
}
