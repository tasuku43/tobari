package dockerruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type dispositionObserverStub struct {
	results []tobari.PermissionWaitResult
	done    []bool
	err     error
	calls   int
	onCall  func()
}

func (s *dispositionObserverStub) ObservePermissionDisposition(context.Context, tobari.PermissionWaitRecord) (tobari.PermissionWaitResult, bool, error) {
	index := s.calls
	s.calls++
	if s.onCall != nil {
		s.onCall()
	}
	if s.err != nil {
		return "", false, s.err
	}
	if index >= len(s.done) {
		return "", false, nil
	}
	return s.results[index], s.done[index], nil
}

func permissionSessionFixture() tobari.InteractiveAttachmentSession {
	return tobari.InteractiveAttachmentSession{
		SchemaVersion:       tobari.PermissionSessionSchema,
		WorkspaceManifestID: "01912345-6789-7abc-8def-0123456789ad", WorkspaceID: "01912345-6789-7abc-8def-0123456789ab",
		AttachmentID: "att_0123456789abcdef0123456789abcdef", OwnerKind: tobari.PermissionSessionOwnerInteractive,
		FrozenPrincipalFingerprint: strings.Repeat("b", 64), OwnerPID: 42, IngestionTransport: tobari.PermissionSessionTransportUnix, IngestionEndpoint: "pws_0123456789abcdef0123456789abcdef.sock", IngestionNonce: strings.Repeat("c", 64),
		CreatedAt: "2026-08-23T00:00:00Z", LeaseIssuedAt: "2026-08-23T00:00:00Z", ExpiresAt: "2026-08-23T00:00:30Z",
	}
}

func TestPermissionWaitRegistryReturnsTerminalAndConsumes(t *testing.T) {
	observer := &dispositionObserverStub{results: []tobari.PermissionWaitResult{"", tobari.PermissionWaitResultAllow}, done: []bool{false, true}}
	registry, err := newPermissionWaitRegistry(permissionSessionFixture(), observer)
	if err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse(time.RFC3339Nano, "2026-08-23T00:00:01Z")
	registry.now = func() time.Time { return now }
	registry.wait = func(context.Context, time.Duration, <-chan struct{}) error { return nil }
	record := permissionWaitRecordFixtureForInfra()
	if err := registry.Register(record); err != nil {
		t.Fatal(err)
	}
	result, err := registry.WaitPermission(context.Background(), record.ID)
	if err != nil || result != tobari.PermissionWaitResultAllow || observer.calls != 2 {
		t.Fatalf("WaitPermission() = %q, %v, calls=%d", result, err, observer.calls)
	}
	if _, err := registry.WaitPermission(context.Background(), record.ID); !hasInfrastructureFaultCode(err, "invalid_permission_wait") {
		t.Fatalf("consumed lookup = %v", err)
	}
}

func TestPermissionWaitRegistryRejectsTerminalAcrossOwnerExpiry(t *testing.T) {
	now, _ := time.Parse(time.RFC3339Nano, "2026-08-23T00:00:01Z")
	observer := &dispositionObserverStub{results: []tobari.PermissionWaitResult{tobari.PermissionWaitResultAllow}, done: []bool{true}}
	registry, err := newPermissionWaitRegistry(permissionSessionFixture(), observer)
	if err != nil {
		t.Fatal(err)
	}
	registry.now = func() time.Time { return now }
	observer.onCall = func() { now = registry.ownerExpiry }
	record := permissionWaitRecordFixtureForInfra()
	if err := registry.Register(record); err != nil {
		t.Fatal(err)
	}
	result, err := registry.WaitPermission(context.Background(), record.ID)
	if result != "" || !hasInfrastructureFaultCode(err, "permission_wait_owner_unavailable") {
		t.Fatalf("terminal after owner expiry = %q, %v", result, err)
	}
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Retryable {
		t.Fatalf("owner-loss fault advertised replay permission: %+v", structured)
	}
}

func TestPermissionWaitRegistryExpiresInsteadOfAcceptingLateTerminal(t *testing.T) {
	now, _ := time.Parse(time.RFC3339Nano, "2026-08-23T00:00:01Z")
	observer := &dispositionObserverStub{results: []tobari.PermissionWaitResult{tobari.PermissionWaitResultDeny}, done: []bool{true}}
	registry, err := newPermissionWaitRegistry(permissionSessionFixture(), observer)
	if err != nil {
		t.Fatal(err)
	}
	registry.now = func() time.Time { return now }
	record := permissionWaitRecordFixtureForInfra()
	expires, _ := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	registry.ownerExpiry = expires.Add(time.Minute)
	observer.onCall = func() { now = expires }
	if err := registry.Register(record); err != nil {
		t.Fatal(err)
	}
	result, err := registry.WaitPermission(context.Background(), record.ID)
	if err != nil || result != tobari.PermissionWaitResultExpired {
		t.Fatalf("terminal after wait expiry = %q, %v", result, err)
	}
	if _, err := registry.WaitPermission(context.Background(), record.ID); !hasInfrastructureFaultCode(err, "invalid_permission_wait") {
		t.Fatalf("late terminal record was not consumed: %v", err)
	}
}

func TestPermissionWaitRegistryReconnectsWithoutConsumptionAndBoundsAttempts(t *testing.T) {
	observer := &dispositionObserverStub{}
	registry, _ := newPermissionWaitRegistry(permissionSessionFixture(), observer)
	now, _ := time.Parse(time.RFC3339Nano, "2026-08-23T00:00:01Z")
	registry.now = func() time.Time { return now }
	registry.wait = func(context.Context, time.Duration, <-chan struct{}) error { return context.Canceled }
	record := permissionWaitRecordFixtureForInfra()
	if err := registry.Register(record); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < tobari.PermissionWaitMaxAttempts; attempt++ {
		if _, err := registry.WaitPermission(context.Background(), record.ID); !hasInfrastructureFaultCode(err, "permission_wait_interrupted") {
			t.Fatalf("attempt %d = %v", attempt+1, err)
		}
	}
	if _, err := registry.WaitPermission(context.Background(), record.ID); !hasInfrastructureFaultCode(err, "invalid_permission_wait") {
		t.Fatalf("exhausted lookup = %v", err)
	}
}

func TestPermissionWaitRegistryRejectsCrossAttachmentAndCapacity(t *testing.T) {
	registry, _ := newPermissionWaitRegistry(permissionSessionFixture(), &dispositionObserverStub{})
	now, _ := time.Parse(time.RFC3339Nano, "2026-08-23T00:00:01Z")
	registry.now = func() time.Time { return now }
	foreign := permissionWaitRecordFixtureForInfra()
	foreign.AttachmentID = "att_ffffffffffffffffffffffffffffffff"
	if err := registry.Register(foreign); err == nil {
		t.Fatal("cross-attachment record was registered")
	}
	for index := 0; index < tobari.PermissionWaitMaxLive; index++ {
		record := permissionWaitRecordFixtureForInfra()
		record.ID = "pwt_" + strings.Repeat(string("0123456789abcdef"[index]), 32)
		if err := registry.Register(record); err != nil {
			t.Fatalf("register %d: %v", index, err)
		}
	}
	overflow := permissionWaitRecordFixtureForInfra()
	overflow.ID = "pwt_" + strings.Repeat("f", 32)
	if err := registry.Register(overflow); err == nil {
		t.Fatal("ninth live wait was registered")
	}
}

func TestPermissionWaitRegistryInvalidationWakesActiveWait(t *testing.T) {
	registry, err := newPermissionWaitRegistry(permissionSessionFixture(), &dispositionObserverStub{})
	if err != nil {
		t.Fatal(err)
	}
	now, _ := time.Parse(time.RFC3339Nano, "2026-08-23T00:00:01Z")
	registry.now = func() time.Time { return now }
	record := permissionWaitRecordFixtureForInfra()
	if err := registry.Register(record); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, waitErr := registry.WaitPermission(context.Background(), record.ID)
		done <- waitErr
	}()
	time.Sleep(20 * time.Millisecond)
	registry.Invalidate()
	select {
	case waitErr := <-done:
		if !hasInfrastructureFaultCode(waitErr, "permission_wait_owner_unavailable") {
			t.Fatalf("invalidated wait = %v", waitErr)
		}
	case <-time.After(time.Second):
		t.Fatal("invalidated wait remained blocked")
	}
	if err := registry.Register(record); !hasInfrastructureFaultCode(err, "permission_wait_owner_unavailable") {
		t.Fatalf("registration after invalidation = %v", err)
	}
}

func hasInfrastructureFaultCode(err error, code string) bool {
	var structured *fault.Error
	return errors.As(err, &structured) && structured.Code == code
}
