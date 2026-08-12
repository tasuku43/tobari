// Command runtimeassetid prints a deterministic embedded component source ID.
package main

import (
	"fmt"
	"os"

	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: runtimeassetid <gateway|authbroker>")
		os.Exit(2)
	}
	version, err := runtimeassets.ComponentVersion(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "runtimeassetid:", err)
		os.Exit(1)
	}
	fmt.Println(version)
}
