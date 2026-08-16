package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func renderPolicyPreset(result tobari.PolicyPresetResult, format successFormat) ([]byte, error) {
	if format == successFormatJSON {
		return json.MarshalIndent(struct {
			SchemaVersion int                       `json:"schema_version"`
			PolicyPresets tobari.PolicyPresetResult `json:"policy_presets"`
		}{1, result}, "", "  ")
	}
	var output strings.Builder
	output.WriteString("Policy presets\n")
	if result.Task == tobari.TaskPolicyPresetList {
		for _, item := range result.Items {
			fmt.Fprintf(&output, "%s  %s  guardrail=%s  destination=%s/%d  methods=%s/%d  immediate_grants=%d\n", safeExternalText(item.Origin), item.Revision, item.Guardrail, item.DestinationCeiling, item.DestinationCount, item.MethodCeiling, item.MethodCount, item.ImmediateGrantCount)
		}
	} else if result.Preset != nil {
		grants := len(result.Preset.BaselineGrants) + len(result.Preset.BaselineTemplates) + len(result.Preset.MCPBaselineGrants)
		fmt.Fprintf(&output, "Origin: %s\nRevision: %s\nGuardrail: %s\nImmediate grants: %d\n", safeExternalText(result.Origin), result.Revision, result.Preset.Guardrail, grants)
		if result.SourcePath != "" {
			fmt.Fprintf(&output, "Source: %s\n", safeExternalText(result.SourcePath))
		}
		fmt.Fprintf(&output, "Scope: %s\n", safeExternalText(result.Scope))
		for _, limitation := range result.Limitations {
			fmt.Fprintf(&output, "Limitation: %s\n", safeExternalText(limitation))
		}
	}
	return []byte(output.String()), nil
}

func policyPresetFormat(command CommandSpec, inputs ParsedInputs) (successFormat, error) {
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return format, fmt.Errorf("%s; usage: %s", err, command.Usage())
	}
	return format, nil
}

func runPolicyPresetList(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.policyPreset == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.policyPreset.List(ctx)
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := policyPresetFormat(command, inputs)
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error(), "help "+command.Path, "Correct the command arguments.")
	}
	output, err := renderPolicyPreset(result, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}
func runPolicyPresetShow(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.policyPreset == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.policyPreset.Show(ctx, inputs.One("--name"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := policyPresetFormat(command, inputs)
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error(), "help "+command.Path, "Correct the command arguments.")
	}
	output, err := renderPolicyPreset(result, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}
func runPolicyPresetValidate(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.policyPreset == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.policyPreset.Validate(ctx, inputs.One("--name"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := policyPresetFormat(command, inputs)
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error(), "help "+command.Path, "Correct the command arguments.")
	}
	output, err := renderPolicyPreset(result, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}
func runPolicyPresetInit(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	intent.Target = operation.TargetRef{Kind: tobari.PolicyPresetCatalogTargetKind, ParentID: tobari.PolicyPresetCatalogTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	if c == nil {
		return ExitInternal
	}
	if c.policyPreset == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	result, err := c.policyPreset.Init(ctx, intent, inputs.One("--name"))
	if err != nil {
		return c.fail(ctx, err)
	}
	format, err := policyPresetFormat(command, inputs)
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error(), "help "+command.Path, "Correct the command arguments.")
	}
	output, err := renderPolicyPreset(result, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}
