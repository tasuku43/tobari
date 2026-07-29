// Package authn owns the application boundary that establishes and revalidates
// authentication before an authenticated operation reaches infrastructure.
package authn

import (
	"context"
	"errors"
	"time"

	"github.com/tasuku43/agentic-cli-foundry/internal/app/portcheck"
	domainauthn "github.com/tasuku43/agentic-cli-foundry/internal/domain/authn"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
)

// Authenticator resolves an infrastructure-owned credential and returns only
// non-secret metadata about the resulting session. Implementations must keep
// raw PATs, OAuth tokens, refresh material, and authenticated transports inside
// internal/infra.
type Authenticator interface {
	Authenticate(context.Context, domainauthn.Requirement) (domainauthn.Session, error)
}

// Action performs one authenticated application step. The supplied Session is
// metadata for binding and audit; it is not a bearer credential. Derived use
// cases pass Session.BindingID unchanged to their task-specific port.
type Action func(context.Context, domainauthn.Session) error

// Gate has no permissive default. A zero Gate or missing Authenticator fails
// before any authenticated action is called.
type Gate struct {
	authenticator Authenticator
	now           func() time.Time
}

// New creates a gate around one credential-owning infrastructure adapter.
func New(authenticator Authenticator) *Gate {
	return &Gate{
		authenticator: authenticator,
		now:           time.Now,
	}
}

// Invoke snapshots the requirement, authenticates once, revalidates the
// returned session against that snapshot, checks cancellation, and invokes the
// action exactly once. Every failure before the final call guarantees zero
// downstream action calls.
func (g *Gate) Invoke(ctx context.Context, requirement domainauthn.Requirement, action Action) error {
	if ctx == nil {
		return fault.New(fault.KindContract, "missing_authentication_context", "authentication context is not configured", false)
	}
	if action == nil {
		return fault.New(fault.KindContract, "missing_authenticated_action", "authenticated action is not configured", false)
	}
	requirementSnapshot := requirement.Clone()
	if err := requirementSnapshot.Validate(); err != nil {
		return fault.New(fault.KindContract, "invalid_authentication_requirement", "authentication requirement is invalid", false)
	}
	if g == nil || portcheck.IsNil(g.authenticator) {
		return fault.New(fault.KindAuthentication, "missing_authenticator", "authentication is not configured", false)
	}
	if g.now == nil {
		return fault.New(fault.KindContract, "missing_authentication_clock", "authentication clock is not configured", false)
	}
	if err := ctx.Err(); err != nil {
		return canceledFault("authentication was canceled before credential resolution")
	}

	session, err := g.authenticator.Authenticate(ctx, requirementSnapshot.Clone())
	if err != nil {
		return sanitizeAuthenticationError(ctx, err)
	}
	if err := ctx.Err(); err != nil {
		return canceledFault("authentication was canceled before session validation")
	}

	sessionSnapshot := session.Clone()
	if err := sessionSnapshot.Validate(); err != nil {
		return fault.New(fault.KindAuthentication, "invalid_authentication_session", "authentication returned invalid session metadata", false)
	}
	if err := requirementSnapshot.Satisfies(sessionSnapshot, g.now().UTC()); err != nil {
		return mismatchFault(err)
	}
	if err := ctx.Err(); err != nil {
		return canceledFault("authentication was canceled before the authenticated action")
	}

	if err := action(ctx, sessionSnapshot.Clone()); err != nil {
		return sanitizeActionError(ctx, err)
	}
	return nil
}

func mismatchFault(err error) error {
	var mismatch *domainauthn.Mismatch
	if !errors.As(err, &mismatch) {
		return fault.New(fault.KindContract, "authentication_evaluation_failed", "authentication metadata could not be evaluated", false)
	}
	switch mismatch.Kind {
	case domainauthn.MismatchCapability:
		return fault.New(fault.KindPermission, "insufficient_authentication_capability", "authentication does not grant a required capability", false)
	case domainauthn.MismatchExpired:
		return fault.New(fault.KindAuthentication, "authentication_expired", "authentication has expired", false)
	case domainauthn.MismatchMethod, domainauthn.MismatchAuthority,
		domainauthn.MismatchAudience, domainauthn.MismatchAccount:
		return fault.New(fault.KindAuthentication, "authentication_context_mismatch", "authentication does not match the required context", false)
	default:
		return fault.New(fault.KindContract, "authentication_evaluation_failed", "authentication metadata could not be evaluated", false)
	}
}

func sanitizeAuthenticationError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return canceledFault("authentication was canceled during credential resolution")
	}
	if structured, ok := safeStructuredFault(err); ok {
		switch structured.Kind {
		case fault.KindAuthentication, fault.KindPermission, fault.KindRateLimited,
			fault.KindUnavailable, fault.KindCanceled, fault.KindUnsupported:
			return structured
		}
	}
	return fault.New(fault.KindAuthentication, "authentication_failed", "authentication could not be established", false)
}

func sanitizeActionError(ctx context.Context, err error) error {
	if structured, ok := safeStructuredFault(err); ok {
		return structured
	}
	if ctx.Err() != nil {
		return canceledFault("authenticated action was canceled")
	}
	return fault.New(fault.KindInternal, "unclassified_authenticated_action_error", "authenticated action returned an unclassified error", false)
}

// safeStructuredFault strips an upstream cause because causes may contain
// request headers, tokens, URLs, or provider prose. The stable fault fields are
// an explicit public contract owned by the adapter that created them.
func safeStructuredFault(err error) (*fault.Error, bool) {
	return fault.PublicCopy(err)
}

func canceledFault(message string) *fault.Error {
	return fault.New(fault.KindCanceled, "authentication_canceled", message, false)
}
