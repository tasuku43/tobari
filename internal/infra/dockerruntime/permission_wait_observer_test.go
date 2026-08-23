package dockerruntime

import (
	"context"
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
		Effect:                     tobari.PermissionWaitEffect{Scheme: "https", Host: "api.example.com", Port: 443, Method: "GET", Path: "/items/a%20b"},
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
			for _, required := range []string{"/v1/data/tobari/aggregate_revision", "/v1/data/tobari/http/decision", `"context_id":"` + record.WorkspaceManifestID, `"project_id":"` + record.WorkspaceID, `"segments":["items","a b"]`} {
				if !strings.Contains(query, required) {
					t.Fatalf("OPA query omitted %q: %s", required, query)
				}
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
