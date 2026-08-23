package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type permissionObserverRunner struct {
	output []byte
	err    error
	args   []string
}

func (r *permissionObserverRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return errors.New("unexpected Run call")
}

func (r *permissionObserverRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.args = append([]string{}, args...)
	return append([]byte{}, r.output...), r.err
}

func permissionObserverRuntime(t *testing.T, output string) (*Runtime, *permissionObserverRunner, tobari.PermissionWaitRecord) {
	t.Helper()
	root := t.TempDir()
	runner := &permissionObserverRunner{output: []byte(output)}
	runtime, err := newRuntime(root+"/config", root+"/state", runner)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	if err := runtime.writeState(state); err != nil {
		t.Fatal(err)
	}
	record := permissionWaitRecordFixtureForInfra()
	return runtime, runner, record
}

func permissionWaitRecordFixtureForInfra() tobari.PermissionWaitRecord {
	return tobari.PermissionWaitRecord{
		SchemaVersion: tobari.PermissionWaitRecordSchema,
		ID:            "pwt_0123456789abcdef0123456789abcdef", DenialCorrelationID: "abcdef0123456789abcdef0123456789",
		FrozenPrincipalFingerprint: strings.Repeat("b", 64),
		WorkspaceManifestID:        "01912345-6789-7abc-8def-0123456789ad",
		WorkspaceID:                "01912345-6789-7abc-8def-0123456789ab",
		AttachmentID:               "att_0123456789abcdef0123456789abcdef",
		Effect:                     tobari.PermissionWaitEffect{Scheme: "https", Host: "api.example.com", Port: 443, Method: "GET", Path: "/items/a%20b", Segments: []string{"items", "a b"}},
		CreatedAt:                  "2026-08-23T00:00:00Z", ExpiresAt: "2026-08-23T00:15:00Z",
	}
}

func TestPermissionObserverReusesCanonicalLiveOPAForTerminalResults(t *testing.T) {
	for name, document := range map[string]string{
		"allow exact or template": `{"allow":true,"reason":"allowed by Context policy","status_code":403,"learnable":false}`,
		"explicit exact deny":     `{"allow":false,"reason":"denied by exact policy","status_code":403,"learnable":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			revision := strings.Repeat("a", 64)
			runtime, runner, record := permissionObserverRuntime(t, `{"revision":"`+revision+`","decision":`+document+`}`)
			result, terminal, err := runtime.ObservePermissionDisposition(context.Background(), record)
			if err != nil || !terminal {
				t.Fatalf("observation = %q, %t, %v", result, terminal, err)
			}
			want := tobari.PermissionWaitResultAllow
			if name == "explicit exact deny" {
				want = tobari.PermissionWaitResultDeny
			}
			if result != want {
				t.Fatalf("result = %q, want %q", result, want)
			}
			query := strings.Join(runner.args, " ")
			for _, required := range []string{"/v1/data/tobari/aggregate_revision", "/v1/data/tobari/http/decision", `"context_id":"` + record.WorkspaceManifestID, `"project_id":"` + record.WorkspaceID, `"segments":["items","a b"]`, `result := {"revision":`} {
				if !strings.Contains(query, required) {
					t.Fatalf("OPA query omitted %q: %s", required, query)
				}
			}
			if len(runner.args) == 0 || !strings.HasPrefix(runner.args[len(runner.args)-1], "[result | ") ||
				!strings.HasSuffix(runner.args[len(runner.args)-1], "}][0]") {
				t.Fatalf("OPA raw query does not emit the bound observation object: %s", query)
			}
		})
	}
}

func TestPermissionObserverKeepsDefaultDenyAndStaleRevisionNonterminal(t *testing.T) {
	tests := map[string]string{
		"default deny":   `{"revision":"` + strings.Repeat("a", 64) + `","decision":{"allow":false,"reason":"request did not match an allow rule","status_code":403,"learnable":true}}`,
		"method ceiling": `{"revision":"` + strings.Repeat("a", 64) + `","decision":{"allow":false,"reason":"denied by Context policy ceiling","status_code":403,"learnable":false}}`,
		"stale revision": `{"revision":"` + strings.Repeat("c", 64) + `","decision":{"allow":true,"reason":"allowed by Context policy","status_code":403,"learnable":false}}`,
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			runtime, _, record := permissionObserverRuntime(t, document)
			result, terminal, err := runtime.ObservePermissionDisposition(context.Background(), record)
			if err != nil || terminal || result != "" {
				t.Fatalf("nonterminal observation = %q, %t, %v", result, terminal, err)
			}
		})
	}
}

func TestPermissionObserverFailsOnUnavailableOrMalformedOPA(t *testing.T) {
	runtime, runner, record := permissionObserverRuntime(t, `{}`)
	if _, _, err := runtime.ObservePermissionDisposition(context.Background(), record); err == nil {
		t.Fatal("malformed OPA output passed")
	}
	runner.output = []byte("canary-private-output")
	runner.err = os.ErrDeadlineExceeded
	if _, _, err := runtime.ObservePermissionDisposition(context.Background(), record); err == nil || strings.Contains(err.Error(), "canary-private-output") {
		t.Fatalf("OPA failure = %v", err)
	}
}

func TestPermissionPolicyObservationRequiresExactDocumentEnd(t *testing.T) {
	valid := `{"revision":"` + strings.Repeat("a", 64) + `","decision":{"allow":true,"reason":"allowed by Context policy","status_code":403,"learnable":false}}`
	for name, document := range map[string]string{
		"whitespace":     valid + " \n\t",
		"second value":   valid + ` {}`,
		"malformed tail": valid + ` {`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parsePermissionPolicyObservation([]byte(document))
			if name == "whitespace" && err != nil {
				t.Fatalf("valid whitespace rejected: %v", err)
			}
			if name != "whitespace" && err == nil {
				t.Fatal("trailing OPA data passed")
			}
		})
	}
}

func TestPermissionObserverUsesBoundGatewaySegmentsWithoutSiblingReconstruction(t *testing.T) {
	record := permissionWaitRecordFixtureForInfra()
	record.Effect.Path = "/a%2Fb//./../%E3%81%82"
	record.Effect.Segments = []string{"a/b", ".", "..", "あ"}
	input, err := permissionPolicyInput(record)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(input)
	if !strings.Contains(string(encoded), `"raw":"/a%2Fb//./../%E3%81%82"`) || !strings.Contains(string(encoded), `"segments":["a/b",".","..","あ"]`) {
		t.Fatalf("policy input diverged from bound effect: %s", encoded)
	}
	for _, path := range []string{"/bad%", "/bad%ff"} {
		record.Effect.Path = path
		record.Effect.Segments = []string{"bad"}
		if _, err := permissionPolicyInput(record); err == nil {
			t.Fatalf("unrepresentable Gateway path accepted: %q", path)
		}
	}
}
