// Command runtimeassetid prints a deterministic embedded component source ID.
package main

import (
	"fmt"
	"os"

	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: runtimeassetid <tobari|gateway|authbroker|exposure-helper|standard-runtime-image>")
		os.Exit(2)
	}
	var version string
	var err error
	if os.Args[1] == "exposure-helper" {
		version, err = runtimeassets.ExposureHelperSourceVersion()
	} else if os.Args[1] == "standard-runtime-image" {
		version, err = runtimeassets.StandardRuntimeImage()
	} else {
		version, err = runtimeassets.ComponentVersion(os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtimeassetid:", err)
		os.Exit(1)
	}
	fmt.Println(version)
}
