package capabilityprofile

import "testing"

func TestCompiledProfileIsValid(t *testing.T) {
	if err := Compiled().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileRejectsRuntimeInventedValue(t *testing.T) {
	if err := Profile("aws-enabled").Validate(); err == nil {
		t.Fatal("unknown capability profile was accepted")
	}
}
