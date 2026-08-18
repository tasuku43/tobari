package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestContextCreateWizardCollectsNameFilesystemAndEveryMethodDecision(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: nil, style: false}
	// name, filesystem=read-write, default=exact-review, GET=allow, then
	// inherit the exact-review default for the remaining eight standard methods.
	input := "coding\n1\ne\na\n" + strings.Repeat("\n", len(contextCreateHTTPMethods)-1) + "1\ny\n"
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
	for _, required := range []string{"Context name:", "Project source access", "Other methods (default)", "GET", "TRACE", "Workspace bootstrap", "Review & Create"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("wizard output lacks %q: %q", required, output.String())
		}
	}
}

func TestContextCreateWizardCanSelectAWSBootstrapProfile(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: nil, style: false}
	input := "coding\n1\ne\n" + strings.Repeat("\n", len(contextCreateHTTPMethods)) + "2\nengineering\ny\n"
	selection, err := wizard.Compose(context.Background(), strings.NewReader(input), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if selection.AWSBootstrapProfile != "engineering" {
		t.Fatalf("AWS bootstrap profile = %q", selection.AWSBootstrapProfile)
	}
}

func TestContextCreateWizardCanComposeAWSAndEKSBootstrap(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: nil, style: false}
	input := "coding\n1\ne\n" + strings.Repeat("\n", len(contextCreateHTTPMethods)) + "3\nengineering\nplatform\ny\n"
	selection, err := wizard.Compose(context.Background(), strings.NewReader(input), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if selection.AWSBootstrapProfile != "engineering" || selection.EKSBootstrapContext != "platform" {
		t.Fatalf("composed bootstrap = %+v", selection)
	}
}

func TestContextCreateWizardFallsBackToBoundedLineModeWhenRawModeIsUnavailable(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{enterErr: errors.New("raw mode unavailable")}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	input := "coding\n1\ne\n" + strings.Repeat("\n", len(contextCreateHTTPMethods)) + "1\ny\n"
	var output bytes.Buffer
	selection, err := wizard.Compose(context.Background(), strings.NewReader(input), &output)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != "coding" || mode.entered != 1 || mode.restored != 0 {
		t.Fatalf("line fallback selection/mode = %+v / %d/%d", selection, mode.entered, mode.restored)
	}
	if strings.Contains(output.String(), "\x1b[") || !strings.Contains(output.String(), "Context name:") {
		t.Fatalf("line fallback output = %q", output.String())
	}
}

func TestContextCreateWizardLineReviewCanCancelTheCompleteSelection(t *testing.T) {
	t.Parallel()
	wizard := &terminalContextCreateWizard{mode: nil, style: false}
	input := "coding\n1\ne\n" + strings.Repeat("\n", len(contextCreateHTTPMethods)) + "1\nn\n"
	var output bytes.Buffer
	_, err := wizard.Compose(context.Background(), strings.NewReader(input), &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if !strings.Contains(output.String(), "Review & Create") || !strings.Contains(output.String(), "Create this Context? [y/N]") {
		t.Fatalf("line review was not shown before cancellation: %q", output.String())
	}
}

func TestContextCreateWizardRawUsesOneContinuousFiveStepSession(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\x1b[Bap\r\r"), &output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if mode.entered != 1 || mode.restored != 1 {
		t.Fatalf("raw mode entered/restored = %d/%d", mode.entered, mode.restored)
	}
	if selection.SourceAccess != tobari.ContextSourceAccessReadWrite || len(selection.MethodPolicy.Overrides) != 1 ||
		selection.MethodPolicy.Overrides[0].Method != "GET" || selection.MethodPolicy.Overrides[0].Decision != tobari.PolicyPresetMethodAllow {
		t.Fatalf("raw wizard selection = %+v", selection)
	}
	for _, required := range []string{
		"1 of 5 · Name", "2 of 5 · Filesystem", "3 of 5 · Network",
		"4 of 5 · Workspace bootstrap", "5 of 5 · Review & Create",
		"Context name:", "Other methods (default)", "Enter Create",
	} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("raw wizard output lacks %q: %q", required, output.String())
		}
	}
	if strings.Count(output.String(), selectorAlternateScreenEnter) != 1 ||
		strings.Count(output.String(), selectorAlternateScreenExit) != 1 {
		t.Fatalf("alternate screen entry/exit count is not one: %q", output.String())
	}
	if strings.Index(output.String(), selectorAlternateScreenEnter) > strings.Index(output.String(), "Context name:") {
		t.Fatalf("name prompt appeared before the full-screen session: %q", output.String())
	}
}

func TestContextCreateWizardRawBackNavigationPreservesStagedFilesystem(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\x1b[B\rb\r\r\r\r"), io.Discard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.SourceAccess != tobari.ContextSourceAccessReadOnly {
		t.Fatalf("source access after back navigation = %q", selection.SourceAccess)
	}
	if mode.entered != 1 || mode.restored != 1 {
		t.Fatalf("raw mode entered/restored = %d/%d", mode.entered, mode.restored)
	}
}

func TestContextCreateWizardRawCollectsAWSProfileWithoutLeavingSession(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\x1b[B\rengineering\r\r"), &output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.AWSBootstrapProfile != "engineering" {
		t.Fatalf("AWS profile = %q", selection.AWSBootstrapProfile)
	}
	if !strings.Contains(output.String(), "AWS profile:") ||
		strings.Count(output.String(), selectorAlternateScreenEnter) != 1 ||
		strings.Count(output.String(), selectorAlternateScreenExit) != 1 {
		t.Fatalf("AWS profile was not collected inside one full-screen session: %q", output.String())
	}
}

func TestContextCreateWizardRawCollectsAWSAndEKSInsideOneSession(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	selection, err := wizard.Compose(
		context.Background(), strings.NewReader("coding\r\r\r\x1b[B\x1b[B\rengineering\rplatform\r\r"), &output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if selection.AWSBootstrapProfile != "engineering" || selection.EKSBootstrapContext != "platform" {
		t.Fatalf("composed bootstrap = %+v", selection)
	}
	if !strings.Contains(output.String(), "Kubernetes context:") || strings.Count(output.String(), selectorAlternateScreenEnter) != 1 || strings.Count(output.String(), selectorAlternateScreenExit) != 1 {
		t.Fatalf("EKS bootstrap left the continuous session: %q", output.String())
	}
}

func TestContextCreateWizardRawCancelRestoresTerminalBeforeSelection(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	_, err := wizard.Compose(context.Background(), strings.NewReader("coding\rq"), &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if mode.entered != 1 || mode.restored != 1 ||
		strings.Count(output.String(), selectorAlternateScreenEnter) != 1 ||
		strings.Count(output.String(), selectorAlternateScreenExit) != 1 {
		t.Fatalf("terminal cleanup entered/restored = %d/%d, output %q", mode.entered, mode.restored, output.String())
	}
}

func TestContextCreateWizardRawKeepsInvalidNameInsideFirstStep(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	_, err := wizard.Compose(context.Background(), strings.NewReader("INVALID\r\x03"), &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if !strings.Contains(output.String(), "name must match [a-z][a-z0-9-]{0,62}") ||
		strings.Contains(output.String(), "2 of 5 · Filesystem") {
		t.Fatalf("invalid name escaped the first step: %q", output.String())
	}
}

type oneTimeoutReader struct {
	remaining io.Reader
	timedOut  bool
}

func (r *oneTimeoutReader) Read(value []byte) (int, error) {
	if !r.timedOut {
		r.timedOut = true
		return 0, nil
	}
	return r.remaining.Read(value)
}

func TestContextCreateWizardRawDoesNotRedrawAnUnchangedTextStepOnPollingTimeout(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	wizard := &terminalContextCreateWizard{mode: mode, style: false}
	var output bytes.Buffer
	input := &oneTimeoutReader{remaining: strings.NewReader("\x03")}
	_, err := wizard.Compose(context.Background(), input, &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wizard error = %v, want canceled", err)
	}
	if count := strings.Count(output.String(), "Context name:"); count != 1 {
		t.Fatalf("unchanged name step rendered %d times after one timeout: %q", count, output.String())
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
	for _, name := range []string{"--name", "--image", "--mode", "--source-access", "--policy-preset", "--native-readiness", "--bootstrap-aws-profile", "--format"} {
		inputs := ParsedInputs{provided: map[string]bool{name: true}}
		if contextCreateInputsOmitted(inputs) {
			t.Errorf("explicit %s unexpectedly selected wizard mode", name)
		}
	}
}
