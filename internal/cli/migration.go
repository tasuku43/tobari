package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type migrationDocument struct {
	SchemaVersion int                    `json:"schema_version"`
	Migration     tobari.MigrationReport `json:"migration"`
}

func runMigration(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	if c == nil {
		return ExitInternal
	}
	if c.migrate == nil {
		return c.fail(ctx, missingRuntimeFault())
	}
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help migrate", "Correct the command arguments.")
	}
	intent.Target = operation.TargetRef{Kind: tobari.MigrationTargetKind, ID: tobari.MigrationTargetID}
	intent.Impact = command.Agent.Mutation.Impact
	result, err := c.migrate.Apply(ctx, intent, c.Err)
	if err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderMigration(result, format, humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitMutationResult(ctx, command, output)
}

func renderMigration(result tobari.MigrationReport, format successFormat, color bool) ([]byte, error) {
	if err := result.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindContract, "invalid_migration_report", "migration report is invalid", false, err)
	}
	if format == successFormatJSON {
		output, err := marshalCommandJSON("migrate apply", migrationDocument{SchemaVersion: 1, Migration: result})
		if err != nil {
			return nil, err
		}
		return append(output, '\n'), nil
	}
	var output strings.Builder
	title := "Migration complete"
	if !result.Changed {
		title = "Migration not required"
	}
	output.WriteString(applyStyleToken(color, styleSuccess, title))
	output.WriteString("\n\n")
	writeContextCardValue(&output, color, "Source", result.Source, styleText)
	writeContextCardValue(&output, color, "Changed", fmt.Sprintf("%t", result.Changed), humanStatusToken(map[bool]string{true: "ready", false: "not_configured"}[result.Changed]))
	if result.Backup != nil {
		writeContextCardValue(&output, color, "Backup", safeExternalText(*result.Backup), styleText)
	}
	output.WriteString("\nContexts\n")
	for _, item := range result.Contexts {
		fmt.Fprintf(&output, "  %s  %s  %s  %s\n", safeExternalText(item.Name), item.State, safeExternalText(item.Runtime), item.PolicyRevision[:19])
	}
	return []byte(output.String()), nil
}
