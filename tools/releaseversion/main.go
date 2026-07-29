// Command releaseversion validates and classifies one public release tag.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tasuku43/agentic-cli-foundry/tools/internal/releaseversion"
)

func main() {
	stableOnly := flag.Bool("stable", false, "require a stable release tag")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "usage: go run ./tools/releaseversion [--stable] <tag>")
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	version, err := releaseversion.ParseReleaseTag(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "releaseversion:", err)
		os.Exit(2)
	}
	if *stableOnly && !version.Stable() {
		fmt.Fprintf(os.Stderr, "releaseversion: tag %q is a prerelease, not a stable release\n", version.Tag)
		os.Exit(2)
	}
	fmt.Printf("version=%s\nstable=%t\n", version.Value, version.Stable())
}
