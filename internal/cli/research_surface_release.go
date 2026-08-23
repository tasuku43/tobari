//go:build !tobari_research

package cli

type researchCLIState struct{}

func configureResearchCLI(*CLI) {}

func researchRuntimeCommandSpecs() []CommandSpec {
	return []CommandSpec{}
}
