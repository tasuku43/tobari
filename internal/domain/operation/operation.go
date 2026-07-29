// Package operation defines the side-effect contract shared by every command.
package operation

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Effect describes how one command execution can change external state.
// EffectUnknown is the zero value so an omitted declaration fails closed.
type Effect uint8

const (
	EffectUnknown Effect = iota
	EffectRead
	EffectCreate
	EffectWrite
)

func (e Effect) String() string {
	switch e {
	case EffectRead:
		return "read"
	case EffectCreate:
		return "create"
	case EffectWrite:
		return "write"
	default:
		return "unknown"
	}
}

// MarshalText gives JSON and other text encoders the stable public spelling.
func (e Effect) MarshalText() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	return []byte(e.String()), nil
}

// UnmarshalText accepts only the stable public spellings emitted by
// MarshalText. Invalid input leaves the receiver unchanged.
func (e *Effect) UnmarshalText(text []byte) error {
	var parsed Effect
	switch string(text) {
	case "read":
		parsed = EffectRead
	case "create":
		parsed = EffectCreate
	case "write":
		parsed = EffectWrite
	default:
		return fmt.Errorf("effect is invalid: %q", text)
	}
	if e == nil {
		return fmt.Errorf("effect receiver is nil")
	}
	*e = parsed
	return nil
}

// UnmarshalJSON accepts only a quoted stable public spelling. In particular,
// JSON null is not treated as an absent update that preserves an earlier
// effect value.
func (e *Effect) UnmarshalJSON(data []byte) error {
	if e == nil {
		return fmt.Errorf("effect receiver is nil")
	}
	text, err := unmarshalSemanticEnumJSON(data, "effect")
	if err != nil {
		return err
	}
	return e.UnmarshalText(text)
}

// Validate rejects the zero value and values that are not part of the public
// effect model.
func (e Effect) Validate() error {
	switch e {
	case EffectRead, EffectCreate, EffectWrite:
		return nil
	default:
		return fmt.Errorf("effect is missing or invalid: %d", e)
	}
}

// TargetRef identifies the object, or parent scope, affected by a mutation.
// Read operations deliberately use the zero value.
type TargetRef struct {
	Kind     string `json:"kind,omitempty"`
	ParentID string `json:"parent_id,omitempty"`
	ID       string `json:"id,omitempty"`
}

// IsZero reports whether no target information has been declared.
func (r TargetRef) IsZero() bool {
	return r.Kind == "" && r.ParentID == "" && r.ID == ""
}

// Cardinality describes the maximum number of external objects affected by
// one logical mutation. CardinalityUnknown is intentionally invalid.
type Cardinality uint8

const (
	CardinalityUnknown Cardinality = iota
	CardinalityOne
	CardinalityMany
	CardinalityUnbounded
)

func (c Cardinality) String() string {
	switch c {
	case CardinalityOne:
		return "one"
	case CardinalityMany:
		return "many"
	case CardinalityUnbounded:
		return "unbounded"
	default:
		return "unknown"
	}
}

// MarshalText gives JSON and other text encoders the stable public spelling.
func (c Cardinality) MarshalText() ([]byte, error) {
	switch c {
	case CardinalityOne, CardinalityMany, CardinalityUnbounded:
		return []byte(c.String()), nil
	default:
		return nil, fmt.Errorf("impact cardinality is missing or invalid: %d", c)
	}
}

// UnmarshalText accepts only the stable public spellings emitted by
// MarshalText. Invalid input leaves the receiver unchanged.
func (c *Cardinality) UnmarshalText(text []byte) error {
	var parsed Cardinality
	switch string(text) {
	case "one":
		parsed = CardinalityOne
	case "many":
		parsed = CardinalityMany
	case "unbounded":
		parsed = CardinalityUnbounded
	default:
		return fmt.Errorf("impact cardinality is invalid: %q", text)
	}
	if c == nil {
		return fmt.Errorf("impact cardinality receiver is nil")
	}
	*c = parsed
	return nil
}

// UnmarshalJSON accepts only a quoted stable public spelling. Invalid JSON
// leaves the receiver unchanged.
func (c *Cardinality) UnmarshalJSON(data []byte) error {
	if c == nil {
		return fmt.Errorf("impact cardinality receiver is nil")
	}
	text, err := unmarshalSemanticEnumJSON(data, "impact cardinality")
	if err != nil {
		return err
	}
	return c.UnmarshalText(text)
}

// Declaration is an explicit yes/no declaration. Its zero value means that a
// mutation author has not considered that impact dimension yet.
type Declaration uint8

const (
	DeclarationUnknown Declaration = iota
	DeclarationNo
	DeclarationYes
)

func (d Declaration) String() string {
	switch d {
	case DeclarationNo:
		return "no"
	case DeclarationYes:
		return "yes"
	default:
		return "unknown"
	}
}

// MarshalText gives JSON and other text encoders the stable public spelling.
func (d Declaration) MarshalText() ([]byte, error) {
	if err := d.validate("impact"); err != nil {
		return nil, err
	}
	return []byte(d.String()), nil
}

// UnmarshalText accepts only the stable public spellings emitted by
// MarshalText. Invalid input leaves the receiver unchanged.
func (d *Declaration) UnmarshalText(text []byte) error {
	var parsed Declaration
	switch string(text) {
	case "no":
		parsed = DeclarationNo
	case "yes":
		parsed = DeclarationYes
	default:
		return fmt.Errorf("impact declaration is invalid: %q", text)
	}
	if d == nil {
		return fmt.Errorf("impact declaration receiver is nil")
	}
	*d = parsed
	return nil
}

// UnmarshalJSON accepts only a quoted stable public spelling. Invalid JSON
// leaves the receiver unchanged.
func (d *Declaration) UnmarshalJSON(data []byte) error {
	if d == nil {
		return fmt.Errorf("impact declaration receiver is nil")
	}
	text, err := unmarshalSemanticEnumJSON(data, "impact declaration")
	if err != nil {
		return err
	}
	return d.UnmarshalText(text)
}

func unmarshalSemanticEnumJSON(data []byte, label string) ([]byte, error) {
	var text *string
	if err := json.Unmarshal(data, &text); err != nil {
		return nil, fmt.Errorf("%s JSON must be a quoted string: %w", label, err)
	}
	if text == nil {
		return nil, fmt.Errorf("%s JSON must be a quoted string", label)
	}
	return []byte(*text), nil
}

func (d Declaration) validate(name string) error {
	if d != DeclarationNo && d != DeclarationYes {
		return fmt.Errorf("%s impact must be declared explicitly", name)
	}
	return nil
}

// Impact captures policy-relevant facts that apply across API-backed CLIs.
// Product-specific impact remains a domain-owned extension in derived
// projects; this base contract deliberately does not encode vendor concepts.
type Impact struct {
	Cardinality  Cardinality `json:"cardinality,omitempty"`
	Notification Declaration `json:"notification,omitempty"`
	AccessChange Declaration `json:"access_change,omitempty"`
	Destructive  Declaration `json:"destructive,omitempty"`
}

// IsZero reports whether no impact declaration was supplied.
func (i Impact) IsZero() bool {
	return i == (Impact{})
}

// Validate requires every generic impact dimension to be considered. This is
// what makes an omitted mutation declaration fail closed rather than looking
// like a safe read.
func (i Impact) Validate() error {
	switch i.Cardinality {
	case CardinalityOne, CardinalityMany, CardinalityUnbounded:
	default:
		return fmt.Errorf("impact cardinality must be declared explicitly")
	}
	if err := i.Notification.validate("notification"); err != nil {
		return err
	}
	if err := i.AccessChange.validate("access change"); err != nil {
		return err
	}
	return i.Destructive.validate("destructive")
}

// Intent is the complete side-effect declaration for one execution.
type Intent struct {
	Command string    `json:"command"`
	Effect  Effect    `json:"effect"`
	Target  TargetRef `json:"target,omitempty"`
	Impact  Impact    `json:"impact,omitempty"`
}

// Validate enforces the target shape required by each effect.
func (i Intent) Validate() error {
	if err := ValidateCommandPath(i.Command); err != nil {
		return err
	}
	if err := i.Effect.Validate(); err != nil {
		return err
	}

	switch i.Effect {
	case EffectRead:
		if !i.Target.IsZero() {
			return fmt.Errorf("read intent must not declare a target")
		}
		if !i.Impact.IsZero() {
			return fmt.Errorf("read intent must not declare mutation impact")
		}
	case EffectCreate:
		if !validName(i.Target.Kind) || !validRefPart(i.Target.ParentID) || i.Target.ID != "" {
			return fmt.Errorf("create intent requires a target kind and parent scope only")
		}
		if err := i.Impact.Validate(); err != nil {
			return fmt.Errorf("create intent: %w", err)
		}
	case EffectWrite:
		if !validName(i.Target.Kind) || !validOptionalRefPart(i.Target.ParentID) || !validRefPart(i.Target.ID) {
			return fmt.Errorf("write intent requires a target kind and object ID")
		}
		if err := i.Impact.Validate(); err != nil {
			return fmt.Errorf("write intent: %w", err)
		}
	}
	return nil
}

// ValidateCommandPath accepts canonical lowercase command words separated by
// one ASCII space.
func ValidateCommandPath(path string) error {
	if path == "" || strings.TrimSpace(path) != path || strings.Contains(path, "  ") {
		return fmt.Errorf("command path is missing or invalid: %q", path)
	}
	for _, word := range strings.Split(path, " ") {
		if !validName(word) {
			return fmt.Errorf("command path is missing or invalid: %q", path)
		}
	}
	return nil
}

func validName(value string) bool {
	if value == "" {
		return false
	}
	for index, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case index > 0 && r >= '0' && r <= '9':
		case index > 0 && r == '-':
		default:
			return false
		}
	}
	return true
}

func validOptionalRefPart(value string) bool {
	return value == "" || validRefPart(value)
}

func validRefPart(value string) bool {
	if value == "" || len(value) > 1024 || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.Is(unicode.C, r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}
