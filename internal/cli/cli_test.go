package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/capabilityprofile"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
)

type cliInspector struct {
	defaultObservation doctor.Observation
	observations       map[doctor.CheckID]doctor.Observation
	err                error
	calls              int
	ctx                context.Context
	roots              []string
}

func (i *cliInspector) ObserveDoctorCheck(ctx context.Context, root string, id doctor.CheckID) (doctor.Observation, error) {
	i.calls++
	i.ctx = ctx
	i.roots = append(i.roots, root)
	if i.err != nil {
		return doctor.Observation{}, i.err
	}
	if observation, exists := i.observations[id]; exists {
		return observation, nil
	}
	if i.defaultObservation.Status == "" {
		return doctor.Observation{Status: doctor.CheckStatusPass, Detail: "observed"}, nil
	}
	return i.defaultObservation, nil
}

func newTestCLI(inspector *cliInspector) (*CLI, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), inspector)
	return command, stdout, stderr
}

func newReferenceTestCLI(in io.Reader, out, errOut io.Writer) *CLI {
	command := New(in, out, errOut)
	commands := DefaultCatalog().registeredCommands()
	list := discoverSpec("items list", "item")
	list.Args = "[--format tsv|json]"
	list.Agent.Inputs = []CommandInput{{
		Name: "--format", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Select the complete test item collection representation.",
		AllowedValues: []string{"tsv", "json"}, DefaultValue: stringPointer("tsv"),
	}}
	read := actSpec("items read", "item", "--id")
	read.Summary = "Read exactly one test item by opaque ID"
	read.Args = "--id <item-id> [--format tsv|json]"
	read.Agent.Inputs = append(read.Agent.Inputs, CommandInput{
		Name: "--format", Source: InputSourceFlag, Required: false,
		ValueKind: InputValueText, Cardinality: InputCardinalitySingle,
		Description:   "Select the test item representation.",
		AllowedValues: []string{"tsv", "json"}, DefaultValue: stringPointer("tsv"),
	})
	read.Agent.Output = CommandOutput{
		Formats:       []OutputFormat{OutputFormatTSV, OutputFormatJSON},
		DefaultFormat: OutputFormatTSV,
		Fields: []OutputField{
			{Name: "id", Type: OutputFieldTypeString, Description: "Exact opaque test item ID requested by the caller."},
			{Name: "name", Type: OutputFieldTypeString, Description: "Test item name."},
		},
		Delivery:           OutputDeliveryComplete,
		CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope:       "item",
		JSONEnvelopeType:   OutputFieldTypeObject,
		JSONSchemaVersion:  1,
	}
	read.Agent.Errors = append(read.Agent.Errors, declaredCommandError(
		fault.KindNotFound,
		"item_not_found",
		false,
		"items list",
		"Discover a current opaque test item ID.",
	))
	command.catalog = NewCatalog(append(commands, list, read)...)
	return command
}

func passingInspector(detail string) *cliInspector {
	return &cliInspector{defaultObservation: doctor.Observation{Status: doctor.CheckStatusPass, Detail: detail}}
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

func TestNoArgsDispatchesThePrimaryRootOutcome(t *testing.T) {
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	if code := runCLI(command, nil); code != ExitInternal {
		t.Fatalf("Run(nil) code = %d, want %d", code, ExitInternal)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !humanOutputHasRow(stderr.String(), "Kind", "internal") || !humanOutputHasRow(stderr.String(), "Code", "missing_runtime") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestDelimiterLedRootInvocationDispatchesExactChildArgv(t *testing.T) {
	t.Parallel()
	var calls int
	var got []string
	var gotContext string
	spec := projectEnterSpec()
	spec.handler = func(_ context.Context, _ *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
		calls++
		got = inputs.Values("command")
		gotContext = inputs.One("--context")
		return 37
	}
	command := newCLI(strings.NewReader(""), io.Discard, io.Discard, catalogWithProjectSpec(spec), nil)
	argv := []string{"--context", "toolbox", "--", "claude", "--model", "", "--model", "-value"}
	if code := command.RunContext(context.Background(), argv); code != 37 {
		t.Fatalf("delimiter-led root exit = %d, want child 37", code)
	}
	want := []string{"claude", "--model", "", "--model", "-value"}
	if calls != 1 || gotContext != "toolbox" || !reflect.DeepEqual(got, want) {
		t.Fatalf("handler calls=%d Context=%q argv=%q, want toolbox and exact %q", calls, gotContext, got, want)
	}
}

func TestDirectRootCommandInvalidFormsFailBeforeHandler(t *testing.T) {
	t.Parallel()
	var calls int
	spec := projectEnterSpec()
	spec.handler = func(context.Context, *CLI, CommandSpec, operation.Intent, ParsedInputs) int {
		calls++
		return ExitOK
	}
	for name, argv := range map[string][]string{
		"bare delimiter":    {"--"},
		"missing delimiter": {"tobari", "claude"},
	} {
		t.Run(name, func(t *testing.T) {
			var stderr bytes.Buffer
			command := newCLI(strings.NewReader(""), io.Discard, &stderr, catalogWithProjectSpec(spec), nil)
			if code := command.RunContext(context.Background(), argv); code != ExitUsage {
				t.Fatalf("RunContext(%q) = %d, stderr=%q", argv, code, stderr.String())
			}
		})
	}
	if calls != 0 {
		t.Fatalf("invalid direct invocations reached handler %d times", calls)
	}
}

func catalogWithProjectSpec(projectSpec CommandSpec) Catalog {
	commands := DefaultCatalog().registeredCommands()
	for index := range commands {
		if commands[index].Path == "tobari" {
			commands[index] = projectSpec
			break
		}
	}
	return NewCatalog(commands...)
}

func TestUnknownCommandUsesUsageExitCode(t *testing.T) {
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	if code := runCLI(command, []string{"missing"}); code != ExitUsage {
		t.Fatalf("Run(missing) code = %d, want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 || !humanOutputHasRow(stderr.String(), "Code", "unknown_command") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestLifecycleInvocationContextNormalizesPrefixAndRejectsDuplicates(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"tobari", "status", "delete"} {
		command, found := DefaultCatalog().Lookup(path)
		if !found {
			t.Fatalf("%s command is absent", path)
		}
		t.Run(path, func(t *testing.T) {
			for _, test := range []struct {
				name string
				root string
				rest []string
				want string
			}{
				{name: "omitted"},
				{name: "prefix", root: "toolbox", want: "toolbox"},
				{name: "command local", rest: []string{"--context", "toolbox"}, want: "toolbox"},
			} {
				t.Run(test.name, func(t *testing.T) {
					normalized := normalizeLifecycleContextInput(command, test.root, test.rest)
					inputs, err := parseCommandInputs(command, normalized)
					if err != nil {
						t.Fatal(err)
					}
					if got := inputs.One("--context"); got != test.want {
						t.Fatalf("normalized Context = %q, want %q (argv=%v)", got, test.want, normalized)
					}
				})
			}

			duplicates := normalizeLifecycleContextInput(command, "toolbox", []string{"--context", "default"})
			if _, err := parseCommandInputs(command, duplicates); err == nil || !strings.Contains(err.Error(), "may be specified only once") {
				t.Fatalf("duplicate normalized Context error = %v (argv=%v)", err, duplicates)
			}
		})
	}
}

func TestInvocationContextPrefixRemainsOutsideNonLifecycleCommands(t *testing.T) {
	t.Parallel()
	command := utilitySpec("probe")
	rest := []string{"--context", "default"}
	got := normalizeLifecycleContextInput(command, "fallback", rest)
	if strings.Join(got, "\x00") != strings.Join(rest, "\x00") {
		t.Fatalf("non-lifecycle argv = %v, want %v", got, rest)
	}
}

func TestLifecyclePrefixAndCommandLocalPlacementReachHandlerIdentically(t *testing.T) {
	t.Parallel()
	var seen []string
	spec := utilitySpec("probe")
	spec.Args = "[--context <name>]"
	spec.Agent.CapabilityID = "tobari.lifecycle"
	spec.Agent.Inputs = []CommandInput{lifecycleContextInput()}
	spec.handler = func(_ context.Context, _ *CLI, _ CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
		seen = append(seen, inputs.One("--context"))
		return ExitOK
	}
	command := newCLI(strings.NewReader(""), io.Discard, io.Discard, NewCatalog(spec), nil)
	if code := command.RunContext(context.Background(), []string{"--context", "toolbox", "probe"}); code != ExitOK {
		t.Fatalf("prefix code = %d", code)
	}
	if code := command.RunContext(context.Background(), []string{"probe", "--context", "toolbox"}); code != ExitOK {
		t.Fatalf("command-local code = %d", code)
	}
	if strings.Join(seen, ",") != "toolbox,toolbox" {
		t.Fatalf("handler Contexts = %v", seen)
	}
}

func TestLifecycleContextDuplicateAndEmptyFailBeforeHandler(t *testing.T) {
	t.Parallel()
	calls := 0
	spec := utilitySpec("probe")
	spec.Args = "[--context <name>]"
	spec.Agent.CapabilityID = "tobari.lifecycle"
	spec.Agent.Inputs = []CommandInput{lifecycleContextInput()}
	spec.handler = func(context.Context, *CLI, CommandSpec, operation.Intent, ParsedInputs) int {
		calls++
		return ExitOK
	}
	for _, args := range [][]string{
		{"probe", "--context="},
		{"probe", "--context", ""},
		{"--context=", "probe"},
		{"--context", "", "probe"},
		{"--context", "toolbox", "probe", "--context", "default"},
	} {
		var stderr bytes.Buffer
		command := newCLI(strings.NewReader(""), io.Discard, &stderr, NewCatalog(spec), nil)
		if code := command.RunContext(context.Background(), args); code != ExitUsage {
			t.Fatalf("args=%v code=%d stderr=%q", args, code, stderr.String())
		}
	}
	if calls != 0 {
		t.Fatalf("invalid Context reached handler %d times", calls)
	}
}

func TestRemovedSampleNamespaceIsUnknown(t *testing.T) {
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	if code := runCLI(command, []string{"sample", "list"}); code != ExitUsage {
		t.Fatalf("removed sample namespace code = %d, want %d", code, ExitUsage)
	}
	if stdout.Len() != 0 || !humanOutputHasRow(stderr.String(), "Code", "unknown_command") {
		t.Fatalf("removed sample namespace stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestHumanRootRecoveryActionIsExecutable(t *testing.T) {
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	ctx := withCommandPath(context.Background(), "delete")
	err := fault.New(
		fault.KindNotFound,
		"project_not_found",
		"no Workspace exists for the current directory",
		false,
		fault.NextAction{Command: "tobari", Reason: "Create a Workspace from the current project directory."},
	)
	if code := command.fail(ctx, err); code != ExitNotFound {
		t.Fatalf("fail() code = %d, want %d", code, ExitNotFound)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if !humanOutputHasRow(stderr.String(), "Next", "tobari — Create a Workspace from the current project directory.") {
		t.Fatalf("stderr = %q, want executable root recovery", stderr.String())
	}
	if strings.Contains(stderr.String(), "Next           tobari tobari") {
		t.Fatalf("stderr duplicated executable name: %q", stderr.String())
	}
}

func TestVersionOutputContract(t *testing.T) {
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	command.Version = "v1.2.3"
	command.Commit = "0123456789abcdef0123456789abcdef01234567"
	if code := runCLI(command, []string{"version"}); code != ExitOK {
		t.Fatalf("Run(version) code = %d, stderr = %q", code, stderr.String())
	}
	identity, err := dockerruntime.BuildIdentity(command.Version, command.Commit)
	if err != nil {
		t.Fatal(err)
	}
	want := "✓ Tobari build\n" +
		"  Version        v1.2.3\n" +
		"  Commit         0123456789abcdef0123456789abcdef01234567\n" +
		"  Resolver       " + string(identity.ResolverChannel) + "\n" +
		"  Capabilities   " + string(capabilityprofile.Compiled()) + "\n" +
		"  Gateway API    required 1, selected 1\n"
	if identity.CapabilityProfile.IncludesExperimental() {
		want += "  Auth Broker API required 1, selected 1\n"
	}
	want += "  Compatibility  compatible\n"
	if build, binary, development := identity.DevelopmentRecovery(); development {
		want += "  Build          " + build + "\n" +
			"  Binary         " + binary + "\n"
	}
	if got := stdout.String(); got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
	stdout.Reset()
	if code := runCLI(command, []string{"version", "--format", "json"}); code != ExitOK {
		t.Fatalf("Run(version --format json) code = %d, stderr = %q", code, stderr.String())
	}
	var document map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	projection := document["build_identity"].(map[string]any)
	_, hasBrokerRequired := projection["auth_broker_required_api"]
	_, hasBrokerSelected := projection["auth_broker_selected_api"]
	if hasBrokerRequired != identity.CapabilityProfile.IncludesExperimental() || hasBrokerSelected != hasBrokerRequired {
		t.Fatalf("version JSON Broker fields = %t/%t, identity = %+v", hasBrokerRequired, hasBrokerSelected, identity)
	}
}

func TestDoctorOutputContract(t *testing.T) {
	inspector := passingInspector("observed")
	inspector.observations = map[doctor.CheckID]doctor.Observation{
		doctor.CheckIDDockerCLI: {Status: doctor.CheckStatusPass, Detail: "runtime-version\ttest/test\nlocal"},
		doctor.CheckIDProxyPort: {Status: doctor.CheckStatusWarn, Detail: "path\\value\x1b"},
	}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor"}); code != ExitOK {
		t.Fatalf("Run(doctor) code = %d, stderr = %q", code, stderr.String())
	}
	output := stdout.String()
	if !strings.HasPrefix(output, "✓ Environment check\n  docker_cli     pass   runtime-version\\ttest/test\\nlocal\n") ||
		!strings.Contains(output, "\n  proxy_port     warn   path\\\\value\\u001B\n") ||
		!strings.Contains(output, "\n  owned_resources pass   observed\n") ||
		len(strings.Split(strings.TrimSuffix(output, "\n"), "\n")) != len(doctorCheckIDValues())+1 {
		t.Fatalf("doctor output = %q", output)
	}
	if strings.Contains(stdout.String(), "\n  Details") {
		t.Fatalf("doctor output repeats detail labels: %q", stdout.String())
	}
	if stderr.Len() != 0 || inspector.calls != len(doctor.CheckInventory()) {
		t.Fatalf("stderr = %q, inspector calls = %d", stderr.String(), inspector.calls)
	}
}

func TestDoctorTSVProjectionRemainsAvailable(t *testing.T) {
	inspector := passingInspector("observed")
	inspector.observations = map[doctor.CheckID]doctor.Observation{
		doctor.CheckIDDockerCLI: {Status: doctor.CheckStatusPass, Detail: "runtime-version"},
	}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor", "--format", "tsv"}); code != ExitOK {
		t.Fatalf("Run(doctor --format tsv) code = %d, stderr = %q", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSuffix(stdout.String(), "\n"), "\n")
	if len(lines) != len(doctorCheckIDValues())+1 ||
		lines[0] != "CHECK\tSTATUS\tBLOCKED_BY\tDETAIL\tRECOVERY_ACTION\tNEXT_COMMAND" ||
		lines[1] != "docker_cli\tpass\t\truntime-version\t\t" ||
		lines[len(lines)-1] != "owned_resources\tpass\t\tobserved\t\t" {
		t.Fatalf("TSV doctor output = %q", stdout.String())
	}
}

func TestDoctorFailureUsesRejectedExitAndStructuredRecovery(t *testing.T) {
	inspector := passingInspector("observed")
	inspector.observations = map[doctor.CheckID]doctor.Observation{
		doctor.CheckIDDockerCLI: {Status: doctor.CheckStatusFail, Detail: "unsupported"},
	}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor"}); code != ExitRejected {
		t.Fatalf("Run(doctor) code = %d, want %d", code, ExitRejected)
	}
	if !strings.Contains(stdout.String(), "docker_cli     fail") ||
		!strings.Contains(stdout.String(), "docker_engine  blocked") ||
		!strings.Contains(stdout.String(), "Recovery\n") ||
		!strings.Contains(stdout.String(), "tobari doctor") ||
		!humanOutputHasRow(stderr.String(), "Code", "diagnostic_failed") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestDoctorJSONPreservesBlockedAndRecoveryFacts(t *testing.T) {
	inspector := passingInspector("observed")
	inspector.observations = map[doctor.CheckID]doctor.Observation{
		doctor.CheckIDDockerCLI: {Status: doctor.CheckStatusFail, Detail: "docker missing"},
	}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor", "--format=json"}); code != ExitRejected {
		t.Fatalf("Run(doctor --format=json) code = %d, stderr = %q", code, stderr.String())
	}
	var document doctorJSONDocument
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("decode doctor JSON: %v, output = %q", err, stdout.String())
	}
	if document.SchemaVersion != 1 || len(document.Report) != len(doctorCheckIDValues()) {
		t.Fatalf("doctor document header = %+v", document)
	}
	root := document.Report[0]
	if root.Check != "docker_cli" || root.Status != "fail" || root.BlockedBy != nil || root.Recovery == nil ||
		root.Recovery.Action == "" || root.Recovery.NextCommand != "doctor" {
		t.Fatalf("docker_cli JSON = %+v", root)
	}
	blocked := document.Report[1]
	if blocked.Check != "docker_engine" || blocked.Status != "blocked" || blocked.BlockedBy == nil ||
		*blocked.BlockedBy != "docker_cli" || blocked.Recovery != nil {
		t.Fatalf("docker_engine JSON = %+v", blocked)
	}
	independent := document.Report[4]
	if independent.Check != "proxy_port" || independent.Status != "pass" || independent.BlockedBy != nil || independent.Recovery != nil {
		t.Fatalf("proxy_port JSON = %+v", independent)
	}
}

func TestDoctorWarningRecoveryIsRenderedWithoutFailing(t *testing.T) {
	inspector := passingInspector("observed")
	inspector.observations = map[doctor.CheckID]doctor.Observation{
		doctor.CheckIDState: {Status: doctor.CheckStatusWarn, Detail: "cluster is not configured"},
	}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor"}); code != ExitOK {
		t.Fatalf("Run(doctor) code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "state          warn") || !strings.Contains(stdout.String(), "Recovery\n") ||
		!strings.Contains(stdout.String(), "tobari cluster up") {
		t.Fatalf("doctor warning output = %q", stdout.String())
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
	if inspector.calls != 0 || stdout.Len() != 0 || !humanOutputHasRow(stderr.String(), "Code", "missing_context") {
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
	if inspector.calls != 0 || stdout.Len() != 0 || !humanOutputHasRow(stderr.String(), "Code", "operation_canceled") {
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
	if stdout.Len() != 0 || !humanOutputHasRow(stderr.String(), "Code", "operation_canceled") {
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
	if !humanOutputHasRow(stderr.String(), "Code", "mutation_output_write_failed") ||
		!humanOutputHasRow(stderr.String(), "Phase", "presentation") ||
		!humanOutputHasRow(stderr.String(), "Change state", "confirmed") ||
		!humanOutputHasRow(stderr.String(), "Retryable", "no") ||
		!humanOutputHasRow(stderr.String(), "Next", ProgramName+" items list — Reconcile the confirmed mutation result without repeating the mutation.") ||
		humanOutputHasRow(stderr.String(), "Code", "operation_canceled") ||
		strings.Contains(stderr.String(), ProgramName+" items update") {
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
		if !humanOutputHasRow(stderr.String(), "Code", "mutation_output_write_failed") ||
			!humanOutputHasRow(stderr.String(), "Phase", "presentation") ||
			!humanOutputHasRow(stderr.String(), "Change state", "confirmed") ||
			!humanOutputHasRow(stderr.String(), "Retryable", "no") ||
			!humanOutputHasRow(stderr.String(), "Next", ProgramName+" items list — Reconcile the confirmed mutation result without repeating the mutation.") ||
			humanOutputHasRow(stderr.String(), "Code", "undeclared_fault_contract") {
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
	commands := DefaultCatalog().registeredCommands()
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
				"items list",
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
	if document.SchemaVersion != 2 || document.Error.Kind != "internal" || document.Error.Code != "internal_error" ||
		document.Error.Phase != fault.PhaseObservation || document.Error.ChangeState != fault.ChangeNotApplicable ||
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
	if !humanOutputHasRow(string(unknown), "Retry after", "unknown") {
		t.Fatalf("rate-limit text timing = %q", unknown)
	}
	nonRateLimit := renderTextError(errorPayload{
		Kind:      fault.KindUnavailable,
		Code:      "provider_unavailable",
		Message:   "The provider is unavailable.",
		Retryable: true,
	})
	if !humanOutputHasRow(string(nonRateLimit), "Retry after", "none") {
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
	ctx := withCommandPath(context.Background(), "items read")
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
		{
			name: "phase and change-state mismatch fails closed",
			runtime: fault.WithClassification(
				fault.New(fault.KindInternal, "test_failed", "A test failed.", false),
				fault.PhasePrecondition, fault.ChangeNone,
			),
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
	if !humanOutputHasRow(stderr.String(), "Code", "output_write_failed") {
		t.Fatalf("stderr = %q", stderr.String())
	}

	stderr.Reset()
	command = New(strings.NewReader(""), errorWriter{err: io.ErrClosedPipe}, &stderr)
	if code := runCLI(command, []string{"version"}); code != ExitInternal {
		t.Fatalf("write error code = %d, stderr = %q", code, stderr.String())
	}
}

func TestDoctorJSONSnapshotEscapesExternalCategoryC(t *testing.T) {
	inspector := passingInspector("observed")
	inspector.observations = map[doctor.CheckID]doctor.Observation{
		doctor.CheckIDDockerCLI: {Status: doctor.CheckStatusPass, Detail: "line\nESC:\x1b bidi:\u202e"},
	}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor", "--format", "json"}); code != ExitOK {
		t.Fatalf("Run() code = %d, stderr = %q", code, stderr.String())
	}
	var document doctorJSONDocument
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("JSON output: %v, output = %q", err, stdout.String())
	}
	if document.SchemaVersion != 1 || len(document.Report) != len(doctorCheckIDValues()) ||
		document.Report[0].Check != "docker_cli" || document.Report[0].Detail != "line\\nESC:\\u001B bidi:\\u202E" ||
		document.Report[0].BlockedBy != nil || document.Report[0].Recovery != nil {
		t.Fatalf("JSON document = %+v", document)
	}
}

func TestDoctorOversizeReturnsNoStdout(t *testing.T) {
	inspector := passingInspector("observed")
	inspector.observations = map[doctor.CheckID]doctor.Observation{
		doctor.CheckIDDockerCLI: {Status: doctor.CheckStatusPass, Detail: strings.Repeat("x", maxDoctorDetailBytes+1)},
	}
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor"}); code != ExitContract {
		t.Fatalf("Run() code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 || !humanOutputHasRow(stderr.String(), "Code", "output_contract_exceeded") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
}

func TestDoctorOversizeRecoveryReturnsNoStdout(t *testing.T) {
	report := completeDoctorCLIReport(doctor.CheckStatusPass, "observed")
	report.Checks[0].Status = doctor.CheckStatusFail
	report.Checks[0].Recovery = &doctor.Recovery{Action: strings.Repeat("x", maxDoctorActionBytes+1), NextCommand: "doctor"}
	if err := validateDoctorProjection(report); err == nil {
		t.Fatal("oversize recovery passed projection validation")
	}
	report.Checks[0].Recovery = &doctor.Recovery{Action: "install Docker", NextCommand: strings.Repeat("x", maxDoctorCommandBytes+1)}
	if err := validateDoctorProjection(report); err == nil {
		t.Fatal("oversize next command passed projection validation")
	}
}

func completeDoctorCLIReport(status doctor.CheckStatus, detail string) doctor.Report {
	checks := make([]doctor.Check, 0, len(doctor.CheckInventory()))
	for _, spec := range doctor.CheckInventory() {
		checks = append(checks, doctor.Check{Name: spec.ID, Status: status, Detail: detail})
	}
	return doctor.Report{Checks: checks}
}

func TestDoctorRejectsArgumentsBeforeInspection(t *testing.T) {
	inspector := passingInspector("unused")
	command, stdout, stderr := newTestCLI(inspector)
	if code := runCLI(command, []string{"doctor", "extra"}); code != ExitUsage {
		t.Fatalf("Run(doctor extra) code = %d, want %d", code, ExitUsage)
	}
	if inspector.calls != 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "usage: tobari doctor") {
		t.Fatalf("calls = %d, stdout = %q, stderr = %q", inspector.calls, stdout.String(), stderr.String())
	}
}

func TestE2EDoctorUsesProductionRuntimeAdapter(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := New(strings.NewReader(""), &stdout, &stderr)
	code := runCLI(command, []string{"doctor"})
	if code != ExitOK && code != ExitRejected {
		t.Fatalf("Run(doctor) code = %d, stderr = %q", code, stderr.String())
	}
	output := stdout.String()
	marker := "✓"
	if code == ExitRejected {
		marker = "✗"
	}
	if !strings.HasPrefix(output, marker+" Environment check\n  docker_cli     pass") {
		t.Fatalf("doctor output = %q", output)
	}
	if !strings.Contains(output, "\n  docker_engine  pass") || !strings.Contains(output, "\n  docker_context pass") {
		t.Fatalf("doctor output does not describe Docker runtime: %q", output)
	}
}

func TestEveryCatalogCommandDispatchesThroughItsSpec(t *testing.T) {
	for _, spec := range DefaultCatalog().Commands() {
		inspector := passingInspector("test/test")
		var stdout, stderr bytes.Buffer
		commands := DefaultCatalog().registeredCommands()
		for index := range commands {
			commands[index].handler = noOpHandler
		}
		command := newCLI(strings.NewReader(""), &stdout, &stderr, NewCatalog(commands...), inspector)
		args := strings.Split(spec.Path, " ")
		for _, input := range spec.Agent.Inputs {
			if !input.Required || input.DefaultValue != nil {
				continue
			}
			value := "value"
			if len(input.AllowedValues) != 0 {
				value = input.AllowedValues[0]
			}
			switch input.Name {
			case "--id":
				value = "smp_2f4a6c8e0b1d"
			case "--root":
				value = "/tmp"
			case "--current":
				value = "1"
			case "command":
				value = "true"
			}
			if input.Source == InputSourceFlag {
				args = append(args, input.Name+"="+value)
			} else if input.Source == InputSourceArgument {
				args = append(args, value)
			}
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
		{args: []string{"--help"}, want: "Tobari\n"},
		{args: []string{"-h"}, want: "Tobari\n"},
		{args: []string{"--version"}, want: "! Tobari build\n"},
		{args: []string{"-v"}, want: "! Tobari build\n"},
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
