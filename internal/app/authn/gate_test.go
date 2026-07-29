package authn

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	domainauthn "github.com/tasuku43/agentic-cli-foundry/internal/domain/authn"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
)

type authenticatorStub struct {
	session domainauthn.Session
	err     error
	calls   int
	seen    domainauthn.Requirement
	mutate  func(*domainauthn.Requirement)
}

type typedNilAuthenticator struct{}

func (*typedNilAuthenticator) Authenticate(context.Context, domainauthn.Requirement) (domainauthn.Session, error) {
	panic("typed nil authenticator must not be called")
}

type bindingTaskPort struct {
	records       map[domainauthn.BindingID]string
	refreshErr    error
	seenBinding   domainauthn.BindingID
	providerCalls int
}

func (p *bindingTaskPort) Read(ctx context.Context, binding domainauthn.BindingID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.seenBinding = binding
	if _, exists := p.records[binding]; !exists {
		return fault.New(fault.KindAuthentication, "authentication_binding_not_found", "authentication binding is unavailable", false)
	}
	if p.refreshErr != nil {
		return p.refreshErr
	}
	p.providerCalls++
	return nil
}

func (a *authenticatorStub) Authenticate(_ context.Context, requirement domainauthn.Requirement) (domainauthn.Session, error) {
	a.calls++
	a.seen = requirement.Clone()
	if a.mutate != nil {
		a.mutate(&requirement)
	}
	return a.session.Clone(), a.err
}

func appRequirement() domainauthn.Requirement {
	return domainauthn.Requirement{
		Methods:              []domainauthn.Method{domainauthn.MethodOAuth2, domainauthn.MethodPAT},
		Authority:            "example-authority",
		Audience:             "example-api",
		AccountID:            "account-1",
		RequiredCapabilities: []string{"items:read"},
	}
}

func appSession() domainauthn.Session {
	bindingID, err := domainauthn.NewBindingID("ephemeral-binding-1")
	if err != nil {
		panic(err)
	}
	return domainauthn.Session{
		Method:              domainauthn.MethodOAuth2,
		Authority:           "example-authority",
		Audience:            "example-api",
		SubjectID:           "subject-1",
		AccountID:           "account-1",
		BindingID:           bindingID,
		GrantedCapabilities: []string{"items:read"},
		ExpiresAt:           time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func newTestGate(authenticator Authenticator) *Gate {
	gate := New(authenticator)
	gate.now = func() time.Time { return time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC) }
	return gate
}

func TestGateAuthenticatesRevalidatesAndCallsActionOnce(t *testing.T) {
	authenticator := &authenticatorStub{session: appSession()}
	wantBinding := appSession().BindingID
	actionCalls := 0
	err := newTestGate(authenticator).Invoke(context.Background(), appRequirement(), func(_ context.Context, session domainauthn.Session) error {
		actionCalls++
		if session.SubjectID != "subject-1" || session.Method != domainauthn.MethodOAuth2 || session.BindingID != wantBinding {
			t.Fatalf("session = %+v", session)
		}
		session.GrantedCapabilities[0] = "mutated"
		return nil
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if authenticator.calls != 1 || actionCalls != 1 {
		t.Fatalf("authenticator calls = %d, action calls = %d", authenticator.calls, actionCalls)
	}
}

func TestGateSnapshotsRequirementAcrossUntrustedAuthenticator(t *testing.T) {
	requirement := appRequirement()
	authenticator := &authenticatorStub{
		session: appSession(),
		mutate: func(got *domainauthn.Requirement) {
			got.Methods[0] = domainauthn.MethodPAT
			got.RequiredCapabilities[0] = "changed"
		},
	}
	if err := newTestGate(authenticator).Invoke(context.Background(), requirement, func(context.Context, domainauthn.Session) error { return nil }); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if requirement.Methods[0] != domainauthn.MethodOAuth2 || requirement.RequiredCapabilities[0] != "items:read" {
		t.Fatal("caller requirement was mutated")
	}
}

func TestGatePassesBindingUnchangedAcrossSimultaneousAccountRecords(t *testing.T) {
	session := appSession()
	otherBinding, err := domainauthn.NewBindingID("ephemeral-binding-2")
	if err != nil {
		t.Fatal(err)
	}
	port := &bindingTaskPort{records: map[domainauthn.BindingID]string{
		session.BindingID: "account-1",
		otherBinding:      "account-2",
	}}
	err = newTestGate(&authenticatorStub{session: session}).Invoke(context.Background(), appRequirement(), func(ctx context.Context, validated domainauthn.Session) error {
		return port.Read(ctx, validated.BindingID)
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if port.seenBinding != session.BindingID || port.seenBinding == otherBinding ||
		port.records[port.seenBinding] != "account-1" || port.providerCalls != 1 {
		t.Fatalf("seen binding selected wrong account record; provider calls = %d", port.providerCalls)
	}
}

func TestGatePreservesIOTimeRefreshFailureBeforeProviderRequest(t *testing.T) {
	const canary = "raw-refresh-token-canary"
	session := appSession()
	port := &bindingTaskPort{
		records: map[domainauthn.BindingID]string{session.BindingID: "account-1"},
		refreshErr: fault.Wrap(
			fault.KindAuthentication,
			"token_refresh_failed",
			"authentication refresh failed",
			false,
			errors.New(canary),
		),
	}
	err := newTestGate(&authenticatorStub{session: session}).Invoke(context.Background(), appRequirement(), func(ctx context.Context, validated domainauthn.Session) error {
		return port.Read(ctx, validated.BindingID)
	})
	structured := assertStructuredFault(t, err)
	if port.providerCalls != 0 || structured.Kind != fault.KindAuthentication || structured.Code != "token_refresh_failed" {
		t.Fatalf("provider calls = %d, fault = %+v", port.providerCalls, structured)
	}
	if errors.Unwrap(structured) != nil {
		t.Fatalf("refresh failure exposed private cause: %#v", structured)
	}
	assertNoSecretRendering(t, err, canary)
}

func TestGateFailuresBeforeAuthenticationMakeZeroCalls(t *testing.T) {
	validAction := func(context.Context, domainauthn.Session) error { return nil }
	tests := []struct {
		name        string
		gate        *Gate
		ctx         context.Context
		requirement domainauthn.Requirement
		action      Action
	}{
		{name: "nil gate", ctx: context.Background(), requirement: appRequirement(), action: validAction},
		{name: "nil context", gate: newTestGate(&authenticatorStub{session: appSession()}), requirement: appRequirement(), action: validAction},
		{name: "nil action", gate: newTestGate(&authenticatorStub{session: appSession()}), ctx: context.Background(), requirement: appRequirement()},
		{name: "zero requirement", gate: newTestGate(&authenticatorStub{session: appSession()}), ctx: context.Background(), action: validAction},
		{name: "nil authenticator", gate: newTestGate(nil), ctx: context.Background(), requirement: appRequirement(), action: validAction},
		{name: "typed nil authenticator", gate: newTestGate((*typedNilAuthenticator)(nil)), ctx: context.Background(), requirement: appRequirement(), action: validAction},
		{name: "nil clock", gate: &Gate{authenticator: &authenticatorStub{session: appSession()}}, ctx: context.Background(), requirement: appRequirement(), action: validAction},
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	tests = append(tests, struct {
		name        string
		gate        *Gate
		ctx         context.Context
		requirement domainauthn.Requirement
		action      Action
	}{name: "canceled", gate: newTestGate(&authenticatorStub{session: appSession()}), ctx: canceled, requirement: appRequirement(), action: validAction})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authCalls := 0
			if test.gate != nil && test.gate.authenticator != nil {
				if stub, ok := test.gate.authenticator.(*authenticatorStub); ok {
					stub.calls = 0
					defer func() { authCalls = stub.calls }()
				}
			}
			actionCalls := 0
			action := test.action
			if action != nil {
				action = func(ctx context.Context, session domainauthn.Session) error {
					actionCalls++
					return test.action(ctx, session)
				}
			}
			err := test.gate.Invoke(test.ctx, test.requirement, action)
			if err == nil {
				t.Fatal("Invoke() succeeded")
			}
			if test.gate != nil {
				if stub, ok := test.gate.authenticator.(*authenticatorStub); ok {
					authCalls = stub.calls
				}
			}
			if authCalls != 0 || actionCalls != 0 {
				t.Fatalf("authenticator calls = %d, action calls = %d; want zero", authCalls, actionCalls)
			}
			assertStructuredFault(t, err)
		})
	}
}

func TestGateAuthenticationFailureAndCancellationMakeZeroActionCalls(t *testing.T) {
	const canary = "raw-token-canary"
	missingBinding := appSession()
	missingBinding.BindingID = domainauthn.BindingID{}
	tests := []struct {
		name          string
		authenticator *authenticatorStub
		wantKind      fault.Kind
		wantCode      string
	}{
		{name: "unstructured error", authenticator: &authenticatorStub{err: errors.New("provider rejected " + canary)}, wantKind: fault.KindAuthentication, wantCode: "authentication_failed"},
		{name: "structured unavailable strips cause", authenticator: &authenticatorStub{err: fault.Wrap(fault.KindUnavailable, "identity_service_unavailable", "identity service is unavailable", true, errors.New(canary))}, wantKind: fault.KindUnavailable, wantCode: "identity_service_unavailable"},
		{name: "refresh authentication failure strips cause", authenticator: &authenticatorStub{err: fault.Wrap(fault.KindAuthentication, "token_refresh_failed", "authentication refresh failed", false, errors.New(canary))}, wantKind: fault.KindAuthentication, wantCode: "token_refresh_failed"},
		{name: "provider permission failure remains permission", authenticator: &authenticatorStub{err: fault.Wrap(fault.KindPermission, "credential_permission_denied", "credential permission was denied", false, errors.New(canary))}, wantKind: fault.KindPermission, wantCode: "credential_permission_denied"},
		{name: "disallowed structured kind collapses", authenticator: &authenticatorStub{err: fault.Wrap(fault.KindInternal, "provider_internal", "provider internal failure", false, errors.New(canary))}, wantKind: fault.KindAuthentication, wantCode: "authentication_failed"},
		{name: "invalid session", authenticator: &authenticatorStub{session: domainauthn.Session{SubjectID: canary + "\n"}}, wantKind: fault.KindAuthentication, wantCode: "invalid_authentication_session"},
		{name: "missing ephemeral binding", authenticator: &authenticatorStub{session: missingBinding}, wantKind: fault.KindAuthentication, wantCode: "invalid_authentication_session"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actionCalls := 0
			err := newTestGate(test.authenticator).Invoke(context.Background(), appRequirement(), func(context.Context, domainauthn.Session) error {
				actionCalls++
				return nil
			})
			if err == nil || actionCalls != 0 {
				t.Fatalf("error = %v, action calls = %d", err, actionCalls)
			}
			structured := assertStructuredFault(t, err)
			if structured.Kind != test.wantKind || structured.Code != test.wantCode {
				t.Fatalf("fault = %+v, want kind %q code %q", structured, test.wantKind, test.wantCode)
			}
			assertNoSecretRendering(t, err, canary)
		})
	}

	canceled, cancel := context.WithCancel(context.Background())
	authenticator := &cancelingAuthenticator{cancel: cancel, session: appSession()}
	actionCalls := 0
	err := newTestGate(authenticator).Invoke(canceled, appRequirement(), func(context.Context, domainauthn.Session) error {
		actionCalls++
		return nil
	})
	if err == nil || authenticator.calls != 1 || actionCalls != 0 {
		t.Fatalf("error = %v, authenticator calls = %d, action calls = %d", err, authenticator.calls, actionCalls)
	}
	if got := assertStructuredFault(t, err); got.Kind != fault.KindCanceled {
		t.Fatalf("fault kind = %q, want canceled", got.Kind)
	}
}

type cancelingAuthenticator struct {
	cancel  context.CancelFunc
	session domainauthn.Session
	calls   int
}

func (a *cancelingAuthenticator) Authenticate(_ context.Context, _ domainauthn.Requirement) (domainauthn.Session, error) {
	a.calls++
	a.cancel()
	return a.session, nil
}

func TestGateMismatchClassesMakeZeroActionCalls(t *testing.T) {
	tests := []struct {
		name     string
		edit     func(*domainauthn.Session)
		wantKind fault.Kind
		wantCode string
	}{
		{name: "method", edit: func(session *domainauthn.Session) {
			session.Method = domainauthn.MethodPAT
			session.ExpiresAt = time.Time{}
		}, wantKind: fault.KindAuthentication, wantCode: "authentication_context_mismatch"},
		{name: "authority", edit: func(session *domainauthn.Session) { session.Authority = "other-authority" }, wantKind: fault.KindAuthentication, wantCode: "authentication_context_mismatch"},
		{name: "audience", edit: func(session *domainauthn.Session) { session.Audience = "other-api" }, wantKind: fault.KindAuthentication, wantCode: "authentication_context_mismatch"},
		{name: "account", edit: func(session *domainauthn.Session) { session.AccountID = "account-2" }, wantKind: fault.KindAuthentication, wantCode: "authentication_context_mismatch"},
		{name: "capability", edit: func(session *domainauthn.Session) { session.GrantedCapabilities = nil }, wantKind: fault.KindPermission, wantCode: "insufficient_authentication_capability"},
		{name: "expired", edit: func(session *domainauthn.Session) { session.ExpiresAt = time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC) }, wantKind: fault.KindAuthentication, wantCode: "authentication_expired"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirement := appRequirement()
			if test.name == "method" {
				requirement.Methods = []domainauthn.Method{domainauthn.MethodOAuth2}
			}
			session := appSession()
			test.edit(&session)
			actionCalls := 0
			err := newTestGate(&authenticatorStub{session: session}).Invoke(context.Background(), requirement, func(context.Context, domainauthn.Session) error {
				actionCalls++
				return nil
			})
			if err == nil || actionCalls != 0 {
				t.Fatalf("error = %v, action calls = %d", err, actionCalls)
			}
			structured := assertStructuredFault(t, err)
			if structured.Kind != test.wantKind || structured.Code != test.wantCode {
				t.Fatalf("fault = %+v, want kind %q code %q", structured, test.wantKind, test.wantCode)
			}
		})
	}
}

func TestGateSanitizesUnclassifiedActionError(t *testing.T) {
	const canary = "raw-token-canary"
	err := newTestGate(&authenticatorStub{session: appSession()}).Invoke(context.Background(), appRequirement(), func(context.Context, domainauthn.Session) error {
		return errors.New("request failed with Authorization: Bearer " + canary)
	})
	structured := assertStructuredFault(t, err)
	if structured.Code != "unclassified_authenticated_action_error" {
		t.Fatalf("fault = %#v", err)
	}
	assertNoSecretRendering(t, err, canary)
}

func TestGatePreservesSafeActionClassificationAndStripsRefreshCauses(t *testing.T) {
	const canary = "raw-refresh-token-canary"
	tests := []struct {
		name      string
		kind      fault.Kind
		code      string
		message   string
		retryable bool
	}{
		{name: "refresh authentication failure", kind: fault.KindAuthentication, code: "token_refresh_failed", message: "authentication refresh failed"},
		{name: "API permission failure", kind: fault.KindPermission, code: "api_permission_denied", message: "API permission was denied"},
		{name: "temporary refresh failure", kind: fault.KindUnavailable, code: "refresh_service_unavailable", message: "authentication refresh is unavailable", retryable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actionCalls := 0
			err := newTestGate(&authenticatorStub{session: appSession()}).Invoke(context.Background(), appRequirement(), func(context.Context, domainauthn.Session) error {
				actionCalls++
				return fault.Wrap(test.kind, test.code, test.message, test.retryable, errors.New(canary))
			})
			structured := assertStructuredFault(t, err)
			if actionCalls != 1 || structured.Kind != test.kind || structured.Code != test.code || structured.Retryable != test.retryable {
				t.Fatalf("action calls = %d, fault = %+v", actionCalls, structured)
			}
			if errors.Unwrap(structured) != nil {
				t.Fatalf("fault exposed refresh cause: %#v", structured)
			}
			assertNoSecretRendering(t, err, canary)
		})
	}
}

func TestGatePreservesClassifiedActionFailureWhenContextIsAlsoCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := newTestGate(&authenticatorStub{session: appSession()}).Invoke(ctx, appRequirement(), func(context.Context, domainauthn.Session) error {
		cancel()
		return fault.New(fault.KindPermission, "api_permission_denied", "API permission was denied", false)
	})
	structured := assertStructuredFault(t, err)
	if structured.Kind != fault.KindPermission || structured.Code != "api_permission_denied" {
		t.Fatalf("fault = %+v, want classified permission failure", structured)
	}
}

func TestGateMapsUnclassifiedCanceledActionToCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := newTestGate(&authenticatorStub{session: appSession()}).Invoke(ctx, appRequirement(), func(context.Context, domainauthn.Session) error {
		cancel()
		return errors.New("transport stopped")
	})
	structured := assertStructuredFault(t, err)
	if structured.Kind != fault.KindCanceled || structured.Code != "authentication_canceled" {
		t.Fatalf("fault = %+v, want canceled", structured)
	}
}

func assertStructuredFault(t *testing.T, err error) *fault.Error {
	t.Helper()
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Validate() != nil {
		t.Fatalf("error = %#v, want valid structured fault", err)
	}
	return structured
}

func assertNoSecretRendering(t *testing.T, err error, canary string) {
	t.Helper()
	for _, rendered := range []string{
		err.Error(),
		fmt.Sprint(err),
		fmt.Sprintf("%+v", err),
		fmt.Sprintf("%#v", err),
	} {
		if strings.Contains(rendered, canary) {
			t.Fatalf("public fault formatting exposed secret canary: %s", rendered)
		}
	}
}
