package cli

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsedInputs preserves whether a value was supplied explicitly or obtained
// from the catalog default. Callers must not reconstruct that distinction from
// a zero value.
type ParsedInputs struct {
	values   map[string][]string
	provided map[string]bool
	defaults map[string]bool
}

// Values returns a detached copy of every effective value for name.
func (p ParsedInputs) Values(name string) []string {
	return cloneSlice(p.values[name])
}

// One returns the sole effective value for a single-cardinality input. It
// returns an empty string when the input is absent; Provided and Defaulted keep
// an explicitly empty value distinguishable from absence.
func (p ParsedInputs) One(name string) string {
	values := p.values[name]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Provided reports whether argv explicitly supplied the input.
func (p ParsedInputs) Provided(name string) bool {
	return p.provided[name]
}

// Defaulted reports whether the effective value came from DefaultValue.
func (p ParsedInputs) Defaulted(name string) bool {
	return p.defaults[name]
}

// Integer returns the validated base-10 integer value when present.
func (p ParsedInputs) Integer(name string) (int64, bool) {
	value, exists := firstValue(p.values[name])
	if !exists {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil
}

// Boolean returns the validated boolean value when present.
func (p ParsedInputs) Boolean(name string) (bool, bool) {
	value, exists := firstValue(p.values[name])
	if !exists {
		return false, false
	}
	return value == "true", value == "true" || value == "false"
}

func firstValue(values []string) (string, bool) {
	if len(values) == 0 {
		return "", false
	}
	return values[0], true
}

// parseCommandInputs is the one argv parser invoked by dispatch before any
// command handler. Catalog validation has already proved the declarations
// internally coherent; this boundary enforces them against one invocation.
func parseCommandInputs(command CommandSpec, args []string) (ParsedInputs, error) {
	parsed := ParsedInputs{
		values:   make(map[string][]string),
		provided: make(map[string]bool),
		defaults: make(map[string]bool),
	}
	flags := make(map[string]CommandInput)
	positionals := make([]CommandInput, 0)
	for _, input := range command.Agent.Inputs {
		switch input.Source {
		case InputSourceFlag:
			flags[input.Name] = input
		case InputSourceArgument:
			positionals = append(positionals, input)
		}
	}

	positionalIndex := 0
	positionalOnly := false
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !positionalOnly && argument == "--" {
			positionalOnly = true
			continue
		}
		if !positionalOnly && strings.HasPrefix(argument, "--") {
			name, inlineValue, hasInlineValue := strings.Cut(argument, "=")
			input, exists := flags[name]
			if !exists {
				return ParsedInputs{}, fmt.Errorf("unknown flag %q", name)
			}

			value := inlineValue
			if input.ValueKind == InputValueBoolean {
				if !hasInlineValue {
					value = "true"
				}
			} else if !hasInlineValue {
				if index+1 >= len(args) {
					return ParsedInputs{}, fmt.Errorf("%s requires a value", name)
				}
				if strings.HasPrefix(args[index+1], "-") {
					return ParsedInputs{}, fmt.Errorf("%s requires a value; use %s=<value> for a dash-prefixed value", name, name)
				}
				index++
				value = args[index]
			}
			if err := addParsedInput(&parsed, input, value); err != nil {
				return ParsedInputs{}, err
			}
			continue
		}

		if positionalIndex >= len(positionals) {
			if strings.HasPrefix(argument, "-") {
				return ParsedInputs{}, fmt.Errorf("unknown flag %q", argument)
			}
			return ParsedInputs{}, fmt.Errorf("unexpected argument %q", argument)
		}
		input := positionals[positionalIndex]
		if !positionalOnly && strings.HasPrefix(argument, "-") && input.ValueKind != InputValueInteger {
			return ParsedInputs{}, fmt.Errorf("unknown flag %q", argument)
		}
		if err := addParsedInput(&parsed, input, argument); err != nil {
			return ParsedInputs{}, err
		}
		if input.Cardinality == InputCardinalitySingle {
			positionalIndex++
		}
	}

	for _, input := range command.Agent.Inputs {
		if !isCommandLineInput(input) {
			continue
		}
		if input.Required && !parsed.provided[input.Name] {
			return ParsedInputs{}, fmt.Errorf("%s is required", input.Name)
		}
		if !parsed.provided[input.Name] && input.DefaultValue != nil {
			parsed.values[input.Name] = []string{*input.DefaultValue}
			parsed.defaults[input.Name] = true
		}
	}
	for _, input := range command.Agent.Inputs {
		if !isCommandLineInput(input) || !parsed.provided[input.Name] {
			continue
		}
		for _, required := range input.Requires {
			if !parsed.provided[required] {
				return ParsedInputs{}, fmt.Errorf("%s requires %s", input.Name, required)
			}
		}
		for _, conflict := range input.ConflictsWith {
			if parsed.provided[conflict] {
				return ParsedInputs{}, fmt.Errorf("%s conflicts with %s", input.Name, conflict)
			}
		}
	}
	return parsed, nil
}

func addParsedInput(parsed *ParsedInputs, input CommandInput, value string) error {
	if parsed.provided[input.Name] && input.Cardinality != InputCardinalityRepeatable {
		return fmt.Errorf("%s may be specified only once", input.Name)
	}
	if err := validateInputValue(input, value); err != nil {
		return fmt.Errorf("%s %w", input.Name, err)
	}
	parsed.values[input.Name] = append(parsed.values[input.Name], value)
	parsed.provided[input.Name] = true
	return nil
}
