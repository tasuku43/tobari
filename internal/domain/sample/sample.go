// Package sample defines the small offline resource used by the template's
// discover-to-act example.
package sample

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	idPrefix       = "smp_"
	idSuffixLength = 12
)

// Item is one stable offline sample.
type Item struct {
	ID      string
	Name    string
	Content string
}

// Summary is the bounded discovery projection. It deliberately excludes
// Content so listing cannot retain every full resource body in memory.
type Summary struct {
	ID   string
	Name string
}

// ValidateID accepts only the canonical opaque sample ID. It does not extract
// IDs from URLs, normalize case, trim spaces, or resolve names.
func ValidateID(value string) (string, error) {
	if len(value) != len(idPrefix)+idSuffixLength || !strings.HasPrefix(value, idPrefix) {
		return "", fmt.Errorf("sample ID must use the canonical smp_<12 lowercase hex digits> form")
	}
	for _, r := range value[len(idPrefix):] {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return "", fmt.Errorf("sample ID must use the canonical smp_<12 lowercase hex digits> form")
		}
	}
	return value, nil
}

// Validate checks data returned by a repository before it reaches output.
func (i Item) Validate() error {
	if err := (Summary{ID: i.ID, Name: i.Name}).Validate(); err != nil {
		return err
	}
	if !utf8.ValidString(i.Content) {
		return fmt.Errorf("sample content is not valid UTF-8")
	}
	return nil
}

// Validate checks the bounded discovery fields returned by an adapter.
func (s Summary) Validate() error {
	if _, err := ValidateID(s.ID); err != nil {
		return err
	}
	if s.Name == "" || strings.TrimSpace(s.Name) != s.Name || !utf8.ValidString(s.Name) ||
		strings.IndexFunc(s.Name, unicode.IsControl) >= 0 {
		return fmt.Errorf("sample name is invalid")
	}
	return nil
}
