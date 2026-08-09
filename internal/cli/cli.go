// Package cli owns command routing and presentation.
package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/tasuku43/tobari/internal/app/authcmd"
	"github.com/tasuku43/tobari/internal/app/contextcmd"
	"github.com/tasuku43/tobari/internal/app/doctorcmd"
	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
	"github.com/tasuku43/tobari/internal/infra/systemdoctor"
	"github.com/tasuku43/tobari/internal/infra/terminalstyle"
)

// CLI contains injected streams and application services.
type CLI struct {
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	Version string
	Commit  string

	catalog Catalog
	doctor  *doctorcmd.Service
	tobari  *tobaricmd.Service
	context *contextcmd.Service
	auth    *authcmd.Service
	config  contextConfigurationWizard
	noColor bool
}

// New builds the production CLI with the Docker-backed Tobari runtime.
func New(in io.Reader, out, errOut io.Writer) *CLI {
	command := newCLI(in, out, errOut, DefaultCatalog(), systemdoctor.New())
	command.noColor = noColorFromEnvironment()
	command.config = newContextConfigurationWizardWithStyle(!command.noColor)
	runtime, err := dockerruntime.New()
	if err == nil {
		command.tobari = tobaricmd.NewWithWorkspaceSelector(
			runtime,
			newWorkspaceSelectorWithStyle(!command.noColor),
		)
		command.context = contextcmd.New(runtime)
		command.auth = authcmd.New(runtime)
	}
	return command
}

func noColorFromEnvironment() bool {
	return terminalstyle.NoColorRequested()
}

func newCLI(in io.Reader, out, errOut io.Writer, catalog Catalog, inspector doctorcmd.InspectorPort) *CLI {
	if in == nil {
		in = strings.NewReader("")
	}
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	return &CLI{
		In: in, Out: out, Err: errOut,
		Version: "dev",
		catalog: catalog,
		doctor:  doctorcmd.New(inspector),
		config:  newContextConfigurationWizard(),
	}
}

// RunContext validates global options and the catalog, resolves one command,
// and propagates the same context to the selected application boundary.
func (c *CLI) RunContext(ctx context.Context, args []string) int {
	if c == nil {
		return ExitInternal
	}
	if ctx == nil {
		return c.fail(nil, fault.New(
			fault.KindContract,
			"missing_context",
			"The command context is not configured.",
			false,
			fault.NextAction{Command: "help", Reason: "Retry through a context-aware CLI entry point."},
		))
	}
	options, commandArgs, err := parseRootOptions(args)
	ctx = withErrorFormat(ctx, options.ErrorFormat)
	ctx = withExecutionContext(ctx, options.ContextName)
	if err != nil {
		return c.failUsage(ctx, "invalid_root_options", err.Error(), "help", "Correct the global options.")
	}
	if err := ctx.Err(); err != nil {
		return c.fail(ctx, err)
	}
	if err := c.catalog.Validate(); err != nil {
		return c.fail(ctx, fault.Wrap(
			fault.KindContract,
			"invalid_catalog",
			"The command catalog is invalid.",
			false,
			err,
			fault.NextAction{Command: "help", Reason: "Repair the catalog before dispatch."},
		))
	}
	if len(commandArgs) == 0 {
		// The root invocation is the primary interactive outcome. Help remains
		// explicit through `help` or `--help` and is handled before this branch.
		commandArgs = []string{"tobari"}
	}

	commandArgs = normalizeRootAlias(commandArgs)
	commandArgs = normalizeTrailingHelpAlias(c.catalog, commandArgs)
	command, rest, found := c.catalog.Match(commandArgs)
	if !found {
		if message, retired := retiredCommandMessage(commandArgs); retired {
			return c.failUsage(ctx, "retired_command", message, "tobari", "Run the root command from the project directory.")
		}
		return c.failUsage(
			ctx,
			"unknown_command",
			fmt.Sprintf("Unknown command %q.", strings.Join(commandArgs, " ")),
			"help",
			"Discover an exact command path or namespace.",
		)
	}
	ctx = withCommandPath(ctx, command.Path)
	if err := ctx.Err(); err != nil {
		return c.fail(ctx, err)
	}
	inputs, err := parseCommandInputs(command, rest)
	if err != nil {
		var nextActions []fault.NextAction
		for _, declared := range command.Agent.Errors {
			if declared.Code == "invalid_arguments" {
				nextActions = cloneSlice(declared.NextActions)
				break
			}
		}
		return c.fail(ctx, fault.Wrap(
			fault.KindInvalidInput,
			"invalid_arguments",
			err.Error()+"; usage: "+command.Usage(),
			false,
			err,
			nextActions...,
		))
	}

	intent := operation.Intent{Command: command.Path, Effect: command.Effect}
	if command.Effect == operation.EffectRead {
		if err := intent.Validate(); err != nil {
			return c.fail(ctx, fault.Wrap(
				fault.KindContract,
				"invalid_intent",
				"The command intent is invalid.",
				false,
				err,
				fault.NextAction{Command: "help " + command.Path, Reason: "Repair the command declaration."},
			))
		}
	}
	return command.handler(ctx, c, command, intent, inputs)
}

func retiredCommandMessage(args []string) (string, bool) {
	if len(args) == 0 {
		return "", false
	}
	switch args[0] {
	case "attach", "lower", "enter", "lift", "shell", "exec", "detach":
		return "The named Tobari lifecycle command has been removed. Run `tobari` from the project directory instead.", true
	case "logs":
		return "The named Tobari logs command has been removed. Run `tobari cluster logs` for shared component logs.", true
	default:
		return "", false
	}
}

func normalizeTrailingHelpAlias(catalog Catalog, args []string) []string {
	if len(args) < 2 || !isHelpFlag(args[len(args)-1]) {
		return args
	}
	selector := strings.Join(args[:len(args)-1], " ")
	commands, _ := catalog.Select(selector)
	if len(commands) == 0 {
		return args
	}
	return append([]string{"help"}, args[:len(args)-1]...)
}

func normalizeRootAlias(args []string) []string {
	switch args[0] {
	case "--help", "-h":
		return append([]string{"help"}, args[1:]...)
	case "--version", "-v":
		return append([]string{"version"}, args[1:]...)
	default:
		return args
	}
}

func isHelpFlag(value string) bool {
	return value == "--help" || value == "-h"
}

type rootOptions struct {
	ErrorFormat errorFormat
	ContextName string
}

type executionContextKey struct{}

func withExecutionContext(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, executionContextKey{}, name)
}

func executionContextName(ctx context.Context) string {
	value, _ := ctx.Value(executionContextKey{}).(string)
	return value
}

func parseRootOptions(args []string) (rootOptions, []string, error) {
	options := rootOptions{ErrorFormat: errorFormatText}
	seenErrorFormat := false
	seenContext := false
	index := 0
	for index < len(args) {
		argument := args[index]
		var value string
		switch {
		case argument == "--error-format":
			if index+1 >= len(args) {
				return options, nil, fmt.Errorf("--error-format requires text or json")
			}
			index++
			value = args[index]
		case strings.HasPrefix(argument, "--error-format="):
			value = strings.TrimPrefix(argument, "--error-format=")
		case argument == "--context":
			if index+1 >= len(args) || seenContext {
				return options, nil, fmt.Errorf("--context requires one Context name")
			}
			index++
			options.ContextName = args[index]
			if options.ContextName == "" {
				return options, nil, fmt.Errorf("--context requires one Context name")
			}
			seenContext = true
			index++
			continue
		case strings.HasPrefix(argument, "--context="):
			if seenContext {
				return options, nil, fmt.Errorf("--context may be specified only once")
			}
			options.ContextName = strings.TrimPrefix(argument, "--context=")
			if options.ContextName == "" {
				return options, nil, fmt.Errorf("--context requires one Context name")
			}
			seenContext = true
			index++
			continue
		default:
			return options, args[index:], nil
		}
		if seenErrorFormat {
			return options, nil, fmt.Errorf("--error-format may be specified only once")
		}
		parsed, err := parseErrorFormat(value)
		if err != nil {
			return options, nil, err
		}
		options.ErrorFormat = parsed
		seenErrorFormat = true
		index++
	}
	return options, args[index:], nil
}
