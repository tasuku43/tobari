//go:build !tobari_experimental

package cli

// authCommandSpecs deliberately returns no public commands in the standard
// profile. Provider CLIs own authentication state inside each Workspace; the
// Broker command surface is a compile-time repository experiment.
func authCommandSpecs() []CommandSpec { return nil }
