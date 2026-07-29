package releaseversion

import "testing"

func TestParseAcceptsSemVer(t *testing.T) {
	tests := []struct {
		tag       string
		stable    bool
		value     string
		buildInfo string
	}{
		{tag: "v0.0.0", stable: true, value: "0.0.0"},
		{tag: "v1.2.3-rc.1", stable: false, value: "1.2.3-rc.1"},
		{tag: "v1.2.3-alpha-beta", stable: false, value: "1.2.3-alpha-beta"},
		{tag: "v1.2.3+build.01", stable: true, value: "1.2.3+build.01", buildInfo: "build.01"},
	}
	for _, test := range tests {
		t.Run(test.tag, func(t *testing.T) {
			version, err := Parse(test.tag)
			if err != nil {
				t.Fatal(err)
			}
			if version.Stable() != test.stable || version.Value != test.value || version.Build != test.buildInfo {
				t.Fatalf("version = %#v", version)
			}
		})
	}
}

func TestParseRejectsInvalidSemVer(t *testing.T) {
	for _, tag := range []string{
		"1.2.3",
		"v1.2",
		"v01.2.3",
		"v1.02.3",
		"v1.2.03",
		"v1.2.3-01",
		"v1.2.3-rc.01",
		"v1.2.3-",
		"v1.2.3+",
		"v1.2.3_rc1",
	} {
		t.Run(tag, func(t *testing.T) {
			if _, err := Parse(tag); err == nil {
				t.Fatalf("Parse(%q) succeeded", tag)
			}
		})
	}
}

func TestParseReleaseTagRejectsBuildMetadata(t *testing.T) {
	if _, err := ParseReleaseTag("v1.2.3+build.1"); err == nil {
		t.Fatal("ParseReleaseTag accepted build metadata")
	}
}
