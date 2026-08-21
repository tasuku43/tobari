//go:build !tobari_experimental

package cli

type experimentalCLIState struct{}

func configureExperimentalCLI(*CLI) {}

func experimentalRuntimeCommandSpecs() []CommandSpec {
	return []CommandSpec{}
}
