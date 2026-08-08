package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

const (
	maxDoctorChecks      = 100
	maxDoctorNameBytes   = 256
	maxDoctorDetailBytes = 64 * 1024
	doctorStatusWidth    = 6
)

func runDoctor(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help doctor", "Correct the command arguments.")
	}
	var report doctor.Report
	if c.tobari != nil {
		report, err = c.tobari.Doctor(ctx, inputs.One("--root"))
	} else {
		report, err = c.doctor.Run(ctx, intent)
	}
	if err != nil {
		return c.fail(ctx, err)
	}
	if err := validateDoctorProjection(report); err != nil {
		return c.fail(ctx, err)
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
			fault.NextAction{Command: "doctor", Reason: "Review the report, correct the failed prerequisite, and rerun diagnostics."},
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
	}
	return nil
}

type doctorJSONDocument struct {
	SchemaVersion int               `json:"schema_version"`
	Report        []doctorJSONCheck `json:"report"`
}

type doctorJSONCheck struct {
	Check  string `json:"check"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func renderDoctorReport(report doctor.Report, format successFormat) ([]byte, error) {
	return renderDoctorReportWithColor(report, format, false)
}

func renderDoctorReportWithColor(report doctor.Report, format successFormat, color bool) ([]byte, error) {
	if format == successFormatJSON {
		document := doctorJSONDocument{SchemaVersion: 1, Report: make([]doctorJSONCheck, 0, len(report.Checks))}
		for _, check := range report.Checks {
			document.Report = append(document.Report, doctorJSONCheck{
				Check:  safeExternalText(check.Name),
				Status: string(check.Status),
				Detail: safeExternalText(check.Detail),
			})
		}
		output, err := json.Marshal(document)
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "The diagnostic JSON could not be encoded.", false, err)
		}
		return append(output, '\n'), nil
	}
	if format == successFormatText {
		output := newHumanOutput(color)
		output.heading("✓", "Environment check", styleSuccess)
		if len(report.Checks) == 0 {
			output.empty("No checks returned", "The diagnostic provider returned an empty report.", "doctor", "Run diagnostics again after checking the local runtime.")
			return output.bytes(), nil
		}
		for _, check := range report.Checks {
			output.doctorCheck(check)
		}
		return output.bytes(), nil
	}

	var output bytes.Buffer
	fmt.Fprintln(&output, "CHECK\tSTATUS\tDETAIL")
	for _, check := range report.Checks {
		fmt.Fprintf(&output, "%s\t%s\t%s\n", escapeTSVCell(check.Name), check.Status, escapeTSVCell(check.Detail))
	}
	return output.Bytes(), nil
}

func (o *humanOutput) doctorCheck(check doctor.Check) {
	name := escapeTSVCell(check.Name)
	status := escapeTSVCell(string(check.Status))
	paddedName := fmt.Sprintf("%-*s", humanOutputLabelWidth, name)
	if check.Detail == "" {
		fmt.Fprintf(
			&o.Buffer, "  %s %s\n",
			applyStyleToken(o.color, styleMuted, paddedName),
			applyStyleToken(o.color, humanStatusToken(status), status),
		)
		return
	}
	paddedStatus := fmt.Sprintf("%-*s", doctorStatusWidth, status)
	fmt.Fprintf(
		&o.Buffer, "  %s %s %s\n",
		applyStyleToken(o.color, styleMuted, paddedName),
		applyStyleToken(o.color, humanStatusToken(status), paddedStatus),
		applyStyleToken(o.color, styleText, escapeTSVCell(check.Detail)),
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
