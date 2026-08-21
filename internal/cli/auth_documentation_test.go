package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var unqualifiedBrokerAuthExample = regexp.MustCompile(`(?m)^[ \t]*(?:\$[ \t]+)?(?:tobari|tobari-dev)[ \t]+auth(?:[ \t]|$)`)

func TestAuthenticationDocumentationSeparatesStandardAndExperimentalProfiles(t *testing.T) {
	t.Parallel()
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	readme := readAuthenticationDocument(t, filepath.Join(repositoryRoot, "README.md"))
	reference := readAuthenticationDocument(t, filepath.Join(repositoryRoot, "docs", "07_authentication.md"))

	standard := sectionBetween(t, readme,
		"### Standard native Workspace authentication",
		"### Experimental Broker research")
	for _, required := range []string{
		"tobari -- claude",
		"tobari -- codex",
		"tobari -- gh auth login",
		"available to every process in the same Workspace",
		"never mounts",
		"host CLI homes",
		"The standard and release binaries have no `auth` namespace",
		"The bridge cannot read the resulting credential and grants no HTTP permission",
	} {
		if !strings.Contains(standard, required) {
			t.Errorf("standard authentication section lacks %q", required)
		}
	}
	for _, retired := range []string{"Broker", "vault", "project-bound handle", "broker_auth_required", "tobari auth"} {
		if strings.Contains(standard, retired) {
			t.Errorf("standard authentication section retains experimental claim %q", retired)
		}
	}

	experimental := sectionBetween(t, readme, "### Experimental Broker research", "## Runtime customization")
	for _, required := range []string{
		"unsupported, unpublished",
		"for development only",
		"absent from the standard and release",
		"task build:dev",
		"bin/tobari-dev auth login",
		"docs/07_authentication.md#experimental-broker-profile",
	} {
		if !strings.Contains(experimental, required) {
			t.Errorf("experimental authentication section lacks %q", required)
		}
	}

	for name, document := range map[string]string{"README.md": readme, "docs/07_authentication.md": reference} {
		if match := unqualifiedBrokerAuthExample.FindString(document); match != "" {
			t.Errorf("%s contains an auth example without bin/tobari-dev: %q", name, strings.TrimSpace(match))
		}
	}

	for _, stale := range []string{
		"Gateway/Auth Broker image indexes",
		"Published binaries use immutable Gateway and Auth Broker identities",
		"publish and inspect only the Gateway and Auth Broker images",
	} {
		if strings.Contains(readme, stale) {
			t.Errorf("README retains stale release claim %q", stale)
		}
	}
}

func readAuthenticationDocument(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func sectionBetween(t *testing.T, document, start, end string) string {
	t.Helper()
	startIndex := strings.Index(document, start)
	if startIndex < 0 {
		t.Fatalf("document lacks section start %q", start)
	}
	endIndex := strings.Index(document[startIndex+len(start):], end)
	if endIndex < 0 {
		t.Fatalf("document lacks section end %q after %q", end, start)
	}
	return document[startIndex : startIndex+len(start)+endIndex]
}
