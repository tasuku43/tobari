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
	output, err := renderContextList(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
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
	output, err := renderContextReport(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
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
	output, err := renderContextReport(result, format, humanStyleAllowed(ctx, c, c.Out))
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
		progress = newClusterUpProgress(c.Err, humanStyleAllowed(ctx, c, c.Err))
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
	output, err := renderContextReport(result, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
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
	output, err := renderContextReport(result, format, humanStyleAllowed(ctx, c, c.Out))
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
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help runtime build", "Correct the command arguments.")
	}
	buildOutput := newRuntimeBuildOutput(c.Err, humanStyleAllowed(ctx, c, c.Err))
	result, err := c.context.BuildRuntimeWithProgress(ctx, intent, buildOutput, buildOutput.Report)
	if err != nil {
		code := c.fail(ctx, err)
		if invocationErrorFormat(ctx) == errorFormatText {
			buildOutput.WriteFailureSummary()
		}
		return code
	}
	buildOutput.Flush()
	output, err := renderContextReport(result, format, humanStyleAllowed(ctx, c, c.Out))
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
	writeStyledLine(&output, color, "Active Context:", safeExternalText(result.Active), styleText)
	output.WriteString("\n")
	output.WriteString(applyStyleToken(color, styleAccent, "Contexts:"))
	output.WriteString("\n")
	for _, item := range result.Items {
		marker := " "
		markerToken := styleMuted
		if item.Active {
			marker = "*"
			markerToken = styleAccent
		}
		fmt.Fprintf(
			&output, "%s %s\t%s=%s\t%s=%s\t%s=%s\t%s=%s\n",
			applyStyleToken(color, markerToken, marker),
			applyStyleToken(color, styleText, safeExternalText(item.Name)),
			applyStyleToken(color, styleMuted, "mode"), applyStyleToken(color, styleText, string(item.PolicyMode)),
			applyStyleToken(color, styleMuted, "image"), applyStyleToken(color, styleText, safeExternalText(item.Image)),
			applyStyleToken(color, styleMuted, "runtime"), applyStyleToken(color, humanStatusToken(string(item.RuntimeStatus)), string(item.RuntimeStatus)),
			applyStyleToken(color, styleMuted, "agent"), applyStyleToken(color, styleText, safeExternalText(item.AgentProfile)),
		)
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
	if result.Task == tobari.TaskRuntimeInit {
		return renderRuntimeInitReportText(result, color)
	}

	var output strings.Builder
	writeStyledLine(&output, color, "Context:", safeExternalText(result.Name), styleText)
	writeStyledLine(&output, color, "Active:", fmt.Sprintf("%t", result.Active), styleText)
	writeStyledLine(&output, color, "Image:", safeExternalText(result.Image), styleText)
	writeStyledLine(&output, color, "Agent profile:", safeExternalText(result.AgentProfile), styleText)
	writeStyledLine(&output, color, "Policy mode:", string(result.PolicyMode), styleText)
	if result.Task == tobari.TaskContextUse {
		writeStyledLine(&output, color, "Cluster:", string(result.Cluster), humanStatusToken(string(result.Cluster)))
	}
	if result.Runtime.Kind != "" {
		writeStyledLine(
			&output, color, "Runtime:",
			string(result.Runtime.Kind)+" ("+string(result.Runtime.Status)+")",
			humanStatusToken(string(result.Runtime.Status)),
		)
		if result.Runtime.Dockerfile != "" {
			writeStyledLine(&output, color, "Runtime Dockerfile:", safeExternalText(result.Runtime.Dockerfile), styleText)
		}
		if result.Runtime.BaseReference != "" {
			writeStyledLine(&output, color, "Runtime base:", safeExternalText(result.Runtime.BaseReference), styleText)
		}
		if result.Runtime.SourceDigest != "" {
			writeStyledLine(&output, color, "Runtime source digest:", safeExternalText(result.Runtime.SourceDigest), styleText)
		}
		if result.Runtime.ImageDigest != "" {
			writeStyledLine(&output, color, "Runtime image digest:", safeExternalText(result.Runtime.ImageDigest), styleText)
		}
	}
	if result.Runtime.Status == tobari.ContextRuntimeStatusOfficial {
		writeStyledLine(
			&output, color, "Tip:",
			strings.TrimPrefix(runtimeCustomizationHint(), "Tip: "),
			styleText,
		)
	}
	switch result.Task {
	case tobari.TaskRuntimeBuild:
		writeStyledLine(&output, color, "Note:", "existing Workspaces keep their home. On the next `tobari`, Tobari recreates only the work container when this runtime image changes the spec.", styleText)
		writeStyledCommandLine(&output, color, "Next:", "run ", "`tobari`", " from a project directory.")
	case tobari.TaskContextUse:
		switch result.Cluster {
		case tobari.ContextClusterStatusReconciled, tobari.ContextClusterStatusAlreadyReady:
			writeStyledCommandLine(&output, color, "Next:", "run ", "`tobari`", " from a project directory.")
		case tobari.ContextClusterStatusNotConfigured, tobari.ContextClusterStatusNotRunning:
			writeStyledCommandLine(&output, color, "Next:", "run ", "`tobari cluster up`, then `tobari`", " from a project directory.")
		}
	}
	writeStyledLine(&output, color, "Policy:", safeExternalText(result.Stores.PolicyDirectory), styleText)
	writeStyledLine(&output, color, "Credential metadata:", safeExternalText(result.Stores.CredentialConfig), styleText)
	writeStyledLine(&output, color, "Credential directory:", safeExternalText(result.Stores.CredentialDirectory), styleText)
	return []byte(output.String())
}

func renderRuntimeInitReportText(result tobari.ContextReport, color bool) []byte {
	output := newHumanOutput(color)
	output.heading("✓", "Runtime Dockerfile created", styleSuccess)
	output.section("Next")
	output.nextStep(1, "Edit the Dockerfile", safeExternalText(result.Runtime.Dockerfile), styleText)
	output.nextStep(2, "Build the runtime", recoveryCommand("runtime build"), styleAccent)
	output.section("Details")
	output.row("Context", safeExternalText(result.Name), styleText)
	output.row("Base image", safeExternalText(result.Runtime.BaseReference), styleText)
	output.row("Status", string(result.Runtime.Status), humanStatusToken(string(result.Runtime.Status)))
	return output.bytes()
}

func writeStyledLine(output *strings.Builder, enabled bool, label, value string, token styleToken) {
	fmt.Fprintf(
		output, "%s %s\n",
		applyStyleToken(enabled, styleMuted, label),
		applyStyleToken(enabled, token, value),
	)
}

func runtimeCustomizationHint() string {
	return "Tip: this Context is using the base runtime. For ongoing work, run `tobari runtime init`, edit the Dockerfile, then run `tobari runtime build` on the host."
}
