package main

import (
	"path/filepath"
	"runtime"
	"testing"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}

func TestValidateRepositoryBase(t *testing.T) {
	root := repositoryRoot(t)
	if _, err := validate(root); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRepositoryClaude(t *testing.T) {
	root := repositoryRoot(t)
	if _, err := validateClaude(root); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRepositoryCodex(t *testing.T) {
	root := repositoryRoot(t)
	if _, err := validateCodex(root); err != nil {
		t.Fatal(err)
	}
}

func TestValidLicenseReview(t *testing.T) {
	for _, status := range []string{"pending", "approved"} {
		if !validLicenseReview(status) {
			t.Fatalf("validLicenseReview(%q) = false", status)
		}
	}
	for _, status := range []string{"", "accepted", "unknown"} {
		if validLicenseReview(status) {
			t.Fatalf("validLicenseReview(%q) = true", status)
		}
	}
}
