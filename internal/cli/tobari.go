package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func runClusterUp(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, _ ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ParentID: tobari.ClusterTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	status, err := c.tobari.ClusterUp(ctx, intent)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderClusterStatusText(status))
}

func runClusterStatus(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	status, err := c.tobari.ClusterStatus(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help cluster status", "Correct the command arguments.")
	}
	output, err := renderClusterStatus(status, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runClusterLogs(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	output, err := c.tobari.ClusterLogs(ctx, tobari.LogRequest{
		Component: inputs.One("--component"), Tail: int(tail),
	})
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, renderSafeLogs(output))
}

func runClusterDown(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	purge, _ := inputs.Boolean("--purge")
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	status, err := c.tobari.ClusterDown(ctx, intent, purge)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderClusterStatusText(status))
}

func runAttach(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ParentID: tobari.ClusterTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	instance, err := c.tobari.Attach(ctx, intent, inputs.One("--name"), inputs.One("--root"))
	if err != nil {
		return c.fail(ctx, err)
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "name: %s\n", escapeTSVCell(instance.Name))
	fmt.Fprintf(&output, "root: %s\n", escapeTSVCell(instance.Root))
	return c.emitMutationResult(ctx, command, output.Bytes())
}

func runList(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.tobari.List(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help list", "Correct the command arguments.")
	}
	output, err := renderTobariList(result, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runTobariShell(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	code, err := c.tobari.Exec(
		ctx, inputs.One("--id"),
		tobari.ExecRequest{Command: []string{"/bin/bash"}, Interactive: true, TTY: true},
		c.In, c.Out, c.Err,
	)
	if err != nil {
		return c.fail(ctx, err)
	}
	return code
}

func runTobariExec(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	code, err := c.tobari.Exec(
		ctx, inputs.One("--id"),
		tobari.ExecRequest{
			HostCWD: inputs.One("--cwd"), Command: inputs.Values("command"),
			CWDExplicit: inputs.One("--cwd") != "", Interactive: true, TTY: true,
		},
		c.In, c.Out, c.Err,
	)
	if err != nil {
		return c.fail(ctx, err)
	}
	return code
}

func runTobariLogs(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	output, err := c.tobari.TobariLogs(ctx, inputs.One("--id"), int(tail))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, renderSafeLogs(output))
}

func runDetach(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	id := inputs.One("--id")
	purge, _ := inputs.Boolean("--purge")
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.TargetKind, ID: id},
		Impact: command.Agent.Mutation.Impact,
	}
	if err := c.tobari.Detach(ctx, intent, id, purge); err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, []byte("detached: true\n"))
}

type clusterStatusDocument struct {
	SchemaVersion int                 `json:"schema_version"`
	Cluster       clusterStatusOutput `json:"cluster"`
}

type clusterStatusOutput struct {
	Configured  bool                     `json:"configured"`
	Running     bool                     `json:"running"`
	Proxy       string                   `json:"proxy"`
	Policy      string                   `json:"policy"`
	TobariCount int                      `json:"tobari_count"`
	Components  []tobari.ComponentStatus `json:"components"`
	RecentError string                   `json:"recent_error"`
}

func renderClusterStatus(status tobari.ClusterStatus, format successFormat) ([]byte, error) {
	if format == successFormatJSON {
		document := clusterStatusDocument{
			SchemaVersion: 1,
			Cluster: clusterStatusOutput{
				Configured: status.Configured, Running: status.Running,
				Proxy: safeExternalText(status.Proxy), Policy: safeExternalText(status.Policy),
				TobariCount: status.TobariCount,
				Components:  append([]tobari.ComponentStatus{}, status.Components...),
				RecentError: safeExternalText(status.RecentError),
			},
		}
		output, err := json.Marshal(document)
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "cluster status JSON could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}
	return renderClusterStatusText(status), nil
}

func renderClusterStatusText(status tobari.ClusterStatus) []byte {
	var output bytes.Buffer
	if !status.Configured {
		fmt.Fprintln(&output, "configured: false")
		fmt.Fprintln(&output, "running: false")
		return output.Bytes()
	}
	fmt.Fprintln(&output, "configured: true")
	fmt.Fprintf(&output, "running: %t\n", status.Running)
	fmt.Fprintf(&output, "proxy: %s\n", escapeTSVCell(status.Proxy))
	fmt.Fprintf(&output, "policy: %s\n", escapeTSVCell(status.Policy))
	fmt.Fprintf(&output, "tobari_count: %d\n", status.TobariCount)
	for _, component := range status.Components {
		fmt.Fprintf(
			&output, "component: %s state=%s health=%s\n",
			escapeTSVCell(component.Name), escapeTSVCell(component.State), escapeTSVCell(component.Health),
		)
	}
	if status.RecentError != "" {
		fmt.Fprintf(&output, "recent_error: %s\n", escapeTSVCell(status.RecentError))
	}
	return output.Bytes()
}

type tobariListDocument struct {
	SchemaVersion int                 `json:"schema_version"`
	Tobari        []tobari.ItemStatus `json:"tobari"`
}

func renderTobariList(result tobari.ListResult, format successFormat) ([]byte, error) {
	if format == successFormatJSON {
		items := append([]tobari.ItemStatus{}, result.Items...)
		for index := range items {
			items[index].Name = safeExternalText(items[index].Name)
			items[index].Root = safeExternalText(items[index].Root)
			items[index].Container = safeExternalText(items[index].Container)
		}
		output, err := json.Marshal(tobariListDocument{SchemaVersion: 1, Tobari: items})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Tobari list JSON could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}
	var output bytes.Buffer
	for _, item := range result.Items {
		fmt.Fprintf(
			&output, "id=%s\tname=%s\troot=%s\trunning=%t\tcontainer=%s\n",
			item.ID, escapeTSVCell(item.Name), escapeTSVCell(item.Root), item.Running, escapeTSVCell(item.Container),
		)
	}
	return output.Bytes(), nil
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
		fault.KindInternal, "missing_runtime", "Tobari runtime is not configured.", false,
		fault.NextAction{Command: "doctor", Reason: "Inspect local runtime configuration."},
	)
}
