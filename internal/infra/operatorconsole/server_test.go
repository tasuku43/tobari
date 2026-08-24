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
	snapshot tobari.FinalOperatorConsoleSnapshot
	applies  atomic.Int32
	change   tobari.PolicyMemoryReviewedResult
	applyErr error
}

func (b *backendFake) Snapshot(context.Context) (tobari.FinalOperatorConsoleSnapshot, error) {
	return b.snapshot, nil
}

func (b *backendFake) ApplyReviewed(
	context.Context, tobari.PolicyMemoryReviewedDecisionSet,
) (tobari.PolicyMemoryReviewedResult, error) {
	b.applies.Add(1)
	if b.applyErr != nil {
		return tobari.PolicyMemoryReviewedResult{}, b.applyErr
	}
	return b.change, nil
}

func validSnapshot() tobari.FinalOperatorConsoleSnapshot {
	review, err := tobari.NewPolicyMemoryReviewSnapshot(tobari.WorkspaceAuthorityCollection{}, false)
	if err != nil {
		panic(err)
	}
	cluster := tobari.FinalClusterStatus{SchemaVersion: tobari.FinalClusterLifecycleSchemaVersion, Task: tobari.TaskClusterStatus, Authority: tobari.FinalClusterAuthorityAbsent, Runtime: tobari.FinalClusterRuntimeAbsent, Receipt: tobari.FinalClusterReceiptAbsent, Contexts: []tobari.FinalClusterContextReceiptObservation{}, Components: []tobari.FinalClusterComponentObservation{}}
	result, err := tobari.NewFinalOperatorConsoleSnapshot(cluster, review)
	if err != nil {
		panic(err)
	}
	return result
}

func validReviewedSnapshot(t *testing.T) tobari.FinalOperatorConsoleSnapshot {
	t.Helper()
	digest := func(value string) tobari.SemanticDigest {
		return tobari.SemanticDigest("sha256:" + strings.Repeat(value, 64))
	}
	const templateID tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789a1"
	const contextID tobari.ContextID = "01912345-6789-7abc-8def-0123456789a2"
	const workspaceID tobari.WorkspaceID = "01912345-6789-7abc-8def-0123456789a3"
	body := tobari.WorkspaceTemplateBody{
		Boundary:      tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadOnly, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}}},
		Policy:        tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, Mode: tobari.ManifestPolicyModeGuided, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: string(digest("f")), Ordinal: 1, Image: tobari.OfficialRuntimeBase}}, SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: "payments", Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, ProjectRoot: "/workspace/payments", TemplateID: templateID}
	workspace := tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID, ProjectRoot: binding.ProjectRoot, Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	record := tobari.WorkspaceAuthorityContextRecord{Context: binding, PolicyMemory: memory}
	effect := tobari.PolicyCandidateEffect{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP}, Match: tobari.PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/pending", Segments: []string{}, Examples: []string{"/pending"}}
	candidate, err := tobari.NewPolicyCandidateAuthority(contextID, workspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection([]tobari.WorkspaceTemplate{template}, []tobari.WorkspaceAuthorityContextRecord{record}, []tobari.WorkspaceBinding{workspace}, []tobari.PolicyCandidateAuthority{candidate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	review, err := tobari.NewPolicyMemoryReviewSnapshot(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	cluster := tobari.FinalClusterStatus{SchemaVersion: tobari.FinalClusterLifecycleSchemaVersion, Task: tobari.TaskClusterStatus, Authority: tobari.FinalClusterAuthorityPresent, Generation: collection.Generation, CollectionRevision: collection.Revision, TemplateCount: 1, ContextCount: 1, WorkspaceCount: 1, Runtime: tobari.FinalClusterRuntimeRunning, Receipt: tobari.FinalClusterReceiptActive, Contexts: []tobari.FinalClusterContextReceiptObservation{{ContextID: contextID}}, Components: []tobari.FinalClusterComponentObservation{}}
	result, err := tobari.NewFinalOperatorConsoleSnapshot(cluster, review)
	if err != nil {
		t.Fatal(err)
	}
	return result
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
	h := newHandler(backend, authority, "http://"+authority, token, backend.snapshot)

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
	snapshot := validReviewedSnapshot(t)
	item := snapshot.Review.Items[0]
	change := tobari.PolicyMemoryReviewedResult{SchemaVersion: 2, Task: tobari.TaskPolicyReviewApply, AllowCount: 1, Applied: true, ActiveRevision: strings.Repeat("b", 64), Decisions: []tobari.PolicyMemoryReviewedResultDecision{{ReviewItemID: item.ID, RuleID: "pmr_11111111111111111111111111111111", Decision: tobari.PolicyMemoryAllow, Match: item.Match}}}
	if err := change.Validate(); err != nil {
		t.Fatal(err)
	}
	backend := &backendFake{snapshot: snapshot, change: change}
	authority, token := "127.0.0.1:43120", strings.Repeat("u", 43)
	origin := "http://" + authority
	h := newHandler(backend, authority, origin, token, snapshot)
	payload := []byte(fmt.Sprintf(
		`{"observed_generation":%d,"observed_revision":%q,"decisions":[{"review_item_id":%q,"decision":"allow"}]}`,
		snapshot.Review.CollectionGeneration, snapshot.Review.CollectionRevision, item.ID,
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
	snapshot := validReviewedSnapshot(t)
	backend := &backendFake{snapshot: snapshot, applyErr: fault.New(
		fault.KindRejected, "policy_review_changed", "policy review changed", false,
	)}
	authority, token := "127.0.0.1:43118", strings.Repeat("t", 43)
	origin := "http://" + authority
	h := newHandler(backend, authority, origin, token, snapshot)
	payload := []byte(fmt.Sprintf(`{"observed_generation":%d,"observed_revision":%q,"decisions":[{"review_item_id":%q,"decision":"allow"}]}`, snapshot.Review.CollectionGeneration, snapshot.Review.CollectionRevision, snapshot.Review.Items[0].ID))
	unknown := append([]byte{}, payload[:len(payload)-1]...)
	unknown = append(unknown, []byte(`,"extra":true}`)...)

	for _, test := range []struct {
		name        string
		origin      string
		contentType string
		body        []byte
		status      int
	}{
		{name: "missing origin", contentType: contentTypeJSON, body: payload, status: http.StatusForbidden},
		{name: "wrong content type", origin: origin, contentType: "application/json; charset=utf-8", body: payload, status: http.StatusUnsupportedMediaType},
		{name: "unknown field", origin: origin, contentType: contentTypeJSON, body: unknown, status: http.StatusBadRequest},
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
	if err := backend.snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
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
