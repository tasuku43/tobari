package permissionwaitcmd

import (
	"context"
	"errors"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type observerStub struct {
	called int
	ctx    context.Context
	result tobari.PermissionWaitResult
	err    error
}

func (s *observerStub) WaitPermission(ctx context.Context, _ string) (tobari.PermissionWaitResult, error) {
	s.called++
	s.ctx = ctx
	return s.result, s.err
}

func hasFaultCode(err error, code string) bool {
	var structured *fault.Error
	return errors.As(err, &structured) && structured.Code == code
}

func TestWaitValidatesBeforeObservationAndPropagatesContext(t *testing.T) {
	observer := &observerStub{result: tobari.PermissionWaitResultAllow}
	service := New(observer)
	if _, err := service.Wait(context.Background(), "invalid"); !hasFaultCode(err, "invalid_permission_wait") || observer.called != 0 {
		t.Fatalf("invalid wait = %v, calls=%d", err, observer.called)
	}

	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "sentinel")
	result, err := service.Wait(ctx, "pwt_0123456789abcdef0123456789abcdef")
	if err != nil || result != tobari.PermissionWaitResultAllow || observer.called != 1 || observer.ctx.Value(key{}) != "sentinel" {
		t.Fatalf("Wait() = %q, %v, calls=%d, context=%v", result, err, observer.called, observer.ctx.Value(key{}))
	}
}

func TestWaitRejectsMissingObserverAndInvalidResult(t *testing.T) {
	if _, err := New(nil).Wait(context.Background(), "pwt_0123456789abcdef0123456789abcdef"); !hasFaultCode(err, "missing_permission_wait_observer") {
		t.Fatalf("missing observer error = %v", err)
	}
	observer := &observerStub{result: "pending"}
	if _, err := New(observer).Wait(context.Background(), "pwt_0123456789abcdef0123456789abcdef"); !hasFaultCode(err, "invalid_permission_wait_result") {
		t.Fatalf("invalid result error = %v", err)
	}
}

func TestWaitPreservesObserverFault(t *testing.T) {
	want := errors.New("transport unavailable")
	observer := &observerStub{err: want}
	if _, err := New(observer).Wait(context.Background(), "pwt_0123456789abcdef0123456789abcdef"); !errors.Is(err, want) {
		t.Fatalf("observer fault = %v", err)
	}
}
