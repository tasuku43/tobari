package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type causalRecoveryEdge struct {
	from string
	to   string
	code string
}

// validateCausalRecoveryGraph closes the gap between an individually valid
// next command and an executable repository-wide recovery chain. Retryable
// caller/output failures are deliberate replay edges; non-retryable faults
// must hand off to a different, directly invocable classifier or terminal
// diagnostic.
func validateCausalRecoveryGraph(c Catalog) error {
	edges := make([]causalRecoveryEdge, 0)
	violations := make([]error, 0)
	for _, command := range c.commands {
		for _, declared := range command.Agent.Errors {
			for _, action := range declared.NextActions {
				target, err := c.resolveRecoveryCommandForProgram(command.programName(), action.Command)
				if err != nil {
					violations = append(violations, fmt.Errorf("catalog command %q error %q recovery graph: %w", command.Path, declared.Code, err))
					continue
				}
				if err := validateExecutableRecoveryTarget(command, declared, action.Command, target); err != nil {
					violations = append(violations, err)
					continue
				}
				if declared.Retryable || declared.Code == "invalid_arguments" || declared.Code == "diagnostic_failed" || isTerminalRecoveryInvocation(action.Command) || isInteractiveRecoveryTransition(command, action.Command) {
					continue
				}
				edges = append(edges, causalRecoveryEdge{
					from: commandCatalogKey(command), to: commandCatalogKey(target), code: declared.Code,
				})
			}
		}
	}
	if err := errors.Join(violations...); err != nil {
		return err
	}
	if _, found := c.ForProgram(ProgramName).Lookup("status"); found {
		if err := validateStatusHomeRecoveryBindings(c); err != nil {
			return err
		}
	}
	return validateAcyclicCausalRecoveries(edges)
}

func validateStatusHomeRecoveryBindings(c Catalog) error {
	for _, path := range tobari.StatusHomeRecoveryPaths() {
		target, err := c.resolveRecoveryCommandForProgram(ProgramName, path)
		if err != nil {
			return fmt.Errorf("status schema 3 recovery path %q: %w", path, err)
		}
		for _, input := range target.Agent.Inputs {
			if input.Required {
				return fmt.Errorf("status schema 3 recovery path %q has unchecked required input %q", path, input.Name)
			}
		}
	}
	for _, guidance := range tobari.StatusHomeRecoveryGuidance() {
		if _, err := c.resolveRecoveryCommandForProgram(ProgramName, guidance); err == nil {
			return fmt.Errorf("status schema 3 guidance %q resolves as a Catalog command", guidance)
		}
	}
	return nil
}

func validateExecutableRecoveryTarget(source CommandSpec, declared CommandError, invocation string, target CommandSpec) error {
	if !declared.Retryable && declared.Code != "invalid_arguments" && declared.Code != "diagnostic_failed" && invocation == source.Path && !isInteractiveRecoveryTransition(source, invocation) {
		return fmt.Errorf("catalog command %q error %q has a non-causal recovery self-loop", source.Path, declared.Code)
	}
	if declared.Code == "output_encoding_failed" {
		if source.programName() == ProgramName {
			want := "version"
			if source.Path == "version" {
				want = "help version"
			}
			if invocation != want {
				return fmt.Errorf("catalog command %q output encoding recovery must hand off to %q", source.Path, want)
			}
		} else if invocation == source.Path {
			return fmt.Errorf("catalog command %q output encoding recovery repeats the encoding-failing helper command", source.Path)
		}
	}
	if declared.Retryable || strings.HasPrefix(invocation, "help ") || invocation == "help" {
		return nil
	}
	for _, input := range target.Agent.Inputs {
		if input.Required {
			return fmt.Errorf("catalog command %q error %q points to %q without required typed input %q", source.Path, declared.Code, invocation, input.Name)
		}
	}
	return nil
}

// A trusted-terminal discover workflow may return to its own path only when
// the next invocation re-observes durable authority and crosses into one of
// its separately cataloged exact-reference actions. It is a workflow state
// transition, not a blind read-only classifier loop.
func isInteractiveRecoveryTransition(source CommandSpec, invocation string) bool {
	return invocation == source.Path && source.Agent.Interactive != nil && len(source.Agent.Interactive.actionCommands()) != 0
}

func isTerminalRecoveryInvocation(invocation string) bool {
	return invocation == "version" || invocation == "help" || strings.HasPrefix(invocation, "help ")
}

func validateAcyclicCausalRecoveries(edges []causalRecoveryEdge) error {
	adjacency := make(map[string][]causalRecoveryEdge)
	for _, edge := range edges {
		adjacency[edge.from] = append(adjacency[edge.from], edge)
	}
	const (
		unseen uint8 = iota
		visiting
		visited
	)
	state := make(map[string]uint8)
	var visit func(string) error
	visit = func(node string) error {
		state[node] = visiting
		for _, edge := range adjacency[node] {
			switch state[edge.to] {
			case visiting:
				return fmt.Errorf("catalog recovery graph contains a closed cycle from %q through error %q", node, edge.code)
			case unseen:
				if err := visit(edge.to); err != nil {
					return err
				}
			}
		}
		state[node] = visited
		return nil
	}
	for node := range adjacency {
		if state[node] == unseen {
			if err := visit(node); err != nil {
				return err
			}
		}
	}
	return nil
}
