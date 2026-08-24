package cli

import (
	"context"
	"io"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

// emitResult performs exactly one checked write after a command has rendered
// and validated its complete output in memory. It resolves the effect from the
// catalog-bound dispatch context so handlers cannot substitute a weaker output
// contract.
func (c *CLI) emitResult(ctx context.Context, output []byte) int {
	path, bound := boundCommandPath(ctx)
	if !bound {
		return c.fail(ctx, fault.New(
			fault.KindContract,
			"missing_context",
			"The command output context is missing.",
			false,
			fault.NextAction{Command: "help", Reason: "Retry through the context-aware CLI entry point."},
		))
	}
	command, found := c.catalog.lookupRegistered(path)
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract,
			"invalid_catalog",
			"The command output contract is missing from the catalog.",
			false,
			fault.NextAction{Command: "help", Reason: "Repair the catalog before emitting output."},
		))
	}
	switch command.Effect {
	case operation.EffectRead:
		if err := ctx.Err(); err != nil {
			return c.fail(ctx, err)
		}
		return c.emitComplete(ctx, output)
	case operation.EffectCreate, operation.EffectWrite:
		return c.emitMutationResult(ctx, command, output)
	default:
		return c.fail(ctx, fault.New(
			fault.KindContract,
			"invalid_catalog",
			"The command output effect is invalid.",
			false,
			fault.NextAction{Command: "help " + command.Path, Reason: "Declare the command effect before emitting output."},
		))
	}
}

// emitMutationResult writes a result after a mutation action has returned
// confirmed success. It deliberately does not reclassify that success as a
// retryable cancellation if the context becomes done after the action. A write
// failure is also non-retryable: the mutation already succeeded, so its catalog
// recovery must reconcile through a read rather than repeat the mutation.
func (c *CLI) emitMutationResult(ctx context.Context, command CommandSpec, output []byte) int {
	return c.emitMutationResultTo(ctx, command, output, c.Out)
}

// emitMutationResultTo preserves the mutation-complete output boundary for a
// catalog-owned action embedded in an interactive workflow. Root workflow
// progress belongs on stderr because stdout is handed to the child session.
func (c *CLI) emitMutationResultTo(
	ctx context.Context, command CommandSpec, output []byte, writer io.Writer,
) int {
	var recovery []fault.NextAction
	for _, declared := range command.Agent.Errors {
		if declared.Code == "mutation_output_write_failed" &&
			declared.Kind == fault.KindInternal && !declared.Retryable {
			recovery = cloneSlice(declared.NextActions)
			break
		}
	}
	if command.Effect == operation.EffectRead || len(recovery) == 0 {
		return c.fail(ctx, fault.New(
			fault.KindContract,
			"invalid_catalog",
			"The mutation output contract is invalid.",
			false,
			fault.NextAction{Command: "help " + command.Path, Reason: "Declare non-retryable mutation output recovery through a read-only command."},
		))
	}
	if _, err := writeOnce(writer, output); err != nil {
		return c.fail(ctx, fault.WithClassification(fault.Wrap(
			fault.KindInternal,
			"mutation_output_write_failed",
			"The mutation succeeded, but its output could not be written completely.",
			false,
			err,
			recovery...,
		), fault.PhasePresentation, fault.ChangeConfirmed))
	}
	return ExitOK
}

// emitComplete performs the common complete-or-no-success write. Callers must
// decide whether cancellation is still authoritative before reaching it.
func (c *CLI) emitComplete(ctx context.Context, output []byte) int {
	if _, err := writeOnce(c.Out, output); err != nil {
		return c.fail(ctx, fault.Wrap(
			fault.KindInternal,
			"output_write_failed",
			"The command output could not be written completely.",
			true,
			err,
			fault.NextAction{Command: invocationCommandPath(ctx), Reason: "Retry with a writable output stream."},
		))
	}
	return ExitOK
}

// emitConsumedReadResult settles a read whose bounded correlation record is
// already terminal and consumed. Later cancellation or output failure cannot
// advertise replay of the same input as safe.
func (c *CLI) emitConsumedReadResult(ctx context.Context, command CommandSpec, output []byte) int {
	if command.Effect != operation.EffectRead || command.Agent.Output.readSettlement != readOutputSettlementConsumed {
		return c.fail(ctx, fault.New(fault.KindContract, "invalid_catalog", "The consumed read output contract is invalid.", false,
			fault.NextAction{Command: "help " + command.Path, Reason: "Repair the command output settlement."}))
	}
	var recovery []fault.NextAction
	for _, declared := range command.Agent.Errors {
		if declared.Code == consumedReadOutputWriteFailureCode && declared.Kind == fault.KindInternal && !declared.Retryable {
			recovery = cloneSlice(declared.NextActions)
			break
		}
	}
	if len(recovery) == 0 {
		return c.fail(ctx, fault.New(fault.KindContract, "invalid_catalog", "The consumed read recovery contract is invalid.", false,
			fault.NextAction{Command: "help " + command.Path, Reason: "Repair the command output settlement."}))
	}
	if _, err := writeOnce(c.Out, output); err != nil {
		return c.fail(ctx, fault.WithClassification(fault.Wrap(
			fault.KindInternal, consumedReadOutputWriteFailureCode,
			"The terminal permission observation was consumed, but its output could not be written completely.", false, err, recovery...,
		), fault.PhasePresentation, fault.ChangeNotApplicable))
	}
	return ExitOK
}
