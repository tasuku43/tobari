package dockerruntime

import (
	"archive/tar"
	"bytes"
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

func permissionObserverRuntime(t *testing.T) (*Runtime, *permissionObserverRunner, tobari.PermissionWaitRecord, string) {
	t.Helper()
	runtime, _, collection := finalPolicyActivationFixture(t)
	if _, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err != nil {
		t.Fatal(err)
	}
	active, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	runner := &permissionObserverRunner{}
	runtime.runner = runner
	record := permissionWaitRecordFixtureForInfra()
	return runtime, runner, record, active.Aggregate.AggregateRevision
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
			runtime, runner, record, revision := permissionObserverRuntime(t)
			runner.output = []byte(`{"revision":"` + revision + `","decision":` + document + `}`)
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
			for _, required := range []string{"/v1/data/tobari/http/permission_wait_observation", `"schema_version":2`, `"context_id":"` + record.WorkspaceManifestID, `"project_id":"` + record.WorkspaceID, `"segments":["items","a b"]`, `result := observation.body.result`} {
				if !strings.Contains(query, required) {
					t.Fatalf("OPA query omitted %q: %s", required, query)
				}
			}
			if strings.Count(query, "http.send(") != 1 || strings.Contains(query, "/v1/data/tobari/aggregate_revision") || strings.Contains(query, "/v1/data/tobari/http/decision\"") {
				t.Fatalf("permission observation was split across OPA snapshots: %s", query)
			}
			if len(runner.args) == 0 || !strings.HasPrefix(runner.args[len(runner.args)-1], "[result | ") ||
				!strings.HasSuffix(runner.args[len(runner.args)-1], "][0]") {
				t.Fatalf("OPA raw query does not emit the bound observation object: %s", query)
			}
		})
	}
}

func TestPermissionObserverKeepsDefaultDenyAndStaleRevisionNonterminal(t *testing.T) {
	tests := map[string]struct {
		decision string
		stale    bool
	}{
		"default deny":   {decision: `{"allow":false,"reason":"request did not match an allow rule","status_code":403,"learnable":true}`},
		"method ceiling": {decision: `{"allow":false,"reason":"denied by Context policy ceiling","status_code":403,"learnable":false}`},
		"stale revision": {decision: `{"allow":true,"reason":"allowed by Context policy","status_code":403,"learnable":false}`, stale: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runtime, runner, record, revision := permissionObserverRuntime(t)
			if test.stale {
				revision = strings.Repeat("c", 64)
			}
			runner.output = []byte(`{"revision":"` + revision + `","decision":` + test.decision + `}`)
			result, terminal, err := runtime.ObservePermissionDisposition(context.Background(), record)
			if err != nil || terminal || result != "" {
				t.Fatalf("nonterminal observation = %q, %t, %v", result, terminal, err)
			}
		})
	}
}

func TestPermissionObserverHotReloadCannotMixRevisionAndDecisionSnapshots(t *testing.T) {
	runtime, runner, record, revisionA := permissionObserverRuntime(t)
	revisionB := strings.Repeat("b", 64)
	if revisionB == revisionA {
		revisionB = strings.Repeat("c", 64)
	}
	allow := `{"allow":true,"reason":"allowed by Context policy","status_code":403,"learnable":false}`
	defaultDeny := `{"allow":false,"reason":"request did not match an allow rule","status_code":403,"learnable":true}`

	// This is the deterministic equivalent of reloading A to B in the old gap
	// between two OPA calls: the one endpoint can return only the complete B
	// snapshot, which cannot match the preloaded A state.
	runner.output = []byte(`{"revision":"` + revisionB + `","decision":` + allow + `}`)
	if result, terminal, err := runtime.ObservePermissionDisposition(context.Background(), record); err != nil || terminal || result != "" {
		t.Fatalf("unpublished B observation = %q, %t, %v", result, terminal, err)
	}
	if strings.Count(strings.Join(runner.args, " "), "http.send(") != 1 {
		t.Fatalf("hot-reload observation used multiple OPA snapshots: %v", runner.args)
	}

	// A failed Apply returning to A remains pending without an explicit final
	// disposition.
	runner.output = []byte(`{"revision":"` + revisionA + `","decision":` + defaultDeny + `}`)
	if result, terminal, err := runtime.ObservePermissionDisposition(context.Background(), record); err != nil || terminal || result != "" {
		t.Fatalf("failed Apply observation = %q, %t, %v", result, terminal, err)
	}

	for name, terminalDecision := range map[string]struct {
		document string
		result   tobari.PermissionWaitResult
	}{
		"allow": {document: allow, result: tobari.PermissionWaitResultAllow},
		"deny":  {document: `{"allow":false,"reason":"denied by exact policy","status_code":403,"learnable":false}`, result: tobari.PermissionWaitResultDeny},
	} {
		t.Run(name, func(t *testing.T) {
			runner.output = []byte(`{"revision":"` + revisionA + `","decision":` + terminalDecision.document + `}`)
			if result, terminal, err := runtime.ObservePermissionDisposition(context.Background(), record); err != nil || !terminal || result != terminalDecision.result {
				t.Fatalf("published active observation = %q, %t, %v", result, terminal, err)
			}
		})
	}
}

func TestPolicyTransitionFencePublishesAtomicPermissionWaitObservation(t *testing.T) {
	_, _, _, revision := permissionObserverRuntime(t)
	archive, _, err := policyFinalFenceArchive(revision)
	if err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(bytes.NewReader(archive))
	var rego []byte
	for {
		header, readErr := reader.Next()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
		contents, readErr := io.ReadAll(reader)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if header.Name == "fence.rego" {
			rego = contents
		}
	}
	if len(rego) == 0 {
		t.Fatal("transition fence archive omitted fence.rego")
	}
	want := `permission_wait_observation := {"revision": data.tobari.aggregate_revision, "decision": decision}`
	if strings.Count(string(rego), want) != 1 {
		t.Fatalf("transition fence omitted atomic permission observation:\n%s", rego)
	}
}

func TestPermissionObserverFailsOnUnavailableOrMalformedOPA(t *testing.T) {
	runtime, runner, record, _ := permissionObserverRuntime(t)
	runner.output = []byte(`{}`)
	if _, _, err := runtime.ObservePermissionDisposition(context.Background(), record); err == nil {
		t.Fatal("malformed OPA output passed")
	}
	runner.output = []byte("canary-private-output")
	runner.err = os.ErrDeadlineExceeded
	if _, _, err := runtime.ObservePermissionDisposition(context.Background(), record); err == nil || strings.Contains(err.Error(), "canary-private-output") {
		t.Fatalf("OPA failure = %v", err)
	}
}

func TestPermissionObserverFailsWhenActivePolicyStateIsUnavailable(t *testing.T) {
	runtime, _, record, _ := permissionObserverRuntime(t)
	if err := os.Remove(runtime.finalPolicyActiveReceiptPath()); err != nil {
		t.Fatal(err)
	}
	legacy := runtimeState(t.TempDir())
	if err := runtime.writeState(legacy); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtime.ObservePermissionDisposition(context.Background(), record); err == nil ||
		!strings.Contains(err.Error(), "active final policy authority is unavailable") {
		t.Fatalf("absent final authority with legacy state present = %v", err)
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
	if !strings.Contains(string(encoded), `"schema_version":2`) || !strings.Contains(string(encoded), `"raw":"/a%2Fb//./../%E3%81%82"`) || !strings.Contains(string(encoded), `"segments":["a/b",".","..","あ"]`) {
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
