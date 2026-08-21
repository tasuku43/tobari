//go:build tobari_exposure_helper

package runtimeassets

import "embed"

// The helper never materializes runtime state. Keeping its build closure free
// of _helper-source prevents a recursive source snapshot.
//
//go:embed assets/versions.env
var embedded embed.FS
