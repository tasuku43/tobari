// Command contractlint verifies the public capability ledger, external schema
// fixtures, and the executable command catalog through one repository gate.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tasuku43/agentic-cli-foundry/internal/cli"
)

func main() {
	root, err := filepath.Abs(".")
	if err != nil {
		fatal(err)
	}
	catalog := cli.DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		fatal(fmt.Errorf("command catalog is invalid: %w", err))
	}
	issues, err := inspectContracts(root, catalogCapabilityIDs(catalog))
	if err != nil {
		fatal(err)
	}
	if len(issues) != 0 {
		for _, issue := range issues {
			fmt.Fprintf(os.Stderr, "%s: %s\n", issue.Path, issue.Message)
		}
		os.Exit(1)
	}
	fmt.Println("contractlint: OK")
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "contractlint: %v\n", err)
	os.Exit(1)
}

func catalogCapabilityIDs(catalog cli.Catalog) map[string]struct{} {
	ids := make(map[string]struct{})
	for _, command := range catalog.Commands() {
		ids[command.Agent.CapabilityID] = struct{}{}
	}
	return ids
}
