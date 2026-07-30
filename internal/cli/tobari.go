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

func runClusterDenials(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	result, err := c.tobari.ClusterDenials(ctx, int(tail))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(
			ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(),
			"help cluster denials", "Correct the command arguments.",
		)
	}
	apply, found := c.catalog.Lookup("policy apply")
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "policy apply command is missing", false,
		))
	}
	output, err := renderClusterDenials(result, ProgramName+" "+apply.Path, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runPolicyCandidates(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	return runPolicyCandidateQueue(ctx, c, command, inputs, false)
}

func runPolicyTail(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	return runPolicyCandidateQueue(ctx, c, command, inputs, true)
}

func runPolicyCandidateQueue(
	ctx context.Context, c *CLI, command CommandSpec, inputs ParsedInputs, tailView bool,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	tail, _ := inputs.Integer("--tail")
	var (
		result tobari.PolicyCandidateReport
		err    error
	)
	if tailView {
		result, err = c.tobari.PolicyTail(ctx, int(tail))
	} else {
		result, err = c.tobari.PolicyCandidates(ctx, int(tail))
	}
	if err != nil {
		return c.fail(ctx, err)
	}
	format := successFormatText
	if !tailView {
		format, err = parseSuccessFormat(inputs.One("--format"))
		if err != nil {
			return c.failUsage(
				ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(),
				"help "+command.Path, "Correct the command arguments.",
			)
		}
	}
	allow, found := c.catalog.Lookup("policy allow")
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "policy allow command is missing", false,
		))
	}
	output, err := renderPolicyCandidates(result, ProgramName+" "+allow.Path, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runPolicyAllow(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	id := inputs.One("--id")
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: id},
		Impact: command.Agent.Mutation.Impact,
	}
	result, err := c.tobari.AllowPolicyCandidate(ctx, intent, id)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderPolicyLearningChange(result))
}

func runPolicyCompactions(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.tobari.PolicyCompactions(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(
			ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(),
			"help "+command.Path, "Correct the command arguments.",
		)
	}
	compact, found := c.catalog.Lookup("policy compact")
	if !found {
		return c.fail(ctx, fault.New(
			fault.KindContract, "invalid_catalog", "policy compact command is missing", false,
		))
	}
	output, err := renderPolicyCompactions(result, ProgramName+" "+compact.Path, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runPolicyCompact(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	id := inputs.One("--id")
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.PolicyCompactionKind, ID: id},
		Impact: command.Agent.Mutation.Impact,
	}
	result, err := c.tobari.CompactPolicy(ctx, intent, id)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, renderPolicyLearningChange(result))
}

func runPolicyApply(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, _ ParsedInputs,
) int {
	if c.tobari == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent := operation.Intent{
		Command: command.Path, Effect: command.Effect,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID},
		Impact: command.Agent.Mutation.Impact,
	}
	result, err := c.tobari.ApplyPolicy(ctx, intent)
	if err != nil {
		return c.fail(ctx, err)
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "policy: %s\n", escapeTSVCell(result.PolicyDirectory))
	fmt.Fprintf(&output, "applied: %t\n", result.Applied)
	return c.emitMutationResult(ctx, command, output.Bytes())
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
	instance, err := c.tobari.Attach(
		ctx, intent, inputs.One("--name"), inputs.One("--root"),
		inputs.One("--image"), inputs.One("--devcontainer"),
	)
	if err != nil {
		return c.fail(ctx, err)
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "name: %s\n", escapeTSVCell(instance.Name))
	fmt.Fprintf(&output, "root: %s\n", escapeTSVCell(instance.Root))
	fmt.Fprintf(&output, "image: %s\n", escapeTSVCell(instance.ImageSelector()))
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

type clusterDenialsDocument struct {
	SchemaVersion int                  `json:"schema_version"`
	Denials       clusterDenialsOutput `json:"denials"`
}

type clusterDenialsOutput struct {
	Policy       string                `json:"policy"`
	WindowLines  int                   `json:"window_lines"`
	Items        []tobari.PolicyDenial `json:"items"`
	ApplyCommand string                `json:"apply_command"`
}

type policyCandidateOutput struct {
	ID                string  `json:"id"`
	ObservedAt        string  `json:"observed_at"`
	Host              string  `json:"host"`
	Method            string  `json:"method"`
	Path              string  `json:"path"`
	Reason            string  `json:"reason"`
	StatusCode        int     `json:"status_code"`
	CredentialProfile *string `json:"credential_profile"`
	AllowCommand      string  `json:"allow_command"`
}

type policyCandidatesDocument struct {
	SchemaVersion    int                     `json:"schema_version"`
	PolicyCandidates []policyCandidateOutput `json:"policy_candidates"`
}

func renderPolicyCandidates(
	result tobari.PolicyCandidateReport, allowCommand string, format successFormat,
) ([]byte, error) {
	items := make([]policyCandidateOutput, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, policyCandidateOutput{
			ID: item.ID, ObservedAt: safeExternalText(item.ObservedAt),
			Host: safeExternalText(item.Host), Method: safeExternalText(item.Method),
			Path: safeExternalText(item.Path), Reason: safeExternalText(item.Reason),
			StatusCode:        item.StatusCode,
			CredentialProfile: safeOptionalExternalText(item.CredentialProfile),
			AllowCommand:      allowCommand + " --id " + item.ID,
		})
	}
	if format == successFormatJSON {
		output, err := json.Marshal(policyCandidatesDocument{
			SchemaVersion: 1, PolicyCandidates: items,
		})
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract, "output_encoding_failed",
				"policy candidates JSON could not be encoded", false, err,
			)
		}
		return append(output, '\n'), nil
	}
	var output bytes.Buffer
	for _, item := range result.Items {
		action := allowCommand + " --id " + item.ID
		profile := "none"
		if item.CredentialProfile != nil {
			profile = *item.CredentialProfile
		}
		fmt.Fprintf(
			&output,
			"id=%s\tobserved_at=%s\thost=%s\tmethod=%s\tpath=%s\treason=%s\tstatus_code=%d\tcredential_profile=%s\tallow_command=%s\n",
			item.ID, escapeTSVCell(item.ObservedAt), escapeTSVCell(item.Host),
			escapeTSVCell(item.Method), escapeTSVCell(item.Path), escapeTSVCell(item.Reason),
			item.StatusCode, escapeTSVCell(profile), escapeTSVCell(action),
		)
	}
	return output.Bytes(), nil
}

type policyCompactionOutput struct {
	ID              string   `json:"id"`
	Host            string   `json:"host"`
	Method          string   `json:"method"`
	PathPrefix      string   `json:"path_prefix"`
	SourceRuleCount int      `json:"source_rule_count"`
	Examples        []string `json:"examples"`
	OutsideCanary   string   `json:"outside_canary"`
	CompactCommand  string   `json:"compact_command"`
}

type policyCompactionsDocument struct {
	SchemaVersion     int                      `json:"schema_version"`
	PolicyCompactions []policyCompactionOutput `json:"policy_compactions"`
}

func renderPolicyCompactions(
	result tobari.PolicyCompactionReport, compactCommand string, format successFormat,
) ([]byte, error) {
	items := make([]policyCompactionOutput, 0, len(result.Items))
	for _, item := range result.Items {
		examples := make([]string, len(item.Examples))
		for index, example := range item.Examples {
			examples[index] = safeExternalText(example)
		}
		items = append(items, policyCompactionOutput{
			ID: item.ID, Host: safeExternalText(item.Host), Method: safeExternalText(item.Method),
			PathPrefix: safeExternalText(item.PathPrefix), SourceRuleCount: len(item.SourceRuleIDs),
			Examples: examples, OutsideCanary: safeExternalText(item.OutsideCanary),
			CompactCommand: compactCommand + " --id " + item.ID,
		})
	}
	if format == successFormatJSON {
		output, err := json.Marshal(policyCompactionsDocument{
			SchemaVersion: 1, PolicyCompactions: items,
		})
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract, "output_encoding_failed",
				"policy compactions JSON could not be encoded", false, err,
			)
		}
		return append(output, '\n'), nil
	}
	var output bytes.Buffer
	for _, item := range result.Items {
		action := compactCommand + " --id " + item.ID
		fmt.Fprintf(
			&output,
			"id=%s\thost=%s\tmethod=%s\tpath_prefix=%s\tsource_rule_count=%d\texamples=%s\toutside_canary=%s\tcompact_command=%s\n",
			item.ID, escapeTSVCell(item.Host), escapeTSVCell(item.Method),
			escapeTSVCell(item.PathPrefix), len(item.SourceRuleIDs),
			escapeTSVCell(strings.Join(item.Examples, ",")), escapeTSVCell(item.OutsideCanary),
			escapeTSVCell(action),
		)
	}
	return output.Bytes(), nil
}

func renderPolicyLearningChange(result tobari.PolicyLearningChange) []byte {
	var output bytes.Buffer
	fmt.Fprintf(&output, "policy: %s\n", escapeTSVCell(result.PolicyDirectory))
	fmt.Fprintf(&output, "target_id: %s\n", result.TargetID)
	fmt.Fprintf(&output, "rule_id: %s\n", result.Rule.ID)
	fmt.Fprintf(&output, "match: %s\n", escapeTSVCell(result.Rule.Match))
	fmt.Fprintf(&output, "host: %s\n", escapeTSVCell(result.Rule.Host))
	fmt.Fprintf(&output, "method: %s\n", escapeTSVCell(result.Rule.Method))
	fmt.Fprintf(&output, "path: %s\n", escapeTSVCell(result.Rule.Path))
	fmt.Fprintf(&output, "source_rule_count: %d\n", result.SourceRuleCount)
	fmt.Fprintf(&output, "applied: %t\n", result.Applied)
	return output.Bytes()
}

func renderClusterDenials(
	result tobari.DenialReport, applyCommand string, format successFormat,
) ([]byte, error) {
	if format == successFormatJSON {
		items := append([]tobari.PolicyDenial{}, result.Items...)
		for index := range items {
			items[index].Timestamp = safeExternalText(items[index].Timestamp)
			items[index].RequestID = safeExternalText(items[index].RequestID)
			items[index].Host = safeExternalText(items[index].Host)
			items[index].Method = safeExternalText(items[index].Method)
			items[index].Path = safeExternalText(items[index].Path)
			items[index].Reason = safeExternalText(items[index].Reason)
		}
		output, err := json.Marshal(clusterDenialsDocument{
			SchemaVersion: 1,
			Denials: clusterDenialsOutput{
				Policy: safeExternalText(result.PolicyDirectory), WindowLines: result.WindowLines,
				Items: items, ApplyCommand: applyCommand,
			},
		})
		if err != nil {
			return nil, fault.Wrap(
				fault.KindContract, "output_encoding_failed",
				"cluster denials JSON could not be encoded", false, err,
			)
		}
		return append(output, '\n'), nil
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "policy: %s\n", escapeTSVCell(result.PolicyDirectory))
	fmt.Fprintf(&output, "window_lines: %d\n", result.WindowLines)
	fmt.Fprintf(&output, "denial_count: %d\n", len(result.Items))
	for _, item := range result.Items {
		fmt.Fprintf(
			&output,
			"denial: timestamp=%s\trequest_id=%s\thost=%s\tmethod=%s\tpath=%s\tstatus_code=%d\treason=%s\n",
			escapeTSVCell(item.Timestamp), escapeTSVCell(item.RequestID),
			escapeTSVCell(item.Host), escapeTSVCell(item.Method),
			escapeTSVCell(item.Path), item.StatusCode, escapeTSVCell(item.Reason),
		)
	}
	fmt.Fprintf(&output, "apply_command: %s\n", escapeTSVCell(applyCommand))
	return output.Bytes(), nil
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
			items[index].Image = safeExternalText(items[index].Image)
			items[index].Container = safeExternalText(items[index].Container)
		}
		output, err := json.Marshal(tobariListDocument{SchemaVersion: 2, Tobari: items})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "Tobari list JSON could not be encoded", false, err)
		}
		return append(output, '\n'), nil
	}
	var output bytes.Buffer
	for _, item := range result.Items {
		fmt.Fprintf(
			&output, "id=%s\tname=%s\troot=%s\timage=%s\trunning=%t\tcontainer=%s\n",
			item.ID, escapeTSVCell(item.Name), escapeTSVCell(item.Root), escapeTSVCell(item.Image),
			item.Running, escapeTSVCell(item.Container),
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
