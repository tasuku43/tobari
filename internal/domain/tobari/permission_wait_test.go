package tobari

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func permissionWaitRecordFixture() PermissionWaitRecord {
	return PermissionWaitRecord{
		SchemaVersion:              PermissionWaitRecordSchema,
		ID:                         "pwt_0123456789abcdef0123456789abcdef",
		DenialCorrelationID:        "abcdef0123456789abcdef0123456789",
		FrozenPrincipalFingerprint: strings.Repeat("a", 64),
		WorkspaceManifestID:        "018f3f18-7a3b-7abc-8def-0123456789ab",
		WorkspaceID:                "018f3f18-7a3b-7abc-8def-0123456789ac",
		AttachmentID:               "att_0123456789abcdef0123456789abcdef",
		Effect: PermissionWaitEffect{
			Scheme: "https", Host: "api.example.com", Port: 443,
			Method: "POST", Path: "/v1/items/42",
		},
		CreatedAt: "2026-08-23T00:00:00Z",
		ExpiresAt: "2026-08-23T00:15:00Z",
	}
}

func TestPermissionWaitIDUsesExactCorrelationShape(t *testing.T) {
	id, err := NewPermissionWaitID(bytes.NewReader(make([]byte, 16)))
	if err != nil {
		t.Fatal(err)
	}
	if id != "pwt_00000000000000000000000000000000" || len(id) != 36 {
		t.Fatalf("ID = %q", id)
	}
	for _, value := range []string{
		"", "pwt_0123456789abcdef0123456789abcde", "pwt_0123456789abcdef0123456789abcdef0",
		"pwt_0123456789ABCDEF0123456789ABCDEF", "pcy_0123456789abcdef0123456789abcdef",
	} {
		if ValidatePermissionWaitID(value) == nil {
			t.Fatalf("invalid wait ID accepted: %q", value)
		}
	}
}

func TestPermissionWaitRecordFixesExactOrdinaryEffectAndLease(t *testing.T) {
	fixture := permissionWaitRecordFixture()
	if err := fixture.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"context_id", "project_id", "workspace_manifest\"", "project_root", "query", "header", "body", "credential", "candidate", "policy_revision"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("record exposes forbidden field %q: %s", forbidden, encoded)
		}
	}

	mutations := map[string]func(*PermissionWaitRecord){
		"schema":         func(r *PermissionWaitRecord) { r.SchemaVersion = 1 },
		"id":             func(r *PermissionWaitRecord) { r.ID = "invalid" },
		"correlation":    func(r *PermissionWaitRecord) { r.DenialCorrelationID = "invalid" },
		"principal":      func(r *PermissionWaitRecord) { r.FrozenPrincipalFingerprint = "invalid" },
		"manifest":       func(r *PermissionWaitRecord) { r.WorkspaceManifestID = "invalid" },
		"workspace":      func(r *PermissionWaitRecord) { r.WorkspaceID = "invalid" },
		"attachment":     func(r *PermissionWaitRecord) { r.AttachmentID = "invalid" },
		"host loopback":  func(r *PermissionWaitRecord) { r.Effect.Host = HostLoopbackHostname },
		"protocol host":  func(r *PermissionWaitRecord) { r.Effect.Host = "API.Example.com" },
		"overlong lease": func(r *PermissionWaitRecord) { r.ExpiresAt = "2026-08-23T00:15:00.000000001Z" },
		"reversed lease": func(r *PermissionWaitRecord) { r.ExpiresAt = r.CreatedAt },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := fixture
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid record passed validation")
			}
		})
	}
}

func TestPermissionWaitExpiryAndClosedResults(t *testing.T) {
	record := permissionWaitRecordFixture()
	for _, test := range []struct {
		now  string
		want bool
	}{
		{"2026-08-23T00:14:59.999999999Z", false},
		{"2026-08-23T00:15:00Z", true},
	} {
		now, _ := time.Parse(time.RFC3339Nano, test.now)
		got, err := record.Expired(now)
		if err != nil || got != test.want {
			t.Fatalf("Expired(%s) = %t, %v", test.now, got, err)
		}
	}
	for _, result := range []PermissionWaitResult{PermissionWaitResultAllow, PermissionWaitResultDeny, PermissionWaitResultExpired} {
		if err := result.Validate(); err != nil {
			t.Fatalf("valid result %q: %v", result, err)
		}
	}
	if err := PermissionWaitResult("pending").Validate(); err == nil {
		t.Fatal("public pending result passed validation")
	}
}

func TestPermissionWaitAttemptStateCountsReconnectWithoutFalseConsumption(t *testing.T) {
	state := PermissionWaitAccessState{}
	for attempt := 1; attempt <= PermissionWaitMaxAttempts; attempt++ {
		active, err := state.StartAttempt()
		if err != nil || !active.Active || active.Attempts != attempt || active.Consumed {
			t.Fatalf("start attempt %d = %+v, %v", attempt, active, err)
		}
		state, err = active.FinishAttempt(false)
		if err != nil || state.Active || state.Consumed {
			t.Fatalf("transport loss %d = %+v, %v", attempt, state, err)
		}
	}
	if _, err := state.StartAttempt(); err == nil {
		t.Fatal("fourth attempt was accepted")
	}

	active, err := (PermissionWaitAccessState{}).StartAttempt()
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := active.FinishAttempt(true)
	if err != nil || !consumed.Consumed || consumed.Active {
		t.Fatalf("terminal finish = %+v, %v", consumed, err)
	}
	if _, err := consumed.StartAttempt(); err == nil {
		t.Fatal("consumed record was reused")
	}
}
