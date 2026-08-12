package dockerruntime

import (
	"strings"
	"testing"
)

const testBrokerContextID = "018bcfe5-687b-7000-8000-000000000099"

func TestBrokerLoginControlExpectationIsGitHubOnly(t *testing.T) {
	valid := []string{"login", "--context-id", testBrokerContextID, "--provider", "github", "--account-label", "octo-user"}
	expectation, err := brokerControlExpectationFor(valid)
	if err != nil || expectation.Provider != "github" || expectation.AccountLabel != "octo-user" {
		t.Fatalf("expectation/error = %+v/%v", expectation, err)
	}
	for _, provider := range []string{"aws", "datadog", "openai", "anthropic", "chatwork"} {
		arguments := append([]string(nil), valid...)
		arguments[4] = provider
		if _, err := brokerControlExpectationFor(arguments); err == nil {
			t.Fatalf("retired provider %q login accepted", provider)
		}
	}
	withDriver := append(append([]string(nil), valid...), "--driver-id", "retired", "--driver-revision", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if _, err := brokerControlExpectationFor(withDriver); err == nil {
		t.Fatal("retired driver metadata accepted")
	}
}

func TestBrokerControlRejectsRetiredCompanionOperations(t *testing.T) {
	for _, arguments := range [][]string{{"companion_status"}, {"companion_prepare", "--epoch-id", "companion-e1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}} {
		if _, err := brokerControlExpectationFor(arguments); err == nil {
			t.Fatalf("retired operation accepted: %v", arguments)
		}
	}
}

func TestBrokerStatusControlUsesConfiguredProviderState(t *testing.T) {
	expectation, err := brokerControlExpectationFor([]string{
		"status", "--context-id", testBrokerContextID, "--provider", "github",
	})
	if err != nil {
		t.Fatal(err)
	}
	configured := `{"schema_version":1,"ok":true,"state":"configured","provider":"github","revision":"revision_static"}`
	response, err := decodeBrokerControlResponse([]byte(configured), expectation)
	if err != nil || response.State != "configured" {
		t.Fatalf("configured response/error = %+v/%v", response, err)
	}
	retired := strings.Replace(configured, `"configured"`, `"ready"`, 1)
	if _, err := decodeBrokerControlResponse([]byte(retired), expectation); err == nil {
		t.Fatal("retired provider-ready state was accepted")
	}
}
