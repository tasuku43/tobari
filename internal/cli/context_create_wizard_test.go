package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestContextCreateWizardCollectsNameFilesystemAndEveryMethodDecision(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: nil, style: false}
	// name, filesystem=read-write, default=exact-review, GET=allow, then
	// inherit the exact-review default for the remaining eight standard methods.
	input := "coding\n1\ne\na\n" + strings.Repeat("\n", len(contextCreateHTTPMethods)-1)
	var output bytes.Buffer
	selection, err := wizard.Compose(context.Background(), strings.NewReader(input), &output)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != "coding" || selection.SourceAccess != tobari.ContextSourceAccessReadWrite ||
		selection.MethodPolicy.Default != tobari.PolicyPresetMethodExactReview ||
		len(selection.MethodPolicy.Overrides) != 1 || selection.MethodPolicy.Overrides[0] != (tobari.PolicyPresetMethodOverride{Method: "GET", Decision: tobari.PolicyPresetMethodAllow}) {
		t.Fatalf("wizard selection = %+v", selection)
	}
	for _, required := range []string{"Context name:", "Project source access", "Other methods (default)", "GET", "TRACE"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("wizard output lacks %q: %q", required, output.String())
		}
	}
}

func TestContextCreateWizardRawUsesOneFilesystemScreenAndOneMethodMatrix(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\n\r\x1b[Bap"), &output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if mode.entered != 2 || mode.restored != 2 {
		t.Fatalf("raw mode entered/restored = %d/%d", mode.entered, mode.restored)
	}
	if selection.SourceAccess != tobari.ContextSourceAccessReadWrite || len(selection.MethodPolicy.Overrides) != 1 ||
		selection.MethodPolicy.Overrides[0].Method != "GET" || selection.MethodPolicy.Overrides[0].Decision != tobari.PolicyPresetMethodAllow {
		t.Fatalf("raw wizard selection = %+v", selection)
	}
	for _, required := range []string{"Tobari · Create Context", "Tobari · Network method policy", "Other methods (default)", "p Create"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("raw wizard output lacks %q: %q", required, output.String())
		}
	}
}

func TestContextCreateWizardRejectsInvalidNameBeforeCompositionAndCancelsWithoutSelection(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: nil, style: false}
	input := "../outside\nsafe\nq\n"
	var output bytes.Buffer
	_, err := wizard.Compose(context.Background(), strings.NewReader(input), &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if !strings.Contains(output.String(), "Use a valid portable Context name.") {
		t.Fatalf("invalid name was not explained: %q", output.String())
	}
}

func TestArgumentFreeContextCreateIsTheOnlyWizardMode(t *testing.T) {
	t.Parallel()
	empty := ParsedInputs{provided: map[string]bool{}}
	if !contextCreateInputsOmitted(empty) {
		t.Fatal("argument-free create did not select wizard mode")
	}
	for _, name := range []string{"--name", "--image", "--mode", "--source-access", "--policy-preset", "--native-readiness", "--format"} {
		inputs := ParsedInputs{provided: map[string]bool{name: true}}
		if contextCreateInputsOmitted(inputs) {
			t.Errorf("explicit %s unexpectedly selected wizard mode", name)
		}
	}
}
