package projectconfig

// Defaults are runnable template values. Bootstrap keeps this declaration
// unchanged so a generated repository can still prove which exact values were
// replaced.
var Defaults = Project{
	Name:             "Agentic CLI Foundry",
	BinaryName:       "agentic-cli-foundry",
	GoModule:         "github.com/tasuku43/agentic-cli-foundry",
	GitHubOwner:      "tasuku43",
	GitHubRepository: "agentic-cli-foundry",
	Description:      "A Go foundation for building agent-discoverable CLIs with coding agents.",
	FormulaClass:     "AgenticCliFoundry",
	LicenseSPDX:      "MIT",
	SecurityContact:  "security@agentic-cli-foundry.example",
}
