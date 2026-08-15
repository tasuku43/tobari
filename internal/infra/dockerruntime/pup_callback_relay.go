package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

const (
	pupCallbackAddress       = "127.0.0.1:8000"
	pupCallbackPath          = "/oauth/callback"
	maxPupCallbackRequestURI = 16 << 10
)

type pupLoginRelay interface {
	Complete(error)
	Close() error
}

type pupLoginRelayFactory func(context.Context, io.WriteCloser) (pupLoginRelay, error)

type httpPupLoginRelay struct {
	server       *http.Server
	listener     net.Listener
	input        io.WriteCloser
	expectedHost string
	completed    chan bool
	serveResult  chan error
	callbackUsed atomic.Bool
	completeOnce sync.Once
	closeOnce    sync.Once
	closeErr     error
}

func newPupLoginRelay(ctx context.Context, input io.WriteCloser) (pupLoginRelay, error) {
	return newPupLoginRelayAt(ctx, pupCallbackAddress, input)
}

func newPupLoginRelayAt(ctx context.Context, address string, input io.WriteCloser) (*httpPupLoginRelay, error) {
	if ctx == nil || input == nil {
		return nil, credentialhostPupSetupError()
	}
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return nil, credentialhostPupSetupError()
	}
	relay := &httpPupLoginRelay{
		listener: listener, input: input, expectedHost: listener.Addr().String(),
		completed: make(chan bool, 1), serveResult: make(chan error, 1),
	}
	relay.server = &http.Server{
		Handler:           http.HandlerFunc(relay.handle),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       5 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	go func() {
		err := relay.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		relay.serveResult <- err
	}()
	go func() {
		<-ctx.Done()
		_ = relay.server.Close()
		_ = relay.input.Close()
	}()
	return relay, nil
}

// credentialhostPupSetupError is kept as a function so this transport helper
// never adds raw listen errors (which can contain host details) to public fault chains.
func credentialhostPupSetupError() error { return credentialhost.ErrPupLoginSetup }

func (r *httpPupLoginRelay) handle(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet || request.Host != r.expectedHost ||
		request.URL.Path != pupCallbackPath || request.URL.RawPath != "" ||
		request.URL.Fragment != "" || request.ContentLength != 0 || len(request.TransferEncoding) != 0 ||
		len(request.RequestURI) == 0 || len(request.RequestURI) > maxPupCallbackRequestURI ||
		!validPupCallbackQuery(request.URL.RawQuery) {
		writePupRelayPage(writer, http.StatusBadRequest, false)
		return
	}
	if !r.callbackUsed.CompareAndSwap(false, true) {
		writePupRelayPage(writer, http.StatusConflict, false)
		return
	}
	callback := make([]byte, 0, len("http://")+len(r.expectedHost)+len(request.RequestURI)+1)
	callback = append(callback, "http://"...)
	callback = append(callback, r.expectedHost...)
	callback = append(callback, request.RequestURI...)
	callback = append(callback, '\n')
	_, err := r.input.Write(callback)
	clear(callback)
	closeErr := r.input.Close()
	if err != nil || closeErr != nil {
		writePupRelayPage(writer, http.StatusBadGateway, false)
		return
	}
	select {
	case success := <-r.completed:
		if success {
			writePupRelayPage(writer, http.StatusOK, true)
		} else {
			writePupRelayPage(writer, http.StatusBadGateway, false)
		}
	case <-request.Context().Done():
		return
	}
}

func validPupCallbackQuery(raw string) bool {
	if raw == "" || len(raw) > maxPupCallbackRequestURI-len(pupCallbackPath)-1 {
		return false
	}
	values, err := url.ParseQuery(raw)
	if err != nil || len(values) == 0 || len(values) > 8 {
		return false
	}
	allowed := map[string]int{
		"code": 4096, "state": 512, "error": 512, "error_description": 4096,
		"client_id": 512, "dd_oid": 512, "dd_org_name": 4096, "domain": 512, "site": 1024,
	}
	for key, entries := range values {
		limit, ok := allowed[key]
		if !ok || len(entries) != 1 || len(entries[0]) == 0 || len(entries[0]) > limit || strings.IndexByte(entries[0], 0) >= 0 {
			return false
		}
	}
	if len(values["state"]) != 1 {
		return false
	}
	hasCode := len(values["code"]) == 1
	hasError := len(values["error"]) == 1
	return hasCode != hasError
}

func writePupRelayPage(writer http.ResponseWriter, status int, success bool) {
	title := "Datadog authorization was not completed"
	message := "Return to the terminal and retry the login."
	if success {
		title = "Datadog authorization complete"
		message = "You can close this window and return to Tobari."
	}
	body := fmt.Sprintf("<!doctype html><html><head><meta charset=\"utf-8\"><title>%s</title></head><body><h1>%s</h1><p>%s</p></body></html>", title, title, message)
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, body)
}

func (r *httpPupLoginRelay) Complete(err error) {
	r.completeOnce.Do(func() { r.completed <- err == nil })
}

func (r *httpPupLoginRelay) Close() error {
	r.closeOnce.Do(func() {
		_ = r.input.Close()
		shutdownErr := r.server.Close()
		serveErr := <-r.serveResult
		if shutdownErr != nil || serveErr != nil {
			r.closeErr = credentialhost.ErrPupLoginCleanup
		}
	})
	return r.closeErr
}
