package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/realm"
)

func runRealmUp(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.realm == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	impact := command.Agent.Mutation.Impact
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: realm.TargetKind, ParentID: realm.TargetID},
		Impact: impact,
	}
	status, err := c.realm.Up(ctx, intent, inputs.One("--root"))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, renderRealmStatusText(status))
}

func runRealmStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.realm == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	status, err := c.realm.Status(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help status", "Correct the command arguments.")
	}
	output, err := renderRealmStatus(status, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runRealmShell(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
	if c.realm == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	code, err := c.realm.Exec(
		ctx,
		realm.ExecRequest{Command: []string{"/bin/bash"}, Interactive: true, TTY: true},
		c.In,
		c.Out,
		c.Err,
	)
	if err != nil {
		return c.fail(ctx, err)
	}
	return code
}

func runRealmExec(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.realm == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	code, err := c.realm.Exec(
		ctx,
		realm.ExecRequest{
			HostCWD: inputs.One("--cwd"), Command: inputs.Values("command"),
			CWDExplicit: inputs.One("--cwd") != "", Interactive: true, TTY: true,
		},
		c.In,
		c.Out,
		c.Err,
	)
	if err != nil {
		return c.fail(ctx, err)
	}
	return code
}

func runRealmLogs(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.realm == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	output, err := c.realm.Logs(ctx, realm.LogRequest{
		Component: inputs.One("--component"),
		Tail:      int(tail),
	})
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, renderSafeLogs(output))
}

func runRealmDown(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.realm == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	purge, _ := inputs.Boolean("--purge")
	impact := command.Agent.Mutation.Impact
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: realm.TargetKind, ID: realm.TargetID},
		Impact: impact,
	}
	status, err := c.realm.Down(ctx, intent, purge)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, renderRealmStatusText(status))
}

type realmStatusDocument struct {
	SchemaVersion int               `json:"schema_version"`
	Status        realmStatusOutput `json:"status"`
}

type realmStatusOutput struct {
	Configured  bool                    `json:"configured"`
	Running     bool                    `json:"running"`
	Root        string                  `json:"root"`
	Proxy       string                  `json:"proxy"`
	Policy      string                  `json:"policy"`
	Components  []realm.ComponentStatus `json:"components"`
	RecentError string                  `json:"recent_error"`
}

func renderRealmStatus(status realm.Status, format successFormat) ([]byte, error) {
	if format == successFormatJSON {
		components := append([]realm.ComponentStatus{}, status.Components...)
		document := realmStatusDocument{
			SchemaVersion: 1,
			Status: realmStatusOutput{
				Configured: status.Configured, Running: status.Running,
				Root: safeExternalText(status.Root), Proxy: safeExternalText(status.Proxy),
				Policy: safeExternalText(status.Policy), Components: components,
				RecentError: safeExternalText(status.RecentError),
			},
		}
		output, err := json.Marshal(document)
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract,
				"output_encoding_failed",
				"realm status JSON could not be encoded",
				false,
				err,
			)
		}
		return append(output, '\n'), nil
	}
	return renderRealmStatusText(status), nil
}

func renderRealmStatusText(status realm.Status) []byte {
	var output bytes.Buffer
	if !status.Configured {
		fmt.Fprintln(&output, "configured: false")
		fmt.Fprintln(&output, "running: false")
		return output.Bytes()
	}
	fmt.Fprintln(&output, "configured: true")
	fmt.Fprintf(&output, "running: %t\n", status.Running)
	fmt.Fprintf(&output, "root: %s\n", escapeTSVCell(status.Root))
	fmt.Fprintf(&output, "proxy: %s\n", escapeTSVCell(status.Proxy))
	fmt.Fprintf(&output, "policy: %s\n", escapeTSVCell(status.Policy))
	for _, component := range status.Components {
		fmt.Fprintf(
			&output,
			"component: %s state=%s health=%s\n",
			escapeTSVCell(component.Name),
			escapeTSVCell(component.State),
			escapeTSVCell(component.Health),
		)
	}
	if status.RecentError != "" {
		fmt.Fprintf(&output, "recent_error: %s\n", escapeTSVCell(status.RecentError))
	}
	return output.Bytes()
}

func renderSafeLogs(raw []byte) []byte {
	var output strings.Builder
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		output.WriteString(safeExternalText(line))
		output.WriteByte('\n')
	}
	return []byte(output.String())
}

func missingRuntimeFault() *fault.Error {
	return fault.New(
		fault.KindInternal,
		"missing_runtime",
		"Tobari runtime is not configured.",
		false,
		fault.NextAction{Command: "doctor", Reason: "Inspect local runtime configuration."},
	)
}
