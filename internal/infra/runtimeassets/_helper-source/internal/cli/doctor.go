package cli

import (
	"bytes"
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

const (
	maxDoctorChecks       = 100
	maxDoctorNameBytes    = 256
	maxDoctorDetailBytes  = 64 * 1024
	maxDoctorActionBytes  = 64 * 1024
	maxDoctorCommandBytes = 4 * 1024
	doctorStatusWidth     = 6
)

func runDoctor(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help doctor", "Correct the command arguments.")
	}
	report, err := c.doctor.Run(ctx, intent, inputs.One("--root"))
	if err != nil {
		return c.fail(ctx, err)
	}
	if err := validateDoctorProjection(report); err != nil {
		return c.fail(ctx, err)
	}
	if !buildIdentityHasBroker() {
		checks := report.Checks[:0]
		for _, check := range report.Checks {
			if !isBrokerDoctorCheck(check.Name) {
				checks = append(checks, check)
			}
		}
		report.Checks = checks
	}
	output, err := renderDoctorReportWithColor(report, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	if code := c.emitResult(ctx, output); code != ExitOK {
		return code
	}
	if !report.Healthy() {
		return c.fail(ctx, fault.New(
			fault.KindRejected,
			"diagnostic_failed",
			"One or more diagnostics failed.",
			false,
			fault.NextAction{Command: "doctor", Reason: "Execute the first failed row recovery, then rerun diagnostics."},
		))
	}
	return ExitOK
}

func validateDoctorProjection(report doctor.Report) error {
	if len(report.Checks) > maxDoctorChecks {
		return outputContractExceeded("The diagnostic report exceeds the declared check limit.", "doctor")
	}
	for _, check := range report.Checks {
		if len(check.Name) > maxDoctorNameBytes || len(check.Detail) > maxDoctorDetailBytes {
			return outputContractExceeded("A diagnostic field exceeds the declared byte limit.", "doctor")
		}
		if check.Recovery != nil && (len(check.Recovery.Action) > maxDoctorActionBytes || len(check.Recovery.NextCommand) > maxDoctorCommandBytes) {
			return outputContractExceeded("A diagnostic recovery field exceeds the declared byte limit.", "doctor")
		}
	}
	return nil
}

type doctorJSONDocument struct {
	SchemaVersion int               `json:"schema_version"`
	Report        []doctorJSONCheck `json:"report"`
}

type doctorJSONCheck struct {
	Check     string              `json:"check"`
	Status    string              `json:"status"`
	Detail    string              `json:"detail"`
	BlockedBy *string             `json:"blocked_by"`
	Recovery  *doctorJSONRecovery `json:"recovery"`
}

type doctorJSONRecovery struct {
	Action      string `json:"action"`
	NextCommand string `json:"next_command"`
}

func renderDoctorReport(report doctor.Report, format successFormat) ([]byte, error) {
	return renderDoctorReportWithColor(report, format, false)
}

func renderDoctorReportWithColor(report doctor.Report, format successFormat, color bool) ([]byte, error) {
	if format == successFormatJSON {
		document := doctorJSONDocument{SchemaVersion: 1, Report: make([]doctorJSONCheck, 0, len(report.Checks))}
		for _, check := range report.Checks {
			var blockedBy *string
			if check.BlockedBy != nil {
				value := safeExternalText(string(*check.BlockedBy))
				blockedBy = &value
			}
			var recovery *doctorJSONRecovery
			if check.Recovery != nil {
				recovery = &doctorJSONRecovery{
					Action:      safeExternalText(check.Recovery.Action),
					NextCommand: safeExternalText(check.Recovery.NextCommand),
				}
			}
			document.Report = append(document.Report, doctorJSONCheck{
				Check:     safeExternalText(string(check.Name)),
				Status:    string(check.Status),
				Detail:    safeExternalText(check.Detail),
				BlockedBy: blockedBy,
				Recovery:  recovery,
			})
		}
		output, err := marshalCommandJSON("doctor", document)
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "The diagnostic JSON could not be encoded.", false, err)
		}
		return append(output, '\n'), nil
	}
	if format == successFormatText {
		output := newHumanOutput(color)
		marker, token := "✓", styleSuccess
		if !report.Healthy() {
			marker, token = "✗", styleDanger
		}
		output.heading(marker, "Environment check", token)
		if len(report.Checks) == 0 {
			output.empty("No checks returned", "The diagnostic provider returned an empty report.", "doctor", "Run diagnostics again after checking the local runtime.")
			return output.bytes(), nil
		}
		for _, check := range report.Checks {
			output.doctorCheck(check)
		}
		if recovery, exists := report.PrimaryRecovery(); exists {
			output.section("Recovery")
			output.next(recovery.NextCommand, recovery.Action)
		}
		return output.bytes(), nil
	}

	var output bytes.Buffer
	fmt.Fprintln(&output, "CHECK\tSTATUS\tBLOCKED_BY\tDETAIL\tRECOVERY_ACTION\tNEXT_COMMAND")
	for _, check := range report.Checks {
		blockedBy, action, nextCommand := "", "", ""
		if check.BlockedBy != nil {
			blockedBy = string(*check.BlockedBy)
		}
		if check.Recovery != nil {
			action = check.Recovery.Action
			nextCommand = check.Recovery.NextCommand
		}
		fmt.Fprintf(
			&output, "%s\t%s\t%s\t%s\t%s\t%s\n",
			escapeTSVCell(string(check.Name)), check.Status, escapeTSVCell(blockedBy),
			escapeTSVCell(check.Detail), escapeTSVCell(action), escapeTSVCell(nextCommand),
		)
	}
	return output.Bytes(), nil
}

func (o *humanOutput) doctorCheck(check doctor.Check) {
	name := escapeTSVCell(string(check.Name))
	status := escapeTSVCell(string(check.Status))
	detail := check.Detail
	if check.BlockedBy != nil {
		if detail != "" {
			detail += "; "
		}
		detail += "blocked by " + string(*check.BlockedBy)
	}
	paddedName := fmt.Sprintf("%-*s", humanOutputLabelWidth, name)
	if detail == "" {
		fmt.Fprintf(
			&o.Buffer, "  %s %s\n",
			applyStyleToken(o.color, styleText, paddedName),
			applyStyleToken(o.color, humanStatusToken(status), status),
		)
		return
	}
	paddedStatus := fmt.Sprintf("%-*s", doctorStatusWidth, status)
	fmt.Fprintf(
		&o.Buffer, "  %s %s %s\n",
		applyStyleToken(o.color, styleText, paddedName),
		applyStyleToken(o.color, humanStatusToken(status), paddedStatus),
		applyStyleToken(o.color, styleText, escapeTSVCell(detail)),
	)
}

func outputContractExceeded(message, command string) *fault.Error {
	return fault.New(
		fault.KindContract,
		"output_contract_exceeded",
		message,
		false,
		fault.NextAction{Command: command, Reason: "Review the bounded output contract and upstream response."},
	)
}
