package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/permissionwaitcmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const permissionWaitTestID = "pwt_0123456789abcdef0123456789abcdef"

type permissionWaitCLIObserver struct {
	result tobari.PermissionWaitResult
	err    error
	calls  int
	onWait func()
}

func (o *permissionWaitCLIObserver) WaitPermission(context.Context, string) (tobari.PermissionWaitResult, error) {
	o.calls++
	if o.onWait != nil {
		o.onWait()
	}
	return o.result, o.err
}

func newPermissionWaitTestCLI(observer *permissionWaitCLIObserver, out, errOut io.Writer) *CLI {
	command := newCLI(strings.NewReader(""), out, errOut, DefaultCatalog().ForProgram(PermissionProgramName), passingInspector("ready"))
	if observer != nil {
		command.permissionWait = permissionwaitcmd.New(observer)
	}
	return command
}

func TestPermissionWaitCatalogOwnsOnlyPlainBoundedUtilityInput(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, found := catalog.Lookup("wait"); found {
		t.Fatal("host program exposed permission wait")
	}
	helper := catalog.ForProgram(PermissionProgramName)
	wait, found := helper.Lookup("wait")
	if !found || wait.Program != PermissionProgramName || wait.Role != RoleUtility || wait.Effect != operation.EffectRead ||
		wait.Agent.CapabilityID != "policy.permission-wait" || wait.Usage() != "tobari-permission wait --id <permission-wait-id> [--format text|json]" {
		t.Fatalf("permission wait command = %+v, found=%t", wait, found)
	}
	if len(wait.Agent.Inputs) != 2 {
		t.Fatalf("permission wait inputs = %+v", wait.Agent.Inputs)
	}
	id := wait.Agent.Inputs[0]
	if id.Name != "--id" || id.Source != InputSourceFlag || !id.Required || id.ValueKind != InputValueText ||
		id.Cardinality != InputCardinalitySingle || id.MinimumLength == nil || *id.MinimumLength != 36 ||
		id.MaximumLength == nil || *id.MaximumLength != 36 || id.ReferenceKind != "" || id.Completion != InputCompletionNone {
		t.Fatalf("permission wait ID contract = %+v", id)
	}
	if produced, consumed := wait.ProducedRefs(), wait.ConsumedRefs(); len(produced) != 0 || len(consumed) != 0 {
		t.Fatalf("permission wait reference edges = produced:%+v consumed:%+v", produced, consumed)
	}
	missing := commandErrorByCode(t, wait.Agent.Errors, "missing_permission_wait_observer")
	if missing.Kind != fault.KindInternal || missing.Phase != fault.PhaseObservation || missing.ChangeState != fault.ChangeNotApplicable {
		t.Fatalf("missing permission wait observer fault = %+v", missing)
	}
	foundGlobal := false
	for _, command := range catalog.PublicCommands() {
		if command.Program == PermissionProgramName && command.Path == "wait" {
			foundGlobal = true
			break
		}
	}
	if !foundGlobal {
		t.Fatal("global public Catalog view omitted permission wait")
	}
	for _, forbidden := range []string{"list", "poll", "pending", "approve", "retry"} {
		if _, found := helper.Lookup(forbidden); found {
			t.Fatalf("permission helper exposed %q", forbidden)
		}
	}
}

func TestPermissionWaitCatalogRejectsWiderScalarAndReplayableSettlement(t *testing.T) {
	for name, mutate := range map[string]func(*CommandSpec){
		"scalar field mismatch": func(spec *CommandSpec) {
			spec.Agent.Output.Fields[0].Name = "other"
		},
		"retryable settlement": func(spec *CommandSpec) {
			spec.Agent.Output.readSettlement = readOutputSettlementRetryable
		},
	} {
		t.Run(name, func(t *testing.T) {
			spec := permissionWaitSpec()
			mutate(&spec)
			if err := NewCatalog(spec).Validate(); err == nil {
				t.Fatal("invalid permission wait output contract passed validation")
			}
		})
	}
}

func TestPermissionWaitRendersOnlyClosedTerminalResults(t *testing.T) {
	for result, text := range map[tobari.PermissionWaitResult]string{
		tobari.PermissionWaitResultAllow: "Allow\n", tobari.PermissionWaitResultDeny: "Deny\n", tobari.PermissionWaitResultExpired: "Expired\n",
	} {
		t.Run(string(result), func(t *testing.T) {
			for _, format := range []struct {
				argv []string
				want string
			}{
				{argv: []string{"wait", "--id", permissionWaitTestID}, want: text},
				{argv: []string{"wait", "--id=" + permissionWaitTestID, "--format=json"}, want: `{"schema_version":1,"result":"` + string(result) + `"}` + "\n"},
			} {
				observer := &permissionWaitCLIObserver{result: result}
				stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
				command := newPermissionWaitTestCLI(observer, stdout, stderr)
				if code := command.RunContext(context.Background(), format.argv); code != ExitOK || stdout.String() != format.want || stderr.Len() != 0 || observer.calls != 1 {
					t.Fatalf("Run(%q) = code:%d stdout:%q stderr:%q calls:%d", format.argv, code, stdout.String(), stderr.String(), observer.calls)
				}
			}
		})
	}
}

func TestPermissionWaitRejectsInvalidInputBeforeObservation(t *testing.T) {
	for name, id := range map[string]string{
		"short": "pwt_0123456789abcdef0123456789abcde",
		"long":  "pwt_0123456789abcdef0123456789abcdef0",
	} {
		t.Run(name, func(t *testing.T) {
			observer := &permissionWaitCLIObserver{result: tobari.PermissionWaitResultAllow}
			command := newPermissionWaitTestCLI(observer, io.Discard, io.Discard)
			if code := command.RunContext(context.Background(), []string{"wait", "--id", id}); code != ExitUsage || observer.calls != 0 {
				t.Fatalf("invalid length = code:%d calls:%d", code, observer.calls)
			}
		})
	}
	observer := &permissionWaitCLIObserver{result: tobari.PermissionWaitResultAllow}
	stderr := &bytes.Buffer{}
	command := newPermissionWaitTestCLI(observer, io.Discard, stderr)
	if code := command.RunContext(context.Background(), []string{"wait", "--id", "pwt_0123456789abcdef0123456789abcdeg"}); code != ExitNotFound || observer.calls != 0 ||
		!strings.Contains(stderr.String(), "invalid_permission_wait") {
		t.Fatalf("invalid domain ID = code:%d calls:%d stderr:%q", code, observer.calls, stderr.String())
	}
}

func TestPermissionWaitMissingObserverNeverCollapsesToUndeclaredContract(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newPermissionWaitTestCLI(nil, stdout, stderr)
	command.permissionWait = permissionwaitcmd.New(nil)
	if code := command.RunContext(context.Background(), []string{"wait", "--id=" + permissionWaitTestID, "--format=json"}); code != ExitInternal {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "missing_permission_wait_observer") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

type permissionWaitFailWriter struct{}

func (permissionWaitFailWriter) Write([]byte) (int, error) {
	return 0, errors.New("synthetic short write")
}

func TestPermissionWaitTerminalConsumptionMakesOutputFailureNonRetryable(t *testing.T) {
	observer := &permissionWaitCLIObserver{result: tobari.PermissionWaitResultAllow}
	stderr := &bytes.Buffer{}
	command := newPermissionWaitTestCLI(observer, permissionWaitFailWriter{}, stderr)
	if code := command.RunContext(context.Background(), []string{"wait", "--id", permissionWaitTestID}); code != ExitInternal || observer.calls != 1 {
		t.Fatalf("consumed output failure = code:%d calls:%d", code, observer.calls)
	}
	for _, want := range []string{consumedReadOutputWriteFailureCode, "Retryable", "no", "tobari-permission help wait"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("consumed output fault lacks %q: %s", want, stderr.String())
		}
	}
}

func TestPermissionWaitTerminalResultWinsOverLaterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	observer := &permissionWaitCLIObserver{result: tobari.PermissionWaitResultDeny, onWait: cancel}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newPermissionWaitTestCLI(observer, stdout, stderr)
	if code := command.RunContext(ctx, []string{"wait", "--id", permissionWaitTestID}); code != ExitOK || stdout.String() != "Deny\n" || stderr.Len() != 0 {
		t.Fatalf("late cancellation = code:%d stdout:%q stderr:%q", code, stdout.String(), stderr.String())
	}
}

func TestPermissionWaitOwnerLossIsNonRetryableAndNeverExpires(t *testing.T) {
	observer := &permissionWaitCLIObserver{err: fault.New(fault.KindUnavailable, "permission_wait_owner_unavailable", "owner gone", false)}
	stderr := &bytes.Buffer{}
	command := newPermissionWaitTestCLI(observer, io.Discard, stderr)
	if code := command.RunContext(context.Background(), []string{"wait", "--id", permissionWaitTestID}); code != ExitUnavailable ||
		strings.Contains(stderr.String(), "Expired") || !strings.Contains(stderr.String(), "Retryable") || !strings.Contains(stderr.String(), "no") {
		t.Fatalf("owner loss = code:%d stderr:%q", code, stderr.String())
	}
}

func TestPermissionWaitPublicEntryFailsClosedWithoutAttachmentChannel(t *testing.T) {
	for _, name := range []string{"TOBARI_PERMISSION_SOCKET", "TOBARI_PERMISSION_CHANNEL", "TOBARI_PERMISSION_ATTACHMENT", "TOBARI_PERMISSION_OWNER_BINDING", "TOBARI_PERMISSION_VERIFY_KEY"} {
		t.Setenv(name, "")
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := NewPermissionHelper(strings.NewReader(""), stdout, stderr)
	if code := command.RunContext(context.Background(), []string{"wait", "--id", permissionWaitTestID}); code != ExitUnavailable ||
		stdout.Len() != 0 || !strings.Contains(stderr.String(), "permission_wait_owner_unavailable") || !strings.Contains(stderr.String(), "Retryable") || !strings.Contains(stderr.String(), "no") {
		t.Fatalf("missing channel = code:%d stdout:%q stderr:%q", code, stdout.String(), stderr.String())
	}
}

func TestPermissionWaitScopedHelpClosesRoutineTaskWithoutAuthority(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newPermissionWaitTestCLI(&permissionWaitCLIObserver{}, stdout, stderr)
	if code := command.RunContext(context.Background(), []string{"help", "wait", "--format=agent"}); code != ExitOK || stderr.Len() != 0 {
		t.Fatalf("help wait = code:%d stderr:%q", code, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{`"program":"tobari-permission"`, `"capability_id":"policy.permission-wait"`, `"role":"utility"`, `"minimum_length":36`, `"maximum_length":36`, `"produces_refs":[]`, `"consumes_refs":[]`} {
		if !strings.Contains(text, want) {
			t.Fatalf("permission help lacks %s: %s", want, text)
		}
	}
	for _, forbidden := range []string{"candidate_id", "policy_revision", "automatic_retry", "target_id_input", `"reference_kind"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("permission help exposes %q: %s", forbidden, text)
		}
	}
}
