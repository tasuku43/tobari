//go:build tobari_dev && tobari_research

package cli

type researchCLIState struct {
	console operatorConsoleRunner
}

func configureResearchCLI(command *CLI) {
	command.console = newOperatorConsoleRunner()
}

func researchRuntimeCommandSpecs() []CommandSpec {
	return []CommandSpec{serveSpec()}
}
