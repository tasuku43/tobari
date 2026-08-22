// Package fault defines stable, machine-classifiable failures shared by the
// application and presentation layers.
package fault

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Kind is Tobari's stable failure taxonomy. Commands may add task-specific
// Codes, but should map them to one of these recovery classes.
type Kind string

const (
	KindInvalidInput   Kind = "invalid_input"
	KindAuthentication Kind = "authentication"
	KindPermission     Kind = "permission"
	KindNotFound       Kind = "not_found"
	KindAmbiguous      Kind = "ambiguous"
	KindRejected       Kind = "rejected"
	KindRateLimited    Kind = "rate_limited"
	KindUnavailable    Kind = "unavailable"
	KindCanceled       Kind = "canceled"
	KindUnsupported    Kind = "unsupported"
	KindContract       Kind = "contract"
	KindInternal       Kind = "internal"
)

// Phase identifies the closed command stage that established a failure.
type Phase string

const (
	PhasePrecondition Phase = "precondition"
	PhaseObservation  Phase = "observation"
	PhaseMutation     Phase = "mutation"
	PhaseVerification Phase = "verification"
	PhaseAttachment   Phase = "attachment"
	PhasePresentation Phase = "presentation"
)

func (p Phase) Validate() error {
	switch p {
	case PhasePrecondition, PhaseObservation, PhaseMutation, PhaseVerification, PhaseAttachment, PhasePresentation:
		return nil
	default:
		return fmt.Errorf("fault phase is missing or invalid: %q", p)
	}
}

// ChangeState is the strongest mutation-state fact proved by the owning layer.
type ChangeState string

const (
	ChangeNotApplicable ChangeState = "not_applicable"
	ChangeNone          ChangeState = "none"
	ChangePartial       ChangeState = "partial"
	ChangeConfirmed     ChangeState = "confirmed"
	ChangeUnknown       ChangeState = "unknown"
)

func (s ChangeState) Validate() error {
	switch s {
	case ChangeNotApplicable, ChangeNone, ChangePartial, ChangeConfirmed, ChangeUnknown:
		return nil
	default:
		return fmt.Errorf("fault change state is missing or invalid: %q", s)
	}
}

// NextAction tells an agent which command can resolve or investigate a fault.
type NextAction struct {
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

// Error retains a human explanation and stable recovery metadata. Cause is
// intentionally omitted from machine output because upstream errors can carry
// credentials, URLs, or unstable prose.
type Error struct {
	Kind        Kind          `json:"kind"`
	Code        string        `json:"code"`
	Message     string        `json:"message"`
	Phase       Phase         `json:"phase,omitempty"`
	ChangeState ChangeState   `json:"change_state,omitempty"`
	Retryable   bool          `json:"retryable"`
	RetryAfter  time.Duration `json:"-"`
	NextActions []NextAction  `json:"next_actions"`
	cause       error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	// Cause can contain an upstream response body, URL, authorization value,
	// or other unstable secret-bearing prose. Callers can inspect it with
	// errors.Is/As, but public presentation receives only the reviewed message.
	return e.Message
}

// Unwrap preserves errors.Is and errors.As behavior for domain sentinels.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Validate detects malformed structured faults before they become a public
// machine contract.
func (e *Error) Validate() error {
	if e == nil {
		return fmt.Errorf("fault is nil")
	}
	switch e.Kind {
	case KindInvalidInput, KindAuthentication, KindPermission, KindNotFound,
		KindAmbiguous, KindRejected, KindRateLimited, KindUnavailable,
		KindCanceled, KindUnsupported, KindContract, KindInternal:
	default:
		return fmt.Errorf("fault kind is missing or invalid: %q", e.Kind)
	}
	if !validCode(e.Code) {
		return fmt.Errorf("fault code is missing or invalid: %q", e.Code)
	}
	if !validPublicText(e.Message, 1024) {
		return fmt.Errorf("fault message is missing or unsafe")
	}
	if (e.Phase == "") != (e.ChangeState == "") {
		return fmt.Errorf("fault phase and change state must be declared together")
	}
	if e.Phase != "" {
		if err := e.Phase.Validate(); err != nil {
			return err
		}
		if err := e.ChangeState.Validate(); err != nil {
			return err
		}
		if e.Phase == PhaseObservation && e.ChangeState != ChangeNotApplicable {
			return fmt.Errorf("observation failure change state must be not_applicable")
		}
		if e.ChangeState == ChangeNotApplicable && e.Phase != PhaseObservation && e.Phase != PhasePresentation {
			return fmt.Errorf("not_applicable change state requires observation or presentation phase")
		}
	}
	if e.RetryAfter < 0 || (e.RetryAfter > 0 && !e.Retryable && e.Kind != KindRateLimited) {
		return fmt.Errorf("retry-after requires a retryable or rate-limited fault")
	}
	for index, action := range e.NextActions {
		if !validPublicText(action.Command, 512) || !validPublicText(action.Reason, 1024) {
			return fmt.Errorf("next action %d requires command and reason", index)
		}
	}
	return nil
}

// WithClassification returns a detached classified fault while retaining the
// private cause only inside the trusted process.
func WithClassification(err *Error, phase Phase, state ChangeState) *Error {
	if err == nil {
		return nil
	}
	clone := *err
	clone.NextActions = append([]NextAction(nil), err.NextActions...)
	clone.Phase = phase
	clone.ChangeState = state
	return &clone
}

func validPublicText(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.Is(unicode.C, r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}

func validCode(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for index, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case index > 0 && r >= '0' && r <= '9':
		case index > 0 && (r == '_' || r == '.'):
		default:
			return false
		}
		if unicode.Is(unicode.C, r) {
			return false
		}
	}
	return true
}

// New creates a fault without an upstream cause.
func New(kind Kind, code, message string, retryable bool, next ...NextAction) *Error {
	return Wrap(kind, code, message, retryable, nil, next...)
}

// PublicCopy extracts the first valid structured fault from an error chain and
// returns a detached copy that cannot unwrap to the upstream cause. Wrappers
// and causes can contain credentials, request URLs, response bodies, or other
// unstable prose, so presentation and cross-boundary code must use this copy
// instead of returning an arbitrary containing error.
func PublicCopy(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	var structured *Error
	if !errors.As(err, &structured) || structured.Validate() != nil {
		return nil, false
	}
	clone := *structured
	clone.NextActions = append([]NextAction(nil), structured.NextActions...)
	clone.cause = nil
	return &clone, true
}

// Wrap creates a fault that preserves an upstream sentinel for errors.Is.
func Wrap(kind Kind, code, message string, retryable bool, cause error, next ...NextAction) *Error {
	return &Error{
		Kind:        kind,
		Code:        code,
		Message:     message,
		Retryable:   retryable,
		NextActions: append([]NextAction(nil), next...),
		cause:       cause,
	}
}
