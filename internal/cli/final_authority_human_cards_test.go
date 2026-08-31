package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestFinalAuthorityDetailCardsUseStructuredHumanVocabulary(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		heading       string
		section       string
		rows          map[string]string
		reference     string
		oldFlatStarts []string
	}{
		{
			name: "Template",
			output: string(finalTemplateText(finalTemplateProjection{
				Lifecycle:          "active",
				TemplateRef:        "wtpl1_01912345-6789-7abc-8def-0123456789ab",
				CurrentRevisionRef: "wtrev1_01912345-6789-7abc-8def-0123456789ac",
				Name:               "standard",
				Generation:         3,
				SourceAccess:       "read-write",
			}, true, false)),
			heading: "· Template details",
			section: "standard",
			rows: map[string]string{
				"Status":        "✓ active · generation 3",
				"Source access": "read-write",
				"GraphQL":       "none",
				"Revision":      "wtrev1_01912345-6789-7abc-8def-0123456789ac",
			},
			reference:     "wtpl1_01912345-6789-7abc-8def-0123456789ab",
			oldFlatStarts: []string{"Template ", "Reference ", "Revision ", "Source access ", "GraphQL endpoints "},
		},
		{
			name: "Context",
			output: string(finalContextText(finalContextProjection{
				Lifecycle:                        "active",
				ContextRef:                       "ctx1_01912345-6789-7abc-8def-0123456789ad",
				TemplateName:                     "standard",
				DesiredTemplateGeneration:        3,
				DesiredTemplateRevision:          "sha256:desired",
				DesiredTemplatePolicySliceDigest: "sha256:policy",
				CurrentPolicyMemoryRevision:      "sha256:memory",
			}, false)),
			heading: "· Context details",
			section: "standard",
			rows: map[string]string{
				"Status":                      "✓ active",
				"Desired Template generation": "3",
				"Active Template policy":      "absent",
				"Active Policy Memory":        "absent",
				"Applied entry":               "absent",
			},
			reference: "ctx1_01912345-6789-7abc-8def-0123456789ad",
			oldFlatStarts: []string{
				"Context ", "Template ", "Project ", "Desired Template generation ",
				"Desired Template revision ", "Desired Template policy ", "Active Template policy ",
				"Current Policy Memory ", "Active Policy Memory ", "Applied entry ",
			},
		},
		{
			name: "Workspace",
			output: string(finalWorkspaceStatusText(finalWorkspaceProjection{
				WorkspaceRef:  "wsp1_01912345-6789-7abc-8def-0123456789ae",
				TemplateName:  "standard",
				ProjectRoot:   "/workspace/example",
				WorkspaceHome: "/workspace/example-home",
			}, false)),
			heading: "· Workspace details",
			section: "/workspace/example",
			rows: map[string]string{
				"Status":        "✓ exists",
				"Template":      "standard",
				"Applied entry": "absent",
				"Home":          "/workspace/example-home",
			},
			reference:     "wsp1_01912345-6789-7abc-8def-0123456789ae",
			oldFlatStarts: []string{"Workspace ", "Project ", "Reference "},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !strings.HasPrefix(test.output, test.heading+"\n\n"+test.section+"\n") {
				t.Fatalf("card hierarchy = %q", test.output)
			}
			for label, value := range test.rows {
				assertFinalAuthorityIndentedRow(t, test.output, label, value)
			}
			if !strings.Contains(test.output, "\nDetails\n") {
				t.Fatalf("card lacks Details section: %q", test.output)
			}
			assertFinalAuthorityIndentedRow(t, test.output, "Reference", test.reference)
			assertFinalAuthorityNoFlatStarts(t, test.output, test.oldFlatStarts...)
		})
	}
}

func TestFinalAuthorityDraftCardsPublishExactNextAndDetails(t *testing.T) {
	tests := []struct {
		name          string
		output        string
		heading       string
		section       string
		next          string
		reference     string
		oldFlatStarts []string
	}{
		{
			name: "Template",
			output: string(finalTemplateDraftText(finalTemplateDraftProjection{
				TemplateRef:  "wtpl1_01912345-6789-7abc-8def-0123456789ab",
				TemplateID:   "01912345-6789-7abc-8def-0123456789ab",
				Name:         "standard",
				SourcePath:   "/authority/templates/standard",
				SourceAccess: "read-write",
			}, "create", false)),
			heading:       "✓ Template source created",
			section:       "standard",
			next:          invocationForPath("template plan") + " --id wtpl1_01912345-6789-7abc-8def-0123456789ab — Review and activate this Template source.",
			reference:     "wtpl1_01912345-6789-7abc-8def-0123456789ab",
			oldFlatStarts: []string{"Template draft ", "Reference ", "Source ", "Next "},
		},
		{
			name: "Context",
			output: string(finalContextDraftText(finalContextDraftProjection{
				ContextRef: "ctx1_01912345-6789-7abc-8def-0123456789ad",
				ContextID:  "01912345-6789-7abc-8def-0123456789ad",
				SourcePath: "/authority/contexts/draft",
			}, "wtpl1_01912345-6789-7abc-8def-0123456789ab", false)),
			heading:       "✓ Context source created",
			section:       "Context draft",
			next:          invocationForPath("context plan") + " --id ctx1_01912345-6789-7abc-8def-0123456789ad — Review and activate this Context source.",
			reference:     "ctx1_01912345-6789-7abc-8def-0123456789ad",
			oldFlatStarts: []string{"Context draft ", "Project ", "Reference ", "Source ", "Next "},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !strings.HasPrefix(test.output, test.heading+"\n\n"+test.section+"\n") {
				t.Fatalf("card hierarchy = %q", test.output)
			}
			assertFinalAuthorityIndentedRow(t, test.output, "Next", test.next)
			if !strings.Contains(test.output, "\nDetails\n") {
				t.Fatalf("card lacks Details section: %q", test.output)
			}
			assertFinalAuthorityIndentedRow(t, test.output, "Reference", test.reference)
			assertFinalAuthorityNoFlatStarts(t, test.output, test.oldFlatStarts...)
		})
	}
}

func TestFinalContextListDraftUsesNonemptyHumanSectionTitle(t *testing.T) {
	const reference = "ctx1_01912345-6789-7abc-8def-0123456789ad"
	output := string(finalContextListText([]finalContextProjection{{
		Lifecycle:   "draft",
		ContextRef:  reference,
		TemplateID:  "01912345-6789-7abc-8def-0123456789ab",
		SourcePath:  "/authority/contexts/draft",
		SourceState: "current",
	}}, false))

	if !strings.HasPrefix(output, "· Contexts · 1\n\nContext draft\n") {
		t.Fatalf("draft Context list hierarchy = %q", output)
	}
	assertFinalAuthorityIndentedRow(t, output, "Status", "! draft · source current")
	assertFinalAuthorityIndentedRow(t, output, "Reference", reference)
	assertFinalAuthorityNoFlatStarts(t, output, reference, "Reference ", "Source ")
}

func assertFinalAuthorityIndentedRow(t *testing.T, output, label, value string) {
	t.Helper()
	prefix := "  " + fmt.Sprintf("%-*s", humanOutputLabelWidth, label) + " "
	for _, line := range strings.Split(stripANSIStyles(output), "\n") {
		if strings.HasPrefix(line, prefix) && strings.TrimPrefix(line, prefix) == value {
			return
		}
	}
	t.Fatalf("output lacks indented %s row %q: %q", label, value, output)
}

func assertFinalAuthorityNoFlatStarts(t *testing.T, output string, starts ...string) {
	t.Helper()
	for _, line := range strings.Split(stripANSIStyles(output), "\n") {
		for _, start := range starts {
			if strings.HasPrefix(line, start) {
				t.Fatalf("output retains old flat line start %q: %q", start, output)
			}
		}
	}
}
