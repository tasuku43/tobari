package operatorconsole

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type backendFake struct {
	snapshot tobari.OperatorConsoleSnapshot
	applies  atomic.Int32
	change   tobari.PolicyReviewChange
	applyErr error
}

func (b *backendFake) Snapshot(context.Context) (tobari.OperatorConsoleSnapshot, error) {
	return b.snapshot, nil
}

func (b *backendFake) ApplyPolicyReview(
	context.Context, tobari.PolicyReviewDecisionSet,
) (tobari.PolicyReviewChange, error) {
	b.applies.Add(1)
	if b.applyErr != nil {
		return tobari.PolicyReviewChange{}, b.applyErr
	}
	return b.change, nil
}

func validSnapshot() tobari.OperatorConsoleSnapshot {
	return tobari.OperatorConsoleSnapshot{
		Task: tobari.TaskOperatorConsoleSnapshot,
		Cluster: tobari.ClusterStatus{
			Task: tobari.TaskClusterStatus, Configured: true, Running: true,
			Policy: "/tmp/policy", ContextCount: 1, PolicyRevision: strings.Repeat("a", 64),
			PolicyProjection: "valid", PrincipalRegistry: "valid", GatewayProjection: "valid",
			Components: []tobari.ComponentStatus{
				{Name: "gateway", State: "running", Health: "healthy"},
				{Name: "opa", State: "running", Health: "healthy"},
			},
		},
		Workspaces:  tobari.ProjectListResult{Task: tobari.TaskProjectList, Items: []tobari.ProjectListItem{}},
		WindowLines: 10_000,
		ReviewItems: []tobari.PolicyReviewItem{},
		Rules: tobari.PolicyRuleReport{
			Task: tobari.TaskPolicyRules, PolicyDirectory: "/tmp/policy", Items: []tobari.PolicyRule{},
		},
	}
}

func request(method, path, authority, token string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, "http://"+authority+path, body)
	r.Host = authority
	r.RemoteAddr = "127.0.0.1:49152"
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

func TestHandlerServesOnlySessionBoundLoopbackSurface(t *testing.T) {
	t.Parallel()
	backend := &backendFake{snapshot: validSnapshot(), applyErr: fault.New(
		fault.KindRejected, "policy_review_changed", "policy review changed", false,
	)}
	authority, token := "127.0.0.1:43117", strings.Repeat("s", 43)
	h := newHandler(backend, authority, "http://"+authority, token)

	t.Run("embedded asset has hardened headers and no cookie", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, request(http.MethodGet, "/", authority, "", nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Operator Console") {
			t.Fatalf("asset response = %d %q", recorder.Code, recorder.Body.String())
		}
		if recorder.Header().Get("Content-Security-Policy") != csp || recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("security headers = %#v", recorder.Header())
		}
		if values := recorder.Header().Values("Set-Cookie"); len(values) != 0 {
			t.Fatalf("operator console set cookies: %q", values)
		}
	})

	t.Run("API requires exact host peer and bearer", func(t *testing.T) {
		cases := []struct {
			name   string
			mutate func(*http.Request)
			status int
		}{
			{name: "missing bearer", status: http.StatusUnauthorized},
			{name: "wrong Host", mutate: func(r *http.Request) { r.Host = "localhost:43117" }, status: http.StatusForbidden},
			{name: "non-loopback peer", mutate: func(r *http.Request) { r.RemoteAddr = "192.0.2.9:4000" }, status: http.StatusForbidden},
		}
		for _, test := range cases {
			t.Run(test.name, func(t *testing.T) {
				recorder := httptest.NewRecorder()
				r := request(http.MethodGet, "/api/v1/snapshot", authority, token, nil)
				if test.name == "missing bearer" {
					r.Header.Del("Authorization")
				}
				if test.mutate != nil {
					test.mutate(r)
				}
				h.ServeHTTP(recorder, r)
				if recorder.Code != test.status {
					t.Fatalf("status = %d, want %d", recorder.Code, test.status)
				}
			})
		}
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, request(http.MethodGet, "/api/v1/snapshot", authority, token, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"task":"serve.snapshot"`) {
			t.Fatalf("snapshot response = %d %q", recorder.Code, recorder.Body.String())
		}
	})
}

func TestPolicyApplyReturnsAuthoritativeValidatedReceipt(t *testing.T) {
	t.Parallel()
	denial := tobari.PolicyDenial{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Timestamp:              "2026-08-18T01:00:00Z", RequestID: "0123456789abcdef0123456789abcdef",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "toolbox",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
		Host: "api.example.com", Port: 443, Method: "GET", Path: "/v1/models",
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	}
	candidate, err := tobari.NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	change := tobari.PolicyReviewChange{
		Task: tobari.TaskPolicyReviewApply, PolicyDirectory: "/tmp/policy",
		AllowCount: 1, Applied: true, ActiveRevision: strings.Repeat("b", 64),
		Decisions: []tobari.PolicyReviewAppliedDecision{{
			PolicyProtocolIdentity: candidate.PolicyProtocolIdentity,
			RuleID:                 rule.ID, ReviewItemID: candidate.ID,
			Decision: tobari.PolicyDecisionAllow, Match: tobari.PolicyMatchExact,
			ContextID: candidate.ContextID, ContextName: candidate.ContextName,
			ProjectID: candidate.ProjectID, ProjectRoot: candidate.ProjectRoot,
			Host: candidate.Host, Port: candidate.Port, Method: candidate.Method, Path: candidate.Path,
			SourceCandidates: []string{candidate.ID},
		}},
	}
	if err := change.Validate(); err != nil {
		t.Fatal(err)
	}
	backend := &backendFake{snapshot: validSnapshot(), change: change}
	authority, token := "127.0.0.1:43120", strings.Repeat("u", 43)
	origin := "http://" + authority
	h := newHandler(backend, authority, origin, token)
	payload := []byte(fmt.Sprintf(
		`{"decisions":[{"review_item_id":%q,"decision":"allow","match":"exact"}]}`,
		candidate.ID,
	))
	recorder := httptest.NewRecorder()
	r := request(http.MethodPost, "/api/v1/policy/apply", authority, token, bytes.NewReader(payload))
	r.Header.Set("Origin", origin)
	r.Header.Set("Content-Type", contentTypeJSON)
	h.ServeHTTP(recorder, r)
	if recorder.Code != http.StatusOK || backend.applies.Load() != 1 ||
		!strings.Contains(recorder.Body.String(), change.ActiveRevision) {
		t.Fatalf("receipt response = status %d, calls %d, body %s", recorder.Code, backend.applies.Load(), recorder.Body.String())
	}
}

func TestEmbeddedAssetsContainSessionBootstrapAndThemeParity(t *testing.T) {
	t.Parallel()
	js, err := assets.ReadFile("assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	css, err := assets.ReadFile("assets/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"sessionStorage.setItem", "history.replaceState", "Authorization", "review_item_id"} {
		if !bytes.Contains(js, []byte(required)) {
			t.Errorf("app.js lacks %q", required)
		}
	}
	for _, required := range []string{`:root{color-scheme:dark`, `html[data-theme="light"]`, `--accent:`} {
		if !bytes.Contains(css, []byte(required)) {
			t.Errorf("app.css lacks %q", required)
		}
	}
	if bytes.Contains(js, []byte("https://")) || bytes.Contains(css, []byte("https://")) {
		t.Fatal("operator console assets reference an external origin")
	}
	if bytes.Contains(js, []byte("innerHTML")) {
		t.Fatal("operator console renders external text through innerHTML")
	}
}

func TestPolicyApplyRequiresOriginStrictJSONAndDelegatesOnce(t *testing.T) {
	t.Parallel()
	backend := &backendFake{snapshot: validSnapshot(), applyErr: fault.New(
		fault.KindRejected, "policy_review_changed", "policy review changed", false,
	)}
	authority, token := "127.0.0.1:43118", strings.Repeat("t", 43)
	origin := "http://" + authority
	h := newHandler(backend, authority, origin, token)
	payload := []byte(`{"decisions":[{"review_item_id":"pcy_0123456789abcdef0123456789abcdef","decision":"allow","match":"exact"}]}`)

	for _, test := range []struct {
		name        string
		origin      string
		contentType string
		body        []byte
		status      int
	}{
		{name: "missing origin", contentType: contentTypeJSON, body: payload, status: http.StatusForbidden},
		{name: "wrong content type", origin: origin, contentType: "application/json; charset=utf-8", body: payload, status: http.StatusUnsupportedMediaType},
		{name: "unknown field", origin: origin, contentType: contentTypeJSON, body: []byte(`{"decisions":[],"extra":true}`), status: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			r := request(http.MethodPost, "/api/v1/policy/apply", authority, token, bytes.NewReader(test.body))
			r.Header.Set("Origin", test.origin)
			r.Header.Set("Content-Type", test.contentType)
			h.ServeHTTP(recorder, r)
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, test.status, recorder.Body.String())
			}
		})
	}
	if backend.applies.Load() != 0 {
		t.Fatalf("invalid writes delegated %d times", backend.applies.Load())
	}
	recorder := httptest.NewRecorder()
	r := request(http.MethodPost, "/api/v1/policy/apply", authority, token, bytes.NewReader(payload))
	r.Header.Set("Origin", origin)
	r.Header.Set("Content-Type", contentTypeJSON)
	h.ServeHTTP(recorder, r)
	if recorder.Code != http.StatusConflict || backend.applies.Load() != 1 {
		t.Fatalf("valid reviewed write = status %d, calls %d, body %s", recorder.Code, backend.applies.Load(), recorder.Body.String())
	}
}

func TestRunOwnsRandomLoopbackSessionAndStopsOnCancellation(t *testing.T) {
	backend := &backendFake{snapshot: validSnapshot()}
	server := New()
	server.random = bytes.NewReader(bytes.Repeat([]byte{0x5a}, 32))
	server.openURL = func(context.Context, string) error {
		return fault.New(fault.KindUnavailable, "open_failed", "open failed", false)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan Session, 1)
	result := make(chan error, 1)
	go func() {
		result <- server.Run(ctx, backend, true, func(session Session) error {
			ready <- session
			return nil
		})
	}()
	session := <-ready
	if session.BrowserOpened || !strings.HasPrefix(session.URL, "http://127.0.0.1:") || !strings.Contains(session.URL, "/#session=") {
		t.Fatalf("session = %#v", session)
	}
	cancel()
	if err := <-result; err != nil {
		t.Fatalf("Run() cancellation error = %v", err)
	}
}
