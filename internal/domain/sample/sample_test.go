package sample

import "testing"

func TestValidateIDPreservesCanonicalOpaqueID(t *testing.T) {
	const id = "smp_2f4a6c8e0b1d"
	got, err := ValidateID(id)
	if err != nil {
		t.Fatalf("ValidateID() error = %v", err)
	}
	if got != id {
		t.Fatalf("ValidateID() = %q, want unchanged %q", got, id)
	}
}

func TestValidateIDRejectsNonCanonicalAndAmbiguousInputs(t *testing.T) {
	invalid := []string{
		"",
		"alpha",
		"smp_2f4a",
		"smp_2F4A6C8E0B1D",
		" smp_2f4a6c8e0b1d",
		"smp_2f4a6c8e0b1d ",
		"smp_2f4a 6c8e0b1d",
		"https://example.invalid/samples/smp_2f4a6c8e0b1d",
		"samples/smp_2f4a6c8e0b1d",
	}
	for _, value := range invalid {
		if _, err := ValidateID(value); err == nil {
			t.Errorf("ValidateID(%q) succeeded", value)
		}
	}
}

func TestItemValidate(t *testing.T) {
	valid := Item{ID: "smp_2f4a6c8e0b1d", Name: "Alpha", Content: "First offline sample."}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	invalid := []Item{
		{Name: "Alpha"},
		{ID: valid.ID},
		{ID: valid.ID, Name: "bad\nname"},
		{ID: valid.ID, Name: "Alpha", Content: string([]byte{0xff})},
	}
	for index, item := range invalid {
		if err := item.Validate(); err == nil {
			t.Errorf("invalid item %d passed validation", index)
		}
	}
}

func TestSummaryValidatesOnlyBoundedDiscoveryFields(t *testing.T) {
	valid := Summary{ID: "smp_2f4a6c8e0b1d", Name: "Alpha"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	for _, summary := range []Summary{
		{},
		{ID: valid.ID},
		{ID: valid.ID, Name: "bad\nname"},
	} {
		if err := summary.Validate(); err == nil {
			t.Errorf("invalid summary passed: %+v", summary)
		}
	}
}
