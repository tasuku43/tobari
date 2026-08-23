package cli

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type permissionWaitJSONDocument struct {
	SchemaVersion int                         `json:"schema_version"`
	Result        tobari.PermissionWaitResult `json:"result"`
}

func (c *CLI) requirePermissionWait(ctx context.Context) int {
	if c.permissionWait != nil {
		return ExitOK
	}
	if c.permissionWaitInitErr != nil {
		return c.fail(ctx, fault.Wrap(
			fault.KindUnavailable, "permission_wait_owner_unavailable", "The attachment permission wait owner is unavailable.", false,
			c.permissionWaitInitErr, fault.NextAction{Command: "help wait", Reason: "Use the helper only from the same live attachment that received the denial."},
		))
	}
	return c.fail(ctx, fault.New(
		fault.KindUnavailable, "permission_wait_owner_unavailable", "The attachment permission wait observer is unavailable.", false,
		fault.NextAction{Command: "help wait", Reason: "Use the helper only from the same live attachment that received the denial."},
	))
}

func runPermissionWait(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if code := c.requirePermissionWait(ctx); code != ExitOK {
		return code
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil || (format != successFormatText && format != successFormatJSON) {
		return c.failUsage(ctx, "invalid_arguments", "--format must be text or json; usage: "+command.Usage(), "help wait", "Correct the helper arguments.")
	}
	result, err := c.permissionWait.Wait(ctx, inputs.One("--id"))
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderPermissionWait(result, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitConsumedReadResult(ctx, command, output)
}

func renderPermissionWait(result tobari.PermissionWaitResult, format successFormat) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_permission_wait_result", "The permission wait result is invalid.", false, err)
	}
	if format == successFormatJSON {
		encoded, err := marshalCommandJSONForProgram(PermissionProgramName, "wait", permissionWaitJSONDocument{SchemaVersion: 1, Result: result})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "The permission wait JSON could not be encoded.", false, err)
		}
		return append(encoded, '\n'), nil
	}
	if format != successFormatText {
		return nil, fmt.Errorf("unsupported permission wait output format %q", format)
	}
	switch result {
	case tobari.PermissionWaitResultAllow:
		return []byte("Allow\n"), nil
	case tobari.PermissionWaitResultDeny:
		return []byte("Deny\n"), nil
	case tobari.PermissionWaitResultExpired:
		return []byte("Expired\n"), nil
	default:
		return nil, fmt.Errorf("unsupported permission wait result %q", result)
	}
}
