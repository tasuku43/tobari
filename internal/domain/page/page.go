// Package page defines an opaque pagination contract for external adapters.
package page

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

const maxTokenBytes = 4096

// Request asks an adapter for exactly one page. An empty Token means the first
// page. Tokens are opaque and must be forwarded byte-for-byte.
type Request struct {
	Token string
	Size  int
}

// Validate rejects malformed traversal requests without interpreting tokens.
func (r Request) Validate() error {
	if r.Size <= 0 {
		return fmt.Errorf("page size must be positive")
	}
	return ValidateToken(r.Token, true)
}

// Result contains one page. An empty NextToken is the only complete marker.
type Result[T any] struct {
	Items     []T
	NextToken string
}

// Validate checks only the cross-project envelope. Item validation remains in
// the owning domain.
func (r Result[T]) Validate() error {
	return ValidateToken(r.NextToken, true)
}

// ValidateToken checks transport safety without trimming, decoding, parsing,
// or otherwise changing an upstream cursor.
func ValidateToken(token string, allowEmpty bool) error {
	if token == "" {
		if allowEmpty {
			return nil
		}
		return fmt.Errorf("page token is required")
	}
	if len(token) > maxTokenBytes || !utf8.ValidString(token) {
		return fmt.Errorf("page token is invalid")
	}
	for _, r := range token {
		if unicode.Is(unicode.C, r) || r == '\u2028' || r == '\u2029' {
			return fmt.Errorf("page token contains an unsafe structural rune")
		}
	}
	return nil
}
