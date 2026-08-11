package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

// Stable process exit codes let agents classify failures without prose.
const (
	ExitOK             = 0
	ExitUsage          = 2
	ExitInternal       = 3
	ExitAuthentication = 4
	ExitPermission     = 5
	ExitNotFound       = 6
	ExitAmbiguous      = 7
	ExitRateLimited    = 8
	ExitUnavailable    = 9
	ExitRejected       = 10
	ExitCanceled       = 11
	ExitUnsupported    = 12
	ExitContract       = 13
)

type errorFormat string

const (
	errorFormatText errorFormat = "text"
	errorFormatJSON errorFormat = "json"
)

func parseErrorFormat(value string) (errorFormat, error) {
	switch errorFormat(value) {
	case errorFormatText:
		return errorFormatText, nil
	case errorFormatJSON:
		return errorFormatJSON, nil
	default:
		return errorFormatText, fmt.Errorf("--error-format must be text or json")
	}
}

type invocationContextKey uint8

const (
	errorFormatContextKey invocationContextKey = iota
	commandPathContextKey
)

func withErrorFormat(ctx context.Context, format errorFormat) context.Context {
	return context.WithValue(ctx, errorFormatContextKey, format)
}

func withCommandPath(ctx context.Context, path string) context.Context {
	return context.WithValue(ctx, commandPathContextKey, path)
}

func invocationErrorFormat(ctx context.Context) errorFormat {
	if ctx != nil {
		if format, ok := ctx.Value(errorFormatContextKey).(errorFormat); ok {
			return format
		}
	}
	return errorFormatText
}

func invocationCommandPath(ctx context.Context) string {
	if path, ok := boundCommandPath(ctx); ok {
		return path
	}
	return "help"
}

func boundCommandPath(ctx context.Context) (string, bool) {
	if ctx != nil {
		if path, ok := ctx.Value(commandPathContextKey).(string); ok {
			return path, true
		}
	}
	return "", false
}

type errorDocument struct {
	SchemaVersion int          `json:"schema_version"`
	Error         errorPayload `json:"error"`
}

type errorPayload struct {
	Kind        fault.Kind         `json:"kind"`
	Code        string             `json:"code"`
	Message     string             `json:"message"`
	Retryable   bool               `json:"retryable"`
	RetryAfter  *string            `json:"retry_after"`
	NextActions []fault.NextAction `json:"next_actions"`
}

var structuredErrorContractFallback = []byte("{\"schema_version\":1,\"error\":{\"kind\":\"contract\",\"code\":\"error_output_contract_failed\",\"message\":\"The structured error output contract failed.\",\"retryable\":false,\"retry_after\":null,\"next_actions\":[{\"command\":\"help\",\"reason\":\"Inspect the command contract before retrying.\"}]}}\n")

func (c *CLI) failUsage(ctx context.Context, code, message, command, reason string) int {
	return c.fail(ctx, fault.New(
		fault.KindInvalidInput,
		code,
		message,
		false,
		fault.NextAction{Command: command, Reason: reason},
	))
}

func (c *CLI) fail(ctx context.Context, err error) int {
	structured := c.normalizeFault(ctx, err)
	payload := errorPayload{
		Kind:        structured.Kind,
		Code:        structured.Code,
		Message:     structured.Message,
		Retryable:   structured.Retryable,
		NextActions: append([]fault.NextAction{}, structured.NextActions...),
	}
	if structured.RetryAfter > 0 {
		value := structured.RetryAfter.String()
		payload.RetryAfter = &value
	}

	var output []byte
	exitKind := structured.Kind
	if invocationErrorFormat(ctx) == errorFormatJSON {
		encoded, marshalErr := marshalErrorJSON(errorDocument{SchemaVersion: 1, Error: payload})
		if marshalErr != nil {
			output = append([]byte{}, structuredErrorContractFallback...)
			exitKind = fault.KindContract
		} else {
			output = append(encoded, '\n')
		}
	} else {
		output = renderTextErrorWithColor(payload, humanStyleAllowed(ctx, c, c.Err))
	}
	_, _ = writeOnce(c.Err, output)
	return exitCodeForKind(exitKind)
}

func (c *CLI) normalizeFault(ctx context.Context, err error) *fault.Error {
	structured := normalizeUnboundFault(ctx, err)
	path, bound := boundCommandPath(ctx)
	if !bound {
		return structured
	}
	command, found := c.catalog.lookupRegistered(path)
	if !found {
		return structured
	}
	for _, declared := range command.Agent.Errors {
		if declared.Code != structured.Code {
			continue
		}
		if declared.Kind != structured.Kind || declared.Retryable != structured.Retryable {
			return undeclaredFaultContract(path)
		}
		structured.NextActions = append([]fault.NextAction{}, declared.NextActions...)
		return structured
	}
	return undeclaredFaultContract(path)
}

func normalizeUnboundFault(ctx context.Context, err error) *fault.Error {
	if public, ok := fault.PublicCopy(err); ok {
		return public
	}

	var structured *fault.Error
	if errors.As(err, &structured) {
		return fault.New(
			fault.KindContract,
			"invalid_fault_contract",
			"A failure did not satisfy the structured error contract.",
			false,
			fault.NextAction{Command: "help", Reason: "Repair the fault declaration before retrying."},
		)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fault.Wrap(
			fault.KindCanceled,
			"operation_canceled",
			"The operation was canceled.",
			true,
			err,
			fault.NextAction{Command: invocationCommandPath(ctx), Reason: "Retry when the caller is ready."},
		)
	}

	return fault.New(
		fault.KindInternal,
		"internal_error",
		"The command failed unexpectedly.",
		false,
	)
}

func undeclaredFaultContract(path string) *fault.Error {
	return fault.New(
		fault.KindContract,
		"undeclared_fault_contract",
		"A command emitted a failure that is not declared by its agent contract.",
		false,
		fault.NextAction{Command: "help " + path, Reason: "Align the runtime fault with the command catalog before retrying."},
	)
}

func renderTextError(payload errorPayload) []byte {
	return renderTextErrorWithColor(payload, false)
}

func renderTextErrorWithColor(payload errorPayload, color bool) []byte {
	if color {
		output := newHumanOutput(true)
		output.heading("✗", "Command failed", styleDanger)
		output.row("Message", escapeTSVCell(payload.Message), styleDanger)
		output.row("Kind", string(payload.Kind), styleText)
		output.row("Code", payload.Code, styleText)
		retryToken := styleText
		if payload.Retryable {
			retryToken = styleWarning
		}
		output.row("Retryable", humanBool(payload.Retryable), retryToken)
		if payload.RetryAfter == nil {
			if payload.Kind == fault.KindRateLimited {
				output.row("Retry after", "unknown", styleWarning)
			} else {
				output.row("Retry after", "none", styleText)
			}
		} else {
			output.row("Retry after", *payload.RetryAfter, styleWarning)
		}
		for _, action := range payload.NextActions {
			output.next(action.Command, action.Reason)
		}
		return output.bytes()
	}

	var output strings.Builder
	fmt.Fprintf(&output, "error: %s\n", applyStyleToken(color, styleDanger, escapeTSVCell(payload.Message)))
	fmt.Fprintf(&output, "kind: %s\n", applyStyleToken(color, styleText, string(payload.Kind)))
	fmt.Fprintf(&output, "code: %s\n", applyStyleToken(color, styleText, payload.Code))
	fmt.Fprintf(&output, "retryable: %t\n", payload.Retryable)
	if payload.RetryAfter == nil {
		if payload.Kind == fault.KindRateLimited {
			fmt.Fprintln(&output, "retry_after: unknown")
		} else {
			fmt.Fprintln(&output, "retry_after: none")
		}
	} else {
		fmt.Fprintf(&output, "retry_after: %s\n", *payload.RetryAfter)
	}
	for _, action := range payload.NextActions {
		fmt.Fprintf(
			&output, "next_action: %s — %s\n",
			applyStyleToken(color, styleAccent, recoveryCommand(action.Command)),
			applyStyleToken(color, styleText, escapeTSVCell(action.Reason)),
		)
	}
	return []byte(output.String())
}

func recoveryCommand(command string) string {
	if command == ProgramName {
		return ProgramName
	}
	return ProgramName + " " + escapeTSVCell(command)
}

func exitCodeForKind(kind fault.Kind) int {
	switch kind {
	case fault.KindInvalidInput:
		return ExitUsage
	case fault.KindAuthentication:
		return ExitAuthentication
	case fault.KindPermission:
		return ExitPermission
	case fault.KindNotFound:
		return ExitNotFound
	case fault.KindAmbiguous:
		return ExitAmbiguous
	case fault.KindRateLimited:
		return ExitRateLimited
	case fault.KindUnavailable:
		return ExitUnavailable
	case fault.KindRejected:
		return ExitRejected
	case fault.KindCanceled:
		return ExitCanceled
	case fault.KindUnsupported:
		return ExitUnsupported
	case fault.KindContract:
		return ExitContract
	default:
		return ExitInternal
	}
}

func writeOnce(writer io.Writer, data []byte) (int, error) {
	written, err := writer.Write(data)
	if err != nil {
		return written, err
	}
	if written != len(data) {
		return written, io.ErrShortWrite
	}
	return written, nil
}
