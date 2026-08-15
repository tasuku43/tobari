package dockerruntime

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPupLoginRelayTransfersOneBoundedCallbackOnlyThroughStdin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	relay, err := newPupLoginRelayAt(ctx, "127.0.0.1:0", writer)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		relay.Complete(context.Canceled)
		if err := relay.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	responseResult := make(chan *http.Response, 1)
	errorResult := make(chan error, 1)
	callbackURL := "http://" + relay.expectedHost + pupCallbackPath + "?code=single-use-code-canary&state=" + strings.Repeat("s", 32)
	go func() {
		response, requestErr := http.Get(callbackURL) // #nosec G107 -- local synthetic callback server.
		if requestErr != nil {
			errorResult <- requestErr
			return
		}
		responseResult <- response
	}()
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil || line != callbackURL+"\n" {
		t.Fatalf("callback stdin = %q, error=%v", line, err)
	}
	relay.Complete(nil)
	select {
	case response := <-responseResult:
		defer response.Body.Close()
		body, _ := io.ReadAll(response.Body)
		if response.StatusCode != http.StatusOK || strings.Contains(string(body), "single-use-code-canary") {
			t.Fatalf("callback response = %d %q", response.StatusCode, body)
		}
	case err := <-errorResult:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("callback response did not complete")
	}
}

func TestPupLoginRelayRejectsMalformedOrDuplicateCallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	relay, err := newPupLoginRelayAt(ctx, "127.0.0.1:0", writer)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		relay.Complete(context.Canceled)
		_ = relay.Close()
	}()
	for _, suffix := range []string{
		"/other?code=x&state=y",
		pupCallbackPath + "?code=x&state=y&unexpected=value",
		pupCallbackPath + "?code=x&code=y&state=z",
	} {
		response, err := http.Get("http://" + relay.expectedHost + suffix) // #nosec G107 -- local synthetic callback server.
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("malformed callback %q status = %d", suffix, response.StatusCode)
		}
	}
}
