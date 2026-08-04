package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func runContextList(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.context.List(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help context list", "Correct the command arguments.")
	}
	output, err := renderContextList(result, format, format == successFormatText && humanColorAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runContextShow(
	ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.context.Show(ctx, inputs.One("--name"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help context show", "Correct the command arguments.")
	}
	output, err := renderContextReport(result, format, format == successFormatText && humanColorAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runContextCreate(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	mode := tobari.ContextPolicyMode(inputs.One("--mode"))
	result, err := c.context.Create(ctx, intent, inputs.One("--name"), inputs.One("--image"), mode)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help context create", "Correct the command arguments.")
	}
	output, err := renderContextReport(result, format, humanColorAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runContextUse(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.ContextTargetKind, ID: tobari.ActiveContextTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help context use", "Correct the command arguments.")
	}
	var progress *clusterUpProgress
	var progressSink tobari.ClusterUpProgressSink
	if format == successFormatText && c.tobari != nil && c.tobari.IsTerminal(c.Err) && clusterUpProgressAllowed(ctx) {
		progress = newClusterUpProgress(c.Err, true)
		progress.Start()
		progressSink = progress.Report
		defer progress.Close()
	}
	result, err := c.context.UseWithProgress(ctx, intent, inputs.One("--name"), progressSink)
	if err != nil {
		if progress != nil {
			progress.Fail()
		}
		return c.fail(ctx, err)
	}
	output, err := renderContextReport(result, format, format == successFormatText && humanColorAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runRuntimeInit(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.ContextRuntimeTargetKind, ParentID: tobari.ActiveContextRuntimeID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.context.InitRuntime(ctx, intent)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime init", "Correct the command arguments.")
	}
	output, err := renderContextReport(result, format, humanColorAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func runRuntimeBuild(
	ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs,
) int {
	if c == nil {
		return ExitInternal
	}
	if c.context == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	intent.Target = operation.TargetRef{Kind: tobari.ContextRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.context.BuildRuntime(ctx, intent)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime build", "Correct the command arguments.")
	}
	output, err := renderContextReport(result, format, humanColorAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

type contextListDocument struct {
	SchemaVersion int `json:"schema_version"`
	Contexts      struct {
		Active string                  `json:"active"`
		Items  []tobari.ContextSummary `json:"items"`
	} `json:"contexts"`
}

type contextReportDocument struct {
	SchemaVersion int                  `json:"schema_version"`
	Context       tobari.ContextReport `json:"context"`
}

func renderContextList(result tobari.ContextListResult, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_context_list", "Context list is invalid", false, err)
	}
	if format == successFormatJSON {
		document := contextListDocument{SchemaVersion: 2}
		document.Contexts.Active = result.Active
		document.Contexts.Items = append([]tobari.ContextSummary{}, result.Items...)
		output, err := json.Marshal(document)
		if err != nil {
			return nil, err
		}
		return append(output, '\n'), nil
	}
	var output strings.Builder
	output.WriteString("Active Context: ")
	output.WriteString(safeExternalText(result.Active))
	output.WriteString("\n\nContexts:\n")
	for _, item := range result.Items {
		marker := " "
		if item.Active {
			marker = "*"
		}
		fmt.Fprintf(&output, "%s %s\tmode=%s\timage=%s\truntime=%s\tagent=%s\n", marker,
			safeExternalText(item.Name), item.PolicyMode, safeExternalText(item.Image), item.RuntimeStatus, safeExternalText(item.AgentProfile))
	}
	return []byte(output.String()), nil
}

func renderContextReport(result tobari.ContextReport, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_context_report", "Context report is invalid", false, err)
	}
	if format == successFormatJSON {
		output, err := json.Marshal(contextReportDocument{SchemaVersion: 2, Context: result})
		if err != nil {
			return nil, err
		}
		return append(output, '\n'), nil
	}
	return renderContextReportText(result, color), nil
}

func renderContextReportText(result tobari.ContextReport, color bool) []byte {
	var output strings.Builder
	fmt.Fprintf(&output, "Context: %s\n", safeExternalText(result.Name))
	fmt.Fprintf(&output, "Active: %t\n", result.Active)
	fmt.Fprintf(&output, "Image: %s\n", safeExternalText(result.Image))
	fmt.Fprintf(&output, "Agent profile: %s\n", safeExternalText(result.AgentProfile))
	fmt.Fprintf(&output, "Policy mode: %s\n", result.PolicyMode)
	if result.Task == tobari.TaskContextUse {
		fmt.Fprintf(&output, "Cluster: %s\n", result.Cluster)
	}
	if result.Runtime.Kind != "" {
		fmt.Fprintf(&output, "Runtime: %s (%s)\n", result.Runtime.Kind, result.Runtime.Status)
		if result.Runtime.Dockerfile != "" {
			fmt.Fprintf(&output, "Runtime Dockerfile: %s\n", safeExternalText(result.Runtime.Dockerfile))
		}
		if result.Runtime.BaseReference != "" {
			fmt.Fprintf(&output, "Runtime base: %s\n", safeExternalText(result.Runtime.BaseReference))
		}
		if result.Runtime.SourceDigest != "" {
			fmt.Fprintf(&output, "Runtime source digest: %s\n", safeExternalText(result.Runtime.SourceDigest))
		}
		if result.Runtime.ImageDigest != "" {
			fmt.Fprintf(&output, "Runtime image digest: %s\n", safeExternalText(result.Runtime.ImageDigest))
		}
	}
	if result.Runtime.Status == tobari.ContextRuntimeStatusOfficial {
		fmt.Fprintln(&output, runtimeCustomizationHint())
	}
	switch result.Task {
	case tobari.TaskRuntimeInit:
		if result.Runtime.Dockerfile != "" {
			fmt.Fprintf(&output, "Next: edit %s, then run `tobari runtime build`.\n", safeExternalText(result.Runtime.Dockerfile))
		}
	case tobari.TaskRuntimeBuild:
		fmt.Fprintln(&output, "Note: existing Workspaces keep their home. On the next `tobari`, Tobari recreates only the work container when this runtime image changes the spec.")
		fmt.Fprintln(&output, "Next: run `tobari` from a project directory.")
	case tobari.TaskContextUse:
		switch result.Cluster {
		case tobari.ContextClusterStatusReconciled, tobari.ContextClusterStatusAlreadyReady:
			fmt.Fprintln(&output, "Next: run `tobari` from a project directory.")
		case tobari.ContextClusterStatusNotConfigured, tobari.ContextClusterStatusNotRunning:
			fmt.Fprintln(&output, "Next: run `tobari cluster up`, then `tobari` from a project directory.")
		}
	}
	fmt.Fprintf(&output, "Policy: %s\n", safeExternalText(result.Stores.PolicyDirectory))
	fmt.Fprintf(&output, "Credential metadata: %s\n", safeExternalText(result.Stores.CredentialConfig))
	fmt.Fprintf(&output, "Credential directory: %s\n", safeExternalText(result.Stores.CredentialDirectory))
	return []byte(output.String())
}

func runtimeCustomizationHint() string {
	return "Tip: this Context is using the base runtime. For ongoing work, run `tobari runtime init`, edit the Dockerfile, then run `tobari runtime build` on the host."
}
