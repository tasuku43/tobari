// Package apicall defines the minimum retry and timeout contract for external
// API adapters without supplying a shared HTTP framework.
package apicall

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
)

// Idempotency describes the upstream guarantee for repeated attempts.
type Idempotency uint8

const (
	IdempotencyUnknown Idempotency = iota
	IdempotencySafe
	IdempotencyKeyed
	IdempotencyUnsafe
)

func (i Idempotency) String() string {
	switch i {
	case IdempotencySafe:
		return "safe"
	case IdempotencyKeyed:
		return "keyed"
	case IdempotencyUnsafe:
		return "unsafe"
	default:
		return "unknown"
	}
}

// Policy is declared per adapter operation. MaxAttempts counts the initial
// call; the safe default is therefore one, not zero or an implicit retry.
type Policy struct {
	Timeout        time.Duration
	MaxAttempts    int
	Idempotency    Idempotency
	IdempotencyKey string
}

// Validate rejects indefinite calls and unsafe mutation retry. Key contents
// are opaque but must be created once per logical operation and reused by the
// adapter for every transport attempt.
func (p Policy) Validate(effect operation.Effect) error {
	if err := effect.Validate(); err != nil {
		return err
	}
	if p.Timeout <= 0 {
		return fmt.Errorf("API timeout must be positive")
	}
	if p.MaxAttempts < 1 {
		return fmt.Errorf("API maximum attempts must be at least one")
	}
	switch p.Idempotency {
	case IdempotencySafe, IdempotencyKeyed, IdempotencyUnsafe:
	default:
		return fmt.Errorf("API idempotency must be declared explicitly")
	}
	if p.Idempotency == IdempotencyKeyed {
		if !validIdempotencyKey(p.IdempotencyKey) {
			return fmt.Errorf("keyed API operation requires an idempotency key")
		}
	} else if p.IdempotencyKey != "" {
		return fmt.Errorf("idempotency key is allowed only for keyed operations")
	}
	if p.MaxAttempts > 1 && p.Idempotency != IdempotencySafe && p.Idempotency != IdempotencyKeyed {
		return fmt.Errorf("API retry requires a safe or keyed upstream operation")
	}
	return nil
}

func validIdempotencyKey(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.Is(unicode.C, r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}

// SingleAttempt returns the fail-safe baseline for an adapter operation. The
// caller must still choose and declare its upstream idempotency property.
func SingleAttempt(timeout time.Duration, idempotency Idempotency) Policy {
	return Policy{Timeout: timeout, MaxAttempts: 1, Idempotency: idempotency}
}
