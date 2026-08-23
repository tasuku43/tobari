//go:build tobari_dev && tobari_research

package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/infra/operatorconsole"
)

type operatorConsoleRunnerFake struct {
	open  bool
	calls int
}

type operatorConsoleFailureRunner struct {
	err error
}

func (f operatorConsoleFailureRunner) Run(
	_ context.Context, _ operatorconsole.Backend, _ bool, _ func(operatorconsole.Session) error,
) error {
	return f.err
}

func (f *operatorConsoleRunnerFake) Run(
	_ context.Context, _ operatorconsole.Backend, open bool, ready func(operatorconsole.Session) error,
) error {
	f.calls++
	f.open = open
	return ready(operatorconsole.Session{
		URL: "http://127.0.0.1:43119/#session=test-token", BrowserOpened: open,
	})
}

func TestServeUsesCatalogInputAndPrintsForegroundSession(t *testing.T) {
	t.Parallel()
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	command.tobari = tobaricmd.New(nil)
	runner := &operatorConsoleRunnerFake{}
	command.console = runner
	if code := runCLI(command, []string{"serve", "--no-open"}); code != ExitOK {
		t.Fatalf("serve code = %d, stderr = %q", code, stderr.String())
	}
	if runner.calls != 1 || runner.open {
		t.Fatalf("runner calls/open = %d/%t", runner.calls, runner.open)
	}
	for _, required := range []string{"Operator Console ready", "http://127.0.0.1:43119/#session=test-token", "manual URL ready", "Ctrl-C"} {
		if !strings.Contains(stdout.String(), required) {
			t.Errorf("serve output lacks %q: %q", required, stdout.String())
		}
	}
}

func TestServeCatalogDeclaresClosedPublicSurface(t *testing.T) {
	t.Parallel()
	spec, found := DefaultCatalog().Lookup("serve")
	if !found || spec.Agent.CapabilityID != "operator.console" || spec.Args != "[--no-open]" {
		t.Fatalf("serve spec = %#v, found=%t", spec, found)
	}
	if len(spec.Agent.Inputs) != 1 || spec.Agent.Inputs[0].Name != "--no-open" {
		t.Fatalf("serve inputs = %#v", spec.Agent.Inputs)
	}
}

func TestServePreservesComposedSnapshotReadFaults(t *testing.T) {
	t.Parallel()
	failures := append([]CommandError{{
		Kind: fault.KindUnavailable, Code: "cluster_reconcile_interrupted", Retryable: false,
	}}, policyClusterReadinessErrors()...)
	for _, declared := range failures {
		declared := declared
		t.Run(declared.Code, func(t *testing.T) {
			t.Parallel()
			command, _, stderr := newTestCLI(passingInspector("unused"))
			command.tobari = tobaricmd.New(nil)
			command.console = operatorConsoleFailureRunner{err: fault.New(
				declared.Kind, declared.Code, "simulated operator console preflight failure", declared.Retryable,
			)}
			if code := runCLI(command, []string{"serve", "--no-open"}); code == ExitContract {
				t.Fatalf("serve replaced %q with a contract fault: %q", declared.Code, stderr.String())
			}
			if !humanOutputHasRow(stderr.String(), "Code", declared.Code) ||
				strings.Contains(stderr.String(), "undeclared_fault_contract") {
				t.Fatalf("serve fault = %q", stderr.String())
			}
		})
	}
}

func TestServeStartupOutputFailureUsesDeclaredReadFault(t *testing.T) {
	t.Parallel()
	command, _, stderr := newTestCLI(passingInspector("unused"))
	command.Out = shortWriter{}
	command.tobari = tobaricmd.New(nil)
	command.console = &operatorConsoleRunnerFake{}
	if code := runCLI(command, []string{"serve", "--no-open"}); code != ExitInternal {
		t.Fatalf("serve short-write code = %d, stderr = %q", code, stderr.String())
	}
	if !humanOutputHasRow(stderr.String(), "Code", "output_write_failed") ||
		!humanOutputHasRow(stderr.String(), "Retryable", "yes") {
		t.Fatalf("serve short-write fault = %q", stderr.String())
	}
}
