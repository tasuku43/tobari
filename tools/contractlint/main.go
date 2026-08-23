// Command contractlint verifies the public capability ledger, product command
// table, external schema fixtures, and executable command catalog through one
// repository gate.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tasuku43/tobari/internal/cli"
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
	schemaIssues, err := validatePublicJSONSchemaTables(root, catalog)
	if err != nil {
		fatal(err)
	}
	issues = append(issues, schemaIssues...)
	commandIssues, err := validatePublicCommandTable(root, catalog)
	if err != nil {
		fatal(err)
	}
	issues = append(issues, commandIssues...)
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
	for _, command := range catalog.PublicCommands() {
		ids[command.Agent.CapabilityID] = struct{}{}
	}
	return ids
}
