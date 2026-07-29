// Package doctor defines the diagnostic result returned by the doctor use case.
package doctor

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CheckStatus is the machine-readable outcome of one diagnostic check.
type CheckStatus string

const (
	CheckStatusPass CheckStatus = "pass"
	CheckStatusWarn CheckStatus = "warn"
	CheckStatusFail CheckStatus = "fail"
)

// Validate rejects empty and unknown statuses.
func (s CheckStatus) Validate() error {
	switch s {
	case CheckStatusPass, CheckStatusWarn, CheckStatusFail:
		return nil
	default:
		return fmt.Errorf("check status is missing or invalid: %q", s)
	}
}

// Check is one row in a diagnostic report.
type Check struct {
	Name   string
	Status CheckStatus
	Detail string
}

// Report is the complete result of a doctor inspection.
type Report struct {
	Checks []Check
}

// Validate ensures that adapters return a complete, deterministic shape.
func (r Report) Validate() error {
	if len(r.Checks) == 0 {
		return fmt.Errorf("doctor report must contain at least one check")
	}
	seen := make(map[string]struct{}, len(r.Checks))
	for index, check := range r.Checks {
		if check.Name == "" || strings.TrimSpace(check.Name) != check.Name || !utf8.ValidString(check.Name) ||
			strings.IndexFunc(check.Name, unicode.IsControl) >= 0 {
			return fmt.Errorf("doctor check %d has an invalid name", index)
		}
		if _, exists := seen[check.Name]; exists {
			return fmt.Errorf("doctor report contains duplicate check %q", check.Name)
		}
		seen[check.Name] = struct{}{}
		if err := check.Status.Validate(); err != nil {
			return fmt.Errorf("doctor check %q: %w", check.Name, err)
		}
		if !utf8.ValidString(check.Detail) {
			return fmt.Errorf("doctor check %q has invalid UTF-8 detail", check.Name)
		}
	}
	return nil
}

// Healthy reports whether every check passed or produced only a warning.
func (r Report) Healthy() bool {
	for _, check := range r.Checks {
		if check.Status == CheckStatusFail {
			return false
		}
	}
	return true
}
