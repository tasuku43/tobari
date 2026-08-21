package cli

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/tasuku43/tobari/internal/app/completioncmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

const (
	completionMaxWords = 128
	completionMaxBytes = 16 * 1024
)

type completionRecord struct {
	kind  string
	value string
}

func completionCommandSpecs() []CommandSpec {
	minimumCurrent := int64(1)
	maximumCurrent := int64(completionMaxWords)
	minimumWord := int64(0)
	return []CommandSpec{
		{
			Path: "completion zsh", Summary: "Generate the zsh adapter for catalog-backed interactive completion",
			Args: "", Effect: operation.EffectRead, Role: RoleUtility,
			Agent: AgentContract{
				CapabilityID: "cli.completion",
				Outcome:      "Generate a static zsh adapter that asks the current Tobari executable for typed completion candidates on each Tab",
				Inputs:       []CommandInput{},
				Output: CommandOutput{
					Formats: []OutputFormat{OutputFormatText}, DefaultFormat: OutputFormatText, TextPresentation: TextPresentationSemanticTokens,
					Fields:   []OutputField{{Name: "script", Type: OutputFieldTypeString, Description: "Complete zsh adapter source without an embedded command registry."}},
					Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
				},
				Prerequisites: []string{"zsh completion initialization is enabled before sourcing the generated adapter."},
				Errors:        readCommandErrors("completion zsh", true),
			},
			handler: runCompletionZsh,
		},
		{
			Path: "completion candidates", Summary: "Return typed candidates for one bounded shell completion request",
			Args: "--current <index> <word>...", Effect: operation.EffectRead, Role: RoleUtility,
			Agent: AgentContract{
				CapabilityID: "cli.completion",
				Outcome:      "Return bounded catalog-derived or validated local-state candidates for the current command word without mutation, Docker, or network access",
				Inputs: []CommandInput{
					{Name: "--current", Source: InputSourceFlag, Required: true, ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle, Description: "One-based index of the word currently being completed.", AllowedValues: []string{}, Minimum: &minimumCurrent, Maximum: &maximumCurrent},
					{Name: "word", Source: InputSourceArgument, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable, Description: "Exact shell words, including the program word and current partial word.", AllowedValues: []string{}, MinimumLength: &minimumWord},
				},
				Output: CommandOutput{
					Formats: []OutputFormat{OutputFormatTSV}, DefaultFormat: OutputFormatTSV,
					Fields: []OutputField{
						{Name: "record_type", Type: OutputFieldTypeString, Description: "Candidate record or shell-owned completion directive.", Enum: []string{"candidate", "directive"}},
						{Name: "value", Type: OutputFieldTypeString, Description: "Exact safe candidate, or the directories directive."},
					},
					Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageExhaustive,
				},
				Prerequisites: []string{"Dynamic candidates require readable owner-local Context and Runtime manifests; no Docker daemon or network is used."},
				Errors: readCommandErrors("completion candidates", true,
					declaredCommandError(fault.KindInternal, "completion_context_read_failed", false, "context list", "Inspect the local Context catalog."),
					declaredCommandError(fault.KindInternal, "completion_runtime_read_failed", false, "runtime list", "Inspect the local Runtime catalog."),
					declaredCommandError(fault.KindContract, "invalid_completion_candidates", false, "doctor", "Repair the local candidate source contract."),
					declaredCommandError(fault.KindInternal, "missing_runtime", false, "doctor", "Configure the Tobari runtime."),
				),
			},
			handler: runCompletionCandidates,
		},
	}
}

func runCompletionZsh(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
	return c.emitResult(ctx, []byte(zshCompletionAdapter))
}

const zshCompletionAdapter = `#compdef tobari

_tobari() {
  local -a response candidates
  local line record value

  response=("${(@f)$(command tobari completion candidates --current "$CURRENT" -- "${words[@]}" 2>/dev/null)}") || return 0
  for line in "${response[@]}"; do
    record="${line%%$'\t'*}"
    value="${line#*$'\t'}"
    case "$record" in
      candidate) candidates+=("$value") ;;
      directive)
        [[ "$value" == directories ]] && { _directories; return 0; }
        ;;
    esac
  done
  (( ${#candidates} )) && compadd -Q -- "${candidates[@]}"
}

compdef _tobari tobari
`

func runCompletionCandidates(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	current, ok := inputs.Integer("--current")
	if !ok {
		return c.failUsage(ctx, "invalid_arguments", "--current must be a valid word index; usage: "+command.Usage(), "help completion candidates", "Supply the current one-based shell word index.")
	}
	requestWords := inputs.Values("word")
	records, err := c.planCompletion(ctx, int(current), requestWords)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderCompletionRecords(records)
	if err != nil {
		return c.fail(ctx, fault.Wrap(fault.KindContract, "invalid_completion_candidates", "Completion candidates are invalid.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Repair the local candidate source contract."}))
	}
	return c.emitResult(ctx, output)
}

func (c *CLI) planCompletion(ctx context.Context, current int, words []string) ([]completionRecord, error) {
	if current < 1 || current > len(words) || len(words) > completionMaxWords {
		return nil, invalidCompletionRequest("current word index is outside the bounded word collection")
	}
	total := 0
	for _, word := range words {
		if !utf8.ValidString(word) {
			return nil, invalidCompletionRequest("word collection is not valid UTF-8")
		}
		total += len(word)
	}
	if total > completionMaxBytes {
		return nil, invalidCompletionRequest("word collection exceeds the bounded request size")
	}

	before := append([]string{}, words[1:current-1]...)
	partial := words[current-1]
	commandWords, activeRoot, valid := parseCompletionRoot(before)
	if !valid {
		return []completionRecord{}, nil
	}
	if activeRoot != nil {
		return c.recordsForInput(ctx, *activeRoot, partial, "")
	}
	if strings.HasPrefix(partial, "--context=") && len(commandWords) == 0 {
		return c.recordsForInput(ctx, rootContextCompletionInput(), strings.TrimPrefix(partial, "--context="), "--context=")
	}
	if strings.HasPrefix(partial, "--error-format=") && len(commandWords) == 0 {
		return c.recordsForInput(ctx, rootErrorFormatCompletionInput(), strings.TrimPrefix(partial, "--error-format="), "--error-format=")
	}
	if len(commandWords) == 0 && strings.HasPrefix(partial, "--") {
		return candidateRecords(filterPrefix([]string{"--context", "--error-format"}, partial)), nil
	}

	selected, consumed := completionCommand(c.catalog, commandWords)
	if selected == nil {
		return candidateRecords(commandWordCandidates(c.catalog, commandWords, partial)), nil
	}
	return c.completeCommandInputs(ctx, *selected, commandWords[consumed:], partial)
}

func parseCompletionRoot(words []string) ([]string, *CommandInput, bool) {
	for index := 0; index < len(words); {
		word := words[index]
		var input CommandInput
		switch word {
		case "--context":
			input = rootContextCompletionInput()
		case "--error-format":
			input = rootErrorFormatCompletionInput()
		default:
			if strings.HasPrefix(word, "--context=") || strings.HasPrefix(word, "--error-format=") {
				index++
				continue
			}
			return append([]string{}, words[index:]...), nil, true
		}
		if index+1 >= len(words) {
			return nil, &input, true
		}
		index += 2
	}
	return []string{}, nil, true
}

func rootContextCompletionInput() CommandInput {
	return CommandInput{Name: "--context", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, AllowedValues: []string{}, Completion: InputCompletionContextName}
}

func rootErrorFormatCompletionInput() CommandInput {
	return CommandInput{Name: "--error-format", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, AllowedValues: []string{"text", "json"}}
}

func completionCommand(catalog Catalog, words []string) (*CommandSpec, int) {
	var selected *CommandSpec
	consumed := 0
	for _, command := range catalog.Commands() {
		pathWords := strings.Fields(command.Path)
		if len(pathWords) > len(words) || len(pathWords) <= consumed {
			continue
		}
		match := true
		for index := range pathWords {
			if pathWords[index] != words[index] {
				match = false
				break
			}
		}
		if match {
			copy := command
			selected = &copy
			consumed = len(pathWords)
		}
	}
	return selected, consumed
}

func commandWordCandidates(catalog Catalog, prefix []string, partial string) []string {
	values := make([]string, 0)
	seen := make(map[string]struct{})
	for _, command := range catalog.Commands() {
		path := strings.Fields(command.Path)
		if len(path) <= len(prefix) {
			continue
		}
		matches := true
		for index := range prefix {
			if path[index] != prefix[index] {
				matches = false
				break
			}
		}
		value := path[len(prefix)]
		if !matches || !strings.HasPrefix(value, partial) {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func (c *CLI) completeCommandInputs(ctx context.Context, command CommandSpec, before []string, partial string) ([]completionRecord, error) {
	flags := make(map[string]CommandInput)
	positionals := make([]CommandInput, 0)
	for _, input := range command.Agent.Inputs {
		if input.Source == InputSourceFlag {
			flags[input.Name] = input
		} else if input.Source == InputSourceArgument {
			positionals = append(positionals, input)
		}
	}
	used := make(map[string]bool)
	positionValues := make([]string, 0)
	for index := 0; index < len(before); index++ {
		word := before[index]
		if word == "--" {
			positionValues = append(positionValues, before[index+1:]...)
			break
		}
		name, _, inline := strings.Cut(word, "=")
		if input, exists := flags[name]; exists {
			used[name] = true
			if input.ValueKind != InputValueBoolean && !inline {
				if index+1 >= len(before) {
					return c.recordsForInput(ctx, input, partial, "")
				}
				index++
			}
			continue
		}
		positionValues = append(positionValues, word)
	}

	if strings.HasPrefix(partial, "--") {
		name, valuePrefix, inline := strings.Cut(partial, "=")
		if input, exists := flags[name]; inline && exists {
			if input.ValueKind == InputValueBoolean {
				return candidateRecords(withPrefix(filterPrefix([]string{"true", "false"}, valuePrefix), name+"=")), nil
			}
			return c.recordsForInput(ctx, input, valuePrefix, name+"=")
		}
		values := make([]string, 0, len(flags))
		for _, input := range command.Agent.Inputs {
			if input.Source != InputSourceFlag || (used[input.Name] && input.Cardinality == InputCardinalitySingle) || completionInputConflicts(input, used) {
				continue
			}
			if strings.HasPrefix(input.Name, partial) {
				values = append(values, input.Name)
			}
		}
		return candidateRecords(values), nil
	}

	if positional, exists := activePositional(positionals, len(positionValues)); exists {
		if positional.Completion == InputCompletionCommand {
			return candidateRecords(commandWordCandidates(c.catalog, positionValues, partial)), nil
		}
		return c.recordsForInput(ctx, positional, partial, "")
	}
	if partial == "" {
		values := make([]string, 0, len(flags))
		for _, input := range command.Agent.Inputs {
			if input.Source == InputSourceFlag && (!used[input.Name] || input.Cardinality == InputCardinalityRepeatable) && !completionInputConflicts(input, used) {
				values = append(values, input.Name)
			}
		}
		return candidateRecords(values), nil
	}
	return []completionRecord{}, nil
}

func completionInputConflicts(input CommandInput, used map[string]bool) bool {
	for _, conflict := range input.ConflictsWith {
		if used[conflict] {
			return true
		}
	}
	return false
}

func activePositional(inputs []CommandInput, count int) (CommandInput, bool) {
	for _, input := range inputs {
		if input.Cardinality == InputCardinalityRepeatable || count == 0 {
			return input, true
		}
		count--
	}
	return CommandInput{}, false
}

func (c *CLI) recordsForInput(ctx context.Context, input CommandInput, partial, outputPrefix string) ([]completionRecord, error) {
	if len(input.AllowedValues) > 0 {
		return candidateRecords(withPrefix(filterPrefix(input.AllowedValues, partial), outputPrefix)), nil
	}
	if input.ValueKind == InputValueBoolean {
		return candidateRecords(withPrefix(filterPrefix([]string{"true", "false"}, partial), outputPrefix)), nil
	}
	if input.Completion == InputCompletionDirectory {
		return []completionRecord{{kind: "directive", value: "directories"}}, nil
	}
	if input.Completion == InputCompletionCommand {
		return candidateRecords(withPrefix(commandWordCandidates(c.catalog, nil, partial), outputPrefix)), nil
	}
	kind, dynamic := completionCandidateKind(input.Completion)
	if !dynamic {
		return []completionRecord{}, nil
	}
	if c.completion == nil {
		return nil, fault.New(fault.KindInternal, "missing_runtime", "Completion candidate discovery is not configured", false,
			fault.NextAction{Command: "doctor", Reason: "Configure the Tobari runtime."})
	}
	values, err := c.completion.Candidates(ctx, kind)
	if err != nil {
		return nil, err
	}
	return candidateRecords(withPrefix(filterPrefix(values, partial), outputPrefix)), nil
}

func completionCandidateKind(source InputCompletion) (completioncmd.CandidateKind, bool) {
	switch source {
	case InputCompletionContextName:
		return completioncmd.CandidateContextName, true
	case InputCompletionRuntimeName:
		return completioncmd.CandidateRuntimeName, true
	case InputCompletionManagedRuntimeName:
		return completioncmd.CandidateManagedRuntimeName, true
	case InputCompletionReadyRuntimeReference:
		return completioncmd.CandidateReadyRuntimeReference, true
	default:
		return "", false
	}
}

func candidateRecords(values []string) []completionRecord {
	records := make([]completionRecord, len(values))
	for index, value := range values {
		records[index] = completionRecord{kind: "candidate", value: value}
	}
	return records
}

func filterPrefix(values []string, prefix string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			result = append(result, value)
		}
	}
	return result
}

func withPrefix(values []string, prefix string) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = prefix + value
	}
	return result
}

func renderCompletionRecords(records []completionRecord) ([]byte, error) {
	if len(records) > 8192 {
		return nil, fmt.Errorf("completion record count exceeds 8192")
	}
	seen := make(map[string]struct{}, len(records))
	var output bytes.Buffer
	for _, record := range records {
		if record.kind != "candidate" && record.kind != "directive" {
			return nil, fmt.Errorf("completion record kind %q is invalid", record.kind)
		}
		if err := validateCompletionRecordValue(record.value); err != nil {
			return nil, err
		}
		key := record.kind + "\x00" + record.value
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		fmt.Fprintf(&output, "%s\t%s\n", record.kind, record.value)
		if output.Len() > 256*1024 {
			return nil, fmt.Errorf("completion output exceeds 262144 bytes")
		}
	}
	return output.Bytes(), nil
}

func validateCompletionRecordValue(value string) error {
	if value == "" || !utf8.ValidString(value) {
		return fmt.Errorf("completion value is empty or invalid UTF-8")
	}
	for _, r := range value {
		if r == '\t' || r == '\n' || r == '\r' || r == 0 || r == '\u2028' || r == '\u2029' {
			return fmt.Errorf("completion value contains unsafe structure")
		}
	}
	return nil
}

func invalidCompletionRequest(reason string) error {
	return fault.New(fault.KindInvalidInput, "invalid_arguments", "Completion request is invalid: "+reason+".", false,
		fault.NextAction{Command: "help completion candidates", Reason: "Supply one bounded shell completion request."})
}
