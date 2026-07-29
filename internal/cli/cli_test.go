package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/doctor"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
)

type cliInspector struct {
	report doctor.Report
	err    error
	calls  int
	ctx    context.Context
}

func (i *cliInspector) Inspect(ctx context.Context) (doctor.Report, error) {
	i.calls++
	i.ctx = ctx
	return i.report, i.err
}

func newTestCLI(inspector *cliInspector) (*CLI, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), inspector)
	return command, stdout, stderr
}

func passingInspector(detail string) *cliInspector {
	return &cliInspector{report: doctor.Report{Checks: []doctor.Check{
		{Name: "runtime", Status: doctor.CheckStatusPass, Detail: detail},
	}}}
}

func runCLI(command *CLI, args []string) int {
	return command.RunContext(context.Background(), args)
}

func TestExitCodesAreStable(t *testing.T) {
	want := []int{0, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	got := []int{ExitOK, ExitUsage, ExitInternal, ExitAuthentication, ExitPermission, ExitNotFound, ExitAmbiguous, ExitRateLimited, ExitUnavailable, ExitRejected, ExitCanceled, ExitUnsupported, ExitContract}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("exit code %d = %d, want %d", index, got[index], want[index])
		}
	}
}

func TestNoArgsReturnsStructuredUsageFailure(t *testing.T) {
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	if code := runCLI(command, nil); code != ExitUsage {
		t.Fatalf("Run(nil) code = %d, want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "kind: invalid_input") || !strings.Contains(stderr.String(), "code: missing_command") ||
		!strings.Contains(stderr.String(), "next_action: agentic-cli-foundry help") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestUnknownCommandUsesUsageExitCode(t *testing.T) {
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	if code := runCLI(command, []string{"missing"}); code != ExitUsage {
		t.Fatalf("Run(missing) code = %d, want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "code: unknown_command") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestVersionOutputContract(t *testing.T) {
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	command.Version = "v1.2.3"
	command.Commit = "0123456789abcdef0123456789abcdef01234567"
	if code := runCLI(command, []string{"version"}); code != ExitOK {
		t.Fatalf("Run(version) code = %d, stderr = %q", code, stderr.String())
	}
	want := "agentic-cli-foundry v1.2.3 (0123456789abcdef0123456789abcdef01234567)\n"
	if got := stdout.String(); got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestDoctorOutputContract(t *testing.T) {
	inspector := &cliInspector{report: doctor.Report{Checks: []doctor.Check{
		{Name: "runtime", Status: doctor.CheckStatusPass, Detail: "runtime-version\ttest/test\nlocal"},
		{Name: "configuration", Status: doctor.CheckStatusWarn, Detail: "path\\value\x1b"},
	}}}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor"}); code != ExitOK {
		t.Fatalf("Run(doctor) code = %d, stderr = %q", code, stderr.String())
	}
	want := "CHECK\tSTATUS\tDETAIL\n" +
		"runtime\tpass\truntime-version\\ttest/test\\nlocal\n" +
		"configuration\twarn\tpath\\\\value\\u001B\n"
	if got := stdout.String(); got != want {
		t.Fatalf("doctor output = %q, want %q", got, want)
	}
	if stderr.Len() != 0 || inspector.calls != 1 {
		t.Fatalf("stderr = %q, inspector calls = %d", stderr.String(), inspector.calls)
	}
}

func TestDoctorFailureUsesRejectedExitAndStructuredRecovery(t *testing.T) {
	inspector := &cliInspector{report: doctor.Report{Checks: []doctor.Check{
		{Name: "runtime", Status: doctor.CheckStatusFail, Detail: "unsupported"},
	}}}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor"}); code != ExitRejected {
		t.Fatalf("Run(doctor) code = %d, want %d", code, ExitRejected)
	}
	if !strings.Contains(stdout.String(), "runtime\tfail\tunsupported") || !strings.Contains(stderr.String(), "code: diagnostic_failed") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestRunContextPropagatesExactContext(t *testing.T) {
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("trace"), "value")
	inspector := passingInspector("unused")
	command, _, stderr := newTestCLI(inspector)
	if code := command.RunContext(ctx, []string{"doctor"}); code != ExitOK {
		t.Fatalf("RunContext() code = %d, stderr = %q", code, stderr.String())
	}
	if inspector.ctx == nil || inspector.ctx.Value(contextKey("trace")) != "value" {
		t.Fatalf("inspector context = %#v", inspector.ctx)
	}
}

func TestRunContextRejectsNilContextWithoutDownstreamCall(t *testing.T) {
	inspector := passingInspector("unused")
	command, stdout, stderr := newTestCLI(inspector)
	if code := command.RunContext(nil, []string{"doctor"}); code != ExitContract {
		t.Fatalf("RunContext(nil) code = %d, want %d", code, ExitContract)
	}
	if inspector.calls != 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "code: missing_context") {
		t.Fatalf("calls = %d, stdout = %q, stderr = %q", inspector.calls, stdout.String(), stderr.String())
	}
}

func TestCanceledContextStopsBeforeDownstreamCall(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inspector := passingInspector("unused")
	command, stdout, stderr := newTestCLI(inspector)
	if code := command.RunContext(ctx, []string{"doctor"}); code != ExitCanceled {
		t.Fatalf("RunContext() code = %d, stderr = %q", code, stderr.String())
	}
	if inspector.calls != 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "code: operation_canceled") {
		t.Fatalf("calls = %d, stdout = %q, stderr = %q", inspector.calls, stdout.String(), stderr.String())
	}
}

func TestCanceledContextHonorsGlobalJSONErrorFormat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	if code := command.RunContext(ctx, []string{"--error-format=json", "doctor"}); code != ExitCanceled {
		t.Fatalf("RunContext() code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 || !json.Valid(stderr.Bytes()) || !strings.Contains(stderr.String(), `"code":"operation_canceled"`) {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestEmitChecksCancellationImmediatelyBeforeStdout(t *testing.T) {
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	ctx, cancel := context.WithCancel(context.Background())
	ctx = withCommandPath(ctx, "version")
	cancel()
	if code := command.emitResult(ctx, []byte("must-not-be-written\n")); code != ExitCanceled {
		t.Fatalf("emit() code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "code: operation_canceled") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestEmitMutationResultPreservesConfirmedSuccessAfterCancellation(t *testing.T) {
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	command.catalog = NewCatalog(mutationOutputCommand())
	ctx, cancel := context.WithCancel(context.Background())
	ctx = withCommandPath(ctx, "items update")
	cancel()

	if code := command.emitResult(ctx, []byte("confirmed mutation result\n")); code != ExitOK {
		t.Fatalf("emitMutationResult() code = %d, stderr = %q", code, stderr.String())
	}
	if got, want := stdout.String(), "confirmed mutation result\n"; got != want || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, want %q; stderr = %q", got, want, stderr.String())
	}
}

func TestEmitMutationResultStillRequiresCompleteWrite(t *testing.T) {
	var stderr bytes.Buffer
	command := New(strings.NewReader(""), shortWriter{}, &stderr)
	command.catalog = NewCatalog(mutationOutputCommand())
	ctx, cancel := context.WithCancel(context.Background())
	ctx = withCommandPath(ctx, "items update")
	cancel()

	if code := command.emitResult(ctx, []byte("confirmed mutation result\n")); code != ExitInternal {
		t.Fatalf("emitMutationResult() code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "code: mutation_output_write_failed") ||
		!strings.Contains(stderr.String(), "retryable: false") ||
		!strings.Contains(stderr.String(), "next_action: "+ProgramName+" sample list") ||
		strings.Contains(stderr.String(), "code: operation_canceled") ||
		strings.Contains(stderr.String(), "next_action: "+ProgramName+" items update") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCatalogBoundMutationFinalizerCannotBeDowngradedByHandler(t *testing.T) {
	var cancelInvocation context.CancelFunc
	mutation := catalogBoundMutationCommand(func(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, _ ParsedInputs) int {
		// A handler-local copy is not authoritative. The finalizer resolves the
		// actual effect from the catalog-bound command path.
		command.Effect = operation.EffectRead
		cancelInvocation()
		return c.emitResult(ctx, []byte("confirmed mutation result\n"))
	})
	catalog := NewCatalog(discoverSpec("items list", "item"), mutation)
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}

	t.Run("late cancellation preserves confirmed output", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		command := newCLI(strings.NewReader(""), &stdout, &stderr, catalog, passingInspector("unused"))
		ctx, cancel := context.WithCancel(context.Background())
		cancelInvocation = cancel
		if code := command.RunContext(ctx, []string{"items", "update", "--id=-opaque-item"}); code != ExitOK {
			t.Fatalf("RunContext() code = %d, stderr = %q", code, stderr.String())
		}
		if stdout.String() != "confirmed mutation result\n" || stderr.Len() != 0 {
			t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
		}
	})

	t.Run("short write is normalized as non-retryable", func(t *testing.T) {
		var stderr bytes.Buffer
		command := newCLI(strings.NewReader(""), shortWriter{}, &stderr, catalog, passingInspector("unused"))
		ctx, cancel := context.WithCancel(context.Background())
		cancelInvocation = cancel
		if code := command.RunContext(ctx, []string{"items", "update", "--id=-opaque-item"}); code != ExitInternal {
			t.Fatalf("RunContext() code = %d, stderr = %q", code, stderr.String())
		}
		if !strings.Contains(stderr.String(), "code: mutation_output_write_failed") ||
			!strings.Contains(stderr.String(), "retryable: false") ||
			!strings.Contains(stderr.String(), "next_action: "+ProgramName+" items list") ||
			strings.Contains(stderr.String(), "code: undeclared_fault_contract") {
			t.Fatalf("stderr = %q", stderr.String())
		}
	})
}

func TestDispatchParsesCatalogInputsBeforeCallingHandler(t *testing.T) {
	called := 0
	spec := utilitySpec("probe")
	spec.Args = "--count <count>"
	spec.Agent.Inputs = []CommandInput{{
		Name: "--count", Source: InputSourceFlag, Required: true,
		ValueKind: InputValueInteger, Cardinality: InputCardinalitySingle,
		Description: "Bounded probe count.", AllowedValues: []string{},
	}}
	spec.handler = func(_ context.Context, _ *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
		called++
		value, present := inputs.Integer("--count")
		if !present || value != 2 || !inputs.Provided("--count") {
			t.Fatalf("handler inputs = value %d, present %t, provided %t", value, present, inputs.Provided("--count"))
		}
		return ExitOK
	}
	commands := DefaultCatalog().Commands()
	commands = append(commands, spec)
	command := newCLI(strings.NewReader(""), io.Discard, io.Discard, NewCatalog(commands...), passingInspector("unused"))

	if code := command.RunContext(context.Background(), []string{"probe", "--count", "invalid"}); code != ExitUsage {
		t.Fatalf("invalid dispatch code = %d", code)
	}
	if called != 0 {
		t.Fatalf("handler called %d times for invalid argv", called)
	}
	if code := command.RunContext(context.Background(), []string{"probe", "--count", "2"}); code != ExitOK {
		t.Fatalf("valid dispatch code = %d", code)
	}
	if called != 1 {
		t.Fatalf("handler called %d times after valid argv", called)
	}
}

func mutationOutputCommand() CommandSpec {
	return CommandSpec{
		Path:   "items update",
		Effect: operation.EffectWrite,
		Agent: AgentContract{Errors: []CommandError{
			declaredCommandError(
				fault.KindInternal,
				"mutation_output_write_failed",
				false,
				"sample list",
				"Reconcile the confirmed mutation result without repeating the mutation.",
			),
		}},
	}
}

func catalogBoundMutationCommand(handler commandHandler) CommandSpec {
	spec := actSpec("items update", "item", "--id")
	spec.Effect = operation.EffectWrite
	spec.Summary = "Update one selected item"
	spec.Agent.Outcome = "Update the selected item after policy approval"
	spec.Agent.Errors = mutationErrors(spec.Agent.Errors, spec.Path)
	spec.Agent.Mutation = &MutationContract{
		TargetKind: "item", TargetInputs: []string{"--id"}, TargetIDInput: "--id",
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
		},
	}
	spec.handler = handler
	return spec
}

func TestJSONErrorIsStableAndDoesNotExposePlainCause(t *testing.T) {
	headerName := "Authori" + "zation"
	scheme := "Bear" + "er"
	canary := "redaction" + "-canary"
	inspector := &cliInspector{err: errors.New(headerName + ": " + scheme + " " + canary + " https://redaction.invalid")}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"--error-format", "json", "doctor"}); code != ExitInternal {
		t.Fatalf("Run() code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 || strings.Contains(stderr.String(), canary) || strings.Contains(stderr.String(), "redaction.invalid") {
		t.Fatalf("stdout = %q, stderr leaked cause = %q", stdout.String(), stderr.String())
	}
	var document errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &document); err != nil {
		t.Fatalf("JSON error = %v, output = %q", err, stderr.String())
	}
	if document.SchemaVersion != 1 || document.Error.Kind != "internal" || document.Error.Code != "internal_error" ||
		document.Error.RetryAfter != nil || len(document.Error.NextActions) != 1 {
		t.Fatalf("error document = %+v", document)
	}
}

func TestRateLimitTimingPresentationDoesNotAuthorizeRetry(t *testing.T) {
	unknown := renderTextError(errorPayload{
		Kind:      fault.KindRateLimited,
		Code:      "provider_rate_limited",
		Message:   "The provider rate limit was reached.",
		Retryable: true,
	})
	if !strings.Contains(string(unknown), "retry_after: unknown") {
		t.Fatalf("rate-limit text timing = %q", unknown)
	}
	nonRateLimit := renderTextError(errorPayload{
		Kind:      fault.KindUnavailable,
		Code:      "provider_unavailable",
		Message:   "The provider is unavailable.",
		Retryable: true,
	})
	if !strings.Contains(string(nonRateLimit), "retry_after: none") {
		t.Fatalf("non-rate-limit text timing = %q", nonRateLimit)
	}

	mutation := catalogBoundMutationCommand(func(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
		rateLimited := fault.New(
			fault.KindRateLimited,
			"mutation_rate_limited",
			"The mutation was rate limited.",
			false,
		)
		rateLimited.RetryAfter = 10 * time.Second
		return c.fail(ctx, rateLimited)
	})
	mutation.Agent.Errors = append(mutation.Agent.Errors, declaredCommandError(
		fault.KindRateLimited,
		"mutation_rate_limited",
		false,
		"items list",
		"Wait for the provider window and reconcile before another mutation.",
	))
	mutationCatalog := NewCatalog(discoverSpec("items list", "item"), mutation)
	if err := mutationCatalog.Validate(); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, mutationCatalog, passingInspector("unused"))
	if code := command.RunContext(context.Background(), []string{"--error-format=json", "items", "update", "--id", "item-1"}); code != ExitRateLimited {
		t.Fatalf("RunContext() code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	var document errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &document); err != nil {
		t.Fatalf("JSON error = %v, output = %q", err, stderr.String())
	}
	if document.Error.Retryable || document.Error.RetryAfter == nil || *document.Error.RetryAfter != "10s" {
		t.Fatalf("non-retryable rate-limit error = %+v", document.Error)
	}

	read := utilitySpec("provider inspect")
	read.Agent.Errors = append(read.Agent.Errors, declaredCommandError(
		fault.KindRateLimited,
		"provider_rate_limited",
		true,
		read.Path,
		"Retry only when a new provider window is available.",
	))
	read.handler = func(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
		return c.fail(ctx, fault.New(
			fault.KindRateLimited,
			"provider_rate_limited",
			"The provider rate limit was reached.",
			true,
		))
	}
	readCatalog := NewCatalog(read)
	if err := readCatalog.Validate(); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	command = newCLI(strings.NewReader(""), &stdout, &stderr, readCatalog, passingInspector("unused"))
	if code := command.RunContext(context.Background(), []string{"--error-format=json", "provider", "inspect"}); code != ExitRateLimited {
		t.Fatalf("RunContext() unknown timing code = %d, stderr = %q", code, stderr.String())
	}
	var unknownDocument errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &unknownDocument); err != nil {
		t.Fatalf("unknown timing JSON error = %v, output = %q", err, stderr.String())
	}
	if unknownDocument.Error.Kind != fault.KindRateLimited ||
		!unknownDocument.Error.Retryable || unknownDocument.Error.RetryAfter != nil {
		t.Fatalf("unknown rate-limit error = %+v", unknownDocument.Error)
	}
	var rawDocument struct {
		Error map[string]json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(stderr.Bytes(), &rawDocument); err != nil {
		t.Fatalf("raw JSON error = %v, output = %q", err, stderr.String())
	}
	retryAfter, present := rawDocument.Error["retry_after"]
	if !present || string(retryAfter) != "null" {
		t.Fatalf("unknown rate-limit retry_after = present %t, value %s", present, retryAfter)
	}
}

func TestFaultNormalizationPreservesValidStructuredClassificationBeforeCancellation(t *testing.T) {
	const canary = "private-deadline-canary"
	ctx := withCommandPath(context.Background(), "sample read")
	providerFault := fault.Wrap(
		fault.KindUnavailable,
		"mutation_outcome_unknown",
		"The provider did not confirm the mutation outcome.",
		false,
		fmt.Errorf("%s: %w", canary, context.DeadlineExceeded),
	)
	got := normalizeUnboundFault(ctx, providerFault)
	if got.Kind != fault.KindUnavailable || got.Code != "mutation_outcome_unknown" || got.Retryable {
		t.Fatalf("normalized structured fault = %+v", got)
	}
	if errors.Unwrap(got) != nil || errors.Is(got, context.DeadlineExceeded) || strings.Contains(got.Error(), canary) {
		t.Fatalf("normalized structured fault retained private cause: %#v", got)
	}

	invalid := fault.Wrap(fault.KindUnavailable, "INVALID", "Invalid structured fault.", false, context.DeadlineExceeded)
	if got := normalizeUnboundFault(ctx, invalid); got.Kind != fault.KindContract || got.Code != "invalid_fault_contract" {
		t.Fatalf("invalid structured fault normalized as %+v", got)
	}

	if got := normalizeUnboundFault(ctx, context.DeadlineExceeded); got.Kind != fault.KindCanceled ||
		got.Code != "operation_canceled" || !got.Retryable {
		t.Fatalf("unstructured deadline normalized as %+v", got)
	}
}

func TestRuntimeFaultMustMatchCatalogAndUsesCatalogRecovery(t *testing.T) {
	tests := []struct {
		name     string
		runtime  *fault.Error
		wantCode string
		wantExit int
	}{
		{
			name:     "catalog recovery replaces runtime prose",
			runtime:  fault.New(fault.KindInternal, "test_failed", "A test failed.", false, fault.NextAction{Command: "untrusted command", Reason: "Untrusted recovery."}),
			wantCode: "test_failed", wantExit: ExitInternal,
		},
		{
			name:     "deadline cause does not replace catalog fault",
			runtime:  fault.Wrap(fault.KindInternal, "test_failed", "A test failed.", false, context.DeadlineExceeded),
			wantCode: "test_failed", wantExit: ExitInternal,
		},
		{
			name:     "undeclared code fails closed",
			runtime:  fault.New(fault.KindInternal, "unexpected_code", "An unexpected test failed.", false),
			wantCode: "undeclared_fault_contract", wantExit: ExitContract,
		},
		{
			name:     "kind mismatch fails closed",
			runtime:  fault.New(fault.KindUnavailable, "test_failed", "A test source is unavailable.", false),
			wantCode: "undeclared_fault_contract", wantExit: ExitContract,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := utilitySpec("test")
			spec.handler = func(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
				return c.fail(ctx, test.runtime)
			}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader(""), &stdout, &stderr, NewCatalog(spec), passingInspector("unused"))
			if code := runCLI(command, []string{"--error-format=json", "test"}); code != test.wantExit {
				t.Fatalf("Run() code = %d, stderr = %q", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), `"code":"`+test.wantCode+`"`) || strings.Contains(stderr.String(), "untrusted command") {
				t.Fatalf("stderr = %q", stderr.String())
			}
			if test.wantCode == "test_failed" && !strings.Contains(stderr.String(), `"command":"test"`) {
				t.Fatalf("catalog recovery was not projected: %q", stderr.String())
			}
		})
	}
}

func TestEveryFaultKindHasStableExitCode(t *testing.T) {
	tests := map[fault.Kind]int{
		fault.KindInvalidInput: ExitUsage, fault.KindAuthentication: ExitAuthentication,
		fault.KindPermission: ExitPermission, fault.KindNotFound: ExitNotFound,
		fault.KindAmbiguous: ExitAmbiguous, fault.KindRateLimited: ExitRateLimited,
		fault.KindUnavailable: ExitUnavailable, fault.KindRejected: ExitRejected,
		fault.KindCanceled: ExitCanceled, fault.KindUnsupported: ExitUnsupported,
		fault.KindContract: ExitContract, fault.KindInternal: ExitInternal,
	}
	for kind, want := range tests {
		if got := exitCodeForKind(kind); got != want {
			t.Errorf("exitCodeForKind(%q) = %d, want %d", kind, got, want)
		}
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	return len(data) - 1, nil
}

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestSuccessWriterFailureIsNotReportedAsSuccess(t *testing.T) {
	var stderr bytes.Buffer
	command := New(strings.NewReader(""), shortWriter{}, &stderr)
	if code := runCLI(command, []string{"version"}); code != ExitInternal {
		t.Fatalf("short write code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "code: output_write_failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}

	stderr.Reset()
	command = New(strings.NewReader(""), errorWriter{err: io.ErrClosedPipe}, &stderr)
	if code := runCLI(command, []string{"version"}); code != ExitInternal {
		t.Fatalf("write error code = %d, stderr = %q", code, stderr.String())
	}
}

func TestDoctorJSONSnapshotEscapesExternalCategoryC(t *testing.T) {
	inspector := &cliInspector{report: doctor.Report{Checks: []doctor.Check{{
		Name: "runtime", Status: doctor.CheckStatusPass, Detail: "line\nESC:\x1b bidi:\u202e",
	}}}}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor", "--format", "json"}); code != ExitOK {
		t.Fatalf("Run() code = %d, stderr = %q", code, stderr.String())
	}
	want := "{\"schema_version\":1,\"report\":[{\"check\":\"runtime\",\"status\":\"pass\",\"detail\":\"line\\\\nESC:\\\\u001B bidi:\\\\u202E\"}]}\n"
	if stdout.String() != want {
		t.Fatalf("JSON output = %q, want %q", stdout.String(), want)
	}
}

func TestDoctorOversizeReturnsNoStdout(t *testing.T) {
	inspector := &cliInspector{report: doctor.Report{Checks: []doctor.Check{{
		Name: "runtime", Status: doctor.CheckStatusPass, Detail: strings.Repeat("x", maxDoctorDetailBytes+1),
	}}}}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor"}); code != ExitContract {
		t.Fatalf("Run() code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "code: output_contract_exceeded") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestDoctorRejectsArgumentsBeforeInspection(t *testing.T) {
	inspector := passingInspector("unused")
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor", "extra"}); code != ExitUsage {
		t.Fatalf("Run(doctor extra) code = %d, want %d", code, ExitUsage)
	}
	if inspector.calls != 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "usage: agentic-cli-foundry doctor") {
		t.Fatalf("calls = %d, stdout = %q, stderr = %q", inspector.calls, stdout.String(), stderr.String())
	}
}

func TestE2EDoctorUsesProductionOfflineAdapter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := New(strings.NewReader(""), &stdout, &stderr)
	if code := runCLI(command, []string{"doctor"}); code != ExitOK {
		t.Fatalf("Run(doctor) code = %d, stderr = %q", code, stderr.String())
	}
	output := stdout.String()
	if !strings.HasPrefix(output, "CHECK\tSTATUS\tDETAIL\nruntime\tpass\t") {
		t.Fatalf("doctor output = %q", output)
	}
	if !strings.Contains(output, runtime.Version()) || !strings.Contains(output, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Fatalf("doctor output does not describe runtime: %q", output)
	}
}

func TestEveryCatalogCommandDispatchesThroughItsSpec(t *testing.T) {
	for _, spec := range DefaultCatalog().Commands() {
		inspector := passingInspector("test/test")
		command, _, stderr := newTestCLI(inspector)
		args := strings.Split(spec.Path, " ")
		if spec.Path == "sample read" {
			args = append(args, "--id", "smp_2f4a6c8e0b1d")
		}
		if code := runCLI(command, args); code != ExitOK {
			t.Errorf("Run(%q) code = %d, stderr = %q", spec.Path, code, stderr.String())
		}
	}
}

func TestRootAliasesUseCatalogCommands(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"--help"}, want: "Agentic CLI Foundry\n"},
		{args: []string{"-h"}, want: "Agentic CLI Foundry\n"},
		{args: []string{"--version"}, want: "agentic-cli-foundry dev\n"},
		{args: []string{"-v"}, want: "agentic-cli-foundry dev\n"},
	}
	for _, test := range tests {
		command, stdout, stderr := newTestCLI(passingInspector("unused"))
		if code := runCLI(command, test.args); code != ExitOK {
			t.Errorf("Run(%v) code = %d, stderr = %q", test.args, code, stderr.String())
		}
		if !strings.HasPrefix(stdout.String(), test.want) {
			t.Errorf("Run(%v) output = %q, want prefix %q", test.args, stdout.String(), test.want)
		}
	}
}
