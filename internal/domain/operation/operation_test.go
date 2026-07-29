package operation

import (
	"encoding/json"
	"strings"
	"testing"
)

func declaredSingleImpact() Impact {
	return Impact{
		Cardinality:  CardinalityOne,
		Notification: DeclarationNo,
		AccessChange: DeclarationNo,
		Destructive:  DeclarationNo,
	}
}

func TestEffectZeroValueFailsClosed(t *testing.T) {
	if err := EffectUnknown.Validate(); err == nil {
		t.Fatal("EffectUnknown.Validate() succeeded")
	}
	if err := Effect(99).Validate(); err == nil {
		t.Fatal("unknown Effect.Validate() succeeded")
	}
}

func TestSemanticEnumsRoundTripThroughTextAndJSON(t *testing.T) {
	t.Run("effect", func(t *testing.T) {
		for _, value := range []Effect{EffectRead, EffectCreate, EffectWrite} {
			text, err := value.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText(%v) error = %v", value, err)
			}
			var decodedText Effect
			if err := decodedText.UnmarshalText(text); err != nil {
				t.Fatalf("UnmarshalText(%q) error = %v", text, err)
			}
			if decodedText != value {
				t.Fatalf("text round trip = %v, want %v", decodedText, value)
			}

			data, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("json.Marshal(%v) error = %v", value, err)
			}
			var decodedJSON Effect
			if err := json.Unmarshal(data, &decodedJSON); err != nil {
				t.Fatalf("json.Unmarshal(%s) error = %v", data, err)
			}
			if decodedJSON != value {
				t.Fatalf("JSON round trip = %v, want %v", decodedJSON, value)
			}
		}
	})

	t.Run("cardinality", func(t *testing.T) {
		for _, value := range []Cardinality{CardinalityOne, CardinalityMany, CardinalityUnbounded} {
			text, err := value.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText(%v) error = %v", value, err)
			}
			var decodedText Cardinality
			if err := decodedText.UnmarshalText(text); err != nil {
				t.Fatalf("UnmarshalText(%q) error = %v", text, err)
			}
			if decodedText != value {
				t.Fatalf("text round trip = %v, want %v", decodedText, value)
			}

			data, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("json.Marshal(%v) error = %v", value, err)
			}
			var decodedJSON Cardinality
			if err := json.Unmarshal(data, &decodedJSON); err != nil {
				t.Fatalf("json.Unmarshal(%s) error = %v", data, err)
			}
			if decodedJSON != value {
				t.Fatalf("JSON round trip = %v, want %v", decodedJSON, value)
			}
		}
	})

	t.Run("declaration", func(t *testing.T) {
		for _, value := range []Declaration{DeclarationNo, DeclarationYes} {
			text, err := value.MarshalText()
			if err != nil {
				t.Fatalf("MarshalText(%v) error = %v", value, err)
			}
			var decodedText Declaration
			if err := decodedText.UnmarshalText(text); err != nil {
				t.Fatalf("UnmarshalText(%q) error = %v", text, err)
			}
			if decodedText != value {
				t.Fatalf("text round trip = %v, want %v", decodedText, value)
			}

			data, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("json.Marshal(%v) error = %v", value, err)
			}
			var decodedJSON Declaration
			if err := json.Unmarshal(data, &decodedJSON); err != nil {
				t.Fatalf("json.Unmarshal(%s) error = %v", data, err)
			}
			if decodedJSON != value {
				t.Fatalf("JSON round trip = %v, want %v", decodedJSON, value)
			}
		}
	})
}

func TestSemanticEnumUnmarshalRejectsMalformedValuesWithoutMutation(t *testing.T) {
	t.Run("effect", func(t *testing.T) {
		for _, malformed := range []string{"", "unknown", "Read", " read", "read ", "1"} {
			got := EffectCreate
			if err := got.UnmarshalText([]byte(malformed)); err == nil {
				t.Fatalf("UnmarshalText(%q) succeeded", malformed)
			}
			if got != EffectCreate {
				t.Fatalf("UnmarshalText(%q) changed receiver to %v", malformed, got)
			}
		}
		for _, malformed := range []string{
			`null`, `1`, `true`, `{}`, `[]`, `"unknown"`, `"Read"`, `" read"`, `"read "`,
		} {
			got := EffectCreate
			if err := json.Unmarshal([]byte(malformed), &got); err == nil {
				t.Fatalf("json.Unmarshal(%s) succeeded", malformed)
			}
			if got != EffectCreate {
				t.Fatalf("json.Unmarshal(%s) changed receiver to %v", malformed, got)
			}
		}
	})

	t.Run("cardinality", func(t *testing.T) {
		for _, malformed := range []string{"", "unknown", "One", " one", "one ", "1"} {
			got := CardinalityMany
			if err := got.UnmarshalText([]byte(malformed)); err == nil {
				t.Fatalf("UnmarshalText(%q) succeeded", malformed)
			}
			if got != CardinalityMany {
				t.Fatalf("UnmarshalText(%q) changed receiver to %v", malformed, got)
			}
		}
		for _, malformed := range []string{
			`null`, `1`, `true`, `{}`, `[]`, `"unknown"`, `"One"`, `" one"`, `"one "`,
		} {
			got := CardinalityMany
			if err := json.Unmarshal([]byte(malformed), &got); err == nil {
				t.Fatalf("json.Unmarshal(%s) succeeded", malformed)
			}
			if got != CardinalityMany {
				t.Fatalf("json.Unmarshal(%s) changed receiver to %v", malformed, got)
			}
		}
	})

	t.Run("declaration", func(t *testing.T) {
		for _, malformed := range []string{"", "unknown", "Yes", " yes", "yes ", "true"} {
			got := DeclarationNo
			if err := got.UnmarshalText([]byte(malformed)); err == nil {
				t.Fatalf("UnmarshalText(%q) succeeded", malformed)
			}
			if got != DeclarationNo {
				t.Fatalf("UnmarshalText(%q) changed receiver to %v", malformed, got)
			}
		}
		for _, malformed := range []string{
			`null`, `1`, `true`, `{}`, `[]`, `"unknown"`, `"Yes"`, `" yes"`, `"yes "`,
		} {
			got := DeclarationNo
			if err := json.Unmarshal([]byte(malformed), &got); err == nil {
				t.Fatalf("json.Unmarshal(%s) succeeded", malformed)
			}
			if got != DeclarationNo {
				t.Fatalf("json.Unmarshal(%s) changed receiver to %v", malformed, got)
			}
		}
	})
}

func TestSemanticEnumUnmarshalRejectsNilReceivers(t *testing.T) {
	var effect *Effect
	if err := effect.UnmarshalText([]byte("read")); err == nil {
		t.Fatal("nil Effect receiver accepted text")
	}
	if err := effect.UnmarshalJSON([]byte(`"read"`)); err == nil {
		t.Fatal("nil Effect receiver accepted JSON")
	}
	var cardinality *Cardinality
	if err := cardinality.UnmarshalText([]byte("one")); err == nil {
		t.Fatal("nil Cardinality receiver accepted text")
	}
	if err := cardinality.UnmarshalJSON([]byte(`"one"`)); err == nil {
		t.Fatal("nil Cardinality receiver accepted JSON")
	}
	var declaration *Declaration
	if err := declaration.UnmarshalText([]byte("no")); err == nil {
		t.Fatal("nil Declaration receiver accepted text")
	}
	if err := declaration.UnmarshalJSON([]byte(`"no"`)); err == nil {
		t.Fatal("nil Declaration receiver accepted JSON")
	}
}

func TestIntentValidatesEffectSpecificTargets(t *testing.T) {
	tests := []struct {
		name    string
		intent  Intent
		wantErr bool
	}{
		{
			name:   "read without target",
			intent: Intent{Command: "doctor", Effect: EffectRead},
		},
		{
			name: "create with parent scope",
			intent: Intent{
				Command: "items create",
				Effect:  EffectCreate,
				Target:  TargetRef{Kind: "item", ParentID: "collection-1"},
				Impact:  declaredSingleImpact(),
			},
		},
		{
			name: "write with object ID",
			intent: Intent{
				Command: "items update",
				Effect:  EffectWrite,
				Target:  TargetRef{Kind: "item", ParentID: "collection-1", ID: "item-1"},
				Impact:  declaredSingleImpact(),
			},
		},
		{
			name:    "unknown effect",
			intent:  Intent{Command: "doctor"},
			wantErr: true,
		},
		{
			name:    "read with target",
			intent:  Intent{Command: "doctor", Effect: EffectRead, Target: TargetRef{Kind: "system", ID: "local"}},
			wantErr: true,
		},
		{
			name:    "create without parent",
			intent:  Intent{Command: "items create", Effect: EffectCreate, Target: TargetRef{Kind: "item"}},
			wantErr: true,
		},
		{
			name:    "write without object ID",
			intent:  Intent{Command: "items update", Effect: EffectWrite, Target: TargetRef{Kind: "item"}},
			wantErr: true,
		},
		{
			name: "write with line separator in opaque ID",
			intent: Intent{
				Command: "items update", Effect: EffectWrite,
				Target: TargetRef{Kind: "item", ID: "item\u2028unsafe"}, Impact: declaredSingleImpact(),
			},
			wantErr: true,
		},
		{
			name: "create with paragraph separator in parent",
			intent: Intent{
				Command: "items create", Effect: EffectCreate,
				Target: TargetRef{Kind: "item", ParentID: "parent\u2029unsafe"}, Impact: declaredSingleImpact(),
			},
			wantErr: true,
		},
		{
			name: "mutation without impact declaration",
			intent: Intent{
				Command: "items update",
				Effect:  EffectWrite,
				Target:  TargetRef{Kind: "item", ID: "item-1"},
			},
			wantErr: true,
		},
		{
			name: "mutation with one omitted impact dimension",
			intent: Intent{
				Command: "items update",
				Effect:  EffectWrite,
				Target:  TargetRef{Kind: "item", ID: "item-1"},
				Impact: Impact{
					Cardinality:  CardinalityOne,
					Notification: DeclarationNo,
					AccessChange: DeclarationNo,
				},
			},
			wantErr: true,
		},
		{
			name: "read with mutation impact",
			intent: Intent{
				Command: "items list",
				Effect:  EffectRead,
				Impact:  declaredSingleImpact(),
			},
			wantErr: true,
		},
		{
			name:    "invalid command path",
			intent:  Intent{Command: "Items Update", Effect: EffectRead},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.intent.Validate()
			if test.wantErr && err == nil {
				t.Fatal("Validate() succeeded")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestIntentJSONUsesStableSemanticEnumNames(t *testing.T) {
	intent := Intent{
		Command: "items update",
		Effect:  EffectWrite,
		Target:  TargetRef{Kind: "item", ID: "item-1"},
		Impact:  declaredSingleImpact(),
	}
	data, err := json.Marshal(intent)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		`"effect":"write"`,
		`"cardinality":"one"`,
		`"notification":"no"`,
		`"access_change":"no"`,
		`"destructive":"no"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("JSON = %s, want %s", got, want)
		}
	}

	if _, err := json.Marshal(Intent{Command: "items list", Effect: EffectUnknown}); err == nil {
		t.Fatal("JSON marshal accepted an unknown effect")
	}
}
