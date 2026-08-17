package dockerruntime

import "fmt"

type workspaceLoginBrowserAction struct {
	callbackPort  int
	relayCallback bool
}

type workspaceLoginTargetParser func(string) (workspaceLoginAuthorization, bool)

type workspaceLoginDriver struct {
	id            string
	relayCallback bool
	parseTarget   workspaceLoginTargetParser
}

// reviewedWorkspaceLoginDrivers returns a fresh copy of the closed compiled
// Workspace login union. Adding a reviewed client requires its exact semantic
// target parser and one entry here; transport, browser opening, callback relay,
// ownership checks, replay limits, and cleanup stay shared by the bridge.
func reviewedWorkspaceLoginDrivers() []workspaceLoginDriver {
	return []workspaceLoginDriver{
		openOnlyWorkspaceLoginDriver("github-device", exactWorkspaceLoginTarget(githubDeviceURL)),
		openOnlyWorkspaceLoginDriver("twg", validatedWorkspaceLoginTarget(validTWGWorkspaceLoginVerificationURL)),
		openOnlyWorkspaceLoginDriver("claude", validatedWorkspaceLoginTarget(validWorkspaceClaudeLoginAuthorizationURL)),
		callbackWorkspaceLoginDriver("codex", parseCodexLoginAuthorizationURL),
		callbackWorkspaceLoginDriver("pup", parsePupWorkspaceLoginAuthorizationURL),
		callbackWorkspaceLoginDriver("github-oauth", parseGitHubLoginAuthorizationURL),
	}
}

func openOnlyWorkspaceLoginDriver(id string, parse workspaceLoginTargetParser) workspaceLoginDriver {
	return workspaceLoginDriver{id: id, parseTarget: parse}
}

func callbackWorkspaceLoginDriver(id string, parse workspaceLoginTargetParser) workspaceLoginDriver {
	return workspaceLoginDriver{id: id, relayCallback: true, parseTarget: parse}
}

func exactWorkspaceLoginTarget(expected string) workspaceLoginTargetParser {
	return func(target string) (workspaceLoginAuthorization, bool) {
		return workspaceLoginAuthorization{}, target == expected
	}
}

func validatedWorkspaceLoginTarget(validate func(string) bool) workspaceLoginTargetParser {
	return func(target string) (workspaceLoginAuthorization, bool) {
		return workspaceLoginAuthorization{}, validate != nil && validate(target)
	}
}

func parseWorkspaceLoginBrowserAction(target string) (workspaceLoginBrowserAction, bool) {
	_, action, ok := selectWorkspaceLoginDriver(target, reviewedWorkspaceLoginDrivers())
	return action, ok
}

func selectWorkspaceLoginDriver(
	target string, drivers []workspaceLoginDriver,
) (workspaceLoginDriver, workspaceLoginBrowserAction, bool) {
	if err := validateWorkspaceLoginDrivers(drivers); err != nil {
		return workspaceLoginDriver{}, workspaceLoginBrowserAction{}, false
	}
	var selected workspaceLoginDriver
	var action workspaceLoginBrowserAction
	matches := 0
	for _, driver := range drivers {
		authorization, ok := driver.parseTarget(target)
		if !ok {
			continue
		}
		candidate := workspaceLoginBrowserAction{
			callbackPort:  authorization.callbackPort,
			relayCallback: driver.relayCallback,
		}
		if !validWorkspaceLoginDriverAction(candidate) {
			return workspaceLoginDriver{}, workspaceLoginBrowserAction{}, false
		}
		selected = driver
		action = candidate
		matches++
	}
	if matches != 1 {
		return workspaceLoginDriver{}, workspaceLoginBrowserAction{}, false
	}
	return selected, action, true
}

func validateWorkspaceLoginDrivers(drivers []workspaceLoginDriver) error {
	if len(drivers) == 0 {
		return fmt.Errorf("reviewed Workspace login driver registry is empty")
	}
	seen := make(map[string]struct{}, len(drivers))
	for index, driver := range drivers {
		if driver.id == "" {
			return fmt.Errorf("reviewed Workspace login driver %d has no ID", index)
		}
		if driver.parseTarget == nil {
			return fmt.Errorf("reviewed Workspace login driver %q has no target parser", driver.id)
		}
		if _, duplicate := seen[driver.id]; duplicate {
			return fmt.Errorf("reviewed Workspace login driver ID %q is duplicated", driver.id)
		}
		seen[driver.id] = struct{}{}
	}
	return nil
}

func validWorkspaceLoginDriverAction(action workspaceLoginBrowserAction) bool {
	if !action.relayCallback {
		return action.callbackPort == 0
	}
	return action.callbackPort >= workspaceMinimumCallbackPort && action.callbackPort <= 65535
}
