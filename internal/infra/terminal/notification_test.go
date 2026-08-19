package terminal

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPermissionInboxNotificationMethodsUseOnlyFixedTrustedPayload(t *testing.T) {
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
		{method: NotificationAuto, want: permissionInboxOSC9},
		{method: NotificationOSC9, want: permissionInboxOSC9},
		{method: NotificationBEL, want: permissionInboxBEL},
		{method: NotificationOff, want: ""},
	} {
		t.Run(test.method, func(t *testing.T) {
			var output bytes.Buffer
			if err := writePermissionInboxNotification(&output, test.method, lookup); err != nil {
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

func TestPermissionInboxNotificationAutoFallsBackToBEL(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	if err := writePermissionInboxNotification(&output, NotificationAuto, func(string) (string, bool) {
		return "unknown-terminal", true
	}); err != nil {
		t.Fatal(err)
	}
	if output.String() != permissionInboxBEL {
		t.Fatalf("auto fallback = %q", output.String())
	}
}

type notificationFailureWriter struct{}

func (notificationFailureWriter) Write([]byte) (int, error) {
	return 0, errors.New("synthetic write failure")
}

func TestPermissionInboxNotificationReportsWriterFailure(t *testing.T) {
	t.Parallel()
	if err := writePermissionInboxNotification(notificationFailureWriter{}, NotificationBEL, nil); err == nil {
		t.Fatal("notification writer failure was hidden")
	}
}
