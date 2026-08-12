// Package buildidentity defines the public-safe identity of one Tobari build
// and its compiled runtime-image resolver.
package buildidentity

import (
	"fmt"
	"regexp"
)

const (
	UnknownCommit = "unknown"

	RequiredGatewayAPI    = 1
	RequiredAuthBrokerAPI = 1

	DevelopmentBuildCommand = "task build"
	DevelopmentBinary       = "bin/tobari"
)

// ResolverChannel identifies the one image authority compiled into a binary.
type ResolverChannel string

const (
	ResolverPublished   ResolverChannel = "published"
	ResolverDevelopment ResolverChannel = "development"
)

// Component records the source-required and resolver-selected API identities.
type Component struct {
	RequiredAPI int
	SelectedAPI int
}

// Compatible reports whether the selected component implements the required API.
func (c Component) Compatible() bool {
	return c.RequiredAPI > 0 && c.SelectedAPI == c.RequiredAPI
}

// Identity is deterministic, secret-free metadata compiled into one executable.
type Identity struct {
	Version           string
	Commit            string
	ResolverChannel   ResolverChannel
	DevelopmentSource bool
	Gateway           Component
	AuthBroker        Component
}

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// NormalizeCommit makes absent local build metadata explicit.
func NormalizeCommit(commit string) string {
	if commit == "" {
		return UnknownCommit
	}
	return commit
}

// Validate rejects identity states that could misrepresent resolver authority.
func (i Identity) Validate() error {
	if i.Version == "" {
		return fmt.Errorf("CLI version is empty")
	}
	if i.Commit != UnknownCommit && !commitPattern.MatchString(i.Commit) {
		return fmt.Errorf("source commit must be unknown or a full lowercase Git SHA")
	}
	if i.ResolverChannel != ResolverPublished && i.ResolverChannel != ResolverDevelopment {
		return fmt.Errorf("resolver channel is invalid")
	}
	if i.DevelopmentSource != (i.ResolverChannel == ResolverDevelopment) {
		return fmt.Errorf("development source metadata does not match resolver channel")
	}
	if i.Gateway.RequiredAPI <= 0 || i.Gateway.SelectedAPI <= 0 ||
		i.AuthBroker.RequiredAPI <= 0 || i.AuthBroker.SelectedAPI <= 0 {
		return fmt.Errorf("component API identities must be positive")
	}
	return nil
}

// APIsCompatible reports compatibility independently from source-commit completeness.
func (i Identity) APIsCompatible() bool {
	return i.Gateway.Compatible() && i.AuthBroker.Compatible()
}

// Complete reports whether the executable carries a reproducible source identity.
func (i Identity) Complete() bool {
	return i.Version != "" && i.Commit != UnknownCommit
}

// Compatible reports complete build metadata and matching component APIs.
func (i Identity) Compatible() bool {
	return i.Validate() == nil && i.Complete() && i.APIsCompatible()
}

// DevelopmentRecovery returns repository-only commands only for a proven dev build.
func (i Identity) DevelopmentRecovery() (string, string, bool) {
	if !i.DevelopmentSource || i.ResolverChannel != ResolverDevelopment {
		return "", "", false
	}
	return DevelopmentBuildCommand, DevelopmentBinary, true
}
