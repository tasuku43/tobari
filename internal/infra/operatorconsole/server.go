// Package operatorconsole owns the short-lived loopback HTTP presentation used
// by `tobari serve`. It contains no task interpretation or policy decisions.
package operatorconsole

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	maxRequestBytes = 64 << 10
	contentTypeJSON = "application/json"
	csp             = "default-src 'none'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
)

//go:embed assets/index.html assets/app.css assets/app.js
var assets embed.FS

type Backend interface {
	Snapshot(context.Context) (tobari.OperatorConsoleSnapshot, error)
	ApplyPolicyReview(context.Context, tobari.PolicyReviewDecisionSet) (tobari.PolicyReviewChange, error)
}

type Session struct {
	URL           string
	BrowserOpened bool
}

type Server struct {
	random  io.Reader
	listen  func(string, string) (net.Listener, error)
	openURL func(context.Context, string) error
}

func New() *Server {
	return &Server{random: rand.Reader, listen: net.Listen, openURL: openBrowser}
}

// Run preflights one typed snapshot, serves until cancellation, and owns every
// resource created for the session.
func (s *Server) Run(
	ctx context.Context, backend Backend, open bool, ready func(Session) error,
) error {
	if s == nil || backend == nil || s.random == nil || s.listen == nil || s.openURL == nil {
		return fault.New(fault.KindContract, "invalid_operator_console", "operator console is not configured", false)
	}
	if ctx == nil {
		return fault.New(fault.KindContract, "missing_context", "operator console context is not configured", false)
	}
	preflight, err := backend.Snapshot(ctx)
	if err != nil {
		return err
	}
	if err := preflight.Validate(); err != nil {
		return fault.Wrap(fault.KindContract, "invalid_operator_console_snapshot", "operator console snapshot is invalid", false, err)
	}
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(s.random, tokenBytes); err != nil {
		return fault.Wrap(fault.KindInternal, "operator_console_session_failed", "operator console session could not be created", false, err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	listener, err := s.listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fault.Wrap(fault.KindUnavailable, "operator_console_unavailable", "operator console loopback listener could not be started", false, err)
	}
	defer listener.Close()
	authority := listener.Addr().String()
	origin := "http://" + authority
	pageURL := origin + "/#session=" + token
	handler := newHandler(backend, authority, origin, token)
	httpServer := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 2*time.Minute + 5*time.Second, IdleTimeout: 30 * time.Second,
		MaxHeaderBytes: 16 << 10,
	}
	serveResult := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serveResult <- err
	}()
	opened := false
	if open {
		opened = s.openURL(ctx, pageURL) == nil
	}
	if ready != nil {
		if err := ready(Session{URL: pageURL, BrowserOpened: opened}); err != nil {
			_ = httpServer.Close()
			<-serveResult
			return err
		}
	}
	select {
	case err := <-serveResult:
		if err != nil {
			return fault.Wrap(fault.KindInternal, "operator_console_failed", "operator console stopped unexpectedly", false, err)
		}
		return nil
	case <-ctx.Done():
		if err := httpServer.Close(); err != nil {
			return fault.Wrap(fault.KindInternal, "operator_console_shutdown_failed", "operator console could not stop cleanly", false, err)
		}
		if err := <-serveResult; err != nil {
			return fault.Wrap(fault.KindInternal, "operator_console_failed", "operator console stopped unexpectedly", false, err)
		}
		return nil
	}
}

type handler struct {
	backend   Backend
	authority string
	origin    string
	token     string
}

func newHandler(backend Backend, authority, origin, token string) http.Handler {
	return &handler{backend: backend, authority: authority, origin: origin, token: token}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	secureHeaders(w.Header())
	if !validLoopbackPeer(r.RemoteAddr) || r.Host != h.authority {
		writeAPIError(w, http.StatusForbidden, "forbidden", "request is outside this operator console session")
		return
	}
	if r.URL.RawQuery != "" {
		writeAPIError(w, http.StatusNotFound, "not_found", "route is not available")
		return
	}
	switch r.URL.Path {
	case "/":
		h.serveAsset(w, r, "assets/index.html", "text/html; charset=utf-8")
	case "/assets/app.css":
		h.serveAsset(w, r, "assets/app.css", "text/css; charset=utf-8")
	case "/assets/app.js":
		h.serveAsset(w, r, "assets/app.js", "text/javascript; charset=utf-8")
	case "/api/v1/snapshot":
		h.serveSnapshot(w, r)
	case "/api/v1/policy/apply":
		h.serveApply(w, r)
	default:
		writeAPIError(w, http.StatusNotFound, "not_found", "route is not available")
	}
}

func secureHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store")
	header.Set("Content-Security-Policy", csp)
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}

func (h *handler) serveAsset(w http.ResponseWriter, r *http.Request, name, contentType string) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method is not available")
		return
	}
	data, err := assets.ReadFile(name)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "asset_unavailable", "operator console asset is unavailable")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *handler) authorize(w http.ResponseWriter, r *http.Request) bool {
	presented := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if len(presented) != len(h.token) || subtle.ConstantTimeCompare([]byte(presented), []byte(h.token)) != 1 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="tobari-operator-console"`)
		writeAPIError(w, http.StatusUnauthorized, "invalid_session", "operator console session is invalid")
		return false
	}
	return true
}

func (h *handler) serveSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method is not available")
		return
	}
	if !h.authorize(w, r) {
		return
	}
	snapshot, err := h.backend.Snapshot(r.Context())
	if err != nil {
		writeBackendError(w, err)
		return
	}
	if err := snapshot.Validate(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "invalid_operator_console_snapshot", "operator console snapshot is invalid")
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (h *handler) serveApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method is not available")
		return
	}
	if !h.authorize(w, r) {
		return
	}
	if r.Header.Get("Origin") != h.origin {
		writeAPIError(w, http.StatusForbidden, "invalid_origin", "operator console write origin is invalid")
		return
	}
	if r.Header.Get("Content-Type") != contentTypeJSON {
		writeAPIError(w, http.StatusUnsupportedMediaType, "invalid_content_type", "content type must be application/json")
		return
	}
	body := http.MaxBytesReader(w, r.Body, maxRequestBytes)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var set tobari.PolicyReviewDecisionSet
	if err := decoder.Decode(&set); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_policy_review_set", "reviewed policy decision set is invalid")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeAPIError(w, http.StatusBadRequest, "invalid_policy_review_set", "reviewed policy decision set must contain one JSON value")
		return
	}
	if err := set.Validate(); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_policy_review_set", "reviewed policy decision set is invalid")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	change, err := h.backend.ApplyPolicyReview(ctx, set)
	if err != nil {
		writeBackendError(w, err)
		return
	}
	if err := change.Validate(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "invalid_policy_review_result", "reviewed policy result is invalid")
		return
	}
	writeJSON(w, http.StatusOK, change)
}

func validLoopbackPeer(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() != nil && ip.IsLoopback()
}

func writeBackendError(w http.ResponseWriter, err error) {
	if public, ok := fault.PublicCopy(err); ok {
		status := http.StatusInternalServerError
		switch public.Kind {
		case fault.KindInvalidInput:
			status = http.StatusBadRequest
		case fault.KindAuthentication:
			status = http.StatusUnauthorized
		case fault.KindPermission:
			status = http.StatusForbidden
		case fault.KindNotFound:
			status = http.StatusNotFound
		case fault.KindRejected, fault.KindAmbiguous:
			status = http.StatusConflict
		case fault.KindUnavailable, fault.KindRateLimited:
			status = http.StatusServiceUnavailable
		}
		writeAPIError(w, status, public.Code, public.Message)
		return
	}
	writeAPIError(w, http.StatusInternalServerError, "operator_console_failed", "operator console request failed")
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func openBrowser(ctx context.Context, target string) error {
	if !strings.HasPrefix(target, "http://127.0.0.1:") || !strings.Contains(target, "/#session=") {
		return fmt.Errorf("operator console browser target is invalid")
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.CommandContext(ctx, "open", target) // #nosec G204 -- executable is fixed and target passed the exact loopback session allowlist above.
	case "windows":
		command = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", target) // #nosec G204 -- executable and handler are fixed and target passed the exact loopback session allowlist above.
	default:
		command = exec.CommandContext(ctx, "xdg-open", target) // #nosec G204 -- executable is fixed and target passed the exact loopback session allowlist above.
	}
	return command.Run()
}
