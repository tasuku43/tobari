package dockerruntime

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

const testBrokerRevision = "revision_synthetic"

func TestDecodeBrokerControlResponseAcceptsOnlyOperationSpecificSuccessFrames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		response string
		state    string
	}{
		{name: "health locked", args: []string{"health"}, response: `{"schema_version":1,"ok":true,"state":"locked"}`, state: "locked"},
		{name: "health unlocked", args: []string{"health"}, response: `{"schema_version":1,"ok":true,"state":"unlocked"}`, state: "unlocked"},
		{name: "unlock", args: []string{"unlock"}, response: `{"schema_version":1,"ok":true,"state":"unlocked"}`, state: "unlocked"},
		{name: "status locked", args: []string{"status", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"state":"locked","provider":"github"}`, state: "locked"},
		{name: "status absent", args: []string{"status", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"state":"not_configured","provider":"github"}`, state: "not_configured"},
		{name: "status ready", args: []string{"status", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"state":"ready","provider":"github","revision":"revision_synthetic"}`, state: "ready"},
		{name: "status ready label", args: []string{"status", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"state":"ready","provider":"github","revision":"revision_synthetic","account_label":"octocat"}`, state: "ready"},
		{name: "login", args: []string{"login", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"provider":"github","revision":"revision_synthetic","account_label":"octocat"}`},
		{name: "import", args: []string{"import", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"provider":"github","revision":"revision_synthetic"}`},
		{name: "logout changed", args: []string{"logout", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"provider":"github","state":"logged_out","changed":true}`, state: "logged_out"},
		{name: "logout unchanged", args: []string{"logout", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"provider":"github","state":"logged_out","changed":false}`, state: "logged_out"},
		{name: "issue handle", args: []string{"issue_handle", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"provider":"github","revision":"revision_synthetic","handle":"tobari-h1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`},
		{name: "binding ready", args: []string{"binding_status", "--provider", "github", "--revision", testBrokerRevision}, response: `{"schema_version":1,"ok":true,"state":"ready","provider":"github","revision":"revision_synthetic"}`, state: "ready"},
		{name: "binding missing", args: []string{"binding_status", "--provider", "github", "--revision", testBrokerRevision}, response: `{"schema_version":1,"ok":true,"state":"missing","provider":"github","revision":"revision_synthetic"}`, state: "missing"},
		{name: "binding stale", args: []string{"binding_status", "--provider", "github", "--revision", testBrokerRevision}, response: `{"schema_version":1,"ok":true,"state":"stale","provider":"github","revision":"revision_synthetic"}`, state: "stale"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			expectation, err := brokerControlExpectationFor(test.args)
			if err != nil {
				t.Fatal(err)
			}
			response, err := decodeBrokerControlResponse([]byte(test.response), expectation)
			if err != nil {
				t.Fatalf("decodeBrokerControlResponse() error = %v", err)
			}
			if !response.OK || response.State != test.state {
				t.Fatalf("response = %+v, want ok and state %q", response, test.state)
			}
			if expectation.Provider != "" && response.Provider != expectation.Provider {
				t.Fatalf("provider = %q, want %q", response.Provider, expectation.Provider)
			}
		})
	}
}

func TestDecodeBrokerControlResponseRejectsCrossOperationAndAmbiguousFrames(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		response string
	}{
		{name: "duplicate field", args: []string{"health"}, response: `{"schema_version":1,"ok":true,"ok":true,"state":"unlocked"}`},
		{name: "trailing object", args: []string{"health"}, response: `{"schema_version":1,"ok":true,"state":"unlocked"}{}`},
		{name: "health provider field", args: []string{"health"}, response: `{"schema_version":1,"ok":true,"state":"unlocked","provider":"github"}`},
		{name: "wrong schema", args: []string{"health"}, response: `{"schema_version":2,"ok":true,"state":"unlocked"}`},
		{name: "wrong provider", args: []string{"status", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"state":"not_configured","provider":"example"}`},
		{name: "absent status revision", args: []string{"status", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"state":"not_configured","provider":"github","revision":"revision_synthetic"}`},
		{name: "ready status null label", args: []string{"status", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"state":"ready","provider":"github","revision":"revision_synthetic","account_label":null}`},
		{name: "login missing label", args: []string{"login", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"provider":"github","revision":"revision_synthetic"}`},
		{name: "import has label", args: []string{"import", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"provider":"github","revision":"revision_synthetic","account_label":"octocat"}`},
		{name: "logout missing changed", args: []string{"logout", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"provider":"github","state":"logged_out"}`},
		{name: "logout wrong state", args: []string{"logout", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"provider":"github","state":"ready","changed":true}`},
		{name: "issue invalid handle", args: []string{"issue_handle", "--provider", "github"}, response: `{"schema_version":1,"ok":true,"provider":"github","revision":"revision_synthetic","handle":"tobari-h1_predictable"}`},
		{name: "binding wrong revision", args: []string{"binding_status", "--provider", "github", "--revision", testBrokerRevision}, response: `{"schema_version":1,"ok":true,"state":"ready","provider":"github","revision":"other_revision"}`},
		{name: "binding unknown state", args: []string{"binding_status", "--provider", "github", "--revision", testBrokerRevision}, response: `{"schema_version":1,"ok":true,"state":"unknown","provider":"github","revision":"revision_synthetic"}`},
		{name: "error extra top level", args: []string{"import", "--provider", "github"}, response: `{"schema_version":1,"ok":false,"error":{"code":"locked"},"provider":"github"}`},
		{name: "error extra nested", args: []string{"import", "--provider", "github"}, response: `{"schema_version":1,"ok":false,"error":{"code":"locked","detail":"canary"}}`},
		{name: "error duplicate code", args: []string{"import", "--provider", "github"}, response: `{"schema_version":1,"ok":false,"error":{"code":"locked","code":"transport_error"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			expectation, err := brokerControlExpectationFor(test.args)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeBrokerControlResponse([]byte(test.response), expectation); err == nil {
				t.Fatalf("decodeBrokerControlResponse() accepted %s", test.response)
			}
		})
	}
}

func TestDecodeBrokerControlResponseAcceptsOnlyExactErrorFrames(t *testing.T) {
	t.Parallel()
	expectation, err := brokerControlExpectationFor([]string{"import", "--provider", "github"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := decodeBrokerControlResponse(
		[]byte(`{"schema_version":1,"ok":false,"error":{"code":"locked"}}`), expectation,
	)
	if err != nil || response.OK || response.Error == nil || response.Error.Code != "locked" {
		t.Fatalf("error response = %+v, err=%v", response, err)
	}
}

type brokerProtocolRunner struct {
	stdout string
	err    error
	calls  int
}

func (r *brokerProtocolRunner) Run(
	_ context.Context, _ []string, _ []string, _ io.Reader, stdout, _ io.Writer,
) error {
	r.calls++
	_, _ = io.WriteString(stdout, r.stdout)
	return r.err
}

func (*brokerProtocolRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, nil
}

func TestRunBrokerControlPreservesExactAuthoritativeMutationFrames(t *testing.T) {
	t.Parallel()
	runner := &brokerProtocolRunner{
		stdout: `{"schema_version":1,"ok":true,"provider":"github","revision":"revision_synthetic"}`,
		err:    errors.New("docker reported a later exit failure"),
	}
	runtime := &Runtime{runner: runner}
	response, err := runtime.runBrokerControl(context.Background(), strings.NewReader("synthetic-secret"), "import", "--provider", "github")
	if err != nil || response.Revision != testBrokerRevision || runner.calls != 1 {
		t.Fatalf("response = %+v, calls=%d, err=%v", response, runner.calls, err)
	}

	runner.stdout = `{"schema_version":1,"ok":false,"error":{"code":"locked"}}`
	response, err = runtime.runBrokerControl(context.Background(), nil, "logout", "--provider", "github")
	var protocol brokerControlError
	if response.OK || !errors.As(err, &protocol) || protocol.Code != "locked" {
		t.Fatalf("authoritative error response = %+v, err=%v", response, err)
	}
}

func TestRunBrokerControlClassifiesLostMutationAcknowledgementAsNonRetryable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		stdout string
	}{
		{name: "missing", stdout: ""},
		{name: "malformed", stdout: `{"schema_version":1,"ok":true`},
		{name: "transport", stdout: `{"schema_version":1,"ok":false,"error":{"code":"transport_error"}}`},
		{name: "cross operation", stdout: `{"schema_version":1,"ok":true,"provider":"github","state":"logged_out","changed":true}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := &Runtime{runner: &brokerProtocolRunner{stdout: test.stdout}}
			_, err := runtime.runBrokerControl(context.Background(), strings.NewReader("synthetic-secret"), "import", "--provider", "github")
			public, ok := fault.PublicCopy(classifyBrokerError(err, "auth import github"))
			if !ok || public.Kind != fault.KindContract || public.Code != "auth_mutation_outcome_unknown" || public.Retryable {
				t.Fatalf("fault = %+v, ok=%t, private=%v", public, ok, err)
			}
			if len(public.NextActions) != 1 || public.NextActions[0].Command != "auth status" {
				t.Fatalf("next actions = %+v", public.NextActions)
			}
			publicText := strings.ToLower(public.Message + " " + public.NextActions[0].Reason)
			for _, forbidden := range []string{"retry", "rollback", "unchanged"} {
				if strings.Contains(publicText, forbidden) {
					t.Fatalf("unknown outcome claims %q in %q", forbidden, publicText)
				}
			}
		})
	}
}
