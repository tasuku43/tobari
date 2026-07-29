package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func pagedDiscoverSpec(path, itemKind, cursorKind string) CommandSpec {
	spec := discoverSpec(path, itemKind)
	spec.Args = "[--cursor <cursor>]"
	spec.Agent.Inputs = append(spec.Agent.Inputs, CommandInput{
		Name: "--cursor", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description: "Opaque cursor returned by the preceding page.", AllowedValues: []string{}, ReferenceKind: cursorKind,
	})
	spec.Agent.Output.Formats = []OutputFormat{OutputFormatJSON}
	spec.Agent.Output.DefaultFormat = OutputFormatJSON
	spec.Agent.Output.Delivery = OutputDeliveryPaged
	spec.Agent.Pagination = &PaginationContract{
		CursorInput: "--cursor",
		CursorOutput: OutputField{
			Name: "next_cursor", Type: OutputFieldTypeString,
			Description: "Opaque cursor for the next page; an empty string means traversal is complete.", ReferenceKind: cursorKind,
		},
		Completion: PaginationCompletionEmptyCursor,
	}
	return spec
}

func TestPagedOutputRequiresExactOpaqueCursorBinding(t *testing.T) {
	paged := pagedDiscoverSpec("items list", "item", "item-page")
	act := actSpec("items read", "item", "--id")
	catalog := NewCatalog(paged, act)
	if err := catalog.Validate(); err != nil {
		t.Fatalf("valid paged catalog: %v", err)
	}
	workflows := catalog.referenceWorkflows()
	foundCursorLoop := false
	for _, workflow := range workflows {
		if workflow.ReferenceKind != "item-page" {
			continue
		}
		for _, producer := range workflow.Producers {
			for _, consumer := range workflow.Consumers {
				if producer.Path == paged.Path && producer.Field == "next_cursor" &&
					consumer.Path == paged.Path && consumer.Input == "--cursor" {
					foundCursorLoop = true
				}
			}
		}
	}
	if !foundCursorLoop {
		t.Fatalf("reference workflows lack exact cursor continuation: %+v", workflows)
	}
	if len(paged.Agent.Output.Fields) != 2 {
		t.Fatalf("paged item fields include top-level cursor metadata: %+v", paged.Agent.Output.Fields)
	}
	if got := paged.ProducedRefs(); len(got) != 2 ||
		got[0] != (ProducedRef{Kind: "item", Field: "id"}) ||
		got[1] != (ProducedRef{Kind: "item-page", Field: "next_cursor"}) {
		t.Fatalf("paged produced refs = %+v", got)
	}

	encoded, err := (&CLI{catalog: catalog}).renderAgentHelp(paged.Path, true, []CommandSpec{paged})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"pagination":{"cursor_input":"--cursor","cursor_output":{"name":"next_cursor","type":"string","description":"Opaque cursor for the next page; an empty string means traversal is complete.","reference_kind":"item-page"},"completion":"empty_cursor"}`)) {
		t.Fatalf("agent help projection lacks pagination binding: %s", encoded)
	}
	var help agentDocument
	if err := json.Unmarshal(encoded, &help); err != nil {
		t.Fatal(err)
	}
	if len(help.Commands) != 1 || help.Commands[0].Contract.Pagination == nil ||
		help.Commands[0].Contract.Pagination.CursorInput != "--cursor" ||
		help.Commands[0].Contract.Pagination.CursorOutput.Name != "next_cursor" ||
		help.Commands[0].Contract.Pagination.Completion != PaginationCompletionEmptyCursor {
		t.Fatalf("scoped agent help pagination = %+v", help.Commands)
	}
	complete, err := json.Marshal(utilitySpec("complete").Agent)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(complete, []byte(`"pagination"`)) {
		t.Fatalf("complete contract projected a pagination binding: %s", complete)
	}
}

func TestDefaultSampleOutputRemainsCompleteAndUnpaged(t *testing.T) {
	sampleList, found := DefaultCatalog().Lookup("sample list")
	if !found {
		t.Fatal("default catalog lacks sample list")
	}
	if sampleList.Agent.Output.Delivery != OutputDeliveryComplete ||
		sampleList.Agent.Output.CollectionCoverage != CollectionCoverageExhaustive ||
		sampleList.Agent.Pagination != nil {
		t.Fatalf("sample list output contract = %+v, pagination = %+v", sampleList.Agent.Output, sampleList.Agent.Pagination)
	}
}

func TestOutputDeliveryAndCollectionCoverageAreIndependent(t *testing.T) {
	for _, coverage := range []CollectionCoverage{
		CollectionCoverageExhaustive,
		CollectionCoverageBoundedWindow,
		CollectionCoverageDifferentialWindow,
	} {
		t.Run("complete_"+string(coverage), func(t *testing.T) {
			discover := discoverSpec("items list", "item")
			discover.Agent.Output.CollectionCoverage = coverage
			act := actSpec("items read", "item", "--id")
			if err := NewCatalog(discover, act).Validate(); err != nil {
				t.Fatalf("complete delivery with %s coverage: %v", coverage, err)
			}
		})

		t.Run("paged_"+string(coverage), func(t *testing.T) {
			discover := pagedDiscoverSpec("items list", "item", "item-page")
			discover.Agent.Output.CollectionCoverage = coverage
			act := actSpec("items read", "item", "--id")
			if err := NewCatalog(discover, act).Validate(); err != nil {
				t.Fatalf("paged delivery with %s coverage: %v", coverage, err)
			}
		})
	}
}

func TestPaginationContractFailsClosed(t *testing.T) {
	tests := map[string]struct {
		mutate func(*CommandSpec)
		want   string
	}{
		"paged without binding": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination = nil },
			want:   "paged output must declare",
		},
		"complete with binding": {
			mutate: func(spec *CommandSpec) { spec.Agent.Output.Delivery = OutputDeliveryComplete },
			want:   "complete output must not declare",
		},
		"paged without collection coverage": {
			mutate: func(spec *CommandSpec) {
				spec.Agent.Output.CollectionCoverage = CollectionCoverageNotApplicable
			},
			want: "paged output requires collection coverage",
		},
		"non JSON output": {
			mutate: func(spec *CommandSpec) {
				spec.Agent.Output.Formats = []OutputFormat{OutputFormatJSON, OutputFormatTSV}
			},
			want: "must support only JSON",
		},
		"non JSON default": {
			mutate: func(spec *CommandSpec) {
				spec.Agent.Output.Formats = []OutputFormat{OutputFormatJSON, OutputFormatTSV}
				spec.Agent.Output.DefaultFormat = OutputFormatTSV
			},
			want: "must support only JSON",
		},
		"missing input": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination.CursorInput = "--missing" },
			want:   "not a structured input",
		},
		"required input": {
			mutate: func(spec *CommandSpec) {
				spec.Args = "--cursor <cursor>"
				spec.Agent.Inputs[0].Required = true
			},
			want: "must be optional",
		},
		"non CLI input": {
			mutate: func(spec *CommandSpec) {
				spec.Args = ""
				spec.Agent.Inputs[0].Name = "cursor"
				spec.Agent.Inputs[0].Source = InputSourceConfiguration
				spec.Agent.Pagination.CursorInput = "cursor"
			},
			want: "must be a command argument or flag",
		},
		"non opaque input": {
			mutate: func(spec *CommandSpec) { spec.Agent.Inputs[0].ReferenceKind = "" },
			want:   "must consume an opaque reference",
		},
		"missing output name": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination.CursorOutput.Name = "" },
			want:   "output field name is empty",
		},
		"invalid output name": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination.CursorOutput.Name = "next.cursor" },
			want:   "output field name is invalid",
		},
		"schema version collision": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination.CursorOutput.Name = "schema_version" },
			want:   "collides with top-level JSON metadata",
		},
		"envelope collision": {
			mutate: func(spec *CommandSpec) {
				spec.Agent.Pagination.CursorOutput.Name = spec.Agent.Output.JSONEnvelope
			},
			want: "collides with top-level JSON metadata",
		},
		"non string output": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination.CursorOutput.Type = OutputFieldTypeObject },
			want:   "must have string type",
		},
		"missing output description": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination.CursorOutput.Description = "" },
			want:   "pagination cursor output description",
		},
		"non opaque output": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination.CursorOutput.ReferenceKind = "" },
			want:   "reference name is empty",
		},
		"invalid output kind": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination.CursorOutput.ReferenceKind = "page_cursor" },
			want:   "reference name is invalid",
		},
		"kind mismatch": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination.CursorOutput.ReferenceKind = "other-page" },
			want:   "same reference kind",
		},
		"missing completion": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination.Completion = PaginationCompletionUnknown },
			want:   "pagination completion is missing or invalid",
		},
		"unknown completion": {
			mutate: func(spec *CommandSpec) { spec.Agent.Pagination.Completion = PaginationCompletion("missing_cursor") },
			want:   "pagination completion is missing or invalid",
		},
		"extra input": {
			mutate: func(spec *CommandSpec) {
				spec.Args += " [--resume <cursor>]"
				spec.Agent.Inputs = append(spec.Agent.Inputs, CommandInput{
					Name: "--resume", Source: InputSourceFlag, Required: false,
					ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
					Description: "Ambiguous second cursor.", AllowedValues: []string{}, ReferenceKind: "item-page",
				})
			},
			want: "extra cursor input",
		},
		"extra output": {
			mutate: func(spec *CommandSpec) {
				spec.Agent.Output.Fields = append(spec.Agent.Output.Fields, OutputField{
					Name: "resume", Type: OutputFieldTypeString,
					Description: "Ambiguous second cursor.", ReferenceKind: "item-page",
				})
			},
			want: "extra cursor output",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			spec := pagedDiscoverSpec("items list", "item", "item-page")
			test.mutate(&spec)
			err := validateAgentContract(spec)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateAgentContract() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPaginationCursorKindIsDedicatedToOneCommand(t *testing.T) {
	tests := map[string]CommandSpec{
		"other producer":      discoverSpec("items related", "item-page"),
		"other consumer":      actSpec("items resume", "item-page", "--cursor"),
		"other paged command": pagedDiscoverSpec("items other", "other-item", "item-page"),
	}
	for name, extra := range tests {
		t.Run(name, func(t *testing.T) {
			paged := pagedDiscoverSpec("items list", "item", "item-page")
			itemConsumer := actSpec("items read", "item", "--id")
			err := NewCatalog(paged, itemConsumer, extra).Validate()
			if err == nil || !strings.Contains(err.Error(), "must be dedicated") {
				t.Fatalf("Validate() error = %v, want dedicated pagination kind rejection", err)
			}
		})
	}
}

func TestPaginationContractDeepCopies(t *testing.T) {
	spec := pagedDiscoverSpec("items list", "item", "item-page")
	clone := cloneCommandSpec(spec)
	clone.Agent.Pagination.CursorInput = "--changed"
	clone.Agent.Pagination.CursorOutput.Name = "changed"
	clone.Agent.Pagination.CursorOutput.Description = "Changed."
	clone.Agent.Pagination.Completion = PaginationCompletionUnknown
	if spec.Agent.Pagination.CursorInput != "--cursor" ||
		spec.Agent.Pagination.CursorOutput.Name != "next_cursor" ||
		spec.Agent.Pagination.CursorOutput.Description != "Opaque cursor for the next page; an empty string means traversal is complete." ||
		spec.Agent.Pagination.Completion != PaginationCompletionEmptyCursor {
		t.Fatalf("pagination binding shares storage: %+v", spec.Agent.Pagination)
	}
}
