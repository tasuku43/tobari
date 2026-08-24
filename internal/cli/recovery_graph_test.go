package cli

import (
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

func TestDefaultCatalogCausalRecoveryGraph(t *testing.T) {
	if buildIdentityHasBroker() {
		t.Skip("WP10 first-entry recovery is a release-surface contract; WP04 owns research-only recovery")
	}
	if err := validateCausalRecoveryGraph(DefaultCatalog()); err != nil {
		t.Fatal(err)
	}
}

func TestCausalRecoveryGraphRejectsNonRetryableSelfLoop(t *testing.T) {
	spec := utilitySpec("inspect")
	spec.Agent.Errors[0].Code = "inspection_failed"
	spec.Agent.Errors[0].NextActions[0].Command = spec.Path
	if err := validateCausalRecoveryGraph(NewCatalog(spec)); err == nil || !strings.Contains(err.Error(), "self-loop") {
		t.Fatalf("non-retryable self-loop error=%v", err)
	}
}

func TestCausalRecoveryGraphAcceptsInteractiveRecoveryTransition(t *testing.T) {
	spec := utilitySpec("review items")
	spec.Agent.Interactive = &InteractiveWorkflowContract{ActionCommand: "repair item"}
	if err := validateCausalRecoveryGraph(NewCatalog(spec)); err != nil {
		t.Fatalf("interactive recovery transition error=%v", err)
	}
}

func TestCausalRecoveryGraphRejectsUncheckedRequiredInputs(t *testing.T) {
	discover := utilitySpec("inspect")
	action := utilitySpec("repair")
	action.Agent.Inputs = []CommandInput{{
		Name: "--id", Source: InputSourceFlag, Required: true, ValueKind: InputValueText,
		Cardinality: InputCardinalitySingle, Description: "Exact repair target.", AllowedValues: []string{},
	}}
	discover.Agent.Errors[0].NextActions[0].Command = action.Path
	if err := validateCausalRecoveryGraph(NewCatalog(discover, action)); err == nil || !strings.Contains(err.Error(), "required typed input") {
		t.Fatalf("unchecked recovery input error=%v", err)
	}
}

func TestCausalRecoveryGraphRejectsClosedCommandCycle(t *testing.T) {
	first := utilitySpec("inspect first")
	second := utilitySpec("inspect second")
	first.Agent.Errors = []CommandError{declaredCommandError(fault.KindInternal, "first_failed", false, second.Path, "Inspect the second fixture.")}
	second.Agent.Errors = []CommandError{declaredCommandError(fault.KindInternal, "second_failed", false, first.Path, "Inspect the first fixture.")}
	if err := validateCausalRecoveryGraph(NewCatalog(first, second)); err == nil || !strings.Contains(err.Error(), "closed cycle") {
		t.Fatalf("closed recovery cycle error=%v", err)
	}
}

func TestCausalRecoveryGraphRejectsRepeatedEncodingCommand(t *testing.T) {
	inspect := utilitySpec("inspect")
	inspect.Agent.Errors = []CommandError{declaredCommandError(fault.KindContract, "output_encoding_failed", false, "help inspect", "Retry encoding.")}
	commands := append([]CommandSpec{}, DefaultCatalog().commands...)
	commands = append(commands, inspect)
	if err := validateCausalRecoveryGraph(NewCatalog(commands...)); err == nil || !strings.Contains(err.Error(), "output encoding recovery") {
		t.Fatalf("encoding recovery error=%v", err)
	}
}

func TestStatusHomeRecoveryBindingsRejectMissingCatalogTask(t *testing.T) {
	catalog := DefaultCatalog()
	commands := make([]CommandSpec, 0, len(catalog.commands)-1)
	for _, spec := range catalog.commands {
		if spec.programName() == ProgramName && spec.Path == "cluster up" {
			continue
		}
		commands = append(commands, spec)
	}
	if err := validateStatusHomeRecoveryBindings(NewCatalog(commands...)); err == nil || !strings.Contains(err.Error(), `status schema 3 recovery path "cluster up"`) {
		t.Fatalf("missing status recovery path error=%v", err)
	}
}

func TestReleaseRecoveryGraphDoesNotReachResearchSurface(t *testing.T) {
	if buildIdentityHasBroker() {
		t.Skip("release-only recovery assertion")
	}
	catalog := DefaultCatalog()
	for _, source := range catalog.commands {
		if source.programName() != ProgramName {
			continue
		}
		for _, declared := range source.Agent.Errors {
			for _, action := range declared.NextActions {
				target, err := catalog.resolveRecoveryCommandForProgram(ProgramName, action.Command)
				if err != nil {
					t.Fatalf("%s/%s: %v", source.Path, declared.Code, err)
				}
				if strings.HasPrefix(target.Path, "auth ") || target.Path == "serve" {
					t.Fatalf("release recovery %s/%s reaches research-only %q", source.Path, declared.Code, target.Path)
				}
			}
		}
	}
}
