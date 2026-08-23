package main

import (
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/cli"
)

func TestPublicCommandTableAcceptsSemanticPlaceholderAndEnumEquivalence(t *testing.T) {
	catalog := cli.NewCatalog(
		testCommandSpec("help", []cli.CommandInput{
			{Name: "command", Source: cli.InputSourceArgument, Required: false, ValueKind: cli.InputValueText, Cardinality: cli.InputCardinalityRepeatable, AllowedValues: []string{}},
			{Name: "--format", Source: cli.InputSourceFlag, Required: false, ValueKind: cli.InputValueText, Cardinality: cli.InputCardinalitySingle, AllowedValues: []string{"text", "agent"}},
		}),
		testCommandSpec("items create", []cli.CommandInput{
			{Name: "--name", Source: cli.InputSourceFlag, Required: true, ValueKind: cli.InputValueText, Cardinality: cli.InputCardinalitySingle, AllowedValues: []string{}},
			{Name: "--mode", Source: cli.InputSourceFlag, Required: false, ValueKind: cli.InputValueText, Cardinality: cli.InputCardinalitySingle, AllowedValues: []string{"fast", "safe"}},
			{Name: "--force", Source: cli.InputSourceFlag, Required: false, ValueKind: cli.InputValueBoolean, Cardinality: cli.InputCardinalitySingle, AllowedValues: []string{}},
		}),
		cli.CommandSpec{Path: "items apply", Visibility: cli.CommandVisibilityInternal},
	)
	document := `
| Command | Role | Effect | Outcome |
|---|---|---|---|
| ` + "`help [SELECTOR...] [--format agent\\|text]`" + ` | utility | read | Help |
| ` + "`items create --name <item-name> [--mode safe|fast] [--force]`" + ` | act | create | Create |
`
	issues := validatePublicCommandTableDocument(productContractPath, document, catalog)
	if len(issues) != 0 {
		t.Fatalf("public command table issues = %+v", issues)
	}
}

func TestPublicCommandTableAcceptsAndEnforcesPositionalOnlyRepeatableArgv(t *testing.T) {
	catalog := cli.NewCatalog(testCommandSpec("tobari", []cli.CommandInput{
		{Name: "--manifest", Source: cli.InputSourceFlag, Required: false, ValueKind: cli.InputValueText, Cardinality: cli.InputCardinalitySingle, AllowedValues: []string{}},
		{Name: "command", Source: cli.InputSourceArgument, Required: false, ValueKind: cli.InputValueText, Cardinality: cli.InputCardinalityRepeatable, AllowedValues: []string{}, PositionalOnly: true},
	}))
	valid := `
| Command | Role | Effect | Outcome |
|---|---|---|---|
| ` + "`tobari [--manifest <name>] [-- <command>...]`" + ` | act | create | Enter |
`
	if issues := validatePublicCommandTableDocument(productContractPath, valid, catalog); len(issues) != 0 {
		t.Fatalf("positional-only command issues = %+v", issues)
	}

	missingMarker := strings.Replace(valid, "[-- <command>...]", "[<command>...]", 1)
	assertIssuesContain(t, validatePublicCommandTableDocument(productContractPath, missingMarker, catalog), "positional_only=false")
	missingRepeatability := strings.Replace(valid, "<command>...", "<command>", 1)
	assertIssuesContain(t, validatePublicCommandTableDocument(productContractPath, missingRepeatability, catalog), "repeatable=false")
}

func TestPublicCommandTableRejectsVisibilityCoverageAndGrammarDrift(t *testing.T) {
	catalog := cli.NewCatalog(
		testCommandSpec("help", []cli.CommandInput{
			{Name: "command", Source: cli.InputSourceArgument, Required: false, ValueKind: cli.InputValueText, Cardinality: cli.InputCardinalityRepeatable, AllowedValues: []string{}},
			{Name: "--format", Source: cli.InputSourceFlag, Required: false, ValueKind: cli.InputValueText, Cardinality: cli.InputCardinalitySingle, AllowedValues: []string{"text", "agent"}},
		}),
		testCommandSpec("items create", []cli.CommandInput{
			{Name: "--name", Source: cli.InputSourceFlag, Required: true, ValueKind: cli.InputValueText, Cardinality: cli.InputCardinalitySingle, AllowedValues: []string{}},
			{Name: "--mode", Source: cli.InputSourceFlag, Required: false, ValueKind: cli.InputValueText, Cardinality: cli.InputCardinalitySingle, AllowedValues: []string{"fast", "safe"}},
		}),
		testCommandSpec("items show", []cli.CommandInput{}),
		cli.CommandSpec{Path: "items apply", Visibility: cli.CommandVisibilityInternal},
	)
	document := `
| Command | Role | Effect | Outcome |
|---|---|---|---|
| ` + "`help [selector] [--format text]`" + ` | utility | read | Help |
| ` + "`items create [--name NAME] [--unknown VALUE] [EXTRA]`" + ` | act | create | Create |
| ` + "`items apply`" + ` | act | write | Apply |
`
	issues := validatePublicCommandTableDocument(productContractPath, document, catalog)
	assertIssuesContain(t, issues,
		"is not a public Catalog command",
		"is missing Catalog command \"items show\"",
		"flag \"--format\" allowed values",
		"flag \"--name\" required=false",
		"is missing Catalog flag \"--mode\"",
		"has non-Catalog flag \"--unknown\"",
		"has non-Catalog positional argument 1",
	)
}

func TestPublicCommandTableRequiresOneStructuredTable(t *testing.T) {
	catalog := cli.NewCatalog(testCommandSpec("help", []cli.CommandInput{}))
	issues := validatePublicCommandTableDocument(productContractPath, "The public commands are described in prose.\n", catalog)
	assertIssuesContain(t, issues, "expected exactly one public command table")
}

func testCommandSpec(path string, inputs []cli.CommandInput) cli.CommandSpec {
	return cli.CommandSpec{Path: path, Agent: cli.AgentContract{Inputs: inputs}}
}
