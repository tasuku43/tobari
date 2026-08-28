package terminal

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestServiceReviewNotificationMethodsUseOnlyFixedTrustedPayload(t *testing.T) {
	t.Parallel()
	lookup := func(name string) (string, bool) {
		if name == "TERM_PROGRAM" {
			return "iTerm.app", true
		}
		return "", false
	}
	for _, test := range []struct {
		method string
		want   string
	}{
		{method: NotificationAuto, want: serviceReviewOSC9},
		{method: NotificationOSC9, want: serviceReviewOSC9},
		{method: NotificationBEL, want: notificationBEL},
		{method: NotificationOff, want: ""},
	} {
		t.Run(test.method, func(t *testing.T) {
			var output bytes.Buffer
			if err := writeServiceReviewNotification(&output, test.method, lookup); err != nil {
				t.Fatal(err)
			}
			if output.String() != test.want {
				t.Fatalf("notification %q = %q, want %q", test.method, output.String(), test.want)
			}
			for _, hostile := range []string{"api.example.com", "\x1b]52;", "request denied", "../secret"} {
				if strings.Contains(output.String(), hostile) {
					t.Fatalf("notification payload contains evidence %q: %q", hostile, output.String())
				}
			}
		})
	}
}

func TestServiceReviewNotificationAutoFallsBackToBEL(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := writeServiceReviewNotification(&output, NotificationAuto, func(name string) (string, bool) {
		if name == "TERM_PROGRAM" {
			return "unknown-terminal", true
		}
		return "", false
	}); err != nil {
		t.Fatal(err)
	}
	if output.String() != notificationBEL {
		t.Fatalf("auto fallback = %q", output.String())
	}
}

func TestServiceReviewNotificationAutoUsesOSC9InCmuxTerminal(t *testing.T) {
	t.Parallel()
	lookup := func(name string) (string, bool) {
		values := map[string]string{
			"TERM_PROGRAM":      "ghostty",
			"CMUX_WORKSPACE_ID": "workspace:example",
			"CMUX_SURFACE_ID":   "surface:example",
		}
		value, found := values[name]
		return value, found
	}
	var output bytes.Buffer
	if err := writeServiceReviewNotification(&output, NotificationAuto, lookup); err != nil {
		t.Fatal(err)
	}
	if output.String() != serviceReviewOSC9 {
		t.Fatalf("cmux auto notification = %q, want %q", output.String(), serviceReviewOSC9)
	}
}

func TestServiceReviewNotificationAutoRequiresCompleteCmuxIdentity(t *testing.T) {
	t.Parallel()
	for _, present := range []string{"CMUX_WORKSPACE_ID", "CMUX_SURFACE_ID"} {
		present := present
		t.Run(present, func(t *testing.T) {
			var output bytes.Buffer
			if err := writeServiceReviewNotification(&output, NotificationAuto, func(name string) (string, bool) {
				if name == present {
					return "example", true
				}
				return "", false
			}); err != nil {
				t.Fatal(err)
			}
			if output.String() != notificationBEL {
				t.Fatalf("partial cmux identity %s selected %q", present, output.String())
			}
		})
	}
}

type notificationFailureWriter struct{}

func (notificationFailureWriter) Write([]byte) (int, error) {
	return 0, errors.New("synthetic write failure")
}

func TestServiceReviewNotificationReportsWriterFailure(t *testing.T) {
	t.Parallel()
	if err := writeServiceReviewNotification(notificationFailureWriter{}, NotificationBEL, nil); err == nil {
		t.Fatal("notification writer failure was hidden")
	}
}
