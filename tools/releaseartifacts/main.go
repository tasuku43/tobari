// Command releaseartifacts creates and verifies the subject metadata that
// accompanies one complete Tobari CLI release archive matrix.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tasuku43/tobari/tools/internal/projectconfig"
	"github.com/tasuku43/tobari/tools/internal/releaseversion"
)

func main() {
	if len(os.Args) != 7 || (os.Args[1] != "create" && os.Args[1] != "verify" && os.Args[1] != "verify-final") {
		fatal(fmt.Errorf("usage: releaseartifacts <create|verify|verify-final> <tag> <revision> <builder-id> <invocation-id> <artifact-directory>"))
	}
	version, err := releaseversion.ParseReleaseTag(os.Args[2])
	if err != nil {
		fatal(err)
	}
	config, err := projectconfig.Load(".")
	if err != nil {
		fatal(err)
	}
	directory, err := filepath.Abs(os.Args[6])
	if err != nil {
		fatal(err)
	}
	request := artifactRequest{
		Tag:          version.Tag,
		Version:      version.Value,
		Revision:     os.Args[3],
		BuilderID:    os.Args[4],
		InvocationID: os.Args[5],
		Directory:    directory,
		Project:      config.Project,
		LicenseSPDX:  config.Project.LicenseSPDX,
		Stable:       version.Stable(),
	}
	if os.Args[1] == "create" {
		err = createReleaseMetadata(request)
	} else if os.Args[1] == "verify" {
		err = verifyReleaseMetadata(request)
	} else {
		err = verifyFinalReleaseAssets(request)
	}
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "releaseartifacts:", err)
	os.Exit(1)
}
