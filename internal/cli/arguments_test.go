package cli

import (
	"reflect"
	"strings"
	"testing"
)

func TestCatalogOwnedParserPreservesTypesCardinalityAndPresence(t *testing.T) {
	minimum := int64(1)
	maximum := int64(100)
	spec := utilitySpec("messages inspect")
	spec.Args = "[--verbose] [--limit <count>] [--sender <sender>] [--context <lines>] [--brief]"
	spec.Agent.Inputs = []CommandInput{
		{
			Name: "--verbose", Source: InputSourceFlag, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle,
			Description: "Enable verbose output.", AllowedValues: []string{}, DefaultValue: stringPointer("false"),
		},
		{
			Name: "--limit", Source: InputSourceFlag, ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle,
			Description: "Select a bounded result count.", AllowedValues: []string{}, DefaultValue: stringPointer("10"), Minimum: &minimum, Maximum: &maximum,
		},
		{
			Name: "--sender", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable,
			Description: "Select one or more senders.", AllowedValues: []string{},
		},
		{
			Name: "--context", Source: InputSourceFlag, ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle,
			Description: "Select context lines around sender matches.", AllowedValues: []string{}, Minimum: &minimum, Maximum: &maximum,
			Requires: []string{"--sender"}, ConflictsWith: []string{"--brief"},
		},
		{
			Name: "--brief", Source: InputSourceFlag, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle,
			Description: "Suppress expanded context.", AllowedValues: []string{},
		},
	}
	if err := validateAgentContract(spec); err != nil {
		t.Fatalf("valid typed input contract: %v", err)
	}

	parsed, err := parseCommandInputs(spec, []string{"--sender", "one", "--limit=3", "--sender", "two", "--context", "1", "--verbose=false"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed.Values("--sender"), []string{"one", "two"}) || !parsed.Provided("--sender") {
		t.Fatalf("senders = %v, provided=%t", parsed.Values("--sender"), parsed.Provided("--sender"))
	}
	if limit, ok := parsed.Integer("--limit"); !ok || limit != 3 || parsed.Defaulted("--limit") {
		t.Fatalf("limit = %d, ok=%t, defaulted=%t", limit, ok, parsed.Defaulted("--limit"))
	}
	if verbose, ok := parsed.Boolean("--verbose"); !ok || verbose || !parsed.Provided("--verbose") {
		t.Fatalf("verbose = %t, ok=%t, provided=%t", verbose, ok, parsed.Provided("--verbose"))
	}

	defaults, err := parseCommandInputs(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if limit, ok := defaults.Integer("--limit"); !ok || limit != 10 || !defaults.Defaulted("--limit") || defaults.Provided("--limit") {
		t.Fatalf("default limit = %d, ok=%t, defaulted=%t, provided=%t", limit, ok, defaults.Defaulted("--limit"), defaults.Provided("--limit"))
	}
	if verbose, ok := defaults.Boolean("--verbose"); !ok || verbose || !defaults.Defaulted("--verbose") {
		t.Fatalf("default verbose = %t, ok=%t, defaulted=%t", verbose, ok, defaults.Defaulted("--verbose"))
	}
}

func TestCatalogOwnedParserRejectsInvalidInvocationBeforeHandler(t *testing.T) {
	minimum := int64(1)
	maximum := int64(5)
	spec := utilitySpec("items choose")
	spec.Args = "[--limit <count>] [--item <id>] [--context <lines>] [--brief]"
	spec.Agent.Inputs = []CommandInput{
		{Name: "--limit", Source: InputSourceFlag, ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle, Description: "Bound the result count.", AllowedValues: []string{}, Minimum: &minimum, Maximum: &maximum},
		{Name: "--item", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalityRepeatable, Description: "Select item IDs.", AllowedValues: []string{}},
		{Name: "--context", Source: InputSourceFlag, ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle, Description: "Expand context.", AllowedValues: []string{}, Minimum: &minimum, Maximum: &maximum, Requires: []string{"--item"}, ConflictsWith: []string{"--brief"}},
		{Name: "--brief", Source: InputSourceFlag, ValueKind: InputValueBoolean, Cardinality: InputCardinalitySingle, Description: "Use brief output.", AllowedValues: []string{}},
	}
	if err := validateAgentContract(spec); err != nil {
		t.Fatal(err)
	}

	for name, invocation := range map[string][]string{
		"range":      {"--limit", "6"},
		"type":       {"--limit", "many"},
		"dependency": {"--context", "1"},
		"conflict":   {"--item", "one", "--context", "1", "--brief"},
		"duplicate":  {"--limit", "1", "--limit", "2"},
		"unknown":    {"--unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCommandInputs(spec, invocation); err == nil {
				t.Fatalf("parseCommandInputs(%v) succeeded", invocation)
			}
		})
	}
}

func TestCatalogOwnedParserDistinguishesAbsentDefaultAndExplicitEmpty(t *testing.T) {
	spec := utilitySpec("labels inspect")
	spec.Args = "[--mode normal|raw] [--label <text>]"
	spec.Agent.Inputs = []CommandInput{
		{Name: "--mode", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Select the mode.", AllowedValues: []string{"normal", "raw"}, DefaultValue: stringPointer("normal")},
		{Name: "--label", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Select an optional label.", AllowedValues: []string{}},
	}
	if err := validateAgentContract(spec); err != nil {
		t.Fatal(err)
	}
	absent, err := parseCommandInputs(spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := parseCommandInputs(spec, []string{"--label="})
	if err != nil {
		t.Fatal(err)
	}
	if absent.Provided("--label") || absent.Defaulted("--label") || len(absent.Values("--label")) != 0 {
		t.Fatalf("absent label = %+v", absent)
	}
	if !explicit.Provided("--label") || explicit.Defaulted("--label") || explicit.One("--label") != "" || len(explicit.Values("--label")) != 1 {
		t.Fatalf("explicit empty label = %+v", explicit)
	}
	if !absent.Defaulted("--mode") || absent.Provided("--mode") || absent.One("--mode") != "normal" {
		t.Fatalf("default mode = %+v", absent)
	}
}

func TestCatalogOwnedParserLeavesNonCommandLineSourcesForTheirResolvers(t *testing.T) {
	spec := utilitySpec("configuration inspect")
	spec.Agent.Inputs = []CommandInput{
		{
			Name: "CLI_ACCOUNT", Source: InputSourceEnvironment, Required: true,
			ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
			Description: "Account selected by the environment resolver.", AllowedValues: []string{},
		},
		{
			Name: "display.mode", Source: InputSourceConfiguration,
			ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
			Description: "Mode selected by the configuration resolver.", AllowedValues: []string{"compact", "full"},
			DefaultValue: stringPointer("compact"),
		},
	}
	if err := validateAgentContract(spec); err != nil {
		t.Fatal(err)
	}
	parsed, err := parseCommandInputs(spec, nil)
	if err != nil {
		t.Fatalf("argv parser tried to resolve a non-argv source: %v", err)
	}
	if parsed.Provided("CLI_ACCOUNT") || parsed.Defaulted("display.mode") || len(parsed.Values("display.mode")) != 0 {
		t.Fatalf("non-argv values leaked into argv result: %+v", parsed)
	}
}

func TestCatalogOwnedParserPreservesDashPrefixedOpaqueValues(t *testing.T) {
	spec := utilitySpec("references inspect")
	spec.Args = "--id <id> <target>"
	spec.Agent.Inputs = []CommandInput{
		{
			Name: "--id", Source: InputSourceFlag, Required: true,
			ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
			Description: "Opaque provider reference.", AllowedValues: []string{}, ReferenceKind: "provider-item",
		},
		{
			Name: "target", Source: InputSourceArgument, Required: true,
			ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
			Description: "Opaque positional reference.", AllowedValues: []string{}, ReferenceKind: "provider-item",
		},
	}
	if err := validateAgentContract(spec); err != nil {
		t.Fatal(err)
	}
	parsed, err := parseCommandInputs(spec, []string{"--id=--provider-flag-value", "--", "-provider-positional"})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.One("--id") != "--provider-flag-value" || parsed.One("target") != "-provider-positional" {
		t.Fatalf("opaque values changed: flag=%q positional=%q", parsed.One("--id"), parsed.One("target"))
	}
	for _, args := range [][]string{
		{"--id", "--provider-flag-value", "--", "target"},
		{"--id", "--", "target"},
	} {
		if _, err := parseCommandInputs(spec, args); err == nil || !strings.Contains(err.Error(), "use --id=<value>") {
			t.Fatalf("ambiguous dash-prefixed flag value %v error = %v", args, err)
		}
	}
}

func TestCatalogRejectsAmbiguousPositionalOrdering(t *testing.T) {
	reversed := utilitySpec("items compare")
	reversed.Args = "<first> <second>"
	reversed.Agent.Inputs = []CommandInput{
		{Name: "second", Source: InputSourceArgument, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Second item.", AllowedValues: []string{}},
		{Name: "first", Source: InputSourceArgument, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "First item.", AllowedValues: []string{}},
	}
	if err := validateAgentContract(reversed); err == nil || !strings.Contains(err.Error(), "positional input order") {
		t.Fatalf("reversed positional order error = %v", err)
	}

	optionalBeforeRequired := utilitySpec("items compare")
	optionalBeforeRequired.Args = "[first] <second>"
	optionalBeforeRequired.Agent.Inputs = []CommandInput{
		{Name: "first", Source: InputSourceArgument, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Optional first item.", AllowedValues: []string{}},
		{Name: "second", Source: InputSourceArgument, Required: true, ValueKind: InputValueText, Cardinality: InputCardinalitySingle, Description: "Required second item.", AllowedValues: []string{}},
	}
	if err := validateAgentContract(optionalBeforeRequired); err == nil || !strings.Contains(err.Error(), "required positional") {
		t.Fatalf("optional-before-required error = %v", err)
	}
}

func TestCatalogInputMetadataFailsClosed(t *testing.T) {
	base := utilitySpec("items inspect")
	base.Args = "[--mode fast|safe]"
	base.Agent.Inputs = []CommandInput{{
		Name: "--mode", Source: InputSourceFlag, ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Select an inspection mode.", AllowedValues: []string{"fast", "safe"}, DefaultValue: stringPointer("fast"),
	}}
	if err := validateAgentContract(base); err != nil {
		t.Fatal(err)
	}

	tests := map[string]func(*CommandSpec){
		"missing value kind":  func(spec *CommandSpec) { spec.Agent.Inputs[0].ValueKind = InputValueUnknown },
		"missing cardinality": func(spec *CommandSpec) { spec.Agent.Inputs[0].Cardinality = InputCardinalityUnknown },
		"invalid default":     func(spec *CommandSpec) { spec.Agent.Inputs[0].DefaultValue = stringPointer("other") },
		"unknown dependency":  func(spec *CommandSpec) { spec.Agent.Inputs[0].Requires = []string{"--missing"} },
		"self conflict":       func(spec *CommandSpec) { spec.Agent.Inputs[0].ConflictsWith = []string{"--mode"} },
		"unsafe name":         func(spec *CommandSpec) { spec.Agent.Inputs[0].Name = "--mo\u200bde" },
		"translated enum":     func(spec *CommandSpec) { spec.Agent.Inputs[0].AllowedValues[0] = "高速" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			spec := cloneCommandSpec(base)
			mutate(&spec)
			if err := validateAgentContract(spec); err == nil || strings.TrimSpace(err.Error()) == "" {
				t.Fatalf("invalid input metadata passed: %v", err)
			}
		})
	}
}

func TestCatalogRejectsTypedEnumAndNonCLIIdentifierDrift(t *testing.T) {
	minimum := int64(1)
	maximum := int64(5)
	integer := utilitySpec("items count")
	integer.Args = "[--limit 1|many]"
	integer.Agent.Inputs = []CommandInput{{
		Name: "--limit", Source: InputSourceFlag, ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle,
		Description: "Select a bounded count.", AllowedValues: []string{"1", "many"}, Minimum: &minimum, Maximum: &maximum,
	}}
	if err := validateAgentContract(integer); err == nil || !strings.Contains(err.Error(), "invalid allowed value") {
		t.Fatalf("non-integer enum error = %v", err)
	}
	integer.Args = "[--limit 1|9]"
	integer.Agent.Inputs[0].AllowedValues = []string{"1", "9"}
	if err := validateAgentContract(integer); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("out-of-range enum error = %v", err)
	}

	for name, input := range map[string]CommandInput{
		"environment": {
			Name: "アカウント", Source: InputSourceEnvironment, ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
			Description: "Environment account.", AllowedValues: []string{},
		},
		"configuration": {
			Name: "表示.mode", Source: InputSourceConfiguration, ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
			Description: "Configuration mode.", AllowedValues: []string{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			spec := utilitySpec("inputs inspect")
			spec.Agent.Inputs = []CommandInput{input}
			if err := validateAgentContract(spec); err == nil {
				t.Fatal("translated machine input name passed validation")
			}
		})
	}
}
