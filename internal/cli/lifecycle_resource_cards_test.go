package cli

import (
	"strings"
	"testing"
)

func TestFinalLifecycleListsLeadWithHumanResourceAndKeepReferenceSecondary(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		heading   string
		resource  string
		reference string
		facts     []string
	}{
		{
			name: "templates", heading: "· Templates · 1", resource: "standard", reference: "wtpl1_01912345-6789-7abc-8def-0123456789ab",
			output: string(finalTemplateListText([]finalTemplateProjection{{Lifecycle: "active", TemplateRef: "wtpl1_01912345-6789-7abc-8def-0123456789ab", Name: "standard", Generation: 2, SourceAccess: "read-write"}}, false)),
			facts:  []string{"✓ active · generation 2", "Source access", "read-write"},
		},
		{
			name: "contexts", heading: "· Contexts · 1", resource: "standard", reference: "ctx1_01912345-6789-7abc-8def-0123456789ac",
			output: string(finalContextListText([]finalContextProjection{{Lifecycle: "active", ContextRef: "ctx1_01912345-6789-7abc-8def-0123456789ac", TemplateName: "standard"}}, false)),
			facts:  []string{"✓ active", "Template", "standard"},
		},
		{
			name: "workspaces", heading: "· Workspaces · 1", resource: "/workspace/example", reference: "wsp1_01912345-6789-7abc-8def-0123456789ad",
			output: string(finalWorkspaceListText([]finalWorkspaceProjection{{WorkspaceRef: "wsp1_01912345-6789-7abc-8def-0123456789ad", TemplateName: "standard", ProjectRoot: "/workspace/example", Applied: false}}, false)),
			facts:  []string{"✓ exists", "Applied entry", "absent"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !strings.HasPrefix(test.output, test.heading+"\n") {
				t.Fatalf("heading = %q", test.output)
			}
			resourceAt, referenceAt := strings.Index(test.output, test.resource), strings.Index(test.output, test.reference)
			if resourceAt < 0 || referenceAt < 0 || resourceAt >= referenceAt {
				t.Fatalf("resource/reference hierarchy = %q", test.output)
			}
			for _, fact := range test.facts {
				if !strings.Contains(test.output, fact) {
					t.Errorf("output lacks %q: %q", fact, test.output)
				}
			}
		})
	}
}

func TestFinalLifecycleListsRenderExplicitEmptyScope(t *testing.T) {
	for name, output := range map[string]string{
		"templates":  string(finalTemplateListText(nil, false)),
		"contexts":   string(finalContextListText(nil, false)),
		"workspaces": string(finalWorkspaceListText(nil, false)),
	} {
		if got := strings.TrimSpace(output); got != "· "+strings.ToUpper(name[:1])+name[1:]+" · 0" {
			t.Errorf("%s empty scope = %q", name, output)
		}
	}
}
