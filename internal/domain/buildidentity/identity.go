// Package buildidentity defines the public-safe identity of one Tobari build
// and its compiled runtime-image resolver.
package buildidentity

import (
	"fmt"
	"regexp"

	"github.com/tasuku43/tobari/internal/domain/capabilitysurface"
)

const (
	UnknownCommit = "unknown"

	RequiredGatewayAPI    = 1
	RequiredAuthBrokerAPI = 1 // research surface only

	ReleaseDevelopmentBuildCommand  = "task build"
	ReleaseDevelopmentBinary        = "bin/tobari"
	ResearchDevelopmentBuildCommand = "task build:dev"
	ResearchDevelopmentBinary       = "bin/tobari-research"
)

// ResolverChannel identifies the one image authority compiled into a binary.
type ResolverChannel string

const (
	ResolverEmbedded    ResolverChannel = "embedded"
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
	CapabilitySurface capabilitysurface.CapabilitySurface
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
	if i.ResolverChannel != ResolverEmbedded && i.ResolverChannel != ResolverDevelopment {
		return fmt.Errorf("resolver channel is invalid")
	}
	if i.DevelopmentSource != (i.ResolverChannel == ResolverDevelopment) {
		return fmt.Errorf("development source metadata does not match resolver channel")
	}
	if err := i.CapabilitySurface.Validate(); err != nil {
		return err
	}
	if i.CapabilitySurface.IncludesResearch() && i.ResolverChannel != ResolverDevelopment {
		return fmt.Errorf("research surface requires the development resolver")
	}
	if i.Gateway.RequiredAPI <= 0 || i.Gateway.SelectedAPI <= 0 {
		return fmt.Errorf("Gateway API identities must be positive")
	}
	if i.CapabilitySurface.IncludesResearch() {
		if i.AuthBroker.RequiredAPI <= 0 || i.AuthBroker.SelectedAPI <= 0 {
			return fmt.Errorf("research Auth Broker API identities must be positive")
		}
	} else if i.AuthBroker != (Component{}) {
		return fmt.Errorf("release surface must not select an Auth Broker API")
	}
	return nil
}

// APIsCompatible reports compatibility independently from source-commit completeness.
func (i Identity) APIsCompatible() bool {
	if !i.Gateway.Compatible() {
		return false
	}
	return !i.CapabilitySurface.IncludesResearch() || i.AuthBroker.Compatible()
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
	if i.CapabilitySurface.IncludesResearch() {
		return ResearchDevelopmentBuildCommand, ResearchDevelopmentBinary, true
	}
	return ReleaseDevelopmentBuildCommand, ReleaseDevelopmentBinary, true
}
