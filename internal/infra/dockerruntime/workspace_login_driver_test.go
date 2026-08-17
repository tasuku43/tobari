package dockerruntime

import (
	"strings"
	"testing"
)

func TestReviewedWorkspaceLoginDriverRegistryOwnsClosedUnion(t *testing.T) {
	drivers := reviewedWorkspaceLoginDrivers()
	if err := validateWorkspaceLoginDrivers(drivers); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		id            string
		relayCallback bool
	}{
		{id: "github-device"},
		{id: "twg"},
		{id: "claude"},
		{id: "codex", relayCallback: true},
		{id: "pup", relayCallback: true},
		{id: "github-oauth", relayCallback: true},
	}
	if len(drivers) != len(want) {
		t.Fatalf("reviewed Workspace login driver count = %d, want %d", len(drivers), len(want))
	}
	for index, expected := range want {
		driver := drivers[index]
		if driver.id != expected.id || driver.relayCallback != expected.relayCallback || driver.parseTarget == nil {
			t.Fatalf("reviewed Workspace login driver %d = (%q, %t, parser=%t), want (%q, %t, parser=true)",
				index, driver.id, driver.relayCallback, driver.parseTarget != nil, expected.id, expected.relayCallback)
		}
	}

	// Callers receive values, not mutable registration state.
	drivers[0].id = "changed"
	if fresh := reviewedWorkspaceLoginDrivers(); fresh[0].id != "github-device" {
		t.Fatalf("reviewed Workspace login registry retained caller mutation: %q", fresh[0].id)
	}
}

func TestReviewedWorkspaceLoginDriverRegistrySelectsEveryProvider(t *testing.T) {
	tests := []struct {
		id            string
		target        string
		callbackPort  int
		relayCallback bool
	}{
		{id: "github-device", target: githubDeviceURL},
		{id: "twg", target: syntheticTWGVerificationURL},
		{id: "claude", target: syntheticClaudeWorkspaceAuthorizationURL()},
		{id: "codex", target: syntheticCodexAuthorizationURLWithPort(strings.Repeat("c", 43), strings.Repeat("s", 43), 27890), callbackPort: 27890, relayCallback: true},
		{id: "pup", target: syntheticPupWorkspaceAuthorizationURL(8000, "dashboards_read metrics_read"), callbackPort: 8000, relayCallback: true},
		{id: "github-oauth", target: syntheticGitHubAuthorizationURL(37405, strings.Repeat("a", 20), githubAuthorizationScope), callbackPort: 37405, relayCallback: true},
	}
	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			driver, action, ok := selectWorkspaceLoginDriver(test.target, reviewedWorkspaceLoginDrivers())
			if !ok || driver.id != test.id || action.callbackPort != test.callbackPort || action.relayCallback != test.relayCallback {
				t.Fatalf("Workspace login selection = (%q, %+v, %t), want (%q, port=%d, relay=%t)",
					driver.id, action, ok, test.id, test.callbackPort, test.relayCallback)
			}
			if !validLoginBrowserTarget(test.target) {
				t.Fatal("selected Workspace login target was rejected by the host browser boundary")
			}
		})
	}
}

func TestWorkspaceLoginDriverRegistryFailsClosed(t *testing.T) {
	accept := exactWorkspaceLoginTarget(githubDeviceURL)
	callback := func(string) (workspaceLoginAuthorization, bool) {
		return workspaceLoginAuthorization{callbackPort: 23456}, true
	}
	tests := map[string][]workspaceLoginDriver{
		"empty registry": nil,
		"empty ID": {
			openOnlyWorkspaceLoginDriver("", accept),
		},
		"nil parser": {
			{id: "missing-parser"},
		},
		"duplicate ID": {
			openOnlyWorkspaceLoginDriver("same", accept),
			openOnlyWorkspaceLoginDriver("same", exactWorkspaceLoginTarget("https://example.com/other")),
		},
		"ambiguous target": {
			openOnlyWorkspaceLoginDriver("first", accept),
			openOnlyWorkspaceLoginDriver("second", accept),
		},
		"open-only parser returned callback": {
			openOnlyWorkspaceLoginDriver("wrong-mode", callback),
		},
		"callback parser returned no port": {
			callbackWorkspaceLoginDriver("missing-port", accept),
		},
	}
	for name, drivers := range tests {
		t.Run(name, func(t *testing.T) {
			if driver, action, ok := selectWorkspaceLoginDriver(githubDeviceURL, drivers); ok {
				t.Fatalf("invalid registry selected driver/action = (%q, %+v)", driver.id, action)
			}
		})
	}
}
