package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
)

type helpFormat uint8

const (
	helpFormatText helpFormat = iota
	helpFormatAgent
	agentHelpSchemaVersion = 6
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
	Formats            []string          `json:"formats"`
	DefaultFormat      string            `json:"default_format"`
	JSONSchemaVersion  int               `json:"json_schema_version"`
	Fields             []agentErrorField `json:"fields"`
	ExitCodes          []agentExitCode   `json:"exit_codes"`
	GlobalErrors       []CommandError    `json:"global_errors"`
	CommandErrorsField string            `json:"command_errors_field"`
}

type agentErrorField struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type agentExitCode struct {
	Kind fault.Kind `json:"kind"`
	Code int        `json:"code"`
}

type agentCommand struct {
	Path         string        `json:"path"`
	Summary      string        `json:"summary"`
	Usage        string        `json:"usage"`
	Args         string        `json:"args,omitempty"`
	Effect       string        `json:"effect"`
	Role         string        `json:"role"`
	Contract     AgentContract `json:"contract"`
	ProducesRefs []ProducedRef `json:"produces_refs"`
	ConsumesRefs []ConsumedRef `json:"consumes_refs"`
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
		return c.emitResult(ctx, c.renderRootHelp())
	}
	if exact {
		return c.emitResult(ctx, renderCommandHelp(commands[0]))
	}
	return c.emitResult(ctx, renderNamespaceCommandIndex(selector, commands))
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
	for _, command := range c.commands {
		if strings.HasPrefix(command.Path, selector+" ") {
			commands = append(commands, cloneCommandSpec(command))
		}
	}
	return commands, false
}

func (c *CLI) renderRootHelp() []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, "Agentic CLI Foundry")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Usage:")
	fmt.Fprintf(&output, "  %s [--error-format text|json] <command> [arguments]\n", ProgramName)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Global options:")
	fmt.Fprintln(&output, "  --error-format text|json  Select structured failure presentation (default: text)")
	fmt.Fprintln(&output)
	output.Write(renderRootCommandIndex(c.catalog.Commands()))
	fmt.Fprintln(&output)
	fmt.Fprintf(&output, "Run '%s help <command-or-namespace>' for scoped details.\n", ProgramName)
	fmt.Fprintf(&output, "Run '%s help <command-or-namespace> --format agent' for a scoped machine contract.\n", ProgramName)
	return output.Bytes()
}

func renderRootCommandIndex(commands []CommandSpec) []byte {
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
	fmt.Fprintln(&output, "Commands:")
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
		fmt.Fprintf(&output, "  %-*s  %s\n", width, command.Path, command.Summary)
	}
	for _, namespace := range namespaces {
		fmt.Fprintf(&output, "  %-*s  Namespace with %d commands\n", width, namespace.name, namespace.count)
	}
	return output.Bytes()
}

func renderNamespaceCommandIndex(selector string, commands []CommandSpec) []byte {
	labels := make([]string, 0, len(commands))
	for _, command := range commands {
		labels = append(labels, strings.TrimPrefix(command.Path, selector+" "))
	}
	return renderNamedCommandIndex("Commands in namespace "+selector+":", commands, labels)
}

func renderCommandIndex(title string, commands []CommandSpec) []byte {
	labels := make([]string, 0, len(commands))
	for _, command := range commands {
		labels = append(labels, command.Path)
	}
	return renderNamedCommandIndex(title, commands, labels)
}

func renderNamedCommandIndex(title string, commands []CommandSpec, labels []string) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, title)
	width := 0
	for _, label := range labels {
		if len(label) > width {
			width = len(label)
		}
	}
	for index, command := range commands {
		fmt.Fprintf(&output, "  %-*s  %s\n", width, labels[index], command.Summary)
	}
	return output.Bytes()
}

func renderCommandHelp(command CommandSpec) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, "Usage:")
	fmt.Fprintln(&output, "  "+command.Usage())
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, command.Summary+".")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Capability: "+command.Agent.CapabilityID)
	fmt.Fprintln(&output, "Outcome: "+command.Agent.Outcome)
	fmt.Fprintln(&output, "Effect: "+command.Effect.String())
	fmt.Fprintln(&output, "Role: "+command.Role.String())
	fmt.Fprintln(&output)
	renderHumanInvocationGrammar(&output)
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "Inputs:")
	if len(command.Agent.Inputs) == 0 {
		fmt.Fprintln(&output, "  None")
	}
	for _, input := range command.Agent.Inputs {
		fmt.Fprintf(&output, "  %s\n", input.Name)
		fmt.Fprintf(&output, "    source: %s; required: %t; value: %s; cardinality: %s\n", input.Source, input.Required, input.ValueKind, input.Cardinality)
		fmt.Fprintf(&output, "    %s\n", input.Description)
		if len(input.AllowedValues) != 0 {
			fmt.Fprintf(&output, "    allowed: %s\n", strings.Join(input.AllowedValues, " | "))
		}
		if input.DefaultValue != nil {
			fmt.Fprintf(&output, "    default when omitted: %q\n", *input.DefaultValue)
		}
		if input.Minimum != nil || input.Maximum != nil {
			minimum, maximum := "unbounded", "unbounded"
			if input.Minimum != nil {
				minimum = fmt.Sprintf("%d", *input.Minimum)
			}
			if input.Maximum != nil {
				maximum = fmt.Sprintf("%d", *input.Maximum)
			}
			fmt.Fprintf(&output, "    range: %s..%s\n", minimum, maximum)
		}
		if len(input.Requires) != 0 {
			fmt.Fprintf(&output, "    requires when supplied: %s\n", strings.Join(input.Requires, ", "))
		}
		if len(input.ConflictsWith) != 0 {
			fmt.Fprintf(&output, "    conflicts with: %s\n", strings.Join(input.ConflictsWith, ", "))
		}
		if input.ReferenceKind != "" {
			fmt.Fprintf(&output, "    opaque reference kind: %s\n", input.ReferenceKind)
		}
	}
	if target := command.Agent.FixedTarget; target != nil {
		fmt.Fprintf(&output, "Fixed target: %s %s (%s) - %s\n", target.Kind, target.ID, target.Scope, target.Description)
	}
	for _, reference := range command.ProducedRefs() {
		fmt.Fprintf(&output, "Produces reference: %s in field %s\n", reference.Kind, reference.Field)
	}
	for _, reference := range command.ConsumedRefs() {
		fmt.Fprintf(&output, "Consumes reference: %s from input %s\n", reference.Kind, reference.Argument)
	}
	return output.Bytes()
}

func renderHumanInvocationGrammar(output *bytes.Buffer) {
	grammar := defaultAgentInvocationGrammar()
	fmt.Fprintln(output, "Invocation grammar:")
	fmt.Fprintf(output, "  Value flags: %s\n", strings.Join(grammar.ValueFlagForms, " or "))
	fmt.Fprintf(output, "  Dash-prefixed flag values: %s\n", grammar.DashPrefixedFlagValueForm)
	fmt.Fprintf(output, "  Boolean flags: %s\n", strings.Join(grammar.BooleanFlagForms, ", "))
	fmt.Fprintf(output, "  Dash-prefixed positional values: %s\n", grammar.DashPrefixedPositionalUsage)
}

func defaultAgentInvocationGrammar() agentInvocationGrammar {
	return agentInvocationGrammar{
		ValueFlagForms:              []string{"--flag value", "--flag=value"},
		DashPrefixedFlagValueForm:   "--flag=-value",
		BooleanFlagForms:            []string{"--flag", "--flag=true", "--flag=false"},
		PositionalOnlyMarker:        "--",
		DashPrefixedPositionalUsage: "-- -value",
	}
}

func (c *CLI) renderAgentIndex(commands []CommandSpec) ([]byte, error) {
	document := agentIndexDocument{
		SchemaVersion: agentHelpSchemaVersion,
		View:          "index",
		Program:       ProgramName,
		ScopeRequest: agentScopeRequest{
			InvocationTemplate:           ProgramName + " help <command-or-namespace> --format agent",
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
	workflows := c.catalog.referenceWorkflows()
	scopeKind := "namespace"
	if exact {
		scopeKind = "command"
	}
	document := agentDocument{
		SchemaVersion:     agentHelpSchemaVersion,
		View:              "scope",
		Program:           ProgramName,
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
		document.Commands = append(document.Commands, agentCommand{
			Path:         command.Path,
			Summary:      command.Summary,
			Usage:        command.Usage(),
			Args:         command.Args,
			Effect:       command.Effect.String(),
			Role:         command.Role.String(),
			Contract:     cloneAgentContract(command.Agent),
			ProducesRefs: command.ProducedRefs(),
			ConsumesRefs: command.ConsumedRefs(),
		})
	}
	output, err := json.Marshal(document)
	if err != nil {
		return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "The agent help document could not be encoded.", false, err)
	}
	return append(output, '\n'), nil
}

func defaultAgentErrorContract() agentErrorContract {
	return agentErrorContract{
		Formats:           []string{"text", "json"},
		DefaultFormat:     "text",
		JSONSchemaVersion: 1,
		Fields: []agentErrorField{
			{Name: "kind", Description: "Cross-command recovery class."},
			{Name: "code", Description: "Stable command-specific failure code."},
			{Name: "message", Description: "Safe human explanation that excludes upstream causes."},
			{Name: "retryable", Description: "Whether repeating the same logical command without changing intent is permitted."},
			{Name: "retry_after", Description: "Authoritative rate-window duration when known, otherwise null; timing never grants logical replay permission."},
			{Name: "next_actions", Description: "Structured commands and reasons for recovery."},
		},
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
