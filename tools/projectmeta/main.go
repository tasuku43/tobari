// Command projectmeta prints one validated project metadata field for scripts.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tasuku43/agentic-cli-foundry/tools/internal/projectconfig"
)

func main() {
	field := flag.String("field", "", "metadata field to print")
	rootFlag := flag.String("root", ".", "repository root")
	flag.Parse()
	root, err := filepath.Abs(*rootFlag)
	if err != nil {
		fatal(err)
	}
	config, err := projectconfig.Load(root)
	if err != nil {
		fatal(err)
	}
	values := map[string]string{
		"name": config.Project.Name, "binary_name": config.Project.BinaryName,
		"go_module": config.Project.GoModule, "github_owner": config.Project.GitHubOwner,
		"github_repository": config.Project.GitHubRepository, "description": config.Project.Description,
		"formula_class": config.Project.FormulaClass, "license_spdx": config.Project.LicenseSPDX,
		"security_contact": config.Project.SecurityContact, "profile": config.Profile,
	}
	value, ok := values[*field]
	if !ok {
		fatal(fmt.Errorf("unknown field %q", *field))
	}
	fmt.Println(value)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "projectmeta:", err)
	os.Exit(1)
}
