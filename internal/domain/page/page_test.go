package page

import (
	"strings"
	"testing"
)

func TestTokenValidationPreservesOpaqueValues(t *testing.T) {
	tokens := []string{"", "cursor/with+opaque==", " leading-and-trailing "}
	for _, token := range tokens {
		request := Request{Token: token, Size: 25}
		if err := request.Validate(); err != nil {
			t.Errorf("Request(%q).Validate() error = %v", token, err)
		}
	}
}

func TestTokenValidationRejectsUnsafeEnvelope(t *testing.T) {
	for _, token := range []string{"line\nbreak", "escape\x1b", "line\u2028break", "paragraph\u2029break", strings.Repeat("a", maxTokenBytes+1)} {
		if err := ValidateToken(token, true); err == nil {
			t.Errorf("ValidateToken(%q) succeeded", token)
		}
	}
	if err := (Request{Size: 0}).Validate(); err == nil {
		t.Fatal("zero page size was accepted")
	}
}
