package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/sample"
)

const (
	maxSampleOutputItems  = 10_000
	maxSampleNameBytes    = 1024
	maxSampleContentBytes = 1024 * 1024
	maxSampleListBytes    = 4 * 1024 * 1024
)

func runSampleList(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help sample list", "Correct the command arguments.")
	}
	items, err := c.samples.List(ctx, intent)
	if err != nil {
		return c.fail(ctx, err)
	}
	if err := validateSampleSummaries(items); err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderSampleList(items, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func runSampleRead(ctx context.Context, c *CLI, command CommandSpec, intent operation.Intent, inputs ParsedInputs) int {
	id := inputs.One("--id")
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help sample read", "Correct the command arguments.")
	}
	validatedID, err := sample.ValidateID(id)
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error(), "sample list", "Copy an opaque sample ID unchanged.")
	}
	item, err := c.samples.Read(ctx, intent, validatedID)
	if err != nil {
		return c.fail(ctx, err)
	}
	if err := validateSampleItem(item); err != nil {
		return c.fail(ctx, err)
	}
	output, err := renderSampleRead(item, format)
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func validateSampleSummaries(items []sample.Summary) error {
	if len(items) > maxSampleOutputItems {
		return outputContractExceeded("The sample result exceeds the declared item limit.", "sample list")
	}
	totalBytes := 0
	for _, item := range items {
		if len(item.Name) > maxSampleNameBytes {
			return outputContractExceeded("A sample field exceeds the declared byte limit.", "sample list")
		}
		totalBytes += len(item.ID) + len(item.Name)
		if totalBytes > maxSampleListBytes {
			return outputContractExceeded("The sample list exceeds the declared total byte limit.", "sample list")
		}
	}
	return nil
}

func validateSampleItem(item sample.Item) error {
	if len(item.Name) > maxSampleNameBytes || len(item.Content) > maxSampleContentBytes {
		return outputContractExceeded("A sample field exceeds the declared byte limit.", "sample read")
	}
	return nil
}

type sampleListJSONDocument struct {
	SchemaVersion int                  `json:"schema_version"`
	Items         []sampleListJSONItem `json:"items"`
}

type sampleReadJSONDocument struct {
	SchemaVersion int                `json:"schema_version"`
	Item          sampleReadJSONItem `json:"item"`
}

type sampleListJSONItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type sampleReadJSONItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

func renderSampleList(items []sample.Summary, format successFormat) ([]byte, error) {
	if format == successFormatJSON {
		document := sampleListJSONDocument{SchemaVersion: 1, Items: make([]sampleListJSONItem, 0, len(items))}
		for _, item := range items {
			document.Items = append(document.Items, sampleListJSONItem{ID: item.ID, Name: safeExternalText(item.Name)})
		}
		output, err := json.Marshal(document)
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "The sample list JSON could not be encoded.", false, err)
		}
		return append(output, '\n'), nil
	}
	var output bytes.Buffer
	fmt.Fprintln(&output, "id\tname")
	for _, item := range items {
		fmt.Fprintf(&output, "%s\t%s\n", item.ID, escapeTSVCell(item.Name))
	}
	return output.Bytes(), nil
}

func renderSampleRead(item sample.Item, format successFormat) ([]byte, error) {
	if format == successFormatJSON {
		output, err := json.Marshal(sampleReadJSONDocument{
			SchemaVersion: 1,
			Item: sampleReadJSONItem{
				ID: item.ID, Name: safeExternalText(item.Name), Content: safeExternalText(item.Content),
			},
		})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "The sample JSON could not be encoded.", false, err)
		}
		return append(output, '\n'), nil
	}
	var output bytes.Buffer
	fmt.Fprintln(&output, "id\tname\tcontent")
	fmt.Fprintf(&output, "%s\t%s\t%s\n", item.ID, escapeTSVCell(item.Name), escapeTSVCell(item.Content))
	return output.Bytes(), nil
}
