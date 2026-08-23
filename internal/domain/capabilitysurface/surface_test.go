package capabilitysurface

import "testing"

func TestValidateAndResearchInclusion(t *testing.T) {
	for _, test := range []struct {
		name     string
		surface  CapabilitySurface
		research bool
	}{
		{name: "release", surface: CapabilitySurfaceRelease, research: false},
		{name: "research", surface: CapabilitySurfaceResearch, research: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.surface.Validate(); err != nil {
				t.Fatal(err)
			}
			if got := test.surface.IncludesResearch(); got != test.research {
				t.Fatalf("research inclusion = %t, want %t", got, test.research)
			}
		})
	}
	if err := CapabilitySurface("other").Validate(); err == nil {
		t.Fatal("unknown capability surface was accepted")
	}
}
