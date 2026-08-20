package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/tasuku43/tobari/internal/cli"
)

const productContractPath = "docs/01_product_contract.md"

type publicCommandTableRow struct {
	Invocation string
	Line       int
}

type documentedCommandInput struct {
	Required       bool
	TakesValue     bool
	AllowedValues  []string
	Repeatable     bool
	PositionalOnly bool
}

type documentedCommandGrammar struct {
	Flags       map[string]documentedCommandInput
	Positionals []documentedCommandInput
}

type documentedArgumentToken struct {
	Value         string
	Optional      bool
	OptionalGroup int
}

func validatePublicCommandTable(root string, catalog cli.Catalog) ([]issue, error) {
	encoded, err := readRegularManifest(root, productContractPath)
	if err != nil {
		return nil, err
	}
	return validatePublicCommandTableDocument(productContractPath, string(encoded), catalog), nil
}

func validatePublicCommandTableDocument(path, document string, catalog cli.Catalog) []issue {
	rows, issues := extractPublicCommandTableRows(path, document)
	if rows == nil {
		return issues
	}

	commands := catalog.Commands()
	commandsByPath := make(map[string]cli.CommandSpec, len(commands))
	seen := make(map[string]int, len(rows))
	for _, command := range commands {
		commandsByPath[command.Path] = command
	}

	for _, row := range rows {
		command, args, found := matchPublicCommandInvocation(row.Invocation, commands)
		location := fmt.Sprintf("%s:%d", path, row.Line)
		if !found {
			issues = append(issues, issue{
				Path:    location,
				Message: fmt.Sprintf("public command table invocation %q is not a public Catalog command", documentedCommandPath(row.Invocation)),
			})
			continue
		}
		if firstLine, duplicate := seen[command.Path]; duplicate {
			issues = append(issues, issue{
				Path:    location,
				Message: fmt.Sprintf("public command table declares Catalog command %q more than once; first declared at line %d", command.Path, firstLine),
			})
			continue
		}
		seen[command.Path] = row.Line

		grammar, grammarIssues := parseDocumentedCommandGrammar(args, command)
		for _, message := range grammarIssues {
			issues = append(issues, issue{Path: location, Message: fmt.Sprintf("command %q invocation: %s", command.Path, message)})
		}
		for _, message := range compareDocumentedCommandGrammar(command, grammar) {
			issues = append(issues, issue{Path: location, Message: message})
		}
	}

	for commandPath := range commandsByPath {
		if _, exists := seen[commandPath]; !exists {
			issues = append(issues, issue{
				Path:    path,
				Message: fmt.Sprintf("public command table is missing Catalog command %q", commandPath),
			})
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Path != issues[j].Path {
			return issues[i].Path < issues[j].Path
		}
		return issues[i].Message < issues[j].Message
	})
	return issues
}

func extractPublicCommandTableRows(path, document string) ([]publicCommandTableRow, []issue) {
	lines := strings.Split(document, "\n")
	headerLines := make([]int, 0, 1)
	for index, line := range lines {
		cells := plainMarkdownTableCells(line)
		if len(cells) == 4 && strings.EqualFold(cells[0], "Command") && strings.EqualFold(cells[1], "Role") &&
			strings.EqualFold(cells[2], "Effect") && strings.EqualFold(cells[3], "Outcome") {
			headerLines = append(headerLines, index)
		}
	}
	if len(headerLines) != 1 {
		return nil, []issue{{
			Path:    path,
			Message: fmt.Sprintf("expected exactly one public command table with Command, Role, Effect, and Outcome columns; found %d", len(headerLines)),
		}}
	}

	header := headerLines[0]
	if header+1 >= len(lines) || !isMarkdownTableDelimiter(lines[header+1], 4) {
		return nil, []issue{{Path: fmt.Sprintf("%s:%d", path, header+2), Message: "public command table header is not followed by a four-column delimiter"}}
	}

	rows := make([]publicCommandTableRow, 0)
	var issues []issue
	for index := header + 2; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "" || !strings.HasPrefix(line, "|") {
			break
		}
		invocation, ok := firstMarkdownCodeCell(line)
		if !ok {
			issues = append(issues, issue{
				Path:    fmt.Sprintf("%s:%d", path, index+1),
				Message: "public command table row must place one code-formatted invocation in the Command column",
			})
			continue
		}
		rows = append(rows, publicCommandTableRow{Invocation: invocation, Line: index + 1})
	}
	return rows, issues
}

func plainMarkdownTableCells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil
	}
	line = strings.TrimPrefix(strings.TrimSuffix(line, "|"), "|")
	parts := strings.Split(line, "|")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

func isMarkdownTableDelimiter(line string, columns int) bool {
	cells := plainMarkdownTableCells(line)
	if len(cells) != columns {
		return false
	}
	for _, cell := range cells {
		cell = strings.Trim(cell, ":")
		if len(cell) < 3 || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}

func firstMarkdownCodeCell(line string) (string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") {
		return "", false
	}
	cell := strings.TrimSpace(strings.TrimPrefix(line, "|"))
	if !strings.HasPrefix(cell, "`") {
		return "", false
	}
	closing := strings.Index(cell[1:], "`")
	if closing < 0 {
		return "", false
	}
	closing++
	invocation := cell[1:closing]
	remainder := strings.TrimSpace(cell[closing+1:])
	if invocation == "" || !strings.HasPrefix(remainder, "|") {
		return "", false
	}
	return strings.ReplaceAll(invocation, `\|`, "|"), true
}

func matchPublicCommandInvocation(invocation string, commands []cli.CommandSpec) (cli.CommandSpec, string, bool) {
	fields := strings.Fields(invocation)
	matchedWords := 0
	var matched cli.CommandSpec
	for _, command := range commands {
		words := strings.Fields(command.Path)
		if len(words) <= matchedWords || len(words) > len(fields) {
			continue
		}
		matches := true
		for index := range words {
			if fields[index] != words[index] {
				matches = false
				break
			}
		}
		if matches {
			matched = command
			matchedWords = len(words)
		}
	}
	if matchedWords == 0 {
		return cli.CommandSpec{}, "", false
	}
	return matched, strings.Join(fields[matchedWords:], " "), true
}

func documentedCommandPath(invocation string) string {
	fields := strings.Fields(invocation)
	path := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.HasPrefix(field, "[") || strings.HasPrefix(field, "--") || strings.HasPrefix(field, "<") || isUpperPlaceholder(field) {
			break
		}
		path = append(path, strings.Trim(field, "[]"))
	}
	if len(path) == 0 {
		return invocation
	}
	return strings.Join(path, " ")
}

func isUpperPlaceholder(value string) bool {
	value = strings.TrimSuffix(strings.Trim(value, "[]<>"), "...")
	hasLetter := false
	for _, r := range value {
		if unicode.IsLetter(r) {
			hasLetter = true
			if !unicode.IsUpper(r) {
				return false
			}
		}
	}
	return hasLetter
}

func parseDocumentedCommandGrammar(args string, command cli.CommandSpec) (documentedCommandGrammar, []string) {
	grammar := documentedCommandGrammar{Flags: make(map[string]documentedCommandInput), Positionals: []documentedCommandInput{}}
	tokens, err := tokenizeDocumentedArguments(args)
	if err != nil {
		return grammar, []string{err.Error()}
	}

	expectedFlags := make(map[string]cli.CommandInput)
	expectedPositionals := make([]cli.CommandInput, 0)
	for _, input := range command.Agent.Inputs {
		switch input.Source {
		case cli.InputSourceFlag:
			expectedFlags[input.Name] = input
		case cli.InputSourceArgument:
			expectedPositionals = append(expectedPositionals, input)
		}
	}

	var issues []string
	positionalOnly := false
	for index := 0; index < len(tokens); index++ {
		token := tokens[index]
		if token.Value == "--" {
			if positionalOnly {
				issues = append(issues, "positional-only marker is declared more than once")
			}
			positionalOnly = true
			continue
		}
		if !positionalOnly && strings.HasPrefix(token.Value, "--") {
			parts := strings.SplitN(token.Value, "=", 2)
			name := parts[0]
			expected, known := expectedFlags[name]
			entry := documentedCommandInput{Required: !token.Optional}
			value := ""
			if len(parts) == 2 {
				entry.TakesValue = true
				value = parts[1]
				if value == "" {
					issues = append(issues, fmt.Sprintf("flag %q has an empty value grammar", name))
				}
			} else if known && expected.ValueKind != cli.InputValueBoolean {
				if index+1 < len(tokens) && sameDocumentedArgumentGroup(token, tokens[index+1]) && !strings.HasPrefix(tokens[index+1].Value, "--") {
					index++
					entry.TakesValue = true
					value = tokens[index].Value
				}
			} else if !known && index+1 < len(tokens) && sameDocumentedArgumentGroup(token, tokens[index+1]) &&
				!strings.HasPrefix(tokens[index+1].Value, "--") {
				index++
				entry.TakesValue = true
				value = tokens[index].Value
			}
			entry.AllowedValues, err = documentedAllowedValues(value, known && len(expected.AllowedValues) != 0)
			if err != nil {
				issues = append(issues, fmt.Sprintf("flag %q: %v", name, err))
			}
			if _, duplicate := grammar.Flags[name]; duplicate {
				issues = append(issues, fmt.Sprintf("flag %q is declared more than once", name))
				continue
			}
			grammar.Flags[name] = entry
			continue
		}

		repeatable := strings.HasSuffix(token.Value, "...")
		positionalValue := strings.TrimSuffix(token.Value, "...")
		expectsEnumeration := len(grammar.Positionals) < len(expectedPositionals) && len(expectedPositionals[len(grammar.Positionals)].AllowedValues) != 0
		allowedValues, err := documentedAllowedValues(positionalValue, expectsEnumeration)
		if err != nil {
			issues = append(issues, fmt.Sprintf("positional argument %d: %v", len(grammar.Positionals)+1, err))
		}
		grammar.Positionals = append(grammar.Positionals, documentedCommandInput{
			Required:       !token.Optional,
			TakesValue:     true,
			AllowedValues:  allowedValues,
			Repeatable:     repeatable,
			PositionalOnly: positionalOnly,
		})
	}
	return grammar, issues
}

func tokenizeDocumentedArguments(args string) ([]documentedArgumentToken, error) {
	rawTokens := strings.Fields(args)
	tokens := make([]documentedArgumentToken, 0, len(rawTokens))
	inOptional := false
	optionalGroup := 0
	for _, raw := range rawTokens {
		opens := strings.HasPrefix(raw, "[")
		closes := strings.HasSuffix(raw, "]")
		if opens {
			if inOptional {
				return nil, fmt.Errorf("contains nested optional groups")
			}
			inOptional = true
			optionalGroup++
			raw = strings.TrimPrefix(raw, "[")
		}
		if closes {
			if !inOptional {
				return nil, fmt.Errorf("contains an unmatched closing bracket")
			}
			raw = strings.TrimSuffix(raw, "]")
		}
		if raw == "" || strings.ContainsAny(raw, "[]") {
			return nil, fmt.Errorf("contains an empty or malformed token")
		}
		token := documentedArgumentToken{Value: raw, Optional: inOptional}
		if inOptional {
			token.OptionalGroup = optionalGroup
		}
		tokens = append(tokens, token)
		if closes {
			inOptional = false
		}
	}
	if inOptional {
		return nil, fmt.Errorf("contains an unclosed optional group")
	}
	return tokens, nil
}

func sameDocumentedArgumentGroup(left, right documentedArgumentToken) bool {
	if left.Optional != right.Optional {
		return false
	}
	return !left.Optional || left.OptionalGroup == right.OptionalGroup
}

func documentedAllowedValues(value string, expectsEnumeration bool) ([]string, error) {
	if value == "" {
		return []string{}, nil
	}
	if strings.Contains(value, "|") {
		values := strings.Split(value, "|")
		seen := make(map[string]struct{}, len(values))
		for _, candidate := range values {
			if candidate == "" {
				return nil, fmt.Errorf("contains an empty allowed value")
			}
			if _, duplicate := seen[candidate]; duplicate {
				return nil, fmt.Errorf("declares allowed value %q more than once", candidate)
			}
			seen[candidate] = struct{}{}
		}
		sort.Strings(values)
		return values, nil
	}
	if !expectsEnumeration || isPlaceholder(value) {
		return []string{}, nil
	}
	return []string{value}, nil
}

func isPlaceholder(value string) bool {
	return (strings.HasPrefix(value, "<") && strings.HasSuffix(value, ">")) || isUpperPlaceholder(value)
}

func compareDocumentedCommandGrammar(command cli.CommandSpec, actual documentedCommandGrammar) []string {
	expectedFlags := make(map[string]cli.CommandInput)
	expectedPositionals := make([]cli.CommandInput, 0)
	for _, input := range command.Agent.Inputs {
		switch input.Source {
		case cli.InputSourceFlag:
			expectedFlags[input.Name] = input
		case cli.InputSourceArgument:
			expectedPositionals = append(expectedPositionals, input)
		}
	}

	var issues []string
	flagNames := make([]string, 0, len(expectedFlags))
	for name := range expectedFlags {
		flagNames = append(flagNames, name)
	}
	sort.Strings(flagNames)
	for _, name := range flagNames {
		expected := expectedFlags[name]
		got, exists := actual.Flags[name]
		if !exists {
			issues = append(issues, fmt.Sprintf("command %q invocation is missing Catalog flag %q", command.Path, name))
			continue
		}
		if got.Required != expected.Required {
			issues = append(issues, fmt.Sprintf("command %q flag %q required=%t does not match Catalog required=%t", command.Path, name, got.Required, expected.Required))
		}
		expectedTakesValue := expected.ValueKind != cli.InputValueBoolean
		if got.TakesValue != expectedTakesValue {
			issues = append(issues, fmt.Sprintf("command %q flag %q takes_value=%t does not match Catalog takes_value=%t", command.Path, name, got.TakesValue, expectedTakesValue))
			continue
		}
		if !equalStringSets(got.AllowedValues, expected.AllowedValues) {
			issues = append(issues, fmt.Sprintf("command %q flag %q allowed values %v do not match Catalog %v", command.Path, name, got.AllowedValues, expected.AllowedValues))
		}
	}

	actualFlagNames := make([]string, 0, len(actual.Flags))
	for name := range actual.Flags {
		actualFlagNames = append(actualFlagNames, name)
	}
	sort.Strings(actualFlagNames)
	for _, name := range actualFlagNames {
		if _, exists := expectedFlags[name]; !exists {
			issues = append(issues, fmt.Sprintf("command %q invocation has non-Catalog flag %q", command.Path, name))
		}
	}

	commonPositionals := len(expectedPositionals)
	if len(actual.Positionals) < commonPositionals {
		commonPositionals = len(actual.Positionals)
	}
	for index := 0; index < commonPositionals; index++ {
		expected := expectedPositionals[index]
		got := actual.Positionals[index]
		if got.Required != expected.Required {
			issues = append(issues, fmt.Sprintf("command %q positional argument %d (%q) required=%t does not match Catalog required=%t", command.Path, index+1, expected.Name, got.Required, expected.Required))
		}
		if !equalStringSets(got.AllowedValues, expected.AllowedValues) {
			issues = append(issues, fmt.Sprintf("command %q positional argument %d (%q) allowed values %v do not match Catalog %v", command.Path, index+1, expected.Name, got.AllowedValues, expected.AllowedValues))
		}
		if got.Repeatable != (expected.Cardinality == cli.InputCardinalityRepeatable) {
			issues = append(issues, fmt.Sprintf("command %q positional argument %d (%q) repeatable=%t does not match Catalog repeatable=%t", command.Path, index+1, expected.Name, got.Repeatable, expected.Cardinality == cli.InputCardinalityRepeatable))
		}
		if got.PositionalOnly != expected.PositionalOnly {
			issues = append(issues, fmt.Sprintf("command %q positional argument %d (%q) positional_only=%t does not match Catalog positional_only=%t", command.Path, index+1, expected.Name, got.PositionalOnly, expected.PositionalOnly))
		}
	}
	for index := commonPositionals; index < len(expectedPositionals); index++ {
		issues = append(issues, fmt.Sprintf("command %q invocation is missing Catalog positional argument %d (%q)", command.Path, index+1, expectedPositionals[index].Name))
	}
	for index := commonPositionals; index < len(actual.Positionals); index++ {
		issues = append(issues, fmt.Sprintf("command %q invocation has non-Catalog positional argument %d", command.Path, index+1))
	}
	return issues
}

func equalStringSets(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]string{}, left...)
	rightCopy := append([]string{}, right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	for index := range leftCopy {
		if leftCopy[index] != rightCopy[index] {
			return false
		}
	}
	return true
}
