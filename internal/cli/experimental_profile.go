//go:build tobari_experimental

package cli

type experimentalCLIState struct {
	console operatorConsoleRunner
}

func configureExperimentalCLI(command *CLI) {
	command.console = newOperatorConsoleRunner()
}

func experimentalRuntimeCommandSpecs() []CommandSpec {
	return []CommandSpec{serveSpec()}
}
