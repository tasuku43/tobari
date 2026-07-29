// Package authn defines secret-free authentication requirements, user setup,
// and session metadata. Credential acquisition and every token-bearing value
// belong to infrastructure adapters, not to this package.
package authn

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Method identifies a credential family without describing its secret.
// MethodUnknown is the zero value so an omitted declaration fails closed.
type Method string

const (
	MethodUnknown Method = ""
	MethodOAuth2  Method = "oauth2"
	MethodPAT     Method = "pat"
)

// Validate rejects an omitted or unsupported authentication method.
func (m Method) Validate() error {
	switch m {
	case MethodOAuth2, MethodPAT:
		return nil
	default:
		return fmt.Errorf("authentication method is missing or invalid")
	}
}

// Requirement describes the authentication context required by one use case.
// It deliberately contains no endpoint, credential, token, refresh material,
// client secret, or storage handle.
type Requirement struct {
	Methods              []Method `json:"methods"`
	Authority            string   `json:"authority"`
	Audience             string   `json:"audience"`
	AccountID            string   `json:"account_id,omitempty"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
}

// BindingID is a non-secret, exact correlation value for one infrastructure-
// owned authentication record. Its representation is intentionally private so
// application code can pass and compare it but cannot treat it as a provider
// token, public identifier, or storage path.
type BindingID struct {
	value string
}

// NewBindingID validates an ephemeral identifier created inside
// infrastructure. The input must be independent of credential bytes.
func NewBindingID(value string) (BindingID, error) {
	if err := validateMetadata("authentication binding ID", value, true); err != nil {
		return BindingID{}, err
	}
	return BindingID{value: value}, nil
}

// Validate rejects an omitted or unsafe binding identifier.
func (id BindingID) Validate() error {
	return validateMetadata("authentication binding ID", id.value, true)
}

// String prevents generic formatting from exposing the process-local
// correlation value. BindingID remains comparable for infrastructure maps.
func (id BindingID) String() string {
	return "<authentication-binding>"
}

// GoString applies the same redaction to %#v diagnostics and test failures.
func (id BindingID) GoString() string {
	return "authn.BindingID(<redacted>)"
}

// Clone returns a copy whose slices do not share backing storage with the
// source. Authentication gates use it to snapshot caller-owned declarations.
func (r Requirement) Clone() Requirement {
	clone := r
	clone.Methods = append([]Method(nil), r.Methods...)
	clone.RequiredCapabilities = append([]string(nil), r.RequiredCapabilities...)
	return clone
}

// Validate rejects incomplete, ambiguous, or unsafe requirement metadata.
func (r Requirement) Validate() error {
	if len(r.Methods) == 0 {
		return fmt.Errorf("at least one authentication method is required")
	}
	seenMethods := make(map[Method]struct{}, len(r.Methods))
	for _, method := range r.Methods {
		if err := method.Validate(); err != nil {
			return err
		}
		if _, exists := seenMethods[method]; exists {
			return fmt.Errorf("authentication methods must be unique")
		}
		seenMethods[method] = struct{}{}
	}
	if err := validateMetadata("authority", r.Authority, true); err != nil {
		return err
	}
	if err := validateMetadata("audience", r.Audience, true); err != nil {
		return err
	}
	if err := validateMetadata("account ID", r.AccountID, false); err != nil {
		return err
	}
	return validateUniqueMetadata("required capabilities", r.RequiredCapabilities)
}

// Session is the non-secret result of authentication. It is evidence about
// the infrastructure-owned credential, not a bearer credential or proof that
// may be sent to an external service.
type Session struct {
	Method              Method    `json:"method"`
	Authority           string    `json:"authority"`
	Audience            string    `json:"audience"`
	SubjectID           string    `json:"subject_id"`
	AccountID           string    `json:"account_id,omitempty"`
	BindingID           BindingID `json:"-"`
	GrantedCapabilities []string  `json:"granted_capabilities,omitempty"`
	ExpiresAt           time.Time `json:"expires_at,omitempty"`
}

// Clone returns a session metadata snapshot with independent slice storage.
func (s Session) Clone() Session {
	clone := s
	clone.GrantedCapabilities = append([]string(nil), s.GrantedCapabilities...)
	return clone
}

// Validate rejects incomplete or unsafe session metadata. A zero ExpiresAt is
// permitted because some PATs and provider sessions do not advertise expiry;
// a derived security model may impose a stricter rule.
func (s Session) Validate() error {
	if err := s.Method.Validate(); err != nil {
		return err
	}
	if err := validateMetadata("authority", s.Authority, true); err != nil {
		return err
	}
	if err := validateMetadata("audience", s.Audience, true); err != nil {
		return err
	}
	if err := validateMetadata("subject ID", s.SubjectID, true); err != nil {
		return err
	}
	if err := validateMetadata("account ID", s.AccountID, false); err != nil {
		return err
	}
	if err := s.BindingID.Validate(); err != nil {
		return err
	}
	return validateUniqueMetadata("granted capabilities", s.GrantedCapabilities)
}

// MismatchKind classifies why valid session metadata does not satisfy a valid
// requirement. It never contains the rejected metadata itself.
type MismatchKind string

const (
	MismatchMethod     MismatchKind = "method"
	MismatchAuthority  MismatchKind = "authority"
	MismatchAudience   MismatchKind = "audience"
	MismatchAccount    MismatchKind = "account"
	MismatchCapability MismatchKind = "capability"
	MismatchExpired    MismatchKind = "expired"
)

// Mismatch is safe to expose through structured recovery mapping because it
// carries only a stable classification and no external value.
type Mismatch struct {
	Kind MismatchKind
}

func (e *Mismatch) Error() string {
	if e == nil {
		return "authentication context does not satisfy the requirement"
	}
	return "authentication context mismatch: " + string(e.Kind)
}

// Satisfies checks exact, secret-free binding between a requirement and a
// session. Opaque metadata is compared without normalization or URL parsing.
func (r Requirement) Satisfies(s Session, now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if now.IsZero() {
		return fmt.Errorf("authentication evaluation time is required")
	}
	if !containsMethod(r.Methods, s.Method) {
		return &Mismatch{Kind: MismatchMethod}
	}
	if r.Authority != s.Authority {
		return &Mismatch{Kind: MismatchAuthority}
	}
	if r.Audience != s.Audience {
		return &Mismatch{Kind: MismatchAudience}
	}
	if r.AccountID != "" && r.AccountID != s.AccountID {
		return &Mismatch{Kind: MismatchAccount}
	}
	if !s.ExpiresAt.IsZero() && !now.Before(s.ExpiresAt) {
		return &Mismatch{Kind: MismatchExpired}
	}
	for _, required := range r.RequiredCapabilities {
		if !containsString(s.GrantedCapabilities, required) {
			return &Mismatch{Kind: MismatchCapability}
		}
	}
	return nil
}

func containsMethod(values []Method, wanted Method) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func validateUniqueMetadata(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := validateMetadata(name, value, true); err != nil {
			return err
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s must be unique", name)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateMetadata(name, value string, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}
	if len(value) > 1024 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s is invalid", name)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.Is(unicode.C, r) {
			return fmt.Errorf("%s is invalid", name)
		}
	}
	return nil
}
