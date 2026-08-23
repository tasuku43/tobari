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

func TestValidatedGoBuilderImage(t *testing.T) {
	root := repositoryRoot(t)
	if _, err := validatedGoBuilderImage(root); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAgentArtifactLockRejectsIncompleteMatrix(t *testing.T) {
	lock := agentArtifactLock{
		Name:          "agent",
		Version:       "1.2.3",
		Source:        "https://example.com/releases",
		LicenseReview: "approved",
		Platforms: map[string]struct {
			Asset  string `json:"asset"`
			SHA256 string `json:"sha256"`
			Size   int    `json:"size"`
		}{},
	}
	if err := validateAgentArtifactLock(lock, "agent", "1.2.3", "https://example.com/releases", map[string]string{"linux/amd64": "agent"}); err == nil {
		t.Fatal("validateAgentArtifactLock accepted an incomplete architecture matrix")
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
