package terminal

import (
	"fmt"
	"io"
	"os"
)

const (
	NotificationAuto = "auto"
	NotificationOSC9 = "osc9"
	NotificationBEL  = "bel"
	NotificationOff  = "off"

	// The Service notification payload is deliberately fixed trusted ASCII.
	// External evidence must never enter a control string.
	serviceReviewOSC9 = "\x1b]9;Tobari review needed\x07"
	notificationBEL   = "\x07"
)

// WriteServiceReviewNotification emits only a fixed generic attention cue.
// It never carries Service roots, ports, URLs, references, IDs, or owner text.
func WriteServiceReviewNotification(out io.Writer, preference string) error {
	return writeServiceReviewNotification(out, preference, os.LookupEnv)
}

func writeServiceReviewNotification(out io.Writer, preference string, lookup func(string) (string, bool)) error {
	method := preference
	if method == NotificationAuto {
		method = autoNotificationMethod(lookup)
	}
	switch method {
	case NotificationOff:
		return nil
	case NotificationOSC9:
		_, err := io.WriteString(out, serviceReviewOSC9)
		return err
	case NotificationBEL:
		_, err := io.WriteString(out, notificationBEL)
		return err
	default:
		return fmt.Errorf("unsupported terminal notification method")
	}
}

func autoNotificationMethod(lookup func(string) (string, bool)) string {
	if lookup != nil {
		// cmux injects both protected identities into every owned terminal and
		// documents OSC 9 as one of its notification inputs. Check only presence;
		// identity values never enter the fixed control payload.
		workspaceID, workspaceFound := lookup("CMUX_WORKSPACE_ID")
		surfaceID, surfaceFound := lookup("CMUX_SURFACE_ID")
		if workspaceFound && workspaceID != "" && surfaceFound && surfaceID != "" {
			return NotificationOSC9
		}
		// iTerm2 also documents OSC 9. Unknown terminals deliberately receive
		// the portable audible/visual bell rather than an assumed escape protocol.
		if program, found := lookup("TERM_PROGRAM"); found && program == "iTerm.app" {
			return NotificationOSC9
		}
	}
	return NotificationBEL
}
