//go:build !tobari_research

package cli

// authCommandSpecs deliberately returns no public commands in the release
// surface. Provider CLIs own authentication state inside each Workspace; the
// Broker command surface is a compile-time repository research boundary.
func authCommandSpecs() []CommandSpec { return nil }
