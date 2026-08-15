package authbroker

import (
	"fmt"
	"sort"
)

const (
	maxOAuthScopes     = 32
	maxOAuthScopeBytes = 128
)

// NormalizeOAuthScopes validates the RFC 6749 scope-token shape, rejects
// duplicates, and returns a deterministic lexical set representation.
func NormalizeOAuthScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 || len(scopes) > maxOAuthScopes {
		return nil, fmt.Errorf("OAuth scope collection must contain 1..%d values", maxOAuthScopes)
	}
	normalized := append([]string(nil), scopes...)
	seen := make(map[string]struct{}, len(normalized))
	for _, scope := range normalized {
		if !validOAuthScope(scope) {
			return nil, fmt.Errorf("OAuth scope is invalid")
		}
		if _, duplicate := seen[scope]; duplicate {
			return nil, fmt.Errorf("OAuth scope is duplicated")
		}
		seen[scope] = struct{}{}
	}
	sort.Strings(normalized)
	return normalized, nil
}

func validOAuthScope(scope string) bool {
	if len(scope) == 0 || len(scope) > maxOAuthScopeBytes {
		return false
	}
	for index := 0; index < len(scope); index++ {
		character := scope[index]
		if character != 0x21 && (character < 0x23 || character > 0x5b) && (character < 0x5d || character > 0x7e) {
			return false
		}
	}
	return true
}
