package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/operatorconsole"
)

type operatorConsoleRunner interface {
	Run(context.Context, operatorconsole.Backend, bool, func(operatorconsole.Session) error) error
}

func newOperatorConsoleRunner() operatorConsoleRunner { return operatorconsole.New() }

type operatorConsoleBackend struct {
	service *tobaricmd.Service
	apply   CommandSpec
	tail    int
}

func (b operatorConsoleBackend) Snapshot(ctx context.Context) (tobari.OperatorConsoleSnapshot, error) {
	return b.service.OperatorConsoleSnapshot(ctx, b.tail)
}

func (b operatorConsoleBackend) ApplyPolicyReview(
	ctx context.Context, set tobari.PolicyReviewDecisionSet,
) (tobari.PolicyReviewChange, error) {
	intent := operation.Intent{
		Command: b.apply.Path, Effect: b.apply.Effect,
		Target: operation.TargetRef{Kind: b.apply.Agent.FixedTarget.Kind, ID: b.apply.Agent.FixedTarget.ID},
		Impact: b.apply.Agent.Mutation.Impact,
	}
	return b.service.ApplyPolicyReviewDecisionSet(withCommandPath(ctx, b.apply.Path), intent, set)
}

func runServe(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	if c.console == nil {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_operator_console",
			"The operator console is not configured.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the local Tobari installation."},
		))
	}
	apply, found := c.catalog.lookupRegistered("policy apply-reviewed")
	if !found || apply.Agent.Mutation == nil || apply.Agent.FixedTarget == nil {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "The reviewed policy Apply contract is missing.", false,
			fault.NextAction{Command: "help policy review", Reason: "Repair the catalog-owned review workflow."},
		))
	}
	noOpen, _ := inputs.Boolean("--no-open")
	backend := operatorConsoleBackend{service: c.tobari, apply: apply, tail: 10_000}
	err := c.console.Run(ctx, backend, !noOpen, func(session operatorconsole.Session) error {
		opened := "manual URL ready"
		if session.BrowserOpened {
			opened = "opened in host browser"
		}
		output := fmt.Sprintf(
			"Operator Console ready\n  URL      %s\n  Browser  %s\n  Bind     IPv4 loopback, random port\n  Stop     Ctrl-C\n",
			session.URL, opened,
		)
		written, err := io.WriteString(c.Out, output)
		if err == nil && written != len(output) {
			err = io.ErrShortWrite
		}
		if err != nil {
			return fault.Wrap(
				fault.KindInternal, "output_write_failed",
				"The operator console URL could not be written.", true, err,
				fault.NextAction{Command: command.Path, Reason: "Retry with a writable output stream."},
			)
		}
		return nil
	})
	if err != nil {
		return c.fail(ctx, err)
	}
	return ExitOK
}
